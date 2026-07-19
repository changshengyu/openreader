# Reader 移动工具层控制合同（P0）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-17 已实施并通过源码、单元、构建和三视口真实浏览器合同。

本合同以固定提交的模板、最终响应式 CSS 和事件处理方法为权威依据，取代此前仅按模板
节点顺序判断移动工具顺序的结论。

## 权威文件

- 上游 `web/src/views/Reader.vue`
  - 顶部工具模板、移动端 `order: -1`、左右浮动按钮与 `eventHandler()`。
  - `toTop(0)` / `toBottom(0)` 及 `toShelf()` 动作。
- 当前 `frontend/src/components/reader/ReaderMobileChrome.vue`
- 当前 `frontend/src/composables/useReaderTools.js`
- 当前 `frontend/src/views/Reader.vue`

## 差异矩阵

| 项目 | 固定上游行为 | 当前行为 | 判定与动作 |
|---|---|---|---|
| 移动顶部最终顺序 | 模板节点是“书架→书源→目录→设置→首页”，但 mini 模式给“首页”节点内联 `order:-1`；flex 最终可见顺序是 **首页→书架→书源→目录→设置**。 | `ReaderMobileChrome.vue` 已直接按最终可见顺序渲染，桌面组件仍保留模板顺序。 | `aligned`：不复制内联 order 的实现细节，但最终可见行为一致。 |
| 移动左侧浮动按钮 | 书签、搜索正文、书籍信息后，在 mini 模式继续显示“顶部”和“底部”，分别调用 `toTop(0)`、`toBottom(0)`。 | 已显示五项并复用现有 `scrollToTop` / `scrollToBottom` 动作。 | `aligned`：源码、动作和两种移动高度的浏览器合同均已覆盖。 |
| 工具层状态 | `showToolBar` 初始为 `true`。上述工具按钮位于正文事件层之外；点击首页/顶部/底部不先隐藏工具层。 | 初始显示与主面板并存已一致；新增动作必须沿用当前不强制隐藏 chrome 的工具分发器。 | `aligned`，增加回归保护。 |
| 主面板并存 | 书架、书源、目录、设置打开时，正文 `eventHandler()` 先返回，工具层仍显示。 | `useReaderPrimaryPanels` 与 pointer guard 已保持工具层；按钮变更不得破坏。 | `aligned`，三视口重跑。 |
| 格式可达性 | 顶部/底部按钮只按 mini 模式显示，不按 EPUB、图片、音频或远程/本地书隐藏。 | 新增按钮不得加格式/书籍类型禁用条件。 | `must-preserve`。 |
| iPad / 宽屏手机模式 | 上游只用 `window.innerWidth <= 750` 决定 mini；OpenReader 额外支持移动浏览器识别和用户手动“手机模式”。这两个入口必须驱动同一套 Reader 模板与样式。 | 2026-07-19 复审发现 JS 会把 iPad Pro 判为 mini，但 `Reader.vue` 和 `ReaderMobileChrome.vue` 的关键 CSS 仍只在 `max-width:750px` 生效；宽屏 iPad 因此渲染移动主面板却隐藏移动顶部工具，四个主面板都失去同按钮关闭路径。现已由 Reader 根节点语义 class 统一模板和样式状态。 | `resolved 2026-07-19`：移动/桌面工具按同一状态挂载；移动布局不再以独立宽度 media query 作为生效前提。1024×1366 与 1366×1024 已证明四面板均可由顶部同按钮关闭。 |

## 状态转换

```text
进入移动 Reader
  -> chrome = visible
  -> 顶部顺序 = 首页 / 书架 / 书源 / 目录 / 设置
  -> 左浮动区 = 书签 / 搜索 / 信息 / 顶部 / 底部

点击顶部
  -> 调用现有 scrollToTop()
  -> chrome 保持 visible
  -> primary panel 状态不被隐式改变

点击底部
  -> 调用现有 scrollToBottom()
  -> chrome 保持 visible
  -> primary panel 状态不被隐式改变
```

## 实施前测试合同

1. 源码契约同时断言：移动最终顺序为“首页→书架→书源→目录→设置”，桌面仍为
   “书架→书源→目录→设置→首页→顶部→底部”。
2. `useReaderTools` 单元测试证明移动 `top` / `bottom` 复用动作映射，且不会修改
   `mobileChromeVisible`。
3. 真实浏览器在 390×844、360×800：
   - 默认工具层可见；
   - 顶部五项顺序正确；
   - 左侧五个按钮全部可见且不发生纵向重叠；
   - 在足够长的正文中，底部按钮移动到末端、顶部按钮返回开头；
   - 两次动作后工具层仍可见，正文左右留白不变；
   - 打开四个主面板后工具层仍可见且点击不穿透。
4. 1440×900 复验桌面左栏顺序，确保移动修复没有改变桌面合同。
5. iPad Pro 1024×1366 与 1366×1024（iPad user agent + touch）：
   - Reader 根节点、正文 16px 对称布局和移动工具层使用同一 mini 状态；
   - 桌面左右工具和桌面进度区不挂载；
   - 书架、书源、目录、设置打开后，顶部工具始终位于面板之上；
   - 再点当前工具可以关闭面板，且面板高度不超出可视区。

## 允许差异

- Vue 3 组件可以直接按最终 flex 可见顺序渲染，不要求保留上游的内联 `order:-1`
  实现方式。
- 用户要求的原生连续滚动与减号/数值/加号设置控件保持不变。

## 2026-07-17 实施与验证记录

- `ReaderMobileChrome.vue` 直接按最终 flex 可见顺序渲染
  “首页→书架→书源→目录→设置”；桌面左栏仍保持
  “书架→书源→目录→设置→首页→顶部→底部”。
- 移动左侧浮动区补回“顶部”和“底部”，复用 Reader 已有动作映射；没有新增格式、
  本地/远程或临时阅读限制，也不修改 `mobileChromeVisible`。
- `frontend/tests/readerToolOrderContract.test.mjs` 与
  `frontend/tests/readerTools.test.mjs` 覆盖双端顺序、按钮归属、动作分发和工具层状态。
- `scripts/smoke/reader-mobile-contract.mjs` 使用足够长的正文，在 1440×900、390×844、
  360×800 生产构建中验证移动五项顶部工具、五项左浮动工具、无重叠、滚动到顶/底、
  工具层保持、正文左右几何和四主面板隔离。
- 全量结果：前端 411 项测试通过，后端 `go test ./...` 通过，Vite 生产构建通过；仅保留
  既有 Element Plus 大 chunk 警告。

## 2026-07-19 iPad Pro 宽屏修复记录

- 根因是响应状态分裂：`shouldUseMiniInterface()` 会按移动 UA / iPad touch platform 将
  1024px 或 1366px 的 iPad 判为 mini，但 Reader 主布局和 `ReaderMobileChrome` 仍各自
  依赖 `max-width:750px`。因此移动主面板已挂载，位于其上方的关闭/切换工具却被隐藏。
- `Reader.vue` 现在把 `isMobileReader` 映射为根节点 `mini-interface` class；正文、flip、
  主面板和 click-away 几何均由该语义 class 驱动。桌面左右工具/进度和移动工具也按同一
  状态条件挂载。
- `ReaderMobileChrome.vue` 只在 mini 状态挂载，因此其 visible 样式不再重复使用宽度
  media query；顶部工具继续以 z-index 11 位于 z-index 10 主面板之上。
- `frontend/tests/readerIPadWorkspaceContract.test.mjs` 保护状态单一来源；Reader 全部 292
  项定向测试与 Vite 生产构建通过。
- `scripts/smoke/reader-mobile-contract.mjs` 已扩展 iPad Safari UA + touch 合同；
  1440×900、390×844、360×800、1024×1366 和 1366×1024 全部通过。iPad 两种方向
  均验证四个主面板可打开/关闭、顶部工具可命中、桌面工具不挂载、正文 16px 对称。
