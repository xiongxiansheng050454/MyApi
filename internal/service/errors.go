package service

import "errors"

var (
	ErrInvalidKey     = errors.New("invalid api key")
	ErrKeyDisabled    = errors.New("api key disabled")
	ErrKeyExpired     = errors.New("api key expired")
	ErrUserSuspended  = errors.New("user suspended")
	ErrModelNotFound  = errors.New("model not found")
	ErrStoreDown      = errors.New("store unavailable")
	ErrNotImplemented = errors.New("not implemented yet")
)
