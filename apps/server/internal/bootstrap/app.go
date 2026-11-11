// Package bootstrap 负责依赖装配：把配置、基础设施、仓储、服务、handler 串起来。
//
// 组装逻辑集中在这一处，好处是依赖关系一眼可见，也避免各处 New 满天飞。
// 将来依赖复杂度上升，可以平滑替换成 wire 生成。
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"example.com/app/internal/config"
	"example.com/app/internal/handler"
	"example.com/app/internal/infra/cache"
	"example.com/app/internal/infra/database"
	"example.com/app/internal/infra/logger"
	"example.com/app/internal/model"
	"example.com/app/internal/repository"
	"example.com/app/internal/server"
	"example.com/app/internal/service"
)

// App 持有进程级资源，供 main 启动与优雅退出使用。
type App struct {
	Config *config.Config
	// Server 是 HTTP 层的成品（路由 + 中间件 + 文档），对外只给 Handler()。
	Server *server.Server
	DB     *gorm.DB
	Cache  cache.Cache
	// Tx 把一次跨仓储的写入收进同一个事务。仓储从 ctx 取连接，所以只要在最外层
	// 套一次 Transaction，里面所有仓储调用自动加入，不会漏掉某一条写。
	//
	// 示例服务都是单表写入，不需要它；它作为装配产物交出去，供接业务时使用。
	Tx  repository.TxManager
	Log *slog.Logger
}

// models 是全部需要同步结构的模型。
//
// 集中声明一次：结构同步、将来的备份与迁移都读同一份清单，
// 分开写必然漏表——而漏表的表现是「第一次写入这张表时 no such table」。
var models = []any{
	&model.User{},
	&model.Note{},
}

// New 完成全部装配。任何一步失败都直接返回，避免带病启动。
//
// version 由 main 传入（构建期注入），只用于 OpenAPI 文档里的版本号。
func New(cfg *config.Config, version string) (*App, error) {
	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(log)

	db, err := database.Open(cfg.Database, log)
	if err != nil {
		return nil, err
	}

	if cfg.Database.AutoMigrate {
		if err := database.AutoMigrate(db, models...); err != nil {
			_ = database.Close(db)
			return nil, err
		}
		log.Info("数据库结构已同步")
	}

	c, err := cache.New(cfg.Cache, log)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("初始化缓存: %w", err)
	}

	// ---- 数据访问层 ----
	userRepo := repository.NewUserRepository(db)
	noteRepo := repository.NewNoteRepository(db)
	// 跨仓储的写入走它：ctx 里带事务，仓储按 ctx 自动加入。
	// 示例服务是单表写入，用不到；接业务时把 txs 传给需要多处落库的服务。
	txs := repository.NewTxManager(db)

	// ---- 业务层 ----
	authSvc := service.NewAuthService(userRepo, c, service.AuthConfig{
		Secret:       cfg.JWT.Secret,
		Expire:       cfg.JWT.ExpireDuration(),
		MaxLoginFail: 5,
	}, service.WithAuthLogger(log))

	noteSvc := service.NewNoteService(noteRepo, service.WithNoteLogger(log))
	userSvc := service.NewUserService(userRepo, service.WithUserLogger(log))

	// ---- HTTP 层 ----
	// 缓存同时交给 HTTP 层做限流计数：登录接口按 IP 分桶，计数与会话共享同一套存储，
	// 切 redis 之后多实例也共用一份。
	httpServer := server.New(cfg, log, authSvc, c, server.Handlers{
		Auth: handler.NewAuthHandler(authSvc),
		Note: handler.NewNoteHandler(noteSvc),
		User: handler.NewUserHandler(userSvc),
	}, version)

	app := &App{Config: cfg, Server: httpServer, DB: db, Cache: c, Tx: txs, Log: log}

	// 种子数据放在装配最后：它要往各表写数据，依赖上面全部就位。
	if cfg.Database.Seed {
		if err := app.seed(context.Background(), userRepo, authSvc); err != nil {
			app.Close()
			return nil, fmt.Errorf("初始化种子数据: %w", err)
		}
	}

	return app, nil
}

// Close 释放进程级资源，按依赖倒序关闭。
func (a *App) Close() {
	if a.Cache != nil {
		if err := a.Cache.Close(); err != nil {
			a.Log.Warn("关闭缓存出错", "err", err)
		}
	}
	if a.DB != nil {
		if err := database.Close(a.DB); err != nil {
			a.Log.Warn("关闭数据库出错", "err", err)
		}
	}
}
