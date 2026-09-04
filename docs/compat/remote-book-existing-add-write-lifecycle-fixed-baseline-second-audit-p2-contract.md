# Remote Book Existing Add 写入生命周期第二轮固定基准合同

审查日期：2026-09-04

状态：**inventory-complete / tests-and-implementation-pending**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审 `POST /api/books/remote` 已按当前用户 `bookUrl` 命中现有 Book 后的分组写入、请求取消、
并发字段/删除和 response/event 权威性。新 URL 的远程 BookInfo/TOC、临时 Reader、BookInfo/结果卡
可见动作、body/owner/source/parser 边界和单本/批量 category API 均有已签收合同，不因本轮重建。

## 1. 权威源码与允许适配

固定上游：

- `BookController.kt:1189-1325#saveBook`；
- `web/src/components/BookInfo.vue`、`Search.vue`、`Explore.vue` 的加入书架动作。

上游 `saveBook` 每次读取当前用户 bookshelf namespace，按书名+作者定位已有项，保留已有阅读位置，
再替换该位置或追加新项。OpenReader 已发布合同改用当前用户 `bookUrl` 作为远程书身份，并将新书
创建和既有 URL 幂等入架分成 `201/200`；这是 Gin/SQLite、多用户与前端收敛的允许适配。本轮不把
身份改回书名+作者，也不改变结果卡先选分组、BookInfo 直接加入的可见动作。

OpenReader：

- `backend/api/books.go#createRemoteBook/setBookCategories/broadcastBookShelfUpdate`；
- `backend/api/api_test.go#TestCreateRemoteBookReusesExistingURLAndPreservesCategoriesWithoutSelection`；
- `backend/api/book_control_request_boundary_contract_test.go`；
- `frontend/src/composables/useRemoteBookAddToShelf.js` 和 `bookInfoAddToShelf.test.mjs`；
- `docs/compat/bookinfo-fixed-baseline-second-audit-p2-contract.md`、`api-contract.md`。

## 2. 当前反例与合同缺口

handler 已在远程工作前使用 request context，并在 existing 分支传播 relation 写错误；但命中现有
Book 后仍存在三个提交边界：

1. owner URL 查询在 transaction 前返回完整 `models.Book`。显式分组先替换 relation，再对该旧快照
   调用 `tx.Save(&existing)`；category 动作因此可写回 source/url/title/author/cover/intro/variable/
   catalogue/can-update/timestamps 等不拥有的列。
2. GORM `Save` 的 UPDATE 未命中后可 fallback insert。目标在 URL 查询后被删除时，迟到分组写可能
   重建 Book，且不会恢复原 chapters/progress/bookmarks/candidates，形成残缺书架行。
3. 成功 response/event 直接使用 transaction 前的 `existing`。即使 transaction 中或并发路径保存了
   更新字段，客户端仍可能收到旧 Book 投影。现有测试只顺序验证 200、无重复、分组保留和 relation
   trigger rollback，不能触发上述状态转换。

| 合同点 | 审计时行为 | 裁决 |
|---|---|---|
| 列所有权 | 显式分组最后 `Save` 完整 Book | **must-fix**：只更新 guarded `category_id` 与 relation |
| 并发字段 | transaction 使用 URL 查询时的旧 Book | **must-fix**：transaction 内重读 owner target，不覆盖其它列 |
| 并发删除 | `Save` 可 fallback insert | **must-fix guard**：删除保持删除且不广播成功 |
| 取消 | transaction 已绑定 request context | **aligned / regression**：补确定性 commit 前取消证明 |
| response/event | 使用 pre-transaction Book | **must-fix**：成功前重载 Book/relation 权威投影 |
| wire/UI/source/body | 200/201、URL identity、分组省略语义和前端动作已签收 | **aligned / regression-only** |

## 3. 稳定 API 与提交合同

- 保留 JWT、1 MiB actual-read 单 UTF-8 object、字段/variable/category 上限、active caller source
  校验优先级、当前用户 URL identity、错误 envelope 和新 URL `201` 路径。
- existing URL 请求无正分组选择时保持 `200`，不清空现有 relation/primary，不推进 `updated_at`；
  返回当前 shelf projection。现有前端 upsert/成功提示与 scoped shelf event 行为不改变。
- existing URL 请求带显式正分组时，在一个 request-context SQLite transaction 内按 `id + user_id`
  重读当前 Book，只以请求 ID 集替换 caller relation，并只更新 legacy `books.category_id`。不得
  `Save` 完整 Book，不得修改 source/url/metadata/variable/catalogue/can-update/creation 字段。
- 目标在 transaction 重读或 guarded update 时已消失，删除必须获胜：不得创建 Book/relation/chapter/
  candidate/progress/bookmark 或 event，返回 owner-safe `404 book not found`，不得转入新书远程抓取。
- relation 读写/Book 更新/重载失败回滚整次 existing mutation，返回稳定 path/value-free 500；不得输出
  SQLite text、URL、source rule、variable、host path 或请求正文。
- transaction 内重载成功 Book 与 relation；commit 后 response 和唯一 `bookshelf_update` 使用该当前
  projection。caller 在 commit 前取消时保持空响应及零 row/relation/time/event side effect。

## 4. 数据、迁移与允许差异

- 不新增表、列、索引、migration marker、锁、环境变量、文件、backup member 或 browser key。
- 不扫描、回填或重写既有 Book/Category/BookCategory/Chapter/Progress/Bookmark/Candidate、
  `data/cache/library` 或 logical/portable/Legado/WebDAV 备份。
- URL identity、numeric owner、many-to-many、request-context transaction、guarded partial SQL 和稳定
  404/500 是 OpenReader 技术栈/数据完整性适配；上游当前用户 shelf 定位、重复入架不重复、可见动作
  和成功后当前书架结果保持。
- 回滚旧镜像读取同一 SQLite/备份格式，只会重新暴露 existing branch 的 stale full-row `Save`、
  fallback insert 与 stale event 风险。

## 5. 必须先失败的测试

1. package-private barrier 在 existing URL 命中后、relation/Book commit 前通过同一 tx 修改
   title/intro/can-update/source-adjacent 非拥有列；请求只替换分组并保留这些当前值。
2. barrier 删除已重读 Book 及 relation；请求返回 owner-safe 404，不 fallback insert，不产生残缺 Book
   或 event。
3. relation 写后取消 request：空响应，Book/BookCategory/updated-at/Hub 逐字节不变。
4. barrier 在 Book category 写后更新当前 metadata；response/event 必须投影重载值，而不是初始快照。
5. relation trigger/查询/重载错误回滚 primary 与 relation，返回稳定 500 且无 raw SQLite 文本/event。
6. 既有顺序合同继续覆盖：显式多分组精确替换；省略/空 `categoryIds` 保留已有分组和时间；相同 URL
   不重复；另一用户同 URL 不可见；active source/body/category/variable 错误优先级不变。
7. focused/race、API/full Go、vet、frontend full/build、BookInfo/Search/Explore 三视口真实 API 与可信
   Actions fresh/historical/portable 门通过后才可发布。

## 6. Inventory 结论

判定：**must-fix**。`POST /api/books/remote` 的新书远程工作和公开边界已签收，但 existing URL
category branch 仍把 pre-transaction Book 以全行 `Save` 提交，拥有超出请求的列并可复活删除目标。
下一阶段先以确定性 barrier 固定列覆盖、删除复活、取消和 stale projection，再改为 request-context
重读、guarded `category_id`/relation 写入与权威重载；本 inventory 不修改应用或测试代码。
