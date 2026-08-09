# UserManage 权限部分更新第二轮固定基准合同（P2）

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：`aligned / Docker-published / awaiting-device-verification`。本轮只复审 UserManage 权限开关到 Go 更新接口的字段所有权；
不重开已经签收的用户创建、密码规则、删除、书源 namespace、工作区清理和 WebSocket recipient scope。

## 权威映射

上游：

- `web/src/components/UserManage.vue#toggleUserWebdav`
- `web/src/components/UserManage.vue#toggleUserLocalStore`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt#updateUser`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt#resetPassword`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt#login`

当前：

- `frontend/src/components/overlays/OverlayUserManagement.vue`
- `frontend/src/composables/useOverlayUserManagement.js#updatePermission`
- `frontend/src/api/admin.js#updateUser`
- `backend/api/admin.go#updateUser`
- `backend/api/auth.go#login`
- `backend/models/models.go#User`

## 二次复审矩阵

| 项目 | 固定上游行为 | 当前行为 | 裁决 |
|---|---|---|---|
| 开关请求字段 | WebDAV 开关只发送 `{username,enableWebdav}`；书仓开关只发送 `{username,enableLocalStore}`。 | 任一开关都发送 `canEditSources/canAccessStore/canAccessWebdav/bookLimit/sourceLimit` 的整行快照。 | `must-fix`：每次 UI 动作只拥有自己改变的一个字段。 |
| 服务端写入列 | `updateUser` 只在对应可空请求字段存在时修改那一项，不触碰 password、登录时间或其它账户字段。 | 先读取完整 `models.User`，修改请求字段，再 `Save(&user)`；GORM 会把密码散列、角色、登录时间、创建时间等旧快照一并写回。 | `must-fix`：改成显式 updates map，禁止整行 Save。 |
| 同字段重复点击 | 上游一次 switch change 对应一次请求；界面没有把一次动作扩张成其它字段写入。 | 同一字段快速切换可产生并发整行请求，较早请求后完成时可能成为最终持久值。 | `must-fix technical adaptation`：字段写入期间该开关显示 busy 并拒绝重复提交；其它字段可独立更新。 |
| 失败恢复 | 上游失败提示，但 v-model 已切换且未恢复，是原项目缺陷。 | 当前失败后整表 reload，可恢复权威值，但期间所有字段都在请求内。 | `intentional-safe-fix`：立即撤回当前字段，再网络重载；不得回滚其它成功字段。 |
| 管理员保护 | 上游保护 `default` namespace；OpenReader 保护管理员角色和当前管理员。 | 后端读取目标后拒绝 `role=admin`，前端不显示其 mutable controls。 | `acceptable multi-user adaptation`，保持。 |
| 响应 | 上游返回更新后的完整列表。 | OpenReader `PUT` 返回目标用户；legacy `can_access_webdav=NULL` 时 raw model 可能省略有效 WebDAV 权限。 | `must-fix projection`：路径和单用户响应保持，但必须投影 effective WebDAV 值，不因 nullable 迁移状态省略。 |

## API 合同

`PUT /api/admin/users/:id` 保持现有 JWT 管理员路径和 JSON envelope。请求允许以下字段的任意**非空**
子集：

```json
{
  "bookLimit": 0,
  "sourceLimit": 0,
  "canEditSources": true,
  "canAccessStore": true,
  "canAccessWebdav": true
}
```

1. 每个缺失字段不参与 SQL `SET`；不得通过读取后的零值或旧值写回。
2. `username`、`role`、`password_hash`、`last_active_at`、`created_at` 永远不属于该接口的写入列。
3. `bookLimit/sourceLimit` 保持 `0` 的既有无限制语义并拒绝负数；布尔 `false` 必须可显式持久化。
4. 空对象或只有未知字段为 `400 {error:{code:"BAD_REQUEST",...}}`，不写数据库、不更新
   `updated_at`、不广播。
5. 普通用户成功为 `200`，返回更新后 fresh row；`canAccessWebdav` 使用 nullable 迁移的 effective
   projection。目标不存在 `404`，管理员目标 `403`，非管理员调用者 `403`。
6. 只有 SQL 成功改变合法字段后才发送一次 `users_update`；事件 recipient 继续由
   `websocket-sync-p2-contract.md` 约束。

## 数据与并发边界

- 不新增 SQLite 表、列、索引或迁移，不改 `data/cache/library`、备份或 WebDAV。
- 权限更新与 `POST /api/auth/login`、密码重置并发时，不得覆盖后两者新写的
  `last_active_at/password_hash`。
- 不用全局 UserManage mutex 掩盖整行写入；SQL 列所有权本身必须正确。
- 前端只把当前字段值发送给 API；不能用“后端会忽略”作为继续发送整行快照的理由。
- busy 状态只锁定 `userID + field`，不能阻塞另一个用户或另一个独立权限字段。
- 账号 generation 失效后不显示旧成功/失败提示、不把旧响应合并进新会话。

## 失败测试先行

1. Go API 用 SQLite trigger 在权限 UPDATE 前模拟并发密码与登录时间写入；旧整行 `Save` 必须稳定
   覆盖并使测试失败，显式列更新必须保留并发新值。
2. Go API 验证 `false`/`0` 正确写入、缺失字段不变、负限额和空对象 `400`、失败零广播。
3. Go API 验证 legacy `can_access_webdav=NULL` 只改书仓时，数据库仍为 NULL，响应却返回 effective
   WebDAV 值；不得借投影暗中回填迁移列。
4. 前端 controller 验证三个 switch 各自只发送一个字段；旧五字段 payload 作为红灯。
5. 前端验证同一 `userID + field` pending 时只发一次，另一个字段仍可独立提交；失败只撤回该字段
   并重载，身份切换后不提示或合并。
6. 真实浏览器在 1440×900、390×844、360×800 验证三个开关 payload、字段级 loading、成功和
   失败恢复；移动表格固定列、AddUser、密码、书源默认/重置、删除流程保持。

## 实施与验证结果（2026-08-09）

- Go 更新接口已改为仅包含显式请求列的 `Updates(map)`，负限额和空/未知 patch 返回 `400`；成功后
  重新读取 fresh row，再投影 legacy nullable WebDAV 有效值并广播。
- 前端三个权限开关均只提交自己的单字段 payload；busy 以 `userID + field` 隔离，同字段重复写入被
  拒绝，不同字段可并行；失败立即撤回该字段并重新载入权威数据。
- SQLite trigger 并发回归已证明权限更新不会覆盖较新的密码散列或登录时间；`false`、`0`、缺失字段、
  负限额、空 patch、零广播和 legacy WebDAV NULL 投影均有 API 合同测试。
- focused Go、focused `-race`、`go vet ./...`、全量 Go、frontend 707/707 和生产构建通过。
- UserManage 真实浏览器合同已在 1440×900、1024×1366、390×844、360×800 通过，覆盖单字段
  payload、字段级 loading、独立字段并行和失败恢复。

本切片代码与浏览器闸门已经关闭。实现提交 `77a60d8345ac28be9aa0542bc016ba98dcc89bc0` 已由本机
OrbStack 构建并发布，不使用云端构建：

- `ghcr.io/changshengyu/openreader:77a60d8`
- `ghcr.io/changshengyu/openreader:latest`
- amd64/arm64 OCI index：`sha256:a1a37b223e10a3c43febd23250dd7790394c200d69e7c9548255cf1fdba3b017`
- amd64 manifest：`sha256:45120281cd09162f547ee2574f6b4feca4fcb078c96f1b7b423baaf095369e77`
- arm64 manifest：`sha256:111ea9d2ba2dec923b01f3081f3c4467af5dc69cdcb6035239db9dfde532d25d`

候选镜像通过 fresh volume 的 portable v1/v2 assets、cross-user、restart，以及 historical volume
的 TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation。当前等待真实设备验证开关 busy、并行和
失败恢复的用户可见反馈。
