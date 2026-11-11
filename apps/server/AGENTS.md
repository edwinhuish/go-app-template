# apps/server — 后端开发规范

本文件是**后端规范的唯一正本**，只管 `apps/server/` 下的改动。目标不是描述项目，
而是约束改动：照着做，改出来的代码和现有的看起来像一个人写的。

**本文件自包含**：只描述本目录的约定、结构与用法，不引用任何目录之外的文档——仓库根的说明、
公共契约、`apps/` 下其他子项目的规范都不在这里出现。需要跨项目的信息（对外接口形状、任务名）
下面直接给出，读这一份就能开工。

## 0. 独立工程约束

- **改代码必须同改本文件。** 本文件写的是「现在长什么样」，不是历史。改到接口形状、分层位置、
  配置项、任务名、目录结构时，在同一个提交里更新对应段落：先改代码再想起来补文档，通常就忘了，
  下一个人拿到的是一份错的规范。确实来不及，就在提交说明里写清欠下哪一段、为什么。
- 不 import 仓库根或 `apps/` 下其他子项目的代码，也不读它们的文件当配置；与外部只通过 HTTP 交互。
- 不依赖本目录之外的任何文件才能构建：`task install && task build` 单独跑必须成功。
  唯一的软读取：`Taskfile.yml` 会把仓库根的 `VERSION` 当版本号缺省值读一次，
  读不到就退化为 `dev`（构建照常，只是产物自证 `dev`）。新增类似读取必须遵守同一条：缺失即降级。
- 外部编排方（仓库根的统一入口）按**任务名**调用本目录的 `Taskfile.yml`，所以这些名字要保持：
  `install`（可带 `-- --ci`）/ `build` / `dev` / `test` / `lint` / `fmt` / `clean`。
  单端入口是 `task server:<任务>`，全量入口派发这几个名字，没定义就跳过本端。
- 根目录有两种调法，都把参数**原样**转进来：`task server:<任务> -- <参数...>`（根 Taskfile 的
  `includes` 直连本目录，参数零解析）、以及全局命令的 `task -d apps/server <任务> -- <参数...>`。
  其中可能有属于其他子项目的开关，因此本端的任务只挑自己认识的参数用，
  对不认识的参数**一律忽略、不报错**（`install` 只认 `--ci`，`dev` 认 `--db=*` / `--once` / `--down`，
  其余任务不读参数）。参数值本身写错（如 `--db=oracle`）仍然报错——那是本端自己的开关。
  `run`、`tidy`、`tool:air` 只在本地使用（`task -d apps/server <任务>` 或 `task server:<任务>`）。
  改名前先确认调用方。
- 本端开发环境需要的容器由本端自己负责：编排文件是 `deployments/docker-compose.yml`，
  由 `dev -- --db=pg` / `dev -- --db=mysql` 在使用前拉起。不要指望仓库根去启动容器。

## 1. 对外接口约定

这一节是本服务对外的承诺，消费方照它对接，所以它是**兼容性约束**：改动等于破坏性变更。

| 项 | 约定 |
|---|---|
| 前缀 | 业务接口一律挂在 `/api/v1` 下 |
| 方法 | 只用 `GET` / `POST` / `PUT` / `DELETE` |
| 鉴权头 | `Authorization: Bearer <token>`；缺失或失效返回 401 |
| 请求 ID | 中间件生成或透传，响应头 `X-Request-ID` 与响应体 `request_id` 一致 |
| 字段命名 | json 一律 `snake_case` |
| 时间 | `time.Time` 序列化为 RFC3339；「业务日期」存本地日历日的 UTC 零点（不带时分秒）；令牌过期 `expires_at` 用 Unix 秒 |

响应体只有一种形状：

```json
{ "code": 0, "message": "ok", "data": {}, "request_id": "0f3c1a7e9b2d4c55" }
```

- `code = 0` 表示成功（HTTP 200，新建 201）；失败时 `data` 省略。
- **兜底响应不套信封**：未注册的路径回 `404`、路径存在但方法不对回 `405`（带 `Allow`），
  两者都是 `text/plain`。它们不是业务接口，所以不占用码表，也不要改成信封形状。
- `message` 是给人看的文案，消费方可直接展示；成功时恒为 `ok`（新建也是 `ok`，
  201 与 200 的差别看状态码，不看文案）。
- HTTP 状态码**只由业务码推导**：取 `code / 100` 的前三位。handler 不为某个错误单独挑状态码。

业务码表（新增码的前三位必须与 HTTP 状态一致，且只在 `internal/pkg/errs` 定义，不写字面量）：

| code | HTTP | 语义 |
|---|---|---|
| `0` | 200 / 201 | 成功 |
| `40001` | 400 | 参数错误 / 业务校验不通过 |
| `40101` | 401 | 未登录或登录态失效 |
| `40301` | 403 | 已登录但无权限 |
| `40401` | 404 | 资源不存在（含已软删除） |
| `40901` | 409 | 唯一约束、乐观锁命中 0 行、状态不允许流转 |
| `42901` | 429 | 请求过于频繁（同一 IP 的登录尝试超过阈值） |
| `50001` | 500 | 内部错误（非业务错误统一折叠，真实原因只进日志） |
| `50401` | 504 | 请求处理超时（ctx 截止时间到） |

两条**不占码表**但必须知道的失败：请求体超限是 `413`（`code` 仍为 `40001`，形状与其它错误一致、
带 `request_id`），单请求超时是 `504`（`50401`）。请求体上限 8MB、超时 30s，是 `router.go` 里的
常量，不可配置——改这两个数等于改对外行为，要同步契约。

分页：

| 入参 | 默认 | 上限 | 说明 |
|---|---|---|---|
| `page` | 1 | — | 非法值（含非数字）归 1 |
| `page_size` | 20 | 200 | 非法值（含非数字）归默认，超上限截断 |
| `keyword` | 空 | — | 各资源自行解释是否使用；用到的资源一律按**字面包含**匹配，`%` 与 `_` 不作通配符 |

响应 `{ "list": [], "total": 0, "page": 1, "page_size": 20 }`；`list` 空结果是 `[]` 而不是 `null`。

乐观锁（可选）：更新接口回传读到的 `updated_at` 原值，服务端据此判冲突；不传则退化为最后写入者获胜。

### 资源可见范围与登录锁定

这两条是行为承诺，前端能直接感知，改动按契约变更处理：

- **按身份隔离的资源**（当前是 `/api/v1/notes`）：管理员可见全部；其他角色只能看到自己创建的
  记录，请求里带了别人的 `owner_id` 也不会放开。越权访问单条记录返回 **404（40401）而不是
  403**——403 等于确认「这条 id 存在」，连续试就能数出别人的数据量。
- **登录失败锁定**：同一用户名连续失败达到 5 次后，15 分钟内一律返回 `40301`，期间不再比对
  密码；登录成功清零计数。它是「真拒绝」而不是「事后换文案」，否则爆破速率一点不会降。
  清零失败（缓存删除报错）不把登录翻成 500——令牌已经签出——但会重试一次并打一条 warn：
  那条日志意味着该账号可能在窗口期内被误锁。
- **登录按 IP 限流**：同一 IP 对 `/api/v1/auth/login` 超过 20 次/分钟返回 `42901`
  （`middleware.RateLimit`，计数走 `cache`）。两者互补：账号锁定限「猜同一个账号」，
  IP 限流限「换着用户名刷」——后者每个账号只错一次，账号锁定一次都不会触发，CPU 却被打满。
- **`PUT` 是全量覆盖**：省略 `content` 等于清空它。`omitempty` 只影响序列化，不影响反序列化，
  所以「没传」与「传空串」在服务端无法区分；要局部更新得先读后写，或另开 `PATCH`。

**改了 `internal/dto` 的字段名或 `internal/pkg/errs` 的码，就是在改这份承诺**：同一个提交里更新本节，
并在提交信息里点名「对外接口变更」，让消费方知道要跟。

### 自描述文档

OpenAPI 文档由路由注册同一处生成，不手写 yaml、不提交静态产物：

| 项 | 说明 |
|---|---|
| 生成时机 | 注册路由时同步写入 spec。加一条路由就多一条文档，不存在「代码改了文档没改」 |
| 访问入口 | `GET /openapi.json`（文档本体）、`GET /docs`（可交互页面） |
| 暴露条件 | 仅 `http.mode != release` 且 `app.env != prod`。生产不注册这两个路由，对外接口集合与无文档时一致 |
| 字段约束 | dto 的 `binding` 标签同时驱动运行时校验与 schema（`min`/`max`/`oneof`/`email`） |
| 必填 | 由 json 标签的 `omitempty` 决定（序列化语义） |
| 失败响应 | 用 `errorResponses(400, 401, 404 ...)` 逐条声明，文案取自本节码表 |

写请求结构体时有一条容易踩的：**`binding` 里没有 `required` 的字段，json 标签必须带 `omitempty`**。
不带的话文档会把它标成必填，而实际不传也能通过——文档比没有文档更容易误导人。
这类结构体只在请求方向使用，加 `omitempty` 不改变对外线格式。

## 2. 先确认改的是哪一层

| 你要做的事 | 落点 | 不要碰 |
|---|---|---|
| 加一个表 / 字段 / 索引 | `internal/model` | handler、dto |
| 加一条查询 | `internal/repository`（接口 + 实现） | service 里的 SQL |
| 加业务规则、状态流转、事务 | `internal/service` | repository 里的 if 判断 |
| 加接口参数、响应字段 | `internal/dto` | 直接改 model 的 json tag |
| 加路由 / 中间件 | `internal/server` | handler 里拼状态码 |
| 加连接、缓存、日志实现 | `internal/infra` | service 里 new 客户端 |

依赖方向只能从上往下：

```
handler ──> service ──> repository ──> model
              │             │
              └──> infra    └──> pkg（errs / response / dialect / role，叶子包）
```

`pkg` 里放的是**被多个层共用、又不属于任何一层**的纯判断与常量。角色就是这种：
`pkg/role` 定义 `Admin`/`User` 与 `SeesAllData`，service 判数据范围、`ctxs.Principal.IsAdmin`
判能不能进管理端接口，两边用同一份定义。别把它塞回 `model`——那会让 HTTP 层为了比较一个
字符串去 import ORM，换持久化实现时 `ctxs` 也得跟着改。

反向 import 一定是设计错了：要么这段逻辑放错了层，要么该用 Option 注入。

## 3. 硬性约束

- `dto` 不 import `model`；互转写在 service 的 `toXxxItem` 里。
- `service` 不 import HTTP 框架（Fuego / `net/http`）；要当前用户，用 `ctxs.ActorFrom(ctx)`。
  （`service` 会 import `ctxs`，所以 `ctxs` 里不许出现 `*http.Request` 一类的类型，否则
  `net/http` 会顺着这条边进到 service 的依赖图里。）
- **`ctxs` 不 import `model`**：身份只带角色字符串，判断走 `pkg/role`。想图省事写
  `p.Role == model.RoleAdmin`，就等于把整个持久化层拖进传输层。
- **请求级数据只有一家**：`request_id` 与登录身份的 key、类型（`ctxs.Principal`）与读写
  都在 `internal/server/ctxs`。中间件只负责「算出来 + 放进去」，不定义 key、不提供读取函数。
  （`repository/tx.go` 的 `txKey` 是事务句柄，不出本包，不属于这一条。）
- 身份只存一份（`ctxs.WithActor`），读取一律 `ctxs.ActorFrom`。存两份的后果是同一个请求
  有两个身份来源，忘写一个就表现为「身份凭空消失」，只能运行时才发现。
  身份类型不在 `service` 里再定义一份：`ctxs` 也不能反过来 import `service`，会成环。
- **`pkg/response` 不读 context**：它只按给定值拼失败响应，`request_id` 由调用方从
  `ctxs` 取来传进 `response.Fail`。pkg 是叶子包，读了就得反向依赖 internal。
- `repository` 不返回 dto；也不 import `service`。
- `internal/pkg/*` 不 import `internal` 其它包（`pkg` 是叶子）。
- **错误只走 `internal/pkg/errs`**：service 返回 `errs.BadRequest/ErrNotFound/...`，
  handler 直接 `return` 这个错误，由统一序列化器翻成响应；不要自己挑状态码、不要自己写 JSON。
  框架自己抛出的错误（`fuego.HTTPError`、`fuego.BadRequestError` 之类）由
  `internal/server/error_handler.go` 按状态码映射成业务错误——那里是「错误是什么」的唯一入口，
  `internal/server/hooks.go` 只负责「怎么写出去」。别把 `WithErrorHandler` 改回恒等函数：
  框架错误会落到序列化器的「非业务错误」分支，400 被折成 500。
- **不静默吞错**：`_ = err` 只允许出现在「这一步失败不影响本次结果」的地方，且同一处必须
  留下 `Warn` 及以上的日志，带上出错的 key 与原始错误。吞错的代价不是这次请求，而是下一次
  才发作的症状（例如登录成功但计数没清，用户下次立刻被锁），到那时日志里什么都没有就没法查。
  能重试且代价小的（一次缓存删除）先重试，仍失败再记日志。
- **handler 只返回业务数据**：签名是 `(业务类型, error)`，函数体里不出现 `response.*`。
  信封与 `request_id` 由 `internal/server/envelope.go` 里的 Fuego 序列化钩子统一加：
  钩子是唯一同时握有返回值与请求的位置，所以注册处不用再逐条包一层；
  成功文案恒为 `ok`，201 与 200 的区别只由 `OptionDefaultStatusCode` 表达。
- **成功信封改了要同时改文档**：Fuego 按 controller 的返回类型生成 2xx 的 schema，
  钩子里包的信封它看不见，靠 `envelopeSuccessResponses` 事后补（同 `sanitizeErrorResponses`）。
  漏补不报错，只会让文档写成一个裸对象；`internal/server/access_test.go` 的
  `TestOpenAPISuccessEnvelope` 兜这一条。
- `response.Fail` 只给中间件与错误序列化器（`internal/server/hooks.go`）用——它们在
  Fuego 的 Flow 之外，拿不到 handler 的返回值，只能自己写响应。
- **每个对外函数带 `ctx context.Context`** 并一路传到底：超时与取消靠它传导到驱动层。
- **软删除用 `model.DeletedAt`**，唯一索引把 `deleted_at` 排进去（`priority` 见 `model/user.go`）。
- **跨表写入用 `repository.TxManager`**，不要在 service 里手写 `db.Begin()`。装配产物是
  `bootstrap.App.Tx`：接业务时把它当构造参数或选项传给需要多处落库的 service，
  仓储方法会自动加入同一个事务（它们从 ctx 取连接）。示例服务都是单表写入，所以启动路径上
  没有它的调用点——但它是**交出来的**，不是构造完就丢。

## 4. 新增一个模块的清单

按顺序做，每一步都能单独编译：

1. `internal/model/xxx.go`：字段、索引、`TableName()`。
2. `internal/bootstrap/app.go` 的 `models` 切片里登记（漏了会在首次写入时报 no such table）。
3. `internal/repository/xxx.go`：接口 + 实现；通用 CRUD 用嵌入的 `Base[T]`，只写独有查询。
4. `internal/dto/xxx.go`：`XxxItem`（响应）、`XxxCreateRequest`/`XxxUpdateRequest`（入参，带 binding 标签）、
   `XxxListQuery`（嵌入 `PageQuery`）。
5. `internal/service/xxx.go`：接口 + `NewXxxService` + `XxxOption`；业务校验、事务、模型↔契约。
6. `internal/handler/xxx.go`：`readBody`/`bindQuery`/`parseID` → 调 service → **直接返回业务数据**。
   请求体走框架绑定（`readBody` → `c.Body()`），因此**校验器只有一个**：`handler.Validator()`
   在 `server` 装配时用 `fuego.WithValidator` 注入；不要在别处再 `validator.New()`。
   不要在这里包信封，也不要挑状态码。
7. `internal/server/router.go`：把 handler 直接交给 `fuego.Get/Post/...`（信封是全局限定的，
   不用逐条包装；新建另加 `OptionDefaultStatusCode(201)`），再用 `slices.Concat(doc(...), secured)`
   复用「鉴权 + 安全声明 + 失败码」；管理端换成 `adminOnly`，其余失败码用 `errorResponses(409)` 补。
   漏 `secured` 会把需要登录的接口变成公开接口，且不会报错——由 `internal/server/access_test.go`
   兜住，所以别删那个文件。
8. 测试：至少覆盖 service 的校验分支；仓储的特殊查询补集成测试。
9. 第 4 步定下的字段名，就是第 1 节那份对外承诺的一部分：改名按破坏性变更处理。

本目录只有 Go 服务：页面、请求封装、前端构建都不在这里，也不为了迁就某个消费方在服务端加展示逻辑。

## 5. 数据与存储约定

| 主题 | 约定 |
|---|---|
| 主键 | `uint` 自增，`Base` 提供 |
| 软删除 | `soft_delete` 的整数标记（0 = 未删），唯一索引把 `deleted_at` 排进去，实现「未删才唯一」 |
| 表名 | 显式 `TableName()`；系统表 `sys_`、业务表 `biz_` |
| 时间 | 一律 `time.Time`；「业务日期」存本地日历日的 UTC 零点——带时分秒会让「同一天」各自成一条，也容易在存成 UTC 后跨时区退到前一天 |
| 分页 | 统一 `repository.Pagination` + `dto.PageResult`，参数归一化走 `PageQuery.Normalize` |
| 乐观锁 | 更新时把 `updated_at` 放进 fields（键为 `repository.FieldUpdatedAt`），命中 0 行则 409 |
| 事务 | 跨仓储写入用 `repository.TxManager.Transaction`（装配后是 `bootstrap.App.Tx`），仓储按 ctx 自动加入；回滚/提交/嵌套三条语义由 `internal/repository/tx_test.go` 钉住 |
| 多库支持 | 上层永远拿 `*gorm.DB`；驱动差异只在 `infra/database` 与 `pkg/dialect` 里消化 |

「换存储不改代码」是这个工程的前提：`database.driver` 改成 `postgres` / `mysql` / `sqlite`，
`cache.driver` 改成 `redis` / `memory`，业务层零改动。改完至少跑一次 `task test`，再接真实库验证。

## 6. 配置与密钥

优先级：**配置文件 < 环境变量（`APP_` 前缀）**，生产模板 `configs/config.prod.yaml`。

变量名由字段的 yaml 路径推导，**不需要单独登记**：`APP_` + 路径，层级与键名统一用下划线，
`app` 段并进前缀。例：`http.addr` → `APP_HTTP_ADDR`，`database.dsn` → `APP_DATABASE_DSN`，
`cache.memory.save_interval` → `APP_CACHE_MEMORY_SAVE_INTERVAL`。加一个字段就自动多一个变量。

- 列表用逗号分隔，自动去首尾空格与空项：`APP_HTTP_CORS_ORIGINS="https://a,https://b"`。
- 空字符串视为「未设置」，保持配置文件里的值——`VAR=${VAR}` 展开成空串不会把生产密钥清空。
- 取值非法（`APP_HTTP_SHUTDOWN_TIMEOUT=abc`）直接拒绝启动，并点名是哪个变量。
- `database.params` 这类 map 没有环境变量出口，改它们请直接改配置文件。

| 项 | 默认 | 说明 |
|---|---|---|
| `http.addr` | `:8080` | 监听地址；消费方连的就是这个端口 |
| `http.cors_origins` | 本地 `5173` 两个来源 | 跨域白名单；**生产必须非空**，空名单的语义是允许任意来源，`Validate` 会拒绝启动 |
| `app.seed_admin_*` | `admin` / `admin123` | 仅在 `database.seed: true` 时建号，上线前必须改 |
| `database.auto_migrate` | `true` | 启动即同步表结构；生产建议改显式迁移 |
| `database.driver` | `sqlite` | 也可 `postgres` / `mysql`，字段名一致 |
| `cache.driver` | `memory`（落盘 `data/cache.snapshot`） | 也可 `redis`；`fallback_to_memory` 生产应为 false |
| `jwt.secret` | 开发用固定串 | 生产必须用 `APP_JWT_SECRET` 注入 32 位以上随机串 |
| `log.level` / `log.format` | `info` / `text` | 生产建议 json |

- 密码、JWT 密钥、DSN 生产一律走环境变量，不进仓库。
- 启动期就校验（`config.Validate`）：驱动名、必填项、生产环境的密钥长度、seed 开关、
  `cors_origins` 非空，以及几个时长/间隔不能为负（负值会在 `time.NewTicker` 里 panic）。
  失败就退出，而不是等到第一个请求才 500。
- 新增配置项要同时改：结构体字段（带 `yaml` tag）+ 默认值 + 校验 + 本节表格；只加结构体字段等于给了个零值。
  环境变量出口由 `yaml` tag 自动推导，不用再登记一次——但 tag 写错，变量名就会跟着错。

## 7. 注释与命名

- 注释写**为什么**，不写**是什么**。`// 返回列表` 没有信息量；
  `// 计数与取数分别构建查询：复用同一个 *gorm.DB 会让 Count 的 statement 残留到 Find 上` 才是。
- 踩过的坑值得写进注释：它比文档更靠近代码，改代码的人一定会看到。
- 命名用业务语言（`Note`、`OwnerID`），不要用技术缩写（`Nt`、`oid`）。
- 表名 `sys_` / `biz_` 前缀，和现有保持一致。

## 8. 运行与自检

在本目录执行（不必依赖仓库根的入口）：

```bash
task install             # go mod download + tidy
task install -- --ci     # 只 go mod download，不改 go.mod / go.sum
task dev                 # air 热重载，默认 SQLite + 内存缓存，监听 :8080（不需要容器）
task dev -- --db=pg      # PostgreSQL + Redis：先拉起本端容器，再热重载
task dev -- --db=mysql   # MySQL + Redis：同上
task dev -- --once       # 单次启动、不监听（可与 --db 组合）
task dev -- --down       # 停掉上面拉起的容器（数据卷保留）
task fmt                 # gofmt
task lint                # go vet + staticcheck（后者管「写了没人调用」这类死代码）
task test                # 单元测试
task build               # 产物到 bin/app-server，版本号构建期注入
```

首次启动会自动建表并写入 `app.seed_admin_*` 那个账号；`data/` 下是 SQLite 与缓存快照，
要推倒重来就删 `data/app.db*` 与 `data/cache.snapshot`。

驱动相关的语义（MySQL 的 UPDATE 行数、软删除唯一索引、乐观锁时间精度）在 SQLite 上
跑不出来，`cache` 两套实现「语义一致」的对照表也只有真 Redis 能验。这两个用例默认
跳过，起一个一次性容器再指定地址即可（用户/库默认 `app`）：

```bash
docker run -d --name probe-mysql -p 13306:3306 \
  -e MYSQL_ROOT_PASSWORD=root123 -e MYSQL_DATABASE=app \
  -e MYSQL_USER=app -e MYSQL_PASSWORD=app mysql:8.4
APP_TEST_DRIVER=mysql APP_TEST_PORT=13306 go test ./internal/repository -v
docker rm -f probe-mysql         # 改成 postgres:17-alpine 与 15432:5432 即测 Postgres

docker run -d --name probe-redis -p 16379:6379 redis:7-alpine
APP_TEST_REDIS_ADDR=127.0.0.1:16379 go test ./internal/infra/cache -v
docker rm -f probe-redis
```

改动后的连带检查：

- 动了线格式（前缀、响应体、码表、分页字段）→ 更新第 1 节，并在提交信息里点名；
- 动了配置项 / 环境变量名 → 更新第 6 节的表；
- 动了 `Taskfile.yml` 的任务名 → 确认第 0 节列出的名字还在（外部编排按名调用）。

## 9. 不要做的事

- 不要把业务逻辑写进 `middleware`（它只做横切，不认识任何实体）。
- 不要在 handler 里直接调 repository（绕过 service 等于绕过业务校验）。
- 不要在 `internal/pkg` 里加与项目相关的类型（那等于把业务下沉到叶子包）。
- 不要为了「以后可能用到」预留没有调用方的抽象；有第二个调用方时再抽。
- 不要新增第二套错误处理 / 响应格式 / 日志库；先用现有的。
- 不要 import 仓库根或其他子项目的代码，也不要读它们的文件当配置（对外只走 HTTP）。
- 不要让本目录的构建依赖目录之外的某个文件或变量：单独拷出去必须还能 `task install && task build`。
