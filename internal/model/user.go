package model

import "time"

type User struct {
	ID           int64      `gorm:"primaryKey" json:"id"`
	PasswordHash string     `gorm:"not null" json:"-"`
	UserGroup    string     `gorm:"not null;default:default" json:"user_group"`
	Status       string     `gorm:"not null;default:active" json:"status"`
	Nickname     *string    `json:"nickname"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (User) TableName() string { return "users" }

type UserBalance struct {
	UserID           int64     `gorm:"primaryKey" json:"user_id"`
	AvailableBalance float64   `gorm:"not null;default:0" json:"available_balance"`
	FrozenBalance    float64   `gorm:"not null;default:0" json:"frozen_balance"`
	Version          int64     `gorm:"not null;default:1" json:"version"`
	UpdatedAt        time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	CreatedAt        time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (UserBalance) TableName() string { return "user_balances" }

type BalanceTransaction struct {
	ID             int64     `gorm:"primaryKey" json:"id"`
	UserID         int64     `gorm:"not null;index:idx_tx_user_created,sort:desc" json:"user_id"`
	Amount         float64   `gorm:"not null" json:"amount"`
	BalanceBefore  float64   `gorm:"not null" json:"balance_before"`
	BalanceAfter   float64   `gorm:"not null" json:"balance_after"`
	TxType         string    `gorm:"not null;size:20" json:"tx_type"`
	RelatedRequest *string   `json:"related_request_id"`
	RelatedOrderID *string   `json:"related_order_id"`
	Description    *string   `json:"description"`
	CreatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (BalanceTransaction) TableName() string { return "balance_transactions" }
