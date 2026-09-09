package service

import "context"

func (s *Service) rateLimit(ctx context.Context, key *KeyIdentity) error {
	// TODO: 读取 rate_limit_rules 与 client_api_keys.rate_limit_overrides，
	// 用 s.Limiter / redis 计数（rpm/tpm/rpd/tpd/concurrency），按优先级取更严者。
	return ErrNotImplemented
}
