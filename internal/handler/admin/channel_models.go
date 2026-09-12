package admin

import (
	"strings"

	"github.com/gin-gonic/gin"

	"MyApi/internal/platform/httpx"
)

func (h *Handler) AddChannelModel(c *gin.Context) {
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
	if strings.TrimSpace(req.ModelName) == "" || strings.TrimSpace(req.UpstreamModel) == "" {
		httpx.Fail(c, CodeChRequiredMissing, "model_name/upstream_model 不能为空")
		return
	}
	out, err := h.channels.AddModel(c.Request.Context(), channelID, req.ModelName, req.UpstreamModel, req.Enabled)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListChannelModels(c *gin.Context) {
	channelID, ok := idParam(c, "channelId")
	if !ok {
		return
	}
	list, err := h.channels.ListModels(c.Request.Context(), channelID, c.Query("enabled"), c.Query("model_name"))
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": len(list)})
}

func (h *Handler) UpdateChannelModel(c *gin.Context) {
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
	out, err := h.channels.UpdateModel(c.Request.Context(), modelID, req.UpstreamModel, req.Enabled)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeleteChannelModel(c *gin.Context) {
	modelID, ok := idParam(c, "modelId")
	if !ok {
		return
	}
	if err := h.channels.DeleteModel(c.Request.Context(), modelID); err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": modelID})
}

func (h *Handler) ModelCatalog(c *gin.Context) {
	list, err := h.catalog.Catalog(c.Request.Context(), c.Query("model_name"))
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": len(list)})
}
