package ratelimit

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"MyApi/internal/auth"
	"MyApi/internal/config"
	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/platform/cachekeys"
)

var metricDefaultWindow = map[string]int64{
	model.MetricRPM: 60,
	model.MetricRPD: 86400,
	model.MetricTPM: 60,
	model.MetricTPD: 86400,
}

type Service struct {
	db      *gorm.DB
	counter counterStore
	cache   rateRulesCache
	cfg     *config.Config
	log     *slog.Logger
}

func New(db *gorm.DB, rdb *redis.Client, cfg *config.Config, log *slog.Logger) *Service {
	s := &Service{db: db, cfg: cfg, log: log}
	if rdb != nil {
		s.counter = newRedisCounter(rdb)
	}
	return s
}

func scopeRank(tt string) int {
	switch tt {
	case model.TargetAPIKey:
		return 0
	case model.TargetUser:
		return 1
	case model.TargetModel:
		return 2
	case model.TargetGlobal:
		return 3
	default:
		return 9
	}
}

func targetMatches(tt, tv string, ident *auth.Identity, modelVal string) bool {
	switch tt {
	case model.TargetAPIKey:
		return tv == "*" || tv == strconv.FormatInt(ident.ApiKeyID, 10)
	case model.TargetUser:
		return tv == "*" || tv == strconv.FormatInt(ident.UserID, 10)
	case model.TargetModel:
		return tv == "*" || tv == modelVal
	case model.TargetGlobal:
		return true
	default:
		return false
	}
}

func selectRulesForMetric(rules []model.RateLimitRule, ident *auth.Identity, modelVal, metric string) []model.RateLimitRule {
	var best []model.RateLimitRule
	bestRank := 99
	for _, r := range rules {
		if r.Metric != metric || !targetMatches(r.TargetType, r.TargetValue, ident, modelVal) {
			continue
		}
		rank := scopeRank(r.TargetType)
		if rank > bestRank {
			continue
		}
		if rank < bestRank {
			bestRank = rank
			best = best[:0]
		}
		best = append(best, r)
	}
	return best
}

func queueTimeoutSeconds(r *model.RateLimitRule) time.Duration {
	if len(r.Extras) > 0 {
		var extras map[string]any
		if json.Unmarshal(r.Extras, &extras) == nil {
			if v, ok := extras["queue_timeout_seconds"].(float64); ok && v > 0 {
				return time.Duration(v * float64(time.Second))
			}
		}
	}
	return 5 * time.Second
}

// Gate 在转发前执行限流/限额；返回的可选 release 用于释放 concurrency 占用。
// tokens 为惰性 token 计数（仅 tpm/tpd 需要时调用）。
func (s *Service) Gate(ctx context.Context, ident *auth.Identity, modelVal string, tokens func() int64) (func(), error) {
	if s.counter == nil {
		return nil, nil
	}

	rules := s.loadRules(ctx)
	overrides := ident.Overrides

	for _, metric := range []string{model.MetricRPM, model.MetricRPD, model.MetricTPM, model.MetricTPD} {
		if ov, ok := overrides[metric]; ok {
			window := metricDefaultWindow[metric]
			if window <= 0 {
				window = 60
			}
			if err := s.settleSliding(ctx, model.TargetGlobal, "*", metric, window, ov, model.ActionReject, 0, countDelta(metric, tokens)); err != nil {
				return nil, err
			}
			continue
		}
		chosen := selectRulesForMetric(rules, ident, modelVal, metric)
		for i := range chosen {
			r := &chosen[i]
			if err := s.settleSliding(ctx, r.TargetType, r.TargetValue, metric,
				int64(r.WindowSeconds), int64(r.LimitValue), r.Action, queueTimeoutSeconds(r), countDelta(metric, tokens)); err != nil {
				return nil, err
			}
		}
	}

	return s.acquireConcurrency(ctx, ident, rules, overrides)
}

func countDelta(metric string, tokens func() int64) int64 {
	switch metric {
	case model.MetricTPM, model.MetricTPD:
		return tokens()
	default:
		return 1
	}
}

func (s *Service) settleSliding(ctx context.Context, targetType, targetValue, metric string, window, limit int64, action string, queueTimeout time.Duration, delta int64) error {
	if delta <= 0 {
		return nil
	}
	base := cachekeys.RateLimitBase(targetType, targetValue, metric, window)
	now := time.Now().Unix()
	var deadline time.Time
	if action == model.ActionQueue {
		deadline = time.Now().Add(queueTimeout)
	}
	for {
		allowed, retry, err := s.counter.AllowSliding(ctx, base, now, window, limit, delta)
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
		if action != model.ActionQueue {
			return &apperr.RateLimitedError{RetryAfter: retry}
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return apperr.ErrQueueTimeout
		}
		wait := 100 * time.Millisecond
		if retry > 0 {
			if w := time.Duration(retry) * time.Second; w < wait {
				wait = w
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return apperr.ErrQueueTimeout
		}
	}
}

const concurrencyTTL int64 = 3600

func (s *Service) acquireConcurrency(ctx context.Context, ident *auth.Identity, rules []model.RateLimitRule, overrides map[string]int64) (func(), error) {
	var capacity int64
	var key string
	var action string
	var queueTimeout time.Duration

	if ov, ok := overrides[model.MetricConcurrency]; ok {
		capacity, action = ov, model.ActionReject
		key = "c:ovr:" + strconv.FormatInt(ident.ApiKeyID, 10)
	} else {
		chosen := selectRulesForMetric(rules, ident, "", model.MetricConcurrency)
		if len(chosen) == 0 {
			return nil, nil
		}
		capacity = int64(chosen[0].LimitValue)
		for i := 1; i < len(chosen); i++ {
			if int64(chosen[i].LimitValue) < capacity {
				capacity = int64(chosen[i].LimitValue)
			}
		}
		r := &chosen[0]
		action = r.Action
		queueTimeout = queueTimeoutSeconds(r)
		key = "c:" + r.TargetType + ":" + r.TargetValue
	}
	if capacity <= 0 {
		return nil, nil
	}

	var deadline time.Time
	if action == model.ActionQueue {
		deadline = time.Now().Add(queueTimeout)
	}
	for {
		okAcq, err := s.counter.TryAcquire(ctx, key, capacity, concurrencyTTL)
		if err != nil {
			return nil, err
		}
		if okAcq {
			return func() { _ = s.counter.Release(context.Background(), key) }, nil
		}
		if action != model.ActionQueue {
			return nil, &apperr.RateLimitedError{RetryAfter: 1}
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, apperr.ErrQueueTimeout
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, apperr.ErrQueueTimeout
		}
	}
}
