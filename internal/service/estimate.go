package service

import (
	"encoding/json"
	"unicode/utf8"
)

func (s *Service) estimateTokens(body []byte) int64 {
	per := s.cfg.RateLimit.CharsPerToken
	if per <= 0 {
		per = 4
	}
	var payload struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	var chars int
	for _, m := range payload.Messages {
		chars += 3
		if content, ok := m["content"]; ok {
			chars += contentChars(content)
		}
	}
	tokens := chars / per
	if chars%per > 0 {
		tokens++
	}
	return int64(tokens)
}

func contentChars(content any) int {
	switch v := content.(type) {
	case string:
		return utf8.RuneCountInString(v)
	case []any:
		total := 0
		for _, part := range v {
			switch p := part.(type) {
			case string:
				total += utf8.RuneCountInString(p)
			case map[string]any:
				if text, ok := p["text"].(string); ok {
					total += utf8.RuneCountInString(text)
				}
			}
		}
		return total
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			return utf8.RuneCountInString(text)
		}
	}
	return 0
}
