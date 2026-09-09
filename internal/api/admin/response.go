package admin

import "github.com/gin-gonic/gin"

const CodeOK = 0

type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func OK(c *gin.Context, data any) {
	c.JSON(200, response{Code: CodeOK, Message: "ok", Data: data})
}

func Fail(c *gin.Context, code int, message string) {
	c.JSON(200, response{Code: code, Message: message})
}
