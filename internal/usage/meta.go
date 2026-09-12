package usage

import "time"

// Meta 是一次请求的用量上下文；随响应生命周期累积，最终由网关读取并结算。
type Meta struct {
	UserID        int64
	APIKeyID      int64
	RequestID     string
	ClientIP      string
	Model         string
	ChannelID     int64
	UpstreamModel string
	StartedAt     time.Time
	StatusCode    int

	Stream bool

	EstimatedInput  int
	UsageKnown      bool
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	OutputTokensEst int
	OutputText      string
	TTFTMs          *int
	Count           func(string) int
}

func (m *Meta) SetUsage(in, out, cached int) {
	m.UsageKnown = true
	m.InputTokens = in
	m.OutputTokens = out
	m.CachedTokens = cached
}

func (m *Meta) MarkFirstByte() {
	if m.TTFTMs == nil {
		ms := int(time.Since(m.StartedAt).Milliseconds())
		m.TTFTMs = &ms
	}
}

func (m *Meta) AddOutputText(text string) {
	if text == "" || m.Count == nil {
		return
	}
	m.OutputTokensEst += m.Count(text)
}

func (m *Meta) CountText(text string) int {
	if text == "" || m.Count == nil {
		return 0
	}
	return m.Count(text)
}
