package user

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"MyApi/internal/auth"
	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
)

var keyPrefixRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,10}$`)

type KeyItem struct {
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

func keyToItem(k *model.ClientApiKey) KeyItem {
	return KeyItem{
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

// lastUsedMap 从 usage_logs 派生每个 Key 的最后使用时间（只读）。
func (s *Service) lastUsedMap(ctx context.Context, keyIDs []int64) map[int64]time.Time {
	out := map[int64]time.Time{}
	if len(keyIDs) == 0 {
		return out
	}
	var rows []struct {
		APIKeyID int64     `gorm:"column:api_key_id"`
		Last     time.Time `gorm:"column:last"`
	}
	_ = s.db.WithContext(ctx).Model(&model.UsageLog{}).
		Select("api_key_id, MAX(created_at) AS last").
		Where("api_key_id IN ?", keyIDs).
		Group("api_key_id").
		Scan(&rows).Error
	for _, r := range rows {
		out[r.APIKeyID] = r.Last
	}
	return out
}

func validJSONObject(raw json.RawMessage) error {
	var m any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if _, ok := m.(map[string]any); !ok {
		return errors.New("必须是 JSON 对象")
	}
	return nil
}

type CreateKeyInput struct {
	KeyName            string
	Prefix             string
	Permissions        json.RawMessage
	RateLimitOverrides json.RawMessage
	ExpiresAt          *string
	IsActive           *bool
}

func (s *Service) CreateKey(ctx context.Context, userID int64, in CreateKeyInput) (*KeyItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	name := in.KeyName
	if name == "" {
		name = "default"
	}
	prefix := in.Prefix
	if prefix == "" {
		prefix = model.KeyPrefixDefault
	}
	if !keyPrefixRe.MatchString(prefix) {
		return nil, apperr.Invalid("prefix 需为 1~10 位 [A-Za-z0-9_-]")
	}
	perms := in.Permissions
	if len(perms) == 0 {
		perms = json.RawMessage(`{"models":["*"]}`)
	} else if err := validJSONObject(perms); err != nil {
		return nil, apperr.Invalid("permissions " + err.Error())
	}
	overrides := in.RateLimitOverrides
	if len(overrides) == 0 {
		overrides = json.RawMessage(`{}`)
	} else if err := validJSONObject(overrides); err != nil {
		return nil, apperr.Invalid("rate_limit_overrides " + err.Error())
	}
	expires, err := parseTimePtr(deref(in.ExpiresAt))
	if err != nil {
		return nil, apperr.Invalid("expires_at 需为 RFC3339")
	}
	isActive := true
	if in.IsActive != nil {
		isActive = *in.IsActive
	}

	db := s.db.WithContext(ctx)
	u, found := s.userByID(ctx, userID)
	if !found {
		return nil, ErrUserNotFound
	}
	if u.Status != "active" {
		return nil, ErrUserSuspended
	}
	var dup int64
	_ = db.Model(&model.ClientApiKey{}).Where("user_id = ? AND key_name = ?", userID, name).Count(&dup).Error
	if dup > 0 {
		return nil, ErrKeyNameDuplicate
	}

	secretPart, err := randomSecret(24)
	if err != nil {
		return nil, err
	}
	full := prefix + secretPart

	k := model.ClientApiKey{
		UserID:             userID,
		KeyName:            name,
		KeyPrefix:          prefix,
		KeyHash:            auth.Hash(full),
		Permissions:        datatypes.JSON(perms),
		RateLimitOverrides: datatypes.JSON(overrides),
		ExpiresAt:          expires,
		IsActive:           isActive,
	}
	if err := db.Create(&k).Error; err != nil {
		return nil, err
	}
	out := keyToItem(&k)
	out.FullKey = full
	return &out, nil
}

type KeyFilter struct {
	UserID   int64 // 0 表示不按用户过滤
	IsActive string
	KeyName  string
	Page     int
	PageSize int
}

func (s *Service) ListKeys(ctx context.Context, f KeyFilter) ([]KeyItem, int64, error) {
	if s.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	q := db.Model(&model.ClientApiKey{})
	if f.UserID > 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.IsActive == "true" || f.IsActive == "false" {
		q = q.Where("is_active = ?", f.IsActive == "true")
	}
	if kw := strings.TrimSpace(f.KeyName); kw != "" {
		q = q.Where("key_name LIKE ?", "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.ClientApiKey
	if err := q.Order("id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	lastUsed := s.lastUsedMap(ctx, ids)
	list := make([]KeyItem, 0, len(rows))
	for i := range rows {
		o := keyToItem(&rows[i])
		if t, ok := lastUsed[rows[i].ID]; ok {
			o.LastUsedAt = &t
		}
		list = append(list, o)
	}
	return list, total, nil
}

type UpdateKeyInput struct {
	KeyName            *string
	Permissions        json.RawMessage
	RateLimitOverrides json.RawMessage
	ExpiresAt          *string
	IsActive           *bool
}

func (s *Service) UpdateKey(ctx context.Context, userID, keyID int64, in UpdateKeyInput) (*KeyItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	var k model.ClientApiKey
	if err := db.Where("id = ? AND user_id = ?", keyID, userID).First(&k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}
	if in.IsActive != nil && *in.IsActive {
		u, found := s.userByID(ctx, userID)
		if !found {
			return nil, ErrUserNotFound
		}
		if u.Status != "active" {
			return nil, ErrUserSuspended
		}
	}

	updates := map[string]any{}
	if in.KeyName != nil {
		name := strings.TrimSpace(*in.KeyName)
		if name == "" {
			return nil, apperr.Invalid("key_name 不能为空")
		}
		var dup int64
		_ = db.Model(&model.ClientApiKey{}).
			Where("user_id = ? AND key_name = ? AND id <> ?", userID, name, keyID).Count(&dup).Error
		if dup > 0 {
			return nil, ErrKeyNameDuplicate
		}
		updates["key_name"] = name
	}
	if len(in.Permissions) > 0 {
		if err := validJSONObject(in.Permissions); err != nil {
			return nil, apperr.Invalid("permissions " + err.Error())
		}
		updates["permissions"] = datatypes.JSON(in.Permissions)
	}
	if len(in.RateLimitOverrides) > 0 {
		if err := validJSONObject(in.RateLimitOverrides); err != nil {
			return nil, apperr.Invalid("rate_limit_overrides " + err.Error())
		}
		updates["rate_limit_overrides"] = datatypes.JSON(in.RateLimitOverrides)
	}
	if in.ExpiresAt != nil {
		t, err := parseTimePtr(*in.ExpiresAt)
		if err != nil {
			return nil, apperr.Invalid("expires_at 需为 RFC3339")
		}
		if t == nil {
			updates["expires_at"] = nil
		} else {
			updates["expires_at"] = t
		}
	}
	if in.IsActive != nil {
		updates["is_active"] = *in.IsActive
	}
	if len(updates) == 0 {
		return nil, apperr.Invalid("没有可更新字段")
	}
	if err := db.Model(&model.ClientApiKey{}).Where("id = ?", keyID).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = db.First(&k, keyID).Error
	out := keyToItem(&k)
	return &out, nil
}

func (s *Service) ResetKey(ctx context.Context, userID, keyID int64) (*KeyItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	var k model.ClientApiKey
	if err := db.Where("id = ? AND user_id = ?", keyID, userID).First(&k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}
	u, found := s.userByID(ctx, userID)
	if !found {
		return nil, ErrUserNotFound
	}
	if u.Status != "active" {
		return nil, ErrUserSuspended
	}
	secretPart, err := randomSecret(24)
	if err != nil {
		return nil, err
	}
	full := k.KeyPrefix + secretPart
	if err := db.Model(&model.ClientApiKey{}).Where("id = ?", keyID).
		Update("key_hash", auth.Hash(full)).Error; err != nil {
		return nil, err
	}
	_ = db.First(&k, keyID).Error
	out := keyToItem(&k)
	out.FullKey = full
	return &out, nil
}

func (s *Service) DeleteKey(ctx context.Context, userID, keyID int64) error {
	if s.db == nil {
		return gorm.ErrInvalidDB
	}
	res := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", keyID, userID).Delete(&model.ClientApiKey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrKeyNotFound
	}
	return nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
