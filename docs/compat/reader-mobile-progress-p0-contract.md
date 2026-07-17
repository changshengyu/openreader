# Reader 移动底部进度合同（P0）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-17 已实施，并通过单元、生产构建和三类真实 Reader 浏览器合同。

## 权威文件

- 上游 `web/src/views/Reader.vue`
  - `.read-bar .progress`、`currentPage`、`totalPages`、`progressValue`、
    `computePages()`、`showPage()`、`scrollHandler()`、`readingProgress`。
- 当前 `frontend/src/components/reader/ReaderMobileChrome.vue`
- 当前 `frontend/src/composables/useReaderProgressControls.js`
- 当前 `frontend/src/composables/useReaderLayout.js`
- 当前 `frontend/src/utils/readerPagination.js`
- 当前 `frontend/src/views/Reader.vue`

## 语义矩阵

| 项目 | 固定上游行为 | 当前行为 | 判定与动作 |
|---|---|---|---|
| 移动滑条范围 | mini 且非音频时，滑条绑定 `currentPage`，范围 `1…totalPages`。 | 已由 `mobilePageSliderValue/Max` 暴露 1-based 当前渲染页。 | `aligned`。 |
| 滑条标签 | 拖动时显示 `progressValue`，格式为 `第 x/y 页`。 | `mobilePageSliderDraft` 只在 input 期间覆盖标签，格式一致。 | `aligned`。 |
| 拖动定位 | `showPage(page)` 只移动当前渲染文档：左右翻页移动当前章节列；普通上下翻页移动本章；连续模式移动当前连续章节窗口。它不把百分比换算成另一个目录索引。 | `seekRenderedPage()` 只设置当前列或当前滚动窗口，不再调用路由；错误的 `readerBookSeekTarget()` 管线已删除。 | `technical-stack-equivalent`：Vue 内部保留 0-based page，UI 为上游 1-based。 |
| 连续模式页数 | `computePages()` 用当前 Content 的完整滚动高度；`scrollHandler()` 随滚动更新 `currentPage`。连续章节窗口因此也有真实页数。 | `useReaderLayout()` 已为 `page/scroll/scroll2` 统一派生竖向页数和当前页；滚动仍由原生容器拥有。 | `aligned`，连续跨章 smoke 通过。 |
| 音频 | `.progress` 使用 `v-if="miniInterface && !isAudio"`；音频没有文本页码滑条，底部只剩单行 tools。 | `pageSliderVisible` 在音频为 false，并以 `without-page-slider` 收缩 footer。 | `aligned`，390/360/1440 音频合同通过。 |
| 底部中间进度 | `readingProgress = parseInt((chapterIndex+1)*100/catalog.length) + '%'`；mini 文案为 `阅读进度: N%`，点击打开内联缓存区。 | 可见结构已恢复单行 `阅读进度: N%`，章节标题不再重复；点击仍打开同一内联缓存区。 | `acceptable-change`：保留包含章内比例的平滑百分比算法。 |
| 上一章/下一章 | 与进度按钮同一 bottom tools 行；滑条拖动不替代章节按钮。 | 章节按钮存在。 | `aligned`，必须回归边界禁用和工具层状态。 |

## 状态转换

```text
布局/滚动更新
  -> page = 0-based 当前渲染页
  -> pageCount >= 1
  -> UI value = page + 1
  -> UI label = 第 UI value/pageCount 页

滑条 input(value)
  -> 只保存 1-based 草稿值并更新标签
  -> 不滚动、不切章、不写路由

滑条 change(value)
  -> clamp 到 1…pageCount
  -> flip: 设置当前列 page=value-1
  -> vertical: 定位到当前渲染滚动范围的对应页
  -> 保存当前阅读进度
  -> 清除草稿
  -> currentIndex/Reader 路由保持不变
```

竖向定位采用当前 Vue 3 渲染窗口的 `scrollHeight/clientHeight` 与 `pageCount` 做稳定映射，
这是对上游 document scroll 的技术栈适配；不能退化为全书目录跳转。

## 实施前测试

1. 替换 `readerProgressControls` 中“移动全书 seek”测试：
   - 1-based 值/标签；
   - input 不移动；change 在 flip 与 vertical 内定位；
   - 不调用路由导航；工具层状态不参与。
2. `useReaderLayout` 测试覆盖 `scroll`、`scroll2` 的真实 `page/pageCount`，并证明没有改变
   `overflow` 或原生滚动行为。
3. 源码合同证明移动滑条范围不是 `0…1000`，音频隐藏，底部中间没有章节标题。
4. 390×844、360×800 生产浏览器：
   - 长正文初始标签为 `第 1/N 页` 且 `N>1`；
   - input 只改标签；change 到末页后仍是原章节/原路由且滚动到对应末端；
   - 返回第 1 页；工具层一直可见，正文左右留白不变；
   - 单行 `阅读进度: N%` 可打开/关闭内联缓存区。
5. 1440×900 桌面章节百分比控件保持不变；音频真实浏览器确认没有移动页码滑条。

## 允许差异

- 全书进度可继续包含章内比例，比上游仅按章节整数计算更平滑，但只能用于可见百分比和
  缓存入口，不能继续作为移动滑条的导航语义。
- 原生连续手指/滚轮滚动和点击分段翻页保持用户要求，不因补页码状态而改回分段滚动。

## 2026-07-17 实施与验证记录

- 删除只服务于错误移动跨章滑条的 `readerBookSeekTarget()` 与 `seekBookProgress()`。
- `useReaderProgressControls` 将移动页码草稿、1-based 标签和当前渲染页定位独立于桌面章内
  百分比；input 不移动，change 才定位并保存。
- `useReaderLayout` 为 `page/scroll/scroll2` 统一维护当前渲染页状态，没有修改原生
  `overflow`、touch 或 wheel 分支。
- `ReaderMobileChrome` 非音频显示 `第 x/y 页`；音频隐藏滑条并收缩为单行 footer；缓存
  入口恢复单行 `阅读进度: N%`。
- 合并动画/浏览器运行器修复后，全量前端 418 项测试、后端 `go test ./...` 和 Vite 生产构建通过。
- `reader-mobile-contract.mjs` 在 1440×900、390×844、360×800 验证页码 input/change、
  不切章、不改路由、到顶/底、工具层与正文几何。
- `reader-audio-contract.mjs` 在 390×844、360×800、1440×900 验证音频不显示文本页码
  且移动 footer 为单行；`reader-continuous-contract.mjs` 验证 scroll/scroll2 连续跨章未回归。
- 旧 smoke 直接启动系统 Chrome 会在 macOS GUI 注册阶段 `SIGABRT`；现已统一改用
  Playwright Chromium Headless Shell。移动、连续和音频合同连续独立启动均通过，断言未放宽。
