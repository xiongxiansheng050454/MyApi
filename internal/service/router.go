package service

import "context"

func (s *Service) routeAndForward(ctx context.Context, req *ChatCompletionRequest) error {
	// TODO:
	//  1. 按 req.Model 查 enabled channel_models 且 channel.status=1 的候选
	//  2. 按 priority/weight 负载均衡（配合 s.Breaker 熔断状态过滤）
	//  3. 冻结估算费用 -> 转发上游(替换 api key / upstream_model) -> 按 usage 结算
	//  4. 写 usage_logs、user_daily_stats、balance_transactions
	return ErrNotImplemented
}
