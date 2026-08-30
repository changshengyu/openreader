# Book Patch 与分组写入生命周期第二轮固定基准合同

审查日期：2026-08-27

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审已有 `PUT /api/books/:id` 与 `PUT /api/books/:id/category` 在 body/owner 校验完成后的
SQLite 列所有权、请求取消和并发删除生命周期。BookInfo、BookManage、BookGroup 的可见入口与文案，
1 MiB/16 KiB wire 边界、字段长度、Category ownership、many-to-many 投影、上传 capability、备份和
本地 archive 已由专项合同签收，不因本轮重建。

## 1. 权威源码与状态转换

固定上游：

- `web/src/views/Index.vue#saveBook`、`components/BookInfo.vue#saveBook`；
- `BookController.kt:1189-1326#saveBook`；
- `BookController.kt:1417-1505#saveBookGroupId/addBookGroupMulti/removeBookGroupMulti`；
- `BookController.kt:1947-1977#editShelfBook`。

上游的分组、换源和局部 shelf 动作都先从当前 namespace 重新取得现有 Book，再在
`editShelfBook` handler 中修改动作拥有的字段；目标已不存在时不会重新添加。OpenReader 已把上游宽
Book 保存拆成结构化 metadata patch、追更单字段 patch 和 Category many-to-many transaction，这是已
签收的多用户/SQLite 安全适配。该拆分只有在数据库也执行显式列更新时才成立。

OpenReader：

- `backend/api/books.go#updateBook/updateBookCategory`；
- `backend/api/book_write_boundary.go`、`book_group_write_boundary.go`；
- `backend/api/book_edit_metadata_contract_test.go`；
- `frontend/src/composables/useOverlayBookInfo.js`、`useOverlayBookGroups.js`、`stores/bookshelf.js`。

## 2. 当前反例与合同缺口

两个 handler 都在 transaction 前通过 `ensureBook` 读取完整 Book，随后修改内存快照并调用
`tx.Save(&book)`。GORM `Save` 会更新整行，目标在读取后被删除时还可 fallback insert。两个 transaction
也没有使用 `c.Request.Context()`。

已有 `TestBookMetadataPatchPreservesConcurrentShelfFields` 只在请求开始前预置分组和 `canUpdate`，然后
顺序执行一次 metadata patch；它没有在 handler 读取 Book 后并发提交，因此不能证明已签收的“编辑期间
并发保存保持”。真实浏览器旧记录同样只证明客户端 payload 精确和前置状态保持，不证明 SQLite read-
modify-save race。

| 合同点 | 审计时行为 | 裁决 |
|---|---|---|
| metadata 列所有权 | request 只解析显式字段，但最终 `Save` 写回读取时的所有 Book 列 | **must-fix**：只更新显式 metadata/category/can-update 列 |
| 独立分组列所有权 | many-to-many relation 正确替换，随后 `Save` 写回整行 Book | **must-fix**：只同步 legacy `category_id`，不得覆盖 metadata/追更/source/catalogue |
| 并发删除 | read 后 delete 可在迟到 `Save` 时 fallback insert；分组 transaction 还可能留下 relation | **must-fix**：事务内复验 owner target，删除保持权威且无孤儿 relation |
| 请求取消 | decode 后 transaction 不使用 request context | **must-fix**：取消在 commit 前被观察时不得写 row/relation/event |
| 返回投影 | response 使用 transaction 前的内存 Book | **must-fix**：成功事务内重载当前 Book，返回/广播合并后的权威 shelf item |
| wire/UI/data | body、字段、Category owner、状态码、可见动作、schema/backup 已签收 | **aligned / regression-only** |

## 3. 稳定 API 合同

### `PUT /api/books/:id`

- JWT、正 ID、owner-first 404、1 MiB 单 non-null JSON、字段/null/UTF-8 边界和现有平面错误保持。
- writable 列仍只有请求显式提交的 `title`、`author`、`cover_url`、`custom_cover_url`、`intro`、
  `category_id`、`can_update`；未知及 server-owned 字段忽略。未提交列不得进入 SQL update。
- 显式 `categoryId/categoryIds` 继续与 relation replacement 在一个 transaction 内提交；没有 category
  key 时绝不改 `category_id` 或 `book_categories`。
- `{}` 保持 200 no-op compatibility，但不需要执行整行 UPDATE、推进 `updated_at` 或制造可 fallback 的
  insert。现有成功 response schema 保持。

### `PUT /api/books/:id/category`

- JWT、owner-first 404、16 KiB 单 JSON、最终有效 ID 选择/owner 校验、空选择兼容和错误保持。
- transaction 只替换当前用户该 Book 的 `book_categories`，并更新 legacy `books.category_id`；不得写回
  title/author/cover/intro/can-update/source/url/parser/catalogue/time 等其它快照列。

### 共同提交生命周期

- 两个 handler 的 transaction 使用 `c.Request.Context()`。caller 在 commit 前取消时保持空响应，且不
  开始后续 row/relation mutation、不推进时间、不广播。
- 在任何 relation 或 Book mutation 前，transaction 必须按 `id + user_id` 重读当前 Book。目标在前置
  lookup 后被删除时返回与 owner-missing 相同的稳定 `404` `NOT_FOUND` envelope；caller 已取消则空响应。
  不能复活 Book、Chapter、BookCategory、Progress、Bookmark 或其它关联状态。
- 显式列 update 必须以 `id + user_id` 为 guard，并要求一行仍存在。并发写入未提交列时采用列级合并，
  不把无关变化误判为冲突；同一显式列的正常 SQLite 提交顺序保持 last committed write。
- 成功 transaction 内重载当前 Book；commit 后使用该 Book 构造完整 shelf projection 并只广播一次。
  event 不得包含旧 metadata、旧追更或旧 primary category。
- transaction/重载失败保持既有 path-free 500，且没有 durable relation/row 或 false success event。

## 4. 数据、迁移与允许差异

- 不新增表、列、索引、migration marker、锁、环境变量、文件、backup member 或浏览器 key。
- 不扫描、回填、删除或重写现有 Book/Category/BookCategory、`data/cache/library`、logical/portable/
  Legado/WebDAV 备份和旧卷。
- GORM guarded column update、request context、数字 owner ID 和 many-to-many transaction 是 OpenReader
  技术栈适配；上游可见字段、分组结果、成功提示和 shelf replacement 仍是产品合同。
- 回滚旧镜像可读取同一数据格式，只会重新暴露 full-row `Save` 覆盖与 fallback insert 风险。

## 5. 必须先失败的测试

1. metadata handler 读取目标后，并发正常 API 保存 `canUpdate` 与分组，再完成 title/author/intro patch：
   所有动作 200，最终值按列合并，response/event 是当前完整 shelf projection。
2. category handler 读取目标后，并发正常 API 保存 title/intro/canUpdate，再完成分组：metadata/追更保持，
   legacy primary 与 many-to-many relation 一致，event 不携带旧快照。
3. 两个 handler 分别在 read 后并发删除目标：delete 保持成功，迟到动作稳定 404，不能 fallback insert
   Book 或留下 BookCategory/Chapter/Progress/Bookmark，不能广播 shelf update。
4. 两个 handler 分别在 decode/校验后取消：空响应，Book/relation/updated-at/Hub 队列逐字节不变。
5. `{}` metadata patch 保持 200，但不推进 `updated_at`；显式单字段 patch 只改变该列。相同字段的顺序
   更新仍按最后提交值，不引入全局锁或 snapshot 409。
6. 已签收 1 MiB/16 KiB、strict JSON、target/body priority、字段 byte、custom cover、Category owner、
   空/单/多分组、BookInfo/BookManage/BookGroup/Reader 消费者和多用户错误语义继续通过。
7. focused/race、Go full/vet、frontend full/build、真实 API/BookInfo/BookManage 三视口和 trusted Actions
   fresh/historical/portable 门通过后，才可发布 amd64/arm64 OCI index。

## 6. 实施与验证结论

审计判定的 **must-fix** 已关闭。合同 `8e1a2e4`、旧实现红测 `946df03`、既有 404 envelope 勘误
`7aad4b7`、实现 `4b0a599` 和 BookInfo 聚焦浏览器合同 `0edbbb5` 按阶段顺序落地。两个 handler 现使用
request-context transaction，在任何 mutation 前重读 owner Book；metadata 只更新请求显式列，category
只更新 guarded `category_id` 与 caller-scoped relation，成功后重载当前 Book 再响应/广播。取消不落库，
并发删除不复活目标，并发无关列按提交列合并，空 patch 不推进 `updated_at`。

focused、adjacent、race、API/full Go、vet、frontend 742/742、Vite build、Compose、BookManage 与 BookInfo
1440x900、390x844、360x800 真实 Go/SQLite/Chromium 均通过。可信 Actions run `33308641504` 又通过
native、fresh portable、historical volume 和 published-platform 门，发布 `4b0a599`/`latest` OCI index
`sha256:03158e390e967f6ef4f6addc9125de504bdd781aa81e85ed5aaa403d58ead0fd`。回拉不可变标签后，OCI revision
和 `/api/health` 均确认完整提交 `4b0a599d1bd773a78f7864a34ae5b784ff7af259`。
