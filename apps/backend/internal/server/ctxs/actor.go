package ctxs

import "context"

// actorKey 是登录身份在请求上下文里的键。
//
// 值按 any 存取，类型化（*service.Principal）留在 service：本包 import service 会成环。
type actorKey struct{}

// WithActor 写入登录身份；传 nil 不写入。
func WithActor(ctx context.Context, v any) context.Context {
	if v == nil {
		return ctx
	}
	return context.WithValue(ctx, actorKey{}, v)
}

// Actor 取出登录身份；没有时 ok=false，调用方应当拒绝，而不是当作管理员。
func Actor(ctx context.Context) (any, bool) {
	v := ctx.Value(actorKey{})
	return v, v != nil
}
