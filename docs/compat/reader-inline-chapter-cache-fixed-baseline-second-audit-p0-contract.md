# Reader 内联章节缓存第二轮固定基准合同（P0）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同最初只完成合同提取和现状映射，并以 `03e337a` 独立推送；应用与测试实施结果记录在第 7 节。
此前 2026-07-10 的聚焦审查只确认了内联面板的所有权和几何，并把“缓存引擎和取消行为未变”当成
正确结论；本轮逐行比较证明该结论遗漏了目标区间、适用书籍、完成文案和取消状态转换，不能继续
作为签收依据。

## 1. 权威文件与当前映射

固定上游：

- `web/src/views/Reader.vue` 的 `.cache-content-zone`、`showCacheContent()`、
  `cacheChapterContent()`、`cancelCaching()`；
- `web/src/App.vue#getBookContent()`；
- `web/src/plugins/helper.js#LimitResquest`、`cacheFirstRequest`。

OpenReader：

- `frontend/src/components/reader/ReaderCachePanel.vue`；
- `frontend/src/components/reader/ReaderDesktopProgress.vue`、`ReaderMobileChrome.vue`；
- `frontend/src/composables/useReaderChapterCache.js`；
- `frontend/src/utils/readerChapterCache.js`、`bookChapterCache.js`；
- `frontend/src/views/Reader.vue`；
- 既有 `GET /api/books/:id/chapters/:index/content` 和临时 Reader `no-store` 合同。

## 2. 固定上游状态机

1. 点击底栏阅读进度只切换同一 Reader 内的 `.cache-content-zone`，不打开抽屉或对话框。
2. 空闲区按顺序显示 `缓存章节`、`后面50章`、`后面100章`、`后面全部`。
3. 三个动作都从当前目录项的下一章开始切片；数字动作最多取 N 章，`true` 取到目录末尾。
   目标区间不先按浏览器缓存命中情况缩短。
4. 没有后续章节时提示 `不需要缓存`。只要目录仍有后续章节，即使它们已在浏览器缓存中，也进入
   cache-first 队列；已有缓存会快速命中并计入完整区间进度。
5. 队列并发固定为 2。每个任务调用 `getBookContent(index, timeout=30000, refresh=false,
   cache=true)`；浏览器缓存未命中时才请求服务端，并允许服务端写自己的章节缓存。
6. 开始时显示 `正在缓存章节  0/总数`；每个成功或失败的任务结束都推进完成数。单章失败不终止
   其余队列。
7. 队列自然结束只提示 `缓存完成`，随后恢复三个动作。
8. 取消只清空尚未开始的队列；最多两个已经在途的请求可以自然结束。点击取消的同步状态转换必须
   立即把 `isCachingContent` 设为 false、清空状态文本并恢复三个动作，不显示额外取消 toast。

## 3. 差异矩阵

| 合同层 | 固定上游 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 所有权与几何 | 单一内联区；桌面位于进度左侧，mini 位于底栏内。 | 单一 `ReaderCachePanel` 复用于桌面和手机；不创建 workspace/drawer。 | **aligned**；保留现有 Vue 3 组件化和响应式布局。 |
| 可见表面与动作 | 继承底栏背景，无独立卡片边框/阴影；50、100、全部是扁平文本动作；缓存时状态和关闭图标替换动作。 | 面板增加独立边框/阴影，三个动作是描边按钮，取消又改为文字按钮。 | **must-fix**；保留语义化 `button`、focus/ARIA 和响应式触控尺寸，但恢复扁平表面与关闭图标，不增加“取消”文字产品动作。 |
| 目标区间 | 目录的完整后续切片，cache-first 命中仍计入。 | `readerChapterCacheTargets` 和 `cacheBookChaptersToBrowser` 都先剔除已缓存 index；全命中时错误提示“不需要缓存”。 | **must-fix**；“不需要缓存”只能表示当前章已是目录末尾。 |
| 适用书籍 | Reader 没有按本地/远程隐藏或拒绝该动作；同一内容入口处理书架书。 | `useReaderChapterCache` 对 `!isRemoteBook` 静默返回，本地 TXT/EPUB/UMD/CBZ 点击无结果。 | **must-fix**；已入架本地书使用相同受保护章节 API 和用户作用域浏览器 key。 |
| 临时 Reader | 上游未入架搜索结果仍走浏览器 cache-first。 | 临时会话响应为 `Cache-Control:no-store`，专项合同禁止 progress/bookmark/cache writer。 | **intentional security/runtime difference**；继续明确拒绝，不把会话正文写入 IndexedDB、SQLite、`cache/` 或备份。 |
| 并发与失败 | 并发 2；单项错误计数后继续。 | `cacheBookChaptersToBrowser` 默认两个 worker，单项异常后继续。 | **aligned**；不得改成无界并发或因单章失败中止完整队列。 |
| 完成反馈 | 仅 `缓存完成`。 | `缓存完成：N章`；取消后还提示 `已取消，缓存 N 章`。 | **must-fix**；恢复精确成功文案，取消无 toast。 |
| 取消状态 | 同步清空 pending，并立即恢复空闲 UI；在途最多 2 项自然结束。 | 只置 `cancelled=true` 并显示 `正在取消缓存...`，直到在途请求完成后才恢复。 | **must-fix**；慢请求不得把面板锁住至 30 秒。 |
| 结束后副作用 | 不刷新目录；浏览器 cache-first 自身持久化。 | `finally` 还调用 `loadChapters()`，可能产生无关目录重载。 | **must-fix**；结束只刷新当前书浏览器缓存投影，不重取/重建目录。 |
| 身份/书籍切换 | 上游 Reader 生命周期隔离当前任务。 | 任务捕获旧 book/id，但 `finally refresh()` 读取当前 refs；旧任务可能刷新新书状态或显示迟到 toast。 | **must-fix security/runtime adapter**；账号 generation、路由书籍或会话变化后，旧任务不得提示、刷新或改写新作用域状态。 |

## 4. API、数据与安全边界

- 不新增或修改 REST 路径。已入架书继续使用
  `GET /api/books/:id/chapters/:index/content`，JWT 与当前用户 book scope 不变。
- 浏览器 cache-first 命中不得发请求；未命中才调用既有 API。API 的 text/EPUB/CBZ/audio 返回、
  capability、错误码、书源变量和服务端缓存合同不由本切片改写。
- 浏览器 key 继续使用现有 `localCache@<user-scope>@...@chapterContent-<index>` 结构；不做迁移、
  重命名或跨用户复制。已有缓存必须继续可读。
- 不修改 SQLite、`data/`、`cache/`、`library/`、备份/WebDAV 或 portable archive 格式。
- 临时 Reader 的 `no-store` 响应和不写缓存合同优先于上游表面行为；面板可以保持可见，但动作须
  使用现有明确不可用反馈，不能静默，也不能绕过会话预算/TTL。
- 取消不是网络授权撤销：在途请求可自然结束并写入它们已经获得授权的当前用户缓存，但其迟到结果
  不得更新已切换账号/书籍的 UI。后续队列不得继续启动。

## 5. 测试先行门

实现前必须先让以下合同在当前代码上失败：

1. 纯函数：50/100/all 返回完整后续 index，包括已缓存项；只有目录末尾返回空。
2. cache-first worker：完整区间总数稳定；已缓存项不发 API 但仍推进 progress；并发峰值为 2；
   单章失败不停止后续项。
3. composable：本地书可执行；临时 Reader 明确拒绝且零持久化；开始状态、精确 `缓存完成`、取消
   无 toast 和不调用 `loadChapters()`。
4. 取消：点击后同步恢复空闲 UI；已有两个在途任务可结束，但 pending 不再启动。
5. 作用域：任务期间切换书籍或认证 generation，旧任务不提示、不刷新新书 cache map，也不把旧 key
   写进新用户作用域。
6. 浏览器：1440x900、390x844、360x800、1024x1366 验证内联几何、继承底栏的扁平表面、
   关闭图标、完整进度、cache-first 零重复请求、慢请求取消立即恢复，以及本地书动作不再静默。
7. 回归：frontend 全量、production build、Go 全量和完整 Reader 浏览器合同。没有持久格式变化，
   但适合发布的 Reader 切片仍须跑 fresh/historical mounted-volume 门。

## 6. 实施边界

本切片只重建 Reader 内联章节缓存状态机。BookManage 的整本服务器/浏览器缓存、全局
`/api/cache/stats`、`DELETE /api/cache`、章节图片缓存和服务端 cache stream 已有独立合同，不因
本轮重新设计。若测试发现既有 chapter-content API 或 browser key 不能满足上述状态机，必须先更新
对应 API/数据合同，不能在前端加入第二套未签约存储格式。

## 7. 实施与验证结果（2026-08-11）

- `readerChapterCacheTargets` 和共用 `cacheBookChaptersToBrowser` 现在保留完整后续区间；已有浏览器
  缓存仍由 `loadBrowserChapterContent` cache-first 命中，计入完整进度但不重复发 API。
- 队列继续使用两个 worker，并在任务开始时冻结用户 scope。每个 Reader 缓存任务拥有独立取消
  令牌；取消同步恢复三个动作，旧任务的两个在途请求可结束，但不能因新任务启动而恢复 pending。
- `useReaderPanels` 删除已入架本地书的静默 guard；TXT/EPUB/UMD/CBZ 使用同一用户作用域章节入口。
  临时 Reader 的 `no-store` 前置拒绝保持不变。
- 自然完成只提示 `缓存完成`，取消不提示；结束不再重载目录。书籍 URL/source、路由身份或账号 scope
  变化会同步取消旧任务，并抑制其迟到进度、提示和缓存投影刷新。
- `ReaderCachePanel` 保留语义化按钮和 focus/ARIA，恢复继承底栏的无边框/无阴影表面、扁平文本动作
  和关闭图标。

测试先在旧实现上失败于完整区间、关闭图标、本地书入口和取消状态。实现后：

- 聚焦 Node 合同 19 项通过；frontend 全量 **740/740**、production build 通过；
- backend `go test ./...` 通过；本批没有 Go 代码改动；
- 新增 `scripts/smoke/reader-inline-cache-contract.mjs`，在 1440x900、390x844、360x800、
  1024x1366 的生产构建上验证本地书、预置缓存零重复请求、并发 2、慢请求取消、完整区间、精确
  toast、扁平表面和视口边界；
- 完整 `reader-mobile-contract.mjs` 同时通过桌面、两种手机、自适应/强制移动 iPad 合同。

本批不改变 API、SQLite、挂载目录、备份、WebDAV 或浏览器 cache key。实现提交 `4da98fa` 的本地
arm64 候选先后通过：

- fresh volume 的 portable v1、portable v2 assets、cross-user 和 restart；
- historical volume 的 TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation 和 portable restore。

随后同一提交在本机生成并发布 `ghcr.io/changshengyu/openreader:4da98fa` 与 `latest`。两个远端标签
共同指向 amd64/arm64 OCI index
`sha256:771df515341f46f35a07b7f62de913490cdefe4489f3016d7d82f4436aa8f75d`；amd64 manifest 为
`sha256:f239157a67a381c552662cc83e341e4525fe22c840062be72ecf71faa6ea9c3f`，arm64 manifest 为
`sha256:0044561f83e3e605ca942b46f9c38a83499cdb8d4b27f6be9d0d8194f9094f4f`。GHCR 回读已确认两标签一致。
