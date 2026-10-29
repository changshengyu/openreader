# 本地书封面：EPUB/UMD 书架封面与导入确认框封面合同（P1，M8）

状态：**aligned / Docker-published / awaiting-device-verification**（发布见文末；记录见第 6 节）。审查日期：2026-10-29。审查基线：`OpenReader@c62c619`。

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
| C3 | 导入确认框封面 | **must-fix** | 预览（直接上传、书仓、WebDAV、token 重新解析）返回 `coverUrl`：EPUB/CBZ/UMD 的封面图在 2 MiB 以内时为 `data:` URL，否则省略；一个预览响应里的内联封面合计不超过 8 MiB（A7）；只有确认框用的预览计算封面，导入落库与内部重新解析不计算 |
| A1 | 封面图格式 | acceptable-change | EPUB 只认 JPG/PNG/GIF（同上游）；UMD 封面与确认框预览封面按文件头识别 JPEG/PNG/GIF/WebP，其它字节不当封面（上游原样写 `.jpg` 交给浏览器猜）；CBZ 书架封面规则不变 |
| A2 | 预览封面大小 | acceptable-change | 超过 2 MiB 的封面只在确认框里显示默认封面，导入后书架照常显示 |
| A3 | 封面存放 | technical-stack-equivalent | 不写 `/assets/.../covers`、不改 `Book.CoverURL`：封面是可重建的派生文件，能力地址只出现在响应里，备份/恢复格式不变 |
| A4 | EPUB3 `cover-image` | 不采用 | 上游不认，只有该属性的 EPUB 仍无封面；若要改进需用户另行确认 |
| A5 | 旧 EPUB 的封面时机 | acceptable-change | 列书架时不解包 EPUB，只用已有抽取：2026-07-18（`e24d5773`）之前导入、此后从未打开过的 EPUB 打开一次后才显示封面；其后导入的书抽取标记里没有封面信息时，首次显示时查一次并补记 |
| A7 | 批量预览的封面总量 | acceptable-change（缓存加固） | 上游每本写封面文件并返回地址；OpenReader 内联 `data:` URL，书仓/WebDAV 一次最多预览 200 本，故一个预览响应的封面文本合计不超过 8 MiB，按顺序先到先得；超出的行在确认框显示默认封面，单本重新解析时按新的预算补回，导入后书架照常 |
| A6 | UMD 首次扫描 | technical-stack-equivalent | 每个 UMD 版本只在首次投影时读一次原文件（256 MiB 以内，更大的不扫描、显示默认封面），结果连同“无封面”记入标记；原文件按大小与修改时间判断是否变化 |

不重开：CBZ 封面（已对齐）、远程书封面代理、自定义封面上传与删除、书架布局。

## 5. 计划测试（先红）

1. engine：EPUB meta（id/href 两种写法）、guide 图片、guide 指向 XHTML、SVG/WebP 封面、只有 `cover-image`、清单媒体类型缺失按扩展名；
   UMD 有/无段 130、非图片字节、超限；对应的预览封面字节与内容类型。
2. api：直接上传 EPUB/UMD 带封面——预览 `coverUrl` 为 `data:image/...`；导入响应、书架、详情的 `coverUrl` 分别为
   `/api/epub-resource/`、`/api/umd-cover/` 且能取回原图；库里 `cover_url` 为空；自定义封面优先；无封面时为空；旧格式抽取标记仍能补出封面；
   UMD 能力地址被篡改为 403、书删除后 404、不能跨用户。
3. 前端：预览行带 `coverUrl`，确认框封面使用它。
3a. 批量上限（A7）：书仓一次预览多本大封面 EPUB 时，响应里的封面文本合计 ≤ 8 MiB、首本有封面、超出的行没有封面，该行单本重新解析得到封面；
    localbook `Prepare`（暂存确认与落库路径）不计算预览封面。
4. 浏览器 1440×900、390×844、360×800：导入带封面的 EPUB 与 UMD，确认框与书架都显示封面图。

## 6. 记录（2026-10-29）

提交（分支 `claude/day-night-import-alignment`）：合同 `6b1fe96`、A7 补充 `765a999`；行为不变的骨架 `4dd84e0`；红测 `5ac9d00`、`7105dbf`、`e24def9`；
测试修正 `0007fe2`；实现 `7901e1f`、`b5db7d6`；浏览器合同 `ad74f13`；UMD 封面服务测试 `8fb8f88`。

- 红测（均为断言失败）：`7105dbf` 上 engine EPUB meta/guide 规则 4 例、EPUB 封面字节、UMD 段 130，api 预览 `coverUrl`、旧抽取标记的书架封面、
  UMD 能力地址，访问日志 `/api/umd-cover/` 未脱敏，前端确认框封面与预览行 `coverUrl`（无封面对照用例始终通过），`7901e1f` 后转绿；
  `e24def9` 上（代码与 `7901e1f` 相同）书仓一次预览 6 本 1.5 MiB 封面 EPUB 的响应内联 12,583,266 字节封面、`Prepare`/`Preview` 也计算封面，`b5db7d6` 后转绿。
  `0007fe2` 只改测试准备：改写旧标记时原先经 `float64` 丢了 `modTimeNano` 的纳秒，标记描述的已不是原文件；改为保留原始 JSON 值，断言不变。
- 实现要点：
  - EPUB：`engine.EPUBCoverImagePathFromZip` 按 epublib 取封面图（meta 先、guide 后，JPG/PNG/GIF，EPUB3 `cover-image` 不认）；导入时的抽取标记
    记 `coverPath`/`coverChecked`，旧标记在首次显示时查一次并补记；书架只用已有抽取，不在列表请求里解包（A5）。
  - UMD：新 `services/umdcover` 取段 130 后的附加段，按文件头识别 JPEG/PNG/GIF/WebP，写 `.umd-resources/<sha256>/cover.<ext>` 与标记
    （无封面也记，A6）；原文件与派生文件都经 `webdavfs.NewScoped` 打开、不跟随符号链接；建目录用 `Mkdir`，不重建已删除的书目录；
    `/api/umd-cover/<capability>` 为 HMAC 签名（用户/书/指纹，12 小时），按当前书行授权，原文件换了旧地址即失效；篡改 403、格式错 400、
    书删除 404；`nosniff`、`same-origin`、`private, max-age=300`；访问日志脱敏。
  - 预览：只有确认框用的预览（直接上传、书仓、WebDAV、token 重新解析）经 `Importer.PreparePreview` 计算封面，`Prepare`/`Preview`
    （暂存确认、落库、可移植恢复）不再解码编码封面；单张 2 MiB 以内，一个响应合计不超过 8 MiB 的 `data:` URL（A7）；前端预览行保存
    `coverUrl`，重新解析时刷新，确认框 `BookCover` 用它。
  - 封面地址只在响应里：`books.cover_url` 不写，备份/恢复格式不变，自定义封面优先。
- 测试：backend `go test ./...` 30 个包、`go vet ./...`、`go test -race -timeout 45m`（api 1049 秒，及 db、middleware、engine、services/umdcover、epubreader、localbook、importstage、cbzreader）在 `b5db7d6` 上通过，未见 DATA RACE（之前一次 race 漏设 `-timeout`，api 在默认 10 分钟处超时中止，不是竞态）；`services/umdcover` 7 个服务测试（含 `-race`）通过；frontend 829/829 与 build 通过（沙箱内 3 个依赖公告 worker 用例因禁止
  监听本地端口失败，沙箱外全部通过）；公告门脚本测试 109/109，Go/前端公告门 exit 0；`docker compose config` 通过；
  `store-read-list-lifecycle-contract` 在 `b5db7d6` 上通过。
- 真实浏览器（本地 Go 服务 `b5db7d6` + 前端构建）：`local-book-cover-contract` 在 1440×900、390×844、360×800 直接导入脚本生成的 EPUB
  （meta 封面）与 UMD（段 130 封面）：确认框封面为 `data:image/png` 且解码尺寸与原图一致，导入后书架与书籍信息分别经
  `/api/epub-resource/`、`/api/umd-cover/` 显示同一张图，无封面相关 4xx/5xx、无页面错误、无横向溢出，截图人工查看布局正常；
  `local-book-import-contract`（取消竞态、单本、批量、逐本）与 `workspace-storage-retry-contract`（书仓、WebDAV）三视口通过。
- 未做：EPUB3 `cover-image`（A4，不采用）；2026-07-18 前导入且从未打开的 EPUB 打开一次后才有封面（A5）。

状态：**implementation-complete / regression-validated**，待可信 Actions；镜像未发布；生产未访问。

## 发布（2026-10-29）

main 快进到 `21f749d`（ls-remote 核验，未强推；本机代理 127.0.0.1:20808 端口关闭，推送与查询改走直连）；可信 Actions run `40880848625` 全部成功（13:19–13:41Z）。`ghcr.io/changshengyu/openreader:21f749d` 与 `latest` 为 OCI index `sha256:fad3291832efb6ffba3866725069fce47ba1a5f96c05836ee829742396dd97a7`（linux/amd64 `sha256:3a3d433a…`、linux/arm64 `sha256:7c79a0c6…`），两平台 `org.opencontainers.image.revision` 均为 `21f749d31e12f00e370181d3bca9b755e683dd3c`。状态 **aligned / Docker-published / awaiting-device-verification**；生产未访问，实际运行版本未知，需在生产升级容器后实机确认（含已有 EPUB/UMD 书的书架封面）。
