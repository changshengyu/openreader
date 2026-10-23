# Reader 桌面工作区面板打开时的正文控件盘点（P0）

状态：**contract-only**。本切片只写文档，产品代码和测试都没有改。第 2.4 节、第 4 节的真实浏览器复现用的是会话 scratchpad 里的
临时探针，没有入库；探针和原始输出的副本在共享笔记目录 `panel-content-controls/`。红测与实现按第 9 节在后续阶段进行。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。约定路径
`/private/tmp/changshengyu-reader-dev` 在本会话沙箱里不可写，用的是另一个会话 scratchpad 里已 checkout 同一提交的克隆
（工作树干净）。  
盘点基线有两份，第 4 节分别实测：

- `OpenReader@98f4ddb`：R7 之前。`git diff 2528d9c 98f4ddb -- frontend backend Dockerfile` 为空，产品代码就是上一次发布的
  `2528d9c`。
- `OpenReader@0962dca`：本 worktree 分支 `claude/busy-shirley-8d0038` 在 `98f4ddb` 上普通合并 R7 分支
  `claude/heuristic-payne-80b7a6`（`e497358`/`a6af2d6`/`d3603b8`）得到。它和 main 上的合并提交 `238e3f3` 父提交相同，
  `git diff 238e3f3 0962dca` 为空。`238e3f3` 已由可信 Actions `39125862052` 发布为 `ghcr.io/changshengyu/openreader:238e3f3`
  与 `latest`（index `sha256:f45c8995a47c7ce696c07600533e620bf68b5b4e8df77d684539bb823afca4cf`，见错误面合同第 12 节），所以这一份
  就是现行发布的 Reader。之后 main 到 `a7fd6aa` 没有改 `frontend/src`，只有依赖、构建、CI 与安全修复；本文没有在 `a7fd6aa` 上重测。

本文 OpenReader 行号以 `0962dca` 为准。`Reader.vue`、`ReaderDesktopWorkspacePanel.vue`、`ReaderDesktopTools.vue`、
`ReaderDesktopProgress.vue`、`ReaderClickZones.vue` 在两份基线上完全相同；`ReaderChapterContent.vue` 在 `98f4ddb` 上的写法另行标出。

## 1. 范围

- 对象：桌面场景（`isMobileReader` 为假，即宽度大于 750 的自适应模式，含 iPad 1366×1024、1024×1366），四个工作区面板
  （书架、书源、目录、设置，共用 `ReaderDesktopWorkspacePanel`）打开时，按到露在面板外、仍然可见的正文控件：
  - 章末 `加载下一章`（`Reader.vue:195–204`，`.reader-chapter-end`）；
  - 主错误块的“重新加载”（`ReaderChapterContent.vue:4–7`）；
  - 连续模式内联错误块的“重新加载”（`ReaderChapterContent.vue:57–61`）。
- 对照组：面板外的正文空白处（关闭层）、右栏工具、桌面进度块的上一章/下一章、另一个面板的入口。这些只记录现状，用来确定规则；
  本切片不对它们提出改动。
- 不含：移动端（mini 界面）的主面板与关闭层；点击层的分区与映射；R7 的按钮层级和 `.stop`；键盘；音频、EPUB 内的控件；
  正文 HTML 里的链接。
- 起因：[错误面合同](reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md)第 10 节第 4 条、第 11 节“附带影响”
  第 3 条。R7 实施时只用 `elementFromPoint` 做了命中测试，读了两个处理函数，没有逐一点击。

## 2. 上游证据（fa22f271）

### 2.1 四个桌面面板是没有遮罩的 click popover

- `web/src/views/Reader.vue` L9–55：书架、书源、目录、设置各是一个
  `el-popover placement="right" trigger="click" :visible-arrow="false" v-model="…Visible"`，引用元素是左栏的 `.tool-icon`。
- `web/package.json` L15 `"element-ui": "^2.15.9"`、L22 `"vue": "^2.6.10"`，仓库没有 lockfile。按这两个范围，现在解析到
  element-ui 2.15.14 和 vue 2.7.16（`npm view`）。下面的行号取自 element-ui 2.15.14 的 npm 包：
  - `packages/popover/src/main.vue` L117–119：`trigger === 'click'` 时，在引用元素上监听 `click` 切换显示，同时在 `document` 上
    监听 `click`，处理函数是 `handleDocumentClick`；
  - 同文件 L193–207 `handleDocumentClick`：点击目标不在 popover 自身、引用元素或弹出层里时，`showPopper = false`；
  - `src/utils/dom.js` L22–26：`on` 就是 `addEventListener(event, handler, false)`，监听在冒泡阶段；
  - `src/utils/vue-popper.js` L42–45、L99：弹出层默认挂到 `body`；L116、L125 用 `PopupManager.nextZIndex()` 设层级。
    弹出层之外没有任何遮罩。
- 结论：面板打开时，按面板外的任何地方，事件先交给被按的元素处理，再冒泡到 `document`，面板才关闭。被按的元素如果在自己的
  处理函数里阻止冒泡，事件到不了 `document`，面板就不关。

### 2.2 正文点击在面板打开时直接返回

`.content`（L230–231）的 `handlerClick`（L1596–1604）调用 `eventHandler`（L1713–1733）。四个面板任一打开时，
`eventHandler` 直接返回；EPUB 下还会顺手把面板都关掉。触摸路径 `handleTouchEnd` 最终也走 `eventHandler`。所以面板打开时，
按正文空白处只关闭面板，不翻页，也不切换工具栏。错误页（错误面合同第 2.4 节）和连续模式的失败章（第 2.6 节）都是正文文字，
也属于这一条。

### 2.3 面板外的控件

| 控件 | 位置 | 处理 | 面板打开时按下 |
| --- | --- | --- | --- |
| `加载下一章` | `Reader.vue` L246–247：`.bottom-bar` 里的 `span.bottom-btn`，在 `.content` 之外；`v-if="show && !isSlideRead && !error && !isScrollRead"` | `@click="toNextChapter()"`，不阻止冒泡 | 先执行 `toNextChapter()`（L1350–1374，非连续模式下是 `getContent(index + 1)`），再关闭面板 |
| 右栏浮动按钮：书签、搜索、信息、刷新、自动阅读、听书、夜间 | L79–110 | `@click`，不阻止冒泡 | 执行，面板关闭 |
| 右栏阅读进度、上一章、下一章 | L140–156 | 同上 | 执行，面板关闭 |
| 左栏首页、顶部、底部 | L56–73 | 同上 | 执行，面板关闭 |
| 另一个面板的入口 | L9–55 | 引用元素 | 打开那个面板，原面板关闭 |
| 音频控件 | `web/src/components/Content.vue` L316–359 | `vOn:click_stop_prevent`，阻止冒泡 | 执行，面板**不关**。这是正文里唯一阻止冒泡的控件 |

- `isSlideRead` 只在 mini 界面下为真（`web/src/plugins/vuex.js` L451–453，`Reader.vue` L586–594）。所以桌面上“上下滑动”
  和“左右滑动”都显示 `加载下一章`，“上下滚动”“上下滚动2”不显示（`isScrollRead`，L595–603）。
- 正文里没有“重新加载”按钮（错误面合同第 2.3、2.4 节）。

### 2.4 真实浏览器复现（popover 机制）

上游前端的依赖没有锁定，仓库里也没有构建产物，无法原样启动上游前端。复现的做法：用上面两个范围解析出的 vue 2.7.16 和
element-ui 2.15.14，把 `Reader.vue` 的相关片段原样搭成一个最小页面。片段包括：

- 四个 popover（L9–55），面板内容换成 340px 高的占位块；
- 左栏首页（L56–61），右栏刷新、下一章（L96–98、L151–156）；
- `.content` 及 `handlerClick`、`eventHandler` 的面板分支（L230–231、L1596–1604、L1720–1733）；
- `.bottom-bar` 里的 `加载下一章`（L241–248）；
- 按 `Content.vue` 写法带 `click.stop.prevent` 的音频按钮。

阅读宽度 800，`popperWidth` 为 767。1440×900 用鼠标，1366×1024 用触摸（`hasTouch`，`touchscreen.tap`）。每打开一个面板，
就按一次面板外的一个控件：

| 按下 | 结果（四个面板 × 两种输入，结果完全相同） |
| --- | --- |
| `加载下一章`（面板底边 371px，按钮在 y=848 / 972） | 命中按钮，执行 `toNextChapter`，面板关闭 |
| 正文空白处 | `eventHandler` 因面板打开而返回，没有翻页；面板关闭 |
| 带 `click.stop.prevent` 的音频按钮 | 执行，面板**不关** |
| 右栏刷新、右栏下一章、左栏首页 | 执行，面板关闭 |
| 另一个面板的入口 | 原面板关闭，另一个打开 |

复现只覆盖 popover 机制和这些模板片段。上游的触摸翻页处理（`handleTouchStart/End`）没有搬进来，正文空白处的触摸结果依据
第 2.2 节读代码。

### 2.5 上游可见行为小结

1. 面板打开时，按面板外的正文空白处（包括错误页和失败章的文字）：只关闭面板，不翻页，不切换工具栏。
2. 按面板外的控件：控件照常执行，面板随之关闭。只有自己阻止冒泡的控件（音频）执行后面板不关。
3. `加载下一章` 属于第 2 条：加载下一章，同时关闭面板。
4. 被面板盖住的地方，按到的是面板本身。

## 3. 当前 OpenReader

### 3.1 面板、关闭层与层级

- `ReaderDesktopWorkspacePanel.vue:2–8`：面板外是一个全屏透明按钮 `.reader-workspace-dismiss`（`:52–59`：
  `position: fixed; inset: 0; z-index: 3`），绑定 `@pointerdown.stop` 和 `@click.stop="$emit('close')"`。面板本身
  `.reader-desktop-workspace`（`:61–84`）是 `fixed`、`z-index: 4`。宽度不超过 750 时两者都隐藏（`:168–173`）。
- `Reader.vue:19–24` 把面板组件渲染在 `.reader-page`（`:135–145`）之前。`.reader-page`（`:2485–2501`）是
  `position: relative`，但没有 `z-index`，不形成层叠上下文。所以正文里带 `z-index` 的元素和关闭层在同一个层叠上下文里比较。
- 层级：点击层 `.reader-tap-zones` 为 2（`ReaderClickZones.vue:42–45`），关闭层 3，面板 4，左右栏 5（`ReaderDesktopTools.vue:111–116`、
  `:167–172`）。这四个值由 `readerDesktopPrimaryPopoverContract.test.mjs:60–66` 固定。另外，桌面进度块 `.reader-page-control`
  是 `fixed`、`z-index: 4`（`ReaderDesktopProgress.vue:62–67`），在 DOM 里排在关闭层之后；调暗层 `.reader-page::after` 为 4
  （`Reader.vue:2503–2510`，`pointer-events: none`）。
- 正文控件：
  - `加载下一章`：`Reader.vue:195–204`，绑定 `@click.stop="handleChapterEndNext" @touchstart.stop @touchend.stop`；样式
    `:2552–2567` 为 `position: relative; z-index: 3`。显示条件 `showChapterEndPrompt`（`:959–965`）：`effectiveReaderMode === 'page'`、
    已加载、没有错误、不是音频。桌面上“左右翻页”设置按 page 渲染，所以 page 和 flip 两种设置都显示这个按钮。
  - 两个“重新加载”：R7 之后（`0962dca`），`ReaderChapterContent.vue:6`、`:60` 都绑定 `@click.stop … @touchstart.stop @touchend.stop`，
    共用 `:340–343` 的 `position: relative; z-index: 3`。R7 之前（`98f4ddb`），主错误块是 `@click`（`:6`），内联是
    `@click.stop`（`:60`），两者都没有定位和 `z-index`。
  - 这些元素和关闭层同为 3，在 DOM 里更靠后，所以画在关闭层之上。面板（4）之外露出来的部分，按到的是控件本身。
- 处理函数：`handleChapterEndNext`（`:2409–2415`）调用 `goChapter(currentIndex + 1)`；主错误块走 `@reload="reloadChapter"`（`:175`）；
  内联错误块走 `@retry-block="retryContinuousChapter"`（`:193`）。三者都不碰面板状态。
- 已有“先关面板再执行”的函数：`runWithDesktopWorkspaceClosed`（`:1464–1467`，移动端不关任何面板）。左栏首页、顶部、底部，以及右栏
  重新载入章节、自动阅读、朗读、夜间、清缓存都经过它（`:1991–2016`）；面板之间的切换走 `openDesktopToolPanel`（`:1455–1462`）。
  不经过它的有：桌面进度块的上一章/下一章（`:228–229`），以及右栏书签、搜索、信息、笔记和缓存入口（`:1996–2003`）。这个函数是
  `ac27a5a`（2026-07-06，“align reader workspace with upstream”）加入的，没有单独的合同条目。

### 3.2 已签收的相关合同

- [iPad 面板关闭合同](reader-ipad-panel-dismiss-p0-contract.md)（2026-07-19）与[第二轮](reader-ipad-panel-fixed-baseline-second-audit-p0-contract.md)
  （2026-08-09）：外点关闭判为 `aligned technical equivalent`，层级 2/3/4/5 判为 `aligned`（理由：“外点不能翻页，工具仍可切换”）。
  第一份合同把上游证据写成：外点被 popover 消费，不得翻页、切换工具层或到达正文。它的浏览器检查
  （`reader-mobile-contract.mjs:2024–2053`）只在章首、面板下方 32px 的空白处轻点，从没有按到正文控件。
- [移动点击合同](reader-mobile-page-click-p0-contract.md)第 78 行（`加载下一章` 拦截 touch/click 穿透），以及错误面合同 R7（两个
  “重新加载”在点击层之上并拦截穿透）：都是面板关闭时的规则。

## 4. 真实浏览器复现（2026-10-19；一组补测于 2026-10-23）

### 4.1 环境

- 两份生产前端：`98f4ddb` 用 `git archive` 导出 `frontend`，借本 worktree 的 `node_modules` 构建；`0962dca` 在本 worktree 构建。
  两份都用 `vite build` 输出到 scratchpad。后端用本 worktree 编译的 go1.26.9 二进制（两份基线的后端相同）。每个服务有独立的临时
  data/cache/library 和随机 JWT secret（未输出），分别监听 `127.0.0.1:18471` 和 `:18472`。会话沙箱禁止 loopback 监听，Chromium
  在沙箱内也无法注册 mach port，所以服务和 Playwright 都在沙箱外运行。
- Playwright 1.61.1、chromium-headless-shell 1228、Node v26.4.0。
- 探针 `panel-controls-probe.mjs`：`page.route` 模拟接口，与 R7 面板探针相同（假 token、`/ws/sync` 中止）。夹具是 8 章
  `0 第一章 起点`…`7 第八章 终点`，失败章返回后端真实的失败体
  `502 {"error":"failed to load chapter content","code":"content_unavailable","stage":"content"}`。每组用一个全新的浏览器上下文：
  1. 打开 `?chapter=0`；主错误块场景再用路由跳到第 5 行。
  2. 需要时把控件中心放到视口 88% 处。连续模式的窗口稳定后可能回到阅读位置，所以放好后反复确认，打开面板后再确认一次。
  3. 按左栏入口打开面板。内联场景这时让第 1 行“恢复”，重试成功时内联块会被正文替换。
  4. 用真实指针按控件中心。1440×900 用鼠标（细指针，点击层显示）；iPad 1366×1024、1024×1366 用触摸（`hasTouch`、iPad UA、
     `deviceScaleFactor: 2`，与 `reader-mobile-contract.mjs` 相同；细指针媒体查询不成立，点击层隐藏）。
- 每组记录：`elementFromPoint` 命中、面板前后状态、路由、文档滚动、正文请求、显示的章节、内联块是否还在、进度写入、提示、
  目录面板当前项，以及 `document` 上捕获和冒泡阶段的 `click`、`touchend`。后两项用来判断事件有没有到达 `document`，有没有落在
  点击层或 `.reader-page` 里。
- 组数：首轮全量 136 组（两份基线 × 2 个视口 × 4 个面板 × 各场景；对照组只在 `0962dca` 上跑）。其中 9 组内联场景因窗口回到
  第 0 行开头而按到了视口之外，作废后用加了位置确认的探针重跑（32 组）。另加 1024×1366 的 40 组，其中 1 组（`98f4ddb`、书源、
  章末）等待选择器超时，单独重跑两次，结果与其余各组相同。

### 4.2 正文控件（面板先打开；四个面板结果相同；除注明外两份基线相同）

| 场景 | 视口 / 输入 | 命中 | 按下后 |
| --- | --- | --- | --- |
| 章末 `加载下一章`（page；flip 设置在桌面按 page 渲染，结果相同） | 1440×900 鼠标 | (720,792) 按钮 | 进入第二章：路由 `?chapter=0` → `?chapter=1`，滚动 1545 → 0，写一次指向第 1 行的进度。**面板不关**；目录面板随即把当前项换成 `第二章 后邻`。`click` 停在按钮上，没有到达 `document`，点击层没有收到 |
| 同上 | iPad 1366×1024 触摸 | (683,901) 按钮 | 同上（滚动 1436 → 0）；`touchend` 和 `click` 都停在按钮上 |
| 同上（只测 page） | iPad 1024×1366 触摸 | (512,1202) 按钮 | 同上（滚动 1135 → 0） |
| 内联“重新加载”（scroll、scroll2；第 1 行失败，按钮在 88%） | `98f4ddb`，三个视口 | 关闭层 `.reader-workspace-dismiss` | 面板关闭；**没有重试**（第 1 行没有请求，内联块还在）；滚动不变（1808 / 1727 / 1503） |
| 同上 | `0962dca`，三个视口 | 按钮 | 第 1 行请求一次，内联块原位替换成正文；滚动不变；没有进度写入；**面板不关**；事件停在按钮上 |
| 主错误块“重新加载”（page、scroll；1024×1366 只测 page） | 两份基线，三个视口 | 按钮中心 (720 / 683 / 512, 157) 落在面板里：书架是列表或空状态插图，书源是空状态插图，目录是当前章 `button.toc-item.active`，设置是 `div.settings-list` | 不可达，没有按。面板底边：书源 390，书架与目录 392，设置 503 / 559 / 713（依次对应三个视口） |

### 4.3 对照组

| 按下 | 基线与视口 | 结果 |
| --- | --- | --- |
| 面板下方的正文空白处（章首，水平居中，视口 88% 处） | 两份基线，三个视口 | 命中关闭层；面板关闭；路由、滚动不变；没有请求和进度写入；点击层没有收到。鼠标的 `click` 停在关闭层上；触摸的 `touchend` 冒泡到 `document`，但关闭层不在 `.reader-page` 里，正文的触摸判定不会运行 |
| 右栏“重新载入章节” | `0962dca`，1440×900 与 1366×1024 | 面板关闭；第 0 行重新请求一次；提示 `章节已重新载入`（滚动 0 → 90，见第 10 节第 5 条） |
| 桌面进度块“下一章” | 同上 | 进入第二章（路由 `?chapter=1`，写一次进度）；**面板不关**；目录面板随即高亮 `第二章 后邻`；`click` 到达 `document`（按钮没有 `.stop`） |
| 另一个面板的入口（目录打开时按书架，其余按目录） | 同上 | 原面板关闭，另一个打开 |

### 4.4 小结

- 两份基线上，章末 `加载下一章` 都是“执行，面板不关”。
- 内联“重新加载”：R7 之前（上一次发布 `2528d9c`）是“只关面板，不重试”；R7 之后（现行发布 `238e3f3`）是“只重试，面板不关”。
- 主错误块的按钮在三个视口都被面板盖住。
- 所有场景里，点击层和正文的触摸、点击判定都没有收到事件，没有翻页。
- 面板外空白处、右栏重新载入、面板切换与上游一致；桌面进度块的上一章/下一章和正文控件一样，“执行，面板不关”。

## 5. 差异分类

| # | 场景 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | 面板打开时按面板外的正文空白处 | 只关闭面板，不翻页，不切换工具栏 | 关闭层接住：只关闭面板（4.3） | `aligned`（iPad 合同已签收），保留为回归断言 |
| 2 | 按露出的 `加载下一章` | 加载下一章，面板关闭 | 加载下一章，面板不关（4.2） | **`must-fix`**：保留动作，同时关闭面板 |
| 3 | 按露出的内联“重新加载” | 没有这个按钮；失败章是正文文字，按到只关闭面板 | R7 之前只关闭、不重试；R7 之后（现行发布）重试、面板不关（4.2） | **`must-fix`**：重试并关闭面板（理由见下） |
| 4 | 主错误块的“重新加载” | 没有这个按钮；错误页被面板盖住 | 被面板盖住，不可达（4.2） | `aligned`：按到的是面板本身 |
| 5 | 点击层与正文的触摸、点击判定 | 面板打开时 `eventHandler` 直接返回 | 所有场景都没有收到事件（4.2、4.3） | `aligned`，保留为回归断言 |
| 6 | 右栏工具，左栏首页、顶部、底部 | 执行，面板关闭 | 右栏重新载入实测为执行并关闭；其余走同一个函数（3.1） | `aligned` |
| 7 | 另一个面板的入口 | 切换 | 切换（4.3） | `aligned` |
| 8 | 桌面进度块的上一章/下一章 | 执行，面板关闭（2.3、2.4） | 执行，面板不关（4.3） | **`must-fix`**，但它不是正文控件，不在本切片目标内，见第 10 节第 1 条 |

第 3 行在上游没有对应控件。判 `must-fix` 并以“重试并关闭”为目标，理由：

1. 错误面合同第 5 节第 7、10 行签收的是“明确的重试入口”，入口看得见时必须可用。R7 之前“按下没有反应，只关面板”不满足这一点。
2. 上游所有不阻止冒泡的面板外控件都是“执行并关闭”，`加载下一章` 也是（2.3、2.4）。同一个正文区里的两个“重新加载”应当和
   章末入口遵循同一条规则。“执行但面板不关”在上游只出现在音频控件上，原因是它阻止了冒泡，不能拿来当先例。
3. 是否保留这个按钮本身（严格对齐上游就是删掉它）已由错误面合同第 5 节第 7 行记录为用户的可选项，本文不重开。

没有判为 `intentional-redesign` 或 `unknown` 的项。

关于 iPad 合同的措辞：第一份 iPad 合同把上游证据写成“外点不得到达正文”。第 2.1–2.4 节表明，这句话对正文空白处成立
（`eventHandler` 返回），对正文控件不成立（控件照常执行）。本文只补“露出的正文控件”这一种情况；iPad 合同签收的关闭层、
同工具关闭、可见关闭按钮、层级和测试都不变。

## 6. 已签收、不重开

- 点击层：30/40/30 分区，只在细指针桌面渲染，移动端隐藏，zone 与触摸点到动作的映射（`readerTapZoneAction`、
  `readerTapPointAction`）；原生连续手指/滚轮滚动加点击仍分页（用户要求的 UX 改进）。
- R7：两个“重新加载”的 `position: relative; z-index: 3`、三个 `.stop`，以及 `handleTouchEnd` 对控件目标不做点击判定。
  `readerChapterErrorSurface.test.mjs` 要求按钮层级严格介于点击层（2）和调暗层（4）之间。章末 `加载下一章` 的层级与 `.stop`
  （移动点击合同第 78 行）。
- 桌面面板：关闭层只 emit `close`，同工具关闭，面板切换，44×44 可见关闭按钮，几何与内部 300px / 45vh 边界，层级 2/3/4/5
  （两份 iPad 合同，`readerDesktopPrimaryPopoverContract.test.mjs`）。
- 移动端（mini 界面）主面板、`.reader-mobile-primary-dismiss`（z-index 9）与工具条层级。
- `加载下一章` 的显示条件与末章提示 `本章是最后一章`；错误块的文字、标题、提示与重试语义（错误面合同 R1–R6、R8、D1）。
- 后端 API、数据库、浏览器存储 key、持久文件；镜像与生产。

## 7. 目标合同

P1. 适用范围：桌面场景（`isMobileReader` 为假），书架、书源、目录、设置任一工作区面板打开时。鼠标和触摸相同。

P2. 面板外的正文空白处：保持现状。只关闭面板，不翻页，不切换工具层，路由、滚动和进度都不变。

P3. 露在面板外的 `加载下一章`：一次按下既执行 `handleChapterEndNext`（有下一章时进入下一章，末章时提示 `本章是最后一章`），
  也关闭当前面板。结果与“先关面板、再按 `加载下一章`”完全相同。

P4. 露在面板外的“重新加载”：内联错误块按下时，既按错误面合同 R5–R7 重新加载该章（`retryContinuousChapter(index)`），也关闭
  当前面板。主错误块的按钮目前总被面板盖住（第 4.2 节）；以后布局变化使它露出时，规则相同（`reloadChapter`，并关闭面板）。

P5. 被面板盖住的正文控件不可达，按下由面板自己处理（现状）。

P6. 不变：

- 按下不得触发点击层、`.reader-page` 的触摸或点击判定、工具层切换；除了控件本身的动作和关闭面板，路由、滚动和进度不变
  （R7、移动点击合同）；
- 关闭层、面板、左右栏、进度块、调暗层、点击层和三个正文控件的 `z-index`、DOM 顺序与 `.stop` 都不改，按钮外观不改；
- 没有面板打开时，这些控件的行为不变；
- 移动端不变：移动主面板仍由它的关闭层（z-index 9）先接住按下；
- 不新增 API、数据库字段、浏览器存储 key 或持久文件，后端不改。

实现提示（不强制）：在 `Reader.vue` 里让这三个处理函数经过已有的 `runWithDesktopWorkspaceClosed`，写法同右栏“重新载入章节”
（`:2005`）。三个按钮都绑定了 `@click.stop`，点击到不了 `document`（第 4.2 节），所以不能靠文档级的外点监听来关闭面板。

不采用的做法：把关闭层抬到正文控件之上，或把控件压到关闭层之下。那样按下会变成“只关闭，不执行”，`加载下一章` 就偏离了上游；
而且要改 `readerDesktopPrimaryPopoverContract` 和 `readerChapterErrorSurface` 固定的层级，影响面板与左右栏、进度块、调暗层的叠放。

需要用户决定的事项：无。行为由上游证据确定（第 2 节）；实现不需要改关闭层或面板层级，所以“是否改关闭层或面板层级”不构成决定点。
第 10 节第 1 条的范围问题由协调方决定。

## 8. 测试计划（后续阶段执行，本切片不写测试）

### 8.1 单元合同（`node --test`）

1. 新增 `frontend/tests/readerDesktopPanelContentControls.test.mjs`（源码合同，写法同 `readerDesktopPrimaryPopoverContract.test.mjs`）：
   - `Reader.vue` 里 `.reader-chapter-end` 的 click 处理，以及 `<ReaderChapterContent>` 的 `@reload`、`@retry-block` 处理，都经过
     `runWithDesktopWorkspaceClosed`（或在同一个处理函数里先调用 `closeDesktopWorkspace`）；
   - `runWithDesktopWorkspaceClosed` 只在 `!isMobileReader` 时关闭面板（锁住现有实现，防止移动端被波及）；
   - 章末按钮仍是 `@click.stop @touchstart.stop @touchend.stop`。

   源码正则断言比较脆。另一种写法是把这几处包装提成一个小组合函数（例如 `useReaderDesktopWorkspace` 暴露
   `runWithDesktopWorkspaceClosed`），再写行为单元：调用后四个 `show*Drawer` 都为假，动作只被调用一次，移动端不关闭。二选一，
   由实现阶段决定。
2. 现有断言不动：`readerChapterErrorSurface.test.mjs`（`.stop` 与层级）、`readerDesktopPrimaryPopoverContract.test.mjs`
   （2/3/4/5、关闭层只 emit `close`）、`readerPageEndPrompt.test.mjs`。
3. 预期红点：当前实现上第 1 项全部失败（三处都直接绑定原处理函数）。

### 8.2 真实浏览器

新增 `scripts/smoke/reader-desktop-panel-controls-contract.mjs`（模拟接口，写法同 `reader-error-surface-contract.mjs`，步骤同第 4.1 节，
包括放置后的位置确认）。视口：1440×900 鼠标；iPad 1366×1024、1024×1366 触摸（`hasTouch`、iPad UA，同 `reader-mobile-contract.mjs`）。
每个视口 × 四个面板：

- C1 章末（page、flip 设置）：按钮在 88% 时命中按钮；按下后面板关闭，路由 `?chapter=1`，显示第二章，只写一次指向第 1 行的进度；
  点击层和 `document` 都没有收到这次按下。
- C2 内联（scroll、scroll2；第 1 行失败，打开面板后恢复）：按钮在 88% 时命中按钮；按下后面板关闭，第 1 行只请求一次并原位替换，
  滚动不变，没有进度写入。
- C3 主错误块（page、scroll）：面板打开时按钮中心命中面板内容；关闭面板后命中按钮（回归）。
- C4 对照：面板下方空白处只关闭面板（路由、滚动、进度不变，点击层没有收到）；右栏“重新载入章节”关闭并重载；另一个面板的入口
  切换面板。
- 预期红点（按第 4 节在现行发布代码上的实测）：C1 的 24 组（3 个视口 × 4 个面板 × 2 种模式）都在“面板关闭”断言失败，其中
  1024×1366 的 flip 设置本次没有实测；C2 的 24 组都在“面板关闭”断言失败。C3、C4 通过。逐视口、逐面板、逐模式记录精确失败点。

### 8.3 回归与临时变异（实现阶段）

- frontend `npm test`（写明新增条数）、`npm run build`；真实浏览器跑新合同，另跑 `reader-error-surface-contract`（R7 的 30 例）、
  `reader-continuous-contract`、`reader-mobile-contract`（iPad 外点、同工具、可见关闭，以及手机面板）、`reader-text-modes-contract`、
  `openreader-smoke`。
- 临时变异（只改工作区，构建到独立目录，构建后立即还原，不提交）：
  - M1：章末按钮去掉关闭包装 → C1 全红，其余不变；
  - M2：`@retry-block` 去掉关闭包装 → C2 全红；
  - M3：把关闭层改成 `z-index: 4` 并挪到 `.reader-page` 之后（“只关闭”做法）→ C1、C2 的动作断言失败，
    `readerDesktopPrimaryPopoverContract` 失败；
  - M4：`runWithDesktopWorkspaceClosed` 去掉移动端判断 → 8.1 第 1 项失败（移动端不在这份浏览器合同里）。
- 跳过并写明原因：Go `test`/`vet`/`race`（后端不改）；依赖 advisory、Compose 和卷门（依赖、Dockerfile、数据格式都不改）。
- 会话沙箱禁止 loopback 监听，Chromium 在沙箱内也起不来，服务和 Playwright 要在沙箱外运行。

## 9. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交（本切片）。
2. **范围确认**：第 10 节第 1 条（桌面进度块上一章/下一章）是否并入同一次实现，由协调方决定。并入时在 8.2 加 C5（按下后执行
   并关闭面板），预期同样全红。
3. **红测**：8.1、8.2，记录每个视口、面板和模式的精确失败点，提交 `test: expose desktop panels staying open over content controls`。
4. **实现**：`fix: close the desktop workspace when content controls run`，只改 `Reader.vue` 的三处绑定。
5. **回归**：按 8.3 执行，包括临时变异。
6. **记录**：把提交号、各视口/面板/模式的结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
7. **发布**：实现提交改了 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；是否合入由用户决定。

## 10. 观察（不在本切片）

1. 桌面进度块的上一章/下一章（`Reader.vue:228–229`）：上游执行并关闭面板（2.3、2.4），当前执行但面板不关（4.3），属于同一规则。
   它不是正文控件，不在本切片的简报范围里，修法与本文相同（经过 `runWithDesktopWorkspaceClosed`）。右栏书签、搜索、信息、笔记
   和缓存入口（`:1996–2003`）也不经过这个函数，但它们打开的是对话框或缓存区，面板和对话框怎样叠放要另看，本次没有实测。
2. 移动端：主面板打开时 `.reader-mobile-primary-dismiss`（z-index 9）盖住正文控件，按到 `加载下一章` 只会关闭面板（读代码，
   未实测）；上游 mini 界面同样是 click popover，`加载下一章` 执行并关闭。移动端面板本文不重开，需要时另行盘点。
3. 音频控件：上游是 `click.stop.prevent`，面板打开时执行，面板不关；OpenReader 的音频控件没有 `z-index`，面板打开时按下应当
   落在关闭层上（读代码，未实测）。音频不在本文范围。
4. 连续模式的阅读位置：探针把内联按钮放到 88% 后，窗口稳定的那一两秒里，有 9 组被拉回到第 0 行开头（文档滚动 72）。这是探针
   时序里的现象，没有定性；如果真实用户在加载刚结束时滚动也会被拉回，值得在连续模式合同里看一眼。
5. 右栏“重新载入章节”之后，文档滚动从 0 变成 90（第 4.3 节）。本文不裁决，归“刷新正文”盘点（错误面合同第 10 节第 1 条）。
