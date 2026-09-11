package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
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

func balanceKey(uid int64) string     { return "bal:" + strconv.FormatInt(uid, 10) }
func balanceLockKey(uid int64) string { return "bal:lock:" + strconv.FormatInt(uid, 10) }

func (b *redisBalance) PreDeduct(ctx context.Context, uid int64, amountMicro int64) (string, error) {
	if err := b.ensure(ctx, uid); err != nil {
		return "", err
	}
	for attempt := 0; attempt < 2; attempt++ {
		res, err := b.rdb.Eval(ctx, balancePreDeductLua, []string{balanceKey(uid)},
			strconv.FormatInt(amountMicro, 10), strconv.FormatInt(b.ttl, 10)).Result()
		if err != nil {
			return "", err
		}
		arr, ok := res.([]interface{})
		if !ok || len(arr) < 3 {
			return "", ErrStoreDown
		}
		switch toInt64(arr[0]) {
		case 0:
			return toString(arr[1]), nil
		case 1:
			return "", ErrInsufficientBalance
		default:
			if err := b.ensure(ctx, uid); err != nil {
				return "", err
			}
		}
	}
	return "", ErrStoreDown
}

func (b *redisBalance) CheckPositive(ctx context.Context, uid int64) error {
	if err := b.ensure(ctx, uid); err != nil {
		return err
	}
	res, err := b.rdb.Eval(ctx, balanceCheckLua, []string{balanceKey(uid)}).Result()
	if err != nil {
		return err
	}
	switch toInt64(res) {
	case 0:
		return nil
	case 1:
		return ErrInsufficientBalance
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
	_, err := b.rdb.Eval(ctx, balanceSettleLua, []string{balanceKey(uid)},
		gen,
		strconv.FormatInt(estMicro, 10),
		strconv.FormatInt(actualMicro, 10),
		strconv.FormatInt(b.ttl, 10),
	).Result()
	return err
}

func (b *redisBalance) Invalidate(ctx context.Context, uid int64) error {
	return b.rdb.Del(ctx, balanceKey(uid)).Err()
}

func (b *redisBalance) ensure(ctx context.Context, uid int64) error {
	exists, err := b.rdb.Exists(ctx, balanceKey(uid)).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	token := randomToken()
	locked, err := b.rdb.SetNX(ctx, balanceLockKey(uid), token, time.Duration(b.lockTTLms)*time.Millisecond).Result()
	if err != nil {
		return err
	}
	if !locked {
		// 其他节点正在重建：短暂等待后复查
		for i := 0; i < 10; i++ {
			time.Sleep(30 * time.Millisecond)
			if n, _ := b.rdb.Exists(ctx, balanceKey(uid)).Result(); n > 0 {
				return nil
			}
		}
		// 锁持有者可能已崩溃，兜底自行重建
	}
	defer func() {
		_ = b.rdb.Eval(ctx, balanceUnlockLua, []string{balanceLockKey(uid)}, token).Err()
	}()

	// 双重检查
	if n, _ := b.rdb.Exists(ctx, balanceKey(uid)).Result(); n > 0 {
		return nil
	}

	availMicro, err := b.load(ctx, uid)
	if err != nil {
		return err
	}
	gen := randomToken()
	pipe := b.rdb.TxPipeline()
	pipe.HSet(ctx, balanceKey(uid), "v", strconv.FormatInt(availMicro, 10), "gen", gen)
	pipe.Expire(ctx, balanceKey(uid), time.Duration(b.ttl)*time.Second)
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
