package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
	"MyApi/internal/platform/apperr"
	"MyApi/internal/platform/money"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrUserSuspended    = errors.New("user suspended")
	ErrUserStateOp      = errors.New("user state operation not allowed")
	ErrStateNotActive   = errors.New("user not active")
	ErrKeyNotFound      = errors.New("key not found")
	ErrKeyNameDuplicate = errors.New("key name duplicate")
)

var allowedUserGroups = map[string]bool{"default": true, "vip": true, "enterprise": true}
var allowedUserStatus = map[string]bool{"active": true, "suspended": true, "deleted": true}

// BalanceInvalidator 由计费模块实现，用于在余额变化后失效 Redis 缓存。
type BalanceInvalidator interface {
	InvalidateBalance(ctx context.Context, uid int64)
}

type Service struct {
	db          *gorm.DB
	invalidator BalanceInvalidator
}

func New(db *gorm.DB, inv BalanceInvalidator) *Service {
	return &Service{db: db, invalidator: inv}
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashPassword(pw string) (string, error) {
	hb, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hb), nil
}

func parseTimePtr(s string) (*time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

type BalanceOut struct {
	AvailableBalance string `json:"available_balance"`
	FrozenBalance    string `json:"frozen_balance"`
	UpdatedAt        any    `json:"updated_at"`
}

type UserItem struct {
	ID            int64       `json:"id"`
	Nickname      *string     `json:"nickname"`
	UserGroup     string      `json:"user_group"`
	Status        string      `json:"status"`
	LastLoginAt   *time.Time  `json:"last_login_at"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Balance       *BalanceOut `json:"balance,omitempty"`
	PasswordPlain string      `json:"password_plaintext,omitempty"`
	KeyCount      int64       `json:"key_count,omitempty"`
}

func balanceOf(b *model.UserBalance) *BalanceOut {
	if b == nil || b.UserID == 0 {
		return &BalanceOut{AvailableBalance: "0.000000", FrozenBalance: "0.000000", UpdatedAt: nil}
	}
	return &BalanceOut{
		AvailableBalance: money.Format6(b.AvailableBalance),
		FrozenBalance:    money.Format6(b.FrozenBalance),
		UpdatedAt:        b.UpdatedAt,
	}
}

func (s *Service) userByID(ctx context.Context, id int64) (*model.User, bool) {
	var u model.User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, false
	}
	return &u, true
}

func (s *Service) loadBalance(ctx context.Context, userID int64) model.UserBalance {
	var b model.UserBalance
	_ = s.db.WithContext(ctx).Where("user_id = ?", userID).First(&b).Error
	return b
}

type CreateUserInput struct {
	Nickname  *string
	UserGroup string
	Status    string
	Password  string
}

func (s *Service) Create(ctx context.Context, in CreateUserInput) (*UserItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	group := in.UserGroup
	if group == "" {
		group = "default"
	}
	status := in.Status
	if status == "" {
		status = "active"
	}
	if !allowedUserGroups[group] || !allowedUserStatus[status] {
		return nil, apperr.Invalid("user_group 或 status 非法")
	}
	db := s.db.WithContext(ctx)

	gen := false
	password := in.Password
	if password == "" {
		r, err := randomSecret(12)
		if err != nil {
			return nil, err
		}
		password = r
		gen = true
	}
	ph, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	u := model.User{
		PasswordHash: ph,
		UserGroup:    group,
		Status:       status,
		Nickname:     in.Nickname,
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserBalance{UserID: u.ID}).Error
	})
	if err != nil {
		return nil, err
	}

	out := UserItem{
		ID: u.ID, Nickname: u.Nickname, UserGroup: u.UserGroup,
		Status: u.Status, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		Balance: &BalanceOut{AvailableBalance: "0.000000", FrozenBalance: "0.000000"},
	}
	if gen {
		out.PasswordPlain = password
	}
	return &out, nil
}

func (s *Service) List(ctx context.Context, status, group, keyword string, page, pageSize int) ([]UserItem, int64, error) {
	if s.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	q := db.Model(&model.User{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if group != "" {
		q = q.Where("user_group = ?", group)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		q = q.Where("id::text = ? OR nickname LIKE ?", kw, "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.User
	if err := q.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	balances := map[int64]model.UserBalance{}
	if len(ids) > 0 {
		var bs []model.UserBalance
		_ = db.Where("user_id IN ?", ids).Find(&bs).Error
		for i := range bs {
			balances[bs[i].UserID] = bs[i]
		}
	}
	list := make([]UserItem, 0, len(rows))
	for i := range rows {
		o := UserItem{
			ID: rows[i].ID, Nickname: rows[i].Nickname, UserGroup: rows[i].UserGroup,
			Status: rows[i].Status, LastLoginAt: rows[i].LastLoginAt,
			CreatedAt: rows[i].CreatedAt, UpdatedAt: rows[i].UpdatedAt,
		}
		if b, ok := balances[rows[i].ID]; ok {
			o.Balance = balanceOf(&b)
		}
		list = append(list, o)
	}
	return list, total, nil
}

func (s *Service) Get(ctx context.Context, id int64) (*UserItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	u, found := s.userByID(ctx, id)
	if !found {
		return nil, ErrUserNotFound
	}
	b := s.loadBalance(ctx, id)
	var keyCount int64
	_ = db.Model(&model.ClientApiKey{}).Where("user_id = ?", id).Count(&keyCount).Error
	return &UserItem{
		ID: u.ID, Nickname: u.Nickname, UserGroup: u.UserGroup, Status: u.Status,
		LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		Balance: balanceOf(&b), KeyCount: keyCount,
	}, nil
}

type UpdateUserInput struct {
	Nickname  *string
	UserGroup *string
	Password  *string
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateUserInput) (*UserItem, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	db := s.db.WithContext(ctx)
	u, found := s.userByID(ctx, id)
	if !found {
		return nil, ErrUserNotFound
	}
	updates := map[string]any{}
	if in.Nickname != nil {
		if *in.Nickname == "" {
			updates["nickname"] = nil
		} else {
			updates["nickname"] = *in.Nickname
		}
	}
	if in.UserGroup != nil {
		if !allowedUserGroups[*in.UserGroup] {
			return nil, apperr.Invalid("user_group 非法")
		}
		updates["user_group"] = *in.UserGroup
	}
	gen := ""
	if in.Password != nil {
		if *in.Password == "" {
			r, err := randomSecret(12)
			if err != nil {
				return nil, err
			}
			*in.Password = r
			gen = r
		}
		ph, err := hashPassword(*in.Password)
		if err != nil {
			return nil, err
		}
		updates["password_hash"] = ph
	}
	if len(updates) == 0 {
		return nil, apperr.Invalid("没有可更新字段")
	}
	if err := db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = db.First(u, id).Error
	return &UserItem{
		ID: u.ID, Nickname: u.Nickname, UserGroup: u.UserGroup, Status: u.Status,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, PasswordPlain: gen,
	}, nil
}

func (s *Service) SetStatus(ctx context.Context, id int64, status string) error {
	if s.db == nil {
		return gorm.ErrInvalidDB
	}
	if !allowedUserStatus[status] {
		return apperr.Invalid("status 非法")
	}
	db := s.db.WithContext(ctx)
	u, found := s.userByID(ctx, id)
	if !found {
		return ErrUserNotFound
	}
	if u.Status == "deleted" && status != "deleted" {
		return ErrUserStateOp
	}
	return db.Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
}

// Delete 永久删除用户及其关联数据。
func (s *Service) Delete(ctx context.Context, id int64) error {
	if s.db == nil {
		return gorm.ErrInvalidDB
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.ClientApiKey{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.UserBalance{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.BalanceTransaction{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.UserDailyStat{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.UsageLog{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&model.User{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrUserNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}
	if s.invalidator != nil {
		s.invalidator.InvalidateBalance(context.Background(), id)
	}
	return nil
}

func (s *Service) GetBalance(ctx context.Context, id int64) (*BalanceOut, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if _, found := s.userByID(ctx, id); !found {
		return nil, ErrUserNotFound
	}
	b := s.loadBalance(ctx, id)
	return balanceOf(&b), nil
}

type RechargeOut struct {
	TxID          int64  `json:"tx_id"`
	BalanceBefore string `json:"balance_before"`
	BalanceAfter  string `json:"balance_after"`
}

func (s *Service) Recharge(ctx context.Context, id int64, amount string, orderID, description *string) (*RechargeOut, error) {
	if s.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	amt, err := money.ParsePositive(amount)
	if err != nil {
		return nil, err
	}
	var out RechargeOut
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var u model.User
		if err := tx.First(&u, id).Error; err != nil {
			return err
		}
		if u.Status != "active" {
			return ErrStateNotActive
		}
		var bal model.UserBalance
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", id).First(&bal).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			bal = model.UserBalance{UserID: id}
			if err := tx.Create(&bal).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		before := bal.AvailableBalance
		after := before + amt
		bal.AvailableBalance = after
		bal.Version++
		if err := tx.Save(&bal).Error; err != nil {
			return err
		}
		tr := model.BalanceTransaction{
			UserID:         id,
			Amount:         amt,
			BalanceBefore:  before,
			BalanceAfter:   after,
			TxType:         "recharge",
			RelatedOrderID: orderID,
			Description:    description,
		}
		if err := tx.Create(&tr).Error; err != nil {
			return err
		}
		out.TxID = tr.ID
		out.BalanceBefore = money.Format6(before)
		out.BalanceAfter = money.Format6(after)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.invalidator != nil {
		s.invalidator.InvalidateBalance(context.Background(), id)
	}
	return &out, nil
}

type TxFilter struct {
	UserID   int64
	TxType   string
	Start    *time.Time
	End      *time.Time
	Page     int
	PageSize int
}

type TxItem struct {
	ID               int64     `json:"id"`
	TxType           string    `json:"tx_type"`
	Amount           string    `json:"amount"`
	BalanceBefore    string    `json:"balance_before"`
	BalanceAfter     string    `json:"balance_after"`
	RelatedRequestID *string   `json:"related_request_id"`
	RelatedOrderID   *string   `json:"related_order_id"`
	Description      *string   `json:"description"`
	CreatedAt        time.Time `json:"created_at"`
}

func (s *Service) ListTransactions(ctx context.Context, f TxFilter) ([]TxItem, int64, error) {
	if s.db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	q := s.db.WithContext(ctx).Model(&model.BalanceTransaction{}).Where("user_id = ?", f.UserID)
	if f.TxType != "" {
		q = q.Where("tx_type = ?", f.TxType)
	}
	if f.Start != nil {
		q = q.Where("created_at >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("created_at <= ?", *f.End)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.BalanceTransaction
	if err := q.Order("id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	list := make([]TxItem, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		list = append(list, TxItem{
			ID:               r.ID,
			TxType:           r.TxType,
			Amount:           money.Format6(r.Amount),
			BalanceBefore:    money.Format6(r.BalanceBefore),
			BalanceAfter:     money.Format6(r.BalanceAfter),
			RelatedRequestID: r.RelatedRequest,
			RelatedOrderID:   r.RelatedOrderID,
			Description:      r.Description,
			CreatedAt:        r.CreatedAt,
		})
	}
	return list, total, nil
}
