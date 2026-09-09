package channelmanager

import (
	"errors"
	"fmt"

	"github.com/sony/gobreaker"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type breakerWrap struct {
	cb *gobreaker.CircuitBreaker
}

func (w *breakerWrap) isOpen() bool {
	return w.cb.State() == gobreaker.StateOpen
}

func (w *breakerWrap) execute(fn func() error) error {
	_, err := w.cb.Execute(func() (any, error) { return nil, fn() })
	if errors.Is(err, gobreaker.ErrOpenState) {
		return ErrCircuitOpen
	}
	return err
}

func (m *Manager) gobreakerSettings(id int64) gobreaker.Settings {
	return gobreaker.Settings{
		Name:        fmt.Sprintf("channel-%d", id),
		MaxRequests: m.settings.BreakerMaxRequests,
		Interval:    m.settings.BreakerInterval,
		Timeout:     m.settings.BreakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			fail := m.settings.BreakerConsecutiveFailures
			if fail == 0 {
				fail = 5
			}
			return counts.ConsecutiveFailures >= fail
		},
	}
}
