package admin

import "github.com/gin-gonic/gin"

type route struct {
	method  string
	path    string
	summary string
}

var placeholderRoutes = []route{
	{"POST", "/pricing", "设置/覆盖单价"},
	{"GET", "/pricing", "查询单价"},
	{"DELETE", "/pricing", "删除单价"},

	{"GET", "/usage-logs", "请求日志列表"},
	{"GET", "/usage-logs/:logId", "请求日志详情"},
	{"GET", "/stats/daily", "用户日汇总"},
	{"GET", "/stats/overview", "总览"},
	{"GET", "/stats/channels", "渠道成本"},
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/channels", h.createChannel)
	rg.GET("/channels", h.listChannels)
	rg.GET("/channels/:channelId", h.getChannel)
	rg.PUT("/channels/:channelId", h.updateChannel)
	rg.PUT("/channels/:channelId/status", h.setChannelStatus)
	rg.DELETE("/channels/:channelId", h.deleteChannel)
	rg.POST("/channels/:channelId/test", h.testChannel)
	rg.POST("/channels/:channelId/remote-models", h.channelRemoteModels)
	rg.POST("/remote-models", h.previewRemoteModels)
	rg.GET("/channels/:channelId/models", h.listChannelModels)
	rg.POST("/channels/:channelId/models", h.addChannelModel)
	rg.PUT("/channels/:channelId/models/:modelId", h.updateChannelModel)
	rg.DELETE("/channels/:channelId/models/:modelId", h.deleteChannelModel)
	rg.GET("/models", h.modelCatalog)

	rg.POST("/users", h.createUser)
	rg.GET("/users", h.listUsers)
	rg.GET("/users/:userId", h.getUser)
	rg.PUT("/users/:userId", h.updateUser)
	rg.PUT("/users/:userId/status", h.setUserStatus)
	rg.GET("/users/:userId/balance", h.getUserBalance)
	rg.POST("/users/:userId/recharge", h.recharge)
	rg.GET("/users/:userId/balance-transactions", h.listBalanceTransactions)
	rg.POST("/users/:userId/keys", h.createKey)
	rg.GET("/users/:userId/keys", h.listUserKeys)
	rg.PUT("/users/:userId/keys/:keyId", h.updateKey)
	rg.POST("/users/:userId/keys/:keyId/reset", h.resetKey)
	rg.DELETE("/users/:userId/keys/:keyId", h.deleteKey)
	rg.GET("/keys", h.listKeys)

	rg.POST("/rate-limits", h.createRateRule)
	rg.GET("/rate-limits", h.listRateRules)
	rg.GET("/rate-limits/:ruleId", h.getRateRule)
	rg.PUT("/rate-limits/:ruleId", h.updateRateRule)
	rg.DELETE("/rate-limits/:ruleId", h.deleteRateRule)

	notImplemented := func(summary string) gin.HandlerFunc {
		return func(c *gin.Context) {
			h.log.Warn("admin endpoint not implemented", "path", c.Request.URL.Path, "summary", summary)
			Fail(c, CodeNotImplemented, "接口未实现（骨架阶段）: "+summary)
		}
	}
	for _, r := range placeholderRoutes {
		rg.Handle(r.method, r.path, notImplemented(r.summary))
	}
}
