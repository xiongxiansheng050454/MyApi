package openai

import (
	"strings"
	"testing"
)

func TestParseChatRequest(t *testing.T) {
	body := []byte(`{"model":"gpt-4","stream":true,"max_tokens":77,"user":"s1","messages":[{"role":"user","content":"hi"}]}`)
	pr, err := ParseChatRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Model != "gpt-4" || !pr.Stream || pr.MaxOutputTokens != 77 || pr.SessionUser != "s1" || pr.MessageCount != 1 {
		t.Fatalf("unexpected parsed request: %+v", pr)
	}
}

func TestParseChatRequestMissingModel(t *testing.T) {
	if _, err := ParseChatRequest([]byte(`{}`)); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestRewriteUpstreamBody(t *testing.T) {
	out, stream, wants, err := RewriteUpstreamBody([]byte(`{"model":"gpt-4","stream":true}`), "gpt-4-0613")
	if err != nil {
		t.Fatal(err)
	}
	if !stream || wants {
		t.Fatalf("stream=%v wants=%v", stream, wants)
	}
	if !strings.Contains(string(out), `"gpt-4-0613"`) {
		t.Fatalf("model not rewritten: %s", out)
	}
	if !strings.Contains(string(out), "include_usage") {
		t.Fatalf("stream_options not injected: %s", out)
	}
}

func TestRewriteUpstreamBodyKeepsExplicitUsage(t *testing.T) {
	_, stream, wants, err := RewriteUpstreamBody(
		[]byte(`{"model":"gpt-4","stream":true,"stream_options":{"include_usage":true}}`), "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	if !stream || !wants {
		t.Fatalf("stream=%v wants=%v", stream, wants)
	}
}

func TestDecoder(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\n\n" +
		"data: [DONE]\n\n"
	dec := NewDecoder(strings.NewReader(sse))

	c1, err := dec.Next()
	if err != nil {
		t.Fatal(err)
	}
	if c1.Content != "hi" {
		t.Fatalf("content=%q", c1.Content)
	}
	c2, _ := dec.Next()
	if c2.Usage == nil || c2.Usage.Input != 1 || c2.Usage.Output != 2 {
		t.Fatalf("usage=%+v", c2.Usage)
	}
	c3, _ := dec.Next()
	if !c3.Done {
		t.Fatal("expected [DONE]")
	}
}

func TestRewriteResponseModel(t *testing.T) {
	out := RewriteResponseModel([]byte(`{"model":"up","system_fingerprint":"x"}`), "gpt-4")
	s := string(out)
	if !strings.Contains(s, `"gpt-4"`) || strings.Contains(s, "system_fingerprint") {
		t.Fatalf("unexpected body: %s", s)
	}
}
