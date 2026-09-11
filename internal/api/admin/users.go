package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"MyApi/internal/model"
)

var allowedUserGroups = map[string]bool{"default": true, "vip": true, "enterprise": true}
var allowedUserStatus = map[string]bool{"active": true, "suspended": true, "deleted": true}

func moneyFmt(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }

func parseMoney(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, errors.New("金额必须为大于 0 的数字")
	}
	return v, nil
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

type userOut struct {
	ID            int64       `json:"id"`
	Nickname      *string     `json:"nickname"`
	UserGroup     string      `json:"user_group"`
	Status        string      `json:"status"`
	LastLoginAt   *time.Time  `json:"last_login_at"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Balance       *balanceOut `json:"balance,omitempty"`
	PasswordPlain string      `json:"password_plaintext,omitempty"`
	KeyCount      int64       `json:"key_count,omitempty"`
}

func (h *Handler) userByID(db *gorm.DB, id int64) (*model.User, bool) {
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return nil, false
	}
	return &u, true
}

func (h *Handler) loadBalance(db *gorm.DB, userID int64) model.UserBalance {
	var b model.UserBalance
	_ = db.Where("user_id = ?", userID).First(&b).Error
	return b
}

func (h *Handler) createUser(c *gin.Context) {
	var req struct {
		Nickname  *string `json:"nickname"`
		UserGroup string  `json:"user_group"`
		Status    string  `json:"status"`
		Password  string  `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	group := req.UserGroup
	if group == "" {
		group = "default"
	}
	status := req.Status
	if status == "" {
		status = "active"
	}
	if !allowedUserGroups[group] || !allowedUserStatus[status] {
		Fail(c, CodeParamError, "user_group 或 status 非法")
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}

	gen := false
	password := req.Password
	if password == "" {
		r, err := randomSecret(12)
		if err != nil {
			Fail(c, CodeInternal, "生成密码失败")
			return
		}
		password = r
		gen = true
	}
	ph, err := hashPassword(password)
	if err != nil {
		h.log.Error("hash password", "err", err)
		Fail(c, CodeInternal, "密码哈希失败")
		return
	}

	u := model.User{
		PasswordHash: ph,
		UserGroup:    group,
		Status:       status,
		Nickname:     req.Nickname,
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		return tx.Create(&model.UserBalance{UserID: u.ID}).Error
	})
	if err != nil {
		h.log.Error("create user", "err", err)
		Fail(c, CodeInternal, "创建用户失败")
		return
	}

	out := userOut{
		ID: u.ID, Nickname: u.Nickname, UserGroup: u.UserGroup,
		Status: u.Status, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		Balance: &balanceOut{AvailableBalance: "0.000000", FrozenBalance: "0.000000"},
	}
	if gen {
		out.PasswordPlain = password
	}
	OK(c, out)
}

func (h *Handler) listUsers(c *gin.Context) {
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.User{})
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if g := c.Query("user_group"); g != "" {
		q = q.Where("user_group = ?", g)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("id::text = ? OR nickname LIKE ?", kw, "%"+kw+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		h.log.Error("count users", "err", err)
		Fail(c, CodeInternal, "查询用户失败")
		return
	}
	var rows []model.User
	if err := q.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list users", "err", err)
		Fail(c, CodeInternal, "查询用户失败")
		return
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
	list := make([]userOut, 0, len(rows))
	for i := range rows {
		o := userOut{
			ID: rows[i].ID, Nickname: rows[i].Nickname, UserGroup: rows[i].UserGroup,
			Status: rows[i].Status, LastLoginAt: rows[i].LastLoginAt,
			CreatedAt: rows[i].CreatedAt, UpdatedAt: rows[i].UpdatedAt,
		}
		if b, ok := balances[rows[i].ID]; ok {
			o.Balance = balanceOf(&b)
		}
		list = append(list, o)
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) getUser(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	u, found := h.userByID(db, id)
	if !found {
		Fail(c, CodeUserNotFound, "用户不存在")
		return
	}
	b := h.loadBalance(db, id)
	var keyCount int64
	_ = db.Model(&model.ClientApiKey{}).Where("user_id = ?", id).Count(&keyCount).Error
	o := userOut{
		ID: u.ID, Nickname: u.Nickname, UserGroup: u.UserGroup, Status: u.Status,
		LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		Balance: balanceOf(&b), KeyCount: keyCount,
	}
	OK(c, o)
}

func (h *Handler) updateUser(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	var req struct {
		Nickname  *string `json:"nickname"`
		UserGroup *string `json:"user_group"`
		Password  *string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	u, found := h.userByID(db, id)
	if !found {
		Fail(c, CodeUserNotFound, "用户不存在")
		return
	}
	updates := map[string]any{}
	if req.Nickname != nil {
		if *req.Nickname == "" {
			updates["nickname"] = nil
		} else {
			updates["nickname"] = *req.Nickname
		}
	}
	if req.UserGroup != nil {
		if !allowedUserGroups[*req.UserGroup] {
			Fail(c, CodeParamError, "user_group 非法")
			return
		}
		updates["user_group"] = *req.UserGroup
	}
	gen := ""
	if req.Password != nil {
		if *req.Password == "" {
			r, err := randomSecret(12)
			if err != nil {
				Fail(c, CodeInternal, "生成密码失败")
				return
			}
			*req.Password = r
			gen = r
		}
		ph, err := hashPassword(*req.Password)
		if err != nil {
			h.log.Error("hash password", "err", err)
			Fail(c, CodeInternal, "密码哈希失败")
			return
		}
		updates["password_hash"] = ph
	}
	if len(updates) == 0 {
		Fail(c, CodeParamError, "没有可更新字段")
		return
	}
	if err := db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		h.log.Error("update user", "err", err)
		Fail(c, CodeInternal, "更新用户失败")
		return
	}
	_ = db.First(u, id).Error
	o := userOut{
		ID: u.ID, Nickname: u.Nickname, UserGroup: u.UserGroup, Status: u.Status,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, PasswordPlain: gen,
	}
	OK(c, o)
}

func (h *Handler) setUserStatus(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if !allowedUserStatus[req.Status] {
		Fail(c, CodeParamError, "status 非法")
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	u, found := h.userByID(db, id)
	if !found {
		Fail(c, CodeUserNotFound, "用户不存在")
		return
	}
	if u.Status == "deleted" && req.Status != "deleted" {
		Fail(c, CodeUserStateOp, "已删除用户不可恢复")
		return
	}
	res := db.Model(&model.User{}).Where("id = ?", id).Update("status", req.Status)
	if res.Error != nil {
		h.log.Error("set user status", "err", res.Error)
		Fail(c, CodeInternal, "更新状态失败")
		return
	}
	OK(c, gin.H{"id": id, "status": req.Status})
}

type balanceOut struct {
	AvailableBalance string `json:"available_balance"`
	FrozenBalance    string `json:"frozen_balance"`
	UpdatedAt        any    `json:"updated_at"`
}

func balanceOf(b *model.UserBalance) *balanceOut {
	if b == nil || b.UserID == 0 {
		return &balanceOut{AvailableBalance: "0.000000", FrozenBalance: "0.000000", UpdatedAt: nil}
	}
	return &balanceOut{
		AvailableBalance: moneyFmt(b.AvailableBalance),
		FrozenBalance:    moneyFmt(b.FrozenBalance),
		UpdatedAt:        b.UpdatedAt,
	}
}

func (h *Handler) getUserBalance(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	if _, found := h.userByID(db, id); !found {
		Fail(c, CodeUserNotFound, "用户不存在")
		return
	}
	b := h.loadBalance(db, id)
	OK(c, balanceOf(&b))
}

func (h *Handler) recharge(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	var req struct {
		Amount         string  `json:"amount"`
		RelatedOrderID *string `json:"related_order_id"`
		Description    *string `json:"description"`
	}
	if !bindJSON(c, &req) {
		return
	}
	amount, err := parseMoney(req.Amount)
	if err != nil {
		Fail(c, CodeUserAmountInvalid, err.Error())
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}

	var txOut struct {
		TxID          int64  `json:"tx_id"`
		BalanceBefore string `json:"balance_before"`
		BalanceAfter  string `json:"balance_after"`
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var u model.User
		if err := tx.First(&u, id).Error; err != nil {
			return err
		}
		if u.Status != "active" {
			return errStateNotActive
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
		after := before + amount
		bal.AvailableBalance = after
		bal.Version++
		if err := tx.Save(&bal).Error; err != nil {
			return err
		}
		tr := model.BalanceTransaction{
			UserID:         id,
			Amount:         amount,
			BalanceBefore:  before,
			BalanceAfter:   after,
			TxType:         "recharge",
			RelatedOrderID: req.RelatedOrderID,
			Description:    req.Description,
		}
		if err := tx.Create(&tr).Error; err != nil {
			return err
		}
		txOut.TxID = tr.ID
		txOut.BalanceBefore = moneyFmt(before)
		txOut.BalanceAfter = moneyFmt(after)
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			Fail(c, CodeUserNotFound, "用户不存在")
		case errors.Is(err, errStateNotActive):
			Fail(c, CodeUserStateOp, "用户状态非 active，禁止充值")
		default:
			h.log.Error("recharge", "err", err)
			Fail(c, CodeInternal, "充值失败")
		}
		return
	}
	if h.svc != nil {
		h.svc.InvalidateBalance(context.Background(), id)
	}
	OK(c, txOut)
}

func (h *Handler) listBalanceTransactions(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	db, ok := h.db(c)
	if !ok {
		return
	}
	page, pageSize := pageParams(c)
	q := db.Model(&model.BalanceTransaction{}).Where("user_id = ?", id)
	if t := c.Query("tx_type"); t != "" {
		q = q.Where("tx_type = ?", t)
	}
	if start := c.Query("start_time"); start != "" {
		t, err := time.Parse(time.RFC3339, start)
		if err != nil {
			Fail(c, CodeParamError, "start_time 需为 RFC3339")
			return
		}
		q = q.Where("created_at >= ?", t)
	}
	if end := c.Query("end_time"); end != "" {
		t, err := time.Parse(time.RFC3339, end)
		if err != nil {
			Fail(c, CodeParamError, "end_time 需为 RFC3339")
			return
		}
		q = q.Where("created_at <= ?", t)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		h.log.Error("count balance tx", "err", err)
		Fail(c, CodeInternal, "查询流水失败")
		return
	}
	var rows []model.BalanceTransaction
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		h.log.Error("list balance tx", "err", err)
		Fail(c, CodeInternal, "查询流水失败")
		return
	}
	list := make([]gin.H, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		list = append(list, gin.H{
			"id":                 r.ID,
			"tx_type":            r.TxType,
			"amount":             moneyFmt(r.Amount),
			"balance_before":     moneyFmt(r.BalanceBefore),
			"balance_after":      moneyFmt(r.BalanceAfter),
			"related_request_id": r.RelatedRequest,
			"related_order_id":   r.RelatedOrderID,
			"description":        r.Description,
			"created_at":         r.CreatedAt,
		})
	}
	OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

var errStateNotActive = errors.New("user not active")
