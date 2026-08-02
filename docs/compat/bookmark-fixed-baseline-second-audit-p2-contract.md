# Bookmark 固定上游第二轮复审合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游工作副本：`/private/tmp/reader-dev-bookmark-audit`。

状态：**2026-08-02 第二轮实现、源码/真实浏览器门与 Docker 发布均完成；等待设备验收。**

本合同重新检查此前已声称完成的 Bookmark 切片。既有
[`reader-dev-openreader-gap-analysis.md`](reader-dev-openreader-gap-analysis.md) 的 P2
Bookmark 记录仍可证明段落上下文、ID 隔离、顺序和备份语义，但不能证明管理器和表单的
用户可见结构已经与固定上游对齐。以下裁决覆盖旧记录中与当前 UI、反馈文案和导入预校验
冲突的部分。

## 上游权威

- `web/src/components/Bookmark.vue`
- `web/src/components/BookmarkForm.vue`
- `web/src/views/Reader.vue`
- `web/src/App.vue`
- `web/src/plugins/vuex.js`
- `web/src/plugins/config.js`
- `src/main/java/com/htmake/reader/api/controller/BookmarkController.kt`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`
- `src/main/java/io/legado/app/data/entities/Bookmark.kt`

当前映射：

- `frontend/src/components/overlays/OverlayBookmarks.vue`
- `frontend/src/components/overlays/OverlayBookmarkForm.vue`
- `frontend/src/composables/useBookBookmarks.js`
- `frontend/src/composables/useOverlayBookmarkActions.js`
- `frontend/src/composables/useReaderBookmarkActions.js`
- `frontend/src/utils/bookmark.js`
- `frontend/src/utils/readerBookmarkContext.js`
- `frontend/src/stores/overlay.js`
- `frontend/src/views/Reader.vue`
- `backend/api/bookmarks.go`
- `backend/models/models.go#Bookmark`
- `backend/api/webdav.go` 与 backup service 的书签导入导出

## 前端结构与状态转换矩阵

| 合同层 | 固定上游行为 | 当前证据 | 裁决 |
|---|---|---|---|
| 根级所有权 | `App.vue` 始终持有独立的 Bookmark 管理器和 BookmarkForm。编辑时管理器不先关闭，表单叠加在其上。 | `GlobalOverlayHost` 持有 `OverlayBookmarks` 和 `OverlayBookmarkForm`；共享 Pinia store 保存一次性 resolver。 | `technical-equivalent`：保留 Vue 3 根级双 Dialog；保存或取消表单后管理器继续显示。 |
| 管理器桌面宽度 | `dialogWidth = min(max(70vw, 750px), 1000px)`；1440px 宽时为 1000px，1024px 宽时为 750px。 | 固定 `880px`。 | `must-fix`：改为上游动态宽度。 |
| 管理器桌面纵向几何 | `dialogTop` 根据内容高度居中；表格高度最多 400px。 | 使用 Element Plus 默认 top，表格只有 `max-height="520"`。 | `must-fix`：恢复上游 top 与 400px 内容门，不让少量/大量行改变 Dialog 族几何。 |
| 管理器移动几何 | mini 模式 Dialog 全屏；表格高度为视口高度减去约 184px。 | Dialog 已全屏，但表格仍固定最多 520px。 | `must-fix`：390×844 和 360×800 必须使用剩余可见高度，页脚与关闭入口始终可达。 |
| 表格固定列 | mini 模式 selection 列宽 25px 且固定左侧；书籍列固定左侧；操作列宽 100px、不固定。 | selection 为 42px、不固定；书籍列不固定；操作列 112px 并固定右侧。 | `must-fix`：按上游恢复移动横向滚动时的列归属。 |
| 书籍单元格 | 显示 `bookName - bookAuthor`。 | 每行只重复当前书名，作者缺失。 | `must-fix`：当前书 ID API 仍可复用 overlay book，但必须显示“书名 - 作者”。 |
| 标题操作 | 上游只有“导入”。 | “导入”旁增加“添加当前段落”。 | `intentional-redesign`：这是用户明确要求，继续保留；只有 Reader 提供真实单段 draft 时显示。 |
| 列表内容 | 章节、正文上下文、备注；空值保持空白；操作为“跳转/编辑”。 | 字段相同，但正文/备注空值显示 `—`。 | `must-fix`：恢复保存值的直接展示，不把占位符伪装成数据。 |
| 空选择批量删除 | 删除按钮始终可点击；空选择后显示 `请选择需要删除的书签`，不弹确认框。 | 按钮 disabled，动作层对空数组静默返回。 | `must-fix`：恢复上游显式反馈。 |
| 删除确认与结果 | `确认要删除所选择的书签吗?`，标题“提示”，warning；成功为 `删除书签成功`。 | 文案加入数量、标题改为“批量删除书签”，成功为“书签已删除”。 | `must-fix`：恢复固定上游可见文案和确认按钮语义。 |
| 导入确认与结果 | 读取非空 JSON array 后提示 `确认要导入文件中的N条书签吗?`，标题“提示”，warning；成功为 `导入书签成功`。 | 加入“到当前书籍”、info 类型和不同成功文案。 | `must-fix`：保留当前书籍隔离实现，但用户可见流程与文案恢复上游。 |
| 导入预校验 | 上游公开字段为 `time/bookName/bookAuthor/chapterIndex/chapterPos/chapterName/bookText/content`。 | 已映射 `chapterName/bookText/content`，但标准化后仍可能保留无 `excerpt` 的行，随后被后端整批拒绝。 | `must-fix`：确认前只保留含真实 `bookText/excerpt` 的行；全空/无正文数组直接报告文件无可导入内容，不发 API。`chapterPos` 是上游分页序号，不能错误当成像素 offset。 |
| FileReader 失败回退 | 上游浏览器读取失败时把文件交给 `/readSourceFile`。 | 当前浏览器读取失败直接报错。 | `acceptable security/runtime adaptation`：现代浏览器本地 JSON 读取保持客户端完成，不为该极少分支新增任意服务器文件读取入口。 |
| 表单几何 | 与管理器共用 750–1000px 动态宽度、动态 top；mini 模式全屏。 | 固定 640px，移动已全屏。 | `must-fix`：恢复同一 Dialog 几何族。 |
| 表单字段 | 书名、作者、章节和内容只读；只有备注可编辑；无书籍身份或正文时拒绝。 | 字段与只读状态一致；后端还执行 ID/章节所有权和大小限制。 | `aligned + security adaptation`。 |
| 表单关闭状态 | 根 watcher 在保存、取消、Esc 或关闭后只调用一次回调，解除 Reader 添加锁。 | resolver 在 resolve 前清空；会话 reset 也完成 promise。 | `technical-equivalent`：继续验证只完成一次、不得把旧账号 toast/写入带到新账号。 |
| 书签跳转 | 管理器先关闭；目标章节渲染后用保存的 `bookText` 逐级相似度定位，失败显示 `无法定位内容所在段落`。 | 路由携带 chapter/offset/percent 和 excerpt；Reader 渲染后执行段落匹配。 | `aligned + allowed fast-path`：offset/percent 可先恢复，但不能替代上下文定位。 |
| 选中文字创建 | 只接受能匹配一至两段完整正文的选择，保存匹配段开始最多五段/约 150 字。 | 已有相同段落匹配和上下文窗口。 | `aligned`；移动 Selection API 与 EPUB iframe 仍须真实浏览器复验。 |
| 添加当前段落 | 上游没有此按钮。 | 从视口 32% 阅读焦点冻结一个真实文本/EPUB 段落。 | `intentional-redesign`：按用户明确要求保留；音频、纯图片、空正文和临时阅读不得伪造 draft。 |

## API、数据与同步合同

OpenReader 保留现有 REST/SQLite/JWT 适配，不恢复 reader-dev 的 JSON 文件写接口：

| 路径/数据 | 必须保持的语义 | 当前裁决 |
|---|---|---|
| `GET /api/books/:id/bookmarks` | 只允许调用者拥有的书；`id ASC` 稳定创建顺序；返回位置、上下文和备注。 | 已实现，需补明确的跨账号读测试。 |
| `POST /api/books/:id/bookmarks` | owned book；可选 chapter 必须属于该书；正文上下文非空；数字归一化；持久化成功后才广播。 | 已实现。 |
| `POST /api/books/:id/bookmarks/batch` | 全部行先验证，再在一个事务内按请求顺序创建；一行失败不保留前缀。 | 已实现；前端还需在确认前过滤无正文 legacy 行。 |
| `PUT /api/bookmarks/:id` | 只允许 owner；只改 note，原位置/上下文保持只读。 | 已实现，需补跨账号更新测试。 |
| 单条/批量删除 | 单条只按 owner；批量还按当前 book；稳定返回实际删除 ID，事务后广播。 | 已实现，需补跨账号删除和“同账号另一册书 ID”回归证据。 |
| 多书签身份 | 不复制上游按 `bookName/bookAuthor` 覆盖整本书唯一行的控制器缺陷。 | `intentional technical adaptation`：保留多个 ID-backed 书签和既有数据。 |
| 备份/恢复 | 继续读取 singular `bookmark.json` 与 plural `bookmarks.json`；现代行保留创建顺序/独立身份，目标实例重绑 chapter。 | 已实现；不得改 schema 或破坏 portable/历史卷。 |
| 同步事件 | 只向当前用户广播，且只在事务/写入成功后；前端只刷新匹配 book。 | 已实现；继续保留认证 lifecycle guard。 |

本轮不新增数据库列，不迁移 `data/`、`cache/` 或 `library/`，不合并既有书签，也不改变
portable/逻辑备份格式。

## 测试先行门

实施前必须先让当前代码在以下合同上失败：

1. 静态组件合同：
   - 管理器和表单使用 `min(1000px, max(750px, 70vw))` 与上游 top；
   - 管理器表格 desktop 400px、mobile `100dvh - 184px`；
   - mobile selection/book 两列固定，操作列不固定；
   - 行内显示 `书名 - 作者`，不为正文/备注注入 `—`。
2. 动作合同：
   - 空选择调用删除显示 `请选择需要删除的书签`，且不 confirm、不请求；
   - 非空删除、导入的确认标题/类型和成功提示与上游一致；
   - 取消/关闭不产生成功提示或请求。
3. 导入合同：
   - reader-dev `chapterIndex/chapterName/bookText/content` 精确映射；
   - `chapterPos` 不错误映射为 offset；
   - 无正文、错误数组、混合空行在确认前确定性处理，不能让后端用整批 `400` 代替前端预校验。
4. API/数据合同：
   - 第二账号无法 list/update/delete 第一账号的任一书签；
   - 同账号另一册书的 ID 不能被当前书批量删除；
   - 失败写不广播，成功批量写/删只广播一次且在 commit 后；
   - 既有顺序、备份恢复和 chapter rebinding 测试继续通过。
5. 真实浏览器：
   - 1440×900：1000px Dialog、400px 表格、完整书名作者、编辑表单同宽；
   - 1024×1366 与 1366×1024 iPad：750px/956px 自适应 Dialog，关闭入口可见，不能覆盖到无法关闭；
   - 390×844 与 360×800：全屏、剩余高度表格、固定左列、可横向查看操作、工具层保持显示且无点击穿透；
   - 覆盖空选择错误、导入、编辑、添加当前段落、批量删除和上下文跳转。
6. 发布闸门：frontend 全量、Go 全量、production build、普通正文与真实 EPUB Bookmark
   smoke、新卷和历史卷兼容门全部通过后，才提交本地多架构 Docker 发布。

## 允许差异

- Vue 3/Pinia/Element Plus 根级 Dialog 实现。
- JWT、多用户/书籍 ID、SQLite 多书签、事务和 WebSocket 同步。
- 显式把 legacy 导入归属到当前书籍，不允许导入文件跨账号/跨书写入。
- offset/percent 快速恢复，随后仍由保存上下文完成上游式定位。
- 用户明确要求的“添加当前段落”入口。
- 不新增上游 `/readSourceFile` 任意文件读取回退。

除以上差异外，当前组件或旧测试不是保留错误 UI、文案或几何的理由。

## 第二轮实施结果（2026-08-02）

- `OverlayBookmarks` 与 `OverlayBookmarkForm` 已恢复同一动态 Dialog 几何：桌面宽度
  `min(1000px, max(750px, 70vw))`，top 使用固定上游等价的动态居中表达式；mini
  模式继续全屏。
- 书签表格桌面内容高度最多 400px，手机为 `100dvh - 184px`；mini 模式 selection
  25px 与书籍列固定在左侧，操作列 100px 且不固定。书籍单元格恢复
  `书名 - 作者`，正文和备注直接显示保存值，不再注入破折号占位。
- 空选择批量删除恢复 `请选择需要删除的书签`；删除和导入的确认标题、按钮、warning
  类型及成功文案均恢复固定上游。用户要求的“添加当前段落”继续作为明确允许差异保留。
- JSON array 无论为空还是只含无正文 legacy 行，均由统一导入动作确定性返回
  `书签文件没有可导入内容`；混合数组在确认前过滤无正文行。`chapterPos` 继续只作为
  上游分页信息读取，不映射为 OpenReader 像素 offset。
- Go 合同新增跨账号 list/update/delete/batch-delete 隔离、同账号另一册书 ID 的当前书
  删除隔离，以及批量写失败零持久化/零广播、成功批量写删各一次事务后广播证据；未改
  schema、备份格式或现有用户数据。

## 第二轮验证与发布结果

- frontend 聚焦 Bookmark：16/16；frontend 全量：654/654。
- Go 全量 `go test ./...` 通过；Vite production build 通过。
- 普通 Reader 浏览器合同通过 1440×900、390×844、360×800、1024×1366 与
  1366×1024 自适应 iPad，以及 1024×1366 强制移动 iPad。合同直接测量桌面
  1000px/400px、iPad 750px/956px、手机全屏剩余高度、固定列、关闭入口、空选择、
  空/无正文/混合导入、编辑、当前段落、批量删除和上下文跳转。
- 真实 Go API EPUB 合同通过 1440×900、390×844、360×800；验证同一管理器/表单
  几何、书名作者、当前 iframe 段落保存以及保存后管理器继续显示。
- 干净提交 `f2f0d6e0772a5a57726152f0fcdb6ac56e2465cb` 先完成本地候选构建；新卷
  portable v1、portable v2 assets、跨用户与重启门，以及历史 TXT、EPUB、UMD、CBZ、
  相对缓存与 owner-isolation 升级卷门全部通过。
- 同一提交由本机完成 `linux/amd64` 与 `linux/arm64` 构建并发布为
  `ghcr.io/changshengyu/openreader:f2f0d6e` 和 `latest`。两个远端标签共同指向 OCI index
  `sha256:cfaba7d453bde2a4b44198aa57ef8ef5ecbaa3b4cce0ab77b4c816311455c736`；amd64
  manifest 为 `sha256:11cc11aa6907e2e57275e545249a65ce80dd7c1f3cb6f823df0db6f4139c010f`，
  arm64 manifest 为 `sha256:b96a88b536af4ae5ea31265df227c5880818b78b1fa0f781d4f0b1251e1989bf`。
- 本轮状态现在为 `aligned / Docker-published / awaiting device verification`。镜像同时包含
  先前 `6fde5ab` 的纯黑阅读内容面与纯白文字修复；部署实例仍需 pull 并强制重建容器，
  `/api/health` 返回 `f2f0d6e` 后才是在验证本版。
