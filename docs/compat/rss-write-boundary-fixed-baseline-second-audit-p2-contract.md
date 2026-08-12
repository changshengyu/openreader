# RSS 写入与文章缓存并发边界第二轮固定基准合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只审查已部署 RSS REST 写入和内部缓存提交边界：

- `POST /api/rss/sources`
- `POST /api/rss/sources/import`
- `PUT /api/rss/sources/:id`
- `DELETE /api/rss/sources/:id`（只审查与其它 source mutation 的并发关系）
- `POST /api/rss/sources/:id/refresh`（只审查远端结果提交）
- `PUT /api/rss/articles/:id`
- `GET /api/rss/articles/:id/content`（只审查正文缓存提交）

可见 RSS 工作台、请求页/排序/解析/抓取安全、HTML 清理和 WebSocket recipient 已由
[`rss-visible-workspace-fixed-baseline-second-audit-p2-contract.md`](rss-visible-workspace-fixed-baseline-second-audit-p2-contract.md)
等已发布合同约束。本合同不得借机重开已签收 UI，也不把隐藏 read/favourite API 重新接入可见界面。

## 1. 权威文件与动作映射

固定上游：

- `src/main/java/com/htmake/reader/api/controller/RssSourceController.kt` 的
  `saveRssSource`、`saveRssSources`、`deleteRssSource`、`getRssArticles`、`getRssContent`；
- `web/src/components/RssSourceList.vue`、`RssArticleList.vue`；
- reader-dev 每用户 `rssSources` JSON namespace 和远端即时文章结果。

OpenReader：

- `backend/api/server.go`、`rss.go`、`request_body.go`；
- `backend/services/rss/service.go`、`backend/models/models.go#RSSSource/#RSSArticle`；
- `frontend/src/api/rss.js`、`components/RSSManager.vue`、`utils/rssSourceImport.js`；
- `backend/services/backup/backup.go#addRSSSources` 和
  `backend/api/webdav.go#restoreRSSSourcesFromData`。

上游把每个用户的 source list 作为一个 JSON 整体替换，单条与批量均以精确 `sourceUrl` 为身份；缺少
URL/name 的单条保存失败，批量导入则跳过该项。OpenReader 已发布的 JWT、caller-owned numeric ID、
SQLite source/article cache、事务、post-commit event 和 bounded fetch 是明确的多用户/安全适配，继续
保持。上游没有持久 read/favourite，也没有可直接复制的 HTTP body 预算；这些边界属于窄 Go 安全适配。

## 2. 当前证据与裁决

2026-08-12 对 `OpenReader@dc2b810` 的源码、固定上游和现有合同复审确认：

| 合同点 | 当前行为 | 裁决 |
|---|---|---|
| source create/update body | 无实际读取上限，`ShouldBindJSON` 只消费第一个 JSON 值。 | **must-fix security adaptation**：8 MiB actual-read、单个非 null JSON object。 |
| source import body | 已有 8 MiB `MaxBytesReader` 与 5,000 项上限，但仍接受第一个 JSON 后的第二文档。 | **must-fix wire boundary**：保留现有 8 MiB/5,000/平面 400，只接受一个非 null array。 |
| same-URL identity | create/import 都先查后写且互不协调；并发 create/import 可产生同用户同 URL 重复行。 | **must-fix data integrity**：同用户 source mutation 串行化并在事务内复验；另一用户不共享 identity。 |
| source ID update | owner 和 URL collision 预查后全行 `Save`；并发 delete 可触发 GORM fallback insert，复活目标。 | **must-fix concurrency**：显式列 update、zero affected 为 404、成功返回 fresh row。 |
| source delete | 已把 owner 查询、article 删除、source 删除放在一笔事务中。 | **aligned but shared lock required**：保持原子删除，并与同用户 create/import/update 使用相同 source mutation 边界。 |
| hidden article patch | `{}`、unknown-only、null 均可成功并广播；body 无上限；完整快照 `Save` 可覆盖刷新结果或复活删除行。 | **must-fix field ownership**：16 KiB 单 object；至少一个显式 non-null bool；仅更新提交的状态列。 |
| refresh cache upsert | 事务中读取 existing/duplicates 后以 `Save` 写完整 article，remote refresh 与 state patch 可互相覆盖。 | **must-fix column ownership**：远端刷新只拥有 parser/remote 列，不能写 read/favourite。 |
| content cache | 远端抓取后以请求开始时的 article 快照 `Save`；可能覆盖并发状态/刷新字段或在 source/article 删除后复活。 | **must-fix column ownership/liveness**：只写 content，复验 caller-owned article/source，zero affected 不复活。 |
| backup/data | `rssSources.json` 导出/恢复 source；RSS articles 是可重建缓存，不进入 portable logical backup。 | **aligned**：无 schema、索引、迁移或历史数据清理。 |

已有测试证明顺序执行时同 URL 替换、跨用户隔离、source delete 回滚、requested-page 和隐藏状态保持，
但不能证明 wire single-document、并发 create/import、删除后不复活或列所有权。现有绿测不得替代本合同。

## 3. 授权、JSON 与精确预算

- 所有入口继续先通过 JWT。ID update/patch 在读取 body 前解析正 ID 并查询 caller-owned target；非法 path
  保持 `400`，missing/foreign target 保持 `404`，即使 body 超限或 malformed 也不得先读取 body。
- source create/update 各只接受一个非 null JSON object，actual-read wire 上限 `8 << 20` bytes。精确
  8 MiB（含尾部 whitespace）进入现有字段校验；8 MiB + 1 返回
  `413 {"error":"request body too large"}`。
- source import 只接受一个非 null JSON array，actual-read wire 上限继续为 `8 << 20` bytes，原始数组
  最多 5,000 项。为兼容已发布客户端，malformed、错误 shape、空数组、5,001 项和 overflow 都保持
  `400 {"error":"invalid RSS source import"}`，且发生在逐项 SQLite 工作和 event 前。
- article state patch 只接受一个非 null JSON object，actual-read wire 上限 `16 << 10` bytes。精确 16 KiB
  进入字段校验；16 KiB + 1 返回 `413 {"error":"request body too large"}`。
- declared `Content-Length` 与 chunked/未知长度按 actual read 使用同一边界。空 body、`null`、错误顶层类型、
  第二 JSON 或尾随非 whitespace 垃圾按各入口上述稳定平面错误返回；合法文档后的 whitespace 可接受。
- 上限只属于这些 handler，不做全局 middleware，不改变已发布 BookSource、Bookmark、backup/restore、
  remote fetch 或 parser 的预算。

8 MiB source object 是兼容 reader-dev 完整 JSON 规则/header/script 与现有批量导入上限的保守边界；
16 KiB article patch 只容纳两个 bool 和兼容未知字段，防止无界 whitespace/unknown payload。

## 4. source 身份、字段与并发事务

- 继续接受 `title/url/icon/group/comment` 和 reader-dev 的
  `sourceName/sourceUrl/sourceIcon/sourceGroup/sourceComment` aliases。title 与 URL trim 后必填；其余完整
  source JSON 字段、header object/string 归一化和现有默认值保持。手动 create 缺少 `singleUrl` 默认
  `true`；import 缺少该字段默认 `false`。
- 单条 create 的同用户同 URL 是 upsert identity：新建保持 `201`，替换最早现有行保持该 ID/order 与
  `200`。ID update 与另一同用户 URL 冲突保持 `409`。另一用户的相同 URL 完全无关。
- create/import/update/delete 必须按 user ID 使用窄 source mutation lock；refresh/content 的持久提交阶段也
  进入同一边界以复验 source 存活，但远端网络工作不得持锁。锁内执行事务并重新读取身份、owner、
  collision 与 max order。不得使用全局锁，也不得让一个用户阻塞另一用户。
- 同用户并发 create/create、create/import 或 import/import 的相同 normalized URL 最终只能有一条 active
  row；最终配置按获得同用户锁后的提交顺序决定。不同 URL 仍使用现有 custom order 规则，批量新项按
  输入顺序追加。
- 已存在 source 的替换/更新必须使用显式配置列 update，SQL 条件至少包含 `user_id + id`；不得使用
  `Save`、struct full-row update 或 upsert fallback。row ID、user ID、created time 不可由请求改写。
- ID target 在 handler 预查后被删除时，事务内复验或 zero affected 返回 `404`；不得创建旧 ID/新 ID
  替代行。source delete 继续在一笔事务内删除 caller-owned articles 后删除 source。
- import 保持全部一笔事务、blank identity 跳过和 `{created,updated,skipped}` 计数。任一 SQL 失败全批
  回滚；成功且 created/updated 非零才发送一次 `source-import`。其它成功 mutation 也只在 durable commit
  后发送一次既有 kind；失败/404/409/no-op 不广播。

本合同不新增 `(user_id,url)` 唯一索引：历史数据库可能已有重复行，直接加索引会使升级失败。窄的每用户
运行时串行化关闭新 HTTP 竞争，同时保留历史行可列出、导出、更新和删除；重复历史数据的显式清理不是
本切片的一部分。

## 5. article 状态、刷新与正文列所有权

三类写入必须遵循明确的业务列所有权和正文优先级：

1. `PUT /api/rss/articles/:id` 只拥有请求中显式提交的 `isRead` 和/或 `favorite`。至少一个字段必须存在且
   是 non-null JSON boolean；`{}`、unknown-only、null 或 wrong type 返回
   `400 {"error":"invalid RSS article payload"}`。未知字段可在有合法状态字段时忽略。
2. `UpsertArticlePage` 拥有远端/parser metadata：`sort,title,link,guid,author,image,summary,pub_date,
   published_at`。新文章可写 feed/parser 给出的初始 `content`；更新 existing 且 source 有 `ruleContent`
   时，refresh 只能填充空 content，不能覆盖已由详情规则缓存的正文；没有 `ruleContent` 的 feed source
   继续由 refresh 更新 content。任何 existing update 都不得把内存快照中的 `is_read/favorite` 放进
   `SET`。历史 duplicate collapse 仍把所有 duplicate 的 read/favourite 用 OR 合并到保留行；合并状态
   写入与 duplicate delete 必须在同一事务内并按实际提交顺序收敛。
3. article content fetch 完成是 `ruleContent` source 的权威正文写入，只拥有 `content`；不能写
   title/link/guid/summary/state 或 created time。提交时必须按 caller `user_id + article id + source id`
   复验 article/source 仍存在，只更新 content/updated_at，然后重读 fresh row 再清理并响应。

状态 patch 使用显式列 map 和 `user_id + id` 条件；zero affected 时复验 target，不得 `Save` 或 fallback
insert。成功后返回 fresh caller-owned row，并发送一次携带 fresh row 的既有 `article-update`。并发 refresh
更新 metadata/feed content、content fetch 更新权威详情、state patch 更新状态时，必须按上述所有权和正文
优先级收敛，不能把请求开始时的完整快照写回。

refresh 远端请求开始前的 source owner 检查不是提交授权。`UpsertArticlePage` 必须在写入事务中确认
caller-owned source 仍存在；若请求期间 source 被删除，返回 `404`，不写孤儿 article、不广播。content
fetch 同理：若 article/source 在网络工作期间被删除，最终为 `404` 且不复活。公开错误继续是安全平面
JSON，不回显规则、header/cookie、带 query URL、article content、JWT、SQLite 文本或内部路径。

## 6. 数据、备份与允许差异

- 不新增/删除/修改 SQLite 表、列、索引或 migration；不改 `data/cache/library`、浏览器 storage key、
  Router path 或 frontend request/response shape。
- RSS source 继续由 `rssSources.json` 导出并通过 reader-dev/portable/WebDAV restore 恢复；restore 有自己的
  archive/entry 预算，不经过本轮 HTTP body gate。RSS article 继续是可重建 cache，不加入 logical backup。
- 历史 oversized source 行继续可列出、导出、恢复、更新或删除；新 HTTP 上限不触发 startup scan、
  truncate、duplicate cleanup 或 cache purge。
- 继续保留 JWT、numeric ID、per-user SQLite cache、hidden read/favourite、WebSocket invalidation、bounded
  fetch/sanitize 和原子 source delete。它们是明确的技术栈/安全适配，不新增可见 RSS 行为。

## 7. 失败测试先行闸门

实现前必须先让当前代码在以下合同上失败：

1. source create/update 的 declared/chunked 8 MiB + 1 为精确 `413`；article patch 16 KiB + 1 同样失败；
   exact limit 进入原业务校验。import 8 MiB + 1 保持精确平面 `400`。
2. 三类 body 拒绝第二 JSON、尾随垃圾、`null` 和错误 shape，且 row/timestamp/event 不变；foreign/missing
   ID 继续优先返回 `404`。import 5,000 项进入事务，5,001 项在逐行 SQLite/event 前 `400`。
3. 同用户并发 create/create、create/import、import/import 的同 URL 最终一行；另一用户相同 URL 独立，
   不被共享锁串行或覆盖。
4. SQLite trigger/受控并发在 source ID update 前删除 target 时不能复活；collision/SQL rollback 均零 event。
5. article `{}`、unknown-only、null/wrong-type state 为 `400`；显式 false/true 可写；只提交一个字段时另一
   状态保持。并发 remote refresh/content 变更不能被 state patch 旧快照覆盖，预写 delete 不能复活。
6. refresh upsert 与 state patch 交错时 remote/state 列都保留；duplicate OR 合并保持。远端 fetch 期间删除
   source、content fetch 期间删除 article/source 后，迟到结果不能写孤儿行或复活目标。
7. source aliases/defaults/status/import counts、requested-page/parser order、sanitize、visible four-viewport RSS
   workspace、backup/restore、WebSocket scope 和 mounted historical data 回归全部继续通过。

实现后运行 focused API/service tests、关键并发合同 `-race -count=3`、`go vet ./...`、`go test ./...`、
frontend 全量和 production build。真实 HTTP 探针覆盖 declared/chunked/single-JSON/cardinality/zero side
effect/SQLite trigger；四视口继续执行 `scripts/smoke/rss-workspace-contract.mjs`。发布前顺序执行
fresh/historical mounted-volume 与 portable backup 门，再由本机完成 amd64/arm64 构建和 GHCR 发布。

## 8. 实施约束

- 复用 `decodeBoundedSingleJSON` 的 actual-read primitive，在 RSS handler 内映射本文既有平面错误；不改变
  共享 decoder 或其它 endpoint 语义。
- source mutation lock 和业务事务属于 `backend/services/rss`；Gin handler 只做 owner 优先级、decode、
  DTO normalize、service error 到 HTTP/event 映射。lock registry 必须按 user ID 引用计数回收。
- existing source/article 更新使用显式 column map 和 fresh reload；不要以 GORM `Save`、全行快照或全局
  mutex 替代字段所有权。
- 并发测试使用 barrier、SQLite trigger 或 service hook 稳定复现旧覆盖/复活风险，不以 sleep 依赖偶然
  调度，也不通过测试专用全局串行化掩盖竞争。

本 inventory pass 不修改应用或测试代码。下一门是提交能证明旧实现违反上述 wire、身份和列所有权合同的
失败测试；合同、红测和实现必须保持独立提交。

## 9. 2026-08-12 实施、验证与发布记录

本合同按三阶段门完成：`051bd12` 固定上游与当前差异；`1d0e5a0` 先提交旧实现红测；`5236389`
实现有界单 JSON、每用户 source 写事务、显式列所有权、正文优先级和远程工作后的存活性复验。
`01d1503`/`0986d8e` 再加入可复现运行探针；后者区分宿主 `HTTP + SQLite trigger` 与容器纯公开 API
模式，避免把 macOS/OrbStack bind mount 的跨 VM WAL 可见性当成产品语义。

实现结果：

- source create/update 使用 8 MiB actual-read 单 object；article state 使用 16 KiB 单 object；import
  保留 8 MiB、5,000 项与平面 400。declared/chunked、精确上限、多 JSON、null/shape 和 target 优先级
  均有 API 与真实 HTTP 证据。
- source create/import/update/delete 共享按 user ID 引用计数回收的窄锁和事务；SQLite `BUSY/LOCKED`
  对整笔 RSS 写事务做有界重试。同用户并发同 URL 收敛到一行，不新增历史数据不兼容的唯一索引。
- source/article 不再使用 `Save` 提交 existing row；state、refresh metadata 和 detail content 各自只写
  所拥有列，成功后重读 fresh row。SQLite trigger 证明并发字段不被旧快照覆盖、删除行不复活。
- refresh/content 的网络工作不持锁；提交阶段重新确认 caller-owned source/article。受控远程 callback
  在响应前删除目标时返回 404，不写孤儿缓存、不广播迟到成功。

验证结果：

- 新增 API/service 合同及既有 RSS 测试通过；关键 API/service 并发合同 `-race -count=3`、并发身份
  `-count=5`、`go vet ./...` 与 `go test ./...` 通过。
- frontend `740/740`、production build、`rss-workspace-contract.mjs` 四视口通过。
- `rss-write-boundary-contract.mjs` 在真实 Go 服务的 HTTP + SQLite trigger 模式，以及候选/回拉容器的
  public-API 模式通过；覆盖 8 MiB/16 KiB、5,000/5,001、同 URL 并发、显式列、正文优先级和远程删除。
- 本地候选与 GHCR 回拉镜像均通过 fresh volume 的 portable-v1/v2-assets、cross-user、restart；
  historical volume 的 TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation 通过。候选 historical
  首次出现一次无上下文 404，原样顺序复跑通过；正式回拉镜像首次即通过。

本机双架构发布标签为 `ghcr.io/changshengyu/openreader:0986d8e` 与 `latest`，OCI index digest 为
`sha256:9884d0e9b41c1a1a109f3034159ae2968fe9d889da063deaca2514e1ac371e25`；平台 manifest 为
linux/amd64 `sha256:fb145fafb58b0cef60be62b46b150fbd7e9dfa7d760457407e5adefa8235b5a0` 和
linux/arm64 `sha256:0724dc6d880d7dc1c89aa77488eae2f599ec3f8be06a938294c31afac067dd91`。
本批无 schema、索引、迁移、备份成员、路径、前端路由或可见 RSS 工作台变化；允许差异仍只有第 6 节
列出的 JWT/numeric ID/per-user SQLite cache/hidden state/WebSocket 与安全抓取适配。
