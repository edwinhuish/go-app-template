// Package model 定义持久化模型。
//
// 这一层只描述数据形状，不包含业务规则，也不依赖上层的 service / handler。
// 表名统一显式声明在 TableName() 里，便于按前缀分组（sys_ / biz_）。
package model

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// DeletedAt 是软删除标记的类型别名（gorm.io/plugin/soft_delete）。
//
// 用别名而不是让每个模型文件都 import 插件：模型里只写 `DeletedAt DeletedAt`，
// 真要换实现时改这一处。
//
// 为什么不用 gorm.DeletedAt（内部是 sql.NullTime）：它把「未删除」表示成 NULL，
// 而 NULL 在唯一索引里不参与比较——两行未删记录各自是 NULL，互不冲突，
// 「同一业务键只能有一行未删除」这条规则在数据库层根本落不下去。
// 插件用整数标记（未删除 = 0，删除 = Unix 毫秒），可以直接进复合唯一索引。
type DeletedAt = soft_delete.DeletedAt

// EnableStatus 表示一条主数据是否还能被业务引用。
//
// 它只回答「还能不能用」，不表达业务流转状态；那是另一套语义，
// 不要让一张表同时承载两种 status。
type EnableStatus int8

const (
	// EnableStatusOff 停用：记录保留，但不出现在可选列表里。
	EnableStatusOff EnableStatus = 0
	// EnableStatusOn 启用。
	EnableStatusOn EnableStatus = 1
)

// Base 是所有业务表的公共字段。
// 软删除只置标记：删除不抹数据，历史与追溯链不会断。
type Base struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// 未删除为 0，删除时写 Unix 毫秒（列类型是整数，不是时间戳）。
	//
	// 带业务唯一键的模型会在自己的结构体里再声明一次同名字段，并把 deleted_at
	// 排进复合唯一索引——外层字段遮蔽这里这个，gorm 解析时只认外层那份。
	//
	// 序列化用 omitempty：未删除时值是 0 被省掉，只有已删除的行才带出这个字段。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;index" json:"deleted_at,omitempty"`
}
