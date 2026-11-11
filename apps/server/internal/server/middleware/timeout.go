package middleware

import (
	"context"
	"net/http"
	"time"
)

// RequestTimeout 给每个请求的 context 加截止时间。
//
// 它**不**在超时后直接写 504：写响应的是 handler 那一侧（查库会因为 ctx 取消
// 立刻返回错误，错误序列化器再把它翻成 504）。这样做而不是「起个 goroutine
// 到点就写响应」，是因为后者会让 handler 继续跑在已写出的响应之后——
// 连接已经结束，后续写入既没人看得到，也可能与流式响应的写入打架。
//
// 真正让慢查询停下来的是 gorm 对 ctx 的支持：db.WithContext(ctx) 在 ctx 取消时
// 会中断驱动层的等待。所以仓储方法一律带 ctx，缺一处这一层就只能保证「不再等下去」。
func RequestTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if d <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
