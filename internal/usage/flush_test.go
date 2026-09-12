package usage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"MyApi/internal/config"
)

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeDeltaStore struct {
	mu   sync.Mutex
	live map[string]map[int64]usageDelta
	max  map[string]int64
	proc map[string]*fakeProc
	seq  int
}

type fakeProc struct {
	date string
	data map[int64]usageDelta
	max  int64
}

func newFakeDeltaStore() *fakeDeltaStore {
	return &fakeDeltaStore{live: map[string]map[int64]usageDelta{}, max: map[string]int64{}, proc: map[string]*fakeProc{}}
}

func (f *fakeDeltaStore) Add(ctx context.Context, date string, uid int64, d usageDelta, maxID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.live[date] == nil {
		f.live[date] = map[int64]usageDelta{}
	}
	cur := f.live[date][uid]
	cur.add(d)
	f.live[date][uid] = cur
	if maxID > f.max[date] {
		f.max[date] = maxID
	}
	return nil
}

func (f *fakeDeltaStore) ActiveDates(ctx context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for d, m := range f.live {
		if len(m) > 0 {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeDeltaStore) Rename(ctx context.Context, date string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.live[date]) == 0 {
		return "", false, nil
	}
	f.seq++
	key := date + ":proc"
	f.proc[key] = &fakeProc{date: date, data: f.live[date], max: f.max[date]}
	delete(f.live, date)
	return key, true, nil
}

func (f *fakeDeltaStore) Read(ctx context.Context, processing string) (map[int64]usageDelta, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.proc[processing]
	if p == nil {
		return nil, 0, errors.New("proc gone")
	}
	out := map[int64]usageDelta{}
	for k, v := range p.data {
		out[k] = v
	}
	return out, p.max, nil
}

func (f *fakeDeltaStore) Drop(ctx context.Context, date, processing string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.proc, processing)
	return nil
}

func (f *fakeDeltaStore) MergeBack(ctx context.Context, date, processing string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.proc[processing]
	if p == nil {
		return nil
	}
	if f.live[date] == nil {
		f.live[date] = map[int64]usageDelta{}
	}
	for uid, d := range p.data {
		cur := f.live[date][uid]
		cur.add(d)
		f.live[date][uid] = cur
	}
	if p.max > f.max[date] {
		f.max[date] = p.max
	}
	delete(f.proc, processing)
	return nil
}

type dailyCall struct {
	date  string
	per   map[int64]usageDelta
	maxID int64
}

type fakeDaily struct {
	mu        sync.Mutex
	calls     []dailyCall
	failTimes int
}

func (f *fakeDaily) Apply(ctx context.Context, date string, per map[int64]usageDelta, maxID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failTimes > 0 {
		f.failTimes--
		return errors.New("db down")
	}
	f.calls = append(f.calls, dailyCall{date: date, per: per, maxID: maxID})
	return nil
}

func newFlushService(delta deltaStore, daily dailyAggregator) *Service {
	s := &Service{cfg: config.Default(), log: discardLog(), delta: delta, daily: daily}
	s.loc = time.UTC
	return s
}

func TestFlushOnceAppliesAndDrops(t *testing.T) {
	delta := newFakeDeltaStore()
	daily := &fakeDaily{}
	s := newFlushService(delta, daily)
	_ = delta.Add(context.Background(), "2026-09-09", 1, usageDelta{Input: 10, Output: 5, CostMicro: 123, Req: 1, OK: 1}, 42)

	s.flushOnce()

	if len(daily.calls) != 1 {
		t.Fatalf("expected 1 apply, got %d", len(daily.calls))
	}
	c := daily.calls[0]
	if c.date != "2026-09-09" || c.maxID != 42 || c.per[1].Input != 10 || c.per[1].CostMicro != 123 {
		t.Fatalf("unexpected daily call: %+v", c)
	}
	dates, _ := delta.ActiveDates(context.Background())
	if len(dates) != 0 {
		t.Fatalf("live should be empty after flush, got %v", dates)
	}
}

func TestFlushOnceMergesBackOnFailure(t *testing.T) {
	delta := newFakeDeltaStore()
	daily := &fakeDaily{failTimes: 1}
	s := newFlushService(delta, daily)
	_ = delta.Add(context.Background(), "2026-09-09", 1, usageDelta{Input: 10, Req: 1, OK: 1}, 42)

	s.flushOnce() // fails -> merge back

	if len(daily.calls) != 0 {
		t.Fatal("no successful apply expected")
	}
	dates, _ := delta.ActiveDates(context.Background())
	if len(dates) != 1 {
		t.Fatalf("delta should be merged back, got %v", dates)
	}
	s.flushOnce()
	if len(daily.calls) != 1 || daily.calls[0].per[1].Input != 10 {
		t.Fatalf("expected retry to apply, got %+v", daily.calls)
	}
}

func TestDeltaRenameSnapshot(t *testing.T) {
	delta := newFakeDeltaStore()
	_ = delta.Add(context.Background(), "d", 1, usageDelta{Input: 1, Req: 1, OK: 1}, 1)
	proc, ok, _ := delta.Rename(context.Background(), "d")
	if !ok {
		t.Fatal("rename should succeed")
	}
	_ = delta.Add(context.Background(), "d", 1, usageDelta{Input: 2, Req: 1, OK: 1}, 2)
	old, _, _ := delta.Read(context.Background(), proc)
	if old[1].Input != 1 {
		t.Fatalf("processing snapshot should be frozen, got %+v", old[1])
	}
	live, _ := delta.ActiveDates(context.Background())
	if len(live) != 1 {
		t.Fatal("new live delta should exist independently")
	}
	_ = delta.Drop(context.Background(), "d", proc)
	per, _, _ := delta.Read(context.Background(), proc)
	if per != nil {
		t.Fatal("proc should be dropped")
	}
}
