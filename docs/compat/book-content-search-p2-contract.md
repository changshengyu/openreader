# 书内正文搜索固定上游兼容合同（P2）

状态：**2026-07-27 已完成只读审计；尚未编写本批失败测试或修改应用代码。**

固定基准为 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
本合同替代“现有搜索测试通过即可视为与上游一致”的结论。

## 权威文件

固定上游：

- `web/src/components/SearchBookContent.vue`
- `web/src/views/Reader.vue#showSearchBookContentDialog/showMatchKeyword`
- `web/src/App.vue` 的 `showSearchBookContentDialog` 根 Dialog 所有权
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#searchBookContent/searchChapter/searchPosition/getResultAndQueryIndex`
- `src/main/java/io/legado/app/data/entities/SearchResult.kt`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`

OpenReader 当前映射：

- `frontend/src/components/overlays/OverlayBookContentSearch.vue`
- `frontend/src/composables/useBookContentSearch.js`
- `frontend/src/composables/useReaderSearchNavigation.js`
- `frontend/src/utils/readerBookSearch.js`
- `frontend/src/stores/overlay.js`
- `frontend/src/views/Reader.vue`
- `frontend/src/api/books.js`
- `backend/api/books.go#searchBookContent/legacySearchBookContent/collectContentMatchesContext`
- `backend/api/content_search_contract_test.go`

## 行为与状态矩阵

| 关注点 | 固定上游行为 | OpenReader `9f9bf27` | 裁决 |
|---|---|---|---|
| 根场景所有权 | `App.vue` 挂载一个独立 `SearchBookContent` 根 Dialog；桌面共享 dialogWidth/top，mini interface 全屏。Reader 只发出打开请求。 | `GlobalOverlayHost` 挂载一个 `OverlayBookContentSearch` 根 Dialog，移动全屏；Reader 不再拥有第二套搜索面板。 | **aligned / technology-equivalent**；不得重新放回 Reader drawer。 |
| 初始状态与换书 | 同一本书关闭再打开保留关键词、结果、游标和上次表格 scrollTop；只有 `bookUrl` 变化才清空。 | 关闭只 abort，不清空；`id/bookUrl` 变化清空；行点击保存 scrollTop，重新打开恢复。 | **aligned + safer cancellation**。 |
| 搜索与续页 | Enter 以 `lastIndex=-1` 新搜；“加载更多”从返回游标继续；服务端扫描完整章节，结果达到 `size` 后停止，最后扫描章节不能只返回一部分后跳过。 | 分页游标已保证完整扫描最后一章；远程初搜最多自动跑 4×10 章，本地有更大有界窗口，并额外提供“搜完全书”。 | **acceptable bounded/network adaptation**：不得跳章或把加载失败伪装成普通无结果；全书按钮是允许增强。 |
| 取消 | 上游关 Dialog 不主动取消 Axios/服务端任务，连接断开后服务端停止。 | AbortController 贯穿浏览器请求和 Go context，关闭/换词停止后续章节。 | **acceptable reliability enhancement**。 |
| 书籍与书源前置条件 | 必须登录、必须按 `bookUrl` 找到书架书；远程书找不到其配置书源时立即返回“未配置书源”。 | ID 路径保持 JWT/用户隔离，但远程 `SourceID` 已失效时逐章记为 unavailable 并返回 200 incomplete；legacy 路径也缺少前置书源校验。 | **must-fix**：新旧接口都应在任何章节抓取前给出明确“未配置书源”；不能产生 N 次无意义章节失败。 |
| 搜索正文版本 | `searchChapter` 直接搜索 `BookHelp.getContent`；注释明确 `useReplace=false`，不把 Reader 全局替换规则写进搜索输入。 | `collectContentMatchesContext` 调用 `loadChapterTextContextResult`，后者对普通文本执行 `applyUserReplaceRules`。 | **错误重构 / must-fix**：搜索必须读取同一缓存/远程章节的原始文本；正文显示仍可继续应用替换规则。 |
| 精确匹配 | Kotlin `String.indexOf(pattern, start)`，区分大小写；下一次从 `index + 1` 开始，因此允许重叠命中。例如 `aaaa` 搜 `aa` 得到 0、1、2。 | Go 先转小写，再忽略空白/标点/符号，还可把多个词按先后顺序模糊命中；直接匹配从 `position + len(keyword)` 继续，丢失重叠命中。前端定位也把这一差异固化为不区分大小写/标点的测试。 | **错误重构 / must-fix**：恢复上游精确、区分大小写、允许重叠的结果集合和 occurrence 序号。现有冲突测试必须重写。 |
| 结果片段与字段 | 每个命中返回 `resultCountWithinChapter`、`resultText`、`chapterTitle`、`query`、`chapterIndex`、`queryIndexInResult`、`queryIndexInChapter`；片段左右各最多 20 个字符。 | 新接口返回 excerpt/offset/line/percent；legacy 虽返回 `resultText`，但缺少两个 queryIndex 字段，片段使用约左 42/右 82 rune。 | **must-fix legacy contract**：legacy 补齐 queryIndex 字段并恢复 ±20 字符片段；新接口可保留 offset/line/percent 作为 Vue/Go 跳转增强，但可见片段应与上游一致。 |
| 行点击与浏览器历史 | 上游行点击保存表格 scrollTop、发出 `showSearchContent`、关闭 Dialog；Reader 每次都执行同章定位或跨章加载。它不新增浏览器历史，同一行重复点击也会再次定位。 | Overlay 关闭后 `router.push` 到带 chapter/line/match/q 的 Reader URL。不同位置同时触发 position/search 两个 watcher；相同 URL 再点是 no-op；每次不同结果都会增加历史层。`useReaderSearchNavigation.jumpToResult` 已存在但未接线。 | **错误重构 / must-fix**：搜索选择应成为一次可重复消费的 Reader intent，直接调用统一跳转；同一结果每次都生效，不得增加历史。兼容 URL 最多用 `replace` 镜像当前位置。 |
| 同章/跨章定位 | 同章直接按 `resultCountWithinChapter` 扫描 `.reading-chapter h3,p`；普通跨章等内容 ready 后定位；连续模式先把目标章设为窗口锚点并重建，再定位。 | 路由 watcher 间接加载；同一结果重复选择无法触发。独立 `jumpToResult` 会加载章节再定位，但目前未处理 Overlay intent，也未由 Reader 使用。 | **must-fix**：同章不重载，跨章只加载一次；连续模式先重建目标窗口；加载 ready 后优先 occurrence，失败才使用 line/percent 安全回退。 |
| 无结果、章节失败与安全上限 | 普通空结果保留空表；单章读取失败会被跳过，界面没有完整性提示，也没有显式匹配上限。 | unavailable/truncated/incomplete 明确显示；单章 2000 命中安全上限可见。 | **acceptable security/reliability enhancement**；提示不得改变成功结果或游标。 |

## API 合同

### OpenReader 主接口

`GET /api/books/:id/search`

- 鉴权与书籍所有权：JWT 用户必须拥有 `:id`。
- 查询：`q`/`keyword`；分页适配参数可保留 `paged`、`lastIndex`、`chapterLimit`、
  `matchLimit`、`scanLimit`、`localFull`。
- 成功响应继续返回 `list`、`lastIndex`、`hasMore`、`total`、`incomplete`、
  `unavailableChapters`、`truncated`。
- 远程书的 `SourceID` 不存在时，在扫描前返回可由前端显示的“未配置书源”错误。
- 搜索输入必须是未应用用户 Reader 替换规则的原始章节正文。
- 结果顺序为章节 `index ASC`，章内为精确命中位置 ASC，允许重叠。

### reader-dev 兼容接口

`GET|POST /api/reader3/searchBookContent`

- 保留 `url|bookUrl`、`keyword`、`lastIndex`、`size`。
- 逻辑错误继续使用 HTTP 200、`isSuccess:false` 和上游中文 `errorMsg`。
- 成功的 `data.list` 必须包含上游 `SearchResult` 可见字段，尤其
  `queryIndexInResult` 与 `queryIndexInChapter`。
- `lastIndex=-1` 从第 0 章开始；续页从上一 `lastIndex + 1` 开始。

## 先失败测试

### Backend

1. 有替换规则把“目标”替换掉时，搜索“目标”仍命中原始章节；搜索替换后的文字不命中。
2. `aaaa` 搜 `aa` 返回三个命中；`目标` 不得命中 `目 标`；`Ab` 不得命中 `ab`。
3. 远程书引用已删除书源时，主接口和 legacy 接口都在零次章节抓取后返回“未配置书源”。
4. legacy 片段左右最多 20 字符，并返回正确的 `queryIndexInResult/queryIndexInChapter`。
5. 保留现有密集章节不丢结果、取消传播、unavailable/truncated 明示及用户隔离测试。

### Frontend

1. Overlay 行点击发出递增 selection intent，不调用 `router.push`。
2. 同一本书同一结果连续选择两次，Reader 执行两次定位；手动离开后可再次回到命中段落。
3. 同章不重新请求章节；普通跨章只加载一次；连续模式先重建目标章窗口再定位。
4. 搜索选择不增加 history；返回手势仍按进入 Reader 前的历史返回。
5. 关闭/换词 abort；同书重开保留结果/scrollTop；换书清空。
6. URL 中既有 `chapter/line/match/q` 仍可冷启动恢复，作为旧链接兼容，不再作为现场行点击的唯一事件通道。

### 真实浏览器

- 1440×900、390×844、360×800：打开搜索、首次搜索、加载更多、同章命中、跨章命中、
  重复点击同一结果、关闭重开恢复 scrollTop。
- 连续滚动模式跨章搜索后正文锚点正确，工具层/搜索 Dialog 状态不穿透。
- 有效替换规则、缺失书源、网络章节失败、请求取消分别验证。
- 搜索结果跳转后一次浏览器返回不得落到上一条搜索结果。

## 实施边界

- 不删除 JWT、多用户隔离、AbortController、有界扫描、完整性提示和安全匹配上限。
- 不通过恢复第二套 Reader 搜索面板解决事件接线；仍由一个 App-level Dialog 与一个 Reader
  跳转控制器协作。
- 不做数据库迁移，不改变章节缓存、替换规则持久化或阅读进度格式。
- 先让本合同测试失败，再修改应用代码；当前已有“标点归一化搜索成功”测试与固定上游冲突，
  必须改为明确的精确匹配合同。
