package model

import (
	"time"

	"gorm.io/datatypes"
)

type RateLimitRule struct {
	ID            int64          `gorm:"primaryKey" json:"id"`
	RuleName      string         `gorm:"not null;size:100" json:"rule_name"`
	TargetType    string         `gorm:"not null;size:20" json:"target_type"`
	TargetValue   string         `gorm:"not null;default:*;type:text" json:"target_value"`
	Metric        string         `gorm:"not null;size:20" json:"metric"`
	LimitValue    int            `gorm:"not null" json:"limit_value"`
	WindowSeconds int            `gorm:"not null" json:"window_seconds"`
	Action        string         `gorm:"default:reject;size:20" json:"action"`
	Priority      int            `gorm:"default:0" json:"priority"`
	Enabled       bool           `gorm:"default:true" json:"enabled"`
	Description   *string        `json:"description"`
	Extras        datatypes.JSON `gorm:"type:jsonb" json:"extras"`
	CreatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (RateLimitRule) TableName() string { return "rate_limit_rules" }

const (
	TargetGlobal  = "global"
	TargetUser    = "user"
	TargetAPIKey  = "api_key"
	TargetModel   = "model"
	TargetChannel = "channel"

	MetricRPM         = "rpm"
	MetricTPM         = "tpm"
	MetricRPD         = "rpd"
	MetricTPD         = "tpd"
	MetricConcurrency = "concurrency"

	ActionReject = "reject"
	ActionQueue  = "queue"
)
