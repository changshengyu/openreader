# P1 Index 工作台认证会话隔离合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-27 已完成固定上游与当前实现取证，并按测试先行完成候选实现。前端
611/611、生产构建与后端全量测试通过；真实浏览器与 Docker 发布闸门尚未完成。

本合同承接：

- [`authenticated-runtime-scope-p2-contract.md`](authenticated-runtime-scope-p2-contract.md)
  已完成的 Pinia 异步 operation 与 WebSocket generation 隔离；
- [`reader-reauthentication-isolation-p0-contract.md`](reader-reauthentication-isolation-p0-contract.md)
  已完成的 Reader、全局书籍弹层和退出保存隔离；
- [`index-search-p1b-contract.md`](index-search-p1b-contract.md)
  已完成的 Index 搜索、探索、续页和 BookInfo 主流程。

上述合同没有覆盖仍由 `indexWorkspace` 持有的搜索/探索场景，也没有证明旧 Index 组件的迟到
回调不能在新账号登录后修改共享 store、偏好、弹层或路由。本合同只修认证会话边界，不改变
既有搜索/探索 API、结果布局、分页语义或数据库。

## 1. 固定上游权威行为

| 场景 | 上游证据 | 产品语义 |
| --- | --- | --- |
| 登录失效 | `web/src/plugins/axios.js` 的 `NEED_LOGIN` 打开根登录框并令 `loginAuth=false`。 | 当前工作台停止使用失效身份；登录框覆盖当前场景。 |
| 同身份恢复 | `web/src/App.vue#login/#init` 保存 token、加载用户信息并重载书架、书源、分组、RSS、规则和书签；`Index.vue` 监听 `loginAuth` 后调用 `init(true)`。 | 认证恢复后重新读取当前身份的数据，不能把失效前的网络响应当成新鲜结果。 |
| 用户空间切换 | `web/src/views/Index.vue` 监听 `userNS` 并调用 `init(true)`。 | 可见工作台必须切换到当前用户空间的数据。 |
| 搜索/探索现场 | `Index.vue` 将 `isSearchResult`、`isExploreResult`、`searchResult`、分页和滚动位置保存在组件 data；`backToShelf()` 才显式清空。 | 同一 Index 内保留结果场景是上游交互；搜索、探索和书架不是互相独立的产品路由。 |
| 迟到请求 | 上游搜索新请求会关闭前一个 SSE，但普通 Promise 与 Explore 没有跨用户 generation。 | 这是固定上游在单命名空间模型中的缺口，不能复制到 JWT 多账号运行时。 |

上游的 `init(true)` 并不清空 `searchResult`。在 OpenReader 中，结果对象包含账号可见书源 ID、
来源名、远程 URL、规则变量和本地书仓路径；直接保留给另一 JWT 账号会违反多用户隔离。因此：

- 同账号重新认证可以恢复**意图**，但必须丢弃旧结果并重新请求；
- 不同账号或无法证明旧身份时必须进入干净书架；
- 这是允许且必需的多用户安全适配，不是放弃上游 Index 交互。

## 2. 当前实现审查矩阵

| 合同层 | 当前证据 | 判定 |
| --- | --- | --- |
| 工作台 store 生命周期 | `frontend/src/stores/indexWorkspace.js` 是全局 Pinia store，保存 mode、结果、搜索/探索 intent、分页、滚动位置和三个 revision；没有 scope、session generation 或 `resetSessionState()`。 | **must-fix**：注销、401 和账号切换后会继续持有上一账号现场。 |
| 会话清理 | `userStore.clearSession()` 已重置 overlay、bookshelf、preferences 和 Reader，但不触碰 `indexWorkspace`。 | **must-fix**：凭证移除前必须同步挂起/清空可见 Index 私有状态，并淘汰旧请求。 |
| 根壳阻塞 | `App.vue` 只在 Reader 路由检查 `readerSessionBlocked`；认证 Dialog 登录成功一写入 token，非 Reader 的 `AppLayout` 会在账号判定和路由清理前重新挂载。 | **must-fix**：认证恢复的路由/账号决策完成前，Reader 与 Index 都不能挂载。 |
| 当前 UI 搜索入口 | `AppLayout.beginWorkspaceSearch()` 直接写 store；在 canonical `/` 上不写 `workspace` query。 | **技术栈等价**：Index 场景可不依赖路由；但同账号恢复不能假设 URL 一定含完整 intent。 |
| 旧链接恢复 | `/search`、`/discover` 重定向为 `/?workspace=...`；`Home.applyRouteWorkspaceIntent()` 会在挂载时重新执行 query。 | **acceptable-change + must-fix**：兼容链接保留；不同/未知账号不得自动消费上一账号遗留 query。 |
| 远程搜索/续页 | `Search.vue` 的远程分支同时校验本地 request gate 与 workspace mode/revision。 | **部分一致**：新搜索/切场景正确；workspace 无 session generation，且旧组件其它异步链不受保护。 |
| Search 初始化书源 | `Search.vue#loadSources` 在 await 后会设置本地 sources，并可能通过 computed setter 修改共享 `preferences.search`；组件卸载只 invalidate 远程搜索 gate。 | **must-fix**：A 的迟到书源列表可为 B 选择/保存 A 的 group/source ID，随后触发错误搜索。 |
| 本地搜索 | `searchLocalBooks()` 没有 request token、workspace stamp 或账号 scope；finally 和消息也无门禁。 | **must-fix**：A 的本地书仓/书架结果、loading 或提示可在 B 已进入搜索后回写。 |
| 本地导入 | `importLocalPaths()` 在响应后直接 `bookshelf.upsertBook()`、修改本地结果并提示成功。 | **must-fix**：A 的迟到导入响应不得写入 B 的书架内存或显示为 B 的成功操作。服务端事务仍属于原 Bearer 用户，不改 API。 |
| 搜索结果临时阅读 | `openRemoteReader()` 创建会话后无 scope/generation 检查便 push `/reader/remote/:sessionId`。 | **must-fix**：A 的迟到响应不得把 B 导航到 A 的临时阅读会话。 |
| Explore 入口 | `ExploreWorkspacePopover.selectEntry()` 只有组件本地 request gate，缺少 unmount invalidation 和 workspace/session stamp；`loadSources()` 也无门禁。 | **must-fix**：旧入口响应可调用 `showExploreResults()`，把 B 切到 A 的来源/URL/结果。 |
| Explore 续页 | `Discover.vue` 有本地 gate 与 workspace mode/revision，并在 unmount invalidate。 | **部分一致**：场景切换正确；增加 session generation 后才能证明账号切换安全。 |
| 路由 BookInfo | `AppLayout.openRouteBookInfoOverlay()` await `/books/:id` 后直接 upsert/打开全局 overlay，没有 operation guard。 | **must-fix**：A 的迟到 200 可在 overlay 已清空后重新把 A 书籍挂到 B。 |
| 侧栏快速搜索书源 | `useAppSidebarSearch.loadSources/refreshSourcesCache` 在 await 后直接应用数据并写共享 search preference；组件无 dispose/generation。 | **must-fix**：A 的缓存/网络书源回调可污染 B 的搜索配置。 |
| 书架、偏好、Reader、WebSocket | 已由前置合同冻结 scope/token/operation generation，并在 session clear 重置。 | **已对齐，保留**；本批不得重写这些既有门禁。 |
| 全局 overlay | `overlay.resetSessionState()` 已在凭证移除前关闭并解决待处理 Promise。 | **已对齐但消费者仍 must-fix**：旧 Index callback 不能在重置后重新打开 overlay。 |

## 3. 目标状态机

### 3.1 会话失效

`authenticated(A) → invalidating(A)` 必须同步完成：

1. 记录可证明的旧账号 scope。
2. 若是被动 401，最多挂起一个不含结果行的 Index intent：
   - `mode=search`：关键词、remote/local、搜索类型、分组、source ID、并发；
   - `mode=explore`：source ID、分组、入口 URL、入口名、来源名；
   - `mode=shelf`：只记录 shelf。
3. 清空当前 mode、结果行、分页、loading、滚动位置和可见 chooser。
4. 增加 workspace session generation，并使 search、explore、chooser 的旧 revision 全部失效。
5. 在移除 token 前派发 session invalidation；旧组件的 success/error/finally 均不得再提交。
6. 阻塞全部 authenticated route body，仅保留中性等待面与根登录 Dialog。

结果行、本地路径、远程变量、书籍对象和 loading 不进入挂起 intent。

### 3.2 同账号重新认证

`invalidating(A) → authenticating(A) → authenticated(A')`：

1. token 与 profile 写入后仍保持根壳阻塞。
2. 恢复挂起的场景 intent，但不恢复结果、分页或滚动位置。
3. 递增对应 revision；新挂载 Search/Explore 只能用 A' token 从第一页重新请求。
4. canonical store-only 搜索也能恢复；旧链接 query 允许重新规范化同一 intent。
5. 完成路由与 intent 恢复后才解除阻塞。

Explore 恢复不得先展示旧结果。允许重新打开 chooser，或直接按已选择入口重新获取第一页；实现必须
选择一种确定行为并以测试固定。本合同推荐直接重取已选择入口第一页，减少重复选择。

### 3.3 不同账号或未知旧身份

`invalidating(A|unknown) → authenticating(B) → authenticated(B)`：

1. 丢弃挂起 intent。
2. 清理 `workspace/q/mode/searchType/group/sourceId/concurrent/url/name` 等工作台兼容 query，
   保留与工作台无关且对 B 安全的 query；若无法证明则直接 replace canonical Home。
3. 保持 workspace 为干净 shelf；不得自动打开 chooser、BookInfo 或旧操作 overlay。
4. 路由 settled 后解除阻塞。

启动时本地 token 已被客户端移除、因而无法证明旧 scope 的情况按 unknown 处理。用户在完全未认证
状态下主动打开的安全站内 `/search` 或 `/discover` 登录回跳仍可执行，因为不存在可见的旧账号
现场；它不能携带远程结果对象。

### 3.4 显式注销

显式 logout 不挂起场景：立即清空 Index，进入 Login。之后无论登录同账号还是其它账号都从干净
书架开始。被动 401 与主动 logout 必须是不同动作，避免“退出后又恢复旧搜索”。

## 4. 异步提交规则

每个 Index 私有异步动作开始时冻结：

```text
scope + bearer token identity + user session generation + workspace session generation
```

场景型请求再冻结对应 mode/revision。只有全部仍匹配时才允许：

- 写 Pinia store、localStorage 或浏览器缓存；
- 更新 loading；
- 显示成功、警告或错误消息；
- 打开 overlay；
- `router.push/replace`；
- 将导入结果 upsert 到书架。

`onBeforeUnmount` 仍要 invalidate 组件本地 gate，以便普通路由切换及时取消；但 unmount 不是账号
隔离的唯一证明，因为 token、Vue flush 和 Promise completion 存在同一事件循环竞争。

## 5. 路由、API 与数据合同

- 不新增或修改 Go API。
- 不改 JWT、SQLite schema、`data/`、`cache/`、`library/`、备份或 WebDAV 格式。
- 不持久化 workspace session generation 或挂起 intent；刷新浏览器后只按安全 URL 重新建立状态。
- `/search`、`/discover` 与 root query 兼容入口继续保留。
- 不把结果对象编码进 URL、localStorage 或 sessionStorage。
- `safeReturnTo()` 的同源路径约束继续适用；账号切换决策必须在 consume returnTo 前完成。

## 6. 测试先行闸门

实现前必须先建立会在当前代码失败的测试：

1. `indexWorkspace.reset/suspend/resume`：
   - session clear 后 mode=shelf、rows/continuation/scroll/intent 可见态清空；
   - generation/revision 淘汰旧 stamp；
   - 同账号只恢复 intent 并从空结果开始；
   - 不同账号/显式 logout 不恢复。
2. 根壳与认证：
   - token 已写入但 reauthentication 尚未 settle 时，Reader 和 AppLayout 都不挂载；
   - 同账号恢复 store-only Search；
   - 不同/unknown 账号 replace Home 并清工作台 query；
   - 首次匿名登录仍可使用安全 returnTo。
3. 迟到请求：
   - Search 远程、本地、书源初始化、导入与临时阅读；
   - Explore 来源加载、入口第一页与续页；
   - route BookInfo；
   - sidebar cache-first/network refresh。
   每项都要断言 A callback 在 B 会话中不能写 store/preference、改变 loading、弹消息、开 overlay
   或导航。
4. 保留既有 P1-B 行为：同会话内新搜索、返回书架、切 Explore 的 revision gate，单/多源续页、
   `bookUrl` 去重和 BookInfo 五入口测试继续通过。
5. 全量 `npm test`、`npm run build`、`go test ./...`。
6. 真实浏览器 1440×900、390×844、360×800：
   - Search/Explore 请求挂起时触发 401；
   - 同账号重新登录只显示新请求结果；
   - 不同账号回到空净书架；
   - 无旧 toast、旧 overlay、旧临时 Reader 跳转或水平溢出。

## 7. 受控实施顺序

1. 先写 store、根壳、路由和迟到回调失败测试。
2. 为 `indexWorkspace` 增加非持久 session generation 与显式 suspend/reset/resume/discard 动作。
3. 把 Index reset 接入 `userStore.clearSession()`，区分被动失效与主动 logout。
4. 让 AuthDialog/Login 在账号判定、路由和 workspace intent settled 后统一解除 authenticated shell
   阻塞。
5. 给 Search、Explore、sidebar source load、route BookInfo、导入和远程阅读交接补齐冻结身份门。
6. 完成自动回归后再做三视口浏览器；浏览器闸门通过前不发布本切片 Docker。

## 8. 2026-07-27 候选实施记录

- `indexWorkspace` 新增非持久 `sessionGeneration`、一次性挂起 intent 以及
  suspend/resume/discard/reset 动作。清理立即移除结果、分页、loading、滚动位置、搜索/探索
  可见 intent，并让旧 scene stamp 失效；结果行、书籍对象、本地路径和远程变量不会进入挂起态。
- 被动认证失效会挂起一个最小工作台 intent；同账号登录只恢复 intent 并从空结果重新执行。
  Explore 使用一次性 `exploreChooserPending`，所以认证期间产生的恢复请求可在 AppLayout 挂载后
  消费一次，也不会在以后从 Reader 返回时重复打开。不同/未知账号和显式 logout 保持干净书架。
- 根壳在 token 写入后仍以现有 session-blocked 状态阻止 Reader 和 AppLayout 挂载。AuthDialog
  对不同/未知账号先 replace canonical Home，Login 对已知异账号忽略旧 returnTo；路由 settled
  后才解除阻塞。完全匿名首次登录仍可消费经过 `safeReturnTo()` 校验的站内路径。
- Search 的书源初始化、本地搜索、本地导入和临时阅读交接，Explore 的来源/入口/续页和临时
  阅读，侧栏 cache-first/network 书源加载，以及 route BookInfo 都加入身份或
  workspace-session 提交门。旧 success/error/finally 不能改新账号 store/preference/loading、
  toast、overlay 或路由。
- 新失败测试先在原实现得到 13 个预期失败，实施后聚焦 42/42；另有真实 deferred Promise 用例
  验证身份变化与 dispose 后的侧栏书源响应不会设置新账号默认分组/来源。最终
  `npm test` 为 611/611，`npm run build`、`go test ./...` 与 `git diff --check` 通过。
- 本批不改 Go API、JWT 格式、SQLite、`data/`、`cache/`、`library/`、备份或 WebDAV。旧 token
  只存在短生命周期 operation closure，不写 Pinia、URL、storage、日志或错误文本。
- 三视口真实浏览器仍是未完成门禁；前一批外部浏览器/本地服务申请被环境拒绝后未使用替代路径。
  因此本候选可以同步 GitHub，但还不能据此发布 Docker 或宣称本合同最终验收完成。
