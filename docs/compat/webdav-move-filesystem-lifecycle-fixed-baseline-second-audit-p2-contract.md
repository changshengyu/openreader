# WebDAV MOVE 文件系统生命周期固定基准第二轮合同（P2）

状态：**inventory-complete / red-tests-and-implementation-pending**。
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

COPY `463b487` Actions `37189091695` 仍在发布；本轮合同文档允许先推送，红测/backend 修改推送
等待该 run 终态，以免 workflow 的 cancel-in-progress 取消已验证候选。
