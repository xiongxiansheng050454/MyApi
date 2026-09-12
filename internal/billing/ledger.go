package billing

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
	"MyApi/internal/platform/money"
)

type ledger interface {
	Charge(ctx context.Context, uid int64, requestID string, amountMicro int64) error
}

type gormLedger struct {
	db *gorm.DB
}

func (l *gormLedger) Charge(ctx context.Context, uid int64, requestID string, amountMicro int64) error {
	if amountMicro <= 0 {
		return nil
	}
	amount := money.MicroToAmount(amountMicro)
	return l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var bal model.UserBalance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", uid).First(&bal).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		before := bal.AvailableBalance
		bal.AvailableBalance = money.Round8(bal.AvailableBalance - amount)
		if bal.AvailableBalance < 0 {
			bal.AvailableBalance = 0
		}
		bal.Version++
		if err := tx.Save(&bal).Error; err != nil {
			return err
		}
		return tx.Create(&model.BalanceTransaction{
			UserID:         uid,
			Amount:         -amount,
			BalanceBefore:  before,
			BalanceAfter:   bal.AvailableBalance,
			TxType:         "consume",
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
