package server

import (
	"net/http"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"

	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/ctxs"
)

// 本文件是「handler 只返回业务数据」与「所有成功响应带信封」之间的唯一接缝。
//
// 信封在 Fuego 的序列化钩子里组装，而不是在每个 handler 外面套一层装饰器：
// 钩子是唯一同时握有「返回值」与「请求」的位置——data 就是返回值本身，
// request_id 从 ctx 取。于是 controller 的签名就是业务类型，注册处也不必
// 逐条再包一层（少写一层，也就没有「新接口忘了包」这种错法）。
//
// 代价是文档：Fuego 按 controller 的返回类型生成 2xx 响应的 schema，钩子里
// 包的信封它看不见。所以注册完路由要用 envelopeSuccessResponses 把 2xx 的
// schema 换成信封——与 sanitizeErrorResponses 同一个手法，同一个理由：
// 文档与真实响应不一致，比没有文档更费时间。

// envelope 是成功响应的线格式。
//
// Data 是 any 而不是泛型：类型信息只对文档有意义，而文档由
// envelopeSuccessResponses 从路由的返回类型补，序列化这一层不需要它。
//
// 刻意不给 Data 加 omitempty：它可能是切片，encoding/json 会把空切片省略掉，
// 与「list 永远是 [] 而不是 null」的约定冲突。失败响应没有 data，那是另一个结构。
type envelope struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"request_id,omitempty"`
}

// serialize 是交给 Fuego 的序列化钩子（fuego.WithSerializer）：
// 框架每写出一条成功响应，都要经过这里。
//
// 刻意不做内容协商：框架默认会按 Accept 头把 JSON 改成 HTML / XML / 纯文本，
// 那会让「所有接口只返回 JSON」这条契约取决于请求头。
//
// 也不写状态码：显式状态码（新建的 201）已经由框架在序列化之前写出去了，
// 这里再写一次只会触发 superfluous WriteHeader——201 与 200 的区别因此
// 只由路由上的 OptionDefaultStatusCode 表达，与响应体无关。
func serialize(w http.ResponseWriter, r *http.Request, v any) error {
	// 文档路由（/openapi.json）也走 Flow，但它不是业务接口：Swagger UI 按 OpenAPI
	// 规范解析响应体，多套一层 data 就打不开了。判据用类型而不是路径——路径来自配置，
	// 序列化器不该知道它。
	if spec, ok := v.(openapi3.T); ok {
		return response.WriteBody(w, spec)
	}

	return response.WriteBody(w, envelope{
		// 成功只有这一种 message：客户端按 code 判断成败，成功时不会去读 message。
		// 「新建」与「查询」的区别不该用文案表达——文案一多，前端就得为一个
		// 没有语义的字段写分支。
		Code:      0,
		Message:   "ok",
		Data:      v,
		RequestID: ctxs.RequestID(r.Context()),
	})
}

// envelopeSuccessResponses 把文档里所有 2xx 响应的模型换成统一信封。
//
// 不改的话文档里 200 的 schema 会是 handler 的业务类型本身（例如 NoteItem），
// 而真实响应外面还有 code / message / data / request_id 一层。
func envelopeSuccessResponses(spec *openapi3.T) {
	if spec == nil {
		return
	}

	for _, item := range spec.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op.Responses == nil {
				continue
			}
			for code, resp := range op.Responses.Map() {
				status, err := strconv.Atoi(code)
				if err != nil || status < 200 || status > 299 || resp == nil || resp.Value == nil {
					continue
				}
				for _, media := range resp.Value.Content {
					if media == nil || media.Schema == nil {
						continue
					}
					media.Schema = successEnvelope(media.Schema)
				}
			}
		}
	}
}

// successEnvelope 把一条业务 schema 包进信封。
//
// data 原样挂上去而不是重建：它可能是 $ref（具名结构体），也可能是内联的
// 数组或基础类型，重建会丢掉 Fuego 已经算好的类型信息。
func successEnvelope(data *openapi3.SchemaRef) *openapi3.SchemaRef {
	obj := openapi3.NewObjectSchema().
		WithProperty("code", described(openapi3.NewIntegerSchema(), "业务码，0 表示成功")).
		WithProperty("message", described(openapi3.NewStringSchema(), "给人看的文案，成功时恒为 ok")).
		WithRequired([]string{"code", "message", "data"})
	obj.Properties["data"] = data
	return &openapi3.SchemaRef{Value: obj}
}
