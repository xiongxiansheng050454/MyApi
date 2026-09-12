package channel

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
)

type ModelItem struct {
	ID            int64     `json:"id"`
	ChannelID     int64     `json:"channel_id"`
	ModelName     string    `json:"model_name"`
	UpstreamModel string    `json:"upstream_model"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func toModelItem(m *model.ChannelModel) ModelItem {
	return ModelItem{
		ID:            m.ID,
		ChannelID:     m.ChannelID,
		ModelName:     m.ModelName,
		UpstreamModel: m.UpstreamModel,
		Enabled:       m.Enabled,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func (s *Service) channelExists(ctx context.Context, id int64) bool {
	var n int64
	_ = s.db.WithContext(ctx).Model(&model.Channel{}).Where("id = ?", id).Count(&n).Error
	return n > 0
}

func (s *Service) AddModel(ctx context.Context, channelID int64, modelName, upstreamModel string, enabled *bool) (*ModelItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	if !s.channelExists(ctx, channelID) {
		return nil, ErrNotFound
	}
	var dup int64
	_ = db.Model(&model.ChannelModel{}).Where("channel_id = ? AND model_name = ?", channelID, modelName).Count(&dup).Error
	if dup > 0 {
		return nil, ErrModelDuplicate
	}
	en := true
	if enabled != nil {
		en = *enabled
	}
	m := model.ChannelModel{
		ChannelID:     channelID,
		ModelName:     strings.TrimSpace(modelName),
		UpstreamModel: strings.TrimSpace(upstreamModel),
		Enabled:       en,
	}
	if err := db.Create(&m).Error; err != nil {
		return nil, err
	}
	s.notifyChanged(channelID)
	out := toModelItem(&m)
	return &out, nil
}

func (s *Service) ListModels(ctx context.Context, channelID int64, enabled, modelName string) ([]ModelItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Model(&model.ChannelModel{}).Where("channel_id = ?", channelID)
	if enabled == "true" || enabled == "false" {
		q = q.Where("enabled = ?", enabled == "true")
	}
	if mn := strings.TrimSpace(modelName); mn != "" {
		q = q.Where("model_name = ?", mn)
	}
	var rows []model.ChannelModel
	if err := q.Order("model_name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]ModelItem, 0, len(rows))
	for i := range rows {
		list = append(list, toModelItem(&rows[i]))
	}
	return list, nil
}

func (s *Service) UpdateModel(ctx context.Context, modelID int64, upstreamModel *string, enabled *bool) (*ModelItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	var m model.ChannelModel
	if err := db.First(&m, modelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrModelNotFound
		}
		return nil, err
	}
	updates := map[string]any{}
	if upstreamModel != nil && strings.TrimSpace(*upstreamModel) != "" {
		updates["upstream_model"] = strings.TrimSpace(*upstreamModel)
	}
	if enabled != nil {
		updates["enabled"] = *enabled
	}
	if len(updates) == 0 {
		return nil, apperr.Invalid("没有可更新字段")
	}
	if err := db.Model(&model.ChannelModel{}).Where("id = ?", modelID).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.notifyChanged(m.ChannelID)
	_ = db.First(&m, modelID).Error
	out := toModelItem(&m)
	return &out, nil
}

func (s *Service) DeleteModel(ctx context.Context, modelID int64) error {
	if s.db == nil {
		return gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	var m model.ChannelModel
	if err := db.First(&m, modelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrModelNotFound
		}
		return err
	}
	if err := db.Where("channel_id = ? AND model_name = ?", m.ChannelID, m.ModelName).
		Delete(&model.ModelPricing{}).Error; err != nil {
		return err
	}
	if err := db.Delete(&model.ChannelModel{}, modelID).Error; err != nil {
		return err
	}
	s.notifyChanged(m.ChannelID)
	return nil
}
