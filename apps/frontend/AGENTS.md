# apps/frontend — 前端开发规范

本文件是**前端规范的唯一正本**，只管 `apps/frontend/` 下的改动。目标不是描述项目，
而是约束改动：照着做，改出来的代码和现有的看起来像一个人写的。

- 后端规范：[`../backend/AGENTS.md`](../backend/AGENTS.md)
- 接口协议与错误码（两端共用）：[`../../docs/contracts/api.md`](../../docs/contracts/api.md)
- 跨端全景与文档地图：[`../../docs/architecture.md`](../../docs/architecture.md)
- 怎么跑起来、占位替换：[`../../README.md`](../../README.md)

响应体形状、码表、分页字段这里**不重复写**：只写在契约正本里，本文只写「怎么接、怎么展示」。

> **本目录是独立工程**：不 import 总框架或 `apps/backend` 的任何代码，也不依赖框架的配置才能
> 构建（缺了就降级，不能报错）。根 `Taskfile.yml` 通过 `includes` 调用本目录，方向只能是
> 框架 → 这里。边界细则见 [`../../AGENTS.md`](../../AGENTS.md) 第 3 节，
> 整端替换步骤见 [`../../README.md`](../../README.md)「替换前端」。

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
├── types/        与后端 dto 一一对应的类型
├── utils/        纯函数（格式化、查询串）
├── theme.ts      antd 主题
└── App.tsx       挂 providers + 路由
```

依赖方向只能从上往下：`pages → components / hooks / api → constants / types / utils`。
反向 import（比如 `api/` 里 import `pages/`）一定是设计错了；`utils` 与 `types` 之间不留环。

## 2. 硬性约束

- **页面不直接 `fetch`**：一律走 `api/`，令牌注入与错误处理只写一遍。
- **`types` 与后端 `dto` 一一对应**：字段名照抄 json tag，不要另起一套更顺手的名字。
- **路径别名只用 `@/`**：`@/*` 在 `tsconfig.json` 与 `vite.config.ts` 各有一份，缺一边就是
  「tsc 通过、vite build 报 Could not resolve」。新增别名要同时改两处。
- **令牌只走 `api/client.ts` 的 `getToken/setToken`**，键名来自 `constants`，不要在组件里
  直接 `localStorage.getItem('app_token')`。
- **请求只走 `constants` 的 `API_BASE`**：开发期由 Vite 代理到后端，代码里永远写相对路径，
  不要硬编码 `http://localhost:8080`。
- **错误是异常**：`client.ts` 把非 `code === 0` 折叠成 `ApiError`，页面用 `err.message` 展示、
  用 `err.code` 分支；不要自己解析响应体里的 `code`。
- **一个请求库**：不要为了「顺手」再引 axios / swr / react-query；换实现只换 `client.ts`。

## 3. 新增一个页面 / 模块的清单

按顺序做，每一步都能单独跑起来：

1. `types/index.ts`：照后端 `dto` 加 `XxxItem`、请求体、`PageResult<T>`（字段名不改写）。
2. `api/xxx.ts`：一个资源一个文件，函数名用动词（`listNotes`/`createNote`），路径拼 `API_BASE` 之后的段。
3. `pages/xxx/index.tsx`：页面组件，默认导出，负责取数、加载态、空态、错误态。
4. `router/index.tsx`：需要登录的挂进 `RequireAuth` 包裹的 children。
5. `config/nav.ts`：加菜单项（key 与路由路径一致）。
6. 需要新的通用块时：页面级放 `components/page/`，无业务语义的放 `components/ui/`。

后端侧对应的动作（`model` → … → `handler`）不属于本文件，见后端规范。

## 4. 取数、加载态与错误

- **列表用 `hooks/use-list.ts`**：它只管 loading / error / reload；筛选条件作为 `deps` 传入，
  漏写依赖的表现是「改了筛选条件但列表没刷新」。
- **加载态与错误态在页面里处理**：hook 只负责取数与重试，怎么展示由页面决定
  （表格骨架、`Alert`、空态文案）。
- **401 由 `client.ts` 统一处理**（清本地令牌），页面不要写「401 就跳登录」的分支；
  跳转是路由层 `RequireAuth` 的职责。
- **别吞异常**：`catch` 里要么展示、要么上报，不要 `catch {}` 之后当没事发生。
- **过期响应要丢弃**：手写 `useEffect` 取数时，用 `cancelled` 标记防止旧响应覆盖新状态
  （`use-list.ts` 里的写法就是模板）。

## 5. 组件与样式

- **UI 库只用 antd**，主题在 `theme.ts` 收口；不要在页面里写覆盖 antd 内部类名的 CSS。
- **`components/ui` 不带业务语义**：能被任意页面复用，不 import `types` 里的业务实体。
- **`components/page` 是页面级公共块**（如列表工具栏），可以认识业务实体，但不能发请求。
- **全局样式只有 `index.css`**；其余用 antd 组件属性或内联 style，不新增第二个 CSS 文件
  来做同一件事。

## 6. 命名

| 对象 | 规则 | 例 |
|---|---|---|
| 页面目录 | kebab-case，入口固定 `index.tsx` | `pages/notes/index.tsx` |
| 组件文件 | PascalCase | `components/layout/AppLayout.tsx` |
| hook / 工具 / api | kebab-case | `hooks/use-list.ts`、`api/client.ts` |
| 类型 | PascalCase，字段 snake_case | `NoteItem`、`page_size` |
| 常量 | UPPER_SNAKE，集中在 `constants/` | `API_BASE`、`DEFAULT_PAGE_SIZE` |

## 7. 碰到契约时的联动

`types/` 与 `api/` 是契约的前端半边，改它们要按
[契约正本的「变更流程」](../../docs/contracts/api.md#7-变更流程) 走：

1. 先改 `docs/contracts/api.md`；
2. 再改后端 `dto` / `errs`；
3. 同一个提交里改 `src/types/index.ts` 与对应 `api/*.ts`；
4. `task lint` 会跑 `tsc`，`types` 与 `dto` 对不上在这里暴露。

只有前端内部的改动（拆分组件、调样式）不需要走这套流程。

## 8. 改动后必须自检

```bash
task -d apps/frontend typecheck   # tsc --noEmit，仓库当前没有 eslint，这一步就是主要防线
task -d apps/frontend build       # 类型检查 + 打包，确认 alias 与产物没问题
```

根目录 `task lint` 会同时跑后端 `go vet` 与前端 `typecheck`，提 PR 前跑一次即可。
改了 `package.json` 的依赖或脚本时，连带检查 `../../README.md` 的命令表是否还对。

## 9. 不要做的事

- 不要在页面里写「没登录就跳转」的分支（路由层 `RequireAuth` 负责）。
- 不要在组件里拼 URL、拼查询串（`utils` 里已有纯函数）。
- 不要在 `api/` 里做业务计算（重排、过滤、聚合放页面或 `utils`）。
- 不要为「以后可能用到」预留没有调用方的组件 / hook；有第二个调用方时再抽。
- 不要新增第二套请求封装、第二套通知组件、第二套主题；先用现有的。
- 不要把后端逻辑搬进前端兜底（前端不负责补后端缺失的校验）。
- 不要 import 后端或总框架的代码，也不要读后端 `configs/` 当接口配置（跨端只走 HTTP 契约）。
- 不要让本目录的构建依赖仓库根的某个文件或变量：单独拷出去必须还能 `pnpm install && pnpm build`。
