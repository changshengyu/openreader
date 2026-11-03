# BookInfo 追更开关：本地书架书与真实接口合同种子（P2，F1–F3）

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

审计日期：2026-11-03。

状态：**implementation-complete / regression-validated**（第 7 节；完整模式封面一步的偶发失败另起，见第 8 节）。

本合同是 [`bookinfo-fixed-baseline-second-audit-p2-contract.md`](bookinfo-fixed-baseline-second-audit-p2-contract.md)
第 5 节“追更……覆盖远程与本地书架书”的补齐，并修正真实接口浏览器合同
`scripts/smoke/bookinfo-shelf-mutations-real-api-contract.mjs` 的远程书种子。API 边界沿用
[`bookinfo-shelf-mutations-p2-contract.md`](bookinfo-shelf-mutations-p2-contract.md)（精确 `PUT /api/books/:id {canUpdate}`）。

## 1. 现象与根因

真实接口合同在第一个视口（1440×900）第 335 行失败：点击书籍信息里的 `.inline-update-switch .el-switch` 后 15 秒内没有
`PUT /api/books/<远程书 id>`。`69c27bc`（当前 `origin/main`）与 `claude/reader-menu-alignment` 同样失败；2026-11-02 夜间
Element Plus 记录已注明“改前构建同样失败，另起”。

2026-11-03 在 `69c27bc`、新数据目录上实测：

- `POST /api/books` 带 `sourceId: 1` 返回 `201`，保存的 `sourceId` 是 `0`；`GET /api/books/:id` 同为 `0`。
- 这本“远程书”的书籍信息显示 `来源： 本地` 和本地“更新”按钮。
- 点击追更：没有任何 `PUT /api/books/:id`（只有设置同步的 `PUT /api/settings/*`），开关仍为开，没有提示。

选择器 `.inline-update-switch .el-switch`、路由 `PUT /api/books/:id`、载荷 `{canUpdate}` 都是对的，不是原因。原因有三层：

1. **产品**：`frontend/src/composables/useOverlayBookInfo.js#toggleBookCanUpdate` 在 `!book.sourceId` 时直接返回。这个条件来自
   `855cc25`（2026-05-21），当时开关只在 `sourceId>0` 时渲染。BookInfo 第二轮复审 `7f7e2ef`（2026-08-02）按上游把开关恢复为
   所有书架书可见（`BookInfoPanel.vue` `v-if="inShelf"`），目标写明“覆盖远程与本地书架书”，但处理函数的条件没有改：本地书架书
   显示的追更开关点了没有请求、不翻转、没有提示。
2. **合同种子**：真实接口合同用 `POST /api/books` 带 `sourceId: 1` 造“远程书”。`8de9a38`（2026-08-12，书籍写入边界）之后，这个
   兼容 DTO 按设计忽略服务端拥有的书源字段（`api-contract.md` 的 `POST /api/books` 一行：来源由专用的远程确认与本地导入路由决定）。
   所以从 2026-08-12 起这本“远程书”其实是无书源的书，正好落进第 1 条的条件。之后加的两个聚焦模式 `LOCAL_REFRESH_ONLY`
   （2026-08-27）与 `BOOK_PATCH_ONLY`（2026-08-30）都跳过追更一步，完整模式一直红到 2026-11-02 才被记录。
3. **覆盖缺口**：合同的本地分支只断言追更开关存在（第 448 行），从不切换；单元测试也只切 `sourceId: 8` 的书。

## 2. 上游权威

- `web/src/components/BookInfo.vue` 51–61：`<span class="book-prop-btn" v-if="isInShelf">追更 <el-switch v-model="showBookInfo.canUpdate"
  … @change="toggleBookCanUpdate(showBookInfo)">`；125–129：`isInShelf` 按 `bookUrl` 在书架中查找，本地书（`origin=loc_book`）同样算在架。
- 同文件 235–237：`toggleBookCanUpdate(book) { return this.saveBook(book) }`，没有来源或书源条件；238–255：`POST /saveBook`，
  成功“操作成功”并 `updateShelfBook`，失败“操作失败…”。
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#saveBook` 1189–1329：只要求 `origin` 与 `bookUrl` 非空；已在架的
  本地书（`bookUrl` 在 `storage/data/…`）不进入任何导入分支，直接替换书架记录，所以本地书的 `canUpdate` 同样会保存。
- 同文件 1753：书架刷新只对非本地书读取 `canUpdate`（`!book.isLocalBook() && book.canUpdate && refresh`）。本地书的追更值会保存，
  但不影响刷新。

## 3. 差异与目标

| # | 项 | 上游 | 当前 | 分类 | 目标 |
|---|---|---|---|---|---|
| F1 | 本地书架书切追更 | 保存，成功“操作成功”，书架记录更新。 | 静默丢弃：无请求、开关不翻转、无提示。 | `must-fix` | 任何有 `id` 的书架书（本地或远程）都发送精确 `PUT /api/books/:id {canUpdate}`，同样的 loading、服务端投影合并、成功“操作成功”、失败“操作失败”并保留服务端值。 |
| F2 | 真实接口合同的远程书种子 | — | `POST /api/books` 忽略 `sourceId`，“远程书”实为无书源书（来源“本地”、有本地“更新”）。 | `must-fix`（测试种子） | 改走 `POST /api/books/remote`：本机回环夹具书源（`OPENREADER_SOURCE_NETWORK_ALLOWLIST=127.0.0.1`）+ 当前用户自己的 `source.id`；断言远程书籍信息显示该书源名、没有本地“更新”。 |
| F3 | 真实接口合同的本地分支 | — | 只断言开关存在。 | `must-fix`（覆盖） | 本地书籍信息切追更：请求精确为 `{canUpdate:false}`、持久化、开关显示为关；随后本地更新不改回追更值。 |
| A1 | 精确 `{canUpdate}` 与上游整本 `saveBook` | 整本保存 | 单字段 | `acceptable-change` | 不变（已在 `bookinfo-shelf-mutations-p2-contract.md` 记录：不用旧快照覆盖并发保存）。 |
| A2 | 开关跟随服务端投影 | `v-model` 先翻转，失败不回退 | 等服务端返回再翻转，失败保留服务端值 | `acceptable-change` | 不变（第二轮复审第 5 节）。 |

## 4. 不重开

- `POST /api/books` 兼容 DTO 保持收窄：书源绑定只走 `POST /api/books/remote` 与本地导入路由。
- 后端 `PUT /api/books/:id` 不改：已接受无书源书的 `{canUpdate}`（`book_write_boundary_contract_test.go` 历史超长行的单字段
  更新；`local_book_refresh_request_lifecycle_contract_test.go` 本地刷新期间并发保存的 `canUpdate:false` 被保留）。
- 定时检查只对远程书生效（`TestSchedulerSkipsBooksWithCanUpdateDisabled`）、本地更新、封面、分组、BookManage、Reader。
- 真实接口合同的其它断言（尺寸、字段、封面、分组、已存在远程书再加入、本地更新）原样保留，只换远程书的造法。

## 5. 计划测试

| # | 位置 | 断言 | 改前预期 |
|---|---|---|---|
| T1 | `frontend/tests/overlayBookInfo.test.mjs` | 本地书架书（`sourceId: 0`）切追更：只调用一次 `updateBook(id, {canUpdate:false})`，书籍信息与书架采用服务端投影，`updatingBookId` 复位，提示“操作成功”。 | 失败（没有调用） |
| T2 | 同上 | 本地书切追更失败：提示“操作失败”，书籍信息保持服务端值 `canUpdate:true`，`updatingBookId` 复位。 | 失败（没有调用） |
| T3 | 真实接口合同种子（F2） | 远程书经 `POST /api/books/remote` 建立；书籍信息显示该用户书源名、无本地“更新”；远程追更精确 `{canUpdate:false}`、持久化、开关显示为关。 | 通过（对照） |
| T4 | 真实接口合同本地分支（F3） | 本地书追更精确 `{canUpdate:false}`、持久化、开关显示为关；本地更新后仍为 `false`。 | 失败（等待 `PUT /api/books/<本地书 id>` 超时） |

对照：现有远程书切追更的单元测试；Go 端上述两项已有测试。

回归：frontend `npm test` 与 `npm run build`；backend `go test ./...`、`go vet ./...`；依赖公告门；真实接口合同完整模式五个视口
（1440×900、390×844、360×800、1024×1366、1366×1024），以及 `BOOK_PATCH_ONLY=1`、`LOCAL_REFRESH_ONLY=1` 两个聚焦模式三视口。

## 6. 允许差异

只保留 A1、A2 与第二轮复审第 9 节已列的差异。

## 7. 记录（2026-11-03）

分支 `claude/bookinfo-follow-update-toggle`，从 `origin/main` `69c27bc` 切出：合同 `1f6cced`、红测 `b8a106d`、实现 `408cf1a`。

- 红测（`69c27bc` 构建）：T1、T2 在 `assert.deepEqual(fixture.calls[0], ['update-book', 2, { canUpdate: false }])` 失败（实际
  `undefined`），其余 9 项通过。浏览器合同 1440×900 远程分支全部通过（远程添加种子、书源名、无本地“更新”、远程追更精确/持久化/
  开关为关，以及封面、分组、已存在远程书再加入），停在本地追更（第 508 行）：15 秒内没有 `PUT /api/books/<本地书 id>`。
- 实现：`toggleBookCanUpdate` 只要求书有 `id`。同一断言转绿，单元 11/11。
- 回归（本分支）：frontend `npm test` 857/857 与 `npm run build`；backend `go test ./...` 31 个包通过、7 个无测试（api 124.5s），
  `go vet ./...` 无输出；公告门自测 109/109；实际前端公告门通过；`docker compose config` 通过；真实接口合同 `BOOK_PATCH_ONLY=1`、
  `LOCAL_REFRESH_ONLY=1` 三视口通过，完整模式五个视口通过（封面一步的偶发失败见第 8 节）。实际 Go 公告门在本分支报
  “advisory changed: review required”：这是 GO-2026-5932 的变化，本分支基于 `69c27bc`，早于 main 上的 `544b980` x/crypto 固定，
  与本切片无关。
- 合入预检（临时把本分支合并到 `origin/main` `636b1ff`，未提交、已删除）：只有 `reader-dev-openreader-gap-analysis.md` 与
  `refactor-audit-matrix.md` 顶部条目冲突（两边都保留，新的在前）；frontend 872/872 与 build、实际 Go 公告门通过、完整模式五个视口
  与 `LOCAL_REFRESH_ONLY=1` 三视口通过。
- 状态 **implementation-complete / regression-validated**，完整模式的封面一步在第 8 节修复前会偶发失败；待合入与可信 Actions。
  镜像未构建、未发布；生产未动。

## 8. 新发现（另起）：并发认证请求让 `PUT /api/books/:id` 返回 500

回归中完整模式偶发在“cover state persisted”超时。抓到的每一次失败里，封面 `PUT /api/books/:id` 都返回
`500 {"error":"failed to update book"}`，服务端日志为 `api/books.go:599 database is locked`（事务里的 `Updates`）。

- 原因：`a0edce3`（2026-08-26）起每个认证请求都会在 `authsession.Service.renew` 里提交一次 `UPDATE user_sessions`。SQLite 为 WAL、
  连接池不限、事务默认 deferred（DSN 没有 `_txlock`）。`updateBook` 的事务先读书籍行、校验封面文件、再写；这期间另一个认证请求
  提交了写入，读快照就无法升级为写，SQLite 立即返回 busy，不等 5 秒的 busy_timeout（浏览器日志里这个 500 在请求发出约 16ms 后返回）。
- 只用接口复现：每次 `PUT` 同时发 4 个认证 `GET /api/books/:id`，封面 150 次里 60 次 500（不并发 0/150）；`{canUpdate}` 300 次里
  58 次 500（不并发 0/300）。真实用户在多标签、多设备或页面并发请求时同样会看到“操作失败”。
- 真实接口合同在浏览器发出 `PUT` 后立即用 `GET` 轮询持久化结果，正好制造了这种并发：完整模式 18 次中 14 次通过、4 次失败，失败都在
  封面一步，追更一步没有出现。
- 本切片不修改也不掩盖：合同轮询保持原样，修复需要单独的后端/SQLite 切片（按 CLAUDE.md 不禁用 WAL、不改单连接）。
