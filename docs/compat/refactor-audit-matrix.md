# OpenReader 全量上游复审矩阵

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游工作副本：`/private/tmp/reader-dev-upstream-audit`。本矩阵创建于
2026-07-13，用来替代“当前实现/既有测试通过即可视为重构完成”的判断方式。

## 判定规则

- **已复核一致**：本次已同时核对上游源码、当前源码和对应状态转换；仍须在模块发布前重跑真实浏览器/后端门禁。
- **技术栈等价**：Vue 3、Pinia、Gin、多用户或安全适配改变了结构，但已明确不改变上游可见操作流程。
- **允许差异**：仅限用户明确要求的连续滚动与减号/数值/加号设置控件，或有明确的数据/安全理由的适配。
- **必须重建**：本次源码审查已经发现用户可见行为、默认值、状态或数据语义偏离上游。
- **尚未验证**：历史文档或测试声称完成，但本轮尚未以固定上游基准重新核查；不得据此继续扩展功能或宣称对齐。

“历史证据”只说明曾经做过什么，不能取代本矩阵的本轮复核。

## 总览矩阵

| 范围 | 上游权威文件 / 动作 | 当前映射 | 本轮结论 | 后续门禁 |
|---|---|---|---|---|
| Index 工作台、搜索、探索、侧边栏 | `web/src/views/Index.vue` | `layouts/AppLayout.vue`、`views/Home.vue`、`Search.vue`、`Discover.vue`、`stores/indexWorkspace.js` | **P1-A、P1-B 已完成**：根场景/旧链接、三态结果、260/270 手势、24 并发、无书源语义、标题续页、稳定 cursor、跨页去重和陈旧请求隔离已验证；搜索/探索远程卡遵循封面→共享 BookInfo、正文→无持久化临时 Reader；书架、搜索、探索、已保存/临时 Reader、旧链接的 BookInfo 五入口和关闭 query 清理均已三视口复核。 | `docs/compat/index-workspace-p1-contract.md`、`docs/compat/index-search-p1b-contract.md`；P2 只继续审查书架态字段变更/数据副作用，不能重开第二套 BookInfo 流程。 |
| 书架、BookManage、BookGroup、BookInfo | `BookShelf.vue`、`BookManage.vue`、`BookGroup.vue`、`BookInfo.vue` | `Home.vue`、`OverlayBookManagement.vue`、`OverlayBookGroups.vue`、`BookInfoDialog.vue`/`BookInfoPanel.vue` | **BookManage / BookGroup 的 P1-D/P2 复审已验证；BookInfo 五入口已由 P1-B 验证**：管理与分组均为 Index 内单一 Dialog（紧凑视口全屏）；真实 Go API 三视口已验证预选、空分组无请求拦截、设置、批量分组和确认删除的 Vue→Go→SQLite 链。用户隔离的多对多分组替代上游 bit mask 属于技术栈等价。书架态封面/编辑/追更/本地刷新等副作用仍需 P2 逐动作审查，但不得以此否定已完成的共享 BookInfo 入口矩阵。 | 保留 Go 数据清理/隔离回归；P2 对 BookInfo 仅建立书架态字段变更、缓存和同步语义合同。 |
| 书源与搜索结果 | `Index.vue`、`Explore.vue`、`BookSource.vue`、`BookSourceController.kt` | `SourceManager.vue`、`Search.vue`、`Discover.vue`、`SourceSwitchPanel.vue`、`backend/api/sources.go` | **P1-C 工作台收敛、P2 失效缓存、Reader 换源、CSS/JSONPath/XPath 主解析链、脚本能力透明化和元数据后处理均已实施验证并发布 `e2f9f31`**；书名/作者/简介现共享固定上游转换，`canReName` 恢复为配置存在标志，真正脚本入口的安全差异仍显式。全局书源 + 用户编辑权限仍是已记录的数据模型重设计。 | [`booksource-metadata-normalization-p2-contract.md`](booksource-metadata-normalization-p2-contract.md)；后续只抽取尚无合同的新解析语义，不重开已完成的解析主链。 |
| 本地导入、书仓、WebDAV | `BookController.kt`、`LocalBook.kt`、`TextFile.kt`、`EpubFile.kt`、`UmdFile.kt`、`CbzFile.kt`、`WebDAV.vue` | `OverlayBookImport.vue`、`LocalStore.vue`、`WebDAVBrowser.vue`、`OverlayStorageImport.vue`、`useStorageImportWorkflow.js`、`backend/services/localbook/*`、`engine/*_parser.go` | **P1-E1、P1-E2、P1-E3 已完成；P1-E4 固定 EPUB 目录和 CBZ 运行时纠正均已实现、验证并发布**：EPUB 目录按 href 去重，支持 TOC-only resource 和实际标题副作用；CBZ 保留 archive-first cover 与字典序图片章节，新导入预建私有不可变派生树。两格式的旧卷惰性恢复、原 archive 不变、portable backup 和用户隔离均已通过 Docker 门禁。 | [`epub-fixed-baseline-catalog-reader-contract.md`](epub-fixed-baseline-catalog-reader-contract.md)、[`reader-cbz-fixed-baseline-p0-contract.md`](reader-cbz-fixed-baseline-p0-contract.md)；其余格式/工作台后续矩阵不得重开已完成合同。 |
| 用户、备份、RSS、替换规则、书签 | `UserManage.vue`、`WebDAV.vue`、`Rss*`、`ReplaceRule*`、`Bookmark*` 及对应 Kotlin 控制器 | `OverlayUserManagement.vue`、`OverlayBackups.vue`、`RSSManager.vue`、`OverlayReplaceRules.vue`、`OverlayBookmarks.vue`、Go API/services | **已完成各自抽取的 P2 切片**：用户规则、独立 WebDAV/书仓权限和安全删除已实施；全局书源所有权明确为多用户数据模型差异，不伪造单用户书源按钮；备份/WebDAV、RSS 三层 Dialog、替换规则、段落书签和可移植本地 archive 均有 API/三视口/卷证据。 | 保留 [`user-management-p2-contract.md`](user-management-p2-contract.md) 与各 focused contract；后续只审查尚未提取的新动作，不重开已完成的数据流。 |
| Reader：工具层、面板、正文、翻页 | `Reader.vue`、`Content.vue`、`ReadSettings.vue`、`PopCatalog.vue`、`BookShelf.vue`、`BookSource.vue` | `views/Reader.vue`、`components/reader/*`、`composables/useReader*`、`stores/reader.js` | **P0 持续复核；连续跨章二次纠正已完成并发布**：标题排版、书源可用性、移动最终顶部顺序、浮动按钮、底部渲染页进度和竖向真实动画时长已完成；连续模式现按顶部安全区确定章/offset，窗口替换期隔离进度，并以 book/mode/generation 丢弃旧 append/retry。 | [`reader-continuous-fixed-baseline-p0-contract.md`](reader-continuous-fixed-baseline-p0-contract.md)；单元 434/434、continuous/mobile/image/text/TTS/volume/audio、真实 EPUB/CBZ 浏览器、历史卷/backup 与双架构 Docker 均通过。 |
| Reader：EPUB、漫画/CBZ、音频、连续跨章、TTS | `Reader.vue`、`Content.vue`、本地格式解析类 | `ReaderChapterContent.vue`、`ReaderEpubContent.vue`、`ReaderAudioContent.vue`、`ReaderTTSBar.vue`、`useReaderChapterReady.js`、格式 parser / cache | **EPUB、CBZ、连续跨章、音频和 TTS 固定基准切片均已完成实现、三视口验证和 Docker 发布**：音频恢复上游结构、边界行为与真实 autoplay；TTS 恢复显式 voice、贴底栏、可取消跨章和关闭段落定位。 | [`reader-audio-tts-fixed-baseline-p0-contract.md`](reader-audio-tts-fixed-baseline-p0-contract.md) 及前三份格式合同；本批 frontend 444/444、Go/build、Reader 全矩阵通过，镜像 `5260efd`/`latest` 已发布。当前 volume 脚本受 Codex socket 授权额度阻断，兼容证据继承无后端/持久化差异的 `370d0f7` 已通过门禁。 |
| Pinia 状态、缓存、同步、数据事务 | `plugins/vuex.js`、`plugins/cache.js`、后端 controller/model | `stores/*.js`、`utils/*cache*`、`backend/models`、`services`、`sync` | **P2 进行中**：书架 revision、本地即时变更与重连强刷已保留；本轮又确认冷启动抢先提交旧持久缓存、Hub backpressure 静默丢事件及 connected 阻止前台校准为 `must-fix`。其它 store/缓存/事务仍须逐项复审。 | [`bookshelf-network-first-sync-p2-contract.md`](bookshelf-network-first-sync-p2-contract.md)；先写网络优先、慢客户端和双客户端收敛测试，再实施。 |
| Go REST、鉴权与错误语义 | Kotlin `*Controller.kt`、`ReturnData.kt` | `backend/api/*.go`、`middleware/auth.go`、前端 `api/*.js` | **尚未验证**：Go 路径可不同，但每项上游动作都要有参数、响应、错误、副作用映射。 | 路由契约测试、401/403/404/400、前端错误文案和多用户测试。 |
| 书源解析、RSS、远程抓取 | `AnalyzeRule*`、`Rss*`、`BookSourceController.kt` | `backend/engine/source_*.go`、`rss_parser.go`、fetcher | **尚未验证**：安全限制是允许差异，但不能静默改变可支持规则或失败语义。 | `booksource-parser-compat`、golden fixtures、SSRF/重定向/大小限制测试。 |
| 测试、构建、Docker、卷升级 | 上游功能契约；OpenReader Docker/data 约束 | `frontend/tests`、`scripts/smoke`、`backend/**/*_test.go`、Dockerfile、release scripts | **当前发布 `e2f9f31` 已验证**：前端 448/448、Go 全量、生产构建、真实 CSS/JSONPath/XPath 解析工作流、书源工作台三视口、当前镜像 volume/backup、远端 amd64/arm64 manifest 均通过；以后每次发布仍必须以新提交重跑。 | `openreader-regression`、`openreader-docker-release`；当前 index `sha256:71241114608d4489ff21aebdfd6af1415780e3291b1781321df83e509e0e62e3`。 |

## P0 Reader 重新审查（已完成的源码证据）

| 项目 | 上游证据 | 当前证据 | 判定与必须动作 |
|---|---|---|---|
| 进入移动阅读器的工具层 | `Reader.vue` data: `showToolBar: true`。 | `Reader.vue`: `mobileChromeVisible = ref(true)`。 | **已复核一致**；加载章节不可暗中改为隐藏。 |
| 主面板打开后的工具层 | `eventHandler()` 在书架、书源、目录、设置任一 popover 打开时直接返回，不改 `showToolBar`。 | `useReaderPrimaryPanels` 仅切换面板；`useReaderPointer` 在主面板打开时返回。 | **已复核一致**；补全四个面板及全局对话框的点击穿透浏览器断言。 |
| 主 Popover 的移动端根几何 | `Reader.vue` 传入 `popperWidth = windowWidth - 33`；但 `App.vue` 的 `.mini-interface .popper-component { left:0; top:0; width:100vw !important; }` 是最终权威 CSS。 | `ReaderMobileWorkspacePanel.primary` 为无通用 padding 的 `(0,0,100vw,100dvh)` 根，内容组件自行持有内边距。 | **技术栈等价**；此前把 `windowWidth - 33` 当最终宽度的判断已撤销。不得把当前全宽根误改成抽屉或 33px 留缝。 |
| 主面板层级/点击 | 上游工具栏 `z-index:2001`，popover 在其下；正文点击在面板状态直接返回。 | 当前工具层 `z-index:8`、主面板 `z-index:7`，主面板停传播且 pointer/keyboard 有状态保护。 | **技术栈等价，待浏览器复验**；层级数字可不同，但工具层可见、面板不穿透、同工具关闭/A→B 原子切换必须固定。 |
| 移动顶部工具顺序 | 上游模板源顺序为书架/书源/目录/设置/首页；mini 模式给首页内联 `order:-1`，最终可见顺序为 **首页、书架、书源、目录、设置**。 | `ReaderMobileChrome.vue` 直接按最终可见顺序渲染；桌面顺序独立保留。 | **2026-07-17 已复核一致**：源码与三视口真实 DOM 均通过；见 [`reader-mobile-controls-p0-contract.md`](reader-mobile-controls-p0-contract.md)。 |
| 移动左侧浮动按钮 | 上游 mini 模式依次显示书签、搜索、信息、顶部、底部；顶部/底部分别调用 `toTop(0)` / `toBottom(0)`。 | 当前已补齐五项并复用 `scrollToTop` / `scrollToBottom`；不按格式隐藏、不修改工具层。 | **2026-07-17 已复核一致**：两种移动高度无重叠，滚动和正文几何浏览器合同通过。 |
| 移动底部进度 | 上游 mini 非音频滑条是 `1…totalPages` 当前渲染页，标签 `第 x/y 页`；底部中间另显示单行 `阅读进度: N%` 并打开缓存区。 | 已恢复 1-based 当前渲染页；scroll/scroll2 有真实页数；拖动不跨章；音频隐藏并收缩底栏；中间恢复单行进度。 | **2026-07-17 已复核一致**：[`reader-mobile-progress-p0-contract.md`](reader-mobile-progress-p0-contract.md)；保留更平滑全书百分比算法为允许差异。 |
| 阅读内书源入口 | 上游 `BookSource` 工具没有按本地/远程禁用；点击后由书源流程决定可用结果。 | `ReaderMobileChrome.vue` 和 `ReaderDesktopTools.vue` 用 `:disabled="!remoteBook"`；`useReaderPanels.openSource()` 对本地书直接返回。 | **必须重建**：入口始终可点，保留安全的空结果/提示，但不能在工具层消失。 |
| 移动正文横向几何 | 上游 mini `.chapter`: `width:100vw; padding:0 16px; box-sizing:border-box; text-align:justify`；slide 内容同样 16px 两侧留白。 | 当前 `.reader-page` 与 `.reader-body` 同样使用 100vw/16px/justify；工具层显隐不参与正文宽度。 | **已复核一致，待像素复验**：390 与 360 下首段左右可见留白误差不得超过 1px。 |
| 移动正文纵向起点 | 上游 `.content-inner`: `margin-top: 30px + safe-area`、`padding-top:15px`。 | 当前 `.reader-body` 使用相同语义。 | **已复核一致，待像素复验**。 |
| 标题元素和排版 | `Content.vue` 渲染章节标题为 `h3`；CSS 固定 `font-size:28px; line-height:1.2; margin:1em 0; text-align:center`。 | `ReaderChapterContent.vue` 渲染 `h1`，字号为 `fontSize × 1.36`、行高 1.35、普通模式底部 margin 76px、移动端 28px；`Reader.vue` 的书签上下文却只查询 `h3,p`。 | **必须重建**：恢复 `h3`、28px/1.2/1em 规则；同步连续阅读、书签、搜索定位、TTS 的 DOM 查询与分页测试。 |
| 段落语义 | 上游 `p` 有 `word-wrap:break-word`、`word-break:break-all`、首行缩进 2em，行高/间距来自配置。 | 当前保留首行缩进、行高/间距配置，但没有显式两个换行规则。 | **必须重建**：补齐上游断行语义；以长无空格文本的渲染/分页 fixture 验证。 |
| 中心点击、边缘翻页、自动阅读、TTS | `eventHandler()` 使用中间横纵各 20% 区域；主面板打开先返回；自动阅读点按切工具；TTS/read-bar 仅禁止中心菜单切换，边缘翻页保留。 | `readerInteraction.js` / `useReaderPointer.js` 同样以 20% 区域映射；TTS 仅抑制 `toggle-chrome`。 | **已复核一致，待重新运行**：旧单元/浏览器用例只可作覆盖起点，不能替代当前 DOM 改动后的回归。 |
| 翻页与滚动差异 | 上游离散翻页/滚动模式。 | 原生连续手指/滚轮滚动，点击仍分页。 | **允许差异**：用户明确要求；模式选择、中心点击和边缘点击仍要复刻上游。 |
| 设置数值控件 | 上游有离散选择及数值调整控件。 | `ReaderSettingStepper` 使用减号/数值/加号。 | **允许差异**：用户明确要求；默认值与存储语义仍必须与 `plugins/config.js` 对齐。 |

## 不能再沿用的历史测试假设

1. 测试不能把“当前元素存在”当作上游对齐。特别是 `ReaderMobileWorkspacePanel` 的存在只能证明 Vue 实现；必须同时检查上游的工具顺序、根几何、层级和状态转换。
2. `ReaderChapterContent` 的标题测试不得继续接受 `h1`，因为上游、书签定位和阅读内搜索的共同契约都是 `h3,p`。
3. 书源测试不得把“本地书按钮禁用”作为预期；需要验证入口可点，以及无候选时的明确空态/错误行为。
4. 每一个已有 smoke 都要在修改后的生产构建中重跑；mock API smoke 不能替代至少一个真实 Go 服务 + 已导入书籍阅读用例。

## 接下来的受控实施顺序

1. 只为上述三项 **必须重建** 的 Reader 偏差新增/替换单元和浏览器契约：工具顺序与书源入口、`h3` 排版/书签查询、长词断行及 16px 对称留白。
2. 通过测试定义后，删除本地书源禁用分支，按上游顺序调整移动工具栏，并把普通/卷/错误章节标题统一到 `h3` 语义。
3. 在 1440×900、390×844、360×800 跑文本、连续阅读、EPUB、图片、音频、TTS 的回归；文本 Reader 是本批 Docker 的最低发布门槛。
4. P0 发布后才进入 Index；每个 P1/P2 模块先将其从“尚未验证”变成有源码证据的专门合同，再编写代码。

## 2026-07-13 Reader P0-A 实施记录：工具入口与文本排版

> 2026-07-17 勘误：当时关于移动顶部顺序“已调整”的实施记录后来被回归覆盖，且遗漏了
> mini 模式的“顶部/底部”浮动按钮。当前权威状态以上方矩阵和
> [`reader-mobile-controls-p0-contract.md`](reader-mobile-controls-p0-contract.md) 为准。

完成项：

- 移动顶栏按上游最终可见顺序调整为 `首页 → 书架 → 书源 → 目录 → 设置`；桌面和移动的书源入口不再因为本地书而被禁用。
- 本地书点击书源会打开与远程书相同的候选来源面板；候选为空或请求失败仍由该面板给出结果，不在工具层静默拒绝。
- 普通、卷和行内错误章节标题恢复为 `h3`；标题 CSS 恢复为 `28px / 1.2 / 1em 0 / 居中`。书签上下文、阅读内搜索和 TTS 统一使用上游 `h3,p` DOM 范围。
- 段落补回 `word-wrap: break-word`、`word-break: break-all` 与既有的 `text-indent: 2em`，避免长无空格文本破坏分页和左右留白。

允许差异仍只有：原生连续手指/滚轮滚动而点击仍分页，以及数值设置的减号/数值/加号控件。

本批验证：

- `frontend/npm test`：364 个测试通过；新增工具顺序/本地书源入口/`h3` 排版与断行合同。
- `frontend/npm run build`：通过。
- `backend/go test ./...`：通过。
- 真实浏览器：`reader-mobile-contract.mjs`（1440×900、390×844、360×800）、`reader-continuous-contract.mjs`、`reader-image-contract.mjs`：通过。
- 未计为通过：`reader-tts-contract.mjs` 与 `reader-audio-contract.mjs` 在创建浏览器上下文前被 macOS 终止（Chrome `SIGABRT`）；没有触发任何产品断言，且 TTS 的 `h3,p` 单元合同已通过。此环境限制必须在后续独立浏览器窗口重跑，不能作为完整 Reader P0 完成的证据。

本批适合进行 Docker 的用户验收，范围仅是上述工具入口与文本阅读排版；EPUB、漫画、音频、TTS、连续跨章的最终 Reader P0 签收仍需完成各自的真实浏览器复跑。
