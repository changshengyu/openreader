# 发布门新增依赖公告维护合同（P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本切片不涉及上游可见行为）

当前实施基线：`OpenReader@9e77cd9`（认证会话 P0 修复分支，见
[`authenticated-session-legacy-public-secret-fixed-baseline-second-audit-p0-contract.md`](authenticated-session-legacy-public-secret-fixed-baseline-second-audit-p0-contract.md)）；
`origin/main` 为 `a1a1795`。

审查日期：2026-10-22
状态：**implementation-complete / regression-validated / Docker-published**（证据见第 9 节，发布见第 12 节）

前置合同：Go 依赖与工具链见
[`runtime-dependency-advisory-fixed-baseline-second-audit-p2-contract.md`](runtime-dependency-advisory-fixed-baseline-second-audit-p2-contract.md)
第 10–12 节和 [`go-advisory-release-gate-contract.md`](go-advisory-release-gate-contract.md)；前端依赖与全锁审计门见
[`frontend-dependency-advisory-fixed-baseline-second-audit-p2-contract.md`](frontend-dependency-advisory-fixed-baseline-second-audit-p2-contract.md)。
这些合同的已签收结论不重开。

## 1. 范围与结论

认证会话 P0 修复完成后，按 `openreader-regression` 在本地运行与 CI 相同的发布门。上次成功发布是 Actions
`39125862052`（2026-10-18，`238e3f3`）；之后公开了三组新公告，两个失败关闭的门在当前树上都失败：

| 门 | 结果 |
| --- | --- |
| `node scripts/check-go-advisories.mjs` | exit 1：`unreviewed advisory GO-2026-6809` |
| `node scripts/check-frontend-advisories.mjs` | exit 1：`npm audit` 报 2 个 high（`follow-redirects`、`vite`） |

CI 在构建镜像前运行同样的门，所以此时任何合入 main 的提交都不会产生镜像，P0 修复也发不出去。用户在本会话
确认按独立切片一并修复（2026-10-22）。裁决：**must-fix（发布阻塞）**，按第 4 节做最小升级。

## 2. 公告与适用性

| 编号 | 公告 | 当前版本 | 修复版本 | 适用性 |
| --- | --- | --- | --- | --- |
| A1 | [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809)（CVE-2026-111943），`crypto/x509` 不再信任被攻破的中间证书；2026-10-21 公开 | Go 1.26.9 | Go 1.26.10 | **可达**。govulncheck v1.9.0 符号级发现：`crypto/x509 Certificate.Verify` 经 `crypto/tls` 客户端握手可达（发现轨迹之一从 `services/backup` 的读取进入）。对外 HTTPS 请求（例如备份、书源、封面与章节图片抓取）用系统根证书校验服务端证书链；1.26.9 仍会接受经这些中间证书签发的证书链。 |
| A2 | vite：GHSA-698v-4r3x-3vcv、GHSA-r825-w8mm-5pxw、GHSA-587h-7wmj-h5pm（`launch-editor`）、GHSA-9jrq-w75r-8gcw（自定义中间件跨源脚本执行）、GHSA-rq7h-c2jc-7f22（`server.fs.allow` 绕过）；影响 `>=8.0.0 <=8.1.5` | 8.0.16（devDependency，精确固定） | 8.1.6 | 只影响开发服务器（`npm run dev` / `vite preview`）。生产镜像由 Go 二进制服务 `vite build` 的静态产物，不运行 vite。 |
| A3 | follow-redirects：GHSA-q3jh-wrc6-jgq8、GHSA-8r9p-f939-6h3c（跨源跳转时没剥掉凭据请求头）；影响 `>=1.16.0 <1.16.1` | 1.16.0（经 `axios@1.20.0` 引入的生产依赖） | 1.16.1 | axios 的浏览器构建使用 XHR/fetch 适配器，follow-redirects 只服务 Node 端 http 适配器，预期不进入浏览器产物（第 6 节核验）。全锁门不区分运行时可达性，必须升级。 |

`GO-2026-5932`（不再维护的 `crypto/openpgp`）的既有不适用结论及其审查期限不变。

## 3. 差异分类

| 编号 | 分类 |
| --- | --- |
| A1 | **must-fix**：可达的标准库安全修复，无可签收的例外理由 |
| A2 | **must-fix**：只影响开发服务器，但全锁门失败关闭；没有 8.0.x 修复版，最小修复是 8.1 线的安全回补版 8.1.6 |
| A3 | **must-fix**：预期不在浏览器产物里，但全锁门失败关闭 |

不得通过给门加例外、放宽严重度或跳过开发依赖来过门。

## 4. 目标与不变项

### 4.1 Go 1.26.10（A1）

- `backend/go.mod` 的 `go` 指令 `1.26.9` → `1.26.10`；模块依赖与 `go.sum` 不变。CI 的 `setup-go` 读取
  `go-version-file: backend/go.mod`，随之使用 1.26.10。
- `Dockerfile` 后端构建阶段改为官方 `golang:1.26.10-alpine3.23` 的 OCI index
  `sha256:107301490755aa2ea1b0b43788b6e62cff2faae45a538028b193becc06aaffba`（`linux/amd64`
  `sha256:9d1e2c4fda593efb2c405e1c855c75ec17fe42dc02f6b7142a5ea7af7f386bf4`、`linux/arm64/v8`
  `sha256:8c61d9486d6cf1e494c4cdd9704f52ad817938bc7aee2b1de816b46f6712e6e6`；2026-10-21T21:25:10Z 由
  `docker-library/golang@650cf2f338d22da87df8c773cfc88afa26699502` 构建，base `alpine:3.23`）。
- 运行时 `alpine:3.23.6` pin、构建期签名 zlib `1.3.2-r1`、`node:22-alpine3.23` 前端构建阶段、CGO/vendor 构建方式、
  单二进制、挂载目录与环境变量都不变。

### 4.2 前端锁文件（A2、A3）

- `frontend/package.json`：`vite` `8.0.16` → `8.1.6`（继续精确固定）；`overrides` 增加
  `"follow-redirects": "1.16.1"`，与既有 `postcss`/`nanoid`/`source-map-js` 精确 override 同一做法。
- 用 `npm install --package-lock-only` 重新生成 `package-lock.json`；锁文件变化只允许是 vite、follow-redirects
  及 vite 8.1.6 自身声明需要的传递依赖。不升级 axios、vue、element-plus 或其他直接依赖，不批量 latest，
  不选 8.3.x（8.3.5 发布不足两天）。

### 4.3 不变项

- 发布门脚本、其合同测试与 `GO-2026-5932` 例外不改；不新增例外。
- 不改 API、数据、前端源码和界面行为，不改 schema、备份或启动流程。
- README 里的 “Go 1.26.9+” 字样（徽章、源码构建要求、技术栈表）不在本切片修改：README 正由分支
  `claude/readme-user-facing` 整体改写，届时同步为 1.26.10。在此之前，`go` 1.26.9 用默认的
  `GOTOOLCHAIN=auto` 构建源码时会自动下载 1.26.10 工具链。

## 5. 红（既有发布门即红测）

本切片不新增测试代码：两个发布门本身就是已提交、在 CI 中强制执行的失败关闭检查，它们在当前树上的失败
（第 1 节）就是本切片的红；门的精度与失败处理已由 `scripts/check-*-advisories.test.mjs` 覆盖（本地 128/128）。
GO-2026-6809 涉及的具体中间证书不在本仓库可控范围内，不编写证书链行为测试。

## 6. 验证门（实现后）

1. backend 内 `go version` 为 `go1.26.10`；`go test ./...`、`go vet ./...`、`go test -race ./api ./db ./services/...`
   （api 需 `-timeout 40m`）全部通过。
2. 实际 Go 门 exit 0，扫描配置 `go_version=go1.26.10`，不再有 GO-2026-6809 发现，GO-2026-5932 前提照旧成立。
3. 前端 `npm ci`、`npm test`、`npm run build` 通过；实际前端门 exit 0（npm audit 0 漏洞）；构建产物中没有
   follow-redirects（A3 适用性核验）。
4. `docker compose config --quiet`、`node scripts/smoke/store-read-list-lifecycle-contract.mjs` 通过。
5. 合入 main 后，可信 Actions 完整运行双平台候选镜像构建、OS APK 门、fresh / historical 卷门并发布；用
   `gh run view` 与 `docker buildx imagetools inspect` 核验 run 结论、OCI index digest 与两平台。

## 7. 非目标

- 不升级到 vite 8.3.x，不升级 axios 或其他依赖，不改 Node 22 构建镜像。
- 不修改 README（见 4.3）；不处理与本次发布无关的其它开放审计项。

## 8. 补充：vite 8.1.6 生产产物空白页（2026-10-22，实施中发现）

`99a8900`（vite 8.1.6）的 `npm test` 810/810 和 `npm run build` 都通过，但产物在真实浏览器里是空白页。
`scripts/smoke/openreader-smoke.mjs` 对“`6ca7fbf` 二进制 + 新产物”在 1440×900 即失败（`blank page at /`），
对“`a1a1795` 二进制 + 旧产物”三视口都正常渲染 `/login`。Playwright 捕获的页面错误为
`TypeError: e is not a function`（`http-vendor` chunk），Vue 没有挂载。

根因：rolldown 1.0.3（vite 8.0.16）把运行时辅助函数（`__export`、`__toESM` 等）单独输出成无依赖的
`rolldown-runtime` chunk；rolldown 1.1+ 把它们寄放进一个共享的应用 chunk（这里是
`_plugin-vue_export-helper`，其中还有导入 axios 的前端拦截器代码）。于是 `http-vendor` 从该 chunk 导入运行时，
该 chunk 又从 `http-vendor` 导入 axios，形成循环；`http-vendor` 先求值时运行时函数仍是 `undefined`。
`manualChunks` 收不到 `\0rolldown/runtime.js`，无法把运行时单独分包。vite 8.2.4、8.3.5（rolldown 1.2.15）
在同一配置下复现同一错误，所以换修复版本绕不开。

决定（**must-fix**，属于依赖升级的必要适配）：在 `frontend/vite.config.js` 的 `build.rolldownOptions.output`
增加 `strictExecutionOrder: true`（rolldown 文档用来保证模块按导入顺序执行、避免无效 chunk 的选项）。试验
构建中 rolldown 重新输出独立的 `rolldown-runtime` chunk，`http-vendor` 从它导入；产物文件结构与旧构建一致
（63 个文件、同名 chunk），总字节 3,742,107 → 3,800,368（+1.56%，主要是 element-plus/图标的初始化包装），
`/login` 正常渲染。`manualChunks` 分包规则、源码和界面不变。

本节的红就是既有浏览器 smoke 在 `99a8900` 产物上的失败。实现后必须重跑：`npm test`、`npm run build`、
`openreader-smoke.mjs` 三视口，以及登录后进入工作台与阅读器的真实 API 浏览器合同
（`book-management-real-api-contract.mjs`、`reader-progress-multiclient-contract.mjs`）。

观察 O3：CI 只跑前端单元测试和构建，不在浏览器里加载产物；同样的空白页如果合入 main，Actions 会照常发布
镜像。给发布流程补一个“产物能启动”的浏览器门另起后续，不在本切片修改 workflow。后续合同：
[`release-browser-boot-gate-fixed-baseline-second-audit-p1-contract.md`](release-browser-boot-gate-fixed-baseline-second-audit-p1-contract.md)。

## 9. 实施与验证证据（2026-10-22）

### 9.1 提交

| 提交 | 内容 |
| --- | --- |
| `37503a3` | 本合同（第 1–7 节）及两份前置合同、gap-analysis、matrix 的指针 |
| `99a8900` | `vite` 8.0.16 → 8.1.6；`overrides` 增加 `follow-redirects` 1.16.1；锁文件重算 |
| `6ca7fbf` | `go 1.26.10`；后端构建镜像改为官方 `golang:1.26.10-alpine3.23` index pin |
| `a673c61` | 第 8 节：vite 8.1.6 产物空白页的根因与决定 |
| `3351458` | `frontend/vite.config.js` 增加 `strictExecutionOrder: true` |

锁文件变化只有 `vite`、它要求的 `rolldown` 1.1.5 链（`@oxc-project/types` 0.139.0、`@emnapi/core`/`runtime`
1.11.1、`@emnapi/wasi-threads` 1.2.2、各平台 binding）和 `follow-redirects` 1.16.1；`go mod verify` 全部通过，
`go mod tidy` 不改 `go.sum`。

### 9.2 发布门（失败 → 通过）

| 门 | 升级前 | 升级后 |
| --- | --- | --- |
| Go advisory 实际扫描 | exit 1：未审查的 GO-2026-6809 | exit 0：`pass-with-not-applicable`，go1.26.10，符号级，数据库时点 2026-10-21T00:27:29Z，247 条公告，唯一发现仍是既有不适用的 GO-2026-5932 |
| 前端全锁 advisory 实际扫描 | exit 1：2 个 high | exit 0：全锁 132 包 0 发现，lockHash `1a4f72372dafdedcb3c48e04b6c229bb509c4d8556d6c4271061c3d8649e530b` |
| 门的离线合同测试 | 128/128 | 128/128（脚本未改） |

### 9.3 前端

- 本机 Node 26.4.0 / npm 11.17.0：`npm ci`、`npm test` 810/810（0 失败 0 跳过）、`npm run build` 通过。
- 与 Dockerfile 前端阶段同一镜像（`node:22-alpine3.23@sha256:baf676f7…`，Node 22.23.3 / npm 10.9.9），仓库
  布局下 `npm ci`、`npm test` 810/810、`npm run build` 通过，`rolldown-runtime` chunk 哈希与本机构建一致。
- A3 核验：follow-redirects 独有的 `ERR_FR_REDIRECTION_FAILURE`、`ERR_FR_MAX_BODY_LENGTH_EXCEEDED` 在新旧产物里都
  不存在，产物中只有 axios 自己定义的错误码常量和 fetch 适配器文案。
- 第 8 节的红：`99a8900` 产物上 `openreader-smoke.mjs` 在 1440×900 失败；`3351458` 产物三视口通过，与 `a1a1795`
  产物结果相同（`/login`，可见节点 26、文字 33）。
- 真实 API 浏览器合同：`book-management-real-api-contract.mjs`（1440×900、390×844、360×800，元数据编辑、分组、
  批量分类、批量删除）与 `reader-progress-multiclient-contract.mjs`（三视口双客户端 CAS、WebSocket 收敛、冷启动
  恢复、WebDAV 镜像，以及 ingress 三视口）通过。

### 9.4 后端

- backend 内 `go version` 为 go1.26.10；`go test ./...` 29 包通过；`go vet ./...` exit 0。
- `go test -race -timeout 40m ./api ./db ./services/...`（Go 1.26.10，最终 Go 代码）：24 包通过，api 1040.8 s，
  无 DATA RACE，总耗时 17 分 46 秒。
- `docker compose config --quiet`、`node scripts/smoke/store-read-list-lifecycle-contract.mjs` 通过。

### 9.5 状态

本节写于首次合入 main 之前；之后的 CI 失败、补充修复与发布见第 10–12 节。

## 10. 补充：CI 的 OS APK 门拦下复制的 CA 证书包（2026-10-22）

`ae4736d` 合入 main 后，可信 Actions run `39758440810` 的 Go 测试、两项 advisory 门、前端测试与构建、Compose、
存储 HTTP 合同、native 与 arm64 候选镜像构建都通过，但“Check runtime APK packages against official Alpine secdb
on both platforms”失败：`linux/amd64 APK file changed after install: etc/ssl/certs/ca-certificates.crt`，
实际 `Q1mT0WUgIZu3o4++yVm+9ubDFAdCA=`，期望 `Q1reWe8OzoZ6gWJYQl50oNHcmLboc=`。后续卷门和发布步骤被跳过，**没有发布镜像**。

根因：运行时阶段 `COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt …` 用构建镜像里的证书包覆盖了
运行时镜像里由 APK 包 `ca-certificates-bundle` 管理的同一文件。只读检查官方镜像（`--network none`，两平台）：
`alpine:3.23.6` 与旧构建镜像 `golang:1.26.9-alpine3.23` 都是 Alpine 3.23.6、`ca-certificates-bundle 20260909-r0`，
该文件的 APK 记录为 `Z:Q1reWe8O…`；新构建镜像 `golang:1.26.10-alpine3.23` 基于 Alpine 3.23.7、
`ca-certificates-bundle 20261020-r0`，记录为 `Z:Q1mT0WUg…`。SQLite/OS-C 合同已记录“builder 复制的 CA bundle 与包内
字节相同”这一前提，OS 门的测试也专门断言字节不同即失败关闭，这次是门按设计生效。

决定（**must-fix**，属于构建镜像升级的必要适配）：删除这行复制。运行时镜像本来就安装 `ca-certificates-bundle`
20260909-r0，而此前复制进来的文件与它逐字节相同，所以删除后运行时信任库与上次已发布镜像完全一致；不升级运行时
证书包，不改 OS 门脚本、基线或其测试。门的离线测试（含“运行时 Dockerfile 只安装签名 zlib 修复”）不依赖这行。

红就是 run `39758440810` 的 OS 门失败。修复后由下一次可信 Actions 运行双平台候选镜像构建、OS APK 门、fresh /
historical 卷门并发布；本地不构建镜像。

## 11. 补充：OS APK 门报 musl 待修复项（2026-10-22）

删除复制的证书包后，可信 Actions run `39762716947`（`d15c384`）在 OS APK 门之前全部通过，OS 门失败：
`pending Alpine secfix blocks release: linux/amd64 musl 1.2.5-r23 < 1.2.5-r24 CVE-2026-107891`，`musl-utils` 与
`linux/arm64` 同样。卷门和发布被跳过，**没有发布镜像**。v3.23 main secdb（Last-Modified Wed, 21 Oct 2026 18:11:19 GMT）
相对 10-13 快照只新增这一项；运行时其余 14 个包都不低于各自的修复版本。

决定（**must-fix**，与
[`sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md`](sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md)
的 zlib 构建期修复同一做法）：运行时基础镜像仍为固定的 `alpine:3.23.6`，构建期在同一条 `apk add` 中安装签名索引里的精确
`musl=1.2.5-r24` 与 `musl-utils=1.2.5-r24`。不升级基础镜像，不加门例外，不改门脚本逻辑。

证据（固定 `alpine:3.23.6`，两平台，在容器内执行与运行时阶段相同的安装）：

| | x86_64 | aarch64 |
| --- | --- | --- |
| v3.23 main APKINDEX sha256 | `e9e83b0cca8e0b7857cf8dd2c95e04eb27429ddff0619f8260c91db756b1e718` | `6f2b3dbd90d2bd502f811caffd2b0d10f5d1710ef24acbe0d17dfb0fcb62e60e` |
| `apk verify`（镜像自带可信密钥） | OK，`.SIGN.RSA.alpine-devel@lists.alpinelinux.org-6165ee59.rsa.pub` | OK，`…-616ae350.rsa.pub` |
| 索引描述 / Last-Modified | `v3.23.7-2-gddd3ce4d481` / Thu, 22 Oct 2026 07:45:27 GMT | 同描述 / 07:45:32 GMT |
| `musl` 1.2.5-r24 `C:` | `Q1XUI1pl90RATg8vH/H5+0fF2ukao=` | `Q1Gpmv84P2QM7qzmrxAM68h3dvVVQ=` |
| `musl-utils` 1.2.5-r24 `C:` | `Q1rtAC/LK9HesmJYexXp60XOc2Ii8=` | `Q19Wb50/jiOW4Y3ETW/Ke1KcyIHgk=` |

两个包的 aports 提交为 `fd05fc094264bce4fb76efc6c7f61ba4abfcb609`；安装后的数据库身份行与签名索引记录逐字相同，文件清单与
r23 相同；`etc/apk/world` 变为 `musl=1.2.5-r24`、`musl-utils=1.2.5-r24`，其余条目不变。`apk --no-network version -t`
（apk-tools 3.0.8，`--network none`，两平台结果相同）：r24 对全部 7 个 musl 修复版本为 `>` 或 `=`，r23 对 r24 为 `<`。

实施：

- `Dockerfile` 运行时阶段：`RUN apk add --no-cache 'zlib=1.3.2-r1' 'musl=1.2.5-r24' 'musl-utils=1.2.5-r24'`。
- `scripts/os-apk-baseline.json`：world 加两个精确版本，`buildTimeFixes` 增加 musl（r23 → r24、CVE-2026-107891、上述
  aports 提交与索引描述），两平台 `musl`/`musl-utils` 改为上表身份。
- 门测试先行：夹具加入签名索引记录与真实安装块（`musl-1.2.5-r24-apkindex.json`）、10-21 secdb 子集
  （`secdb-v3.23-main-subset-2026-10-21.json`）及对应原生比较；候选镜像包含两项构建期修复；“只有 zlib 修复”的镜像
  在 10-21 secdb 下必须精确报出 4 条 musl 待修复项，完整候选在 10-21 secdb 下通过；运行时 Dockerfile 只允许这一条
  精确安装。这些测试在基线与 Dockerfile 更新前失败。

红就是 run `39762716947` 的 OS 门失败与上述先行测试。修复后由下一次可信 Actions 完成双平台构建、OS 门、卷门与发布。

## 12. 发布（2026-10-22）

| 可信 Actions run | 提交 | 结论 |
| --- | --- | --- |
| `39758440810` | `ae4736d` | 失败：OS APK 门，复制的构建镜像证书包（第 10 节）；未发布 |
| `39762716947` | `d15c384` | 失败：OS APK 门，musl 待修复项（第 11 节）；未发布 |
| `39768628680` | `92c98a9` | 成功：Go 测试、Go/前端 advisory 门、OS 门测试、前端测试与构建、Compose、存储 HTTP 合同、native 与 arm64 候选镜像、OS APK 门、fresh 卷与 portable 备份、历史卷兼容、发布、平台核验 |

补充提交：`2acff2e` / `d15c384`（不再复制构建镜像的证书包）、`b3f8ef3` / `7d81858` / `92c98a9`（musl 构建期修复：
合同、先行门测试、实现）。门测试 131/131（基线更新前 9 条按预期失败），`docker compose config` 通过。

镜像：`ghcr.io/changshengyu/openreader:92c98a9` 与 `latest` 为同一 OCI index
`sha256:3b29f501e659454ea096fddcc4e0a05ba3c881cccacf8c3a684218649ba218d2`，`linux/amd64`
`sha256:f102c6af2681bd1b3ad97fd698100df297f3866dbc0e5429e27a2ebb18a6d7f0`、`linux/arm64`
`sha256:7cfba03369e47c35660bae197dae2307d61747320b0def443fab2987cfc99e17`；两平台 revision 标签为
`92c98a971dce5c9166c0426695f89cd75f37cf94`，创建于 2026-10-22T11:10:55Z。

状态：

- Git：`main` 与分支 `claude/auth-legacy-public-secret-forgery` 在 `92c98a9`（`git ls-remote` 核验）。
- 镜像：已发布，见上。
- 生产：未访问，运行版本未知。
- 后续：O3（发布流程补浏览器启动门）。README 的 Go 版本字样已由 README 改写（`9f94117`）更新为 1.26.10。
