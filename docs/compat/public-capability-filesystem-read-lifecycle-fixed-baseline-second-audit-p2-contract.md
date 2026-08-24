# 公开 capability 文件读取生命周期固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前审查基线：`OpenReader@910cb38`  
审查日期：2026-08-17  
状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

## 1. 范围与权威证据

本轮继续对 `backend/api/server.go` 的公开文件读取面做 route/path/work 差集，只覆盖：

- `GET|HEAD /api/epub-resource/:capability/*resourcePath`；
- `GET|HEAD /api/cbz-resource/:capability/*resourcePath`；
- `GET|HEAD /api/audio-resource/:capability/*resourcePath`；
- `GET|HEAD /api/cover/:capability` 的已落盘封面缓存命中路径。

固定上游通过 `YueduApi.kt:96-122` 的 `/assets/*`、`/epub/*` 静态资源，以及
`BookController.kt:535-573` 的 EPUB/CBZ 章节投影让浏览器读取本地/派生资源。OpenReader 的
私有书根、不可变 EPUB/CBZ generation、短期用途隔离 capability、CSP、MIME allowlist 和安全封面代理
是已签收的多用户/安全适配；本轮不恢复上游公开目录，也不改变 Reader 的可见章节状态机。

当前权威文件：

- `backend/api/epub.go`、`cbz.go`、`audio.go`、`cover_image.go`；
- `backend/services/epubreader/service.go`；
- `backend/services/cbzreader/service.go`；
- `backend/services/audioreader/service.go`；
- `backend/services/coverimage/storage.go`、`service.go`。

`/api/chapter-image/:capability` 不并入本切片：它在响应前按 capability 中的 blob fingerprint 再验证
最终内存字节，且没有 handler path reopen。公开 `/uploads/*` 已由独立合同签收。本轮也不重开 archive
解析、提取预算、章节目录、封面抓取 SSRF 或正文图片缓存引用生命周期。

## 2. 固定可见合同

| 路由 | 必须保留的成功语义 | 必须保留的授权/错误语义 |
|---|---|---|
| EPUB | XHTML 仍动态清理并注入 bridge/CSP；CSS、图片、字体继续支持相对 URL；GET/HEAD 与已有 MIME、cache/security headers 保持。 | capability 继续绑定 user/book/fingerprint/document fragment/expiry；400/403/404/415/422 的既有安全 envelope 不变。 |
| CBZ | 已完成 generation 的图片继续流式返回，保留 Content-Length、Last-Modified、HEAD、Range/206/416。 | capability 继续绑定 user/book/fingerprint/expiry；旧 generation 在源文件暂时缺失时仍可读，错误保持 400/403/404/415/422。 |
| 本地音频 | GET/HEAD/Range 继续支持浏览器 seek，MIME 与 private/no-referrer/nosniff headers 不变。 | capability 继续绑定 user/book/resourcePath/fingerprint/expiry；远程 HTTP(S) 音频仍走既有直接播放合同。 |
| 封面缓存 | 缓存命中继续返回已验证 raster bytes、精确 Content-Length/MIME 和既有 private headers；缓存 miss 仍可走 SSRF-safe 抓取。 | capability 继续绑定 user/规范化 URL/expiry；无效 token 403，unsafe/unavailable 404，意外存储错误 500。 |

`HEAD` 不得读取响应 body。流式路由必须继续由标准库处理 conditional/range 语义，不能退化为把整本
archive 或整个媒体文件读入内存。

## 3. 当前实现差异

| 面 | 当前实现 | 裁决 |
|---|---|---|
| EPUB document | `resourceFile` 先 `Stat/EvalSymlinks` 返回路径，随后 `OpenResource` 再 `os.Open(path)` 读取 XHTML。 | **must-fix**：授权/路径验证与实际读取不是同一 opened file identity。 |
| EPUB asset | service 返回 `Resource.Path`，handler 再交给 `http.ServeFile` 按路径重新 stat/open。 | **must-fix**：mounted entry/ancestor 可在验证后替换；响应必须消费 service 已验证的同一句柄。 |
| CBZ image | `resourceFile` 返回已解析路径，handler 的 `ServeFile` 再打开。 | **must-fix**：不可变 generation 是逻辑合同，不是 mounted path 身份保证。 |
| local audio | service 对路径执行 `Stat` 和一次 fingerprint open，返回 path；handler 再 `os.Open`。 | **must-fix**：第二次打开没有与 capability fingerprint 对应的 same-file 证明。 |
| cover cache | `Lstat(path)` 后 `os.Open(path)`，未比较 `os.SameFile`；读取后又按 path `Chtimes`。 | **must-fix**：替换窗口可读取或 touch 另一个 mounted regular image。 |
| chapter image | 最终 bytes 重新校验 MIME、大小和 capability fingerprint。 | **technical-stack-equivalent / out of scope**；后续若改变为流式响应必须重新签约 opened-file identity。 |

这些差异不要求能通过普通 HTTP 参数选择任意路径。威胁边界是已挂载 `library/`、`cache/` 可被宿主、
同步工具或并发维护任务替换时，一个已授权 capability 不能在验证后切换到另一个文件对象。普通缺失、
损坏或被替换资源必须 fail closed，错误中不得出现 capability、主机路径或源 URL 凭证。

## 4. 目标 opened-file 合同

1. 每个流式资源在 service 完成 capability、owner、generation/source identity、规范路径和 MIME 检查后，
   返回一个由调用方负责关闭的已打开普通文件句柄及其同句柄 metadata；不得只返回 path。
2. 打开必须以已经确认的 caller/book/generation/cache root 为边界，逐组件拒绝越界与 mounted symlink，
   并以 `Lstat/open/fstat/SameFile` 或等价 rooted API 证明校验对象就是响应对象。
3. EPUB XHTML 的有界读取、清理和 bridge 注入必须从该已验证句柄完成。非 document EPUB、CBZ 和
   音频由 `http.ServeContent` 消费同一句柄，保留标准 HEAD/Range/conditional 行为。
4. 封面缓存继续有界读入内存并验证 raster MIME；`Lstat` 身份必须与 opened handle 一致。LRU 时间更新
   只能作用于同一已验证对象；身份已变化时可以跳过 touch，但不能操作替换对象。
5. 目录、FIFO/socket/device、root/ancestor/entry symlink 和验证后替换对象均 fail closed。已有 mounted
   对象原样保留，不删除、不重写、不迁移；请求失败不得触发 archive 重写或跨书清理。
6. 服务返回错误后 handler 必须关闭已取得的句柄；正常、HEAD、Range、客户端断开和 ServeContent
   早退路径也必须恰好关闭一次。

## 5. 数据与兼容边界

- 不新增 SQLite 表、列、索引、migration 或启动扫描。
- 不改变 `Book`/`Chapter` 字段、原 archive、`.epub-resources/<fingerprint>`、
  `.cbz-resources/<fingerprint>`、本地音频路径或 `cache/cover-images` 布局。
- 不改变 capability 编码、purpose、TTL、URL、响应 JSON、备份/WebDAV/portable 格式或环境变量。
- historical volume 的合法 regular file 和完整 marker 继续惰性可读；旧卷无需重建派生目录。
- unsafe mounted symlink/special file 只在 HTTP 读取面不可见，仍由宿主或原管理面负责处理。

## 6. 测试先行门

1. API/service 旧实现红测：对 EPUB asset/XHTML、CBZ、audio 和 cached cover 分别锁定 root、ancestor、
   entry symlink、目录/FIFO 和普通文件成功；失败响应 path-free，外部 sentinel 不被读取或 touch。
2. 确定性 replacement 测试：在 capability/owner/path 验证完成、响应读取前替换 mounted entry；旧实现
   必须证明会消费 replacement，目标实现必须只消费已验证句柄或安全失败。
3. 文件身份/关闭测试：service 返回的 metadata 与 handle `Stat` 为同一对象；正常、HEAD、Range、
   sanitize failure 和 handler error 都不泄漏 descriptor。
4. HTTP 回归：EPUB CSP/bridge/relative asset、CBZ GET/HEAD/Range/416、audio GET/HEAD/Range seek、cover
   hit/miss/304-equivalent client behavior 和既有固定错误状态不退化。
5. 并发/race：generation rebuild/prune、source replacement、cover cache refresh/eviction 与并发 GET 不得
   读到跨 generation、跨书、跨用户或 root 外对象。
6. runtime：真实 Go + Chromium 在 1440x900、390x844、360x800 验证 EPUB、CBZ、audio 与封面请求无
   新 4xx/5xx；mounted probe 验证 unsafe object 原样；fresh/historical/portable volume 和 restart 通过。

## 7. 非目标与允许差异

- 不改变 Reader UI、EPUB iframe、CBZ 图片布局、音频 controls、封面回退或 capability 时长。
- 不重开已签收的 archive extraction、source fingerprint、EPUB fragment、CBZ cover、远程音频 URL、
  cover SSRF 和 chapter-image contracts。
- OpenReader 继续比固定上游更严格地使用私有 root、用途隔离 capability、MIME/size/CSP 和多用户
  owner 校验；rooted same-file open 是该安全适配的补完，不是产品行为分叉。

## 8. 实施顺序

1. 单独提交本合同和总矩阵，不修改应用或测试。
2. 在 `OpenReader@910cb38` 旧实现上提交可复现的 red tests。
3. 复用仓库现有 rooted opened-file helper/pattern，实现四路同句柄读取并保持 API surface。
4. 运行 focused/race/full/vet、frontend 全量/build、真实浏览器/mounted probe 与卷门。
5. 形成可验证切片后提交并推送；只用本机构建 amd64/arm64 并发布 GHCR。

## 9. 测试先行与实现证据

- `2587299` 只提交本合同和总矩阵，固定 upstream/OpenReader 差集未混入代码改动。
- `df49535` 在旧实现上加入确定性 replacement 红测：EPUB/CBZ handler 会在授权后重开 mounted
  replacement，audio service 返回的 path 可被调用方重开为 replacement，cached cover 会按 path touch
  replacement。
- `a90f7b3` 让 EPUB/CBZ/audio service 返回同一 rooted、symlink-rejecting、`SameFile` 验证的 opened
  regular handle 与 metadata；handler 只用该句柄 `ServeContent` 并拥有关闭责任。EPUB XHTML 的有界
  sanitize/bridge 读取也改为消费同一句柄。cached cover 用同一 verified handle 有界读取，并只在 path
  仍指向同一普通文件时 touch/remove；unsafe mounted 对象原样保留。
- 路由、capability claim/purpose/TTL、CSP、MIME、private headers、HEAD/Range、generation、archive、
  SQLite 与 `data/cache/library` 布局均未改变。

## 10. 当前验证边界

focused 正常/替换/symlink 回归、affected packages、focused `-race`、`go test ./...`、`go vet ./...`、
frontend 741/741 与 Vite build 已通过。真实 Chromium 中 EPUB、audio UI 和真实 Go/API cover proxy
均通过 1440x900、390x844、360x800；CBZ 通过 desktop page、390/360 scroll 和额外 390 flip，所有
capability 请求无 4xx/5xx。CBZ 先前 `page`/`scroll` 失败已由 `a0c0206` 证明为 smoke 未关闭
`autoTheme`，`5313c49` 显式冻结该测试前置条件，未改变 Reader 产品行为。

宿主与 `5313c49` 候选容器均确认 CBZ HEAD 200、Range 206/正确 `Content-Range`；派生页替换为指向
`/etc/hosts` 的 symlink 时返回既有 path-free 400，mounted symlink 未被消费，恢复原页后再次 200。
本地 `PUSH=0` 候选镜像 label revision 为
`5313c49f6a18b3cce769ea03e4f8cdf8fddafebe`。fresh 卷通过 portable v1/v2 assets、cross-user、restart；
historical 卷通过 TXT/EPUB/UMD/CBZ、relative-cache、owner isolation。正式本机 amd64/arm64 GHCR 发布
已在最终证据提交 `5e63eb1` 上完成。

## 11. Docker 发布结果（2026-08-24）

- 本机 OrbStack/BuildKit 构建并上传 `ghcr.io/changshengyu/openreader:5e63eb1` 与 `latest`；两 tag 远端
  回读为同一 OCI index
  `sha256:8b7bc4cd8542f79eccc54d393cf2d79041f5fe9a90b05776c473cd3f1e4c2cee`。
- linux/amd64 manifest 为
  `sha256:f5ac952ead7e410c3debe7c2892f84cc5d6e0212a3982c3cd18209e084186c3d`，linux/arm64 manifest 为
  `sha256:a7a243fe0bc4083ebdb6f4f1df5206315a6d3cbac05544525544e44244663d76`；两平台 config 均确认完整
  revision `5e63eb1854a95dc3fd79ebafec89f4723f37f8da`。
- 从远端 tag 强制拉取 arm64 后，`/api/health` 返回 `status:"ok"`、`version:"5e63eb1"` 和同一完整
  revision。发布前门禁包括 Go full/race/vet、frontend 741/741、Vite build、EPUB/CBZ/audio/真实 cover
  三视口、宿主/候选 mounted probe、fresh/historical/portable/restart。
- 允许差异仍限于私有用途隔离 capability、rooted same-file open、MIME/CSP/Range 和多用户 owner；
  本切片没有新增未完成的数据迁移或格式工作，真实设备签收仍待用户生产环境升级后确认。
