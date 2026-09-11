package service

import (
	"context"
	"errors"
	"testing"

	"MyApi/internal/config"
)

type fakeBalance struct {
	preGen    string
	preErr    error
	checkErr  error
	preCalls  int
	checkCall int
	releases  []int64
	settles   []struct {
		est, actual int64
	}
}

func (f *fakeBalance) PreDeduct(ctx context.Context, uid int64, amountMicro int64) (string, error) {
	f.preCalls++
	return f.preGen, f.preErr
}
func (f *fakeBalance) CheckPositive(ctx context.Context, uid int64) error {
	f.checkCall++
	return f.checkErr
}
func (f *fakeBalance) Release(ctx context.Context, uid int64, gen string, amountMicro int64) error {
	f.releases = append(f.releases, amountMicro)
	return nil
}
func (f *fakeBalance) Settle(ctx context.Context, uid int64, gen string, estMicro, actualMicro int64) error {
	f.settles = append(f.settles, struct{ est, actual int64 }{estMicro, actualMicro})
	return nil
}
func (f *fakeBalance) Invalidate(ctx context.Context, uid int64) error { return nil }

type fakeLedger struct {
	charges []int64
}

func (f *fakeLedger) Charge(ctx context.Context, uid int64, requestID string, amountMicro int64) error {
	f.charges = append(f.charges, amountMicro)
	return nil
}

func newBillingService(b balanceCache, l ledger) *Service {
	cfg := config.Default()
	return &Service{cfg: cfg, log: discardLog(), balance: b, ledger: l, tokenizer: newTokenizerCache(cfg.Billing.DefaultEncoding)}
}

func billingReq(body string) *ChatCompletionRequest {
	return &ChatCompletionRequest{
		Key:   &KeyIdentity{UserID: 1, ApiKeyID: 2, AllowAll: true},
		Model: "gpt-4",
		Body:  []byte(body),
	}
}

func TestReserveCheckInsufficient(t *testing.T) {
	b := &fakeBalance{checkErr: ErrInsufficientBalance}
	s := newBillingService(b, &fakeLedger{})
	if _, err := s.reserve(context.Background(), billingReq(`{"messages":[{"role":"user","content":"hi"}]}`), 1); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
}

func TestReserveUnpricedChecksPositive(t *testing.T) {
	b := &fakeBalance{}
	s := newBillingService(b, &fakeLedger{})
	// DB nil → lookupPricing 返回 0 → 无单价 → 仅校验余额>0
	res, err := s.reserve(context.Background(), billingReq(`{"messages":[]}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.active || b.checkCall != 1 {
		t.Fatalf("unpriced should only CheckPositive, res=%+v checkCall=%d", res, b.checkCall)
	}
}

func TestReserveAndSettleReservation(t *testing.T) {
	b := &fakeBalance{preGen: "g1"}
	s := newBillingService(b, &fakeLedger{})
	// 使用有效 JSON 且提供定价：直接调用 settleReservation 验证"多退少补"转发
	res := reservation{active: true, gen: "g1", amountMicro: 1000}
	s.settleReservation(context.Background(), 1, res, 250)
	if len(b.settles) != 1 || b.settles[0].est != 1000 || b.settles[0].actual != 250 {
		t.Fatalf("unexpected settle: %+v", b.settles)
	}
	s.releaseReservation(context.Background(), 1, res)
	if len(b.releases) != 1 || b.releases[0] != 1000 {
		t.Fatalf("unexpected release: %+v", b.releases)
	}
}

func TestTokenizerCounts(t *testing.T) {
	s := newBillingService(&fakeBalance{}, &fakeLedger{})
	if n := s.tokenizer.count("gpt-4", "hello world"); n <= 0 {
		t.Fatalf("token count should be > 0, got %d", n)
	}
	if n := s.tokenizer.count("unknown-model-xyz", "你好，世界"); n <= 0 {
		t.Fatalf("fallback token count should be > 0, got %d", n)
	}
}
