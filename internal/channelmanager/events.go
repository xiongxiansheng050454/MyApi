package channelmanager

import (
	"context"
	"encoding/json"

	"github.com/go-redis/redis/v8"
)

const (
	EventDeleted = "deleted"
	EventChanged = "changed"
)

type Event struct {
	Type      string `json:"type"`
	ChannelID int64  `json:"channel_id"`
}

type Bus interface {
	Publish(ctx context.Context, ev Event) error
	Listen(ctx context.Context, handle func(Event)) error
}

type RedisBus struct {
	rdb     *redis.Client
	channel string
}

func NewRedisBus(rdb *redis.Client, channel string) *RedisBus {
	return &RedisBus{rdb: rdb, channel: channel}
}

func (b *RedisBus) Publish(ctx context.Context, ev Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return b.rdb.Publish(ctx, b.channel, payload).Err()
}

func (b *RedisBus) Listen(ctx context.Context, handle func(Event)) error {
	sub := b.rdb.Subscribe(ctx, b.channel)
	defer sub.Close()
	for {
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			return err
		}
		var ev Event
		if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
			continue
		}
		handle(ev)
	}
}
