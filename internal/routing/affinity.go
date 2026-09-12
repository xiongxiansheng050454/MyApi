package routing

import (
	"context"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"

	"MyApi/internal/platform/cachekeys"
)

type affinityStore interface {
	Get(ctx context.Context, key string) (int64, bool, error)
	Set(ctx context.Context, key string, channelID int64, ttl time.Duration) error
}

type redisAffinity struct {
	rdb *redis.Client
}

func newRedisAffinity(rdb *redis.Client) *redisAffinity {
	return &redisAffinity{rdb: rdb}
}

func (a *redisAffinity) Get(ctx context.Context, key string) (int64, bool, error) {
	v, err := a.rdb.Get(ctx, key).Result()
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

func (a *redisAffinity) Set(ctx context.Context, key string, channelID int64, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return a.rdb.Set(ctx, key, strconv.FormatInt(channelID, 10), ttl).Err()
}

// scopeKey 解析粘性会话键：按 (用户, 会话, 模型) 维度；session 为空时回退用户维度。
func scopeKey(uid int64, session, model string) string {
	if session == "" {
		session = "_user"
	}
	return cachekeys.Affinity(uid, session, model)
}
