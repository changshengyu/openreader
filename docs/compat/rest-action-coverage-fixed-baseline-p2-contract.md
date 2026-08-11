# REST 长尾动作固定基准覆盖合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前审查基线：`OpenReader@d2f7bfe`  
审查日期：2026-08-11

## 1. 目的与范围

本合同补齐 `backend/api/server.go` 中此前没有以精确路径写入专项合同的六个动作。路径字符串没有
出现在旧文档不等于功能缺失；每个动作仍须分别核对上游入口、状态转换、请求/响应、持久化和可见
调用方。结论只分为 `aligned`、`technical-stack-equivalent`、`compatibility-only`、`must-fix`。

覆盖路径：

- `POST /api/auth/login`
- `POST /api/auth/register`
- `GET /api/txt-toc-rules`
- `GET /api/books/:id/source-candidates`
- `POST /api/books/:id/bookmarks/batch-delete`
- `PUT /api/categories/reorder`

## 2. 动作矩阵

| OpenReader 动作 | reader-dev 权威动作 | 当前语义 | 裁决 |
|---|---|---|---|
| `POST /api/auth/login` | `POST /reader3/login`，`isLogin=true` | 独立登录路由，JWT `{token,user}`；未知用户和错误密码统一 401；成功后更新 `last_active_at`。 | **technical-stack-equivalent**。拆分路由、JWT、通用 401 是 Go/多用户安全适配；既有账号包括旧卷账号必须继续可登录。 |
| `POST /api/auth/register` | 同一 `/reader3/login`，`isLogin=false` | 独立注册路由；新账号用户名/密码规则与上游一致；首个账号成为管理员；重复账号 409；成功直接签发 JWT。 | **technical-stack-equivalent**。显式注册界面替代 `isLogin` 开关，不能把注册重新混回登录或破坏旧账号兼容。 |
| `GET /api/txt-toc-rules` | `GET /reader3/getTxtTocRules` | 当前仅返回 10 条 `enable=true` 规则，导入前端又过滤 `enable:false`。 | **must-fix**。必须返回固定上游全部 18 条及原始顺序，手动规则列表不得按 `enable` 过滤。 |
| `GET /api/books/:id/source-candidates` | `/reader3/getAvailableBookSource`、`searchBookSource(SSE)` | 已用 `available/refresh/search` 分离无网络读取、缓存来源复验和分组增量搜索；每用户/每书派生缓存、精确书名+作者、稳定 cursor、当前项保留和换源事务已落地。 | **technical-stack-equivalent / implemented**。用有界 JSON 批响应替代 SSE 是 Go 栈适配；见 [`reader-source-switch-fixed-baseline-second-audit-p0-contract.md`](reader-source-switch-fixed-baseline-second-audit-p0-contract.md)。 |
| `POST /api/books/:id/bookmarks/batch-delete` | `POST /reader3/deleteBookmarks` | 当前按不可变 bookmark ID、当前用户和当前书过滤；事务删除；按请求中的首次有效顺序返回 `deletedIds`；提交后只广播一次。 | **technical-stack-equivalent / aligned**。ID 身份修复上游按书名作者删除首条的歧义，必须保留。 |
| `PUT /api/categories/reorder` | `/reader3/saveBookGroupOrder` 的旧 Category-only 映射 | 当前按当前用户 category ID 更新顺序；前端 API/store 仍保留，但可见 BookGroup 已统一使用 `/api/book-groups/reorder {keys}`。 | **compatibility-only**。旧内部 API 保留以兼容历史调用，不得重新接入可见 UI；统一混合分组排序继续以 `book-groups/reorder` 为唯一主链。 |

## 3. 登录与注册合同

### reader-dev

`UserController.login` 用一个动作和 `isLogin` 区分登录/注册：空用户名或密码失败；注册时执行固定的
用户名、保留名和密码校验；登录时读取既有用户、验证密码并保存 session/最后登录时间。

### OpenReader 映射

- `AuthForm` 显式切换登录/注册，分别调用两个路由，用户可见能力等价于上游 `isLogin`。
- 新账号仍执行：trim 后用户名至少 5 位、只允许 ASCII 字母数字、大小写不敏感保留 `default`、
  密码至少 8 位。
- 上述新账号规则不得追溯拒绝旧 SQLite 用户；登录只 trim 用户名，不重新执行注册校验。
- 登录成功后 `lastActiveAt` 和兼容字段 `lastLoginAt` 必须反映本次登录；失败登录不得更新时间。
- 首个注册用户成为管理员，以及 JWT token/role/permission 投影，是 OpenReader 多用户运行所需适配。
- 不泄漏“用户名存在但密码错误”的差异；当前统一 401 文案属于允许的安全增强。

既有 `user_management_p2_contract_test.go` 已覆盖新账号约束、旧账号登录与登录时间；本轮不以路径
遗漏为理由重写已对齐逻辑。后续安全门应单独验证认证 JSON body 上限，但不得借此改变可见状态机。

## 4. TXT 目录规则合同

固定上游 `defaultData/txtTocRule.json` 是权威清单：`id=-1…-18`、`serialNumber=0…17`，其中
`-3/-4/-5/-10/-12/-15/-16/-17` 为 `enable=false`。`enable` 只控制自动探测是否尝试该规则，
不控制用户能否在导入界面手动选择。

必须满足：

1. API 直接数组必须逐项保留 `id/enable/name/rule/serialNumber`，顺序与上游 JSON 完全一致。
2. `useOverlayBookImport.loadTocRules()` 只剔除没有规则文本的损坏行，不得过滤 `enable=false`。
3. 自动检测仍只遍历 `enable=true`，并继续受 512 KiB probe、输入/解码文本/章节数预算约束。
4. 手动选择 18 条中的任一条都必须可执行；不能只把 RE2 无法编译的 Java 正则显示出来。
5. `-16` 前向双标题和 `-17` 后向双标题包含跨行 lookaround。Go 实现必须保留相邻标题语义，
   不能简单删除 lookaround 后把每一个普通章节行都当成匹配。
6. 自定义用户规则继续支持当前已实现的上游 lookbehind 前缀和负向排除兼容，超长规则与解析预算
   继续拒绝。

错误测试 `overlayBookImport.test.mjs` 中“filters enabled TXT TOC rules once”必须改写；
`TestDefaultTXTTocRulesIncludeUpstreamEnabledRules` 必须升级为精确 18 条合同，并为八条手动规则增加
至少一组成功/不应匹配 fixture，特别覆盖 `-16/-17` 的相邻行语义。

## 5. 书签批删合同

上游以书名和作者定位，每个 payload 删除找到的第一项；在同书多书签和跨用户环境中身份不稳定。
OpenReader 的 bookmark ID、`user_id + book_id + id` 过滤、去重、事务及一次 post-commit event 是明确
允许的数据安全适配。请求中的外用户、外书和不存在 ID 被忽略，响应只返回实际删除 ID；空/无正数
请求为 400。此语义由
[`bookmark-fixed-baseline-second-audit-p2-contract.md`](bookmark-fixed-baseline-second-audit-p2-contract.md)
继续约束，本轮只补精确路径覆盖。

## 6. 分类排序兼容层合同

第二轮 BookGroup 重建已确认可见排序必须同时覆盖四个内置组和自定义 Category，并以稳定字符串 key
执行完整顺序事务。因此：

- `OverlayBookGroups`、`useOverlayBookGroups` 与新代码只能调用
  `PUT /api/book-groups/reorder {keys}`。
- `PUT /api/categories/reorder {ids}` 和 `reorderCategoryIds()` 只作为旧 OpenReader API 兼容层保留；
  不得在模板、composable 或新测试中把它当成主流程。
- 兼容层仍必须只更新当前用户 ID、事务失败不留下部分顺序，并广播 Category 和统一 BookGroup 投影。
- 如果未来有版本化 API 删除计划，必须先提供迁移窗口；本轮不得直接删除，因为旧合同和旧调用方
  已明确承诺兼容。

## 7. 实施闸门

本合同完成后，应用实现顺序为：

1. TXT 精确清单与手动解析测试先失败，再修改 parser/API/导入前端。
2. 阅读器换源按专项合同先建立 API、数据和前端状态失败测试，再做加法迁移与实现。
3. 登录/注册、书签批删和分类兼容层只补覆盖或安全上限，不做无证据的功能改写。
4. 聚焦测试后必须跑 Go 全量、frontend 全量与 production build；TXT 导入和换源各做真实浏览器。
5. 涉及候选缓存表时必须通过 fresh/historical volume、用户删除/书籍删除清理和 portable backup
   不包含派生缓存的合同。

## 8. TXT 实施与验证结果（2026-08-11）

- `DefaultTXTTocRules()` 已逐字段恢复固定上游 18 条规则、原始顺序和八条 `enable=false` 状态；
  `GET /api/txt-toc-rules` 直接数组由 API 合同逐项比对该清单。
- 导入界面只过滤空规则，八条默认关闭规则仍出现在手动选择列表；自动探测继续只遍历
  `enable=true`。
- `-16/-17` 使用仅针对固定上游原始规则的有界相邻行匹配器，最多检查相邻 8 个 Unicode 空白；
  不把跨行 lookaround 删除成普通章节匹配，也不把用户修改过的相似规则静默映射到专用匹配器。
- 原始输入、解码文本、规则字节数和章节数继续使用既有 parser limits；普通规则仍由 Go RE2 执行，
  没有引入回溯型正则引擎、脚本执行或新的文件/网络入口。
- 18 条精确清单、八条手动规则正反 fixture、自动探测禁用项、双标题相邻/孤立语义和 API 投影均
  通过；相关包、全量 Go、focused race、`go vet`、frontend 730/730 和 production build 通过。
- 真实 Go + Chromium 在 1440×900、390×844、360×800 均完成默认关闭的“数字(纯数字标题)”
  手动选择、2 章重新解析，以及既有 TXT 重试/确认和 EPUB 导入全流程。移动侧栏测试同步改为等待
  元素真实进入视口，避免在 260px 过渡期间误报不可见。

TXT 切片不修改 API 路径、SQLite、缓存、书架数据、备份/WebDAV 格式或用户文件。Reader 换源候选
已在后续专项切片完成实现和回归，但 Docker 发布仍须独立通过 mounted-volume 门。

## 9. TXT Docker 发布结果（2026-08-11）

实现提交 `33c7b15b064afb3943686348a260cb5eef89fd4b` 已先推送 GitHub，再仅由本机 OrbStack 构建和上传：

- `ghcr.io/changshengyu/openreader:33c7b15`
- `ghcr.io/changshengyu/openreader:latest`
- OCI index：`sha256:e3c88f10fb213abae9d730ca9eec62c46d09fbc2487b2af8c42d1f4ebb1b9a24`
- amd64：`sha256:a378b2732c5eeefbb4dec23420dcd2f906429a9212534535004a1bd43441cb65`
- arm64：`sha256:9a33be3ca80fd68de37cb1e94eccf2cd08ad7eedd645f88ab6f15d0306dc81a2`

本地候选先通过 fresh volume 的 portable-v1、portable-v2 assets、cross-user、restart；历史卷第一次与
fresh 门并行运行时出现一次无上下文 404，发布暂停后顺序重跑完整通过 TXT、EPUB、UMD、CBZ、
relative-cache、owner-isolation 和 portable restore/restart。正式发布因此采用顺序门结果，不把并行
容器资源竞争当作数据兼容通过证据。远端 `33c7b15` 与 `latest` 已回读为同一上述双架构 index。

## 10. Reader 换源实施与验证结果（2026-08-11）

- `available` 不发远程请求并幂等播种历史书当前项；`refresh` 只访问缓存 source ID；`search` 按分组
  独立 cursor 有界扫描，只保存书名和作者双精确结果。
- `book_source_candidates` 是 user/book scoped、200 行上限的派生表，不进入普通/portable/WebDAV
  backup；建书、换源、删除、用户删除和 source COW 的事务/隔离合同均有测试。
- 前端拆分 opening/refreshing/loadingMore，换源不再二次搜索；面板关闭后使用段落视口锚点恢复正文。
- Go 全量、API/service race、`go vet`、frontend 734/734、production build 及四视口真实 Chromium
  available/refresh/search/change/empty 流程通过。
- 本机 `a2ecc17` 候选通过 fresh 与成功顺序 historical mounted-volume 门；第一次 historical 运行的
  无上下文 404 未记为通过，原样重跑完整成功后才发布。`a2ecc17`/`latest` 远端回读为同一双架构
  OCI index `sha256:311ca87a75e4b77c49c95c033c80ac4a6d7baa1598092b630ac5002ce5493754`。

当前状态：**TXT aligned / Docker-published / awaiting-device-verification；source-switch aligned / Docker-published / awaiting-device-verification**。

## 11. 公开认证请求边界第二轮（2026-08-11）

六动作的可见登录/注册状态机继续保持 `technical-stack-equivalent`，但其公开 wire boundary 尚未签收：
`POST /api/auth/login` 与 `/register` 直接使用无上限 `ShouldBindJSON`，Gin 又只解码首个 JSON 值；
注册密码超过 bcrypt 的 72-byte 硬限制还会错误返回 `500`。这些不是上游产品行为，属于 Go 运行时/
安全适配的 P2 must-fix。

精确 16 KiB body、单 JSON、`413/400/401`、零副作用、旧账号和测试先行合同见
[`auth-request-boundary-fixed-baseline-second-audit-p2-contract.md`](auth-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同先以 `052de86` 独立提交。随后旧实现红测证明 declared/chunked 超限可继续认证/写入、尾随 JSON
被忽略、73-byte 注册误成 `500`，且共享前 72 bytes 的 73-byte 登录会错误成功；实现现已在查库、
bcrypt 和写入前执行认证专用有界单值解码及 72-byte 分流。focused/full/race/vet、frontend 740/740、
build 和真实 HTTP smoke 均通过。当前状态为 **aligned / regression-validated / Docker-pending**。
