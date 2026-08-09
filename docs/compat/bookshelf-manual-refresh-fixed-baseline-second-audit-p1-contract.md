# 书架手动刷新固定基准第二轮合同（P1）

状态：`aligned / Docker-published / awaiting-device-verification`
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
盘点日期：2026-08-09

## 范围与结论

本合同只处理用户主动点击首页书架或 Reader 内书架的“刷新”。初始化、窗口重新聚焦、WebSocket
同步和普通的强制重新读取书架不得因此触发远程书源请求。

当前两个可见入口都只重新读取 `/api/books`，没有执行上游 `refresh=1` 所代表的远程目录检查；
已有 `POST /api/books/check-updates` 虽可追加章节，但顺序执行，并把“现有数据库行数”当作远端
目录的稳定前缀。它不能处理目录同数改名/换 URL、重排、缩短或持久化书源变量，也不能保证一本书
的章节和书籍摘要在同一事务内提交。判定为 **must-fix**，不得把当前测试的“重新 GET 即刷新”作为
上游对齐证据。

## 上游权威行为

| 层 | 固定基准证据 | 行为合同 |
|---|---|---|
| 首页入口 | `web/src/views/Index.vue#refreshShelf/loadBookshelf` | 点击“刷新”进入独占 loading，并调用 `loadBookshelf(null, true)`。 |
| Reader 内入口 | `web/src/components/BookShelf.vue#getBookshelf/refreshShelf` | 重复点击被 `refreshLoading` 阻止；请求 `getBookshelf?refresh=1`，成功后替换整份 shelf 并重新定位当前书。 |
| 前端请求 | `web/src/App.vue#loadBookShelf` | `refresh=true` 映射为 `GET /getBookshelf?refresh=1`；返回完整书架而不是只返回新增章节。 |
| 控制器 | `BookController.kt#getBookshelf` | GET/POST 均读取 `refresh`；鉴权后调用 `getBookShelfBooks(refresh > 0, namespace)`。 |
| 检查范围 | `BookController.kt#getBookShelfBooks` | 只检查当前 namespace 中非本地且 `canUpdate` 的书；每轮远程检查并发上限为 16。 |
| 单书失败 | 同上 | 找不到书源或某书抓取/解析失败只跳过该书，不阻断其余书和完整书架响应。 |
| 目录持久化 | `getLocalChapterList(..., refresh=true)`、`saveShelfBookLatestChapter` | 成功抓取后以最新目录替换该书的活动目录缓存，并更新最后章节与总章数。 |
| 更新时间 | `getBookShelfBooks`、`saveShelfBookLatestChapter` | 仅当新目录章数大于此前 `totalChapterNum` 时推进 `lastCheckTime` 并记录新增数；同数、缩短和失败不推进。 |

reader-dev 使用 namespace JSON 文件而非 SQLite 事务。OpenReader 可以保留独立 REST 路径、JWT、
SQLite 和 WebSocket，但必须达到相同的可见动作、单书失败隔离和目录替换结果，并补上当前运行时需要的
事务与并发陈旧结果保护。

## 当前映射与差异矩阵

| 项目 | 当前文件/行为 | 裁决 |
|---|---|---|
| 首页刷新 | `frontend/src/views/Home.vue#refreshShelf` 并发重载分类、BookGroup 和 `/books`。 | `must-fix`：先执行一次远程检查，再以网络权威书架收敛；分类/BookGroup 刷新仍可保留。 |
| Reader 内刷新 | `frontend/src/composables/useReaderShelf.js#refresh` 只重载 `/books`。 | `must-fix`：与首页共用同一个 store 动作，完成后继续定位当前书。 |
| 已有客户端 | `frontend/src/api/books.js#checkBookUpdates` 已定义但无调用者。 | `must-use`：保留稳定路径，不另造 refresh API。 |
| API | `backend/api/books.go#checkUpdates` 总是 `200`，仅返回 `newChapters/books`；底层书架读取失败也无法上报。 | `must-fix`：补充 checked/updated/failed/replacedBookIds，并区分顶层失败与单书失败。 |
| 调度器 | `backend/services/scheduler` 顺序检查，忽略 `Book.Variable`，只按行数追加，并逐行写后 `Save(&book)`。 | `must-fix`：有界并发抓取、变量语义、目录差异分类、单书事务和显式列更新。 |
| 目录变化 | 同数、重排、缩短和前缀内 URL/标题变化均被当作“无更新”。 | `must-fix`：成功抓取的远端目录是该书最新权威目录。 |
| 失败原子性 | 新章节逐行提交；中途失败会留下部分章节但旧书摘要。 | `must-fix`：一本书的章节、摘要、变量、进度/书签重绑必须全部提交或全部不变。 |
| 并发陈旧结果 | 抓取期间修改书源、URL、canUpdate 或删除书，旧结果仍可回写。 | `must-fix`：提交前复核 owner、ID、URL、sourceID 和 canUpdate 快照。 |
| 初始/前台刷新 | `AppLayout` 的后台 freshness 只重取书架。 | `must-preserve`：不得自动发起昂贵的远程目录检查。 |

## OpenReader API 合同

### `POST /api/books/check-updates`

- **鉴权**：必须使用现有 JWT；只检查当前用户拥有、`sourceId > 0`、`canUpdate=true` 的远程书。
- **请求**：无必填字段；空 body 或 `{}` 均有效。保留已部署客户端发送多余 JSON 字段时忽略的兼容行为。
- **成功**：即使部分书失败也返回 `200`：

```json
{
  "checked": 3,
  "updated": 1,
  "failed": 1,
  "newChapters": 2,
  "replacedBookIds": [12],
  "books": [{ "id": 12, "chapterCount": 8, "lastChapter": "第八章" }]
}
```

  - `checked` 是本轮实际取得快照并尝试检查的当前用户候选书数；
  - `updated` 是发生持久化目录、摘要或变量变化的书数；
  - `failed` 是远程获取、规则解析、书源缺失、陈旧快照或单书事务失败的书数；
  - `newChapters` 只累计远端章数相对抓取前 `Book.chapterCount` 的正增长；
  - `replacedBookIds` 只列出发生非前缀目录替换、需要失效旧浏览器章节缓存的书；
  - `books` 返回所有成功持久化变化后的完整 shelf projection，顺序稳定按书 ID；无变化时为空数组。
- **事件**：所有单书事务结束后至多广播一次当前用户 `bookshelf_update`，payload 为成功变化的
  shelf items；零持久化变化不广播。事件不得先于数据库提交。
- **顶层错误**：无法读取候选书或无法生成最终 shelf projection 时返回 `500 {"error":"检查书籍更新失败"}`；
  JWT 失败沿用中间件 `401`。单书远程/解析错误不得升级为顶层 `500`。
- **错误安全**：响应、事件和日志中的用户可见字段不得包含书源规则、凭证、完整远端响应、JWT、
  代理凭证或主机绝对路径。部分失败只向前端提供数量，服务端可记录脱敏后的 book ID 与错误类别。

新增字段均为向后兼容；已部署客户端继续读取 `newChapters/books` 不受影响。

## 单书抓取与提交合同

1. 在开始远程 I/O 前读取当前用户候选书和其 owned source；远程抓取最多同时进行 16 本。SQLite
   写阶段不得无界并发，可串行应用每本结果以避免 WAL 锁争用。
2. TOC-only 抓取必须从持久化 `Book.Variable` 启动规则变量状态，返回新的 book variable 和每章
   variable；不得为了刷新目录而同时覆盖书名、作者、封面、简介等 BookInfo 字段。
3. 对远端章节做与正常导入相同的 URL 解析和标题规范化；空目录视为该书失败，保留现有目录。
4. 提交前在事务内重新读取这本书并复核 `userID/id/sourceID/url/canUpdate`。任一值已改变或书已删除，
   本次结果按陈旧失败丢弃，不得复活或覆盖新状态。
5. 按 `(index,title,url,variable)` 比较数据库目录：
   - 完全一致：不重写章节行或缓存；只在摘要/variable 确实陈旧时显式更新对应列；
   - 旧目录是新目录的精确前缀：事务内只追加尾部，保留原章节 ID 和缓存；
   - 改名、换 URL/variable、重排、插入、删除、缩短或同数变化：事务内完整替换活动章节行，
     按现有 catalogue replacement 规则重绑 progress/bookmarks；不存在的新索引清除陈旧 chapterId，
     但保留可恢复的 index/offset 记录。
6. 同一事务显式更新 `lastChapter`、`chapterCount` 和变量列；不得使用包含抓取前陈旧字段的全行
   `Save`。`lastCheckTime` 仅在远端章数大于事务开始时持久化的 `chapterCount` 时推进。
7. 单书事务失败时，该书章节、书籍摘要、变量、progress/bookmarks 全部回滚；其它书的成功事务不回滚。
8. 非前缀替换提交后才删除已无引用的服务端派生缓存；原始导入、LocalStore/WebDAV 文件和其它用户
   文件不得成为清理目标。前端收到 `replacedBookIds` 后清除对应用户范围内的浏览器章节缓存。

## 前端状态与可见行为合同

1. Pinia 书架 store 提供唯一的手动刷新动作并持有共享 in-flight promise；首页和 Reader 内书架同时/
   连续点击不得产生重复 `/books/check-updates`。
2. 动作顺序固定为：远程检查 → 失效必要浏览器章节缓存 → `loadBooks({force:true,all:true,
   settleProgress:true})`。无论远程检查是部分失败还是顶层失败，均尝试一次网络权威书架重载；若两者
   都失败，优先报告远程检查错误并保留当前可用 shelf。
3. 首页在该动作之外继续刷新分类和 BookGroup。成功且 `failed=0` 显示“书架已刷新”；`failed>0`
   显示“书架已刷新，N 本书检查失败”；顶层失败显示稳定错误。Reader 内书架通过现有 `onError`
   使用相同语义，并在成功重载后重新定位当前书。
4. loading 在完整动作结束前保持；重复入口不另起请求。面板、当前书、筛选和阅读进度不因刷新关闭/
   重置；progress 的 pending/CAS 合同继续由 `bookshelf-refresh-progress-p1-contract.md` 约束。
5. 初次进入、窗口 focus/visibility、WebSocket 事件、普通 `loadBooks(force)` 和分类切换只读取书架，
   不调用 `/books/check-updates`。

## 数据与迁移

- 不新增、删除或重命名 SQLite 列、索引和表。
- 不改变 `data/`、`cache/`、`library/`、备份、WebDAV 或 portable backup 格式。
- 允许在成功的目录替换后清理当前用户已无引用的派生章节缓存；这是已有 cache replacement 合同，
  不是数据迁移。
- 旧卷首次手动刷新必须保持现有 Book、Chapter、Progress、Bookmark ID/索引和文件可恢复性；失败不得
  让旧目录从 Reader 中消失。

## 实施前红灯测试

1. Go/API：当前用户范围、16 并发上限、一个失败源不阻断另一本成功、全失败仍返回完整统计。
2. Go/engine：TOC-only 刷新消费并持久化 book/chapter variable，但不覆盖 BookInfo。
3. Go/事务：在第二条章节 INSERT 注入失败，断言没有部分章节、摘要、变量或 progress/bookmark 写入。
4. Go/目录差异：精确相同、前缀增长、同数改名/URL、重排、缩短分别进入正确路径；增长时间语义保持。
5. Go/并发：抓取期间删除书或改变 URL/source/canUpdate，旧结果不能提交。
6. Go/缓存：非前缀替换只在提交后清理无引用派生缓存；回滚、另一用户和原始文件保持。
7. Frontend：两个可见入口调用同一 store 动作；请求顺序、共享去重、部分警告、错误后权威重载和
   replacedBookIds 浏览器缓存失效均有契约。
8. 真实浏览器：1440×900、390×844、360×800 下，首页与 Reader 内刷新每次只发一个 check；
   最新章节/章数无需整页 reload 即可见；一个失败源不阻止成功书更新，且无控制台/API 未处理错误。

## 与既有合同的关系

- 本合同只取代 `bookshelf-refresh-progress-p1-contract.md` 中把“可见刷新 = 强制 GET /books”视为
  完整对齐的部分；pending progress、CAS、跨客户端收敛继续有效。
- `bookshelf-last-check-time-p1-contract.md` 的增长才推进时间规则保持不变。
- `bookshelf-network-first-sync-p2-contract.md` 的网络优先、账号 scope 与迟到响应门禁保持不变。
- 显式单书 `POST /api/books/:id/refresh` 继续负责完整 BookInfo + TOC 刷新；手动书架刷新只检查 TOC，
  不得借机改变书籍元数据。

## 实施与回归结果（2026-08-09）

- `services/scheduler` 现先按固定快照并发抓取（上限 16），再串行执行每书 SQLite 事务；TOC-only
  规则运行从持久化 `Book.Variable` 开始，不覆盖 BookInfo。
- 完全一致目录不重写；精确前缀只追加并保留旧章节 ID、缓存、进度和书签引用；改名、URL/变量变化、
  重排或缩短统一复用 catalogue replacement，事务内重绑引用，提交后才清理无引用派生缓存。
- `POST /api/books/check-updates` 已返回安全统计、稳定 shelf projection 和 `replacedBookIds`；一本书失败
  只增加 `failed`，候选读取/最终投影失败才返回稳定 `500`。所有成功事务结束后至多广播一次。
- 首页和 Reader 内书架共用 Pinia `refreshFromSources()`；共享 in-flight，顺序为 check → 浏览器缓存失效
  → 等待 pending progress 后权威重载。检查失败仍尝试重载；初始化/focus/WebSocket 不触发远程检查。
- 回归通过：Go 全量、`go vet`、受影响 scheduler/API race、frontend 713/713、production build、
  `git diff --check`。完整 Reader 浏览器合同通过桌面、390×844、360×800、自适应/强制手机 iPad，
  并再次精确得到 Reader 内书架 350/320px。独立 Home+Reader 真实 Go 刷新脚本本轮因 Codex 外部执行
  额度拒绝未能重跑；其新增请求计数已由前端运行时测试覆盖，发布记录保留这一限制。

本实现不迁移 SQLite 或 mounted paths。

## Docker 发布结果（2026-08-09）

- 实现提交 `43635a1c4a89c0c660e0f47b6e32df08b5ad745f` 已在发布前推送 `main`。
- 候选镜像通过 fresh volume 的 portable v1/v2 assets、cross-user、restart，以及 historical volume
  的 TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation 和 portable restore。
- 双架构镜像完全由本机 OrbStack 构建；第一次 GHCR 上传在第 11/23 个 blob 遇到网络中断，复用同一
  OCI 归档重试后只补传缺失 blob 并成功提交标签，没有改用云端构建。
- 发布标签：`ghcr.io/changshengyu/openreader:43635a1` 与
  `ghcr.io/changshengyu/openreader:latest`。
- OCI index：`sha256:0f75a0434d209af901cde81f86127f8e62fa78d6cb3610d6c10ef2e0863053c0`；
  amd64 manifest：`sha256:9186bbdecaebda5dc7cc7dca47f9c4c612d86d7236e0b77f1ca66027bfdbb761`；
  arm64 manifest：`sha256:95ff4645eb28cd853d5888363f54e0a28d86784ba24c82215c29653cf967644c`。

本批允许差异只有 OpenReader 的 JWT/SQLite/Pinia/WebSocket 技术栈映射；可见“刷新”动作、单书失败
隔离、目录替换和整份书架收敛均按固定上游合同实现。尚未完成项仍是独立 Home+Reader 真实 Go 浏览器
脚本的本轮重跑与真实设备验收；它们不改变已经通过的 API、事务、前端运行时和新旧卷证据。
