# BookGroup P2 固定上游契约

状态：2026-07-19 已按合同完成应用实现、全量自动测试、真实 Go/SQLite 三视口与多客户端验证。

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
本合同覆盖书架分组标签、`BookGroup` 设置/管理两种模式、API、SQLite、同步和备份恢复。
它覆盖并纠正此前把“自定义分类 CRUD + Dialog 外壳”标成 BookGroup 已对齐的结论。

## 1. 权威来源

- `web/src/components/BookGroup.vue`
- `web/src/views/Index.vue#getShowShelfBooks`、`bookGroupSetList`、`bookGroupDisplayList`、
  `showBookGroup`
- `web/src/plugins/vuex.js#builtInBookGroup`、`setBookGroupList`、`builtInBookGroupMap`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#getBookGroups`、
  `saveBookGroup`、`saveBookGroupOrder`、`deleteBookGroup`、`saveBookGroupId`、
  `addBookGroupMulti`、`removeBookGroupMulti`、`saveToWebdav`、`syncFromWebdav`
- `src/main/java/io/legado/app/data/entities/BookGroup.kt`

当前映射：

- `frontend/src/views/Home.vue`
- `frontend/src/components/overlays/OverlayBookGroups.vue`
- `frontend/src/composables/useOverlayBookGroups.js`
- `frontend/src/stores/bookshelf.js`、`frontend/src/composables/useSync.js`
- `backend/api/categories.go`、`backend/api/books.go`、`backend/api/webdav.go`
- `backend/services/backup/backup.go`

## 2. 审查矩阵

| 契约层 | 固定上游行为 | 当前证据 | 判定与要求 |
|---|---|---|---|
| 默认数据 | 首次读取必须得到 `全部(-1,-10)`、`本地(-2,-9)`、`音频(-3,-8)`、`未分组(-4,-7)`，均默认显示。Vuex 还会向缺失旧数据补齐四项。 | `Home.vue` 临时拼出全部/本地/未分组，缺少音频；后端没有内置分组状态。 | `must-fix`：四项必须成为每用户可持久化的 BookGroup 数据，不能继续散落硬编码。 |
| 设置模式数据 | 只列 `groupId > 0` 的自定义分组；按书的 bit mask 预选；空选择报 `请选择书籍分组`。 | 多对多 `categoryIds`、预选和空选择保护已经等价。 | `acceptable-change/aligned`：保留多对多，不迁回 bit mask。 |
| 设置模式操作 | 设置模式仍显示“添加分组”，每个自定义分组仍可编辑；拖动句柄可见但 Sortable 被禁用。 | 当前只显示确认/取消，没有添加和编辑入口。 | `must-fix`：恢复添加、编辑；设置模式不得排序。 |
| 管理模式数据 | 四个内置分组与自定义分组在同一表格、同一顺序中管理。 | 当前表格只读取 `bookshelf.categories`。 | `must-fix`。 |
| 内置分组编辑 | 四项均可改名、显隐；显示名为当前名称加不可变语义后缀，如 `自定义名(全部)`。 | 名称、显隐均硬编码。 | `must-fix`：语义 key 不可改，显示名/显示状态可改。 |
| 删除 | UI 只给 `groupId > 0` 且没有书的自定义分组显示删除。 | 自定义非空删除有前后端双重保护。 | `aligned`；保留服务端强保护。内置分组永不提供删除。 |
| 统一排序 | 管理模式可拖动所有行；脏状态才显示保存；一次保存覆盖内置与自定义完整顺序。 | 只排序自定义分类。 | `must-fix`：建立稳定 token 的全量事务排序。 |
| 书架标签 | 只展示 `show=true` 且非空的分组，按统一 `order` 排序。 | 全部固定第一；本地/未分组按非空临时加入；自定义跟在后面；无音频。 | `must-fix`。 |
| 分组筛选 | 全部=所有书；本地=`origin === loc_book`；音频=`type === 1`；未分组=`group === 0`；自定义=bit mask 命中。 | 本地使用更稳健的 `isLocalBook`；未分组和多对多自定义正确；音频缺失。 | `must-fix` 音频；`isLocalBook` 是本地格式的技术等价增强。 |
| 当前标签 | 上游把 `showBookGroup` 存入 shelfConfig；没有可见非空标签时回退全部。 | `selectedGroup` 是 `Home.vue` 内存 ref，刷新后丢失；失效时固定回空字符串。 | `must-fix`：持久化稳定 token；回退到当前第一项可见非空分组。 |
| 新分组顺序 | 新自定义分组排在现有最大 order 之后。 | `createCategory` 只比较自定义分类最大值。 | `must-fix`：统一排序后仍必须追加到所有内置/自定义项末尾。 |
| 多用户 | 上游 namespace 隔离；OpenReader 必须用户隔离。 | Category/BookCategory 已按用户隔离。 | `must-fix`：新增内置状态、缓存、同步、备份同样按 user scope。 |
| 同步 | 上游重新加载用户存储；OpenReader 的多客户端适配必须收敛。 | 只有 `category_*` 事件，无法同步内置名称/显隐/统一顺序。 | `must-fix`：增加 `book_groups_update`；旧 category 事件继续兼容。 |
| 备份 | 上游写入并恢复完整 `bookGroup.json`；bookshelf 的 `group` bit mask 引用自定义 groupId。 | 只导出/恢复 `categories.json` 和书中扩展的 categoryNames，忽略 `bookGroup.json`。 | `must-fix`：OpenReader round-trip 与 reader-dev 导入都不能丢内置状态、自定义组或书籍关系。 |
| 既有数据 | 上游会补缺失内置项。 | 已部署数据库只有 Category。 | `must-fix`：仅做加法迁移/惰性补齐；不得破坏 Category、BookCategory 或旧备份。 |

## 3. 允许差异

以下差异保留并必须写进测试：

- Vue 3、Pinia、Element Plus 与 Go REST 是运行时适配。
- 自定义分组继续使用用户级 `Category + BookCategory` 多对多关系，不退回上游整数 bit mask。
- 批量移除保持幂等集合删除，不复制上游 `xor` 在重复调用时重新加回分组的缺陷。
- 本地书判定继续使用 `isLocalBook`，兼容 `sourceId=0`、`local://` 和本地文件字段。
- 后端对非空分组删除、跨用户 token、重复/缺失排序项做更强校验。

## 4. 数据与 API 合同

### 4.1 非破坏性数据结构

新增仅保存四个内置分组偏好的用户级表：

```text
book_group_preferences
  user_id       用户隔离键
  key           all | local | audio | ungrouped
  name          可编辑显示名
  show          是否显示
  sort_order    与 Category.sort_order 共用的排序空间
```

`(user_id,key)` 唯一。缺失项在读取/写入事务中按 `-10/-9/-8/-7` 惰性补齐；
Category 与 BookCategory 的表、ID 和现有关系不迁移。删除用户时必须在同一数据库事务删除偏好行。

### 4.2 稳定投影

`GET /api/book-groups` 返回按 `sortOrder` 排好的统一数组：

```json
{
  "key": "builtin:all",
  "kind": "builtin",
  "semantic": "all",
  "categoryId": null,
  "name": "全部",
  "defaultName": "全部",
  "show": true,
  "sortOrder": -10,
  "assignable": false,
  "deletable": false
}
```

自定义项的 key 为 `category:<id>`，`semantic` 为 `category`，`categoryId` 为真实 Category ID，
`defaultName` 为空，`assignable/deletable` 为 true。前端不得从名称反推语义。

### 4.3 写接口

| 方法与路径 | 请求 | 成功 | 错误/副作用 |
|---|---|---|---|
| `GET /api/book-groups` | 无 | `200` 完整统一投影；缺失四项先补齐 | 仅当前用户；读取失败 `500`。 |
| `PUT /api/book-groups/:key` | `key=all/local/audio/ungrouped`；`{name?,show?}` | `200` 更新后的内置项 | 空名/空请求/未知 key `400`；广播 `book_groups_update`。 |
| `PUT /api/book-groups/reorder` | `{keys:[完整稳定 token...]}` | `200` 完整统一投影 | 重复、缺失、额外、跨用户 category token 均 `400`；一个事务同时更新内置与 Category；广播 `book_groups_update` 和兼容 `categories_update`。 |

自定义分组继续使用 `/api/categories`。创建时 sortOrder 必须大于当前统一最大值；自定义改名、显隐、
删除和旧 `/categories/reorder` 除原事件外还要触发 BookGroup 投影刷新。旧接口保持兼容，
但新的管理 UI 只使用统一 reorder。

## 5. 前端状态与交互合同

- `bookshelf.categories` 继续只包含可分配的自定义分类，避免导入/设置书籍时选到内置项。
- 新增 `bookshelf.bookGroups`、独立 scoped cache/request/revision；切换用户必须同步清空。
- Home、管理弹层只从 `bookGroups` 读取名称、显隐和顺序，不再自己拼内置行。
- 设置模式从 `categories` 读取；管理模式从 `bookGroups` 读取。
- 设置模式底部顺序为“添加分组 / 确认 / 取消”，行操作允许编辑自定义分组。
- 管理模式允许拖动/编辑/显隐所有行；删除只显示于空的自定义行。
- 内置行可见名称为 `name(defaultName)`；书架 tab 只显示 `name`。
- shelf preference 增加 `groupKey`，保留现有 view；旧数据没有该字段时默认 `builtin:all`。
- 当前 token 不再可见或变空时选择第一项可见非空投影；没有任何可见非空项时正文为空且状态回退
  `builtin:all`，后续有书时可重新出现。
- `book_groups_update` 使用完整 payload 时原子替换；无 payload 时强制网络刷新。任何 category 事件都要
  同时更新或失效统一投影，不能出现一个客户端分类已改而标签仍旧的窗口。

## 6. 备份与恢复合同

- 新备份继续保留 `categories.json`、categoryNames 和全部旧逻辑文件；额外写上游命名的
  `bookGroup.json`。
- `bookGroup.json` 的四个内置项使用固定 `groupId -1..-4`；自定义项按统一顺序分配可移植的正
  power-of-two `groupId`，同时带 `categoryId`/稳定 key 扩展，不能把数据库自增 ID 误当 bit mask。
- `bookshelf.json` 增加与该映射一致的 `group`；OpenReader 的 `categoryNames` 仍是超过上游 mask
  容量时的无损 round-trip 权威字段。
- 恢复顺序固定为：用户设置/Category → `bookGroup.json`（补/映射自定义组并恢复内置状态/统一顺序）
  → bookshelf（优先 categoryNames，否则使用 `group` mask 映射）→ 其余数据。
- reader-dev 仅含 `bookGroup.json + bookshelf.json` 的备份必须能创建自定义 Category 并恢复关系。
- 没有 `bookGroup.json` 的既有 OpenReader 备份继续按 categories/categoryNames 恢复，四个内置项使用默认值。
- portable backup allowlist 必须接纳大小写规范化后的 `bookGroup.json`；所有解析仍受既有 ZIP 项数、
  单项和总大小限制。
- 恢复完成同时广播 categories、bookGroups、bookshelf；不得把另一个用户的组或关系带入当前用户。

## 7. 测试先行门禁

### 后端

1. 首次/缺项读取补齐四内置项，投影合并自定义组且用户隔离。
2. 内置改名/隐藏、空名和未知 key；自定义 API 仍保持原合同。
3. 全量混合排序事务；重复、缺失、外用户 token 必须回滚。
4. 统一排序后新增分类始终追加末尾；删除用户无残留偏好。
5. 备份含 `bookGroup.json` 和匹配的 bookshelf mask；OpenReader round-trip 与 reader-dev fixture 均恢复
   名称、显隐、顺序和多分组关系；旧备份仍通过。
6. sync payload 与旧 category 事件兼容。

### 前端

1. store 的统一投影、缓存 scope、迟到响应和 WebSocket 替换/强刷。
2. 设置模式只列自定义、保留预选/空保护，并可添加/编辑。
3. 管理模式有四内置行，内置不可删但可编辑/显隐，混合拖拽一次保存完整 token。
4. Home 验证全部/本地/音频/未分组/多对多自定义筛选、非空/显隐/统一顺序和持久 token 回退。

### 真实浏览器

在 1440×900、390×844、360×800 使用真实 Go/SQLite：

- 初始四内置行、音频标签与筛选；
- 内置改名/隐藏、混合拖拽、刷新及第二客户端同步；
- 设置模式添加/编辑/预选/空选择不写入；
- 书架 tab 的非空、排序和当前组持久化；
- Dialog compact fullscreen、无横向溢出、关闭后仍停留 Index 工作台。

只有上述证据同时通过，BookGroup 才能重新标记为对齐。

## 8. 2026-07-19 实现记录

- 后端新增每用户 `book_group_preferences` 加法表，以及统一的
  `GET /api/book-groups`、内置项更新、完整混合排序接口。四项缺失数据惰性补齐，Category 与
  BookCategory 的既有 ID/关系不迁移。
- 新分类追加到统一最大顺序之后；分类增删改、旧分类排序和恢复操作都会广播完整
  `book_groups_update`。前端投影缓存和迟到响应均按认证用户 scope 隔离。
- Home 只展示可见且非空的统一投影，支持全部、本地、音频、未分组和多对多自定义筛选；稳定 token
  写入 shelf preference，并按首个可见非空项回退。
- 设置模式继续只列可分配的自定义分组，并恢复添加/编辑；管理模式统一显示四内置项与自定义项，支持
  内置改名/显隐、空自定义删除和全量混合拖拽排序。
- 备份新增 `bookGroup.json` 和一致的 bookshelf `group` 位掩码；保留 `categories.json` 与
  `categoryNames`。OpenReader 往返和仅含 reader-dev `bookGroup.json + bookshelf.json` 的恢复 fixture
  均通过。
- 自动门禁：前端 `502/502`、生产构建、后端 `go test ./...` 全部通过。
- 真实浏览器：`scripts/smoke/book-group-real-api-contract.mjs` 使用真实 Go/SQLite 在 1440×900、
  390×844、360×800 通过；覆盖四内置标签、音频筛选数据、内置改名、显隐、多客户端同步、混合拖拽
  与刷新后的持久化。
- 允许差异保持不变：自定义分组继续使用 Category/BookCategory 多对多；本地书继续使用增强的
  `isLocalBook` 判断；空组删除、跨用户 token 与不完整排序受到更强服务端校验。
