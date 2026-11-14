# 用户配置写入的请求取消合同（P2）

状态：**implementation-complete / regression-validated**（2026-11-14，见第 9 节）。

基于 main `9602dc4`，分支 `claude/settings-write-client-cancel`。固定上游
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 的 `saveUserConfig` 把配置写进文件，
客户端断开不会中止写入，也没有“请求取消”这一状态，所以本合同没有上游行为可对齐：全部差异归为
**运行时适配**（Go 请求 context 贯穿 GORM 后才出现的取消路径）和**日志加固**。

本合同只补 `PUT /api/settings/:key` 的取消路径和 GORM 语句日志。
[`user-setting-write-boundary-fixed-baseline-second-audit-p2-contract.md`](user-setting-write-boundary-fixed-baseline-second-audit-p2-contract.md)
签收的认证与 key 优先级、8 MiB wire 上限、单 JSON、CAS、`force`、Reader 字段清理、响应与
`settings_update` 规则全部保持，不在这里重开。

## 1. 现象与复现

- 2026-11-13 三次合并发布（run `43400193943`、`43407882444`、`43438877834`）的浏览器回归里，
  `scripts/smoke/registration-limits-contract.mjs` 注册成功后立刻关闭页面，首页首次加载发出的设置写入
  被取消。每次运行出现 1–4 条：
  - 访问日志把 `PUT /api/settings/shelf`、`PUT /api/settings/reader`、`PUT /api/settings/search` 记为 `|500|`；
  - GORM 日志 `settings.go:116 context canceled; sql: transaction has already been committed or rolled back`
    和 `settings.go:120 context canceled`。
- 复现：全新数据目录启动真实 Go 服务（`OPENREADER_INVITE_CODE=smoke-invite OPENREADER_USER_LIMIT=3`），再运行
  `TARGET_URL=... REGISTRATION_INVITE_CODE=smoke-invite node scripts/smoke/registration-limits-contract.mjs`。
  取消落点取决于时序，不是每次都出现。三次发布都带着它通过，属于原有行为。

## 2. 根因

`backend/api/settings.go#updateUserSetting` 用 `s.db.WithContext(c.Request.Context())` 做三步：读当前行（第 95 行）、
upsert（第 110–116 行，GORM 隐式事务，`db.Open` 的 `_txlock=immediate` 让它以 `BEGIN IMMEDIATE` 开始）、
读回（第 120 行）。三处都把非 `record not found` 的错误映射为 500 `{"error":"failed to save setting"}`。

| 取消落点 | 结果 | 持久化 | 事件 |
|---|---|---|---|
| W1 读当前行之前或之中 | database/sql 返回 `context canceled`，第 102 行 500；GORM 在第 95 行打印错误 | 无 | 无 |
| W2 `BEGIN IMMEDIATE` 返回前（含等写锁） | `BeginTx` 返回 `context canceled` 时 GORM 不生成 SQL、不打印，第 117 行仍 500；BEGIN 若已拿到锁则落入 W3 | 无 | 无 |
| W3 `BEGIN` 之后、`COMMIT` 之前 | INSERT 返回 `context canceled`；database/sql 的 `awaitDone` 已回滚，GORM 再回滚得到 `sql.ErrTxDone`，按 `"%v; %w"` 拼成观测到的文本，第 117 行 500，第 116 行打印 | 无（SQLite 回滚） | 无 |
| W4 `COMMIT` 返回之后 | 读回返回 `context canceled`，第 121 行 500，第 120 行打印 | **已持久化** | **缺失** |

- GORM 的 `"%v; %w"` 只把回滚错误留在错误链里：W3 的错误 `errors.Is(err, context.Canceled)` 为 false，
  只有 `errors.Is(err, sql.ErrTxDone)` 为 true。判断取消必须看语句自己的 `ctx.Err()`，不能只看错误链。
- `Create` 返回错误时 upsert 一定没有持久化：database/sql 的 `Tx.Commit` 先检查 context，已结束就不提交；
  mattn 的 `COMMIT` 失败会回滚。`Create` 返回 nil 时一定已持久化。
- W4 违反既有合同第 3.2 节“durable write 后仍广播一次当前用户 `settings_update`”：行已改变，
  同一用户的其它设备收不到事件。
- 首页首次加载并发发出 shelf/reader/search 三个写入；immediate 事务让它们排队等写锁，关闭页面时各自停在
  不同落点，所以一次运行能同时看到 W1–W4 的不同组合。客户端已断开，看不到响应；可见影响是访问日志的 500、
  GORM 错误行，以及 W4 丢失的事件。

## 3. 盘点：请求 context 错误映射为 5xx 的同类写法

方法：用 `go/ast` 扫描 `backend/api` 与 `backend/middleware` 的非测试文件，找出调用 `c.Request.Context()` 的
75 个函数里的 159 处 5xx 写入，检查同一路径上是否先有取消判断（`ctx.Err()`、`isRequestContextError`、
`requestCanceled`、`context.Canceled` 等）；66 处已有判断。其余 93 处逐一人工核对出错的调用是否绑定请求
context，结果如下。扫描脚本只在会话草稿区，不入库。

### 3.1 已有取消处理：不写业务响应（不重开）

`books.go`（PATCH、批量分组/删除/缓存、本地刷新、远程添加、换源、导出）、`categories.go`、`replace_rules.go`、
`backup.go`、`webdav.go` 的 `writeWebDAVServiceError`、`store_read_response.go`、`uploads.go`、
`source_debug.go` 的流式调试、`remote_reader.go`、`explore.go`、认证中间件 `middleware/auth.go`（`c.Abort()`）。
这些路径在请求 context 结束时直接返回、不写业务响应，未写过响应头的请求在访问日志里记为 Gin 默认的 200。
本合同的设置写入采用同一约定。

### 3.2 合同固定的取消 500（不重开）

导入流程按各自合同在取消时返回固定 500，且明确“不新增 499”：
`local book import canceled`（`imports.go`、`localstore.go`、`webdav.go`，见
[`local-book-import-durable-commit-lifecycle-fixed-baseline-second-audit-p2-contract.md`](local-book-import-durable-commit-lifecycle-fixed-baseline-second-audit-p2-contract.md)第 3 节）、
`import source read canceled`（`writeStorageImportLifecycleError`，见
[`storage-import-source-read-lifecycle-fixed-baseline-second-audit-p2-contract.md`](storage-import-source-read-lifecycle-fixed-baseline-second-audit-p2-contract.md)第 8 条）、
`local import stage canceled`。

### 3.3 同类：取消落入通用 5xx（本合同只改 C1，其余列为后续）

| # | 位置 | 路由 | 取消时的响应 |
|---|---|---|---|
| C1 | `settings.go:102/117/121` | `PUT /api/settings/:key` | 500 `failed to save setting`（本合同） |
| C2 | `auth.go:271 respondWithToken` | 登录、注册 | `sessions.Issue(ctx)` → 500 `failed to create token` |
| C3 | `auth.go:296` | `POST /api/auth/logout` | `sessions.Revoke(ctx)` → 500 `failed to revoke session` |
| C4 | `sync.go:25` | WebSocket 同步升级 | `sessions.Authenticate(ctx)` → 500 `failed to authenticate sync connection` |
| C5 | `admin.go:111/120/166/182/191/452/645/674` | 设为默认书源、重置用户书源、重置密码、删除用户、清理不活跃用户 | 500 `internalError` |
| C6 | `sources.go:616/633/682/691/1052` | 默认书源状态、保存、恢复；`requireSourceEdit`（新建/修改/删除书源的权限门） | 500 |
| C7 | `search.go:104` | 单书源搜索 | 远程请求绑定请求 context → 502 `failed to search source`（`recordSourceFailure` 已忽略取消，不写失败缓存） |
| C8 | `localstore_boundary.go:243`、`localstore.go:61/135/156/222/262/283` | LocalStore 根初始化、列表、上传建目录与保存、下载、新建目录 | `writeLocalStoreFilesystemError` 默认 500；`localstore.go:335` 改名取消时为 409。store-read 合同只要求“不返回成功数据” |

扫描命中但不属同类：出错调用不带请求 context（`s.bookSources.FindActive`/`ListActiveByIDs`、`s.rss.*`、
`createBook` 的 `s.db.Transaction`、章节读取等）、路径校验、bcrypt、zip 写入。WebDAV 认证中间件取消时按认证失败
处理（401 质询），不是 500，也不在本合同范围。

### 3.4 GORM 语句日志（全局）

`backend/db/db.go#databaseLogger` 用 GORM 默认 logger（Warn 级、`ParameterizedQueries`、彩色）。任何绑定到已结束
请求 context 的语句失败都会打印一条错误，与 handler 是否正确处理取消无关：3.1 的 handler 不写 500，但
GORM 仍会在日志里留下 `context canceled`。这一条只能在 logger 层修，修了对所有 handler 生效。

## 4. 差异分类

| 编号 | 差异 | 分类 |
|---|---|---|
| D1 | 设置写入在请求取消时返回并在访问日志记录 500（W1–W4） | **must-fix**（运行时适配） |
| D2 | 语句因自己的 context 结束而失败时，GORM 打印错误行 | **must-fix**（日志加固，全局） |
| D3 | W4：upsert 已持久化，却因读回失败不广播 `settings_update` | **must-fix**（违反既有合同 3.2） |
| D4 | 3.3 的 C2–C8 | **unknown / 后续切片**：本合同只记录，不修改 |
| D5 | 取消且未写响应的请求在访问日志里记为 200 | **acceptable-change**：与 3.1 现有 handler 一致，不新增 499 |

## 5. 目标行为

### 5.1 `PUT /api/settings/:key`

- S1：请求 context 在读当前行之前已结束：不查询、不写、不广播，handler 不写响应体（与 3.1 一致），不是 500。
- S2：context 在读当前行时或 upsert 提交前结束（W1–W3）：upsert 没有持久化，原有行或“无行”保持不变；
  不广播；不写响应体；不是 500。
- S3：context 在 upsert 提交后结束（W4）：写入已持久化，读回与事件不再依赖客户端。读回改用
  `context.WithoutCancel(ctx)`（保留 context 值，去掉取消；WAL 下单行唯一索引读不会等锁），之后照常广播一次
  `settings_update {key,updatedAt}`，并写正常的 200 `{key,value,updatedAt}`（连接已断开，响应被丢弃）。
- 非 context 的数据库失败保持 500 `{"error":"failed to save setting"}`，且不广播。
- 不改事务语义：upsert 仍是一条 GORM 语句和它的隐式事务（`BEGIN IMMEDIATE`），不加显式事务；
  不禁用 WAL、不改连接池或 busy_timeout；事件仍只在持久写入成功后发出。

### 5.2 GORM 语句日志

- 语句自己的 context 已结束，且错误是 `context.Canceled`、`context.DeadlineExceeded` 或 `sql.ErrTxDone`
  （第 2 节 W3 的拼接形式）时，不打印。调用方照常拿到错误。
- 其它错误（包括照旧打印的 `record not found`、SQLite 错误、context 仍有效时的 `sql.ErrTxDone`）、慢 SQL
  警告、Info 级输出、参数化 SQL（日志不含参数值）和彩色格式都不变。
- 日志里的 `文件:行号` 必须仍指向发起语句的代码（例如 `settings.go:116`）。GORM 的 `utils.FileWithLineNum`
  报告第一个不在 GORM 源码目录里的调用帧，所以 `Trace` 不能写成转调 GORM logger 的包装（那样每一行都会指向
  包装文件），而要在 `backend/db` 里自己实现、自己调用 `utils.FileWithLineNum`；`Info`/`Warn`/`Error`
  直接嵌入 GORM logger（提升方法不占调用帧），`ParamsFilter` 显式实现以保留参数化 SQL。

### 5.3 访问日志

取消且没有写响应的请求按 Gin 默认记为 200，与 3.1 一致；不新增 499，不改访问日志格式。

## 6. 计划测试（先红后绿）

`backend/api/user_setting_request_cancellation_contract_test.go`（新增）。每个用例用新服务器，并把服务器的
GORM logger 换成写入缓冲区的生产 logger，从而断言日志：

1. T1（S1/W1）：`Query().Before("gorm:query")` 在第一次读 `user_settings` 前取消。三个 key 各一次：不是 500、
   无响应体、原有行不变、无事件、日志里没有取消错误行。
2. T2（S2/W3）：`Create().Before("gorm:create")` 在 upsert 事务内取消。三个 key 各一次、分有行与无行：
   不是 500、无响应体、行不变或仍不存在、无事件、日志里没有 `context canceled` 或
   `transaction has already been committed or rolled back`。
3. T3（S3/W4）：`Create().After("gorm:commit_or_rollback_transaction")` 在提交后取消。三个 key 各一次：
   不是 500、行为新值、恰好一次 `settings_update` 且 `updatedAt` 与库中一致、响应为正常 200、日志无取消错误行。
4. 对照（改前改后都通过）：不取消时 200 并广播一次；`Create().Before("gorm:commit_or_rollback_transaction")`
   注入非 context 错误时仍为 500 `failed to save setting`、行不变、无事件，且日志**有**这条错误（证明缓冲区确实
   接到生产 logger）。GORM 会把 `After("gorm:create")` 回调排到提交之后，所以注入点放在提交前。

`backend/db/logger_contract_test.go`（新增）：

5. L1（红）：已取消 context 的查询、事务内 INSERT，以及直接传入的 `"context canceled; sql: transaction has already
   been committed or rolled back"` 形式，日志都为空。
6. L2（对照）：context 有效时同样的 `sql.ErrTxDone` 仍打印；关闭的数据库错误仍打印且不含参数值（既有用例）；
   `record not found`、慢 SQL 仍打印；错误行、慢 SQL 行与 Warn 行的位置指向测试文件 `logger_contract_test.go`；
   `LogMode(Silent)` 不打印。

测试需要从 `api` 包构造生产 logger，因此红测提交把 `databaseLogger` 改名导出为 `db.NewLogger`，不改行为。

## 7. 不重开

- 既有设置写入合同的全部条款；Reader/Index 前端设置流程；`GET /api/settings/:key`。
- 3.1 已处理的 handler、3.2 导入流程的固定 500、不新增 499 的既有结论。
- 3.3 的 C2–C8：只列入本合同，后续按各自切片处理。
- `_txlock=immediate`、WAL、连接池、busy_timeout、访问日志格式、认证写节流。

## 8. 回归门

`cd backend && go test ./... && go vet ./...`；`go test -race -timeout 40m ./api ./db ./services/...`
（端口测试需关闭沙箱）。前端未改动，跳过前端门与浏览器三视口。

## 9. 实施与回归记录（2026-11-14）

- 提交（分支 `claude/settings-write-client-cancel`，基于 main `9602dc4`）：合同 `b2d4a9e`、红测 `42a08c9`、实现 `65f85c5`。
- 红测在 `9602dc4` 的实现上：API 12 个子测试失败，2 个对照通过。
  - T1（三个 key）：500 `failed to save setting`，日志 `settings.go:95 context canceled`。
  - T2（三个 key × 有/无行）：500。日志有两种：`settings.go:116 context canceled`（4 例），以及与发布回归逐字相同的
    `settings.go:116 context canceled; sql: transaction has already been committed or rolled back`（2 例）。行与事件本来就不变（SQLite 已回滚）。
  - T3（三个 key）：500，持久化后没有 `settings_update`，日志 `settings.go:120 context canceled`。
  - db：L1 三个子测试失败（取消的查询、事务内取消的 INSERT、拼接形式与过期 deadline）。L2 四个对照和既有参数化日志测试通过。
- 实现：`settings.go` 在读当前行前检查 context。读或 upsert 失败且 context 已结束时直接返回，不写响应。提交后用
  `context.WithoutCancel(ctx)` 读回，广播一次，回 200。`backend/db/logger.go` 新增 `NewLogger`/`statementLogger`，
  `Trace` 自行调用 `utils.FileWithLineNum`。同一批断言全部转绿，既有设置写入边界、CAS、`force` 和八路并发初写合同继续通过。
- 编写测试时确认两点：首次写入时第一次读本来就打印 `settings.go:95 record not found`（`IgnoreRecordNotFoundError: false`，原有行为，本合同不改），
  日志断言放行这一行；注入失败的回调放在提交前，原因见第 6 节第 4 条。
- 回归（go1.26.11）：`go test ./...` 31 个包通过（另 8 个包无测试，api 122.3 s）；`go vet ./...` 通过；
  `go test -race -timeout 40m ./api ./db ./services/...` 26 个包通过（另 5 个包无测试，api 1108.1 s、db 15.9 s），无数据竞争。前端未改动，跳过前端门。
- 真实服务（本机，非 CI）：用同一份前端构建（SW `e5c767f8429a3a17`，64 个预缓存文件），每次全新数据目录、
  `OPENREADER_INVITE_CODE=smoke-invite OPENREADER_USER_LIMIT=3`，交替运行 `registration-limits-contract.mjs`。
  - `9602dc4`：12 次中 7 次出现设置写入 500，共 8 条（shelf 4、search 4），伴随 8 条 `settings.go:116` 取消日志（6 条拼接形式、2 条 `context canceled`）。
  - 本分支：16 次全部通过，134 个设置 PUT 全为 200，没有 5xx，也没有取消日志。
  - 本地没有碰到 W4（`settings.go:120`）的时序，由 T3 覆盖。
- 状态 implementation-complete / regression-validated。未合入 main，镜像未发布，生产未访问。
