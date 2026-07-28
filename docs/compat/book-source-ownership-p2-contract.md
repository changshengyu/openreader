# P2 书源所有权、默认快照与用户管理联动合同

状态：**P2-S1…P2-S4 implemented / full-regression-passed / Docker-published**。关联表、
namespace、可重试旧卷迁移、owner-scoped service、管理/调试/搜索/探索/Reader/scheduler
消费者、管理员默认动作、备份/WebDAV/缓存隔离均已测试先行实施；双账号三视口和专门的
旧全局源 Docker 迁移/COW/备份恢复门已完成。实现与测试提交 `0db752e` 已从本机发布，
`0db752e`/`latest` 共同指向
`sha256:83f53fe3aa523fc1196454d4c5f1d413648eb72ad1e87c83c838e7200859207e`。
本合同不把旧的全局查询或测试视为正确性依据。

固定上游：

- `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
- `web/src/components/UserManage.vue`
- `web/src/views/Index.vue`
- `src/main/java/com/htmake/reader/api/controller/BookSourceController.kt`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`

当前映射：

- `backend/models/models.go`
- `backend/db/db.go`
- `backend/api/sources.go`
- `backend/api/source_debug.go`
- `backend/api/search.go`
- `backend/api/explore.go`
- `backend/api/books.go`
- `backend/api/admin.go`
- `backend/api/backup_restore_plan.go`
- `backend/api/webdav.go`
- `backend/services/backup/backup.go`
- `backend/services/scheduler/scheduler.go`
- `frontend/src/components/workspace/SourceManager.vue`
- `frontend/src/components/overlays/OverlayUserManagement.vue`
- `frontend/src/composables/useOverlayUserManagement.js`
- `frontend/src/api/sources.js`
- `frontend/src/api/admin.js`

## 1. 上游权威状态机

### 1.1 用户书源不是全局共享配置

上游的权威持久化键是：

```text
storage/data/<user namespace>/bookSource.json
```

`BookSourceController.getUserBookSourceJson(userNameSpace)` 的状态转换是：

1. 用户文件存在时，读取该用户文件，包括显式保存的空数组。
2. 用户文件不存在且用户不是 `default` 时，读取
   `storage/data/default/bookSource.json`。
3. 默认文件存在时，把当时的默认列表复制成用户自己的文件，再返回该副本。
4. 默认文件不存在时，返回空列表；不存在的默认文件不是一个永久共享视图。

因此默认书源是**首次初始化快照**。修改默认书源不会改写已经有私有文件的用户；
删除某个用户的书源文件后，该用户下一次读取才会复制当时的新默认值。

### 1.2 普通书源动作只处理当前用户

`saveBookSource`、`saveBookSources`、`getBookSource(s)`、
`deleteBookSource(s)` 和 `deleteAllBookSources` 都通过当前认证 namespace 读写。上游
以 `bookSourceUrl` 作为单个书源的稳定身份；新增或导入同 URL 的书源会替换该用户
已有配置，不会触碰其他用户。

`BookController.loadBookSourceStringList(userNameSpace)` 以及搜索、探索、换源、目录、
正文和定时更新分支都从目标书籍所属用户的 namespace 查找书源。不存在“用户 A
编辑一行、用户 B 的搜索和阅读同时改变”的状态。

### 1.3 管理员与默认书源动作

`UserManage.vue` 提供两个与私有书源直接相关的动作：

- `setAsDefaultBookSources(user)`：确认
  `确认要将用户<username>的书源设为默认书源（新用户有效）吗?`，服务端要求管理
  权限，把目标用户已有的私有书源复制到 `default`。目标用户文件不存在时返回
  `用户书源不存在`。
- `deleteUserBookSource()`：对已选择、非 `default` 的用户确认
  `确认要删除所选择的用户书源吗?`，删除这些用户的私有书源文件。下一次读取时按
  当前默认快照重新初始化。

当前用户还可以调用 `deleteBookSourcesFile` 删除自己的私有文件并恢复默认。它不同于
`deleteAllBookSources`：后者写入一个显式空列表，后续读取仍为空，不会再次复制默认。

显式空数组仍是“用户书源存在”或“默认书源存在”。因此管理员可以把目标用户的空私有
列表设为默认；用户恢复一个已配置但为空的默认列表时应得到空活动列表，而不是
“默认书源为空”的失败。只有 namespace/file 从未存在才是“未配置”。

## 2. 当前实现审查矩阵

| 合同层 | 当前 OpenReader | 与上游的差异 | 判定 |
|---|---|---|---|
| 数据所有权 | `BookSource` 没有 `UserID`；所有账号共用一张活动表。 | 任一有编辑权限的普通用户都能修改所有人的搜索、探索、换源和阅读配置。 | **错误重构 / P0 数据隔离问题** |
| 列表与计数 | `/api/sources` 返回全表；管理员列表给每个用户重复同一个全局 `sourceCount`。 | 用户看不到自己的独立列表，管理员计数不表示目标用户状态。 | **must-fix** |
| CRUD/导入/调试 | ID、批量 ID、导入匹配和导出都没有用户条件；导入按名称而不是上游 URL 身份更新。 | 可读取、修改、删除、导出或调试另一个账号的书源；同 URL 改名可能产生重复项。 | **must-fix** |
| 搜索与探索 | `search.go`、`explore.go` 从全表加载启用书源。 | 用户 A 的启停、分组和规则立即改变用户 B 的搜索/探索。 | **must-fix** |
| 加书与换源 | `createRemoteBook`、候选列表、`changeBookSource` 只按全局 source ID 查找。 | 请求可引用不属于当前用户的 ID；恢复私有所有权后这些查询会成为越权入口。 | **must-fix** |
| 阅读与定时更新 | 书籍按全局 `SourceID` 读取；scheduler 也只按该 ID 查表。 | 当前靠全局表偶然可用，不能证明书籍与书源属于同一用户。 | **must-fix** |
| 失败缓存 | `SourceFailure` 已有 `UserID + SourceID`。 | 失败状态是私有的，但它指向的配置仍是全局共享，所有权只完成了一半。 | **保留并随迁移重映射** |
| 默认快照 | `defaultBookSources.json` 存在，但任一可编辑用户都能“设为默认”；恢复会删除全表再导入。 | 设默认缺少管理员边界；恢复影响所有账号，还会让现有书籍引用已删除的 source ID。 | **高危 must-fix** |
| 用户管理 | 没有“设为默认书源”和“删除用户书源”；UI 明示“全局书源”。 | 上游两个真实动作无法表达。 | **数据合同完成后重建** |
| 备份 | 用户级备份的 `bookSource.json` 仍导出全表；恢复也导入全表。 | 用户 A 的备份包含并可覆盖用户 B 的书源。 | **高危 must-fix** |
| WebSocket | `sources_update` 使用 `BroadcastAll`。 | 一个用户的私有变更会让所有账号清缓存和重载。 | **must-fix** |
| 浏览器缓存 | key 已带账号 scope，但值来自全局 API；source ID 没有 schema 版本。 | 缓存看似隔离，内容实际相同；迁移后旧 ID 可能短暂回显。 | **must-fix** |
| 删除用户 | 完整删除计划没有 `BookSource` namespace/state。 | 恢复私有书源后会遗留目标用户配置。 | **must-fix** |

结论：全局书源不能继续登记为“有意的数据模型重设计”。它既不满足固定上游行为，
也不满足 OpenReader 已宣称的 JWT 多用户隔离；当前 `CanEditSources` 只限制能否编辑，
不能把跨账号共享变成安全的所有权模型。

## 3. 目标数据合同

### 3.1 活动书源与初始化标记

采用加法、写时复制的关系模型，避免升级时复制全部配置并重写所有既有 source ID：

- `UserBookSource`（实现名可按现有命名规范调整）：`UserID + SourceID` 唯一关联，
  `UserID = 0` 保留给默认模板；`Detached` 表示该配置只为本用户既有书籍保留，不参加
  活动列表。
- `BookSourceNamespace`：每个用户一行初始化标记。它必须区分“从未初始化”和“用户
  显式清空后仍为空”；`UserID = 0` 的标记区分“未配置默认”和“已配置为空”。
- `BookSource` 继续保存不带账号凭证的规则快照。多个刚初始化的用户可以暂时引用同一
  不可变快照；任何编辑、启停、分组或导入更新都必须通过 service 做**写时复制**，把
  目标用户的关联、该用户书籍和该用户失败缓存切到新行，绝不能原地修改其他用户或
  默认模板仍在引用的行。

现有 `data/defaultBookSources.json` 必须导入为 `UserID = 0` 的关联并继续作为兼容镜像，
不能在升级时丢弃。

活动列表、计数、导出、搜索、探索和换源候选只读取当前用户 `Detached = false` 的
关联。通过当前用户书籍读取 source 时，必须证明该用户仍有活动或 detached 关联；
detached 配置可继续服务既有书籍，以免默认重置或备份恢复产生悬空引用。

### 3.2 身份与唯一性

- 上游兼容身份优先使用规范化后的 `BaseURL/bookSourceUrl`，限定在同一用户的关联集。
- 为兼容历史上允许空 URL 的行，空 URL 只能在同一用户关联内以稳定 ID/规范化名称兜底；
  不能跨用户或跨默认 namespace 匹配。
- 不把数据库数值 ID 写入上游 `bookSource.json` 作为可移植身份。
- 所有批量 ID、调试 ID、搜索 `sourceIds`、加书和换源请求必须先经过当前用户所有权
  查询；“ID 存在但属于他人”对外按不存在处理。

### 3.3 非破坏性旧卷迁移

迁移必须在一个 SQLite 事务中完成，并有旧卷 fixture：

1. 在添加所有权前识别旧的全局表，快照所有源行、用户、远程书籍和
   `SourceFailure` 引用。
2. 给每个现有用户建立指向全部旧活动行的关联并创建 initialized marker；不复制
   `BookSource`，也不改写 `books.source_id` 或 `source_failures.source_id`。
3. 当前 `defaultBookSources.json` 存在时，按 URL identity 导入/匹配后建立
   `UserID = 0` 的默认关联；不存在时，以升级前全部活动行作为默认关联，保持旧系统中
   新账号看到相同书源的行为。
4. 提交前验证每本远程书和失败缓存都能通过其用户关联解析同一个 source，所有用户的
   活动源数量与升级前全局数量一致；没有关联的用户不能直接使用该 ID。
5. 迁移可重复启动且不重复关联；任何一步失败时，旧表、书籍和失败缓存保持原样。

迁移不得重写书籍内容、章节、进度、书签、source ID、缓存路径或用户凭证。旧浏览器
`bookSourceList@<scope>` 缓存在首次所有权上线时内容仍等价，但后续写时复制会返回新 ID；
每次复制提交后必须只向目标用户广播并失效其 scoped cache。

### 3.4 默认恢复与已使用书源

默认恢复、管理员删除用户书源和备份恢复都必须在目标用户事务内做 reconcile：

- 默认中同 URL 的书源把目标用户关联切到对应快照；若目标用户需要修改该快照，先写时
  复制，不修改默认或其他用户。
- 新默认源为目标用户建立活动关联。
- 不在新列表且未被目标用户书籍使用的旧关联可以删除；底层快照只有在没有任何关联、
  书籍或失败缓存引用时才可回收。
- 不在新列表但仍被目标用户书籍使用的旧关联改为 detached；它不再参加搜索、探索、
  候选、管理列表和计数。
- 后续重新导入同 URL 时复用/复制快照并重新激活该用户的 detached 关联。

这是 OpenReader 为关系数据库和用户数据安全保留的技术适配：可见活动书源列表与
上游一致，但不会因为替换一个配置文件而破坏已有书籍引用。

## 4. REST/API 目标合同

保留当前 `/api` REST 风格和旧路径兼容，不倒退到 `/reader3/*`：

| 路径 | 目标语义 |
|---|---|
| 现有 `/api/sources*` | 所有列表、单项、批量、导入/导出、调试、默认状态与恢复均限定当前用户；写操作继续检查 `CanEditSources`。 |
| `GET /api/admin/users` | 保持现有用户摘要；`sourceCount` 改为目标用户活动关联数，不含 detached。未初始化用户只投影当前默认数量，不能因管理员打开列表而创建 namespace、冻结默认快照。 |
| `POST /api/sources/default/save` | 兼容路径；必须额外要求管理员，把当前管理员自己的活动书源设为默认。前端不再向普通用户显示该入口；显式空活动列表可保存为空默认。 |
| `POST /api/sources/default/restore` | 当前用户按当前默认 reconcile；显式空列表与“恢复默认”保持不同状态。已配置的空默认恢复成功并得到空活动列表，只有未配置默认才返回 `404`。 |
| `POST /api/admin/users/:id/sources/default` | 无 body；管理员把目标用户**已经初始化**的活动书源复制为默认。目标用户不存在返回 `404 {"error":"user not found"}`；用户 namespace 尚未初始化返回 `409 {"error":"user sources are not initialized"}`；成功返回 `200 {"count":N}`，其中 `N` 可为 `0`。不得先懒初始化目标用户或回退到调用者书源。 |
| `POST /api/admin/users/sources/reset` | body 为 `{ids:[...]}`。去重后为空返回 `400`；任一目标不存在返回 `404` 且全批不变；默认未配置返回 `404`。管理员、当前账号和普通账号都可作为书源重置目标，但账号删除仍保留现有保护。所有目标在一个事务中按当前默认 reconcile，成功返回 `200 {reset,imported,updated,skipped}`。 |

源编辑权限不是管理员权限。普通用户可在授权时编辑**自己的**书源，但不能设置全局
默认、读取目标用户源、恢复其他用户或借备份覆盖其他用户。

远程预览虽不落库，仍属于书源管理和外部抓取动作；它必须同时遵守编辑权限以及已有
SSRF、超时、大小和重定向限制。

## 5. 数据流影响清单

实施不能只改 `sources.go`。以下消费者必须使用同一个 owner-scoped repository/service：

- source CRUD、导入、导出、批量、默认快照、调试和失效检测；
- Index 搜索、探索和 source ID 排序；
- 新增远程书、换源候选、切换书源、书籍刷新、章节/正文、缓存、封面和章节图片；
- scheduler 按书籍用户读取书源；
- `SourceFailure` 记录、读取、清理，以及写时复制时只重映射目标用户；
- 用户级逻辑备份导出、备份/WebDAV 恢复、书架恢复时按用户 URL 解析 source ID；
- 管理员用户计数、删除用户、设默认和重置用户书源；
- `sources_update` 只广播目标用户；默认模板变化不强制改写已初始化用户；
- 浏览器 source cache key 版本和所有打开中的 SourceManager/BookInfo/Reader 刷新。

## 6. 测试先行闸门

实现前先新增失败测试，至少覆盖：

1. **迁移**：两用户共享旧 source ID 的旧卷升级后各有独立活动关联，原 source、书籍
   和失败缓存 ID 不变；默认文件优先、无默认文件回退旧列表；二次启动不重复关联；
   注入失败全事务回滚。
2. **CRUD 越权**：A 的 list/search/explore/export/debug/batch/get/update/delete 只看到
   A；伪造 B 的 ID 返回 `404` 或空结果，B 的行和缓存不变。
3. **书籍链路**：创建远程书、候选、换源、刷新、章节正文、缓存和 scheduler 不接受
   他人 source；迁移后的现有书仍可读。
4. **初始化状态**：新用户第一次读取复制默认；默认后续变化不影响已初始化用户；
   显式清空后保持空；恢复默认才重新 reconcile。
5. **默认管理**：只有管理员可把指定用户设为默认；批量恢复只处理目标用户；确认取消、
   事务失败和账号切换不 dispatch/不提交。
6. **引用安全**：默认恢复不会产生悬空 `source_id`；匹配 URL 保留/重映射，未匹配但
   已使用的源 detached 且不出现在活动列表。
7. **备份恢复**：A 的 `bookSource.json` 不含 B；A 恢复不改 B；书架 source 解析限定 A；
   无编辑权限时只跳过 A 的源部分，不影响其他个人数据。
8. **同步和缓存**：A 的 source 更新只通知 A 的客户端；管理员重置 B 只通知 B；旧缓存
   schema 不回显迁移前 ID。
9. **前端**：UserManage 恢复“设为默认书源”和“删除用户书源”，每用户显示真实计数；
   SourceManager 移除普通用户“设为默认”，保留当前用户“恢复默认”与显式清空差异。
10. **真实浏览器与 Docker 旧卷**：1440×900、390×844、360×800 双账号并行验证；
    本地构建镜像挂载旧卷升级，重启后源/书籍/进度不丢且互不串号。

旧测试中 `TestAdminUsersIncludesGlobalSourceCount` 以及所有不带 owner 创建/断言全表的
source 用例不能继续证明正确；应改为显式用户 fixture。测试通过后仍须完成双账号真实
浏览器和旧卷 Docker 闸门，才能把本模块标记为对齐。

## 7. 建议实施切片

1. **P2-S1 数据层与迁移**：模型、namespace、owner repository、旧卷迁移和引用完整性。
2. **P2-S2 API/运行时隔离**：写时复制 CRUD、搜索/探索、书籍/reader/scheduler、
   用户级广播。
3. **P2-S3 默认与管理员联动**：默认 reconcile、UserManage 两个动作、SourceManager
   入口调整。
4. **P2-S4 备份/恢复与发布门**：用户级 source artifacts、旧卷恢复、缓存版本、双账号
   浏览器和 Docker volume 验证。

每个切片可独立提交并推送 GitHub；只有达到可供用户验证的连贯状态且通过对应回归后，
才允许按本地构建流程发布 Docker。

### P2-S1 实施记录（2026-07-27）

- 新增 `UserBookSource`、`BookSourceNamespace` 和通用 `SchemaMigration`；没有给旧
  `BookSource` 强填 owner，也没有复制规则行。
- `book-source-ownership-v1` 在一个 SQLite 事务内为所有现有用户建立旧活动源关联；
  有旧源时同时建立默认关联，无旧源时仍为现有用户保存显式空 namespace。
- `books.source_id`、`source_failures.source_id`、章节、进度、书签和文件路径均不重写。
- marker 与关联同事务提交；注入关联写入失败时 namespace/marker 全部回滚，第二次启动
  会重新执行而不是因新表已存在而误判完成。
- 当前 source handlers 仍读取全表，所以本切片只可作为后续 owner-scoped service 的
  数据地基，不可单独发布 Docker 或宣称多用户隔离已修复。

### P2-S2a 实施记录（2026-07-27）

- 新增 owner-scoped 书源 service；活动列表、单项读取、创建、更新和删除都以
  `user_id + source_id` 关联为权限边界，不再把知道全局 source ID 视为可访问。
- 用户 namespace 第一次读取时只复制当时的默认活动关联；之后默认变化不覆盖该用户，
  已初始化的显式空集合也不会被重新填充。
- 多用户共享旧快照时，编辑先执行写时复制；只重映射目标用户的关联、远程书籍和
  `SourceFailure`，规则语义变化时也只清目标用户书籍/章节变量。
- 删除先检查目标用户自己的书籍引用，只移除目标用户关联和失败缓存；底层快照仅在没有
  任何用户关联、书籍或失败缓存引用时回收。
- 当前 REST handlers、搜索/探索、Reader、scheduler、备份和广播尚未接入该 service；
  本切片仍不能发布 Docker 或宣称运行时隔离完成。

### P2-S2b 实施记录（2026-07-27）

- `/api/sources*` 的列表、单项、CRUD、清空、批量、导入/远程导入、导出、默认状态/保存/
  恢复、单项/批量调试和失效源列表已切到 authenticated-user association。
- 伪造其他用户 source ID 时，单项读取/修改/删除/调试统一返回原有 `404`，导出和批量
  操作忽略外部 ID；`usedBookCount` 只统计当前用户书架。
- 批量写入在一个 SQLite 事务中完成；任一实际写入失败会回滚此前条目。清空会把仍被当前
  用户书籍使用的源转为 detached，避免破坏已有书籍引用。
- 导入身份由同名纠正为当前用户 namespace 内的 `bookSourceUrl/baseUrl`；同名不同 URL
  可以并存，共享快照的同 URL 更新仍执行写时复制。
- “保存为默认”兼容路径现在额外要求管理员；默认文件使用临时文件原子替换，数据库保存
  失败时补偿恢复旧文件。恢复默认只 reconcile 当前用户，不重写已初始化的其他账号。
- `sources_update` 已从全账号广播改为只通知目标用户。测试环境对旧的全局 source fixture
  有显式 owner bridge，新双账号契约测试通过 service 创建，确保该桥不会掩盖越权回归。
- 本切片提交时，搜索、探索、远程书、换源、Reader、scheduler、备份恢复、管理员计数/
  重置仍是 P2-S2c/P2-S3/P2-S4 的剩余消费者；因此当时只提交 Git，未发布 Docker、
  未宣称模块完成。

### P2-S2c 实施记录（2026-07-27）

- 搜索和探索只解析当前用户活动且启用的书源；请求中伪造其他用户 source ID 时按未配置/
  不存在处理，不发起外部抓取。
- 新增远程书、临时 Reader session 和换源只接受当前用户活动且启用的书源；换源候选的
  分组、顺序、分页和计数也只来自该用户活动列表。
- 已有书籍的刷新、章节正文、正文搜索、缓存和 scheduler 通过
  `user_id + source_id` 关联读取；本用户 detached 快照仍可服务既有书籍，跨用户残留引用
  则失败，避免通过损坏或旧数据越权抓取。
- 正文搜索继续保持既有错误合同：缺失或跨用户书源在现代路径返回
  `400 {"error":"未配置书源"}`，legacy 路径返回 `200` 的失败 envelope，不升级成
  未区分的 `500`。
- 双账号 API 契约覆盖搜索、探索、候选、远程加书、临时 Reader、换源、刷新、正文搜索
  和“越权请求不得触发远程 HTTP”；scheduler 另有 namespace 隔离测试。
- P2-S3 的管理员计数/设默认/重置/删除用户联动，以及 P2-S4 的用户级备份、WebDAV
  恢复、浏览器缓存版本、双账号浏览器和 Docker 旧卷门禁仍未完成，因此本切片只提交
  Git，不发布 Docker、不宣称整个书源所有权模块完成。

### P2-S3 审查矩阵（2026-07-27，实施前）

| 动作 | 固定上游 | 当前 OpenReader | 判定与目标 |
|---|---|---|---|
| 管理员列表 | `UserManage.vue` 不显示全局书源计数；用户列表读取不会创建用户 `bookSource.json`。 | `listUsers` 对 `book_sources` 全表计数，并把同一个值写给每个账号；UI 标签为“全局书源”。 | **错误重构**：保留 OpenReader 的加法 `sourceCount`，但改为目标用户活动数；未初始化用户只投影默认数且无写副作用，标签改为“书源”。 |
| 目标用户设默认 | 行操作确认后提交 `{username}`；服务端要求管理权限，只读取目标用户已经存在的私有文件；不存在报“用户书源不存在”，空数组仍可保存。 | 只有 `/sources/default/save`，它固定读取调用管理员自己的源；UserManage 无入口，SourceManager 向普通用户显示一个最终会 `403` 的按钮。 | **must-fix**：新增稳定 ID 路径，目标不存在 `404`、未初始化 `409`；UserManage 恢复行操作；SourceManager 的兼容入口仅管理员可见。 |
| 删除/重置用户书源 | 表格选择除 `default` 外的用户，确认“确认要删除所选择的用户书源吗?”；删除私有文件，下一次读取复制当时默认。 | 无 API/UI；现有选择器只允许可删除账号，当前账号和管理员无法成为书源重置目标。 | **must-fix + 安全适配**：全部真实账号可选作书源重置目标；批量账号删除仍过滤受保护账号。一个事务 reconcile 当前默认，目标失败整批回滚。 |
| 空默认 | 存在的空 `bookSource.json` 是有效默认/用户状态。 | service 以 `ErrEmptyDefault` 拒绝恢复，保存兼容路径也拒绝空活动列表。 | **must-fix**：已配置空默认可保存、可恢复；恢复后既有书籍源 detached，活动列表为空。 |
| 删除账号 | 上游删除整个用户目录，因此私有书源同时消失。 | SQLite 删除计划未删除 `user_book_sources` 与 `book_source_namespaces`，会留下 owner 关联和不可回收快照。 | **must-fix**：在账号数据库事务中移除目标 namespace/关联；书籍和失败记录删除后仅回收真正无引用的快照。 |
| 同步事件 | 文件动作没有跨账号共享状态。 | 尚无管理员源动作事件。 | **技术栈等价**：默认模板变化不广播私有源；重置成功后只给每个目标用户发 `sources_update`，另发 `users_update` 让管理员刷新计数，所有事件必须在事务提交后。 |

P2-S3 测试顺序：

1. service 先覆盖无副作用计数投影、未初始化目标设默认失败、空默认、双用户批量原子回滚
   和删除 namespace 后的安全回收；
2. API 覆盖管理员/普通用户权限、目标 `404/409`、去重/空 body、真实 per-user count、
   目标用户事件隔离和账号删除无遗留；
3. frontend 先改控制器契约测试，再恢复 UserManage 行操作/批量动作，并锁定 SourceManager
   仅管理员显示兼容“设为默认”入口；
4. 全量回归后提交 Git；P2-S4 和双账号浏览器/Docker 旧卷门仍未完成时不发布镜像。

### P2-S3 实施记录（2026-07-27）

- 管理员用户列表的 `sourceCount` 已改为每个目标用户自己的活动书源数；未初始化用户仅
  投影当前默认数量，不创建 namespace，也不因此冻结默认快照。
- 新增管理员目标用户设默认和批量重置 API。设默认只读取已经初始化的目标 namespace，
  显式空列表可以成为有效空默认；批量重置先校验全部用户，再在一个事务中 reconcile，
  任一目标失败则整批不变。
- UserManage 恢复上游“设为默认书源”和“删除用户书源”动作。所有真实账号都可选作
  书源重置目标，而账号删除按钮仍只处理允许删除的账号；普通 SourceManager 不再显示
  管理员默认动作，管理员可以保存非空或显式空默认。
- 删除账号时同步移除该用户的书源关联和 namespace；只有不再被默认、其他用户或书籍
  引用的共享快照才会被回收。
- 默认模板变化不伪造跨账号私有更新；批量重置提交后只向各目标账号发送
  `sources_update`，并向管理员发送 `users_update`。
- service、API 和 frontend 契约测试覆盖投影无副作用、空默认、`404/409`、去重、批量
  原子回滚、事件隔离与账号删除清理。全量 Go 测试、前端 628/628 和生产构建通过。
- P2-S4 的用户级备份/WebDAV 恢复、浏览器缓存版本、双账号真实浏览器和 Docker 旧卷
  门禁仍未完成，因此本切片继续只提交 Git，不发布 Docker。

### P2-S4 审查矩阵（2026-07-27，实施前）

固定上游补充证据：

- `BookController.kt#saveToWebdav` 只在目标用户自己的
  `storage/data/<namespace>/bookSource.json` 已存在时把它写入 ZIP；不存在不会为了备份
  初始化用户文件，显式空文件则作为有效 `[]` 写入。
- `BookController.kt#syncFromWebdav` 只用 ZIP 中的 `bookSource.json` 替换目标用户文件；
  书架、分组、RSS、规则、书签和进度也全部限定相同 namespace。
- `WebdavController.kt#backupToWebdav/#restoreFromWebdav` 从认证用户 namespace 和
  WebDAV home 派生路径，不存在管理员读取所有用户配置的备份语义。
- 上游浏览器 key 含用户名但无关系模型版本；OpenReader 引入用户 ID scope 后仍需对本次
  source ID/COW 迁移增加逻辑版本，不能读取迁移前缓存。

| 动作 | 固定上游 | 当前 OpenReader | 判定与目标 |
|---|---|---|---|
| 用户逻辑备份书源 | 只复制目标用户已存在的 `bookSource.json`；未初始化时不创建也不写该成员，显式空时写 `[]`。 | `backup.Service.addSources` 忽略传入的 `userID`，查询整张 `book_sources` 并总是写 `bookSource.json`。 | **高危 must-fix**：只导出目标用户 active association，排除 detached；未初始化不产生成员且无写副作用，初始化空产生 `[]`。 |
| 管理员触发备份 | 仍是当前认证 namespace。 | `triggerBackup` 对管理员调用 `RunNow()`，把所有用户的书架、设置、RSS、规则及全局书源写入管理员旧 WebDAV 根。 | **高危 must-fix + 路径兼容**：管理员文件仍写旧根，但内容只过滤管理员 `userID`；普通用户私有根不变。生产认证路由不得再调用无 owner 的全量 helper。 |
| portable v1/v2 | 上游无此扩展。 | 两种 portable 都复用 `writeLogicalEntries(...,&userID)`，但书源步骤忽略该 ID。 | **允许扩展中的 must-fix**：本地书/资产合同不变，逻辑 `bookSource.json` 必须同样只含调用者 active sources。 |
| ZIP/WebDAV 恢复书源 | 替换目标用户文件；archive 不影响其他 namespace。 | `restoreSourcesFromDataStrict` 调用全局 `importBookSourcesStrictWithDB`，按全表名称修改或新增；未删除 archive 中缺失的当前活动源。 | **高危 must-fix**：在外层 restore 事务内按目标 `userID` 和 URL identity reconcile；缺失且在用的旧源 detached，未使用的解除关联；A 的恢复绝不改 B。 |
| 恢复后书架绑定 | 书架与书源来自同一用户目录，`origin/bookSourceUrl` 在该目录解析。 | `restoredBookSourceIDStrict` 按全表 `name/base_url` 查询，可能把 A 的书绑定到 B 的私有或 detached 快照。 | **高危 must-fix**：只在目标用户 active association 中先 URL、后名称解析；archive 数值 ID 继续不可信。 |
| 备份中的书架/章节变量 | 用户文件天然只引用自己的书源。 | `addBookshelf` 和 `addChapterVariables` 直接 join/读取 `book_sources`；损坏的跨用户 `source_id` 可把他人源元数据写进 A 的 ZIP。 | **must-fix / fail closed**：每条远程书和章节变量都必须证明 `user_id + source_id` active/detached 关联；无法证明时不泄露源规则，书架按无可移植源处理。 |
| 权限跳过 | 上游账号功能开关不授予跨 namespace 权力。 | `canEditSources=false` 已跳过书源 artifact 并恢复个人数据，但其余书架解析仍可能命中全局表。 | **部分完成**：保留 `sourcesSkipped:true`，同时让书架只解析调用者已有 active source；无匹配则 `sourceId=0`。 |
| 同步/回滚 | 恢复只改变目标文件。 | 逻辑 artifact 已共用 SQLite 外层事务并在提交后广播，但全局 source helper 绕过 owner contract。 | **技术地基可保留**：owner reconcile 必须复用同一事务；失败回滚 source association/COW/书架全部写入且零事件，成功只通知目标用户。 |
| 浏览器书源缓存 | key 仅承载上游 namespace。 | `bookSourceList@<user-scope>` 仍会命中迁移前保存的全局 source ID。 | **must-fix，无数据迁移**：新读取键固定为 `bookSourceList@source-owner-v1@<scope>`；旧键永不作为 source list 回退，但仍可被当前用户的缓存统计/清理识别。IndexedDB schema 版本不变。 |
| 发布门 | 无 OpenReader Docker。 | P2-S1…S3 尚未经过本模块双账号浏览器和升级卷验证。 | **必须完成**：双账号同浏览器验证备份/恢复/缓存；本地 Docker 对旧 SQLite、默认文件、管理员旧根、普通用户根、logical/portable 往返和重启做门禁。 |

P2-S4 API 与数据合同：

1. `POST /api/backup/trigger` 的请求/响应保持不变。管理员继续在
   `data/webdav/` 创建 `backup_*.zip`，普通用户继续在
   `data/webdav/users/<safe-username>/` 创建；两者内容都只属于认证 `userID`。
2. `POST /api/backup/restore-legado` 与 `/restore-webdav` 的路径、状态码、归档预检、
   `sourcesSkipped` 和其它 result count 保持不变。允许写源时，`bookSource.json` 是目标
   用户活动列表的替换/reconcile，不是全表 merge。
3. ZIP 无 `bookSource.json` 时不得初始化、清空或修改目标 namespace；ZIP 中显式 `[]`
   时则把目标 active 列表恢复为空，同时保留在用 detached 快照。
4. source restore 结果 `sources` 继续表示本次成功导入/更新/重新激活的条数；可增加
   `sourceDetached/sourceRemoved` 等加性诊断，但旧客户端不依赖它们。
5. 书架源解析只接受恢复事务中目标用户可见的 active association。无源编辑权限或成员
   缺失时，可恢复书架个人数据，但不能借全局名称/URL 绑定其他用户 source ID。
6. 不增加 SQLite 表/列，不移动 `data/`、`cache/`、`library/`，不改写已有 ZIP。缓存只
   版本化逻辑 key；旧缓存是可清理的派生数据，不迁移、不认领、不删除用户业务数据。

P2-S4 测试顺序：

1. service/backup 先建立双用户导出、detached 排除、未初始化省略、初始化空数组和管理员
   旧根内容隔离测试；旧的直接全表 source fixture 必须改成 owner association fixture。
2. restore 先建立 A replace/reconcile 不改 B、在用旧源 detached、书架 URL/名称仅在 A
   解析、权限 skip、archive 数值 ID 忽略和注入失败全事务回滚测试。
3. frontend 先把 source cache key 契约改为 `source-owner-v1`，证明旧键不回显但仍能被
   当前用户清理；`sources_update` 清理新旧两个当前账号键，不触碰其他账号。
4. 全量 Go/frontend/build 后，运行 1440×900、390×844、360×800 双账号浏览器合同：
   旧缓存不能回显，A 备份无 B 源，A 恢复后 A 更新/B 不变。
5. 最后才运行本地 Docker 新旧卷、logical/portable、管理员旧根/普通用户根、重启和
   amd64/arm64 发布门；全部通过前不发布镜像。

### P2-S4 实施记录（2026-07-27）

- 普通、scheduled 与 portable 逻辑备份只导出目标用户 active source association；
  detached 不进入 `bookSource.json`，未初始化 namespace 省略该成员且无写副作用，
  已初始化空 namespace 写入显式 `[]`。
- 管理员手动 trigger 继续写管理员旧 WebDAV 根，但所有逻辑 artifact 都传入管理员
  `userID` 过滤；认证路由不再调用 ownerless `RunNow()`。
- 书架和章节变量导出要求当前用户 active/detached association。损坏的跨用户
  `source_id` 被按无可移植源处理，既不输出他人源名称/规则/变量，也不输出可复用数据库 ID。
- `booksources.Service.ReplaceActive` 在恢复外层事务内以 URL、无 URL 时名称为 identity
  reconcile 当前用户；共享快照写时复制，归档缺失且仍被本用户书籍使用的源转 detached，
  未使用关联移除，其他用户 association、快照和书籍保持不变。
- 恢复书架只在目标用户 active association 中先 URL、后名称解析。显式空恢复通过加性
  `sourceDetached/sourceRemoved` 计数触发目标用户 `sources_update`；失败回滚 source、
  书架及后续 artifact 且不发事件。旧的全表名称导入和全用户变量清理 helper 已删除。
- 前端读取键已改为 `bookSourceList@source-owner-v1@<scope>`；旧键永不回显，但缓存统计、
  分组清理和 `sources_update` 仍可只针对当前账号清除新旧两键。
- 自动证据：沙箱外完整 `go test ./...` 通过；前端 `636/636` 和 Vite production build
  通过；fixed-baseline、portable v1/v2、双用户导出/恢复、COW、显式空、权限 skip、
  数据库回滚和缓存键契约均已覆盖。
- 真实证据：隔离 Go 实例且不拦截 `/api`，在 1440×900、390×844、360×800 验证 A/B
  SourceManager 只显示自己的源；A 的 versioned source cache 可见并可单独清到空，旧键不读取、
  其他账号新旧键保留由同一缓存合同锁定；三视口均无横向溢出，移动侧栏保持 260px 隐藏几何。
  同一实例的真实 logical backup 下载内容只含 A 源，A 改名后恢复为归档值，B 源保持不变。
- 补门同时捕获并修复了登录根渲染竞态：登录路由现在保持挂载直到成功回调完成路由与账号
  settle，B 注销后登录 A 会直接收敛到 `/`，不再卡在 `/login` 或“正在恢复当前账号…”。
  Docker 新旧卷、重启和多架构发布仍是最后门禁。

### P2-S4 Docker 发布门二次复审（2026-07-28）

本轮只复审发布证据，不修改应用代码。固定上游没有 OpenReader Docker，因此产品合同仍以
同一用户 namespace 的 `bookSource.json` 备份/恢复语义为准；Docker 门负责证明当前
SQLite/目录适配在真实升级卷中没有改变该语义。

| 证据层 | 当前实际覆盖 | 缺口与裁决 |
|---|---|---|
| 通用新卷 smoke | 第一位注册用户、portable v1/v2 外观资产、跨用户本地书、重启与备份恢复。 | 没有通过 `/api/sources` 创建任何书源；ZIP 中的 `bookSource.json` 为空或缺席都无法区分。**不能证明**管理员旧 WebDAV 根只导出自己的书源，也不能证明普通用户私有根。 |
| 通用历史卷 fixture | `create-old-volume-fixture` 先运行当前 `AutoMigrate`，此时 `book-source-ownership-v1` marker 已写入；之后才创建两个用户。fixture 只有本地书，`Book.SourceID=0`，没有 `BookSource`、`SourceFailure` 或旧全局书源引用。 | 这不是旧全局书源数据库。容器启动时不会执行所需的旧源 ownership 数据迁移。既有 `HISTORICAL_VOLUME=1` 只证明本地书归档、相对缓存和用户书架隔离，**不能关闭 P2-S1/P2-S4 升级门**。 |
| 通用历史卷备份 | 普通历史用户的 logical/portable 文件写入私有根，并可恢复到新账号；另一用户本地书不泄漏。 | 没有源行可泄漏，不能证明 active/detached 过滤、COW、源绑定或 `bookSource.json` owner scope。 |
| 已发布 `a90d10b` | 包含 P2-S1…S4 实现，且通用新卷/历史卷 smoke 已通过。 | 镜像可继续作为专项验证对象，但“书源 ownership Docker 门完成”仍是未经证明的声明。必须增加专项 fixture/assertion 后重建同源码候选；不能用已经发布本身倒推门禁通过。 |

专项测试合同：

1. 历史 fixture 必须在关闭数据库前放入至少两个旧全局 `BookSource`，让两个既有用户的
   远程书和失败记录保留旧 `source_id`；随后移除 ownership 关联表、namespace 表和迁移
   marker，使镜像首次启动真实执行 `book-source-ownership-v1`。本地书夹具与旧列删除继续
   保留，不能把旧卷缩窄成只服务本模块的假数据库。
2. 首次启动后，两个历史账号和默认 namespace 都应得到旧全局源的 active association，
   原书籍/失败记录 ID 不改。A 编辑共享源必须触发 COW：A 的列表、远程书和失败记录切到
   新快照，B 与默认模板仍保留原快照。
3. A、B 的 logical 与 portable ZIP 分别只包含各自 active sources；detached 不导出。
   A 恢复自己的 ZIP 后恢复 A 的源状态，B 的列表、书籍和 source ID 不变。
4. 新卷同时创建管理员和普通用户的不同书源。管理员备份继续写 `data/webdav/` 旧根，
   普通用户备份写 `data/webdav/users/<safe-name>/`；两类 logical/portable ZIP 都只能
   包含调用者书源，恢复后不得交叉修改。
5. 停止并重启同一容器卷后重复核对源列表、COW 结果、书籍绑定、迁移 marker 和备份根；
   再检查用户 0、A、B 的 association/namespace，不能仅靠 API 返回“没看到泄漏”。
6. 专项脚本必须可独立运行并清理临时容器/卷，失败时保留明确阶段错误。通过后再运行完整
   Go、frontend、build、通用新旧卷门；最后才把矩阵改为 Docker-published。

### P2-S4 Docker 发布结果（2026-07-28）

- `create-old-volume-fixture` 现在保留既有 TXT/EPUB/UMD/CBZ/相对缓存/双用户本地书，
  同时加入两个旧全局源、两个跨用户共享旧源 ID 的远程书和两条失败缓存；关闭前移除
  ownership 表、namespace 和 marker，确保镜像首次启动真实执行迁移。
- `scripts/docker-source-ownership-smoke.sh` 在新卷验证管理员旧根、普通用户私有根、
  logical/portable 精确 `bookSource.json`、恢复隔离与重启；在旧卷验证迁移前结构、迁移后
  user 0/A/B association、书籍/失败 ID 保留、首次共享源 COW、双用户 ZIP、恢复与最终
  SQLite owner 关系。
- 测试开发中明确修正两条错误假设：恢复可能为 A 创建同 URL/名称的私有快照，因此后续编辑
  不必重复换 ID；`SourceFailure` 是不进入备份的派生缓存，A 恢复可清 A，但不得清 B。最终
  断言通过 association 而不是全表同名查询解析身份。
- 精确本地候选 `0db752e` 通过 ownership 专项 smoke、通用新卷 smoke 和加入旧全局源后的
  通用历史卷 smoke；Go 全量、frontend `639/639` 和 production build 同时通过。
- 本机发布的 `ghcr.io/changshengyu/openreader:0db752e` 与 `:latest` 均包含
  `linux/amd64`、`linux/arm64`，远端 OCI index 为
  `sha256:83f53fe3aa523fc1196454d4c5f1d413648eb72ad1e87c83c838e7200859207e`。
  P2-S1…S4 至此关闭；后续书源修改必须保留本专项门，不再把物理 `book_sources.name`
  当作跨 namespace 唯一身份。
