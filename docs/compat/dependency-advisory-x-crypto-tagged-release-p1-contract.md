# 发布门依赖：x/crypto 由 GO-2026-5932 修复伪版本换成标签版本 v0.59.0（P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本切片不涉及上游可见行为）。

审查日期：2026-11-14。实施基线：`origin/main` = `0584ae78`；分支 `claude/go-advisory-x-crypto-tagged-release` 从 `origin/main` 切出。
状态：**contract-only**。

前置合同：[`dependency-advisory-x-crypto-openpgp-fix-p1-contract.md`](dependency-advisory-x-crypto-openpgp-fix-p1-contract.md)（D1 精确固定项与
2026-12-16 期限、D2 任何 finding 都阻止发布、D3 OpenPGP 无条件禁止）、[`go-advisory-release-gate-contract.md`](go-advisory-release-gate-contract.md)。
本合同只取代前者的 D1 与 D4 中的 `pinReviewUntil`；D2、D3 与门的其它判定边界不变。

## 1. 范围与结论

main 的 `backend/go.mod` 固定 `golang.org/x/crypto v0.58.1-0.20261009181718-2f1f834330b2`（提交 `2f1f834330b2`，即 `v0.58.0` 只删
`openpgp/`，GO-2026-5932 的修复）。`scripts/check-go-advisories.mjs` 只在 **2026-12-16T00:00:00Z**（不含）之前接受这个伪版本，到期报
`x/crypto fix pin expired: move to a tagged release`。CI 在构建镜像前跑同一门，到期后 main 上任何提交都发不出镜像。前置合同要求到期前换成
包含该修复的标签版本，并删除门里的固定项常量与对应测试。

结论：**已有包含修复的标签版本 `v0.59.0`**（2026-11-11T07:23:42Z），审查日距期限还有 32 天。裁决 **must-fix（期限前处理的发布阻塞）**：
现在升级，同时退役固定项。

## 2. 查证（2026-11-14）

### 2.1 Go 模块代理 `proxy.golang.org`

| 版本 | `.info`（时间 / 提交 / ref） | `.zip` 条目 | 其中 `openpgp/` |
| --- | --- | --- | --- |
| `v0.58.0` | 2026-10-09T16:24:17Z / `8fdf07b3114278fb3b78c1cf1814e08bd851eb4c` / `refs/tags/v0.58.0` | 388 | 51 |
| 固定伪版本 | 2026-10-09T18:17:18Z / `2f1f834330b2d36ab3db8bfb33aaa1ac16794588` / 无 ref | 337 | 0 |
| **`v0.59.0`** | 2026-11-11T07:23:42Z / `6c966a7517bfc4fa0f268afb1fe663013ed932a5` / `refs/tags/v0.59.0` | 340 | **0** |

- `@v/list` 的标签版本到 `v0.59.0` 为止，`@latest` 也是 `v0.59.0`；没有 v0.59.x 补丁或更新的版本。
- 解压比较固定伪版本与 `v0.59.0`：只改了 `blake2s`（新增 loong64 汇编与 `blake2s_var.go`、改 `blake2s_ref.go`）、`ssh/**`（含 agent、
  knownhosts、testdata）、`x509roots/gen_fallback_bundle.go` 以及 `go.mod`/`go.sum`。三张依赖图里只有 `bcrypt`、`blowfish`、`sha3`
  三个 x/crypto 包，这三个包在两版之间逐字节相同。
- `v0.59.0` 的 `go.mod`：`go 1.26.0`；要求 `golang.org/x/net v0.62.0`、`golang.org/x/sys v0.50.0`、`golang.org/x/term v0.48.0`，
  间接要求 `golang.org/x/text v0.44.0`。

### 2.2 Go 漏洞库 `vuln.go.dev`

- `index/db.json`：`modified 2026-11-11T02:51:11Z`。
- [GO-2026-5932](https://vuln.go.dev/ID/GO-2026-5932.json)（`modified 2026-11-02T22:01:43Z`，`REVIEWED`）与前置合同记录时相同：
  `golang.org/x/crypto` 范围是 `introduced 0`、`fixed 0.58.1-0.20261009181718-2f1f834330b2`；独立模块 `golang.org/x/crypto/openpgp`
  是 `introduced 0`、无修复；imports 都是 openpgp 族 7 个包。记录还没有改写成 `v0.59.0`，但按 SEMVER 顺序 `0.59.0` 大于修复伪版本，
  已在受影响范围之外。
- 按 `index/modules.json` 取出 x/crypto（32 条）、x/net（37 条）、x/sys（2 条）、x/text（5 条）的全部记录，逐条按 SEMVER 范围求值：
  `x/crypto v0.59.0`、`x/net v0.62.0`、`x/sys v0.50.0`、`x/text v0.44.0` 都不在任何范围内；`x/term` 在库里没有记录。用作对照的
  `x/crypto v0.58.0`（命中 GO-2026-5932）、`x/net v0.61.0`（命中 GO-2026-6860/6866）、`x/text v0.40.0`（命中 GO-2026-6629）都按预期命中。

### 2.3 校验和库 `sum.golang.org`

| 模块 | `h1`（zip） | `h1`（`go.mod`） |
| --- | --- | --- |
| `golang.org/x/crypto v0.59.0` | `h1:xS2jhqYbzNrpvkWP2YTDf4TJwBy3ykR03i8vFDxEc7c=` | `h1:veDyUwATGs3c+vN3VfTbzViMGXeTBk3WyhBOkn+F/4U=` |
| `golang.org/x/sys v0.50.0` | `h1:dkKZEFTEKaLr1sjhs0WiKqoi8Jj5nHbS3Njmx1UoMJs=` | `h1:hNLxWAXmnKAxqDtdwIYC4bM9oQPEecfsnNMuSxOs3og=`（与 v0.49.0 相同） |
| `golang.org/x/text v0.44.0` | `h1:YItdp/mQNGnsmrgpcdA5yQ3bnRxCfo/Tj/iByGW9GGo=` | `h1:S4FR1waQ9LbjQTls97SyRjDaUUVkaM+cXwwuG/G+MwY=` |

与下面预演生成的 `go.sum` 逐字一致。

## 3. 连带依赖与兼容性

按最小版本选择，升级 x/crypto 会连带把直接依赖 `golang.org/x/sys` 从 `v0.49.0` 升到 `v0.50.0`（2026-11-06，`00227f1d`），
`golang.org/x/text` 从 `v0.43.0` 升到 `v0.44.0`（2026-11-09，`6070ac32`），两者都是 `@latest`；`golang.org/x/net v0.62.0` 不变。

- **Go 版本**：x/crypto、x/sys、x/text、x/term 的 `go.mod` 都是 `go 1.26.0`，低于项目的 `go 1.26.11`；不需要 `toolchain` 行，`go` 指令不变。
- **x/sys v0.50.0**（非测试文件）：`unix/dirent.go` 让 Linux 上 `ParseDirent` 保留零 inode 条目（OpenReader 不调用 `ParseDirent`/`ReadDirent`，
  它用的是 rootedfs 等处的 openat、fstat、fstatat、unlinkat、mkdirat、renameat、renameat2、renameatx_np、linkat、mkfifo、fcntl）；
  `unix/ioctl_linux.go` 新增 `IoctlGetUint64`；`unix/sockcmsg_linux.go` 给 `ParseUnixCredentials` 加长度检查；其余是 cpu 的 wasm32 构建标签、
  windows、freebsd/386 与 ppc64 汇编，都不在本项目的调用路径或目标平台上。
- **x/text v0.44.0**（非测试文件）：`language/parse.go` 的 1000 个分隔符上限同时计入 `_`（`ParseAcceptLanguage` 的 DoS 加固）；
  `internal/export/idna/punycode.go` 不编译进本项目（x/net/idna 有自己的副本）。书源与本地书解码用的 encoding 各包（GBK/GB18030、Big5、
  Shift-JIS/EUC-JP、EUC-KR、charmap、htmlindex、unicode、utf32）以及 transform、unicode/norm 逐字节相同。
- **仅出现在模块图**：`golang.org/x/term v0.48.0` 与 x/text 的 `x/tools v0.52.0`、`x/mod v0.42.0`、`x/sync v0.25.0`；
  `go mod why -m golang.org/x/term` 为 “main module does not need module golang.org/x/term”，`go.mod`/`go.sum` 不记录它们。
- **依赖图**：现场重新生成宿主 `./...`、linux/amd64、linux/arm64（CGO 1）三张图，分别是 361、360、359 个包，与 main 上逐字节相同，
  都有 main/bcrypt，都没有 openpgp 族。

预演（草稿副本，`go get golang.org/x/crypto@v0.59.0` 后 `go mod tidy`）：`go.mod` 改 3 行（x/crypto、x/sys、x/text），`go.sum` 6+/6-，
`go mod verify` 为 “all modules verified”。在同一副本上用**现有**门脚本（它对标签版本走通用规则）实跑：exit 0，`pass`，go1.26.11 /
darwin-arm64 / CGO 1，DB `2026-11-11T02:51:11Z`，250 条公告，**0 个发现**，`crypto.version` 为 `v0.59.0`、`pinReviewUntil` 为 `null`。
证据目录 `/var/folders/5z/h__cbxf93c5gc1p4myzqpd480000gn/T/openreader-go-advisories-W8QXqM`。

## 4. 差异分类

| 编号 | 现状 | 分类 |
| --- | --- | --- |
| X1 | x/crypto 是未打标签的修复伪版本，门只接受到 2026-12-16 | **must-fix**：到期后阻止所有发布；已有包含修复的标签版本 |
| X2 | 门保留固定项特例（`cryptoPin`、`cryptoPinReviewUntil`、摘要 `pinReviewUntil`）；升级后仍允许在期限前退回这个伪版本 | **must-fix**：前置合同 D1 要求换版后删除；留着就是一个不再有用途的例外 |
| X3 | x/sys `v0.49.0` → `v0.50.0`、x/text `v0.43.0` → `v0.44.0` | **acceptable-change**：最小版本选择强制、都是 `@latest`，在本项目路径上只有加固 |

## 5. 方案

| 方案 | 内容 | 结论 |
| --- | --- | --- |
| A | 升级到 `v0.59.0`，删除固定项特例：x/crypto 只能是标签版本 | **采用** |
| B | 保持伪版本，延长复审期限 | 不采用：已有包含修复的标签版本，延期只会把未打标签的依赖和例外留得更久 |
| C | 在 A 的基础上，门再加 `x/crypto >= v0.59.0` 的下限 | 不采用：`go.mod` 的要求本身就是最小版本选择的下限；退回带 openpgp 的标签版本会产生 GO-2026-5932 模块级 finding，按 D2 阻止发布；导入 openpgp 族按 D3 阻止发布。下限只是又一个要手工维护的常量 |

## 6. 目标

1. **T1 依赖**：`backend/go.mod` 只改 `golang.org/x/crypto v0.59.0`、`golang.org/x/sys v0.50.0`、`golang.org/x/text v0.44.0` 三行；`go.sum`
   只换这三个模块的 6 行（与第 2.3 节一致）。`go 1.26.11`、其它依赖、应用代码不变，不加 `toolchain` 行。
2. **T2 门**：删除 `cryptoPin`、`cryptoPinReviewUntil` 及其注释。x/crypto 选中版本必须匹配 `vX.Y.Z`，否则报
   `x/crypto must be a tagged release`；旧固定伪版本在任何时刻都被拒绝；不新增任何带日期的常量。
3. **T3 摘要**：`crypto` 只有 `{version}`，去掉 `pinReviewUntil`（仓库里没有其它地方读它）。
4. **T4 不变**：三张图的 OpenPGP 禁令（D3）、任何 finding 都阻止发布（D2）、DB 14 天时效、协议/扫描器版本/source/symbol、SBOM 与实际版本一致、
   main/bcrypt 存在、进程组生命周期、workflow 接线。
5. 允许差异：无上游可见行为变化。

## 7. 计划测试（先红）

改写 `scripts/check-go-advisories.test.mjs`。OSV fixture 仍用 2026-11-02 版记录（修复事件为伪版本），基础 fixture 不变。

旧门脚本上应失败（3 项）：

- R1：标签版本 `v0.59.0` 通过且无发现，摘要 `crypto` 恰为 `{version:'v0.59.0'}`（旧门多出 `pinReviewUntil: null`）；
- R2：旧固定伪版本在旧期限前（2026-12-15T23:59:59Z）和期限时刻（2026-12-16T00:00:00Z）都以 `x/crypto must be a tagged release` 被拒，
  同一 DB 时点 `v0.59.0` 都通过（旧门在期限前接受伪版本）；
- R3：在默认时刻，旧固定伪版本以同一消息被拒（并入“不是标签版本”的用例表）。

两边都应通过的对照：其它 6 个伪版本/预发布/`+incompatible`/不完整版本被拒（用例表加上同一消息断言）；`v0.59.0` 在 2027-06-01（DB 新鲜）
仍通过，即门里不再有带日期的 x/crypto 例外；9 种 finding、修复前 `v0.58.0` 的模块级 finding、三张图 × 两条 openpgp 路径、协议/DB/SBOM/图/
进程/workflow 原有检查都不变。

随固定项一起退役的用例：“reviewed x/crypto fix pin passes clean”与“reviewed pin is accepted until 2026-12-16”，由 R1、R2 取代（不是放宽：
旧固定项从期限内接受变为任何时刻都拒绝）。

## 8. 不重开

govulncheck `v1.9.0`、DB 14 天时效、Go 工具链 `go 1.26.11` 与构建镜像 pin、x/net 与其它依赖、前端门、OS/APK 门、workflow 次序不变；
source 门仍不代表整镜像 clean（C/OS/Node/frontend 未扫描）；本地不构建镜像；不访问生产。

## 9. 验证

1. 红：新门测试在旧门脚本上只有第 7 节的 3 项失败。
2. 实现后同一断言转绿；`go mod verify` 通过；现场 Go 门（沙箱外）exit 0、`pass`、0 个发现，`crypto` 为 `{version:'v0.59.0'}`；前端门 exit 0。
3. backend `go test ./...`、`go vet ./...`、`go test -race -timeout 40m ./api ./db ./services/...`（沙箱外）；
   `node --test scripts/check-go-advisories.test.mjs scripts/check-frontend-advisories.test.mjs`；按 workflow 的 `go mod vendor -o` 生成 vendor，
   `-mod=vendor -trimpath` 编译，二进制记录的依赖版本与 `go.sum` 一致。
4. frontend `npm test`、`npm run build`；真实 Go 服务上的 `scripts/smoke/openreader-smoke.mjs`（1440×900、390×844、360×800 首页非空白、
   无页面错误、无 5xx API）；隔离存储 HTTP 合同 `scripts/smoke/store-read-list-lifecycle-contract.mjs`（真实服务的注册与 WebDAV Basic
   都经过 bcrypt）。
5. 推送分支；合入 main 与发布由协调会话决定，发布后由可信 Actions 复核。
