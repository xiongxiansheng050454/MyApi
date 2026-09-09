package channelmanager

type Handle struct {
	mgr *Manager
	e   *entry
}

func (h *Handle) Info() ChannelInfo {
	h.mgr.mu.RLock()
	defer h.mgr.mu.RUnlock()
	return h.e.info
}

func (h *Handle) Done() {
	h.mgr.release(h.e)
}

func (h *Handle) Execute(fn func() error) error {
	m := h.mgr
	m.mu.Lock()
	w := m.ensureBreakerLocked(h.e.info.ID)
	m.mu.Unlock()
	return w.execute(fn)
}
