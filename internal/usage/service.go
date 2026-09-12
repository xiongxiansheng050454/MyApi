package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/config"
	"MyApi/internal/model"
)

type Service struct {
	db    *gorm.DB
	delta deltaStore
	daily dailyAggregator
	cfg   *config.Config
	log   *slog.Logger
	loc   *time.Location

	flushCtx    context.Context
	flushCancel context.CancelFunc
	flushWG     sync.WaitGroup
}

func New(db *gorm.DB, rdb *redis.Client, cfg *config.Config, log *slog.Logger) *Service {
	s := &Service{db: db, cfg: cfg, log: log}
	if rdb != nil {
		s.delta = newRedisDeltaStore(rdb, int64(cfg.Stats.RedisTTLHours))
	}
	if db != nil {
		s.daily = &gormDaily{db: db}
	}
	if loc, err := time.LoadLocation(cfg.Stats.Timezone); err == nil {
		s.loc = loc
	} else {
		log.Warn("invalid stats.timezone, fallback UTC", "timezone", cfg.Stats.Timezone, "err", err)
		s.loc = time.UTC
	}
	return s
}

func (s *Service) Start() {
	if s.delta != nil && s.daily != nil {
		s.startFlusher()
	}
}

func (s *Service) Stop() { s.stopFlusher() }

type RecordInput struct {
	RequestID     string
	UserID        int64
	APIKeyID      int64
	ChannelID     int64
	Model         string
	UpstreamModel string
	Input         int
	Output        int
	Cached        int
	PriceIn       float64
	PriceOut      float64
	Cost          float64
	DurationMs    int
	TTFTMs        *int
	Status        string
	ErrorCode     *string
	ClientIP      string
}

// Record 写入一条用量明细（request_id 冲突则忽略）。
func (s *Service) Record(in RecordInput) int64 {
	if s.db == nil {
		return 0
	}
	upstream := in.UpstreamModel
	row := model.UsageLog{
		RequestID:            in.RequestID,
		UserID:               in.UserID,
		ApiKeyID:             in.APIKeyID,
		ChannelID:            in.ChannelID,
		Model:                in.Model,
		UpstreamModel:        &upstream,
		InputTokens:          in.Input,
		OutputTokens:         in.Output,
		CachedInputTokens:    in.Cached,
		UnitPriceInputPer1M:  in.PriceIn,
		UnitPriceOutputPer1M: in.PriceOut,
		TotalCost:            in.Cost,
		DurationMs:           in.DurationMs,
		TtftMs:               in.TTFTMs,
		Status:               in.Status,
		ErrorCode:            in.ErrorCode,
	}
	if in.ClientIP != "" {
		ip := in.ClientIP
		row.ClientIP = &ip
	}
	if err := s.db.WithContext(context.Background()).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		s.log.Error("insert usage log", "err", err, "request_id", row.RequestID)
		return 0
	}
	return row.ID
}

func (s *Service) StatDate(t time.Time) string {
	if s.loc != nil {
		t = t.In(s.loc)
	}
	return t.Format("2006-01-02")
}

func (s *Service) AddDelta(uid int64, input, output, cached int, costMicro int64, ok bool, maxID int64) {
	d := usageDelta{
		Input:     int64(input),
		Output:    int64(output),
		Cached:    int64(cached),
		CostMicro: costMicro,
		Req:       1,
	}
	if ok {
		d.OK = 1
	} else {
		d.Err = 1
	}
	date := s.StatDate(time.Now())
	if s.delta != nil {
		if err := s.delta.Add(context.Background(), date, uid, d, maxID); err != nil {
			s.log.Warn("redis usage delta failed, fallback to db", "err", err)
			s.applyDeltaDirect(date, uid, d, maxID)
		}
		return
	}
	s.applyDeltaDirect(date, uid, d, maxID)
}

func (s *Service) applyDeltaDirect(date string, uid int64, d usageDelta, maxID int64) {
	if s.daily == nil {
		return
	}
	if err := s.daily.Apply(context.Background(), date, map[int64]usageDelta{uid: d}, maxID); err != nil {
		s.log.Error("apply daily stats", "err", err, "user_id", uid, "date", date)
	}
}

func (s *Service) startFlusher() {
	s.flushCtx, s.flushCancel = context.WithCancel(context.Background())
	s.flushWG.Add(1)
	go s.flushLoop()
}

func (s *Service) stopFlusher() {
	if s.flushCancel != nil {
		s.flushCancel()
	}
	s.flushWG.Wait()
}

func (s *Service) flushLoop() {
	defer s.flushWG.Done()
	interval := time.Duration(s.cfg.Stats.FlushIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.flushCtx.Done():
			return
		case <-ticker.C:
			s.flushOnce()
		}
	}
}

func (s *Service) flushOnce() {
	if s.delta == nil || s.daily == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dates, err := s.delta.ActiveDates(ctx)
	if err != nil {
		s.log.Warn("stats flush: list active dates", "err", err)
		return
	}
	for _, date := range dates {
		proc, ok, err := s.delta.Rename(ctx, date)
		if err != nil {
			s.log.Warn("stats flush: rename", "err", err, "date", date)
			continue
		}
		if !ok {
			continue
		}
		per, maxID, err := s.delta.Read(ctx, proc)
		if err != nil {
			s.log.Warn("stats flush: read", "err", err, "date", date)
			_ = s.delta.MergeBack(ctx, date, proc)
			continue
		}
		if len(per) == 0 {
			_ = s.delta.Drop(ctx, date, proc)
			continue
		}
		if err := s.daily.Apply(ctx, date, per, maxID); err != nil {
			s.log.Error("stats flush: apply daily", "err", err, "date", date)
			if merr := s.delta.MergeBack(ctx, date, proc); merr != nil {
				s.log.Error("stats flush: merge back", "err", merr, "date", date)
			}
			continue
		}
		if err := s.delta.Drop(ctx, date, proc); err != nil {
			s.log.Warn("stats flush: drop", "err", err, "date", date)
		}
	}
}
