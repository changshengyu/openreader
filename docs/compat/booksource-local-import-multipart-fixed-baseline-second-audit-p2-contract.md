# BookSource 本地导入 multipart 固定基准第二轮合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本轮先提取合同，再按红测试、实现、运行时和发布门依次关闭。已发布的书源管理 UI、reader-dev 字段往返、5,000 项、
`sourceLimit`、owner/COW、失败缓存和事务合同继续权威；本合同只关闭原始本地文件读取与已部署
`POST /api/sources/import` multipart 适配层尚未签收的 wire、part shape、错误和资源所有权。

## 1. 范围与权威证据

固定上游：

- `web/src/views/Index.vue#uploadBookSource/#onSourceFileChange/#saveSourceList`；
- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `readSourceFile`、`saveBookSources`；
- `BookSourceController.kt#readSourceFile/#saveBookSources`。

OpenReader：

- `frontend/src/components/workspace/SourceTransferOverlay.vue`；
- `frontend/src/composables/useSourceTransfer.js#createSourceImportForm/#importFile`；
- `frontend/src/api/sources.js#importSources`；
- `backend/api/server.go`、`sources.go#importSources/#decodeBookSources`；
- `backend/services/booksources` 与既有 source ownership/write/import tests。

固定上游 chooser 没有 `multiple`，只取 `event.target.files[0]`。正常路径在浏览器用 `FileReader`
解析非空 JSON 数组并打开选择预览；读取失败才把该文件作为 `file` multipart 交给 `readSourceFile`，
服务端返回文件文本，最终仍以所选 JSON 数组调用 `saveBookSources`。fallback controller 虽遍历全部
uploads，但规范 UI 一次只提交一个文件，也没有 scalar metadata。

OpenReader 保留一个部署适配路由：浏览器读取原始文件并预览，确认后把所选数组重新序列化为
`bookSources.json` 的单个 `file` part；Go 在同一请求内解码并事务导入。该合并不改变上游可见状态，
但要求 multipart API 自己拥有明确的字节、形状和清理合同。

## 2. 当前差异与裁决

| 边界 | `OpenReader@54f2c83` | 裁决 |
|---|---|---|
| 原始 chooser 文件 | `importFile` 在任何 size 检查前调用 `file.text()`；16 MiB 后端限额只作用于预览后重新生成的 selected-source Blob。 | **must-fix browser work boundary**：已知 `file.size > 16 MiB` 必须在读取/JSON 解析前拒绝。 |
| multipart 总体 | 已有 `MaxBytesReader(16 MiB + 1 MiB)`，但不预检 declared length，解析 overflow 会被统一映射成 `400 file is required`。 | **must-fix wire/error**：declared/chunked actual overflow 均为稳定 `413 request body too large`；精确边界进入 shape/file 检查。 |
| file bytes | 读取最多 16 MiB + 1，超限为 `413 source file is too large`。 | **aligned**：保持精确 16 MiB；不得扩大或把 JSON/parser 错误误报 413。 |
| multipart shape | `c.FormFile("file")` 取首项；额外同名/异名 file 和任意 scalar part 被忽略后仍可提交。 | **must-fix parser ambiguity**：只接受恰好一个名为 `file` 的 file part，拒绝所有 scalar 和额外 file part。 |
| filename/MIME | 内容按 JSON 解码，文件名不持久化，也不依赖扩展名或 MIME。 | **aligned security behavior**：不按名称、扩展名或 MIME 判断内容；part 仍须由 multipart parser 分类为 file。 |
| 临时 multipart | handler 不显式 `RemoveAll`；标准 `net/http` server 在请求结束会清理，但 handler 测试、替代嵌入和后续 memory threshold 变更没有局部所有权。 | **must-fix ownership hardening**：只要 form 已建立，handler 在所有成功/失败返回上显式清理。 |
| JSON、身份与事务 | 接受数组、`bookSources`/`sources` wrapper 和单对象；最多 5,000 raw item，按 caller namespace identity/COW/quota 一次事务导入。 | **aligned**：不得因 multipart 整理重写或收窄。 |
| 认证与事件 | protected JWT 后再检查 `CanEditSources`；只有 durable mutation 清 failure cache 并广播。 | **aligned**：`401/403` 必须先于 body read；拒绝请求零持久化和零事件。 |

选择该项为 remote-work 后的下一批，是为了关闭一个有界但仍有解析歧义的上传入口，并纠正原始浏览器
文件仍可在后端 admission 前消耗无界内存的遗漏。它不授权重开可见 SourceManager、书源解析器、远程
导入或已发布的 source 写入/配额模块。reading progress 与其它 batch/control JSON 继续留在动作差集。

## 3. 前端本地文件合同

- chooser 继续只处理第一项；`accept=.json,application/json`、侧栏独立入口、预览几何、默认空选、
  JavaScript/WebView 自动跳过、文案和账号 operation generation 保持。
- 导入预算固定为 `16 << 20` bytes。浏览器 `File.size` 为有限数且超过预算时，不调用 `text()`、不打开
  预览、不调用后端，显示明确 `书源文件过大`，然后由现有 overlay owner 关闭流程。
- size 缺失的测试/兼容 File-like 对象继续进入 `text()`；真实浏览器 File 的非负 size 是权威 admission。
  精确 16 MiB 仍读取并进入既有 JSON/空数组裁决。
- JSON parse、非数组或空数组仍显示 `书源文件错误`。本轮不让本地 chooser 接受 wrapper/单对象，
  因为固定上游可见路径只预览非空数组；后端 wrapper/单对象是已部署 API 兼容，不删除。
- 确认后 `createSourceImportForm` 继续只生成一个 `file=bookSources.json` part，内容只含当前选中项；
  成功、reload、关闭和迟到账号请求隔离不变。

## 4. `POST /api/sources/import` wire 与错误合同

- 路径、Bearer JWT、`CanEditSources`、成功 `200 {imported,updated,skipped}` 保持。`401/403` 在 declared
  length、multipart parse 或文件读取前返回，不泄露 body/shape 是否有效。
- multipart request 总实际读取最多 `17 << 20` bytes，即 16 MiB file budget 加 1 MiB envelope。
  `Content-Length > 17 MiB` 可在授权后直接拒绝；未知/chunked body 必须由 `MaxBytesReader` 得到同一
  `413 {"error":"request body too large"}`。不得依赖 Gin 32 MiB memory threshold 作为请求限额。
- form 必须恰好含一个 `form.File["file"]`，且所有 file part 总数为一；`form.Value` 必须为空。
  duplicate `file`、异名 file、scalar `file`、任意 metadata/value 或 mixed part 均在文件打开、JSON、
  SQLite、failure cache 和 broadcast 前返回 `400 {"error":"invalid source import request"}`。
- 合法空 multipart 或非 multipart/missing file 保持 `400 {"error":"file is required"}`；损坏 boundary/
  header 为 `400 {"error":"invalid source import request"}`。已能识别的 envelope overflow 不得降级成
  missing-file 400。
- file 内容最多 `16 << 20` bytes。精确边界进入 `decodeBookSources`；+1 保持
  `413 {"error":"source file is too large"}`。打开/读取失败继续使用现有安全文本，不返回宿主路径。
- JSON 内容继续接受 array、两个 wrapper alias 或单对象，保留 unknown dormant fields 与 reader-dev
  aliases。invalid JSON 仍为 `400 invalid JSON format`；5,001 raw item、quota conflict、服务错误和
  成功 envelope 均由已发布 BookSource 写合同权威。

总 envelope 与 file 两个 413 必须区分：前者表示 HTTP multipart 本身超限，后者表示唯一 file part
超限。合法 file 不因 boundary/header 开销占用自身 16 MiB 配额。

## 5. 资源、副作用与数据合同

- multipart form 一旦创建，handler 立即取得所有权并 `defer form.RemoveAll()`；成功、shape/JSON/
  cardinality/quota/service error 都必须清理。清理失败不覆盖已确定的业务响应，也不得进入日志泄露路径。
- body/shape/file/JSON 拒绝不得调用 `bookSources.Import`，不得创建 source/association、清 failure cache、
  更新 namespace timestamp 或广播。持久 mutation 后既有单事务、单用户 event 语义保持。
- 不新增表/列/索引，不迁移或删除历史 source，不改变 `data/cache/library`、backup/portable/WebDAV、
  default namespace 或 source archive 格式。
- MIME、filename 和客户端扩展名不成为信任边界；只把唯一 file 的有界内容交给现有 JSON decoder。
  本轮不执行 JavaScript/WebView，也不修改 selector、远程 fetch 或 parser runtime。

## 6. 允许差异

16 MiB 浏览器预读、17 MiB multipart actual-read、严格单 file/零 scalar 和 handler-owned cleanup 是
OpenReader 的 Go/security adaptation。固定上游服务端 fallback 可遍历多个 uploads，但其可见 chooser
只提交一个文件；拒绝额外 part 不删减规范产品工作流。JWT REST、合并 read+save、关系型 ID、COW、
quota 和安全脚本筛选继续是已批准适配。

## 7. 必须先写的失败测试

1. 前端 `file.size` 精确 16 MiB 调用一次 `text()`；+1 不读取、不预览、不请求后端并显示
   `书源文件过大`。现有 JSON error、空数组、正常预览/选择和 session generation tests 保持。
2. 无 token/无效 token/禁用 source edit 携带 declared/chunked overflow 或 malformed multipart 时仍先
   返回 `401/403`，并证明 body 未被消费。
3. declared 与 chunked 17 MiB +1 为精确 413；合法 envelope 和 16 MiB file 精确边界进入业务流程；
   16 MiB file +1 为既有 `source file is too large` 413。
4. duplicate `file`、异名 file、file+foreign file、任意 scalar、scalar+file 均为 shape 400，且第一
   个合法 JSON 不能因后续歧义 part 被导入。
5. non-multipart、空 multipart、损坏 boundary、missing file 分别符合第 4 节错误文本，不把 parse
   overflow 误报为 `file is required`。
6. 以 `router.MaxMultipartMemory=1` 强制磁盘临时文件，证明 success、invalid JSON、5,001 项、quota/
   service failure 后 form temp file 均已删除；不得只依赖 `net/http` 请求结束清理。
7. 数组、wrapper、单对象、unknown fields、5,000/5,001、满额 identity update/COW、全批回滚、no-op
   event 和 owner 隔离既有合同继续通过。
8. 真实 Go + Chromium 在 1440x900、390x844、360x800 覆盖 oversize raw file 无请求、正常预览选择、
   单 file wire、成功 reload 和账号切换迟到隔离。

## 8. 实施与发布闸门

先提交能在 `54f2c83` 失败的 frontend/backend 合同，再实现。实现后运行 focused source/import/API tests
与 race、`go test ./...`、`go vet ./...`、frontend full/build、SourceManager 与本合同三视口真实浏览器，
以及 source ownership、fresh/historical/portable 卷门。只有全部通过并由本地 amd64/arm64 构建发布后，
状态才能改为 `aligned / Docker-published / awaiting-device-verification`。

## 9. 实施、验证与发布结果（2026-08-16）

- 合同 `d7bc00a`、旧实现红测 `ddbac4c`、实现 `8c66dc9` 和真实运行时合同 `3f3c9c8` 按闸门顺序落地。
  旧实现证据覆盖浏览器 16 MiB +1 仍调用 `text()`、歧义/损坏 multipart 被首文件或 missing-file 语义
  接受、declared/chunked overflow 错映射，以及磁盘 multipart 临时文件残留。
- 前端在读取前拒绝已知超限 `File.size`；后端在认证与 `CanEditSources` 后统一执行 17 MiB actual-read
  包络、恰好一个 `file`/零 scalar 的 shape、16 MiB file read，并在所有已建立 form 的返回路径调用
  `RemoveAll()`。数组/wrapper/单对象、5,000 项、identity/COW/quota、事务和事件合同未改变。
- Go focused/full、focused race 与 `go vet ./...` 通过；frontend focused 与全量 `738/738`、Vite build
  通过。真实 Go + Chromium 在 1440x900、390x844、360x800 验证超限 chooser 零请求、正常预览选择、
  唯一 file 持久导入，并通过 direct API 的认证优先级、declared/chunked 双层 413、shape 和零副作用检查。
- 本地 arm64 candidate 通过 fresh portable-v1/v2-assets、cross-user、restart 和 source ownership/COW 门。
  historical 首次普通运行在 fixture 后出现一次瞬时 404；同镜像 trace 重跑通过 TXT/EPUB/UMD/CBZ、
  relative-cache、owner isolation 和 restore 全链，该未复现事件保留在发布台账。
- 本机为 `linux/amd64`、`linux/arm64` 构建并发布 `ghcr.io/changshengyu/openreader:3f3c9c8` 与
  `latest`；两标签指向 OCI index
  `sha256:62ee55ffab7859aef4334f8fb8dd31520953521da494edd5f37cc56741731070`。GHCR 强制回拉容器的
  `/api/health` 报告完整 revision `3f3c9c8461e60a12dd0ba08ce4a4f95860dbf319`。

本合同现为 **aligned / regression-validated / Docker-published / awaiting-device-verification**。reading
progress 与其它 batch/control JSON 仍是独立动作差集，不因本入口关闭而合并签收。
