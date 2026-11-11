package middleware

import (
	"net/http"
	"strings"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/ctxs"
	"example.com/app/internal/service"
)

// Auth 校验 Bearer 令牌，并把登录身份注入上下文。
//
// 本包只做「验证 + 注入」两件事：key 与类型都在 ctxs（ctxs.WithActor / ctxs.ActorFrom），
// 中间件不定义 key、也不提供读取函数。顺手再存一份自己的 key，同一个请求里就有两个
// 身份来源，忘写其中一个的表现是「身份凭空消失」——这类 bug 只能靠运行时发现。
func Auth(auth service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				response.Fail(w, nil, ctxs.RequestInfo(r), errs.ErrUnauthorized)
				return
			}

			p, err := auth.Verify(r.Context(), token)
			if err != nil {
				response.Fail(w, nil, ctxs.RequestInfo(r), err)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctxs.WithActor(r.Context(), p)))
		})
	}
}

// RequireAdmin 在 Auth 之后使用，限制仅管理员可访问。
//
// 权限判断放在中间件而不是 handler 里：加一条同类接口时，
// 忘了包这一层会立刻从代码审查与「无令牌应 401」的用例里看出来。
func RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := ctxs.ActorFrom(r.Context())
			if !ok {
				response.Fail(w, nil, ctxs.RequestInfo(r), errs.ErrUnauthorized)
				return
			}
			if !p.IsAdmin() {
				response.Fail(w, nil, ctxs.RequestInfo(r), errs.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) string {
	// auth-scheme 大小写不敏感（RFC 7235）：只认 "Bearer " 会把 "bearer xxx"
	// 判成未登录，而这类写法在各类 SDK 与代理里都出现过。
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
