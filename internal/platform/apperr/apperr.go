package apperr

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidKey          = errors.New("invalid api key")
	ErrKeyDisabled         = errors.New("api key disabled")
	ErrKeyExpired          = errors.New("api key expired")
	ErrUserSuspended       = errors.New("user suspended")
	ErrModelNotFound       = errors.New("model not found")
	ErrNoHealthyUpstream   = errors.New("no healthy upstream channel")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrStoreDown           = errors.New("store unavailable")
	ErrNotImplemented      = errors.New("not implemented yet")
	ErrQueueTimeout        = errors.New("rate limit queue timeout")
	ErrCounterStoreDown    = errors.New("counter store unavailable")
)

type RateLimitedError struct {
	RetryAfter int64
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("rate limit exceeded, retry after %ds", e.RetryAfter)
}

// InvalidError 表示请求参数/领域校验失败，由 HTTP 层映射为参数错误码。
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

func Invalid(msg string) error { return &InvalidError{Msg: msg} }
