# Reader CBZ 固定基准运行时合同（P0）

状态：**2026-07-18 已完成契约、失败测试、实现、全量自动化、三视口真实浏览器验证、历史卷/portable backup 门禁和本地双架构 Docker 发布。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同取代旧审计中“CBZ 已完成”的宽泛结论。既有 mock 图片 smoke 只能证明 Vue 图片组件的
局部几何，不能证明真实 CBZ archive、资源 API、首次打开、连续翻页或控制状态已经对齐。

## 1. 固定上游可观察合同

| 层 | 上游权威证据 | 固定行为 |
|---|---|---|
| 目录 | `CbzFile.kt#getChapterList()` 遍历 ZIP 非目录项，排除 `.xml`，按 entry name 字典序排序；每项 title/url 都是 archive path。 | CBZ 顺序由 archive path 字典序决定；`ComicInfo.xml` 不进入目录。固定实现实际上也会保留其它非 XML 文件。 |
| 书籍信息与封面 | `CbzFile.kt#parseBookInfo/upBookInfo/updateCover()` 读取 `ComicInfo.xml` 的 Title/Writer，并取 archive 遍历中的第一张支持图片作封面。 | 封面顺序与排序后的第一章是两个不同概念；封面不能改用排序后第一项。 |
| 派生资源 | `BookController.extractCbz()` 仅在 `index` 解压目录不存在或显式 force 时重新解压。 | 正常打开/翻页不为每张图片重新哈希、遍历和解压整本 CBZ。显式刷新可以重建派生目录。 |
| 章节正文 | `BookController#getBookContent()` 检查已解压章节文件；图片扩展返回 `<img src='__API_ROOT__...'>`，其它文件返回静态 URL；缺文件/解压失败显式报错。 | 图片章是一张全宽图片，不显示 CBZ 章节标题；加载失败不能变成空白页。 |
| 图片渲染 | `Content.vue` 的图片行走 lazy 容器；全局 `.content-body img` 为 `width:100%; max-width:100vw; display:block`。`renderScrollChapterList()` 复用同一分支。 | 单章、scroll、scroll2 都保持全宽；图片迟到后重新计算页数。 |
| 两类图片状态 | `Content.vue.isCarToon` 会把 CBZ `<img>` 当图片布局；但 `Reader.vue.isCarToon` 明确 `!isCbz`。 | “图片布局”包含 CBZ；“Reader 普通漫画控制状态”不包含 CBZ，二者不能再共用一个布尔值。 |
| 模式 | `Reader.vue.isSlideRead` 只因 EPUB、普通图片漫画、音频、自动阅读或 read bar 关闭 slide；CBZ 自身不强制 page。scroll 规则同样不排除 CBZ。 | CBZ 保留用户选择的 flip/page/scroll/scroll2；普通网页图片漫画仍强制竖向 page。自动阅读或 TTS read bar 打开时 flip 临时进入竖向 page。 |
| 自动阅读/TTS | 两个按钮条件均为 `!isEpub && !isCarToon && !isAudio`；这里的 Reader `isCarToon` 排除 CBZ。 | CBZ 显示自动阅读和浏览器 TTS 入口；EPUB、普通图片漫画和音频隐藏。CBZ 没有可朗读文本时可得到正常“无内容”结果，但入口状态不能被错误隐藏。 |
| 位置 | CBZ heading 被隐藏，但 `Content.vue` 仍从 `title.length + 2` 计算图片行 `data-pos`；图片 lazy load 触发 `computePages()`。 | 图片块保留稳定位置，flip 以渲染页记录进度；重排不能让当前页跳回开头。 |

## 2. 当前 OpenReader 映射与判定

| 关注点 | 当前证据 | 判定 | 目标 |
|---|---|---|---|
| parser 顺序/元数据 | `engine/cbz_parser.go` 保留支持图片，字典序建章；封面是 archive 首张图片；解析 ComicInfo Title/Writer。 | `aligned + security adaptation` | 保持顺序和封面差异。非图片项不进入目录，作为安全/质量修正显式保留，避免暴露任意 ZIP 文件和上游“URL 当正文”的缺陷。 |
| archive 安全 | parser/resource 拒绝 NUL、反斜杠、绝对/盘符/`..`、symlink、大小写冲突和解压预算超限。 | `acceptable-change` | 不为复刻上游公开解压目录而放松。 |
| 资源运行时 | `cbzreader` 现在使用 user/book 私有的不可变 `.cbz-resources/<fingerprint>/`；完整 marker 的 size/mtime 命中时不再重哈希或重开源 CBZ，资源直接映射到 allow-listed 派生图片。 | `aligned technical equivalent` | 保持一次有界安全解压和 capability 流式读取。 |
| 首次打开 | importer 在新分配 archive 内调用 `PrepareBookResources`；数据库失败沿用整目录补偿。旧书没有派生树时仍惰性创建。 | `aligned technical equivalent` | 保持原 archive、rows 和备份格式不变。 |
| 资源响应 | `GET/HEAD /api/cbz-resource/...` 通过标准文件响应服务派生图片，支持 Content-Length、Last-Modified、Range/206/416；不再整张读入 Go 内存。 | `aligned technical equivalent` | 保持 capability、headers 和 client-safe error。 |
| 原 archive 与 capability | 当前 capability 绑定 user/book/fingerprint/expiry，资源 path 再归一化；source bytes 是权威数据。 | `aligned technical equivalent` | capability 可读取对应已完成的不可变 fingerprint；source 缺失时已存在的已签名派生版本在有效期内仍可读。source 变化产生新 fingerprint，旧能力不能切到新内容。 |
| 前端图片布局 | `ReaderChapterContent.vue` 为 `isComic` 图片全宽，CBZ 隐藏 `h3`；image load 通知 Reader 重排。 | `aligned` | 保持 Element Plus preview/lazy 适配和点击阻断。 |
| Reader 控制状态 | `comicPresentation`/`isComicChapter` 继续负责图片布局；`isOrdinaryImageComicChapter` 和共享 capability helper 单独负责 mode、自动阅读与 TTS。CBZ flip 开启 auto/read-bar 临时转 page，关闭恢复持久 flip。 | `aligned` | 保持两类图片状态分离。 |
| 真实浏览器证据 | `scripts/smoke/reader-cbz-contract.mjs` 使用真实 Go、真实 multipart import、ComicInfo、archive-first cover、字典序目录和非图片成员；已通过 1440×900 page、390×844/360×800 scroll 和 390×844 flip。 | `verified` | Docker 发布前继续作为浏览器闸门。 |
| 数据兼容 | 原 CBZ、chapter resource path、portable backup 已存在；历史卷能惰性恢复 path。 | `must-preserve` | 不迁移/重写 archive 和 rows；`.cbz-resources` 是可删除派生数据，portable backup 仍只携带原 CBZ。 |

## 3. 状态机合同

```text
新 CBZ confirm
  -> 原 archive 已安全复制
  -> 一次有界解析/不可变资源准备
  -> SQLite + chapters.json 提交
  -> 首章直接签发 capability

旧 CBZ / 派生目录缺失
  -> 保留旧 rows
  -> 首次请求一次重建当前 fingerprint
  -> 回填缺失 resourcePath（不改变章节数）

已完成 fingerprint + 有效 capability
  -> 校验 user/book/purpose/expiry/path
  -> 直接打开派生图片
  -> 不读、不哈希、不遍历原 CBZ

原 CBZ 变化或显式 refresh
  -> 新 fingerprint / 新派生 generation
  -> 原子替换目录 rows 后清理无引用旧派生资源
  -> 旧 capability 不得映射到新 bytes
```

Reader 可见状态：

```text
CBZ 图片布局 = true
普通图片漫画状态 = false
用户 mode = flip/page/scroll/scroll2（保留）
auto-reading 或 TTS read bar 打开 + mode=flip -> 临时 page
关闭后 -> 恢复用户持久 mode
```

## 4. API 与数据合同

- 公开章节 API 不改路径和既有 JSON：`format:"cbz"`、`content`、`resourceUrl`、
  `resourceExpiresAt` 保持兼容。
- `/api/cbz-resource/:capability/*resourcePath` 继续无需登录 JWT；能力只绑定一个 user、book、
  fingerprint、purpose 和期限。path 必须是该不可变目录内的 allow-listed 图片。
- 派生目录固定在 `library/<Book.LibraryPath>/.cbz-resources/<sha256>/`，通过完整 marker 后才激活；
  临时目录必须同父级原子 rename，失败即清理。
- SQLite、`chapters.json`、Book/Chapter schema、原 CBZ、WebDAV/LocalStore source 和 backup 格式不变。
- portable backup 仍只复制原始 `.cbz`；恢复到空卷后必须能惰性重建派生目录。
- 删除书籍、显式刷新和失败补偿只能清理当前书根内派生资源，不能越过 user/book root。

## 5. 先失败的契约测试

| 编号 | 必须先失败的断言 | 层 |
|---|---|---|
| CBZ-FIX-1 | 新 confirm 产生完整 `.cbz-resources/<fingerprint>`；数据库失败不留下 archive/派生目录。 | importer/data |
| CBZ-FIX-2 | warm `PrepareChapter` 与同 capability 的 GET/HEAD/Range 不再次调用 fingerprint、不重开原 CBZ；移走 source 后已签名 immutable resource 仍可读。 | cbzreader/API |
| CBZ-FIX-3 | 缺失派生图片且 source 可用时只重建一次；source identity 变化后旧 capability 403，新请求返回新 fingerprint。 | cbzreader/security |
| CBZ-FIX-4 | parser 保持 ComicInfo、archive-first cover、图片字典序、非图片排除；历史空 resourcePath 按 index 回填。 | engine/data |
| CBZ-FIX-5 | CBZ 保留 mode；自动阅读/TTS 入口可见。普通图片漫画和 EPUB 隐藏两入口；CBZ flip 开启 auto/read-bar 临时转 page，关闭恢复 flip。 | frontend state |
| CBZ-FIX-6 | 真实 Go 导入含 ComicInfo、乱序多图和非图片 fixture；三视口验证目录顺序、无标题、全宽/16px 对称、preview 不穿透、延迟图片不跳页、无 4xx/5xx/console error。 | real browser |
| CBZ-FIX-7 | 旧 volume 无派生目录可读；显式 refresh 不改 archive hash；portable backup 到空卷、恢复、重启后仍可重建。 | Docker/backup |

## 6. 允许差异与非目标

- 保留 capability、私有 library root、路径/ZIP budget、MIME allow-list 和不公开任意非图片 entry；
  这些是多用户/安全适配。
- 保留 Vue 3、Element Plus lazy/preview 和用户要求的原生连续手指/滚轮滚动；点击翻页仍离散。
- 本批不顺带签收 audio、TTS 引擎内容、在线漫画源解析或普通 EPUB 图片；只修复 CBZ 与
  Reader 图片状态共用造成的合同偏差。
- 现有 mock 图片 smoke 继续作为普通图片布局回归，但不能再作为 CBZ 完成证据。

## 7. 实施顺序

1. 先写 CBZ-FIX-1…5 的失败测试。
2. 重建 CBZ 不可变派生资源运行时和 importer 补偿。
3. 分离 Reader 图片布局状态与普通漫画控制状态。
4. 新增真实 Go CBZ smoke，跑三视口与历史 volume/portable backup。
5. 全量 Go、前端测试、生产构建通过后，本地构建并决定中途 Docker 发布。

## 8. 2026-07-18 实施证据

- CBZ-FIX-1…4：`backend/services/cbzreader/service_test.go`、
  `backend/services/localbook/importer_test.go`、`backend/api/api_test.go` 和既有 engine/历史卷测试。
- CBZ-FIX-5：`frontend/tests/readerMode.test.mjs`、`readerTTS.test.mjs` 与全量 427 项前端测试。
- CBZ-FIX-6：`scripts/smoke/reader-cbz-contract.mjs` 在 1440×900、390×844、360×800，另加
  390×844 flip，真实资源请求无 4xx/5xx/console error；截图已人工检查。
- 当前全量 `go test ./...`、`npm test`（427 项）、`npm run build` 已通过。
- CBZ-FIX-7 已由 `HISTORICAL_VOLUME=1` Docker 门禁验证：旧 SQLite 卷中的 TXT、EPUB、UMD、
  CBZ 和相对 cache 均可读取/刷新，原 archive 哈希不变，用户隔离成立；portable backup 导出到
  空卷、恢复、重启后仍可重建派生 CBZ 资源。
- 源码 commit `d1bc05af14f8a6b930f396fb87071c910753f1ed` 已在本机完成 `linux/amd64` 与
  `linux/arm64` 构建，并发布为 `ghcr.io/changshengyu/openreader:d1bc05a` 与 `latest`。两 tag
  的远端 OCI index digest 均为
  `sha256:97c5fe40e7c6d66a2bca6d98939000862b9c7ca835fb5c039c3c36be05017f23`；包含
  `linux/amd64@sha256:bd5c1765d9fd4f16c8a3f086a6a8a37da5d0558554941aff4e958fb28c74e75d`
  和 `linux/arm64@sha256:d7e4d522528d1583e59831dcbd07a1b1d030eae8a0cb07183794ddb1e8eace1a`。
