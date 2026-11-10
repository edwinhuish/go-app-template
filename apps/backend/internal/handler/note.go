package handler

import (
	"github.com/gin-gonic/gin"

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
func (h *NoteHandler) List(c *gin.Context) {
	var q dto.NoteListQuery
	if !bindQuery(c, &q) {
		return
	}

	res, err := h.notes.List(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Get 处理 GET /notes/:id。
func (h *NoteHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	res, err := h.notes.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Create 处理 POST /notes。
func (h *NoteHandler) Create(c *gin.Context) {
	var req dto.NoteCreateRequest
	if !bindJSON(c, &req) {
		return
	}

	res, err := h.notes.Create(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, res)
}

// Update 处理 PUT /notes/:id。
func (h *NoteHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var req dto.NoteUpdateRequest
	if !bindJSON(c, &req) {
		return
	}

	res, err := h.notes.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, res)
}

// Delete 处理 DELETE /notes/:id（软删除）。
func (h *NoteHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.notes.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"deleted": true})
}
