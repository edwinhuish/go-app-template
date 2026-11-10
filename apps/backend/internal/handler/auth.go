package handler

import (
	"github.com/gin-gonic/gin"

	"example.com/app/internal/dto"
	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/middleware"
	"example.com/app/internal/service"
)

// AuthHandler 处理登录、登出与当前身份查询。
type AuthHandler struct {
	auth service.AuthService
}

// NewAuthHandler 构造鉴权 handler。
func NewAuthHandler(auth service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Login 处理 POST /auth/login。
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !bindJSON(c, &req) {
		return
	}

	res, err := h.auth.Login(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Logout 处理 POST /auth/logout，删除服务端会话。
func (h *AuthHandler) Logout(c *gin.Context) {
	p, ok := middleware.PrincipalFrom(c)
	if !ok {
		response.Fail(c, errs.ErrUnauthorized)
		return
	}
	if err := h.auth.Logout(c.Request.Context(), p.TokenID); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"logged_out": true})
}

// Profile 处理 GET /auth/profile，返回当前登录身份。
func (h *AuthHandler) Profile(c *gin.Context) {
	p, ok := middleware.PrincipalFrom(c)
	if !ok {
		response.Fail(c, errs.ErrUnauthorized)
		return
	}
	response.OK(c, dto.UserInfo{
		ID:       p.UserID,
		Username: p.Username,
		Nickname: p.Nickname,
		Role:     p.Role,
	})
}
