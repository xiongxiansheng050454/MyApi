package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"MyApi/internal/model"
)

type RateLimitedError struct {
	RetryAfter int64
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("rate limit exceeded, retry after %ds", e.RetryAfter)
}

var ErrQueueTimeout = errors.New("rate limit queue timeout")

var metricDefaultWindow = map[string]int64{
	model.MetricRPM: 60,
	model.MetricRPD: 86400,
	model.MetricTPM: 60,
	model.MetricTPD: 86400,
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

func targetMatches(tt, tv string, ident *KeyIdentity, modelVal string) bool {
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

func selectRulesForMetric(rules []model.RateLimitRule, ident *KeyIdentity, modelVal, metric string) []model.RateLimitRule {
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

// rateLimitGate 在转发前执行限流/限额；返回的可选 release 用于释放 concurrency 占用（请求结束时调用）。
func (s *Service) rateLimitGate(ctx context.Context, req *ChatCompletionRequest) (func(), error) {
	if s.counter == nil {
		return nil, nil
	}

	rules := s.loadRateRules(ctx)
	overrides := req.Key.Overrides

	var tokenDelta int64
	needTokens := func() int64 {
		if tokenDelta == 0 {
			tokenDelta = s.estimateTokens(req.Body, req.Model)
		}
		return tokenDelta
	}

	for _, metric := range []string{model.MetricRPM, model.MetricRPD, model.MetricTPM, model.MetricTPD} {
		if ov, ok := overrides[metric]; ok {
			window := metricDefaultWindow[metric]
			if window <= 0 {
				window = 60
			}
			if err := s.settleSliding(ctx, model.TargetGlobal, "*", metric, window, ov, model.ActionReject, time.Duration(0), countDelta(metric, needTokens)); err != nil {
				return nil, err
			}
			continue
		}
		chosen := selectRulesForMetric(rules, req.Key, req.Model, metric)
		for i := range chosen {
			r := &chosen[i]
			if err := s.settleSliding(ctx, r.TargetType, r.TargetValue, metric,
				int64(r.WindowSeconds), int64(r.LimitValue), r.Action, queueTimeoutSeconds(r), countDelta(metric, needTokens)); err != nil {
				return nil, err
			}
		}
	}

	return s.acquireConcurrency(ctx, req, rules, overrides)
}

func countDelta(metric string, tokens func() int64) int64 {
	switch metric {
	case model.MetricTPM, model.MetricTPD:
		return tokens()
	default:
		return 1
	}
}

func (s *Service) slidingKey(targetType, targetValue, metric string, window int64) string {
	return "rl:" + metric + ":" + targetType + ":" + targetValue + ":" + strconv.FormatInt(window, 10)
}

func (s *Service) settleSliding(ctx context.Context, targetType, targetValue, metric string, window, limit int64, action string, queueTimeout time.Duration, delta int64) error {
	if delta <= 0 {
		return nil
	}
	base := s.slidingKey(targetType, targetValue, metric, window)
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
			return &RateLimitedError{RetryAfter: retry}
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return ErrQueueTimeout
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
			return ErrQueueTimeout
		}
	}
}

const concurrencyTTL int64 = 3600

func (s *Service) acquireConcurrency(ctx context.Context, req *ChatCompletionRequest, rules []model.RateLimitRule, overrides map[string]int64) (func(), error) {
	var cap int64
	var key string
	var action string
	var queueTimeout time.Duration

	if ov, ok := overrides[model.MetricConcurrency]; ok {
		cap, action = ov, model.ActionReject
		key = "c:ovr:" + strconv.FormatInt(req.Key.ApiKeyID, 10)
	} else {
		chosen := selectRulesForMetric(rules, req.Key, req.Model, model.MetricConcurrency)
		if len(chosen) == 0 {
			return nil, nil
		}
		cap = int64(chosen[0].LimitValue)
		for i := 1; i < len(chosen); i++ {
			if int64(chosen[i].LimitValue) < cap {
				cap = int64(chosen[i].LimitValue)
			}
		}
		r := &chosen[0]
		action = r.Action
		queueTimeout = queueTimeoutSeconds(r)
		key = "c:" + r.TargetType + ":" + r.TargetValue
	}
	if cap <= 0 {
		return nil, nil
	}

	var deadline time.Time
	if action == model.ActionQueue {
		deadline = time.Now().Add(queueTimeout)
	}
	for {
		okAcq, err := s.counter.TryAcquire(ctx, key, cap, concurrencyTTL)
		if err != nil {
			return nil, err
		}
		if okAcq {
			return func() { _ = s.counter.Release(context.Background(), key) }, nil
		}
		if action != model.ActionQueue {
			return nil, &RateLimitedError{RetryAfter: 1}
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, ErrQueueTimeout
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, ErrQueueTimeout
		}
	}
}
