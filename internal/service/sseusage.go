package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type usagePayload struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type sseUsageTee struct {
	src        *bufio.Reader
	meta       *usageMeta
	stripUsage bool
	out        []byte
}

func newSSEUsageTee(r io.Reader, meta *usageMeta, stripUsage bool) io.ReadCloser {
	return &sseUsageTee{src: bufio.NewReader(r), meta: meta, stripUsage: stripUsage}
}

func (t *sseUsageTee) Close() error { return nil }

func (t *sseUsageTee) Read(p []byte) (int, error) {
	for len(t.out) == 0 {
		ev, err := t.readEvent()
		if len(ev) > 0 {
			if keep, out := t.processEvent(ev); keep && len(out) > 0 {
				t.out = out
			}
		}
		if err != nil {
			if len(t.out) == 0 {
				return 0, err
			}
			break
		}
	}
	n := copy(p, t.out)
	t.out = t.out[n:]
	return n, nil
}

func (t *sseUsageTee) readEvent() ([]byte, error) {
	var buf bytes.Buffer
	for {
		line, err := t.src.ReadString('\n')
		if len(line) > 0 {
			buf.WriteString(line)
			if line == "\n" || line == "\r\n" {
				return buf.Bytes(), nil
			}
		}
		if err != nil {
			return buf.Bytes(), err
		}
	}
}

type sseChunk struct {
	Usage   *usagePayload `json:"usage"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (t *sseUsageTee) processEvent(ev []byte) (keep bool, out []byte) {
	keep = true
	hasData := false
	var buf bytes.Buffer

	lines := strings.Split(strings.TrimRight(string(ev), "\n"), "\n")
	for _, line := range lines {
		raw := strings.TrimRight(line, "\r")
		if !strings.HasPrefix(raw, "data:") {
			if raw != "" {
				buf.WriteString(raw + "\n")
			}
			continue
		}
		hasData = true
		payload := strings.TrimSpace(strings.TrimPrefix(raw, "data:"))
		if payload == "" || payload == "[DONE]" {
			buf.WriteString("data: " + payload + "\n")
			continue
		}

		var chunk sseChunk
		_ = json.Unmarshal([]byte(payload), &chunk)
		if chunk.Usage != nil {
			cached := 0
			if chunk.Usage.PromptTokensDetails != nil {
				cached = chunk.Usage.PromptTokensDetails.CachedTokens
			}
			t.meta.setUsage(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, cached)
			if len(chunk.Choices) == 0 && t.stripUsage {
				keep = false
				return false, nil
			}
		}
		for i := range chunk.Choices {
			t.meta.addOutputText(chunk.Choices[i].Delta.Content)
		}
		if t.meta.model != "" {
			var m map[string]any
			if json.Unmarshal([]byte(payload), &m) == nil {
				m["model"] = t.meta.model
				delete(m, "system_fingerprint")
				if b, err := json.Marshal(m); err == nil {
					payload = string(b)
				}
			}
		}
		buf.WriteString("data: " + payload + "\n")
	}

	if hasData {
		t.meta.markFirstByte()
	}
	buf.WriteString("\n")
	return true, buf.Bytes()
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
