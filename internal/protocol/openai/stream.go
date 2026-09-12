package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"MyApi/internal/platform/httpx"
)

// Chunk 是解码后的单个 SSE 事件（中性）。
type Chunk struct {
	Raw     []byte
	Usage   *Usage
	Content string
	Done    bool
	IsError bool
	HasData bool
}

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// Next 读取一个完整 SSE 事件（以空行结束）。
func (d *Decoder) Next() (Chunk, error) {
	var ev Chunk
	var buf bytes.Buffer
	for {
		line, err := d.r.ReadString('\n')
		if len(line) > 0 {
			buf.WriteString(line)
			if strings.TrimRight(line, "\r\n") == "" {
				ev.Raw = buf.Bytes()
				parseChunk(&ev)
				return ev, nil
			}
		}
		if err != nil {
			ev.Raw = buf.Bytes()
			parseChunk(&ev)
			return ev, err
		}
	}
}

func parseChunk(ev *Chunk) {
	for _, line := range strings.Split(string(ev.Raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		ev.HasData = true
		if payload == "[DONE]" {
			ev.Done = true
			continue
		}
		var chunk sseChunk
		if json.Unmarshal([]byte(payload), &chunk) == nil {
			if chunk.Usage != nil {
				u := Usage{
					Input:  chunk.Usage.PromptTokens,
					Output: chunk.Usage.CompletionTokens,
				}
				if chunk.Usage.PromptTokensDetails != nil {
					u.Cached = chunk.Usage.PromptTokensDetails.CachedTokens
				}
				ev.Usage = &u
			}
			for i := range chunk.Choices {
				ev.Content += chunk.Choices[i].Delta.Content
			}
		}
		var errObj struct {
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(payload), &errObj) == nil && len(errObj.Error) > 0 && string(errObj.Error) != "null" {
			ev.IsError = true
		}
	}
}

// RewriteEventModel 将事件内各 data 负载的 model 改为网关别名并移除指纹。
func RewriteEventModel(raw []byte, model string) []byte {
	if model == "" {
		return raw
	}
	var buf bytes.Buffer
	for _, line := range strings.SplitAfter(string(raw), "\n") {
		trimmed := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trimmed, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if payload != "" && payload != "[DONE]" {
				var m map[string]any
				if json.Unmarshal([]byte(payload), &m) == nil {
					m["model"] = model
					delete(m, "system_fingerprint")
					if b, err := json.Marshal(m); err == nil {
						buf.WriteString("data: " + string(b) + "\n")
						continue
					}
				}
			}
		}
		buf.WriteString(line)
	}
	return buf.Bytes()
}

func WriteStreamError(w httpx.StreamWriter) {
	_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"upstream stream interrupted\",\"type\":\"upstream_error\",\"param\":null,\"code\":\"upstream_error\"}}\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	w.Flush()
}
