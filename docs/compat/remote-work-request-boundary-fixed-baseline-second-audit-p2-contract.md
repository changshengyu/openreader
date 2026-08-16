# 远程工作 JSON 请求边界固定基准第二轮合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本轮按合同、失败测试、实现、浏览器/卷门和本地 Docker 发布顺序完成。已发布的 Index 搜索、书源
调试、失效缓存和 BookManage 整本缓存状态机继续成立；本合同只关闭这些动作此前尚未签收的客户端
JSON、单次工作量和取消边界。

## 1. 范围与权威证据

覆盖七个已部署动作：

- `POST /api/search`
- `POST /api/sources/:id/test`
- `POST /api/sources/:id/test-chapter`
- `POST /api/sources/:id/test-content`
- `POST /api/sources/batch-test`
- `POST /api/books/:id/cache`
- `POST /api/books/:id/cache/stream`

固定上游：

- `web/src/views/Index.vue`、`web/src/plugins/config.js`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#searchBookMulti`
- `BookController.kt#searchBookMultiSSE/#bookSourceDebugSSE/#cacheBookSSE`
- `src/main/java/com/htmake/reader/api/controller/BaseController.kt#limitConcurrent`
- `web/src/components/BookManage.vue`

当前对应：

- `frontend/src/views/Search.vue`、`utils/searchPreference.js`
- `frontend/src/components/workspace/SourceManager.vue`
- `frontend/src/composables/useOverlayBookItemActions.js`
- `backend/api/search.go`、`source_debug.go`、`books.go`、`cache_stream.go`
- `backend/engine/fetcher.go`、`services/chaptercache`、`services/sourcedebug`

`POST /api/sources/:id/debug/stream` 已遵守 16 KiB、单 JSON、取消和有界事件合同，由
[`source-debug-fixed-baseline-second-audit-p2-contract.md`](source-debug-fixed-baseline-second-audit-p2-contract.md)
继续权威，本轮不得重写其状态机。Reader 换源候选也不在本合同范围。

## 2. 固定上游与当前差异

| 边界 | 固定上游行为 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 多源并发 | 新用户默认 24；可见选项 `12..60`。服务端按 `concurrentCount` 分批。 | 前端已保留默认、上游选项和旧 `8/16/32` 偏好；后端对任意正数不设上限，可一次为所有源创建 goroutine。 | **must-fix security adaptation**：非正数仍回退 24，正数最多 60；旧偏好继续有效。 |
| 搜索轮次 | `concurrentLoopCount=8`；每轮达到并发窗口后可因结果数量、断开或八轮停止。 | 多源请求会一直扫描到结果数满足或源耗尽；无分页兼容请求必扫完全部源。 | **must-fix work boundary**：每个请求最多检查 `8 * normalizedConcurrent` 个稳定 source ordinal；失败缓存跳过项也消耗 ordinal，不能借跳过越过预算。 |
| 搜索游标 | 从 `lastIndex + 1` 继续；返回已扫描源位置。 | 已有稳定原序列 cursor 和 `hasMore`，失败缓存不会重编号。 | **aligned**：保留；达到八轮上限时返回真实 cursor 和 `hasMore=true`。旧数组响应用附加 header 暴露截断，不改 body。 |
| 手工健康检查 | 固定上游普通失败列表依赖短期缓存；OpenReader 的即时 `batch-test` 是显式允许增强。 | 并发夹到 3..15，但可选择任意多源，并先为每个源创建一个 goroutine，再由 channel 阻塞。 | **must-fix bounded adaptation**：原始 ID/实际源最多 300；只创建最多 15 个 worker，取消停止领取新任务。 |
| 三个旧调试探针 | 上游规范入口是自动 SSE 调试链。三个 REST 探针仅为 OpenReader 部署兼容。 | body 无上限、接受首个 JSON 后的尾随文档；空白 URL/keyword 可进入解析器，且同步 helper 不传请求 Context。 | **must-fix wire/cancellation**：保留兼容响应，只增加有界单 object、短字段和取消；仍不写 `source_failures`。 |
| BookManage 缓存 | 从第 0 章缓存整本；断开停止后续工作。 | 整本范围、顺序变量语义、进度和取消已发布；两个 JSON 入口仍无 actual-read/single-document 上限。 | **must-fix wire only**：不得以安全名义把整本改回 20/50/300 章；只约束 body 并保留 owner/cancel。 |
| 远端 transport | 上游没有 OpenReader 的 Go 网络安全层。 | 共享 fetcher 已有 HTTP(S)、SSRF/DNS、15 秒默认 timeout、16 MiB response、redirect/retry 和脱敏。 | **aligned security adaptation**：本合同不复制 transport，也不放宽既有边界。 |

选择该批为下一项 must-fix 的理由是：搜索可由一个小 JSON 放大为最多全部用户书源的并发抓取，
`batch-test` 又会按源数创建等待 goroutine；两者的网络/内存放大面高于单次进度写。书源 multipart、
阅读进度 wire 和其它批量 JSON 保留在动作差集，不能因本合同建立而标为关闭。

## 3. 共同 wire、认证与错误合同

- 七个动作继续先通过 Bearer JWT。缺失/无效 token 保持 `401`，不得为了 body 大小先读取请求。
- 带 `:id` 的三个探针先解析正整数并验证 caller-active source；两个缓存动作先验证 caller-owned book。
  因此非法 ID `400`、foreign/missing `404` 继续优先于 body 错误。该顺序与已部署 API 一致。
- search body 最大 `64 << 10` bytes；其余六个 control body 最大 `16 << 10` bytes。字节数包含 JSON
  标点、转义、未知字段和尾部空白。精确边界进入业务校验，declared/chunked `+1` 均为
  `413 {"error":"request body too large"}`。
- 每个 body 必须是一个有效 UTF-8、非 null JSON object。空 body、数组、scalar、`null`、第二 JSON
  或尾随非空垃圾使用该路由现有 malformed `400` 文本；JSON 尾部空白允许。未知 object 字段继续
  忽略，避免破坏部署中的扩展客户端。
- body、字段或 cardinality 失败发生在任何远程请求、缓存文件/章节写、失败缓存变更和完成广播前。
  owner/source 前置查询可以保留，但不得把拒绝请求记为书源失败。
- 客户端断开或 Context 取消不是 source failure。取消后不得领取/开始新的源或章节工作，不得发送
  虚假的成功终态；已提交的章节缓存文件和健康检查中已完成的真实结果仍可保留。

## 4. `POST /api/search` 合同

请求字段保持 `{keyword,sourceIds?,concurrentCount?,page?,lastIndex?,searchSize?}`；单源继续按 `page`，
多源继续按 `lastIndex`，不把两个 cursor 混用。

- `keyword` trim 后必须非空且最多 1024 UTF-8 bytes；失败保持
  `400 {"error":"keyword is required"}`，不得查询书源或抓取。
- 原始 `sourceIds` 最多 5,000 项，发生在去重、SQL 和排序前；5,001 项为
  `400 {"error":"too many sources"}`。5,000 与本地/远程 BookSource import 的既有单批上限一致。
  ID 只在当前用户 active namespace 中解析；外用户/detached ID 继续被排除。
- `concurrentCount<=0` 回退 24；正数夹紧到最多 60。保留旧偏好 `8/16/32` 和上游所有可见选项，
  不要求请求值必须属于 UI 枚举。
- `searchSize` 继续归一到 `1..200`，默认 20。单源只执行一个源页请求；多源每个 HTTP 请求最多
  八个并发窗口，每个窗口最多检查 normalized concurrency 个稳定 ordinal。
- 临时抑制源占用自己的 ordinal 但不发网络请求。一个窗口不能为了填满“实际请求数”跨过任意多
  suppressed row；响应 `lastIndex` 必须是最后检查 ordinal，`hasMore` 基于原始有序选择集。
- 指定 ID 的顺序、源失败局部容忍、现有结果去重、cover 投影、`source_failures` 记录和安全
  `error/code/stage` 保持。达到工作预算不是错误：分页响应为 `200`、真实 cursor 和 `hasMore`。
- 旧无 `page/lastIndex/searchSize` 请求继续返回顶层数组。若因八窗口边界仍有源，增加
  `X-OpenReader-Search-Truncated: 1` 和 `X-OpenReader-Search-Last-Index: <n>`；不得静默继续扫完整个
  namespace，也不得改变旧数组 body。
- 实现不得为整个无限 namespace 或每个未开始源创建 goroutine。每个活动 search batch 的远程
  goroutine 数最多 60；请求取消后所有已启动调用通过同一 Context 收敛。

## 5. 旧调试探针与批量健康合同

### 三个部署兼容探针

| 路径 | 字段限制 | 保留的成功/错误语义 |
|---|---|---|
| `/api/sources/:id/test` | trim 后 `keyword` 1..1024 UTF-8 bytes | `200 {results,error,code?,stage?}`；缺失/空白为 `400 {error:"keyword is required"}`。 |
| `/api/sources/:id/test-chapter` | trim 后 `bookUrl` 1..8192 UTF-8 bytes | `200 {chapters,count,error,code?,stage?}`；缺失/空白为 `400 {error:"bookUrl is required"}`。 |
| `/api/sources/:id/test-content` | trim 后 `chapterUrl` 1..8192 UTF-8 bytes | `200 {content,fullLength,error,code?,stage?}`；正文 preview 仍最多 2000 rune；缺失/空白为 `400 {error:"chapterUrl is required"}`。 |

三条探针改用对应 Context-aware engine 调用。解析/远程错误仍在 authenticated `200` diagnostic
envelope 中，不写 `source_failures`，不改 canonical debug stream 的自动状态链。

### `POST /api/sources/batch-test`

- body 保持 `{sourceIds?,keyword?,timeout?,concurrent?}`。`keyword` 空白仍默认“测试”，非空最多
  1024 UTF-8 bytes；超长为 `400 {"error":"invalid batch test payload"}`。
- 原始 `sourceIds` 最多 300。省略/空数组仍表示所有 caller-active source，但实际结果超过 300 时
  在任何远程请求前返回 `400 {"error":"too many sources"}`；规范 UI 已按 3..15 项分块，不受影响。
- `concurrent` 继续夹紧到 `3..15`，`timeout` 继续夹紧到 `1000..15000` ms，默认分别保持当前 3/5000
  语义。结果顺序仍按选定 source 顺序。
- 使用固定 worker pool；最多 15 个 worker/goroutine 领取最多 300 个任务，不能先按 source 数创建
  goroutine 再阻塞。取消后 worker 不领取新任务；只为取消前实际完成的真实请求结果更新 caller 的
  health failure cache。

## 6. 两个章节缓存入口

两个路径继续共享 `{chapterIndex?,all,count,refresh}`：

- 只增加 16 KiB、单个非 null object、UTF-8 和尾随拒绝；字段含义、unknown-field 兼容不变。
- `all=false` 仍要求 `chapterIndex`；`all=true,count<=0` 仍表示从指定 index（省略为 0）缓存到目录
  末尾。`count>300` 继续夹紧到 300；不能把明确整本请求夹紧到 300。
- 本地书、missing source、已有/失效缓存、`refresh`、next chapter URL、顺序变量、规范进度、旧字段
  alias 和混合/全失败终端语义继续由
  [`book-management-cache-p2-contract.md`](book-management-cache-p2-contract.md) 权威。
- REST 路径继续返回 JSON；stream 继续在 validation 后打开 SSE。取消不广播完成态书架更新，但已经
  原子写入的缓存仍可读。整个计划顺序执行，不新增按章节 goroutine。

## 7. 数据、权限与允许差异

- 不新增 SQLite 字段/表，不改变 backup/portable/WebDAV 格式，不移动或删除 `data/cache/library`。
- 失败缓存仍是 user/source scoped 派生数据；普通搜索可写真实远程失败，三个 debug probe 不写，
  batch health 可写已完成诊断失败。取消、body 拒绝和本地 validation 都不能写。
- 搜索最多 60 并发、八窗口、5,000 raw IDs；健康最多 15 worker/300 source；短 keyword/URL 与
  actual-read 上限均是允许的 Go/security adaptation。它们不复制固定上游的无界输入。
- 整本缓存、OpenReader 顺序 chapter fetch、JWT header、failure suppression 和 REST/SSE response
  adaptation 均保持已批准差异。本轮不改前端可见布局或产品命令。

## 8. 必须先写的失败测试

1. 七路 declared/chunked `+1`、精确 limit、第二 JSON、垃圾、`null`/array/scalar 和 invalid UTF-8；
   验证 `401` 以及 source/book ID/owner 优先级，拒绝请求零远程/缓存/失败行/event 副作用。
2. 搜索 5,000/5,001 raw IDs、1024/+1 keyword、默认 24、旧 8/16/32、60/61；证明每批最多 60
   goroutine、每请求最多八窗口、cursor/hasMore 正确，suppressed ordinal 不被跨越填充。
3. 旧无分页数组在八窗口截断时保持数组 body 并返回两个附加 header；正常小请求响应完全不变。
4. 搜索取消停止新 source，取消错误不写 failure；真实局部失败仍写当前用户记录且不污染另一用户。
5. 三个旧 probe 的空白/超长字段、Context 取消、既有 200 diagnostic envelope、2000-rune preview 和
   零 failure-cache 副作用。
6. batch-test 300/301、空 ID 选择超过 300、3/15 worker、timeout clamp、稳定结果顺序和取消；用阻塞
   transport 证明没有按全部 source 数预建 goroutine。
7. 两个 cache 路径的 16 KiB 边界、owner 前置、整本大于 300 仍完整、显式 count 仍夹紧、取消停止
   下一章且 stream 不发送伪 end/完成广播。
8. 前端现有搜索并发选项、旧偏好、分页 payload、健康分块和 BookManage `{all:true}` payload 静态/
   composable 合同必须继续通过；本批不新增另一套 UI 状态。
9. 真实 Go + Chromium 在 1440×900、390×844、360×800 覆盖多源八窗口续页、单源分页、健康分块、
   整本缓存与取消；随后跑 Go full/race/vet、frontend full/build、fresh/historical/portable 卷门。

只有红测、实现、完整回归、真实浏览器和本地 Docker 门都通过后，状态才能改为 `aligned` 并发布。

## 9. 实施与验证结果（2026-08-16）

- `5aadf9b` 只提交固定上游 inventory；`94d0a4e` 使旧实现在线路读取、顶层类型、搜索工作窗口、
  batch worker、取消和整本缓存边界上正式变红；`346a49d` 完成共享有界读取和七路实现；`6157466`
  增加真实 Go/Chromium 回归脚本。四个提交均已推送。
- search 现按实际读取执行 64 KiB 单 UTF-8 object、1024-byte keyword、5,000 raw ID、最多 60 并发和
  八个稳定 ordinal 窗口；旧数组响应保持 body 并在截断时返回真实 last-index header。三个兼容探针、
  batch health 和两个缓存入口共享 16 KiB control boundary；batch 只创建最多 15 个 worker，缓存整本
  语义未被 300 章显式 count 上限替代。
- focused contract、既有 search/source-debug/cache/failure suites、Go full、独立 sandbox 外 full race、
  `go vet ./...`、frontend 737/737 和 Vite production build 均通过。race 的首次 sandbox 运行仅因环境
  禁止 `httptest` IPv6 bind 失败，获准在同一代码上重跑后全量通过。
- `scripts/smoke/remote-work-request-boundary-contract.mjs` 在 1440x900、390x844、360x800 的真实 Go
  服务上通过：70 个源证明八窗口 64 ordinal 与后续 6 项续页、单源分页、14 个五源健康分块、整本
  缓存、真实 transport 取消和七路 413/单文档合同；既有四视口 source-debug smoke 同时通过。
- 本地 arm64 candidate 通过 fresh、portable v1/v2 assets、跨用户和重启卷门。历史卷首次普通运行在
  fixture 建立后遇到一次瞬时 HTTP 404；立即对同一镜像以 shell trace 重跑，TXT/EPUB/UMD/CBZ、
  relative-cache、owner isolation 和历史迁移全链通过，未形成可复现产品失败。

本机为 `linux/amd64` 与 `linux/arm64` 构建并发布
`ghcr.io/changshengyu/openreader:6157466` 和 `latest`；两标签均解析到 OCI index
`sha256:1e890a60a1b75879dd99074b1da13b17f91bbd4173e945b92cb8cec0fe8001b6`，平台 manifest 分别为
`sha256:02d2055bec076f2590e2a952c9dffad84c55c959965ea1f3dd212da5ef9ff424` 和
`sha256:84f17fca80e57e00bdd833c00d300b3cbd31dbaf706dd3ae20d475812037ff53`。两个平台 label 均记录完整
revision `6157466687c15d2ce48007443992523dd6a26834`。用户生产环境运行提交仍未知，发布不等于已升级。
