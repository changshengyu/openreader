# WebDAV COPY 文件系统生命周期固定基准第二轮合同（P2）

状态：**inventory-complete / red-tests-and-implementation-pending**。
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

## 权威行为和差异

固定 `WebdavController.kt#webdavCopy`（430–464）在当前 namespace 的 WebDAV home 内递归复制
文件/目录。source 不存在 412、Destination 缺失/不可解释 400；既有 destination 且没有 Overwrite
值 412；显式覆盖先删除旧 destination，再 copyRecursively，最后 201。上游覆盖删除前没有事务保护，
不能把 OpenReader 的 staged failure preservation 记作上游自身的保证。

OpenReader 双前缀共用 `webdavTransfer`，Destination 已支持绝对 URL、双前缀/旧相对路径；只有
`Overwrite: T` 开启覆盖，是已签收的协议/数据保护适配。本轮不改变认证、私有根、Destination
解释、无 body、状态码或 COPY UI，也不让复制 progress JSON 产生 PUT 专属的进度接收副作用。

| 生命周期点 | 当前事实 | 裁决 |
|---|---|---|
| source/destination 初检 | validTransfer 做 Resolve/Lstat 并拒绝 root、自身、父子包含、symlink 与特殊文件 | preserve；初检不是工作阶段身份凭证 |
| source 遍历与读取 | copyTree 对完整绝对路径 Lstat/ReadDir/Open，file handle 只与紧邻 Lstat 比较 | must-fix：ancestor 或真实目录替换能把后来实体当成原 source，读取不能离开 current admitted tree |
| destination stage | 在绝对 parent MkdirTemp，递归创建 new，后续发布仍按绝对名 | must-fix：上传/复制期间 parent 改变会追随替换实体，可能根外写入/发布 |
| stage cleanup | defer os.RemoveAll(stageDir) 沿先前字符串定位 | must-fix：不能删除同名替换实体或在替换 ancestor 下清理，失败应只处理本请求拥有的 stage |
| overwrite | installTransfer/replaceByRename 在工作后重新 Lstat/rename，不绑定初始 final 的 identity；恢复也普通 rename | must-fix：初始目标被替换时不能删除新实体，恢复不能覆盖后来 final；失败保存 source、旧目标、同期 newcomer |
| caller cancellation | COPY 已传 context，tree/copyContext 边界检查，但 publication/cleanup 无 current cancellation 与 identity 资格 | preserve + must-fix：最后复制后取消不得发布 |

MOVE 同样仍使用旧 absolute replaceByRename，且 API 未传 caller context，已记录未覆盖；不从 COPY
整改推导 MOVE 已修复，也不借本轮扩展到 PUT、DELETE、目录 GET 或 WebSocket 协议。

## 目标合同与允许适配

按 caller 的 scoped root/boundary 打开源树与目标 parent，目录遍历、regular read、staged tree、
publication、compensation 与 cleanup 统一使用 admitted opened handles。复验 root/所有 ancestors、
source tree/子节点、destination parent、原 destination、stage 身份；变更 fail closed，API 403 空 body。
保留正常文件/目录复制、覆盖、权限、空文件/空格名、source 原字节、201 空 body；缺失 source 412、
missing parent 409、目录类型冲突、无覆盖 412 等走既有错误映射。

copy failure/取消不发布部分树，不删除 source、旧 destination 或同期新 final。覆盖采用 no-replace
install/recovery；若新 final 阻止旧字节恢复，保留可恢复 quarantine，并记录诊断，不静默删除旧字节。
不增加 SQLite/schema、配置、备份成员，也不清理用户 data/cache/library。opened-handle 与 no-replace
属于明确允许的安全/数据保护差异，不能声称任意外部 actor 的改动具有跨文件系统原子性。

## 下一步红测和门禁

先以测试 context 的复制阶段边界触发确定性替换，不在生产卷试验：source ancestor/目录、
destination parent symlink/真实目录、原 destination regular/目录/symlink、stage identity、最后复制后
取消、同期 final 抢占。必须在旧实现重现根外读取/发布、覆盖新实体或错误清理，再实现。

文件/递归树正常复制、fail preservation、双前缀 Basic/Bearer、普通用户私有根和当前 PUT/DELETE/
progress ingress 相邻回归必须通过。Go full/race/vet、frontend/build、实际隔离卷 Basic/curl 与可信
fresh/historical/portable/Docker 双架构发布为完成门；无 UI 修改时不重开已签收 Reader 几何。
