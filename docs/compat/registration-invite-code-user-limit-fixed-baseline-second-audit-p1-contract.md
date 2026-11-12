# 注册邀请码与用户数上限 P1 合同

日期：2026-11-11。分支 `claude/registration-invite-user-limit`，基于 main `e8f3214`。固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

用户要求“先补注册限制”。2026-11-10 的进度分析发现：上游有注册邀请码与用户数上限；OpenReader 注册完全开放，兼容总账里没有对应合同。
用户数上限的默认规则与错误提示的范围，原本要请用户确认，但确认问题因应用退出没有得到回答。因此本合同按“上游行为是产品合同”取上游规则；
如果用户另有要求，在实施前修改本合同。

## 1. 上游行为（fa22f271）

- 配置：`src/main/resources/application.yml` 中 `reader.app.inviteCode: ""`、`userLimit: 50`（`config/AppConfig.kt`）。
  - `doc.md`：inviteCode 为空时开放注册，否则注册需要邀请码。
  - `docker-compose.yml`：`READER_APP_USERLIMIT` 注释“用户上限,默认且最大值为50”；`READER_APP_INVITECODE=registercode` 注释“建议修改”。
- `UserController.kt#getUserLimit`：`min(max(appConfig.userLimit, 1), userMaxCount)`，`userMaxCount = 50`。作者自有站点 500 人的特例不属于产品行为。
- 注册（`UserController.kt#login`，`isLogin=false` 且用户不存在）依次检查：
  1. 用户名为空 → “请输入用户名”；密码为空 → “请输入密码”；
  2. 用户名少于 5 位 → “用户名不能低于5位”；密码少于 8 位 → “密码不能低于8位”；
  3. 用户名为 `default` → “用户名不能为非法字符”；用户名含字母、数字以外的字符 → “用户名只能由字母和数字组成”；
  4. `inviteCode` 非空时：请求 `code` 为空 → “请输入邀请码”，与 `inviteCode` 不相等 → “邀请码错误”；
  5. 已有用户数 ≥ 上限 → “超过用户数上限”；
  6. 创建账号并登录。

  其它情况：注册时用户名已存在 → “用户名已被占用”；登录时用户不存在 → “用户不存在”，密码错误 → “密码错误”。
- 管理员新增用户（`UserController.kt#addUser`）：用户名和密码规则同上；用户已存在 → “用户已存在”；已有用户数 ≥ 上限 → “超过用户数上限”。
  不检查邀请码。
- 上限只在新增账号时检查，已有账号照常登录。
- 前端 `web/src/App.vue` 的登录对话框：
  - 注册模式（`!isLogin`）显示“邀请码(没有则不填)”输入框（`loginForm.code`），随 `isLogin:false` 一起提交；登录模式不显示。
  - 失败时由 `plugins/axios.js` 以 `Message.error(errorMsg)` 弹出上面的中文原文。

## 2. 当前 OpenReader（e8f3214）

- `backend/api/auth.go#register`：
  - 16 KiB 单 JSON 请求，`authRequest` 只有 `username`/`password` 两个字段，未知字段被忽略；
  - 72 字节密码上限，`validateNewAccountCredentials` 的规则与上游相同，消息是英文；
  - `registerMu` 加事务：查重返回 409；首个账号成为管理员；建号后签发会话；
  - 没有邀请码，也没有用户数上限。
- `backend/api/admin.go#createUser`：管理员鉴权，同样的校验，事务建号，冲突时返回 409 `errResp("CONFLICT", …)`。没有用户数上限。
- `backend/config/config.go`：没有相关配置。
- `frontend/src/components/AuthForm.vue`（`/login` 页面与 `AuthDialog` 共用）：
  - 只有用户名、密码两项，以及登录/注册切换（“创建新账号”/“已有账号，去登录”），没有邀请码输入框；
  - 失败时 `ElMessage.error` 直接显示后端英文，例如 `username must be at least 5 characters`；提交前也不做空值提示。
- `frontend/src/components/overlays/OverlayUserManagement.vue#readError`：显示后端 `error.message` 或 `error` 的原文。
- `README.md`：“Anyone who can open the page can currently create an account”，建议用 VPN 或 IP 白名单挡住陌生人；FAQ 回答“可以注册”。

## 3. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| R1 | 没有注册邀请码 | `must-fix`（P1 安全） | 见第 4 节 |
| R2 | 注册与管理员新增都没有用户数上限 | `must-fix` | 见第 4 节 |
| R3 | 注册表单没有邀请码输入框 | `must-fix` | 注册模式显示“邀请码(没有则不填)”，登录模式不显示；只在注册时提交 `code` |
| R4 | 注册/登录失败显示后端英文，提交前没有空值提示 | `must-fix`（可见文案） | 按第 5 节映射为上游中文 |
| R5 | 校验顺序 | `must-fix` + 安全适配 | 字段/长度/字符校验 → 邀请码 → 用户名占用 → 用户数上限 → 建号。邀请码先于占用检查：不持有邀请码时，不能借 409 枚举用户名 |
| A1 | 登录失败统一为一个提示（上游分“用户不存在”与“密码错误”） | `acceptable-change`（防枚举，已签收的通用 401） | 只把文案改为中文“用户名或密码错误” |
| A2 | 邀请码只从环境变量读取；配置值与提交值都去掉首尾空白后按常量时间比较；不出现在任何响应或日志中 | `acceptable-change`（安全） | — |
| A3 | 环境变量用 `OPENREADER_` 前缀，不接受上游 `READER_APP_*` 名称 | `acceptable-change` | README 写明对应关系 |
| A4 | 72 字节密码上限、413、429 是 OpenReader 自有的错误 | `acceptable-change` | 给出中文文案（第 5 节） |
| O1 | 每用户书籍上限默认 200（`userBookLimit`）、“记住登录信息”复选框、管理员界面其它英文提示 | 不在本切片 | 另起 |
| R6 | `/login` 卡片在窄屏下超出视口（第 10 节补充，原有缺陷） | `must-fix` | 卡片与每个表单项都落在视口内 |

## 4. 目标行为（R1、R2、R5）

- `OPENREADER_INVITE_CODE`：默认空，表示开放注册。
  - 非空时，`POST /api/auth/register` 必须提交相同的 `code`。缺少或为空返回 `400 {"error":"invite code required"}`；不相等返回 `403 {"error":"invalid invite code"}`。
  - 管理员新增用户不检查邀请码。
- `OPENREADER_USER_LIMIT`：默认 50，有效值为 `min(max(n,1),50)`，非整数按默认值。
  - 计数在建号的同一个事务内进行，并发注册也不会超过上限。
  - 达到上限时，注册返回 `403 {"error":"user limit reached"}`，管理员新增返回 `403 errResp("USER_LIMIT_REACHED","user limit reached")`。
  - 不建行，不签发会话，不广播。已有账号照常登录。
- 注册的检查顺序：解码 → 72 字节 → `validateNewAccountCredentials` → 邀请码 → 事务内（用户名占用 409 → 用户数上限 403 → 建号，首个账号为管理员）。
- 现有错误的状态码与响应体（只有一个 `error` 键）不变；邀请码 `code` 是请求中唯一新增的字段。

## 5. 前端文案（R3、R4）

- `AuthForm` 的注册模式在密码下方显示“邀请码(没有则不填)”输入框。提交时先检查：用户名为空 →“请输入用户名”，密码为空 →“请输入密码”；
  注册时随请求提交 `code`。
- 后端错误映射（`utils/authErrors.js`，只有一个写入点）：

| 后端（状态 + `error`） | 显示 |
| --- | --- |
| 400 `username and password are required` | 请输入用户名和密码 |
| 400 `username must be at least 5 characters` | 用户名不能低于5位 |
| 400 `password must be at least 8 characters` | 密码不能低于8位 |
| 400 `password must be at most 72 bytes` | 密码不能超过72字节 |
| 400 `username is reserved` | 用户名不能为非法字符 |
| 400 `username may contain only letters and numbers` | 用户名只能由字母和数字组成 |
| 409 `username already exists` | 用户名已被占用 |
| 401 `invalid username or password` | 用户名或密码错误 |
| 400 `invite code required` | 请输入邀请码 |
| 403 `invalid invite code` | 邀请码错误 |
| 403 `user limit reached` | 超过用户数上限 |
| 413 | 请求内容过大 |
| 429 `RATE_LIMITED` | 请求过于频繁，请稍后再试 |
| 其它 | 后端原文；都没有时显示“请求失败” |

- 管理员界面：`USER_LIMIT_REACHED` 显示“超过用户数上限”，其它错误保持现状（O1）。

## 6. 文档

- `README.md`：在 General 环境变量表里加入两个变量，并写明与上游 `READER_APP_INVITECODE` / `READER_APP_USERLIMIT` 的对应关系；“Exposing OpenReader to the internet”第 3 条改为
  建议设置邀请码；FAQ 同步。
- `docker-compose.yml`：传入 `OPENREADER_INVITE_CODE` 与 `OPENREADER_USER_LIMIT`，默认为空，即不改变现有部署。

## 7. 测试计划（先红后绿）

后端（`backend/api`）：
- G1 设置邀请码：不带 `code` 返回 400，`code` 错误返回 403，两种情况都不建行；`code` 正确时注册成功。
- G2 设置邀请码时，用已存在的用户名且不带 `code` 注册，返回 400 邀请码错误，而不是 409。
- G3 未设置邀请码：带不带 `code` 都能注册（对照）。
- G4 上限为 2：前两个注册成功，第三个返回 403，表里只有 2 行，第三个用户名没有会话；已有账号照常登录（对照）。
- G5 管理员新增：达到上限返回 403 `USER_LIMIT_REACHED`，不建行；未达到上限时返回 201（对照）。
- G6 上限为 3 时并发 8 个注册，最终只有 3 行。
- G7 配置：默认 50；`0`、`-5` 夹到 1；`99` 夹到 50；`abc` 按 50；空白邀请码视为未设置。
- G8 任何注册响应都不包含配置的邀请码。

前端（`node --test`）：
- F1 `AuthForm` 只在注册模式渲染邀请码输入框，标签为“邀请码(没有则不填)”；只在注册时把 `code` 传给 store。
- F2 store 与 API：注册请求体为 `{username, password, code}`，登录请求体不含 `code`。
- F3 `utils/authErrors.js` 的映射表逐项覆盖，包括空值提示与兜底。
- F4 管理员界面把 `USER_LIMIT_REACHED` 映射为“超过用户数上限”。

浏览器：真实 Go 服务（设置邀请码，上限为 2）加 Chromium，在 1440×900、390×844、360×800 三个视口下检查：
- 注册模式出现邀请码输入框，登录模式没有；
- 不填邀请码提示“请输入邀请码”，填错提示“邀请码错误”，填对后登录成功；
- 用户名太短提示“用户名不能低于5位”，重名提示“用户名已被占用”；
- 达到上限后提示“超过用户数上限”。

## 8. 明确不重开

- 已签收的认证会话、JWT 与实例签名密钥、16 KiB 请求边界、bcrypt 72 字节、通用 401、首个账号为管理员、管理员鉴权、`/login` 与 `AuthDialog` 的结构和重新认证流程。
- 用户管理表格的布局与其它操作（`user-management-p2-contract.md`）。

## 9. 状态

**contract-only**。

## 10. 补充：`/login` 卡片的窄屏溢出（R6，2026-11-12）

实现后，浏览器合同在 360×800 报邀请码输入框超出视口。逐项测量（全新数据目录，注册模式）：
- 390×844 与 360×800 下，`.auth-card` 都是 x 20、宽 400、右缘 420，分别超出视口 30px 和 60px；
- 用户名、密码、邀请码和按钮的表单项都是 x 56 到 384，所以在 360 下全部越过右边。

原因：`.auth-card` 是 `width: 400px; max-width: 100%`，而 `.auth-page` 的 grid 列宽按内容撑开，`max-width: 100%` 相对的正是这 400px，没有起到限制作用。
这是 `/login` 页原有的缺陷，不是邀请码输入框引入的：没有邀请码框时，用户名、密码和按钮同样越界。上游登录是 `App.vue` 中按 `dialogWidth`
自适应的对话框，`/login` 页属于 OpenReader 的 JWT 适配；页面可以不同，但不能在窄屏上被裁掉。

- 目标：`.auth-page` 的 grid 列为 `minmax(0, 1fr)`，卡片宽度不超过页面内容区，桌面仍是 400px；对话框 `AuthDialog` 已有 `max-width: calc(100vw - 28px)`，不改。
- 测试：浏览器合同在三个视口都检查卡片和每个表单项（用户名、密码、邀请码、按钮）落在视口内（右缘不超过视口宽度加 1px），并在当前构建上先红。
