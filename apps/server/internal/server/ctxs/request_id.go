// Package ctxs 保存与单次请求绑定的上下文数据：request_id 与登录身份。
package ctxs

import "context"

type requestIDKey struct{}

// WithRequestID 把请求 ID 挂到 context 上，由 RequestID 中间件调用。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID 取请求 ID；取不到返回空串。
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
