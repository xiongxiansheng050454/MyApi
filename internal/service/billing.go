package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

func round8(v float64) float64 { return math.Round(v*1e8) / 1e8 }

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

// estimateFreezeMicro 返回该渠道+模型的预估冻结额（微单位，1e8=1）；priced=false 表示无单价。
func (s *Service) estimateFreezeMicro(ctx context.Context, channelID int64, req *ChatCompletionRequest) (int64, bool) {
	in, out, _ := s.lookupPricing(ctx, channelID, req.Model)
	if in <= 0 && out <= 0 {
		return 0, false
	}
	inTokens := float64(s.estimateTokens(req.Body, req.Model))
	outTokens := float64(s.outputBudget(req.Body))
	amount := (inTokens*in + outTokens*out) / 1e6
	return int64(math.Round(amount * 1e8)), true
}

// reservation 表示一次上游尝试的 Redis 预扣。
type reservation struct {
	active      bool
	gen         string
	amountMicro int64
}

// reserve 执行预扣：有单价走 Redis 预扣；无单价仅校验可用余额 > 0。
func (s *Service) reserve(ctx context.Context, req *ChatCompletionRequest, channelID int64) (reservation, error) {
	if !s.cfg.Billing.Enabled {
		return reservation{}, nil
	}
	if s.balance == nil {
		return reservation{}, ErrStoreDown
	}
	amountMicro, priced := s.estimateFreezeMicro(ctx, channelID, req)
	if !priced {
		if err := s.balance.CheckPositive(ctx, req.Key.UserID); err != nil {
			if errors.Is(err, ErrInsufficientBalance) {
				return reservation{}, err
			}
			return reservation{}, ErrStoreDown
		}
		return reservation{}, nil
	}
	gen, err := s.balance.PreDeduct(ctx, req.Key.UserID, amountMicro)
	if err != nil {
		if errors.Is(err, ErrInsufficientBalance) {
			return reservation{}, err
		}
		return reservation{}, ErrStoreDown
	}
	return reservation{active: true, gen: gen, amountMicro: amountMicro}, nil
}

func (s *Service) releaseReservation(ctx context.Context, uid int64, r reservation) {
	if !r.active || s.balance == nil {
		return
	}
	if err := s.balance.Release(ctx, uid, r.gen, r.amountMicro); err != nil {
		s.log.Warn("release reservation", "err", err, "user_id", uid)
	}
}

func (s *Service) settleReservation(ctx context.Context, uid int64, r reservation, actualMicro int64) {
	if !r.active || s.balance == nil {
		return
	}
	if err := s.balance.Settle(ctx, uid, r.gen, r.amountMicro, actualMicro); err != nil {
		s.log.Warn("settle reservation", "err", err, "user_id", uid)
	}
}

func (s *Service) loadBalanceMicro(ctx context.Context, uid int64) (int64, error) {
	db := s.WithContext(ctx)
	if db == nil {
		return 0, ErrStoreDown
	}
	var bal modelBalanceRow
	if err := db.Table("user_balances").Select("available_balance").Where("user_id = ?", uid).Scan(&bal).Error; err != nil {
		return 0, err
	}
	return int64(math.Round(bal.AvailableBalance * 1e8)), nil
}

func (s *Service) InvalidateBalance(ctx context.Context, uid int64) {
	if s.balance != nil {
		_ = s.balance.Invalidate(ctx, uid)
	}
}

type modelBalanceRow struct {
	AvailableBalance float64 `gorm:"column:available_balance"`
}
