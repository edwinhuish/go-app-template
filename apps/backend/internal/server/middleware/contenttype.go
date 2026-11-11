package middleware

import "net/http"

// ContentTypeJSON 提前定下响应的 Content-Type。
//
// 为什么必须提前：net/http 一旦被 WriteHeader 就会把响应头刷出去，之后再改不再生效。
// 框架在控制器返回后才按 DefaultStatusCode 写状态码（201 之类），那时序列化器
// 再去设 Content-Type 已经晚了，net/http 会按响应体嗅探出 text/plain。
// 200 反而看不出问题——它没有显式 WriteHeader，是靠第一次 Write 隐式落地的。
//
// 只挂在已注册的路由上，所以 404 兜底与预检响应不受影响，它们各有自己的头。
func ContentTypeJSON() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			next.ServeHTTP(w, r)
		})
	}
}
