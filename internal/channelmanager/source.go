package channelmanager

import (
	"context"

	"gorm.io/gorm"

	"MyApi/internal/model"
)

type Source interface {
	Load(ctx context.Context) (*Snapshot, error)
}

type DBSource struct {
	db *gorm.DB
}

func NewDBSource(db *gorm.DB) *DBSource {
	return &DBSource{db: db}
}

func (s *DBSource) Load(ctx context.Context) (*Snapshot, error) {
	snap := newEmptySnapshot()

	var channels []model.Channel
	if err := s.db.WithContext(ctx).
		Select("id", "name", "base_url", "auth_type", "weight", "priority", "balance").
		Where("status = ?", 1).
		Find(&channels).Error; err != nil {
		return nil, err
	}
	for i := range channels {
		c := &channels[i]
		snap.Channels[c.ID] = ChannelInfo{
			ID:       c.ID,
			Name:     c.Name,
			BaseURL:  c.BaseURL,
			AuthType: c.AuthType,
			Weight:   c.Weight,
			Priority: c.Priority,
			Balance:  c.Balance,
		}
	}

	var binds []model.ChannelModel
	if err := s.db.WithContext(ctx).
		Select("channel_id", "model_name", "upstream_model").
		Where("enabled = ?", true).
		Find(&binds).Error; err != nil {
		return nil, err
	}
	for i := range binds {
		b := &binds[i]
		if _, ok := snap.Channels[b.ChannelID]; !ok {
			continue
		}
		snap.Models[b.ModelName] = append(snap.Models[b.ModelName], b.ChannelID)
		if snap.ModelBinds[b.ModelName] == nil {
			snap.ModelBinds[b.ModelName] = map[int64]string{}
		}
		snap.ModelBinds[b.ModelName][b.ChannelID] = b.UpstreamModel
	}
	return snap, nil
}
