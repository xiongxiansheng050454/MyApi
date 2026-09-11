package admin

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
)

func priceFmt(v float64) string { return strconv.FormatFloat(v, 'f', 8, 64) }

func parsePrice(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, false
	}
	return v, true
}

type pricingOut struct {
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

func pricingToOut(p *model.ModelPricing, channelName, upstreamModel string) pricingOut {
	out := pricingOut{
		ID: p.ID, ChannelID: p.ChannelID, ChannelName: channelName,
		ModelName: p.ModelName, UpstreamModel: upstreamModel,
		InputPricePer1M:  priceFmt(p.InputPricePer1M),
		OutputPricePer1M: priceFmt(p.OutputPricePer1M),
		Currency:         p.Currency,
		CreatedAt:        p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	if p.CachedInputPricePer1M != nil {
		s := priceFmt(*p.CachedInputPricePer1M)
		out.CachedInputPricePer1M = &s
	}
	return out
}

func (h *Handler) setPricing(c *gin.Context) {
	var req struct {
		ChannelID             int64   `json:"channel_id"`
		ModelName             string  `json:"model_name"`
		InputPricePer1M       string  `json:"input_price_per_1m"`
		OutputPricePer1M      string  `json:"output_price_per_1m"`
		CachedInputPricePer1M *string `json:"cached_input_price_per_1m"`
		Currency              string  `json:"currency"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.ModelName = strings.TrimSpace(req.ModelName)
	if req.ChannelID <= 0 || req.ModelName == "" {
		Fail(c, CodeParamError, "channel_id/model_name 不能为空")
		return
	}
	in, ok := parsePrice(req.InputPricePer1M)
	if !ok {
		Fail(c, CodePricingInvalid, "input_price_per_1m 必须为 >= 0 的数字")
		return
	}
	out, ok := parsePrice(req.OutputPricePer1M)
	if !ok {
		Fail(c, CodePricingInvalid, "output_price_per_1m 必须为 >= 0 的数字")
		return
	}
	var cached *float64
	if req.CachedInputPricePer1M != nil {
		v, ok := parsePrice(*req.CachedInputPricePer1M)
		if !ok {
			Fail(c, CodePricingInvalid, "cached_input_price_per_1m 必须为 >= 0 的数字")
			return
		}
		cached = &v
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "USD"
	}
	if currency != "USD" {
		Fail(c, CodePricingCurrency, "仅支持 currency=USD")
		return
	}

	db, ok := h.db(c)
	if !ok {
		return
	}
	var mapping model.ChannelModel
	if err := db.Where("channel_id = ? AND model_name = ?", req.ChannelID, req.ModelName).First(&mapping).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodePricingModelNotFound, "该渠道未发布此模型，请先添加模型映射")
			return
		}
		h.log.Error("get channel model for pricing", "err", err)
		Fail(c, CodeInternal, "查询模型映射失败")
		return
	}

	row := model.ModelPricing{
		ChannelID:             req.ChannelID,
		ModelName:             req.ModelName,
		InputPricePer1M:       in,
		OutputPricePer1M:      out,
		CachedInputPricePer1M: cached,
		Currency:              currency,
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}, {Name: "model_name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"input_price_per_1m":        in,
			"output_price_per_1m":       out,
			"cached_input_price_per_1m": cached,
			"currency":                  currency,
			"updated_at":                time.Now(),
		}),
	}).Create(&row).Error
	if err != nil {
		h.log.Error("set pricing", "err", err)
		Fail(c, CodeInternal, "保存单价失败")
		return
	}
	if err := db.Where("channel_id = ? AND model_name = ?", req.ChannelID, req.ModelName).First(&row).Error; err != nil {
		h.log.Error("reload pricing", "err", err)
		Fail(c, CodeInternal, "保存单价失败")
		return
	}
	var ch model.Channel
	_ = db.Select("name").First(&ch, req.ChannelID).Error
	OK(c, pricingToOut(&row, ch.Name, mapping.UpstreamModel))
}

func (h *Handler) listPricing(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)

	countQ := db.Table("model_pricing AS p")
	if v := c.Query("channel_id"); v != "" {
		countQ = countQ.Where("p.channel_id = ?", v)
	}
	if v := strings.TrimSpace(c.Query("model_name")); v != "" {
		countQ = countQ.Where("p.model_name LIKE ?", "%"+v+"%")
	}
	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		h.log.Error("count pricing", "err", err)
		Fail(c, CodeInternal, "查询单价失败")
		return
	}

	q := db.Table("model_pricing AS p").
		Select("p.id, p.channel_id, p.model_name, p.input_price_per_1m, p.output_price_per_1m, " +
			"p.cached_input_price_per_1m, p.currency, p.created_at, p.updated_at, " +
			"COALESCE(c.name,'') AS channel_name, COALESCE(cm.upstream_model,'') AS upstream_model").
		Joins("LEFT JOIN channels c ON c.id = p.channel_id").
		Joins("LEFT JOIN channel_models cm ON cm.channel_id = p.channel_id AND cm.model_name = p.model_name")
	if v := c.Query("channel_id"); v != "" {
		q = q.Where("p.channel_id = ?", v)
	}
	if v := strings.TrimSpace(c.Query("model_name")); v != "" {
		q = q.Where("p.model_name LIKE ?", "%"+v+"%")
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
		h.log.Error("list pricing", "err", err)
		Fail(c, CodeInternal, "查询单价失败")
		return
	}
	list := make([]pricingOut, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		p := model.ModelPricing{
			ID: r.ID, ChannelID: r.ChannelID, ModelName: r.ModelName,
			InputPricePer1M: r.InputPricePer1M, OutputPricePer1M: r.OutputPricePer1M,
			CachedInputPricePer1M: r.CachedInputPricePer1M, Currency: r.Currency,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
		list = append(list, pricingToOut(&p, r.ChannelName, r.UpstreamModel))
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) deletePricing(c *gin.Context) {
	var req struct {
		ChannelID int64  `json:"channel_id"`
		ModelName string `json:"model_name"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.ModelName = strings.TrimSpace(req.ModelName)
	if req.ChannelID <= 0 || req.ModelName == "" {
		Fail(c, CodeParamError, "channel_id/model_name 不能为空")
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	res := db.Where("channel_id = ? AND model_name = ?", req.ChannelID, req.ModelName).
		Delete(&model.ModelPricing{})
	if res.Error != nil {
		h.log.Error("delete pricing", "err", res.Error)
		Fail(c, CodeInternal, "删除单价失败")
		return
	}
	if res.RowsAffected == 0 {
		Fail(c, CodePricingNotFound, "定价记录不存在")
		return
	}
	OK(c, gin.H{"channel_id": req.ChannelID, "model_name": req.ModelName})
}
