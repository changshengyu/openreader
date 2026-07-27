# P2 书源所有权、默认快照与用户管理联动合同

状态：2026-07-27 完成固定上游取证与当前实现审查；**尚未修改业务代码**。本合同是
后续测试和实现的前置闸门，不把当前全局 `book_sources` 表、现有 REST 路径或旧测试
视为正确性依据。

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
| `POST /api/sources/default/save` | 兼容路径；必须额外要求管理员，把当前管理员自己的活动书源设为默认。前端不再把它作为普通书源管理按钮。 |
| `POST /api/sources/default/restore` | 当前用户删除私有快照并按当前默认 reconcile；显式空列表与“恢复默认”保持不同状态。 |
| `POST /api/admin/users/:id/sources/default` | 管理员把目标用户现有活动书源复制为默认；目标未初始化/不存在时返回明确 `404/409`，不回退到调用者书源。 |
| `POST /api/admin/users/sources/reset` | body 为去重后的用户 ID 数组；管理员将每个可选目标恢复为当前默认，单个目标失败时整批事务回滚。 |

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
