# 夜间 Element Plus 组件外观合同（P1，上游 `App.vue` `.night-theme`）

状态：**contract-only**（用户 2026-10-30 确认第 5 节 D1–D4：全局，同上游；目录标签用上游标签惯例；空状态与遮罩改深色；照上游实际渲染。2026-11-02 实现中补充 D5/D6：首页侧栏与登录页保持原样）。审查日期：2026-10-30。审查基线：`OpenReader@6193f79`。

## 1. 来源与范围

[`reader-night-chrome-palette-fixed-baseline-second-audit-p1-contract.md`](reader-night-chrome-palette-fixed-baseline-second-audit-p1-contract.md)（`24511c0`）
与 [`reader-night-list-typography-fixed-baseline-second-audit-p1-contract.md`](reader-night-list-typography-fixed-baseline-second-audit-p1-contract.md)（`21d78ab`）
都在第 4 节“不重开”里把 `App.vue` `.night-theme` 的全局 Element 夜间覆盖留到另起切片：那两次只改了 Reader 自己画的工具栏、设置控件与
三块列表，面板里的 Element Plus 组件、从 Reader 打开的对话框与消息框在夜间仍是浅色。本切片就是那一项。

上游 `.night-theme` 挂在 `body` 上，对所有页面与弹窗生效，所以范围有两种（第 5 节 D1）：

| 范围 | 涉及的 Element 外观 |
| --- | --- |
| 阅读页 | 换源面板分组 `el-select` 与下拉浮层；目录“本地/已缓存” `el-tag`；书架与换源面板 `el-empty`；书架刷新 `v-loading` 遮罩；设置面板 4 个 `el-color-picker` 的色值输入与按钮；消息框（新增/删除方案、修改目录规则、阅读页各类确认）；从阅读页打开的对话框：书签、书签编辑、正文搜索、书籍信息、替换规则、分组（对话框、表格、按钮、输入框、复选框、开关、提示条） |
| 全局另含 | 首页工作台（侧栏、书架、搜索、探索）、书源管理、书籍管理、导入、本地书仓、WebDAV、用户管理、RSS、备份恢复等弹窗；书源调试页；登录页；以及只在这些地方出现的标签页、分页、下拉菜单、数字输入框、下拉多选标签 |

日间外观、Reader 自己画的工具栏/设置/列表（前两个切片）、阅读面与正文色都不在本切片内。

## 2. 固定上游

`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`，Element UI `^2.15.9`（测量用 2.15.14）。

`web/src/App.vue`：

- `setTheme`（480–490）在 `isNight`（`plugins/vuex.js:457`：`config.themeType === "night"`，与内置主题还是自定义方案无关）时给
  `document.body` 加 `night-theme`，`beforeMount`（329）与 `isNight` 监听（425）调用。Element 的对话框、消息框、下拉浮层都挂到 `body`，
  所以覆盖对首页、阅读页和所有弹窗一律生效。
- `.night-theme`（939–1141）：`background-color: #222`；`.el-message-box` `#212121` 底与 `1px solid #212121`，标题 `#888`，内容 `#777`；
  `.el-button--default` `#888` 底与边、`#ddd` 字，`.el-button:focus/:hover` `#bbb` 底与边、`#eee` 字，`.el-button--text:focus/:hover`
  `#66b1ff` 透明，`.el-button.is-disabled` `#666` 字，`.el-button--primary` `#185798`，悬停/聚焦 `#2b67bb`；`.el-input-number__increase/__decrease`
  `#909399` 底与边、白字；`.el-checkbox__inner` `#bbb`；`.el-input__inner`/`.el-textarea__inner` `#444` 底、`1px solid #444 !important`、`#ddd` 字；
  `.el-tabs__item` `#ddd`，左右滚动钮 `#aaa`，分隔 `#444`；`.el-select-dropdown` `#333` 底与边（`!important`），选项 `#ddd`，悬停 `#444`；
  `.el-select .el-tag--info` `#777`/`#ddd`；多选浮层选中/悬停 `#444`/`#555`；浮层箭头 `#333`；`.el-dialog` `#222`、`.el-dialog__title` `#bbb`；
  分页 `#444`/`#ddd`、当前页 `#409EFF`；表格透明底、表头与行 `#222 !important`、正文 `#888`、分隔 `#555`、悬停 `#444` + `#ccc`、
  底线透明；`.el-dropdown-menu` `#444`（`!important`）、项 `#bbb`、悬停 `#666`；`.check-tip`（“已选择 N 个”）`#bbb`；表格/对话框滚动条
  `#333`（上游 `*::-webkit-scrollbar { display: none }` 全局隐藏滚动条，看不到）；代码编辑器 token 底色继承。
- 层叠顺序：`main.js` 先 `import App from "./App.vue"`（2 行）再 `import "./plugins/element.js"`（4 行，`babel-plugin-component` 按需引入
  theme-chalk），打包后 Element UI 自己的 CSS 排在 `App.vue` 之后。**同权重时 Element UI 自己的规则赢**，所以上游实际效果不能只看
  `.night-theme` 的写法。

上游**没有**覆盖、夜间保持 Element 默认（浅色）的：`el-switch`、`el-alert`、下拉框外的 `el-tag`（上游按组件自己写
`:effect="isNight ? 'dark' : 'light'"`：`Index.vue`、`Explore.vue`、`BookInfo.vue:76`）、`el-message` 消息提示、`el-color-picker` 面板底色、
`el-popover`（阅读页面板用主题浮窗色）、对话框正文与表单标签 `#606266`、关闭图标 `#909399`、表头文字 `#909399`。上游没有 `el-empty`，
也不用 `v-loading`：`$loading` 每次自己给底色——阅读正文 `rgba(0,0,0,0)`（`Reader.vue:880`、`1002`），首页书架 `isNight ? "#222" : "#fff"`
（`Index.vue:1264`）。

组件自己的 `.night-theme` 规则（不在 `App.vue`）：`BookInfo.vue` `.book-name #eee`（OpenReader `BookInfoPanel.vue:291` 的写法从未生效，见第 3 节）、RSS 三个组件
（`#aaa`/`#333`/`#666`）、阅读页朗读语音单选（`Reader.vue:3762`）、`Index.vue` `.popper-intro`/`.el-popover__title`（3663–3680）、
`Index.vue` `.night` 里的 `.el-input__inner` `#444`/`#aaa`（3407）。本切片只处理 `App.vue` 那一块。

### 2.1 上游实测

方法：npm 包 `element-ui@2.15.14` + `vue@2.6.14` 渲染同一批组件，把 `App.vue` 939–1141 的 stylus 摊平为 CSS，按上游打包顺序放在
Element UI CSS **之前**，`body.night-theme`，Chromium 1440×900 读 computed style（悬停/聚焦等 Element 过渡结束后再读）。结果（与上一段
规则不一致的地方加粗）：

| 组件 / 状态 | 上游夜间实测 |
| --- | --- |
| `body` | 底 `#222` |
| 消息框 | 底与边 `#212121`；标题 `#888`；内容 `#777`；关闭图标 `#909399`；取消按钮（默认、small）与确定按钮（主要）同下 |
| 默认按钮（含朴素） | `#888` 底与边、`#ddd` 字；悬停/聚焦 `#bbb` 底与边、`#eee` 字 |
| 主要按钮 | `#185798` 底与边、白字；悬停/聚焦 `#2b67bb` |
| 危险等类型按钮 | 常态 Element 默认（危险 `#f56c6c`）；**悬停/聚焦变成 `#bbb` 底与边、`#eee` 字**（通用 `.el-button:hover` 权重更高） |
| 朴素主要按钮 | **保持 Element 浅色** `#ecf5ff` 底、`#409eff` 字、`#b3d8ff` 边（Element 规则同权重、排在后面） |
| 文字按钮（`type="text"`） | `#409eff`，悬停 `#66b1ff`，透明底（与 Element 默认相同） |
| 禁用按钮 | 字 `#666`；**底与边保持 Element 默认**：默认按钮白底 `#ebeef5` 边，主要按钮 `#a0cfff` 底与边（悬停不变） |
| 输入框 / 文本框 / 选择框输入面 / 消息框输入 | `#444` 底、`#444` 边（悬停、聚焦不变色）、`#ddd` 字；占位 `#c0c4cc`（默认）；**禁用：底 `#f5f7fa`、字 `#c0c4cc`（默认）**，边 `#444` |
| 数字输入框加减钮 | `#909399` 底与边、白字 |
| 选择下拉浮层 | 底与边 `#333`，箭头 `#333`；选项 `#ddd`；悬停 `#444`；**选中 `#409eff` 700**；**禁用项 `#c0c4cc`**；多选已选 `#444` 底 |
| 下拉框内多选标签 | `#777` 底与边、`#ddd` 字 |
| 标签页 | 项 `#ddd`；**选中与悬停 `#409eff`**；分隔线 `#444`；选中条 `#409eff` |
| 对话框 | 底 `#222`；标题 `#bbb`；正文与表单标签 `#606266`、关闭图标 `#909399`（默认） |
| 表格 | 表底透明；表头格与行 `#222`；正文 `#888`；表头字 `#909399`（默认）；分隔 `#555`；悬停格 `#444` 底、`#ccc` 字；底线透明 |
| 分页 | 上一页/下一页与页码 `#444` 底、`#ddd` 字；当前页 `#409eff` |
| 下拉菜单 | 底与边 `#444`；项 `#bbb`；悬停 `#666` 底、`#66b1ff` 字；禁用项 `#bbb`（Element 默认） |
| 复选框 | 未选内框 `#bbb`（边 `#dcdfe6`）；**选中 `#409eff`**；文字 `#606266` |
| “已选择 N 个”（`.check-tip`） | `#bbb` |
| 开关、提示条、下拉外标签、消息提示、`el-empty`、默认遮罩 | Element 默认浅色（遮罩 `rgba(255, 255, 255, 0.9)`） |

## 3. 当前 OpenReader（`6193f79`）

`App.vue:127–134` 在 `readerStore.themeType === 'night'` 时给 `<html>` 加 `dark-reader`；`styles/tokens.css:36` 的 `html.dark-reader` 只改
`--app-*`（页面底 `#181715` 等）。`main.js:5` 引入 `element-plus/dist/index.css`（2.14.0），之后没有任何夜间 Element 覆盖，所以夜间所有
Element Plus 组件都是默认浅色。用同一批组件在 Element Plus 2.14 + OpenReader `global.css`（`html.dark-reader`）下实测：

| 组件 / 状态 | 当前夜间 |
| --- | --- |
| `body` | `#181715`（`--app-bg`） |
| 消息框 | `#fff` 底；标题 `#303133`；内容 `#606266` |
| 默认按钮 | `#fff` 底、`#dcdfe6` 边、`#606266` 字；悬停 `#ecf5ff`/`#c6e2ff`/`#409eff` |
| 主要按钮 | `#409eff`；悬停 `#79bbff` |
| 禁用按钮 | 字 `#a8abb2`（主要白字），底边同上游 |
| 输入框 / 选择框 | `#fff` 底、`#dcdfe6` 环（聚焦 `#409eff`）、`#606266` 字 |
| 选择下拉浮层 | `#fff` 底、`#e4e7ed` 边；选项 `#606266`；悬停 `#f5f7fa` |
| 对话框 | `#fff` 底；标题 `#303133` |
| 表格 | `#fff`；正文 `#606266`；分隔 `#ebeef5`；悬停 `#f5f7fa` |
| 复选框 | 未选 `#fff` |
| 标签页 / 分页 / 下拉菜单 / 数字输入框 | Element Plus 默认浅色 |
| “已选择 N 个” | 继承正文（`.check-tip` 或无类名的 `span`） |

已有的 OpenReader 夜间 Element 处理（不在本切片改动）：首页书架搜索 `#444`/`#aaa`（`home-shelf.css:396–403`，对应上游 `Index.vue`
`.night`）；首页书架加载遮罩 `#222`（`Home.vue:151`，对应上游 `Index.vue:1264`）；书籍信息与远程结果的 `el-tag` 夜间 `effect="dark"`
（`BookInfoPanel.vue:64`、`RemoteBookResultList.vue:33`，对应上游）。

实现中发现（2026-11-02）：`AppLayout.vue:1023–1071` 的首页侧栏夜间样式与 `BookInfoPanel.vue:291` 的 `.book-name #eee` 都写成
`:global(html.dark-reader) .x`。Vue 3 的 `:global()` 会把整个选择器替换成括号里的部分，构建产物里只剩 `html.dark-reader { … }`
（作用在 `<html>` 上），这些规则**从未生效**：夜间首页侧栏一直是日间浅色（侧栏设置下拉 `#fffdf8` 底、`#e6e6e6` 环、`#606266` 字，
搜索框 `#fffdf8`），书籍信息书名是 `--app-text`（夜间 `#d8d4c8`）。登录页（`Login.vue`）是固定浅色的独立页面（`#faf8f2` 底、`#fff`
卡片），上游是夜间会变深的登录对话框。

阅读页里 OpenReader 新增、上游没有的 Element 元素：目录 `el-tag`（`ReaderTocPanel.vue:17`，`type="success" effect="plain"`，白底绿字）；书架与
换源面板 `el-empty`（`ReaderShelfPanel.vue:18`、`SourceSwitchPanel.vue:40`，浅色插图）；书架刷新 `v-loading`（`Reader.vue:43`、`279`，白色 90%
遮罩）；EPUB 目录规则选择用原生 `select`，行内样式取 `--el-bg-color` 等变量（`useReaderLocalTocRulePicker.js:22–29`，上游这里是
`el-select size="mini"`，`PopCatalog.vue:211`）。

## 4. 差异与裁决

只看 `themeType === 'night'`（= 上游 `isNight`），日间逐项保持现状。“范围”一列：R = 阅读页里就能看到；G = 只在全局范围出现。

| 编号 | 项目 | 范围 | 上游夜间（2.1 实测） | 当前夜间 | 裁决 |
| --- | --- | --- | --- | --- | --- |
| N1 | 页面底色 | R/G | `#222` | `#181715` | **must-fix** |
| N2 | 消息框 | R/G | `#212121` 底与边，标题 `#888`，内容 `#777` | 白底、`#303133`/`#606266` | **must-fix** |
| N3 | 默认按钮（含朴素）及其悬停/聚焦 | R/G | `#888`/`#ddd`；`#bbb`/`#eee` | 白底浅灰边；浅蓝悬停 | **must-fix** |
| N4 | 主要按钮及其悬停/聚焦 | R/G | `#185798`；`#2b67bb` | `#409eff`；`#79bbff` | **must-fix** |
| N5 | 禁用按钮 | R/G | 字 `#666`；底边保持 Element 默认（白/`#a0cfff`） | 字 `#a8abb2`/白 | **must-fix**：字 `#666`；底边保持 Element Plus 默认（D4） |
| N6 | 危险等类型按钮的悬停/聚焦 | R/G | `#bbb` 底与边、`#eee` 字 | 本色变浅 | **must-fix**（D4） |
| N7 | 朴素主要按钮、文字/链接按钮 | R/G | 保持 Element 默认 | Element Plus 默认 | 一致：夜间不改 |
| N8 | 输入框、文本框、消息框输入、颜色选择器色值 | R/G | `#444` 底与边（各状态）、`#ddd` 字；占位与禁用为默认色 | 白底、`#dcdfe6` 环、聚焦 `#409eff` | **must-fix** |
| N9 | 选择框输入面（换源分组；EPUB 目录规则原生 `select`） | R/G | 同 N8 | 同上 | **must-fix** |
| N10 | 选择下拉浮层与箭头 | R/G | `#333`；选项 `#ddd`；悬停 `#444`；选中 `#409eff` 700；禁用默认 | 白底、`#606266`、`#f5f7fa` | **must-fix** |
| N11 | 对话框 | R/G | `#222` 底、`#bbb` 标题；正文/标签/关闭图标默认 | 白底、`#303133` | **must-fix** |
| N12 | 表格 | R/G | 透明表底、`#222` 表头与行、`#888` 正文、`#555` 分隔、悬停 `#444`/`#ccc`、底线透明 | 白底、`#606266`、`#ebeef5` | **must-fix** |
| N13 | 复选框未选内框 | R/G | `#bbb`；选中保持 `#409eff` | `#fff` | **must-fix** |
| N14 | “已选择 N 个” | R/G | `#bbb` | 继承正文 | **must-fix**（书签、替换规则底栏的无类名 `span` 一并） |
| N15 | 下拉框内多选标签 | G | `#777`/`#ddd` | `#f4f4f5`/`#909399` | **must-fix**（D1） |
| N16 | 数字输入框加减钮 | G | `#909399` 底与边、白字 | `#f5f7fa`/`#606266` | **must-fix**（D1） |
| N17 | 标签页 | G | 项 `#ddd`、选中/悬停 `#409eff`、分隔 `#444`、滚动钮 `#aaa` | `#303133`、`#e4e7ed` | **must-fix**（D1） |
| N18 | 分页 | G | `#444`/`#ddd`、当前页 `#409eff` | 默认浅色 | **must-fix**（D1） |
| N19 | 下拉菜单 | G | `#444` 底与边、项 `#bbb`、悬停 `#666` + `#66b1ff` | 白底、`#606266`、`#ecf5ff` | **must-fix**（D1） |
| N20 | 上游没覆盖的 Element 组件：开关、提示条、下拉外标签、消息提示、颜色选择器面板、图片预览、弹出框 | R/G | Element 默认浅色 | Element Plus 默认 | 一致：不改 |
| N21 | 上游没覆盖的状态里 Element Plus 与 Element UI 2.15 的默认色差（占位 `#a8abb2`↔`#c0c4cc`、禁用字 `#a8abb2`↔`#c0c4cc`、禁用边 `#e4e7ed`↔`#ebeef5`、朴素主要边 `#a0cfff`↔`#b3d8ff`） | R/G | — | — | **acceptable-change**（Vue 3 / Element Plus 运行时适配，与日间相同的取舍） |
| N22 | 结构映射：Element Plus 输入/选择用内阴影环代替边框；按钮键盘焦点有 `:focus-visible` 轮廓 | R/G | 边框 `#444`；无焦点轮廓 | — | **acceptable-change**：环色取上游边框值；保留键盘焦点轮廓（无障碍） |
| O1 | 目录“本地/已缓存”标签（OpenReader 新增） | R | 上游目录没有标签；上游标签夜间惯例 `effect="dark"` | 白底绿字 | **intentional-redesign**（D2） |
| O2 | 空状态插图（OpenReader 新增） | R/G | 上游没有 | 浅色插图、`#909399` 说明 | **intentional-redesign**（D3） |
| O3 | `v-loading` 遮罩（OpenReader 新增；上游 `$loading` 按场景给底色） | R/G | 首页书架夜间 `#222`，阅读正文透明 | 白色 90% | **intentional-redesign**（D3） |

**不重开（另起切片）**：

- 首页侧栏整体（底色、文字、搜索框、设置下拉的输入面）按 D5 保持现状，与失效的 `:global()` 写法一起另起切片按上游 `Index.vue`
  `.night`（导航 `#121212` 底与 `#555` 边、标题 `#bbb`、输入框 `#444`/`#aaa` 等）处理；书籍信息书名 `#eee` 一并。`Index.vue` 的
  `.popper-intro`/`.el-popover__title`、RSS 组件与朗读语音单选的 `.night-theme` 规则也不在 `App.vue` 那一块，各自另起。
- 登录页按 D6 保持现状（固定浅色），夜间登录页另起。
- 文字/链接按钮日间与上游 `type="text"` 的配色差异、其它日间 Element 差异；对话框尺寸、圆角、间距等 Element Plus 结构差异。
- 书源调试页在上游是独立静态页（`public/bookSourceDebug`，没有夜间）；按 D1 全局，它的 Element 组件随 `body.night-theme` 一起变暗，属于随全局
  规则的 `acceptable-change`，不另做对齐。
- Reader 自己画的工具栏、设置、三块列表（前两个切片）；阅读面、正文色、自定义方案的颜色与背景图。

## 5. 用户决定（2026-10-30 已确认）

用户在问答中选择（四项都是推荐项）：

- **D1（作用范围）= 全局，同上游**：夜间时首页工作台、阅读页、书源调试页、登录页与所有对话框、消息框、下拉都按第 2.1 节改写
  （N1–N19 全部 must-fix），同一个弹窗从哪里打开都一样。沿用前两个切片的 D2：与上游 `isNight` 一样对所有夜间方案（内置夜间、隐藏
  `black`、自定义夜间）生效。未采用的备选：只做阅读页（含从阅读页打开的弹窗），首页工作台保持浅色。
- **D2（目录标签 O1）= 上游标签惯例**：夜间 `effect="dark"`（绿底 `#67c23a` 白字），与上游 `BookInfo.vue`/`Explore.vue`/`Index.vue` 和
  OpenReader `BookInfoPanel.vue` 的夜间标签一致；日间仍是 `plain`。未采用的备选：夜间透明底保留绿字绿边；保持白底。
- **D3（空状态 O2、遮罩 O3）= 改深色**：夜间 `el-empty` 插图用 Element Plus 自带的暗色填充（`theme-chalk/dark/css-vars.css` 的
  `--el-empty-fill-color-0…9`），说明文字 `#909399` 不变；夜间 `v-loading` 遮罩 `#222`（上游 `Index.vue:1264` 夜间值，OpenReader 首页书架
  已在用），转圈颜色不变。未采用的备选：保持 Element 默认浅色。
- **D4（层叠结果 N5/N6）= 照上游实际渲染**：夜间禁用按钮字 `#666`，底边保持 Element Plus 默认（默认按钮白底浅灰边，主要按钮
  `#a0cfff`）；危险、成功、警告、信息等类型按钮悬停/聚焦为 `#bbb` 底与边、`#eee` 字。未采用的备选：禁用按钮 `#888`/`#185798` 底、类型按钮
  悬停保持本色。

- **D5（首页侧栏，2026-11-02 实现中补充）= 侧栏保持原样，另起切片**：侧栏设置下拉的输入面保持现在的浅色（`#fffdf8`/`#e6e6e6`/
  `#606266`），展开浮层作为全局浮层照常按 N10 夜间；侧栏其余部分本来就不受本切片影响。未采用的备选：下拉框先按上游 `#444`/`#aaa`
  变深；本切片一起按上游改整个侧栏。
- **D6（登录页，2026-11-02 实现中补充）= 登录页保持原样**：登录路由不加 `night-theme`，登录页完全保持现在的浅色；应用内弹出的
  “重新登录”对话框（`AuthDialog.vue`）照常夜间。未采用的备选：登录页按上游登录对话框改成夜间；接受浅色卡片里的深色控件。

## 6. 目标行为

1. `App.vue`：新增一个监听，在 `themeType === 'night'` 且不在登录路由时给 `document.body` 切换 `night-theme`（D1、D6），与上游
   `setTheme` 同名；`dark-reader` 的监听与 `--app-*` 不变。
2. 新增 `frontend/src/styles/night-element.css`，在 `main.js` 里紧跟 `element-plus/dist/index.css` 引入、排在 `App.vue` 等组件样式之前
   （与上游打包顺序一样，组件自己的样式同权重时赢），所有规则都以 `.night-theme` 开头（日间没有这个类，日间渲染不变）。Element Plus 的类名和 CSS 变量与 Element UI 不同，按第 2.1 节**实测结果**映射，而不是逐条照抄选择器：
   - 消息框：`--el-messagebox-title-color` `#888`、`--el-messagebox-content-color` `#777`，底 `#212121`、`1px solid #212121`。
   - 按钮：无类型按钮（不含 `is-text`/`is-link`）设 `--el-button-*` 为 `#888`/`#ddd`、悬停 `#bbb`/`#eee`；主要按钮（不含朴素）`#185798`、
     悬停 `#2b67bb`；`--el-button-disabled-text-color` `#666`，禁用底边不设（D4）；其它类型按钮（不含朴素）悬停 `#bbb`/`#eee`（D4）。
   - 输入：`.el-input`/`.el-textarea`/`.el-select` 的底、文字、环色（含悬停与聚焦）为 `#444`/`#ddd`/`#444`；禁用保持 Element Plus 默认底与字。
   - 浮层：选择下拉 `#333` 底与边与箭头、选项 `#ddd`、悬停 `#444`；下拉菜单 `#444`、项 `#bbb`、悬停 `#666` + `#66b1ff`。
   - 对话框 `--el-dialog-bg-color` `#222`、标题 `#bbb`；表格 `--el-table-*` 按 N12；复选框未选内框 `#bbb`；标签页、分页、数字输入框、
     多选标签按 N15–N19；“已选择 N 个”`#bbb`。
   - `.el-empty` 的 `--el-empty-fill-color-0…9` 取 Element Plus 暗色值，`.el-loading-mask` 底 `#222`（D3）。
3. `ReaderTocPanel.vue` 的目录标签夜间 `effect="dark"`、日间 `plain`（D2），只多一个夜间判断，类型与文字不变。
4. `useReaderLocalTocRulePicker.js` 的原生 `select` 改为跟随输入框夜间色（N9），日间仍是现在的变量值。
5. `AppLayout.vue` 的侧栏设置下拉在自己的输入面规则里固定 Element 的常规文字色（`--el-input-text-color`、`--el-select-multiple-input-color`
   = `var(--el-text-color-regular)`，与日间取值相同），夜间不随全局选择框文字变成 `#ddd`（D5）。

## 7. 计划测试（先红）

`frontend/tests`（node --test）：

1. 新增 `nightElementThemeContract.test.mjs`：`App.vue` 给 `body` 切换 `night-theme`，且只看 `themeType === 'night'`；`night-element.css`
   在 Element Plus CSS 之后加载，每条规则以 `.night-theme` 开头，第 6 节每个值等于第 2.1 节实测值；D2 目录标签夜间 `dark`、日间 `plain`；
   D3 空状态填充与遮罩；D4 不给禁用按钮设底边、类型按钮悬停 `#bbb`；原生目录规则 `select` 跟随输入框夜间变量。
2. 对照：夜间配色与夜间列表两个合同的断言、`dark-reader` 与 `--app-*` 不变。

浏览器（`scripts/smoke/reader-mobile-contract.mjs` 内置夜间段，1440×900、390×844、360×800、iPad 1024×1366 / 1366×1024）：`body` 底色；
换源面板分组选择框与展开后的下拉浮层（选项、悬停、选中）；目录标签；设置面板“新增方案”消息框（底、标题、内容、输入框、取消/确定按钮，
之后取消）；空状态与遮罩（D3）；从阅读页打开的一个对话框（底、标题、表格、按钮）。日间在切换前后各读一次同一批元素，要求逐项相同。
另在首页工作台夜间打开一个对话框（底、标题、表格、按钮、输入框）与一个消息框读取同样的值，日间同样前后一致（D1）。
D5/D6（2026-11-02 补充）：单元测试断言 `night-theme` 监听排除登录路由、`night-element.css` 排在 `App.vue` 之前、侧栏设置下拉固定常规
文字色；首页合同读取侧栏设置下拉的输入面（夜间与日间逐项相同）与它展开后的浮层（夜间 `#333`/`#ddd`），以及登录页（夜间不带
`night-theme`，输入框与按钮与日间逐项相同）。

另用第 2.1 节的方法在 Element Plus 2.14 上渲染同一批组件（OpenReader 的 CSS + 新样式表）与上游实测逐项比对，作为实现自检。
