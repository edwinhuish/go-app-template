package service

import (
	"context"
	"log/slog"

	"example.com/app/internal/dto"
	"example.com/app/internal/model"
	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/role"
	"example.com/app/internal/repository"
	"example.com/app/internal/server/ctxs"
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
	actor, err := noteActor(ctx)
	if err != nil {
		return nil, err
	}
	q.Normalize()

	filter := repository.NoteFilter{Keyword: q.Keyword, OwnerID: q.OwnerID}
	// 非管理员的数据范围由身份决定，不由 query 决定：否则传一个 owner_id
	// 就能把「只能看自己的」这条规则降级成一句注释。管理员的 owner_id 仍当筛选条件。
	if !role.SeesAllData(actor.Role) {
		filter.OwnerID = actor.UserID
	}

	list, total, err := s.notes.Paginate(ctx,
		repository.Pagination{Page: int(q.Page), PageSize: int(q.PageSize)}, filter)
	if err != nil {
		return nil, err
	}

	items := make([]dto.NoteItem, len(list))
	for i, m := range list {
		items[i] = toNoteItem(&m)
	}
	return dto.NewPageResult(items, total, int(q.Page), int(q.PageSize)), nil
}

func (s *noteService) Get(ctx context.Context, id uint) (*dto.NoteItem, error) {
	m, err := s.findVisible(ctx, id)
	if err != nil {
		return nil, err
	}
	item := toNoteItem(m)
	return &item, nil
}

func (s *noteService) Create(
	ctx context.Context, req dto.NoteCreateRequest,
) (*dto.NoteItem, error) {
	actor, err := noteActor(ctx)
	if err != nil {
		return nil, err
	}

	// 预检重名是为了给出一句能定位的提示；真正的并发拦截靠数据库唯一索引
	// （database 包把约束错误统一翻成了 409）。
	exists, err := s.notes.ExistsByTitle(ctx, req.Title, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errs.ErrConflict.WithMsg("标题已存在")
	}

	// 创建者只取自请求上下文，不接受入参：允许客户端指定 owner 等于允许代替别人建数据。
	m := &model.Note{Title: req.Title, Content: req.Content, OwnerID: actor.UserID}

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
	if _, err := s.findVisible(ctx, id); err != nil {
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
	if _, err := s.findVisible(ctx, id); err != nil {
		return err
	}
	return s.notes.DeleteByID(ctx, id)
}

// noteActor 取当前身份。示例实体是纯业务数据，没有身份就没有数据范围可算，
// 所以这里 fail-closed：装配漏了鉴权中间件会表现为 401，而不是「看到全表」。
func noteActor(ctx context.Context) (*ctxs.Principal, error) {
	actor, ok := ctxs.ActorFrom(ctx)
	if !ok {
		return nil, errs.ErrUnauthorized
	}
	return actor, nil
}

// findVisible 读取记录并校验归属。管理员可见全部，其他角色只可见自己创建的。
//
// 越权时返回 404 而不是 403：403 等于确认「这条 id 确实存在，只是不归你」，
// 连续试 id 就能数出别人有多少条数据。对调用方来说这两种情况的结论都是「没有」。
func (s *noteService) findVisible(ctx context.Context, id uint) (*model.Note, error) {
	actor, err := noteActor(ctx)
	if err != nil {
		return nil, err
	}

	m, err := s.notes.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !role.SeesAllData(actor.Role) && m.OwnerID != actor.UserID {
		return nil, errs.ErrNotFound
	}
	return m, nil
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
