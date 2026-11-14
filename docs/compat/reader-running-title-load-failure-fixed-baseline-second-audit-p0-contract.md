# Reader 运行标题加载中与失败时的章节名盘点（P0）

状态：**implementation-complete / regression-validated**（2026-11-12，见第 11、12 节；分支 `claude/reader-error-title-progress-59bcb6`，
未合入 main）。原盘点只写文档，第 4 节的真实浏览器复现用的是会话 scratchpad 里的临时探针；第 12 节的红绿证据由入库的单元与浏览器合同取得。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。约定路径
`/private/tmp/changshengyu-reader-dev` 在本会话沙箱里不可写，上游克隆到会话 scratchpad 后 checkout 同一提交。  
当前基线：`OpenReader@c8ebd30`（本分支 `claude/jovial-wilson-889933` 从 `5bc38a3` 快进到
`claude/youthful-blackburn-9df7fb`）；`git diff 186c762 c8ebd30 -- frontend backend` 为空，Reader 产品代码与已发布的
`186c762` 相同。本文所有 OpenReader 行号（含测试和 smoke 脚本）都以 `c8ebd30` 为准。桌面计数删除 `035762d` 不在这个
基线里；它合入后，`Reader.vue` 行号在 L148 之后减 1、L1425 之后减 2，内容不变。

## 1. 范围

- 对象：文本与 EPUB Reader 页头的运行标题 `.reader-running-chapter-title`（`Reader.vue:146–149`）在以下时段显示的
  章节名：切章加载中、正文加载失败后、失败后重新加载中、被取代的请求迟到返回时，以及首次进入和 Reader 内换书的加载
  阶段。桌面和 mini interface 用的是同一个 `runningChapterLabel`，两边都在范围内。
- 导航入口：上一章/下一章控件，点击、按键、滑动翻到相邻章，目录，书签，正文搜索，路由 `chapter` 参数，进度恢复与
  同步，换源和目录刷新后的重载，替换规则刷新，错误块“重新加载”，EPUB 内链，自动翻页、朗读和音频进入下一章，连续滚动
  跨章与已加载窗口内跳转，Reader 内书架换书，远程临时阅读。
- 不含：标签页标题 `document.title`（见[标签页标题合同](browser-document-title-fixed-baseline-second-audit-p0-contract.md)，
  本文只要求两者的章节名同源）；正文错误面本身（错误文字、按钮、样式）和进度持久化载荷（见第 10 节观察）；音频与 CBZ
  （不渲染运行标题，`showRunningChapterHeader`）。
- 起因：[标签页标题合同](browser-document-title-fixed-baseline-second-audit-p0-contract.md)第 10 节的观察。当时读代码
  推断运行标题会在加载中和失败时显示“新序号 + 旧章节名”，没有复现，留给本盘点。

## 2. 上游证据（fa22f271）

### 2.1 顶栏与正文标题的来源

| 位置 | 行为 |
| --- | --- |
| `web/src/views/Reader.vue` L227–229 `.top-bar` | `{{ $store.state.miniInterface ? title : "" }}`：mini 显示 `this.title`，桌面为空（桌面空 top-bar 已由[桌面运行标注合同](reader-desktop-running-header-fixed-baseline-second-audit-p0-contract.md)裁决，不重开）。 |
| 同文件 L233 | `<Content :title="title" :error="error" :showContent="show" …>`：非连续模式的正文 `<h3>` 用同一个 `this.title`（`web/src/components/Content.vue` L62）；连续滚动列表里每章用自己的 `chapter.title`（L231）。 |
| 同文件 L335–343、L410–416 | 标签页标题也由 `this.title` 派生（标签页标题合同第 2 节）。 |

上游只有 `this.title` 这一个变量，mini 顶栏、非连续模式正文 `<h3>` 和标签页标题始终指向同一章。

### 2.2 `this.title` 的写入时机

| 位置 | 时机 | 写入 |
| --- | --- | --- |
| `init()` L870–912（L878） | 首次进入，或进入另一本书 | `""` |
| `loadCatalog()` L927–963（L946–949） | 初始化时目录加载失败 | `""`，正文显示“获取章节目录失败” |
| `getContent(index)` L998–1119（L1038–1040） | 每次调用 | 目标章目录标题 `catalog[index].title`。L1014 写 `readingBook.index`、L1040 写标题，都在请求正文（L1045）**之前**，中间没有 `await`。目录里没有这一行时（L1023–1036）提前返回，只写了 index |
| `epubLocationChangeHandler()` L1684–1712（L1703–1709） | EPUB iframe 内链跳到另一章 | 同一个同步块里先写 `readingBook.index`，再写该章目录标题；没有正文请求 |
| `saveReadingPosition()` L2488–2549（L2521–2531） | 连续滚动中可见段落所属章节变化（`scrollHandler` L2440 防抖 100ms） | 同一个同步块里写 `readingBook.index` 和该章目录标题；`this.error` 为真时（L2490）不执行 |

调用 `getContent` 的入口：目录（`PopCatalog.vue` L158 → `Reader.vue` L34），非连续模式的上一章/下一章（L1369、
L1394），非连续模式的正文搜索跳转（L311）和书签跳转（L332），目录加载完成（L944），刷新正文 `refreshContent()`
（L995–997）。

连续模式的上一章/下一章（L1364–1367、L1389–1392）、正文搜索跳转（L301–306）和书签跳转（L322–327）不走
`getContent`：它们设置 `scrollStartChapterIndex` 后调用 `computeShowChapterList()`（L1249–1324），等
`loadShowChapter()`（L1163–1224）把章节放进列表、成为可见章之后，由下一次 `saveReadingPosition` 同时更新 index 和标题。
连续模式的目录跳转仍走 `getContent`。

### 2.3 加载中、失败与迟到响应

- 加载中：`getContent` L1000 先 `show = false`，L1001–1009 在阅读区盖上 `$loading` 遮罩（“正在获取内容”），`Content`
  因 `v-if="show"`（L232）不渲染。此时 mini 顶栏已经是目标章标题。
- 失败：正文接口返回 `isSuccess: false`（L1072–1085）或请求出错（L1090–1118），都把 `content` 设为
  `获取章节内容失败！` 加原因、`error = true`、`show = true`，**不改 `this.title`**。`Content` 渲染
  `<h3>目标章标题</h3>` 加错误文字；请求出错时还弹出 `获取章节内容失败 …`（L1110–1112）。上游正文里没有“重新加载”
  按钮，重试走工具栏刷新 `refreshContent()` → `getContent(index, true)`，标题仍是该章。
- 迟到响应：成功和失败两个回调开头都比较 `bookUrl` 与 `readingBook.index`（L1047–1053、L1091–1097），已换书或换章
  就直接返回。标题在导航开始时已经是最新目标，迟到响应不会改它。
- 目录缺行：`catalog[index]` 不存在时（L1023–1036）先刷新一次目录，再不行就显示“获取章节内容失败，请更新目录！”。这条
  路径不写 `this.title`，顶栏保留旧章节名。

### 2.4 换书

- 从书架进入另一本书：`init()` 先把 `this.title` 清空（L878），目录到达后由 `getContent` 写成目标章标题，顶栏为
  “空 → 目标章”。
- Reader 内书架换书 `changeBook()`（L913–919）不清空 `this.title`：新目录到达之前，顶栏保留旧书当前章的章节名，然后由
  `getContent` 写成新书目标章。顶栏只显示章节名、不显示书名或序号，这期间显示的是旧书的完整标签，不会把两本书拼在一起。

### 2.5 上游可见行为（mini 顶栏）

1. 经过 `getContent` 的导航：导航开始时就换成目标章目录标题，加载中保持，正文失败也保持；正文 `<h3>` 和标签页标题
   同时指向目标章。
2. 连续模式的上一章/下一章、正文搜索和书签跳转：加载期间 index 和标题都还是旧章，新章成为可见章后一起换。
3. 连续滚动跨章、EPUB 内链：index 和标题在同一个同步块里一起更新。
4. 迟到响应被丢弃，被取代的导航不会把标题改回去。
5. 进入另一本书：先为空，目录到达即为目标章，不等正文；Reader 内换书：保留旧书章节名，直到新书目标章。
6. 除“目录缺行”这一边缘情况外，`readingBook.index` 和 `this.title` 要么在同一个同步块里一起改，要么一起推迟，不会
   出现“新位置 + 旧章节名”。

## 3. 当前 OpenReader（c8ebd30）

### 3.1 派生

- `Reader.vue:1419–1423` `formatRunningChapterLabel(index, title)`：`第 ${index + 1} 章` 加去掉首尾空白的标题，标题为空
  时只有序号。
- `Reader.vue:1426`：`runningChapterLabel = formatRunningChapterLabel(currentIndex.value, chapter.value?.title)`。序号来自
  `currentIndex`，章节名来自 `chapter`（最近一次提交的章节对象），两者由不同代码在不同时刻写入。
- `Reader.vue:543–547`：`book`、`chapters`（目录）、`chapter`、`currentIndex`。`currentIndex` 的初值是
  `Number(route.query.chapter || 0)`（L547）。

### 3.2 统一加载事务 `useReaderChapterLoader.load`

- `:24`：在任何 `await` 之前把 `currentIndex` 设为目标章（按目录长度截断，`:20–23`）。
- `:42–47`：目标不在内存缓存时立即把 `chapterLoading` 设为真，正文区显示“正在加载章节...”（`ReaderChapterContent.vue:2`）。
- `:50–58`：正文接口返回、`isCurrent()`（`:28–30`，比较 generation、`currentIndex` 和 scope key）通过后，才提交
  `chapter = data.chapter`。
- `:128–134`：失败分支设置 `chapterLoadError`、清空资源，**不改 `chapter`**。正文区显示错误块（`ReaderChapterContent.vue:4–7`，
  只有错误文字和“重新加载”，没有章节标题）。
- `:129`：被取代或中止的请求直接返回，什么都不提交。
- 结果：从 `:24` 到 `:58` 之间，以及失败之后直到下一次成功提交，`runningChapterLabel` 都是“新序号 + 上一次提交的
  章节名”。

走这条事务的入口：`goChapter`（`useReaderNavigation.js:186–208`，目标不在已加载窗口时经 `navigate` 改路由）→ 路由
`positionQuery` 监听（`useReaderRouteSync.js:27–51`）→ `loadChapter`；首次进入和换书（`useReaderBookLoad.js:99`、`:169`）；
EPUB 内链（`Reader.vue:2156–2165`）；错误块“重新加载”（`useReaderChapterMaintenance.js:39–47`）；替换规则刷新
（`useReaderExternalUpdates.js:71` 起）；进度同步跳转（同文件 `:6` 起）；书籍数据同步（同文件 `:46–65`）；换源
（`useReaderCatalogActions.js:77–101`）；目录刷新（`useReaderChapterMaintenance.js:4–23` 之后重载）。

### 3.3 同时提交的路径（不受影响）

- 连续滚动跨章 `useReaderChapterWindow.js:206–221`（`syncCurrentChapter`），窗口激活 `:106–116`，已加载窗口内跳转
  `useReaderNavigation.js:106–117`，正文搜索跳转到已渲染的章 `useReaderSearchNavigation.js:69–70`，设置变更后的位置恢复
  `Reader.vue:1514–1526`：都在同一个同步块里写 `currentIndex` 和 `chapter`，多数直接用目录行 `chapters[i]`。
- 连续模式追加章失败时的内联错误块 `useReaderChapterWindow.js:68–79` 用目录行标题；滚入视口后同样同时提交。

### 3.4 正文响应里的章节对象

- 书架书：`chapterContent`（`backend/api/books.go:2581` 起）按 `book_id` 和 `index` 取目录行，返回前按 id、index、url
  复验（`:2626–2636`），响应里的 `chapter` 就是这一行。
- 远程临时阅读：`remoteReaderSessionChapterContent`（`backend/api/remote_reader.go:164–186`）返回会话目录里的同一行。
- 所以在真实服务下 `chapter.title` 与 `chapters[currentIndex].title` 相同，差别只在提交时刻。浏览器合同的模拟夹具可以
  让两者不同（例如重新认证夹具的目录是 `第一章`、正文响应是 `第1章`），目标合同以目录行为准。

### 3.5 首次进入与换书

- 首次进入：目录到达前 `chapter` 为 `null`、`currentIndex` 取路由默认值；目录到达后，`useReaderBookLoad.js:136` 和
  `:152`/`:154` 在同一个同步块里写目录和 `currentIndex`；正文到达后才有章节名。没有 `chapter` 参数时（直接打开
  `/books/:id/read`、按已存进度续读），先显示 `第 1 章`。
- Reader 内换书：`App.vue:5` 的 Reader 只以 `sessionGeneration` 为 key（`:59`），换书复用同一实例。`useReaderShelf.js:62–73`
  `router.push` 到另一本书，带 `readerRouteQueryFromBook` 生成的 `resume=1&chapter=…`（`readerRoute.js:9–21`）。
  `useReaderRouteSync.js:18–25` 的 `bookId` 监听开始加载新书；`useReaderBookLoad.js:115` 同步换上书架里的新书对象，
  `:121` 第一次 `await` 之后，同一次路由变化触发的 `positionQuery` 监听（`useReaderRouteSync.js:27–51`）立即对新书的
  目标序号调用 `loadChapter`，此时 `chapters` 还是旧书的目录。`chapter` 全程不重置（`onBookLoadStart`
  `Reader.vue:2028–2032` 只重置删书标记、恢复进度保存、清空错误），所以页头显示“新书目标序号 + 旧书当前章节名”，直到
  新书正文提交；新书正文失败时一直保留。

## 4. 真实浏览器复现（2026-10-17）

### 4.1 环境

- 产品代码：`c8ebd30` 的 frontend 生产构建（`npm ci`、`npm run build`）和用 Go 1.26.9 编译的 backend 二进制，独立的
  临时 data/cache/library 目录和随机 JWT secret，监听 `127.0.0.1:18431`。会话沙箱禁止 loopback 监听，服务和 Playwright
  在沙箱外运行。
- Playwright 1.61.1（根目录 devDependency），chromium-headless-shell 1228。
- 探针（会话 scratchpad，未入库）：
  - 文本：与 `scripts/smoke/reader-continuous-contract.mjs` 相同的 `page.route` 模拟（假 token，`/ws/sync` 中止），8 章
    夹具，可按章设置延迟或 500。`addInitScript` 在 `document` 上安装 `MutationObserver`，记录运行标题的每一个不同的值，
    包括没被绘制的中间值。
  - EPUB：真实服务注册测试账号，导入与 `reader-epub-contract.mjs` 同形的 EPUB 夹具（目录 `0 封面`、`1 第一章（下）`、
    `2 第二章`），只拦截第 2 行的正文请求（延迟 2.5s 或返回 500）。每一步用全新的浏览器上下文，避免浏览器章节缓存命中。
- 视口 1440×900、390×844、360×800；文本模式 page、flip、scroll、scroll2。

### 4.2 文本：控件、路由、迟到请求与重新加载

3 个视口 × 4 个模式共 12 组，标签序列完全相同。夹具目录：`0 第一章 起点`、`1 第二章 延迟`、`2 第三章 缓存`、
`3 第四章 预载`、`4 第五章 失败`、`5 第六章 迟到`、`6 第七章 取代`、`7 第八章 终点`。预载半径为 2（`Reader.vue:710`），
失败章和慢章都放在预载范围外。

| 步骤 | 操作 | 当前观察值 | 上游等价行为 |
| --- | --- | --- | --- |
| S0 | 打开 `?chapter=0` | `第 1 章 第一章 起点` | — |
| S1 | 桌面 `下一章`（`ReaderDesktopProgress.vue:15`）/ 移动工具层 `下一章`（`ReaderMobileChrome.vue:98`）；第 1 行正文延迟 3s，打开 Reader 前就设置，预载也被延迟 | 700ms 时 `第 2 章 第一章 起点`，正文“正在加载章节...”；混合值持续 2753–2791ms，之后 `第 2 章 第二章 延迟` | 非连续模式：导航开始即 `第二章 延迟`；连续模式：加载期间保持旧章 `第一章 起点`（第 5 节第 7 行） |
| S2 | 路由 `?chapter=4`，正文 500 | 错误块出现后 `第 5 章 第二章 延迟`，1.5s 后不变；错误块里没有章节标题 | `第五章 失败`；正文 `<h3>第五章 失败</h3>` 加错误文字 |
| S3 | 路由 `?chapter=5`（正文 4s），600ms 后改到已在内存缓存的 `?chapter=2` | 加载中 `第 6 章 第二章 延迟`；切到第 2 行时先出现 `第 3 章 第二章 延迟`，1–4ms 后变成 `第 3 章 第三章 缓存`；第 5 行的 200 迟到返回后标题不变 | 加载中 `第六章 迟到`，切换后 `第三章 缓存`，迟到响应被丢弃 |
| S4 | 路由 `?chapter=4`（500），源恢复后点错误块“重新加载”（正文 1.5s） | 失败后 `第 5 章 第三章 缓存`，重新加载中不变，成功后 `第 5 章 第五章 失败` | 失败与刷新期间都是 `第五章 失败` |

- S3 的迟到请求没有回写，与运行标题合同第 3 条和上游一致，这是现状已满足的部分。
- S3 说明目标章已在内存缓存时也会出现一次“新序号 + 旧章节名”的 DOM 状态。它可能不被绘制，但 MutationObserver 能
  记录到，浏览器合同可以据此断言。
- 1440×900 上，错误块的“重新加载”被 `.reader-tap-zones` 里的 `button.tap-zone.tap-upper` 盖住，`elementFromPoint` 命中
  点击层，4 个模式相同；390×844 和 360×800 命中按钮本身。探针改用 `dispatchEvent('click')` 继续观察，见第 10 节。

### 4.3 文本：目录面板（3 视口，page 模式，结果相同）

| 步骤 | 操作 | 当前观察值 | 上游 |
| --- | --- | --- | --- |
| T1 | 目录 → `第六章 迟到`（正文 2.5s） | 加载中 `第 6 章 第一章 起点`，之后 `第 6 章 第六章 迟到` | 导航开始即 `第六章 迟到` |
| T2 | 目录 → `第五章 失败`（500） | `第 5 章 第六章 迟到` 一直保留 | `第五章 失败` |

### 4.4 EPUB（真实服务，3 视口，结果相同）

| 步骤 | 操作 | 当前观察值 | 上游 |
| --- | --- | --- | --- |
| E1 | 第 1 行 iframe 内链 `#next-chapter` → `two.xhtml`，第 2 行正文接口延迟 2.5s | 800ms 时 `第 3 章 第一章（下）`，正文“正在加载章节...”，之后 `第 3 章 第二章` | 链接导航当下同步写成 `第二章`，没有正文请求 |
| E2 | 同上，第 2 行正文接口 500 | `第 3 章 第一章（下）` 一直保留，错误块显示 `fixture EPUB chapter failure` | 同 E1（上游没有这次请求，也就没有这种失败） |

- 移动端工具层默认可见，会盖住 iframe 里的链接；按 EPUB 合同的做法，先点中间收起工具层再点链接。
- 第一次运行时 E2 排在 E1 后面、共用一个上下文，第 2 行已被 E1 写进浏览器章节缓存，请求没有到网络；MutationObserver 仍
  记录到约 2ms 的 `第 3 章 第一章（下）` 中间值。改为每步新上下文后，两步都按预期复现。

### 4.5 Reader 内换书与首次进入（模拟接口，3 视口，page 模式，结果相同）

| 场景 | 操作 | 当前观察值 | 上游 |
| --- | --- | --- | --- |
| B1 | 书一第 3 行（`甲三 当前`）→ 按 `useReaderShelf.select` 的方式 `router.push` 到书二 `resume=1&chapter=3`；书二目录和第 4 行正文各延迟 1.5s | 约 1.5s 的 `第 4 章 甲三 当前`，之后 `第 4 章 乙四 目标` | 新目录到达前保留 `甲三 当前`（旧书完整标签），之后 `乙四 目标` |
| B2 | 同上，书二第 4 行正文 500 | `第 4 章 甲三 当前` 一直保留：书二的序号拼上书一的章节名 | `乙四 目标` |
| F1 | 直接打开 `/books/1/read`（无 `chapter`，已存进度在第 4 行），目录和正文各延迟 1.5s | `第 1 章`（约 1.5s，序号错）→ `第 4 章`（没有章节名，约 1.5s）→ `第 4 章 甲四 预载` | 空 → `甲四 预载`（目录到达即写，不等正文） |

### 4.6 其他记录

- 服务日志：1186 个 200、7 个 201（注册和导入）；91 个 401 全部来自模拟上下文里的 `/ws/sync`（假 token，`page.route`
  拦不到 WebSocket，与 EPUB 移动端章节计数合同第 9 节的记录相同）。没有 5xx（500 都是浏览器侧注入的），没有 panic。
- 文本探针的进度 `PUT` 都发生在成功加载之后（如 `1:12:第二章 延迟`、`2:13:第三章 缓存`），这次没有观察到失败状态下的
  进度写入，第 10 节第 3 条仍是读代码推断。

## 5. 差异分类

| # | 场景 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | 经统一加载事务的切章，加载中：上一章/下一章，目录，书签和正文搜索跳到未加载的章，路由参数，进度跳转，自动翻页、朗读和音频下一章 | 导航开始即为目标章目录标题 | `第 新 章 旧章节名`，持续整个请求（S1、T1） | **`must-fix`**：上游可见行为。运行标题里用户要求的只是序号（运行标题合同第 6 条），序号与章节名指向不同章不属于任何允许差异 |
| 2 | 同上，目标已在内存或浏览器缓存 | 同上，没有中间值 | 持续 1–4ms 的中间值（S3、E2 首次运行） | **`must-fix`**：同一根因。目标合同禁止任何中间值 |
| 3 | 正文加载失败 | 保留目标章标题，正文 `<h3>` 也是目标章 | `第 新 章 旧章节名` 一直保留（S2、T2、E2、B2） | **`must-fix`** |
| 4 | 失败后“重新加载”或刷新期间 | 刷新走 `getContent(index, true)`，标题仍是该章 | 旧章节名保留到成功（S4） | **`must-fix`**，随第 3 项 |
| 5 | EPUB 内链跨章 | 链接当下同步写 index 和标题 | 走加载事务，加载中和失败同第 1、3 项（E1、E2） | **`must-fix`**。OpenReader 为了刷新签名资源 URL 而重新请求正文（`Reader.vue:2160–2162` 注释），这次请求本身是已签收的运行时适配，不重开；只要求标题在导航开始时就是目标章 |
| 6 | 被取代或迟到的请求 | 丢弃 | 丢弃，不回写（S3） | `aligned`，保留为回归断言 |
| 7 | 连续模式的上一章/下一章、正文搜索和书签跳转到未加载的章 | 加载期间 index 和标题都保持旧章，新章可见后一起换 | 走加载事务，`currentIndex` 在开始时就换（目前还带着旧章节名，即第 1 项） | 修复后为“开始时整体换成目标章”：**`acceptable-change`**（运行时适配）。OpenReader 所有未加载章的导航共用一个加载事务，`currentIndex` 同时驱动目录高亮、进度和上一章/下一章；上游在连续模式下对这几个入口推迟切换，对目录跳转却也是开始即换。两种时机都不会让序号和章节名指向不同章，最终状态相同。如果用户要求连续模式照搬上游的推迟时机，另开切片 |
| 8 | 连续滚动跨章，已加载窗口内跳转，正文搜索跳到已渲染的章，连续模式内联错误块 | 同一个同步块里更新 | 同时提交（第 3.3 节） | `aligned` |
| 9 | 首次进入，目录未到达 | 空 | 无 `chapter` 参数时为 `第 1 章`，序号错误（F1）；有 `chapter` 参数时为路由给出的 `第 N 章`，没有章节名 | **`must-fix`**：不能显示猜出来的序号。目录行未知时运行标题为空，与上游一致 |
| 10 | 首次进入，目录已到达、正文未到达 | 目标章标题 | `第 N 章`，没有章节名（F1） | **`must-fix`**，随第 1 项：章节名取目录行，不等正文 |
| 11 | Reader 内换书 | 新目录到达前保留旧书章节名，之后为新书目标章 | 新书序号 + 旧书章节名，失败时一直保留（B1、B2） | **`must-fix`**：中间态只允许旧书的完整标签或空，不允许跨书拼接 |
| 12 | 换源、目录刷新、书籍数据同步后的重载 | `changeBookSource` → `loadCatalog(true)` → `getContent`：用新目录标题 | 新目录先替换，`chapter` 仍是旧对象，章节名要等重载成功才换 | **`must-fix`**，随第 1 项：章节名取新目录行 |
| 13 | 目标章不在目录里 | 刷新目录或显示“请更新目录”，标题保留旧章节名 | 加载器把目标截断到目录范围（`useReaderChapterLoader.js:20–23`） | **`acceptable-change`**（运行时适配）：不复现上游越界时的陈旧标题 |
| 14 | 运行标题与标签页标题的章节名来源 | 同一个 `this.title` | 运行标题用 `chapter.title`；标签页标题合同 R1 规定用 `chapters[currentIndex].title`（红测 `377e8b8` 已按此编写，实现未合入） | **`must-fix`**：必须同源，否则加载中和失败时标签页与页头指向不同章 |

没有一项判为 `intentional-redesign`：仓库里没有文档把“正文加载完成后才换章节名”作为有意的设计提出过，见第 6 节对
运行标题合同第 3 条措辞的说明。

## 6. 已签收、不重开

- 运行标题格式 `第 N 章 章节名`、1-based 序号（用户要求，运行标题合同第 6 条；`9b96c50`，桌面分页合同第 28 行），空标题
  只显示 `第 N 章`，标题去掉首尾空白。
- 桌面和移动都显示运行标题（桌面运行标注合同第 5 节第 1 行），mini 的 30px + safe area 栏，单行省略，`pointer-events: none`，
  桌面文档滚动时的固定页头（`437a940`），音频和 CBZ 例外。
- 桌面 `N / total` 的删除（`4092ea2` 裁决，`035762d` 实现，未合入 main）。
- 标签页标题合同（`c8ebd30`）的 R1–R6 与 D1。本文只新增“运行标题与标签页标题的章节名同源”这一约束，不改它的任何条目。
- 运行标题合同第 3 条的前半句（连续滚动以 viewport 已更新的 `currentIndex` 切换标签）和最后一句（加载中的旧请求不能回写
  标题）。
- 加载器的 generation、abort、scope key 机制，EPUB 内链重新请求签名资源，预载半径，正文区“正在加载章节...”提示。
- 运行标题合同第 3 条中间一句“点击翻页、目录跳转、进度恢复、EPUB hash 跳转也在章节事务完成后更新”：这是 OpenReader
  自己的措辞，不是用户要求（用户要求只有第 6 条的序号）；写的时候没有对照上游的加载中和失败状态，现状也不符合这句话
  （序号在事务开始时就换了）。本文按上游把它改为“导航开始时整体换成目标章，失败不回退”。这是对这句措辞的重新裁决，
  不是重开用户要求。第 1 条“章节名来自当前可见/当前激活 Chapter”保留，本文把“当前激活”明确为 `currentIndex` 对应的
  目录行（R1）。
- API、数据库、浏览器存储 key、持久文件、镜像与生产：本切片都不改，不在本地构建或推送镜像，不做任何生产操作。

## 7. 目标合同

R1. 单一来源：运行标题由 `currentIndex` 和 `chapters` 派生。设 `i = currentIndex`，标签为 `第 ${i + 1} 章` 加
  `chapters[i].title` 去掉首尾空白；标题为空或只有空白时为 `第 ${i + 1} 章`。不再读取已提交的 `chapter` 对象。标签页标题
  合同 R1 的章节名取同一行。

R2. 目录行未知时为空：`chapters[i]` 不存在（目录尚未到达、加载失败、下标越界）时，运行标题为空串，不显示按路由默认值
  猜出的序号。

R3. 导航开始即切换：凡是改变 `currentIndex` 的导航，都在同一次渲染里显示目标章标签；加载期间、正文失败后、失败后重新
  加载期间都保持目标章标签，直到下一次导航。任何时候都不允许出现序号与章节名来自不同目录行的值，包括只存在于一次 DOM
  更新、不会被绘制的中间值。

R4. 迟到与中止：被取代或中止的加载结束后（无论成功还是失败），运行标题仍是最新导航的目标章，正文也不回写。（现状已满足，
  作为回归断言。）

R5. 同一本书：序号和章节名必须来自路由当前这本书的目录。Reader 内换书时，中间态只允许保留旧书的完整标签，或为空；
  不允许“新书序号 + 旧书章节名”，也不允许“旧书序号 + 新书章节名”。新书正文失败时显示新书目标章的标签。远程临时阅读
  （`useReaderBookLoad.js:93–94` 同步替换 `book` 和 `chapters`）同样适用。

R6. 目录替换：换源、目录刷新、书籍数据同步、更换本地目录规则之后，运行标题立即使用新目录在 `currentIndex` 处的标题，
  不等正文重载。

R7. 不变：连续滚动跨章和已加载窗口内跳转的切换时机，加载器的事务和取消机制，正文区的加载与错误显示，运行标题的格式与
  几何，桌面/移动与音频/CBZ 例外。不新增浏览器存储 key、API、数据库字段或持久文件；后端不改。

建议的实现形状（不强制）：

- 把 `formatRunningChapterLabel` 移到纯工具模块（例如 `frontend/src/utils/readerRunningTitle.js`），新增
  `runningChapterLabelFor({ chapters, index })`，目录行缺失时返回 `''`；`Reader.vue` 的 `runningChapterLabel` 改为调用它。
  标签页标题的实现可以和它共用同一个“当前目录行”计算属性。
- R5 的机制由实现阶段决定，但必须由第 8.1 节的单元合同固定下来。可选做法：在 `useReaderRouteSync` 里，同一次路由变化
  同时改了 `bookId` 时，不让 `positionQuery` 监听用旧目录调用 `loadChapter`（新书的加载流程本来就按路由参数定位）；或者
  比较目录行的 `bookId` 与路由里的书号（书架书的目录行带 `bookId`，`backend/models/models.go:413`；远程临时阅读的目录行
  没有会话身份，需要单独处理）。

## 8. 测试计划（后续阶段执行，本切片不写测试）

### 8.1 单元合同（`node --test`）

1. `frontend/tests/readerTextModeFrameContract.test.mjs`：
   - 现状：第 113 行的测试“reader exposes the current chapter number and title in the upstream running header”，第 119–123 行
     （`035762d` 之后是第 129–133 行）断言 `runningChapterLabel = computed(() => formatRunningChapterLabel(currentIndex.value,
     chapter.value?.title))`，失败消息是“the label must derive from the current 1-based chapter position and active title”。
   - 影响：这条断言固定的正是本文判为 `must-fix` 的派生方式。按 CLAUDE.md“与上游冲突的 Reader 测试应删除或重写”，**重写**
     这条断言：`runningChapterLabel` 由当前目录行派生（调用新工具函数，参数包含 `chapters` 和 `currentIndex`），并加
     `assert.doesNotMatch(readerSource, /formatRunningChapterLabel\([^)]*chapter\.value\?\.title/)`。同一测试的其余断言
     （页头结构、`showRunningChapterHeader` 例外、mini 不得 `display: none`、mini safe-area 几何、单行省略）不动。
2. `frontend/tests/readerRunningTitle.test.mjs`（新增，纯函数）：
   - 下标 0 的 `第一章 起点` → `第 1 章 第一章 起点`；中间章；空标题和只有空白的标题 → `第 N 章`；首尾空白被去掉；
   - 目录为空、下标越界、下标为负 → `''`；
   - 不加书名，不做简繁转换，不套用替换规则（与标签页标题合同 R1 的章节名一致）。
3. `frontend/tests/readerRunningTitleTransaction.test.mjs`（新增，`effectScope` + `ref`，接上真实的 `useReaderChapterLoader`，
   `loadContent` 换成受控 promise；用 `watch(label, …, { flush: 'sync' })` 记录每一个中间值）：
   - `chapters = [甲, 乙, 丙]`、`currentIndex = 0`、`chapter = 甲` 时调用 `load(2)`：promise 未决时标签为 `第 3 章 丙`，
     记录里不得出现 `第 3 章 甲`；
   - 让 promise 以 HTTP 500 形状的错误拒绝：标签仍是 `第 3 章 丙`，`chapterLoadError` 非空，`chapter` 仍是 `甲`（失败不提交
     正文，现状保留）；
   - 失败后 `load(2, { refresh: true })` 期间标签不变；
   - 先 `load(1)`（慢）再 `load(2)`（快），之后让 1 成功：标签是 `第 3 章 丙`，`chapter` 是 `丙`（R4）；
   - 目录为空时标签为 `''`；目录到达且 `currentIndex` 在同一个同步块里写入后，直接是目标章标签（R2）；
   - 按 `useReaderBookLoad` 的真实顺序换书，并按选定的 R5 机制模拟 `positionQuery` 的竞态：记录里不得出现
     “新书序号 + 旧书章节名”。
   - 红测写法同标签页标题合同第 8.1 节：在测试体内 `await import(...)` 新模块，把导入失败转成断言消息
     （例如“running title helper must exist”），让每条测试以明确的断言失败。
   - 预期：当前实现上，第 1 项改写后的断言和第 2、3 项全部失败（新模块不存在，或 `Reader.vue` 仍从 `chapter` 派生）。
     其中 R4 的行为现状已经满足，实现后它和其余断言一起转绿。

### 8.2 真实浏览器

1. 新增 `scripts/smoke/reader-running-title-load-contract.mjs`（模拟接口，写法同 `reader-continuous-contract.mjs`），视口
   1440×900、390×844、360×800（可加 1024×1366），模式 page、flip、scroll、scroll2。`addInitScript` 在 `document` 上安装
   `MutationObserver`，把运行标题的每个不同值记入 `window.__runningTitleLog`。步骤沿用第 4.2–4.5 节的 S1–S4、T1–T2、
   B1–B2、F1，夹具同第 4.2 节。断言：
   - 记录里每个非空值都等于 `第 k 章 <夹具第 k 行的标题>`（或空标题规则），也就是序号和章节名来自同一目录行。这一条能抓住
     只存在于一次 DOM 更新的中间值；
   - S1、T1：延迟期间（“正在加载章节...”可见时）标签等于目标章；S2、T2、B2：错误块出现后、1.5s 后都等于目标章；S4：重新
     加载期间等于目标章；S3：迟到响应之后标签等于最新目标，正文 `<h3>` 也是最新目标；B1、B2：记录里不出现“书二序号 +
     书一章节名”；F1：目录到达前记录里只有空值，不出现 `第 1 章`；
   - 预期红点（按第 4 节实测）：第一个失败点是 S1 的“延迟期间等于目标章”，实际 `第 2 章 第一章 起点`（12 组都一样）。
     T1、B1、F1 在各自的新上下文里运行，第一个失败点分别是 `第 6 章 第一章 起点`、`第 4 章 甲三 当前`、`第 1 章`；如果按
     第 4 节的顺序继续执行 S2，实际是 `第 5 章 第二章 延迟`。逐视口、逐模式记录精确失败点；
   - S4 的“重新加载”在 1440×900 被点击层盖住（第 10 节第 1 条），本合同不修它。脚本里先用 `dispatchEvent('click')`，注释写明
     原因，等点击层问题单独修好后改回真实点击。
   - 与标签页标题浏览器合同的关系：`377e8b8` 新增的 `scripts/smoke/document-title-contract.mjs` 第 3–7 步（下一章、目录、空标题章、
     500 失败章、换书）与本文的 S1、S2、T2、B2 用的是同类夹具。两者都落地后，可以在同一步里同时断言运行标题和标签页标题，
     但本文要求的延迟期间、重新加载、迟到请求和首次进入断言不能省。
2. `scripts/smoke/reader-epub-contract.mjs`（真实服务，3 视口）：在第 704 行点击 `#next-chapter` 之前，对第 2 行正文接口
   `page.route` 延迟约 1.5s；点击之后、第 710–712 行的等待之前，断言运行标题已经是 `第 3 章 第二章`。当前实际值是
   `第 3 章 第一章（下）`，3 个视口都应失败。失败分支（正文 500）放在新脚本的 EPUB 部分，并用独立浏览器上下文（同一上下文
   里第 2 行会被前面的步骤写进浏览器章节缓存，见第 4.4 节）。
3. `scripts/smoke/reader-continuous-contract.mjs`（加固，不是红测）：第 229、255、265、299、414 行用 `startsWith('第 N 章')`
   判断运行标题。夹具章节名本身就是 `第 N 章`，“新序号 + 旧章节名”（如 `第 3 章 第 2 章`）也能通过这些判断。把它们改为精确
   相等（如 `第 3 章 第 3 章`）。这些路径都走同时提交，当前和实现后都应通过。

### 8.3 回归与临时变异（实现阶段）

- frontend `npm test`（写明新增的测试条数）、`npm run build`。
- 真实浏览器：8.2 的全部脚本；另跑 `reader-mobile-contract`、`reader-text-modes-contract`、`reader-reauthentication-contract`
  作为对照。
- 临时变异只在工作区修改并构建到独立目录，构建后立即还原，不提交：
  - M1：运行标题改回读取 `chapter.value?.title` → 8.1 第 1、3 项和 8.2 第 1 项的 S1、S2、T1、T2 失败；
  - M2：章节名取目录行，但只在正文成功后才换（例如在加载器 `:58` 处另存一个“已显示下标”）→ S1、S2 失败，证明断言守住的是
    “开始即换、失败保留”，而不只是最终状态；
  - M3：去掉 R5 的机制 → 8.1 第 3 项的换书用例和 B1、B2 失败；
  - M4：目录未到达时显示 `第 ${currentIndex + 1} 章` → 8.1 第 2 项和 F1 失败；
  - M5：去掉加载器 `:57` 的 `isCurrent()` 检查 → S3 的正文 `<h3>` 断言失败。按 R1，标签本身不受这个变异影响，所以 S3 必须同时
    检查正文。
- 跳过并写明原因：Go `test`/`vet`/`race`（后端不改）；依赖 advisory、Compose 和卷门（依赖、Dockerfile、数据格式都不改）。
- 会话沙箱禁止 loopback 监听，服务和 Playwright 需要在沙箱外运行。

## 9. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交（本切片）。
2. **排期**：桌面计数切片（`035762d`、`9839270`，`claude/suspicious-ptolemy-1af85f`）和标签页标题红测（`377e8b8`，
   `claude/xenodochial-carson-5004c3`，基于 `9839270` 和与 `c8ebd30` 内容相同的 `c75e730`，实现尚未提交）改的是同一批文件：
   `Reader.vue`、`readerTextModeFrameContract.test.mjs`、`reader-epub-contract.mjs`、`reader-continuous-contract.mjs`。建议在它们
   落地之后、在其之上切分支；R1 的“当前目录行”可以和标签页标题实现共用。
3. **红测**：8.1 第 1–3 项、8.2 第 1–2 项，记录每个视口和模式的精确失败点。提交
   `test: expose stale running chapter title during load and failure`。
4. **实现**：最小改动让同一批断言转绿。提交 `fix: derive running chapter title from the target catalog row`。
5. **回归**：按 8.3 执行，包括临时变异。
6. **记录**：提交号、各视口和模式的结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
7. **发布**：实现提交改动 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；是否合入由用户决定。

## 10. 观察（不在本切片）

1. 1440×900 文本四种模式下，正文错误块的“重新加载”按钮被 `.reader-tap-zones` 里的 `button.tap-zone.tap-upper` 盖住，
   `elementFromPoint` 命中点击层，鼠标点击会落到翻页或菜单区；390×844 和 360×800 能命中按钮。上游正文里没有这个按钮（重试
   走工具栏刷新），这是 OpenReader 自有控件的可用性问题。连续模式的内联错误块按钮在 390×844 能点到
   （`reader-continuous-contract.mjs` 的 `runFailureRetry`）。需要另行盘点。
2. 正文失败时，上游渲染 `<h3>目标章标题</h3>` 加错误文字（`Content.vue` L62），OpenReader 的错误块
   （`ReaderChapterContent.vue:4–7`）没有章节标题。这属于正文错误面，需要另行对照上游盘点。运行标题按本文修好后，页头会显示
   目标章。
3. 进度载荷 `readerProgressPayload`（`frontend/src/utils/readerProgressPersistence.js:62–90`）在没有可见快照时，用 `chapter`
   的 id 和标题配 `currentIndex` 的序号。失败后 `chapter` 仍是旧章，如果这时触发保存（`isBlocked` 只看窗口事务和删书，
   `Reader.vue:1722`），可能写出“新序号 + 旧章 id/标题”。上游 `saveBookProgress` 只传序号，标题由服务端按目录行取
   （`BookController.kt` L390–429，L424–426）。本次探针没有观察到失败状态下的进度写入，也没有核对后端是否校验 id 与序号的
   对应关系，这是读代码推断，需要另行盘点进度持久化。
   2026-11-14 补记：[失败态进度载荷盘点](reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md)第 4 节在真实服务上
   证实了这个载荷，服务端按 `ErrChapterIdentity` 以 400 拒绝；分类与目标见该文第 5、7 节。

## 11. 实施补充（2026-11-12，实施前）

基线：分支 `claude/reader-error-title-progress-59bcb6` 上错误面切片 R1–R6、R8 完成之后的提交（见[错误面合同](reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md)
第 14 节）。标签页标题（`useReaderDocumentTitle`）早已按 `currentIndex` 的目录行派生并要求目录行属于当前书；错误面切片新增了
`frontend/src/utils/readerCatalogRow.js#readerCatalogRow`，错误页 `<h3>` 用的就是这一行。第 3 节的现状在这个基线上仍然成立：
`Reader.vue` 的 `runningChapterLabel` 仍是 `formatRunningChapterLabel(currentIndex.value, chapter.value?.title)`。本节只确定实施细节，
不改第 7 节的目标。

1. **同一行**。新增 `frontend/src/utils/readerRunningTitle.js#runningChapterLabelFor({ book, chapters, index })`，目录行取
   `readerCatalogRow`：只有属于当前书的 `chapters[currentIndex]` 才算数（书架目录行带 `bookId`，远程临时阅读两边都是 0）。标签页
   标题改用同一个函数取行，运行标题、标签页标题和错误页标题从此读同一行（第 5 节第 14 行）。
2. **R5 的机制**。两层：
   - 目录行属于当前书。书在书架上时，`useReaderBookLoad` 先同步换上新书对象，旧目录的行不再属于它，标签为空。
   - 路由同步。同一次路由变化同时改了书号时，`useReaderRouteSync` 的位置监听不再用旧目录调用 `loadChapter`：新书的加载会按路由或已存进度定位。
     书不在书架上时，新书对象要等书籍请求返回，旧书对象和旧目录都还在。原来的位置监听会把 `currentIndex` 改成新书的目标序号，标签变成
     “旧书另一章”的完整标签，还会提前请求新书正文。这一层把这两件事都去掉。
3. **测试粒度**。单元事务测试用两个记录器：`flush: 'sync'` 记录每个值，要求任何值的序号和章节名都来自同一本书的同一行；`flush: 'pre'`
   记录每次 DOM 更新显示的值，要求换书时只出现旧书完整标签、空串或新书目标章。`useReaderBookLoad` 在同一个同步块里先写目录、后写
   `currentIndex`，两次写入之间会有一个“新目录第 i 行”的值，同一行、同一本书，不进入任何 DOM 更新，所以不算违例。浏览器合同用
   MutationObserver，粒度和 DOM 更新相同。
4. **测试改写（不是弱化）**：
   - `readerTextModeFrameContract.test.mjs` 固定了 `formatRunningChapterLabel(currentIndex.value, chapter.value?.title)` 这个派生方式，
     按第 8.1 节第 1 项改写为由目录行派生，并禁止从 `chapter` 派生。同一测试的其余断言不动。
   - `reader-continuous-contract.mjs` 的五处 `startsWith('第 N 章')` 改为精确相等（第 8.2 节第 3 项）。
5. **S4 的按钮**。第 8.2 节要求 S4 先用 `dispatchEvent`，等点击层问题修好再改回真实点击。R7 已发布，新脚本直接用真实指针（桌面鼠标、
   手机触摸）。连续模式下失败章按错误面 R6 显示为内联错误块，S4 按它的“重新加载”。
6. **EPUB**。`reader-epub-contract.mjs` 在点 `#next-chapter` 前延迟第 2 行正文请求约 1.5s，点击 400ms 后断言运行标题已是 `第 3 章 第二章`。
   同时用 MutationObserver 记下所有值，要求没有别的值。第 2 行可能已经因为预载进了内存缓存，这时请求不会走网络，但中间值仍会出现在
   一次 DOM 更新里，观察器能记到。
7. **新浏览器合同**：`scripts/smoke/reader-running-title-load-contract.mjs`。S1–S4 跑 3 个视口 × 4 种模式；T1–T2、B1–B2、F1 跑 3 个视口
   的 page 模式。B 用两本不在书架上的书（书一第 2 行 `甲三 当前`，书二第 3 行 `乙四 目标`），书二的目录和目标章正文各延迟 1.5s。

## 12. 实施与验证（2026-11-12）

| 提交 | 阶段 | 内容 |
| --- | --- | --- |
| `11a067ec` | 合同 | 第 11 节实施补充，gap analysis 与 matrix。 |
| `314300c1` | 红测 | 见下。 |
| `c2445c54` | 实现 | `frontend/src/utils/readerRunningTitle.js#runningChapterLabelFor` 经 `readerCatalogRow` 取行；`Reader.vue` 的 `runningChapterLabel` 改由它派生，删掉页面里的 `formatRunningChapterLabel`；`useReaderDocumentTitle` 改用同一个 `readerCatalogRow`（行为不变）；`useReaderRouteSync` 在打开另一本书的那次路由变化里跳过位置监听的 `loadChapter`。 |

`git diff b33eb7a5 c2445c54 -- backend Dockerfile` 与依赖锁文件为空。

**单元**（沙箱外）：

| 代码 | 结果 |
| --- | --- |
| `b33eb7a5` 的产品代码加 `314300c1` 的测试 | 931 条中 918 通过，13 条失败，全部是新增或改写的断言：`readerRunningTitle` 5 条与 `readerRunningTitleTransaction` 6 条（模块不存在）、`readerRouteSync` 1 条（实际 `[start, book, chapter 3, jump]`）、帧合同 1 条（仍从 `chapter.value?.title` 派生）。另在只补上辅助函数、路由同步仍是旧代码的状态下单独跑过事务测试：两条换书用例仍在“no chapter of the new book may load against the previous catalog”上失败（实际 `[3]`），证明它们锁的是第二层机制。 |
| `c2445c54`（工作树里另有切片 3 的两条单元） | 933/933；`vite build` 通过。 |
| `b650eede`（四个切片都完成后） | 935/935。 |

**浏览器**：go1.26.11 二进制加 `vite build` 生产前端；红测基线是错误面切片之后的前端（`127.0.0.1:18492`），修复是 `c2445c54`
（`:18494`），各自全新的 data/cache/library/store 与随机 JWT secret；服务与 Playwright 都在沙箱外。

| 合同 | 红测基线 | 修复后 |
| --- | --- | --- |
| `reader-running-title-load-contract.mjs`（24 例） | 24/24 FAIL，取值与第 4 节一致：S1 `第 2 章 第一章 起点`，S2 `第 5 章 第二章 延迟`，S3 加载中 `第 6 章 第二章 延迟`，切到缓存章时还记到一次中间值 `第 3 章 第二章 延迟`，S4 重新加载中 `第 5 章 第三章 缓存`；T1 `第 6 章 第一章 起点`，T2 `第 5 章 第六章 迟到`；B1、B2 都出现 `第 4 章 甲三 当前`，并在新书目录到达前请求了新书第 3 行正文；F1 依次是 `第 1 章`、`第 4 章`、`第 4 章 甲四 预载` | 24/24 PASS（4 种模式下 S1–S4 都是目标章标签，记录里没有异行值；B 只出现旧书完整标签、空串和新书目标章，目录到达前没有正文请求；F1 目录到达前只有空串） |
| `reader-epub-contract.mjs`（真实服务） | 1440×900 失败：内链导航记录到 `["第 3 章 第一章（下）","第 3 章 第二章"]`（第 2 行来自预载的内存缓存，中间值存在于一次 DOM 更新） | 3 个视口通过 |
| `reader-continuous-contract.mjs`（加固为精确相等） | 通过 | 通过 |

浏览器层面的 M3（只去掉路由同步的跳过，单独构建，`:18495`）：B 的 6 例全部失败，都出现 `第 4 章 甲四 预载`（旧书另一章的完整标签），
目录到达前请求了新书第 3 行。第二层机制因此不是多余的。

**临时变异**（单元，改工作区后按原文恢复，逐字节核对）：M1 运行标题改回 `chapter.value?.title` 和 M2 序号改取已提交章节，都会让帧合同失败；
M3 去掉路由同步的跳过会让路由同步用例和两条换书事务用例失败；M4 目录行未知时显示序号会让 5 条失败；M5 去掉加载器的 `isCurrent()`
会让迟到用例和既有的加载器用例失败。

**回归**（`:18494`，产品代码与 `c2445c54` 相同）：`reader-error-surface-contract` 48/48、`document-title-contract`、`reader-text-modes-contract`、`reader-reauthentication-contract`（4 视口）、`reader-mobile-contract`（含 iPad）、`reader-audio-contract`、`openreader-smoke`（3 视口）、`reader-desktop-panel-controls-contract` 120/120（含 C5）全部通过；运行标题合同 24/24、EPUB 3 视口与连续合同见上表。

**其余门**：Go `test`/`vet`/`race` 跳过（后端无改动）；依赖 advisory、Compose 与卷门跳过（依赖、Dockerfile、数据格式都没有改）。

**状态**：R1–R6 **implementation-complete / regression-validated**。Git：分支 `claude/reader-error-title-progress-59bcb6`，未合入 main。镜像：未构建，
未推送。生产：未访问，未操作。第 10 节第 3 条（失败态进度载荷）仍未盘点。

## 13. 发布（2026-11-13）

本节内容（运行标题 R1–R6）随本次合并发布。合并提交 `4856215`（闲读）、`310745d`（书籍上限与记住登录信息）、`f5bfd5d`（Reader P0）、`6175cae`（桌面“左右滑动”两项），main 由 `f82e0f5` 快进到 `f8786ab`（ls-remote 核验，未强推）。

推送前在合并树上：后端 `go test ./...` 31 个包与 vet，race api/db/services（api 1088.9 秒）无数据竞争；前端 971/971 与 build；公告门自测 114/114，现场 Go 与前端扫描 0 个发现（推送前重跑）；compose 配置、存储 HTTP 合同、浏览器 smoke 接线 10/10；secdb main 与 11-10 快照一致。真实 Go 服务、全新数据目录下浏览器合同全部通过：openreader-smoke，标签页标题四个流程（壳标题 `闲读`），错误页 48 例，运行标题，桌面面板正文控件连续 3 次 120/120（`C1 flip` 36/36），晚到 flip 设置，D4 9/9，设置位置，TTS 五个视口，连续，重新认证四视口，记住登录信息，书籍上限，注册限制，移动（含 iPad），会话生命周期，首页会话隔离，文本模式，EPUB，音频，书源调试。服务日志没有 panic；唯一的 5xx 是注册限制合同关页时被取消的 `PUT /api/settings/shelf`（`settings.go` 把取消的请求记为 500，原有行为，另起切片）。合并适配：`f45f766` 让“闲读”技术名对照改查 `utils/authToken.js` 的 `AUTH_TOKEN_KEY = 'openreader_token'`，`f8786ab` 让 D4 合同从 `da438bc7` 之后的 `.rail-item[aria-label="设置"]` 打开设置，断言都没改。合入 flip 分支前，`C1 flip` 在 main `f82e0f5`（三次共 8/36 失败）、Reader 分支与合并树上都偶发超时，原因是晚到的账号设置让桌面停在 flip；合入后消失。

可信 Actions run `43400193943` 43/43 步成功（09:18–09:35 UTC）：两平台 OS 门 `pass-with-unknown-origins`（16/16 包、110/109 个 APK 文件、74 组比较无待修复项，secdb main 11-12 22:52:32，复审期限 2026-12-10），Go 公告门 go1.26.11 0 个发现（250 条公告，x/crypto pin 复审期限 2026-12-16），前端公告门 0 个发现，卷门与平台核验通过。

镜像：`ghcr.io/changshengyu/openreader:f8786ab` 与 `latest` 指向 OCI index `sha256:0066943b9fb879041a503391723bf9bc4b31ee19aee06d273073b6ac4142890f`；linux/amd64 `sha256:6944f67944131f2d90e15ccd83e56d6b808ec3cbb4a4f8474d808f15fa064d13`，linux/arm64 `sha256:2537a5de7dc67046ee4449db329c77d0028ab03d291cfe142b290ec4c0bfa729`，另有两个 attestation。两平台 revision 标签均为 `f8786abe2679ca34b76b6aaa6017558225662ecd`；两平台 `/app/openreader` 均由 go1.26.11 构建，`/app/public/index.html` 标题为 `闲读`（逐层校验 sha256 后读取）。

状态 **released / Docker-published / awaiting-device-verification**；生产未访问，实际运行版本未知。
