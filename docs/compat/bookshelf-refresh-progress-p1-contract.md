# P1 书架刷新与阅读进度权威收敛合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-22 用户实机回归后重新打开。第一批已完成上游复审、测试先行、实现、全量回归、
三视口真实浏览器验证、挂载卷/备份门禁和本地双架构 Docker 发布；但该批浏览器 fixture 只覆盖
`非 pending 的未来时间旧值`，没有覆盖 Reader 返回书架时由 keepalive 留下的真实
`pendingSync`，也没有分别覆盖首页书架与 Reader 内书架两个可见刷新入口。用户再次确认：点击
“刷新”不能同步阅读进度，整页刷新后才更新。因此旧的“已完成”结论无效，本合同重新进入
`must-fix`。本轮仍不改变并发 CAS、WebDAV 镜像、书架排序或最新章节检查。

## 权威文件与状态转换

| 场景 | 固定上游行为 | 当前 OpenReader 行为 | 判定 |
|---|---|---|---|
| 刷新入口 | `web/src/views/Index.vue#refreshShelf` 调用 `loadBookshelf(null, true)`，最终进入 `web/src/App.vue#loadBookShelf(refresh)`。 | `frontend/src/views/Home.vue#refreshShelf` 以 `force:true, all:true` 调用 `bookshelf.loadBooks`，同时刷新分类和分组。 | `aligned`：按钮确实绕过内存节流并发起网络请求。 |
| 网络响应 | 上游 `GET /getBookshelf?refresh=1` 成功后直接 `commit("setShelfBooks", response.data.data)`；`setShelfBooks` 由响应整批重建 `state.shelfBooks`，包括 `durChapterIndex/Pos/Time/Title`。刷新前的内存进度不参与“谁的时间更大”判断。 | `GET /api/books` 返回数据库中的嵌入式 `progress`，但 `syncServerProgressFromBooks -> reader.applyServerProgress` 和 `sortBooks` 都会把服务器进度与 `progressByBook`/localStorage 按 `updatedAt` 取较新者。 | **must-fix**：成功的全量网络快照必须成为书架进度权威，不能被已经确认过但陈旧的客户端内存再次覆盖。 |
| 无服务器进度 | 上游整批替换后，响应没有进度就不会保留刷新前书架对象上的旧 `durChapter*`。 | 当前同步只遍历有 `book.progress` 的条目；服务器已经没有进度时，`progressByBook` 仍会被 `sortBooks` 注回书架。 | **must-fix**：全量快照内某本书没有进度时，清除该书非 pending 的内存与 scoped localStorage 进度。 |
| 本地未提交进度 | 上游没有离线 pending/CAS 模型。 | OpenReader 在发送前保存 `pendingSync + baseUpdatedAt`，用于页面退出、离线恢复和冲突协调。 | `acceptable runtime adaptation`：真正 pending 的本地用户动作不能因书架刷新丢失；保留并立即用快照基线重试同步。 |
| 推送更新 | 上游没有多客户端 WebSocket。 | `progress_update` 可能与 REST 请求乱序，因此普通推送仍需按服务器 `updatedAt` 防止旧事件覆盖新事件。 | `acceptable runtime adaptation`：只有已完成的全量网络快照使用“服务器权威”模式；普通事件保持乱序保护。 |
| API 与数据 | 上游把进度投影在书籍字段中。OpenReader 的 `GET /api/books` 已从当前用户 SQLite 读取 `ReadingProgress` 并投影到每本书。 | 请求、响应、JWT、数据库和持久目录均已满足现有合同。 | `aligned`：本批不改 API、Go、SQLite、备份或 WebDAV。 |

## 目标状态机

1. 书架按钮继续并发刷新分类、分组和 `GET /api/books`；只有书籍请求失败才报告刷新失败。
2. 网络书架响应通过既有 user scope 与 request revision 门后，对响应中的每本书执行一次
   “权威进度协调”。
3. 本地进度不是 `pendingSync`：无论其时间戳比响应早、晚或相等，都以服务器进度替换；服务器
   没有该书进度时同时清除内存与当前用户 scoped localStorage 条目。
4. 本地进度是 `pendingSync` 且比服务器进度更新（或服务器无进度）：保留本地位置，使用服务器
   `updatedAt` 作为基线尝试既有 `syncLocalProgress`；刷新不能静默丢弃未提交的真实阅读动作。
5. pending 不再占优或同步冲突返回服务器赢家时，沿用既有 CAS 逻辑清除 pending 标志并收敛。
6. 书架列表、`reader.progressByBook` 和浏览器书架缓存必须在同一提交中使用同一个协调结果；
   不能先替换列表又由 computed 的 `newestBookProgress` 把旧进度注回。
7. 网络失败的离线 fallback 不是服务器权威，继续使用现有本地合并语义；WebSocket 单条事件也
   继续按服务器时间防乱序。

## 兼容边界

- 保留 `GET /api/books` 的路径、参数、JSON、状态码、JWT 和用户隔离。
- 不修改 `ReadingProgress`、`Book`、SQLite schema、`data/`、`cache/`、`library/`、备份或 WebDAV。
- 保留书架网络优先、请求 revision、本地导入即时 upsert、前台校准和 WebSocket 重连逻辑。
- 保留服务端 `shelfOrderAt` 和上游“最后阅读时间排序”语义；本批只确保显示和排序消费同一权威进度。
- 不把手动刷新解释为覆盖尚未同步的当前设备阅读动作；只有明确的 `pendingSync` 可以暂时保留。

## 测试先行闸门

1. 纯状态测试：服务器快照必须替换时间戳更晚的非 pending 内存进度；无服务器进度必须得到空值；
   较新的 pending 本地进度必须保留并标记需要重试。
2. Pinia/静态接线：网络成功分支必须调用权威协调；fallback 和普通 `applyServerProgress` 不得错误
   获得权威清除语义；Home 刷新仍传 `force:true, all:true`。
3. 真实浏览器：同账号客户端 A 先显示旧章节，客户端 B 保存新章节；阻断 A 的 WebSocket 后点击
   A 的“刷新”，必须在不 reload、不重建路由的情况下更新“已读”章节和书架顺序。
4. 反向时钟 fixture：把 A 的非 pending localStorage `updatedAt` 设为未来时间，服务器快照仍必须
   胜出；另以 pending fixture 证明未提交位置不会被清掉。
5. 前端全量测试、生产构建和三个目标视口的书架 smoke 通过；本批无后端改动，但发布前仍运行
   `go test ./...` 和 Docker 卷/备份门禁。

## 禁止的伪修复

- 不调用 `window.location.reload()`，不通过延时或清空整个 localStorage 掩盖状态错误。
- 不移除离线 pending/CAS 保护，不让任意旧 WebSocket 消息无条件覆盖新进度。
- 不增加每本书一次的 `/api/progress/:id` N+1 请求；`GET /api/books` 已包含权威进度。
- 不把分类或书源更新当成阅读进度刷新，也不改变“刷新”按钮的其它上游职责。

## 2026-07-22 实施与验证记录

- 新增纯状态协调器：完整网络书架快照无条件替换所有非 pending 客户端进度；服务器无进度时
  返回空值；只有比快照更新的 `pendingSync` 才暂时保留并请求 CAS 重试。
- `reader.reconcileShelfProgress` 在同一 user scope 内同步更新 `progressByBook` 和 scoped
  localStorage；服务器空值会精确删除该书的内存/本地记录，不清空其它书或其它用户数据。
- `bookshelf.loadBooks` 的 network 分支先协调完整快照，再排序并写书架缓存。fallback、单条
  WebSocket 事件和普通 Reader 加载继续保留原有时间戳乱序保护。
- 失败测试先证明 2099 年非 pending 旧进度会压住 2026 年服务器进度、服务器空值无法清除旧值；
  实现后对应 Pinia 测试和 6 项纯合同全部通过。
- 前端全量 **520/520**、生产构建、后端 `go test ./...` 均通过。
- `bookshelf-refresh-progress-contract.mjs` 使用真实 Go、SQLite 和 Chromium，在 1440×900、
  390×844、360×800 禁用 WebSocket、写入 2099 年旧 scoped 进度，再由同账号远端 API 客户端
  保存第三章。三个视口只点击“刷新”均更新已读章节与 localStorage，主文档导航计数不变且无
  横向溢出、控制台错误或 API 500。
- 应用提交 `ed4ee27dd6d51e1d1f75b9970baa0c0b0e7ef56f` 已先推送 GitHub。本地 arm64 候选通过
  全新卷/备份恢复；历史卷第一次执行遇到一次资源 404，随后以逐步骤跟踪完整重跑，TXT、EPUB、
  UMD、CBZ、相对缓存、owner isolation、容器重启和 portable restore 全链通过。
- 本机随后构建并上传 linux/amd64、linux/arm64 OCI，标签 `ed4ee27` 与 `latest` 均指向 index
  `sha256:f369cc6610312987e068dcdd015887e27569a9dcbe6c048525de12bb5ab95d89`；amd64 为
  `sha256:a98655524d6f55680ecf14766c8f56b363f615b37f8865a43e897b9c3904e4ee`，arm64 为
  `sha256:a6348c2ef9ae56caf6769fb0a593f9234db13a735c89ff78665cef1a3cb72166`。远端两个标签已分别
  `imagetools inspect`，未使用云端构建。
- 允许差异仍只有 OpenReader 的 pending/CAS 离线保护；未完成项是用户在真实多设备书架复验，
  以及全量重构矩阵中与本切片无关的后续模块。

## 2026-07-22 实机回归复审

| 项目 | 固定上游 / 用户合同 | 当前实现 | 新判定 |
|---|---|---|---|
| Reader 返回书架 | 上游保存进度后，书架刷新以 `getBookshelf` 响应整批替换 `durChapter*`。 | `goShelf()` 和 Reader 卸载都可走 `background:true`；`sendKeepAlive()` 在请求发出后立即返回成功，但不读取响应、不调用 `onSaved`，因此已经被服务器接收的快照仍可在 Pinia/localStorage 留作 `pendingSync`。 | **must-fix**：keepalive 响应在 SPA 仍存活时必须完成确认；同一次离开不得制造两个互相竞争的后台保存。 |
| 显式刷新与 pending | 用户点击刷新后，可见章节、未读数、排序和时间必须在按钮完成前收敛；不得要求 reload。 | `reconcileShelfProgress()` 对较新的 pending 只异步触发 `syncLocalProgress(...).catch()`，`loadBooks()` 不等待结果便返回并显示旧 pending；旧测试只断言了纯函数。 | **must-fix**：显式刷新事务要等待 pending 的 CAS 结果并以最终赢家一次性提交书架、Reader store 与 scoped cache。 |
| 两个刷新入口 | 首页和 Reader 内书架都应复用同一刷新语义。 | 两者都调用 `loadBooks({force:true,all:true})`，但旧 smoke 只点击首页按钮。 | **must-fix together**：同一 store 事务修复，分别做真实浏览器断言。 |
| 普通后台刷新 | WebSocket 重连/元数据事件不能阻塞 UI，也不能把临时网络失败变成清空进度。 | 普通同步事件共享 `loadBooks()`，但没有“用户等待刷新”和“后台校准”的完成语义区分。 | **acceptable runtime adaptation**：后台调用可继续异步；只有显式按钮要求等待最终进度赢家。 |

### 回归测试闸门

1. 先用真实 Reader `返回书架` 产生 keepalive pending，证明响应成功时能清除 pending，并且同次离开
   只发送一个最终快照。
2. 同账号客户端 B 保存新位置，客户端 A 保留已提交但未确认/时钟偏移的 pending；首页点击刷新后，
   按钮结束前必须显示 CAS 最终赢家，不 reload、不靠 WebSocket。
3. 在 Reader 内打开书架并执行相同刷新，非当前书与当前书的已读章节、未读数、排序均使用同一
   最终结果；活动正文不能被书架列表刷新偷偷跳章。
4. 网络失败继续保留本地 pending 并明确报错；其他用户 scoped localStorage、书架缓存和进度不变。
5. 1440×900、390×844、360×800 真实浏览器覆盖两入口，前端全量、构建和 Go 全量通过。

## 2026-07-22 第二次实施与发布前验证记录

- Reader 的 keepalive 保存现在按阅读位置指纹合并同一次路由离开/卸载产生的重复后台请求；
  成功响应仍会经过 generation 门并调用既有 `onSaved`，从 Pinia 和 scoped localStorage 清除
  已被服务器确认的 `pendingSync`。页面已切换到新用户/新阅读会话时，旧响应不会提交。
- `bookshelf.loadBooks` 为可见刷新增加 `settleProgress` 事务语义。首页书架和 Reader 内书架
  都要求等待 pending CAS；冲突直接接受服务器赢家并一次性写入书架、Reader store 和浏览器
  缓存，不再先显示旧 pending、等下次整页加载才收敛。
- 后台 WebSocket/前台自动校准仍保持非阻塞；显式同步失败会拒绝刷新、保留本地 pending 并
  进入原有可见错误提示，不会把暂时断网解释为服务器删除了阅读位置。
- `bookshelf-refresh-progress-contract.mjs` 使用真实 Go、SQLite 和 Chromium，在 1440×900、
  390×844、360×800 分别验证首页和 Reader 内两个刷新入口：未来时间 pending 与远端更新
  冲突时，每个入口只发出一次 CAS，最终显示服务器赢家；主文档不 reload，Reader 正文章节
  和路由不跳转。WebSocket 在 fixture 中保持静默，因此结果不依赖推送补救。
- 针对性 28 项测试、前端全量 537/537、Vite 生产构建和 Go 全量通过。本批未改 API、Go、
  SQLite schema、持久目录、WebDAV 或备份格式。
- 实现提交 `a54bd725991bc19535fb8263ab4a17efbc7d7f87` 已推送，并从本机发布
  `ghcr.io/changshengyu/openreader:a54bd72` 与 `latest`；两标签指向 OCI index
  `sha256:5caae8c4277459431c9265e159e85702d8b1433e11d4083af8fb413d0aeedb96`。候选镜像的新卷、
  历史卷、重启、用户隔离和便携备份恢复全链通过；最终仍等待用户真实多设备书架复验。
