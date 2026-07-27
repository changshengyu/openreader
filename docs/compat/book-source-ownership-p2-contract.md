# P2 书源所有权、默认快照与用户管理联动合同

状态：2026-07-27 完成固定上游取证与当前实现审查；P2-S1 的关联表、namespace、
可重试迁移标记和旧卷事务迁移，P2-S2a 的 owner-scoped service，P2-S2b 的书源
管理/调试 REST，以及 P2-S2c 的搜索、探索、远程书、Reader、正文/缓存和 scheduler
运行时消费者已经测试先行实施；P2-S3 的固定上游管理员/default 状态机已完成取证，
**管理员消费者、备份/WebDAV 恢复和浏览器发布门仍未实施完成**。本合同不把仍存的
全局查询或旧测试视为正确性依据。

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
