# Bookmark 写入与并发删除边界第二轮固定基准合同（P2）

状态：**inventory-complete / implementation-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只审查现有 Bookmark REST 写入口的 wire、字段所有权、批量预算和并发删除语义：

- `POST /api/books/:id/bookmarks`
- `POST /api/books/:id/bookmarks/batch`
- `PUT /api/bookmarks/:id`
- `POST /api/books/:id/bookmarks/batch-delete`

无 body 的单项删除、列表、管理器几何、表单文案、Reader 定位、WebSocket recipient 和备份恢复继续由
[`bookmark-fixed-baseline-second-audit-p2-contract.md`](bookmark-fixed-baseline-second-audit-p2-contract.md)
约束。本合同不得借请求边界重开已经发布的 UI 或改变独立多书签身份。

## 1. 权威文件与动作映射

固定上游：

- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `saveBookmark`、`saveBookmarks`、
  `deleteBookmark`、`deleteBookmarks` 路由；
- `src/main/java/com/htmake/reader/api/controller/BookmarkController.kt`；
- `src/main/java/io/legado/app/data/entities/Bookmark.kt`；
- `web/src/components/Bookmark.vue`、`BookmarkForm.vue` 与 `web/src/views/Reader.vue`。

OpenReader：

- `backend/api/server.go`、`bookmarks.go`、`request_body.go`；
- `backend/models/models.go#Bookmark`；
- `frontend/src/api/books.js`、`components/overlays/OverlayBookmarks.vue`、
  `OverlayBookmarkForm.vue`、`composables/useOverlayBookmarkActions.js`；
- `backend/api/bookmarks_contract_test.go` 与现有 Bookmark 前端合同。

固定上游把每个用户 namespace 的书签 JSON 作为整表读写，并以书名/作者识别一条记录。OpenReader 已发布
的 JWT、caller-owned Book/Chapter/Bookmark、SQLite ID、多书签、事务和一次 post-commit event 是明确的
多用户/数据安全适配，继续保持。上游没有可直接移植的 HTTP body 或数组预算；这些预算必须作为 Go
服务的窄安全边界加入，而不能改变可见书签流程。

## 2. 当前证据与裁决

2026-08-12 对 `OpenReader@a747cd4` 的源码与现有合同复审确认：

| 合同点 | 当前行为 | 裁决 |
|---|---|---|
| 四个 JSON body | 直接 `ShouldBindJSON`，没有 declared/actual-read 上限，并只消费第一个 JSON 值。 | **must-fix security adaptation**：按入口实施 actual-read 上限并只接受一个 JSON 文档。 |
| 单项 create | DTO 已阻止客户端写 `id/userId/bookId/timestamps`，并验证 context、chapter owner 和字段长度。 | **aligned**：保留公开位置字段与服务端身份所有权；未知兼容字段继续忽略。 |
| 批量 create | 先验证全部行再事务写入，但数组没有原始基数或总 body 上限。 | **must-fix work boundary**：最多 2,000 个原始条目且总 wire 最多 16 MiB。 |
| note update 请求 | 后端实际只读取 `note`，但空对象或未知字段会把备注清空；前端编辑仍发送完整位置/context 快照。 | **must-fix field ownership**：必须显式提交 `note`；前端编辑只发送该字段。 |
| note update SQL | 读取完整行后 `Save(&bookmark)`，把旧位置/context/timestamps 整行回写；目标在读取后被删除时，GORM `Save` 还可能 fallback insert/upsert。 | **must-fix concurrency/data integrity**：只更新 caller-owned 行的 `note`，零 affected 不得重新创建。 |
| 批量删除 | 已按 caller+book+ID 过滤、按首次有效请求顺序返回并在事务后广播，但原始 ID 无上限。 | **partial / must-fix work boundary**：最多 2,000 个原始 ID；现有去重、过滤和响应保持。 |
| target 与事件 | create/batch-delete 先验证 owned book；update 先查询 caller-owned bookmark；失败零广播。 | **aligned**：owner/missing 优先级不得因 decoder 调整而改变。 |

当前 `maxBookmarkTitleBytes=320`、`maxBookmarkExcerptBytes=16 KiB`、`maxBookmarkNoteBytes=16 KiB`
只限制成功解码后的字符串，不限制 JSON whitespace、未知字段、数组数量或并发整行回写，因此不能替代
wire/cardinality/column-ownership 合同。

## 3. 授权、JSON 与精确预算

- 四个入口继续先通过 JWT。create/batch/batch-delete 在读取 body 前解析正 `bookID` 并查询 caller-owned
  Book；update 在读取 body 前解析正 `bookmarkID` 并查询 caller-owned Bookmark。非法 path 保持 `400`，
  missing/foreign target 保持 `404`，即使 body 超限或 malformed 也不得先读取 body。
- 单项 create 和 update 各只接受一个非 null JSON object，actual-read wire 上限为 `64 << 10` bytes。
  精确 64 KiB 加尾部 JSON whitespace 进入原业务校验；64 KiB + 1 返回
  `413 {"error":"request body too large"}`。
- batch create 只接受一个非 null JSON array，actual-read wire 上限为 `16 << 20` bytes；原始数组最多
  2,000 项。2,001 项为既有平面 `400 {"error":"invalid bookmarks payload"}`，发生在逐行 chapter 查询、
  SQLite 或事件前。
- batch-delete 只接受一个非 null JSON object，actual-read wire 上限为 `16 << 10` bytes；`ids` 原始数组
  最多 2,000 项。2,001 项为 `400 {"error":"invalid bookmark ids"}`，发生在去重和 bookmark 查询前。
- declared `Content-Length` 和未知长度/chunked 按实际读取使用相同边界。空 body、`null`、错误顶层类型、
  第二 JSON 或尾随非 whitespace 垃圾按各入口既有 malformed `400`；合法文档后的 whitespace 可接受。
- overflow 始终是平面 `413`，不得回显请求、书名、正文、备注、JWT、数据库文本或内部路径。不得把这些
  上限做成全局 middleware 或改变 auth/admin/settings/book/source 已发布的预算和错误 envelope。

64 KiB 可完整容纳现有单条 title/excerpt/note 最大值和 JSON 开销。16 MiB/2,000 条与已发布的批量规则
边界同量级，可覆盖浏览器 JSON 导入，同时阻止无界解码、chapter 查询和 SQLite 批量写放大。

## 4. 字段、数值与批量原子性

- create/batch 继续接受 `chapterId/chapterIndex/offset/percent/title/excerpt/note`。`id/userId/bookId`、
  created/updated timestamps 和其它服务端字段没有写权限；未知对象字段继续忽略以兼容已部署客户端。
- `chapterIndex/offset` 的负值归零，`percent` 夹在 0..1；trim 后 excerpt 必须非空；空 title 使用现有
  `书签`；字段 byte 上限、可选 chapter 必须属于当前 Book 的语义不变。
- batch 的 2,000 上限按最外层原始数组长度计算，不因无效/重复行减少。全部行和 chapter ownership 必须
  在第一次写入前通过；任一行失败不保留前缀、不推进 timestamp、不广播。
- update 请求只拥有 `note`。必须存在一个非 null JSON string `note`；显式空字符串继续表示清空备注。
  `{}`、只有未知字段、`{"note":null}` 或错误类型为 `400 {"error":"invalid bookmark payload"}`，不更新
  `updated_at`、不广播。附带未知字段可忽略，但不能让它们获得位置/context 写权限。
- `OverlayBookmarkForm` 的新增继续发送完整创建 DTO；编辑只发送 `{note}`，不能以“后端会忽略”为理由
  继续发送书、章节、offset、title 或 excerpt 快照。
- batch-delete 继续先按首次出现顺序去重正 ID，只删除 caller+current-book 实际匹配项，返回稳定
  `{deletedIds}`。外用户、另一册书和不存在 ID 继续被忽略；没有实际删除仍是 `200` 空列表且零广播。

## 5. note-only SQL 与并发删除合同

- update 的目标预查只决定现有 `404` 优先级和 BookID event payload，不能授权后续无 owner 条件写入。
- 持久化必须使用显式列更新，SQL 条件至少包含 `user_id + id`；不得使用 `Save`、全行 struct update 或
  upsert。请求中和预查快照中的 book/chapter/location/context/created time 永远不进入 `SET`。
- 如果目标在预查和 UPDATE 之间被删除，UPDATE 的零 affected 必须返回 `404`，不 fallback insert，
  不创建新 ID/旧 ID 行，不广播。并发删除成功的结果是最终无行，不能由迟到的备注请求复活。
- 如果另一个事务在预查后更新了任一非 note 列，note UPDATE 只能写 note/`updated_at`，不得覆盖较新的
  context 或位置。成功后重新读取 caller-owned fresh row 并返回，响应不能来自旧快照。
- SQL 错误保持既有安全 `500`；失败不得产生成功响应或事件。成功一次只发送一个当前用户
  `bookmarks_update kind=update`，并使用持久 fresh row。

本轮不引入全局 Bookmark mutex。正确性来自列所有权、owner 条件和 RowsAffected；不同用户、不同书签
与批量创建不应被无关请求串行化。

## 6. 数据、历史兼容与允许差异

- 不新增/删除/修改 SQLite 表、列、索引或迁移；不改 `data/cache/library`、普通/portable/WebDAV backup、
  `bookmark.json`/`bookmarks.json`、浏览器 storage key 或 Router 路径。
- 历史 oversized Bookmark 行继续可列出、导出、恢复、删除；更新 note 时只验证新提交 note，不重验或
  截断历史 title/excerpt。新 HTTP 上限不触发启动扫描、数据合并或清理。
- 继续保留 OpenReader 的独立 ID 多书签、当前书导入归属、chapter ownership、事务和 WebSocket
  recipient scope，不恢复上游按书名作者覆盖/删除首条的歧义。
- 继续保留用户要求的“添加当前段落”、上下文定位、offset/percent 快速恢复及现代浏览器本地 JSON 读取。
  本切片没有新的可见产品差异。

## 7. 失败测试先行闸门

实现前必须先让当前代码在以下合同上失败：

1. create/update 的 declared 与 chunked 64 KiB + 1 为精确 `413`；batch create 的 16 MiB + 1 和
   batch-delete 的 16 KiB + 1 同样失败。精确边界与尾部 whitespace 进入原业务状态机。
2. 四入口拒绝第二 JSON、尾随垃圾、`null` 和错误顶层类型；拒绝前后 Bookmark 行、timestamp 和事件
   不变。foreign/missing Book/Bookmark 继续先于 body 返回 `404`。
3. batch create 2,000 条进入完整验证/事务，2,001 条在 chapter/SQLite/event 前 `400`；batch-delete
   2,000 个原始 ID 保持去重过滤，2,001 个在查询/删除/event 前 `400`。
4. update 的 `{}`、unknown-only、null/wrong-type note 为 `400`；显式 `""` 可清空；客户端静态/控制器
   合同证明编辑只发送 `{note}`，新增仍发送完整创建字段。
5. SQLite trigger 在 note UPDATE 前改写 context/location 时，新值必须保留；在 UPDATE 前删除目标时，
   请求不得由 GORM fallback upsert 复活书签，最终无行且零成功事件。
6. create/batch 的 server-owned IDs/timestamps 不能写入；跨用户 chapter、bookmark 和另一册书批删继续
   隔离。批量中一行失败保持全批零写和零广播，成功批量只广播一次。
7. 现有稳定排序、Reader/manager/form 状态、上下文定位、备份/恢复、WebSocket generation 与 recipient
   scope 回归全部继续通过。

实现后运行 focused API 与 frontend 合同、focused race、`go vet ./...`、`go test ./...`、frontend 全量和
production build。隔离生产形态真实 HTTP 覆盖 declared/chunked/single-JSON/cardinality/zero-side-effect；
1440x900、390x844、360x800 的 Bookmark 编辑浏览器门直接断言 update payload 仅含 note。发布前顺序执行
fresh/historical mounted-volume 与 portable backup 门，再由本机完成 amd64/arm64 构建和 GHCR 发布。

## 8. 当前实施约束

- 复用 `decodeBoundedSingleJSON` 的 actual-read primitive，在 Bookmark handler 内映射现有平面错误；
  不修改共享 decoder 或其它已发布 endpoint 的语义。
- 为 update 使用可区分 absent/null/string 的窄 request DTO 或 raw field gate；不要把 create DTO 的零值
  语义带进 note patch。
- 批量 cardinality 必须在有界 JSON 完整解码后、任何逐项 DB 查询前检查；不能依赖前端导入文件大小、
  selection 数或 `Content-Length`。
- 并发测试必须稳定复现旧 `Save` 的整行/复活风险并证明新列更新关闭，不以时间 sleep 或全局锁制造
  偶然顺序。

本合同阶段不修改应用或测试代码。下一门是先提交失败测试，不能把既有绿测当成该 wire/并发边界已实现。
