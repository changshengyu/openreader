# 登录框“记住登录信息” P2 合同

日期：2026-11-12。分支 `claude/account-book-limit-remember-login-a2fb9c`，接在每用户书籍上限切片之后（基于 main `f82e0f5a`）。
固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

来源：注册限制合同（`registration-invite-code-user-limit-fixed-baseline-second-audit-p1-contract.md`）的 O1 项把“记住登录信息”列为另起。
书籍上限是另一个切片（`user-book-limit-fixed-baseline-second-audit-p2-contract.md`）。

## 1. 上游行为（fa22f271）

- 登录对话框 `web/src/App.vue`：
  - 用户名、密码、注册模式下的“邀请码(没有则不填)”之后，是 `<el-checkbox v-model="remember">记住登录信息</el-checkbox>`（L35）。
    它不带 `v-if`，登录和注册两种模式都显示。
  - `data.remember: true`（L209）：每次加载页面都默认勾选。`App` 组件常驻，页面不刷新时保留用户上一次的选择。
- 保存 token（`App.vue#login`，L534–535）：成功后 `if (this.remember && res.data.data && res.data.data.accessToken) this.$store.commit("setToken", accessToken)`。
  - `plugins/vuex.js#setToken`（L238–240）写入 `state.token` 和 `localStorage.api_token`（`cache.js#setCache`）。
  - 页面加载时，`state.token` 从 `localStorage.api_token` 读取（L37）。`plugins/axios.js` 把它作为 `accessToken` 参数随每个请求发送。
  - 不勾选时不调用 `setToken`：既不写 `localStorage`，也不清除已有的 `api_token`。
- 两种登录在服务端的区别：
  - 服务端在两种情况下都签发同一种 token：`BaseController.kt#saveUserSession` 写 `token_map`，有效期 `loginExpireDays = 7` 天，每次使用延长（`#checkAuth` L153）；
  - 同时把用户名放进服务端会话。`RestVerticle.kt` 用 `SessionHandler` 加 `LocalSessionStore`（L33–46）：cookie `reader.session`，会话在内存里，7 天无访问过期，服务重启即失效；
  - `checkAuth`（L116）先认会话中的用户名，再认 `accessToken`。
  - 所以“不记住”就是浏览器不保存长期 token，登录只靠服务端内存会话维持。
- 注销：`Index.vue#logout`（L2732）调用 `/logout` 后 `setToken("")`；`UserController.kt#logout` 销毁会话，并从 `token_map` 中删除请求带来的 token。

## 2. 当前 OpenReader（f82e0f5a）

- `frontend/src/components/AuthForm.vue`（`/login` 页与 `AuthDialog` 共用）：只有用户名、密码，以及注册模式的邀请码，没有复选框。
- `frontend/src/stores/user.js`：
  - `login()` 总是 `localStorage.setItem('openreader_token', token)`，`clearSession()` 删除它；
  - 初始 `token` 读 `localStorage`。
- 服务端只认 `Authorization: Bearer` JWT，没有 cookie 会话（`authsession.SessionTTL` 7 天，使用时续期）。
- 前端在 11 个文件中直接读写 `localStorage.openreader_token`：18 处读，1 处写，4 处删：
  - `api/client.js`：请求拦截器，401 门禁与删除；
  - `api/books.js`：`cacheBookContentStream` 的 token 与 401；
  - `api/sources.js`：`debugSourceStream` 的 token 与 401；
  - `stores/user.js`、`stores/preferences.js#hasAuthToken`、`stores/reader.js`（阅读设置同步 3 处）；
  - `composables/useSync.js`（WebSocket 3 处）、`composables/useReaderProgressPersistence.js#sendKeepAlive`；
  - `utils/authScope.js#storedAuthToken`、`utils/authenticatedOperation.js#currentAuthenticatedIdentity`、`router/index.js` 守卫。
- 重新认证合同（`reader-reauthentication-isolation-p0-contract.md`）依赖：
  - 401 只在被拒的 token 仍是当前 token 时删除它并广播 `openreader:auth-required`；
  - `clearSession` 在清除 token 之前广播会话失效；
  - 显式注销撤销被捕获的 token。
  - `frontend/tests/readerReauthenticationWiring.test.mjs` 用两个静态正则检查 `client.js` 与 `books.js` 中的 `localStorage.getItem('openreader_token')` 比较。
- 应用自己打开的新窗口只有一处：`AppLayout.vue#openSourceDebugWorkspace` 调用 `window.open(href, '_target')`，没有 `noopener`。

## 3. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| M1 | 没有“记住登录信息”复选框 | `must-fix` | 登录与注册都显示，默认勾选（第 4 节） |
| M2 | 不勾选时也把 token 长期保存在 `localStorage` | `must-fix`（共享设备上的隐私问题） | 不勾选只保存到本标签页的会话存储（第 4 节） |
| A1 | 不勾选的登录保存在 `sessionStorage`：刷新仍在，关闭标签页或浏览器后消失。上游靠服务端内存会话（7 天无访问或重启后失效） | `acceptable-change`（JWT 适配：OpenReader 没有 cookie 会话；改用 cookie 需要另做 CSRF 和会话设计，不在本切片） | — |
| A2 | 独立打开的新标签页需要重新登录；从应用里 `window.open` 打开的窗口（书源调试）会复制会话存储，仍是登录状态。上游同一浏览器的标签页共享 cookie | `acceptable-change`（A1 的结果） | — |
| A3 | 不勾选登录时，同时删除 `localStorage` 中旧的 token。上游保留旧的 `api_token`，关闭浏览器后会用它自动登录 | `acceptable-change`（修正上游的隐私缺口：选择不记住后，浏览器里不留任何长期登录） | — |
| C1 | 服务端签发同一种 token，有效期、续期和撤销不变 | 一致 | 不改后端 |
| C2 | 复选框的值在页面生命周期内保留，刷新后恢复默认勾选 | 一致 | — |

## 4. 目标行为

- 存储：`frontend/src/utils/authToken.js` 是唯一读写 token 的地方，两处的键都是 `openreader_token`：
  - `readAuthToken()`：先读 `sessionStorage`（本标签页的不记住登录），再读 `localStorage`。存储不可用或读取抛错时视为空。
  - `writeAuthToken(token, { remember })`：
    - 勾选时写 `localStorage`，并删除本标签页 `sessionStorage` 中的 token；
    - 不勾选时写 `sessionStorage`，并删除 `localStorage` 中的 token（A3）。
  - `discardAuthToken(token)`：只删除值恰好等于该 token 的存储项。401 门禁用它，被拒 token 以外的值不动。
  - `clearAuthToken()`：两处都删除，用于注销与会话失效。和现在一样，注销或失效会让整个浏览器退出登录。
- 读取点：第 2 节列出的所有直接读写都改用这些函数，`authToken.js` 之外不再出现 `openreader_token` 的存储读写。
  - 401 处理的顺序与语义不变：`readAuthToken() !== rejectedToken` 时忽略；否则 `discardAuthToken(rejectedToken)`，再广播；
  - `requireLogin` 与 `clearSession` 的 scope 捕获、generation、Reader 挂起和弹层清理都不变。
- `user` store：
  - 新增 `rememberLogin: true`，不持久化；
  - `login()` 按它写 token；
  - `login` 的签名和 `AuthForm` 的调用不变，复选框绑定这个状态。
- 表单：
  - 用户名、密码（注册模式还有邀请码）之后、提交按钮之前，显示 `<el-checkbox>`，文字为“记住登录信息”；
  - 登录与注册都显示，默认勾选；
  - 在 `/login` 页与 `AuthDialog` 中、1440×900、390×844、360×800 下都不超出视口。
- 不改后端、API、JWT、会话有效期、注销撤销和 `/login` 页其它布局。`Login.vue` 的标题正由另一个会话改名，本切片不改该文件。

## 5. 测试计划（先红后绿）

前端（`node --test`）：
- T1 `utils/authToken.js`：
  - 读取时 `sessionStorage` 优先；
  - 勾选写入：`localStorage` 有 token，`sessionStorage` 没有；
  - 不勾选写入：只有 `sessionStorage`，旧的 `localStorage` token 被删除；
  - `discardAuthToken` 只删除值相同的那一处；`clearAuthToken` 两处都删除；
  - 没有存储时不抛错。
- T2 `user` store：
  - `rememberLogin` 默认 true；
  - 为 false 时登录只写 `sessionStorage`，为 true 时只写 `localStorage`；
  - 初始 `token` 读本标签页的会话 token；
  - `clearSession` 两处都清除。
- T3 拦截器：
  - 不记住登录的 token 也会放进 `Authorization`；
  - 它被 401 拒绝时会删除并广播；
  - 过期的 401（当前 token 已不同）不删除也不广播。
- T4 静态：
  - `src/` 中只有 `utils/authToken.js` 直接读写 `openreader_token`；
  - `AuthForm.vue` 的复选框 `v-model="user.rememberLogin"`，文字为“记住登录信息”，位置在密码（与邀请码）之后、提交按钮之前，不带 `v-if`。
- T5 `readerReauthenticationWiring.test.mjs` 的两个正则改为检查 `readAuthToken() !== rejectedToken` 与 `readAuthToken() === token`。性质不变，只是读取点统一了；同时由 T3 的运行时检查补强。

浏览器：真实 Go 服务、全新数据目录，在 1440×900、390×844、360×800 下：
- 默认勾选：token 只在 `localStorage`；刷新和新标签页都保持登录。
- 不勾选：token 只在 `sessionStorage`；刷新保持登录；独立的新标签页回到 `/login`；带上 `storageState` 新建上下文（模拟重开浏览器）回到 `/login`；`window.open` 打开的窗口仍是登录状态。
- 注销后两处都没有 token。
- 不记住的会话在服务端被撤销后：出现重新登录对话框；它的复选框保持未勾选；重新登录后新 token 只在 `sessionStorage`。
- 复选框在 `/login` 页与对话框中都在视口内。

另外，`registration-limits-contract.mjs` 与 `reader-reauthentication-contract.mjs` 仍须通过；后者的默认勾选路径继续断言 `localStorage`。

## 6. 明确不重开

- 重新认证与账号切换隔离、JWT 与会话策略、注册、邀请码、用户数上限、`/login` 页标题与布局。
- 书籍上限（另一切片）。

## 7. 状态

**contract-only**。
