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
