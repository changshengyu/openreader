# P1 书架刷新与阅读进度权威收敛合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-22 已完成上游复审、测试先行、实现、全量回归和三视口真实浏览器验证；Docker
发布待本地镜像门禁完成。用户报告书架“刷新”不能刷新阅读进度，只有
整页刷新后才能看到其它客户端已经保存的位置。本合同只修复书架列表中的进度收敛，不改变
Reader 的进度保存、并发 CAS、WebDAV 镜像、书架排序或最新章节检查。

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
