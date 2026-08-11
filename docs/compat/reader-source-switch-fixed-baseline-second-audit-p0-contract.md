# Reader 换源固定基准第二轮合同（P0）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前审查基线：`OpenReader@d2f7bfe`  
审查日期：2026-08-11

## 1. 权威文件

reader-dev：

- `web/src/components/BookSource.vue`
- `web/src/views/Reader.vue`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt`
- `src/main/java/io/legado/app/data/entities/Book.kt#toSearchBook`

OpenReader：

- `frontend/src/components/reader/SourceSwitchPanel.vue`
- `frontend/src/composables/useBookSourceCandidates.js`
- `frontend/src/composables/useReaderCatalogActions.js`
- `frontend/src/views/Reader.vue`
- `backend/api/books.go#listBookSourceCandidates/changeBookSource`
- `backend/services/booksources`、`backend/services/sourcefailure`

## 2. 上游状态机

### 打开面板

`BookSource.vue` 的 `visible` 从 false 变 true 时调用 `getBookSource(false)`：

1. `/getAvailableBookSource {url,refresh:0}` 只读取该书已保存的候选列表。
2. 加入书架时，`saveBook()` 已用 `book.toSearchBook()` 为候选列表播种当前来源。
3. 打开动作不扫描全部活动书源，不依赖网络速度，不改变阅读器工具层显示状态。
4. 返回后定位当前 `bookUrl`，标题显示 `来源(N)`。

### 刷新

点击“刷新”调用 `/getAvailableBookSource {url,refresh:1}`：只重新验证已保存候选对应的书源。每个
远程源执行 `searchBookWithSource(... accurate=true)`，结果必须同时满足书名和作者精确相等；本地
来源直接保留。刷新结果替换原候选缓存，不从未扫描的书源补充结果。

### 加载更多

点击“加载更多”调用 `searchBookSourceSSE`（非 SSE 是兼容备选）：

1. 按当前书源分组和该分组自己的 `lastIndex` 从后续活动书源继续扫描。
2. `searchBookWithSource` 默认 `accurate=true`，只接收 `name == book.name && author == book.author`。
3. 结果按 `bookUrl` 去重并增量合并到可见列表及每书候选缓存。
4. 扫描达到目标数量、源尾、连接取消或有界循环上限即结束；无新增时提示“没有更多啦”。
5. 切换分组使用独立 cursor，不清除已经显示的候选。

### 选择来源

选择候选先保证书已在书架，再调用 `setBookSource`。成功后：

- 阅读书籍和书架记录切到新 URL/来源并重载目录；
- Reader 关闭当前来源 Popover，工具层保持显示；
- `BookSource` 组件中的候选数组本身不因一次新的全网搜索被替换，下次打开仍可立即显示缓存并定位
  新的当前来源。

## 3. 实施前结构偏差

当前 `GET /api/books/:id/source-candidates` 同时承担打开、刷新和加载更多：每次调用都会把活动书源
按 offset 切片并执行远程搜索，最多接收每源前三条任意结果。因此已确认：

| 场景 | 当前行为 | 裁决 |
|---|---|---|
| 首次打开 | 立即联网搜索，慢源会让面板初次打开变慢。 | **must-fix** |
| 候选过滤 | 接受标题或作者不相等的结果。 | **must-fix** |
| 候选持久化 | 没有每用户/每书缓存，只有临时 Vue 数组。 | **must-fix** |
| 刷新 | 与首次打开一样重搜第一批活动源，而非只验证已保存候选。 | **must-fix** |
| 加载更多 | 只扫描下一页源并合并，cursor 基础可复用，但语义没有和已保存候选联动。 | **must-fix** |
| 换源后 | `applySourceChange()` 再次调用 `refreshSourceCandidates()`，触发额外远程搜索后关闭面板。 | **must-fix** |
| owner/failure/fetch | 当前用户书籍/书源过滤、失败缓存、安全 fetch、超时、取消和有界并发已存在。 | **必须保留的技术增强** |

## 4. OpenReader 最终 API 合同

保留现有路径，使用显式 `mode` 区分三种动作；不新增云端任务或无界 SSE：

### `GET /api/books/:id/source-candidates?mode=available`

- `mode` 缺失时等同 `available`，使面板打开默认无网络。
- 只读取当前用户、当前书的候选缓存；如果历史书尚无缓存，事务播种当前书自身后返回。
- 返回当前候选投影，按稳定保存顺序，按 `bookUrl` 去重，并投影 `current` 与安全封面 capability。
- 不因候选对应书源已禁用/删除而泄漏其他用户数据；损坏行可忽略并在后续刷新清理。

### `GET /api/books/:id/source-candidates?mode=refresh`

- 仅重新验证缓存里引用的候选来源；本地当前项直接保留。
- 远程搜索结果只保留标题和作者都与当前书精确相等的条目。
- 完成后以验证结果替换候选缓存；当前来源即使书源临时失败也必须通过当前书快照保留，避免列表无法
  指示当前项。
- 单源错误记录到现有 user-scoped failure cache；部分失败不把其他成功结果回滚。

### `GET /api/books/:id/source-candidates?mode=search`

- 接受 `group`、`offset`、`limit`、`paged=1`；offset 表示该分组活动源中的下一扫描位置。
- 从 offset 起以当前受限并发、单请求 timeout、失败源过滤和请求取消机制扫描；只收精确书名+作者。
- 在达到目标候选数、源尾或有界轮次后返回 `nextOffset/hasMore/searched/matched/failed/empty`。
- 新结果按 `bookUrl` merge/upsert 到候选缓存；响应列表只包含本次新增/更新结果，前端负责与现有
  列表合并。
- 分组 cursor 只保存在前端会话；候选数据持久保存，不把 cursor 当用户数据备份。

非法 `mode` 返回 400。所有模式先验证当前用户拥有该书；不得接受客户端传入任意 title/author
绕过精确身份。

## 5. 数据合同

采用新增的派生候选表，而不是把 JSON 塞进 `Book` 或浏览器缓存。每行至少包含：

- `user_id`、`book_id`、`source_id`（本地来源可为 0）；
- `source_url`/来源稳定标识、`source_name`、`source_group`；
- `title`、`author`、`book_url`、`cover_url`、`intro`、`kind`、`word_count`、`latest_chapter_title`、
  `type`、`respond_time`；
- 稳定 `sort_order` 与时间字段；唯一身份为当前用户/当前书内的 `book_url`。

约束：

1. 新表是加法迁移，旧 SQLite 卷不得重建、删除或改写既有表。
2. 创建远程书时在同一业务事务中播种当前候选；历史书在第一次 available 时幂等补种。
3. 删除书必须删除其候选；删除用户必须删除其全部候选。删除/失效书源不得跨用户删除共享快照；
   refresh/search 根据当前 namespace 处理陈旧候选。
4. 换源事务成功后更新/插入被选候选并标记新的当前投影；候选本身不需要额外“current”持久字段，
   `book.source_id + book.url` 是权威。
5. 候选属于可再生派生数据，不进入普通或 portable backup；恢复后的书首次打开按当前书幂等播种。
6. 每书候选数必须有上限，超限按稳定最旧/最低顺序清理，但不得清理当前来源。

以 Book ID 作为缓存归属而不是上游 `name_author` 文件路径，是避免改名碰撞和路径问题的允许增强；
用户可见打开/刷新/加载更多流程不得因此变化。

## 6. 前端状态合同

`useBookSourceCandidates` 必须显式暴露：

- `open()`/`ensure()`：available；
- `refresh()`：refresh；
- `loadMore()`：search，并维护每分组 cursor；
- `applyChangedSource(candidate)`：只更新列表的 current 投影和被选项数据，不发网络请求。

打开、刷新、加载更多应有独立 busy 状态，避免刷新时把“加载更多”误显示为同一操作。切分组不得清空
已显示候选；只切换该组后续搜索 cursor。面板结构继续保持上游 `来源(N) / 分组 / 刷新 / 加载更多`
和来源名称、耗时、最新章节两行。面板点击不穿透正文，工具层保持显示；换源成功后关闭来源面板并
重载目录，这是固定上游行为。

## 7. 安全与并发合同

- 保留 `FindActive/ListActiveByIDs` 当前用户 namespace 过滤，不允许用候选 `source_id` 访问别人的源。
- 保留共享 fetcher 的 HTTP(S)、userinfo、重定向、body、DNS/private-network、credential 和 proxy
  边界；本合同不放宽任何远程抓取安全策略。
- 保留请求 context 取消、每源 timeout、失败缓存和有界并发。上游并发数字不是必须照搬；结果顺序
  必须按活动源稳定顺序合并。
- 缓存写必须在远程结果完全验证并完成 owner 投影后进行；失败请求不能以空列表覆盖已有缓存，
  `refresh` 的明确部分成功结果除外。
- 候选字段必须受长度和每书行数限制，不能让恶意书源制造无界 SQLite 增长。

## 8. 测试先行顺序

1. Go API 失败测试：available 不触发远程请求、历史书播种当前项、两用户隔离。
2. Go API 失败测试：search 拒绝标题相同作者不同/作者相同标题不同，只缓存双精确匹配；cursor、
   group、失败源、取消、稳定顺序和上限。
3. Go API 失败测试：refresh 只访问已缓存来源、替换成功结果、部分失败保留当前项；换源后候选立即
   指向新 URL 且不额外搜索。
4. 数据测试：fresh migration、historical volume、书/用户删除清理、备份不携带派生候选、恢复后
   幂等播种。
5. 前端失败测试：打开只调用 available；刷新和加载更多 mode 分离；每组 cursor；换源后没有
   `refreshSourceCandidates()`，工具层与关闭状态符合上游。
6. 真实浏览器在 1440×900、390×844、360×800、1024×1366 验证即时打开、三按钮状态、当前项
   定位、无候选、部分失败、换源关闭面板和正文位置不跳变。
7. Go/full/race/vet、frontend 全量、production build、fresh/historical volume 后才可发布 Docker。

## 9. 实施与回归结果（2026-08-11）

- 后端保留 `GET /api/books/:id/source-candidates`，以 `available`、`refresh`、`search` 三种 mode
  分离无网络读取、缓存来源复验和分组增量搜索；非法 mode 返回 400，书籍身份始终从当前 owner 行读取。
- 新增 `book_source_candidates` 派生表；远程建书和成功换源在原事务中播种当前快照，历史书首次
  available 幂等播种，书/用户删除清理，书源 copy-on-write 只重映射目标用户。每书最多 200 行，
  字段有界，稳定清理最旧非当前项，当前项不得被裁剪。
- search 使用精确书名+作者、每组独立 source cursor、4 并发、每源 12 秒 timeout、单批最多扫描
  40 源，并沿用 user-scoped failure cache、请求取消和共享安全 fetcher；refresh 只解析缓存 source ID，
  部分失败仍保留当前书快照。
- 前端打开、刷新、加载更多使用独立 busy 状态；换组不清空候选，换源后只本地更新 current 投影，
  不再触发额外候选搜索。换源事务清理旧正文搜索 owner、关闭面板，并以段落和视口偏移锚点恢复
  连续阅读位置，offset 继续作为翻页/非连续模式回退。
- 数据/API/生命周期、上限、取消、精确匹配、稳定 cursor、用户隔离、建书/换源事务、COW、删除和
  backup 排除测试通过；Go 全量、换源 API/service race、`go vet`、frontend 734/734 和 production
  build 通过。
- 真实 Chromium 在 1440×900、390×844、360×800、1024×1366 全部通过 available/refresh/search、
  当前项、独立按钮状态、部分失败、空态、换源关闭、无额外候选请求和正文段落位置不跳变。

## 10. Docker 发布结果（2026-08-11）

实现提交 `a2ecc175985677b83514ca3d3d005b59dc058a05` 已先推送 GitHub，再仅由本机 OrbStack 构建和上传：

- `ghcr.io/changshengyu/openreader:a2ecc17`
- `ghcr.io/changshengyu/openreader:latest`
- OCI index：`sha256:311ca87a75e4b77c49c95c033c80ac4a6d7baa1598092b630ac5002ce5493754`
- amd64：`sha256:e86f4fda61b79d211d2865348afd0289ca2e8718fd8072c734c96a4fff584177`
- arm64：`sha256:3e6b87a29702465d1c6fdd206fde1cbbe796619d4ef792e7a03aade761fd55f1`

本机 arm64 候选先通过 fresh volume 的 portable-v1、portable-v2 assets、cross-user 和 restart。
historical volume 首次运行在创建旧卷后出现一次无上下文 404，未记为通过；原样顺序重跑后完整通过
TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation 和 portable backup/restore。远端 commit tag
与 `latest` 已回读为同一上述双架构 index。

当前状态：**aligned / Docker-published / awaiting-device-verification**。
