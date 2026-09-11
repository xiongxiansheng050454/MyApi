package service

import "testing"

func TestAffinityScope(t *testing.T) {
	s := &Service{}
	key := func(sessionID string, body string) string {
		return s.affinityScope(&ChatCompletionRequest{
			Key:       &KeyIdentity{UserID: 7},
			Model:     "gpt-4",
			SessionID: sessionID,
			Body:      []byte(body),
		})
	}

	// X-Session-Id 优先
	if got := key("sess-1", `{"user":"alice"}`); got != "affinity:7:sess-1:gpt-4" {
		t.Fatalf("session header should win, got %q", got)
	}
	// 其次请求体 user 字段
	if got := key("", `{"model":"gpt-4","user":"alice"}`); got != "affinity:7:alice:gpt-4" {
		t.Fatalf("body user should be used, got %q", got)
	}
	// 都没有 → 回退用户维度
	if got := key("", `{"model":"gpt-4"}`); got != "affinity:7:_user:gpt-4" {
		t.Fatalf("fallback should be user scope, got %q", got)
	}
	if got := key("", ``); got != "affinity:7:_user:gpt-4" {
		t.Fatalf("empty body fallback should be user scope, got %q", got)
	}
}
