package openai

import (
	"errors"
	"net/http"

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

func writeServiceError(c *gin.Context, err error) {
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
	case errors.Is(err, service.ErrStoreDown):
		writeOpenAIError(c, http.StatusServiceUnavailable, "internal_error", "server_error", "Gateway backend unavailable.")
	case errors.Is(err, service.ErrNotImplemented):
		writeOpenAIError(c, http.StatusNotImplemented, "not_implemented", "server_error", "This endpoint is not implemented yet.")
	default:
		writeOpenAIError(c, http.StatusInternalServerError, "internal_error", "server_error", "Internal server error.")
	}
}
