# 本地书刷新请求取消与陈旧提交第二轮固定基准合同

审查日期：2026-08-27

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审 `POST /api/books/:id/refresh-local` 在已有 body、parser、rooted archive 和正常
stage/promote 合同之外的请求生命周期与 SQLite 提交所有权。BookInfo/Reader 可见入口、TXT/EPUB/UMD/
CBZ/PDF/Markdown 解析结果、目录重绑、archive rooted filesystem、导出、删除清理和 portable/旧卷格式
继续由既有专项合同权威；不因本轮并发修复重开。

## 1. 权威源码与状态转换

### reader-dev

- `web/src/components/BookInfo.vue:219-233#refreshLocalBook`；
- `BookController.kt:296-326#refreshLocalBook`；
- `BookController.kt:1947-1977#editShelfBook`；
- `Book.kt:243-255#updateFromLocal`。

固定上游在当前请求内读取现有 shelf Book、执行一次本地信息刷新，然后只通过 `editShelfBook` 修改仍在
书架中的现有条目；目标已经不存在时不会重新添加。可见成功仍是“更新成功”并以返回 Book 刷新书架。
上游没有 Go request context、SQLite row race、inactive generation 或多用户 ID；OpenReader 必须做技术栈
等价翻译，不能用读取/解析前的旧整行快照复活删除或覆盖请求期间完成的编辑。

### OpenReader

- `backend/api/books.go:1562-1695#refreshLocalBook`；
- `backend/api/local_book_source.go`；
- `backend/api/local_refresh_stage.go`；
- `backend/api/book_cleanup.go#replaceBookChapterRows`；
- `frontend/src/composables/useOverlayBookInfo.js`、`useReaderCatalogActions.js`。

## 2. 当前差异矩阵

| 合同点 | 当前行为 | 裁决 |
|---|---|---|
| 请求取消 | owner/body 后的 opened-file read、最多 1 GiB legacy parse、chapter staging 和 transaction 均不读取 `c.Request.Context()` | **must-fix**：caller 取消后不得继续进入后续阶段、提交目录、提升 generation、剪枝或广播 |
| 远程工作后的目标存活 | 读取/解析/stage 前只读一次 Book，transaction 首先替换 chapter rows，随后 `tx.Save(&book)` | **must-fix**：并发删除可由 GORM `Save` fallback insert 复活 Book，并留下新 Chapter |
| 并发编辑所有权 | refresh 使用旧整行 Book 并全行保存 | **must-fix**：并发标题、作者、封面、简介、分组或追更编辑可被迟到 refresh 覆盖 |
| inactive stage | stale/cancel 没有显式裁决；正常 defer 只在 handler 返回时清理请求自己的 stage | **must-fix / 保持结构**：拒绝发生在 chapter transaction 和 promotion 前，请求 stage 必须清理，活动 generation/metadata 不变 |
| 正常成功与 rooted archive | body 先于源读取；opened regular source、parser budgets、stage -> transaction -> promote -> prune、引用重绑和一次 shelf broadcast 已签收 | **aligned / regression-only** |

## 3. 稳定 API 合同

### `POST /api/books/:id/refresh-local`

- 现有 JWT、path ID、owner-first 404、本地书 400、空 body、32 KiB 单 JSON、可选 16 KiB `tocRule`、
  成功 `200 {book,chapterCount}` 和 path-free parser/file error 保持。
- handler 必须使用 `c.Request.Context()`。取消在 DB commit 前被观察时不得再开始后续 source read、parse、
  chapter stage、transaction、promotion、prune 或 event；正在执行的有界同步 parser 可以完成当前调用，
  但返回后必须立即检查 context，不能把结果变成后台任务或持久结果。
- opened source 的完整读取必须可在 chunk 边界观察 context；chapter cache staging 至少逐章检查 context。
  取消产生的 inactive generation 由当前请求清理，不触碰活动 cache/metadata 或原 archive。
- 进入任何 chapter mutation 前，request-context GORM transaction 必须按 `id + user_id` 重读 Book，并
  验证至少 `source_id`、`url`、`library_path`、`original_file`、`toc_file`、`source_file`、`toc_rule`、
  `updated_at` 与读取前快照一致。目标已删除或任一字段变化时整笔结果陈旧。
- caller 仍有效时陈旧结果返回稳定 `409 {"error":"book changed during refresh"}`；caller 已取消时保持
  空响应。missing/foreign target 在工作前仍为 404，不通过竞争结果泄露其它用户状态。
- 成功事务只更新本动作拥有的 `url` 空值规范化、`last_chapter`、`chapter_count`、`toc_rule` 和
  `variable`，不用全行 `Save`。标题、作者、封面、简介、分类、追更、归档路径、创建时间及其它用户
  编辑字段保持当前行值。
- chapter replacement、progress/bookmark ID 重绑、archive metadata staging、promotion、旧派生正文剪枝和
  一次 durable shelf broadcast 保持原顺序。DB 已提交后即使 caller 随后离开，也必须完成该已提交
  generation 的 promotion/一致性收尾；不得让取消把已提交 chapter rows 指向未发布 stage。

## 4. 数据、文件与回滚

- 不增加表、列、索引、migration marker、环境变量、锁文件、generation 格式或 backup member。
- 不扫描、迁移、重写现有 `data/cache/library`、Book/Chapter/Progress/Bookmark、TOC/source metadata、
  original archive、portable/Legado/WebDAV 或浏览器状态。
- stale/cancel 只丢弃尚未提交的当前请求结果并清理该请求 inactive stage；并发删除、编辑和当前活动
  generation 是权威，不做补偿性覆盖。
- 回滚旧镜像可读取相同 SQLite 与文件。回滚会重新暴露取消后继续和 stale `Save` 风险，但没有格式
  不兼容或不可逆写入。
- 已发布 rooted archive contract 继续权威：本轮不得放宽 same-open-file、owner root、symlink/special-file、
  promotion/prune 或删除清理边界。

## 5. 测试先行门

旧实现上必须先证明以下用例失败：

1. source 已安全打开但读取/解析前取消：handler 返回后 Book/Chapter/Progress/Bookmark、active metadata/
   cache、原 archive、Hub 队列逐字节不变，且没有残留 `.refresh-*`。
2. stage 完成、transaction 前并发删除目标：删除保持成功，迟到 refresh 固定 409，不能重新插入
   Book/Chapter/Category relation/Progress/Bookmark 或提升 generation。
3. stage 完成、transaction 前通过正常 Book update 修改 title/author/cover/intro/category/canUpdate：迟到
   refresh 固定 409，新 Book 与现有目录/cache/metadata 保持，不广播旧 shelf item。
4. snapshot 字段 `url/library/original/toc/source/tocRule` 任一变化也拒绝；外用户同 ID/路径不可观察。
5. 正常空 body/显式 `tocRule` 继续完整替换目录、恢复 progress/bookmark、提升 metadata/cache、剪枝旧
   派生正文并只广播一次；TXT/EPUB/UMD/CBZ/PDF/Markdown 和历史相对/绝对字段保持。
6. focused/race、Go full/vet、frontend full/build、真实 HTTP/BookInfo/Reader 三视口与 trusted Actions
   fresh/historical/portable gates 通过后，才可发布 amd64/arm64 OCI index。

## 6. Inventory 结论

判定：**must-fix**。本地 refresh 是当前 route scan 中下一项长工作后仍以旧整行 `Save` 提交的动作。
既有合同只证明 body admission、rooted opened-file、正常 stage/promotion 和格式兼容，没有证明 caller
取消、并发删除或编辑后的 liveness/ownership。本阶段只记录合同和测试门，不修改应用或测试代码。

## 7. 实施与发布结论

合同 `474b992`、旧实现红测 `e6138f3` 和实现 `8df38f1` 按三阶段顺序落地；聚焦浏览器模式
`b338c3f` 只增加验证资产，不改变镜像应用代码。实现后：

- opened-source 完整读取按 chunk 观察 request context；同步 parser 返回后立即复查；逐章 stage 使用
  context-aware cache writer，取消清理当前 inactive generation 且不写业务响应、目录、metadata 或 event；
- GORM transaction 使用 request context，并在任何 chapter mutation 前按 owner 重读 Book、比较
  source/url/library/original/toc/source-file/toc-rule/updated-at snapshot；删除或变化固定返回 409；
- 全行 `Save` 已替换为 guarded owned-field update，只拥有 URL 空值规范化、last chapter、chapter count、
  TOC rule 和 variable；事务内重载当前 Book 后才生成 metadata 和最终 shelf projection；
- DB 成功提交后继续完成 generation promotion、旧派生正文剪枝和一次 durable shelf broadcast；现有
  TXT/EPUB/UMD/CBZ/PDF/Markdown、progress/bookmark 重绑、rooted archive 和错误/响应语义保持。

取消前读取、stage 后取消、并发删除、并发正常编辑和逐 snapshot 字段测试均通过；旧实现曾分别产生
200/持久副作用、删除后 Book/Chapter 复活和 metadata 编辑被覆盖。修复后 focused/race/API full、Go full/
vet、frontend 742/742、build、Compose，以及真实 BookInfo -> refresh-local -> Reader 的 1440x900、
390x844、360x800 Chromium 均通过。

可信 GitHub Actions run `33068512106` 又通过 backend/frontend/Compose、native image、fresh volume、
portable backup、historical volume 和 published-platform 门，发布 `8df38f1`/`latest`。OCI index 为
`sha256:1f6c8c509457043400f19e181b4d52fb8c648d5f84509c7b4fbdd44fdb610232`；amd64/arm64 manifests 分别为
`sha256:c2a753bbf1dae93bb5c11fe5ec2a478a07950cdcae902d843c4d6bb119e3e587`、
`sha256:a83a45245ca889f9c09a18371a9cb066df36a4c3101b7374aeba9785b1ccfc7d`。不可变标签回拉后的 OCI label
和 `/api/health` 均报告完整 revision `8df38f18ba84c9c8bdd850e3ac795e49cec6bee0`；`unknown/unknown`
entries 是对应平台的 provenance attestations，不是可运行镜像。当前状态
**aligned / regression-validated / Docker-published / awaiting-device-verification**。
