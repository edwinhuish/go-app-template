package handler

import (
	"github.com/go-fuego/fuego"

	"example.com/app/internal/dto"
	"example.com/app/internal/service"
)

// NoteHandler 是示例实体的 HTTP 适配层。
//
// 每个方法只做三件事：解析参数、调 service、返回业务数据或错误。
// 信封、request_id、状态码由 server 层的装饰器统一处理（internal/server/decorate.go），
// 所以这里看不到 response.* 的痕迹。
type NoteHandler struct {
	notes service.NoteService
}

// NewNoteHandler 构造示例 handler。
func NewNoteHandler(notes service.NoteService) *NoteHandler {
	return &NoteHandler{notes: notes}
}

// List 处理 GET /notes。
func (h *NoteHandler) List(c fuego.ContextNoBody) (*dto.PageResult[dto.NoteItem], error) {
	var q dto.NoteListQuery
	if err := bindQuery(c.Request(), &q); err != nil {
		return nil, err
	}
	return h.notes.List(c.Request().Context(), q)
}

// Get 处理 GET /notes/{id}。
func (h *NoteHandler) Get(c fuego.ContextNoBody) (*dto.NoteItem, error) {
	id, err := parseID(c.PathParam("id"))
	if err != nil {
		return nil, err
	}
	return h.notes.Get(c.Request().Context(), id)
}

// Create 处理 POST /notes。状态码 201 由路由上的 OptionDefaultStatusCode 声明。
func (h *NoteHandler) Create(
	c fuego.ContextWithBody[dto.NoteCreateRequest],
) (*dto.NoteItem, error) {
	req, err := readBody(c)
	if err != nil {
		return nil, err
	}
	return h.notes.Create(c.Request().Context(), req)
}

// Update 处理 PUT /notes/{id}。
func (h *NoteHandler) Update(
	c fuego.ContextWithBody[dto.NoteUpdateRequest],
) (*dto.NoteItem, error) {
	id, err := parseID(c.PathParam("id"))
	if err != nil {
		return nil, err
	}

	req, err := readBody(c)
	if err != nil {
		return nil, err
	}
	return h.notes.Update(c.Request().Context(), id, req)
}

// Delete 处理 DELETE /notes/{id}（软删除）。
func (h *NoteHandler) Delete(c fuego.ContextNoBody) (dto.NoteDeleted, error) {
	id, err := parseID(c.PathParam("id"))
	if err != nil {
		return dto.NoteDeleted{}, err
	}

	if err := h.notes.Delete(c.Request().Context(), id); err != nil {
		return dto.NoteDeleted{}, err
	}
	return dto.NoteDeleted{Deleted: true}, nil
}
