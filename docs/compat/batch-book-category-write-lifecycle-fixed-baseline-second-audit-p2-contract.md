# Batch Book Category 写入生命周期第二轮固定基准合同

审查日期：2026-08-31

状态：**aligned / regression-validated / Docker-publication-pending-verification**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审 `POST /api/books/batch` 的 `category`、`category-add` 和 `category-remove`
在 owner/category/body 校验后的 SQLite 列所有权、关系读写、请求取消、并发删除和响应/事件权威性。
BookManage 单表 UI、确认文案、200 本/200 分组 wire 上限、owner 优先级、单本分组、删书收敛和
`cache/clear-cache` 扩展各有已签收合同，不因本轮重建。

## 1. 权威源码与允许适配

固定上游：

- `web/src/components/BookManage.vue:297-343#addBookGroupMulti/removeBookGroupMulti/operateBookGroupMulti`；
- `BookController.kt:1417-1501#saveBookGroupId/addBookGroupMulti/removeBookGroupMulti`；
- `BookController.kt:1947-1977#editShelfBook`。

上游对选中书籍逐项调用 `editShelfBook`；该 helper 每次从当前用户 bookshelf namespace 按
`bookUrl` 或书名+作者重新定位现存条目，再修改当前 `group`。已删除条目不会被客户端快照重新加入。
上游 remove 对不存在 bit 使用 XOR 会反向加入；OpenReader 已签收的幂等 remove 是数据正确性修复，
本轮继续保留。

OpenReader：

- `backend/api/books.go#batchBooks/bookCategoryIDs/setBookCategories`；
- `backend/api/book_control_request_boundary_contract_test.go`、`api_test.go#TestBookMultiCategoryMembership/
  TestBatchBooksCategoryAndDelete`；
- `frontend/src/stores/bookshelf.js#batchSetCategory`、`useOverlayBookBatchActions.js`；
- `scripts/smoke/book-management-real-api-contract.mjs`。

## 2. 当前反例与合同缺口

handler 在 transaction 前验证全部 Book/Category ownership，但 category 分支仍存在三个提交边界：

1. `s.db.Transaction` 不传递 `c.Request.Context()`；decode/校验后取消仍可以替换关系、更新 Book
   并广播成功。
2. `category-add/remove` 在 transaction callback 中调用 `s.bookCategoryIDs`；该 helper 却通过 `s.db`
   另行读关系并忽略错误，不属于当前 transaction snapshot/context。
3. 关系替换后对 transaction 开始时读取的完整 `models.Book` 调用 `tx.Save`。该动作只拥有
   `category_id` 和 `book_categories`，但 `Save` 可写回 title/author/cover/intro/source/catalogue/progress-adjacent
   timestamps 等未提交列，且 UPDATE 未命中时具有 fallback insert 语义。

现有测试只顺序验证关系结果、owner/wire 上限和 UI 确认；真实浏览器也只顺序执行 add/remove。
它们不会在关系更新与 `Save` 之间改变 Book，不会取消 request，也不会让目标在二次定位后消失。

| 合同点 | 审计时行为 | 裁决 |
|---|---|---|
| 列所有权 | relation 动作最后 `Save` 整行 Book | **must-fix**：只更新 guarded `category_id` |
| 关系 snapshot | add/remove 绕过 tx/context 读取，且丢弃读错误 | **must-fix**：所有 relation 读写使用同一 tx |
| 请求取消 | category transaction 与后续 event 不感知 caller cancellation | **must-fix**：commit 前取消零 row/relation/time/event |
| 并发删除 | tx 会再读 owner 列表，但后续 `Save` 仍不得具有复活能力 | **must-fix guard**：消失目标只从 actual survivors 中省略 |
| response/event | 使用 tx 初始读取的 `updatedBooks` 快照构造 shelf items | **must-fix**：提交前重载存活 Book/relation |
| wire/UI/data | 三 action、affected/books、确认/成功文案、many-to-many、owner 上限与备份 | **aligned / regression-only** |

## 3. 稳定 API 与提交合同

- 保留 JWT、16 KiB 单 UTF-8 object、200 raw Book/Category ID、unique-positive、全量 owner/category
  校验、三 action 名、`200 {affected,books}` 和一次 array `bookshelf_update`。外用户/缺失 ID 的前置错误优先级不变。
- category 分支使用 `c.Request.Context()` 驱动一个 SQLite transaction。在任何关系/Book mutation
  前按 `user_id + requested ids` 重读存活目标；前置 owner 校验后被删除的目标保持删除，不因客户端快照失败整批。
- `category` 以请求 IDs 替换；`category-add/remove` 从同一 transaction 中当前 relation 计算，
  add 去重、remove 幂等。任何 relation 读/写错误回滚全批，不使用空数组伪装成功。
- Book 更新只提交计算后的 `category_id`，并以 `id + user_id` guard 要求目标仍存在；不得调用
  `Save`，不得写回其它 Book 列或 fallback insert。
- transaction 内重载存活 Book 及 relation，commit 后只使用该权威集合构造 `books`、
  `affected` 和 event。同批中消失的 ID 不返回/广播；全部消失时保持 `200 {affected:0,books:[]}` 且不广播。
- caller 在 commit 前取消时保持空响应，Book/BookCategory/Category、时间和 Hub 队列不变。
  未预期数据库错误使用稳定 path/value-free 500，不直接返回 SQLite `err.Error()`。

## 4. 数据、迁移与允许差异

- 不新增表、列、索引、migration marker、锁、环境变量、文件、backup member 或浏览器 key。
- 不扫描、回填或重写现有 Book/Category/BookCategory、`data/cache/library` 或
  logical/portable/Legado/WebDAV 备份。
- many-to-many relation、数字 owner ID、部分列 transaction、幂等 remove 和返回权威 shelf projection 是
  OpenReader 技术栈/数据正确性适配；上游选中集、确认、成功提示与重新加载不变。
- 回滚旧镜像读取同一 SQLite/备份格式，只会重新暴露 contextless relation 读取、整行 `Save`
  和快照 event 风险。

## 5. 必须先失败的测试

1. package-private barrier 在 relation 替换后、Book 写入前通过同一 tx 改变 title/intro/
   can-update；批量 category 只改 primary/relation，不覆盖这些列，response/event 显示当前值。
2. barrier 在同一 tx 删除一个已重读 Book；批量动作不 fallback insert，其它目标持久，
   `affected/books/event` 只含存活目标。
3. relation 读取失败不被当成空分组；整批 Book/BookCategory/updated-at 回滚且无 event。
4. relation mutation 后取消 request：空响应，整批 relation/primary/Book 列/时间/Hub 逐字节不变。
5. `category`、`category-add`、`category-remove` 分别验证单/多目标、已存在 add、缺失 remove、
   空集与 primary 投影；返回顺序和 affected 语义保持。
6. 已签收 auth/owner/body 优先级、16 KiB strict JSON、200/201 上限、外用户、BookManage
   确认/选择保持、单本 category、delete/cache actions 和多用户同步继续通过。
7. focused/race、API/full Go、vet、frontend full/build、BookManage 1440x900、390x844、360x800
   真实 API 与可信 Actions fresh/historical/portable 门通过后才可发布。

## 6. 实施结论

判定：**aligned / regression-validated**。合同 `271b545`、旧实现红测 `5b04825` 和实现 `95aa598`
按顺序落地。category durable branch 现在只在 request-context transaction 内读取和替换 relation，
以 `id + user_id` guarded update 提交 legacy primary `category_id`，不再 `Save` 完整 Book；消失目标
不会复活，relation 错误和 commit 前取消会回滚全批。transaction 内按请求顺序重载实际存活 Book
及 relation，commit 后的 `affected/books/bookshelf_update` 只投影该权威集合。

确定性测试证明了旧实现会覆盖并发 title/intro/can-update、fallback insert 已删除 Book、吞掉 relation
读取错误、在 caller 取消后仍提交，并广播 transaction 最终状态之前的快照。修复后 focused、`-race`、
Go 全量、`go vet`、frontend 742/742、Vite build、Compose config 均通过；BookManage 使用真实
Go/SQLite/API/Chromium 在 1440x900、390x844、360x800 完成 metadata edit、group set、batch
add/remove 与 delete。未新增 schema、迁移、备份成员、持久路径、环境变量、浏览器 key 或 UI 流程。
可信 Actions run `33366021370` 已由实现提交触发；其 fresh/historical/portable、多架构发布结果与
OCI digest 尚待重新读取，不以此前的 `090a643` 镜像代替本切片发布证据。
