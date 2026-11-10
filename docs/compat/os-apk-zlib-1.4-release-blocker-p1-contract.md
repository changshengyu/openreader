# OS APK 发布门：运行时 zlib 1.4-r0（CVE-2026-4738）构建期修复 P1 合同

日期：2026-11-10。分支 `claude/alpine-zlib-cve-2026-4738`，基于 main `95f2840`。用户在发布被拦后批准本修复（“Yes, unblock it”）。
门的设计见 [`sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md`](sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md)
第 11–13 节，musl 先例见 [`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)
第 11 节；本合同不重开门的设计。

## 1. 发布被拦（must-fix，所有发布都被拦）

可信 Actions run `42775373137`（`95f2840`，退役书源/RSS 按用户写锁）在 OS APK 门之前 30/41 步通过，OS 门失败：
`pending Alpine secfix blocks release: linux/amd64 zlib 1.3.2-r1 < 1.4-r0 CVE-2026-4738; linux/arm64 zlib 1.3.2-r1 < 1.4-r0 CVE-2026-4738`。
卷门与发布被跳过，**没有发布镜像**：`ghcr.io/changshengyu/openreader:95f2840` 不存在，`latest` 仍为 `6006659`（index `9b64829f…23e5`）。

v3.23 main secdb（Last-Modified Tue, 10 Nov 2026 07:25:11 GMT，sha256 `d51b07b486dd1f9832cc27e0934d435ad9ebfaec90553c20c43e4d54128d5647`，283 个包）
相对 10-21 快照，在运行时的 origin（apk-tools、busybox、musl、openssl、zlib）中只新增 zlib `1.4-r0: [CVE-2026-4738]`。
community secdb（Last-Modified Sat, 07 Nov 2026 07:30:27 GMT，sha256 `fee2491c8a77d6268b33966d79424941a13bdfa5502cc3dfb812e3048c104ae4`，461 个包）
仍未收录任何 unlisted origin。

另一个问题：签名索引已不再提供 `zlib 1.3.2-r1`（固定 base 内 `apk policy zlib`：已安装 1.3.2-r0，仓库只有 1.4-r0），所以 Dockerfile
现有的精确 pin 在没有层缓存时无法构建；run `42775373137` 复用了 gha 缓存层。

上游 reader-dev 不涉及（运行时镜像的 OS 包维护，属于允许差异中的安全修复）。

## 2. 证据（固定 `alpine:3.23.6`，两平台，在容器内执行与运行时阶段相同的安装）

| | x86_64 | aarch64 |
| --- | --- | --- |
| v3.23 main APKINDEX sha256 | `f961a8b173cf90b7fc388e1c201ebc356a694b4498b8160198773a3d3a51e3e0` | `7a580b06d20ba285dcf10f86cd79c8a9708d54ddd3b3da4457c4510a854e1ce4` |
| `apk verify`（镜像自带可信密钥） | OK，`.SIGN.RSA.alpine-devel@lists.alpinelinux.org-6165ee59.rsa.pub` | OK，`…-616ae350.rsa.pub` |
| 索引描述 / Last-Modified | `v3.23.8-20-g8b367fdee54` / Tue, 10 Nov 2026 07:25:31 GMT | 同描述 / 07:25:30 GMT |
| `zlib` 1.4-r0 `C:`（S） | `Q1w8ww/Ov6R+0J7O4tgeMeDsf/vp0=`（56762） | `Q1w+byPv5bYwvQ54Xq5EaEc9vy8Go=`（55052） |

- aports 提交 `0436c6d655a04cbfd7987202a970f5aa7870a901`（两架构相同）。安装后数据库的身份行与签名索引记录逐字相同。
- 文件：`usr/lib/libz.so.1 → libz.so.1.4`（符号链接）与 `usr/lib/libz.so.1.4`，取代 r1 的 `libz.so.1.3.2`；每个包仍是 2 个文件。
- `etc/apk/world`：`zlib=1.4-r0` 取代 `zlib=1.3.2-r1`，其余条目（含 `musl=1.2.5-r24`、`musl-utils=1.2.5-r24`）不变；musl 两个包的身份与基线相同。
- `apk --no-network version -t`（apk-tools 3.0.8，`--network none`，两平台结果相同）：1.4-r0 对 zlib 全部 6 个修复版本
  （0、1.2.11-r4、1.2.12-r2、1.3.2-r0、1.3.2-r1、1.4-r0）为 `>` 或 `=`；1.3.2-r1 与 1.3.2-r0 对 1.4-r0 为 `<`。
- 运行时镜像里只有 `apk-tools` 与 `libapk` 依赖 zlib；OpenReader 二进制（Go）不链接 zlib。
- 索引描述为 `v3.23.8-…`，说明已有 3.23.8 point release；基础镜像仍固定 `alpine:3.23.6`，与 zlib/musl 先例一致，不刷新 base。

## 3. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| Z1 | 运行时 zlib 1.3.2-r1 有待修复的 CVE-2026-4738，OS 门失败关闭，所有发布被拦 | `must-fix` | 构建期精确安装签名索引中的 `zlib=1.4-r0` |
| Z2 | 精确 pin `zlib=1.3.2-r1` 已不在签名索引中，无缓存构建失败 | `must-fix` | 同上 |
| Z3 | zlib 上游从 1.3.x 到 1.4，共享库文件名变为 `libz.so.1.4`（soname `libz.so.1` 不变），只有 apk-tools/libapk 使用 | `acceptable-change`（安全修复） | 记录于此 |
| Z4 | 基础镜像 3.23.6、门逻辑、musl pin、openssl、unknown origin 复审期限 2026-11-16 | 不重开 | 保持；复审期限另行处理（第 6 节） |

## 4. 目标与实施约束

- `Dockerfile` 运行时阶段：`RUN apk add --no-cache 'zlib=1.4-r0' 'musl=1.2.5-r24' 'musl-utils=1.2.5-r24'`（仍在 FROM 之后、ARG 之前，
  不用 `--allow-untrusted`、upgrade、额外仓库或 latest），注释同步。
- `scripts/os-apk-baseline.json`：world 中 `zlib=1.4-r0`；`buildTimeFixes.zlib` 改为 1.3.2-r0 → 1.4-r0、
  `[CVE-2026-85091, CVE-2026-4738]`、上述 aports 提交与索引描述；两平台 `zlib` 改为第 2 节的身份。
- 门脚本 `scripts/check-os-advisories.mjs` 不改；不加门例外。

## 5. 测试先行与验证

先行测试（`scripts/check-os-advisories.test.mjs`，在基线与 Dockerfile 更新前失败）：

- 夹具：签名索引记录与真实安装块（`zlib-1.4-r0-apkindex.json`）、2026-11-10 main secdb 子集与 2026-11-07 community secdb 子集
  （`secdb-v3.23-main-subset-2026-11-10.json`、`secdb-v3.23-community-subset-2026-11-07.json`）、对应原生比较
  （`apk-version-native-2026-11-10-zlib.json`，并在 `apk-version-native.json` 加 1.4-r0 对旧修复版本的 5 组比较）。
- 候选镜像改为 zlib 1.4-r0 加 musl r24；上一次发布候选（zlib 1.3.2-r1 加 musl r24）在 11-10 secdb 下必须精确报出两条
  `zlib 1.3.2-r1 < 1.4-r0 CVE-2026-4738`（与 run `42775373137` 的失败相同），并被新基线的身份 pin 拒绝；完整候选在 11-10 secdb
  下通过；运行时 Dockerfile 只允许这一条精确安装。原有用例改用新候选的身份、文件名与比较对，断言不放宽。

验证：OS 门测试与 Go/前端公告门自测；`docker compose config`。候选镜像的实际构建、两平台 OS 门、卷门与可信双平台发布只能在合入 main 后
由 Actions 完成；本地不构建、不推送镜像，生产不操作。

## 6. 明确不重开

- 门设计、基础镜像版本、openssl 3.5.8-r0（secdb 仍无对应修复）。
- unknown origin 复审期限 `reviewUntil` 2026-11-16：到期后每次发布都会失败关闭，需要对 5 个无 secdb 记录的 origin 重新复审，
  另起切片处理（本合同不延期）。

## 7. 状态

**contract-only**（由第 8 节记录取代）。

## 8. 记录（2026-11-10）

提交：合同 `fdee10a`；测试 `db81534`（夹具与 24 个门测试，实现前 10 个红，全部是 zlib 身份 pin 与 Dockerfile 安装行；上一次发布候选在
11-10 secdb 下精确报出与 run `42775373137` 相同的两条待修复项）；构建 `00fff33`（Dockerfile 一行，基线 world、`buildTimeFixes.zlib` 与两平台身份）。

验证：OS 门测试 24/24；Go、前端、OS 公告门自测共 138/138；浏览器 smoke 自测 10/10；`docker compose config` 通过。在固定 base 上执行运行时阶段
同一条安装后，两平台已安装的 16 个包（版本、origin、`C:`）与新基线完全一致。

未验证：含 zlib 1.4-r0 的候选镜像实际构建、两平台 OS 门实际运行、卷门与可信双平台发布，需合入 main 后由 Actions 执行；本地不构建、不推送
镜像。状态 **build-fix-applied / locally-validated / Actions-pending**；镜像仍为 `6006659`；生产未访问。
