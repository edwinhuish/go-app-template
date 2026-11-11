package errs_test

// 本文件把码表钉住：码 → HTTP 状态码这件事是契约的一部分，两端都按前三位分支。
//
// 没有它的时候，改一个常量或漏一个 case 都不会有编译错误：状态码悄悄变成 500，
// 前端按 4xx 写的重试/提示逻辑全部失效，而单元测试与集成测试都是绿的。

import (
	"errors"
	"net/http"
	"testing"

	"example.com/app/internal/pkg/errs"
)

// TestCodeHTTPStatus 逐个断言码表与 HTTP 状态码的对应关系。
func TestCodeHTTPStatus(t *testing.T) {
	cases := []struct {
		code   errs.Code
		status int
	}{
		{errs.CodeOK, http.StatusOK},
		{errs.CodeInvalidParam, http.StatusBadRequest},
		{errs.CodeUnauthorized, http.StatusUnauthorized},
		{errs.CodeForbidden, http.StatusForbidden},
		{errs.CodeNotFound, http.StatusNotFound},
		{errs.CodeConflict, http.StatusConflict},
		{errs.CodeTooManyRequests, http.StatusTooManyRequests},
		{errs.CodeInternal, http.StatusInternalServerError},
		{errs.CodeTimeout, http.StatusGatewayTimeout},
	}

	for _, c := range cases {
		if got := c.code.HTTPStatus(); got != c.status {
			t.Errorf("Code(%d).HTTPStatus() = %d，期望 %d", int(c.code), got, c.status)
		}
	}
}

// TestFromStatus 断言反向映射：框架决定的状态码要能落回本项目自己的码。
func TestFromStatus(t *testing.T) {
	cases := []struct {
		status int
		want   errs.Code
	}{
		{http.StatusBadRequest, errs.CodeInvalidParam},
		{http.StatusUnauthorized, errs.CodeUnauthorized},
		{http.StatusForbidden, errs.CodeForbidden},
		{http.StatusNotFound, errs.CodeNotFound},
		{http.StatusConflict, errs.CodeConflict},
		{http.StatusTooManyRequests, errs.CodeTooManyRequests},
		{http.StatusGatewayTimeout, errs.CodeTimeout},
		// 未知状态码宁可折成 500，也不凭空造一个码表里没有的业务码。
		{http.StatusTeapot, errs.CodeInternal},
		{http.StatusInternalServerError, errs.CodeInternal},
	}

	for _, c := range cases {
		if got := errs.FromStatus(c.status).Code; got != c.want {
			t.Errorf("FromStatus(%d).Code = %d，期望 %d", c.status, int(got), int(c.want))
		}
	}
}

// TestFromUnwraps 断言业务错误穿过 fmt.Errorf 包装后仍能被识别。
//
// 仓储与 service 都会用 fmt.Errorf("...: %w") 加定位信息；一旦 From 认不出来，
// 400 会在最外层被折成 500，正常提示变成「服务器内部错误」。
func TestFromUnwraps(t *testing.T) {
	inner := errs.BadRequest("标题不能为空")
	wrapped := errors.Join(errors.New("上下文"), inner)

	e, ok := errs.From(wrapped)
	if !ok {
		t.Fatal("From 没有识别出被包装的业务错误")
	}
	if e.Code != errs.CodeInvalidParam || e.Msg != "标题不能为空" {
		t.Errorf("还原出的错误 = %d / %q，期望 40001 / 标题不能为空", int(e.Code), e.Msg)
	}

	if _, ok := errs.From(errors.New("普通错误")); ok {
		t.Error("普通错误不该被认成业务错误")
	}
}

// TestWithMsgCopies 断言 WithMsg 返回副本，不改包级变量。
//
// 改包级变量是那种「第一次请求之后就再也回不去」的 bug：默认文案被永久污染，
// 而且只在同一个进程里复现。
func TestWithMsgCopies(t *testing.T) {
	custom := errs.ErrNotFound.WithMsg("记录已被删除")
	if custom.Msg != "记录已被删除" {
		t.Errorf("副本文案 = %q", custom.Msg)
	}
	if errs.ErrNotFound.Msg != "资源不存在" {
		t.Errorf("包级变量的文案被改动了：%q", errs.ErrNotFound.Msg)
	}
	if custom.Code != errs.ErrNotFound.Code {
		t.Error("副本的业务码应与原值一致")
	}
}

// TestErrorImplementsStatusCode 断言 *Error 满足 Fuego 的 ErrorWithStatus。
//
// 这条接口保证「即使自定义错误处理器被换掉、回落到框架默认的那个」，
// 状态码也不会一律变成 500。用编译期断言以外的形式再验一次，避免有人删掉方法。
func TestErrorImplementsStatusCode(t *testing.T) {
	var e any = errs.ErrForbidden
	type statusCoder interface{ StatusCode() int }
	sc, ok := e.(statusCoder)
	if !ok {
		t.Fatal("*errs.Error 不再实现 StatusCode()，Fuego 会把所有错误当 500")
	}
	if got := sc.StatusCode(); got != http.StatusForbidden {
		t.Errorf("StatusCode() = %d，期望 403", got)
	}
}
