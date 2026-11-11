package dto

// LoginRequest 是登录入参。
//
// 这里只校验「非空 + 长度上限」，不校验最短长度：密码强度是注册/改密时的规则，
// 搬到登录接口上会让存量账号（导入的、历史遗留的五位密码）永远登不上，返回的
// 还是「参数错误」这种与密码本身无关的提示。长度上限保留，避免把超长串喂给 bcrypt。
type LoginRequest struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=64"`
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
