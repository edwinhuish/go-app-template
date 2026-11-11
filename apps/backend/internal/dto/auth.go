package dto

// LoginRequest 是登录入参。
type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

// UserInfo 是可以对外返回的账号信息（永不包含密码）。
type UserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

// LoginResponse 是登录成功的响应体。
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt int64     `json:"expires_at"` // Unix 秒
	User      *UserInfo `json:"user"`
}

// LogoutResult 是登出成功的响应体。
type LogoutResult struct {
	LoggedOut bool `json:"logged_out"`
}
