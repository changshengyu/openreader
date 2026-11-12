# RSS 订阅源按用户与 URL 查找的非唯一索引（导入持写锁数秒）P2 合同

日期：2026-11-10。分支 `claude/rss-user-url-index`，基于 origin/main `752aa4e`（含 `74cf0d0` 发布的 RSS 按用户写锁退役 `59b8f95`，
以及 `1b50602` 起的 `BEGIN IMMEDIATE`）。

## 1. 背景与范围

[退役按用户写锁合同](sqlite-source-user-lock-retirement-p2-contract.md)第 5、10、11 节记录：`ImportSources` 对每个候选 URL 在该用户的
全部行中逐行查找（`rss_sources` 只有 `user_id` 索引），用户已有上万条时单次 5,000 条导入的 `BEGIN IMMEDIATE` 事务持写锁 3 秒以上，
期间其他所有写入都在 `BEGIN` 排队；同一用户 4 个并发 5,000 条导入时第 4 个 `database is locked`。该合同把 URL 查找索引留给另起的性能切片。

本切片只做一件事：给 `rss_sources` 增加非唯一索引 `idx_rss_sources_user_url (user_id, url)`，经现有的非破坏性迁移
（`backend/db/db.go#AutoMigrate`，模型标签声明）在全新库与历史库上建立。不改查询语义、接口形状、状态码、事件、DSN、WAL、连接池、
busy_timeout，不加唯一约束，不清理重复行。

## 2. 上游行为

上游 reader-dev（`fa22f27`）`RssSourceController.kt#saveRssSource`（第 88–127 行）与 `#saveRssSources`（第 129–173 行）读出用户的
`rssSources.json`，对每个来源在内存列表中从头线性查找第一个 `sourceUrl` 相等的条目：找到则原位替换，否则追加，最后整体写回。
`deleteRssSource` 同样按第一个相等的 `sourceUrl` 删除。查找发生在内存里，不阻塞其他用户。

OpenReader 把来源存在 SQLite 行中，“第一个相等的条目”对应“同一用户该 URL 中 `id` 最小的行”（`ORDER BY id asc LIMIT 1`）。
索引只改变 SQLite 找到这一行的方式，不改变找到哪一行，属于 Go/SQLite 运行时适配；上游没有“查找时持有全库写锁”的代价。

## 3. 当前行为（逐项核实，`752aa4e`，SQLite 3.51.3）

1. **表结构。** `models.RSSSource`（`backend/models/models.go` 第 273–303 行）只给 `user_id` 建索引。`AutoMigrate` 写出的结构为：

   ```sql
   CREATE TABLE `rss_sources` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` integer NOT NULL,`title` text NOT NULL,`url` text NOT NULL,`icon` text,`group` text,`comment` text,`custom_order` integer DEFAULT 0,`concurrent_rate` text,`header` text,`login_url` text,`login_check_js` text,`single_url` numeric,`article_style` integer,`sort_url` text,`rule_articles` text,`rule_next_page` text,`rule_title` text,`rule_pub_date` text,`rule_description` text,`rule_image` text,`rule_link` text,`rule_content` text,`style` text,`enable_js` numeric,`load_with_base_url` numeric,`enabled` numeric,`created_at` datetime,`updated_at` datetime)
   CREATE INDEX `idx_rss_sources_user_id` ON `rss_sources`(`user_id`)
   ```

2. **四处按 URL 查找**，都以去掉首尾空白的 Go 字符串绑定 `url = ?`，列上没有函数：
   - `backend/services/rss/service.go` 第 60 行 `CreateOrReplaceSource`：`SELECT * … WHERE user_id = ? AND url = ? ORDER BY id asc LIMIT 1`；
   - 第 124 行 `UpdateSource` 冲突检查：`SELECT count(*) … WHERE user_id = ? AND url = ? AND id <> ?`；
   - 第 184 行 `ImportSources`：与新建相同，**每个候选一次**，都在同一个 immediate 事务里；
   - `backend/api/webdav.go` 第 881 行 `restoreRSSSourcesFromData`：`Where(user_id, url).Assign(…).FirstOrCreate`，生成
     `SELECT * … WHERE user_id = ? AND url = ? ORDER BY rss_sources.id LIMIT 1`，**每个恢复条目一次**，在备份恢复事务里。

   四者的查询计划（`EXPLAIN QUERY PLAN`，捕获服务实际执行的 SQL 与参数）都是
   `SEARCH rss_sources USING INDEX idx_rss_sources_user_id (user_id=?)`：每次查找遍历该用户的每一行，并为比较 `url` 读出整行。
   库里没有 `sqlite_stat*` 表（代码不运行 `ANALYZE`/`PRAGMA optimize`），计划只由结构决定。

3. **代价（本机，`go test`，临时测试，不提交）。** 用户 1 已有 N 行（另一用户 3,000 行），一次 5,000 条全新 URL 的导入（新建），随后同样
   5,000 条再导入一次（全部更新）：

   | 用户已有行数 | 新建 | 更新 |
   | --- | --- | --- |
   | 0 | 0.66 s | 0.69 s |
   | 5,000 | 1.62 s | 1.63 s |
   | 10,000 | 2.74 s | 2.65 s |
   | 20,000 | 6.11 s | 5.99 s |
   | 40,000 | 11.33 s | — |

   整个导入是一个 `BEGIN IMMEDIATE` 事务，上表即持写锁时间。用户已有 2 万行时已超过 busy_timeout（5 s）：任何在导入开始后不久等待写锁的
   写入（其他用户的设置保存、进度上传、会话续期等）都会 `database is locked` 失败。单条新建（`CreateOrReplaceSource`）同样逐行查找，
   2 万行时约 2.7 ms。

4. **只按用户过滤的查询**（列表 `ORDER BY custom_order asc, updated_at desc`、`COALESCE(MAX(custom_order), 0)`、备份导出
   `ORDER BY user_id, custom_order, updated_at`、管理员删除 `user_id IN ?`）用 `idx_rss_sources_user_id`；按 `id` 的读写用主键。

5. **历史重复行。** [RSS 写边界合同](rss-write-boundary-fixed-baseline-second-audit-p2-contract.md)第 4 节：历史库可能已有同一用户同一
   URL 的多行，因此拒绝 `(user_id,url)` 唯一索引。现在对这些行的行为：单条保存、导入与备份恢复都改写 `id` 最小的那一行；
   `PUT /api/rss/sources/:id` 保持原 URL 时，冲突检查看到另一行，任一重复行都返回 409；删除任一行成功；列表与导出列出全部行。

## 4. 分析

**为什么是 `(user_id, url)` 非唯一索引。** 四处查找都是两列等值条件。SQLite 的普通索引隐含 rowid（`id` 是 `INTEGER PRIMARY KEY`），
同一 `(user_id, url)` 的条目按 `id` 排列，因此 `ORDER BY id LIMIT 1` 直接取第一条、不需要临时排序，冲突计数只读索引（covering）。
唯一索引会让含重复行的历史库在启动时 `CREATE UNIQUE INDEX` 失败，违反“挂载卷原样升级”。非唯一索引对任何数据都能建立。

**加索引后的计划（临时测试实测）。** 新建、替换、导入、恢复：`SEARCH rss_sources USING INDEX idx_rss_sources_user_url (user_id=? AND url=?)`；
冲突计数：`SEARCH rss_sources USING COVERING INDEX idx_rss_sources_user_url (user_id=? AND url=?)`。第 3 节第 4 点的查询计划不变
（仍用 `idx_rss_sources_user_id` 或主键），因此列表、导出中排序键相同的行的先后顺序也不变。

**代价消失。** 同一测试：用户已有 0 / 5,000 / 10,000 / 20,000 / 40,000 行时，5,000 条导入新建 0.17–0.18 s、更新 0.20–0.22 s，不再随用户行数增长
（`-race` 下 4 万行约 0.80 s）；单条新建 2 万行时约 1.7 ms（剩下的是 `nextSourceOrder` 每次一次的 `MAX(custom_order)`）。

**迁移机制。** `main.go` 第 72 行在开始服务前调用 `db.AutoMigrate`。gorm v1.25.12 `AutoMigrate` 对模型声明的每个索引先 `HasIndex`
（查 `sqlite_master`），缺失时 `CreateIndex`（`CREATE INDEX … ON rss_sources(user_id,url)`），重复启动不再执行。gorm.io/driver/sqlite v1.5.6
解析列定义时忽略索引语句（`ddlmod.go` 的 `indexRegexp` 分支），加索引不会触发 `AlterColumn` 的整表重建；表结构语句保持原样。
建立是一条语句、原子完成：20 万行约 68 ms。

**回退。** 回退到旧镜像时，旧代码的 `AutoMigrate` 不认识也不删除这个索引；旧代码的查询照常工作（计划也会用上它），重复行仍可写入。
索引不带约束，不改变任何数据含义。

**写入开销。** 每次新建多维护一个索引项，改写 `url` 时更新它；上表的导入新建时间说明这部分可以忽略。

**保留 `idx_rss_sources_user_id`。** 新索引的首列也是 `user_id`，旧索引在功能上冗余；但删除索引属于破坏性迁移，且会改变第 3 节第 4 点
查询的计划。保留它没有正确性代价。

## 5. 方案

**A. 模型标签声明非唯一复合索引，由 `AutoMigrate` 建立（选定）。** `UserID` 标签加 `index:idx_rss_sources_user_url,priority:1`
（原有 `index` 保持 `idx_rss_sources_user_id`），`URL` 加 `index:idx_rss_sources_user_url,priority:2`。现有索引都在模型标签里声明，
模型仍是结构的唯一来源。查询不需要改。

**B. 唯一索引（不选）。** 含重复行的历史库无法升级；见第 4 节。

**C. 在 `db.go` 里另写 `CREATE INDEX IF NOT EXISTS`（不选）。** 与 A 效果相同，但模型不再反映真实结构，`AutoMigrate` 也看不到它。

**D. 导入时先把该用户的 URL 全部读进内存（不选）。** 每次导入仍读一遍用户的全部行，新建、更新冲突与恢复照旧逐行查找，代码更多。

不考虑禁用 WAL、改单连接或改 busy_timeout。

## 6. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| U1 | 新建、更新冲突、导入、备份恢复四处按 URL 查找都遍历该用户的全部行；5,000 条导入持写锁时间随用户行数线性增长（1 万行 2.7 s、2 万行 6.1 s、4 万行 11.3 s），超过 busy_timeout 后其他写入失败 | `must-fix` | 四处都走 `(user_id, url)` 索引查找；导入持锁时间与用户已有行数无关 |
| U2 | `rss_sources` 新增非唯一索引 `idx_rss_sources_user_url (user_id, url)`，全新库与历史库启动时由 `AutoMigrate` 建立 | `acceptable-change`（新增、非破坏性的存储适配） | 已有行（包括重复行）逐字节不变；重复启动幂等；表结构语句不变 |
| U3 | 历史重复行的行为（`id` 最小的行被改写、更新冲突 409、删除、列表与导出） | 不重开 | 保持 |
| U4 | 备份恢复对每个缺少 `customOrder` 的条目执行一次 `COALESCE(MAX(custom_order), 0)`，仍遍历该用户的行 | 不重开（后续切片） | 见第 9 节 |
| U5 | 唯一约束、重复行清理、`idx_rss_sources_user_id`、RSS 文章表索引、DSN、WAL、连接池、busy_timeout | 不重开 | 保持 |

## 7. 目标行为

1. 全新库与历史库（包括同一用户同一 URL 有多行的库）启动后，`rss_sources` 有非唯一、非部分索引 `idx_rss_sources_user_url`，列依次为
   `user_id`、`url`；`idx_rss_sources_user_id` 保留；已有行与表结构语句不变；再次启动不改变任何东西。
2. 四处按 URL 查找的计划都是 `SEARCH rss_sources USING [COVERING] INDEX idx_rss_sources_user_url (user_id=? AND url=?)`，没有临时排序。
3. 用户已有 4 万行时，一次 5,000 条导入在 busy_timeout 之内提交；它持写锁期间开始的另一用户写入成功。
4. 第 3 节第 5 点的重复行行为、第 3 节第 4 点的查询计划、接口形状、状态码与事件不变。

## 8. 计划测试

红测（`752aa4e` 上必须失败）：

- T1 `backend/api/rss_source_url_index_contract_test.go#TestRSSSourceURLLookupsUseTheUserURLIndex`：经真实路由依次新建
  （`POST /api/rss/sources`）、同 URL 替换、`PUT /api/rss/sources/:id`、`POST /api/rss/sources/import`、`POST /api/backup/restore-legado`
  （`rssSources.json`），查询回调对每条按 `url` 过滤的 `rss_sources` 查询用同一连接、同一 SQL 与参数执行 `EXPLAIN QUERY PLAN`；
  要求每一步至少捕获一条，且每条计划都只有第 7 节第 2 点那一行。
- T2 `backend/services/rss/user_url_index_contract_test.go#TestMaximalRSSImportForALargeUserDoesNotFailAConcurrentWrite`：用户已有
  40,000 行，导入事务第一次查询后（已持写锁）另一用户经同一应用库更新自己的来源；要求导入在 busy_timeout（读自 `PRAGMA busy_timeout`）
  内完成、另一写入成功。
- T3 `backend/db/rss_source_user_url_index_migration_contract_test.go#TestAutoMigrateAddsTheRSSSourceUserURLIndex`：全新库的索引名、
  非唯一、非部分、列顺序，旧索引保留。
- T4 同文件 `#TestAutoMigrateIndexesHistoricalRSSSourcesWithDuplicateURLs`：用第 3 节第 1 点的原样结构建表，写入同一用户同一 URL 的两行、
  另一用户同一 URL 的一行与其他行；`AutoMigrate` 两次都成功，索引建立，全部行与表结构语句逐字不变。

对照（实现前后都通过）：

- C1 `backend/services/rss/user_url_index_contract_test.go#TestRSSWritesKeepHistoricalDuplicateURLBehavior`：先去掉新索引、写入重复行、
  再 `AutoMigrate`（模拟升级），验证第 3 节第 5 点的全部行为与另一用户隔离。
- 既有：api `TestRSSSourceIdentitySerializesConcurrentCreateAndImport`、`TestCreateRSSSourceReplacesSameUserURLAndPreservesUserIsolation`、
  `TestUpdateRSSSourceRejectsSameUserURLCollision`，`services/rss` G6/G7 与导入测试，备份恢复合同。
- H1 历史卷门：`backend/cmd/create-old-volume-fixture` 去掉新索引（`DROP INDEX IF EXISTS`），写入所有者同一 URL 的两行与另一用户同一 URL
  的一行；`scripts/docker-volume-backup-smoke.sh` 历史卷模式在首次启动后检查两个用户的 RSS 列表仍是这些行。由 Actions 在推送 main 时运行；
  本地用同一夹具与本机后端（不构建镜像）验证。

回归：`go test ./...`、`go vet ./...`、`go test -race ./db ./services/...` 与 `go test -race -timeout 40m ./api`，前端 `npm test` 与
`npm run build`，公告门自测。

## 9. 明确不重开

- **备份恢复的排序值。** `restoreRSSSourcesFromData` 对每个缺少正数 `customOrder` 的条目调用 `orderOrDefaultStrict`
  （`backend/api/rss.go` 第 218 行），即一次 `COALESCE(MAX(custom_order), 0) WHERE user_id = ?`，仍遍历该用户的行；`COALESCE` 包住聚合，
  SQLite 的 min/max 优化不适用，单加 `(user_id, custom_order)` 索引也不够。实测（临时测试，恢复事务内只跑 RSS 步骤，5,000 条、用户已有
  1 万行）：条目带 `customOrder` 时 2.69 s → 加索引后 0.19 s；不带时 5.04 s → 2.64 s。改它要同时改查询与排序值语义的证明，并涉及
  [退役按用户写锁合同](sqlite-source-user-lock-retirement-p2-contract.md)第 11 节不重开的恢复持锁时间，另起切片。
  （2026-11-11 更正：上面关于 `COALESCE` 的判断是错的。SQLite 只要求查询只有一个聚合函数，`COALESCE(MAX(custom_order), 0)` 加
  `(user_id, custom_order)` 索引后每次约 5.8 µs，不需要改查询或编号语义；见
  [`rss-restore-custom-order-index-p2-contract.md`](rss-restore-custom-order-index-p2-contract.md)。）
- 唯一约束与重复行清理；`idx_rss_sources_user_id` 的去留。
- RSS 文章表的查找与索引（`UpsertArticlePage` 等）。
- 单条新建的 `nextSourceOrder`（每次一次 `MAX`）。
- DSN、WAL、连接池、busy_timeout、会话续期频率。

## 10. 状态

**contract-only**（由第 11 节记录取代）。

## 11. 记录（2026-11-11）

提交：合同 `48f2c28`；红测 `cee1589`（T1–T4、对照 C1、历史卷夹具与 smoke H1）；实现 `8f6bde5`（`fix:`，只改 `backend/models/models.go`
的 `RSSSource` 标签与注释，+6/−2，查询代码不变）。

红测在 `752aa4e` 的实现上（`cee1589`）：

- T1：五步共捕获 7 条按 URL 的查询（新建 1、替换 1、更新冲突计数 1、导入 2、恢复 2），计划全部为
  `SEARCH rss_sources USING INDEX idx_rss_sources_user_id (user_id=?)`。
- T2：`a 5000-source import for a user with 40000 sources held the write lock for 11.413s (busy_timeout 5s); another user's write that
  waited for it returned after 5.18s with database is locked`（另一次运行同样失败，整个测试 11.64 s）。
- T3/T4：索引只有 `idx_rss_sources_user_id`；T4 在索引断言之前的行内容、表结构语句与 `integrity_check` 检查都通过。
- C1 通过（对照）。

`8f6bde5` 后同一断言全部通过：T1 七条计划都是 `idx_rss_sources_user_url (user_id=? AND url=?)`（计数为 COVERING）；T2 0.25–0.29 s
（`-count=3`），`-race` 下 0.95–0.97 s（`-count=3`，含 4 万行写入）；T3/T4 通过；C1 `-count=3` 通过。

补充实测（临时测试，不提交）：同一用户 4 个并发 5,000 条导入（空用户）全部成功，依次 0.17/0.36/0.62/0.87 s（复跑 0.17/0.36/0.53/0.73 s）；
退役按用户写锁合同第 10 节在无索引时是 0.67/2.35/5.83 s，第 4 个 5.14 s 后 `database is locked`。

H1 本地验证（不构建镜像）：用 `8f6bde5` 的夹具工具生成历史卷（所有者同一 URL 两行、另一用户一行，只有 `idx_rss_sources_user_id`），本机编译的
后端二进制直接挂这个卷启动，`/api/health` 正常；从 `scripts/docker-volume-backup-smoke.sh` 原样取出 `assert_historical_rss_rows`（及 `json_field`、
`historical_login`）用 `sh` 运行，两个用户都通过，故意少写一个标题的反向检查按预期失败；停止后库中两个索引都在（新索引列为 `user_id`、`url`），
三行不变，`integrity_check` 为 `ok`。镜像内的 fresh/历史卷门由 Actions 在推送 main 时运行。

回归（`8f6bde5`）：

- 后端：`go vet ./...` 通过，gofmt 无输出；`go test ./...` 31 个包通过（api 123.0 s）；`go test -race ./db ./services/...` 25 个包通过、
  无数据竞争；`go test -race -timeout 40m ./api` 通过、无数据竞争（1070.0 s）。
- 前端未改：`npm test` 877/877、`npm run build` 通过（`node_modules` 以 APFS 克隆自锁文件相同的工作树，本机 Node v26.4.0；CI 用 Node 22）。
- 发布门自测：Go、前端、OS 三个公告门 138/138，浏览器 smoke 自测 10/10；现场 Go 公告扫描 `pass`、0 个发现（249 条公告，数据库
  2026-11-05T23:49:57Z），现场前端扫描 `pass`、0 个发现。
- 含端口监听的测试（api、前端公告门 worker、smoke）与现场 Go 扫描（沙箱代理下 `govulncheck` 下载 TLS 失败）在沙箱外运行。

未跑：Compose、存储 HTTP 合同、镜像内 fresh/历史卷与可移植备份门（Actions 在推送 main 时运行，历史卷门已含 H1）；浏览器 smoke
（没有前端或接口形状变化）。

origin/main 在切出后前进到 `7ab3fe3`（只改一份无关合同文档）；本分支与它 `git merge-tree` 无冲突，合入时需要一个合并提交。

状态 **implementation-complete / regression-validated**。分支 `claude/rss-user-url-index` 已推送，未合入 main，镜像未发布，生产未访问。

## 发布（2026-11-12）

按用户选择，与 Go 1.26.11（GO-2026-6859）、注册邀请码与用户数上限、RSS 两项索引一起发布：合并提交 `516a4f0`（注册）、`08b3e09`（RSS）、`6c4f004` 与 `28813c2`（Go），main 由 `e8f3214` 快进到 `28813c2`（ls-remote 核验，未强推）。

推送前在合并树上（go1.26.11）：后端 `go test ./...` 31 个包与 vet，race api/db/config/services 27 个包（api 1150.3 秒），无数据竞争；前端 882/882 与 build；公告门自测 139/139；现场 Go 与前端扫描 0 个发现；compose、存储 HTTP 合同；注册限制合同与 openreader-smoke 三视口、重新认证合同四视口通过；secdb main 11-11 22:16:35 下运行时 origin 与夹具相同，5 个未列 origin 仍无记录。

可信 Actions run `43191938289` 43/43 步成功（09:29–09:46 UTC）：两平台 OS 门 `pass-with-unknown-origins`（16/16 包、110/109 个 APK 文件、74 组比较无待修复项，复审期限 2026-12-10），Go 公告门 go1.26.11 0 个发现，卷门与平台核验通过。

镜像：`ghcr.io/changshengyu/openreader:28813c2` 与 `latest` 指向 OCI index `sha256:83d9918ca08e764a574d1d9b64707ded70d7b1949decf4ae4457b8d05f9cddb0`；linux/amd64 `sha256:a6995415214474f72b976bae134fd6043e97d054c664c816e49811c8942edf44`，linux/arm64 `sha256:20bcf60fbff86eeca2b062f2975e03965453011506d44e6773654d6e6d2494bd`，另有 attestation。两平台 revision 标签均为 `28813c259f93955e67ad53a9b97fb326a3bc982e`，两平台镜像内 `/app/openreader` 均为 go1.26.11 构建。

状态 **released / Docker-published / awaiting-device-verification**；生产未访问，实际运行版本未知。
