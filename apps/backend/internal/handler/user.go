package handler

import (
	"github.com/gin-gonic/gin"

	"example.com/app/internal/pkg/response"
	"example.com/app/internal/service"
)

// UserHandler 是账号目录的 HTTP 适配层。
type UserHandler struct {
	users service.UserService
}

// NewUserHandler 构造账号 handler。
func NewUserHandler(users service.UserService) *UserHandler {
	return &UserHandler{users: users}
}

// List 处理 GET /users。路由上已挂了 RequireAdmin，越权由中间件拦住。
func (h *UserHandler) List(c *gin.Context) {
	res, err := h.users.List(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}
