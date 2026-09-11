package openai

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Models(c *gin.Context) {
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

	names := h.svc.Models(ident)
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
