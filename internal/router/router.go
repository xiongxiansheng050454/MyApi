package router

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"MyApi/internal/config"
	"MyApi/internal/middleware"
	"MyApi/internal/store"
)

func New(cfg *config.Config, st *store.Store, log *slog.Logger) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery(), middleware.RequestID())

	engine.GET("/healthz", func(c *gin.Context) {
		status := "ok"
		if st == nil || st.DB == nil {
			status = "degraded:no_db"
		}
		if st == nil || st.Redis == nil {
			status = "degraded:no_redis"
		}
		c.JSON(http.StatusOK, gin.H{"status": status})
	})

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
	handler := http.StripPrefix(urlPath, http.FileServer(http.Dir(dir)))
	serve := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, must-revalidate")
		handler.ServeHTTP(c.Writer, c.Request)
	}
	engine.GET(urlPath, func(c *gin.Context) { c.Redirect(http.StatusFound, urlPath+"/") })
	engine.GET(urlPath+"/*filepath", serve)
	engine.HEAD(urlPath+"/*filepath", serve)
	log.Info("static mounted", "url", urlPath, "dir", dir)
}
