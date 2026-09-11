package router

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"MyApi/internal/api/middleware"
	"MyApi/internal/config"
	"MyApi/internal/service"
)

func New(cfg *config.Config, svc *service.Service, log *slog.Logger) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery(), middleware.RequestID())

	engine.GET("/healthz", func(c *gin.Context) {
		status := "ok"
		if svc.DB == nil {
			status = "degraded:no_db"
		}
		if svc.Redis == nil {
			status = "degraded:no_redis"
		}
		c.JSON(http.StatusOK, gin.H{"status": status})
	})

	// 静态前端控制台（dashboard/）与接口文档（docs/）
	mountStatic(engine, log, "/dashboard", cfg.Server.DashboardDir)
	mountStatic(engine, log, "/docs", cfg.Server.DocsDir)
	if cfg.Server.DashboardDir != "" {
		if _, err := os.Stat(cfg.Server.DashboardDir); err == nil {
			engine.GET("/", func(c *gin.Context) {
				c.Redirect(http.StatusFound, "/dashboard/")
			})
		}
	}

	return engine
}

func mountStatic(engine *gin.Engine, log *slog.Logger, urlPath, dir string) {
	if dir == "" {
		return
	}
	if _, err := os.Stat(dir); err != nil {
		log.Warn("static dir not found, skip mount", "url", urlPath, "dir", dir)
		return
	}
	engine.Static(urlPath, dir)
	log.Info("static mounted", "url", urlPath, "dir", dir)
}
