# 架构与约定

这份文档回答「代码该放哪、依赖指向哪、改一处要连带改什么」。
怎么跑起来见 [`../README.md`](../README.md)，给 AI 协作者看的规则见 [`../AGENTS.md`](../AGENTS.md)。

## 1. 全景

```
.
├── apps/
│   ├── backend/           Go 服务（gin + gorm）
│   └── frontend/          React 管理端（Vite + TS + antd）
├── deployments/           本地基础设施（docker compose）
├── docs/                  架构、术语、接口等长期文档
├── scripts/               发版等本地脚本
├── Taskfile.yml           命令总入口（只编排，不实现）
└── VERSION                版本号正本
```

前后端各自有独立的 `Taskfile.yml`，根只做编排。
CI 与本地跑的是同一批 Task 任务，不存在「CI 里另有一套命令」。

## 2. 后端分层

依赖方向是单向的，箭头表示「可以 import」：

```
cmd/server ──> bootstrap ──> server ──> handler ──> service ──> repository ──> model
                   │                                   │             │
                   └──────────> infra/* <──────────────┘             │
                                                       └──> pkg/* <──┘
```

| 目录 | 职责 | 不该出现的东西 |
|---|---|---|
| `cmd/server` | 解析参数、装配、启停 HTTP 服务 | 任何业务逻辑 |
| `internal/bootstrap` | 依赖装配（db、cache、repo、service、handler、router） | 业务判断 |
| `internal/config` | 读配置、补默认值、环境变量覆盖、启动期校验 | 业务常量 |
| `internal/server` | gin 引擎、路由表、中间件装配 | 具体 handler 实现 |
| `internal/server/middleware` | 横切能力（请求 ID、日志、恢复、CORS、限流、鉴权） | 业务规则 |
| `internal/handler` | 解析参数 → 调 service → 写响应 | 业务判断、SQL |
| `internal/service` | 业务规则编排、事务边界选择、模型与契约互转 | gin、SQL、直接读写 HTTP |
| `internal/repository` | 数据访问，返回 model；接口在这一层声明 | 业务规则、HTTP 概念 |
| `internal/model` | 表结构（字段、索引、表名） | 业务方法之外的逻辑 |
| `internal/dto` | 请求 / 响应 / 查询契约 | 对 `model` 的 import |
| `internal/infra/*` | 具体技术实现：database、cache、logger | 业务概念 |
| `internal/pkg/*` | 与被依赖方向无关的小工具：errs、response、dialect | 对 `internal` 其它包的依赖 |

几条硬性约束：

- **`dto` 不 import `model`**：契约层与持久化层解耦，改表结构不连带改接口。
  需要互转就在 service 里写 `toXxxItem`。
- **`service` 不认识 gin**：要拿当前登录身份，用 `service.ActorFrom(ctx)`，
  由中间件在 `Auth` 里写进 request context。
- **`repository` 不返回 dto**：它只管把行变成 model，字段裁剪是 service 的事。
- **`pkg` 不依赖 `internal` 其它包**：否则任何一次改动都会沿着依赖反向扩散。

## 3. 一次请求的流转

以 `POST /api/v1/notes` 为例：

1. `middleware.RequestID` 生成/透传 `X-Request-ID`，`Recovery` 兜 panic，
   `AccessLog` 记一条结构化日志，`BodyLimit` 给请求体设上限，`RequestTimeout` 给 ctx 加截止时间。
2. `middleware.Auth` 校验 Bearer 令牌 → 调 `AuthService.Verify` → 把 `Principal`
   写进 gin.Context 与 request context。
3. `handler.NoteHandler.Create`：`bindJSON` 绑定并校验 → 调 `service.NoteService.Create`。
4. `service` 里做业务校验（重名预检、创建者归属）、组装 `model`、调仓储。
5. `repository.Base.Create` 写库；唯一约束冲突由 `infra/database` 的回调统一翻成 409。
6. handler 用 `response.Created` 写出 `{code,message,data,request_id}`。

错误只在两处翻译：service 返回 `errs.*Error`，handler 交给 `response.Fail` 折叠成状态码。
不要在 handler 里自己拼状态码，也不要把 `err.Error()` 直接当文案返回。

## 4. 数据与存储约定

| 主题 | 约定 |
|---|---|
| 主键 | `uint` 自增，`Base` 提供 |
| 软删除 | `soft_delete` 的整数标记（0 = 未删），唯一索引把 `deleted_at` 排进去，实现「未删才唯一」 |
| 表名 | 显式 `TableName()`；系统表 `sys_`、业务表 `biz_` |
| 时间 | 一律 `time.Time`；「业务日期」用 `model.DateOnly` 截到日历日 |
| 分页 | 列表查询统一 `repository.Pagination` + `dto.PageResult`，上限 200 |
| 乐观锁 | 更新时把 `updated_at` 放进 fields（键为 `repository.FieldUpdatedAt`），命中 0 行则 409 |
| 事务 | 跨仓储写入用 `repository.TxManager.Transaction`，仓储按 ctx 自动加入 |
| 多库支持 | 上层永远拿 `*gorm.DB`；驱动差异只在 `infra/database` 与 `pkg/dialect` 里消化 |

「换存储不改代码」是这个模板的承诺之一：
`database.driver` 改成 `postgres` / `mysql` / `sqlite`，`cache.driver` 改成 `redis` / `memory`，
业务层零改动。改完请至少跑一次 `task test`，再接真实库验证。

## 5. 配置与密钥

优先级：**配置文件 < 环境变量（`APP_` 前缀）**。

- 配置文件按环境拆：`config.yaml`（默认 SQLite）、`config.postgres.yaml`、`config.mysql.yaml`、`config.prod.yaml`。
- 密码、JWT 密钥、DSN 生产一律走环境变量，不进仓库。
- 启动期就校验（`config.Validate`）：驱动名、必填项、生产环境的密钥长度与 seed 开关。
  失败就退出，而不是等到第一个请求才 500。

## 6. 前端分层

```
src/
├── api/          接口封装：client.ts 统一注入令牌/拆包/抛 ApiError，其余按资源分文件
├── components/   layout（外壳）、page（页面级公共块）、ui（无业务语义的小组件）
├── config/       导航等静态配置
├── constants/    常量（令牌键、分页大小）
├── hooks/        可复用逻辑（useList）
├── pages/        页面，一个目录一页
├── providers/    全局状态（登录态）
├── router/       路由表与守卫
├── types/        与后端 dto 对应的类型
└── utils/        纯函数（格式化、查询串）
```

约定：

- **页面不直接 `fetch`**：一律走 `api/`，令牌注入与错误码处理只写一遍。
- **`types` 与后端 `dto` 一一对应**：字段名保持一致，不要在前端另起一套更顺手的名字。
- **加载态与错误态在页面里处理**：`useList` 只负责取数与重试，怎么展示由页面决定。
- **登录守卫在路由层**（`RequireAuth`）：页面不写「没登录就跳转」的分支。

## 7. 新增一个业务模块的顺序

1. `model/`：定义表结构与索引（含 `deleted_at` 复合唯一）。
2. `bootstrap/app.go` 的 `models` 清单里登记。
3. `repository/`：接口 + 实现，只放该实体独有的查询。
4. `dto/`：请求、响应、查询条件。
5. `service/`：接口 + 构造函数 + Option；业务规则、事务、模型转契约。
6. `handler/`：解析参数、调 service、写响应。
7. `server/router.go`：挂路由（需要登录的挂进 `secured` 组）。
8. 前端：`types/` → `api/` → `pages/` → `router/` → `config/nav.ts`。
9. 测试：优先覆盖 service 的校验与状态流转；仓储的特殊查询再补集成测试。
