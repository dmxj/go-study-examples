// Package service 是业务逻辑层：编排 repository、执行业务规则。
//
// service 不认识 HTTP（没有 *gin.Context），也不认识 GORM，
// 只依赖 model、apperr 和 repository 接口，因此可以纯单元测试。
package service

import (
	"context"
	"fmt"

	"github.com/dmxj/go-study-examples/rest-api/internal/model"
	"github.com/dmxj/go-study-examples/rest-api/internal/repository"
)

// BookService 提供图书相关业务。
type BookService struct {
	repo repository.BookRepository
}

// NewBookService 创建 BookService。
func NewBookService(repo repository.BookRepository) *BookService {
	return &BookService{repo: repo}
}

// Create 新增图书，userID 为创建者。ISBN 重复时返回 apperr.ErrConflict。
func (s *BookService) Create(ctx context.Context, userID uint, req model.CreateBookRequest) (*model.Book, error) {
	b := &model.Book{
		Title:     req.Title,
		Author:    req.Author,
		ISBN:      normalizeISBN(req.ISBN),
		Price:     req.Price,
		Year:      req.Year,
		Stock:     req.Stock,
		CreatedBy: userID,
	}
	if err := s.repo.Create(ctx, b); err != nil {
		return nil, fmt.Errorf("创建图书 ISBN=%s: %w", b.ISBN, err)
	}
	return b, nil
}

// Get 按 ID 查询图书。
func (s *BookService) Get(ctx context.Context, id uint) (*model.Book, error) {
	return s.repo.GetByID(ctx, id)
}

// Update 部分更新图书：只修改请求中出现的字段。
func (s *BookService) Update(ctx context.Context, id uint, req model.UpdateBookRequest) (*model.Book, error) {
	b, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Title != nil {
		b.Title = *req.Title
	}
	if req.Author != nil {
		b.Author = *req.Author
	}
	if req.Price != nil {
		b.Price = *req.Price
	}
	if req.Year != nil {
		b.Year = *req.Year
	}
	if req.Stock != nil {
		b.Stock = *req.Stock
	}
	if err := s.repo.Update(ctx, b); err != nil {
		return nil, fmt.Errorf("更新图书 #%d: %w", id, err)
	}
	return b, nil
}

// Delete 删除图书。
func (s *BookService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

// List 分页查询图书。
func (s *BookService) List(ctx context.Context, q model.ListBooksQuery) (*model.Page[model.Book], error) {
	q.Normalize()
	books, total, err := s.repo.List(ctx, repository.BookFilter{
		Keyword: q.Keyword,
		Author:  q.Author,
		Sort:    q.Sort,
		Offset:  q.Offset(),
		Limit:   q.PageSize,
	})
	if err != nil {
		return nil, err
	}
	return &model.Page[model.Book]{Items: books, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

// normalizeISBN 去掉连字符和空格，保证 "978-7-111" 与 "9787111" 视为同一本书。
func normalizeISBN(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if c := s[i]; c != '-' && c != ' ' {
			out = append(out, c)
		}
	}
	return string(out)
}
