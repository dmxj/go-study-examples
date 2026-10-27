// Package apperr 定义跨层共享的业务错误。
//
// repository 把数据库错误翻译成这里的错误，service 直接返回或包装它们，
// handler 再统一映射为 HTTP 状态码。这样 handler 不需要知道 GORM 的存在。
package apperr

import "errors"

// 业务错误（哨兵错误）。用 errors.Is 判断。
var (
	ErrNotFound           = errors.New("资源不存在")
	ErrConflict           = errors.New("资源已存在")
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUnauthorized       = errors.New("未登录或登录已过期")
	ErrForbidden          = errors.New("没有权限")
	// ErrInvalidArgument 表示 binding 标签无法表达的业务校验失败。
	// 用法：fmt.Errorf("%w: 具体原因", apperr.ErrInvalidArgument)，响应会返回 400 与具体原因。
	ErrInvalidArgument = errors.New("参数错误")
)
