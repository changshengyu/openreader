# 认证会话：旧格式 JWT 收编与公开默认密钥合同（P0）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前实施基线：`OpenReader@a1a1795`（`origin/main`）

审查日期：2026-10-22
状态：**implementation-complete / regression-validated / Docker-published**（证据见第 13 节，发布见第 13.6 节）

前置合同：[`authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md`](authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md)
（下称“会话合同”）。本合同只收紧会话合同第 4 节“旧 JWT 过渡窗”的适用条件，并补充第 5 节“双默认值兼容”
对旧格式 token 的边界；会话合同其余条款不重开。

## 1. 范围与结论

会话合同为避免升级时一次性登出所有人，设计了七天旧格式 JWT 收编窗：没有 `jti`、没有 `authVersion` 的
HS256 token，只要签名能验证、`iat` 不晚于 marker、账号 `auth_version=1`、当前时间在 marker 后七天内，就
收编为一条 hashed session。这个设计默认“签名能验证”等于“本实例以前签发过这个 token”。下面两个条件让
这个前提失效：

1. **验证密钥是公开值。** `OPENREADER_JWT_SECRET` 未设置时，`config.Load()` 使用仓库里公开的
   `change-this-before-deploy`；标准 `docker-compose.yml` 不设置这个变量，`.env.example` 也直接写着这个值，
   所以多数部署实际在用公开值。`compatibleSecrets()` 在公开值下还会接受另一个公开旧默认值
   `change-me-in-production`。
2. **账号晚于 marker。** `initializeAuthenticatedSessionMigration()` 在 marker 不存在时写入当时时间。全新
   安装首次启动就写下 marker，之后注册的账号（包括第一个、即管理员账号）从未被旧二进制签发过旧格式
   token，但收编条件不看账号创建时间。

两者叠加，任何人都能用公开密钥签一个 `{"userId":1,"iat":<2020 年的时间戳>}`，在全新安装首次启动后七天内
拿到第一个注册账号（管理员）的会话。另一会话已用临时 Go 测试复现（未提交、已删除）：
`setupTestServerWithConfig` 把 `JWTSecret` 设为 `change-this-before-deploy`，注册首个用户，再带伪造 token
请求 `GET /api/me`，返回 `200` 且 `role=admin`。REST `AuthRequired`、WebDAV Bearer（`/webdav/*`、
`/reader3/webdav/*`）和 `/ws/sync` 三个入口都调用同一个 `authsession.Service.Authenticate()`，受同样影响。

暴露面有两类：

- **全新安装 + 公开默认值**：首次启动后七天内，管理员及所有账号可被接管（上面复现的情况）。
- **从早于 `a0edce3`（2026-08-26，引入会话迁移）的镜像今天才升级 + 公开默认值**：升级后七天内，升级前已有
  的所有账号可被接管。

裁决：**must-fix（P0）**，按第 5 节修复。

新格式会话不受影响：它必须带 256 bit 随机 `jti`，其 SHA-256 摘要必须存在于 `user_sessions` 且与账号认证
代次匹配。签名密钥公开与否不改变这一点，用公开密钥伪造的新格式 token 因为 `jti` 不在库里而失败。本合同
不重开新格式会话。

## 2. 当前实现证据

| 位置 | 当前行为 |
| --- | --- |
| `backend/config/config.go#Load` | `JWTSecret: env("OPENREADER_JWT_SECRET", "change-this-before-deploy")`，未设置即公开值。 |
| `docker-compose.yml`、`.env.example` | Compose 只设置 `OPENREADER_TRUSTED_PROXIES`；`.env.example` 写 `OPENREADER_JWT_SECRET=change-this-before-deploy`。 |
| `backend/services/authsession/service.go#Authenticate` | `jti` 与 `authVersion` 都缺失时进入旧格式分支：只检查 `auth_version == 1`、`iat <= marker`、`now <= marker + 7d`，随后 `adoptLegacy`。不检查验证密钥是否公开，也不检查账号创建时间。 |
| `backend/services/authsession/service.go#compatibleSecrets` | 配置为任一公开默认值时两个公开值都参与验证；自定义值只用自身。 |
| `backend/db/db.go#initializeAuthenticatedSessionMigration` | marker 不存在时写入 `time.Now().UTC()`，`OnConflict DoNothing` 保证幂等、不重开窗口。全新安装时它早于所有账号。 |
| `backend/api/auth.go#register`、`admin.go` | 账号只由注册和管理员创建两条路径写入，`created_at` 由 GORM 在服务端赋值；`User.CreatedAt` 自初始提交 `131d3f8` 起就存在。备份/恢复不创建账号行。 |
| `backend/middleware/auth.go`、`webdav_auth.go`、`backend/api/sync.go` | 三个 Bearer 入口都把 token 交给同一个 `Authenticate`；`RegisterRoutes` 总是注入会话服务。 |
| `docs/security-review-checklist.md` “Authentication and authorization” | 写着 `OPENREADER_JWT_SECRET is required`；代码有公开默认值，主分支 README 写“标准 Compose 部署无需修改”。 |

## 3. 固定上游

固定上游没有 JWT：reader-dev 为每次登录生成随机 token，写入用户的 `token_map`（会话合同第 2 节），校验时
按 token 查表，不存在“由签名推断来源”的路径。因此本问题不是上游行为差异，而是 OpenReader 自有过渡设计的
安全缺口；修复属于“安全修复”类允许差异。

## 4. 差异分类

| 编号 | 内容 | 分类 |
| --- | --- | --- |
| F1 | 配置为公开默认值（或空值）时，旧格式分支仍信任签名。 | **must-fix（P0）** |
| F2 | 旧格式收编不要求账号早于 marker；全新安装的所有账号、升级后才注册的账号都在收编条件内。 | **must-fix（P0）** |
| F3 | 同一密钥派生的资源 capability（第 7 节）在公开值下同样可被第三方构造。 | **must-fix（P1）**，方向见第 8 节，另起切片 S2，本切片不改 |
| F4 | 安全检查清单写“必填”、主分支 README 写“无需修改”、`.env.example` 直接给出公开值，与代码和风险不一致。 | **must-fix（文档）**：检查清单在本切片记录时更正；README 按第 10 节处理 |
| O1 | 会话合同第 4 节写“已收编行按自己的 idle TTL 工作”，实际上旧格式 token 每次请求都重走窗口检查，窗口结束后已收编行同样被拒绝。 | 观察：实现比合同更严格，本切片不改，只记录 |
| O2 | `middleware.ParseToken` / `GenerateToken` 在未注入会话服务时只验签名、永久有效。生产路由总是注入会话服务，只有中间件单测走这条路径。 | 观察：生产不可达，本切片不改 |

## 5. 目标行为

### 5.1 修复（本切片）

- **T1 公开密钥下不收编旧格式 token。** 服务配置的密钥去掉首尾空白后为空、`change-this-before-deploy` 或
  `change-me-in-production` 时，旧格式 token（`jti` 与 `authVersion` 都缺失）一律按无效会话处理：在查询账号
  之前返回 `ErrInvalidSession`，不读取账号和 marker，不写 `user_sessions`。
- **T2 只收编早于 marker 的账号。** 其余情况下，旧格式收编在现有条件之外还要求 `users.created_at` 非零且
  **严格早于** marker。账号年龄只看服务端写入的这个字段，不依赖 token 里的任何内容。全新安装的 marker
  早于所有账号，因此**全新安装没有旧格式收编窗**，包括已经用当前版本写下 marker、窗口尚未结束的实例。
- 两条规则都只作用于旧格式分支。失败与其它会话失败共用 `ErrInvalidSession`，三个入口沿用各自既有的
  通用 401（REST JSON `invalid token`；WebDAV `WWW-Authenticate` 401；WS 握手 JSON 401），响应和日志不含
  token、claims、用户名或密钥，不暴露拒绝原因。

### 5.2 保持不变

- 新格式会话（含 `jti` 与 `authVersion`）的签发、验证、续期、退出、改密撤销、删用户撤销、64 个上限，在
  公开默认值和自定义密钥下都不变；公开默认值实例上现有的新格式登录**不会**因本修复失效。
- `compatibleSecrets()` 对新格式 token 的双默认值兼容不变。
- 自定义密钥的已升级实例：账号早于 marker、`iat` 不晚于 marker、处于七天窗口、`auth_version=1` 的合法旧
  token，收编行为与现在相同；窗口结束、改密、退出墓碑的现有规则不变。
- marker 写入逻辑、`schema_migrations`、`users`、`user_sessions` 结构不变；不新增表、列、文件或启动步骤；
  `data/`、`cache/`、`library/` 布局与 ordinary/portable/Legado 备份格式不变。
- `config.Load()` 的默认值、资源 capability 的签名方式不变（S2 处理）。WebDAV Basic 仍按用户名和 bcrypt
  校验，不受影响。

### 5.3 明确放弃的行为及理由

- **公开默认值的已升级实例上，升级前签发的旧格式 token 不再收编。** 窗口内已经收编的旧格式会话也随之
  失效，因为旧格式 token 每次请求都重走同一分支。理由：用公开值签名的旧 token 与第三方构造的 token 在
  密码学上无法区分，无法只保留合法那部分。影响面：只有今天才从早于 `a0edce3` 的镜像升级、且没设置密钥
  的实例；代价是升级后重新登录一次，而七天后这些 token 本来也会失效。七天前就已升级到 `a0edce3` 或更新
  版本的实例窗口早已结束，全新安装从未有过合法旧 token，两者都不受影响。
- **T2 不放弃任何合法 token。** marker 之后本实例只签发新格式 token；回滚到旧二进制后签发的旧 token 的
  `iat` 晚于 marker，现有规则本来就拒绝。唯一的例外是账号创建时服务器时钟比升级时还快（`created_at`
  落在 marker 之后），这类账号需要重新登录一次。`created_at` 为零或无法解析时按“不早于 marker”处理，
  失败关闭。

## 6. 允许差异

全部属于“安全修复”类：T1、T2 和 5.3 的放弃项。没有新增 UI、API 字段、状态码或持久化。

## 7. 资源 capability 评估（F3）

`services/epubreader`、`cbzreader`、`audioreader`、`chapterimage` 用 HMAC-SHA256，`coverimage` 用 AES-GCM
签发资源链接。密钥都由 `OPENREADER_JWT_SECRET` 加用途前缀派生，对应路由（`/api/epub-resource`、
`/api/cbz-resource`、`/api/audio-resource`、`/api/chapter-image`、`/api/cover`）不经过会话。密钥公开时，第三方
可以构造任意 claims：

| 资源 | 链接绑定 | 公开密钥下第三方能做什么 | 风险 |
| --- | --- | --- | --- |
| EPUB / CBZ / 音频 | 用户、书籍、源文件内容的 SHA-256 指纹（十六进制校验）、规范化归档路径；打开时复核书籍归属、解包标记，重建时重算指纹 | 必须事先拥有同一份文件才能算出指纹；最多确认“某账号的某本书是这份文件”，读回自己已有的内容 | 低 |
| 章节图片 | 用户、书籍、书源、图片键、缓存图片字节的指纹；只读已缓存文件 | 同上，必须已有同一张图片 | 低 |
| 封面 | 远程图片地址，有效期 48 小时 | 让服务端以某个现有账号的名义抓取任意公网图片（私网、回环地址被 `forbiddenIP` 拦截，有大小、超时、跳转上限），写入该账号的封面缓存（每账号上限 256 MiB，按 LRU 淘汰，会挤掉正常封面）；等于一个匿名图片代理。读不到其他用户的数据，也进不了内网 | 中 |

根因与 F1 相同：用公开值签名。在会话层打补丁解决不了资源链接，正确方向是“永远不用公开值签名”（第 8 节）。
这会改变启动和数据卷行为，所以不并入本 P0 切片，避免扩大本次自动发布的影响面。

## 8. 后续切片 S2：不用公开值签名（已完成）

2026-11-10 更新：S2 已按
[`instance-signing-secret-fixed-baseline-second-audit-p1-contract.md`](instance-signing-secret-fixed-baseline-second-audit-p1-contract.md)
实现（`26948f92`），2026-10-30 以 `4c40a4a` 发布（该合同第 11 节），F3 因此关闭；2026-11-10 的 `latest`
（`74cf0d04`，Actions run `42780054569`）仍包含它。下文保留当时定下的方向，实际行为以 S2 合同为准。

本合同只确定方向，实现另起切片，有自己的合同、红测和发布门：

- 密钥未设置或为两个公开默认值之一时，首次启动生成 256 bit 随机实例密钥并持久化在 `data/` 下，之后用它
  签名会话和资源链接；显式设置的自定义密钥行为不变。文件须原子创建、权限 0600，不能被 WebDAV、
  `/uploads`、本地书仓或任何备份（ordinary/portable/Legado）读到；整份复制 `data/` 迁移主机时随之保留。
- 已登录用户不应因此被登出：新格式会话由 `user_sessions` 中的随机 `jti` 摘要认证，可以在过渡期内继续接受
  公开值签名的新格式 token；旧格式 token 已由本切片 T1 拒绝。
- 必须写进 S2 合同与发布说明的代价：切换前签发的资源链接（12–48 小时有效）失效一次，刷新页面即可；回滚
  到不认识实例密钥的旧镜像时，切换后签发的会话失效、需要重新登录。
- 发布门：fresh / historical / restart / 回滚再升级的卷测试，以及“未设置密钥”路径的真实启动测试（现有卷门
  都显式设置了密钥，覆盖不到这条路径）。

## 9. 测试计划（先红后绿）

红测必须在 `a1a1795` 上因被测行为本身失败，不能是编译或夹具错误。

### 9.1 api 层（真实 Gin + SQLite）

新文件 `backend/api/authenticated_session_legacy_public_secret_contract_test.go`：

1. **R1 全新安装 + 公开默认值**：第一个注册账号是管理员；分别用两个公开默认值签旧格式 token
   `{"userId":<管理员>,"iat":2020-01-01}`。`GET /api/me`、`PROPFIND /webdav/`、`PROPFIND /reader3/webdav/`、
   `/ws/sync` 握手都应为 401，且不产生 legacy session 行。
2. **R2 已升级实例 + 公开默认值（T1）**：账号先存在、再运行会话迁移写 marker（与真实升级同序）；用公开
   默认值签 `iat` 不晚于 marker 的旧格式 token。三个入口都应为 401，不产生 legacy 行。
3. **R3 全新安装 + 自定义密钥（T2）**：marker 之后注册的账号，用自定义密钥签的旧格式 token，三个入口都应为
   401，不产生 legacy 行。
4. **正常对照（新旧实现都必须通过）**：
   - 公开默认值与自定义密钥下，注册得到的新格式 token 在 `/api/me`（200，返回本账号）、两个 WebDAV 前缀
     （207）和 `/ws/sync`（握手成功）都可用；
   - 公开默认值下伪造的新格式 token（随机 `jti`、`authVersion=1`）为 401；
   - 自定义密钥、账号早于 marker 的已升级实例：窗口内合法旧 token 在三个入口都可用、收编出一条 legacy 行，
     退出返回 204 后同一 token 为 401（墓碑）。

### 9.2 服务层

新文件 `backend/services/authsession/legacy_public_secret_contract_test.go`：

1. **公开值规则（T1）**：配置为 `change-this-before-deploy`、`change-me-in-production` 或纯空白时，用任一公开值
   签的旧格式 token（账号早于 marker、处于窗口）返回 `ErrInvalidSession`，不产生行；数据库已关闭时同样
   返回 `ErrInvalidSession`，证明拒绝发生在查询账号之前。
2. **账号年龄规则（T2）**：自定义密钥下，`created_at` 晚于 marker、等于 marker、为零时都返回
   `ErrInvalidSession` 且不产生行；早于 marker 时照常收编（对照）。
3. **正常对照**：公开默认值下签发的新格式 token 可认证，两个公开默认值之间的新格式兼容不变。
4. **夹具更正**：现有 `newServiceFixture` 在 marker（2026-08-25）之后才创建账号，不符合“已升级实例”的真实
   形态。红测提交把夹具账号的 `created_at` 设为 marker 前一天，**断言不变**；这一改动在旧实现上同样通过。

## 10. 文档同步

- 记录提交更正 `docs/security-review-checklist.md` 的密钥条目：写明默认值是公开的、旧格式收编不再信任公开值、
  全新安装没有收编窗、F3 由 S2 跟进。
- README / README_CN 的“在公网上使用 → 换成自己的登录密钥”一节在发布后复核。改写在分支
  `claude/readme-user-facing`（`7d1af95`，截至本合同未合入 main），不在本切片的分支上修改。

## 11. 实施与发布闸门

- 后端：`go test ./...`、`go vet ./...`、`go test -race ./api ./db ./services/...`（api 需 `-timeout 40m`）。
- advisory 发布门：`node --test scripts/check-go-advisories.test.mjs scripts/check-frontend-advisories.test.mjs`
  与两项实际扫描。
- 本切片不改启动、数据卷、备份和前端；fresh / historical 卷门由 Actions 在合入 main 后照常执行，本地不另跑。
- 合入 main 即触发可信 Actions 发布；用 `gh run view` 与 `docker buildx imagetools inspect` 核验 run 结论、
  digest 和平台。不做任何生产操作。

## 12. 非目标

- 不改 `config.Load()` 默认值、不生成实例密钥（S2）、不改资源 capability、不改前端或 API 形状。
- 不处理 O1、O2；不改变新格式会话的双默认值兼容。
- 不把其它弱的自定义密钥列入公开值名单：T2 已让这类实例的风险只剩“升级后七天内、升级前的账号”。

## 13. 实施与验证证据（2026-10-22）

### 13.1 提交

| 提交 | 内容 |
| --- | --- |
| `a7bdc8b` | 本合同及 gap-analysis / matrix 登记 |
| `cfdcd8a` | 红测（9.1、9.2）及 9.2 第 4 条的夹具更正（断言不变） |
| `9e77cd9` | 实现：只改 `backend/services/authsession/service.go` 的旧格式分支 |

同一分支随后是依赖维护切片
[`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)，
它是本修复能够发布的前提。

### 13.2 红 → 绿

- `a1a1795` 上：api 层 R1（2 个子测试）、R2（2 个子测试）、R3 在 REST、两个 WebDAV 前缀和 `/ws/sync` 上全部按预期
  失败，且各产生 1 条 legacy 行；服务层 T1 用例 6 个子测试、T2 用例 4 个子测试按预期失败。正常对照（新格式会话
  ×2 种密钥、未知 `jti` 的新格式 token、已升级自定义密钥实例的合法收编与退出墓碑、早于 marker 的收编、两个公开
  默认值之间的新格式兼容）与夹具更正后的既有服务层测试全部通过。
- `9e77cd9` 上：同一组断言全部通过；既有会话、删用户、改密、过期、WebSocket 握手合同测试照常通过。
- 临时变异（未提交）：只去掉 T1 时 T1 服务层用例与 R2 变红；只去掉 T2 时 T2 服务层用例与 R3 变红；两种情况下
  R1 都仍为绿，说明两条规则各自独立生效。

### 13.3 真实二进制与浏览器

- 用 `a1a1795` 与修复后的提交分别构建二进制，不设置 `OPENREADER_JWT_SECRET`、使用全新数据目录：旧二进制复现
  第 1 节的问题；新二进制在 REST、两个 WebDAV 前缀和 `/ws/sync` 都返回 401，正常登录照常可用。
- `scripts/smoke/authenticated-session-lifecycle-real-api-contract.mjs` 在同样的默认密钥实例上通过（双登录、
  logout 204/401、改密与删用户后 401，REST/WebDAV/WS）；服务日志未出现 token 类内容。
- 浏览器：`openreader-smoke.mjs` 三视口、`book-management-real-api-contract.mjs` 与
  `reader-progress-multiclient-contract.mjs` 三视口通过（结果在维护切片合同第 9 节）。

### 13.4 回归

| 门 | 结果 |
| --- | --- |
| `go test ./...` | `9e77cd9`（Go 1.26.9）29 包通过；最终 Go 代码（Go 1.26.10）29 包通过 |
| `go vet ./...` | 两次均 exit 0 |
| `go test -race -timeout 40m ./api ./db ./services/...` | `9e77cd9` 24 包通过，api 1029.9 s，无 DATA RACE；最终树结果见维护切片合同第 9 节 |
| advisory 门 | 离线合同测试 128/128；实际扫描在维护切片后通过（见维护切片合同第 9 节） |
| 其它 | `docker compose config --quiet`、`store-read-list-lifecycle-contract.mjs` 通过 |

本切片不改启动、数据卷、备份和前端，本地未跑 fresh / historical 卷门，由 Actions 在合入 main 后执行。

### 13.5 推送方式

仓库公开。为避免修复发布前公开问题细节，合同、红测和实现没有按阶段逐个推送，而是在全部本地门通过后与
依赖维护切片一起合入 main。首次合入（`ae4736d`，09:49Z）后，可信 Actions 两次在 OS APK 门失败、没有发布镜像
（复制的构建镜像证书包、musl 待修复项，见维护切片合同第 10、11 节）；修复后第三次运行于 11:36Z 发布。问题细节在
main 上公开到镜像可用之间约 1 小时 47 分钟。

### 13.6 发布与状态

- Git：`main` 与分支 `claude/auth-legacy-public-secret-forgery` 都在 `92c98a971dce5c9166c0426695f89cd75f37cf94`
  （`git ls-remote` 核验）；其间 `9f94117`（README 改写，另一会话）位于 `ae4736d` 之后。
- 镜像：可信 Actions run `39768628680` 成功（Go 测试、advisory 门、前端测试与构建、Compose、存储 HTTP 合同、双平台
  候选镜像、OS APK 门、fresh 卷与 portable 备份、历史卷兼容、发布、平台核验全部通过）。
  `ghcr.io/changshengyu/openreader:92c98a9` 与 `latest` 为同一 OCI index
  `sha256:3b29f501e659454ea096fddcc4e0a05ba3c881cccacf8c3a684218649ba218d2`：`linux/amd64`
  `sha256:f102c6af2681bd1b3ad97fd698100df297f3866dbc0e5429e27a2ebb18a6d7f0`、`linux/arm64`
  `sha256:7cfba03369e47c35660bae197dae2307d61747320b0def443fab2987cfc99e17`，两平台 revision 标签均为完整提交号。
- 生产：未访问，运行版本未知；用户更新容器后才生效（标准 Compose 用 `latest` 加 `pull_policy: always`）。
- README：改写版（`9f94117`）“在公网上使用”一节的理由与修复后的行为一致，不需要再改。
- 后续：F3 已由 S2 关闭（见第 8 节）；O1、O2 仍开放。
