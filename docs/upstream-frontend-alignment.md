# OpenReader 与 hectorqin/reader 前端对齐审阅

> 审阅日期：2026-05-18  
> 上游参照仓库：`hectorqin/reader`，本地快照：`/private/tmp/hectorqin-reader-current`（commit `827dd82`）
> 当前项目：`/Users/yuchangsheng/Documents/OpenReader-dev`

这份文档不是新的设计稿，而是对重构遗漏项的核对清单：逐项确认上游 Web 前端有哪些页面、弹窗、按钮和业务入口，当前 OpenReader 已经实现了什么，还缺什么，以及后续应该按什么顺序补齐。

## 一、上游前端结构

上游 Web 前端的路由很少，主要能力都集中在首页和阅读页，通过弹窗、Popover 和组件挂载出来。

| 上游文件 | 职责 | 当前应对齐位置 |
| --- | --- | --- |
| `web/src/views/Index.vue` | 首页、书架、搜索、书源设置、书架设置、用户空间、WebDAV、本地缓存 | `frontend/src/views/Home.vue`、`Search.vue`、`Sources.vue`、`Settings.vue` |
| `web/src/views/Reader.vue` | 阅读主界面、左右工具栏、目录、书源、书签、设置、自动阅读、听书 | `frontend/src/views/Reader.vue` |
| `web/src/components/BookShelf.vue` | 阅读页内书架弹窗 | `Reader.vue` 内的书架入口，后续应抽成组件 |
| `web/src/components/BookSource.vue` | 阅读页换源弹窗，刷新、分组、加载更多 | `Reader.vue` / `BookDetail.vue` 换源能力 |
| `web/src/components/PopCatalog.vue` | 阅读页目录弹窗，跳章、更新阅读位置 | `Reader.vue` 目录抽屉 |
| `web/src/components/ReadSettings.vue` | 阅读主题、字体、行高、宽度、翻页、自动阅读、点击区、过滤规则 | `Reader.vue` 设置面板、`stores/reader.js` |
| `web/src/components/SearchBookContent.vue` | 当前书内搜索 | `Reader.vue` 右侧搜索按钮，当前未完整实现 |
| `web/src/components/BookInfo.vue` | 书籍信息弹窗、封面、简介、追更、分组、加入书架 | `BookDetail.vue`，阅读页信息按钮 |
| `web/src/components/Bookmark.vue` / `BookmarkForm.vue` | 书签列表、导入、跳转、编辑、批量删除、添加/编辑书签 | `Reader.vue` / `BookDetail.vue` 书签能力 |
| `web/src/components/BookManage.vue` | 书籍批量管理、编辑、分组、导出、缓存删除 | `Home.vue` 书架管理，当前缺大部分批量能力 |
| `web/src/components/BookGroup.vue` | 分组管理、排序、批量设置分组 | `Home.vue` 分类管理，当前只有新增/筛选 |
| `web/src/components/LocalStore.vue` | 本地书仓浏览、目录进入、删除、批量加入书架、上传 | `LocalStore.vue`，当前只做了平铺列表和导入 |
| `web/src/components/WebDAV.vue` | WebDAV 文件浏览、还原、下载、加入书架、删除、上传 | `Settings.vue` / 后续 `WebDAVPanel.vue` |
| `web/src/components/ReplaceRule.vue` / `ReplaceRuleForm.vue` | 替换规则管理、导入、启停、编辑、批量删除 | 当前缺前端入口，后端模型只内嵌在书源规则里 |
| `web/src/components/Explore.vue` | 书海/探索书源，按分组展示发现结果 | 当前缺发现页/探索入口 |
| `web/src/components/RssSourceList.vue` / `RssArticleList.vue` | RSS 源和文章阅读 | 当前后端和前端都缺 |
| `web/src/components/UserManage.vue` / `AddUser.vue` | 用户空间管理、重置密码、默认书源、删除用户书源 | `Settings.vue` / `api/admin.js`，当前只接了一部分后端 |

## 二、阅读页对齐情况

### 已对齐

- 左侧固定工具栏已恢复为上游布局：`首页`、`书架`、`书源`、`目录`、`设置`、`顶部`、`底部`。
- 右侧浮动圆形工具已按上游方向恢复：书签、搜索、信息、笔记、下载/底部、刷新、视图、听书、夜间。
- 右下角进度和上一章/下一章区域已按上游比例恢复。
- 页面宽度已按上游默认 `readWidth=800`、正文宽度 `670` 调整。
- 默认字号、行高已按上游 `fontSize=18`、`lineHeight=1.8` 调整，并增加旧配置归一化。

### 仍未完整对齐

| 上游能力 | 当前状态 | 后续动作 |
| --- | --- | --- |
| 阅读页书架 Popover | 目前按钮跳转书架页 | 做阅读页内书架浮层，能直接切书 |
| 阅读页书源 Popover | 目前跳转书籍详情/换源 | 做换源浮层，支持可用来源、刷新、分组、加载更多 |
| 目录 Popover | 当前已有目录抽屉，但样式和行为不是上游弹层 | 调整为上游窄浮层/移动端抽屉双形态 |
| 阅读设置 | 当前只覆盖字号、行高、主题、模式等基础项 | 补字体、粗细、段距、宽度、点击区、自动阅读、方案 |
| 当前书内搜索 | 右侧按钮只是打开目录搜索 | 新增正文搜索弹窗，支持结果跳转 |
| 书籍信息 | 右侧信息按钮跳到详情页 | 做阅读页信息弹窗，展示封面、简介、来源、追更、分组 |
| 书签管理 | 目前可添加、列表展示有限 | 补书签弹窗、编辑、删除、跳转、导入 |
| 笔记 | 按钮存在但无完整交互 | 与书签表的 `note` 字段合并设计 |
| 自动阅读 | 图标存在但无上游同等控制 | 补自动滚动/段落滚动设置和启停 |
| 听书 | 有基础 TTS composable | 补段落定位、续读、移动端控制条 |
| 阅读进度保存 | 后端有 `/progress`，前端未充分使用 | 阅读切页/切章时保存并恢复 |

## 三、首页与书架对齐情况

上游首页左侧是导航/设置区，右侧是书架或搜索结果。当前 OpenReader 已改成应用式书架页，但仍需要补齐上游的管理能力。

| 上游能力 | 当前状态 | 后续动作 |
| --- | --- | --- |
| 书架分组 Tabs | 当前为左侧分类面板 | 保留当前更清晰布局，但补分组排序、编辑、删除 |
| 最近阅读 | 当前未突出 | 在书架页顶部加入最近阅读/继续阅读 |
| 刷新书架 | 已有检查更新按钮 | 补刷新状态、更新结果、失败提示 |
| 编辑模式 | 当前无统一编辑模式 | 增加批量选择、批量删除、批量分组 |
| 书籍管理弹窗 | 缺 | 用独立管理抽屉/弹窗实现上游 `BookManage` 能力 |
| 分组管理弹窗 | 只有新增分类 | 补重命名、排序、删除、批量设置 |
| 导入书籍 | 已有本地上传 | 补多文件导入进度、格式提示、失败原因 |
| 浏览书仓 | 有基础页面 | 补目录浏览、上传、删除、批量加入 |
| 搜索结果并入书架布局 | 当前搜索是独立页 | 可保留独立页，但交互要补“返回书架/加载更多/加入书架后阅读” |
| RSS/书海入口 | 缺 | 因后端缺能力，先保留入口规划，不硬做假功能 |

## 四、搜索与书源对齐情况

### 搜索

| 上游能力 | 当前状态 | 后续动作 |
| --- | --- | --- |
| 搜索方式：全部/分组/单源 | 当前可多选启用书源 | 补分组筛选和单源快捷选择 |
| 并发线程配置 | 后端当前固定并发/超时 | 后端支持参数后再补 UI；前端先预留状态展示 |
| 搜索结果加载更多 | 当前一次性结果 | 后端支持分页或事件流后补 |
| 搜索失败来源展示 | 当前无 | 在结果页展示成功/失败/超时来源 |
| 结果加入书架 | 已有 | 补选择分类、加入后跳详情/阅读 |
| 书海探索 | 缺 | 后端缺 `exploreBook` 同等能力，暂列二期 |

### 书源

| 上游能力 | 当前后端 | 当前前端 | 后续动作 |
| --- | --- | --- | --- |
| 书源列表 | 已有 `/sources` | 已有表格 | 增加分组筛选、分页、密度调整 |
| 新增/编辑书源 | 已有 CRUD | 编辑能力不足 | 做完整编辑抽屉，支持规则 JSON 和请求头 |
| 启用/停用 | 已有 `enabled` | 已有 | 保留 |
| 导入书源 | 已有文件导入 | 已有 | 补导入预览和重复处理 |
| 远程书源 | 已有 `/sources/remote` | 已有 | 补远程导入预览 |
| 导出书源 | 已有 | 已有 | 保留 |
| 恢复默认/清空 | 后端缺专门接口 | 缺 | 需要确认是否加入后端 |
| 失效书源检测 | 后端只有单源调试接口 | 有调试，不成体系 | 前端可先批量调用调试接口，后续后端加批量接口 |
| 调试书源 | 已有搜索/目录/正文测试 | 已有基础 UI | 调整成上游式三段调试面板 |
| 替换规则管理 | 模型内有 `TextReplaceRules` | 缺独立 UI | 先集成到书源编辑器，再评估独立规则表 |

## 五、书籍详情对齐情况

当前 `BookDetail.vue` 已有目录、书签、换源入口，但和上游 `BookInfo.vue` 的职责不完全一致。

| 上游能力 | 当前状态 | 后续动作 |
| --- | --- | --- |
| 封面背景和封面预览 | 当前详情视觉较基础 | 按 `BookInfo` 做封面、标题、作者、来源、最新章节区域 |
| 加入书架/开始阅读 | 已有开始阅读；搜索页可加入 | 保持，加入后提供阅读入口 |
| 追更开关 | 后端模型无 `canUpdate` | 需要后端字段或以默认追更策略替代 |
| 分组设置 | 详情页显示分类 | 增加直接修改分类 |
| 本地书籍刷新 | 后端无单书刷新接口 | 需后端补能力 |
| 换源 | 有 `/books/:id/change-source` | 当前入口粗糙 | 改为专用换源面板 |
| 目录跳转 | 已有 | 补搜索、倒序、缓存状态 |
| 书签跳转 | 已有 | 补编辑和删除 |

## 六、设置、同步、备份、用户对齐情况

| 上游能力 | 当前后端 | 当前前端 | 后续动作 |
| --- | --- | --- | --- |
| 后端设定 | 当前固定 Vite proxy/API | 无连接设置 | 保持本项目部署方式，不照搬 IP 设置 |
| 用户登录/注销 | 已有 JWT | 已有登录/退出 | 补用户资料、权限展示 |
| 用户空间管理 | 有 admin 用户列表/更新/清理 | 仅 Settings 粗略接入 | 补管理表格、权限开关、清理确认 |
| 备份用户配置 | 有备份触发/列表/下载/恢复 Legado | 有基础入口 | 补备份列表、下载、恢复上传进度 |
| WebDAV 文件管理 | 有 `/webdav/*` GET/PUT，备份文件在 data/webdav | 无文件管理 UI | 补 WebDAV 文件浏览、下载、上传、删除/还原能力；删除/还原需后端接口 |
| 本地缓存清理 | 后端无细粒度缓存清理 API | 无 | 需后端补章节缓存/书源缓存清理 |
| 自定义背景/字体上传 | 后端无通用 upload/delete 文件接口 | 无 | 需评估是否保留该能力 |
| 简繁转换 | 前端缺 | 无 | 可前端实现，但非第一优先级 |

## 七、后端重构对齐缺口

上游很多 UI 依赖后端能力。当前 Go 后端已经覆盖基础阅读链路，但下列能力还不完整，前端不能直接伪造。

### 已具备

- 用户注册、登录、`/api/me`。
- 书源 CRUD、导入、远程导入、导出、单源调试。
- 书架列表、新增、分类筛选、本地/远程入架。
- 分类列表和创建。
- 章节列表、章节内容获取与缓存。
- 阅读进度模型和 `/api/progress`。
- 书签列表、新增、删除。
- 本地书仓列表和导入。
- WebDAV 基础 GET/PUT。
- 备份触发、列表、下载、Legado 恢复。
- WebSocket 同步通知。

### 明显缺口

| 能力 | 上游对应 | 当前缺口 |
| --- | --- | --- |
| 删除/编辑书籍 | `deleteBook`、`saveBook` | 后端没有 `DELETE /books/:id`、通用更新接口 |
| 批量书籍管理 | `deleteBooks`、批量分组 | 后端缺批量接口 |
| 分组管理 | `saveBookGroup`、排序、删除 | 当前只有创建分类，缺更新/删除/排序 |
| 单书刷新/更新 | `refreshLocalBook`、刷新书架 | 后端只有全局 `check-updates` |
| 追更开关 | `canUpdate` | `Book` 模型缺字段 |
| 换源搜索更多 | `getAvailableBookSource`、`searchBookSource` | 当前只能按指定 `sourceId` 换源，缺候选搜索 |
| 书内搜索 | `searchBookContent` | 后端缺接口 |
| 书源失效检测 | `getInvalidBookSources` | 后端缺批量检测接口 |
| 书源恢复默认/清空 | `deleteAllBookSources`、`deleteBookSourcesFile` | 后端缺接口 |
| 替换规则独立管理 | `saveReplaceRule(s)` | 当前只内嵌在书源规则 |
| WebDAV 删除/还原/导入 | `deleteWebdavFile`、`restoreFromWebdav`、`importFromLocalPathPreview` | 当前只有 GET/PUT 和备份恢复 |
| 本地书仓删除/上传/目录浏览 | `deleteLocalStoreFile`、`uploadFileToLocalStore`、目录列表 | 当前只列根目录文件和导入 |
| RSS | `getRssSources`、`getRssArticles` | 前后端都缺 |
| 书海探索 | `exploreBook` | 前后端都缺 |
| 文件上传背景/字体/封面 | `uploadFile`、`deleteFile` | 后端缺通用文件接口 |

## 八、下一步实施顺序

### P0：先修正在用体验

1. 阅读页继续对齐上游：补阅读页内书架、目录、设置、书源、书签、书籍信息弹层，不再让按钮只跳页面。
2. 阅读进度接入 `/api/progress`：进入恢复，切页/切章保存。
3. 书架页补最近阅读、编辑模式、批量删除/分组的 UI；如果后端缺接口，先同步补后端。

### P1：补核心管理能力

1. `BookDetail.vue` 改造成上游 `BookInfo` + 目录/书签/换源的组合页面。
2. `Sources.vue` 按上游书源管理补分组筛选、导入预览、编辑抽屉、批量失效检测。
3. `Search.vue` 补分组/单源筛选、失败来源、加入书架后的流转。
4. `LocalStore.vue` 补目录浏览、上传、删除、批量导入。

### P2：补设置和同步

1. `Settings.vue` 拆成用户、备份、WebDAV、系统缓存、阅读配置几个面板。
2. 补 WebDAV 文件管理 UI，并补后端删除、下载、还原、导入接口。
3. 补管理员用户空间管理。

### P3：后续能力

1. RSS。
2. 书海/探索。
3. 自定义字体/背景。
4. 简繁转换。
5. Kindle/simple-web 模式。

## 九、当前前端文件的重构归属

| 当前文件 | 现状 | 建议归属 |
| --- | --- | --- |
| `frontend/src/views/Home.vue` | 书架主页面，视觉已重写但管理能力不够 | 拆出 `bookshelf/BookCard.vue`、`CategorySidebar.vue`、`BookManageDialog.vue` |
| `frontend/src/views/Reader.vue` | 阅读页功能集中，已做上游布局修复 | 拆出 `reader/ReaderToolbar.vue`、`ReaderSideRail.vue`、`ReaderSettingsPanel.vue`、`ReaderSourcePanel.vue` |
| `frontend/src/views/Search.vue` | 基础远程搜索 | 拆出 `search/SearchSourceFilter.vue`、`SearchResultList.vue` |
| `frontend/src/views/Sources.vue` | 基础书源管理和调试 | 拆出 `sources/SourceTable.vue`、`SourceEditorDrawer.vue`、`SourceDebugPanel.vue` |
| `frontend/src/views/BookDetail.vue` | 书籍详情、目录、书签、换源混合 | 改为详情壳 + `BookInfoPanel`、`BookTocPanel`、`BookBookmarkPanel`、`BookSourcePanel` |
| `frontend/src/views/LocalStore.vue` | 基础文件列表 | 拆出 `local-store/FileBrowser.vue`、`ImportResultDialog.vue` |
| `frontend/src/views/Settings.vue` | 设置入口粗略 | 拆出 `settings/UserPanel.vue`、`BackupPanel.vue`、`WebDavPanel.vue`、`AdminUserPanel.vue` |
| `frontend/src/api/*` | 已开始模块化 | 继续补 `progress.js`、`bookmarks.js`、`webdav.js` |

## 十、验收标准

后续每完成一块，都按以下标准核对：

1. 上游对应按钮在当前前端有明确入口，不能出现无意义占位按钮。
2. 如果后端未提供能力，前端必须明确隐藏、禁用或提示“后端暂未支持”，不能做假成功。
3. 桌面布局要参考上游比例，阅读页正文默认 800 总宽、670 正文宽。
4. 移动端使用抽屉或全屏弹层，不照搬桌面双侧工具栏。
5. 所有管理类功能必须有加载态、空态、失败态和操作后刷新。
6. 每个页面改完后至少执行 `npm run build`，阅读页相关改动还要用浏览器截图检查桌面和移动端。
