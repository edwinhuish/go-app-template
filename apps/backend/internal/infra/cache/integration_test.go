// 本文件验证 redis 与 memory 两套实现的语义真的一致——cache 包的文件头把这张
// 对照表写成了承诺（未命中 ErrMiss、ttl<=0 永不过期、Incr 重置 TTL、非数字报错），
// 但只有真 Redis 能验：memory 的「一致」是它自己实现的，redis 的差异藏在
// Redis 命令的细节里（EXPIRE 0 会删键、INCR 撞上非数字值报错文案不同）。
//
// 默认跳过，起一个一次性容器再跑：
//
//	docker run -d --name probe-redis -p 16379:6379 redis:7-alpine
//	APP_TEST_REDIS_ADDR=127.0.0.1:16379 go test ./internal/infra/cache -v
//	docker rm -f probe-redis
package cache_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"example.com/app/internal/config"
	"example.com/app/internal/infra/cache"
)

func TestRedisAndMemoryShareSemantics(t *testing.T) {
	addr := os.Getenv("APP_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("未设置 APP_TEST_REDIS_ADDR，跳过真实 Redis 用例")
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := func(driver string) config.Cache {
		return config.Cache{
			Driver: driver,
			Prefix: "cachetest:",
			Redis:  config.RedisConfig{Addr: addr, DialTimeout: 2},
			Memory: config.MemoryConfig{Persist: boolPtr(false)},
		}
	}

	ctx := context.Background()

	// 同一组断言跑两遍：任何一边不满足，都说明对照表是假的。
	for _, driver := range []string{config.CacheRedis, config.CacheMemory} {
		t.Run(driver, func(t *testing.T) {
			c, err := cache.New(cfg(driver), log)
			if err != nil {
				t.Fatalf("创建缓存: %v", err)
			}
			t.Cleanup(func() { _ = c.Close() })

			// 用带随机后缀的 key，避免容器复用时的脏数据影响断言。
			key := "probe:" + t.Name() + ":" + time.Now().Format("150405.000000")

			// --- 未命中是 ErrMiss，而不是「空字符串 + nil」 ---
			if _, err := c.Get(ctx, key); err != cache.ErrMiss {
				t.Fatalf("未命中 err = %v，期望 ErrMiss", err)
			}

			// --- 写读一致 ---
			if err := c.Set(ctx, key, "v1", time.Minute); err != nil {
				t.Fatalf("Set: %v", err)
			}
			if v, err := c.Get(ctx, key); err != nil || v != "v1" {
				t.Fatalf("Get = %q, %v，期望 v1", v, err)
			}

			// --- ttl 到点后必须真的消失（redis 靠过期，memory 靠惰性清理）---
			shortKey := key + ":ttl"
			if err := c.Set(ctx, shortKey, "v", 100*time.Millisecond); err != nil {
				t.Fatalf("Set(短 ttl): %v", err)
			}
			time.Sleep(200 * time.Millisecond)
			if _, err := c.Get(ctx, shortKey); err != cache.ErrMiss {
				t.Errorf("ttl 到期后 err = %v，期望 ErrMiss", err)
			}

			// --- ttl<=0 表示永不过期 ---
			if err := c.Set(ctx, shortKey, "forever", 0); err != nil {
				t.Fatalf("Set(无 ttl): %v", err)
			}
			if v, err := c.Get(ctx, shortKey); err != nil || v != "forever" {
				t.Errorf("无 ttl 的键 = %q, %v，期望永久有效", v, err)
			}

			// --- Incr 自增 ---
			counterKey := key + ":count"
			for i := int64(1); i <= 3; i++ {
				n, err := c.Incr(ctx, counterKey, time.Minute)
				if err != nil {
					t.Fatalf("第 %d 次 Incr: %v", i, err)
				}
				if n != i {
					t.Fatalf("第 %d 次 Incr = %d，期望 %d", i, n, i)
				}
			}

			// --- Incr 撞上非数字值必须报错，而不是静默归零 ---
			badKey := key + ":bad"
			if err := c.Set(ctx, badKey, "not-a-number", time.Minute); err != nil {
				t.Fatalf("Set(非数字): %v", err)
			}
			if _, err := c.Incr(ctx, badKey, time.Minute); err != cache.ErrNotInteger {
				t.Errorf("非数字值 Incr err = %v，期望 ErrNotInteger", err)
			}

			// --- Incr 每次都重置 TTL（滑动窗口）：这是登录失败计数依赖的语义 ---
			windowKey := key + ":window"
			if _, err := c.Incr(ctx, windowKey, 150*time.Millisecond); err != nil {
				t.Fatalf("Incr(短 ttl): %v", err)
			}
			time.Sleep(100 * time.Millisecond)
			if _, err := c.Incr(ctx, windowKey, 150*time.Millisecond); err != nil {
				t.Fatalf("Incr(重置 ttl): %v", err)
			}
			time.Sleep(100 * time.Millisecond) // 距第一次 200ms，距第二次 100ms
			if v, err := c.Get(ctx, windowKey); err != nil || v != "2" {
				t.Errorf("滑动窗口 = %q, %v，期望仍然是 2（TTL 被重置过）", v, err)
			}

			// --- Del 之后回到未命中 ---
			if err := c.Del(ctx, key, shortKey, counterKey, badKey, windowKey); err != nil {
				t.Fatalf("Del: %v", err)
			}
			if _, err := c.Get(ctx, key); err != cache.ErrMiss {
				t.Errorf("Del 之后 err = %v，期望 ErrMiss", err)
			}

			// --- Ping ---
			if err := c.Ping(ctx); err != nil {
				t.Errorf("Ping: %v", err)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }
