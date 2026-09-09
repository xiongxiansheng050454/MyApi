package openai

import (
	"io"
	"log/slog"
	"net/http"
	"strings"

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

	if err := h.svc.ChatCompletion(c.Request.Context(), &service.ChatCompletionRequest{
		Key: ident, Model: model, Body: body,
	}); err != nil {
		writeServiceError(c, err)
		return
	}
	writeServiceError(c, service.ErrNotImplemented)
}

func bearerToken(c *gin.Context) (string, bool) {
	h := c.GetHeader("Authorization")
	if h == "" {
		return "", false
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}
