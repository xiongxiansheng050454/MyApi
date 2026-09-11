package service

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

type affinityStore interface {
	Get(ctx context.Context, uid int64, model string) (int64, bool, error)
	Set(ctx context.Context, uid int64, model string, channelID int64, ttl time.Duration) error
}

type redisAffinity struct {
	rdb *redis.Client
}

func newRedisAffinity(rdb *redis.Client) *redisAffinity {
	return &redisAffinity{rdb: rdb}
}

func affinityKey(uid int64, model string) string {
	return "affinity:u:" + strconv.FormatInt(uid, 10) + ":" + model
}

func (a *redisAffinity) Get(ctx context.Context, uid int64, model string) (int64, bool, error) {
	v, err := a.rdb.Get(ctx, affinityKey(uid, model)).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, false, nil
		}
		return 0, false, err
	}
	id, perr := strconv.ParseInt(v, 10, 64)
	if perr != nil {
		return 0, false, nil
	}
	return id, true, nil
}

func (a *redisAffinity) Set(ctx context.Context, uid int64, model string, channelID int64, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return a.rdb.Set(ctx, affinityKey(uid, model), strconv.FormatInt(channelID, 10), ttl).Err()
}
