# Reader 书签面板添加当前页（用户要求的允许差异）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-17 已实施，并通过单元、生产构建及三视口真实浏览器验证。

## 用户目标

普通页书签不再依赖长期启用“选择文字 → 操作弹窗”。Reader 内打开书签管理后，应提供
“添加当前页”按钮，把当前阅读位置交给现有书签表单；选中文字创建摘录书签的能力继续保留。

这是用户明确要求的 `intentional-redesign`，不是对上游行为的误判。

## 上游权威行为

- `web/src/components/Bookmark.vue`
  - 书签管理标题区只有“导入”；列表支持跳转、编辑、批量删除。
  - 不提供从当前 Reader 页面创建书签的按钮。
- `web/src/views/Reader.vue`
  - `checkSelection()` 只在选择文字且设置为“过滤弹窗/操作弹窗”时调用 `showTextOperate()`。
  - `showTextOperate()` 的取消分支为“添加书签”，再打开全局 `BookmarkForm`。
- `web/src/components/BookmarkForm.vue`
  - 书名、作者、章节和正文上下文只读，备注可编辑。

## 当前 OpenReader 基础

- `frontend/src/composables/useReaderBookmarkActions.js`
  - `currentPayload()` 已能生成 `chapterId/chapterIndex/offset/percent/title/excerpt/note`。
  - `createCurrent()` 已能打开共享新增表单，但没有入口接到书签管理界面。
- `frontend/src/components/overlays/OverlayBookmarks.vue`
  - 当前标题区只有“导入”，与上游一致；编辑已复用全局 `OverlayBookmarkForm`。
- `frontend/src/stores/overlay.js`
  - `openBookmark(book)` 只携带书籍，不携带 Reader 当前页上下文。
- 后端继续复用 `POST /api/books/:id/bookmarks`；`excerpt` 必须非空，无需新增路由、字段或迁移。

## 目标状态与转换

| 场景 | 目标行为 | 判定 |
|---|---|---|
| Reader 点击书签工具 | 打开共享书签管理，并冻结一份当时的当前页 draft。 | `intentional-redesign` |
| 有当前页 draft | 标题区在“导入”旁显示“添加当前页”。 | `must-implement` |
| 无 Reader draft | 不显示“添加当前页”，避免从书架/其他入口伪造阅读位置。 | `must-implement` |
| 点击“添加当前页” | 保持书签管理打开，在其上打开既有全局书签表单；书籍、章节、内容只读，备注可编辑。 | `must-implement` |
| 保存成功 | 复用现有创建 API 和 `openreader:bookmarks-updated`，书签管理自动刷新并继续显示。 | `must-implement` |
| 取消表单 | 只关闭表单，书签管理继续显示，且不写入记录。 | `must-implement` |
| 文本页摘录 | 使用当前可见段落及上下文窗口。 | `technical-stack-equivalent` |
| EPUB/音频/图片页 | 当前 DOM 无普通段落时，至少使用章节标题；再退化到书名，保证上下文非空。 | `allowed format fallback` |
| 选中文字操作 | 保留现有“操作弹窗 → 添加书签/替换规则”，与新按钮互不依赖。 | `preserve` |
| 临时远程阅读 | 不提供持久书签按钮，沿用当前不可写限制。 | `preserve` |

## API 与数据合同

- 方法/路径：`POST /api/books/:id/bookmarks`。
- 认证、用户隔离、状态码与错误语义保持现状。
- 请求继续使用：
  `chapterId, chapterIndex, offset, percent, title, excerpt, note`。
- 不新增 SQLite 字段，不迁移 `data/`、`cache/` 或 `library/`。
- draft 只存在于当前前端 overlay 会话；关闭书签管理时必须清理，不能带到另一册书。

## 实施前测试

1. overlay store：Reader 上下文随 `openBookmark()` 保存，关闭/切书后清理；普通入口无 draft。
2. Reader bookmark actions：能只生成当前页 draft，不提前打开表单；无普通段落时摘录非空。
3. `OverlayBookmarks`：仅有 draft 时显示“添加当前页”，点击后以 create 模式打开共享表单。
4. 保存/取消：保存事件刷新当前列表，取消不创建且管理对话框保持打开。
5. 真实浏览器桌面 1440×900、移动 390×844/360×800：从 Reader 打开书签，点击按钮，核对
   只读书名/作者/章节/当前页内容；工具层和管理面板状态不得被意外关闭。

## 实施与验证记录

- `useReaderBookmarkActions.currentDraft()` 只生成当前页 draft，不打开表单、不写 API；无章节或
  非空摘录时返回 `null`。
- Reader 打开书签管理时冻结当前 `chapterId/chapterIndex/offset/percent/title/excerpt/note`；
  普通入口不带 draft。管理对话框关闭后清理书籍和 draft，防止跨书复用旧位置。
- `OverlayBookmarks` 仅在有效 Reader draft 存在时显示“添加当前页”；点击后复用根级
  `OverlayBookmarkForm` 的 create 模式。保存事件触发列表刷新，取消/保存都不关闭书签管理。
- 文本正文继续使用当前可见段落上下文；无法取得普通段落的 EPUB、音频和图片页使用章节标题，
  再退化到书名/“当前阅读位置”，满足既有后端非空上下文合同。
- 前端测试由 418 增至 421 项并全通过；生产构建通过。`reader-mobile-contract.mjs` 在
  1440×900、390×844、360×800 中创建“当前页面创建”书签，验证只读上下文、管理界面保留、
  创建顺序以及后续编辑/导入/删除/跳转。选中文字创建流程仍在同一合同中通过。
