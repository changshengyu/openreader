# Explore 请求入口、分页与取消生命周期第二轮固定基准合同

审查日期：2026-08-26

状态：**inventory-complete / tests-and-implementation-pending**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审 `GET /api/explore/:sourceId` 的 source 归属、探索入口、页码、远程请求取消、失败缓存和
稳定错误。Index 书海 chooser、探索结果卡、BookInfo、临时 Reader、书源解析器和通用远程抓取预算
已有专项合同；不因本轮请求生命周期修复重开可见 UI 或解析语义。

## 1. 权威源码

### reader-dev

- `src/main/java/com/htmake/reader/api/controller/BookController.kt:621-643`
- `web/src/components/Explore.vue:153-229,245-273`
- `src/main/java/io/legado/app/model/webBook/WebBook.kt`

固定上游先认证并解析当前 namespace 的书源。chooser 从该源的 `exploreUrl` 投影入口，选择入口时
发送同一 `ruleFindUrl` 与 `page=1`；“加载更多”只把 page 加一并复用同一入口。服务端以该入口执行
一次当前页远程工作。上游没有服务端页码上限、请求取消和多用户 COW，这是 OpenReader 必须明确的
安全/技术栈适配，而不是删除入口、分页或结果顺序的理由。

### OpenReader

- `backend/api/server.go:205-206`
- `backend/api/explore.go:29-91`
- `backend/engine/source_parser.go:258-336`
- `backend/engine/source_request.go:60-116`
- `backend/api/source_failure.go`
- `frontend/src/api/explore.js`
- `frontend/src/components/workspace/ExploreWorkspacePopover.vue`
- `frontend/src/views/Discover.vue`

## 2. 差异矩阵

| 合同点 | 固定上游 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 身份与 source | 当前 namespace 书源 | JWT user + active owned/COW source，foreign/detached 返回 404 | **aligned security adaptation / 保持** |
| 入口来源 | chooser 只发送该书源 `exploreUrl` 中选中的 `ruleFindUrl` | API 接受任意非空 `url`/`exploreUrl` override | **must-fix**：任意公网 URL 可成为携带 source headers/proxy policy 的首次请求 |
| 页码 | 初始 1，每次只加一 | 只验证可解析且 `>=1`，没有上限 | **must-fix**：API 已承诺 bounded pagination，应与 RSS 可见分页使用同一 `1..100000` admission |
| query 工作量 | chooser 发送已持久化短入口 | override 可占满 512 KiB header 包络 | **must-fix**：入口实际 UTF-8 bytes 最大 8192，与远程 probe URL 边界一致 |
| 请求取消 | Vert.x coroutine/连接生命周期 | handler 调用 `ExploreBooksPageWithURL`，内部强制 `context.Background()` | **must-fix**：浏览器离开/超时后仍继续 fetch/parse，迟到请求失败还可能写 `source_failures` |
| 返回与 UI | 当前页结果，前端合并并去重 | `{items,page,hasMore,nextUrl}` + request gate | **aligned / 保持**；不改字段、顺序、chooser/结果状态或“没有更多”反馈 |

## 3. 稳定 API 合同

### `GET /api/explore/:sourceId`

- JWT 必需。必须先按 caller 查找 active、enabled 且具有 explore 能力的 source；missing、foreign、
  detached 或禁用 source 保持 `404 {"error":"source not found"}`。该目标优先级高于 query 校验，避免
  用错误差异探测其它 namespace 的 source。
- query 保持 `page`、`url` 和兼容别名 `exploreUrl`。`url` 非空时仍优先于 `exploreUrl`；两者都空时
  保持 engine 的默认 explore rule 行为。
- `page` 省略时为 1；只接受十进制 `1..100000`。其它值返回
  `400 {"error":"invalid page"}`，不得远程请求或写失败缓存。
- 非空入口 trim 后实际 UTF-8 bytes 不超过 8192，并且必须精确等于该 source 当前规则经
  `parseExploreGroups` 投影出的一个 entry URL。相对 URL、`{page}`/`{{page}}`、URL options、JSON
  `layout_flexBasisPercent/layout_flexGrow` 和空行分组继续可用；服务端在 admission 后仍由 engine 按
  source base URL 解析和替换页码。
- 超长或未声明入口统一返回 `400 {"error":"invalid explore URL"}`；不得回显入口、source URL、
  header/cookie/proxy、host path、响应内容或底层错误。
- 成功保持 `200 {items,page,hasMore,nextUrl}`；items 顺序、cover capability、source projection、
  parser 和 pagination 规则不变。远程/规则失败继续使用已有 `error` + 可选 `code/stage=explore`。

`GET /api/explore/sources` 继续只投影当前用户 active explore sources 及已安全解析的入口分组；本轮不
改变其 JSON、顺序或无远程请求语义。

## 4. 入口所有权与安全边界

- source 行和 parsed rule 是入口 allowlist 的权威。请求不得把客户端 override 当成新 fetch
  capability，也不得只做 same-origin 字符串检查后接受未声明路径。
- 精确匹配发生在 trim 后、URL resolution/placeholder substitution 前。这样保持 reader-dev chooser
  发出的原始相对模板和 URL options，同时拒绝客户端构造的新 host/path/body/header option。
- 已声明 entry 自身可以是跨 origin URL；这是 source 作者持久化的显式能力。通用 SSRF、redirect、
  response-size、timeout、proxy 和跨 origin redirect header stripping 继续由已发布 fetcher 合同约束。
- 当前用户无书源编辑权限时仍可使用 owner/default source 已声明的入口，但不能借 override 扩大它；
  本轮不改变 source COW、identity、配额或默认书源镜像。

## 5. Context、失败缓存与副作用

- handler 必须调用 `ExploreBooksPageWithURLContext(c.Request.Context(), ...)`。请求在 fetch 前取消时零
  remote work；DNS/dial/read/parser 中取消时尽快停止后续工作。
- `context.Canceled`/`DeadlineExceeded` 或 request context 已结束时，handler 不再写业务错误响应、
  不调用 `recordSourceFailure`，也不产生 Explore 业务行、WebSocket、cache/library/data 或浏览器持久
  副作用。认证层既有 session 滑动和 `last_active_at` 不属于本动作，不在本轮重开。
- 真正的 source request failure 仍按现有 600 秒、caller/source-scoped failure cache 记录；rule
  validation/parser error 和无结果不记录。成功探索仍是只读动作，不新增事件或数据库行。
- 前端现有 workspace/session request gate 继续拒绝迟到 UI 提交；本轮不要求改路由、loading 文案、
  chooser 几何或 Axios 公共超时。

## 6. 数据、迁移与回滚

- 不增加表、列、索引或迁移 marker，不扫描或改写现有 BookSource/SourceFailure。
- 不改变 `data/cache/library`、backup/portable/Legado/WebDAV、环境变量或 Docker volume。
- 升级后旧书源入口原样可用；旧客户端发送 source 当前声明的 `url`/`exploreUrl` 仍成功。未声明
  override 从可执行远程请求收紧为 400，是明确的安全修复。
- 回滚旧版本可继续读取相同 SQLite 和 source JSON；本轮没有不可逆数据写入。

## 7. 测试先行门

实现前必须在旧实现上锁定失败：

1. missing/foreign/disabled source 保持 404 且先于 invalid page/URL；匿名保持 401。
2. page 缺省、1、100000 成功并精确替换模板；0、负数、非数字、100001 返回固定 400 且零 remote。
3. 8192-byte declared entry 可执行；8193-byte、未声明相对/同源/跨源 URL 和客户端注入 URL options
   均固定 400、零 remote、零 `source_failures`，错误不含原值。
4. JSON/line/default group 中声明的相对、绝对、`{page}`/`{{page}}` 与 POST URL options 继续按现有
   request method/body/header 执行，成功 JSON shape/顺序不变。
5. 阻塞远程请求开始后取消 caller：上游请求 context 被取消，handler 结束，不写 failure row；真正
   timeout/request failure 仍记录当前用户且不影响其它用户。
6. focused/race、Go 全量/vet、frontend 全量/build、真实 HTTP、Explore chooser/result 四视口和
   trusted Actions fresh/historical/portable gates 通过后，才可发布 amd64/arm64 OCI index。

## 8. Inventory 结论

判定：**must-fix**。现有公开 API 文档已承诺 bounded explore fetch 与 caller cancellation，但 handler
仍允许任意 override、无上限 page，并丢弃 request context。修复范围只收紧远程能力和生命周期，
不改变固定上游可见 chooser、分页、结果或 OpenReader 已发布的多用户/API 适配。

本提取阶段只新增/更新合同与审计矩阵，没有修改应用或测试代码。
