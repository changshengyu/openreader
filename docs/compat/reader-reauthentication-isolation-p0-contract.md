# P0 Reader 登录失效与账号切换隔离合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-27 完成固定上游与当前实现取证；本文件是实施前合同。按照
`readerdev-compat-inventory` 门禁，本阶段只记录行为、差异和测试，不修改应用代码。

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
| 401 通知 | `frontend/src/api/client.js` 只在非登录请求携带的当前 token 被 `401` 拒绝时移除 localStorage token，并发送带 `rejectedToken` 的 `openreader:auth-required`。 | **aligned / hardened**：保留 rejected-token 防止旧 401 清除新登录。 |
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
