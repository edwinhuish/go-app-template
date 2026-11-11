package model

// Note 是一个**示例实体**，用来演示 model -> repository -> service -> handler
// 这条纵向链路怎么写。接入真实业务时，把这一层连同对应的 dto / repository /
// service / handler 一起替换掉，或者直接删掉。
type Note struct {
	Base
	// 遮蔽 Base 的同名字段：把 deleted_at 排进唯一索引，
	// 于是「同名只能有一条未删除记录」，删掉之后同名可以重建。
	//
	// 边界：删除标记是 Unix 毫秒，同一毫秒内删两条同名记录会撞这个唯一索引。
	// repository.Base.DeleteByID 会对这一种冲突换一个毫秒重试一次，所以调用方
	// 看不到 409；复合索引仍然只需要这两列。
	DeletedAt DeletedAt `gorm:"softDelete:milli;not null;default:0;uniqueIndex:uni_note_title,priority:2" json:"deleted_at,omitempty"`
	Title     string    `gorm:"size:128;not null;uniqueIndex:uni_note_title,priority:1" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	// OwnerID 是创建者，用于演示按身份过滤数据；不需要就把这一列去掉。
	OwnerID uint `gorm:"index" json:"owner_id"`
}

// TableName 固定表名。业务表统一用 biz_ 前缀，与系统表 sys_ 分开。
func (Note) TableName() string { return "biz_note" }
