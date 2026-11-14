# 其余 handler 的请求取消合同（P2）

状态：**contract-only**（2026-11-14）。

基于 main `0584ae7`（已含设置写入的取消修复 `65f85c5` 与全局 GORM 日志器 `backend/db/logger.go`），分支
`claude/request-cancellation-remaining-handlers`。来源是
[`user-setting-write-request-cancellation-p2-contract.md`](user-setting-write-request-cancellation-p2-contract.md)
第 3.3 节盘点的 C2–C8（C1 已在该切片修复）。

固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 的 vert.x handler 不观察客户端断开：
登录、登出（`UserController.kt#logout` 销毁 session 并删除 `token_map` 项）、书源与用户管理、搜索都会做完，再往已关闭的
连接写响应，没有“请求取消”这一状态。所以本合同和设置切片一样没有上游行为可对齐，全部差异归为**运行时适配**
（Go 请求 context 贯穿 GORM、远程抓取和文件操作之后才出现的取消路径）。

## 1. 规则

沿用设置切片（其第 5 节与第 3.1 节的既有约定）：

- **R1 提交前取消**：不写库、不发事件、不写响应体、不返回 5xx。
- **R2 提交后取消**：写入已持久化，之后的读回、派生文件和广播改用 `context.WithoutCancel(ctx)`（保留 context 值，去掉取消），
  照常写成功响应（连接已断开，响应被丢弃）。
- **R3 鉴权与权限判断被取消**：不得写 401、403 或 5xx，也不写响应体。判断已经完成（查询成功）后得出的 401/403 不受影响。
- **R4 访问日志**：格式不变。没有写响应的请求按 Gin 默认记 200（设置合同 D5），不新增 499。

“请求已结束”统一用 `c.Request.Context().Err() != nil` 判断，不看错误链，原因有两个：

1. GORM 把失败语句和回滚错误拼成 `"%v; %w"`，事务内取消得到的错误链里只有 `sql.ErrTxDone`，没有 `context.Canceled`
   （设置合同第 2 节）。
2. 单源搜索有自己的 30 秒派生超时。请求仍在时的超时是真实失败，必须保持 502 和失败缓存；只看错误链会把它误当成取消。

`database/sql` 的 `Tx.Commit` 先检查 context，已结束就不提交；mattn 的 `COMMIT` 失败会回滚。所以写语句（GORM 隐式事务或
显式 `Transaction`）返回错误且请求已结束时，这次写入一定没有持久化，按 R1 处理；返回 nil 时已持久化，按 R2 处理。

## 2. 盘点方法

在 `0584ae7` 上逐个核对第 3.3 节的行号，再沿每个入口的调用链找绑定请求 context 的数据库、远程和文件调用，以及取消后的
映射。因为有 R3，权限门的 401/403 也一并核对（设置切片的扫描只统计 5xx）：

- 绑定请求 context 的权限门：`requireAdmin`（`admin.go:31`，9 个入口）、`requireSourceEdit`（`sources.go:1046`，10 个入口）、
  同步升级的 `Authenticate`（`sync.go:19`）、登出的 `Revoke`（`auth.go:292`）。
- 不绑定请求 context、不在本合同范围：登录的用户查询与 `last_active_at` 更新、注册的建用户事务（都用 `s.db`）、
  `requireStorageAccess`（LocalStore/WebDAV 的权限门）、`s.bookSources.*`、`cleanupInactiveUsers` 的候选查询、
  `listUsers`、`createUser`、`updateUser` 的写入、LocalStore 删除（`service.Remove`）、`service.Stat`。
- 认证中间件 `middleware/auth.go:94` 已在取消时 `c.Abort()`（设置合同 3.1）；被测 handler 的取消点都在中间件之后。

## 3. 逐项：当前行为、目标与分类

### C2 登录、注册发 token（`auth.go:268-273 respondWithToken`）

- 当前：`sessions.Issue(ctx)` 在一个事务里读 `auth_version`、删除过期会话、插入新会话、裁剪到 64 个。context 在提交前结束
  （`BEGIN` 前或事务中）时返回 `persist session: context canceled`（或 `sql.ErrTxDone`），handler 返回
  500 `{"error":"failed to create token"}`；会话行没有落库。提交之后只剩本地 JWT 签名，不再绑定 context，正常 200。
- 注册的用户行由不带请求 context 的 `s.db.Transaction` 提交，取消时仍已创建；登录的 `last_active_at` 同样不带 context。
  客户端之后可以直接登录；这与“提交后连接断开”是同一种结果，本合同不改。
- 目标：Issue 失败且请求已结束 → R1，不写响应，不存在会话行。真实失败（请求仍在，例如事务里的数据库错误）仍为
  500 `failed to create token`。提交后取消不变（200）。
- 分类：**must-fix**（运行时适配）。
- 不改：签名失败时的补偿删除（`authsession/service.go:125`）绑定请求 context。HS256 对 `[]byte` 密钥实际不会失败，
  且位于 `services/`，不在本切片范围。

### C3 登出（`auth.go:286-301`）

- 当前：`Revoke(ctx)` 是一条 DELETE（legacy 会话为 UPDATE），GORM 隐式事务，`_txlock=immediate` 让它以 `BEGIN IMMEDIATE`
  开始。提交前取消 → 500 `{"error":"failed to revoke session"}`，会话未吊销；提交后取消 → 204，已吊销。
  错误分支先判 `ErrInvalidSession`（401）再判其它（500）。
- 目标：错误分支先判请求是否结束：结束 → 不写响应（R1/R3）；否则照旧 401/500。提交后取消照旧 204。
- 分类：**must-fix**。
- **D12 待决定（unknown，不在本切片实现）**：按 R1，提交前被取消的登出不吊销会话，这与当前持久化结果相同（不是回退）。
  但上游登出不受断开影响，前端 `stores/user.js#logout` 发出请求后不等待就清掉本地 token，被取消的登出会让服务端会话保持
  有效直到过期（7 天，期间不续期）。若要“断开也完成登出”，还要处理认证中间件：中间件里取消时 `c.Abort()`，handler
  根本不执行，只改 handler 只能覆盖一部分窗口。建议另开切片决定。

### C4 同步 WebSocket 升级（`sync.go:17-38`）

- 当前：`Authenticate(ctx)` 读用户、读会话，距上次续期满一分钟时 UPDATE 续期。任一步取消 → 500
  `{"error":"failed to authenticate sync connection"}`。错误分支先判 `ErrInvalidSession`（401）。
- 目标：与 `middleware/auth.go:94` 相同，错误分支先判请求是否结束：结束 → 不写响应（R3）；否则照旧 401/500。
  续期 UPDATE 提交后取消：`Authenticate` 返回 nil，`Upgrade` 在已断开的连接上失败后返回，不变。
- 分类：**must-fix**。

### C5 管理员用户与书源操作（`admin.go`）

| 位置 | 路由 | 取消时 |
|---|---|---|
| `:31-33 requireAdmin` | 9 个入口：`GET/POST /api/admin/users`、`PUT /api/admin/users/:id`、`PUT /api/admin/users/:id/password`、`POST /api/admin/users/:id/sources/default`、`POST /api/admin/users/sources/reset`、`POST /api/admin/users/batch-delete`、`POST /api/admin/cleanup-inactive`、`POST /api/sources/default/save` | 查询错误一律 403 `FORBIDDEN admin access required`：**R3 误判** |
| `:107-113` | 设为默认书源：目标用户查询 | 500 `failed to load user` |
| `:114-122` | 设为默认书源：保存（含提交后的镜像，见 C6-M） | 500 `failed to save default sources` |
| `:165-168` | 重置用户书源：目标计数 | 500 `failed to load users` |
| `:176-183` | 重置用户书源：默认初始化 | 500 `failed to load default sources` |
| `:185-192` | 重置用户书源：重置事务 | 500 `failed to reset user sources` |
| `:439-453` | 重置密码：改哈希、`auth_version+1`、删会话同一事务 | 500 `failed to reset password` |
| `:639-646` | 删除用户：`deleteUserData` 事务 | 500 `failed to delete users` |
| `:672-675` | 清理不活跃用户：`deleteUserData` 事务 | 500 `cleanup failed` |

- 这些写入取消时都已回滚：密码与会话不变、用户与其数据不删、不清工作区文件、不发 `users_update`/`sources_update`。
  提交后的 `broadcastUsersUpdate`、`cleanupDeletedUserCoverImages`、`cleanupUserWorkspaces` 本来就不绑定请求 context。
- 目标：`requireAdmin` 查询失败且请求已结束 → 不写响应、返回 false（R3）；查询成功但不是管理员仍 403；查询的其它错误
  仍 403（原有行为）。其余各处：请求已结束 → R1；404、409、400 等真实结果不变。
- 分类：**must-fix**。

### C6 书源（`sources.go`）

| 位置 | 路由 | 取消时 |
|---|---|---|
| `:1046-1053 requireSourceEdit` | 10 个入口：书源新建、修改、删除、清空、批量、导入、远程导入与预览、默认保存与恢复 | 500 `failed to load user` |
| `:610-618` | `GET /api/sources/default` | 500 `failed to load default sources` |
| `:631-635` | `POST /api/sources/default/save` | 500 `failed to save default sources` |
| `:676-683` | `POST /api/sources/default/restore`：默认初始化 | 500 `failed to load default sources` |
| `:685-692` | 同上：恢复事务 | 500 `failed to restore default sources` |

- `requireSourceEdit`：错误分支先判请求是否结束：结束 → 不写响应、返回 false（R3）；`record not found` 仍 401，
  其它错误仍 500，`can_edit_sources=false` 仍 403。
- 恢复是 `DefaultStatus` 读、`EnsureNamespace`（独立提交，只把默认投影物化成关联，可见列表不变）和恢复事务三步。
  恢复事务前取消：物化可能已提交，恢复未做，不广播，不写响应（R1）。
- **C6-M 默认快照的提交后步骤（R2）**：
  - `saveDefaultSourceSnapshotContext`（`:643-665`，管理员“设为默认”与“保存默认”共用）在保存事务提交后，用请求 context
    读回默认书源（`ListExistingActive(0)`）并写 `data/defaultBookSources.json`。
  - `ensureDefaultBookSourceNamespaceLocked`（`:995-1024`）未配置分支在 `ReplaceActive(0)` 提交后，用请求 context 读回
    （`DefaultStatus`）并写镜像。
  - 这两处提交后取消时，当前返回 500，SQLite 已是新默认，镜像还是旧内容，要等下一次 `GET /api/sources/default` 或重启修复
    （[`default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md`](default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md)
    第 4.1 节的恢复路径）。
  - 目标：提交后的读回与镜像写入改用 `context.WithoutCancel(ctx)`，与该合同第 4.1 节“完成响应前 SQLite 与镜像应对应同一
    已提交快照”一致，并照常返回成功响应。提交前的 namespace 检查、legacy 文件读取和事务仍用请求 context（该合同第 3 节
    与第 6 节第 5 条不变）。已配置分支对镜像的规范化重写不伴随新提交，取消时照旧中止，按 R1 不写响应。
- 分类：**must-fix**（5xx 与 R2）。

### C7 单书源分页搜索（`search.go:101-106`）

- 当前：远程请求绑定请求 context。取消时 `recordSourceFailure` 因 `errors.Is(err, context.Canceled)` 不写失败缓存，
  随后 502 `{"error":"failed to search source", ...}`。
- 目标：先判请求是否结束：结束 → 不记失败、不写响应（R1）。请求仍在时的 30 秒超时（派生 context）仍为 502
  `search timeout` 并照旧记失败缓存；其它远程失败仍为 502 并记失败缓存。
- 多源搜索在取消时返回已收集的部分结果 200，不是 5xx，不在范围。
- 分类：**must-fix**。

### C8 LocalStore 管理操作（`localstore_boundary.go`、`localstore.go`）

| 位置 | 操作 | 取消时 |
|---|---|---|
| `localstore_boundary.go:242-245 localStoreFileService` | 根初始化 `EnsureRootContext`：列表、上传、下载、新建目录、改名、删除 6 个入口 | 500 `failed to create local store` |
| `localstore.go:55-62`、`:75-78`、`:91-94` | 列表：`AdmitRead`、`ListLocal`、`Validate` | 500 `failed to read local store` |
| `localstore.go:133-137` | 上传：建目标目录 `MkdirContext` | 500 `failed to create directory` |
| `localstore.go:140-157` | 上传：`Put`（逐文件替换） | 500 `failed to save file` |
| `localstore.go:212-224` | 下载：`OpenContext` | 500 `failed to download local store item` |
| `localstore.go:259-265`、`:280-287` | 新建目录：父目录与目标 `MkdirContext` | 500 `failed to create parent directory` / `failed to create directory` |
| `localstore.go:325-339` | 改名：`MoveContext` | 409 `failed to rename local store item`：把取消当成冲突 |

- `EnsureRootContext`、`MkdirContext`、`MoveContext` 开始前检查 context，`Put` 与读路径逐块检查，取消时不建目录、不写文件、
  不改名；多文件上传在后续文件取消时，已写完的文件保留（逐文件替换、不回滚，与现状相同）。下载的响应写出已在
  `serveOpenedStoreFile` 里观察 context（设置合同 3.1）。
- 目标：上表各处请求已结束 → 不写响应（R1）。400、404、409、413 等真实结果不变；列表普通文件仍为原有 500。
- store-read 合同（`TestStoreReadCancelledAfterRootInitializationReturnsNoSuccessfulData`）只要求取消时不返回成功数据，R1 满足它。
- 导入入口 `POST /api/local-store/import-preview` 与 `POST /api/local-store/import` 共用根初始化，但它们的取消响应由导入合同
  固定为 500（设置合同 3.2，不重开）。实现时给导入单独的入口函数，保留它们在根初始化取消时的 500。
- 分类：**must-fix**（5xx 与 409 误判）。

## 4. 差异分类

| 编号 | 差异 | 分类 |
|---|---|---|
| D1 | C2 发 token 取消时返回 500 | **must-fix**（运行时适配） |
| D2 | C3 登出取消时返回 500 | **must-fix** |
| D3 | C4 同步升级认证取消时返回 500 | **must-fix** |
| D4 | `requireAdmin` 把取消判成 403 | **must-fix**（R3） |
| D5 | C5 管理员操作取消时返回 500 | **must-fix** |
| D6 | `requireSourceEdit` 取消时返回 500 | **must-fix**（R3） |
| D7 | C6 默认书源状态、保存、恢复取消时返回 500 | **must-fix** |
| D8 | C6-M 提交后取消：镜像与 SQLite 不一致，并返回 500 | **must-fix**（R2） |
| D9 | C7 单源搜索取消时返回 502 | **must-fix** |
| D10 | C8 LocalStore 管理操作取消时返回 500 | **must-fix** |
| D11 | C8 改名把取消判成 409 | **must-fix** |
| D12 | 提交前被取消的登出不吊销会话（第 3 节 C3） | **unknown / 待决定**，另开切片 |
| D13 | 取消且未写响应的请求在访问日志里记为 200 | **acceptable-change**（与设置合同 D5 相同） |
| D14 | 导入入口在根初始化取消时仍返回 500 | **不重开**（导入合同固定的取消 500） |

## 5. 实现约束

- 新增一个判断 `requestContextEnded(c)`（`c.Request.Context().Err() != nil`），只放在上述错误分支里；400/404/409/413 等
  真实结果的映射顺序不变，只有 R3 的几处把“请求已结束”放到 401/403 判断之前。
- C6-M 只在提交成功之后切换到 `context.WithoutCancel`；事务本身、提交前的读取与文件读取仍用请求 context。
- 不改事务语义：不加或拆事务，不改 `_txlock=immediate`、WAL、连接池、`busy_timeout`；事件仍只在持久写入成功后发出。
- 不改 `backend/services/*`、`backend/middleware/*`、`go.mod`、前端、访问日志格式、认证写节流与 GORM 日志器。

## 6. 计划测试（先红后绿）

新增 `backend/api/request_cancellation_remaining_handlers_contract_test.go`。每个用例用新服务器，把服务器的 GORM logger 换成
写入缓冲区的生产 logger（`db.NewLogger`），订阅相关用户的事件；取消点用 GORM 回调（按表名、`Select` 列、`Dest` 类型与是否
在事务内匹配，只在被测请求里触发一次，用例先断言回调确实触发）、现有的 `beforeStoreReadTestHook`，以及红测提交新增的
nil-in-production 观察接缝 `beforeLocalStoreWriteTestHook`（上传、新建目录、改名在根初始化之后、写入之前触发，不加任何检查）。

红测（在 `0584ae7` 上失败）：

1. C2：登录与注册各两处（事务开始前、事务内插入会话前）：不是 500、无响应体、会话行数不变；注册的用户行存在。
2. C3：登出在 DELETE 开始前与执行前取消：不是 401/500、无响应体、会话仍有效。
3. C4：同步升级在用户查询与续期 UPDATE 时取消：不是 401/500、无响应体。
4. C5：`requireAdmin` 取消（经 `GET /api/admin/users` 与 `POST /api/admin/cleanup-inactive`）不是 403；设为默认书源的目标查询与
   保存事务、重置用户书源的计数与重置事务、重置密码事务、删除用户事务、清理不活跃用户事务：不是 500、无响应体、数据不变、
   无事件。
5. C6：`requireSourceEdit` 取消（经新建书源）不是 500 且不建书源；默认状态、保存事务、恢复事务取消：不是 500、无响应体、
   无事件、用户书源不变。
6. C6-M：保存与设为默认在提交后、读回镜像前取消：不是 500，SQLite 默认与 `defaultBookSources.json` 都是新快照；未配置分支在
   `ReplaceActive(0)` 提交后取消：不是 500，SQLite 已配置，镜像与 SQLite 一致。
7. C7：远程请求中取消：不是 502、无响应体、无失败缓存行。
8. C8：6 个管理入口在根初始化取消；列表、下载在读取前取消；上传（有目录、无目录）、新建目录（有父目录、无父目录）、改名在
   写入前取消：不是 5xx/409、无响应体、不建目录、不写文件、不改名。

对照（改前改后都通过）：

- 正常请求：登录/注册 200、登出 204、同步握手成功（真实 HTTP 服务，端口测试需关沙箱）、管理员与书源操作原有成功响应、
  单源搜索 200、LocalStore 列表/上传/下载/新建目录/改名成功。
- 真实错误保持原状态码：事务里注入非 context 错误时 Issue 500 `failed to create token`、Revoke 500 `failed to revoke session`、
  同步认证 500；无效 token 401；非管理员 403；`can_edit_sources=false` 403；请求仍在时单源搜索远程失败 502 且记失败缓存；
  LocalStore 已存在目录 409、缺失路径 404、列表普通文件 500；导入预览在根初始化取消仍为 500（D14）。
- 提交后取消：登出提交后取消仍 204 且会话已吊销。
- 日志：被取消的请求没有 `context canceled` 或 `transaction has already been committed or rolled back` 行；注入的真实错误
  仍按调用位置打印。

## 7. 不重开

- 设置写入合同与设置取消合同的全部条款；设置合同 3.1 已处理的 handler；3.2 导入流程固定的取消 500 与“不新增 499”。
- 默认快照合同的串行化、文件边界、迁移与恢复路径（C6-M 只把提交后的两步改为不随客户端取消）。
- 认证中间件、认证写节流、会话 TTL/续期规则、JWT 格式；D12 留待另开切片。
- `_txlock=immediate`、WAL、连接池、`busy_timeout`、访问日志格式、GORM 日志器。

## 8. 回归门

- `cd backend && go test ./... && go vet ./...`；`go test -race -timeout 40m ./api ./db ./services/...`（端口测试需关闭沙箱）。
- 真实 Go 服务、全新数据目录（`OPENREADER_INVITE_CODE=smoke-invite OPENREADER_USER_LIMIT=3`）：
  `registration-limits-contract.mjs` 多次运行且服务日志没有 5xx；`reader-reauthentication-contract.mjs`、
  `authenticated-session-lifecycle-real-api-contract.mjs`、`user-management-contract.mjs`、`source-workspace-contract.mjs`、
  `store-read-list-lifecycle-contract.mjs`、`local-store-directory-lifecycle-contract.mjs`、`openreader-smoke.mjs`。
- 前端未改动，跳过前端单元门与三视口截图；浏览器合同使用同一份前端构建。
