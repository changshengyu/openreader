# WebDAV / LocalStore 目录创建生命周期固定基准第二轮合同（P2）

状态：**implemented / regression-validated / Docker-published / awaiting-device-verification**。
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
审查实现：`5338f261002bd8f234414c8f8e14ac89068ebab5`。本阶段只改合同，不改测试/应用。

## 权威行为与当前映射

- 固定 `WebdavController.kt#webdavMkdir` 316–331：在 caller home 创建请求目录，`mkdirs()`
  包含缺失父层；已存在路径仍 201 空 body。上游忽略 mkdirs 的 false 结果以及目标类型，不复制其
  “已有 regular 或创建失败也假成功”缺陷。OpenReader 既有 directory 幂等 201、regular 409、
  ancestor 非目录 409、root/symlink/special 403、一般 I/O 500 均保持。
- 固定 `BaseController.kt#getUserWebdavHome` 275–290：按 namespace 决定 home 并惰性 mkdirs。
  OpenReader 的管理员历史根/普通用户 `users/<safe-name>`、独立权限和 Basic/Bearer 是允许适配。
  新卷中缺失的配置根和私有根仍能初始化，不以“先手工创建根”缩小兼容范围。
- 固定 `BookController.kt#uploadFileToLocalStore` 2337–2353：上传选择的目录缺失时递归创建父层。
  当前 multipart admission、逐文件提交和旧文件保护合同保持；目录创建必须承接 request context。
- 固定 `BookController.kt#getLocalStoreFileList` 2387–2405：不存在的选择路径报“路径不存在”，
  不创建该子目录。当前 `listLocalStore` 却 Stat missing→Mkdir→200；这是未获许可的行为偏差，
  **must-fix**：根惰性初始化可保留，显式不存在的子路径须 404 且不创建。
- 固定 `LocalStore.vue` / `WebDAV.vue` 无可见 create/rename 控件。REST directory/rename 为旧客户端
  兼容入口，不新增 UI。`POST /api/local-store/directory` 仍接收 `{path,name}`，成功
  `201 {path}`；目标已存在 409，非法路径 400，权限/身份 403/401，bounded single JSON 保持。

当前共享服务 `webdavfs.Service.EnsureRoot/Mkdir`：Resolve/逐组件 Lstat 后使用绝对
`os.MkdirAll`，末尾再 rejectSymlinks；调用不传 context，工作期间没有 original root/ancestor
identity 绑定。调用范围为 `webDAVFileService` / `webdavMkcol`、`localStoreFileService`、
`uploadToLocalStore`、`createLocalStoreDirectory`，以及待移除的列表自动创建分支。章节缓存和导入
只继承其原服务/业务语义，不额外改变 cache CAS、source-independent token 或 parser。

| 点 | 当前证据 | 裁决 |
|---|---|---|
| MKCOL 正常与递归 | MkdirAll、existing directory 成功 | preserve；双前缀 201 空 body、空白/中文名、旧 URL 不改 |
| cancellation | MKCOL / root / LocalStore creation 未传播 request context | must-fix；授权后取消不得再创建根、父层或目标 |
| root/ancestor 校验到使用 | 只验证绝对路径，MkdirAll 可沿替换后的名字工作 | must-fix；opened original anchor/parent，相对句柄创建和前后 identity 复验 |
| 已有实体 | 正常 mkdir 不应删除/chmod/覆盖已有目录/文件 | preserve；newcomer 不给覆盖/清理许可 |
| 初始化 | 配置 boundary 本身可缺失；user root 可缺失 | preserve；受信配置路径现存 anchor 与新层必须显式接收，不退回绝对 MkdirAll |
| LocalStore missing list | 自动创建并返回 200；固定上游报错且不创建 | must-fix；缺失子路径 404，与合法空目录200区分 |
| 可见组件 | storage UI 没有创建/改名控件 | preserve；此次不添加控件、不重开已签收几何 |

## 文件系统、取消与数据合同

1. 授权/admission 先于初始化或创建。新增 request-context 入口，既有内部 background 包装可保留；
   WebDAV/LocalStore 请求所有目录创建和 root 初始化必须使用同一 request context。
2. 对已有 configured boundary，打开 original root、逐层 no-follow parent 并保存 identity。
   对新卷缺失 boundary，只在服务端配置路径中选取已存在的可信配置祖先作为 opened anchor；
   所有待创建层（含 boundary/private users/user/requested path）均在该 anchor 下逐组件执行。
   不把客户端 path、EvalSymlinks 的结果或校验后出现的 symlink 作为新信任根。
3. 既有已接收目录保持 inode、权限与内容，不为写入方便 chmod；regular/special/symlink 的既有
   错误映射保持。工作阶段 root/私有祖先/parent 被同名 real dir 或 symlink 替换均 fail closed。
4. 创建相对已打开 parent，禁止覆盖任何既有名字。请求新建节点的身份须记录；正常完成时验证
   全部可见链仍是接收的目录。不声称能禁止任意外部 actor 在最后 syscall 后修改文件系统。
5. 提交前取消/失败停止后续创建；只可从 original opened parent 中回收本请求新建且身份仍一致
   的**空目录**，自底向上，不递归删除、不 chmod、不触碰未知实体。无法安全收回则保留新层；
   这不等于授权覆盖/删除同时加入的未知子文件。可以用 private staged missing-chain + no-replace
   publication 减少活动路径中的半完成层，但不可用仅创建最后一级来丢弃递归合同。
6. 外部 cancellation 不新增 WebDAV错误正文或虚构201；LocalStore沿用client-safe取消/I/O错误。
   unsafe WebDAV403空 body、LocalStore400固定invalid path；不暴露配置anchor/host路径/credentials。
7. `GET /api/local-store?path=<missing-child>` 返回 `404 {error:"local store path not found"}`，
   不创建子目录，不写 DB/event。合法已有空目录仍200；根可按既有约定初始化后200。允许的
   REST/JSON translation 不把上游读取报错转换成新的持久副作用。
8. 不改 SQLite/schema、配置变量、备份成员、mounted data/cache/library 或管理员/用户布局。
   不启动扫描、迁移、清理或改写历史目录。目录创建不触发 PUT 专属 ReadingProgress ingress。

## 红测与完整门禁

合同提交后先补确定性旧实现红灯：已授权已取消 MKCOL（两前缀、已有根/新私有根、单层/递归）
仍201且创建，以及 LocalStore missing list 200并创建。后者无取消请求，证明上游可见语义偏差。
夹具不得让 auth DB 提前失败而绕过实际 handler/service；禁止以没有触发的工作阶段 hook 声称
旧实现的 identity race 已复现。原实现如不调用 context seam，记录该限制，实施后 fixture 必须 fired。

然后测试 root/anchor/users/user/parent 验证后 real-dir/symlink 替换、newcomer target、创建中取消、
未知子成员保留、权限拒绝、existing directory幂等、regular409/non-directory409/unsafe403、
spaces/中文/多层父目录、fresh configured boundary、管理员历史根/双普通用户、无权限零初始化。
LocalStore directory/upload-parent/list404与 token/cache相邻合同单独覆盖，不从MOVE绿测推导。

实现后至少 Go full/focused race/vet、frontend full/build、Compose、双架构服务编译、非root Linux
目录/权限/身份运行测试、真实 Basic/curl MKCOL两前缀和LocalStore HTTP副作用探针；无 UI改变不
新增工作台控件。候选仍须可信 Actions native/fresh/historical/portable/backup/双架构发布门。
GET/PROPFIND/Open 的独立读取/列表生命周期仍属未来审查，不能据此宣称全WebDAV模块完成。

## 旧实现红灯（2026-10-08）

合同 `d06f954` 后，`webdav_directory_lifecycle_contract_test.go` 在未修改应用的 `5338f26`
实现上运行：两前缀×existing-admin/fresh-admin/fresh-member×单层/多层共12项取消MKCOL均返回
201空body并创建目录；fresh情况还初始化了原本缺失的根。直接已授权Gin handler明确排除了认证
查询因取消提前失败的干扰。LocalStore directory的fresh/existing两项同样在取消后创建并201原JSON。
管理员/普通用户×recursive0/1共4项非取消missing list均200空列表且创建missing/child；这是真实
上游可见偏差，不是取消导致的附带错误。总计18个确定性失败，证据日志
`/private/tmp/openreader-directory-red.log`。实际源文件路径见仓库API测试，不依赖日志长期存在。

正常fresh管理员/普通用户根、双前缀递归/重复MKCOL与LocalStore根列表控制组不失败；不能为了
让取消红灯通过而禁掉新卷初始化或递归父层。没有工作期race夹具触发证据，不声称已确定性重现
root/parent替换漏洞。当前仍是tests-and-implementation-pending，应用没有改动。

## 实施与验证记录（2026-10-08）

合同 `d06f954`、确定性红测 `d5f00bc` 后实施 `rootedfs.CreateDirectories`。原configured root
或缺失配置根的最近现存配置ancestor从第一次Lstat即绑定original inode，打开root并逐层no-follow；
随机private目录在original parent fd创建，绑定opened inode后no-replace发布到请求component。
工作阶段复验root/全部已接收parent，取消/失败仅逆序unlink当前仍为owned inode的空目录；没有
绝对MkdirAll、递归RemoveAll或chmod既有目录。opened新节点持有到回收结束，owned-stat复验/
unlink无需再分配fd，不因回收额外open失败而误取其它名字。

正常并发创建时，no-replace遇到EEXIST只可重新接收同一original parent下的safe directory满足
幂等；不覆盖该实体、不将其记录为本请求owned cleanup。文件仍409，symlink/special拒绝。
实现期间的变量遮蔽曾使EEXIST分支误报missing，已修正，并保持16并发×10轮真实断言，不删测试。
existing final目录000无需内容读取而幂等，保留inode/mode；只读parent失败保留权限和空内容。

EnsureRootContext/MkdirContext贯穿WebDAV和LocalStore请求；内部background包装继续供既有
cache/stage调用，不改其补偿或CAS。LocalStore missing-child list已移除Mkdir分支，404固定JSON；
根惰性初始化、正常directory201/重复409、multipart父目录创建保持。LocalStore元数据name已有
trim-outer-whitespace合同不改；raw MKCOL与rooted helper原始空白名继续保持。没有增加UI控件。

18个旧红灯转绿；两前缀实际Gin/Basic-Bearer的工作期root替换fixture **fired=true**，403空body，
original/external均无新节点。实际multipart父目录创建阶段取消fixture fired=true，无新文件。
root/parent/users/user/fresh-anchor在接收后被real-dir或symlink替换均fail closed；stage换实体、
newcomer file/link/directory、第二层stage/install后取消、未知成员保留、fresh层/中文空白名、
existing000、readonly555和16并发幂等有本项测试，不从MOVE绿测推导。

rootedfs目录合同重复10轮、rootedfs/webdavfs全包race、目录API race通过；更宽WebDAV/LocalStore/
ChapterCache API race通过（176.296s）。Go full/vet、frontend762/762/build、Compose、Linux双架构
服务编译和非root Linuxarm64真实目录/权限/完整WebDAV服务测试通过。最终owned-stat版本又通过
Go full（API79.474s）/vet、rootedfs/webdavfs全包race、目录API race（19.258s）、刷新后的Linux
双架构编译/非root实测与最终二进制全新临时卷HTTP复验，均已核对exit0。

隔离真实Go/SQLite Basic/curl协议和LocalStore新探针已通过：recursive/idempotent MKCOL201、
regular409、PROPFIND207及相邻PUT/COPY/MOVE/DELETE/LOCK；missing404/no-child-write、lazyroot、
directory201/重复409、中文/internal-space、upload-parent和exact download bytes。首个诊断服务
已停止；最终二进制在独立fresh临时卷同样通过两份协议探针。此切片无前端交互/几何变化，不添加或重复Reader控件截图；
真实协议和状态副作用是本项运行门。trusted native/fresh/historical/portable/backup/双架构发布
仍待新候选，红测run37738124537的backend失败符合预期且没有发布镜像。

原生产书籍故障已device-verified关闭；本轮再次health确认production commit `db1ea21`，不把本地通过记作部署。
整体固定基准审计、独立GET/PROPFIND/Open动作未完成；npm同一锁文件当前报告8high，未升级依赖
或改锁，需另行authoritative advisory与实际可达性inventory，不据metadata宣称生产可利用。

## 2026-10-08 可信 Docker 发布签收（覆盖上文 publication-pending）

实现 `08de4decfb91248901ca73adee5f102563181d68` 的 Actions
[`37740704252`](https://github.com/changshengyu/openreader/actions/runs/37740704252) 已终态success。
backend/frontend/build/Compose/native、fresh/portable、historical/backup以及最终发布平台门全部通过。
本地再次只读核验exact tag `ghcr.io/changshengyu/openreader:08de4de` 与 `latest` 为同一OCI index：

- index：`sha256:7ab7cb27c3f1a114e0987f6f4c7b1100a3ebda2f228b73cc3e1ee144b046a08c`
- linux/amd64：`sha256:a82c31ed9abfe5a712de9c3c421e2ba3b32cb742588b7812735c1db1052ea491`
- linux/arm64：`sha256:f936e04a6f8a9ebff00b94b146a8298b7ed79515f482dcac3c3de3a4adbdb4fa`

没有重新启动已有构建，也没有升级另一台Mac。生产health仍是
`db1ea216f9849bc44a90b5b760241df1c6d069b0`；用户已确认的原书恢复保持关闭。
允许差异为Go opened-fd/context安全适配、multi-user隔离及缺失子路径读操作不写盘；无schema/
配置/layout/backup变化。独立read/list合同和30项新红测尚未实施，整体重构继续，不能记作全模块完成。
