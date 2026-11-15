# Reader 离开时 keepalive 保存失败后本地待同步进度丢失盘点（P0）

状态：**contract-only**。本切片只写文档；Reader 产品代码、后端、单元合同和浏览器合同都没有改。第 4 节的复现用的是会话
scratchpad 里的临时探针，没有入库；脚本、`report.json`、`summary*.txt`、截图和服务日志统计放在共享笔记目录
`side-session-notes/local_5898921b-baed-4ec8-956f-1e5aa5daf56f/pending-sync-keepalive/`。红测与实现按第 9 节在后续阶段进行。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。约定路径
`/private/tmp/changshengyu-reader-dev` 在本会话沙箱里不存在且不可写，上游从 GitHub 克隆到会话 scratchpad 后 checkout 同一提交。
当前基线：`OpenReader@ebb2640`（本分支 `claude/competent-hypatia-1e8938` 从 `5bc38a3` 快进到正文失败进度载荷盘点
`claude/clever-easley-4e4500`）；`git diff 186c762 ebb2640 -- frontend backend` 为空，产品代码与已发布的 `186c762` 相同。
本文所有 OpenReader 行号以 `ebb2640` 为准。

## 1. 范围

- 对象：Reader 写入的本地进度快照（Pinia `reader.progressByBook` 与 scoped localStorage
  `openreader_chapter_progress@<用户>@<书>`）上的 `pendingSync`、`baseUpdatedAt` 从写入到被服务端确认之间的生命周期；
  离开 Reader 时 keepalive 保存失败（离线、5xx）后它是否保留；之后首页书架、Reader 打开时的协调是保留并重试，还是用服务端
  旧行覆盖；以及由它派生的可见状态：首页 `已读：` 与未读数、最近阅读、续读位置。同一个字段去除顺带造成的 `baseUpdatedAt`
  退化（第 3.5 节）也在本切片内，因为根因和修复点相同。
- 触发：全部 `background`（keepalive）保存：桌面“返回首页”、移动端“首页”、Escape、任何路由离开（后退）、页面隐藏、
  `pagehide`（关闭标签页）、Reader 卸载。普通保存（节流保存、`saveAfterLoad`、强制保存）作为对照。
- 不含：正文加载失败时载荷的章节身份与位置（[正文失败进度合同](reader-progress-load-failure-fixed-baseline-second-audit-p0-contract.md)）；
  离线切章时保存失败把已显示正文换成错误块（该合同第 10 节第 5 条，本次顺带复现，见第 10 节第 1 条）；CAS 冲突协议、
  WebSocket 收敛、请求边界本身；协调时客户端时间与服务端时间的比较（第 10 节第 2 条）。
- 起因：[正文失败进度合同](reader-progress-load-failure-fixed-baseline-second-audit-p0-contract.md)第 10 节第 1 条。当时只在
  1440×900 的写入时间线里看到 `pendingSync` 写入后 1ms 被去掉，没有离线复现，暂记 `unknown`。

## 2. 上游证据（fa22f271）

### 2.1 保存只发一次，失败静默，没有重试

| 位置 | 行为 |
| --- | --- |
| `web/src/views/Reader.vue#saveBookProgress` L1325–1336 | `POST /saveBookProgress {url, index}`，`silent`，`.catch(() => { })` 吞掉错误；不记录失败、不重试、没有“待同步”状态。 |
| `deactivated` L372–373 | 离开 Reader（keep-alive 停用：回首页、进入其他路由）调用一次 `saveBookProgress()`。 |
| `getContent` L1011–1016、L1054–1057 | 请求正文前 `setReadingBook({ ...readingBook, index })`；响应 100ms 内到达时补存一次。服务端取正文时本身也保存进度（`BookController.kt#getBookContent` L492–495，见正文失败进度合同 2.2）。 |
| `saveReadingPosition` L2488–2549 | 连续滚动跨章时 `setReadingBook` 并 `saveBookProgress(chapterIndex)`（L2521–2531）；章内段落位置只写浏览器缓存 `bookChapterProgress@书名_作者`（L2539–2545），不上传；`unload` 只调它（L292）。 |
| `autoShowPosition` L2550–2569 | 打开时从同一个本地缓存恢复段落位置；服务端的 `durChapterPos` 不参与。 |

### 2.2 本地状态与书架重载

| 位置 | 行为 |
| --- | --- |
| `web/src/plugins/vuex.js#setReadingBook` L145–173 | 100ms 后把内存书架里同一本书的 `durChapterIndex/Title/Time` 改成 `readingBook`；把去掉目录的 `readingBook`（含 `index`）写入本地 `<用户>@readingRecent`（L169–173）。 |
| `vuex.js` 初始化 L647–657 | 启动时从 `readingRecent` 恢复 `readingBook`。 |
| `web/src/views/Index.vue` L128–137、L2843–2852 | 首页“最近阅读”标签取 `readingBook`，点击 `toDetail(readingRecent)`。 |
| `Index.vue#toDetail` L1477–1500 | `index: book.index ?? book.durChapterIndex ?? 0`（L1488）：最近阅读带本地 `index`；书架卡片没有 `index`，用服务端 `durChapterIndex`。 |
| `web/src/App.vue#loadBookShelf` L627–644 | `networkFirstRequest`（`plugins/helper.js` L76–102）取 `getBookshelf`，成功后 `setShelfBooks`（`vuex.js` L72–100）用响应整批重建书架，包括 `durChapter*`；离线时用上次成功响应的缓存。 |

### 2.3 上游可见行为（离线读、离开 Reader、恢复网络、重载书架）

1. 离线期间的保存全部失败并被丢弃；服务端停在离线前最后一次成功保存的章节。没有重试。
2. 重载书架后卡片显示服务端旧章节的 `已读：` 和未读数，点卡片打开旧章节。
3. “最近阅读”来自本地 `readingRecent`，仍指向离线时读到的章节，点它打开该章；在线取正文时服务端随即把这一章写成进度。
4. 章内段落位置只在本地缓存，服务端从不覆盖，打开时恢复（按书不按章，正文失败进度合同第 5 节第 11 项已判不复现这个怪癖）。

所以上游会丢失“服务端进度”，但不会丢失“本设备读到的位置”：章内位置和最近阅读都留在本地，不被书架重载改写。

## 3. 当前 OpenReader（ebb2640）

### 3.1 已签收的 OpenReader 合同

上游没有待同步与重试。OpenReader 用 `pendingSync` + `baseUpdatedAt` 的本地快照做离线保护，这是已签收的运行时适配：

- [阅读进度 P2 合同](reading-progress-p2-contract.md)“前端状态合同”（2026-07-18，`9f19d21`）：“`background` 保存只排队一次
  keepalive；排队失败才调用普通保存。无论网络结果如何，本地 scoped pending 快照必须保留，供下次打开与服务器协调。”以及
  “只有本地 pending 快照确实更新且内容不同，才用新基线重试一次。”
- [请求边界合同](reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md)第 5 节：正常保存、离开 keepalive、
  pending snapshot、最多一次冲突重试不变；“服务端 400/413 继续由现有离线 pending/retry 流程处理，不清除本地用户位置。”
- [书架刷新进度 P1 合同](bookshelf-refresh-progress-p1-contract.md)（2026-07-22，`ed4ee27`、`a54bd72`）：网络书架快照是权威，
  非 pending 的本地进度一律被服务端行替换；只有比服务端新的 `pendingSync` 才保留并重试；“不把手动刷新解释为覆盖尚未同步的
  当前设备阅读动作”。它的实机复审还要求 keepalive 成功时经 `onSaved` 清除 pending。

这三份合同都假定 Reader 写出的快照在服务端确认之前一直带着 `pendingSync`。下面说明这个前提不成立。

### 3.2 `pendingSync` 在写入后 0–1ms 被去掉

1. `useReaderProgressPersistence.js#save`（`:44–70`）：先取基线（`:53`），再 `applyLocal`（`:55`），然后 `background` 走
   `sendKeepAlive`（`:61–65`），否则 `flush`（`:69`）。
2. `useReaderLocalProgress.js#apply`（`:55–72`）：`reader.applyProgress({ ...payload, updatedAt: now(), pendingSync: true })`
   （`:65–70`；`stores/reader.js:620–628` 写内存，`:868–893` 写 localStorage，`pendingSync`/`baseUpdatedAt` 只在为真时落盘
   `:882–885`），接着 `upsert(reader.progressByBook[bookId])`（`:71`）。
3. `upsert`（`:36–53`）：当前书走 `mergeShelfBook`（`stores/bookshelf.js:739–748`，较新的进度就是这份 pending 快照），再
   `bookshelf.upsertBook(nextBook)`（`:49`）。
4. `bookshelf.upsertBook`（`stores/bookshelf.js:387–402`）把 `nextBook.progress` 当成服务端进度：
   `reader.applyServerProgress(nextBook.progress)`（`:392–395`）。
5. `reader.applyServerProgress`（`stores/reader.js:629–639`）：本地取内存与 localStorage 中较新者，就是同一份快照；时间戳相等，
   `:633` 的 `>` 不成立，于是 `replaceProgress(progress)`（`:637`）→ `clearLocalProgressFlags`（`:812–816`）去掉
   `pendingSync` 和 `baseUpdatedAt`，覆盖内存和 localStorage（`:686–692`）。

真实浏览器里 69 次 `applyLocal` 的 pending 写入全部紧跟着这一步，间隔 0ms 33 次、1ms 36 次（第 4.3 节）。

### 3.3 普通保存会补写 pending，keepalive 不会

- 普通保存：`flush` → `saveRemote` → `reader.saveProgress`（`stores/reader.js:699–730`）在请求前再写一次带 `pendingSync` 的
  乐观快照（`:703–710`）。请求失败时异常从 `useReaderProgressPersistence.js:143–145` 抛出，乐观快照留下。真实浏览器 42 次乐观
  写入全部带 `pendingSync`。（读代码：乐观快照与 `applyLocal` 的 `updatedAt` 落在同一毫秒时，`newestProgress` 在时间相等时
  保留已有的那份，`utils/bookOrder.js:10–20`，乐观快照会被丢弃；本次 0/42，未观察到。）
- keepalive：`sendKeepAlive`（`:72–114`）只发 `fetch`；非 2xx 直接返回（`:96`），网络错误被 `.catch(() => {})` 吞掉（`:103`），
  没有任何补写。`:111` 的注释“The optimistic local snapshot remains pending for the next sync attempt”描述的正是合同要求，
  但在它之前快照已经被去掉了。
- 更糟的是 keepalive 会清掉先前失败的普通保存留下的 pending：离线读书时节流保存失败，快照是 pending；离开时 keepalive 路径的
  `applyLocal` 再写一次并立即被去掉，keepalive 又失败，最后留下非 pending 的快照（第 4.2 节 S1、S4 的“离线读”到“离开”）。
- 一次离开会跑多次后台保存（首页按钮 `useReaderPanels.js:28–31`、`onBeforeRouteLeave` `Reader.vue:2132–2134`、卸载
  `useReaderPageLifecycle.js:63–70`；隐藏 `Reader.vue:2259–2261` 与 `pagehide` `:2255–2257`），每次的 `applyLocal` 都重复这一步。

### 3.4 谁用服务端旧行覆盖它

- 网络书架加载：`bookshelf.loadBooks` 的网络分支（`stores/bookshelf.js:150–159`）→ `reconcileServerProgressFromBooks`
  （`:616–629`）→ `reader.reconcileShelfProgress`（`stores/reader.js:640–680`）→ `reconcileAuthoritativeShelfProgress`
  （`utils/bookOrder.js:22–30`）：只有 `pendingSync` 且更新的本地快照被保留并重试，其余 `replaceProgress(server)`。触发点：
  回到首页后立即的书架请求（每次离开后都观察到一个 `GET /api/books`）；`online` 事件 → `setOnline` → `refreshShelfInForeground`
  （`layouts/AppLayout.vue:818–821`、`:802–805`，`utils/shelfSyncFreshness.js:9–44`，`loadBooks({ force: true, all: true })`）；
  页面可见与 `focus`（`AppLayout.vue:807–812`、`:915–916`）；WebSocket 连上后（`composables/useSync.js:219`、`:237`）；
  应用启动（`AppLayout.vue:923–925`）；首页与 Reader 内书架的“刷新”。
- Reader 打开：`useReaderBookLoad.js:110–113` 调 `loadProgress(bookId, { preferLocal: true })`，只有本地 pending 时才优先本地
  （`stores/reader.js:735–742`），否则服务端行 `replaceProgress`（`:757`）。
- 可见状态：首页 `已读：`、未读数和续读路由（`views/Home.vue:243–259`、`:270–272`），最近阅读（`useAppRecentReading.js:7–22`、
  `:44–51`）都取 `newestBookProgress(book, progressByBook)`，协调之后就是服务端旧行。

### 3.5 同一次去除让 CAS 基线变成客户端时间

- `readerProgressBaseUpdatedAt`（`utils/readerProgressPersistence.js:51–55`）：pending 时取 `baseUpdatedAt`，否则取 `updatedAt`，
  前提是“非 pending 的快照就是服务端确认过的行”。快照被去掉标记后，它的 `updatedAt` 是客户端 `now()`
  （`useReaderLocalProgress.js:68`），下一次 `save()` 的基线（`useReaderProgressPersistence.js:53`）和下一次 `applyLocal` 写进
  pending 的基线（`useReaderLocalProgress.js:59–60`）就都是客户端时间。
- 连续阅读模式（`scroll`、`scroll2`，`Reader.vue:955–957`）每次滚动同步都先 `applyLocalProgress(snapshot)`
  （`useReaderScrollSync.js:44–51`，`Reader.vue:1846`）再排期保存，所以这两种模式下每一次普通保存的 `baseUpdatedAt` 都是客户端
  时间。在线实测（第 4.5 节）：`page` 9/9 是服务端版本，`scroll` 9/9、`scroll2` 9/9 是客户端时间，移动端 `flip` 第一次保存是
  客户端时间；离线隐藏后关闭页面的第二个 keepalive 也带客户端时间（三视口）。
- 服务端陈旧判断 `IsStaleUpdate`（`backend/services/readingprogress/service.go:192–201`）是 `serverUpdatedAt.After(base)`。基线变成
  客户端时间后，这一判断比较的是服务端时间与客户端时间：另一台设备在两者之间的写入不再报冲突；客户端时钟慢于服务端时，本设备
  没有并发写入也可能被判为冲突。可见后果需要并发写入或时钟偏差，本次没有复现。

### 3.6 为什么已有测试没发现

- `frontend/tests/readerLocalProgress.test.mjs:18–21` 的 `upsertBook` 只记录调用；`:53–62` 断言的是组合函数自己的
  `book.value.progress.pendingSync`，不看 store 和 localStorage。
- `frontend/tests/readerProgressPersistence.test.mjs:166–294` 的 keepalive 测试都用 `applyLocal: () => {}`；`:254` 的断言消息
  “an accepted keepalive must clear the optimistic pending state”说明设计意图是“被接受后才清除”。
- `frontend/tests/bookshelfRefreshProgress.test.mjs:70–108` 和 `scripts/smoke/bookshelf-refresh-progress-contract.mjs:239–248`、
  `:297–306` 的 pending 都是直接写进 localStorage 的 2099 年快照，不是 Reader 产生的。
- 没有任何浏览器合同把网络断开或让 keepalive 失败；`reader-progress-multiclient-contract.mjs:475–543` 的 CAS 断言通过 API
  显式传基线，不经过 Reader 的基线推导。

### 3.7 历史

- 2026-06-01 `c784f8d` 引入本地 pending 快照；`e8f7fbc` 引入 `replaceProgress` 去标记。
- 2026-06-06 `4677cb7` 本地写入同步书架条目；`a4ffed3` 让 `upsertBook` 调 `applyServerProgress`。从这时起写入后立即被去掉。
- 2026-07-02 `2ed2945` 抽出 `useReaderLocalProgress`，行为不变。
- 2026-07-18 `9f19d21` P2 合同写明 pending 保留；相关单元测试都是模拟的。
- 2026-07-22 `ed4ee27` 书架快照改为权威，非 pending 一律被替换，丢失从此在每次网络书架加载时都可见；`a54bd72` 让 keepalive
  成功走 `onSaved`。

## 4. 真实浏览器复现（2026-10-18）

### 4.1 环境

- 产品代码：`ebb2640` 的 frontend 生产构建（`npm ci`、`npm run build`）和用 Go 1.26.9 编译的 backend 二进制，独立的临时
  data/cache/library 目录、随机 JWT secret，监听 `127.0.0.1`。会话沙箱禁止 loopback 监听，服务和 Playwright 在沙箱外运行。
  Playwright 1.61.1（根目录 devDependency），chromium-headless-shell 1228。
- 全部走真实实现：注册测试账号、导入 TXT、进度 `PUT`、SQLite、WebSocket、书架投影。浏览器侧只做两件事：
  `context.setOffline(true/false)`；5xx 场景在离开期间把 `PUT /api/progress` 用 503 应答（`GET` 不拦截）。
- 夹具：6 章各 70 段，`0 第一章 开端`、`1 第二章 已存`（服务端进度预置在这一章）、`2 第三章 离线`（离线时进入，在预载半径内）、
  `3 第四章 预载`、`4 第五章 后续`、`5 第六章 终点`；page 模式；每个场景一个新浏览器上下文、一本新导入的书。每个视口一个账号。
- 共同开头：首页点书进入（`?resume=1&chapter=1`），按两次 PageDown，等 1.2s 节流保存成功（200），作为基线（数据库行）。
- 记录：所有对 `openreader_chapter_progress@…` 的 localStorage 写入（时间、字段、调用栈）；每个 `PUT /api/progress`（`fetch` 即
  keepalive，`xhr` 即 axios；请求体基线；状态或失败原因）、`GET /api/books`、`GET /api/progress/:id`；只读 SQLite 行；首页行文字；
  重新打开的路由。视口 1440×900、390×844、360×800（后两者 `isMobile`、`hasTouch`）。

### 4.2 场景与结果

表中位置写作“章序号/offset”，`P` 表示带 `pendingSync`。各列依次是：基线数据库行 → 离线阅读后本地 → 离开 Reader 后本地 →
恢复网络或重载后本地与数据库 → 重新打开的路由。

| 场景 | 1440×900 | 390×844 | 360×800 |
| --- | --- | --- | --- |
| **S1 离线读后点首页**：断网，PageDown 三次（每次一个普通保存，全部 `ERR_INTERNET_DISCONNECTED`），点首页（keepalive 失败），恢复网络，重载，重开 | 1/1594 → 1/3918P → 1/3918 → 1/1594（库 1/1594）→ `offset=1594` | 1/1009 → 1/2480P → 1/2480 → 1/1009 → `offset=1009` | 1/953 → 1/2340P → 1/2340 → 1/953 → `offset=953` |
| **S2 离线切到下一章后点首页**：断网，`router.replace({ query: { chapter: 2 } })`（正文来自内存缓存，`saveAfterLoad` 失败），点首页，恢复网络，重载，重开 | 1/1669 → 2/54P → 2/0；首页 `已读：第三章 离线`/未读 3 → 恢复网络后 1/1669，首页 `已读：第二章 已存`/未读 4 → `chapter=1&offset=1669` | 1/1056 → 2/38P → 2/0；同上 → 1/1056 → `chapter=1&offset=1056` | 1/998 → 2/38P → 2/0；同上 → 1/998 → `chapter=1&offset=998` |
| **S3 只有 keepalive 返回 503**：在线，PageDown 两次后 0.6s 内点首页，新位置只在 keepalive 里；之后撤掉拦截，重载，重开 | 1/1619 → keepalive 1/3192 → 503 → 回首页后 30ms 内的 `GET /api/books` 200 把本地改回 1/1619 → `offset=1619` | 1/1025 → keepalive 1/2020 → 503 → 1/1025 → `offset=1025` | 1/968 → keepalive 1/1907 → 503 → 1/968 → `offset=968` |
| **S4 离线读后页面隐藏再关闭**：断网，PageDown 两次（普通保存失败，P），`visibilitychange` 隐藏（keepalive 失败），关闭页面（`pagehide` 再发 keepalive），恢复网络后新开首页，重开 | 1/1768 → 1/3487P → 1/3487 → 1/1768 → `offset=1768` | 1/1119 → 1/2207P → 1/2207 → 1/1119 → `offset=1119` | 1/800 → 1/1576P → 1/1576 → 1/800 → `offset=800` |
| **S5 对照组**：同 S1 到离开后；探针把本会话最后一次 pending 写入的 `pendingSync` 和 `baseUpdatedAt` 写回 localStorage（`updatedAt` 加 1ms，压过内存里去掉标记的同一份），再恢复网络 | 1/1669 → 1/4102P → 1/4102 → 恢复网络后书架协调重试 `PUT`（xhr）1/4102 → 200，库 1/4102 → 重载、重开 `offset=4102` | 1/1056 → 1/2596P → 1/2596 → 重试 200，库 1/2596 → `offset=2596` | 1/754 → 1/1853P → 1/1853 → 重试 200，库 1/1853 → `offset=1853` |

- S1–S4 共 12 次，离开后的本地快照全部没有 `pendingSync`；恢复网络（或 S3 回到首页、S4 新开页面）后第一次网络书架加载就把
  本地改成服务端旧行；数据库从未收到离线或失败期间的位置；重载和重开都回到旧位置。离线期间首页显示的是本地位置（S2 的
  `已读：第三章 离线`/未读 3），恢复网络后不经任何操作退回 `已读：第二章 已存`/未读 4（截图
  `390x844-offline-chapter-home-after-leave.png` 与 `-home-online.png`）。
- S5 三次都由已有的书架协调发出一次重试并成功，数据库、首页和重开位置都是离线时读到的位置。已有的协调、重试和
  `syncLocalProgress` 都能工作，缺的只是 `pendingSync` 这个标记。写回的基线取自离开时第二次 `applyLocal` 的写入，已经是客户端
  时间（第 3.5 节），服务端因为数据库版本不晚于它而接受；换成服务端版本作基线同样不会被判陈旧（`IsStaleUpdate` 要求严格晚于）。
- 1440×900 的单独试跑（第一个注册账号是管理员，配置了 `data/webdav/bookProgress`）中，WebDAV 镜像始终停在基线位置 1594，
  与数据库一致；正式矩阵的账号都不是管理员，镜像按设计跳过。

### 4.3 写入时间线（摘自 390×844 S4，时间为距场景开始的毫秒数）

```text
+7205  i1 off1663 pending=true  base=02:42:20.276（服务端）  ← applyLocal → applyProgress
+7205  i1 off1663 pending=false                              ← upsertBook → applyServerProgress → replaceProgress
+7207  i1 off1663 pending=true  base=02:42:20.276            ← saveProgress → applyProgress（乐观写入）
+7212  PUT[xhr] i1 off1663 → ERR_INTERNET_DISCONNECTED       （pending 保留）
+9739  i1 off2207 pending=true  base=02:42:20.276            ← applyLocal（visibilitychange 隐藏）
+9740  i1 off2207 pending=false                              ← upsertBook → applyServerProgress → replaceProgress
+9744  PUT[keepalive] i1 off2207 base=02:42:20.276 → ERR_INTERNET_DISCONNECTED   （没有补写）
+11266 PUT[keepalive] i1 off2207 base=02:42:26.031（客户端时间）                  （pagehide）
+12918 i1 off1119 pending=false                              ← 新页面 GET /api/books → reconcileShelfProgress → replaceProgress
```

全部 15 次运行：69 次 `applyLocal` 的 pending 写入都在 0–1ms 内被去掉；42 次普通保存的乐观写入都带 `pendingSync`；keepalive 路径
没有一次补写。

### 4.4 服务日志与其他记录

- 本次服务（试跑 1 次、正式矩阵 15 次、基线检查 12 次）GIN 状态：1823 个 200、28 个 201、13 个 304；`PUT /api/progress` 82 个
  全是 200（离线和被拦截的请求没有到达服务端）。没有 5xx，没有 panic。
- 浏览器错误只有离线或 503 造成的资源加载失败；没有 `pageerror`，失败的普通保存没有变成未处理拒绝。
- 证据文件里没有 token、cookie 或 JWT secret；服务日志只统计状态码和去掉查询串的路径。

### 4.5 在线基线检查（`scroll`/`scroll2` 的客户端时间基线）

同一服务，在线，不拦截。每个视口每种模式新导入一本书，首页进入后 PageDown 三次，每次等 2.6s：

| 视口 | page | scroll | scroll2 | flip |
| --- | --- | --- | --- | --- |
| 1440×900 | 3/3 服务端 | 3/3 客户端 | 3/3 客户端 | 4/4 服务端 |
| 390×844 | 3/3 服务端 | 3/3 客户端 | 3/3 客户端 | 第 1 次客户端，第 2 次服务端 |
| 360×800 | 3/3 服务端 | 3/3 客户端 | 3/3 客户端 | 第 1 次客户端，第 2 次服务端 |

“服务端”是带 6 位小数、等于此前某个服务端响应 `updatedAt` 的值；“客户端”是 3 位毫秒、等于本地快照 `updatedAt` 的值。全部
200、无冲突（单客户端，没有并发写入）。

## 5. 差异分类

| # | 场景 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | keepalive 保存（离开、隐藏、关闭、卸载）失败后的本地状态 | 没有 pending；服务端进度丢失，但章内位置和最近阅读留在本地 | 写入后 0–1ms 去掉 `pendingSync`/`baseUpdatedAt`，失败后是普通本地快照（S1–S4，三视口） | **`must-fix`**：违反 P2 前端状态合同“无论网络结果如何，本地 scoped pending 快照必须保留”和请求边界合同第 5 节 |
| 2 | 先前普通保存失败留下的 pending | 不适用 | 被离开时 keepalive 路径的本地写入清掉（S1、S4） | **`must-fix`**，随第 1 项 |
| 3 | 恢复网络、回到首页、重载、重开之后 | 书架卡片退回服务端旧章节；最近阅读和章内位置仍是本设备读到的 | 第一次网络书架加载或 Reader 打开就用服务端旧行覆盖；`已读：`、未读数、最近阅读、续读位置全部退回；离线读到的章和位置都丢了（S1–S4） | **`must-fix`**：比上游丢得更多，也违反书架刷新 P1“不把手动刷新解释为覆盖尚未同步的当前设备阅读动作” |
| 4 | 5xx 时 | 同第 1 项 | 回到首页后 30ms 内的书架请求就覆盖（S3，三视口） | **`must-fix`**，随第 1 项 |
| 5 | 失败保存的重试 | 不重试 | 不重试：标记没了，协调和 `loadProgress` 都不会重试 | **`must-fix`**：重试是已签收的 OpenReader 适配；S5 证明协调、重试本身可用，只差标记 |
| 6 | 去掉标记后的 `baseUpdatedAt` | 无基线概念 | `scroll`/`scroll2` 每次普通保存、隐藏后的第二个 keepalive 都以客户端时间作基线（4.3、4.5） | **`must-fix`**（同一根因）：已签收的 CAS 以服务端 `updatedAt` 为版本，基线不能是客户端时间。可见后果（漏报冲突、时钟偏差下误报冲突）需要并发写入或时钟偏差，未复现 |
| 7 | 乐观写入与 `applyLocal` 同一毫秒 | 不适用 | 时间相等时保留去掉标记的那份，乐观 pending 被丢弃（读代码，0/42） | **`must-fix`**（读代码，随第 1 项）：pending 是否保留不能取决于两个时间戳是否落在同一毫秒 |
| 8 | 普通保存失败 | 静默 | 乐观快照保留 pending，失败不成为未处理拒绝（42/42） | `aligned`，保留为回归断言 |
| 9 | 有 pending 时的协调 | 不适用 | 书架协调保留更新的 pending 并重试一次，成功后清除（S5，三视口） | `aligned`，不改 |
| 10 | keepalive 成功 | 不适用 | `onSaved` 经 generation 门清除 pending（`a54bd72`） | `aligned`，不改 |
| 11 | 失败提示 | 静默 | 静默（离线时首页只有书架请求失败的既有提示） | `aligned`：不新增 UI |
| 12 | 最近阅读的来源 | 本地 `readingRecent`，与服务端无关 | 取进度协调结果；pending 保留后显示本设备的新位置，服务端也会被重试收敛 | `acceptable-change`（已签收的权威进度与 pending 适配），修好第 1 项后不需要另做 |
| 13 | 章内位置 | 只存本地，按书不按章，不上传 | 精确 `offset` 上传并以服务端为准 | `acceptable-change`（已签收的精确位置增强），不重开 |

没有 `intentional-redesign` 项，本切片也没有 `unknown` 项；第 10 节的观察各自另行盘点。

## 6. 已签收、不重开

- 后端进度合同：章节身份规范化、CAS、`200` + `X-OpenReader-Progress-Conflict`、提交后才镜像和广播（P2，`9f19d21`）；16 KiB 单
  UTF-8 JSON、显式 `chapterIndex`、时间戳与 `mode`/`clientId` 短界（请求边界合同）。后端不改，`IsStaleUpdate` 不改。
- keepalive 单发、排队失败才回退普通保存、keepalive 成功经 `onSaved` 清除 pending、认证 operation scope、冲突最多重试一次、
  WebSocket 收敛与不回声、`clientId`。
- 书架刷新 P1 的权威协调规则：非 pending 一律被服务端替换，比服务端新的 pending 保留并重试，显式刷新等待最终赢家，离线 fallback
  不当作权威。本切片只要求 Reader 真正产生这些规则依赖的 pending 标记，规则本身不改。
- 多用户隔离、`openreader_chapter_progress@<用户>@<书>` 作用域与旧 key 迁移、存储格式（`pendingSync`/`baseUpdatedAt` 只在为真时
  落盘）。
- 精确 `offset`/`percent`/`chapterPercent`；不复现上游按书缓存段落位置的怪癖。
- 保存触发点、1200ms 节流、`saveAfterLoad`、`markProgressSaved`。
- [正文失败进度合同](reader-progress-load-failure-fixed-baseline-second-audit-p0-contract.md) R1–R8 与它第 10 节的其余观察；该合同第 10 节
  第 1 条（暂记 `unknown`）由本文裁决，那份文件本身不改，避免与其他链合并时冲突。
- 协调与冲突重试用客户端 `updatedAt` 和服务端 `updatedAt` 比较新旧（P1、P2 的既有设计，见第 10 节第 2 条）。
- API、数据库、浏览器存储 key、持久文件、镜像与生产：本切片都不改，不在本地构建或推送镜像，不做任何生产操作。

## 7. 目标合同

R1. **确认前一直待同步。** Reader 写出的本地进度快照（`applyLocal` 和普通保存的乐观写入）在内存 `progressByBook` 和 scoped
localStorage 里都保持 `pendingSync: true` 和它所基于的服务端版本 `baseUpdatedAt`，直到出现下列之一：该快照（或更新的本地快照）
被服务端以非冲突响应确认（普通保存成功、keepalive 2xx 经 `onSaved`、`syncLocalProgress` 成功）；冲突按既有规则裁决为服务端
赢家；既有的作用域切换、删书清除。把本地快照转投到书架（`upsertBook`）或任何与它时间戳相同的行回流，都不算确认。

R2. **keepalive 失败不改变它。** 网络错误、非 2xx、`fetch` 同步抛错、页面在响应前销毁时，本地快照保持 R1 的状态和位置。同一次
离开里的多次后台保存（首页按钮、路由离开、卸载；隐藏、`pagehide`）互不清除。

R3. **失败不累积成丢失。** 先前普通保存失败留下的 pending 不会被之后的任何本地写入清除；更新的本地写入只能用新位置替换旧的
pending，并沿用同一个服务端基线（最后一次被确认的服务端 `updatedAt`）。

R4. **基线只来自服务端。** 每个 `PUT /api/progress` 的 `baseUpdatedAt` 要么是该客户端此前从服务端收到的某个 `updatedAt`，要么在
没有服务端进度时为空；不得是客户端生成的时间。适用于所有模式（含 `scroll`/`scroll2` 每次滚动同步）和每个 keepalive。

R5. **恢复后由既有协调收敛。** 网络书架快照或 Reader 打开时，本地 pending 比服务端行新则保留本地位置并按既有规则重试一次，
成功后清除；在裁决之前，首页 `已读：`、未读数、最近阅读和续读路由显示本地 pending 的位置。这是既有合同，本切片要求它对 Reader
真实产生的 pending 生效（S5 已证明机制可用）。

R6. **时序无关。** 离开 Reader 后首页立即发起的书架请求、`online` 事件、页面可见、WebSocket 重连、应用启动和手动刷新，哪个先到
都不能把刚失败的保存当作非 pending 覆盖。pending 是否保留也不能取决于两个时间戳是否落在同一毫秒。

R7. **其余不变。** keepalive 仍只排队一次，排队失败才回退普通保存；keepalive 成功仍经 generation 门清除 pending；普通保存失败不成为
未处理拒绝；冲突最多重试一次；不新增 UI 提示；后端不改。

不接受的做法：去掉权威协调或让任何非 pending 本地进度压过服务端（会重新引入 P1 问题）；靠调整时间戳（如加 1ms）制造“更新”；为
保住标记而不再更新书架条目（首页和 Reader 内书架必须立即显示本地位置）；每次 keepalive 后再发普通请求；把非 2xx 当成功；
只改模拟测试而不覆盖真实 store。

## 8. 测试计划（后续阶段执行，本切片不写测试）

### 8.1 单元合同（`node --test`）

1. 真实 Pinia `reader` 与 `bookshelf` store，`useReaderLocalProgress` 接真实 `upsertBook`：`apply()` 之后内存、scoped localStorage
   都是 `pendingSync: true`，`baseUpdatedAt` 等于预置的服务端 `updatedAt`。`ebb2640` 上失败。
2. `useReaderProgressPersistence` 接真实 store：`save({ force: true, background: true })` 时 `fetch` 拒绝、返回
   `{ ok: false, status: 503 }`、同步抛错，pending 都保留；返回 `{ ok: true }` 与服务端行时清除（既有断言）。首页按钮、路由离开、
   卸载三次连续后台保存不互相清除。
3. 普通保存失败后接一次后台保存：pending 和基线都保留（R3）。
4. 基线：连续模式的 `applyLocalProgress(snapshot)` 之后，下一次保存的 `baseUpdatedAt` 等于最后一次服务端 `updatedAt`，不等于快照的
   客户端 `updatedAt`；第一个 keepalive 失败后的第二个 keepalive 同样（R4）。
5. 同一毫秒：固定 `now()` 与 `saveProgress` 的 `Date`，乐观写入仍带 `pendingSync`（R6）。
6. 协调：由 Reader 路径产生（不是手写）的 pending 快照遇到较旧的网络书架快照时 `retryPending`，`syncLocalProgress` 的请求带服务端
   基线；非 pending 仍被替换（既有断言保留）。
7. 改写：`readerLocalProgress.test.mjs:18–21` 的假 `upsertBook` 改为真实 store（或复现 `applyServerProgress` 的假实现）；
   `readerProgressPersistence.test.mjs` 的 keepalive 测试保留 `applyLocal: () => {}` 的版本，另加接真实 store 的版本。

### 8.2 Go

后端不改，不新增 Go 测试；实现阶段照常跑 `go test ./...`、`go vet ./...`。

### 8.3 真实浏览器

新增 `scripts/smoke/reader-progress-offline-pending-contract.mjs`，真实 Go、SQLite、WebSocket，1440×900、390×844、360×800：

1. S1–S4：离开后本地 `pendingSync: true` 且基线是服务端版本；离线首页显示本地位置（S2 为 `已读：第三章 离线`/未读 3）；恢复网络
   后由书架协调发出重试 `PUT /api/progress`（xhr，对照组每个视口 1 次），带服务端基线并返回 200；SQLite 行、重载后的首页、重开
   路由都是离线时读到的位置；没有 `pageerror`。
2. S3：回到首页时的书架请求不覆盖；撤掉 503 后的下一次书架加载（`online`、可见、手动刷新任一）重试成功。
3. `page`、`scroll`、`scroll2`、`flip` 在线各保存三次：每个请求的 `baseUpdatedAt` 都等于此前某个服务端响应的 `updatedAt`（R4）。
4. 在线离开：keepalive 成功后 pending 被清除，且没有重复的普通请求（既有 P2 断言）。
5. 既有浏览器合同不变：`bookshelf-refresh-progress-contract.mjs`、`reader-progress-multiclient-contract.mjs`（一胜一冲突）、
   `reader-text-modes-contract.mjs`、`reader-mobile-contract.mjs`。

### 8.4 回归与临时变异（实现阶段）

- M1：恢复“书架转投即按服务端进度去标记”的路径，8.1 第 1–3 项和 8.3 第 1 项必须失败。
- M2：keepalive 失败时清除 pending，8.1 第 2 项和 8.3 第 1–2 项必须失败。
- M3：基线改回取快照 `updatedAt`，8.1 第 4 项和 8.3 第 3 项必须失败。
- 全量：frontend test/build、Go full/vet、advisory 门、上述浏览器合同三视口。

## 9. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交（本切片）。
2. **排期**：改动集中在 `useReaderLocalProgress.js`、`stores/bookshelf.js#upsertBook`、`stores/reader.js`（`applyServerProgress`、
   `replaceProgress`、`applyProgress`）和 `utils/readerProgressPersistence.js#readerProgressBaseUpdatedAt`，不需要等运行标题或正文失败
   进度的实现。正文失败进度的实现也会改 `useReaderLocalProgress.js` 与 `readerLocalProgress.test.mjs`，两者都基于 `ebb2640`，后落地的
   一方需要 rebase。
3. **红测**：8.1 第 1–6 项、8.3 第 1–3 项，记录每个视口与模式的精确失败点。提交
   `test: expose reader pending progress cleared before the keepalive save`。
4. **实现**：最小改动让同一批断言转绿。提交 `fix: keep reader progress pending until the server confirms it`。
5. **回归**：按 8.4 执行，包括临时变异。
6. **记录**：提交号、各视口与模式的结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
7. **发布**：实现提交改动 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；是否合入由用户决定。

## 10. 观察（不在本切片）

1. 离线切章变成错误块（[正文失败进度合同](reader-progress-load-failure-fixed-baseline-second-audit-p0-contract.md)第 10 节第 5 条，
   本次复现）：S2 中目标章正文来自内存缓存并已提交（请求体的 `chapterId` 是目标行 15/45/75），随后 `saveAfterLoad` 的 `PUT` 离线失败，
   加载器的 catch（`useReaderChapterLoader.js:118–119`、`:128–134`）把已显示的正文换成“章节加载失败，请检查书源或网络后重试”
   （三视口，截图 `*-offline-chapter-offline-chapter.png`）。上游前端的进度保存失败静默，不影响已取得的正文。修好本切片后 pending
   会保留，但错误块仍在，需单独盘点，暂记 `unknown`（复现结果与上游不同，倾向 `must-fix`）。
2. 跨时钟比较：协调（`utils/bookOrder.js:22–30`）和冲突重试（`stores/reader.js:833–838`）都用客户端 `updatedAt` 比较服务端
   `updatedAt`。客户端时钟慢于服务端时，即使本切片修好，一个有效的 pending 也可能被判为“不比服务端新”而被替换。书架刷新 P1 的
   反向时钟夹具只覆盖非 pending 的未来时间。没有复现，需另行盘点，暂记 `unknown`。

## 2026-11-15 主线复核

在本机上游仓库 `/Users/yuchangsheng/Documents/reader-dev` 核对固定提交 `fa22f271` 的 `web/src/views/Reader.vue`。OpenReader 起点为 `49392de5`。

上游保存进度失败时保留本地阅读位置。当前修复让书架更新和重开时的本地快照保留待同步标记。首次没有服务端进度时，读取成功触发补写；读取失败仍保留待同步位置。适配当前接口后 18 项单测中 12 项先红后绿，另有缓存重开和服务端无行的专项红测。三视口真实 Go 服务与 SQLite 浏览器合同通过 24/24；补充复审的无服务端行分支由单测覆盖。
