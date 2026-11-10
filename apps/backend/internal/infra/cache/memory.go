package cache

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"example.com/app/internal/config"
)

// memoryCache 是进程内缓存。
//
// 默认带落盘能力：写操作只置脏标记，由后台按间隔合并成一次快照写入磁盘，
// 退出时再补一次。目的是让「本地开发不起 Redis」也能保留登录态与限流计数。
//
// 边界要说清楚：它是单进程缓存，不做跨实例共享；快照最后一次写入到进程崩溃之间
// 的窗口（默认 30s）会丢。跨实例、强一致场景必须用 redis。
type memoryCache struct {
	mu     sync.Mutex
	items  map[string]memEntry
	prefix string

	persist  bool
	path     string
	interval time.Duration
	dirty    bool

	log      *slog.Logger
	stop     chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
	closeErr error
}

// memEntry 用值类型而非指针：快照拷贝时直接复制，不需要深拷贝判空。
type memEntry struct {
	value    string
	expireAt time.Time // 零值表示永不过期
}

func (e memEntry) expired(now time.Time) bool {
	return !e.expireAt.IsZero() && now.After(e.expireAt)
}

func newMemory(cfg config.Cache, log *slog.Logger) (*memoryCache, error) {
	c := &memoryCache{
		items:    make(map[string]memEntry),
		prefix:   cfg.Prefix,
		persist:  cfg.Memory.PersistEnabled(),
		path:     cfg.Memory.Path,
		interval: time.Duration(cfg.Memory.SaveInterval) * time.Second,
		log:      log,
		stop:     make(chan struct{}),
	}

	if c.persist {
		restored, err := loadSnapshot(c.path, log)
		if err != nil {
			return nil, err
		}
		c.items = restored
		log.Info("缓存已连接", "driver", config.CacheMemory,
			"persist", c.path, "restored_keys", len(restored))
	} else {
		log.Info("缓存已连接", "driver", config.CacheMemory, "persist", "off")
	}

	c.wg.Add(1)
	go c.sweep()

	if c.persist {
		c.wg.Add(1)
		go c.flushLoop()
	}
	return c, nil
}

func (c *memoryCache) k(key string) string { return c.prefix + key }

func (c *memoryCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	full := c.k(key)
	e, ok := c.items[full]
	if !ok {
		return "", ErrMiss
	}
	if e.expired(time.Now()) {
		delete(c.items, full)
		c.dirty = true
		return "", ErrMiss
	}
	return e.value, nil
}

func (c *memoryCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[c.k(key)] = memEntry{value: value, expireAt: expireAt(ttl)}
	c.dirty = true
	return nil
}

func (c *memoryCache) Del(_ context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, k := range keys {
		full := c.k(k)
		if _, ok := c.items[full]; ok {
			delete(c.items, full)
			c.dirty = true
		}
	}
	return nil
}

// Incr 与 redis 实现保持同一语义：自增后重置 TTL（滑动窗口）；
// 命中非数字值时返回错误，而不是静默归零。
func (c *memoryCache) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	full := c.k(key)
	now := time.Now()

	e, ok := c.items[full]
	if !ok || e.expired(now) {
		e = memEntry{value: "0"}
	}

	n, err := strconv.ParseInt(e.value, 10, 64)
	if err != nil {
		// 写回原值，保持与 redis 一致：失败不改变已有内容。
		c.items[full] = e
		return 0, ErrNotInteger
	}

	n++
	c.items[full] = memEntry{
		value:    strconv.FormatInt(n, 10),
		expireAt: expireAt(ttl),
	}
	c.dirty = true
	return n, nil
}

func (c *memoryCache) Ping(context.Context) error { return nil }

func (c *memoryCache) Driver() string { return config.CacheMemory }

// Close 停止后台任务并做最后一次落盘，保证正常退出不丢数据。
func (c *memoryCache) Close() error {
	c.once.Do(func() {
		close(c.stop)
		c.wg.Wait()
		c.closeErr = c.flush()
	})
	return c.closeErr
}

// flush 把当前未过期的数据写成快照。失败时保留脏标记，下一轮重试。
func (c *memoryCache) flush() error {
	if !c.persist {
		return nil
	}

	c.mu.Lock()
	if !c.dirty {
		c.mu.Unlock()
		return nil
	}
	now := time.Now()
	entries := make(map[string]snapshotEntry, len(c.items))
	for k, e := range c.items {
		if e.expired(now) {
			continue
		}
		var exp int64
		if !e.expireAt.IsZero() {
			exp = e.expireAt.UnixNano()
		}
		entries[k] = snapshotEntry{Value: e.value, ExpireAt: exp}
	}
	c.dirty = false
	c.mu.Unlock()

	if err := writeSnapshot(c.path, entries); err != nil {
		c.mu.Lock()
		c.dirty = true
		c.mu.Unlock()
		return err
	}
	return nil
}

// flushLoop 按配置间隔合并写入，避免每次 Set 都变成一次磁盘 IO。
func (c *memoryCache) flushLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			if err := c.flush(); err != nil {
				c.log.Warn("缓存落盘失败", "path", c.path, "err", err)
			}
		}
	}
}

// sweep 定期清理过期键，避免长期运行时内存无界增长。
func (c *memoryCache) sweep() {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			for k, e := range c.items {
				if e.expired(now) {
					delete(c.items, k)
					c.dirty = true
				}
			}
			c.mu.Unlock()
		}
	}
}

func expireAt(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}
