# P1 工作台全局弹层认证会话隔离合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-28 已完成固定上游取证、P1-A/P1-B 实施、自动门禁、真实浏览器
`6 场景 × 3 视口` 最终签收和本地 Docker 发布。

> 2026-08-02 名称更新：BookInfo 远程加书的共享安全实现现为
> `useRemoteBookAddToShelf`；结果卡与 BookInfo 分别调用“确认分组”和“直接加入”策略。
> 本文关于认证 generation、迟到响应拒绝和会话重置的合同不变。

本合同承接：

- [`authenticated-runtime-scope-p2-contract.md`](authenticated-runtime-scope-p2-contract.md)
  已完成的 Pinia、偏好、进度与 WebSocket operation generation；
- [`reader-reauthentication-isolation-p0-contract.md`](reader-reauthentication-isolation-p0-contract.md)
  已完成的根壳阻塞、Reader 卸载与全局 overlay 同步清空；
- [`index-authenticated-session-p1-contract.md`](index-authenticated-session-p1-contract.md)
  已完成的搜索、探索、侧栏和路由 BookInfo 入口隔离。

前置合同证明会话失效时 `overlay.resetSessionState()` 会关闭现有弹层，也证明重新认证路由落定前
`GlobalOverlayHost` 不会重新挂载；它们没有证明已发出的弹层异步操作会随组件卸载失效。当前多数
弹层在 `await` 后仍会更新 Pinia、弹消息、导航、打开/关闭下一层弹窗，或继续发出下一次请求。
因此 A 账号发出的迟到回调仍可能在 B 账号登录后污染 B 的运行时。

## 1. 固定上游权威行为

| 场景 | 上游证据 | 产品语义 |
| --- | --- | --- |
| 弹层所有权 | `web/src/App.vue` 常驻持有 BookInfo、BookManage、UserManage、BookGroup、RSS、书签和替换规则等全局 Dialog；`Index.vue` 持有 LocalStore、WebDAV 和导入 Dialog。 | 工作台和 Reader 复用同一组业务弹层；子弹层之间可以通过根事件总线衔接。 |
| 登录失效 | `web/src/plugins/axios.js` 收到 `NEED_LOGIN` 后打开根登录框。 | 失效身份不能继续驱动当前工作台动作。 |
| 登录恢复 | `App.vue#login` 保存 token、读取用户信息、同步本地状态并调用 `init(true)`；`Index.vue` 监听 `loginAuth` 和 `userNS` 后重新初始化。 | 恢复后显示当前身份重新加载的数据，不能把旧用户响应当作新用户状态。 |
| Dialog 打开加载 | BookManage、BookGroup、Bookmark、LocalStore、WebDAV、ReplaceRule、RSS 和 UserManage 都在 `show/visible` 变为 true 后读取各自数据。 | 重开弹层必须从当前用户空间读取，不能恢复另一身份的结果行或选择。 |
| 异步请求取消 | 上游只有 SSE/少量缓存任务显式关闭，普通 Promise 没有跨 `loginAuth/userNS` generation。 | 这是上游单命名空间模型的缺口；OpenReader 的 JWT 多账号运行时不得复制。 |

`init(true)` 不能解决迟到 Promise：请求 A 可以在 `init(true)` 已开始加载 B 后完成。OpenReader
必须把“当前账号、当前 token、当前组件代际”作为提交条件。这属于允许且必需的多用户安全适配，
不改变上游可见流程。

## 2. 当前实现审查矩阵

| 模块 | 当前证据 | 风险与裁决 |
| --- | --- | --- |
| overlay 根状态 | `stores/overlay.js#resetSessionState` 同步关闭全部弹层，并解决选择分组/书签表单 Promise。 | **已对齐，保留**；但不能替代消费者异步门禁。 |
| 根壳 | `App.vue` 在 session blocked 时卸载 `GlobalOverlayHost`。 | **已对齐，保留**；Vue 卸载不会取消已经发出的 Promise。 |
| BookInfo / 编辑 / 加书 | `OverlayBookInfo.vue`、`useOverlayBookInfo`、`useBookInfoAddToShelf` 在刷新、上传封面、编辑、追更和远程加书返回后直接 upsert 书架、改 overlay、写 Reader cache、广播并提示。 | **must-fix / 最高风险**：A 的书对象可进入 B 的书架和 Reader cache，或在重置后重新改弹层。 |
| BookManage / 缓存 / 导出 | 打开时加载书架和浏览器缓存统计；批量分组/删除、单书缓存、清缓存和导出在 await 后直接写共享 store 或提示。服务器/浏览器缓存任务由模块级 map 持有。 | **must-fix**：普通动作必须隔离；现有 `cancelAllBookManagementCacheJobs()` 在凭证移除前取消长期任务，应保留，不把 cache job 生命周期误缩短为 Dialog 生命周期。 |
| BookGroup | `useOverlayBookGroups` 的设置分组、增删改、显隐与排序在 await 后直接更新 bookshelf、BookInfo overlay、广播和关闭 Dialog。 | **must-fix**：迟到设置分组不能覆盖 B 的书对象或关闭 B 新开的分组弹层。 |
| Bookmark / 表单 / 正文搜索 | `useBookBookmarks` 只有 book id/load token；表单保存、导入、删除、跳转和更新事件没有认证 identity。正文搜索仅有查询 revision。 | **must-fix**：不同账号可能存在相同自增 book id；仅比较 id 不能证明身份，迟到导航、toast、表单 resolve 和结果写入都需拦截。 |
| 上传导入 | `useOverlayBookImport` 预览有 AbortController 与本地 generation，但分组加载、目录规则加载和正式导入没有认证 guard。 | **部分一致 / must-fix**：保留预览取消；正式导入的 A 响应不得 upsert 到 B、关闭 B 弹层或提示 B 成功。 |
| LocalStore | `LocalStoreManager` 在 mount 自动加载；目录加载、上传、删除和批量删除没有 unmount/identity guard。 | **must-fix**：A 的目录结果和消息不得提交到 B；一个动作切换账号后不得继续以 B token 执行 `load()`。 |
| WebDAV / 恢复 | `WebDAVBrowser` 在 mount 自动加载；上传、下载、删除、导入与恢复没有认证 guard。恢复成功会调用 `applyRestoreResult` 刷新多个全局 store。 | **must-fix / 最高风险**：A 的恢复响应绝不能在 B 会话调用全局恢复收敛；账号变化后不得继续请求 B 的目录。 |
| StorageImport | `useStorageImportWorkflow` 的 reset 只清本地 phase；预览、重解析、逐本/批量导入在 await 后仍可 upsert、提示、关闭 overlay，批量循环可在换号后继续下一项。 | **must-fix / 最高风险**：会话变化必须淘汰当前 workflow，并停止后续队列请求。 |
| SourceManager / 导入 | 来源加载、默认状态、健康检测、编辑、批量操作、默认源保存/恢复、调试和 `useSourceTransfer` 都没有认证 guard；卸载只清 timer/listener。 | **must-fix**：迟到结果不得改 B 的来源列表、默认状态或调试结果；确认框后账号已变时不得发写请求。 |
| ReplaceRule | `useOverlayReplaceRules` 有 manager/editor request 与 timer reset，能够淘汰同组件旧 load；不比较账号/token，写操作会继续 load。 | **部分一致 / must-fix**：保留现有同场景 revision；增加认证 identity，避免 A 写响应驱动 B load、toast 或编辑器状态。 |
| RSS | `RSSManager` 只有 list/article request counters 和 reload timer；源/文章加载、保存、导入、刷新、删除、正文和已读/收藏更新均不比较账号。 | **must-fix**：本地 request counter 只解决同组件竞态，不解决卸载后换号；浏览器 RSS cache 的账号 scope 也必须由提交门保护。 |
| UserManage | `useOverlayUserManagement` 具有 manager request/reset，但创建、改权限、重置密码、删除和后续 load 不比较身份。 | **must-fix / 最高风险**：管理员 A 的迟到动作不得改变 B 的管理列表或继续以 B token 请求；旧提示不得泄漏管理动作。 |
| 备份动作 | `useWorkspaceBackupActions` 在确认后触发备份并弹成功/失败。 | **must-fix**：确认框期间账号变化时不得执行；A 的完成消息不能出现在 B。 |
| 仅局部 UI ref | loading、selection、editor draft 等随组件卸载消失。 | **仍需门禁**：虽然不持久，但旧 finally/toast/后续 API 会跨会话；不能以“组件已卸载”判定安全。 |

## 3. 目标状态机

### 3.1 操作身份

每个异步动作开始时冻结：

```text
scope + bearer token identity + component/session generation + operation key revision
```

同一 key 的后发动作淘汰前发动作；组件卸载或 `openreader:session-invalidated` 增加 generation 并
淘汰所有 key。只有 identity、generation 与 key revision 全部匹配时才允许提交。

### 3.2 会话失效

`authenticated(A) → invalidating(A)` 必须同步完成：

1. 根状态关闭所有账号私有弹层并解决待处理 UI Promise；
2. 各挂载弹层收到 session invalidation 并立即 reset 本地 operation guard；
3. 可取消的预览、SSE、缓存和上传请求执行既有 abort/cancel；
4. 不可取消请求可以在服务端以原 Bearer A 完成，但其 success/error/finally 不再提交；
5. 队列、批处理和“写后刷新”链不得继续发下一次请求。

### 3.3 同账号与异账号重新认证

不论是否同账号，弹层都不恢复：

- 根壳解除阻塞后保持 overlay 全部关闭；
- 用户主动重开某弹层时使用新 token 从初始路径重新加载；
- 不恢复目录 path、选中行、编辑 draft、导入预览 token、RSS 文章、调试结果或 loading。

这是账号私有操作界面，与可恢复最小 intent 的 Index 搜索/探索不同。上游登录恢复会重新 `init(true)`
而不是恢复 Dialog 内中间事务，因此关闭并重载符合产品语义。

### 3.4 允许提交与禁止提交

旧 operation 失效后禁止：

- 写 Pinia、localStorage、CacheStorage/IndexedDB 或组件 ref；
- upsert/remove 书架、应用恢复结果、写 Reader cache、派发业务更新事件；
- 打开、关闭或 resolve 新会话 overlay；
- `router.push/replace`、下载文件或创建 object URL；
- 显示 success/info/warning/error；
- 继续执行批量队列、写后 reload 或下一阶段 API。

服务端已经收到的单次写操作属于 dispatch 时 Bearer 对应用户，不尝试客户端“回滚”。如果服务端
操作成功，同账号或其它客户端可通过后续明确刷新/WebSocket 收敛；客户端只禁止把它伪装成当前
新会话的本地结果。

## 4. 实施边界

- 不改 Go API、JWT、SQLite schema、`data/`、`cache/`、`library/`、WebDAV 或备份格式。
- 不把 token 放入 Pinia、URL、storage、日志、事件或错误文本；token 只存在短生命周期 operation
  closure。
- 复用 `createAuthenticatedOperationGuard`，增加 Vue 生命周期适配层，避免每个组件各造一套
  scope 比较。
- 业务 composable 接受可注入的 begin/canCommit/reset，使 Node 单测可用 deferred Promise
  精确验证，不依赖字符串匹配。
- 保留现有模块内 revision、AbortController、timer 清理和缓存 job cancel；认证门是叠加条件，
  不是替换。
- 本批只处理前端运行时隔离，不顺带重写各模块 UI/API 兼容性。

## 5. 测试先行闸门

实现前必须让当前代码出现预期失败：

1. 生命周期 guard：
   - identity 改变、session invalidation、scope dispose 后 `canCommit=false`；
   - 不暴露 token，不跨 operation key 相互误伤。
2. 最高风险真实 deferred Promise：
   - BookInfo/远程加书的 A 响应不能 upsert B 书架、写 Reader cache、广播或关弹层；
   - StorageImport 批量导入切号后停止队列，不能导入下一本或 upsert；
   - WebDAV 恢复切号后不能调用 `applyRestoreResult` 或继续 load；
   - UserManage 写动作切号后不能继续 load 或弹消息。
3. 其余模块 wiring/行为：
   - BookManage、BookGroup、Bookmark、Source、ReplaceRule、RSS、LocalStore、上传导入和备份均在
     operation 开始冻结身份，在每个跨 await 边界提交前复查；
   - onBeforeUnmount/session invalidation 会 reset；
   - error/finally 同样受门禁，不产生旧 toast 或覆盖新 loading。
4. 保留原有操作合同：
   - 同账号同组件的正常加载、编辑、导入、删除、缓存、恢复与导航行为不变；
   - BookManage 长期缓存任务仍可在普通关闭 Dialog 后继续，只有 session invalidation 才取消；
   - StorageImport 原有 staged token、重解析与逐本/批量流程继续通过。
5. 全量 `npm test`、`npm run build`、`go test ./...`。
6. 真实浏览器 1440×900、390×844、360×800：
   - 在 BookInfo 加书、StorageImport、WebDAV 恢复、Source/RSS 保存和 UserManage 操作挂起时触发
     401；
   - 同账号重登后弹层保持关闭，手动重开只出现新请求数据；
   - 异账号不出现旧行、旧 toast、旧下载、旧导航或旧 overlay；
   - 无控制台错误、401 重试风暴或横向溢出。

## 6. 受控实施顺序

1. 新增通用 Vue 认证 operation 生命周期适配层及失败测试。
2. 先覆盖会写全局状态的 BookInfo、StorageImport、WebDAV restore、UserManage。
3. 再覆盖 BookManage、BookGroup、Bookmark、LocalStore、Source、ReplaceRule、RSS、上传导入和备份。
4. 运行全量自动门禁并同步 GitHub。
5. 完成真实浏览器闸门后，才将本切片与此前待验收的 Reader/cache/Index 候选一起纳入本地 Docker
   构建与 GHCR 发布。

## 7. 2026-07-27 P1-A 候选实施记录

- 新增 Vue 生命周期认证 operation guard：冻结 scope/token，监听
  `openreader:session-invalidated`，并在 scope dispose 时统一淘汰。
- BookInfo 的远程加书、编辑、刷新本地书、上传封面和追更更新，在每个跨 await 提交前复查；
  失效响应不再 upsert 书架、写 Reader cache、广播、提示或关闭弹层。
- StorageImport 的预览、重解析和确认导入叠加认证门；批量导入在身份失效后立即停止，不继续下一本、
  不 upsert、不完成旧事务。
- WebDAV 的目录、上传、下载、删除和恢复均冻结身份；恢复在 `applyRestoreResult` 前后复查，
  `applyRestoreResult` 本身也支持提交门，避免迟到恢复派发全局更新事件。
- UserManage 的加载、新增、重置密码、批量删除和权限更新均使用同一生命周期门，确认框期间换号
  不继续写请求，迟到响应不提示或触发写后 reload。
- 先得到 7 个预期失败，实施后聚焦 31/31、前端全量 618/618、生产构建、Go 全量与
  `git diff --check` 通过。
- P1-A 仍是自动门禁候选；BookManage、BookGroup、Bookmark、LocalStore、Source、
  ReplaceRule、RSS、上传导入和备份动作属于 P1-B，真实浏览器与 Docker 均未完成。

## 8. 2026-07-27 P1-B 候选实施记录

- BookManage 的打开加载、批量分组/删除、单书服务器/浏览器缓存、清缓存、导出和缓存统计共用
  生命周期认证门；模块级长期 cache job 仍按账号 scope 保存，普通关闭 Dialog 不取消，
  `session-invalidated` 继续通过既有全局清理取消。
- BookGroup、Bookmark、书签表单和正文搜索在加载、确认、写入、导航/关闭、广播和提示前复查
  operation；仅有相同 book id 不再被视为相同身份。
- 上传导入保留原有 AbortController、preview generation 和 staged token，同时覆盖分组/目录规则
  加载与正式导入；身份变化后不再提示成功、关闭弹层或继续回填。
- LocalStore 冻结目录、上传文件和删除集合；确认框后换号不发请求，旧响应不写路径、列表、
  selection、toast 或后续 reload。
- SourceManager 覆盖列表/默认状态/失败缓存、编辑、批量动作、默认快照、恢复、健康检测和调试；
  `useSourceTransfer` 覆盖本地/远程预览、确认导入和导出。确认框、文件读取和请求每一阶段都验证
  身份，selection/source id 在 await 前冻结。
- ReplaceRule 在原有 manager request 与刷新 timer 之外叠加认证代际；加载、导入、编辑、测试、
  启停和删除均不会由旧账号触发写后 reload、事件或提示。
- RSS 保留文章 query request gate，并增加 root lifecycle identity：源缓存/加载、文章分页、
  保存/导入/刷新/删除、正文和已读/收藏均受门禁；延迟 timer 与子 Dialog 关闭会淘汰对应请求。
- 工作台普通/可移植备份在确认前冻结身份；确认期间换号不会 dispatch，迟到结果不会向新账号
  显示路径或错误。
- 测试先得到 6 个预期失败；实施后增加 deferred 行为测试，证明旧书源导入不能刷新新会话、
  旧替换规则确认不能在新会话删除。聚焦 60/60 与补充 35/35 通过，前端全量 626/626、生产
  构建、Go 全量和 `git diff --check` 通过。
- P1-A/P1-B 当前均为自动门禁候选。真实浏览器需覆盖 1440×900、390×844、360×800 的
  401/同账号重登/异账号切换；该闸门完成前不把本合同标记为最终验收，也不发布 Docker。

## 9. 2026-07-28 真实浏览器最终门

- 新增 `scripts/smoke/workspace-overlay-session-isolation-contract.mjs`，使用一个浏览器顺序运行
  BookInfo 加书、StorageImport、WebDAV 恢复、Source 保存、RSS 保存和 UserManage 新增六条
  真实 Overlay 流程，避免并发创建多个 Chromium 导致设备压力。
- 每条流程让 A 的写请求保持 pending，触发与 Axios 401 拦截器相同的根认证失效事件，完成
  同账号续登或 B 账号登录后才释放 A 的旧响应。断言覆盖：账号私有弹层保持关闭、旧 toast/
  业务事件/写后 reload 为零、B 书架不被替换，以及手动重开只显示当前账号请求的数据。
- 默认覆盖 `1440×900`、`390×844`、`360×800`。为降低设备排障时的单次浏览器压力，可用
  `OVERLAY_SCENARIOS=book-info,storage-import` 和
  `OVERLAY_VIEWPORTS=390x844` 选择受控子集；完整签收仍必须不带过滤运行全矩阵。
- 首次执行发现并修正三项测试合同问题：整页导航只能在 storage 无 token 时注入初始 A
  token；书源兼容入口必须使用 `/sources` 而不是不存在的
  `/settings?panel=sources`；移动搜索按上游保持侧栏打开，测试在点击结果前显式点击工作区收起。
- 真实执行同时发现一个产品回归：RSS `openEditor()` 把手动新增的 `null` sentinel 直接传给
  高级字段提取，抛出 `TypeError` 并使新增弹窗无法出现。固定上游会先把 falsy 值归一化为
  新源默认对象；OpenReader 已恢复该状态转换，并给编辑器增加稳定的私有弹层根标识。
- 最终不带过滤运行完整矩阵，输出覆盖：
  `1440x900`、`390x844`、`360x800` 各自的
  `book-info/storage-import/webdav-restore/source-save/rss-save/user-create=ok`，并确认
  `staleToasts=0 staleEvents=0 staleReloads=0 manualReopen=current-account`。
- 当前自动门：frontend `645/645`、Vite production build、Go `go test ./...` 与
  `git diff --check` 全部通过。本合同由 **candidate** 转为
  **implemented / browser-validated / Docker-published**。
- 本地候选 `ghcr.io/changshengyu/openreader:342d736` 通过 portable v1、portable v2
  assets、cross-user、restart，以及历史 TXT/EPUB/UMD/CBZ、relative-cache 和
  owner-isolation 两套挂载卷门禁。随后从本机发布 `342d736` 与 `latest`；二者共同指向
  amd64/arm64 OCI index
  `sha256:1643625269f5a04f867c56da9e3bee04c1318d807e73ca6fc0913ab408645921`。
