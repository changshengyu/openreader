# P2 WebSocket 同步协议、方向与账号隔离合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-08-09 已完成固定基准、当前实现、前端调用点和既有专项合同取证，并按测试先行实施；
`implemented / regression-validated / Docker-pending`。合同提交 `7953bc6`、失败测试提交 `598b00b`
均先于应用实现。

## 范围与权威证据

固定上游没有 WebSocket。书架、设置、进度、用户管理和书源动作都由当前页面完成 REST 请求后
直接重载或更新本地 Vuex 状态；不存在浏览器上传一个未经业务控制器验证的“同步事件”并让服务器
替它转发的产品动作。因此 OpenReader 的 `/ws/sync` 是 JWT 多用户、多标签运行环境所需的
`technical-stack-equivalent`，只能传播已经由服务端成功持久化的结果，不能成为第二套写 API。

上游证据：

- `web/src/App.vue`、`web/src/views/Index.vue`、`web/src/views/Reader.vue`；
- `web/src/plugins/vuex.js`；
- `src/main/java/com/htmake/reader/api/controller/*Controller.kt`。

当前证据：

- `backend/api/sync.go`、`backend/sync/hub.go`；
- `backend/api/{books,progress,categories,book_groups,settings,sources,bookmarks,rss,replace_rules,admin}.go`；
- `frontend/src/composables/useSync.js`、`frontend/src/App.vue`、`frontend/src/layouts/AppLayout.vue`；
- `frontend/tests/authenticatedRuntimeScope.test.mjs` 与多客户端浏览器 smoke。

仓库全历史搜索确认：`useSync().send()` 从初始提交起从未有调用者。当前后端却会解析任意客户端
`{type,payload}` 并原样转发给同账号其它客户端；这绕过 REST 参数、所有权、事务和“提交后广播”
合同，属于错误重构，不是兼容能力。

## 差异矩阵

| 契约层 | 当前行为 | 裁决 |
|---|---|---|
| 握手 Origin | `websocket.Upgrader.CheckOrigin` 无条件返回 `true`，任意浏览器 Origin 都可尝试升级。 | **must-fix**：同源浏览器握手；无 Origin 的非浏览器兼容客户端可继续使用。不能用普通 HTTP CORS 反射逻辑放宽 WebSocket。 |
| 握手身份 | `GET /ws/sync?token=<jwt>` 解析 token 并取得 user ID，但不确认用户行仍存在。 | **must-fix**：缺失、无效或已删除用户统一 `401 {"error":"invalid token"}`；有效当前用户才可升级。保留 query token 作为浏览器 WebSocket API 适配，并继续整段日志脱敏。 |
| 事件方向 | 浏览器虽然从不发送，但服务端接受任意文本事件并向同账号转发。 | **must-fix**：协议改为严格 server→client。普通客户端 text/binary application message 以 policy violation 关闭发送方，绝不广播、写库或影响其它连接；超过输入上限的 frame 可由 WebSocket 层先以 message-too-big 关闭。读循环只为处理 close/ping/pong 控制帧保留。 |
| 输入边界 | `ReadMessage` 会完整分配客户端 payload，未设置消息大小边界。 | **must-fix**：在读取前设置很小的 application-message 上限；由于协议禁止客户端数据，首个 application frame 即关闭，不执行 JSON 分配/解析。 |
| 普通业务事件 | 书架、进度、分类、设置、书源、书签、RSS、替换规则均按 `userID` 调用 `Broadcast`。 | **aligned / must-preserve**：事件只能在对应 durable mutation 成功后发送；失败、回滚、冲突输家不得发送。 |
| 用户管理事件 | `users_update` 使用 `BroadcastAll`，全部账号都收到完整 `userIds`。 | **must-fix**：所有管理员收到完整变更集合以刷新管理器；每个受影响普通用户只收到包含自己 ID 的事件以刷新 profile，删除/清理时触发退出；无关普通用户收不到事件或其它账号 ID。 |
| 慢连接 | 发送队列满时移除并关闭客户端，前端重连后强制 REST 校准。 | **aligned / must-preserve**：不静默丢状态事件后继续显示健康连接。 |
| 前端连接代际 | socket/token/scope/generation 全部匹配才允许旧回调提交。 | **aligned / must-preserve**：账号切换后旧 socket 的 open/close/error/message 都不得影响新账号。 |
| 权威状态 | WebSocket 无 replay、revision 或持久队列；重连/前台恢复会请求 REST。 | **acceptable runtime adaptation**：WebSocket 是低延迟提示，不是数据权威；REST/SQLite 仍决定最终状态。 |

## 线协议

### 握手

| 字段 | 合同 |
|---|---|
| Method / path | `GET /ws/sync?token=<URL-encoded JWT>`；不新增第二路径。 |
| Auth | token 必须签名有效、包含非零 user ID，且对应当前用户行存在。 |
| Origin | 浏览器 `Origin` host 必须与请求 `Host` 相同；同源反向代理必须保留外部 Host。没有 Origin 的 CLI/诊断连接允许。 |
| Success | HTTP `101 Switching Protocols`，连接登记到唯一 user ID 的 Hub 集合。 |
| Missing/invalid/deleted identity | `401 {"error":"invalid token"}`，不得升级或登记连接。 |
| Cross-origin browser | `403`，不得升级；不得因为 `OPENREADER_CORS_ORIGIN` 为空或反射 Origin 而放行。 |
| Logging | `/ws/sync` 的整个 query 继续输出为 `/ws/sync?<redacted>`；JWT 不进入应用错误、事件、浏览器持久缓存或测试输出。 |

### 服务端事件信封

所有业务消息继续使用 JSON：

```json
{"type":"event_name","payload":{}}
```

事件族保持现有消费者兼容：

| Type | 范围 / payload 规则 |
|---|---|
| `bookshelf_update`, `bookshelf_delete` | 当前用户；payload 是既有 shelf projection、数组、删除 `id/ids`，或缺失 payload 触发 REST 刷新。 |
| `progress_update` | 当前用户；现有 progress、`clientId` 与可选 shelf-book projection 不变。 |
| `category_update`, `category_delete`, `categories_update`, `book_groups_update` | 当前用户；保留增量或缺失 payload 的刷新兼容。 |
| `settings_update` | 当前用户；`key` 与 `updatedAt` 不变。 |
| `sources_update`, `replace_rules_update`, `rss_update`, `bookmarks_update` | 当前用户；保留现有 `kind` 及专项字段。 |
| `users_update` | 管理员收到完整 `{kind,userIds}`；受影响普通用户只收到 `{kind,userIds:[self]}`；无关用户不收到。 |

客户端不得上传上述或任何其它 type。所有写动作仍只能经过已有 authenticated REST API。

## 状态、数据和部署兼容

- 不新增或迁移 SQLite 表、列、索引；不修改 `data/`、`cache/`、`library/`、备份或 WebDAV。
- 不改变任何业务 REST 路径、body、成功响应、错误 envelope 或持久副作用。
- 不改变服务端事件 type 和既有业务 payload；只收紧事件生产方向和接收账号集合。
- 删除未使用的前端 `send()` 不会移除产品动作或历史调用者；仓库全历史没有调用点。
- 旧页面与新服务并存期间，旧页面也只监听事件；连接继续可用。若第三方曾私自上传事件，该行为
  从未形成文档/测试/API 合同，将按 policy violation 明确失败。
- 同源反向代理必须转发外部 `Host` 与 WebSocket Upgrade；这是部署协议要求，不引入新的环境变量。

## 测试先行闸门

1. Go 真实 WebSocket：缺失/无效 token 为 401；有效 token 但用户已删除也为 401；Hub 不登记。
2. Go 真实 WebSocket：同源 Origin 和无 Origin 可 101；不同 host Origin 为 403，即使 token 有效。
3. Go 双连接：客户端 A 上传伪造 `bookshelf_delete` 后收到 policy close；超过 1 KiB 的 application
   frame 收到 message-too-big；同账号 B 不收到伪造事件且仍可接收后续服务端合法广播；另一个用户
   始终收不到。
4. Go 用户管理事件：管理员收到完整变更 ID；目标普通用户只收到自己的 ID 并可在删除后退出；
   无关普通用户在短有界窗口内收不到事件。失败/回滚动作不发送。
5. 前端静态/运行时合同：`useSync` 不再暴露或调用 `send`；所有既有事件 consumer、连接代际、
   重连强刷与日志脱敏继续通过。
6. 全量 Go、Go race（至少 `./sync ./api`）、frontend tests/build；真实两个浏览器上下文验证同账号
   服务端更新收敛和账号切换。无持久数据变化，因此 Docker 发布前仍运行既有 fresh/historical
   volume/backup smoke，但不新增迁移 fixture。

## 不授权的变化

- 不借本切片更换 JWT secret、token 格式或登录流程；不恢复此前 `invalid token` 回归。
- 不把 query token 改成普通 URL 参数以外的未兼容协议，也不把 token 写 Cookie/local DB。
- 不允许客户端事件 type allowlist 取代 server-only 方向；即使 type 合法也不能绕过 REST。
- 不把 WebSocket 变成书架/进度的唯一权威，不删除前台/重连 REST 校准。
- 不使用 `BroadcastAll` 发送私有书源、书架、设置、进度或完整用户 ID 集合。

## 实施与验证记录

- `websocket.Upgrader` 恢复 Gorilla 的 safe default Origin 检查并增加 5 秒握手上限；token 解析后还会
  验证当前用户行。缺失、无效、已删除账号均在登记 Hub 前返回同一 401。
- Hub 删除任意账号全局广播能力。读泵只处理控制帧；普通 text/binary application frame 收到
  1008 policy close，超过 1 KiB 的 frame 在分配前收到 1009 message-too-big，不再 JSON 解析或 relay。
  写泵增加 10 秒写 deadline，
  读/写任一结束都幂等移除并关闭客户端。
- 前端删除从初始提交起无人调用的 `send()`；socket generation、重连和所有服务端 consumer 不变。
- `users_update` 先查询当前管理员并向其发送完整 ID 集合，再给每个受影响普通用户发送 self-only
  projection；删除后的目标连接仍能收到退出提示，无关账号不再收到事件。
- 新增真实 WebSocket Go 合同先在旧实现稳定复现 cross-Origin 101、deleted-user 101、伪造事件 relay
  和全局用户 ID 四项失败，实施后全部通过；缺失 token、同源、Origin-less、合法同用户广播和
  跨用户静默也有断言。
- 全量 Go、frontend **706/706**、production build、`go test -race ./sync ./api`（API race
  414.677 秒）通过。真实 Go/SQLite/Chrome 双客户端在 1440×900、390×844、360×800 均通过
  network-first 冷启动与实时同账号导入收敛。
- 多客户端 smoke 原来等待已经从上游对齐 UI 删除的“同步在线”文案；本轮改为 init-script 包装
  浏览器原生 WebSocket、只记录 attempts/open/close/error 数字且不记录 URL/JWT，以真实 open
  作为传输门。该测试修复不重新暴露产品 UI。
- Docker fresh/historical volume 与 portable backup 门已在备份事务 worker 修复后全部通过；
  `go vet ./...` 发现的 `Server`/Mutex 浅拷贝也已改为显式 transaction worker 并通过专项 race。
- WebSocket 实现已包含在本机构建并发布的 `ghcr.io/changshengyu/openreader:2ea6e8c` 与
  `latest`；两个标签均为 amd64/arm64 OCI index
  `sha256:678b019c34ac1f92a38dbd650de48867002ae6425a4206aff2e8f315e189d6ac`。状态为
  `implemented / regression-validated / Docker-published`。
