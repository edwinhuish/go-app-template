package handler

import (
	"github.com/go-fuego/fuego"

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
//
// 请求体类型写在泛型参数上（ContextWithBody[dto.LoginRequest]）：
// 框架据此生成 OpenAPI 的 requestBody schema，前端在文档里能看到字段与约束。
// 真正的解析与校验仍走 bindJSON —— 框架自带的绑定会把错误折成它自己的文案。
func (h *AuthHandler) Login(
	c fuego.ContextWithBody[dto.LoginRequest],
) (response.Envelope[*dto.LoginResponse], error) {
	req := &dto.LoginRequest{}
	if err := bindJSON(c.Response(), c.Request(), req); err != nil {
		return fail[*dto.LoginResponse](err)
	}

	res, err := h.auth.Login(c.Request().Context(), *req)
	if err != nil {
		return fail[*dto.LoginResponse](err)
	}
	return response.OK(requestID(c.Request()), res), nil
}

// Logout 处理 POST /auth/logout，删除服务端会话。
func (h *AuthHandler) Logout(
	c fuego.ContextNoBody,
) (response.Envelope[dto.LogoutResult], error) {
	p, ok := middleware.PrincipalFrom(c.Request().Context())
	if !ok {
		return fail[dto.LogoutResult](errs.ErrUnauthorized)
	}

	if err := h.auth.Logout(c.Request().Context(), p.TokenID); err != nil {
		return fail[dto.LogoutResult](err)
	}
	return response.OK(requestID(c.Request()), dto.LogoutResult{LoggedOut: true}), nil
}

// Profile 处理 GET /auth/profile，返回当前登录身份。
func (h *AuthHandler) Profile(c fuego.ContextNoBody) (response.Envelope[dto.UserInfo], error) {
	p, ok := middleware.PrincipalFrom(c.Request().Context())
	if !ok {
		return fail[dto.UserInfo](errs.ErrUnauthorized)
	}

	return response.OK(requestID(c.Request()), dto.UserInfo{
		ID:       p.UserID,
		Username: p.Username,
		Nickname: p.Nickname,
		Role:     p.Role,
	}), nil
}
