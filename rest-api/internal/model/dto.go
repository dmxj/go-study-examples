package model

// 以下是 HTTP 层的请求/响应结构（DTO）。
//
// binding 标签由 Gin 内置的 go-playground/validator 解析：
//   required   必填            min/max   数字范围或字符串长度
//   omitempty  为空时跳过后续规则   isbn      ISBN-10 或 ISBN-13
//   alphanum   仅字母数字        oneof     枚举值

// CreateBookRequest 是新增图书的请求体。
type CreateBookRequest struct {
	Title  string `json:"title" binding:"required,max=200"`
	Author string `json:"author" binding:"required,max=100"`
	ISBN   string `json:"isbn" binding:"required,isbn"`
	Price  int64  `json:"price" binding:"min=0"`
	Year   int    `json:"year" binding:"omitempty,min=1000,max=2100"`
	Stock  int    `json:"stock" binding:"min=0"`
}

// UpdateBookRequest 是部分更新（PATCH 语义）的请求体。
// 指针字段：nil 表示"客户端没传，不修改"；非 nil 表示要改成该值（哪怕是 0）。
type UpdateBookRequest struct {
	Title  *string `json:"title" binding:"omitempty,min=1,max=200"`
	Author *string `json:"author" binding:"omitempty,min=1,max=100"`
	Price  *int64  `json:"price" binding:"omitempty,min=0"`
	Year   *int    `json:"year" binding:"omitempty,min=1000,max=2100"`
	Stock  *int    `json:"stock" binding:"omitempty,min=0"`
}

// ListBooksQuery 是列表接口的查询参数（来自 URL ?page=1&page_size=10）。
type ListBooksQuery struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Keyword  string `form:"q" binding:"max=100"`
	Author   string `form:"author" binding:"max=100"`
	Sort     string `form:"sort" binding:"omitempty,oneof=id -id price -price year -year created_at -created_at"`
}

// Normalize 填充分页默认值。
func (q *ListBooksQuery) Normalize() {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 10
	}
	if q.Sort == "" {
		q.Sort = "-id"
	}
}

// Offset 返回 SQL OFFSET。
func (q ListBooksQuery) Offset() int { return (q.Page - 1) * q.PageSize }

// Page 是通用分页结果。用泛型避免为每种资源写一个 XxxPage。
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// RegisterRequest 是注册请求体。
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32,alphanum"`
	Password string `json:"password" binding:"required,min=8,max=72"` // bcrypt 最多处理 72 字节
}

// LoginRequest 是登录请求体。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse 是登录成功的响应。
type TokenResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"token_type"`
	ExpiresIn int64  `json:"expires_in"` // 秒
}
