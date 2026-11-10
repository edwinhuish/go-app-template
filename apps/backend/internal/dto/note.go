package dto

import "time"

// NoteItem 是示例实体的响应体。
type NoteItem struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	OwnerID   uint      `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NoteListQuery 是列表查询参数。嵌入 PageQuery 后 form 标签会被一并解析。
type NoteListQuery struct {
	PageQuery
	OwnerID uint `form:"owner_id"`
}

// NoteCreateRequest 是新建入参。
type NoteCreateRequest struct {
	Title   string `json:"title" binding:"required,min=1,max=128"`
	Content string `json:"content" binding:"max=20000"`
}

// NoteUpdateRequest 是更新入参。
//
// UpdatedAt 是乐观锁版本：前端把读到这条记录时的 updated_at 原样回传，
// 服务端据此判断「你动手时看到的那份底稿，现在还是不是那份底稿」。
// 不传则退化为最后写入者获胜——要不要强制，由各业务自己定。
type NoteUpdateRequest struct {
	Title     string     `json:"title" binding:"required,min=1,max=128"`
	Content   string     `json:"content" binding:"max=20000"`
	UpdatedAt *time.Time `json:"updated_at"`
}
