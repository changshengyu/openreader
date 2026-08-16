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
| 书架、BookManage、BookGroup、BookInfo | `Index.vue` 普通 shelf、`BookManage.vue`、`BookGroup.vue`、`BookInfo.vue` | `Home.vue`、`OverlayBookManagement.vue`、`OverlayBookGroups.vue`、`BookInfoDialog.vue`/`BookInfoPanel.vue`、`BookCover.vue`、stores | BookManage、BookGroup、BookInfo 与可见布局第二轮均已发布。**手动刷新第二轮已随 `43635a1` 发布**：两个入口共用一次 check→缓存失效→权威 shelf 重载；后端恢复 16 并发、变量、目录替换、每书事务、陈旧结果拒绝和安全部分失败。 | [`bookshelf-manual-refresh-fixed-baseline-second-audit-p1-contract.md`](bookshelf-manual-refresh-fixed-baseline-second-audit-p1-contract.md) 状态 `aligned / Docker-published / awaiting-device-verification`；Go/full/race/vet、frontend 713/713、build、完整 Reader 浏览器合同及新旧卷均通过。`43635a1` OCI index 为 `sha256:0f75a0434d209af901cde81f86127f8e62fa78d6cb3610d6c10ef2e0863053c0`。 |
| 书源与搜索/探索结果 | `Index.vue#toDetail`、`Index.vue#debugBookSource`、`Index.vue:720-890` 的书源管理器、`bookSourceDebug/*`、`Debugger.kt`、`Reader.vue#getCatalog`、`App.vue#getBookContent`、`Explore.vue`、`BookSource.vue`、`BookController.kt` | `SourceManager.vue`、`OverlaySources.vue`、`SourceDebug.vue`、`Search.vue`、`Discover.vue`、`RemoteBookResultList.vue`、`ExploreWorkspacePopover.vue`、`backend/api/source_debug.go`、`services/sourcedebug`、`remote_reader.go`、`services/remotereader`、Go source APIs | 书源 owner/API/解析主链、搜索/探索、临时 Reader 和独立书源调试器继续保持已发布；**可见书源管理器第二轮已按固定上游重建并作为 `c74be70` 发布**：恢复单 table、动态正常/失败标题、固定列/分组/footer、完整 reader-dev JSON 编辑器和独立本地/远程导入；删除移动卡片、Drawer 与额外 batch/health 命令。多用户事务、安全抓取、失败缓存、同步和旧 URL 保留。 | [`source-manager-fixed-baseline-second-audit-p1-contract.md`](source-manager-fixed-baseline-second-audit-p1-contract.md) 已通过 frontend 730/730、Go full/race/vet、build/diff、四视口浏览器和 fresh/historical volume；`usedBookNames` 当前用户投影、未知字段无迁移往返和 16 MiB 导入上限均有专项测试。状态 `aligned / Docker-published / awaiting-device-verification`，OCI index `sha256:3dd824fb006890504110124c48a84d0ce10aebab719eec7a3c9104f071e02eba`。 |
| 本地导入、书仓、WebDAV | `Index.vue#importBookPreview/importMultiBooks`、`BookController.kt`、`LocalBook.kt`、`TextFile.kt`、`EpubFile.kt`、`UmdFile.kt`、`CbzFile.kt`、`LocalStore.vue`、`WebDAV.vue` | `OverlayBookImport.vue`、`OverlayStorageImport.vue`、`useStorageImportWorkflow.js`、`backend/api/direct_local_import_boundary.go`、`imports.go`、`localstore*`、`webdav*`、`services/localbook/*`、`engine/*_parser.go` | P1-E1…P1-E5、P2 parser 预算、TXT 默认规则和 mounted LocalStore/WebDAV 边界保持已发布。**直接浏览器导入第二轮已实现并发布 `429444a`**：恢复 1..64 多选、单本直达确认、多本共享批量/逐一状态机、同名稳定 identity、token-only 确认；三个 direct multipart 路由补齐认证后 actual-read 包络、严格 shape/metadata/category 和 handler-owned 临时文件清理。 | [`direct-local-book-import-multipart-workflow-fixed-baseline-second-audit-p1-contract.md`](direct-local-book-import-multipart-workflow-fixed-baseline-second-audit-p1-contract.md) 状态 `aligned / Docker-published / awaiting-device-verification`；frontend 737/737、Go full/race/vet、build、真实 HTTP、三视口 direct/storage flow 与 fresh/historical/portable 卷门均通过，OCI index `sha256:41f430a5fbf944b9a1dcf25aec6c9f6e92a11a3ff75e395d1a73120da5a6f4d5`。 |
| 外部 WebDAV 协议 | `WebdavController.kt` 的 `/reader3/webdav*`、Basic、OPTIONS/PROPFIND/MKCOL/PUT/GET/DELETE/MOVE/COPY/LOCK/UNLOCK | `backend/api/server.go`、`backend/api/webdav.go`、`middleware/webdav_auth.go`、`services/webdavfs` | **2026-07-22 已实施、全量验证并随 `544e1fb` 发布**：双前缀、Bearer/Basic、DAV 发现/列表、完整文件方法、调用者私有根、原子事务和逐组件 symlink 防护已通过 API/service/CORS、全量 Go/frontend/build、真实 Basic curl、三视口工作台、候选容器协议与新旧卷/便携备份门禁；现有 `/webdav` GET 适配器保留。 | [`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md)；后续 WebDAV 修改不得削弱 caller scope、事务和 symlink 合同。 |
| 用户、备份、RSS、书签 | `UserManage.vue`、`AddUser.vue`、`BaseController.kt#saveUserSession`、`WebDAV.vue`、`Rss*`、`Bookmark*` 及对应 Kotlin 控制器 | `OverlayUserManagement.vue`、`backend/api/auth.go`/`admin.go`、`WebDAVBrowser.vue`、`RSSManager.vue`、`components/rss/*`、`services/rss`、`OverlayBookmarks.vue` | Bookmark 可见流程、会话/owner 隔离和 RSS 可见第二轮保持已发布；**RSS 写入/缓存并发边界已按测试先行关闭并发布 `0986d8e`**：source/article 使用 actual-read 单 JSON，source mutation 按用户串行事务化，existing row 改为显式列更新，refresh/content 在远程工作后复验存活并遵循正文优先级。Bookmark 写入边界继续由 `a9a55db` 签收。 | [`rss-write-boundary-fixed-baseline-second-audit-p2-contract.md`](rss-write-boundary-fixed-baseline-second-audit-p2-contract.md) 已通过 focused/full/race/vet、frontend 740/740、build、RSS 四视口、宿主 HTTP/SQLite trigger、GHCR 回拉容器纯 API 及新旧卷；状态 `aligned / Docker-published / awaiting-device-verification`，OCI index `sha256:9884d0e9b41c1a1a109f3034159ae2968fe9d889da063deaca2514e1ac371e25`。 |
| 替换规则 | `ReplaceRule.vue`、`ReplaceRuleForm.vue`、`Reader.vue#filterContent/showTextFilterPrompt`、`ReplaceRuleController.kt`、`ReplaceRule.kt` | `OverlayReplaceRules.vue`、`useOverlayReplaceRules.js`、`services/replacerules`、`replace_rules.go`、`books.go#applyUserReplaceRules`、backup/restore | **P2 固定基准重建已完成并发布 `a7abcdd`**：manager/editor 可见结构、共享几何、Reader 直达 editor、精确导入/保存/scope、`id ASC` pipeline、JavaScript replacement string、name restore identity 和 durable-only broadcast 均已测试先行重建；旧新增/刷新/逐行删除/测试器/mobile cards 已删除。 | [`replace-rule-fixed-baseline-p2-contract.md`](replace-rule-fixed-baseline-p2-contract.md)：frontend 649/649、Go/build、1440×900/1024×1366/390×844/360×800 manager 与 Reader 桌面/手机/iPad、新旧卷均通过；`a7abcdd`/`latest` index 为 `sha256:93840cf72e9a0a783333ac5ab485551d892e42b9bf2e8eb2e2a1039e56b5dd53`。JWT/SQLite、legacy 空 scope、隐藏兼容 API，以及 RE2 pattern/32 捕获组/20,000 匹配/有界输出为明确允许差异。 |
| Reader：工具层、面板、正文、翻页 | `Reader.vue`、`Content.vue`、`ReadSettings.vue`、`PopCatalog.vue`、`BookShelf.vue`、`BookSource.vue` | `views/Reader.vue`、`components/reader/*`、`composables/useReader*`、`stores/reader.js` | 工具层状态机、翻页、格式运行时、设置和夜间面此前均已专项发布；Reader 移动端阅读内书架横向仍为 100vw、左右 20px、390/360 列表 350/320px。多条目 Grid 行轨坍缩已由 `a445121` 修复。**内联章节缓存第二轮 must-fix 已实施并随 `4da98fa` 发布**：恢复完整 cache-first 区间、已入架本地书、并发 2、取消即时恢复、精确反馈、扁平表面和陈旧任务隔离；临时 Reader 继续 `no-store`。 | 书架见 [`reader-mobile-shelf-width-p0-contract.md`](reader-mobile-shelf-width-p0-contract.md)。章节缓存见 [`reader-inline-chapter-cache-fixed-baseline-second-audit-p0-contract.md`](reader-inline-chapter-cache-fixed-baseline-second-audit-p0-contract.md)，frontend 740/740、build、Go 全量、四视口专项、完整 Reader/iPad 与新旧卷通过；状态 `aligned / Docker-published / awaiting-device-verification`，OCI index `sha256:771df515341f46f35a07b7f62de913490cdefe4489f3016d7d82f4436aa8f75d`。 |
| Reader：设置第二轮复审 | `ReadSettings.vue`、`config.js`、`vuex.js#setConfig/setNightTheme`、`Reader.vue` TTS/read bar | `ReaderSettingsPanel.vue`、`ReaderSettingStepper.vue`、`stores/reader.js`、`ReaderTTSBar.vue`、appearance/mode composables | **2026-08-02 固定基准重建已完成并发布 `40f124f`**：方案 allowlist、无损重置、normal/kindle 双快照、14 内置背景、独立主题纹理、五字体单操作、精确顺序/divider/操作区和 TTS 去重均已落地；亮度、可编辑 stepper、字体预览/字号预设、纯黑白夜间和连续滚动/离散点击为明确允许差异。 | [`reader-settings-fixed-baseline-second-audit-p0-contract.md`](reader-settings-fixed-baseline-second-audit-p0-contract.md)：frontend 680/680、Go/build/diff，1440×900、390×844、360×800、1024×1366、1366×1024、强制手机 iPad 与新旧卷全部通过；`40f124f`/`latest` OCI index 为 `sha256:d9395b19f45bfe9412facbcdcec63e776c881c13437c6049e70140f3f87e6b45`，状态 `aligned / Docker-published / awaiting-device-verification`。 |
| Reader：书内正文搜索 | `SearchBookContent.vue`、`vuex.js#dialog*`、`Reader.vue#showSearchContent`、`BookController.searchBookContent/searchChapter`、`SearchResult.kt` | `OverlayBookContentSearch.vue`、`useBookContentSearch.js`、`useReaderContentSearchIntent.js`、`useReaderSearchNavigation.js`、`readerBookSearch.js`、`backend/services/contentsearch`、`backend/api/books.go` | **2026-08-02 第二轮固定基准重建已完成并发布 `1801037`**。恢复动态 width/top/table、75% 非自动聚焦标题输入、100/250 列、非阻塞表格、上游 footer 与正常模式 gate；同 ID 换 URL 会 reset，前端 intent/Reader/现代及 legacy Go 全链保留原始空白查询。原始正文、精确/大小写/重叠、游标、取消、UTF-16、多用户、同/跨章跳转继续成立。 | [`book-content-search-fixed-baseline-second-audit-p2-contract.md`](book-content-search-fixed-baseline-second-audit-p2-contract.md)：frontend 659/659、Go/build，普通 Reader 桌面/手机/iPad、真实 EPUB 三视口及新旧卷通过；状态 `aligned / Docker-published`，OCI index `sha256:5d2fdb171e734d5debece77f91ae31495fc1ba7ee9eec28c88aa2b3f41eeeee5`。 |
| Reader：登录失效与账号切换 | `plugins/axios.js` 的 `NEED_LOGIN`、根 `App.vue#login`、`Reader.vue#loginAuth` | `api/client.js`、`App.vue`、`AuthDialog.vue`、`stores/user.js`、Reader lifecycle/progress、`stores/overlay.js` | **P0 已完成并发布 `59e11a9`**：401 按真实拦截顺序先挂起旧 Reader 再清凭证，未认证根场景不渲染私有 DOM；overlay reset、同账号 generation 重挂载、异账号返回书架、安全 returnTo、旧进度写入抑制均已验证。 | [`reader-reauthentication-isolation-p0-contract.md`](reader-reauthentication-isolation-p0-contract.md)；1440×900、1024×1366、390×844、360×800、frontend 643/643、Go/build 和新旧卷门通过。 |
| Reader：EPUB、漫画/CBZ、音频、连续跨章、TTS | `Reader.vue`、`Content.vue`、本地格式解析类 | `ReaderChapterContent.vue`、`ReaderEpubContent.vue`、`ReaderAudioContent.vue`、`ReaderTTSBar.vue`、`useReaderChapterReady.js`、格式 parser / cache | **EPUB、CBZ、连续跨章、音频和 TTS 固定基准切片均已完成实现、三视口验证和 Docker 发布**：音频恢复上游结构、边界行为与真实 autoplay；TTS 恢复显式 voice、贴底栏、可取消跨章和关闭段落定位。 | [`reader-audio-tts-fixed-baseline-p0-contract.md`](reader-audio-tts-fixed-baseline-p0-contract.md) 及前三份格式合同；本批 frontend 444/444、Go/build、Reader 全矩阵通过，镜像 `5260efd`/`latest` 已发布。当前 volume 脚本受 Codex socket 授权额度阻断，兼容证据继承无后端/持久化差异的 `370d0f7` 已通过门禁。 |
| Pinia 状态、缓存、同步、数据事务 | `plugins/vuex.js`、`plugins/cache.js`、后端 controller/model | `stores/*.js`、`utils/*cache*`、`backend/models`、`services`、`sync` | 书架、认证 scope 与阅读进度 P2 已完成并发布；**WebSocket 协议第二轮已测试先行实施并发布 `2ea6e8c`**：任意客户端 event relay、无条件 Origin、deleted-user 连接和全局 `users_update` 已关闭；服务端 event type/payload、同用户收敛、重连 REST 权威和数据格式保持。 | [`reading-progress-p2-contract.md`](reading-progress-p2-contract.md)、[`websocket-sync-p2-contract.md`](websocket-sync-p2-contract.md)；WebSocket 状态 `implemented / regression-validated / Docker-published`，Go/full race、frontend 706/706、build、三视口双客户端及新旧卷通过。 |
| Go REST、鉴权与错误语义 | Kotlin `*Controller.kt`、`ReturnData.kt` | `backend/api/*.go`、`middleware/*.go`、前端 `api/*.js` | **按动作逐项复审；已关闭模块不从旧日志重开**。WebDAV、direct local、remote-work 与 BookSource multipart 保持已发布；reading progress 第二轮已按 `f924604` 合同、`a10facb` 红测、`8d3790d` 实现和 `1563bc3` runtime 顺序关闭 16 KiB 单 UTF-8 JSON、显式 chapter index、RFC3339Nano CAS 字段和 mode/clientId 短界，既有 CAS/mirror/Hub 业务合同未重开。 | [`reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md`](reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md) 状态 `aligned / regression-validated / local-Docker-candidate / release-gate-pending`。剩余其它 batch/control JSON 仍须逐项取证。 |
| 书源解析、RSS、远程抓取 | `AnalyzeRule*`、`Rss*`、`BookSourceController.kt` | `backend/engine/source_*.go`、`rss_parser.go`、fetcher、`services/rss` | **CSS/JSONPath/XPath 书源主链、RSS 可见请求页语义、P2-N1/P2-N2 抓取边界和 RSS 持久提交边界均已发布**。refresh 只写 parser/remote 列并按 detail rule 保留权威正文；content cache 只写 content；state 只写 read/favourite；三者不再用全行 `Save` 覆盖。 | 抓取预算/SSRF 合同不重开；[`rss-write-boundary-fixed-baseline-second-audit-p2-contract.md`](rss-write-boundary-fixed-baseline-second-audit-p2-contract.md) 已用 trigger/API 证明列所有权、删除不复活、无孤儿 article 和远程工作后的 source/article 存活复验。 |
| 测试、构建、Docker、卷升级 | 上游功能契约；OpenReader Docker/data 约束 | `frontend/tests`、`scripts/smoke`、`backend/**/*_test.go`、Dockerfile、release scripts | `1563bc3` 通过 frontend 741/741、Go full/race/vet、build 和 reading progress 三视口 runtime；本机 arm64 candidate 已构建且 revision label 为完整提交。 | mounted-volume 门因 Codex Docker socket 使用额度在授权阶段被拒绝，故没有执行本轮 fresh/historical/portable 或正式发布。当前已发布镜像仍为 `3f3c9c8`/`latest`，OCI index `sha256:62ee55ffab7859aef4334f8fb8dd31520953521da494edd5f37cc56741731070`；用户生产环境运行提交未知。 |

## 当前整体进度快照（2026-08-16，`1563bc3` 阅读进度请求边界 regression pass）

按全量计划的模块/合同口径而不是代码行数估算，整体约 **98%**。该数字表示固定基准合同和测试先行
实现覆盖度；用户配置、BookGroup/Category、Book、BookSource、Bookmark 与 RSS 写入/导入边界均完成
实现、全量、运行时、新旧卷和正式 Docker 发布；剩余 2% 仍包含未逐动作签约的长尾，不是简单收尾。

- **P0 Reader 主链已覆盖**：工具层/面板状态机、正文排版、移动点击与连续滚动、设置、书签、正文
  搜索、登录恢复、普通文本、EPUB、CBZ/漫画、音频、连续跨章、TTS、夜间对比度均有专项合同和
  Docker 证据；Reader 换源三 mode、派生候选缓存、精确匹配、换源事务和位置保持，以及内联章节
  cache-first 完整区间、本地书和取消隔离均已通过回归并发布，等待真实设备签收。
- **P1 Index 工作台主链已覆盖**：侧边栏、普通书架、搜索/探索、唯一 BookInfo、BookManage、
  BookGroup、导入、本地书仓、WebDAV 与兼容旧路由均已重建/收敛；书架 freshness、进度刷新和
  lastCheckTime 专项已关闭；临时 Reader 的预算、TTL/LRU、变量、取消和脱敏，以及可见书源管理器
  第二轮均已发布；直接浏览器导入的多选、共享确认状态机和 multipart 边界已完成实现、运行时、卷门
  与 Docker 发布，等待真实设备签收。
- **P2 已覆盖的高风险项**：账号/overlay/cache 隔离、原子阅读进度、删除收敛、替换规则、书签、
  用户管理、RSS 第二轮、书源 owner/API/主解析链、外部 WebDAV、portable backup、封面/章节图片
  capability，共享抓取器 P2-N1/N2 请求与私网/DNS/代理边界，本地格式 parser 的输入、解码文本、
  章节与历史读取预算、独立书源调试器完整链、公共/管理员/用户配置/BookGroup/Book 写入 body、字段、
  owner 与归档引用边界，BookSource JSON/cardinality/原子 `sourceLimit`，Bookmark wire/cardinality/
  note-only/并发删除边界，RSS actual-read/single-JSON/同 URL 身份/列所有权/远程存活边界，用户资产
  33 MiB multipart/单 part/临时文件/16 KiB 删除 JSON 边界，LocalStore 聚合 multipart/JSON/cardinality/
  symlink/special-file/opened-file 边界，remote-work 七路 actual-read/single-object、搜索八窗口/60 并发、
  health 300-source/15-worker、取消与整本缓存不退化边界，BookSource local import 浏览器 16 MiB 预读、
  17/16 MiB 双层 multipart、严格 part/error/cleanup 边界，以及
  reading progress 16 KiB/single UTF-8 JSON/显式 identity/CAS 控制字段与 client ID 自愈，以及
  server-only WebSocket/recipient scope 和备份事务 worker。
- **尚未完成的主线**：其余尚未逐动作签约的 Go REST/错误/事务语义；仍待第二轮固定基准复审的长尾组件；
  以及后续真实设备反馈暴露出的上游可见偏差。reading progress 请求边界已完成实现与运行时回归，
  但本轮 mounted-volume/正式 Docker 发布因 socket 授权额度尚未完成；其它 batch/control JSON 尚未逐项
  取证。`c74be70` 已发布但尚待服务器部署；移动书架在
  390×844 线上真实账号复测中保持上游 390/350px 几何和内容高度行轨，但设备“明显窄”的反馈仍待
  完整截图区分首页书架、Reader 内书架或设备可见层；书源管理、临时阅读和调试器继续等待真实设备
  签收。上述项目继续按“合同→失败测试→实现→浏览器/
  新旧卷→本地 Docker”推进。

2026-08-16 的 BookSource local import multipart 按 `d7bc00a` inventory、`ddbac4c` 旧实现红测、
`8c66dc9` 实现和 `3f3c9c8` runtime contract 顺序关闭：原始 chooser 在 `text()` 前拒绝已知 16 MiB
超限，API 在认证/权限后执行 17 MiB actual-read、唯一 file/零 scalar、16 MiB file read、稳定 envelope/
file 413 和 handler-owned form cleanup；reader-dev JSON、5,000 项、identity/COW/quota、事务与事件保持。
Go full/race/vet、frontend 738/738、build、三视口真实 Go/Chromium、fresh/portable/cross-user/restart、
source ownership 和 historical trace 重跑卷门通过。本机发布 `3f3c9c8`/`latest`，OCI index 为
`sha256:62ee55ffab7859aef4334f8fb8dd31520953521da494edd5f37cc56741731070`，状态为
**aligned / Docker-published / awaiting-device-verification**，整体比例仍保持 98%。

2026-08-16 的 remote-work 请求边界按 `5aadf9b` inventory、`94d0a4e` 红测、`346a49d` 实现和
`6157466` browser contract 顺序关闭：七路 JSON 入口现有 actual-read/single-object 与短字段边界，
多源搜索最多 60 并发和八个稳定 ordinal 窗口，batch health 最多 300 source/15 worker；旧数组响应、
诊断 envelope、failure cache owner、Context 取消和 BookManage 整本缓存均保持。Go full/race/vet、
frontend 737/737、build、三视口 remote-work、四视口 source-debug 和 fresh/portable/cross-user/restart
卷门通过；历史卷首次普通运行在 fixture 后瞬时 404，同镜像 trace 重跑 TXT/EPUB/UMD/CBZ、
relative-cache、owner isolation 全链通过。本机发布 `6157466`/`latest`，OCI index 为
`sha256:1e890a60a1b75879dd99074b1da13b17f91bbd4173e945b92cb8cec0fe8001b6`，状态为
**aligned / Docker-published / awaiting-device-verification**。

2026-08-12 在 `12036de` 后重新对 `backend/api/server.go` 做动作差集，下一项 must-fix 收敛为
`POST/DELETE /api/uploads` 的 wire boundary。现有 8/32 MiB 是解析后的单文件 admission，不限制完整
multipart；Gin 32 MiB 只是内存阈值，chunked/额外 part 仍可继续读盘，DELETE 也仍接受无界首个 JSON。
固定 33 MiB multipart 包络、单 file/type、临时文件清理和 16 KiB 单 JSON 删除合同见
[`user-asset-write-boundary-fixed-baseline-second-audit-p2-contract.md`](user-asset-write-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同、旧实现红测、实现和真实 HTTP 探针已依次落地；Go full/race/vet、frontend 740/740、build、宿主/
候选/GHCR 回拉 runtime 与 fresh/historical 卷门通过。本机发布 `be83a0f`/`latest`，远端 OCI index 为
`sha256:e1f31f3dd728bc27fbc89bbc8c21f81e8c5511c5e99196891feb21cd47138b73`。当前状态为
**aligned / Docker-published / awaiting-device-verification**。

2026-08-12 的 LocalStore 文件系统/请求边界已完成合同、旧实现红测、实现和真实 HTTP 探针：多文件
上传现在具有聚合 actual-read 包络和 part/metadata/临时文件所有权，legacy JSON 与目录展开有界，
所有动作按 caller root 拒绝 symlink/special file 并对 download/import 复验 opened regular file。
固定上游多选/逐文件可见语义、OpenReader 私有根与稳定 API、允许安全差异和完整证据见
[`local-store-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](local-store-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
实现 `bba99e1` 与探针/candidate `930be4d` 已推送；Go full/race/vet、frontend 740/740、build、宿主与
candidate HTTP 均通过。后续 `65a9870` 重新通过 LocalStore candidate 探针、fresh/historical/portable
三卷、跨用户和重启，并由本机发布；当前状态为 **aligned / Docker-published /
awaiting-device-verification**，整体比例暂不变。

2026-08-16 从 `server.go`、固定上游 `WebDAV.vue`/controllers 和当前 handler 重新做动作差集后，
WebDAV mounted `import-preview`、`import`、`restore-webdav` 已按合同→红测→实现关闭：1 MiB/16 KiB
actual-read single JSON、200 项 raw/expanded 完整计划、caller-rooted `Service.Open`、token-only
source independence 与 restore caller-private snapshot 均已落地。精确合同见
[`webdav-import-restore-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](webdav-import-restore-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同 `cf46e22`、旧实现红测 `1bb904a`、实现/runtime `616a076` 已推送；Go/full/race/vet、frontend
740/740、build、宿主/candidate 真实 HTTP、导入/重试/恢复三视口和 fresh/historical/portable 卷门通过。
本机发布 `65a9870`/`latest`，OCI index 为
`sha256:255c81b43dbb7f49c707d6c609b920aa183b730401ad1c1ca32157eb0a945c71`；状态为
**aligned / Docker-published / awaiting-device-verification**，整体比例仍保持 96%。

2026-08-11 的长尾切片 Reader 内联章节缓存已实施并回归：完整后续区间、已入架本地书、取消
即时恢复、精确完成反馈和跨书/跨身份迟到任务隔离均已关闭；临时 Reader 继续保持 `no-store`。
frontend 740/740、build、Go 全量、四视口专项和完整 Reader/iPad 浏览器门通过；实现提交 `4da98fa`
又通过顺序 fresh/historical 卷门并由本机发布为同名标签与 `latest`。

2026-08-11 长尾动作复审已将“尚未逐动作签约”进一步收敛：六个精确路径已有正式合同；TXT 默认
目录规则缺项、禁用项过滤和双标题语义已完成实现及三视口真实导入。Reader 换源原先缺少每书候选
缓存并在打开时联网宽搜的 must-fix 已关闭：available 现在无网络，refresh/search、精确身份、缓存
生命周期、前端状态和四视口位置保持均已验证，并已完成卷门与 Docker 发布。移动书架设备反馈仍
需完整截图区分首页书架、Reader 内书架或设备可见层。

2026-08-12 的长尾切片关闭公开认证请求边界：login/register 现在使用 16 KiB 实际读取上限与单 JSON
文档，声明/chunked 超限统一 `413`；注册密码超过 bcrypt 72 bytes 为可操作 `400`，登录保持通用
`401` 并在查库前拒绝截断碰撞。focused/full/race/vet、frontend 740/740、build、真实 HTTP smoke、
 fresh/historical 卷门和本机双架构发布均完成；实现提交与 Docker 标签为 `f5c15d7`。

2026-08-12 的后续长尾切片关闭管理员用户写入边界与共享密码长度：五个管理员 JSON mutation 现在
管理员鉴权优先，统一 16 KiB actual-read 单 JSON；批量原始 ID 最多 2,000；public/admin 新密码统一按
8 UTF-16 code units 与 bcrypt 72 UTF-8 bytes 校验，负限额、role 与 reset target 在 bcrypt 前裁决。
旧实现红测、focused/full/race/vet、frontend 740/740、build、真实 declared/chunked HTTP、零副作用、
fresh/historical 卷门和本机双架构发布均完成；实现提交与 Docker 标签为 `6c1c6db`。

2026-08-12 的下一批长尾切片关闭用户配置 PUT、BookGroup/Category 六路和通用 Book create/update 写入
边界：分别使用 8 MiB、16 KiB 和 1 MiB actual-read single-JSON，保留各自业务状态机并在查询/写入/event
前拒绝超限或多 JSON。Book create 不再接受身份/来源/存储/解析/进度字段；最终分类和自定义封面按当前
用户验证，字段按 UTF-8 bytes 限制；删除本地书档只在最后一个同用户真实目标引用消失后执行，并对
查询失败、越界链接和删除候选链接 fail closed。旧实现红测、focused/full/race/vet、frontend 740/740、
build、真实 HTTP、新旧卷和本机 amd64/arm64 发布均完成；最终实现/Docker 提交为 `231aa9e`，OCI index
为 `sha256:e4affbeaf133220409c82dc1316d7cc2e2e7267fe8623d817205b1fa0340a5c6`。

2026-08-12 的 BookSource 长尾切片按 `45a73ca` 合同、`4f502df` 红测、`d9ddc0f` 实现顺序关闭写入、
导入和配额边界：create/update 为 16 MiB actual-read 单 object，batch/remote control 为 16 KiB，
local/remote JSON 原始数组上限 5,000；同用户窄锁、SQLite user-row writer lock 和事务内 projected count
使 `sourceLimit` 在 create/import 并发时不超额，同时保留 0=无限、满额 identity update/COW、历史超额
和 restore 保真。focused/full/race/vet、frontend 740/740、build、三轮真实 HTTP、新旧卷、source ownership
与 portable 门均通过；本机发布 `d9ddc0f`/`latest`，OCI index 为
`sha256:548bf0984e7fa5039411bd75f9ae8ac8496052010255bfe746bf36fa9336dc8f`。

2026-08-12 的 Bookmark 长尾切片按 `7fa0e07` 合同、`030696b` 红测、`e5a7ea9` 实现顺序关闭四路
actual-read/single-document、2,000 原始项、显式 note-only payload/SQL、fresh response 和并发删除不复活。
focused/full/race/vet、frontend 740/740、build、Reader 多视口、宿主/候选/回拉 HTTP、SQLite trigger 与
fresh/historical 卷门均通过；本机发布 `a9a55db`/`latest`，OCI index 为
`sha256:944a85881170bc900c1fda0acb885bedc1dc4b17ed4e635305988163e1b635e5`。

2026-08-12 的 RSS 写入/缓存并发切片按 `051bd12` 合同、`1d0e5a0` 红测、`5236389` 实现顺序关闭
source/article actual-read single-JSON、5,000 项、同用户 URL 身份、显式列所有权、正文优先级和网络
工作后的 source/article 存活复验。focused/full/race/vet、frontend 740/740、build、RSS 四视口、宿主
HTTP + SQLite trigger、候选/回拉容器纯 API 与 fresh/historical 卷门均通过；本机发布
`0986d8e`/`latest`，OCI index 为
`sha256:9884d0e9b41c1a1a109f3034159ae2968fe9d889da063deaca2514e1ac371e25`。

## P0 Reader 重新审查（已完成的源码证据）

| 项目 | 上游证据 | 当前证据 | 判定与必须动作 |
|---|---|---|---|
| 进入移动阅读器的工具层 | `Reader.vue` data: `showToolBar: true`。 | `Reader.vue`: `mobileChromeVisible = ref(true)`。 | **已复核一致**；加载章节不可暗中改为隐藏。 |
| 主面板打开后的工具层 | `eventHandler()` 在书架、书源、目录、设置任一 popover 打开时直接返回，不改 `showToolBar`。 | `useReaderPrimaryPanels` 仅切换面板；`useReaderPointer` 在主面板打开时返回。 | **已复核一致**；补全四个面板及全局对话框的点击穿透浏览器断言。 |
| 主 Popover 的移动端根几何 / iPad 场景 | `Reader.vue` 传入 `popperWidth = windowWidth - 33`；但 `App.vue` 的 `.mini-interface .popper-component { left:0; top:0; width:100vw !important; }` 是最终权威 CSS。该 mini 形态仅在自适应宽度 `<=750px` 或显式强制时出现。 | `responsive.js` 已使用同一 `<=750px` 宽度 predicate；自适应 1024/1366 为桌面场景，显式 `mobile` 才在宽屏进入完整手机结构。桌面共享面板另保留同工具、外点和 44×44 可见关闭路径。 | **2026-08-09 第二轮已复审一致并完成当前构建浏览器复验**：专项 7/7、frontend 713/713、Go/build 及桌面/双手机/双 iPad/强制手机 iPad 全部通过；旧“宽屏 iPad 自动进入手机面板”结论已过期。见 [`reader-ipad-panel-fixed-baseline-second-audit-p0-contract.md`](reader-ipad-panel-fixed-baseline-second-audit-p0-contract.md)。`30dbe53..HEAD` 无应用代码差异，沿用最新镜像而不重复构建。 |
| 主面板层级/点击 | 上游工具栏 `z-index:2001`，popover 在其下；正文点击在面板状态直接返回。 | 桌面为正文点击区 z2、外点层 z3、面板 z4、工具栏 z5；手机为正文点击区 z2、外点层 z9、面板 z10、工具条 z11。两种场景均在主面板状态保护 pointer/keyboard。 | **技术栈等价并已复验**：四面板的可见关闭、外点关闭、同工具关闭和 A→B 切换均通过；外点不改变滚动、页 transform 或路由。层级数字可不同，但该相对顺序不得回退。 |
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
