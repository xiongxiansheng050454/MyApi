package billing

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"MyApi/internal/config"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/pricing"
)

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeBalance struct {
	preGen    string
	preErr    error
	checkErr  error
	preCalls  int
	checkCall int
	releases  []int64
	settles   []struct{ est, actual int64 }
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
	return &Service{
		cfg: config.Default(), log: discardLog(), balance: b, ledger: l,
		pricing: pricing.New(nil),
	}
}

func TestReserveCheckInsufficient(t *testing.T) {
	b := &fakeBalance{checkErr: apperr.ErrInsufficientBalance}
	s := newBillingService(b, &fakeLedger{})
	if _, err := s.Reserve(context.Background(), 1, 1, "gpt-4", 0, 0); !errors.Is(err, apperr.ErrInsufficientBalance) {
		t.Fatalf("expected ErrInsufficientBalance, got %v", err)
	}
}

func TestReserveUnpricedChecksPositive(t *testing.T) {
	b := &fakeBalance{}
	s := newBillingService(b, &fakeLedger{})
	// 无单价 -> 仅校验余额 > 0
	res, err := s.Reserve(context.Background(), 1, 1, "gpt-4", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Active || b.checkCall != 1 {
		t.Fatalf("unpriced should only CheckPositive, res=%+v checkCall=%d", res, b.checkCall)
	}
}

func TestSettleAndReleaseReservation(t *testing.T) {
	b := &fakeBalance{preGen: "g1"}
	s := newBillingService(b, &fakeLedger{})
	res := Reservation{Active: true, Gen: "g1", AmountMicro: 1000}
	s.Settle(context.Background(), 1, res, 250)
	if len(b.settles) != 1 || b.settles[0].est != 1000 || b.settles[0].actual != 250 {
		t.Fatalf("unexpected settle: %+v", b.settles)
	}
	s.Release(context.Background(), 1, res)
	if len(b.releases) != 1 || b.releases[0] != 1000 {
		t.Fatalf("unexpected release: %+v", b.releases)
	}
}
