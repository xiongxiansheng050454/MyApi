package channelmanager

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testSettings() Settings {
	return Settings{
		CacheTTL:                   50 * time.Millisecond,
		RetireGrace:                time.Hour,
		EventsChannel:              "test:events",
		BreakerMaxRequests:         1,
		BreakerInterval:            0,
		BreakerTimeout:             50 * time.Millisecond,
		BreakerConsecutiveFailures: 5,
	}
}

func chanInfo(id int64, name string) ChannelInfo {
	return ChannelInfo{ID: id, Name: name, BaseURL: "http://up.example", AuthType: "bearer", Weight: 100, Priority: 0}
}

func snapOf(cis ...ChannelInfo) *Snapshot {
	s := newEmptySnapshot()
	for _, ci := range cis {
		s.Channels[ci.ID] = ci
	}
	return s
}

func addModel(s *Snapshot, model string, ids ...int64) {
	s.Models[model] = append(s.Models[model], ids...)
}

type fakeSource struct {
	mu    sync.Mutex
	snap  *Snapshot
	calls int
}

func newFakeSource(snap *Snapshot) *fakeSource {
	return &fakeSource{snap: snap}
}

func (f *fakeSource) Load(ctx context.Context) (*Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.snap, nil
}

func (f *fakeSource) set(snap *Snapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snap = snap
}

func (f *fakeSource) loadCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeBus struct {
	mu     sync.Mutex
	handle func(Event)
	events []Event
}

func newFakeBus() *fakeBus { return &fakeBus{} }

func (b *fakeBus) Publish(ctx context.Context, ev Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
	return nil
}

func (b *fakeBus) Listen(ctx context.Context, handle func(Event)) error {
	b.mu.Lock()
	b.handle = handle
	b.mu.Unlock()
	<-ctx.Done()
	b.mu.Lock()
	b.handle = nil
	b.mu.Unlock()
	return ctx.Err()
}

func (b *fakeBus) emit(ev Event) {
	b.mu.Lock()
	h := b.handle
	b.mu.Unlock()
	if h != nil {
		h(ev)
	}
}

func (b *fakeBus) listening() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.handle != nil
}

func newManager(src Source, bus Bus) *Manager {
	m := New(testSettings(), src, bus, discardLog())
	return m
}

func waitCond(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within", d)
}

func TestSyncRemovalDisablesAndCleans(t *testing.T) {
	c1 := chanInfo(1, "c1")
	src := newFakeSource(snapOf(c1))
	m := newManager(src, nil)
	m.syncOnce()

	h, err := m.Acquire(c1.ID)
	if err != nil {
		t.Fatalf("acquire should succeed: %v", err)
	}
	h.Done()

	src.set(snapOf())
	m.syncOnce()

	if _, err := m.Acquire(c1.ID); err == nil {
		t.Fatal("acquire after removal should fail")
	}
	m.cleanupOnce()
	m.mu.Lock()
	_, has := m.channels[c1.ID]
	m.mu.Unlock()
	if has {
		t.Fatal("channel should be evicted when refs==0 after retiring")
	}
}

func TestInFlightDelaysEviction(t *testing.T) {
	c1 := chanInfo(1, "c1")
	src := newFakeSource(snapOf(c1))
	m := newManager(src, nil)
	m.syncOnce()

	h, err := m.Acquire(c1.ID)
	if err != nil {
		t.Fatal(err)
	}

	src.set(snapOf())
	m.syncOnce()

	m.mu.Lock()
	_, has := m.channels[c1.ID]
	m.mu.Unlock()
	if !has {
		t.Fatal("retiring channel should stay while in-flight")
	}
	if _, err := m.Acquire(c1.ID); err == nil {
		t.Fatal("new acquire must be rejected while retiring")
	}

	h.Done()

	m.mu.Lock()
	_, has = m.channels[c1.ID]
	m.mu.Unlock()
	if has {
		t.Fatal("channel should be evicted after in-flight completes")
	}
	h.Done()
}

func TestGraceForceEvicts(t *testing.T) {
	c1 := chanInfo(1, "c1")
	src := newFakeSource(snapOf(c1))
	now := time.Now()
	m := New(testSettings(), src, nil, discardLog())
	m.now = func() time.Time { return now }
	m.syncOnce()

	h, err := m.Acquire(c1.ID)
	if err != nil {
		t.Fatal(err)
	}
	src.set(snapOf())
	m.syncOnce()

	now = now.Add(time.Hour + time.Second)
	m.cleanupOnce()

	m.mu.Lock()
	_, has := m.channels[c1.ID]
	m.mu.Unlock()
	if has {
		t.Fatal("channel should be force-evicted after grace")
	}
	h.Done()
}

func TestEventDeleteRetires(t *testing.T) {
	c1 := chanInfo(1, "c1")
	src := newFakeSource(snapOf(c1))
	bus := newFakeBus()
	m := newManager(src, bus)
	m.Start()
	defer m.Shutdown()

	m.syncOnce()
	if _, err := m.Acquire(c1.ID); err != nil {
		t.Fatalf("channel should be active: %v", err)
	}

	waitCond(t, time.Second, bus.listening)
	bus.emit(Event{Type: EventDeleted, ChannelID: c1.ID})

	waitCond(t, 2*time.Second, func() bool {
		_, err := m.Acquire(c1.ID)
		return err != nil
	})
}

func TestChangedTriggersReload(t *testing.T) {
	src := newFakeSource(snapOf(chanInfo(1, "c1")))
	m := newManager(src, nil)
	m.syncOnce()
	before := src.loadCalls()

	m.handleEvent(Event{Type: EventChanged, ChannelID: 1})

	waitCond(t, time.Second, func() bool { return src.loadCalls() > before })
}

func TestCandidatesFilterOpenBreaker(t *testing.T) {
	c1 := chanInfo(1, "c1")
	c2 := chanInfo(2, "c2")
	snap := snapOf(c1, c2)
	addModel(snap, "gpt-4", c1.ID, c2.ID)
	src := newFakeSource(snap)
	m := newManager(src, nil)
	m.syncOnce()

	h, err := m.Acquire(c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		_ = h.Execute(func() error { return context.DeadlineExceeded })
	}
	h.Done()

	cands := m.Candidates("gpt-4")
	if len(cands) != 1 || cands[0].ID != c1.ID {
		t.Fatalf("expected only healthy channel, got %+v", cands)
	}
}

func TestConcurrentLifecycle(t *testing.T) {
	c1 := chanInfo(1, "c1")
	src := newFakeSource(snapOf(c1))
	m := newManager(src, nil)
	m.syncOnce()

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				h, err := m.Acquire(c1.ID)
				if err == nil {
					_ = h.Info()
					h.Done()
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			src.set(snapOf())
			m.syncOnce()
			src.set(snapOf(c1))
			m.syncOnce()
		}
	}()
	wg.Wait()
	m.cleanupOnce()
}

func TestCandidatesFilterByBalance(t *testing.T) {
	zero := 0.0
	neg := -1.5
	five := 5.0
	c1 := chanInfo(1, "c1")
	c1.Balance = &zero
	c2 := chanInfo(2, "c2") // nil = 不限
	c3 := chanInfo(3, "c3")
	c3.Balance = &neg
	c4 := chanInfo(4, "c4")
	c4.Balance = &five

	snap := snapOf(c1, c2, c3, c4)
	addModel(snap, "gpt-4", 1, 2, 3, 4)

	// threshold 0：剔除 <=0，保留 nil 与 5
	m := New(Settings{FilterExhaustedChannels: true, LowBalanceThreshold: 0}, newFakeSource(snap), nil, discardLog())
	m.syncOnce()
	cands := m.Candidates("gpt-4")
	if len(cands) != 2 || cands[0].ID != 2 || cands[1].ID != 4 {
		t.Fatalf("threshold=0 expect channels [2,4], got %v", idsOf(cands))
	}

	// threshold 10：仅保留 nil(2)
	m2 := New(Settings{FilterExhaustedChannels: true, LowBalanceThreshold: 10}, newFakeSource(snap), nil, discardLog())
	m2.syncOnce()
	if got := idsOf(m2.Candidates("gpt-4")); len(got) != 1 || got[0] != 2 {
		t.Fatalf("threshold=10 expect [2], got %v", got)
	}

	// 关闭筛选：全部保留
	m3 := New(Settings{FilterExhaustedChannels: false}, newFakeSource(snap), nil, discardLog())
	m3.syncOnce()
	if got := m3.Candidates("gpt-4"); len(got) != 4 {
		t.Fatalf("filter disabled expect 4, got %v", idsOf(got))
	}
}

func idsOf(list []ChannelInfo) []int64 {
	out := make([]int64, 0, len(list))
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

func TestRetireIdempotentAndPublish(t *testing.T) {
	c1 := chanInfo(1, "c1")
	src := newFakeSource(snapOf(c1))
	bus := newFakeBus()
	m := newManager(src, bus)
	m.syncOnce()

	if !m.retire(c1.ID, "t1") {
		t.Fatal("first retire should transition")
	}
	if m.retire(c1.ID, "t2") {
		t.Fatal("second retire should be no-op")
	}
	m.NotifyDeleted(c1.ID)
	if m.retire(c1.ID, "t3") {
		t.Fatal("retire after delete event should be no-op")
	}
	bus.mu.Lock()
	n := len(bus.events)
	bus.mu.Unlock()
	if n < 1 {
		t.Fatal("delete should be published")
	}
}
