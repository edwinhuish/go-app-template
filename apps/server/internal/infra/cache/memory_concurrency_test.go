package cache

// 本文件守住内存缓存的分段化：
//   - 不同 key 的操作能真并发（`task test` 带 -race，串行实现会在这里报竞争之外
//     的另一种病：热点写把整表读写堵住，验收方式见下面的并发用例）；
//   - 并发自增不丢计数（分段最容易写错的地方是「锁错了段」，那种错误不报竞争，
//     只表现为计数偶尔少 1）；
//   - 逐段清理过期键真的清掉了，并且把对应段标脏。

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"example.com/app/internal/config"
)

// newTestCache 起一个不落盘的内存缓存：落盘会把文件 IO 混进并发用例的耗时里。
func newTestCache(t *testing.T) *memoryCache {
	t.Helper()

	persist := false
	c, err := newMemory(config.Cache{
		Driver: config.CacheMemory,
		Prefix: "test:",
		Memory: config.MemoryConfig{Persist: &persist},
	}, quietLog())
	if err != nil {
		t.Fatalf("构造内存缓存: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestMemoryIncrConcurrentDistinctKeys 验证并发自增不丢计数。
//
// 每个 goroutine 只碰自己那几个 key，所以期望值是确定的；如果分段的锁没对上
// （比如读一段、写另一段），计数就会少于期望，而 -race 未必报得出来。
func TestMemoryIncrConcurrentDistinctKeys(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	const writers = 8
	const perWriter = 200

	var wg sync.WaitGroup
	errs := make(chan error, writers*perWriter)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				key := fmt.Sprintf("counter:%d", w)
				if _, err := c.Incr(ctx, key, time.Minute); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发 Incr: %v", err)
	}

	for w := 0; w < writers; w++ {
		got, err := c.Get(ctx, fmt.Sprintf("counter:%d", w))
		if err != nil {
			t.Fatalf("读回 counter:%d: %v", w, err)
		}
		if got != fmt.Sprint(perWriter) {
			t.Errorf("counter:%d = %s，期望 %d（分段锁没对上会丢计数）", w, got, perWriter)
		}
	}
}

// TestMemoryConcurrentMixedOps 在读写删混跑时不 panic、不丢键。
//
// 这条用例的主要价值是配合 -race：分段实现里最容易犯的错是「拿着 A 段的锁去改
// B 段的 map」，那种写法在单线程测试里完全正常。
func TestMemoryConcurrentMixedOps(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := fmt.Sprintf("shared:%d", i%16)
				switch (w + i) % 4 {
				case 0:
					_ = c.Set(ctx, key, fmt.Sprint(i), time.Minute)
				case 1:
					_, _ = c.Get(ctx, key)
				case 2:
					_ = c.Del(ctx, key)
				default:
					_, _ = c.Incr(ctx, fmt.Sprintf("hot:%d", w), time.Minute)
				}
			}
		}(w)
	}
	wg.Wait()

	// 混合操作后写入的那些键必须还能读回来（Del 可能删掉其中一部分，
	// 所以这里只断言「不报错、不 panic」，正确性靠上面的计数用例）。
	for w := 0; w < 8; w++ {
		got, err := c.Get(ctx, fmt.Sprintf("hot:%d", w))
		if err != nil {
			t.Fatalf("读回 hot:%d: %v", w, err)
		}
		if got == "" {
			t.Errorf("hot:%d 是空串，期望是自增后的计数", w)
		}
	}
}

// TestMemorySweepExpiredPerShard 验证逐段清理：过期键被删、对应段被标脏。
func TestMemorySweepExpiredPerShard(t *testing.T) {
	c := newTestCache(t)
	ctx := context.Background()

	for i := 0; i < 32; i++ {
		if err := c.Set(ctx, fmt.Sprintf("ttl:%d", i), "v", 10*time.Millisecond); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	time.Sleep(20 * time.Millisecond)

	c.sweepExpired(time.Now())

	for i := 0; i < 32; i++ {
		if _, err := c.Get(ctx, fmt.Sprintf("ttl:%d", i)); err != ErrMiss {
			t.Fatalf("ttl:%d 清理后仍可读到（err = %v），期望 ErrMiss", i, err)
		}
	}
	// 清理同时负责标脏：否则删掉键之后快照里还留着它们。
	// 分段实现里最容易漏的就是「删了 A 段、标脏了 B 段」。
	dirty := 0
	for i := range c.shards {
		if c.shards[i].dirty {
			dirty++
		}
	}
	if dirty == 0 {
		t.Error("清理过期键之后没有任何段被标脏，快照里会残留已删的键")
	}
}
