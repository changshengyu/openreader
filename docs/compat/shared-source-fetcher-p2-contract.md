# 书源 / RSS 共享远程抓取器 P2 合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：**P2-N1 aligned / Docker-published；P2-N2 aligned / Docker-published / awaiting-device-verification**。
2026-08-09 已完成固定上游、当前 Go 实现和调用面的源码盘点，并按两个可独立验证的安全切片推进：
不改变合法目标可达性的 `P2-N1` 通用请求边界已经测试先行实现和发布；需要显式部署策略的
`P2-N2` 私网地址边界已按合同实现，并通过代码级、真实 Docker 网络、新旧卷和镜像发布门；共享
Source/RSS 抓取器的本合同 SSRF 债务已经关闭，后续 parser/upload 资源上限仍由独立 P2 合同追踪。

## 权威文件

上游：

- `src/main/java/io/legado/app/help/http/HttpHelper.kt`
  - 全局 OkHttp client；connect/write/read timeout 均为 15 秒；允许 HTTP/HTTPS redirect；
  - 允许 cleartext，并使用不安全 TLS verifier（OpenReader 不复制这一行为）；
  - 支持 HTTP/SOCKS4/SOCKS5 source proxy。
- `src/main/java/io/legado/app/help/http/OkHttpUtils.kt`
  - 非 2xx 按 URL option 的 `retry` 重试；最终仍返回响应 body；
  - `ResponseBody.text()`/`bytes()` 会把完整响应读入内存，没有统一字节上限。
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeUrl.kt`
  - GET/POST、headers/body/charset/retry/type/proxy、相对 URL 和最终 URL 语义；
  - source header、URL-option header 和 cookie 参与请求。
- `src/main/java/io/legado/app/data/entities/BaseSource.kt`、`BookSource.kt`、`RssSource.kt`
  - 书源/RSS 源本身就是用户配置的网络目标；上游没有私网、metadata、userinfo 或 DNS-rebinding 防护。

当前：

- `backend/engine/fetcher.go`
  - 书源和 RSS 共用入口；12 秒 `http.Client.Timeout`、context cancel、限速、retry、charset、binary hex、
    HTTP/SOCKS4/SOCKS5 proxy 已实现；
  - `io.ReadAll(response.Body)` 无上限；redirect 使用 Go 默认 10 次；URL scheme/host/userinfo 未统一校验；
    redirect 后 source headers 的跨 origin 保留没有项目级合同。
- `backend/engine/source_request.go`、`source_parser.go`
  - URL option 与所有搜索、探索、详情、目录、正文链路；`retry` 当前没有上限。
- `backend/api/rss.go`
  - RSS feed、rule page、正文详情均走共享 fetcher；source URL/sort URL ownership 已在 RSS 第二轮锁定，
    但 transport/body/redirect 仍继承共享缺口。
- `backend/api/sources.go#fetchRemoteBookSources`
  - 远程书源 JSON 预览/导入直接调用无 source policy 的 `engine.FetchText`。
- `backend/services/chapterimage`、`coverimage`
  - 已有独立的 URL、DNS/dial、redirect、timeout 和 byte-cap 实现，只作为安全设计参考；本项目不得
    让共享文本 fetcher 反向削弱这两个已经发布的专用资源合同。

## 固定行为矩阵

| 项目 | 固定上游行为 | 当前 OpenReader | 判定 / 目标 |
|---|---|---|---|
| 方法与请求体 | GET/POST；URL option 可携带 headers/body/type/charset/retry/proxy。 | 已实现。 | `must-preserve`；安全边界不得改写合法请求字段。 |
| 状态码与 retry | 非 2xx 可按 `retry` 重试，最后仍交给解析器处理 body；transport error 不伪造成功。 | 已实现，但 retry 无上限。 | `acceptable security change`：保留响应语义，把 retry 限为最多 3 次重试（4 次请求）。 |
| charset / binary | 文本按显式或自动 charset 解码；`type` 非空返回原始 bytes 的 hex。 | 已实现。 | `must-preserve`；byte cap 在解码/hex 之前生效。 |
| timeout | 上游 connect/write/read 各 15 秒。 | 当前 client 总 timeout 12 秒；测试可注入 client。 | `technology-equivalent`：增加显式总 timeout 配置，默认 15 秒；调用方 context 先取消时仍优先退出。 |
| response bytes | 上游完整读入，无上限。 | 同样无上限。 | `must-fix security`：默认单响应 16 MiB；先检查可信的正 Content-Length，再用 `LimitReader(max+1)` 验证实际 body。 |
| redirect | 上游跟随 redirect；没有产品级次数/目标合同。 | Go 默认最多 10 次。 | `must-fix security`：显式最多 5 次；每一跳重新校验 URL，跨 origin 不携带 Cookie、Authorization、Proxy-Authorization 及自定义凭证头。 |
| URL 语法 | 上游允许网络库接受的 URL。 | 未统一校验。 | `must-fix security`：只允许绝对 HTTP/HTTPS、非空 host、合法 port、无 userinfo；fragment 不发送。 |
| 错误 | 上游错误可能包含请求信息。 | 部分 API 已净化，RSS 正文抓取仍可能拼接底层 `url.Error`。 | `must-fix security`：公开错误保留既有 status/top-level `error`，但不得包含 query、userinfo、headers、body、proxy credential 或主机文件路径。context cancel/deadline 仍可 `errors.Is`。 |
| TLS | 上游关闭证书/hostname 校验。 | Go 默认安全 TLS。 | `allowed security difference`：必须保持证书与 hostname 校验，不能为“兼容”关闭。 |
| 私网目标 | 上游不限制，局域网书源天然可用。 | 共享 fetcher 不限制；专用图片抓取器有 trusted-host 规则。 | `P2-N2 design-required`：不能在 N1 粗暴封禁。默认严格拒绝 private/loopback/link-local/metadata；部署管理员可通过明确 host/CIDR allowlist 恢复 NAS/局域网源，且每次 DNS 与 redirect/dial 均复核。 |
| source proxy | 上游支持 HTTP/SOCKS4/SOCKS5 和凭证。 | 已实现。 | `must-preserve with bounds`：代理地址本身也需 URL/port/私网策略；凭证永不出现在错误/日志。P2-N1 保留现有可达性，P2-N2 与目标地址一起处理。 |
| 持久化 | 网络策略不进入书源/RSS 导出格式。 | 无相关配置列。 | `must-preserve`：新增限制只用环境配置，不迁移或重写 SQLite、备份、source JSON。 |

## P2-N1：通用请求边界

### 配置

新增无损环境配置，未设置时使用：

- `OPENREADER_SOURCE_REQUEST_TIMEOUT_SECONDS=15`
- `OPENREADER_MAX_SOURCE_RESPONSE_BYTES=16777216`
- `OPENREADER_MAX_SOURCE_REDIRECTS=5`
- `OPENREADER_MAX_SOURCE_RETRIES=3`

这些值只控制新发起的远程请求，不修改数据库、缓存文件、书源/RSS JSON 或备份。非法、零或负值
回退到安全默认值。测试注入的 client 仍可替换 transport；生产 policy 必须显式包装 timeout、redirect
和 body reader，不能把安全性依赖于 Go 的隐式默认值。

### 请求 / 响应合同

1. 初始 URL 和每次 redirect 都必须是绝对 `http`/`https`，有 host、合法端口且无 userinfo；
   `file:`、`data:`、`ftp:`、scheme-relative、空 host 和含凭证 URL 在 transport 前失败。
2. redirect 次数从第一跳起计，允许 5 跳，第 6 跳失败；same-origin 保留 source headers，cross-origin
   只保留无凭证的浏览器协商头（`Accept`、`Accept-Language`、`Cache-Control`、`Pragma`、
   `User-Agent`），并由 Go 正常决定 301/302/303/307/308 的方法/请求体行为。
3. 单次响应 body 先按 16 MiB 上限读取。`Content-Length > limit` 或实际第 `limit+1` 字节均返回同一
   有界错误；不能进入 charset detector、goquery、XML/JSON parser 或 binary hex。
4. 每个非 2xx 响应在关闭 body 后按上游 retry 语义重试；最多 3 次 retry。最后一个非 2xx body
   仍按既有行为返回给 parser。超限、非法 URL、redirect-limit、context cancel、deadline、transport
   error 不重试。
5. 所有返回路径必须关闭 response body。错误字符串不得回显 URL/query、headers/cookies、请求/响应
   body 或 proxy credential；`ErrSourceRequest` 分类和 context sentinel 必须继续可由调用方识别。
6. `SetHTTPClient` 测试接口继续可用，但恢复后不能泄漏前一 client/policy；并发测试不得造成 data race。

### API 与可见行为

- 不新增/删除路由，不改变成功 JSON schema、状态码、文章/书籍顺序、source failure 分类或前端流程。
- `/api/search`、Explore、远程详情/目录/正文、换源、更新检查、RSS page/content 与远程 source JSON
  预览/导入都必须经过同一个 N1 body/redirect/scheme/retry 边界。
- 失败仍使用各端点现有 status；只把不安全的底层诊断替换成稳定、安全的错误文本/代码。
- source headers/cookies 仍可到达初始/same-origin 目标；跨 origin redirect 的凭证剥离是明确安全差异。

## P2-N2：私网、DNS/dial 与代理边界

状态：**aligned / Docker-published / awaiting-device-verification**。

N2 在 N1 通过后独立实施。固定上游只支持书源内显式 HTTP/SOCKS 代理，没有进程环境代理、私网阻断或
部署白名单；因此 OpenReader 保留合法公网请求和显式代理能力，但把默认 SSRF 边界作为允许的安全差异。

### 管理员配置合同

唯一新增配置为：

```text
OPENREADER_SOURCE_NETWORK_ALLOWLIST=
```

- 空值是默认值，表示只允许公网目标和公网代理端点。
- 值为逗号分隔，最多 256 项、原始值最多 16 KiB；每项去除首尾空白，空项忽略。
- 每项只能是：精确 DNS hostname、裸 IP 或 CIDR。示例：
  `nas.home,192.168.1.20,192.168.1.0/24,fd00:1234::/48`。
- hostname 转为小写并去除末尾的点后精确匹配；允许常用单标签 LAN hostname 和 ASCII/punycode，
  不支持 `*`、后缀匹配、URL、端口、userinfo 或 Unicode hostname。IP/CIDR 使用规范化地址；
  IPv4-mapped IPv6 先还原为 IPv4。
- 精确 hostname 项明确授权该 hostname 当前解析出的全部 A/AAAA 地址；IP/CIDR 项只授权匹配地址，
  不隐式授权同名或同网域主机。redirect 到另一 hostname 后必须重新匹配。
- 非空非法项、超过条目/字节上限均使进程在监听端口和打开数据库前启动失败；错误只报告条目序号，
  不回显内部 hostname/IP。不能静默忽略拼写错误后以更宽松策略启动。
- 该变量只由部署管理员提供，不进入 REST、前端、SQLite、source/RSS JSON、WebDAV 或备份。历史源记录
  原样保留；未放行源只在真正请求时失败，不被删除、禁用或重写。

### 地址判定合同

默认拒绝以下地址；allowlist 可对明确的 hostname/IP/CIDR 做部署级例外：

- unspecified、loopback、RFC1918/ULA private、link-local、interface/link-local multicast、multicast；
- `0.0.0.0/8`、`100.64.0.0/10`、`192.0.0.0/24`、IPv4 TEST-NET、`198.18.0.0/15`、
  `224.0.0.0/4`、`240.0.0.0/4`；
- IPv6 discard/documentation/benchmark/ORCHID/6to4 和 NAT64 special-use 前缀，以及 IPv4-mapped IPv6；
- 因上述范围自然覆盖的 `169.254.169.254`、`fd00:ec2::254` 等云 metadata 地址。

IP literal 在发网前判定；hostname 每次解析必须得到至少一个地址。除非 exact-host 明确放行，否则一次
解析结果中任一地址被禁止且未被 IP/CIDR 放行时，整次请求失败，不能只挑其中公网地址继续。

### 初始请求、redirect 与实际拨号

1. 初始 URL 在 RoundTripper 前解析并检查；每次 redirect 先继续执行 N1 URL/header 合同，再按 N2
   重新解析和检查新目标。
2. direct transport 不使用解析后仍按 hostname 再拨号的窗口：实际 `DialContext` 再解析、复核全部
   答案，并且只向本次复核得到的已允许 IP literal 拨号。预检查为公网、拨号时变成私网的 DNS rebinding
   必须失败。
3. 已建立并复用的 keep-alive 连接只可能指向此前已验证 IP；策略重新配置时关闭 idle 连接。caller
   cancellation/deadline 在 DNS、拨号和代理握手中继续可由 `errors.Is` 识别。
4. 默认 shared source transport 忽略进程 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`，避免未进入书源
   合同的隐式远程 DNS/代理绕过；上游本来也只使用 source JSON 中的显式 `proxy`。FlClash/TUN 等系统
   路由不受影响。需要应用层代理时继续使用上游兼容的 source `proxy` 字段。

### 显式 HTTP/SOCKS 代理

- 保留现有 `http|socks4|socks5://host:port@username@password` 语义和合法端口；代理凭证永不进入
  policy、日志或公开错误。
- 代理 endpoint 与 target 是两个独立目标：两者在握手前都执行同一公网/allowlist 规则，endpoint
  在实际拨号时再次解析并只拨已验证 IP。
- HTTP proxy：OpenReader 在本地解析 target，将已验证 IP:port 写入 absolute-form/CONNECT 目标，
  同时保留原始 HTTP `Host` 与 HTTPS TLS SNI/证书 hostname 校验。proxy 不再替 OpenReader 远程解析
  未验证 hostname；redirect 后重新固定新 target。
- SOCKS4/SOCKS5：OpenReader 在本地解析 target，并只把已验证 IP literal 交给握手。SOCKS4 只选择
  IPv4；SOCKS5 支持 IPv4/IPv6。生产请求不使用 SOCKS4a/SOCKS5 hostname remote-DNS。
- 私网 proxy endpoint 也必须由管理员 allowlist；“用户在 source JSON 配了 proxy”不是访问容器
  loopback、bridge、host gateway 或 NAS 的授权。

### 测试 transport 合同

现有 Go fixture 通过 `SetHTTPClient` 注入不发真实 socket 的 RoundTripper。N2 将其明确重命名为
`SetHTTPClientForTesting`，并以测试 override frame 标记：该 frame 继续执行 N1 URL/body/redirect/retry
边界，但跳过 N2 DNS/dial policy；生产文件不得调用该入口，静态合同锁定所有调用都位于 `_test.go`。
N2 自身网络测试不得使用此 bypass，而使用 policy resolver/dial hooks 与生产 transport 验证。

### API 与错误合同

- 不增加或修改业务路由、成功 JSON、认证、缓存、WebSocket、source failure 或 retry 语义。
- 新增内部 sentinel `ErrUnsafeSourceNetwork`，同时仍属于 `ErrSourceRequest`；API 继续使用 N1 已固定的
  endpoint status、顶层 `error` 及可选 `source_request_failed`/`stage`，不得返回 DNS answer、目标、
  allowlist、代理 endpoint/credential 或底层拨号文本。
- allowlisted LAN source 的 GET/POST、relative URL、same-origin redirect、headers/body、charset/type、
  retry 与最终 URL 行为保持；只有访问权限从隐式开放改为管理员显式授权。

## 先写的失败测试

P2-N1：

1. URL：HTTP/HTTPS 成功；file/data/ftp、userinfo、空 host、坏 port 在 RoundTripper 前失败。
2. body：exact limit 成功；Content-Length 超限零读取；chunked `limit+1` 失败；超限内容不进入
   charset/HTML/XML/JSON/binary hex；所有 body 被关闭。
3. redirect：第 5 跳成功、第 6 跳失败；非法目标失败；same-origin headers 保留；cross-origin 的
   Cookie/Authorization/custom token/proxy auth 全部剥离，安全协商头保留。
4. retry/timeout：retry 大数被截为 3；非 2xx 最终 body 保持；limit/redirect/transport/cancel 不重试；
   15 秒默认和更早 caller deadline 生效。
5. 错误：API 层针对 search/RSS content/remote source import 的错误不含 secret query/header/body/proxy
   credential；source failure 仍只记录安全、短文本。
6. 回归 fixture：CSS/JSONPath/XPath 搜索、详情、目录、正文，RSS feed/rule/content，GBK/Big5、POST、
   binary type、HTTP/SOCKS proxy 的已有合同保持。

P2-N2：

1. allowlist parser：空值、trim/空项、exact host/IP/CIDR、大小写/末尾点、IPv4-mapped、非法 wildcard/
   URL/端口/Unicode、256 项与 16 KiB 上限；非法值不替换既有运行策略。
2. direct IPv4/IPv6、hostname→private、混合 DNS、DNS rebinding、redirect→metadata、默认/自定义端口；
   allowlisted exact host/IP/CIDR 成功，exact host 不授权 redirect 后另一 hostname。
3. 默认 transport 不读取进程代理；direct 的预检查与实际 dial 分别取 DNS，第二次变私网时零拨号。
4. HTTP proxy endpoint 与 target 分别校验；CONNECT/absolute-form 使用已验证 IP，但保留原 Host、TLS SNI
   和证书校验。SOCKS4/SOCKS5 endpoint 分别复核，握手 target 必须为本地解析后的 IP 而不是 hostname。
5. API search/RSS/source remote import 对私网失败保持既有 status/schema 和安全错误；测试 client bypass
   只允许 `_test.go`。
6. 真实 Docker 网络下验证公网 source、默认拒绝容器 loopback/bridge/host gateway、显式 allowlist 后
   LAN fixture 可用；移除 allowlist 并重启后再次拒绝，历史 SQLite/source JSON 不变化。

## 发布闸门

每一切片至少通过：

- focused engine/API contract；`go test -race` 覆盖 fetcher 包相关测试；
- `backend/go test ./...`、frontend 全量测试、production build、`git diff --check`；
- 真实公开/fixture CSS、JSONPath、XPath、RSS source 流；1440×900、390×844、360×800 的搜索到阅读与
  RSS 阅读 smoke，确认新错误不会清空已有缓存或触发错误登录失效；
- 本地 Docker fresh/historical volume、portable backup/restart 门；不使用云构建。

P2-N1 可在完成上述门后作为半模块镜像发布，但必须在进度报告中明确 `P2-N2 private-network policy`
仍未完成；只有 N2 也发布后，`docs/security-review-checklist.md` 的共享 source/RSS SSRF 项才能整体签收。

## P2-N1 实施与发布结果

- `backend/engine/fetcher.go` 已实现 15 秒默认总 timeout、16 MiB 单响应上限、5 次 redirect、最多
  3 次 retry、绝对 HTTP(S)/host/port/userinfo 校验、fragment 清除、所有 body 关闭及跨 origin
  credential/header 剥离；测试 client 覆盖栈和并发恢复不会泄漏旧 client。
- `backend/api/rss.go`、`backend/api/sources.go` 已把 RSS 和远程 source JSON 预览/导入的公开失败
  收敛为稳定无凭证文本；远程预览在读取 body 或发网前先完成编辑权限检查。
- 新环境变量只约束后续请求，不修改 SQLite、`data/`、`cache/`、`library/`、书源/RSS JSON、
  WebDAV 或备份格式。安全 TLS 保持为明确允许差异；N1 为兼容局域网书源，尚未改变 private target
  可达性。
- 验证：focused `go test -race ./engine`、全量 Go、frontend 706/706、production build；真实 API 的
  CSS/JSONPath/XPath 搜索→BookInfo→目录→正文、Source/RSS/Index 工作区 1440×900、390×844、
  360×800（RSS 另含 1024×1366）；fresh/historical volume、portable backup、restart 均通过。
- 实现提交 `981bca7` 已推送 `main`。OrbStack 本机完成 amd64/arm64 构建并直接发布
  `ghcr.io/changshengyu/openreader:981bca7` 与 `latest`，没有使用云构建。OCI index 为
  `sha256:02160e0797b3371fdfadccb550b8766d412c3e09df632ba1e36d192b26eb500d`；amd64 为
  `sha256:fd9f37f2d20c326c06cf0110fd8c6e529455523141e91383440bbb431dad06ce`，arm64 为
  `sha256:852f6ef18e7f7506c7b03ea377e35f1f3b12d0ef5cc7a6833fc871307e5a9719`。

## P2-N2 实施与发布结果

- 新增 `OPENREADER_SOURCE_NETWORK_ALLOWLIST` 的 fail-closed parser、启动时安装和 README 配置说明；
  非法配置在目录/SQLite/监听初始化前失败，错误不回显条目值。
- 默认 direct transport 关闭 ambient process proxy，初始/redirect RoundTrip 与实际 dial 分别解析并
  验证，mixed DNS 和 public→private rebinding 均在零私网拨号下失败；allowlisted exact host/IP/CIDR
  恢复明确的 LAN/NAS 访问。
- HTTP proxy endpoint 与 target 分开验证，absolute-form/CONNECT 使用本地固定 IP，原 Host、TLS SNI
  和证书验证保持；SOCKS4/5 endpoint 复核且握手只发送本地验证 IP。
- 测试 transport 已明确重命名为 `SetHTTPClientForTesting`；AST 合同证明生产文件无调用，既有 parser/API
  fixture 只跳过 N2 socket policy，仍通过 N1 request/response bounds。
- 失败测试提交 `4cb88ed` 在旧实现因配置/策略/transport 全部缺失而失败；实现后 focused engine/config/API、
  Engine race、全量 Go、frontend 706/706、production build 和 `git diff --check` 通过。受影响包
  `go vet . ./engine ./config` 通过；全仓 `go vet ./...` 仍有既存 `api/backup_restore_plan.go` 复制
  `Server` mutex 警告，不属于本切片且未据此冒充全仓 vet 通过。

- 本机候选镜像在默认严格策略下使用真实公网 IP source 成功发网并进入 JSON 格式错误；
  `host.docker.internal` 与 `127.0.0.1` 在 fixture 零请求的情况下被拒绝。exact-host allowlist 只恢复
  host-gateway fixture，删除 allowlist 并重启后重新拒绝，SQLite 中的历史 source 行始终保留。
- FlClash fake-IP 环境会把 `raw.githubusercontent.com` 解析到 `198.18.0.0/15`，首轮公网探针因此按
  合同被拒绝。正式门禁改用 `https://1.1.1.1/cdn-cgi/trace` 验证默认公网 IP 可达；这不是放宽策略。
  部署继续使用 fake-IP DNS 时，应优先使用 real-IP/Redir-Host，或由管理员明确将
  `198.18.0.0/15` 加入 allowlist。
- fresh volume 的 portable v1/v2 assets、cross-user、restart，以及 historical volume 的 TXT、EPUB、
  UMD、CBZ、relative-cache、owner-isolation、backup/restore 均通过。
- 实现提交 `d198c2e` 已推送 `main`。OrbStack 本机构建 amd64/arm64 并直接发布
  `ghcr.io/changshengyu/openreader:d198c2e` 与 `latest`，没有使用云构建。两个标签共同指向 OCI index
  `sha256:021817e602aa589c1583ec7ccb65828172c1a2afe1e038e23651dd51c455fcc1`；amd64 为
  `sha256:ef6bbd76f6b748b2597a57154fc54826c4afde3cc60615fbd63f867a7fa6217b`，arm64 为
  `sha256:d45132098b26dbf8a4ebbc9ca1f3e22b5fbb2ec48430e813a2c13f29204bee50`。
