# AGENTS.md

本文件是框架层的索引与规则，不放任何一端的开发规范。前后端规范按项目边界分家，各自待在
自己的工程目录里：改哪一端，就读哪一份，不需要在一份大文件里跳过另一半的内容。

## 0. 三条铁律

**一、改代码必须同改文档。** 文档与实际不一致，比没有文档更费时间——它会让人按错的形状写代码。
所以「改完代码再看文档要不要动」不算完成，判定标准是：

- 同一个提交里完成。不允许「先合并，回头补文档」；确实来不及，就在提交说明里写清欠下哪一段、
  为什么——提交说明是最不容易被跳过的一处，它和代码在同一个 diff 里。
- 改哪一类东西要动哪些文档，见[第 5 节](#5-一次改动要闭合哪几张纸)的对照表。拿不准就按
  「对外行为变了 → 契约 + 两端摘要」这条最保守的路径走。
- 文档里出现已删除的符号（函数、字段、目录、任务名）视为缺陷，不是「历史遗留」，当次改掉。

**二、根目录只放索引与规则。** 想加 `STYLE.md`、`CONVENTIONS.md`、`BACKEND_RULES.md`
这类文件，一概改到 `apps/<端>/` 下，或合并进已有的端级 `AGENTS.md`。

**三、引用是单向的。** 框架层的文档（本文件、`docs/`、`README.md`）可以指向 `apps/` 下的文件；
`apps/` 下任何文档都必须自包含，不得反向引用根目录或另一个子项目的文档。判据与自检见第 4 节。

## 1. 读哪份：按改动路径路由

日常最常问的是这一句。先定位改的是哪条路径，再读对应的那一份（端级文档自带接口摘要，
不需要再翻契约才能看懂）：

| 你改了 | 必读 |
|---|---|
| `apps/server/**` | `apps/server/AGENTS.md` |
| `apps/web/**` | `apps/web/AGENTS.md` |
| `apps/server/internal/dto`、`internal/pkg/errs`、`internal/pkg/response`、`apps/web/src/types`、`apps/web/src/api` | 上表对应的一份 **+** [`docs/contracts/api.md`](docs/contracts/api.md) |
| `apps/*/Taskfile.yml`、根 `Taskfile.yml`、`scripts/`、`.github/workflows/` | [`README.md`](README.md) 的命令表、参数规则与发版一节；改完四处（根 Taskfile 含 `includes`、端 Taskfile、README、CI）对齐，并确认没引入 apps → 框架的依赖（第 4 节） |
| 整端替换（换掉 `apps/server` 或 `apps/web`） | `README.md` 的「替换前端 / 替换后端」+ 第 4 节 + 那一端的 `AGENTS.md`（规范要改成新工程的形状） |
| `docs/**` | 本文第 5 节的一致性规则 |
| 只改注释与文案 | 不需要动契约；但**过时注释本身就是缺陷**，改代码时顺手修掉，见第 0 节 |

## 2. 文档地图

| 文档 | 回答什么问题 | 维护者 |
|---|---|---|
| [`apps/server/AGENTS.md`](apps/server/AGENTS.md) | 后端改动落在哪一层、硬性约束、新增模块清单、存储与配置约定；**自带对外接口摘要，不依赖本表任何其它文件** | 改后端的人 |
| [`apps/web/AGENTS.md`](apps/web/AGENTS.md) | 前端目录与依赖方向、取数与错误处理、组件与命名、新增页面清单；**自带要消费的接口摘要，不依赖本表任何其它文件** | 改前端的人 |
| [`docs/contracts/api.md`](docs/contracts/api.md) | 两端共用的接口协议：响应体、错误码、分页、鉴权头、字段与时序 | 契约变更的发起人 |
| [`docs/architecture.md`](docs/architecture.md) | 跨端全景、目录树、一次请求从前端到后端的链路、文档地图 | 架构调整时 |
| [`README.md`](README.md) | 怎么跑起来、命令表、占位替换清单、**替换整端（前端 / 后端）的步骤与配置项** | 工程配置变更时 |

**新增一个文档时，三处地图要同时加行**：本表、`docs/architecture.md` 的目录树与文档地图、
`README.md` 的目录树。漏一处，那个文件就成了没人读得到的孤岛。

## 3. 新文档放哪儿、叫什么

| 类型 | 路径 | 命名 | 说明 |
|---|---|---|---|
| 端级规范 | `apps/<端>/AGENTS.md` | 文件名固定 `AGENTS.md` | 每端**有且只有一份**；文件名不许改（`BACKEND_RULES.md` 这类名字 AI 与新人默认不会读） |
| 端内专项规范 | `apps/<端>/docs/<topic>.md` | 小写 kebab-case，如 `migrations.md` | 只有某端某主题确实长到需要单独成篇时才拆；拆了在端级 `AGENTS.md` 用**本目录内**的相对路径链接它 |
| 公共契约 | `docs/contracts/<topic>.md` | 同上 | **一份**契约一个文件，作为框架侧的评审与对照正本；端级规范各自内联必要摘要（见第 5 节） |
| 跨端说明 | `docs/*.md` | 同上 | 只写「两端怎么接起来」，不写任何一端的编码规则 |
| 工作清单 | `docs/TODO.md` | 文件名固定 | **按需存在**：欠着整改项时才建，只列未完成项、一条一项带验收；一项做完就删掉那行，全部清空就把文件删掉并在第 2 节三处地图里去掉它（当前仓库没有这个文件） |

硬边界：

- **根目录不新增规范文档**（第 0 节第二条）。
- **端内规则只有一个正本**：后端规则只写在 `apps/server/AGENTS.md`，前端规则只写在
  `apps/web/AGENTS.md`。
- **端级文档必须自包含**：`apps/<端>/AGENTS.md`（以及该端 `docs/` 下的任何文件）里不得出现
  指向本端目录之外文档的链接 —— `../../docs/contracts/api.md`、`../../README.md`、
  `../server/AGENTS.md`、`../../AGENTS.md` 都算越界。需要的跨项目信息（接口形状、任务名契约）
  在该文件里**直接写清**，不能写「见某某文档」。

## 4. apps 子项目必须自治

`apps/` 下每个子项目都是**能独立拥有生命周期**的工程：能单独拷出仓库、单独构建、单独部署。
所以依赖方向只有一个：

```
总框架（根 Taskfile.yml、scripts/、.github/workflows/、docs/、VERSION.txt、.gitignore）
   │  只允许这一向：框架读取 / 调用 / 引用 apps
   ▼
apps/<端>   ──✗──>  apps 下的其他子项目
            ──✗──>  总框架的代码、模块、配置
```

- **允许**：框架 → apps。根 `Taskfile.yml` 的 `includes` 把 `<子项目>:<命令>` 命名的任务
  直接交给 `apps/<子项目>/Taskfile.yml`；根目录的 8 个全局命令由 `scripts/task-entry.sh`
  派发到 `apps/*/Taskfile.yml` 的同名任务；`scripts/release.sh` 写 `apps/web/package.json`
  的版本号；框架的文档（本文件、`docs/`、`README.md`）链接到 `apps/` 内的文件。
- **禁止**：apps → 框架，以及 apps A → apps B。跨端只能通过
  [`docs/contracts/api.md`](docs/contracts/api.md) 这条契约见面 —— HTTP 是边界，不是 import。

| 禁止 | 长什么样 |
|---|---|
| import 另一个子项目的代码 | 前端 `import '../server/...'`、后端 import 前端导出的类型 |
| 读另一个子项目的文件或配置 | 后端读 `../web/package.json`；前端读后端 `configs/*.yaml` |
| 复用框架层的代码或模块 | 子项目 import 根目录 `scripts/` 里的实现来生成代码 |
| 缺了框架就跑不起来 | 构建前必须先 `cd` 到仓库根；依赖根 `Taskfile.yml` 的变量才不报错 |
| 相对路径逃出本目录 | 子项目的源码、配置、Taskfile 里出现 `../../` |
| 文档里外链别处 | 端级 `AGENTS.md` 写「见 `../../docs/contracts/api.md`」「见前端规范」；该端 `docs/` 下的文件同样不能外链 |

唯一判据：**把这个目录单独拷到别处，`install` 与 `build` 必须还能跑通**（降级可以，报错不行）。

已知的软读取（唯一登记处；新增一条必须同时满足「缺失即降级」并写进这张表）：

| 位置 | 读什么 | 缺失时的行为 | 为什么允许 |
|---|---|---|---|
| `apps/server/Taskfile.yml` | `../../VERSION.txt` | 退化 `dev`，构建照常 | 版本号正本是仓库根的 `VERSION.txt`；它只当缺省值，不参与编译逻辑 |

自检（在仓库根执行，CI 的 `docs` job 里也跑这两条，失败即红）：

```bash
# 代码与配置：预期只命中上面登记的那一行
grep -rn --exclude-dir=node_modules --exclude-dir=dist '\.\./\.\.' apps/ \
  --include='*.go' --include='*.ts' --include='*.tsx' --include='*.yml' --include='*.yaml' --include='*.json'

# 文档：预期无输出（apps 下的文档不引用任何外部路径）
grep -rn --exclude-dir=node_modules '](\.\./' apps/
```

替换整端时最容易破的就是这两层（代码、文档），步骤见 [`README.md`](README.md) 的「替换前端 / 替换后端」。

## 5. 一次改动要闭合哪几张纸

公共约定指：接口前缀、响应体形状、错误码、分页参数、鉴权头、字段命名。

因为端级文档必须自包含（第 3 节），这类信息会在三个地方各出现一次。这是刻意的冗余，
不是「写了两遍」：

| 位置 | 角色 |
|---|---|
| [`docs/contracts/api.md`](docs/contracts/api.md) | 正本：评审、对照、对外解释都以它为准 |
| `apps/server/AGENTS.md` 第 1 节 | 后端自己那份「本服务要实现什么」 |
| `apps/web/AGENTS.md` 第 2 节 | 前端自己那份「本工程要消费什么」 |

第 0 节那条铁律落到具体改动的对照表（这就是「必须同步的文档」的完整清单）：

| 你改了什么 | 同一个提交里必须一起改 |
|---|---|
| 接口路径、字段、错误码、分页、鉴权头 | `docs/contracts/api.md` 正本 → 两端实现（`dto`/`errs`/`response`、`types`/`client.ts`）→ 两端 `AGENTS.md` 里的摘要 |
| 路由注册、中间件、分层位置、请求链路 | 端级 `AGENTS.md` 的分层与约束 + `docs/architecture.md` 的链路一节 |
| 配置项、环境变量、默认值、配置校验 | 端级 `AGENTS.md` 的配置表 + `README.md` 的存储/环境变量示例 |
| 命令、任务名、目录结构、端口 | `README.md` 命令表与目录树 + 两份端级规范里引用命令的段落 + CI 工作流 |
| 新增或删除一个文档 | 第 2 节的三处地图 |
| 清单里的一项做完 | 删掉那一行；顺带确认它的验收条件真的被验证过。全部清空时删掉 `docs/TODO.md` 本身，并同步第 2 节的三处地图 |

四条执行细节：

1. **摘要只保必要字段**：端级文档写端点、响应体形状、码表、分页、鉴权头即可，不搬实现细节、
   不抄整份契约。语义必须与正本一致；冲突时以正本为准，并当次修掉端级那一句。
2. **顺序是「契约 → 实现 → 摘要」**：反着来最容易漏第三处，因为前两处都有编译或测试盯着，
   摘要没有。
3. **代码锚点**：契约正本里的锚点表列了「契约要素 ↔ 后端实现 ↔ 前端实现」，
   改的时候逐行对号入座，不靠记忆找文件。
4. **靠工具与计数兜底**：`task lint`（`go vet` + `staticcheck` + 前端 `tsc`）抓 `types` 与 `dto` 的漂移；
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
  规范」既是外链（违反第 3 节），也让那个文件在被单独拿走时失效。
- **可以整端换掉**：每一端都能被另一个同语言工程整体替换，换个目录 + 改几处接线即可，
  步骤与配置项见 [`README.md`](README.md)「替换前端 / 替换后端」。替换时不允许顺手把
  另一端或框架改造成彼此耦合 —— 那会直接违反第 4 节。
- **各自自检**：后端 `task -d apps/server test`、前端 `task -d apps/web lint`；
  根目录 `task fmt / lint / test / build` 只是编排入口，不新增命令。
- **删示例别删规范**：`README.md` 的占位替换清单里删 `Note` 示例模块时，
  两份 `AGENTS.md` 里的示例引用只是「样板」，不必跟着删，除非该约定本身失效。

## 7. 收尾自检

```bash
task fmt        # gofmt
task lint       # go vet + staticcheck + 前端 tsc
task test       # 后端单测（CI 里带 -race）
task build      # 后端二进制 + 前端静态产物
```

改了路由、配置字段、环境变量名、文档路径时，连带检查：`docs/architecture.md` 的目录树与
文档地图、`README.md` 的链接与命令表、以及两份端级 `AGENTS.md` 是否仍然自包含
（第 4 节的两条 grep）。这三件事重复率高，值得在提交前当成一个固定的动作跑完。
