# 访问日志 Query 脱敏固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
当前实现基线：`OpenReader@b21bd6b`
审查日期：2026-08-25
状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

## 1. 范围

本轮只覆盖共享访问日志对 request target 的投影：

- 所有 route、状态码和认证结果上的 raw query；
- 已有 WebSocket JWT 与 EPUB/CBZ/audio/chapter-image/cover capability 脱敏；
- method、status、latency、client IP 和无敏感 path 的可观测性；
- 大 query 的日志放大与 percent-encoded credential/keyword/path 泄漏。

handler 如何读取 query、API 路径/响应、认证、业务日志、请求 body/header、WebDAV 协议、文件和数据合同
均不重开。本轮不改变请求对象，只改变一行 access log 的显示文本。

## 2. 固定上游与安全适配

固定上游 `RestVerticle.kt` 安装 Vert.x `LoggerHandler`，并对 `/reader3/*` 额外记录 decode 后的
`absoluteURI()`；小于 1000 字符的非 PUT body 还会被记录。该行为说明 method/path 访问日志是上游运维
合同，但不能作为 OpenReader 多用户/JWT 部署复制 query、body 或 credential 的依据。

OpenReader 已有明确安全适配：`middleware.AccessLogger` 不记录 header/body，并专门遮蔽
`/ws/sync?token=...` 和五类公开 capability。仓库其它专项合同也反复要求日志不得包含 JWT、URL query、
书源 credential、用户搜索文本和 host path。因此保留 path、统一遮蔽 query 是现有策略的补全，不是
删除业务行为或可见功能。

## 3. 当前差异与真实反例

| 边界 | `OpenReader@b21bd6b` | 裁决 |
|---|---|---|
| WebSocket/capability | JWT query 与 capability path token 已有专项替换。 | **aligned / must-preserve**。 |
| 其它 query | `RedactAccessPath` 原样返回普通 request target；Gin formatter 的 `params.Path` 含 raw query。 | **must-fix**。handler 是否认识参数不能决定日志是否泄漏。 |
| 敏感面 | content search `q/keyword/url/bookUrl`、Explore `url/exploreUrl`、LocalStore `path`、RSS sort URL、source group 等均走 query。 | **must-fix**。书名、正文短语、文件路径、远端 userinfo/token 可进入日志。 |
| 放大 | query 受 HTTP header/request-line 总边界约束，但 access logger 会逐请求输出完整值。 | **must-fix**。认证前、404 和无效参数同样可制造大日志行。 |

真实 `f394c1a` 二进制收到：

`GET /api/health?q=private-reading-phrase&url=https%3A%2F%2Fuser%3Apassword%40source.example%2Fbook%3Ftoken%3Dsecret`

返回 200 后，access log 原样包含完整 `q` 和编码后的 userinfo/token。该 route 不消费这些参数，证明
不能靠各 handler 白名单修补；共享 formatter 必须在任何路由匹配、认证和状态码之外统一投影。

## 4. 目标日志合同

1. `RedactAccessPath` 先按第一个 `?` 分离 path 与 query。只要 request target 含 query delimiter，日志
   只保留脱敏后的 path 加固定 `?<redacted>`；不得保留 key、value、分隔符数量、长度或编码形态。
2. 不调用 `url.QueryUnescape`，不解析 query，不根据参数名做 allowlist。malformed、重复、空 value、
   percent-encoded、UTF-8、userinfo、JWT 和未知参数使用同一固定投影。
3. 现有五类 capability path 继续只把 capability segment 替换为 `<redacted>`，可保留其后无敏感的
   resource path；若同时有 query，结果为 `<redacted>` capability path 加 `?<redacted>`。
4. `/ws/sync?token=...` 继续输出 `/ws/sync?<redacted>`；实现应由统一 query 规则得到同样结果，不再维护
   只针对 token 参数的特殊分支。
5. 无 query 的普通 path、method、status、latency 和 client IP 保持；query 的存在仍通过固定 marker
   可见。access logger 继续不记录 Authorization、Cookie、WebDAV credential、body 或响应内容。
6. 脱敏只作用于 formatter 的字符串副本。handler 必须仍能读到完整 `c.Query`/`RawQuery`，响应、错误、
   DB/file/event/remote fetch 副作用与认证优先级完全不变。
7. 输出长度不随 query 长度增长；含 256 KiB query 的单行日志只能比无 query path 增加固定 marker。
   不新增 hash、截断前缀或 debug bypass，避免形成可关联 secret。
8. 本轮不扩大 path segment 脱敏范围；WebDAV/upload/backup filename 等 path 隐私若有固定上游与真实
   反例，必须另立合同，不能借本轮无证据改变运维 path 可见性。

## 5. 测试先行门

1. 在旧实现上锁定普通 200、认证 401、404 三种请求的 query secret 均进入 formatter 输出；目标输出
   只含 path 和 `?<redacted>`，且 handler 仍收到原 query。
2. 覆盖 legacy content search 的 `url/bookUrl/keyword`、modern `q`、LocalStore `path`、Explore URL、
   arbitrary health query、空 value、重复 key、encoded userinfo/token 和 Unicode；日志不得出现原始或
   decode 后 secret。
3. 保留 queryless ordinary path、WebSocket、五类 capability/resource path 现有断言；新增 capability
   query 组合和“capability prefix 只出现在 query”反例，避免 query 参与 path token 解析。
4. 用 256 KiB raw query 证明 formatter 输出固定有界，且请求仍按现 route 的 header/handler 合同处理。
5. focused middleware、focused race、`go vet ./...`、Go 全量、frontend 全量/build、真实二进制
   200/401/404/large-query 日志探针及 fresh/historical/portable/restart 卷门通过后才可发布 Docker。

## 6. 数据与允许差异

- 不新增 schema、migration、startup scan、目录、环境变量、route、payload、backup member、manifest、
  浏览器 key 或 UI；`data/cache/library` 不扫描、不移动、不重写。
- 固定上游记录完整 URI/body 的行为被明确列为不复制的安全差异。OpenReader 仍保留 method/path/status/
  latency/client IP，统一 query redaction 只减少 credential、阅读隐私与日志放大风险。
- 既有日志解析器若依赖 query 明文，必须改为使用固定 `?<redacted>` marker；不得通过配置重新打开 secret
  logging。该变化不属于持久用户数据迁移。

## 7. 实施顺序

1. 独立提交本合同及 API/REST/gap/matrix/security/data inventory，不改应用或测试。
2. 在 `b21bd6b` 旧实现上提交 formatter/integration/bounded-output 红测。
3. 只修改共享 access-path 投影，删除冗余 WebSocket 特判并保留 capability path helper。
4. 完成 focused/race/full/runtime/卷门；形成可验证切片后提交推送并只在本机发布 amd64/arm64。

## 8. 实施与发布证据

- 合同 `9161ce5`、旧实现红测 `cce9efd` 和实现 `f88ecec` 已依次提交并推送 `main`。实现只在
  `RedactAccessPath` 中先按第一个 `?` 分离 path，再沿用 capability segment 脱敏并追加固定
  `?<redacted>`；WebSocket 特判被统一规则替代。handler 的 `RawQuery`、route、auth、status、响应和
  副作用未改。
- 旧实现测试先证明普通/legacy/LocalStore/Explore/capability 组合、200/401/404 和 256 KiB query 会泄漏
  原值并按输入放大。实现后 focused middleware、focused race、`go vet ./...`、Go 全量、frontend 741/741
  和 production build 均通过。
- 真实 `f88ecec` 宿主二进制在 health 200、未认证 books 401、缺失 upload 404 和 256 KiB health query
  上均只记录 path 加固定 marker；完整原始/编码 credential、阅读短语和大 query 未进入日志。SIGINT
  cleanup 正常完成。
- 本地 candidate 的 fresh portable-v1/v2-assets/cross-user/restart，以及 historical TXT/EPUB/UMD/CBZ/
  relative-cache/owner-isolation 卷门通过。没有 schema、目录、archive、backup 或既有日志迁移。
- 本机发布 `ghcr.io/changshengyu/openreader:f88ecec` 与 `latest`。两标签共同指向 OCI index
  `sha256:832216dbacb0650a5a6cb30b14731432714f4d48393516aed10c957a97549a29`；amd64/arm64 manifests 分别为
  `sha256:ebaaa5deaa6cc2e11d68efc5025ad6fe0d28804ab91e161b8802b207e6642684` 和
  `sha256:16f330ee7108dbacae53c72a44d5080c27651944bc21fbb35d137b8486c6eae0`。远端两平台 config 均确认
  完整 revision `f88ecec460e71145bf12432b012fa03a54c5796e`。用户生产环境运行提交仍未知。
