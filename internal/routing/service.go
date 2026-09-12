package routing

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"

	"MyApi/internal/channelmanager"
	"MyApi/internal/config"
)

type Service struct {
	channels *channelmanager.Manager
	affinity affinityStore
	cfg      *config.Config
	log      *slog.Logger
}

func New(channels *channelmanager.Manager, rdb *redis.Client, cfg *config.Config, log *slog.Logger) *Service {
	s := &Service{channels: channels, cfg: cfg, log: log}
	if rdb != nil {
		s.affinity = newRedisAffinity(rdb)
	}
	return s
}

func (s *Service) HasModel(model string) bool {
	return s.channels != nil && s.channels.HasModel(model)
}

func (s *Service) UpstreamModel(model string, channelID int64) (string, bool) {
	if s.channels == nil {
		return "", false
	}
	return s.channels.UpstreamModel(model, channelID)
}

func (s *Service) Acquire(id int64) (*channelmanager.Handle, error) {
	if s.channels == nil {
		return nil, channelmanager.ErrDisabled
	}
	return s.channels.Acquire(id)
}

// Order 返回按粘性 / 优先级 / 权重排序后的候选尝试顺序。
func (s *Service) Order(ctx context.Context, model string, userID int64, session string) []channelmanager.ChannelInfo {
	if s.channels == nil {
		return nil
	}
	cands := s.channels.Candidates(model)
	if len(cands) == 0 {
		return nil
	}
	var stickyID int64
	if s.affinity != nil && s.cfg.Routing.StickyEnabled {
		if id, ok, err := s.affinity.Get(ctx, scopeKey(userID, session, model)); err == nil && ok {
			stickyID = id
		}
	}
	return planCandidates(cands, stickyID,
		s.cfg.Routing.MaxAttemptsPerPriority, s.cfg.Routing.MaxTotalAttempts, s.cfg.Routing.TryNextPriority)
}

func (s *Service) SetAffinity(ctx context.Context, userID int64, session, model string, channelID int64) {
	if s.affinity == nil || !s.cfg.Routing.StickyEnabled {
		return
	}
	ttl := time.Duration(s.cfg.Routing.StickyTTLSeconds) * time.Second
	if err := s.affinity.Set(ctx, scopeKey(userID, session, model), channelID, ttl); err != nil {
		s.log.Debug("set affinity failed", "err", err)
	}
}
