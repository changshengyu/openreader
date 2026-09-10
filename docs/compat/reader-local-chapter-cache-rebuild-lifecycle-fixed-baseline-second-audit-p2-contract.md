# Reader 本地章节缓存回建生命周期第二轮固定基准合同（P2）

审查日期：2026-09-10

状态：**aligned / regression-validated / trusted-publication-pending**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审已入架本地 TXT/text/Markdown/PDF/UMD 和 EPUB 在派生章节文本缓存缺失时，从 caller-owned
归档读取正文、回建 cache 文件并更新 `chapters.cache_path` 的生命周期。已发布的本地导入、格式 parser、
rooted archive、EPUB resource capability、显式 `refresh-local`、Reader 前端 generation 和远程章节正文
生命周期保持关闭。

## 1. 权威源码

### reader-dev

- `src/main/java/com/htmake/reader/api/controller/BookController.kt:432-618#getBookContent`
- `src/main/java/io/legado/app/help/BookHelp.kt:70-121#getContent/saveText`
- `src/main/java/io/legado/app/model/localBook/LocalBook.kt:18-72#getContent`
- `src/main/java/io/legado/app/model/localBook/TextFile.kt:30-49#getContent`
- `src/main/java/io/legado/app/model/localBook/UmdFile.kt:26-38#getContent`

固定上游按当前 namespace 的 Book 和当前目录项，从本地文件范围或当前 EPUB/UMD 资源读取正文。本地
TXT/UMD 读取不把目录实体写回 shelf/catalogue；EPUB 的可选文本 cache 只写派生文件。删除或刷新书架项
不会因为一个旧正文读取而重新创建目录项。

### OpenReader

- `backend/api/books.go#chapterContent/loadChapterTextContextResultWithPolicy`
- `backend/api/books.go#rebuildLocalChapterTextContext/persistRebuiltLocalChapterTextContext`
- `backend/api/books.go#refreshLocalBook/deleteBook`
- `backend/api/local_book_archive.go#resolveLocalBookArchive/openLocalBookSource`
- `backend/api/local_refresh_stage.go`
- `backend/api/remote_cache_filesystem.go#stageLocalChapterCache`
- `backend/services/epubreader/service.go#ReadChapterTextContext`
- `backend/services/bookcatalog/catalog.go#ReplaceChapterRows`

OpenReader 把本地正文缓存路径保存在 SQLite 是技术栈适配。该适配必须以提交时仍有效的 caller Book、
Chapter 和 archive 为边界，不能反向改变当前目录或归档。

## 2. 当前差异矩阵

| 合同点 | 固定上游 / 已签收语义 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| request context | 当前请求读取当前本地正文；连接结束不应继续发布用户可见状态 | caller context 已贯穿 bounded archive read、EPUB recovery、stage 和 GORM；取消后禁止发布 | **aligned** |
| Book/Chapter 存活 | 本地正文读取不创建 shelf/catalogue 实体 | 回建后与 transaction 内都复验 caller-owned Book 和完整 Chapter snapshot；不再 `Save` 旧实体 | **aligned** |
| 刷新竞争 | 显式 refresh 产生当前目录；旧读取不把旧目录写回 | refresh、cleanup 与 cache publish 共用本地 cache coordinator；旧 snapshot 安全 409，不复活目录 | **aligned** |
| 列所有权 | 正文读取不修改目录 title/url/resource/variable/time | 只执行 old-snapshot-guarded `UpdateColumn("cache_path", ...)`；合成 URL 只用于 hash | **aligned** |
| 文件发布 | cache 是可重建派生物，不是目录权威 | request-private stage 在事务内 promote；陈旧、取消、DB 或 publish 失败均 rollback/清理 | **aligned** |
| archive 身份 | 当前本地文件是正文来源 | opened archive 和 source 的 regular same-file identity 在提交前再验证，同时完成 DB snapshot 复验 | **aligned** |
| 正常恢复 | TXT/EPUB/UMD/历史卷 cache miss 仍应可读 | 正常回建、格式预算、缺失 cache path 恢复和旧路径兼容均通过回归 | **aligned / preserve** |

## 3. 读取与取消合同

1. 主路由继续使用 `GET /api/books/:id/chapters/:index/content`、Bearer JWT、owner-first Book lookup 和
   server-authoritative chapter index。缓存命中、EPUB/CBZ/audio 分支及正常 response shape 不变。
2. 本地 cache miss 开始时冻结 caller、Book 本地来源语义、Chapter 解析身份、旧 cache path，以及已打开
   archive/source 的 same-file 身份。Book 语义至少包含 ID/user/source=0、URL、LibraryPath、OriginalFile、
   TOCFile、SourceFile 和 TOCRule；Chapter 语义至少包含 ID/book/index/title/URL/resource fragments/variable。
3. 归档读取继续使用已签收的 owner root、opened regular file 和 legacy parser budgets。HTTP request
   context 必须贯穿 bounded read 和 cache write；不能中断的 parser/EPUB 阶段至少在前后检查取消，并在
   取消后禁止任何 DB/file publication。
4. 从旧 snapshot 读出的正文只是一份待返回/待缓存结果。它在提交复验通过前不是当前目录正文，不能
   用来更新 cache path、覆盖 current cache bytes，或让 final response 绕过 current-row 检查。

## 4. 提交资格与文件事务

在任何最终 cache 文件或 SQLite 路径发布前，同一 request-context 协调边界必须：

1. 重读 `id + user_id` Book，确认仍是同一个本地 archive/parser snapshot；删除、改为远程、替换 archive、
   修改 URL/TOC rule 或显式 refresh 改变语义时，旧结果陈旧。
2. 重读 Chapter，确认仍是同一个 ID/book/index/title/URL/resource/fragment/variable/cache snapshot。目录
   refresh/delete/replacement、并发 cache clear 或另一路成功回建都必须使旧提交失败或安全复用 current。
3. 再验证 opened archive root/source identity 仍 current。只验证路径字符串或目录仍存在不足以授权写入。
4. cache 内容先写同一受控根下的 request-private stage；陈旧、取消或 DB 失败删除 stage。最终 promote、
   guarded DB path update 和回滚/补偿必须在同一 per-cache coordinator 下收敛，不能留下 current row 指向
   错误 bytes，也不能覆盖更新目录的 active generation。
5. SQLite 只允许 old-snapshot-guarded `UpdateColumn("cache_path", next)` 并验证 `RowsAffected == 1`；不得
   `Save` Chapter，不得更新 URL/title/index/resource/variable/timestamps。历史空 URL 可只作为本次 hash
   fallback，不在正文读取路径持久化；规范目录只能由导入或显式 refresh 产生。

若 cache persistence 的普通故障使 current snapshot 无法证明，返回现有安全章节加载错误且不发布文件/
路径。若同一 current Chapter 已由并发请求成功缓存，可权威重读并复用其 cache；不得把较晚旧结果覆盖
上去。

## 5. API、调用方与错误

- 初始 missing/foreign Book 或 Chapter 保持 404。初始归档/正文不可读保持当前安全错误边界，不暴露宿主
  path、用户目录或原始 parser 细节。
- parse 后 Book/Chapter/archive snapshot 失效复用现有
  `409 {"error":"chapter content changed; retry"}`。caller cancellation 不合成响应。
- 正文搜索、导出和 cache stream 复用同一 loader；必须传播其现有 context，并按各自既有 envelope
  处理 stale/cancelled。旧本地目录正文不得进入搜索结果或导出，也不得被计为成功缓存。
- Reader 已发布的 AbortSignal/generation 会丢弃迟到 UI；本轮不修改前端 route、按钮、提示、缓存 key、
  连续滚动或点击翻页。

## 6. 数据、迁移与允许差异

- 不增加/删除表、列、索引、migration marker，不扫描或重写旧 SQLite。
- `data/`、`cache/`、`library/`、Book/Chapter JSON、普通/portable/Legado/WebDAV backup 和环境变量不变。
- 已存在的安全相对/历史绝对 cache path 继续懒读；新回建仍位于 caller-owned archive 的 `content/` 派生
  子树。request-private stage 必须被 backup/export 忽略，并在失败或下次安全清理时收敛。
- request context、snapshot guard、显式单列更新、per-cache coordinator 和 staged publication 是并发/
  多用户安全适配；不得改变固定上游当前本地正文可读结果。
- 回滚旧镜像继续读取同一卷，只会重新引入 contextless parse、full-row `Save`、旧章节复活和 orphan
  cache 风险。

## 7. 测试先行门

实现前用确定性 barrier 在旧实现上证明：

1. parse 后删除 Book：请求安全 409；Book/Chapter 不复活，archive 删除结果不被重建，零 cache/file/event。
2. parse 后完成 `refresh-local`：旧请求 409；新 Chapter IDs/metadata/cache generation、Progress/Bookmark 和
   archive metadata 完整保留，旧 `content/<hash>` 不出现。
3. parse 后改变 Book archive/TOC snapshot 或 Chapter URL/title/resource/variable/cache path：旧请求 409，
   不覆盖任何列或 bytes；并发普通 Book metadata 编辑不被正文回建写回。
4. caller cancellation 在 bounded read、parse 后、stage 后和 DB 前均产生零 row/file/event 副作用；旧
   `context.Background()` 路径由测试明确暴露。
5. guarded update/transaction/promote 注入失败：stage/backup 收敛，原 current cache bytes/path 保持；无
   fallback insert、unique-index 噪声或半发布状态。
6. 两个同章回建并发时只有 current snapshot 可发布；成功响应的 Chapter 来自权威重载。另一用户的书、
   archive 和相同相对 cache path 不变。
7. TXT、EPUB、标准/legacy UMD 与历史卷正常 cache miss 仍恢复相同正文；搜索、导出、普通/stream cache
   的 current/cancel 语义不退化。
8. 实现后运行 focused/race、相邻 local-refresh/archive/old-volume/parser tests、Go full/vet、frontend
   full/build、Compose、Reader 本地书四视口，以及可信 Actions fresh/historical/portable/platform 门。

## 8. 实施与回归结论

合同 `1b2ea90`、旧实现红测 `b75f640` 和实现 `a131aa9` 已按顺序提交。红测确定性证明了删除后
Chapter 复活、refresh 旧 cache orphan、并发字段覆盖、取消后副作用和 DB 错误被吞掉；同一组测试
在新实现上全部通过。

实现以 caller context、Book/Chapter/archive 快照、显式 `cache_path` 单列更新、request-private stage
和共享本地 cache coordinator 关闭生命周期。EPUB metadata recovery 只改工作副本，不再写回 archive
catalogue。全局 coordinator 比合同的 per-cache 最小要求更强，是不改变可见结果的 Go/SQLite 安全适配。

focused 和 `-race`、Go 全量、`go vet`、frontend 748/748、Vite build、Compose 以及
1440×900、390×844、360×800、1024×1366 Chromium Reader 回归已通过。本切片未增加 schema、migration、
环境变量、持久根或 backup member。可信 Actions run `34471037381` 正在执行，因此当前不宣称
Docker 已发布。
