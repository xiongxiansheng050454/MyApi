package openai

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"MyApi/internal/service"
)

type Error struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   any    `json:"param"`
	Code    string `json:"code"`
}

type ErrorResponse struct {
	Error Error `json:"error"`
}

func writeOpenAIError(c *gin.Context, status int, code, typ, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Error: Error{Message: message, Type: typ, Code: code},
	})
}

func writeRateLimitError(c *gin.Context, code, typ, message string, retryAfter int64) {
	if retryAfter <= 0 {
		retryAfter = 1
	}
	c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
	writeOpenAIError(c, http.StatusTooManyRequests, code, typ, message)
}

func writeServiceError(c *gin.Context, err error) {
	var rl *service.RateLimitedError
	if errors.As(err, &rl) {
		writeRateLimitError(c, "rate_limit_exceeded", "rate_limit_error",
			"You are sending requests too quickly. Slow down or retry later.", rl.RetryAfter)
		return
	}
	switch {
	case errors.Is(err, service.ErrInvalidKey):
		writeOpenAIError(c, http.StatusUnauthorized, "invalid_api_key", "authentication_error", "The api key is invalid.")
	case errors.Is(err, service.ErrKeyDisabled):
		writeOpenAIError(c, http.StatusUnauthorized, "key_disabled", "authentication_error", "The api key is disabled.")
	case errors.Is(err, service.ErrKeyExpired):
		writeOpenAIError(c, http.StatusUnauthorized, "key_expired", "authentication_error", "The api key is expired.")
	case errors.Is(err, service.ErrUserSuspended):
		writeOpenAIError(c, http.StatusForbidden, "account_suspended", "permission_error", "The account is suspended.")
	case errors.Is(err, service.ErrModelNotFound):
		writeOpenAIError(c, http.StatusNotFound, "model_not_found", "invalid_request_error", "The model does not exist.")
	case errors.Is(err, service.ErrNoHealthyUpstream):
		writeOpenAIError(c, http.StatusBadGateway, "upstream_error", "server_error", "All upstream channels are unavailable.")
	case errors.Is(err, service.ErrQueueTimeout):
		writeRateLimitError(c, "engine_overloaded", "server_error",
			"The request has been waiting too long in the queue.", 0)
	case errors.Is(err, service.ErrInsufficientBalance):
		writeOpenAIError(c, http.StatusTooManyRequests, "insufficient_quota", "insufficient_quota",
			"You exceeded your current quota, please check your balance.")
	case errors.Is(err, service.ErrStoreDown):
		writeOpenAIError(c, http.StatusServiceUnavailable, "internal_error", "server_error", "Gateway backend unavailable.")
	case errors.Is(err, service.ErrNotImplemented):
		writeOpenAIError(c, http.StatusNotImplemented, "not_implemented", "server_error", "This endpoint is not implemented yet.")
	default:
		writeOpenAIError(c, http.StatusInternalServerError, "internal_error", "server_error", "Internal server error.")
	}
}
