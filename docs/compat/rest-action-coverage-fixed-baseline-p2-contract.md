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
2. 共享 `useStorageImportWorkflow.loadTocRules()` 只剔除没有规则文本的损坏行，不得过滤
   `enable=false`；direct、LocalStore、WebDAV 使用同一规则投影。
3. 自动检测仍只遍历 `enable=true`，并继续受 512 KiB probe、输入/解码文本/章节数预算约束。
4. 手动选择 18 条中的任一条都必须可执行；不能只把 RE2 无法编译的 Java 正则显示出来。
5. `-16` 前向双标题和 `-17` 后向双标题包含跨行 lookaround。Go 实现必须保留相邻标题语义，
   不能简单删除 lookaround 后把每一个普通章节行都当成匹配。
6. 自定义用户规则继续支持当前已实现的上游 lookbehind 前缀和负向排除兼容，超长规则与解析预算
   继续拒绝。

前端规则投影由 `localBookImportRetryContract.test.mjs` 与共享 workflow 合同锁定；
`TestDefaultTXTTocRulesIncludeUpstreamEnabledRules` 已升级为精确 18 条合同，并为八条手动规则增加
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
build 和真实 HTTP smoke 均通过。`f5c15d7` 随后顺序通过 fresh/historical 卷与 portable backup 门，并
由本机发布为 `f5c15d7`/`latest`；远端 OCI index 为
`sha256:db667de319ae2721cbd35990896612a738b4570a94920875ea14e2aed613503f`。当前状态为
**aligned / Docker-published / awaiting-device-verification**。

## 12. 管理员用户写入请求边界与共享密码长度（2026-08-12）

六动作中的公开认证 wire boundary 已发布，但它刻意没有覆盖管理员 user mutations。当前管理员创建、
权限更新、密码重置、书源重置和批量删除仍直接 `ShouldBindJSON`，没有 actual-read/single-JSON；两个
批量动作没有 cardinality，create/reset 的 73-byte bcrypt 密码变成 `500`。固定上游和当前 Vue 又
证明新密码最小长度应按 UTF-16 code units，而不是 Go UTF-8 bytes。

精确路由、`401/403/413/400`、2,000 项、8 UTF-16/72 UTF-8、零副作用和测试先行合同见
[`admin-user-write-boundary-fixed-baseline-second-audit-p2-contract.md`](admin-user-write-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同提交时状态为 **inventory-complete / implementation-pending**；该提取阶段未修改应用或测试。

合同已先以 `61512f7` 独立提交。旧实现红测随后证明五入口的 declared/chunked 超限、第二 JSON/垃圾、
2,001 项批量、UTF-16 最小长度、73-byte bcrypt 与负限额均违反合同，并能产生 row/hash/source
namespace/event 副作用。`6c1c6db` 引入窄共享有界单 JSON decoder、8 UTF-16/72 UTF-8 新密码边界、
2,000 原始 ID 上限和 bcrypt 前确定性检查；focused/full/race/vet、frontend 740/740、build 与真实 HTTP
smoke 均通过。该提交顺序通过 fresh/historical 卷门并由本机发布为 `6c1c6db`/`latest`；远端 OCI
index 为 `sha256:55326ed147aea4370c0161d75568fe85a5095abb6dad6b487856dfeea09832a2`。当前状态为
**aligned / Docker-published / awaiting-device-verification**。

## 13. Bookmark 写入与并发删除边界（2026-08-12）

六动作中的 batch-delete 身份/事务语义保持 `technical-stack-equivalent`，其 wire/cardinality 与同模块
create/update 入口由
[`bookmark-write-boundary-fixed-baseline-second-audit-p2-contract.md`](bookmark-write-boundary-fixed-baseline-second-audit-p2-contract.md)
统一签约。合同 `7fa0e07`、红测 `030696b`、实现 `e5a7ea9` 已按顺序推送：单项 64 KiB、批量创建
16 MiB/2,000、批删 16 KiB/2,000、单 JSON、显式 note-only payload/SQL、fresh row 与并发删除不复活
全部关闭。

focused/full/race/vet、frontend 740/740、production build、Reader 多视口 payload、宿主/本地候选/GHCR
回拉 HTTP、SQLite trigger 与顺序 fresh/historical 卷门均通过。本机发布 `a9a55db` 与 `latest`；远端
amd64/arm64 OCI index 为
`sha256:944a85881170bc900c1fda0acb885bedc1dc4b17ed4e635305988163e1b635e5`，两平台 revision label 均为
`a9a55db16d490af61b31b5e1470ee477e2bba613`。当前状态为
**aligned / Docker-published / awaiting-device-verification**。

## 14. RSS 写入与缓存并发边界（2026-08-12）

六动作台账完成后，下一批按同一动作级方法复审 RSS source/article 持久写入。专项合同
[`rss-write-boundary-fixed-baseline-second-audit-p2-contract.md`](rss-write-boundary-fixed-baseline-second-audit-p2-contract.md)
以 `051bd12` 固定，旧实现红测 `1d0e5a0` 证明无界/多 JSON、同 URL create/import 竞争、全行 `Save`
覆盖和删除后复活；`5236389` 实现 actual-read 单 JSON、每用户 source 写事务、显式列所有权、正文
优先级与网络工作后的 source/article 存活复验。

focused/full/race/vet、frontend 740/740、production build、RSS 四视口、宿主 HTTP + SQLite trigger、
候选/回拉容器纯 API 与顺序 fresh/historical 卷门均通过。本机发布 `0986d8e` 与 `latest`；远端
amd64/arm64 OCI index 为
`sha256:9884d0e9b41c1a1a109f3034159ae2968fe9d889da063deaca2514e1ac371e25`。当前状态为
**aligned / Docker-published / awaiting-device-verification**。

本文件最初列出的六个动作以及后续 auth/admin/settings/group/category/book/BookSource/Bookmark/RSS
写边界均已关闭。下一轮不得从旧 pending 文案重开这些模块；应重新对 `backend/api/server.go` 与固定
上游 controller 做差集，先形成新的动作 inventory，再选择下一项 must-fix。

## 15. 用户资产上传/删除请求边界（2026-08-12 implemented）

重新执行动作差集后，`POST /api/uploads` 的 8/32 MiB 限制被确认只发生在 Gin 完整解析 multipart
之后；默认 32 MiB `MaxMultipartMemory` 只是临时文件分界，不是请求上限。额外 file/type part 被静默
忽略，成功解析的 multipart 又依赖外层 `net/http.Server` 清理。`DELETE /api/uploads` 则仍无 actual-read
上限并接受首个 JSON 后的第二文档。

固定上游、多用户允许差异、33 MiB 总包络、单 file/type、handler-owned `RemoveAll`、16 KiB 单 JSON、
稳定状态/错误与失败测试见
[`user-asset-write-boundary-fixed-baseline-second-audit-p2-contract.md`](user-asset-write-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同 `ce38478`、旧实现红测 `bb0c067`、实现 `f386016` 和真实 HTTP 探针 `be83a0f` 已依次推送；
33 MiB actual-read 包络、唯一 file/type、handler-owned `RemoveAll`、16 KiB 单 JSON 和既有 8/32 MiB
文件 admission 均有精确边界测试。Go full/race/vet、frontend 740/740、build、宿主/候选/GHCR 回拉
runtime、fresh 与成功重跑的 historical 卷门通过。本机发布 `be83a0f`/`latest`，远端 OCI index 为
`sha256:e1f31f3dd728bc27fbc89bbc8c21f81e8c5511c5e99196891feb21cd47138b73`。当前状态为
**aligned / Docker-published / awaiting-device-verification**。

## 16. LocalStore 文件系统与请求边界（2026-08-12 implementation）

重新盘点 `server.go` 后，下一项 must-fix 落在 `/api/local-store*`：多文件上传虽已有每文件上限、
私有根和原子替换，但 `PostForm`/`MultipartForm` 会在业务检查前完整读取无界聚合 body；legacy
directory/rename/import JSON 仍无 actual-read/single-document/cardinality 边界。更严重的是
`localStorePath` 只做 lexical prefix，后续 `Stat/Open/Mkdir/Rename/RemoveAll` 和导入读取会跟随挂载卷
中的 symlink，无法证明最终文件仍位于当前用户根。

固定上游多选上传、当前 OpenReader 稳定路由/响应/逐文件提交、允许的安全差异、multipart/JSON
精确边界、symlink-safe regular-file 读取和测试门禁见
[`local-store-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](local-store-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同 `69145fc`、旧实现红测 `8c78775`、实现 `bba99e1` 和真实 HTTP 探针 `930be4d` 已依次推送。
现在 multipart 总包络、1..64 part/metadata、临时文件所有权、16 KiB/1 MiB single-JSON、200 项请求/
目录展开、逐组件 symlink/special-file 拒绝和 opened regular-file 复验均已落地；固定上游多文件顺序、
逐文件提交、旧路由/响应、private root 和 immutable token 保持。

Go focused/full/race/vet、frontend 740/740、build、宿主和本机 arm64 candidate 真实 HTTP 均通过。
后续 `65a9870` 重新通过 LocalStore candidate 探针、fresh/historical/portable 卷、跨用户与重启，并由
本机发布为同名标签与 `latest`。远端 amd64/arm64 OCI index 为
`sha256:255c81b43dbb7f49c707d6c609b920aa183b730401ad1c1ca32157eb0a945c71`；GHCR 回拉 health revision
一致。当前状态为 **aligned / Docker-published / awaiting-device-verification**。

## 17. WebDAV 导入/恢复文件系统与请求边界（2026-08-16 implemented）

再次扫描 `server.go` 与所有仍直接使用 `ShouldBindJSON`/multipart form 的 handler 后，下一项 must-fix
选择三个 mounted WebDAV 读取动作，而不是重开已签收的原生 DAV 协议：

- `POST /api/webdav/import-preview`
- `POST /api/webdav/import`
- `POST /api/backup/restore-webdav`

固定上游 `WebDAV.vue`、`WebdavController.kt` 与 `BookController.importFromLocalPathPreview` 证明：用户
从唯一 WebDAV 文件管理器显式选择有序文件，预览后逐本确认；恢复只在确认一项 ZIP 后执行，mounted
源保留。当前 OpenReader 的稳定 JSON 路由、private root、immutable token、logical/portable ZIP 和
逐项结果是允许适配，不能被改回上游弱路径拼接或 token-in-URL。

旧实现三个 handler 无 actual-read/single-document JSON；import path/category/目录展开无 cardinality。
`webdavPath` 仅对顶层请求 path 执行 `Service.Resolve`，随后目录内项进入 `WalkDir`/`os.Open`，因此
嵌套 symlink/special file 和 restore 的 path 重新打开没有 caller-rooted opened regular-file 证据。
动作矩阵还发现 direct/source multipart、remote search/debug/cache JSON、progress 和其它 batch JSON
仍待后续排序；本项因同时触及 mounted data、目录展开、parser 与恢复事务而优先。

精确 1 MiB/16 KiB single JSON、200 项 raw/expanded admission、path 正规化、token-source independence、
`Service.Open` identity、restore caller-private snapshot、稳定响应/逐项提交、零迁移与红测/卷门见
[`webdav-import-restore-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](webdav-import-restore-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
一次性 Go overlay probe 实际证明超限+第二 JSON 仍 stage/restore、目录内根外 symlink 字节进入 stage，
且一个目录可返回 201 个 preview item。合同 `cf46e22`、正式旧实现红测 `1bb904a`、实现与真实 runtime
探针 `616a076` 随后依次推送：三个动作现在按认证优先执行 actual-read UTF-8 single JSON，完整规划
最多 200 个唯一 target；source-backed 文件逐项用 caller-rooted `Service.Open`，token-only 不访问 mounted
root；restore 使用 opened regular ZIP 的 caller-private bounded snapshot。

Go focused/full/race/vet、frontend 740/740、build、宿主/candidate declared/chunked + symlink/FIFO +
token/snapshot 探针，以及 1440x900/390x844/360x800 导入、token 重试和恢复会话隔离均通过。
`65a9870` 又通过 fresh/historical 三卷和 logical/portable v1/v2、跨用户、重启，由本机发布为同名标签
与 `latest`；amd64/arm64 OCI index 为
`sha256:255c81b43dbb7f49c707d6c609b920aa183b730401ad1c1ca32157eb0a945c71`。状态为 **aligned /
Docker-published / awaiting-device-verification**。direct/source multipart、remote-work JSON、progress 及其它
batch JSON 仍保留在下一轮动作差集，不因本项完成而误报关闭。

## 18. 直接本地图书多选与 multipart 边界（2026-08-16 implemented）

按第 17 节保留的差集核对固定上游 `Index.vue#onBookFileChange/importMultiBooks`、
`BookController.importBookPreview` 和旧 `OverlayBookImport/useOverlayBookImport/imports.go` 后，direct
local import 曾被确定为 must-fix：固定上游隐藏 chooser 支持多选，单本进入确认，多本选择批量或逐一，
并与 LocalStore/WebDAV 复用同一状态机。

`05343ec` 保留单 object preview/import API、caller-scoped token 和 prepared snapshot，以顺序单文件
adapter 聚合 1..64 项并接入 shared workflow；旧 direct composable 已删除。三个 direct multipart 路由
在任何 `PostForm/FormFile` 或 stage 前执行 `maxLocalImportBytes + 1 MiB` declared/actual-read 包络，只
接受一个 file 或 token、有限已知 scalar/category，并由 handler 清理成功解析的 multipart 临时文件。
认证、parser、stage、archive、分类和 durable-only 广播的既有成功/失败语义保持。

精确上游证据、可见状态、允许适配、wire/field/error/副作用和测试先行门见
[`direct-local-book-import-multipart-workflow-fixed-baseline-second-audit-p1-contract.md`](direct-local-book-import-multipart-workflow-fixed-baseline-second-audit-p1-contract.md)。
状态为 **aligned / regression-validated / Docker-published / awaiting-device-verification**。`cd8f073` 先使旧实现
在 direct 多选/共享状态机、declared/chunked、ambiguous part、metadata/category 和临时文件所有权上
正式变红，`05343ec` 实现，`3b9ae54` 补齐真实 HTTP 与三视口 browser 证据；frontend 737/737、Go
full/race/vet、build、fresh/historical/portable 卷门和 GHCR 回拉 revision 均通过。本机发布
`429444a`/`latest`，OCI index 为 `sha256:41f430a5fbf944b9a1dcf25aec6c9f6e92a11a3ff75e395d1a73120da5a6f4d5`；
下一步从当前 server 重新生成动作差集。

## 19. 当前 `server.go` 动作差集与远程工作优先级（2026-08-16 inventory）

以 `649f2eb` 的 `backend/api/server.go` 重新枚举 route，并逐个反查仍直接使用
`ShouldBindJSON`、`FormFile` 或只做 `MaxBytesReader + ShouldBindJSON` 的 handler。已发布 direct、
LocalStore、WebDAV、upload、auth/admin、BookGroup、Bookmark、RSS、BookSource 写边界不因旧调用痕迹
重开；本轮开放差集如下：

| 候选 | 当前放大面 | 既有合同覆盖 | 排序 |
|---|---|---|---|
| remote-work JSON：`/search`、三个 `/sources/:id/test*`、`/sources/batch-test`、两个 book cache | 小 JSON 可触发多源/多章远程工作；旧 search 无 60/八轮上限，旧 batch health 按全部源预建 goroutine，七路曾缺统一 actual-read/single-object。 | 已补 64/16 KiB 单 object、字段/cardinality、60 并发/八窗口、15-worker/300-source、Context 取消，并保留搜索 cursor、诊断 envelope、failure cache 和整本缓存。 | **aligned / regression-validated / Docker-published / awaiting-device-verification**。见 [`remote-work-request-boundary-fixed-baseline-second-audit-p2-contract.md`](remote-work-request-boundary-fixed-baseline-second-audit-p2-contract.md)。 |
| BookSource local multipart `/sources/import` | 原始浏览器 File 现于 `text()` 前执行 16 MiB admission；API 具有 17 MiB actual-read 包络、严格单 file/零 scalar、稳定双层 413 与 handler-owned cleanup。 | selected payload bytes/cardinality/quota/COW 保持；raw chooser、multipart shape/error/temporary ownership 已有红测、运行时与发布证据。 | **aligned / regression-validated / Docker-published / awaiting-device-verification**。见 [`booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md`](booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md)。 |
| reading progress `/progress` | 旧无界首 JSON、缺省 chapter index 与无短界 CAS 控制字段已由 16 KiB 单 UTF-8 object、显式 identity 和 RFC3339Nano/字段长度 admission 关闭。 | 进度身份、CAS、镜像、冲突和多客户端收敛保持；`f924604` 合同、`a10facb` 红测、`8d3790d` 实现、`1563bc3` runtime 已完成并随 `65199f6` 通过卷门/正式发布。 | **aligned / regression-validated / Docker-published / awaiting-device-verification**。 |
| ReplaceRule control JSON | books control 已关闭；重新扫描所得五路 create/update/batch/batch-delete/test 已统一移除 direct Gin binder，使用 actual-read 单 UTF-8 文档、稳定 413 和 request-context GORM transaction。 | 已发布的 UI/Reader/SQLite/backup 合同保持关闭；512 KiB/16 MiB/128 KiB/4 MiB、2,000 raw row/ID、PUT target-first、精确字符串和 durable-only event 均保持。 | **aligned / regression-validated / Docker-published / awaiting-device-verification**；见 [`replace-rule-request-boundary-fixed-baseline-second-audit-p2-contract.md`](replace-rule-request-boundary-fixed-baseline-second-audit-p2-contract.md)。 |

固定上游 `BookController.searchBookMulti` 与 SSE 版本使用可见最高 60 并发和
`concurrentLoopCount=8`；inventory 时 OpenReader 只有前端枚举，没有服务端 60/八轮边界。旧健康检查
虽把执行并发夹到 15，却按源数创建等待 goroutine。由此 remote-work 的网络/内存放大风险高于
progress，成为本轮实现切片。BookManage 整本缓存是明确上游产品合同，本轮只补 wire/cancel，没有借
安全收紧改回固定章数。

该 inventory 阶段本身只新增/更新文档；后续已按 `5aadf9b` inventory、`94d0a4e` 旧实现红测、
`346a49d` 实现和 `6157466` 真实浏览器合同顺序关闭。Go full/race/vet、frontend 737/737、build、
三视口真实 Go/Chromium、source-debug smoke 和 fresh/historical/portable 卷门通过；本机发布
`6157466`/`latest`，OCI index 为
`sha256:1e890a60a1b75879dd99074b1da13b17f91bbd4173e945b92cb8cec0fe8001b6`。历史卷首次普通运行出现一次
fixture 后瞬时 404，同镜像 trace 重跑全链通过。BookSource multipart 后续亦按独立合同关闭；下一轮
reading progress 后续已由独立合同关闭；下一轮仍须从其它 batch/control JSON 重新取证，不能因这些项目
关闭而合并签收。

## 20. BookSource 本地导入 multipart 与前端预读（2026-08-16 implemented）

固定上游 chooser 只取第一项，正常路径由浏览器 `FileReader` 解析非空数组，失败 fallback 才把该文件
交给 `readSourceFile`，最终以用户勾选数组调用 `saveBookSources`。OpenReader 的规范前端同样只取第一项，
并在确认后生成唯一 `file=bookSources.json` 调用合并的 `/api/sources/import`，因此严格单 file/零 scalar
不会删减可见流程。

`54f2c83` 的后端已有 17 MiB `MaxBytesReader` 与 16 MiB file read，但 `c.FormFile` 会忽略额外同/异名
file 与 scalar，body overflow 又会降级成 `400 file is required`，handler 也未局部拥有 multipart temp。
更重要的是，前端原始 chooser 在任何 size 检查前调用 `file.text()`，所以既有 16 MiB 声明只覆盖预览
后重新生成的 selected-source Blob，不覆盖用户选入浏览器的原文件。

精确上游映射、17/16 MiB 双层 413、单 file shape、错误优先级、显式 `RemoveAll`、前端预读和必须先写
的失败测试见
[`booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md`](booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md)。
后续按 `d7bc00a` inventory、`ddbac4c` 旧实现红测、`8c66dc9` 实现和 `3f3c9c8` 真实运行时合同顺序
关闭。前端已在读取前拒绝已知超限 File；后端已落实认证优先、17 MiB actual-read、唯一 file/零 scalar、
16 MiB file、稳定双层 413 和 form `RemoveAll()`。Go full/race/vet、frontend 738/738、build、三视口真实
Go/Chromium、fresh/historical/portable 与 source ownership 门通过。本机发布 `3f3c9c8`/`latest`，OCI
index 为 `sha256:62ee55ffab7859aef4334f8fb8dd31520953521da494edd5f37cc56741731070`；状态为
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 29. 本地书归档文件系统生命周期（2026-08-24 implemented/published）

继续按持久 path 与 destructive side effect 枚举后，下一项 must-fix 覆盖
`GET /api/books/:id/chapters/:index/content`、`POST /api/books/:id/refresh-local`、
`POST /api/books/export` 的本地书分支及 Book 删除后的 archive cleanup。当前 resolver 把
resolved owner root 当作新信任根，refresh stage 还直接使用 lexical private path；因此
`library/data/<user>` symlink 可把 source/cache 读、generation/metadata 写和最后引用删除带到
`LibraryDir` 外。

2026-08-24 的 `/tmp` 真实 Go/HTTP 探针已证明 refresh/read 返回库外正文并写入库外，batch delete 在
提交 Book 删除后真实删除库外书目录。original export 自身因额外 library check 回退，但生成式导出仍会
消费同一不安全 cache/source 链。固定上游本地书可见行为、旧绝对字段 rebase、原 archive、进度/书签、
导出格式和 path-free envelope 均保留；目标只补 `LibraryDir -> owner -> book -> entry` 逐组件边界、
same-file opened handle、rooted refresh stage/promote/prune 和 identity-safe delete detach。

完整合同与红测/卷门见
[`local-book-archive-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md`](local-book-archive-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md)。
合同 `cae9bf2`、alias 勘误 `852df65`、红测 `92a5fa4`、实现 `125fd93` 已顺序推送；修复后真实 HTTP
探针、focused/full/race/vet、frontend 741/741、build、Reader/BookManage 三视口和 fresh/historical/portable/
restart 卷门通过。本机发布 `125fd93`/`latest`，OCI index
`sha256:777ca720981b8a3529009211ce179b430bb354cb01e2957681f191036699f6a5`。状态：
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

reading progress 后续已由独立合同关闭；其它 batch/control JSON 继续排队，不因 BookSource multipart
关闭而合并签收。

## 30. 备份上传恢复 multipart 请求边界（2026-08-25 implemented/published）

重新生成当前 route/wire 差集后，下一项 must-fix 收敛到 `POST /api/backup/restore-legado`。已发布的
compressed/expanded ZIP budgets、logical/portable restore、caller/source permission、transaction/
rollback、WebDAV opened snapshot 均保持关闭；本轮只补 multipart singularity 和 handler-owned temp
cleanup。

当前 `c.FormFile("file")` 忽略额外 scalar、同名/异名 file，且 handler 不调用
`MultipartForm.RemoveAll()`。`7045827` 一次性 overlay 探针提交合法 ZIP、scalar 与 34 MiB 额外 file 后
仍恢复 Book，并在直接 handler 返回后留下 temp。固定上游单 ZIP 确认动作、OpenReader 稳定 route/error/
counts 和精确 red-test 门见
[`backup-restore-multipart-request-boundary-fixed-baseline-second-audit-p2-contract.md`](backup-restore-multipart-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同 `7a2a44a`、红测 `20ac551` 和实现 `a0fb1bd` 已按测试先行顺序关闭。focused/full/race/vet、
frontend 741/741、build、真实 HTTP declared/chunked/shape/temp 探针、WebDAV restore 三视口及
fresh/historical/portable/restart 卷门通过。本机发布 `a0fb1bd`/`latest`，OCI index 为
`sha256:b25f5b05df983532bf656ec8647e553188db3ba7fb291b826cb45b65deae6f3c`。状态：
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 21. 阅读进度 JSON 与 CAS 控制字段（2026-08-16 implemented）

固定上游 `saveBookProgress` 只提交书籍 URL 和显式目录 index；POST 缺省 index 为 `-1`，服务端先确认
当前用户书架书籍，再从目录取得规范章节。OpenReader 保留已发布的数字 identity、精确位置、SQLite
CAS、200 conflict、WebSocket 和 existing-directory WebDAV 镜像，不重开该业务合同。

`cfb06e9` 的 `PUT /api/progress` 仍直接 `ShouldBindJSON`：Gin 1.10 只 decode 第一个 JSON value，没有
actual-read 上限或 UTF-8/single-document 检查；value `chapterIndex` 缺省为 0，省略字段可误写第一章。
`baseUpdatedAt/clientUpdatedAt` 无长度/语法检查，非法非空时间可落入 non-stale fallback；持久 `mode`
与广播 `clientId` 也没有应用层短界。

本轮裁决为 auth-first 16 KiB 单 UTF-8 object、显式 book/index、64-byte RFC3339Nano timestamps、20-byte
mode 和 128-byte clientId；所有拒绝必须先于 service/SQLite/WebDAV/Hub。精确合同和必须先写的测试见
[`reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md`](reading-progress-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
后续按 `f924604` 合同、`a10facb` 旧实现红测、`8d3790d` 实现和 `1563bc3` 真实运行时合同顺序关闭。
Go full/race/vet、frontend 741/741、production build，以及 1440x900、390x844、360x800 的真实
Go/Chromium 请求边界、client ID 自愈、双客户端 CAS/WebSocket、冷恢复和 WebDAV mirror 均通过。
`1563bc3` 本机 Docker 候选当时因 socket 使用额度未完成卷门；后续 `65199f6` 重新通过
fresh/historical/portable 门并由本机发布。`65199f6`/`latest` OCI index 为
`sha256:57eda43d437d98a4f2d748164d58c5816f3ff3dc199397bd9dc8f6d48334a8cb`，强制回拉 arm64 revision
通过。当前状态为 **aligned / regression-validated / Docker-published / awaiting-device-verification**；
其它尚未签约 action 继续排队。

## 22. Book 控制动作 JSON、cardinality 与取消（2026-08-16 implemented/published）

重新枚举 `books.go` 中仍直接 `ShouldBindJSON` 的入口后，下一项 must-fix 收敛为 batch、export、
refresh-local、remote add、change-source 和 legacy content-search POST。固定上游证明这些分别对应已签收
的 BookManage、BookInfo、本地重解析、入架、Reader 换源和正文搜索状态机；本轮不改可见流程或成功
语义，只关闭请求可放大的 wire/work 边界。

当前六路都接受首个 object 后的第二 JSON 且无 actual-read/UTF-8 admission；batch category IDs 无 raw
cardinality，batch cache 和 remote add/change 使用 `context.Background()`，local refresh 又在读取整个
原书档后才绑定可选规则。完整 16 KiB/32 KiB/1 MiB、200 项、Book 字段、16 KiB TOC rule、legacy 200 envelope、
target-first priority、取消及零迁移合同见
[`book-control-request-boundary-fixed-baseline-second-audit-p2-contract.md`](book-control-request-boundary-fixed-baseline-second-audit-p2-contract.md)。

后续已按 `097c862` 合同、`669aa5b` 32 KiB TOC 包络勘误、`5cc4b18` 旧实现红测和 `65199f6` 实现
顺序关闭。六路已执行 single UTF-8 object/actual-read admission，category/Book/TOC 短界和 request
context 取消，同时保留完整导出、本地原文件、换源 transaction、legacy 200 envelope 和零迁移。
Go focused/full/race/vet、frontend 741/741、build、三视口真实 Go/Chromium、fresh/historical/portable
卷门与 GHCR arm64 强制回拉 revision 均通过。本机发布 `65199f6`/`latest`，OCI index 为
`sha256:57eda43d437d98a4f2d748164d58c5816f3ff3dc199397bd9dc8f6d48334a8cb`；状态为
**aligned / regression-validated / Docker-published / awaiting-device-verification**。下一轮必须从剩余
server action 重新取证，不因本项签收而合并关闭 replace-rule 等 first-document 长尾。

## 23. ReplaceRule JSON 与事务取消（2026-08-16 implemented/published）

重新枚举 `backend/api` 后，剩余直接 `ShouldBindJSON` 只有 `replace_rules.go` 的 create、update、batch、
batch-delete 和隐藏 test。固定上游证明 object/array shape、精确字符串、name-upsert、输入顺序和 skip
语义；当前业务层已有字段、2,000 row/ID、RE2、match/output、事务、owner 和 durable-only broadcast，
因此本轮不重开已由 `a7abcdd` 发布的管理器、Reader、SQLite 或 backup 合同。

inventory 时五路的 `MaxBytesReader + ShouldBindJSON` 只消费首个 JSON，actual overflow 被各路普通 400
吸收，非法 UTF-8 又可被替换后进入精确规则字段；GORM/batch transaction 也没有 request context。完整
边界、旧实现红测和实施证据见
[`replace-rule-request-boundary-fixed-baseline-second-audit-p2-contract.md`](replace-rule-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同 `ff6d7e3`、红测 `c70f04e`、实现 `9f5a52b` 已依次关闭五路 actual-read/single UTF-8 document、稳定
413、PUT target-first 和 pre-commit transaction 取消；拒绝零写入/零广播，持久提交仍发一次既有事件。
focused/full/race/vet、frontend 741/741、build、真实 Go HTTP、ReplaceRule 四视口及 fresh/historical/portable
卷门通过。本机发布 `9f5a52b`/`latest`，OCI index 为
`sha256:7a72f2d01b26d1d28c35bb13970cb64a1f7dbf97ddebc3aa704957f58f2f56c3`；远端 arm64 config 通过 Registry
API 确认完整 revision，Docker CLI 强制回拉仅受本机 `osxkeychain -50` 阻断。状态为
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

`backend/api` 当前 direct `ShouldBindJSON`/`ShouldBind` 差集为空，但这只关闭该类 binder；下一项仍须按
路由、工作放大、事务和错误优先级重新枚举，不能据此宣称全部 REST action 已签收。

## 24. 备份生成错误与请求取消（2026-08-16 inventory）

改按 route/work amplification 枚举后，`POST /api/backup/trigger` 和 `/backup/portable/trigger` 成为下一项
确定 must-fix。固定上游 `backupToWebdav` 失败只返回“备份失败”；OpenReader 已发布的普通备份 API
合同同样要求安全固定 500，但当前 handler 仍把 `Service.run` 的 `err.Error()` 拼入响应。底层 OS/GORM/
ZIP 错误可含 mounted path、SQL 或内部归档信息。

两个生成服务也都没有 context：取消请求仍会等待全局锁并继续数据库查询、ZIP/asset/archive I/O 直到
rename。现有 temp+rename、caller scope、logical/portable 格式和 typed 409/413 均保持；下一切片只增加
HTTP context lifecycle、安全错误投影与 path-free 日志，final rename 前取消清理 temp，rename 后 durable
包不补偿删除。完整合同和红测门见
[`backup-generation-request-boundary-fixed-baseline-second-audit-p2-contract.md`](backup-generation-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同和旧实现红测已依次完成；当前实现把两个 handler 的 request context 贯穿可取消生成 gate、GORM
snapshot、logical entry、archive/asset validation/copy、ZIP close、sync 和 rename 前提交边界。普通 trigger
固定安全 500，portable typed 409/413 保持；预取消零查询、锁 waiter、logical/portable 中途取消、temp 清理、
rename 后 durable ZIP 与 path-free 日志均有测试。focused/backup/race、Go 全量/vet、frontend 741/741 与 build
通过；真实 HTTP safe-500/success/list/download/128 MiB portable cancel 与 fresh/historical/portable 卷门亦
通过。本机发布 `cd3a17c`/`latest`，OCI index 为
`sha256:08e9a5ba94646e5955e9c0d4586a4be95d004d6a015b518331c02748a9e53f70`，远端 amd64/arm64 config
均确认完整 revision。当前状态 **aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 25. 备份 list/download 文件系统读取边界（2026-08-17 implemented/published）

generation lifecycle 发布后按 path-traversal 与 route/work amplification 继续枚举，下一项确定 must-fix
收敛到 `GET /api/backup/list` 和 `/backup/download/:name`。既有合同只允许 caller root 内
`backup_*.zip`/`portable_backup_*.zip`；当前 `backupFileNameAllowed` 却只检查前缀，list 信任 `ReadDir`
entry，download 再由 `c.File(path)` 跟随路径。

已发布 `cd3a17c` 的真实 HTTP 反例证明：regular user 的 allowed-name symlink 可返回 caller root 外内容，
前缀匹配非 ZIP 返回 200，目录 direct download 返回 301，list 还暴露 symlink/非 ZIP。完整 scoped-root、
opened-handle、same-file、format、错误和测试先行门见
[`backup-list-download-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md`](backup-list-download-filesystem-request-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同 `b9deec2` 与旧实现红测 `d7810ca` 已按顺序完成。当前实现以 `webdavfs.NewScoped` 约束 caller root，
list 对严格 ZIP basename 逐项执行普通文件/same-file open，并从同一句柄取得 metadata 与 portable format；
download 直接发送该句柄，不再由 `c.File(path)` 重开。missing/unsafe/non-regular 使用固定安全 404，
其它打开失败使用固定安全 500。focused/race、Go 全量/vet、frontend 741/741 与 build 已通过。

真实 HTTP symlink/non-ZIP/directory/FIFO/valid/ancestor、跨用户同名与 path-free 错误探针，以及
fresh portable-v1/v2-assets/cross-user/restart 和 historical TXT/EPUB/UMD/CBZ/relative-cache/
owner-isolation 卷门均通过。本机 amd64/arm64 发布 `2986357`/`latest`，OCI index 为
`sha256:bdb8195077000a898569e0f3f6664a5760c2b56058d67b2d6ae1d4aaf42fea5e`；远端两平台 config 均确认
完整 revision。当前状态 **aligned / regression-validated / Docker-published / awaiting-device-verification**；
generation/restore/格式未重开。

## 26. 上传资产公开读取文件系统边界（2026-08-17 inventory）

继续枚举 `server.go` 的非 `/api` 文件读取面后，下一项确定 must-fix 收敛到
`GET|HEAD /uploads/*resourcePath`。固定上游公开 `/assets/<namespace>/...` 供封面、背景和字体直接加载；
OpenReader 的 `/uploads` capability、JWT user-ID 随机路径和 legacy 全局 URL 是已签收适配，不能改成
Bearer-only、短期签名或新 URL。

当前 `router.Static` 使用 Gin `onlyFilesFS(http.Dir)`：它只禁用目录 `Readdir`，仍跟随 root/ancestor/
entry symlink，并先 open/close 检查、再由 `http.FileServer` 按 path 重开。已发布 `2986357` 的真实容器
反例确认 entry symlink 与 ancestor symlink 均返回 200 和 uploads root 外 bytes。完整 GET/HEAD/Range、
rooted same-file-open、special-file、legacy/portable/data 与测试门见
[`upload-public-read-filesystem-boundary-fixed-baseline-second-audit-p2-contract.md`](upload-public-read-filesystem-boundary-fixed-baseline-second-audit-p2-contract.md)。

合同 `d0c948c` 与旧实现红测 `7181634` 已按顺序完成。当前实现以显式 GET/HEAD handler 替代
`router.Static/http.Dir`，复用 `webdavfs.New/Open` 的逐组件 symlink、普通文件与 same-file 验证，并从
同一 `*os.File` 执行 `http.ServeContent`。root/ancestor/entry symlink、反斜杠、目录、FIFO、安全 404 与
legacy/current GET/HEAD/304/Range 均有测试；focused/race、Go 全量/vet、frontend 741/741 与 build 通过。

真实 Go + Chromium 三视口、本地候选与 GHCR 回拉 HTTP 探针、fresh portable-v1/v2-assets/
cross-user/restart 与 historical TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation 卷门均通过。
本机 amd64/arm64 发布 `277e512`/`latest`，OCI index 为
`sha256:ca50fd59dce4f4bb13a1450ee7ee39b2a3d7b392de3902a7f3c21272e8ac9c70`，远端两平台 config 均确认
完整 revision `277e512fa1a0135cff4089298d4644ee72ddf518`。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**；upload write/delete、
BookInfo/Reader/portable v2 合同不重开。

## 27. 远程章节文本缓存文件系统生命周期（2026-08-17 inventory）

公开 upload read 发布后继续按 mounted path 与数据引用枚举，下一项 must-fix 不只是
`GET /api/cache/stats`/`DELETE /api/cache`，而是远程章节文本缓存的读、写、统计、剪枝和
DB path 发布共享生命周期。固定上游只统计/删除当前 namespace 书架中远程书的实际
`.txt` 缓存；OpenReader 当前用户级 JSON stats/clear 和共享物理 path 是已发布多用户适配。

当前 `remoteCacheFilePath` 只做 lexical prefix，后续 `os.ReadFile/Stat/Remove` 会跟随 mounted
root/ancestor/entry symlink；`WriteChapterCache` 的 `MkdirAll/WriteFile` 也可被 ancestor/entry symlink
扩展到 `cache/` 外。stats 会把 missing/unsafe 非空 DB path 计为 cached chapter；prune 在全局
引用查询失败时反而继续删候选，且文件写入→DB 发布与 DB 清行→剪枝无共享串行边界。

详细 rooted opened-file、原子写、实际文件统计、all-user reference fail-closed、write/prune 并发、
历史 relative/current-absolute path 和无迁移边界见
[`remote-chapter-cache-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md`](remote-chapter-cache-filesystem-lifecycle-fixed-baseline-second-audit-p2-contract.md)。
`f8e5c04` 先在旧实现上锁定读/写/统计/删除和引用查询失败反例；`75cc238` 已实现 rooted
同句柄有界读取、原子可取消写入、实际文件统计、全用户引用 fail-closed 和 write/prune 共享串行边界。
专项/focused race、Go full/vet、frontend 741/741、build、Reader/BookManage/侧边栏真实浏览器、宿主与
候选 mounted probe、named-volume restart 及 fresh/historical/portable 卷门均通过。本机发布
`3cef8df`/`latest`，OCI index 为
`sha256:8cfe72e56af0cbb191d6b31fa243153a3ce14010614c5153881b262229facf86`；两平台回拉 config 均确认
完整 revision `3cef8dfdccd45970596b3d8916a2cb6fab1480dc`。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 28. 公开 capability 文件读取生命周期（2026-08-17 implemented）

远程章节文本缓存发布后继续枚举公开读取路由。旧实现中 `/api/epub-resource`、
`/api/cbz-resource` 和 `/api/audio-resource` 在 service 完成 capability/owner/path/fingerprint 检查后
只返回 path，随后由 service 或 handler 再次 `os.Open/http.ServeFile`；cached `/api/cover` 也存在
`Lstat -> Open -> Chtimes` 而没有 same-file 证明。`df49535` 已在旧实现上锁定 mounted replacement
反例，`a90f7b3` 已把四路收敛为 rooted same-file opened handle，并让 handler 只消费该句柄。

固定上游的 `/assets/*`、`/epub/*` 与本地 EPUB/CBZ 章节投影只要求浏览器能读取对应资源；OpenReader
私有 generation、用途隔离 capability、CSP/MIME/Range 和多用户 owner 是必须保留的安全适配。本切片
不改变 URL、token、派生目录、原 archive、响应 schema 或备份格式，只把授权、路径验证和响应收敛到
rooted same-file opened handle。完整合同见
[`public-capability-filesystem-read-lifecycle-fixed-baseline-second-audit-p2-contract.md`](public-capability-filesystem-read-lifecycle-fixed-baseline-second-audit-p2-contract.md)。

章节图片 capability 在响应前按 token fingerprint 重验最终内存字节，公开 uploads 已由独立合同关闭，
二者不并入本轮。focused/race/full/vet、frontend 741/741、build、EPUB/CBZ/audio/真实 cover 三视口、
宿主和 `5313c49` 候选容器 HEAD/Range/mounted-symlink probe，以及 fresh/historical/portable/restart
卷门均已通过。最终证据提交由本机发布为 `5e63eb1`/`latest`；OCI index 为
`sha256:8b7bc4cd8542f79eccc54d393cf2d79041f5fe9a90b05776c473cd3f1e4c2cee`，amd64/arm64 manifest 分别为
`sha256:f5ac952ead7e410c3debe7c2892f84cc5d6e0212a3982c3cd18209e084186c3d` 和
`sha256:a7a243fe0bc4083ebdb6f4f1df5206315a6d3cbac05544525544e44244663d76`；远端两平台 config 与回拉
arm64 runtime 均确认完整 revision `5e63eb1854a95dc3fd79ebafec89f4723f37f8da`。状态：
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 31. HTTP 服务生命周期（2026-08-25 implemented/published）

备份 multipart 发布后重新枚举 route 以外的进程级 HTTP 边界，下一项 must-fix 收敛为 Go PID 1 的
请求头与停止生命周期。固定上游 startup 显式使用 512 KiB header 上限，shutdown 脚本发送标准终止
信号；当前 `router.Run` 沿用 Go 1 MiB 默认 header 且不接管 `SIGINT`/`SIGTERM`，因此容器停止会直接
跳过 cleanup、scheduler 和 backup defer，也不等待普通在途请求。

目标只引入显式 `http.Server`、10 秒 header deadline、512 KiB header、有界 8 秒 drain、WebSocket hub
关闭和幂等 cleanup；全局 read/write timeout 必须保持为零，以免截断章节缓存 SSE、书源调试 SSE、
WebSocket、大文件上传或备份。完整固定上游证据、目标合同、红测与容器门见
[`http-server-lifecycle-fixed-baseline-second-audit-p2-contract.md`](http-server-lifecycle-fixed-baseline-second-audit-p2-contract.md)。
合同 `5b06084`、旧实现红测 `6bee4e0` 与实现 `f394c1a` 已依次推送。focused/race/full/vet、frontend
741/741、build、真实二进制 header/listen/SIGTERM/WebSocket、candidate `docker stop` 及
fresh/historical/portable/restart 卷门通过。本机发布 `f394c1a`/`latest`，OCI index 为
`sha256:4af0cf100434ed852fdf6727d351425cca6935c8f7f6a00eaec220de9865eafa`。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 32. 访问日志 Query 脱敏（2026-08-25 implemented/published）

HTTP 生命周期发布后继续枚举 process/middleware 边界，下一项 must-fix 收敛为共享 access log 的 query
投影。固定上游保留 method/path 运维日志但会记录完整 URI；OpenReader 已明确遮蔽 WebSocket JWT 和
capability token，却仍让普通 query 原样进入 Gin log。真实 `f394c1a` 证明任意 200 health 请求可把阅读
短语、编码 userinfo 和 token 写入日志，且日志长度随 query 放大。

目标统一保留 method/status/latency/client IP/path，将任何 query 整体替换为固定 `?<redacted>`，不解析、
不 decode、不改变 handler `c.Query` 或任何 API 行为；现有 capability path 脱敏保持。完整固定上游证据、
真实反例和红测门见
[`access-log-query-redaction-fixed-baseline-second-audit-p2-contract.md`](access-log-query-redaction-fixed-baseline-second-audit-p2-contract.md)。
合同 `9161ce5`、旧实现红测 `cce9efd` 与实现 `f88ecec` 已依次推送。focused/race/full/vet、frontend
741/741、build、真实二进制 200/401/404/256 KiB query 及 fresh/historical/portable/restart 卷门通过。本机
发布 `f88ecec`/`latest`，OCI index 为
`sha256:832216dbacb0650a5a6cb30b14731432714f4d48393516aed10c957a97549a29`。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 33. 可信代理、客户端身份与限流（2026-08-25 inventory）

访问日志 query 脱敏发布后继续枚举 process/middleware 边界。当前 `gin.New()` 没有覆盖 Gin 1.10 的
“信任全部代理”默认值，而全局 `RateLimiter` 和 access log 都以 `c.ClientIP()` 为身份。真实二进制将
limit 设为 1 后，同一 TCP peer 使用同一 `X-Forwarded-For` 的第二次请求为 429，只变换 header 即再次
得到 401；日志也把两个伪造值记录为客户端地址。

固定上游 Nginx 文档证明显式反向代理是合法部署，但没有把任意直连请求方声明为可信代理，也没有
客户端可自行选择限流桶的行为。目标是默认忽略 forwarded client-IP header，并以可选
`OPENREADER_TRUSTED_PROXIES` 显式列出代理 IP/CIDR；非法配置须在 listen 前失败，可信链使用 Gin
右到左算法，limiter/logger 共用同一验证身份。现有路由、429 envelope、限流豁免、CORS、JWT、
WebDAV、graceful shutdown 和日志脱敏保持。完整合同见
[`trusted-proxy-client-identity-rate-limit-fixed-baseline-second-audit-p2-contract.md`](trusted-proxy-client-identity-rate-limit-fixed-baseline-second-audit-p2-contract.md)。
合同 `30b7630` 与旧实现红测 `db89593` 已按顺序推送。当前实现默认不信任 forwarded client headers，
只接受 `OPENREADER_TRUSTED_PROXIES` 明确列出的 IP/CIDR；空项/非法项在任何持久或后台工作及 listen 前
失败。focused/full/race/vet、frontend 741/741、build、Compose、README 配置映射及真实二进制三组探针
通过。GitHub Actions `32828470325` 又通过原生镜像与 fresh/historical/portable/restart 卷门，发布
`f5b3869`/`latest`；amd64/arm64 OCI index 为
`sha256:6a2fc83bf79426e93423b1dd5756c8ea49b716d1321441d5c194efff9c03b066`。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 34. 前端静态资源与 SPA 路由失败分流（2026-08-25 implemented/published）

可信代理发布后继续枚举 process/router 边界。固定上游以 hash router 配合普通静态 handler，不把未知
服务端请求改写成首页；OpenReader 为 Vue history route 增加 SPA fallback 属于必要技术适配，但当前
`serveFrontend` 对所有 `NoRoute`、所有 method 都无条件发送 `index.html`，同时只挂载 `/assets`。

真实 `acaae61` 二进制证明未知 `/api/*`、`/ws/*`、缺失 `/assets/*.js`、`PATCH /api/health`、未知页面
和 `POST` 前端 route 都是 `200 text/html`；构建输出中的 `/manifest.webmanifest`、`/openreader.svg`
也只能得到首页 HTML。目标是 rooted same-file 服务根级普通构建文件，只允许已注册 Vue history route
在 GET/HEAD 回退，并让未知 route 与已注册路径错误 method 分别使用安全 JSON 404/405；WebDAV、
OPTIONS、uploads/capability、CORS、日志和限流优先级保持。

完整上游证据、路由 allowlist、wire envelope、文件安全边界和测试门见
[`frontend-static-spa-route-boundary-fixed-baseline-second-audit-p2-contract.md`](frontend-static-spa-route-boundary-fixed-baseline-second-audit-p2-contract.md)。
合同 `c079857`、勘误 `575f269`、旧实现红测 `3c87c89` 与实现 `bf114a6` 已按顺序推送。focused/race/
full/vet、frontend 741/741、build、Compose、真实 Go 200/401/404/405/MIME/HEAD 与三视口 Chromium
深链接通过。受信 Actions run `32847847945` 又通过原生候选、fresh/historical/portable/restart、双架构
发布和平台核验；`bf114a6`/`latest` OCI index 为
`sha256:ed700c5e4e04274b47d69a7c6613eeb8a02bb6838f40bbd22e2f53295386b6d3`。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 35. 前端 public 静态子树（2026-08-25 implemented/published）

frontend/static/router 发布后继续核对真实 production build 与 HTTP 路由可达性。Vite 已把
`frontend/public/themes/*` 和 `frontend/public/bg/*` 原样输出到 `dist/`，Reader store 也以
`/themes/*.png`、`/bg/*.jpg` 直接引用；固定上游普通 StaticHandler 同样服务完整 web 静态树。但当前
Go `NoRoute` 只允许 Vue history route 和根级单文件，真实服务对 `themes/content_0.png` 与中文背景
`bg/山水画.jpg` 均返回 JSON 404，manifest 则为 200。

下一项 must-fix 是复用已发布 rooted same-file opener 服务受信 build 内普通 public 子树，同时保持
API/WebDAV/WebSocket/uploads/assets namespace、已知 history route、统一 404/405、GET/HEAD/MIME/Range
和 symlink/特殊文件边界。完整合同与红测门见
[`frontend-public-static-tree-fixed-baseline-second-audit-p2-contract.md`](frontend-public-static-tree-fixed-baseline-second-audit-p2-contract.md)。
合同 `9d32418`、旧实现红测 `525a4b6` 与实现 `5163262` 已按顺序推送。focused/race/full/vet、frontend
741/741、build、Compose、真实 Go bytes/MIME/HEAD/Range/hash 和 1440/390/1024 Chromium Reader
加载/解码均通过。受信 Actions run `32851803480` 又通过原生候选、fresh/historical/portable/restart、
双架构发布和平台核验；`5163262`/`latest` OCI index 为
`sha256:3a70be27680b32d51c11e20f56efa2be4824b12f8dff53135b45153dd2f2758d`。GHCR 回拉容器也确认
health revision 与 theme/bg bytes。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**。

## 36. 认证会话生命周期（2026-08-26 implemented/published）

继续按 auth/middleware/action 差集复审后，下一项 must-fix 收敛到登录后的 JWT 生命周期。固定上游
`saveUserSession/checkAuth/logout` 为每个 token 保存七天期限、认证活动时滑动续期，并在退出时删除当前
token；账号删除后也不再存在可供 token 定位的用户。

旧实现的 REST `AuthRequired` 和 WebDAV Bearer 只验证 HS256 签名与非零 `userId`，JWT 没有 expiry，
前端 logout 只清 localStorage，管理员改密也不撤销 token。该差异已由 `2396537` 红测固定，并由
`a0edce3` 关闭。

实现采用加法 `auth_version + user_sessions + migration marker`：新会话随机、数据库只存 hashed identity、
七天滑动过期、每用户最多 64 个；老 JWT 只在幂等迁移 marker 后七天内收编。新增
`POST /api/auth/logout` 仅撤销当前会话，管理员改密事务递增代次并撤销目标全部会话，删用户事务删除
session；REST、WebDAV Bearer、WS 共用同一权威认证。WebDAV Basic、登录响应、现有用户数据和
`data/cache/library` 不变。完整合同见
[`authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md`](authenticated-session-lifecycle-fixed-baseline-second-audit-p2-contract.md)。
当前状态 **aligned / regression-validated / Docker-published / awaiting-device-verification**。Go full/race/vet、
frontend 742/742、build、四视口浏览器、真实 API 和 Actions run `32914105929` 新旧卷门均通过；
`a0edce3`/`latest` OCI index 为
`sha256:5d7fe23ba96107c5c545e9e44815514fe277e5a6f83eb25cb006859c5d515d78`。

## 37. 默认书源快照文件系统与提交生命周期（2026-08-26 implemented/published）

认证会话发布后从当前 `server.go` 重新枚举尚未签约的 action。下一项 must-fix 收敛到
`GET/POST /api/sources/default*`、目标用户设默认、管理员批量重置及旧卷
`data/defaultBookSources.json` 初始化边界。固定上游完整复制目标用户文件并以存在的空数组表示有效
空默认；OpenReader 的 SQLite namespace 是允许的关系模型翻译，但必须和兼容镜像表达同一快照。

旧实现的无界/symlink 读取、raw status error、A/B 交叉提交和旧卷默认覆盖已由 `6d8b8f1` 红测固定，
`07761b5` 校正串行化测试后，由 `a36b888` 关闭。新实现以 SQLite 为迁移后的运行时权威，固定 JSON
为有界 legacy 输入和 canonical mirror；所有默认动作和启动修复共用生命周期锁，请求 context 贯穿
GORM/文件 I/O，ownership-v1 直接升级保留安全显式 JSON（含空数组）及普通用户旧 source ID。精确 API、
16 MiB/300-source、same-file regular read、取消/并发/恢复和历史卷测试见
[`default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md`](default-book-source-snapshot-filesystem-transaction-fixed-baseline-second-audit-p2-contract.md)。
当前状态 **aligned / regression-validated / Docker-published / awaiting-device-verification**。Go full/race/vet、
frontend 742/742、build、四视口、真实 HTTP、GHCR 回拉和 Actions run
`32919553203` 新旧卷门均通过；`a36b888`/`latest` OCI index 为
`sha256:63979a0e01d8942a9c594d444e6d5cdf28f0ac5c382825f71a051a52b02a21e4`。
