// Package response 统一 HTTP 响应格式。
//
// 所有接口（含错误）只有一种形状：
//
//	{"code":0,"message":"ok","data":...,"request_id":"..."}
//
// code != 0 时没有 data，前端按 code 分支处理；HTTP 状态码只由业务码推导
// （取 code / 100 的前三位），任何一层都不要为某个错误单独挑状态码。
//
// 本包只依赖 net/http 与 pkg/errs，且**不读请求上下文**：请求级数据（request_id）
// 由调用方从 internal/server/reqctx 取来传进来。pkg 是叶子包，不 import internal
// 下的其它包；把「存哪儿」的知识留在这里也会逼着它认识 HTTP 层的上下文。
package response

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"example.com/app/internal/pkg/errs"
)

// contentTypeJSON 是唯一的响应类型。显式带 charset：消费方的解析代码与网关规则
// 可能按这个字符串匹配，换实现不该改变它。
const contentTypeJSON = "application/json; charset=utf-8"

// Envelope 是统一响应体。
//
// Data 用泛型参数而不是 any：any 会让 OpenAPI 只能生成一个空对象，
// 前端在文档里看不到 data 的真实结构。泛型实例在反射时是具体类型，能生成完整 schema。
//
// 刻意不给 Data 加 omitempty：它可能是切片，而 encoding/json 会把空切片省略掉，
// 与「list 永远是 [] 而不是 null」的约定冲突。失败响应走 ErrorBody，本来就没有 data。
type Envelope[T any] struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      T      `json:"data"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorBody 是失败响应体：没有 data 字段。
type ErrorBody struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// RequestInfo 是写失败响应与记日志时需要的请求信息。
//
// 单独成为一个值而不是直接传 *http.Request：本包不该知道「请求 ID 存在 context
// 的哪个键上」，那是 HTTP 层的事；也不该为了两个日志字段把整个请求搬进来。
type RequestInfo struct {
	Method    string
	Path      string
	RequestID string
}

// ErrResponseWritten 表示响应已经由调用方写出（典型是 413：中间件先写，
// 解析层随后也会拿到一个错误）。错误序列化器见到它必须直接返回，
// 否则会在已写出的响应后面再追加一段 JSON。
var ErrResponseWritten = errors.New("response: 响应已写出")

// OK 构造 200 成功响应。
func OK[T any](requestID string, data T) Envelope[T] {
	return Envelope[T]{Code: 0, Message: "ok", Data: data, RequestID: requestID}
}

// Created 构造 201 创建成功响应。
func Created[T any](requestID string, data T) Envelope[T] {
	return Envelope[T]{Code: 0, Message: "created", Data: data, RequestID: requestID}
}

// WriteBody 只序列化并写响应体，不碰状态码。
//
// 给「状态码已经由别人决定」的调用方用：框架在序列化之前就把显式状态码
// （201 之类）写出去了，这里再写一次会触发 superfluous WriteHeader。
func WriteBody(w http.ResponseWriter, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	_, err = w.Write(b)
	return err
}

// WriteJSON 写状态码与响应体。
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_, err = w.Write(b)
	return err
}

// Fail 输出错误响应：业务错误原样透出，非业务错误统一折叠为 500。
//
// 折叠不等于丢掉：响应体只留一句「服务器内部错误」，真实原因必须进日志，
// 否则线上只能靠复现去猜。业务错误（4xx）不记，避免用户输错参数刷成告警。
func Fail(w http.ResponseWriter, log *slog.Logger, req RequestInfo, err error) {
	if errors.Is(err, ErrResponseWritten) {
		return
	}
	if log == nil {
		log = slog.Default()
	}

	e, ok := errs.From(err)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		// 超时不是「服务自己错了」，而是「这次没算完」：报 500 会让人去翻代码。
		e = errs.ErrTimeout
	case !ok:
		e = errs.Wrap(err, errs.CodeInternal, "服务器内部错误")
	}

	if e.Code.HTTPStatus() >= http.StatusInternalServerError {
		log.Error("服务端错误",
			"method", req.Method,
			"path", req.Path,
			"request_id", req.RequestID,
			"err", err,
		)
	}

	_ = WriteJSON(w, e.Code.HTTPStatus(), ErrorBody{
		Code:      int(e.Code),
		Message:   e.Msg,
		RequestID: req.RequestID,
	})
}

// PayloadTooLarge 写 413。
//
// 形状与其它失败响应不同：只有 code 与 message、没有 request_id。
// 这是改造前就有的形状，保持一致以免前端按 request_id 是否存在分支时出错。
func PayloadTooLarge(w http.ResponseWriter, limit int64) {
	msg := "请求体过大"
	if limit > 0 {
		msg += "（上限 " + humanSize(limit) + "）"
	}
	_ = WriteJSON(w, http.StatusRequestEntityTooLarge, ErrorBody{
		Code:    int(errs.CodeInvalidParam),
		Message: msg,
	})
}

// HandleTooLarge 把 MaxBytesReader 抛出的读错误翻成 413，返回是否已经写过响应。
//
// 放在这里而不是某个中间件包里：它决定的是响应的形状（code + message），
// 属于 response 的职责；调用方既有解析层（handler 读完 body 才知道超没超），
// 也可能有中间件。放中间件包里会逼着 handler 反向 import server 层。
//
// 不翻的话它会在解析层变成「请求参数格式不正确」这类 400，而 400 的意思是
// 「你的请求写错了」——实际是「太大了」，两种提示对应的动作完全不同。
func HandleTooLarge(w http.ResponseWriter, err error) bool {
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) {
		return false
	}
	PayloadTooLarge(w, tooLarge.Limit)
	return true
}

func humanSize(n int64) string {
	const mb = 1 << 20
	if n >= mb {
		return strconv.FormatInt(n/mb, 10) + "MB"
	}
	return strconv.FormatInt(n/(1<<10), 10) + "KB"
}
