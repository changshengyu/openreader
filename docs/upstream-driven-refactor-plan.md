# OpenReader 上游驱动重构计划

> 目标：从用户保留的 `changshengyu/reader-dev` fork 真实前端结构出发，重新规划 OpenReader 的前端模块、页面逻辑、组件职责、移动端适配和后端补齐顺序；此前重构结果不自动成为后续骨架。
> 上游源码快照：`/private/tmp/changshengyu-reader-dev/web/src`（`changshengyu/reader-dev`，commit `fa22f271849d45f93349ae1636223e27b16a4691`）
> 当前项目：Vue 3 + Vite + Pinia + Element Plus + Go REST API

## 1. 为什么要重新规划

前面几轮改造解决了一些肉眼可见问题，但仍然偏“发现一个界面问题就修一个界面”。这会导致两个风险：

1. 只模仿了局部视觉，没有继承上游的产品工作流。
2. 当前项目拆成多个路由后，容易丢掉上游通过全局弹窗、事件总线和阅读页 Popover 形成的连贯操作链。

所以下一步不能从当前页面倒推，而要从上游项目出发，先弄清：

- 上游为什么只有 `Index.vue` 和 `Reader.vue` 两个主页面。
- 哪些功能是首页工作台的一部分，哪些是阅读器内浮层，哪些是全局管理弹窗。
- 用户从“找书 -> 入架 -> 分组 -> 阅读 -> 换源 -> 书签/笔记 -> 备份同步”的完整路径如何闭环。
- 移动端为什么不是简单压缩桌面，而是使用抽屉、隐藏导航、手势和安全区。

## 2. 上游真实架构

### 2.1 路由层

上游路由极少：

| 路由 | 文件 | 意义 |
| --- | --- | --- |
| `/` | `views/Index.vue` | 首页工作台：书架、搜索、书源、书架管理、用户空间、WebDAV、RSS、书海等入口 |
| `/reader` | `views/Reader.vue` | 阅读器：正文、翻页、目录、书源、书架、阅读设置、书签、搜索正文、听书、自动阅读 |

这说明上游不是“后台管理系统式多页面”，而是“阅读应用式双主场景”：

- 首页负责管理和选择。
- 阅读页负责沉浸阅读和阅读中操作。
- 大量能力通过弹窗/Popover 进入，而不是切换路由打断上下文。

### 2.2 App 全局弹窗层

上游 `App.vue` 不是单纯壳组件，它承载全局弹窗注册中心：

| 全局组件 | 触发方式 | 作用 |
| --- | --- | --- |
| `BookManage` | `showBookManageDialog` | 书籍批量管理、编辑、分组、导出、缓存 |
| `BookInfo` | `showBookInfoDialog` | 书籍信息、封面、简介、追更、分组、加入书架 |
| `BookGroup` | `showBookGroupDialog` | 分组管理或给书设置分组 |
| `ReplaceRule` / `ReplaceRuleForm` | `showReplaceRuleDialog` / `showReplaceRuleForm` | 内容过滤规则管理 |
| `Bookmark` / `BookmarkForm` | `showBookmarkDialog` / `showBookmarkForm` | 书签列表和书签编辑 |
| `SearchBookContent` | `showSearchBookContentDialog` | 当前书内搜索 |
| `UserManage` / `AddUser` | `showUserManageDialog` / `showAddUserDialog` | 用户空间管理 |
| `RssSourceList` / `RssArticleList` / `RssArticle` | 事件总线 | RSS 源、文章列表、文章详情 |
| `MPCode` | 事件总线 | 宣传二维码，当前项目不需要照搬 |

当前 OpenReader 可以继续保留多路由，但必须补一个“应用级弹层层”：跨页面复用同一套书籍信息、书签、分组、书源调试、WebDAV 文件管理组件，避免每个页面各写一套。

## 3. 上游模块拆解和当前重构方向

### 3.1 首页工作台：`Index.vue`

上游首页由两大区域组成：

- 左侧/抽屉导航：搜索设置、最近阅读、后端设定、书源设置、书架设置、用户空间、WebDAV、缓存、主题。
- 右侧主区：书架或搜索/探索结果列表。

核心逻辑：

1. 初始化时加载用户、书架、分组、书源、替换规则、书签、RSS。
2. 书架状态和搜索结果共用同一个图书列表组件形态。
3. 搜索结果中的书可以直接加入书架，加入时先选择分组。
4. 书架列表能切分组、刷新、编辑、进入详情/阅读。
5. 书源导入支持本地 JSON、远程 URL、预览选择、失效检测、调试。
6. 本地书籍导入有预览确认，不是上传后立即默默入库。

当前 OpenReader 方向：

| 上游首页能力 | 当前归属 | 重构策略 |
| --- | --- | --- |
| 书架主列表 | `Home.vue` | 保持独立路由，但做成首页第一屏主体验 |
| 左侧搜索设置 | `Search.vue` + `AppLayout` 快搜 | 搜索页承接高级筛选，侧栏只保留快捷搜索 |
| 最近阅读 | `Home.vue` | 从 `/progress` 或书籍更新时间恢复，作为书架第一块 |
| 书源设置 | `Sources.vue` | 独立页承接完整管理能力，书架只放入口 |
| 书架设置 | `Home.vue` + 全局 `BookManageDialog` | 后端补完后支持批量编辑/删除/分组/导出 |
| 用户空间 | `Settings.vue` | 迁移为设置页用户管理 Tab |
| WebDAV | `Settings.vue` + 后续 `WebDavFileDialog` | 先接备份和说明，再补文件管理 |
| 本地缓存 | `Settings.vue` | 后端补缓存清理 API 后接入 |
| 书海/RSS | 后续 `Discover.vue` 或 `Settings/RSS` | 当前后端缺，先不要做假入口 |

### 3.2 阅读器：`Reader.vue`

上游阅读器是最复杂模块，不能只看布局。它由以下能力组成：

| 能力 | 上游实现 | 当前应拆模块 |
| --- | --- | --- |
| 阅读壳布局 | 左工具栏、右工具栏、底部进度、移动端工具栏 | `ReaderLayout.vue` / `ReaderRails.vue` |
| 正文渲染 | `Content.vue` 支持文本、图片/漫画、音频、EPUB iframe | `ReaderContent.vue`，按内容类型拆渲染器 |
| 目录 | `PopCatalog.vue`，支持刷新目录、缓存状态、跳章 | `ReaderTocPanel.vue` |
| 阅读设置 | `ReadSettings.vue`，主题、字体、行高、宽度、翻页、自动阅读、点击区、过滤规则 | `ReaderSettingsPanel.vue` + `reader` store |
| 换书 | `BookShelf.vue` 阅读页内浮层 | `ReaderShelfPanel.vue` |
| 换源 | `BookSource.vue` 可用来源、刷新、分组、加载更多、SSE | `ReaderSourcePanel.vue` |
| 书内搜索 | `SearchBookContent.vue` + `showSearchContent` 回跳正文 | `ReaderSearchPanel.vue` |
| 书签/笔记 | `Bookmark.vue` + `BookmarkForm.vue` | `ReaderBookmarkPanel.vue` / `BookmarkDialog.vue` |
| 自动阅读 | 像素滚动/段落滚动 | `useAutoReading.js` |
| 听书 | speechSynthesis、段落定位、上一段/下一段 | `useSpeechReader.js` |
| 点击/手势 | 桌面点击区、移动端滑动、工具栏显隐 | `useReaderGesture.js` |
| 进度保存 | 本地 cache + 后端进度 | `useReaderProgress.js` |

当前阅读页已经做了外观对齐，但下一步要把“按钮真实意义”继续补全：

- 左侧 `书架` 不能只跳详情，应打开阅读页内书架浮层。
- 左侧 `书源` 应打开阅读页内换源浮层，不应跳页面。
- 右侧 `搜索` 应支持全书搜索，而不只是当前章节搜索；后端缺全书搜索接口则先做当前章节并标明。
- `信息` 应复用全局书籍信息组件。
- `笔记` 应进入书签/笔记编辑组件，而不是简单 textarea。
- `下载/缓存` 应对应上游缓存章节内容，不应只是“跳到底部”。

### 3.3 书籍信息：`BookInfo.vue`

上游 `BookInfo` 的意义不是普通详情页，它是所有场景共用的“书籍信息弹窗”：

- 书架中点击封面可打开。
- 搜索结果中可打开。
- 阅读页右侧信息按钮可打开。
- 如果书未入架，显示“加入书架”。
- 如果已入架，显示分组、追更、来源、最新章节。

当前 OpenReader 应该拆成：

| 组件 | 作用 |
| --- | --- |
| `BookInfoPanel.vue` | 展示封面、书名、作者、来源、最新章节、简介 |
| `BookActionBar.vue` | 开始阅读、加入书架、换源、设置分组、更新 |
| `BookDetail.vue` | 路由页，承接目录、书签、来源、详情 Tab |
| `BookInfoDialog.vue` | 全局弹窗，阅读页/搜索页/书架复用 |

### 3.4 书架管理：`BookManage.vue` + `BookGroup.vue`

上游书架不只是列表，它有两个管理层：

- `BookManage`：批量删除、编辑书籍 JSON、分组、导出、缓存。
- `BookGroup`：分组显示/隐藏、排序、添加、删除、给书设置分组。

当前后端缺口很明显：

- `DELETE /books/:id`
- 批量删除书籍
- 更新书籍信息
- 导出书籍
- 删除章节缓存
- 更新/删除/排序分类

所以计划必须是前后端一起做，而不是前端摆按钮。

### 3.5 书源管理

上游书源相关能力分三层：

1. 首页书源设置入口：导入、远程、失效、调试、探索。
2. 书源管理表格：分组、分页、新增、编辑、清空、恢复默认、导出、删除。
3. 阅读页换源：当前书的可用来源、搜索更多来源、加载更多、SSE。

当前 OpenReader 已有基础 CRUD 和调试，但缺：

- 导入预览和重复处理。
- 恢复默认/清空书源。
- 批量失效检测接口。
- 当前书可用换源候选。
- 事件流搜索更多来源。
- 书海探索。

### 3.6 本地书仓与 WebDAV

上游 `LocalStore.vue` 和 `WebDAV.vue` 都是文件浏览器，不只是列表：

- 支持目录进入和 `..` 返回。
- 支持删除、批量删除。
- 支持上传。
- 支持预览并加入书架。
- WebDAV 支持还原备份、下载文件、导入文件到书架。

当前 OpenReader 后端只覆盖了一部分：

- 本地书仓：根目录列表 + 导入。
- WebDAV：GET/PUT + 备份文件下载。

下一步要按“文件浏览器组件”设计，而不是两个孤立表格页：

| 组件 | 用于 |
| --- | --- |
| `FileBrowserTable.vue` | LocalStore 和 WebDAV 共用 |
| `FileImportPreviewDialog.vue` | 本地/云端文件导入前确认 |
| `BackupList.vue` | WebDAV 备份文件和下载/还原 |

### 3.7 替换规则

上游替换规则是阅读体验的一部分：

- 阅读设置里有“过滤规则管理”入口。
- 规则可以按书名/书 URL 作用域生效。
- 阅读内容进入渲染前执行替换。

当前 Go 模型里只有书源规则内嵌 `TextReplaceRules`，还没有用户级替换规则表。因此要分两阶段：

1. 先在书源编辑器里支持书源级正文替换。
2. 后端增加用户级替换规则表后，再做全局 `ReplaceRule` 管理。

### 3.8 RSS 和书海

上游支持 RSS 和书海探索，但当前后端没有对应模型和接口。这里不要在前端做空壳冒充，应列为二期：

- RSS：`rss_sources`、`rss_articles`、抓取/解析接口。
- 书海：基于书源 `exploreUrl` 的发现接口。

## 4. 移动端适配原则

上游移动端不是普通响应式压缩，它有自己的交互策略：

### 4.1 首页移动端

上游 `Index.vue` 在窄屏使用：

- `collapseMenu`：导航区默认隐藏。
- 顶部菜单按钮打开侧边导航。
- 手势右滑打开导航，左滑关闭导航。
- 弹窗全屏。
- 书架列表保持阅读应用的卡片/行项目，不使用密集后台表格。

OpenReader 对应策略：

- 桌面保留侧边导航；移动端必须保留上游式侧边导航，不使用底部主导航。
- 书架页移动端第一屏只保留：书架标题、分组和书架列表；搜索、最近阅读、导入和管理入口放在侧边导航。
- 管理能力进入全局弹层或阅读页内浮层，入口位置必须能对应上游组件职责。
- 表格页移动端不能硬塞列，应改成上游式行项目/卡片列表或全屏弹层。

### 4.2 阅读器移动端

上游阅读器移动端有：

- `miniInterface`。
- 左右工具栏隐藏，使用顶部/底部工具栏。
- 中心点击区显示/隐藏工具栏。
- `clickMethod` 支持“下一页 / 自动 / 不翻页”。
- 自动点击区：左右滑动模式按左右翻页；上下滑动和上下滚动模式按上下翻页；中心区域只负责工具栏显隐。
- 只有左右滑动/上下滑动模式拦截滑动翻页；上下滚动模式必须保留浏览器原生手指滚动距离。
- safe-area 支持。
- 弹窗全屏。

OpenReader 对应策略：

- 阅读器桌面继续按 800/670 宽度和左右工具栏。
- 移动端正文全宽，不显示左右固定栏。
- 移动端工具栏仅在点击中心区域后显示。
- 目录、书签、书源、设置统一全屏抽屉。
- 翻页和滚动模式都要保存位置，字体/行高变化后重新定位；不得把上下滚动手势改造成固定距离翻页。

## 5. 当前项目目标信息架构

我们可以保留多路由，但需要“上游双场景”的心智模型：

```text
AppLayout
  Home.vue              书架主场景
    BookShelfList
    RecentReading
    GroupSidebar
    BookManageDialog
    ImportBookDialog

  Search.vue            找书和入架
    SearchConsole
    SourceScopeSelector
    SearchResultList
    AddBookDialog

  Sources.vue           书源维护
    SourceTable
    SourceEditorDrawer
    SourceImportPreview
    SourceDebugDialog
    SourceHealthPanel

  LocalStore.vue        本地文件入架
    FileBrowserTable
    ImportPreviewDialog

  Settings.vue          用户、备份、WebDAV、系统能力
    AccountPanel
    BackupPanel
    WebDavPanel
    AdminUsersPanel
    CachePanel

ReaderLayout
  Reader.vue            阅读主场景
    ReaderContent
    ReaderRails
    ReaderShelfPanel
    ReaderSourcePanel
    ReaderTocPanel
    ReaderSettingsPanel
    ReaderBookmarkPanel
    ReaderSearchPanel
    ReaderSpeechBar
```

## 6. 跨页面流转

### 6.1 找书到阅读

```text
Search.vue 输入关键词
  -> 选择 全部/分组/单源/自选书源
  -> 展示搜索结果
  -> 点击书籍信息：BookInfoDialog
  -> 加入书架：选择分组
  -> 成功后：
       A. 去详情页 BookDetail.vue
       B. 直接进入 Reader.vue
```

### 6.2 书架到阅读

```text
Home.vue
  -> 最近阅读：直接 Reader.vue + progress
  -> 书籍卡片：BookDetail.vue
  -> 封面/信息按钮：BookInfoDialog
  -> 管理：BookManageDialog
```

### 6.3 阅读中操作

```text
Reader.vue
  左栏书架 -> ReaderShelfPanel -> changeBook
  左栏书源 -> ReaderSourcePanel -> changeSource -> reload catalog/content
  左栏目录 -> ReaderTocPanel -> goChapter
  左栏设置 -> ReaderSettingsPanel -> reflow/reposition
  右栏书签 -> ReaderBookmarkPanel
  右栏搜索 -> ReaderSearchPanel -> show matched paragraph
  右栏信息 -> BookInfoDialog
  右栏听书 -> ReaderSpeechBar
  右栏缓存 -> CacheContentDialog
```

### 6.4 管理和设置

```text
Settings.vue
  备份 -> trigger/list/download/restore
  WebDAV -> file browser
  用户管理 -> admin table
  缓存 -> clear source/chapter/content cache
```

## 7. 后端补齐顺序

前端如果要完整对齐，后端必须补这些能力。优先级如下：

### P0：直接影响当前体验

1. `DELETE /books/:id`
2. `PUT /books/:id` 更新书籍元信息
3. `PUT /categories/:id`、`DELETE /categories/:id`、分类排序字段
4. `GET /progress/:bookID` 在书架列表聚合最近阅读信息
5. `POST /books/:id/refresh` 单书刷新
6. 阅读页书内搜索接口：`GET /books/:id/search?q=`

### P1：对齐上游核心管理

1. 批量书籍删除/分组
2. 书籍导出
3. 章节缓存清理和缓存状态
4. 书源导入预览、重复处理
5. 批量失效检测
6. 当前书换源候选：`GET /books/:id/available-sources`
7. 搜索更多换源：SSE 或分页接口

### P2：文件和备份

1. LocalStore 目录浏览、上传、删除
2. WebDAV 文件删除、还原、导入到书架
3. 缓存统计和清理

### P3：扩展能力

1. 用户级替换规则
2. RSS
3. 书海探索
4. 自定义字体/背景上传
5. EPUB/漫画/音频专项渲染完善

## 8. 前端实施顺序

### 阶段 A：停止页面级重复，建立共享组件

1. `components/book/BookInfoPanel.vue`
2. `components/book/BookActionBar.vue`
3. `components/book/BookTocPanel.vue`
4. `components/book/BookmarkList.vue`
5. `components/common/FileBrowserTable.vue`
6. `components/sources/SourceEditorDrawer.vue`
7. `components/reader/ReaderPanelShell.vue`

验收：书架、搜索、详情、阅读页展示同一本书信息时，使用同一套组件。

### 阶段 B：书架和搜索按上游工作流闭环

1. 书架补最近阅读的真实进度。
2. 书架管理弹窗补批量选择，但没有后端能力的按钮先隐藏。
3. 搜索结果支持书籍信息弹窗。
4. 加入书架必须支持选择分组。
5. 搜索页展示来源成功/失败/超时状态。

验收：用户从搜索入架到阅读，不需要理解技术细节，也不会遇到假按钮。

### 阶段 C：阅读器组件化

1. 从 `Reader.vue` 拆出左右栏和所有面板。
2. 实现阅读页内书架浮层。
3. 实现阅读页内换源浮层。
4. 阅读设置补齐上游字段：字体粗细、段距、宽度、点击区、自动阅读、方案。
5. 书签/笔记编辑复用同一组件。
6. 移动端全屏抽屉和工具栏显隐。

验收：阅读页左/右每个按钮都有真实上游对应意义。

### 阶段 D：书源、书仓、WebDAV 管理

1. 书源编辑器组件化。
2. 导入书源先预览再保存。
3. 失效检测结果可筛选。
4. 本地书仓改文件浏览器。
5. WebDAV 改文件浏览器。

验收：管理页不再是接口演示，而是可维护大量数据的工具。

### 阶段 E：二期能力

1. 替换规则全局管理。
2. RSS。
3. 书海探索。
4. 自定义字体/背景。
5. EPUB/漫画/音频专项阅读体验。

## 9. 设计验收口径

每个模块完成时都按这些标准验收：

1. 能在上游源码中找到对应组件/按钮意义。
2. 当前实现必须说明它对应上游哪个组件、哪个方法、哪个工作流。
3. 后端没有的能力不能做假按钮；要么隐藏，要么禁用并说明缺口。
4. 桌面端布局参考上游比例，不变成后台系统。
5. 移动端必须单独验收，不允许只是挤压桌面表格。
6. 关键链路要验：搜索入架、书架阅读、阅读中换章、书签跳转、设置变化后重排、备份恢复。
7. 每次改动后执行 `npm run build`；阅读器改动还要浏览器截图检查桌面和移动端。

## 10. 对当前已改动的再评价

已经完成的改动可以保留，但需要纳入这个蓝图重新整理：

- 阅读页外观对齐是正确方向，但必须继续补“阅读页内浮层”。
- 书架、搜索、书源、书仓、设置页已开始工作台化，但还需要共享组件化和后端补齐。
- 之前的多路由结构可以保留，但不能丢失上游“全局弹窗 + 阅读页 Popover”的交互关系。
- 下一步不应继续堆单文件页面，而应该先抽共享组件，再逐页替换。

## 11. 上游对齐闸门

后续每一批提交前必须执行这个检查：

1. 标明本批改动对应上游哪个文件、组件和方法。
2. 如果当前实现和上游不同，必须写明原因：后端缺口、技术栈差异，或用户明确要求。
3. 不允许用“更像移动 App / 更现代 / 更方便”作为偏离上游的理由。
4. 移动端书架检查：侧边导航可滑动打开/关闭，正文区不出现横向溢出，第一屏直接看到书架列表。
5. 移动端阅读检查：默认不显示工具栏和上一章/进度/下一章；中心点击才出现；滚动模式手指滑动距离必须等于原生滚动距离。
6. 阅读设置检查：阅读方式、全屏点击、字体、字号、行高、主题等行为必须能在正文立即生效。
