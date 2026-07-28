# P2 用户管理上游复审合同

状态：2026-07-28 重新取证并完成本轮修复。账户规则、独立 WebDAV/书仓权限、安全删除和
user/default 书源所有权已经实施；书源所有权的最终合同见
[`book-source-ownership-p2-contract.md`](book-source-ownership-p2-contract.md)。本次复审
确认的用户管理可见结构和“上次登录”差距已由 `f44447f` 关闭；旧移动卡片、合并权限列及
`lastActiveAt` 旧测试均已删除或重写。本合同以固定基准
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 为准。

上游权威文件：

- `web/src/components/UserManage.vue`
- `web/src/components/AddUser.vue`
- `src/main/java/com/htmake/reader/api/controller/BaseController.kt`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt`
- `src/main/java/com/htmake/reader/api/controller/BookSourceController.kt`

当前映射：

- `frontend/src/components/overlays/OverlayUserManagement.vue`
- `frontend/src/composables/useOverlayUserManagement.js`
- `frontend/src/api/admin.js`
- `backend/api/admin.go`
- `backend/api/helpers.go` 的 storage root/access 边界

## 上游动作与当前差距

| 动作 | reader-dev 可见/持久语义 | 当前 OpenReader | 判定 |
|---|---|---|---|
| 打开管理器 | Index 内单一“用户管理” Dialog；打开清空选择，移动端全屏。 | 根 Overlay Dialog，紧凑端全屏，打开加载并关闭时重置。 | **技术栈等价**；必须保留单一根 Dialog，不重新变成路由或抽屉。 |
| 新增用户 | Dialog 标题区右侧只有“新增”；AddUser 是独立根 Dialog，用户名/密码表单和取消/确定底栏。 | 已恢复标题区“新增”和正文表格起点，删除重复标题/刷新；AddUser 仍是独立根 Dialog，额外提供三项初始权限。 | **已实现 + allowed extension**：初始权限是 Go 多用户扩展，不改变普通用户角色或默认值。 |
| 受保护用户 | `default` namespace 不可选择、不可改 WebDAV/书仓；其他用户可选。 | `admin` 与当前登录管理员不可删除；`admin` 不可修改/重置。 | **允许的多用户安全适配**：管理员不是可删除的默认 namespace；不能放松为可删/可改。 |
| 新建用户名 | 管理员创建用户至少 5 位，仅允许 ASCII 字母/数字，且保留 `default` namespace。 | 共享后端校验覆盖注册与管理员创建；UI 同步提示。旧用户名不重写、不锁定。 | **已实现**：至少 5 位、字母/数字、拒绝 `default`；旧账户仍可登录。 |
| 新建/重置密码 | 管理员新建用户和重置密码均拒绝少于 **8** 位密码。 | 创建、重置与 UI 均改为 8 位；既有散列未重写。 | **已实现**。 |
| WebDAV、书仓授权 | `enableWebdav`、`enableLocalStore` 是两个可独立切换的字段；Index 分别据此显示对应入口。 | 新增 nullable `can_access_webdav`；UI 与 API 显示独立 WebDAV/书仓开关。 | **已实现**：旧行 `NULL` 回退 `can_access_store`；LocalStore 与 WebDAV/Backup 在后端逐路由独立授权。 |
| 用户书源：设为默认、删除用户书源 | 管理员可把目标用户的私有书源复制为新用户默认书源，或删除所选用户的私有文件并让其回退默认。 | 已用 `UserBookSource`/`BookSourceNamespace` 恢复 user/default 域、目标用户计数和两个管理员动作。重置采用事务调和；仍被书籍引用但不在默认模板中的快照会 detached，不制造悬空引用。 | **已实现 / 允许的数据安全适配**：可见活动书源与上游一致，保留被书籍引用的不可见快照是 Go/SQLite 安全差异。 |
| 批量删除 | 确认后删除用户记录和该用户 namespace 目录。 | SQLite 事务覆盖 chapters、book categories、progress、bookmarks、RSS、rules、settings、source failures 与用户；提交后才清理 regular-user 私有 roots。 | **已实现**：保护管理员/当前账户；另一个用户和管理员 legacy 根均有回归覆盖。 |
| 清理不活跃用户 | 上游没有此产品动作。 | 已从管理器 UI 移除；保留的兼容 API 复用完整删除计划。 | **已实现**：不再存在仅删 `users` 行的路径。 |
| 上次登录 | `saveUserSession` 在成功登录时写 `last_login_at`；列表显示 `lastLoginAt`，空值显示空白。 | 成功注册/登录更新兼容 `last_active_at` 存储值；列表使用 canonical `lastLoginAt`，旧 `lastActiveAt` 保留为等值响应别名。 | **已实现 / 非破坏兼容**。 |
| 列表/操作布局 | 一个表格用于桌面和移动全屏：选择、用户名、上次登录、注册、WebDAV、书仓、操作；移动固定选择/用户名列并横向滚动。底栏依次为批量删除、删除用户书源、选择数、取消。 | 桌面、iPad 和移动共用一张表；核心列、移动固定列和底栏次序均已恢复。书源编辑作为独立扩展列保留，role/统计不再抢占核心表格。 | **已实现 + allowed extension**。 |

## OpenReader API 与数据合同

现有 REST 路径保留为技术适配，不倒退到 `/reader3/*`：

| 目标路径 | 成功副作用 | 失败与边界 |
|---|---|---|
| `POST /api/admin/users` | 创建普通用户：用户名至少 5 位、仅字母/数字且不为 `default`，密码 8 位及以上；旧 `canAccessStore` 和新增的 LocalStore/WebDAV 权限均有确定默认值。 | 非管理员 `403`；角色提升、非法用户名、短密码、重复用户名 `400/409`；不产生管理员。 |
| `PUT /api/admin/users/:id` | 精确更新授权/限额；WebDAV、备份与 LocalStore 权限有独立字段与服务端检查。 | 当前管理员/任一管理员不可修改；不存在 `404`；失败不留下半更新。 |
| `PUT /api/admin/users/:id/password` | 只更新目标普通用户的密码散列。 | 8 位以下 `400`；管理员/当前账户保护、非管理员 `403`。 |
| `POST /api/admin/users/batch-delete` | 一个 SQLite 事务删除用户、books、chapters、book categories、progress、bookmarks、RSS、rules、settings、source failure 等所有 user-owned rows；提交后清理私有 storage/library/upload descendants，广播一次用户更新。 | 输入去重；当前/管理员 ID 不可删除；若没有可删用户 `400`；事务/文件计划失败不可破坏其他用户或 legacy 管理员根。 |
| `POST /api/admin/cleanup-inactive`（兼容扩展） | 仅在保留时，先找出符合条件的普通用户，再完全复用批量删除计划。 | 不在上游 UI 暴露；必须具备明确确认、审计级测试和同样的数据/文件边界。 |
| `POST /api/admin/users/:id/sources/default` | 把目标用户已经初始化的活动书源（包括显式空列表）复制为默认模板，返回 `{count}`。 | 管理员专用；目标不存在 `404`，namespace 未初始化 `409`；不懒初始化目标，也不回退调用者书源。 |
| `POST /api/admin/users/sources/reset` | 先校验全部去重目标和默认模板，再在一个事务内调和；提交后发目标 `sources_update` 和管理员 `users_update`。 | 管理员专用；空选择 `400`，目标/默认缺失 `404`；任一校验或写入失败时整批不变。 |
| `POST /api/auth/login` | 凭证成功后更新用户的兼容 `last_active_at` 存储值，响应 user 同时提供 `lastLoginAt` 和弃用中的 `lastActiveAt`。 | 错误凭证不得更新时间；更新时间失败返回 `500`，不得为未记录的登录签发 token。 |
| `GET /api/admin/users` | 从兼容持久字段返回 `lastLoginAt`，并保留 `lastActiveAt` 旧响应别名。 | 管理员专用；零值在 UI 显示空白。 |

### 加法迁移与存储边界

- 旧 SQLite 的 `can_access_store` 绝不被重写；新增独立 WebDAV/Backup 许可列时，以
  该旧值作为读取回退和新列默认语义，后续管理员显式保存才区分。
- `data/webdav/users/<safe-username>/`、`library/localStore/users/<safe-username>/`、
  user-local imported archive root 和 `data/uploads/users/<user-id>/` 只能在该用户数据库
  删除提交后清理；`data/webdav/`、`library/localStore/` 的管理员 legacy 根绝不能由删除
  regular user 的动作触碰。
- Source 所有权的决策和迁移闸门见
  [`book-source-ownership-p2-contract.md`](book-source-ownership-p2-contract.md)。必须通过
  加法关联迁移保留旧 source ID，以用户关联和写时复制隔离后续编辑并保留默认快照；
  禁止直接给旧行填一个 owner，导致其他用户的既有书籍越权或失效。

## 必须先写的测试

1. Go：管理员新建用户拒绝 4 位、非字母数字及 `default`，接受 5 位合法用户名；新建/重置密码拒绝 7 位，接受 8 位；已存在短密码或旧用户名账户仍可登录。
2. Go/API：成功登录推进持久化登录时间，并在登录响应和管理员列表同时返回一致的
   `lastLoginAt`；错误密码不推进时间，旧 `lastActiveAt` 别名继续存在。
3. Go：两个普通用户各自拥有 books/chapters/categories/book_categories/progress/bookmarks/
   RSS/rules/settings/source-failure/私有 WebDAV、LocalStore、uploads；批量删除或兼容
   cleanup 只完整删除目标用户，管理员 legacy 根与另一个用户均不变。
4. Go：数据库事务失败和文件清理计划失败时不删除用户行或其他数据；提交后文件清理
   失败只报告/记录，不回滚已经完成的数据库删除，也不能越界。
5. Go/API：LocalStore 禁用不阻止有 WebDAV 权限的 WebDAV/备份，反之亦然；旧卷的
   单权限行保持与历史 `canAccessStore` 相同的可访问性直到显式管理员变更。
6. 前端：先写结构契约，要求单一表格、核心列与底栏顺序、显式取消、移动固定选择/用户名
   列、准确的 `lastLoginAt` 字段；删除把移动卡片和“最近活跃”当成正确行为的旧测试。
7. 真实浏览器（1440×900、390×844、360×800）：管理员创建用户、独立切换书源编辑/
   WebDAV/书仓、重置密码、设默认、删除用户书源、批量删除确认/取消；移动端同一表格可
   横向滚动且选择/用户名保持可见。非管理员无管理入口且 API 返回 403。

## 实施与发布记录

- Git：`f44447f`（合同取证提交为 `8ee578c`）。
- Docker：`ghcr.io/changshengyu/openreader:f44447f` 与 `:latest`，远端 index digest
  `sha256:a3fb3e55d0e7002390f34e2bb30fb54adda9b5a64e2a8e1ca87b0f7941f7a64c`；
  amd64 `sha256:80522fa3872cbb0ff1c61e571eb96db8b86064ea861877c36de943701b8b379e`，
  arm64 `sha256:0115c294a716c76ec3f301117d37e0a00e093dee4f33b56629ef46365cfdac72`。
- 自动门：Go 全量、frontend `641/641`、生产构建和 UserManage
  `1440×900 / 1024×1366 / 390×844 / 360×800` 浏览器合同通过。
- 卷门：新卷、重启、portable v1/v2 assets、跨用户以及历史
  TXT/EPUB/UMD/CBZ/相对缓存/owner 隔离全部通过。
- GHCR 在发布后回拉阶段连续返回 `502 Bad Gateway`，但 tag/index 已由远端 registry
  manifest 查询确认；卷门使用同提交、本机重新加载的 arm64 镜像完成。

用户管理本轮标记为 `aligned / Docker-published`。备份/WebDAV 文件操作与 RSS 的独立
专项合同仍然成立，不能借本模块发布宣称全项目复审完成。
