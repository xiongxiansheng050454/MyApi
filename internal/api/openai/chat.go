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

	resp, err := h.svc.ChatCompletion(c.Request.Context(), &service.ChatCompletionRequest{
		Key: ident, Model: model, Body: body,
	})
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

	if strings.Contains(ct, "text/event-stream") {
		streamCopy(c.Writer, resp.Body())
		return
	}
	_, _ = io.Copy(c.Writer, resp.Body())
}

func streamCopy(w gin.ResponseWriter, src io.Reader) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, src)
		return
	}
	buf := make([]byte, 4*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, werr := w.Write(buf[:n])
			flusher.Flush()
			if werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

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
