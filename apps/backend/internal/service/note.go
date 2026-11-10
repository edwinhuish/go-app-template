package service

import (
	"context"
	"log/slog"

	"example.com/app/internal/dto"
	"example.com/app/internal/model"
	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/repository"
)

// NoteService 是示例实体的业务接口。
//
// 接口而不是具体类型：handler 只依赖它，测试里可以换成假实现，
// 将来拆服务时也只需要换装配。
type NoteService interface {
	List(ctx context.Context, q dto.NoteListQuery) (*dto.PageResult[dto.NoteItem], error)
	Get(ctx context.Context, id uint) (*dto.NoteItem, error)
	Create(ctx context.Context, req dto.NoteCreateRequest) (*dto.NoteItem, error)
	Update(ctx context.Context, id uint, req dto.NoteUpdateRequest) (*dto.NoteItem, error)
	Delete(ctx context.Context, id uint) error
}

// NoteOption 是示例服务的可选依赖。
type NoteOption = Option[noteService]

// WithNoteLogger 注入 logger。
func WithNoteLogger(l *slog.Logger) NoteOption {
	return func(s *noteService) { s.log = l }
}

type noteService struct {
	notes repository.NoteRepository
	log   *slog.Logger
}

// NewNoteService 构造示例服务。
func NewNoteService(notes repository.NoteRepository, opts ...NoteOption) NoteService {
	s := &noteService{notes: notes, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *noteService) List(
	ctx context.Context, q dto.NoteListQuery,
) (*dto.PageResult[dto.NoteItem], error) {
	q.Normalize()

	list, total, err := s.notes.Paginate(ctx,
		repository.Pagination{Page: q.Page, PageSize: q.PageSize},
		repository.NoteFilter{Keyword: q.Keyword, OwnerID: q.OwnerID})
	if err != nil {
		return nil, err
	}

	items := make([]dto.NoteItem, len(list))
	for i, m := range list {
		items[i] = toNoteItem(&m)
	}
	return dto.NewPageResult(items, total, q.Page, q.PageSize), nil
}

func (s *noteService) Get(ctx context.Context, id uint) (*dto.NoteItem, error) {
	m, err := s.notes.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	item := toNoteItem(m)
	return &item, nil
}

func (s *noteService) Create(
	ctx context.Context, req dto.NoteCreateRequest,
) (*dto.NoteItem, error) {
	// 预检重名是为了给出一句能定位的提示；真正的并发拦截靠数据库唯一索引
	// （database 包把约束错误统一翻成了 409）。
	exists, err := s.notes.ExistsByTitle(ctx, req.Title, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errs.ErrConflict.WithMsg("标题已存在")
	}

	m := &model.Note{Title: req.Title, Content: req.Content}
	// 创建者取自请求上下文；内部调用没有身份时留 0，由业务自行决定是否允许。
	if actor, ok := ActorFrom(ctx); ok {
		m.OwnerID = actor.UserID
	}

	if err := s.notes.Create(ctx, m); err != nil {
		return nil, err
	}

	s.log.Info("创建示例记录", "id", m.ID, "owner_id", m.OwnerID)
	item := toNoteItem(m)
	return &item, nil
}

func (s *noteService) Update(
	ctx context.Context, id uint, req dto.NoteUpdateRequest,
) (*dto.NoteItem, error) {
	if _, err := s.notes.FindByID(ctx, id); err != nil {
		return nil, err
	}

	exists, err := s.notes.ExistsByTitle(ctx, req.Title, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errs.ErrConflict.WithMsg("标题已存在")
	}

	fields := map[string]any{
		"title":   req.Title,
		"content": req.Content,
	}
	// 带上版本号就走条件更新：底稿被别人换过时返回 409，而不是静默覆盖。
	if req.UpdatedAt != nil {
		fields[repository.FieldUpdatedAt] = *req.UpdatedAt
	}

	if err := s.notes.UpdateFields(ctx, id, fields); err != nil {
		return nil, err
	}

	// 重新读一次：updated_at 由数据库写入，直接返回入参会让前端拿着旧版本号。
	m, err := s.notes.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	item := toNoteItem(m)
	return &item, nil
}

func (s *noteService) Delete(ctx context.Context, id uint) error {
	return s.notes.DeleteByID(ctx, id)
}

// toNoteItem 把持久化模型转成对外契约。字段映射集中在一处，
// 以后加字段只需要改这里，不必翻遍每个方法。
func toNoteItem(m *model.Note) dto.NoteItem {
	return dto.NoteItem{
		ID:        m.ID,
		Title:     m.Title,
		Content:   m.Content,
		OwnerID:   m.OwnerID,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
