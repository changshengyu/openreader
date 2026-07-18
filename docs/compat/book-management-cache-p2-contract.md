# BookManage 整本缓存与缓存操作 P2 兼容合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 完成重新审查；本文件只固定上游合同、当前差异、允许差异与失败测试，
不包含应用代码修改。此前 P1-D4-B3 只恢复了“单个有界任务的进度与取消”，不能证明
BookManage 的整本范围、多书并行、浏览器任务和可见动作已经与上游一致。

## 权威文件

上游：

- `web/src/components/BookManage.vue`
- `web/src/App.vue#getBookContent`
- `web/src/views/Reader.vue#cacheChapterContent`（只用于区分阅读器内“后 50/100/全部”）
- `web/src/plugins/helper.js#LimitResquest`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#cacheBookSSE`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#deleteBookCache`

当前：

- `frontend/src/components/overlays/OverlayBookManagement.vue`
- `frontend/src/components/overlays/BookManagementActions.vue`
- `frontend/src/components/overlays/BookManagementBatchFooter.vue`
- `frontend/src/composables/useOverlayBookItemActions.js`
- `frontend/src/composables/useOverlayBookBatchActions.js`
- `frontend/src/utils/bookChapterCache.js`
- `frontend/src/api/books.js#cacheBookContentStream`
- `backend/api/cache_stream.go`
- `backend/api/books.go#cacheBookContent`
- `backend/api/books.go#batchCacheBooks`

## 上游状态与可见行为

### BookManage 单书动作

1. 缓存菜单顺序固定为：远程书“缓存到服务器”、所有书“缓存到浏览器”、远程书
   “删除服务器缓存”、所有书“删除浏览器缓存”。
2. 服务器缓存从目录第 `0` 章覆盖到最后一章；`refresh=0` 时先枚举有效已有缓存并跳过，
   不是从阅读进度开始，也没有 20/50/300 章产品上限。
3. 浏览器缓存同样从第 `0` 章覆盖整本；已有有效浏览器内容由 `cacheFirstRequest` 复用，
   新请求以并发 `2` 执行，单章失败不会停止整个队列。
4. 再次点击正在缓存的同一本书会取消该书任务。服务器任务由该书自己的 EventSource
   关闭；浏览器任务由该书自己的 `LimitResquest.cancel()` 清空尚未开始的请求。
5. 活动任务按 `bookUrl` 分别保存在映射中，因此不同书可以同时缓存；关闭再打开
   BookManage 后仍能看见任务活动状态。
6. 删除服务器缓存和删除浏览器缓存都必须先确认；取消确认不得发送请求或删除本地项。
7. 标题显示 `❗️只能缓存文本内容`。行内章节信息同时显示总章数、服务器缓存数（远程书）
   和浏览器缓存数。
8. 单书导出菜单只有 TXT 与 EPUB。JSON 是备份/互操作能力，不属于此上游菜单。
9. footer 只有批量删除、批量添加分组、批量移除分组、已选数量和取消；没有批量缓存、
   批量清缓存或批量导出。

### 服务器 SSE 与数据语义

1. 请求必须鉴权，书籍必须在当前用户书架中；本地书和缺失书源在打开任务前返回明确错误。
2. 默认 `concurrentCount=24`，非正数回退 24；请求断开后不再继续安排工作。
3. 每章抓取必须带相邻下一章 URL。成功后写章节文本并保存正文图片；首次异常会结束任务。
4. `refresh<=0` 时跳过已有有效缓存；`refresh>0` 时强制重新抓取并覆盖。
5. 进度字段含义固定为：
   - `cachedCount`：已有有效缓存加本次成功缓存的总章数；
   - `successCount`：本次新抓取成功章数；
   - `failedCount`：本次失败章数。
6. 正常或失败收尾都使用终端 `event: end`；客户端断开不要求终端事件。
7. 删除服务器缓存删除该用户该书的整本服务器缓存目录。

### 与 Reader 内缓存的边界

Reader 内缓存是另一条上游流程：只缓存当前章之后的 50/100/全部，浏览器并发为 2，
并且可以取消。当前 `ReaderCachePanel` / `useReaderChapterCache` 已映射这条流程；本批不得把
BookManage 的“整本缓存”改成 Reader 的“从当前章向后缓存”，也不得更改 Reader 选项。

## 当前差异矩阵

| 项目 | 当前证据 | 判定与目标 |
|---|---|---|
| 服务器缓存范围 | `useOverlayBookItemActions` 从阅读进度开始发送 `count:20`；Go 未给 count 时默认 50、最多 300。 | `must-fix`：BookManage 必须从第 0 章覆盖整本；显式 count 的旧 API 窗口仍可兼容，但不能再作为 BookManage 默认。 |
| 浏览器缓存范围 | 从阅读进度开始，固定 `count:100`。 | `must-fix`：从第 0 章覆盖整本，跳过已有有效内容。 |
| 任务所有权 | 一个全局 `cachingBookId` 和一个 `activeCacheController`；第二本书会被拒绝。 | `must-fix`：按 book id 维护服务器/浏览器任务，允许不同书同时运行。 |
| 浏览器取消 | helper 支持 `cancelled()`，但 BookManage 未传入取消状态；活动按钮只会取消服务器流。 | `must-fix`：同一个活动入口可取消该书当前服务器或浏览器任务。 |
| 关闭后状态 | Dialog 使用 `destroy-on-close`，组件级任务映射会丢失。 | `must-fix`：任务 registry 生命周期必须高于 Dialog 内容；再次打开能显示仍在运行的任务。 |
| 删除确认 | 单书服务器/浏览器删除直接执行。 | `must-fix`：两项分别恢复上游确认文本和取消无副作用。 |
| 动作顺序/提示/导出 | 顺序为浏览器→服务器→删浏览器→删服务器；无标题警告；单书显示 JSON。 | `must-fix`：恢复上游可见顺序、警告和 TXT/EPUB 菜单。 |
| footer | 显示批量缓存、清缓存和 JSON 导出。 | `must-fix`：从 BookManage footer 移除。后端旧 batch action 可作为不展示的部署兼容接口保留。 |
| SSE 进度 | `{cached,requested,total,failed}`，没有已有与本次成功的正式区分。 | `must-fix`：增加 `cachedCount/successCount/failedCount`；保留旧字段别名，避免破坏已部署客户端。 |
| 已有缓存/刷新 | loader 会读已有缓存，但任务仍逐章进入 loader；请求无 `refresh` 字段且不能绕过缓存强制抓取。 | `must-fix`：默认先识别并跳过有效文件；支持显式 `refresh` 并真实绕过已有缓存。 |
| 失败透明度 | stream 调用丢弃 error 的 wrapper，空字符串才计失败。 | `must-fix`：使用带 error 的加载结果，发出稳定、无敏感细节的终端错误与准确计数。 |
| 后端并发 | 当前顺序执行；上游默认并发 24。 | `acceptable-change`：远端规则变量会把 book/chapter 状态持久化，盲目并发会产生顺序竞争。保留可取消的顺序抓取是 Go/SQLite 可靠性适配；必须保持完整整本范围、相邻 next URL 和进度语义。 |
| 部分失败 | 当前继续缓存后续章节；上游首次异常停止。 | `acceptable-change`：继续后续独立章节可提高可用性，但必须准确报告失败并仅在至少一章成功或已有缓存可用时正常结束。 |
| 全局缓存统计/清空 | OpenReader 提供当前用户 `/api/cache/stats` 与 `DELETE /api/cache`。 | `intentional-redesign`：保留当前用户隔离、引用安全的运维能力，不放进上游单书动作菜单。 |
| 鉴权传输 | OpenReader 以带 JWT 的 POST fetch 读取 SSE，上游使用 cookie EventSource GET。 | `acceptable-change`：保留，JWT 不得进入 URL、日志或错误事件。 |

## API 兼容合同

### `POST /api/books/:id/cache/stream`

请求：

```json
{
  "all": true,
  "chapterIndex": 0,
  "count": 0,
  "refresh": false
}
```

- `all=true` 且 `count<=0` 表示从 `chapterIndex`（省略时为 0）直到目录末尾；BookManage
  使用这个形式。
- `count>0` 保留现有有界窗口兼容，最多 300；`all=false` 仍要求 `chapterIndex`。
- `refresh=false` 跳过有效已有服务器缓存；缓存路径记录存在但文件缺失/为空时必须重新抓取。
- `refresh=true` 绕过已有服务器缓存并覆盖，不得通过临时清空共享数据库字段实现。

每个 `message` 与终端 `end` 至少提供：

```json
{
  "bookId": 7,
  "chapterIndex": 12,
  "processed": 13,
  "total": 900,
  "cachedCount": 420,
  "successCount": 8,
  "failedCount": 1,
  "cached": 420,
  "requested": 13,
  "failed": 1
}
```

旧 `cached/requested/failed` 作为只增不删的兼容别名保留；新 UI 只用规范字段展示。
终端成功额外提供合并后的 `book`。取消不广播完成态书架更新，但已完成写入仍可用。

### `POST /api/books/:id/cache`

保留同步兼容路径并接受同一请求体；`all=true,count<=0` 同样表示整本，不能继续隐式回退 50。
单章和显式 count 窗口保持兼容。响应增加规范计数字段，保留旧字段。

### 已部署 batch API

`POST /api/books/batch` 的 `cache` / `clear-cache` 动作不是上游 BookManage 合同。本批只从 UI
移除入口，不删除路由、不改变已有 50 本和每本 10 章的资源上限，避免破坏外部客户端。
测试必须把它标为部署兼容扩展，不能再用它证明 BookManage 上游对齐。

## 数据、安全与生命周期要求

- 不修改 SQLite schema，不破坏 `data/cache/library` 旧卷。
- 缓存路径读取、覆盖和删除继续通过已存在的 rooted/reference-safe helper；不得把绝对主机路径
  返回前端。
- 每个任务只能访问当前用户拥有的书、章节与书源；外用户 id 在打开流前返回 `404`。
- 远端失败文本不得包含 Cookie、Authorization、WebDAV 凭证、主机路径或完整内部堆栈。
- 任务 registry 不持久化 JWT、响应正文或 AbortController 到 localStorage/Pinia persist。
- 整本缓存可以很长，但请求断开、显式取消和组件卸载不得泄漏 reader、timer 或 fetch。

## 先写的失败测试

1. Go：60 章书以 `{all:true}` 请求时 `total/requested` 为 60，而不是 50；显式 `count:20`
   仍只处理 20 章。
2. Go：已有 3 章、新抓取 2 章、失败 1 章时规范字段分别为 `cachedCount=5`、
   `successCount=2`、`failedCount=1`；缺失/空缓存文件不得计为已有。
3. Go：`refresh=false` 不调用已缓存章的 fetch；`refresh=true` 会重新抓取并覆盖；下一章 URL
   仍传给解析器；取消后不再调度下一章、不广播完成态。
4. API：本地书、外用户书、缺失书源和全失败分别返回稳定状态/终端事件，不泄漏敏感错误。
5. 前端：BookManage 服务器与浏览器 payload 都从 0 覆盖整本，不读取阅读进度作为起点。
6. 前端：两本不同书可同时缓存；取消 A 不取消 B；同书第二次点击取消该书当前任务。
7. 前端：浏览器任务在并发 2 下可取消，取消后不启动剩余章节；重新打开 Dialog 仍显示活动任务。
8. 前端：两种删除取消确认时零请求/零 IndexedDB 删除，确认后只清目标书。
9. 静态/组件：动作顺序、标题警告、TXT/EPUB 导出和 footer 均与上游一致，JSON/batch 扩展
   不在 BookManage 可见 DOM。
10. 真浏览器（1440×900、390×844、360×800）：两书并行进度、单书取消、删除确认、整本
    大于 20 章、关闭重开任务状态、无横向溢出；Reader 内 50/100/全部缓存仍不变。

## 实施顺序与发布闸门

1. 先补上述 Go、composable 和静态失败合同。
2. 后端增加整本选择、refresh 和规范进度；不引入 schema 迁移。
3. 前端建立 Dialog 外的按书任务 registry，恢复整本范围、浏览器取消、确认和可见动作。
4. 重跑后端全量、前端全量/构建、BookManage 三视口和 Reader cache 回归。
5. 用旧卷执行本地 Docker volume/backup 门禁；形成可验收切片后立即提交 GitHub，并本地构建、
   推送 GHCR，报告完成项、允许差异、未完成项、标签和 digest。
