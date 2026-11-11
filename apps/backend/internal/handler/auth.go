package handler

import (
	"github.com/go-fuego/fuego"

	"example.com/app/internal/dto"
	"example.com/app/internal/pkg/errs"
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
//
// 返回值是业务数据本身：信封、request_id 与状态码由 server 层的装饰器统一处理。
func (h *AuthHandler) Login(
	c fuego.ContextWithBody[dto.LoginRequest],
) (*dto.LoginResponse, error) {
	req := &dto.LoginRequest{}
	if err := bindJSON(c.Response(), c.Request(), req); err != nil {
		return nil, err
	}
	return h.auth.Login(c.Request().Context(), *req)
}

// Logout 处理 POST /auth/logout，删除服务端会话。
func (h *AuthHandler) Logout(c fuego.ContextNoBody) (dto.LogoutResult, error) {
	p, ok := service.ActorFrom(c.Request().Context())
	if !ok {
		// 路由上挂了鉴权中间件，走到这里说明装配漏了 Authorization 之外的东西；
		// 仍然返回 401 而不是空结构体，免得一个配置错误变成「静默成功」。
		return dto.LogoutResult{}, errs.ErrUnauthorized
	}

	if err := h.auth.Logout(c.Request().Context(), p.TokenID); err != nil {
		return dto.LogoutResult{}, err
	}
	return dto.LogoutResult{LoggedOut: true}, nil
}

// Profile 处理 GET /auth/profile，返回当前登录身份。
func (h *AuthHandler) Profile(c fuego.ContextNoBody) (dto.UserInfo, error) {
	p, ok := service.ActorFrom(c.Request().Context())
	if !ok {
		return dto.UserInfo{}, errs.ErrUnauthorized
	}

	return dto.UserInfo{
		ID:       p.UserID,
		Username: p.Username,
		Nickname: p.Nickname,
		Role:     p.Role,
	}, nil
}
