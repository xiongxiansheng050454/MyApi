package model

func All() []interface{} {
	return []interface{}{
		&Channel{},
		&ChannelModel{},
		&ModelPricing{},
		&User{},
		&ClientApiKey{},
		&UserDailyStat{},
		&RateLimitRule{},
		&UserBalance{},
		&BalanceTransaction{},
		&UsageLog{},
	}
}
