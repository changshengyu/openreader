# Reader 菜单栏与弹出面板（移动/桌面）第二轮固定基准合同（P0）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

日期：2026-11-03。状态：**implementation-complete / regression-validated**（发布阻塞于 Go 公告门，见第 8 节）。

## 1. 来源、范围与方法

用户 2026-11-03 要求：移动端点击中间唤起的菜单栏及其次级布局、桌面端菜单栏布局，以及它们的内部逻辑都与上游对齐，
之前要求过的优化除外。

上游权威文件：

- `web/src/views/Reader.vue`：模板 1–251（`.tool-bar`、两个 `.read-bar`、`.click-zone`）；`leftBarTheme`/`rightBarTheme`/
  `popperWidth`/`readWidthConfig`/`readingProgress`/`showPrevPageStyle`/`showMenuZoneStyle`/`showNextPageStyle`（636–754）；
  `toNextChapter`/`toLastChapter`（1350–1399）、`computePages`/`showPage`（1403–1536）、`eventHandler`（1713–1793）、
  `scrollHandler`（2388–2441）、`showCacheContent`/`cacheChapterContent`（2678–2736）；样式 3040–3816。
- `web/src/components/BookShelf.vue`、`BookSource.vue`、`PopCatalog.vue`：弹出面板的标题区、列表与样式。
- `web/src/App.vue`：`.popper-component`（mini：`top/left:0`、`100vw`、`overflow-x:hidden`）、`.popper-component.el-popover`
  无边框无阴影、`#app` 字体；`web/src/assets/fonts/iconfont.*`；`web/src/plugins/vuex.js#currentThemeConfig`。

OpenReader 对应：`frontend/src/components/reader/ReaderMobileChrome.vue`、`ReaderDesktopTools.vue`、`ReaderDesktopProgress.vue`、
`ReaderCachePanel.vue`、`ReaderMobileWorkspacePanel.vue`、`ReaderDesktopWorkspacePanel.vue`、`ReaderShelfPanel.vue`、
`ReaderTocPanel.vue`、`SourceSwitchPanel.vue`、`ReaderClickZones.vue`、`views/Reader.vue`，以及
`composables/useReaderProgressControls.js`、`useReaderLayout.js`、`useReaderTools.js`、`utils/readerPagination.js`。

方法：在会话临时目录用上游 `web/` 原样构建（vue-cli 4，element-ui 2.15.14，vue 2.7.16），Playwright 拦截 `/reader3/*`
返回模拟数据；OpenReader 用当前 main（`69c27bc`）构建，拦截 `/api/*` 返回同一份数据（12 章书、当前第 3 章、6 本书架书、
4 个来源）。两边在 390×844、360×800、1440×900、1024×1366 测量几何、computed style 与命中点，日间与夜间各一遍并截图。
探针脚本只在临时目录，不入库；下面的数字是实测值。

## 2. 不重开的用户要求差异

- 原生连续手指/滚轮滚动 + 点击仍分页；设置里的减号/数值/加号步进器（含朗读条数值）。
- 亮度、字体预览与字号预设（设置面板）；运行章节标题（桌面与移动都显示 `第 N 章 章节名`）。
- 书签面板“添加当前段落”；首页移动侧栏固定底部按钮（不在本切片）。
- 桌面/iPad 面板的可见关闭按钮：用户 2026-11-03 决定**保留，移到面板外**（D2，见 C5）。
- 已签收的状态/行为：工具层进入即显示、点中间切换、打开主面板不隐藏工具层、朗读条打开时隐藏工具层、`<=750px`
  自适应判定、四个主面板同一时间只开一个、设置面板内部各行（第二轮设置合同）、夜间配色变量（C1–C10、L1–L16、N1–N22）、
  空状态插图与 `v-loading` 遮罩（D3）。

## 3. 上游实测

### 3.1 移动（mini，390×844；360×800 同公式）

| 部件 | 上游实测 |
| --- | --- |
| 顶栏 `.tool-bar` | `fixed` `0,0`、宽 100vw、高 59、z 2001；背景 = `popup`（主题 0 为纸纹图，主题 6 为 `#121212`），无边框无阴影；字体 `Avenir, Helvetica, Arial, sans-serif`。 |
| 顶栏按钮 | 顺序 首页 书架 书源 目录 设置；`flex` `space-around`；每个 58×48 + `padding-top:12`（共 60 高）、`margin-top:-1`；390 宽时 x = 10/88/166/244/322，y = −1。图标是上游 `iconfont.woff` 字形（首页 `&#58920;` 细左尖括号、书架 `&#58892;` 实心书、目录 `&#58905;` 列表、设置 `&#58971;` 齿轮），16px、行高 22；书源是 Element `el-icon-menu`（2×2 方块）18px；文字 12px。日间图标 `#000`、文字 `rgba(0,0,0,.4)`；夜间都是 `#666`。 |
| 悬浮按钮 | 左列 书签/搜索/信息/顶部/底部，x=20；右列 刷新/自动阅读/朗读/日夜，x=334（右 20）；列底距视口底 135；36×36 圆、`margin-top:20`（间距 56）；390×844 左列 y=449…673、右列 y=485…653。背景 = `popupPure`（主题 0 同纸纹，夜间 `#121212`），无边框无阴影；图标 16px，日间 `#121212`，夜间 `#666`；日间月亮 `#121212`、夜间太阳 `#666`。 |
| 底栏 `.read-bar` | `fixed` 底 0、宽 100vw、z 100；背景 `popupPure`，无边框无阴影；高 103 = 页码行 58 + 工具行 45。 |
| 页码行 | `padding:10px 36px`，`el-slider`（runway 6px、按钮 16px 带 2px 边框、日间 runway `#fff`、夜间 `#282828`/`#185798`）占满，右侧 `第 x/y 页` 14px、`margin-left:5`、日间 `rgba(0,0,0,.4)`、夜间 `#666`；音频不显示。 |
| 缓存区 | 点“阅读进度”切换；插在页码行与工具行之间，单行 `padding:10px 36px`、`space-between`、14px、日间 `rgba(0,0,0,.4)`：`缓存章节 后面50章 后面100章 后面全部`（缓存中为进度文字 + 关闭图标）；高 40，底栏变 143；悬浮按钮不移动。 |
| 工具行 | 高 45、`padding:0 15px`、`space-around`；`上一章`（iconfont `&#58920;` + 文字）x=34.33、`阅读进度: 25%`（16px）x=142.98、`下一章`（文字 + `&#58913;`）x=285.67；18px，日间 `#000`、夜间 `#666`；**按钮从不禁用**。 |
| 运行标题 `.top-bar` | `fixed` 高 30、12px、`padding:6px 16px`、z 50、背景继承正文；日间 `rgba(0,0,0,.4)`、夜间 `#666`；工具层显示时被顶栏盖住。 |
| 主面板（书架/书源/目录/设置） | `.popper-component` `fixed` `0,0`、宽 100vw，**z 2101+，盖住整个顶栏**；可见面 = 面板盒，背景 `popup`、无边框无阴影；文字继承 `el-popover`：`#606266`、14px/1.4、`justify`、`break-all`、浏览器默认字体（测得 `Times`）。内容左右/上内边距 20、下 23：书架/目录高 389.19（标题区 26.19 + 20 + 列表 300），书源 391（标题区 28），设置 472.8（列表 45vh）。 |
| 面板打开时的命中 | 顶部中点命中面板标题区（顶栏不可达）；左列悬浮按钮、底栏按钮、正文都可命中：点正文只关闭面板（`eventHandler` 先因面板可见返回，再由 Element 的文档点击关闭），点悬浮/底栏按钮执行该按钮**并**关闭面板。 |

### 3.2 桌面（1440×900；1024×1366 同公式）

`rwc` = `readWidthConfig`：从设置宽度起每次减 20，直到 `<= innerWidth − 140`（1440、1024 下都是 800）。

| 部件 | 上游实测 |
| --- | --- |
| 左栏 `.tool-bar` | `fixed` 顶 0、`left:50%`、`margin-left:-(rwc/2+68)`（1440 下 x=252），宽 60、高 427（只包住 7 个按钮，**不铺满整列**），背景 `popup`。按钮 书架/书源/目录/设置/首页/顶部/底部，58×48 + 12 + 1px 边框（60×62），`margin-top:-1`（步长 61，y=−1…365）；边框日间 `rgba(0,0,0,.1)`、夜间 `#444`；图标/文字同移动。无悬停底色、无“当前面板”高亮。 |
| 右列 `.read-bar` | `fixed` 底 0、`right:50%`、`margin-right:-(rwc/2+52)` → x = 50% + rwc/2 + 8（1440 下 1128），宽 44；背景 `popupPure`。悬浮按钮在列内 `left:4`（x=1132）、列底上方 135，书签/搜索/信息 + 刷新/自动阅读/朗读/日夜，36 圆、`margin-top:20`、右组 `margin-bottom:20`（1440×900 下 y=373…709）。 |
| 进度与章节键 | 42×31 + 12 + 1px 边框（44×45）、`margin-top:-1`（y=767/811/855）：`25%`（16px）、`&#58920;`、`&#58913;`（16px）；不禁用。 |
| 桌面缓存区 | 右列内 `absolute`、`right:55px`、顶与右列顶对齐、宽 300 + `padding:10px 36px`（1440 下 [745,768 372×40]），背景 `popupPure`，单行 14px `rgba(0,0,0,.4)`。 |
| 主面板 | `fixed` 顶 0；可见面 x = 50% − rwc/2（1440 下 320，正文框内沿）、宽 rwc − 1（799）；内边距上 20、左右/下 24（内容 x=344…1095）；书架/目录高 390.19、书源 392、设置 499；背景 `popup`，无边框/阴影/滤镜；标题区与移动相同（各组件自带）。书架单列（`@media (min-width:900px)` 四列规则嵌套错误、从不生效）；目录两列各 50%（文字 `margin-right:26`）。 |
| 面板打开时的命中 | 正文点击只关闭面板；左栏换面板/关面板；右列按钮执行并关闭面板。 |

### 3.3 面板列表（日间；夜间值已由 L/N 切片对齐）

| 列表 | 上游实测 |
| --- | --- |
| 标题区 | `margin-bottom:20`、`space-between`、可换行；标题 18px/25.2、400、`#ed4259`、下划线 1px、通用无衬线栈；书架 `书架(6)`、目录 `目录 (12)`（有空格）、书源 `来源(4)`；按钮 14px/26 `#ed4259`、间距 15，加载中 `#606266`。 |
| 书架 | 单列、`gap:16`；项 `padding:8px 0`、下边框 `#eee`（夜 `#333`）、高 64.98；书名 16px/22.4 继承 `#606266`，当前书**只有书名** `#eb4259`；未读数 12px 继承色；章节 14px `#888`（夜 `#555`）、`margin-top:6`；无悬停变色。打开时把当前书滚到列表**顶部**（`jump`）。 |
| 书源 | 分组 `el-select` mini（140×28），选中项显示 `全部分组 (N)`；项 `padding:8px 0`、高 64.98，来源名继承 `#606266`、当前 `#eb4259`，耗时 12px，最新章节 14px `#888`、`margin-top:6`。打开时当前来源滚到顶部。 |
| 目录 | 行高 40 + 1px 下边框（`#eee`/夜 `#333`），`16px/40px PingFangSC-Regular, HelveticaNeue-Light, 'Helvetica Neue Light', 'Microsoft YaHei', sans-serif`，单行省略；`<=500px` 单列，否则两列 50%；当前章 `#eb4259`；浏览器已缓存章节日间 `#bbb`、夜间 `#444`；无标签、无序号副行、无加粗/底色/悬停。打开时当前章滚到列表**顶部**；顶部/底部按钮同样按元素顶对齐。 |

### 3.4 显示翻页区域

`fixed`（左右滑动时 `absolute`）铺满视口、z 120、底 `#333`、`opacity:.8`、白字 14px；上一页区（上下模式为上半屏，左右滑动为
左半屏且 `padding-right:20%W`）`#43987324`，下一页区（下半屏/右半屏 `padding-left:20%W`）`#6b1a7324`，菜单区（30%–70% 见方，
z 10）`#636060`；文字固定为 `点击前一页`、`点击显示菜单`、`点击后一页`；底部 `关闭` 条 `bottom:20`、高 45、通宽。除关闭条外
`pointer-events:none`：遮罩打开时点击照常翻页/切换菜单。

### 3.5 逻辑

- 阅读进度 = `parseInt((chapterIndex + 1) * 100 / catalog.length) + '%'`（第 3/12 章 = 25%）。
- 页码（非左右滑动）：`totalPages = ceil(正文元素高度 / (innerHeight − scrollOffset))`，`scrollOffset = 2·字号·行高 + 2·字号·段距`；
  滚动时 `currentPage = round((scrollTop + innerHeight) / (innerHeight − scrollOffset))`，滑条把它夹在 `1…totalPages`；拖动只改标签，
  松开 `showPage(page)` 滚到 `(innerHeight − 10)·(page − 1)`（动画 `animateMSTime`）。左右滑动：`ceil(scrollWidth / (innerWidth − 16))`。
- 上一章/下一章：边界提示 `本章是第一章` / `本章是最后一章`，不禁用按钮；不隐藏工具层。
- 首次触摸被上游忽略（`lastMoveY` 初值 `undefined`）是上游缺陷，不复刻。

## 4. 当前 OpenReader 与裁决

| 编号 | 项目 | 当前 | 裁决 |
| --- | --- | --- | --- |
| A1 | 移动顶栏外观 | 96% 透明纯色（无纸纹）、下边框 + 阴影、5 列等分 grid（74.8 宽）、Element Plus 图标（Notebook/Grid/List/Setting/ArrowLeft）、图标与文字同为 `#262626` | **must-fix**：按 3.1 |
| A2 | 移动悬浮按钮 | 38×38、白边框 + 阴影、半透明底、左/右 18、底 134、间距 50；缓存区打开时上移到 202；图标 ArrowUpBold/ArrowDownBold | **must-fix**：按 3.1，缓存区不移动按钮，顶部/底部用 `Top`/`Bottom` |
| A3 | 移动底栏 | 96% 透明（正文透出）、上边框 + 阴影、grid、页码行 20 高原生滑条（日间填充蓝轨道）、工具行 36 高 13px 文字、缓存区两行 | **must-fix**：按 3.1；滑条保留原生 `input[type=range]`（测试钩子与键盘可达），外观画成上游 `el-slider` |
| A4 | 章节键边界 | 第一章/最后一章时禁用（透明度 .45），无提示 | **must-fix**：不禁用，边界提示 |
| A5 | 阅读进度 | `(index + 章内比例) / 总章数` 四舍五入（17%），移动进度合同记为 acceptable-change（非用户要求） | **must-fix**：按 3.5，移动与桌面一致 |
| A6 | 页码 | `ceil(滚动容器总高 / 步长)`、`round(scrollTop / 可滚高度 · (页数−1))`，拖动按比例映射（9 页） | **must-fix**：按 3.5；左右滑动已一致 |
| A7 | 移动运行标题颜色 | `var(--reader-text)` + `opacity:.58` | **must-fix**：日间 `rgba(0,0,0,.4)`、夜间 `#666`；文字内容不变（用户要求） |
| B1 | 移动主面板容器 | 顶 58 起（让出顶栏，顶栏 z 11 在面板 z 10 之上）、97% 透明、文字 `#262626`、系统无衬线字体 | **must-fix**：顶 0 盖住顶栏、内边距 20/20/23、不透明 `popup` 面、`el-popover` 文字默认值 |
| B2 | 移动面板打开时的点击 | 透明关闭层 z 9 盖住悬浮按钮与底栏，点它们只关面板 | **must-fix**：关闭层在悬浮/底栏之下；悬浮/底栏按钮执行并关闭面板；正文只关闭 |
| B3 | 书架列表 | 整卡变红、书名 500 字重 `#262626`、未读数强调色、章节 72% 透明、悬停变红、当前书滚到中间、≥900px 四列 | **must-fix**：按 3.3（含桌面单列） |
| B4 | 目录列表 | 两行（`第 N 章` 副行）、`本地/已缓存` 绿色标签、14px、移动 48 高/桌面 42 高、当前章蓝字浅蓝底加粗、悬停蓝底、标题 `目录(12)`、当前章滚到中间 | **must-fix**（用户 2026-11-03 决定 D1：按上游移除副行与标签）：按 3.3 |
| B5 | 书源列表 | 项 `padding:10px 0`、来源名 `#262626`、分组框 small（24 高）显示占位 `全部分组`、当前来源滚到中间 | **must-fix**：按 3.3；当前来源不可点、切换中状态等换源合同已签收逻辑不重开 |
| B6 | 设置面板容器 | 同 B1；标题 `#262626` | **must-fix**：随 B1，标题继承 `#606266`；各行不重开 |
| C1 | 桌面左栏 | 整列铺满（顶 0 到底）、58 宽、半透明底与模糊、灰黄边框、`rail-item` 56×60、悬停/当前面板高亮、Element Plus 图标、`left:max(8px,…)` | **must-fix**：按 3.2 |
| C2 | 桌面右列 | 圆钮 x=1130、间距 52、底 150、带边框阴影；进度列 x=1130 宽 46、项 44×43、24px 箭头、半透明、边界禁用 | **must-fix**：按 3.2 |
| C3 | 桌面缓存区 | 贴右列底、两段式按钮 | **must-fix**：按 3.2 |
| C4 | 桌面主面板 | x=324 宽 791、内边距 24/60/24/24、左右边框 + 内阴影 + 亮度滤镜、通用标题头（`书架 (6)` 500 字重无下划线）、书架四列、目录 342.5 宽 14px | **must-fix**：按 3.2/3.3 |
| C5 | 桌面关闭按钮 | 面板内右上 44×44（挤占右侧 60px） | **user-decision D2**：保留一个 44×44 关闭按钮，移到面板外、与右列对齐（x = 右列左缘，y = 8），面板内容按上游排版 |
| C6 | 阅读框宽度 | `min(设置宽度, 100vw − 150px)` | **must-fix**：按 `rwc`（每次减 20 直到 `<= innerWidth − 140`），左栏/右列/面板都按 `rwc` 定位 |
| D1 | 显示翻页区域 | 30/40/30 网格、红色虚线框、`点击向上/向下翻页`、红色药丸关闭、遮罩拦截点击 | **must-fix**：按 3.4 |
| E1 | 简洁（Kindle）模式工具色 | 无 | **must-fix**：日间工具边框 `#fefefefe`、文字/页码/缓存区/运行标题 `#444` |

**acceptable-change（技术栈适配）**：Element Plus SVG 图标代替 Element UI 字体图标（同名同形）；`env(safe-area-*)` 写法保留；
按钮用 `<button>`（可键盘访问，样式复位）；保留现有类名作测试钩子。

## 5. 目标行为

- 引入上游 `iconfont.woff`（`frontend/src/assets/fonts/`，`@font-face` 名 `iconfont`），顶栏/左栏/章节键用同一码位。
- 工具层与面板按第 3 节几何、颜色、字体重写样式；夜间沿用已对齐的夜间变量（`--reader-chrome-text #666`、`--reader-chrome-border #444`、
  滑条 `#185798/#282828`、列表变量）。
- 层级（移动）：正文/点击区 < 关闭层 < 悬浮按钮与底栏 < 顶栏 < 主面板；悬浮/底栏动作先关闭主面板再执行。
- 层级（桌面）：保持现有“点击区 < 关闭层 < 面板 < 左栏/右列”，关闭按钮在面板外。
- 书架/目录的标题区移入各自组件（同上游组件自带标题），桌面与移动共用；书源/设置已自带。
- 进度、页码与章节键边界逻辑按 3.5。

## 6. 计划测试（先红）

1. 单元/源码合同 `frontend/tests/readerMenuUpstreamAlignmentContract.test.mjs`：iconfont 码位与顺序；顶栏/悬浮/底栏/左栏/右列关键 CSS
   （58/48/12、36/20/135、45/0 15px、10px 36px、`margin-left:-(rwc/2+68)`、`+8`、无整列铺满、无边框阴影、不透明 `popup` 面）；
   章节键无 `disabled`；缓存区不移动悬浮按钮；移动面板 `top:0` 且层级高于顶栏、关闭层低于悬浮/底栏；桌面面板 `x = 50% − rwc/2`、
   宽 `rwc − 1`、关闭按钮在面板外；目录无 `el-tag`/副行、40px/16px；书架无四列；翻页区域文字与 `pointer-events`。
2. 纯函数合同：`readerChapterProgressLabel(2, 12) === '25%'`、`readerUpstreamPageCount`、`readerUpstreamCurrentPage`、
   `readerUpstreamPageScrollTop`；`readerReadWidthConfig(800, 900) === 760`。
3. 浏览器合同 `scripts/smoke/reader-menu-upstream-contract.mjs`（模拟接口）：390×844、360×800、1440×900、1024×1366 日间与
   390×844、1440×900 夜间，逐项比对第 3 节实测值（±1px）、命中测试（面板盖住顶栏；面板打开时悬浮“日夜”按钮执行并关闭面板；
   点正文只关闭面板且不翻页）、`阅读进度: 25%`、第一章点上一章出现 `本章是第一章`、缓存区单行、翻页区域遮罩下点下半屏照常翻页。
4. 与本合同冲突的既有断言改写（不得放宽无关断言）：`readerMobileProgressContract`、`readerToolOrderContract`、
   `readerMobilePrimaryPopoverContract`、`readerDesktopPrimaryPopoverContract`、`readerIPadWorkspaceContract`、
   `readerNightChromePaletteContract`、`readerNightListTypographyContract`、`readerPagination` 等，以及依赖旧几何的 smoke 步骤。

## 7. 不重开 / 另起

- 键盘：左方向键跳到上一章末尾、边界无提示（`useReaderKeyboard`）——不在菜单范围，另起。
- 提示样式：Reader 自有提示条与上游 `$message` 的差异（全局），本切片边界提示沿用 Reader 提示条。
- 朗读条、书签/搜索/书籍信息对话框、设置面板各行、正文排版与字体。
- 上游首次触摸被忽略的缺陷。

## 8. 实现与回归证据（2026-11-03）

分支 `claude/reader-menu-alignment`：

| 阶段 | 提交 | 内容 |
| --- | --- | --- |
| 合同 | `7a8b073` | 本文件、差异总账与矩阵条目 |
| 红测 | `5c08708` | `readerMenuUpstreamAlignmentContract` 15 项在 `69c27bc` 全部失败；`reader-menu-upstream-contract.mjs` 在 `69c27bc` 构建上记录 693 项检查失败 |
| 实现 | `da438bc` | A1–A7、B1–B6、C1–C6、D1、E1，D1/D2 按用户决定；纯函数实现名为 `readerChapterProgressLabel`、`readerVerticalPageCount`、`readerVerticalCurrentPage`、`readerVerticalPageScrollTop`、`readerReadWidthConfig`；12 个既有单元测试文件里钉住旧布局的断言按上游值改写（步进器与夜间配色期望不变） |
| 类名钩子 | `f1c92da` | 移动主面板保留 `reader-mobile-primary-{shelf,toc,source,settings}`（第 4 节“保留现有类名作测试钩子”） |
| 浏览器合同迁移 | `4be6e20` | 18 个 smoke 跟随上游布局，不放宽无关断言：工具按钮从 `title` 改用 `aria-label`（上游无提示气泡）；移动面板盖住顶栏，改为点面板下方关闭；桌面右列按钮执行并关闭已开面板；页码提示改读 `.progress-tip`；手机低位点击带 88% → 84%（上游 103px 底栏盖住 88%）；目录缓存标记改读 `.log-text.cached`；夜间快照改为上游实测值；书源分组下拉取未选中项（`#ddd`）并核对默认选中的 `全部分组 (N)` 保持 `#409eff`；移动设置外壳的 `overflow-y` 按上游 `overflow-x:hidden` 计算为 `auto`，改为直接断言外壳没有自己的滚动范围；手机上面板盖住顶栏后指针停在分组框上，取色前先移开 |

验证（最终构建，与 `f1c92da` 源码一致）：

- 前端 `npm test` 870/870，`npm run build` 通过；后端未改动，`go test ./...`（31 包）与 `go vet ./...` 通过；未改 db/api/services，race 门不适用。
- 依赖公告门：自测 109/109、前端实时扫描通过；Go 实时扫描失败（见下文“发布阻塞”）。
- 浏览器（模拟接口）：`reader-menu-upstream-contract.mjs` 六视口全部通过（与第 3 节实测值相差 ≤1.5px，亚像素取整）；
  reader-mobile、reader-desktop-panel-controls、reader-source-switch、reader-text-modes、reader-continuous、reader-tts（390/1440/360）、
  reader-settings-position、reader-audio、reader-error-surface、document-title、reader-inline-cache、reader-image、reader-volume、
  reader-deep-page、reader-reauthentication、night-element-theme、remote-reader、reader-progress-multiclient 通过。
- 真实 Go 服务 + 最终构建：reader-epub、reader-cbz 三视口；真实接口合同 bookshelf-refresh-progress、reader-appearance-assets、
  source-parser-workflow、book-deletion-convergence 通过。
- 已知旧失败（在改前构建 `69c27bc` 上以相同断言失败，与本切片无关，另起）：portable-appearance-assets 首页书架封面未使用恢复后的
  封面 URL（1440）；bookinfo-shelf-mutations 书籍信息“追更”开关 15 秒内未发出 `PUT /api/books/:id`。

发布阻塞：Go 公告门实时扫描失败——GO-2026-5932（`golang.org/x/crypto`）在漏洞库里新增修复版本
`v0.58.1-0.20261009181718-2f1f834330b2`（伪版本，无 tag）。门要求该已知公告“无修复版本”，且升级只接受带 tag 的版本，因此失败关闭。
与本切片无关，需用户决定处理方式；在此之前不合入 `main`（合入即触发发布）。

状态：**implementation-complete / regression-validated**；发布阻塞于 Go 公告门。
