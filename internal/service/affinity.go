package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
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

// affinityScope 解析粘性会话键：按 (用户, 会话, 模型) 维度。
// 会话标识优先级：X-Session-Id 头（req.SessionID）→ 请求体 OpenAI `user` 字段 → 回退用户维度（_user）。
func (s *Service) affinityScope(req *ChatCompletionRequest) string {
	sess := strings.TrimSpace(req.SessionID)
	if sess == "" && len(req.Body) > 0 {
		var body struct {
			User string `json:"user"`
		}
		if json.Unmarshal(req.Body, &body) == nil {
			sess = strings.TrimSpace(body.User)
		}
	}
	if sess == "" {
		sess = "_user"
	}
	return fmt.Sprintf("affinity:%d:%s:%s", req.Key.UserID, sess, req.Model)
}
