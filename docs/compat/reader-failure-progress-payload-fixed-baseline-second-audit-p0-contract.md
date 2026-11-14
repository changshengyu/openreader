# Reader 失败态进度载荷盘点（P0）

状态：**contract-only**（只写文档，不改应用代码）；用户决定已齐（2026-11-14，第 8、12 节）：D1 选 A，按上游把进度改到失败章；
D2 选 (a)，不重开 R6 第 13 节第 7 条。基线 main `9602dc4`，分支 `claude/reader-failure-progress-inventory`，盘点提交 `feeb256`。
第 4 节的真实浏览器实测用的是会话 scratchpad 里的临时探针，没有入库。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。约定路径 `/private/tmp/changshengyu-reader-dev`
已被系统清理，本次从本机持久克隆 `git archive fa22f27` 出 `web/src` 与 `BookController.kt` 到 scratchpad 阅读。

## 1. 范围

- 对象：Reader 当前章正文或目录加载失败时（主错误页 `.chapter-load-error`、连续模式内的失败块 `.chapter-inline-error`），以及
  正文尚未返回的加载中，Reader 是否写阅读进度、写什么、写到哪里：服务端 `ReadingProgress`，本地 scoped 快照
  `openreader_chapter_progress@<scope>@<bookId>`，书架投影（`bookshelf.books[].progress` 与 `reader.progressByBook`）。
- 模式：page、flip、scroll、scroll2；视口 1440×900（细指针）、390×844 与 360×800（触摸）。桌面上 flip 设置按 page 布局
  （`readerInterfaceMode`，`useReaderMode.js:6–8`），进度 `mode` 字段写界面模式 `page`，所以桌面实际是三种布局。
- 不含：错误页的可见结构与重试按钮（[错误面合同](reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md)
  R1–R8，已发布）；运行标题与标签页标题；`PUT /api/progress` 的校验、CAS 与 WebDAV 镜像
  （[阅读进度 P2 合同](reading-progress-p2-contract.md)，已签收）；WebSocket 同步；成功状态下的位置精度。
- 起因：[运行标题合同](reader-running-title-load-failure-fixed-baseline-second-audit-p0-contract.md)第 10 节第 3 条（读代码推断
  “新序号 + 旧章 id/标题”）；错误面合同第 10 节第 2 条与第 14 节末尾的观察（R6 之后连续模式写 `5:第六章 失败`，并认为
  “上游在失败状态下不保存进度”）。

## 2. 上游证据（fa22f271）

### 2.1 进度存在哪里

| 层 | 位置 | 内容 |
| --- | --- | --- |
| 服务端书架 | `BookController.kt#saveShelfBookProgress`（L1912–1922） | `durChapterIndex`、`durChapterTitle`（服务端目录行标题）、`durChapterTime`；不改 `durChapterPos` |
| 服务端 WebDAV 镜像 | `#saveBookProgressToWebdav`（L2079–2097） | 只在用户 WebDAV 根下已有 `bookProgress/`（或 `legado/bookProgress/`）时写 `书名_作者.json`，`durChapterPos` 固定为 0 |
| 客户端最近阅读 | `plugins/vuex.js#setReadingBook`（L145–175） | `readingBook.index` 存入 `<用户>@readingRecent`；100ms 后把书架项的 `durChapterIndex/durChapterTitle/durChapterTime` 改成该 index |
| 客户端章节内位置 | `Reader.vue#saveReadingPosition`（L2488–2549） | `bookChapterProgress@书名_作者`：按书存一个段落文字偏移，不按章 |

上游客户端没有“章节 id”：保存请求 `Reader.vue#saveBookProgress`（L1325–1336）只发 `{url, index}`（silent，错误吞掉），
服务端 `#saveBookProgress`（L390–430）确认书在书架、读目录、`index` 在范围内后按目录行写标题。序号与标题不可能不一致。

### 2.2 写入点

| # | 写入点 | 条件 | 失败态下 |
| --- | --- | --- | --- |
| W1 | 服务端 `#getBookContent` L489–496：`saveShelfBookProgress` + WebDAV | 书在书架、目录可读、`index` 在范围内、`cache != 1` | 进度在抓正文**之前**写成目标章。正文的各种失败（L515 起：本地文件不存在、EPUB/CBZ 解压失败、章节文件不存在、书源抓取异常等）都在保存之后返回，失败不撤销进度 |
| W2 | `Reader.vue#getContent` L1011–1019：请求前 `setReadingBook({...book, index})` | 每次 `getContent` | `readingRecent` 与书架投影立即改成目标章 |
| W3 | `#getContent` L1054–1057：响应在 100ms 内就补发 `saveBookProgress()` | 视为命中浏览器缓存（服务端没收到请求） | 这一行在判断 `isSuccess` 之前，业务失败的快速响应也会补发；请求出错分支没有。浏览器缓存只存成功正文（`plugins/helper.js#cacheFirstRequest` L132–136），失败正文总会到达服务端，由 W1 写入 |
| W4 | `#deactivated` L372–374：`saveBookProgress()`，index 为 `readingBook.index` | Reader 在 `<keep-alive>`（`App.vue` L3–5）里，离开 Reader 就触发 | 不看 `this.error`：失败后离开会把目标章再写一次 |
| W5 | `#saveReadingPosition` L2488–2549 | `this.error \|\| !startSavePosition` 时直接返回（L2490） | 失败期间不写章节内位置，也不做连续模式的跨章切换与 `saveBookProgress(chapterIndex)`（L2517–2531）。触发它的有滚动 100ms 防抖（L2439–2440）、`watch.content`（L391–398）、`computeShowChapterList`（L1305）和 `unload`（L292） |

预载（`watch.currentPage` L476–483）与连续窗口 `loadShowChapter`（L1193）都带 `cache=1`，服务端不写进度。

### 2.3 失败态逐场景

| 场景 | 服务端进度（W1/W4） | `readingRecent` 与书架投影（W2） | 章节内位置（W5） |
| --- | --- | --- | --- |
| U1 当前章正文失败（任一模式，书架书） | 请求时写成目标章；离开 Reader 再写一次 | 目标章 | 不更新，保留上一章最后的偏移 |
| U2 连续模式目标章失败后继续滚进后一章 | 目标章（`getContent` L1042–1044 先把窗口起点设为目标章，失败分支 L1086–1088、L1113–1115 重算窗口并补齐后一章） | 目标章：`this.error` 挡住 W5，滚动不切换 index 和标题，直到下一次成功的 `getContent` | 不更新 |
| U3 连续模式相邻章失败（窗口里的别的章），滚进失败块 | 滚进时 W5 `saveBookProgress(失败章)`：`this.error` 为假，失败块是普通 `.chapter-content[data-index]`（`Content.vue` L219–262） | 失败章 | 失败块里段落的偏移 |
| U4 目录业务失败（初始化，`loadCatalog` L946–952） | 没有 `getContent`，不写；离开时 W4 按原 index 再写，index 不变 | 不变 | 不更新 |
| U5 目录请求出错（L956–961） | 同 U4 | 不变 | 不更新（`showContent` 从未发出，`startSavePosition` 不会置真） |
| U6 Reader 内换书后目录失败（`changeBook` L913–919） | 不写；离开时按新书书架里的原 index 写，index 不变 | 新书，index 为新书原值 | — |
| U7 请求到不了服务端（断网、超时） | 请求时没写；离开时 W4 补发，仍断网则丢 | 目标章 | 不更新 |

目录面板点当前章：`PopCatalog.vue#gotoChapter`（L155–159）→ `getContent(index)`。对当前章同样重新请求，失败正文不在浏览器缓存里，
所以这就是一次重试，并经 W1 再写一次目标章。

### 2.4 上游可见结果

1. 当前章失败后，“读到哪一章”就是失败章：书架显示失败章标题；离开后再打开（或换设备打开）停在失败章，可以直接重试。
2. 失败期间不更新章节内位置；连续模式失败期间滚动不切换当前章。
3. 目录失败不改变已存的章节序号。
4. 进度的身份只有目录 index，标题由服务端按目录行取，不会出现序号与标题指向不同章。
5. **纠正**：错误面合同第 2.6 节与第 14 节末尾“上游在失败状态下不保存进度”只对客户端章节内位置（W5）成立。服务端章节进度
   在请求时已经写成目标章（W1），离开 Reader 时再写一次（W4）。

## 3. 当前 OpenReader（9602dc4，读代码）

### 3.1 存储与写入管道

- 服务端：正文接口 `GET /books/:id/chapters/:index/content`（`backend/api/books.go:2626`）不写进度；进度只来自
  `PUT /api/progress`（`backend/api/progress.go:59–107`）。`readingprogress.Service.Save`（`service.go:85–135`）按 `(book_id, index)`
  取规范章节，`chapterId` 非零且不是该章时返回 `ErrChapterIdentity`，API 回 `400 {"error":"invalid progress payload"}`；标题一律取
  服务端目录行。这是阅读进度 P2 合同签收的校验，本文不改。
- 前端唯一管道 `useReaderProgressPersistence.save`（`useReaderProgressPersistence.js:45–71`）：
  - `isBlocked` 只看连续窗口事务与删书（`Reader.vue:1809`），不看 `chapterLoadError`、`chapterLoading`；
  - `getPayload` 在 `chapter` 为空时返回 null（`Reader.vue:1810`），否则用 `currentProgressPayload()`；
  - 先 `applyLocal` 乐观写本地快照和书架投影（`useReaderLocalProgress.js:55–73`），再走 keepalive（background，`:73–115`）或
    `reader.saveProgress`（普通，`stores/reader.js:728–755`，`flush` 把服务端错误重新抛出，`:144–147`）。
- 载荷 `readerProgressPayload`（`frontend/src/utils/readerProgressPersistence.js:62–91`）：有可见快照时用快照；快照要求顶部可见元素
  位于 `.chapter-content[data-index]` 内，章节身份取该 index 的目录行（`useReaderViewportProgress.js:132–163`）。没有快照时用
  `chapter`（最近一次成功加载的章节对象）的 `id`/`title` 配 `currentIndex`，offset 与比例由当前视口算。
- 加载器 `useReaderChapterLoader.load`：开始就 `currentIndex = 目标`（`:24`）并 `cancelProgressSave()`（`:37`）；成功后按
  `saveAfterLoad` 保存或 `markProgressSaved`（`:118–122`）；失败分支（`:128–147`）只写 `chapterLoadError` 并重算版面，不改 `chapter`、
  `content`、`chapterFormat`，不写进度。
- 主错误页 `.chapter-load-error`（`ReaderChapterContent.vue:4–9`）不在 `.chapter-content` 里，没有可见快照，载荷退回
  `chapter` + `currentIndex`；连续模式的错误块是 `.chapter-content[data-index]`（`:47–62`），快照取目录行。
- `mode` 字段：普通保存写界面模式 `interfaceReaderMode`（`Reader.vue:1813`，桌面 flip → page）；keepalive 写
  `effectiveReaderMode`（`Reader.vue:1815` → `useReaderProgressPersistence.js:91`）。

### 3.2 失败态下仍可触发的写入

| 触发 | 位置 | 方式 |
| --- | --- | --- |
| 页面隐藏、pagehide、路由离开 | `Reader.vue:2370–2376`、`:2220–2222` | background（keepalive） |
| 卸载 | `useReaderPageLifecycle.js:63–70` | background |
| 返回首页 | `useReaderPanels.js:28–31` | background |
| 书架抽屉换书 | `useReaderShelf.js:62–72`（Reader 传入 `saveCurrentProgress({ force: true })`，`Reader.vue:658`） | 普通保存，`await` 之后才 `router.push` |
| 目录面板点当前章 | `useReaderToc.js:53` → `useReaderNavigation.js:186`（目标就是当前章）→ `:157` `jumpWithinCurrentChapter` → `saveProgress()` | 普通保存，不 `await`、不 `catch` |
| 回到顶部、跳到底部 | `useReaderNavigation.js:252`、`:265` | 普通保存，不 `await`、不 `catch` |
| 滚动同步 | `useReaderScrollSync.js:23–53` | 防抖 1200ms 后普通保存 |
| 移动页码滑块、切换阅读方式、自动阅读、正文搜索跳转 | `useReaderProgressControls.js`、`useReaderMode.js:166`（`Reader.vue:1701`）、`useReaderAutoReading.js:19`、`useReaderSearchNavigation.js:95` | 普通保存，除 `Reader.vue:2537` 一处外都不 `catch` |

### 3.3 逐场景（读代码）

| 场景 | `chapter` | `currentIndex` | 可见快照 | 写出的载荷 |
| --- | --- | --- | --- | --- |
| O1 分页（page、flip，含桌面 flip→page）在成功章之后目标章失败 | 上一章 | 目标章 | 无 | 目标序号 + 上一章 id/标题 + offset 0 |
| O2 首次加载即失败 | 空 | 目标章 | 无 | 不写（`getPayload` 为 null） |
| O3 连续模式目标章失败（R6 窗口 `[错误块, 后一章…]`） | 上一章 | 目标章 | 错误块 | 目标序号 + 目标目录行 + offset 0 |
| O4 连续模式目标章失败后滚进后一章 | 目录行（`syncCurrentChapter`） | 后一章 | 后一章 | 后一章及其偏移；R6 的 `watch(currentIndex)`（`Reader.vue:1113–1122`）同时结束失败状态 |
| O5 连续模式相邻章失败，滚进失败块 | 目录行 | 失败章 | 失败块 | 失败序号 + 目录行 + 块内偏移 |
| O6 首次挂载目录失败 | 空 | 0 | 无 | 不写 |
| O7 Reader 内换书后目录失败 | 上一本书的章节 | 上一本书的序号（`useReaderBookLoad.load` 在 `chapters` 赋值前就抛出，`:121`、`:136`） | 无 | 新书 bookId + 上一本书的章节 id/标题与序号 |
| O8 加载中（正文未返回） | 上一章 | 目标章 | 无（`正在加载章节...` 取代了全部章节块） | 同 O1，offset 按上一章正文和当前滚动位置算 |

另有两条读代码得到、第 4 节实测证实的路径：

- 书架抽屉换书（`useReaderShelf.select`）先 `await` 一次普通保存再 `router.push`；O1 状态下（第 4 节 A5）保存被服务端拒绝，`select`
  抛出，换书不会发生。按读代码，O7、O8 状态下同理。
- 目录面板点当前章走 `jumpWithinCurrentChapter`（滚到章首并保存），不重新加载；失败状态下这一下既不是重试，又在 O1 状态下写出
  被拒的载荷，而且这次 `saveProgress()` 没有 `catch`，拒绝成为未处理的 Promise 拒绝。

## 4. 真实浏览器实测（2026-11-13）

### 4.1 环境

- 产品代码：`9602dc4` 的 frontend 生产构建（`npm run build`，service worker 构建号 `e5c767f8429a3a17`、64 个预缓存文件）和同一
  worktree 用 go1.26.11 编译的 backend，全新的临时 data/cache/library 目录，随机 JWT secret（未输出），监听 `127.0.0.1:18611`。
  会话沙箱禁止 loopback 监听，服务和 Playwright 都在沙箱外运行。Playwright 1.61.1，chromium-headless-shell 1228，Node v26.4.0。
- 真实注册、真实 TXT 导入（8 章，标题与错误面合同夹具相同：`0 第一章 起点` … `4 第五章 前邻`、`5 第六章 失败`、`6 第七章 后续`、
  `7 第八章 终点`；换书目标是 3 章的另一本），`PUT /api/progress` 由真实服务端处理。`page.route` 只拦三种请求：第 5 行正文返回
  后端真实失败体 `502 {"error":"failed to load chapter content","code":"content_unavailable","stage":"content"}`；换书目标的目录
  `500 {"error":"failed to list chapters"}`；或把第 5 行正文挂起 12s（加载中）。
- 每组用一本新导入的书和全新浏览器上下文；阅读设置用 `PUT /api/settings/reader`（force）写入，并预置本地 `reader` 键。1440×900
  用默认上下文（细指针），390×844 与 360×800 用 `isMobile + hasTouch`。
- 步骤：打开 `?chapter=4`，滚动（flip 用 → 键）让服务端得到一个已知位置；`router.push` 到第 5 行；然后依次触发页面隐藏（改写
  `document.hidden` 后派发 `visibilitychange`）、目录面板点当前章（`.toc-item.active`）、`router.push` 回首页（路由离开）；回到
  首页后再调用一次 `bookshelf.loadBooks({ force: true, all: true, settleProgress: true })`（与工作台“刷新缓存”相同）。移动端的
  目录与书架按钮用 DOM `click()` 打开：这里观察的是写入，不是命中。
- 探针与原始结果（scratchpad，未入库）的副本在共享笔记目录 `failure-progress/`。

### 4.2 结果（请求体写作 `chapterIndex:chapterId:chapterTitle@offset`，箭头后是响应状态）

| # | 场景 | 视口与模式 | 写入 | 服务端进度 | 本地快照与书架投影 |
| --- | --- | --- | --- | --- | --- |
| A1 | 读到第 4 行后跳第 5 行失败 | page × 3 视口 | 失败后没有自动写入；隐藏 `5:<第4行id>:第五章 前邻@0` → **400**；目录点当前章同一载荷 1 次（1440）或 2 次（390、360）→ **400**，并有 pageerror `Request failed with status code 400`；离开 → **400** | 停在第 4 行（1440 `@561`，手机 `@296`） | 隐藏后为 `5:<第4行id>:第五章 前邻@0`，目录点击后变 pending；回首页后被服务端的第 4 行记录替换 |
| A2 | 同上 | flip × 3 视口（桌面 mode 字段为 `page`，手机为 `flip`） | 同 A1（目录点当前章各 1 次），全部 **400** | 停在上一次成功位置（1440 的 → 键基线落在第 6 行 `第七章 后续@55`，手机为第 4 行） | 同 A1 |
| A3 | 同上 | scroll、scroll2 × 3 视口 | 手机在失败块出现后自动写一次 `5:<第5行id>:第六章 失败@0` → 200；桌面在一次 40px 滚动后写同一载荷；隐藏、离开同一载荷 → 200；目录点当前章不写 | 第 5 行 `@0` | 书架显示 `第六章 失败` |
| A4 | 首次打开 `?chapter=5` 即失败 | page、scroll × 3 视口 | 自动、隐藏、离开都不写 | 无进度行 | 无 |
| A5 | 失败中从 Reader 书架换书 | page × 3 视口 | `5:<第4行id>:第五章 前邻@0` → **400**，控制台 `AxiosError: Request failed with status code 400`；路由仍是 `?chapter=5`，**没有换书** | 第 4 行 | — |
| A6 | 同上 | scroll × 3 视口 | `5:<第5行id>:第六章 失败@0` → 200，换到目标书 `?resume=1` | 第 5 行 `@0` | — |
| A7 | 从书 A 第 4 行在 Reader 内换到目录失败的书 B | page × 3 视口 | 先为 A 写 `4:<A第4行>:第五章 前邻@561/296` → 200（正确）；目录失败页出现后为 B 写 `4:<A第4行id>:第五章 前邻@0`（1440）或 `@1477`（手机）→ **400**；隐藏再 **400** | B 无进度行 | **B 的本地快照与书架卡片变成 A 的 `4:第五章 前邻`**：那次普通保存后是 pending，隐藏的 keepalive 之后标记被清（第 11 节第 1 条），内容不变 |
| A8 | A7 之后 B 的目录恢复，刷新页面 | page × 3 视口 | 刷新时的 pagehide keepalive 发出同一载荷（页面卸载，响应未捕获） | B 无进度行 | **360×800：B 打开在第 2 行 `第三章 平稳`、scrollTop 2085**（推断：A 的序号 4 被截断到 B 的末章，再按 A 的偏移恢复）；1440×900、390×844 打开在第 0 行。三组最后本地快照都被清空（推断：取决于 Reader 读快照与书架协调哪个先完成） |
| A9 | 加载中：第 5 行正文挂起 12s，显示 `正在加载章节...` 时隐藏 | page、scroll × 3 视口 | `5:<第4行id>:第五章 前邻@0`（1440）或 `@1477`（手机）→ **400**；正文返回后 `saveAfterLoad` 写 `5:<第5行id>:第六章 失败@offset` → 200 | 加载中保持上一位置，返回后为第 5 行 | 加载中为错配载荷 |

- 失败提示每次一条（`获取章节内容失败 书源没有返回可用正文`；目录为 `获取书籍目录列表 服务器处理失败，请稍后重试`），与错误面合同
  R4 一致。
- A1–A3 的 `settleProgress` 书架刷新（共 15 组）全部成功：离开 Reader 的 keepalive 在它之前已经清掉了本地 pending 标记（第 11 节
  第 1 条），没有残留的错配 pending 快照去重试。
- A8 的位置恢复与快照清理顺序只在一次运行里观察到，是竞态，不保证复现；实施阶段的 E4 应断言“无论先后都不使用别的书的位置”。
- 服务日志 4119 行：2941 个 200、62 个 201（TXT 导入），59 个 400，全部是 `PUT /api/progress`；没有 5xx（502/500 都是浏览器侧
  注入），没有 panic。

### 4.3 与旧观察的对应

- 错误面合同第 4.3 节的 `PUT 5:第一章 起点`、运行标题合同第 10 节第 3 条的推断，就是 A1/A2 的载荷形状；那两次实测用的是模拟接口，
  `PUT /progress` 一律回 200，所以看不到服务端的 400。
- 错误面合同第 14 节末尾的 `5:第六章 失败` 就是 A3。

## 5. 差异分类

| # | 场景 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| F1 | 载荷身份：分页失败（O1）与任一模式加载中（O8） | 进度只有目录 index，标题由服务端按目录行取，不会错配 | `目标序号 + 上一章 id/标题`（offset 甚至按上一章正文算），服务端 400；本地快照和书架投影在离开 Reader 前显示“目标序号 + 上一章标题”（A1、A2、A9） | **`must-fix`（P0）**，与 D1 无关 |
| F2 | 失败章是否成为已存进度 | 是：W1 在请求时写目标章，W4 离开时再写（U1） | 分页因 F1 被拒，服务端停在上一次成功位置（A1、A2）；连续模式写失败章 `@0`（A3） | D1 已决定为 A（按上游）：分页现状 **`must-fix`**，连续模式 `aligned` |
| F3 | 首次加载即失败 | 目标章（W1） | 不写（A4） | **`must-fix`**（D1-A）：在隐藏、离开、卸载等触发点写目标章 |
| F4 | 失败章的章内位置 | 不更新 `bookChapterProgress@`，保留上一章的偏移，再打开时套在失败章上 | 写 offset 0、本章比例 0 | `acceptable-change`（运行时适配：OpenReader 按章在服务端存位置，失败章没有可定位的正文；不复现把上一章偏移套到另一章的上游缺陷） |
| F5 | 写入时机 | 请求时（W1）与离开时（W4） | 失败本身不写，只在隐藏、离开、滚动同步等触发点写；手机连续模式的自动写入取决于错误块出现后的滚动事件（A3） | **`must-fix`**（D1-A）：路由导航失败后立即写一次，与成功导航的 `saveAfterLoad` 对称 |
| F6 | 连续模式目标章失败后滚进后一章 | 当前章与进度冻结在失败章，直到下一次成功加载（U2） | 切换到后一章并写后一章（O4） | 已签收（错误面合同第 13 节第 7 条，已发布），**不重开**：D2 由用户确认为 (a) |
| F7 | 连续模式相邻章失败，滚进失败块 | 写失败章 + 块内段落偏移（U3） | 写失败章目录行 + 块内偏移（实际为 0） | `aligned` |
| F8 | 首次挂载目录失败 | 不改进度（U4、U5） | 不写（O6） | `aligned` |
| F9 | Reader 内换书后目录失败 | 新书进度不变（U6） | 为新书写上一本书的章节身份、序号与偏移（400）；新书本地快照与书架卡片变成上一本书的章节；目录恢复后刷新可能按它打开到错误位置（A7、A8） | **`must-fix`（P0）** |
| F10 | 失败中从书架换书 | `changeBook` 不等任何保存 | `select` 等待普通保存，400 时抛出，换书没有发生（A5） | **`must-fix`（P0）**：保存是尽力而为，任何保存失败都不得阻止换书 |
| F11 | 目录面板点当前失败章 | `getContent(index)`：一次重试，并写目标章 | 不重新加载；分页写 F1 载荷（400 + 未处理拒绝），连续模式不写（A1–A3） | **`must-fix`**（重试语义；与错误面合同 R5 的重试同类，在本切片的实施中一并修正） |
| F12 | 非 `await` 的保存调用 | `saveBookProgress` 一律 `.catch(() => {})` | `jumpWithinCurrentChapter`、回到顶部/跳到底部、flip 翻页等的 `saveProgress()` 不处理拒绝，服务端拒绝时成为 pageerror（A1、A2） | **`must-fix`**：进度保存失败不得产生未处理的 Promise 拒绝 |
| F13 | 离开 Reader 时写进度 | W4 每次离开写一次 | 路由离开、pagehide、隐藏、卸载都只排一次 keepalive | `aligned`（阅读进度 P2 已签收） |

没有一项判为 `intentional-redesign`。

## 6. 已签收、不重开

- 服务端进度校验：`chapterId` 与 `(book_id, index)` 不一致时 400，标题取服务端目录（阅读进度 P2）。本文要求的是前端不再发出
  错配载荷，不放宽校验。
- keepalive 只排队一次、排队失败才回退普通保存；冲突重试最多一次；认证 operation scope（阅读进度 P2）。
- 错误页 R1–R8 的结构、文字、提示、重试按钮与 R7 的穿透隔离；R6 第 13 节第 7 条“窗口把另一章变成当前章时失败状态结束”
  （D2 已由用户确认不重开）。
- 运行标题与标签页标题的派生（同一目录行）；`readerCatalogRow` 的“目录行属于当前书”规则。
- 普通保存的 `mode` 字段写界面模式（桌面 flip → `page`）；按章、按字符 offset 的精确位置（阅读进度 P2 的允许增强）。
- 后端 API、状态码、错误 envelope、数据库、浏览器存储 key 与持久文件；镜像与生产。本切片都不改。

## 7. 目标合同

P1. 载荷身份：进度载荷的 `chapterId`、`chapterIndex`、`chapterTitle` 必须来自同一目录行，且该行属于当前书（`readerCatalogRow`
  规则）。`currentIndex` 与最近一次成功加载的 `chapter` 不是同一章时（加载中、失败、Reader 内换书后目录未到），不得用 `chapter`
  的 id/标题配 `currentIndex`；offset 与比例只能来自同一章的正文，没有该章正文时为 0。

P2. 失败章（D1 已决定为 A，按上游）：当前章失败（主错误页，或连续模式当前块是错误块）时，载荷为 `currentIndex` 的目录行
  + offset 0 + 本章比例 0（整书比例 `currentIndex / 总章数`）。一次路由导航（`saveAfterLoad: true`）失败后立即保存一次；首次加载
  失败不立即保存，之后的隐藏、离开、卸载等触发写同一载荷。离开后再打开停在失败章，书架显示失败章。（未选的 B“保留上一次成功
  位置、失败期间不写”不实现。）

P3. 加载中：正文返回前的任何保存都写目标章目录行 + offset 0 + 本章比例 0，不得写出 P1 禁止的组合。

P4. 目录未知：当前书的目录行未知（目录失败，或 Reader 内换书后目录尚未到达）时不保存。任何保存都不得把另一本书的章节身份、
  序号或偏移写进当前书的服务端进度、本地 scoped 快照或书架投影。

P5. 换书：书架抽屉换书前的保存是尽力而为。保存失败（任何状态码或网络错误）不得阻止 `router.push`，也不得产生未处理的 Promise 拒绝。

P6. 目录点当前失败章：当前章处于失败状态时，目录面板点这一章按上游重新加载它（与“重新载入章节”相同，绕过前端缓存），不调用
  `jumpWithinCurrentChapter`，不按失败态写进度；成功后按成功路径处理。非失败状态下点当前章的现有行为不变。

P7. 无错误写入：page、flip（含桌面按 page 布局）、scroll、scroll2，1440×900、390×844、360×800 上，失败态与加载中的进度写入都不得
  收到 400，页面上不得出现未处理的 Promise 拒绝。不 `await` 的保存调用统一吞掉错误（上游 `saveBookProgress` 的
  `.catch(() => {})`）；本地 pending 快照的现有语义不变。

P8. 不变：第 6 节各项；不新增 API、数据库字段、浏览器存储 key 或持久文件；后端不改。

## 8. 待用户决定

- **D1（F2、F3、F5、P2、P3）（已决定，2026-11-14）**：当前章加载失败时，已存进度要不要改到失败章？
  - (A) **按上游（建议）**：改到失败章（目录行、章首）。书架显示失败章，离开后再打开停在失败章，可以直接重试；与运行标题、标签页
    标题一样以“目标章”为准。连续模式现状（A3）已经如此。
  - (B) 保留上一次成功读到的位置：失败与加载中都不写进度，再打开回到上一章原位置。与上游不同，需作为用户要求的差异记录；连续
    模式现状（A3）也要改成不写。
  - 无论选哪一个，F1、F9、F10、F11、F12 都要修。
  - **用户选 (A)**：按上游改到失败章。F2、F3、F5 因此判 `must-fix`，P2、P3 只保留 A，第 9 节的测试按 A 写。
- **D2（F6）（已决定，2026-11-14）**：连续模式目标章失败后滚进后一章，要不要重开已发布的 R6 第 13 节第 7 条，改成按上游
  在目标章仍失败时冻结当前章和进度？
  - (a) **不重开（建议）**：保持现状，当前章、运行标题、目录高亮和进度跟随实际正在读的章节。上游的冻结来自单个 `this.error` 标志
    同时挡住了位置保存与跨章切换：可见正文已经是后一章，标题、目录高亮和进度却停在失败章。
  - (b) 重开：目标章仍失败时滚动不切换当前章、不写进度，直到成功加载；需要同时改错误面合同 R6、运行标题合同与连续合同。
  - **用户选 (a)**：不重开，保持现状。F6 维持已签收；滚进后一章之后写后一章的进度，不在失败章冻结。

## 9. 测试计划（实施阶段执行，本切片不写测试）

### 9.1 单元（`node --test`）

1. `frontend/tests/readerProgressPersistence.test.mjs`（或新增 `readerFailureProgressPayload.test.mjs`）：载荷纯函数（例如
   `readerProgressPayload` 增加 `book`、`chapters` 输入，或新函数 `readerProgressIdentity`）：
   - `chapter` 为第 4 行对象、`currentIndex` 为 5、没有快照：得到第 5 行的 id 与标题、offset 0、本章比例 0。现状返回第 4 行 id，**红**。
   - 目录行属于另一本书（行的 `bookId` 不等于 `book.id`）：null（P4）。**红**。
   - 快照存在、`chapter` 与 `currentIndex` 同章：结果与现状相同（对照，绿）。
2. `frontend/tests/readerLocalProgress.test.mjs`：`currentPayload()` 在上述两种错配输入下符合 P1/P4；载荷为 null 时 `apply()` 不调用
   `reader.applyProgress`、不改书架投影。**红**。
3. `frontend/tests/readerChapterLoader.test.mjs`：`saveAfterLoad: true` 的加载失败后调用一次 `saveProgress({ force: true })`，载荷为
   第 5 行目录行、offset 0（**红**）；`saveAfterLoad: false` 的失败不立即保存；`AbortError` 与被取代的加载不保存（对照，绿）。
4. `frontend/tests/readerShelf.test.mjs`：`saveProgress` 拒绝（400 与网络错误两种）时 `select(item)` 仍调用一次 `router.push`，目标为
   所选书，且没有未处理拒绝。**红**。
5. `frontend/tests/readerToc.test.mjs`（或 `readerNavigation.test.mjs`）：当前章处于失败状态时 `jump(currentIndex)` 走重新加载
   （`loadChapter(currentIndex, 0, { refresh: true })` 或 `reloadChapter`），不调用 `jumpWithinCurrentChapter` 与 `saveProgress`
   （**红**）；非失败状态下行为不变（对照，绿）。
6. `frontend/tests/readerNavigation.test.mjs`：`saveProgress` 返回被拒的 Promise 时，`jumpWithinCurrentChapter`、`scrollToTop`、
   `scrollToBottom` 与 flip 的 `nextPage`/`previousPage` 不产生 `unhandledRejection`（在测试里监听 `process`）。**红**。
7. `frontend/tests/readerBookLoad.test.mjs`：书籍加载在目录请求处失败时，Reader 不能再为新书构造出带上一本书章节的载荷（例如在
   等待目录前清空 `chapter` 与 `chapters`，或由 P4 的目录行校验拦住）。**红**。
8. 红测写法同错误面合同：新模块在测试体内 `await import(...)`，把导入失败转成断言消息；预期第 1–7 项在当前实现上按上面标注失败。

### 9.2 真实浏览器

新增 `scripts/smoke/reader-failure-progress-contract.mjs`：沿用 `reader-progress-multiclient-contract.mjs` 的真实服务启动（构建
backend、全新数据目录、真实注册与 TXT 导入），`page.route` 只用于第 4.1 节的三种注入；`PUT /api/progress` 必须由真实服务端回答，
否则看不到 400（第 4.3 节）。视口 1440×900（鼠标）、390×844 与 360×800（`isMobile + hasTouch`），模式 page、flip、scroll、scroll2。

- E1（P1、P2、P7）：第 4.1 节的步骤。断言：每一步都没有状态为 400 的 `PUT /api/progress`，没有 pageerror；每个
  `chapterIndex: 5` 的载荷都带第 5 行 id；失败后（离开之前）服务端已是 `5:<第5行>:第六章 失败@0`，离开后仍是。预期红点：page、flip
  三视口的 400 与 pageerror（A1、A2），离开后的服务端仍是第 4 行。
- E2（P2）：首次打开 `?chapter=5` 即失败，离开后服务端为第 5 行 `@0`。预期红：现状不写（A4）。
- E3（P3）：第 5 行正文挂起 12s，显示 `正在加载章节...` 时隐藏：写第 5 行目录行 `@0` 且为 2xx。预期红：A9 的 400。
- E4（P4）：从书 A 第 4 行在 Reader 内换到目录 500 的书 B，再隐藏：没有任何 bookId 为 B 的写入；B 的本地快照与书架卡片不出现
  `第五章 前邻`；目录恢复后刷新，B 停在自己的位置（第 0 行，scrollTop 0）。预期红：A7 的 400 与快照污染，A8 在 360×800 打开到第 2 行。
- E5（P5）：page 模式失败时从 Reader 书架换书，路由变为 `/books/<目标>/read`；再让 `PUT /api/progress` 返回 500（`page.route`）重复
  一次，同样换书。预期红：A5 停在 `?chapter=5`。
- E6（P6）：四种模式失败时目录面板点当前章，第 5 行正文恰好多一次请求；源恢复后再点一次显示第六章正文。预期红：现状 0 次请求。
- E7（回归对照）：连续模式相邻章失败后滚进失败块，写 `1:<第1行>:第二章 后邻@0` 且为 2xx（F7）；正常翻章、离开的写入不变。

### 9.3 回归与临时变异（实施阶段）

- frontend `npm test`（写明新增条数）与 `npm run build`；后端不改，Go `test`/`vet`/`race` 跳过并写明原因。
- 真实浏览器：9.2 全部用例，另跑 `reader-progress-multiclient-contract`、`reader-error-surface-contract`（48 例）、
  `reader-continuous-contract`、`reader-running-title-load-contract`、`reader-text-modes-contract`、`reader-mobile-contract`、
  `bookshelf-refresh-progress-contract`、`openreader-smoke`。
- 临时变异（只改工作区，验证后立即还原，不提交）：M1 载荷退回 `chapter` + `currentIndex` → E1、E3 红；M2 `select` 改回等待保存
  并抛错 → E5 红；M3 目录点当前失败章改回 `jumpWithinCurrentChapter` → E6 红；M4 去掉 P4 的目录行校验 → E4 红；M5 去掉不 `await`
  调用的错误吞并 → E1 的 pageerror 断言红；M6 去掉“导航失败后立即保存一次” → E1 的离开前服务端断言红。

## 10. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交（本切片）。
2. **用户决定**：已齐（2026-11-14）。D1 选 A，D2 选 (a)。
3. **红测**：9.1 第 1–7 项、9.2 E1–E6，记录每个视口、模式的精确失败点，提交 `test: expose reader failure progress payload gaps`。
4. **实现**（按风险拆提交，每个都让对应断言转绿）：
   - `fix: keep reader progress payloads on one catalog row`（P1、P3、P4、P7，P0）；
   - `fix: switch books from the reader shelf when the progress save fails`（P5）；
   - `fix: reload a failed current chapter from the reader catalog`（P6）；
   - `fix: save the failed target chapter as reader-dev does`（P2，D1-A）。
5. **回归**：按 9.3 执行，包括临时变异。
6. **记录**：提交号、各视口与模式的结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
7. **发布**：实现提交改 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；由协调会话统一决定。

## 11. 观察（不在本切片）

1. **keepalive 保存不留 pending 快照**：`useReaderLocalProgress.apply` 写入 `pendingSync: true` 后立刻 `upsert`；当前书走
   `bookshelf.upsertBook` → `reader.applyServerProgress`，时间戳相同，于是 `replaceProgress` 清掉了 `pendingSync`（`stores/reader.js:658–667`、
   `stores/bookshelf.js:387–402`）。普通保存随后在 `reader.saveProgress` 里重新标 pending，keepalive 路径没有这一步。实测 A1：隐藏
   （keepalive）后本地快照不是 pending，目录点击（普通保存）后是 pending，离开（keepalive）后又不是，回首页即被服务端记录替换。
   阅读进度 P2 合同要求“无论网络结果如何，本地 scoped pending 快照必须保留”，离线时离开 Reader 会因此丢掉最后位置。需要另行盘点。
2. 进度 `mode` 字段：普通保存写界面模式（`Reader.vue:1813`），keepalive 写有效模式（`:1815`，EPUB、音频、漫画、朗读栏、自动阅读时为
   `page`）。失败页上 `chapterFormat` 停在上一章，所以失败页的 keepalive `mode` 由上一章的格式决定。上游没有这个字段，需另行确认。
3. 读代码：如果某次普通保存留下的错配 pending 快照没有被随后的 keepalive 清掉，书架“刷新”与工作台“刷新缓存”
   （`loadBooks({ settleProgress: true })` → `syncLocalProgress(..., { throwOnError: true })`）会因 400 整体失败，提示的是英文
   `invalid progress payload`。第 4 节没有观察到（第 1 条先清掉了标记）；P1 实现后这条路径不再出现，第 1 条修复时需要一并核对。
4. 模拟接口的浏览器合同（`reader-error-surface-contract.mjs`、`reader-continuous-contract.mjs`、
   `reader-desktop-panel-controls-contract.mjs`）把 `PUT /progress` 一律回 200，看不到服务端校验结果；涉及进度载荷的断言应放在真实
   服务的合同里（9.2）。
5. 错误面合同第 2.6 节与第 14 节末尾“上游在失败状态下不保存进度”的说法按本文第 2.4 节第 5 条纠正；运行标题合同第 10 节第 3 条
   的推断已由 A1、A2 证实：错配载荷确实发出，且被服务端以 400 拒绝。两处都补了指向本文的记录。

## 12. 决定记录

- 2026-11-14（盘点提交 `feeb256` 之后）：用户对 D1 选 (A)，按上游把进度改到失败章。由此 F2、F3、F5 由“待用户决定 / 随 D1”改判
  `must-fix`，P2、P3 只保留 A，第 9 节去掉 B 的分支（单元第 1、3 项，浏览器 E1–E3，变异 M6）。F1、F4、F6–F13 与其余目标不变。
- 2026-11-14（D1 记录提交 `227df0a` 之后）：用户对 D2 选 (a)，不重开 R6 第 13 节第 7 条，保持现状。F6 维持“已签收、不重开”，
  目标与测试计划不变。决定已齐，下一步是第 10 节第 3 步的红测；实施由协调会话排期。
