package model

import (
	"time"

	"gorm.io/datatypes"
)

type UsageLog struct {
	ID                   int64          `gorm:"primaryKey" json:"id"`
	RequestID            string         `gorm:"not null;uniqueIndex;size:64" json:"request_id"`
	UserID               int64          `gorm:"not null" json:"user_id"`
	ApiKeyID             int64          `gorm:"not null" json:"api_key_id"`
	ChannelID            int64          `gorm:"not null" json:"channel_id"`
	Model                string         `gorm:"not null;size:100" json:"model"`
	UpstreamModel        *string        `gorm:"size:100" json:"upstream_model"`
	InputTokens          int            `gorm:"not null;default:0" json:"input_tokens"`
	OutputTokens         int            `gorm:"not null;default:0" json:"output_tokens"`
	CachedInputTokens    int            `gorm:"default:0" json:"cached_input_tokens"`
	TotalTokens          int            `gorm:"->;not null;default:0" json:"total_tokens"`
	UnitPriceInputPer1M  float64        `gorm:"column:unit_price_input_per_1m;not null;type:numeric(12,8)" json:"unit_price_input_per_1m"`
	UnitPriceOutputPer1M float64        `gorm:"column:unit_price_output_per_1m;not null;type:numeric(12,8)" json:"unit_price_output_per_1m"`
	TotalCost            float64        `gorm:"not null;type:numeric(12,8)" json:"total_cost"`
	DurationMs           int            `gorm:"not null" json:"duration_ms"`
	TtftMs               *int           `json:"ttft_ms"`
	Status               string         `gorm:"not null;default:success;size:20" json:"status"`
	ErrorCode            *string        `gorm:"size:50" json:"error_code"`
	ClientIP             *string        `gorm:"type:inet" json:"client_ip"`
	Extra                datatypes.JSON `gorm:"type:jsonb" json:"extra"`
	CreatedAt            time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (UsageLog) TableName() string { return "usage_logs" }

const (
	StatusSuccess = "success"
	StatusError   = "error"
)

type UserDailyStat struct {
	UserID                 int64     `gorm:"primaryKey" json:"user_id"`
	StatDate               time.Time `gorm:"primaryKey;type:date" json:"stat_date"`
	TotalInputTokens       int64     `gorm:"default:0" json:"total_input_tokens"`
	TotalOutputTokens      int64     `gorm:"default:0" json:"total_output_tokens"`
	TotalCachedInputTokens int64     `gorm:"default:0" json:"total_cached_input_tokens"`
	TotalTokens            int64     `gorm:"default:0" json:"total_tokens"`
	TotalCost              float64   `gorm:"default:0;type:numeric(12,8)" json:"total_cost"`
	RequestCount           int       `gorm:"default:0" json:"request_count"`
	SuccessCount           int       `gorm:"default:0" json:"success_count"`
	ErrorCount             int       `gorm:"default:0" json:"error_count"`
	LastProcessedID        int64     `gorm:"default:0" json:"last_processed_id"`
	UpdatedAt              time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (UserDailyStat) TableName() string { return "user_daily_stats" }
