# 认证请求每次两次 SQLite 提交的节流（P2）合同

日期：2026-11-04。分支 `claude/throttle-per-request-auth-writes`，基于 `claude/sqlite-immediate-write-transactions`（`5bfb0b8`，已推送、未合入
main）。合入顺序：先合入该分支，再合入本分支。

## 1. 背景与范围

[`sqlite-immediate-write-transactions-p1-contract.md`](sqlite-immediate-write-transactions-p1-contract.md) 第 3 节第 3 条核实：每个认证请求
提交两次 SQLite 写入。

- 处理前：`authsession.Service.renew` 执行 `UPDATE user_sessions SET last_seen_at, expires_at`（`a0edce3` 起）。旧 JWT 走 `adoptLegacy` 的写事务。
- 处理后：`middleware.TrackActivity` 执行 `UPDATE users SET last_active_at`（首个提交 `131d3f8` 起）。

该合同第 5 节方案 C 记录了两点。第一，节流不是并发保存失败的修复，那由 `_txlock=immediate` 解决。第二，两次写入都去掉后，并发 `GET` 的
p50 从 2.8–3.3 ms 降到 1.8–2.2 ms。它在第 10 节把节流留作单独的性能切片，就是本切片。

本切片只改这两处写入的频率。以下不变：

- 七天会话 TTL、每用户 64 个会话上限、旧 JWT 收编窗；
- 登录、注册、退出、改密与删用户的行为；登录与注册对 `last_active_at` 的写入；
- 接口路径、请求、响应、状态码与错误文案；
- 表结构（没有迁移）、WAL、连接池、busy_timeout、`_txlock=immediate`。

## 2. 上游行为（`fa22f27`）

- `src/main/java/com/htmake/reader/verticle/RestVerticle.kt#start`：Vert.x `SessionHandler.create(LocalSessionStore.create(vertx))`，
  `setSessionTimeout(7L * 86400 * 1000)`，cookie `reader.session`。会话保存在内存里，每次访问滑动，不写磁盘。
- `src/main/java/com/htmake/reader/api/controller/BaseController.kt#checkAuth`：
  - 会话里已有 `username` 时直接返回 `true`，不读也不写用户存储。
  - 没有会话时才按 `accessToken=username:token` 自动登录。与 `user.token` 相同的当前 token 直接通过。`token_map` 里未过期的历史 token
    把到期时间改成当前时间后 `loginExpireDays`（7）天（源码注释“延长有效期”），过期的删除。
  - 自动登录成功后调用 `saveUserSession(context, userMap, existedUser, false)`：写 `last_login_at = now`、`saveStorage("data", "users")`，
    再把 `username` 放进会话。
  - 所以上游只在一个浏览器会话的第一次请求（token 自动登录）时写一次用户存储；同一会话之后的请求都不写。
- `last_login_at` 的消费者：`web/src/components/UserManage.vue` 的“上次登录”列；`src/main/java/com/htmake/reader/api/YueduApi.kt#clearUser`
  按 `autoClearInactiveUser` 天数清理用户。

OpenReader 用无状态 JWT 加 SQLite 会话行代替 Vert.x 内存会话（认证会话合同
[`authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md`](authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md)
第 8 节：Go/SQLite 技术等价）。这里没有可以跳过写入的“已建立会话”，所以 `a0edce3` 让每个请求都续期。

节流后的写入频率介于两者之间：每个会话最多每分钟写一次，比上游的每个浏览器会话一次更细，也比现在的每个请求一次更接近上游。

该合同第 3.1 节写的是：每次成功的 REST、WebDAV Bearer 和 WS 握手认证“都可把到期时间续至该次认证后七天”。第 2 节写的是“成功活动滑动续期
七天”。节流保留滑动续期，误差上限 60 秒（第 6 节目标 1）。

## 3. 当前行为（`5bfb0b8`，已核实）

1. **认证。** `Authenticate` 由 REST `AuthRequired`、WebDAV Bearer 和 `/ws/sync` 握手共用。它依次解析 JWT、读 `users(id, auth_version)`，
   再调用 `renew`：

   ```sql
   UPDATE user_sessions SET last_seen_at = now, expires_at = now + 7d
   WHERE id = ? AND user_id = ? AND auth_version = ? AND revoked_at IS NULL AND expires_at > now
   ```

   `RowsAffected != 1` 时返回 401。这一条语句同时完成会话存活、代次与期限的检查和续期，所以每次认证都提交一次。
   旧 JWT 的 `adoptLegacy` 是一个写事务：先插入（冲突时什么也不做），再按同样条件更新，最后修剪。
2. **活动。** `TrackActivity` 挂在 `/api` 受保护组和 WebDAV 组上，WebDAV Basic 认证也经过它。处理后执行
   `UPDATE users SET last_active_at = time.Now()`（本地时区），每个请求提交一次。`/ws/sync` 不经过它。
3. **实测。** 测试二进制给每个 SQLite 连接装 commit hook 计数（第 7 节 A1）。注册后几秒内连续三轮：
   - `GET /api/me`：每次提交 2 个写事务；
   - WebDAV Bearer `GET /webdav/`：每次 2 个；
   - WebDAV Basic `GET /webdav/`：每次 1 个。

   `/ws/sync` 握手按代码阅读是 1 个（只有续期）。
4. **影响。** 在 `_txlock=immediate` 下，这两次提交各取一次全库写锁，所以并发的 `GET` 与真正的写者（如 `PUT /api/books/:id`）互相排队。

## 4. 方案

**选定：按上次写入时间节流，每次先读检查，过期才写。** 间隔 60 秒，代码里是 `authsession.RenewInterval` 和 `middleware.ActivityInterval`。

- **会话。** `renew` 先按与原来相同的条件读出 `last_seen_at`：
  `SELECT … WHERE id AND user_id AND auth_version AND revoked_at IS NULL AND expires_at > now`。
  - 读不到行：401。
  - `now − last_seen_at < 60 s`：通过，不写。
  - 否则执行原来的条件更新；`RowsAffected != 1` 时返回 401。
- **旧 JWT。** 已收编、未撤销、未过期并在 60 秒内续过期的行直接通过。其余情况走原来的收编事务，包括尚未收编、已撤销的墓碑和已满 60 秒的行。
- **活动。** `TrackActivity` 在处理后读 `users.last_active_at`，在 Go 里按时刻比较。只有存储值为空、零值或无法解析，或者已满 60 秒，
  才写入 `time.Now()`。用户已不存在时不写。

理由：

- **检查不缓存。** 每个请求仍读用户行和会话行。退出、改密、删用户和过期提交后，下一个请求立即被拒绝。
- **不用只在过期时更新的条件 UPDATE。** 一条没有匹配行的 UPDATE 照样取写锁并提交（GORM 默认事务加 `BEGIN IMMEDIATE`；裸 UPDATE 语句
  同样以写事务开始），省不掉锁。读检查在 WAL 下不取写锁。
- **不在 SQL 里比较 `last_active_at`。** mattn 按值所带的时区写文本：`TrackActivity` 和管理员建用户用本地时区，登录和注册用 UTC。
  SQLite 按文本比较时会差出整个时区差，例如存储值是 UTC+8 时，UTC 截止时刻会让它显得新了 8 小时。所以读出后由驱动解析成时刻再比较。
  `user_sessions` 一直只写 UTC，原有的 `expires_at > now` 文本比较保持不变。
- **不用进程内缓存（如 `map[userID]time.Time`）。** 缓存看不到重启、同库的其他服务实例（测试里很常见）和登录写入，还要另外管理内存。
  按主键读一行只要微秒级。
- **为什么是 60 秒。** 管理页“上次登录”按分钟显示（`OverlayUserManagement.vue#formatUserTime`）。60 秒只占七天的约 0.01%。
  节流后，每个活跃会话最多每分钟续期写入一次，每个活跃用户最多每分钟活动写入一次。

## 5. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| W1 | 每个认证请求（REST、WebDAV Bearer、WS 握手）都提交会话续期；上游只在 token 自动登录时写 | `must-fix`（性能） | 60 秒内的后续认证只读不写 |
| W2 | 每个认证请求（含 WebDAV Basic）都提交 `last_active_at`；上游只在登录与自动登录时写 `last_login_at` | `must-fix`（性能） | 60 秒内只读不写 |
| W3 | 成功认证后，到期时间至少是该次认证后 7 天减 60 秒，而不是正好 7 天 | `acceptable-change`（Go/SQLite 运行时适配；上游只在自动登录时延长） | 记录于此 |
| W4 | `last_active_at`（管理页“上次登录”）最多比最后一个请求早 60 秒；登录、注册的写入仍然精确 | `acceptable-change`（上游只在登录与自动登录时写，更粗） | 记录于此 |
| W5 | 64 个会话上限按 `last_seen_at` 修剪，排序精度变为 60 秒 | `acceptable-change` | 记录于此 |
| W6 | 每个认证请求多读一次 `user_sessions` 主键行；处理后多读一次 `users.last_active_at` | `acceptable-change` | 记录于此 |
| W7 | 退出、改密（代次）、删用户与过期的检查 | 不变 | 每个请求仍读库判断 |

## 6. 目标行为

1. **续期。** 同一会话在上次续期（`last_seen_at`）后 60 秒内的认证，不提交任何写事务。满 60 秒后的第一次认证用原来的条件更新续期一次。
   任何一次成功认证之后都满足：`expires_at ≥ 认证时刻 + 7 天 − 60 秒`，且 `expires_at = last_seen_at + 7 天`。
2. **检查顺序不变。** 签名与 claims → 用户存在 → 代次 → 会话行存在、未撤销、代次一致且 `expires_at > now`。任何一步不满足都返回
   `ErrInvalidSession`（入口原有的 401），与是否在 60 秒内无关。
3. **并发。** 续期只条件更新现存行，`RowsAffected != 1` 时返回 401。退出、改密或删用户在读检查与更新之间提交时，这次请求返回 401，与现在一样。
   不写的路径不会延长或重建已撤销的行。读检查早于退出提交的请求照常完成，与现在续期早于退出提交的请求一样。
4. **旧 JWT。** 收编窗内，60 秒内续过的已收编行直接通过；其余情况走原来的收编事务，撤销墓碑仍被拒绝。
5. **活动。** 处理后，只有存储值为空、零值或无法解析，或者已满 60 秒，才写入 `time.Now()`。比较按时刻进行，与存储的时区无关。
   用户已删除时不写。登录、注册和管理员建用户的写入不变。
6. **结果。** 注册、登录或上次续期后 60 秒内，`GET /api/me`、WebDAV Bearer 和 Basic 的 `GET /webdav/` 每次 0 个写事务。
   之后的第一个 Bearer 请求恰好 2 个（续期与活动），Basic 恰好 1 个；随后又是 0 个。

## 7. 计划测试

**红测**（在 `5bfb0b8` 上必须失败）：

- A1 `backend/api/authenticated_request_commit_contract_test.go#TestAuthenticatedRequestsWithinAMinuteCommitNothing`。测试二进制在 `init`
  里给每个 SQLite 连接装 commit hook，按数据库文件计数。SQLite 只在写事务提交时调用它，包括没有改行的 `BEGIN IMMEDIATE` 和 UPDATE，
  读不调用。注册后三轮 `GET /api/me`（200）、WebDAV Bearer 与 Basic `GET /webdav/`（207），每个请求期望 0。
- A2 同文件 `TestAuthenticatedRequestAfterAMinuteRenewsSessionAndActivityOnce`：把会话和 `last_active_at` 调到两分钟前。第一个
  `GET /api/me` 期望恰好 2 个提交，且 `last_seen_at`、`expires_at`、`last_active_at` 前进；第二个期望 0。
- U1 `backend/services/authsession/renew_interval_contract_test.go#TestAuthenticateRenewsSessionOnlyOnceAMinute`：注入时钟。签发后 +59 秒
  认证不改行；+60 秒续期到 `last_seen_at = +60 s`、`expires_at = +60 s + 7 天`。之后每 30 秒认证一次、共 10 分钟，每次
  `expires_at ∈ [t + 7d − 60s, t + 7d]`。
- U2 同文件 `TestLegacySessionRenewsOnlyOnceAMinute`：收编后 +59 秒不改行，+60 秒续期；撤销后仍被拒绝。
- M1 `backend/middleware/activity_contract_test.go#TestTrackActivityRewritesLastActiveAtOnlyOnceAMinute`，用真实时钟：
  - 存储值为 30 秒前（UTC、UTC+8、UTC−5 三种文本时区）：不改；
  - 两分钟前（UTC+8、UTC−5）、零值、NULL：改写为本次请求的时刻；
  - 用户不存在：不建行。

**对照**（新旧实现都通过）：

- U3 同包 `TestAuthenticateWithinAMinuteStillRejectsRevokedResetExpiredAndDeletedSessions`：60 秒内退出、`auth_version` 变化、
  `expires_at` 已过、用户已删除，都返回 `ErrInvalidSession`。
- U4 同包 `TestConcurrentRenewAfterAMinuteCannotResurrectRevokedSession`：会话已满 60 秒，16 个并发认证全都走续期更新，同时退出。
  之后认证失败、行数为 0（含 `-race`）。
- 既有测试。authsession：`TestIssueAuthenticateRenewAndRevokeSession`、`TestSessionRetentionCapAndExpiry`、
  `TestLegacySessionAdoptionWindowAndRevocationTombstone`、`TestConcurrentRenewCannotResurrectRevokedSession`。
  api：`TestAuthenticatedSessionLogoutRevokesOnlyCurrentLogin`、`TestDeletedUserTokenFailsBeforeRESTAndWebDAVSideEffects`、
  `TestPasswordResetRevokesExistingSessions`、`TestExpiredSessionFailsAcrossRESTWebDAVAndWebSocket`。它们都在注册后几秒内运行，
  也就是走 60 秒内不写的路径，用来证明检查仍然精确。

**T3 夹具更新**（`backend/api/book_patch_concurrent_commit_contract_test.go`，来自 immediate 切片）。

T3（`TestBookPatchSurvivesConcurrentSessionRenew`）和 C3（`commit-before-transaction`）用 fixture 自己的 token，在 `PUT` 事务里或事务前
再认证一次，模拟另一个请求提交。节流后，这个刚续过的会话 60 秒内不再提交，两个测试都会变成空测。

改为：同一用户第二次登录得到另一个会话，把它的 `last_seen_at` 调到两分钟前，让“另一个请求”一定提交。再断言该会话的 `last_seen_at`
确实前进，证明真的发生过写入。改后在 `5bfb0b8` 和本分支上都通过；把 `db.go` 换回 deferred 时，T3 仍应失败。

## 8. 回归与测量

- `go test ./...`、`go vet ./...`、`go test -race ./api ./db ./services/...`（api 加 `-timeout 40m`）。绑定端口的测试在沙箱外运行。
  前端没有改动。
- 压力：用 immediate 切片的 `book-put-stress.mjs`（`MODE=follow|cover`、`CONCURRENT_KIND=get|setting-put`），前（`5bfb0b8`）后
  （本分支）两个二进制交替运行。记录 `PUT` 与并发请求的 p50、p95、最大值和失败数。脚本在注册后几秒内跑完，所以“后”的并发 `GET`
  都走不写的路径。

## 9. 明确不重开

- 七天 TTL、64 个上限、旧 JWT 收编窗规则；退出、改密与删用户的语义；401 文案。
- 登录写 `last_active_at` 的方式；`TrackActivity` 用本地时区写入、不带请求 context；90 天清理；`lastLoginAt` 与 `lastActiveAt` 别名。
- WAL、连接池、busy_timeout、`_txlock=immediate`；已有的重试与先写绕行（另有切片）。
- WebSocket 连接建立后不随会话撤销断开（既有行为）。

## 10. 状态

**contract-only**。
