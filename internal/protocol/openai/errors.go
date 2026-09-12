package openai

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"MyApi/internal/platform/apperr"
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

func WriteError(c *gin.Context, status int, code, typ, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Error: Error{Message: message, Type: typ, Code: code},
	})
}

func WriteRateLimitError(c *gin.Context, code, typ, message string, retryAfter int64) {
	if retryAfter <= 0 {
		retryAfter = 1
	}
	c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
	WriteError(c, http.StatusTooManyRequests, code, typ, message)
}

func WriteServiceError(c *gin.Context, err error) {
	var rl *apperr.RateLimitedError
	if errors.As(err, &rl) {
		WriteRateLimitError(c, "rate_limit_exceeded", "rate_limit_error",
			"You are sending requests too quickly. Slow down or retry later.", rl.RetryAfter)
		return
	}
	switch {
	case errors.Is(err, apperr.ErrInvalidKey):
		WriteError(c, http.StatusUnauthorized, "invalid_api_key", "authentication_error", "The api key is invalid.")
	case errors.Is(err, apperr.ErrKeyDisabled):
		WriteError(c, http.StatusUnauthorized, "key_disabled", "authentication_error", "The api key is disabled.")
	case errors.Is(err, apperr.ErrKeyExpired):
		WriteError(c, http.StatusUnauthorized, "key_expired", "authentication_error", "The api key is expired.")
	case errors.Is(err, apperr.ErrUserSuspended):
		WriteError(c, http.StatusForbidden, "account_suspended", "permission_error", "The account is suspended.")
	case errors.Is(err, apperr.ErrModelNotFound):
		WriteError(c, http.StatusNotFound, "model_not_found", "invalid_request_error", "The model does not exist.")
	case errors.Is(err, apperr.ErrNoHealthyUpstream):
		WriteError(c, http.StatusBadGateway, "upstream_error", "server_error", "All upstream channels are unavailable.")
	case errors.Is(err, apperr.ErrQueueTimeout):
		WriteRateLimitError(c, "engine_overloaded", "server_error",
			"The request has been waiting too long in the queue.", 0)
	case errors.Is(err, apperr.ErrInsufficientBalance):
		WriteError(c, http.StatusTooManyRequests, "insufficient_quota", "insufficient_quota",
			"You exceeded your current quota, please check your balance.")
	case errors.Is(err, apperr.ErrStoreDown):
		WriteError(c, http.StatusServiceUnavailable, "internal_error", "server_error", "Gateway backend unavailable.")
	case errors.Is(err, apperr.ErrNotImplemented):
		WriteError(c, http.StatusNotImplemented, "not_implemented", "server_error", "This endpoint is not implemented yet.")
	default:
		WriteError(c, http.StatusInternalServerError, "internal_error", "server_error", "Internal server error.")
	}
}
