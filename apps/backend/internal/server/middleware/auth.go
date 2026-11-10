// Package middleware 提供 HTTP 中间件。
package middleware

import (
	"github.com/gin-gonic/gin"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/service"
)

const principalKey = "principal"

// WithPrincipal 把当前登录身份写入请求上下文。
func WithPrincipal(c *gin.Context, p *service.Principal) {
	c.Set(principalKey, p)
}

// PrincipalFrom 取出当前登录身份。
func PrincipalFrom(c *gin.Context) (*service.Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return nil, false
	}
	p, ok := v.(*service.Principal)
	return p, ok
}

// Auth 校验 Bearer 令牌，并把身份注入上下文。
//
// 身份放两份：gin.Context 给 handler 与其它中间件用，request context 给 service 用
// （service 不认识 gin，只能从 context.Context 取）。
func Auth(auth service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			response.Fail(c, errs.ErrUnauthorized)
			return
		}

		p, err := auth.Verify(c.Request.Context(), token)
		if err != nil {
			response.Fail(c, err)
			return
		}

		WithPrincipal(c, p)
		c.Request = c.Request.WithContext(service.WithActor(c.Request.Context(), p))
		c.Next()
	}
}

// RequireAdmin 在 Auth 之后使用，限制仅管理员可访问。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := PrincipalFrom(c)
		if !ok {
			response.Fail(c, errs.ErrUnauthorized)
			return
		}
		if !p.IsAdmin() {
			response.Fail(c, errs.ErrForbidden)
			return
		}
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	const prefix = "Bearer "
	v := c.GetHeader("Authorization")
	if len(v) <= len(prefix) || v[:len(prefix)] != prefix {
		return ""
	}
	return v[len(prefix):]
}
