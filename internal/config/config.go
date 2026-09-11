package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   Server   `yaml:"server"`
	Database Database `yaml:"database"`
	Redis    Redis    `yaml:"redis"`
	Upstream Upstream `yaml:"upstream"`
	Log      Log      `yaml:"log"`
}

type Server struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
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
	MaxForwardAttempts         int    `yaml:"max_forward_attempts"`
	HealthTimeoutSeconds       int    `yaml:"health_timeout_seconds"`
	DialTimeoutSeconds         int    `yaml:"dial_timeout_seconds"`
	TLSHandshakeTimeoutSeconds int    `yaml:"tls_handshake_timeout_seconds"`
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Default() *Config {
	return &Config{
		Server: Server{Host: "0.0.0.0", Port: 8080, Mode: "debug"},
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
			MaxForwardAttempts:         2,
			HealthTimeoutSeconds:       8,
			DialTimeoutSeconds:         5,
			TLSHandshakeTimeoutSeconds: 5,
		},
		Log: Log{Level: "info", Format: "text"},
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
