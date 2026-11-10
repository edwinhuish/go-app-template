package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"example.com/app/internal/pkg/response"
)

// AccessLog 输出结构化访问日志，request_id 与响应体中的值一致。
//
// 不打请求体：它可能带密码、令牌与个人信息，日志往往比数据库更容易被拿走。
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		rid, _ := c.Get(response.RequestIDKey)
		log.Info("http 请求",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", rid,
		)
	}
}
