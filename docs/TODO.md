# 待办清单

本文件列的是**还没做**的事：代码整改与文档整改混排，按「先修什么」排序。做完就把那一行删掉，
不要留已完成项当装饰——清单一旦掺了已完成项，就没人再信它。

写一条的规矩：

- 一条一项、能独立提交；不要写「优化后端」这种没法验收的条目。
- 位置写成 `文件:行`。行号会随改动漂移，它只是「去哪儿找」的线索，不是断言。
- 每条都带验收方式。写不出来的，说明这件事还没想清楚，先别进清单。

## 0. 优先级口径

| 级别 | 含义 | 期望节奏 |
|---|---|---|
| P0 | 会让对外契约失真，或把风险藏起来（错了没人发现） | 下一个提交就修 |
| P1 | 明确的规范/一致性问题，或关键路径没有测试 | 本迭代内清掉 |
| P2 | 清理类：死代码、注释漂移、性能与体验 | 顺手做，不占排期 |

## 1. P0

### 后端代码

| # | 位置 | 要做的事 | 验收 |
|---|---|---|---|
| 1 | `internal/pkg/response/response.go:85`、`internal/service/auth.go:131,154,176,182,246,260` | 5xx 的 `message` 统一折叠成「服务器内部错误」：`Fail` 遇到 `Code/100 >= 5` 一律换文案，原始 `err` 只进日志（现文案「查询账号失败」等会直接回给客户端） | 新增用例断言 500 的 `message` 恒为统一文案，且日志里仍有原始错误 |
| 2 | `internal/repository/base.go:83,97`、`internal/server/access_test.go:88` | 补「建 → 改成功 → 带旧 `updated_at` 再改 → 409」的 e2e；PUT 目前只测过越权 404，乐观锁整条路径从未执行 | `UpdateFields` 与 `takeOptimisticLock` 覆盖率 > 0；成功与冲突各一条用例 |
| 3 | `internal/bootstrap/app.go:75`、`internal/repository/tx.go:54` | 决定 `TxManager` 的去留：要么真接进某个多表写入并加测试，要么删掉并把 `repository/tx.go` 的用法注释一并删。不允许 `_ = 构造` 挂着 | 仓库里不再出现「构造了却丢弃」；若保留，`Transaction` 有调用方与用例 |

### 文档

| # | 位置 | 要做的事 | 验收 |
|---|---|---|---|
| 4 | `docs/architecture.md:88-89` | 删掉「路由注册处的 `ok` 装饰器包成 `Envelope[T]`」：装饰器与泛型信封已在 `052d590` 删除，现在由序列化钩子组装 | 全文 grep `ok` 装饰器 / `Envelope` 无命中 |
| 5 | `docs/architecture.md:92-93`、`docs/contracts/api.md:65` | 与代码 P0-1 同步：这两处承诺「非业务错误一律 500、真实原因只进日志」，代码目前会把业务化 5xx 的自定义文案透出 | 改动与代码 P0-1 同一个提交；三处口径一致 |
| 6 | `README.md:47`、`README.md:76-80` | 删掉「首次会生成 `pnpm-lock.yaml`」与「把锁文件提交上去」两段：锁文件已在仓库里，照着做的人会以为要重新生成 | README 里不再出现「提交锁文件」的指示；`grep -n pnpm-lock.yaml README.md` 只剩安装相关的正常引用 |

## 2. P1

### 后端代码

| # | 位置 | 要做的事 | 验收 |
|---|---|---|---|
| 7 | `internal/handler/auth.go:39,54` | 补 `/api/v1/auth/profile` 与 `/api/v1/auth/logout` 的 e2e（成功 + 未带令牌 401 + 登出后令牌失效） | 三个断言各一条；`handler.Auth.Logout/Profile` 与 `service.Logout` 覆盖率 > 0 |
| 8 | `internal/infra/cache/snapshot.go:35,84`、`memory.go:171` | 给内存缓存快照加单测：损坏文件备份、版本不匹配、过期键跳过、原子写（`.tmp` 不残留）。`persist: true` 是开发默认值，这段代码默认就在跑 | 四条分支各有用例；`loadSnapshot`/`writeSnapshot` 覆盖率明显上升 |
| 9 | `internal/pkg/errs/errs.go` | 给码表加单测：每个 `Code` 的 `HTTPStatus()`、`FromStatus` 的双向映射、未知状态码回落 500 | 码表每一行都有断言；新增码时用例先红 |
| 10 | `internal/server/middleware/cors.go:7`、`internal/config/config.go:294` | 生产环境白名单为空时拒绝启动（或要求显式 `allow_all`）。现在是静默 `Access-Control-Allow-Origin: *` | `app.env=prod` + 空 `cors_origins` 启动失败，有用例 |
| 11 | `internal/handler/handler.go:132`、`middleware/requestid.go:39`、`middleware/accesslog.go:73` | 补 handler / 中间件辅助函数的单测：`validationMessage` 的 email/min/gte/default 分支、`validRequestID` 拒绝分支、`clientIP` 的 X-Real-IP 与 RemoteAddr 分支 | 三个函数覆盖率 ≥ 80% |
| 12 | `internal/infra/database/database.go:149,181,223` | 补单测：`postgresDSN` 的引号/空格转义、`sqliteFilePath` 的两种 DSN、`logLevel` 与 `slogLevel` 的映射 | 三个函数各有用例；数据库相关改动不再只靠集成测试 |

### 文档

| # | 位置 | 要做的事 | 验收 |
|---|---|---|---|
| 13 | `README.md:391` | 环境变量表在 `apps/server/AGENTS.md` 第 6 节（配置与密钥），不是第 5 节（数据与存储） | 引用改为第 6 节，并核对同段其余节号 |
| 14 | `README.md:341-342` vs `README.md:186` vs `apps/web/AGENTS.md:17` | 三处对 `fmt` 的说法打架：替换清单说 `fmt` 参与「`task lint` 的格式化步骤」，命令表说 lint「不顺手做」格式化，前端规范说本端没有 `fmt` 任务 | 统一成「lint 只检查、fmt 单独跑」，三处措辞一致 |
| 15 | `docs/contracts/api.md:56`、`apps/server/AGENTS.md:60`、`apps/web/AGENTS.md:69` | 三处码表都没有 413，而 `internal/server/router.go:330` 已按 413 写了文案。二选一：补 `413`（`40001`）行，或说明「413 复用 40001、状态码由中间件直接给出」 | 三处码表行数仍相等（`grep -c '^| \`[0-9]'`）；413 的说明与代码一致 |
| 16 | `apps/web/AGENTS.md:53` | 「不带令牌的接口只有登录」不准确：`/healthz` 同样不需要令牌，而且它没出现在任何文档里 | 补一句健康检查接口（路径、无鉴权、不进 `/api/v1`），或明确它不属于对外契约 |
| 17 | `AGENTS.md:22-28`（落点表） | 文档分类里没有「工作清单」这一类，新增 `docs/TODO.md` 后分类不完整 | 已在本轮登记（本文档类型 + 三处地图），后续新增同类文件按同一行登记 |

## 3. P2

### 后端代码

| # | 位置 | 要做的事 | 验收 |
|---|---|---|---|
| 18 | `internal/infra/cache/redis.go:84` | `Incr` 不再按错误文案判断「非整数」，改用 `errors.As`/错误前缀等结构化判据 | 换 go-redis 版本后仍能命中 `ErrNotInteger`，集成测试可验证 |
| 19 | `internal/infra/cache/memory.go:128,171,225` | 收窄锁粒度：`sweep`/`flush` 改为分批或写时复制，`Incr` 避免与其他键争同一把锁 | 有并发基准或至少一条多键并发用例，读写不被全表扫描长时间阻塞 |
| 20 | `internal/config/config.go:220,233,242` | `max_lifetime`/`dial_timeout`/`save_interval` 增加正数校验：负值会让 `time.NewTicker` 直接 panic | 负值启动即报「必须大于 0」，与其它校验同一风格 |
| 21 | `middleware/auth.go:60`、`server/hooks.go:20`；`router.go:304,339`；`dto/auth.go:14`、`dto/user.go:4` | 收敛重复：`RequestInfo` 只拼一次；失败 schema 不再「先生成再覆盖」；`UserInfo`/`UserItem` 合并或由同一映射函数产出 | 每种语义只有一个实现点，改动面一眼可数 |
| 22 | `internal/repository/base.go:142` | 软删除标记冲突的 `time.Sleep(2ms)` 从请求路径里挪走（改重试策略或直接返回可判定的错误） | 仓储方法不再含 `Sleep` |
| 23 | `internal/service/auth.go:44` | `dummyPasswordHash` 生成失败时要记日志并降级（例如阻塞启动），不要静默返回 nil 让时序对齐失效 | 生成失败有可见信号，用例可注入失败 |

### 文档

| # | 位置 | 要做的事 | 验收 |
|---|---|---|---|
| 24 | `README.md:73` vs `apps/server/AGENTS.md:259-260` | 「推倒重来」的清理口径不一致：README 只删 `data/app.db*`，后端规范还要求删 `data/cache.snapshot` | 两处命令一致；保留快照会留下旧登录态与限流计数，要写明 |
| 25 | `docs/architecture.md:100-107` | 该节声明「分层与目录职责只写在各自正本里，本文件不复述」，但 §4 步骤 5 复述了实现细节并已漂移（见 P0-4）。要么删掉步骤 5 的细节，要么把它降低为「回程由统一序列化器写出」一句话 | §4 不再出现随代码变的具体类型名 |
| 26 | `README.md:255` | `-version` 示例输出写 `go1.27.3`，前置表写 Go ≥ 1.26，读者会以为版本要求不一致 | 示例与前置表对齐，或标注「示例输出」 |
| 27 | `apps/server/AGENTS.md`（配置表）、`docs/contracts/api.md:22-29`（传输层表） | 请求体上限（8MB）与单请求超时（30s）是 `router.go:40,44` 的常量，既不可配也没登记；413 与 504 的行为都依赖它们 | 在配置表或契约里写明这两个值，或改成配置项 |

## 4. 工具与流程

| # | 位置 | 要做的事 | 验收 |
|---|---|---|---|
| 28 | `apps/server/Taskfile.yml:160`、`.github/workflows/ci.yml:55` | 测试加 `-race`；内存缓存与中间件都有并发路径，现在只跑串行单测 | 本地与 CI 都跑 `go test -race ./...`，不新增失败 |
| 29 | `apps/server/Taskfile.yml:165` | `lint` 只有 `go vet`，查不出未使用的包级函数——上一批死代码就是它漏掉的 | 接入 staticcheck 或 golangci-lint，把 `U1000` 级别的问题纳管 |
| 30 | `AGENTS.md:78-92`（第 3 节自检）、`docs/contracts/api.md:142-143` | 两条文档自检（码表行数、`apps/` 内不得外链）目前靠人手跑，改成 CI 的一步 | CI 里直接执行那两条 grep，失败即红 |

## 5. 已完成

- [x] 成功信封改由 Fuego 序列化钩子组装，删除 `internal/server/decorate.go` 的 `ok` 装饰器与 `response.Envelope` / `response.OK`（`052d590`）
- [x] 补 `TestOpenAPISuccessEnvelope`，守住「2xx 的 schema 也带信封」（`052d590`）
- [x] 文档分类与三处地图登记 `docs/TODO.md` 这一类文件
