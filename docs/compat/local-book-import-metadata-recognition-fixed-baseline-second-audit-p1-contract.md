# 本地书导入：书名/作者/简介/目录规则自动识别合同（P1）

状态：**contract-only**。审查日期：2026-10-29。审查基线：`OpenReader@b4cf2be`。

## 1. 来源与结论

用户 2026-10-29 要求导入书籍的逻辑向上游对齐，并补充“包括自动读取章节/作者等完整的逻辑”。用户自己的书
`前浪(校对加料版全本 1-425) 作者：刀一耕.txt` 在上游导入为书名 `前浪(校对加料版全本 1-425)`、作者 `刀一耕`；
OpenReader 把整个文件名当书名、作者留空。这也让 reader-dev 逻辑备份恢复出的占位书（按 `书名 + 作者` 精确匹配回填，见
[`readerdev-local-book-reattachment-device-feedback-p0-contract.md`](readerdev-local-book-reattachment-device-feedback-p0-contract.md)）
在补传原文件时对不上，只能新建一本。

结论：文件名解析书名/作者（M1、M2）、EPUB 作者格式与简介（M3、M4）、UMD 类型（M5）、自动选中的 TXT 目录规则的回显与保存（M6）、
导入确认框的字段（M7）为 **must-fix**；EPUB/UMD 封面图片（M8）另起切片。

## 2. 固定上游

`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`：

- `BookController.kt#importBookPreview`（直接上传）：扩展名取 `getFileExtetion(fileName)`（区分大小写），去掉扩展名后删除
  `AppPattern.fileNameRegex = [\\/:*?"<>|.]`，截到前 50 个 UTF-16 单元，再拼回扩展名，作为本地文件名交给 `Book.initLocalBook`。
- `BookController.kt#importFromLocalPathPreview`（书仓/WebDAV）：直接用原文件名 `File(path).name`，不删字符、不截断。
- `Book.kt#initLocalBook`：`LocalBook.analyzeNameAuthor(File(localPath).name)` 得到书名/作者，`canUpdate = false`，
  再 `updateFromLocal()` 用格式元数据覆盖。
- `LocalBook.kt#analyzeNameAuthor`：取最后一个 `.` 之前的部分，依次 `find` 四个正则：
  `(.*?)《([^《》]+)》.*?作者：(.*)`、`(.*?)《([^《》]+)》(.*)`、`(^)(.+) 作者：(.+)$`、`(^)(.+) by (.+)$`；
  命中则书名 = 第 2 组，作者 = `formatBookAuthor(第 1 组 + 第 3 组)`。都不命中时书名 = `formatBookName(全名)`，
  作者 = `formatBookAuthor(全名去掉书名)`，若作者长度等于全名长度则为空。
- `BookHelp.kt#formatBookName/formatBookAuthor`：分别删除 `\s+作\s*者.*|\s+\S+\s+著`、`^\s*作\s*者[:：\s]+|\s+著`，
  再去掉首尾 `<= ' '` 的字符。
- `EpubFile.kt#upBookInfo`：书名 = 第一个非空 `dc:title`（为空时用 `originName` 去 `.epub`，实际是存储路径）；作者存在时 =
  `authors[0].toString()` 再删 `^, |, $`，epublib 在最后一个半角空格处拆成名/姓，`toString` 为“姓, 名”；
  简介 = `Jsoup.parse(descriptions[0]).text()`；封面写到 `/assets/<ns>/covers/<md5>.jpg`。没有作者时保留文件名得到的作者。
- `UmdFile.kt#upBookInfo`：书名/作者 = UMD 头 2、3 段（直接覆盖，包括空值），`kind` = 第 7 段“小说类型”，封面 = 附加段 130。
- `CbzFile.kt#upBookInfo`：`ComicInfo.xml` 的 Title/Writer 存在时覆盖，否则保留文件名结果（已对齐，见 `local-book-p1e4-contract.md`）。
- `TextFile.kt#getChapterList`：`tocUrl` 为空时用前 512000 字节自动选规则，**把选中规则的正则写回 `book.tocUrl`**；
  预览返回该 `book`，`Index.vue` 的“规则”文本框显示 `importBookInfo.tocUrl`，`saveBook` 时随书保存。
  `EpubFile.kt#getChapterList` 在 `tocUrl` 为空时写回 `spin+toc`。
- `Index.vue` 导入确认框（895–1001 行）：封面（无封面时显示默认图）、`书名：`/`作者：` 输入框、`分组：` 多选、
  TXT/EPUB 时 `规则：` 下拉（占位“内置规则”）+“刷新目录”+ 显示/编辑规则的文本框，`章节列表(N)` 与 `序号. 标题`。

用 `eclipse-temurin:21-jdk`（`--network none`）逐字移植上述 Kotlin/Java 逻辑实测 26 个文件名与 6 个 EPUB 作者，结果作为测试夹具，节选：

| 文件名 | 书仓/WebDAV | 直接上传 |
| --- | --- | --- |
| `前浪(校对加料版全本 1-425) 作者：刀一耕.txt` | `前浪(校对加料版全本 1-425)` / `刀一耕` | 同左 |
| `《斗破苍穹》（精校版）作者：天蚕土豆.txt` | `斗破苍穹` / `天蚕土豆` | 同左 |
| `【完结】《凡人修仙传》.txt` | `凡人修仙传` / `【完结】` | 同左 |
| `三体 刘慈欣 著.txt` | `三体` / `刘慈欣` | 同左 |
| `The Hobbit by J R R Tolkien.txt` | `The Hobbit` / `J R R Tolkien` | 同左 |
| `三体.第一部.txt` | `三体.第一部` / 空 | `三体第一部` / 空 |
| `书名 作者:张三.txt` | `书名` / `张三` | `书名` / `作者张三` |
| `书名　作者：甲.txt`（全角空格） | 整名 / 空 | 同左 |
| 50 字以上书名 ` 作者：某人` | 完整书名 / `某人` | 完整书名 / `某`（截断） |

EPUB 作者：`天蚕土豆`→`天蚕土豆`，`John Smith`→`Smith, John`，`J. K. Rowling`→`Rowling, J. K.`，`刘 慈欣`→`慈欣, 刘`。

## 3. 当前 OpenReader（`b4cf2be`）

- `services/localbook/importer.go#previewResult` 与 `durable_import.go#ImportPreparedResult`：书名 = 请求值 → 格式元数据 →
  去扩展名的文件名；作者 = 请求值 → 格式元数据，**没有文件名解析**。
- `engine/epub_parser.go`：书名/作者取第一个非空 `dc:title`/`dc:creator` 原文；不读 `dc:description`。
- `engine/umd_parser.go`：读头 2、3 段，忽略第 7 段与封面。
- `engine/txt_parser.go#detectTXTTitlePattern` 选出的规则不返回；预览 JSON 只有 `title/author/chapterCount/chapters/importToken`；
  `Book.TOCRule` 只保存请求里的规则（自动时为空）。
- `OverlayStorageImport.vue` 单本确认：无封面、无标签（只用占位符）、无规则文本框，`章节列表（N）`。

## 4. 差异与裁决

| 编号 | 项目 | 裁决 | 目标 |
| --- | --- | --- | --- |
| M1 | 文件名解析书名/作者（所有格式的底层结果） | **must-fix** | 逐字移植 `analyzeNameAuthor` 与两个格式化函数；格式元数据为空时使用 |
| M2 | 直接上传的文件名规范化 | **must-fix** | 直接上传（含其后的刷新目录/确认）先删 `\/:*?"<>|.`、截到 50 个 UTF-16 单元再解析；书仓/WebDAV 用原名。暂存元数据记录来源 |
| M3 | EPUB 作者格式 | **must-fix** | 第一个非空 `dc:creator`，在最后一个半角空格处拆为“姓, 名”，删 `^, |, $`，再去首尾空白；没有作者时用 M1 |
| M4 | EPUB 简介 | **must-fix** | 第一个 `dc:description` 去标签、解实体、合并空白后存入 `Book.Intro`（新建与占位回填都写，只在为空时由导入写入） |
| M5 | UMD 类型 | **must-fix** | 第 7 段存入 `Book.Kind` |
| M6 | 自动选中的 TXT 规则 | **must-fix** | 预览 JSON 增加 `tocRule`（生效规则：显式规则、自动选中的规则、EPUB 为 `spin+toc`）；请求未指定规则时把自动选中的 TXT 规则存入 `Book.TOCRule` |
| M7 | 导入确认框 | **must-fix** | 封面位（无封面显示默认封面）、`书名：`/`作者：`/`分组：`/`规则：` 标签、规则下拉占位“内置规则”、显示生效规则的可编辑文本框、`章节列表(N)` |
| M8 | EPUB/UMD 封面图片 | **另起切片** | 需要新的封面资源投影与备份范围评估；本切片确认框与书架对这两种格式仍显示默认封面 |
| A1 | 扩展名大小写、带 `?` 的文件名 | acceptable-change | 上游 `Book.TXT` 或文件名含 `?` 时拒绝导入；OpenReader 继续按真实扩展名接受，名称按 M2 删字符 |
| A2 | EPUB 无书名 | acceptable-change | 上游得到存储路径；OpenReader 用 M1 书名 |
| A3 | UMD 头书名/作者为空 | acceptable-change | 上游用空值覆盖；OpenReader 保留 M1 结果 |
| A4 | 截断不拆 UTF-16 代理对 | acceptable-change | 上游可能留下半个 emoji；OpenReader 截到 50 单元内最后一个完整字符 |
| A5 | TXT 章节标题去首尾空白 | acceptable-change | 上游标题含行首缩进；已签收的现有行为，不改 |
| A6 | `canUpdate=false` | 不重开 | 本地书在 OpenReader 本就不参与远程更新 |

不重开：编码识别、目录规则匹配本身、暂存/快照/补偿事务、分组多对多、批量/逐本状态机、64 本上限、`.text/.md/.pdf` 隐藏兼容；
已导入的书不重新解析（重新导入或在书籍信息里修改即可得到新书名/作者）。

## 5. 数据与接口

- `PreviewResult` 只增字段 `tocRule`；导入接口、错误格式、状态码不变。
- `importstage.Metadata` 增加可省略的 `directUpload`；旧暂存缺省按书仓规则解析（24 小时内过期，影响有限）。
- `PreparedImport` 快照增加可省略字段（生效规则、简介、类型）；旧快照缺省为空，不触发失败。
- 不改 SQLite schema：`Intro`、`Kind`、`TOCRule` 均为现有列。逻辑/portable 备份格式不变。

## 6. 计划测试（先红）

1. `engine`：上表全部 26 个文件名 × 两种来源、6 个 EPUB 作者，与 Java 实测夹具逐项相等。
2. `localbook`：TXT/EPUB/UMD/CBZ 预览与导入的书名/作者/简介/类型/`TOCRule`；请求里填写的书名/作者优先（对照）。
3. `api`：直接上传 `前浪(校对加料版全本 1-425) 作者：刀一耕.txt` 预览得到拆分后的书名/作者与 `tocRule`，经 token 确认后
   `Book.TOCRule` 为该规则；书仓同名文件得到相同结果；名称含 `.` 时两种来源不同；reader-dev 占位书 `前浪…`/`刀一耕` 被原地回填。
4. 前端：确认框源码合同（标签、封面、规则文本框、`章节列表(`）与工作流把预览 `tocRule` 写入行数据但确认时仍提交用户选择的规则。
5. 浏览器 1440×900、390×844、360×800：直接导入示例 TXT，确认框显示拆分后的书名/作者与规则文本，导入后书架显示同样书名/作者。
