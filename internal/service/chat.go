package service

import (
	"context"
	"io"
	"sync"
	"time"
)

type ChatCompletionRequest struct {
	Key       *KeyIdentity
	Model     string
	Body      []byte
	RequestID string
	ClientIP  string
	StartedAt time.Time
}

type CompletionResponse struct {
	StatusCode  int
	ContentType string
	body        io.ReadCloser
	release     func()
	closeOnce   sync.Once
	meta        *usageMeta
}

func (r *CompletionResponse) Body() io.ReadCloser { return r.body }

func (r *CompletionResponse) Close() {
	r.closeOnce.Do(func() {
		if r.body != nil {
			_ = r.body.Close()
		}
		if r.release != nil {
			r.release()
		}
	})
}

func (r *CompletionResponse) addRelease(fn func()) {
	if fn == nil {
		return
	}
	if r.release == nil {
		r.release = fn
		return
	}
	prev := r.release
	r.release = func() { prev(); fn() }
}

func (s *Service) ChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*CompletionResponse, error) {
	if !req.Key.AllowAll {
		allowed := false
		for _, m := range req.Key.Models {
			if m == req.Model {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrModelNotFound
		}
	}

	release, err := s.rateLimitGate(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := s.routeAndForward(ctx, req)
	if err != nil {
		if release != nil {
			release()
		}
		return nil, err
	}
	if resp == nil {
		if release != nil {
			release()
		}
		return nil, ErrNoHealthyUpstream
	}
	resp.addRelease(release)
	resp.addRelease(func() { s.finalizeMeta(resp.meta) })
	return resp, nil
}

// ChatCompletionStream 处理流式请求：逐候选尝试，未向下游写出前可同渠道重试；
// 已写出后失败则补发 error 事件终止；客户端断开即取消上游。
func (s *Service) ChatCompletionStream(ctx context.Context, req *ChatCompletionRequest, sink StreamWriter) error {
	if !req.Key.AllowAll {
		allowed := false
		for _, m := range req.Key.Models {
			if m == req.Model {
				allowed = true
				break
			}
		}
		if !allowed {
			return ErrModelNotFound
		}
	}

	release, err := s.rateLimitGate(ctx, req)
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}

	if s.Channels == nil {
		return ErrStoreDown
	}
	if !s.Channels.HasModel(req.Model) {
		return ErrModelNotFound
	}
	cands := s.Channels.Candidates(req.Model)
	if len(cands) == 0 {
		return ErrNoHealthyUpstream
	}

	var stickyID int64
	if s.affinity != nil && s.cfg.Routing.StickyEnabled {
		if id, ok, err := s.affinity.Get(ctx, req.Key.UserID, req.Model); err == nil && ok {
			stickyID = id
		}
	}
	ordered := planCandidates(cands, stickyID,
		s.cfg.Routing.MaxAttemptsPerPriority, s.cfg.Routing.MaxTotalAttempts, s.cfg.Routing.TryNextPriority)

	retries := s.cfg.Routing.MaxRetriesPerChannel
	if retries < 0 {
		retries = 0
	}
	totalLimit := s.cfg.Routing.MaxTotalAttempts
	totalAttempts := 0
	idle := time.Duration(s.cfg.Upstream.StreamIdleTimeoutSeconds) * time.Second

	for _, info := range ordered {
		upstreamName, okUp := s.Channels.UpstreamModel(req.Model, info.ID)
		if !okUp {
			continue
		}
		for attempt := 0; attempt <= retries; attempt++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if totalLimit > 0 && totalAttempts >= totalLimit {
				return ErrNoHealthyUpstream
			}
			totalAttempts++
			attemptReqID := attemptRequestID(req.RequestID, totalAttempts)

			handle, err := s.Channels.Acquire(info.ID)
			if err != nil {
				break
			}
			res, err := s.reserve(ctx, req, info.ID)
			if err != nil {
				handle.Done()
				return err
			}

			upCtx, cancel := context.WithCancel(ctx)
			conn, sent, retryable, oerr := s.openUpstream(upCtx, req, handle, info, upstreamName)
			if oerr != nil {
				cancel()
				handle.Done()
				if sent {
					s.settleFailedAttempt(ctx, req, info, upstreamName, attemptReqID, res)
				} else {
					s.releaseReservation(ctx, req.Key.UserID, res)
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if !retryable {
					return oerr
				}
				continue
			}

			meta := conn.Meta
			meta.reserve = res
			sink.Header(conn.StatusCode, conn.ContentType)

			if conn.StatusCode >= 400 {
				full, _ := io.ReadAll(io.LimitReader(conn.Body, maxBufferedBody))
				_ = conn.Body.Close()
				_, _ = sink.Write(full)
				sink.Flush()
				cancel()
				handle.Done()
				s.finalizeMeta(meta)
				return nil
			}

			// 下游断开（ctx 取消）时立即取消上游
			watchDone := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					cancel()
				case <-watchDone:
				}
			}()

			written, rerr := s.relayStream(upCtx, conn.Body, sink, meta, meta.injectedUsage, idle, cancel)
			close(watchDone)
			_ = conn.Body.Close()
			cancel()
			handle.Done()

			if rerr == nil {
				s.finalizeMeta(meta)
				s.setAffinity(ctx, req.Key.UserID, req.Model, info.ID)
				return nil
			}
			if written {
				// 已向下游输出，无法重试：结算已收内容并补发 error 事件终止
				s.finalizeMeta(meta)
				writeStreamError(sink)
				return nil
			}
			// 未输出：按场景2（输入计费）并同渠道重试
			s.settleFailedAttempt(ctx, req, info, upstreamName, attemptReqID, res)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.log.Warn("stream attempt failed", "channel_id", info.ID, "attempt", attempt+1, "err", rerr)
		}
	}
	return ErrNoHealthyUpstream
}
