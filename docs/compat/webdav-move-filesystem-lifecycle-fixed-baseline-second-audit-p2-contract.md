# WebDAV MOVE 文件系统生命周期固定基准第二轮合同（P2）

状态：**implemented / regression-validated / Docker-publication-pending**。
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

## 权威行为与调用范围

固定 `WebdavController.kt#webdavMove`（394–428）在当前 namespace home 中将 source rename 到
Destination；source 缺失 412、Destination 缺失 400、既有目标没有 Overwrite 值 412。覆盖先删旧
目标，再 renameTo，201 空 body。上游忽略 renameTo 的失败返回，不提供失败保护或取消保证；
OpenReader 已接受严格 `Overwrite: T`、错误映射及失败保留字节的安全/数据保护适配，不复制这个缺陷。

OpenReader 双前缀 `MOVE /reader3/webdav/*path` / `MOVE /webdav/*path`，Basic/Bearer、caller
私有根、Destination URL/旧相对路径解释、无请求 body/新 query 和 201 空 body 保持。missing source
与不允许覆盖 412、missing parent 409、unsafe/root/自身/父子包含 403、无效 Destination 400 保持。
移动进度 JSON 不产生 PUT 专属进度接收、SQL 更新或 WebSocket 回声。

同一 `Service.Move` 还由 LocalStore rename 和远程章节缓存 stage/backup/publish/restore 使用。
这些共享调用必须纳入回归，不能把 WebDAV 修复与旧缓存/本地目录兼容割裂。保留旧内部 Move 方法
作为 background-context 包装，新增 context-aware 入口供 WebDAV/LocalStore 请求使用；缓存补偿不因
原请求已取消而跳过恢复。不得改变章节缓存业务 CAS/数据库事务、旧路径或 LocalStore JSON。

## 生命周期差异

| 点 | 当前实现证据（COPY 实施 `463b487`） | 判定 |
|---|---|---|
| source/target/root 初检 | validTransfer Resolve/Lstat 后仅返回绝对字符串 | preserve admission；must-fix 工作阶段仍须绑定 original root/ancestor/source/target |
| caller cancellation | WebDAV 不传 request context；Service.Move 无 context 参数 | must-fix：授权处理阶段已取消仍移动；提交前取消必须保持 source/old target |
| missing target install | os.Lstat 后普通 os.Rename | must-fix：同期 target 出现会被覆盖；必须 no-replace |
| overwrite detach/install | absolute MkdirTemp、target→backup/source→target 普通 rename | must-fix：source/parent/target 替换后可操作后来实体；同源 inode 原子 rename 不能变成 copy/delete |
| failure restore | backup→target 普通 rename | must-fix：不能覆盖 newcomer final/source；恢复受阻保留 admitted 字节 quarantine |
| cleanup | defer absolute RemoveAll(backupDir) | must-fix：不得删除同名/ancestor 替换实体，旧树清理只限本请求 admitted nodes |
| normal tree move | directory rename，不遍历读取 descendants | preserve：nested symlink/special 随目录 inode 移动而不被跟随，源顶层仍 regular/dir only |

## 目标合同与允许差异

从同一 trusted boundary 打开两侧 ancestor chains、source 和旧 target，按 handles 做同 inode rename，
复验所有已接收节点，保留权限/原字节/目录/空白名；不把目录中的 symlink 解引用或拷贝其外部内容。
工作期间 source/target/root/ancestor 身份或已接收 metadata 变化 fail closed（WebDAV 403 空 body）。
允许请求内确认的 rename ctime 更新，不整体放宽共享 inode 检查；历史硬链接正常 MOVE 保持 source
inode 为 final，旧同 inode target 的删除仅减少原旧链接，不破坏已发布的新路径。

源的身份凭证是顶层 inode/metadata 与两侧 opened parent/ancestor，不进行源树内容读取或复制；
这是固定上游 directory rename 的语义，不把 COPY 的完整内容遍历作为 MOVE 许可条件。普通文件
或目录不可读但父目录允许 rename 时，正常移动仍保持同 inode 与原权限（包括 000/555）。源树
descendant 没有被打开、读取或独立删除；并发 descendant 自身修改随同一 directory inode 移动，
不声称移动操作能锁住或还原外部 actor 对既有 descendant 的修改。
需要删除的旧 target 树另行逐节点接收并复验；regular 旧文件只做 no-follow metadata 接收，
无需读取其字节。旧目录若权限不允许安全接收成员，则在移开 source/target 前失败保留两者，
不 chmod live 目录来规避接收限制；owned quarantine 清理阶段的 chmod 不影响 live source/final。

取消/失败在新 final 完整提交之前，不发布部分树，不丢 source/old target/newcomer。临时 claim、旧
target detach、install 与 compensation 都 no-replace；不能恢复原名时保留可恢复 source/old-target
quarantine，不能用 copy/delete 伪装跨文件系统原子 move。预先检查明显 cross-device destination，
剩余 rename 错误必须安全补偿；权限错误不 chmod source 或已有 live final 来取得许可。

完整 source inode 已确认发布后，仅清理原旧目标；未知实体或清理失败保留剩余 quarantine，WebDAV
返回 201 空 body 与 `X-OpenReader-WebDAV-Cleanup: pending`，不伪称 MOVE 未提交/不回滚新树。
LocalStore 同种已提交诊断保持 200 原 JSON 并加该固定头；缓存无覆盖调用不进入旧目标清理分支。
不声称已成功删除的旧成员能够跨文件系统回滚，不声称可禁止任意外部 actor 在最终 syscall 后改名。

不新增 SQLite/schema、配置、备份成员或根目录，不启动扫描、迁移、清理 data/cache/library，旧
regular WebDAV/LocalStore/cache 路径继续有效。opened handles、no-replace 与 cancellation 是明确
允许的安全/数据保护差异；不能从 COPY/PUT/DELETE 通过推导本项已经对齐。

## 红测与门禁

先重现授权 handler 在已取消 request 上仍返回 201 且移动 source 的确定性红测。身份变更 fixture
必须有已接收后/最终 publication 前的确定性边界；如果旧无 context 实现无法注入该边界，应明确
报告该证据限制，不能将未触发 fixture 记为已证明漏洞或已有修复。

之后覆盖 source/ancestor/target/parent 替换、no-overwrite newcomer、detach 后取消/compensation
抢占、未知 cleanup、硬链接、不同父目录/只读权限、空文件/目录/空白名、nested symlink 根外字节
不变、两前缀/认证/私有 user，以及 LocalStore rename、历史章节缓存 publish/restore 相邻测试。
Go full/race/vet、frontend/build、实际隔离 Basic/curl 与可信 fresh/historical/portable/backup/双架构
门禁为完成条件；无 UI 结构变更时不重开 Reader 已签收几何。

合同/红测阶段，COPY `463b487` Actions `37189091695` 尚在发布；合同文档先推送，红测/backend
推送等待该 run 终态，以免 cancel-in-progress 取消已验证候选。该 run 后续已成功，红测已推送。

## 红灯证据（2026-10-04）

合同 `ff1fbfa` 后新增 `TestWebDAVMoveCancelledAfterAuthorizationPreservesBothPaths`：使用既有
测试用户的已授权 storage context，直接运行真实 MOVE handler/service，避免“取消导致认证 DB
读取失败”提前短路。两前缀×regular/directory×new/overwrite 八项全部失败；实际返回 201 空 body，
original source 路径消失。此证据证明授权后的取消未阻止提交，不用于宣称尚未注入的 identity race
已经被旧实现确定性重现。取红灯时应用代码尚未修改；红测推送在 COPY run 成功后进行。

另新增 `TestWebDAVMoveAdmittedBoundaryRejectsTargetChangeOrCancellation`，以未来 source claim
存在作为已接收边界。旧实现两项均 `fired=false, status=201`；这仅表明尚无边界支持，不作为
旧实现目标替换竞态已经重现的证据。实施后必须实际触发并断言 403/取消、source 与 current final
字节保存；不能删除 fired 断言以使其假通过。

## 实施与回归（2026-10-04）

合同 `ff1fbfa`、红测 `9b6a5f6` 后实施 rootedfs.MoveTree：同一 trusted boundary 打开两側
ancestor/parent，源顶层 metadata/inode claim，旧树独立接收，no-replace detach/install/restore 与
owned-only 旧目标清理；无源内容读取、copy/delete 或 source chmod。regular/dir 本请求 rename
的已确认 ctime 变化同步到旧 target 硬链接快照，外部修改不整体放行。删除旧 absolute
replaceByRename/MkdirTemp/RemoveAll，内部 Move 保留 background-context 包装，WebDAV/LocalStore
入口传 request context。缓存 publish/rollback 的数据/CAS 与普通/历史 NULL 布局不变。

八项授权后取消红测转绿；两个 source-claim 边界 fixture 在新实现 **实际 fired=true**，分别验证
target 替换 403 与后续取消的 source/current final 字节保护。额外覆盖接收后 source/parent/target
变更、source/old claim 替换、恢复 source/final 抢占、提交后未知旧成员、普通用户私有根和有效进度
JSON MOVE 零接收副作用。三路真实 Gin→Service→MoveTree 提交后注入 unknown，WebDAV 两前缀
201 空 body、LocalStore 200 原 JSON 均附固定 pending 头，保留完整新文件与未知/剩余旧成员。

发现并修正的是测试 Context 的并发 Err 标志：认证 SQL Rows 后台也调用 Err，旧夹具不满足
Context 的并发接口合同。互斥保护一次性注入、原子观察 fired 后，COPY/MOVE/LocalStore/cache
API race 重跑通过；不把这个夹具 race 宣称为生产业务 race。旧目录 000 的 no-follow open 按
既有 helper 映射为 unsafe，不要求裸 EACCES；原 inode/bytes/permissions 与未 detach 断言保持。

Go full/vet、rootedfs/webdavfs 全包 race、COPY/MOVE/LocalStore/ChapterCache API race、frontend
762/762/build、Compose 与 Linux amd64/arm64 服务交叉编译通过。非 root Linux arm64 实际运行
覆盖只读/不可读源、不可读旧 regular/旧目录拒绝、nested FIFO/links、硬链接、恢复与 pending。
双 tmpfs 实测 EXDEV 在任何 claim 前保持两侧 inode/bytes；测试环境变量只为诊断 fixture，不是
应用配置或迁移。

最新隔离 Go/SQLite Basic/curl 协议 smoke 验证文件、目录、空目录、覆盖、无覆盖、原名消失、
跨 parent MOVE 与相邻 COPY/PUT/PROPFIND/LOCK/UNLOCK/DELETE；LocalStore 真实 HTTP 上传→覆盖
改名 200 原 JSON→下载 exact bytes→旧名 404 通过。三视口真实 Go/SQLite/Chromium 相邻 Reader
多客户端/CAS/WebSocket/cold restore 与外部进度无回声全部通过，没有 mock API。隔离服务已停止。

Docker 本实施候选尚待可信 workflow fresh/historical/portable/backup/published-platform 门与
exact OCI digest。COPY `463b487` run `37189091695` 已成功发布；MOVE 红测 run `37190626551`
失败符合预期，不是发布候选。生产 health 仍是 `db1ea21`，未远程升级；整体固定基准审计未完成。
