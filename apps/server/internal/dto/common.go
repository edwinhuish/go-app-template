// Package dto 定义应用层的数据传输契约（请求 / 响应 / 查询条件）。
//
// service 层只依赖 dto，不感知 gin；handler 只做绑定与转发。
// dto 刻意不 import model：契约层与持久化层解耦，改表结构不会连带改接口。
package dto

import (
	"strconv"
	"strings"
)

const (
	// DefaultPageSize 是未指定 page_size 时的每页条数。
	DefaultPageSize = 20
	// MaxPageSize 是每页条数上限，防止一次拉全表。
	MaxPageSize = 200
)

// LenientInt 是查询参数里的整数：解析失败退化为 0，交给 Normalize 兜底。
//
// 为什么不让 handler 直接吞掉解码错误：gorilla/schema 遇到非数字会返回
// ConversionError，整个请求变成 400，而对外承诺是「非法值归 1 / 归默认」。
// 用 encoding.TextUnmarshaler 把容错收在类型上，Normalize 仍然只管范围。
type LenientInt int

// UnmarshalText 实现 encoding.TextUnmarshaler，永远不返回错误。
func (n *LenientInt) UnmarshalText(text []byte) error {
	v, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil {
		*n = 0
		return nil
	}
	*n = LenientInt(v)
	return nil
}

// PageQuery 是通用分页参数，各列表查询结构体嵌入它即可。
type PageQuery struct {
	Page     LenientInt `form:"page"`
	PageSize LenientInt `form:"page_size"`
	// Keyword 是否使用由各资源自己决定；用到时按字面包含匹配，% 与 _ 不是通配符。
	Keyword string `form:"keyword"`
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

// HealthStatus 是存活检查的响应体。
//
// Time 用字符串而不是 time.Time：健康检查是给机器看的，
// RFC3339 的字符串比再引入一层时间序列化规则更省事。
type HealthStatus struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}
