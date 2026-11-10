package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
)

// Recovery 捕获 panic，保证单个请求出错不会拖垮进程。
//
// 顺序上它必须排在最外层：一旦 panic 发生在别的中间件里，
// 只有最外层还能把响应写出去，否则客户端只会看到连接被断开。
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("请求 panic",
					"panic", r,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"stack", string(debug.Stack()),
				)
				response.Fail(c, errs.Wrap(fmt.Errorf("%v", r), errs.CodeInternal, "服务器内部错误"))
			}
		}()
		c.Next()
	}
}
