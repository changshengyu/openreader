# 发布流程浏览器启动门合同（P1，O3）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本切片不涉及上游可见行为）

当前实施基线：`origin/main` 为 `b1e84f3`（应用代码与已发布的 `92c98a9` 相同）。

审查日期：2026-10-22
状态：**implementation-complete / regression-validated**（证据见第 8 节；尚未经可信 Actions 运行）

来源：[`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)
第 8 节的观察 O3。该合同已签收的结论（vite 8.1.6、`strictExecutionOrder: true` 等）不重开。

## 1. 范围与结论

2026-10-22 把 vite 8.0.16 升到 8.1.6 后，`npm test` 810/810 和 `vite build` 都通过，产物在浏览器里却是空白页：
rolldown 1.1+ 把运行时辅助函数放进一个共享应用 chunk，该 chunk 又从 `http-vendor` 导入 axios，`http-vendor` 先求值时
抛出 `e is not a function`，Vue 没有挂载。这次只是在本地手动运行 `scripts/smoke/openreader-smoke.mjs` 时才发现，修复
（`strictExecutionOrder: true`）随 `92c98a9` 发布。

可信发布流程 `.github/workflows/docker-publish.yml` 依次运行 Go 测试、Go/前端 advisory 门、前端单元测试与构建、Compose、
存储 HTTP 合同、native 与 arm64 候选镜像、OS APK 门、fresh/historical 卷门，然后发布。没有一步在浏览器里加载产物；卷门
只请求 `/api/*` 和资源地址，从不请求 `/`。同样的空白页如果合入 main，所有门都会通过，`latest` 会被换成打不开的版本
（标准 Compose 是 `pull_policy: always`）。

裁决：**must-fix（P1，发布流程缺口）**。在发布前加一个失败关闭的浏览器启动门，检查逻辑复用既有 checker，不另写。

## 2. 现状

### 2.1 发布流程

- 前端在发布流程里只有 `npm test`（`docker-publish.yml` 第 96 行）和 `npm run build`（第 100 行）。runner 上的
  `frontend/dist` 只被存储 HTTP 合同（第 107 行）当作 `OPENREADER_PUBLIC_DIR`，而该合同只发 HTTP 请求。
- 镜像里的产物不是 runner 上的这份：Dockerfile 前端阶段 `FROM --platform=$BUILDPLATFORM node:22-alpine3.23@sha256:baf676f7…`
  在构建平台上重新 `npm ci && npm run build`，再 `COPY --from=frontend-builder /src/frontend/dist /app/public`；
  `.dockerignore` 排除 `frontend/dist`。两个平台的镜像拷贝的是同一个构建平台阶段的产物。
- 服务端启动时 `public` 下没有 `index.html` 就不注册前端路由（`backend/main.go` 第 186–189 行），`/` 返回 404。
- 耗时（run `39768628680`）：native 候选镜像 121 s，arm64 候选镜像（QEMU）962 s，全流程约 26 分钟。

### 2.2 既有 checker

`scripts/smoke/openreader-smoke.mjs`：

- `collectPageSignals`（第 14–30 行）：`pageerror`、`console.error`、`/api/` 的 5xx 响应都记为失败。
- `assertAppNotBlank`（第 32–54 行）：`networkidle` 之后等 `domcontentloaded` 再等 800 ms；要求存在 `#app`，并要求
  **整个 body** 有文字或可见元素超过 3 个，否则报 `blank page at <path>`。
- `smokeViewport`（第 56–65 行）：先断言非空白，再断言没有收集到失败。
- `main`：依次检查 1440×900、390×844、360×800；`TARGET_URL` 默认 `http://127.0.0.1:8080`；可选 `SMOKE_READER_URL`。

`scripts/smoke/playwright-runtime.mjs` 默认启动 Playwright 的 Chromium Headless Shell；`CDP_URL`、`CHROME_PATH` 是显式覆盖。

本地复现（撰写本合同时：`b1e84f3` 树、Go 1.26.10 本地二进制、全新数据目录、Playwright 1.61.1 / headless shell
149.0.7827.55）：

| 产物 | 结果 |
| --- | --- |
| 当前树 `npm run build` | 三视口跳到 `/login`，可见节点 26、文字 33，没有失败；除 `#app` 本身外，所有可见元素都在 `#app` 内，`#app` 带 `data-v-app` |
| 删除 `strictExecutionOrder: true` 后的构建（没有 `rolldown-runtime` chunk，与 2026-10-22 的产物结构相同） | 三视口停在 `/`：`#app` 为空、没有 `data-v-app`，body 没有文字和可见元素，`pageerror: e is not a function`；checker 在 1440×900 报 `blank page at /` 退出 |

### 2.3 checker 与门的要求之间的差距

- C1：空白判断统计整个 body，而不是 `#app`。`index.html` 的 body 目前只有空的 `#app`，所以现在两者等价；但只要有内容
  渲染在 `#app` 之外（Element Plus 传送到 body 的弹层或消息、静态标记），Vue 没挂载也会判为通过。门的要求是“`#app`
  为空即失败”。
- C2：空白断言在失败列表断言之前，空白时报告里没有 `pageerror`。上述复现只输出 `blank page at /`，看不到根因
  `e is not a function`；而发布门失败时，CI 日志是唯一的现场。
- C3：报告里没有视口尺寸。

### 2.4 浏览器的版本固定方式

- 根 `package.json` 精确固定 `playwright` 为 `1.61.1`；`package-lock.json` 里 `playwright` 与 `playwright-core` 1.61.1
  都带 `sha512` integrity。
- `npm run smoke:install-browser` 即 `playwright install chromium-headless-shell`。下载哪个版本由锁定的
  `playwright-core/browsers.json` 决定：`chromium-headless-shell` revision `1228`（Chrome 149.0.7827.55）。
- Playwright 1.61.1 下载浏览器时不做逐文件哈希校验，只按 revision 从官方 CDN 经 HTTPS 下载。仓库对浏览器的固定就是
  “锁文件固定 Playwright，Playwright 固定 revision”；发布流程沿用同一方式。

## 3. 差异分类

| 编号 | 差异 | 分类 |
| --- | --- | --- |
| G1 | 发布流程不在浏览器里加载将要发布的产物 | **must-fix**（O3 本身） |
| G2 | checker 不按 `#app` 判断空白（C1） | **must-fix**：门的要求是 `#app` 为空即失败 |
| G3 | 空白失败时不报告页面错误和视口（C2、C3） | **must-fix**：失败关闭的门必须在日志里给出原因 |
| G4 | 只在 native（amd64）候选镜像上加载 | acceptable-change：两平台拷贝同一个构建平台阶段的产物，浏览器端结果与服务端 CPU 架构无关；在 QEMU 下再跑一次只增加时间 |
| G5 | 只检查全新实例的入口（`/` → `/login`） | acceptable-change：这是“产物能启动、Vue 能挂载”的启动门；登录后的工作台、阅读器真实 API 浏览器合同仍按 `openreader-regression` 在本地运行 |

## 4. 目标

### 4.1 门脚本 `scripts/smoke/release-browser-smoke.sh`

- 输入 `IMAGE`（必填），用 `docker run -d` 起一个全新容器：不挂卷，`/app/data`、`/app/cache`、`/app/library` 是镜像里
  新建的空目录；只额外设置一次性 `OPENREADER_JWT_SECRET`；端口只绑定 `127.0.0.1` 且由 Docker 分配，健康检查只能到达
  这个容器。
- 最多等 60 s `/api/health`，不健康就打印容器日志并失败。
- 用锁定的 headless shell 运行 `openreader-smoke.mjs`，`TARGET_URL` 指向这个容器；清除 `CDP_URL`、`CHROME_PATH`、
  `SMOKE_READER_URL`，保证用的是锁定的浏览器，且只检查入口。
- checker 失败时打印容器日志并非零退出；任何一步失败都非零退出；退出时删除容器。
- `SMOKE_PUBLIC_DIR` 只用于本地：把一个本地 `frontend/dist` 只读挂到 `/app/public`，不在本地构建镜像也能用发布镜像的
  二进制检查本地构建。发布流程不设置它（接线测试断言）。

### 4.2 checker

- `#app` 内没有文字、可见元素也不超过 3 个时报 `empty #app at <path>`，取代按 body 统计的判断；阈值与原来相同。
- 任一视口失败时，错误信息以视口名和尺寸开头，并附上该页收集到的全部 `pageerror`、`console.error`、`api 5xx`。
- 通过行写出视口尺寸。视口、等待方式、`/api/` 5xx 规则和 `SMOKE_READER_URL` 不变。

### 4.3 发布流程

- 在 “Validate isolated storage read-list and import HTTP contracts” 之后、QEMU/Buildx 之前增加：
  - `Install pinned smoke browser`：仓库根 `npm ci && npm run smoke:install-browser`；
  - `Test browser smoke gate precision and wiring`：`node --test scripts/smoke/release-browser-smoke.test.mjs`。
- 紧接 “Build native validation image” 之后、“Build arm64 validation image” 之前增加门本身：以
  `IMAGE=openreader-ci:${{ github.sha }}` 运行门脚本，步骤超时 10 分钟。空白产物因此在约 16 分钟的 arm64 构建之前失败，
  其后的 OS 门、卷门、GHCR 登录和发布都被跳过。
- 不加 `continue-on-error`，不加跳过条件。
- `paths` 增加 `package.json`、`package-lock.json`、`scripts/smoke/openreader-smoke.mjs`、`scripts/smoke/playwright-runtime.mjs`、
  `scripts/smoke/release-browser-smoke.sh`、`scripts/smoke/release-browser-smoke.test.mjs`。
- 预计总共增加约 1 分钟（根依赖与浏览器下载、门测试、一次容器启动和三视口加载）。

### 4.4 不变项

- 应用代码、`frontend/vite.config.js`、Dockerfile、`.dockerignore`、镜像内容、其它门及其测试都不变。
- 不缓存浏览器，不加 `--with-deps`：ubuntu-latest 已带 Chromium 的系统依赖；缺依赖时 Playwright 启动即失败，门同样
  失败关闭。
- 本地不构建镜像，不访问生产。

## 5. 红

- R1 接线：`release-browser-smoke.test.mjs` 的接线测试在当前 workflow 上失败（没有浏览器安装、门测试和门步骤，`paths`
  不含门文件）。锁文件固定测试是对照，当前就通过。
- R2 checker 精度（测试自己起 loopback 页面，以子进程运行与门相同的 checker）：`#app` 为空而 `#app` 外有文字时，当前
  checker 通过（应当失败）；挂载前抛错时，当前 checker 虽然失败，报告里却没有 `pageerror`。对照：正常页三视口通过；
  `console.error`、`/api/` 500、缺 `#app` 都失败并给出原因。
- R3 行为：门脚本随测试提交（此时 workflow 尚未接线），本地拉取（不构建）已发布的 `92c98a9` 镜像：挂上删除
  `strictExecutionOrder: true` 的本地构建时失败；镜像自带产物、当前树构建都通过。

## 6. 验证门（实现后）

- `node --test scripts/smoke/release-browser-smoke.test.mjs` 全部通过；三项 advisory 门测试（同样读取 workflow）仍通过。
- 门脚本在 R3 三种产物上的结果符合预期，失败信息包含 `empty #app` 与 `pageerror: e is not a function`。
- 前端 `npm test`、`npm run build`；`docker compose config --quiet`；workflow 能被 YAML 解析。
- 本切片不改 Go 代码：Go 全量、vet 与 race 由可信 Actions 的 “Test backend” 运行，本地不重复。
- 可信 Actions：新步骤运行并通过，镜像发布，按 `openreader-docker-release` 核验 digest 与平台。

## 7. 非目标

- 发布流程不运行登录后的真实 API 浏览器合同、Reader 或 iPad 视口。
- 不在 arm64 候选镜像上运行浏览器。
- 不升级 Playwright，不改其它 smoke 脚本或卷门。
- 不处理 S2 等其它开放项。

## 8. 实施与验证证据（2026-10-23）

### 8.1 提交

| 提交 | 内容 |
| --- | --- |
| `b41b33f` | 本合同第 1–7 节，gap-analysis、matrix 与 O3 来源合同的指针 |
| `2453d01` | 门脚本 `release-browser-smoke.sh` 与 `release-browser-smoke.test.mjs`（接线、锁文件固定、门脚本形态、checker 精度） |
| `501eb58` | checker：按 `#app` 判空白，失败信息带视口与页面错误，通过行带视口尺寸 |
| `06800f0` | 测试用的 loopback 服务改为模块加载时监听（不用没有先例的顶层 `before`），断言不变 |
| `a83264f` | workflow：浏览器安装、门测试、门步骤与 `paths`；`docs/regression-smoke.md` |

### 8.2 红与转绿（R1、R2）

`node --test scripts/smoke/release-browser-smoke.test.mjs`，本机 Node 26.4.0，Playwright 1.61.1 / headless shell
149.0.7827.55：

| 用例 | `2453d01`（旧 checker、旧 workflow） | `501eb58` | `a83264f` |
| --- | --- | --- | --- |
| workflow 接线 | 失败：`paths` 缺 `package.json`（第一个缺项） | 失败 | 通过 |
| 锁文件固定 | 通过（对照） | 通过 | 通过 |
| 门脚本形态 | 通过（对照） | 通过 | 通过 |
| 正常页三视口通过 | 通过（对照） | 通过 | 通过 |
| 通过行带视口尺寸 | 失败：`desktop: ok /rendered/ …` | 通过 | 通过 |
| 挂载前抛错 | 失败：只有 `blank page at /throws/`，没有 `pageerror` | 通过 | 通过 |
| `#app` 为空、外面有文字 | 失败：三视口都通过（`visibleNodes=1 text=32`） | 通过 | 通过 |
| `console.error`、`/api/` 500、缺 `#app` | 通过（对照） | 通过 | 通过 |
| 合计 | 6/10 | 9/10 | 10/10（约 13 s） |

### 8.3 门脚本行为（R3）

本地拉取（不构建）已发布的 `ghcr.io/changshengyu/openreader:92c98a9`（index
`sha256:3b29f501e659454ea096fddcc4e0a05ba3c881cccacf8c3a684218649ba218d2`，本机取 arm64，revision
`92c98a971dce5c9166c0426695f89cd75f37cf94`），以 `IMAGE=<该镜像>` 运行门脚本：

| 产物 | 旧 checker（`2453d01`） | 新 checker（`501eb58` 起） |
| --- | --- | --- |
| 镜像自带产物 | exit 0，7 s；三视口 `/login`，可见节点 26、文字 33 | exit 0，6 s；`desktop 1440x900: ok /login …` 等三行 |
| `SMOKE_PUBLIC_DIR`=当前树构建 | exit 0，6 s | exit 0，6 s |
| `SMOKE_PUBLIC_DIR`=删除 `strictExecutionOrder: true` 的构建 | exit 1，3 s：`blank page at /` 与容器日志 | exit 1，4 s：`desktop 1440x900: empty #app at /`、`pageerror: e is not a function` 与容器日志 |

健康检查返回镜像自身的 commit；每次结束后没有残留容器。门脚本在 `dash`（Ubuntu 的 `/bin/sh`）下同样通过，
`sh -n` / `dash -n` 语法检查通过。删除 `strictExecutionOrder: true` 的构建在 `frontend/vite.config.js` 临时改动后输出到
草稿目录，随即 `git checkout` 恢复；其产物没有 `rolldown-runtime` chunk，与 2026-10-22 的空白产物结构相同。

### 8.4 回归

| 门 | 结果 |
| --- | --- |
| 门测试 `release-browser-smoke.test.mjs` | 10/10 |
| 三项 advisory 门测试（同样读取 workflow） | 131/131 |
| Go advisory 实际扫描 | `pass-with-not-applicable`，go1.26.10，符号级，数据库 2026-10-21T00:27:29Z，247 条，唯一发现仍是既有不适用的 GO-2026-5932（复审期 2026-11-16） |
| 前端全锁 advisory 实际扫描（`CI=true`） | `pass`，132 包 0 发现，lockHash `1a4f72372dafdedcb3c48e04b6c229bb509c4d8556d6c4271061c3d8649e530b`（与上次发布相同） |
| 前端 `npm test` / `npm run build` | 810/810；构建通过，产物含 `rolldown-runtime` chunk |
| `docker compose config --quiet`、存储 HTTP 合同 | 通过 |
| workflow YAML 解析 | 29 步；新步骤为第 17、18、22 步，顺序符合 4.3 |
| Go full / vet / race | 未在本地重跑：本切片不改 Go 代码；可信 Actions 的 “Test backend” 在发布前运行 |

无效的首次尝试（均已重跑，不计为通过）：沙箱内 `npm test` 806/810（沙箱禁止 Vite 测试服务监听本地端口）；沙箱外
一次 `npm test` 中 `dependency advisory actual worker: source-map-offset-negative` 停顿 1066 s 后被取消（该用例自身超时
12 s，说明是进程整体停顿而非断言失败），单独运行 26/26、整套重跑 810/810；同一时段 Go 扫描报 `tool timeout`，重跑 11 s
通过；前端扫描首次因本机 npm 11.17.0 在 stderr 打印升级提示被门拒绝，按 CI 的 `CI=true`（关闭 npm 升级提示）重跑通过。

### 8.5 状态

- Git：分支 `claude/peaceful-burnell-ca0ef7` 在本记录提交；`origin/main` 仍为 `b1e84f3`。
- 镜像：未发布；新门尚未在可信 Actions 上运行。
- 生产：未访问，运行版本未知。
