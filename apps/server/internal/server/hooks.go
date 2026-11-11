package server

import (
	"log/slog"
	"net/http"

	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/ctxs"
)

// 本文件是 Fuego 的错误输出钩子（成功响应那条在 envelope.go）。
//
// 放在这里而不是 pkg/response：它要把框架的 (w, r, err) 接到统一响应上，
// 而「request_id 存在哪」属于 HTTP 层（ctxs）。pkg 是叶子包，读不到它，
// 也不该为了这个字段反向依赖 internal 下的包。

// errorSerializer 输出失败响应。
func errorSerializer(log *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		response.Fail(w, log, ctxs.RequestInfo(r), err)
	}
}
