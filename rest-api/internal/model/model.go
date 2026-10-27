// Package model 定义数据库模型与请求/响应 DTO。
package model

import "time"

// Book 是图书表的 GORM 模型。
//
// 价格用 int64 存"分"而不是 float64 存"元"：浮点数无法精确表示 0.1，
// 金额计算必须用整数（或 decimal 库）。
type Book struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:200;not null;index" json:"title"`
	Author    string    `gorm:"size:100;not null;index" json:"author"`
	ISBN      string    `gorm:"size:20;not null;uniqueIndex" json:"isbn"`
	Price     int64     `gorm:"not null;default:0" json:"price"` // 单位：分
	Year      int       `json:"year,omitempty"`
	Stock     int       `gorm:"not null;default:0" json:"stock"`
	CreatedBy uint      `gorm:"index" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User 是用户表模型。PasswordHash 的 json:"-" 保证它永远不会被序列化到响应里。
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:32;not null;uniqueIndex" json:"username"`
	PasswordHash string    `gorm:"size:100;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}
