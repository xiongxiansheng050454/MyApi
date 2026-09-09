package router

import (
	"log/slog"
	"net/http"

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

	return engine
}
