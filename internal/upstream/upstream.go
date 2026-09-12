package upstream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"MyApi/internal/channelmanager"
	"MyApi/internal/config"
	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/secret"
)

var errChannelGone = errors.New("channel row gone while in flight")

// Request 是一次上游转发所需的中性请求上下文；Body 已由协议层改写。
type Request struct {
	Model         string
	UpstreamModel string
	Body          []byte
	UserID        int64
	APIKeyID      int64
	RequestID     string
	ClientIP      string
	StartedAt     time.Time
	SessionID     string
}

type Conn struct {
	StatusCode  int
	ContentType string
	Body        io.ReadCloser
}

type Service struct {
	db       *gorm.DB
	channels *channelmanager.Manager
	secret   *secret.Secret
	client   *http.Client
	cfg      *config.Config
	log      *slog.Logger
}

func New(db *gorm.DB, channels *channelmanager.Manager, sec *secret.Secret, client *http.Client, cfg *config.Config, log *slog.Logger) *Service {
	return &Service{db: db, channels: channels, secret: sec, client: client, cfg: cfg, log: log}
}

// Open 建立到上游的一次连接（不读取/转发 body），经渠道熔断器执行。
func (s *Service) Open(ctx context.Context, req *Request, handle *channelmanager.Handle, info channelmanager.ChannelInfo) (*Conn, bool, bool, error) {
	if info.AuthType != "" && info.AuthType != "bearer" {
		return nil, false, true, fmt.Errorf("unsupported auth_type %q", info.AuthType)
	}

	key, err := s.loadChannelKey(ctx, info.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errChannelGone) {
			return nil, false, true, err
		}
		return nil, false, false, err
	}
	defer secret.Zero(key)

	url := strings.TrimRight(info.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(req.Body))
	if err != nil {
		return nil, false, true, fmt.Errorf("build upstream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+string(key))

	sent := false
	var conn *Conn
	cbErr := handle.Execute(func() error {
		sent = true
		resp, derr := s.client.Do(httpReq)
		if derr != nil {
			return derr
		}
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			return fmt.Errorf("upstream http %d", resp.StatusCode)
		}
		conn = &Conn{
			StatusCode:  resp.StatusCode,
			ContentType: resp.Header.Get("Content-Type"),
			Body:        resp.Body,
		}
		return nil
	})
	if cbErr != nil {
		if ctx.Err() != nil {
			return nil, sent, false, ctx.Err()
		}
		return nil, sent, true, cbErr
	}
	return conn, sent, false, nil
}

func (s *Service) loadChannelKey(ctx context.Context, id int64) ([]byte, error) {
	if s.db == nil {
		return nil, apperr.ErrStoreDown
	}
	var row struct {
		APIKey string
	}
	if err := s.db.WithContext(ctx).Model(&model.Channel{}).Select("api_key").Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errChannelGone
		}
		return nil, err
	}
	if s.secret == nil {
		return nil, secret.ErrNotConfigured
	}
	return s.secret.Decrypt(row.APIKey)
}

// ChargeChannelBalance 按本地估算费用扣减渠道余额（NULL 余额=不限，不扣）。
func (s *Service) ChargeChannelBalance(ctx context.Context, channelID int64, amountMicro int64) {
	if amountMicro <= 0 || s.db == nil {
		return
	}
	amount := float64(amountMicro) / 1e8
	err := s.db.WithContext(ctx).Model(&model.Channel{}).
		Where("id = ? AND balance IS NOT NULL", channelID).
		Updates(map[string]any{
			"balance":            gorm.Expr("balance - ?", amount),
			"balance_updated_at": time.Now(),
		}).Error
	if err != nil {
		s.log.Error("charge channel balance", "err", err, "channel_id", channelID)
	}
}
