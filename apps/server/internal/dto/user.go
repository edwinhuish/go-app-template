package dto

// UserItem 是账号列表的一行：账号信息 + 状态。
//
// 嵌入 UserInfo 而不是把四个字段再列一遍：登录响应里的 user、Profile 的返回体、
// 列表里的每一行是同一个形状，分头声明的结果是加一个字段时漏掉一处，
// 同一个账号在两处接口里长得不一样，而编译不会报错。
type UserItem struct {
	UserInfo
	// Status 1 启用 / 0 停用。
	Status int8 `json:"status"`
}
