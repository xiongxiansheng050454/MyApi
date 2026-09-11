package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"MyApi/internal/config"
	"MyApi/internal/model"
)

type Store struct {
	DB    *gorm.DB
	Redis *redis.Client
}

func Open(cfg *config.Config, log *slog.Logger) (*Store, error) {
	s := &Store{}
	if cfg.Database.Enabled {
		db, err := openDB(cfg, log)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
		s.DB = db
	}
	if cfg.Redis.Enabled {
		rdb, err := openRedis(cfg, log)
		if err != nil {
			return nil, fmt.Errorf("open redis: %w", err)
		}
		s.Redis = rdb
	}
	return s, nil
}

func (s *Store) Close() {
	if s.DB != nil {
		if sqlDB, err := s.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	if s.Redis != nil {
		_ = s.Redis.Close()
	}
}

func openDB(cfg *config.Config, log *slog.Logger) (*gorm.DB, error) {
	c := cfg.Database
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(c.MaxOpenConns)
	sqlDB.SetMaxIdleConns(c.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, err
	}
	log.Info("postgres connected", "host", c.Host, "port", c.Port, "db", c.Name)
	return db, nil
}

func openRedis(cfg *config.Config, log *slog.Logger) (*redis.Client, error) {
	c := cfg.Redis
	rdb := redis.NewClient(&redis.Options{Addr: c.Addr, Password: c.Password, DB: c.DB})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	log.Info("redis connected", "addr", c.Addr, "db", c.DB)
	return rdb, nil
}

func AutoMigrate(db *gorm.DB, log *slog.Logger) error {
	if err := db.AutoMigrate(model.All()...); err != nil {
		return err
	}
	log.Info("auto migrate finished")
	return nil
}
