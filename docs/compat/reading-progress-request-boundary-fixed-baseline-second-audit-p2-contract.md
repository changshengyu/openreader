# 阅读进度请求边界固定基准第二轮合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本轮只提取合同，不修改应用或测试。2026-07-18 已发布的用户/章节身份、数据库 CAS、冲突响应、
WebSocket 收敛、existing-directory WebDAV 镜像和前端 keepalive 合同继续权威；本合同只关闭部署路由
`PUT /api/progress` 尚未签收的 HTTP JSON、字段 admission 和错误边界。

## 1. 范围与权威证据

固定上游：

- `web/src/views/Reader.vue#saveBookProgress`、离开/连续阅读触发和本地章节位置缓存；
- `YueduApi.kt` 的 `POST /reader3/saveBookProgress`；
- `BookController.kt#saveBookProgress/#saveBookProgressToWebdav`。

OpenReader：

- `backend/api/server.go`、`progress.go`、`request_body.go`；
- `backend/services/readingprogress/service.go`、`models.ReadingProgress`、同步 Hub 和备份恢复；
- `frontend/src/stores/reader.js`、`useReaderProgressPersistence.js`、`useSync.js`；
- `progress_p2_contract_test.go`、`readerProgressPersistence.test.mjs` 和
  `reader-progress-multiclient-contract.mjs`。

固定上游只提交书籍 URL 和显式目录 index；POST 缺省 index 为 `-1`。服务端先按用户 namespace 确认
书籍已在书架，再加载目录并以 index 取得规范章节，保存书架状态和既有 WebDAV feature directory。
OpenReader 的 JWT、数字 book/chapter identity、精确 offset/percent、SQLite CAS 和 WebSocket 是已批准
运行时适配，不在本轮改回上游文件模型。

## 2. 当前差异与裁决

| 边界 | `OpenReader@cfb06e9` | 裁决 |
|---|---|---|
| 路由与认证 | protected `PUT /api/progress`；JWT middleware 先于 handler。 | **aligned**：路径、Bearer JWT、`401` 文本与 auth-first 保持。 |
| JSON wire | `ShouldBindJSON` 没有 body limit；Gin 1.10 只执行一次 `Decode`，因此首个合法对象后的第二个 JSON 值可被忽略。无效 UTF-8 会由标准 decoder 替换。 | **must-fix**：16 KiB declared/actual-read 上限、单一 UTF-8 JSON object；overflow 稳定 413，其余 wire/shape 错误稳定 400。 |
| `chapterIndex` presence | value `int` 缺省为 0；请求省略该字段可把第一章当成显式选择并进入 CAS/镜像/广播。 | **must-fix identity**：字段必须显式存在且 `>=0`；这与固定上游显式 index 和既有章节规范化合同一致。 |
| 数值域 | `bookId>0` 由 binding；chapter/index/offset 身份由 service 校验；percent 在 service 夹紧 `[0,1]`。 | **aligned**：保持 book/chapter lookup、非负 offset、percent clamp 和安全 400/404。 |
| `baseUpdatedAt` / `clientUpdatedAt` | 无长度/语法校验；非法非空值会进入 fallback，两个时间都不可解析时可被当作 non-stale。 | **must-fix CAS control**：空值继续兼容旧客户端；非空值最多 64 UTF-8 bytes 且必须可按 RFC3339Nano 解析，否则 400、零副作用。 |
| `mode` | 直接持久化到声明 `size:20` 的列，SQLite 不替应用执行该限制。 | **must-fix stored field**：空/历史值兼容，但请求值最多 20 UTF-8 bytes；不在本轮新增 mode enum 或改写值。 |
| `clientId` | 不持久化，但原样放入 committed `progress_update`；当前无长度限制。 | **must-fix reflected control**：空值兼容旧客户端，非空最多 128 UTF-8 bytes；只用于同客户端回声抑制。 |
| `chapterTitle` / unknown fields | 客户端标题已被 service 忽略并由目录标题覆盖；标准 struct decoder 忽略 unknown fields。 | **aligned deployed tolerance**：保持 title compatibility field 与 bounded unknown fields；二者不得决定持久章节或副作用。 |
| CAS / mirror / broadcast | 已实现 user/book scoped CAS、200 conflict header、提交后镜像、赢家单广播。 | **aligned, do not reopen**：wire 拒绝必须发生在 service、SQLite、文件和 Hub 之前。 |

该项成为 BookSource multipart 后的下一批，因为每次普通阅读都会调用它；当前小请求可携带无界尾部，
而时间戳直接控制 CAS。它不授权重开阅读位置算法、Reader UI、书架 freshness、WebSocket 协议、备份恢复
或既有镜像目录规则。其它 batch/control JSON 继续留在动作差集。

## 3. `PUT /api/progress` wire 合同

- Bearer JWT 先于 declared length、body read、UTF-8/JSON/字段检查。缺失/无效 token 继续为现有
  `401 {"error":"missing bearer token"}` / `401 {"error":"invalid token"}`，不得读取请求 body。
- request body 最大 `16 << 10` bytes。`Content-Length > 16 KiB` 可在认证后直接拒绝；未知长度或
  chunked actual overflow 必须同样返回 `413 {"error":"request body too large"}`。
- body 必须是有效 UTF-8、恰好一个非 null JSON object。空 body、null、array/scalar、损坏 JSON、
  第二个 JSON 值和类型错误均返回 `400 {"error":"invalid progress payload"}`。bounded unknown object
  fields 继续忽略，避免破坏已部署客户端的附加 metadata。
- `bookId` 必须显式且为正整数；`chapterIndex` 必须显式且为非负整数。`chapterId` 可省略/为 0，
  `offset` 可省略为 0；非零 chapter ID、index 和当前用户目录身份继续由 service 一致裁决。
- `percent`、`chapterPercent` 继续允许省略并由 service 夹紧 `[0,1]`；JSON 非有限数本身不合法。
  不新增客户端正文长度读取，也不因本轮把 offset 与章节正文长度耦合。
- `mode` 最多 20 bytes；`clientId` 最多 128 bytes。空字符串继续有效；本轮只限制字节，不新增
  Unicode/字符 allowlist。规范前端仍只生成 `page/scroll/scroll2/flip` 与 `web-<UUID>`。
- `baseUpdatedAt`、`clientUpdatedAt` 各最多 64 bytes；空字符串保持 legacy 兼容，非空必须通过
  `time.Parse(time.RFC3339Nano, value)`。服务器只把自己的 UTC `updatedAt` 持久化为版本与排序事实。
- 所有 wire/field 拒绝使用现有 flat `{error}`；overflow 是 413，其余 admission 是 400。不得把
  invalid timestamp 降级为 non-stale，不得把 oversized field 截断后继续。

## 4. 业务、副作用与响应保持

- 当前用户不存在/外书为 404；目录 index 不存在、chapter ID 不匹配、负 offset 为 400。目录标题/ID
  由服务端规范化，客户端 `chapterTitle` 不持久化。
- existing/initial CAS、stale 判断、冲突 `200` + `X-OpenReader-Progress-Conflict: 1` 和赢家响应体保持。
  invalid/oversized 请求不能读取/创建/更新 `reading_progresses`，不能改变 shelf ordering。
- 只有 durable winner 才尝试 existing-directory WebDAV mirror，随后发送一个 caller-scoped
  `progress_update`。wire/field/identity/CAS loser/DB failure 不镜像、不广播；镜像失败仍只用现有
  path-free response header，不能回滚数据库事实。
- `clientId` 仅在 committed event payload 中回显，不进入 SQLite、WebDAV、backup、日志或响应 body。
  rejection 不得把该值或时间戳写入错误文本。
- `GET /api/progress/:bookID`、ReadingProgress schema、backup/portable/WebDAV restore、`data/cache/library`
  和旧 URL 均不改变，不新增迁移。

## 5. 前端适配合同

- 正常保存、离开 keepalive、pending snapshot、最多一次 conflict retry、operation generation 和其它客户端
  WebSocket 收敛不变；所有规范 payload 远低于 16 KiB。
- `readerClientId()` 不得盲信 sessionStorage 中超出服务端 128-byte 合同的历史/篡改值。缺失、空值、
  超限或非字符串等价值应生成新的 `web-<UUID>` 并覆盖 session key，避免后续每次进度保存都失败。
- Reader mode 已由现有 settings normalization 收敛到四个已知值；本轮不新增可见设置、提示、请求或
  本地进度格式。服务端 400/413 继续由现有离线 pending/retry 流程处理，不清除本地用户位置。

## 6. 允许差异

16 KiB 单 UTF-8 JSON、字段短界、严格时间戳和显式 chapter index 是 Go/JWT/SQLite 安全适配。固定上游
没有多浏览器 CAS/clientId，也只保存章节 index；OpenReader 保留精确 offset/percent、SQLite 独立行、
200 conflict compatibility、用户私有 WebDAV 根和 WebSocket，但不得借适配改变可见阅读位置。

## 7. 必须先写的失败测试

1. 无 token/无效 token 携带 declared/chunked overflow、invalid UTF-8 或 multi JSON 时仍先返回现有 401，
   并证明 body 未被消费、无 SQLite/镜像/Hub 副作用。
2. declared 与 chunked 16 KiB +1 为 413；精确 16 KiB 的单 object 可进入字段/业务裁决。空/null/array/
   scalar、损坏 JSON、无效 UTF-8 和两个 JSON 值均为 400。
3. 省略 `bookId` 或 `chapterIndex`、负 index/offset、类型错误和不存在章节为 400；省略 index 不得再默认
   写第一章。外书仍为 404。
4. `mode` 20/21 bytes、`clientId` 128/129 bytes、两个 timestamp 64/65 bytes 边界；非空非法 RFC3339Nano
   为 400。合法 server/client timestamp 保持一胜一 conflict，非法值不能绕过 stale CAS。
5. 所有拒绝路径证明原进度行、WebDAV mirror bytes 和 Hub 队列不变；合法 winner 仍恰好写一次镜像、
   广播一次且只在 event 中携带 bounded clientId。
6. 前端测试证明超限 session client ID 会被替换并持久到 sessionStorage；正常 UUID、keepalive、pending、
   conflict retry、账号 operation scope 与四种 mode 保持。
7. 真实 Go + Chromium 在 1440x900、390x844、360x800 复验普通/keepalive 保存、双客户端 CAS 收敛、
   超限旧 client ID 自愈、冷启动恢复和零回声 PUT。
8. Go full/race/vet、frontend full/build、fresh/historical/portable 卷门和既有 progress JSON restore 全部通过。

## 8. 实施与发布闸门

先提交能在 `cfb06e9` 失败的 API/前端合同，再实现。实现后运行 progress focused tests 与 race、
`go test ./...`、`go vet ./...`、frontend full/build、三视口真实进度合同以及 fresh/historical/portable
卷门。只有本地 amd64/arm64 构建、GHCR 推送和强制回拉 revision 核验完成后，状态才能改为
`aligned / regression-validated / Docker-published / awaiting-device-verification`。

## 9. 实施与验证结果（2026-08-16）

- 合同、旧实现红测、实现和真实运行时分别以 `f924604`、`a10facb`、`8d3790d`、`1563bc3` 顺序提交
  并推送。后端现在在 service、SQLite、WebDAV 和 Hub 之前执行 auth-first 16 KiB actual-read、单 UTF-8
  JSON object、显式 book/index、RFC3339Nano 与 mode/clientId 短界；前端会替换超限 session client ID。
- 聚焦与全量 Go、focused race、`go vet`、frontend 741/741、production build 和脚本语法检查通过。
  真实 Go + Chromium 在 1440x900、390x844、360x800 通过请求边界、client ID 自愈、双客户端 CAS、
  WebSocket 收敛、冷恢复和 WebDAV winner mirror。
- `PUSH=0` 已从 `1563bc3` 完成本机候选构建，arm64 revision label 为完整提交
  `1563bc3b0d92b274761743f8629d17f845da9096`。本轮 mounted-volume 门因 Codex Docker socket 使用额度
  在授权阶段被拒绝，未将未执行的 fresh/historical/portable 验证记为通过，也未执行 `RELEASE=1`。

后续 Book 控制动作切片恢复了 Docker gate：包含该进度实现的 `65199f6` 候选通过 fresh 与
historical/portable mounted-volume 门，并由本机构建发布为 `65199f6`/`latest`。两个标签共同指向
amd64/arm64 OCI index `sha256:57eda43d437d98a4f2d748164d58c5816f3ff3dc199397bd9dc8f6d48334a8cb`；
强制回拉 arm64 后 revision 为完整 `65199f666723010beb39a982f941e18af3927697`。因此本合同的 release gate
现已关闭；用户生产环境运行提交仍未知，继续等待真实设备签收。
