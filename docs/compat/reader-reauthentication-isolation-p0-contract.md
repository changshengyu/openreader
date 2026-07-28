# P0 Reader 登录失效与账号切换隔离合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-27 完成固定上游与当前实现取证，随后按测试先行完成第一版候选实现。2026-07-28
按真实 Axios/stream 401 调用顺序复审时发现失效账号 scope 捕获仍有缺口：拦截器先删除
localStorage token，store 后续只从 localStorage 解码身份，因此同账号重新登录可能被误判为
未知/不同账号。该项重新标记为 `must-fix`；真实浏览器与 Docker 发布闸门仍未完成。

本切片补充已经完成的
[`authenticated-runtime-scope-p2-contract.md`](authenticated-runtime-scope-p2-contract.md)：
后者隔离 Pinia 异步 operation 和 WebSocket generation，本切片隔离仍然挂载在页面上的 Reader
组件、正文 DOM、全局书籍弹层以及退出时的进度保存。

## 固定上游权威行为

| 场景 | 上游证据 | 产品语义 |
|---|---|---|
| 请求要求登录 | `web/src/plugins/axios.js` 收到 `NEED_LOGIN` 后提交 `setShowLogin(true)`；`vuex.js#setShowLogin` 同时把 `loginAuth` 设为 `false`。 | 当前操作停止，根级登录框出现；Reader 不应把失败继续解释成普通章节错误。 |
| Reader 保持路由 | `web/src/App.vue` 的根登录 Dialog 与 keep-alive `router-view` 并存。 | 登录窗口覆盖当前场景，用户不需要先手工返回 Index。 |
| 登录后重新初始化 | `App.vue#login` 保存 token、读取用户信息、同步该用户 localStorage 后执行 `init(true)`；`Reader.vue` 监听 `loginAuth=true` 并执行 `init(true)`。 | 认证恢复后重新读取当前身份的书籍、目录和正文，不能继续信任失效前的请求结果。 |
| 取消登录 | 上游 `App.vue#cancel` 允许关闭登录框。 | 是否允许关闭属于 UI 选择；但 JWT 多用户版本不能因此重新暴露上一账号的私有正文。 |

上游是单命名空间 token 模型，没有 OpenReader 的 JWT owner、Pinia、REST ID 和多用户浏览器缓存。
因此“认证失效后立即移除上一身份的可见数据”和“不同账号不得恢复上一账号 Reader”是必要的安全/
运行时适配，不是对上游阅读流程的任意改写。

## 当前实现审查矩阵

| 链路 | 当前证据 | 裁决 |
|---|---|---|
| 401 通知 | `frontend/src/api/client.js` 只在非登录请求携带的当前 token 被 `401` 拒绝时移除 localStorage token，并发送带 `rejectedToken` 的 `openreader:auth-required`。 | **partial / must-fix**：rejected-token 防止旧 401 清除新登录的门禁正确，但该 token 还必须只作为失效身份 scope 的一次性提示传给 store；当前先删除 storage 会让后续身份捕获退化为 anonymous。 |
| 根场景门禁 | `App.vue` 先判断 `isReader`，后判断 `isLoggedIn`。活动 Reader 在 token 已清空后仍渲染；路由 guard 只在下一次导航时执行。 | **must-fix**：未认证时 Reader 组件和私有正文 DOM 必须立即卸载/清空，不能依靠后续导航。 |
| 登录提示 | `AuthDialog` 会显示“登录状态已失效”，但 Element Plus 默认允许关闭、按 Esc 或点击遮罩退出。关闭后当前 Reader 仍可见。 | **must-fix**：session 失效时提示必须保持可达；若允许取消，底层只能是无私有内容的认证阻断面。 |
| 登录成功 | `AuthForm` 先由 `user.login()` 写入新 token，再由 `AuthDialog#handleSuccess` 执行 `window.location.reload()`。 | **must-fix**：不得用整页刷新充当会话隔离；以新的认证 generation 重新挂载 Reader。 |
| 退出进度 | Reader 的 `pagehide`、`visibilitychange`、route leave 和 `useReaderPageLifecycle#unmount` 都可能强制保存。`useReaderProgressPersistence#sendKeepAlive` 在调用时读取“现在”的 token，`applyLocal` 同样按“现在”的 scope 写本地进度。 | **P0 must-fix**：认证失效必须在 token 变化前同步挂起 Reader 进度 generation；旧 Reader 不能把 A 的书/位置写到 anonymous 或 B。 |
| Reader 异步加载 | `useReaderBookLoad`、`useReaderChapterLoader` 主要用 book ID/cache key 判断迟到结果；组件会在 401 后继续挂载。 | **must-fix by lifecycle**：失效时卸载 Reader 并取消其提交资格；A 的迟到 book/catalog/content/EPUB/audio/TTS 回调不得重新产生可见状态。 |
| 浏览器缓存 | Reader book/catalog/chapter key 已带用户 scope，且认证账号不回退读取未归属 legacy 章节正文。 | **aligned，必须保留**：本切片不删除另一账号缓存，也不恢复未归属 fallback。 |
| 全局弹层 | `GlobalOverlayHost` 随登录状态卸载，但 `useOverlayStore` 没有会话 reset。B 登录后 Host 重新挂载时，A 的 BookInfo、书签、正文搜索、编辑或导入状态可重新出现。 | **must-fix**：会话清理要结算未完成 Promise，并清空所有账号相关 overlay 对象、可见状态和跳转 intent。 |
| Pinia/WS scope | `24feff5` 已给资料、Reader 设置、偏好、进度、分类和 WebSocket 添加 scope/token/generation guard。 | **resolved / do not reopen**：继续复用；本切片只补活动页面生命周期。 |
| 后端 owner | `/books/:id`、目录、正文、进度、书签、远程 Reader session 均按 JWT 用户授权。 | **aligned**：不改 API、数据库或 owner 条件；前端不得把服务端 404/401 当作可继续显示旧正文的理由。 |

## 目标状态机

### 1. 认证失效

1. 当前 token 的 401 触发一次 session invalidation；旧 token 的迟到 401 仍为 no-op。
2. 在清除 token、profile 和用户 store 之前，同步使活动 Reader 的进度/章节/媒体 generation 失效，
   取消定时保存、自动阅读、TTS、音频意图和后台章节缓存任务。
3. 同一状态转换关闭并清空账号相关全局弹层；等待书签/分类等选择结果的 Promise 以
   `session-invalidated` 失败/取消结果结算，不能永久悬挂。
4. `App` 立即停止渲染 Reader 和 GlobalOverlayHost。认证 Dialog 下方只能显示不含书名、正文、
   目录、书签、封面或进度的中性阻断面。
5. 随后的 `pagehide`、`visibilitychange`、route leave、组件 unmount 和旧 Promise continuation
   均不得调用远端进度 PUT/keepalive，也不得写 anonymous/new-user localStorage。

### 2. 重新登录同一账号

1. 登录成功创建新的非持久 session generation，不复用旧 Reader 组件。
2. 保留原 Reader URL、chapter/offset/percent intent；从该账号的书架、进度、目录和正文重新加载。
3. 不执行 `window.location.reload()`；新 Reader 的 loading/error 只属于新 generation。
4. 登录 Dialog 关闭后工具层默认显示、面板状态重新使用上游默认值；不恢复失效前可能属于旧
   generation 的临时面板、选区、TTS 或音频播放。

### 3. 重新登录不同账号

1. 比较失效前捕获的用户 scope 与新 token scope；不得尝试把 A 的数据库 book ID 当作 B 的书。
2. 返回 Index/书架并清除旧 Reader 的 route intent。B 可从自己的书架重新选择书籍。
3. A 的缓存、SQLite 行和文件保持原样，不做跨账号删除；只是 B 的运行时不可读取或显示它们。

### 4. 未认证直达

路由 guard 保留完整的安全 `returnTo`。用户从未认证状态打开 Reader 时进入 Login 页面；登录后只
在没有“不同账号接管旧会话”风险时返回该 URL，否则进入书架。禁止 `//host` 等开放重定向。

## API、数据和允许差异

- 不改 REST/WS 路径、请求体、响应、JWT、状态码、SQLite schema、`data/`、`cache/`、`library/`、
  backup/WebDAV 或用户持久设置。
- 不清理另一用户的浏览器缓存；只清空当前页面内存和 Pinia runtime。
- 保留 `rejectedToken`、operation guard、progress CAS、WebSocket generation 和浏览器 scope 隔离。
- 新增非持久 session generation、认证阻断面和跨账号返回书架属于 JWT 多用户安全适配。
- 上游允许取消登录框；OpenReader 可以在 session 失效时禁止误关闭，或允许关闭后显示中性重新登录
  入口。两种实现都不得重新显示旧正文。本轮默认采用不可误关闭的 session Dialog。

## 测试先行闸门

1. `App` 渲染合同：Reader 路由 + token 有效才挂载 Reader；token 清空后旧 Reader/Overlay Host
   同步退出，中性阻断面不含上一账号数据；新 generation 才重新挂载。
2. user/session 单元：`clearSession` 在 token 变化前发出 invalidation；重复 401 幂等；旧 rejected
   token 不影响新 token；登录递增 generation 并保留失效前 scope 供一次恢复裁决。
3. Reader 生命周期：session invalidation 先 suspend，再触发 unmount；之后 pagehide、hidden、
   route leave 和 teardown 均产生零个本地/远端 progress write。
4. overlay 单元：所有账号相关可见状态、book 对象、搜索跳转、导入请求和编辑 draft 清空；书签/
   分类选择 Promise 被明确结算。
5. 同账号流程：mock 401 → Dialog → 同账号登录；不 reload，原 Reader URL 重新挂载，新请求恢复
   chapter/offset，旧延迟响应不提交。
6. 异账号流程：A Reader 401 → B 登录；进入书架，A 书名/正文/目录/弹层/本地进度均不出现，
   `PUT /api/progress` 对 A book 为零。
7. 真实浏览器在 1440×900、390×844、360×800 验证 Dialog 可见、正文立即消失、同账号恢复、
   异账号返回书架、无 401 循环/控制台错误/横向溢出。
8. 全量 frontend test、frontend build、backend test；形成可验证切片后执行本地 Docker
   历史卷/备份门禁并发布。

## 本切片之外

- Index 搜索结果、RSS、书源编辑器等各自的账号切换 draft/operation 继续由对应模块合同审查；
  本切片不声称所有 overlay 内部请求均已自动隔离。
- 不改变密码、注册、JWT 有效期、管理员权限或服务端会话策略。
- 不用整页刷新、延时、吞掉 401 或隐藏测试错误作为完成证据。

## 2026-07-27 候选实施记录

- API 拦截器和章节缓存流只允许仍与 localStorage 当前 token 相同的 401 创建一次认证失效事件；
  旧账号/已退出请求的迟到 401 不会清除或阻断新会话。监听器安装前发生的首个 401 仍可通过唯一
  待处理事件恢复登录 Dialog。
- `clearSession()` 在移除 token 前同步广播无 token 的 session generation；活动 Reader 立即
  suspend 进度、自动阅读、TTS、音频意图和章节缓存任务。pagehide、hidden 与 teardown 在该
  generation 下不再保存。
- `App` 未认证时不渲染 Reader、workspace 或 GlobalOverlayHost，只显示中性阻断面/Login。
  Reader 以非持久 session generation 为 key；同账号重新登录原路重新加载，异账号或未知旧身份
  返回书架，不执行硬刷新。
- 全局 overlay 会先结算书签/分类选择 Promise，再清空书籍、搜索、导入、编辑和可见状态；不删除
  任一用户的持久缓存、SQLite 行或文件。
- route guard 保留经过 `safeReturnTo()` 校验的站内完整路径，拒绝协议 URL 和 `//host`。
- 聚焦 30 项状态机/接线测试、frontend 599/599、`vite build`、`go test ./...` 均通过。
- 真实浏览器未计为通过：本地 Go 服务在 sandbox 内不能绑定 `:8080`，外部启动审批因工作区额度
  不足被拒绝。没有尝试绕过；1440×900、390×844、360×800 和 Docker 仍是发布闸门。

## 2026-07-28 真实 401 顺序复审

### 新发现的残余缺口

固定上游收到 `NEED_LOGIN` 后直接在仍保留当前 Vuex 身份现场的根层打开登录框；登录成功后
Reader 重新 `init(true)`。OpenReader 的 JWT 适配多了一层“被拒绝 token 必须仍是当前 token”
门禁，这是正确的多用户强化，但当前实现把两个动作排成了错误顺序：

1. `frontend/src/api/client.js` 和 `frontend/src/api/books.js` 确认请求 token 仍是当前 token；
2. 两处都先执行 `localStorage.removeItem("openreader_token")`；
3. 随后才发送携带 `rejectedToken` 的 `openreader:auth-required`；
4. `user.requireLogin()` 调用 `clearSession()`，而 `clearSession()` 的
   `currentUserScope()` 只读取已经为空的 localStorage。

因此正常运行中的 A 账号 401、以及 App 监听器安装前暂存的 startup 401，都可能把
`invalidatedScope` 留空。A 重新登录后 `previousScope=""`、`currentScope="user:A"`，
`sameAuthenticatedScope=false`，`AuthDialog` 会错误返回书架；原 Reader URL 不会按上游语义
重新初始化。现有测试直接在 token 仍存在时调用 `clearSession()`，或者明确断言 startup 401 的
`invalidatedScope` 为空，因此没有覆盖真实拦截器顺序，后一个断言本身属于错误测试。

### 修订合同

| 项目 | 目标 |
|---|---|
| scope 解析 | `authScope` 提供从指定 token 解码 `user:<id>` 的纯函数；默认读取 localStorage 的现有调用保持兼容。JWT payload 只用于本地身份相等比较，不作为授权依据。 |
| 失效捕获 | `requireLogin(reason, rejectedToken)` 必须把通过“仍是当前请求”门禁的 token 作为一次性 `tokenHint` 传入 `clearSession()`。`clearSession()` 优先使用 Pinia 中仍存在的 token，其次使用该 hint，最后才读取 storage。 |
| 最小暴露 | 不持久化、记录、返回或放入路由任何 token；store 只保留解析后的 `user:<id>` scope。登录结果继续只返回 `previousScope/currentScope/sameAuthenticatedScope`。 |
| 幂等与旧响应 | 已打开 session Dialog 的重复事件不得再次增加 generation；A 的迟到 401 在 B token 已生效后仍必须 no-op，不能借 `tokenHint` 覆盖 B。 |
| 路由语义 | 同账号重新登录保持当前 Reader URL并以新 generation 重载；不同账号或无法安全识别旧账号时返回书架。 |

### 修订测试门

1. 模拟真实拦截器顺序：Pinia 已持有 A token，但 localStorage 已先删除，再调用
   `requireLogin("session", rejectedTokenA)`；必须捕获 `invalidatedScope="user:A"`。
2. 模拟监听器安装前 startup 401：Pinia token 和 storage 都为空，仅 pending
   `rejectedTokenA` 可用；只打开一次 Dialog，并捕获 `user:A`。
3. 上述两条分别完成同账号 A 登录与异账号 B 登录，断言前者
   `sameAuthenticatedScope=true`、后者为 false，返回对象及序列化内容不包含 token。
4. 重复事件和旧 token 事件保持 generation 不变；已有 Reader suspend、overlay reset、
   progress 零写入和安全 `returnTo` 合同继续全量运行。
5. 增加真实浏览器 smoke：在已加载 Reader 中先删除 storage 再派发与拦截器相同的事件；
   1440×900、1024×1366、390×844、360×800 均验证旧正文立即卸载、Dialog 不可误关闭、
   同账号原路重载、异账号返回书架、无旧进度写入和控制台错误。
