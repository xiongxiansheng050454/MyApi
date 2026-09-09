package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"MyApi/internal/channelmanager"
	"MyApi/internal/model"
	"MyApi/internal/secret"
)

var errChannelGone = errors.New("channel row gone while in flight")

func (s *Service) routeAndForward(ctx context.Context, req *ChatCompletionRequest) (*CompletionResponse, error) {
	if s.Channels == nil {
		return nil, ErrStoreDown
	}
	if !s.Channels.HasModel(req.Model) {
		return nil, ErrModelNotFound
	}
	cands := s.Channels.Candidates(req.Model)
	if len(cands) == 0 {
		return nil, ErrNoHealthyUpstream
	}

	attempts := s.cfg.Upstream.MaxForwardAttempts
	if attempts < 1 {
		attempts = 2
	}
	used := map[int64]bool{}
	networkAttempts := 0

	for networkAttempts < attempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, ok := pickNext(cands, used)
		if !ok {
			break
		}
		used[info.ID] = true

		upstreamName, okUp := s.Channels.UpstreamModel(req.Model, info.ID)
		if !okUp {
			continue
		}
		handle, err := s.Channels.Acquire(info.ID)
		if err != nil {
			continue
		}
		networkAttempts++

		resp, retryable, ferr := s.forwardOnce(ctx, req, handle, info, upstreamName)
		if ferr == nil {
			return resp, nil
		}
		handle.Done()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !retryable {
			return nil, ferr
		}
		s.log.Warn("forward attempt failed", "channel_id", info.ID, "channel", info.Name, "err", ferr)
	}
	return nil, ErrNoHealthyUpstream
}

func (s *Service) forwardOnce(
	ctx context.Context,
	req *ChatCompletionRequest,
	handle *channelmanager.Handle,
	info channelmanager.ChannelInfo,
	upstreamName string,
) (*CompletionResponse, bool /*retryable*/, error) {
	if info.AuthType != "" && info.AuthType != "bearer" {
		return nil, true, fmt.Errorf("unsupported auth_type %q", info.AuthType)
	}

	key, err := s.loadChannelKey(ctx, info.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errChannelGone) {
			return nil, true, err
		}
		if errors.Is(err, secret.ErrNotConfigured) || errors.Is(err, secret.ErrPlaintext) {
			return nil, false, err
		}
		return nil, false, err
	}
	defer secret.Zero(key)

	body, err := rewriteModel(req.Body, upstreamName)
	if err != nil {
		return nil, false, fmt.Errorf("rewrite body: %w", err)
	}

	url := strings.TrimRight(info.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, true, fmt.Errorf("build upstream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+string(key))

	var success *CompletionResponse
	cbErr := handle.Execute(func() error {
		resp, derr := s.forwardClient.Do(httpReq)
		if derr != nil {
			return derr
		}
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			return fmt.Errorf("upstream http %d", resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		success = &CompletionResponse{
			StatusCode:  resp.StatusCode,
			ContentType: ct,
			body:        resp.Body,
			release:     handle.Done,
		}
		return nil
	})
	if cbErr != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		if errors.Is(cbErr, channelmanager.ErrCircuitOpen) {
			return nil, true, cbErr
		}
		return nil, true, cbErr
	}
	return success, false, nil
}

func (s *Service) loadChannelKey(ctx context.Context, id int64) ([]byte, error) {
	db := s.WithContext(ctx)
	if db == nil {
		return nil, ErrStoreDown
	}
	var row struct {
		APIKey string
	}
	if err := db.Model(&model.Channel{}).Select("api_key").Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errChannelGone
		}
		return nil, err
	}
	if s.secret == nil {
		return nil, secret.ErrNotConfigured
	}
	return s.secret.Decrypt(row.APIKey)
}

func rewriteModel(body []byte, upstreamModel string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	m["model"] = upstreamModel
	out, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func pickNext(cands []channelmanager.ChannelInfo, used map[int64]bool) (channelmanager.ChannelInfo, bool) {
	maxP := math.MinInt
	for _, c := range cands {
		if used[c.ID] {
			continue
		}
		if c.Priority > maxP {
			maxP = c.Priority
		}
	}
	var pool []channelmanager.ChannelInfo
	for _, c := range cands {
		if !used[c.ID] && c.Priority == maxP {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		return channelmanager.ChannelInfo{}, false
	}
	return weightedRandom(pool), true
}

func weightedRandom(pool []channelmanager.ChannelInfo) channelmanager.ChannelInfo {
	total := 0
	for _, c := range pool {
		w := c.Weight
		if w <= 0 {
			w = 100
		}
		total += w
	}
	pick := rand.Intn(total)
	for _, c := range pool {
		w := c.Weight
		if w <= 0 {
			w = 100
		}
		if pick < w {
			return c
		}
		pick -= w
	}
	return pool[0]
}
