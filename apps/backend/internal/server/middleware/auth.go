package middleware

import (
	"net/http"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/ctxs"
	"example.com/app/internal/service"
)

// Auth 校验 Bearer 令牌，并把登录身份注入上下文。
//
// 本包只做「验证 + 注入」两件事，不定义 context key，也不提供读取函数。
// 身份的唯一存储处是 service.WithActor：service 层的业务规则（谁能看全部数据、
// 这条记录归谁）要读它，而本仓库不允许 service 反向 import server
// （依赖只能 handler → service），所以存储必须落在 service 那一侧。
//
// 读取由使用方各自负责：handler 用 service.ActorFrom，service 直接用 ActorFrom。
// 中间件如果顺手再存一份自己的 key，同一个请求里就有两个身份来源，
// 忘写其中一个的表现是「身份凭空消失」——这类 bug 只能靠运行时发现。
func Auth(auth service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				response.Fail(w, nil, ctxs.Info(r), errs.ErrUnauthorized)
				return
			}

			p, err := auth.Verify(r.Context(), token)
			if err != nil {
				response.Fail(w, nil, ctxs.Info(r), err)
				return
			}

			next.ServeHTTP(w, r.WithContext(service.WithActor(r.Context(), p)))
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
			p, ok := service.ActorFrom(r.Context())
			if !ok {
				response.Fail(w, nil, ctxs.Info(r), errs.ErrUnauthorized)
				return
			}
			if !p.IsAdmin() {
				response.Fail(w, nil, ctxs.Info(r), errs.ErrForbidden)
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
