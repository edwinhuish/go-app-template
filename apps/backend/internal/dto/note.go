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
	Title string `json:"title" binding:"required,min=1,max=128"`
	// Content 不是必填（binding 里没有 required），所以加 omitempty 让文档里的
	// required 与校验语义一致。这类只在请求方向使用的结构体不会被序列化成响应，
	// omitempty 不改变对外线格式。
	Content string `json:"content,omitempty" binding:"max=20000"`
}

// NoteUpdateRequest 是更新入参。
//
// UpdatedAt 是乐观锁版本：前端把读到这条记录时的 updated_at 原样回传，
// 服务端据此判断「你动手时看到的那份底稿，现在还是不是那份底稿」。
// 不传则退化为最后写入者获胜——要不要强制，由各业务自己定。
type NoteUpdateRequest struct {
	Title string `json:"title" binding:"required,min=1,max=128"`
	// Content 与 UpdatedAt 都不是必填，同理加 omitempty 对齐文档里的 required。
	// UpdatedAt 是乐观锁的期望版本，不传就表示「不做并发校验」。
	Content   string     `json:"content,omitempty" binding:"max=20000"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// NoteDeleted 是删除成功的响应体。
//
// 用具名类型而不是 map[string]any：map 在 OpenAPI 里只能生成一个空对象，
// 前端在文档里看不到 deleted 这个字段，也就没法据此写类型。
type NoteDeleted struct {
	Deleted bool `json:"deleted"`
}
