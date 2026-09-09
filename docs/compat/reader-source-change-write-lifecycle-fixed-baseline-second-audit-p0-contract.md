# Reader 换源写入生命周期第二轮固定基准合同（P0）

审查日期：2026-09-09

状态：**inventory-complete / tests-and-implementation-pending**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审 `POST /api/books/:id/change-source` 从远程 BookInfo/TOC 工作完成到 Book、Chapter、
ReadingProgress、Bookmark、候选缓存和事件提交的生命周期。已发布的候选 available/refresh/search、
Reader 面板、正文位置恢复、parser、source ownership/COW、抓取预算和章节正文请求生命周期保持关闭。

## 1. 权威源码

### reader-dev

- `web/src/components/BookSource.vue:148-181#changeBookSource`
- `web/src/views/Reader.vue:920-925#changeBookSource`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:1331-1414#setBookSource`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:1780-1822#getLocalChapterList`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:1924-1941#saveShelfBookLatestChapter`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:1947-1977#editShelfBook`

固定上游先按旧 `bookUrl` 读取书架书并抓取目标来源。写入来源字段时，`editShelfBook` 再读取当前
namespace 的书架数组、定位仍存在的条目，只修改 origin/originName/bookUrl/tocUrl 和必要的 cover；目录
完成后再次通过 `editShelfBook` 更新 latest chapter/count。它不会把远程工作前的整本 Book 快照写回，
也不会在当前条目已经消失时重新加入该书。

### OpenReader

- `backend/api/server.go` 的 `POST /api/books/:id/change-source`
- `backend/api/books.go#changeBookSource`
- `backend/api/book_cleanup.go#replaceBookChapterRows`
- `backend/services/bookcatalog/catalog.go#ReplaceChapterRows`
- `backend/services/sourcecandidates/service.go#SeedCurrent`
- `frontend/src/composables/useBookSourceChange.js`
- `frontend/src/composables/useReaderCatalogActions.js`

OpenReader 的 SQLite transaction、many-to-many 分组、Chapter ID 重建与 Progress/Bookmark 重绑是允许的
技术栈适配，但必须以提交时仍有效的 caller Book 和目标 Source 为边界。

## 2. 当前差异矩阵

| 合同点 | 固定上游 / 已签收语义 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 远程工作后的目标存活 | `editShelfBook` 写入时重新定位现存书架项；不存在则不重新添加 | fetch 后不重读 Book；`ReplaceChapterRows` 不验证父 Book，随后 `tx.Save(&book)` 可 fallback insert | **P0 must-fix** |
| 两次换源/当前来源变化 | 写入作用于当时仍存在的当前书架实体，Reader 最终重载书架/目录 | 不比较初始 source ID/URL；较早请求可在较新换源后覆盖 Book 与目录 | **P0 must-fix** |
| 目标 Source 有效性 | `setBookSource` 从当前 namespace 解析书源 | 只在 fetch 前 `FindActive`；fetch 后不复验 association、enabled 或 source semantic snapshot | **P2 must-fix** |
| Book 列所有权 | 上游两次 current-row edit 只改来源和最新目录字段 | `tx.Save` 写回 CategoryID、CustomCoverURL、CanUpdate、local/archive 字段、时间和全部 metadata | **P2 must-fix** |
| 目录/位置原子性 | 成功后 Reader 重载新目录；现有位置语义保持 | Chapter replacement、Progress/Bookmark 重绑和 Book/candidate 在同一 transaction | **aligned / preserve** |
| 响应与事件 | 前端成功后重拉 shelf，最终以当前持久项为准 | response/event 使用 fetch 前构造并 `Save` 的 Book，而不是 commit 后权威重载 | **must-fix** |
| 取消、抓取与可见 UX | 失败不切源；成功关闭面板并恢复位置 | fetch 和 transaction 已带 request context，错误/候选/位置已有专项合同 | **closed / preserve** |

## 3. 请求与远程工作合同

1. 路径、JWT、owner-first 404、1 MiB 单 JSON object、现有字段长度、`sourceId`/`bookUrl` 输入和正常
   parser/source 错误保持。
2. 开始远程工作时冻结 caller、Book ID/source ID/URL，以及目标 BookSource 的 active association 和
   parser/fetch semantic snapshot。目标 URL 仍来自请求候选，空 URL 兼容回退到当前 Book URL。
3. 远程 fetch 使用 caller context。取消或 deadline 不合成响应，不写 Book/Chapter/candidate/event，也
   不记 source failure；真实 fetch/parser 错误保持现有安全 `400` envelope 和 caller-scoped failure cache。
4. 远程结果只是一份待提交计划，不得在 fetch 返回时被视为当前书、当前来源或当前目录。

## 4. 提交资格与列所有权

任何 Chapter 删除/创建、Progress/Bookmark 重绑或 Book/candidate 写入前，同一个 request-context
transaction 必须：

1. 重读 `id + user_id` Book。已删除目标或当前 `source_id/url` 不再等于请求初始快照时，整个结果陈旧。
2. 重读 caller 的非 detached 目标 `UserBookSource` association 及 BookSource。目标被删除、禁用、COW
   重映射，或 BaseURL/SearchURL/BookURLPattern/SourceType/Charset/Header/LoginURL/LoginCheckJS/Rules 等
   fetch/parser 语义变化时，整个结果陈旧。
3. 陈旧检测必须发生在 `replaceBookChapterRows` 前；任何 guard 未命中都回滚整笔目录/位置/candidate
   变化，不清理旧 cache/image，不广播。
4. 以 transaction 内当前 Book 为合并基底，并只显式更新换源拥有列：`source_id/type/url/variable`、
   现有已签收 metadata merge 的 `title/author/cover_url/intro/kind/word_count`，以及
   `last_chapter/chapter_count/last_check_time`。不得写 `category_id/custom_cover_url/can_update`、本地归档
   字段、created time 或其它完整快照。
5. Book update 必须带 owner + 初始 source identity guard 并验证 `RowsAffected == 1`。随后在 transaction
   内重载权威 Book，再以该 Book 和已复验的目标 Source 播种 current candidate。

两次并发换源时，先成功改变 source identity 的事务获胜；迟到事务安全冲突，不得把较新的来源、目录
或变量改回。并发普通分组、CustomCoverURL、CanUpdate 等非换源列不使操作失败，也不得被覆盖。metadata
仍按已发布的 canRename/first-nonblank 规则，以 transaction 内当前值为 fallback，避免旧 snapshot 回写。

## 5. API、文件与可见状态

- 正常成功保持 `200` shelf-book object；Chapter 全量替换、Progress/Bookmark 重绑、候选 current 投影、
  lastCheckTime 和一次 durable-only `bookshelf_update` 保持。
- 初始 missing/foreign Book 保持 owner-safe `404`；初始 unavailable target Source 保持现有 `400`。
- fetch 后 Book/source snapshot 失效返回路径和值无关的
  `409 {"error":"book changed during source switch"}`。数据库/事务失败保持安全 `500`。
- 只有成功 commit 返回的 superseded cache path 才可在事务后按现有全局引用规则清理；陈旧、取消或失败
  不得删除旧 cache、章节图片或任何本地归档文件。
- success response、candidate 和 WebSocket payload 必须来自 commit 后权威 Book；不得投影请求开始时的
  CategoryID、CustomCoverURL、CanUpdate、metadata 或时间快照。
- 前端现有 changingSource 串行、409 error 路径、成功关闭面板、清 browser cache、重载 catalogue 和位置
  恢复保持；本轮不新增按钮、提示、route 或本地存储键。

## 6. 数据、迁移与允许差异

- 不增加/删除表、列、索引、migration marker，不扫描或重写旧 SQLite。
- `data/`、`cache/`、`library/`、章节 cache hash、Book/Chapter/Progress/Bookmark/candidate 格式、普通/
  portable/Legado/WebDAV backup 和环境变量不变。
- SQLite transaction、owner/source snapshot guard、409 stale conflict、显式列更新和权威响应重载是
  多用户/并发安全适配；不得改变固定上游“只对仍存在的当前书架项完成换源”的最终结果。
- 回滚旧镜像继续读取同一数据，只会重新引入迟到换源覆盖、全行 Save 和删除后复活风险。

## 7. 测试先行门

实现前用确定性 barrier 在旧实现上证明：

1. fetch 后删除 Book：请求为安全 409，Book/Chapter/Progress/Bookmark/candidate 不复活、不新增，旧 cache
   不删且无 bookshelf event。
2. fetch 后第二次换源或直接改变当前 source ID/URL：旧请求 409，较新 Book/catalogue/variable/candidate
   完整保留。
3. fetch 后编辑/禁用/删除/COW 重映射目标 Source：请求 409，零 Book/catalogue/candidate/failure/event
   副作用。
4. fetch 后并发修改 CategoryID/relation、CustomCoverURL、CanUpdate：换源正常成功并逐字段保留并发值；
   response/event 使用相同权威 projection。
5. transaction 中 candidate seed 或 guarded Book update 失败：Chapter、Progress、Bookmark、Book 和
   candidate 全部回滚，旧 cache 不清理且无 event。
6. fetch 后 caller cancellation：零持久/文件/event/failure 副作用。正常换源仍保持 metadata merge、
   catalogue replacement、位置重绑、candidate current 和 cache/image 清理。
7. 实现后运行 focused/race、相邻 source-switch/source-COW/catalogue/cache tests、Go full/vet、frontend
   full/build、Compose，以及 1440x900、390x844、360x800、1024x1366 延迟换源真实浏览器门。
8. 同步 GitHub 后由可信 Actions 完成 fresh/historical/portable/published-platform 与 GHCR digest 证据。

## 8. Inventory 结论

判定：**must-fix**。当前测试证明普通成功、fetch 失败、目录/位置重绑、缓存清理、候选和可见位置，
没有在 fetch 与 transaction 之间删除/编辑 Book 或 Source，也没有证明 full-row `Save` 不覆盖并发字段。
下一阶段先提交本合同，再提交旧实现红测，最后实施 current-row/source-snapshot validation、owned-column
guarded update 和 authoritative reload；本 inventory 不修改应用或测试代码。
