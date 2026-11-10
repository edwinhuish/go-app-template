# 项目名

Go 后端 + React 管理端的 monorepo 模板：分层清晰、零依赖即可启动、
存储与缓存可配置切换，命令统一由 Taskfile 编排。

**这份 README 讲怎么跑起来**；编码规范按端分家，改哪端读哪份：

| 你改的是 | 先读 |
|---|---|
| 后端 | [`apps/backend/AGENTS.md`](apps/backend/AGENTS.md) |
| 前端 | [`apps/frontend/AGENTS.md`](apps/frontend/AGENTS.md) |
| 接口字段 / 错误码（跨端） | [`docs/contracts/api.md`](docs/contracts/api.md) |
| 想看全局 | [`docs/architecture.md`](docs/architecture.md) 与 [`AGENTS.md`](AGENTS.md) |

> 仓库里带了一个示例模块 `Note`（`apps/backend/internal/**/note*.go` 与前端 `pages/notes`），
> 它的唯一作用是让每条链路都有可运行的样板。接真实业务时按
> [「占位内容」](#占位内容替换清单) 一节替换或删除。

## 前置

| 依赖 | 版本 | 说明 |
|---|---|---|
| Go | ≥ 1.26 | 见 `apps/backend/go.mod` |
| Node | ≥ 20 | 前端 |
| pnpm | ≥ 11 | 版本锁在 `apps/frontend/package.json` 的 `packageManager` |
| `task` | 任意近期版本 | 安装方式见文末 |

```bash
corepack enable pnpm        # 之后 pnpm 按 packageManager 自动取对应版本
```

## 快速开始

```bash
task install            # 装依赖（后端 go mod + 前端 pnpm install，首次会生成 pnpm-lock.yaml）
task backend:tool:air   # 一次性：装后端热重载工具 air（go install，不进 go.mod）
task dev                # 并行起后端 :8080（air 热重载）与前端 :5173（Vite HMR）
```

打开 <http://localhost:5173>，开发环境默认账号 **admin / admin123**
（来自 `apps/backend/configs/config.yaml` 的 `app.seed_admin_*`，上线前必须改）。

首次启动会自动建表并写入这个管理员账号。两个开关在配置里：

- `database.auto_migrate`：启动时同步表结构（骨架期用，生产建议改显式迁移脚本）
- `database.seed`：写入初始账号（**生产必须为 false**，`config.Validate` 会直接拒绝启动）

想推倒重来：

```bash
rm -f apps/backend/data/app.db*    # WAL 模式有三个文件
```

然后把锁文件提交上去，CI 才能用 `--frozen-lockfile` 严格安装：

```bash
git add apps/frontend/pnpm-lock.yaml && git commit -m "chore: add pnpm lockfile"
```

## 目录结构

```
.
├── apps/
│   ├── backend/                  Go 服务
│   │   ├── AGENTS.md             后端规范正本（只此一份）
│   │   ├── cmd/server/           入口：解析参数 → 装配 → 启停
│   │   ├── configs/              分环境配置（默认 SQLite / postgres / mysql / prod）
│   │   ├── internal/
│   │   │   ├── bootstrap/        依赖装配、种子数据
│   │   │   ├── config/           配置加载、默认值、校验
│   │   │   ├── dto/              请求/响应契约（不依赖 model）
│   │   │   ├── handler/          HTTP 适配层
│   │   │   ├── infra/            database / cache / logger
│   │   │   ├── model/            表结构
│   │   │   ├── pkg/              errs / response / dialect（叶子依赖）
│   │   │   ├── repository/       数据访问（接口 + gorm 实现）
│   │   │   ├── server/           gin 引擎、路由表、middleware
│   │   │   └── service/          业务规则、事务、模型↔契约互转
│   │   ├── .air.toml             热重载配置
│   │   ├── Dockerfile            多阶段构建
│   │   └── Taskfile.yml          后端任务
│   └── frontend/                 React 管理端
│       ├── AGENTS.md             前端规范正本（只此一份）
│       ├── src/{api,components,config,constants,hooks,pages,providers,router,types,utils}
│       └── Taskfile.yml          前端任务
├── deployments/docker-compose.yml  本地 PostgreSQL / MySQL / Redis
├── AGENTS.md                       规范索引：改哪端读哪份 + 命名与维护规则
├── docs/
│   ├── architecture.md             跨端全景（不写单端规范）
│   └── contracts/api.md            接口协议与错误码正本（两端共用）
├── scripts/release.sh              发版流程
├── Taskfile.yml                    命令总入口
└── VERSION                         版本号正本
```

规范按端存放：`apps/backend/AGENTS.md` 与 `apps/frontend/AGENTS.md` 各管一端，
根目录的 [`AGENTS.md`](AGENTS.md) 只是索引；两端共用的接口协议只有
[`docs/contracts/api.md`](docs/contracts/api.md) 一份，端级规范只引用、不复制。

## 存储：默认零依赖

开发默认 **SQLite + 内存缓存**，不装任何数据库就能跑：
`data/app.db`（WAL）与 `data/cache.snapshot`（登录态与限流计数的落盘快照），两者都在 `.gitignore` 里。

| 想跑的组合 | 执行 |
|---|---|
| SQLite + 内存（默认） | `task backend:dev` |
| PostgreSQL + Redis | `task infra:up`，再 `task backend:dev:pg` |
| MySQL + Redis | `task infra:up` + `docker compose --profile mysql up -d`，再 `task backend:dev:mysql` |

切换只改配置，不改代码。也可以用环境变量整段覆盖：

```bash
APP_DB_DRIVER=postgres APP_DB_HOST=127.0.0.1 APP_DB_USER=app \
APP_DB_PASSWORD=app APP_DB_NAME=app \
APP_CACHE_DRIVER=redis APP_REDIS_ADDR=127.0.0.1:6379 \
task backend:run
```

优先级：配置文件 < `APP_*` 环境变量。生产模板见 `configs/config.prod.yaml`。

## 命令

| 命令 | 作用 |
|---|---|
| `task` | 列出全部任务 |
| `task install` | 安装前后端依赖 |
| `task install:ci` | 按锁文件严格安装（前端 `--frozen-lockfile`） |
| `task dev` | 并行启动前后端（air + Vite HMR） |
| `task backend:dev:once` | 后端单次启动、不监听变更 |
| `task build` | 后端二进制 + 前端静态产物 |
| `task test` | 后端单元测试 |
| `task lint` | `go vet` + 前端类型检查 |
| `task fmt` | gofmt |
| `task release` | 发版：问版本号 → 同步版本号 → 提交打 tag → 问是否推送 |
| `task clean` | 清理 `bin/` 与 `dist/` |
| `task infra:up` / `infra:down` | 启停 PostgreSQL + Redis（MySQL 加 `--profile mysql`） |

子目录也有各自任务：`task -d apps/backend -- --list`、`task -d apps/frontend -- --list`。

## 发版

版本号正本是仓库根目录的 `VERSION`（一行数字），`apps/frontend/package.json` 的 `version`
跟着一起改。产物不读这个文件，而是**构建期注进去** —— 二进制要能在任何目录自证：

```
app-server 0.1.0
  commit: 7b2b231
  built:  2026-11-06T10:18:49Z
  go:     go1.27.3
```

| 优先级 | 来源 | 用在哪 |
|---|---|---|
| 1（最高） | `-X main.version=…` 链接参数 | 发布流水线把 tag 名注进去；`docker build --build-arg VERSION=` 也走这条 |
| 2 | `task backend:build APP_VERSION=1.2.3` | 本地试打某个号 |
| 3 | 根目录 `VERSION` | `task backend:build` 没显式给号时的缺省值 |
| 4（兜底） | 代码里的 `"dev"` | 裸 `go build` / air 热重载不带 ldflags，一眼看出不是发布产物 |

```bash
task release                      # 回车 = 当前版本 +1 patch，也可以敲 1.3.0
task release VERSION=1.3.0        # 指定版本号，不再询问
task release PUSH=false           # 只做本地部分（提交 + tag）
task release DRY_RUN=true         # 只打印将要发生什么
```

## 占位内容替换清单

模板里的名字都是占位，按下面这张表改一遍即可：

| 占位 | 出现位置 | 替换为 |
|---|---|---|
| `example.com/app` | `apps/backend/go.mod` 与所有 import | 你的模块路径（如 `github.com/you/project`） |
| `app` / `App` | `configs/*.yaml` 的 `app.name`、缓存前缀、`index.html` 标题、`AppLayout` 里的品牌名 | 项目名 |
| `app-server` | `apps/backend/Taskfile.yml`、`Dockerfile`、`main.go` 的 `-version` 输出 | 二进制名 |
| `APP_` | `internal/config/config.go` 的所有环境变量 | 你的环境变量前缀 |
| `admin` / `admin123` | `configs/config.yaml` 的 `app.seed_admin_*` | 初始管理员账号 |
| `app_token` | `apps/frontend/src/constants/index.ts` | 前端令牌键（换项目名时一起换） |
| `app-frontend` | `apps/frontend/package.json` 的 `name` | 前端包名 |
| `app-postgres` / `app-mysql` / `app-redis` | `deployments/docker-compose.yml` | 容器名与库名 |
| `0.1.0` | `VERSION`、`apps/frontend/package.json` | 起始版本 |
| `https://app.example.com` | `configs/config.prod.yaml` 的 CORS | 你的域名 |
| `biz_note` / `Note` / `/notes` | 示例模块（见下） | 真实业务实体 |

示例模块要整体删除时，按这条链路清一遍（删完 `go build ./...` 会告诉你漏了哪里）：

```
apps/backend/internal/model/note.go
apps/backend/internal/dto/note.go
apps/backend/internal/repository/note.go
apps/backend/internal/service/note.go
apps/backend/internal/handler/note.go
apps/backend/internal/bootstrap/app.go        # models 清单 + 装配 + Handlers
apps/backend/internal/server/router.go        # 路由组
apps/frontend/src/types/index.ts              # NoteItem
apps/frontend/src/api/notes.ts                # 整个文件
apps/frontend/src/pages/notes/                # 整个目录
apps/frontend/src/router/index.tsx            # 路由
apps/frontend/src/config/nav.ts               # 菜单项
```

## 安装 task

```bash
go install github.com/go-task/task/v3/cmd/task@latest   # 装到 $(go env GOPATH)/bin
brew install go-task/tap/go-task                        # macOS
sudo snap install task --classic                        # Ubuntu
```
