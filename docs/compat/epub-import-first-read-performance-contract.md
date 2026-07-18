# EPUB 本地导入与首次阅读性能兼容合同

状态：**本批已实施并通过自动化、三视口真实浏览器及历史 Docker volume/backup 门禁；跨 resource Reader 正文仍为下一批 `must-fix`。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同处理用户报告的两条同源问题：EPUB 本地导入的目录预览明显慢，以及书架进入 EPUB
阅读器的首次加载明显慢。它补充
[`local-book-import-catalog-p0-contract.md`](local-book-import-catalog-p0-contract.md) 和
[`epub-fragment-p1e4-contract.md`](epub-fragment-p1e4-contract.md)，不改变已经确定的移动端
工具层、iframe 导航或返回手势合同。

## 1. 上游权威数据流

| 阶段 | 上游证据 | 行为合同 |
|---|---|---|
| 上传预览 | `BookController.kt#importBookPreview` 把上传文件复制到用户本地资产目录，然后调用 `LocalBook.getChapterList(book)`。 | 网络只参与一次上传；之后目录刷新和确认均读取同一个服务器本地文件。 |
| EPUB 打开 | `EpubFile.kt#readEpub` 调用 `EpubReader.readEpubLazy(ZipFile, "utf-8")`；`ResourcesLoader` 把 archive entry 建成 `LazyResource`，正文/图片等在第一次 `getData()` 时才解压。 | 建立 EPUB package、manifest、spine 和 TOC 时不能无条件把所有 XHTML 正文解压并常驻内存。 |
| 默认目录规则 | `EpubFile.getChapterList` 在空规则时写入 `spin+toc`，再调用 `getChapterListBySpinAndToc()`；该方法先读 TOC，再读 spine。NCX/NAV 的 `TitledResourceReference` 会把目录标题写回 resource。 | 默认预览应主要读取 container、OPF、NAV/NCX 和目录元数据；只有仍缺标题的 spine resource 才读取 XHTML `<title>`。不能为了目录预览抽取整本正文纯文本。 |
| 六种规则 | `spin`、`spin+toc`、`spin<toc`、`toc`、`toc+spin`、`toc<spin` 分别决定顺序和标题优先级；空/未知值回退 `spin+toc`。 | 目录优化不能改变顺序、fragment 边界、首个无标题 spine 的“封面”回退或标题优先级。 |
| 章节标题回退 | `getChapterListBySpine` / `getChapterList` 只在 resource title 为空时解析 XHTML 的 `<title>`；它不把正文 `h1/h2` 当 resource title。 | OpenReader 现有 `h1, h2, title` 首个节点规则不是上游等价规则，必须以 `<title>` 为正文标题回退。 |
| 确认导入 | `Index.vue#saveBook` 保存预览所得本地 `Book`；上游不要求浏览器重新上传，也不在确认前重新抽取所有正文。 | OpenReader 可以为搜索和安全 iframe 生成派生数据，但不得再次依赖网络，也不得让目录预览等待全书正文物化。 |
| 首次阅读 | `BookController.kt#getBookContent` 首次调用 `extractEpub(book)`，将 archive 解压到固定 `index` 目录；随后直接返回当前 XHTML URL。已有目录下不重复解压。 | 首章只应等待一次有界资源准备；后续章节/资源不能重新扫描或哈希整本 archive。 |
| 正文读取 | `EpubFile.kt#getContent` 按当前目录 URL、`nextUrl` 和 fragment 惰性读取当前章涉及的 resource，并明确注释首次正文获取较慢、之后应缓存。 | 搜索纯文本/兼容内容必须按章惰性生成并缓存，不能成为目录预览的前置条件；跨 resource 章节不能因性能优化丢失中间正文。 |

上游直接解压 ZIP 且没有 OpenReader 的 capability/CSP/多用户边界。OpenReader 必须继续保留
ZIP path、entry count、单项/总解压大小、资源 MIME、用户与 archive 指纹限制；这些是允许且必须的
安全适配，不是继续重复全书工作的理由。

## 2. 当前 OpenReader 差异

| 合同层 | 当前证据 | 判定 | 必须调整 |
|---|---|---|---|
| 预览 parser | 审计前 `engine.ParseEPUBWithLimits` 会物化所有 spine 正文；本批新增 `ParseEPUBCatalogWithLimits`，预览 snapshot 只保存 metadata、顺序、标题、resource path 和 fragment。 | `aligned in this slice` | 正文 materialization 已移至确认期；预览不再保存全书正文。 |
| 标题语义 | 审计前使用 `h1, h2, title` 中 DOM 顺序的第一个节点；本批 catalogue 以流式 tokenizer 仅读文档 `<title>`。 | `aligned` | 六种 TOC/spine 规则、空标题封面及正文标题回退均有契约测试。 |
| 确认导入 | catalogue-only snapshot 在确认时物化正文；同一 XHTML 只解压并解析一次，fragment 使用隔离的克隆 DOM。旧 full-content snapshot 保持可确认。 | `aligned + intentional-redesign` | 保留 token/hash/rule 精确绑定；确认期同时预热 caller-owned extraction，以缩短首次进入 Reader 的等待。 |
| 首次 Reader API | 新 EPUB 在确认事务前完成有界 extraction；`PrepareChapter` 可在完整 marker 的 source size/mtime 精确匹配时零 SHA-256 复用。 | `aligned + security adaptation` | source 变化、marker 损坏或资源缺失仍回退到 SHA-256/原子重建，旧 capability 失效。 |
| 后续资源 | extraction marker 已保存 fingerprint、source size 和 mtime；`OpenResource` 在 marker 未变化时不会重新哈希 source。 | `aligned after first prepare` | 保留 immutable extraction/capability；把同一安全快路径前移到 `PrepareChapter`，并用测试保证 source 替换仍使 capability 失效。 |
| 纯文本 cache | 本批 `ReadChapterText` 从已验证 extraction 仅读取当前 XHTML/fragment，`rebuildLocalChapterText` 不再为 EPUB cache miss 调用整本 parser。 | `aligned for current-resource chapters` | API 测试删除目标章 cache 后验证自动恢复，并确认相邻章 cache 字节不变。跨 resource 合并仍由下一项跟踪。 |
| 跨 resource 章节 | parser 的 TOC fragment 文本只截取起始 XHTML；Reader capability 也只服务一个 XHTML。上游 `getContent` 可从当前 resource 连续读到下一目录 resource。 | `must-fix（独立正确性项）` | catalogue 必须保存足够的 spine 边界；纯文本和可见 Reader 内容都要覆盖当前章到下一目录边界。可分后续提交实施，但性能重构不得固化现有丢失。 |
| 数据兼容 | SQLite/`chapters.json` 已保存 resource path 和 fragment；旧 archive/cache 可惰性恢复。 | `aligned` | 不做破坏性迁移；旧正文 snapshot/cache、缺失 marker 和旧 metadata 均继续可读，并在首次访问时安全升级派生数据。 |

## 3. 目标架构与状态合同

1. EPUB 解析拆成两个明确阶段：

   - **Catalogue**：有界校验 ZIP central directory，读取 container、OPF、NAV/NCX；按所选六规则
     生成标题、顺序、resource/fragment 和下一目录边界。仅在上游确实需要标题回退时读取该
     XHTML 的 `<title>`，不抽取 body 纯文本。
   - **Materialize**：只在确认导入、正文搜索或读章需要时，从已经验证的本地 stage/archive/
     extraction 生成派生正文。相同 resource 的多个 fragment 必须共享一次读取/DOM 解析。

2. `PreparedImport` 继续绑定 owner、版本、extension、精确 trimmed rule 和 staged raw SHA-256。
   新 catalogue-only snapshot 与上一批 full-content snapshot 都必须可确认；旧 cache 缺字段时按
   full snapshot 或安全重建处理，不能向用户返回 `invalid token`。

3. 确认导入的数据库/文件事务仍以“无书架广播、无 token 消耗、无残留新 archive”为失败原子性。
   EPUB 派生 extraction 可以在该新 archive 下准备；任何提取/事务失败必须由既有补偿删除整个
   新目录，不能触碰旧书或 mounted source。

4. 新导入 EPUB 的第一次 `GET /api/books/:id/chapters/:index/content` 不应重新读取、哈希或解压
   整本 source。旧书或被清 cache 的书允许第一次安全构建；构建完成后，同一 source identity
   的后续章节和 CSS/图片/字体资源均复用。

5. extraction 快路径只有在 marker fingerprint 格式有效，且 marker size/mtime 与当前
   regular source 完全一致时成立。任何 source identity 变化、marker 损坏、目录不完整或路径
   越界都必须回到 SHA-256 + 原子重建；不能用性能优化绕过 capability 的 archive 绑定。

6. API 公共 schema、JWT、书架即时 upsert/WebSocket、旧 URL 和 SQLite rows 不变。EPUB
   `content` 字段仍提供当前章可搜索纯文本，但允许按章惰性生成；Reader iframe 不应等待无关
   章节的纯文本 cache。

## 4. 契约测试

| 编号 | 测试 | 断言 |
|---|---|---|
| EPUB-PERF-A | engine catalogue fixture：数百个 XHTML，每个 body 由会触发 `MaxParsedTextBytes` 的大文本构成，NAV 提供完整标题。 | Catalogue 预览成功且正文为空；不触发 body text limit。完整 materialize 仍受该 limit 拒绝。六规则目录顺序/标题/resource/fragment 与既有完整 parser 一致。 |
| EPUB-PERF-B | 标题 fixture：TOC 标题、HTML `<title>`、正文 `h1/h2` 故意不同。 | 六规则严格使用上游 TOC/spine `<title>` 优先级；`h1/h2` 不冒充 resource title；首个无标题 spine 仍为“封面”。 |
| EPUB-PERF-C | importer preview→confirm fixture，给 ZIP/resource 读取和 DOM materialize 计数。 | 预览不物化 body；确认不重新上传/重跑 catalogue，且同一 XHTML 最多物化一次；旧 full-content snapshot 仍可确认。 |
| EPUB-PERF-D | epubreader service marker/fingerprint fixture。 | 已准备且 source size/mtime 相同的首次 `PrepareChapter` 为零次 SHA-256/零次 extraction；替换 source 或篡改 marker 时必须重新哈希/重建并拒绝旧 capability。 |
| EPUB-PERF-E | 删除一个 EPUB chapter text cache 后请求该章。 | 只重建目标章节及其合法跨-resource 范围，不调用整本 parser；其它章节 cache/SQLite 不变。 |
| EPUB-PERF-F | 真实 Go + Chrome，代表性大 EPUB，1440×900、390×844、360×800。 | 上传预览、规则刷新、确认、书架即时显示、首次进入、下一章和返回书架全部成功；记录阶段耗时/请求数，无 5xx、console/page error、重复上传或 Reader 白屏。 |
| EPUB-PERF-G | 历史 volume：旧 EPUB 无 extraction、旧 marker、旧 full-content parsed snapshot、缺 fragment row。 | 均可惰性读取/升级；`data/`、`cache/`、`library/`、backup/WebDAV 和原 archive 不丢失、不迁移到宿主绝对路径。 |

性能门禁不采用脆弱的固定毫秒值作为单元测试结论；以“读取/哈希/解压/DOM 次数”和真实浏览器
阶段计时共同判断。Docker 发布前仍须运行全量 Go/前端测试、生产 build、三视口 EPUB smoke
和旧 volume/backup smoke。

本批验证结果（2026-07-18）：

- `go test ./...` 全量通过；engine、importer、epubreader、API 与日志脱敏契约均有直接测试。
- 前端 426 项测试及 Vite 生产构建通过。
- `reader-epub-contract.mjs` 与 `local-book-import-contract.mjs` 在 1440×900、390×844、360×800
  全部通过；实际访问日志中的 WebSocket 查询显示为 `/ws/sync?<redacted>`。
- 本地候选镜像通过 `HISTORICAL_VOLUME=1` volume/portable-backup smoke，覆盖 TXT、EPUB、UMD、
  CBZ、旧相对缓存和 owner isolation。

## 5. 允许差异与分批边界

- 允许 OpenReader 在确认导入期提前准备受限 extraction，以换取用户明确要求的“书架进入阅读器
  更快”；这是本地 Go 运行时优化，必须记录为 `intentional-redesign`，且失败原子性不能弱于上游。
- 允许按 source size/mtime 复用当前机器自己写入的完整 marker；身份变化时仍以 SHA-256 为最终
  权威，不允许只用 mtime 作为跨 archive capability。
- 不允许恢复上游无边界 ZIP 解压、公开 `/epub/` 路径、浏览器可猜测文件 URL或全局资源目录。
- 本批可以先交付 catalogue-only preview + extraction 快路径，再在下一小批交付跨-resource Reader
  合并；但前一批必须把该缺口保留为 `must-fix`，不能宣称 EPUB 全量对齐。
