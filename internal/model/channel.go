package model

import (
	"time"

	"gorm.io/datatypes"
)

type Channel struct {
	ID               int64          `gorm:"primaryKey" json:"id"`
	Name             string         `gorm:"not null;size:100" json:"name"`
	BaseURL          string         `gorm:"not null;size:255" json:"base_url"`
	APIKey           string         `gorm:"not null;size:500" json:"-"`
	AuthType         string         `gorm:"default:bearer;size:20" json:"auth_type"`
	ExtraConfig      datatypes.JSON `gorm:"type:jsonb" json:"extra_config"`
	Status           int16          `gorm:"not null;default:1" json:"status"`
	Weight           int            `gorm:"default:100" json:"weight"`
	Priority         int            `gorm:"default:0" json:"priority"`
	Balance          *float64       `gorm:"column:balance;type:numeric(14,8)" json:"balance"`
	BalanceUpdatedAt *time.Time     `gorm:"column:balance_updated_at" json:"balance_updated_at"`
	CreatedAt        time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt        time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Channel) TableName() string { return "channels" }

type ChannelModel struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	ChannelID     int64     `gorm:"not null;uniqueIndex:uk_channel_model,priority:2" json:"channel_id"`
	ModelName     string    `gorm:"not null;size:100;uniqueIndex:uk_channel_model,priority:1;index:idx_model_name" json:"model_name"`
	UpstreamModel string    `gorm:"not null;size:100" json:"upstream_model"`
	Enabled       bool      `gorm:"default:true" json:"enabled"`
	CreatedAt     time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ChannelModel) TableName() string { return "channel_models" }
