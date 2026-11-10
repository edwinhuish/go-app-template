# AGENTS.md

给在本仓库工作的 AI 协作者（以及新同事）的规则。目标不是描述项目，而是**约束改动**：
照着做，改出来的代码和现有的看起来像一个人写的。

跑起来、目录树、占位替换见 [`README.md`](README.md)；分层理由见 [`docs/architecture.md`](docs/architecture.md)。

## 1. 先确认改的是哪一层

| 你要做的事 | 落点 | 不要碰 |
|---|---|---|
| 加一个表 / 字段 / 索引 | `internal/model` | handler、dto |
| 加一条查询 | `internal/repository`（接口 + 实现） | service 里的 SQL |
| 加业务规则、状态流转、事务 | `internal/service` | repository 里的 if 判断 |
| 加接口参数、响应字段 | `internal/dto` | 直接改 model 的 json tag |
| 加路由 / 中间件 | `internal/server` | handler 里拼状态码 |
| 加连接、缓存、日志实现 | `internal/infra` | service 里 new 客户端 |

依赖方向只能从上往下（`handler → service → repository → model`）。
反向 import 一定是设计错了：要么这段逻辑放错了层，要么该用 Option 注入。

## 2. 硬性约束

- **`dto` 不 import `model`**；互转写在 service 的 `toXxxItem` 里。
- **`service` 不 import gin**；要当前用户，用 `service.ActorFrom(ctx)`。
- **`repository` 不返回 dto**；也不 import `service`。
- **`pkg/*` 不 import `internal` 其它包**（`pkg` 是叶子）。
- **错误只走 `pkg/errs`**：service 返回 `errs.BadRequest/ErrNotFound/...`；
  handler 统一 `response.Fail`，不要自己写 `c.JSON(400, ...)`。
- **响应体只有一种形状**：`{code, message, data, request_id}`，成功用 `response.OK/Created`。
- **每个对外函数带 `ctx context.Context`** 并一路传到底：超时与取消靠它传导到驱动层。
- **软删除用 `model.DeletedAt`**，唯一索引把 `deleted_at` 排进去（`priority` 见 `model/user.go`）。
- **跨表写入用 `repository.TxManager`**，不要在 service 里手写 `db.Begin()`。

## 3. 新增一个模块的清单

按顺序做，每一步都能单独编译：

1. `model/xxx.go`：字段、索引、`TableName()`。
2. `bootstrap/app.go` 的 `models` 切片里登记（漏了会在首次写入时报 no such table）。
3. `repository/xxx.go`：接口 + 实现；通用 CRUD 用嵌入的 `Base[T]`，只写独有查询。
4. `dto/xxx.go`：`XxxItem`（响应）、`XxxCreateRequest`/`XxxUpdateRequest`（入参，带 binding 标签）、`XxxListQuery`（嵌入 `PageQuery`）。
5. `service/xxx.go`：接口 + `NewXxxService` + `XxxOption`；业务校验、事务、模型↔契约。
6. `handler/xxx.go`：`bindJSON`/`bindQuery`/`parseID` → 调 service → `response.*`。
7. `server/router.go`：需要登录的挂进 `secured` 组；管理端接口再叠 `middleware.RequireAdmin()`。
8. 前端：`types` → `api/xxx.ts` → `pages/xxx/` → `router/index.tsx` → `config/nav.ts`。
9. 测试：至少覆盖 service 的校验分支；仓储的特殊查询补集成测试。

## 4. 注释与命名

- 注释写**为什么**，不写**是什么**。`// 返回列表` 没有信息量；
  `// 计数与取数分别构建查询：复用同一个 *gorm.DB 会让 Count 的 statement 残留到 Find 上` 才是。
- 踩过的坑值得写进注释：它比文档更靠近代码，改代码的人一定会看到。
- 命名用业务语言（`Note`、`OwnerID`），不要用技术缩写（`Nt`、`oid`）。
- 表名 `sys_` / `biz_` 前缀，和现有保持一致。

## 5. 改动后必须自检

```bash
task fmt        # gofmt
task lint       # go vet + 前端 tsc
task test       # 后端单测
task build      # 确认产物能构建
```

改了路由、配置字段、环境变量名时，连带检查：

- `docs/architecture.md` 里的约定是否还成立；
- `README.md` 的命令与占位表是否还对；
- 前端 `types` 与后端 `dto` 是否仍然一致。

## 6. 不要做的事

- 不要把业务逻辑写进 `middleware`（它只做横切，不认识任何实体）。
- 不要在 handler 里直接调 repository（绕过 service 等于绕过业务校验）。
- 不要在 `pkg` 里加与项目相关的类型（那等于把业务下沉到叶子包）。
- 不要为了「以后可能用到」预留没有调用方的抽象；有第二个调用方时再抽。
- 不要新增第二套错误处理 / 响应格式 / 日志库；先用现有的。
