# 架构与跨端约定

这份文档回答「仓库怎么切、两端在哪交接、改一处要连带改什么」。
**它不写任何一端的编码规范**——那些在各自的工程目录里，见下面的文档地图。

## 1. 全景

```
.
├── apps/
│   ├── backend/           Go 服务（Fuego + gorm）
│   │   ├── AGENTS.md      后端规范正本
│   │   └── deployments/   本端开发环境用的容器（docker compose）
│   └── frontend/          React 管理端（Vite + TS + antd）
│       └── AGENTS.md      前端规范正本
├── docs/
│   ├── architecture.md    本文件：跨端全景
│   └── contracts/api.md   接口协议与错误码正本（两端共用）
├── scripts/               全局入口（task-entry.sh）与发版、发布流水线的脚本
├── Taskfile.yml           命令总入口：includes 提供 <子项目>:<命令>，8 个全局命令转调脚本
└── VERSION                版本号正本
```

前后端各自有独立的 `Taskfile.yml`；根目录有两套入口，按**任务名**区分：单端是
`task backend:build` 这种命名空间任务（根 `Taskfile.yml` 的 `includes` 直接交给那一端，
根目录不参与、参数零解析），全量是 8 个统一命令（`install` / `build` / `clean` / `dev` /
`lint` / `fmt` / `release` / `test`），由 `scripts/task-entry.sh` 运行时遍历 `apps/*`
把同名任务派发下去，子项目没定义就跳过。7 个业务命令共用同一个循环：不认命令的特殊含义、
不做串联步骤、也不区分串行与并行。本地 `task build` / `task lint` / `task test` / `task fmt`
与 CI 里的编译、vet、单测、gofmt、前端类型检查是同一条链路，不存在「CI 里另有一套命令」。
`apps/` 下每个子项目都是独立工程（可单独拷出、单独构建），依赖方向见下一节。

## 2. 依赖方向：只有「框架 → apps」这一向

`apps/` 下每个子项目都是**独立工程**，不是总框架的模块：

```
总框架（Taskfile.yml、scripts/、.github/workflows/、docs/、VERSION、.gitignore）
   │  允许：框架读取 / 调用 / 引用 apps
   │  禁止：apps 反向读取框架的代码、模块、配置
   ▼
apps/backend        apps/frontend
      └────✗───────┘  子项目之间也禁止互相引用
```

| 方向 | 允许 | 例子 |
|---|---|---|
| 框架 → apps | 是 | 根 `Taskfile.yml` 的 `includes` 把 `<子项目>:<命令>` 交给那一端；全局命令经 `scripts/task-entry.sh` 调子项目的同名任务；`scripts/release.sh` 写 `apps/frontend/package.json` 版本号；文档链接到 `apps/` 内文件 |
| apps → 框架 | 否 | 子项目 import 根目录实现；缺了根 `VERSION` / 根变量就构建失败 |
| apps A → apps B | 否 | 前端 import 后端代码；后端读前端 `package.json`；共用一份「两边都能用」的源码目录 |

跨端唯一的合法通道是 HTTP + [`contracts/api.md`](contracts/api.md)。文档层面同理：
本文件与 `README.md` 可以链接到 `apps/` 内的文件，`apps/` 下的文档不反过来链接这里。

三条子项目的判据：

- 把这个目录单独拷到别处，`install` 与 `build` 还能跑通（降级可以，报错不行）；
- 子项目的源码、配置、Taskfile 里不出现 `../../`；
- 子项目的文档里不出现指向目录之外的链接
  （`grep -rn --exclude-dir=node_modules '](\.\./' apps/` 应当无输出）。

细节与「已知软读取」登记表在 [`../AGENTS.md`](../AGENTS.md) 第 3 节，自检命令也在那里。

## 3. 两端的边界

| 边界 | 由谁定义 | 正本位置 |
|---|---|---|
| 接口形状与错误码 | 后端实现、两端共同遵守 | [`contracts/api.md`](contracts/api.md) |
| 字段命名 | 后端 `dto` 的 json tag，前端照抄 | `apps/backend/internal/dto` ↔ `apps/frontend/src/types` |
| 开发期连通 | Vite 把 `/api` 代理到 `127.0.0.1:8080` | `apps/frontend/vite.config.ts` |
| 部署期连通 | 网关按 `/api` 前缀转发 | 部署侧（仓库不含网关配置） |

跨越这条边界的改动只有一种：**契约变更**。它必须在同一个提交里闭合 ——
先改契约文档，再改后端，同一个提交里改前端。流程见契约正本第 7 节。

## 4. 一次请求的跨端链路

以新建一条笔记（`POST /api/v1/notes`）为例：

1. **前端**：页面调 `api/notes.ts` 的 `createNote` → `api/client.ts` 注入
   `Authorization: Bearer` 与 `Content-Type` → `fetch('/api/v1/notes')`。
2. **代理**：开发期 Vite 把 `/api` 转发到 `127.0.0.1:8080`；部署期由网关转发。
3. **中间件**：`RequestID` 生成/透传 `X-Request-ID`，`Recovery` 兜 panic，`AccessLog`
   记结构化日志，`BodyLimit` 给请求体设上限，`RequestTimeout` 给 ctx 加截止时间，
   `Auth` 校验令牌并把 `Principal` 写进 context。
4. **后端**：`handler` 绑定并校验 → `service` 做业务校验与事务选择 → `repository` 写库
   （唯一约束冲突由 `infra/database` 的回调统一翻成 409）。
5. **回程**：handler 只返回业务数据，由路由注册处的 `ok` / `created` 装饰器包成 `Envelope[T]`，
   再由统一序列化器写出 `{code,message,data,request_id}`。
6. **前端**：`client.ts` 拆包；`code !== 0` 抛 `ApiError`；页面按 `err.message` / `err.code` 处理展示。

错误只在两处翻译：后端 service 返回 `errs.*Error`，handler 把它原样 `return` 出去，
统一错误序列化器再折叠成状态码（非业务错误一律 500，真实原因只进日志）；
前端 `client.ts` 把非 0 的 `code` 折叠成异常。两端都不在业务代码里拼状态码、解析错误码。

同一个动作里还会生成 OpenAPI 文档：路由注册时同步写入 spec，`GET /openapi.json` 与
`GET /docs` 可读（仅非 release 环境暴露）。既是给消费方的接口说明，也让「代码改了文档没改」
这种中间状态不存在。

## 5. 分层

- **后端分层**（`handler → service → repository → model`，依赖单向）：
  落点表、硬性约束、存储与配置约定 = [`../apps/backend/AGENTS.md`](../apps/backend/AGENTS.md)。
- **前端分层**（`pages → components/hooks/api → constants/types/utils`）：
  目录职责、取数与错误处理、组件与命名 = [`../apps/frontend/AGENTS.md`](../apps/frontend/AGENTS.md)。

分层图与目录职责只写在各自的正本里，本文件不复述——复述就会漂移。
某一端被整体替换（换成别的框架 / 别的工程）时，端级正本要跟着换成新工程的形状，
步骤见 [`../README.md`](../README.md)「替换前端 / 替换后端」。

## 6. 新增一个业务模块的跨端顺序

这是**协作顺序**，不是编码规范；每一步的细则看对应端的正本。

1. 后端 `model`：表结构与索引。
2. 后端 `bootstrap/app.go`：`models` 清单里登记。
3. 后端 `repository` → `dto` → `service` → `handler` → `router`。
4. **契约对齐**：`dto` 定下的字段名同步到 `docs/contracts/api.md` 与前端 `types`。
5. 前端 `types` → `api/` → `pages/` → `router/` → `config/nav.ts`。
6. 测试：后端至少覆盖 service 的校验分支；前端靠 `tsc` 兜住类型，交互密集的页面再补测试。

示例模块 `Note` 就是这条链路的样板（`apps/backend/internal/**/note*.go`、
`apps/frontend/src/pages/notes`），替换或删除见 [`../README.md`](../README.md) 的占位替换清单。

## 7. 文档地图

| 想知道 | 看 |
|---|---|
| 怎么跑起来、命令、占位替换 | [`../README.md`](../README.md) |
| 怎么把前端 / 后端整端换成别的工程 | [`../README.md`](../README.md)「替换前端 / 替换后端」 |
| 后端怎么改 | [`../apps/backend/AGENTS.md`](../apps/backend/AGENTS.md) |
| 前端怎么改 | [`../apps/frontend/AGENTS.md`](../apps/frontend/AGENTS.md) |
| 接口形状、错误码、分页、鉴权 | [`contracts/api.md`](contracts/api.md) |
| 改哪端、读哪份、命名规则、自治边界 | [`../AGENTS.md`](../AGENTS.md) |
