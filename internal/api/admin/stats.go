package admin

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"MyApi/internal/model"
)

type usageLogOut struct {
	ID                   int64     `json:"id"`
	RequestID            string    `json:"request_id"`
	UserID               int64     `json:"user_id"`
	ApiKeyID             int64     `json:"api_key_id"`
	ChannelID            int64     `json:"channel_id"`
	ChannelName          string    `json:"channel_name"`
	Model                string    `json:"model"`
	UpstreamModel        *string   `json:"upstream_model"`
	InputTokens          int       `json:"input_tokens"`
	OutputTokens         int       `json:"output_tokens"`
	CachedInputTokens    int       `json:"cached_input_tokens"`
	TotalTokens          int       `json:"total_tokens"`
	UnitPriceInputPer1M  string    `json:"unit_price_input_per_1m"`
	UnitPriceOutputPer1M string    `json:"unit_price_output_per_1m"`
	TotalCost            string    `json:"total_cost"`
	DurationMs           int       `json:"duration_ms"`
	TtftMs               *int      `json:"ttft_ms"`
	Status               string    `json:"status"`
	ErrorCode            *string   `json:"error_code"`
	ClientIP             *string   `json:"client_ip"`
	CreatedAt            time.Time `json:"created_at"`
}

func usageLogToOut(r *model.UsageLog, channelNames map[int64]string) usageLogOut {
	return usageLogOut{
		ID: r.ID, RequestID: r.RequestID, UserID: r.UserID, ApiKeyID: r.ApiKeyID,
		ChannelID: r.ChannelID, ChannelName: channelNames[r.ChannelID],
		Model: r.Model, UpstreamModel: r.UpstreamModel,
		InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
		CachedInputTokens: r.CachedInputTokens, TotalTokens: r.InputTokens + r.OutputTokens,
		UnitPriceInputPer1M:  moneyFmt(r.UnitPriceInputPer1M),
		UnitPriceOutputPer1M: moneyFmt(r.UnitPriceOutputPer1M),
		TotalCost:            moneyFmt(r.TotalCost),
		DurationMs:           r.DurationMs, TtftMs: r.TtftMs,
		Status: r.Status, ErrorCode: r.ErrorCode, ClientIP: r.ClientIP,
		CreatedAt: r.CreatedAt,
	}
}

func (h *Handler) channelNameMap(db *gorm.DB, ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	var rows []model.Channel
	_ = db.Select("id", "name").Where("id IN ?", ids).Find(&rows).Error
	for i := range rows {
		out[rows[i].ID] = rows[i].Name
	}
	return out
}

func (h *Handler) listUsageLogs(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.UsageLog{})
	if v := c.Query("user_id"); v != "" {
		q = q.Where("user_id = ?", v)
	}
	if v := c.Query("api_key_id"); v != "" {
		q = q.Where("api_key_id = ?", v)
	}
	if v := c.Query("channel_id"); v != "" {
		q = q.Where("channel_id = ?", v)
	}
	if v := c.Query("model"); v != "" {
		q = q.Where("model = ?", v)
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if v := c.Query("error_code"); v != "" {
		q = q.Where("error_code = ?", v)
	}
	if v := c.Query("request_id"); v != "" {
		q = q.Where("request_id = ?", v)
	}
	if v := c.Query("start_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			Fail(c, CodeTimeRangeInvalid, "start_time 需为 RFC3339")
			return
		}
		q = q.Where("created_at >= ?", t)
	}
	if v := c.Query("end_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			Fail(c, CodeTimeRangeInvalid, "end_time 需为 RFC3339")
			return
		}
		q = q.Where("created_at <= ?", t)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		h.log.Error("count usage logs", "err", err)
		Fail(c, CodeInternal, "查询日志失败")
		return
	}
	var rows []model.UsageLog
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list usage logs", "err", err)
		Fail(c, CodeInternal, "查询日志失败")
		return
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ChannelID)
	}
	names := h.channelNameMap(db, ids)
	list := make([]usageLogOut, 0, len(rows))
	for i := range rows {
		list = append(list, usageLogToOut(&rows[i], names))
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) getUsageLog(c *gin.Context) {
	id, ok := idParam(c, "logId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	var row model.UsageLog
	if err := db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeLogNotFound, "日志不存在")
			return
		}
		h.log.Error("get usage log", "err", err)
		Fail(c, CodeInternal, "查询日志失败")
		return
	}
	names := h.channelNameMap(db, []int64{row.ChannelID})
	OK(c, usageLogToOut(&row, names))
}

func (h *Handler) statsDaily(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.UserDailyStat{})
	if v := c.Query("user_id"); v != "" {
		q = q.Where("user_id = ?", v)
	}
	if v := c.Query("date_from"); v != "" {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			Fail(c, CodeDateFormatInvalid, "date_from 需为 YYYY-MM-DD")
			return
		}
		q = q.Where("stat_date >= ?", v)
	}
	if v := c.Query("date_to"); v != "" {
		if _, err := time.Parse("2006-01-02", v); err != nil {
			Fail(c, CodeDateFormatInvalid, "date_to 需为 YYYY-MM-DD")
			return
		}
		q = q.Where("stat_date <= ?", v)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		h.log.Error("count daily stats", "err", err)
		Fail(c, CodeInternal, "查询统计失败")
		return
	}
	var rows []model.UserDailyStat
	if err := q.Order("stat_date DESC").Order("user_id ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list daily stats", "err", err)
		Fail(c, CodeInternal, "查询统计失败")
		return
	}
	list := make([]gin.H, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		list = append(list, gin.H{
			"user_id":                   r.UserID,
			"stat_date":                 r.StatDate.Format("2006-01-02"),
			"total_input_tokens":        r.TotalInputTokens,
			"total_output_tokens":       r.TotalOutputTokens,
			"total_cached_input_tokens": r.TotalCachedInputTokens,
			"total_tokens":              r.TotalTokens,
			"total_cost":                moneyFmt(r.TotalCost),
			"request_count":             r.RequestCount,
			"success_count":             r.SuccessCount,
			"error_count":               r.ErrorCount,
		})
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) statsOverview(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	q := db.Model(&model.UsageLog{})
	q, ok = applyTimeRange(c, q)
	if !ok {
		return
	}
	var agg struct {
		Requests   int64   `gorm:"column:requests"`
		Success    int64   `gorm:"column:success"`
		Errors     int64   `gorm:"column:errors"`
		Input      int64   `gorm:"column:input"`
		Output     int64   `gorm:"column:output"`
		Cost       float64 `gorm:"column:cost"`
		ActiveUser int64   `gorm:"column:active_user"`
	}
	err := q.Select(
		"COUNT(*) AS requests, " +
			"COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END),0) AS success, " +
			"COALESCE(SUM(CASE WHEN status <> 'success' THEN 1 ELSE 0 END),0) AS errors, " +
			"COALESCE(SUM(input_tokens),0) AS input, " +
			"COALESCE(SUM(output_tokens),0) AS output, " +
			"COALESCE(SUM(total_cost),0) AS cost, " +
			"COUNT(DISTINCT user_id) AS active_user").
		Scan(&agg).Error
	if err != nil {
		h.log.Error("stats overview", "err", err)
		Fail(c, CodeInternal, "查询统计失败")
		return
	}
	OK(c, gin.H{
		"request_count":       agg.Requests,
		"success_count":       agg.Success,
		"error_count":         agg.Errors,
		"total_input_tokens":  agg.Input,
		"total_output_tokens": agg.Output,
		"total_tokens":        agg.Input + agg.Output,
		"total_cost":          moneyFmt(agg.Cost),
		"active_user_count":   agg.ActiveUser,
	})
}

func (h *Handler) statsChannels(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	q := db.Table("usage_logs AS u").
		Select("u.channel_id AS channel_id, COALESCE(c.name,'') AS channel_name, " +
			"COUNT(*) AS request_count, " +
			"COALESCE(SUM(u.input_tokens),0) AS total_input_tokens, " +
			"COALESCE(SUM(u.output_tokens),0) AS total_output_tokens, " +
			"COALESCE(SUM(u.input_tokens + u.output_tokens),0) AS total_tokens, " +
			"COALESCE(SUM(u.total_cost),0) AS total_cost, " +
			"COALESCE(SUM(CASE WHEN u.status='success' THEN 1 ELSE 0 END),0) AS success_count, " +
			"COALESCE(SUM(CASE WHEN u.status<>'success' THEN 1 ELSE 0 END),0) AS error_count").
		Joins("LEFT JOIN channels c ON c.id = u.channel_id").
		Group("u.channel_id, c.name").
		Order("total_cost DESC")
	if v := c.Query("channel_id"); v != "" {
		q = q.Where("u.channel_id = ?", v)
	}
	if v := c.Query("start_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			Fail(c, CodeTimeRangeInvalid, "start_time 需为 RFC3339")
			return
		}
		q = q.Where("u.created_at >= ?", t)
	}
	if v := c.Query("end_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			Fail(c, CodeTimeRangeInvalid, "end_time 需为 RFC3339")
			return
		}
		q = q.Where("u.created_at <= ?", t)
	}
	var rows []struct {
		ChannelID         int64   `json:"channel_id"`
		ChannelName       string  `json:"channel_name"`
		RequestCount      int64   `json:"request_count"`
		TotalInputTokens  int64   `json:"total_input_tokens"`
		TotalOutputTokens int64   `json:"total_output_tokens"`
		TotalTokens       int64   `json:"total_tokens"`
		TotalCost         float64 `json:"-"`
		SuccessCount      int64   `json:"success_count"`
		ErrorCount        int64   `json:"error_count"`
	}
	if err := q.Scan(&rows).Error; err != nil {
		h.log.Error("stats channels", "err", err)
		Fail(c, CodeInternal, "查询统计失败")
		return
	}
	list := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		list = append(list, gin.H{
			"channel_id":          r.ChannelID,
			"channel_name":        r.ChannelName,
			"request_count":       r.RequestCount,
			"total_input_tokens":  r.TotalInputTokens,
			"total_output_tokens": r.TotalOutputTokens,
			"total_tokens":        r.TotalTokens,
			"total_cost":          moneyFmt(r.TotalCost),
			"success_count":       r.SuccessCount,
			"error_count":         r.ErrorCount,
		})
	}
	OK(c, gin.H{"list": list})
}

func applyTimeRange(c *gin.Context, q *gorm.DB) (*gorm.DB, bool) {
	if v := strings.TrimSpace(c.Query("start_time")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			Fail(c, CodeTimeRangeInvalid, "start_time 需为 RFC3339")
			return q, false
		}
		q = q.Where("created_at >= ?", t)
	}
	if v := strings.TrimSpace(c.Query("end_time")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			Fail(c, CodeTimeRangeInvalid, "end_time 需为 RFC3339")
			return q, false
		}
		q = q.Where("created_at <= ?", t)
	}
	return q, true
}
