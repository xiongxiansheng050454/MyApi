package billing

import (
	"context"
	"errors"
	"log/slog"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"MyApi/internal/config"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/platform/money"
	"MyApi/internal/pricing"
)

var (
	errInsufficient     = apperr.ErrInsufficientBalance
	errBalanceStoreDown = errors.New("balance cache unavailable")
)

type Service struct {
	db      *gorm.DB
	balance balanceCache
	ledger  ledger
	pricing *pricing.Service
	cfg     *config.Config
	log     *slog.Logger
}

// Reservation 表示一次上游尝试的 Redis 预扣。
type Reservation struct {
	Active      bool
	Gen         string
	AmountMicro int64
}

func New(rdb *redis.Client, db *gorm.DB, cfg *config.Config, log *slog.Logger, pricingSvc *pricing.Service) *Service {
	s := &Service{db: db, pricing: pricingSvc, cfg: cfg, log: log}
	if rdb != nil {
		s.balance = newRedisBalance(rdb, cfg.Billing.BalanceTTLSeconds, cfg.Billing.LockTTLSeconds, s.loadBalanceMicro)
	}
	if db != nil {
		s.ledger = &gormLedger{db: db}
	}
	return s
}

// EstimateMicro 返回该渠道+模型的预估冻结额（微单位，1e8=1）；priced=false 表示无单价。
func (s *Service) EstimateMicro(ctx context.Context, channelID int64, modelName string, inputTokens, outputBudget int) (int64, bool) {
	in, out, _ := s.pricing.Lookup(ctx, channelID, modelName)
	if in <= 0 && out <= 0 {
		return 0, false
	}
	amount := (float64(inputTokens)*in + float64(outputBudget)*out) / 1e6
	return money.RoundMicro(amount), true
}

// Reserve 执行预扣：有单价走 Redis 预扣；无单价仅校验可用余额 > 0。
func (s *Service) Reserve(ctx context.Context, uid, channelID int64, modelName string, inputTokens, outputBudget int) (Reservation, error) {
	if !s.cfg.Billing.Enabled {
		return Reservation{}, nil
	}
	if s.balance == nil {
		return Reservation{}, apperr.ErrStoreDown
	}
	amountMicro, priced := s.EstimateMicro(ctx, channelID, modelName, inputTokens, outputBudget)
	if !priced {
		if err := s.balance.CheckPositive(ctx, uid); err != nil {
			if errors.Is(err, apperr.ErrInsufficientBalance) {
				return Reservation{}, err
			}
			return Reservation{}, apperr.ErrStoreDown
		}
		return Reservation{}, nil
	}
	gen, err := s.balance.PreDeduct(ctx, uid, amountMicro)
	if err != nil {
		if errors.Is(err, apperr.ErrInsufficientBalance) {
			return Reservation{}, err
		}
		return Reservation{}, apperr.ErrStoreDown
	}
	return Reservation{Active: true, Gen: gen, AmountMicro: amountMicro}, nil
}

func (s *Service) Release(ctx context.Context, uid int64, r Reservation) {
	if !r.Active || s.balance == nil {
		return
	}
	if err := s.balance.Release(ctx, uid, r.Gen, r.AmountMicro); err != nil {
		s.log.Warn("release reservation", "err", err, "user_id", uid)
	}
}

func (s *Service) Settle(ctx context.Context, uid int64, r Reservation, actualMicro int64) {
	if !r.Active || s.balance == nil {
		return
	}
	if err := s.balance.Settle(ctx, uid, r.Gen, r.AmountMicro, actualMicro); err != nil {
		s.log.Warn("settle reservation", "err", err, "user_id", uid)
	}
}

// LedgerCharge 在数据库权威扣减用户余额并写资金流水。
func (s *Service) LedgerCharge(ctx context.Context, uid int64, requestID string, amountMicro int64) {
	if !s.cfg.Billing.Enabled || s.ledger == nil {
		return
	}
	if err := s.ledger.Charge(ctx, uid, requestID, amountMicro); err != nil {
		s.log.Error("charge balance", "err", err, "request_id", requestID, "user_id", uid)
	}
}

func (s *Service) InvalidateBalance(ctx context.Context, uid int64) {
	if s.balance != nil {
		_ = s.balance.Invalidate(ctx, uid)
	}
}

func (s *Service) loadBalanceMicro(ctx context.Context, uid int64) (int64, error) {
	if s.db == nil {
		return 0, apperr.ErrStoreDown
	}
	var bal modelBalanceRow
	if err := s.db.WithContext(ctx).Table("user_balances").Select("available_balance").Where("user_id = ?", uid).Scan(&bal).Error; err != nil {
		return 0, err
	}
	return money.AmountToMicro(bal.AvailableBalance), nil
}

type modelBalanceRow struct {
	AvailableBalance float64 `gorm:"column:available_balance"`
}
