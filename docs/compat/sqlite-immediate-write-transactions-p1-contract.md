# SQLite 写事务加锁方式（BEGIN IMMEDIATE）P1 合同

日期：2026-11-04。分支 `claude/sqlite-immediate-write-transactions`，基于 origin/main `0f5e6b1`。

## 1. 背景与范围

BookInfo 追更真实接口合同（分支 `claude/bookinfo-follow-update-toggle`，`5afc0c0`，未合入）第 8 节记录：并发认证请求让
`PUT /api/books/:id` 返回 `500 {"error":"failed to update book"}`，服务端日志 `api/books.go:599 database is locked`；完整模式
smoke 18 次中 4 次在 “cover state persisted” 失败。该切片按约定不改轮询、不掩盖。

本切片只处理 SQLite 写事务如何取得写锁（`backend/db/db.go#Open` 的连接 DSN），并审计 `backend/api/`、`backend/services/`、
`backend/db/` 中全部显式事务。不改接口形状、状态码、表结构、会话语义、WAL、连接池和 busy_timeout。

## 2. 上游行为

上游 reader-dev（`fa22f27`）没有 SQLite 事务层：书架、用户等数据经 `saveStorage` 写 JSON 文件（例如
`BaseController.kt#saveUserSession` → `saveStorage("data", "users")`）。BookInfo 的追更、封面、书名/作者/简介/分组保存走
`BookController.kt#saveBook`（见分支 `claude/bookinfo-follow-update-toggle` 上 BookInfo 追更合同第 2 节），不会因为同一用户或
其他用户的另一个请求恰好同时写入而失败。

OpenReader 的 SQLite/GORM/WAL 属于 Go 运行时适配；它的加锁方式不能引入上游没有的保存失败。

## 3. 当前行为与根因（已逐项核实）

1. **事务是 deferred。** `db.Open` 用 `sqlite.Open(cfg.DatabasePath)`，DSN 没有 `_txlock`。mattn/go-sqlite3 v1.14.36
   `sqlite3.go` 默认 `txlock := "BEGIN"`；`SQLiteConn.BeginTx` 忽略 `driver.TxOptions`（包括 `ReadOnly`），直接执行 `c.txlock`。
   所以每个 `Transaction(...)` 和 GORM 单条写入的默认事务都以 deferred `BEGIN` 开始：第一条读语句取读快照，第一条写语句才申请写锁。
   连接池不限连接数。`PRAGMA busy_timeout = 5000` 和 `synchronous = NORMAL` 只经 `database.Exec` 作用到一个连接，但 mattn 每个连接的
   默认值相同（`busyTimeout := 5000`、`synchronousMode := "NORMAL"`），已有测试 `TestOpenAppliesWALAndBusyTimeoutToEveryPooledConnection`
   覆盖每个连接的 busy_timeout 与 WAL。
2. **读事务升级为写时不等待。** 随 mattn 打包的 SQLite 3.51.3（`sqlite3-binding.c`）：
   - `sqlite3BtreeBeginTrans` 只在 `pBt->inTransaction==TRANS_NONE` 时调用 busy handler 重试；已持有读事务的连接遇到别的写者时
     直接返回 `SQLITE_BUSY`；
   - `sqlite3WalBeginWriteTransaction` 在读事务开始后已有其他连接写入时返回 `SQLITE_BUSY_SNAPSHOT`。
   两者在 mattn 中的文字都是 `database is locked`。结论：先读后写的 deferred 事务，只要在“第一次读”与“第一次写”之间另一个连接
   持有写锁或已提交，就立即失败，5 秒 busy_timeout 不起作用。
3. **每个认证请求提交两次写入。**
   - `authsession.Service.renew`：处理前 `UPDATE user_sessions SET last_seen_at, expires_at`（`a0edce3`，2026-08-26）；
     `adoptLegacy` 同样写入。
   - `middleware.TrackActivity`：处理后 `UPDATE users SET last_active_at`（首个提交 `131d3f8`，2026-05-18 起就有）。
   - 对原诊断的修正：竞争窗口在 `a0edce3` 之前就存在（`TrackActivity`）；`a0edce3` 在请求开始处又加了一次提交，与同时发出的 `PUT`
     撞上的概率大幅上升。
4. **`updateBook` 先读后写。** `api/books.go#updateBook` 的事务：`First(&current)` → 封面时 `validateBookCustomCoverURL`（打开上传文件）
   → `Updates(updates)`（第 599 行）→ `setBookCategories` → 重读。

接口复现（2026-11-04，`0f5e6b1` 二进制，每次 `PUT` 同时发 4 个认证请求；脚本见第 9 节）：

| 场景 | 无并发 | 4 个并发 `GET /api/books/:id` | 4 个并发 `PUT /api/settings/search` |
| --- | --- | --- | --- |
| 封面 `{customCoverUrl}` | 0/150 失败 | 56/150 返回 500 | 92/150 返回 500 |
| 追更 `{canUpdate}` | 0/300 失败 | 60/300 返回 500 | 134/300 返回 500 |

失败立即返回（`PUT` p95 ≤ 6.7 ms），服务端每次都记 `api/books.go:599 database is locked`，条数等于 500 的次数。

## 4. 审计：62 个显式事务

方法：用 Go AST 扫描每个 `Transaction(func(tx *gorm.DB) error {...})` 回调，按源码顺序列出数据库调用，再逐个读辅助函数。
api 26 个、services 34 个、db 2 个。另有 `cmd/reparse-local-book` 用自己的 `sql.Open` 与 `Begin()`，不经 `db.Open`，不在范围内。

**先读后写（40 个，全部会在并发写入时立即 `database is locked`）：**

- 书架与书籍（api）：`books.go` `updateBook`、`deleteBook`（`captureBookCleanup` 先读）、`batchBooks`、`batchClearBookCache`、
  `refreshBook`、`refreshLocalBook`、`updateBookCategory`、`createRemoteBook`（已在书架）、`changeBookSource`、
  `persistRebuiltLocalChapterTextContext`；`cache.go#clearCache`；`categories.go#updateCategory`；`replace_rules.go#upsertReplaceRules`。
- 账户与恢复（api）：`auth.go#register`、`admin.go#deleteUserData`、`backup_restore_plan.go` 逻辑备份恢复、
  `webdav.go#restoreChapterVariablesFromData`。
- 服务：`authsession.Issue`（登录/注册签发）；`readingprogress.applyUploadedProgress`（阅读进度上传）；`booksources`
  `initializeNamespace`、`Update`、`Delete`、`BatchDelete`、`saveDefaultFromUser`、`RemoveUserNamespaces`、`batchUpdate`；
  `rss` 6 个写入（`CreateOrReplaceSource`、`UpdateSource`、`ImportSources`、`DeleteSource`、`UpsertArticlePage`、`CommitArticleContent`）；
  `scheduler.applyFetchedBook`（后台更新）；`sourcecandidates` `Available`、`Merge`、`Replace`；`localbook.persistParsedBook`（本地导入）；
  `backup` `run` 与 `portableLogicalEntries`（先读书源、RSS、设置、分类 4 张表，第 5 步 `bookgroups.List` → `ensureBuiltIns`
  执行 `INSERT … ON CONFLICT DO NOTHING`）。
- 启动：`db.migrateLegacyBookSourceOwnership`（启动时单线程，只有理论暴露）。

**先写（22 个，不受影响）：** `admin` 建用户与重置密码、书签增删、`createBook`、`createRemoteBook`（新书）、`persistRemoteChapterFetch`、
分类排序、替换规则删除、`db.backfillBookLastCheckTimes`、`authsession.adoptLegacy`、`bookgroups` 5 个（`ensureBuiltIns` 先写）、
`booksources` `Create`/`Import`（`lockSourceLimit` 先写）、`ClearActive`、`ReplaceActive`、`RestoreDefault`、`RestoreDefaultForUsers`。
事务外的单条写入（GORM 默认事务）也是先写，不受影响。

**其他审计结论：**

- 没有只读的显式事务。所有备份入口（HTTP 与 23:50 定时）都按用户导出，第 5 步就写；无用户过滤的 `RunNow` 没有生产调用方。
  `GET` 书籍分组也会进入 `bookgroups.List` 的写事务。
- 已有局部绕行，证实同一根因：`booksources.EnsureNamespace` 注释写明 “the loser receives SQLITE_BUSY immediately even with
  busy_timeout configured”，用互斥锁加重试；`lockSourceLimit` 用 `UPDATE users SET source_limit = source_limit` 先写；
  `rss` 每个写入都包在 `retryRSSSQLiteWrite` 里重试。
- 事务内没有网络请求。文件操作只有 `captureBookCleanup` 的 `os.Lstat`、`userWorkspaceCleanupPlans` 的路径计算、
  `updateBook` 打开封面文件，以及备份写 zip（`run` 写临时文件，`portableLogicalEntries` 写内存）。
- 回调里的句柄都来自 `tx`（`scoped := New(tx)`、`query := tx.Where(...)`、恢复用的 `Server{db: tx}`），静态扫描没有发现在回调里
  用外层 `s.db` 写入。回归时再用动态探测补证（第 8 节）。
- 事务内的测试钩子只取消 context、改文件，或通过 `tx` 写入（`batchBookCategoryWriteLifecycleTestHook`、
  `remoteBookExistingAddWriteLifecycleTestHook`、`readerLocalChapterCacheRebuildLifecycleTestHook`），不依赖 deferred 语义。

## 5. 方案比较（证据）

同一脚本，每个二进制各起一个全新服务；表中为返回 500 的次数：

| 构建 | 封面 + 4 GET | 封面 + 4 设置写 | 追更 + 4 GET | 追更 + 4 设置写 |
| --- | --- | --- | --- | --- |
| main `0f5e6b1`（deferred） | 56/150 | 92/150 | 60/300 | 134/300 |
| 实验：只把 renew 节流为每分钟一次 | 83/150 | 145/150 | 80/300 | 188/300 |
| 实验：renew 节流并去掉 `TrackActivity` 写入 | 0/150 | 129/150 | 0/300 | 154/300 |
| 实验：DSN 加 `_txlock=immediate` | 0/150 | 0/150 | 0/300 | 0/300 |

**A. `_txlock=immediate`（选定）。** 每个池连接的事务都以 `BEGIN IMMEDIATE` 开始：在没有任何事务时申请写锁，所以 busy handler
生效（最多等 5 秒），快照在拿到写锁之后才取，升级失败不再可能。

- 只读路径：自动提交的读（几乎所有 `GET` 的查询）不发 `BEGIN`，不受影响；WAL 下读者不等写锁（第 8 节对照测试）；审计没有只读显式事务，
  没有读路径被新串行化。
- 锁等待：并发写者等当前写事务结束，不再让它失败。实验中 4 路并发时 `PUT` p50 2.8–4.7 ms、p95 7.8–8.4 ms、最大 16.8 ms；
  并发请求 p95 6.8–10.2 ms、最大 18.1 ms（main 的 `PUT` p95 4.7–6.7 ms 含失败快速返回，并发请求 p95 5.5–7.3 ms）。
  先读后写的事务从 `BEGIN` 起持锁，多持的时间就是它的读阶段：处理器内的查询、`os.Lstat`、打开封面文件，都在毫秒级；
  备份导出多出第 1–4 步（单个用户的书源、RSS、设置、分类），原来从第 5 步到结束本来就持锁，最重的书架、章节变量、书签、进度都在那一段。
- 不改数据文件、不需要迁移；会话、接口、表结构不变。
- 一次例外数据：immediate 实验“追更 + 设置写”的无并发阶段出现一次 261 ms 最大值；该阶段没有任何并发请求，不是锁等待，回归时复测。

**B. 只在写路径上用 begin-immediate 辅助函数（不选）。** database/sql 没有逐事务的 BEGIN 模式，mattn 忽略 `TxOptions`。要么每个事务开头
先做一次无变化的写（`lockSourceLimit` 的做法），要么固定一个 `*sql.Conn` 手工 `BEGIN IMMEDIATE`，再自写 ConnPool 包装才能保住 GORM 的
嵌套 savepoint 与默认事务。两者都要改 40 处，以后新增的事务很容易漏掉；对已覆盖的事务，锁等待效果与 A 相同。

**C. 让 renew 不在每个请求提交（不选，记为可选后续）。** 实验表明：只节流 renew 时 `TrackActivity` 仍每请求写入，失败不减；
两者都去掉后，只读并发不再失败，但任何真实写者（另一台设备保存设置、上传进度、后台更新、其他用户）仍让 51%–86% 的 `PUT` 失败。
它还会改动 `a0edce3` 合同的“成功活动滑动续期七天”和 `last_active_at` 语义。它能省掉每个 `GET` 的两次提交（实验中 `GET` p50
从 2.8–3.3 ms 降到 1.8–2.2 ms），属于性能优化，不是本缺陷的修复。

**D. 按处重试 `SQLITE_BUSY`（不选）。** 这是掩盖，要逐处添加并重复执行业务。`rss` 与 `booksources` 的已有重试保留不动。

CLAUDE.md 禁止的禁用 WAL、改单连接不考虑。

## 6. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| S1 | 另一个连接在事务期间持有写锁或提交时，`PUT /api/books/:id`（封面、追更、元数据、分组）返回 500 | `must-fix` | 等写锁后成功保存 |
| S2 | 其余 39 个先读后写事务同样会立即失败（登录签发、进度上传、书源、RSS、分类、替换规则、备份与恢复、后台更新、本地导入等） | `must-fix` | 同上，由 A 统一覆盖 |
| S3 | 写事务从 `BEGIN` 起持有写锁，并发写者最多等 busy_timeout（5 秒），而不是先读后写的一方立即失败 | `acceptable-change`（Go 运行时适配） | 记录于此 |
| S4 | 每请求的 renew 与 `TrackActivity` 写入、已有重试/先写绕行、WAL、连接池、busy_timeout 与 PRAGMA 写法、smoke 轮询 | 不重开 | 保持 |

## 7. 目标行为

1. 经 `db.Open` 打开的数据库，每个池连接上的每个事务（显式 `Transaction` 与 GORM 默认写事务）都以 `BEGIN IMMEDIATE` 开始。
   配置路径里已有 `?` 参数时追加 `&_txlock=immediate`，否则追加 `?_txlock=immediate`。
2. 先读后写的事务不会因其他连接在期间持锁或提交而失败；它在 `BEGIN` 处等待，最多 busy_timeout。
3. 写事务未提交时，其他连接的读立即返回最后已提交的值，不等待。
4. 另一个认证请求续期会话时，`PUT /api/books/:id` 的 `{canUpdate}` 与 `{customCoverUrl}` 返回 200 并持久化；那次续期不会丢，
   在 `PUT` 提交后完成。

## 8. 计划测试

红测（在 `0f5e6b1` 上必须失败）：

- T1 `backend/db/write_transaction_lock_contract_test.go#TestOpenWriteTransactionsHoldTheWriteLockFromBegin`：固定池中两个连接，
  各自 `BeginTx` → 读 → 另一个 `_busy_timeout=0` 的连接试图提交 → 写 → 提交。期望事务提交成功；外部写在事务打开时得到 `SQLITE_BUSY`，
  事务结束后成功。旧实现：外部写提交成功，事务的写 `database is locked`。
- T2 同文件 `TestOpenGormTransactionSurvivesConcurrentCommitBetweenReadAndWrite`：同样的交错通过 `database.Transaction`（40 处所用的写法）。
- T3 `backend/api/book_patch_concurrent_commit_contract_test.go#TestBookPatchSurvivesConcurrentSessionRenew`（子测试 `follow`、`cover`）：
  在 `updateBook` 事务内读完书籍行后（GORM query 回调），用真实 `authsession.Service.Authenticate` 在另一个 `_busy_timeout=0` 的句柄上
  为同一会话续期。期望 `PUT` 200 且持久化；续期若需等待，则在 `PUT` 后补做并成功，`last_seen_at` 前进。旧实现：`PUT` 500
  `failed to update book`。

对照（旧实现与新实现都通过）：

- C1 `TestOpenWriteTransactionDoesNotBlockReaders`：写事务持有未提交修改时，池上自动提交读和 `_busy_timeout=0` 连接上的读都立即返回旧值。
- C2 同样的 `PUT` 没有并发提交：200。
- C3 并发续期在事务开始前提交（`bookPatchWriteLifecycleTestHook("metadata")`）：200。

回归：`go test ./...`、`go vet ./...`、`go test -race ./api ./db ./services/...`（api `-timeout 40m`）、
`cd frontend && npm test && npm run build`、两个压力脚本 0 失败（原脚本和第 9 节矩阵）、BookInfo 真实接口合同完整模式重复运行
（用 `claude/bookinfo-follow-update-toggle` 的 smoke，先 `npm run build`）。另做一次只在本地的动态探测（不提交）：记录每个 goroutine
持有的事务，在事务内用外层句柄写入或开新事务时报告，跑 api/db 测试，确认 0 处。

## 9. 证据与复现

- 压力脚本：`book-put-stress.mjs`（由 BookInfo 会话的两个脚本参数化而来：`BINARY`、`MODE=cover|follow`、`CONCURRENT_KIND=get|setting-put`，
  输出状态与 p50/p95/最大延迟），与事务审计扫描器 `txaudit` 一并放在本组会话的共享笔记目录。
- 实验构建只在临时目录打补丁，不进入本分支。

## 10. 明确不重开

- 会话续期频率与 `last_active_at` 写入（方案 C，可另起性能切片）。
- `lockSourceLimit`、`EnsureNamespace` 互斥与重试、`retryRSSSQLiteWrite`：现在多数情况下不再触发，清理另起。
- WAL、连接池、busy_timeout、`db.Open` 中 PRAGMA 的执行方式；`cmd/reparse-local-book`。
- BookInfo 真实接口合同的轮询方式。

## 11. 状态

**contract-only**。
