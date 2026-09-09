package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-redis/redis_rate/v9"
	"github.com/sony/gobreaker"
	"gorm.io/gorm"

	"MyApi/internal/config"
	"MyApi/internal/store"
)

type Service struct {
	cfg     *config.Config
	log     *slog.Logger
	DB      *gorm.DB
	Redis   *redis.Client
	Breaker *gobreaker.CircuitBreaker
	Limiter *redis_rate.Limiter
}

func New(st *store.Store, log *slog.Logger, cfg *config.Config) *Service {
	s := &Service{
		cfg:   cfg,
		log:   log,
		DB:    st.DB,
		Redis: st.Redis,
		Breaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "upstream",
			MaxRequests: 1,
			Timeout:     30 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 5
			},
		}),
	}
	if st.Redis != nil {
		s.Limiter = redis_rate.NewLimiter(st.Redis)
	}
	return s
}

func (s *Service) Log() *slog.Logger      { return s.log }
func (s *Service) Config() *config.Config { return s.cfg }
func (s *Service) WithContext(ctx context.Context) *gorm.DB {
	if s.DB == nil {
		return nil
	}
	return s.DB.WithContext(ctx)
}
