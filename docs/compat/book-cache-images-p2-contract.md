# BookManage 章节正文图片离线缓存 P2 合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-19 已完成上游/当前实现审查、失败测试、实现、全量自动化、三视口图片 Reader、
Docker 历史卷/备份门禁和本地双架构 GHCR 发布。此合同是
[`book-management-cache-p2-contract.md`](book-management-cache-p2-contract.md) 中“整本正文缓存”
之后的下一切片，实施前必须先写失败测试。

## 权威文件

上游：

- `src/main/java/io/legado/app/help/BookHelp.kt`
  - `saveContent()`、`saveImages()`、`saveImage()`、`getImage()`、`getImageSuffix()`；
- `src/main/java/com/htmake/reader/api/controller/BookController.kt`
  - 普通远程章节读取、`cacheBookSSE()`、`deleteBookCache()`、`fixPic()`、`exportToEpub()`；
- `web/src/components/Content.vue`
  - 正文 `<img>` 的懒加载、普通/连续阅读渲染；
- `web/src/main.js#getImagePath()`。

当前：

- `backend/api/cache_stream.go`、`backend/api/books.go#loadChapterTextContextResultWithOptions`；
- `backend/api/cache.go`、`backend/api/book_cleanup.go`；
- `backend/engine/source_parser.go#normalizeChapterHTMLWithImageStyle`；
- `backend/services/epubreader`、`cbzreader`、`audioreader` 的 capability 资源模式；
- `frontend/src/utils/readerContent.js`、`ReaderChapterContent.vue`；
- `backend/api/books.go#exportBookEPUB`。

## 上游合同与当前差异

| 项目 | 固定上游行为 | 当前 OpenReader | 判定 |
|---|---|---|---|
| 图片发现 | `saveImages()` 从正文 `<img>` 取 `src`，按章节 URL 补成绝对地址；上游实现每行只取第一个匹配。 | 解析器已经把 `src/data-src/data-original/data-url` 归一成安全的绝对 HTTP(S) `src`，但缓存流程不再读取图片。 | `must-fix`。处理每个规范图片标签；同一行多图全部处理属于无损健壮性改进。 |
| 请求语义 | `AnalyzeUrl(src, source=bookSource)` 携带书源请求配置获取二进制。 | 章节文本抓取支持源 headers、限速、代理和取消；没有有界二进制图片抓取器。 | `must-fix`，但不能把 Cookie/Authorization 转发给不受信的跨站图片域。Referer 等非凭证头可按策略保留。 |
| URL/SSRF | 上游接受解析后的 HTTP(S) URL，没有服务端私网保护。 | 多用户 Go 服务若直接抓取任意正文 URL，会形成 SSRF。 | `acceptable security change`：仅 HTTP(S)、无 userinfo；每次 DNS/重定向重新校验。与明确配置的书源/章节同主机可保留局域网书源能力；跨主机图片必须拒绝 loopback、private、link-local、multicast、unspecified 和 metadata 目标。 |
| 超时与大小 | 上游等待请求完成，没有显式单图/单章字节上限。单图异常由 `saveImage()` 吞掉。 | 没有图片限制。 | `acceptable security change`：增加正数环境配置，默认每章最多 64 图、单图 8 MiB、单章 32 MiB、每请求 12 秒、最多 3 次重定向；超过限制只跳过该图。 |
| MIME | 上游按 URL 后缀写文件，未知后缀回退 `.jpg`，不验证内容。 | 不落盘。 | `acceptable security change`：按实际字节识别并只接受 JPEG/PNG/GIF/WebP/BMP/AVIF；HTML、脚本、SVG、空体和声明/嗅探均非图片的响应不得进入同源资源目录。 |
| 失败语义 | 图片失败不使章节文本缓存失败；已完成图片可继续使用。`saveImages()` 等待该章所有任务结束。 | 章节成功只代表正文文件。 | `must-fix`：显式缓存流程等待该章的有界图片计划；单图失败不增加章节 `failedCount`。请求取消必须停止图片读取并阻止安排下一章。 |
| 去重 | 上游按图片绝对 URL 的 MD5 在同一书目录复用，并用进程集合避免同 URL 并发重复下载。 | 无图片缓存。 | `must-fix`：按 `userID + bookID + SHA-256(normalized URL)` 隔离/去重；写临时文件后原子 rename，绝不能使用 URL 后缀或标题作为路径。 |
| 文件布局 | 图片位于该书章节缓存目录的 `images/`，删除书缓存会递归删除正文和图片。 | 正文文件位于 `CacheDir` 的可移植相对路径，删除/统计只追踪章节行。 | `technology-equivalent`：新增派生根 `cache/chapter-images/user-<id>/book-<id>/`，含共享 blobs 和按章节引用；不新增 SQLite 字段。单书清缓存、全局清缓存、删书、删用户、换源/目录替换必须清理对应引用和无引用 blob。 |
| 正文持久化 | 上游 `.txt` 保留远程图片 URL；图片文件另外保存。 | `.txt` 同样保留规范远程 URL。 | `aligned invariant`：不得把短期 capability、JWT 或主机绝对路径写入正文、SQLite、备份、WebSocket 或导出中间状态。 |
| 阅读使用 | 上游 Web Reader 仍按正文远程 URL 渲染；缓存图片主要供 EPUB 导出使用。 | 浏览器无法带 JWT 读取私有缓存文件，直接暴露文件目录又会跨用户泄漏。 | `allowed Go/browser adaptation`：章节响应保持原正文并另带“原 URL → 短期 HMAC capability”的可选映射；前端在完成稳定位置解析后只替换图片 `src`。缺失/失败图片保留原远程 URL。资源端点重新核对用户/书、blob key、指纹、MIME 与 rooted path。 |
| EPUB 导出 | `fixPic()` 把已缓存图片写入 `Images/` 并改写章节引用；缺失图片不阻止导出。 | 当前 EPUB 导出把整行 `<img>` HTML 转义为普通文本，不携带图片。 | `must-fix`：仅把已缓存、验证过的图片写入 EPUB `OEBPS/Images/` 和 OPF manifest，并改写对应章节 `<img src>`；缺图保持安全文本/远程回退，不读取网络、不泄漏 capability。 |
| 样式与预览 | 正文图片保留顺序；`ContentImageStyle=FULL` 影响显示。 | `readerContent.js` 保留 `alt` 和 `data-image-style=FULL`，`el-image` 负责预览。 | `must-preserve`：URL 替换不得丢失 alt、FULL、段落位置、图片顺序、预览列表和 image-load 分页重算。 |
| 旧卷/备份 | 图片是可删除派生缓存；上游删除缓存可重建。 | `cache/` 独立挂载且不属于逻辑 portable backup。 | `technology-equivalent`：旧卷无需迁移；缺失图片根按空缓存处理。备份/恢复不携带 capability 或图片 blob，重启后已挂载 `cache/` 可继续读。 |

## 固定 API 与资源合同

1. 现有 `POST /api/books/:id/cache`、`POST /api/books/:id/cache/stream` 请求和进度字段不变。
   图片失败不改变章节成功/失败计数；请求取消继续以当前上下文为任务生命周期。
2. `GET /api/books/:id/chapters/:index/content` 的 JSON schema 保持兼容，`content` 必须继续等于
   持久化的原 URL 正文。可选 `cachedImages` 映射以原 URL 为 key、`/api/chapter-image/<capability>`
   为 value，并可带统一过期时间；旧客户端忽略它仍能渲染远程图片。前端必须先按原正文计算
   `data-pos`，再把已命中的图片对象 `src` 替换为映射值，不能把 capability 混入位置计算。
3. 新增无需登录 header 的 `GET|HEAD /api/chapter-image/:capability`。capability 是短期、用途隔离、
   HMAC 签名的 bearer，仅绑定一个 `userID/bookID/blobKey/fingerprint/expiry`。端点不得接受文件路径、
   原 URL 或 JWT query；日志必须把 token 全部抹除。
4. 资源成功返回真实、已验证的 `Content-Type`、`X-Content-Type-Options: nosniff`、短期 private cache；
   token 篡改/过期返回 `403`，书/文件不存在或所有权变化返回 `404`，不暴露物理路径。

## 数据、清理与事务边界

- 每章引用清单和 blob 都是派生文件，不修改 schema。引用清单必须原子替换：章节文本抓取成功但
  图片计划部分失败时，只记录真实存在的 blob。
- 删除一章/替换目录时先提交数据库变化，再清该章引用；只删除已无任何章节引用的 blob。
- 单书/批量/全局清缓存以及删书/删用户必须同时清正文与其图片根；清理统计包含真实删除的图片
  文件和字节。外用户操作保持 `404`/作用域隔离。
- 刷新某章只替换该章引用，失败不得删除旧的可用 blob；成功后才清理不再引用的旧 blob。
- 任何读取和删除都必须通过 `CacheDir` 下的 canonical/rooted helper，拒绝 symlink、绝对路径、
  `..`、伪造 blob key 和跨书目录。

## 先写的失败测试

1. parser/service fixture：相对 URL、`src/data-src`、同章重复、多图、alt/FULL 顺序得到稳定规范引用。
2. fetcher：源头/URL option headers 生效；跨站 Cookie/Authorization 被剥离；同源本地书源允许，
   跨站 loopback/private/metadata、DNS 重绑定、第四次重定向、非 HTTP(S)/userinfo 被拒绝。
3. limits/MIME：单图、图片数、单章总量、timeout、HTML 伪图、SVG/空体失败且不留下 temp/blob；
   PNG/JPEG/GIF/WebP/BMP/AVIF 的无扩展 URL 成功。
4. lifecycle：两用户相同 URL得到不同书根；同书重复 URL只抓一次；取消停止读取/下一章；刷新失败
   保留旧引用，成功替换；清缓存、删书、删用户、换源/目录替换删除正确引用且不碰其它书。
5. capability：正常 GET/HEAD、MIME/nosniff；篡改、过期、错误 purpose/blob、所有权变化、文件变更、
   traversal/symlink、另一用户均不可读；access log 不出现 token。
6. chapter API：有 blob 时只增加可选映射，无 blob/失败时映射为空且正文仍保留远程 URL；缓存文件、
   SQLite、WebSocket 不含 capability；前端映射后 alt/FULL/位置和图片预览列表不变。
7. EPUB：已缓存图片出现在 `OEBPS/Images` 和 OPF，章节引用正确；缺图不使导出失败；导出不联网，
   不包含 capability、绝对路径或凭证。
8. Docker 历史卷：旧 TXT/EPUB/UMD/CBZ/相对缓存继续读取；`cache/` 图片根在重启后可用，portable
   backup/restore 不依赖或复制派生图片。

## 实施顺序与发布闸门

1. 添加 service 级 parser/fetch/capability/lifecycle 失败合同和 API/EPUB 合同。
2. 在 `backend/services/chapterimage` 实现有界抓取、原子派生存储、引用与 capability；handler 只映射
   请求/响应。
3. 接入显式整书缓存、章节响应、导出和所有缓存清理路径；不改变前端交互结构。
4. 更新配置/README/API/data/security 文档，运行 targeted Go、Go 全量、前端全量/构建，以及
   1440×900、390×844、360×800 的真实远程图片 Reader/BookManage 合同。
5. 形成可验收切片后立即提交 GitHub；本地构建镜像，跑历史 volume/backup，再决定发布 Docker。

## 2026-07-19 实施与验证结果

- 新增 `backend/services/chapterimage`：按用户/书/规范 URL 的 SHA-256 隔离并复用 blob，章节引用
  清单原子替换；刷新部分失败时保留旧可用引用，成功后才清理无引用文件。
- 图片请求只接受 HTTP(S)；同书源主机保留私有网络书源兼容，跨主机逐次校验 DNS/重定向并拒绝
  私网、回环、链路本地、多播和保留地址。凭证头只发往精确同源，跨源仅保留安全请求头并把
  Referer 降为 origin。
- 默认限制为每章 64 图、单图 8 MiB、单章 32 MiB、12 秒、3 次重定向；只接收实际字节可识别
  的 JPEG/PNG/GIF/WebP/BMP/AVIF。单图超时、超限或类型失败不使正文缓存任务失败。
- `GET|HEAD /api/chapter-image/:capability` 使用用途隔离的短期 HMAC capability，并在读取时重新
  验证用户/书/source、引用、MIME、指纹和 rooted path；访问日志只记录固定前缀。
- 章节 JSON 保持原始 `content` 不变，仅可选返回原 URL 到 capability 的 `cachedImages`；前端先
  计算稳定 `data-pos` 再替换图片 `src`，capability 失败时回退原远程 URL。
- EPUB 仅嵌入已缓存且重新验证过的图片，不在导出期间联网；OPF/XHTML 不含 capability、主机路径、
  查询凭证或源头 URL。单书/全局清缓存、删书、删用户、换源和刷新目录均接入派生根清理。
- targeted Go 合同和 `go test ./...` 通过；前端全量 489/489、生产 build 通过；
  `reader-image-contract.mjs` 在 1440×900、390×844、360×800 验证缓存命中、404 远程回退、位置
  不变和图片/CBZ 几何。
- 实现提交 `32dc6161e4fe559b21855b4b9f963b538098313a` 已推送 `main`。本地 ARM64 镜像通过
  历史 TXT/EPUB/UMD/CBZ、相对缓存、owner 隔离、`data/cache/library` 挂载和 portable
  backup/restore 门禁；图片派生根不修改旧卷或逻辑备份格式。
- 本机生成并上传 `ghcr.io/changshengyu/openreader:32dc616` 与 `latest`；两标签共同指向 OCI
  index `sha256:e5db5dd67e9dafc93803230ec2dba9c4ce09dc39632fcec3d9882b47a6ae781d`。
  AMD64 manifest 为 `sha256:cd0566859e17a7b89f6a739e62d165af8ced674e614c0571996aff8f5f55010b`，
  ARM64 manifest 为 `sha256:80d1c1d89fa7a952b8ec4dc4780e55684409b6af2d500bb4940edd92e970f79a`；
  两个远端标签均由 `docker buildx imagetools inspect` 核验。
