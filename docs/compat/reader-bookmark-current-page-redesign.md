# Reader 书签面板添加当前段落（用户要求的允许差异）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 已按用户验收反馈由“当前页”修订为“当前段落”，并通过全量、三视口及真实 EPUB 验证。

## 用户目标

普通段落书签不再依赖长期启用“选择文字 → 操作弹窗”。Reader 内打开书签管理后，应提供
“添加当前段落”按钮，把当前阅读焦点所在的单一段落及其准确位置交给现有书签表单；选中文字
创建摘录书签的能力继续保留。

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
  - 已发布版本的 `excerpt` 由 `captureReaderBookmarkExcerpt()` 从焦点段落向后扩展，最多包含
    5 段/150 字；这不满足新的“单一当前段落”语义。
- `frontend/src/components/overlays/OverlayBookmarks.vue`
  - 当前标题区只有“导入”，与上游一致；编辑已复用全局 `OverlayBookmarkForm`。
- `frontend/src/stores/overlay.js`
  - `openBookmark(book, { createDraft })` 已能冻结 Reader 上下文，不需要改变 store 协议。
- 后端继续复用 `POST /api/books/:id/bookmarks`；`excerpt` 必须非空，无需新增路由、字段或迁移。

## 目标状态与转换

| 场景 | 目标行为 | 判定 |
|---|---|---|
| Reader 点击书签工具 | 以视口 32% 高度附近的正文块为阅读焦点，冻结该单一段落的 draft。 | `intentional-redesign` |
| 有当前段落 draft | 标题区在“导入”旁显示“添加当前段落”。 | `must-implement` |
| 无 Reader draft | 不显示“添加当前段落”，避免从书架/其他入口伪造阅读位置或把图片/音频伪装成段落。 | `must-implement` |
| 点击“添加当前段落” | 保持书签管理打开，在其上打开既有全局书签表单；书籍、章节、单段正文只读，备注可编辑。 | `must-implement` |
| 保存成功 | 复用现有创建 API 和 `openreader:bookmarks-updated`，书签管理自动刷新并继续显示。 | `must-implement` |
| 取消表单 | 只关闭表单，书签管理继续显示，且不写入记录。 | `must-implement` |
| 文本正文 | `excerpt` 只能是当前焦点块的完整文本，不再附带后续段落；`chapterId/chapterIndex/offset/percent` 必须对应该块所在章节与段落起点。 | `must-implement` |
| EPUB 正文 | 在同源 EPUB iframe 中按同一视口锚点选择可见 `p/li/blockquote`；位置继续使用该 EPUB 章节的当前滚动进度。 | `must-implement` |
| 音频/纯图片/空正文 | 不生成段落 draft，也不显示按钮；不能以章节名、书名或“当前阅读位置”冒充段落。 | `must-implement` |
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
2. Reader bookmark actions：只接受单段上下文 draft，不提前打开表单；无普通段落时返回 `null`。
3. `OverlayBookmarks`：仅有 draft 时显示“添加当前段落”，点击后以 create 模式打开共享表单。
4. 保存/取消：保存事件刷新当前列表，取消不创建且管理对话框保持打开。
5. 真实浏览器桌面 1440×900、移动 390×844/360×800：从 Reader 打开书签，点击按钮，核对
   只读书名/作者/章节/单一当前段落；工具层和管理面板状态不得被意外关闭。

## 已发布旧切片与本次修订

- 已发布的 `9ef0cb6` 实现了“添加当前页”，但其正文摘录可能包含当前段落后的上下文段落。
  用户验收后明确要求改成“添加当前段落”，因此旧三视口通过记录不再证明新目标完成。
- 新实现必须让 `useReaderBookmarkActions.currentDraft()` 接收冻结的段落级上下文；无章节或
  无真实段落时返回 `null`。
- Reader 打开书签管理时冻结当前 `chapterId/chapterIndex/offset/percent/title/excerpt/note`；
  普通入口不带 draft。管理对话框关闭后清理书籍和 draft，防止跨书复用旧位置。
- `OverlayBookmarks` 仅在有效 Reader draft 存在时显示“添加当前段落”；点击后复用根级
  `OverlayBookmarkForm` 的 create 模式。保存事件触发列表刷新，取消/保存都不关闭书签管理。
- 必须删除章节标题/书名/“当前阅读位置”的伪段落兜底，并新增文本与 EPUB 的单段焦点测试。
- 原有 421 项和三视口“当前页面创建”验证将在实现后改写并重新执行；选中文字创建流程继续保留。

## 2026-07-18 实施与验证记录

- `useReaderBookmarkActions` 接受 Reader 冻结的 `getCurrentContext()`；该上下文统一覆盖
  `chapterId/chapterIndex/offset/percent/title/excerpt`，无章节或无真实段落时返回 `null`。
- 普通文本使用与阅读进度一致的 32% 视口锚点，只读取当前 `p[data-reader-block]` 的完整文本；
  offset 固定为该段落起点，连续跨章时使用段落所属章节，而不是可能滞后的外层章节引用。
- EPUB 从同源 iframe 的 `p/li/blockquote` 中按同一锚点选择一个可见段落，并保留当前 EPUB
  章节滚动位置。音频、纯图片、错误占位和空正文不生成 draft，不再用章节名或书名伪装段落。
- 书签管理入口和操作已改名为“添加当前段落”，继续复用根级 `OverlayBookmarkForm` 和既有
  `POST /api/books/:id/bookmarks`，无数据迁移。
- 前端全量 423 项、生产构建、后端全量测试通过。`reader-mobile-contract.mjs` 在
  1440×900、390×844、360×800 严格比较表单摘录与一个焦点段落，并保留选中文字、编辑、
  导入、删除和跳转流程。`reader-epub-contract.mjs` 通过真实 EPUB 导入，在同样三个视口保存
  iframe 当前段落并验证管理面板/移动工具层保持显示。
