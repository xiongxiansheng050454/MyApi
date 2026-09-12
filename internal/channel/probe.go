package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"MyApi/internal/model"
	"MyApi/internal/secret"
)

type HealthUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type HealthResult struct {
	ModelAlias    string       `json:"model_alias"`
	UpstreamModel string       `json:"upstream_model"`
	OK            bool         `json:"ok"`
	LatencyMs     int64        `json:"latency_ms"`
	HTTPStatus    int          `json:"http_status"`
	Error         string       `json:"error"`
	Usage         *HealthUsage `json:"usage,omitempty"`
}

// Test 对渠道发起真实健康检查，返回各模型映射结果与是否为全量检查。
func (s *Service) Test(ctx context.Context, channelID int64, modelAlias *string, checkAll bool) ([]HealthResult, bool, error) {
	if s.db == nil {
		return nil, false, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	var m model.Channel
	if err := db.First(&m, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}

	var mappings []model.ChannelModel
	q := db.Model(&model.ChannelModel{}).Where("channel_id = ?", channelID).Order("id ASC")
	if modelAlias != nil {
		q = q.Where("model_name = ?", strings.TrimSpace(*modelAlias))
	} else if !checkAll {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Find(&mappings).Error; err != nil {
		return nil, false, err
	}
	if len(mappings) == 0 {
		return nil, false, ErrNoModelMapping
	}

	if s.secret == nil {
		return nil, false, ErrSecretMissing
	}
	key, err := s.secret.Decrypt(m.APIKey)
	if err != nil {
		return nil, false, ErrSecretMissing
	}
	defer secret.Zero(key)

	timeout := time.Duration(s.cfg.Upstream.HealthTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	results := make([]HealthResult, 0, len(mappings))
	for i := range mappings {
		results = append(results, runHealthProbe(ctx, m.BaseURL, string(key),
			mappings[i].ModelName, mappings[i].UpstreamModel, timeout))
	}
	all := checkAll || (len(mappings) > 1 && modelAlias == nil)
	return results, all, nil
}

func runHealthProbe(ctx context.Context, baseURL, apiKey, alias, upstreamModel string, timeout time.Duration) HealthResult {
	res := HealthResult{ModelAlias: alias, UpstreamModel: upstreamModel}

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
		res.Usage = &HealthUsage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		}
	}
	return res
}

type RemoteModel struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type RemoteResult struct {
	OK        bool          `json:"ok"`
	LatencyMs int64         `json:"latency_ms"`
	Models    []RemoteModel `json:"models"`
	Error     string        `json:"error"`
}

func (s *Service) RemoteModels(ctx context.Context, channelID int64) (*RemoteResult, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var m model.Channel
	if err := s.db.WithContext(ctx).First(&m, channelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if s.secret == nil {
		return nil, ErrSecretMissing
	}
	key, err := s.secret.Decrypt(m.APIKey)
	if err != nil {
		return nil, ErrSecretMissing
	}
	defer secret.Zero(key)
	return fetchRemoteModels(ctx, m.BaseURL, string(key)), nil
}

func (s *Service) PreviewRemoteModels(ctx context.Context, baseURL, apiKey string) *RemoteResult {
	return fetchRemoteModels(ctx, strings.TrimSpace(baseURL), strings.TrimSpace(apiKey))
}

func fetchRemoteModels(ctx context.Context, baseURL, apiKey string) *RemoteResult {
	result := &RemoteResult{Models: []RemoteModel{}}
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
		Data []RemoteModel `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		result.Error = "响应解析失败: " + err.Error()
		return result
	}
	result.OK = true
	result.Models = payload.Data
	if result.Models == nil {
		result.Models = []RemoteModel{}
	}
	return result
}
