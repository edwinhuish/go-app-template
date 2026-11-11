package ctxs

import (
	"context"
	"time"

	"example.com/app/internal/dto"
	"example.com/app/internal/pkg/role"
)

// Principal 是鉴权通过后的身份信息，由 Auth 中间件写进请求上下文。
type Principal struct {
	UserID    uint
	Username  string
	Nickname  string
	Role      string
	TokenID   string
	ExpiresAt time.Time
}

// IsAdmin 报告当前身份是否为管理员。
//
// 判断本身在 pkg/role：这里只要一个答案，不想因此认识 ORM。
func (p *Principal) IsAdmin() bool { return role.SeesAllData(p.Role) }

// Info 把身份转成对外的账号信息，`GET /auth/profile` 直接返回它。
//
// 转换放在这里是为了让「账号信息长什么样」只有一份构造点：Profile 拿得到身份、
// 拿不到账号模型，而登录响应走的是 model.User，两条来源的字段清单必须一致。
func (p *Principal) Info() dto.UserInfo {
	return dto.UserInfo{
		ID:       p.UserID,
		Username: p.Username,
		Nickname: p.Nickname,
		Role:     p.Role,
	}
}

type actorKey struct{}

// WithActor 写入登录身份；传 nil 不写入。
func WithActor(ctx context.Context, p *Principal) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, actorKey{}, p)
}

// ActorFrom 取出登录身份；没有时 ok=false，调用方应当拒绝，而不是当作管理员。
func ActorFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(actorKey{}).(*Principal)
	return p, ok && p != nil
}
