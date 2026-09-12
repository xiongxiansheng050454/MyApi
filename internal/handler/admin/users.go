package admin

import (
	"github.com/gin-gonic/gin"

	"MyApi/internal/platform/httpx"
	"MyApi/internal/user"
)

func (h *Handler) CreateUser(c *gin.Context) {
	var req struct {
		Nickname  *string `json:"nickname"`
		UserGroup string  `json:"user_group"`
		Status    string  `json:"status"`
		Password  string  `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.users.Create(c.Request.Context(), user.CreateUserInput{
		Nickname: req.Nickname, UserGroup: req.UserGroup, Status: req.Status, Password: req.Password,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListUsers(c *gin.Context) {
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.users.List(c.Request.Context(), c.Query("status"), c.Query("user_group"), c.Query("keyword"), page, pageSize)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

func (h *Handler) GetUser(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	out, err := h.users.Get(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) UpdateUser(c *gin.Context) {
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
	out, err := h.users.Update(c.Request.Context(), id, user.UpdateUserInput{
		Nickname: req.Nickname, UserGroup: req.UserGroup, Password: req.Password,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) SetUserStatus(c *gin.Context) {
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
	if err := h.users.SetStatus(c.Request.Context(), id, req.Status); err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": id, "status": req.Status})
}

func (h *Handler) DeleteUser(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	if err := h.users.Delete(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"id": id})
}

func (h *Handler) GetUserBalance(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	out, err := h.users.GetBalance(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) Recharge(c *gin.Context) {
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
	out, err := h.users.Recharge(c.Request.Context(), id, req.Amount, req.RelatedOrderID, req.Description)
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListBalanceTransactions(c *gin.Context) {
	id, ok := idParam(c, "userId")
	if !ok {
		return
	}
	start, end, ok := parseTimeRange(c)
	if !ok {
		return
	}
	page, pageSize := httpx.PageParams(c)
	list, total, err := h.users.ListTransactions(c.Request.Context(), user.TxFilter{
		UserID: id, TxType: c.Query("tx_type"), Start: start, End: end, Page: page, PageSize: pageSize,
	})
	if err != nil {
		failErr(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}
