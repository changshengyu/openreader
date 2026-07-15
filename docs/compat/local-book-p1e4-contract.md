# P1-E4 本地书真实格式与旧挂载卷兼容合同

状态：**审查完成；E4-TXT-2 已实现并完成后端、前端和三视口真实浏览器验证。其余真实格式与旧挂载卷项目仍未开始。**

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
   `toc+spin`、`toc<spin` 都是公开行为。章节可引用片段，并可跨资源拼接到下一章节；
   阅读端以解压后的资源（含封面页）显示，而非把 XHTML 简化为普通文本。
4. UMD 使用标准 `0xde9a9b89`、UTF-16LE/压缩分段格式；每个 UMD 标题对应一个章节。
5. CBZ 从 `ComicInfo.xml` 取标题/作者；每个归档条目对应漫画章节，阅读端返回图片
   内容并隐藏普通正文标题。
6. 上游直接解压 EPUB/CBZ 到派生 `index` 目录。OpenReader 不应复制其不受限解压、
   任意归档条目暴露或跨用户路径行为。

## 3. 差异矩阵与裁决

| 合同层 | 上游行为 | 当前证据 | 裁决 | P1-E4 必须证明/处理 |
|---|---|---|---|---|
| TXT 自定义规则无匹配预览/确认 | 上游预览正常返回空章节；`BookController.saveBook` 对本地书不要求目录非空，因此用户可保留空目录书籍，之后再按规则刷新。 | `Importer.Preview`/`Import` 返回空章节/零章节书；直接、LocalStore、WebDAV UI 显示可恢复说明并保持同一个 stage token。 | **aligned（E4-TXT-2 已完成）** | API 仅消费当前用户确认的 stage；不制造虚假 chapter，错误格式仍保持失败。 |
| TXT 规则目录长章节 | 固定基准的 `Book.getSplitLongChapter()` 返回 `false`，因此不会启用 `TextFile` 内约 100 KiB 的可选分支。 | `parseTXTText` 不切分规则目录长章节。 | **aligned** | 用超过 100 KiB 的规则目录夹具锁定“不被隐式拆分”、标题顺序和正文连续性；不得无依据引入新的切分设置。 |
| TXT 无目录/编码 | 512000 探测、10 KiB 伪章节、BOM/编码字节读取。 | `txt_parser.go` 已有 512000、10 KiB、UTF-8 边界和历史编码测试。 | **partial** | 用真实 UTF-8 BOM、GBK/GB18030 及无目录文件贯穿 preview→import→reader；不能只测纯函数。 |
| EPUB 目录规则 | 六种策略，默认 `spin+toc`。 | parser 单元测试覆盖 spine、NAV、NCX 和部分规则。 | **partial** | 补所有六种规则、`titlepage.xhtml`、fragment、跨资源章节的真实归档/API/iframe 夹具；资源 URL 必须仍受当前用户和 book capability 限制。 |
| EPUB 阅读资源 | 解压资源页显示，图片/样式/锚点有效。 | `epubreader` 用受控临时资源和 iframe，属实现替换。 | **acceptable-change，待验证** | 证明正文、相对资源、封面页、锚点和旧 archive 的懒恢复可读；不退回不受限解压。 |
| 标准 UMD | 标准 reader-dev 分段 UMD。 | `umd_parser_contract_test.go` 已用上游写入 framing；所有入口有契约测试。 | **partial** | 补真实导入后 reader/刷新/旧卷恢复；当前 `#TEXTNOV` 伪 UMD 仅可作为旧 OpenReader 数据兼容分支。 |
| CBZ 章节列表 | 上游把非 XML 归档项也作为章节，资源直接暴露。 | 当前只接受规范化后的安全图片条目，并经短时 capability 读取。 | **acceptable-change（安全）** | 记录为有意安全收紧；验证 ComicInfo、自然排序、图片资源、无图片、路径逃逸/重复路径都不泄漏或越界。 |
| PDF、Markdown、`.text` | 上游工作台导入并不提供这些格式。 | 直接上传 API/UI 支持 `.text/.md/.pdf`；P1-E3 书仓 UI 不展示它们。 | **OpenReader-only extension** | 只能维持为直接上传兼容能力；要补真实 PDF（含无文本）和 Markdown 夹具。未经新合同不得重新加入 LocalStore/WebDAV 工作台入口。 |
| 预览的文件保存 | 上游会写临时/导入路径，且目录为空也可预览。 | 当前用用户范围、不可变 stage token；预览成功前无书架写入。 | **acceptable-change（多用户/安全）** | 空目录与失败重试都必须保留同一用户 token；不能因挂载卷、WebDAV 或网络变化重新读取原路径。 |
| `library/` archive 位置 | 上游 `storage/data/<namespace>/<name_author>/` 及派生 `index`。 | 当前 `library/data/<user>/<safe-name>/` 存 `OriginalFile`、`chapters.json`、`bookSource.json` 和 `content/`。 | **technical-stack-equivalent，待旧卷验证** | 不迁移或删除既有目录。验证相对/绝对旧字段、缺失 content cache、旧 `ResourcePath` 与 archive 文件仍可恢复。 |
| 新旧资源限制 | 上游没有 ZIP/解压/文本上限。 | 新导入受严格上限；旧 archive 使用较宽但有界的 `LegacyLocalBookParseLimits`。 | **acceptable-change（安全）** | 新输入拒绝必须在归档/DB 写入前发生；旧卷恢复必须仍受界且可读，不得突破用户隔离。 |

## 4. 先写的回归夹具和测试

以下测试先于任何实现修改加入；夹具仅可含可再分发的自建最小内容，不提交受版权保护书籍。

| 编号 | 夹具与断言 | 覆盖的入口 |
|---|---|---|
| E4-TXT-1 | UTF-8 BOM、GBK/GB18030、无目录、规则目录和超过 100 KiB 的单章 TXT；断言章节边界、连续正文、默认不拆分长规则章节和重新解析。 | 直接上传、LocalStore、WebDAV、reader content |
| E4-TXT-2 | 显式规则无匹配：预览 `200` + 空章节 + 原 token；界面展示可恢复空态；确认可创建零章节本地书，或重新解析后创建正常目录。 | importer、`/preview`、OverlayBookImport、OverlayStorageImport |
| E4-EPUB-1 | NAV/NCX、六种规则、封面页、fragment、跨资源、相对图片/样式的 EPUB。 | parser、import、资源 capability、iframe real browser |
| E4-UMD-1 | 标准 reader-dev UMD 的导入、刷新、删除派生 cache 后重读；保留旧 pseudo-UMD 只读恢复。 | direct/storage/WebDAV、chapter content |
| E4-CBZ-1 | ComicInfo、按路径排序的真实最小图片、无图片、重复/逃逸 zip path。 | parser、import、`/api/cbz-resource`、移动/桌面 Reader |
| E4-PDFMD-1 | 文本 PDF、扫描/无文本 PDF、Markdown 和 `.text`。 | 直接上传 API/UI；错误状态不得创建书架/archive |
| E4-VOLUME-1 | 人工构造的旧挂载卷：旧 SQLite book/chapter 行、相对和历史绝对 `OriginalFile`、`chapters.json`/`bookSource.json`、缺失派生 content，以及 EPUB/CBZ/UMD/TXT archive。 | 启动、列表、章节读取、刷新、备份恢复、Docker volume smoke |
| E4-SEC-1 | 用户 A 的 token、archive 与资源 URL 被用户 B 或过期 token 读取。 | stage、EPUB/CBZ resource、WebDAV/LocalStore |

每项都必须同时断言“失败不创建 book/chapter/archive 或不损坏既有 archive”，并记录 fixture
的格式、大小、哈希和来源说明。

## 5. 数据和发布门禁

1. 不改 SQLite schema、`data/`、`cache/`、`library/` 的已有根、旧链接或备份格式。
2. 在旧卷测试中，缺失的可再生 cache 可以重建；原始 archive、已有 chapter progress、
   bookmarks 和用户范围不得被删除或跨用户读取。
3. 新导入采用严格限额，旧 archive 恢复采用更宽但有限的兼容限额；两者都必须有
   ZIP 路径、展开量、PDF 页数/文本量、UMD 章节量测试。
4. P1-E4 Docker 候选只能在下列全绿后构建和发布：`go test ./...`、`npm test`、
   `npm run build`、真实 Go 服务上的 1440×900/390×844/360×800 EPUB/CBZ/TXT 验证、
   以及包含 E4-VOLUME-1 的本地 `docker-volume-backup-smoke.sh`。
5. Docker 发布报告必须列出这份合同中仍未完成的格式、允许的安全收紧、镜像 tag、
   digest 和旧卷验证结果。

## 6. 非目标与下一步

- 本批不重新扩张 P1-E3 已收敛的 LocalStore/WebDAV UI，也不把 PDF/Markdown 重新
  暴露为书仓入口。
- 不以“当前单元测试通过”替代真实格式、Reader 和挂载卷验证。
- 下一步是只新增上述夹具与契约测试；所有失败用例先稳定复现后，再处理
  TXT 空目录预览这一项 `must-fix`。
