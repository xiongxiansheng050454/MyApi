package openai

import (
	"encoding/json"
	"strings"
)

// Usage 是中性化的 token 用量。
type Usage struct {
	Input  int
	Output int
	Cached int
}

type usagePayload struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type sseChunk struct {
	Usage   *usagePayload `json:"usage"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ParseUsageJSON 解析非流式响应中的 usage 与正文内容；ok=false 表示无 usage。
func ParseUsageJSON(body []byte) (Usage, string, bool) {
	var payload struct {
		Usage   *usagePayload `json:"usage"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Usage{}, "", false
	}
	var b strings.Builder
	for i := range payload.Choices {
		b.WriteString(payload.Choices[i].Message.Content)
	}
	content := b.String()
	if payload.Usage == nil {
		return Usage{}, content, false
	}
	u := Usage{
		Input:  payload.Usage.PromptTokens,
		Output: payload.Usage.CompletionTokens,
	}
	if payload.Usage.PromptTokensDetails != nil {
		u.Cached = payload.Usage.PromptTokensDetails.CachedTokens
	}
	return u, content, true
}

// RewriteResponseModel 将非流式响应体顶层 model 回写为网关别名，并移除上游指纹。
func RewriteResponseModel(body []byte, alias string) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	if _, ok := m["model"]; !ok {
		return body
	}
	m["model"] = alias
	delete(m, "system_fingerprint")
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}
