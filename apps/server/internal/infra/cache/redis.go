package cache

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/app/internal/config"
)

type redisCache struct {
	client *redis.Client
	prefix string
}

func newRedis(cfg config.Cache) *redisCache {
	timeout := time.Duration(cfg.Redis.DialTimeout) * time.Second
	return &redisCache{
		client: redis.NewClient(&redis.Options{
			Addr:         cfg.Redis.Addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			DialTimeout:  timeout,
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
		}),
		prefix: cfg.Prefix,
	}
}

func (c *redisCache) k(key string) string { return c.prefix + key }

func (c *redisCache) Get(ctx context.Context, key string) (string, error) {
	v, err := c.client.Get(ctx, c.k(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (c *redisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	// redis 的 Set 传负 TTL 会报错，这里统一按「永不过期」处理，
	// 与 memory 实现（ttl<=0 即无过期）保持一致。
	if ttl < 0 {
		ttl = 0
	}
	return c.client.Set(ctx, c.k(key), value, ttl).Err()
}

func (c *redisCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = c.k(k)
	}
	return c.client.Del(ctx, prefixed...).Err()
}

// Incr 用 pipeline 把「自增 + 重置过期」合并成一个往返。
// ttl<=0 时不发 EXPIRE —— redis 的 EXPIRE 传 0 会直接删键，与 memory 语义相反。
func (c *redisCache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	fullKey := c.k(key)

	pipe := c.client.TxPipeline()
	incr := pipe.Incr(ctx, fullKey)
	if ttl > 0 {
		// 用 PEXPIRE 而不是 EXPIRE：后者精度是秒，亚秒 TTL 会被向上取整，
		// 与 memory 实现（纳秒精度）不一致。
		ms := ttl.Milliseconds()
		if ms < 1 {
			ms = 1
		}
		pipe.PExpire(ctx, fullKey, time.Duration(ms)*time.Millisecond)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		if strings.Contains(err.Error(), "not an integer") {
			return 0, ErrNotInteger
		}
		return 0, err
	}
	return incr.Val(), nil
}

func (c *redisCache) Ping(ctx context.Context) error { return c.client.Ping(ctx).Err() }

func (c *redisCache) Driver() string { return config.CacheRedis }

func (c *redisCache) Close() error { return c.client.Close() }
