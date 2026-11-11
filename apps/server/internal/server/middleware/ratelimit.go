package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
)

// Limiter 是限流需要的最小能力：按 key 自增并返回当前窗口内的累计次数。
//
// 只声明这一个方法而不是直接依赖 cache.Cache：中间件只需要计数，
// 认了缓存接口就等于把「限流」和「缓存实现」绑在一起，测试也要拖一个假缓存进来。
type Limiter interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// RateLimit 按调用方 IP 分桶限流：窗口内超过 limit 次直接 429。
//
// 为什么需要它：登录接口每次都要跑 bcrypt（几十毫秒 CPU），按用户名锁定只能
// 拦住「猜同一个账号」，拦不住「换着用户名刷」——那种请求每个用户名都只错一次，
// 一次锁定都不会触发，CPU 却照样被打满。这一层管的是调用方，那一层管的是账号。
//
// 已知边界：IP 取自 X-Forwarded-For（见 clientIP），客户端可以伪造它来换桶。
// 真正部署时必须在入口把该头改写掉；即使被绕过，按用户名锁定仍然把每个账号的
// 猜测次数压在上限内，所以绕过只影响「CPU 保护」而不影响「账号保护」。
func RateLimit(
	log *slog.Logger, limiter Limiter, scope string, limit int64, window time.Duration,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			key := "ratelimit:" + scope + ":" + clientIP(r)
			n, err := limiter.Incr(r.Context(), key, window)
			if err != nil {
				// 缓存不可用时放行，与登录失败计数的取舍一致（见 service.locked）：
				// 限流是为了保护自己，不该因为缓存抖动把所有人挡在门外。
				log.Warn("限流计数失败，本次放行", "scope", scope, "err", err)
				next.ServeHTTP(w, r)
				return
			}

			if n > limit {
				response.Fail(w, nil, reqInfo(r), errs.ErrTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
