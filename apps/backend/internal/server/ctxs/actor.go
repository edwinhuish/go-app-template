package ctxs

import "context"

// actorKey 是登录身份在请求上下文里的键。
//
// 值的类型（`service.Principal`）是业务概念，本包不 import service：那会变成
// ctxs → service，而 service 层的业务规则又要读身份（service → ctxs），绕成环。
// 所以这里只负责「存在哪个 key 上、怎么写怎么读」，值按 any 保管，
// 收窄回 *Principal 的那一步由 service 的 WithActor / ActorFrom 完成。
type actorKey struct{}

// WithActor 把登录身份放到请求上下文上，由 Auth 中间件在验证通过后调用。
// 传 nil 时不写入：下游要么拿到一个真身份，要么拿不到，不存在「存在但为空」。
func WithActor(ctx context.Context, v any) context.Context {
	if v == nil {
		return ctx
	}
	return context.WithValue(ctx, actorKey{}, v)
}

// Actor 取出 WithActor 放进去的值。内部调用（定时任务、迁移、服务间调用）没有身份，
// 此时 ok=false，需要身份的判断应当拒绝，而不是当作管理员。
func Actor(ctx context.Context) (any, bool) {
	v := ctx.Value(actorKey{})
	return v, v != nil
}
