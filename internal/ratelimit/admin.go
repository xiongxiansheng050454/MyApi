package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
)

var (
	validTargetTypes = map[string]bool{
		model.TargetGlobal: true, model.TargetUser: true, model.TargetAPIKey: true,
		model.TargetModel: true, model.TargetChannel: true,
	}
	validMetrics = map[string]bool{
		model.MetricRPM: true, model.MetricTPM: true,
		model.MetricRPD: true, model.MetricTPD: true, model.MetricConcurrency: true,
	}
	validActions = map[string]bool{model.ActionReject: true, model.ActionQueue: true}
)

var ErrRuleNotFound = errors.New("rate limit rule not found")

type RuleItem struct {
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

func toRuleItem(r *model.RateLimitRule) RuleItem {
	out := RuleItem{
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

func ValidateRule(tt, metric, action, targetValue string, limitValue, windowSeconds int, extras json.RawMessage) string {
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

type CreateRuleInput struct {
	RuleName      string
	TargetType    string
	TargetValue   string
	Metric        string
	LimitValue    int
	WindowSeconds int
	Action        string
	Priority      int
	Enabled       bool
	Description   *string
	Extras        json.RawMessage
}

func (s *Service) CreateRule(ctx context.Context, in CreateRuleInput) (*RuleItem, error) {
	if s.db == nil {
		return nil, errors.New("db unavailable")
	}
	r := model.RateLimitRule{
		RuleName:      in.RuleName,
		TargetType:    in.TargetType,
		TargetValue:   in.TargetValue,
		Metric:        in.Metric,
		LimitValue:    in.LimitValue,
		WindowSeconds: in.WindowSeconds,
		Action:        in.Action,
		Priority:      in.Priority,
		Enabled:       in.Enabled,
		Description:   in.Description,
		Extras:        datatypes.JSON(in.Extras),
	}
	if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
		return nil, err
	}
	s.Invalidate()
	out := toRuleItem(&r)
	return &out, nil
}

func (s *Service) ListRules(ctx context.Context, targetType, metric, enabled string, page, pageSize int) ([]RuleItem, int64, error) {
	if s.db == nil {
		return nil, 0, errors.New("db unavailable")
	}
	q := s.db.WithContext(ctx).Model(&model.RateLimitRule{})
	if targetType != "" {
		q = q.Where("target_type = ?", targetType)
	}
	if metric != "" {
		q = q.Where("metric = ?", metric)
	}
	if enabled == "true" || enabled == "false" {
		q = q.Where("enabled = ?", enabled == "true")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.RateLimitRule
	if err := q.Order("priority ASC").Order("id ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	list := make([]RuleItem, 0, len(rows))
	for i := range rows {
		list = append(list, toRuleItem(&rows[i]))
	}
	return list, total, nil
}

func (s *Service) GetRule(ctx context.Context, id int64) (*RuleItem, error) {
	if s.db == nil {
		return nil, errors.New("db unavailable")
	}
	var r model.RateLimitRule
	if err := s.db.WithContext(ctx).First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRuleNotFound
		}
		return nil, err
	}
	out := toRuleItem(&r)
	return &out, nil
}

type UpdateRuleInput struct {
	RuleName      *string
	TargetType    *string
	TargetValue   *string
	Metric        *string
	LimitValue    *int
	WindowSeconds *int
	Action        *string
	Priority      *int
	Enabled       *bool
	Description   *string
	Extras        json.RawMessage
}

func (s *Service) UpdateRule(ctx context.Context, id int64, in UpdateRuleInput) (*RuleItem, error) {
	if s.db == nil {
		return nil, errors.New("db unavailable")
	}
	db := s.db.WithContext(ctx)
	var r model.RateLimitRule
	if err := db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRuleNotFound
		}
		return nil, err
	}

	updates := map[string]any{}
	if in.RuleName != nil {
		updates["rule_name"] = strings.TrimSpace(*in.RuleName)
	}
	if in.TargetType != nil {
		updates["target_type"] = *in.TargetType
	}
	if in.TargetValue != nil {
		updates["target_value"] = strings.TrimSpace(*in.TargetValue)
	}
	if in.Metric != nil {
		updates["metric"] = *in.Metric
	}
	if in.LimitValue != nil {
		updates["limit_value"] = *in.LimitValue
	}
	if in.WindowSeconds != nil {
		updates["window_seconds"] = *in.WindowSeconds
	}
	if in.Action != nil {
		updates["action"] = *in.Action
	}
	if in.Priority != nil {
		updates["priority"] = *in.Priority
	}
	if in.Enabled != nil {
		updates["enabled"] = *in.Enabled
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	if len(in.Extras) > 0 {
		updates["extras"] = datatypes.JSON(in.Extras)
	}

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
	if len(in.Extras) > 0 {
		extras = in.Extras
	} else if len(r.Extras) > 0 {
		extras = json.RawMessage(r.Extras)
	}
	if msg := ValidateRule(targetType, metric, action, targetValue, limitValue, windowSeconds, extras); msg != "" {
		return nil, apperr.Invalid(msg)
	}

	if len(updates) == 0 {
		return nil, apperr.Invalid("没有可更新字段")
	}
	if err := db.Model(&model.RateLimitRule{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.Invalidate()
	if err := db.First(&r, id).Error; err != nil {
		return nil, err
	}
	out := toRuleItem(&r)
	return &out, nil
}

func (s *Service) DeleteRule(ctx context.Context, id int64) error {
	if s.db == nil {
		return errors.New("db unavailable")
	}
	res := s.db.WithContext(ctx).Delete(&model.RateLimitRule{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRuleNotFound
	}
	s.Invalidate()
	return nil
}
