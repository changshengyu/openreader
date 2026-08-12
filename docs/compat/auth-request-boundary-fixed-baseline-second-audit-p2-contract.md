# 公开认证请求边界第二轮固定基准合同（P2）

状态：**aligned / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

合同先在 `052de86` 中独立提取并映射现状；应用实现和测试在合同提交后完成。范围严格限定为：

- `POST /api/auth/login`
- `POST /api/auth/register`

管理员创建/重置用户、公开注册开关、账号总量、速率限制和 JWT 生命周期不在本切片中重设计。
2026-08-12 的固定上游 follow-up 证明，共享的新密码最小长度还需按 UTF-16 code units 修正；管理员
写入口的 body/batch/bcrypt 边界也需单独签收。见
[`admin-user-write-boundary-fixed-baseline-second-audit-p2-contract.md`](admin-user-write-boundary-fixed-baseline-second-audit-p2-contract.md)。

## 1. 权威文件与入口

固定上游：

- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `POST /reader3/login`；
- `src/main/java/com/htmake/reader/api/controller/UserController.kt#login`；
- `src/main/java/com/htmake/reader/api/ReturnData.kt`；
- `web/src/views/Index.vue` 的登录/注册调用。

OpenReader：

- `backend/api/server.go`、`auth.go`；
- `backend/api/user_management_p2_contract_test.go`、`api_test.go`；
- `frontend/src/components/AuthForm.vue`、`stores/user.js`、`api/user.js`、`api/client.js`。

## 2. 固定上游与技术栈映射

固定上游以一个 `/reader3/login` JSON 动作和 `isLogin` 区分登录/注册，读取 `username/password`；空字段
失败，注册继续执行用户名至少 5 位、ASCII 字母数字、保留名 `default` 和密码至少 8 位校验。登录
成功写回 session/最后登录状态。上游没有显式 body 上限，也不会提供可复制到 Go 的安全边界。

OpenReader 将动作拆为 `/api/auth/login` 与 `/api/auth/register`，成功返回 `{token,user}`；未知用户和
错误密码统一 `401`。JWT、首用户管理员、bcrypt、通用错误 JSON 和拆分路由均为已经签收的多用户/
Go 技术栈适配。本轮不得借请求边界重开这些状态机。

## 3. 现状差异矩阵

| 合同点 | 审计时 OpenReader | 裁决 |
|---|---|---|
| 请求体总量 | 两个公开路由直接 `ShouldBindJSON`，未使用 `http.MaxBytesReader`；声明长度和 chunked body 都可在认证前无界读取。 | **must-fix security adaptation**：每个 decoded body 最多 16 KiB。 |
| JSON 文档边界 | Gin JSON binding 只 `Decode` 一次，合法首对象后的第二个 JSON 值或垃圾可被忽略。 | **must-fix**：只接受一个 JSON 值；尾部仅允许 JSON whitespace。 |
| 超限错误 | 无可达 `413`；超大但合法 JSON 会继续查库或注册校验。 | **must-fix**：声明或流式超限统一 `413 {"error":"request body too large"}`，不得查库、bcrypt 或写行。 |
| 格式/必填错误 | malformed、缺字段和空字符串为 `400 {"error":"username and password are required"}`。 | **aligned**：继续保持现有状态、字段和消息；超限不能伪装成该 `400`。 |
| 新密码长度 | 注册只检查至少 8 字符；Go bcrypt 对超过 72 **bytes** 返回 `ErrPasswordTooLong`，当前被映射成 `500 failed to hash password`。 | **must-fix runtime adapter**：新密码必须为 8 字符且至多 72 bytes；过长为可操作的 `400`，不暴露库错误。 |
| 登录失败 | 未知账号和错误密码统一 `401`；失败不更新 `last_active_at`。 | **aligned security enhancement**：过长/错误密码同样保持通用 `401`，不得泄漏账号是否存在。 |
| 旧账号 | 登录只 trim 用户名，不重新执行注册规则；历史含连字符账号可登录。 | **aligned data compatibility**：请求上限不得变成旧账号用户名/密码注册规则迁移。 |
| 前端错误 | `AuthForm` 显示顶层字符串 `error`；auth 请求的 `401` 不触发私有会话失效拦截器。 | **aligned**：`400/413/401` 都沿当前表单错误路径显示，不清除其它当前 token。 |

## 4. 目标 API 合同

### 请求

- Method/path 保持 `POST /api/auth/login`、`POST /api/auth/register`。
- 无需 JWT；body 保持 JSON `{username,password}`，未知字段继续忽略以兼容现有客户端。
- wire body 上限固定为 16 KiB，包含 JSON 标点、字符串转义、未知字段和尾部空白。
- 声明 `Content-Length` 与未知长度/chunked 输入使用同一实际读取上限；不能只检查 header。
- 只允许一个 JSON 值。首对象后的空格、tab、CR/LF 可接受；第二对象、数组、数字或非空垃圾为 `400`。

### 响应与副作用

| 场景 | Status / body | 副作用 |
|---|---|---|
| 合法登录/注册 | 保持现有 `200 {token,user}`。 | 保持现有登录时间或用户创建事务。 |
| body 超过 16 KiB | `413 {"error":"request body too large"}`。 | 零 DB 查询/写入、零 bcrypt、零 token。 |
| malformed、多 JSON 值、缺失/空字段 | `400 {"error":"username and password are required"}`。 | 零登录时间/用户写入。 |
| 注册密码超过 72 bytes | `400 {"error":"password must be at most 72 bytes"}`。 | 零 bcrypt/用户写入。 |
| 登录凭证不匹配 | 保持 `401 {"error":"invalid username or password"}`。 | 不更新登录时间。 |
| 重复注册 | 保持 `409 {"error":"username already exists"}`。 | 不创建第二行。 |

错误不得包含请求正文、用户名、密码、bcrypt 错误、JWT 或 SQLite 文本。正文不得写入日志、缓存、
SQLite、备份或 WebSocket event。

## 5. 数据与兼容边界

- 不修改 SQLite schema、User row、JWT claim、`data/`、`cache/`、`library/`、备份/WebDAV 或浏览器存储。
- 16 KiB 明显覆盖正常账号请求，但它是新的显式 wire contract；未来修改必须版本化记录和测试。
- bcrypt 72-byte 上限只约束新注册密码。旧账号登录仍由既有 hash 比较决定，不对用户名执行新注册校验。
- 不加入前端 `maxlength` 作为安全边界；服务器必须独立处理直接 API、错误 Content-Length 和 chunked body。
- 本切片不宣称解决暴力破解、注册配额或反向代理限制；这些必须另有部署/产品合同，不能用 body cap
  代签。

## 6. 测试先行闸门

实现前必须先让以下 Go API 合同在当前代码上失败：

1. login/register 的 declared oversized body 都返回精确 `413` 和安全 JSON。
2. `ContentLength=-1` 的流式 oversized body 同样返回 `413`，证明不是 header-only 检查。
3. 合法首对象后追加第二个 JSON 值返回 `400`；只追加 whitespace 仍进入正常认证状态机。
4. 超限/多值注册不创建用户；超限/多值登录不更新既有用户 `last_active_at`。
5. 73-byte 新密码返回精确 `400` 而非 `500`，且不创建用户；72-byte 新密码仍可注册和登录。
6. 普通成功、重复注册、未知/错误密码、旧连字符账号及 `lastLoginAt` 既有测试继续通过。
7. 错误正文和测试日志不包含提交的密码或 token。

实现后运行 auth/API 聚焦测试、`go test ./...`、focused race、`go vet ./...`、frontend 全量和 production
build。该切片无 UI 几何变化，真实运行时门使用生产二进制对声明/流式 413、正常注册/登录和健康检查
做 HTTP smoke；适合发布时仍执行顺序 fresh/historical mounted-volume 门和本机双架构发布。

## 7. 实施边界

实现应把有界单值 JSON 解码集中在认证 handler 的窄 helper 中；handler 仍只负责 bind/validate、状态码
和序列化。不得新增第二套路由、全局 body 中间件或改变其它 JSON endpoint 的既有上限。若实施发现
需要账号速率/总量策略，先另建合同，不在本切片中顺带加入。

## 8. 实施与回归结果（2026-08-12）

- `backend/api/auth.go` 使用认证专用 helper，同步检查声明长度并通过 `http.MaxBytesReader` 限制实际读取；
  首值后第二次 decode 必须得到 EOF，因此只保留尾部 JSON whitespace 兼容。
- 注册在 bcrypt 前拒绝超过 72 bytes 的密码并返回精确 `400`；登录在查库前将同类输入映射为通用
  `401`，关闭 bcrypt 截断导致“前 72 bytes 相同即可登录”的运行时风险。
- `backend/api/auth_request_boundary_contract_test.go` 先在旧实现上证明 declared/chunked 超限会注册/
  登录、第二 JSON 值被忽略、73-byte 注册变成 `500` 且 73-byte 登录可错误成功，再锁定精确状态、
  错误体、72-byte 正常路径和 SQLite 零副作用。
- `scripts/smoke/auth-request-boundary-contract.mjs` 在隔离的生产形态服务上通过 declared/chunked `413`、
  单 JSON、精确 16 KiB、72/73-byte 边界、错误不回显和拒绝请求不新增用户。
- `go test ./...`、认证 focused race、`go vet ./...`、frontend 740/740、production build、
  `git diff --check` 全部通过。该切片没有前端几何变化，不需要新增视口截图门。
- SQLite schema、JWT、`data/`、`cache/`、`library/`、backup/WebDAV 和旧账号行均未改变。
- 实现提交 `f5c15d7` 顺序通过 fresh portable-v1/v2-assets/cross-user/restart 与 historical
  TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation；本机双架构发布后，`f5c15d7` 与 `latest` 远端回读
  同为 OCI index `sha256:db667de319ae2721cbd35990896612a738b4570a94920875ea14e2aed613503f`，包含
  `linux/amd64` manifest `sha256:b4ade586cf8b2d3eb04eb0767826aec3395c1f91eedeadad4d961a71a06cc6b1`
  和 `linux/arm64` manifest `sha256:d41e46a29b613d3cae5d86911e0e625c95190b947beead317935c8e24665feec`。
- 共享最小长度 follow-up 已随管理员写入专项 `6c1c6db` 完成：public register 现在按固定上游 Kotlin/
  JavaScript 的 UTF-16 code units 拒绝 6、接受 8，同时继续执行 72 UTF-8 bytes 上限；旧账号登录不受
  影响。该提交完成全量/race/vet、真实 HTTP、新旧卷并发布为 `6c1c6db`/`latest`，OCI index 为
  `sha256:55326ed147aea4370c0161d75568fe85a5095abb6dad6b487856dfeea09832a2`。
