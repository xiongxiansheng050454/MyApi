package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"

	"MyApi/internal/platform/cachekeys"
)

type balanceCache interface {
	PreDeduct(ctx context.Context, uid int64, amountMicro int64) (gen string, err error)
	CheckPositive(ctx context.Context, uid int64) error
	Release(ctx context.Context, uid int64, gen string, amountMicro int64) error
	Settle(ctx context.Context, uid int64, gen string, estMicro, actualMicro int64) error
	Invalidate(ctx context.Context, uid int64) error
}

const balancePreDeductLua = `
local bal = KEYS[1]
if redis.call('EXISTS', bal) == 0 then
  return {2, '', '0'}
end
local amount = tonumber(ARGV[1])
local v = tonumber(redis.call('HGET', bal, 'v') or '0')
if v < amount then
  return {1, '', tostring(v)}
end
local gen = redis.call('HGET', bal, 'gen')
redis.call('HINCRBY', bal, 'v', -amount)
redis.call('EXPIRE', bal, tonumber(ARGV[2]))
return {0, gen, tostring(v - amount)}
`

const balanceCheckLua = `
local bal = KEYS[1]
if redis.call('EXISTS', bal) == 0 then
  return 2
end
local v = tonumber(redis.call('HGET', bal, 'v') or '0')
if v <= 0 then
  return 1
end
return 0
`

const balanceSettleLua = `
local bal = KEYS[1]
if redis.call('EXISTS', bal) == 0 then
  return 0
end
local gen = redis.call('HGET', bal, 'gen')
if gen ~= ARGV[1] then
  redis.call('DEL', bal)
  return -1
end
redis.call('HINCRBY', bal, 'v', tonumber(ARGV[2]) - tonumber(ARGV[3]))
redis.call('EXPIRE', bal, tonumber(ARGV[4]))
return 1
`

const balanceUnlockLua = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`

type redisBalance struct {
	rdb       *redis.Client
	ttl       int64
	lockTTLms int64
	load      func(ctx context.Context, uid int64) (int64, error)
}

func newRedisBalance(rdb *redis.Client, ttlSeconds, lockTTLSeconds int, load func(context.Context, int64) (int64, error)) *redisBalance {
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	if lockTTLSeconds <= 0 {
		lockTTLSeconds = 3
	}
	return &redisBalance{
		rdb:       rdb,
		ttl:       int64(ttlSeconds),
		lockTTLms: int64(lockTTLSeconds) * 1000,
		load:      load,
	}
}

func (b *redisBalance) PreDeduct(ctx context.Context, uid int64, amountMicro int64) (string, error) {
	if err := b.ensure(ctx, uid); err != nil {
		return "", err
	}
	for attempt := 0; attempt < 2; attempt++ {
		res, err := b.rdb.Eval(ctx, balancePreDeductLua, []string{cachekeys.Balance(uid)},
			strconv.FormatInt(amountMicro, 10), strconv.FormatInt(b.ttl, 10)).Result()
		if err != nil {
			return "", err
		}
		arr, ok := res.([]interface{})
		if !ok || len(arr) < 3 {
			return "", errBalanceStoreDown
		}
		switch toInt64(arr[0]) {
		case 0:
			return toString(arr[1]), nil
		case 1:
			return "", errInsufficient
		default:
			if err := b.ensure(ctx, uid); err != nil {
				return "", err
			}
		}
	}
	return "", errBalanceStoreDown
}

func (b *redisBalance) CheckPositive(ctx context.Context, uid int64) error {
	if err := b.ensure(ctx, uid); err != nil {
		return err
	}
	res, err := b.rdb.Eval(ctx, balanceCheckLua, []string{cachekeys.Balance(uid)}).Result()
	if err != nil {
		return err
	}
	switch toInt64(res) {
	case 0:
		return nil
	case 1:
		return errInsufficient
	default:
		if err := b.ensure(ctx, uid); err != nil {
			return err
		}
		return nil
	}
}

func (b *redisBalance) Release(ctx context.Context, uid int64, gen string, amountMicro int64) error {
	return b.settle(ctx, uid, gen, amountMicro, 0)
}

func (b *redisBalance) Settle(ctx context.Context, uid int64, gen string, estMicro, actualMicro int64) error {
	return b.settle(ctx, uid, gen, estMicro, actualMicro)
}

func (b *redisBalance) settle(ctx context.Context, uid int64, gen string, estMicro, actualMicro int64) error {
	_, err := b.rdb.Eval(ctx, balanceSettleLua, []string{cachekeys.Balance(uid)},
		gen,
		strconv.FormatInt(estMicro, 10),
		strconv.FormatInt(actualMicro, 10),
		strconv.FormatInt(b.ttl, 10),
	).Result()
	return err
}

func (b *redisBalance) Invalidate(ctx context.Context, uid int64) error {
	return b.rdb.Del(ctx, cachekeys.Balance(uid)).Err()
}

func (b *redisBalance) ensure(ctx context.Context, uid int64) error {
	exists, err := b.rdb.Exists(ctx, cachekeys.Balance(uid)).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	token := randomToken()
	locked, err := b.rdb.SetNX(ctx, cachekeys.BalanceLock(uid), token, time.Duration(b.lockTTLms)*time.Millisecond).Result()
	if err != nil {
		return err
	}
	if !locked {
		for i := 0; i < 10; i++ {
			time.Sleep(30 * time.Millisecond)
			if n, _ := b.rdb.Exists(ctx, cachekeys.Balance(uid)).Result(); n > 0 {
				return nil
			}
		}
	}
	defer func() {
		_ = b.rdb.Eval(ctx, balanceUnlockLua, []string{cachekeys.BalanceLock(uid)}, token).Err()
	}()

	if n, _ := b.rdb.Exists(ctx, cachekeys.Balance(uid)).Result(); n > 0 {
		return nil
	}

	availMicro, err := b.load(ctx, uid)
	if err != nil {
		return err
	}
	gen := randomToken()
	pipe := b.rdb.TxPipeline()
	pipe.HSet(ctx, cachekeys.Balance(uid), "v", strconv.FormatInt(availMicro, 10), "gen", gen)
	pipe.Expire(ctx, cachekeys.Balance(uid), time.Duration(b.ttl)*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	return nil
}

func randomToken() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
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
