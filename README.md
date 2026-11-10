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
task install                  # 装依赖（后端 go mod + 前端 pnpm install，首次会生成 pnpm-lock.yaml）
task -d apps/backend tool:air # 一次性：装后端热重载工具 air（go install，不进 go.mod）
task dev                      # 并行起后端 :8080（air 热重载）与前端 :5173（Vite HMR）
```

根目录只有 8 个统一命令，每个都会自动派发到 `apps/` 下所有子项目的同名任务
（子项目没定义就跳过，新增一端不用改根目录）：`install` / `build` / `clean` / `dev` /
`lint` / `fmt` / `release` / `test`。单个端的任务直接对那一端执行：`task -d apps/backend build`。
需要传参数的放进 `--` 之后：`task install -- --ci`、`task lint -- --no-fmt`。

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
│   │   ├── deployments/          本端开发环境用的容器（PostgreSQL / MySQL / Redis）
│   │   ├── .air.toml             热重载配置
│   │   ├── Dockerfile            多阶段构建
│   │   └── Taskfile.yml          后端任务
│   └── frontend/                 React 管理端
│       ├── AGENTS.md             前端规范正本（只此一份）
│       ├── src/{api,components,config,constants,hooks,pages,providers,router,types,utils}
│       └── Taskfile.yml          前端任务
├── AGENTS.md                    规范索引：改哪端读哪份 + 命名与维护规则
├── docs/
│   ├── architecture.md          跨端全景（不写单端规范）
│   └── contracts/api.md         接口协议与错误码正本（两端共用）
├── .github/workflows/
│   ├── ci.yml                   提交即跑的检查（编译、vet、gofmt、单测、前端类型检查）
│   └── release.yml              发版流水线（tag 触发：编译、打包、发 Release、推镜像）
├── scripts/
│   ├── task-entry.sh            统一入口：把 8 个命令派发到 apps/ 下所有子项目
│   ├── release.sh               本地发版：改版本号 → 提交 → 打 tag → 推送
│   ├── release-version.sh       解析本次发布的版本号（流水线用）
│   ├── release-previous-tag.sh  查找上一个版本标签（流水线用）
│   └── release-notes.sh         生成 Release 正文（流水线用）
├── Taskfile.yml                 命令总入口（只做转调，逻辑在 scripts/task-entry.sh）
└── VERSION                      版本号正本
```

规范按端存放：`apps/backend/AGENTS.md` 与 `apps/frontend/AGENTS.md` 各管一端，且**各自自包含**
——不链接根目录或其他子项目的文档，需要的接口形状就写在自己文件里；根目录的
[`AGENTS.md`](AGENTS.md) 只是索引。引用方向单向：根目录文档可以指向 `apps/`，反之不行。
两端共用的接口协议在 [`docs/contracts/api.md`](docs/contracts/api.md) 定稿（评审基准），
端级文档各留一份必要摘要，改契约时三处一起改。

## 存储：默认零依赖

开发默认 **SQLite + 内存缓存**，不装任何数据库就能跑：
`data/app.db`（WAL）与 `data/cache.snapshot`（登录态与限流计数的落盘快照），两者都在 `.gitignore` 里。

| 想跑的组合 | 执行 |
|---|---|
| SQLite + 内存（默认） | `task dev`，或只起后端 `task -d apps/backend dev` |
| PostgreSQL + Redis | `task -d apps/backend dev:pg`（容器由这条命令自己拉起） |
| MySQL + Redis | `task -d apps/backend dev:mysql`（同上） |
| 停掉上面拉起的容器 | `task -d apps/backend dev:down`（数据卷保留） |

容器属于**后端自己的开发环境**：编排文件在 `apps/backend/deployments/docker-compose.yml`，
由后端的 `dev:pg` / `dev:mysql` 负责启动，根目录不再有统一的 `infra:up` / `infra:down`——
哪个端需要什么容器，由那个端的 dev 命令自己管。

切换只改配置，不改代码。也可以用环境变量整段覆盖：

```bash
APP_DB_DRIVER=postgres APP_DB_HOST=127.0.0.1 APP_DB_USER=app \
APP_DB_PASSWORD=app APP_DB_NAME=app \
APP_CACHE_DRIVER=redis APP_REDIS_ADDR=127.0.0.1:6379 \
task -d apps/backend run
```

优先级：配置文件 < `APP_*` 环境变量。生产模板见 `configs/config.prod.yaml`。

## 命令

根目录只有下面 8 个命令，每个都会遍历 `apps/` 下所有子项目、执行同名任务，
**子项目没定义就跳过**：

| 命令 | 作用 | 参数 |
|---|---|---|
| `task install` | 装所有子项目依赖 | `--ci`：严格按锁文件（前端 `--frozen-lockfile`，锁文件与 `package.json` 不一致直接失败） |
| `task build` | 构建所有子项目产物 | 无 |
| `task clean` | 清理所有子项目构建产物 | 无 |
| `task dev` | 并行启动所有子项目开发环境（Ctrl-C 一并退出） | 无 |
| `task lint` | 先格式化、再检查所有子项目 | `--no-fmt`：只检查不格式化 |
| `task fmt` | 格式化所有子项目代码 | 无 |
| `task test` | 运行所有子项目测试 | 无 |
| `task release` | 发版：先跑子项目的 `release`（若有），再问版本号 → 同步版本号 → 提交打 tag → 问是否推送 | 版本号走 `VERSION=` 等变量 |

忘了有哪些命令就跑 `task help`（不带任何任务名也一样）。

参数放在 `--` 之后 —— Task 自己的参数解析不接受裸 `--ci`：

```bash
task install -- --ci
task lint -- --no-fmt
```

单个端的任务直接对那一端执行，`task -d apps/backend -- --list` 可以看它有哪些：
`task -d apps/backend dev:once`（单次启动）、`task -d apps/frontend build` 等。

新增一个子项目时，只要在 `apps/<名字>/Taskfile.yml` 里定义同名任务，根命令会自动带上它，
不需要改根目录任何文件。

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
| 2 | `task -d apps/backend build APP_VERSION=1.2.3` | 本地试打某个号 |
| 3 | 根目录 `VERSION` | `task -d apps/backend build` 没显式给号时的缺省值 |
| 4（兜底） | 代码里的 `"dev"` | 裸 `go build` / air 热重载不带 ldflags，一眼看出不是发布产物 |

```bash
task release                      # 回车 = 当前版本 +1 patch，也可以敲 1.3.0
task release VERSION=1.3.0        # 指定版本号，不再询问
task release PUSH=false           # 只做本地部分（提交 + tag）
task release DRY_RUN=true         # 只打印将要发生什么
```

### 发布流水线

`task release` 负责本地这一半（先跑子项目自己定义的 `release` 任务，再改版本号、提交、打 tag、
问是否推送）；**tag 推上去之后由
[`.github/workflows/release.yml`](.github/workflows/release.yml) 接管**：

| 触发 | 说明 |
|---|---|
| 推送 `v1.2.3` 或 `1.2.3` 形式的 tag | 走完整流程 |
| Actions 里手动 Run workflow，填 `version` | 同上；留空 = 按仓库最新 tag 重发一次 |

四组 job，逻辑都在 `scripts/` 下，工作流只按顺序调用：

| job | 做什么 | 用到的脚本 |
|---|---|---|
| `build` | 交叉编译 5 个目标（windows x64、linux amd64/arm64、darwin amd64/arm64），版本号、提交、构建时间经 `-X` 注入；**不含 windows/386**，SQLite 驱动没有 386 实现，收了只会每次发版都红一次 | `release-version.sh` |
| `frontend` | 按锁文件装依赖、打包前端静态产物 | — |
| `release` | 汇总产物 + `checksums.txt`，按上一个标签生成变更日志，发 GitHub Release | `release-version.sh`、`release-previous-tag.sh`、`release-notes.sh` |
| `docker` | 构建 `linux/amd64,linux/arm64` 镜像并推送到 GHCR（`ghcr.io/<owner>/<repo>`） | `release-version.sh` |

三个脚本都能本地直接跑（不设 `GITHUB_OUTPUT` 时结果打到标准输出，提示信息走标准错误），
排查流水线时不必推 tag：

```bash
scripts/release-version.sh --tag v1.3.0        # -> version=1.3.0 / tag=v1.3.0 / commit / build_time
scripts/release-previous-tag.sh --minor v1.3.0 # -> previous_tag=v1.2.0（首次发版为空）
scripts/release-notes.sh v1.3.0 v1.2.0         # 第二个参数留空 = 首次发版，写出 release-notes.md
```

## 替换前端 / 替换后端

`apps/` 下的每个子项目都是**独立工程**：它不 import 另一个子项目的代码，也不 import
总框架的代码或配置。所以替换一端 = 换掉一个目录 + 改几处框架侧的接线，**另一端的代码零改动**。

接线只有四处，先记住它们，后面每节都会回到这张表：

| 接线点 | 谁来改 | 为什么 |
|---|---|---|
| `apps/<端>/Taskfile.yml` 的**任务名** | 子项目 | 根目录的统一入口按名字派发（见各节清单），名字不对就被跳过 |
| `scripts/release.sh` 的 `PKG_FILE` | 总框架 | 发版时同步前端 `package.json` 的 `version` |
| 端口与接口契约 | 两端各自 | 前端 `vite.config.ts` 的代理目标 ↔ 后端监听端口；契约见 [`docs/contracts/api.md`](docs/contracts/api.md) |

根目录不再有 `includes`：`scripts/task-entry.sh` 运行时遍历 `apps/*/Taskfile.y*ml`，
所以换掉整个目录、或改名成 `apps/<新名字>`，只要里面还叫 `Taskfile.yml` 就自动接上。

### 替换前端

**步骤**

1. 移走旧工程：`git rm -r apps/frontend`（想留后路就先 `git mv apps/frontend apps/frontend.old`）。
2. 把新前端工程**拷成 `apps/frontend`**（`cp -r` / `git clone` 到该路径）。换名成别的目录名也可以，
   但要同步改 `scripts/release.sh` 的 `PKG_FILE`、以及两份 `AGENTS.md` 与本文的路径引用。
3. 补 `apps/frontend/Taskfile.yml`，任务名对上统一入口（定义哪个就派发哪个，没定义就跳过）：

   | 任务 | 根里由谁调用 |
   |---|---|
   | `install`（可选 `install:ci`） | `task install` / `task install -- --ci` |
   | `dev` | `task dev` |
   | `build` | `task build` |
   | `lint` | `task lint` |
   | `clean` | `task clean` |
   | `fmt`（可选） | `task fmt` 与 `task lint` 的格式化步骤 |
   | `test`（可选） | `task test` |

4. 调整下列配置（**只在新前端自己的目录里改**）：

   | 配置项 | 位置 | 改什么 |
   |---|---|---|
   | 包名、包管理器、版本字段 | `package.json` 的 `name` / `packageManager` / `version` | 包名换成自己的；`version` 字段**必须保留**，`scripts/release.sh` 靠它同步发版号 |
   | 脚本名 | `package.json` 的 `scripts` | 至少保证 `dev` / `build` / `typecheck` 三个脚本存在（Taskfile 转调它们） |
   | 开发端口 | `vite.config.ts` 的 `server.port` | 默认 5173，本文与 CI 都引用它；改了就同步本文 |
   | 接口代理 | `vite.config.ts` 的 `server.proxy['/api'].target` | 指向后端实际端口（默认 `http://127.0.0.1:8080`） |
   | 接口前缀、令牌键、分页大小 | `src/constants` | 与 [`docs/contracts/api.md`](docs/contracts/api.md) 对齐；令牌键要带应用前缀，避免同域多应用互踩 |
   | 路径别名 | `tsconfig.json` 的 `paths` + `vite.config.ts` 的 `resolve.alias` | 两处必须同时改：`tsc` 只读 `tsconfig`，`vite` 只读自己的 alias |

5. 生效：`task install`（首次会生成锁文件，**把锁文件提交上去**）→ `task dev` 打开
   <http://localhost:5173> → `task lint` 与 `task build` 通过。

**后端不需要改**，除非新前端换了端口（那就改后端 `configs/config.yaml` 的监听端口，或改
Vite 的 proxy target，二选一，不要两边都改）。

### 替换后端

**步骤**

1. 移走旧服务：`git rm -r apps/backend`（或 `git mv` 到 `apps/backend.old`）。
2. 把新服务拷成 `apps/backend`。换目录名要同步本文与 `docs/architecture.md` 里的路径引用，
   以及 `.gitignore` 里按目录名匹配的 `data/`、`configs/config.local.yaml`（确认仍覆盖新路径）。
   新工程要额外容器时，把自己的 compose 文件放在自己的目录里，由自己的 `dev` 负责拉起
   （现有后端就是这样做的：`apps/backend/deployments/docker-compose.yml`）。
3. 补 `apps/backend/Taskfile.yml`，任务名对上统一入口（定义哪个就派发哪个，没定义就跳过）：

   | 任务 | 根里由谁调用 |
   |---|---|
   | `install`（可选 `install:ci`） | `task install` / `task install -- --ci` |
   | `dev` | `task dev` |
   | `build` | `task build` |
   | `test` | `task test` |
   | `lint` | `task lint` |
   | `fmt` | `task fmt` 与 `task lint` 的格式化步骤 |
   | `clean` | `task clean` |

4. 调整下列配置（只在新服务自己的目录里改）：

   | 配置项 | 位置 | 改什么 |
   |---|---|---|
   | 监听端口 | `configs/config*.yaml` | 与前端 `vite.config.ts` 的 proxy target 对上，默认 8080 |
   | 环境变量前缀 | `internal/config`（或新框架的对应位置） | 换了前缀要同步本文「存储」一节的环境变量示例与 `apps/backend/AGENTS.md` 第 5 节 |
   | 配置文件位置与文件名 | `configs/config.yaml` 等 | 本文、`.gitignore` 的 `configs/config.local.yaml`、Taskfile 的 `CONFIG_FILE` 都引用这个路径，换名要一起改 |
   | 数据目录 | `data/` | 默认 SQLite 与缓存快照落在这里，`.gitignore` 已忽略同名的 `data/` |
   | 二进制名 | Taskfile 的 `APP_NAME`、`Dockerfile`、`.github/workflows/release.yml` 的 `APP_NAME` | 本文 `-version` 示例与 `docker build` 引用它 |
   | 热重载配置 | `.air.toml` 与 Taskfile 的 `AIR_CONFIG` | `task dev` 派发的 `dev` 任务走 `air -c .air.toml`；新工程不用 air 就改 `dev` 任务，别让根 `task dev` 直接挂掉 |
   | 接口契约 | 响应体、错误码、分页、鉴权头 | 必须满足 [`docs/contracts/api.md`](docs/contracts/api.md)。新服务自带另一套形状就是契约变更：先改契约，再改前端 |
   | 版本号来源 | Taskfile 的 `APP_VERSION` | 框架约定根目录 `VERSION` 是正本，构建期用 `-X` 注入；独立使用（读不到 `VERSION`）时必须能退化成 `dev`，不能报错 |

5. 生效：`task install` → `task -d apps/backend dev:once`（单次启动，确认能连库、能起来）→
   `task test`、`task lint`、`task build` 全绿。

**前端不需要改**，除非新后端换了端口或改了契约形状。

### 替换之后

- **改那一端的规范**：`apps/backend/AGENTS.md` / `apps/frontend/AGENTS.md` 是那一端的规范正本，
  旧的是 gin + gorm 分层、React + antd 约定；新工程换了框架，就把这份文件改成新工程的形状。
  留着过时的正本比没有更糟——AI 会照着它改。
  改的时候保持**自包含**：不链接根目录或另一端的文档，需要的接口形状、任务名直接写在里面。
- **保持契约单向**：新的一端只通过 `docs/contracts/api.md` 与另一端见面，不要为了省事让它
  直接 import / 读取另一端的代码或配置。
- **跑一遍仓库级自检**：`task fmt && task lint && task test && task build`。

## 占位内容替换清单

模板里的名字都是占位，按下面这张表改一遍即可：

| 占位 | 出现位置 | 替换为 |
|---|---|---|
| `example.com/app` | `apps/backend/go.mod` 与所有 import | 你的模块路径（如 `github.com/you/project`） |
| `app` / `App` | `configs/*.yaml` 的 `app.name`、缓存前缀、`index.html` 标题、`AppLayout` 里的品牌名 | 项目名 |
| `app-server` | `apps/backend/Taskfile.yml`、`Dockerfile`、`main.go` 的 `-version` 输出、`.github/workflows/release.yml` 的 `APP_NAME` | 二进制名 |
| `APP_` | `internal/config/config.go` 的所有环境变量 | 你的环境变量前缀 |
| `admin` / `admin123` | `configs/config.yaml` 的 `app.seed_admin_*` | 初始管理员账号 |
| `app_token` | `apps/frontend/src/constants/index.ts` | 前端令牌键（换项目名时一起换） |
| `app-frontend` | `apps/frontend/package.json` 的 `name` | 前端包名 |
| `app-postgres` / `app-mysql` / `app-redis` | `apps/backend/deployments/docker-compose.yml` | 容器名与库名 |
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
