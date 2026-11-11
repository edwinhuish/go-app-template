// Package middleware 提供 HTTP 中间件。
//
// 一律写成标准库的 func(http.Handler) http.Handler：不绑定任何 router，
// 也不认识 gin / Fuego 的上下文类型，换框架时这一层可以原样搬走。
package middleware

import "net/http"

// Chain 把多个中间件串成一个，第一个参数在最外层（最先执行）。
//
// 这样读起来与调用顺序一致，不必像逐层包裹那样反过来想。
// 注意：框架如果自己逐层包裹中间件，顺序语义可能相反（Fuego 的
// globalMiddlewares 就是后者），所以串接统一走这里，不把顺序留在注册处。
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}
