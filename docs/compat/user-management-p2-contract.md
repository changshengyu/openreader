# P2 用户管理上游复审合同

状态：2026-07-17 已实施账户规则、独立 WebDAV/书仓权限和安全删除切片，并以
固定基准 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
回归验证。书源所有权动作仍是独立 P2 依赖；本合同不把当前 OpenReader 的组件、
路由或旧测试当作正确性依据。

上游权威文件：

- `web/src/components/UserManage.vue`
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
| 受保护用户 | `default` namespace 不可选择、不可改 WebDAV/书仓；其他用户可选。 | `admin` 与当前登录管理员不可删除；`admin` 不可修改/重置。 | **允许的多用户安全适配**：管理员不是可删除的默认 namespace；不能放松为可删/可改。 |
| 新建用户名 | 管理员创建用户至少 5 位，仅允许 ASCII 字母/数字，且保留 `default` namespace。 | 共享后端校验覆盖注册与管理员创建；UI 同步提示。旧用户名不重写、不锁定。 | **已实现**：至少 5 位、字母/数字、拒绝 `default`；旧账户仍可登录。 |
| 新建/重置密码 | 管理员新建用户和重置密码均拒绝少于 **8** 位密码。 | 创建、重置与 UI 均改为 8 位；既有散列未重写。 | **已实现**。 |
| WebDAV、书仓授权 | `enableWebdav`、`enableLocalStore` 是两个可独立切换的字段；Index 分别据此显示对应入口。 | 新增 nullable `can_access_webdav`；UI 与 API 显示独立 WebDAV/书仓开关。 | **已实现**：旧行 `NULL` 回退 `can_access_store`；LocalStore 与 WebDAV/Backup 在后端逐路由独立授权。 |
| 用户书源：设为默认、删除用户书源 | 管理员可把被选用户的私有书源复制为新用户默认书源，或删除所选用户的私有文件并让其回退默认。 | `BookSource` 是全局 SQLite 表，无 `user_id`；用户表的 `sourceCount` 也明确为全局数。无对应动作。 | **依赖 P2 书源所有权审查**：不能新增一个“成功但无效果”的按钮。若全局书源模型被判定为合法技术适配，界面必须明确其全局含义并记录这两个单用户动作不适用；若恢复 user/default 书源域，则同一事务实现两个上游动作。 |
| 批量删除 | 确认后删除用户记录和该用户 namespace 目录。 | SQLite 事务覆盖 chapters、book categories、progress、bookmarks、RSS、rules、settings、source failures 与用户；提交后才清理 regular-user 私有 roots。 | **已实现**：保护管理员/当前账户；另一个用户和管理员 legacy 根均有回归覆盖。 |
| 清理不活跃用户 | 上游没有此产品动作。 | 已从管理器 UI 移除；保留的兼容 API 复用完整删除计划。 | **已实现**：不再存在仅删 `users` 行的路径。 |
| 列表/操作布局 | 表格：用户名、最后登录、注册、WebDAV、书仓、重置密码/设默认；底栏：批量删除、删除用户书源、选择数。 | 额外显示 role、书籍/全局书源计数、刷新/清理；权限只有书源/书仓，底栏只有批量删除。 | **must-fix after data contract**：恢复上游动作与选择/确认顺序；role、限额和全局计数只能作为不抢占上游操作的多用户信息。 |

## OpenReader API 与数据合同

现有 REST 路径保留为技术适配，不倒退到 `/reader3/*`：

| 目标路径 | 成功副作用 | 失败与边界 |
|---|---|---|
| `POST /api/admin/users` | 创建普通用户：用户名至少 5 位、仅字母/数字且不为 `default`，密码 8 位及以上；旧 `canAccessStore` 和新增的 LocalStore/WebDAV 权限均有确定默认值。 | 非管理员 `403`；角色提升、非法用户名、短密码、重复用户名 `400/409`；不产生管理员。 |
| `PUT /api/admin/users/:id` | 精确更新授权/限额；WebDAV、备份与 LocalStore 权限有独立字段与服务端检查。 | 当前管理员/任一管理员不可修改；不存在 `404`；失败不留下半更新。 |
| `PUT /api/admin/users/:id/password` | 只更新目标普通用户的密码散列。 | 8 位以下 `400`；管理员/当前账户保护、非管理员 `403`。 |
| `POST /api/admin/users/batch-delete` | 一个 SQLite 事务删除用户、books、chapters、book categories、progress、bookmarks、RSS、rules、settings、source failure 等所有 user-owned rows；提交后清理私有 storage/library/upload descendants，广播一次用户更新。 | 输入去重；当前/管理员 ID 不可删除；若没有可删用户 `400`；事务/文件计划失败不可破坏其他用户或 legacy 管理员根。 |
| `POST /api/admin/cleanup-inactive`（兼容扩展） | 仅在保留时，先找出符合条件的普通用户，再完全复用批量删除计划。 | 不在上游 UI 暴露；必须具备明确确认、审计级测试和同样的数据/文件边界。 |

### 加法迁移与存储边界

- 旧 SQLite 的 `can_access_store` 绝不被重写；新增独立 WebDAV/Backup 许可列时，以
  该旧值作为读取回退和新列默认语义，后续管理员显式保存才区分。
- `data/webdav/users/<safe-username>/`、`library/localStore/users/<safe-username>/`、
  user-local imported archive root 和 `data/uploads/users/<user-id>/` 只能在该用户数据库
  删除提交后清理；`data/webdav/`、`library/localStore/` 的管理员 legacy 根绝不能由删除
  regular user 的动作触碰。
- Source 所有权是单独 P2 合同的前置决策，不在本合同中悄悄给全局 `BookSource` 加
  `user_id` 或复制现有行。

## 必须先写的测试

1. Go：管理员新建用户拒绝 4 位、非字母数字及 `default`，接受 5 位合法用户名；新建/重置密码拒绝 7 位，接受 8 位；已存在短密码或旧用户名账户仍可登录。
2. Go：两个普通用户各自拥有 books/chapters/categories/book_categories/progress/bookmarks/
   RSS/rules/settings/source-failure/私有 WebDAV、LocalStore、uploads；批量删除或兼容
   cleanup 只完整删除目标用户，管理员 legacy 根与另一个用户均不变。
3. Go：数据库事务失败和文件清理计划失败时不删除用户行或其他数据；提交后文件清理
   失败只报告/记录，不回滚已经完成的数据库删除，也不能越界。
4. Go/API：LocalStore 禁用不阻止有 WebDAV 权限的 WebDAV/备份，反之亦然；旧卷的
   单权限行保持与历史 `canAccessStore` 相同的可访问性直到显式管理员变更。
5. 前端：恢复上游用户名/8 位提示、选择保护、确认取消、批量删除、两项独立权限与对应
   API 精确 payload；不显示危险的 cleanup 入口。
6. 真实浏览器（1440×900、390×844、360×800）：管理员创建用户、切换两项权限、
   重置密码、删除确认/取消的 root Dialog 操作；非管理员无管理入口且 API 返回 403。

本合同、上述回归和 Docker 用户卷验证未通过前，用户管理不能从“尚未验证”标记为
P2 对齐；备份/WebDAV 文件操作与 RSS 的单独复审也不能借此宣称完成。
