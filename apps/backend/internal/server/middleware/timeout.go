package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimeout 给每个请求的 context 加截止时间。
//
// 它**不**在超时后直接写 504：写响应的是 handler 那一侧（查库会因为 ctx 取消
// 立刻返回错误，response.Fail 再把它翻成 504）。这样做而不是「起个 goroutine
// 到点就写响应」，是因为后者会让 handler 继续跑在已写出的响应之后——
// 连接已经结束，后续写入既没人看得到，也可能与流式响应的写入打架。
//
// 真正让慢查询停下来的是 gorm 对 ctx 的支持：db.WithContext(ctx) 在 ctx 取消时
// 会中断驱动层的等待。所以仓储方法一律带 ctx，缺一处这一层就只能保证「不再等下去」。
func RequestTimeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if d <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
