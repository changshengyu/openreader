# P2 书籍编辑元数据与并发保存合同

状态：**2026-07-22 已完成固定基准与当前实现审查；待先补失败测试，再实施。**

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同只处理已经加入书架的书籍编辑动作。共享 BookInfo、BookManage、书架卡片和旧链接
必须继续使用同一个全局编辑器；搜索/探索中的临时书不能借此生成第二套详情或直接持久化。

## 1. 上游状态合同

| 场景 | 固定上游证据 | 必须保持的行为 |
|---|---|---|
| 编辑入口 | `web/src/views/Index.vue#editBook` 由书架编辑态和 `BookManage.vue#editBook` 调用；搜索结果只有先加入书架后才能编辑。 | 编辑目标必须是当前书架中的书；普通搜索/探索临时结果不能直接更新书架 API。 |
| 编辑内容 | 上游打开通用 JSON 编辑器，至少校验 `name`、`bookUrl`、`origin`，再调用同一 `saveBook`。 | OpenReader 可保留只暴露书名、作者、封面和简介的结构化编辑器，作为减少误改 URL、来源、进度和解析状态的安全差异。 |
| 保存完成 | `saveBook` 用服务端返回书籍替换 Vuex 书架项；BookManage 的完成回调重新加载管理列表。 | 保存成功后，书架、已打开 BookInfo、BookManage 和同书 Reader 必须收到同一服务端书籍投影；不新建路由、不关闭父级工作台。 |
| 保存失败 | JSON/必填校验失败或请求失败时不关闭编辑器，也不改本地书架。 | 前端校验、404/400/500 均保持草稿和编辑器；不得乐观写入未被服务器确认的数据。 |

## 2. 当前差异与根因

| 项目 | 当前 OpenReader 证据 | 判定 |
|---|---|---|
| 唯一编辑器 | `BookEditDialog.vue` 由 `GlobalOverlayHost → OverlayBookInfo` 唯一挂载；`Home` 与 `OverlayBookManagement` 只调用 `overlay.openBookEdit(book)`。 | `aligned`：结构化编辑器是允许差异，不恢复任意整书 JSON 编辑。 |
| 入口范围 | 可见入口目前都来自书架和 BookManage，但 `overlay.openBookEdit` 本身接受任意对象，保存只检查 `id`。 | `must-fix/test`：打开和保存都必须解析到当前书架记录；无 id、已删除或非当前用户/非当前书架对象不得进入写事务。 |
| 请求字段 | `useOverlayBookInfo#saveEditedBook` 在草稿字段之外附带打开时的 `categoryIds` 与 `canUpdate`。 | `must-fix`：旧快照会覆盖编辑期间由另一客户端或另一个面板保存的新分组/追更值。元数据编辑只允许发送 `title`、`author`、`customCoverUrl`、`intro`。 |
| API 部分更新 | `PUT /api/books/:id` 通过指针字段和原始 JSON key 判断，仅修改请求中出现的列；分类关系只在 `categoryId/categoryIds` 出现时进入事务。 | `technical-stack-equivalent`：后端已具备精确 patch 语义，本批不新增路由或 schema。 |
| 成功同步 | `applyUpdatedBookToOverlay` upsert 书架、更新当前 BookInfo，并发送 `openreader:book-info-updated` 与 `openreader:reader-book-data-updated`。 | `partial`：基础路径存在；需证明 BookManage、BookInfo 和同书 Reader 都使用服务端返回值，并且不重载章节/丢失阅读位置。 |

根因不是后端缺少部分更新，而是客户端为了沿用旧的整书保存习惯，把与编辑器无关的并发字段
重新放进了请求。OpenReader 的精确 REST 更新属于多用户/多客户端环境所需的安全适配。

## 3. 目标事务

1. 打开编辑器时，以 `book.id` 在当前 `bookshelf.books` 中解析规范书架行，并用该行建立草稿；
   无规范行则拒绝打开或立即安全关闭，不发送请求。
2. 编辑器继续只允许修改：

   - `title`：trim 后非空；
   - `author`：trim，可空；
   - `customCoverUrl`：只接受既有值、空值或当前用户刚上传的受控 cover URL，最终授权仍由后端决定；
   - `intro`：允许空文本。

3. 保存请求必须精确为上述四个 key；不得携带 `categoryId/categoryIds`、`canUpdate`、进度、
   章节、source、URL、缓存计数或时间字段。
4. 后端在单个 SQLite 事务中只保存请求出现的元数据列；编辑期间已由其他事务保存的分组、
   追更和阅读进度保持不变。返回完整当前书架投影并广播一次 `bookshelf_update`。
5. 客户端只在 200 后用服务端投影更新书架、当前 BookInfo、BookManage 和同书 Reader；
   元数据变化不清空章节缓存、不重新加载正文、不改变当前章/offset/percent。
6. 保存失败保留编辑器和用户草稿。目标书在保存前被删除时显示安全错误，并在后续书架删除事件中
   清理失效 overlay；绝不能以旧草稿重新创建书籍。

## 4. 必须先失败的测试

| 编号 | 测试 | 断言 |
|---|---|---|
| BOOK-EDIT-1 | `frontend/tests/overlayBookInfo.test.mjs` | 编辑保存只发送四个元数据 key；即使打开时含旧 `categoryIds/canUpdate` 也不得发送。成功后使用服务端返回的最新分组/追更值更新全部消费者。 |
| BOOK-EDIT-2 | overlay/store 合同测试 | 无 id、非书架对象和已从书架删除的对象不能启动保存；失败保持 dialog/draft，不产生本地 upsert 或 Reader 事件。 |
| BOOK-EDIT-3 | Go API contract | 先修改分组/追更，再只提交元数据，响应与数据库必须保留新分组/追更；外用户书 404，空标题 400，均无广播/副作用。 |
| BOOK-EDIT-4 | 真实 Go + 浏览器 | 在 BookManage 打开编辑器，模拟另一客户端更新分组/追更后保存元数据；1440×900、390×844、360×800 均显示新标题和并发字段，BookManage 保持打开，无横向溢出。 |
| BOOK-EDIT-5 | Reader 回归 | 同书 Reader 打开时保存元数据只更新标题/书籍对象，不触发章节内容请求、不改变当前章、offset、percent 或工具层/面板状态。 |

## 5. API、数据与允许差异

- 保留 `PUT /api/books/:id`、JWT 所有权、现有响应 schema 和 `bookshelf_update`；不新增数据库列、
  mounted volume 文件或备份字段。
- 保留 OpenReader 的结构化编辑器，不恢复上游可任意修改 `bookUrl`、`origin`、变量、进度和
  解析字段的 JSON 编辑器；这是多用户与数据完整性所需的明确安全差异。
- 上传接口与用户私有 cover 归属继续遵循
  [`bookinfo-shelf-mutations-p2-contract.md`](bookinfo-shelf-mutations-p2-contract.md)，本批不得放宽。
- 通过前端/Go 全量、生产构建和三视口真实浏览器后，本切片适合作为独立 Docker 用户验收批次。

