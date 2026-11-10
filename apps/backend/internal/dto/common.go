// Package dto 定义应用层的数据传输契约（请求 / 响应 / 查询条件）。
//
// service 层只依赖 dto，不感知 gin；handler 只做绑定与转发。
// dto 刻意不 import model：契约层与持久化层解耦，改表结构不会连带改接口。
package dto

const (
	// DefaultPageSize 是未指定 page_size 时的每页条数。
	DefaultPageSize = 20
	// MaxPageSize 是每页条数上限，防止一次拉全表。
	MaxPageSize = 200
)

// PageQuery 是通用分页参数，各列表查询结构体嵌入它即可。
type PageQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Keyword  string `form:"keyword"`
}

// Normalize 修正非法分页参数，避免越界与全表扫描。
func (q *PageQuery) Normalize() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = DefaultPageSize
	}
	if q.PageSize > MaxPageSize {
		q.PageSize = MaxPageSize
	}
}

// Offset 返回当前页偏移量。
func (q PageQuery) Offset() int { return (q.Page - 1) * q.PageSize }

// PageResult 是分页响应体。
type PageResult[T any] struct {
	List     []T   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// NewPageResult 构造分页响应；list 为 nil 时序列化成 []，前端不必判空。
func NewPageResult[T any](list []T, total int64, page, pageSize int) *PageResult[T] {
	if list == nil {
		list = make([]T, 0)
	}
	return &PageResult[T]{List: list, Total: total, Page: page, PageSize: pageSize}
}
