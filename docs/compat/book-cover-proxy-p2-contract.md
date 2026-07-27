# 远程书籍封面代理 P2 合同

状态：2026-07-27 已完成固定上游合同提取；尚未修改应用代码。

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游权威文件：

- `web/src/main.js#getImagePath/getCover`
- `web/src/views/Index.vue`
- `web/src/components/BookInfo.vue`
- `web/src/components/Content.vue`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#getBookCover`

当前 OpenReader 对应文件：

- `frontend/src/utils/bookCover.js`
- `frontend/src/components/BookCover.vue`
- `frontend/src/views/Home.vue`
- `frontend/src/components/BookInfoPanel.vue`
- `frontend/src/components/overlays/BookManagementMobileList.vue`
- `frontend/src/components/reader/ReaderAudioContent.vue`
- `backend/api/books.go`
- `backend/api/search.go`
- `backend/api/explore.go`
- `backend/api/remote_reader.go`
- `backend/services/chapterimage/fetch.go`

## 固定上游行为

1. `getImagePath` 把 `http://`、`https://` 和 `//` 远程图片交给
   `${api}/cover?path=<remote-url>`；普通服务器相对资源则拼接 `apiRoot`。
2. 书架、BookInfo 和 Reader 封面都消费该转换。书架虽可在 service worker
   已就绪时选择浏览器直连，但固定 Index 调用只传入第二个 `normal` 参数，
   因而普通书架仍使用服务端代理。RSS 图片的 service-worker 直连分支不属于
   本合同的书籍封面范围。
3. `getCover` 在空 URL 时返回内置 `noCover`，懒加载图片失败时也回退到
   `noCover`；损坏封面不得留下一个看似有封面但实际空白的方块。
4. `GET /reader3/cover` 是公开图片资源动作。`path` 为空时返回 `404`；命中
   `storage/cache/<md5(url)>.<url-extension>` 时直接返回，并设置
   `Cache-Control: 86400`。
5. 缓存未命中时，上游服务端用 3 秒超时直连 URL，把非空响应体写入缓存后
   返回；请求或响应体为空时返回 `404`。
6. 固定上游此方法没有注入书源 header/cookie，也没有检查响应状态、图片
   类型、大小、重定向、私网地址或并发写。OpenReader 不得把这些缺陷当成
   为了“对齐”必须复制的行为。

## 当前差异矩阵

| 合同点 | 当前实现 | 裁决 |
|---|---|---|
| 远程封面传输 | `bookCoverUrl()` 直接返回 `customCoverUrl || coverUrl`，浏览器直接请求第三方域名。 | **错误重构 / must-fix**：恢复同源代理。第三方拒绝浏览器、混合内容、跨网络路径或缓存冷启动时，当前会显示空白，而上游可由服务端抓取并缓存。 |
| 加载失败回退 | `BookCover.vue`、书架和移动管理使用 CSS `background-image`，没有 `load/error` 状态；只要字符串非空就隐藏占位文字。 | **错误重构 / must-fix**：失败后显示稳定占位封面；模糊背景失败不得影响前景回退。 |
| 重复封面逻辑 | `Home.vue`、`BookCover.vue`、`BookInfoPanel.vue`、`BookManagementMobileList.vue` 各自拼 CSS URL。 | **错误结构 / must-fix**：可见封面收敛到共享组件/工具，不能修一处而让其它入口继续直连。 |
| 原始 URL 持久化 | `books.cover_url`、搜索/探索结果和换源请求使用 `coverUrl` 作为源数据。 | **必须保留**：代理 capability 只能是响应投影，绝不能覆盖 SQLite、parser result、换源 payload、同步事件、普通/portable 备份中的原始 URL。 |
| 本地/上传/格式封面 | 用户上传使用受控 `/uploads/...`；CBZ 已使用独立签名 resource capability。 | **已验证能力**：同源 URL 继续直用；不得用远程代理包裹上传、CBZ、EPUB/audio 本地资源。 |
| 上游公开 `path` 接口 | 任意调用者可令服务端抓取任意 URL，且没有大小/地址边界。 | **不允许复制的安全缺陷**：OpenReader 使用服务端签发的不透明 capability，不提供接受任意原始 URL 的公开 GET。 |
| 书源请求头 | 上游封面代理没有注入 source headers。 | **勘误 / 非缺口**：本轮不以“对齐”为由携带 Cookie、Authorization 或动态 source header；后续若需防盗链兼容，必须另立凭证隔离合同。 |

## OpenReader API 与投影合同

### 响应投影

- 对一个远程 HTTP(S) 书籍封面，已有 `coverUrl` 保持原始值，并增加可选
  `coverResourceUrl: "/api/cover/<capability>"`。
- 前端显示优先级固定为：
  `customCoverUrl → coverResourceUrl → coverUrl → 占位封面`。其中最后一个
  原始 `coverUrl` 只用于旧服务端/旧缓存兼容，不得在新服务端已经返回
  `coverResourceUrl` 后并行直连或自动绕过安全失败。
- 以下可见响应必须统一投影：
  - 书架列表、单书、创建/修改/刷新/换源/导入后的书架对象；
  - 搜索的普通数组与分页 `list`；
  - 探索结果的 `items`；
  - 换源候选的普通数组与分页 `list`；
  - 临时 Reader session 的 `book`。
- `coverResourceUrl` 是展示字段，禁止写入 SQLite、Book/Chapter variable、
  WebSocket 持久 payload、导入 archive、备份、WebDAV 或导出文件。新实例、
  新登录和重启后的正常列表请求重新投影。
- 空 URL、同源 `/uploads`、CBZ/EPUB/audio capability 和其它已认可的
  OpenReader 相对资源不生成远程封面 capability。

### `GET|HEAD /api/cover/:capability`

| 项目 | 合同 |
|---|---|
| Auth | 不要求 `Authorization`；capability 自身是短期 bearer resource token，供 `<img>`/CSS 请求使用。 |
| 输入 | 只接受服务端为一个已认证响应签发的、不透明且带目的域隔离的 capability。路径和 query 中都不接受原始远程 URL。 |
| 成功 | `200` 返回已校验的图片字节；`HEAD` 返回相同相关 header 且无 body。 |
| Header | 精确 `Content-Type`/`Content-Length`、`X-Content-Type-Options: nosniff`、`Referrer-Policy: no-referrer`，以及 24 小时私有浏览器缓存。 |
| Capability 错误 | 格式错误、篡改、过期或错误 purpose 返回统一 `403`，不得泄露 URL、用户、source、主机路径或 token 内容。 |
| 资源错误 | URL 不安全、远端失败/非 2xx、非受支持图片、超限或缓存不可恢复时返回统一 `404`；不可预期存储错误返回路径无关的 `500`。 |
| 日志 | access log 必须把完整 capability 段替换为 `<redacted>`；不得记录原始 URL、query、Cookie、Authorization 或响应体。 |

Capability 必须：

- 加密并认证原始 URL，或使用等价的不透明服务端映射；仅把 JSON
  base64 后签名仍会暴露带 query 的封面 URL，不满足合同；
- 绑定签发用户、可用时绑定 source，并包含明确 purpose/version/expiry；
- 只能由正常书籍/搜索/探索/换源/临时 Reader 响应投影签发，不能由公开
  `path` 参数自行制造；
- 使用与登录 JWT、章节图片、EPUB、CBZ 和 audio capability 分离的密钥
  派生/purpose。

## 抓取、缓存与数据边界

1. 只允许 `http`/`https`；协议相对 URL 按当前 HTTPS 部署语义规范化。
   用户信息 URL、空 host、无效端口和其它 scheme 失败关闭。
2. DNS 校验和实际 dial 必须使用同一安全结果，拒绝 loopback、private、
   link-local、multicast、unspecified 和云元数据地址；每次重定向重新校验，
   最多 3 次。
3. 默认总超时 3 秒以匹配上游体感；单图最多 8 MiB。只接受经 magic
   检测的 JPEG、PNG、GIF、WebP、BMP 或 AVIF，不能相信扩展名或远端
   `Content-Type`。
4. 不携带 Cookie/Authorization/source header，不使用传入的代理凭证，
   不转发浏览器 Referer。可发送固定安全 `Accept` 和 OpenReader
   `User-Agent`。
5. 缓存位于 `cache/cover-images/user-<id>/` 下，以规范 URL 的
   SHA-256/等价不可逆键命名；路径解析、symlink 和原子写规则与现有章节图片
   缓存同级。缓存文件必须再次校验大小和 magic 后才能发送。
6. 同一用户同一 URL 的并发 miss 只能触发一次下载；失败不得发布临时或半写
   文件。命中缓存不再次请求远端，并刷新受控的最近使用时间。
7. 每用户封面缓存必须有显式总量上限和确定性旧项淘汰；用户删除与全局缓存
   清理需移除对应派生数据。它不是用户内容，不进入普通/portable 备份。
8. capability 过期不删除有效缓存；下一次正常 API 投影可签发新 capability
   并复用同一用户缓存。

## 前端状态合同

1. `BookCover` 以真实 `<img>` 或等价的可观测加载状态渲染；加载中可保留
   占位背景，失败后必须显示“暂无封面”/既有标题占位，而不是透明空白。
2. URL 变化会清除前一次的成功/失败代际；迟到的旧图片事件不得覆盖新封面。
3. 书架列表、搜索/探索卡、BookInfo 前景、BookInfo 模糊背景、书籍管理和
   Reader audio 封面都复用相同 URL 优先级。模糊背景只能是装饰层。
4. 封面失败不弹全局错误、不阻塞书架、搜索、BookInfo 或 Reader 加载，也
   不触发登录失效、书源失败缓存或自动重试风暴。
5. 旧 OpenReader 服务端没有 `coverResourceUrl` 时继续显示原
   `coverUrl`，以保留前后端滚动升级兼容。

## 测试先行门禁

实现前先增加失败测试：

1. Go service：
   - capability 不包含原 URL/query，篡改、错误 purpose、过期失败；
   - 私网/loopback/用户信息 URL、DNS rebinding 和越界重定向失败；
   - 非 2xx、HTML 伪装、超 8 MiB 和截断图片失败；
   - 同 URL 并发只下载一次，缓存命中不联网，半写/损坏缓存不被发送；
   - 用户缓存隔离、总量淘汰、symlink/path traversal 和删除清理。
2. Go API：
   - `GET|HEAD` 成功 header/body、403/404/500 错误和 access-log 脱敏；
   - 书架、搜索分页/非分页、探索、换源候选和临时 Reader 都增加
     `coverResourceUrl`，同时原 `coverUrl` 不变；
   - 创建、刷新、换源、同步、export 和普通/portable backup 从不保存
     capability。
3. Frontend：
   - URL 优先级和旧服务端 fallback；
   - 成功、失败、URL 代际切换及装饰背景；
   - 书架、搜索/探索、BookInfo、管理和 audio 不再各自拼远程 CSS URL。
4. 真实 Go + Chromium：
   - 在 1440×900、390×844、360×800 以浏览器不可直连但服务端可抓取的
     fixture 验证书架、搜索、BookInfo 和 Reader 封面；
   - 远端 404/HTML/超时显示占位且页面仍可操作；
   - 第二次加载命中服务端缓存，无第三方浏览器请求、无 401/500、无控制台
     错误和布局偏移。
5. 全量 Go、frontend test、生产 build、Reader/Index 核心 smoke 通过后，
   才能进入本地 Docker 新卷/历史卷与发布门禁。

## 发布边界

本切片会修改后端资源路由、远程抓取、缓存和多个可见前端入口，适合作为独立
Docker 验证批次。发布报告必须列明 capability/SSRF/缓存验证、允许的安全差异、
未完成的真实设备验证、镜像标签和 OCI digest。
