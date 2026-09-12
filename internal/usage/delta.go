package usage

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"MyApi/internal/platform/cachekeys"
)

type usageDelta struct {
	Input     int64
	Output    int64
	Cached    int64
	CostMicro int64
	Req       int64
	OK        int64
	Err       int64
}

func (d *usageDelta) add(o usageDelta) {
	d.Input += o.Input
	d.Output += o.Output
	d.Cached += o.Cached
	d.CostMicro += o.CostMicro
	d.Req += o.Req
	d.OK += o.OK
	d.Err += o.Err
}

type deltaStore interface {
	Add(ctx context.Context, date string, uid int64, d usageDelta, maxID int64) error
	ActiveDates(ctx context.Context) ([]string, error)
	Rename(ctx context.Context, date string) (processing string, ok bool, err error)
	Read(ctx context.Context, processing string) (map[int64]usageDelta, int64, error)
	Drop(ctx context.Context, date, processing string) error
	MergeBack(ctx context.Context, date, processing string) error
}

const deltaAddLua = `
local live  = KEYS[1]
local dates = KEYS[2]
local p = 'u:' .. ARGV[2] .. ':'
local fields = {'in','out','cached','cost_micro','req','ok','err'}
local vals   = {ARGV[3], ARGV[4], ARGV[5], ARGV[6], ARGV[7], ARGV[8], ARGV[9]}
for i = 1, #fields do
  local v = tonumber(vals[i])
  if v ~= 0 then
    redis.call('HINCRBY', live, p .. fields[i], v)
  end
end
local mid = tonumber(ARGV[10])
local cur = tonumber(redis.call('HGET', live, '_max_id') or '0')
if mid > cur then
  redis.call('HSET', live, '_max_id', ARGV[10])
end
redis.call('EXPIRE', live, tonumber(ARGV[1]))
redis.call('SADD', dates, ARGV[11])
return 1
`

const deltaMergeLua = `
local proc  = KEYS[1]
local live  = KEYS[2]
local dates = KEYS[3]
local h = redis.call('HGETALL', proc)
for i = 1, #h, 2 do
  local f = h[i]
  local v = h[i+1]
  if f == '_max_id' then
    local cur = tonumber(redis.call('HGET', live, f) or '0')
    if tonumber(v) > cur then
      redis.call('HSET', live, f, v)
    end
  else
    redis.call('HINCRBY', live, f, v)
  end
end
redis.call('EXPIRE', live, tonumber(ARGV[1]))
redis.call('SADD', dates, ARGV[2])
redis.call('DEL', proc)
return 1
`

const deltaDropLua = `
local proc  = KEYS[1]
local live  = KEYS[2]
local dates = KEYS[3]
redis.call('DEL', proc)
if redis.call('EXISTS', live) == 0 then
  redis.call('SREM', dates, ARGV[1])
end
return 1
`

type redisDeltaStore struct {
	rdb *redis.Client
	ttl int64
}

func newRedisDeltaStore(rdb *redis.Client, ttlSeconds int64) *redisDeltaStore {
	if ttlSeconds <= 0 {
		ttlSeconds = 168 * 3600
	}
	return &redisDeltaStore{rdb: rdb, ttl: ttlSeconds}
}

func (s *redisDeltaStore) Add(ctx context.Context, date string, uid int64, d usageDelta, maxID int64) error {
	return s.rdb.Eval(ctx, deltaAddLua,
		[]string{cachekeys.StatsDelta(date), cachekeys.StatsDates()},
		strconv.FormatInt(s.ttl, 10),
		strconv.FormatInt(uid, 10),
		strconv.FormatInt(d.Input, 10),
		strconv.FormatInt(d.Output, 10),
		strconv.FormatInt(d.Cached, 10),
		strconv.FormatInt(d.CostMicro, 10),
		strconv.FormatInt(d.Req, 10),
		strconv.FormatInt(d.OK, 10),
		strconv.FormatInt(d.Err, 10),
		strconv.FormatInt(maxID, 10),
		date,
	).Err()
}

func (s *redisDeltaStore) ActiveDates(ctx context.Context) ([]string, error) {
	return s.rdb.SMembers(ctx, cachekeys.StatsDates()).Result()
}

func (s *redisDeltaStore) Rename(ctx context.Context, date string) (string, bool, error) {
	proc := cachekeys.StatsFlushing(date, timeNow().UnixNano())
	err := s.rdb.Rename(ctx, cachekeys.StatsDelta(date), proc).Err()
	if err != nil {
		if strings.Contains(err.Error(), "no such key") {
			return "", false, nil
		}
		return "", false, err
	}
	return proc, true, nil
}

func (s *redisDeltaStore) Read(ctx context.Context, processing string) (map[int64]usageDelta, int64, error) {
	raw, err := s.rdb.HGetAll(ctx, processing).Result()
	if err != nil {
		return nil, 0, err
	}
	out := map[int64]usageDelta{}
	var maxID int64
	for field, val := range raw {
		if field == "_max_id" {
			maxID, _ = strconv.ParseInt(val, 10, 64)
			continue
		}
		if !strings.HasPrefix(field, "u:") {
			continue
		}
		rest := strings.TrimPrefix(field, "u:")
		idx := strings.LastIndex(rest, ":")
		if idx <= 0 {
			continue
		}
		uid, err := strconv.ParseInt(rest[:idx], 10, 64)
		if err != nil {
			continue
		}
		n, _ := strconv.ParseInt(val, 10, 64)
		d := out[uid]
		switch rest[idx+1:] {
		case "in":
			d.Input = n
		case "out":
			d.Output = n
		case "cached":
			d.Cached = n
		case "cost_micro":
			d.CostMicro = n
		case "req":
			d.Req = n
		case "ok":
			d.OK = n
		case "err":
			d.Err = n
		}
		out[uid] = d
	}
	return out, maxID, nil
}

func (s *redisDeltaStore) Drop(ctx context.Context, date, processing string) error {
	return s.rdb.Eval(ctx, deltaDropLua,
		[]string{processing, cachekeys.StatsDelta(date), cachekeys.StatsDates()}, date).Err()
}

func (s *redisDeltaStore) MergeBack(ctx context.Context, date, processing string) error {
	return s.rdb.Eval(ctx, deltaMergeLua,
		[]string{processing, cachekeys.StatsDelta(date), cachekeys.StatsDates()},
		strconv.FormatInt(s.ttl, 10), date).Err()
}

var timeNow = time.Now
