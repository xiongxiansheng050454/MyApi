package catalog

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"

	"MyApi/internal/auth"
	"MyApi/internal/channelmanager"
)

type Service struct {
	db       *gorm.DB
	channels *channelmanager.Manager
}

func New(db *gorm.DB, channels *channelmanager.Manager) *Service {
	return &Service{db: db, channels: channels}
}

// Models 返回对下游发布的网关模型名，并按 Key 白名单过滤。
func (s *Service) Models(ident *auth.Identity) []string {
	if s.channels == nil {
		return nil
	}
	snap := s.channels.Snapshot()
	out := make([]string, 0, len(snap.Models))
	for name := range snap.Models {
		if !ident.AllowAll {
			allowed := false
			for _, m := range ident.Models {
				if m == name {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type CatalogChannel struct {
	ChannelID     int64  `json:"channel_id"`
	ChannelName   string `json:"channel_name"`
	UpstreamModel string `json:"upstream_model"`
}

type CatalogItem struct {
	ModelName    string           `json:"model_name"`
	ChannelCount int              `json:"channel_count"`
	Channels     []CatalogChannel `json:"channels"`
}

type catalogRow struct {
	ModelName     string
	UpstreamModel string
	ChannelID     int64
	ChannelName   string
}

func (s *Service) Catalog(ctx context.Context, modelName string) ([]CatalogItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Table("channel_models AS cm").
		Select("cm.model_name, cm.upstream_model, ch.id AS channel_id, ch.name AS channel_name").
		Joins("JOIN channels ch ON ch.id = cm.channel_id").
		Where("cm.enabled = ? AND ch.status = ?", true, 1)
	if mn := strings.TrimSpace(modelName); mn != "" {
		q = q.Where("cm.model_name = ?", mn)
	}
	var rows []catalogRow
	if err := q.Order("cm.model_name ASC, ch.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	index := map[string]*CatalogItem{}
	for _, r := range rows {
		item, ok := index[r.ModelName]
		if !ok {
			item = &CatalogItem{ModelName: r.ModelName}
			index[r.ModelName] = item
		}
		item.Channels = append(item.Channels, CatalogChannel{
			ChannelID:     r.ChannelID,
			ChannelName:   r.ChannelName,
			UpstreamModel: r.UpstreamModel,
		})
	}
	list := make([]CatalogItem, 0, len(index))
	for _, item := range index {
		item.ChannelCount = len(item.Channels)
		list = append(list, *item)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ModelName < list[j].ModelName })
	return list, nil
}
