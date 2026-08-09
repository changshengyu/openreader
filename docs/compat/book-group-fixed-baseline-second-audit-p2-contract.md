# BookGroup 第二轮固定上游复审合同（P2）

状态：**aligned / Docker-published / awaiting-device-verification**。第二轮应用重建与完整发布证据见第 10 节。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只提取合同，不修改应用代码。2026-07-19 的
[`book-group-p2-contract.md`](book-group-p2-contract.md) 对内置分组持久化、统一投影、
多用户、同步和备份的结论继续有效；其中关于当前前端已对齐、失效标签自动回退及现有测试
足以证明上游一致性的结论由本合同取代。

## 1. 权威来源与当前映射

固定上游权威文件：

- `web/src/components/BookGroup.vue`
- `web/src/App.vue#showBookGroupDialog/loadBookGroup`
- `web/src/views/Index.vue#showManageBookGroup/getShowShelfBooks/bookGroupDisplayList/showBookGroup`
- `web/src/components/BookInfo.vue#showSetBookGroup`
- `web/src/components/BookManage.vue#setBookGroup`
- `web/src/plugins/vuex.js#setBookGroupList/builtInBookGroupMap/dialog*`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#getBookGroups`、
  `saveBookGroup`、`saveBookGroupOrder`、`deleteBookGroup`、`saveBookGroupId`
- `src/main/java/io/legado/app/data/entities/BookGroup.kt`

当前 OpenReader 映射：

- `frontend/src/components/overlays/OverlayBookGroups.vue`
- `frontend/src/composables/useOverlayBookGroups.js`
- `frontend/src/stores/overlay.js`、`frontend/src/stores/bookshelf.js`
- `frontend/src/views/Home.vue`、`frontend/src/utils/bookGroups.js`
- `frontend/src/components/overlays/OverlayBookInfo.vue`
- `frontend/src/components/overlays/OverlayBookManagement.vue`
- `backend/api/categories.go`、`backend/api/book_groups.go`、`backend/api/books.go`
- `backend/services/bookgroups/service.go`、`backend/models/models.go`

BookGroup 没有业务路由或查询参数。上游由根 `App` 常驻唯一组件并通过事件总线打开；当前由
`GlobalOverlayHost` 常驻唯一 overlay 并通过 Pinia intent 打开，属于 Vue 3 等价所有权。

## 2. 所有权、打开状态与弹窗几何

| 合同 | 固定上游行为 | 当前行为 | 裁决 |
|---|---|---|---|
| 唯一所有权 | 根 `App` 只挂载一个 `BookGroup`；Index、BookInfo 和 BookManage 只发送 manage/set intent。 | `GlobalOverlayHost` 只挂载一个 `OverlayBookGroups`；三个入口共享 Pinia overlay。 | `aligned / Vue 3 equivalent`。 |
| 页面门禁 | 仅 `config.pageType === "正常"` 渲染；切离正常模式后对话框不可继续存在。 | 没有 normal-page gate。 | `must-fix`：与 BookManage 使用同一规范化 `reader.pageType === "normal"` 门禁并关闭可见状态。 |
| manage 打开 | Index 先调用 `loadBookGroup(true)` 强制刷新，再打开“分组管理”；刷新与打开不互相阻塞。 | 直接打开；visible watcher 只 `ensure*Loaded`，已有内存数据时不请求服务器。 | `must-fix`：manage intent 每次必须触发当前账号完整 BookGroup 网络读取；书架只需保证已加载。 |
| set 打开 | 从当前全局 BookInfo 取书；清空旧选择、只列自定义组，并在 nextTick 按书的 group mask 预选。 | 目标书与预选基本等价，但选择由表格外部数组和自绘 checkbox 管理。 | 状态结果 `aligned`，结构 `must-fix`：恢复 Element table selection 为单一选择权威。 |
| 关闭 | `cancel()` 把内部 set 标记恢复为 false 并关闭；父 BookInfo/BookManage 不关闭。 | 只关闭 visible，store 中 mode 仍保留；父 overlay 可并存。 | 父层并存 `aligned`；关闭时恢复 manage 默认值，防止直接重开继承旧 set intent。 |
| 桌面宽度 | `min(max(70vw, 750px), 1000px)`。 | 固定 `min(760px, 100vw - 48px)`。 | `must-fix`。 |
| 桌面 top | `(viewportHeight - dialogContentHeight - 184px) / 2`，等价 CSS 为 `max(15dvh, (100dvh - 584px)/2)`。 | Element Plus 默认 top。 | `must-fix`。 |
| table 高度 | mini 为 `100dvh - 184px`；桌面为 `min(400px, 70dvh - 184px)`。 | 未指定高度，随内容伸展。 | `must-fix`。 |
| mini | 同一个 Dialog fullscreen，仍是同一张表和控制器。 | fullscreen 正确，但 set/manage 渲染两张不同表。 | fullscreen `aligned`；双表 `must-fix`。 |

`destroy-on-close`、请求代际保护、pending loading 和 authenticated operation guard 可以作为
Vue 3/多用户并发适配保留，但不得改变上述可见结构、默认模式或父弹层状态。

## 3. 唯一表格与两种模式

上游只有一个 `el-table`，通过条件列和数据投影切换模式。当前两个 template 分支不是技术栈
要求，必须收敛。

| 区域 | 固定上游 | 当前偏差 | 必须结果 |
|---|---|---|---|
| 数据 | set 只列正 ID 自定义组；manage 列四个内置组和全部自定义组，统一顺序。 | 数据投影正确。 | 保留 `categories` / `bookGroups` 双投影。 |
| selection | set 显示原生 selection 列，width `25`；manage 不显示。只点击 checkbox 改选择，整行点击不切换。 | 自绘 checkbox width `46`，整行点击切换。 | 恢复原生 selection，删除 row-click 选择。 |
| 分组名 | 一列 `分组名`，min-width `100`；拖动图标位于名称左侧，set 模式也可见但排序被禁用。内置显示 `当前名(不可变默认语义名)`。 | 拖动图标是独立 `46px` 列；名称 min-width `130`；set 没有图标；附加 `N 本` 描述。 | 恢复单列；计数只可内部用于删除门禁，不得显示。 |
| 显示 | 仅 manage 显示，min-width `80`；switch 无 active/inactive 文本。 | width `120`，显示“显示/隐藏”文字和 per-row loading。 | 列宽与无文字外观恢复；loading 可保留但不得挤出上游列宽。 |
| 操作 | width `100px`；所有行均有“编辑”。仅 manage、正 ID、自定义且空组显示红色“删除”。 | set width `84`、manage min-width `180`；删除条件基本正确。 | 两模式共用 `100px` 操作列。 |
| 空态 | 表格自己的空态。 | manage 额外渲染“还没有自定义分组”，即使四个内置行仍存在。 | 删除额外 empty。 |
| Sortable | 打开后绑定同一 tbody；set 时 disabled；拖动后只更新 draft，脏时显示保存。 | 只在 manage 创建，增加 animation/fallback 产品参数。 | 使用同一表 lifecycle；保留明确 drag handle 修正，但移除未经授权的动画/fallback 参数。 |

上游 group list 在 set 模式重新加载时会重建 table data，只有首次打开会按当前书重新预选；不得
让外部 selected-id 数组在重载后悄悄保留一套不同于 table selection 的状态。

## 4. footer 与精确动作合同

同一个 footer 的固定顺序：左侧 primary medium `添加分组`；脏排序时左侧 primary medium
`保存排序`；set 模式右侧 primary medium `确认`；最后 medium `取消`。不得把手机 footer
改成另一套全宽网格，set 的添加按钮也不是 plain 变体。

| 动作 | 固定上游可见合同 | 当前偏差 | 裁决 |
|---|---|---|---|
| 添加 | 空 prompt body；标题 `添加分组`；input validator `分组名不能为空`；成功 `添加成功`。 | body `输入分组名称`；validator `分组名称不能为空`；成功 `分组已创建`。 | `must-fix` 精确文案与提交状态。 |
| 编辑 | 空 prompt body；标题 `编辑分组`；填入当前名称；相同名称也提交；成功 `修改成功`。 | 标题 `重命名分组`，body 有提示，相同名称静默返回；成功 `分组已重命名`。 | `must-fix`。 |
| 显隐 | 保存当前行完整语义；成功统一 `修改成功`；失败前缀 `修改失败 `。 | 成功 `分组已显示/分组已隐藏`。 | `must-fix` 文案；REST patch 是等价 transport。 |
| 删除 | UI 仅空自定义组可点；确认 `确认要删除该分组吗?`，标题 `提示`；成功 `删除分组成功`。 | 确认包含名称、标题 `删除分组`；成功 `分组已删除`；直接 controller guard 另发警告。 | `must-fix` 可见确认与成功值；后端非空/内置强保护保留。 |
| 设置 | 空选择 error `请选择书籍分组`，不请求；成功 `设置成功`，关闭 BookGroup，更新 BookInfo 与书架。 | 空保护正确；成功 `分组已设置`。 | `must-fix` 成功文案；多对多原子替换保留。 |
| 排序 | 仅脏时可见；成功 `保存成功`；失败前缀 `保存失败`；成功后强制刷新组列表。 | 成功 `分组排序已更新`，失败 `分组排序失败`。 | `must-fix` 文案和刷新合同。 |

取消 prompt/confirm 必须无请求、无 toast、无状态突变。服务端对名称 trim、80 字节模型上限、
每用户唯一名称和严格排序 token 的限制是既有 SQLite/数据安全适配，不迁移或放宽；前端仍恢复
上游可见文案。

## 5. 书架分组标签合同的复审修正

- 四个内置组和自定义组的名称、显隐、统一顺序、非空过滤和筛选语义已经由真实投影实现；
  `Category + BookCategory` 多对多与增强 `isLocalBook` 保留。
- 上游 tab 只显示分组名。当前 `title="名称 (数量)"` 是未授权的额外可见信息，应删除；ARIA
  tab 语义可保留。
- 上游把当前 groupId 留在 shelfConfig。若该 token 仍有书但被隐藏，tab 列表没有它，正文仍按
  该组筛选；若没有任何可见非空 tab，computed 临时返回“全部”语义，但不覆写持久值。
- 当前 `resolveBookGroupSelection()` 在 token 不可见时自动改用第一项可见分组。2026-07-19 合同
  把这一行为写成正确结果，但固定源码不支持该结论；它属于 `must-fix` 的错误重构。稳定 token
  存储可以保留，但不得暗中改变上游选择状态机。
- 当前 button-based tab 可作为 Vue 3 等价结构保留的前提，是五视口浏览器证明 stretch、横向
  滚动、active underline、隐藏 token 与正文筛选均和上游一致；现有纯源码测试不足以证明。

## 6. API 与数据合同

| 上游动作 | OpenReader 映射 | 裁决 |
|---|---|---|
| `getBookGroups` | `GET /api/book-groups` | 四内置惰性补齐、用户级统一投影已对齐。 |
| `saveBookGroup` 内置 | `PUT /api/book-groups/:key` | 稳定语义 key、事务与用户隔离为允许适配。 |
| `saveBookGroup` 自定义 | `POST /api/categories` / `PUT /api/categories/:id` | Category ID、唯一名称与多对多是允许适配。 |
| `saveBookGroupOrder` | `PUT /api/book-groups/reorder {keys}` | 完整混合事务、拒绝重复/缺失/外用户 token 是更强数据保护。 |
| `deleteBookGroup` | `DELETE /api/categories/:id` | UI 空组门禁外，服务端再次拒绝非空组是允许增强。 |
| `saveBookGroupId` | `PUT /api/books/:id/category {categoryIds}` | UI 拒绝空选择；事务替换关系和一次 post-commit shelf event 已对齐。 |

数据库、API 主链和备份结构不需要重写，但本轮确认一个真实持久化缺口：

1. `Category.Show` 带 `gorm:"default:true"`。一次性 GORM/SQLite 探针证明用
   `models.Category{Show:false}` 创建时实际存储为 `true`。
2. `bookgroups.Service.Restore()` 为 reader-dev `bookGroup.json` 新建隐藏自定义组时正走该创建
   分支，因此 `show:false` 丢失；现有测试只断言内置项的隐藏状态，没有断言该自定义项。
3. `restoreCategoriesFromData()` 构造恢复行时完全没有复制 `Show`，旧 OpenReader
   categories-only 备份的隐藏状态也会丢失。

这两条恢复路径判定为 `must-fix`。修复必须使用可区分“字段缺失”和显式 false 的 restore DTO
或显式 column write：旧归档缺少 `show` 时默认 true，显式 false 必须原样恢复。不得修改现有表、
ID、关系、volume 路径或备份成员。

## 7. 旧测试的错误假设

必须替换而非迁就：

1. `bookGroupDialogContract.test.mjs` 要求 set/manage 两个 template 分支，却没有锁定唯一
   `el-table`、normal gate、动态几何或精确列宽。
2. `overlayBookGroups.test.mjs` 把行内 `N 本`、`分组已创建/重命名/隐藏/删除/排序已更新`、
   含名称删除确认和 manager-only Sortable 参数当成上游合同。
3. `book-management-dialog-contract.mjs`、`book-management-real-api-contract.mjs`、
   `bookinfo-shelf-mutations-real-api-contract.mjs` 固定 `.group-set-table` 与自绘 checkbox 结构。
4. `book-group-real-api-contract.mjs` 固定当前重命名 prompt/成功文案，只覆盖 1440/390/360
   manage 模式，没有覆盖 iPad、normal gate、统一表和 set/add/edit 状态。
5. `bookGroups.test.mjs` 把“失效 token 自动回退第一项”作为正确行为。
6. `book_groups_p2_contract_test.go` 没有断言 reader-dev 隐藏自定义组和旧 categories-only 隐藏
   状态的恢复结果，因此无法发现已证实的 GORM false→true 缺口。

## 8. 测试先行与实施顺序

1. 静态合同先要求 normal gate、动态 width/top/table height、源码只有一个 `el-table`、条件
   selection/show 列、名称内 drag icon、`25/100/80/100` 列语义、单 footer，并拒绝 count、
   switch text、extra empty、row-click 和手机 footer 网格。
2. controller 失败测试锁定初次预选、table selection 权威、空选择、重载选择语义、精确 prompt/
   confirm/success/error、每次 manage force load、关闭 mode reset、脏排序和取消无副作用。
3. Home/store 测试锁定上游隐藏/失效 token 状态；保留统一投影、多用户缓存和 WebSocket 收敛。
4. Go 失败测试先证明 reader-dev `show:false` 自定义组与 categories-only `show:false` 恢复错误，
   再以显式字段写入修复；保留现有全量排序、跨用户、round-trip 与删除保护测试。
5. mock 生产浏览器覆盖 `1440×900`、`390×844`、`360×800`、`1024×1366`、
   `1366×1024`：唯一表、精确几何/列/footer、manage/set、父 overlay 共存、iPad 可关闭、无根
   横向溢出、normal gate。
6. 无 API 拦截的真实 Go/SQLite 浏览器至少覆盖 1440/390/360：强制刷新、四内置、自定义
   add/edit/show/delete、混合排序、set 初选/空保护/保存、第二客户端同步和 fresh API 持久化。
7. 前端全量、Go 全量、production build、新旧 volume/backup 门通过后，才允许提交 Docker 用户验收。

## 9. 完成判定与允许差异

只有应用代码、旧测试和浏览器探针全部按本合同重建，隐藏自定义组恢复缺口被修复，且上述门禁
同时通过，BookGroup 才能重新标记 `aligned`。

允许差异仅为 Vue 3/Pinia/Element Plus/Gin/GORM transport、JWT 多用户、Category/BookCategory
多对多、稳定 token、严格事务与跨用户/非空删除保护、authenticated operation generation、
增强本地书识别及兼容旧 API/备份。当前双表、额外计数/文案、自动切组和恢复丢失 false 均不是
允许差异。

## 10. 实施与验证记录（2026-08-02）

本合同已按“失败测试 → 实现 → 真实浏览器”完成，不再处于 implementation pending：

- `OverlayBookGroups.vue` 只保留一个 normal-page `el-dialog` 和一个 `el-table`；桌面 width/top、
  mini fullscreen、桌面/mini table height、条件 selection/show 列、名称内 drag icon、操作列和
  单 footer 均恢复固定上游结构。manage 每次打开 force 读取投影，关闭恢复 manage 默认 mode。
- `useOverlayBookGroups.js` 以 Element table selection 为 set 模式权威，恢复初次预选、空选择
  error、添加/编辑/显隐/删除/设置/排序精确文案和刷新顺序。set 模式复用同一 Sortable 并禁用；
  组投影变化按上游 watcher 语义重置 manage draft，分类变化会清除 set 的陈旧选择。
- 浏览器测试先发现一次真实 Vue 3 适配缺口：Sortable 已移动 tbody 后，旧实现又用响应式 draft
  重排 table data，造成可见顺序与落库顺序“双重位移”。最终实现与上游一致：Sortable 独占
  保存前 DOM 顺序，draft 只记录 key，强制刷新后才由服务器投影重新渲染。
- Home 分组 tab 不再显示额外计数 title；隐藏、空组或未知的持久 token 不再被自动改写为第一项
  可见组。仅没有任何可见非空组时临时使用 `builtin:all`，不覆写持久偏好。
- reader-dev restore 新建隐藏自定义组后在同一事务中显式写 `show=false`；categories-only restore
  使用 `*bool` 区分字段缺失与显式 false。旧归档缺失 `show` 仍默认可见，不改表、不改 ID/关系。

最终验证证据：

- frontend `663/663`；Vite production build 通过；Go `go test ./...` 全量通过；差异和脚本语法
  检查通过。
- `book-group-real-api-contract.mjs` 在 1440×900、390×844、360×800、1024×1366、
  1366×1024 全部通过：真实 Go/SQLite、唯一表、精确几何、四内置、自定义添加/编辑/删除、
  显隐、多客户端同步和可见顺序逐项落库。
- `book-management-real-api-contract.mjs` 在 1440/390/360 通过 set 初选、空保护和保存；
  `bookinfo-shelf-mutations-real-api-contract.mjs` 同三视口通过共享 BookInfo set 流程；mock
  `book-management-dialog-contract.mjs` 在五视口通过父 overlay 共存及 iPad 可关闭合同。
- 后端聚焦恢复合同证明 reader-dev 隐藏自定义组与 categories-only 显式 false 均可保留，缺失
  `show` 仍为 true。

实现提交 `5459f02c0a543b342f6ce8e722d3db0588504807` 已推送 `main`，并由本机 OrbStack
按固定 Node/Go/Alpine 基础镜像构建和发布
`ghcr.io/changshengyu/openreader:5459f02` 与 `ghcr.io/changshengyu/openreader:latest`。
两个标签共同指向 amd64/arm64 OCI index
`sha256:a8d04ee3e5f6aac3e2a41b908da175e17b7a57e1fb440df51677f35d9496afe9`；amd64 manifest 为
`sha256:e5bac334f271f5f78b21e6d8305d1b7dfda1b7c7dc7463a5b2c2370d6b2bb4d7`，arm64 manifest 为
`sha256:4ab465047829e9411635a25417e0595e94192868c6b670f076c25370c7cedc2a`。运行时 health 已核对
`version=5459f02`、完整 commit 与 build date；本地验证镜像的 arm64 manifest 与远端逐字一致。

新卷 portable v1/v2 assets、跨用户恢复与重启，以及历史 TXT/EPUB/UMD/CBZ、relative-cache、
owner-isolation Docker 门禁均通过。状态更新为 **aligned / Docker-published / awaiting device
verification**。允许差异仍仅限第 9 节所列 Vue 3/Go/多用户/多对多和安全增强；双表、可见计数、
扩展文案、自动切组与恢复丢失 false 已删除。部署端必须 pull 并 force-recreate，且
`/api/health` 返回 `5459f02` 后才算运行本批。
