# 书内正文搜索固定上游第二轮复审合同（P2）

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游工作副本：`/private/tmp/reader-dev-content-search-audit`。

状态：**2026-08-02 第二轮源码合同已完成；测试先行、实现、真实浏览器与 Docker 均待执行。**

本合同重新检查此前已标记为完成的
[`book-content-search-p2-contract.md`](book-content-search-p2-contract.md)。旧合同已经证明原始正文、
精确/区分大小写/重叠匹配、UTF-16 索引、游标完整性、取消、跨章定位和多用户隔离；这些
结论继续有效。但旧合同只把“根 Dialog + 移动全屏”当成可见外壳对齐证据，没有核对共享
Dialog 几何、标题输入、表格、加载状态和页脚，也没有验证搜索词在前后端及 Reader intent
中保持原样。本合同覆盖旧记录中与这些项目冲突的结论。

## 固定上游权威

- `web/src/components/SearchBookContent.vue`
- `web/src/plugins/vuex.js#dialogWidth/dialogTop/dialogContentHeight/isNormalPage`
- `web/src/App.vue` 的根级所有权和事件接线
- `web/src/views/Reader.vue#showSearchBookContentDialog/showMatchKeyword`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#searchBookContent/searchChapter/searchPosition/getResultAndQueryIndex`
- `src/main/java/io/legado/app/data/entities/SearchResult.kt`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`

OpenReader 当前映射：

- `frontend/src/components/overlays/OverlayBookContentSearch.vue`
- `frontend/src/composables/useBookContentSearch.js`
- `frontend/src/composables/useReaderContentSearchIntent.js`
- `frontend/src/composables/useReaderSearchNavigation.js`
- `frontend/src/utils/readerBookSearch.js`
- `frontend/src/stores/overlay.js`
- `frontend/src/components/GlobalOverlayHost.vue`
- `frontend/src/views/Reader.vue`
- `frontend/src/api/books.js`
- `backend/api/books.go#searchBookContent/legacySearchBookContent/collectContentMatchesContext`
- `backend/services/contentsearch`

## 可见结构与状态转换矩阵

| 合同层 | 固定上游行为 | 当前证据 | 第二轮裁决 |
|---|---|---|---|
| 根级所有权 | `App.vue` 始终挂载唯一 `SearchBookContent`；Reader 只发出当前书打开事件并消费所选结果。 | `GlobalOverlayHost` 挂载唯一 overlay，Reader 通过 Pinia 单调 intent 消费。 | `technical-equivalent`：继续保留，不回到 Reader Drawer/Workspace。 |
| 可用页面模式 | Dialog 仅在 `pageType === 正常` 时存在；简洁/Kindle 模式不会继续显示根搜索。 | overlay 不读取 `reader.pageType`；切换到简洁模式时既有 Dialog 可继续存在。 | `must-fix`：正常模式之外关闭并清理可见会话，不允许不可达的模态层残留。 |
| 桌面宽度 | `min(max(70vw, 750px), 1000px)`；1440px 为 1000px、1024px 为 750px、1366px 为 956.2px。 | 固定 `880px`。 | `must-fix`。旧测试仅断言宽度 `>=760`，本身错误，必须替换。 |
| 桌面 top | `(viewportHeight - dialogContentHeight - 184px) / 2`；900px 高时 158px、1024px 高时 220px、1366px 高时 391px。 | 未传 `top`，使用 Element Plus 默认值。 | `must-fix`：与 Bookmark/BookmarkForm 使用同一固定上游 Dialog 几何族。 |
| 表格高度 | 桌面 `min(70vh - 184px, 400px)`；mini 为 `100vh - 184px`。 | `max-height="520"`；手机仍最多 520px。 | `must-fix`：1440×900/宽屏 iPad 为 400px；390×844 为 660px；360×800 为 616px。 |
| 标题输入 | title slot 内为 `size=mini`、75% 宽、20% X 位移、搜索前缀图标；打开只恢复表格位置，不自动聚焦。 | 输入占满 header、`clearable`、无前缀；`handleOpened` 自动 focus。 | `must-fix`：移除移动端自动弹键盘的偏差，恢复上游标题输入结构。 |
| 表格列 | 章节列最小 100px；搜索结果列最小 250px；直接显示 `resultText`。 | 150px/300px；优先 `excerpt` 并为空值注入 `—`。 | `must-fix`：新接口可继续提供 additive `excerpt`，但投影必须统一到上游结果文本且空值保持空。 |
| 加载状态 | 既有表格不被遮罩；只有“加载更多”禁用并改成“加载中”。搜索期间仍可选择已加载结果。 | `v-loading` 遮住整张表，按钮还使用 loading spinner。 | `must-fix`：恢复非阻塞表格及上游按钮状态。 |
| 空状态 | 上游始终显示空表，不加入引导/无结果占位。 | 额外显示两套 `el-empty`。 | `must-fix`：删除未获授权的结构；章节失败/安全截断 warning 是已记录的安全增强，继续保留。 |
| 页脚 | “加载更多”和有条件的“跳转上次位置”浮在左侧；“取消”在右侧；加载更多只在 loading 时禁用，文案只有“加载中/加载更多”。 | 所有按钮右对齐/手机均分；终态改成 disabled“没有更多”，并增加“搜完全书”。 | `must-fix + retained enhancement`：恢复上游三项的顺序、对齐、文案和状态；有界扫描所需“搜完全书”仅作为明确增强保留，但不得挤坏手机 184px 几何，可限制在非 mini 场景。 |
| 同书关闭/重开 | 取消只隐藏；同一 `bookUrl` 保留关键词、结果、游标、表格 scrollTop，打开后恢复。 | 关闭 abort 且保留已完成状态，行点击保存滚动位置。 | `aligned + reliability enhancement`；补真实 scrollTop 恢复证据。 |
| 换书判定 | 只要 `bookUrl` 改变便清空关键词、结果和游标。 | key 为 `id || bookUrl`；有 ID 时同 ID 的 URL/来源变化被 ID 掩盖，不会 reset。 | `must-fix`：使用包含 URL 的稳定复合身份；账号 lifecycle reset 继续保留。 |
| 搜索词 | 输入值原样发送；Kotlin 仅拒绝长度为 0，`String.indexOf` 对前后空白也执行精确搜索。 | composable、overlay intent、Reader matcher、现代/legacy Go handler 多处 `trim()`。 | `must-fix`：只拒绝真正空字符串；从输入、请求、响应、intent 到段落定位均不得改写查询。 |
| Enter/续页 | Enter 以 `lastIndex=-1` 替换结果；加载更多从返回 cursor 继续并追加完整的最后扫描章。 | 当前 cursor、完整章、取消和有界多轮语义已经有合同。 | `aligned + bounded adaptation`；不得因本轮 UI 重建回退。 |
| 行点击 | 先保存表格 scrollTop，发完整结果，关闭 Dialog；同章直接定位，跨章 ready 后定位，重复同一行仍再次执行。 | 单调 Pinia intent、同章/跨章/连续模式与 history-neutral replace 已实现。 | `technical-equivalent`；继续保留每次点击一次 intent 和无 history push。 |

## API、数据与安全合同

| 路径/数据 | 固定语义 | 第二轮要求 |
|---|---|---|
| `GET /api/books/:id/search` | 当前 JWT/owned-book 主接口。 | 保留 REST 状态、分页和 additive 完整性字段；`q`/`keyword` 值必须原样进入 exact matcher，只拒绝长度 0。 |
| `GET|POST /api/reader3/searchBookContent` | `url|bookUrl`、`keyword`、`lastIndex`、`size`，逻辑错误保持 HTTP 200 legacy envelope。 | 继续兼容；不得 trim `keyword`。URL 归一化、JWT namespace、size 上限可作为运行时/安全适配保留。 |
| 搜索结果 | 原始正文、章节顺序、章内重叠位置、±20 UTF-16 code units、完整最后扫描章。 | 现有 service 结论继续成立；不改数据库、章节缓存、替换规则或阅读进度。 |
| 远程失败/取消 | 上游断开连接停止后续抓取。 | 继续保留 AbortController、Go request context、超时/大小限制、显式 unavailable/truncated；不得把取消显示为失败。 |
| 多用户 | 上游 namespace；OpenReader JWT + book ID。 | 继续只允许 owner，账号切换后旧结果、旧 toast、旧 intent 均不得提交。 |

## 测试先行门

实施前必须先让当前代码在以下合同上失败：

1. 静态/组件合同：动态 width/top/content-height、标题输入、100/250px 列、直接结果文本、
   无全表 loading mask、无 `el-empty`、左/右页脚和正常模式 gate。
2. composable/store/Reader 合同：
   - 查询 `" 目标 "` 和纯空格必须原样交给 API；只有 `""` 不请求；
   - row intent、段落 occurrence 和旧 URL 冷恢复均保留原始 query；
   - 同 ID 但 `bookUrl` 改变时 abort 并 reset；同 ID/URL 关闭重开保留状态；
   - loading 时已加载行仍可选择，取消/换词/换书继续中止旧请求。
3. Go API：现代和 Reader3 接口对前后空格及纯空格执行 exact search；返回 `query`、
   UTF-16 index 和片段与原始关键词一致。现有大小写、重叠、原始正文、dense cursor、
   unavailable、取消、书源与账号隔离测试全部继续通过。
4. 真实浏览器：
   - 1440×900：1000px width、158px top、400px table；
   - 1024×1366：750px/391px/400px；1366×1024：约 956px/220px/400px；
   - 390×844 与 360×800：全屏、660px/616px table、关闭入口可见、无横向溢出；
   - 打开时输入不自动聚焦；footer 左右关系、加载期间行可点、scrollTop 恢复、换书清空、
     原始空白查询、同/跨章重复跳转、warning、工具层状态和点击不穿透均成立；
   - 简洁模式不保留搜索 Dialog。
5. 发布门：frontend 全量、Go 全量、production build、普通/连续/真实 EPUB Reader、
   新卷和历史卷兼容门通过后，才允许本地构建并发布 Docker。

## 允许差异

- Vue 3、Pinia、Element Plus 根级 Dialog 和单调 selection intent。
- JWT、owned book ID、REST 主接口及 legacy Reader3 adapter。
- AbortController/Go context 取消、远程抓取限制、显式章节失败与截断 warning。
- 有界多轮扫描及非 mini 场景的“搜完全书”增强。
- additive `offset/line/percent/excerpt` 导航数据和旧 query URL 冷启动兼容。

除此之外，当前固定 `880px/520px`、自动聚焦、占位空状态、全表 loading mask、trimmed
query 或既有宽松测试均不构成保留理由。

## 实施顺序

1. 本合同和总矩阵先独立提交、同步 GitHub。
2. 添加会失败的前端静态/状态、Go raw-query 及浏览器几何合同。
3. 只按失败合同重建 overlay、查询传递和 page-type/book identity 生命周期；不扩大 API/数据范围。
4. 完成全量、真实浏览器和卷门后提交推送，并按可验证完整度决定是否发布 Docker。
