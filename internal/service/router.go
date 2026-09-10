package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"MyApi/internal/channelmanager"
	"MyApi/internal/model"
	"MyApi/internal/secret"
)

var errChannelGone = errors.New("channel row gone while in flight")

type upstreamConn struct {
	StatusCode  int
	ContentType string
	Body        io.ReadCloser
	Meta        *usageMeta
}

// openUpstream 建立到上游的一次连接（不做 body 读取/转发）。
// 返回 (conn, sent, retryable, err)：sent 表示请求已实际发出。
func (s *Service) openUpstream(
	ctx context.Context,
	req *ChatCompletionRequest,
	handle *channelmanager.Handle,
	info channelmanager.ChannelInfo,
	upstreamName string,
) (*upstreamConn, bool, bool, error) {
	if info.AuthType != "" && info.AuthType != "bearer" {
		return nil, false, true, fmt.Errorf("unsupported auth_type %q", info.AuthType)
	}

	key, err := s.loadChannelKey(ctx, info.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errChannelGone) {
			return nil, false, true, err
		}
		return nil, false, false, err
	}
	defer secret.Zero(key)

	body, stream, wantsUsage, err := rewriteBody(req.Body, upstreamName)
	if err != nil {
		return nil, false, false, fmt.Errorf("rewrite body: %w", err)
	}
	injectedUsage := stream && !wantsUsage

	meta := &usageMeta{
		userID:         req.Key.UserID,
		apiKeyID:       req.Key.ApiKeyID,
		requestID:      req.RequestID,
		clientIP:       req.ClientIP,
		model:          req.Model,
		channelID:      info.ID,
		upstreamModel:  upstreamName,
		startedAt:      req.StartedAt,
		injectedUsage:  injectedUsage,
		stream:         stream,
		estimatedInput: int(s.estimateTokens(req.Body, req.Model)),
	}
	meta.count = func(text string) int { return s.tokenizer.count(req.Model, text) }
	if meta.startedAt.IsZero() {
		meta.startedAt = time.Now()
	}

	url := strings.TrimRight(info.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, false, true, fmt.Errorf("build upstream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+string(key))

	sent := false
	var conn *upstreamConn
	cbErr := handle.Execute(func() error {
		sent = true
		resp, derr := s.forwardClient.Do(httpReq)
		if derr != nil {
			return derr
		}
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			return fmt.Errorf("upstream http %d", resp.StatusCode)
		}
		meta.statusCode = resp.StatusCode
		conn = &upstreamConn{
			StatusCode:  resp.StatusCode,
			ContentType: resp.Header.Get("Content-Type"),
			Body:        resp.Body,
			Meta:        meta,
		}
		return nil
	})
	if cbErr != nil {
		if ctx.Err() != nil {
			return nil, sent, false, ctx.Err()
		}
		return nil, sent, true, cbErr
	}
	return conn, sent, false, nil
}

// forwardOnce 非流式：打开上游并缓冲完整响应，返回 CompletionResponse。
func (s *Service) forwardOnce(
	ctx context.Context,
	req *ChatCompletionRequest,
	handle *channelmanager.Handle,
	info channelmanager.ChannelInfo,
	upstreamName string,
) (*CompletionResponse, bool, bool, error) {
	conn, sent, retryable, err := s.openUpstream(ctx, req, handle, info, upstreamName)
	if err != nil {
		return nil, sent, retryable, err
	}

	full, rerr := io.ReadAll(io.LimitReader(conn.Body, maxBufferedBody))
	_ = conn.Body.Close()
	if rerr != nil {
		return nil, true, false, fmt.Errorf("read upstream body: %w", rerr)
	}

	meta := conn.Meta
	if in, out, cached, content, ok := parseUsageJSON(full); ok {
		meta.setUsage(in, out, cached)
	} else {
		meta.outputText = content
	}
	meta.markFirstByte()
	full = rewriteResponseModel(full, req.Model)

	resp := &CompletionResponse{
		StatusCode:  conn.StatusCode,
		ContentType: conn.ContentType,
		body:        io.NopCloser(bytes.NewReader(full)),
		release:     handle.Done,
		meta:        meta,
	}
	return resp, true, false, nil
}

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

	var stickyID int64
	if s.affinity != nil && s.cfg.Routing.StickyEnabled {
		if id, ok, err := s.affinity.Get(ctx, req.Key.UserID, req.Model); err == nil && ok {
			stickyID = id
		}
	}

	ordered := planCandidates(cands, stickyID,
		s.cfg.Routing.MaxAttemptsPerPriority, s.cfg.Routing.MaxTotalAttempts, s.cfg.Routing.TryNextPriority)

	retries := s.cfg.Routing.MaxRetriesPerChannel
	if retries < 0 {
		retries = 0
	}
	totalLimit := s.cfg.Routing.MaxTotalAttempts
	totalAttempts := 0

	for _, info := range ordered {
		upstreamName, okUp := s.Channels.UpstreamModel(req.Model, info.ID)
		if !okUp {
			continue
		}
		for attempt := 0; attempt <= retries; attempt++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if totalLimit > 0 && totalAttempts >= totalLimit {
				return nil, ErrNoHealthyUpstream
			}
			totalAttempts++
			attemptReqID := attemptRequestID(req.RequestID, totalAttempts)

			handle, err := s.Channels.Acquire(info.ID)
			if err != nil {
				break
			}
			res, err := s.reserve(ctx, req, info.ID)
			if err != nil {
				handle.Done()
				return nil, err
			}

			resp, sent, retryable, ferr := s.forwardOnce(ctx, req, handle, info, upstreamName)
			if ferr == nil {
				resp.meta.reserve = res
				s.setAffinity(ctx, req.Key.UserID, req.Model, info.ID)
				return resp, nil
			}
			handle.Done()
			if sent {
				s.settleFailedAttempt(ctx, req, info, upstreamName, attemptReqID, res)
			} else {
				s.releaseReservation(ctx, req.Key.UserID, res)
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !retryable {
				return nil, ferr
			}
			s.log.Warn("forward attempt failed", "channel_id", info.ID, "channel", info.Name, "attempt", attempt+1, "err", ferr)
		}
	}
	return nil, ErrNoHealthyUpstream
}

func attemptRequestID(base string, n int) string {
	if n <= 1 {
		return base
	}
	return fmt.Sprintf("%s#a%d", base, n)
}

// settleFailedAttempt 场景2：已发送上游但未获得响应 → 按输入 token 计费。
func (s *Service) settleFailedAttempt(ctx context.Context, req *ChatCompletionRequest, info channelmanager.ChannelInfo, upstreamName, attemptReqID string, res reservation) {
	input := int(s.estimateTokens(req.Body, req.Model))
	pIn, pOut, _ := s.lookupPricing(ctx, info.ID, req.Model)
	cost := computeCost(input, 0, 0, pIn, pOut, pIn)
	actualMicro := int64(roundMicro(cost))

	up := upstreamName
	row := model.UsageLog{
		RequestID:            attemptReqID,
		UserID:               req.Key.UserID,
		ApiKeyID:             req.Key.ApiKeyID,
		ChannelID:            info.ID,
		Model:                req.Model,
		UpstreamModel:        &up,
		InputTokens:          input,
		OutputTokens:         0,
		UnitPriceInputPer1M:  pIn,
		UnitPriceOutputPer1M: pOut,
		TotalCost:            cost,
		DurationMs:           int(time.Since(req.StartedAt).Milliseconds()),
		Status:               model.StatusError,
		ErrorCode:            strPtr("upstream_error"),
	}
	if req.ClientIP != "" {
		ip := req.ClientIP
		row.ClientIP = &ip
	}
	id := s.insertUsageRow(&row)

	s.chargeChannelBalance(ctx, info.ID, actualMicro)

	if s.cfg.Billing.Enabled && s.ledger != nil {
		if err := s.ledger.Charge(ctx, req.Key.UserID, attemptReqID, actualMicro); err != nil {
			s.log.Error("charge failed attempt", "err", err, "request_id", attemptReqID)
		}
	}
	s.settleReservation(ctx, req.Key.UserID, res, actualMicro)
	s.addUsageDelta(req.Key.UserID, input, 0, 0, actualMicro, false, id)
}

func (s *Service) setAffinity(ctx context.Context, uid int64, model string, channelID int64) {
	if s.affinity == nil || !s.cfg.Routing.StickyEnabled {
		return
	}
	ttl := time.Duration(s.cfg.Routing.StickyTTLSeconds) * time.Second
	if err := s.affinity.Set(ctx, uid, model, channelID, ttl); err != nil {
		s.log.Debug("set affinity failed", "err", err)
	}
}

const maxBufferedBody = 10 << 20

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

func rewriteBody(body []byte, upstreamModel string) (out []byte, stream bool, wantsUsage bool, err error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, false, false, err
	}
	m["model"] = upstreamModel
	if v, ok := m["stream"].(bool); ok && v {
		stream = true
	}
	if so, ok := m["stream_options"].(map[string]any); ok {
		if u, ok := so["include_usage"].(bool); ok && u {
			wantsUsage = true
		}
	}
	if stream && !wantsUsage {
		m["stream_options"] = map[string]any{"include_usage": true}
	}
	out, err = json.Marshal(m)
	if err != nil {
		return nil, stream, wantsUsage, err
	}
	return out, stream, wantsUsage, nil
}
