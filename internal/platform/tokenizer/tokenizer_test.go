package tokenizer

import "testing"

func TestCount(t *testing.T) {
	c := New("cl100k_base")
	if n := c.Count("gpt-4", "hello world"); n <= 0 {
		t.Fatalf("token count should be > 0, got %d", n)
	}
	if n := c.Count("unknown-model-xyz", "你好，世界"); n <= 0 {
		t.Fatalf("fallback token count should be > 0, got %d", n)
	}
	if n := c.Count("gpt-4", ""); n != 0 {
		t.Fatalf("empty text should be 0, got %d", n)
	}
}
