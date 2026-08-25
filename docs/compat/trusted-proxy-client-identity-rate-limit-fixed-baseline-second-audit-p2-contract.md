# 可信代理、客户端身份与限流固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前审查基线：`OpenReader@8b72b40`

审查日期：2026-08-25

## 1. 范围与结论

本轮在已签收的 HTTP 生命周期和访问日志 query 脱敏之后，继续复审进程级 middleware。下一项
`must-fix` 收敛为 Gin 的可信代理默认值、`ClientIP()` 投影与全局 API 限流身份：

- `backend/main.go` 使用 `gin.New()`，但没有调用 `SetTrustedProxies`；
- Gin 1.10 默认信任 `0.0.0.0/0` 和 `::/0`，并从 `X-Forwarded-For`、`X-Real-IP` 读取客户端地址；
- `backend/middleware/ratelimit.go` 直接以 `c.ClientIP()` 作为限流桶 key；
- `AccessLogger` 也把同一个 `ClientIP()` 写入运维日志。

因此，直接连接 OpenReader 的客户端可以自行指定转发头，绕过每 IP 限流并伪造访问日志中的来源地址。
该问题不改变 reader-dev 的阅读、书源、数据或前端状态机，但会使 OpenReader 自己增加的安全边界失效，
裁决为 **must-fix security / implementation-pending**。

## 2. 固定上游与当前证据

### reader-dev

- `src/main/java/com/htmake/reader/verticle/RestVerticle.kt` 直接创建 Vert.x HTTP server，没有按客户端
  地址执行应用层限流，也没有读取转发头来决定授权或业务身份。
- `doc.md` 的 Nginx 示例会设置 `X-Real-IP` 和 `X-Forwarded-For`。这证明反向代理部署是合法场景，
  但不构成“任意直连方都可信”的授权。
- 上游没有“客户端可通过请求头自行选择限流桶”的产品行为；OpenReader 的限流属于允许的安全增强。

### OpenReader

- `backend/go.mod` 固定 `github.com/gin-gonic/gin v1.10.0`。
- 该版本 `gin.New()` 默认 `ForwardedByClientIP=true`、`RemoteIPHeaders=[X-Forwarded-For,
  X-Real-IP]`，并将全部 IPv4/IPv6 地址设为可信代理。
- `OPENREADER_RATE_LIMIT_PER_MINUTE` 默认 6000；除 health、静态资源、WebSocket 等明确豁免路径外，
  API 和 WebDAV 请求都使用 `ClientIP()` 分桶。
- 当前配置没有可信代理字段，README 和 Compose 也没有可审计的代理信任入口。

### 真实运行时反例

在当前二进制将 `OPENREADER_RATE_LIMIT_PER_MINUTE=1` 后，对同一个 `/api/me` 依次请求：

1. `X-Forwarded-For: 198.51.100.10` 返回 401；
2. 同一 header 再请求返回 429；
3. 只改为 `X-Forwarded-For: 203.0.113.11` 又返回 401。

三次 TCP 请求都来自 `127.0.0.1`，日志却分别记录两个伪造地址。该反例同时证明限流绕过和日志身份
污染，不以源码推断代替运行时证据。

## 3. 最终配置合同

新增可选环境变量 `OPENREADER_TRUSTED_PROXIES`：

- 默认空值：不信任任何代理，`ClientIP()` 只使用 TCP peer 地址并忽略所有 forwarded client-IP header；
- 非空值：逗号分隔的 IPv4、IPv6、IPv4 CIDR 或 IPv6 CIDR，仅这些直接/链式代理可提供转发地址；
- 每项必须 trim，空项、非法 IP/CIDR 或无法解析的值使进程在 listen 前启动失败；
- 不提供 `*`、隐式“信任全部”或静默跳过坏项；管理员若明确配置 `0.0.0.0/0` 或 `::/0`，属于其
  显式选择，但文档必须标注风险；
- 该变量只决定客户端 IP 投影，不改变 source proxy、CORS、监听地址、WebDAV 路径或登录 JWT。

README 的常用部署段不增加反向代理前置要求；只在高级变量表中用普通语言说明：直连部署保持空值，
确实位于反向代理后时填写代理自身 IP/CIDR，而不是访问者网段。`docker-compose.yml` 以注释形式提供
可选变量，不强制普通部署者配置。

## 4. 运行时合同

### 默认直连

1. `X-Forwarded-For`、`X-Real-IP` 和组合/重复值不得改变限流桶。
2. 同一 TCP peer 变换 header 后仍共享同一计数；达到限额继续返回现有 429 JSON：
   `{"error":{"code":"RATE_LIMITED","message":"too many requests, try again later"}}`。
3. Access log 的 client IP 必须使用 peer 地址；已发布的 path/query/capability 脱敏格式保持。

### 显式可信代理

1. 只有请求的直接 peer 位于可信集合时才读取 forwarded header。
2. `X-Forwarded-For` 链按 Gin 的右到左可信代理算法，选择第一个不可信 hop；不能直接取最左或最右
   字符串，也不能接受非法 IP。
3. 经同一可信代理到达的不同真实客户端使用不同限流桶；同一客户端的重复请求仍共享桶。
4. `X-Real-IP` 作为既有 Gin 后备 header 保留，但只有可信 peer 可使其生效。
5. 限流和 access log 必须消费同一个经验证的 `ClientIP()`，避免两个身份口径分叉。

### 生命周期与错误优先级

- 可信代理配置必须在注册 middleware、启动 scheduler/backup 的外部可见工作和 `net.Listen` 之前完成；
  非法配置不得短暂监听端口。
- 无效配置错误可以写入启动日志，但不得包含请求数据、JWT 或其它 secret。
- health、静态资源、公开 capability、uploads 和 WebSocket 的既有限流豁免不在本轮改变。
- 既有 graceful shutdown、512 KiB header、10 秒 header deadline、8 秒 drain 与 access-log query
  固定 marker 保持关闭，不因本轮重写。

## 5. 数据与兼容合同

- 不新增或修改 SQLite 表、索引、备份字段、WebDAV 文件、cache 或 library 路径。
- 不改变 API 路由、请求/响应 body、认证、状态码或前端调用顺序；唯一可见差异是伪造转发头不能再
  绕过限流或污染日志。
- 默认值必须适合 README 的直连 Compose/`docker run` 部署；升级旧卷不需要迁移或人工配置。
- 合法反向代理部署通过显式 CIDR 恢复真实客户端分桶，不把所有访问者错误合并为代理单一桶。

## 6. 测试先行门

实现前必须先在旧代码上建立失败证据：

1. config 测试：默认空值、trim 后的逗号列表、IPv4/IPv6/IP/CIDR 和非法/空项。
2. router/middleware 测试：默认模式下变换 `X-Forwarded-For`/`X-Real-IP` 仍命中同一 429 桶，日志
   只记录 peer 地址。
3. trusted proxy 测试：可信 peer 的两个 forwarded client 分桶；同一 client 重复请求为 429；非可信
   peer 的 header 被忽略；多 hop 链选择第一个不可信地址。
4. startup 测试：非法配置在 listen 前失败，scheduler/backup 等后台工作不得启动或遗留。
5. focused/race、Go 全量和 `go vet`；frontend 全量与 production build 用于发布门，虽无前端改动仍需
   防止整仓回归。
6. 真实二进制以 limit=1 验证直连 header 不能绕过、显式 loopback trusted 后可以区分两个客户端、
   429 envelope 不变、日志身份与 query 脱敏同时正确。
7. fresh/historical/portable/restart 卷门通过后才可发布 Docker；本项不需要新增浏览器交互 smoke。

## 7. 当前状态

合同先以 `30b7630` 独立提交。旧实现红测 `db89593` 随后证明配置模型和启动层没有可信代理入口，并
锁定默认直连、显式可信代理、多 hop、非可信 peer、429 JSON 和 access-log 身份合同。

当前实现：

- `Config` 新增原样读取的 `OPENREADER_TRUSTED_PROXIES`，默认空值；
- `run()` 在目录、SQLite、scheduler、backup、middleware 和 listen 之前调用
  `configureTrustedProxies()`；
- 空值执行 `SetTrustedProxies(nil)`；非空值严格 trim 逗号列表并交给 Gin 验证 IP/CIDR，空项或非法项
  直接使启动失败；
- limiter 和 AccessLogger 继续共用 Gin `ClientIP()`，没有新增第二套代理链解析；
- Compose 提供带人类说明的空默认值，中英文 README 高级配置表均解释直连与反向代理填写方式。

回归证据：focused 测试、focused race、Go 全量、`go vet`、frontend 741/741、production build、
`docker compose config --quiet` 和 README 42/42 配置变量检查通过。真实二进制在 limit=1 时：默认直连
变换两个伪造 header 得到 `401 -> 429`，日志只记录 `127.0.0.1` 且 query 为固定 `<redacted>`；显式信任
loopback 后两个 forwarded client 得到 `401(A) -> 401(B) -> 429(A)`；含空项的配置在后台服务和 listen
前退出。

可信 GitHub Actions 运行 `32828470325` 随后重新通过 backend、frontend 741/741、production build、
Compose、原生验证镜像、fresh portable-v1/v2-assets/cross-user/restart 和 historical
TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation 卷门，并发布：

- `ghcr.io/changshengyu/openreader:f5b3869`
- `ghcr.io/changshengyu/openreader:latest`
- OCI index：`sha256:6a2fc83bf79426e93423b1dd5756c8ea49b716d1321441d5c194efff9c03b066`
- amd64 manifest：`sha256:0c8dcb1374ab39bab0fe5aeeba229c3a36e325d96a06e6dc9e5bedd26de37fb3`
- arm64 manifest：`sha256:5960e765a6f77f4a7453fb66f526758fa3acc4e70f83e237c219e441dac27cfe`

远端 commit tag 与 `latest` 已回读为同一索引。不涉及前端交互，因此没有新增浏览器 smoke。当前状态：
**aligned / regression-validated / Docker-published / awaiting-device-verification**；用户生产环境运行提交
仍未知。
