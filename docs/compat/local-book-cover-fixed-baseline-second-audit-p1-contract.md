# 本地书封面：EPUB/UMD 书架封面与导入确认框封面合同（P1，M8）

状态：**contract-only**。审查日期：2026-10-29。审查基线：`OpenReader@c62c619`。

## 1. 来源与结论

[`local-book-import-metadata-recognition-fixed-baseline-second-audit-p1-contract.md`](local-book-import-metadata-recognition-fixed-baseline-second-audit-p1-contract.md)
把 M8（EPUB/UMD 封面图片）留作后续；用户 2026-10-29 要求“接着做封面”。上游导入 EPUB/UMD 时读出封面，书架、书籍信息与导入确认框
都显示；OpenReader 只有 CBZ 有封面（[`local-book-p1e4-contract.md`](local-book-p1e4-contract.md) E4-CBZ-1），EPUB/UMD 一律显示
“暂无封面”，确认框也只有默认封面。结论：C1（EPUB）、C2（UMD）、C3（确认框预览封面）为 **must-fix**。

## 2. 固定上游

`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`：

- `Book.kt#initLocalBook → updateFromLocal`：导入预览时就调用 `EpubFile/UmdFile/CbzFile.upBookInfo`，其中 `updateCover()` 把
  `coverUrl` 设为 `/assets/<ns>/covers/<md5(bookUrl)>.jpg` 并写出图片字节；没有封面时写入抛异常被吞掉，`coverUrl` 指向不存在的文件，
  前端 `getCover` 加载失败后显示默认封面。
- `EpubFile.kt#updateCover`：写 `epubBook.coverImage`。epublib `PackageDocumentReader.readCover/findCoverHrefs`：候选为
  `<meta name="cover" content="X">`（X 是清单 id 时取该项 `href`，否则把 X 当 href）与 `<guide><reference type="cover" href="Y">`；
  候选资源是 XHTML 时只算“封面页”，媒体类型为 `MediaTypes.isBitmapImage`（`image/jpeg`、`image/png`、`image/gif`）时才是封面图。
  资源媒体类型取清单 `media-type`（未知时按扩展名）。EPUB3 的 `properties="cover-image"` 不参与。
- `UmdFile.kt#updateCover` / umdlib `UmdReader#ReadSection/ReadAdditionalSection`：段 130（`0x82`，1 字节类型 + 4 字节校验号）之后的附加段
  就是封面字节（不校验检查号），原样写成 `.jpg`。
- `Index.vue` 导入确认框：`getCover(getBookCoverUrl(importBookInfo))`，即预览阶段已写出的封面；TXT 显示默认封面。
- 书架/书籍信息：`customCoverUrl` 优先，其次 `coverUrl`。

## 3. 当前 OpenReader（`c62c619`）

- `api/books.go#projectBookShelfListItem`：只对本地 CBZ、且 `CoverURL`/`CustomCoverURL` 都为空时调用 `cbzreader.PrepareCover`，把同源
  `/api/cbz-resource/<capability>/<path>` 写进响应里的 `coverUrl`（不入库、不进归档/备份）；导入响应、书架、详情共用这一投影。
- `epubreader`：EPUB 资源抽取在书目录 `.epub-resources/<fingerprint>/`，`/api/epub-resource/<capability>/<path>` 按用户/书/指纹授权
  抽取目录里的任意资源（图片按扩展名给出 `image/*`）；不识别封面。
- UMD：原文件归档后不再抽取任何资源，解析器忽略段 130。
- 预览 JSON 没有封面字段；确认框显示“暂无封面”。

## 4. 差异与裁决

| 编号 | 项目 | 裁决 | 目标 |
| --- | --- | --- | --- |
| C1 | EPUB 书架/详情/导入响应封面 | **must-fix** | 按 epublib 规则选出封面图路径（meta 先、guide 后；JPG/PNG/GIF），记入 `.epub-resources` 抽取标记（派生数据，可重建）；投影为现有 `/api/epub-resource` 能力地址，规则与 CBZ 相同（只在响应里、自定义封面优先、出错时为空） |
| C2 | UMD 书架/详情/导入响应封面 | **must-fix** | 首次投影时从归档原文件取段 130 后的附加段，写入书目录 `.umd-resources/<fingerprint>/` 并记标记（无封面也记，避免反复扫描）；新增 `/api/umd-cover/<capability>`，按用户/书/指纹授权，`nosniff`、`same-origin`、`private` 缓存 |
| C3 | 导入确认框封面 | **must-fix** | 预览（直接上传、书仓、WebDAV、token 重新解析）返回 `coverUrl`：EPUB/CBZ/UMD 的封面图在 2 MiB 以内时为 `data:` URL，否则省略 |
| A1 | 封面图格式 | acceptable-change | EPUB 只认 JPG/PNG/GIF（同上游）；UMD/CBZ 按文件头识别 JPEG/PNG/GIF/BMP/WebP，其它字节不当封面（上游原样写 `.jpg` 交给浏览器猜） |
| A2 | 预览封面大小 | acceptable-change | 超过 2 MiB 的封面只在确认框里显示默认封面，导入后书架照常显示 |
| A3 | 封面存放 | technical-stack-equivalent | 不写 `/assets/.../covers`、不改 `Book.CoverURL`：封面是可重建的派生文件，能力地址只出现在响应里，备份/恢复格式不变 |
| A4 | EPUB3 `cover-image` | 不采用 | 上游不认，只有该属性的 EPUB 仍无封面；若要改进需用户另行确认 |

不重开：CBZ 封面（已对齐）、远程书封面代理、自定义封面上传与删除、书架布局。

## 5. 计划测试（先红）

1. engine：EPUB meta（id/href 两种写法）、guide 图片、guide 指向 XHTML、SVG/WebP 封面、只有 `cover-image`、清单媒体类型缺失按扩展名；
   UMD 有/无段 130、非图片字节、超限；对应的预览封面字节与内容类型。
2. api：直接上传 EPUB/UMD 带封面——预览 `coverUrl` 为 `data:image/...`；导入响应、书架、详情的 `coverUrl` 分别为
   `/api/epub-resource/`、`/api/umd-cover/` 且能取回原图；库里 `cover_url` 为空；自定义封面优先；无封面时为空；旧格式抽取标记仍能补出封面；
   UMD 能力地址被篡改为 403、书删除后 404、不能跨用户。
3. 前端：预览行带 `coverUrl`，确认框封面使用它。
4. 浏览器 1440×900、390×844、360×800：导入带封面的 EPUB 与 UMD，确认框与书架都显示封面图。
