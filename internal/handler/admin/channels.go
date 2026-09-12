package admin

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"

	"MyApi/internal/channel"
	"MyApi/internal/platform/httpx"
)

func (h *Handler) CreateChannel(c *gin.Context) {
	var req struct {
		Name        string          `json:"name"`
		BaseURL     string          `json:"base_url"`
		APIKey      string          `json:"api_key"`
		AuthType    string          `json:"auth_type"`
		ExtraConfig json.RawMessage `json:"extra_config"`
		Status      *int            `json:"status"`
		Weight      *int            `json:"weight"`
		Priority    *int            `json:"priority"`
		Balance     *string         `json:"balance"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.APIKey) == "" {
		httpx.Fail(c, CodeChRequiredMissing, "name/base_url/api_key 不能为空")
		return
	}
	out, err := h.channels.Create(c.Request.Context(), channel.CreateInput{
		Name: req.Name, BaseURL: req.BaseURL, APIKey: req.APIKey, AuthType: req.AuthType,
		ExtraConfig: req.ExtraConfig, Status: req.Status, Weight: req.Weight,
		Priority: req.Priority, Balance: req.Balance,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListChannels(c *gin.Context) {
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.channels.List(c.Request.Context(), c.Query("status"), c.Query("keyword"), page, pageSize)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) GetChannel(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	out, err := h.channels.Get(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) UpdateChannel(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	var req struct {
		Name        string          `json:"name"`
		BaseURL     string          `json:"base_url"`
		APIKey      string          `json:"api_key"`
		AuthType    string          `json:"auth_type"`
		ExtraConfig json.RawMessage `json:"extra_config"`
		Status      *int            `json:"status"`
		Weight      *int            `json:"weight"`
		Priority    *int            `json:"priority"`
		Balance     *string         `json:"balance"`
	}
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.channels.Update(c.Request.Context(), id, channel.UpdateInput{
		Name: req.Name, BaseURL: req.BaseURL, APIKey: req.APIKey, AuthType: req.AuthType,
		ExtraConfig: req.ExtraConfig, Status: req.Status, Weight: req.Weight,
		Priority: req.Priority, Balance: req.Balance,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) SetChannelStatus(c *gin.Context) {
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
		httpx.Fail(c, CodeParamError, "status 仅允许 0/1")
		return
	}
	out, err := h.channels.SetStatus(c.Request.Context(), id, body.Status)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) SetChannelBalance(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	var req struct {
		Balance     *string `json:"balance"`
		Delta       *string `json:"delta"`
		Description *string `json:"description"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Balance == nil && req.Delta == nil {
		httpx.Fail(c, CodeParamError, "需提供 balance(覆盖) 或 delta(增减)")
		return
	}
	out, err := h.channels.SetBalance(c.Request.Context(), id, req.Balance, req.Delta, req.Description)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeleteChannel(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	if err := h.channels.Delete(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": id})
}

func (h *Handler) TestChannel(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	var req struct {
		Model    *string `json:"model"`
		CheckAll bool    `json:"check_all"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Model != nil && strings.TrimSpace(*req.Model) == "" {
		httpx.Fail(c, CodeParamError, "model 不能为空")
		return
	}
	results, all, err := h.channels.Test(c.Request.Context(), id, req.Model, req.CheckAll)
	if err != nil {
		failErr(c, err)
		return
	}
	if all {
		httpx.OK(c, gin.H{"check_all": true, "list": results})
		return
	}
	httpx.OK(c, results[0])
}

func (h *Handler) ChannelRemoteModels(c *gin.Context) {
	id, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	res, err := h.channels.RemoteModels(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *Handler) PreviewRemoteModels(c *gin.Context) {
	var req struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.APIKey) == "" {
		httpx.Fail(c, CodeParamError, "base_url/api_key 不能为空")
		return
	}
	httpx.OK(c, h.channels.PreviewRemoteModels(c.Request.Context(), req.BaseURL, req.APIKey))
}
