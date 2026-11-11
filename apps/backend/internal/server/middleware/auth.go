package middleware

import (
	"context"
	"net/http"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/service"
)

// principalKey 是登录身份在请求上下文里的键。
type principalKey struct{}

// WithPrincipal 把当前登录身份写入上下文，供 handler 取 TokenID、角色等字段。
func WithPrincipal(ctx context.Context, p *service.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom 取出当前登录身份。
func PrincipalFrom(ctx context.Context) (*service.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*service.Principal)
	return p, ok && p != nil
}

// Auth 校验 Bearer 令牌，并把身份注入上下文。
//
// 身份放两份：context 里的 principal 给 handler 用（登录接口之外的接口都要知道
// 自己是谁），service.WithActor 那份给 service 用（service 不认识 HTTP，
// 只能从 context.Context 取）。
func Auth(auth service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				response.Fail(w, r, nil, errs.ErrUnauthorized)
				return
			}

			p, err := auth.Verify(r.Context(), token)
			if err != nil {
				response.Fail(w, r, nil, err)
				return
			}

			ctx := WithPrincipal(r.Context(), p)
			ctx = service.WithActor(ctx, p)
			next.ServeHTTP(w, r.WithContext(ctx))
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
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				response.Fail(w, r, nil, errs.ErrUnauthorized)
				return
			}
			if !p.IsAdmin() {
				response.Fail(w, r, nil, errs.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) string {
	const prefix = "Bearer "
	v := r.Header.Get("Authorization")
	if len(v) <= len(prefix) || v[:len(prefix)] != prefix {
		return ""
	}
	return v[len(prefix):]
}
