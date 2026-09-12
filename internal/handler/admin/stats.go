package admin

import (
	"github.com/gin-gonic/gin"

	"MyApi/internal/platform/httpx"
	"MyApi/internal/usage"
)

func queryInt64(c *gin.Context, key string) int64 {
	if v := c.Query(key); v != "" {
		if parsed, err := parseID(v); err == nil {
			return parsed
		}
	}
	return 0
}

func (h *Handler) ListUsageLogs(c *gin.Context) {
	start, end, ok := parseTimeRange(c)
	if !ok {
		return
	}
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.usage.ListLogs(c.Request.Context(), usage.LogFilter{
		UserID: queryInt64(c, "user_id"), APIKeyID: queryInt64(c, "api_key_id"),
		ChannelID: queryInt64(c, "channel_id"), Model: c.Query("model"),
		Status: c.Query("status"), ErrorCode: c.Query("error_code"), RequestID: c.Query("request_id"),
		Start: start, End: end, Page: page, PageSize: pageSize,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) GetUsageLog(c *gin.Context) {
	id, ok := idParam(c, "logId")
	if !ok {
		return
	}
	item, err := h.usage.GetLog(c.Request.Context(), id)
	if err != nil {
		if err == usage.ErrLogNotFound {
			httpx.Fail(c, CodeLogNotFound, "日志不存在")
			return
		}
		failErr(c, err)
		return
	}
	httpx.OK(c, item)
}

func (h *Handler) StatsDaily(c *gin.Context) {
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.usage.StatsDaily(c.Request.Context(), usage.DailyFilter{
		UserID: queryInt64(c, "user_id"), DateFrom: c.Query("date_from"), DateTo: c.Query("date_to"),
		Page: page, PageSize: pageSize,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) StatsOverview(c *gin.Context) {
	start, end, ok := parseTimeRange(c)
	if !ok {
		return
	}
	out, err := h.usage.StatsOverview(c.Request.Context(), start, end)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) StatsChannels(c *gin.Context) {
	start, end, ok := parseTimeRange(c)
	if !ok {
		return
	}
	list, err := h.usage.StatsChannels(c.Request.Context(), queryInt64(c, "channel_id"), start, end)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list})
}
