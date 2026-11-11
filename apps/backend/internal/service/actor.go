package service

import (
	"context"

	"example.com/app/internal/server/ctxs"
)

// WithActor 把当前登录身份挂到 context 上。
//
// 为什么要在 HTTP 上下文之外再复制一份：框架的上下文只陪到 handler，进了 service
// 就只剩 context.Context。而「谁能看全部数据」「这条记录归谁」这类判断属于业务规则，
// 只能靠参数一路透传才不会丢——每加一处判断就改一次方法签名，很快就没人愿意加了。
//
// 存取本身在 ctxs（key 与读写集中在那一个包里），这里只做类型化：值进 ctxs 时是 any，
// 回来时收窄成 *Principal。类型不能搬进 ctxs——那边一 import service 就与
// service → ctxs 成环。
func WithActor(ctx context.Context, p *Principal) context.Context {
	if p == nil {
		return ctx
	}
	return ctxs.WithActor(ctx, p)
}

// ActorFrom 取出当前登录身份。内部调用（定时任务、迁移、服务间调用）没有身份，
// 此时 ok=false，需要身份的判断应当拒绝，而不是当作管理员。
func ActorFrom(ctx context.Context) (*Principal, bool) {
	v, ok := ctxs.Actor(ctx)
	if !ok {
		return nil, false
	}
	p, ok := v.(*Principal)
	return p, ok && p != nil
}
