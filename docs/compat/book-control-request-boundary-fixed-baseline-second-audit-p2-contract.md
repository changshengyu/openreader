# Book 控制动作请求边界固定基准第二轮合同（P2）

状态：**inventory-complete / implementation-pending**

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本轮只提取合同，不修改应用或测试。BookManage、Reader 换源、本地目录重解析、正文搜索、Book 写入、
远程抓取与缓存主合同继续权威；本合同只关闭 `backend/api/books.go` 中剩余六个直接 JSON 绑定入口的
wire、cardinality、字段 admission 和请求取消边界。

## 1. 范围与权威证据

固定上游：

- `web/src/components/BookManage.vue` 的批量删除、分组、缓存和单书 TXT/EPUB 导出；
- `web/src/components/BookInfo.vue#refreshLocalBook`；
- `web/src/views/Index.vue#saveBook`、`components/BookSource.vue#changeBookSource`；
- `web/src/components/SearchBookContent.vue#searchBookContent`；
- `BookController.kt#deleteBooks/#refreshLocalBook/#saveBook/#setBookSource/#exportBook/#searchBookContent`。

OpenReader：

- `POST /api/books/batch`、`POST /api/books/export`；
- `POST /api/books/:id/refresh-local`、`POST /api/books/remote`；
- `POST /api/books/:id/change-source`、`POST /api/reader3/searchBookContent`；
- `frontend/src/api/books.js`、`stores/bookshelf.js`、`useRemoteBookAddToShelf`、
  `useBookSourceChange`、`useReaderCatalogActions`。

固定上游以当前 namespace 的书籍 URL、来源 URL 和显式选择集执行动作。OpenReader 的 JWT、数字 ID、
SQLite transaction、多对多 Category、Blob 导出、派生缓存和稳定 `/api` 路径是已签收技术适配。本轮
不得把这些动作合并回宽 `saveBook`，也不得重开已发布的换源候选、章节位置、解析规则或可见 UI。

## 2. 当前差异与裁决

| 动作 | `OpenReader@c9b50e3` | 裁决 |
|---|---|---|
| 六路 JSON wire | 均直接 `ShouldBindJSON`；无 actual-read/single-document/UTF-8 边界，首个合法 object 后的第二文档可被忽略。 | **must-fix**：认证后执行 route-specific 16 KiB/32 KiB/1 MiB actual-read、单 UTF-8 non-null object；现代路由 overflow 413，其它 wire 400，legacy 保持 HTTP 200 失败 envelope。 |
| batch/export cardinality | `bookIds` 已有 200 与 unique-positive/owner 检查；`categoryIds` 无 raw 上限。 | **partial / must-fix**：保留 200 book IDs，并把 category IDs 限为 200 raw 项后再去重/owner 校验。 |
| batch cache 取消 | 每书循环调用 `cacheBookChapters(context.Background())`；断开后仍可继续最多 50 本的远程工作并发送完成投影。 | **must-fix remote work**：使用 request context，取消后不启动下一本、不发伪完成广播；已 durable 的章节缓存不回滚。 |
| export | body 拒绝前没有上限；TXT/EPUB 可在请求断开后继续遍历后续书/章。 | **must-fix admission/cancel**：拒绝发生在 DB/文件/章节读取前；生成循环检查 request context。完整导出、单本本地原文件和现有格式不截断。 |
| local refresh | 先解析整个本地源文件，再读取可选 JSON；`tocRule` 无 HTTP 字段上限。 | **must-fix work ordering**：owner/local-book 检查后先解析 body；空 body 保持有效，非空为单 object，规则最多 16 KiB，拒绝不得读取/暂存书档。 |
| remote add/change | 1 MiB 以内外均无 wire 边界，客户端字段没有既有 Book byte limit；抓取调用固定 `context.Background()`。 | **must-fix stored/control boundary**：1 MiB 单 object、Book 字段短界、200 categories、既有 variable limit，并把请求 context 传入 detail/TOC 抓取；取消不写 failure cache。 |
| legacy content search POST | 无 body 上限/单文档；POST 的 `lastIndex/size` 不使用 GET 的 clamp。 | **must-fix compatibility adapter**：16 KiB 单 object；原始 keyword 保持；POST control 数值与 GET 同界，wire 错误仍是 HTTP 200 `请求格式不正确`。 |

这些入口可触发删除、数百次关系写入、整书导出、本地解析或远程详情/目录/正文工作，优先于只操作
短 SQLite 行的 replace-rule first-document 长尾。它不授权修改导出内容、章节匹配、换源事务或缓存
完整区间。

## 3. 共同 wire、认证与错误合同

- JWT middleware 始终先于 body admission。无 token/无效 token 携带 declared/chunked overflow、无效
  UTF-8 或 multi JSON 时保持既有 401，且不得读取 body。
- `batch`、`export`、legacy content-search POST 的完整 body 上限为 `16 << 10` bytes；`refresh-local` 为
  `32 << 10` bytes，使合法 16 KiB TOC rule 加 JSON 包装仍可提交；`books/remote` 与 `change-source` 为
  `1 << 20` bytes，以容纳已发布的完整简介和候选投影。
- declared `Content-Length` 与 unknown-length/chunked 使用相同 actual-read 上限；精确 limit 可进入业务
  校验，`+1` 必须在任何 DB、文件、解析、远程请求、缓存或事件前拒绝。
- 非空 body 必须是有效 UTF-8 且恰好一个 non-null JSON object。空、`null`、array/scalar、损坏 JSON、
  尾随垃圾或第二 JSON 均拒绝；尾随 JSON whitespace 和 bounded unknown fields 保持兼容。
- 五个现代路由 overflow 返回 `413 {"error":"request body too large"}`；其它 wire/shape 错误保留各入口
  既有 flat 400 message。legacy POST 对 overflow/shape 均保留
  `200 {"isSuccess":false,"errorMsg":"请求格式不正确"}`，不能暴露 limit 或内部错误。
- `refresh-local` 与 `change-source` 先解析 path ID、确认 caller-owned book；`refresh-local` 还先确认本地
  类型，再读取 body。非法 ID 保持 400，缺失/外用户书保持 404；拒绝 body 不改变 target-first 优先级。

## 4. Batch 与导出动作

### `POST /api/books/batch`

- `action` 只允许 `delete`、`category`、`category-add`、`category-remove`、`cache`、`clear-cache`；未知值
  保持 400，不进入 transaction。
- 原始 `bookIds` 为 1..200 个 unique positive ID 且全部属于当前用户。继续以 404 隐藏混入的外用户书；
  不把外用户项静默丢弃后执行部分动作。
- `categoryIds` 最多 200 个原始值，再按现有 positive/dedupe/fallback 语义得到最终集合并完整验证 owner。
  `category-add/remove` 继续要求一个 caller-owned `categoryId`，移除保持幂等。
- `cache` 继续最多 50 本、每本既有整书/10 章适配不变；`clear-cache` 继续最多 100 本。cache 使用
  `c.Request.Context()`，在每本前检查取消；取消前已提交的缓存可保留，但不能开始后续书、写失败缓存、
  返回伪成功统计或广播完成投影。
- delete/category transaction、提交后文件清理、durable-only WebSocket 及现有 response schema 不变。

### `POST /api/books/export`

- 原始 `bookIds` 为 1..200 unique positive owner IDs；`format` 只允许空值/`json`、`txt`、`epub`，大小写
  和周围空白继续规范化。unsupported format 保持 400。
- 固定上游可见流程仍是单书 TXT/EPUB；OpenReader JSON、多书 ZIP 与 JWT Blob 作为已部署兼容能力保留。
  本地单书仍优先返回原文件，安全路径校验和 attachment filename 不变。
- body 拒绝时不查询章节/书签、不读取本地文件、不抓取正文。生成阶段在每本/每章前检查 request
  context；取消不启动后续远程正文请求，也不产生数据库、缓存引用或事件写入。
- 不以本轮安全适配截断完整书、降低 200 IDs、改写章节顺序或改变 JSON/TXT/EPUB bytes。输出内存模型
  后续若需流式化必须另立数据/下载合同。

## 5. 本地重解析、远程入架与换源

### `POST /api/books/:id/refresh-local`

- 零字节 body 继续表示使用持久 `book.tocRule`；这同时适用于已知 `Content-Length: 0` 和 unknown-length
  实际 EOF。非空 body 只允许 `{tocRule?: string|null}`；null/缺失保持旧规则，显式字符串 trim 后替换。
- `tocRule` 最多 `engine.MaxTXTTocRuleBytes`（16 KiB）；`+1` 在读取原书档前返回 400。普通规则、固定上游
  特殊规则、TXT/EPUB/UMD/CBZ/PDF/Markdown 分支和 parser budgets 均不改变。
- owner 与本地类型检查后，body admission 先于 `localBookSourcePath`、源文件读取、parser、stage 和
  transaction。失败不能修改 Book/Chapter、TOC/archive bytes、缓存路径、进度或事件。
- 成功继续采用既有有界源读取、stage + transaction + promote、原 archive 引用与目录/当前章回退；
  不要求用户重新上传或生成新 token。

### `POST /api/books/remote`

- `title`、`bookUrl` trim 后非空，`sourceId` 为正且指向当前用户 active source；`categoryIds` 最多 200
  raw 项并完整 owner 校验。现有同 URL 已入架分支、分类更新和 200/201 响应保持。
- 客户端 `title/author/coverUrl/kind/wordCount/bookUrl` 使用既有 Book 240/160/600/400/120/800 byte
  限制；`intro` 只受 1 MiB wire cap。`variable` 继续使用 32 项、16 KiB total 的 string-map 规范化。
  `sourceName/type` 继续作为兼容字段忽略，未知字段不获得持久化所有权。
- detail/TOC 使用 request context。取消、deadline 或 body/field rejection 不记录 source failure、不创建
  Book/Chapter/Category/Candidate、不广播；真实远程错误仍进入当前 user/source failure cache。
- 成功的 remote projection、canRename、章节 transaction 和候选播种保持 Reader/搜索合同。

### `POST /api/books/:id/change-source`

- caller-owned book lookup 先于 body；`sourceId` 为正且属于当前用户 active source。可选 `bookUrl` 与
  candidate metadata 使用同一 Book byte 限制，`intro` 只受 1 MiB wire cap。
- detail/TOC 使用 request context；取消不得记录 failure、替换目录、清缓存、更新 lastCheckTime、播种
  candidate 或广播。真实源错误继续沿既有安全 error/failure cache。
- 成功仍在单 transaction 内替换目录、来源/URL/变量/元数据、lastCheckTime 和当前候选，提交后修剪旧
  cache/image；前端只本地更新候选 current、关闭来源面板并恢复正文锚点，不发额外候选搜索。

## 6. Legacy 正文搜索 POST

- `GET /api/reader3/searchBookContent` 不改；POST 在 16 KiB 单 object 内接受 `url|bookUrl`、原始
  `keyword`、`lastIndex`、`size`。keyword 包括前后空白和纯空格，只有长度 0 才失败。
- POST `lastIndex` 夹到 `[-1, 1_000_000]`，`size` 夹到 `[1, 20_000]`，与既有 GET adapter 一致；缺失
  仍分别使用 0/20。章节完成式 cursor、dense result、UTF-16 offset、不可用章节和 request context
  取消合同不变。
- body/wire/field 拒绝、外书、无源与业务错误继续使用 HTTP 200 reader3 envelope；不得把现代 flat error
  混入该兼容路径。拒绝发生在 book/source/chapter 查询和正文抓取前。

## 7. 数据、前端与允许差异

- 不新增或修改 SQLite 表/列/index，不迁移或重写历史 Book/Chapter/Category/Progress/Bookmark 行。
- 不修改 `data/cache/library` 路径、local archive/TOC、chapter cache、backup/portable/WebDAV 格式；拒绝
  请求不清理文件。fresh/historical volume 仍需证明现有书可导出、重解析、换源和备份恢复。
- 规范 Vue payload 已远低于上限，不新增可见输入、提示、spinner 或状态；BookManage、BookInfo、Reader
  和搜索 overlay 的固定上游几何/文案/工具层合同不变。客户端继续使用现有 API/error pipeline。
- 16 KiB/32 KiB/1 MiB、UTF-8 single JSON、200 categories、字段短界和 request-context 取消是明确 Go/JWT/
  SQLite/远程抓取安全适配。上游的完整导出、精确搜索、已入架判重、重解析与换源可见状态不得退化。

## 8. 必须先写的失败测试

1. 六路无 token/无效 token 携带 declared/chunked overflow、invalid UTF-8/multi JSON 时仍先 401，且 body
   不被消费；现代路由 exact/+1 与 413/400、legacy 200 envelope 全覆盖。
2. 每路空/null/array/scalar、尾随垃圾、第二 JSON、unknown fields 和精确上限；拒绝路径用 DB callback、
   fetch transport、filesystem witness 和 Hub 队列证明零业务工作。
3. batch 的 200/201 book IDs、200/201 category IDs、unique-positive/owner、六 action、50/100 子上限和
   transaction/broadcast 保持；阻塞 transport 证明取消后不启动下一本或发送完成投影。
4. export body 拒绝不查 chapters/bookmarks、不读文件；TXT/EPUB/JSON 与 local original bytes 保持；
   取消停止下一章/下一本且无持久副作用。
5. refresh-local 的 known/unknown-length empty body、null tocRule、16 KiB/+1；恶意 body 在 missing/阻塞
   source witness 前失败，成功 TXT/EPUB/UMD/CBZ/PDF/Markdown 仍复用原书档和原子 stage。
6. remote add/change 的 1 MiB exact/+1、字段 byte 边界、200/201 categories、variable limits、owner/source
   priority；取消 detail/TOC 后无 failure/row/chapter/candidate/cache/event，真实失败仍记录。
7. legacy POST 原始空白 keyword、lastIndex/size clamp、dense cursor、外书/无源和取消保持；wire 错误在
   HTTP 200 且正文 transport 零调用。
8. 前端静态/状态测试证明规范 payload、BookManage/BookInfo/Reader/搜索动作和错误处理没有新增流程。
9. focused/full/race/vet、frontend full/build、真实 Go + Chromium 1440x900、390x844、360x800，以及
   fresh/historical/portable 卷门通过后才可发布 Docker。

## 9. 实施闸门

本合同先独立提交。随后添加能在 `c9b50e3` 失败的 Go/前端合同，再实现共享窄 decoder、route-specific
validation 与 context 传播。不得引入全局 body middleware，不得顺带重构 parser、导出格式、换源 UI、
数据库 schema 或 backup。只有应用回归、真实运行时、卷门和本地双架构发布全部完成后，状态才能改为
`aligned / regression-validated / Docker-published / awaiting-device-verification`。
