package server

import (
	"log/slog"
	"net/http"

	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/reqctx"
)

// 本文件是 Fuego 的两个输出钩子。
//
// 放在这里而不是 pkg/response：它们要把框架的 (w, r, v) 接到统一响应上，
// 而「request_id 存在哪」属于 HTTP 层（reqctx）。pkg 是叶子包，读不到它，
// 也不该为了这个字段反向依赖 internal 下的包。

// serialize 序列化成功响应。
//
// 刻意不做内容协商：框架默认会按 Accept 头把 JSON 改成 HTML / XML / 纯文本，
// 那会让「所有接口只返回 JSON」这条契约取决于请求头。
//
// 也不写状态码：显式状态码（新建的 201）已经由框架在序列化之前写出去了，
// 这里再写一次只会触发 superfluous WriteHeader。
func serialize(w http.ResponseWriter, _ *http.Request, v any) error {
	return response.WriteBody(w, v)
}

// errorSerializer 输出失败响应。
func errorSerializer(log *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		response.Fail(w, log, reqctx.Info(r), err)
	}
}
