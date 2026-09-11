package model

import "time"

type ModelPricing struct {
	ID                    int64     `gorm:"primaryKey" json:"id"`
	ChannelID             int64     `gorm:"not null;uniqueIndex:uk_channel_model_pricing,priority:2" json:"channel_id"`
	ModelName             string    `gorm:"not null;size:100;uniqueIndex:uk_channel_model_pricing,priority:1" json:"model_name"`
	InputPricePer1M       float64   `gorm:"not null;default:0;type:numeric(12,8)" json:"input_price_per_1m"`
	OutputPricePer1M      float64   `gorm:"not null;default:0;type:numeric(12,8)" json:"output_price_per_1m"`
	CachedInputPricePer1M *float64  `gorm:"type:numeric(12,8)" json:"cached_input_price_per_1m"`
	Currency              string    `gorm:"not null;default:USD;size:3" json:"currency"`
	CreatedAt             time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt             time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ModelPricing) TableName() string { return "model_pricing" }
