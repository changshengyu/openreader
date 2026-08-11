# 公开认证请求边界第二轮固定基准合同（P2）

状态：**inventory-complete / implementation-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只提取合同并映射现状，不修改应用或测试代码。范围严格限定为：

- `POST /api/auth/login`
- `POST /api/auth/register`

管理员创建/重置用户、公开注册开关、账号总量、速率限制和 JWT 生命周期不在本切片中重设计。

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
