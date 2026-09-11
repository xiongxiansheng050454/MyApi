package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    Server    `yaml:"server"`
	Database  Database  `yaml:"database"`
	Redis     Redis     `yaml:"redis"`
	Upstream  Upstream  `yaml:"upstream"`
	Routing   Routing   `yaml:"routing"`
	Billing   Billing   `yaml:"billing"`
	RateLimit RateLimit `yaml:"rate_limit"`
	Stats     Stats     `yaml:"stats"`
	Log       Log       `yaml:"log"`
}

type Routing struct {
	StickyEnabled           bool    `yaml:"sticky_enabled"`
	StickyTTLSeconds        int     `yaml:"sticky_ttl_seconds"`
	MaxAttemptsPerPriority  int     `yaml:"max_attempts_per_priority"`
	TryNextPriority         bool    `yaml:"try_next_priority"`
	MaxTotalAttempts        int     `yaml:"max_total_attempts"`
	MaxRetriesPerChannel    int     `yaml:"max_retries_per_channel"`
	FilterExhaustedChannels bool    `yaml:"filter_exhausted_channels"`
	LowBalanceThreshold     float64 `yaml:"low_balance_threshold"`
}

type Billing struct {
	Enabled             bool   `yaml:"enabled"`
	DefaultOutputBudget int    `yaml:"default_output_budget"`
	DefaultEncoding     string `yaml:"default_encoding"`
	BalanceTTLSeconds   int    `yaml:"balance_ttl_seconds"`
	LockTTLSeconds      int    `yaml:"lock_ttl_seconds"`
}

type RateLimit struct {
	CacheTTLSeconds int `yaml:"cache_ttl_seconds"`
	CharsPerToken   int `yaml:"chars_per_token"`
}

type Stats struct {
	FlushIntervalSeconds int    `yaml:"flush_interval_seconds"`
	Timezone             string `yaml:"timezone"`
	RedisTTLHours        int    `yaml:"redis_ttl_hours"`
}

type Server struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Mode         string `yaml:"mode"`
	DashboardDir string `yaml:"dashboard_dir"`
	DocsDir      string `yaml:"docs_dir"`
}

type Database struct {
	Enabled      bool   `yaml:"enabled"`
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	Name         string `yaml:"name"`
	SSLMode      string `yaml:"sslmode"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
	AutoMigrate  bool   `yaml:"auto_migrate"`
}

type Redis struct {
	Enabled  bool   `yaml:"enabled"`
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type Upstream struct {
	CacheTTLSeconds            int    `yaml:"cache_ttl_seconds"`
	RetireGraceSeconds         int    `yaml:"retire_grace_seconds"`
	EventsChannel              string `yaml:"events_channel"`
	BreakerMaxRequests         uint32 `yaml:"breaker_max_requests"`
	BreakerIntervalSeconds     int    `yaml:"breaker_interval_seconds"`
	BreakerTimeoutSeconds      int    `yaml:"breaker_timeout_seconds"`
	BreakerConsecutiveFailures uint32 `yaml:"breaker_consecutive_failures"`
	HealthTimeoutSeconds       int    `yaml:"health_timeout_seconds"`
	StreamIdleTimeoutSeconds   int    `yaml:"stream_idle_timeout_seconds"`
	DialTimeoutSeconds         int    `yaml:"dial_timeout_seconds"`
	TLSHandshakeTimeoutSeconds int    `yaml:"tls_handshake_timeout_seconds"`
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Default() *Config {
	return &Config{
		Server: Server{Host: "0.0.0.0", Port: 8080, Mode: "debug", DashboardDir: "dashboard", DocsDir: "docs"},
		Database: Database{
			Enabled: true, Host: "127.0.0.1", Port: 5432,
			User: "postgres", Password: "postgres", Name: "myapi",
			SSLMode: "disable", MaxOpenConns: 20, MaxIdleConns: 10, AutoMigrate: false,
		},
		Redis: Redis{Enabled: true, Addr: "127.0.0.1:6379", DB: 0},
		Upstream: Upstream{
			CacheTTLSeconds:            3,
			RetireGraceSeconds:         600,
			EventsChannel:              "myapi:channel:events",
			BreakerMaxRequests:         1,
			BreakerIntervalSeconds:     0,
			BreakerTimeoutSeconds:      30,
			BreakerConsecutiveFailures: 5,
			HealthTimeoutSeconds:       8,
			StreamIdleTimeoutSeconds:   60,
			DialTimeoutSeconds:         5,
			TLSHandshakeTimeoutSeconds: 5,
		},
		RateLimit: RateLimit{CacheTTLSeconds: 2, CharsPerToken: 4},
		Stats:     Stats{FlushIntervalSeconds: 5, Timezone: "UTC", RedisTTLHours: 168},
		Routing: Routing{
			StickyEnabled:           true,
			StickyTTLSeconds:        1800,
			MaxAttemptsPerPriority:  0,
			TryNextPriority:         true,
			MaxTotalAttempts:        0,
			MaxRetriesPerChannel:    2,
			FilterExhaustedChannels: true,
			LowBalanceThreshold:     0,
		},
		Billing: Billing{Enabled: true, DefaultOutputBudget: 1024, DefaultEncoding: "cl100k_base", BalanceTTLSeconds: 3600, LockTTLSeconds: 3},
		Log:     Log{Level: "info", Format: "text"},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, err
		}
	}
	cfg.applyEnv()
	return cfg, nil
}

func (c *Config) applyEnv() {
	c.Server.Port = envInt("MYAPI_SERVER_PORT", c.Server.Port)
	c.Server.Mode = envStr("MYAPI_SERVER_MODE", c.Server.Mode)
	c.Database.Host = envStr("MYAPI_DB_HOST", c.Database.Host)
	c.Database.Port = envInt("MYAPI_DB_PORT", c.Database.Port)
	c.Database.User = envStr("MYAPI_DB_USER", c.Database.User)
	c.Database.Password = envStr("MYAPI_DB_PASSWORD", c.Database.Password)
	c.Database.Name = envStr("MYAPI_DB_NAME", c.Database.Name)
	c.Database.SSLMode = envStr("MYAPI_DB_SSLMODE", c.Database.SSLMode)
	c.Database.AutoMigrate = envBool("MYAPI_DB_AUTO_MIGRATE", c.Database.AutoMigrate)
	c.Redis.Addr = envStr("MYAPI_REDIS_ADDR", c.Redis.Addr)
	c.Redis.Password = envStr("MYAPI_REDIS_PASSWORD", c.Redis.Password)
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
