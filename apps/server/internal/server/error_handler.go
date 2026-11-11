package server

import (
	"context"
	"errors"

	"github.com/go-fuego/fuego"

	"example.com/app/internal/pkg/errs"
)

// 编译期断言：业务错误实现了 Fuego 的 ErrorWithStatus。
//
// 官方 custom-errors 示例的入口就是这两个接口（ErrorWithStatus / ErrorWithDetail）。
// 实现它不是为了「用上框架的类型」，而是为了让状态码只由 Code 推导一次——框架回落到
// 默认错误处理时也不会把 400 变成 500。
var _ fuego.ErrorWithStatus = (*errs.Error)(nil)

// errorHandler 是注入给 Fuego 的错误处理器，也是官方 custom-errors 示例那一层的落地。
//
// 分工：**ErrorHandler 决定错误「是什么」，ErrorSerializer 决定「怎么写出去」**
// （后者见 hooks.go → response.Fail）。上一版把它写成恒等函数，于是框架自己抛出的
// 错误（ValidateParams 的缺参、transformOut 的转换失败、fuego.HTTPError 之类）会直接
// 落到 response.Fail 的「非业务错误」分支，被统一折成 500 —— 一个参数问题在监控上
// 看起来像服务挂了。
func errorHandler(_ context.Context, err error) error {
	// 已经是本项目的业务错误：原样返回，信封与文案由序列化器决定。
	if _, ok := errs.From(err); ok {
		return err
	}

	// 带状态码的框架错误：按状态码翻成业务错误，别让它降级成 500。
	var withStatus fuego.ErrorWithStatus
	if errors.As(err, &withStatus) {
		mapped := errs.FromStatus(withStatus.StatusCode())
		if mapped.Code.HTTPStatus() >= 500 {
			// 5xx 要把原始错误挂回去：response.Fail 只对 5xx 记日志，
			// 丢掉 cause 就等于丢掉唯一能定位问题的信息。
			return errs.Wrap(err, mapped.Code, mapped.Msg)
		}
		return mapped
	}

	// 其余错误原样交给序列化器，由它折叠成 500 并记日志。
	return err
}
