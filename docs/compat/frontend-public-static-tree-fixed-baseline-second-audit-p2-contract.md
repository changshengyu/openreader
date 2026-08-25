# 前端 public 静态子树固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前实现基线：`OpenReader@a92f8d6`  
审查日期：2026-08-25  
状态：**inventory-complete / implementation-pending**

## 1. 范围

本轮只补齐单容器中 Vite `public/` 子目录的读取边界：

- 当前阅读方案纹理 `frontend/public/themes/*`；
- 当前自定义背景图库 `frontend/public/bg/*`；
- 后续由受信前端构建放入 `OPENREADER_PUBLIC_DIR` 的普通静态子目录文件；
- GET/HEAD、MIME、Range/conditional request、缺失对象与文件系统安全语义。

上一轮 [`frontend-static-spa-route-boundary-fixed-baseline-second-audit-p2-contract.md`](frontend-static-spa-route-boundary-fixed-baseline-second-audit-p2-contract.md)
已经关闭 `/assets/*`、根级 manifest/icon、Vue history fallback 和统一 404/405。本轮不重开这些已发布
行为，也不修改 API、WebDAV、WebSocket、uploads/capability、数据库或持久目录。

## 2. 固定上游合同

固定上游 `YueduApi.kt` 在业务动作前注册 `router.route("/*")` 的 Vert.x
`StaticHandler.create("web")`。它服务打包 web 根下的真实静态文件；不存在的文件继续进入失败语义，
不会改写成首页。

上游 `web/src/plugins/config.js` 把 `body_0...6`、`content_0...6` 和 `popup_0...5` 作为 Reader 方案
资源导入；`web/public/bg/` 又包含固定背景图库。OpenReader 的 Vue 3 适配把同一资源复制到
`frontend/public/themes/` 和 `frontend/public/bg/`，并在 `stores/reader.js` 中以绝对 URL
`/themes/*.png`、`/bg/*.jpg` 引用。Vite production build 会原样输出为 `dist/themes/` 和 `dist/bg/`。
因此这些 URL 返回图片 bytes 是 Reader 外观合同，不是可选部署附件。

## 3. 当前反例与裁决

`OpenReader@a92f8d6` 的 `openFrontendFile()` 已能安全打开多段相对路径，但 `serveFrontend()` 的
`NoRoute` 只尝试已知 Vue route 和 `frontendRootFilename()`；后者拒绝包含 `/` 的文件名。因此构建
产物存在，HTTP 入口却无法到达。

2026-08-25 使用当前 `frontend/dist` 启动真实 Go 服务后得到：

| 请求 | 当前结果 | 裁决 |
|---|---|---|
| `GET /themes/content_0.png` | `404 application/json`，`route not found` | **must-fix**：返回真实 PNG bytes 与 `image/png`。 |
| `GET /bg/山水画.jpg` | `404 application/json`，`route not found` | **must-fix**：UTF-8/percent-encoded URL 均返回真实 JPEG bytes 与 `image/jpeg`。 |
| `GET /manifest.webmanifest` | `200 application/manifest+json` | **must-preserve**：根级文件合同保持。 |
| `GET /assets/<hash>.js` | 已由显式 handler 服务 | **must-preserve**：不改变 Vite hashed assets 路由和 404/405。 |

结果解释了为什么此前只直接打开登录、Reader route 或检查 JS asset 的 smoke 没有证明阅读纹理已加载：
CSS `background-image` 的请求可以失败而 Vue 应用本身仍成功挂载。

## 4. 目标 HTTP 合同

1. 当 `index.html` 存在时，GET/HEAD 可读取 `OPENREADER_PUBLIC_DIR` 下任意真实普通文件，包括多级
   子目录和 UTF-8 文件名；请求 query 不参与文件定位。
2. 文件必须通过已有 rooted、逐组件 `Lstat`、symlink 拒绝、普通文件和 same-file opened-handle
   检查；响应必须从同一已验证句柄发送，不能验证后按 path 重开。
3. 目录、missing、root/ancestor/entry symlink、FIFO/socket/device、NUL、反斜杠、`.`/`..`、编码后
   traversal 和 public root 外对象统一返回安全 JSON 404；不得列目录或泄漏物理路径/OS error。
4. 保留 `http.ServeContent` 的 GET/HEAD、`Content-Length`、`Last-Modified`、conditional request、Range
   和 MIME 推断。PNG/JPEG 必须分别得到 `image/png`/`image/jpeg`；HEAD 不返回 body。
5. 已注册服务端 namespace 优先于 public 文件。未知 `/api/*`、`/ws/*`、`/webdav/*`、
   `/reader3/webdav/*`、`/uploads/*` 和 `/assets/*` 继续由其现有 route/404/405 合同处理，即使受信构建
   目录意外含同名文件，也不能改变 API、认证或 capability 语义。
6. 已知 Vue history route 仍优先返回 `index.html`，不得被同名 public 文件替换；不存在的普通页面仍
   为 JSON 404。静态 public fallback 只接受 GET/HEAD；其它 method 不读取文件。
7. 当 `index.html` 缺失时，不注册 public 文件服务，也不创建/扫描/修复构建目录，保持上一轮启动边界。
8. public 目录是部署者提供的受信前端构建产物；本轮不会把它变成上传面。用户可写资源仍只能走
   `/uploads`、chapter/cover capability、WebDAV 和各自 ownership 合同。

## 5. 测试先行门

1. 先在旧实现添加 GET/HEAD `themes/content_0.png`、UTF-8 与 percent-encoded `bg/山水画.jpg` 合同，
   保存其 404 红灯；同时保留根级 manifest、hashed asset 和全部 history route 绿灯。
2. 覆盖 MIME、bytes、HEAD、Range、If-Modified-Since，以及嵌套普通文件；不能只断言状态码。
3. 覆盖 missing、目录、traversal、反斜杠、NUL、root/ancestor/entry symlink、路径替换后的同句柄 bytes
   和平台支持时的 FIFO；错误体不得含 public root、目标或文件内容。
4. 在 fixture 中放入 `api/health`、`ws/sync`、`uploads/example`、`assets/example` 与已知 history route
   同名文件，证明真实路由/namespace 优先级不变。
5. production build 后启动真实 Go 服务，读取至少一张 theme PNG 和含中文名称的 bg JPEG；再用
   Chromium 在 1440x900、390x844、1024x1366 打开 Reader，断言方案纹理和背景图片请求为 200、
   MIME 正确、可解码，且 console/network 无静态资源失败。
6. focused、focused race、`go vet ./...`、Go 全量、frontend 全量/build、Compose、fresh/historical/
   portable/restart 卷门和双架构镜像核验全部通过后才更新为 published。

## 6. 数据、兼容与允许差异

- 不新增 schema、migration、环境变量、API 路径、前端 route、浏览器存储 key、备份 member 或持久文件。
- 不扫描、不移动、不重写 `data/`、`cache/`、`library/`；只读现有 production frontend build。
- 相对固定上游普通静态 handler，保留 OpenReader 对 API/WebDAV/WebSocket/uploads/assets namespace 的
  显式优先级和 rooted same-file 防护，属于单容器与安全适配。
- 本合同完成前，Reader 外观静态资源不能标记为完整 HTTP/runtime 签收；此前 Reader 交互、排版、夜间
  纯黑、EPUB/CBZ/音频/TTS 合同本身不重开。

## 7. 实施顺序

1. 独立提交本合同、矩阵和 REST action 台账，不改应用或测试。
2. 提交只暴露旧实现缺口的红测。
3. 复用 `openFrontendFile()` 和统一 404/405，增加 public 子树分流；不复制第二套文件验证器。
4. 完成全部验证后提交推送，由受信 runner 发布 amd64/arm64 镜像，再补发布证据。
