// Package response 统一 HTTP 错误响应的写出口。
//
// 对外只有一种形状（成功那条在 internal/server/envelope.go 的序列化钩子里组装）：
//
//	{"code":40101,"message":"...","request_id":"..."}
//
// 失败时没有 data，前端按 code 分支处理；HTTP 状态码只由业务码推导
// （取 code / 100 的前三位），任何一层都不要为某个错误单独挑状态码。
//
// 本包只依赖 net/http 与 pkg/errs，且**不读请求上下文**：请求级数据（request_id）
// 由调用方从 internal/server/ctxs 取来传进来。pkg 是叶子包，不 import internal
// 下的其它包；把「存哪儿」的知识留在这里也会逼着它认识 HTTP 层的上下文。
//
// 它之所以还在，是因为中间件（鉴权、限流、恢复、体积上限）在 Fuego 的 Flow 之外：
// 它们拿不到 handler 的返回值与错误，也没有错误序列化器可用，只能自己写响应体。
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

// Fail 输出错误响应：4xx 的业务错误原样透出，5xx 统一折叠文案。
//
// 折叠不等于丢掉：响应体只留一句「服务器内部错误」，真实原因必须进日志，
// 否则线上只能靠复现去猜。业务错误（4xx）不记，避免用户输错参数刷成告警。
//
// 折叠是**无条件**的，不看调用方写的文案：service 里 `errs.Wrap(err, CodeInternal,
// "查询账号失败")` 这种写法很自然，但它会把内部语义（连表名、调用顺序都猜得出来）
// 直接送到客户端，也与契约「真实原因只进日志」冲突。所以要折的是文案而不是错误码，
// 码保留，前端仍按 code 分支。
//
// 504 不折：它的文案（「请求处理超时，请稍后重试」）是给调用方的动作提示，
// 不含内部信息；只有映射到 500 的才换成统一文案。
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
		e = errs.Wrap(err, errs.CodeInternal, errs.ErrInternal.Msg)
	}

	status := e.Code.HTTPStatus()
	if status >= http.StatusInternalServerError {
		log.Error("服务端错误",
			"method", req.Method,
			"path", req.Path,
			"request_id", req.RequestID,
			"err", err,
		)
	}
	if status == http.StatusInternalServerError {
		e = errs.New(e.Code, errs.ErrInternal.Msg)
	}

	_ = WriteJSON(w, status, ErrorBody{
		Code:      int(e.Code),
		Message:   e.Msg,
		RequestID: req.RequestID,
	})
}

// PayloadTooLarge 写 413。
//
// 形状与其它失败响应完全一致（code / message / request_id）：413 虽然不是业务错误，
// 但它同样是「出错了，去查日志」的场景，而前端找日志只能靠 request_id。
// 早先这里刻意不带 request_id，结果只有超限的请求在日志里对不上号。
//
// 业务码复用 40001（没有单独的「太大」码）：状态码本身已经把区别说清楚了，
// 再加一个码只会让前端的 switch 多一个等价分支。
func PayloadTooLarge(w http.ResponseWriter, req RequestInfo, limit int64) {
	msg := "请求体过大"
	if limit > 0 {
		msg += "（上限 " + humanSize(limit) + "）"
	}
	_ = WriteJSON(w, http.StatusRequestEntityTooLarge, ErrorBody{
		Code:      int(errs.CodeInvalidParam),
		Message:   msg,
		RequestID: req.RequestID,
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
func HandleTooLarge(w http.ResponseWriter, req RequestInfo, err error) bool {
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) {
		return false
	}
	PayloadTooLarge(w, req, tooLarge.Limit)
	return true
}

func humanSize(n int64) string {
	const mb = 1 << 20
	if n >= mb {
		return strconv.FormatInt(n/mb, 10) + "MB"
	}
	return strconv.FormatInt(n/(1<<10), 10) + "KB"
}
