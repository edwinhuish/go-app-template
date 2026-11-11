package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"example.com/app/internal/model"
)

// NoteFilter 是示例实体的列表筛选条件。
//
// 筛选条件用结构体传，而不是一串可选参数：加一个条件不必改所有调用点，
// 也不会出现 `List(ctx, "", 0, true, false)` 这种看不出含义的调用。
type NoteFilter struct {
	// Keyword 匹配标题，空串表示不过滤。
	Keyword string
	// OwnerID 为 0 表示不按创建者过滤。
	OwnerID uint
}

// NoteRepository 是示例实体的数据访问接口。
type NoteRepository interface {
	Create(ctx context.Context, m *model.Note) error
	FindByID(ctx context.Context, id uint) (*model.Note, error)
	UpdateFields(ctx context.Context, id uint, fields map[string]any) error
	DeleteByID(ctx context.Context, id uint) error
	Paginate(ctx context.Context, p Pagination, f NoteFilter) ([]model.Note, int64, error)
	ExistsByTitle(ctx context.Context, title string, excludeID uint) (bool, error)
}

type noteRepository struct {
	*Base[model.Note]
}

// NewNoteRepository 构造示例实体仓储。
func NewNoteRepository(db *gorm.DB) NoteRepository {
	return &noteRepository{Base: NewBase[model.Note](db)}
}

func (r *noteRepository) Paginate(
	ctx context.Context, p Pagination, f NoteFilter,
) ([]model.Note, int64, error) {
	// 条件写在这里而不是散到 service：service 不该知道字段名，
	// 换存储或加索引时只需要动这一个方法。
	scopes := make([]Scope, 0, 2)
	if f.Keyword != "" {
		// 关键词按字面包含匹配：% 与 _ 由 likeContains 转义，不让用户的输入变成通配符。
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where(likeContainsCondition("title"), likeContains(f.Keyword))
		})
	}
	if f.OwnerID != 0 {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("owner_id = ?", f.OwnerID)
		})
	}

	list, total, err := r.PaginateOrdered(ctx, p, "id DESC", scopes...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询示例列表: %w", err)
	}
	return list, total, nil
}

func (r *noteRepository) ExistsByTitle(ctx context.Context, title string, excludeID uint) (bool, error) {
	return r.Exists(ctx, func(db *gorm.DB) *gorm.DB {
		q := db.Where("title = ?", title)
		if excludeID != 0 {
			q = q.Where("id <> ?", excludeID)
		}
		return q
	})
}
