package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

var errUpstreamStream = errors.New("upstream stream interrupted")

// StreamWriter 由下游 handler 实现（gin），service 通过它下发流式内容。
type StreamWriter interface {
	Header(status int, contentType string)
	Write(p []byte) (int, error)
	Flush()
}

type sseEvent struct {
	raw     []byte
	usage   *usagePayload
	content string
	isError bool
	isDone  bool
	hasData bool
}

type sseReader struct {
	r *bufio.Reader
}

func newSSEReader(r io.Reader) *sseReader {
	return &sseReader{r: bufio.NewReader(r)}
}

// next 读取一个完整 SSE 事件（以空行结束）。
func (s *sseReader) next() (sseEvent, error) {
	var ev sseEvent
	var buf bytes.Buffer
	for {
		line, err := s.r.ReadString('\n')
		if len(line) > 0 {
			buf.WriteString(line)
			if strings.TrimRight(line, "\r\n") == "" {
				ev.raw = buf.Bytes()
				parseSSEEvent(&ev)
				return ev, nil
			}
		}
		if err != nil {
			ev.raw = buf.Bytes()
			parseSSEEvent(&ev)
			return ev, err
		}
	}
}

func parseSSEEvent(ev *sseEvent) {
	for _, line := range strings.Split(string(ev.raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		ev.hasData = true
		if payload == "[DONE]" {
			ev.isDone = true
			continue
		}
		var chunk sseChunk
		if json.Unmarshal([]byte(payload), &chunk) == nil {
			ev.usage = chunk.Usage
			for i := range chunk.Choices {
				ev.content += chunk.Choices[i].Delta.Content
			}
		}
		var errObj struct {
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(payload), &errObj) == nil && len(errObj.Error) > 0 && string(errObj.Error) != "null" {
			ev.isError = true
		}
	}
}

// relayStream 将上游 SSE 逐事件下发给 sink。
// 返回 written 表示是否已向 sink 写出任何内容；err 非 nil 表示上游/下游失败。
func (s *Service) relayStream(
	ctx context.Context,
	body io.Reader,
	sink StreamWriter,
	meta *usageMeta,
	stripUsage bool,
	idle time.Duration,
	cancel context.CancelFunc,
) (written bool, err error) {
	reader := newSSEReader(body)

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
		ev, rerr := reader.next()
		if timer != nil && len(ev.raw) > 0 {
			timer.Reset(idle)
		}
		if ev.isError {
			return written, errUpstreamStream
		}
		if ev.isDone {
			if written {
				_, _ = sink.Write(ev.raw)
				sink.Flush()
			}
			return written, nil
		}
		if len(ev.raw) > 0 {
			if ev.usage != nil {
				cached := 0
				if ev.usage.PromptTokensDetails != nil {
					cached = ev.usage.PromptTokensDetails.CachedTokens
				}
				meta.setUsage(ev.usage.PromptTokens, ev.usage.CompletionTokens, cached)
				if ev.content == "" && stripUsage {
					// 注入的 usage-only 帧：不下发
				} else {
					if _, werr := sink.Write(rewriteEventModel(ev.raw, meta.model)); werr != nil {
						return written, werr
					}
					sink.Flush()
					written = true
				}
			} else {
				if ev.content != "" {
					meta.addOutputText(ev.content)
				}
				if _, werr := sink.Write(rewriteEventModel(ev.raw, meta.model)); werr != nil {
					return written, werr
				}
				sink.Flush()
				written = true
				if ev.hasData {
					meta.markFirstByte()
				}
			}
		}
		if rerr != nil {
			return written, rerr
		}
	}
}

// rewriteEventModel 将事件内各 data 负载的 model 改为网关别名并移除指纹。
func rewriteEventModel(raw []byte, model string) []byte {
	if model == "" {
		return raw
	}
	var buf bytes.Buffer
	for _, line := range strings.SplitAfter(string(raw), "\n") {
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trimmed, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if payload != "" && payload != "[DONE]" {
				var m map[string]any
				if json.Unmarshal([]byte(payload), &m) == nil {
					m["model"] = model
					delete(m, "system_fingerprint")
					if b, err := json.Marshal(m); err == nil {
						buf.WriteString("data: " + string(b) + "\n")
						continue
					}
				}
			}
		}
		buf.WriteString(line)
	}
	return buf.Bytes()
}

func writeStreamError(sink StreamWriter) {
	_, _ = sink.Write([]byte("data: {\"error\":{\"message\":\"upstream stream interrupted\",\"type\":\"upstream_error\",\"param\":null,\"code\":\"upstream_error\"}}\n\n"))
	_, _ = sink.Write([]byte("data: [DONE]\n\n"))
	sink.Flush()
}
