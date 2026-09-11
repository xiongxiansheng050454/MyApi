package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"MyApi/internal/model"
)

type KeyIdentity struct {
	UserID    int64
	ApiKeyID  int64
	Prefix    string
	AllowAll  bool
	Models    []string
	Overrides map[string]int64
}

func HashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Service) ResolveKey(ctx context.Context, raw string) (*KeyIdentity, error) {
	db := s.WithContext(ctx)
	if db == nil {
		return nil, ErrStoreDown
	}

	var key model.ClientApiKey
	if err := db.Where("key_hash = ?", HashKey(raw)).First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidKey
		}
		return nil, err
	}
	if !key.IsActive {
		return nil, ErrKeyDisabled
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, ErrKeyExpired
	}

	var user model.User
	if err := db.First(&user, key.UserID).Error; err != nil {
		return nil, ErrUserSuspended
	}
	if user.Status != "active" {
		return nil, ErrUserSuspended
	}

	ident := &KeyIdentity{
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
