package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"MyApi/internal/auth"
	"MyApi/internal/billing"
	"MyApi/internal/catalog"
	"MyApi/internal/channel"
	"MyApi/internal/channelmanager"
	"MyApi/internal/config"
	"MyApi/internal/gateway"
	adminhandler "MyApi/internal/handler/admin"
	openaihandler "MyApi/internal/handler/openai"
	"MyApi/internal/logger"
	"MyApi/internal/platform/tokenizer"
	"MyApi/internal/pricing"
	"MyApi/internal/ratelimit"
	"MyApi/internal/router"
	"MyApi/internal/routing"
	"MyApi/internal/secret"
	"MyApi/internal/store"
	"MyApi/internal/upstream"
	"MyApi/internal/usage"
	"MyApi/internal/user"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path of config yaml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		return
	}

	log, err := logger.New(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		slog.Error("init logger", "err", err)
		return
	}

	st, err := store.Open(cfg, log)
	if err != nil {
		log.Error("open store", "err", err)
		return
	}
	defer st.Close()

	if cfg.Database.AutoMigrate && st.DB != nil {
		if err := store.AutoMigrate(st.DB, log); err != nil {
			log.Error("auto migrate", "err", err)
			return
		}
	}

	sec := loadSecret(cfg, log)
	chMgr := newChannelManager(cfg, st, log)
	chMgr.Start()
	defer chMgr.Shutdown()

	client := newHTTPClient(cfg)

	authSvc := auth.New(st.DB)
	pricingSvc := pricing.New(st.DB)
	limits := ratelimit.New(st.DB, st.Redis, cfg, log)
	rt := routing.New(chMgr, st.Redis, cfg, log)
	upstreamSvc := upstream.New(st.DB, chMgr, sec, client, cfg, log)
	billingSvc := billing.New(st.Redis, st.DB, cfg, log, pricingSvc)
	usageSvc := usage.New(st.DB, st.Redis, cfg, log)
	usageSvc.Start()
	defer usageSvc.Stop()
	catalogSvc := catalog.New(st.DB, chMgr)
	channelSvc := channel.New(st.DB, sec, chMgr, cfg, log)
	userSvc := user.New(st.DB, billingSvc)
	tok := tokenizer.New(cfg.Billing.DefaultEncoding)

	gw := gateway.New(limits, rt, upstreamSvc, pricingSvc, billingSvc, usageSvc, catalogSvc, tok, cfg, log)

	engine := router.New(cfg, st, log)
	router.Register(engine, router.Deps{
		OpenAI: openaihandler.New(authSvc, gw, log),
		Admin:  adminhandler.New(channelSvc, userSvc, limits, pricingSvc, usageSvc, catalogSvc, log),
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("server listening", "addr", addr, "mode", cfg.Server.Mode)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
}

func loadSecret(cfg *config.Config, log *slog.Logger) *secret.Secret {
	sec, generated, err := secret.FromEnvOrFile(cfg.Security.APIKeyEncKeyFile)
	if err != nil {
		log.Warn("api key encryption key unavailable", "err", err,
			"hint", "set "+secret.EnvKey+" or ensure "+cfg.Security.APIKeyEncKeyFile+" is writable")
		return nil
	}
	if generated {
		log.Warn("generated a new api key encryption key; keep it safe and backed up",
			"file", cfg.Security.APIKeyEncKeyFile)
	}
	return sec
}

func newChannelManager(cfg *config.Config, st *store.Store, log *slog.Logger) *channelmanager.Manager {
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
		FilterExhaustedChannels:    cfg.Routing.FilterExhaustedChannels,
		LowBalanceThreshold:        cfg.Routing.LowBalanceThreshold,
	}
	var src channelmanager.Source
	if st.DB != nil {
		src = channelmanager.NewDBSource(st.DB)
	}
	var bus channelmanager.Bus
	if st.Redis != nil {
		bus = channelmanager.NewRedisBus(st.Redis, uc.EventsChannel)
	}
	return channelmanager.New(settings, src, bus, log)
}

func newHTTPClient(cfg *config.Config) *http.Client {
	dialTimeout := time.Duration(cfg.Upstream.DialTimeoutSeconds) * time.Second
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	tlsTimeout := time.Duration(cfg.Upstream.TLSHandshakeTimeoutSeconds) * time.Second
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
	return &http.Client{Transport: transport}
}
