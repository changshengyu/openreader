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

- U3 同包 `TestAuthenticateWithinAMinuteStillRejectsRevokedResetExpiredAndDeletedSessions`：60 秒内退出、用户 `auth_version` 变化、
  会话行代次不同、`expires_at` 已过、用户已删除，都返回 `ErrInvalidSession`。
- U4 同包 `TestConcurrentRenewAfterAMinuteCannotResurrectRevokedSession`：会话已满 60 秒，16 个并发认证与退出竞争。先读到旧续期时间的
  认证执行续期条件更新；续期提交后才读的只做检查。之后认证失败、行数为 0（含 `-race`）。
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

**contract-only**（由第 11 节记录取代）。

## 11. 记录（2026-11-04）

提交：

- 合同 `dccc96d`；
- 红测与 T3/C3 夹具 `b312782`；
- 实现 `b3ea517`：`services/authsession/service.go` +45/−3，`middleware/auth.go` +22/−1。

**红测在 `5bfb0b8` 上的失败原文**（对照全部通过）：

- A1：三轮都失败，例如 `REST GET /api/me (round 1) committed 2 SQLite write transactions within a minute of the session's last
  renewal, want 0`。WebDAV Bearer 每次 2 个，WebDAV Basic 每次 1 个。
- A2：`GET /api/me right after a renewal committed 2 SQLite write transactions, want 0`。两分钟后的第一个请求恰好 2 个，这一步通过。
- U1：`authentication 59s after issue renewed the session: last_seen_at=+59s expires_at=+168h0m59s, want +0s and +168h0m0s`。
- U2：`legacy authentication 59s after adoption renewed the session: last_seen_at=+59s expires_at=+168h0m59s, want +0s and +168h0m0s`。
- M1：三个“30 秒前”子测（UTC、UTC+8、UTC−5）失败，例如 `last_active_at stored 30 seconds ago in UTC+8 = {Time:2026-11-04
  13:27:14.930913 +0800 +0800 Valid:true} after a request, want it kept at 2026-11-04 13:26:44.929924 +0800 UTC+8`。
  两分钟前、零值与 NULL 子测通过。
- 通过的对照：U3 的 5 种情况、U4、缺失用户、既有会话测试，以及新夹具下的 T3/C3。

实现后同一批断言全部通过；新增测试 `-count=10` 稳定。

**T3 夹具核验。** 用两棵临时导出树，都是 `b3ea517` 加上 `0f5e6b1` 的 deferred `db.go`：

- 用新夹具时，T3 的 `follow`/`cover` 都失败：`PUT … while another authenticated request renewed its session = 500
  {"error":"failed to update book"}, want 200`。这与 immediate 合同第 12 节记录的红测相同。
- 换回 `5bfb0b8` 的原夹具时，T3 通过。可见节流后原测试确实成了空测，夹具更新是必要的。
- C3 在两棵树上都通过：续期提交在事务开始之前，本来就不冲突。

**回归：**

- `go vet ./...` 通过；`go test ./...` 31 个包通过（api 119.2 s，沙箱外运行）。
- `go test -race ./api ./db ./services/...`（`-timeout 40m`）：26 个包通过，无数据竞争（api 1055.1 s，沙箱外运行）。
- 前端、依赖公告门与发布门没有重跑：本切片不改前端和依赖（`go.mod`、`go.sum`、`package-lock.json` 不变）。合入 main 时由
  GitHub Actions 运行。

**压力。** 用 `book-put-stress.mjs`，`5bfb0b8` 与 `b3ea517` 两个二进制交替各跑 3 轮，每轮交换先后顺序，每次起全新服务。
表中 p50、p95 是 3 轮的范围，最大值取 3 轮最大，单位 ms：

| 场景 | 构建 | `PUT` p50 / p95 | 并发请求 p50 / p95 / 最大 | 失败 |
| --- | --- | --- | --- | --- |
| 追更 + 4 个 `GET` | 前 | 3.8–3.9 / 7.0–7.3 | 2.8–3.0 / 6.5–6.7 / 13.4 | 0 |
| 追更 + 4 个 `GET` | 后 | 2.3–2.4 / 3.3–3.5 | 1.9–2.0 / 3.0–3.1 / 5.7 | 0 |
| 封面 + 4 个 `GET` | 前 | 4.4–4.9 / 7.7–8.5 | 3.1–3.4 / 7.0–7.2 / 21.4 | 0 |
| 封面 + 4 个 `GET` | 后 | 3.3–3.6 / 4.3–5.1 | 2.2–2.4 / 3.4–3.7 / 5.1 | 0 |
| 追更 + 4 个设置写 | 前 | 2.4–2.5 / 6.8–7.3 | 4.3–4.4 / 11.7–12.9 / 15.1 | 0 |
| 追更 + 4 个设置写 | 后 | 2.0–2.2 / 4.4–4.7 | 2.9–3.1 / 6.0–6.6 / 13.5 | 0 |
| 封面 + 4 个设置写 | 前 | 3.1–3.9 / 7.5–7.8 | 4.3–4.7 / 11.4–13.1 / 25.8 | 0 |
| 封面 + 4 个设置写 | 后 | 3.3–3.6 / 5.7–6.5 | 3.6–3.9 / 7.0–7.4 / 14.2 | 0 |

- 并发 `GET`：p50 从 2.8–3.4 ms 降到 1.9–2.4 ms，p95 从 6.5–7.2 ms 降到 3.0–3.7 ms。这与 immediate 合同第 5 节方案 C 的实验
  （p50 1.8–2.2 ms）一致。
- 并发设置写本身仍是写者，它的 p95 从 11.4–13.1 ms 降到 6.0–7.4 ms：每个请求少了两次排队取锁。
- 无并发阶段两者相同（追更 `PUT` p50 前 1.6–1.9、后 1.8–1.9 ms；封面前 2.5–2.8、后 2.4–2.6 ms）。WAL 加 `synchronous=NORMAL`
  下单次提交本身很便宜，收益来自不再排队等全库写锁。
- 两个构建都是 0 失败，服务端都没有 `database is locked` 日志。并发保存的正确性由 immediate 切片保证。

状态 **implementation-complete / regression-validated**。分支已推送，未合入 main（须在 `claude/sqlite-immediate-write-transactions`
之后合入）。镜像未发布，生产未访问。
