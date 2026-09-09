package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/secret"
)

type healthResult struct {
	ModelAlias    string       `json:"model_alias"`
	UpstreamModel string       `json:"upstream_model"`
	OK            bool         `json:"ok"`
	LatencyMs     int64        `json:"latency_ms"`
	HTTPStatus    int          `json:"http_status"`
	Error         string       `json:"error"`
	Usage         *healthUsage `json:"usage,omitempty"`
}

type healthUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// testChannel 对渠道发起真实 POST {base}/chat/completions 健康检查：
// 固定 messages=[{role:user,content:"hi"}]、max_tokens=1，短超时，整体 recover 捕获 panic。
func (h *Handler) testChannel(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Error("channel health panic", "panic", r)
			if !c.Writer.Written() {
				Fail(c, CodeInternal, fmt.Sprintf("健康检查内部异常: %v", r))
			}
		}
	}()

	channelID, ok := idParam(c, "channelId")
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
	db, ok := h.db(c)
	if !ok {
		return
	}
	var m model.Channel
	if err := db.First(&m, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, CodeChNotFound, "渠道不存在")
			return
		}
		h.log.Error("get channel for health", "err", err)
		Fail(c, CodeInternal, "查询渠道失败")
		return
	}

	var mappings []model.ChannelModel
	q := db.Model(&model.ChannelModel{}).Where("channel_id = ?", channelID).Order("id ASC")
	if req.Model != nil {
		alias := strings.TrimSpace(*req.Model)
		if alias == "" {
			Fail(c, CodeParamError, "model 不能为空")
			return
		}
		q = q.Where("model_name = ?", alias)
	} else if !req.CheckAll {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Find(&mappings).Error; err != nil {
		h.log.Error("query channel models for health", "err", err)
		Fail(c, CodeInternal, "查询模型映射失败")
		return
	}
	if len(mappings) == 0 {
		Fail(c, CodeChModelNotFound, "该渠道无可用模型映射，请先配置并启用映射")
		return
	}

	if h.svc == nil || h.svc.Secret() == nil {
		Fail(c, CodeChSecretMissing, "未配置 "+secret.EnvKey)
		return
	}
	key, err := h.svc.Secret().Decrypt(m.APIKey)
	if err != nil {
		Fail(c, CodeChSecretMissing, "解密 api_key 失败: "+err.Error())
		return
	}
	defer secret.Zero(key)

	timeout := time.Duration(h.svc.Config().Upstream.HealthTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	results := make([]healthResult, 0, len(mappings))
	for i := range mappings {
		results = append(results, runHealthProbe(c.Request.Context(), m.BaseURL, string(key),
			mappings[i].ModelName, mappings[i].UpstreamModel, timeout))
	}

	if req.CheckAll || len(mappings) > 1 && req.Model == nil {
		OK(c, gin.H{"check_all": true, "list": results})
		return
	}
	OK(c, results[0])
}

func runHealthProbe(ctx context.Context, baseURL, apiKey, alias, upstreamModel string, timeout time.Duration) healthResult {
	res := healthResult{ModelAlias: alias, UpstreamModel: upstreamModel}

	payload := map[string]any{
		"model":      upstreamModel,
		"messages":   []any{map[string]any{"role": "user", "content": "hi"}},
		"max_tokens": 1,
		"stream":     false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	httpCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		res.Error = "构造请求失败: " + err.Error()
		return res
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	start := time.Now()
	resp, err := http.DefaultClient.Do(httpReq)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.HTTPStatus = resp.StatusCode

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	res.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !res.OK {
		res.Error = strings.TrimSpace(string(raw))
		if res.Error == "" {
			res.Error = resp.Status
		}
		return res
	}

	var parsed struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if parsed.Usage.TotalTokens > 0 {
		res.Usage = &healthUsage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		}
	}
	return res
}
