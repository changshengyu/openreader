# EPUB 固定基准目录与 Web Reader 资源边界合同

状态：**2026-07-18 已完成固定基准复审、失败测试、实现、全量自动化、三视口真实浏览器验证、历史 volume/portable backup 门禁和双架构 Docker 发布。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同纠正此前把 Legado 原生正文辅助分支当成固定基准 Web Reader 行为的错误。当前
OpenReader 文件、既有 fragment 测试和历史合同都不构成正确性依据。

## 1. 固定上游的实际数据流

| 层 | 固定上游证据 | 可观察合同 |
|---|---|---|
| EPUB TOC 数据 | `me/ag2s/epublib/domain/TableOfContents.java#getAllUniqueResources()` 使用 `Resource.href` 集合去重，只返回 `List<Resource>`，不返回 `TOCReference.fragmentId`。深度优先遍历每一个引用时都会先调用 `TitledResourceReference#getResource()`；只要 reference title 非 `null`（空串也算），就会覆盖同一个 `Resource.title`。 | `toc` 主列表是一 resource 一项；同一 XHTML 的多个 `#fragment` 不生成多个目录项。最终使用最后一次遍历写入的非 `null` TOC title；若它为空串，再回退 XHTML `<title>`。 |
| 本地 EPUB 目录 | `EpubFile.kt#getChapterList()` 遍历 `tableOfContents.allUniqueResources`，只把 `resource.href` 写入 `BookChapter.url`；不写 `startFragmentId`、`endFragmentId` 或 `nextUrl`。该列表来自整本 manifest/resource 集合，不要求 resource 同时存在于 spine。 | 新建目录 row 不携带 fragment；`toc`/`toc+spin`/`toc<spin` 的顺序以首个唯一 href 出现位置为准，合法的 TOC-only XHTML 也保留。 |
| Spine 与混合规则 | `getChapterListBySpine()` 一 spine resource 一项。两个混合方法都**先**调用 `getChapterList()`，该调用已经把最终 TOC title 写回与 spine 共用的 `Resource`，再调用 `getChapterListBySpine()`；源码注释也明确“如果读取了 toc，那么 spin 就会使用 toc 的章节名”。纯 `toc` 直接返回 TOC list；只有混合方法在一侧为空时回退另一侧。 | 六种公开规则仍保留；`spin` 单独使用文档标题，而其余含 TOC 的五种规则对 TOC 已引用的 resource 都观察到最终 TOC title（空串则文档标题）。`+`/`<` 的布尔标题覆盖分支在这些 resource 上通常不再造成可见差异；`toc` 无 TOC 时是空目录，`toc+spin`/`toc<spin` 才回退 spine。主列表顺序和仅存在于某一列表的 resource 仍有差异。 |
| Web 章节 API | `BookController.kt#getBookContent()` 对本地 EPUB 先 `extractEpub()`，随后直接返回 `chapterInfo.url` 对应的解压 XHTML URL，并在该分支提前返回；它不调用下面的 `LocalBook.getContent()`。 | Web Reader 一个目录项加载一个 XHTML resource；API 不拼接从当前 resource 到下一目录 resource 的 DOM。 |
| Web Reader | `web/src/components/Content.vue#renderEpub()` 只渲染一个 `iframe :src="apiRoot + content"`。相对 CSS、图片、字体与链接由该 XHTML 的浏览器 URL 解析。 | OpenReader 继续使用单 iframe + capability 是技术栈/安全等价实现；无需制造多 iframe 或合成跨 resource HTML。 |
| 正文搜索辅助分支 | `BookController.kt#searchBookContent()` 调用 `BookHelp.getContent()`，再进入 `EpubFile#getContent()`。该方法具备 `nextUrl`/fragment 跨 resource 代码，但固定目录创建路径从未写入这些字段；仓库内也没有其它本地 EPUB `nextUrl` 写入点。 | 这是固定源中的内部不完整/缺陷分支，不能反推 Web Reader 必须合并可见 resource。OpenReader 保持“每个目录 resource 只搜索自身正文”，避免把后续全书错误计入当前章；记录为用户已要求搜索稳定与加载性能下的 `intentional-redesign`。 |

## 2. 当前 OpenReader 差异矩阵

| 合同层 | 当前证据 | 判定 | 目标 |
|---|---|---|---|
| TOC 去重键 | `buildEPUBChaptersWithResources()` 现按 canonical href 去重，保留首次顺序和最后 title 写入；合法 TOC-only XHTML 进入 TOC 主列表，纯 `toc`/混合空目录语义也已分开。 | **aligned（本批实现）** | NAV/NCX、六规则、最后空 title 和 TOC-only fixture 已锁定。 |
| 新目录 metadata | 新 preview/import/refresh 的 SQLite 与 `chapters.json` 现每 href 一 row，fragment 字段为空；升级前 staged snapshot 仍可确认。 | **aligned + staged compatibility** | nullable 字段与历史 slice 读取继续保留，不做破坏性迁移。 |
| Web Reader iframe | `ReaderEpubContent.vue` 一个 capability iframe；跨 XHTML 链接由 bridge 映射回标准 Reader 章节事务。 | **technical-stack-equivalent** | 保留 capability/CSP/返回手势修复；不新增跨 resource 合成容器。 |
| 纯文本与内容搜索 | 新导入确认期、cache miss 和搜索均以当前 `resourcePath` 为边界。 | **intentional-redesign** | 保持当前 resource 边界，避免固定上游辅助分支在缺失 `nextUrl` 时吞入后续全书；不改变 Web Reader 可见内容。 |
| 历史 fragment rows | 已发布版本可能已有 `(resourcePath, resourceFragment)` 多 row，书签/进度可能指向这些 index。普通读章继续按签名 slice 返回；显式刷新时按 canonical resource path 将旧引用映射到新 row。 | **data-compatible（本批实现）** | 升级启动不合并、不删除旧 rows；无法匹配 resource 的引用沿用 index/清 ID 兼容规则。 |
| 历史缺失 resource path | 旧 SQLite/`chapters.json` 可能只有目录 index，且曾把 `toc` 规则用于无 NAV/NCX 的 EPUB。固定规则重解析会得到空目录。 | **data-compatible runtime fallback** | 仅在旧 row 缺失 `resourcePath` 且纯 `toc` 恢复结果为空时，用 spine 定位同 index resource 并回填；新 preview/import/refresh 仍保持纯 `toc` 空目录。 |
| 相对资源与安全 | capability 绑定 user/book/fingerprint/path，XHTML 经过 CSP/DOM 清理。 | **acceptable-change（安全）** | 继续保留，不回退到上游公开解压目录。 |

## 3. API、数据与状态合同

1. `POST /api/imports/books/preview`、确认导入及 `refresh-local` 的公开 schema 不变；变化仅是
   `toc*` 规则对重复 fragment 引用返回一个 resource chapter。
2. 新 chapter 的 `resourcePath` 为规范 archive POSIX path；`resourceFragment` 和
   `resourceEndFragment` 为空。SQLite nullable 列、旧 `chapters.json` 字段和旧 capability
   读取逻辑继续存在，不能通过迁移删除。
3. `toc` 主列表顺序取每个 manifest 中合法可读 resource 在 TOC 深度优先遍历中首次出现的位置；
   resource 不需要同时位于 spine。同 path 的重复
   TOC reference 不增加章节。每次 reference title 非 `null` 都按遍历顺序写回 title，包含空串；
   最终空串再回退当前 XHTML `<title>`。
4. `spin` 单独使用 spine 顺序及 resource/XHTML 标题。其余含 TOC 的规则都先读取 TOC，故
   spine 与 TOC 共用 resource 已被写入最终 TOC title：`spin+toc` 与 `spin<toc` 对这些 resource
   产生相同标题；`toc+spin` 与 `toc<spin` 也相同。`spin*` 仍以 spine 为主列表，`toc*` 仍以
   去重 TOC 为主列表；只存在于一侧的 resource 按各自方法的原有回退/排除规则处理。纯 `toc`
   无有效 TOC 时返回空目录；两个 `toc` 混合规则才回退 spine。
5. Reader 每次只显示当前 resource；同 resource hash 可以原地滚动，跨 resource link 继续通过
   parent Reader 事务切换目录，不能写 iframe session history 或改变移动返回书架语义。
6. 旧 fragment row 继续按已签名 slice 读取。它是历史兼容路径，不是新目录生成依据。
7. 旧 row 缺失 `resourcePath` 时，运行时先按持久化规则恢复。若规则恰为纯 `toc` 且 archive
   没有有效 TOC，允许只为该旧 row 按 spine 同索引恢复并安全回填；这个升级回退不参与新目录生成。

## 4. 先失败后通过的契约测试

| 编号 | 夹具与断言 | 覆盖 |
|---|---|---|
| EPUB-FIXED-1 | NAV 与 NCX 都依次引用 `one.xhtml#part-a`、`one.xhtml#part-b`、`two.xhtml#opening`，前两个标题不同，并包含“最后 title 为空串”的变体。重复 href 收敛；顺序由首次 href 决定，非空变体使用最后 TOC title，空串变体回退 XHTML `<title>`；fragment 字段为空。另含一个合法 TOC-only XHTML，断言只进入 `toc*` 主列表；无 TOC 时纯 `toc` 为空而 `toc+spin` 回退。六规则同时锁定 TOC→Resource title side effect。 | engine 六规则 |
| EPUB-FIXED-2 | preview→token confirm→SQLite/`chapters.json`。新导入只存两章；升级前生成的 full-content 与 catalogue-only staged snapshot 均继续确认；旧三 fragment row 启动后不被自动合并。 | importer、API、data |
| EPUB-FIXED-3 | `refresh-local` 把当前书按两 resource 重建，但原 archive hash 不变；旧 fragment row 的进度/书签优先按 canonical resource path 映射到新索引并绑定新 row。无法识别 resource 的历史引用沿用既有“保留 index/offset、清空失效 chapterId”规则，书签数据不删除。 | lifecycle、progress/bookmark |
| EPUB-FIXED-4 | 一个 resource 正文不包含下一个 XHTML 的唯一标记；内容搜索也不把下一 resource 命中归到当前章。 | chapter API、search |
| EPUB-FIXED-5 | 真实 Go + Chrome 在 1440×900、390×844、360×800 验证 fixture 目录为“封面 + 两个正文 resource”三项、单 iframe、同 XHTML 连续正文、跨 resource link 进入下一章、浏览器/手机返回书架、相对 CSS/图片/字体及工具层状态不回归。 | Reader browser smoke |
| EPUB-FIXED-6 | API 数据夹具证明旧 fragment rows 可继续打开并在显式刷新后按 resource path 收敛；历史 Docker volume 另验证旧 EPUB 可读取，纯 `toc`/无 TOC 的默认刷新安全拒绝且旧章仍可读，显式切换 `spin` 后刷新成功；数据库、archive、cache roots、用户隔离和 portable backup 不破坏。 | API historical rows + Docker historical volume/backup |
| EPUB-FIXED-7 | 旧 volume fixture 删除 EPUB resource columns，并保存“纯 `toc` + 无 NAV/NCX + spine 有正文”；运行时按 spine 恢复缺失 path，而 engine 对同 archive 的新纯 `toc` 目录仍为空。 | epubreader runtime migration + engine contract |

## 5. 允许差异与非目标

- capability、CSP、受限 extraction、多用户隔离和 archive 限额是必须保留的安全适配。
- 旧 OpenReader fragment row 的只读/继续阅读兼容是数据兼容，不代表新解析继续偏离固定上游。
- 本批不实现多 iframe、DOM 跨 resource 拼接或新增 schema；这些都不是固定 Web Reader 合同。
- EPUB 内容搜索按当前目录 resource 限定，是对固定上游缺失 `nextUrl` 所导致错误扩张的显式
  修正，并符合用户此前要求的搜索稳定和首次加载性能。

## 6. 实现与验证记录

- `backend/engine/parser_test.go` 覆盖 NAV/NCX href 去重、六规则实际标题副作用、TOC-only
  resource、纯 `toc` 空目录、最后空 title 回退、首封面和全文 resource 边界。
- `backend/services/localbook/importer_test.go` 证明新 catalogue-only snapshot 正常物化，且升级前
  full-content / catalogue-only fragment snapshot 都不会变成 `invalid token`。
- `backend/api/api_test.go` 覆盖 preview→confirm→SQLite/`chapters.json`、当前 resource 搜索、
  历史 fragment row 普通读取、显式刷新不改原 archive，以及进度/书签按 resource path 重绑。
- `backend/services/epubreader/resource_runtime_test.go` 覆盖旧纯 `toc`/无 TOC row 缺失
  `resourcePath` 时的 spine 定位回填；该回退不进入 parser/import/refresh。
- Go 全量、前端 426 项测试和 Vite 生产构建通过。
- `reader-epub-contract.mjs` 与 `local-book-import-contract.mjs` 在 1440×900、390×844、360×800
  均通过；无 5xx、console/page error 或移动返回历史回归。
- 本地候选镜像的 `HISTORICAL_VOLUME=1` 门禁通过：旧 EPUB 缺失 path 可读取；纯 `toc`/无 TOC
  默认刷新安全拒绝且旧章仍可读；显式 `spin` 刷新、原 archive 哈希、用户隔离、portable
  backup 到全新卷、恢复后刷新/重启均通过。
- 源码 commit `e50f4a60179140115660870e5679085bcc1ab117` 已本地构建并发布为
  `ghcr.io/changshengyu/openreader:e50f4a6` 与 `latest`。两 tag 的远端 OCI index digest 均为
  `sha256:326505799f740fcbe0beef1ba658f4d17e4435858d413009065493703dd16ee9`；包含
  `linux/amd64@sha256:a0e773290ee3e0e405e1861177e4b151fd106c92cb83026d49738ad699c58458`
  和 `linux/arm64@sha256:5f4fd7c1cfd032b427093f1a09bfa77b87c96b250dc9fde18e782cb2995dfe55`。
