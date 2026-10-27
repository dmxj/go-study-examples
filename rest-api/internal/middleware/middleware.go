// Package middleware 提供 Gin 中间件：请求 ID、访问日志、panic 恢复、JWT 鉴权。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dmxj/go-study-examples/rest-api/internal/apperr"
	"github.com/dmxj/go-study-examples/rest-api/internal/auth"
	"github.com/dmxj/go-study-examples/rest-api/internal/response"
)

const (
	headerRequestID = "X-Request-ID"
	keyRequestID    = "request_id"
	keyUserID       = "user_id"
	keyUsername     = "username"
)

// RequestID 为每个请求生成（或透传上游的）请求 ID，并写回响应头，便于全链路排查。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(headerRequestID)
		if id == "" || len(id) > 64 {
			var b [8]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		c.Set(keyRequestID, id)
		c.Header(headerRequestID, id)
		c.Next()
	}
}

// Logger 用 slog 记录访问日志。c.Next() 之前是"请求进来"，之后是"响应写完"。
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency", time.Since(start).String(),
			"ip", c.ClientIP(),
			"request_id", c.GetString(keyRequestID),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}
		log.Log(c.Request.Context(), level, "http request", attrs...)
	}
}

// Recovery 捕获 handler 中的 panic，返回统一的 500 响应，而不是让连接直接断开。
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, rec any) {
		log.Error("panic recovered", "panic", rec, "path", c.Request.URL.Path,
			"request_id", c.GetString(keyRequestID))
		c.AbortWithStatusJSON(500, response.Body{Code: response.CodeInternal, Message: "服务器内部错误"})
	})
}

// JWTAuth 校验 Authorization: Bearer <token>，成功后把用户信息写入 Context。
func JWTAuth(tm *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			response.Error(c, apperr.ErrUnauthorized)
			return
		}
		claims, err := tm.Parse(token)
		if err != nil {
			response.Error(c, apperr.ErrUnauthorized)
			return
		}
		uid, err := claims.UserID()
		if err != nil {
			response.Error(c, apperr.ErrUnauthorized)
			return
		}
		c.Set(keyUserID, uid)
		c.Set(keyUsername, claims.Username)
		c.Next()
	}
}

// UserID 读取 JWTAuth 写入的用户 ID。只能在 JWTAuth 之后的 handler 中调用。
func UserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get(keyUserID)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}
