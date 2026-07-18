# P2 认证运行时 scope 与同步连接代际合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 完成上游/当前实现取证；本文件是实施前合同，不代表修复完成。按照
`readerdev-compat-inventory` 门禁，本阶段只更新审计文档，不修改应用代码。

本切片处理“旧账号异步结果/旧 WebSocket 回调在新账号运行时提交”的统一问题。它覆盖用户资料、
Reader 设置、书架/搜索偏好、阅读进度、分类列表以及 WebSocket 连接代际。书架列表已经完成的
request revision、网络优先和多客户端收敛合同继续保留，不重复重写。

## 固定上游权威行为

| 场景 | 上游证据 | 必须保留的产品语义 |
|---|---|---|
| 登录后初始化 | `web/src/App.vue#login` 先保存 token，再等待 `getUserInfo()`，随后执行 `syncFromLocalStorage()` 和 `init(true)`。 | 当前会话身份确定后才把该用户/终端的数据提交到可见 store；旧初始化不能在新会话后补写。 |
| 用户命名空间 | `web/src/plugins/vuex.js#getCurrentUserName` 从当前用户/manager namespace 派生；书架、最近阅读等 key 带当前用户名。 | 用户切换后，书架与阅读现场不得读取或写入上一用户命名空间。 |
| 配置恢复 | `Index.vue#restoreUserConfig` 经确认取回当前认证用户快照、写四个 localStorage key，再同步 Vuex。 | 配置应用是当前用户的一次原子可见转换；一个旧请求不能把另一用户配置混进当前终端状态。 |
| 配置保存 | `Index.vue#saveUserConfig` 读取当前终端四个配置值，写当前认证用户空间。 | 保存响应只属于发起它的身份；迟到响应不能改变后来登录用户的同步基线或错误状态。 |

固定上游没有 OpenReader 的 JWT Pinia 持久化、WebSocket 和多标签同步，因此严格的 token/scope/
connection-generation 校验是必要的技术栈适配。它不能被解释为允许刷新页面、关闭其它客户端或
等待旧连接自然结束后才正确。

## 当前实现审查矩阵

| 链路 | 当前证据 | 裁决 |
|---|---|---|
| 书架列表 | `bookshelf.loadBooks` 捕获 `shelfScope` 和 revision；网络/缓存返回前后都验证，用户切换会 reset revision。 | **aligned，必须保留**。本切片不得削弱网络优先或 mutation revision。 |
| 分类列表 | `loadCategories` 只在开始时 `ensureShelfScope()`；旧缓存读取和 `/api/categories` 返回后直接写 `categories`，写缓存时又临时调用 `currentUserScope()`。用户切换后，A 的结果可进入 B 的内存和 B 的缓存 key。 | **must-fix**：捕获 scope/revision/cache key；缓存和网络结果均只允许提交到原 scope。 |
| Reader 设置读取 | `loadReaderSettings` 只在请求前校验 `settingsScope`。响应后 `applyReaderSettings` 会先切到“现在的” scope，再把旧 payload 应用进去；空记录分支还可能调用 `saveReaderSettings`。 | **must-fix**：A 的迟到读取不得重置/覆盖 B，也不得触发 B 的默认设置写入。 |
| Reader 设置保存 | `saveReaderSettings` 的 payload 和 base 来自请求开始时状态，但成功、冲突和失败分支无 operation/scope 所有权。A 的冲突响应可把 A 的设置应用到 B；A 的完成还会改 B 的 base、updatedAt、syncing/error。 | **must-fix**：服务器可完成已授权的 A 请求，但其响应只结算 A operation；B 的状态完全不变。 |
| shelf/search 偏好 | `preferences.loadPreference/savePreference` 与 Reader 设置存在同类迟到提交；`loadPreferences` 并发两项，可能把不同会话的 shelf/search 混成一个持久 Pinia 快照。 | **must-fix**：每 key 独立 operation revision，同时受 immutable scope 约束；旧请求不能清除新请求的错误/忙碌状态。 |
| 用户资料 | `userStore.loadMe` 直接将响应写到 `profile`，没有 token/scope/generation 校验。A 的 `/me` 可在 B 登录后把导航用户名、权限和用户 ID 改回 A。 | **must-fix**：仅当前 token 对应的最新 `/me` 可提交；401 的 rejected-token 保护继续保留。 |
| 阅读进度 | `saveProgress/loadProgress/syncLocalProgress` 只在请求前 `ensureProgressScope`；响应通过 `replaceProgress/applyServerProgress` 重新读取当前 scope。旧响应可写入新 scope 的 `progressByBook` 和 `openreader_chapter_progress@<new-user>`。 | **must-fix**：捕获 scope/operation/book，迟到结果不得写新 scope、触发冲突重试或生成新用户 localStorage key。 |
| WebSocket 实例 | `useSync` 使用模块级 `socket`；旧实例的 `close` 无条件把全局 `socket` 清空并安排重连，`error` 调用 `socket?.close()` 可能关闭后来创建的新实例，`message` 也不校验实例/token/scope。 | **must-fix**：每个 handler 绑定 candidate + token + scope + generation；被替代实例的任何回调均为 no-op（仅允许关闭自身资源）。 |
| 重连/定时任务 | `disconnect` 会清理当前 timer，但旧 socket 的异步 `close` 仍可在新登录后重新安排 timer；已有刷新 pending 只在显式 disconnect 时清理。 | **must-fix**：disconnect/supersede 增加 generation；timer 执行前再校验 generation、token、scope，且一个浏览器运行时至多一个 current socket/重连 timer。 |
| 后端认证隔离 | `/api/me`、`settings`、`categories`、`progress` 与 `/ws/sync` 都按请求/握手 token 取得 user ID；Hub 只向该 user ID 广播。 | **aligned**：问题在前端迟到提交，不改 REST/WS 路径、JWT、响应和数据库 owner 条件。 |

## 目标状态机

### 通用异步 operation

1. 请求开始时捕获不可变 `{scope, token fingerprint, operation revision}`；需要持久缓存时同时捕获
   完整 cache key，不在回调中重新派生当前用户 key。
2. 每次 `await` 或 Promise continuation 后，在读取/修改 Pinia、localStorage、busy/error/base 时间、
   发起冲突重试或默认写入前检查 operation 是否仍是该 scope/key 的 current owner。
3. scope 或 revision 已变化时返回明确的 discarded/no-op 结果；不得把旧错误显示给新用户，也不得
   清除新 operation 的 loading/syncing 状态。
4. logout/require-login/reset 使该 scope 的所有前端 operation 失效。已经到达服务器的旧授权写可
   正常完成其原用户事务，但响应不能污染当前会话。
5. 同一 scope 的新请求仍遵循现有 last-request/current-operation 规则；本合同不把结果永久 union，
   不改变冲突算法和服务器时间权威。

### WebSocket generation

1. `connect()` 捕获 candidate socket、完整 token、scope 和递增 generation。只有同时满足
   `candidate === currentSocket`、generation 当前且 token/scope 未变的 handler 才能修改状态。
2. 旧 candidate 的 `error` 只关闭旧 candidate；不能通过模块级引用关闭新 socket。
3. 旧 candidate 的 `close` 不得清空新引用、修改新连接的 connected 状态或安排重连。
4. `disconnect()` 先使 generation 失效、清引用并清所有 pending/timer，再关闭旧 candidate。
5. 重连 timer 捕获 expected generation/token/scope；执行时任一不匹配即退出。当前 socket 存在时
   不创建第二条连接。
6. 旧 candidate 的 `message` 全部丢弃。当前 candidate 的事件继续走既有同用户 store/窗口事件，
   Hub backpressure 断开后仍通过当前 generation 的重连 + full shelf refresh 收敛。

## API 与数据兼容边界

- 不改 `/api/me`、`GET/PUT /api/settings/:key`、`GET /api/categories`、`GET/PUT /api/progress*`、
  `/ws/sync` 的路径、请求体、响应、状态码、JWT、事件 payload 或错误文案。
- 不新增 SQLite 表/列/index，不改 `user_settings`、`reading_progress`、`categories`、`books` 数据，
  不改 backup/WebDAV、`data/`、`cache/`、`library/`。
- 保留现有 Pinia key 和用户 scoped 章节进度/书架缓存 key；不删除旧数据。修复只阻止把 A 的结果
  写进 B 的现有 key。
- `pageMode` 仍是终端本地设置；Reader 用户配置仍剔除它。既有 settings/progress conflict header
  与本机新进度优先重试语义继续保留，但只在原 scope operation 内执行。
- 不把完整 JWT 写入 Pinia、日志、测试快照或新持久位置；scope 可继续用 JWT user ID 派生值。

## 测试先行闸门

1. 通用 operation 单元：A 请求挂起 → reset/login B → A 成功/失败，断言 B 的 value/base/error/busy
   和 localStorage 不变；B 的更新请求可正常完成。
2. Reader/偏好契约：迟到 load、空值触发默认 save、save success、conflict、failure、并发 shelf/search
   均覆盖 scope 与 operation revision；同 scope 最新请求仍提交。
3. 进度契约：A 的前台 GET、prefer-local 后台 GET、PUT success/conflict/retry 在切到 B 后均丢弃，
   不创建 `openreader_chapter_progress@user:B@<A-book>`；B 的同书操作不被 A finally 清理。
4. 分类契约：旧缓存读取和旧网络响应均不能提交或写 B cache key；B 请求正常写自己的 key。
5. WebSocket fake-runtime：构造 socket A，disconnect/connect B，再依次触发 A error/close/message；B 保持
   current/connected，只存在一个重连链。B close 才按当前 token 安排一次重连。
6. 用户资料：A `/me` 延迟到 B 登录后返回，导航 profile/权限保持 B；A 的 401 不能清除 B token，
   沿用现有 rejected-token interceptor 合同。
7. 真实浏览器：两个真实用户，延迟 A 的 settings/categories/progress/me 响应并快速切换 B；释放 A
   后在 1440×900、390×844、360×800 验证 B 用户名、主题/偏好、分类和阅读现场不变，无重复 socket、
   API 500、控制台错误或横向溢出。
8. 全量 frontend/backend/build；若形成可验证中途镜像，运行本地 Docker 历史卷/备份门禁后发布。

## 本切片之外

- 书架直接增删、批量操作、导入和各 overlay 自身的 in-flight transaction 仍须在各模块合同内逐项
  审查；本合同不以修复共享 scope controller 为由宣称所有业务 mutation 已完成。
- 不改变用户主动同时阅读同一本书的进度冲突产品规则，不新增“自动切换账号”UI，不改变登录框。
- 不用页面刷新、强制关闭其它客户端、人工延时或忽略 console error 掩盖旧 operation/连接。
