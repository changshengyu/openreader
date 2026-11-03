# 发布门依赖公告：GO-2026-5932（`x/crypto/openpgp`）出现修复版本（P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本切片不涉及上游可见行为）。

审查日期：2026-11-03。实施基线：`origin/main` = `69c27bc`；分支 `claude/go-advisory-x-crypto-openpgp-fix` 从 `origin/main` 切出。
状态：**contract-only**。

前置合同：[`go-advisory-release-gate-contract.md`](go-advisory-release-gate-contract.md)（“唯一有条件不适用项”）、
[`dependency-advisory-x-net-html-quadratic-p1-contract.md`](dependency-advisory-x-net-html-quadratic-p1-contract.md)。
本合同取代前者的不适用项一节；该门其它判定边界不变。

## 1. 范围与结论

Go 依赖公告门在 `69c27bc` 上失败关闭（沙箱外实跑，证据目录
`/var/folders/5z/h__cbxf93c5gc1p4myzqpd480000gn/T/openreader-go-advisories-NYnhp9`）：

| 门 | 结果 |
| --- | --- |
| `node scripts/check-go-advisories.mjs` | exit 1：`advisory changed: review required`；go1.26.10，DB `2026-11-02T22:01:43Z`，249 条公告，1 个发现 |
| 现场三张依赖图 | 宿主 `./...` 360、linux/amd64 359、linux/arm64 358 个包；都有 main/bcrypt，都没有 openpgp 族 |

唯一发现仍是既有不适用项 GO-2026-5932 的纯模块级 x/crypto 发现，但现在带 `fixed_version`，原合同第 1 条“无已修复版本”的前提失效，
门按设计要求复审。CI 在构建镜像前跑同一门，main 上任何提交（包括等待中的 Reader 菜单切片）都不会产生镜像；即使不处理，原不适用项
也在 2026-11-16 到期。裁决：**must-fix（发布阻塞）**。

## 2. 公告变化与修复提交

[GO-2026-5932](https://vuln.go.dev/ID/GO-2026-5932.json) 于 2026-11-02T22:01:43Z 修改（`review_status: REVIEWED`）：

| 项 | 2026-10-16 裁决时 | 现在 |
| --- | --- | --- |
| `golang.org/x/crypto` 范围 | `introduced 0`，无 `fixed` | `introduced 0`，`fixed 0.58.1-0.20261009181718-2f1f834330b2` |
| 其它 affected | 无 | 新增独立模块 `golang.org/x/crypto/openpgp`（代理上只有 `v0.1.0-deprecated`，2026-10-09），`introduced 0`，无修复 |
| imports | `openpgp` 及 6 个子包 | 两个 affected 都是同一 openpgp 族 7 个包 |
| 本仓发现 | 纯模块级，无 `fixed_version` | 纯模块级，`fixed_version v0.58.1-0.20261009181718-2f1f834330b2` |

修复是 x/crypto 提交 `2f1f834330b2d36ab3db8bfb33aaa1ac16794588`（2026-10-09T18:17:18Z），紧跟 `v0.58.0`（`8fdf07b3`，同日
16:24:17Z）。2026-11-03 代理 `@latest` 仍是 `v0.58.0`：没有包含修复的标签版本，只能以伪版本选择。逐文件比较模块缓存，伪版本相对
`v0.58.0` **只删除 `openpgp/` 目录**；`go.mod`（`h1:Xh+kl5A+…`，两版相同）、`bcrypt`、`blowfish` 等其余文件逐字节相同；模块哈希
`h1:zGDaB+/Mus2mScKvZ23k+W9NAUbP56sNM9SptgvWI5A=` 与 sum.golang.org 记录一致。

适用性不变：OpenReader 从不导入 openpgp 族，发现只因该包位于同一模块。

## 3. 方案与用户裁决

| 方案 | 内容 | 结论 |
| --- | --- | --- |
| A | x/crypto 固定到修复伪版本；门只接受这一个精确伪版本并设复审期限；退役不适用项 | **用户 2026-11-03 选定** |
| B | 保持 `v0.58.0`，带新期限重新认定不适用 | 不采用：已有修复仍豁免，且门要同时放宽修复版本、修复事件、新增模块三处检查 |
| C | 等 x/crypto 打出包含修复的标签版本再升级 | 不采用：期间所有发布阻塞，时间不确定 |

在草稿副本预演方案 A（`go get` 后 `go mod tidy`，govulncheck v1.9.0 实扫）：exit 0、stderr 为空、DB `2026-11-02T22:01:43Z`、
249 条公告、**0 个发现**（GO-2026-5932 记录仍在输出里，但不再匹配选中版本）。

## 4. 目标

1. 依赖：`backend/go.mod` 只把 `golang.org/x/crypto` 精确改为 `v0.58.1-0.20261009181718-2f1f834330b2`；`go.sum` 只把 x/crypto 的
   两行换成该版本的两行（预演 2+/2-）。其它依赖、Go 工具链 `go 1.26.10`、应用代码不变。
2. 门脚本 `scripts/check-go-advisories.mjs`：
   - **D1 精确固定项**：x/crypto 选中版本仍须是标签版本 `vX.Y.Z`，唯一例外是精确等于上述伪版本，且只到
     **2026-12-16T00:00:00Z**（不含）。到期即失败，要求换成包含该修复的标签版本，届时删除固定项。其它伪版本、预发布、
     `+incompatible` 一律失败。
   - **D2 退役不适用项**：任何 finding（模块/包/符号级，有无修复版本，任何 OSV 元数据）都阻止发布。结果只有 `pass`，
     不再有 `pass-with-not-applicable`，也不再有公告级到期日。
   - **D3 OpenPGP 无条件禁止**：原为不适用项前提的“三张图都没有 openpgp 族”保留为无条件检查，有无 finding 都适用
     （例如引入独立的 `golang.org/x/crypto/openpgp` 模块）。
   - **D4 摘要**：summary 增加 `crypto.version` 与 `crypto.pinReviewUntil`（标签版本为 `null`），去掉 `notApplicable`/`reviewUntil`。
   - 协议、扫描器版本、source/symbol、DB 时效、Go 版本、SBOM 与实际版本一致、main/bcrypt 存在、进程组生命周期与 workflow
     接线都不变。
3. 允许差异：无上游可见行为变化。固定到未打标签的提交是为安全修复设的限时例外（D1）。

## 5. 计划测试（先红）

门合同测试改写为修复后的情形：基础 fixture 是标签版本 x/crypto、2026-11-02 版 OSV 记录、无 finding。

旧门脚本上应失败（9 项）：

- 精确固定伪版本 + 现行 OSV + 无 finding 通过，摘要给出该版本与 `2026-12-16T00:00:00.000Z`；
- 固定项在 2026-12-15T23:59:59Z 仍通过、在 2026-12-16T00:00:00Z 以到期失败，标签版本同一时刻通过（同一测试，DB 时点相同）；
- 修复前形态（`v0.58.0`、无修复的旧 OSV、纯模块 finding）不再算不适用项，阻止发布；
- 三张图 × `openpgp`/`openpgp/packet`：没有 finding 也阻止发布（6 项）。

两边都应通过的对照：标签版本无 finding 通过；6 个其它伪版本/预发布/`+incompatible` 被拒；9 种 finding（原不适用的纯模块形态、
本次现场带修复版本的形态、未知公告、空 trace、版本不符、缺版本、其它模块、包级、符号级）都阻止发布；协议/DB/SBOM/图/进程/
workflow 原有检查。

随不适用项一起退役的用例（不是放宽：D2 让任何 finding 都失败）：“只有精确 OpenPGP 模块 finding 不适用”、“不适用项到期需复审”，
以及只服务于不适用项前提的 4 个 OSV 元数据用例（缺 imports、新增非 OpenPGP 包、异常模块、新增修复版本）。原 7 个 finding 帧用例
并入上面的 finding 对照表。

## 6. 不重开

govulncheck 版本、DB 14 天时效、Go 工具链、其它依赖、前端门、OS/APK 门、workflow 次序不变；source 门仍不代表整镜像 clean
（C/OS/Node/frontend 未扫描）；本地不构建镜像；不访问生产。

## 7. 验证

1. 红：旧门脚本上第 5 节 9 项失败、其余通过；现场门在 `69c27bc` 上 exit 1（第 1 节）。
2. 实现后同一断言转绿；现场 Go 门 exit 0、`pass`、0 个发现；前端门 exit 0。
3. backend `go test ./...`、`go vet ./...`（沙箱外）；`node --test scripts/check-go-advisories.test.mjs scripts/check-frontend-advisories.test.mjs`。
4. 推送分支；合入 main 与发布由用户决定，发布后由可信 Actions 复核。
