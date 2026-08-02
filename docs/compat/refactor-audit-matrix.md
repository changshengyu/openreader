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
| Index 工作台、搜索、探索、侧边栏 | `web/src/views/Index.vue` | `layouts/AppLayout.vue`、`views/Home.vue`、`Search.vue`、`Discover.vue`、`stores/indexWorkspace.js` | **工作台主流程及设置/操作面第二轮已实现并发布 `3746d62`**：后端状态只检查同源 health；刷新缓存重取当前账号工作台所有者；重复书架/RSS/替换规则入口已清理；Home 恢复 `编辑 → 刷新 → RSS → 书海`；260/270 手势、固定底部图标、认证会话和 owner-v1 缓存继续通过。原作者公众号/TG 明确不复制，JWT 管理员、同源后端状态与可移植备份为允许适配。 | [`index-settings-action-surface-second-audit-p1-contract.md`](index-settings-action-surface-second-audit-p1-contract.md) 已通过 frontend 685/685、Go/build/diff、Index 三视口、移动侧边栏双视口、缓存/工作台三视口及新旧卷；状态 `aligned / Docker-published / awaiting-device-verification`，OCI index `sha256:eb57e0094baeb7d0cc354a0b97e5d366059fe47032d83fd2b5f42819a3d9e23b`。书架卡片布局与 overlay 内部仍按各自专项复审。 |
| 书架、BookManage、BookGroup、BookInfo | `Index.vue` 普通 shelf、`BookManage.vue`、`BookGroup.vue`、`BookInfo.vue` | `Home.vue`、`OverlayBookManagement.vue`、`OverlayBookGroups.vue`、`BookInfoDialog.vue`/`BookInfoPanel.vue`、`BookCover.vue`、stores | **BookManage、BookGroup、BookInfo 第二轮已分别发布；普通书架可见布局第二轮已作为 `60984b6` 发布**。固定 380px grid、当前筛选计数、精确编辑搜索、历史 list→grid v3 迁移、独立 metadata DOM、loading/blank/night 和单一 750px 响应合同均已落地；外部 scoped CSS 的夜间选择器编译偏差也已由 computed-color smoke 修复。书架 freshness、进度和 lastCheckTime 专项继续保持关闭。 | [`bookshelf-visible-layout-fixed-baseline-second-audit-p1-contract.md`](bookshelf-visible-layout-fixed-baseline-second-audit-p1-contract.md)：frontend 689/689、Go/build/diff、1440×900/1024×1366/390×844/360×800 专项及移动侧栏、Index 操作面、工作台、刷新竞态、新旧卷/备份门全部通过；状态 `aligned / Docker-published / awaiting-device-verification`，OCI index `sha256:05c36dd96c1ba3d3a201b713a731d27bb26fe9c34988626437230d349b3e1ad8`。 |
| 书源与搜索/探索结果 | `Index.vue`、`Explore.vue`、`BookSource.vue`、`BookSourceController.kt` | `SourceManager.vue`、`Search.vue`、`Discover.vue`、`RemoteBookResultList.vue`、`RemoteBookJsonEditorDialog.vue`、`ExploreWorkspacePopover.vue`、presentation/composable、Go source APIs | **书源 owner/API/解析主链保持已发布；搜索/探索可见工作区第二轮已完成实现与验证，待本机 Docker 发布**。可见搜索现只保留 single/multi 并按场景显示控件；结果保持服务端顺序并与普通书架共用固定 380px grid/card；共享 JSON editor/add 双动作已恢复；Explore 恢复 desktop 600px/top0 和 mini 100vw 非全高、无 backdrop、多开并保留现场。错误的 source regroup 富卡片、独立 page header/empty/loading、三模式、520px/100vh modal 和 accordion 已删除或替换。 | [`index-search-explore-visible-workspace-fixed-baseline-second-audit-p1-contract.md`](index-search-explore-visible-workspace-fixed-baseline-second-audit-p1-contract.md) 状态 `implementation-complete / validation-complete / Docker-pending`；frontend `701/701`、Go/build/diff、Index 四视口、Remote Reader 三视口、双 session-isolation 与真实 CSS/JSONPath/XPath 书源流通过。P2-S1…S4 owner 隔离、稳定 cursor、JWT REST、临时 Reader 与安全 Go exploreUrl 解析保持不变。 |
| 本地导入、书仓、WebDAV | `BookController.kt`、`LocalBook.kt`、`TextFile.kt`、`EpubFile.kt`、`UmdFile.kt`、`CbzFile.kt`、`LocalStore.vue`、`WebDAV.vue` | `OverlayBookImport.vue`、`components/workspace/LocalStoreManager.vue`、`OverlayLocalStore.vue`、`WebDAVBrowser.vue`、`OverlayStorageImport.vue`、`useStorageImportWorkflow.js`、`backend/services/localbook/*`、`engine/*_parser.go` | **P1-E1…P1-E5 已完成**：用户可见流程、格式运行时、旧卷、portable backup 和用户隔离均已发布；`/local-store` 保持 Index overlay 兼容意图，不可达独立页面与 `embedded` 双壳已删除。P1-E5 通过 571 项前端、全量 Go、生产构建和三视口工作台合同，未改变文件/导入/API/数据行为。 | [`local-store-workspace-ownership-p1-contract.md`](local-store-workspace-ownership-p1-contract.md)、[`epub-fixed-baseline-catalog-reader-contract.md`](epub-fixed-baseline-catalog-reader-contract.md)、[`reader-cbz-fixed-baseline-p0-contract.md`](reader-cbz-fixed-baseline-p0-contract.md)。 |
| 外部 WebDAV 协议 | `WebdavController.kt` 的 `/reader3/webdav*`、Basic、OPTIONS/PROPFIND/MKCOL/PUT/GET/DELETE/MOVE/COPY/LOCK/UNLOCK | `backend/api/server.go`、`backend/api/webdav.go`、`middleware/webdav_auth.go`、`services/webdavfs` | **2026-07-22 已实施、全量验证并随 `544e1fb` 发布**：双前缀、Bearer/Basic、DAV 发现/列表、完整文件方法、调用者私有根、原子事务和逐组件 symlink 防护已通过 API/service/CORS、全量 Go/frontend/build、真实 Basic curl、三视口工作台、候选容器协议与新旧卷/便携备份门禁；现有 `/webdav` GET 适配器保留。 | [`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md)；后续 WebDAV 修改不得削弱 caller scope、事务和 symlink 合同。 |
| 用户、备份、RSS、书签 | `UserManage.vue`、`AddUser.vue`、`BaseController.kt#saveUserSession`、`WebDAV.vue`、`Rss*`、`Bookmark*` 及对应 Kotlin 控制器 | `OverlayUserManagement.vue`、`backend/api/auth.go`/`admin.go`、`WebDAVBrowser.vue`、`RSSManager.vue`、`OverlayBookmarks.vue`、Go services | UserManage、RSS、Overlay 会话隔离和 P2-S4 owner 隔离已发布。**Bookmark 2026-08-02 第二轮固定基准重建已完成并发布 `f2f0d6e`**：恢复 750–1000px manager/form、桌面/手机表格高度、mini 固定列、书名作者、空选择及导入/删除文案；legacy 无正文行在确认前过滤。段落上下文、添加当前段落、ID/顺序/事务/备份及账号/书籍隔离继续成立。 | [`bookmark-fixed-baseline-second-audit-p2-contract.md`](bookmark-fixed-baseline-second-audit-p2-contract.md)：frontend 654/654、Go/build，1440×900、390×844、360×800、1024×1366、1366×1024、真实 EPUB 三视口和新旧卷均通过；状态 `aligned / Docker-published`，OCI index `sha256:cfaba7d453bde2a4b44198aa57ef8ef5ecbaa3b4cce0ab77b4c816311455c736`。会话隔离证据仍见 [`workspace-overlay-authenticated-session-p1-contract.md`](workspace-overlay-authenticated-session-p1-contract.md)。 |
| 替换规则 | `ReplaceRule.vue`、`ReplaceRuleForm.vue`、`Reader.vue#filterContent/showTextFilterPrompt`、`ReplaceRuleController.kt`、`ReplaceRule.kt` | `OverlayReplaceRules.vue`、`useOverlayReplaceRules.js`、`services/replacerules`、`replace_rules.go`、`books.go#applyUserReplaceRules`、backup/restore | **P2 固定基准重建已完成并发布 `a7abcdd`**：manager/editor 可见结构、共享几何、Reader 直达 editor、精确导入/保存/scope、`id ASC` pipeline、JavaScript replacement string、name restore identity 和 durable-only broadcast 均已测试先行重建；旧新增/刷新/逐行删除/测试器/mobile cards 已删除。 | [`replace-rule-fixed-baseline-p2-contract.md`](replace-rule-fixed-baseline-p2-contract.md)：frontend 649/649、Go/build、1440×900/1024×1366/390×844/360×800 manager 与 Reader 桌面/手机/iPad、新旧卷均通过；`a7abcdd`/`latest` index 为 `sha256:93840cf72e9a0a783333ac5ab485551d892e42b9bf2e8eb2e2a1039e56b5dd53`。JWT/SQLite、legacy 空 scope、隐藏兼容 API，以及 RE2 pattern/32 捕获组/20,000 匹配/有界输出为明确允许差异。 |
| Reader：工具层、面板、正文、翻页 | `Reader.vue`、`Content.vue`、`ReadSettings.vue`、`PopCatalog.vue`、`BookShelf.vue`、`BookSource.vue` | `views/Reader.vue`、`components/reader/*`、`composables/useReader*`、`stores/reader.js` | **第十三次移动点击已发布 `49a273e`；第八轮夜间实际文字承载层修复始于 `6fde5ab` 并已包含在当前 `5459f02`**：黑色内容面按最终“不透明纯黑页面 + 无背景图”接管；普通正文 `p/span/mark` 与 EPUB 实际后代节点自身均显式纯黑/纯白，不再依赖祖先透明穿透，退出夜间完整恢复作者样式。非黑自定义主题与背景图保持不变。 | [`reader-night-contrast-p0-contract.md`](reader-night-contrast-p0-contract.md)：frontend 652/652、Go/build、普通 Reader 与真实 EPUB 1440×900/390×844/360×800 及新旧卷通过。当前 `5459f02`/`latest` index 为 `sha256:a8d04ee3e5f6aac3e2a41b908da175e17b7a57e1fb440df51677f35d9496afe9`；部署端 pull/force-recreate 且 `/api/health` 返回 `5459f02` 后才代表已升级。 |
| Reader：设置第二轮复审 | `ReadSettings.vue`、`config.js`、`vuex.js#setConfig/setNightTheme`、`Reader.vue` TTS/read bar | `ReaderSettingsPanel.vue`、`ReaderSettingStepper.vue`、`stores/reader.js`、`ReaderTTSBar.vue`、appearance/mode composables | **2026-08-02 固定基准重建已完成并发布 `40f124f`**：方案 allowlist、无损重置、normal/kindle 双快照、14 内置背景、独立主题纹理、五字体单操作、精确顺序/divider/操作区和 TTS 去重均已落地；亮度、可编辑 stepper、字体预览/字号预设、纯黑白夜间和连续滚动/离散点击为明确允许差异。 | [`reader-settings-fixed-baseline-second-audit-p0-contract.md`](reader-settings-fixed-baseline-second-audit-p0-contract.md)：frontend 680/680、Go/build/diff，1440×900、390×844、360×800、1024×1366、1366×1024、强制手机 iPad 与新旧卷全部通过；`40f124f`/`latest` OCI index 为 `sha256:d9395b19f45bfe9412facbcdcec63e776c881c13437c6049e70140f3f87e6b45`，状态 `aligned / Docker-published / awaiting-device-verification`。 |
| Reader：书内正文搜索 | `SearchBookContent.vue`、`vuex.js#dialog*`、`Reader.vue#showSearchContent`、`BookController.searchBookContent/searchChapter`、`SearchResult.kt` | `OverlayBookContentSearch.vue`、`useBookContentSearch.js`、`useReaderContentSearchIntent.js`、`useReaderSearchNavigation.js`、`readerBookSearch.js`、`backend/services/contentsearch`、`backend/api/books.go` | **2026-08-02 第二轮固定基准重建已完成并发布 `1801037`**。恢复动态 width/top/table、75% 非自动聚焦标题输入、100/250 列、非阻塞表格、上游 footer 与正常模式 gate；同 ID 换 URL 会 reset，前端 intent/Reader/现代及 legacy Go 全链保留原始空白查询。原始正文、精确/大小写/重叠、游标、取消、UTF-16、多用户、同/跨章跳转继续成立。 | [`book-content-search-fixed-baseline-second-audit-p2-contract.md`](book-content-search-fixed-baseline-second-audit-p2-contract.md)：frontend 659/659、Go/build，普通 Reader 桌面/手机/iPad、真实 EPUB 三视口及新旧卷通过；状态 `aligned / Docker-published`，OCI index `sha256:5d2fdb171e734d5debece77f91ae31495fc1ba7ee9eec28c88aa2b3f41eeeee5`。 |
| Reader：登录失效与账号切换 | `plugins/axios.js` 的 `NEED_LOGIN`、根 `App.vue#login`、`Reader.vue#loginAuth` | `api/client.js`、`App.vue`、`AuthDialog.vue`、`stores/user.js`、Reader lifecycle/progress、`stores/overlay.js` | **P0 已完成并发布 `59e11a9`**：401 按真实拦截顺序先挂起旧 Reader 再清凭证，未认证根场景不渲染私有 DOM；overlay reset、同账号 generation 重挂载、异账号返回书架、安全 returnTo、旧进度写入抑制均已验证。 | [`reader-reauthentication-isolation-p0-contract.md`](reader-reauthentication-isolation-p0-contract.md)；1440×900、1024×1366、390×844、360×800、frontend 643/643、Go/build 和新旧卷门通过。 |
| Reader：EPUB、漫画/CBZ、音频、连续跨章、TTS | `Reader.vue`、`Content.vue`、本地格式解析类 | `ReaderChapterContent.vue`、`ReaderEpubContent.vue`、`ReaderAudioContent.vue`、`ReaderTTSBar.vue`、`useReaderChapterReady.js`、格式 parser / cache | **EPUB、CBZ、连续跨章、音频和 TTS 固定基准切片均已完成实现、三视口验证和 Docker 发布**：音频恢复上游结构、边界行为与真实 autoplay；TTS 恢复显式 voice、贴底栏、可取消跨章和关闭段落定位。 | [`reader-audio-tts-fixed-baseline-p0-contract.md`](reader-audio-tts-fixed-baseline-p0-contract.md) 及前三份格式合同；本批 frontend 444/444、Go/build、Reader 全矩阵通过，镜像 `5260efd`/`latest` 已发布。当前 volume 脚本受 Codex socket 授权额度阻断，兼容证据继承无后端/持久化差异的 `370d0f7` 已通过门禁。 |
| Pinia 状态、缓存、同步、数据事务 | `plugins/vuex.js`、`plugins/cache.js`、后端 controller/model | `stores/*.js`、`utils/*cache*`、`backend/models`、`services`、`sync` | **书架、认证 scope 与阅读进度 P2 已完成并发布 `9f19d21`**：进度现由规范章节 + 数据库原子 CAS 决定唯一赢家，后台退出不重复请求；远端恢复不再经路由 watcher 回声保存。真实三视口双客户端收敛、冷恢复、格式与历史卷门禁均通过。 | [`reading-progress-p2-contract.md`](reading-progress-p2-contract.md)；后续选择下一未审查数据事务，不重开本合同已关闭的语义。 |
| Go REST、鉴权与错误语义 | Kotlin `*Controller.kt`、`ReturnData.kt` | `backend/api/*.go`、`middleware/*.go`、前端 `api/*.js` | **按动作逐项复审；阅读进度、外部 WebDAV 与远程封面动作均已发布**：`GET/HEAD /api/cover/:capability` 以签发式图片 capability 替代上游公开 `path` SSRF 接口，响应投影与原始持久数据分离。其它模块仍只能以专项 API 合同为证。 | [`reading-progress-p2-contract.md`](reading-progress-p2-contract.md)、[`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md)、[`book-cover-proxy-p2-contract.md`](book-cover-proxy-p2-contract.md)；封面发布证据固定为 `ceb4baa`。 |
| 书源解析、RSS、远程抓取 | `AnalyzeRule*`、`Rss*`、`BookSourceController.kt` | `backend/engine/source_*.go`、`rss_parser.go`、fetcher | **CSS/JSONPath/XPath 主链、变量、错误透明化、元数据与真实浏览器书源流已完成；脚本入口维持显式安全差异**。RSS 三层工作台和有界抓取有独立合同。 | [`online-booksource-parser.md`](online-booksource-parser.md) 与 [`booksource-metadata-normalization-p2-contract.md`](booksource-metadata-normalization-p2-contract.md)；只为尚未抽取的规则/抓取动作建立新合同，不再标记整条主链“尚未验证”。 |
| 测试、构建、Docker、卷升级 | 上游功能契约；OpenReader Docker/data 约束 | `frontend/tests`、`scripts/smoke`、`backend/**/*_test.go`、Dockerfile、release scripts | **当前普通书架可见布局切片已由本机作为 `60984b6` 发布**：frontend 689/689、Go、build、书架四视口、Index 相关浏览器门、新卷 portable v1/v2 assets/跨用户/重启和历史 TXT/EPUB/UMD/CBZ/相对缓存/owner isolation 均通过。 | `60984b6`/`latest` 共同指向 amd64/arm64 OCI index `sha256:05c36dd96c1ba3d3a201b713a731d27bb26fe9c34988626437230d349b3e1ad8`；等待设备验收。 |

## P0 Reader 重新审查（已完成的源码证据）

| 项目 | 上游证据 | 当前证据 | 判定与必须动作 |
|---|---|---|---|
| 进入移动阅读器的工具层 | `Reader.vue` data: `showToolBar: true`。 | `Reader.vue`: `mobileChromeVisible = ref(true)`。 | **已复核一致**；加载章节不可暗中改为隐藏。 |
| 主面板打开后的工具层 | `eventHandler()` 在书架、书源、目录、设置任一 popover 打开时直接返回，不改 `showToolBar`。 | `useReaderPrimaryPanels` 仅切换面板；`useReaderPointer` 在主面板打开时返回。 | **已复核一致**；补全四个面板及全局对话框的点击穿透浏览器断言。 |
| 主 Popover 的移动端根几何 | `Reader.vue` 传入 `popperWidth = windowWidth - 33`；但 `App.vue` 的 `.mini-interface .popper-component { left:0; top:0; width:100vw !important; }` 是最终权威 CSS。该 mini 形态仅在自适应宽度 `<=750px` 或显式强制时出现。 | `ReaderMobileWorkspacePanel.primary` 是无通用 padding 的全宽、内容高度根，内容组件自行持有内边距；当前错误在于宽屏 iPad也自动进入这套形态。 | **移动根结构可保留，场景判定 must-fix**；不得用 iPad 专用 CSS掩盖共享响应 predicate 的偏差。 |
| 主面板层级/点击 | 上游工具栏 `z-index:2001`，popover 在其下；正文点击在面板状态直接返回。 | 当前工具层 `z-index:8`、主面板 `z-index:7`，主面板停传播且 pointer/keyboard 有状态保护。 | **技术栈等价，待浏览器复验**；层级数字可不同，但工具层可见、面板不穿透、同工具关闭/A→B 原子切换必须固定。 |
| 移动顶部工具顺序 | 上游模板源顺序为书架/书源/目录/设置/首页；mini 模式给首页内联 `order:-1`，最终可见顺序为 **首页、书架、书源、目录、设置**。 | `ReaderMobileChrome.vue` 直接按最终可见顺序渲染；桌面顺序独立保留。 | **2026-07-17 已复核一致**：源码与三视口真实 DOM 均通过；见 [`reader-mobile-controls-p0-contract.md`](reader-mobile-controls-p0-contract.md)。 |
| 移动左侧浮动按钮 | 上游 mini 模式依次显示书签、搜索、信息、顶部、底部；顶部/底部分别调用 `toTop(0)` / `toBottom(0)`。 | 当前已补齐五项并复用 `scrollToTop` / `scrollToBottom`；不按格式隐藏、不修改工具层。 | **2026-07-17 已复核一致**：两种移动高度无重叠，滚动和正文几何浏览器合同通过。 |
| 移动底部进度 | 上游 mini 非音频滑条是 `1…totalPages` 当前渲染页，标签 `第 x/y 页`；底部中间另显示单行 `阅读进度: N%` 并打开缓存区。 | 已恢复 1-based 当前渲染页；scroll/scroll2 有真实页数；拖动不跨章；音频隐藏并收缩底栏；中间恢复单行进度。 | **2026-07-17 已复核一致**：[`reader-mobile-progress-p0-contract.md`](reader-mobile-progress-p0-contract.md)；保留更平滑全书百分比算法为允许差异。 |
| 阅读内书源入口 | 上游 `BookSource` 工具没有按本地/远程禁用；点击后由书源流程决定可用结果。 | 当前桌面/移动按钮均无 `disabled`；`useReaderPanels.openSource()` 只拒绝临时阅读，不再按本地/远程禁用。 | **已复核一致**：原“当前证据”已被 `32a1eaa` 后的实现替代；保留本地书点击可打开面板的合同。 |
| 移动正文横向几何 | 上游 mini `.chapter`: `width:100vw; padding:0 16px; box-sizing:border-box; text-align:justify`；slide 内容同样 16px 两侧留白。 | 当前 `.reader-page` 与 `.reader-body` 同样使用 100vw/16px/justify；工具层显隐不参与正文宽度。 | **已复核一致，待像素复验**：390 与 360 下首段左右可见留白误差不得超过 1px。 |
| 移动正文纵向起点 | 上游 `.content-inner`: `margin-top: 30px + safe-area`、`padding-top:15px`。 | 当前 `.reader-body` 使用相同语义。 | **已复核一致，待像素复验**。 |
| 标题元素和排版 | `Content.vue` 渲染章节标题为 `h3`；CSS 固定 `font-size:28px; line-height:1.2; margin:1em 0; text-align:center`。 | 当前普通/卷/错误章节均为 `h3[data-pos="0"]` 且样式一致；书签、搜索、TTS、viewport progress 与已加载章节位置跳转均已同步。 | **已复核一致**：残留 `h1[data-pos]` consumer 已由失败测试关闭；见 [`reader-text-position-selector-p0-contract.md`](reader-text-position-selector-p0-contract.md)。 |
| 段落语义 | 上游 `p` 有 `word-wrap:break-word`、`word-break:break-all`、首行缩进 2em，行高/间距来自配置。 | 当前三项均已显式存在。 | **已复核一致**：保留长无空格文本分页与 16px 留白浏览器回归。 |
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

## 2026-07-23 Reader 设置变更位置事务复审

| 模块 | 上游权威点 | 当前状态 | 裁决 | 后续测试 |
|---|---|---|---|---|
| 直接翻页方式切换 | `ReadSettings.vue#setReadMethod` 先 emit；`Reader.vue#beforeReadMethodChange` 先保存可见段落；`isSlideRead` 重分页后恢复该段落。 | `useReaderMode` 现于 mutation 前从旧 viewport 捕获段落锚点，再重建并恢复；`readerMode.test.mjs` 覆盖 page/scroll/scroll2 ↔ flip。 | **resolved / aligned** | 保留直接模式切换与 `data-pos` 回归。 |
| Kindle、配置方案、自动昼夜 | 整套配置可同时改变阅读方式和排版；slide 分支变化后优先恢复已维护的 `currentParagraph`。 | mode/pageMode/排版变化合并为一个 generation 化位置事务；新事务取消旧恢复，自动昼夜走同一 store 边界。 | **resolved / aligned** | 保留同批配置合并、自动昼夜和旧事务不得回拉合同。 |
| 页面模式与有效滚动宿主 | mini interface/窗口重排后保持当前页；文本横竖模式分别使用既定宿主。 | auto/mobile 或横竖变化前捕获旧 viewport，变化后按新根/内部宿主恢复；390×844、360×800、1024×1366 已验证。 | **resolved / aligned** | 保留手机双尺寸与 iPad 的根/内部 scrollTop、首可见段落合同。 |
| EPUB/音频/普通图片 | raw readMethod 不改变这些格式的有效非 slide 分支。 | 事务以 `readerEffectiveMode()`/有效宿主为门禁；固定格式仅 raw mode 变化不会重建或清空内容。 | **resolved / technical-stack-equivalent** | 保留 fixed-format raw mode no-op 回归。 |

本轮仅完成固定上游合同提取和当前实现映射，未修改应用或测试代码。权威细节见
[`reader-mobile-page-click-p0-contract.md`](reader-mobile-page-click-p0-contract.md) 第十二次复审。

实施后状态：上述四项 Reader 设置位置偏差已进入 **browser-validated**。统一布局事务在旧
viewport 捕获段落、合并同批 mode/pageMode/排版变化、取消陈旧恢复，并按有效模式跳过固定格式
的 raw mode 噪声。390×844、360×800、1024×1366 的直接模式、配置方案、Kindle、自动昼夜和
页面模式合同通过。Docker 候选进一步通过真实后端 EPUB 三视口与普通/历史持久卷备份门禁，
已发布 `a7254e3`/`latest`；远端 OCI 索引为
`sha256:fd4c84bb9da97d4bbab39783d3949d610543195d169756cc2faa13a8e1c2722c`。
