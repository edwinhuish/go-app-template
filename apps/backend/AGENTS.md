# apps/backend — 后端开发规范

本文件是**后端规范的唯一正本**，只管 `apps/backend/` 下的改动。目标不是描述项目，
而是约束改动：照着做，改出来的代码和现有的看起来像一个人写的。

- 前端规范：[`../frontend/AGENTS.md`](../frontend/AGENTS.md)
- 接口协议与错误码（两端共用）：[`../../docs/contracts/api.md`](../../docs/contracts/api.md)
- 跨端全景与文档地图：[`../../docs/architecture.md`](../../docs/architecture.md)
- 怎么跑起来、占位替换：[`../../README.md`](../../README.md)

接口形状、码表、分页字段这里**不重复写**：只写在契约正本里，本文只写「怎么实现」。

> **本目录是独立工程**：不 import 总框架或 `apps/frontend` 的任何代码，也不依赖框架的配置才能
> 构建（缺了就降级，不能报错）。根 `Taskfile.yml` 通过 `includes` 调用本目录，方向只能是
> 框架 → 这里。边界细则见 [`../../AGENTS.md`](../../AGENTS.md) 第 3 节，
> 整端替换步骤见 [`../../README.md`](../../README.md)「替换后端」。

## 1. 先确认改的是哪一层

| 你要做的事 | 落点 | 不要碰 |
|---|---|---|
| 加一个表 / 字段 / 索引 | `internal/model` | handler、dto |
| 加一条查询 | `internal/repository`（接口 + 实现） | service 里的 SQL |
| 加业务规则、状态流转、事务 | `internal/service` | repository 里的 if 判断 |
| 加接口参数、响应字段 | `internal/dto` | 直接改 model 的 json tag |
| 加路由 / 中间件 | `internal/server` | handler 里拼状态码 |
| 加连接、缓存、日志实现 | `internal/infra` | service 里 new 客户端 |

依赖方向只能从上往下：

```
handler ──> service ──> repository ──> model
              │             │
              └──> infra    └──> pkg（errs / response / dialect，叶子包）
```

反向 import 一定是设计错了：要么这段逻辑放错了层，要么该用 Option 注入。

## 2. 硬性约束

- `dto` 不 import `model`；互转写在 service 的 `toXxxItem` 里。
- `service` 不 import gin；要当前用户，用 `service.ActorFrom(ctx)`。
- `repository` 不返回 dto；也不 import `service`。
- `internal/pkg/*` 不 import `internal` 其它包（`pkg` 是叶子）。
- **错误只走 `internal/pkg/errs`**：service 返回 `errs.BadRequest/ErrNotFound/...`；
  handler 统一 `response.Fail`，不要自己写 `c.JSON(400, ...)`。
- **响应只用 `response.OK/Created/Fail`**，形状与状态码推导见契约正本。
- **每个对外函数带 `ctx context.Context`** 并一路传到底：超时与取消靠它传导到驱动层。
- **软删除用 `model.DeletedAt`**，唯一索引把 `deleted_at` 排进去（`priority` 见 `model/user.go`）。
- **跨表写入用 `repository.TxManager`**，不要在 service 里手写 `db.Begin()`。

## 3. 新增一个模块的清单

按顺序做，每一步都能单独编译：

1. `internal/model/xxx.go`：字段、索引、`TableName()`。
2. `internal/bootstrap/app.go` 的 `models` 切片里登记（漏了会在首次写入时报 no such table）。
3. `internal/repository/xxx.go`：接口 + 实现；通用 CRUD 用嵌入的 `Base[T]`，只写独有查询。
4. `internal/dto/xxx.go`：`XxxItem`（响应）、`XxxCreateRequest`/`XxxUpdateRequest`（入参，带 binding 标签）、
   `XxxListQuery`（嵌入 `PageQuery`）。
5. `internal/service/xxx.go`：接口 + `NewXxxService` + `XxxOption`；业务校验、事务、模型↔契约。
6. `internal/handler/xxx.go`：`bindJSON`/`bindQuery`/`parseID` → 调 service → `response.*`。
7. `internal/server/router.go`：需要登录的挂进 `secured` 组；管理端接口再叠 `middleware.RequireAdmin()`。
8. 测试：至少覆盖 service 的校验分支；仓储的特殊查询补集成测试。
9. 第 4 步定下的字段名就是契约的一部分，同一个提交里要改前端 `types`（见第 6 节）。

前端侧对应的动作（`types` → `api/` → `pages/`）不属于本文件，见前端规范。

## 4. 数据与存储约定

| 主题 | 约定 |
|---|---|
| 主键 | `uint` 自增，`Base` 提供 |
| 软删除 | `soft_delete` 的整数标记（0 = 未删），唯一索引把 `deleted_at` 排进去，实现「未删才唯一」 |
| 表名 | 显式 `TableName()`；系统表 `sys_`、业务表 `biz_` |
| 时间 | 一律 `time.Time`；「业务日期」用 `model.DateOnly` 截到日历日 |
| 分页 | 统一 `repository.Pagination` + `dto.PageResult`，参数归一化走 `PageQuery.Normalize` |
| 乐观锁 | 更新时把 `updated_at` 放进 fields（键为 `repository.FieldUpdatedAt`），命中 0 行则 409 |
| 事务 | 跨仓储写入用 `repository.TxManager.Transaction`，仓储按 ctx 自动加入 |
| 多库支持 | 上层永远拿 `*gorm.DB`；驱动差异只在 `infra/database` 与 `pkg/dialect` 里消化 |

「换存储不改代码」是这个模板的承诺之一：`database.driver` 改成 `postgres` / `mysql` / `sqlite`，
`cache.driver` 改成 `redis` / `memory`，业务层零改动。改完至少跑一次 `task test`，再接真实库验证。

## 5. 配置与密钥

优先级：**配置文件 < 环境变量（`APP_` 前缀）**。

- 配置文件按环境拆：`configs/config.yaml`（默认 SQLite）、`config.postgres.yaml`、`config.mysql.yaml`、`config.prod.yaml`。
- 密码、JWT 密钥、DSN 生产一律走环境变量，不进仓库。
- 启动期就校验（`config.Validate`）：驱动名、必填项、生产环境的密钥长度与 seed 开关。
  失败就退出，而不是等到第一个请求才 500。
- 新增配置项要同时改：结构体字段 + 默认值 + 环境变量覆盖 + 校验；只加结构体字段等于给了个零值。

## 6. 碰到契约时的联动

改了 `internal/dto` 或 `internal/pkg/errs` 就是在改公共契约，按
[契约正本的「变更流程」](../../docs/contracts/api.md#7-变更流程) 走，不要只改后端半边：

1. 先改 `docs/contracts/api.md`；
2. 再改 dto / errs；
3. 同一个提交里改 `apps/frontend/src/types/index.ts` 与对应 `api/*.ts`；
4. `task lint` 会跑前端 `tsc`，`types` 与 `dto` 对不上在这里暴露。

## 7. 注释与命名

- 注释写**为什么**，不写**是什么**。`// 返回列表` 没有信息量；
  `// 计数与取数分别构建查询：复用同一个 *gorm.DB 会让 Count 的 statement 残留到 Find 上` 才是。
- 踩过的坑值得写进注释：它比文档更靠近代码，改代码的人一定会看到。
- 命名用业务语言（`Note`、`OwnerID`），不要用技术缩写（`Nt`、`oid`）。
- 表名 `sys_` / `biz_` 前缀，和现有保持一致。

## 8. 改动后必须自检

```bash
task -d apps/backend fmt
task -d apps/backend vet
task -d apps/backend test
task -d apps/backend build      # 确认产物能构建
```

根目录的 `task fmt / lint / test / build` 是同一批任务的编排入口，CI 与本地跑的是同一批命令。
改了路由、配置字段、环境变量名时，连带检查：

- `docs/contracts/api.md` 是否需要同步（路由前缀、鉴权头、字段名都算契约）；
- `../../README.md` 的命令与占位表是否还对；
- 前端 `types` 与后端 `dto` 是否仍然一致。

## 9. 不要做的事

- 不要把业务逻辑写进 `middleware`（它只做横切，不认识任何实体）。
- 不要在 handler 里直接调 repository（绕过 service 等于绕过业务校验）。
- 不要在 `internal/pkg` 里加与项目相关的类型（那等于把业务下沉到叶子包）。
- 不要为了「以后可能用到」预留没有调用方的抽象；有第二个调用方时再抽。
- 不要新增第二套错误处理 / 响应格式 / 日志库；先用现有的。
- 不要在前端仓库之外的地方补前端逻辑（后端不渲染页面，也不做前端兜底）。
- 不要 import 总框架或 `apps/frontend` 的代码，也不要读它们的文件当配置（跨端只走 HTTP 契约）。
- 不要让本目录的构建依赖仓库根的某个文件或变量：单独拷出去必须还能 `task install && task build`。
