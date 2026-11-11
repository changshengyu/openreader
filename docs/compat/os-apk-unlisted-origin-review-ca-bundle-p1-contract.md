# OS APK 发布门：unknown origin 复审续期与运行时 CA bundle 20261020-r0（emSign 根移除）P1 合同

日期：2026-11-10。分支 `claude/os-apk-unlisted-origin-rereview`，基于 main `752aa4e`。门的设计见
[`sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md`](sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md) 第 11–13 节，
最近的构建期修复见 [`os-apk-zlib-1.4-release-blocker-p1-contract.md`](os-apk-zlib-1.4-release-blocker-p1-contract.md)；本合同不重开门的设计。
复审中发现 CA bundle 问题后，用户批准在本切片内修复（“Pin it in this slice”）。

## 1. 问题

**复审到期（所有发布都会被拦）。** `scripts/os-apk-baseline.json` 把 v3.23 secdb 中没有记录的 5 个 origin（alpine-base、alpine-baselayout、
alpine-keys、ca-certificates、pax-utils）记为 unknown（不是 clean），复审期限 `reviewUntil` 为 2026-11-16T00:00:00Z，理由仍引用 10-13 的 secdb 快照。
到期后门报 `unlisted origin review expired: alpine-base`，此后每次发布都失败关闭。复审需要对每个 origin 用新证据重新确认。

**复审中发现：运行时 CA bundle 仍信任已被移除的 emSign 根。** 固定 base 自带的 `ca-certificates-bundle 20260909-r0`（已发布镜像 `74cf0d0` 中也是它）
仍包含 emSign ECC Root CA - G3、emSign Root CA - C1、emSign ECC Root CA - C3：

- Mozilla bug 2083449（2026-10-17 报告）：eMudhra 8 个中间 CA 的 P-384 私钥生成有缺陷，几乎可以立即恢复。其中 6 个是链到受信根的 TLS 中间 CA，
  持有私钥者可以为任意域名签发证书，实施 TLS 中间人；CT 日志中已有大量这类概念验证证书。Firefox 依靠 OneCRL 吊销这些中间 CA。
- Mozilla 于 10-20 紧急移除 emSign ECC Root CA - G3（bug 2084101），用于保护不使用吊销信息的客户端；C1/C3 按 eMudhra 的双用途根过渡计划
  （bug 2018979）应其请求整根移除（bug 2083709）。三者都在 NSS 3.131.1 中落地。
- Alpine `ca-certificates` 20261020 采用 certdata `NSS_3_131_1_RTM`（aports `ac857c298f6fd212498ce76bb87836d940cdba2d`，提交标题为
  “security upgrade”），相对 20260909 只移除这 3 个根，没有新增。APKBUILD 没有 secfixes，secdb 因此没有记录，OS 门看不到这次修复。
- OpenReader 的出站 HTTPS（`backend/engine/fetcher.go` 书源与 RSS 抓取、`services/coverimage`、`services/chapterimage`）使用 Go 系统根池
  （后端不设置 `RootCAs`，也没有 `InsecureSkipVerify`），Linux 上读 `/etc/ssl/certs/ca-certificates.crt`；Go 不检查吊销。因此当前运行时会接受
  这类可伪造的证书链，网络中间人可以伪装任意书源站点。
- Sectigo 曾交叉签名部分受影响的中间 CA，所用根 AAA Certificate Services 早已不在 Mozilla 根库，两个 bundle 都不含它；
  移除 3 个 emSign 根即切断已知的全部链。

上游 reader-dev 不涉及（运行时镜像的 OS 包维护，属于允许差异中的安全修复）。

## 2. 证据（2026-11-10 08:39–08:52 UTC 观察）

**secdb。** 5 个 origin 在 v3.23 main 与 community 中都没有记录（按名称精确匹配与子串匹配都没有）：

| 文档 | Last-Modified | sha256 | 包数 |
| --- | --- | --- | --- |
| v3.23 main | Tue, 10 Nov 2026 07:25:11 GMT | `d51b07b486dd1f9832cc27e0934d435ad9ebfaec90553c20c43e4d54128d5647` | 283 |
| v3.23 community | Sat, 07 Nov 2026 07:30:27 GMT | `fee2491c8a77d6268b33966d79424941a13bdfa5502cc3dfb812e3048c104ae4` | 461 |

与 run `42780054569` 读到的是同一份文档；运行时有记录的 origin（apk-tools、busybox、musl、openssl、zlib）与夹具
`secdb-v3.23-main-subset-2026-11-10.json` 相同。对照：edge main（11-10 07:52:19，297 包）/ community（11-09 14:19:30，503 包）与
v3.22 main（11-10 07:25:17，276 包）/ community（11-05 23:51:48，439 包）也都没有这 5 个 origin。

**固定 base。** Docker Hub 上 `alpine:3.23.6` 仍指向 index `85fe1e81…`：amd64 manifest `1f3591b8…`（层 `d0c1d894…`，diff_id `e63b02c2…`），
arm64 manifest `ae959799…`（层 `ace1621b…`，diff_id `18932007…`），创建于 2026-09-17；`alpine:3.23` 与 `3.23.8` 已是 `469c9ea4…`（不刷新 base）。
两层按摘要下载后用门自己的 `readTar`/`flattenLayers`/`parseApkDatabase` 解析：`lib/apk/db/installed` 与夹具 `fc4692c-linux-*-installed`
逐字节相同；5 个 origin 的 6 个包与基线的版本、origin、`C:` 相同，它们拥有的文件校验和全部一致（只有已审的 `etc/shadow` 不同）。
其中只有 `usr/bin/scanelf` 是 ELF，其余是 `/etc` 配置与目录、签名公钥、版本标记和 CA bundle，没有脚本。

**签名索引（v3.23 main）。** x86_64 `f961a8b1…e3e0`（Last-Modified 07:25:31）、aarch64 `7a580b06…1ce4`（07:25:30），描述 `v3.23.8-20-g8b367fdee54`
（与 zlib 修复时同一份），用固定 base 自带的 `etc/apk/keys`（6165ee59 / 616ae350）验签通过：

| origin | 包 | 基线（= 固定 base） | 当前签名索引 |
| --- | --- | --- | --- |
| alpine-base | alpine-release | 3.23.6-r0 | 3.23.8-r0（版本标记） |
| alpine-baselayout | alpine-baselayout、alpine-baselayout-data | 3.7.2-r0 | 同一构建，`C:` 相同 |
| alpine-keys | alpine-keys | 2.6-r0 | 同一构建，`C:` 相同 |
| ca-certificates | ca-certificates-bundle | 20260909-r0 | **20261020-r0** |
| pax-utils | scanelf | 1.3.8-r2 | 同一构建，`C:` 相同 |

**CA bundle 20261020-r0。** 包 x86_64 sha256 `a4c5de1bf37652cd98cbe71aec209512ae0508a964961957a6c9650f795e46c4`（107116 B）、aarch64
`bdf6033710a251d8239f63236eacf40c84afdf8f7b25bb58d69a2c75202350e7`（107097 B）。控制段 SHA1 与签名索引 `C:` 相同（x86_64
`Q1GWiqMbsMKDpDSINOqReDnfKrxJg=`，aarch64 `Q1pIV20Ndb7zRnRnaqozseLjzimd0=`），数据段 sha256 与 `.PKGINFO` 的 datahash `986485d8…ad20` 相同，
两架构 bundle 内容相同（178791 B）。证书由 121 个减为 118 个，只移除：

| 根 | SHA-256 |
| --- | --- |
| emSign ECC Root CA - G3（eMudhra Technologies Limited，P-384） | `86A1ECBA089C4A8D3BBE2734C612BA341D813E043CF9E8A862CD5C57A36BBE6B` |
| emSign ECC Root CA - C3（eMudhra Inc，P-384） | `BC4D809B15189D78DB3E1D8CF4F9726A795DA1643CA5F1358E1DDB0EDC0D7EB3` |
| emSign Root CA - C1（eMudhra Inc，RSA 2048） | `125609AA301DA0A249B97A8239CB6A34216F44DCAC9F3954B14292F2E8C8608F` |

**安装后的数据库块。** 沙箱内无法访问本地 Docker socket，因此没有在固定 base 中实际安装。块按 apk-tools 3.0.8 的写法推导：
签名索引记录 + `.PKGINFO` 的 replaces（`r:`）+ 数据段 PAX 校验和（`F:`/`R:`/`a:`/`Z:`）。两条规则都已对照：固定 base 中 7 个未变包的身份行
都等于索引记录加 `r:`/`q:`；用同一规则从 base 层重建的 20260909-r0 文件段与 base 自身的块逐字相同。新块只有 `ca-certificates.crt` 的 `Z:`
变为 `Q1mT0WUgIZu3o4++yVm+9ubDFAdCA=`，仍是 4 个文件。实际安装结果由 Actions 构建与两平台门核验。

## 3. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| O1 | 复审 2026-11-16 到期后所有发布失败关闭 | `must-fix` | 用本次证据重写 5 个理由，`reviewUntil` 改为 2026-12-10T00:00:00Z（30 天，与门对 secdb 的 30 天新鲜度上限一致） |
| O2 | 运行时 CA bundle 20260909-r0 仍信任 emSign ECC Root CA - G3（其中间 CA 私钥可恢复）与 C1/C3 | `must-fix`（安全修复） | 构建期精确安装签名索引中的 `ca-certificates-bundle=20261020-r0`，基线身份、world 与 `buildTimeFixes` 同步 |
| O3 | alpine-release 3.23.6-r0，索引已是 3.23.8-r0 | `acceptable-change` | 只是版本标记；base 与 os-release 3.23.6 由门固定，不刷新 base |
| O4 | alpine-baselayout 3.7.2-r0、alpine-keys 2.6-r0、scanelf 1.3.8-r2 | 不变 | 仍是当前签名索引中的同一构建 |
| O5 | 门逻辑、base 版本；索引中已有 apk-tools 3.0.9-r0、openssl 3.5.9-r0 | 不重开 | 有 secdb 记录，已安装版本无待修复项；出现即失败关闭 |

## 4. 目标与实施约束

- `Dockerfile` 运行时阶段：`RUN apk add --no-cache 'zlib=1.4-r0' 'musl=1.2.5-r24' 'musl-utils=1.2.5-r24' 'ca-certificates-bundle=20261020-r0'`
  （仍在 FROM 之后、ARG 之前，不用 `--allow-untrusted`、upgrade、额外仓库或 latest），注释同步；不从 builder 复制 CA bundle。
- `scripts/os-apk-baseline.json`：world 加 `ca-certificates-bundle=20261020-r0`；两平台 `ca-certificates-bundle` 改为第 2 节的身份；
  `buildTimeFixes` 新增 `ca-certificates`（20260909-r0 → 20261020-r0，`mozilla-bug-2083449`、`mozilla-bug-2084101`、`mozilla-bug-2083709`，
  上述 aports 提交与索引描述）；5 个理由改为引用 2026-11-10 的证据并保留 “unknown, not clean”；`reviewUntil` 为 2026-12-10T00:00:00Z。
- 门脚本 `scripts/check-os-advisories.mjs` 不改，不加门例外。
- 与 zlib Z2 相同：Alpine 发布更新的 ca-certificates 后，签名索引不再提供 20261020-r0，无缓存构建会失败，届时需要重新复审。

## 5. 测试先行与验证

先行测试（`scripts/check-os-advisories.test.mjs`，在 Dockerfile 与基线更新前失败）：

- 夹具 `ca-certificates-bundle-20261020-r0-apkindex.json`：两架构的签名索引记录、推导出的安装块、索引与包的来源（sha256、Last-Modified、签名）。
- 候选镜像改为 zlib 1.4-r0、musl r24 加 CA bundle 20261020-r0；测试中的全部历史清单（已发布 fc4692c、只修 zlib、只修 musl、10-22 发布、`74cf0d0`）
  都带 base 的 20260909-r0，必须先因 CA bundle 身份被拒；只把 CA bundle 换成 20261020-r0 后，各自仍按原来的 zlib/musl 身份被拒（原断言不变）。
  候选在 11-10 secdb 下通过并报告 `buildTimeFixes.ca-certificates`；world 缺少 CA pin 或 CA 版本漂移失败关闭。
- 复审：在旧期限 2026-11-16T00:00:00Z 仍通过且报告 5 个 unknown origin 和新期限，在 2026-12-09T23:59:59Z 通过，在 2026-12-10T00:00:00Z
  报 `unlisted origin review expired: alpine-base`；每个理由都引用 2026-11-10 的证据。原有的缺少复审与被 secdb 收录两个失败用例不变。
- 运行时 Dockerfile 只允许新的这一条精确安装。

预期：测试提交时，所有使用候选清单的用例、Dockerfile 与复审用例为红；CA 修复提交后只剩复审用例为红；复审续期提交后全部转绿。

验证：OS 门测试与 Go/前端公告门自测；`docker compose config`。候选镜像的实际构建、两平台 OS 门、卷门与可信双平台发布只能在合入 main 后由 Actions
完成；本地不构建、不推送镜像，生产不操作。

## 6. 明确不重开

- 门设计；基础镜像 3.23.6（3.23.7/3.23.8 已发布）；musl 与 zlib pin；apk-tools 3.0.9-r0 与 openssl 3.5.9-r0（secdb 无待修复项）。
- `reviewUntil` 是 5 个 origin 共用的一个期限，不能只续其中一部分；本次复审覆盖全部 5 个。

## 7. 状态

**contract-only**（由第 8 节记录取代）。

## 8. 记录（2026-11-10 至 11-11）

提交：合同 `07fb8ae`；测试 `1beb527`（夹具与 25 个 OS 门测试，修复前 11 个红：候选清单都因 `ca-certificates-bundle 20261020-r0` 对基线
`20260909-r0` 被拒，Dockerfile 缺少 pin）；构建 `5769695`（Dockerfile 一行、基线 world、两平台身份与 `buildTimeFixes.ca-certificates`，之后 24/25，
只剩复审用例在旧期限 2026-11-16 报 `unlisted origin review expired: alpine-base`）；复审续期 `8d34715`（5 个理由与 `reviewUntil` 2026-12-10，25/25）。

验证：OS 门测试 25/25；Go、前端、OS 公告门自测共 139/139；`docker compose config` 通过；浏览器 smoke 自测 10/10（在沙箱外运行：沙箱内它的
本地回环服务无法监听端口，第 4 项起挂起）。各阶段的红/绿都与第 5 节的预期一致。

推送前复查（2026-11-11 01:07 UTC）：v3.23 main secdb 已更新为 Tue, 10 Nov 2026 22:37:59 GMT（`e04eccd675a5f5e462b9d417e7a60552d5a24b378560e3caf5ffd60e31e09dd6`，
283 包），只新增 git、krb5、xen 的修复（都不在运行时镜像中），运行时 origin 的记录与夹具相同；community 未变；5 个 origin 仍没有记录。
签名索引已更新为 `v3.23.8-23-g2f20de35278`（x86_64 `76240b86…2a17`，22:43:31 GMT；aarch64 `a33bbc89…09fe`，22:46:29 GMT），用 base 自带公钥验签通过，
其中 zlib 1.4-r0、musl/musl-utils 1.2.5-r24、ca-certificates-bundle 20261020-r0 的 `C:` 与基线 pin 相同，alpine-baselayout、alpine-keys、scanelf 仍是同一构建。

未验证：含 CA bundle 20261020-r0 的候选镜像实际构建（CI 中的 `apk add`）、推导出的安装块与真实安装是否逐字相同、两平台 OS 门实际运行、卷门与可信
双平台发布，需合入 main 后由 Actions 执行；本地不构建、不推送镜像。在发布之前，已发布的 `74cf0d0`/`latest` 仍带 20260909-r0。状态
**build-fix-applied / review-renewed / locally-validated / Actions-pending**；分支已推送，未合入 main；镜像仍为 `74cf0d0`；生产未访问，实际运行版本未知。

## 9. 更正与补充核验（2026-11-11）

**更正第 1 节的暴露面。** OpenReader 自 `6ca7fbf`（2026-10-22，go.mod 与构建镜像升到 Go 1.26.10，随 `92c98a9` 发布）起用 Go 1.26.10 构建。
GO-2026-6809（CVE-2026-111943，Go issue 82345，CL 851047）在 `Certificate.Verify` 中按 SPKI sha256 拒绝已知的 8 个泄露中间证书，含它们的链会被过滤，
所以自 `92c98a9` 起发布的二进制已经拒绝经由它们的伪造链。第 1 节“当前运行时会接受这类可伪造的证书链，网络中间人可以伪装任意书源站点”，
以及 gap-analysis 与 matrix 中“出站 HTTPS 可被中间人伪装”，对这 8 个已知中间证书不成立。仍然成立的是：运行时 bundle 20260909-r0 仍信任它们的
3 个上级根。pin 20261020-r0 按 Mozilla 的整根移除，补上 Go 拒绝列表之外的中间证书，是纵深防御而不是唯一防线。同一事件的发行版 CVE 为 Red Hat
CVE-2026-114406（ca-certificates）。O2 的 must-fix 分类与实施不变；第 1 节、两条总账与构建提交 `5769695` 的说明保留原文，以本节为准。

**真实安装核验（关闭第 2 节与第 8 节中“推导、未实际安装”这一项）。** 在固定 `alpine:3.23.6@sha256:85fe1e81…` 上用一次性容器（`--rm`，linux/amd64
与 linux/arm64，不构建、不推送镜像）执行第 4 节同一条 `apk add`。执行前先把 `etc/ssl/certs/ca-certificates.crt` 恢复为 base 层原始字节
（`Q1reWe8OzoZ6gWJYQl50oNHcmLboc=`）：本机 OrbStack 在容器启动时会注入 `orbstack-root.crt` 并改写该文件，apk 遇到被改过的受保护文件就把新包写成
`.apk-new`、保留旧文件；CI 构建不受影响，门也会在该文件的校验和上失败关闭。结果：
- 两架构安装库除 `ca-certificates-bundle` 外，与现有候选（已发布 base 加 zlib 1.4-r0、musl r24 的真实安装块）逐字相同，包的顺序不变；
- 该块与本合同夹具中的推导块、签名索引记录在两架构都逐字相同；
- 证书文件原地替换为 `Q1mT0WUgIZu3o4++yVm+9ubDFAdCA=`（118 个根证书），没有 `.apk-new`；world 增加 `ca-certificates-bundle=20261020-r0`。

**并行分支。** `claude/os-unknown-origin-review`（`296ac3c`，复审期限 2026-12-16）独立完成了等价修复，Dockerfile 安装行与两平台身份完全相同；
按用户选择未合入、保留。main 采用本分支，`reviewUntil` 为 2026-12-10。

## 10. 发布（2026-11-11）

推送前（合并树）：OS 门测试 25/25；Go、前端、OS 公告门自测 139/139；浏览器 smoke 自测 10/10；`docker compose config` 通过；现场 Go 扫描 `pass`、0 个发现
（vulndb 2026-11-05T23:49:57Z），现场前端扫描 `pass`、0 个发现（132 个锁定包）；secdb main 仍为 22:37:59 GMT `e04eccd6…`、community `fee2491c…`，
5 个 origin 无记录，运行时 origin 与夹具相同；没有进行中的发布 run。按用户在 `claude/os-unknown-origin-review` 会话中的选择（发布本分支），
以合并提交 `2de73087`（`7ab3fe3` 加本分支 `fed1ab6`，`--no-ff`）快进 main（ls-remote 核验，未强推）。

可信 Actions run `42939172405` 43/43 步成功（01:28:14–01:45:12 UTC）：两平台 OS 门 `pass-with-unknown-origins`（16/16 包、110/109 个 APK 文件、
74 组比较无待修复项、secdb main 11-10 22:37:59 与 community 11-07、5 个 unknown origin、`reviewUntil` 2026-12-10），Go/前端公告门、卷门、发布与平台核验通过。

镜像：`ghcr.io/changshengyu/openreader:2de7308` 与 `latest` 指向 OCI index
`sha256:49b02ce1eed5c57eb211194120329eba10d771160abb5a0ac72c5264aae0ae75`；linux/amd64
`sha256:338746ed37a48b1132ba16a8a7ecad7979c9133d67ff7ce8e2199d2d7723f985`，linux/arm64
`sha256:da528bac0ea1ab224fb360f6a321d9952f40d976ee42dc8881181123248eda9a`，另有 attestation。两平台 `org.opencontainers.image.revision`
均为 `2de73087c3643667cab8ac3c98261e1ea6087831`。

发布后独立核验（按平台 digest 拉取，`docker create` 后 `docker cp`，不启动容器）：两平台 `ca-certificates-bundle` 为 20261020-r0，`C:` 分别为
`Q1GWiqMbsMKDpDSINOqReDnfKrxJg=` 与 `Q1pIV20Ndb7zRnRnaqozseLjzimd0=`，与基线 pin 相同，aports 提交 `ac857c2`；`etc/ssl/certs/ca-certificates.crt` 为
`Q1mT0WUgIZu3o4++yVm+9ubDFAdCA=`、118 个根证书，emSign 只剩 `emSign Root CA - G1`；world 与基线相同，16 个包。

状态 **released / Docker-published**；生产未访问，实际运行版本未知。下次复审期限 2026-12-10T00:00:00Z（x/crypto 伪版本 pin 2026-12-16 另行处理）。
