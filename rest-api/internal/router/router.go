// Package router 组装依赖并注册路由。
//
// 这里就是"手写依赖注入"：config → db → repository → service → handler → router。
// 项目规模变大后可以换成 google/wire，但手写版最直观。
package router

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/dmxj/go-study-examples/rest-api/internal/auth"
	"github.com/dmxj/go-study-examples/rest-api/internal/handler"
	"github.com/dmxj/go-study-examples/rest-api/internal/middleware"
	"github.com/dmxj/go-study-examples/rest-api/internal/repository"
	"github.com/dmxj/go-study-examples/rest-api/internal/response"
	"github.com/dmxj/go-study-examples/rest-api/internal/service"
)

// Deps 是构建路由所需的外部依赖。
type Deps struct {
	DB         *gorm.DB
	Tokens     *auth.TokenManager
	Log        *slog.Logger
	BcryptCost int    // 0 表示默认值；测试中设为 bcrypt.MinCost 加速
	GinMode    string // debug/release/test
}

// New 创建配置好全部路由与中间件的 *gin.Engine。
func New(d Deps) *gin.Engine {
	if d.GinMode != "" {
		gin.SetMode(d.GinMode)
	}
	// 用 gin.New() 而不是 gin.Default()：后者自带的 Logger/Recovery 输出纯文本，
	// 我们用自己的 slog 版本替换，日志格式统一为结构化 JSON。
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(d.Log), middleware.Recovery(d.Log))
	// 只信任本机反向代理传来的 X-Forwarded-For，防止客户端伪造 IP。
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})

	bookSvc := service.NewBookService(repository.NewBookRepository(d.DB))
	authSvc := service.NewAuthService(repository.NewUserRepository(d.DB), d.Tokens)
	if d.BcryptCost > 0 {
		authSvc.WithBcryptCost(d.BcryptCost)
	}
	books := handler.NewBookHandler(bookSvc)
	users := handler.NewAuthHandler(authSvc, d.Tokens)

	r.GET("/healthz", handler.Health(d.DB))

	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/register", users.Register)
		v1.POST("/auth/login", users.Login)

		// 读接口公开
		v1.GET("/books", books.List)
		v1.GET("/books/:id", books.Get)

		// 写接口需要登录：路由组 + 中间件
		authed := v1.Group("", middleware.JWTAuth(d.Tokens))
		authed.GET("/users/me", users.Me)
		authed.POST("/books", books.Create)
		authed.PATCH("/books/:id", books.Update)
		authed.DELETE("/books/:id", books.Delete)
	}

	// 未匹配的路由也返回统一格式
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, response.Body{Code: response.CodeNotFound, Message: "接口不存在"})
	})
	return r
}
