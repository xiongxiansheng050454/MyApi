package channelmanager

import "time"

type ChannelInfo struct {
	ID       int64
	Name     string
	BaseURL  string
	AuthType string
	Weight   int
	Priority int
	Balance  *float64
}

type Snapshot struct {
	Channels   map[int64]ChannelInfo
	Models     map[string][]int64
	ModelBinds map[string]map[int64]string
}

func newEmptySnapshot() *Snapshot {
	return &Snapshot{
		Channels:   map[int64]ChannelInfo{},
		Models:     map[string][]int64{},
		ModelBinds: map[string]map[int64]string{},
	}
}

type Settings struct {
	CacheTTL                   time.Duration
	RetireGrace                time.Duration
	EventsChannel              string
	BreakerMaxRequests         uint32
	BreakerInterval            time.Duration
	BreakerTimeout             time.Duration
	BreakerConsecutiveFailures uint32
	FilterExhaustedChannels    bool
	LowBalanceThreshold        float64
}

func (s Settings) cacheInterval() time.Duration {
	if s.CacheTTL > 0 {
		return s.CacheTTL
	}
	return 3 * time.Second
}

func (s Settings) retireGrace() time.Duration {
	if s.RetireGrace > 0 {
		return s.RetireGrace
	}
	return 10 * time.Minute
}

type entry struct {
	info     ChannelInfo
	retiring bool
	refs     int64
	retireAt time.Time
}

func newEntry(info ChannelInfo) *entry {
	return &entry{info: info}
}
