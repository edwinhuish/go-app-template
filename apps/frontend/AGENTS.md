# apps/frontend — 前端开发规范

本文件是**前端规范的唯一正本**，只管 `apps/frontend/` 下的改动。目标不是描述项目，
而是约束改动：照着做，改出来的代码和现有的看起来像一个人写的。

**本文件自包含**：只描述本目录的约定、结构与用法，不引用任何目录之外的文档——仓库根的说明、
公共契约、`apps/` 下其他子项目的规范都不在这里出现。需要跨项目的信息（接口形状、任务名）
下面直接给出，读这一份就能开工。

## 0. 独立工程约束

- 不 import 仓库根或 `apps/` 下其他子项目的代码，也不读它们的文件当配置；与外部只通过 HTTP 交互。
- 不依赖本目录之外的任何文件才能构建：`pnpm install && pnpm build` 单独跑必须成功。
- 外部编排方（仓库根的统一入口）按**任务名**调用本目录的 `Taskfile.yml`，所以这些名字要保持：
  `install`（可带 `-- --ci`）/ `dev` / `build` / `lint` / `clean`。单端入口是
  `task frontend:<任务>`，全量入口派发这几个名字，没定义就跳过本端。本端没有 `fmt` 任务
  （工程里没有格式化工具），`preview` 仅供本地使用。改名前先确认调用方。
- 根目录有两种调法，都把参数**原样**转进来：`task frontend:<任务> -- <参数...>`（根 Taskfile 的
  `includes` 直连本目录，参数零解析）、以及全局命令的 `task -d apps/frontend <任务> -- <参数...>`。
  其中可能有属于其他子项目的开关（例如后端的 `--db=pg`），因此本端的任务只挑自己认识的参数用，
  对不认识的参数**一律忽略、不报错**：`install` 只认 `--ci`，`dev` / `build` / `lint` / `clean` 不读参数。
- `package.json` 的 `version` 字段会被外部发版流程同步写回，**不要删这个字段**，也不要改成别的结构。

## 1. 目录与依赖方向

```
src/
├── api/          接口封装：client.ts 统一注入令牌 / 拆包 / 抛 ApiError，其余按资源分文件
├── components/   layout（外壳）、page（页面级公共块）、ui（无业务语义的小组件）
├── config/       导航等静态配置
├── constants/    常量（API_BASE、令牌键、分页大小）
├── hooks/        可复用逻辑（useList）
├── pages/        页面，一个目录一页
├── providers/    全局状态（登录态）
├── router/       路由表与守卫
├── types/        与服务端接口字段一一对应的类型
├── utils/        纯函数（格式化、查询串）
├── theme.ts      antd 主题
└── App.tsx       挂 providers + 路由
```

依赖方向只能从上往下：`pages → components / hooks / api → constants / types / utils`。
反向 import（比如 `api/` 里 import `pages/`）一定是设计错了；`utils` 与 `types` 之间不留环。

## 2. 对接的服务端接口

这一节是本工程依赖的线格式，与 `src/api/client.ts`、`src/types/index.ts` 一一对应；
服务端按它实现，所以它是**兼容性约束**：改这里等于换接口，要先确认服务端。

| 项 | 约定 |
|---|---|
| 前缀 | `constants` 的 `API_BASE = '/api/v1'`，开发期由 Vite 代理到 `http://127.0.0.1:8080` |
| 鉴权头 | `Authorization: Bearer <token>`，由 `client.ts` 注入；不带令牌的接口只有登录 |
| 请求 ID | 响应头 `X-Request-ID` 与响应体 `request_id`，排查问题时让用户报这个值 |
| 字段命名 | 一律 `snake_case`，`types` 里照抄，不另起名字 |
| 时间 | RFC3339 字符串（`2026-11-10T09:30:00Z`）；`expires_at` 是 Unix 秒 |

响应体只有一种形状，`client.ts` 负责拆包：

```json
{ "code": 0, "message": "ok", "data": {}, "request_id": "0f3c1a7e9b2d4c55" }
```

- `code === 0` 才是成功；失败时 `data` 省略，`message` 可直接展示给用户。
- `client.ts` 把非 0 折叠成 `ApiError(message, code)`；页面按 `err.code` 分支、`err.message` 展示。

业务码表（页面分支只按语义写，不要按 HTTP 状态猜）：

| code | HTTP | 含义 | 页面通常怎么处理 |
|---|---|---|---|
| `0` | 200 / 201 | 成功 | — |
| `40001` | 400 | 参数错误 | 展示 `message`（服务端给的校验文案） |
| `40101` | 401 | 未登录 / 登录态失效 | `client.ts` 清令牌，路由层送回登录页 |
| `40301` | 403 | 无权限 | 提示无权限，不要弹登录 |
| `40401` | 404 | 资源不存在（含已删除） | 提示已被删除并刷新列表 |
| `40901` | 409 | 冲突（重名、并发修改） | 提示刷新重试 |
| `50001` | 500 | 服务端内部错误 | 通用错误提示 |
| `50401` | 504 | 服务端处理超时 | 提示稍后重试 |

分页：请求 `page`（从 1 开始，默认 1）、`page_size`（默认 20，上限 200）、可选 `keyword`；
响应 `{ "list": [], "total": 0, "page": 1, "page_size": 20 }`，`list` 空结果是 `[]` 而不是 `null`。
对应 `types` 里的 `PageResult<T>` 与 `constants` 的 `DEFAULT_PAGE_SIZE`。

乐观锁（可选）：部分更新接口要求回传读到的 `updated_at`，服务端据此判并发；不回传则最后写入者获胜。

两条会直接看到的行为，别当成 bug：

- **数据范围由服务端按身份收窄**：普通用户的列表里只会出现自己的记录，带别人的 `owner_id`
  也不会放开；读写别人的单条记录一律 `40401`（不是 `40301`），按「记录不存在」处理即可。
- **登录失败会锁定**：同一用户名连续失败 5 次后 15 分钟内一律 `40301`，锁定期间密码正确也拒绝。
  登录页要把 `message` 原样展示，不要把它当成「角色不够」。
- **`PUT` 是全量覆盖**：省略的字段会被清空，而不是保持原值。表单提交前要把整行数据带上，
  或者先读一次再补全字段——否则「只改标题」会把正文清掉。

## 3. 硬性约束

- **页面不直接 `fetch`**：一律走 `api/`，令牌注入与错误处理只写一遍。
- **`types` 与服务端字段一一对应**：字段名照抄 `snake_case`，不要另起一套更顺手的名字。
- **路径别名只用 `@/`**：`@/*` 在 `tsconfig.json` 与 `vite.config.ts` 各有一份，缺一边就是
  「tsc 通过、vite build 报 Could not resolve」。新增别名要同时改两处。
- **令牌只走 `api/client.ts` 的 `getToken/setToken`**，键名来自 `constants`，不要在组件里
  直接 `localStorage.getItem('app_token')`。
- **请求只走 `constants` 的 `API_BASE`**：代码里永远写相对路径，不要硬编码 `http://localhost:8080`。
- **错误是异常**：页面用 `err.message` 展示、`err.code` 分支；不要自己解析响应体里的 `code`。
- **一个请求库**：不要为了「顺手」再引 axios / swr / react-query；换实现只换 `client.ts`。

## 4. 新增一个页面 / 模块的清单

按顺序做，每一步都能单独跑起来：

1. `types/index.ts`：加 `XxxItem`、请求体、`PageResult<T>`（字段名与第 2 节一致，不改写）。
2. `api/xxx.ts`：一个资源一个文件，函数名用动词（`listNotes`/`createNote`），路径拼 `API_BASE` 之后的段。
3. `pages/xxx/index.tsx`：页面组件，默认导出，负责取数、加载态、空态、错误态。
4. `router/index.tsx`：需要登录的挂进 `RequireAuth` 包裹的 children。
5. `config/nav.ts`：加菜单项（key 与路由路径一致）。
6. 需要新的通用块时：页面级放 `components/page/`，无业务语义的放 `components/ui/`。

服务端侧的表结构、路由实现不在本工程职责内；本工程只消费第 2 节那套线格式。

## 5. 取数、加载态与错误

- **列表用 `hooks/use-list.ts`**：它只管 loading / error / reload；筛选条件作为 `deps` 传入，
  漏写依赖的表现是「改了筛选条件但列表没刷新」。
- **加载态与错误态在页面里处理**：hook 只负责取数与重试，怎么展示由页面决定
  （表格骨架、`Alert`、空态文案）。
- **401 由 `client.ts` 统一处理**（清本地令牌），页面不要写「401 就跳登录」的分支；
  跳转是路由层 `RequireAuth` 的职责。
- **别吞异常**：`catch` 里要么展示、要么上报，不要 `catch {}` 之后当没事发生。
- **过期响应要丢弃**：手写 `useEffect` 取数时，用 `cancelled` 标记防止旧响应覆盖新状态
  （`use-list.ts` 里的写法就是模板）。

## 6. 组件与样式

- **UI 库只用 antd**，主题在 `theme.ts` 收口；不要在页面里写覆盖 antd 内部类名的 CSS。
- **`components/ui` 不带业务语义**：能被任意页面复用，不 import `types` 里的业务实体。
- **`components/page` 是页面级公共块**（如列表工具栏），可以认识业务实体，但不能发请求。
- **全局样式只有 `index.css`**；其余用 antd 组件属性或内联 style，不新增第二个 CSS 文件
  来做同一件事。

## 7. 命名

| 对象 | 规则 | 例 |
|---|---|---|
| 页面目录 | kebab-case，入口固定 `index.tsx` | `pages/notes/index.tsx` |
| 组件文件 | PascalCase | `components/layout/AppLayout.tsx` |
| hook / 工具 / api | kebab-case | `hooks/use-list.ts`、`api/client.ts` |
| 类型 | PascalCase，字段 snake_case | `NoteItem`、`page_size` |
| 常量 | UPPER_SNAKE，集中在 `constants/` | `API_BASE`、`DEFAULT_PAGE_SIZE` |

## 8. 运行与自检

在本目录执行（不必依赖仓库根的入口）：

```bash
task install        # pnpm install
task install -- --ci # pnpm install --frozen-lockfile（锁文件与 package.json 不一致直接失败）
task dev            # Vite 开发服务 :5173，/api 代理到 :8080
task lint           # tsc --noEmit；本工程没有 eslint，这一步就是主要防线
task build          # 类型检查 + 打包到 dist/
```

改动后的连带检查：

- 动了线格式（前缀、响应体、码表、分页字段）→ 更新第 2 节，并确认 `types` 与 `client.ts` 同步，
  `task lint` 必须通过；
- 动了 `vite.config.ts` 的端口或代理 → 确认与服务端监听端口一致；
- 动了 `Taskfile.yml` 的任务名或 `package.json` 的 `version` / `scripts` → 确认第 0 节的要求还成立。

## 9. 不要做的事

- 不要在页面里写「没登录就跳转」的分支（路由层 `RequireAuth` 负责）。
- 不要在组件里拼 URL、拼查询串（`utils` 里已有纯函数）。
- 不要在 `api/` 里做业务计算（重排、过滤、聚合放页面或 `utils`）。
- 不要为「以后可能用到」预留没有调用方的组件 / hook；有第二个调用方时再抽。
- 不要新增第二套请求封装、第二套通知组件、第二套主题；先用现有的。
- 不要把服务端逻辑搬进前端兜底（前端不负责补服务端缺失的校验）。
- 不要 import 仓库根或其他子项目的代码，也不要读服务端的配置文件当接口定义（只认第 2 节）。
- 不要让本目录的构建依赖目录之外的某个文件或变量：单独拷出去必须还能 `pnpm install && pnpm build`。
