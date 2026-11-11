// Package server 负责组装 Fuego 引擎、路由表与中间件链。
//
// 路由集中在这一处定义，方便一眼看清 API 全貌；handler 本身不关心自己被挂在哪个路径下。
// 每注册一条路由，OpenAPI 文档里就同步多出一条：契约与路由是同一个动作，
// 不存在「代码里加了接口、文档忘了改」这种中间状态。
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"

	"example.com/app/internal/config"
	"example.com/app/internal/handler"
	"example.com/app/internal/pkg/response"
	"example.com/app/internal/server/middleware"
	"example.com/app/internal/service"
)

// Handlers 汇总所有 HTTP handler，由 bootstrap 注入。
//
// 显式字段而不是 map 或反射：少一个 handler 编译期就会报错，
// 而 map 要等到运行到那条路由才发现是 nil。
type Handlers struct {
	Auth *handler.AuthHandler
	Note *handler.NoteHandler
	User *handler.UserHandler
}

const (
	// bodyLimitBytes 是单个请求体的兜底上限。取 8MB：普通 JSON 接口远用不到，
	// 而它挡住的是「几 GB 的 POST 把内存吃光」这类请求。
	bodyLimitBytes = 8 << 20

	// requestTimeout 是单个请求的处理上限。
	// 卡住时早点返回 504，比让浏览器转到超时更好查——日志里会留下带 request_id 的记录。
	requestTimeout = 30 * time.Second

	// securitySchemeBearer 是 spec 里鉴权方案的键名，路由与组件必须用同一个。
	securitySchemeBearer = "bearerAuth"
)

// Server 是 HTTP 层的成品：路由已注册、中间件已串好，对外只暴露 Handler()。
//
// 刻意不把 *fuego.Server 传出去：能拿到它就等于能绕过这里再注册一条路由，
// 而那条路由不会有 OpenAPI 描述——「完全 code-first」的那点保证就破了。
type Server struct {
	fg      *fuego.Server
	handler http.Handler
}

// Handler 返回可直接交给 http.Server 的处理器。
func (s *Server) Handler() http.Handler { return s.handler }

// OpenAPISpec 返回已生成的 OpenAPI 文档，供导出与测试使用。
func (s *Server) OpenAPISpec() *openapi3.T { return s.fg.OpenAPI.Description() }

// OpenAPIJSON 把文档序列化成 JSON。
func (s *Server) OpenAPIJSON() ([]byte, error) {
	return json.MarshalIndent(s.OpenAPISpec(), "", "  ")
}

// New 构建整套 HTTP 层。
//
// version 只用于文档里的版本号，与二进制自己的 -version 同源。
func New(
	cfg *config.Config, log *slog.Logger, auth service.AuthService, h Handlers, version string,
) *Server {
	fg := newFuegoServer(cfg, log, version)

	registerRoutes(fg, h, auth)
	// 兜底 404 直接挂在 mux 上，不走 Fuego：它是基础设施而不是业务接口，
	// 不该出现在文档里，也不该占用一个 operation。
	fg.Mux.Handle("/", http.HandlerFunc(notFound))

	// 文档路由最后注册：它自己也是两条路由，先注册会让它们排在业务路由之前，
	// 出问题时不容易判断「文档是不是把业务路由盖住了」。
	fg.RegisterOpenAPIRoutes(fg)
	sanitizeErrorResponses(fg.OpenAPI.Description())

	return &Server{
		fg: fg,
		handler: middleware.Chain(
			// 顺序有讲究：
			//   - RequestID 最外层，后面所有日志（含访问日志）才带得上它；
			//   - AccessLog 在 Recovery 外面，panic 时它仍在正常的返回路径上，
			//     访问日志不会因为一次 panic 整条消失；
			//   - Recovery 包住业务链路，panic 时由它决定「补 500」还是「断开连接」。
			middleware.RequestID(),
			middleware.AccessLog(log),
			middleware.Recovery(log),
			middleware.CORS(cfg.HTTP.CORSOrigins),
			middleware.BodyLimit(bodyLimitBytes),
			middleware.RequestTimeout(requestTimeout),
		)(fg.Mux),
	}
}

// newFuegoServer 装配 Fuego。
func newFuegoServer(cfg *config.Config, log *slog.Logger, version string) *fuego.Server {
	return fuego.NewServer(
		// 关掉 Fuego 自带的访问日志：它会把每个请求再打一行，与 AccessLog 重复。
		// 注意不要用 fuego.WithoutLogger()——它会把 slog 的默认 handler 换成 io.Discard，
		// 连带把我们自己的日志一起静音。
		fuego.WithLoggingMiddleware(fuego.LoggingConfig{
			DisableRequest:  true,
			DisableResponse: true,
		}),
		// 拒绝未知字段会让 {"title":"x","extra":1} 变成 400，与改造前不一致。
		// 注意这条只在 handler 调 c.Body() 时生效；本服务的解析走 bindJSON
		// （要自己的中文文案），所以它是给「将来改用框架绑定」留的正确缺省。
		fuego.WithDisallowUnknownFields(false),
		// 请求体上限：与 middleware.BodyLimit 用同一个值，而不是写 0。
		// Fuego 把 0 解释成「不限制」（body() 里 only if MaxBodySize != 0 才包
		// MaxBytesReader），而 HTTP 层真实生效的是 BodyLimit。两处写同一个数，
		// 将来谁改成 c.Body() 都不会出现「框架不限制、中间件限制」的隐性差异。
		fuego.WithMaxBodySize(bodyLimitBytes),
		fuego.WithSerializer(serialize),
		fuego.WithErrorSerializer(errorSerializer(log)),
		// 所有已注册路由的响应对外只有 JSON 一种格式，包括带显式状态码的那几条。
		// 挂成路由级而不是全局：404 兜底与 CORS 预检各有自己的 Content-Type。
		fuego.WithRouteOptions(fuego.OptionMiddleware(middleware.ContentTypeJSON())),
		fuego.WithSecurity(openapi3.SecuritySchemes{
			securitySchemeBearer: &openapi3.SecuritySchemeRef{
				Value: openapi3.NewSecurityScheme().
					WithType("http").
					WithScheme("bearer").
					WithBearerFormat("JWT"),
			},
		}),
		fuego.WithEngineOptions(
			// 去掉 Fuego 自动加的 X-Powered-By / Server-Timing / Trailer 三个响应头：
			// 改造前的响应里没有它们，凭空多出来会让网关与客户端的白名单对不上。
			fuego.WithoutFlowHeaders(),
			// 错误原样交给序列化器：统一信封的形状由 pkg/response 决定，
			// 不能被折成 Fuego 自己的 {title,status,detail}。
			fuego.WithErrorHandler(func(_ context.Context, err error) error { return err }),
			// 只承认 JSON。框架默认把 application/xml 也写进文档，而本服务根本没有 XML 分支；
			// 请求体那边的默认值是 */*，同样会让文档看起来「收什么都行」。
			fuego.WithRequestContentType("application/json"),
			fuego.WithResponseContentType("application/json"),
			// 让 binding 标签进入 schema，否则文档里的字段没有任何长度约束。
			fuego.WithOpenAPIGeneratorSchemaCustomizer(bindingSchemaCustomizer),
			fuego.WithOpenAPIConfig(openapiConfig(cfg, version)),
		),
	)
}

// openapiConfig 是文档相关的配置。
func openapiConfig(cfg *config.Config, version string) fuego.OpenAPIConfig {
	return fuego.OpenAPIConfig{
		Info: &openapi3.Info{
			Title:       cfg.App.Name + " API",
			Version:     version,
			Description: "由后端代码生成，与路由注册同源；业务码表见各接口的错误响应说明。",
		},
		SpecURL:    "/openapi.json",
		SwaggerURL: "/docs",
		// 不落盘：启动时顺手写一个 doc/openapi.json 会让工作区凭空多出文件。
		// 需要静态产物时从运行中的服务导出即可。
		DisableLocalSave: true,
		// 生产不暴露文档路由：改造前没有这些路由，保持生产对外接口集合不变。
		Disabled: !docsEnabled(cfg),
	}
}

// docsEnabled 报告是否暴露 OpenAPI 文档。
func docsEnabled(cfg *config.Config) bool {
	return cfg.HTTP.Mode != "release" && !cfg.IsProd()
}

// registerRoutes 注册全部路由。
//
// 每条路由的选项里同时写着「谁能调」与「会怎么失败」：
// 权限漏写等于开了一条公开接口，失败码漏写等于文档少一页——
// 两者都靠这一处集中表达，而不是散在 handler 里。
func registerRoutes(s *fuego.Server, h Handlers, auth service.AuthService) {
	// 需要登录的路由统一带：鉴权中间件、文档里的 security、以及最常见的失败码。
	// 加同类接口时复用 secured，别自己拼 OptionMiddleware —— 漏挂不会报错。
	secured := slices.Concat(
		[]fuego.RouteOption{
			fuego.OptionMiddleware(middleware.Auth(auth)),
			fuego.OptionSecurity(openapi3.SecurityRequirement{securitySchemeBearer: {}}),
		},
		errorResponses(http.StatusBadRequest, http.StatusUnauthorized,
			http.StatusInternalServerError, http.StatusGatewayTimeout),
	)
	// 管理端：先校验令牌、再校验角色，所以 Auth 写在 Chain 的前面（外层）。
	adminOnly := slices.Concat(
		[]fuego.RouteOption{
			fuego.OptionMiddleware(
				middleware.Chain(middleware.Auth(auth), middleware.RequireAdmin())),
			fuego.OptionSecurity(openapi3.SecurityRequirement{securitySchemeBearer: {}}),
		},
		errorResponses(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
			http.StatusInternalServerError, http.StatusGatewayTimeout),
	)

	fuego.Get(s, "/healthz", ok(handler.Health),
		doc("health", "存活检查", "system")...,
	)

	fuego.Post(s, "/api/v1/auth/login", ok(h.Auth.Login),
		slices.Concat(
			doc("login", "账号密码登录", "auth"),
			errorResponses(http.StatusBadRequest, http.StatusUnauthorized,
				http.StatusForbidden, http.StatusInternalServerError, http.StatusGatewayTimeout),
		)...,
	)

	fuego.Get(s, "/api/v1/auth/profile", ok(h.Auth.Profile),
		slices.Concat(doc("getProfile", "当前登录身份", "auth"), secured)...,
	)

	fuego.Post(s, "/api/v1/auth/logout", ok(h.Auth.Logout),
		slices.Concat(doc("logout", "登出并失效当前会话", "auth"), secured)...,
	)

	fuego.Get(s, "/api/v1/notes", ok(h.Note.List),
		slices.Concat(
			doc("listNotes", "分页查询示例记录", "note"),
			secured,
			[]fuego.RouteOption{
				fuego.OptionQueryInt("page", "页码，从 1 开始；非数字或小于 1 均归 1"),
				fuego.OptionQueryInt("page_size", "每页条数，默认 20；非数字归默认，超过 200 截断"),
				fuego.OptionQuery("keyword", "标题关键字，空表示不过滤"),
				fuego.OptionQueryInt("owner_id", "按创建者筛选，0 表示不过滤；非管理员只能看到自己的记录"),
			},
		)...,
	)

	// created 与 OptionDefaultStatusCode 说的是同一件事（新建成功），
	// 前者决定响应体的 message，后者决定状态码与文档。两者不一致时线格式比对会失败。
	fuego.Post(s, "/api/v1/notes", created(h.Note.Create),
		slices.Concat(
			doc("createNote", "新建示例记录", "note"),
			[]fuego.RouteOption{fuego.OptionDefaultStatusCode(http.StatusCreated)},
			secured,
			errorResponses(http.StatusConflict),
		)...,
	)

	fuego.Get(s, "/api/v1/notes/{id}", ok(h.Note.Get),
		slices.Concat(doc("getNote", "按 ID 查询示例记录", "note"), secured,
			errorResponses(http.StatusNotFound))...,
	)

	fuego.Put(s, "/api/v1/notes/{id}", ok(h.Note.Update),
		slices.Concat(doc("updateNote", "更新示例记录（可选乐观锁）", "note"), secured,
			errorResponses(http.StatusNotFound, http.StatusConflict))...,
	)

	fuego.Delete(s, "/api/v1/notes/{id}", ok(h.Note.Delete),
		slices.Concat(doc("deleteNote", "软删除示例记录", "note"), secured,
			errorResponses(http.StatusNotFound))...,
	)

	fuego.Get(s, "/api/v1/users", ok(h.User.List),
		slices.Concat(doc("listUsers", "账号目录（仅管理员）", "user"), adminOnly)...,
	)
}

// doc 是一条路由的通用文档选项。
func doc(operationID, summary, tag string) []fuego.RouteOption {
	return []fuego.RouteOption{
		fuego.OptionOperationID(operationID),
		fuego.OptionTags(tag),
		fuego.OptionSummary(summary),
	}
}

// errorResponses 为指定的状态码声明失败响应。
//
// 用 OptionAddResponse 而不是 OptionAddError：后者已被标记弃用，
// 且它走的是「已存在就不覆盖」的路径，400/500 会被框架的默认值占住。
func errorResponses(codes ...int) []fuego.RouteOption {
	opts := make([]fuego.RouteOption, 0, len(codes))
	for _, code := range codes {
		opts = append(opts, fuego.OptionAddResponse(
			code,
			errorResponseDescriptions[code],
			fuego.Response{
				Type:         response.ErrorBody{},
				ContentTypes: []string{"application/json"},
			},
		))
	}
	return opts
}

// errorSchemaName 是失败响应共用的 schema 组件名。
const errorSchemaName = "ErrorBody"

// errorResponseDescriptions 是各失败状态码在文档里的口径，与 pkg/errs 的码表一致。
var errorResponseDescriptions = map[int]string{
	http.StatusBadRequest:            "参数错误（code 40001）：绑定失败或业务校验不通过",
	http.StatusUnauthorized:          "未登录或登录态失效（code 40101）",
	http.StatusForbidden:             "已登录但无权限（code 40301）",
	http.StatusNotFound:              "资源不存在（code 40401）",
	http.StatusConflict:              "唯一约束 / 乐观锁命中 0 行 / 状态不允许（code 40901）",
	http.StatusRequestEntityTooLarge: "请求体过大（code 40001）",
	http.StatusInternalServerError:   "服务器内部错误（code 50001），真实原因只进日志",
	http.StatusGatewayTimeout:        "请求处理超时（code 50401）",
}

// sanitizeErrorResponses 把文档里所有失败响应的模型换成统一信封。
//
// Fuego 默认给 400 与 500 挂的是它自己的 {title,status,detail}，而本服务实际返回
// {code,message,request_id}。不改的话文档比没有文档更误导人——照着写的前端会读错字段。
func sanitizeErrorResponses(spec *openapi3.T) {
	if spec == nil || spec.Components == nil {
		return
	}
	if spec.Components.Schemas == nil {
		spec.Components.Schemas = openapi3.Schemas{}
	}

	// 无条件覆盖：OptionAddResponse 已经从 response.ErrorBody 生成过一份同名 schema，
	// 但那份没有字段说明；留着它等于把文档里最需要解释的两个字段写成一串类型名。
	spec.Components.Schemas[errorSchemaName] = &openapi3.SchemaRef{Value: openapi3.NewObjectSchema().
		WithProperty("code", described(openapi3.NewIntegerSchema(), "业务码，非 0 表示失败")).
		WithProperty("message", described(openapi3.NewStringSchema(), "给人看的文案，可直接展示")).
		WithProperty("request_id", described(openapi3.NewStringSchema(), "与响应头 X-Request-ID 一致")).
		WithRequired([]string{"code", "message"})}

	// 用 $ref 而不是内联：43 处失败响应各写一份，等于把同一个结构复制 43 遍，
	// 改一处字段就得改 43 处，而漏改的表现是文档里两种错误形状并存。
	content := openapi3.NewContentWithSchemaRef(
		&openapi3.SchemaRef{Ref: "#/components/schemas/" + errorSchemaName},
		[]string{"application/json"},
	)

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
				if err != nil || status < http.StatusBadRequest || resp == nil || resp.Value == nil {
					continue
				}
				if desc, ok := errorResponseDescriptions[status]; ok {
					resp.Value.Description = &desc
				}
				resp.Value.Content = content
			}
		}
	}
}

// notFound 是兜底 404，输出与改造前逐字一致。
//
// 用 mux 的默认 404 会是 text/plain; charset=utf-8 加一个换行；
// 这里的形状（text/plain、无换行）是既有行为，网关的规则可能按它匹配。
func notFound(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("404 page not found"))
}

// described 设置 schema 的描述。kin-openapi 没有 WithDescription，
// 而字段说明对读文档的人是主要信息量，所以这里补一个短小的构造函数式辅助。
func described(s *openapi3.Schema, desc string) *openapi3.Schema {
	s.Description = desc
	return s
}
