package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
)

type balanceStore interface {
	Check(ctx context.Context, uid int64) error
	Freeze(ctx context.Context, uid int64, amount float64, requestID string) error
	Settle(ctx context.Context, uid int64, requestID string, frozen, actual float64) error
	Unfreeze(ctx context.Context, uid int64, requestID string, amount float64) error
}

type gormBalance struct {
	db *gorm.DB
}

func round8(v float64) float64 { return math.Round(v*1e8) / 1e8 }

func (b *gormBalance) lockRow(tx *gorm.DB, uid int64) (*model.UserBalance, error) {
	var bal model.UserBalance
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", uid).First(&bal).Error
	if err != nil {
		return nil, err
	}
	return &bal, nil
}

func (b *gormBalance) Check(ctx context.Context, uid int64) error {
	var bal model.UserBalance
	err := b.db.WithContext(ctx).Where("user_id = ?", uid).First(&bal).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInsufficientBalance
		}
		return err
	}
	if bal.AvailableBalance <= 0 {
		return ErrInsufficientBalance
	}
	return nil
}

func (b *gormBalance) Freeze(ctx context.Context, uid int64, amount float64, requestID string) error {
	amount = round8(amount)
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bal, err := b.lockRow(tx, uid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInsufficientBalance
			}
			return err
		}
		if bal.AvailableBalance+1e-9 < amount {
			return ErrInsufficientBalance
		}
		before := bal.AvailableBalance
		bal.AvailableBalance = round8(bal.AvailableBalance - amount)
		bal.FrozenBalance = round8(bal.FrozenBalance + amount)
		bal.Version++
		if err := tx.Save(bal).Error; err != nil {
			return err
		}
		return tx.Create(&model.BalanceTransaction{
			UserID:         uid,
			Amount:         -amount,
			BalanceBefore:  before,
			BalanceAfter:   bal.AvailableBalance,
			TxType:         "freeze",
			RelatedRequest: strPtr(requestID),
		}).Error
	})
}

func (b *gormBalance) Settle(ctx context.Context, uid int64, requestID string, frozen, actual float64) error {
	frozen = round8(frozen)
	actual = round8(actual)
	if actual < 0 {
		actual = 0
	}
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bal, err := b.lockRow(tx, uid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		charged := actual
		remaining := frozen - actual
		beforeAvail := bal.AvailableBalance
		bal.FrozenBalance = round8(bal.FrozenBalance - frozen)
		if bal.FrozenBalance < 0 {
			bal.FrozenBalance = 0
		}
		if remaining > 0 {
			bal.AvailableBalance = round8(bal.AvailableBalance + remaining)
		} else if remaining < 0 {
			extra := -remaining
			if bal.AvailableBalance < extra {
				extra = bal.AvailableBalance
			}
			bal.AvailableBalance = round8(bal.AvailableBalance - extra)
			charged = round8(frozen + extra)
		}
		bal.Version++
		if err := tx.Save(bal).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.BalanceTransaction{
			UserID:         uid,
			Amount:         -charged,
			BalanceBefore:  beforeAvail,
			BalanceAfter:   bal.AvailableBalance,
			TxType:         "consume",
			RelatedRequest: strPtr(requestID),
		}).Error; err != nil {
			return err
		}
		if remaining > 0 {
			return tx.Create(&model.BalanceTransaction{
				UserID:         uid,
				Amount:         remaining,
				BalanceBefore:  beforeAvail,
				BalanceAfter:   bal.AvailableBalance,
				TxType:         "unfreeze",
				RelatedRequest: strPtr(requestID),
			}).Error
		}
		return nil
	})
}

func (b *gormBalance) Unfreeze(ctx context.Context, uid int64, requestID string, amount float64) error {
	amount = round8(amount)
	if amount <= 0 {
		return nil
	}
	return b.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bal, err := b.lockRow(tx, uid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		bal.FrozenBalance = round8(bal.FrozenBalance - amount)
		if bal.FrozenBalance < 0 {
			bal.FrozenBalance = 0
		}
		beforeAvail := bal.AvailableBalance
		bal.AvailableBalance = round8(bal.AvailableBalance + amount)
		bal.Version++
		if err := tx.Save(bal).Error; err != nil {
			return err
		}
		return tx.Create(&model.BalanceTransaction{
			UserID:         uid,
			Amount:         amount,
			BalanceBefore:  beforeAvail,
			BalanceAfter:   bal.AvailableBalance,
			TxType:         "unfreeze",
			RelatedRequest: strPtr(requestID),
		}).Error
	})
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Service) outputBudget(body []byte) int {
	var req struct {
		MaxTokens           int `json:"max_tokens"`
		MaxCompletionTokens int `json:"max_completion_tokens"`
	}
	_ = json.Unmarshal(body, &req)
	b := req.MaxTokens
	if b <= 0 {
		b = req.MaxCompletionTokens
	}
	if b <= 0 {
		b = s.cfg.Billing.DefaultOutputBudget
	}
	if b <= 0 {
		b = 1024
	}
	return b
}

// estimateFreeze 返回该渠道+模型的预估冻结额；priced=false 表示无单价（仅做余额>0 校验）。
func (s *Service) estimateFreeze(ctx context.Context, channelID int64, req *ChatCompletionRequest) (amount float64, priced bool) {
	in, out, _ := s.lookupPricing(ctx, channelID, req.Model)
	if in <= 0 && out <= 0 {
		return 0, false
	}
	inTokens := float64(s.estimateTokens(req.Body))
	outTokens := float64(s.outputBudget(req.Body))
	amount = round8((inTokens*in + outTokens*out) / 1e6)
	return amount, true
}
