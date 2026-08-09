# 书源 / RSS 共享远程抓取器 P2 合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-08-09 已完成固定上游、当前 Go 实现和调用面的源码盘点；尚未修改应用代码。
本项目分为两个可独立验证的安全切片：先完成不改变合法目标可达性的 `P2-N1` 通用请求边界，
再完成需要显式部署策略的 `P2-N2` 私网地址边界。任一切片均须先写失败合同测试。

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

## P2-N2：私网与 DNS/dial 边界

N2 在 N1 通过后独立实施，避免把网络兼容问题混入 body/redirect 修复：

1. 默认拒绝 loopback、private、link-local、multicast、unspecified、CGNAT、benchmark/documentation
   网段和云 metadata 目标；IPv4、IPv6、IPv4-mapped IPv6 一致处理。
2. 新增管理员环境 allowlist（准确名称和 grammar 在 N2 测试前锁定），支持 exact hostname/IP 和 CIDR。
   allowlist 是部署权限，不从用户可导入的 source/RSS JSON、header、URL option 或数据库字段读取。
3. 初始 URL、每次 redirect、实际 dial 和 source proxy endpoint 都复核；DNS 返回任一禁止地址时失败，
   direct transport 必须只 dial 已验证地址，防止 DNS rebinding。
4. allowlist 中的 NAS/局域网源保持相对 URL、same-origin redirect、headers、proxy 与 charset 行为；
   未 allowlist 的历史源数据仍原样保留，只在发请求时返回可操作的安全错误。
5. HTTP proxy 的 target resolution 与 SOCKS4/5 remote-DNS 行为必须在实现前写专项测试；不能以“已配置
   proxy”为由绕过目标地址 policy。

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

1. direct IPv4/IPv6、hostname→private、混合 DNS、DNS rebinding、redirect→metadata、默认端口与自定义
   端口；allowlisted exact host/IP/CIDR 成功。
2. HTTP/SOCKS4/SOCKS5 proxy endpoint 与 target 分别校验；proxy credential 不进入错误。
3. 真实 Docker 网络下验证公网 source、默认拒绝容器 loopback/bridge/host gateway、显式 allowlist 后
   局域网 fixture 可用。

## 发布闸门

每一切片至少通过：

- focused engine/API contract；`go test -race` 覆盖 fetcher 包相关测试；
- `backend/go test ./...`、frontend 全量测试、production build、`git diff --check`；
- 真实公开/fixture CSS、JSONPath、XPath、RSS source 流；1440×900、390×844、360×800 的搜索到阅读与
  RSS 阅读 smoke，确认新错误不会清空已有缓存或触发错误登录失效；
- 本地 Docker fresh/historical volume、portable backup/restart 门；不使用云构建。

P2-N1 可在完成上述门后作为半模块镜像发布，但必须在进度报告中明确 `P2-N2 private-network policy`
仍未完成；只有 N2 也发布后，`docs/security-review-checklist.md` 的共享 source/RSS SSRF 项才能整体签收。
