package admin

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"MyApi/internal/model"
)

var validTargetTypes = map[string]bool{
	model.TargetGlobal: true, model.TargetUser: true, model.TargetAPIKey: true,
	model.TargetModel: true, model.TargetChannel: true,
}

var validMetrics = map[string]bool{
	model.MetricRPM: true, model.MetricTPM: true,
	model.MetricRPD: true, model.MetricTPD: true, model.MetricConcurrency: true,
}

var validActions = map[string]bool{model.ActionReject: true, model.ActionQueue: true}

type ruleOut struct {
	ID            int64           `json:"id"`
	RuleName      string          `json:"rule_name"`
	TargetType    string          `json:"target_type"`
	TargetValue   string          `json:"target_value"`
	Metric        string          `json:"metric"`
	LimitValue    int             `json:"limit_value"`
	WindowSeconds int             `json:"window_seconds"`
	Action        string          `json:"action"`
	Priority      int             `json:"priority"`
	Enabled       bool            `json:"enabled"`
	Description   *string         `json:"description"`
	Extras        json.RawMessage `json:"extras"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func ruleToOut(r *model.RateLimitRule) ruleOut {
	out := ruleOut{
		ID: r.ID, RuleName: r.RuleName, TargetType: r.TargetType,
		TargetValue: r.TargetValue, Metric: r.Metric, LimitValue: r.LimitValue,
		WindowSeconds: r.WindowSeconds, Action: r.Action, Priority: r.Priority,
		Enabled: r.Enabled, Description: r.Description,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if len(r.Extras) == 0 {
		out.Extras = json.RawMessage("{}")
	} else {
		out.Extras = json.RawMessage(r.Extras)
	}
	return out
}

func validateRuleFields(tt, metric, action string, targetValue string, limitValue, windowSeconds int, extras json.RawMessage) string {
	if !validTargetTypes[tt] {
		return "target_type 非法"
	}
	if !validMetrics[metric] {
		return "metric 非法"
	}
	if !validActions[action] {
		return "action 非法"
	}
	if limitValue <= 0 || windowSeconds <= 0 {
		return "limit_value/window_seconds 必须 > 0"
	}
	if len(extras) > 0 {
		var m any
		if err := json.Unmarshal(extras, &m); err != nil || m == nil {
			return "extras 需为 JSON 对象"
		}
		if _, ok := m.(map[string]any); !ok {
			return "extras 需为 JSON 对象"
		}
	}
	if action == model.ActionQueue {
		timeout := 0.0
		if len(extras) > 0 {
			var e map[string]any
			_ = json.Unmarshal(extras, &e)
			if v, ok := e["queue_timeout_seconds"].(float64); ok {
				timeout = v
			}
		}
		if timeout <= 0 {
			return "queue 动作需在 extras.queue_timeout_seconds 指定 >0 的超时"
		}
	}
	if targetValue == "" {
		return "target_value 不能为空"
	}
	return ""
}

func (h *Handler) invalidateRate() {
	if h.svc != nil {
		h.svc.InvalidateRateCache()
	}
}

func (h *Handler) createRateRule(c *gin.Context) {
	var req struct {
		RuleName      string          `json:"rule_name"`
		TargetType    string          `json:"target_type"`
		TargetValue   string          `json:"target_value"`
		Metric        string          `json:"metric"`
		LimitValue    int             `json:"limit_value"`
		WindowSeconds int             `json:"window_seconds"`
		Action        string          `json:"action"`
		Priority      *int            `json:"priority"`
		Enabled       *bool           `json:"enabled"`
		Description   *string         `json:"description"`
		Extras        json.RawMessage `json:"extras"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.RuleName = strings.TrimSpace(req.RuleName)
	if req.RuleName == "" {
		Fail(c, CodeParamError, "rule_name 不能为空")
		return
	}
	req.TargetValue = strings.TrimSpace(req.TargetValue)
	if req.TargetValue == "" {
		req.TargetValue = "*"
	}
	action := req.Action
	if action == "" {
		action = model.ActionReject
	}
	if msg := validateRuleFields(req.TargetType, req.Metric, action, req.TargetValue, req.LimitValue, req.WindowSeconds, req.Extras); msg != "" {
		Fail(c, CodeParamError, msg)
		return
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	r := model.RateLimitRule{
		RuleName:      req.RuleName,
		TargetType:    req.TargetType,
		TargetValue:   req.TargetValue,
		Metric:        req.Metric,
		LimitValue:    req.LimitValue,
		WindowSeconds: req.WindowSeconds,
		Action:        action,
		Priority:      priority,
		Enabled:       enabled,
		Description:   req.Description,
		Extras:        datatypes.JSON(req.Extras),
	}
	if err := db.Create(&r).Error; err != nil {
		h.log.Error("create rate rule", "err", err)
		Fail(c, CodeInternal, "创建规则失败")
		return
	}
	h.invalidateRate()
	OK(c, ruleToOut(&r))
}

func (h *Handler) listRateRules(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.RateLimitRule{})
	if tt := c.Query("target_type"); tt != "" {
		q = q.Where("target_type = ?", tt)
	}
	if m := c.Query("metric"); m != "" {
		q = q.Where("metric = ?", m)
	}
	if en := c.Query("enabled"); en == "true" || en == "false" {
		q = q.Where("enabled = ?", en == "true")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		h.log.Error("count rate rules", "err", err)
		Fail(c, CodeInternal, "查询规则失败")
		return
	}
	var rows []model.RateLimitRule
	if err := q.Order("priority ASC").Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list rate rules", "err", err)
		Fail(c, CodeInternal, "查询规则失败")
		return
	}
	list := make([]ruleOut, 0, len(rows))
	for i := range rows {
		list = append(list, ruleToOut(&rows[i]))
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) getRateRule(c *gin.Context) {
	id, ok := idParam(c, "ruleId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	var r model.RateLimitRule
	if err := db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeRateRuleNotFound, "规则不存在")
			return
		}
		h.log.Error("get rate rule", "err", err)
		Fail(c, CodeInternal, "查询规则失败")
		return
	}
	OK(c, ruleToOut(&r))
}

func (h *Handler) updateRateRule(c *gin.Context) {
	id, ok := idParam(c, "ruleId")
	if !ok {
		return
	}
	var req struct {
		RuleName      *string         `json:"rule_name"`
		TargetType    *string         `json:"target_type"`
		TargetValue   *string         `json:"target_value"`
		Metric        *string         `json:"metric"`
		LimitValue    *int            `json:"limit_value"`
		WindowSeconds *int            `json:"window_seconds"`
		Action        *string         `json:"action"`
		Priority      *int            `json:"priority"`
		Enabled       *bool           `json:"enabled"`
		Description   *string         `json:"description"`
		Extras        json.RawMessage `json:"extras"`
	}
	if !bindJSON(c, &req) {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	var r model.RateLimitRule
	if err := db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeRateRuleNotFound, "规则不存在")
			return
		}
		h.log.Error("get rate rule for update", "err", err)
		Fail(c, CodeInternal, "查询规则失败")
		return
	}

	updates := map[string]any{}
	if req.RuleName != nil {
		name := strings.TrimSpace(*req.RuleName)
		if name == "" {
			Fail(c, CodeParamError, "rule_name 不能为空")
			return
		}
		updates["rule_name"] = name
	}
	if req.TargetType != nil {
		updates["target_type"] = *req.TargetType
	}
	if req.TargetValue != nil {
		updates["target_value"] = strings.TrimSpace(*req.TargetValue)
	}
	if req.Metric != nil {
		updates["metric"] = *req.Metric
	}
	if req.LimitValue != nil {
		updates["limit_value"] = *req.LimitValue
	}
	if req.WindowSeconds != nil {
		updates["window_seconds"] = *req.WindowSeconds
	}
	if req.Action != nil {
		updates["action"] = *req.Action
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(req.Extras) > 0 {
		updates["extras"] = datatypes.JSON(req.Extras)
	}

	// 校验合并后的最终形态
	targetType, _ := updates["target_type"].(string)
	if targetType == "" {
		targetType = r.TargetType
	}
	metric, _ := updates["metric"].(string)
	if metric == "" {
		metric = r.Metric
	}
	action, _ := updates["action"].(string)
	if action == "" {
		action = r.Action
	}
	targetValue, _ := updates["target_value"].(string)
	if targetValue == "" {
		targetValue = r.TargetValue
	}
	limitValue, _ := updates["limit_value"].(int)
	if limitValue == 0 {
		limitValue = r.LimitValue
	}
	windowSeconds, _ := updates["window_seconds"].(int)
	if windowSeconds == 0 {
		windowSeconds = r.WindowSeconds
	}
	extras := json.RawMessage(nil)
	if len(req.Extras) > 0 {
		extras = req.Extras
	} else if len(r.Extras) > 0 {
		extras = json.RawMessage(r.Extras)
	}
	if msg := validateRuleFields(targetType, metric, action, targetValue, limitValue, windowSeconds, extras); msg != "" {
		Fail(c, CodeParamError, msg)
		return
	}

	if len(updates) == 0 {
		Fail(c, CodeParamError, "没有可更新字段")
		return
	}
	if err := db.Model(&model.RateLimitRule{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		h.log.Error("update rate rule", "err", err)
		Fail(c, CodeInternal, "更新规则失败")
		return
	}
	h.invalidateRate()
	_ = db.First(&r, id).Error
	OK(c, ruleToOut(&r))
}

func (h *Handler) deleteRateRule(c *gin.Context) {
	id, ok := idParam(c, "ruleId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	res := db.Delete(&model.RateLimitRule{}, id)
	if res.Error != nil {
		h.log.Error("delete rate rule", "err", res.Error)
		Fail(c, CodeInternal, "删除规则失败")
		return
	}
	if res.RowsAffected == 0 {
		Fail(c, CodeRateRuleNotFound, "规则不存在")
		return
	}
	h.invalidateRate()
	OK(c, gin.H{"id": id})
}
