package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"example.com/app/internal/server/ctxs"
)

// statusWriter 记录响应状态码，供访问日志使用。
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap 让 http.ResponseController 还能拿到底层 writer（Flush / Hijack 之类）。
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// AccessLog 输出结构化访问日志，request_id 与响应体中的值一致。
//
// 不打请求体：它可能带密码、令牌与个人信息，日志往往比数据库更容易被拿走。
func AccessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}

			// 用 defer 记这一行：handler panic 时它照样要出现。
			// 写在 next.ServeHTTP 之后的话，panic 会直接跳过它，现象是「报了 panic
			// 但访问日志里没有这次请求」，排查时容易误判成请求根本没到服务端。
			defer func() {
				status := sw.status
				if status == 0 {
					// 处理函数什么都没写：按 200 记，与 net/http 最终的隐含状态码一致。
					status = http.StatusOK
				}
				log.Info("http 请求",
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"latency_ms", time.Since(start).Milliseconds(),
					"ip", clientIP(r),
					"request_id", ctxs.RequestID(r.Context()),
				)
			}()

			next.ServeHTTP(sw, r)
		})
	}
}

// clientIP 取调用方地址，顺序与常见的反代约定一致。
//
// 这几个头都可以被客户端伪造，所以它只用于日志与限流这类「大致对就行」的场合；
// 真正的鉴权判断不要依赖它。
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i >= 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
