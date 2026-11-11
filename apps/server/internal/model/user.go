package model

// 系统角色。骨架期用固定枚举，后续要细粒度权限时再换成 role / permission 表。
const (
	// RoleAdmin 可访问全部数据与系统设置类接口（账号维护等）。
	RoleAdmin string = "admin"
	// RoleUser 普通用户：只能访问自己的数据。
	RoleUser string = "user"
)

// RoleSeesAllData 报告该角色能否看到全部数据。
//
// 认不出的角色一律按「只看自己的」处理：角色是字符串，写错一个字母不会报错，
// 而把它当成「能看全部」就等于一次拼写错误变成了越权。
func RoleSeesAllData(role string) bool { return role == RoleAdmin }

// User 是系统账号。
type User struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_user_username,priority:2" json:"deleted_at,omitempty"`
	Username  string    `gorm:"size:64;uniqueIndex:uni_user_username,priority:1;not null" json:"username"`
	// Password 存 bcrypt 摘要，任何情况下都不出口。
	Password string       `gorm:"size:128;not null" json:"-"`
	Nickname string       `gorm:"size:64" json:"nickname"`
	Role     string       `gorm:"size:32;not null;default:user;index" json:"role"`
	Status   EnableStatus `gorm:"not null;default:1;index" json:"status"`
}

// TableName 固定表名，避免改名时连带迁移数据。
func (User) TableName() string { return "sys_user" }

// IsEnabled 报告账号是否可用。
func (u *User) IsEnabled() bool { return u.Status == EnableStatusOn }

// IsAdmin 报告账号是否为管理员。
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }
