package pricing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
	"MyApi/internal/platform/money"
)

var (
	ErrModelMappingNotFound = errors.New("channel model mapping not found")
	ErrNotFound             = errors.New("pricing not found")
)

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

// Lookup 按渠道与模型读取单价，cached 缺省回退为输入价。
func (s *Service) Lookup(ctx context.Context, channelID int64, modelName string) (in, out, cached float64) {
	if s.db == nil {
		return 0, 0, 0
	}
	var p model.ModelPricing
	if err := s.db.WithContext(ctx).Where("channel_id = ? AND model_name = ?", channelID, modelName).First(&p).Error; err != nil {
		return 0, 0, 0
	}
	cached = p.InputPricePer1M
	if p.CachedInputPricePer1M != nil {
		cached = *p.CachedInputPricePer1M
	}
	return p.InputPricePer1M, p.OutputPricePer1M, cached
}

// ComputeCost 为纯函数费用公式（美元）。
func ComputeCost(input, output, cached int, pIn, pOut, pCached float64) float64 {
	uncached := input - cached
	if uncached < 0 {
		uncached = 0
	}
	return (float64(uncached)*pIn + float64(cached)*pCached + float64(output)*pOut) / 1e6
}

type Item struct {
	ID                    int64     `json:"id"`
	ChannelID             int64     `json:"channel_id"`
	ChannelName           string    `json:"channel_name"`
	ModelName             string    `json:"model_name"`
	UpstreamModel         string    `json:"upstream_model"`
	InputPricePer1M       string    `json:"input_price_per_1m"`
	OutputPricePer1M      string    `json:"output_price_per_1m"`
	CachedInputPricePer1M *string   `json:"cached_input_price_per_1m"`
	Currency              string    `json:"currency"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type SetInput struct {
	ChannelID             int64
	ModelName             string
	InputPricePer1M       float64
	OutputPricePer1M      float64
	CachedInputPricePer1M *float64
	Currency              string
}

func toItem(p *model.ModelPricing, channelName, upstreamModel string) Item {
	out := Item{
		ID: p.ID, ChannelID: p.ChannelID, ChannelName: channelName,
		ModelName: p.ModelName, UpstreamModel: upstreamModel,
		InputPricePer1M:  money.Format8(p.InputPricePer1M),
		OutputPricePer1M: money.Format8(p.OutputPricePer1M),
		Currency:         p.Currency,
		CreatedAt:        p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	if p.CachedInputPricePer1M != nil {
		s := money.Format8(*p.CachedInputPricePer1M)
		out.CachedInputPricePer1M = &s
	}
	return out
}

func (s *Service) Set(ctx context.Context, in SetInput) (*Item, error) {
	if s.db == nil {
		return nil, errors.New("db unavailable")
	}
	db := s.db.WithContext(ctx)

	var mapping model.ChannelModel
	if err := db.Where("channel_id = ? AND model_name = ?", in.ChannelID, in.ModelName).First(&mapping).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrModelMappingNotFound
		}
		return nil, err
	}

	row := model.ModelPricing{
		ChannelID:             in.ChannelID,
		ModelName:             in.ModelName,
		InputPricePer1M:       in.InputPricePer1M,
		OutputPricePer1M:      in.OutputPricePer1M,
		CachedInputPricePer1M: in.CachedInputPricePer1M,
		Currency:              in.Currency,
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}, {Name: "model_name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"input_price_per_1m":        in.InputPricePer1M,
			"output_price_per_1m":       in.OutputPricePer1M,
			"cached_input_price_per_1m": in.CachedInputPricePer1M,
			"currency":                  in.Currency,
			"updated_at":                time.Now(),
		}),
	}).Create(&row).Error
	if err != nil {
		return nil, err
	}
	if err := db.Where("channel_id = ? AND model_name = ?", in.ChannelID, in.ModelName).First(&row).Error; err != nil {
		return nil, err
	}
	var ch model.Channel
	_ = db.Select("name").First(&ch, in.ChannelID).Error
	item := toItem(&row, ch.Name, mapping.UpstreamModel)
	return &item, nil
}

func (s *Service) List(ctx context.Context, channelID int64, modelName string, page, pageSize int) ([]Item, int64, error) {
	if s.db == nil {
		return nil, 0, errors.New("db unavailable")
	}
	db := s.db.WithContext(ctx)

	countQ := db.Table("model_pricing AS p")
	if channelID > 0 {
		countQ = countQ.Where("p.channel_id = ?", channelID)
	}
	if modelName != "" {
		countQ = countQ.Where("p.model_name LIKE ?", "%"+modelName+"%")
	}
	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := db.Table("model_pricing AS p").
		Select("p.id, p.channel_id, p.model_name, p.input_price_per_1m, p.output_price_per_1m, " +
			"p.cached_input_price_per_1m, p.currency, p.created_at, p.updated_at, " +
			"COALESCE(c.name,'') AS channel_name, COALESCE(cm.upstream_model,'') AS upstream_model").
		Joins("LEFT JOIN channels c ON c.id = p.channel_id").
		Joins("LEFT JOIN channel_models cm ON cm.channel_id = p.channel_id AND cm.model_name = p.model_name")
	if channelID > 0 {
		q = q.Where("p.channel_id = ?", channelID)
	}
	if modelName != "" {
		q = q.Where("p.model_name LIKE ?", "%"+modelName+"%")
	}
	var rows []struct {
		ID                    int64     `gorm:"column:id"`
		ChannelID             int64     `gorm:"column:channel_id"`
		ModelName             string    `gorm:"column:model_name"`
		InputPricePer1M       float64   `gorm:"column:input_price_per_1m"`
		OutputPricePer1M      float64   `gorm:"column:output_price_per_1m"`
		CachedInputPricePer1M *float64  `gorm:"column:cached_input_price_per_1m"`
		Currency              string    `gorm:"column:currency"`
		CreatedAt             time.Time `gorm:"column:created_at"`
		UpdatedAt             time.Time `gorm:"column:updated_at"`
		ChannelName           string    `gorm:"column:channel_name"`
		UpstreamModel         string    `gorm:"column:upstream_model"`
	}
	if err := q.Order("p.channel_id ASC").Order("p.model_name ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	list := make([]Item, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		p := model.ModelPricing{
			ID: r.ID, ChannelID: r.ChannelID, ModelName: r.ModelName,
			InputPricePer1M: r.InputPricePer1M, OutputPricePer1M: r.OutputPricePer1M,
			CachedInputPricePer1M: r.CachedInputPricePer1M, Currency: r.Currency,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
		list = append(list, toItem(&p, r.ChannelName, r.UpstreamModel))
	}
	return list, total, nil
}

func (s *Service) Delete(ctx context.Context, channelID int64, modelName string) error {
	if s.db == nil {
		return errors.New("db unavailable")
	}
	res := s.db.WithContext(ctx).Where("channel_id = ? AND model_name = ?", channelID, modelName).
		Delete(&model.ModelPricing{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
