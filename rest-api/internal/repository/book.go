// Package repository 是数据访问层：只负责与数据库交互，不含业务规则。
//
// 对外暴露接口（BookRepository / UserRepository），service 层依赖接口而非 GORM，
// 单元测试时可以用内存实现替换。
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/dmxj/go-study-examples/rest-api/internal/apperr"
	"github.com/dmxj/go-study-examples/rest-api/internal/model"
)

// BookFilter 是列表查询条件。
type BookFilter struct {
	Keyword string // 匹配书名或作者
	Author  string // 作者精确匹配
	Sort    string // 已通过白名单校验的排序字段，如 "-price"
	Offset  int
	Limit   int
}

// BookRepository 定义图书的数据访问操作。
type BookRepository interface {
	Create(ctx context.Context, b *model.Book) error
	GetByID(ctx context.Context, id uint) (*model.Book, error)
	Update(ctx context.Context, b *model.Book) error
	Delete(ctx context.Context, id uint) error
	List(ctx context.Context, f BookFilter) ([]model.Book, int64, error)
}

type gormBookRepo struct{ db *gorm.DB }

// NewBookRepository 创建基于 GORM 的实现。
func NewBookRepository(db *gorm.DB) BookRepository { return &gormBookRepo{db: db} }

func (r *gormBookRepo) Create(ctx context.Context, b *model.Book) error {
	// WithContext 让请求取消/超时能传递到数据库操作。
	return translate(r.db.WithContext(ctx).Create(b).Error)
}

func (r *gormBookRepo) GetByID(ctx context.Context, id uint) (*model.Book, error) {
	var b model.Book
	if err := r.db.WithContext(ctx).First(&b, id).Error; err != nil {
		return nil, translate(err)
	}
	return &b, nil
}

func (r *gormBookRepo) Update(ctx context.Context, b *model.Book) error {
	// Save 会更新所有字段（包括零值），适合"先查后改"的流程。
	return translate(r.db.WithContext(ctx).Save(b).Error)
}

func (r *gormBookRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.Book{}, id)
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotFound
	}
	return nil
}

// sortColumns 是排序字段白名单。
// 绝不能把用户输入直接拼进 ORDER BY——那是 SQL 注入的经典入口。
var sortColumns = map[string]string{
	"id": "id", "price": "price", "year": "year", "created_at": "created_at",
}

func (r *gormBookRepo) List(ctx context.Context, f BookFilter) ([]model.Book, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Book{})
	if f.Keyword != "" {
		// 占位符 ? 由驱动做参数化，用户输入不会被当作 SQL 执行。
		like := "%" + escapeLike(f.Keyword) + "%"
		// 带 OR 的条件显式加括号，避免与后续 AND 条件组合时优先级出错。
		q = q.Where("(title LIKE ? ESCAPE '\\' OR author LIKE ? ESCAPE '\\')", like, like)
	}
	if f.Author != "" {
		q = q.Where("author = ?", f.Author)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err)
	}

	order := "id DESC"
	desc := strings.HasPrefix(f.Sort, "-")
	if col, ok := sortColumns[strings.TrimPrefix(f.Sort, "-")]; ok {
		order = col
		if desc {
			order += " DESC"
		}
		if col != "id" {
			order += ", id DESC" // 次级排序保证分页结果稳定
		}
	}

	books := make([]model.Book, 0, f.Limit)
	err := q.Order(order).Offset(f.Offset).Limit(f.Limit).Find(&books).Error
	if err != nil {
		return nil, 0, translate(err)
	}
	return books, total, nil
}

// escapeLike 转义 LIKE 中的通配符，防止用户输入 "%" 匹配全部数据。
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// translate 把 GORM 错误翻译成业务错误，其他错误原样包装返回。
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return apperr.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return apperr.ErrConflict
	default:
		return fmt.Errorf("数据库错误: %w", err)
	}
}
