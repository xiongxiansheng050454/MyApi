package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"MyApi/internal/model"
)

type rateRulesCache struct {
	mu       sync.Mutex
	loadedAt time.Time
	rules    []model.RateLimitRule
}

func (s *Service) loadRateRules(ctx context.Context) []model.RateLimitRule {
	s.rateCache.mu.Lock()
	defer s.rateCache.mu.Unlock()
	ttl := time.Duration(s.cfg.RateLimit.CacheTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 2 * time.Second
	}
	if time.Since(s.rateCache.loadedAt) < ttl && s.rateCache.rules != nil {
		return s.rateCache.rules
	}
	db := s.WithContext(ctx)
	if db == nil {
		return s.rateCache.rules
	}
	var rules []model.RateLimitRule
	if err := db.Where("enabled = ?", true).
		Order("priority ASC").Order("id ASC").
		Find(&rules).Error; err != nil {
		s.log.Warn("load rate limit rules", "err", err)
		return s.rateCache.rules
	}
	s.rateCache.rules = rules
	s.rateCache.loadedAt = time.Now()
	s.log.Debug("rate limit rules loaded", slog.Int("count", len(rules)))
	return rules
}

func (s *Service) invalidateRateRules() {
	s.rateCache.mu.Lock()
	s.rateCache.loadedAt = time.Time{}
	s.rateCache.rules = nil
	s.rateCache.mu.Unlock()
}

func (s *Service) InvalidateRateCache() { s.invalidateRateRules() }
