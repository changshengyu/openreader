# OpenReader 前端重写计划

> 目标：以 `hectorqin/reader` / Reader3 的 Web 体验为参照，把当前 OpenReader 前端从“功能页面集合”重写成一个可长期维护、适合桌面和移动端的阅读应用。

## 参考来源

- 上游项目功能清单：[hectorqin/reader README](https://github.com/hectorqin/reader)
- 上游项目文档：[hectorqin/reader doc.md](https://github.com/hectorqin/reader/blob/master/doc.md)
- 当前项目后端能力：`backend/api/*`、`backend/engine/*`、`backend/services/*`
- 当前项目前端入口：`frontend/src/router/index.js`、`frontend/src/views/*`、`frontend/src/stores/*`

## 重写原则

1. 保留 Vue 3 + Vite + Pinia + Vue Router + Element Plus 技术栈，避免在重写前端时扩大技术变量。
2. 页面不再按“接口演示”组织，而是按阅读应用的实际工作流组织：找书、入架、管理、阅读、同步、备份。
3. 阅读器优先级最高。书架和管理页可以逐步完善，但阅读器必须稳定、沉浸、可恢复进度。
4. 所有新增 UI 都要承接后端已有能力，后端已有但前端缺入口的能力要显式补齐。
5. 移动端不是事后适配。书架、搜索、阅读器和抽屉交互都要有移动端形态。
6. 先建立组件边界，再重写页面，避免继续把状态、布局、请求和业务逻辑堆在单个 `*.vue` 文件里。

## 当前前端问题

### 产品结构问题

- `App.vue` 使用固定侧边栏后台式布局，和阅读应用气质不匹配。
- 书架页把导入、分类、创建分类和图书列表堆在同一层，缺少主次。
- 搜索、书源、本地书仓是“能调用接口”的管理页，还没有形成 Reader3 式的找书和维护流程。
- 用户、同步、备份、WebDAV、管理能力没有统一入口。
- 书籍详情页承担了目录、书签、换源等动作，但信息密度和动作层级仍偏粗糙。

### 阅读器问题

- `Reader.vue` 体积过大，章节加载、分页、设置、TTS、手势、键盘、书签、进度保存耦合在一个文件里。
- 翻页和滚动模式虽然已有，但测量、恢复、字体变化后的重新分页还需要系统化。
- 工具栏和底栏更接近“功能按钮集合”，还没有形成沉浸阅读的显隐逻辑。
- 移动端点击区域、手势、抽屉、顶部/底部工具条需要重新设计。
- TTS 控制只有基础播放状态，缺少段落定位、高亮和自动续读的结构。

### 工程结构问题

- 缺少 `components/`、`layouts/`、`composables/reader/` 等清晰分层。
- API 调用分散在页面里，后续错误态、加载态和重试不好统一。
- 书架、搜索、书源、阅读器状态边界不清晰。
- 全局样式和页面样式混杂，主题系统没有形成设计 token。

## 上游功能对照

| 功能 | hectorqin/reader | 当前后端 | 当前前端 | 重写动作 |
| --- | --- | --- | --- | --- |
| 书源管理 | 有 | 已有 CRUD、导入、导出、远程导入、调试接口 | 有基础页 | 重写为分组、筛选、检测、调试一体页 |
| 书架管理 | 有 | 已有书籍、分类、远程书籍、本地导入 | 有基础页 | 重写网格/列表、分类侧栏、最近阅读、更新状态 |
| 书架布局 | 有 | 不涉及 | 简单网格 | 增加密度、排序、视图切换、移动端布局 |
| 搜索 | 有 | 已有并发搜索 API | 有基础页 | 重写聚合结果、来源筛选、去重、加入书架流程 |
| 书海/发现 | 有 | 暂无明确 API | 无 | 预留发现页和书源推荐/排行容器 |
| 看书 | 有 | 已有章节、缓存、进度、书签 | 有阅读器 | 重点重写 |
| 移动端适配 | 有 | 不涉及 | 不完整 | 全页面响应式重写 |
| 换源 | 有 | 已有 `change-source` | 书籍详情里有入口 | 做成专用换源面板，支持搜索更多来源 |
| 翻页方式 | 有 | 不涉及 | 有滚动/翻页/分页 | 抽出分页引擎，做稳定恢复 |
| 手势支持 | 有 | 不涉及 | 有 composable 雏形 | 重写点击区、滑动、工具栏显隐 |
| 自定义主题 | 有 | 不涉及 | 有基础主题 | 设计 token 化，支持阅读器主题预览 |
| 自定义样式 | 有 | 暂无 reader.css 加载 | 无 | 预留自定义 CSS 管理入口 |
| WebDAV 同步 | 有 | 已有 WebDAV API | 无管理入口 | 新增同步/备份设置页 |
| 文字替换过滤 | 有 | 模型已有规则字段 | 无配置 UI | 书源编辑器补齐替换规则 |
| 听书 | 有 | 前端能力 | 有基础 TTS | 补齐段落高亮、续读、移动端控制 |
| 用户配置备份恢复 | 有 | 已有备份服务/API | 无完整入口 | 新增备份恢复页 |
| 漫画 | 有 | 暂无完整能力 | 无 | 标记为后续，预留内容类型 |
| 音频 | 有 | 暂无完整能力 | 无 | 标记为后续，预留内容类型 |
| 书源失效检测 | 有 | 可基于调试/搜索扩展 | 无 | 先做检测入口和结果视图 |
| TXT/EPUB/UMD/PDF | 有 | 已有解析器 | 导入 UI 粗糙 | 重写导入体验和状态展示 |
| 书籍分组 | 有 | 已有分类 | 有基础标签 | 重写为书架分类侧栏/管理弹窗 |
| RSS 订阅 | 有 | 暂无 | 无 | 后续预留 `Feeds` 页面 |
| 定时更新书架 | 有 | 已有 scheduler | 无入口 | 新增手动检查和结果提示 |
| 并发搜书 | 有 | 已有 | 有基础页 | 重写进度、超时、失败来源展示 |
| 本地书仓 | 有 | 已有 | 有基础页 | 重写批量导入和文件状态 |
| Kindle/simple-web | 有 | 暂无 | 无 | 后续预留轻量阅读模式 |

## 目标信息架构

### 主导航

- 书架：默认首页，承载阅读入口。
- 搜索：远程找书、并发搜书、加入书架。
- 发现：预留书海/RSS/推荐入口，第一阶段可隐藏或显示空状态。
- 书源：管理、导入、订阅、调试、失效检测。
- 本地书仓：扫描本地书籍并批量导入。
- 设置：用户、同步、备份、WebDAV、外观、系统信息。

### 阅读器独立导航

阅读器不使用主侧边栏，采用独立全屏壳：

- 顶部：返回、书名、章节、同步/缓存状态。
- 正文：阅读纸张或全屏阅读区。
- 左右点击区：上一页/下一页。
- 中央点击区：显示/隐藏工具栏。
- 底部：进度、上一页、下一页、章节进度拖动。
- 抽屉：目录、书签、设置、换源、TTS。

## 推荐目录结构

```text
frontend/src/
  api/
    client.js
    books.js
    categories.js
    sources.js
    search.js
    localStore.js
    backup.js
    admin.js
  layouts/
    AppLayout.vue
    ReaderLayout.vue
  components/
    common/
      EmptyState.vue
      ErrorState.vue
      PageHeader.vue
      LoadingBlock.vue
    bookshelf/
      BookCard.vue
      BookListRow.vue
      BookshelfToolbar.vue
      CategorySidebar.vue
      ImportBookDialog.vue
    reader/
      ReaderSurface.vue
      ReaderToolbar.vue
      ReaderTocDrawer.vue
      ReaderBookmarksDrawer.vue
      ReaderSettingsDrawer.vue
      ReaderProgressBar.vue
      ReaderTtsBar.vue
    sources/
      SourceTable.vue
      SourceImportDialog.vue
      SourceEditorDrawer.vue
      SourceDebugPanel.vue
      SourceHealthPanel.vue
    search/
      SearchBox.vue
      SearchSourceFilter.vue
      SearchResultList.vue
      SearchResultItem.vue
    settings/
      BackupPanel.vue
      WebDavPanel.vue
      UserPanel.vue
  composables/
    reader/
      useReaderSession.js
      usePagination.js
      useReaderProgress.js
      useReaderGestures.js
      useReaderKeyboard.js
      useReaderTTS.js
    useAsyncState.js
    useResponsive.js
  stores/
    app.js
    user.js
    bookshelf.js
    reader.js
    sources.js
    settings.js
  styles/
    tokens.css
    global.css
    reader.css
```

## 页面重写方案

### AppLayout

目标：从后台侧边栏改成阅读应用外壳。此文档早于上游二次审查；移动端导航以后续 `docs/upstream-driven-refactor-plan.md` 和 `docs/upstream-alignment-audit.md` 为准。

- 桌面端：左侧主导航 + 顶部工作区标题 + 内容区。
- 移动端：按上游 `Index.vue` 保留可滑动侧边导航，不使用底部导航。
- 登录状态、离线状态、同步状态统一放在 shell 层。
- 快捷搜索保留，但交互改为跳转搜索页并自动触发搜索。

### 书架页

核心体验：打开就是“我现在可以读什么”。

- 顶部标题区按上游保留书架标题、书海、RSS、刷新、编辑等轻量文字入口；不放“导入书籍/本地书仓/搜索书架”这类会挤压移动端第一屏的工具组。
- 分组栏按上游作为书架主区顶部标签：全部、本地、未分组和显示状态开启且有书的分类。
- 主区：桌面默认上游式多列紧凑书籍块；移动端固定单列书籍行，不使用桌面表格压缩。
- 卡片信息：书名、作者、最近章节、阅读进度、更新状态、来源类型。
- 快捷动作：点击书籍继续阅读，封面/详情入口打开全局书籍信息；批量管理、导入、本地书仓和设置入口放在上游式侧边导航或全局弹层。
- 空状态：引导导入本地书籍或搜索远程书籍。

### 书籍详情页

核心体验：围绕一本书做阅读前后的管理。

- 顶部：封面/标题/作者/来源/最新章节/继续阅读。
- 操作：开始阅读、检查更新、换源、分类、删除。
- Tab：目录、书签、书源、详情。
- 目录支持搜索、倒序、跳转。
- 书签支持删除和跳转。
- 换源面板支持当前书源、可用书源、搜索更多来源。

### 搜索页

核心体验：像 Reader3 一样并发找书并快速入架。

- 搜索框固定在顶部，支持路由参数 `?q=`。
- 来源筛选：全部、启用书源、分组、单个书源。
- 搜索过程展示：已完成/超时/失败的书源数量。
- 结果列表按书名作者聚合，展开可见不同来源。
- 加入书架时可选择分类。
- 加入后提示进入详情或开始阅读。

### 书源页

核心体验：书源很多时仍然能维护。

- 顶部：导入 JSON、远程订阅、导出、失效检测。
- 表格：名称、分组、启用、搜索 URL、最后检测、错误摘要、操作。
- 侧栏/抽屉编辑器：基础字段、规则 JSON、请求头、替换规则。
- 调试面板：搜索测试、目录测试、正文测试。
- 失效检测：批量跑测试关键词，展示成功率和错误。

### 本地书仓页

核心体验：批量导入本地文件。

- 文件列表：文件名、格式、大小、是否已导入、解析状态。
- 批量选择、批量导入、刷新扫描。
- 导入弹窗支持选择分类、覆盖策略。
- 导入结果显示成功、跳过、失败原因。

### 设置页

第一阶段需要承接已有后端能力：

- 用户信息：当前用户、退出登录。
- 同步状态：WebSocket 在线状态、最近同步时间。
- 备份恢复：手动备份、备份列表、下载、恢复 Legado 备份。
- WebDAV：地址说明、连接状态、最近文件。
- 系统：版本、健康检查、存储目录说明。

## 阅读器重写方案

### 状态模型

`useReaderSession` 负责当前书籍阅读会话：

- `bookId`
- `book`
- `chapters`
- `currentChapterIndex`
- `chapter`
- `content`
- `loading`
- `error`
- `loadChapter(index, restoreOffset)`
- `nextChapter()`
- `prevChapter()`

`usePagination` 负责分页：

- 输入：内容容器、正文容器、模式、字体、行高、页面尺寸。
- 输出：`page`、`pageCount`、`nextPage()`、`prevPage()`、`measure()`。
- 字体、行高、窗口尺寸变化后重新测量。
- 保存进度时区分滚动偏移和页码。

`useReaderProgress` 负责保存和恢复：

- 防抖保存。
- 章节切换立即保存。
- 页面卸载前保存。
- 保存失败时轻提示，不阻塞阅读。

`useReaderGestures` 负责移动端：

- 左右滑动翻页。
- 中央点击显示工具栏。
- 左右点击区翻页。
- 长按或边缘动作后续预留。

`useReaderTTS` 负责听书：

- 播放、暂停、停止。
- 语速、音调、声音选择。
- 当前段落索引。
- 朗读完成后自动下一段/下一页/下一章。

### 阅读模式

- 滚动：正文容器自然滚动，保存 `scrollTop`。
- 翻页：一屏一页，垂直 `translateY`，保存页码。
- 分页：第一阶段与翻页共用引擎，后续再扩展仿真或横向分页。

### 阅读设置

- 字体：系统、宋体、楷体、仿宋。
- 字号：18-44。
- 行高：1.4-3.5。
- 页面宽度：移动端自动，桌面端 560-920。
- 主题：白天、护眼、羊皮纸、夜间、纯黑、自定义。
- 背景：颜色、图片。
- 亮度。
- 段落间距。
- 首行缩进。

### 阅读器验收点

- 从书架继续阅读能恢复到正确章节和位置。
- 字号、行高、页面宽度变化后不会出现空白页或页码越界。
- 最后一页继续下一页能进入下一章。
- 第一页上一页能进入上一章末尾。
- 目录跳转、书签跳转、详情页跳转都能工作。
- 移动端点击区不误触抽屉。
- TTS 播放时切章、退出页面会正确停止。

## API 承接清单

前端重写需要显式承接这些已有接口：

```text
GET    /api/me
GET    /api/categories
POST   /api/categories
GET    /api/books
POST   /api/books
POST   /api/books/remote
POST   /api/books/check-updates
GET    /api/books/:id
PUT    /api/books/:id/category
POST   /api/books/:id/change-source
GET    /api/books/:id/chapters
GET    /api/books/:id/chapters/:index/content
GET    /api/books/:id/bookmarks
POST   /api/books/:id/bookmarks
DELETE /api/bookmarks/:id
POST   /api/search
GET    /api/sources
POST   /api/sources
GET    /api/sources/:id
PUT    /api/sources/:id
DELETE /api/sources/:id
POST   /api/sources/import
GET    /api/sources/export
POST   /api/sources/remote
POST   /api/sources/:id/test
POST   /api/sources/:id/test-chapter
POST   /api/sources/:id/test-content
GET    /api/local-store
POST   /api/local-store/import
GET    /api/progress/:bookID
PUT    /api/progress
POST   /api/backup/trigger
GET    /api/backup/list
GET    /api/backup/download/:name
POST   /api/backup/restore-legado
GET    /ws/sync
GET    /webdav/*
PUT    /webdav/*
```

## 实施阶段

### Phase 1：前端骨架和设计系统

- 新建 `layouts/`、`components/`、`api/` 分层。
- 拆分 API 模块，保留 `client.js` 作为 Axios 实例。
- 新建 `styles/tokens.css`，定义颜色、间距、阴影、字体、阅读器 token。
- 重写 `App.vue` 为壳组件，实际布局进入 `AppLayout.vue`。
- 增加响应式基础：桌面侧栏，移动端上游式可滑动侧边导航。
- 保证登录页和路由守卫正常。

验收：

- `npm run build` 通过。
- 登录、退出、主导航、移动端导航可用。

### Phase 2：书架和书籍详情

- 重写 `Home.vue` 为书架工作台。
- 新增书籍卡片、列表行、分类侧栏、导入弹窗。
- 重写 `BookDetail.vue`，拆出目录、书签、换源、详情。
- 接入检查更新、分类变更、继续阅读。

验收：

- 导入 TXT/EPUB/PDF/UMD 的入口清晰。
- 分类筛选可用；侧边栏搜索框按上游进入书源搜索流程，本地书籍搜索作为独立入口，不在首页自创 `shelfQ` 过滤。
- 从书架和详情页都能继续阅读。

### Phase 3：阅读器

- 重写 `Reader.vue`，引入 `ReaderLayout` 和 reader 组件。
- 抽出阅读器 composables。
- 重做分页、进度、设置、目录、书签、TTS、手势、键盘。
- 添加移动端阅读布局。

验收：

- 滚动、翻页、分页全部可用。
- 章节切换和进度恢复稳定。
- 桌面/移动端阅读器不出现文本遮挡和按钮溢出。

### Phase 4：搜索、书源、本地书仓

- 重写 `Search.vue`，聚合并发搜索结果。
- 重写 `Sources.vue`，补齐导入、远程订阅、导出、编辑、调试、检测。
- 重写 `LocalStore.vue`，优化批量导入体验。

验收：

- 搜索远程书籍并加入书架可用。
- 书源导入、导出、启停、调试可用。
- 本地书仓批量导入有明确结果反馈。

### Phase 5：设置、同步、备份

- 新增 `Settings.vue` 或设置相关子页。
- 接入 WebSocket 同步状态。
- 接入备份触发、备份列表、下载、恢复 Legado。
- WebDAV 说明和状态入口。
- 管理员能力可后续展开，但导航中预留入口。

验收：

- 备份恢复相关 API 有前端入口。
- 用户能看懂 WebDAV 使用方式。
- 同步在线/离线状态展示一致。

### Phase 6：查漏补缺和文档更新

- 更新 `docs/frontend-refactor.md`。
- 维护上游功能对照表，标记已完成、部分完成、未开始。
- 补充 Playwright 或手动验收记录。
- 清理旧样式和废弃 composables。

## 第一轮不做但要预留

- 漫画阅读器。
- 音频播放器。
- RSS 源完整管理。
- Kindle/simple-web 独立轻量端。
- 书源登录。
- WebView 高级书源兼容。
- 自定义 CSS 文件上传和热加载。

## 风险和注意事项

- 当前 git 工作区几乎全是新增/修改文件，重写时不能回滚用户已有改动。
- 阅读器分页依赖真实 DOM 尺寸，必须用浏览器验证，不能只靠 `npm run build`。
- Element Plus 可以继续用，但页面不能呈现为默认后台模板。
- API 错误响应格式目前不完全统一，前端需要兼容 `error` 字符串和 `error.message` 两类格式。
- 搜索和书源调试涉及远程网络，UI 要明确展示超时和失败来源。
- 自定义主题、背景图片、TTS 设置会落到 localStorage/Pinia persist，字段变更要考虑兼容旧数据。

## 最小可交付版本

如果需要尽快看到效果，最小闭环建议是：

1. 新 AppLayout。
2. 新书架页。
3. 新阅读器。
4. 新搜索页。
5. 新书源页。

这五部分完成后，OpenReader 的核心体验就会从“重构验证版”变成“可日常使用版”。设置、备份、本地书仓可以随后补强，但目录结构和导航入口应在第一轮就建好。
