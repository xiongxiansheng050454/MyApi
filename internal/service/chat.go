package service

import (
	"context"
	"io"
	"sync"
)

type ChatCompletionRequest struct {
	Key   *KeyIdentity
	Model string
	Body  []byte
}

type CompletionResponse struct {
	StatusCode  int
	ContentType string
	body        io.ReadCloser
	release     func()
	closeOnce   sync.Once
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

	// TODO: 接入限流模块（rateLimit 尚为占位，故暂不执行）
	return s.routeAndForward(ctx, req)
}
