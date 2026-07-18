# P2 阅读进度 API、并发与 WebDAV 合同

状态：2026-07-18 已完成固定上游审查、失败测试、应用实现、全量回归、历史卷与
本地 Docker 发布门禁，并以 `9f19d21` 发布。本文件只覆盖已加入书架书籍的阅读进度；
临时远程阅读不得创建 `ReadingProgress`、书架行或 WebDAV 文件。

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

## 权威文件

- 上游前端：`web/src/views/Reader.vue#saveBookProgress`、`deactivated`、
  `scrollHandler`，以及 `web/src/plugins/vuex.js` 的书架进度投影。
- 上游后端：`BookController.kt#saveBookProgress`、`saveShelfBookProgress`、
  `saveBookProgressToWebdav`、`syncBookProgressFromWebdav`；数据字段来自
  `io/legado/app/data/entities/Book.kt`。
- 当前后端：`backend/api/progress.go`、`backend/models/models.go`、
  `backend/api/books.go`、`backend/api/webdav.go`、备份服务和同步 Hub。
- 当前前端：`stores/reader.js`、`useReaderProgressPersistence.js`、
  `useReaderExternalUpdates.js`、`useSync.js` 及书架进度投影。

## 上游状态转换与当前差距

| 边界 | 固定上游行为 | 当前 OpenReader | 判定 |
|---|---|---|---|
| 保存触发 | Reader 离开时保存；连续阅读跨入另一章时保存当前目录索引。章节内位置另存浏览器缓存。 | 页面内节流保存章节、字符 offset、整书/本章比例；离开时保存，跨客户端经 WebSocket 收敛。 | `technical-stack-equivalent + allowed enhancement`；保留更精确位置。 |
| 书籍与章节身份 | 请求用书籍 URL 和目录 index；服务端确认书已在当前用户书架、加载目录，再从 `chapterList[index]` 取得规范标题。 | JWT 隔离书籍，但直接相信客户端 `chapterId`、`chapterIndex`、`chapterTitle`；负 index/offset、外书 chapter ID 或不存在的 index 可进入进度行。 | `must-fix`：按当前用户书籍的目录规范化，不能持久化悬空/错书章节身份。 |
| 可见书架状态 | 更新 `durChapterIndex/title/time`，书架未读数、已读标题和最近阅读排序随后使用该状态。 | 独立 `ReadingProgress` 嵌入书架投影，`shelfOrderAt` 使用进度时间。 | `technical-stack-equivalent`；保留独立表和精确位置字段。 |
| 并发写入 | 上游单用户文件模型按到达顺序编辑书架，没有 OpenReader 的多浏览器 JWT/SQLite 合同。 | 已有 `baseUpdatedAt` 和冲突响应，但当前先 SELECT、后无条件 upsert；两个同基线请求可同时通过并各自广播。 | `must-fix runtime adaptation`：以数据库条件更新/唯一创建实现原子 CAS，只有提交赢家广播。 |
| 冲突响应 | 无 REST 并发版本。 | 旧客户端依赖成功体；冲突使用 `200`、现有进度和 `X-OpenReader-Progress-Conflict: 1`。 | `acceptable deployed API`；保持形状，不改为破坏性的 `409`。 |
| WebDAV 进度 | 仅当用户 WebDAV 根下已存在 `bookProgress/` 或 `legado/bookProgress/` 时，在书架保存后写入书名/作者对应 JSON；不会为了保存进度自动创建功能目录。 | 能从备份 ZIP 的 `bookProgress/` 恢复，也会在整包备份中导出 `readingProgress.json`，但普通阅读保存不写现存目录。 | `must-fix`：恢复既有目录的逐书进度镜像；多用户私有根和权限是允许且必须的收紧。 |
| 页面退出请求 | 上游只发一次保存请求。 | `background` 分支先发 `fetch(...keepalive)`，随后又调用普通 `flush()`，同一快照可能并发写两次。 | `must-fix`：keepalive 成功排队时不得再发送普通请求；无法排队时才回退普通保存。 |
| 远端进度收敛 | 上游没有 OpenReader 多浏览器 WebSocket；读取服务器进度本身不得产生新的用户阅读动作。 | 旧实现收到进度后先 `router.replace`，路由 watcher 以 `saveAfterLoad:true` 加载一次，随后外部同步又以 `false` 加载一次，造成每个在线 Reader 回声保存。 | `must-fix runtime adaptation`：服务器确认的位置只恢复一次并标记已保存；不得产生额外 PUT/广播。 |
| 临时阅读 | 未入书架会返回“书籍未加入书架”，不保存。 | 临时远程 Reader 不调用书架进度 API。 | `aligned`；保持无持久化。 |

## OpenReader API 合同

### `GET /api/progress/:bookID`

- JWT 必需；只读当前用户书架书籍。非法正整数 ID 为 `400`，外书/不存在书籍为
  `404`，无进度为 `200 {}`，有进度为 `200 ReadingProgress`。
- 返回行必须属于当前用户和该书；不得因同 ID 的其他用户行泄漏状态。

### `PUT /api/progress`

请求保持现有字段：

```json
{
  "bookId": 1,
  "chapterId": 10,
  "chapterIndex": 3,
  "offset": 128,
  "percent": 0.24,
  "chapterPercent": 0.67,
  "chapterTitle": "客户端显示值",
  "mode": "scroll",
  "baseUpdatedAt": "2026-07-18T00:00:00Z",
  "clientUpdatedAt": "2026-07-18T00:00:01Z",
  "clientId": "session-id"
}
```

- `bookId` 必须是当前用户书架书籍；`chapterIndex >= 0`、`offset >= 0`，且该 index
  必须存在于当前目录。`percent` / `chapterPercent` 继续兼容性夹紧到 `[0,1]`。
- 服务端按 `(book_id,index)` 读取规范章节。`chapterId=0` 时补入规范 ID；非零 ID
  必须就是该规范章节，否则 `400 {error}`。持久化标题始终使用服务端目录标题，
  不相信客户端标题。该规则同时适用于 TXT、EPUB、UMD、CBZ、音频和远程书。
- 对已有行，以读取到的 `id + updated_at` 作为 CAS 条件。条件更新影响一行才算成功；
  零行意味着另一写入已提交，重新读取赢家并返回现有冲突响应。首次创建若命中
  `(user_id,book_id)` 唯一键，也重新读取赢家并按冲突返回。
- `baseUpdatedAt`/`clientUpdatedAt` 的现有兼容判断保留；现代客户端必须提交基线。
  无基线旧客户端仍可工作，但不得绕过用户/章节校验。服务端 `updatedAt` 是唯一持久
  排序时间，不将客户端时钟直接写入数据库。
- 成功后才广播一个当前用户 `progress_update`；payload 保留 `clientId` 和含最新进度的
  `book` 书架投影。CAS 失败者、校验失败和数据库失败均不得广播。
- 处理失败继续使用现有顶层 `{error}` 兼容形状；外书表现为 `404`，非法 payload/目录
  身份为 `400`，数据库不可用为 `500`。

## WebDAV 镜像与数据边界

- 不新增 SQLite 字段、不迁移 `ReadingProgress`，保留唯一索引
  `(user_id,book_id)`、现有备份 JSON 和旧卷。
- 管理员沿用历史 `data/webdav` 根；普通用户只能使用
  `data/webdav/users/<safe-username>`。没有 WebDAV 权限时跳过镜像，不能让进度保存
  返回 `403`。
- 仅在用户根下已经存在目录时写入，优先 `bookProgress/`，否则
  `legado/bookProgress/`；不因第一次阅读自动创建目录。
- 文件名保留上游“书名_作者.json”语义，但必须经过 `SafeFilename`；空名称回退稳定的
  `book-<id>.json`。写入采用同目录临时文件、close、原子 rename，禁止跟随目录外
  symlink 或写到用户根之外。
- JSON 至少包含 `name`、`author`、`bookUrl`、`durChapterIndex`、
  `durChapterPos`、`durChapterTime`、`durChapterTitle`。OpenReader 将精确 offset 写入
  `durChapterPos` 是兼容增强；现有恢复器已支持单对象和数组两种形状。
- 数据库提交是主事实。提交后镜像失败不得回滚或伪装数据库未保存；响应可用不含路径的
  诊断 header 标记失败。不得在 API body、日志或 WebSocket 中暴露宿主机路径。

## 前端状态合同

- 正常保存继续节流、合并最新快照，并按认证 operation scope 阻止旧账号响应落入新账号。
- `background` 保存只排队一次 keepalive；排队失败才调用普通保存。无论网络结果如何，
  本地 scoped pending 快照必须保留，供下次打开与服务器协调。
- 收到冲突时先采用服务器赢家；只有本地 pending 快照确实更新且内容不同，才用新基线
  重试一次。重试仍受 operation scope 约束，不允许跨账号或无限循环。
- 其他客户端的 committed WebSocket 进度更新当前书时，Reader 只在不处于恢复/保存临界区
  时跳转；同 clientId 回声忽略。外部位置更新路由时必须一次性抑制普通路由 watcher 的
  `saveAfterLoad`，由外部恢复事务加载一次并 `markProgressSaved`；下一次真正的用户路由
  变化恢复正常保存。书架投影必须与 Reader 使用同一赢家。

## 必须先写的失败测试

1. Go CAS：从同一数据库快照产生两个候选；第一次条件更新成功，第二次影响零行并读取
   第一个赢家。API 只对赢家广播一次。
2. Go 章节规范化：省略 chapter ID 可由 index 补齐并覆盖伪造标题；外书 chapter ID、
   ID/index 不匹配、负 index、负 offset 和不存在 index 均为安全 `400`，无行/无广播。
3. Go 用户隔离：外书 GET/PUT 为 `404`；同 book/index 数值不能读取或写入另一用户行。
4. Go WebDAV：既有顶层和 `legado/` 目录分别原子写入可恢复 JSON；无目录/无权限不创建；
   普通用户不能触碰管理员或另一用户文件；失败不回滚已提交进度。
5. 前端：background 保存产生一次 keepalive、零次普通 API；无法使用 keepalive 时只回退
   一次普通 API。冲突最多重试一次，过期 auth operation 不能提交。
6. 真实浏览器：两个登录同一账号的 390×844 客户端从同一基线保存不同位置，必须恰好
   一胜一冲突；SQLite、两个 Reader、本地 scoped 快照与全新上下文重开恢复位置一致，且
   远端恢复不回声 PUT。1440×900 和 360×800 执行同一合同。
7. 备份/卷：`readingProgress.json` 与新写入的单书 `bookProgress/*.json` 都能恢复；历史
   TXT/EPUB/UMD/CBZ archive 和用户隔离门禁不回归。

只有合同、失败测试、实现、全量测试、真实双客户端浏览器和卷门禁都通过后，才可把
阅读进度 API 从总矩阵的“尚未验证”改为 P2 对齐并发布 Docker。

## 2026-07-18 实施中证据

- `backend/services/readingprogress` 已接管用户/章节规范化、已有行和首次行的原子 CAS，
  API 仅为提交赢家广播；既有 WebDAV 进度目录在数据库提交后安全原子镜像。
- 页面退出现在只选择 keepalive 或普通保存之一；认证 scoped pending 快照继续保留。
- `useReaderRouteSync` 增加精确一次的位置重载抑制，BookLoad、搜索结果和外部进度这种
  “更新 URL 后自行加载”的事务不再与普通路由 watcher 双重加载。普通目录/翻章路由仍
  由 watcher 加载并保存，抑制不会延续到下一次用户操作。
- `reader-progress-multiclient-contract.mjs` 在三个目标视口使用真实 Go、SQLite、WebSocket、
  两个隔离上下文和全新重开上下文，已证明一胜一冲突、两端收敛、冷恢复及磁盘
  `bookProgress` JSON 一致。
- 全量 Go、474 个前端测试、生产构建、Reader text/mobile/continuous、书架多客户端以及
  真实 EPUB/CBZ 三视口合同通过；并发 CAS/WebDAV 重点测试连续 20 轮通过。
- 本地候选镜像通过全新卷/可移植备份恢复，以及历史 TXT、EPUB、UMD、CBZ、相对缓存和
  owner 隔离门禁。随后从本机构建并推送 `ghcr.io/changshengyu/openreader:9f19d21`
  与 `latest`；两个标签共同指向 amd64/arm64 索引
  `sha256:433f64126c65bc82b456da2be8e1cea644b1c53affcfbb98e3a8a4326cfc57cb`。
