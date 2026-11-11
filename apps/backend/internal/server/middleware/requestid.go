package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"example.com/app/internal/server/ctxs"
)

// HeaderRequestID 是请求 ID 的响应头（同时接受上游传进来的同名请求头）。
const HeaderRequestID = "X-Request-ID"

// RequestID 透传上游请求 ID，没有则生成一个，便于日志串联。
//
// 值同时写进响应头与请求 context：handler 用它填响应体的 request_id，
// service 与日志用它串同一次调用。存储本身由 ctxs 提供——中间件只负责
// 「算出来 + 放进去」，不定义存哪儿。
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			if id == "" {
				id = newRequestID()
			}
			w.Header().Set(HeaderRequestID, id)
			next.ServeHTTP(w, r.WithContext(ctxs.WithRequestID(r.Context(), id)))
		})
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
