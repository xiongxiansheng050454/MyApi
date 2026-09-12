package admin

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"MyApi/internal/platform/httpx"
	"MyApi/internal/user"
)

func (h *Handler) CreateKey(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	var req struct {
		KeyName            string          `json:"key_name"`
		Prefix             string          `json:"prefix"`
		Permissions        json.RawMessage `json:"permissions"`
		RateLimitOverrides json.RawMessage `json:"rate_limit_overrides"`
		ExpiresAt          *string         `json:"expires_at"`
		IsActive           *bool           `json:"is_active"`
	}
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.users.CreateKey(c.Request.Context(), userID, user.CreateKeyInput{
		KeyName: req.KeyName, Prefix: req.Prefix, Permissions: req.Permissions,
		RateLimitOverrides: req.RateLimitOverrides, ExpiresAt: req.ExpiresAt, IsActive: req.IsActive,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListUserKeys(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.users.ListKeys(c.Request.Context(), user.KeyFilter{
		UserID: userID, IsActive: c.Query("is_active"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) ListKeys(c *gin.Context) {
	var uid int64
	if v := c.Query("user_id"); v != "" {
		if parsed, err := parseID(v); err == nil {
			uid = parsed
		}
	}
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.users.ListKeys(c.Request.Context(), user.KeyFilter{
		UserID: uid, IsActive: c.Query("is_active"), KeyName: c.Query("key_name"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) UpdateKey(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	keyID, ok := idParam(c, "keyId")
	if !ok {
		return
	}
	var req struct {
		KeyName            *string         `json:"key_name"`
		Permissions        json.RawMessage `json:"permissions"`
		RateLimitOverrides json.RawMessage `json:"rate_limit_overrides"`
		ExpiresAt          *string         `json:"expires_at"`
		IsActive           *bool           `json:"is_active"`
	}
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.users.UpdateKey(c.Request.Context(), userID, keyID, user.UpdateKeyInput{
		KeyName: req.KeyName, Permissions: req.Permissions,
		RateLimitOverrides: req.RateLimitOverrides, ExpiresAt: req.ExpiresAt, IsActive: req.IsActive,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ResetKey(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	keyID, ok := idParam(c, "keyId")
	if !ok {
		return
	}
	out, err := h.users.ResetKey(c.Request.Context(), userID, keyID)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeleteKey(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	keyID, ok := idParam(c, "keyId")
	if !ok {
		return
	}
	if err := h.users.DeleteKey(c.Request.Context(), userID, keyID); err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": keyID})
}
