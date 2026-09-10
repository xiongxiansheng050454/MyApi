package service

import (
	"encoding/json"
	"strings"
)

// estimateTokens 用专业分词器估算请求输入 token（按 Chat 消息拼接 + 每条固定开销）。
func (s *Service) estimateTokens(body []byte, model string) int64 {
	var payload struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	var b strings.Builder
	for _, m := range payload.Messages {
		if role, ok := m["role"].(string); ok {
			b.WriteString(role)
			b.WriteString(": ")
		}
		if content, ok := m["content"]; ok {
			b.WriteString(contentText(content))
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return 0
	}
	n := s.tokenizer.count(model, b.String())
	n += 3 * len(payload.Messages)
	return int64(n)
}

func contentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, part := range v {
			switch p := part.(type) {
			case string:
				b.WriteString(p)
			case map[string]any:
				if text, ok := p["text"].(string); ok {
					b.WriteString(text)
				}
			}
		}
		return b.String()
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			return text
		}
	}
	return ""
}
