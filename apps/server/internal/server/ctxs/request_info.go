package ctxs

import (
	"net/http"

	"example.com/app/internal/pkg/response"
)

// RequestInfo 收集写失败响应与记日志要用的请求信息。
//
// 放在 ctxs 而不是让每个调用方自己拼：request_id 存在哪个 key 上是 ctxs 的知识，
// 拼装点一多，必有一处漏带——漏带的表现是响应里悄悄少一个字段、日志里对不上号，
// 而不是报错。鉴权中间件、错误序列化器、413 都从这里取。
func RequestInfo(r *http.Request) response.RequestInfo {
	return response.RequestInfo{
		Method:    r.Method,
		Path:      r.URL.Path,
		RequestID: RequestID(r.Context()),
	}
}
