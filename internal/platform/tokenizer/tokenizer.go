package tokenizer

import (
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

// Cache 按模型缓存 tiktoken 编码，未知模型回退到配置编码，再回退到朴素估算。
type Cache struct {
	mu       sync.Mutex
	codecs   map[string]tokenizer.Codec
	fallback tokenizer.Encoding
}

func New(fallbackEncoding string) *Cache {
	enc := tokenizer.Cl100kBase
	switch strings.ToLower(strings.TrimSpace(fallbackEncoding)) {
	case "o200k_base", "o200k":
		enc = tokenizer.O200kBase
	case "p50k_base", "p50k":
		enc = tokenizer.P50kBase
	case "r50k_base", "r50k":
		enc = tokenizer.R50kBase
	}
	return &Cache{codecs: map[string]tokenizer.Codec{}, fallback: enc}
}

func (t *Cache) codecFor(model string) tokenizer.Codec {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c, ok := t.codecs[model]; ok {
		return c
	}
	var c tokenizer.Codec
	if model != "" {
		if cc, err := tokenizer.ForModel(tokenizer.Model(model)); err == nil {
			c = cc
		}
	}
	if c == nil {
		if cc, err := tokenizer.Get(t.fallback); err == nil {
			c = cc
		}
	}
	t.codecs[model] = c
	return c
}

func (t *Cache) Count(model, text string) int {
	if text == "" {
		return 0
	}
	c := t.codecFor(model)
	if c == nil {
		return naiveTokens(text)
	}
	n, err := c.Count(text)
	if err != nil {
		return naiveTokens(text)
	}
	return n
}

func naiveTokens(text string) int {
	r := len([]rune(text))
	n := r / 4
	if r%4 > 0 {
		n++
	}
	return n
}
