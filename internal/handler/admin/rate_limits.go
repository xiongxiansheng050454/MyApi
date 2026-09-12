package admin

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"

	"MyApi/internal/platform/httpx"
	"MyApi/internal/ratelimit"
)

func (h *Handler) CreateRateRule(c *gin.Context) {
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
		httpx.Fail(c, CodeParamError, "rule_name 不能为空")
		return
	}
	req.TargetValue = strings.TrimSpace(req.TargetValue)
	if req.TargetValue == "" {
		req.TargetValue = "*"
	}
	action := req.Action
	if action == "" {
		action = "reject"
	}
	if msg := ratelimit.ValidateRule(req.TargetType, req.Metric, action, req.TargetValue, req.LimitValue, req.WindowSeconds, req.Extras); msg != "" {
		httpx.Fail(c, CodeParamError, msg)
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
	out, err := h.limits.CreateRule(c.Request.Context(), ratelimit.CreateRuleInput{
		RuleName: req.RuleName, TargetType: req.TargetType, TargetValue: req.TargetValue,
		Metric: req.Metric, LimitValue: req.LimitValue, WindowSeconds: req.WindowSeconds,
		Action: action, Priority: priority, Enabled: enabled, Description: req.Description, Extras: req.Extras,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListRateRules(c *gin.Context) {
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.limits.ListRules(c.Request.Context(), c.Query("target_type"), c.Query("metric"), c.Query("enabled"), page, pageSize)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) GetRateRule(c *gin.Context) {
	id, ok := idParam(c, "ruleId")
	if !ok {
		return
	}
	out, err := h.limits.GetRule(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) UpdateRateRule(c *gin.Context) {
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
	out, err := h.limits.UpdateRule(c.Request.Context(), id, ratelimit.UpdateRuleInput{
		RuleName: req.RuleName, TargetType: req.TargetType, TargetValue: req.TargetValue,
		Metric: req.Metric, LimitValue: req.LimitValue, WindowSeconds: req.WindowSeconds,
		Action: req.Action, Priority: req.Priority, Enabled: req.Enabled,
		Description: req.Description, Extras: req.Extras,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeleteRateRule(c *gin.Context) {
	id, ok := idParam(c, "ruleId")
	if !ok {
		return
	}
	if err := h.limits.DeleteRule(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": id})
}
