# 发布门新增标准库公告：Go 1.26.11（GO-2026-6859，`net/http` MaxBytesReader 响应头竞争，P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本切片不涉及上游可见行为）。

审查日期：2026-11-12。实施基线：`origin/main` = `e8f3214`；分支 `claude/go-toolchain-1.26.11` 用 `git switch --no-track -c` 从 `origin/main` 切出。
状态：**implementation-complete / regression-validated**（合同 `a2b4ff6`、红证据 `56c07dd`、构建 `fcf62a3`、README `461a309`；证据见第 8、9 节）。
未合入 main，镜像未发布，生产未访问。

前置合同：[`go-advisory-release-gate-contract.md`](go-advisory-release-gate-contract.md)（门的判定边界）、
[`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)
第 4.1 节（GO-2026-6809：Go 1.26.9 → 1.26.10 的同类先例，合同 `37503a3`、构建 `6ca7fbf`）、
[`dependency-advisory-x-crypto-openpgp-fix-p1-contract.md`](dependency-advisory-x-crypto-openpgp-fix-p1-contract.md)
（D2：门不再有不适用项，任何 finding 都阻止发布；x/crypto 固定伪版本及其 2026-12-16 期限）。这些合同的已签收结论不重开。

## 1. 范围与结论

2026-11-12 在 `e8f3214` 上于沙箱外运行 `node scripts/check-go-advisories.mjs`（沙箱内 govulncheck 下载会因 TLS `OSStatus -26276`
失败，不能当作门结果），门失败关闭：`advisory findings block release: GO-2026-6859`，47 个发现，其中 45 个符号级。CI 在构建镜像前
运行同一门（`docker-publish.yml` 的 Go advisory 步骤，在 setup-go 读取 `go-version-file: backend/go.mod` 之后），所以此后合入 main 的
任何提交都会在 Go 门失败、不产生镜像；等待发布的 `claude/rss-user-url-index` 与叠加其上的 `claude/rss-restore-custom-order`
（都不改 Go 版本、`Dockerfile` 或门脚本）同样被拦住。

裁决：**must-fix（发布阻塞）**。按第 4 节做最小的工具链补丁升级，独立成切片；精确的红证据见第 8 节。

## 2. 公告与适用性

| 项 | 内容 |
| --- | --- |
| 公告 | [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859)（CVE-2026-97054），2026-11-11T02:51:11Z 公开，`review_status: REVIEWED` |
| 问题 | HTTP/1 服务端：`MaxBytesReader` 超限时，读取请求体的一方不加同步地自动设置 `Connection: close` 响应头，可能与 handler 显式写响应头竞争；handler 并发地读请求体与写响应头时，服务进程可因 runtime panic 崩溃 |
| 范围 / 修复 | 标准库 `net/http`：`< 1.26.11`、`1.27.0-0 … < 1.27.4`；修复版本 Go 1.26.11 / 1.27.4（[CL 862185](https://go.dev/cl/862185)、[issue 82563](https://go.dev/issue/82563)、golang-announce） |
| 当前 | `backend/go.mod` 为 `go 1.26.10`；后端构建镜像为 `golang:1.26.10-alpine3.23` |

**govulncheck 判定可达**（v1.9.0，source / symbol 级）：OSV 列出的 `net/http` 符号覆盖服务端与客户端的大部分入口，OpenReader 的
HTTP/1 服务（`Server.Serve`/`Close`/`Shutdown`）、书源与封面等出站抓取（`Transport.RoundTrip`、`Client.Do`）、备份与 WebSocket 同步都在
发现轨迹里。

**代码检查（只说明风险，不作为放行理由）**：OpenReader 用 `http.Server` 直接提供明文 HTTP/1.1（没有 TLS/HTTP/2 服务端），`backend/api`
中 9 个文件共 10 处用 `http.MaxBytesReader(c.Writer, …)` 限制请求体（`request_body.go` 的两个 `decodeBounded*` 助手，以及
`source_debug.go`、`remote_reader.go`、`book_source_import_multipart.go`、`direct_local_import_boundary.go`、`localstore_boundary.go`、
`backup_restore_multipart.go`、`book_control_request_boundary.go`、`uploads.go`）。在当前代码里崩溃前提看起来不成立：

1. 传给 `MaxBytesReader` 的是 gin v1.10.0 的 `*responseWriter`，它只嵌入 `http.ResponseWriter` 接口，标准库未导出的
   `requestTooLarge()` 不会被提升；`maxBytesReader.Read` 对它的类型断言不成立，读取一方不会去写 `Connection` 头。
2. 这些 handler 都在 handler goroutine 上先读完请求体、再写响应头；`api` 包里另起的 goroutine 只做书源搜索与批量测试，不读请求体。

这只是对当前代码的检查，不是证明：新增直接使用底层 writer 的 handler、gin 改为透传底层 writer、或改用 `MaxBytesHandler`，都可能
满足前提；门也没有例外机制（D2）。修复只存在于标准库补丁版本，所以按 must-fix 升级，不申请例外。

## 3. 差异分类

| 编号 | 内容 | 分类 |
| --- | --- | --- |
| G1 | 可达的标准库安全修复，门失败关闭，无可签收的例外理由 | **must-fix** |
| G2 | 官方 `golang:1.26.11-alpine3.23` 的 base 从 Alpine 3.23.7 变为 3.23.8（只影响构建阶段，见 4.2） | **acceptable-change** |
| G3 | README 的源码构建要求 “Go 1.26.10 or newer” 随 `go` 指令同步为 1.26.11（见 4.3） | **acceptable-change** |

不得通过给门加例外、放宽精度或跳过符号级发现来过门。

## 4. 目标与不变项

### 4.1 Go 指令（G1）

`backend/go.mod` 的 `go` 指令 `1.26.10` → `1.26.11`，不加 `toolchain` 行；模块依赖与 `go.sum` 不变（`go mod verify` 通过、`go mod tidy`
不改文件）。CI 的 `setup-go` 读取 `go-version-file: backend/go.mod`，随之使用 1.26.11；本地 `GOTOOLCHAIN=auto` 时 go 命令自动下载并
经 sum.golang.org 校验 `golang.org/toolchain@v0.0.1-go1.26.11.<os>-<arch>`。

### 4.2 构建镜像（G1、G2）

`Dockerfile` 后端构建阶段改为官方 `golang:1.26.11-alpine3.23` 的 OCI index：

| 项 | 值 |
| --- | --- |
| index（`docker-content-digest` 与响应体 sha256 一致） | `sha256:db99415e2625ce0ee03eed09edf7f32ae8c58dc399f28cc602a87fe7238119e4` |
| `linux/amd64` | manifest `sha256:6fa4e6dc2fc42920e53eb6b6a7e928155dc68282060f0901ec3cdd134c3a0eb5`，config `sha256:7112a03fdc0b65a29119dce314aea166996eda5719399bf90b9cab54f369e8a6`，2026-11-10T18:21:25Z |
| `linux/arm64/v8` | manifest `sha256:8961afa74714275cd683db74552a3dd9d1a1e542491b03c088822d30b4ca2c36`，config `sha256:ea94fc08427eecf1f1a583b5a3fae6bc4c6c3100acffe8139a3435ac932d343a`，2026-11-10T18:20:18Z |
| 来源 | `docker-library/golang@f66ba4e64dfbab452b77f8ae045428aacadc24a2`（`1.26/alpine3.23`），`org.opencontainers.image.base.name` = `alpine:3.23`（minirootfs 3.23.8） |
| 两平台共同 | `GOLANG_VERSION=1.26.11`、`GOTOOLCHAIN=local`；Go 层内 `/usr/local/go/VERSION` 为 `go1.26.11`（time 2026-11-02T20:36:21Z）；APK 库中 `musl` 1.2.5-r24 的 `C:` 与运行时构建期 pin 相同（x86_64 `Q1XUI1pl…`、aarch64 `Q1Gpmv84…`），`ca-certificates-bundle` 20261020-r0 |

旧 pin（`sha256:107301490755…`，2026-10-21，`docker-library/golang@650cf2f`）是 Go 1.26.10、Alpine 3.23.7。构建阶段的 base 点版本
变化（G2）不进入运行时镜像：运行时阶段不再复制构建镜像的任何系统文件，只复制 `/out/openreader` 二进制；`apk add --no-cache build-base`
照旧从同一 v3.23 仓库安装编译工具。镜像的 manifest、config、两个小层与 Go 层都按 digest 下载并校验 sha256。

### 4.3 README（G3）

`README.md` 与 `README_CN.md` 的源码构建要求从 “Go 1.26.10 or newer / 1.26.10 或更新版本” 改为 1.26.11，在构建提交之后单独提交
`docs:`。理由：`GOTOOLCHAIN=local` 的 Go 1.26.10 会拒绝构建（`go.mod requires go >= 1.26.11`）；默认的 `GOTOOLCHAIN=auto` 会自动下载
1.26.11，但 README 写的最低版本不应低于 `go.mod`。

### 4.4 不变项

- 运行时 `alpine:3.23.6@sha256:85fe1e81…` 与构建期精确安装的 `zlib=1.4-r0`、`musl=1.2.5-r24`、`musl-utils=1.2.5-r24`、
  `ca-certificates-bundle=20261020-r0`；OS APK 基线、`reviewUntil`（2026-12-10）与 OS 门测试（只检查运行时阶段）。
- `node:22-alpine3.23@sha256:baf676f7…` 前端构建阶段；CGO、`go mod vendor -o`、`-mod=vendor -trimpath` 的构建方式；单二进制、挂载目录与环境变量。
- 全部 Go 模块与 `go.sum`，包括 x/crypto 固定伪版本 `v0.58.1-0.20261009181718-2f1f834330b2` 及门里的 2026-12-16 期限。
- 门脚本 `scripts/check-*-advisories.mjs`、它们的自测与 workflow 接线；不新增例外。
- API、数据、schema、前端源码与界面行为。

## 5. 红（既有发布门即红测）

与 GO-2026-6809 先例相同，本切片不新增测试代码：已提交、在 CI 中强制执行的失败关闭门在 Go 1.26.10 上的失败就是红（第 8 节记录
精确输出），门的精度与失败处理已由 `scripts/check-*-advisories.test.mjs` 覆盖。公告里的竞争是非确定性的 map 并发写，而且第 2 节的
检查表明 OpenReader 的调用点不满足前提，所以不编写会随机崩溃的复现测试；第 8 节另用草稿实验确认 gin 调用点与原生 `net/http` 的差别。

## 6. 验证门（实现后）

1. backend 内 `go version` 为 `go1.26.11`；`go -C backend test ./...`、`go -C backend vet ./...`、
   `go -C backend test -race ./db ./services/...`、`go -C backend test -race -timeout 40m ./api` 全部通过（绑定端口的测试在沙箱外运行）。
2. 现场 Go 门在沙箱外 exit 0：`pass`，`goVersion` 为 go1.26.11，**0 个发现**；三张依赖图仍无 openpgp 族，x/crypto 仍为固定伪版本。
3. `node --test scripts/check-*-advisories.test.mjs` 通过（门脚本未改，计数应与 main 相同）；现场前端门 exit 0。
4. `cd frontend && npm test && npm run build`（沙箱外）通过；`go mod verify` 通过、`go mod tidy` 不改文件。
5. 两个 Linux 目标在 1.26.11 下通过 cmd/go 对 `#cgo` 指令的安全检查（1.26.11 收紧了 cgo 编译/链接参数白名单，见第 8 节）。
6. 推送分支并记录；合入 main 与发布由用户决定，发布后由可信 Actions 的 Go 门、OS APK 门、双平台构建与卷门复核；本地不构建镜像。

## 7. 非目标

- 不升级到 Go 1.27.x，不升级任何 Go 模块或前端依赖，不改门脚本、门测试与 workflow。
- 不改应用代码（例如把 `MaxBytesReader` 换成别的限制方式）；第 2 节的检查不构成放行理由，也不需要代码改动。
- 不推送 main、不触发发布、不访问生产。

## 8. 红证据（2026-11-12，Go 1.26.10）

### 8.1 现场门

在 `e8f3214`（backend 树与合同提交 `a2b4ff6` 相同）的 `backend/` 内 `go version` 为 go1.26.10；2026-11-12T05:00:34Z 于沙箱外运行
`node scripts/check-go-advisories.mjs`，05:00:43Z **exit 1**：

```text
Go advisory evidence: /var/folders/5z/h__cbxf93c5gc1p4myzqpd480000gn/T/openreader-go-advisories-mbPSza
Go advisory gate failed: advisory findings block release: GO-2026-6859

47 !== 0
```

证据目录由门自己写入：`failure.txt`、`environment.json`、三张依赖图与原始 `scanner.json`（927,661 字节）。失败来自 finding 断言；
在它之前的检查都已通过：协议 `v1.0.0`、govulncheck `v1.9.0`、source / symbol、DB `https://vuln.go.dev` 时点 `2026-11-11T02:51:11Z`、
SBOM 与实际 Go 均为 go1.26.10（darwin/arm64，CGO 1，33 个模块）、x/crypto 为固定伪版本 `v0.58.1-0.20261009181718-2f1f834330b2`（期限内）、
三张依赖图 host 360 / linux-amd64 359 / linux-arm64 358 个包，都有 main 与 bcrypt、都没有 openpgp 族。

| 项 | 结果 |
| --- | --- |
| OSV | 250 条唯一（275 条消息，重复的完全相同） |
| 发现 | 47 条，全部是 `GO-2026-6859`、`stdlib@v1.26.10`、`fixed_version v1.26.11`：模块级 1、包级 1（`net/http`）、符号级 45 |
| 1.26.11 修复的公告 | 250 条中 stdlib/toolchain 范围在 1.26.11 修复的只有 `GO-2026-6859`，所以升级后应为 0 个发现 |
| 符号 | 45 个，全在 `net/http`：服务端（`(*Server).Serve`/`Close`/`Shutdown`、`(*response).Write`/`WriteHeader`/`Flush`、`(*connReader).Read`、`(*maxBytesReader).Read`/`Close` 等）与客户端（`(*Client).Do`、`(*Transport).RoundTrip`/`Clone`/`CloseIdleConnections`、`(*persistConn).Read` 等），以及 `Header.Get`/`Set`/`Del`、`CanonicalHeaderKey`、`NewRequestWithContext` |
| 入口 | 16 个：`openreader/backend.main`；`api.storeReadResponseWriter.Write`；`engine` 的 `DecodeBody`、`FetchSourceTextWithURLContext`、`ConfigureSourceNetworkPolicy`、`(*sourceHTTPProxyRoundTripper).RoundTrip`/`CloseIdleConnections`；`services/backup` 的 `(*Service).RunPortableV2ForUserContext`、`contextReader.Read`；`services/instancesecret.Apply`；`services/scheduler.(*Scheduler).Stop`；`services/sourcecompat.WriteSnapshotFile`；`services/sourcedebug` 的 `(*StageError).Error`、`(*boundedEmitter).Emit`；`sync.(*Client)` 的 `ReadPump`/`WritePump` |

轨迹示例：`main → run → serveHTTPServer → (*Server).Serve`（`http_server.go:40`）；`api.storeReadResponseWriter.Write → gin (*responseWriter).Write
→ WriteHeaderNow → (*response).WriteHeader`；`engine.FetchSourceTextWithURLContext → fetchTextRequestWithURLContext → (*Client).Do`；
`backup.contextReader.Read → io.(*LimitedReader).Read → (*persistConn).Read`。唯一到达 `(*maxBytesReader).Read` 的轨迹是
`engine.DecodeBody → io.ReadAll`（`fetcher.go:781`），即出站响应体经 `io.Reader` 接口的保守连接，不是服务端 handler。

### 8.2 标准库差异（1.26.10 → 1.26.11）

比较模块缓存中经 sum.golang.org 校验的 `golang.org/toolchain@v0.0.1-go1.26.10.darwin-arm64` 与 `…go1.26.11.darwin-arm64`
（ziphash `h1:cGxzEKi4mhhQFd6Cz/2NSL4Y9nfIBCirzU1yn2rnOY8=`，`VERSION` time 2026-11-02T20:36:21Z）的 `src/`：

- **修复本体**（`net/http/server.go`、`transfer.go`，新增 `http1_server_test.go`）：`(*response).requestTooLarge()` 从“设置
  `closeAfterReply`、`requestBodyLimitHit`，并在未写头时 `w.Header().Set("Connection", "close")`”改为只做
  `requestBodyLimitHit.Store(true)`（`atomic.Bool`）；写响应头（`(*chunkWriter).writeHeader`）与 `shouldReuseConnection` 改为读取该标志
  来决定关闭连接。`response.reqBody` 改为保存原始 `*body`（`expectContinueReader` 单独记录），丢弃剩余请求体与提前关闭的判断不再
  依赖 handler 替换后的 `req.Body` 类型；`*body` 的方法对 nil 安全。
- **同一补丁版本的其它变化**（DB 中没有对应公告）：`net/dial.go`（`DialTCP`/`DialUDP`/`DialIP` 对无效 netip 地址传 nil）、
  `crypto/internal/fips140/*` 与 `crypto/rsa/fips.go`（FIPS-only 模式也检查 MGF 哈希）、`runtime` 的 32 位 Linux 定义，以及
  `cmd/{cgo,compile,dist,link}` 与 `cmd/go/internal/work/security.go`：`#cgo` 编译/链接参数白名单收紧（`-x` 只允许语言名、`-l` 不得含
  路径字符、新增 `-gen-*`/`-gnat*` 等禁用项、对 `-L`、`-isysroot`、`-framework`、`-Wl,` 与直接链接输入做路径/文件检查）。
  go-sqlite3 v1.14.36 在 Linux 默认构建中使用的 `-lm`、`-ldl`、`-I.`、`-std=gnu99`、`-D…`、`-W…` 都在新规则内；第 6 节第 5 项实测两个 Linux 目标。

### 8.3 草稿实验（只在 scratchpad，不入库）

用 backend 的 `go.mod`/`go.sum`（gin v1.10.0）建一个临时模块，真实 `http.Server` 跑在 `net.Pipe` 监听器上（不绑定端口），限额 10 字节、
请求体 64 字节（剩余量远小于 256 KiB），分别用 `GOTOOLCHAIN=go1.26.10` 与 `go1.26.11` 运行：

| | 原生 `net/http` handler（`MaxBytesReader(w, …)`） | gin handler（`MaxBytesReader(c.Writer, …)`，OpenReader 的写法） |
| --- | --- | --- |
| go1.26.10 | 读到 `*http.MaxBytesError`；读取一方把 `Connection: close` 写进了 handler 的响应头 map（公告所说的无同步写）；响应关闭连接 | 读到 `*http.MaxBytesError`；响应头 map 没被改动；剩余请求体被丢弃，连接保持 |
| go1.26.11 | 读到 `*http.MaxBytesError`；响应头 map 没被改动；写响应头时读取标志，响应仍关闭连接 | 与 go1.26.10 相同 |

结论：1.26.10 的无同步写只在原生 writer 上发生；OpenReader 的 10 处 gin 调用点不会执行它（第 2 节第 1 条），1.26.11 下这些调用点的
413 行为与连接处理不变。实验不替代门，也不作为例外理由。

状态（2026-11-12，本节）：**behavioral-reds-pending**——红就是现场门在 Go 1.26.10 上的失败（门脚本未改），等待第 4 节的实现。

## 9. 实施与验证证据（2026-11-12）

### 9.1 提交

| 提交 | 内容 |
| --- | --- |
| `a2b4ff6` | 本合同第 1–7 节；gap-analysis、matrix 条目与运行时依赖合同的指针 |
| `56c07dd` | 第 8 节：1.26.10 上现场门的红、1.26.10 → 1.26.11 标准库差异与草稿实验 |
| `fcf62a3` | `go 1.26.11`；后端构建镜像改为官方 `golang:1.26.11-alpine3.23` index pin（`go.mod`、`Dockerfile` 各 1 行） |
| `461a309` | README / README_CN 的源码构建最低版本同步为 1.26.11（第 4.3 节） |

`go mod verify` 通过（all modules verified），离线（`GOPROXY=off`）`go mod tidy` 不改 `go.mod`/`go.sum`。

### 9.2 发布门（失败 → 通过）

| 门 | 升级前（`e8f3214`，go1.26.10） | 升级后（`461a309`，go1.26.11） |
| --- | --- | --- |
| Go advisory 现场扫描（沙箱外） | exit 1：`GO-2026-6859`，47 个发现（第 8 节） | exit 0：`pass`，go1.26.11 / darwin-arm64 / CGO 1，DB `2026-11-11T02:51:11Z`，250 条公告，**0 个发现**；x/crypto 仍为固定伪版本，`pinReviewUntil` `2026-12-16T00:00:00.000Z`；三张依赖图 360/359/358 与红运行逐字节相同，无 openpgp 族。证据目录 `/var/folders/5z/h__cbxf93c5gc1p4myzqpd480000gn/T/openreader-go-advisories-4wYeSE`（2026-11-12T08:57:58Z–08:58:09Z） |
| 前端全锁 advisory 现场扫描（沙箱外） | 本切片未改前端 | exit 0：`pass`，132 个锁包，0 个发现，lockHash `608a6a9ea769adebf6a3b9c99edf610810fa63c9fbc406294eba1d58bc6f24a8` |
| 门自测 `node --test scripts/check-*-advisories.test.mjs`（沙箱外） | — | 139/139（Go 65、前端 49、OS 25），0 失败 0 跳过；门脚本与测试未改 |

### 9.3 后端

- `backend/` 内 `go version` 为 go1.26.11（`GOTOOLCHAIN=auto` 经 proxy / sum.golang.org 下载的 `golang.org/toolchain@v0.0.1-go1.26.11.darwin-arm64`）。
- `go -C backend vet ./...` exit 0，无输出。
- `go -C backend test -count=1 ./...`（沙箱外）：31 个包通过、7 个无测试文件、0 失败，api 127.9 s。
- `go -C backend test -race -count=1 ./db ./services/...`（沙箱外）：25 个包通过、4 个无测试文件，无 DATA RACE。
- `go -C backend test -race -timeout 40m -count=1 ./api`（沙箱外，08:53:33Z–09:13:32Z）：通过，api 1173.5 s（与其它门同时运行，比通常的约
  1050 s 稍长），无 DATA RACE。
- `go build -trimpath` 的产物记录 go1.26.11、`CGO_ENABLED=1`，依赖版本与 h1 不变（例如 x/crypto 固定伪版本 `h1:zGDaB+/Mus2mScKvZ23k+W9NAUbP56sNM9SptgvWI5A=`）。
- 第 6 节第 5 项：go1.26.11 下 `GOOS=linux GOARCH=amd64|arm64 CGO_ENABLED=1 go -C backend build -n -trimpath` 都 exit 0、0 条 `invalid flag`；
  cgo 包为 `runtime/cgo`、`net`、`github.com/mattn/go-sqlite3`（LDFLAGS 只有 `-ldl`）。cmd/go 在 `runCgo` 里先用 `b.CFlags` 检查 `#cgo` 指令、
  再调用 C 编译器，干跑也执行这一步；对照包 `#cgo LDFLAGS: -l../x` 在 1.26.10 下通过、在 1.26.11 下两个目标都报
  `invalid flag in #cgo LDFLAGS: -l../x`，说明干跑确实执行了新规则。真正的 musl 构建在可信 Actions 的双平台镜像构建里完成。
- `node scripts/smoke/store-read-list-lifecycle-contract.mjs`（沙箱外；脚本在 `backend/` 内 `go build`，即 go1.26.11，再启动真实 HTTP/1 服务）：
  10 项 PASS，包括 LocalStore 上传与 WebDAV 的 HTTP 路径；`docker compose config --quiet` 通过。

### 9.4 前端

本 worktree 原有的 `frontend/node_modules`（10-22）与 11-10 的锁文件不一致，先按锁文件 `npm ci`：已安装的 102 个包版本与锁文件逐一相同，
未安装的 30 个都是其它平台的可选包。Node 26.4.0 / npm 11.17.0：`npm test` 877/877（0 失败 0 跳过）、`npm run build` 通过。前端源码与锁文件未改。

### 9.5 未跑及原因

- 三视口浏览器 smoke：前端源码、锁文件与构建配置都未改，产物与 main 相同；本切片只改 Go 工具链。
- 镜像构建、OS APK 门、fresh / historical 卷与 portable 备份门、双平台候选：只在可信 Actions 上运行（本地不构建镜像）。合入 main 后由
  Actions 用 `GOTOOLCHAIN=local` 的 go1.26.11 构建镜像以 `-mod=vendor` 编译两平台，并复核 OS 门（运行时镜像不变）。
- 沙箱内会失败的项（绑定端口的测试、govulncheck 与 toolchain 下载、npm 注册表）都在沙箱外跑过，不是跳过。

### 9.6 状态

**implementation-complete / regression-validated**。Git：分支 `claude/go-toolchain-1.26.11` 已推送（`git ls-remote` 核验），未合入 main。
镜像：未发布，GHCR `latest` 仍是 `2de7308`（index `sha256:49b02ce1…ae75`）。生产：未访问。合入 main 与发布由用户决定，本会话不推送 main。

### 9.7 发布（2026-11-12）

按用户选择，与 Go 1.26.11（GO-2026-6859）、注册邀请码与用户数上限、RSS 两项索引一起发布：合并提交 `516a4f0`（注册）、`08b3e09`（RSS）、`6c4f004` 与 `28813c2`（Go），main 由 `e8f3214` 快进到 `28813c2`（ls-remote 核验，未强推）。

推送前在合并树上（go1.26.11）：后端 `go test ./...` 31 个包与 vet，race api/db/config/services 27 个包（api 1150.3 秒），无数据竞争；前端 882/882 与 build；公告门自测 139/139；现场 Go 与前端扫描 0 个发现；compose、存储 HTTP 合同；注册限制合同与 openreader-smoke 三视口、重新认证合同四视口通过；secdb main 11-11 22:16:35 下运行时 origin 与夹具相同，5 个未列 origin 仍无记录。

可信 Actions run `43191938289` 43/43 步成功（09:29–09:46 UTC）：两平台 OS 门 `pass-with-unknown-origins`（16/16 包、110/109 个 APK 文件、74 组比较无待修复项，复审期限 2026-12-10），Go 公告门 go1.26.11 0 个发现，卷门与平台核验通过。

镜像：`ghcr.io/changshengyu/openreader:28813c2` 与 `latest` 指向 OCI index `sha256:83d9918ca08e764a574d1d9b64707ded70d7b1949decf4ae4457b8d05f9cddb0`；linux/amd64 `sha256:a6995415214474f72b976bae134fd6043e97d054c664c816e49811c8942edf44`，linux/arm64 `sha256:20bcf60fbff86eeca2b062f2975e03965453011506d44e6773654d6e6d2494bd`，另有 attestation。两平台 revision 标签均为 `28813c259f93955e67ad53a9b97fb326a3bc982e`，两平台镜像内 `/app/openreader` 均为 go1.26.11 构建。

状态 **released / Docker-published / awaiting-device-verification**；生产未访问，实际运行版本未知。
