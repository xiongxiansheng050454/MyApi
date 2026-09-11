package admin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	log *slog.Logger
}

func New(log *slog.Logger) *Handler {
	return &Handler{log: log}
}

type route struct {
	method  string
	path    string
	summary string
}

var routes = []route{
	{"POST", "/users", "创建下游用户"},
	{"GET", "/users", "用户列表"},
	{"GET", "/users/:userId", "用户详情"},
	{"PUT", "/users/:userId", "更新用户"},
	{"PUT", "/users/:userId/status", "变更用户状态"},
	{"GET", "/users/:userId/balance", "查询余额"},
	{"POST", "/users/:userId/recharge", "充值"},
	{"GET", "/users/:userId/balance-transactions", "资金流水"},
	{"GET", "/users/:userId/keys", "Key 列表"},
	{"POST", "/users/:userId/keys", "创建 Key"},
	{"PUT", "/users/:userId/keys/:keyId", "更新 Key"},
	{"POST", "/users/:userId/keys/:keyId/reset", "重置 Key"},
	{"DELETE", "/users/:userId/keys/:keyId", "删除 Key"},
	{"GET", "/keys", "Key 全局检索"},

	{"POST", "/channels", "创建上游渠道"},
	{"GET", "/channels", "渠道列表"},
	{"GET", "/channels/:channelId", "渠道详情"},
	{"PUT", "/channels/:channelId", "更新渠道"},
	{"PUT", "/channels/:channelId/status", "启停渠道"},
	{"DELETE", "/channels/:channelId", "删除渠道"},
	{"POST", "/channels/:channelId/test", "渠道连通性测试"},
	{"GET", "/channels/:channelId/models", "模型映射列表"},
	{"POST", "/channels/:channelId/models", "添加模型映射"},
	{"PUT", "/channels/:channelId/models/:modelId", "更新模型映射"},
	{"DELETE", "/channels/:channelId/models/:modelId", "删除模型映射"},
	{"GET", "/models", "对外发布模型目录"},

	{"POST", "/pricing", "设置/覆盖单价"},
	{"GET", "/pricing", "查询单价"},
	{"DELETE", "/pricing", "删除单价"},

	{"POST", "/rate-limits", "创建限流规则"},
	{"GET", "/rate-limits", "规则列表"},
	{"GET", "/rate-limits/:ruleId", "规则详情"},
	{"PUT", "/rate-limits/:ruleId", "更新规则"},
	{"DELETE", "/rate-limits/:ruleId", "删除规则"},

	{"GET", "/usage-logs", "请求日志列表"},
	{"GET", "/usage-logs/:logId", "请求日志详情"},
	{"GET", "/stats/daily", "用户日汇总"},
	{"GET", "/stats/overview", "总览"},
	{"GET", "/stats/channels", "渠道成本"},
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	notImplemented := func(summary string) gin.HandlerFunc {
		return func(c *gin.Context) {
			h.log.Warn("admin endpoint not implemented", "path", c.Request.URL.Path, "summary", summary)
			Fail(c, CodeNotImplemented, "接口未实现（骨架阶段）: "+summary)
		}
	}
	for _, r := range routes {
		rg.Handle(r.method, r.path, notImplemented(r.summary))
	}
}
