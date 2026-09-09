# Reader 章节正文请求与持久提交生命周期第二轮固定基准合同

审查日期：2026-09-09

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审已入架书籍章节正文请求从 Reader 发起、远程解析、服务器缓存/变量提交到浏览器展示的
生命周期。Reader 布局、翻页/连续滚动、换源候选面板、正文 parser 规则、抓取安全预算、缓存文件
rooted 边界和整本缓存可见交互均已有专项合同，不因本轮重建。

## 1. 权威源码

### reader-dev

- `web/src/views/Reader.vue:920-925#changeBookSource`
- `web/src/views/Reader.vue:998-1072#getContent`
- `web/src/views/Reader.vue:1163-1223#loadShowChapter`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:581-618#getBookContent`
- `src/main/java/io/legado/app/help/BookHelp.kt:70-121`
- `src/main/java/io/legado/app/model/webBook/WebBook.kt:201-245#getBookContent`
- `src/main/java/io/legado/app/model/webBook/BookContent.kt`

固定上游以当前书 `bookUrl` 和 catalogue chapter index 请求正文。主正文响应完成时再次比较当前
`bookUrl` 与阅读 index；若期间换书、换源或换章，迟到响应不进入正文、进度或当前缓存。远程正文
parser 只修改本次内存中的 Book/BookChapter variable，正文文件由当前书 URL 和 chapter index 派生；
请求不会把旧 BookChapter 实体写回 catalogue。

### OpenReader

- `frontend/src/views/Reader.vue`
- `frontend/src/api/books.js#getChapterContent`
- `frontend/src/utils/bookChapterCache.js#loadBrowserChapterContent`
- `frontend/src/composables/useReaderChapterContent.js`
- `frontend/src/composables/useReaderChapterLoader.js`
- `frontend/src/composables/useReaderCatalogActions.js`
- `backend/api/books.go#chapterContent/loadChapterTextContextResultWithPolicy`
- `backend/api/cache_stream.go#cacheBookChapters`
- `backend/models/models.go#Book/#Chapter/#BookSource`
- `docs/compat/online-booksource-parser.md#P2-Parser-1G`
- `docs/compat/remote-chapter-cache-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md`

OpenReader 持久化 Book/Chapter parser variable 和 `chapters.cache_path` 是已发布的 SQLite/多请求适配；
它必须补足固定上游不需要的陈旧提交保护，不能让适配反向改变换源后的正文或变量状态。

## 2. 当前差异矩阵

| 合同点 | 固定上游 / 已签收语义 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 主正文迟到响应 | 完成时比较当前 `bookUrl` 和 index；换书/换源/换章后直接丢弃 | `useReaderChapterLoader.load()` 没有 request generation 或 scope guard，旧 Promise 完成后无条件覆盖 chapter/content/layout/progress | **P0 must-fix** |
| 请求取消 | 连接关闭停止当前请求链；换源后旧结果不展示 | shelf chapter GET 不接收 `AbortSignal`；cache reset 不能取消旧正文 fetch | **P0 must-fix** |
| 浏览器缓存收敛 | 上游缓存按旧 `bookUrl` 隔离，迟到主请求不成为当前正文 | OpenReader 虽按 URL 分 key，但 source-change 先 clear 后，旧请求仍可再次写回旧 browser cache | **must-fix** |
| parser variable 提交 | 上游只在本次内存对象中传播，不把旧 chapter 写回 catalogue | fetch 后用无 request context 的 transaction 按 Book/Chapter ID 更新；不验证 source/url/rule/variable snapshot 或 RowsAffected | **P2 must-fix** |
| 缓存文件发布 | 文件由当前 book/chapter identity 派生 | 先覆盖最终 cache 文件，再提交未校验的变量/path；陈旧或取消结果可覆盖当前正文文件或留下无引用文件 | **P2 must-fix** |
| legacy path 归一化 | 上游不需要 SQLite path 回写 | cache hit 用 contextless、忽略错误的 full-row `Save(chapter)`；可覆盖并发 chapter 字段或 fallback-insert 已替换章节 | **P2 must-fix** |
| 正常解析/展示 | 当前 source rule、变量优先级、图片、替换规则和响应 shape | 已有专项测试和合同 | **closed / preserve** |

## 3. 前端请求与展示合同

1. 每次主章节 load 捕获不可变 scope：认证用户、Book ID、Book URL/source ID、chapter index 和递增
   generation。只有仍为当前 generation 且 scope 未变的结果可以修改 loading/error、chapter/content、
   format/resource、chapterBlocks、布局、位置、预加载和进度。
2. 新主 load、换书、换源、目录刷新、缓存 reset、会话失效和页面卸载必须使旧 generation 失效；可取消
   的 HTTP 请求同时 abort。迟到成功与失败都静默丢弃，不能覆盖新正文、显示旧错误或关闭新 loading。
3. `getChapterContent` 和 browser-cache loader 传递 `AbortSignal`。一个 scope 内同 chapter/mode 仍可复用
   in-flight Promise；失效后不能把旧响应重新写入 memory/browser cache，也不能标记当前章已缓存。
4. 连续阅读 window 已有 transaction guard 保持；本轮补齐最外层主章节 load 和共享内容缓存，不改变
   finger/wheel 连续滚动、点击翻页、预载半径、EPUB/CBZ/audio 分支或现有提示文字。
5. 固定上游的最终可见结果保持：换源成功后只显示新 catalogue 对应正文并恢复当前位置；旧请求是否
   已从网络返回不得改变页面。

## 4. 后端工作与提交合同

远程正文 fetch 开始时冻结：

- caller user、Book ID/source ID/URL/variable；
- BookSource ID、当前用户 active association 和 source semantic snapshot；
- Chapter ID/book ID/index/URL/variable/cache path。

远程工作成功后，在任何持久发布前必须使用同一个 request context 重读并核对当前状态：

1. Book 仍属于 caller，且 source ID/URL 与请求快照一致；Book 被删除、换源或换 URL 时旧结果失效。
2. Source 仍是 caller 当前可用快照，规则/URL/header/charset 等语义未变；同 ID source 编辑清空变量后，
   旧请求不得把变量重新写回。
3. Chapter 仍是同一 Book 下的同一 ID/index/URL；目录刷新/换源替换或删除章节后，不得重建旧行。
4. 开始解析时的 Book/Chapter variable 仍为当前值；并发成功请求不得以旧初值覆盖较新的变量状态。
5. 只更新本动作拥有的 `books.variable`、当前 `chapters.variable/cache_path`；不得写 title/URL/index/
   resource/time 等完整快照。guard 未命中视为陈旧，不是成功。

提交与文件发布必须在既有 `remoteCacheMu` 协调边界内收敛：陈旧、取消或数据库失败不能覆盖当前有效
cache 文件、发布新的 DB path、留下可见 orphan 或清除其它引用。正常成功仍原子提交 Book/当前 Chapter
变量和 cache path；其他章节变量逐字节不变。legacy absolute path 的懒归一化只可使用 request-context、
old-path-guarded 的单列 update；失败继续按 cache hit 返回正文，但不得 full-row Save 或插入行。

## 5. API、错误与调用方

- `GET /api/books/:id/chapters/:index/content` 的路径、JWT、server-authoritative index、正常
  `200 {chapter,content,format,...}` 和 parser/source `502` 保持。
- 请求在远程工作后发现书/source/chapter/variable snapshot 已变化时，返回路径和值无关的
  `409 {"error":"chapter content changed; retry"}`。Book/Chapter 在初始 owner lookup 前不存在仍保持
  既有 404。caller cancellation 不合成响应。
- 陈旧提交和 cancellation 不是 source failure，不写 `source_failures`；真实抓取/parser 错误继续走
  已签收的安全 code/stage 和 600 秒 caller-scoped failure cache。
- 正文搜索、导出、普通/stream 整本缓存都复用同一 loader 和 snapshot guard。它们可沿既有各自 envelope
  报告 unavailable/failed/cancelled，但不得绕过提交保护、回写旧变量或发布旧 cache。
- 成功响应中的 chapter 必须是本次已验证的当前行，而不是请求开始时已经失效的结构体。

## 6. 数据、迁移与允许差异

- 不增加或删除表、列、索引、迁移标记，不扫描或改写旧卷。
- `data/`、`cache/`、`library/`、章节 cache hash、Book/Chapter variable JSON、普通/portable/Legado/
  WebDAV backup 和环境变量均不变。
- 现有安全普通 relative/当前 absolute cache path 继续懒读；只把未来成功归一化改成 guarded 单列更新。
- JWT/REST/SQLite、持久 parser variable、request cancellation、409 stale conflict 和 browser cache scope
  是技术/安全适配；不得改变固定上游最终显示“当前书、当前来源、当前章节正文”的产品结果。
- 回滚旧镜像继续读取同一数据，只会重新引入迟到 UI、陈旧 variable/cache 提交和 full-row Save 风险。

## 7. 测试先行门

应用实现前，以确定性 barrier 在旧实现上证明失败：

1. Go：fetch 后并发 change-source、source semantic edit、catalogue refresh、Book/Chapter delete 和 variable
   update；旧请求不能改当前变量/cache、复活章节、覆盖文件或写 failure cache，且 stale 为安全 409。
2. Go：commit 前 cancellation 传播到 GORM；零 Book/Chapter/path/file/failure side effect。正常 fetch 仍
   原子保存 Book/当前 Chapter variable/cache，其他章节不变。
3. Go：legacy local/remote absolute path 归一化与并发 metadata/目录替换；只改匹配 old path 的现存行，
   不覆盖 URL/title/variable/resource，不 fallback insert，DB 错误仍可返回已打开 cache bytes。
4. 前端：先发旧来源 A，再换源并完成 B，最后完成 A；A 的成功/失败都不得改变 B 的正文、格式、
   loading/error、浏览器/内存缓存、布局、预载或进度。断言 HTTP signal 被 abort。
5. 前端：同 scope 去重和连续 window/EPUB/CBZ/audio/refresh 正常路径保持；切书、目录刷新、session reset
   和 unmount 均使旧 generation 失效。
6. 真实浏览器在 1440x900、390x844、360x800 以可控延迟源验证“旧正文请求中换源”：最终只显示新
   来源正文，目录、当前位置、工具层和移动连续滚动不跳变；服务端 DB/cache 只保留新来源状态。

实现后运行 focused/race、相邻 source-switch/cache/parser tests、Go full/vet、frontend full/build、Compose
以及可信 Actions fresh/historical/portable/published-platform 门。发布报告继续区分 Git HEAD、Docker
commit 和用户生产 commit。

## 8. 实施与验证结论

合同 `c500e81`、旧实现红测 `5bf7c66` 与实现 `0a8a0ef` 已按顺序落地。前端主 loader 和共享缓存现在
使用 AbortSignal、generation 与 book scope 拒绝迟到结果；换源时冻结待恢复位置，失效的 reload 不再
显示成功提示。后端以 caller/source/book/chapter/initial-variable snapshot 复验提交资格，只 guarded
更新拥有列，并用 staged/backup 文件事务确保陈旧、取消或数据库失败不会发布 cache；legacy path
归一化不再 full-row Save 或复活章节。

确定性旧实现红测分别暴露 3 个前端和 4 个后端失败。实现后 focused、相邻 API/parser、race、Go full、
`go vet`、frontend **748/748**、Vite build、Compose 均通过；延迟旧来源的真实浏览器换源场景在
1440x900、390x844、360x800、1024x1366 四视口通过，并保持新正文与当前位置。全量 Go 在受限环境中
仅因旧 httptest 无法绑定 IPv6 loopback 失败，解除网络沙箱后同一命令全部通过。

本切片不改变 schema、备份格式、持久目录、环境变量或正常 API envelope。实现提交为 `0a8a0ef`。
GitHub Actions run `34321320014` 的 backend/frontend/Compose、native image、fresh/portable、historical
volume 和 published-platform 门全部通过，并将包含该实现的 `a7917ed`/`latest` 发布为 amd64/arm64 OCI
index `sha256:36c7d42ee048a061e44f639fa45ac5e1060bcc0e70583990de0655addf309d76`。两平台 config 均报告完整
revision `a7917ed0540f43e1bea8f958dfd5d7736ed0a9f0`。
