package openai

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"MyApi/internal/service"
)

type Handler struct {
	svc *service.Service
	log *slog.Logger
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc, log: svc.Log()}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/chat/completions", h.ChatCompletions)
	rg.GET("/models", h.Models)
}

func (h *Handler) ChatCompletions(c *gin.Context) {
	raw, ok := bearerToken(c)
	if !ok {
		writeOpenAIError(c, http.StatusUnauthorized, "invalid_api_key", "authentication_error",
			"You didn't provide an api key. You need to provide your api key in an Authorization header using Bearer auth.")
		return
	}

	ident, err := h.svc.ResolveKey(c.Request.Context(), raw)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "Cannot read request body.")
		return
	}

	model, err := modelName(body)
	if err != nil {
		writeOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "Field 'model' is required.")
		return
	}

	h.log.Debug("chat completion", "user_id", ident.UserID, "api_key_id", ident.ApiKeyID, "model", model)

	creq := &service.ChatCompletionRequest{
		Key:       ident,
		Model:     model,
		Body:      body,
		RequestID: c.GetString("request_id"),
		ClientIP:  c.ClientIP(),
		StartedAt: time.Now(),
		SessionID: c.GetHeader("X-Session-Id"),
	}

	if streamFlag(body) {
		sw := &ginStreamWriter{c: c}
		if err := h.svc.ChatCompletionStream(c.Request.Context(), creq, sw); err != nil && !sw.started {
			writeServiceError(c, err)
		}
		return
	}

	resp, err := h.svc.ChatCompletion(c.Request.Context(), creq)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	if resp == nil {
		writeOpenAIError(c, http.StatusInternalServerError, "internal_error", "server_error", "Internal server error.")
		return
	}
	defer resp.Close()

	ct := resp.ContentType
	if ct == "" {
		ct = "application/json"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", ct)
	_, _ = io.Copy(c.Writer, resp.Body())
}

type ginStreamWriter struct {
	c       *gin.Context
	started bool
}

func (g *ginStreamWriter) Header(status int, contentType string) {
	if g.started {
		return
	}
	g.c.Status(status)
	if contentType != "" {
		g.c.Header("Content-Type", contentType)
	}
	g.started = true
}

func (g *ginStreamWriter) Write(p []byte) (int, error) { return g.c.Writer.Write(p) }

func (g *ginStreamWriter) Flush() { g.c.Writer.Flush() }

func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}
