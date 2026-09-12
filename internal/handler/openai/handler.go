package openai

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"MyApi/internal/auth"
	"MyApi/internal/gateway"
	"MyApi/internal/platform/httpx"
	protocol "MyApi/internal/protocol/openai"
)

type Handler struct {
	auth *auth.Service
	gw   *gateway.Gateway
	log  *slog.Logger
}

func New(a *auth.Service, gw *gateway.Gateway, log *slog.Logger) *Handler {
	return &Handler{auth: a, gw: gw, log: log}
}

func (h *Handler) ChatCompletions(c *gin.Context) {
	raw, ok := httpx.BearerToken(c)
	if !ok {
		protocol.WriteError(c, http.StatusUnauthorized, "invalid_api_key", "authentication_error",
			"You didn't provide an api key. You need to provide your api key in an Authorization header using Bearer auth.")
		return
	}
	ident, err := h.auth.Resolve(c.Request.Context(), raw)
	if err != nil {
		protocol.WriteServiceError(c, err)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		protocol.WriteError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "Cannot read request body.")
		return
	}
	parsed, err := protocol.ParseChatRequest(body)
	if err != nil {
		protocol.WriteError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request_error", "Field 'model' is required.")
		return
	}

	h.log.Debug("chat completion", "user_id", ident.UserID, "api_key_id", ident.ApiKeyID, "model", parsed.Model)

	meta := gateway.RequestMeta{
		RequestID: c.GetString("request_id"),
		ClientIP:  c.ClientIP(),
		StartedAt: time.Now(),
		SessionID: c.GetHeader("X-Session-Id"),
	}

	if parsed.Stream {
		sw := &ginStreamWriter{c: c}
		if err := h.gw.ChatStream(c.Request.Context(), ident, parsed, body, meta, sw); err != nil && !sw.started {
			protocol.WriteServiceError(c, err)
		}
		return
	}

	resp, err := h.gw.Chat(c.Request.Context(), ident, parsed, body, meta)
	if err != nil {
		protocol.WriteServiceError(c, err)
		return
	}
	if resp == nil {
		protocol.WriteError(c, http.StatusInternalServerError, "internal_error", "server_error", "Internal server error.")
		return
	}
	defer resp.Body.Close()

	ct := resp.ContentType
	if ct == "" {
		ct = "application/json"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", ct)
	_, _ = io.Copy(c.Writer, resp.Body)
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

func (h *Handler) Models(c *gin.Context) {
	raw, ok := httpx.BearerToken(c)
	if !ok {
		protocol.WriteError(c, http.StatusUnauthorized, "invalid_api_key", "authentication_error",
			"You didn't provide an api key. You need to provide your api key in an Authorization header using Bearer auth.")
		return
	}
	ident, err := h.auth.Resolve(c.Request.Context(), raw)
	if err != nil {
		protocol.WriteServiceError(c, err)
		return
	}

	names := h.gw.Models(ident)
	created := time.Now().Unix()
	data := make([]gin.H, 0, len(names))
	for _, name := range names {
		data = append(data, gin.H{
			"id":       name,
			"object":   "model",
			"created":  created,
			"owned_by": "myapi",
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}
