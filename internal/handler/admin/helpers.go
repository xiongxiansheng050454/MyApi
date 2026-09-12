package admin

import (
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"MyApi/internal/catalog"
	"MyApi/internal/channel"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/platform/httpx"
	"MyApi/internal/pricing"
	"MyApi/internal/ratelimit"
	"MyApi/internal/usage"
	"MyApi/internal/user"
)

type Handler struct {
	channels *channel.Service
	users    *user.Service
	limits   *ratelimit.Service
	pricing  *pricing.Service
	usage    *usage.Service
	catalog  *catalog.Service
	log      *slog.Logger
}

func New(
	channels *channel.Service,
	users *user.Service,
	limits *ratelimit.Service,
	pricingSvc *pricing.Service,
	usageSvc *usage.Service,
	catalogSvc *catalog.Service,
	log *slog.Logger,
) *Handler {
	return &Handler{
		channels: channels, users: users, limits: limits,
		pricing: pricingSvc, usage: usageSvc, catalog: catalogSvc, log: log,
	}
}

func parseID(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func idParam(c *gin.Context, key string) (int64, bool) {
	id, ok := httpx.IDParam(c, key)
	if !ok {
		httpx.Fail(c, CodeParamError, "路径参数 "+key+" 非法")
		return 0, false
	}
	return id, true
}

func bindJSON(c *gin.Context, out any) bool {
	if !httpx.BindJSON(c, out) {
		httpx.Fail(c, CodeBadJSON, "请求体 JSON 非法")
		return false
	}
	return true
}

func failErr(c *gin.Context, err error) {
	var inv *apperr.InvalidError
	switch {
	case errors.As(err, &inv):
		httpx.Fail(c, CodeParamError, inv.Msg)
	case errors.Is(err, gorm.ErrRecordNotFound):
		httpx.Fail(c, CodeNotFound, "记录不存在")
	case errors.Is(err, channel.ErrNotFound):
		httpx.Fail(c, CodeChNotFound, "渠道不存在")
	case errors.Is(err, channel.ErrModelNotFound):
		httpx.Fail(c, CodeChModelNotFound, "模型映射不存在")
	case errors.Is(err, channel.ErrModelDuplicate):
		httpx.Fail(c, CodeChModelDuplicate, "该渠道已存在此 model_name")
	case errors.Is(err, channel.ErrSecretMissing):
		httpx.Fail(c, CodeChSecretMissing, "未配置上游密钥加密主密钥")
	case errors.Is(err, channel.ErrNoModelMapping):
		httpx.Fail(c, CodeChModelNotFound, "该渠道无可用模型映射，请先配置并启用映射")
	case errors.Is(err, user.ErrUserNotFound):
		httpx.Fail(c, CodeUserNotFound, "用户不存在")
	case errors.Is(err, user.ErrUserSuspended):
		httpx.Fail(c, CodeUserSuspended, "用户状态非 active")
	case errors.Is(err, user.ErrUserStateOp):
		httpx.Fail(c, CodeUserStateOp, "用户状态不允许该操作")
	case errors.Is(err, user.ErrStateNotActive):
		httpx.Fail(c, CodeUserStateOp, "用户状态非 active，禁止充值")
	case errors.Is(err, user.ErrKeyNotFound):
		httpx.Fail(c, CodeKeyNotFound, "Key 不存在")
	case errors.Is(err, user.ErrKeyNameDuplicate):
		httpx.Fail(c, CodeKeyNameDuplicate, "同一用户下 key_name 已存在")
	case errors.Is(err, ratelimit.ErrRuleNotFound):
		httpx.Fail(c, CodeRateRuleNotFound, "规则不存在")
	case errors.Is(err, pricing.ErrNotFound):
		httpx.Fail(c, CodePricingNotFound, "定价记录不存在")
	case errors.Is(err, pricing.ErrModelMappingNotFound):
		httpx.Fail(c, CodePricingModelNotFound, "该渠道未发布此模型，请先添加模型映射")
	default:
		httpx.Fail(c, CodeInternal, err.Error())
	}
}

// parseTimeRange 解析 RFC3339 的 start_time/end_time 查询参数。
func parseTimeRange(c *gin.Context) (start, end *time.Time, ok bool) {
	if v := c.Query("start_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httpx.Fail(c, CodeTimeRangeInvalid, "start_time 需为 RFC3339")
			return nil, nil, false
		}
		start = &t
	}
	if v := c.Query("end_time"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httpx.Fail(c, CodeTimeRangeInvalid, "end_time 需为 RFC3339")
			return nil, nil, false
		}
		end = &t
	}
	return start, end, true
}
