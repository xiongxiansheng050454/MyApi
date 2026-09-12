package openai

import (
	"encoding/json"
	"errors"
	"strings"
)

// ParsedRequest 是从 OpenAI Chat Completions 请求体中解析出的中性字段。
type ParsedRequest struct {
	Model           string
	Stream          bool
	MaxOutputTokens int
	SessionUser     string
	InputText       string
	MessageCount    int
}

func ParseChatRequest(body []byte) (*ParsedRequest, error) {
	var m struct {
		Model               string           `json:"model"`
		Stream              bool             `json:"stream"`
		MaxTokens           int              `json:"max_tokens"`
		MaxCompletionTokens int              `json:"max_completion_tokens"`
		User                string           `json:"user"`
		Messages            []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	model := strings.TrimSpace(m.Model)
	if model == "" {
		return nil, errors.New("field 'model' is required")
	}
	pr := &ParsedRequest{
		Model:           model,
		Stream:          m.Stream,
		MaxOutputTokens: m.MaxTokens,
		SessionUser:     strings.TrimSpace(m.User),
		MessageCount:    len(m.Messages),
		InputText:       messagesText(m.Messages),
	}
	if pr.MaxOutputTokens <= 0 {
		pr.MaxOutputTokens = m.MaxCompletionTokens
	}
	return pr, nil
}

func messagesText(msgs []map[string]any) string {
	var b strings.Builder
	for _, m := range msgs {
		if role, ok := m["role"].(string); ok {
			b.WriteString(role)
			b.WriteString(": ")
		}
		if content, ok := m["content"]; ok {
			b.WriteString(contentText(content))
		}
		b.WriteString("\n")
	}
	return b.String()
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

// RewriteUpstreamBody 将请求体中的 model 改为上游真实模型名，并在流式且未要求 usage 时注入 include_usage。
func RewriteUpstreamBody(body []byte, upstreamModel string) (out []byte, stream bool, wantsUsage bool, err error) {
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
