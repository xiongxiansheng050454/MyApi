package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/secret"
)

type remoteModel struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type remoteModelsResult struct {
	OK        bool          `json:"ok"`
	LatencyMs int64         `json:"latency_ms"`
	Models    []remoteModel `json:"models"`
	Error     string        `json:"error"`
}

func (h *Handler) previewRemoteModels(c *gin.Context) {
	var req struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if !bindJSON(c, &req) {
		return
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.BaseURL == "" || req.APIKey == "" {
		Fail(c, CodeParamError, "base_url/api_key 不能为空")
		return
	}
	// 预览密钥不入库，仅本次调用使用，返回后由 GC 回收。
	res := fetchRemoteModels(c.Request.Context(), req.BaseURL, req.APIKey)
	OK(c, res)
}

func (h *Handler) channelRemoteModels(c *gin.Context) {
	channelID, ok := idParam(c, "channelId")
	if !ok {
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
		h.log.Error("get channel for remote models", "err", err)
		Fail(c, CodeInternal, "查询渠道失败")
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

	res := fetchRemoteModels(c.Request.Context(), m.BaseURL, string(key))
	OK(c, res)
}

func fetchRemoteModels(ctx context.Context, baseURL, apiKey string) remoteModelsResult {
	result := remoteModelsResult{Models: []remoteModel{}}
	httpCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = "构造请求失败: " + err.Error()
		return result
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		result.Error = "读取响应失败: " + err.Error()
		return result
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = strings.TrimSpace(string(body))
		if result.Error == "" {
			result.Error = resp.Status
		}
		return result
	}
	var payload struct {
		Data []remoteModel `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		result.Error = "响应解析失败: " + err.Error()
		return result
	}
	result.OK = true
	result.Models = payload.Data
	if result.Models == nil {
		result.Models = []remoteModel{}
	}
	return result
}
