package usage

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
	"MyApi/internal/platform/money"
)

type dailyAggregator interface {
	Apply(ctx context.Context, date string, per map[int64]usageDelta, maxID int64) error
}

type gormDaily struct {
	db *gorm.DB
}

func (g *gormDaily) Apply(ctx context.Context, date string, per map[int64]usageDelta, maxID int64) error {
	statDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for uid, d := range per {
			row := model.UserDailyStat{
				UserID:                 uid,
				StatDate:               statDate,
				TotalInputTokens:       d.Input,
				TotalOutputTokens:      d.Output,
				TotalCachedInputTokens: d.Cached,
				TotalTokens:            d.Input + d.Output,
				TotalCost:              money.MicroToAmount(d.CostMicro),
				RequestCount:           int(d.Req),
				SuccessCount:           int(d.OK),
				ErrorCount:             int(d.Err),
				LastProcessedID:        maxID,
			}
			err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}, {Name: "stat_date"}},
				DoUpdates: clause.Assignments(map[string]any{
					"total_input_tokens":        gorm.Expr("user_daily_stats.total_input_tokens + ?", d.Input),
					"total_output_tokens":       gorm.Expr("user_daily_stats.total_output_tokens + ?", d.Output),
					"total_cached_input_tokens": gorm.Expr("user_daily_stats.total_cached_input_tokens + ?", d.Cached),
					"total_tokens":              gorm.Expr("user_daily_stats.total_tokens + ?", d.Input+d.Output),
					"total_cost":                gorm.Expr("user_daily_stats.total_cost + ?", money.MicroToAmount(d.CostMicro)),
					"request_count":             gorm.Expr("user_daily_stats.request_count + ?", int(d.Req)),
					"success_count":             gorm.Expr("user_daily_stats.success_count + ?", int(d.OK)),
					"error_count":               gorm.Expr("user_daily_stats.error_count + ?", int(d.Err)),
					"last_processed_id":         gorm.Expr("GREATEST(user_daily_stats.last_processed_id, ?)", maxID),
					"updated_at":                time.Now(),
				}),
			}).Create(&row).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}
