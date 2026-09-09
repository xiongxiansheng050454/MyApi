package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/secret"
	"MyApi/internal/service"
)

type Handler struct {
	svc *service.Service
	log *slog.Logger
}

func New(svc *service.Service) *Handler {
	if svc == nil {
		return &Handler{log: slog.Default()}
	}
	return &Handler{svc: svc, log: svc.Log()}
}

func (h *Handler) db(c *gin.Context) (*gorm.DB, bool) {
	if h.svc == nil || h.svc.DB == nil {
		Fail(c, CodeInternal, "服务端数据库不可用")
		return nil, false
	}
	return h.svc.DB.WithContext(c.Request.Context()), true
}

func (h *Handler) notifyDeleted(id int64) {
	if h.svc != nil && h.svc.Channels != nil {
		h.svc.Channels.NotifyDeleted(id)
	}
}

func (h *Handler) notifyChanged(id int64) {
	if h.svc != nil && h.svc.Channels != nil {
		h.svc.Channels.NotifyChanged(id)
	}
}

func idParam(c *gin.Context, key string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil {
		Fail(c, CodeParamError, "路径参数 "+key+" 非法")
		return 0, false
	}
	return id, true
}

func bindJSON(c *gin.Context, out any) bool {
	if err := c.ShouldBindJSON(out); err != nil {
		Fail(c, CodeBadJSON, "请求体 JSON 非法: "+err.Error())
		return false
	}
	return true
}

func (h *Handler) encryptAPIKey(c *gin.Context, plain string) (string, bool) {
	if h.svc == nil || h.svc.Secret() == nil {
		Fail(c, CodeChSecretMissing, "未配置 "+secret.EnvKey+", 无法加密存储 api_key")
		return "", false
	}
	enc, err := h.svc.Secret().Encrypt(plain)
	if err != nil {
		h.log.Error("encrypt api key", "err", err)
		Fail(c, CodeInternal, "api_key 加密失败")
		return "", false
	}
	return enc, true
}

type channelReq struct {
	Name        string          `json:"name"`
	BaseURL     string          `json:"base_url"`
	APIKey      string          `json:"api_key"`
	AuthType    string          `json:"auth_type"`
	ExtraConfig json.RawMessage `json:"extra_config"`
	Status      *int            `json:"status"`
	Weight      *int            `json:"weight"`
	Priority    *int            `json:"priority"`
}

type channelOut struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	BaseURL      string         `json:"base_url"`
	APIKeyMasked string         `json:"api_key_masked"`
	AuthType     string         `json:"auth_type"`
	ExtraConfig  datatypes.JSON `json:"extra_config"`
	Status       int16          `json:"status"`
	Weight       int            `json:"weight"`
	Priority     int            `json:"priority"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	ModelCount   int64          `json:"model_count,omitempty"`
}

func maskSecret(s string) string {
	switch {
	case s == "":
		return ""
	case len(s) <= 8:
		return "****"
	default:
		return s[:3] + "****" + s[len(s)-4:]
	}
}

func channelToOut(m *model.Channel) channelOut {
	out := channelOut{
		ID:           m.ID,
		Name:         m.Name,
		BaseURL:      m.BaseURL,
		APIKeyMasked: maskSecret(m.APIKey),
		AuthType:     m.AuthType,
		ExtraConfig:  m.ExtraConfig,
		Status:       m.Status,
		Weight:       m.Weight,
		Priority:     m.Priority,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
	if len(m.ExtraConfig) == 0 {
		out.ExtraConfig = datatypes.JSON("{}")
	}
	return out
}

func (h *Handler) createChannel(c *gin.Context) {
	var req channelReq
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.APIKey) == "" {
		Fail(c, CodeChRequiredMissing, "name/base_url/api_key 不能为空")
		return
	}
	encKey, ok := h.encryptAPIKey(c, req.APIKey)
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}

	status := int16(1)
	if req.Status != nil {
		status = int16(*req.Status)
	}
	weight := 100
	if req.Weight != nil {
		weight = *req.Weight
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	authType := "bearer"
	if req.AuthType != "" {
		authType = req.AuthType
	}

	m := model.Channel{
		Name:        strings.TrimSpace(req.Name),
		BaseURL:     strings.TrimRight(strings.TrimSpace(req.BaseURL), "/"),
		APIKey:      encKey,
		AuthType:    authType,
		ExtraConfig: datatypes.JSON(req.ExtraConfig),
		Status:      status,
		Weight:      weight,
		Priority:    priority,
	}
	if err := db.Create(&m).Error; err != nil {
		h.log.Error("create channel", "err", err)
		Fail(c, CodeInternal, "创建渠道失败")
		return
	}
	h.notifyChanged(m.ID)
	OK(c, channelToOut(&m))
}

func (h *Handler) listChannels(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.Channel{})
	if s := c.Query("status"); s == "1" || s == "0" {
		q = q.Where("status = ?", s)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("id::text = ? OR name LIKE ?", kw, "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		h.log.Error("count channels", "err", err)
		Fail(c, CodeInternal, "查询渠道失败")
		return
	}
	var rows []model.Channel
	if err := q.Order("priority DESC").Order("id ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list channels", "err", err)
		Fail(c, CodeInternal, "查询渠道失败")
		return
	}
	list := make([]channelOut, 0, len(rows))
	for i := range rows {
		list = append(list, channelToOut(&rows[i]))
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) getChannel(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	var m model.Channel
	if err := db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeChNotFound, "渠道不存在")
			return
		}
		h.log.Error("get channel", "err", err)
		Fail(c, CodeInternal, "查询渠道失败")
		return
	}
	var count int64
	_ = db.Model(&model.ChannelModel{}).Where("channel_id = ?", id).Count(&count).Error
	out := channelToOut(&m)
	out.ModelCount = count
	OK(c, out)
}

func (h *Handler) updateChannel(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	var req channelReq
	if !bindJSON(c, &req) {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	if req.BaseURL != "" {
		updates["base_url"] = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	}
	if req.APIKey != "" {
		encKey, ok := h.encryptAPIKey(c, req.APIKey)
		if !ok {
			return
		}
		updates["api_key"] = encKey
	}
	if req.AuthType != "" {
		updates["auth_type"] = req.AuthType
	}
	if len(req.ExtraConfig) > 0 {
		updates["extra_config"] = datatypes.JSON(req.ExtraConfig)
	}
	if req.Status != nil {
		updates["status"] = int16(*req.Status)
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if len(updates) == 0 {
		Fail(c, CodeParamError, "没有可更新字段")
		return
	}

	res := db.Model(&model.Channel{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		h.log.Error("update channel", "err", res.Error)
		Fail(c, CodeInternal, "更新渠道失败")
		return
	}
	if res.RowsAffected == 0 {
		Fail(c, CodeChNotFound, "渠道不存在")
		return
	}
	h.notifyChanged(id)

	var m model.Channel
	_ = db.First(&m, id).Error
	OK(c, channelToOut(&m))
}

func (h *Handler) setChannelStatus(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	var body struct {
		Status int `json:"status"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if body.Status != 0 && body.Status != 1 {
		Fail(c, CodeParamError, "status 仅允许 0/1")
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	res := db.Model(&model.Channel{}).Where("id = ?", id).Update("status", int16(body.Status))
	if res.Error != nil {
		h.log.Error("set channel status", "err", res.Error)
		Fail(c, CodeInternal, "更新状态失败")
		return
	}
	if res.RowsAffected == 0 {
		Fail(c, CodeChNotFound, "渠道不存在")
		return
	}
	h.notifyChanged(id)

	var m model.Channel
	_ = db.First(&m, id).Error
	OK(c, channelToOut(&m))
}

func (h *Handler) deleteChannel(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", id).Delete(&model.ModelPricing{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", id).Delete(&model.ChannelModel{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&model.Channel{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeChNotFound, "渠道不存在")
			return
		}
		h.log.Error("delete channel", "err", err)
		Fail(c, CodeInternal, "删除渠道失败")
		return
	}
	h.notifyDeleted(id)
	OK(c, gin.H{"id": id})
}

func pageParams(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v > 0 {
		pageSize = v
		if pageSize > 100 {
			pageSize = 100
		}
	}
	return page, pageSize
}
