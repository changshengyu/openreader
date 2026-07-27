# 书内正文搜索固定上游兼容合同（P2）

状态：**2026-07-27 已按固定上游完成合同、先失败测试、实现与完整回归；待本批 Docker 发布记录。**

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

| 关注点 | 固定上游行为 | OpenReader 实施结果 | 裁决 |
|---|---|---|---|
| 根场景所有权 | `App.vue` 挂载一个独立 `SearchBookContent` 根 Dialog；桌面共享 dialogWidth/top，mini interface 全屏。Reader 只发出打开请求。 | `GlobalOverlayHost` 挂载一个 `OverlayBookContentSearch` 根 Dialog，移动全屏；Reader 不再拥有第二套搜索面板。 | **aligned / technology-equivalent**；不得重新放回 Reader drawer。 |
| 初始状态与换书 | 同一本书关闭再打开保留关键词、结果、游标和上次表格 scrollTop；只有 `bookUrl` 变化才清空。 | 关闭只 abort，不清空；`id/bookUrl` 变化清空；行点击保存 scrollTop，重新打开恢复。 | **aligned + safer cancellation**。 |
| 搜索与续页 | Enter 以 `lastIndex=-1` 新搜；“加载更多”从返回游标继续；服务端扫描完整章节，结果达到 `size` 后停止，最后扫描章节不能只返回一部分后跳过。 | 分页游标已保证完整扫描最后一章；远程初搜最多自动跑 4×10 章，本地有更大有界窗口，并额外提供“搜完全书”。 | **acceptable bounded/network adaptation**：不得跳章或把加载失败伪装成普通无结果；全书按钮是允许增强。 |
| 取消 | 上游关 Dialog 不主动取消 Axios/服务端任务，连接断开后服务端停止。 | AbortController 贯穿浏览器请求和 Go context，关闭/换词停止后续章节。 | **acceptable reliability enhancement**。 |
| 书籍与书源前置条件 | 必须登录、必须按 `bookUrl` 找到书架书；远程书找不到其配置书源时立即返回“未配置书源”。 | 主接口和 legacy 适配器都在章节扫描前校验非零 `SourceID`；缺失书源分别返回 REST `400` 与 legacy HTTP 200 中文错误。 | **aligned + JWT/multi-user adaptation**。 |
| 搜索正文版本 | `searchChapter` 直接搜索 `BookHelp.getContent`；注释明确 `useReplace=false`，不把 Reader 全局替换规则写进搜索输入。 | 章节加载策略已拆分：Reader 显示继续应用替换规则，正文搜索只读取相同缓存/远程来源的原始文本。 | **aligned**。 |
| 精确匹配 | Kotlin `String.indexOf(pattern, start)`，区分大小写；下一次从 `index + 1` 开始，因此允许重叠命中。例如 `aaaa` 搜 `aa` 得到 0、1、2。 | Go 服务和前端 occurrence 定位均恢复精确、区分大小写、从命中位置 `+1` 继续；冲突的模糊测试已替换。 | **aligned**。 |
| 结果片段与字段 | 每个命中返回 `resultCountWithinChapter`、`resultText`、`chapterTitle`、`query`、`chapterIndex`、`queryIndexInResult`、`queryIndexInChapter`；片段左右各最多 20 个 UTF-16 code units。 | 独立 `contentsearch` service 返回字节 offset 供 Go 内部使用，同时按 Java/Kotlin UTF-16 索引生成 legacy 字段与 ±20 片段；新接口保留 additive offset/line/percent。 | **aligned + additive navigation metadata**。 |
| 行点击与浏览器历史 | 上游行点击保存表格 scrollTop、发出 `showSearchContent`、关闭 Dialog；Reader 每次都执行同章定位或跨章加载。它不新增浏览器历史，同一行重复点击也会再次定位。 | Overlay 每次点击发出递增 Reader intent；相同结果可重复消费。同章不改路由，跨章仅 `router.replace` 镜像当前位置，不产生新的返回历史。 | **aligned / Pinia event adaptation**。 |
| 同章/跨章定位 | 同章直接按 `resultCountWithinChapter` 扫描 `.reading-chapter h3,p`；普通跨章等内容 ready 后定位；连续模式先把目标章设为窗口锚点并重建，再定位。 | 同章直接定位且不重新加载；普通跨章只加载目标章；连续模式加载后先重建目标窗口，再按 occurrence 定位并保留 line fallback。 | **aligned**。 |
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
- 两个 query index 与片段边界按 Java/Kotlin `String` 的 UTF-16 code-unit 语义计算。
- `lastIndex=-1` 从第 0 章开始；续页从上一 `lastIndex + 1` 开始。

## 先失败测试（已执行）

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

## 2026-07-27 实施与验证记录

- 先失败证据：前端 6 项失败覆盖缺失 intent、模糊/非重叠定位、`router.push`、同章重载和
  连续窗口；后端 3 项失败覆盖替换后正文、缺失书源和 legacy 索引/片段。
- 自动回归：后端 `go test ./...` 通过；前端 569/569 通过；生产构建通过；`git diff --check`
  通过。
- 真实浏览器：`reader-mobile-contract.mjs` 在 1440×900、390×844、360×800，以及自适应/
  强制移动 iPad 场景通过；验证同章、同一结果重复定位、跨章定位、搜索状态保留、工具层保持
  和零 `history.pushState`。`reader-continuous-contract.mjs` 通过连续跨章窗口回归。
- 后端合同覆盖原始正文与替换规则隔离、大小写/标点/空白精确语义、重叠结果、UTF-16 legacy
  索引、±20 片段、缺失书源前置失败、网络章节 unavailable、取消和显式安全截断。
- 允许差异未扩大：继续保留 JWT/多用户、AbortController、有界扫描、完整性/截断提示与
  “搜完全书”；无数据库、缓存或持久化格式变更。
