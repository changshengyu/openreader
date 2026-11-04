# 退役 SQLite BUSY 局部绕行 P2 合同

日期：2026-11-04。分支 `claude/retire-sqlite-busy-workarounds`，从 `claude/sqlite-immediate-write-transactions`（`5bfb0b8`，基于 main
`0f5e6b1`；`db.Open` 的 DSN 加 `_txlock=immediate`）切出：用户 2026-11-04 暂不合入 P1，本分支依赖它的 immediate 语义。
合入顺序：先合入 `claude/sqlite-immediate-write-transactions`，再合入本分支；本分支包含 P1 的全部提交，单独合入会把 P1 一并带入 main。

## 1. 背景与范围

[`sqlite-immediate-write-transactions-p1-contract.md`](sqlite-immediate-write-transactions-p1-contract.md) 第 4 节把下面三处局部绕行
列为同一根因（deferred 事务先读后写，升级写锁时立即 `database is locked`）的证据，第 10 节把清理留给单独切片。本切片只处理这三处：

- W1 `backend/services/rss/service.go#retryRSSSQLiteWrite` 及其 6 个调用点；
- W2 `backend/services/booksources/service.go#EnsureNamespace` 的进程级 `sourceNamespaceInitMu` 互斥锁与重试循环；
- W3 同文件 `lockSourceLimit` 中无值变更的 `UPDATE users SET source_limit = source_limit`（保留 `source_limit` 读取）。

不改接口形状、状态码、表结构、DSN、WAL、连接池、busy_timeout；不动 rss 与 booksources 的按用户写锁 `lockSourceUserWrite`。

## 2. 上游行为

上游 reader-dev（`fa22f27`）没有 SQLite：书源写 `saveStorage` JSON 文件，RSS 源同样按用户存文件，首次访问时从默认书源文件复制。
并发请求不会因为另一个请求同时写入而失败，也没有“重试 50 秒”或“等待另一个首次访问”这类行为。SQLite 锁等待属于 Go 运行时适配
（P1 合同第 2 节），这里不引入上游没有的失败或长时间挂起。

## 3. 当前行为（逐项核实）

| 编号 | 位置与机制 | 引入提交与当时的原因 |
| --- | --- | --- |
| W1 | `CreateOrReplaceSource`、`UpdateSource`、`ImportSources`、`DeleteSource`、`UpsertArticlePage`、`CommitArticleContent` 的整笔事务包在 `retryRSSSQLiteWrite` 里：遇到 `SQLITE_BUSY/LOCKED` 最多执行 10 次，每次后退 `(n+1)×5 ms`，每次重新清空结果变量。 | `5236389`（2026-08-12，RSS 写边界；[`rss-write-boundary-fixed-baseline-second-audit-p2-contract.md`](rss-write-boundary-fixed-baseline-second-audit-p2-contract.md) 第 9 节“对整笔 RSS 写事务做有界重试”）。 |
| W2 | `EnsureNamespace`：命名空间缺失时取进程级 `sourceNamespaceInitMu`，锁内复查，再最多 10 次 `initializeNamespace`，每次失败后退 `(n+1)×10 ms` 并复查。 | `f9723ad`（互斥）与 `9a13d8e`（重试），2026-07-28：首次登录时 `GET /api/explore/sources` 与设置保存并发，读事务升级写锁 `database is locked`（差异总账 2026-07-28 条目）。 |
| W3 | `Create` 与 `Import` 的事务第一条语句是 `UPDATE users SET source_limit = source_limit`，先取得写锁再读 `source_limit` 与 active 关联数；`RowsAffected == 0` 返回 `gorm.ErrRecordNotFound`。 | `d9ddc0f`（2026-08-12，书源写边界；[`book-source-write-boundary-fixed-baseline-second-audit-p2-contract.md`](book-source-write-boundary-fixed-baseline-second-audit-p2-contract.md) 第 9 节“以 user row 无值变更写入先取得 SQLite writer lock”）。 |

## 4. immediate 事务下的分析

`db.Open` 打开的每个池连接都以 `BEGIN IMMEDIATE` 开始事务：在没有任何事务时申请写锁，busy handler 生效（最多 5 秒），快照在拿到
写锁之后才取，读后升级失败不再可能。逐项：

1. **W1 与 W2 的重试**只在 `BEGIN` 等写锁超过 busy_timeout 时才会触发，每次重试又在 `BEGIN` 等满 5 秒：最长约
   `10 × 5 s + 退避`（W1 约 50.3 s，W2 约 50.5 s）。5 秒内的等待它们不起作用（第 8 节守护测试在没有它们时全部通过）。
2. **W2 的互斥锁**原本让同进程两个首次访问不同时升级读事务；immediate 下写锁本身已经串行化它们。互斥锁内的复查还有一个语义作用：
   输掉竞争的请求不会再次复制默认书源。去掉互斥锁时必须把复查移到事务里（写锁之下），否则输家会把用户已删除的默认书源复制回来
   （第 9 节变异 M2 实测）。
3. **W2 的互斥锁在 immediate 下会互等（新发现）。** `RestoreDefaultForUsers`（`POST /api/admin/users/sources/reset`）在自己的事务里
   对每个目标调用 `New(tx).EnsureNamespace`。该事务从 `BEGIN` 起持有写锁；目标未初始化时它去取 `sourceNamespaceInitMu`。如果此时
   另一个新用户的首次访问已经拿到互斥锁、正在 `initializeNamespace` 的 `BEGIN IMMEDIATE` 等写锁，两者互等：首次访问每次等满 5 秒后
   重试，共 10 次，始终持有互斥锁；重置事务等互斥锁，始终持有写锁；这段时间里进程内其余写入（包括每个认证请求的会话续期）每次等满
   5 秒后失败。实测（第 9 节 T1）：重置 53.8 s 后才完成，首次访问返回 `database is locked`（HTTP 层为 500）。deferred 时重置事务在
   那一刻只持有读快照，所以这种互等是 P1 的 immediate 改动合入后才出现的。
4. **W3** 在 immediate 下多余：`BEGIN` 已持有写锁，事务内读到的 `source_limit` 与 active 关联数在提交前不会被其他连接改变。
   `RowsAffected == 0 → ErrRecordNotFound` 的作用由随后的 `First(&user, userID)` 保留（同一个错误）。
5. **等待超过 busy_timeout。** 退役 W1/W2 后，RSS 六个写入与命名空间初始化在写锁被占用超过 5 秒时与其他写入一样 5 秒后失败，
   不再最多等约 50 秒。实测（另一连接 `BEGIN IMMEDIATE` 持锁 6 s）：有绕行时 RSS 写入与命名空间初始化在 6.0 s 成功，普通写入
   5.3 s 失败；无绕行时三者都在 5.3 s 失败。判断为可接受：
   - P1 合同 S3 已把“并发写者最多等 busy_timeout”定为 acceptable-change；P1 审计的 62 个显式事务中，除这 7 个（RSS 6 个、
     `initializeNamespace` 1 个）外的 55 个和全部单条写入都是这个上限，特例不构成任何合同。
   - 目前每个认证请求在处理器之前提交会话续期（`authsession.renew`），它等写锁超过 5 秒就让请求返回
     `500 {"error":"failed to authenticate"}`。长时间持锁期间到达的请求在到达这些写入之前就已失败；重试只帮到续期恰好在持锁开始前完成的请求。
   - 重试把这类请求变成最长约 50 秒的挂起（`http.Server` 只设了 `ReadHeaderTimeout`，没有 `WriteTimeout`，也没有处理器超时中间件）。
   - 正常运行中没有请求接近这个时长（P1 压力测试中请求最大延迟 17.7 ms）。可能较长的是单用户备份导出与恢复；它们持锁多久与这三处
     有没有重试无关，如果某天超过 5 秒，应在持锁一方解决，而不是给几个写入加特例。

## 5. 方案

- W1：6 个调用点直接调用 `s.db.Transaction`，去掉每次重试前的结果清空；删除 `retryRSSSQLiteWrite`、`isRSSSQLiteWriteContention`、
  两个重试常量和 `sqlite3`、`time` 导入。
- W2：删除 `sourceNamespaceInitMu`、重试循环、两个重试常量和 `isSQLiteWriteContention`。`EnsureNamespace` 只做事务外的快速检查，
  `initializeNamespace` 在事务里先用 `New(tx).namespaceInitialized` 复查：immediate 下复查与复制在同一把写锁下完成；
  `RestoreDefaultForUsers` 这类嵌套调用作为保存点运行在已持锁的外层事务里。
- W3：删除无值变更写入，函数改名 `sourceLimit`（它不再加锁，锁由事务本身持有），保留读取与 `ErrRecordNotFound`。
- 同步改写 `docs/compat/api-contract.md` 中描述命名空间互斥与重试的段落。

## 6. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| R1 | 管理员重置用户书源与另一新用户首次访问同时发生时互等：重置 53.8 s 后完成，首次访问 500，期间其余写入每次等满 5 秒失败 | `must-fix` | 两者都在写锁正常等待内完成（毫秒级） |
| R2 | W1、W2、W3 在 immediate 下对 5 秒内的并发不起作用，W2 的互斥锁还造成 R1 | `acceptable-change`（内部清理，5 秒内行为不变） | 删除；W2 的复查移入事务 |
| R3 | 写锁被占用超过 5 秒时，RSS 六个写入与命名空间初始化从最多约 50 s 重试改为与其他写入一样 5 s 后失败 | `acceptable-change`（与 P1 S3 一致，见第 4 节第 5 点） | 记录于此 |
| R4 | 按用户写锁 `lockSourceUserWrite`、DSN、WAL、连接池、busy_timeout、会话续期频率 | 不重开 | 保持 |

## 7. 目标行为

1. 同一新用户的并发首次访问（含不同用户同时首次访问）全部成功，每个用户恰好一行命名空间、一份默认书源副本。
2. 输掉首次访问竞争的请求不会复制用户在其间删除的默认书源。
3. 管理员重置用户书源与另一新用户首次访问同时发生时，两者都成功，不等满 busy_timeout。
4. 书源新建与导入在写入前读到的 `source_limit` 在提交前不会被其他连接改变；之后降低的限额对下一次写入生效；各用户并发写入都不超限。
5. RSS 六个写入在另一用户的写入试图于其读与写之间提交时成功，后者等它提交后再成功；多用户并发 RSS 写入全部成功，同用户同 URL 收敛为一行。

## 8. 计划测试

守护测试（没有 W1/W2/W3 时在 immediate 下必须通过；它们也必须在 deferred 且没有绕行时失败，证明测到了当初的竞争）：

- `backend/services/booksources/immediate_write_contract_test.go`
  - G1 `TestEnsureNamespaceConcurrentFirstAccessCopiesDefaultsOnce`（W2，并发命名空间初始化）：2 个用户各 4 个并发 `ListActive`，
    前两个读默认书源的事务在读处会合（最多等 200 ms）；每个用户 1 行命名空间、3 个默认书源。
  - G2 `TestEnsureNamespaceDoesNotRecopyDefaultsAfterAnotherFirstAccessWon`（W2 锁内复查的语义）：输家在“发现未初始化”后暂停，赢家完成
    初始化，用户删除一个默认书源，输家继续；不得复制回来。
  - G3 `TestSourceLimitWritesHoldTheLimitAgainstAConcurrentCommit`（W3，`create`/`import`）：`_busy_timeout=0` 的连接在限额读取与插入之间
    把限额降到 1；它必须得到 `SQLITE_BUSY`，写入成功；之后降额生效，下一次新建 `ErrSourceLimitExceeded`。
  - G4 `TestConcurrentSourceWritesStayWithinEachUsersLimit`（W3，并发导入对限额）：3 个用户（限额 2/3/4）各 6 个并发新建或导入，前两个
    读限额的事务会合；每个用户恰好接受“限额”个，其余 `ErrSourceLimitExceeded`，没有其他错误。
- `backend/services/booksources/service_contract_test.go`
  - G5 `TestEnsureNamespaceHoldsTheWriteLockAgainstAnUnrelatedWriter`（W2 的重试；改写原 `TestEnsureNamespaceRetriesReadUpgradeBlockedByConcurrentWriter`）：
    无关写入在命名空间事务读与写之间提交，得到 `SQLITE_BUSY`；`EnsureNamespace` 成功，无关写入随后成功，命名空间 1 行。原断言全部保留。
    原版本让回调最多等 2 s 等一个在 immediate 下根本开不了事务的写者，只靠超时通过。
- `backend/services/rss/immediate_write_contract_test.go`
  - G6 `TestRSSWritesHoldTheWriteLockAgainstAConcurrentCommit`（W1，每个调用点一个子测试：`create`、`update`、`import`、`delete`、
    `refresh-articles`、`article-content`）：另一用户的 RSS 写入经 `_busy_timeout=0` 连接在首次读与首次写之间提交，得到 `SQLITE_BUSY`；
    操作成功且结果正确；另一写入随后成功。
  - G7 `TestConcurrentRSSWritesFromSeveralUsersAllCommit`（W1，并发 RSS 写入）：4 个用户，每人两个并发同 URL `CreateOrReplaceSource`
    加一串六种写入，前两个读源的事务会合；全部成功，同 URL 一行。

红测（合入 P1 后的 main 上必须失败）：

- T1 `TestAdminSourceResetDoesNotStallBehindAnotherFirstAccess`（R1）：重置事务在目标命名空间检查处暂停，新用户首次访问开始并（若能）
  通过互斥锁内复查后，重置继续；要求两者 4 s 内都成功。

保留不改：`TestEnsureNamespaceSerializesConcurrentFirstAccess`、api 层 `TestBookSourceLimitSerializesConcurrentFinalSlot`。

回归：`go test ./...`、`go vet ./...`、`go test -race ./api ./db ./services/...`（api `-timeout 40m`），前端 `npm test` 与 `npm run build`，
公告门自测。

## 9. 证据与复现

- 变异只在本地工作树临时改动并已还原，不提交：M1 把 `db.go#sqliteDSN` 的 `_txlock=immediate` 改成 `_txlock=deferred` 并去掉绕行；
  M2 在实现上删除 `initializeNamespace` 的事务内复查。
- 长时间持锁测量：临时外部测试包，另一连接 `BEGIN IMMEDIATE` 持锁 6 s，同时调用 RSS `CreateOrReplaceSource`、`EnsureNamespace` 与
  普通 `UPDATE users`，分别在有绕行（`5bfb0b8`）与无绕行的服务代码上运行。

## 10. 明确不重开

- 按用户写锁 `lockSourceUserWrite`（rss 与 booksources）。immediate 下它们对同进程写入也多余；另有一处既有隐患：备份恢复在自己的事务里
  经 `ReplaceActive → Create` 取该用户的写锁，同一用户同时在另一请求里新建或导入书源时，后者持有用户锁在 `BEGIN` 等写锁，两者互等到
  后者 5 秒超时（deferred 时由 W3 的先写同样等 5 秒，不是本切片引入）。可另起切片一并处理。
- DSN、WAL、连接池、busy_timeout、会话续期频率（另一会话处理）、长事务（备份导出、恢复）的持锁时间。

## 11. 状态

**contract-only**（由第 12 节记录取代）。

## 12. 记录（2026-11-04）

提交：合同 `8a7dd1b`；测试 `5166e03`（守护 G1–G7、红测 T1，改写 G5）；实现 `ed855a8`（忽略缩进：rss +7/−57，booksources +16/−54）。

在 `5bfb0b8`（immediate，绕行仍在）上：两个包的其余 30 项结果（含子测试）通过；T1 失败（两次运行 53.8 s、53.6 s），原文：
`admin reset = <nil>, first access = database is locked after 53.599s, want both to succeed`，此前一行
`after 4s the admin reset (done false) and the first access (done false) were still waiting on each other`。
实现后 T1 0.42 s 通过，G1–G7 通过；两个包 `-count=3` 与 `-race -count=2` 通过。

变异（第 9 节，本地临时改动，已还原）：

- M1 deferred 且没有绕行：G1（4 次）、G3 `create`/`import`、G4（10 次，用户 2、3 只接受 2/3 与 3/4）、G5、G6 六个子测试、G7（7 次）以及原有
  `TestEnsureNamespaceSerializesConcurrentFirstAccess` 都以 `database is locked` 失败，例如 G6 `create failed once another user's write tried
  to commit between its read and its write: database is locked (concurrent commit error: <nil>)`。T1 也失败（重置事务的读快照过期）；G2 通过
  （它不涉及并发提交）。
- M2 immediate、没有绕行、删除事务内复查：G2 失败 `active sources after the losing first access = [1 2], want only 1; it copied back default 2
  that the user had removed`；其余通过。

长时间持锁（另一连接 `BEGIN IMMEDIATE` 持锁 6 s，三个调用同时开始）：

| 服务代码 | RSS `CreateOrReplaceSource` | `EnsureNamespace`（新用户） | 普通 `UPDATE users` |
| --- | --- | --- | --- |
| `5bfb0b8`（有绕行） | 6.01 s 成功 | 6.04 s 成功 | 5.29 s `database is locked` |
| `ed855a8`（无绕行） | 5.33 s `database is locked` | 5.34 s `database is locked` | 5.33 s `database is locked` |

回归：

- 后端：`go test ./...` 31 个包通过（api 122.5 s）；`go vet ./...` 通过，gofmt 无输出；`go test -race ./api ./db ./services/...` 26 个包通过、无数据竞争（api 1051.0 s，`-timeout 40m`）。
- 前端未改：`npm test` 870/870、`npm run build` 通过（`node_modules` 克隆自锁文件相同的工作树）。
- 公告门自测：114/114。

未跑：现场公告扫描（没有依赖变化，合入 main 时 Actions 运行）；Compose、存储 HTTP 合同、fresh/历史卷与可移植备份门（不涉及，Actions 在推送 main 时运行）；
浏览器 smoke（没有前端或接口形状变化）。

状态 **implementation-complete / regression-validated**。分支已推送（基于 `5bfb0b8`），须在 P1 之后合入；未合入 main，镜像未发布，
生产未访问。
