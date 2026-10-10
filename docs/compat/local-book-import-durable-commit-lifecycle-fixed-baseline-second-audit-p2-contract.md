# 本地书导入持久提交全生命周期第二轮固定基准合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
审查日期：2026-10-10；OpenReader 源码基线：`9cb6a030b9b2c6c5e6f55d2b19dcc719b554571a`。  
状态：**inventory-complete / red-tests-pending / implementation-pending**。

本提交只盘点合同，不修改应用或测试。源码窗口不是已复现漏洞，也不是生产故障结论。
已恢复的原书事件保持关闭；stage 应用 b2b32f7 已发布，不等于本项持久提交已签收。

## 1. 固定上游、共享入口和不可改变的结果

以下均以 `git show` 读取固定 SHA，而非 upstream 工作副本 HEAD：

- `BookController.kt:212–300`：认证、TXT/EPUB/UMD/CBZ 上传、namespace 工作文件、目录预览和
  同工作文件改规则；空目录仍可确认，preview 不保存书架。
- 同文件 `2263–2323`：LocalStore/WebDAV 权限、源文件存在检查，回到共同 preview/confirm 流。
- 同文件 `1189–1335#saveBook`：校验 origin/URL、caller namespace、本地原文件和 EPUB/CBZ 资源、
  book source 与 shelf 保存；提交 Book 包含 group。相同书名/作者合并已有阅读进度。
- `web/src/views/Index.vue:1512–1554`：只有保存成功才关闭导入框、提示成功并刷新书架。
- 同文件 `2310–2415`：统一分组应用于每个提交 Book，批量逐项保存、逐本确认和取消当前项继续。

固定上游没有证明 SQLite↔文件系统原子事务、request cancellation 或 exactly-once；其同名原文件
覆盖不能复制到 OpenReader。数字 Book ID、同名独立归档、唯一 reader-dev placeholder 原位恢复、
many-to-many 分组、失败保留 stage、多用户私有根和已签收 parser/Reader/backup 是允许技术适配。

本项覆盖四条 confirm 路由：`POST /api/imports/books`、`/api/imports/txt`、
`/api/local-store/import`、`/api/webdav/import`；raw/token、new/placeholder 两条 service 路径。
边界从已取得有界源 bytes/prepared catalogue 到：新归档分配→原文件→正文/资源→Book/Chapter/
分组事务→source/TOC→提交确认→成功响应/事件与原 stage best-effort consume。
共享 `RestoreExisting` 的 portable consumer 必须回归，不能用“只改 HTTP”留下第二套不安全归档。

## 2. 当前调用链与分类矩阵

| 生命周期 | 当前源码行为（9cb6a03） | 裁决 |
|---|---|---|
| archive allocation | `engine/library.go#ArchiveImportedBook/uniqueDirectory`：Stat 找空名字，绝对 MkdirAll/WriteFile；并发检查并非 exclusive directory ownership。 | **must-fix**：configured LibraryDir/data/user 原身份、独占分配新书目录，不能覆盖同时出现的同名对象。 |
| 原文件和初始失败 | 原文件直接绝对 WriteFile；失败在 Importer 接收 archive 之前，没有统一 partial compensation。 | **must-fix**：同一原 session 有界实际写、ctx、owned partial rollback，关闭全部 handles。 |
| 正文和 metadata | `importer.go` 事务中调用 Background `WriteChapterCache`，随后绝对 source/TOC JSON 写入；每个 helper 重新接收 pathname。 | **must-fix**：贯穿新归档的原 parent/book/entry identity；每步实际 I/O 使用 request context，不能把独立安全 helper 串接当完整生命周期。 |
| EPUB/CBZ resources | 先 `PrepareBookResources`，独立 source/root admission、extraction staging/rename；不接收 importer context/session。 | **must-fix**：导入初始化的实际源读取、展开、发布属于同归档 ownership/context；保持现有限额/marker/布局和已签收 Reader 服务。 |
| Book/Chapter | db.Transaction 无 request context；new Create/Save URL，placeholder reread 三字段后整行 Save。 | **must-fix**：同 request-context item transaction、owner/target 复验、guarded owned columns、无 fallback resurrection。 |
| 所选分组 | ImportRequest 只传 primary；四 handler 在 Book 提交后 `_ = setBookCategories(s.db,...)`。helper 先 delete 再逐行 create，错误被忽略。 | **must-fix**：全 selected memberships 和 legacy primary 属于同 item transaction，失败不能报成功或留下部分关系。 |
| token handoff | direct 在 category 后 consume；storage helper 在 category 前已经 consume。 | **must-fix**：只有包括所选分组的整个 item durable 才 consume；失败原 token/prepared 可重试。 |
| rollback | new/placeholder 两路径均 defer 绝对 `os.RemoveAll(archiveRoot)`。 | **must-fix**：原自有实体补偿，未知替换目录/成员/外部 bait 必须保留，不能以安全拒绝为理由盲删。 |
| response/event | 失败的分组写入仍可能有 201/200 Book 和 bookshelf_update；storage raw durability 可泄漏 err.Error。 | **must-fix**：仅 authoritative durable items；固定安全内部错误，先前成功项独立保留/通知。 |
| parser/old data | 既有 format/encoding/rule、Prepared Matches、空 TOC、旧两文件 stage、数字 URL、旧卷/portable。 | **aligned controls**：不重新设计，不以迁移/重新上传代替修复。 |

已签收 `local-book-archive-filesystem-lifecycle-...`@125fd93 的对象是**已导入** Reader、refresh、
export/delete，不包括这里的新 ArchiveImportedBook allocation 与 importer rollback。
已签收 stage b2b32f7 绑定 cache/bundle，不证明后续 library/SQL/category 原子取消。
PUT metadata/category、batch category 合同也不覆盖 import confirmation，不能用其绿灯代替本项。
旧 local-book-import-catalog P0 的 broad category-compensation claim 只保留为历史记录；本项重新取证。

## 3. API 和逐项 durable 合同

1. 保持 JWT/active-user、LocalStore/WebDAV capability、body/multipart/文件/rule/200项限额和
   validation priority、请求字段、原 numeric URL 和正常 response shape。direct 为 201 Book shelf
   projection；storage 为 200 `{imported:[{path,book|error}]}`，不新增全批 transaction。
2. 正常 parser、格式、raw overflow、invalid token、stage read/write 的既有 400/413/500/item
   mapping 不变。纯 SQL/archive/resource durability 的内部失败：direct 固定 500
   `{"error":"failed to import book"}`；storage 200 的当前项固定 `error:"failed to import book"`，
   不泄漏 PathError、宿主路径、用户名目录、SQLite、资源成员或内部 callback 文本。
3. category 前置不存在/foreign 继续既有 400 `category not found`；提交期间 category identity
   或数据库读写失败属于安全 durability failure，不静默降级为空分组或创建孤儿关联。
4. 本项实际归档/资源 I/O 或 SQL commit 前观察到 caller canceled/deadline，停止本请求后续项，
   整体固定 500 `{"error":"local book import canceled"}`，不新增 499。direct 可以在安全交付时
   加既有 `importToken`，不伪造新 token。原 stage cancellation 文案仍归原 stage service 所有。
5. 每 item 的 Book、Chapter、所选 BookCategory、legacy primary 在同一 transaction；source/TOC/
   正文/资源已完整写到本请求独占归档并在 commit 前复验原身份。写入、重载或 commit 失败都不
   留当前项 row/partial relation，不返回或广播该失败项。正常失败也不能 consume stage。
6. batch 第 k 项失败不撤销 1..k-1 的 durable Books；普通失败按既有规则继续，取消整请求停止。
   response/event 只能含实际成功集合，之前成功项在后来取消时仍准确通知。零成功不广播空成功。
7. SQLite 已确认 commit 后才允许成功通知/consume。commit 后 ctx/consume/cleanup 受阻不得伪造
   回滚或失败、删除已 durable Book、吞掉成功事件；原 stage unknown newcomer 保留。仍不承诺
   崩溃、跨进程或 retry-after-commit 的 exactly-once。

## 4. 新归档原实体、资源、取消与失败补偿

1. 文件工作前检查 ctx。LibraryDir 是 configured trust boundary；从原 trusted anchor 初始化合法
   缺失目录，绑定 library/data/safe-user 的原 opened identity，不把 resolved user root 作为新信任根。
   stable/late symlink、特殊文件、祖先 replacement、不可用原权限 fail closed；不修复/chmod 老卷。
2. 新书目录必须 exclusive/no-replace 创建，同名普通书仍按当前 friendly `_2/_3` 规则选择下一名字。
   不覆盖现存目录、原文件、metadata、资源或另一请求新实体；ownership 来自实际创建并验证的
   inode，不是“Stat 不存在”或“已接收 inode”。并发同名导入只可各得独立目录或安全失败。
3. 创建起持有一个原归档 session，贯穿原文件、content、resource temporary/marker/publication、
   source/TOC、durable 前复验和失败补偿。no-follow/nonblock regular handle 实际读取/写入；
   检查后不得按绝对 pathname 重开到 replacement。仅最终文件合法 rename 可以消费原 held bytes，
   但新同名实体不能被覆盖/删除。原祖先变更不能被另一 helper 的重新 admission 接受。
4. 原 bytes、EPUB/CBZ entry count/entry bytes/total expanded/text/chapter 等现有预算继续有效，
   实际 bounded read/write/extraction copy 前后和每章节/SQL步骤观察 caller ctx；所有限额 +1 与
   MaxInt64 饱和探测不溢出。完整 parser CPU/regex/HTML DOM interrupt 仍独立 unknown，最终检查
   或只给 db.WithContext 不等于实际归档 I/O 已取消。
5. metadata 和 cache 相对字段、原文件格式及 EPUB/CBZ marker/generation/capability 不变；新公开
   archive directory/file 保持现有 755/644 语义，不直接套 stage 私有 700/600。private temporary
   可以 600，发布前按既有目标权限；不改变旧目录/文件权限或对外 Reader 能力边界。
6. 失败先回滚当前项 SQL，只回收原 session 确认由本请求创建的 files/empty directories；递归回收
   需要原父 detach/no-replace 与原 tree identity/owned成员证明。未知 newcomer、额外成员、被
   替换的目录/link/special/外部 target 全保留。不能绝对 RemoveAll 当前同名书目录、user/data/root，
   也不追踪 rename 到其它 namespace 来修复。安全补偿不使用已取消 ctx 跟随未知对象；残留
   不伪称自动清理完成，不为它扫描/删用户历史目录。
7. 即使 allocation 后原文件写失败也要有 owned partial compensation。所有成功、取消、失败、
   collision、resource extraction 和 cleanup 路径关闭自有 fd；正常 cleanup 不泄漏 handles。
   无法强行中断 native syscall、进程崩溃、掉电/fsync/跨 FS 原子性不属于本合同证明。

## 5. Book、placeholder、分组与投影

- service 接收全 normalized requested category set；保留现有 stable positive dedup、categoryIds
  优先和 categoryId fallback。未选正 ID 的 new book 为未分组；placeholder 不选时保留已有
  memberships，explicit positive set 精确替换。空/省略不是新的“清空恢复书分组”操作。
- 在同 tx 重新验证 selected category 的 user ownership；关系删除/第一或后续插入错误、primary
  更新和最终重载错误均回滚当前 item。legacy primary 与 durable effective memberships 一致；
  旧 primary 只在无关系时 fallback，不能和非空 relations 合并制造 phantom 分组。
- placeholder 按 caller ID/source/URL/原 library 字段复验当前目标，不复活已删除/已重绑定目标；
  只更新本次 import 拥有的 parser/title/author/archive/catalogue 和 explicit category 列，保留
  当前其它 metadata、追更、时间语义和已有 progress/bookmark 既有 bookcatalog 重绑规则。
  不因“同名找到一个旧快照”Save 整行或改外用户同名书；两个歧义 placeholder 不误合并。
- 事务内得到 authoritative Book/effective relations，commit 后生成同一 shelf projection 用于
  response/event。失败重载不可空数组伪成功；不持久化 resource capability 或 host absolute paths。
- portable RestoreExisting 保持原 numeric identity、URL、category/progress/bookmark、format 和
  archive backup 规则；可以用显式 Background wrapper 支持非 HTTP consumer，但不绕开 shared
  原实体归档所有权。不能从 API ctx 绿灯推导 portable lifecycle 自动完成。

## 6. 必须先于实现的真实红测与控制

测试只使用 test-owned roots/SQLite/accounts；不改生产、不复制用户书库。nil-in-production seam
放在实际旧 allocation/write/resource/SQL/rollback 边界；每个 fixture 必须 fired、实际结果可观察、
worker 有界 join。编译失败、没触发 hook、睡眠概率、旧相邻绿灯不当 red。

1. 四 confirm 路由 × raw/token × new/placeholder：注入第一/第二 BookCategory create 和 delete/
   primary/最终 reload failure，证明当前 success、部分关系、消耗 token 或存续 Book；目标当前项
   rows/files/event 零新增或原 placeholder snapshot/progress/bookmark 保持，原 token 可重试。
2. 原 library/data/user stable link/special/000 和 admission 后 replacement：allocation/raw/chapter/
   metadata/resource 实际边界不得把 bait 写入外部或接受为 durable；外部 sentinel/hash/mode保持。
3. 新目录 Stat→mkdir collision、concurrent friendly name、raw write 部分失败和失败补偿前 same-name
   replacement/unknown member：只删自有实体，不覆盖/删除对手或历史归档，无 orphan owned handle。
4. 实际 raw/chapter/JSON/EPUB/CBZ resource read/write/copy 中和 SQL mutation 后 ctx 取消/deadline，
   不只 pre-canceled/final guard；回滚当前 item，保留 stage、旧 target 和前项已成功集合/通知。
5. placeholder lookup 后 delete/rebind 与无关 metadata/follow/category 更新；owner guarded updates
   无 fallback insert，不覆盖未提交列，positive selection 精确、空选择保留旧 membership。
6. controls：所有 formats/rules/GB18030/空 TOC、legacy prepared match no-reparse/miss fallback、
   mounted 源删除后的同 token retry、无分组/单/多/重复/零/foreign、两个用户同名、歧义 placeholder、
   多个批次的 earlier success、durable 后 cancel/consume 阻塞、旧两/三文件 stage与 FD/权限保持。
7. full Go/vet/focused+full race、frontend full/build、source-built Go+SQLite HTTP 和真实 Chromium
   1440×900/390×844/360×800 preview→confirm→shelf→精确 Reader，非 root native Linux两架构，
   同 SHA candidate fresh/historical/portable-v1/v2/backup/restart和 trusted Actions双架构发布。
   必须独立核验 exact/latest OCI index 和真实平台 config；本地成功不等于生产升级或设备签收。

## 7. 数据、实施顺序与仍未签收

无 schema/table/column/index/migration marker、API URL/field/前端 key、根目录、持久 cache/archive/
backup member 或环境变量改变。保留 data/cache/library/旧 URL/用户数据，不扫描、迁移、chmod、
删除或重建用户历史归档，不要求消费者本地编译镜像。

顺序：独立 contract/matrix/API/data/security 提交推送 → old app 精确 actual reds 独立提交推送 →
shared service/rooted archive/context tx/thin API →全部相邻与真实门→coherent candidate提交推送→
trusted Actions正式镜像→独立OCI→设备验收。故意 red 只放 feature，不能推 main发布。

允许差异：Go/SQLite、多用户、guarded columns、原 opened ownership、bounded I/O/ctx、安全内部错误、
失败保留未知实体和 durable 成功不伪回滚；不改变上游导入/分组/逐本成功可见结果。
本项测试、实现、全量/runtime/发布仍全部 pending。完整 parser CPU、跨进程 exactly-once、扫描
cardinality/depth/FD 总预算、桌面 click/wheel、dependency advisory、整体审计仍未完成。
本盘点时 Git 9cb6a03、已发布 Docker b2b32f7/index ed57cc69…、生产最近只读 db1ea21 分别记录；
没有生产升级/写入，原已恢复阅读故障不重开。
