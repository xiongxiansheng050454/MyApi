package router

import (
	"github.com/gin-gonic/gin"

	"MyApi/internal/handler/admin"
	openaihandler "MyApi/internal/handler/openai"
)

// Deps 是所有 HTTP handler 的集合，由组合根装配后交给 Register。
type Deps struct {
	OpenAI *openaihandler.Handler
	Admin  *admin.Handler
}

// Register 在一处集中注册全部路由。
func Register(engine *gin.Engine, d Deps) {
	v1 := engine.Group("/v1")
	{
		v1.POST("/chat/completions", d.OpenAI.ChatCompletions)
		v1.GET("/models", d.OpenAI.Models)
	}

	a := engine.Group("/admin")
	{
		// 渠道
		a.POST("/channels", d.Admin.CreateChannel)
		a.GET("/channels", d.Admin.ListChannels)
		a.GET("/channels/:channelId", d.Admin.GetChannel)
		a.PUT("/channels/:channelId", d.Admin.UpdateChannel)
		a.PUT("/channels/:channelId/status", d.Admin.SetChannelStatus)
		a.PUT("/channels/:channelId/balance", d.Admin.SetChannelBalance)
		a.DELETE("/channels/:channelId", d.Admin.DeleteChannel)
		a.POST("/channels/:channelId/test", d.Admin.TestChannel)
		a.POST("/channels/:channelId/remote-models", d.Admin.ChannelRemoteModels)
		a.POST("/remote-models", d.Admin.PreviewRemoteModels)
		a.GET("/channels/:channelId/models", d.Admin.ListChannelModels)
		a.POST("/channels/:channelId/models", d.Admin.AddChannelModel)
		a.PUT("/channels/:channelId/models/:modelId", d.Admin.UpdateChannelModel)
		a.DELETE("/channels/:channelId/models/:modelId", d.Admin.DeleteChannelModel)
		a.GET("/models", d.Admin.ModelCatalog)

		// 用户与 Key
		a.POST("/users", d.Admin.CreateUser)
		a.GET("/users", d.Admin.ListUsers)
		a.GET("/users/:userId", d.Admin.GetUser)
		a.PUT("/users/:userId", d.Admin.UpdateUser)
		a.PUT("/users/:userId/status", d.Admin.SetUserStatus)
		a.DELETE("/users/:userId", d.Admin.DeleteUser)
		a.GET("/users/:userId/balance", d.Admin.GetUserBalance)
		a.POST("/users/:userId/recharge", d.Admin.Recharge)
		a.GET("/users/:userId/balance-transactions", d.Admin.ListBalanceTransactions)
		a.POST("/users/:userId/keys", d.Admin.CreateKey)
		a.GET("/users/:userId/keys", d.Admin.ListUserKeys)
		a.PUT("/users/:userId/keys/:keyId", d.Admin.UpdateKey)
		a.POST("/users/:userId/keys/:keyId/reset", d.Admin.ResetKey)
		a.DELETE("/users/:userId/keys/:keyId", d.Admin.DeleteKey)
		a.GET("/keys", d.Admin.ListKeys)

		// 限流规则
		a.POST("/rate-limits", d.Admin.CreateRateRule)
		a.GET("/rate-limits", d.Admin.ListRateRules)
		a.GET("/rate-limits/:ruleId", d.Admin.GetRateRule)
		a.PUT("/rate-limits/:ruleId", d.Admin.UpdateRateRule)
		a.DELETE("/rate-limits/:ruleId", d.Admin.DeleteRateRule)

		// 用量与统计
		a.GET("/usage-logs", d.Admin.ListUsageLogs)
		a.GET("/usage-logs/:logId", d.Admin.GetUsageLog)
		a.GET("/stats/daily", d.Admin.StatsDaily)
		a.GET("/stats/overview", d.Admin.StatsOverview)
		a.GET("/stats/channels", d.Admin.StatsChannels)

		// 定价
		a.POST("/pricing", d.Admin.SetPricing)
		a.GET("/pricing", d.Admin.ListPricing)
		a.DELETE("/pricing", d.Admin.DeletePricing)
	}
}
