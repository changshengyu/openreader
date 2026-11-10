# 退役书源与 RSS 按用户写锁（备份恢复事务内互等）P2 合同

日期：2026-11-04 起，2026-11-10 定稿。分支 `claude/sqlite-restore-user-lock`，基于 origin/main `9076cfa`
（含 `1b50602` 发布的 P1 `BEGIN IMMEDIATE` 与退役 BUSY 绕行，以及 `6006659` 合入的会话续期节流 `b3ea517`）。

## 1. 背景与范围

[P1 合同](sqlite-immediate-write-transactions-p1-contract.md)第 13 节与[退役 BUSY 绕行合同](sqlite-busy-workaround-retirement-p2-contract.md)
第 10 节记录了一处未修复的“事务内取得 Go 锁”：备份恢复在自己的事务里经 `ReplaceActive → Create` 取同一用户的进程内写锁；
同一用户此时在另一请求里新建或导入书源，两者互等，直到后者 5 秒超时（“需另起切片”）。本切片处理两把按用户写锁：

- L1 `backend/services/booksources/service.go#lockSourceUserWrite`，`Create` 与 `Import` 两处取锁（`d9ddc0f`，2026-08-12）。
- L2 `backend/services/rss/service.go#lockSourceUserWrite`，`CreateOrReplaceSource`、`UpdateSource`、`ImportSources`、`DeleteSource`、
  `UpsertArticlePage`、`CommitArticleContent` 六处取锁（`5236389`，2026-08-12）。

不改接口形状、状态码、表结构、DSN、WAL、连接池、busy_timeout，不加迁移。

## 2. 上游行为

上游 reader-dev（`fa22f27`）没有 SQLite，也没有进程内写锁。`BookSourceController.kt#saveBookSource`/`saveBookSources` 读出用户的
`bookSource.json`，按 `bookSourceUrl` 合并后经 `saveUserStorage` 整体写回；`RssSourceController.kt#saveRssSource`/`saveRssSources` 同理。
WebDAV 恢复 `WebdavController.kt#restoreFromWebdav → BookController.kt#syncFromWebdav` 解压备份后用 `renameTo` 直接替换用户的
`bookSource.json`、`rssSources.json` 等文件。恢复与同时发生的书源保存互不等待、都不失败（文件层面后写者覆盖先写者）。

OpenReader 用 SQLite 事务把两者串行化并保留两者的结果，这是允许的事务化差异（不丢写）；锁等待属于 Go 运行时适配（P1 合同
第 2 节），不得引入上游没有的挂起或失败。

## 3. 当前行为（逐项核实）

1. **L1。** 进程级注册表按用户 ID 保存引用计数的互斥锁。`Create`、`Import` 先 `EnsureNamespace`，再取锁，再开 `s.db.Transaction`。
   引入时（[书源写边界合同](book-source-write-boundary-fixed-baseline-second-audit-p2-contract.md)第 8、9 节）的理由是 deferred 事务下
   同用户的并发新建/导入在读后升级写锁时失败，需要“窄的同用户写串行化”。
2. **L2。** 同样的注册表，六个 RSS 写入在 `s.db.Transaction` 之前取锁。[RSS 写边界合同](rss-write-boundary-fixed-baseline-second-audit-p2-contract.md)
   第 4、8 节要求“create/import/update/delete 必须按 user ID 使用窄 source mutation lock”，以便同用户并发同 URL 收敛为一行。
3. **事务内取 L1。** `ReplaceActive` 在 `s.db.Transaction` 里，对备份中每个找不到同身份书源的条目调用 `New(tx).Create(userID, source)`，
   于是在事务内取 L1。调用方：
   - 备份恢复事务 `api/backup_restore_plan.go#restoreLegadoBackupDataWithPermissions → restoreSourcesFromDataStrict`（用户有书源编辑权限时）。
     两个恢复入口 `POST /api/backup/restore-legado` 与 `POST /api/backup/restore-webdav` 都经 `restoreBackupFileWithPermissions` 到这里：
     旧版备份直接调用，可移植备份经 `restorePortableBackupFileWithPermissions` 调用；
   - `api/sources.go#ensureDefaultBookSourceNamespaceLocked → ReplaceActive(0, …)`（启动与默认书源恢复）。它取用户 0 的锁，而处理器只用
     已认证用户 ID（≥ 1）取锁，目前不会互等。
4. **L2 不在任何事务内取得。** 恢复的 RSS 步骤 `api/webdav.go#restoreRSSSourcesFromData` 直接用恢复事务的 `tx` 执行 `FirstOrCreate`，
   不经 rss 服务。
5. **这是既有问题，不是 P1 引入。** `d9ddc0f` 起，deferred 时恢复事务的第一条语句（`ReplaceActive` 的命名空间写入）已经取得写锁，
   而 `Create` 的事务以无值变更写入（退役 BUSY 绕行合同的 W3）开头，在 busy handler 里同样等满 5 秒；P1 与退役 BUSY 绕行都没有改变它。

## 4. 审计：事务内取得的 Go 锁

P1 审计漏掉了这一类（P1 合同第 13 节）。本切片在 `9076cfa` 上用两种方法完整检查（工具只在临时目录使用，不提交，见第 10 节）：

- **静态。** 用 golang.org/x/tools v0.52.0 对 `backend/...` 建 SSA 与 VTA 调用图，从全部 62 个 `(*gorm.DB).Transaction` 回调出发，
  只沿模块内函数以及交给库函数的模块闭包（`sort.Slice`、`once.Do`、`FindInBatches` 等）遍历，可达 361 个模块函数；
  锁位点为 `(*sync.Mutex).Lock/TryLock`、`(*sync.RWMutex).Lock/RLock` 与字段名为 `token`/`slot`/`generationGate`/`serial` 的通道门，
  分布在 54 个模块函数中。结果只有 L1 的 3 个位点（注册表锁、按用户锁、释放路径），路径都是 `ReplaceActive$1 → Create → lockSourceUserWrite`。
  非测试代码没有注册 gorm 回调或模型钩子（`dataDirectoryPlugin` 的 `Initialize` 为空），gorm 不会回调进模块代码。
- **动态。** 临时副本在 60 个 `Lock()`/通道门位点前插入检查，并包装连接池记录哪些 goroutine 持有未结束的事务；跑完整 `go test ./...`：
  18 条报告全部是 L1，来自 6 个测试、4 条路径：WebDAV 恢复处理器（1）、旧版恢复（3）、可移植恢复（1）、启动时
  `ensureDefaultBookSourceNamespace → ReplaceActive(0)`（1）。
  （`TestBookPatchSurvivesConcurrentSessionRenew` 用 `ConnPool.(*sql.Tx)` 判断事务，在包装连接池下失败，属于审计假象。）

跨写事务持有、但从不在事务内取得的锁：`registerMu`（注册）、`defaultSourcesMu`（默认书源快照与镜像文件）、`localCacheMu`/`remoteCacheMu`
（本地/远程章节缓存文件）、阅读器章节门、用户资产门、进度上传门、备份生成门，以及 L2。按第 5 节的规则它们是安全的；除 L2 外它们都保护
数据库之外的文件或进程内状态，不在本切片范围内。

## 5. 分析

**规则。** `db.Open` 让每个事务以 `BEGIN IMMEDIATE` 开始，从 `BEGIN` 起持有 SQLite 唯一的写锁。只要有代码在持有事务时取某把 Go 锁，
而这把锁的另一个持有者又在 `BEGIN` 等写锁，两者就互等，直到 `BEGIN` 一方 busy_timeout（5 秒）失败。跨事务持有的 Go 锁只有在任何事务
都不取它时才安全。

**R1 实测（第 10 节）。** 恢复在事务内暂停；同一用户的新建/导入取得 L1 后在 `BEGIN` 等待；恢复继续，卡在 L1 上。恢复继续 2 秒后两者
仍在互等；书源写入 5.19–5.20 s 后返回 `500 {"error":"failed to create source"}`（导入为 `failed to import sources`），恢复随后才返回 200。
这段时间进程内其余写入都排在恢复之后：恢复继续 52 ms 后开始的无关用户写入等了 5.32–5.38 s；更早开始等待的写入（设置保存、进度上传、
到期的会话续期、后台更新等）会先用完 busy_timeout 而失败。

**L1 多余。** `Create`、`Import` 的限额读取（`sourceLimit`）、活动关联计数（`ensureSourceCapacity`）、导入的身份查找与全部写入都在一个
immediate 事务里：提交前没有其他连接能写入，同用户的第二个新建/导入在 `BEGIN` 等待，之后看到第一个的提交。L1 当初要保证的限额与身份
（退役 BUSY 绕行合同第 4 节第 4 点）不依赖它。

**L2 多余。** 六个 RSS 写入各自在一个 immediate 事务里读取身份、冲突、最大序号或存活并写入；同用户同 URL 的并发写入被写锁串行化，
后者看到前者的行。RSS 写边界合同要求的串行化与事务内复验由事务本身提供；它还要求“不得让一个用户阻塞另一用户”，而 immediate 下
所有写事务本来就在 SQLite 写锁上串行（P1 S3 已接受），L2 对此没有影响。L2 目前不在事务内取得，但它是同一隐患：恢复的 RSS 步骤若改为
复用 `rss.New(tx)` 的写入，就会重现 R1。

**去掉锁后唯一的行为变化（R3）。** 同一用户的并发写入不再在 Go 锁上无限期排队，而是与其他写入一样在 `BEGIN` 最多等 busy_timeout。
实测（第 10 节）：书源导入单次 5,000 条约 0.12–0.14 s，同一用户 3 个并发满额导入有锁 444 ms、无锁 537 ms 全部完成。RSS 导入更慢，因为
`ImportSources` 对每个 URL 在该用户的行中逐行查找（`rss_sources` 只有 `user_id` 索引）：同一用户 3 个并发 5,000 条导入的工作量依次约
0.66 s、1.6 s、3.5 s，有锁与无锁都全部成功（5.8 s）；4 个并发时，有锁全部成功（10.8 s），无锁有 1 个在 5.1 s 后 `database is locked`。
判断为可接受：

- 与 P1 S3、退役 BUSY 绕行 R3 一致：并发写者最多等 busy_timeout，L1/L2 是仅有的例外，不构成任何合同。
- 到达这种量级时，单个 RSS 导入事务本身已持有写锁 3 秒以上，同时其他用户的写入与会话续期一样要等它；L2 只让同一用户排在 Go 锁上，
  对其他人没有保护。应在持锁时间一方解决（URL 查找索引，见第 11 节），而不是给一个用户保留不限时的排队。

## 6. 方案

**A. 删除 L1 与 L2（选定）。** 删除两个服务里的注册表类型、`lockSourceUserWrite` 与 8 处取锁；在 `Create`/`Import` 与 RSS 服务处注明：
写入由 immediate 事务串行化，这些事务可能运行在调用方的事务里（`ReplaceActive`），因此不能在其内外再加 Go 锁。L1 与 L2 分两个提交，
L2 可单独回退。

**B. 恢复在 `BEGIN` 前取 L1，`Create` 在事务内跳过（不选）。** 把服务内部的锁扩散到 api 恢复层；每个在事务内调用的新调用方（包括用户 0
的 `ensureDefaultBookSourceNamespaceLocked`）都要记得先取锁；锁本身仍不提供事务之外的保证。

**C. 给 Go 锁加超时或 TryLock（不选）。** 仍然停顿到超时并让一方失败，属于掩盖。

**D. 只删 L1、保留 L2（不选）。** L2 没有正确性作用，又留着同一隐患；两套规则更难维护。它唯一的效果是 R3 中的不限时排队。

不考虑禁用 WAL、改单连接或改 busy_timeout。

## 7. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| R1 | 备份恢复（全部入口）与同一用户新建/导入书源同时发生时互等：书源写入约 5.2 s 后 500，恢复约 5.2 s 后才完成，期间进程内其余写入等待或失败 | `must-fix` | 两者都在写锁的正常排队内完成（毫秒级），两者的结果都保留 |
| R2 | L1、L2 在 immediate 下不提供额外的正确性保证；L1 造成 R1，L2 是同一隐患 | `acceptable-change`（内部清理） | 删除；每个写入仍在一个 immediate 事务里读取、校验并写入 |
| R3 | 同一用户的并发书源/RSS 写入从在 Go 锁上不限时排队改为与其他写入一样在 `BEGIN` 最多等 busy_timeout（5 秒） | `acceptable-change`（与 P1 S3、退役 BUSY 绕行 R3 一致） | 记录于此，实测见第 10 节 |
| R4 | 第 4 节其余跨事务持有的锁、DSN、WAL、连接池、busy_timeout、会话续期频率、RSS URL 查找索引、长事务持锁时间 | 不重开 | 保持 |

## 8. 目标行为

1. 备份恢复事务（`restoreLegadoBackupDataWithPermissions`，全部入口）持有写锁期间不等待任何 Go 锁。同一用户此时新建或导入书源的请求在
   `BEGIN` 排队，恢复提交后成功；最终活动书源同时包含恢复的书源与新写入的书源。
2. 任何事务回调内都不取得 Go 锁：静态与动态审计均为 0。
3. 书源限额、导入身份与写时复制、RSS 同 URL 收敛、ID 更新冲突与不复活、刷新与正文提交的存活复验保持不变（由 immediate 事务提供）。
4. 同一用户的并发写入在 `BEGIN` 排队，等待上限与其他写入相同。

## 9. 计划测试

红测（`9076cfa` 上必须失败）：

- T1 `backend/api/backup_restore_source_write_lock_contract_test.go#TestBackupRestoreDoesNotStallBehindTheSameUsersSourceWrite`
  （子测试 `create`、`import`）：同一账户经真实路由 `POST /api/backup/restore-legado` 恢复一个只含新书源的备份，恢复在事务内第一次写入
  （`ReplaceActive` 的命名空间行）处暂停；随后经 `POST /api/sources`（JSON）或 `POST /api/sources/import`（文件）写入另一个书源，测试连接池
  报告它已到达 `BEGIN`（L1 在此之前取得）后让恢复继续。要求恢复继续后 2 s 内两者都完成，状态分别为 200 与 201/200，且最终活动书源恰为
  恢复的书源与新写入的书源（若写入先于恢复提交，恢复会移除它）。两次请求都用刚签发的会话：续期与活动写入每分钟至多一次（`b3ea517`），
  它们在处理器之前不写入，暂停后的第一个 `BEGIN` 就是书源写入自己的。

守护（实现前后都通过；改回 deferred 且没有 L1/L2 时应失败，证明它们测到了这两把锁当初针对的同用户竞争）：

- api `TestBookSourceLimitSerializesConcurrentFinalSlot`、`TestRSSSourceIdentitySerializesConcurrentCreateAndImport`（各含 create-create、
  create-import、import-import）；
- `services/booksources` G3 `TestSourceLimitWritesHoldTheLimitAgainstAConcurrentCommit`、G4 `TestConcurrentSourceWritesStayWithinEachUsersLimit`；
- `services/rss` G6 `TestRSSWritesHoldTheWriteLockAgainstAConcurrentCommit`、G7 `TestConcurrentRSSWritesFromSeveralUsersAllCommit`。

审计：实现后在同一提交上重跑静态与动态审计，都应为 0。

回归：`go test ./...`、`go vet ./...`、`go test -race ./db ./services/...` 与 `go test -race -timeout 40m ./api`，前端 `npm test` 与
`npm run build`，公告门自测。

## 10. 证据与复现

T1 在 `9076cfa` 上（实现前）：

- `create`：`2s after the restore resumed, the restore (done false) and the same user's source create (done false) were still waiting on each other`；
  `restore = 200 {…"sources":1}, same user's source create = 500 {"error":"failed to create source"} after 5.189s, want 200 and 201`。
- `import`：同样互等；`same user's source import = 500 {"error":"failed to import sources"} after 5.199s, want 200 and 200`。
- 此前在 `8c3cf79`（两个服务与恢复代码与 `9076cfa` 完全相同）上，直接调用服务的版本 6 次运行得到 5.203–5.228 s 后 `database is locked`。

无关写入（临时测试变体，不提交）：恢复继续后约 52 ms 开始的另一用户 `UPDATE users` 在 `9076cfa` 上等 5.317–5.381 s（4 次），在 `8c3cf79`
上等 5.204–5.227 s（4 次）。

同用户并发导入耗时（临时测试，`8c3cf79`；“无锁”为删除 8 处取锁的临时副本）：

| 场景 | 有 L1/L2 | 无 L1/L2 |
| --- | --- | --- |
| 书源，3 个并发 5,000 条导入 | 全部成功，126/264/444 ms（复跑 121/263/443 ms） | 全部成功，131/293/532 ms（复跑 122/287/537 ms） |
| RSS，3 个并发 5,000 条导入 | 全部成功，0.66/2.26/5.81 s（另两次 0.67/2.26/5.90、0.66/2.24/5.78 s） | 全部成功，0.67/2.33/5.75 s |
| RSS，4 个并发 5,000 条导入 | 全部成功，0.67/2.29/5.77/10.77 s | 3 个成功（0.67/2.35/5.83 s），1 个 5.14 s 后 `database is locked` |

审计工具（静态 `lockreach`、动态 `lockinstr` + `txheld` + 连接池包装）与输出放在本组会话共享笔记目录的
`sqlite-source-user-lock-tools/`，只在临时副本上使用。

## 11. 明确不重开

- 第 4 节其余跨事务持有的锁：它们都不在事务内取得，并保护数据库之外的状态。
- RSS 导入的 URL 查找：每个候选在该用户的行中逐行查找，用户已有上万条时单次 5,000 条导入的事务持锁 3 秒以上。RSS 写边界合同第 4 节
  因历史重复行拒绝 `(user_id,url)` 唯一索引；非唯一索引是新增迁移，可另起性能切片。
- DSN、WAL、连接池、busy_timeout、会话续期频率、备份导出与恢复本身的持锁时间。
- 恢复的 RSS 步骤继续直接用 `tx` 写入，不改为复用 rss 服务。

## 12. 状态

**contract-only**（由第 13 节记录取代）。

## 13. 记录（2026-11-10）

提交：合同 `883c0c2`；红测 `faa6cab`（T1）；实现 `8141c28`（书源，L1，+8/−38）与 `59b8f95`（`refactor(rss)`，L2，+7/−52）；守护补充 `4935b3c`。

T1 在 `9076cfa` 上 3 次运行都失败：新建 5.189/5.367/5.424 s 后 `500 {"error":"failed to create source"}`，导入 5.199/5.400/5.366 s 后
`500 {"error":"failed to import sources"}`，每次之前都有 `2s after the restore resumed, … were still waiting on each other`。`8141c28` 后
`-count=3` 通过，每个子测试 0.05–0.07 s。

守护补充（第 9 节没有列出，实现后发现需要）：变异显示，现有守护不能单独说明 L1 当初提供了什么——`services/booksources` 的 G3/G4 与
`services/rss` 的 G6/G7 在 deferred 下有锁、无锁都失败（它们测的是跨用户写入，只有 immediate 能修），api
`TestBookSourceLimitSerializesConcurrentFinalSlot` 只有两个不同步的请求，deferred 且无锁时 3 次运行也没有撞上。新增
`TestSameUserSourceWritesSerializeWithoutAProcessLock`：同一用户 6 个并发新建/导入在事务内读取用户行处会合（最多等 200 ms）。

| 守护 | 实现（immediate，无锁） | deferred，无锁 | deferred，有锁（`9076cfa` 的服务代码） |
| --- | --- | --- | --- |
| `TestSameUserSourceWritesSerializeWithoutAProcessLock` | 5/5 通过 | 5/5 失败：`database is locked`（共 21 次），6 个只留下 1–2 个 | 5/5 通过 |
| api `TestRSSSourceIdentitySerializesConcurrentCreateAndImport` | 通过 | 3/3 失败：三种场景共 31 次 `500` | 3/3 通过 |
| api `TestBookSourceLimitSerializesConcurrentFinalSlot` | 通过 | 3/3 通过（两个请求没有重叠） | 3/3 通过 |
| G3/G4、G6/G7 | 通过 | 失败（G4 共 35 次 `database is locked`） | 失败（G4 共 21 次） |

即：deferred 下同一用户的串行化原来靠 L1/L2；immediate 下没有它们也成立。

审计（第 4 节方法，`4935b3c`）：静态 62 个事务回调、可达 359 个模块函数、有锁位点的模块函数 50 个，事务内可达的锁位点 **0**；
动态（54 个插桩位点、完整 `go test ./...`）：**0** 条报告（实现前 18 条）。唯一失败的仍是第 4 节的审计假象，它也证明包装连接池已安装；
另加的阳性对照（事务内外各调用一次检查）只报告事务内那一次。

回归：

- 后端（`59b8f95`）：`go test ./...` 31 个包通过（api 128.2 s）；`go vet ./...` 通过，gofmt 无输出；`go test -race ./db ./services/...` 25 个包
  通过、无数据竞争；`go test -race -timeout 40m ./api` 通过、无数据竞争（1069.2 s）。之后的 `4935b3c` 只加了 `services/booksources` 的守护测试，该包
  `-count=3` 与 `-race -count=2` 通过。
- 前端未改：`npm test` 877/877、`npm run build` 通过（`node_modules` 克隆自锁文件相同的工作树）。
- 发布门自测：Go、前端、OS 三个公告门 136/136，浏览器 smoke 自测 10/10；现场 Go 与前端公告扫描 `pass`、0 个发现。

未跑：Compose、存储 HTTP 合同、fresh/历史卷与可移植备份门（不涉及数据文件，Actions 在推送 main 时运行）；浏览器 smoke（没有前端或接口形状
变化）。

状态 **implementation-complete / regression-validated**。分支 `claude/sqlite-restore-user-lock` 已推送（基于 `9076cfa`），未合入 main，镜像未发布，
生产未访问。

## 14. 发布（2026-11-10）

经用户确认，推送前在同一树上另跑 `docker compose config --quiet` 与存储读写/列表生命周期合同（通过），main 由 `9076cfa` 快进到 `95f2840`
（ls-remote 核验，未强推）。可信 Actions run `42775373137` 30/41 步后在 OS APK 门失败（两平台 `zlib 1.3.2-r1 < 1.4-r0 CVE-2026-4738`，该 secdb 条目在
run 开始前约两分钟发布），卷门与发布被跳过，没有发布镜像。经用户批准按
[`os-apk-zlib-1.4-release-blocker-p1-contract.md`](os-apk-zlib-1.4-release-blocker-p1-contract.md) 修复后，main 快进到 `74cf0d0`，可信 Actions run
`42780054569` 43/43 步成功；`ghcr.io/changshengyu/openreader:74cf0d0` 与 `latest` 为 OCI index
`sha256:d2ed3607a1fb3d888ed919fe618c16386de2f676f2218243b02e9ce2fbd2eb92`（linux/amd64 `sha256:bb760ee4…`、linux/arm64 `sha256:812f0093…`），
两平台 revision 标签均为 `74cf0d04dfba41703a3903316705412eb384f59c`。本切片的 `8141c28`、`59b8f95` 随该镜像发布。
状态 **aligned / Docker-published / awaiting-device-verification**；生产未访问。
