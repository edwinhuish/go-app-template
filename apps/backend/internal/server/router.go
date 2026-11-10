// Package server 负责组装 gin 引擎与路由表。
//
// 路由集中在这一处定义，方便一眼看清 API 全貌；
// handler 本身不关心自己被挂在哪个路径下，也就不必为改路径而改代码。
package server

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"example.com/app/internal/config"
	"example.com/app/internal/handler"
	"example.com/app/internal/server/middleware"
	"example.com/app/internal/service"
)

// Handlers 汇总所有 HTTP handler，由 bootstrap 注入。
//
// 显式字段而不是 map 或反射：少一个 handler 编译期就会报错，
// 而 map 要等到运行到那条路由才发现是 nil。
type Handlers struct {
	Auth *handler.AuthHandler
	Note *handler.NoteHandler
	User *handler.UserHandler
}

const (
	// bodyLimitBytes 是单个请求体的兜底上限。取 8MB：普通 JSON 接口远用不到，
	// 而它挡住的是「几 GB 的 POST 把内存吃光」这类请求。
	bodyLimitBytes = 8 << 20

	// requestTimeout 是单个请求的处理上限。
	// 卡住时早点返回 504，比让浏览器转到超时更好查——日志里会留下带 request_id 的记录。
	requestTimeout = 30 * time.Second
)

// New 构建 gin 引擎。
func New(cfg *config.Config, log *slog.Logger, auth service.AuthService, h Handlers) *gin.Engine {
	gin.SetMode(ginMode(cfg))

	engine := gin.New()
	engine.Use(
		// 顺序有讲究：RequestID 要在最前面，后面所有日志才带得上它；
		// Recovery 要在最外层，panic 时才能兜住整个链路。
		middleware.RequestID(),
		middleware.Recovery(log),
		middleware.AccessLog(log),
		middleware.CORS(cfg.HTTP.CORSOrigins),
		middleware.BodyLimit(bodyLimitBytes),
		middleware.RequestTimeout(requestTimeout),
	)

	engine.GET("/healthz", handler.Health)

	api := engine.Group("/api/v1")

	authGroup := api.Group("/auth")
	{
		authGroup.POST("/login", h.Auth.Login)
		authGroup.GET("/profile", middleware.Auth(auth), h.Auth.Profile)
		authGroup.POST("/logout", middleware.Auth(auth), h.Auth.Logout)
	}

	// 需要登录的接口统一挂在这一组，避免每条路由各写一遍中间件、漏掉一条就是漏洞。
	secured := api.Group("", middleware.Auth(auth))
	{
		notes := secured.Group("/notes")
		notes.GET("", h.Note.List)
		notes.POST("", h.Note.Create)
		notes.GET("/:id", h.Note.Get)
		notes.PUT("/:id", h.Note.Update)
		notes.DELETE("/:id", h.Note.Delete)

		// 账号目录只读，且只有管理员能看。权限判断放在路由上而不是 handler 里：
		// 加一条同类接口时，忘了加判断会立刻从代码审查里看出来。
		secured.GET("/users", middleware.RequireAdmin(), h.User.List)
	}

	return engine
}

func ginMode(cfg *config.Config) string {
	if cfg.HTTP.Mode == "release" || cfg.IsProd() {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}
