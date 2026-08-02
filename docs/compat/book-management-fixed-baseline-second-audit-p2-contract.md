# BookManage 第二轮固定上游复审合同（P2）

状态：**已按第二轮固定上游合同重建并通过完整候选验证；Docker 待发布**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮权威文件：

- `web/src/components/BookManage.vue`
- `web/src/plugins/vuex.js` 的 `isNormalPage`、`dialogWidth`、`dialogTop`、
  `dialogContentHeight`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt` 的
  `getShelfBookWithCacheInfo`、`deleteBooks`、`addBookGroupMulti`、
  `removeBookGroupMulti`、`cacheBookSSE`、`deleteBookCache`、`exportBook`

当前 OpenReader 对应面：

- `frontend/src/components/overlays/OverlayBookManagement.vue`
- `frontend/src/components/overlays/BookManagement*.vue`
- `frontend/src/composables/useOverlayBookManagement.js`
- `frontend/src/composables/useOverlayBookBatchActions.js`
- `frontend/src/composables/useOverlayBookItemActions.js`
- `frontend/src/stores/bookshelf.js`
- `backend/api/books.go`、`backend/api/cache_stream.go`

## 1. 所有权、生命周期与弹窗几何

| 合同 | 固定上游行为 | 当前行为 | 裁决 |
|---|---|---|---|
| 所有权 | 根 `App` 只挂载一个 `BookManage`；Index 的“书籍管理”只切换该对话框，不产生业务路由。 | `GlobalOverlayHost` 只挂载一个 `OverlayBookManagement`，导航动作只调用 `overlay.openBookManage()`。 | `aligned / Vue 3 equivalent`，必须保留唯一所有权。 |
| 页面门禁 | 根节点 `v-if="$store.getters.isNormalPage"`；`isNormalPage` 只在 `config.pageType === "正常"` 时成立。 | 当前没有正常模式门禁，Kindle/简洁模式仍可渲染管理弹窗。 | `must-fix`：使用规范化后的 `reader.pageType === "normal"`；切出正常模式时关闭该弹窗。 |
| 桌面宽度 | `min(max(windowWidth × 0.7, 750), 1000)px`。 | 固定 `min(1180px, 100vw - 48px)`。 | `must-fix`。 |
| 内容高度 | 桌面 `min(0.7 × windowHeight - 184, 400)`；mini 为 `windowHeight - 184`。表格再减 `42px`。 | 表格固定 `calc(100vh - 188px)`，未使用共享内容高度。 | `must-fix`；使用动态视口单位的技术适配可以保留。 |
| 顶部位置 | `(windowHeight - dialogContentHeight - 184) / 2 px`。 | 未设置 `top`，使用 Element Plus 默认值。 | `must-fix`。 |
| mini 形态 | 同一 `el-dialog` 在 mini interface 下 `fullscreen`；仍是同一张表、同一列定义和同一控制器。 | fullscreen 正确，但另渲染移动卡片列表并隐藏桌面表。 | `must-fix`：删除移动卡片产品结构；手机仍使用同一张可横向滚动的上游表。 |
| 关闭状态 | 关闭只清空 `manageBookSelection`；`searchQuery` 保留，缓存任务继续存在。 | 关闭同时清空搜索词和选择；缓存任务在模块级继续存在。 | `must-fix`：保留搜索词、清空选择；缓存任务跨关闭/重开继续存在。 |
| 打开加载 | 每次打开调用 `getShelfBookWithCacheInfo`，成功后计算浏览器缓存数。 | `ensureBooksLoaded({all:true})` 受内存去重影响，可能不发网络请求；随后计算浏览器缓存。 | `must-fix`：每次打开强制读取完整书架；类别读取可以并行且失败不阻断书架管理。 |

## 2. 可见结构与搜索合同

| 区域 | 固定上游 | 当前差异 | 裁决 |
|---|---|---|---|
| 标题 | `书架管理`；右侧 `❗️只能缓存文本内容`。 | 文案存在。 | 保留；桌面保持同一行。 |
| 搜索框 | small、clearable、搜索前缀图标；placeholder 精确为 `搜索书名或作者`；只匹配 trim/lowercase 后的书名和作者。 | placeholder 为 `搜索书名、作者或文件名`；还匹配文件名、分组和阅读进度；移动端增加“全选/清空”。 | `must-fix`：只搜标题/作者，删除移动附加工具。 |
| 列 1 | selection，宽 `25`；mini 时固定。 | 宽 `42`，未按 mini 固定。 | `must-fix`。 |
| 列 2 | property `name`，可见标签精确为上游的 `书名名`，min-width `100`；mini 时固定；标题按钮打开共享 BookInfo。 | 标签 `书名`、min-width `180`；移动卡片另有一套入口。 | `must-fix`：恢复固定上游可见值与单入口。 |
| 作者 | min-width `100`。 | `120`。 | `must-fix`。 |
| 分组 | min-width `120`；正组名称以空格连接。 | `120`，多对多名称以 `、` 连接。 | 列宽恢复；名称分隔恢复为空格。多对多关系本身是允许的数据适配。 |
| 章节 | min-width `120`；`共 N 章`；远程书显示服务器缓存；所有书显示浏览器缓存。 | min-width `150`，额外显示阅读进度；移动卡片还显示书籍类型和最新章。 | `must-fix`：删除所有额外可见字段。 |
| 操作 | width `100px`；依次为编辑、分组、缓存下拉、导出下拉。 | width `150` 且 fixed-right。 | `must-fix`：恢复 `100px` 和上游顺序，不固定操作列。 |
| 空结果 | 上游表格空态。 | 移动端单独显示 `el-empty`。 | 删除移动空态；统一交给表格。 |

字段名从上游 `name/origin/totalChapterNum` 映射到 Go API 的
`title/sourceId/chapterCount` 属于技术栈等价，不得借此改变可见语义。

## 3. 每本书动作合同

| 动作 | 固定上游状态转换 | OpenReader disposition |
|---|---|---|
| BookInfo | 点击书名打开共享 BookInfo；BookManage 保持打开。 | 保留唯一 overlay 入口，浏览器验证叠层关闭后 manager 仍在。 |
| 编辑 | `editBook` 打开共享编辑器；成功回调重新载入 manager。 | 保留共享编辑器与字段隔离增强；成功后强制刷新 manager。 |
| 分组 | 打开共享 BookGroup set 模式；BookManage 保持打开。 | 保留多对多数据适配与空选择保护；BookGroup 独立合同另审。 |
| 缓存菜单 | 远程：服务器、浏览器、删除服务器、删除浏览器；本地只显示浏览器两项，顺序固定。 | 当前菜单顺序已一致。 |
| 缓存中外观 | 按钮显示 loading 图标和 `缓存中`，下拉仍可打开；再次选择缓存命令时取消该书任务并提示 `已取消缓存`。 | `must-fix`：删除直接按钮“停止 n/total”；保留 JWT fetch SSE、进度传输、per-user/per-book task 和 AbortController，但不把额外进度暴露为另一产品交互。 |
| 删除服务器缓存 | 确认 `确认要删除服务器上《书名》的缓存章节吗?`；成功 `删除服务器缓存成功` 并刷新列表。 | 恢复精确可见文案；底层事务、路径安全、图片清理和 owner scope 保留。 |
| 删除浏览器缓存 | 确认 `确认要删除浏览器中《书名》的缓存章节吗?`；成功 `删除浏览器缓存成功` 并重算所有行。 | 恢复精确可见文案；账号/书籍作用域增强保留。 |
| 导出 | 只显示 `导出为TXT`、`导出为Epub`，新窗口下载。 | 只保留 TXT/EPUB；Blob/JWT 下载是技术等价。空格必须按上游删除。 |

缓存引擎的全文、并行独立任务、断开取消、失败计数、JWT 不进 URL、登录失效隔离、远端图片
安全缓存等既有增强继续由
[`book-management-cache-p2-contract.md`](book-management-cache-p2-contract.md) 约束。本轮只撤销与上游冲突的可见“停止/进度”结构，不回退底层能力。

## 4. 批量动作与精确状态合同

footer 顺序：左侧 `批量删除`、`批量添加分组`、`批量移除分组`、
`已选择 N 个`；右侧 `取消`。

| 动作 | 无选择 | 确认 | 成功后 |
|---|---|---|---|
| 删除 | `请选择需要删除的书籍` | `确认要删除所选择的书籍吗?`，标题 `提示` | 清空选择；刷新 manager 与根书架；`删除书籍成功`。 |
| 添加分组 | `请选择需要添加分组的书籍` | `确认要将所选择的书籍添加到${groupName}分组吗?` | 选择不清空；刷新 manager 与根书架；`操作成功`。 |
| 移除分组 | `请选择需要移除分组的书籍` | `确认要将所选择的书籍从${groupName}分组中移除吗?` | 选择不清空；刷新 manager 与根书架；`操作成功`。 |

当前 footer 在无选择时禁用三个按钮，批量分组不确认，删除确认文案为
`确定删除选中的 N 本书吗？`，成功提示也不同。这些都判定为 `must-fix`，不能继续由旧测试
标记为“上游风格”。

上游 `removeBookGroupMulti` 使用 bit-mask XOR，理论上会把一个原本不属于该组的选中项加入
该组。OpenReader 的 many-to-many `category-remove` 必须保持幂等删除，不能复制这个数据错误；
这是明确记录的正确性/数据模型适配。UI 仍向整个选择集发送“移除”意图，不显示当前额外的
`选中书籍不在该分组中` 分支。

## 5. API 与数据映射

| 上游动作 | OpenReader API | 必须保持的语义 |
|---|---|---|
| `GET /reader3/getShelfBookWithCacheInfo` | `GET /api/books` | JWT user scope；完整书架；章节总数、远端服务器缓存数、分组投影；manager 每次打开强制读取。 |
| `POST /reader3/deleteBooks` | `POST /api/books/batch {action:"delete",bookIds}` | 最多 200 是允许的安全边界；事务删除 book/chapter/progress/bookmark/category rows，提交后清理仅属当前用户的派生文件并广播。 |
| `addBookGroupMulti` | `POST /api/books/batch {action:"category-add",categoryId}` | 分类与书籍均需当前用户拥有；事务更新并返回新 shelf items。 |
| `removeBookGroupMulti` | `POST /api/books/batch {action:"category-remove",categoryId}` | 同上；幂等移除。 |
| `cacheBookSSE` | `POST /api/books/:id/cache/stream` | JWT header fetch、整本、逐章 progress、断开取消、路径与 user scope；可见按钮仍按上游。 |
| `deleteBookCache` | `POST /api/books/batch {action:"clear-cache"}` | 单本调用；事务清 DB 引用，提交后清理不再引用的远端文件/图片；返回后刷新行。 |
| `GET /reader3/exportBook` | `POST /api/books/export` | 单本 TXT/EPUB；JWT Blob 是允许适配；本地原文件兼容和安全路径校验保留。 |

REST 路径、GORM 行模型、SQLite 事务、WebSocket 同步、多对多 `BookCategory` 与上游 JSON
storage/bit-mask 不同，均为已记录技术适配；本轮没有破坏性迁移授权。

## 6. 旧测试的错误假设与替换门

必须删除或重写以下假设：

1. `bookManagementDialogContract.test.mjs` 把 `BookManagementMobileList` 存在作为正确合同。
2. `book-management-dialog-contract.mjs` 和 real-API smoke 按 viewport 在表格/移动卡片间分支。
3. real-API smoke 把批量添加分组“无确认立即提交”称作上游行为。
4. composable 测试固定了无选择静默 return、额外“不在该组”提示、当前批量确认与成功文案。
5. cache 测试可以保留底层进度/取消断言，但可见合同不得再要求 `停止 n/total`。

测试先行门：

- 静态合同锁定正常模式 gate、动态 width/top/table height、唯一表格、精确列/宽度、搜索与
  footer 文案，并明确拒绝移动卡片和阅读进度列。
- controller 合同锁定三个无选择提示、三个确认文案、删除后清空选择、分组后保留选择、
  每次打开 force load，以及关闭保留搜索词。
- 真实 mock 浏览器在 1440×900、390×844、360×800、1024×1366、1366×1024 验证：
  相同表格、选择/固定列、搜索仅书名作者、BookInfo/编辑/BookGroup 叠层、缓存中外观、
  精确确认和无横向页面溢出。
- real Go API 浏览器至少覆盖 1440×900、390×844、360×800：分类添加/移除确认、删除、
  fresh `GET /api/books` 持久化、manager 保持可用；不得拦截 `/api`。
- 保留现有 Go 生命周期、owner isolation、cache stream、TXT/EPUB、卷升级门。

## 7. 完成判定

只有在上述测试先失败于当前结构、实现后全部通过，且旧 mobile-card/extra-field/extra-message
代码和断言被删除后，BookManage 才可从 `错误重构` 改判为 `aligned`。BookGroup 的完整
管理面仍是下一份独立固定基准复审，不能由本合同代签。

## 8. 实施与验证记录（2026-08-02）

- 删除 `BookManagementDesktopTable.vue` 与 `BookManagementMobileList.vue`，桌面、手机和
  iPad 统一使用 `BookManagementTable.vue`。手机 fullscreen 仍是上游同一张表，selection
  与书名列固定；不存在移动卡片、移动全选工具或额外空态。
- `OverlayBookManagement.vue` 恢复 normal-page gate、750–1000px 动态宽度、上游内容高度与
  顶部位置。每次打开强制获取完整书架；关闭只清选择并保留查询；搜索仅匹配书名和作者。
- 表格恢复 `25 / 100 / 100 / 120 / 120 / 100` 的上游列语义，分组名称以空格连接；删除
  阅读进度、类型、最新章和 fixed-right 操作列。BookInfo、编辑和 BookGroup 继续使用共享
  overlay，并可与 BookManage 叠层共存。
- 单书动作恢复编辑、分组、缓存、导出顺序；缓存任务继续按账号/书籍独立并可跨关闭重开，
  但可见状态只显示 loading + `缓存中`。再次选择相同缓存命令只提示 `已取消缓存`；缓存、
  清理和 TXT/EPUB 文案恢复上游值。
- footer 恢复三个始终可触发的批量动作、精确无选择错误、确认文案和成功提示。分组成功后
  table selection 保留，删除成功后清空；添加与移除都向完整选择集发送事务意图。
- 测试先行在旧结构上产生 12 个聚焦失败；实现后聚焦 **20/20**、frontend **661/661**、
  Go `go test ./...`、Vite production build、脚本语法和 `git diff --check` 均通过。
- mock 生产构建浏览器合同通过 `1440×900`、`390×844`、`360×800`、`1024×1366`、
  `1366×1024`，证明单表格、精确几何、固定列、查询保留、每次强制刷新、三个空选择错误、
  BookInfo/BookGroup 共存、独立缓存/取消和无根横向溢出。
- 无 API 拦截的真实 Go/SQLite 浏览器合同通过 `1440×900`、`390×844`、`360×800`；每个
  视口使用隔离账号和临时卷，证明并发元数据编辑不覆盖分组/追更、BookGroup 空选择不写库、
  单书分组、批量添加/移除、选择保持、批量删除及 fresh `GET /api/books` 持久化。

允许差异仍只有 Vue 3/Element Plus 动态视口单位、JWT/GORM/SQLite、多用户隔离、many-to-many
幂等移除、缓存/文件安全与 Blob 下载。数据库结构、持久数据和兼容 URL 均未改变。BookGroup
完整管理面的第二轮固定基准复审仍是下一模块，本合同不代签其完成。
