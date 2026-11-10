# 架构与跨端约定

这份文档回答「仓库怎么切、两端在哪交接、改一处要连带改什么」。
**它不写任何一端的编码规范**——那些在各自的工程目录里，见下面的文档地图。

## 1. 全景

```
.
├── apps/
│   ├── backend/           Go 服务（gin + gorm）
│   │   └── AGENTS.md      后端规范正本
│   └── frontend/          React 管理端（Vite + TS + antd）
│       └── AGENTS.md      前端规范正本
├── deployments/           本地基础设施（docker compose）
├── docs/
│   ├── architecture.md    本文件：跨端全景
│   └── contracts/api.md   接口协议与错误码正本（两端共用）
├── scripts/               发版等本地脚本
├── Taskfile.yml           命令总入口（只编排，不实现）
└── VERSION                版本号正本
```

前后端各自有独立的 `Taskfile.yml`，根只做编排；CI 与本地跑的是同一批 Task 任务，
不存在「CI 里另有一套命令」。

## 2. 两端的边界

| 边界 | 由谁定义 | 正本位置 |
|---|---|---|
| 接口形状与错误码 | 后端实现、两端共同遵守 | [`contracts/api.md`](contracts/api.md) |
| 字段命名 | 后端 `dto` 的 json tag，前端照抄 | `apps/backend/internal/dto` ↔ `apps/frontend/src/types` |
| 开发期连通 | Vite 把 `/api` 代理到 `127.0.0.1:8080` | `apps/frontend/vite.config.ts` |
| 部署期连通 | 网关按 `/api` 前缀转发 | `deployments/` |

跨越这条边界的改动只有一种：**契约变更**。它必须在同一个提交里闭合 ——
先改契约文档，再改后端，同一个提交里改前端。流程见契约正本第 7 节。

## 3. 一次请求的跨端链路

以新建一条笔记（`POST /api/v1/notes`）为例：

1. **前端**：页面调 `api/notes.ts` 的 `createNote` → `api/client.ts` 注入
   `Authorization: Bearer` 与 `Content-Type` → `fetch('/api/v1/notes')`。
2. **代理**：开发期 Vite 把 `/api` 转发到 `127.0.0.1:8080`；部署期由网关转发。
3. **中间件**：`RequestID` 生成/透传 `X-Request-ID`，`Recovery` 兜 panic，`AccessLog`
   记结构化日志，`BodyLimit` 给请求体设上限，`RequestTimeout` 给 ctx 加截止时间，
   `Auth` 校验令牌并把 `Principal` 写进 context。
4. **后端**：`handler` 绑定并校验 → `service` 做业务校验与事务选择 → `repository` 写库
   （唯一约束冲突由 `infra/database` 的回调统一翻成 409）。
5. **回程**：`response.Created` 写出 `{code,message,data,request_id}`。
6. **前端**：`client.ts` 拆包；`code !== 0` 抛 `ApiError`；页面按 `err.message` / `err.code` 处理展示。

错误只在两处翻译：后端 service 返回 `errs.*Error`，handler 交给 `response.Fail` 折叠成状态码；
前端 `client.ts` 把非 0 的 `code` 折叠成异常。两端都不在业务代码里拼状态码、解析错误码。

## 4. 分层

- **后端分层**（`handler → service → repository → model`，依赖单向）：
  落点表、硬性约束、存储与配置约定 = [`../apps/backend/AGENTS.md`](../apps/backend/AGENTS.md)。
- **前端分层**（`pages → components/hooks/api → constants/types/utils`）：
  目录职责、取数与错误处理、组件与命名 = [`../apps/frontend/AGENTS.md`](../apps/frontend/AGENTS.md)。

分层图与目录职责只写在各自的正本里，本文件不复述——复述就会漂移。

## 5. 新增一个业务模块的跨端顺序

这是**协作顺序**，不是编码规范；每一步的细则看对应端的正本。

1. 后端 `model`：表结构与索引。
2. 后端 `bootstrap/app.go`：`models` 清单里登记。
3. 后端 `repository` → `dto` → `service` → `handler` → `router`。
4. **契约对齐**：`dto` 定下的字段名同步到 `docs/contracts/api.md` 与前端 `types`。
5. 前端 `types` → `api/` → `pages/` → `router/` → `config/nav.ts`。
6. 测试：后端至少覆盖 service 的校验分支；前端靠 `tsc` 兜住类型，交互密集的页面再补测试。

示例模块 `Note` 就是这条链路的样板（`apps/backend/internal/**/note*.go`、
`apps/frontend/src/pages/notes`），替换或删除见 [`../README.md`](../README.md) 的占位替换清单。

## 6. 文档地图

| 想知道 | 看 |
|---|---|
| 怎么跑起来、命令、占位替换 | [`../README.md`](../README.md) |
| 后端怎么改 | [`../apps/backend/AGENTS.md`](../apps/backend/AGENTS.md) |
| 前端怎么改 | [`../apps/frontend/AGENTS.md`](../apps/frontend/AGENTS.md) |
| 接口形状、错误码、分页、鉴权 | [`contracts/api.md`](contracts/api.md) |
| 改哪端、读哪份、命名规则 | [`../AGENTS.md`](../AGENTS.md) |
