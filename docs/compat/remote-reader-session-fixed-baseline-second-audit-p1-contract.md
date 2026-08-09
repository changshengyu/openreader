# 搜索/探索临时阅读会话固定上游第二轮合同（P1）

状态：`inventory-complete / implementation-pending`

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本轮只提取合同并映射现状，不修改应用或测试代码。该模块负责“搜索/探索结果未加入书架也可进入 Reader”的完整生命周期；现有 Go 会话结构不是正确性的依据。

## 1. 上游权威链路

| 层 | 固定上游证据 | 产品合同 |
|---|---|---|
| 进入 Reader | `web/src/views/Index.vue#toDetail` | 搜索结果只写浏览器内存中的 `readingBook` 并进入 `/reader?search=1`；不要求先加入书架，也不触发 `saveBook`。缺少 `bookUrl` 时停留原处。 |
| 目录 | `web/src/views/Reader.vue#getCatalog`、`BookController.kt#getChapterList` | 临时书把 `bookSourceUrl=readingBook.origin` 带给目录动作。后端优先书架书；未入架时从 BookInfo 缓存或调用方书源重建书籍信息，再取得目录。 |
| 正文 | `web/src/App.vue#getBookContent`、`BookController.kt#getBookContent` | Reader 只提交书籍标识和章节序号；服务端从目录确定章节，不信任浏览器提供的正文 URL。前端请求超时为 30 秒。 |
| 持久化边界 | `App.vue#isInShelf`、`BookController.kt#saveBookProgress` | 只有显式“加入书架”才保存书籍。未入架书保存进度会返回“书籍未加入书架”；临时阅读本身不得产生书架、进度、书签或分组记录。 |
| 变量 | `WebBook.kt#getChapterList/getBookContent`、`AnalyzeUrl.kt#put/get`、`AnalyzeRule.kt#put/get` | BookInfo/目录阶段的 `@put` 写书籍变量；目录项可拥有各自章节变量。正文 `@get` 按“当前章节 → 书籍”读取，正文 `@put` 只写当前章节。一个章节的变量不能覆盖另一章节。 |

上游没有独立服务端 session：生命周期由每个浏览器标签页的 Vuex `readingBook` 和缓存承担。OpenReader 使用用户绑定的高熵服务端会话，是 Vue 3/Pinia、JWT 多用户与隐藏书源凭证所需的技术栈等价适配；它不能扩张为新的持久化实体。

## 2. 当前映射

| 合同层 | OpenReader 映射 | 第二轮裁决 |
|---|---|---|
| Search/Explore → Reader | `Search.vue`、`Discover.vue` → `POST /api/reader/remote-sessions` → `/reader/remote/:sessionId` | **技术栈等价**。创建会话与显式 `POST /api/books/remote` 相互独立。 |
| 会话所有权 | `backend/api/remote_reader.go` 的随机 32-byte ID、`UserID` 检查、`Cache-Control: no-store` | **已复核一致**。未知 ID 和其他用户 ID 均为 404；到期为 410，不得误报 JWT 失效。 |
| 临时数据边界 | 内存 `remoteReaderSessionStore`，Reader 临时分支不调用 shelf ID API | **已复核一致**。不得写 Book、Chapter、ReadingProgress、Bookmark、分类、缓存文件、备份/WebDAV或 WebSocket shelf 事件。 |
| 服务端权威取章 | content 路由只接受 session ID 和 index；URL、书源快照、下一章 URL 均取自服务端会话 | **已复核一致**。客户端不得覆盖 source、header、cookie、chapter URL 或 next URL。 |
| 变量状态 | 创建保存 Book/Chapter variable；正文成功后回写 book 与当前 chapter variable | **已复核一致，但缺测试**。必须用至少两个章节证明章节变量隔离、书籍变量跨章可见和失败/取消不提交变量。 |
| 取消 | 正文 fetch 使用 `c.Request.Context()`；取消分支不构造成功响应 | **实现方向正确，缺合同测试**。取消后不得继续解析、回写变量、记录 source failure 或产生持久副作用。 |
| 错误脱敏 | `writeSourceError` 与共享 fetcher 的 typed/redacted error | **实现方向正确，缺合同测试**。响应和日志不得泄露规则、header、cookie、代理凭证或带 query/fragment 的远端 URL。 |
| 请求体预算 | 创建接口直接 `ShouldBindJSON` | **必须修复**：在 JSON 解码前加硬上限，超限返回安全的 `413`；不得先访问书源。 |
| 会话内存预算 | 当前 map 只清理到期条目，无个体/用户/进程预算 | **必须修复**：认证用户可反复创建会话并保留大目录，形成内存 DoS。 |
| 非法章节索引 | 当前先 `get()` 续期，再解析 index | **必须修复**：格式错误或负数索引不得延长 idle lease；先校验 index，再触碰会话。 |
| 显式关闭 | 当前无 DELETE；依靠 TTL | **允许差异/无需新增**：固定上游也没有服务端关闭动作。到期清理和受限 LRU 足够；浏览器卸载不建立不可靠的必达合同。 |

## 3. 稳定 API 合同

### `POST /api/reader/remote-sessions`

- JWT 必需；body 在解码前限制为 **64 KiB**。超限返回 `413 {"error":"remote reader payload too large"}`，且书源 transport 调用次数必须为 0。
- `{sourceId,bookUrl,title}` 必填。`variable` 继续受 `32` 项、单 key `128` bytes、单 value `4096` bytes、合计 `16 KiB` 的现有规则约束。
- 只可解析调用者当前 active 且 enabled 的 source。`sourceName` 不参与授权。
- 成功返回 `201 {id,expiresAt,book,chapters}` 和 `Cache-Control: no-store`。`book.id=0`。
- 解析或 transport 错误继续使用安全的 `502 {error,code?,stage:"book_info"}`；不得把原始错误、规则或请求 URL 拼入 `error`。
- 成功与失败都不得写任何 shelf 数据或同步事件。仅明确的 source request failure 可以进入当前用户的短时 failure cache；规则错误与取消不得进入。

### `GET /api/reader/remote-sessions/:id`

- 成功返回该用户原始会话投影并续期 idle lease；`Cache-Control: no-store`。
- 未知、已被预算驱逐或属于其他用户的 ID 返回相同的 `404`；自然到期返回 `410`。
- idle TTL 保持 30 分钟；absolute TTL 保持 4 小时。任何续期都不得超过 absolute deadline。

### `GET /api/reader/remote-sessions/:id/chapters/:index/content`

- 必须先把 `index` 校验为非负整数，非法值直接 `400`，不查询或续期会话。
- 成功查询会话后，只按目录项的稳定 `chapter.index` 取章；不存在为 `404`。
- 正文成功后才原子提交变量：BookInfo/TOC 得到的书籍变量保留，当前章节变量更新，其他章节变量逐字节不变。
- request context 取消时立即停止，不生成伪成功/502，不提交变量、不记 source failure、不写 shelf/cache。
- 远端/解析失败为安全 `502 {error,code?,stage:"content"}`；不返回 source snapshot 或可请求凭证。

## 4. 会话保留预算

这是 OpenReader 的安全适配，不改变正常临时阅读流程：

- 单个创建请求解析完成后，待保留的 source snapshot、book 与 chapters 估算总量不得超过 **8 MiB**；超限返回安全 413，不插入 store。
- 每用户最多 **8** 个会话且估算保留量最多 **32 MiB**。
- 单进程最多 **128** 个会话且估算保留量最多 **128 MiB**。
- create/get 成功更新最近访问顺序；create 前先清理自然到期项，再按最久未访问顺序驱逐，直到同时满足用户与进程的数量/字节预算。
- 新会话自身超过单会话或用户/进程字节上限时必须拒绝，不能靠驱逐所有其他用户后仍插入。
- 驱逐只删除内存会话；不得触碰数据库、文件、failure cache 或发送事件。被驱逐 ID 与未知 ID 同为 404，避免披露运行时压力。

估算函数必须计入所有保留字符串，尤其 source rules/header/login/proxy、书籍元数据、章节 title/url/tag/variable；不能只用章节数量代替字节预算。

## 5. 前后端数据与保密边界

- `models.Book.Variable` 与 `models.Chapter.Variable` 是上游 Book/BookChapter 数据模型中的有界、不透明解析状态。同一认证用户可以在 session 投影中取回它们，以支持 Reader 状态和后续显式加入书架；前端不得解析或把它们当 URL。
- 完整 `BookSource` snapshot、header/cookie、login rule、代理凭证以及正文请求的最终 URL 永不下发。
- 临时 Reader 可以在当前组件内保留已加载正文以避免重复渲染，但不得写 OpenReader 的持久 shelf chapter cache、localStorage/IndexedDB、备份或 WebDAV。该 no-persistence 差异优先于上游旧实现的磁盘/浏览器缓存。
- 离开临时 Reader 不要求可靠 DELETE；重新进入同一未到期 session 可恢复目录，但不承诺持久阅读进度。

## 6. 实施前失败测试清单

1. 创建体：缺字段、无效 variable、超 64 KiB 均在 transport 前失败；超限为 413。
2. 生命周期：idle 续期、absolute TTL 不延长、自然到期 410、未知/foreign/驱逐 404。
3. 索引：`abc`、`-1` 不续期；有效但不存在 index 为 404。
4. 预算：单会话、每用户、全局数量与字节限制；确定性 LRU；驱逐无数据库/文件/事件副作用。
5. 变量：两章 fixture 覆盖 book variable 跨章、chapter variable 隔离、成功提交、失败/取消回滚。
6. 错误：BookInfo 与 content 的 transport/rule 错误响应不含 secret header、cookie、代理密码、raw rule、query/fragment。
7. 取消与 failure cache：取消不写 failure；typed source request failure 只写当前用户；rule error 不写。
8. 前端：Search 与 Explore 共用 create→route；临时 Reader 不调用 progress/bookmark/cache/category/source-change writer；显式加入书架仍携带 bounded book variable。
9. 回归：真实 Go fixture 完成 Search/Explore → BookInfo → temporary Reader → 跨章 → 显式加入书架，并在 1440×900、390×844、360×800 验证。

只有上述失败测试先建立、实现通过并完成真实浏览器门禁后，本专项才可从 `implementation-pending` 改为 `aligned`。
