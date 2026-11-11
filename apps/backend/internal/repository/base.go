// Package repository 是数据访问层。
//
// 上层（service）只依赖本包导出的接口，不接触 gorm；
// 具体实现可以随时换成 ent / sqlc 而不影响业务代码。
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"example.com/app/internal/pkg/errs"
)

// Pagination 是数据访问层的分页入参，与 HTTP 参数解耦。
type Pagination struct {
	Page     int
	PageSize int
}

// 分页的兜底上限。对外契约里的默认值与上限定义在 dto（DefaultPageSize / MaxPageSize），
// 这里只在「不经过 HTTP 的调用」上兜底，两边数值要跟着一起改——这是刻意的冗余：
// 仓储是数据访问的最后一道闸，不能假设调用方一定做过 Normalize。
const (
	defaultPageSize = 20
	maxPageSize     = 200
)

func (p Pagination) normalize() Pagination {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = defaultPageSize
	}
	if p.PageSize > maxPageSize {
		p.PageSize = maxPageSize
	}
	return p
}

// Offset 返回当前页的偏移量。
func (p Pagination) Offset() int { return (p.Page - 1) * p.PageSize }

// Scope 是 gorm 条件构造器，把「查询条件」从泛型基类里解耦出去。
// 用类型别名而非命名类型，才能直接展开传给 gorm 的 Scopes(...)。
type Scope = func(*gorm.DB) *gorm.DB

// FieldUpdatedAt 是乐观锁的约定键。
//
// 调用方把「读到这条记录时的 updated_at」放进 fields，UpdateFields 会把它从
// SET 里摘出来当 WHERE 条件。service 层不需要改仓储接口，也不用记另一种方法名。
const FieldUpdatedAt = "updated_at"

// Base 是泛型仓储基类，承载所有实体的通用 CRUD。
// 具体仓储通过嵌入它复用代码，只实现自己独有的查询方法。
type Base[T any] struct {
	db *gorm.DB
}

// NewBase 构造泛型仓储基类。
func NewBase[T any](db *gorm.DB) *Base[T] { return &Base[T]{db: db} }

// conn 取这次操作该用的连接：ctx 里带着事务就用事务，否则用池子。
//
// 所有仓储方法都走它，所以「要不要进事务」由调用方在 TxManager 处决定，
// 仓储本身仍然无状态。反过来做（让仓储持有事务副本）就要在每个调用点
// 判断该拿哪个仓储，漏一处就是一次写到事务外面。
func (r *Base[T]) conn(ctx context.Context) *gorm.DB { return dbFor(ctx, r.db) }

// Create 插入一条记录。
func (r *Base[T]) Create(ctx context.Context, m *T) error {
	if err := r.conn(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("创建记录: %w", err)
	}
	return nil
}

// takeOptimisticLock 从 fields 里摘出版本条件，返回期望值与是否带了版本。
func takeOptimisticLock(fields map[string]any) (any, bool) {
	expected, ok := fields[FieldUpdatedAt]
	if !ok {
		return nil, false
	}
	// 摘掉它：这一列由 gorm 自己维护，不能拿来当要写入的值。
	delete(fields, FieldUpdatedAt)
	return expected, true
}

// UpdateFields 按字段更新，避免全量覆盖时误清空未提交的字段。
//
// fields 里带 FieldUpdatedAt 时做**条件更新**：两个人同时编辑同一条记录，
// 后提交的那个会被拒绝（409），而不是把前一个的改动静默盖掉。
func (r *Base[T]) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return errs.BadRequest("没有需要更新的字段")
	}

	q := r.conn(ctx).Model(new(T)).Where("id = ?", id)
	expected, hasLock := takeOptimisticLock(fields)
	if hasLock {
		if len(fields) == 0 {
			// 只传了版本号：没有要写的列。返回成功会让调用方以为改动生效了。
			return errs.BadRequest("没有需要更新的字段")
		}
		q = q.Where("updated_at = ?", expected)
	}

	// 下面把 RowsAffected 当「命中的行数」用，所以 MySQL 的连接串必须带
	// clientFoundRows（见 config.ResolvedDSN）。它默认返回的是「真正改动的行数」，
	// SET 的值与库里相同时为 0，会被误判成 404 / 409，而 SQLite 与 Postgres 是 200。
	res := q.Updates(fields)
	if res.Error != nil {
		return fmt.Errorf("更新记录 id=%d: %w", id, res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	if hasLock {
		// 条件没命中：记录还在，只是底稿被别人换过了；记录本身没了则是另一种情况。
		var count int64
		if err := r.conn(ctx).Model(new(T)).Where("id = ?", id).Count(&count).Error; err != nil {
			return fmt.Errorf("确认记录 id=%d 是否存在: %w", id, err)
		}
		if count > 0 {
			return errs.ErrConflict.WithMsg("这条记录已被他人修改，请刷新后重试")
		}
	}
	return errs.ErrNotFound
}

// DeleteByID 执行软删除，历史数据与追溯链不受影响。
func (r *Base[T]) DeleteByID(ctx context.Context, id uint) error {
	res := r.conn(ctx).Delete(new(T), id)
	if isSoftDeleteMarkerConflict(res.Error) {
		// 软删除标记是 Unix 毫秒，且被排进了「未删才唯一」的复合索引：同一毫秒内
		// 删掉两条同名记录会撞索引。换一个毫秒重试一次就够，不必把这个每秒最多
		// 出现一次的巧合暴露成 409——调用方看不出自己哪里做错了。
		time.Sleep(2 * time.Millisecond)
		res = r.conn(ctx).Delete(new(T), id)
	}
	if res.Error != nil {
		return fmt.Errorf("删除记录 id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return errs.ErrNotFound
	}
	return nil
}

// isSoftDeleteMarkerConflict 判断这次失败是不是「软删除标记撞了唯一索引」。
//
// 数据库层已经把约束错误统一翻成 errs.ErrConflict（见 infra/database 的回调），
// 所以这里按业务码判断，而不是 errors.Is(gorm.ErrDuplicatedKey)——那个值已经被换掉了。
func isSoftDeleteMarkerConflict(err error) bool {
	e, ok := errs.From(err)
	return ok && e.Code == errs.CodeConflict
}

// FindByID 按主键取一条记录。
func (r *Base[T]) FindByID(ctx context.Context, id uint) (*T, error) {
	var m T
	err := r.conn(ctx).First(&m, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询记录 id=%d: %w", id, err)
	}
	return &m, nil
}

// Exists 判断满足条件的记录是否存在。
//
// 不用 Count：Count 会忽略 Limit，实际扫的是全部命中行——一张十万行的表上
// 「判断标题是否重复」也会被算成全表统计。判存在只需要取一行。
func (r *Base[T]) Exists(ctx context.Context, scopes ...Scope) (bool, error) {
	var found []struct{ ID uint }
	err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Select("id").Limit(1).Find(&found).Error
	if err != nil {
		return false, fmt.Errorf("查询记录是否存在: %w", err)
	}
	return len(found) > 0, nil
}

// Count 统计满足条件的记录数。
func (r *Base[T]) Count(ctx context.Context, scopes ...Scope) (int64, error) {
	var count int64
	if err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("统计记录: %w", err)
	}
	return count, nil
}

// Paginate 返回当前页数据与总数（按 id 倒序）。
func (r *Base[T]) Paginate(ctx context.Context, p Pagination, scopes ...Scope) ([]T, int64, error) {
	return r.PaginateOrdered(ctx, p, "id DESC", scopes...)
}

// PaginateOrdered 与 Paginate 相同，但由调用方指定排序。
//
// 排序必须在这里传，不能写进 scope：gorm 合并 ORDER BY 时后写的会覆盖前面的，
// 而 Paginate 内部的 Order 在 scopes 之后执行——写在 scope 里的排序会被静默吃掉，
// 表现是「列表看着没排序」，但不是每次都看得出来。
func (r *Base[T]) PaginateOrdered(
	ctx context.Context, p Pagination, order string, scopes ...Scope,
) ([]T, int64, error) {
	p = p.normalize()

	// 计数与取数分别构建查询：复用同一个 *gorm.DB 会让 Count 的 statement 残留到 Find 上。
	var total int64
	if err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计记录: %w", err)
	}
	if total == 0 {
		return []T{}, 0, nil
	}

	var list []T
	err := r.conn(ctx).Model(new(T)).Scopes(scopes...).
		Order(order).
		Offset(p.Offset()).
		Limit(p.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询记录: %w", err)
	}
	return list, total, nil
}

// PaginateOrderedWithPreload 与 PaginateOrdered 相同，额外地预加载关联。
//
// 列表要带明细一起回吐时用它：逐条再查一次关联，会让一页 20 条变成 21 次往返。
func (r *Base[T]) PaginateOrderedWithPreload(
	ctx context.Context, p Pagination, order string, preloads []string, scopes ...Scope,
) ([]T, int64, error) {
	p = p.normalize()

	var total int64
	if err := r.conn(ctx).Model(new(T)).Scopes(scopes...).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计记录: %w", err)
	}
	if total == 0 {
		return []T{}, 0, nil
	}

	q := r.conn(ctx).Model(new(T)).Scopes(scopes...)
	for _, name := range preloads {
		q = q.Preload(name)
	}

	var list []T
	if err := q.Order(order).Offset(p.Offset()).Limit(p.PageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("查询记录: %w", err)
	}
	return list, total, nil
}
