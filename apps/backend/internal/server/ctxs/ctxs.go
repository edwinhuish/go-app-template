// Package ctxs 保存与单次请求绑定的 HTTP 层数据。
//
// 为什么单独成一个包：这些数据既要在中间件里写（RequestID 生成、AccessLog 记录），
// 又要在序列化与 handler 里读。放在任何一个「使用者」包里，另一个方向就得反过来
// import 它——pkg/response 是叶子包，handler 也不该认识中间件。集中在这里之后，
// 读写双方都只依赖它，key 也只有一处定义。
//
// 登录身份也在这里（见 actor.go）：key 与读写属于本包，值的类型属于 service，
// 由 service.WithActor / service.ActorFrom 做类型化。这样 service 不必也不该
// 在自己的包里再定义一份 key。
//
// 新增一类请求级数据时遵循同一条：key 与读写放这里，不要在中间件里就地定义 key；
// 值的类型留在它真正所属的包，本包按 any 存取。
package ctxs

import (
	"context"
	"net/http"

	"example.com/app/internal/pkg/response"
)

type requestIDKey struct{}

// WithRequestID 把请求 ID 挂到 context 上，由 RequestID 中间件调用。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID 取请求 ID；取不到返回空串，消费方按空值处理，不要 panic。
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// Info 汇总写响应与日志时需要的请求信息。
//
// 这样 pkg/response 不必认识 *http.Request，也不必知道 request_id 存在哪个 key 上——
// 它只负责按给定的值拼响应体。
func Info(r *http.Request) response.RequestInfo {
	return response.RequestInfo{
		Method:    r.Method,
		Path:      r.URL.Path,
		RequestID: RequestID(r.Context()),
	}
}
