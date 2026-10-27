package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/dmxj/go-study-examples/rest-api/internal/apperr"
	"github.com/dmxj/go-study-examples/rest-api/internal/auth"
	"github.com/dmxj/go-study-examples/rest-api/internal/middleware"
	"github.com/dmxj/go-study-examples/rest-api/internal/model"
	"github.com/dmxj/go-study-examples/rest-api/internal/response"
	"github.com/dmxj/go-study-examples/rest-api/internal/service"
)

var errInvalidID = errors.New("id 必须是正整数")

// AuthHandler 处理注册、登录与当前用户查询。
type AuthHandler struct {
	svc *service.AuthService
	tm  *auth.TokenManager
}

// NewAuthHandler 创建 AuthHandler。
func NewAuthHandler(svc *service.AuthService, tm *auth.TokenManager) *AuthHandler {
	return &AuthHandler{svc: svc, tm: tm}
}

// Register 注册用户。
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}
	u, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, u)
}

// Login 登录并返回 JWT。
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err)
		return
	}
	token, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, model.TokenResponse{
		Token:     token,
		TokenType: "Bearer",
		ExpiresIn: int64(h.tm.TTL().Seconds()),
	})
}

// Me 返回当前登录用户。
func (h *AuthHandler) Me(c *gin.Context) {
	uid, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperr.ErrUnauthorized)
		return
	}
	u, err := h.svc.Me(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, u)
}

// Health 是存活/就绪探针：检查数据库连通性。K8s 的 livenessProbe/readinessProbe 可直接使用。
func Health(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err == nil {
			err = sqlDB.PingContext(c.Request.Context())
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
