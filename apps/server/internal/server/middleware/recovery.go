package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/ctxs"
)

// writeTracker 只回答一个问题：响应头有没有写出去过。
//
// panic 的处理方式取决于它：还没写就补一个 500 信封，已经写了就绝不能再写第二段
// JSON——那会得到「半截业务响应 + 半截错误信封」，而客户端很可能把它当成完整响应。
type writeTracker struct {
	http.ResponseWriter
	wrote bool
}

func (w *writeTracker) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *writeTracker) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Unwrap 让 http.ResponseController 还能拿到底层 writer（Flush / Hijack 之类）。
func (w *writeTracker) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Recovery 捕获 panic，保证单个请求出错不会拖垮进程。
//
// 它在链上的位置是「业务链路的最外层、但访问日志之内」：往外一层是 AccessLog
// （panic 后它照样要留一条访问日志），往里是所有业务中间件与 handler。
func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tw := &writeTracker{ResponseWriter: w}
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				log.Error("请求 panic",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)

				// 别人主动要求中断连接（例如超时处理器判定无法继续）：照原样抛上去，
				// 不要把它包装成一个「服务器内部错误」。
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				if tw.wrote {
					// 已经写过响应头：状态码与部分响应体都发出去了，此时唯一安全的动作是
					// 断开连接。net/http 对 ErrAbortHandler 静默处理（只关连接、不打堆栈），
					// 客户端看到的是「响应不完整」，而不是一个看起来合法的残缺 JSON。
					panic(http.ErrAbortHandler)
				}

				response.Fail(tw, log, ctxs.RequestInfo(r),
					errs.Wrap(fmt.Errorf("%v", rec), errs.CodeInternal, errs.ErrInternal.Msg))
			}()
			next.ServeHTTP(tw, r)
		})
	}
}
