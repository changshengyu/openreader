# 认证会话生命周期固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前实施基线：`OpenReader@a0edce3`

审查日期：2026-08-26
状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

## 1. 范围与结论

本轮只复审登录后凭据的签发、验证、续期、退出、改密与删用户生命周期，以及三种 Bearer 入口：

- `/api/*` 受保护 REST；
- `/webdav/*`、`/reader3/webdav/*` 的 Bearer 分支；
- `/ws/sync?token=...`。

固定上游为每次登录生成 token，把 token 和七天到期时间写入当前用户 `token_map`；自动登录只接受
未到期 token，并将同一 token 的到期时间续至当前时间后七天；退出删除当前 token。账号不存在时没有
可供 token 查找的用户记录。OpenReader 使用 JWT 是允许的 Go/多用户适配，但不能把“签名仍正确”当成
永久、与账号状态无关的会话。

当前实现存在四个必须修复的差异：

1. `middleware.ParseToken()` 使用 `WithoutClaimsValidation()`，签发的 JWT 也没有 expiry，因而签名密钥
   不变时凭据永久有效。
2. REST `AuthRequired()` 只校验签名和 `userId`，不校验 User 是否仍存在；WebDAV Bearer 分支同样只
   调用 `ParseToken()`。部分 handler 不先加载 User，删除账号后的旧 JWT 可重新写入孤立 user-owned 行。
3. 前端退出只删除浏览器 localStorage；服务端没有撤销动作，同一 token 的其它副本继续有效。
4. 管理员重置密码只替换 hash，不撤销已有 JWT。固定上游本身没有在 resetPassword 中清空 token map，
   但 OpenReader 的管理员改密场景应把立即撤销作为明确安全增强。

裁决：**must-fix**。实现必须先由失败合同证明上述当前行为，不得以全局 JWT secret 轮换或删除用户数据
替代单用户/单会话撤销。

## 2. 固定上游证据

| 动作 | 固定上游行为 | OpenReader 目标 |
|---|---|---|
| 登录/注册 | `saveUserSession(..., regenerateToken=true)` 生成新 token，写入 `token_map[token] = now + 7d`，清除过期项。 | 每次成功登录/注册创建独立随机会话；响应仍为 `200 {token,user}`。 |
| token 自动登录 | `checkAuth()` 先定位用户，再查当前/历史 token；未到期 token 续期七天，过期 token 删除。 | 三个 Bearer 入口先验证签名，再验证用户、认证代次和持久会话；成功活动滑动续期七天。 |
| 退出 | `POST /reader3/logout` 删除当前 token，并销毁当前 session。 | 新增 canonical `POST /api/auth/logout`，只撤销当前会话；前端在清本地状态前 best-effort 调用。 |
| 删除用户 | 用户记录和私有目录删除后，旧 token 无法再定位用户。 | 用户删除事务同时删除会话；旧 token 在 REST/WebDAV/WS 都返回通用 401，不能重建孤立行。 |
| 重置密码 | 上游替换 salt/hash，但未显式清空 token map。 | **允许安全增强**：递增认证代次并撤销目标用户全部会话；不改变密码表单或成功响应。 |

上游文件：

- `src/main/java/com/htmake/reader/api/controller/BaseController.kt#saveUserSession/checkAuth`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt#login/logout/resetPassword/deleteUsers`
- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `/reader3/logout`
- `web/src/views/Index.vue#logout`

## 3. API 与错误合同

### 3.1 签发和认证

- `POST /api/auth/login`、`POST /api/auth/register` 的路径、请求、成功状态和 `{token,user}` 不变。
- 新 token 保持 HS256 和现有 `userId` claim，并增加随机 token identity 与用户认证代次。原始 token、随机
  identity 和 Authorization header 不得写入 SQLite、日志、错误、备份或事件；SQLite 只保存不可逆
  SHA-256 identity。
- 活跃会话的 idle TTL 为七天。每次成功的 REST/WebDAV Bearer/WS handshake 身份验证都可把到期时间
  续至该次认证后七天；过期行必须在验证时失败并可清理。
- 认证顺序为签名/claims -> User 存活 -> 认证代次 -> session 存活/期限。任一步失败统一使用该入口既有
  401，不暴露用户、代次、session 或过期原因。
- WebDAV Basic 继续按当前用户名和 bcrypt 校验，不创建 JWT session，也不受 logout 影响；改密后旧
  Basic 密码自然失效。

### 3.2 退出

`POST /api/auth/logout`：

- 需要当前有效 Bearer，会话认证先于 handler；
- 无请求 body；成功返回 `204`；
- 只删除当前 token 对应的 session，当前用户的另一登录 token 保持有效；
- 重复使用已退出 token 与缺失/无效 token 均走普通 `401`；
- 不广播业务数据事件，不删除本地用户数据。

前端 `user.logout()` 必须先截取当前 token，发起 best-effort server logout，然后立即执行现有完整本地
session reset；网络失败不能阻止本机退出，也不能把 token 写入通知或日志。

### 3.3 管理员改密与删用户

- `PUT /api/admin/users/:id/password` 的 body、权限、成功 envelope 不变。密码 hash 更新、认证代次递增
  和该用户全部 session 删除必须在一个 SQLite transaction 中完成；失败不得出现“密码已改但 token
  仍在”或反向的半提交。
- `POST /api/admin/users/batch-delete` 与 inactive cleanup 的既有用户数据事务必须包含 session 行。
- 删除后，旧 token 对 `/api/settings/reader`、普通只读 REST、WebDAV Bearer 和 WebSocket 均为 401；
  不能创建 UserSetting、source namespace、文件或 Hub client。
- 用户权限/限额普通更新不递增认证代次；handler 每次仍从数据库读取当前权限，不能把旧权限写进 JWT。

## 4. 数据与升级合同

允许且仅允许以下加法持久化：

1. `users.auth_version`：非空正整数，旧行迁移为 `1`；注册用户显式从 `1` 开始。它只在密码重置时
   事务递增。
2. `user_sessions`：保存 hashed identity、`user_id`、`auth_version`、`created_at`、`last_seen_at`、
   `expires_at`。不保存 raw JWT/JTI、IP、User-Agent、密码或设备名。
3. 一个 `schema_migrations` marker 记录旧 JWT 过渡窗开始时间。

`user_sessions` 是可丢弃的运行时认证数据，不进入 ordinary/portable/Legado backup，不进入 WebDAV，
不影响 `data/cache/library` 路径。恢复用户数据不会恢复登录态；保留相同 JWT secret 仍只保留处于过渡窗
或已持久化 session 的凭据。

为避免升级时一次性登出所有用户：

- 老 JWT 指缺少新 token identity/auth-version claims、但现有 HS256 签名和 `userId/iat` 合法的凭据；
  `iat` 必须不晚于 marker 且不能位于未来，防止回滚到旧二进制后签发的新 legacy token 被再次收编；
- 首次迁移写入固定 marker，此后最多七天允许老 JWT 在账号仍存在且 `auth_version=1` 时收编为 hashed
  session，并从本次活动起获得七天 idle TTL；
- marker 七天后，尚未收编的老 JWT 必须拒绝；已收编行按自己的 idle TTL 工作；
- 改密将 `auth_version` 增为 2，因此任何尚未收编的老 JWT 也不能在过渡期内复活；
- 迁移 marker 必须幂等，重启、升级或回滚再升级不得重开七天窗口。

旧 users 表、时间字段、书架、书源、设置、缓存、备份和挂载文件不得被扫描改写。GORM migration 需有
历史卷测试，确认 auth_version 默认值和 marker/session 表只做加法。

## 5. 并发、资源和安全边界

- 签发使用 `crypto/rand` 至少 256 bit 随机 identity；数据库只保存带 domain prefix 的 SHA-256。
- 同一用户最多保留 64 个未过期 session；第 65 次成功登录在同一事务中删除最旧 session。该上限是
  防止认证表无界增长的允许安全适配，不改变正常多设备登录。
- issue、adopt、renew、logout、password reset 和 user delete 的 SQL 都必须携带 request context；续期
  只能条件更新现存且代次匹配的行，不能在 logout/delete 并发后重新插入。
- 认证成功后继续运行现有 `TrackActivity`；本轮不重新定义管理页 `lastLoginAt` 投影，也不改变 90 天
  inactive cleanup 的当前产品语义。
- 401、500、日志和 WebSocket close reason 不得包含 token、hash identity、claims、用户名、密码、
  SQLite 文本或 host path。access log 继续将 WS query 整体脱敏。
- JWT secret 的现有双默认值兼容只用于读取历史默认部署；自定义 secret 不尝试其它 key。算法继续只
  接受 HS256。

## 6. 测试先行闸门

实现前必须先提交能在 `569c75d` 失败的合同测试：

1. 登录 token 对应一条不含 raw token/JTI 的 session；两个登录 token 相互独立。
2. 过期 session 对 REST、WebDAV Bearer、WS 均返回安全 401，且不会续期或创建副作用。
3. logout 返回 204，当前 token 失效而同用户另一 token 仍可访问 `/api/me`。
4. 管理员改密后旧 token 失效，新密码可登录；hash、auth_version 和 session 删除具备 rollback 测试。
5. 删除用户后旧 token 不能 GET `/api/sources`、PUT `/api/settings/reader`、访问 WebDAV 或注册 WS；
   UserSetting/session/文件/Hub 均无孤儿副作用。
6. legacy token 在迁移七天内可被收编并续期，窗口后未收编 token 失败；重启不重置 marker，改密后的
   legacy token 在窗口内也失败。
7. 64-session 上限、过期清理、同 token 并发续期和 logout/renew 竞争稳定且 race 通过。
8. AutoMigrate 历史 users 行不变，仅得到 `auth_version=1` 和幂等 marker；ordinary/portable backup 不含
   session 或 token identity。
9. 前端测试证明 logout API 使用截取 token、失败仍清空完整本地账号状态，并且 late 401 不能清除新
   登录 token。

## 7. 实施与发布闸门

顺序固定为：本合同 -> 旧实现红测 -> 实现。实现后至少运行：

```bash
cd backend && go test ./...
cd backend && go test -race ./middleware ./services/authsession ./api
cd backend && go vet ./...
cd frontend && npm test
cd frontend && npm run build
```

另需真实 HTTP 验证双登录/logout、改密、删用户 stale-token、WebDAV Bearer 和 WS；真实浏览器在
1440x900、390x844、360x800 验证用户空间退出、重新登录及 Reader/session suspend。发布前通过
fresh/historical/portable/restart mounted-volume 门，再由受信 release workflow 发布 amd64/arm64，回拉
不可变标签确认 health revision、session 401/204 和旧数据可读。

## 8. 非目标与允许差异

- 不改登录/注册表单、账号密码规则、JWT secret 配置、CORS、rate limit、Reader 恢复状态机或用户管理
  可见布局。
- 不新增 refresh-token/cookie/OAuth/device-name UI；Bearer localStorage 是当前技术栈合同。
- 管理员改密立即撤销全部 token、hashed session identity 和 64-session cap 是明确安全增强。
- 七天滑动 session 是固定上游语义；JWT payload 不直接携带可续期 expiry，而由 SQLite 权威 session
  行控制，是 Go/SQLite 技术等价实现。

## 9. 实施与发布证据

合同 `8db7c85`、旧实现红测 `2396537` 与实现 `a0edce3` 已按固定顺序落地。实现增加随机 hashed session
identity、七天滑动续期、64-session 上限、旧 JWT 幂等收编窗、当前会话 logout、改密全撤销，以及
REST/WebDAV Bearer/WebSocket 共用的用户与 session 存活校验；普通/portable 备份继续排除认证运行态。

本地最终代码通过 Go full、20 分钟上限的 middleware/authsession/api race、vet、frontend 742/742、生产
build、Compose、四视口 logout/重新认证浏览器合同和真实 API 双登录/logout/改密/删用户三入口合同。
受信 GitHub Actions run `32914105929` 又通过 backend/frontend/Compose、native image、fresh portable 与
historical volume 门禁，并发布 `ghcr.io/changshengyu/openreader:a0edce3` 和 `latest`。amd64/arm64 OCI index
为 `sha256:5d7fe23ba96107c5c545e9e44815514fe277e5a6f83eb25cb006859c5d515d78`；对应 manifest 为
`sha256:dc50d7db9eb1bddb8c3a094fd507fa1e1f4631b633cacc58d635070b1ba606ab` 和
`sha256:1785dee2ec3d62894a20b35c33535fc1bd2c9167585b12bc567b71c71bff13df`。

从 GHCR 回拉不可变标签后，health 报告完整 revision `a0edce35c928813e33c15881016c2450b0881669`；真实
API 再次通过 logout `204/401`、另一会话存活、改密/删用户旧 token 401，以及 REST/WebDAV/WS 拒绝。
容器日志中的 WebSocket query 为 `?<redacted>`，GORM SQL 使用参数占位符。剩余工作仅为用户真实设备
验收和后续长尾固定基准复审；不把 Docker/自动化验证等同于生产实例已经升级。
