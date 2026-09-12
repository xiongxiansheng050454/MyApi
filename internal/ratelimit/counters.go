package ratelimit

import (
	"context"
	"strconv"

	"github.com/go-redis/redis/v8"

	"MyApi/internal/platform/apperr"
)

const slidingLua = `
local base  = KEYS[1]
local now   = tonumber(ARGV[1])
local win   = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local delta = tonumber(ARGV[4])

local cur     = math.floor(now / win)
local elapsed = now - cur * win
local curKey  = base .. ':' .. tostring(cur)
local prevKey = base .. ':' .. tostring(cur - 1)

local curV  = tonumber(redis.call('GET', curKey) or '0')
local prevV = tonumber(redis.call('GET', prevKey) or '0')
local est   = curV + prevV * ((win - elapsed) / win)

if est + delta <= limit then
  redis.call('INCR', curKey)
  redis.call('EXPIRE', curKey, win * 2 + 10)
  redis.call('EXPIRE', prevKey, win * 2 + 10)
  return {1, 0}
end

local retry = 0
if prevV > 0 then
  local need = est + delta - limit
  retry = math.ceil(need * win / prevV)
else
  retry = win - elapsed
end
if retry < 1 then retry = 1 end
return {0, retry}
`

const gaugeAcquireLua = `
local k     = KEYS[1]
local limit = tonumber(ARGV[1])
local ttl   = tonumber(ARGV[2])
local c     = tonumber(redis.call('GET', k) or '0')
if c >= limit then
  return {0, c}
end
local n = redis.call('INCR', k)
redis.call('EXPIRE', k, ttl)
if n > limit then
  redis.call('DECR', k)
  return {0, c}
end
return {1, n}
`

type counterStore interface {
	AllowSliding(ctx context.Context, base string, now, window, limit, delta int64) (allowed bool, retryAfter int64, err error)
	TryAcquire(ctx context.Context, key string, limit, ttl int64) (bool, error)
	Release(ctx context.Context, key string) error
}

type redisCounter struct {
	rdb *redis.Client
}

func newRedisCounter(rdb *redis.Client) *redisCounter {
	return &redisCounter{rdb: rdb}
}

func (c *redisCounter) AllowSliding(ctx context.Context, base string, now, window, limit, delta int64) (bool, int64, error) {
	res, err := c.rdb.Eval(ctx, slidingLua, []string{base},
		strconv.FormatInt(now, 10),
		strconv.FormatInt(window, 10),
		strconv.FormatInt(limit, 10),
		strconv.FormatInt(delta, 10),
	).Result()
	if err != nil {
		return false, 0, err
	}
	arr, ok := res.([]interface{})
	if !ok || len(arr) != 2 {
		return false, 0, apperr.ErrCounterStoreDown
	}
	return toInt64(arr[0]) == 1, toInt64(arr[1]), nil
}

func (c *redisCounter) TryAcquire(ctx context.Context, key string, limit, ttl int64) (bool, error) {
	res, err := c.rdb.Eval(ctx, gaugeAcquireLua, []string{key},
		strconv.FormatInt(limit, 10),
		strconv.FormatInt(ttl, 10),
	).Result()
	if err != nil {
		return false, err
	}
	arr, ok := res.([]interface{})
	if !ok || len(arr) == 0 {
		return false, apperr.ErrCounterStoreDown
	}
	return toInt64(arr[0]) == 1, nil
}

func (c *redisCounter) Release(ctx context.Context, key string) error {
	n, err := c.rdb.Decr(ctx, key).Result()
	if err != nil {
		return err
	}
	if n < 0 {
		c.rdb.Set(ctx, key, 0, 0)
	}
	return nil
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	}
	return 0
}
