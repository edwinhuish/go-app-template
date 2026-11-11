// Package database 负责建立数据库连接与执行结构同步。
//
// 三种驱动（postgres / mysql / sqlite）共用同一套上层代码：
// 差异只在这里消化——连接串、连接池默认值、日志级别、方言细节，
// 业务层拿到的始终是一个 *gorm.DB。
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"example.com/app/internal/config"
	"example.com/app/internal/pkg/errs"
)

// Open 按配置打开数据库连接并设置连接池。
func Open(cfg config.Database, log *slog.Logger) (*gorm.DB, error) {
	dsn, err := prepareDSN(cfg)
	if err != nil {
		return nil, err
	}

	gormCfg := &gorm.Config{
		Logger: newGormLogger(log, cfg.LogLevel),
		// 表名用单数更贴近业务实体：user / note。
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		// 关闭 gorm 的默认事务包装：单条写入本来就在隐式事务里，
		// 多一层显式事务会让 sqlite 的写锁持有时间变长。
		SkipDefaultTransaction: cfg.Driver == config.DriverSQLite,
		// 把驱动返回的约束错误统一成 gorm.ErrDuplicatedKey，
		// 否则三个库给出三种错误文本，上层没法一处翻译。
		TranslateError: true,
	}

	dialector, err := newDialector(cfg.Driver, dsn)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败: %w", cfg.Driver, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接池失败: %w", err)
	}

	maxOpen, maxIdle := cfg.MaxOpen, cfg.MaxIdle
	if cfg.Driver == config.DriverSQLite {
		// sqlite 是单写者模型：连接数开太大只会在并发写上抢锁。
		// 读并发靠 WAL，不靠连接数，所以这里收敛到个位数。
		maxOpen = min(maxOpen, 8)
		maxIdle = min(maxIdle, 4)
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MaxLifetime) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		// 已经打开的连接池要关掉：这里直接 return 会漏掉 conn 与它背后的 goroutine。
		_ = sqlDB.Close()
		return nil, fmt.Errorf("数据库 ping 失败: %w", err)
	}

	if err := registerErrorTranslators(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("注册错误翻译: %w", err)
	}

	log.Info("数据库已连接", "driver", cfg.Driver, "max_open", maxOpen)
	return db, nil
}

// registerErrorTranslators 把驱动层的唯一约束错误翻译成业务错误码。
//
// 放在 gorm 回调里而不是各个调用点：写入出口有通用仓储方法、各仓储自己开的
// 事务、事务里的 tx.Create 三条路，逐个翻译必然漏一处。漏掉的那处表现为
// 「偶尔保存失败，刷新一下又好了」——其实是一次 500，用户只会当成网络问题重试。
func registerErrorTranslators(db *gorm.DB) error {
	translate := func(tx *gorm.DB) {
		if errors.Is(tx.Error, gorm.ErrDuplicatedKey) {
			tx.Error = errs.ErrConflict.WithMsg("数据已存在：唯一字段与已有记录重复，可能是一次重复提交")
		}
	}

	specs := []struct {
		name string
		reg  func(name string, fn func(*gorm.DB)) error
	}{
		{"app:translate_duplicate_create", db.Callback().Create().After("gorm:create").Register},
		{"app:translate_duplicate_update", db.Callback().Update().After("gorm:update").Register},
	}
	for _, s := range specs {
		if err := s.reg(s.name, translate); err != nil {
			return err
		}
	}
	return nil
}

func newDialector(driver, dsn string) (gorm.Dialector, error) {
	switch driver {
	case config.DriverPostgres:
		return postgres.Open(dsn), nil
	case config.DriverMySQL:
		return mysql.Open(dsn), nil
	case config.DriverSQLite:
		return sqlite.Open(dsn), nil
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", driver)
	}
}

// prepareDSN 做驱动特有的事前准备，目前只有 sqlite 需要建目录。
func prepareDSN(cfg config.Database) (string, error) {
	dsn := cfg.ResolvedDSN()
	if cfg.Driver != config.DriverSQLite {
		return dsn, nil
	}

	path := sqliteFilePath(dsn)
	if path == "" || path == ":memory:" {
		return dsn, nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("创建 sqlite 目录 %s: %w", dir, err)
		}
	}
	return dsn, nil
}

// sqliteFilePath 从 DSN 里取出真实文件路径，兼容 file:xxx?pragma 与裸路径两种写法。
func sqliteFilePath(dsn string) string {
	s := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	return s
}

// AutoMigrate 同步表结构。
//
// 只适合骨架期与开发环境：生产应改为显式迁移脚本，
// 否则一次模型改动就可能在存量表上做出意外的 DDL。
func AutoMigrate(db *gorm.DB, models ...any) error {
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}

// Close 关闭连接池。
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func logLevel(level string) gormlogger.LogLevel {
	switch strings.ToLower(level) {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "info":
		return gormlogger.Info
	default:
		return gormlogger.Warn
	}
}

// newGormLogger 把 gorm 的日志接到本项目的 slog 上。
//
// 不用 gormlogger.Default，它有两个副作用会直接坑到线上：
//   - 带 ANSI 颜色：JSON 日志里出现 \u001b[31;1m，行采集解析直接坏掉；
//   - 把 ErrRecordNotFound 当 Warn 打出来：而「按用户名查不到」在登录流程里是
//     正常分支，一次用户名探测就能刷一行红字，既能淹没真实告警，也放大了日志量。
//
// 另外它只认自己的 LogLevel，与 log.level 是两套开关；这里转成 slog 的级别，
// 让「改 log.level」对 SQL 日志同样生效。
func newGormLogger(log *slog.Logger, level string) gormlogger.Interface {
	return gormlogger.New(gormSlogWriter{log: log, level: slogLevel(level)}, gormlogger.Config{
		SlowThreshold: 200 * time.Millisecond,
		LogLevel:      logLevel(level),
		// 查不到记录是业务分支，不是异常。
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
}

type gormSlogWriter struct {
	log   *slog.Logger
	level slog.Level
}

func (w gormSlogWriter) Printf(format string, args ...any) {
	w.log.Log(context.Background(), w.level, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

// slogLevel 把配置里的日志级别翻成 slog 级别，供 SQL 日志复用同一个开关。
func slogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "error", "silent":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}
