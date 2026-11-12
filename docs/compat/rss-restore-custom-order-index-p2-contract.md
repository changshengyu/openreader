# RSS 订阅源按用户与排序值的非唯一索引（恢复逐条取最大排序值）P2 合同

日期：2026-11-11。分支 `claude/rss-restore-custom-order`，基于 `claude/rss-user-url-index` 的 `2152114`（该分支基于 origin/main `752aa4e`，
新增了 `idx_rss_sources_user_url`，尚未合入 main）。

## 1. 背景与范围

[RSS 订阅源 URL 索引合同](rss-source-user-url-index-p2-contract.md)第 9 节（U4）留下：备份恢复对每个缺少正数 `customOrder` 的 RSS 条目执行一次
`COALESCE(MAX(custom_order), 0) WHERE user_id = ?`，仍遍历该用户的全部行，加了 URL 索引后 5,000 条恢复进 1 万行的用户仍要 2.64 s。

该节还写道“`COALESCE` 包住聚合，SQLite 的 min/max 优化不适用，单加 `(user_id, custom_order)` 索引也不够”。**这一判断是错的**（第 4 节实测）：
SQLite 只要求整个查询只有一个聚合函数，`COALESCE(MAX(x), 0)` 仍走 min/max 优化。因此本切片只给 `rss_sources` 增加非唯一索引
`idx_rss_sources_user_custom_order (user_id, custom_order)`，不改查询、不改编号语义、不改接口形状、状态码与事件，经现有 `AutoMigrate` 建立。

## 2. 上游行为

上游 reader-dev（`fa22f27`）的 WebDAV 恢复 `WebdavController.kt#restoreFromWebdav → BookController.kt#syncFromWebdav` 解压备份后直接替换用户的
`rssSources.json`（见[退役按用户写锁合同](sqlite-source-user-lock-retirement-p2-contract.md)第 2 节），条目的 `customOrder` 原样保留，不重新编号。

OpenReader 把恢复的条目合并进 SQLite 行，并给缺少正数 `customOrder` 的条目编号（`bc518cd6` 起，
[备份恢复固定基准合同](backup-restore-fixed-baseline-p2-contract.md) RSS 行“保持并补齐严格错误传播”）。这一编号规则是既有的 OpenReader 行为，
本切片不改变它，只改变 SQLite 计算最大排序值的方式（Go/SQLite 运行时适配）。

## 3. 当前行为（逐项核实，`2152114`，SQLite 3.51.3）

1. **编号规则。** `backend/api/webdav.go#restoreRSSSourcesFromData`（第 848 行）对每个条目调用 `backend/api/rss.go#orderOrDefaultStrict`
   （第 218 行）：条目的 `customOrder` 为正数时直接使用，否则为该用户当前的 `COALESCE(MAX(custom_order), 0) + 1`，“当前”包括本次恢复中前面
   条目的写入（同一事务）；随后 `FirstOrCreate` 改写同一 URL 中 `id` 最小的行或新建一行。前面的条目可以把原来最大值所在的行改小，之后的
   编号跟着变小。
2. **同样的查询还有两处**，各执行一次：`backend/services/rss/service.go#nextSourceOrder`（单条新建）与 `ImportSources` 开头。
3. **计划。** 三处都是 `SEARCH rss_sources USING INDEX idx_rss_sources_user_id (user_id=?)`：每次为读 `custom_order` 遍历该用户的每一行。
4. **代价（本机，`go test`，临时测试）。** 用户已有 4 万行时每次约 2.08 ms（不包 `COALESCE` 2.05 ms）。恢复 5,000 个不带 `customOrder` 的条目
   （恢复事务内只跑 RSS 步骤）：用户已有 1 万行 2.64 s；4 万行时经真实路由 `POST /api/backup/restore-legado` 整个恢复 11.54 s，期间开始等待的
   另一用户写入 5.198 s 后 `database is locked`（第 8 节 T1 在 `2152114` 上的结果）。
5. **只按用户过滤的查询**（列表 `ORDER BY custom_order asc, updated_at desc`、备份导出 `ORDER BY user_id, custom_order, updated_at`，单用户
   与全部用户）用 `idx_rss_sources_user_id` 并整体排序；管理员删除用 `idx_rss_sources_user_id`；URL 查找用 `idx_rss_sources_user_url`。

## 4. 分析

**索引让每次取最大值变成一次查找。** 加 `(user_id, custom_order)` 后（临时测试，4 万行）：`COALESCE(MAX(custom_order), 0)` 每次 5.75 µs，
不包 `COALESCE` 5.39 µs，计划为 `SEARCH rss_sources USING COVERING INDEX idx_rss_sources_user_custom_order (user_id=?)`。SQLite 在查询只有一个
聚合函数时对 `min()`/`max()` 做“取索引一端”的优化，与外层 `COALESCE` 无关；空用户时 `MAX` 为 NULL，`COALESCE` 仍给 0；NULL 排序值被 `MAX`
忽略（第 8 节 T3 历史库含一行 NULL）。结果与逐行遍历完全相同，所以编号语义不变。

**被改变计划的查询。** 列表与导出改用新索引按 `custom_order` 顺序读取，只再排后一项：
`SEARCH rss_sources USING INDEX idx_rss_sources_user_custom_order (user_id=?) | USE TEMP B-TREE FOR LAST TERM OF ORDER BY`（全部用户导出为
`SCAN … USING INDEX idx_rss_sources_user_custom_order`）。排序键不同的行结果不可能改变；排序键完全相同的行，SQL 不规定先后，实测也与原计划一致：
3,000 行（7 个排序值 × 2 个 `updated_at`）与 4.3 万行只取 `id`、以及 12 万行每行 2 KB 规则文本（远超 2 MB 缓存、排序溢出到临时文件）的 `SELECT *`，
加索引前后三条查询的 `id` 顺序逐个相同。管理员删除与 URL 查找的计划不变。

**写入开销与迁移。** 4 万行用户 5,000 条导入：新建 188 → 185 ms、更新 215 → 219 ms（无可测差异）。`AutoMigrate` 对缺失索引 `CREATE INDEX`，
20 万行约 41 ms，原子完成；表结构语句不变；旧镜像回退时忽略该索引；非唯一，任何历史数据都能建立。

**不改代码的其他方案（不选）。**
- 恢复开始时读一次最大值、之后在内存里递增：第 3 节第 1 点的“前面的条目把最大值所在的行改小”会得到不同编号（第 8 节 C1 中 D 应为 6，这样会是 10）。
- 在内存里精确模拟：要读入该用户全部行、维护排序值的多重集合，并复现 `FirstOrCreate` 按最小 `id` 匹配 URL 的规则，代码多且容易偏离数据库的真实结果。

## 5. 方案

**A. 模型标签声明非唯一复合索引，由 `AutoMigrate` 建立（选定）。** `UserID` 再加 `index:idx_rss_sources_user_custom_order,priority:1`，
`CustomOrder` 加 `index:idx_rss_sources_user_custom_order,priority:2`。查询代码不变。

## 6. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| O1 | 恢复对每个缺少正数 `customOrder` 的条目遍历该用户全部行取最大排序值；4 万行用户 5,000 条恢复持写锁 11.5 s，其他写入失败 | `must-fix` | 每次取最大值为一次索引查找；该恢复在 busy_timeout 内完成 |
| O2 | `rss_sources` 新增非唯一索引 `idx_rss_sources_user_custom_order (user_id, custom_order)`；列表与导出改用它读取 | `acceptable-change`（新增、非破坏性的存储适配；结果实测不变） | 已有行与表结构语句不变；重复启动幂等 |
| O3 | 编号规则（第 3 节第 1 点）、单条新建与导入的编号 | 不重开 | 保持，由 C1 固定 |
| O4 | `idx_rss_sources_user_id`、`idx_rss_sources_user_url`、唯一约束、重复行、DSN、WAL、连接池、busy_timeout | 不重开 | 保持 |

## 7. 目标行为

1. 全新库与历史库（含重复 URL、重复排序值、NULL 排序值）启动后有非唯一、非部分索引 `idx_rss_sources_user_custom_order`，列依次为 `user_id`、
   `custom_order`；已有行、表结构语句与其他索引不变；再次启动不改变任何东西。
2. 单条新建、导入与恢复的 `MAX(custom_order)` 查询计划都是 `SEARCH rss_sources USING COVERING INDEX idx_rss_sources_user_custom_order (user_id=?)`。
3. 用户已有 4 万行时，经真实路由恢复 5,000 个不带 `customOrder` 的条目在 busy_timeout 内完成；其间开始等待的另一用户写入成功；编号依次为
   40001–45000。
4. 编号规则与列表、导出结果不变。

## 8. 计划测试

红测（`2152114` 上必须失败）：

- T1 `backend/api/rss_restore_custom_order_contract_test.go#TestRSSRestoreForALargeUserDoesNotFailAConcurrentWrite`：如第 7 节第 3 点，恢复事务
  第一次查询 `rss_sources` 后另一用户经同一应用库更新自己的来源；busy_timeout 读自 `PRAGMA busy_timeout`。
- T2 同文件 `#TestRSSSourceOrderLookupsUseTheUserCustomOrderIndex`：经 `POST /api/rss/sources`、`POST /api/rss/sources/import`、
  `POST /api/backup/restore-legado`（两个不带 `customOrder` 的条目），在 gorm 的 Query 与 Row 回调里对每条 `MAX(custom_order)` 查询用同一连接、
  SQL 与参数执行 `EXPLAIN QUERY PLAN`，要求每一步至少一条且都只有第 7 节第 2 点那一行。
- T3 `backend/db/rss_source_user_custom_order_index_migration_contract_test.go#TestAutoMigrateAddsTheRSSSourceUserCustomOrderIndex`
  （子测试 fresh、historical）：历史库用 `752aa4e` 的原样结构与含重复与 NULL 的行，`AutoMigrate` 两次，行逐字不变、表结构语句不变、
  `integrity_check` 为 ok，两个新索引都在，`MAX` 忽略 NULL。

对照（实现前后都通过）：

- C1 同 T1 文件 `#TestRSSRestoreKeepsItsCustomOrderSequence`：已有 A=5、B=9、C=2，恢复 B(1)、D、A、E(3)、F(0)、F，要求
  A=7、B=1、C=2、D=6、E=3、F=9，另一用户的排序值不变。
- 既有：RSS 列表、导入、备份导出与恢复合同，上一切片的 T1–T4、C1。
- H1 历史卷：`backend/cmd/create-old-volume-fixture` 同时去掉两个新索引；`scripts/docker-volume-backup-smoke.sh` 的检查不变。

回归：`go test ./...`、`go vet ./...`、`go test -race ./db ./services/...` 与 `go test -race -timeout 40m ./api`，前端 `npm test` 与
`npm run build`，公告门自测。

## 9. 明确不重开

- 编号规则本身（含上游不重新编号的差异）、恢复的其余持锁时间、`FirstOrCreate` 只改写非零字段的既有行为。
- 列表与导出中排序键完全相同的行的先后（SQL 不规定，实测不变）。
- DSN、WAL、连接池、busy_timeout。

## 10. 状态

**contract-only**（由第 11 节记录取代）。

## 11. 记录（2026-11-12）

提交：合同 `10c9e69`（含 URL 索引合同第 9 节的更正）；红测 `4931b8d`（T1–T3、对照 C1、夹具同时去掉两个新索引）；实现 `7e1a267`（`fix:`，只改
`backend/models/models.go` 的 `RSSSource` 标签与注释，+5/−3，查询代码不变）。

红测在 `2152114` 的实现上（`4931b8d`）：

- T1：`restoring 5000 RSS entries without customOrder for a user with 40000 sources took 11.54s (busy_timeout 5s); another user's write that
  waited for it returned after 5.198s with database is locked`（另一次运行同样失败，整个测试 12.79 s）。
- T2：四条 `MAX(custom_order)` 查询（新建 1、导入 1、恢复 2）计划都是 `SEARCH rss_sources USING INDEX idx_rss_sources_user_id (user_id=?)`。
  第一版测试只注册了 gorm 的 Query 回调，没有捕获到 `Select(...).Scan(...)` 走的 Row 回调（“no MAX(custom_order) query was captured”），
  提交前改为两种回调都注册。
- T3：fresh 与 historical 都缺 `idx_rss_sources_user_custom_order`；historical 在索引断言之前的行内容、表结构语句与 `integrity_check` 检查都通过。
- C1 与既有的 RSS、备份、恢复接口测试都通过。

`7e1a267` 后同一断言全部通过：T1 整个测试 0.38–0.39 s（含写入 4 万行，`-count=3`）；T2 四条计划都是
`SEARCH rss_sources USING COVERING INDEX idx_rss_sources_user_custom_order (user_id=?)`；T3 通过；C1 编号与实现前相同；URL 索引切片的 T1–T4 仍通过。

H1 本地验证（不构建镜像）：用 `7e1a267` 的夹具工具生成历史卷（两个新索引都已去掉、只有 `idx_rss_sources_user_id`），本机编译的后端二进制挂该卷启动，
`/api/health` 正常，smoke 的 `assert_historical_rss_rows` 两个用户都通过，反向检查按预期失败；停止后三个索引都在
（`idx_rss_sources_user_custom_order` 列为 `user_id`、`custom_order`），三行与排序值不变，`integrity_check` 为 `ok`。

回归（`7e1a267`）：

- 后端：`go vet ./...` 通过，gofmt 无输出；`go test ./...` 31 个包通过（api 121.2 s）；`go test -race ./db ./services/...` 25 个包通过、
  无数据竞争；`go test -race -timeout 40m ./api` 通过、无数据竞争（1122.8 s）。
- 前端未改：`npm test` 877/877、`npm run build` 通过。
- 发布门自测：三个公告门 138/138，浏览器 smoke 自测 10/10；现场前端扫描 `pass`、0 个发现。
- **现场 Go 公告扫描失败：`GO-2026-6859`**（CVE-2026-97054，2026-11-11T02:51Z 发布）：`net/http` 的 `MaxBytesReader` 超限时无同步地设置
  `Connection: close`，可能与处理器设置响应头竞争导致 HTTP/1 服务器崩溃；标准库在 Go 1.26.11 修复，本仓库为 1.26.10（47 个发现，45 个到函数级）。
  与本切片无关，main `e8f3214` 与 `2152114` 同样受影响：在升级到 Go 1.26.11 之前，任何推送 main 都会在 Actions 的 Go 公告门失败。已另提切片
  （同 `6ca7fbf5` 升级 1.26.10 的做法：`go` 指令与按 OCI index 摘要固定的 builder 镜像）。

未跑：Compose、存储 HTTP 合同、镜像内 fresh/历史卷与可移植备份门（Actions 在推送 main 时运行，历史卷门已含两个新索引）；浏览器 smoke（没有前端或
接口形状变化）。

合入：本分支叠在 `claude/rss-user-url-index`（`2152114`）之上，可一起合入。origin/main 已到 `e8f3214`；与它合并时只有
`reader-dev-openreader-gap-analysis.md` 与 `refactor-audit-matrix.md` 顶部冲突（两边都在顶部追加，保留两边、新的在前）。

状态 **implementation-complete / regression-validated**（发布被 Go 公告门挡住，见上）。分支 `claude/rss-restore-custom-order` 已推送，未合入 main，
镜像未发布，生产未访问。
