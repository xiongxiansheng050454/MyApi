package service

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-redis/redis_rate/v9"
	"gorm.io/gorm"

	"MyApi/internal/channelmanager"
	"MyApi/internal/config"
	"MyApi/internal/secret"
	"MyApi/internal/store"
)

type Service struct {
	cfg      *config.Config
	log      *slog.Logger
	DB       *gorm.DB
	Redis    *redis.Client
	Channels *channelmanager.Manager
	Limiter  *redis_rate.Limiter

	secret        *secret.Secret
	forwardClient *http.Client
}

func New(st *store.Store, log *slog.Logger, cfg *config.Config) *Service {
	s := &Service{
		cfg:   cfg,
		log:   log,
		DB:    st.DB,
		Redis: st.Redis,
	}

	sec := func(n int) time.Duration { return time.Duration(n) * time.Second }
	uc := cfg.Upstream
	settings := channelmanager.Settings{
		CacheTTL:                   sec(uc.CacheTTLSeconds),
		RetireGrace:                sec(uc.RetireGraceSeconds),
		EventsChannel:              uc.EventsChannel,
		BreakerMaxRequests:         uc.BreakerMaxRequests,
		BreakerInterval:            sec(uc.BreakerIntervalSeconds),
		BreakerTimeout:             sec(uc.BreakerTimeoutSeconds),
		BreakerConsecutiveFailures: uc.BreakerConsecutiveFailures,
	}

	var src channelmanager.Source
	if st.DB != nil {
		src = channelmanager.NewDBSource(st.DB)
	}
	var bus channelmanager.Bus
	if st.Redis != nil {
		bus = channelmanager.NewRedisBus(st.Redis, uc.EventsChannel)
	}
	s.Channels = channelmanager.New(settings, src, bus, log)

	if st.Redis != nil {
		s.Limiter = redis_rate.NewLimiter(st.Redis)
	}

	if sec, err := secret.FromEnv(); err != nil {
		log.Warn("api key encryption key missing", "err", err,
			"hint", "set "+secret.EnvKey+" to store/read upstream keys")
	} else {
		s.secret = sec
	}

	dialTimeout := sec(uc.DialTimeoutSeconds)
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	tlsTimeout := sec(uc.TLSHandshakeTimeoutSeconds)
	if tlsTimeout <= 0 {
		tlsTimeout = 5 * time.Second
	}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: tlsTimeout,
	}
	s.forwardClient = &http.Client{Transport: transport}

	return s
}

func (s *Service) Secret() *secret.Secret { return s.secret }
func (s *Service) Start() {
	if s.Channels != nil {
		s.Channels.Start()
	}
}

func (s *Service) Shutdown() {
	if s.Channels != nil {
		s.Channels.Shutdown()
	}
}

func (s *Service) Log() *slog.Logger      { return s.log }
func (s *Service) Config() *config.Config { return s.cfg }
func (s *Service) WithContext(ctx context.Context) *gorm.DB {
	if s.DB == nil {
		return nil
	}
	return s.DB.WithContext(ctx)
}
