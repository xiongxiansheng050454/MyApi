package service

import (
	"context"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
)

type usageMeta struct {
	userID        int64
	apiKeyID      int64
	requestID     string
	clientIP      string
	model         string
	channelID     int64
	upstreamModel string
	startedAt     time.Time
	statusCode    int

	injectedUsage bool
	stream        bool

	estimatedInput int
	usageKnown     bool
	inputTokens    int
	outputTokens   int
	cachedTokens   int
	outputChars    int
	ttftMs         *int

	frozenAmount  float64
	balanceFrozen bool
}

func (m *usageMeta) setUsage(in, out, cached int) {
	m.usageKnown = true
	m.inputTokens = in
	m.outputTokens = out
	m.cachedTokens = cached
}

func (m *usageMeta) addOutputChars(n int) { m.outputChars += n }

func (m *usageMeta) markFirstByte() {
	if m.ttftMs == nil {
		ms := int(time.Since(m.startedAt).Milliseconds())
		m.ttftMs = &ms
	}
}

type dailyAggregator interface {
	Apply(ctx context.Context, date string, per map[int64]usageDelta, maxID int64) error
}

type gormDaily struct {
	db *gorm.DB
}

func (g *gormDaily) Apply(ctx context.Context, date string, per map[int64]usageDelta, maxID int64) error {
	statDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for uid, d := range per {
			row := model.UserDailyStat{
				UserID:                 uid,
				StatDate:               statDate,
				TotalInputTokens:       d.Input,
				TotalOutputTokens:      d.Output,
				TotalCachedInputTokens: d.Cached,
				TotalTokens:            d.Input + d.Output,
				TotalCost:              float64(d.CostMicro) / 1e8,
				RequestCount:           int(d.Req),
				SuccessCount:           int(d.OK),
				ErrorCount:             int(d.Err),
				LastProcessedID:        maxID,
			}
			err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "stat_date"}},
				DoUpdates: clause.Assignments(map[string]any{
					"total_input_tokens":        gorm.Expr("user_daily_stats.total_input_tokens + ?", d.Input),
					"total_output_tokens":       gorm.Expr("user_daily_stats.total_output_tokens + ?", d.Output),
					"total_cached_input_tokens": gorm.Expr("user_daily_stats.total_cached_input_tokens + ?", d.Cached),
					"total_tokens":              gorm.Expr("user_daily_stats.total_tokens + ?", d.Input+d.Output),
					"total_cost":                gorm.Expr("user_daily_stats.total_cost + ?", float64(d.CostMicro)/1e8),
					"request_count":             gorm.Expr("user_daily_stats.request_count + ?", int(d.Req)),
					"success_count":             gorm.Expr("user_daily_stats.success_count + ?", int(d.OK)),
					"error_count":               gorm.Expr("user_daily_stats.error_count + ?", int(d.Err)),
					"last_processed_id":         gorm.Expr("GREATEST(user_daily_stats.last_processed_id, ?)", maxID),
					"updated_at":                time.Now(),
				}),
			}).Create(&row).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) lookupPricing(ctx context.Context, channelID int64, modelName string) (in, out, cached float64) {
	db := s.WithContext(ctx)
	if db == nil {
		return 0, 0, 0
	}
	var p model.ModelPricing
	if err := db.Where("channel_id = ? AND model_name = ?", channelID, modelName).First(&p).Error; err != nil {
		return 0, 0, 0
	}
	cached = p.InputPricePer1M
	if p.CachedInputPricePer1M != nil {
		cached = *p.CachedInputPricePer1M
	}
	return p.InputPricePer1M, p.OutputPricePer1M, cached
}

func computeCost(input, output, cached int, pIn, pOut, pCached float64) float64 {
	uncached := input - cached
	if uncached < 0 {
		uncached = 0
	}
	return (float64(uncached)*pIn + float64(cached)*pCached + float64(output)*pOut) / 1e6
}

func (s *Service) statDate(t time.Time) string {
	if s.statsLoc != nil {
		t = t.In(s.statsLoc)
	}
	return t.Format("2006-01-02")
}

func (s *Service) finalizeUsage(resp *CompletionResponse) {
	meta := resp.meta
	if meta == nil {
		return
	}

	input, output, cached := meta.inputTokens, meta.outputTokens, meta.cachedTokens
	if !meta.usageKnown {
		input = meta.estimatedInput
		per := s.cfg.RateLimit.CharsPerToken
		if per <= 0 {
			per = 4
		}
		output = meta.outputChars / per
		if meta.outputChars%per > 0 {
			output++
		}
	}
	if input < 0 {
		input = 0
	}

	pIn, pOut, pCached := s.lookupPricing(context.Background(), meta.channelID, meta.model)
	cost := computeCost(input, output, cached, pIn, pOut, pCached)

	status := model.StatusSuccess
	var errorCode *string
	if meta.statusCode < 200 || meta.statusCode >= 300 {
		status = model.StatusError
		ec := "upstream_error"
		errorCode = &ec
	}

	upstream := meta.upstreamModel
	row := model.UsageLog{
		RequestID:            meta.requestID,
		UserID:               meta.userID,
		ApiKeyID:             meta.apiKeyID,
		ChannelID:            meta.channelID,
		Model:                meta.model,
		UpstreamModel:        &upstream,
		InputTokens:          input,
		OutputTokens:         output,
		CachedInputTokens:    cached,
		UnitPriceInputPer1M:  pIn,
		UnitPriceOutputPer1M: pOut,
		TotalCost:            cost,
		DurationMs:           int(time.Since(meta.startedAt).Milliseconds()),
		TtftMs:               meta.ttftMs,
		Status:               status,
		ErrorCode:            errorCode,
	}
	var clientIP *string
	if meta.clientIP != "" {
		ip := meta.clientIP
		clientIP = &ip
	}
	row.ClientIP = clientIP

	db := s.DB
	if db != nil {
		if err := db.WithContext(context.Background()).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&row).Error; err != nil {
			s.log.Error("insert usage log", "err", err, "request_id", meta.requestID)
		}
	}

	if meta.balanceFrozen && s.billing != nil && s.cfg.Billing.Enabled {
		if err := s.billing.Settle(context.Background(), meta.userID, meta.requestID, meta.frozenAmount, cost); err != nil {
			s.log.Error("settle balance", "err", err, "request_id", meta.requestID, "user_id", meta.userID)
		}
	}

	delta := usageDelta{
		Input:     int64(input),
		Output:    int64(output),
		Cached:    int64(cached),
		CostMicro: int64(math.Round(cost * 1e8)),
		Req:       1,
	}
	if status == model.StatusSuccess {
		delta.OK = 1
	} else {
		delta.Err = 1
	}

	date := s.statDate(time.Now())
	if s.delta != nil {
		if err := s.delta.Add(context.Background(), date, meta.userID, delta, row.ID); err != nil {
			s.log.Warn("redis usage delta failed, fallback to db", "err", err)
			s.applyDeltaDirect(date, meta.userID, delta, row.ID)
		}
		return
	}
	s.applyDeltaDirect(date, meta.userID, delta, row.ID)
}

func (s *Service) applyDeltaDirect(date string, uid int64, d usageDelta, maxID int64) {
	if s.daily == nil {
		return
	}
	if err := s.daily.Apply(context.Background(), date, map[int64]usageDelta{uid: d}, maxID); err != nil {
		s.log.Error("apply daily stats", "err", err, "user_id", uid, "date", date)
	}
}
