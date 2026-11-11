// Package role 定义账号角色，以及「这个角色能不能看全部数据」这一条判断。
//
// 为什么不放在 model：判断角色的有两处——鉴权中间件（HTTP 层）与 service。
// 让 HTTP 层 import model 只是为比较一个字符串，却会把 ORM 拖进传输层的依赖图，
// 换持久化实现时连 ctxs 都要跟着改。model 只负责「Role 是个字符串列」。
package role

const (
	// Admin 可访问全部数据与系统设置类接口（账号维护等）。
	Admin = "admin"
	// User 普通用户：只能访问自己的数据。
	User = "user"
)

// SeesAllData 报告该角色能否看到全部数据。
//
// 认不出的角色一律按「只看自己的」处理：角色是字符串，写错一个字母不会报错，
// 而把它当成「能看全部」就等于一次拼写错误变成了越权。
func SeesAllData(r string) bool { return r == Admin }
