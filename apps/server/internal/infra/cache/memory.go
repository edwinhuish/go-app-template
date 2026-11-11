package cache

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"example.com/app/internal/config"
)

// shardCount 是分段数，取 2 的幂。
//
// 分段的意义在锁粒度：单锁实现里，「登录失败计数 +1」这种热点写会和整张表的读写
// 抢同一把锁，而热点恰恰是限流与登录计数——越是被刷的时候越堵。分 16 段之后，
// 不同 key 落到不同的锁上，同一把锁后面挂着的是 1/16 的表。
const shardCount = 16

// cacheShard 是一段缓存。所有字段都在 mu 保护下读写。
type cacheShard struct {
	mu    sync.Mutex
	items map[string]memEntry
	// dirty 表示这一段自上次落盘后有改动。分段落盘失败时只把对应段重新标脏。
	dirty bool
}

// memoryCache 是进程内缓存。
//
// 默认带落盘能力：写操作只置脏标记，由后台按间隔合并成一次快照写入磁盘，
// 退出时再补一次。目的是让「本地开发不起 Redis」也能保留登录态与限流计数。
//
// 边界要说清楚：它是单进程缓存，不做跨实例共享；快照最后一次写入到进程崩溃之间
// 的窗口（默认 30s）会丢。跨实例、强一致场景必须用 redis。
type memoryCache struct {
	shards  [shardCount]cacheShard
	prefix  string
	persist bool
	path    string

	interval time.Duration
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
		prefix:   cfg.Prefix,
		persist:  cfg.Memory.PersistEnabled(),
		path:     cfg.Memory.Path,
		interval: time.Duration(cfg.Memory.SaveInterval) * time.Second,
		log:      log,
		stop:     make(chan struct{}),
	}
	for i := range c.shards {
		c.shards[i].items = make(map[string]memEntry)
	}

	if c.persist {
		restored, err := loadSnapshot(c.path, log)
		if err != nil {
			return nil, err
		}
		for k, e := range restored {
			s := c.shard(k)
			// 加载阶段没有并发，不需要持锁。
			s.items[k] = e
		}
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

// shard 按 key 选出所属段。用 FNV-1a 手写一遍：不为了一个哈希引入依赖，
// 也不需要密码学强度——只要分布均匀、同一 key 恒定。
func (c *memoryCache) shard(key string) *cacheShard {
	var h uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return &c.shards[h%shardCount]
}

func (c *memoryCache) Get(_ context.Context, key string) (string, error) {
	full := c.k(key)

	s := c.shard(full)
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.items[full]
	if !ok {
		return "", ErrMiss
	}
	if e.expired(time.Now()) {
		delete(s.items, full)
		s.dirty = true
		return "", ErrMiss
	}
	return e.value, nil
}

func (c *memoryCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	full := c.k(key)

	s := c.shard(full)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[full] = memEntry{value: value, expireAt: expireAt(ttl)}
	s.dirty = true
	return nil
}

func (c *memoryCache) Del(_ context.Context, keys ...string) error {
	// 逐 key 加锁，不一次锁多段：多段同时持锁是死锁的经典来源，
	// 而删除多个 key 之间本来就不需要原子性。
	for _, k := range keys {
		full := c.k(k)
		s := c.shard(full)
		s.mu.Lock()
		if _, ok := s.items[full]; ok {
			delete(s.items, full)
			s.dirty = true
		}
		s.mu.Unlock()
	}
	return nil
}

// Incr 与 redis 实现保持同一语义：自增后重置 TTL（滑动窗口）；
// 命中非数字值时返回错误，而不是静默归零。
func (c *memoryCache) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	full := c.k(key)

	s := c.shard(full)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	e, ok := s.items[full]
	if !ok || e.expired(now) {
		e = memEntry{value: "0"}
	}

	n, err := strconv.ParseInt(e.value, 10, 64)
	if err != nil {
		// 写回原值，保持与 redis 一致：失败不改变已有内容。
		s.items[full] = e
		return 0, ErrNotInteger
	}

	n++
	s.items[full] = memEntry{
		value:    strconv.FormatInt(n, 10),
		expireAt: expireAt(ttl),
	}
	s.dirty = true
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

// flush 把当前未过期的数据写成快照。失败时重新标脏，下一轮重试。
//
// 只在持锁期间做拷贝，写盘在锁外：磁盘 IO 的耗时与表的规模无关，却可能比整个
// 落盘间隔还长，放在锁里会让这段时间的所有请求都排在它后面。
func (c *memoryCache) flush() error {
	if !c.persist {
		return nil
	}

	now := time.Now()
	entries := make(map[string]snapshotEntry)
	for i := range c.shards {
		s := &c.shards[i]

		s.mu.Lock()
		if !s.dirty {
			s.mu.Unlock()
			continue
		}
		for k, e := range s.items {
			if e.expired(now) {
				continue
			}
			var exp int64
			if !e.expireAt.IsZero() {
				exp = e.expireAt.UnixNano()
			}
			entries[k] = snapshotEntry{Value: e.value, ExpireAt: exp}
		}
		s.dirty = false
		s.mu.Unlock()
	}

	if len(entries) == 0 {
		return nil
	}
	if err := writeSnapshot(c.path, entries); err != nil {
		c.markAllDirty()
		return err
	}
	return nil
}

func (c *memoryCache) markAllDirty() {
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
	}
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
			c.sweepExpired(now)
		}
	}
}

// sweepExpired 逐段清理过期键，每段单独持锁。
//
// 单锁实现里这一步是对整张表的全表扫描：表一大，一次 tick 就能把读写全部卡住。
// 分段之后每次只锁 1/shardCount 的表，相邻段的读写可以照常进行。
func (c *memoryCache) sweepExpired(now time.Time) {
	for i := range c.shards {
		s := &c.shards[i]

		s.mu.Lock()
		for k, e := range s.items {
			if e.expired(now) {
				delete(s.items, k)
				s.dirty = true
			}
		}
		s.mu.Unlock()
	}
}

func expireAt(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}
