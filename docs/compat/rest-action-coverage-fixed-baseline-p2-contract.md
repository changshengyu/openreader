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
| BookSource local multipart `/sources/import` | 约 17 MiB 总包络与 16 MiB file read 已存在，但 overflow 映射不稳定，仍接受歧义 file/scalar part；原始浏览器 File 又在 size admission 前 `text()`。 | BookSource 写合同把 selected payload bytes/cardinality/quota/COW 标为 aligned；raw chooser、multipart shape/error/temporary ownership 未签收。 | **本轮 must-fix / inventory-complete / implementation-pending**。见 [`booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md`](booksource-local-import-multipart-fixed-baseline-second-audit-p2-contract.md)。 |
| reading progress `/progress` | 无界首 JSON可进入 CAS、WebDAV mirror 和 WebSocket；`clientId/mode/timestamp` wire 未设短界。 | 进度身份、CAS、镜像、冲突和多客户端收敛已发布。 | **P2 queued**；副作用频繁但单请求无多源网络放大，因此排在 remote-work 后。 |
| 其它 batch/control JSON | books batch/export、local refresh、remote add/change-source、legacy content search，以及 replace-rule 的 first-document/overflow 映射仍可见。 | 多数动作的业务/数据合同和部分 cardinality 已发布；是否缺 actual-read、single-document 或字段 owner 需逐动作取证。 | **inventory-pending**；不得从本表直接推导实现。 |

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
fixture 后瞬时 404，同镜像 trace 重跑全链通过。下一轮仍须从 BookSource multipart、reading progress
和其它 batch/control JSON 重新取证，不能因本项关闭而合并签收。

## 20. BookSource 本地导入 multipart 与前端预读（2026-08-16 inventory）

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
本阶段只提交 inventory，不修改应用或测试；reading progress 与其它 batch/control JSON 继续排队。
