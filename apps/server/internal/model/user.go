package model

// User 是系统账号。
type User struct {
	Base
	// 遮蔽 Base 里的同名字段，把它排进「未删才唯一」的复合索引。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_user_username,priority:2" json:"deleted_at,omitempty"`
	Username  string    `gorm:"size:64;uniqueIndex:uni_user_username,priority:1;not null" json:"username"`
	// Password 存 bcrypt 摘要，任何情况下都不出口。
	Password string `gorm:"size:128;not null" json:"-"`
	Nickname string `gorm:"size:64" json:"nickname"`
	// Role 的取值定义在 pkg/role：这里只是一个字符串列，判断逻辑不属于持久化层。
	Role   string       `gorm:"size:32;not null;default:user;index" json:"role"`
	Status EnableStatus `gorm:"not null;default:1;index" json:"status"`
}

// TableName 固定表名，避免改名时连带迁移数据。
func (User) TableName() string { return "sys_user" }

// IsEnabled 报告账号是否可用。
func (u *User) IsEnabled() bool { return u.Status == EnableStatusOn }
