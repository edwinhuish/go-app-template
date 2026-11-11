# 项目名

Go 后端 + React 管理端的 monorepo 模板：分层清晰、零依赖即可启动、
存储与缓存可配置切换，命令统一由 Taskfile 编排。

**这份 README 讲怎么跑起来**；编码规范按端分家，改哪端读哪份：

| 你改的是 | 先读 |
|---|---|
| 后端 | [`apps/server/AGENTS.md`](apps/server/AGENTS.md) |
| 前端 | [`apps/web/AGENTS.md`](apps/web/AGENTS.md) |
| 接口字段 / 错误码（跨端） | [`docs/contracts/api.md`](docs/contracts/api.md) |
| 想看全局 | [`docs/architecture.md`](docs/architecture.md) 与 [`AGENTS.md`](AGENTS.md) |

> 仓库里带了一个示例模块 `Note`（`apps/server/internal/**/note*.go` 与前端 `pages/notes`），
> 它的唯一作用是让每条链路都有可运行的样板。接真实业务时按
> [「占位内容」](#占位内容替换清单) 一节替换或删除。

## 开始用这个模板

这是一个 GitHub template repository，三种起步方式选一种：

| 方式 | 做法 | 结果 |
|---|---|---|
| 网页 | 仓库页点 **Use this template → Create a new repository** | 新仓库，历史从一次提交开始 |
| `gh` | `gh repo create <你的项目> --template edwinhuish/go-app-template --public --clone` | 同上，本地已 clone 好 |
| 直接 clone | `git clone https://github.com/edwinhuish/go-app-template <你的项目>` | 保留模板的完整提交历史 |

拿到之后第一件事是把占位名换掉，清单见 [「占位内容」](#占位内容替换清单)。

## 前置

| 依赖 | 版本 | 说明 |
|---|---|---|
| Go | ≥ 1.26 | 见 `apps/server/go.mod` |
| Node | ≥ 20 | 前端 |
| pnpm | ≥ 11 | 版本锁在 `apps/web/package.json` 的 `packageManager` |
| `task` | 任意近期版本 | 安装方式见文末 |

```bash
corepack enable pnpm        # 之后 pnpm 按 packageManager 自动取对应版本
```

## 快速开始

```bash
task install                  # 装依赖（后端 go mod + 前端 pnpm install）
task server:tool:air         # 一次性：装后端热重载工具 air（go install，不进 go.mod）
task server:dev              # 后端 :8080（air 热重载）—— 另开一个终端再跑 task web:dev
```

根目录有两套入口，**靠任务名区分，不靠位置参数猜**：

- **所有端**：`task install` / `build` / `clean` / `dev` / `lint` / `fmt` / `release` / `test`，
  每个都派发到 `apps/` 下所有子项目的同名任务（子项目没定义就跳过）。
- **单个端**：`task server:build`、`task web:dev` —— 任务名里带子项目名，由根 Taskfile
  的 `includes` 直接把任务和参数交给那一端的 `Taskfile.yml`，根目录不参与，参数零解析。

需要传参数的放进 `--` 之后：`task install -- --ci`、`task server:dev -- --db=pg --once`。
参数原样交给子项目，根不校验内容（详见「命令与参数」）。

打开 <http://localhost:5173>，开发环境默认账号 **admin / admin123**
（来自 `apps/server/configs/config.yaml` 的 `app.seed_admin_*`，上线前必须改）。

首次启动会自动建表并写入这个管理员账号。两个开关在配置里：

- `database.auto_migrate`：启动时同步表结构（骨架期用，生产建议改显式迁移脚本）
- `database.seed`：写入初始账号（**生产必须为 false**，`config.Validate` 会直接拒绝启动）

想推倒重来：

```bash
rm -f apps/server/data/app.db*          # WAL 模式有三个文件
rm -f apps/server/data/cache.snapshot   # 登录态与限流计数的落盘快照，留着会带回来
```

## 目录结构

```
.
├── apps/
│   ├── server/                   Go 服务
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
│   │   │   ├── server/           Fuego 引擎、路由表、middleware、OpenAPI 生成
│   │   │   └── service/          业务规则、事务、模型↔契约互转
│   │   ├── deployments/          本端开发环境用的容器（PostgreSQL / MySQL / Redis）
│   │   ├── .air.toml             热重载配置
│   │   ├── Dockerfile            多阶段构建
│   │   └── Taskfile.yml          后端任务
│   └── web/                      React 管理端
│       ├── AGENTS.md             前端规范正本（只此一份）
│       ├── src/{api,components,config,constants,hooks,pages,providers,router,types,utils}
│       └── Taskfile.yml          前端任务
├── AGENTS.md                    规范索引：改哪端读哪份 + 命名与维护规则
├── docs/
│   ├── architecture.md          跨端全景（不写单端规范）
│   └── contracts/api.md         接口协议与错误码正本（两端共用）
├── .github/workflows/
│   ├── ci.yml                   提交即跑的检查（编译、vet + staticcheck、gofmt、-race 单测、前端类型检查、文档自检）
│   └── release.yml              发版流水线（tag 触发：编译、打包、发 Release、推镜像）
├── scripts/
│   ├── task-entry.sh            全局入口：把 8 个命令派发到 apps/ 下所有子项目
│   ├── release.sh               本地发版：改版本号 → 提交 → 打 tag → 推送
│   ├── release-version.sh       解析本次发布的版本号（流水线用）
│   ├── release-previous-tag.sh  查找上一个版本标签（流水线用）
│   └── release-notes.sh         生成 Release 正文（流水线用）
├── Taskfile.yml                 命令总入口：includes 给出 <子项目>:<命令>，8 个全局命令转调脚本
└── VERSION                      版本号正本
```

规范按端存放：`apps/server/AGENTS.md` 与 `apps/web/AGENTS.md` 各管一端，且**各自自包含**
——不链接根目录或其他子项目的文档，需要的接口形状就写在自己文件里；根目录的
[`AGENTS.md`](AGENTS.md) 只是索引。引用方向单向：根目录文档可以指向 `apps/`，反之不行。
两端共用的接口协议在 [`docs/contracts/api.md`](docs/contracts/api.md) 定稿（评审基准），
端级文档各留一份必要摘要，改契约时三处一起改。

## 存储：默认零依赖

开发默认 **SQLite + 内存缓存**，不装任何数据库就能跑：
`data/app.db`（WAL）与 `data/cache.snapshot`（登录态与限流计数的落盘快照），两者都在 `.gitignore` 里。

| 想跑的组合 | 执行 |
|---|---|
| SQLite + 内存（默认） | `task dev`，或只起后端 `task server:dev` |
| PostgreSQL + Redis | `task server:dev -- --db=pg`（容器由这条命令自己拉起） |
| MySQL + Redis | `task server:dev -- --db=mysql`（同上） |
| 跑一次就退、不热重载 | `task server:dev -- --once` |
| 停掉上面拉起的容器 | `task server:dev -- --down`（数据卷保留） |

容器属于**后端自己的开发环境**：编排文件在 `apps/server/deployments/docker-compose.yml`，
由后端 `dev` 的 `--db=pg` / `--db=mysql` 负责启动，根目录不再有统一的 `infra:up` / `infra:down`
——哪个端需要什么容器，由那个端的 dev 命令自己管。

切换只改配置，不改代码。也可以用环境变量整段覆盖：

```bash
APP_DB_DRIVER=postgres APP_DB_HOST=127.0.0.1 APP_DB_USER=app \
APP_DB_PASSWORD=app APP_DB_NAME=app \
APP_CACHE_DRIVER=redis APP_REDIS_ADDR=127.0.0.1:6379 \
task -d apps/server run
```

优先级：配置文件 < `APP_*` 环境变量。生产模板见 `configs/config.prod.yaml`。

## 命令

两套入口，按**任务名**区分。

**单个端** —— 任务名 `<子项目>:<命令>`，由根 Taskfile 的 `includes` 直接交给那一端：

| 命令 | 作用 |
|---|---|
| `task server:build` | 只构建后端 |
| `task server:test -- -run TestLogin` | 只跑后端测试；位置参数也能传 |
| `task server:dev -- --db=pg --once` | 只起后端，开关直达后端 |
| `task web:lint` | 只检查前端 |
| `task server:tool:air` | 后端自己的任务（`tool:air`）也能这样点 |
| `task server:help` | 列出后端自己的任务（等价于 `cd apps/server && task`） |

`--` 之后的参数直接进那一端的任务：根目录不解析、不校验、不改写。

**所有端** —— 8 个全局命令，派发给 `apps/` 下所有子项目的同名任务（**子项目没定义就跳过**）：

| 命令 | 作用 | 参数 |
|---|---|---|
| `task install` | 装所有子项目依赖 | `--ci`：严格按锁文件（前端 `--frozen-lockfile`，锁文件与 `package.json` 不一致直接失败） |
| `task build` | 构建所有子项目产物 | 原样转给各子项目 |
| `task clean` | 清理所有子项目构建产物 | 原样转给各子项目 |
| `task dev` | 逐个启动子项目开发环境（第一个长驻端会占住终端，后面的端不会被执行；多端同时开发用 `task server:dev` + `task web:dev`） | 原样转给各子项目（`--db=pg` 这类只对后端有意义，前端自己忽略） |
| `task lint` | 检查所有子项目（格式化是 `task fmt`，不顺手做） | 原样转给各子项目 |
| `task fmt` | 格式化所有子项目代码 | 原样转给各子项目 |
| `task test` | 运行所有子项目测试 | 原样转给各子项目 |
| `task release` | 发版：先跑子项目的 `release`（若有），再问版本号 → 同步版本号 → 提交打 tag → 问是否推送 | `--version=` / `--remote=` / `--push=` / `--dry-run`，其余原样转给各子项目的 `release` |

忘了有哪些命令就跑 `task help`（不带任何任务名也一样）。

### 参数：`--` 之后原样往下传

规则只有两条：

1. **子项目名不进参数，它写在任务名里**（`task server:build`）。于是裸词（`TestLogin`、
   `1.2.3`、`dist`）只有一个身份：业务参数或根自己的取值，不存在「这到底是 app 还是值」。
   代价是根 `Taskfile.yml` 的 `includes` 必须静态列出每个子项目（见下一节）。
2. **`--` 之后的参数一个不改地转给子项目**：根不校验、不过滤、不重排、不猜含义，也不因为
   「没见过这个开关」报错。它们可能就是给旁边那个子项目的——认不认识、要不要用，由各端自己
   决定（不认识的一律忽略）。

```bash
task install -- --ci                      # 所有端，严格按锁文件
task install -- --ci --db=pg              # 所有端；前端不认识 --db=pg，忽略
task server:install -- --ci              # 只装后端，严格模式
task server:dev -- --db=pg --once        # 只起后端，换 PostgreSQL 跑一次
task web:lint                        # 只检查前端
task test -- -run TestLogin               # 所有端的 test；只有后端有 test，参数照样到
task server:test -- -run TestLogin       # 只跑后端测试
task release -- 1.2.3 --dry-run          # 发版：第一个裸词是版本号
```

整条链路上没有任何一层改写参数：

- 单端入口（`task server:build`）由 Task 的 `includes` 把 `CLI_ARGS` 直接交给那一端，
  根目录没有代码参与，参数连一次解析都没有；
- 全局入口由根 Taskfile 把 `CLI_ARGS` 放进 `TASK_ARGS` 环境变量，`scripts/task-entry.sh`
  用 `eval "set --"` 复原边界（含空格、含 `=`、以 `-` 开头、空串都不会被拆开或加引号），
  再用 `task -d apps/<名字> <任务> -- <参数...>` 交给每个子项目。

7 个业务命令在实现上没有区别：**同一个循环、同一份参数**，没有命令专属的解析、没有串联步骤、
也不区分串行与并行。根自己唯一还解析的是 `release` 的取值（第一个裸词是版本号，另有
`--remote=` / `--push=` / `--dry-run`，见「发版」）。

`release` 是框架级的动作，没有 `server:release` 这种入口；要跟着发某一端的东西，
就在那一端的 `Taskfile.yml` 里定义 `release` 任务，全局 `task release` 会先跑它。

### 新增一个子项目

1. 建 `apps/<名字>/Taskfile.yml`，定义要参与统一的那些任务（`install` / `build` / `dev` /
   `lint` / `clean` …），名字与现有两端保持一致。
2. 在根 `Taskfile.yml` 的 `includes` 里加 4 行（Task 不支持通配符包含，必须静态登记）：

   ```yaml
   includes:
     <名字>:
       taskfile: ./apps/<名字>/Taskfile.yml
       dir: ./apps/<名字>
   ```

3. 完成。全局命令（`task build`）第 1 步之后就会自动带上它；只有 `task <名字>:<命令>`
   需要第 2 步，漏了那条入口就不存在（全局命令照常跑）。

## 发版

版本号正本是仓库根目录的 `VERSION`（一行数字），`apps/web/package.json` 的 `version`
跟着一起改。产物不读这个文件，而是**构建期注进去** —— 二进制要能在任何目录自证：

```
app-server 0.1.0
  commit: 7b2b231
  built:  2026-11-06T10:18:49Z
  go:     go1.27.3
```

上面是**示例输出**：`go:` 那一行是构建时的工具链版本，不是这个仓库的最低要求（最低要求见前面的前置表）。

| 优先级 | 来源 | 用在哪 |
|---|---|---|
| 1（最高） | `-X main.version=…` 链接参数 | 发布流水线把 tag 名注进去；`docker build --build-arg VERSION=` 也走这条 |
| 2 | `task -d apps/server build APP_VERSION=1.2.3` | 本地试打某个号 |
| 3 | 根目录 `VERSION` | `task -d apps/server build` 没显式给号时的缺省值 |
| 4（兜底） | 代码里的 `"dev"` | 裸 `go build` / air 热重载不带 ldflags，一眼看出不是发布产物 |

```bash
task release                                  # 回车 = 当前版本 +1 patch，也可以敲 1.3.0
task release -- 1.3.0                         # 指定版本号，不再询问（第一个裸词就是版本号）
task release -- 1.3.0 --push=false            # 只做本地部分（提交 + tag）
task release -- --dry-run                     # 只打印将要发生什么
task release -- --version=1.3.0               # 同一个版本号的显式写法，效果一样
```

版本号可以直接写在 `--` 之后（第一个裸词），也可以 `--version=1.3.0`；
其余取值一律 `--key=value`：`--remote=origin`、`--push=false`、`--dry-run`。
环境变量通道 `VERSION=1.3.0 REMOTE=origin PUSH=false DRY_RUN=true task release` 仍然可用，
CI 走的就是它；两者都设时以命令行上的为准。

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
| `web` | 按锁文件装依赖、打包前端静态产物 | — |
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

框架侧有两处接线：全局命令靠 `scripts/task-entry.sh` 运行时遍历 `apps/*/Taskfile.y*ml`
（换掉目录内容不用改它）；单端命名空间靠根 `Taskfile.yml` 的 `includes`（换端或改目录名要
同步那 4 行）。所以换掉整个工程、或改名成 `apps/<新名字>` 时，只要里面还叫 `Taskfile.yml`，
全局命令立刻接上，`includes` 那一行跟着改一下即可。

### 替换前端

**步骤**

1. 移走旧工程：`git rm -r apps/web`（想留后路就先 `git mv apps/web apps/web.old`）。
2. 把新前端工程**拷成 `apps/web`**（`cp -r` / `git clone` 到该路径）。换名成别的目录名也可以，
   但要同步改 `scripts/release.sh` 的 `PKG_FILE`、以及两份 `AGENTS.md` 与本文的路径引用。
3. 补 `apps/web/Taskfile.yml`，任务名对上统一入口（定义哪个就派发哪个，没定义就跳过）：

  | 任务 | 根里由谁调用 |
  |---|---|
  | `install`（支持 `-- --ci` 开关） | `task install` / `task install -- --ci`，单端 `task web:install` |
  | `dev` | `task dev` / `task web:dev` |
  | `build` | `task build` / `task web:build` |
  | `lint` | `task lint` / `task web:lint` |
  | `clean` | `task clean` / `task web:clean` |
  | `fmt`（可选） | `task fmt`；本端没有就跳过（`lint` 只检查，不顺手格式化） |
  | `test`（可选） | `task test` |

  目录名若换了，根 `Taskfile.yml` 的 `includes` 里 `web:` 那 4 行要跟着改。

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

1. 移走旧服务：`git rm -r apps/server`（或 `git mv` 到 `apps/server.old`）。
2. 把新服务拷成 `apps/server`。换目录名要同步本文与 `docs/architecture.md` 里的路径引用，
   以及 `.gitignore` 里按目录名匹配的 `data/`、`configs/config.local.yaml`（确认仍覆盖新路径）。
   新工程要额外容器时，把自己的 compose 文件放在自己的目录里，由自己的 `dev` 负责拉起
   （现有后端就是这样做的：`apps/server/deployments/docker-compose.yml`）。
3. 补 `apps/server/Taskfile.yml`，任务名对上统一入口（定义哪个就派发哪个，没定义就跳过）：

   | 任务 | 根里由谁调用 |
   |---|---|
   | `install`（支持 `-- --ci` 开关） | `task install` / `task install -- --ci`，单端 `task server:install` |
   | `dev` | `task dev` / `task server:dev` |
   | `build` | `task build` / `task server:build` |
   | `test` | `task test` / `task server:test` |
   | `lint` | `task lint` / `task server:lint` |
   | `fmt` | `task fmt`（`lint` 只检查，不顺手格式化） |
   | `clean` | `task clean` / `task server:clean` |

   目录名若换了，根 `Taskfile.yml` 的 `includes` 里 `server:` 那 4 行要跟着改。

4. 调整下列配置（只在新服务自己的目录里改）：

   | 配置项 | 位置 | 改什么 |
   |---|---|---|
   | 监听端口 | `configs/config*.yaml` | 与前端 `vite.config.ts` 的 proxy target 对上，默认 8080 |
   | 环境变量前缀 | `internal/config`（或新框架的对应位置） | 换了前缀要同步本文「存储」一节的环境变量示例与 `apps/server/AGENTS.md` 第 6 节（配置与密钥） |
   | 配置文件位置与文件名 | `configs/config.yaml` 等 | 本文、`.gitignore` 的 `configs/config.local.yaml`、Taskfile 的 `CONFIG_FILE` 都引用这个路径，换名要一起改 |
   | 数据目录 | `data/` | 默认 SQLite 与缓存快照落在这里，`.gitignore` 已忽略同名的 `data/` |
   | 二进制名 | Taskfile 的 `APP_NAME`、`Dockerfile`、`.github/workflows/release.yml` 的 `APP_NAME` | 本文 `-version` 示例与 `docker build` 引用它 |
   | 热重载配置 | `.air.toml` 与 Taskfile 的 `AIR_CONFIG` | `task dev` 派发的 `dev` 任务走 `air -c .air.toml`；新工程不用 air 就改 `dev` 任务，别让根 `task dev` 直接挂掉 |
   | 接口契约 | 响应体、错误码、分页、鉴权头 | 必须满足 [`docs/contracts/api.md`](docs/contracts/api.md)。新服务自带另一套形状就是契约变更：先改契约，再改前端 |
   | 版本号来源 | Taskfile 的 `APP_VERSION` | 框架约定根目录 `VERSION` 是正本，构建期用 `-X` 注入；独立使用（读不到 `VERSION`）时必须能退化成 `dev`，不能报错 |

5. 生效：`task install` → `task server:dev -- --once`（单次启动，确认能连库、能起来）→
   `task test`、`task lint`、`task build` 全绿。

**前端不需要改**，除非新后端换了端口或改了契约形状。

### 替换之后

- **改那一端的规范**：`apps/server/AGENTS.md` / `apps/web/AGENTS.md` 是那一端的规范正本，
  旧的是 Fuego + gorm 分层、React + antd 约定；新工程换了框架，就把这份文件改成新工程的形状。
  留着过时的正本比没有更糟——AI 会照着它改。
  改的时候保持**自包含**：不链接根目录或另一端的文档，需要的接口形状、任务名直接写在里面。
- **保持契约单向**：新的一端只通过 `docs/contracts/api.md` 与另一端见面，不要为了省事让它
  直接 import / 读取另一端的代码或配置。
- **跑一遍仓库级自检**：`task fmt && task lint && task test && task build`。

## 占位内容替换清单

模板里的名字都是占位，按下面这张表改一遍即可：

| 占位 | 出现位置 | 替换为 |
|---|---|---|
| `example.com/app` | `apps/server/go.mod` 与所有 import | 你的模块路径（如 `github.com/you/project`） |
| `app` / `App` | `configs/*.yaml` 的 `app.name`、缓存前缀、`index.html` 标题、`AppLayout` 里的品牌名 | 项目名 |
| `app-server` | `apps/server/Taskfile.yml`、`Dockerfile`、`main.go` 的 `-version` 输出、`.github/workflows/release.yml` 的 `APP_NAME` | 二进制名 |
| `APP_` | `internal/config/config.go` 的所有环境变量 | 你的环境变量前缀 |
| `admin` / `admin123` | `configs/config.yaml` 的 `app.seed_admin_*` | 初始管理员账号 |
| `app_token` | `apps/web/src/constants/index.ts` | 前端令牌键（换项目名时一起换） |
| `app-web` | `apps/web/package.json` 的 `name` | 前端包名 |
| `app-postgres` / `app-mysql` / `app-redis` | `apps/server/deployments/docker-compose.yml` | 容器名与库名 |
| `0.1.0` | `VERSION`、`apps/web/package.json` | 起始版本 |
| `https://app.example.com` | `configs/config.prod.yaml` 的 CORS | 你的域名 |
| `biz_note` / `Note` / `/notes` | 示例模块（见下） | 真实业务实体 |

示例模块要整体删除时，按这条链路清一遍（删完 `go build ./...` 会告诉你漏了哪里）：

```
apps/server/internal/model/note.go
apps/server/internal/dto/note.go
apps/server/internal/repository/note.go
apps/server/internal/service/note.go
apps/server/internal/handler/note.go
apps/server/internal/bootstrap/app.go        # models 清单 + 装配 + Handlers
apps/server/internal/server/router.go        # 路由组
apps/web/src/types/index.ts              # NoteItem
apps/web/src/api/notes.ts                # 整个文件
apps/web/src/pages/notes/                # 整个目录
apps/web/src/router/index.tsx            # 路由
apps/web/src/config/nav.ts               # 菜单项
```

## 安装 task

```bash
go install github.com/go-task/task/v3/cmd/task@latest   # 装到 $(go env GOPATH)/bin
brew install go-task/tap/go-task                        # macOS
sudo snap install task --classic                        # Ubuntu
```
