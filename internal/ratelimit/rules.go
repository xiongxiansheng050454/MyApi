package ratelimit

import (
	"context"
	"sync"
	"time"

	"MyApi/internal/model"
)

type rateRulesCache struct {
	mu       sync.Mutex
	loadedAt time.Time
	rules    []model.RateLimitRule
}

func (s *Service) loadRules(ctx context.Context) []model.RateLimitRule {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	ttl := time.Duration(s.cfg.RateLimit.CacheTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 2 * time.Second
	}
	if time.Since(s.cache.loadedAt) < ttl && s.cache.rules != nil {
		return s.cache.rules
	}
	if s.db == nil {
		return s.cache.rules
	}
	var rules []model.RateLimitRule
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).
		Order("priority ASC").Order("id ASC").
		Find(&rules).Error; err != nil {
		s.log.Warn("load rate limit rules", "err", err)
		return s.cache.rules
	}
	s.cache.rules = rules
	s.cache.loadedAt = time.Now()
	return rules
}

// Invalidate 清空规则缓存，规则变更后调用。
func (s *Service) Invalidate() {
	s.cache.mu.Lock()
	s.cache.loadedAt = time.Time{}
	s.cache.rules = nil
	s.cache.mu.Unlock()
}
