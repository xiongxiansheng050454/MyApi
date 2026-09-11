package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
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
			if keep, _ := t.processEvent(ev); keep {
				t.out = ev
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

func (t *sseUsageTee) processEvent(ev []byte) (keep bool, isUsageOnly bool) {
	keep = true
	hasData := false
	for _, line := range strings.Split(string(ev), "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			hasData = true
			continue
		}
		hasData = true
		var chunk sseChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			cached := 0
			if chunk.Usage.PromptTokensDetails != nil {
				cached = chunk.Usage.PromptTokensDetails.CachedTokens
			}
			t.meta.setUsage(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, cached)
			if len(chunk.Choices) == 0 {
				isUsageOnly = true
				if t.stripUsage {
					keep = false
				}
			}
		}
		for i := range chunk.Choices {
			t.meta.addOutputChars(utf8.RuneCountInString(chunk.Choices[i].Delta.Content))
		}
	}
	if hasData && keep {
		t.meta.markFirstByte()
	}
	return keep, isUsageOnly
}

func parseUsageJSON(body []byte) (in, out, cached int, outputChars int, ok bool) {
	var payload struct {
		Usage   *usagePayload `json:"usage"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0, 0, 0, false
	}
	for i := range payload.Choices {
		outputChars += utf8.RuneCountInString(payload.Choices[i].Message.Content)
	}
	if payload.Usage == nil {
		return 0, 0, 0, outputChars, false
	}
	c := 0
	if payload.Usage.PromptTokensDetails != nil {
		c = payload.Usage.PromptTokensDetails.CachedTokens
	}
	return payload.Usage.PromptTokens, payload.Usage.CompletionTokens, c, outputChars, true
}
