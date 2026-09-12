package admin

import (
	"strings"

	"github.com/gin-gonic/gin"

	"MyApi/internal/platform/httpx"
	"MyApi/internal/platform/money"
	"MyApi/internal/pricing"
)

func (h *Handler) SetPricing(c *gin.Context) {
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
		httpx.Fail(c, CodeParamError, "channel_id/model_name 不能为空")
		return
	}
	in, ok := money.ParseNonNegative(req.InputPricePer1M)
	if !ok {
		httpx.Fail(c, CodePricingInvalid, "input_price_per_1m 必须为 >= 0 的数字")
		return
	}
	out, ok := money.ParseNonNegative(req.OutputPricePer1M)
	if !ok {
		httpx.Fail(c, CodePricingInvalid, "output_price_per_1m 必须为 >= 0 的数字")
		return
	}
	var cached *float64
	if req.CachedInputPricePer1M != nil {
		v, ok := money.ParseNonNegative(*req.CachedInputPricePer1M)
		if !ok {
			httpx.Fail(c, CodePricingInvalid, "cached_input_price_per_1m 必须为 >= 0 的数字")
			return
		}
		cached = &v
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "USD"
	}
	if currency != "USD" {
		httpx.Fail(c, CodePricingCurrency, "仅支持 currency=USD")
		return
	}
	item, err := h.pricing.Set(c.Request.Context(), pricing.SetInput{
		ChannelID: req.ChannelID, ModelName: req.ModelName,
		InputPricePer1M: in, OutputPricePer1M: out, CachedInputPricePer1M: cached, Currency: currency,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *Handler) ListPricing(c *gin.Context) {
	page, pageSize := httpx.PageParams(c)
	var channelID int64
	if v := c.Query("channel_id"); v != "" {
		if parsed, err := parseID(v); err == nil {
			channelID = parsed
		}
	}
	list, total, err := h.pricing.List(c.Request.Context(), channelID, c.Query("model_name"), page, pageSize)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) DeletePricing(c *gin.Context) {
	var req struct {
		ChannelID int64  `json:"channel_id"`
		ModelName string `json:"model_name"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.ModelName = strings.TrimSpace(req.ModelName)
	if req.ChannelID <= 0 || req.ModelName == "" {
		httpx.Fail(c, CodeParamError, "channel_id/model_name 不能为空")
		return
	}
	if err := h.pricing.Delete(c.Request.Context(), req.ChannelID, req.ModelName); err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"channel_id": req.ChannelID, "model_name": req.ModelName})
}
