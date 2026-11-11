package middleware

import (
	"net/http"

	"example.com/app/internal/pkg/response"
)

// BodyLimit 给请求体设一个上限，超了直接 413。
//
// 为什么需要它：JSON 解析会把整个请求体读进内存，而 Go 默认没有上限——
// 一个几 GB 的 POST 足以把进程的内存吃光，且这类请求只需要一个 curl 循环，
// 不需要任何账号（登录接口也在保护范围内）。
//
// 业务上如果另有更小的上限（上传 10MB 之类），两层都要留：
// 业务上限决定「允许多大的文件」，这一层决定「允许多大的请求」。
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				// 提前用 Content-Length 拒一次：能省掉把数据读进来再丢掉。
				if r.ContentLength > maxBytes {
					response.PayloadTooLarge(w, maxBytes)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
