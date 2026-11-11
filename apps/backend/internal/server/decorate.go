package server

import (
	"github.com/go-fuego/fuego"

	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/reqctx"
)

// 本文件是「handler 只返回业务数据」与「响应统一带信封」之间的唯一接缝。
//
// 为什么是装饰器而不是中间件：Fuego 的中间件包在 Flow 之外，只看得到已经写出的
// 字节，拿不到 controller 的返回值与错误；硬要用中间件做，就得缓冲整段响应体再
// 二次编解码，流式响应会直接报废。注册点则是唯一同时握有「类型」和「返回值」的位置。
//
// 为什么不用 any：装饰器保留了泛型参数，Fuego 据此生成的 OpenAPI 里 200 响应的
// schema 仍然是 Envelope[T] 而不是裸结构体。变成 any 的话文档会立刻失真。
//
// 三个类型参数是为了一个函数吃下所有形状：fuego.ContextNoBody 就是 Context[any, any]，
// fuego.ContextWithBody[B] 就是 Context[B, any]。

// ok 把只返回业务数据的 handler 适配成 Fuego 要的形状，成功响应的 message 为 "ok"。
func ok[B, P, T any](
	h func(fuego.Context[B, P]) (T, error),
) func(fuego.Context[B, P]) (response.Envelope[T], error) {
	return func(c fuego.Context[B, P]) (response.Envelope[T], error) {
		data, err := h(c)
		if err != nil {
			// 错误原样上抛：折叠状态码、5xx 记日志、以及 ErrResponseWritten（413 已写出）
			// 的短路，都在 response.Fail 里做，这里不再判一遍。
			return response.Envelope[T]{}, err
		}
		return response.OK(reqctx.RequestID(c.Request().Context()), data), nil
	}
}

// created 同 ok，成功响应的 message 为 "created"。
//
// 状态码由路由上的 OptionDefaultStatusCode 决定，与这里只负责文案的分工是刻意的：
// 改状态码要同时想到文档（路由选项），改文案只影响响应体。两者不一致时
// 线格式比对会失败——这条冗余由测试兜住，而不是靠人记住。
func created[B, P, T any](
	h func(fuego.Context[B, P]) (T, error),
) func(fuego.Context[B, P]) (response.Envelope[T], error) {
	return func(c fuego.Context[B, P]) (response.Envelope[T], error) {
		data, err := h(c)
		if err != nil {
			return response.Envelope[T]{}, err
		}
		return response.Created(reqctx.RequestID(c.Request().Context()), data), nil
	}
}
