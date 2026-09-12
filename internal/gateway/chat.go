package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"MyApi/internal/auth"
	"MyApi/internal/billing"
	"MyApi/internal/channelmanager"
	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/platform/httpx"
	"MyApi/internal/platform/money"
	"MyApi/internal/pricing"
	protocol "MyApi/internal/protocol/openai"
	"MyApi/internal/upstream"
	"MyApi/internal/usage"
)

var errUpstreamStream = errors.New("upstream stream interrupted")

// Chat 处理非流式请求：限流 → 路由 → 转发 → 结算。
func (g *Gateway) Chat(ctx context.Context, ident *auth.Identity, parsed *protocol.ParsedRequest, body []byte, meta RequestMeta) (*Response, error) {
	if !allowed(ident, parsed.Model) {
		return nil, apperr.ErrModelNotFound
	}
	release, err := g.limits.Gate(ctx, ident, parsed.Model, g.tokenCounter(parsed))
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}

	if !g.routing.HasModel(parsed.Model) {
		return nil, apperr.ErrModelNotFound
	}
	session := sessionOf(meta, parsed)
	ordered := g.routing.Order(ctx, parsed.Model, ident.UserID, session)
	if len(ordered) == 0 {
		return nil, apperr.ErrNoHealthyUpstream
	}

	inputTokens := g.countInputTokens(parsed)
	budget := g.outputBudget(parsed)
	retries := g.cfg.Routing.MaxRetriesPerChannel
	if retries < 0 {
		retries = 0
	}
	totalLimit := g.cfg.Routing.MaxTotalAttempts
	totalAttempts := 0

	for _, info := range ordered {
		upstreamName, ok := g.routing.UpstreamModel(parsed.Model, info.ID)
		if !ok {
			continue
		}
		for attempt := 0; attempt <= retries; attempt++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if totalLimit > 0 && totalAttempts >= totalLimit {
				return nil, apperr.ErrNoHealthyUpstream
			}
			totalAttempts++
			attemptReqID := attemptRequestID(meta.RequestID, totalAttempts)

			handle, err := g.routing.Acquire(info.ID)
			if err != nil {
				break
			}
			res, err := g.billing.Reserve(ctx, ident.UserID, info.ID, parsed.Model, int(inputTokens), budget)
			if err != nil {
				handle.Done()
				return nil, err
			}

			resp, sent, retryable, ferr := g.forwardOnce(ctx, ident, parsed, body, meta, handle, info, upstreamName, res)
			if ferr == nil {
				g.routing.SetAffinity(ctx, ident.UserID, session, parsed.Model, info.ID)
				return resp, nil
			}
			handle.Done()
			if sent {
				g.settleFailedAttempt(ctx, ident, parsed, info, upstreamName, attemptReqID, meta.StartedAt, res)
			} else {
				g.billing.Release(ctx, ident.UserID, res)
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !retryable {
				return nil, ferr
			}
			g.log.Warn("forward attempt failed", "channel_id", info.ID, "channel", info.Name, "attempt", attempt+1, "err", ferr)
		}
	}
	return nil, apperr.ErrNoHealthyUpstream
}

func (g *Gateway) forwardOnce(
	ctx context.Context,
	ident *auth.Identity,
	parsed *protocol.ParsedRequest,
	body []byte,
	meta RequestMeta,
	handle *channelmanager.Handle,
	info channelmanager.ChannelInfo,
	upstreamName string,
	res billing.Reservation,
) (*Response, bool, bool, error) {
	upBody, _, _, err := protocol.RewriteUpstreamBody(body, upstreamName)
	if err != nil {
		return nil, false, false, err
	}
	upReq := &upstream.Request{
		Model: parsed.Model, UpstreamModel: upstreamName, Body: upBody,
		UserID: ident.UserID, APIKeyID: ident.ApiKeyID, RequestID: meta.RequestID,
		ClientIP: meta.ClientIP, StartedAt: meta.StartedAt, SessionID: meta.SessionID,
	}
	conn, sent, retryable, err := g.upstream.Open(ctx, upReq, handle, info)
	if err != nil {
		return nil, sent, retryable, err
	}

	full, rerr := io.ReadAll(io.LimitReader(conn.Body, maxBufferedBody))
	_ = conn.Body.Close()
	if rerr != nil {
		return nil, true, false, fmt.Errorf("read upstream body: %w", rerr)
	}

	um := g.newMeta(ident, parsed, meta, info, upstreamName, res)
	if u, content, ok := protocol.ParseUsageJSON(full); ok {
		um.SetUsage(u.Input, u.Output, u.Cached)
	} else {
		um.OutputText = content
	}
	um.MarkFirstByte()
	um.StatusCode = conn.StatusCode
	full = protocol.RewriteResponseModel(full, parsed.Model)
	g.finalize(ctx, um, res)

	return &Response{
		StatusCode:  conn.StatusCode,
		ContentType: conn.ContentType,
		Body:        io.NopCloser(bytes.NewReader(full)),
	}, true, false, nil
}

// ChatStream 处理流式请求：逐候选尝试，未向下游写出前可同渠道重试；
// 已写出后失败则补发 error 事件终止；客户端断开即取消上游。
func (g *Gateway) ChatStream(ctx context.Context, ident *auth.Identity, parsed *protocol.ParsedRequest, body []byte, meta RequestMeta, sink httpx.StreamWriter) error {
	if !allowed(ident, parsed.Model) {
		return apperr.ErrModelNotFound
	}
	release, err := g.limits.Gate(ctx, ident, parsed.Model, g.tokenCounter(parsed))
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}

	if !g.routing.HasModel(parsed.Model) {
		return apperr.ErrModelNotFound
	}
	session := sessionOf(meta, parsed)
	ordered := g.routing.Order(ctx, parsed.Model, ident.UserID, session)
	if len(ordered) == 0 {
		return apperr.ErrNoHealthyUpstream
	}

	inputTokens := g.countInputTokens(parsed)
	budget := g.outputBudget(parsed)
	retries := g.cfg.Routing.MaxRetriesPerChannel
	if retries < 0 {
		retries = 0
	}
	totalLimit := g.cfg.Routing.MaxTotalAttempts
	totalAttempts := 0
	idle := time.Duration(g.cfg.Upstream.StreamIdleTimeoutSeconds) * time.Second

	for _, info := range ordered {
		upstreamName, ok := g.routing.UpstreamModel(parsed.Model, info.ID)
		if !ok {
			continue
		}
		for attempt := 0; attempt <= retries; attempt++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if totalLimit > 0 && totalAttempts >= totalLimit {
				return apperr.ErrNoHealthyUpstream
			}
			totalAttempts++
			attemptReqID := attemptRequestID(meta.RequestID, totalAttempts)

			handle, err := g.routing.Acquire(info.ID)
			if err != nil {
				break
			}
			res, err := g.billing.Reserve(ctx, ident.UserID, info.ID, parsed.Model, int(inputTokens), budget)
			if err != nil {
				handle.Done()
				return err
			}

			upBody, stream, wantsUsage, rerr := protocol.RewriteUpstreamBody(body, upstreamName)
			if rerr != nil {
				handle.Done()
				g.billing.Release(ctx, ident.UserID, res)
				return rerr
			}
			injectedUsage := stream && !wantsUsage
			upReq := &upstream.Request{
				Model: parsed.Model, UpstreamModel: upstreamName, Body: upBody,
				UserID: ident.UserID, APIKeyID: ident.ApiKeyID, RequestID: meta.RequestID,
				ClientIP: meta.ClientIP, StartedAt: meta.StartedAt, SessionID: meta.SessionID,
			}

			upCtx, cancel := context.WithCancel(ctx)
			conn, sent, retryable, oerr := g.upstream.Open(upCtx, upReq, handle, info)
			if oerr != nil {
				cancel()
				handle.Done()
				if sent {
					g.settleFailedAttempt(ctx, ident, parsed, info, upstreamName, attemptReqID, meta.StartedAt, res)
				} else {
					g.billing.Release(ctx, ident.UserID, res)
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if !retryable {
					return oerr
				}
				continue
			}

			um := g.newMeta(ident, parsed, meta, info, upstreamName, res)
			sink.Header(conn.StatusCode, conn.ContentType)

			if conn.StatusCode >= 400 {
				full, _ := io.ReadAll(io.LimitReader(conn.Body, maxBufferedBody))
				_ = conn.Body.Close()
				_, _ = sink.Write(full)
				sink.Flush()
				cancel()
				handle.Done()
				g.finalize(ctx, um, res)
				return nil
			}

			watchDone := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					cancel()
				case <-watchDone:
				}
			}()

			written, relayErr := g.relayStream(upCtx, conn.Body, sink, um, injectedUsage, idle, cancel)
			close(watchDone)
			_ = conn.Body.Close()
			cancel()
			handle.Done()

			if relayErr == nil {
				g.finalize(ctx, um, res)
				g.routing.SetAffinity(ctx, ident.UserID, session, parsed.Model, info.ID)
				return nil
			}
			if written {
				g.finalize(ctx, um, res)
				protocol.WriteStreamError(sink)
				return nil
			}
			g.settleFailedAttempt(ctx, ident, parsed, info, upstreamName, attemptReqID, meta.StartedAt, res)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			g.log.Warn("stream attempt failed", "channel_id", info.ID, "attempt", attempt+1, "err", relayErr)
		}
	}
	return apperr.ErrNoHealthyUpstream
}

func (g *Gateway) relayStream(
	ctx context.Context,
	body io.Reader,
	sink httpx.StreamWriter,
	um *usage.Meta,
	stripUsage bool,
	idle time.Duration,
	cancel context.CancelFunc,
) (written bool, err error) {
	dec := protocol.NewDecoder(body)

	var timer *time.Timer
	if idle > 0 {
		timer = time.AfterFunc(idle, func() {
			if cancel != nil {
				cancel()
			}
		})
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		if ctx.Err() != nil {
			return written, ctx.Err()
		}
		ev, rerr := dec.Next()
		if timer != nil && len(ev.Raw) > 0 {
			timer.Reset(idle)
		}
		if ev.IsError {
			return written, errUpstreamStream
		}
		if ev.Done {
			if written {
				_, _ = sink.Write(ev.Raw)
				sink.Flush()
			}
			return written, nil
		}
		if len(ev.Raw) > 0 {
			if ev.Usage != nil {
				um.SetUsage(ev.Usage.Input, ev.Usage.Output, ev.Usage.Cached)
				if ev.Content == "" && stripUsage {
					// 注入的 usage-only 帧：不下发
				} else {
					if _, werr := sink.Write(protocol.RewriteEventModel(ev.Raw, um.Model)); werr != nil {
						return written, werr
					}
					sink.Flush()
					written = true
				}
			} else {
				if ev.Content != "" {
					um.AddOutputText(ev.Content)
				}
				if _, werr := sink.Write(protocol.RewriteEventModel(ev.Raw, um.Model)); werr != nil {
					return written, werr
				}
				sink.Flush()
				written = true
				if ev.HasData {
					um.MarkFirstByte()
				}
			}
		}
		if rerr != nil {
			return written, rerr
		}
	}
}

func (g *Gateway) newMeta(ident *auth.Identity, parsed *protocol.ParsedRequest, meta RequestMeta, info channelmanager.ChannelInfo, upstreamName string, res billing.Reservation) *usage.Meta {
	return &usage.Meta{
		UserID:         ident.UserID,
		APIKeyID:       ident.ApiKeyID,
		RequestID:      meta.RequestID,
		ClientIP:       meta.ClientIP,
		Model:          parsed.Model,
		ChannelID:      info.ID,
		UpstreamModel:  upstreamName,
		StartedAt:      meta.StartedAt,
		Stream:         parsed.Stream,
		EstimatedInput: int(g.countInputTokens(parsed)),
		Count:          func(text string) int { return g.tokenizer.Count(parsed.Model, text) },
	}
}

// finalize 记录用量明细、扣减渠道与用户余额，并完成 Redis 预扣的多退少补。
func (g *Gateway) finalize(ctx context.Context, um *usage.Meta, res billing.Reservation) {
	if um == nil {
		return
	}
	input, output, cached := um.InputTokens, um.OutputTokens, um.CachedTokens
	if !um.UsageKnown {
		input = um.EstimatedInput
		cached = 0
		if um.Stream {
			output = um.OutputTokensEst
		} else {
			output = um.CountText(um.OutputText)
		}
	}
	if input < 0 {
		input = 0
	}
	if output < 0 {
		output = 0
	}

	pIn, pOut, pCached := g.pricing.Lookup(ctx, um.ChannelID, um.Model)
	cost := pricing.ComputeCost(input, output, cached, pIn, pOut, pCached)
	actualMicro := money.RoundMicro(cost)

	status := model.StatusSuccess
	var errorCode *string
	if um.StatusCode < 200 || um.StatusCode >= 300 {
		status = model.StatusError
		ec := "upstream_error"
		errorCode = &ec
	}

	id := g.usage.Record(usage.RecordInput{
		RequestID:     um.RequestID,
		UserID:        um.UserID,
		APIKeyID:      um.APIKeyID,
		ChannelID:     um.ChannelID,
		Model:         um.Model,
		UpstreamModel: um.UpstreamModel,
		Input:         input,
		Output:        output,
		Cached:        cached,
		PriceIn:       pIn,
		PriceOut:      pOut,
		Cost:          cost,
		DurationMs:    int(time.Since(um.StartedAt).Milliseconds()),
		TTFTMs:        um.TTFTMs,
		Status:        status,
		ErrorCode:     errorCode,
		ClientIP:      um.ClientIP,
	})
	g.upstream.ChargeChannelBalance(ctx, um.ChannelID, actualMicro)
	g.billing.LedgerCharge(ctx, um.UserID, um.RequestID, actualMicro)
	g.billing.Settle(ctx, um.UserID, res, actualMicro)
	g.usage.AddDelta(um.UserID, input, output, cached, actualMicro, status == model.StatusSuccess, id)
}

// settleFailedAttempt 场景2：已发送上游但未获得响应 → 按输入 token 计费。
func (g *Gateway) settleFailedAttempt(ctx context.Context, ident *auth.Identity, parsed *protocol.ParsedRequest, info channelmanager.ChannelInfo, upstreamName, attemptReqID string, startedAt time.Time, res billing.Reservation) {
	input := int(g.countInputTokens(parsed))
	pIn, pOut, _ := g.pricing.Lookup(ctx, info.ID, parsed.Model)
	cost := pricing.ComputeCost(input, 0, 0, pIn, pOut, pIn)
	actualMicro := money.RoundMicro(cost)

	ec := "upstream_error"
	id := g.usage.Record(usage.RecordInput{
		RequestID:     attemptReqID,
		UserID:        ident.UserID,
		APIKeyID:      ident.ApiKeyID,
		ChannelID:     info.ID,
		Model:         parsed.Model,
		UpstreamModel: upstreamName,
		Input:         input,
		Output:        0,
		PriceIn:       pIn,
		PriceOut:      pOut,
		Cost:          cost,
		DurationMs:    int(time.Since(startedAt).Milliseconds()),
		Status:        model.StatusError,
		ErrorCode:     &ec,
	})
	g.upstream.ChargeChannelBalance(ctx, info.ID, actualMicro)
	g.billing.LedgerCharge(ctx, ident.UserID, attemptReqID, actualMicro)
	g.billing.Settle(ctx, ident.UserID, res, actualMicro)
	g.usage.AddDelta(ident.UserID, input, 0, 0, actualMicro, false, id)
}

func attemptRequestID(base string, n int) string {
	if n <= 1 {
		return base
	}
	return fmt.Sprintf("%s#a%d", base, n)
}
