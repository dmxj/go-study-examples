// Package handler 是 HTTP 层：解析参数 → 调用 service → 写统一响应。
//
// handler 里不写业务逻辑，也不直接操作数据库。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/dmxj/go-study-examples/rest-api/internal/apperr"
	"github.com/dmxj/go-study-examples/rest-api/internal/middleware"
	"github.com/dmxj/go-study-examples/rest-api/internal/model"
	"github.com/dmxj/go-study-examples/rest-api/internal/response"
	"github.com/dmxj/go-study-examples/rest-api/internal/service"
)

// BookHandler 处理 /api/v1/books 相关请求。
type BookHandler struct {
	svc *service.BookService
}

// NewBookHandler 创建 BookHandler。
func NewBookHandler(svc *service.BookService) *BookHandler {
	return &BookHandler{svc: svc}
}

// List godoc
//
//	@Summary	分页查询图书
//	@Tags		books
//	@Param		page		query	int		false	"页码，从 1 开始"
//	@Param		page_size	query	int		false	"每页条数，最大 100"
//	@Param		q			query	string	false	"书名/作者关键字"
//	@Param		sort		query	string	false	"排序：id/-id/price/-price/year/-year"
//	@Success	200			{object}	response.Body
//	@Router		/books [get]
func (h *BookHandler) List(c *gin.Context) {
	var q model.ListBooksQuery
	// ShouldBindQuery 只绑定 URL 查询参数并执行 binding 校验；
	// 用 Should* 而不是 Bind*：后者校验失败会自动写 400 并且无法自定义响应格式。
	if err := c.ShouldBindQuery(&q); err != nil {
		response.BadRequest(c, err)
		return
	}
	page, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, page)
}

// Get 查询单本图书。
func (h *BookHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	b, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, b)
}

// Create 新增图书（需登录）。
func (h *BookHandler) Create(c *gin.Context) {
	var req model.CreateBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}
	uid, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperr.ErrUnauthorized)
		return
	}
	b, err := h.svc.Create(c.Request.Context(), uid, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Header("Location", "/api/v1/books/"+strconv.FormatUint(uint64(b.ID), 10))
	response.Created(c, b)
}

// Update 部分更新图书（需登录）。
func (h *BookHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req model.UpdateBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}
	b, err := h.svc.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, b)
}

// Delete 删除图书（需登录）。
func (h *BookHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, nil)
}

// parseID 解析路径参数 :id，失败时直接写 400 响应。
func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, errInvalidID)
		return 0, false
	}
	return uint(id), true
}
