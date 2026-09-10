package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSink struct {
	buf       bytes.Buffer
	started   bool
	writes    int
	failAfter int
}

func (f *fakeSink) Header(status int, contentType string) { f.started = true }
func (f *fakeSink) Write(p []byte) (int, error) {
	f.writes++
	if f.failAfter > 0 && f.writes > f.failAfter {
		return 0, errors.New("client gone")
	}
	return f.buf.Write(p)
}
func (f *fakeSink) Flush() {}

func sseChunkOf(content string) string {
	return `data: {"model":"upstream-x","choices":[{"delta":{"content":"` + content + `"}}]}` + "\n\n"
}

const usageOnlyFrame = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":2,\"prompt_tokens_details\":{\"cached_tokens\":1}}}\n\n"
const errorFrame = "data: {\"error\":{\"message\":\"boom\",\"type\":\"server_error\"}}\n\n"

func relayMeta() *usageMeta {
	return &usageMeta{startedAt: time.Now(), model: "gpt-4", count: func(s string) int { return len(s) }}
}

func TestRelayStreamStripsUsageAndRewritesModel(t *testing.T) {
	s := &Service{}
	sink := &fakeSink{}
	meta := relayMeta()
	stream := sseChunkOf("Hel") + sseChunkOf("lo") + usageOnlyFrame + "data: [DONE]\n\n"

	written, err := s.relayStream(context.Background(), strings.NewReader(stream), sink, meta, true, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !written {
		t.Fatal("expected written=true")
	}
	out := sink.buf.String()
	if strings.Contains(out, `"usage"`) {
		t.Fatalf("usage-only frame should be stripped: %s", out)
	}
	if !strings.Contains(out, `"model":"gpt-4"`) || strings.Contains(out, "upstream-x") {
		t.Fatalf("model should be rewritten: %s", out)
	}
	if !strings.Contains(out, "[DONE]") {
		t.Fatal("DONE should be forwarded")
	}
	if !meta.usageKnown || meta.inputTokens != 9 || meta.outputTokens != 2 {
		t.Fatalf("usage not captured: %+v", meta)
	}
	if meta.outputTokensEst != 5 {
		t.Fatalf("outputTokensEst=%d want 5", meta.outputTokensEst)
	}
}

func TestRelayStreamErrorBeforeWrite(t *testing.T) {
	s := &Service{}
	sink := &fakeSink{}
	meta := relayMeta()
	written, err := s.relayStream(context.Background(), strings.NewReader(errorFrame), sink, meta, true, 0, nil)
	if !errors.Is(err, errUpstreamStream) {
		t.Fatalf("expected errUpstreamStream, got %v", err)
	}
	if written {
		t.Fatal("nothing should be written")
	}
}

func TestRelayStreamErrorAfterWrite(t *testing.T) {
	s := &Service{}
	sink := &fakeSink{}
	meta := relayMeta()
	stream := sseChunkOf("hi") + errorFrame
	written, err := s.relayStream(context.Background(), strings.NewReader(stream), sink, meta, false, 0, nil)
	if !errors.Is(err, errUpstreamStream) {
		t.Fatalf("expected errUpstreamStream, got %v", err)
	}
	if !written {
		t.Fatal("content should have been written before error")
	}
}

type blockingReader struct{ done chan struct{} }

func (b *blockingReader) Read(p []byte) (int, error) {
	<-b.done
	return 0, io.EOF
}

func TestRelayStreamIdleTimeoutCancels(t *testing.T) {
	s := &Service{}
	sink := &fakeSink{}
	meta := relayMeta()

	ctx, cancel := context.WithCancel(context.Background())
	br := &blockingReader{done: make(chan struct{})}
	var once sync.Once
	cancelOnce := func() { once.Do(func() { cancel(); close(br.done) }) }

	start := time.Now()
	_, _ = s.relayStream(ctx, br, sink, meta, true, 30*time.Millisecond, cancelOnce)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("idle timeout did not trigger promptly: %v", elapsed)
	}
	if ctx.Err() == nil {
		t.Fatal("context should be canceled by idle timeout")
	}
}

func TestRelayStreamWriteFailureStops(t *testing.T) {
	s := &Service{}
	sink := &fakeSink{failAfter: 1}
	meta := relayMeta()
	stream := sseChunkOf("a") + sseChunkOf("b") + "data: [DONE]\n\n"
	written, err := s.relayStream(context.Background(), strings.NewReader(stream), sink, meta, false, 0, nil)
	if err == nil {
		t.Fatal("expected write error")
	}
	if !written {
		t.Fatal("first write should have succeeded (written=true)")
	}
}
