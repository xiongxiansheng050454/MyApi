package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
)

type Identity struct {
	UserID    int64
	ApiKeyID  int64
	Prefix    string
	AllowAll  bool
	Models    []string
	Overrides map[string]int64
}

func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

// Resolve 按哈希查找网关 Key 并构建身份；只读，不修改任何状态。
func (s *Service) Resolve(ctx context.Context, raw string) (*Identity, error) {
	if s.db == nil {
		return nil, apperr.ErrStoreDown
	}
	db := s.db.WithContext(ctx)

	var key model.ClientApiKey
	if err := db.Where("key_hash = ?", Hash(raw)).First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrInvalidKey
		}
		return nil, err
	}
	if !key.IsActive {
		return nil, apperr.ErrKeyDisabled
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, apperr.ErrKeyExpired
	}

	var user model.User
	if err := db.First(&user, key.UserID).Error; err != nil {
		return nil, apperr.ErrUserSuspended
	}
	if user.Status != "active" {
		return nil, apperr.ErrUserSuspended
	}

	ident := &Identity{
		UserID:   key.UserID,
		ApiKeyID: key.ID,
		Prefix:   key.KeyPrefix,
		AllowAll: true,
	}
	if len(key.Permissions) > 0 {
		var perm struct {
			Models []string `json:"models"`
		}
		if err := json.Unmarshal(key.Permissions, &perm); err == nil && len(perm.Models) > 0 {
			ident.AllowAll = false
			ident.Models = perm.Models
		}
	}
	for _, m := range ident.Models {
		if m == "*" {
			ident.AllowAll = true
			break
		}
	}
	if len(key.RateLimitOverrides) > 0 {
		var ov map[string]int64
		if err := json.Unmarshal(key.RateLimitOverrides, &ov); err == nil && len(ov) > 0 {
			ident.Overrides = ov
		}
	}
	return ident, nil
}
