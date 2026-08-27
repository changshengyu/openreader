# 远程 BookInfo/TOC 请求取消与陈旧提交第二轮固定基准合同

审查日期：2026-08-27

状态：**inventory-complete / tests-and-implementation-pending**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审仍从 HTTP handler 进入 contextless BookInfo + TOC 抓取的两个动作：

- `POST /api/reader/remote-sessions`；
- `POST /api/books/:id/refresh`。

搜索/Explore 本身、临时 Reader 的 GET/content/store 预算、显式远程入架、换源、手动全书架刷新、
source parser/fetcher 和可见 Reader/BookInfo 已有专项合同；不因本轮请求生命周期修复重开。

## 1. 权威源码与状态转换

### reader-dev

- `BookController.kt:139-176` 的 `getBookInfo`；
- `BookController.kt:1189-1330` 的 `saveBook`；
- `BookController.kt:1350-1415` 的 `setBookSource`；
- `web/src/views/Index.vue#toDetail/saveBook`；
- `web/src/views/Reader.vue#getCatalog`。

固定上游把一次详情/目录读取作为当前请求中的一个顺序动作。搜索/Explore 结果进入临时 Reader 不先写
书架；显式保存或刷新只在当前书籍身份上提交最终 Book/目录，已有阅读位置保持。上游没有 Go context、
SQLite row race 或服务端临时 session；这些必须做技术栈等价翻译，不能把浏览器已离开的动作变成新的
后台任务，也不能让远程工作前的旧快照覆盖后续删除、换源或编辑。

### OpenReader

- `backend/api/remote_reader.go:35-122`；
- `backend/api/books.go:1438-1520`；
- `backend/engine/source_parser.go:725-781`；
- `backend/services/remotereader`；
- `backend/services/scheduler/scheduler.go:251-338`；
- `frontend/src/views/Search.vue`、`Discover.vue`、`Reader.vue`。

## 2. 当前差异矩阵

| 合同点 | 当前行为 | 裁决 |
|---|---|---|
| 临时会话创建取消 | handler 调用 `FetchBookInfoAndTOCWithVariables`，该 wrapper 固定使用 `context.Background()` | **must-fix**：caller 离开后仍可继续抓取并创建内存 session 或写 source failure |
| 单书刷新取消 | `refreshBook` 使用同一 contextless wrapper | **must-fix**：断开/超时后仍继续 fetch、SQLite 替换、缓存剪枝和广播 |
| 远程工作后的书籍存活 | refresh 在 fetch 前读取 Book，返回后直接替换章节并 `Save(&book)` | **must-fix**：并发删除可被 `Save` 重新插入；换源/URL/变量/元数据更新可被旧快照覆盖 |
| 正常成功语义 | 完整 BookInfo + TOC、章节引用恢复、cache prune、lastCheckTime 与一次 shelf broadcast | **aligned / 保持** |
| 其它远程动作 | remote add/change-source 已使用 request context；手动书架刷新已有 stale snapshot 拒绝 | **closed / 不重开** |

## 3. 稳定 API 合同

### `POST /api/reader/remote-sessions`

- 现有 JWT、64 KiB 单 JSON、字段/变量校验、active source ownership、错误状态和成功
  `201 {id,expiresAt,book,chapters}` 保持。
- BookInfo/TOC 必须调用
  `FetchBookInfoAndTOCWithVariablesContext(c.Request.Context(), ..., nil)`。
- request context 在 fetch 前或 fetch/parser 中结束时立即返回，不写伪 400/502/201，不调用
  `recordSourceFailure`，不插入/驱逐/续期任何临时 session，也不产生 SQLite、文件或事件副作用。
- 真正 source request failure 仍返回现有安全 502 并进入 caller/source-scoped failure cache；rule error、
  空目录和 store budget 错误保持现有分类。

### `POST /api/books/:id/refresh`

- 现有 JWT、owner-first book lookup、remote-only/source lookup、成功 JSON、BookInfo/TOC parser 顺序和
  source failure 分类保持。
- request context 必须贯穿 BookInfo/TOC fetch 和 GORM transaction。取消时不写业务响应、failure row、
  Book/Chapter/Progress/Bookmark、cache/image、WebSocket 或 lastCheckTime。
- 远程工作后、任何 chapter mutation 前，事务必须按 `id + user_id` 重新读取当前 Book，并验证至少
  `source_id`、`url`、`variable`、`updated_at` 与抓取前快照一致。目标已删除或任一字段改变时，整笔
  结果按陈旧拒绝，不得复活目标、覆盖新来源/变量/元数据或触碰当前目录/缓存/位置。
- 陈旧且 caller 仍有效时返回稳定
  `409 {"error":"book changed during refresh"}`；request 已取消时保持空响应。fetch 前 missing/foreign
  book 继续 404，不通过并发差异泄露其它用户状态。
- 成功事务继续完整替换目录并恢复可映射 progress/bookmark chapter ID；只更新本动作拥有的 BookInfo、
  variable、last chapter/count 和增长时 lastCheckTime，不使用可在目标消失后 fallback insert 的全行
  `Save`。提交后才剪枝 superseded cache/image 并广播一次。

## 4. 数据、并发与回滚

- 不增加表、列、索引、migration marker、环境变量或持久锁。
- 不改变 `data/cache/library`、backup/portable/Legado/WebDAV、source/session JSON 或旧 URL。
- 临时 session 仍是进程内有界数据；取消不会为其新增 tombstone。刷新陈旧拒绝只丢弃尚未提交的远程
  结果，不回滚另一个已完成的用户动作。
- 回滚旧镜像可读取相同 SQLite/文件；本批没有不可逆数据写入。安全回滚会重新暴露后台继续和陈旧
  `Save` 风险，但不会遇到格式不兼容。

## 5. 测试先行门

旧实现上必须先证明以下用例失败：

1. 临时 session 创建在阻塞 BookInfo 或 TOC 请求开始后取消：upstream request context 必须结束，handler
   返回，session count/LRU 不变，零 source failure/SQLite/event。
2. 单书 refresh 在阻塞请求开始后取消：upstream context 结束，原 Book/Chapter/Progress/Bookmark、
   cache path、lastCheckTime 和 event 均逐字节保持。
3. refresh 抓取期间并发删除目标：迟到结果固定 409 且不能重新插入 Book/Chapter。
4. refresh 抓取期间换 source/URL/variable 或编辑 metadata：迟到结果固定 409，新状态和当前目录保持；
   不剪枝新状态引用的 cache，不广播旧 shelf item。
5. 正常刷新继续更新 BookInfo/TOC/variable、增长 lastCheckTime、恢复章节引用、剪枝 superseded cache 并
   只广播一次；真正 source failure 仍按 caller/source 记录。
6. focused/race、Go full/vet、frontend full/build、真实 HTTP/Chromium Reader/remote-reader 回归和 trusted
   Actions fresh/historical/portable gates 通过后，才可发布 amd64/arm64 OCI index。

## 6. Inventory 结论

判定：**must-fix**。当前 route scan 中只有这两个远程 BookInfo/TOC handler 仍调用 background wrapper。
临时会话合同已经要求 cancellation 不建 session/不记失败；显式单书刷新又缺少手动书架刷新已经具备的
post-fetch liveness/snapshot check。下一阶段必须先提交旧实现红测，再改 handler；本 inventory 不修改
应用或测试代码。
