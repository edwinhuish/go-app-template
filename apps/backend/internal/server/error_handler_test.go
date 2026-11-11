// 本文件守住 errorHandler 的分工：错误「是什么」在这里定，「怎么写出去」在序列化器。
//
// 上一版把它写成恒等函数，后果是框架抛出的带状态码错误（缺参、转换失败、HTTPError）
// 会被序列化器当成「非业务错误」折成 500 —— 参数问题在监控上看起来像服务挂了。
package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/go-fuego/fuego"

	"example.com/app/internal/pkg/errs"
)

func TestErrorHandler(t *testing.T) {
	t.Run("业务错误原样返回", func(t *testing.T) {
		in := errs.ErrConflict.WithMsg("标题已存在")
		if got := errorHandler(context.Background(), in); got != error(in) {
			t.Errorf("返回值被替换了：%v，期望原样返回", got)
		}
	})

	t.Run("框架的 BadRequestError 翻成 40001", func(t *testing.T) {
		got := errorHandler(context.Background(),
			fuego.BadRequestError{Title: "bad query param"})
		assertErrCode(t, got, errs.CodeInvalidParam)
	})

	t.Run("框架的 HTTPError 按状态码映射", func(t *testing.T) {
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
			// 码表里没有对应项的状态码一律当内部错误：宁可 500，也不造一个表外的码。
			{http.StatusTeapot, errs.CodeInternal},
		}

		for _, c := range cases {
			got := errorHandler(context.Background(),
				fuego.HTTPError{Status: c.status, Title: "framework"})
			assertErrCode(t, got, c.want)
		}
	})

	t.Run("5xx 保留原始错误", func(t *testing.T) {
		cause := errors.New("driver exploded")
		got := errorHandler(context.Background(),
			fuego.HTTPError{Status: http.StatusInternalServerError, Err: cause})

		assertErrCode(t, got, errs.CodeInternal)
		// response.Fail 只对 5xx 记日志；cause 丢了就等于丢了唯一能定位问题的信息。
		if !errors.Is(got, cause) {
			t.Errorf("原始错误没被挂回去：%v", got)
		}
	})

	t.Run("未知错误原样交给序列化器", func(t *testing.T) {
		in := errors.New("boom")
		got := errorHandler(context.Background(), in)
		if got != in {
			t.Errorf("未知错误被提前转换了：%v（应交给 response.Fail 折叠成 500）", got)
		}
	})
}

func assertErrCode(t *testing.T, err error, want errs.Code) {
	t.Helper()

	e, ok := errs.From(err)
	if !ok {
		t.Fatalf("err = %v，期望业务错误", err)
	}
	if e.Code != want {
		t.Errorf("业务码 = %d（%s），期望 %d", e.Code, e.Msg, want)
	}
}
