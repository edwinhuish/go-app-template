package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
)

// Recovery 捕获 panic，保证单个请求出错不会拖垮进程。
//
// 顺序上它必须排在最外层：一旦 panic 发生在别的中间件里，
// 只有最外层还能把响应写出去，否则客户端只会看到连接被断开。
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("请求 panic",
						"panic", rec,
						"method", r.Method,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
					response.Fail(w, log, reqInfo(r),
						errs.Wrap(fmt.Errorf("%v", rec), errs.CodeInternal, "服务器内部错误"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
