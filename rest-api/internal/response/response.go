// Package response 定义统一的 JSON 响应格式与错误映射。
//
// 所有接口都返回：
//
//	{"code": 0, "message": "ok", "data": {...}}
//
// code=0 表示成功，非 0 为业务错误码；HTTP 状态码同时反映错误类别，
// 方便网关、监控按状态码统计，前端也能按 code 精细处理。
package response

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/dmxj/go-study-examples/rest-api/internal/apperr"
)

// Body 是统一响应结构。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// 业务错误码：前三位与 HTTP 状态码对应，便于记忆。
const (
	CodeOK           = 0
	CodeBadRequest   = 40000
	CodeUnauthorized = 40100
	CodeForbidden    = 40300
	CodeNotFound     = 40400
	CodeConflict     = 40900
	CodeInternal     = 50000
)

// OK 返回 200 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: CodeOK, Message: "ok", Data: data})
}

// Created 返回 201 成功响应。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Code: CodeOK, Message: "created", Data: data})
}

// BadRequest 返回参数错误。err 若是校验错误，会转换为可读的中文提示。
func BadRequest(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusBadRequest, Body{Code: CodeBadRequest, Message: validationMessage(err)})
}

// Error 把业务错误映射为 HTTP 响应。未知错误一律 500，且不把内部细节暴露给客户端。
func Error(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, CodeInternal
	msg := "服务器内部错误"
	switch {
	case errors.Is(err, apperr.ErrInvalidArgument):
		// 这类错误的文本由我们自己构造，可以安全地返回给客户端
		status, code, msg = http.StatusBadRequest, CodeBadRequest, err.Error()
	case errors.Is(err, apperr.ErrNotFound):
		status, code, msg = http.StatusNotFound, CodeNotFound, apperr.ErrNotFound.Error()
	case errors.Is(err, apperr.ErrConflict):
		status, code, msg = http.StatusConflict, CodeConflict, apperr.ErrConflict.Error()
	case errors.Is(err, apperr.ErrInvalidCredentials):
		status, code, msg = http.StatusUnauthorized, CodeUnauthorized, apperr.ErrInvalidCredentials.Error()
	case errors.Is(err, apperr.ErrUnauthorized):
		status, code, msg = http.StatusUnauthorized, CodeUnauthorized, apperr.ErrUnauthorized.Error()
	case errors.Is(err, apperr.ErrForbidden):
		status, code, msg = http.StatusForbidden, CodeForbidden, apperr.ErrForbidden.Error()
	default:
		// 只有真正的内部错误才打 ERROR 日志；完整错误链留在日志里供排查。
		slog.ErrorContext(c.Request.Context(), "内部错误", "err", err, "path", c.FullPath())
	}
	_ = c.Error(err) // 记录到 gin.Context，方便日志中间件读取
	c.AbortWithStatusJSON(status, Body{Code: code, Message: msg})
}

// validationMessage 把 validator 的错误翻译为简短中文。
// 生产项目可使用 go-playground/universal-translator 做完整的国际化。
func validationMessage(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return "请求参数错误: " + err.Error()
	}
	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		field := strings.ToLower(fe.Field())
		var m string
		switch fe.Tag() {
		case "required":
			m = field + " 为必填字段"
		case "min":
			m = fmt.Sprintf("%s 不能小于 %s", field, fe.Param())
		case "max":
			m = fmt.Sprintf("%s 不能大于 %s", field, fe.Param())
		case "isbn":
			m = field + " 不是合法的 ISBN"
		case "oneof":
			m = fmt.Sprintf("%s 必须是 [%s] 之一", field, fe.Param())
		case "alphanum":
			m = field + " 只能包含字母和数字"
		default:
			m = fmt.Sprintf("%s 校验失败(%s)", field, fe.Tag())
		}
		msgs = append(msgs, m)
	}
	return strings.Join(msgs, "; ")
}
