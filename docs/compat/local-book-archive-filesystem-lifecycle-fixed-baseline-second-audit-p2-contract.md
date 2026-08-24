# 本地书归档文件系统生命周期固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前审查基线：`OpenReader@20ba211`  
审查日期：2026-08-24  
状态：**inventory-complete / implementation-pending**

## 1. 范围与权威证据

本轮继续按 mounted path、持久路径字段和文件副作用枚举，只覆盖已导入本地书归档的共享生命周期：

- `GET /api/books/:id/chapters/:index/content` 的本地 cache 读取和原 archive 惰性重建；
- `POST /api/books/:id/refresh-local` 的原文件读取、inactive generation、目录/书源 metadata 提升和旧
  派生正文剪枝；
- `POST /api/books/export` 的单本本地原文件优先和生成式 TXT/EPUB 回退；
- Book 删除后的最后引用归档清理；
- portable backup、本地 EPUB/CBZ/audio service 和旧卷 cache migration 的既有 fail-closed 行为回归。

固定上游 `BookController.kt#getLocalChapterList/#refreshLocalBook`、`BookHelp.kt`、
`LocalBook.kt` 和 `TextFile.kt` 以当前 namespace 的本地书文件恢复目录与正文；删除只清理该书自身存储。
OpenReader 的 `library/data/<safe-user>/<book>/` 私有归档、SQLite 数字 ID、原子 refresh generation、
portable v2 和多用户隔离是已签收技术适配。本轮不改变解析结果、章节顺序、导出格式或可见 Reader 流程。

权威 OpenReader 文件：

- `backend/api/books.go`、`local_book_source.go`、`local_refresh_stage.go`、`book_cleanup.go`；
- `backend/services/epubreader`、`cbzreader`、`audioreader`、`backup/portable.go`；
- `backend/db/db.go`、`backend/api/old_volume_contract_test.go`、
  `backend/api/books_lifecycle_contract_test.go`。

## 2. 已签收可见与数据合同

- 合法 TXT/EPUB/UMD/CBZ 历史卷仍可从原 archive 惰性恢复；相对字段和旧机器绝对
  `OriginalFile` 只可按 archive basename/suffix 重定位到当前用户私有书目录。
- `refresh-local` 继续支持空 body、可选 `tocRule`、有界完整源读取、stage -> SQLite transaction ->
  promotion -> prune；原 archive 字节不变，进度/书签按既有目录替换规则重绑。
- 单本本地 TXT/EPUB 导出继续优先返回原文件；原文件不可用时仍可按现有规则生成导出，不增加新状态码。
- Book 删除继续先提交数据库，再 best-effort 清理最后一个同用户真实归档引用；共享引用和其它用户归档
  不得被删除。
- 不迁移、不扫描、不重写现有 `Book.LibraryPath/OriginalFile/TOCFile/SourceFile`、chapter cache path、
  archive、SQLite、backup member、URL 或环境变量。

## 3. 当前差异与真实反例

| 生命周期 | `OpenReader@20ba211` | 裁决 |
|---|---|---|
| 归档根 | `localBookArchiveRoot` 分别 `EvalSymlinks(candidate)` 和 `EvalSymlinks(ownerRoot)`，只证明前者位于后者；没有证明 resolved owner root 仍位于可信 `LibraryDir`。 | **must-fix**：`library/data/<safe-user>` 自身可被换成指向库外目录的 symlink，随后 source/cache 读取仍成功。 |
| refresh 写入 | `ownedLocalRefreshArchiveRoot` 只做 lexical private path；`MkdirAll/MkdirTemp/Rename/Remove` 可沿 owner/book/content/metadata symlink 写到库外。 | **must-fix**：不安全归档必须在源读取、stage、DB 替换和事件前拒绝。 |
| source/cache 打开 | resolver 返回 pathname；`readBoundedLocalBookSource`、`readChapterCache` 和 original export 随后重新 `Open/ReadFile`。 | **must-fix**：检查后 mounted replacement 可切换实际读取对象；有界读取必须消费同一 verified regular handle。 |
| 删除清理 | `resolvedPrivateImportedBookDirectory` 同样允许 resolved owner root 位于库外，随后 `RemoveAll` 删除该书目录。 | **must-fix**：数据库删除可以成功，但 unsafe/mounted cleanup 只能 fail closed，不能删除库外对象。 |
| EPUB/CBZ/audio/portable | service 已把 book root 重新约束到 canonical `LibraryDir`；portable candidate 解析到库外时 unavailable。 | **regression-only**：不得因共享 helper 改造而放宽这些已签收边界。 |

2026-08-24 的隔离真实 Go/HTTP 探针使用 `/tmp/openreader-export-audit-library`，导入一册 TXT 后把
`library/data/exportaudit` 移到库外并在原位置建立 symlink：

1. `POST /api/books/1/refresh-local` 返回 200，重建 chapter/metadata 和 `.refresh-*` 到库外目录；
2. `GET /api/books/1/chapters/0/content` 返回 200 和库外 archive 正文；
3. `POST /api/books/export` 没有返回库外原文件，因为 handler 的额外 `LibraryDir` 检查使其回退到生成式
   TXT，但生成正文仍可经上述 cache/source 路径读取；
4. `POST /api/books/batch {action:"delete"}` 返回 200 后，库外书目录被真实删除，仅外部用户根保留。

探针只使用 `/tmp` 隔离目录，服务已停止。它直接反证
`local-book-old-volume-p1e4-contract.md` 中“符号链接不得逃逸”和“清理只作用于私有归档”的既有声明。

## 4. 目标 rooted archive 合同

1. `LibraryDir` 是本地书归档的唯一可信边界。归档 resolver 必须从该边界逐组件检查
   `data/<safe-user>`；root、`data` 和 owner root 的 symlink、FIFO/socket/device 或非预期类型均 fail
   closed，不能把 resolved owner root 当作新的信任根。历史书目录 symlink 只有在解析目标仍位于同一已
   验证 owner root、引用判定使用同一真实目录且后续 entry 仍受 rooted identity 检查时可作为安全 alias
   保留；越界 book root、`content`/metadata parent 和最终 entry symlink 或特殊文件均 fail closed。
2. 合法旧 SQLite 相对/绝对字段仍可按当前 basename/suffix 规则重定位，但最终目标必须是同一可信书目录
   内的普通文件。旧宿主绝对路径本身永不成为候选。
3. source、chapter cache 和 original export 从 `Lstat/open/fstat/SameFile` 或等价 rooted API 得到调用方
   负责关闭的同一 opened regular handle；大小预算、解析和响应 bytes 必须消费该句柄，不得在校验后按
   pathname 重开。检查后替换只能消费已验证对象或安全失败，不能读 replacement。
4. refresh 必须先取得可信 archive/source identity，再建立同书目录 inactive generation。stage、metadata
   和 promotion 的所有 parent/target 都受同一 rooted service 约束；任何 unsafe/missing/special-file 错误在
   DB transaction、WebSocket 和活动 generation 变更前失败。原 archive 永不改写。
5. refresh 的 promote/prune 不跟随 symlink；只删除已提交目录不再引用的普通派生文件或该请求自己的
   inactive generation。失败保留旧 chapter rows、progress/bookmark、metadata 和 active cache。
6. Book 删除后的归档清理必须重新证明最后引用和 caller/book root。目录应先以同父原子 detach 或等价
   identity-safe 方式脱离活动路径，再递归删除；验证后被替换成 symlink 时最多移除该 link，绝不跟随到
   库外。unsafe 清理保持 best-effort，不回滚已提交的 Book 删除。
7. 所有拒绝路径使用既有 path-free API envelope：refresh 仍为安全 400/500，章节读取/生成式导出保持
   既有 unavailable/empty fallback，original export 安全回退，删除响应保持 durable DB 结果；不得返回
   host path、用户名目录、SQLite 或内部 syscall 文本。

## 5. 测试先行门

1. 在旧实现上加入 owner-root、越界 book-root、content/metadata ancestor 和 final-entry symlink，以及
   directory/FIFO 夹具；外部 sentinel bytes/hash/目录必须证明会被旧实现读取、写入或删除。
2. `refresh-local` 红测锁定 owner-root symlink 当前返回 200 并在库外产生 generation/metadata；目标实现
   必须在 DB、stage、event 前 path-free 失败，旧 rows/files/hash 不变。
3. chapter cache/source rebuild 和 original export 加 deterministic replacement 测试：在 resolver 校验后、
   实际读取前替换 entry；目标只读取 verified handle 或失败，replacement sentinel 永不进入响应/cache。
4. delete 红测锁定 owner-root symlink 当前会删除库外书目录；目标实现保持 DB 删除结果，但库外 sentinel、
   共享 archive 和外用户同名目录不变。另测验证后 entry replacement。
5. 合法 current relative、historical absolute basename/suffix、同 owner 内安全 book-root alias、
   TXT/EPUB/UMD/CBZ、relative cache、刷新原子性、进度/书签重绑、单本原文件导出、portable v1/v2 和
   startup cache migration 全部保持。
6. focused/race/full/vet、frontend full/build、真实 HTTP mounted probe、fresh/historical/portable/restart
   卷门通过后才可发布 Docker；纯后端边界不要求新增可见 UI，但 Reader/BookManage 导出和本地刷新至少
   完成三视口无回归 smoke。

## 6. 数据与允许差异

- 本切片只收紧未来请求和删除清理；不修复、移动、删除或扫描已存在的 unsafe mounted object。
- 不新增 schema、migration、startup rewrite、archive generation 格式、backup member、URL、API payload、
  frontend key 或环境变量。
- OpenReader 可以比固定上游更严格地拒绝 mounted symlink/special file，并以 opened-file identity、私有
  caller root 和有界读取实现同等可见结果；这是多用户/Docker 安全适配。
- 合法 regular 历史卷必须继续可读；不能以删除历史绝对字段回退、要求重新上传或改变 Reader 流程来
  代替 rooted lifecycle 修复。

## 7. 实施顺序

1. 独立提交本合同、差集、REST/迁移/安全矩阵，不修改应用或测试。
2. 在 `OpenReader@20ba211` 上提交可复现的旧实现红测。
3. 建立共享 caller/book rooted archive helper，先改 source/cache open，再改 refresh stage/promote/prune 和
   delete detach；保留 service/handler ownership 与旧路径回退。
4. 跑专项/race/full/vet、frontend/build、真实 HTTP/浏览器/mounted probe 和三类卷门。
5. 形成连贯切片后提交并推送；只在本机构建 amd64/arm64 并发布 GHCR。
