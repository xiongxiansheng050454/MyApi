package admin

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"MyApi/internal/model"
)

type channelModelOut struct {
	ID            int64     `json:"id"`
	ChannelID     int64     `json:"channel_id"`
	ModelName     string    `json:"model_name"`
	UpstreamModel string    `json:"upstream_model"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func modelToOut(m *model.ChannelModel) channelModelOut {
	return channelModelOut{
		ID:            m.ID,
		ChannelID:     m.ChannelID,
		ModelName:     m.ModelName,
		UpstreamModel: m.UpstreamModel,
		Enabled:       m.Enabled,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}

func (h *Handler) channelExists(db *gorm.DB, id int64) bool {
	var n int64
	_ = db.Model(&model.Channel{}).Where("id = ?", id).Count(&n).Error
	return n > 0
}

func (h *Handler) addChannelModel(c *gin.Context) {
	channelID, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	var req struct {
		ModelName     string `json:"model_name"`
		UpstreamModel string `json:"upstream_model"`
		Enabled       *bool  `json:"enabled"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.ModelName = strings.TrimSpace(req.ModelName)
	req.UpstreamModel = strings.TrimSpace(req.UpstreamModel)
	if req.ModelName == "" || req.UpstreamModel == "" {
		Fail(c, CodeChRequiredMissing, "model_name/upstream_model 不能为空")
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	if !h.channelExists(db, channelID) {
		Fail(c, CodeChNotFound, "渠道不存在")
		return
	}
	var dup int64
	_ = db.Model(&model.ChannelModel{}).Where("channel_id = ? AND model_name = ?", channelID, req.ModelName).
		Count(&dup).Error
	if dup > 0 {
		Fail(c, CodeChModelDuplicate, "该渠道已存在此 model_name")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	m := model.ChannelModel{
		ChannelID:     channelID,
		ModelName:     req.ModelName,
		UpstreamModel: req.UpstreamModel,
		Enabled:       enabled,
	}
	if err := db.Create(&m).Error; err != nil {
		h.log.Error("create channel model", "err", err)
		Fail(c, CodeInternal, "添加模型映射失败")
		return
	}
	h.notifyChanged(channelID)
	OK(c, modelToOut(&m))
}

func (h *Handler) listChannelModels(c *gin.Context) {
	channelID, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	q := db.Model(&model.ChannelModel{}).Where("channel_id = ?", channelID)
	if s := c.Query("enabled"); s == "true" || s == "false" {
		q = q.Where("enabled = ?", s == "true")
	}
	if mn := strings.TrimSpace(c.Query("model_name")); mn != "" {
		q = q.Where("model_name = ?", mn)
	}
	var rows []model.ChannelModel
	if err := q.Order("model_name ASC").Find(&rows).Error; err != nil {
		h.log.Error("list channel models", "err", err)
		Fail(c, CodeInternal, "查询模型映射失败")
		return
	}
	list := make([]channelModelOut, 0, len(rows))
	for i := range rows {
		list = append(list, modelToOut(&rows[i]))
	}
	OK(c, gin.H{"list": list, "total": len(list)})
}

func (h *Handler) updateChannelModel(c *gin.Context) {
	modelID, ok := idParam(c, "modelId")
	if !ok {
		return
	}
	var req struct {
		UpstreamModel *string `json:"upstream_model"`
		Enabled       *bool   `json:"enabled"`
	}
	if !bindJSON(c, &req) {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	var m model.ChannelModel
	if err := db.First(&m, modelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeChModelNotFound, "模型映射不存在")
			return
		}
		h.log.Error("get channel model", "err", err)
		Fail(c, CodeInternal, "查询模型映射失败")
		return
	}
	updates := map[string]any{}
	if req.UpstreamModel != nil && strings.TrimSpace(*req.UpstreamModel) != "" {
		updates["upstream_model"] = strings.TrimSpace(*req.UpstreamModel)
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		Fail(c, CodeParamError, "没有可更新字段")
		return
	}
	if err := db.Model(&model.ChannelModel{}).Where("id = ?", modelID).Updates(updates).Error; err != nil {
		h.log.Error("update channel model", "err", err)
		Fail(c, CodeInternal, "更新模型映射失败")
		return
	}
	h.notifyChanged(m.ChannelID)

	_ = db.First(&m, modelID).Error
	OK(c, modelToOut(&m))
}

func (h *Handler) deleteChannelModel(c *gin.Context) {
	modelID, ok := idParam(c, "modelId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	var m model.ChannelModel
	if err := db.First(&m, modelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeChModelNotFound, "模型映射不存在")
			return
		}
		h.log.Error("get channel model", "err", err)
		Fail(c, CodeInternal, "查询模型映射失败")
		return
	}
	if err := db.Where("channel_id = ? AND model_name = ?", m.ChannelID, m.ModelName).
		Delete(&model.ModelPricing{}).Error; err != nil {
		h.log.Error("delete pricing on model", "err", err)
		Fail(c, CodeInternal, "删除定价失败")
		return
	}
	if err := db.Delete(&model.ChannelModel{}, modelID).Error; err != nil {
		h.log.Error("delete channel model", "err", err)
		Fail(c, CodeInternal, "删除模型映射失败")
		return
	}
	h.notifyChanged(m.ChannelID)
	OK(c, gin.H{"id": modelID})
}

type catalogChannel struct {
	ChannelID     int64  `json:"channel_id"`
	ChannelName   string `json:"channel_name"`
	UpstreamModel string `json:"upstream_model"`
}

type catalogItem struct {
	ModelName    string           `json:"model_name"`
	ChannelCount int              `json:"channel_count"`
	Channels     []catalogChannel `json:"channels"`
}

type modelCatalogRow struct {
	ModelName     string
	UpstreamModel string
	ChannelID     int64
	ChannelName   string
}

func (h *Handler) modelCatalog(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	q := db.Table("channel_models AS cm").
		Select("cm.model_name, cm.upstream_model, ch.id AS channel_id, ch.name AS channel_name").
		Joins("JOIN channels ch ON ch.id = cm.channel_id").
		Where("cm.enabled = ? AND ch.status = ?", true, 1)
	if mn := strings.TrimSpace(c.Query("model_name")); mn != "" {
		q = q.Where("cm.model_name = ?", mn)
	}
	var rows []modelCatalogRow
	if err := q.Order("cm.model_name ASC, ch.id ASC").Scan(&rows).Error; err != nil {
		h.log.Error("model catalog", "err", err)
		Fail(c, CodeInternal, "查询模型目录失败")
		return
	}
	index := map[string]*catalogItem{}
	for _, r := range rows {
		item, ok := index[r.ModelName]
		if !ok {
			item = &catalogItem{ModelName: r.ModelName}
			index[r.ModelName] = item
		}
		item.Channels = append(item.Channels, catalogChannel{
			ChannelID:     r.ChannelID,
			ChannelName:   r.ChannelName,
			UpstreamModel: r.UpstreamModel,
		})
	}
	list := make([]catalogItem, 0, len(index))
	for _, item := range index {
		item.ChannelCount = len(item.Channels)
		list = append(list, *item)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ModelName < list[j].ModelName })
	OK(c, gin.H{"list": list, "total": len(list)})
}
