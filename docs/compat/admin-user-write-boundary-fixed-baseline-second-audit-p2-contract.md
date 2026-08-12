# 管理员用户写入请求边界与共享密码长度第二轮固定基准合同（P2）

状态：**inventory-complete / implementation-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只提取合同并记录当前反例，不修改应用或测试代码。范围是：

- `POST /api/auth/register`：只复审所有新密码共用的最小长度语义；其 16 KiB 单 JSON wire boundary
  继续由公开认证专项合同约束。
- `POST /api/admin/users`
- `PUT /api/admin/users/:id`
- `PUT /api/admin/users/:id/password`
- `POST /api/admin/users/sources/reset`
- `POST /api/admin/users/batch-delete`

用户列表、登录验证、权限列的部分更新所有权、用户删除工作区计划、书源 namespace 与 WebSocket
recipient scope 已有专项合同，本轮不得借请求边界重开。

## 1. 权威文件与入口

固定上游：

- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `/reader3/addUser`、`resetPassword`、
  `updateUser`、`deleteUsers` 与 `/reader3/login`；
- `src/main/java/com/htmake/reader/api/controller/UserController.kt#addUser/resetPassword/updateUser/deleteUsers`；
- `web/src/components/AddUser.vue`、`UserManage.vue`。

OpenReader：

- `backend/api/server.go`、`admin.go`、`auth.go`、`helpers.go`；
- `backend/api/admin_contract_test.go`、`user_management_p2_contract_test.go`、
  `auth_request_boundary_contract_test.go`；
- `frontend/src/components/overlays/OverlayUserManagement.vue`、
  `composables/useOverlayUserManagement.js`、`api/admin.js`。

## 2. 固定上游与技术栈映射

固定上游的新增用户和重置密码都先读取一个 JSON 对象，检查用户名/密码和管理权限，再更新用户
namespace。`password.length < 8` 使用 Kotlin/Java `String.length`，即 UTF-16 code unit；当前 Vue
校验的 JavaScript `String.length` 具有相同计数。上游散列没有 bcrypt 的 72-byte 限制，也没有显式
body/cardinality 上限，因此这两项必须作为 Go 安全适配补充，不能照抄为无界输入。

OpenReader 保留稳定 JWT REST 路径、管理员角色、SQLite 普通用户行和结构化管理员错误体。管理员
创建/重置是上游动作的技术栈映射；权限字段、限额、批量 ID 与 source namespace 是已签收的多用户
扩展。

## 3. 当前反例与差异矩阵

2026-08-12 在隔离的当前生产形态服务上得到以下反例：

- 73-byte 管理员创建和重置密码都返回 `500`；创建不写行，重置保留旧散列。
- `"密码"` 重复三次只有 6 个 UTF-16 code units，却因 Go `len` 得到 18 bytes 而创建成功。
- 超过 16 KiB、含未知 `padding` 的管理员创建请求返回 `201` 并写行。
- 两个连续 JSON 对象只消费首对象，管理员创建返回 `201`，重置返回 `200` 并采用首个密码。

| 合同点 | 审计时 OpenReader | 裁决 |
|---|---|---|
| 管理员 JSON wire body | 五个 JSON 写入口直接 `ShouldBindJSON`，没有声明/实际读取上限。 | **must-fix security adaptation**：统一 16 KiB actual-read 上限。 |
| JSON 文档边界 | Gin 只 decode 首值，第二个值或尾随垃圾被忽略。 | **must-fix**：只接受一个 JSON 值；只允许尾部 whitespace。 |
| 批量 cardinality | reset-sources 与 batch-delete 可解码任意数量 ID。 | **must-fix security adaptation**：原始 ID 数组最多 2,000 项，之后仍按既有去重/保护规则处理。 |
| 新密码最小长度 | Go 用 UTF-8 bytes；多字节短密码可绕过 8 位规则。 | **must-fix runtime adapter**：注册、管理员创建和重置按 UTF-16 code units 至少 8。 |
| bcrypt 最大长度 | 公开注册已处理；管理员创建/重置把 `ErrPasswordTooLong` 映射成 `500`。 | **must-fix runtime adapter**：所有新密码至多 72 UTF-8 bytes，过长为可操作 `400`。 |
| 校验与 bcrypt 顺序 | create 在非法 role 前 hash；reset 在目标不存在/受保护前 hash。 | **must-fix resource boundary**：完成确定性 payload/target 检查后才执行 bcrypt；失败零 hash/写入/event。 |
| 管理员创建限额 | update 拒绝负 book/source limit，create 可写负值。 | **must-fix data validation**：创建与更新统一拒绝负值；0 的既有“无限”语义不变。 |
| 旧账号登录 | 登录不执行新密码最小长度，只验证既有 hash。 | **aligned data compatibility**：短密码/旧用户名账号继续可登录。 |
| 前端错误 | UserManage 读取结构化 `error.message`；公开 AuthForm 兼容字符串/结构化错误。 | **aligned**：不需要新 UI；服务端仍是直接 API 的权威边界。 |

## 4. 目标 API 合同

### 4.1 共同 wire boundary

- 五个管理员 JSON 写入口必须先通过现有 Bearer 与 `requireAdmin`；未认证/非管理员继续返回既有
  `401/403`，不得为了检查 body 而读取、解析或记录凭据。
- 管理员身份通过后，wire body 最多 16 KiB，包含 JSON 标点、转义、未知字段和尾部空白。
- `Content-Length` 与未知长度/chunked 使用同一实际读取上限；不能只检查 header。
- 只允许一个 JSON 值。尾部 JSON whitespace 可接受；第二对象/数组/数字或非空垃圾为 malformed。
- 未知对象字段继续忽略，以保留旧 OpenReader 客户端兼容。

| 场景 | Status / body | 副作用 |
|---|---|---|
| 管理员 body 超限 | `413 {"error":{"code":"REQUEST_TOO_LARGE","message":"request body too large"}}` | 零 bcrypt、用户/source namespace 写入与 sync event。 |
| malformed、多 JSON、缺少必需字段 | 保持管理员结构化 `400 BAD_REQUEST`；解码错误 message 为 `invalid payload`。 | 零 bcrypt/写入/event。 |
| batch 原始 ID 超过 2,000 | `400 BAD_REQUEST`，message `too many users selected`。 | 不查询/删除/重置目标 namespace，不广播。 |

精确 16 KiB 必须进入原有业务状态机；16 KiB + 1 必须为 `413`。上限不应用到无 body 的管理员
动作，也不扩散为会改变其它 API 的全局中间件。

### 4.2 新密码合同

- `POST /api/auth/register`、`POST /api/admin/users` 和管理员 password reset 的新密码同时满足：
  - 至少 8 UTF-16 code units，与固定上游 Kotlin 及当前 Vue `String.length` 一致；
  - 至多 72 UTF-8 bytes，适配 Go bcrypt 的硬限制。
- public register 的错误形状保持已发布的平面 `{error:string}`；管理员动作保持结构化
  `{error:{code,message}}`。
- 管理员创建/重置的 overlong message 为 `password must be at most 72 bytes`，不得暴露 bcrypt 错误。
- 登录继续只 trim 用户名并比较既有 hash；不得用新密码规则拒绝历史短密码或旧用户名。
- 空/短密码、非法用户名、保留名、role escalation、负限额、目标不存在和受保护管理员的既有状态/
  文案保持。能够在 bcrypt 前裁决的失败必须先返回。

### 4.3 成功与持久化

- 管理员创建仍为 `201` 普通 `user` row，保留权限默认/显式字段、重复用户名 `409`、一次 post-commit
  `users_update`。
- 权限部分更新仍为 `200` 完整 user response，只更新显式字段；本轮只增加 body/single-JSON/负限额
  前置边界。
- 密码重置仍为 `200 {success:true}`，只更新目标普通用户 `password_hash` 并在提交后广播；不改
  `last_active_at`、role、权限或限额。
- reset-sources 与 batch-delete 的事务、保护管理员/当前用户、工作区清理和事件语义不变。
- 任何错误都不得包含密码、token、请求正文、bcrypt 文本、SQLite 文本或 host path。

## 5. 数据与迁移边界

- 不修改 SQLite schema、现有 hash、JWT、`data/`、`cache/`、`library/`、backup/WebDAV 或浏览器 key。
- 只验证未来提交的新密码；不扫描、重算、禁用或迁移现有账户。
- 16 KiB 和 2,000 项是新的显式 wire/work 上限；正常 UserManage payload 远低于它们。
- body/cardinality/password 拒绝发生在事务和文件计划前，不得产生待清理目录或 namespace。

## 6. 测试先行闸门

实现前必须先让以下合同在当前代码上失败：

1. 五个管理员 JSON 写入口的 declared/chunked oversized body 都返回精确结构化 `413`；合法首对象
   后第二 JSON/垃圾为 `400`，尾部 whitespace 仍走原状态机。
2. 失败创建不新增 row；失败更新/重置不改字段/hash；失败 reset/delete 不改 namespace/用户数据，
   且不发 sync event。
3. 2,001 个原始 ID 为 `400`；2,000 项仍按既有去重、保护和事务语义处理。
4. 注册、管理员创建和重置拒绝 6 个 UTF-16 单元的多字节密码，接受 8 个单元且不超过 72 bytes。
5. 管理员创建/重置 73-byte 密码为精确 `400` 而非 `500`；72-byte 创建、重置和随后登录成功。
6. 非管理员/缺 token 携 oversized body 仍优先返回既有 `403/401`，不创建、不 hash、不泄漏 body。
7. role escalation、负限额、缺失/受保护目标在 bcrypt/写入前失败；成功响应和现有旧账号登录测试保持。
8. 错误正文与测试输出不包含提交密码或 token。

实现后运行 focused admin/auth tests、`go test ./...`、focused race、`go vet ./...`、frontend 全量和
production build。该切片无 UI 几何变化；真实运行时使用隔离生产服务验证 declared/chunked、双 JSON、
UTF-16/bcrypt 边界、权限优先级和零错误写入。发布前执行顺序 fresh/historical mounted-volume 门。

## 7. 实施边界

实现应抽出只负责“实际读取上限 + 单 JSON”的窄共享 decoder，由 public auth 与管理员 user handlers
各自映射现有错误形状。它不得成为全局中间件，也不得顺带改变 remote Reader、书源、RSS、替换规则
或上传端点的已发布上限。密码长度 helper 必须明确 UTF-16 code units 与 UTF-8 bytes 两种单位；前端
`maxlength` 不能替代服务器验证。
