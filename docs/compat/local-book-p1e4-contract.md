# P1-E4 本地书真实格式与旧挂载卷兼容合同

状态：**TXT、UMD、CBZ、PDF/Markdown 遗留兼容和大部分旧卷门禁已完成；E4-EPUB-2 的固定目录纠正已实现并通过自动化/三视口浏览器验证，Docker/历史 volume 门禁进行中。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同是 P1-E1、P1-E2、P1-E3 后的格式与持久化门禁；它不以当前
OpenReader 的 parser、测试或 UI 为正确性的依据。实现顺序固定为：补夹具和
失败断言、补真实浏览器/旧卷回归、最后才修改实现。

## 1. 上游证据与当前映射

| 范围 | 上游权威证据 | 当前 OpenReader 映射 |
|---|---|---|
| 本地格式分派、文件名元数据 | [`LocalBook.kt`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/io/legado/app/model/localBook/LocalBook.kt) | `backend/services/localbook/importer.go`、`backend/engine/*_parser.go` |
| TXT 编码、目录和正文 | [`TextFile.kt`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/io/legado/app/model/localBook/TextFile.kt) | `backend/engine/txt_parser.go`、`backend/services/localbook/importer.go` |
| EPUB 目录和资源内容 | [`EpubFile.kt`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/io/legado/app/model/localBook/EpubFile.kt)、[`BookController.kt`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/com/htmake/reader/api/controller/BookController.kt) | `backend/engine/epub_parser.go`、`backend/services/epubreader/*`、`ReaderEpubContent.vue` |
| UMD | [`UmdFile.kt`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/io/legado/app/model/localBook/UmdFile.kt) | `backend/engine/umd_parser.go` |
| CBZ、漫画资源 | [`CbzFile.kt`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/io/legado/app/model/localBook/CbzFile.kt)、`BookController.kt` | `backend/engine/cbz_parser.go`、`backend/services/cbzreader/*`、`ReaderChapterContent.vue` |
| 预览、确认、书仓路径 | `BookController.kt#importBookPreview`、`#importFromLocalPathPreview`、`#getBookContent` | `backend/api/imports.go`、`backend/api/local_import_stage.go`、`backend/api/books.go`、`backend/api/localstore.go`、`backend/api/webdav.go` |
| 阅读格式分支 | [`Content.vue`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/web/src/components/Content.vue)、[`Reader.vue`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/web/src/views/Reader.vue) | `ReaderChapterContent.vue`、`ReaderEpubContent.vue`、`useReaderChapterPresentation.js` |

审查使用上述固定提交的 Git blob，而不是不可用的本地空目录副本。

## 2. 已提取的上游行为

1. 直接上传和本地路径预览只分派 `txt`、`epub`、`umd`、`cbz`；目录为空会捕获
   `TocEmptyException`，返回包含书籍信息、`chapters: []` 的正常预览，而不是把
   “没有目录”变成传输/解析失败。
2. TXT 首先读取 512000 字节探测编码和默认目录规则。无目录时按约 10 KiB 在换行处
   生成伪章节。`TextFile.kt` 中虽有约 100 KiB 的规则目录分支，但固定基准的
   `Book.getSplitLongChapter()` 固定返回 `false`，所以该分支默认不可达；正文从原始
   字节范围读取，BOM、编码和多字节边界均属于兼容面。
3. EPUB 空规则默认为 `spin+toc`；`toc`、`spin`、`spin<toc`、`spin+toc`、
   `toc+spin`、`toc<spin` 都是公开行为。固定本地 EPUB 目录按 href 去重，不保存 TOC fragment；
   Web Reader 每次显示一个解压资源（含封面页），不把 XHTML 简化为普通文本或跨 resource 合成。
4. UMD 使用标准 `0xde9a9b89`、UTF-16LE/压缩分段格式；每个 UMD 标题对应一个章节。
5. CBZ 从 `ComicInfo.xml` 取标题/作者；每个归档条目对应漫画章节，阅读端返回图片
   内容并隐藏普通正文标题。
6. 上游直接解压 EPUB/CBZ 到派生 `index` 目录。OpenReader 不应复制其不受限解压、
   任意归档条目暴露或跨用户路径行为。

## 3. 差异矩阵与裁决

| 合同层 | 上游行为 | 当前证据 | 裁决 | P1-E4 必须证明/处理 |
|---|---|---|---|---|
| TXT 自定义规则无匹配预览/确认 | 上游预览正常返回空章节；`BookController.saveBook` 对本地书不要求目录非空，因此用户可保留空目录书籍，之后再按规则刷新。 | `Importer.Preview`/`Import` 返回空章节/零章节书；直接、LocalStore、WebDAV UI 显示可恢复说明并保持同一个 stage token。 | **aligned（E4-TXT-2 已完成）** | API 仅消费当前用户确认的 stage；不制造虚假 chapter，错误格式仍保持失败。 |
| TXT 规则目录长章节 | 固定基准的 `Book.getSplitLongChapter()` 返回 `false`，因此不会启用 `TextFile` 内约 100 KiB 的可选分支。 | `TestDirectTXTEncodingAndLongRuleCatalogRemainReadableAcrossStageImport` 用超过 100 KiB 的显式规则首章验证 preview→token import→删除派生 cache→正文恢复，章节未隐式拆分或跨章合并。 | **aligned（E4-TXT-1）** | 不得无依据引入新的切分设置。 |
| TXT 无目录/编码 | 512000 探测、10 KiB 伪章节、BOM/编码字节读取。 | 同一端到端测试覆盖 UTF-8 BOM、GBK、GB18030 的直接 staged import；`TestStorageGB18030NoTocStageImportRemainsReadable` 覆盖 LocalStore/WebDAV 预览后删除挂载源、token 确认、删除派生 cache 后从归档恢复。 | **aligned（E4-TXT-1）** | BOM 不得进入正文；同一字节的重新解析仍由既有 E1/E3 staged-token 测试覆盖。 |
| EPUB 目录规则 | `toc` 按首次 href 顺序去重，包含合法 TOC-only resource，重复引用最后一次非 `null` title 写回 resource（空串回退 XHTML `<title>`）；纯 `toc` 无有效目录时返回空。`spin` 单独用 spine/document title。其余含 TOC 的混合规则先读 TOC，故 spine 也观察到 TOC 写回标题；混合规则在一侧为空时回退。空规则默认 `spin+toc`。 | 新 parser、preview/import、`chapters.json` 和显式 refresh 已按上述实际执行结果生成；新 row fragment 为空，旧 staged snapshot/旧 fragment rows 继续可读。 | **aligned（EPUB-FIXED-1..5）** | 本地镜像的历史 volume/portable backup 门禁仍待完成。 |
| EPUB 首个封面 spine 资源 | `EpubFile.getChapterListBySpine()` 不因正文为空丢弃首个资源；标题为空的首项命名为“封面”，`titlepage.xhtml` 由阅读端作为封面文档处理。 | `ParseEPUBWithLimits` 现保留每一个可读 spine XHTML；首个无标题资源命名为“封面”。解析、preview、导入、chapter API、受控 capability 与 1440/390/360 iframe 都验证了 `OPS/Text/titlepage.xhtml`。 | **aligned（E4-EPUB-1 cover 子项已完成）** | 不做旧书的破坏性自动迁移；`refresh-local` 已按新 parser 重建目录。E4-VOLUME-1 还需验证旧数据库/archive 与缺失封面 chapter 的恢复/刷新路径。 |
| EPUB 阅读资源 | 固定 Web API 为每个 href 去重后的目录项返回一个解压 XHTML URL；单 iframe 不拼接 resource。`EpubFile#getContent` 的跨 resource 分支用于其它辅助路径，不能代表 Web Reader。 | 单 capability iframe、整 resource 正文/搜索、相对资源、跨 XHTML Reader 事务和手机返回书架均已三视口验证；旧 fragment rows 仍走历史 slice。 | **aligned + intentional search correction** | 不新增跨-resource DOM 合成；继续保留 capability/CSP。 |
| 标准 UMD | `UmdFile` 通过 `UmdReader` 从原始 UMD 取得有序标题，并按章节索引重新读取正文；刷新本地书时重新生成目录。 | `umd_parser_contract_test.go` 使用上游写入 framing；`TestReaderDevUMDRebuildsArchivedChaptersAndRefreshes` 已验证标准 archive 在 cache 缺失时重建、刷新后目录/正文不变且 archive 未改写；LocalStore/WebDAV 在 preview 后移除挂载源、确认后也完成同一重建。`TestLegacyPseudoUMDArchiveRebuildsWithoutMigration` 证明已有 `#TEXTNOV` archive 在无 cache 时只读恢复。 | **aligned（E4-UMD-1 已完成）** | 不扩张 pseudo-UMD 为新格式、不自动迁移已有书；E4-VOLUME-1 仍要覆盖实际旧 SQLite 卷中的绝对路径、章节 metadata 及其他格式。 |
| CBZ 章节列表和封面 | 上游忽略 XML 后按字典序生成目录；`ComicInfo.xml` 提供标题/作者；遍历 archive 时遇到的**首张**支持图片作为书籍封面（封面选择不等于目录排序首项）。 | 当前只接受规范化后的安全图片条目；`ParsedBook.CoverResourcePath` 保留首个安全图片而不影响有序目录。书架、导入响应和详情响应按当前用户/书/archive fingerprint 动态投影同源 capability。 | **aligned（E4-CBZ-1 已完成；安全收紧）** | capability 仅存在于响应；SQLite、archive、备份/WebDAV metadata 和同步存储均保持原格式。`CustomCoverURL` 优先；archive 异常时书架正常返回空封面。 |
| PDF、Markdown、`.text` | 上游工作台导入并不提供这些格式。 | 直接可见 UI 现只给 TXT/EPUB/UMD/CBZ；旧 direct API 和已导入 archive 仍解析 `.text/.md/.pdf`。P1-E3 书仓 UI 不展示它们。 | **aligned UI + 明确的遗留数据/API 兼容差异（E4-PDFMD-1）** | 详见 [`pdf-markdown-p1e4-contract.md`](pdf-markdown-p1e4-contract.md)：历史 archive/阅读/刷新不破坏，未经新合同不得加入 LocalStore/WebDAV。 |
| 预览的文件保存 | 上游会写临时/导入路径，且目录为空也可预览。 | 当前用用户范围、不可变 stage token；预览成功前无书架写入。 | **acceptable-change（多用户/安全）** | 空目录与失败重试都必须保留同一用户 token；不能因挂载卷、WebDAV 或网络变化重新读取原路径。 |
| `library/` archive 位置 | 上游 `storage/data/<namespace>/<name_author>/` 及派生 `index`。 | 当前 `library/data/<user>/<safe-name>/` 存 `OriginalFile`、`chapters.json`、`bookSource.json` 和 `content/`。 | **technical-stack-equivalent，待旧卷验证** | 不迁移或删除既有目录。验证相对/绝对旧字段、缺失 content cache、旧 `ResourcePath` 与 archive 文件仍可恢复。 |
| 新旧资源限制 | 上游没有 ZIP/解压/文本上限。 | 新导入受严格上限；旧 archive 使用较宽但有界的 `LegacyLocalBookParseLimits`。 | **acceptable-change（安全）** | 新输入拒绝必须在归档/DB 写入前发生；旧卷恢复必须仍受界且可读，不得突破用户隔离。 |

## 4. 先写的回归夹具和测试

以下测试先于任何实现修改加入；夹具仅可含可再分发的自建最小内容，不提交受版权保护书籍。

| 编号 | 夹具与断言 | 覆盖的入口 |
|---|---|---|
| E4-TXT-1 | **已完成**：自建 UTF-8 BOM、GBK、GB18030 无目录输入和超过 100 KiB 的显式规则首章。断言 preview、token import、章节边界、缓存删除后正文恢复、BOM 不泄露；LocalStore/WebDAV 在源文件删除后仍从 scoped stage/archive 恢复。 | 直接上传、LocalStore、WebDAV、reader content |
| E4-TXT-2 | 显式规则无匹配：预览 `200` + 空章节 + 原 token；界面展示可恢复空态；确认可创建零章节本地书，或重新解析后创建正常目录。 | importer、`/preview`、OverlayBookImport、OverlayStorageImport |
| E4-EPUB-1 | 六种规则与纯图片 `titlepage.xhtml` 已实现：预览、导入和 iframe 都保留 capability 保护的封面资源。 | parser、import、资源 capability、iframe real browser |
| E4-EPUB-2 | **历史能力已发布，新目录断言待纠正**：Git `8f5e979` 已交付旧 fragment row slice、跨 XHTML 安全跳转、相对资源和 capability；其中“新解析保留同 XHTML 多目录项”经固定基准复审判定为错误。EPUB-FIXED-1..6 将替换 parser/import/refresh 断言，同时保留旧数据可读。 | parser、import、资源 capability、iframe real browser、migration/security |
| E4-UMD-1 | **已完成**：自建标准 `89 9b 9a de` fixture 已覆盖各入口 preview→确认导入→删除 `content/` cache→正文由 archive 重建；直接上传另验证 `POST /refresh-local` 不改 archive，且标题、索引、正文保持一致。自建 `#TEXTNOV` fixture 仅作为已有历史 archive，验证无 cache 的惰性恢复且不触发重导入/迁移。 | direct、LocalStore、WebDAV、reader content、refresh-local |
| E4-CBZ-1 | **已完成**：自建 archive-entry 顺序与目录排序不同的最小图片 CBZ。`TestParseCBZKeepsFirstArchiveImageAsCoverSeparateFromSortedCatalogue` 断言首图和排序目录分离；`TestDirectCBZImportAndResourceCapability` 断言导入、书架、详情均可读取同源 capability，SQLite 不保存 capability，`CustomCoverURL` 保持优先。`reader-image-contract.mjs` 覆盖桌面、390×844、360×800 和移动工具层/图片布局。 | parser、import、书架、BookInfo、`/api/cbz-resource`、移动/桌面 Reader |
| E4-PDFMD-1 | **已完成并发布**：可见 direct UI 收敛为 TXT/EPUB/UMD/CBZ；文本 PDF、扫描/无文本 PDF、Markdown 和 `.text` 的历史 API/archive 阅读、刷新、cache 回建、失败无持久写入与跨用户隔离均有合同测试。Git `d0a0f5b`；GHCR `:d0a0f5b`/`:latest` 的多架构 index digest 为 `sha256:b55e119fbb272065f1c8b447d783a371d00c633f183f583f987d7471aab0914d`。详见 [`pdf-markdown-p1e4-contract.md`](pdf-markdown-p1e4-contract.md)。 | 直接上传 UI/API、历史 archive 阅读/刷新；错误状态不得创建书架/archive |
| E4-VOLUME-1 | **进行中，第三批已发布并扩展验证**：旧 SQLite 缺列、progress/bookmark、历史相对/绝对 `OriginalFile`/`CachePath`、私有 archive root 与安全 cache migration 均有失败夹具和实现；EPUB/UMD/CBZ/TXT archive 的 API 读取、refresh 与 archive hash 回归，以及全格式、相对 cache、已有双用户的真实 Docker 旧卷读取、刷新、逻辑备份不破坏性、restart 均已通过。Git `c7d5abb`；GHCR `:c7d5abb`/`:latest` index digest `sha256:d7000822b4a135c3ee9ab12c4cbef5c5343cfc87c125cc3e5f05f52098d46fa7`。历史绝对路径只能在所属 archive 根内重定位，合法相对 cache 迁移后只保存 `content/...`，不能读取宿主绝对路径；A/B 对对方 list/read/refresh 均为 404，A backup/restore 不改变 B。逻辑备份 ZIP 不含本地 archive，验证目标是“不破坏挂载卷”而非凭 ZIP 重建本地书。剩余 portable archive backup。详见 [`local-book-old-volume-p1e4-contract.md`](local-book-old-volume-p1e4-contract.md)。 | 启动、列表、章节读取、刷新、逻辑备份不破坏性、Docker volume smoke |
| E4-SEC-1 | 用户 A 的 token、archive 与资源 URL 被用户 B 或过期 token 读取。 | stage、EPUB/CBZ resource、WebDAV/LocalStore |

每项都必须同时断言“失败不创建 book/chapter/archive 或不损坏既有 archive”，并记录 fixture
的格式、大小、哈希和来源说明。

## 5. 数据和发布门禁

1. 不改 SQLite schema、`data/`、`cache/`、`library/` 的已有根、旧链接或备份格式。
2. 在旧卷测试中，缺失的可再生 cache 可以重建；原始 archive、已有 chapter progress、
   bookmarks 和用户范围不得被删除或跨用户读取。
3. 新导入采用严格限额，旧 archive 恢复采用更宽但有限的兼容限额；两者都必须有
   ZIP 路径、展开量、PDF 页数/文本量、UMD 章节量测试。
4. 每个可供用户验证的 P1-E4 Docker 切片都必须先通过与该切片相关的 `go test ./...`、
   `npm test`、`npm run build`、真实 Go 服务浏览器验证和本地
   `docker-volume-backup-smoke.sh`；完整 P1-E4 收尾仍需包含 E4-VOLUME-1。E4-EPUB-2 已按此
   切片门禁本地构建并发布，不能被误写成整个 E4 已完成。
5. Docker 发布报告必须列出这份合同中仍未完成的格式、允许的安全收紧、镜像 tag、
   digest 和旧卷验证结果。

## 6. 非目标与下一步

- 本批不重新扩张 P1-E3 已收敛的 LocalStore/WebDAV UI，也不把 PDF/Markdown 重新
  暴露为书仓入口。
- 不以“当前单元测试通过”替代真实格式、Reader 和挂载卷验证。
- E4-TXT-1、E4-TXT-2、EPUB 首封面、E4-EPUB-2、E4-UMD-1、E4-CBZ-1 与 E4-PDFMD-1 已完成；E4-VOLUME-1 已完成合同审查，下一步先加入真实旧卷失败夹具，再处理路径收紧与 Docker 回归；每项仍遵守“合同、失败测试、实现”顺序。
