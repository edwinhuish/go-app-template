// Package errs 定义业务错误码与错误包装。
//
// 约定：错误码前三位与 HTTP 状态码对齐（40001 -> 400），
// handler 不必为每个错误单独挑状态码，前端也能按前三位做统一分支。
package errs

import (
	"errors"
	"fmt"
	"net/http"
)

// Code 是业务错误码。
type Code int

const (
	// CodeOK 表示成功。
	CodeOK Code = 0

	// CodeInvalidParam 参数错误（400）。
	CodeInvalidParam Code = 40001
	// CodeUnauthorized 未登录或登录态失效（401）。
	CodeUnauthorized Code = 40101
	// CodeForbidden 已登录但无权限（403）。
	CodeForbidden Code = 40301
	// CodeNotFound 资源不存在（404）。
	CodeNotFound Code = 40401
	// CodeConflict 唯一约束或状态冲突（409）。
	CodeConflict Code = 40901
	// CodeTooManyRequests 请求过于频繁（429）。
	CodeTooManyRequests Code = 42901
	// CodeInternal 服务内部错误（500）。
	CodeInternal Code = 50001
	// CodeTimeout 请求处理超时（504）。
	CodeTimeout Code = 50401
)

// HTTPStatus 把业务错误码映射为 HTTP 状态码。
func (c Code) HTTPStatus() int {
	switch c / 100 {
	case 400:
		return http.StatusBadRequest
	case 401:
		return http.StatusUnauthorized
	case 403:
		return http.StatusForbidden
	case 404:
		return http.StatusNotFound
	case 409:
		return http.StatusConflict
	case 429:
		return http.StatusTooManyRequests
	case 504:
		return http.StatusGatewayTimeout
	case 500:
		return http.StatusInternalServerError
	}
	if c == CodeOK {
		return http.StatusOK
	}
	return http.StatusInternalServerError
}

// Error 是带业务码的错误。实现 Unwrap 后，errors.Is / errors.As 仍可判断底层错误。
type Error struct {
	Code Code
	Msg  string
	err  error
}

// New 构造一个只带业务码与文案的错误。
func New(code Code, msg string) *Error { return &Error{Code: code, Msg: msg} }

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%d: %s: %v", e.Code, e.Msg, e.err)
	}
	return fmt.Sprintf("%d: %s", e.Code, e.Msg)
}

// Unwrap 暴露底层错误，供 errors.Is 使用。
func (e *Error) Unwrap() error { return e.err }

// WithMsg 返回替换文案后的副本，避免修改包级变量。
func (e *Error) WithMsg(msg string) *Error {
	cp := *e
	cp.Msg = msg
	return &cp
}

// Wrap 保留业务码并挂上底层错误，用于日志追溯。
func Wrap(err error, code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg, err: err}
}

// From 尝试把任意错误还原成 *Error，非业务错误返回 ok=false。
func From(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// 常用错误。需要不同文案时用 WithMsg 复制一份，不要改这里的值。
var (
	ErrNotFound        = New(CodeNotFound, "资源不存在")
	ErrUnauthorized    = New(CodeUnauthorized, "未登录或登录状态已失效")
	ErrForbidden       = New(CodeForbidden, "没有操作权限")
	ErrInternal        = New(CodeInternal, "服务器内部错误")
	ErrConflict        = New(CodeConflict, "数据已存在或状态冲突")
	ErrTooManyRequests = New(CodeTooManyRequests, "请求过于频繁，请稍后再试")
	ErrTimeout         = New(CodeTimeout, "请求处理超时，请稍后重试")
)

// BadRequest 构造参数错误。
func BadRequest(msg string) *Error { return New(CodeInvalidParam, msg) }
