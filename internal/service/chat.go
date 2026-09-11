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
	resp.addRelease(func() { s.finalizeUsage(resp) })
	return resp, nil
}
