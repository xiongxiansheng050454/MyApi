package service

import (
	"encoding/json"
	"strings"
)

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

// rewriteResponseModel 将非流式响应体的顶层 model 回写为网关别名，并移除上游指纹。
func rewriteResponseModel(body []byte, alias string) []byte {
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

func parseUsageJSON(body []byte) (in, out, cached int, content string, ok bool) {
	var payload struct {
		Usage   *usagePayload `json:"usage"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0, 0, "", false
	}
	var b strings.Builder
	for i := range payload.Choices {
		b.WriteString(payload.Choices[i].Message.Content)
	}
	content = b.String()
	if payload.Usage == nil {
		return 0, 0, 0, content, false
	}
	c := 0
	if payload.Usage.PromptTokensDetails != nil {
		c = payload.Usage.PromptTokensDetails.CachedTokens
	}
	return payload.Usage.PromptTokens, payload.Usage.CompletionTokens, c, content, true
}
