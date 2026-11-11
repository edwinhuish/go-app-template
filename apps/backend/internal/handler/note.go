package handler

import (
	"github.com/go-fuego/fuego"

	"example.com/app/internal/dto"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/service"
)

// NoteHandler 是示例实体的 HTTP 适配层。
type NoteHandler struct {
	notes service.NoteService
}

// NewNoteHandler 构造示例 handler。
func NewNoteHandler(notes service.NoteService) *NoteHandler {
	return &NoteHandler{notes: notes}
}

// List 处理 GET /notes。
func (h *NoteHandler) List(
	c fuego.ContextNoBody,
) (response.Envelope[*dto.PageResult[dto.NoteItem]], error) {
	var q dto.NoteListQuery
	if err := bindQuery(c.Request(), &q); err != nil {
		return fail[*dto.PageResult[dto.NoteItem]](err)
	}

	res, err := h.notes.List(c.Request().Context(), q)
	if err != nil {
		return fail[*dto.PageResult[dto.NoteItem]](err)
	}
	return response.OK(requestID(c.Request()), res), nil
}

// Get 处理 GET /notes/{id}。
//
// 只返回业务数据：信封、request_id 与状态码由 server 层的装饰器统一处理，
// handler 不再需要知道这些（见 internal/server/decorate.go）。
func (h *NoteHandler) Get(c fuego.ContextNoBody) (*dto.NoteItem, error) {
	id, err := parseID(c.PathParam("id"))
	if err != nil {
		return nil, err
	}
	return h.notes.Get(c.Request().Context(), id)
}

// Create 处理 POST /notes。
func (h *NoteHandler) Create(
	c fuego.ContextWithBody[dto.NoteCreateRequest],
) (response.Envelope[*dto.NoteItem], error) {
	req := &dto.NoteCreateRequest{}
	if err := bindJSON(c.Response(), c.Request(), req); err != nil {
		return fail[*dto.NoteItem](err)
	}

	res, err := h.notes.Create(c.Request().Context(), *req)
	if err != nil {
		return fail[*dto.NoteItem](err)
	}
	return response.Created(requestID(c.Request()), res), nil
}

// Update 处理 PUT /notes/{id}。
func (h *NoteHandler) Update(
	c fuego.ContextWithBody[dto.NoteUpdateRequest],
) (response.Envelope[*dto.NoteItem], error) {
	id, err := parseID(c.PathParam("id"))
	if err != nil {
		return fail[*dto.NoteItem](err)
	}

	req := &dto.NoteUpdateRequest{}
	if err := bindJSON(c.Response(), c.Request(), req); err != nil {
		return fail[*dto.NoteItem](err)
	}

	res, err := h.notes.Update(c.Request().Context(), id, *req)
	if err != nil {
		return fail[*dto.NoteItem](err)
	}
	return response.OK(requestID(c.Request()), res), nil
}

// Delete 处理 DELETE /notes/{id}（软删除）。
func (h *NoteHandler) Delete(c fuego.ContextNoBody) (response.Envelope[dto.NoteDeleted], error) {
	id, err := parseID(c.PathParam("id"))
	if err != nil {
		return fail[dto.NoteDeleted](err)
	}

	if err := h.notes.Delete(c.Request().Context(), id); err != nil {
		return fail[dto.NoteDeleted](err)
	}
	return response.OK(requestID(c.Request()), dto.NoteDeleted{Deleted: true}), nil
}
