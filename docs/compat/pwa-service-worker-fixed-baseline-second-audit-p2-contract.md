# PWA Service Worker 与离线外壳：固定基准第二轮盘点（P2）

审查日期：2026-11-12。基于 main `f82e0f5a`。状态：**contract-only / awaiting-user-decision**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本切片按 `readerdev-compat-inventory` 只写文档，不改应用代码。上游带一套 PWA Service Worker（下称 SW），
OpenReader 没有，总账此前也没有分类。本文逐项提取上游行为并分类，写明安全与多用户约束，给出目标方案和
先红测试计划。是否实现、实现哪几类缓存由用户决定（第 8 节）；用户批准前不做实现。

## 0. 摘要

- 上游生产构建注册 Workbox 4.3.1 生成的 `service-worker.js`。它先从 Google CDN 加载 Workbox，再导入自写的
  `sw.js`，然后预缓存 51 个构建文件（不含 `index.html`）。首页走 network-first（缓存名 `home`）；
  `/reader3/cover` 走 cache-first（缓存名 `bookCover`，键是原始封面 URL，30 天 / 1000 条）。`/reader3/` 下的
  其它接口只走网络。剩下的请求（主要是第三方图片）永久缓存到 `OPAQUE_CAHCE` / `SITE_CAHCE`。点击版本号会
  检查更新、清首页缓存并刷新。
- 结果：在 HTTPS 或 localhost 下，上游安装到主屏幕的 Web App 断网或服务器不可达时仍能打开外壳，再从
  localforage 读书架快照和已缓存章节。OpenReader 的 IndexedDB 按账号缓存已经覆盖这些数据，但没有 SW，服务器
  不可达时页面本身打不开；README 写着“需要连接服务器”。
- 上游有六处不能照抄：
  - Workbox 来自第三方 CDN；
  - 封面缓存键跨账号共享；
  - 第三方 opaque 响应无上限永久缓存，并剥离 Referer；
  - 运行时缓存发布后从不清理；
  - `SKIP_WAITING` 发给已激活的 worker，从未生效；
  - `public/manifest.json`（pr-701 竖屏锁定）在构建时被丢弃。
- 建议：实现一个同源自托管、白名单式的最小 SW，包括构建资源预缓存、导航 network-first 离线回退，所有接口
  都不缓存；同时给静态文件加明确的缓存头。封面离线缓存是可选项，须按账号隔离、退出登录即清空。

## 1. 范围、权威文件与证据

### 1.1 上游（fa22f27）

| 文件#方法 | 内容 |
| --- | --- |
| `web/vue.config.js:91-245`（`pwa`） | `workboxOptions`：`exclude: ["index.html"]`（110）、`importScripts: ["sw.js"]`（111）、`cleanupOutdatedCaches: true`（112）、`skipWaiting` 被注释（113）、`runtimeCaching`（114-243）；`customWorkboxPlugin`（18-81）；`manifestOptions: {display: "fullscreen"}`（99-102）；构建版本号（4-16） |
| `web/public/sw.js` | `CLEAR_HOME_CACHE` 消息（1-5）；`workbox.routing.setDefaultHandler`（7-121） |
| `web/src/registerServiceWorker.js#registerServiceWorker` | 生产环境、`nopwa`、`ready`、`registered` 版本比较（5-57） |
| `web/src/main.js` | `registerServiceWorker()`（48）；mixin `getImagePath/getCover/getImage`（78-117）；`isWebApp/isPWA`（61-69） |
| `web/src/views/Index.vue` | 版本号 `.version-text`（24）；`#updateForce`（2636-2668）；`#logout`（2732-2738） |
| `web/src/plugins/vuex.js:46` | `version: process.env.VUE_APP_BUILD_VERSION` |
| `web/src/plugins/helper.js` | `#networkFirstRequest`（76-102）、`#cacheFirstRequest`（104-138） |
| `web/src/App.vue` | `#getUserInfo`（572）、`#loadBookShelf`（627-631） |
| `web/src/components/RssArticleList.vue:39,87`、`RssArticle.vue:48` | 仅有的 `useSW=true` 调用 |
| `web/public/manifest.json`、`web/public/index.html` | fork 提交 `0556c7b`（`Merge branch 'pr-701'` 为 `5b7f05a`） |
| `src/main/java/com/htmake/reader/api/YueduApi.kt` | `StaticHandler.create("web")`（93）、`/reader3/cover` 路由（201） |
| `src/main/java/com/htmake/reader/api/controller/BookController.kt#getBookCover` | 178-210 |

### 1.2 OpenReader（f82e0f5a）

`frontend/index.html`、`frontend/public/manifest.webmanifest`、`frontend/vite.config.js`、`frontend/src/main.js`、
`frontend/src/router/index.js`、`frontend/src/layouts/AppLayout.vue`、`frontend/src/api/client.js`、
`frontend/src/stores/user.js`、`frontend/src/stores/bookshelf.js`、`frontend/src/utils/browserCache.js`、
`frontend/src/utils/shelfNetworkFirst.js`、`frontend/src/utils/bookCover.js`、
`frontend/src/components/rss/RSSArticleListDialog.vue`、`backend/main.go`、`backend/api/cover_image.go`、
`backend/services/coverimage/capability.go`、`backend/services/coverimage/service.go`、`README.md`、`README_CN.md`。

### 1.3 上游构建复现

上游 `web/` 没有 lockfile。按 `.github/workflows/release.yml` 的步骤（Node 14，`cd web && npm install && npm run build`），
在隔离的 `node:14-bullseye` 容器（Node 14.21.3、npm 6.14.18）中构建 fa22f27 的 `web/`。依赖解析为
`@vue/cli-plugin-pwa@4.5.19`、`@vue/cli-service@4.5.19`、`workbox-webpack-plugin@4.3.1`、`register-service-worker@1.7.2`、
`copy-webpack-plugin@5.1.2`，都是各自 4.x/5.x/1.7.x 线的最后版本，结果确定。构建日志打印
`WARN The public/manifest.json file will be ignored in favor of pwa.manifestOptions`。

下文“构建产物”指这次产出的 `dist/`。库语义按同版本源码核对：

- `register-service-worker@1.7.2/index.js`；
- `workbox-build@4.3.1` 的 `templates/sw-template.js` 和默认值（`importWorkboxFrom: 'cdn'`、`skipWaiting: false`）；
- `workbox-webpack-plugin@4.3.1` 的 `lib/get-default-config.js`；
- `workbox-precaching@4.3.1/utils/deleteOutdatedCaches.mjs`；
- `@vue/cli-plugin-pwa@4.5.19` 的 `index.js` 与 `lib/HtmlPwaPlugin.js`；
- `copy-webpack-plugin@5.1.2/dist/postProcessPattern.js:138`；
- Vert.x web 3.8.1 的 `StaticHandler.java:52,57` 与 `StaticHandlerImpl.java:121`。

没有运行上游后端，所以第 2.8 节的离线可见行为是由代码推导的，未经浏览器实测。

## 2. 上游行为

### 2.1 注册（`registerServiceWorker.js#registerServiceWorker`）

1. 条件是 `process.env.NODE_ENV === "production"` 且 `!window.getQueryString("nopwa")`。`getQueryString` 是 `index.html`
   内联的正则函数，参数要带非空值才返回真值：`?nopwa=1`、`?nopwa=true`、`?NOPWA=1` 关闭注册，裸 `?nopwa`、`?nopwa=`
   不关闭（已用该函数原文实测）。关闭只是不注册，不会注销已有的 SW，也不清缓存。
2. 注册 `${BASE_URL}service-worker.js`（`publicPath: "./"`，上游是 hash 路由，页面总在 `/`，作用域为根目录）。
   register-service-worker 等到 `window.load` 后才注册。非安全上下文（例如普通 HTTP 局域网地址）中
   `'serviceWorker' in navigator` 为假，它什么也不做。
3. `ready` 时设置 `window.serviceWorkerReady = true`，供 `getImagePath` 使用。
4. `registered(registration)`：比较 localStorage `READER_APP_BUILD_VERSION` 与构建时注入的 `VUE_APP_BUILD_VERSION`
   （`v2.5.4-MMDDHHmm`）。二者不同时调用 `registration.active.postMessage({type: "SKIP_WAITING"})`，再写入新版本号。
   按 register-service-worker 1.7.2 与 SW 规范推导，实际效果如下：
   - 首次访问时 `registration.active` 为 `null`，`postMessage` 抛出 TypeError，被库的 `.catch(handleError)` 吞掉（没有
     `error` 钩子）。本次既不写版本号，也没有挂上 `onupdatefound`。
   - 之后的访问把消息发给**已激活**的 worker。它调用 `self.skipWaiting()` 不会提升等待中的新 worker：Try Activate 只看
     waiting worker 自己的 skip waiting 标志，而页面仍在使用该注册。所以这条分支从未让新版本提前接管，新 SW 只在
     所有受控页面关闭后激活。
5. `SKIP_WAITING` 的监听器不在 `sw.js`，而在生成的 `service-worker.js` 里：workbox-build 4.3.1 模板在
   `skipWaiting: false` 时生成该监听器。

### 2.2 生成的 `service-worker.js`（构建产物）

按执行顺序：

1. `importScripts("https://storage.googleapis.com/workbox-cdn/releases/4.3.1/workbox-sw.js")`：`importWorkboxFrom` 默认是
   `cdn`，Workbox 各模块随后也从同一 CDN 加载。CDN 不可达时 SW 脚本求值失败、注册失败，等于没有 SW。
2. `importScripts("sw.js", "precache-manifest.<hash>.js")`。
3. `workbox.core.setCacheNameDetails({prefix: "reader"})`，`cacheId` 取自 package 名。
4. `SKIP_WAITING` 消息监听。
5. `precacheAndRoute(self.__precacheManifest, {})`，共 51 项：
   - `js/` 4、`css/` 4、`fonts/` 3；
   - `img/` 18（含 15 个 `img/icons/*`）、`bg/` 14、`bookSourceDebug/` 3；
   - 根目录 `browsertest.html`、`favicon.ico`、`manifest.json`、`robots.txt`、`sw.js`。

   用户配置 `exclude: ["index.html"]` **替换**了插件默认的 exclude（`.map`、`img/icons/`、`favicon.ico`、`manifest*.js`），
   所以图标与 manifest 也被预缓存，只有 `index.html` 不在清单里。
6. `cleanupOutdatedCaches()` 只删除名字含 `-precache-`、属于本作用域、且不是当前预缓存的缓存。`home`、`bookCover`、
   `SITE_CAHCE`、`OPAQUE_CAHCE` 都没有版本号，发布后从不清理。
7. 注册两条运行时路由（下表 R1、R2）。`sw.js` 的默认处理器只处理路由与预缓存都没匹配到的请求。

### 2.3 路由与缓存

| ID | 匹配 | 策略 | 缓存名 / 键 | 条件与上限 |
| --- | --- | --- | --- | --- |
| R1 首页 | `^https?://[^/]*/?$`：任何源的根路径，不带查询串 | NetworkFirst，无超时 | `home` / 请求 URL | 只缓存 200；网络失败才读缓存 |
| R2 封面 | `^https?://[^/]*/reader3/cover`：任何源 | CacheFirst | `bookCover` / `path` 查询参数（原始远程 URL 字符串） | 只缓存 200，opaque 不缓存；30 天、1000 条 |
| D1 首页（默认处理器） | 同源且 `pathname` 为 `/`，例如 R1 不匹配的 `/?nopwa=1` | 网络优先，失败读 `home` | `home` / 请求 | 缓存任何非 opaque 响应，**不看状态码** |
| D2 预缓存兜底 | `getCacheKeyForURL(request.url)` 命中 | 读预缓存 | 预缓存 | — |
| D3 接口 | URL 含 `/reader3/` | 只走网络 | 不缓存 | — |
| D4 其它 | 其余请求：第三方图片、`/epub/` 资源、`/assets/` 用户资源等 | 先查 `OPAQUE_CAHCE` 再查 `SITE_CAHCE`，未命中再请求 | opaque 响应进 `OPAQUE_CAHCE`；200 的 GET 且 `Content-Type` 含 `image/` 或 `application/x-font` 进 `SITE_CAHCE` | **无过期、无条数上限**；非导航、URL 不含当前 referrer 的请求改用 `referrer: ""` 重发（剥离 Referer，绕过防盗链） |

补充：

- `sw.js:1-5` 收到 `CLEAR_HOME_CACHE` 时执行 `caches.delete("home")`。
- Chromium 把每个 opaque 响应按约 7 MB 的填充大小计入存储配额，D4 的无上限 opaque 缓存会很快吃掉配额。
- 上游后端的 `/reader3/cover` 公开、无鉴权，抓取任意 `path`。它返回 `Cache-Control: 86400`，缺少 `max-age=`，
  浏览器 HTTP 缓存不会按 1 天生效，实际的封面缓存靠 R2。

### 2.4 图片地址选择（`main.js#getImagePath`）

- 以 `http(s)://` 或 `//` 开头的地址：`useSW && window.serviceWorkerReady` 时直接返回原始 URL，否则返回
  `${api}/cover?path=<url>`（未编码）。相对地址拼接 `apiRoot`。
- 只有 RSS 文章图片传 `useSW=true`：`RssArticleList.vue:39` 的列表缩略图，`:87` 与 `RssArticle.vue:48` 的预览列表。
- 书籍封面（`Index.vue:594,910,2819`、`BookInfo.vue:17`、`Content.vue:294,379,383`）和 RSS 源图标（`RssSourceList.vue:49`）
  都不传，始终经 `/reader3/cover` 和 R2。

### 2.5 版本号（`Index.vue:24` 与 `#updateForce`）

- 显示 `$store.state.version`，即构建时的 `v<package.version>-<MMDDHHmm>`。
- 非安全上下文中 `'serviceWorker' in navigator` 为假，点击什么也不做，也没有提示。
- 否则：`getRegistrations()` → 逐个 `await update()` → 向 `controller` 发 `CLEAR_HOME_CACHE` 和 `SKIP_WAITING`
  （同 2.1，发给已激活的 worker，不会提升新 worker）→ 50 ms 后 `location.reload(true)`。
- 可见效果：首页缓存被清掉，页面刷新；刷新时浏览器通常会向服务器重新验证 `index.html`。新 `index.html` 引用的
  新哈希资源不在旧预缓存里，经 D4 走网络取回，所以即使新 SW 仍在等待，刷新后运行的也是新代码。

### 2.6 HTTP 缓存头（`YueduApi.kt:93`，Vert.x web 3.8.1）

生产模式下 `StaticHandler` 默认开启缓存。所有 web 文件，包括 `index.html`、`service-worker.js`、`sw.js` 和
`manifest.json`，都返回 `cache-control: public, max-age=86400` 与 `Last-Modified`。

浏览器检查 SW 更新时默认绕过顶层脚本的 HTTP 缓存（`updateViaCache` 默认为 `imports`）。但导入的 `sw.js`，以及
R1 发出的 `fetch`，都可能在 1 天内直接命中 HTTP 缓存。这是上游需要“点版本号强制刷新”的原因之一。

### 2.7 数据缓存已移出 SW

`vue.config.js:126-213` 注释掉了四条路由：

- 书架：networkFirst，键 `getBookshelf@<accessToken 用户名>`，`refresh` 时不读缓存；
- 书源：networkFirst；
- 目录：networkFirst，5 s 超时，键 `<url>@chapterList`；
- 正文：cacheFirst，30 天 / 10000 条，键 `<url>@chapterContent-<index>`。

注释里写着“书源手动缓存在 localStorage”。fa22f27 改用 localforage：`helper.js#networkFirstRequest` 与
`#cacheFirstRequest`，键前缀 `localCache@`。例如 `App.vue#loadBookShelf` 的键是 `getBookshelf@<currentUserName>`，
另有 `App.vue#getUserInfo` 和 `Reader.vue#getCatalog`（973）。

### 2.8 离线可见行为（代码推导，未实测）

- SW 已激活（安全上下文，且安装时 CDN 可达）时离线打开：
  1. R1 网络失败，读出 `home` 中最近一次成功的 `index.html`；
  2. 预缓存提供 JS/CSS，应用挂载；
  3. 接口请求失败，`networkFirstRequest` 回退到 localforage 中的书架与用户信息，并 `setConnected(false)`；
  4. 封面来自 R2，已缓存的目录与章节可读。
- 没有 SW 时（普通 HTTP、`?nopwa=1`、CDN 不可达），只能靠 1 天内仍新鲜的浏览器 HTTP 缓存。
- 上游 `Index.vue#logout` 只执行 `setToken("")` 并刷新，不清任何 SW 缓存或 localforage（上游默认单用户）。

### 2.9 manifest 与头部标签（构建产物）

- fork 提交 `0556c7b` 新增 `public/manifest.json`（`standalone`、`orientation: portrait`、`favicon.ico` 图标），并在模板里
  加了一条 `<link rel="manifest">`。但 `vue.config.js` 设置了 `pwa.manifestOptions`：@vue/cli-plugin-pwa 4.5.19 忽略这个
  文件并在构建日志中警告，HtmlPwaPlugin 在 emit 阶段无条件写入生成的 `manifest.json`，copy-webpack-plugin 5.1.2 遇到
  同名资源时跳过。
- 实际发布的 `manifest.json` 是
  `{"name":"阅读","short_name":"阅读","theme_color":"#ffffff","icons":[android-chrome 192/512 与两枚 maskable],"start_url":".","display":"fullscreen","background_color":"#000000"}`，
  **没有 orientation**，display 是 `fullscreen`。
- `index.html` 有两条相同的 `<link rel="manifest" href="manifest.json">`，另有 `apple-mobile-web-app-capable: yes`、
  `black-translucent` 状态栏、`apple-mobile-web-app-title: 阅读`、apple-touch-icon 152、mask-icon 和 msapplication 标签。
- `main.js` 的 `isWebApp`（`navigator.standalone`）和 `isPWA`（display-mode 媒体查询）只用于对话框状态栏样式，与 SW 无关。

## 3. OpenReader 现状（f82e0f5a）

1. **没有 SW。** `frontend/vite.config.js` 只有 Vue 插件，`frontend/src/main.js` 不注册 SW，源码中没有 `serviceWorker`、
   `caches.` 或 Workbox。`frontend/public/` 只有 `manifest.webmanifest`、`openreader.svg`、`bg/`（1.5 MB）和 `themes/`。
2. **静态文件。** 路由是 `createWebHistory`，`Home`、`Login`、`Reader`、`SourceDebug` 都是懒加载分块
   （`frontend/src/router/index.js:1-7,50`）。
   - `backend/main.go#serveFrontend`（186）把 SPA 历史路由（`isFrontendHistoryRoute`，234）回退到 `index.html`，例如
     `/`、`/login`、`/books/:id`、`/books/:id/read`、`/reader/remote/:id`、`/settings`。
   - `/assets/*` 与公开文件经 `serveFrontendFile`（272）的 `http.ServeContent`（284）发送：有 `Last-Modified` 与条件请求，
     但**没有 `Cache-Control`**。浏览器对 `index.html` 按启发式新鲜度缓存（通常是距 Last-Modified 时长的 10%）。
   - 发布后，旧页面再懒加载已删除的旧分块会得到 404 JSON，动态导入失败；前端没有处理 `vite:preloadError` 或
     `router.onError`。
3. **接口。** `/api/*` 用 `Authorization: Bearer`，token 在 localStorage `openreader_token`。只有 HTTP 401 进入重新登录流程
   （`frontend/src/api/client.js:26`），网络错误不会登出。
4. **免 Authorization 的资源。**
   - `/api/cover/<capability>`（`backend/api/cover_image.go:38`）返回 `private, max-age=86400`、CORP same-origin、`nosniff` 和
     `no-referrer`。capability 用 AES-GCM 加随机 nonce（`coverimage/capability.go:44`），TTL 48 h（:19）。同一用户、书源、
     URL 的投影在剩余有效期超过 1 h 时复用（`service.go:130`），进程重启后重新签发。
   - 章节图片、EPUB、CBZ、UMD、audio capability 返回 `private, max-age=300`。
   - `/uploads/*` 按路径公开。
5. **浏览器数据缓存**（已签收，见 `index-local-cache-scope-p1-contract.md`）。IndexedDB `openreader-cache` / `responses`，
   键按 `user:<id>` 作用域，包括：
   - 书架 network-first 回退 IndexedDB 快照（`utils/shelfNetworkFirst.js`，键 `bookshelf@getBookshelf:<request>:user:<id>`），
     快照保留响应中的 `coverResourceUrl`；
   - Reader 书籍/目录缓存；
   - 章节正文缓存。
6. **退出登录。** `stores/user.js#logout`（59）调用 `clearSession`（79），清 token、重置各 store，但不删除按账号作用域的
   IndexedDB 缓存。这是有意设计：其它账号读不到这些键。
7. **离线。** `AppLayout.vue:10` 显示“网络已断开，部分同步能力会在恢复连接后继续。”，恢复 `online` 后刷新书架
   （814-821、909-919）。但服务器不可达时页面本身打不开。README FAQ（`README.md:386-387`、`README_CN.md:381-382`）写明
   需要连接服务器。
8. **版本号。** `AppLayout.vue:19` 显示 `/api/health` 返回的 version/commit，点击执行 `refreshHealthInfo(true)`（619），提示
   “构建 <日期> · <commit>”。这是 `59815ba7`（2026-06-03）加入的，只在 `docs/upstream-alignment-audit.md` 留了一行，
   没有对照 `Index.vue#updateForce` 分类。
   **更正**：`index-settings-action-surface-second-audit-p1-contract.md` §2.2 签收的是“后端设定”状态点击（上游
   `Index.vue#setIP`）收敛为 `/api/health` 检查，不是版本号点击。
9. **RSS 文章图片。** `components/rss/RSSArticleListDialog.vue:41` 直接使用 `article.image` 原始 URL，浏览器直连第三方，
   并按默认策略发送 origin 级 Referer。上游没有 SW 时这些图片走 `/reader3/cover?path=` 服务端代理；有 SW 时直连，
   并由 D4 剥离 Referer、永久缓存。封面合同明确排除了这条分支，此前没有分类。
10. **manifest。** `frontend/public/manifest.webmanifest`（`standalone`、`portrait`）是 2026-06-30 按 `0556c7b` 的意图加入的
    （`docs/upstream-alignment-audit.md:153`）。name/short_name 将由显示名切片改为“闲读”；manifest 归该切片所有，
    本切片不修改。

## 4. 差异矩阵

| ID | 上游行为 | OpenReader | 判定 | 要求 |
| --- | --- | --- | --- | --- |
| W1 | 生产环境注册 SW，`?nopwa=1` 跳过（2.1） | 无 | **must-fix**（待 Q1） | 仅在生产构建、安全上下文、浏览器支持时注册（S8）；`?nopwa` 见 W17 |
| W2 | 预缓存全部构建文件，`index.html` 除外（2.2） | 无 | **must-fix**（待 Q1） | 预缓存 Vite `dist` 的哈希资源与外壳需要的公开文件（`bg/`、`themes/`、图标、manifest），带内容修订号；不含 source map，不含任何用户数据 |
| W3 | 首页 NetworkFirst，离线回退最近一次成功的 `index.html`（R1、D1） | 无，离线打不开 | **must-fix**（待 Q1） | 所有 SPA 历史路由的导航都 network-first，只缓存 200 的同源 HTML；离线回退缓存的外壳 |
| W4 | 离线后读书架快照和已缓存章节（2.8） | 数据缓存已有，缺外壳 | **must-fix**（随 W1–W3） | 按 S6；不新增数据缓存 |
| W5 | Workbox 从 Google CDN 导入（2.2） | — | **intentional-redesign**（安全修复） | SW 不得 `importScripts` 第三方地址，全部代码同源自托管 |
| W6 | 比较 `READER_APP_BUILD_VERSION` 后向已激活 worker 发 `SKIP_WAITING`，实际无效（2.1） | — | **acceptable-change** | 不复制无效分支，也不新增该 localStorage 键。新 SW 按默认生命周期等待，与上游实际效果一致；导航 network-first 保证下次联网加载就用新代码。是否自动接管见 Q3 |
| W7 | 版本号点击强制更新（2.5） | 只提示构建信息 | **unknown**（待 Q2） | 推荐：启用 SW 后点击仍先提示构建信息，再执行修正后的强制更新（`update()`、清导航缓存、向 `registration.waiting` 发 `SKIP_WAITING`、`controllerchange` 后只刷新一次）；没有 SW 时保持现状 |
| W8 | 封面 CacheFirst，键为原始 URL，跨账号共享，30 天 / 1000 条（R2） | 服务端按用户缓存 + 浏览器 HTTP 缓存 24 h | **unknown**（待 Q1 选 B） | 若实现：键用完整 capability URL（S2），只缓存 200 的同源 `image/*`，30 天 / 1000 条，退出登录或切换账号即清空（S5）；不得用原始 URL 作键 |
| W9 | 第三方 opaque 响应无上限永久缓存，剥离 Referer（D4） | 不缓存 | **intentional-redesign**（安全、缓存加固） | SW 不拦截跨源请求、不改写 referrer、不缓存 opaque 响应 |
| W10 | 同源图片和字体永久缓存，包括 `/epub/` 资源（D4） | 无 | **acceptable-change** | 不做运行时缓存；`/uploads/*` 与各类 capability 资源只用 HTTP 缓存（封面见 W8） |
| W11 | `/reader3/` 只走网络（D3） | 无 SW | **必须保持的不变量** | SW 对 `/api/`（W8 的封面路由除外）、`/reader3/`、`/ws`、`/webdav`、`/uploads` 不调用 `respondWith`，不存任何接口响应 |
| W12 | R1 匹配任何源的根路径 | — | **acceptable-change**（修正） | 只处理同源导航 |
| W13 | D1 连非 200 的首页响应也缓存 | — | **acceptable-change**（修正） | 只缓存 200、`type: basic` 的 HTML |
| W14 | 运行时缓存发布后从不清理（2.2） | — | **acceptable-change**（缓存加固） | 缓存名带版本；激活时删除不在白名单内的 `openreader-` 前缀旧缓存，不碰其它名字 |
| W15 | 静态文件 `public, max-age=86400`（2.6） | 没有 `Cache-Control`，按启发式缓存 | **acceptable-change**（缓存加固；Q1 选 A/B/C 都做） | `index.html` 与 SPA 路由用 `no-cache`；`/assets/*` 哈希文件用 `public, max-age=31536000, immutable`；SW 脚本和其余公开文件用 `no-cache` |
| W16 | 数据缓存走 localforage（2.7） | IndexedDB 按账号作用域 | **aligned / 不重开** | SW 不重复缓存书架、书源、目录和正文 |
| W17 | 只有带非空值的 `?nopwa=1` 生效，且不注销已有 SW | — | **acceptable-change**（扩展） | 接受 `?nopwa=1`（大小写不敏感），裸 `?nopwa` 也生效；同时注销已有注册并删除 `openreader-` 前缀缓存，作为故障逃生口 |
| W18 | SW 就绪时 RSS 图片直连（2.4） | 始终直连 | Q1 选 A/B：**aligned**（Referer 剥离与 opaque 缓存按 W9 不复制）；选 C/D：**unknown**，与上游无 SW 时的服务端代理不一致，另立切片 | 可另行给 RSS 图片加 `referrerpolicy="no-referrer"`，不属本切片 |
| W19 | 实际发布的 manifest 为 `fullscreen`、无 orientation（2.9） | `standalone` + `portrait` | **unknown / 不在本切片** | 交显示名切片判断；本切片不改 manifest 与头部标签 |
| W20 | 非安全上下文没有 SW，版本号点击无反应（2.1、2.5） | — | **aligned 约束** | 文档写明离线外壳只在 HTTPS 或 localhost 下可用；普通 HTTP 局域网部署的行为不变 |
| W21 | 有 SW 时，旧页面懒加载的旧分块来自旧预缓存，不受发布影响（2.2） | 发布后旧分块 404，动态导入失败（第 3 节第 2 项） | **must-fix**（随 W2） | 预缓存覆盖全部懒加载分块；非 SW 环境可选在 `vite:preloadError` 时只刷新一次（acceptable-change） |

不重开（已签收）：

- `index-local-cache-scope-p1-contract.md`：key 所有权、四个分组、总量范围、旧版无作用域正文不认领。SW 缓存不计入四个
  分组和总量（上游 `scanCacheStorage` 也只统计 localforage），清理按钮不碰 SW 缓存。
- `book-cover-proxy-p2-contract.md`：capability 格式与 TTL、响应头、服务端按用户缓存、`coverResourceUrl` 的优先级与三态。
- `index-settings-action-surface-second-audit-p1-contract.md` §2.2：后端状态点击只做 `/api/health` 检查。
- 章节图片、EPUB、CBZ、UMD、audio capability；只有 401 才进入重新登录（`index-authenticated-session-p1-contract.md`）。
- `manifest.webmanifest` 与 `index.html` 头部标签，归显示名切片。

## 5. 安全与多用户约束

**S1 不缓存需认证的接口响应。** SW 用白名单，只处理三类请求：

- 同源导航；
- 预缓存清单中的 URL；
- Q1 选 B 时的 `GET /api/cover/<capability>`。

其余请求一律不调用 `respondWith`，交给浏览器原样处理，包括接口、带 `Range` 的音频、SSE 缓存流、WebSocket、WebDAV 和
上传。带 `Authorization` 头的请求和非 GET 请求永不缓存。写入缓存前检查 `response.ok`、`type === 'basic'` 与 `Content-Type`。

**S2 不读其它账号的数据。** Cache Storage 按源共享，同一浏览器里的所有账号都能看到。封面若缓存，键必须是完整的
capability URL：capability 由服务端为某个用户签发（`coverimage` 把 user id 加密在里面），其它账号的页面永远不会请求它，
所以读不到。不得像上游那样改用原始封面 URL 作键，否则账号 B 会命中账号 A 缓存的封面，也会暴露 A 的书单。
capability URL 是短期 bearer 资源，但它已经同样保存在 IndexedDB 书架快照和浏览器 HTTP 缓存里，SW 缓存不新增暴露面。
capability 过期后，缓存中的字节也只会交给持有同一 URL 的页面。

**S3 封面缓存键的轮换。** capability 最多复用约 47 h，服务重启就换，所以缓存按 30 天 / 1000 条淘汰。离线命中依赖
“快照与封面同时写入”：离线时书架快照里的 `coverResourceUrl` 正是当时写进缓存的键。这已满足离线显示，不为提高在线
命中率去改服务端 capability：那会触碰 `backend/api/books.go` 的投影，而该文件目前归账号限制会话。

**S4 发布后的失效。**

1. 外壳缓存名带构建 id（`openreader-shell-<buildId>`），新版本安装成功后才替换，激活时删除旧外壳。
2. 导航 network-first：联网时总是先向服务器取 `index.html`，加上 W15 的 `no-cache`，不会被 HTTP 缓存挡住。
3. 新 `index.html` 引用的新哈希资源在旧预缓存中未命中时走网络；旧页面懒加载的旧分块仍由旧预缓存提供（W21）。
4. 注册时设置 `updateViaCache: 'none'`，SW 脚本返回 `no-cache`。
5. 若发布了有问题的 SW，下一版 SW 可以只做注销与清理（kill switch）；`?nopwa` 是用户侧的逃生口。
6. 构建 id 同时注入前端，并与 `/api/health` 的 commit 对照，供版本号点击判断是否有新版本。

**S5 退出登录与切换账号。** 在以下时机删除封面等账号相关的 SW 缓存（`caches.delete('openreader-cover-v1')`，页面直接
执行即可，不需要发消息给 SW），保留不含用户数据的外壳缓存：

- `logout`；
- 401 触发的 `clearSession`；
- token 换成另一个用户 id。

IndexedDB 作用域缓存按已签收合同保留。删除失败不能阻塞登出。

**S6 离线可见行为（目标）。**

- 有 token 时离线打开：
  - 外壳渲染，顶部显示现有的离线提示；
  - 书架来自 IndexedDB 快照；封面来自 SW 缓存（Q1 选 B），否则显示占位；
  - 已缓存章节可读；未缓存章节、搜索、探索和 RSS 显示现有错误；
  - 网络错误不得弹登录框或清 token，也不得显示其它账号的数据。
- 无 token 时离线打开登录页：登录请求以网络错误失败，不提供离线登录。
- 恢复联网后保持现有的 `online` → 刷新书架。

**S7 与已签收缓存范围的关系。** SW 只缓存外壳（不含用户数据）和可选的封面字节，不缓存 JSON，不与 `localCache@` 或
作用域 key 重复。SW 缓存不进入“本地缓存”统计和四个清理按钮，也不进入普通/portable 备份或 WebDAV。

**S8 注册条件与生命周期。** 注册条件为
`import.meta.env.PROD && window.isSecureContext && 'serviceWorker' in navigator && !nopwa`，作用域 `/`。不调用
`clients.claim()`，与上游一致：首次访问不受控，下次加载起生效。

**S9 依赖。** SW 代码不引入第三方依赖，不访问外部网络。如果确需新增前端依赖，必须通过
`scripts/check-frontend-advisories.mjs`；优先不新增。

## 6. 目标方案（用户批准后实施）

| 步骤 | 内容 | 主要文件 | 备注 |
| --- | --- | --- | --- |
| T0 | 缓存头（W15） | `backend/main.go#serveFrontendFile` | 可单独先做；不涉及数据与 API |
| T1 | SW 本体（W1–W5、W9–W14、W21，S1–S4） | `frontend/public/sw.js` 或构建时生成；一个小型 Vite 插件在 `generateBundle` 后写出预缓存清单与构建 id | 不用 Workbox 或 `vite-plugin-pwa`，避免新依赖和 CDN；路由逻辑写成可在 node 测试中导入的纯模块 |
| T2 | 注册与 `?nopwa`（W1、W17，S8） | `frontend/src/utils/serviceWorker.js`、`frontend/src/main.js` | 不新增“SW 就绪即直连”的 `getImagePath` 式分支 |
| T3 | 版本号点击（W7，按 Q2） | `frontend/src/layouts/AppLayout.vue` | |
| T4 | 登出清理（S5，Q1 选 B 时） | `frontend/src/stores/user.js` | 该文件归账号限制会话，须等其合入后再动 |
| T5 | 文档 | README FAQ（离线需要 HTTPS）、`docs/regression-smoke.md` | |

不在本切片：manifest（W19）、RSS 图片服务端代理（W18 在 Q1 选 C/D 时）、服务端 capability 稳定化（S3）。

## 7. 测试计划（先红）

下列测试先单独以 `test:` 提交，必须在 `f82e0f5a` 上失败。

**后端（Go）**

- B1：`GET /`、`/books/1`、`/login` 返回 `Cache-Control: no-cache`（现在没有该头）。
- B2：`GET /assets/<hash>.js` 返回 `public, max-age=31536000, immutable`。
- B3：公开目录中的 `sw.js` 返回 `Cache-Control: no-cache` 与 JavaScript MIME。
- 对照 C1：`/api/*`、`/api/cover/*` 的现有响应头不变；SPA 回退与 404 JSON 不变。

**前端（`node --test`）**

- F1 注册门：开发构建、非安全上下文、不支持 SW、带 `?nopwa` 时都不注册，四个条件都满足才注册（现在没有该模块）。
- F2 `?nopwa`：注销全部注册，只删除 `openreader-` 前缀的缓存。
- F3 SW 路由表（用假的 `Request` 和 `caches` 驱动可导入的模块）：
  - 导航 network-first，只缓存 200 的 basic HTML，离线回退外壳；
  - `GET /api/books`、`POST`、带 `Authorization`、跨源图片、带 `Range` 的请求都不处理；
  - 封面路由仅在启用时 cache-first，403/404/500 与非图片响应不缓存，超过 1000 条淘汰最旧的；
  - 激活时只删除 `openreader-` 前缀的旧缓存。
- F4 预缓存清单生成：包含全部 `dist` 资源（含所有懒加载分块）和外壳需要的公开文件，不含 `.map` 与 `index.html`；
  任一资源变化时构建 id 随之改变。
- F5 静态检查：SW 源码中没有 `importScripts('http` 或任何外部 URL，缓存名都以 `openreader-` 开头。
- F6（Q1 选 B）：登出、401 清会话、切换到另一用户 id 时删除封面缓存，不碰外壳缓存与 IndexedDB 作用域缓存。
- F7（Q2 选 A）：版本号点击在有 SW 时调用 `update()`、删除导航缓存、向 `registration.waiting`（不是 `active`）发
  `SKIP_WAITING`，`controllerchange` 后只刷新一次；没有 SW 时只提示构建信息（现状）。

**真实浏览器**（Go 服务在 `127.0.0.1`，属于安全上下文；视口 1440×900、390×844、360×800）

- E1：首次加载后注册，第二次加载受控；Cache Storage 有 `openreader-shell-<buildId>`，内容与清单一致。
- E2 离线：主要方式是停掉 Go 服务（Playwright 的 `setOffline` 对 SW 发出的请求不一定生效，只作辅助），然后重新加载 `/`
  和 `/books/<id>/read`。外壳、离线提示、IndexedDB 书架和已缓存章节都可用，不弹登录框，控制台无未处理错误，无横向溢出。
- E3 发布更新：先用构建 A 访问，换成构建 B 后联网刷新，立即运行 B 的资源（核对哈希），旧 SW 仍在；在 B 上线前打开的
  旧页面懒加载 Reader 分块仍然成功（W21）；关闭全部页面后 B 的 SW 激活，A 的外壳缓存被删除。
- E4 账号隔离（Q1 选 B）：账号 A 加载书架后封面进入缓存；登出后缓存被删除；账号 B 登录后只请求 B 的 capability；
  `caches.keys()` 中除封面缓存外没有任何 `/api/` 条目。
- E5：`?nopwa` 注销并清缓存，之后的加载不受控。
- E6 非安全上下文：例如用 `--host-resolver-rules` 把 `openreader.test` 指到 127.0.0.1，再访问 `http://openreader.test`。
  此时不注册 SW，版本号点击保持现状。
- E7（Q2 选 A）：版本号点击强制更新。

实现完成后照常跑回归门：Go 全量、vet、关键包 race，前端 test 与 build，两个公告门，`openreader-smoke` 三视口。

## 8. 待用户决定

**Q1 要不要加 SW，加哪几类缓存？**

- A（推荐）最小离线外壳：预缓存构建资源，导航 network-first 并离线回退，接口一律不缓存，另加 W15 缓存头。
- B：A 再加封面离线缓存，按账号隔离，退出登录或切换账号即清空。
- C：不加 SW，只做 W15 缓存头；把“离线打不开”记为 intentional-redesign，README 维持现状，W18 另立切片。
- D：都不做，只保留本合同。

**Q2 启用 SW 后，版本号点击做什么？**

- A（推荐）：先提示构建信息，再执行修正后的强制更新。
- B：保持只提示构建信息。

**Q3 新版本的 SW 何时接管？**

- A（推荐）：按上游的实际效果，等所有页面关闭或用户点版本号后接管；联网导航始终先取新页面，新代码下次加载即生效。
- B：检测到新版本就自动接管并刷新一次。这是上游代码的本意，但会打断正在阅读的页面。

W19（manifest）由显示名切片处理；W18 只在 Q1 选 C 或 D 时需要另立切片。

## 9. 状态

contract-only / awaiting-user-decision。只修改 `docs/`：没有改应用代码，没有构建或发布镜像，生产未访问。
