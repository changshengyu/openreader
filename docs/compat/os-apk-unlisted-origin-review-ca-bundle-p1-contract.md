# OS APK 发布门：未列来源复审与运行时证书包 20261020-r0（eMudhra 根证书移除）P1 合同

日期：2026-11-10。分支 `claude/os-unknown-origin-review`，基于 main `7ab3fe3`。用户要求“先做门复审”。
门的设计见 [`sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md`](sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md)
第 11–13 节；构建期精确安装的先例见 [`os-apk-zlib-1.4-release-blocker-p1-contract.md`](os-apk-zlib-1.4-release-blocker-p1-contract.md)
与 [`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)
第 11 节。本合同不重开门的设计。上游 reader-dev 不涉及（运行时镜像的 OS 包维护，属于允许差异中的安全修复）。

## 1. 起因（must-fix，2026-11-16 起所有发布被拦）

`scripts/os-apk-baseline.json` 的 5 个 origin 在 v3.23 secdb 中没有记录：`alpine-base`、`alpine-baselayout`、`alpine-keys`、
`ca-certificates`、`pax-utils`。门把它们记为 unknown（不是 clean），只在 `reviewUntil` 之前放行：
`scripts/check-os-advisories.mjs` 第 371 行 `assert.ok(now < reviewUntil, 'unlisted origin review expired: …')`。现有期限
`2026-11-16T00:00:00Z` 来自 2026-10-13 的 secdb 快照，到期后每次发布都会在 OS 门失败关闭。复审要用当前证据重新判断这 5 个 origin，
不能只改日期。

## 2. 复审证据（2026-11-10）

### 2.1 secdb 与签名索引

- v3.23 main secdb：Last-Modified Tue, 10 Nov 2026 07:25:11 GMT，sha256 `d51b07b486dd1f9832cc27e0934d435ad9ebfaec90553c20c43e4d54128d5647`，
  283 个包；community：Sat, 07 Nov 2026 07:30:27 GMT，sha256 `fee2491c8a77d6268b33966d79424941a13bdfa5502cc3dfb812e3048c104ae4`，461 个包。
  与发布 run `42780054569` 读取的两份相同。5 个 origin 在两份中都仍无记录；运行时 origin（apk-tools、busybox、musl、openssl、zlib）
  的记录与夹具 `secdb-v3.23-main-subset-2026-11-10.json` 逐项相同。
- v3.23 main APKINDEX（两架构索引描述都是 `v3.23.8-20-g8b367fdee54`，与 zlib 切片所用相同）：

  | | x86_64 | aarch64 |
  | --- | --- | --- |
  | sha256 | `f961a8b173cf90b7fc388e1c201ebc356a694b4498b8160198773a3d3a51e3e0` | `7a580b06d20ba285dcf10f86cd79c8a9708d54ddd3b3da4457c4510a854e1ce4` |
  | Last-Modified | Tue, 10 Nov 2026 07:25:31 GMT | 07:25:30 GMT |
  | 签名 | `.SIGN.RSA.alpine-devel@lists.alpinelinux.org-6165ee59.rsa.pub`，`openssl dgst -sha1 -verify` OK | `…-616ae350.rsa.pub`，OK |

  验签公钥取自 `alpinelinux.org/keys/`，其内容的 `Q1` 校验和与已发布镜像安装库里 `alpine-keys` 2.6-r0 的同名文件逐字一致
  （`Q1lZlTESNrelWTNkL/oQzmAU8a99A=`、`Q1V/a5P9pKRJb6tihE3e8O6xaPgLU=`）。

### 2.2 逐个 origin

| origin（安装的包） | 基线版本 | 签名索引现版本 | aports（2026-09-01 起，3.23-stable / master） | 外部公告 | 结论 |
| --- | --- | --- | --- | --- | --- |
| alpine-base（alpine-release） | 3.23.6-r0 | 3.23.8-r0 | `13bdc4d` release 3.23.7（10-21）、`76ca532` release 3.23.8（11-05）/ 无 | NVD、OSV 无 | 只差版本号：3.23.8-r0 的 `etc/secfixes.d/alpine` 与 3.23.6-r0 逐字节相同（只列 secdb 地址），只有 `etc/alpine-release`、`usr/lib/os-release` 不同 |
| alpine-baselayout（+ -data） | 3.7.2-r0 | 3.7.2-r0 | 无 / 无 | NVD 0、OSV 无 | 无变化 |
| alpine-keys | 2.6-r0 | 2.6-r0 | 无 / 无 | NVD 0、OSV 无 | 无变化 |
| ca-certificates（ca-certificates-bundle） | 20260909-r0 | 20261020-r0 | `ac857c2` **“main/ca-certificates: security upgrade to 20261020”**（10-20）/ `1c5642b` 同名 | 见 2.3 | **must-fix**（U2） |
| pax-utils（scanelf） | 1.3.8-r2 | 1.3.8-r2 | 无 / `b52be34` upgrade 1.3.9→1.3.11（10-06，无 secfixes） | NVD（pax-utils、scanelf）0、OSV 无；上游 v1.3.8..v1.3.11 共 7 个提交，均非安全修复 | 无变化；OpenReader 运行时不调用 scanelf |

OSV 按 `Alpine:v3.23` 查询 5 个 origin 都无条目（OSV 的 Alpine 数据来自 secdb，只作旁证）；NVD 按关键字查询发布于 2026-09-01 至 11-11 的条目。

### 2.3 ca-certificates 20261020-r0：移除 3 个 eMudhra 根证书

- 旧证书包取自固定 base `alpine:3.23.6@sha256:85fe1e81…`（amd64 清单 `sha256:1f3591b8…`，层 `sha256:d0c1d894…`，均按摘要核验）：
  `etc/ssl/certs/ca-certificates.crt` 的 `Q1reWe8OzoZ6gWJYQl50oNHcmLboc=` 与已发布镜像安装库记录相同，121 个根证书。
- 新包 `ca-certificates-bundle-20261020-r0.apk`：control 段 `Q1` 等于签名索引的 `C:`，`.PKGINFO` 的 `datahash` 等于 data 段 sha256，118 个根证书。
  旧包 20260909-r0 已从 CDN 下架（HTTP 404）。
- 差异只有移除 3 个、新增 0 个：`emSign ECC Root CA - C3`（eMudhra Inc）、`emSign ECC Root CA - G3`（eMudhra Technologies Limited）、
  `emSign Root CA - C1`（eMudhra Inc）。`emSign Root CA - G1` 在新旧包中都保留（Mozilla 未移除）。
- 来源：Alpine `ca-certificates` 标签 20261020 = certdata `NSS_3_131_1_RTM`（MR !32）。NSS 3.131.1 发布说明只有两条：
  Bug 2084101（移除 emSign G3，bug 中的证书为 `emSign ECC Root CA - G3`）与 Bug 2083709（移除 emSign Root CA - C1 与 emSign ECC Root CA - C3），
  两者都挂在 Bug 2018979（eMudhra 双用途根证书过渡计划）下；2083709 的说明是 eMudhra 申请整根移除，取代原定的 2026-06-15 distrust-after。
- 背景事件：eMudhra 的 **8 个中间 CA 证书私钥已泄露**，eMudhra 于 2026-10-18 吊销（CVE-2026-22063，NetApp 追踪）。Red Hat 为系统证书库
  发布 CVE-2026-114406（CVSS 3.1 6.8，`AV:N/AC:H/PR:N/UI:R/S:U/C:H/I:H/A:N`）：中间人可用这些弱密钥中间证书为任意主机名签发证书，
  由于无法在所有客户端只拒绝特定中间证书，签发它们的根证书被移除。Electron、HCL、MongoDB DB Tools、Checkmk、Elastic 均为各自内置证书库发布 CVE。
- Go：GO-2026-6809（CVE-2026-111943，“Distrust compromised intermediate certificates in crypto/x509”，Go issue 82345，CL 851047）在
  1.26.10 / 1.27.3 给 `Certificate.Verify` 加了 8 个 SPKI sha256 的拒绝列表，含这些证书的链会被过滤。OpenReader 自 `92c98a9`（2026-10-22）起用
  Go 1.26.10 构建，所以已知的 8 个中间证书在 OpenReader 的 TLS 校验中已被拒绝；但运行时系统证书库仍信任它们的 3 个上级根证书。
- OpenReader 的出站 HTTPS（书源搜索/探索/详情/目录/正文、封面与章节图片、RSS、远程书源导入）都用 Go `crypto/x509` 读取
  `/etc/ssl/certs/ca-certificates.crt`。运行时镜像里其它 TLS 使用者只有 apk（仅构建期）和 busybox `ssl_client`（运行时不用）。

### 2.4 在固定 base 上执行相同安装（两平台）

一次性容器（`--rm`，不构建、不推送镜像）在固定 `alpine:3.23.6` 上执行：
`apk add --no-cache 'zlib=1.4-r0' 'musl=1.2.5-r24' 'musl-utils=1.2.5-r24' 'ca-certificates-bundle=20261020-r0'`。

- 结果：16 个包；`ca-certificates-bundle` 20261020-r0 的 `C:` 为 x86_64 `Q1GWiqMbsMKDpDSINOqReDnfKrxJg=`（S 107116）、
  aarch64 `Q1pIV20Ndb7zRnRnaqozseLjzimd0=`（S 107097），I 都是 178791，aports 提交 `ac857c298f6fd212498ce76bb87836d940cdba2d`，与签名索引相同。
  安装库除这一块外，与现有候选（已发布 base + zlib 1.4-r0 + musl r24 的真实安装块）逐字相同，顺序不变。
- 证书文件原地替换：`ca-certificates.crt` 变为 `Q1mT0WUgIZu3o4++yVm+9ubDFAdCA=`（118 个证书），与安装库记录相同，没有 `.apk-new`；
  包内仍是 4 个条目（`etc/ssl/cert.pem`、`etc/ssl/certs/ca-certificates.crt`、`etc/ssl1.1/cert.pem`、`etc/ssl1.1/certs`），文件数 110/109 不变。
- `etc/apk/world` 增加 `ca-certificates-bundle=20261020-r0`，其余条目不变。
- 本机注意（U6）：OrbStack 启动容器时会向 `/etc/ssl/certs` 注入 `orbstack-root.crt` 并改写 `ca-certificates.crt`（121 → 122 个证书）。
  apk 遇到被改过的受保护文件时会把新包写成 `ca-certificates.crt.apk-new`、旧文件保留。上面的结果是先把文件恢复成 base 层原始字节
  （`Q1reWe8O…`）后得到的，与 CI 构建的条件一致（10-22 起 CI 中 base 证书文件与安装库记录相同，门一直通过）。

## 3. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| U1 | 未列来源复审 2026-11-16T00:00Z 到期，之后所有发布在 OS 门失败关闭 | `must-fix`（发布阻塞） | 按第 2 节证据重写 5 个 origin 的复审说明，期限改为 U7 |
| U2 | 运行时证书包 20260909-r0 仍信任 NSS 3.131.1 已移除的 3 个 emSign 根证书；Alpine 标为 security upgrade，但 secdb 没有记录，门看不到 | `must-fix`（P1 安全） | 构建期精确安装签名索引中的 `ca-certificates-bundle=20261020-r0`，基线固定两平台身份 |
| U3 | alpine-release 有 3.23.8-r0，只差版本元数据 | `acceptable-change` | 基础镜像仍固定 3.23.6（不重开） |
| U4 | alpine-baselayout、alpine-keys、pax-utils 版本未变，无公告 | `aligned` | 只更新复审说明 |
| U5 | Go 1.26.10 已拒绝 8 个泄露的中间证书；`emSign Root CA - G1` 仍在 Mozilla 证书库 | 不重开 | 保持 |
| U6 | 本机 OrbStack 注入根证书会让 apk 写 `.apk-new`、留下旧证书包 | 测试环境观察（非产品差异） | 门已失败关闭（APK 拥有文件校验和不符）；补一条测试固定这一点；本机取证须先恢复原始文件 |
| U7 | 新的复审期限 `2026-12-16T00:00:00Z`（35 天，与 x/crypto 伪版本复审同一时刻） | `acceptable-change` | 仍记为 unknown，不是 clean；secdb 收录任一 origin 即失败（`replace its unlisted review`） |

代价：精确 pin 在 Alpine 发布更新的证书包后会因旧包下架而无法无缓存构建（20260909-r0 已 404），发布失败关闭，直到新合同更新 pin，
与 zlib/musl 先例相同。只由这 3 个根证书签发的网站，OpenReader 将无法建立 TLS（与 Firefox/Chrome 移除后的行为一致）。

## 4. 目标与实施约束

- `Dockerfile` 运行时阶段只有一条安装：
  `RUN apk add --no-cache 'zlib=1.4-r0' 'musl=1.2.5-r24' 'musl-utils=1.2.5-r24' 'ca-certificates-bundle=20261020-r0'`
  （仍在 FROM 之后、ARG 之前；不用 `--allow-untrusted`、upgrade、额外仓库、`--force` 或 latest），注释同步；证书包仍由 APK 拥有，不从 builder 复制。
- `scripts/os-apk-baseline.json`：两平台 `ca-certificates-bundle` 改为第 2.4 节的身份；world 增加 `ca-certificates-bundle=20261020-r0`；
  `buildTimeFixes['ca-certificates']` = `{from:'20260909-r0', to:'20261020-r0', ids:['CVE-2026-22063','CVE-2026-114406'],
  aportsCommit:'ac857c298f6fd212498ce76bb87836d940cdba2d', index:'v3.23.8-20-g8b367fdee54'}`；5 个 `unlistedOrigins` 说明改为本次复审结论；
  `reviewUntil` 改为 `2026-12-16T00:00:00Z`。
- 门脚本 `scripts/check-os-advisories.mjs` 不改；不加门例外。

## 5. 测试先行与验证

先行测试（`scripts/check-os-advisories.test.mjs`，在基线与 Dockerfile 更新前失败）：

- 夹具 `ca-certificates-bundle-20261020-r0-apkindex.json`：两架构的签名索引记录、在固定 base 上执行第 2.4 节安装得到的真实安装块、
  索引 sha256/描述/Last-Modified/签名，以及移除的 3 个根证书。
- T1 候选（zlib 1.4-r0、musl r24、证书包 20261020-r0）在 11-10 secdb 下通过：`pass-with-unknown-origins`，16/16 包、110/109 个文件，
  74 组比较，`buildTimeFixes['ca-certificates']` 与第 4 节完全相同。
- T2 上一次发布 `74cf0d0` 的安装库（证书包 20260909-r0）被新基线的身份 pin 拒绝（`ca-certificates-bundle 20260909-r0 … expected 20261020-r0`）。
- T3 包已是 20261020-r0、但 `ca-certificates.crt` 仍是旧字节并多出 `.apk-new`（U6 的情形）时，门在 `etc/ssl/certs/ca-certificates.crt` 失败关闭。
- T4 复审在 `2026-12-15T23:59:59Z` 仍有效、在 `2026-12-16T00:00:00Z` 过期；取代原来“11-16 过期”的断言（期限按 U7 移动，断言形式不变）。
- T5 运行时 Dockerfile 只有第 4 节那一条精确安装。
- 既有用例保持原意：代表更早发布的夹具（`previousPolicy`、`zlibOnly`、`baseOnly` 及对应安装库）保留证书包的已发布身份，
  原有断言不放宽。

验证：OS 门测试、Go/前端/OS 公告门自测、浏览器 smoke 自测、`docker compose config`、存储 HTTP 合同、现场 Go/前端扫描；推送前再核对
secdb、进行中的发布 run 与 `reviewUntil`。候选镜像的实际构建、两平台 OS 门、卷门与可信双平台发布只能在合入 main 后由 Actions 完成；
本地不构建、不推送镜像，生产不操作。

## 6. 明确不重开

- 门设计与门脚本、基础镜像 3.23.6、openssl 3.5.8-r0（secdb 仍无对应修复）、zlib 1.4-r0 与 musl r24 pin、x/crypto 伪版本复审、Go 1.26.10。
- `emSign Root CA - G1`：Mozilla 仍保留，本合同不自行移除根证书。

## 7. 状态

**contract-only**。
