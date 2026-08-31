# Category Patch 写入生命周期第二轮固定基准合同

审查日期：2026-08-30

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审已有 `PUT /api/categories/:id` 在 owner/body/字段校验完成后的 SQLite 列所有权、请求取消、
并发排序和并发删除生命周期。BookGroup 单表 UI、路由、16 KiB wire、80/24-byte 字段、Category 唯一名、
many-to-many、混合排序、删除非空门禁、备份和 WebSocket 类型均已由专项合同签收，不因本轮重建。

## 1. 权威源码与允许适配

固定上游：

- `web/src/components/BookGroup.vue#toggleBookGroupShow/saveBookGroup/saveOrder/deleteBookGroup`；
- `BookController.kt:1603-1654#saveBookGroup`；
- `BookController.kt:1656-1689#saveBookGroupOrder`；
- `BookController.kt:1691-1721#deleteBookGroup`；
- `YueduApi.kt:229-231`。

上游每次保存都从当前用户 `bookGroup` namespace 重新定位目标，再替换现有条目；删除后的 ID 不会被迟到
保存重新加入。OpenReader 把位掩码数组映射为多用户 SQLite `Category`，并把 UI 的完整对象保存拆成
`name/color/show` 部分 DTO，这是已签收适配。该适配要求 SQL 也只提交请求拥有的列。

OpenReader：

- `backend/api/categories.go#updateCategory/reorderCategories/deleteCategory`；
- `backend/api/book_group_write_boundary_contract_test.go`；
- `frontend/src/api/categories.js`、`stores/bookshelf.js#renameCategory/setCategoryVisible`；
- `scripts/smoke/book-group-real-api-contract.mjs`。

## 2. 当前反例与合同缺口

`updateCategory` 在 transaction 外按 `user_id + id` 读取完整 Category，修改内存快照后调用
`s.db.Save(&category)`。GORM `Save` 更新整行，并在 UPDATE 未命中时 fallback insert；调用也不传播
`c.Request.Context()`。

现有 boundary 测试只预置 oversized name/color 后顺序执行 show-only update；真实浏览器只顺序执行
create/rename/delete。它们没有让 reorder、另一个 partial update 或 delete 在第一次读取后先提交，因此
不能证明旧合同声称的“只改显式字段”。

| 合同点 | 审计时行为 | 裁决 |
|---|---|---|
| 显式列所有权 | DTO 只解析 `name/color/show`，最终 `Save` 仍写回 `sort_order`、其它未提交字段和时间快照 | **must-fix**：只更新请求显式列 |
| 并发 reorder | 读取后完成的 `sort_order` 可被迟到 rename/show `Save` 恢复为旧值 | **must-fix**：未提交列按列合并 |
| 并发删除 | 读取后删除可被迟到 `Save` fallback insert 复活 | **must-fix**：事务内复验 owner target，删除保持权威 |
| 请求取消 | decode 后数据库写入不使用 request context | **must-fix**：commit 前取消零 row/time/event |
| 空 patch | `{}` 仍执行整行 `Save` 并推进 `updated_at` | **must-fix**：200 no-op 不执行 UPDATE |
| response/event | 返回和 `category_update` 使用 transaction 前快照 | **must-fix**：成功事务内重载当前 Category |
| wire/UI/data | 既有 body、字段、唯一名、状态码、可见动作、schema/backup | **aligned / regression-only** |

## 3. 稳定 API 与提交合同

- JWT、正 ID、owner-first flat 404、16 KiB 单 non-null JSON、字段/null/UTF-8 边界、默认颜色与现有错误
  文本保持；target 仍在 body 前确认，未知字段继续忽略。
- writable 列只有请求显式非-null 提交的 `name`、`color`、`show`。未提交列不得进入 SQL update；真实
  更新继续由 GORM 维护现有 `updated_at`。
- `{}` 和只有未知/null 字段的 object 保持 200 no-op compatibility，但不执行 UPDATE、不推进
  `updated_at`、不制造 fallback insert；成功 response 仍是完整 Category。
- handler 在校验后使用 `c.Request.Context()` 驱动 transaction。任何 mutation 前必须按
  `id + user_id` 重读当前 Category；前置 lookup 后被删除时返回既有
  `404 {"error":"category not found"}`，不得复活 Category 或产生 event。
- 显式列 update 必须带 `id + user_id` guard 并要求一行仍存在。并发 reorder 或未提交字段更新按列合并，
  不引入全局锁或 snapshot 409；同一显式列保持正常 SQLite last-committed-write 顺序。
- 成功 transaction 内重载当前 Category；commit 后一次 `category_update` 和既有一次统一
  `book_groups_update` 都必须投影当前值。取消、not-found、唯一名冲突和数据库失败不广播 false success。
- caller 在 commit 前取消时保持空响应，Category/BookGroupPreference/Book/BookCategory、时间和 Hub 队列
  不变。唯一名冲突继续使用既有 409，不把 SQLite 文本、字段值、JWT 或路径放入错误。

## 4. 数据、迁移与允许差异

- 不新增表、列、索引、migration marker、锁、环境变量、文件、backup member 或浏览器 key。
- 不扫描、回填或重写现有 Category/BookGroupPreference/BookCategory/Book，`data/cache/library` 及
  logical/portable/Legado/WebDAV 备份保持。
- request-context GORM transaction、数字 owner ID、部分列 update 和统一投影是 OpenReader 技术栈适配；
  上游分组名、显示状态、顺序、成功提示和单表刷新保持。
- 回滚旧镜像读取同一数据格式，只会重新暴露 stale full-row `Save` 和 fallback insert 风险。

## 5. 必须先失败的测试

1. rename handler 读取目标后，正常 API 先完成 reorder 与 show/color partial update；迟到 rename 仍 200，
   最终 name 使用 rename 值，sort/show/color 保持并发值，response/event 是当前完整 Category。
2. show handler 读取后，正常 API 先完成 rename/color；迟到 show 只改变 show，统一 BookGroup event 不携带
   旧 name/color/order。
3. handler 读取后并发正常 DELETE 空 Category：delete 保持 204，迟到 update 为稳定 flat 404，Category
   不复活且没有 `category_update`/`book_groups_update`。
4. decode/校验后取消：空响应，Category 全行、`updated_at` 和 Hub 队列逐字节不变。
5. `{}` 与全 null/unknown object 保持 200，返回当前行但不推进 `updated_at`；显式单字段只改变该列。
6. 已签收 auth/target/body 优先级、16 KiB strict JSON、80/24-byte、默认颜色、历史 oversized show、唯一名
   冲突、mixed/category reorder、BookGroup UI/WebSocket 与多用户隔离继续通过。
7. focused/race、Go full/vet、frontend full/build、真实 API/BookGroup 1440x900、390x844、360x800 和可信
   Actions fresh/historical/portable 门通过后，才可发布 amd64/arm64 OCI index。

## 6. 实施与验证结论

审计判定的 **must-fix** 已关闭。合同 `835f950`、旧实现红测 `73b1655`、实现 `92c3ae7`、
浏览器夹具勘误 `4b7d010` 与多架构构建稳定性修复 `090a643` 按阶段落地。`updateCategory`
现在使用 request-context transaction，在任何 mutation 前重读 owner Category，并只更新请求显式提交的
`name/color/show`。空 patch 不执行 UPDATE，取消不落库/广播，并发删除不复活目标，并发排序与未提交列
按列合并；成功响应和两类事件均使用 transaction 内重载的当前 Category。

focused、adjacent、race、API/full Go、vet、frontend 742/742、Vite build 与 Compose 均通过。真实
Go/SQLite/Chromium BookGroup 合同在 1440x900、390x844、360x800、1024x1366 和 1366x1024 通过，
覆盖双客户端同步、创建/改名/删除、显隐、混合排序和精确几何。

可信 Actions run `33361011263` 通过 backend/frontend/Compose、native image、fresh portable、historical
volume 和 published-platform 门，发布 `090a643`/`latest` OCI index
`sha256:0e0532f202ab0090005fd07642e61b551febf0b3a1c44e518fe2bfbf9df1875f`。amd64 manifest 为
`sha256:720bb45e323e26d83ba4d4f1989903ac1296eb8e097a552aa684dc52a0ca58fe`，arm64 manifest 为
`sha256:1b574767ff4fcdd453963fbd9848b0a31ff00530b525b2844331df72f50ab861`。回拉不可变标签后，
OCI revision 和 `/api/health` 均确认完整提交 `090a643051368afd910d3ce1f3aec19cd9b697a5`。
