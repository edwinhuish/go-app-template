package middleware

import "net/http"

// CORS 允许前端跨域访问。origins 为空时允许任意来源（仅适用于开发环境）。
func CORS(origins []string) func(http.Handler) http.Handler {
	allowAll := len(origins) == 0
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Origin,Content-Type,Authorization,X-Request-ID")
			// 跨域时自定义响应头默认对 JS 不可见，必须显式暴露，
			// 否则前端 res.headers.get('X-Request-ID') 恒为 null，排查问题时对不上日志。
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")

			// 预检直接应答，不进业务链路：它不该触发鉴权，也不该产生访问日志之外的行为。
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
