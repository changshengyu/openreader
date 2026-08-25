# 前端静态资源与 SPA 路由边界固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前实现基线：`OpenReader@acaae61`  
审查日期：2026-08-25  
状态：**inventory-complete / implementation-pending**

## 1. 范围

本轮只覆盖单容器入口中 API/WebDAV/WebSocket、构建后的前端文件和 Vue history 路由之间的失败
分流：

- `OPENREADER_PUBLIC_DIR` 中的 `index.html`、`assets/` 和根级公开文件；
- 浏览器直接打开或刷新 OpenReader 已注册前端路由；
- 未知服务端路径、缺失静态文件和已注册路径的错误 HTTP 方法；
- 404/405 的状态、Content-Type、`Allow` 和无敏感信息错误体。

各 API handler、认证、WebDAV 协议、WebSocket 消息、上传/capability 文件、CORS、限流、数据库、
`data/cache/library` 和备份格式均不重开。

## 2. 固定上游与技术栈适配

固定上游 `YueduApi.kt` 先注册 `router.route("/*")` 的 Vert.x `StaticHandler("web")`，再注册
`/assets/*`、`/epub/*` 和业务动作。该 handler 只投影真实静态文件；文件不存在时继续路由，最终由
Vert.x 返回失败状态。上游 Vue Router 保持默认 hash mode，`mode: "history"` 被注释，因此不存在把任意
未知服务端路径改写成 `index.html` 的合同。

OpenReader 使用 Vue Router `createWebHistory()`，已发布的 `/login`、工作台兼容路由、BookInfo、Reader
和临时 Reader 必须支持直接打开和刷新。这是 Vue 3 技术栈适配，允许 Go 仅对**已注册的前端路由**返回
`index.html`；它不授权把 API、静态资源或任意未知 URL 伪装成成功页面。

## 3. 当前反例与裁决

`backend/main.go#serveFrontend` 只挂载 `/assets`，随后以无条件 `router.NoRoute(c.File(indexPath))`
处理所有剩余请求。2026-08-25 对 `OpenReader@acaae61` 的真实二进制探针得到：

| 请求 | 当前结果 | 裁决 |
|---|---|---|
| `GET /api/does-not-exist`, `Accept: application/json` | `200 text/html`，返回完整 `index.html` | **must-fix**：未知 API 为稳定 JSON 404。 |
| `PATCH /api/health` | `200 text/html` | **must-fix**：已注册路径的错误方法为 JSON 405，并保留 Gin 计算出的 `Allow`。 |
| `GET /ws/does-not-exist` | `200 text/html` | **must-fix**：未知 WebSocket/server namespace 为 JSON 404。 |
| `GET /assets/does-not-exist.js` | `200 text/html` | **must-fix**：缺失静态文件为 404，不能让浏览器把 HTML 当 JS。 |
| `GET /manifest.webmanifest`, `/openreader.svg` | 仅能落入 `NoRoute`，返回 `index.html` | **must-fix**：真实根级构建文件必须按文件 bytes/MIME 服务。 |
| `GET|HEAD /books/42/read` | `200 text/html` | **must-preserve**：已知 Vue history 路由直接打开/刷新继续工作。 |
| `POST /books/42/read` | `200 text/html` | **must-fix**：SPA 回退只接受 GET/HEAD。 |
| `GET /no-such-page` | `200 text/html`，Vue 无匹配 route | **must-fix**：任意未知页面不产生假 200。 |

`/webdav/*` 已由已注册 wildcard handler 在认证层返回 DAV `401`，没有进入 `NoRoute`；本轮必须证明启用
方法识别后仍保留 WebDAV 自己的 OPTIONS/认证/405 语义，不能把 DAV 请求改成通用 JSON 或 SPA 页面。

## 4. 目标 HTTP 合同

1. 当 `index.html` 不存在时，`serveFrontend` 不注册静态文件或 SPA fallback，启动不创建或修改 public
   目录；第 6、7 条统一安全 404/405 仍须注册，不能让 API 错误语义依赖前端构建目录是否完整。
2. `GET /` 与 `HEAD /` 返回真实 `index.html`。只有下列 Vue history 路由可回退到同一文件：
   `/login`、`/search`、`/discover`、`/local-store`、`/sources`、`/source-debug`、
   `/bookSourceDebug`（可带末尾 `/`）、`/settings`、`/books/:id`、`/books/:id/read`、
   `/reader/remote/:sessionId`。匹配按完整 path segment 进行；空动态段、多余后缀和近似前缀不匹配。
3. SPA 回退仅接受 GET/HEAD，忽略 query 且保留现有前端 query 状态；HEAD 返回与 GET 相同的状态和
   headers 但没有 body。已知路由无需依赖 `Accept`，以保留浏览器、WebView 和旧客户端直接打开行为。
4. `OPENREADER_PUBLIC_DIR` 根目录内真实普通文件可由 GET/HEAD 读取，至少包括当前 Vite 输出
   `manifest.webmanifest` 和 `openreader.svg`。不得列目录，不得读取子目录（`assets/` 由独立 handler
   服务）、symlink、FIFO/socket/device 或 public root 外路径；URL traversal、反斜杠、NUL 和目录请求
   统一失败。响应使用同一已验证打开句柄，避免验证后按 path 重开。
5. `/assets/*` 的现有真实文件 GET/HEAD/缓存/Range 语义保持；missing、目录和不安全对象为 404，绝不
   回退 `index.html`。`/uploads/*` 和 `/api/*-resource` 继续由各自已签收 handler/capability 合同处理。
6. 无匹配 route 的请求返回 `404 application/json`：
   `{"error":{"code":"NOT_FOUND","message":"route not found"}}`。响应不得包含 public/data/cache/
   library 物理路径、请求 query、header/body、JWT、capability、WebDAV credential 或 OS error。
7. Gin 开启 method-not-allowed 识别。路径在其它 HTTP method 已注册时返回 `405 application/json`：
   `{"error":{"code":"METHOD_NOT_ALLOWED","message":"method not allowed"}}`，并保留 Gin 基于真实 route
   tree 计算的 `Allow`。错误方法不得执行目标 handler、认证后业务、body parse、文件或数据库副作用。
8. CORS、access log query 脱敏、可信代理 client IP 和 rate limiter 继续包裹 404/405；OPTIONS 的现有
   非 WebDAV 204 与 WebDAV protocol handler 优先级不变。

## 5. 测试先行门

1. 在旧实现上新增 `serveFrontend` httptest 合同，锁定根 index、全部已知静态 history 路由、动态段、
   query 和 GET/HEAD；旧实现须先在根级文件、未知页面、未知 API/WS、missing asset 和非 GET/HEAD 上失败。
2. 加入真实 `manifest.webmanifest`/SVG MIME 与 bytes，missing/root directory/traversal/backslash/NUL、
   symlink、directory、FIFO（平台支持时）和同句柄替换测试；不得只断言 `os.Stat`。
3. 启用 405 后覆盖 `/api/health`、一个认证 API、WebSocket、assets、SPA route 及 WebDAV 方法矩阵；断言
   `Allow`、JSON envelope、handler 未运行和 OPTIONS 优先级。
4. 真实 Go 二进制复跑本合同中的 200/404/405/MIME/HEAD 反例；生产构建后用 Chromium 在桌面、手机和
   iPad 视口直接打开 `/login`、`/books/:id/read` 和旧 `/bookSourceDebug/`，确认应用可启动且资源无
   HTML-as-JS/MIME/console 错误。
5. focused、focused race、`go vet ./...`、Go 全量、frontend 全量/build、Compose、fresh/historical/
   portable/restart 卷门和受信 GitHub Actions 双架构发布全部通过后，才将状态改为 implemented/published。

## 6. 数据、兼容和安全边界

- 不新增 schema、migration、目录、环境变量、API 路径、前端路由、浏览器存储 key 或备份 member；
  `data/cache/library` 不扫描、不移动、不重写。
- Vue history route allowlist 是 OpenReader 技术栈适配；固定上游 hash route 与静态文件不存在时的失败
  语义仍是产品依据。新增前端 route 时必须同步更新服务端 matcher 和合同测试。
- 根级 public 文件来自部署者配置的受信构建目录，但请求 path 仍不可信；必须 rooted、普通文件、
  same-file 服务。未知 route 的统一 JSON 是 OpenReader API 形状适配，不暴露 route/文件存在性细节。
- 本轮不把静态 public 文件变成用户上传面，不替代 `/uploads` ownership、chapter/cover capability 或
  WebDAV 私有根；也不更改 CORS 默认值和反向代理部署合同。

## 7. 实施顺序

1. 独立提交本合同及 matrix/REST inventory，不改应用或测试。
2. 在 `acaae61` 旧实现上提交 frontend/static/router 红测，保存旧实现失败证据。
3. 提取窄作用域 route matcher、rooted root-file opener 和统一 404/405 handler；不改业务 route 注册。
4. 完成专项/race/full/runtime/browser/container/卷门后提交推送，由受信 GitHub Actions 本地 runner 构建
   并发布 amd64/arm64 镜像，再补发布证据。
