package service

import (
	"context"
	"time"
)

func (s *Service) startFlusher() {
	s.flushCtx, s.flushCancel = context.WithCancel(context.Background())
	s.flushWG.Add(1)
	go s.flushLoop()
}

func (s *Service) stopFlusher() {
	if s.flushCancel != nil {
		s.flushCancel()
	}
	s.flushWG.Wait()
}

func (s *Service) flushLoop() {
	defer s.flushWG.Done()
	interval := time.Duration(s.cfg.Stats.FlushIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.flushCtx.Done():
			return
		case <-ticker.C:
			s.flushOnce()
		}
	}
}

func (s *Service) flushOnce() {
	if s.delta == nil || s.daily == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dates, err := s.delta.ActiveDates(ctx)
	if err != nil {
		s.log.Warn("stats flush: list active dates", "err", err)
		return
	}
	for _, date := range dates {
		proc, ok, err := s.delta.Rename(ctx, date)
		if err != nil {
			s.log.Warn("stats flush: rename", "err", err, "date", date)
			continue
		}
		if !ok {
			continue
		}
		per, maxID, err := s.delta.Read(ctx, proc)
		if err != nil {
			s.log.Warn("stats flush: read", "err", err, "date", date)
			_ = s.delta.MergeBack(ctx, date, proc)
			continue
		}
		if len(per) == 0 {
			_ = s.delta.Drop(ctx, date, proc)
			continue
		}
		if err := s.daily.Apply(ctx, date, per, maxID); err != nil {
			s.log.Error("stats flush: apply daily", "err", err, "date", date)
			if merr := s.delta.MergeBack(ctx, date, proc); merr != nil {
				s.log.Error("stats flush: merge back", "err", merr, "date", date)
			}
			continue
		}
		if err := s.delta.Drop(ctx, date, proc); err != nil {
			s.log.Warn("stats flush: drop", "err", err, "date", date)
		}
		s.log.Debug("stats flushed", "date", date, "users", len(per), "max_id", maxID)
	}
}
