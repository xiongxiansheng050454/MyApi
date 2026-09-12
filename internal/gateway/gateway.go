package gateway

import (
	"io"
	"log/slog"
	"strings"
	"time"

	"MyApi/internal/auth"
	"MyApi/internal/billing"
	"MyApi/internal/catalog"
	"MyApi/internal/config"
	"MyApi/internal/platform/tokenizer"
	"MyApi/internal/pricing"
	protocol "MyApi/internal/protocol/openai"
	"MyApi/internal/ratelimit"
	"MyApi/internal/routing"
	"MyApi/internal/upstream"
	"MyApi/internal/usage"
)

const maxBufferedBody = 10 << 20

type Gateway struct {
	limits    *ratelimit.Service
	routing   *routing.Service
	upstream  *upstream.Service
	pricing   *pricing.Service
	billing   *billing.Service
	usage     *usage.Service
	catalog   *catalog.Service
	tokenizer *tokenizer.Cache
	cfg       *config.Config
	log       *slog.Logger
}

func New(
	limits *ratelimit.Service,
	rt *routing.Service,
	up *upstream.Service,
	pr *pricing.Service,
	bl *billing.Service,
	us *usage.Service,
	cat *catalog.Service,
	tok *tokenizer.Cache,
	cfg *config.Config,
	log *slog.Logger,
) *Gateway {
	return &Gateway{
		limits: limits, routing: rt, upstream: up, pricing: pr,
		billing: bl, usage: us, catalog: cat, tokenizer: tok, cfg: cfg, log: log,
	}
}

// RequestMeta 是一次下游请求的边缘上下文。
type RequestMeta struct {
	RequestID string
	ClientIP  string
	StartedAt time.Time
	SessionID string // X-Session-Id 头
}

type Response struct {
	StatusCode  int
	ContentType string
	Body        io.ReadCloser
}

// Models 返回对下游发布的模型名（按 Key 白名单过滤）。
func (g *Gateway) Models(ident *auth.Identity) []string {
	return g.catalog.Models(ident)
}

func allowed(ident *auth.Identity, model string) bool {
	if ident.AllowAll {
		return true
	}
	for _, m := range ident.Models {
		if m == model {
			return true
		}
	}
	return false
}

func sessionOf(meta RequestMeta, parsed *protocol.ParsedRequest) string {
	if s := strings.TrimSpace(meta.SessionID); s != "" {
		return s
	}
	return parsed.SessionUser
}

func (g *Gateway) countInputTokens(parsed *protocol.ParsedRequest) int64 {
	n := g.tokenizer.Count(parsed.Model, parsed.InputText)
	n += 3 * parsed.MessageCount
	return int64(n)
}

func (g *Gateway) tokenCounter(parsed *protocol.ParsedRequest) func() int64 {
	var cached int64
	return func() int64 {
		if cached == 0 {
			cached = g.countInputTokens(parsed)
		}
		return cached
	}
}

func (g *Gateway) outputBudget(parsed *protocol.ParsedRequest) int {
	b := parsed.MaxOutputTokens
	if b <= 0 {
		b = g.cfg.Billing.DefaultOutputBudget
	}
	if b <= 0 {
		b = 1024
	}
	return b
}
