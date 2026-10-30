# 实例签名密钥：不再用公开默认值签名合同（S2，P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前实施基线：`OpenReader@b1e84f3`（`origin/main`；应用代码与已发布的 `92c98a9` 相同）

审查日期：2026-10-22
状态：**implementation-complete / regression-validated**（证据见第 10 节；发布见第 11 节）

前置合同：[`authenticated-session-legacy-public-secret-fixed-baseline-second-audit-p0-contract.md`](authenticated-session-legacy-public-secret-fixed-baseline-second-audit-p0-contract.md)
（下称“P0 合同”）。本合同实现 P0 合同第 8 节定下方向的后续切片 S2，处理其 F3；P0 合同的 T1、T2 与
[`authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md`](authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md)
（会话合同）的其余条款不重开。

## 1. 范围与结论

`OPENREADER_JWT_SECRET` 未设置时 `config.Load()` 回退到仓库里公开的 `change-this-before-deploy`。标准
`docker-compose.yml` 不设置这个变量，所以多数部署实际在用公开值；配置为另一个公开旧默认值 `change-me-in-production`
或只有空白时同样等于没有密钥。这个值签名两类东西：

1. **登录会话 JWT。** 新格式会话由 256 bit 随机 `jti` 的 SHA-256 摘要（`user_sessions`）认证，签名公开不构成接管；
   旧格式 token 已由 P0 T1 在公开值下一律拒绝。签名本身在公开值下没有任何意义。
2. **资源 capability。** `/api/epub-resource`、`/api/cbz-resource`、`/api/audio-resource`、`/api/chapter-image`、
   `/api/cover` 不经过会话，只靠签名（封面是加密）认证。P0 合同第 7 节评估：封面中风险（第三方可以构造链接，让服务端
   以现有账号名义抓取任意公网图片、挤占该账号的封面缓存，等于匿名图片代理），其余低风险（必须事先拥有同一份文件或
   图片才能算出指纹）。

裁决：**must-fix（P1）**。未设置、空白或公开默认值时，首次启动生成 256 bit 随机实例密钥并持久化在 `data/` 下，此后用它
签名会话和资源 capability，公开值不再签名任何东西；显式设置的自定义密钥行为不变。已登录用户不因此被登出。

## 2. 当前实现证据（`b1e84f3`）

| 位置 | 当前行为 |
| --- | --- |
| `backend/config/config.go#Load` | `JWTSecret: env("OPENREADER_JWT_SECRET", "change-this-before-deploy")`；`env()` 把空字符串也当作未设置，只有纯空白会原样保留。 |
| `docker-compose.yml`、`Dockerfile`、`.env.example` | Compose 只设置 `OPENREADER_TRUSTED_PROXIES`；镜像 `ENV` 不含密钥；`.env.example` 写公开值。 |
| `backend/main.go#run` | 创建 `data/`、`cache/`、`library/` 后直接打开数据库；没有任何密钥准备步骤。 |
| `backend/api/server.go#RegisterRoutes` | 把同一个 `cfg` 交给 `audioreader`、`cbzreader`、`chapterimage`、`coverimage`、`epubreader`，用 `cfg.JWTSecret` 构造 `authsession.New` 和 `AuthRequired`；`webdav.go` 用同一值构造 `WebDAVAuthRequired`。它已经在 `cfg.DataDir` 下做启动期文件工作（`.portable-restore-journals` 清理、`uploads/` 创建）。 |
| `backend/services/authsession/service.go` | `Issue` 用 `s.secret` 签名；`parseClaims` 按 `compatibleSecrets(s.secret)` 验证（公开值时两个公开默认值都接受）；`publicSecret(s.secret)` 为真时旧格式 token 在查询账号前拒绝（P0 T1）。 |
| `services/{epubreader,cbzreader,audioreader,chapterimage}` | HMAC-SHA256：先 `HMAC(secret, purpose)` 派生密钥，再对 base64url JSON 载荷签名。purpose 分别为 `openreader:{epub,cbz,audio}-resource:v1`、`openreader:chapter-image:v1`；有效期 12 小时。 |
| `services/coverimage/capability.go` | AES-256-GCM，密钥 `SHA-256(purpose ‖ 0x00 ‖ secret)`，AAD 为 purpose，有效期 48 小时；空白密钥时拒绝签发。 |
| `api/{epub,cbz,audio,chapter_image,cover_image}.go` | 签名或解密失败（`ErrInvalidCapability`）一律 `403`；封面在 capability 通过后才查账号、读缓存、抓取。 |
| `api/books.go#projectBookShelfListItem` 等 | capability URL 只是响应投影（代码注释：不写入 SQLite、归档、备份或事件）；前端只在内存和按网络结果刷新的浏览器缓存里持有。 |
| `data/` 布局 | `openreader.db`（默认位置）、`uploads/`（`/uploads/*` 公开读、用户资源）、`webdav/`（管理员 WebDAV 根；普通用户 `webdav/users/<name>`；备份写在这两处）、`defaultBookSources.json`、`.portable-restore-journals/`。本地书仓默认在 `library/localStore`。普通/portable 备份由数据库行、`library/` 原件和 `uploads/` 用户资源组成，不遍历 `data/` 根目录。 |
| `scripts/docker-volume-backup-smoke.sh` 等全部卷门与真实 API 合同 | 都显式设置 `OPENREADER_JWT_SECRET`；“未设置密钥”这条标准部署路径从未被真实启动过。 |

## 3. 固定上游

固定上游没有服务端签名密钥：登录生成随机 token 写进用户的 `token_map`，请求用会话 cookie 或
`accessToken=<username>:<token>` 查表（`BaseController.kt#checkAuth`）。资源方面，`/reader3/cover?path=<url>`
（`BookController.kt#getBookCover`）是不带签名的封面代理，`/epub/*` 直接把 `storage/data` 作为静态目录对外
（`YueduApi.kt`）。OpenReader 早先用带签名的 capability 替代了这两种无签名入口；本合同不改路由、载荷或状态码，只改
签名所用的密钥。因此不是上游行为差异，属于“安全修复”类允许差异。

## 4. 差异分类

| 编号 | 内容 | 分类 |
| --- | --- | --- |
| F1 | 未设置、空白或公开默认值时，新签发的会话 token 用公开值签名。 | **must-fix（P1）**：与 F2 同根，一并修 |
| F2 | 同一情况下五类资源 capability 用公开值签名或加密，第三方可构造（P0 合同 F3：封面中，其余低）。 | **must-fix（P1）** |
| F3 | 没有任何发布门真实启动“未设置密钥”的部署形态，也没有覆盖重启、回滚、再升级时密钥与会话的行为。 | **must-fix（发布门）** |
| O1 | README“在公网上使用”第 1 步要求自己设置密钥，并给出占位值 `paste-the-random-string-here`（中文版为“粘贴刚才生成的随机字符串”）。S2 之后这一步不再是安全前提；照抄占位值的部署会被当作自定义密钥。 | 文档：发布后直接改 main 上的 README（第 9 节）；名单不扩大（第 5.6 节） |
| O2 | `.env.example` 仍写公开值。S2 之后这个值会选中实例密钥，不再有风险。 | 观察：不改 |
| O3 | P0 合同的 O1、O2（已收编旧会话每次请求重走窗口检查；`ParseToken` 无会话服务时只验签名）。 | 不在本切片范围 |

## 5. 目标行为

### 5.1 何时使用实例密钥

- “公开值”沿用 P0 T1 的定义：去掉首尾空白后为空、`change-this-before-deploy` 或 `change-me-in-production`。
- 配置值为公开值时进入**实例密钥模式**；否则是**自定义密钥模式**，行为与今天完全相同，并且**不读取、不创建、不修改**
  实例密钥文件（即使它存在）。

### 5.2 实例密钥文件

- 路径 `<OPENREADER_DATA_DIR>/instance-secret`（镜像内为 `/app/data/instance-secret`），与 `openreader.db` 同级。
- 内容：`crypto/rand` 生成 32 字节，写成 64 个小写十六进制字符加一个换行。读取时接受大小写十六进制和末尾单个 `\n`
  或 `\r\n`；其它任何内容（空文件、长度不对、非十六进制、多行、超过 1 KiB）都失败关闭。用于签名的是这 64 个字符本身。
- **原子创建、不覆盖**：只在文件不存在时创建。先在同目录用 `O_EXCL` 建 0600 临时文件 `.instance-secret-*.tmp`，写入、
  `fsync`、关闭，再用硬链接发布到最终名（目标已存在即失败）；发现已存在时读取并使用那个文件（并发首启只会有一个
  胜出者）。文件系统不支持硬链接时，在确认最终名仍不存在后改用 rename。发布后 `fsync` 目录并删除临时名。任何时刻
  最终名要么不存在，要么是完整内容。
- **权限 0600**。启动时若带有组或其他用户权限位，收紧为 0600 并记一行日志；收紧失败只记日志，不阻止启动。
- 必须是普通文件（`lstat`）：符号链接、目录、设备、FIFO 等一律失败关闭。
- **启动顺序与失败**：`main` 在创建 `data/` 之后、打开数据库之前准备密钥。读取或创建失败时进程以非零状态退出，错误只
  写路径和原因，不写内容；不做任何迁移或写库。逃生方式：设置自定义 `OPENREADER_JWT_SECRET`（自定义模式不碰这个文件），
  或从 `data/` 的备份恢复该文件，或把它移走让服务生成新的（所有人重新登录一次）。
- **日志**：创建时记一行“已生成实例密钥 <路径>”，此后每次以实例密钥模式启动记一行“使用实例密钥 <路径>”；密钥内容
  永不进入日志、错误、响应或事件。
- **不可外读**：WebDAV 根是 `data/webdav`（管理员）或 `data/webdav/users/<name>`，`/uploads` 根是 `data/uploads`，本地书仓
  根是 `library/localStore`，备份下载只读 WebDAV 备份根；文件在这些根之外，`rootedfs` 拒绝越界。普通（Legado 兼容）、
  portable 备份不包含它。整份复制 `data/` 迁移主机时随之保留（它就是 `data/` 里的普通文件）。
- `RegisterRoutes` 收到公开值配置时自己执行同样的准备（`main` 已准备过时它看到的是实例密钥，直接跳过），因此任何调用方
  都构造不出用公开值签名的服务；这条路径准备失败时 panic（只有测试或未来的调用方会走到）。

### 5.3 签名与验证

- 实例密钥模式下，会话 token 与五类资源 capability 都用实例密钥签名和验证。
- **会话过渡**：带 `jti` 与 `authVersion` 的新格式 token 若用公开值签名（来自旧镜像的登录），仍按会话合同验证：
  `jti` 摘要必须在 `user_sessions` 中、属于该账号、认证代次一致、未撤销未过期，续期与退出照旧。接受的公开值与今天
  `compatibleSecrets(配置值)` 完全一致（配置为任一公开默认值时两个默认值都接受）。服务从不再用公开值签名任何东西。
  过渡不设截止时间：新镜像不再产生这类 token，它们在最后一个这样的会话结束（7 天未使用、退出、改密、删用户、64 个上限
  淘汰）时自然消失；回滚期间旧镜像签发的会话在再升级后同样被接受。理由：这类 token 的认证依据是随机 `jti` 摘要，
  签名公开与否不改变谁能使用它；设截止时间只会制造一次本可避免的集体登出，没有安全收益。
- **旧格式 token**：实例密钥模式下无论用哪个密钥签名，一律在查询账号前拒绝（P0 T1 的延伸：配置值是公开值，旧二进制
  只可能用公开值签过它们，与伪造无法区分）。
- **资源 capability 没有过渡**：它们是无状态的，接受公开值签名的链接等于保留漏洞。
- 自定义密钥模式：签名、验证、P0 T2 的旧格式收编全部不变；只认自定义值本身，不认实例密钥或公开值。

### 5.4 代价（写进发布记录与 README）

1. **切换前签发的资源链接失效一次。** EPUB、CBZ、音频、章节图片链接（12 小时）与封面链接（48 小时）在升级后返回 403，
   刷新页面即可拿到新链接。它们从不持久化，所以不会长期坏掉。
2. **回滚到不认识实例密钥的旧镜像（`92c98a9` 及更早）时，切换后签发的会话失效**，需要重新登录；切换后签发的资源链接
   同样失效。切换前就存在的会话在旧镜像上照常可用。旧镜像不读也不删这个文件。
3. **再升级**：沿用同一个文件，不重新生成；回滚前由新镜像签发、尚未过期的会话重新可用，回滚期间旧镜像签发的会话按
   5.3 过渡接受。
4. 不带 `data/` 迁移（例如只用普通或 portable 备份在新实例上恢复）会得到新的实例密钥，需要重新登录；这与今天两台实例
   密钥不同时一样。
5. 删除或替换这个文件等于换密钥：用旧实例密钥签发的会话失效、资源链接失效一次。
6. 之后改为设置自定义密钥：与今天更换密钥相同，所有设备重新登录一次。

### 5.5 保持不变

- 数据库 schema、迁移、`user_sessions`、`authenticated-session-v1` marker、备份与 portable 格式、WebDAV/上传/本地书仓
  布局、全部 API 路由与响应、状态码、前端、Compose 与镜像 `ENV`。
- capability 的载荷、算法、purpose、有效期和路由不变，只换密钥。
- 自定义密钥实例的一切行为，包括不会出现实例密钥文件。

### 5.6 明确放弃或不做

- 不给公开值签名的新格式会话设截止时间（理由见 5.3）；不在请求中换发新 token。
- 不提供密钥轮换的界面或 API。
- 不把 README 占位值或其它弱值加入公开值名单（沿用 P0 合同第 12 节）；O1 通过改写 README 去掉“必须自己设置”的要求来
  消除照抄占位值的动机。
- 不改 `config.Load()` 的默认值字面量（旧镜像回滚时仍依赖它）；不处理 P0 合同的 O1、O2。

## 6. 允许差异

全部属于“安全修复”类：实例密钥文件、启动时的准备与失败关闭、公开值签名会话的过渡接受、切换前资源链接失效一次、
回滚时切换后会话失效。没有新增 UI、API 字段、状态码或数据库持久化。

## 7. 测试计划（先红后绿）

红测必须在 `b1e84f3` 上因被测行为本身失败，不能是编译或夹具错误。新包 `services/instancesecret` 与 `authsession.New`
的新参数在旧代码上不存在，它们的单元测试随实现提交；红测只通过旧代码已有的入口（`RegisterRoutes`、HTTP、真实二进制）
表达行为。

### 7.1 api 层（真实 Gin + SQLite）

新文件 `backend/api/instance_signing_secret_contract_test.go`：

1. **R1 公开值换成实例密钥**：配置为两个公开默认值、空字符串、纯空白时，`RegisterRoutes` 之后 `data/instance-secret`
   存在、是普通文件、权限 0600、内容为 64 个小写十六进制加换行，没有残留临时文件；注册得到的 token 用配置值和两个
   公开默认值都验不过，用文件里的密钥能验过。旧实现：文件不存在、token 用公开值签名。
2. **R2 资源 capability**：实例密钥模式下签发的 EPUB、CBZ、音频、章节图片 capability 用任一公开值重算 HMAC 都不相等、
   用实例密钥重算相等；把同一载荷用公开值重签后请求对应路由为 403，原链接为 200。封面 capability 用公开值派生的密钥
   解不开、用实例密钥能解开；用公开值加密、指向不存在账号的伪造封面链接为 403（旧实现通过签名检查后返回 404）。
3. **R3 重启与复用**：同一 `data/`、同一数据库再次 `RegisterRoutes`，文件内容、inode、权限不变；重启前签发的 token 在
   REST、两个 WebDAV 前缀与 `/ws/sync` 上可用，重启前签发的 CBZ 链接仍为 200。预先放置的合法文件被原样采用（token
   用它签名），预先放置的 0644 文件被收紧为 0600、内容不变。
4. **正常对照（新旧实现都必须通过）**：
   - 过渡：用公开值签发、`user_sessions` 有行的新格式会话（模拟旧镜像登录）在三个入口可用，退出返回 204 后为 401；
   - 公开值签的旧格式 token 与随机 `jti` 的伪造新格式 token 为 401，不产生 legacy 行；
   - 自定义密钥：不产生实例密钥文件，已存在的文件内容、权限、修改时间不变，token 与 capability 用自定义值签名，公开值
     签发的新格式会话为 401；
   - 不可外读：预先放置已知值的实例密钥文件后，管理员 WebDAV（两个前缀，PROPFIND 各深度与越界 GET）、`/uploads`
     越界、本地书仓列表与下载越界、备份下载越界都拿不到它，普通备份与 portable 备份的归档字节里没有它的内容和文件名。

### 7.2 真实启动与卷门（新增，未设置密钥）

新脚本 `scripts/smoke/instance-secret-volume-contract.mjs`，两种运行方式执行同一组断言：

- **Docker**（Actions）：`CANDIDATE_IMAGE` 为本次候选镜像，`PREVIOUS_IMAGE` 为按 digest 固定的
  `ghcr.io/changshengyu/openreader@sha256:3b29f501e659454ea096fddcc4e0a05ba3c881cccacf8c3a684218649ba218d2`（`92c98a9`，最后一个
  不认识实例密钥的发布）。容器不传 `OPENREADER_JWT_SECRET`，挂载独立的 `data/`、`cache/`、`library/`。
- **二进制**（本地）：`CANDIDATE_BIN`、`PREVIOUS_BIN` 为分别从本分支和 `92c98a9` 构建的二进制，从环境中删除
  `OPENREADER_JWT_SECRET` 后启动。

场景（同一卷依次进行）：

1. **historical**：旧版本在空卷上启动、注册管理员，得到公开值签名的会话 P1 和 CBZ 链接；确认没有实例密钥文件。
2. **upgrade**：候选版本启动，生成 0600、64 位十六进制的实例密钥；P1 在 REST、两个 WebDAV 前缀、`/ws/sync` 仍可用；
   旧 CBZ 链接为 403、新链接为 200；新登录的 token 与新链接用公开值验不过；公开值签的旧格式 token 为 401；日志有
   生成记录、没有密钥内容。
3. **exposure**：管理员 WebDAV、`/uploads`、本地书仓、备份下载的越界请求拿不到密钥；普通与 portable 备份下载后字节中
   没有密钥。
4. **restart**：候选版本重启，文件内容与 inode 不变，重启前的会话与链接仍可用。
5. **rollback**：旧版本在同一卷上启动，文件不变；候选版本签发的会话为 401（代价 2），P1 可用；在旧版本上登录得到 P2。
6. **re-upgrade**：候选版本再启动，文件不变；回滚前候选版本签发的会话、P1、P2 都可用，新登录用实例密钥签名。
7. **fresh / custom / malformed**（各用新卷）：候选版本在全新卷上直接生成文件、首个账号为管理员；设置
   `OPENREADER_JWT_SECRET=change-me-in-production` 同样进入实例密钥模式；自定义密钥不生成文件；文件内容非法时进程以非零
   状态退出、日志不含文件内容、文件不变。

旧实现（`b1e84f3` 二进制作为候选）在第 2 步“生成实例密钥”处失败。

`scripts/docker-volume-backup-smoke.sh` 增加 `JWT_SECRET_MODE=unset|custom`（默认 `custom`，现有门不变）：`unset` 时所有
容器都不传 `OPENREADER_JWT_SECRET`，并断言启动后 `data/instance-secret` 存在、是 0600 普通文件，重启后 inode 不变。

### 7.3 Actions 接线

`docker-publish.yml` 在现有 fresh 与 historical 卷门之后新增三步，均在推送镜像之前（实施时比计划多了第一步）：

- `JWT_SECRET_MODE=unset` 的 fresh 卷门（portable v1/v2、跨用户恢复与重启，源卷和恢复目标卷各有自己的实例密钥）；
- `HISTORICAL_VOLUME=1 JWT_SECRET_MODE=unset` 的历史卷门（旧 schema 夹具在未设置密钥时迁移、隔离、备份、重启）；
- `instance-secret-volume-contract.mjs` 的 Docker 方式（上面 1–7）。

新脚本加入 workflow 的 `paths` 触发列表。

### 7.4 随实现提交的单元测试

- `services/instancesecret`：公开值判定表；自定义值不碰文件；首次创建（0600、格式、无临时残留）、再次读取同一值；
  同一目录并发准备只产生一个文件且所有调用得到同一密钥；非法内容各变体、符号链接、目录失败关闭且文件不变、错误
  不含内容；宽权限被收紧；接受大写与 CRLF。
- `services/authsession`：带 previous 的服务用新密钥签发；previous 签名的新格式会话可认证；previous 非空时旧格式 token
  无论用哪个密钥签名都在查询账号前拒绝（关闭数据库仍返回 `ErrInvalidSession`）；不带 previous 时 P0 测试全部不变。

## 8. 实施与发布闸门

- 后端：`go test ./...`、`go vet ./...`、`go test -race -timeout 40m ./api ./db ./services/...`。
- 前端：不改代码，仍跑 `npm test` 与 `npm run build`。
- advisory 门：离线合同测试与两项实际扫描；不新增依赖（`golang.org/x/sys` 已在模块图中，实现只用标准库）。
- 本地：`instance-secret-volume-contract.mjs` 二进制方式在 `b1e84f3` 上按预期失败、在实现上通过；
  `store-read-list-lifecycle-contract.mjs`、`authenticated-session-lifecycle-real-api-contract.mjs` 照常通过。
- Actions：现有门加 7.3 两步全部通过后才推送双平台镜像；用 `gh run view` 与 `docker buildx imagetools inspect` 核验
  run 结论、digest、平台与 revision。不做任何生产操作。

## 9. 文档同步

- 记录提交更新 `docs/security-review-checklist.md` 的密钥条目、`docs/compat/data-migration.md`（新增 `data/instance-secret`
  文件，无 schema 变化）、`docs/docker-volume-backup-compatibility.md`（新门与 `JWT_SECRET_MODE`）。
- 发布后直接改 main 上的 README / README_CN“在公网上使用”第 1 步与配置表：未设置密钥时会自动生成并保存在
  `data/instance-secret`，`data/` 要妥善保管、迁移时整份复制；设置自己的密钥变为可选，并写明第 5.4 节的代价。

## 10. 实施与验证证据（2026-10-22）

### 10.1 提交

| 提交 | 内容 |
| --- | --- |
| `b0972b4` | 本合同及 gap-analysis / matrix 登记 |
| `3dec2ad` | 红测：api 层 R1–R3 与四组正常对照、`instance-secret-volume-contract.mjs`、卷门 `JWT_SECRET_MODE` |
| `f10c266` | Actions 接线：fresh / historical 卷门各加未设置密钥模式，加生命周期合同（旧镜像按 digest 固定） |
| `77dfee0` | 生命周期合同夹具修正：备份探测移到“只有封面的书”之前（portable 备份拒绝没有原件的本地书架条目，候选版本越过升级步骤后才暴露）；新增“非法文件时数据库尚未打开”断言 |
| `26948f9` | 实现：`services/instancesecret`、`config.JWTPreviousSecrets`、`authsession.New(..., previous...)`、`RegisterRoutes` 守卫、`main` 在打开数据库前准备；附单元测试 |

### 10.2 红 → 绿

- `b1e84f3` 上（旧实现）：R1 4/4 子测试失败（文件不存在、token 用配置值签名）；R2 5/5 失败（四类 HMAC capability 用公开值重算相等、公开值重签后请求为 200；封面能用公开值解开、伪造封面为 404）；R3 3/3 失败（重启前文件不存在、已有文件不被采用、0644 不被收紧）。过渡、自定义密钥（2 个子测试）、存储与备份不可外读三组对照通过。
- 生命周期合同二进制方式（候选 = `b1e84f3`，旧版本 = `92c98a9`）：historical 步骤通过，在“upgrade: data/instance-secret was not created”处失败。Docker 方式以已发布的 `92c98a9` 镜像同时充当两端：同一处失败；卷门 `JWT_SECRET_MODE=unset` 的 fresh 与 historical 都在“first start: data/instance-secret is absent”处失败；`custom` 模式 fresh 通过，historical 第一次在夹具创建后遇到一次 HTTP 404，随后两次修改后脚本与一次 `b1e84f3` 原脚本重跑全部通过（与本文件以前记录的历史卷偶发 404 相同，未复现，不归因于本切片）。
- `26948f9` 上：同一组 api 断言全部通过；生命周期合同二进制方式全部通过（historical、upgrade、exposure、restart、rollback、re-upgrade、fresh、previous-default、custom、malformed）。
- 临时变异（未提交，均已恢复并逐字节核对）：去掉 previous 时的旧格式拒绝 → 服务层旧格式用例 3 个子测试变红；去掉 `RegisterRoutes` 守卫 → R1、R2、R3 变红；保留守卫、去掉 `main` 的提前准备 → 生命周期合同只在新增的“数据库尚未打开”断言处变红；把硬链接发布换成 rename → 并发首启用例 3/3 次变红。

### 10.3 真实启动

- 不设置 `OPENREADER_JWT_SECRET` 的二进制：首次启动日志 `created a random instance secret in <data>/instance-secret`，文件 0600、65 字节；再次启动日志 `signing with the instance secret in ...`；文件内容非法时以状态 1 退出，错误只含路径与恢复方法，数据库未被打开。
- `authenticated-session-lifecycle-real-api-contract.mjs` 在未设置密钥的实例上通过（双登录、logout 204/401、改密与删用户后 401，REST/WebDAV/WS）；服务日志中没有实例密钥，也没有 token 形态的内容。
- 浏览器（未设置密钥的实例 + 本树 `frontend/dist`）：`openreader-smoke.mjs` 1440×900、390×844、360×800 通过；
  `reader-cbz-contract.mjs` 与 `reader-epub-contract.mjs` 三视口通过（Reader 经实例密钥签名的 capability 加载 CBZ 页面与
  EPUB 资源）；服务日志中没有实例密钥。前端代码未改，这是对签名换钥后资源链接真实可用的验证。

### 10.4 回归

| 门 | 结果 |
| --- | --- |
| `go test ./...` | 30 包通过（新增 `services/instancesecret`） |
| `go vet ./...` | exit 0 |
| `go test -race -timeout 40m ./api ./db ./services/...` | 25 包通过，api 1179.6 s，无 DATA RACE |
| 前端 | `npm test` 810/810（Node 26），`npm run build` 通过；前端代码未改 |
| advisory 门 | 离线合同测试 131/131；Go 实际扫描 pass-with-not-applicable（1 项既有已审查不适用），前端全锁扫描 pass（0） |
| 其它 | `docker compose config --quiet`、`store-read-list-lifecycle-contract.mjs` 通过 |

未在本地运行：候选镜像的 Docker 卷门与 Docker 方式的生命周期合同（本地不构建镜像），由 Actions 在推送镜像前执行。

### 10.5 合并 main 与发布阻塞（2026-10-23 至 10-30）

- `6137b37`：合并 `a7fd6aa`（发布流程浏览器启动门，另一会话）。只有 gap-analysis 与 matrix 冲突，保留双方条目；workflow
  自动合并，两边的步骤与 `paths` 都在，浏览器门测试 10/10。
- `2749428`：合并 `b4cf2be`（其后 23 个提交），仍只有两份总账冲突，S2 条目改记 2026-10-29 置顶。`92c98a9` 之后没有
  `backend/db`、`backend/models` 变化，回滚伙伴仍有代表性。
- 2026-10-29 重跑发布门时，Go advisory 实际扫描因 2026-10-27 发布的 GO-2026-6860（`golang.org/x/net/html` 文本合并二次
  复杂度）失败，任何发布都会被拦下。经用户确认在本会话修复；本地完成合同、红测、升级三个提交后，发现另一会话已在 main
  上修复并发布同一问题（`53770f4`/`7a18346`，记录 `c62c619`），模块版本与本地升级完全一致。本地三个提交从未推送，已丢弃，
  以 main 的修复为准。
- `af0035a`：合并 `24511c0`（其后 34 个提交：上述 x/net 修复、EPUB/UMD 本地书封面、Reader 夜间配色、本地导入元数据识别等）。
  冲突为 `backend/api/server.go`（S2 的 previous 接线与新增的 `umdCovers` 同时保留）和两份总账。新增的 `services/umdcover`
  （`/api/umd-cover`，purpose `openreader:umd-cover:v1`）由 `RegisterRoutes` 中同一个 `cfg` 构造，S2 守卫自动覆盖；EPUB 封面
  复用 EPUB capability。资源 capability 由五类变为六类。
- `bcba652`：R2 增加 UMD 封面用例（只用实例密钥签名，公开值重签后 403）；去掉 `RegisterRoutes` 守卫时该用例变红。
- 合并树（`bcba652`）回归：`go test ./...` 31 包、`go vet ./...`；race：api 1257.8 s 等 25 包通过，engine 的 XPath advisory
  看门狗子测试（`attribute`）在全量并行负载下超出 3 s 一次，测试未改、与 S2 无关，单独重跑 3/3 通过、engine 包 race 通过，
  无 DATA RACE；前端 836/836 与 build；advisory 离线测试 141/141、Go 扫描 pass-with-not-applicable（仅 GO-2026-5932）、前端扫描
  pass；生命周期合同十个场景、存储 HTTP 合同、未设置密钥实例上的三项浏览器合同通过。
