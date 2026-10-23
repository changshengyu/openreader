# Reader 正文加载失败错误面盘点（P0）

状态：R7 **implementation-complete / regression-validated**（红测 `e497358`、实现 `a6af2d6`，见第 11 节）；R1–R6、R8 仍为
**contract-only**，D1 仍待用户决定。盘点提交 `0427dd4` 只写文档；第 4 节的真实浏览器复现用的是会话 scratchpad 里的临时探针，
没有入库，其中第 4.2、4.3、4.5 节的按钮命中与真实指针部分已由入库的 `scripts/smoke/reader-error-surface-contract.mjs` 承接。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。约定路径
`/private/tmp/changshengyu-reader-dev` 在本会话沙箱里不可写，上游克隆到会话 scratchpad 后 checkout 同一提交。  
当前基线：`OpenReader@15eda53`（本分支从 `5bc38a3` 用 `--ff-only` 快进到 `claude/jovial-wilson-889933`）；
`git diff 186c762 15eda53 -- frontend backend` 为空，Reader 产品代码与已发布的 `186c762` 相同。本文所有 OpenReader 行号都以
`15eda53` 为准。桌面计数删除（`035762d`）和标签页标题实现（`4a0aae8`）都不在这个基线里；`claude/xenodochial-carson-5004c3`
整条链合入后，`Reader.vue` 第 149–428 行减 1、第 548–1424 行加 1，其余行号不变，内容不变。

## 1. 范围

- 对象：Reader 当前章正文加载失败后，正文区显示的“错误面”：
  - 主错误块 `.chapter-load-error`（`ReaderChapterContent.vue:4–7`）：文本 page、flip、scroll、scroll2 四种模式，EPUB、音频、CBZ
    的主加载失败，目录加载失败，EPUB 资源加载失败；
  - 连续模式相邻章失败的内联错误块 `.chapter-inline-error`（同文件 `:57–61`）；
  - 失败提示（toast）、两个重试入口（正文内“重新加载”、工具栏“重新载入章节”）的可达性与语义；
  - 错误面与点击翻页层 `.reader-tap-zones`、正文触摸点击判定、移动底栏页码进度的交互。
- 不含：运行标题（[运行标题合同](reader-running-title-load-failure-fixed-baseline-second-audit-p0-contract.md)，本文第 4 节
  只记录观察值）；标签页标题；失败状态下的进度载荷字段（另行盘点，见第 10 节第 2 条）；加载中提示；后端错误 envelope 与状态码。
- 起因：运行标题合同第 10 节第 1、2 条：1440×900 错误块的“重新加载”被点击层盖住；正文错误块没有章节 `<h3>`。

## 2. 上游证据（fa22f271）

### 2.1 正文失败：`web/src/views/Reader.vue#getContent`（L998–1119）

| 位置 | 行为 |
| --- | --- |
| L1000–1009 | 先 `show = false`，在阅读区盖 `$loading` 遮罩，文字 `正在获取内容`（刷新时 `正在刷新内容`）。 |
| L1038–1040 | 请求前把 `this.title` 写成目标章目录标题（运行标题合同第 2.2 节）。 |
| L1072–1085 | 接口返回 `isSuccess: false`：`content = "获取章节内容失败！\n" + errorMsg`，把这份错误内容写进内存章节缓存（`error: true`），`error = true`、`show = true`，关闭遮罩。 |
| L1090–1118 | 请求出错（网络、超时、HTTP 错误）：`content = "获取章节内容失败！\n" + error.toString()`，同样写缓存、`error = true`、`show = true`；L1110–1112 再弹 `$message.error("获取章节内容失败 " + error)`。 |
| L1086–1088、L1113–1115 | 两个失败分支在连续模式下都调用 `computeShowChapterList(true)`（见第 2.6 节）。 |
| L1023–1036 | 目录里没有这一行：先刷新一次目录，再不行就显示 `获取章节内容失败，请更新目录！`。OpenReader 把下标截断到目录范围，运行标题合同第 5 节第 13 行已判 `acceptable-change`，本文不重开。 |

`isSuccess: false` 时 `web/src/plugins/axios.js#request` 的默认分支（L123–142）还会按非 silent 请求弹出
`Message.error({ message: errorMsg, duration: 2000 })`；`App.vue#getBookContent`（L779–810）没有传 `silent`。所以上游两种正文
失败都会弹一次错误提示：业务失败弹服务端 `errorMsg`，请求出错弹 `获取章节内容失败 <错误>`。

### 2.2 错误页渲染：`web/src/components/Content.vue#render`（L25–98）

- `Reader.vue` L232–238：`<div class="content-inner" v-if="show"><Content :title="title" :content="content" :error="error" …>`。
- 非连续模式（L53–90）：`<h3 data-pos=0>{this.title}</h3>`（L62），然后把内容按换行拆成段落 `<p>`（L63–88）。错误页因此是
  “目标章标题 + `获取章节内容失败！` + 原因”三行，排版与普通章节相同。段落用 `domPropsInnerHTML` 输出（L79、L86）。
- `Content.vue` L129–146 与 `Reader.vue` L828–850：`isCarToon`、`isAudio`、`isEpub`、`isCbz` 都带 `!this.error`。错误时 EPUB、
  音频、CBZ 书都按普通文字页渲染同一个错误页，CBZ 也显示 `<h3>`。
- 底栏 `加载下一章`（`Reader.vue` L246–247）带 `!error`，错误页不显示。
- `Reader.vue` L391–404 `watch.content`：内容一变就 `currentPage = 1` 并 `computePages()`，错误页的页数按错误页自己的高度重算，
  mini 底栏 `第 x/y 页`（L118、`formatProgressTip` L1857–1859）随之变为错误页的页数。

### 2.3 重试

- 正文里没有重试按钮。重试入口是右侧浮动按钮 `el-icon-refresh-right`（`Reader.vue` L96–98，`.read-bar` 的
  `float-right-btn-zone`）：`refreshContent()`（L995–997）→ `getContent(index, true)`，请求带 `refresh=1`（`App.vue` L789–791），
  遮罩文字 `正在刷新内容`，成功后没有提示。
- 桌面右栏常驻；mini 界面的两侧工具栏只在 `showToolBar` 为真时显示（`leftBarTheme`/`rightBarTheme`，L636–658），默认为真，点中间区域切换。

### 2.4 错误页上的点击与触摸

- `.content`（L230–231）挂 `handleTouchStart/Move/End` 和 `handlerClick`；`handlerClick`（L1596–1604）只在 `isEpub` 时返回，而错误时
  `isEpub` 为假，所以错误页的点击照常进入 `eventHandler`（L1713–1792）：中间 ±20% 切换工具栏，其余按 `clickMethod` 和翻页方式
  上一页/下一页。
- `nextPage`/`prevPage`（L1427–1471、L1472–1513）只在 `!this.show` 时返回；错误页 `show` 为真，翻到头就是 `toNextChapter`/
  `toLastChapter`。也就是说错误页和普通页一样可以点击翻页、翻章、呼出菜单；上游错误页上没有任何可点击的控件。

### 2.5 失败提示（toast）小结

| 失败 | 页面 | 提示 |
| --- | --- | --- |
| 正文 `isSuccess: false` | 标题 + `获取章节内容失败！` + `errorMsg` | `errorMsg`（2s，axios 包装） |
| 正文请求出错 | 标题 + `获取章节内容失败！` + `Error: …` | `获取章节内容失败 Error: …`（默认 3s） |
| 相邻章（连续模式 `loadShowChapter`）`isSuccess: false` | 列表里的错误章 | `errorMsg`（axios 包装） |
| 相邻章请求出错 | 列表里的错误章 | 无 |
| 目录 `isSuccess: false`（初始化） | 空 `<h3>` + `获取章节目录失败！` + `errorMsg` | `errorMsg`（axios 包装） |
| 目录请求出错 | 正文区空白（`show` 仍为假，没有错误文字） | `获取书籍目录列表 Error: …` |

### 2.6 连续模式（上下滚动、上下滚动2）

- 目标章失败：`getContent` L1042–1044 先把 `scrollStartChapterIndex` 设为目标章；失败分支把错误内容写进缓存后调用
  `computeShowChapterList(true)`（L1249–1324）。列表从目标章开始，缓存里的错误章直接入列，缺少的后一章用 `loadShowChapter`
  补齐后重算，所以页面是“失败章（`<h3>` + 错误文字）+ 后一章正文”，后一章可以照常往下读（`Content.vue#renderScrollChapterList`
  L199–266，L231 每章一个 `<h3>`）。
- `saveReadingPosition`（L2488–2490）在 `this.error` 为真时直接返回：上游失败后滚进后一章也不切换 index、标题和进度，直到下一次
  成功的 `getContent`。这属于进度规则，本文不裁决（第 10 节第 2 条）。
- 相邻章失败（`loadShowChapter` L1163–1224）：错误内容写进缓存并显示为列表里的一章；没有按钮。OpenReader 的内联错误块和
  明确重试已由[连续合同](reader-continuous-fixed-baseline-p0-contract.md)第 2 节“错误/重试”判 `acceptable-change`。

### 2.7 目录失败

`loadCatalog`（L927–963）：初始化时 `isSuccess: false` → `title = ""`、`content = "获取章节目录失败！\n" + errorMsg`、`error = true`、
`show = true`（L946–952）；请求出错只弹 `获取书籍目录列表 …`（L956–961），正文区保持空白。重试走目录面板的刷新
`refreshCatalog()` → `loadCatalog(true)` → 成功后 `getContent(index)`。工具栏刷新在这时会读 `readingBook.catalog[index]`，目录
不存在时抛错（上游缺陷，不复现）。

### 2.8 上游可见行为小结

1. 当前章失败后，正文区是一个“错误章节页”：目标章标题 + `获取章节内容失败！` + 原因，排版同普通章节；EPUB、音频、CBZ 也一样。
2. 每次当前章失败弹一次错误提示。
3. 重试只有工具栏刷新按钮，重新请求同一章（`refresh=1`），成功后没有提示。
4. 错误页本身没有控件，整页按普通页的点击分区响应（上一页/下一页/菜单）；`加载下一章` 隐藏。
5. 页码进度按错误页重算。
6. 连续模式的失败章留在列表里，后一章照常加载，可以继续往下读。
7. 目录失败：业务失败显示 `获取章节目录失败！` 加原因、标题为空；请求出错只有提示。

## 3. 当前 OpenReader（15eda53）

### 3.1 主错误块

- `ReaderChapterContent.vue:2–7`：`loading` 时 `正在加载章节...`；否则 `error` 非空时只渲染
  `<div class="chapter-load-error"><p>{{ error }}</p><button type="button" @click="emit('reload')">重新加载</button></div>`。
  没有章节标题，没有 `获取章节内容失败！`；样式 `:307–319`、`:339–347`（居中网格，`min-height: 180px`），按钮没有定位和 `z-index`。
- 主错误块优先于 `blocks`、EPUB 和音频分支，所以四种文本模式、连续模式的目标章失败、EPUB/音频/CBZ 主加载失败、目录失败、
  `handleEpubError`（`Reader.vue:2231–2234`）都显示同一个块。
- `Reader.vue:161`、`:176`：`:error="chapterLoadError"`、`@reload="reloadChapter"`。加载器失败分支
  `useReaderChapterLoader.js:128–134` 只设置 `chapterLoadError`、清空资源，不改 `chapter`、`content`、`chapterBlocks`、
  `chapterFormat`、`pageCount`，也不调用 `updateLayout()`。
- 失败时不弹提示：加载器、`useReaderRouteSync.js:18–25` 和 `Reader.vue:2033–2036`、`:2100–2103` 的书籍加载失败回调都只写
  `chapterLoadError`。

### 3.2 文案来源

- `Reader.vue:2462–2464` `readError(err, fallback)` 只读 `err.response.data.error.message` 或 `err.response.data.error`，否则用
  兜底；加载器和连续窗口的 `formatError` 兜底是 `章节加载失败，请检查书源或网络后重试`（`:1074`、`:1822`），书籍/目录失败兜底是
  `章节加载失败`（`:2034`、`:2101`）。
- 后端正文失败返回 `502 {"error":"failed to load chapter content","code":…,"stage":"content"}`（`backend/api/books.go:2623`，
  `source_errors.go:17–74` 只给稳定、脱敏的消息和 code，不回传书源 URL、规则或响应正文）；目录失败 `500 {"error":"failed to list
  chapters"}`（`books.go:2112`）。所以用户看到的是英文 `failed to load chapter content` / `failed to list chapters`。
- stale 409 自动重取一次后仍冲突时，`useReaderChapterContent.js:213–218` 抛出 message 为 `章节状态已更新，请重试` 的 Error，但
  `readError` 不读 `Error.message`，页面显示通用兜底。[请求生命周期合同](reader-chapter-content-request-lifecycle-fixed-baseline-second-audit-p0-contract.md)
  第 9 节第 5 条要求持续冲突显示 `章节状态已更新，请重试`；当时的单元测试只断言组合函数抛出的 message（`811d679`），浏览器合同
  只断言英文内部消息不可见（`2bbb276`），都没有断言页面文字。

### 3.3 点击层、触摸判定与按钮

- `ReaderClickZones.vue:42–48`：`.reader-tap-zones` `position: absolute; inset: 0; z-index: 2`；文档滚动模式（page、scroll、scroll2）
  改为 `position: fixed`（`:50–58`）。只有 `(hover: hover) and (pointer: fine)` 时显示（`:184–211`）：page/scroll/scroll2 是上、下各 30%
  高的整宽 `tap-upper`/`tap-lower`，flip 是左右各 30%；`tap-center` `pointer-events: none`。`max-width: 750px` 时整层隐藏（`:213–217`）。
- `Reader.vue:208–215`：`v-if="chapterFormat !== 'epub' && !isAudioChapter"`。`chapterFormat` 初值 `'text'`（`:585`），只在成功加载时改写，
  所以 EPUB 书第一次加载就失败时点击层仍在，前一章是 EPUB 时则没有。
- 点击层的按钮排在正文之后、`z-index: 2`，而错误块按钮是普通流元素，桌面上落在上/下 30% 带里的按钮被盖住。
  `useReaderPointer.js:79–98` `handleTapZone('upper')` → `readerTapZoneAction`（`readerInteraction.js:62–78`）→ `previous` →
  `previousPage()`（`useReaderNavigation.js:210–227`）：错误页滚动量不足 8px，于是 `goChapter(currentIndex - 1, READER_CHAPTER_END_OFFSET)`。
- 触摸：`Reader.vue:139–144` 在 `.reader-page` 上挂 touch 和 click。`handleContentClick`（`useReaderPointer.js:100–116`）会跳过
  `button, a, input…`，但 `handleTouchEnd`（`:154–204`）不看触摸目标，短按一律走 `tapPoint()`：中间 ±20% 切换工具层，其余上一页/
  下一页，并隐藏工具层。主错误块按钮没有任何 `.stop`；内联错误块按钮只有 `@click.stop`（`ReaderChapterContent.vue:60`），拦不住
  `touchend`。对照：章末 `加载下一章`（`Reader.vue:196–205`、`:2552–2567`）是 `position: relative; z-index: 3` 加
  `@click.stop @touchstart.stop @touchend.stop`，[移动点击合同](reader-mobile-page-click-p0-contract.md)第 78 行要求“按钮拦截 touch/click 穿透”。

### 3.4 连续模式

- 目标章失败走主加载器，显示主错误块，整个窗口不渲染（错误优先），后一章不显示。
- 相邻章失败：`useReaderChapterWindow.js:68–80` `errorBlockFor` 用目录行标题，`content` 是 `获取章节内容失败！\n<原因>`，但模板只渲染
  `block.error`（原因），没有 `获取章节内容失败！` 这一行；`maybeExtend`（`:246–256`）在末块出错时暂停扩章；`retry`（`:223–244`）
  以 `refresh: true` 原位替换。

### 3.5 重试

- 工具栏：`ReaderDesktopTools.vue:43` 桌面右栏 `.round-tool[title="重新载入章节"]`，`ReaderMobileChrome.vue:44` 移动右侧浮动按钮，
  都发 `reload` → `Reader.vue:2005` → `reloadChapter`。
- `useReaderChapterMaintenance.js:39–47` `reloadChapter`：`loadChapter(currentIndex, currentOffset(), { refresh: true })`，成功后提示
  `章节已重新载入`。正文请求 `getChapterContent`（`frontend/src/api/books.js:221–226`）不带 refresh 参数，`refresh: true` 只绕过前端
  内存和浏览器章节缓存（第 10 节第 1 条）。
- 目录失败时 `chapters` 为空，加载器把目标下标截断成 0（`useReaderChapterLoader.js:20–24`），`reloadChapter` 不会重新请求目录。

### 3.6 页码进度

`useReaderProgressControls.js:19`、`:28`：移动底栏 `第 x/y 页` 的最大值来自 `pageCount`，失败时没有重算，沿用上一章的页数。

## 4. 真实浏览器复现（2026-10-17）

### 4.1 环境

- 产品代码：`15eda53` 的 frontend 生产构建（`npm ci`、`npm run build`）和用 Go 1.26.9 编译的 backend 二进制，独立的临时
  data/cache/library 目录和随机 JWT secret，监听 `127.0.0.1:18447`。会话沙箱禁止 loopback 监听（`bind: operation not permitted`），
  沙箱内也连不上沙箱外的 loopback，服务和 Playwright 都在沙箱外运行。
- Playwright 1.61.1（根目录 devDependency），chromium-headless-shell 1228。
- 探针（会话 scratchpad，未入库；副本和报告见共享笔记目录 `chapter-error-surface/`）：
  - 文本：与 `scripts/smoke/reader-continuous-contract.mjs` 相同的 `page.route` 模拟（假 token，`/ws/sync` 中止），8 章夹具
    `0 第一章 起点`…`4 第五章 前邻`、`5 第六章 失败`、`6 第七章 后续`、`7 第八章 终点`。先打开 `?chapter=0`，再用路由跳到第 5 行；
    正文接口返回后端真实的失败体 `502 {"error":"failed to load chapter content","code":"content_unavailable","stage":"content"}`。
  - EPUB：真实服务注册测试账号，导入与 `reader-epub-contract.mjs` 同形的夹具（目录 `0 封面`、`1 第一章（下）`、`2 第二章`），只拦截
    第 2 行的正文请求（500）；每一步用全新浏览器上下文。
  - 输入：1440×900 用鼠标（默认上下文，细指针）；390×844、360×800 各跑两种上下文：`isMobile + hasTouch` 用
    `page.touchscreen.tap`（真机路径），默认上下文用 `page.mouse.click`（现有连续合同的路径）。“真实指针”都按按钮中心坐标点击，
    不用 `dispatchEvent`；`elementFromPoint` 记录命中元素。
  - `addInitScript` 用 MutationObserver 记录 `.el-message` 与 `.reader-toast` 的出现。
- 视口 1440×900、390×844、360×800；文本模式 page、flip、scroll、scroll2。1440×900 上 flip 设置按 page 渲染（壳类为
  `page document-scroll`，桌面没有左右滑动），所以桌面实际是三种布局。

### 4.2 主错误块：结构与命中（3 视口 × 4 模式，结构完全相同）

| 视口 | 按钮中心 | `elementFromPoint` | 其他 |
| --- | --- | --- | --- |
| 1440×900（page/flip/scroll/scroll2） | (720,157) | `button.tap-zone.tap-upper`；点击层 `fixed`、`z-index: 2`，上带 `320,0 800×270`，下带 `320,630 800×270` | 按钮 `position: static`、`z-index: auto` |
| 390×844 | page (195,158)、flip (195,143)、scroll/scroll2 (195,128) | 按钮本身（点击层 `display: none`） | 工具层默认可见 |
| 360×800 | page (180,158)、flip (180,143)、scroll/scroll2 (180,128) | 按钮本身 | 工具层默认可见 |

- 20 组的页面文字都是 `failed to load chapter content` 加 `重新加载`；没有 `<h3>`；1.5s 内没有 `.el-message`。
- 移动底栏页码在四种模式下都是 `第 1/9 页`，即第 0 章的页数（截图 `main-390x844-*-failed.png`、`main-360x800-*-failed.png`）。
- 运行标题都是 `第 6 章 第一章 起点`，属运行标题合同第 5 节第 3 行，本文不再判定。

### 4.3 主错误块：源恢复后用真实指针按“重新加载”

| 视口 | 输入 | 模式 | 结果 |
| --- | --- | --- | --- |
| 1440×900 | 鼠标 | page、flip、scroll、scroll2 | 点到 `tap-upper` → 路由变成 `?chapter=4&offset=-1`，显示第五章 前邻末尾（文档滚动 3516/3333px）；请求 `[4,5,3,6]`；进度 `PUT` `4:第五章 前邻`；没有重新加载第 5 行 |
| 390×844、360×800 | 触摸 | page、flip | 同上：`?chapter=4&offset=-1`、`PUT 4:第五章 前邻`；工具层被隐藏；没有重新加载 |
| 390×844、360×800 | 触摸 | scroll、scroll2 | 错误块仍在；第 5 行没有请求；文档从 30px 滚到 0（`previousPage` 的上翻动画）；工具层被隐藏；进度 `PUT` `5:第一章 起点`（新序号 + 旧章节名） |
| 390×844、360×800 | 鼠标 | 四种模式 | 正常：路由仍是 `?chapter=5`，显示第六章 失败；请求 `[5,6,4,7,3]`（第 5 行加预载）；提示 `章节已重新载入`；没有进度写入 |

只有“窄视口 + 鼠标”这一种组合能重试成功。真机（触摸）和桌面（细指针）上，按“重新加载”要么把读者送到上一章末尾并把进度
存成上一章，要么只翻了一下页、存了一条错误进度。

### 4.4 工具栏“重新载入章节”

1440×900 鼠标 × 4 模式、390×844 与 360×800 触摸 × 4 模式共 12 组全部一样：按钮命中自身；路由保持 `?chapter=5`；请求
`[5,6,4,7,3]`；显示第六章 失败；提示 `章节已重新载入`。与上游刷新按钮的重试语义一致（成功提示的差异见第 10 节第 1 条）。

### 4.5 连续模式内联错误块（第 1 行失败，从第 0 行打开）

- 结构（10 组相同）：`<h3>第二章 后邻</h3>`、`failed to load chapter content`、`重新加载`；没有 `获取章节内容失败！`；没有提示。
- 把按钮滚到视口 12%、50%、88% 处测命中（1440×900 文档到底，12% 实际只能到 y=285）：

| 视口 | 输入 | 命中 | 按钮在下带时按下（源仍失败） | 按钮在中间时按下（源已恢复） |
| --- | --- | --- | --- | --- |
| 1440×900 | 鼠标 | y=285、450 命中按钮；y=792 命中 `tap-lower` | 没有请求；文档 3752→4259px（下翻一页） | 请求 `[1]`，错误块被正文替换 |
| 390×844、360×800 | 触摸 | 三处都命中按钮 | 没有请求；文档下翻约 335px；工具层被隐藏 | 请求 `[1]`，替换成功，但同时切换了工具层（隐藏→显示） |
| 390×844、360×800 | 鼠标 | 三处都命中按钮 | 请求 `[1]`（再次失败，块保留） | 请求 `[1]`，替换成功 |

现有 `reader-continuous-contract.mjs` 的 `runFailureRetry` 只在 390×844 用鼠标点按钮，所以一直通过。

### 4.6 文案变体（page 模式，3 视口相同）

| 注入 | 页面文字 | 提示 |
| --- | --- | --- |
| 网络失败（`route.abort`） | `章节加载失败，请检查书源或网络后重试` | 无 |
| 两次 `409 {"error":"chapter content changed; retry"}`（自动重取一次） | `章节加载失败，请检查书源或网络后重试`（不是 `章节状态已更新，请重试`） | 无 |
| `502 {"error":"fixture chapter failure"}` | `fixture chapter failure` | 无 |

### 4.7 目录失败（`?chapter=2` 进入，`/books/1/chapters` 返回 `500 {"error":"failed to list chapters"}`）

- 3 视口：页面 `failed to list chapters` 加 `重新加载`，没有 `<h3>`，没有提示，没有正文请求；运行标题 `第 3 章`（运行标题合同 R2
  已覆盖）；1440×900 按钮被 `tap-upper` 盖住，390/360 命中按钮。
- 目录恢复后触发按钮（用 `dispatchEvent`，排除点击层影响）：目录没有重新请求；只请求第 0 行正文并显示 `第一章 起点`，路由仍是
  `?chapter=2`；运行标题 `第 1 章 第一章 起点`；桌面上一章/下一章都禁用（目录为空）；提示 `章节已重新载入`。

### 4.8 EPUB（真实服务）

| 场景 | 视口/输入 | 点击层 | 命中 | 按下“重新加载”（源恢复） |
| --- | --- | --- | --- | --- |
| 第 1 行 iframe 内链到第 2 行，第 2 行 500（`chapterFormat` 已是 `epub`） | 1440×900 鼠标 | 不渲染 | 按钮 | 正常：第 2 行 iframe 载入，运行标题 `第 3 章 第二章`，提示 `章节已重新载入` |
| 同上 | 390×844、360×800 触摸 | 不渲染 | 按钮 | `touchend` 走翻页：`?chapter=1&offset=-1`，显示 `第一章（下）`，没有重新加载第 2 行 |
| 直接打开 `?chapter=2`，第 2 行 500（`chapterFormat` 仍是初值 `text`） | 1440×900 鼠标 | 渲染（page、`fixed`） | `tap-upper` | `?chapter=1&offset=-1`，显示 `第一章（下）` |
| 同上 | 390×844、360×800 触摸 | 隐藏 | 按钮 | `?chapter=1&offset=-1`，工具层被隐藏 |

6 组页面文字都是 `fixture EPUB chapter failure` 加 `重新加载`，没有标题，没有提示。

### 4.9 其他记录

- 服务日志 1794 个请求：1628 个 200、3 个 201（注册与导入）、163 个 401，401 全部是模拟上下文的 `/ws/sync`（假 token，
  `page.route` 拦不到 WebSocket）；没有 5xx（500/502 都是浏览器侧注入的），没有 panic。
- 浏览器控制台只有注入失败对应的 46 条 502、12 条 500 资源错误，没有页面异常。

## 5. 差异分类

| # | 场景 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | 当前章失败的错误页标题（四种文本模式、EPUB/音频/CBZ 主加载失败、EPUB 资源失败） | `<h3>` 目标章目录标题 | 没有标题（4.2、4.8） | **`must-fix`** |
| 2 | 错误页第一行 | `获取章节内容失败！` | 直接显示原因：英文服务端消息或中文兜底（4.2、4.6） | **`must-fix`** |
| 3 | 原因行来源 | 服务端原始 `errorMsg` 或 axios `error.toString()` | 后端脱敏后的稳定消息，无响应时中文兜底 | 来源 `acceptable-change`（安全加固：不回传书源异常原文）；英文稳定消息是否改为中文 **`unknown`，待用户决定 D1** |
| 4 | 自动重取后仍 stale | —（上游没有 409） | 显示通用兜底（4.6） | **`must-fix`**：请求生命周期合同第 9 节第 5 条已签收 `章节状态已更新，请重试`，现状不符 |
| 5 | 错误文字的渲染 | `domPropsInnerHTML` | 文本插值 | `acceptable-change`（安全：不把服务端文字当 HTML） |
| 6 | 失败提示 | 每次当前章失败弹一次（2.5） | 从不提示（4.2、4.6–4.8） | **`must-fix`**：正文 `获取章节内容失败 <原因>`，目录 `获取书籍目录列表 <原因>`；预载、相邻章失败、被取代或中止的请求不提示（上游请求出错分支对相邻章同样不提示，预载是运行时适配） |
| 7 | 正文内“重新加载”按钮（主错误块） | 没有，整页是点击分区 | 有（`cd4af25` 为修复加载卡住加入，没有用户要求的记录） | `acceptable-change`：沿用连续合同对“明确重试入口”的签收理由，与内联错误块一致；前提是第 8–10 行全部修好。用户若要求严格对齐上游，可改为删除该按钮、只留工具栏重试，其余条目不受影响 |
| 8 | 桌面细指针下按“重新加载” | （无按钮） | 被点击层盖住，点击变成上一页 → 跳到上一章末尾并写进度（4.2、4.3、4.7、4.8） | **`must-fix`（P0）**：OpenReader 自有按钮必须在点击层之上 |
| 9 | 触摸轻点“重新加载” | （无按钮） | `touchend` 照常做点击判定：page/flip 跳上一章末尾并写进度；scroll/scroll2 上翻、隐藏工具层、写 `5:第一章 起点`，不重载（4.3、4.8） | **`must-fix`（P0）**：按钮必须拦截触摸穿透 |
| 10 | 内联错误块按钮（已签收的入口）的可达性 | （无按钮） | 桌面在上/下 30% 带被盖住；触摸在中间区外翻页不重试，在中间区重试但切换工具层（4.5） | **`must-fix`**：签收的是“明确重试”，入口必须可用且不附带翻页或工具层切换 |
| 11 | 内联错误块文字 | 列表里的错误章：标题 + `获取章节内容失败！` + 原因 | 标题 + 原因（4.5） | **`must-fix`**：连续合同第 1 节已记录上游 `获取章节内容失败！` 错误块，签收的只是重试入口 |
| 12 | 连续模式目标章失败 | 失败章留在列表（标题 + 错误文字），后一章照常加载（2.6） | 主错误块占满正文，后一章不显示 | **`must-fix`** |
| 13 | 工具栏重试 | 刷新按钮 → 同一章 `refresh=1` | “重新载入章节” → 同一章，前端缓存绕过（4.4） | `aligned`（对失败章等价：失败结果不写前端或服务端正文缓存，重试一定重新抓取；失效书源缓存只过滤搜索与换源候选，不拦正文请求）；成功提示与服务端 refresh 语义见第 10 节第 1 条 |
| 14 | 目录失败的页面 | 业务失败：空标题 + `获取章节目录失败！` + 原因 + 提示；请求出错：只有提示、正文空白 | 英文原因或 `章节加载失败`，无提示（4.7） | **`must-fix`**：统一为 `获取章节目录失败！` + 原因、无章节标题、弹 `获取书籍目录列表 <原因>`。不复现上游“请求出错时正文空白”：`acceptable-change`（始终给出可重试的错误页） |
| 15 | 目录失败后重试 | 目录面板刷新 → `loadCatalog(true)` → `getContent(index)`；工具栏刷新抛错 | “重新加载”只按截断后的第 0 行请求正文，`?chapter=2` 下显示第一章，目录仍为空（4.7）；目录面板刷新（本地书）只更新目录、不重载正文（`useReaderToc.js:69–83`、`useReaderChapterMaintenance.js:4–24`，读代码） | **`must-fix`**：重试必须重新加载目录并定位到路由/进度目标章。不复现上游工具栏刷新抛错：`acceptable-change` |
| 16 | 错误页页码进度 | 按错误页重算（2.2） | 沿用上一章 `第 1/9 页`（4.2） | **`must-fix`** |
| 17 | 错误页上按钮以外的点击/触摸 | 普通点击分区：上一页/下一页/菜单 | 点击层和 `tapPoint()` 照常工作 | `aligned`，保留为回归断言 |
| 18 | `加载下一章` | 错误时隐藏 | `showChapterEndPrompt` 要求 `!chapterLoadError` | `aligned` |
| 19 | EPUB/音频/CBZ 失败时的点击层 | 错误页按文字页处理，点击分区可用 | 是否渲染点击层取决于上一次成功加载留下的 `chapterFormat` | 只要第 8–10 行的按钮要求满足，点击层是否渲染不改变可见结果：不单独判定 |
| 20 | 加载中提示 | `$loading` 遮罩 `正在获取内容`/`正在刷新内容` | `正在加载章节...` | 已签收（运行标题合同第 6 节），不重开 |
| 21 | 失败章进缓存 | 内存缓存记录 `error: true` 的章节，下次成功时替换 | 失败不进内存或浏览器缓存 | `acceptable-change`（缓存加固）；可见结果由第 12 行规定 |

没有一项判为 `intentional-redesign`。第 7 行不是新的设计，而是把连续合同已签收的理由用到同一组件里的同类按钮上。

## 6. 已签收、不重开

- 运行标题的格式、派生与切换时机（运行标题合同，含其第 6 节列出的已签收项）；本文只引用它的观察值。标签页标题合同 R1–R6 与 D1。
- 连续模式相邻章失败的内联错误块与明确重试入口（连续合同第 2 节“错误/重试”`acceptable-change`）；自动扩章在错误块处暂停；
  重试受 generation 约束。本文只补它的文字（第 11 行）和可达性（第 10 行）。
- 点击翻页区设计：`ReaderClickZones` 的 30/40/30 分区、只在细指针桌面渲染、移动端隐藏、zone 与触摸点到动作的映射
  （`readerTapZoneAction`、`readerTapPointAction`），原生连续手指/滚轮滚动 + 点击仍分页（用户要求的 UX 改进）。本文只要求
  OpenReader 自有按钮位于点击层之上并拦截穿透，不改分区、映射和设备差异。
- 章末 `加载下一章` 入口及其穿透拦截（移动点击合同）。
- 加载器 generation/abort/scope，stale 409 自动重取一次及按书串行（请求生命周期合同第 9 节）；本文只要求页面文字按该合同显示。
- 预载半径 2、预载静默；正文区 `正在加载章节...`；音频媒体错误文案 `音频加载失败，请检查书源或网络后重试`（音频合同）。
- 后端 API、状态码、错误 envelope 和脱敏消息；数据库、浏览器存储 key、持久文件；镜像与生产。本切片都不改，不在本地构建或推送
  镜像，不做任何生产操作。

## 7. 目标合同

R1. 错误章节页的标题：当前章主加载失败（四种文本模式的非连续显示、EPUB/音频/CBZ、EPUB 资源加载失败）时，正文区第一项是
  `<h3 data-pos="0">`，文字取 `currentIndex` 对应的目录行，规则与同一章正常显示时的 `<h3>` 相同，并与运行标题合同 R1、标签页
  标题合同 R1 取同一行。目录行未知（目录失败、目录为空）时不显示章节标题文字，也不显示猜测的序号。

R2. 失败文字：标题之后依次是第一行 `获取章节内容失败！`（目录失败时为 `获取章节目录失败！`）和第二行原因（R3）。连续模式的内联
  错误块同样是“标题 + 两行文字”。文字以纯文本渲染，不用 `v-html`。错误块的居中排版可以保留。

R3. 原因：来源不变（服务端 `error` 字段；没有响应时用现有中文兜底），只改一处：stale 409 自动重取后仍冲突时显示
  `章节状态已更新，请重试`。英文稳定消息是否改成中文由 D1 决定，决定之前保持原样；无论结果如何，不回传书源原始异常文字。

R4. 失败提示：每次当前章主加载失败（属于当前事务，不是中止或被取代）弹一次 `ElMessage.error`：正文 `获取章节内容失败 <原因>`，
  目录 `获取书籍目录列表 <原因>`。重试再次失败时再弹一次。预载失败、连续模式相邻章失败、被取代或中止的请求不弹。

R5. 重试语义：工具栏“重新载入章节”与正文内“重新加载”都重新加载当前章（保持现状：同一下标、绕过前端缓存，成功后显示该章正文）。
  目录失败时，两者都重新执行整本书的加载：重新请求目录，再按路由或已存进度定位目标章；不得把下标截断成 0 后在 `?chapter=N` 下
  显示另一章。目录面板“刷新”在目录失败状态下成功后，同样接着加载目标章。

R6. 连续模式目标章失败：窗口以目标章为锚点，目标章显示为内联错误块（R1 标题、R2 文字、“重新加载”），后一章按连续合同的窗口
  规则照常加载、可以继续阅读；重试只原位替换该块。跨入后一章之后当前章如何切换、失败章是否写进度，按连续合同和失败态进度载荷
  盘点的结论执行，本文不规定。

R7. 按钮可达与穿透隔离：主错误块和内联错误块的“重新加载”在 1440×900（细指针，page/scroll/scroll2，含按 page 渲染的 flip 设置）、
  390×844 与 360×800（触摸和鼠标，四种模式）、任意滚动位置都必须是命中目标（`elementFromPoint` 为按钮或其后代）。按下（鼠标单击
  或触摸轻点）只做一件事：重新加载该章。不得触发点击层动作，不得触发正文 `touchend`/`click` 的翻页或工具层切换，不得改变路由、
  滚动位置和工具层显隐，在重新加载完成之前不得写入指向其他章或带旧章节名的进度；失败章的正文请求只发一次（成功后的预载不计）。
  错误页上按钮以外的区域保持现有点击分区（第 5 节第 17 行）。实现可参照章末 `加载下一章` 的定位与 `.stop` 组合，不强制。

R8. 页码进度：失败后重新计算布局，移动底栏页码按错误页本身显示（错误页一屏时为 `第 1/1 页`），不得沿用上一章的页数。

R9. 不变：第 6 节列出的已签收项；`加载下一章` 在错误时隐藏；不新增 API、数据库字段、浏览器存储 key 或持久文件；后端不改。

决定点：

- **D1（unknown）**：原因行和提示里的原因是否把后端英文稳定消息改成中文。可选：(a) 保持原样；(b) 按后端 `code`/HTTP 状态映射成
  中文短句，未知时用中文兜底；(c) 只显示中文兜底，不显示服务端消息。建议 (b)：后端 `code` 本来就是为稳定的客户端合同设计的
  （`source_errors.go:17–20` 注释），也与 stale 文案的已签收做法一致。D1 不阻塞 R1、R2、R4–R8 的红测与实现。

## 8. 测试计划（后续阶段执行，本切片不写测试）

### 8.1 单元合同（`node --test`）

1. `frontend/tests/readerChapterErrorSurface.test.mjs`（新增，源码合同，写法同 `readerAudioContent.test.mjs`）：
   - `ReaderChapterContent.vue` 主错误块含 `<h3 data-pos="0">` 绑定标题、含 `获取章节内容失败！` 或由标签属性给出的两行文字，按钮带
     `@click.stop`、`@touchstart.stop`、`@touchend.stop`；`.chapter-load-error button` 与 `.chapter-inline-error button` 都有定位且
     `z-index` 大于 `ReaderClickZones.vue` 中 `.reader-tap-zones` 的值（从两个文件里解析数字比较，不写死）；
   - 内联错误块的按钮同样带三个 `.stop`，文字含 `获取章节内容失败！`；
   - 两处都不使用 `v-html` 渲染错误文字。
2. `frontend/tests/readerChapterErrorText.test.mjs`（新增纯函数，例如 `frontend/src/utils/readerChapterError.js`）：服务端 `error`
   字符串、`error.message` 嵌套、无响应兜底、`ChapterStaleConflictError` → `章节状态已更新，请重试`、目录类型的标签与提示前缀；
   D1 决定后再补映射表用例。
3. `frontend/tests/readerChapterLoader.test.mjs`：失败时调用一次 `notifyError`（或等价回调），文字为 `获取章节内容失败 <原因>`；
   `AbortError` 和被取代的请求不调用；失败后调用一次 `updateLayout`（R8）。
4. `frontend/tests/readerPointer.test.mjs`：`handleTouchEnd` 收到目标在 `button`（或带拦截标记的元素）内的短按时，不调用
   `previousPage`、`nextPage`、`toggleChrome`，也不隐藏工具层；普通正文短按的现有用例不变。
5. `frontend/tests/readerChapterMaintenance.test.mjs` 与 `readerBookLoad.test.mjs`：目录为空（书籍加载失败）时 `reloadChapter`
   重新执行书籍加载、不以截断后的 0 调用 `loadChapter`；目录面板刷新在失败状态下成功后加载目标章。
6. `frontend/tests/readerChapterWindowController.test.mjs`（或 `readerChapterWindow.test.mjs`）：连续模式目标章失败后窗口为
   `[错误块(i), 正文块(i+1)]`，错误块带目录标题；`retry(i)` 原位替换。
7. 红测写法：新模块在测试体内 `await import(...)`，把导入失败转成断言消息，让每条测试以明确的断言失败。预期第 1–6 项在当前
   实现上全部失败。

### 8.2 真实浏览器

1. 新增 `scripts/smoke/reader-error-surface-contract.mjs`（模拟接口，写法同 `reader-continuous-contract.mjs`）。视口 1440×900（鼠标）、
   390×844 与 360×800（`isMobile + hasTouch` 用 `touchscreen.tap`，另加默认上下文用鼠标），模式 page、flip、scroll、scroll2；夹具与
   注入同第 4.1 节。步骤与断言：
   - E1 结构：路由到失败章后，非连续模式错误页 `<h3>` 等于 `第六章 失败`，文字含 `获取章节内容失败！`；出现一次 `.el-message--error`，
     文字以 `获取章节内容失败` 开头；没有 `加载下一章`；移动底栏 `第 1/1 页`。连续模式见 E5。
   - E2 命中：按钮中心 `elementFromPoint` 是按钮。
   - E3 真实指针：源恢复后用鼠标/触摸按按钮：失败章只多一个请求，路由仍是 `?chapter=5`，工具层显隐不变，没有进度写入指向第 4 行
     或带 `第一章 起点`，最后显示第六章 失败。
   - E4 工具栏：现状行为保持（回归）。
   - E5 连续模式：目标章失败时 `.chapter-content[data-index="5"] .chapter-inline-error` 含标题和两行文字，`data-index="6"` 有正文；
     相邻章失败时把内联按钮放到视口 12%/50%/88% 处，命中都是按钮；88% 处按下（源仍失败）只多一个请求、滚动位置和工具层不变；
     恢复后在 50% 处按下替换成功、工具层不变。
   - E6 文案：网络失败显示中文兜底；两次 stale 409 显示 `章节状态已更新，请重试`。
   - E7 目录失败：`?chapter=2` 进入，目录 500：`获取章节目录失败！`、没有章节标题文字、提示 `获取书籍目录列表`；恢复后真实指针按
     按钮：目录重新请求，`?chapter=2` 下显示 `第三章 平稳`。
   - 预期红点（按第 4 节实测）：E1 在 12 组都失败（没有 `<h3>`、没有标签、没有提示，移动 `第 1/9 页`）；E2 在 1440×900 命中
     `tap-upper`；E3 在 1440×900 与触摸 page/flip 得到 `?chapter=4&offset=-1` 和 `PUT 4:第五章 前邻`，触摸 scroll/scroll2 没有请求且
     `PUT 5:第一章 起点`；E5 目标章失败是主错误块、没有第 6 行，88% 处 1440×900 命中 `tap-lower`、触摸下翻不重试；E6 stale 显示通用
     兜底；E7 目录没有重新请求、显示第一章。逐视口、逐模式记录精确失败点。
2. `scripts/smoke/reader-epub-contract.mjs`（真实服务，3 视口）：增加两个独立上下文的失败步骤——第 1 行内链到第 2 行失败、直接打开
   第 2 行失败；断言错误页标题 `第二章` 与标签，真实指针（桌面鼠标、移动触摸）按按钮后重新加载第 2 行、路由与工具层不变、不跳到
   `第一章（下）`。当前预期：触摸两种场景和桌面首次失败都跳到 `?chapter=1&offset=-1`。
3. `scripts/smoke/reader-continuous-contract.mjs` 的 `runFailureRetry`：补触摸上下文（`touchscreen.tap`）和 1440×900 下带位置的按钮；
   现有鼠标用例保留。当前预期：触摸和 1440×900 下带位置失败。

### 8.3 回归与临时变异（实现阶段）

- frontend `npm test`（写明新增条数）、`npm run build`；真实浏览器跑 8.2 全部脚本，另跑 `reader-mobile-contract`、
  `reader-text-modes-contract`、`reader-reauthentication-contract`、`openreader-smoke` 作对照。
- 临时变异只在工作区修改并构建到独立目录，构建后立即还原，不提交：
  - M1：去掉主错误块按钮的定位或 `.stop` → E2/E3 在 1440×900 或触摸上失败；
  - M2：去掉内联按钮的 `.stop` → E5 触摸 88% 用例失败；
  - M3：去掉错误页 `<h3>` → E1 失败；
  - M4：去掉失败提示 → E1 提示断言失败；
  - M5：`reloadChapter` 在目录失败时改回 `loadChapter(currentIndex)` → E7 失败；
  - M6：stale 错误文字改回通用兜底 → E6 失败；
  - M7：连续模式目标章失败改回主错误块 → E5 失败；
  - M8：失败后不重算布局 → E1 移动页码断言失败。
- 跳过并写明原因：Go `test`/`vet`/`race`（后端不改）；依赖 advisory、Compose 和卷门（依赖、Dockerfile、数据格式都不改）。D1 若
  选择在后端改消息，则另行补后端合同与测试。
- 会话沙箱禁止 loopback 监听，服务和 Playwright 需要在沙箱外运行。

## 9. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交（本切片）。
2. **排期**：运行标题实现（R1“当前目录行”）、桌面计数（`035762d`/`9839270`）与标签页标题（`377e8b8`/`4a0aae8`/`2528d9c`，
   `claude/xenodochial-carson-5004c3`）都改 `Reader.vue`；本文 R1 的标题可以和运行标题、标签页标题共用同一个“当前目录行”计算属性。
   建议在它们之上切分支。失败态进度载荷盘点会和 R6、R7 的“不写错误进度”交叉，先看它的结论再定 R6 的进度细节。
3. **红测**：8.1 第 1–6 项、8.2 第 1–3 项，记录每个视口、模式和输入的精确失败点。提交 `test: expose reader chapter error surface gaps`。
4. **实现**：按风险拆成可独立验证的提交，每个都让对应断言转绿：
   - `fix: keep reader retry buttons above tap zones and touch handling`（R7，P0，最小改动；已完成：红测 `e497358`、实现
     `a6af2d6`，见第 11 节）；
   - `fix: render upstream chapter error page with title, label and toast`（R1–R4、R8）；
   - `fix: retry the catalog when the reader book load failed`（R5）；
   - `fix: keep failed target chapters in the continuous window`（R6）。
5. **回归**：按 8.3 执行，包括临时变异。
6. **记录**：提交号、各视口/模式/输入的结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
7. **发布**：实现提交改动 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；是否合入由用户决定。D1 由
   用户决定，不阻塞前面各步。

## 10. 观察（不在本切片）

1. 刷新正文的成功语义：OpenReader“重新载入章节”成功后提示 `章节已重新载入`（初始提交 `131d3f8` 起就有，没有用户要求的记录），
   上游刷新成功不提示；OpenReader 的正文请求不带 refresh 参数（`frontend/src/api/books.js:221–226`，后端 `chapterContent` 也不读），
   `refresh: true` 只绕过前端缓存，而上游 `refresh=1` 让服务端绕过缓存重新抓取。对失败章两者等价（失败结果不写正文缓存），对已成功
   加载的章节不等价。需要另行盘点“刷新正文”。
2. 失败态进度：第 4.3 节触摸 scroll/scroll2 的一次误触产生了 `PUT /progress` `5:第一章 起点`（新序号 + 旧章节名），是运行标题合同
   第 10 节第 3 条推断的第一个实测样本；1440×900 与触摸 page/flip 的误触则把进度存成上一章末尾。R7 修好后误触路径消失，但载荷
   本身的问题仍需失败态进度载荷盘点裁决。上游失败后 `saveReadingPosition` 直接返回（2.6）。
3. EPUB 内链跳到另一章后路由仍是进入时的 `?chapter=1`（第 4.8 节第一行重载后显示第 2 行），属于 EPUB 路由同步，不在本文范围。
4. 桌面工作区面板打开时露出的正文按钮（2026-10-18 R7 实施时实测，见第 11 节“附带影响”第 3 条）：面板外是一层全屏透明的
   `.reader-workspace-dismiss`（`ReaderDesktopWorkspacePanel.vue`，`z-index: 3`，DOM 在正文之前）。章末 `加载下一章` 与 R7 之后的
   两个“重新加载”都是 `z-index: 3` 且在 DOM 中更靠后，露在面板下方时点中的是按钮：执行按钮动作，面板不关。上游四个桌面面板是
   `el-popover trigger="click"`（`Reader.vue` L9–55），没有遮罩：点面板外的控件时控件照常执行，面板也随之关闭；正文的点击分区在
   面板打开时由 `eventHandler` 直接返回。需要另行裁决这类控件在面板打开时应该怎样响应。
   2026-10-23 补记：已在[桌面面板正文控件合同](reader-desktop-panel-content-controls-fixed-baseline-second-audit-p0-contract.md)
   逐面板实测并裁决：两个“重新加载”与 `加载下一章` 判 `must-fix`，目标是执行动作并关闭面板，不改 R7 的按钮层级（contract-only）。

## 11. R7 实施与验证（2026-10-18）

只做 R7（按钮可达与穿透隔离）。R1–R6、R8 与 D1 没有动：第 4.2 节的标题、文字和页码，第 4.6–4.7 节的文案与目录重试，第 3.4 节
的连续模式目标章失败，在 `a6af2d6` 上与盘点时相同。

| 提交 | 阶段 | 内容 |
| --- | --- | --- |
| `0427dd4` | 合同 | 本文件、gap analysis 与 matrix（盘点）。 |
| `e497358` | 红测 | 8.1 第 1 项中属于 R7 的部分（两个按钮的三个 `.stop`、定位与 `z-index`）和第 4 项；8.2 第 1 项的 E2、E3 与 E5 的相邻章部分（新增 `scripts/smoke/reader-error-surface-contract.mjs`）；8.2 第 3 项。 |
| `a6af2d6` | 实现 | 见下。 |

分支 `claude/heuristic-payne-80b7a6` 从 `5bc38a3` 用 `git merge --ff-only claude/elastic-albattani-d2ccc7` 快进到 `0427dd4` 后开工；
`git diff 186c762 0427dd4 -- frontend backend` 为空，红测基线就是已发布的 Reader。`git diff 0427dd4 a6af2d6 -- backend` 为空。

**实现**（产品代码只改两个文件）：

- `ReaderChapterContent.vue`：主错误块按钮 `@click` 改为 `@click.stop`，并加 `@touchstart.stop @touchend.stop`；内联错误块按钮在已有
  `@click.stop` 上加同样两个触摸 `.stop`。两个按钮共用一条 `position: relative; z-index: 3`，取值同章末 `加载下一章`
  （`.reader-chapter-end`）：高于 `.reader-tap-zones` 的 2，低于 `.reader-page::after` 调暗层的 4，调暗时按钮仍和正文一起变暗。
  触摸修饰符不带 `.prevent`，轻点照常合成 click。按钮的外观没有改。
- `useReaderPointer.js`：`handleTouchEnd` 的短按分支在触摸目标位于 `button, a, input, textarea, select, [role="button"]` 内时不再调用
  `tapPoint()`；`handleContentClick` 改用同一个选择器常量，行为不变。横向滑动翻页、长按选择、点击抑制计时不变。两个按钮的触摸
  事件已被 `.stop` 截在按钮上，这条判断是第二层保护，并对 `.reader-page` 里的其他控件同样生效（见“附带影响”）。
- 没改：点击层的 30/40/30 分区、zone 与触摸点到动作的映射、只在细指针桌面渲染与移动端隐藏、`加载下一章`、工具栏重试、加载器、
  API、数据和存储 key。

**单元**（`node --test`，沙箱外）：

| 代码 | 结果 |
| --- | --- |
| `e497358` | 793 条中 788 通过；新增 5 条全部失败：`chapter-load-error 重新加载 click is missing .stop`；`.chapter-load-error button needs a position, got undefined`；`chapter-inline-error 重新加载 is missing @touchstart.stop`；`.chapter-inline-error button needs a position, got undefined`；`page tap on BUTTON at 150,60 reached the page actions`（实际 `[['previous']]`）。 |
| `a6af2d6` | 793/793（新增 5 条：`readerChapterErrorSurface.test.mjs` 4 条、`readerPointer.test.mjs` 1 条）。 |

沙箱内会多出 3 条 `dependencyAdvisoryLifecycleContract` 失败（app-client worker `listen EPERM 127.0.0.1`），与本切片无关。
`npm run build` 通过，只有既有的 Rolldown `#__PURE__` 注释警告。

**浏览器**：环境同第 4.1 节（本 worktree 构建的 go1.26.9 二进制加 `vite build` 生产前端，每个服务独立的临时 data/cache/library 与
随机 JWT secret（未输出），Playwright 1.61.1 / chromium-headless-shell 1228，Node v26.4.0，服务与浏览器都在沙箱外）。红测基线和
修复各自构建到独立目录、各起一个服务。`reader-error-surface-contract.mjs` 每个用例跑完全部检查再汇总；“12%” 位置受文档底部限制，
记录的是实际 y：

| 视口 / 输入 | 主错误块 page · flip · scroll · scroll2（E2、E3） | 内联 scroll · scroll2（E5） |
| --- | --- | --- |
| 1440×900 鼠标 | 红：4 组 FAIL。按钮中心 (720,157) 命中 `button.tap-zone.tap-upper`；按下后请求 `[4,5,3,6]`（page 为 `[4,5,3,6,2]`），路由 `?chapter=4&offset=-1`，进度 `PUT 4:第五章 前邻`。绿：命中按钮；请求 `[5,6,4,7,3]`，路由 `?chapter=5`，无进度写入，显示第六章 失败 | 红：2 组 FAIL。88%（y=792）命中 `tap-lower`，按下没有请求，文档 3752→4259px；12%（只能到 y=285）与 50% 命中按钮并重试。绿：三处命中按钮；12%、88% 按下（源仍失败）各只请求 `[1]`，滚动不变；50% 按下（源恢复）请求 `[1]` 并原位替换 |
| 390×844 触摸 | 红：4 组 FAIL。page/flip 跳到 `?chapter=4&offset=-1`，工具层隐藏，`PUT 4:第五章 前邻`；scroll/scroll2 不重载（请求 `[3]` 或 `[]`），工具层隐藏，`PUT 5:第一章 起点`。绿：请求 `[5,6,4,7,3]`，路由不变，工具层保持显示，无进度写入 | 红：2 组 FAIL。12%（只能到 y=406，落在中间区）重试但隐藏工具层；88%（y=742）没有请求，下翻 5733→6069px；50% 重试成功但工具层又切回显示。绿：三处命中按钮，三次按下各只请求 `[1]`，滚动与工具层不变 |
| 390×844 鼠标 | 红、绿都 PASS（与盘点一致：原来只有这种组合能重试） | 红、绿都 PASS |
| 360×800 触摸 | 同 390×844 触摸（按钮 (180,158)/(180,143)/(180,128)） | 同 390×844 触摸（12% 只能到 y=372，88% 为 y=704，下翻 5762→6094px） |
| 360×800 鼠标 | 红、绿都 PASS | 红、绿都 PASS |

合计：`e497358` 上 18/30 FAIL，正好是 1440×900 鼠标与两种手机触摸的全部用例；`a6af2d6` 上 30/30 PASS。

`reader-continuous-contract.mjs`：原有 4 视口 × 2 模式、原有 390×844 鼠标重试和进度事务在两份代码上都通过。新增变体在红测基线上
`390x844 touch at 88%` FAIL（没有重试，`scroll 10838 -> 11175`，`tools true -> false`），`1440x900 mouse at 88% (lower tap zone)`
FAIL（`(720,792)` 命中 `button.tap-zone.tap-lower`）；修复后全部通过。

对照（`a6af2d6`）：`reader-mobile-contract`、`reader-text-modes-contract`、`reader-audio-contract`、`reader-epub-contract`（真实服务、
3 视口）、`reader-reauthentication-contract`（4 视口）和 `openreader-smoke`（3 视口）全部通过。服务日志除注册、导入、书签的 201 外
都是 200，另有模拟上下文 `/ws/sync` 的 401（假 token，`page.route` 拦不到 WebSocket）；没有 5xx，没有 panic。

**临时变异**（只改工作区并构建到独立目录，构建后立即 `git checkout` 还原，未提交；每次还原后 `git status` 干净）：

| 变异 | 改动 | 单元 | 浏览器（只跑相关用例） |
| --- | --- | --- | --- |
| M1a | 主错误块按钮去掉定位与 `z-index` | `chapter-load-error 重新加载 sits above the desktop tap zones…` FAIL | `^(main\|inline) 1440x900`：主错误块 4 组 FAIL（E2 命中 `tap-upper`；E3 跳到 `?chapter=4&offset=-1`，`PUT 4:第五章 前邻`），内联 2 组 PASS |
| M1b | 主错误块按钮去掉三个 `.stop`，保留触摸判断 | `chapter-load-error 重新加载 stops click and touch…` FAIL | `^main` 20/20 PASS |
| M1c | M1b 再去掉 `handleTouchEnd` 的判断 | 上一条和 `short touch taps on reader buttons…` FAIL | `^main` 触摸 8 组 FAIL，与红测基线相同（page/flip 跳上一章末尾；scroll/scroll2 不重载，写 `5:第一章 起点`）；鼠标 12 组 PASS |
| M2a | 内联按钮去掉两个触摸 `.stop`，保留触摸判断 | `chapter-inline-error 重新加载 stops click and touch…` FAIL | `^inline` 10/10 PASS；连续合同 PASS |
| M2b | M2a 再去掉 `handleTouchEnd` 的判断 | 上一条和 `short touch taps…` FAIL | `^inline` 触摸 4 组 FAIL（12% 切换工具层，88% 下翻 5733→6069px 且不重试），其余 6 组 PASS；连续合同 `390x844 touch at 88%` FAIL |
| M2c | 内联按钮去掉定位与 `z-index` | `chapter-inline-error 重新加载 sits above…` FAIL | `^inline` 1440×900 两组 FAIL（88% 命中 `tap-lower`，下翻 3752→4259px），其余 8 组 PASS；连续合同 `1440x900 mouse at 88%` FAIL |
| MG | 只去掉 `handleTouchEnd` 的判断 | `short touch taps…` FAIL | `(390x844\|360x800) touch` 12/12 PASS |

合同 8.3 把 M1 写成“去掉定位或 `.stop`”、把 M2 写成“去掉内联按钮的 `.stop`”。去掉定位（M1a、M2c）时浏览器直接变红；只去掉
`.stop`（M1b、M2a）或只去掉触摸判断（MG）时，另一层仍然挡住触摸穿透（`handleContentClick` 本来就跳过按钮），浏览器合同不会变红，
由源码合同和 `readerPointer` 单元分别守住；两层都去掉（M1c、M2b）时浏览器重现红测基线的触摸红点。

**附带影响**（同一改动带来的、`.reader-page` 内其他控件的变化）：

1. 音频章节：手机触摸轻点播放器自己的按钮不再切换工具层。390×844、360×800 上播放键在中间区（195,410 / 180,410），修复前轻点会
   隐藏工具层，修复后不变；-15s/+15s 在中间区外，前后都不变（scratchpad 探针，`isMobile + hasTouch`，`touchscreen.tap`）。上游音频
   控件用 `click.stop.prevent`（`Content.vue` L316–359），音频模式下 `handleTouchStart` 直接返回，轻点控件从不切换菜单，所以这是向
   上游靠拢。`reader-audio-contract` 用鼠标，前后都通过。
2. 正文 HTML 里的链接等控件：触摸轻点只交给控件本身，不再同时翻页或切换工具层，与现有鼠标点击（`handleContentClick`）的规则一致
   （读代码，未单独实测）。
3. 桌面工作区面板（目录、书架、书源、设置）打开时，面板外有一层全屏透明的 `.reader-workspace-dismiss`（`z-index: 3`，DOM 在正文之前）。
   主错误块按钮在 y=157，被面板本身（底边 390–503px）盖住，前后一样；内联按钮在 88%（y=792）时露在面板下方，修复前命中的是关闭层
   （它的 `@click` 关闭面板，按钮不重试），修复后命中的是按钮（按钮 `@click.stop` 重试，面板不关）。章末 `加载下一章`（同为
   `z-index: 3`）在红测基线和修复后都命中按钮（同一探针，y=792）。以上是 `elementFromPoint` 命中测试加两个处理函数的代码，没有
   逐一点击。所以两个“重新加载”现在与已签收的章末入口一致；它们在面板打开时应该怎样响应，记为第 10 节第 4 条另行裁决，
   不在 R7 里改（改关闭层或面板的层级会同时改变章末入口以及面板与左右栏、进度条的叠放）。

**其余门**：Go `test`/`vet`/`race` 跳过（后端无改动）；依赖 advisory、Compose 与卷门跳过（依赖、Dockerfile、数据格式都没有改，卷门
只在 Actions 上运行）。实现提交改了 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布，是否合入由用户决定。

**状态**：Git 为本地分支 `claude/heuristic-payne-80b7a6` 上的提交，未推送，未合入 main。镜像：未构建，未推送。生产：未访问，未操作。

## 12. R7 发布记录（2026-10-18）

按用户“每段进度自动推送并构建”的要求，`refs/heads/claude/heuristic-payne-80b7a6` 以合并提交
`238e3f3357a4d99d0f804c6458ffd835dfa091f5` 进入本分支并快进 main（非 force，ls-remote 核验）；冲突只在四份文档，
逐块保留两侧条目，代码自动合并。发布前本地门：frontend 810/810 与 build；本地 Go 服务（临时数据）加生产前端跑
openreader-smoke、错误面合同 30 例、连续阅读、document-title、EPUB、重新认证四视口、text-modes、音频、移动端（含 iPad），
全部通过，访问日志无 5xx（401 只来自 mock 合同伪造 token 的 `/ws/sync`）。后端与锁文件相对已验证的 `2528d9c` 无改动，
本地未重跑 Go，由 Actions 执行。可信 Actions `39125862052` 成功（19m34s，两平台 OS 门 pass-with-unknown-origins，新旧卷与
portable 通过）。`ghcr.io/changshengyu/openreader:238e3f3` 与 `latest` 指向 index
`sha256:f45c8995a47c7ce696c07600533e620bf68b5b4e8df77d684539bb823afca4cf`（amd64
`sha256:d11f20f9c94682c922692596c6a03e4cb839b3ca064e310e2fe276061a196060`，arm64
`sha256:79086bcd96c3e5447c379199e2805744864778ab92142cc6eaf0114c4644f77e`）。按 digest 独立核验两平台：`/app/public`
63 个文件与本地同一提交的 build 逐字节一致；zlib 1.3.2-r1、SQLite 3.51.3 与完整 revision 标签保持。

状态：R7 **released / awaiting-device-verification**；R1–R6、R8 仍 contract-only，D1 待用户决定。生产未访问，实际运行版本未知。
