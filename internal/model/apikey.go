package model

import (
	"time"

	"gorm.io/datatypes"
)

type ClientApiKey struct {
	ID                 int64          `gorm:"primaryKey" json:"id"`
	UserID             int64          `gorm:"not null;uniqueIndex:uk_user_keyname,priority:1" json:"user_id"`
	KeyName            string         `gorm:"not null;default:default;uniqueIndex:uk_user_keyname,priority:2" json:"key_name"`
	KeyPrefix          string         `gorm:"not null;size:10" json:"key_prefix"`
	KeyHash            string         `gorm:"not null" json:"-"`
	Permissions        datatypes.JSON `gorm:"type:jsonb" json:"permissions"`
	RateLimitOverrides datatypes.JSON `gorm:"type:jsonb" json:"rate_limit_overrides"`
	ExpiresAt          *time.Time     `json:"expires_at"`
	IsActive           bool           `gorm:"not null;default:true" json:"is_active"`
	LastUsedAt         *time.Time     `json:"last_used_at"`
	CreatedAt          time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ClientApiKey) TableName() string { return "client_api_keys" }

const (
	KeyPrefixDefault = "sk-"
)
