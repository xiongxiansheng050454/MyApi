package usage

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/platform/money"
)

var ErrLogNotFound = errors.New("usage log not found")

type LogFilter struct {
	UserID    int64
	APIKeyID  int64
	ChannelID int64
	Model     string
	Status    string
	ErrorCode string
	RequestID string
	Start     *time.Time
	End       *time.Time
	Page      int
	PageSize  int
}

type LogItem struct {
	ID                   int64     `json:"id"`
	RequestID            string    `json:"request_id"`
	UserID               int64     `json:"user_id"`
	APIKeyID             int64     `json:"api_key_id"`
	ChannelID            int64     `json:"channel_id"`
	ChannelName          string    `json:"channel_name"`
	Model                string    `json:"model"`
	UpstreamModel        *string   `json:"upstream_model"`
	InputTokens          int       `json:"input_tokens"`
	OutputTokens         int       `json:"output_tokens"`
	CachedInputTokens    int       `json:"cached_input_tokens"`
	TotalTokens          int       `json:"total_tokens"`
	UnitPriceInputPer1M  string    `json:"unit_price_input_per_1m"`
	UnitPriceOutputPer1M string    `json:"unit_price_output_per_1m"`
	TotalCost            string    `json:"total_cost"`
	DurationMs           int       `json:"duration_ms"`
	TTFTMs               *int      `json:"ttft_ms"`
	Status               string    `json:"status"`
	ErrorCode            *string   `json:"error_code"`
	ClientIP             *string   `json:"client_ip"`
	CreatedAt            time.Time `json:"created_at"`
}

func (s *Service) channelNameMap(ctx context.Context, ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 || s.db == nil {
		return out
	}
	var rows []model.Channel
	_ = s.db.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&rows).Error
	for i := range rows {
		out[rows[i].ID] = rows[i].Name
	}
	return out
}

func logToItem(r *model.UsageLog, names map[int64]string) LogItem {
	return LogItem{
		ID: r.ID, RequestID: r.RequestID, UserID: r.UserID, APIKeyID: r.ApiKeyID,
		ChannelID: r.ChannelID, ChannelName: names[r.ChannelID],
		Model: r.Model, UpstreamModel: r.UpstreamModel,
		InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
		CachedInputTokens: r.CachedInputTokens, TotalTokens: r.InputTokens + r.OutputTokens,
		UnitPriceInputPer1M:  money.Format6(r.UnitPriceInputPer1M),
		UnitPriceOutputPer1M: money.Format6(r.UnitPriceOutputPer1M),
		TotalCost:            money.Format6(r.TotalCost),
		DurationMs:           r.DurationMs, TTFTMs: r.TtftMs,
		Status: r.Status, ErrorCode: r.ErrorCode, ClientIP: r.ClientIP,
		CreatedAt: r.CreatedAt,
	}
}

func (s *Service) ListLogs(ctx context.Context, f LogFilter) ([]LogItem, int64, error) {
	if s.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Model(&model.UsageLog{})
	if f.UserID > 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.APIKeyID > 0 {
		q = q.Where("api_key_id = ?", f.APIKeyID)
	}
	if f.ChannelID > 0 {
		q = q.Where("channel_id = ?", f.ChannelID)
	}
	if f.Model != "" {
		q = q.Where("model = ?", f.Model)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.ErrorCode != "" {
		q = q.Where("error_code = ?", f.ErrorCode)
	}
	if f.RequestID != "" {
		q = q.Where("request_id = ?", f.RequestID)
	}
	if f.Start != nil {
		q = q.Where("created_at >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("created_at <= ?", *f.End)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.UsageLog
	if err := q.Order("id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ChannelID)
	}
	names := s.channelNameMap(ctx, ids)
	list := make([]LogItem, 0, len(rows))
	for i := range rows {
		list = append(list, logToItem(&rows[i], names))
	}
	return list, total, nil
}

func (s *Service) GetLog(ctx context.Context, id int64) (*LogItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var row model.UsageLog
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLogNotFound
		}
		return nil, err
	}
	names := s.channelNameMap(ctx, []int64{row.ChannelID})
	item := logToItem(&row, names)
	return &item, nil
}

type DailyFilter struct {
	UserID   int64
	DateFrom string
	DateTo   string
	Page     int
	PageSize int
}

type DailyItem struct {
	UserID                 int64  `json:"user_id"`
	StatDate               string `json:"stat_date"`
	TotalInputTokens       int64  `json:"total_input_tokens"`
	TotalOutputTokens      int64  `json:"total_output_tokens"`
	TotalCachedInputTokens int64  `json:"total_cached_input_tokens"`
	TotalTokens            int64  `json:"total_tokens"`
	TotalCost              string `json:"total_cost"`
	RequestCount           int    `json:"request_count"`
	SuccessCount           int    `json:"success_count"`
	ErrorCount             int    `json:"error_count"`
}

func (s *Service) StatsDaily(ctx context.Context, f DailyFilter) ([]DailyItem, int64, error) {
	if s.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Model(&model.UserDailyStat{})
	if f.UserID > 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.DateFrom != "" {
		q = q.Where("stat_date >= ?", f.DateFrom)
	}
	if f.DateTo != "" {
		q = q.Where("stat_date <= ?", f.DateTo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.UserDailyStat
	if err := q.Order("stat_date DESC").Order("user_id ASC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	list := make([]DailyItem, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		list = append(list, DailyItem{
			UserID: r.UserID, StatDate: r.StatDate.Format("2006-01-02"),
			TotalInputTokens: r.TotalInputTokens, TotalOutputTokens: r.TotalOutputTokens,
			TotalCachedInputTokens: r.TotalCachedInputTokens, TotalTokens: r.TotalTokens,
			TotalCost: money.Format6(r.TotalCost), RequestCount: r.RequestCount,
			SuccessCount: r.SuccessCount, ErrorCount: r.ErrorCount,
		})
	}
	return list, total, nil
}

type Overview struct {
	RequestCount      int64  `json:"request_count"`
	SuccessCount      int64  `json:"success_count"`
	ErrorCount        int64  `json:"error_count"`
	TotalInputTokens  int64  `json:"total_input_tokens"`
	TotalOutputTokens int64  `json:"total_output_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
	TotalCost         string `json:"total_cost"`
	ActiveUserCount   int64  `json:"active_user_count"`
}

func (s *Service) StatsOverview(ctx context.Context, start, end *time.Time) (*Overview, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Model(&model.UsageLog{})
	if start != nil {
		q = q.Where("created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("created_at <= ?", *end)
	}
	var agg struct {
		Requests   int64   `gorm:"column:requests"`
		Success    int64   `gorm:"column:success"`
		Errors     int64   `gorm:"column:errors"`
		Input      int64   `gorm:"column:input"`
		Output     int64   `gorm:"column:output"`
		Cost       float64 `gorm:"column:cost"`
		ActiveUser int64   `gorm:"column:active_user"`
	}
	err := q.Select(
		"COUNT(*) AS requests, " +
			"COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END),0) AS success, " +
			"COALESCE(SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END),0) AS errors, " +
			"COALESCE(SUM(input_tokens),0) AS input, " +
			"COALESCE(SUM(output_tokens),0) AS output, " +
			"COALESCE(SUM(total_cost),0) AS cost, " +
			"COUNT(DISTINCT user_id) AS active_user").
		Scan(&agg).Error
	if err != nil {
		return nil, err
	}
	return &Overview{
		RequestCount:      agg.Requests,
		SuccessCount:      agg.Success,
		ErrorCount:        agg.Errors,
		TotalInputTokens:  agg.Input,
		TotalOutputTokens: agg.Output,
		TotalTokens:       agg.Input + agg.Output,
		TotalCost:         money.Format6(agg.Cost),
		ActiveUserCount:   agg.ActiveUser,
	}, nil
}

type ChannelStat struct {
	ChannelID         int64  `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	RequestCount      int64  `json:"request_count"`
	TotalInputTokens  int64  `json:"total_input_tokens"`
	TotalOutputTokens int64  `json:"total_output_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
	TotalCost         string `json:"total_cost"`
	SuccessCount      int64  `json:"success_count"`
	ErrorCount        int64  `json:"error_count"`
}

func (s *Service) StatsChannels(ctx context.Context, channelID int64, start, end *time.Time) ([]ChannelStat, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Table("usage_logs AS u").
		Select("u.channel_id AS channel_id, COALESCE(c.name,'') AS channel_name, " +
			"COUNT(*) AS request_count, " +
			"COALESCE(SUM(u.input_tokens),0) AS total_input_tokens, " +
			"COALESCE(SUM(u.output_tokens),0) AS total_output_tokens, " +
			"COALESCE(SUM(u.input_tokens + u.output_tokens),0) AS total_tokens, " +
			"COALESCE(SUM(u.total_cost),0) AS total_cost, " +
			"COALESCE(SUM(CASE WHEN u.status='success' THEN 1 ELSE 0 END),0) AS success_count, " +
			"COALESCE(SUM(CASE WHEN u.status<>'success' THEN 1 ELSE 0 END),0) AS error_count").
		Joins("LEFT JOIN channels c ON c.id = u.channel_id").
		Group("u.channel_id, c.name").
		Order("total_cost DESC")
	if channelID > 0 {
		q = q.Where("u.channel_id = ?", channelID)
	}
	if start != nil {
		q = q.Where("u.created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("u.created_at <= ?", *end)
	}
	var rows []struct {
		ChannelID         int64   `gorm:"column:channel_id"`
		ChannelName       string  `gorm:"column:channel_name"`
		RequestCount      int64   `gorm:"column:request_count"`
		TotalInputTokens  int64   `gorm:"column:total_input_tokens"`
		TotalOutputTokens int64   `gorm:"column:total_output_tokens"`
		TotalTokens       int64   `gorm:"column:total_tokens"`
		TotalCost         float64 `gorm:"column:total_cost"`
		SuccessCount      int64   `gorm:"column:success_count"`
		ErrorCount        int64   `gorm:"column:error_count"`
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]ChannelStat, 0, len(rows))
	for _, r := range rows {
		list = append(list, ChannelStat{
			ChannelID: r.ChannelID, ChannelName: r.ChannelName,
			RequestCount: r.RequestCount, TotalInputTokens: r.TotalInputTokens,
			TotalOutputTokens: r.TotalOutputTokens, TotalTokens: r.TotalTokens,
			TotalCost: money.Format6(r.TotalCost), SuccessCount: r.SuccessCount, ErrorCount: r.ErrorCount,
		})
	}
	return list, nil
}
