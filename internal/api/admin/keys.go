package admin

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/service"
)

var keyPrefixRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,10}$`)

type keyOut struct {
	ID                 int64          `json:"id"`
	UserID             int64          `json:"user_id"`
	KeyName            string         `json:"key_name"`
	Prefix             string         `json:"prefix"`
	FullKey            string         `json:"full_key,omitempty"`
	Permissions        datatypes.JSON `json:"permissions"`
	RateLimitOverrides datatypes.JSON `json:"rate_limit_overrides"`
	ExpiresAt          *time.Time     `json:"expires_at"`
	IsActive           bool           `json:"is_active"`
	LastUsedAt         *time.Time     `json:"last_used_at"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

func keyToOut(k *model.ClientApiKey) keyOut {
	return keyOut{
		ID:                 k.ID,
		UserID:             k.UserID,
		KeyName:            k.KeyName,
		Prefix:             k.KeyPrefix,
		Permissions:        k.Permissions,
		RateLimitOverrides: k.RateLimitOverrides,
		ExpiresAt:          k.ExpiresAt,
		IsActive:           k.IsActive,
		LastUsedAt:         k.LastUsedAt,
		CreatedAt:          k.CreatedAt,
		UpdatedAt:          k.UpdatedAt,
	}
}

func validJSONObject(raw json.RawMessage) error {
	var m any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if _, ok := m.(map[string]any); !ok {
		return errNotObject
	}
	return nil
}

var errNotObject = &jsonError{msg: "必须是 JSON 对象"}

type jsonError struct{ msg string }

func (e *jsonError) Error() string { return e.msg }

func (h *Handler) createKey(c *gin.Context) {
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
	name := req.KeyName
	if name == "" {
		name = "default"
	}
	prefix := req.Prefix
	if prefix == "" {
		prefix = model.KeyPrefixDefault
	}
	if !keyPrefixRe.MatchString(prefix) {
		Fail(c, CodeKeyPrefixInvalid, "prefix 需为 1~10 位 [A-Za-z0-9_-]")
		return
	}
	perms := req.Permissions
	if len(perms) == 0 {
		perms = json.RawMessage(`{"models":["*"]}`)
	} else if err := validJSONObject(perms); err != nil {
		Fail(c, CodeParamError, "permissions "+err.Error())
		return
	}
	overrides := req.RateLimitOverrides
	if len(overrides) == 0 {
		overrides = json.RawMessage(`{}`)
	} else if err := validJSONObject(overrides); err != nil {
		Fail(c, CodeParamError, "rate_limit_overrides "+err.Error())
		return
	}
	expires, err := parseTimePtr(orEmpty(req.ExpiresAt))
	if err != nil {
		Fail(c, CodeParamError, "expires_at 需为 RFC3339")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	db, ok := h.db(c)
	if !ok {
		return
	}
	u, found := h.userByID(db, userID)
	if !found {
		Fail(c, CodeUserNotFound, "用户不存在")
		return
	}
	if u.Status != "active" {
		Fail(c, CodeUserSuspended, "用户状态非 active，禁止创建 Key")
		return
	}
	var dup int64
	_ = db.Model(&model.ClientApiKey{}).Where("user_id = ? AND key_name = ?", userID, name).Count(&dup).Error
	if dup > 0 {
		Fail(c, CodeKeyNameDuplicate, "同一用户下 key_name 已存在")
		return
	}

	secretPart, err := randomSecret(24)
	if err != nil {
		Fail(c, CodeInternal, "生成密钥失败")
		return
	}
	full := prefix + secretPart

	k := model.ClientApiKey{
		UserID:             userID,
		KeyName:            name,
		KeyPrefix:          prefix,
		KeyHash:            service.HashKey(full),
		Permissions:        datatypes.JSON(perms),
		RateLimitOverrides: datatypes.JSON(overrides),
		ExpiresAt:          expires,
		IsActive:           isActive,
	}
	if err := db.Create(&k).Error; err != nil {
		h.log.Error("create key", "err", err)
		Fail(c, CodeInternal, "创建 Key 失败")
		return
	}
	out := keyToOut(&k)
	out.FullKey = full
	OK(c, out)
}

func (h *Handler) listKeys(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.ClientApiKey{})
	if uid := c.Query("user_id"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if ia := c.Query("is_active"); ia == "true" || ia == "false" {
		q = q.Where("is_active = ?", ia == "true")
	}
	if kw := strings.TrimSpace(c.Query("key_name")); kw != "" {
		q = q.Where("key_name LIKE ?", "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		h.log.Error("count keys", "err", err)
		Fail(c, CodeInternal, "查询 Key 失败")
		return
	}
	var rows []model.ClientApiKey
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list keys", "err", err)
		Fail(c, CodeInternal, "查询 Key 失败")
		return
	}
	list := make([]keyOut, 0, len(rows))
	for i := range rows {
		list = append(list, keyToOut(&rows[i]))
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) listUserKeys(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	if _, found := h.userByID(db, userID); !found {
		Fail(c, CodeUserNotFound, "用户不存在")
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.ClientApiKey{}).Where("user_id = ?", userID)
	if ia := c.Query("is_active"); ia == "true" || ia == "false" {
		q = q.Where("is_active = ?", ia == "true")
	}
	var total int64
	_ = q.Count(&total).Error
	var rows []model.ClientApiKey
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list user keys", "err", err)
		Fail(c, CodeInternal, "查询 Key 失败")
		return
	}
	list := make([]keyOut, 0, len(rows))
	for i := range rows {
		list = append(list, keyToOut(&rows[i]))
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) updateKey(c *gin.Context) {
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
	db, ok := h.db(c)
	if !ok {
		return
	}
	var k model.ClientApiKey
	if err := db.Where("id = ? AND user_id = ?", keyID, userID).First(&k).Error; err != nil {
		if errorsIsNotFound(err) {
			Fail(c, CodeKeyNotFound, "Key 不存在")
			return
		}
		h.log.Error("get key", "err", err)
		Fail(c, CodeInternal, "查询 Key 失败")
		return
	}
	if req.IsActive != nil && *req.IsActive && k.UserID == userID {
		u, found := h.userByID(db, userID)
		if !found {
			Fail(c, CodeUserNotFound, "用户不存在")
			return
		}
		if u.Status != "active" {
			Fail(c, CodeUserSuspended, "用户状态非 active，禁止启用 Key")
			return
		}
	}

	updates := map[string]any{}
	if req.KeyName != nil {
		name := strings.TrimSpace(*req.KeyName)
		if name == "" {
			Fail(c, CodeParamError, "key_name 不能为空")
			return
		}
		var dup int64
		_ = db.Model(&model.ClientApiKey{}).
			Where("user_id = ? AND key_name = ? AND id <> ?", userID, name, keyID).Count(&dup).Error
		if dup > 0 {
			Fail(c, CodeKeyNameDuplicate, "同一用户下 key_name 已存在")
			return
		}
		updates["key_name"] = name
	}
	if len(req.Permissions) > 0 {
		if err := validJSONObject(req.Permissions); err != nil {
			Fail(c, CodeParamError, "permissions "+err.Error())
			return
		}
		updates["permissions"] = datatypes.JSON(req.Permissions)
	}
	if len(req.RateLimitOverrides) > 0 {
		if err := validJSONObject(req.RateLimitOverrides); err != nil {
			Fail(c, CodeParamError, "rate_limit_overrides "+err.Error())
			return
		}
		updates["rate_limit_overrides"] = datatypes.JSON(req.RateLimitOverrides)
	}
	if req.ExpiresAt != nil {
		t, err := parseTimePtr(*req.ExpiresAt)
		if err != nil {
			Fail(c, CodeParamError, "expires_at 需为 RFC3339")
			return
		}
		if t == nil {
			updates["expires_at"] = nil
		} else {
			updates["expires_at"] = t
		}
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if len(updates) == 0 {
		Fail(c, CodeParamError, "没有可更新字段")
		return
	}
	if err := db.Model(&model.ClientApiKey{}).Where("id = ?", keyID).Updates(updates).Error; err != nil {
		h.log.Error("update key", "err", err)
		Fail(c, CodeInternal, "更新 Key 失败")
		return
	}
	_ = db.First(&k, keyID).Error
	OK(c, keyToOut(&k))
}

func (h *Handler) resetKey(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	keyID, ok := idParam(c, "keyId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	var k model.ClientApiKey
	if err := db.Where("id = ? AND user_id = ?", keyID, userID).First(&k).Error; err != nil {
		if errorsIsNotFound(err) {
			Fail(c, CodeKeyNotFound, "Key 不存在")
			return
		}
		h.log.Error("get key for reset", "err", err)
		Fail(c, CodeInternal, "查询 Key 失败")
		return
	}
	u, found := h.userByID(db, userID)
	if !found {
		Fail(c, CodeUserNotFound, "用户不存在")
		return
	}
	if u.Status != "active" {
		Fail(c, CodeUserSuspended, "用户状态非 active，禁止重置 Key")
		return
	}
	secretPart, err := randomSecret(24)
	if err != nil {
		Fail(c, CodeInternal, "生成密钥失败")
		return
	}
	full := k.KeyPrefix + secretPart
	if err := db.Model(&model.ClientApiKey{}).Where("id = ?", keyID).
		Update("key_hash", service.HashKey(full)).Error; err != nil {
		h.log.Error("reset key", "err", err)
		Fail(c, CodeInternal, "重置 Key 失败")
		return
	}
	_ = db.First(&k, keyID).Error
	out := keyToOut(&k)
	out.FullKey = full
	OK(c, out)
}

func (h *Handler) deleteKey(c *gin.Context) {
	userID, ok := idParam(c, "userId")
	if !ok {
		return
	}
	keyID, ok := idParam(c, "keyId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	res := db.Where("id = ? AND user_id = ?", keyID, userID).Delete(&model.ClientApiKey{})
	if res.Error != nil {
		h.log.Error("delete key", "err", res.Error)
		Fail(c, CodeInternal, "删除 Key 失败")
		return
	}
	if res.RowsAffected == 0 {
		Fail(c, CodeKeyNotFound, "Key 不存在")
		return
	}
	OK(c, gin.H{"id": keyID})
}

func orEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func errorsIsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
