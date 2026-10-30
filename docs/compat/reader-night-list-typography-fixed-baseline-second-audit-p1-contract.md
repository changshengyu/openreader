# Reader 夜间目录/书架/换源列表文字合同（P1）

状态：**implementation-complete / regression-validated**（记录见第 8 节；用户 2026-10-30 确认第 5 节：列表文字全部改回上游，字体预览框夜间用控件夜间面）。审查日期：2026-10-30。审查基线：`OpenReader@ec3736f`。

## 1. 来源与范围

[`reader-night-chrome-palette-fixed-baseline-second-audit-p1-contract.md`](reader-night-chrome-palette-fixed-baseline-second-audit-p1-contract.md)
（`24511c0` 已发布）把夜间工具按钮、设置控件、强调色与页码滑块改回上游，第 4 节“不重开”把目录/书架/换源面板的列表文字与分隔线
留到另起切片：那次只让这些面板的强调色随 C1 变为 `#ed4259`。本切片就是那一项。

范围：Reader 内三块列表面板（桌面工作区与移动主面板共用的 `ReaderTocPanel.vue`、`ReaderShelfPanel.vue`、`SourceSwitchPanel.vue`）的
列表文字、副行、已缓存章节、当前项、悬停、分隔线，以及这三块面板标题栏按钮的加载/不可用色（`ReaderDesktopWorkspacePanel.vue`、
`SourceSwitchPanel.vue` 标题区、`Reader.vue` 移动标题区）。另：设置面板的字体预览框（OpenReader 新增）按用户决定处理（第 5 节 D3）。
阅读面、正文色、工具栏、设置控件、日间配色都不在本切片内。

## 2. 固定上游

`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。三个组件都在 `.data-wrapper` 上按
`$store.getters.isNight`（`config.themeType === "night"`，`plugins/vuex.js:457`）加 `.night`/`.day`，与内置主题还是自定义方案无关。
三个面板都放在 `views/Reader.vue:9–43` 的 `el-popover`（`popper-class="popper-component"`，挂到 `body`）里；没有自己颜色的文字继承
Element UI 2.15 `.el-popover` 的 `#606266`。`App.vue` 的 `.night-theme`（939–1141）不覆盖 `.el-popover`，`.popper-component.el-popover`
（1156）只去掉边框与阴影；面板底色是 `currentThemeConfig.popup`（内置夜间 `#121212`）。

`web/src/components/PopCatalog.vue`（目录）：

- `.title`（322–329）`#ed4259` + `1px solid #ed4259` 下划线；`.title-btn`（331–354）本身 `#606266`，`.span-btn`/`.refresh-btn` `#ed4259`，
  刷新中 `.refresh-btn.loading` `#606266`。
- `.log`（374–387）：桌面两列、`16px / 40px`；`max-width: 500px`（411–418）一列。章节名没有自己的颜色，继承 `#606266`。
- `.cached`（366–368）`#444`（日夜共用）；`.day >>>.cached`（406–408）`#bbb !important`，所以**夜间已缓存章节是 `#444`**、日间 `#bbb`。
- `.selected .log-text`（370–372）`#EB4259 !important`，没有底色；没有悬停样式。
- `.night >>>.log`（395–399）`border-bottom: 1px solid #333`；`.day >>>.log`（401–404）`#eee`。

`web/src/components/BookShelf.vue`（书架）：

- `.title`（186–193）同目录；`.title-btn`（195–207）`#ed4259`，刷新中 `.loading` `#606266`。
- `.book-name`（234–239）16px、`.book-progress`（未读章数，240–243）12px，都没有自己的颜色，继承 `.book-title`；
  `.chapter-text`（246–253）`#888`、14px；`.selected .book-name`（255–259）`#EB4259`（未读数不变色）。
- `.night`（279–293）：`.book-item` `border-bottom: 1px solid #333`，`.book-title` `#888`（书名与未读数），`.chapter-text` `#555`。
  `.day >>>.book-item`（295–299）`#eee`。日间书名/未读数继承 `#606266`。

`web/src/components/BookSource.vue`（换源）：

- `.title`（364–371）同目录；`.title-btn`（373–392）`#ed4259`，`.source-count` `#606266`，加载中 `.loading` `#606266`；分组下拉框
  `el-select size="mini"`（夜间由 `App.vue .night-theme .el-input__inner` 改为 `#444` 底、`#ddd` 字）。
- `.source-name`（415–421）16px、`.source-time`（422–425）12px，没有自己的颜色，继承 `#606266`；`.source-latest-chapter`（428–435）
  `#888`、14px（日夜共用）；`.selected .source-name`（437–441）`#EB4259`。
- `.night >>>.source-item`（450–454）`border-bottom: 1px solid #333`；`.day`（456–460）`#eee`。没有悬停样式。

## 3. 当前 OpenReader（`ec3736f`）

三块面板的容器（`ReaderDesktopWorkspacePanel.vue:72`、`Reader.vue:2796` 移动主面板）文字色都是
`var(--reader-popup-text, var(--reader-text))`。`--reader-popup-text` 对浮窗色做 4.5:1 替换：内置夜间 `#666` 对 `#121212` 只有 3.26:1，
被换成 `#ffffff`，所以**内置夜间的三块列表全是白字**。`html.dark-reader`（`styles/tokens.css:36`）只改 `--app-*`，不改 Element Plus 变量。

| 位置 | 元素 | 当前（日夜相同，除非注明） |
| --- | --- | --- |
| `ReaderTocPanel.vue:159–174` | 章节行 `.toc-item` | 文字继承容器（夜间白色）；分隔 `1px solid #f0f0f0` |
| `ReaderTocPanel.vue:17` | 已缓存章节 | 章节名不变色；另显示绿色 `el-tag`（`本地`/`已缓存`） |
| `ReaderTocPanel.vue:181–184` | 桌面两列当前章 | `--reader-accent`（`#ed4259`），透明底 |
| `ReaderTocPanel.vue:210–214` | 移动一列当前章 | `#409eff`、600、`#ecf5ff` 浅蓝底 |
| `ReaderTocPanel.vue:205–208` | 悬停 | `#409eff` + `#f5f7fa` 浅灰底 |
| `ReaderTocPanel.vue:199–203` | 移动“第 N 章”小字 | `--app-text-muted`（日 `#766a5c`、夜 `#a8a093`） |
| `ReaderShelfPanel.vue:110–129` | 书架卡片 | 文字 `--reader-popup-text`（夜间白色）；分隔 `rgba(160, 139, 91, 0.22)`；当前书/悬停整卡 `--reader-accent` |
| `ReaderShelfPanel.vue:151–156` | 未读章数 `em` | `--reader-accent` |
| `ReaderShelfPanel.vue:158–161` | 最近章节 | `--reader-popup-text` 72%（夜间白色 72%） |
| `SourceSwitchPanel.vue:231–248` | 来源行 | 分隔 `#eee`；悬停 `rgba(237, 66, 89, 0.05)` 底 |
| `SourceSwitchPanel.vue:263–276` | 书源名 / 耗时 | `--reader-popup-text` / 72%（夜间白色 / 72%） |
| `SourceSwitchPanel.vue:278–290` | 最新章节 / “切换中...” | `--reader-popup-text` 72% |
| `SourceSwitchPanel.vue:292–294` | 当前来源名 | `--reader-accent` |
| `SourceSwitchPanel.vue:212–215` | 标题区按钮不可用 | `--reader-popup-text` 62%（夜间白色 62%） |
| `ReaderDesktopWorkspacePanel.vue:143–146` | 桌面目录/书架标题按钮不可用 | `#aaa` |
| `Reader.vue:2860` | 移动目录/书架标题按钮不可用 | `#606266`（已与上游一致） |
| `ReaderSettingsPanel.vue:1096–1104` | 设置字体预览框（OpenReader 新增） | `#2c2a24` 字、`#fffaf0` 底、`#eadfca` 边（夜间仍是浅色块） |

## 4. 差异与裁决

按上游，以下都只看 `themeType === 'night'`（= 上游 `isNight`），日间值逐项保持现状。对比度以内置夜间面板 `#121212` 计。

| 编号 | 项目 | 上游夜间 | 当前夜间 | 裁决 |
| --- | --- | --- | --- | --- |
| L1 | 目录章节名（未缓存） | `#606266`（继承，3.1:1） | 白色（18.7:1） | **must-fix**（D1） |
| L2 | 目录已缓存章节名（上游只认浏览器缓存 `cachedCataMap`） | `#444`（1.9:1） | 白色，另有绿色 `el-tag` | **must-fix**（D1）：浏览器已缓存的章节名 `#444`；`el-tag` 是 OpenReader 新增标记，保留不动（实现中修订判定，见第 8 节） |
| L3 | 目录分隔线 | `#333` | `#f0f0f0` | **must-fix** |
| L4 | 目录当前章（移动一列） | `#EB4259`，无底色 | `#409eff` + `#ecf5ff` 浅蓝块 | **must-fix**：夜间强调色 + 透明底（桌面两列已是如此） |
| L5 | 目录悬停（上游没有） | — | `#409eff` + `#f5f7fa` 浅灰块 | **acceptable-change**：保留悬停状态，夜间与 L4 同色、透明底，不再出现浅色块 |
| L6 | 目录移动“第 N 章”小字（上游没有） | — | `#a8a093`（比 L1 后的章节名更亮） | **acceptable-change**：夜间与书架副行同一变量 `#555`，保持“标题亮于副行”，不新增颜色 |
| L7 | 书架分隔线 | `#333` | `rgba(160, 139, 91, 0.22)` | **must-fix** |
| L8 | 书架书名与未读章数 | `#888`（5.3:1；选中只改书名） | 书名白色；未读数 `#ed4259` | **must-fix**（D1）：书名与未读数 `#888`；当前书/悬停的书名仍为强调色 |
| L9 | 书架最近章节 | `#555`（2.5:1） | 白色 72% | **must-fix**（D1） |
| L10 | 换源分隔线 | `#333` | `#eee` | **must-fix** |
| L11 | 换源书源名、耗时 | `#606266`（继承） | 白色 / 白色 72% | **must-fix**（D1） |
| L12 | 换源最新章节 | `#888`（日夜共用） | 白色 72% | **must-fix**（D1）；“切换中...”（OpenReader 新增）用同一变量 |
| L13 | 三块面板标题按钮的加载/不可用色 | `#606266`（`.loading`） | 桌面 `#aaa`；换源白色 62%；移动已是 `#606266` | **must-fix** |
| L14 | 选中项的红色 | `#EB4259`（标题与按钮用 `#ed4259`） | `--reader-accent` `#ed4259` | **acceptable-change**：上游同一组件混用两值，红通道只差 2/255，统一用强调色（日夜相同） |
| L15 | 标题、下划线、标题按钮 | `#ed4259` | `#ed4259`（夜间配色切片 C1） | 已一致 |
| L16 | 设置字体预览框（OpenReader 新增） | 无对应元素 | 夜间仍是 `#fffaf0` 浅色块 | 第 5 节 D3 |

**不重开（另起切片）**：

- 面板里的 Element Plus 组件夜间外观：换源分组下拉框（上游 `.night-theme` `#444`/`#ddd`）、目录 `el-tag`、`el-empty` 空状态、书架
  `v-loading` 遮罩。属于 `App.vue` `.night-theme` 全局 Element 覆盖，涉及首页工作台与所有弹窗。
- 日间配色：目录分隔 `#f0f0f0`（上游 `#eee`）、日间已缓存不变灰（上游 `#bbb`）、移动当前章与悬停的蓝色、书架日间书名/副行/分隔
  （上游 `#606266`/`#888`/`#eee`）、换源日间副行 72%（上游 `#888`）、日间不可用 `#aaa`/62%、日间未读数强调色。
- 日夜共用的结构与字号：目录 14px 行（上游 `16px / 40px`）、移动目录的“第 N 章”行与 `el-tag`、移动当前章加粗、桌面工作区标题 500 字重
  且无下划线（上游 400 + 下划线）、书架四列布局。
- `--reader-popup-text`/`--reader-text` 的计算与阅读面；自定义方案的颜色、背景图与 4.5:1 正文保护；朗读条。

## 5. 用户决定（2026-10-30 已确认）

用户在问答中选择：

- **D1（取代白字）= 全部改回上游**：L1–L4、L7–L13 采用上游值：目录 `#606266`、已缓存 `#444`，书架书名与未读数 `#888`、副行 `#555`，
  换源书源名与耗时 `#606266`、最新章节 `#888`，分隔线 `#333`，不可用按钮 `#606266`，目录当前章去掉浅蓝块（对 `#121212` 约 1.9–5.3:1，
  现在的白字 18.7:1）。未采用的备选：只改分隔线、去掉浅色块，文字保留白色。
- **D2（作用范围）**：沿用夜间配色切片的 D2，与上游 `isNight` 一样对所有夜间方案生效（内置夜间、隐藏 `black`、自定义夜间）；
  自定义方案自己的颜色、背景图与正文对比度保护不变。
- **D3（字体预览框）= 夜间用控件夜间面**：夜间 `rgba(45, 45, 45, 0.5)` 底、`#666` 边、`#606266` 字（上游 `ReadSettings.vue`
  `.night .span-item` 与 `.el-popover` 继承色），日间 `#fffaf0`/`#eadfca`/`#2c2a24` 不变，不新增颜色。未采用的备选：保持浅色块；删除预览框。

这取代 [`reader-night-contrast-p0-contract.md`](reader-night-contrast-p0-contract.md) 第一轮“实施结果”中弹层文字按 4.5:1 替换为浅色在
目录/书架/换源列表上的效果；`--reader-popup-text` 本身的计算不变（第 6 节第 4 条）。

## 6. 目标行为

1. `Reader.vue`：在 `NIGHT_CHROME_STYLE` 旁新增只在夜间展开的 `NIGHT_LIST_STYLE`（日间不设置，组件以现有表达式为 `var()` 回退，
   日间渲染不变）：

   | 变量 | 夜间值 | 上游来源 | 使用者 |
   | --- | --- | --- | --- |
   | `--reader-list-text` | `#606266` | `.el-popover` 继承色、`.loading` | 目录章节名、换源书源名与耗时、三块面板标题按钮不可用色 |
   | `--reader-list-cached-text` | `#444` | `PopCatalog .cached` | 目录已缓存章节名 |
   | `--reader-list-border` | `#333` | `.night .log` / `.book-item` / `.source-item` | 三块列表的分隔线 |
   | `--reader-list-active-text` | `#ed4259` | `.selected .log-text` | 目录当前章与悬停（移动一列） |
   | `--reader-list-active-bg` | `transparent` | 上游无底色 | 目录当前章与悬停底色 |
   | `--reader-shelf-title-text` | `#888` | `BookShelf .night .book-title` | 书架书名与未读章数 |
   | `--reader-shelf-chapter-text` | `#555` | `BookShelf .night .chapter-text` | 书架最近章节、目录移动“第 N 章” |
   | `--reader-source-chapter-text` | `#888` | `BookSource .source-latest-chapter` | 换源最新章节、“切换中...” |

2. `NIGHT_CHROME_STYLE`（设置面板的夜间变量组）按 D3 增加两项：`--reader-font-preview-bg` `rgba(45, 45, 45, 0.5)`、
   `--reader-font-preview-border` `#666`；`.font-preview` 的文字改用已有的 `--reader-settings-text`（夜间 `#606266`，回退 `#2c2a24`）。
3. 组件只把颜色改为“新变量，回退到现有表达式”，不改尺寸、布局与交互：

   | 组件规则 | 新取色（回退 = 现在的日间表达式） |
   | --- | --- |
   | `.toc-item` | 文字 `--reader-list-text`（回退 `--reader-popup-text`，与现在继承的容器色相同）；分隔 `--reader-list-border`（回退 `#f0f0f0`） |
   | `.toc-item.cached`（新类） | `--reader-list-cached-text`，回退到上一行；规则排在悬停与当前章之前，当前章/悬停仍覆盖它 |
   | `.toc-item:hover` / `.toc-item.active` | 文字 `--reader-list-active-text`（回退 `#409eff`）；底 `--reader-list-active-bg`（回退 `#f5f7fa` / `#ecf5ff`） |
   | `.toc-item small` | `--reader-shelf-chapter-text`（回退 `--app-text-muted, #909399`） |
   | `.reader-shelf-card` | 文字 `--reader-shelf-title-text`（回退 `--reader-popup-text`）；分隔 `--reader-list-border`（回退 `rgba(160, 139, 91, 0.22)`）；当前书/悬停整卡强调色不变 |
   | `.reader-shelf-title-line em` | `--reader-shelf-title-text`（回退 `--reader-accent`） |
   | `.reader-shelf-chapter` | `--reader-shelf-chapter-text`（回退白色 72% 的现有表达式） |
   | `.source-item` | 分隔 `--reader-list-border`（回退 `#eee`） |
   | `.source-name` / `.source-time` | `--reader-list-text`（回退 `--reader-popup-text` / 72%） |
   | `.source-latest-chapter` / `.source-status` | `--reader-source-chapter-text`（回退 72%） |
   | `.title-actions button:disabled`（换源） | `--reader-list-text`（回退 62%） |
   | `.reader-workspace-actions :deep(button:disabled)`（桌面目录/书架） | `--reader-list-text`（回退 `#aaa`） |

   `cached` 类只看浏览器缓存 `browserCachedMap`（`isBrowserCached`），对应上游 `cachedCataMap`：上游每次成功取到正文都经
   `cacheFirstRequest` 写入浏览器缓存，目录只按这些键变暗；服务端 `cachePath`（导入的本地书每章都有）不算。`el-tag` 仍用 `isCached`
   （含服务端 `cachePath`），不调整。（实现中修订：原文写的是沿用 `isCached`，见第 8 节。）
   唯一的 DOM 变化是浏览器已缓存章节行的 `cached` 类。移动标题区不可用色已是 `#606266`，不改。
4. `--reader-popup-text`、`--reader-text` 与阅读面计算不变。

## 7. 计划测试（先红）

`frontend/tests`（node --test）：

1. 新增 `readerNightListTypographyContract.test.mjs`：`NIGHT_LIST_STYLE` 的变量与上游值完全一致，且只在 `reader.themeType === 'night'`
   时展开；三块面板、两个标题区与字体预览框按第 6 节取色，每个回退值等于现在的日间表达式；目录浏览器已缓存行带 `cached` 类
   （只看 `browserCachedMap`），且 `.toc-item.cached` 排在悬停与当前章之前。
2. `readerNightChromePaletteContract.test.mjs` 的变量组断言按 D3 加入两项字体预览变量（扩充，不删改已有项）。
3. 对照：夜间配色切片其余断言、`--reader-popup-text` 计算、自定义方案对比度保护保持通过。

浏览器（`scripts/smoke/reader-mobile-contract.mjs`，模拟接口新增第三、四章及其正文——第二、三章由预加载写入浏览器缓存，第四章只有
服务端 `cachePath`、必须保持未缓存色——以及两个换源候选）：内置夜间段在
1440×900、390×844、360×800 与 iPad 1024×1366 / 1366×1024（自适应）打开书架、换源、目录，读取 computed style：

| 元素 | 夜间期望 |
| --- | --- |
| 目录未缓存章节（含只有服务端 `cachePath` 的） / 浏览器已缓存章节 / 当前章 | `rgb(96, 98, 102)` / `rgb(68, 68, 68)` / `rgb(237, 66, 89)` + `rgba(0, 0, 0, 0)` 底；分隔 `rgb(51, 51, 51)` |
| 移动目录“第 N 章” | `rgb(85, 85, 85)` |
| 书架书名 / 未读数 / 最近章节 / 当前书书名 | `rgb(136, 136, 136)` / `rgb(136, 136, 136)` / `rgb(85, 85, 85)` / `rgb(237, 66, 89)`；分隔 `rgb(51, 51, 51)` |
| 换源书源名 / 耗时 / 最新章节 / 当前来源名 | `rgb(96, 98, 102)` / `rgb(96, 98, 102)` / `rgb(136, 136, 136)` / `rgb(237, 66, 89)`；分隔 `rgb(51, 51, 51)` |
| 设置字体预览框 | 字 `rgb(96, 98, 102)`、边 `rgb(102, 102, 102)`、底 `rgba(45, 45, 45, 0.5)` |

日间在切换前后各读一次同一批元素（含字体预览框），要求逐项相同，且字体预览框为 `rgb(44, 42, 36)`/`rgb(234, 223, 202)`/
`rgb(255, 250, 240)`。另跑真实 Go 服务 + 新构建、新数据目录的账号对照（新构建与 `ec3736f` 构建），确认日间 computed style 相同。

## 8. 记录（2026-10-30）

提交（分支 `claude/reader-night-list-typography`）：合同 `465ad11`；红测 `f54fcbc`；实现 `849c33d`；浏览器夹具修正 `086e987`；
L2 修订红测 `4ee2515`、修订实现 `c576933`。

- 红测：`f54fcbc` 在 `ec3736f` 上 7 项断言失败（6 项新测：夜间列表变量组、目录、书架、换源、桌面标题按钮、字体预览；以及按 D3
  扩充的夜间配色变量组），日间回退对照与夜间配色其余 6 项通过；`849c33d` 后同一批断言全部转绿。浏览器合同在 `ec3736f` 构建上于
  `1440: night lists shelfCard color expected rgb(136, 136, 136), got rgb(255, 255, 255)` 失败。
- 夹具修正（`086e987`）：载入章节会写入浏览器缓存，阅读器又预加载当前章前后两章，三章夹具里所有非当前行都变成已缓存
  （`849c33d` 构建报 `1440: night lists tocItem is missing`）；改为四章，第四章在预加载半径之外。
- 实现中修订 L2（`4ee2515` 红 → `c576933`）：真实后端对照时发现导入的本地书每章都写了服务端 `cachePath`
  （`backend/services/localbook/durable_import.go:251`），沿用 `isCached` 会把整本本地书目录在夜间染成 `#444`。上游 `PopCatalog`
  的 `cachedCataMap` 只读浏览器缓存（`localCache@…@chapterContent-<index>`，`App.vue#getBookContent` 经 `plugins/helper.js#cacheFirstRequest`
  每次成功取正文都写入）。现在 `cached` 类只看 `browserCachedMap`（`isBrowserCached`），`el-tag` 仍用 `isCached`。修订红测在
  `849c33d` 上 1 项单元断言（catalog rows）与浏览器断言 `1440: night lists tocItem color expected rgb(96, 98, 102), got rgb(68, 68, 68)`
  （只有服务端 `cachePath` 的第四章被染暗）失败，`c576933` 后通过。这比原写法更贴近上游，仍在 D1“全部改回上游”之内。
- 测试：frontend 843/843 与 build；backend `go test ./...` 30 个包与 `go vet ./...`（后端未改动；race 门跳过：没有触及
  db/api/services）；advisory 门自测 109/109，Go 与前端公告扫描 exit 0。
- 真实浏览器（本地 Go 服务 + 新构建与 `ec3736f` 构建各一份，新数据目录）：
  - `reader-mobile-contract.mjs`（模拟接口）全量通过：1440×900、390×844、360×800、自定义纯黑夜间、iPad 1024×1366 / 1366×1024
    （自适应）与强制手机模式；内置夜间三块列表与字体预览逐项为第 7 节的值，切回日间后与切换前逐项相同。
  - `reader-source-switch-contract.mjs`、`bookshelf-refresh-progress-contract.mjs`、`document-title-contract.mjs`（三视口）、
    `reader-settings-position-contract.mjs`、`reader-epub-contract.mjs`（三视口）通过。`reader-desktop-panel-controls-contract.mjs`
    108 组中 105 组通过，失败的 3 组都是 C1 flip 开头等待 `.reader-chapter-end` 的 10 秒 harness 超时（还没打开面板）；只跑 C1 flip
    （12 组）各两次：新构建 12/12、11/12，`ec3736f` 构建 12/12、10/12，与本切片无关，未修改该脚本。
  - 真实账号 + 两本导入的 TXT（各 12 章），两份构建对照五个视口（1440×900、390×844、360×800、iPad 1024×1366 / 1366×1024）：
    日间三块列表与字体预览的 computed style 与 `ec3736f` **完全相同**（唯一 DOM 差异是新构建当前章与预加载两章上的 `cached` 类）；
    夜间为目录 `rgb(96, 98, 102)`、浏览器已缓存 `rgb(68, 68, 68)`（带服务端“已缓存”标签的第 4–12 章保持 `rgb(96, 98, 102)`）、
    当前章 `rgb(237, 66, 89)` 透明底、移动“第 N 章” `rgb(85, 85, 85)`、分隔 `rgb(51, 51, 51)`，书架书名与未读数 `rgb(136, 136, 136)`、
    最近章节 `rgb(85, 85, 85)`、当前书 `rgb(237, 66, 89)`，换源分隔 `rgb(51, 51, 51)`，字体预览 `rgb(96, 98, 102)` / `rgb(102, 102, 102)` /
    `rgba(45, 45, 45, 0.5)`。旧构建夜间为白字（`rgb(255, 255, 255)`，书架副行 72%）、未读数 `rgb(237, 66, 89)`、分隔
    `rgb(240, 240, 240)` / `rgba(160, 139, 91, 0.22)` / `rgb(238, 238, 238)`、移动当前章 `rgb(64, 158, 255)` + `rgb(236, 245, 255)`、
    字体预览浅色块。两份构建都没有控制台错误、页面错误或 5xx。
- 观察到但未改动：目录 `el-tag` 在夜间仍是 Element Plus 的白底标签（全局 Element 夜间覆盖，第 4 节不重开）；本地书的换源面板只有
  当前来源一行。

状态：**implementation-complete / regression-validated**，待可信 Actions；镜像未发布；生产未访问。
