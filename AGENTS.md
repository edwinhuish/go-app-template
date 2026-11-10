# AGENTS.md

**根目录只放索引与规则，不放任何一端的开发规范。** 前后端规范按项目边界分家，各自待在自己
的工程目录里：改哪一端，就读哪一份，不需要在一份大文件里跳过另一半的内容。

**引用是单向的**：根目录的文档可以指向 `apps/` 下的文件；`apps/` 下任何文档都必须自包含，
不得反向引用根目录或另一个子项目的文档（判据与自检见第 2、3 节）。

## 1. 文档地图

| 文档 | 回答什么问题 | 维护者 |
|---|---|---|
| [`apps/backend/AGENTS.md`](apps/backend/AGENTS.md) | 后端改动落在哪一层、硬性约束、新增模块清单、存储与配置约定；**自带对外接口摘要，不依赖本表任何其它文件** | 改后端的人 |
| [`apps/frontend/AGENTS.md`](apps/frontend/AGENTS.md) | 前端目录与依赖方向、取数与错误处理、组件与命名、新增页面清单；**自带要消费的接口摘要，不依赖本表任何其它文件** | 改前端的人 |
| [`docs/contracts/api.md`](docs/contracts/api.md) | 两端共用的接口协议：响应体、错误码、分页、鉴权头、字段与时序 | 契约变更的发起人 |
| [`docs/architecture.md`](docs/architecture.md) | 跨端全景、目录树、一次请求从前端到后端的链路、文档地图 | 架构调整时 |
| [`README.md`](README.md) | 怎么跑起来、命令表、占位替换清单、**替换整端（前端 / 后端）的步骤与配置项** | 工程配置变更时 |

## 2. 落点与命名规则

| 类型 | 路径 | 命名 | 说明 |
|---|---|---|---|
| 端级规范 | `apps/<端>/AGENTS.md` | 文件名固定 `AGENTS.md` | 每端**有且只有一份**；文件名不许改（`BACKEND_RULES.md` 这类名字 AI 与新人默认不会读） |
| 端内专项规范 | `apps/<端>/docs/<topic>.md` | 小写 kebab-case，如 `migrations.md` | 只有某端某主题确实长到需要单独成篇时才拆；拆了在端级 `AGENTS.md` 用**本目录内**的相对路径链接它 |
| 公共契约 | `docs/contracts/<topic>.md` | 同上 | **一份**契约一个文件，作为框架侧的评审与对照正本；端级规范各自内联必要摘要（见第 5 节） |
| 跨端说明 | `docs/*.md` | 同上 | 只写「两端怎么接起来」，不写任何一端的编码规则 |

硬边界：

- **根目录不新增规范文档**。想加 `BACKEND.md`、`FRONTEND.md`、`STYLE.md`、`CONVENTIONS.md`
  一概改到 `apps/<端>/` 下，或合并进已有的端级 `AGENTS.md`。
- **端内规则只有一个正本**：后端规则只写在 `apps/backend/AGENTS.md`，前端规则只写在
  `apps/frontend/AGENTS.md`。
- **端级文档必须自包含**：`apps/<端>/AGENTS.md`（以及该端 `docs/` 下的任何文件）里不得出现
  指向本端目录之外文档的链接 —— `../../docs/contracts/api.md`、`../../README.md`、
  `../backend/AGENTS.md`、`../../AGENTS.md` 都算越界。需要的跨项目信息（接口形状、任务名契约）
  在该文件里**直接写清**，不能写「见某某文档」。

## 3. apps 子项目必须自治

`apps/` 下每个子项目都是**能独立拥有生命周期**的工程：能单独拷出仓库、单独构建、单独部署。
所以依赖方向只有一个：

```
总框架（根 Taskfile.yml、scripts/、.github/workflows/、deployments/、docs/、VERSION、.gitignore）
   │  只允许这一向：框架读取 / 调用 / 引用 apps
   ▼
apps/<端>   ──✗──>  apps 下的其他子项目
            ──✗──>  总框架的代码、模块、配置
```

- **允许**：框架 → apps。根 `Taskfile.yml` 用 `includes` 调用子项目的 Taskfile；
  `scripts/release.sh` 写 `apps/frontend/package.json` 的版本号；框架的文档（本文件、
  `docs/`、`README.md`）链接到 `apps/` 内的文件。
- **禁止**：apps → 框架，以及 apps A → apps B。跨端只能通过
  [`docs/contracts/api.md`](docs/contracts/api.md) 这条契约见面 —— HTTP 是边界，不是 import。

| 禁止 | 长什么样 |
|---|---|
| import 另一个子项目的代码 | 前端 `import '../backend/...'`、后端 import 前端导出的类型 |
| 读另一个子项目的文件或配置 | 后端读 `../frontend/package.json`；前端读后端 `configs/*.yaml` |
| 复用框架层的代码或模块 | 子项目 import 根目录 `scripts/` 里的实现来生成代码 |
| 缺了框架就跑不起来 | 构建前必须先 `cd` 到仓库根；依赖根 `Taskfile.yml` 的变量才不报错 |
| 相对路径逃出本目录 | 子项目的源码、配置、Taskfile 里出现 `../../` |
| 文档里外链别处 | 端级 `AGENTS.md` 写「见 `../../docs/contracts/api.md`」「见前端规范」；该端 `docs/` 下的文件同样不能外链 |

唯一判据：**把这个目录单独拷到别处，`install` 与 `build` 必须还能跑通**（降级可以，报错不行）。

已知的软读取（唯一登记处；新增一条必须同时满足「缺失即降级」并写进这张表）：

| 位置 | 读什么 | 缺失时的行为 | 为什么允许 |
|---|---|---|---|
| `apps/backend/Taskfile.yml` | `../../VERSION` | 退化 `dev`，构建照常 | 版本号正本是仓库根的 `VERSION`；它只当缺省值，不参与编译逻辑 |

自检（在仓库根执行）：

```bash
# 代码与配置：预期只命中上面登记的那一行
grep -rn --exclude-dir=node_modules --exclude-dir=dist '\.\./\.\.' apps/ \
  --include='*.go' --include='*.ts' --include='*.tsx' --include='*.yml' --include='*.yaml' --include='*.json'

# 文档：预期无输出（apps 下的文档不引用任何外部路径）
grep -rn --exclude-dir=node_modules '](\.\./' apps/
```

替换整端时最容易破的就是这两层（代码、文档），步骤见 [`README.md`](README.md) 的「替换前端 / 替换后端」。

## 4. 该读哪份：按改动路径路由

| 你改了 | 必读 |
|---|---|
| `apps/backend/**` | `apps/backend/AGENTS.md` |
| `apps/frontend/**` | `apps/frontend/AGENTS.md` |
| `apps/backend/internal/dto`、`apps/backend/internal/pkg/errs`、`apps/backend/internal/pkg/response`、`apps/frontend/src/types`、`apps/frontend/src/api` | 上表对应的一份 **+** `docs/contracts/api.md` |
| `apps/*/Taskfile.yml`、根 `Taskfile.yml`、`deployments/`、`scripts/`、`.github/workflows/` | `README.md` 的命令表与发版一节；改完四处（根 Taskfile、端 Taskfile、README、CI）对齐，并确认没引入 apps → 框架的依赖（第 3 节） |
| 整端替换（换掉 `apps/backend` 或 `apps/frontend`） | `README.md` 的「替换前端 / 替换后端」+ 第 3 节 + 那一端的 `AGENTS.md`（规范要改成新工程的形状） |
| `docs/**` | 本文第 5 节的一致性规则 |

## 5. 公共约定怎么保持一致

公共约定指：接口前缀、响应体形状、错误码、分页参数、鉴权头、字段命名。

因为端级文档必须自包含（第 2 节），这类信息会在三个地方各出现一次。这是刻意的冗余，
不是「写了两遍」：

| 位置 | 角色 |
|---|---|
| [`docs/contracts/api.md`](docs/contracts/api.md) | 正本：评审、对照、对外解释都以它为准 |
| `apps/backend/AGENTS.md` 第 1 节 | 后端自己那份「本服务要实现什么」 |
| `apps/frontend/AGENTS.md` 第 2 节 | 前端自己那份「本工程要消费什么」 |

1. **摘要只保必要字段**：端级文档写端点、响应体形状、码表、分页、鉴权头即可，不搬实现细节、
   不抄整份契约。语义必须与正本一致；冲突时以正本为准，并当次修掉端级那一句。
2. **一次改动闭合（三处同一提交）**：先改契约文档 → 再改实现（后端 `dto`/`errs`/`response`，
   前端 `types`/`client.ts`）→ 同步两端规范里的摘要。落下一处，下一次就没人敢信文档。
3. **代码锚点**：契约正本里的锚点表列了「契约要素 ↔ 后端实现 ↔ 前端实现」，
   改的时候逐行对号入座，不靠记忆找文件。
4. **靠工具与计数兜底**：`task lint`（`go vet` + 前端 `tsc`）抓 `types` 与 `dto` 的漂移；
   码表行数三处应当一致，快速核对：

   ```bash
   grep -c '^| `[0-9]' docs/contracts/api.md apps/*/AGENTS.md
   ```

   三个数字不相等，说明有一处漏改。

## 6. 两端独立维护

- **归属清晰**：一端规范的改动只属于那一端的负责人，`docs/contracts/` 的改动需要两端任意
  一方发起、另一方确认（评审时看契约文档的 diff，不看实现 diff）。
- **各自演进**：后端加一层、前端换组件库，只改自己那一份，不牵动另一端。
  牵动另一端的一定是契约变更，走第 5 节流程。
- **自包含地写，不要互相指路**：一端要说明跨项目信息，就在自己文件里写清；写「见另一端的
  规范」既是外链（违反第 2 节），也让那个文件在被单独拿走时失效。
- **可以整端换掉**：每一端都能被另一个同语言工程整体替换，换个目录 + 改几处接线即可，
  步骤与配置项见 [`README.md`](README.md)「替换前端 / 替换后端」。替换时不允许顺手把
  另一端或框架改造成彼此耦合 —— 那会直接违反第 3 节。
- **各自自检**：后端 `task -d apps/backend test`、前端 `task -d apps/frontend typecheck`；
  根目录 `task fmt / lint / test / build` 只是编排入口，不新增命令。
- **删示例别删规范**：`README.md` 的占位替换清单里删 `Note` 示例模块时，
  两份 `AGENTS.md` 里的示例引用只是「样板」，不必跟着删，除非该约定本身失效。

## 7. 仓库级自检

```bash
task fmt        # gofmt
task lint       # go vet + 前端 tsc
task test       # 后端单测
task build      # 后端二进制 + 前端静态产物
```

改了路由、配置字段、环境变量名、文档路径时，连带检查：`docs/architecture.md` 的目录树与
文档地图、`README.md` 的链接与命令表、以及两份端级 `AGENTS.md` 是否仍然自包含
（`grep -rn --exclude-dir=node_modules '](\.\./' apps/` 无输出）。
