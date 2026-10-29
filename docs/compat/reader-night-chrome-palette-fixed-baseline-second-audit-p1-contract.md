# Reader 夜间工具栏与设置控件配色合同（P1）

状态：**implementation-complete / regression-validated**（记录见第 8 节；用户 2026-10-29 确认第 5 节：全部改回上游）。审查日期：2026-10-29。审查基线：`OpenReader@c62c619`。

## 1. 来源与范围

[`reader-day-night-device-scope-and-palette-fixed-baseline-second-audit-p1-contract.md`](reader-day-night-device-scope-and-palette-fixed-baseline-second-audit-p1-contract.md)
（`b3164e3` 已发布）把内置夜间的阅读面改回上游（`body_6`/`content_6` 纹理、`#121212` 浮窗、`#666` 正文、`#444` 页边框），
但第 4.2 节与第 7 节明确把 Reader 工具栏/设置面板的夜间控件色（`#303030`、`#565656`、`#ff7589`、`#7ed8d4`）留到另起切片：
那次用户只确认了阅读背景与正文色。本切片就是那一项。

这些控件色来自 [`reader-night-contrast-p0-contract.md`](reader-night-contrast-p0-contract.md) 第一轮“实施结果”的可读性增强
（夜间强调色 `#ff7589`，控件面 `#303030`，弹层文字按 4.5:1 替换）。改回上游会**取代**那一部分，因此需要用户确认（第 5 节）。

范围：Reader 内的工具按钮（桌面左栏、右侧圆钮、进度/章节按钮、缓存条、移动顶栏/浮动按钮/底栏）、设置面板控件与文字、
强调色（所有 `--reader-accent` 消费者）、移动页码滑块。阅读面、正文色、自定义方案的页面/正文颜色与背景图、日间配色都不在本切片内。

## 2. 固定上游

`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。上游所有夜间控件样式都只看 `isNight`
（`config.themeType === "night"`），与使用内置主题 6 还是自定义方案无关。

`web/src/views/Reader.vue`：

- `.night`（3494–3555）：`.tool-icon` `border: 1px solid #444`、`margin-top: -1px`、`color: #666`，`.icon-text` `#666`；
  `.progress-tip`、`.cache-content-zone`、`.float-left-btn-zone`、`.float-right-btn-zone`、`.reader-bar-inner`、`.top-bar`/`.bottom-bar`
  均为 `#666`；`.chapter` 边框 `#444`（日夜切片 P5 已对齐）；`.el-slider__runway` `#282828`、`.el-slider__bar` `#185798`、
  `.el-slider__button` `2px solid #185798` + `#282828`；`.play-pause-btn` `#185798`；`.popper__arrow` `#666`。
- `.tool-bar .tools .tool-icon`（3054–3100）与 `.read-bar .tools .tool-icon`（3203–3229）：58×48 / 42×31 的格子，本身无边框色，
  颜色全部来自上面的 `.day`/`.night`。
- `.float-btn`（3150、3176）：36px 圆，无边框、无阴影；背景 `popupAbsoluteBtnStyle`（775）= `currentThemeConfig.popupPure`
  （内置夜间 `#121212`）；夜间显示 `.el-icon-sunny`，颜色 `#666`（3195）。
- `leftBarTheme`（636）/`rightBarTheme`（648）：工具栏背景为不透明的 `popup` / `popupPure`。
- `.chapter-wrapper.mini-interface`（3581–）：移动顶栏与底栏的 `.tool-icon` 一律 `border: none`。
- 上游**没有**自动阅读/朗读按钮的“开启”高亮，也没有工具按钮的悬停色。

`web/src/components/ReadSettings.vue`（根节点 `:class="{ night: isNight, day: !isNight }"`）：

- `.night`（1135–1161）：`.theme-item` `1px solid #666`；`.selected` `1px solid #666`；`.moon-icon` `#ed4259`；
  `.span-item` `1px solid #666` + `rgba(45, 45, 45, 0.5)`；`.resize`（数值步进）`1px solid #666` + `rgba(45, 45, 45, 0.5)`，
  分隔 `b` 为 `border-right: 1px solid #666`。
- 日夜共用：`.span-item.selected`（1002）`1px solid #ed4259` + `#ed4259`（选择器权重高于 `.night .span-item`，夜间选中仍为 `#ed4259`）；
  `.selected`（1070）`#ed4259`（主题色块对勾）；`.title-btn`、`.span-btn`、`.upload-bg-btn`、删除图标、上传字体 `.active` 为 `#ed4259`；
  `.setting-item-title`（942）`#666`；`.upload-font-icon`（986）`#606266`；`.lang`（1101）与 `.setting-input .el-input__inner`
  （1205）数值 `#a6a6a6`；`@media (hover: hover)`（1192）悬停 `#ed4259`。
- 其它文字（标题“设置”、选项文字、`.small-tip` 提示、自定义区标题、步进加减号）没有自己的颜色，继承 Element UI 2.15
  `.el-popover` 的 `#606266`；`App.vue` 的 `.night-theme` 不覆盖 `.el-popover`，`.popper-component.el-popover` 只去掉边框与阴影。

`web/src/App.vue` `.night-theme`（939–1146）：全局 Element 夜间覆盖（对话框 `#222`、消息框、按钮、`.el-button--primary` `#185798`、
输入框 `#444`、表格、分页、下拉）。

## 3. 当前 OpenReader（`c62c619`）

`frontend/src/views/Reader.vue#readerStyle`（1291–1294）只按 `reader.themeType === 'night'`：

| 变量 | 夜间 | 日间 |
| --- | --- | --- |
| `--reader-control-bg` | `#303030` | `rgba(255, 255, 255, 0.5)` |
| `--reader-control-border` | `#565656` | `rgba(0, 0, 0, 0.1)` |
| `--reader-accent` | `#ff7589` | `#ed4259` |
| `--reader-active-text` | `#7ed8d4` | `#0f5451` |

`--reader-popup-text` = `resolveReaderTextColor({ requestedColor: 有效正文色, backgroundColor: 浮窗色, themeType })`（不传 `theme`）：
内置夜间 `#666` 对 `#121212` 只有 3.26:1，被替换成 `#ffffff`。所以内置夜间里**正文是 `#666`，工具按钮与设置文字是纯白**。

工具与控件（夜间实际值）：

- `ReaderDesktopTools.vue`：左栏 `.rail-item` 文字/图标 = 白色 72%（`.rail-home` 100%），格线 `rgba(148, 132, 87, 0.35)`，
  栏左右线 `rgba(148, 132, 87, 0.26/0.38)`；右侧 `.round-tool` 白色图标、`1px solid rgba(255, 255, 255, 0.7)` 白圈；
  悬停/开启 `--reader-active-text`。
- `ReaderDesktopProgress.vue` / `ReaderCachePanel.vue`：白色文字，线 `rgba(148, 132, 87, 0.38/0.32)`。
- `ReaderMobileChrome.vue`：顶栏按钮、浮动按钮、上一章/下一章/阅读进度为白色；浮动按钮 `rgba(255, 255, 255, 0.72)` 白圈；
  页码滑块提示白色 76%；原生 range `accent-color: #409eff`（日夜相同）。
- `ReaderSettingsPanel.vue` / `ReaderSettingStepper.vue`：选项按钮、字体选项、字号预设、步进器 `#303030` + `#565656`；
  文字继承白色，标签白色，提示/上传字体图标白色 72%，步进器禁用白色 48%；主题色块边框 `rgba(0, 0, 0, 0.1)`、选中边框与对勾
  `#ff7589`、月亮只在选中时 `#ff7589`（否则 `rgba(255, 255, 255, 0.2)`）。
- `#ff7589` 同时出现在：设置标题按钮/底部操作/上传/删除、桌面与移动工作区标题/操作/关闭、目录/书架/换源的选中项、左栏当前面板、
  章末按钮焦点框。

## 4. 差异与裁决

按上游，以下都只看 `themeType === 'night'`（= 上游 `isNight`），日间值逐项保持现状。

| 编号 | 项目 | 上游夜间 | 当前夜间 | 裁决 |
| --- | --- | --- | --- | --- |
| C1 | 强调色（所有 `--reader-accent` 消费者） | `#ed4259`（日夜相同） | `#ff7589` | **must-fix** |
| C2 | 设置选项按钮（`.selection-button`、`.font-family-option`；上游 `.span-item`） | 边框 `#666`，底 `rgba(45, 45, 45, 0.5)`；选中/悬停 `#ed4259` | `#565656` / `#303030`；选中 `#ff7589` | **must-fix**（经 `--reader-control-*`） |
| C3 | 数值步进器（上游 `.resize`） | 边框与分隔 `#666`，底 `rgba(45, 45, 45, 0.5)` | `#565656` / `#303030` | **must-fix**；减号/数值/加号的形态是已签收的 `intentional-redesign`，不变 |
| C4 | 主题色块（`.theme-item`） | 边框 `#666`，选中仍 `#666`；对勾 `#ed4259`；月亮恒为 `#ed4259` | 边框 `rgba(0, 0, 0, 0.1)`，选中 `#ff7589`；月亮仅选中时有色 | **must-fix** |
| C5 | 设置面板文字 | 标签 `#666`；标题、选项、提示、自定义区标题、加减号、上传字体图标 `#606266`；数值 `#a6a6a6` | 全部白色（提示/图标 72%） | **must-fix**（第 5 节决定 D1） |
| C6 | 桌面左栏（上游 `.tool-bar .tool-icon`） | 图标与文字 `#666`；格线 `#444` | 白色 72%/100%；棕色半透明格线 | **must-fix**：文字 `#666`，格线与栏左右线 `#444` |
| C7 | 桌面右侧圆钮、移动浮动按钮（上游 `.float-btn`） | 图标 `#666`；无边框 | 白色图标；白圈 0.7/0.72 | **must-fix**：图标 `#666`，夜间边框透明（保留 36/38px 盒子） |
| C8 | 桌面进度/章节按钮与缓存条（上游 `.read-bar .tools .tool-icon`、`.cache-content-zone`） | `#666`；格线 `#444` | 白色；棕色半透明线 | **must-fix** |
| C9 | 移动顶栏按钮、上一章/下一章/阅读进度、页码提示（上游 mini `.tool-icon`、`.progress-tip`） | `#666` | 白色（提示 76%） | **must-fix** |
| C10 | 移动页码滑块 | 已读段 `#185798`，未读段 `#282828`（6px、圆角 3px），滑块 16px `#282828` + `2px solid #185798` | `accent-color: #409eff` | **must-fix**：夜间按上游画出已读段/未读段/滑块；日间保持原生 `#409eff`（实现中修订，见第 8 节：原生 range 配深色 `accent-color` 时 Chromium 把未读段画成浅色，原先“未读段交给原生”的 `acceptable-change` 不成立） |
| C11 | 悬停与“开启”高亮（左栏悬停、圆钮悬停/开启、移动浮动按钮开启、左栏当前面板） | 上游没有这些状态 | 悬停变亮；开启 `#7ed8d4`；当前面板为强调色 | **acceptable-change**：保留这些 OpenReader 状态与 `#7ed8d4`（无上游颜色可对齐，见 D2）；当前面板随 C1 变为 `#ed4259` |
| C12 | 移动顶栏/底栏分隔线与阴影、桌面工具栏半透明 + 模糊 | 无分隔线；不透明 `#121212` | 日夜相同的棕色细线/阴影；64–96% 浮窗色 | **acceptable-change**：日夜共用的结构差异，夜间在 `#121212` 上与上游几乎无差别，不重开 |
| C13 | 设置分隔线（上游 `el-divider`） | Element 默认 `#DCDFE6`（日夜相同） | `--reader-control-border` | **acceptable-change**：继续跟随控件边框，夜间随 C2 变为 `#666` |
| C14 | 字号预设（OpenReader 新增） | — | 与选项按钮同色 | 随 C2/C5 使用同一变量（不新增颜色） |

**不重开（另起切片）**：

- 目录/书架/换源面板的列表文字与分隔线（上游夜间 `#888`/`#555`/`#bbb`、分隔 `#333`，标题 `.title-btn` `#606266`；当前白字、目录分隔
  `#f0f0f0`）。本切片只让它们的强调色随 C1 改变。
- 朗读条：OpenReader 的蓝色朗读条日夜相同；上游夜间播放键/选中语音 `#185798`、语音按钮 `#bbb`、滑块同 C10。
- `App.vue` `.night-theme` 的全局 Element 覆盖（对话框、消息框、按钮、输入框、表格等），涉及首页工作台与所有弹窗。
- 日夜共用、与夜间无关的差异：“新增方案”按钮在 OpenReader 用强调色（上游是普通选项色）；圆钮的轻阴影；颜色选择器沿用 Element 默认。
- 阅读面、正文色、页边框（日夜切片 P1–P6）；自定义方案的页面/浮窗/正文颜色、背景图、4.5:1 对比度保护与纯黑接管；隐藏 `black`
  主题的阅读面；全部日间配色。

## 5. 用户决定（2026-10-29 已确认）

用户在问答中选择“Full upstream”：

- **D1（取代旧可读性增强）**：C1–C10 采用上游值，即夜间工具按钮与设置标签为 `#666`、设置文字 `#606266`（对 `#121212`
  约 3.1–3.3:1，原来的白字约 18.7:1），强调色 `#ed4259`（对 `#121212` 4.9:1，原来 `#ff7589` 对 `#303030` 为 5.1:1）。
  未采用的备选：只改边框/底色/强调色/滑块、文字保留白色。
- **D2（作用范围）**：与上游一样对所有夜间方案生效（内置夜间、隐藏 `black`、自定义夜间）；自定义方案自己的颜色、背景图与正文
  对比度保护不变。自动阅读/朗读开启色 `#7ed8d4` 保留（上游没有该状态，C11）。

这取代 [`reader-night-contrast-p0-contract.md`](reader-night-contrast-p0-contract.md) 第一轮“实施结果”里夜间强调色 `#ff7589`、
控件面 `#303030` 与工具/设置文字按 4.5:1 替换为浅色这三项用户可读性增强；该合同为正文、自定义方案和纯黑接管建立的其它规则不变。

## 6. 目标行为

1. `Reader.vue#readerStyle`：`--reader-control-bg` 夜间 `rgba(45, 45, 45, 0.5)`、`--reader-control-border` 夜间 `#666`、
   `--reader-accent` 日夜都是 `#ed4259`；`--reader-active-text` 与日间值不变。另加一组**只在夜间设置**的上游控件变量
   （日间不设置，各组件以现有表达式作为 `var()` 回退，日间渲染不变）：

   | 变量 | 夜间值 | 上游来源 | 使用者 |
   | --- | --- | --- | --- |
   | `--reader-chrome-text` | `#666` | `.tool-icon`、`.icon-text`、`.float-*-btn-zone`、`.progress-tip`、`.cache-content-zone`、`.setting-item-title` | 桌面左栏/圆钮/进度/章节、缓存条、移动顶栏/浮动按钮/底栏/页码提示、设置标签 |
   | `--reader-chrome-border` | `#444` | `.tool-icon` 边框 | 桌面左栏与进度栏的格线和外框 |
   | `--reader-float-border` | `transparent` | `.float-btn` 无边框 | 桌面右侧圆钮、移动浮动按钮 |
   | `--reader-slider-accent` | `#185798` | `.el-slider__bar`/`__button` 边框 | 移动页码滑块已读段与滑块边框（日间 `accent-color` 回退 `#409eff`） |
   | `--reader-slider-runway` | `#282828` | `.el-slider__runway`/`__button` 底色 | 移动页码滑块未读段与滑块底色 |
   | `--reader-settings-text` | `#606266` | `.el-popover` 继承色 | 设置面板正文、选项、提示、上传字体图标、字号预设、步进器 |
   | `--reader-settings-value-text` | `#a6a6a6` | `.setting-input .el-input__inner`、`.lang` | 步进器中间数值 |
   | `--reader-theme-selected-border` | `#666` | `.night .selected` | 选中的主题色块边框 |
   | `--reader-moon-icon` | `#ed4259` | `.night .moon-icon` | 内置夜间色块上的月亮 |

2. 组件只把颜色改为“新变量，回退到现有表达式”，不改尺寸、布局与交互。主题色块边框改用 `--reader-control-border`
   （日间正好是现在的 `rgba(0, 0, 0, 0.1)`，夜间 `#666`）。步进器禁用色按设置文字的 48% 计算，避免夜间禁用比可用更亮。
   唯一的 DOM 变化：移动页码滑块在夜间加 `night-slider` 类并带 `--page-slider-progress`（当前页占比，拖动时跟随草稿值），
   夜间用 `appearance: none` 画上游 `el-slider`；盒子仍为 16px 高、位置不变。
3. `--reader-popup-text`、`--reader-text` 与阅读面计算不变（目录/书架/换源面板的列表文字因此不变）。

## 7. 计划测试（先红）

`frontend/tests`（node --test）：

1. `readerThemeContrast.test.mjs` 改写“night settings controls use semantic surfaces…”：断言夜间 `--reader-control-bg`
   `rgba(45, 45, 45, 0.5)`、`--reader-control-border` `#666`、`--reader-accent` 恒为 `#ed4259`，以及夜间控件变量组的上游值；
   日间 `rgba(255, 255, 255, 0.5)`/`rgba(0, 0, 0, 0.1)`/`#ed4259`/`#0f5451` 不变（对照）。删除 `#ff7589` 对 `#303030` 的旧对比度断言
   （与新决定冲突，按 CLAUDE.md 改写）。
2. 新增源码合同：左栏、圆钮、进度/缓存、移动顶栏/浮动/底栏、设置面板与步进器分别通过新变量取色，且回退值等于现在的日间表达式；
   主题色块夜间规则；滑块 `accent-color` 走变量、回退 `#409eff`。
3. 对照：`--reader-popup-text` 计算、自定义方案对比度保护、内置夜间阅读面断言保持通过。

浏览器（真实 Go 服务 + 新构建，新数据目录）：1440×900、390×844、360×800，外加 iPad 1024×1366 / 1366×1024 的设置面板。
内置夜间读取左栏/圆钮/进度/移动工具与设置控件的 computed style（`rgb(102, 102, 102)`、`rgb(68, 68, 68)`、
`rgba(45, 45, 45, 0.5)`、`rgb(237, 66, 89)`、`rgb(96, 98, 102)`、`rgb(166, 166, 166)`、滑块 `rgb(24, 87, 152)`），日间同一批
元素与改动前一致；`reader-mobile-contract.mjs` 自定义夜间上传/删除色改为 `rgb(237, 66, 89)`。

## 8. 记录（2026-10-29）

提交（分支 `claude/reader-night-chrome-palette`）：合同 `795d914`；红测 `a47bc03`；实现 `7d8a0b2`；浏览器合同 + 补充红测 `f84dbbc`；
补充实现 `a44ea45`。

- 红测：`a47bc03` 在 `c62c619` 上 6 项断言失败（夜间变量组、夜间控件/强调色、桌面工具、移动工具、设置/步进器 5 项新测，以及按新决定
  改写的 `readerThemeContrast` 夜间控件测试）；日间回退对照与原有 11 项对比度/阅读面测试通过。`7d8a0b2` 后同一断言全部转绿。
- 实现中修订 C10（`f84dbbc` 红 → `a44ea45`）：真实浏览器里夜间滑块的未读段变成白色——原生 range 配深色 `accent-color: #185798`
  时 Chromium 会把未读段画成浅色（旧的 `#409eff` 仍是深灰），“未读段交给原生”的 `acceptable-change` 不成立。现在夜间滑块加
  `night-slider` 类，按上游 `el-slider` 画 `#185798` 已读段、`#282828` 未读段与 16px `#282828` 滑块 + `2px solid #185798` 圈，
  已读比例 `--page-slider-progress` 跟随拖动草稿值；变量组加 `--reader-slider-runway: #282828`。补充红测在 `7d8a0b2` 上 2 项单元
  断言与浏览器滑块断言（`390: night chrome slider appearance expected none, got auto`）失败，`a44ea45` 后通过。
- 测试：frontend 834/834 与 build；backend `go test ./...` 29 个包与 `go vet ./...`（后端未改动）；advisory 门自测 109/109，
  Go 与前端公告扫描 exit 0。
- 真实浏览器（本地 Go 服务 + 新构建与 `c62c619` 构建各一份，新数据目录）：
  - `reader-mobile-contract.mjs`（模拟接口）全量通过：内置夜间在 1440×900、390×844、360×800 与 iPad 1024×1366 / 1366×1024
    （自适应场景）逐项读到上游值，切回日间后工具栏 computed style 与切换前逐项相同；自定义夜间上传/删除色 `rgb(237, 66, 89)`。
    同一脚本在 `c62c619` 构建上于第一项失败：`1440: night chrome rail border-left-color expected rgb(68, 68, 68), got rgba(148, 132, 87, 0.26)`。
  - 真实账号 + 导入的 TXT，两份构建对照五个视口：日间工具栏与设置面板的 computed style 与 `c62c619` **完全相同**；夜间为
    工具/标签 `rgb(102, 102, 102)`、格线 `rgb(68, 68, 68)`、圆钮与浮动按钮边框 `rgba(0, 0, 0, 0)`、设置文字 `rgb(96, 98, 102)`、
    选项/字体/字号预设/步进器 `rgba(45, 45, 45, 0.5)` + `rgb(102, 102, 102)`、数值 `rgb(166, 166, 166)`、选中与重置/月亮
    `rgb(237, 66, 89)`、主题色块（含选中）`rgb(102, 102, 102)`、分隔线 `rgb(102, 102, 102)`、正文仍 `rgb(102, 102, 102)`；旧构建夜间为
    白字（标签/选项 `rgb(255, 255, 255)`、左栏 72%）、`rgb(48, 48, 48)`/`rgb(86, 86, 86)` 控件、`rgb(255, 117, 137)` 强调色、白圈。
    无控制台错误、页面错误或 5xx。390×844 滑块日夜都是 16px 高、位置相同（top 768），拖到第 5/18 页时已读比例 23.5%。
  - `reader-epub-contract.mjs`（三视口）、`reader-settings-position-contract.mjs` 通过。`reader-desktop-panel-controls-contract.mjs`
    的 C2–C4（84 组）通过；C1 flip 设置有偶发 harness 超时：失败组在开始处等待 `.reader-chapter-end` 出现 10 秒超时（还没打开面板），
    哪几组失败每次不同。只跑 C1 flip（12 组）各两次：新构建 12/12、10/12，`c62c619` 构建 9/12、10/12，与本切片无关（日间渲染相同），
    未修改该脚本。
- 观察到但未改动：手机模式的 iPad 点月亮后变为自适应布局——内置“黑夜默认”方案带 `pageMode: '自适应'`，与上游 `setNightTheme`
  整包合并一致；设置里的字体预览框（OpenReader 新增，`#fffaf0` 底）日夜相同，夜间仍是浅色块，上游没有对应元素。

状态：**implementation-complete / regression-validated**，待可信 Actions；镜像未发布；生产未访问。
