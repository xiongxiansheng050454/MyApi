package httpx

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const CodeOK = 0

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func OK(c *gin.Context, data any) {
	c.JSON(200, Response{Code: CodeOK, Message: "ok", Data: data})
}

func Fail(c *gin.Context, code int, message string) {
	c.JSON(200, Response{Code: code, Message: message})
}

func PageParams(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v > 0 {
		pageSize = v
		if pageSize > 100 {
			pageSize = 100
		}
	}
	return page, pageSize
}

func IDParam(c *gin.Context, key string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func BindJSON(c *gin.Context, out any) bool {
	return c.ShouldBindJSON(out) == nil
}

func BearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}

// StreamWriter 由下游 handler 实现（gin），业务层通过它下发流式内容。
type StreamWriter interface {
	Header(status int, contentType string)
	Write(p []byte) (int, error)
	Flush()
}
