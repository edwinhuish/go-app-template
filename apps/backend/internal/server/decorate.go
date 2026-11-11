package server

import (
	"github.com/go-fuego/fuego"

	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/ctxs"
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

// ok 把只返回业务数据的 handler 适配成 Fuego 要的形状，成功响应的 message 恒为 "ok"。
//
// 只有这一个装饰器：成功响应的文案不承载任何语义（客户端按 code 判断成败），
// 201 与 200 的区别由路由上的 OptionDefaultStatusCode 表达。多一个 created
// 装饰器只会多一处「状态码与文案要不要同步」的冗余。
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
		return response.OK(ctxs.RequestID(c.Request().Context()), data), nil
	}
}
