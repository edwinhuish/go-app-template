// Package response 统一 HTTP 响应格式。
//
// 所有接口返回 {"code":0,"message":"ok","data":...,"request_id":"..."}；
// code != 0 时 data 为空，前端按 code 分支处理。
package response

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"example.com/app/internal/pkg/errs"
)

// RequestIDKey 是请求 ID 在 gin.Context 中的键，中间件写入、这里读出。
const RequestIDKey = "request_id"

// Body 是统一响应体。
type Body struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func requestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// OK 返回 200 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data, RequestID: requestID(c)})
}

// Created 返回 201 创建成功响应。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Code: 0, Message: "created", Data: data, RequestID: requestID(c)})
}

// Fail 输出错误并终止后续 handler。非业务错误统一折叠为 500，避免泄露内部细节。
//
// 折叠不等于丢掉：响应体只留一句「服务器内部错误」，真实原因必须进日志，
// 否则线上只能靠复现去猜。业务错误（4xx）不记，避免用户输错参数刷成告警。
func Fail(c *gin.Context, err error) {
	e, ok := errs.From(err)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		// 超时不是「服务自己错了」，而是「这次没算完」：报 500 会让人去翻代码。
		e = errs.ErrTimeout
	case !ok:
		e = errs.Wrap(err, errs.CodeInternal, "服务器内部错误")
	}

	if e.Code.HTTPStatus() >= http.StatusInternalServerError {
		slog.Error("服务端错误",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"request_id", requestID(c),
			"err", err,
		)
	}

	c.AbortWithStatusJSON(e.Code.HTTPStatus(), Body{
		Code:      int(e.Code),
		Message:   e.Msg,
		RequestID: requestID(c),
	})
}
