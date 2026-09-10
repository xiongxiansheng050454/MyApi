package channelmanager

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker"
)

var ErrDisabled = errors.New("channel disabled or not exists")

type Manager struct {
	log      *slog.Logger
	settings Settings
	source   Source
	bus      Bus
	now      func() time.Time

	snapshot atomic.Value

	mu       sync.RWMutex
	channels map[int64]*entry
	breakers map[int64]*breakerWrap

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	syncMu sync.Mutex
}

func New(settings Settings, source Source, bus Bus, log *slog.Logger) *Manager {
	m := &Manager{
		log:      log,
		settings: settings,
		source:   source,
		bus:      bus,
		now:      time.Now,
		channels: map[int64]*entry{},
		breakers: map[int64]*breakerWrap{},
	}
	m.snapshot.Store(newEmptySnapshot())
	return m
}

func (m *Manager) Snapshot() *Snapshot {
	return m.snapshot.Load().(*Snapshot)
}

func (m *Manager) HasModel(model string) bool {
	_, ok := m.Snapshot().Models[model]
	return ok
}

func (m *Manager) UpstreamModel(model string, channelID int64) (string, bool) {
	binds, ok := m.Snapshot().ModelBinds[model]
	if !ok {
		return "", false
	}
	name, ok := binds[channelID]
	return name, ok
}

func (m *Manager) Start() {
	if m.cancel != nil {
		return
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.wg.Add(3)
	go m.syncLoop()
	go m.cleanerLoop()
	go m.listenLoop()
}

func (m *Manager) Shutdown() {
	if m.cancel == nil {
		return
	}
	m.cancel()
	m.wg.Wait()
	m.cleanupOnce()
}

func (m *Manager) Candidates(model string) []ChannelInfo {
	snap := m.Snapshot()
	ids := snap.Models[model]
	if len(ids) == 0 {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ChannelInfo, 0, len(ids))
	for _, id := range ids {
		e := m.channels[id]
		if e == nil || e.retiring {
			continue
		}
		if w := m.breakers[id]; w != nil && w.isOpen() {
			continue
		}
		if m.settings.FilterExhaustedChannels && e.info.Balance != nil && *e.info.Balance <= m.settings.LowBalanceThreshold {
			continue
		}
		out = append(out, e.info)
	}
	return out
}

func (m *Manager) Acquire(id int64) (*Handle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.channels[id]
	if !ok || e.retiring {
		return nil, ErrDisabled
	}
	e.refs++
	return &Handle{mgr: m, e: e}, nil
}

func (m *Manager) retire(id int64, reason string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retireLocked(id, reason)
}

func (m *Manager) retireLocked(id int64, reason string) bool {
	e, ok := m.channels[id]
	if !ok || e.retiring {
		return false
	}
	e.retiring = true
	e.retireAt = m.now().Add(m.settings.retireGrace())
	m.log.Info("channel retired", "channel_id", id, "reason", reason,
		"in_flight", e.refs, "grace", m.settings.retireGrace().String())
	return true
}

func (m *Manager) evictLocked(id int64) {
	delete(m.channels, id)
	delete(m.breakers, id)
}

func (m *Manager) release(e *entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.refs--
	if e.refs < 0 {
		e.refs = 0
	}
	if e.refs == 0 && e.retiring {
		if cur, ok := m.channels[e.info.ID]; ok && cur == e {
			m.evictLocked(e.info.ID)
			m.log.Info("channel evicted", "channel_id", e.info.ID, "reason", "in_flight_zero")
		}
	}
}

func (m *Manager) cleanupOnce() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for id, e := range m.channels {
		if !e.retiring {
			continue
		}
		if e.refs == 0 || !now.Before(e.retireAt) {
			m.evictLocked(id)
			m.log.Info("channel evicted", "channel_id", id, "reason", "cleanup")
		}
	}
}

func (m *Manager) syncOnce() {
	if m.source == nil {
		return
	}
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	if m.ctx != nil && m.ctx.Err() != nil {
		return
	}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	snap, err := m.source.Load(ctx)
	if err != nil {
		m.log.Warn("load channel snapshot", "err", err)
		return
	}
	m.applySnapshot(snap)
	m.log.Debug("channel snapshot synced",
		"channels", len(snap.Channels), "models", len(snap.Models))
}

func (m *Manager) applySnapshot(snap *Snapshot) {
	m.mu.Lock()
	for id, e := range m.channels {
		if _, ok := snap.Channels[id]; !ok {
			if !e.retiring {
				m.retireLocked(id, "missing_in_snapshot")
			}
		}
	}
	for id, info := range snap.Channels {
		e, ok := m.channels[id]
		if !ok {
			m.channels[id] = newEntry(info)
			continue
		}
		if !e.retiring {
			e.info = info
		}
	}
	m.mu.Unlock()
	m.snapshot.Store(snap)
}

func (m *Manager) syncLoop() {
	defer m.wg.Done()
	if m.source == nil {
		return
	}
	m.syncOnce()
	t := time.NewTicker(m.settings.cacheInterval())
	defer t.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-t.C:
			m.syncOnce()
			m.cleanupOnce()
		}
	}
}

func (m *Manager) cleanerLoop() {
	defer m.wg.Done()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-t.C:
			m.cleanupOnce()
		}
	}
}

func (m *Manager) listenLoop() {
	defer m.wg.Done()
	if m.bus == nil {
		return
	}
	backoff := 200 * time.Millisecond
	for {
		err := m.bus.Listen(m.ctx, m.handleEvent)
		if m.ctx.Err() != nil {
			return
		}
		m.log.Warn("channel event bus listen", "err", err)
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(backoff):
			if backoff < 5*time.Second {
				backoff *= 2
			}
		}
	}
}

func (m *Manager) handleEvent(ev Event) {
	switch ev.Type {
	case EventDeleted:
		if m.retire(ev.ChannelID, "event:deleted") {
			m.log.Info("channel retired by event", "channel_id", ev.ChannelID)
		}
	case EventChanged:
		m.syncOnce()
	}
}

func (m *Manager) NotifyDeleted(id int64) {
	if m.retire(id, "notify:deleted") {
		m.log.Info("channel retired locally", "channel_id", id)
	}
	m.publish(Event{Type: EventDeleted, ChannelID: id})
}

func (m *Manager) NotifyChanged(id int64) {
	m.syncOnce()
	m.publish(Event{Type: EventChanged, ChannelID: id})
}

func (m *Manager) publish(ev Event) {
	if m.bus == nil {
		return
	}
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := m.bus.Publish(ctx, ev); err != nil {
		m.log.Warn("publish channel event", "err", err, "event", ev.Type, "channel_id", ev.ChannelID)
	}
}

func (m *Manager) ensureBreakerLocked(id int64) *breakerWrap {
	w, ok := m.breakers[id]
	if !ok {
		w = &breakerWrap{cb: gobreaker.NewCircuitBreaker(m.gobreakerSettings(id))}
		m.breakers[id] = w
	}
	return w
}
