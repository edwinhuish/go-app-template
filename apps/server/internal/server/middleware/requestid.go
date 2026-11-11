package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"example.com/app/internal/server/ctxs"
)

// HeaderRequestID 是请求 ID 的响应头（同时接受上游传进来的同名请求头）。
const HeaderRequestID = "X-Request-ID"

// maxRequestIDLen 限制透传进来的请求 ID 长度。它会原样进日志与响应头，
// 一个几兆的值能把一行日志撑爆，也能被用来在日志里伪造记录边界。
const maxRequestIDLen = 64

// RequestID 透传上游请求 ID，没有则生成一个，便于日志串联。
//
// 值同时写进响应头与请求 context：handler 用它填响应体的 request_id，
// service 与日志用它串同一次调用。存储本身由 ctxs 提供——中间件只负责
// 「算出来 + 放进去」，不定义存哪儿。
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			// 上游传来的值只当「建议」：格式与长度不合规就自己生成一个，
			// 而不是把不可信内容搬进响应头与日志。
			if !validRequestID(id) {
				id = newRequestID()
			}
			w.Header().Set(HeaderRequestID, id)
			next.ServeHTTP(w, r.WithContext(ctxs.WithRequestID(r.Context(), id)))
		})
	}
}

// validRequestID 只接受可安全写进日志与响应头的字符集。
func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > maxRequestIDLen {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
