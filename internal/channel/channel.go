package channel

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"MyApi/internal/channelmanager"
	"MyApi/internal/config"
	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/platform/money"
	"MyApi/internal/secret"
)

var (
	ErrNotFound       = errors.New("channel not found")
	ErrModelNotFound  = errors.New("channel model not found")
	ErrModelDuplicate = errors.New("channel model duplicate")
	ErrSecretMissing  = errors.New("channel secret missing")
	ErrNoModelMapping = errors.New("no enabled model mapping")
)

type Service struct {
	db       *gorm.DB
	secret   *secret.Secret
	channels *channelmanager.Manager
	cfg      *config.Config
	log      *slog.Logger
}

func New(db *gorm.DB, sec *secret.Secret, channels *channelmanager.Manager, cfg *config.Config, log *slog.Logger) *Service {
	return &Service{db: db, secret: sec, channels: channels, cfg: cfg, log: log}
}

type Item struct {
	ID               int64          `json:"id"`
	Name             string         `json:"name"`
	BaseURL          string         `json:"base_url"`
	APIKeyMasked     string         `json:"api_key_masked"`
	AuthType         string         `json:"auth_type"`
	ExtraConfig      datatypes.JSON `json:"extra_config"`
	Status           int16          `json:"status"`
	Weight           int            `json:"weight"`
	Priority         int            `json:"priority"`
	Balance          *string        `json:"balance"`
	BalanceUpdatedAt *time.Time     `json:"balance_updated_at"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	ModelCount       int64          `json:"model_count,omitempty"`
}

func maskSecret(s string) string {
	switch {
	case s == "":
		return ""
	case len(s) <= 8:
		return "****"
	default:
		return s[:3] + "****" + s[len(s)-4:]
	}
}

func toItem(m *model.Channel) Item {
	out := Item{
		ID:           m.ID,
		Name:         m.Name,
		BaseURL:      m.BaseURL,
		APIKeyMasked: maskSecret(m.APIKey),
		AuthType:     m.AuthType,
		ExtraConfig:  m.ExtraConfig,
		Status:       m.Status,
		Weight:       m.Weight,
		Priority:     m.Priority,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
	if len(m.ExtraConfig) == 0 {
		out.ExtraConfig = datatypes.JSON("{}")
	}
	if m.Balance != nil {
		s := money.Format8(*m.Balance)
		out.Balance = &s
	}
	out.BalanceUpdatedAt = m.BalanceUpdatedAt
	return out
}

func (s *Service) notifyChanged(id int64) {
	if s.channels != nil {
		s.channels.NotifyChanged(id)
	}
}

func (s *Service) notifyDeleted(id int64) {
	if s.channels != nil {
		s.channels.NotifyDeleted(id)
	}
}

func (s *Service) encryptAPIKey(plain string) (string, error) {
	if s.secret == nil {
		return "", ErrSecretMissing
	}
	return s.secret.Encrypt(plain)
}

type CreateInput struct {
	Name        string
	BaseURL     string
	APIKey      string
	AuthType    string
	ExtraConfig json.RawMessage
	Status      *int
	Weight      *int
	Priority    *int
	Balance     *string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*Item, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	encKey, err := s.encryptAPIKey(in.APIKey)
	if err != nil {
		return nil, err
	}
	status := int16(1)
	if in.Status != nil {
		status = int16(*in.Status)
	}
	weight := 100
	if in.Weight != nil {
		weight = *in.Weight
	}
	priority := 0
	if in.Priority != nil {
		priority = *in.Priority
	}
	authType := "bearer"
	if in.AuthType != "" {
		authType = in.AuthType
	}
	m := model.Channel{
		Name:        strings.TrimSpace(in.Name),
		BaseURL:     strings.TrimRight(strings.TrimSpace(in.BaseURL), "/"),
		APIKey:      encKey,
		AuthType:    authType,
		ExtraConfig: datatypes.JSON(in.ExtraConfig),
		Status:      status,
		Weight:      weight,
		Priority:    priority,
	}
	if in.Balance != nil && strings.TrimSpace(*in.Balance) != "" {
		v, ok := money.ParseNonNegative(*in.Balance)
		if !ok {
			return nil, apperr.Invalid("balance 必须为 >= 0 的数字")
		}
		m.Balance = &v
	}
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	s.notifyChanged(m.ID)
	out := toItem(&m)
	return &out, nil
}

type UpdateInput struct {
	Name        string
	BaseURL     string
	APIKey      string
	AuthType    string
	ExtraConfig json.RawMessage
	Status      *int
	Weight      *int
	Priority    *int
	Balance     *string
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateInput) (*Item, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	updates := map[string]any{}
	if in.Name != "" {
		updates["name"] = strings.TrimSpace(in.Name)
	}
	if in.BaseURL != "" {
		updates["base_url"] = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	}
	if in.APIKey != "" {
		encKey, err := s.encryptAPIKey(in.APIKey)
		if err != nil {
			return nil, err
		}
		updates["api_key"] = encKey
	}
	if in.AuthType != "" {
		updates["auth_type"] = in.AuthType
	}
	if len(in.ExtraConfig) > 0 {
		updates["extra_config"] = datatypes.JSON(in.ExtraConfig)
	}
	if in.Status != nil {
		updates["status"] = int16(*in.Status)
	}
	if in.Weight != nil {
		updates["weight"] = *in.Weight
	}
	if in.Priority != nil {
		updates["priority"] = *in.Priority
	}
	if in.Balance != nil {
		if strings.TrimSpace(*in.Balance) == "" {
			updates["balance"] = nil
			updates["balance_updated_at"] = nil
		} else {
			v, ok := money.ParseNonNegative(*in.Balance)
			if !ok {
				return nil, apperr.Invalid("balance 必须为 >= 0 的数字")
			}
			updates["balance"] = v
			updates["balance_updated_at"] = time.Now()
		}
	}
	if len(updates) == 0 {
		return nil, apperr.Invalid("没有可更新字段")
	}
	res := db.Model(&model.Channel{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	s.notifyChanged(id)
	var m model.Channel
	_ = db.First(&m, id).Error
	out := toItem(&m)
	return &out, nil
}

func (s *Service) SetStatus(ctx context.Context, id int64, status int) (*Item, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	res := db.Model(&model.Channel{}).Where("id = ?", id).Update("status", int16(status))
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	s.notifyChanged(id)
	var m model.Channel
	_ = db.First(&m, id).Error
	out := toItem(&m)
	return &out, nil
}

func (s *Service) SetBalance(ctx context.Context, id int64, balance, delta *string, description *string) (*Item, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	var m model.Channel
	if err := db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	updates := map[string]any{"balance_updated_at": time.Now()}
	if balance != nil {
		v, ok := money.ParseNonNegative(*balance)
		if !ok {
			return nil, apperr.Invalid("balance 必须为 >= 0 的数字")
		}
		updates["balance"] = v
	} else {
		d, err := parseDelta(*delta)
		if err != nil {
			return nil, err
		}
		cur := 0.0
		if m.Balance != nil {
			cur = *m.Balance
		}
		updates["balance"] = math.Round((cur+d)*1e8) / 1e8
	}
	if err := db.Model(&model.Channel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	if description != nil {
		s.log.Info("channel balance adjusted", "channel_id", id, "description", *description)
	}
	s.notifyChanged(id)
	_ = db.First(&m, id).Error
	out := toItem(&m)
	return &out, nil
}

func parseDelta(s string) (float64, error) {
	d, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(d) || math.IsInf(d, 0) {
		return 0, apperr.Invalid("delta 必须为数字")
	}
	return d, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if s.db == nil {
		return gorm.ErrInvalidDB
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", id).Delete(&model.ModelPricing{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", id).Delete(&model.ChannelModel{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&model.Channel{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.notifyDeleted(id)
	return nil
}

func (s *Service) Get(ctx context.Context, id int64) (*Item, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	var m model.Channel
	if err := db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var count int64
	_ = db.Model(&model.ChannelModel{}).Where("channel_id = ?", id).Count(&count).Error
	out := toItem(&m)
	out.ModelCount = count
	return &out, nil
}

func (s *Service) List(ctx context.Context, status, keyword string, page, pageSize int) ([]Item, int64, error) {
	if s.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Model(&model.Channel{})
	if status == "1" || status == "0" {
		q = q.Where("status = ?", status)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		q = q.Where("id::text = ? OR name LIKE ?", kw, "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.Channel
	if err := q.Order("priority DESC").Order("id ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	list := make([]Item, 0, len(rows))
	for i := range rows {
		list = append(list, toItem(&rows[i]))
	}
	return list, total, nil
}
