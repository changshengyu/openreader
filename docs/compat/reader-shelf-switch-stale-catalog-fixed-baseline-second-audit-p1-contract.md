# Reader 书架换书时新书目录未到达的旧目录竞态盘点（P1）

状态：**contract-only**。本切片只写文档；Reader 产品代码、单元合同和浏览器合同都没有改。第 4 节的复现用的是会话
scratchpad 里的临时探针（真实 composable 加真实 Vue 调度，网络为替身），没有入库，也没有跑真实浏览器。红测与实现按
第 9 节在后续阶段进行。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。约定路径
`/private/tmp/changshengyu-reader-dev` 不存在，本会话在 scratchpad 浅取同一提交（`git rev-parse HEAD` 为 `fa22f271…`）。
当前基线：`OpenReader@5bc38a3`（等于 `origin/main`，本分支 `claude/ecstatic-volhard-4d53ee`）；
`git diff 186c762 5bc38a3 -- frontend backend scripts` 为空，Reader 产品代码与已发布的 `186c762` 相同。本文 OpenReader
行号都以 `5bc38a3` 为准。标签页标题切片（`claude/xenodochial-carson-5004c3`，未合入）只给 `Reader.vue` 加一行
`useReaderDocumentTitle` 调用并删掉桌面计数，不改本文涉及的加载路径；合入后 `Reader.vue` 行号偏移 1 行左右，内容不变。

## 1. 范围

- 对象：Reader 实例保持挂载时，一次路由导航同时改变书籍（`route.params.id`，或从 remote-reader 会话换到书架书）和位置
  参数（`chapter`/`offset`/`percent`），从路由变化到新书目录应用、目标章提交之间的这段时间，下称“换书窗口”。主要入口是
  Reader 书架面板 `useReaderShelf.select`（`useReaderShelf.js:62–73`）。`App.vue:4–5` 的 Reader 只以 `sessionGeneration`
  为 key（`:59`），换书复用同一个实例，所以 `useReaderRouteSync` 的两个监听都会触发。
- 后果：正文请求（目标章和附近预载）、控制台错误、窗口内的正文区和错误块、内存与浏览器章节缓存、进度写入（本地、
  远端、广播、WebDAV 镜像）、`N%` 书籍进度、目录面板、书架换书路由里的 `percent`。运行标题和标签页标题只交叉引用已有
  裁决。
- 不含：首次进入（挂载时 `useReaderPageLifecycle` 直接调 `loadBook`，路由监听不触发）；同一本书内的位置导航；换源和
  目录刷新（各有合同）；“换书成功”提示与目录的缓存/网络策略（第 10 节）。
- 起因：标签页标题合同第 12 节的观察。`scripts/smoke/document-title-contract.mjs` 第 7 步挂起新书目录时，书 3 若只有
  2 章，会请求 `/api/books/3/chapters/2/content` 并得到 404；该切片把夹具里的书 3 设为 3 章避开，没有放宽失败收集。

## 2. 上游证据（fa22f271）

### 2.1 换书路径

| 位置 | 行为 |
| --- | --- |
| `web/src/components/BookShelf.vue` L116–131 `changeBook` | 用书架行构造新的 `readingBook`：`name`、`bookUrl`、`index`（`book.index ?? book.durChapterIndex ?? 0`）、`type`、作者、来源等，**不带 `catalog`**，再 emit `changeBook`。 |
| `web/src/views/Reader.vue` L11、L913–919 `changeBook` | 提示“换书成功”，关闭书架弹层，`show = false`，`setReadingBook(book)`，`loadCatalog(true, true)`。不发其它请求。 |
| 同文件 L927–963 `loadCatalog` | 目录返回后 `book = { ...readingBook, catalog }` 写回 store，再 `getContent(book.index \|\| 0)`；初始化时目录失败显示“获取章节目录失败”。 |
| 同文件 L998–1045 `getContent` | 显示加载遮罩，写 `readingBook.index`；`catalog[index]` 不存在时先刷新一次目录，仍没有就提示“请更新目录”（L1023–1036）；否则按新目录行的 `index` 请求正文（L1045）。 |
| 同文件 L1047–1053、L1091–1097 | 正文成功和失败回调都先比较 `bookUrl` 与 `readingBook.index`，已换书或换章就丢弃。 |
| 同文件 L1054–1057、L1325–1336 `saveBookProgress` | 只在正文 100ms 内返回（视为缓存命中）时保存进度，载荷只有 `url` 和 `index`。服务端 `BookController.kt` L390–429 用自己的目录取章节，`index >= chapterList.size` 时拒绝（L421–422），标题由服务端填写，并镜像到 WebDAV（L428）。 |
| 同文件 L555–565 | `catalog` 为 `readingBook.catalog \|\| []`，`chapterIndex` 为 `readingBook.index`。换书窗口里 `catalog` 是空数组。 |
| 同文件 L692–700 `readingProgress` | 目录为空时进度文字为空串。 |
| 同文件 L232 | `.content-inner v-if="show"`：`changeBook` 之后正文区立即隐藏；目录到达后 `getContent` 盖上加载遮罩，正文返回后再显示。 |
| 同文件 L460–465 `readingBook` 监听 | `bookUrl` 变化时 `startSavePosition = false`，`autoShowPosition()` 在内容显示后恢复段内位置。 |
| 同文件 L466–491 `currentPage` 监听 | 只在当前章剩两页时预读下一章，并以 `readingBook.catalog.length - 1` 为界。 |
| `web/src/App.vue` L779–810 `getBookContent` | 正文缓存 key 为 `书名_作者@bookUrl@chapterContent-<index>`，取自当前 `readingBook`。 |

### 2.2 上游可见行为

1. 换书时只请求新书目录；新书目录到达之前不请求正文，不保存进度。
2. 目标章来自新书书架行的 `durChapterIndex`，并用新书目录校验范围；正文请求和缓存都属于新书。
3. 换书窗口里正文区隐藏，进度文字为空，目录面板为空（`catalog` 为空数组）；mini 顶栏保留旧书章节名（运行标题合同
   2.4 已记录）；弹出“换书成功”提示。
4. 预读只有“下一章”，以当前书的目录为界。
5. 进度只传章节序号，服务端按自己的目录校验和补全。

## 3. 当前 OpenReader（5bc38a3）

### 3.1 两个路由监听

- `Reader.vue:2020–2037` 调用 `useReaderRouteSync`。`bookId` 监听（`useReaderRouteSync.js:18–25`）调 `loadReaderBook()`；
  `positionQuery` 监听（`:27–51`）在路由 `chapter` 与 `currentIndex` 不同、或带 `offset`/`percent` 时调
  `loadChapter(index, offset, { restorePercent, saveAfterLoad: true })`（`:36–48`）。两个监听互不知道对方，同一次路由
  变化会都触发。`searchQuery` 监听（`:53–55`）在任何路由变化时也会触发（getter 每次返回新数组），但没有 `line`/`q`/
  `bookmark` 参数时 `jumpToRouteLine` 直接返回（`useReaderSearchNavigation.js:125–143`）。
- 执行顺序：两者都是组件 setup 里的 `pre` 监听，`job.id` 都是 Reader 实例的 uid，调度器对同 id 的 `pre` 任务保持入队
  顺序（`@vue/runtime-core` 3.5.42 `findInsertionIndex`、`queueJob`）。入队顺序取决于两者在 `currentRoute` 依赖上的订阅
  顺序：`bookId` computed 最晚在 `bookId` 监听创建时订阅路由，`positionQuery` 监听在它之后创建；`Dep.notify` 从尾到头
  遍历订阅者、`batch` 头插，结果按订阅顺序触发（`@vue/reactivity` 3.5.42 `notify`、`batch`）。所以 `bookId` 监听先运行。
  第 4 节的探针确认了这个顺序。
- 换书：`useReaderShelf.select`（`useReaderShelf.js:62–73`）先强制保存旧书进度（`:65`），再 `router.push` 到新书。query 由
  `readerRouteQueryFromBook(item, progress, item?.chapterCount || fallbackCount)` 生成（`readerRoute.js:9–21`）：总有
  `resume=1`；有进度时加 `chapter`，`offset > 0` 时加 `offset`，能算出段内比例时加 `percent`。`fallbackCount` 是**当前
  （旧）书**的 `chapters.length`（`useReaderShelf.js:67`、`Reader.vue:689`）。

### 3.2 换书窗口里的执行顺序

1. `bookId` 监听：`onBookLoadStart`（`Reader.vue:2028–2032`，只清删书标记、恢复进度保存、清错误）→
   `useReaderBookLoad.load()` 同步执行到第一个 `await`：取消计划中的进度保存（`useReaderBookLoad.js:83`），发书签和进度
   请求（`:107–112`），读本地缓存进度（`:113`），用书架行同步替换 `book`（`:114–115`，书架运行时合同要求的即时初始化），
   然后 `await loadCachedChapters`（`:121`）。
2. 同一轮 flush 里的 `positionQuery` 监听：`loadChapter(chapter, offset, { restorePercent, saveAfterLoad: true })`。此时
   `book` 已经是新书，`chapters` 还是旧书目录。
3. `useReaderChapterLoader.load`：按旧目录长度截断目标（`useReaderChapterLoader.js:20–23`）并写 `currentIndex`（`:24`）；
   scope key 已是新书（`:25`）；目标不在内存缓存时显示“正在加载章节...”（`:40–47`）；`loadContent` 用新书对象和新书 id
   发请求（`useReaderChapterContent.js:41–44`），浏览器缓存 key 由新书的 `url`/`bookUrl`/`libraryPath` 派生
   （`bookChapterCache.js:5–26`）。
4. 正文成功：提交 `chapter`/`content`（`useReaderChapterLoader.js:58–95`），恢复位置（`:114`），`preloadNearby(targetIndex)`
   （`:117`）。`useReaderChapterContent.preload` 用 `options.chapters` 的长度计算 `±2`（`useReaderChapterContent.js:125–136`，
   `readerChapterWindow.js:39–55`），这时仍是旧目录。之后 `saveAfterLoad` 触发 `saveProgress({ force: true })`（`:118–119`）。
5. 正文失败：设置 `chapterLoadError`（`:128–134`），正文区显示错误文字和“重新加载”（`ReaderChapterContent.vue:4–7`）。
6. 新书目录到达：`chapters` 换成新目录（`useReaderBookLoad.js:136`），`currentIndex` 取已存进度或路由（`:147–155`），再
   `loadChapter(..., { saveAfterLoad: false })`（`:169–172`）。新 generation 让旧加载失效（`useReaderChapterLoader.js:14–15`、
   `:28–30`）；同一下标的进行中请求被复用（`useReaderChapterContent.js:49–55`），不同下标的被 abort；这次加载只
   `markProgressSaved`，不写进度（`useReaderChapterLoader.js:121`）。

窗口从路由变化开始，到 `useReaderBookLoad.js:136` 结束。它包括目录读取（`cacheFirstRequest`：浏览器缓存命中是一次
IndexedDB 读取，未命中是 `GET /api/books/:id/chapters`）；没有本地缓存进度时还要等 `GET /api/progress/:id`（`:131–133`）；
书架里没有这本书时还要等书籍详情（`:125–129`）。过时加载要在窗口内走完自己的正文读取（内存命中最快，其次是 IndexedDB，
最后是网络）、`nextTick` 和位置恢复，才会走到预载和进度写入。

### 3.3 窗口里由旧目录派生的值

| 值 | 代码 | 换书窗口里的取值 |
| --- | --- | --- |
| 目标章 | `useReaderChapterLoader.js:20–24` | `min(路由 chapter, 旧目录长度 - 1)` |
| 预载范围 | `useReaderChapterContent.js:125–136` | 目标章 `± 2`，以旧目录长度为界 |
| 进度 `percent` | `useReaderLocalProgress.js:18–34` → `readerProgressPersistence.js:62–91` | `(目标章 + 段内比例) / 旧目录长度` |
| `N%` 书籍进度 | `useReaderProgressControls.js:13–18` → `readerPagination.js:5–12` | 同上 |
| 桌面 `N / total` | `Reader.vue:148`、`:1425` | `新序号 / 旧目录长度`（这个计数已裁决删除：`4092ea2`，实现 `035762d`，未合入） |
| 目录面板 | `useReaderToc`（`Reader.vue:720–744`）读 `chapters` | 新书名下列出旧书目录 |
| 书架换书路由的 `percent` | `useReaderShelf.js:66–71` | 新书书架行没有 `chapterCount` 时按旧书目录长度推算（只影响没有 `chapterPercent` 的旧数据和 WebDAV 进度） |
| 运行标题 | `Reader.vue:1419–1426` | `第 新序号 章` 加已提交的旧书章节名（运行标题合同 4.5 B1/B2） |

### 3.4 服务端与前端错误面

- `GET /api/books/:id/chapters/:index/content`（`backend/api/books.go:2581` 起）：下标不存在时返回
  404 `chapter not found`（`:2599–2603`）；下标存在而远程正文未缓存时会去书源抓取并写服务端章节缓存（`:2609` 起）。
- `PUT /api/progress`（`backend/api/progress.go:59–107`，`backend/services/readingprogress/service.go:85–135`）：只校验书
  属于当前用户、`chapterIndex` 对应的行存在且 `chapterId` 一致（`service.go:98–107`）；`percent`/`chapterPercent` 只截断到
  [0, 1]（`:115–116`）；按 `baseUpdatedAt` CAS 写入（`:121`），配置了 WebDAV 时写镜像（`:129–134`），成功后向同一用户的
  其它客户端广播 `progress_update`（`progress.go:101–104`）。按旧目录长度算出的 `percent` 能通过校验并持久化。
- 前端 `api/client.js:18–35` 只处理 401，没有全局错误提示。预载失败在 `useReaderChapterContent.js:134` 被吞掉，浏览器
  仍会在控制台记录失败的资源请求。

## 4. 复现（composable 级探针，2026-10-17）

### 4.1 环境

- 探针放在会话 scratchpad（未入库），副本和输出在本组会话的共享笔记目录 `shelf-switch-stale-catalog/`。它按 `Reader.vue`
  的 setup 顺序接上真实的 `useReaderBookState`、`useReaderShelf`、`useReaderChapterCache`、`useReaderChapterContent`、
  `useReaderLocalProgress`、`useReaderProgressPersistence`、`useReaderChapterLoader`、`useReaderBookLoad`、
  `useReaderRouteSync`、`createMultiBookChapterMemoryCache` 和 `loadBrowserChapterContent`。路由用 vue-router 4.6.4 的
  `createMemoryHistory`，通过 `router.install` 取得与 `useRoute()` 相同的响应式路由对象。Vue 3.5.42 与 lockfile 一致，
  依赖由 `npm ci` 装在本 worktree 的 `frontend/node_modules`（gitignored）。
- 替身：网络（按书、按下标计时；目录可挂起；越界返回 404；可注入 502）；IndexedDB 换成 Map，通过
  `loadBrowserChapterContent` 的 `getCache`/`setCache` 注入，key 仍由真实的 `chapterCacheKey` 生成；布局和位置恢复立即
  完成并记下 offset/percent；reader 与 bookshelf store 记录 `applyProgress` 和 `PUT /progress`。
- 监听在 `effectScope` 里创建，没有组件实例。此时 `pre` 任务没有 `job.id`，同样按入队顺序执行，与组件里两个同 uid
  监听的相对顺序一致。`useReaderShelf.js` 有两处无扩展名的相对导入，Node 需要一个补 `.js` 的 resolve hook（与 Vite
  的解析结果相同）。
- 没有跑真实浏览器（会话沙箱禁止 loopback 监听）。浏览器侧证据是标签页标题切片第 7 步的 404 观察，与下表场景 A
  的请求完全一致。

### 4.2 结果（时间从书架点击算起）

| 场景 | 设置 | 观察 |
| --- | --- | --- |
| A 新书更短、无进度（标题切片第 7 步的形状） | 书 2（3 章）在第 1 章 → 书 3（2 章，无进度，路由 `?resume=1`），书 3 目录挂起 300ms | 10ms `bookId` 监听先运行并请求目录；同一时刻 `positionQuery` 监听调 `loadChapter(0)`，`book` 已是书 3，目录还是书 2 的 3 行。31ms 书 3 第 0 章 200，写入书 3 的浏览器缓存和内存缓存并提交；`preloadNearby(0)` 按 3 行请求书 3 第 1、2 章，**第 2 章 404**；同时 `PUT /progress` 书 3（第 0 章，percent 0）。311ms 目录到达，按 2 行重新加载第 0 章（内存命中）并预载。 |
| B 已存章节超出旧目录 | 书 1（3 章）在第 2 章 → 书 3（12 章，已存第 9 章、offset 300、段内 0.5；路由 `?resume=1&chapter=9&offset=300&percent=0.5`），目录挂起 300ms | `loadChapter(9)` 被旧目录截断为 **2**：请求并**提交书 3 第 2 章**（“乙3”），再预载书 3 第 1、0 章；`PUT /progress` 书 3：`chapterIndex 2`、`offset 300`、`chapterPercent 0.5`、`percent 0.8333`（=(2+0.5)/3，旧目录长度）。308ms 目录到达后加载第 9 章并预载 7、8、10、11，只 `markProgressSaved`。结束时**服务端和本地进度都是第 2 章**（本地为 `pendingSync`），屏幕上是第 9 章。 |
| C 过时请求失败 | 书 1（3 章）→ 书 3（6 章，已存第 1 章），书 3 第 1 章第一次请求返回 502，目录挂起 300ms | 29ms `chapterLoadError` 变为 `failed to load chapter content`，到 308ms 目录到达才清空；随后第 1 章**再请求一次**（第二次）并成功。 |
| D 对照：目录先到 | 同 C 的书，目录 5ms，过时请求 200ms | 13ms 目录到达，按新目录加载第 1 章，复用进行中的同一个请求；没有多余请求，窗口里没有进度写入。 |
| E 对照：同下标、路由无 offset/percent | 书 1 在第 0 章 → 书 3（无进度） | `positionQuery` 监听不加载，目录到达后才请求。窗口里 `chapter`/`content` 仍是书 1 的。 |

- 所有场景里浏览器缓存写入的 key 都属于新书（例如 `user:1@书三_甲@library/3.txt@chapterContent-2`），值也是新书该下标
  的正文；内存缓存同样。没有跨书写入。
- 场景 A、B 结束时，`cachedChapters` 标记里还有旧书的下标（第 10 节第 2 条）。
- 单元红测草案（8.1 第 1 项）在 scratchpad 里按同样的 `shallowRef` 路由跑过：当前调用序列为
  `[["start"],["book"],["chapter",9,300,{"restorePercent":0.5,"saveAfterLoad":true}],["jump"],["jump"]]`，两个 `jump` 分别来自
  `positionQuery` 和 `searchQuery` 监听。

## 5. 差异分类

| # | 后果 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | 新书目录应用前的目标章请求 | 换书只请求目录；目录到达后按新目录 `catalog[index]` 请求一次 | `positionQuery` 监听在窗口里按旧目录截断后请求（A、B、C）。已存章节不小于旧目录长度时，请求并显示的是错误的一章（B：第 2 章代替第 9 章）；请求失败后同一章再请求一次（C） | **`must-fix`**：违反已签收的“章节目录仍是定位章节的权威输入”（书架运行时合同第 14 行）和“不提前显示错误章节”（同合同“本批不授权的变化”） |
| 2 | 附近预载的范围 | 只预读下一章，以当前书目录为界 | 以旧目录长度为界：新书更短时请求越界下标并得到 404（A；标题切片第 7 步的 `/api/books/3/chapters/2/content`）；截断后围绕错误的章预载（B）；远程书的在界下标会去书源抓取并写服务端缓存 | **`must-fix`**。预载半径 2 是已签收的运行时适配，不重开；只要求范围来自当前书的目录 |
| 3 | 控制台错误 | 没有这类请求 | 预载的 404 在应用里被吞掉，浏览器控制台仍记录失败的资源请求；smoke 的失败收集会命中，标题切片只能改夹具避开 | **`must-fix`**，随第 2 项消失 |
| 4 | 过时请求失败时的错误块 | 没有这类请求，正文区隐藏 | 窗口里显示错误文字和“重新加载”（C 中约 280ms），目录到达后重复请求 | **`must-fix`** |
| 5 | 窗口里的正文区 | `show = false` 立即隐藏；目录到达后 `getContent` 显示加载遮罩 | 不触发过时加载时（E），继续显示旧书正文，而 `book` 已是新书；触发时先显示“正在加载章节...”，随后可能是第 1 项的错误章或第 4 项的错误块 | **`must-fix`**：窗口里不显示旧书正文，也不显示按旧目录选出的新书章节。用现有“正在加载章节...”代替上游的空白区，判 **`acceptable-change`**（加载提示出现得比上游早，最终状态相同） |
| 6 | 窗口里的进度写入 | 新目录到达、正文返回之前不保存；进度只传序号，服务端按自己的目录补全 | `saveAfterLoad: true` 在新目录之前强制写入：序号是截断值，`percent` 按旧目录长度计算（B：第 2 章、0.8333，实际位置约 0.79）。先写本地（`pendingSync`）和书架行，再 `PUT /progress`；服务端校验通过、`baseUpdatedAt` 与服务端一致时 CAS 写入，广播 `progress_update`，写 WebDAV 镜像。之后正确位置只被 `markProgressSaved`，服务端和本地一直保留错误值，直到下一次保存（翻页或滚动；页面隐藏或切到后台时的强制后台保存，`Reader.vue:2255–2261`；再次换书） | **`must-fix`**：持久化的阅读进度被改错，并同步到其它客户端 |
| 7 | 内存与浏览器章节缓存 | 缓存 key 用新书 `bookUrl` 和新目录的下标 | key 和值都属于新书（`bookId` 监听先于 `positionQuery` 监听，请求发出前 `book` 已换成新书，4.2 全部场景）；按旧目录选出的下标（截断章及其周围的预载）也被写入并标记为已缓存 | key 与值的归属：**`aligned`**，保留为回归断言。多余条目：**`must-fix`**，随第 1、2 项消失；条目本身是正确数据，不需要迁移或清理已有缓存 |
| 8 | 运行标题 | mini 顶栏保留旧书章节名，直到新目录到达 | 新序号加旧书章节名（运行标题合同 4.5 B1/B2）；截断时先是 `第 3 章 甲三`，过时正文提交后是 `第 3 章 乙3`，目录到达后经过 `第 10 章 乙3`（运行标题合同第 1 项）才是 `第 10 章 乙10`（由探针里的 `currentIndex`、`chapter` 与 `Reader.vue:1419–1426` 推出） | 运行标题合同（`15eda53`，第 5 节第 11 项、R5）已判 **`must-fix`**，本文不重新裁决；截断造成的错误章归入第 1 项。两份合同应共用同一修复机制（第 7 节） |
| 9 | 标签页标题 | `书名 - 章节名`，换书窗口里保留旧标题 | 标签页标题实现（`4a0aae8`，未合入）只在目录行 `bookId` 等于 `book.id` 时取章节名，窗口里只显示新书名 | **`aligned`**（标签页标题合同 R3），本文不改 |
| 10 | `N%` 书籍进度 | 目录为空，进度文字为空 | 按旧目录长度计算（B：过时正文恢复到段内 0.5 后为 83%，目录到达后为 79%；按探针状态和 `readerPagination.js:5–12` 计算，未在浏览器读取） | **`must-fix`**：窗口里为空，不用旧目录长度 |
| 11 | 目录面板 | 目录为空 | 窗口里打开目录会在新书名下列出旧书目录，点击会按旧目录的下标跳转新书（读代码推断，未复现） | **`must-fix`**：窗口里不列出旧书目录 |
| 12 | 书架换书路由的 `percent` | 书架行只传 `index` | 新书书架行没有 `chapterCount` 时用旧书目录长度推算段内比例（只对没有 `chapterPercent` 的旧数据和 WebDAV 进度）；`resume=1` 时书籍加载不用路由 `percent`，只有第 1 项的过时加载会用到它 | **`must-fix`（低）**：不用旧书长度；新书长度未知时不带 `percent` |

没有判为 `intentional-redesign` 的项：仓库里没有文档把“换书时先按旧目录加载”作为设计提出过，`useReaderRouteSync` 抽取时
（`docs/upstream-alignment-audit.md` 2026-07-01 条）只说明保持原有顺序。也没有 `unknown` 项；上游“换书成功”提示不是本竞态的
后果，放在第 10 节。

## 6. 已签收、不重开

- 书架运行时合同（`shelf-reader-runtime-contract.md` 第 14 行和 2026-07-17 实施记录）：从书架进入或换书时，用书架行即时
  初始化 `book`，书籍详情不阻塞目录和首章。本文只要求它同一行里的“章节目录仍是定位章节的权威输入”在 Reader 内换书时
  也成立。
- 正文请求生命周期合同（`reader-chapter-content-request-lifecycle-fixed-baseline-second-audit-p0-contract.md`）：generation、
  abort、scope key、同章进行中请求复用、409 重取、浏览器缓存按用户和书隔离。现状里迟到响应不回写（场景 D），保持。
- 预载半径 2（同合同第 3 节第 4 条；`reader-remote-chapter-concurrency-device-feedback-p0-contract.md` 第 76 行）。
- 按进度续读的优先级（`resume`、显式 `chapter`/`offset`/`percent`、已存进度、服务端协调，见 `readerBookLoad.test.mjs`），以及
  离开旧书前对旧书的强制保存（`useReaderShelf.js:65`）。
- 运行标题合同（`15eda53`，`claude/jovial-wilson-889933`，未合入）的全部条目，含 R5；标签页标题合同（`c8ebd30`/`c75e730`，
  实现 `4a0aae8`，`claude/xenodochial-carson-5004c3`，未合入）的 R1–R6 与 D1；桌面 `N / total` 的删除（`4092ea2`、`035762d`）。
- 进度持久化的 CAS、keepalive、clientId、冲突重试和 WebDAV 镜像格式；后端进度接口的校验规则。
- API、数据库、浏览器存储 key、持久文件、镜像与生产：本切片都不改，不在本地构建或推送镜像，不做生产操作。

## 7. 目标合同

R1. 单一定位入口：Reader 不重新挂载、一次路由变化同时改变书籍身份（`bookId`，含 remote 会话与书架书之间）时，只由书籍
  加载流程定位章节：先应用新书目录，再按现有规则（`resume`、`chapter`、`offset`、`percent`、已存进度）选目标章。新书
  目录应用之前不发任何正文请求（目标章和预载）。这类变化里，`positionQuery` 监听不调用 `loadChapter`，`positionQuery`
  和 `searchQuery` 监听都不单独执行路由定位（书籍加载结束时已调用 `jumpToRouteLine`，`useReaderBookLoad.js:190`）。只改
  位置参数的同书导航不变。

R2. 目录归属：为书 B 计算的下标、范围和比例（目标章截断、预载范围、进度 `percent`、`N%`、目录面板、书架换书路由的
  `percent`）只能用 B 的目录或 B 自己的 `chapterCount`。B 的目录未知时：不发正文请求，不写进度，`N%` 为空，目录面板不
  列出其它书的目录，路由不带推算出来的 `percent`。任何时候都不请求 B 的目录范围以外的下标。

R3. 进度：从换书开始，到书籍加载流程提交 B 的目标章之前，不为 B 写本地进度（`applyProgress`、书架行）或远端进度
  （`PUT /progress`）。离开 A 前对 A 的强制保存不变。之后 B 的第一次写入用 B 的目录长度。

R4. 过渡态：换书开始后，正文区立即进入现有加载态（“正在加载章节...”）；不显示 A 的正文，不显示按 A 的目录选出的 B 的
  章节，也不显示过时请求产生的错误。B 的目录或目标章本身失败时，按现有错误面显示。运行标题按运行标题合同 R5，标签页
  标题按标签页标题合同 R3。

R5. 缓存：内存缓存、浏览器缓存和已缓存标记只因 R1、R2 允许的请求而写入；key 仍按 B 的 `url`/`bookUrl`/`libraryPath`
  （现状已满足，作为回归断言）。

R6. 不变：第 6 节所列各项。不新增 API、数据库字段、浏览器存储 key 或持久文件；后端不改。

建议的实现形状（不强制）：

- `useReaderRouteSync`：让 `positionQuery` 和 `searchQuery` 监听知道书籍是否变化（例如把 `bookId` 加进监听源，或记录上次
  处理时的 `bookId`），书籍变化时跳过。这正是运行标题合同第 7 节 R5 列出的选项之一，两个切片应共用。
- 书籍加载开始时（`onBookLoadStart`，或 `useReaderBookLoad.load` 第一个 `await` 之前）：让章节加载器失效
  （`useReaderChapterLoader` 的 `invalidate`，abort A 的进行中请求），进入加载态，并让 `chapters` 不再代表 B——清空，或让
  派生值核对目录行的 `bookId` 与 `book.id`（书架书的目录行带 `bookId`，`backend/models/models.go:413`；remote 会话的行是 0，
  处理方式见标签页标题合同 R3）。如果选择清空，`N%` 要在目录为空时返回空串：`readerBookProgress` 现在以 1 作分母，会
  得到 100%。
- `useReaderShelf.select`：去掉 `currentChapterCount` 兜底；新书没有 `chapterCount` 时不带 `percent`。
- 防御：`useReaderChapterContent.preload` 在目录行的 `bookId` 与当前书不一致时不预载。

## 8. 测试计划（后续阶段执行，本切片不写测试）

### 8.1 单元合同（`node --test`）

1. `frontend/tests/readerRouteSync.test.mjs` 新增“一次路由变化同时改书和章节时只加载书”：像 vue-router 一样用一个
   `shallowRef` 路由整体替换 `params` 和 `query`，`bookId`、`positionQuery`、`searchQuery` 都从它派生；从
   `{ id: 1, chapter: '2' }` 换成 `{ id: 3, resume: '1', chapter: '9', offset: '300', percent: '0.5' }`。断言调用序列为
   `[['start'], ['book']]`。现状（第 4.2 节已实测）多出 `['chapter', 9, 300, { restorePercent: 0.5, saveAfterLoad: true }]` 和
   两个 `['jump']`。对照：同一本书只改 `chapter` 仍加载章节（现有第 55 行测试），只改搜索参数仍只定位（现有第 95 行测试）。
2. 新增 `frontend/tests/readerShelfSwitchStaleCatalog.test.mjs`（集成测试，接法同第 4.1 节探针：真实 composable、
   `createMemoryHistory` 路由、网络替身挂起目录）。`useReaderShelf.js` 的无扩展名导入在 `node --test` 下解析不了，测试直接
   `router.push`，query 用 `readerRouteQueryFromBook` 生成；若实现阶段给该文件补上 `.js`，可以改用真实的 `select`。
   - 场景 A：目录挂起期间书 3 的正文请求为 0；目录到达后请求的下标都小于 2；没有 404。现状：挂起期间请求第 0、1、2 章，
     第 2 章 404。
   - 场景 B：挂起期间没有正文请求、没有 `PUT /progress`、`applyProgress` 里没有书 3；全程没有提交过书 3 第 2 章；结束时
     服务端和本地进度仍是第 9 章。现状：第 2 章被请求并提交，进度写成第 2 章、`percent` 0.8333。
   - 场景 C：挂起期间 `chapterLoadError` 一直为空；书 3 第 1 章只请求一次。现状：错误持续约 280ms，请求两次。
   - 场景 E：挂起期间正文状态不再是书 1（按 R4 的实现断言，例如 `chapterLoading` 为真，或 `chapter`/`chapterBlocks` 不属于
     书 1）。现状：书 1 的 `chapter`/`content` 保持到新书正文提交。
   - 场景 D（对照，现状已满足）：目录先到时只请求一次，复用进行中的请求。
   - 全部场景：浏览器缓存写入的 key 都属于新书（回归）。
   - 被测模块都已存在，断言按行为失败，不需要把导入失败转换成断言消息。

### 8.2 真实浏览器

1. 新增 `scripts/smoke/reader-shelf-switch-contract.mjs`（模拟接口，写法同 `document-title-contract.mjs`：`page.route` 加目录
   gate），视口 1440×900、390×844、360×800（可加 1024×1366），page 模式，另加一组 scroll2。夹具：书 1（3 章，从第 2 章开始）、
   书 3（12 章，已存第 9 章、offset 300、段内 0.5）、书 4（2 章，无进度）、书 5（6 章，已存第 1 章，第 1 章第一次返回 502）、
   书 6（书架行不带 `chapterCount`，进度只有 `percent`、没有 `chapterPercent`）。全部经 Reader 书架面板点击换书，目标书目录
   挂起 1.5s。记录每个 `GET /api/books/:id/chapters/:i/content` 的时间和状态、每个 `PUT /api/progress` 的请求体、`console`
   error 与 `pageerror`、用 `MutationObserver` 记录正文区文本和错误块、`N%` 标签、换书后的 URL。断言：
   - 挂起期间：目标书没有正文请求、没有 `PUT /progress`；正文区不含书 1 正文，不出现 `.chapter-load-error`，显示
     “正在加载章节...”；`N%` 为空。
   - 释放后：书 3 只请求一次第 9 章，预载下标都在 7–11；第一次 `PUT /progress`（如果有）为 `chapterIndex 9`，`percent` 按
     12 章计算；书 4 没有 `chapters/2` 请求；整轮没有 404，也没有 `Failed to load resource`；书 5 第 1 章只请求一次。
   - 书 6：换书后的 URL 不含按上一本书目录长度推算的 `percent`（夹具数值要让两种长度推算出不同结果，例如
     `chapterIndex 4`、`percent 0.75`：按 6 章得 0.5，按书 6 自己的 10 章超出 1、不带 `percent`）。
   - 预期红点（按第 4 节推断，红测阶段逐视口、逐模式记录实际值）：书 3 是挂起期间出现
     `GET /api/books/3/chapters/2/content`；书 4 是 `chapters/2` 的 404；书 5 是挂起期间出现错误块；书 6 是 URL 里的 `percent`。
2. 标签页标题切片合入后，`document-title-contract.mjs` 的书 3 可以改回 2 章，作为第 7 步的回归。
3. 对照：`reader-continuous-contract.mjs`、`reader-epub-contract.mjs`、`reader-text-modes-contract.mjs`、
   `reader-reauthentication-contract.mjs`、`reader-mobile-contract.mjs`、`remote-reader-contract.mjs`。

### 8.3 回归与临时变异（实现阶段）

- frontend `npm test`（写明新增条数）、`npm run build`。后端不改，Go `test`/`vet`/`race` 跳过并写明原因；依赖 advisory、
  Compose 和卷门不涉及。
- 临时变异只在工作区修改并构建到独立目录，构建后立即还原，不提交：
  - M1：去掉 R1 的书籍变化判断 → 8.1 第 1 项、第 2 项 A/B/C 和 8.2 挂起期间的请求断言失败；
  - M2：保留 R1，但让预载在新目录应用前执行（例如把书籍加载里的 `chapters` 赋值移到 `loadChapter` 之后）→ 预载范围断言失败；
  - M3：去掉换书开始时的加载态 → 场景 E 和 8.2 的正文区断言失败；
  - M4：恢复 `useReaderShelf` 的 `currentChapterCount` 兜底 → 书 6 的 URL 断言失败。
- 会话沙箱禁止 loopback 监听，服务和 Playwright 需要在沙箱外运行。

## 9. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交（本切片）。
2. **排期**：运行标题合同 R5 的修复机制与本文 R1 相同，两者都会改 `useReaderRouteSync.js` 和 `Reader.vue`；桌面计数切片、
   标签页标题切片也改 `Reader.vue`。建议在这些分支落地之后、在其之上切分支，或把本文 R1 和运行标题 R5 放进同一个实现提交。
3. **红测**：8.1、8.2，记录每个视口和模式的精确失败点。提交 `test: expose stale catalog loads on reader shelf switch`。
4. **实现**：最小改动让同一批断言转绿。提交 `fix: load a switched book from its own catalog`。
5. **回归**：按 8.3 执行，包括临时变异。
6. **记录**：提交号、各视口与模式的结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
7. **发布**：实现提交改 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；是否合入由用户决定。

## 10. 观察（不在本切片）

1. 上游 `changeBook` 弹出“换书成功”（`Reader.vue` L914），OpenReader 换书没有提示。仓库里没有找到对它的裁决，状态
   `unknown`，需要单独盘点。
2. `useReaderChapterCache` 的已缓存标记 `cachedChapters`（`useReaderChapterCache.js:17`、`:67–75`）不按书区分，换书时也不
   重置：探针场景 A 结束时标记里有下标 2，而书 3 只有 2 章。打开目录会刷新它（`useReaderToc.js:25–27`），所以只影响刷新
   之前的显示。没有在浏览器里复现，需要另行核对目录面板和缓存面板。
3. 上游换书时目录走网络（`loadCatalog(true, true)` → `getCatalog(true)` → `cacheFirstRequest(..., true)` 跳过缓存，
   `web/src/plugins/helper.js` L104–126），OpenReader 先读浏览器缓存、再在后台刷新（`useReaderBookLoad.js:121`、`:184`）。这让
   换书窗口通常更短，但不改变本文的竞态。是否对齐不在本切片。

## 2026-11-15 主线复核

在本机上游仓库 `/Users/yuchangsheng/Documents/reader-dev` 核对固定提交 `fa22f271` 的 `web/src/views/Reader.vue`。OpenReader 起点为 `49392de5`。

上游换书后调用新书目录加载。主线保留最新失败章进度规则，换书由目标目录负责定位，旧目录不会触发预载或影响百分比。目录请求晚到的错误按加载代际丢弃，包括首次加载后切走再返回同书的情况。浏览器拦截目标目录 1.5 秒，桌面与两个移动视口在 page、scroll2 下通过 6/6；补充复审的迟到目录错误由乱序单测覆盖。

## 10.1 换书成功反馈独立切片（2026-11-16）

固定上游对象在 `/Users/yuchangsheng/Documents/reader-dev` 用 `git show fa22f271849d45f93349ae1636223e27b16a4691:web/src/views/Reader.vue` 核对；该 checkout 的 HEAD 不作为合同。OpenReader 起点为 `84f61de2`，主线同提交由集成代理核实。

| 层 | 上游 fa22f271 | OpenReader 84f61de2 | 裁决与覆盖 |
| --- | --- | --- | --- |
| 可见反馈 | Reader.vue L913–919 的 changeBook 首先 `$message.info("换书成功")`，关闭书架，再换书并加载目录 | useReaderShelf.select 关闭书架、尽力保存旧书进度、router.push；无反馈 | **must-fix**：成功切到不同书时恰好一次 info 提示“换书成功” |
| 成功边界 | 同步替换 readingBook，不等待目录/正文 | Vue Router 异步导航，可能中止、取消、重复或重定向 | **acceptable-change**：等待导航成功且最终 reader 路由是所选书再提示；不等目录或正文，不把后续加载失败当作导航失败 |
| 不变项 | 选择当前书无需换书；不等待保存 | 已签收 P5：保存失败仍切书 | 当前书、重复导航、中止/取消、拒绝或重定向到其它页面不报成功；保存失败而导航成功仍提示一次 |
| API / 数据 | 无此通知相关契约 | 无此通知相关契约 | 不改请求/响应、路由参数、持久化 key、数据库、目录或缓存策略 |

阶段：合同 `f2bf37b0` → 红测 `5ffbb9c7` → 独立实现提交，严格按序。实现只加导航成功回调和 Reader 的 info 消息接线。

验收：用真实 memory router 覆盖成功、保存失败、当前书、重复、守卫中止、守卫抛错、导航取消、重定向到其它书/页面；重复并发选择同一本目标书仅提示一次。浏览器在现有目录 gate 下记录新加入的消息节点，确认导航完成且目录仍挂起时已提示一次、正文/失败章完成后不重复；当前书选择无新提示。覆盖 1440×900、390×844、360×800 的 page 与 scroll2。


### 10.1 验证记录

- `node --test frontend/tests/readerShelf.test.mjs`：红测 14 条、10 绿 / 4 红（正常成功、保存失败后成功、并发重复中的唯一成功、取消旧导航后的新成功均缺少消息）；实现后 14/14 通过。新增 9 条真实 memory router 合同。
- `TARGET_URL=http://127.0.0.1:5182 SMOKE_VIEWPORTS=1440x900,390x844,360x800 CHROME_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' node scripts/smoke/reader-shelf-switch-contract.mjs`：红测六组各 8 个失败（4 次换书在目录挂起和正文完成两个检查点均无 toast），其它旧断言无失败；实现后六组全绿，目录挂起 1500ms，每组含 4 次换书与 4 次当前书选择。书 5 正文失败后仍只有一次导航成功提示。
- `cd frontend && npm test`：1052/1052 通过。首次沙箱内跑时 3 条已有 dependency advisory worker 因 `listen EPERM 127.0.0.1` 失败；允许本机监听后重跑全部通过。
- `cd frontend && npm run build`：通过；保留既有 chunk 大小提示。`git diff --check` 通过。
- 浏览器在沙箱内 Chrome 启动被限制，授权沙箱外重跑通过。后端无修改，集成代理已完成基线 `cd backend && go test ./...`；本切片不重复执行。
- 无发布、推送或生产操作。允许差异仅为上表的 Vue Router 成功边界；目录/正文成功不作为 toast 条件，所有其它合同维持不变。

## 2026-11-16 §10.2 缓存目录标记补充契约

固定上游 `fa22f271` 的 `web/src/components/PopCatalog.vue`：`catalog`、`index`、弹层可见监听调用
`computeCachedCata()`，按当前书的 `书名_作者@bookUrl@chapterContent-<index>` 重新生成标记。
当前 `84f61de2` 的 `useReaderChapterCache` 在上下文变化时只取消任务，保留旧 `cachedChapters`；
新书正文加载通过 `markCached` 合并，故 A 书独有下标会残留到 B。重新打开目录会异步刷新，不能保证刷新前的行正确。

| 层面 | 裁决 | 验证 |
| --- | --- | --- |
| 当前书/来源/remote 会话变化 | must-fix：同步清空标记并取消旧任务；新书正文和显式目录刷新恢复真实标记 | 真实 composable 回归；真实浏览器换书后重开目录，延迟缓存列表返回 |
| 迟到列表读取 | must-fix：上下文切走再返回 A 时，旧 A 读取也不能覆盖新 A 标记 | A→B→A 延迟成功/失败回归 |
| 用户范围 | 保留 snapshot scope 校验；响应式 scope 提供方改变时清空；产品切号由现有 sessionGeneration 卸载 Reader，localStorage token 不是响应式状态 | scope ref 是 composable 防御测试，不代表原地 token 替换有响应性 |
| API/数据 | 无改动，IndexedDB/localStorage 缓存 key、目录和正文 API 不变 | 既有缓存回归 |

本切片只处理目录缓存标记，不改变目录读取策略、换书提示或正文缓存内容。

2026-11-19 验证：`readerInlineChapterCache.test.mjs` 的 9 条用例通过。真实浏览器先打开 A 的目录确认第 3 章有缓存标记，切到 B 后延迟 IndexedDB 游标返回。B 的第 10 章显示已缓存，第 3 章没有标记。桌面和两个手机视口均通过。

## 10.3 重复选择与阅读位置

固定上游 `fa22f271` 的 `web/src/components/BookShelf.vue` L116–130 同步发出选书事件，`web/src/views/Reader.vue` L913–919 同步更新当前书并开始加载目录。上游连点当前书时也会发出换书事件。OpenReader §10.1 保留的交互差异是选择当前书时只关闭面板，成功切换到其他书后才提示。

| 层面 | 当前行为（`1e755c3e`） | 裁决与验证 |
| --- | --- | --- |
| 同一本书的两次选书请求 | 两次都能在 A 书通过当前书判断。若第一笔切到 B 后更新了章节或页内位置，第二笔保存完成后可能通过新的 query 再次导航到 B。 | `must-fix`。保存等待结束后以当前路由核对目标书。若已到 B，保留最新阅读位置与历史记录，本次选择静默结束。 |
| 进度保存 | 旧书进度按 P5 尽力保存。 | `acceptable-change`。保存失败时，仍可导航到不同的目标书。 |

用真实 Vue Router memory history 控制两次保存的返回顺序。依次验证第一次导航到 B、更新 B 的位置、释放第二次保存。最终 B 的完整路径应保持在更新后的位置，路由守卫只应观察到一次书架导航，提示应只有一次。

红测 `8594538c` 有 14 项通过、1 项失败。第二次导航将 `chapter=3&offset=240` 改回 `chapter=0`。加上保存后的路由核对后，书架单测 15 项通过，前端全量 1056 项通过，后端 `go test ./...` 与前端生产构建通过。真实 Chrome 的书架切换回归在 1440×900、390×844、360×800 的 `page` 和 `scroll2` 均通过，目录挂起 1500 毫秒。两次保存的交错顺序由 memory router 单测控制。
