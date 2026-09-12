package ratelimit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"MyApi/internal/auth"
	"MyApi/internal/config"
	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/platform/cachekeys"
)

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testService(counter counterStore, rules []model.RateLimitRule) *Service {
	s := &Service{
		cfg:     config.Default(),
		log:     discardLog(),
		counter: counter,
	}
	s.cache.mu.Lock()
	s.cache.rules = rules
	s.cache.loadedAt = time.Now()
	s.cache.mu.Unlock()
	return s
}

func ident(userID, keyID int64, overrides map[string]int64) *auth.Identity {
	return &auth.Identity{UserID: userID, ApiKeyID: keyID, AllowAll: true, Overrides: overrides}
}

type fakeRLStore struct {
	mu         sync.Mutex
	slideCall  map[string]int
	denyAlways map[string]bool
	denyCount  map[string]int
	retry      map[string]int64
	active     map[string]int
	released   []string
}

func newFakeRLStore() *fakeRLStore {
	return &fakeRLStore{
		slideCall:  map[string]int{},
		denyAlways: map[string]bool{},
		denyCount:  map[string]int{},
		retry:      map[string]int64{},
		active:     map[string]int{},
	}
}

func (f *fakeRLStore) AllowSliding(ctx context.Context, base string, now, window, limit, delta int64) (bool, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slideCall[base]++
	r := f.retry[base]
	if r <= 0 {
		r = 60
	}
	if f.denyAlways[base] {
		return false, r, nil
	}
	if n, ok := f.denyCount[base]; ok && f.slideCall[base] <= n {
		return false, r, nil
	}
	return true, 0, nil
}

func (f *fakeRLStore) TryAcquire(ctx context.Context, key string, limit, ttl int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if int64(f.active[key]) >= limit {
		return false, nil
	}
	f.active[key]++
	return true, nil
}

func (f *fakeRLStore) Release(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active[key] > 0 {
		f.active[key]--
	}
	f.released = append(f.released, key)
	return nil
}

func rule(id int64, tt, tv, metric string, limit, window int, action string, priority int) model.RateLimitRule {
	return model.RateLimitRule{
		ID: id, TargetType: tt, TargetValue: tv, Metric: metric,
		LimitValue: limit, WindowSeconds: window, Action: action, Priority: priority,
		Enabled: true,
	}
}

func TestSelectRulesScope(t *testing.T) {
	rules := []model.RateLimitRule{
		rule(1, model.TargetGlobal, "*", model.MetricRPM, 100, 60, model.ActionReject, 0),
		rule(2, model.TargetUser, "5", model.MetricRPM, 50, 60, model.ActionReject, 0),
		rule(3, model.TargetAPIKey, "9", model.MetricRPM, 30, 60, model.ActionReject, 0),
		rule(4, model.TargetUser, "5", model.MetricTPM, 9000, 60, model.ActionReject, 0),
		rule(5, model.TargetModel, "gpt-4", model.MetricRPM, 40, 60, model.ActionReject, 0),
		rule(6, model.TargetModel, "other", model.MetricRPD, 10, 86400, model.ActionReject, 0),
	}
	k := ident(5, 9, nil)
	rpm := selectRulesForMetric(rules, k, "gpt-4", model.MetricRPM)
	if len(rpm) != 1 || rpm[0].ID != 3 {
		t.Fatalf("rpm should pick api_key rule only, got %+v", rpm)
	}
	tpm := selectRulesForMetric(rules, k, "gpt-4", model.MetricTPM)
	if len(tpm) != 1 || tpm[0].ID != 4 {
		t.Fatalf("tpm should pick user rule, got %+v", tpm)
	}
	rpd := selectRulesForMetric(rules, k, "gpt-4", model.MetricRPD)
	if len(rpd) != 0 {
		t.Fatalf("rpd should match nothing (model other), got %+v", rpd)
	}
}

func TestGateRejectByOverride(t *testing.T) {
	store := newFakeRLStore()
	s := testService(store, nil)
	k := ident(5, 9, map[string]int64{model.MetricRPM: 5})
	base := cachekeys.RateLimitBase(model.TargetGlobal, "*", model.MetricRPM, 60)
	store.denyAlways[base] = true
	_, err := s.Gate(context.Background(), k, "gpt-4", func() int64 { return 1 })
	var rl *apperr.RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter <= 0 {
		t.Fatalf("expected RateLimitedError, got %v", err)
	}
}

func TestGateQueueWaitsThenAllowed(t *testing.T) {
	store := newFakeRLStore()
	r := rule(1, model.TargetUser, "5", model.MetricRPM, 10, 60, model.ActionQueue, 0)
	s := testService(store, []model.RateLimitRule{r})
	base := cachekeys.RateLimitBase(model.TargetUser, "5", model.MetricRPM, 60)
	store.denyCount[base] = 1 // first call denied, second allowed
	k := ident(5, 9, nil)
	_, err := s.Gate(context.Background(), k, "gpt-4", func() int64 { return 1 })
	if err != nil {
		t.Fatalf("queue should eventually allow, got %v", err)
	}
	store.mu.Lock()
	calls := store.slideCall[base]
	store.mu.Unlock()
	if calls < 2 {
		t.Fatalf("expected at least 2 sliding calls, got %d", calls)
	}
}

func TestGateConcurrencyRejectAndRelease(t *testing.T) {
	store := newFakeRLStore()
	r := rule(1, model.TargetUser, "5", model.MetricConcurrency, 1, 60, model.ActionReject, 0)
	s := testService(store, []model.RateLimitRule{r})
	k := ident(5, 9, nil)

	rel, err := s.Gate(context.Background(), k, "gpt-4", func() int64 { return 1 })
	if err != nil {
		t.Fatalf("first acquire should succeed, got %v", err)
	}
	if rel == nil {
		t.Fatal("expected release func")
	}
	rel()
	rel() // double release must be safe

	rel2, err := s.Gate(context.Background(), k, "gpt-4", func() int64 { return 1 })
	if err != nil {
		t.Fatalf("second acquire after release should succeed, got %v", err)
	}

	_, err = s.Gate(context.Background(), k, "gpt-4", func() int64 { return 1 })
	var rl *apperr.RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("expected RateLimitedError for concurrency cap, got %v", err)
	}
	rel2()
}

func TestRateLimitedErrorMessage(t *testing.T) {
	e := &apperr.RateLimitedError{RetryAfter: 7}
	if e.Error() == "" {
		t.Fatal("empty message")
	}
}
