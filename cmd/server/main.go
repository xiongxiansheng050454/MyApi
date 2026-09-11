package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"MyApi/internal/api/admin"
	"MyApi/internal/api/openai"
	"MyApi/internal/config"
	"MyApi/internal/logger"
	"MyApi/internal/router"
	"MyApi/internal/service"
	"MyApi/internal/store"
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

	svc := service.New(st, log, cfg)

	engine := router.New(cfg, svc, log)
	openaiHandler := openai.New(svc)
	openaiHandler.Register(engine.Group("/v1"))
	adminHandler := admin.New(log)
	adminHandler.Register(engine.Group("/admin"))

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
