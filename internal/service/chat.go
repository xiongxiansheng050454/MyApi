package service

import "context"

type ChatCompletionRequest struct {
	Key   *KeyIdentity
	Model string
	Body  []byte
}

func (s *Service) ChatCompletion(ctx context.Context, req *ChatCompletionRequest) error {
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

	if s.Redis != nil && s.Limiter != nil {
		if err := s.rateLimit(ctx, req.Key); err != nil {
			return err
		}
	}

	if s.DB == nil {
		return ErrStoreDown
	}
	return s.routeAndForward(ctx, req)
}
