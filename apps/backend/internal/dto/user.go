package dto

// UserItem 是账号列表的一行。
type UserItem struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
	// Status 1 启用 / 0 停用。
	Status int8 `json:"status"`
}
