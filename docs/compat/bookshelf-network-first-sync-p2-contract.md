# P2 书架网络优先与多客户端收敛合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 已按测试先行合同实施并通过全量/真实浏览器验证；本批 Docker 发布门禁待完成。
本切片只处理“冷启动先显示旧书架”和“同步事件丢失后缺少权威校准”。7 月 17 日已经完成的
请求 revision、本地导入即时 `upsert`、用户 scope 隔离和 WebSocket 重连强刷继续保留，
不重新设计书架、BookInfo、导入 UI 或 SQLite 数据。

## 上游权威行为

| 场景 | 固定上游证据 | 必须保留的语义 |
|---|---|---|
| 书架初次加载 | `web/src/App.vue#loadBookShelf` 调用 `networkFirstRequest(() => GET /getBookshelf, "getBookshelf@<user>")`；`web/src/plugins/helper.js#networkFirstRequest` 先等待网络，仅在请求失败后读取浏览器缓存。 | 冷启动不得先把旧持久缓存当成成功的最新书架展示；网络成功结果是权威列表，缓存只是离线/失败回退。 |
| 显式刷新 | `App.vue#init(refresh)` 在 refresh 或空书架时重新调用 `loadBookShelf`；`Index` 的刷新动作进入同一网络加载链。 | 显式刷新必须发起网络请求，不能被内存或浏览器缓存短路。 |
| 网络失败 | `networkFirstRequest` 在请求失败时读取 `localCache@getBookshelf@<user>`；没有可用缓存才继续抛错。 | 已有用户作用域缓存可以维持离线可读，但不得把缓存命中伪装成一次新的服务器提交。 |
| 导入后状态 | 上游重新加载书架；当前 OpenReader 因导入响应已经含完整 shelf item，可先即时提交再用权威列表校准。 | 新书不能等待其它客户端关闭、路由重建或定时缓存过期才出现。 |

上游是单用户应用，没有 OpenReader 的 JWT/WebSocket 多客户端模型。因此“同用户多个客户端
最终收敛”属于必要的技术栈适配，但不能改变上述网络优先和本地动作即时可见的产品语义。

## 当前实现与裁决

| 链路 | 当前证据 | 裁决 |
|---|---|---|
| 冷启动缓存 | `frontend/src/stores/bookshelf.js#loadBooks` 在发出 `/api/books` 前读取 IndexedDB/localStorage，并立即写入 `books`、`booksLoadedAt`。网络较慢时，旧缓存会先短暂覆盖 UI；这正是“刷新后新导入书籍先不显示、过一会才出现”的可见窗口。 | **must-fix**：恢复网络优先。只有网络失败且当前作用域仍无更新状态时，才提交旧缓存作为 fallback。 |
| 请求竞态 | `createShelfRequestRevisionGate` 已保证迟到请求不能覆盖后来的 force、本地 upsert/delete 或用户切换。 | **aligned，必须保留**；网络失败后的异步缓存读取也必须经过同一 revision 二次校验。 |
| 本地导入 | `bookshelf.importTXT` 成功后立即 `upsertBook(data)`；后端 `importTXT` 在导入/分类写入结束后广播同一 shelf item。 | **aligned，必须保留**；当前客户端不等待 WebSocket 回声。 |
| 远端同步 | `useSync` 对有 payload 的 `bookshelf_update` 即时 upsert，对断线重连强制加载书架/分组。 | **技术栈等价，必须保留**。 |
| 同步可靠性 | `backend/sync/Hub.Broadcast` 的发送队列容量为 16；队列满时 `default` 分支静默丢弃事件。`AppLayout#refreshShelfInForeground` 又在 `syncConnected && books.length` 时直接返回。于是“显示已连接”被错误当成“肯定没有漏事件”。 | **must-fix**：服务端不能静默维持一个已丢状态事件的健康连接；慢客户端应断开并通过重连强刷恢复。前端进入前台/恢复网络时也必须做有节流的权威校准，不以 connected 状态跳过。 |
| 书架 API | `GET /api/books` 每次按当前用户直接查询 SQLite、投影分类/进度/缓存计数并排序；没有服务端书架结果缓存。 | **aligned**：路径和 JSON 不变；前端持久缓存不能改变该响应的权威性。 |
| 双客户端首次启动 | 真实 Go + 两个浏览器上下文同时启动时，两端都先读取空 `reader/shelf/search` 设置并保存默认值。`updateUserSetting` 的 `SELECT -> FirstOrCreate` 不是原子 upsert：两个请求都读到不存在后，第二个 INSERT 触发 `(user_id,key)` UNIQUE constraint，实测 `PUT /api/settings/reader` 返回 `500 failed to save setting`。 | **must-fix**：同一用户/键首次并发保存必须使用 SQLite 原子 conflict upsert；两个请求都返回现有 200 形状，数据库只保留一行。不能靠浏览器串行化、预建设置行或过滤 500 规避。 |

## 目标状态机

### 冷启动与离线回退

1. `loadBooks` 先取得当前 user scope、request key 和 revision，再立即请求 `/api/books`。
2. 网络成功且 revision 仍可提交：完整替换内存列表，更新时间戳，并异步写当前用户、当前
   request key 的浏览器缓存。
3. 网络失败：若内存已由本地动作、WebSocket 或更新请求改变，返回当前内存，不读旧缓存覆盖。
4. 仅当列表仍为空且原 revision 仍可提交时读取缓存；读取完成后再次校验 revision。
5. 缓存 fallback 只表示“离线可用”。恢复网络、进入前台或 WebSocket 重连必须重新请求服务端。
6. `force:true` 始终跳过内存去重并发出请求；旧请求仍由 revision 丢弃。

### 多客户端收敛

1. 正常事件：同一用户的其它客户端收到完整 `bookshelf_update` 后即时 `upsertBook`；删除即时
   `removeBookLocal`。事件 payload 与 REST 响应使用同一 shelf item 投影。
2. 后端发送队列满：不得静默丢事件后继续把连接视为健康。移除并关闭慢客户端；浏览器的既有
   reconnect 路径随后强制拉取完整书架/分组。
3. 浏览器 `focus`、`visibilitychange -> visible` 和 `online` 触发有节流的 full-shelf 校准。
   WebSocket 的 `connected` 只表示传输连接存在，不是数据新鲜证明。
4. 校准请求同一时间最多一个；30 秒窗口按“最近一次成功校准”计算。失败不占用完整成功窗口，
   `online` 或下一次前台事件可重试。
5. 本地导入/upsert 的 mutation revision 继续阻止校准前发出的旧响应删除新书。

### 设置多客户端启动边界

1. `GET /api/settings/reader|shelf|search` 在不存在设置行时仍返回现有 `200 {}`；存在时返回既有
   `{key,value,updatedAt}`，并发读取不创建默认行。
2. `PUT /api/settings/:key` 继续执行既有 key/JSON 校验、Reader 本地布局字段清理和
   `baseUpdatedAt` 陈旧写保护。首次不存在行的并发写必须在数据库唯一键上原子 upsert，不能暴露
   UNIQUE 错误；无 base 的并发初始值按数据库提交顺序 last-write-wins。
3. 成功后仍返回实际持久化的 `{key,value,updatedAt}` 并发布既有 `settings_update`；数据库中
   `(user_id,key)` 始终只有一行。另一个用户的同名 key 不参与冲突。
4. 不改 SQLite 连接数、WAL/busy timeout、数据库路径或 schema；本问题不是读写锁超时。

## API、缓存与数据兼容边界

- 保留 `GET /api/books`、所有请求参数、响应字段、JWT/用户隔离、状态码和排序语义。
- 不新增或迁移 SQLite 表/列，不改变 `data/`、`cache/`、`library/`、备份和 WebDAV 格式。
- 保留现有 `localCache@bookshelf@getBookshelf:<request-key>:<user-scope>` 条目，升级不删除用户缓存。
  它们只从“抢先首屏数据”降级为网络失败 fallback；成功网络响应继续原 key 覆盖。
- 不把 WebSocket payload、JWT 或用户名写入新的持久位置；不增加公开跨用户事件。
- 分类仍沿用现有 `cacheFirst`/同步事件语义，本切片不扩展为分类或设置缓存重写。
- 原子 upsert 只使用现有 `user_settings(user_id,key)` 唯一索引；不新增表、列或迁移。

## 测试先行闸门

1. 前端单元：网络挂起期间旧持久缓存不能提交；网络失败才 fallback；缓存读取期间发生
   `upsertBook` 时旧缓存不得覆盖；force 始终发请求。
2. 前端单元：前台校准在 WebSocket connected 且已有书时仍会运行；30 秒按成功时间节流；
   并发事件合并；失败后 online/下一次 focus 可重试。
3. Go 单元：填满 Hub 客户端发送队列后再次广播会移除/关闭慢客户端，而其它同用户客户端仍收到
   事件；不同用户不收到；`BroadcastAll` 同样不静默保留慢连接。
4. Go API/DB：同一用户并发首次 PUT 同一个 `reader/shelf/search` key；每个请求都得到既有 200
   形状，不出现 UNIQUE/500，最终只有一行且值是其中一个完整请求。不同用户同 key 保持隔离，
   已有 `baseUpdatedAt` 冲突测试继续通过。
5. 真实 Go + 两浏览器上下文：客户端 A 导入，客户端 B 在不刷新路由的情况下看到新书；断开 B
   的同步链后导入第二本，B 恢复前台/网络后通过 full refresh 收敛。桌面、390×844、360×800
   至少各覆盖 UI 几何，双客户端链至少在一个视口运行真实 WebSocket。
6. 全量 `go test ./...`、前端测试/build；若本切片发布 Docker，再跑当前卷/备份 smoke。

## 实施记录（2026-07-18）

- `bookshelf.loadBooks` 现等待 `/api/books` 权威结果；IndexedDB/localStorage 只在网络失败、作用域
  和 revision 仍有效且没有更新内存状态时作为 fallback。fallback 不写入服务器新鲜时间，后续
  `ensureBooksLoaded`、恢复网络、前台或同步重连仍会重新校准。
- 前台校准不再把 WebSocket `connected` 当成数据新鲜证明。它按最近一次成功结果节流 30 秒，
  合并并发刷新，失败不消耗节流窗口，`online` 会立即触发可见页面的下一次尝试。
- Hub 的单用户和全局广播遇到满发送队列时移除并关闭慢客户端；健康客户端仍收到事件，浏览器
  既有重连流程随后强制拉取完整书架，因而不再静默保留已经漏掉状态事件的“健康”连接。
- `PUT /api/settings/:key` 使用现有 `(user_id,key)` 唯一键执行 SQLite conflict upsert，并在返回和
  广播前重新读取实际持久化行；不改变路由、JSON、校验、陈旧写保护、schema 或连接配置。
- 新增网络挂起/失败/revision、前台节流与重试、Hub backpressure、八路首次设置并发契约测试。
  前端全量 **457/457**、生产构建和后端 `go test ./...` 均通过。
- `scripts/smoke/bookshelf-multiclient-contract.mjs` 使用真实 Go、SQLite、WebSocket 与两个同账号
  Chrome 上下文验证：1440×900、390×844、360×800 均不会在延迟网络结果前渲染旧书架；客户端
  A 导入 TXT 后，客户端 B 无需重载即可显示新书；所有 API 无 500，页面无横向溢出或控制台错误。

本记录只关闭本合同列出的 P2 书架新鲜度/收敛切片。其它 Pinia store、缓存与事务仍按全量矩阵
逐项复审；Docker 卷/备份及 GHCR 摘要将在本提交发布后回填。

## 不授权的变化

- 不删除书架持久缓存、不用空白骨架或人为延时掩盖陈旧结果。
- 不取消 7 月 17 日的 revision gate，不把列表合并改成可能保留服务器已删除书籍的永久 union。
- 不让 WebSocket 成为唯一权威，不引入跨用户广播，也不因同步修复修改书架排序、分组或阅读进度。
- 不把“两个客户端最终一致”解释为允许等待其它客户端关闭；正常事件必须即时，故障路径必须自动校准。
