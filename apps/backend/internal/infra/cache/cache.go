// Package cache 提供统一的缓存抽象。
//
// 两套实现（redis / memory）刻意保持**同一套语义**，让上层可以按环境替换：
//
//	| 行为          | redis                  | memory                |
//	|---------------|------------------------|-----------------------|
//	| 未命中        | ErrMiss                | ErrMiss               |
//	| ttl <= 0      | 永不过期                | 永不过期               |
//	| Incr 的 TTL   | 每次自增都重置（滑动窗口） | 同左                  |
//	| Incr 非数字值 | 返回错误                | 返回错误               |
//	| 前缀          | 统一加 prefix          | 同左                   |
//	| 重启后数据    | 保留（外部进程）         | 落盘快照，重启后恢复     |
//
// 差异只在「共享范围」：memory 仅本进程可见，跨实例部署必须用 redis。
package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"example.com/app/internal/config"
)

// ErrMiss 表示键不存在（区别于「缓存不可用」这类真实错误）。
var ErrMiss = errors.New("cache: key not found")

// ErrNotInteger 与 redis 的 "value is not an integer" 对齐，
// 保证 Incr 撞上非数字值时两套实现都报错，而不是一边报错一边自动归零。
var ErrNotInteger = errors.New("cache: value is not an integer")

// Cache 是缓存实现的统一接口。
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	// Set 写入键值，ttl <= 0 表示永不过期。
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	// Incr 自增并把 TTL 重置为 ttl，用于登录失败计数、限流等滑动窗口场景。
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	Ping(ctx context.Context) error
	Driver() string
	// Close 释放资源。memory 实现会在这里把数据落盘，必须由调用方保证被调用。
	Close() error
}

// New 按配置创建缓存实例。
//
// 选择逻辑是显式的：driver 写什么就是什么，不用「addr 为空就悄悄降级」的隐式规则——
// 那种写法在生产上出错时会安静地退化成单机缓存，问题只在多实例数据不一致时才暴露。
func New(cfg config.Cache, log *slog.Logger) (Cache, error) {
	switch cfg.Driver {
	case config.CacheRedis:
		rc := newRedis(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Redis.DialTimeout)*time.Second)
		defer cancel()

		if err := rc.Ping(ctx); err != nil {
			_ = rc.Close()
			if !cfg.FallbackToMemory {
				return nil, fmt.Errorf("连接 redis %s: %w", cfg.Redis.Addr, err)
			}
			log.Warn("redis 不可用，按配置降级为内存缓存", "addr", cfg.Redis.Addr, "err", err)
			return newMemory(cfg, log)
		}
		log.Info("缓存已连接", "driver", config.CacheRedis, "addr", cfg.Redis.Addr)
		return rc, nil

	case config.CacheMemory:
		return newMemory(cfg, log)

	default:
		return nil, fmt.Errorf("不支持的 cache.driver: %s", cfg.Driver)
	}
}
