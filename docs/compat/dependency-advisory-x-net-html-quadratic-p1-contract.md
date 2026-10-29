# 发布门新增依赖公告：`golang.org/x/net/html` 文本节点平方复杂度（P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本切片不涉及上游可见行为）。

审查日期：2026-10-29。实施基线：`OpenReader@07f4752`（`claude/day-night-import-alignment`）；`origin/main` 为 `b4cf2be`。
状态：**implementation-complete / regression-validated**（记录见第 5 节）。

前置合同：[`go-advisory-release-gate-contract.md`](go-advisory-release-gate-contract.md)、
[`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)。
`GO-2026-5932`（`crypto/openpgp`）的既有不适用结论与 2026-11-16 审查期限不变。

## 1. 范围与结论

日间/夜间与本地书导入两个切片（`07f4752`）回归时，Go 依赖公告门失败关闭：

| 门 | 结果 |
| --- | --- |
| `node scripts/check-go-advisories.mjs` | exit 1：`unreviewed advisory GO-2026-6860` |
| `node scripts/check-frontend-advisories.mjs` | exit 0 |

CI 在构建镜像前运行同一门，所以合入 main 的任何提交（包括现在的 `b4cf2be` 本身）都不会产生镜像。裁决：
**must-fix（发布阻塞）**，按第 3 节做最小升级，独立于日夜/导入切片提交。

## 2. 公告与适用性

| 公告 | 当前 | 修复 | 适用性 |
| --- | --- | --- | --- |
| [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860)（CVE-2026-78661，2026-10-27 公开）：`html` 解析在 `addText`/`fosterParent` 合并相邻文本节点时用字符串拼接，大量短文本夹着被忽略的结束标签（如 `a</x>`）会产生 O(n²) 分配与复制 | `golang.org/x/net v0.60.0` | `v0.62.0` | **可达**。govulncheck v1.9.0 符号级发现两条轨迹：`goquery.NewDocumentFromReader → html.Parse`，由 `engine.FetchSourceDocumentWithURLContext` 解析书源返回的页面（main 上已存在）；`html.ParseFragment`，由新加的 `engine.readerDevEPUBIntro` 解析 EPUB `dc:description`。远端书源页面与用户上传的 EPUB 都可触发解析阶段的 CPU/内存放大 |

两条路径的输入都有既有上限（远端响应体大小上限、EPUB 单条目字节上限），但上限内仍是平方复杂度，因此不以上限代替修复。

## 3. 目标

- `golang.org/x/net` 精确升级到 `v0.62.0`；最小版本选择随之要求 `golang.org/x/crypto v0.58.0`、`golang.org/x/sys v0.49.0`、
  `golang.org/x/text v0.43.0`（`x/net v0.62.0` 的 `go.mod`），不升级其它依赖，不改 Go 工具链（`go 1.26.10`）、门脚本或豁免清单。
- 应用代码不变；`go.sum` 只随上述模块变化。

## 4. 验证

1. 红：升级前 `node scripts/check-go-advisories.mjs` 在 `07f4752` 上 exit 1（上表）。
2. 升级后同一门 exit 0，门测试、前端门不变。
3. backend `go test ./...`、`go vet ./...`、`go test -race`（engine、localbook、importstage、api），frontend 全量与 build。
4. 发布后由可信 Actions 的 Go 门与 OS/APK 门复核；本地不构建镜像。

## 5. 记录（2026-10-29）

合同 `53770f4`；升级 `7a18346`（`go.mod` 只改 `x/crypto v0.58.0`、`x/net v0.62.0`、`x/sys v0.49.0`、`x/text v0.43.0`，`go.sum` 8+/8-）。
Go 公告门在 `07f4752` 上 exit 1（`unreviewed advisory GO-2026-6860`），`7a18346` 上 exit 0（249 条公告、1 个发现为已评审不适用的
`GO-2026-5932`）；门测试 109/109；前端公告门 exit 0；`docker compose config` 通过。backend `go test ./...` 29 个包、`go vet ./...`、`go test -race`（engine、services/localbook、services/importstage，以及 api 1037 秒）在 `7a18346` 上通过（其后只改前端与冒烟脚本），未见 DATA RACE；frontend 827/827 与 build 通过。

状态：**implementation-complete / regression-validated**，待可信 Actions；镜像未发布；生产未访问。
