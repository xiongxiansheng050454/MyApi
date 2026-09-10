package model

import "time"

type ModelPricing struct {
	ID                    int64     `gorm:"primaryKey" json:"id"`
	ChannelID             int64     `gorm:"column:channel_id;not null;uniqueIndex:uk_channel_model_pricing,priority:1" json:"channel_id"`
	ModelName             string    `gorm:"column:model_name;not null;size:100;uniqueIndex:uk_channel_model_pricing,priority:2" json:"model_name"`
	InputPricePer1M       float64   `gorm:"column:input_price_per_1m;not null;default:0;type:numeric(12,8)" json:"input_price_per_1m"`
	OutputPricePer1M      float64   `gorm:"column:output_price_per_1m;not null;default:0;type:numeric(12,8)" json:"output_price_per_1m"`
	CachedInputPricePer1M *float64  `gorm:"column:cached_input_price_per_1m;type:numeric(12,8)" json:"cached_input_price_per_1m"`
	Currency              string    `gorm:"column:currency;not null;default:USD;size:3" json:"currency"`
	CreatedAt             time.Time `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (ModelPricing) TableName() string { return "model_pricing" }
