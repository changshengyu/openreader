# EPUB 固定基准目录与 Web Reader 资源边界合同

状态：**2026-07-18 重新审计完成；失败测试与实现待下一阶段。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同纠正此前把 Legado 原生正文辅助分支当成固定基准 Web Reader 行为的错误。当前
OpenReader 文件、既有 fragment 测试和历史合同都不构成正确性依据。

## 1. 固定上游的实际数据流

| 层 | 固定上游证据 | 可观察合同 |
|---|---|---|
| EPUB TOC 数据 | `me/ag2s/epublib/domain/TableOfContents.java#getAllUniqueResources()` 使用 `Resource.href` 集合去重，只返回 `List<Resource>`，不返回 `TOCReference.fragmentId`。深度优先遍历每一个引用时都会先调用 `TitledResourceReference#getResource()`；只要 reference title 非 `null`（空串也算），就会覆盖同一个 `Resource.title`。 | `toc` 主列表是一 resource 一项；同一 XHTML 的多个 `#fragment` 不生成多个目录项。最终使用最后一次遍历写入的非 `null` TOC title；若它为空串，再回退 XHTML `<title>`。 |
| 本地 EPUB 目录 | `EpubFile.kt#getChapterList()` 遍历 `tableOfContents.allUniqueResources`，只把 `resource.href` 写入 `BookChapter.url`；不写 `startFragmentId`、`endFragmentId` 或 `nextUrl`。 | 新建目录 row 不携带 fragment；`toc`/`toc+spin`/`toc<spin` 的顺序以首个唯一 href 出现位置为准。 |
| Spine 与混合规则 | `getChapterListBySpine()` 一 spine resource 一项。两个混合方法都**先**调用 `getChapterList()`，该调用已经把最终 TOC title 写回与 spine 共用的 `Resource`，再调用 `getChapterListBySpine()`；源码注释也明确“如果读取了 toc，那么 spin 就会使用 toc 的章节名”。 | 六种公开规则仍保留；`spin` 单独使用文档标题，而其余含 TOC 的五种规则对 TOC 已引用的 resource 都观察到最终 TOC title（空串则文档标题）。`+`/`<` 的布尔标题覆盖分支在这些 resource 上通常不再造成可见差异；主列表顺序和仅存在于某一列表的 resource 仍有差异。 |
| Web 章节 API | `BookController.kt#getBookContent()` 对本地 EPUB 先 `extractEpub()`，随后直接返回 `chapterInfo.url` 对应的解压 XHTML URL，并在该分支提前返回；它不调用下面的 `LocalBook.getContent()`。 | Web Reader 一个目录项加载一个 XHTML resource；API 不拼接从当前 resource 到下一目录 resource 的 DOM。 |
| Web Reader | `web/src/components/Content.vue#renderEpub()` 只渲染一个 `iframe :src="apiRoot + content"`。相对 CSS、图片、字体与链接由该 XHTML 的浏览器 URL 解析。 | OpenReader 继续使用单 iframe + capability 是技术栈/安全等价实现；无需制造多 iframe 或合成跨 resource HTML。 |
| 正文搜索辅助分支 | `BookController.kt#searchBookContent()` 调用 `BookHelp.getContent()`，再进入 `EpubFile#getContent()`。该方法具备 `nextUrl`/fragment 跨 resource 代码，但固定目录创建路径从未写入这些字段；仓库内也没有其它本地 EPUB `nextUrl` 写入点。 | 这是固定源中的内部不完整/缺陷分支，不能反推 Web Reader 必须合并可见 resource。OpenReader 保持“每个目录 resource 只搜索自身正文”，避免把后续全书错误计入当前章；记录为用户已要求搜索稳定与加载性能下的 `intentional-redesign`。 |

## 2. 当前 OpenReader 差异矩阵

| 合同层 | 当前证据 | 判定 | 目标 |
|---|---|---|---|
| TOC 去重键 | `buildEPUBChapters()` 以 `(path, fragment)` 去重，并为同 XHTML 的相邻 fragment 生成 `ResourceEndFragment`。 | **must-fix（错误重构）** | 新解析按 canonical resource path 去重；首个 href 决定顺序，最后一次非 `null` TOC title 写入决定 title，最终空串回退 XHTML `<title>`。 |
| 新目录 metadata | importer 把 parser 生成的 fragment 起止值写入 SQLite 与 `chapters.json`。 | **must-fix for new/refresh** | 固定基准生成的新目录不写 fragment。字段和读取能力保留给历史 OpenReader 数据，不做破坏性迁移。 |
| Web Reader iframe | `ReaderEpubContent.vue` 一个 capability iframe；跨 XHTML 链接由 bridge 映射回标准 Reader 章节事务。 | **technical-stack-equivalent** | 保留 capability/CSP/返回手势修复；不新增跨 resource 合成容器。 |
| 纯文本与内容搜索 | 新导入确认期、cache miss 和搜索均以当前 `resourcePath` 为边界。 | **intentional-redesign** | 保持当前 resource 边界，避免固定上游辅助分支在缺失 `nextUrl` 时吞入后续全书；不改变 Web Reader 可见内容。 |
| 历史 fragment rows | 已发布版本可能已有 `(resourcePath, resourceFragment)` 多 row，书签/进度可能指向这些 index。 | **data-compat requirement** | 升级启动不合并、不删除旧 rows；旧书可继续读。用户显式 `refresh-local` 后才按固定上游目录重建，并使用既有进度夹取/恢复规则。 |
| 相对资源与安全 | capability 绑定 user/book/fingerprint/path，XHTML 经过 CSP/DOM 清理。 | **acceptable-change（安全）** | 继续保留，不回退到上游公开解压目录。 |

## 3. API、数据与状态合同

1. `POST /api/imports/books/preview`、确认导入及 `refresh-local` 的公开 schema 不变；变化仅是
   `toc*` 规则对重复 fragment 引用返回一个 resource chapter。
2. 新 chapter 的 `resourcePath` 为规范 archive POSIX path；`resourceFragment` 和
   `resourceEndFragment` 为空。SQLite nullable 列、旧 `chapters.json` 字段和旧 capability
   读取逻辑继续存在，不能通过迁移删除。
3. `toc` 主列表顺序取每个有效 resource 在 TOC 深度优先遍历中首次出现的位置；同 path 的重复
   TOC reference 不增加章节。每次 reference title 非 `null` 都按遍历顺序写回 title，包含空串；
   最终空串再回退当前 XHTML `<title>`。
4. `spin` 单独使用 spine 顺序及 resource/XHTML 标题。其余含 TOC 的规则都先读取 TOC，故
   spine 与 TOC 共用 resource 已被写入最终 TOC title：`spin+toc` 与 `spin<toc` 对这些 resource
   产生相同标题；`toc+spin` 与 `toc<spin` 也相同。`spin*` 仍以 spine 为主列表，`toc*` 仍以
   去重 TOC 为主列表；只存在于一侧的 resource 按各自方法的原有回退/排除规则处理。
5. Reader 每次只显示当前 resource；同 resource hash 可以原地滚动，跨 resource link 继续通过
   parent Reader 事务切换目录，不能写 iframe session history 或改变移动返回书架语义。
6. 旧 fragment row 继续按已签名 slice 读取。它是历史兼容路径，不是新目录生成依据。

## 4. 必须先失败的测试

| 编号 | 夹具与断言 | 覆盖 |
|---|---|---|
| EPUB-FIXED-1 | NAV 与 NCX 都依次引用 `one.xhtml#part-a`、`one.xhtml#part-b`、`two.xhtml#opening`，前两个标题不同，并包含“最后 title 为空串”的变体。`toc*` 只生成 `one.xhtml`、`two.xhtml` 两项；顺序由首次 href 决定，非空变体使用最后 TOC title，空串变体回退 XHTML `<title>`；fragment 字段为空。六规则同时锁定上述 TOC→Resource title side effect。 | engine 六规则 |
| EPUB-FIXED-2 | preview→token confirm→SQLite/`chapters.json`。新导入只存两章；升级前生成的 full-content 与 catalogue-only staged snapshot 均继续确认；旧三 fragment row 启动后不被自动合并。 | importer、API、data |
| EPUB-FIXED-3 | `refresh-local` 把当前书按两 resource 重建，但原 archive hash 不变；现有进度索引被夹取到新目录范围，书签数据不删除。 | lifecycle、progress/bookmark |
| EPUB-FIXED-4 | 一个 resource 正文不包含下一个 XHTML 的唯一标记；内容搜索也不把下一 resource 命中归到当前章。 | chapter API、search |
| EPUB-FIXED-5 | 真实 Go + Chrome 在 1440×900、390×844、360×800 验证目录为两项、单 iframe、跨 resource link 进入下一章、浏览器/手机返回书架、相对 CSS/图片/字体及工具层状态不回归。 | Reader browser smoke |
| EPUB-FIXED-6 | 历史 Docker volume 中旧 fragment rows 可继续打开；显式刷新后目录收敛但数据库、archive、cache roots、用户隔离和 portable backup 不破坏。 | historical volume/backup |

## 5. 允许差异与非目标

- capability、CSP、受限 extraction、多用户隔离和 archive 限额是必须保留的安全适配。
- 旧 OpenReader fragment row 的只读/继续阅读兼容是数据兼容，不代表新解析继续偏离固定上游。
- 本批不实现多 iframe、DOM 跨 resource 拼接或新增 schema；这些都不是固定 Web Reader 合同。
- EPUB 内容搜索按当前目录 resource 限定，是对固定上游缺失 `nextUrl` 所导致错误扩张的显式
  修正，并符合用户此前要求的搜索稳定和首次加载性能。
