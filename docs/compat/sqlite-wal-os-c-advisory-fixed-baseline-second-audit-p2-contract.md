# SQLite WAL 与 OS/C 依赖固定基准补充合同（P2）

2026-10-16；固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
提取时Git/main `24e55bab922e6c55206ef0a64d7fd64c02374da4`，已发布应用
`fc4692c474df30d1f0427cba57e7ebc67c91b156`，OCI index
`sha256:74ef48fd0c9f5547fff9787fb97763fff22e3500a376be05d5379f19dd8af765`。
状态 **inventory-complete / contract-only / behavioral-tests-pending / implementation-not-started**。
以上为提取时快照；当前第一批实际反例和测试阶段进展见第6–9节，最新状态以第9节为准。
前端维护交付保持，不把npm/Go扫描当作C、OS或数据库持久性问题的覆盖。生产另一Mac不操作，
原书已恢复事件关闭；本合同不声称发现生产数据库损坏或远程利用。

2026-10-22：OS APK 门在发布时读到 musl 1.2.5-r24（CVE-2026-107891）待修复项，按本合同 zlib 先例在构建期安装签名
精确修复，见
[`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)
第 11 节；本合同的门设计与结论不重开。

2026-11-10：OS APK 门在发布时读到 zlib 1.4-r0（CVE-2026-4738）待修复项，签名索引也已不再提供 1.3.2-r1，按同一先例在构建期安装
签名精确修复，见 [`os-apk-zlib-1.4-release-blocker-p1-contract.md`](os-apk-zlib-1.4-release-blocker-p1-contract.md)；本合同的门设计与结论不重开。

## 1. 上游产品语义与已签收适配

MAIN读取精确fa22的 `WebdavController.kt:726–823`：登录及secure下enable_webdav权限、
GET query/POST body路径、用户WebDAV home、ZIP格式、恢复及备份的可见成功/失败；备份复用
最新Legado ZIP，恢复调用namespace的syncFromWebdav。
`BookController.kt:2099–2255`相关sync/save函数：ZIP中bookSource/bookshelf/bookGroup/
rssSources/replaceRule/bookmark JSON及bookProgress数据对应当前namespace；备份写回逻辑数据。
这不是要求OpenReader复制上游物理删除/路径拼接或容错缺陷。

OpenReader的SQLite/GORM、事务、多用户授权、rooted文件路径及portable-v1/v2为既有运行时/
安全适配。`backend/db/db.go`启用WAL、busy_timeout5000、synchronousNORMAL；不限制单连接，
现有连接池合同实际持有两个连接。`backup_restore.go`的有界ZIP、`backup_restore_plan.go`的
已识别JSON工件/预校验/事务恢复，不是直接打开上传的任意SQLite DB。现有settings/source/
progress等UPSERT为固定列/绑定值；尚不能以局部检索宣称所有SQL路径已审计。

必须保持所有API/认证、进度/书签/分组、旧numeric Reader URL、本地归档、cache/library/data、
WebDAV/backup、浏览器key及已签收原实体I/O/取消/补偿合同。没有schema、存储格式、secret/
token迁移；不禁用WAL、不改成单连接串行化、不复制裸db文件来代替已签收逻辑备份。

## 2. 真实版本、精度与差异矩阵

当前go-sqlite3 **v1.14.22**，C amalgamation **SQLite3.45.1**。自有`:memory:`实际宿主查询
sqlite_version/source_id/compile_options：FTS3/FTS3_PARENTHESIS/RTREE/THREADSAFE1，无FTS5。
两份真实发布Linux binary含同一3.45.1 source ID；静态标记FTS3/RTREE存在、FTS5不出现。
静态字符串只作交叉证据，不冒称已对实际binary执行compile_options SQL。还需Linux同构建
条件的原生探针，不能用宿主Go扫描、包版本或未编译的C分支宣称所有C风险清除。

| 面 | 已读证据与适用性 | 裁决/实施及验证约束 |
| --- | --- | --- |
| WAL-reset | [SQLite官方WAL说明](https://sqlite.org/wal.html#walresetbug)：3.7.0至3.51.2的罕见并发时序可能损坏DB；3.51.3及后续或指定回移分支修复。项目有WAL+多连接，满足必要运行条件，但未证明该时序已发生。 | **must-fix persistence maintenance**。最低候选driver v1.14.36/SQLite3.51.3；不能将Go race或普通并发绿灯当C竞态已覆盖。需要源补丁/真实runtime版本及有界确定性时序证据；若未能触发，诚实保留未复现，绝不把版本检查计实际损坏red。 |
| SQLite正常SQL正确性 | [3.45.2修复记录](https://sqlite.org/releaselog/3_45_2.html)列UPSERT索引一致性与空聚合NOT NULL过度优化；维护者论坛919c6579c8/440f2a2f17已读。当前app未观察到冗余REPLACE+双ON CONFLICT或此空聚合表达式。 | **conditional library / maintenance**。自有微型SQL库级反例及正常控制，不能叫当前API漏洞；不是通过可疑SQL损坏用户DB。当前固定单UPSERT/普通NULL与空表控制保持。 |
| FTS5/CAPI/恶意DB等报告 | [SQLite维护者CVE表](https://sqlite.org/cves.html)将SQL注入、可控DB、FTS5、zipfile/session扩展和CAPI参数等条件分开；部分编号并非SQLite缺陷。 | **conditional / audit-open**。不做整类CVE永久忽略；不添加FTS5/zipfile/Session或危险CAPI来制造可达性，不以“不支持任意SQL”替代逐入口授权/参数及恢复路径审计。 |
| zlib runtime包 | 官方Alpine v3.23 secdb main列CVE-2026-85091在1.3.2-r1修复；两个真实发布平台均1.3.2-r0。aports提交b92621a8及其补丁指向维护者df84af25：非阻塞gzwrite错误返回后重置输入状态。Go服务实际ldd仅musl，没有应用请求到C gzwrite的证据。 | **must-fix image maintenance / exploit conditional**。最低1.3.2-r1，同3.23系列；不称生产RCE。优先官方已修复base重建；当前tag/digest未变，若无该重建，允许仅在构建期安装签名验证的官方精确zlib APK，记录包/来源/两平台哈希。不可生产启动时apk upgrade、全量latest或关闭签名校验。 |
| 其余APK及构建C | 两平台16个安装包/10个origin，官方main/community记录已逐origin和原生apk版本比较；未列记录的5个origin不是clean证明。 | **inventory / remaining audit**。必须核对最终层没有覆盖包清单/库；复制的CA、嵌入SQLite、Node内置C和builder APK分开。不得将base安全数据当全镜像扫描，亦不凭Go/npm门签收这些范围。 |

WAL修复候选标签v1.14.36精确commit `4ea2a9fb44c7ecc6eee436d26a32ab34a79a87ce`，header
source ID `2026-03-13 10:38:09 737ae4a34738ffa0c3ff7f9bb18df914dd1cad163f28fd6b6e114a344fe6d618`。
Go校验下载Sum `h1:EyQAxPedU6e4/lGrYcT2+1yVcOLT7ERS6jvTYKzfzgY=`，最低Go1.19，当前
应用Go1.26.9不变。这里只验证标签/缓存，**没有改go.mod/go.sum**。该driver还涉及Rows.Close/
绑定清理等变更，须测试真实取消/复用/Rows.Close，不能当作只换C常量。GitHub release对象404
不代表标签不存在；标签、绑定header及Go校验和已各自核验。

v1.14.52/SQLite3.53.4仅作为最新维护元数据，不直接采用；3.53系列还涉及浮点文本等语义，
无依据跨到该系列不是本合同选择。若后续适用性或维护者证据要求高于最低候选，先补合同再改。

## 3. OS 取证结果与不能推出的结论

自有只读工具重新按Registry digest/size核验两平台manifest和所有已保存层，读取普通
`usr/lib/os-release`及APK installed DB；后续层仅应用/前端/CA，未覆盖APK DB或系统动态库。
官方secdb main最后修改2026-10-13T20:35:17Z，community为2026-10-09T18:09:41Z；原始JSON/
hash/包origin/所有比较保留，**没有使用字符串或semver词法代替apk版本规则**。

实际两平台安装图相同：当前secdb已列条目中，未达修复版本的是zlib1.3.2-r0→r1；这只是
这两个官方数据文件的比较结果，不是零漏洞/全面可利用性结论。5个未列origin（baselayout、
keys、alpine-base、CA、pax-utils）单列unknown；source-package origin而非安装子包名作为关联键。
初次盘点器读tar符号链接的空内容而失败，未冒称镜像版本错误；改读已核实普通目标，并显式
支持noarch。原失败日志保留，不把工具错误算漏洞red。运行期间的短命对比容器均自有/断网/无卷。

08:13:13Z官方Registry重新核验：当前alpine3.23.6、Go1.26.9-alpine3.23、Node22-alpine3.23
仍为既有三个pin；故“刷新同一个tag”不能自动补齐zlib。后续候选真实APK取证必须再验证。
主要证据：`/private/tmp/openreader-os-c-inventory-final.log`及其报告目录、
`/private/tmp/openreader-os-c-current-official-base-images.json`、
`/private/tmp/openreader-sqlite-host-inventory.json`、`/private/tmp/openreader-sqlite-1.14.36-module-metadata.json`。

## 4. 合同后独立测试，再实施

本合同和matrix/API/data/security先独立提交、推送、精确核验；然后才添加仓库测试。当前没有
新增SQL行为测试、Go模块/Dockerfile/workflow/应用变更，原图仍v1.14.22。

1. 自有worker只接触临时DB/内存DB，严格小表/短值/步数/输出/工作deadline；进入工作握手，
   超时/SIGINT/SIGTERM杀掉并Wait回收。C分配不能只靠Go软内存限制，必要时用容器硬限制。
2. 自有无秘密微型UPSERT/索引与空聚合NULL正确性反例；普通绑定值、Unicode/NULL/空表、
   单UPSERT、多行rollback/索引完整性控制。先重复确认实际old行为；已绿fixture算控制，不凑red。
3. WAL正常多连接/写入/checkpoint/reopen/完整性/并发取消控制，并调查确定性调度/测试VFS。
   不盲跑随机压力等待损坏，不向生产发SQL，不删除WAL/SHM，不靠篡改SQLite核心算法造red；
   调度缝只可test-only且必须证明旧/新都实际进入同一临界序列。不能安全重现则如实保留该项。
4. 若加OS/C发布门：先工具合同测试（命令失败/空/截断/未知schema/过期数据/漏origin/错架构/
   版本比较错误/层覆盖/未签名包/符号精度不足等均失败关闭），再实现；扫描退出0不等于无发现。
   维护者非阻塞gzip异常只允许自有受限worker中设计；不用下载的外部PoC或宿主大分配崩溃。
   APK版本/缺门仅算维护/availability red，不能计为实际gzwrite溢出。
5. 独立test/docs checkpoint推送后，才选择必要模块/精确APK和最小Docker调整；不改Go最低、
   GORM、auth/schema/会话、Reader预算/队列或现有frontend锁。保留原Go/npm精确门。

## 5. 后置签收范围

同一断言/预算的实际反例green；正常SQL/取消/复用/事务/WAL/恢复控制保持。全Go、关键包race/
vet、frontend788/build、两原有审计门、真实HTTP+SQLite/三视口、精确候选native两平台、旧DB/
原挂载目录/新旧卷/portable-v1-v2/备份/跨用户/重启，以及可信Actions双平台+独立OCI/配置/
真实binary/Csource与APK取证逐项终态。不得用SQLite版本常量、base digest或单一库测试支持
整库持久性或生产修复结论，不将“C无Go报告”解释为C安全。

允许差异仅已签收技术栈/多用户安全适配和必要持久性/依赖维护；任何备份格式或WAL/SQL运行
语义变化均须先另签合同。OS/C广度、完整parserCPU/scan/峰值FD/late-tx、设备及全量重构仍开放。
本轮未发布新镜像、未改变生产；上一fc4692c维护交付不重做、不倒填为本项已完成。

## 6. 独立合同后的第一批实际库级反例（2026-10-16，测试阶段未结束）

合同 `5bb35cf5d49f47c550e6083c78f0cbd664716581` 已独立推送并远端精确核验。首次GitHub
TLS握手中断不计推送成功；普通HTTP/1.1非force重试后成功核验，没有关闭TLS校验或改main。
随后仅新增 `backend/db/sqlite_c_maintenance_contract_test.go`；Go图/Dockerfile/frontend/
workflow/应用未改，仍为driver1.14.22/SQLite3.45.1。

自有0行/2行内存DB、固定短SQL/绑定Unicode值；parent进入工作握手后计时，startup5s/work2s/
overall10s/输出16KiB，取消/中断kill+Wait；worker不生成子进程。没有读取用户DB/文件、使用
外部可执行PoC或等待随机损坏。两项实际old失败：

- NOT NULL空聚合返回 `(0,0)`，应为 `(1,0)`，裸列在空聚合中实际为NULL。
- 冗余UPSERT后同一两行表扫描计数2、索引路径计数3，索引与表不一致。

普通绑定值/单UPSERT、普通NULL空表、WAL双连接rollback/checkpoint/reopen/原值和integrity
三项正常控制均通过。原样三轮每轮 **2实际库级red/3正常控制绿**，1.319/0.417/0.426s；
同代码race保持2/3（3.636s），无DATA RACE/启动失败/deadline/未回收worker。
完整db包既有12个root测试均通过，失败精确为新增两叶（包0.496s）；默认未设置case时worker
测试直接返回，不将其额外算正常业务控制。

固定官方Go1.26.9/Alpine Linux原生arm64/CGO1容器（768MiB/2CPU/128PID硬限，源码及module
cache只读，自有tmpfs，临时安装编译器）原样执行仍2red/3green，0.046s；实际版本由C接口
读取为3.45.1。该项是源编译测试探针，不是对发布应用添加SQL调试接口，也没有升级生产镜像。
容器已移除，日志 `openreader-sqlite-c-{initial,repeat-1,repeat-2,repeat-3,race,full-db}-red.log`
和 `openreader-sqlite-c-linux-arm64-red.log` 在 `/private/tmp/` 保留。

这两项是**条件库级正确性反例，不是实际WAL-reset竞态或zlib溢出复现**，不能替代第4节完整
测试阶段。harness故障注入、driver取消/复用/Rows.Close、WAL确定性时序、Linux编译feature
查询、APK修复取证和精确OS/C门测试仍需继续；先独立保存test/docs检查点，不将此红灯推main。
当前状态 **partial-library-reds-verified / implementation-not-started**；全部green回归、候选卷/
正式发布及整体重构待办，Docker仍fc4692c，生产未操作或新核验。

## 7. WAL 确定性测试调度补充（计划，未复现）

官方SQLite修复commit `3c7a03032f9c69feb832b96447701bee6fa3ba81`（Fossil
`5aadfbbdd83b051b5f4a3f06eb60cf9b37aa1d62e676584719e6f785ecf148c7`）及其
`test/walrestart.test`已读，只有源码参考，没有执行下载的测试/PoC。修复是在取得read-lock0后
复核live/cached WAL salt，避免将已重置的新WAL当旧WAL回填。

当前实际amalgamation的 `sqlite3WalCheckpoint` 在读取头之后、调用walCheckpoint之前，若头
变化且VFS为v3，会调用公开I/O的xUnfetch。这提供天然test-only调度点：只在自有checkpointer
文件句柄复制/包装公开I/O方法表，其他方法和xUnfetch底层操作全部原样转发；在此点执行另一
自有连接的正常短事务，再恢复检查点。不得修改WAL数据/header、SQLite算法或伪造I/O成功。

调度测试必须观测初次完整checkpoint、临界回调确实进入、writer实际重置salt/有效帧、后继
较大事务、checkpoint/reopen/integrity和已提交值。正常未触发调度控制与故障调度分开；
未进入回调/帧数前提不成立/ABI或I/O错误不是损坏red。读取自有SHM/WAL格式只作观察，不写入。
测试辅助仅由_test引入、只在可杀的自有worker执行，不能进入生产main依赖图或新增SQL调试API；
若使用C公开ABI声明，须核对当前与候选头文件和架构，不能依赖未公开内部结构。

先在临时隔离探针验证可行性，再用实际Go嵌入C的仓库测试固定；不把独立编译探针、版本门或
理论顺序冒充已发布binary/生产复现。本节先独立保存推送，后写该调度测试；第4–6节其余门不缩小。

## 8. 连接生命周期、测试进程保护和原生运行时控制（测试阶段继续）

第7节合同已以 `3982b9f31333d31fcf095a6170910670e4bec765` 独立推送核验；本批只加测试，
仍未改Go模块、应用代码、Dockerfile、workflow或前端，仍是driver1.14.22/C3.45.1。

- 准备语句依次绑定长/短Unicode、NULL、空串及恢复值，验证没有残留上次参数。
- 16次两行查询只读第一行，提前关闭Rows两次，再复用同一准备语句/单连接，验证释放和重置。
- 两行查询读到首行后取消context，后续Rows返回context.Canceled；取消已写但未提交的事务后
  Commit不得成功，单连接池能恢复查询，文件DB仍保留原提交值且integrity为ok。此项没有证明
  正在执行C step时的中断，不能替代后续in-flight取消或WAL调度验证。
- 自有父/子进程实际注入17KiB输出、工作超时、SIGINT与SIGTERM。每项都必须观察准确拒绝原因、
  测试父进程正常失败退出1、已记录自有worker PID消失；观测超时/外层输出超限不算成功。
  信号只发给本次Start所得父PID；外层异常回收仅针对新建自有进程组，不按进程名批量杀进程。
  正常worker仍不生成子进程，只有这个保护验证显式包含自有父/子两层。
- 新runtime控制实际执行SQL的sqlite_version/source_id及compile_options，与链接C接口身份
  一致；宿主和原生Linuxarm64均为3.45.1/既有e876e51a source，FTS3、FTS3_PARENTHESIS、
  RTREE、THREADSAFE=1存在，FTS5未开启。这是同源码测试binary查询，不冒称已对发布服务执行SQL。

三项新driver控制连续10轮通过；四项harness故障控制连续3轮通过（每轮约2.08s）。完整db包
3.082s，既有12个root测试通过，实际库级失败仍精确为第6节两叶；正常控制现7项，保护验证4项。
同切片race 9.962s结果相同、无DATA RACE；原生Linuxarm64/Go1.26.9/CGO1在768MiB/2CPU/
128PID硬限、源码及module cache只读、自有临时容器内原样2red/7正常绿/4保护绿，2.101s。
容器已自动移除，没有留下该fixture worker。`go vet ./db`通过；既有两审计门合同109/109通过。
日志 `/private/tmp/openreader-sqlite-c-lifecycle-{race,full-db,linux-arm64,gates}.log` 保留。

本批是独立test/docs检查点，不是依赖实施完成或新镜像发布。未跑全Go/前端/build/浏览器/卷/
可信发布：无应用或前端改动且明确仍有两项预期库级红灯；所有实现与发布后置门继续保留。
WAL确定性时序、in-flight C取消、APK精确取证与OS/C门及整体重构仍未完成。生产未访问或修改，
生产commit未新核验；最近已验证发布仍fc4692c，不将本批测试结果等同于生产已升级。

## 9. in-flight C step 取消控制与测试阶段收口（2026-10-16，Claude Code）

交接 `fab8d3e55569750b31a6f4c5241a6e995ef411c3` 后先在同一工作树复现基线：完整db包既有12个root
测试通过、仅第6节两叶失败，7正常控制/4保护验证绿，3.719s；同切片race结果相同，9.900s，无DATA
RACE；`go vet ./db` 通过；审计门合同109/109。backend内Go为go.mod选择的1.26.9（宿主根目录为
1.26.5）。随后仅新增测试提交 `52786f8600f1795f44245735b16ebc638da4ba17`，已推送并ls-remote精确
核验；Go模块/应用/Dockerfile/workflow/前端未改，仍为driver1.14.22/C3.45.1。

- `in-flight-query-cancel`、`in-flight-exec-cancel`：自有worker内经应用 `Open` 打开自有WAL文件库，
  单连接池。只在该 `*sql.Conn` 上经 `Raw` 注册连接级标记函数（仅测试进程，不进生产依赖图，不加
  SQL调试API）；单行递归计数常量内存，在一次C step内执行，不被中断会远超worker时限。标记进入且
  50ms后仍在执行才取消；要求1秒内返回 `context.Canceled` 且无结果。
- 读路径：同一连接500ms内复用 `SELECT ?`，证明被中断语句已重置，没有残留中断或忙语句。
- 写路径：同一被取消事务内先UPDATE、再执行被中断的INSERT；Commit不得成功；新池连接500ms内读到
  原提交值、行数2、integrity为ok，证明单连接池已回收。
- C step若忽略取消，worker直接输出准确原因并退出1（延迟清理会等待仍在C内的语句，把行为失败变成
  harness超时），父测试按实际行为失败分类；标记未到达/取消前已结束单列为前提失败，不算行为red。

证据：两项连续10轮20/20通过，取消到返回20–66µs。临时反向变异（语句改用不可取消的Background，
未提交）使两项均在约1.06s以 `in-flight C step ignored cancellation for 1s` 失败，并按实际行为
失败分类，无遗留worker；文件随后原样恢复。新增后完整db包：既有12root通过，失败仍精确为第6节
两叶，正常控制9项、保护验证4项，2.745s；同切片race结果相同，12.090s，无DATA RACE；vet及审计门
109/109通过。本轮未重跑原生Linuxarm64容器。

第7节WAL确定性调度：本轮Claude Code会话没有编写临时探针、test-only C helper或调度测试（该部分
会话输出被安全检查中止）。按第2节与第4节第3条如实保留 **未复现/open**：不能以版本门、普通并发、
in-flight取消控制或两项SQL red替代，也不得在任何记录中写成已复现或已排除。该项不撤销WAL-reset
的must-fix维护裁决；升级后以源码补丁存在与实际runtime版本为证据，确定性调度仍可后续补做。

据此SQLite驱动维护的测试检查点收口为 **2实际库级red/9正常控制/4保护验证**，状态
**library-reds-verified / in-flight-cancel-verified / wal-schedule-unreproduced /
implementation-not-started**。下一步为独立 `build:` 提交把driver升到第2节精确候选v1.14.36/
SQLite3.51.3（不跨3.53），同一断言红转绿且全部控制保持；OS/C工具合同测试与zlib构建期修复另行
推进。镜像仍为fc4692c，未发布新镜像；生产未访问或修改。

## 10. driver v1.14.36 / SQLite 3.51.3 实施与本地回归（2026-10-16，同一断言红转绿）

第9节记录 `fc289a04b61498e526ab8d0ea210cd816d64e4e2` 推送核验后，独立 `build:` 提交
`7a1bf82` 仅改 `backend/go.mod`/`go.sum`：`github.com/mattn/go-sqlite3` v1.14.22→v1.14.36
（Origin commit `4ea2a9fb44c7ecc6eee436d26a32ab34a79a87ce`、tag v1.14.36、
`h1:EyQAxPedU6e4/lGrYcT2+1yVcOLT7ERS6jvTYKzfzgY=`，go.mod hash不变）；`go mod tidy` 只移除旧版本两行，
`go mod verify` 通过。应用Go代码、gorm sqlite driver v1.5.6、Go最低1.26.9、Dockerfile、workflow、前端均未改；
不跨3.53系列。

- 候选核验：头文件 SQLITE_VERSION 3.51.3、SOURCE_ID `2026-03-13 10:38:09 737ae4a3…fe6d618`；cgo默认编译
  参数与旧版逐行相同。Go侧差异为Rows.Close加closemu并去掉finalizer、绑定前clear_bindings、多语句参数不足时
  关闭语句、新增SetFileControlInt64、文本回调结果内存处理，均由准备绑定/早关Rows/取消/in-flight控制覆盖。
- 同一断言红转绿：`empty-aggregate-not-null`、`redundant-upsert-index` 通过。实际SQL runtime为3.51.3/
  `737ae4a3…`，与链接C一致；compile_options唯一变化为MAX_FUNCTION_ARG 127→1000（上游默认上限），
  FTS3/FTS3_PARENTHESIS/RTREE/THREADSAFE=1保持，FTS5仍未开启。
- 新driver：两项原red与5项driver控制各连续10轮70/70；4项保护验证各3轮12/12；完整db包全部通过（3.228s）；
  同切片race通过（15.107s），无DATA RACE；vet通过；in-flight取消22–25µs。
- 全量回归：backend `go test ./...` 29包ok/7包无测试（沙箱禁止本地监听时4包bind EPERM，沙箱外重跑全过，
  140s）；`go vet ./...` 通过；关键race `./api ./db ./services/...` 全部通过、无DATA RACE（db与23个services包首轮通过；api首轮
在go test默认10分钟时限整体超时，超时时正在运行的测试仅2s且无goroutine位于go-sqlite3，改用40分钟时限
单独重跑1019.965s通过）；frontend按lock
  `npm ci` 后788/788（沙箱内3项因listen EPERM失败，沙箱外全过）、build通过；审计门合同109/109；实际Go源码
  扫描pass-with-not-applicable（246 advisories，唯一finding仍为既有不适用GO-2026-5932，DB
  2026-10-08T22:31:09Z）；前端完整lock 132包0 finding；`docker compose config` 通过；隔离存储读列表/导入
  HTTP合同（真实Go服务+SQLite，跨用户/WebDAV/LocalStore）通过；`go mod vendor` 构建上下文含v1.14.36/3.51.3
  头文件，宿主 `-mod=vendor -trimpath` CGO构建通过。
- 未执行：三视口浏览器（前端未改，本工作树未安装Playwright浏览器）、原生Linux arm64候选、fresh/historical卷与
  portable备份（main上的Actions执行）、可信双平台构建发布与OCI/binary/C来源核验。WAL-reset确定性调度仍
  **未复现/open**；修复证据仅为官方3.51.3 source ID（官方WAL文档称3.51.3起修复）与实际runtime版本，
  未逐行比对官方补丁。

状态 **implementation-complete / locally-regression-validated / Docker-pending /
wal-schedule-unreproduced**。未合入main，未构建或推送镜像；镜像仍为fc4692c；生产未访问或修改。
OS/C的zlib构建期修复与APK/secdb门（交接3d）仍开放。

## 11. OS/C APK发布门与zlib构建期维护合同（2026-10-16，实施前）

Registry只读核验：官方 `alpine:3.23` 与 `alpine:3.23.6` 仍是同一index `85fe1e81…`，3.23.7不存在，
“刷新base”不能修复zlib；按第2节只允许构建期安装签名校验的官方精确zlib APK。

实施前取证（用户批准下载两份APKINDEX；其余均为已存证据或本地已有镜像，断网运行）：

- v3.23 main x86_64/aarch64 APKINDEX（`v3.23.6-61-g95654076fb0`，Last-Modified 10-16 05:46 GMT，
  sha256 `d25ac254…`/`c65d4f28…`）以已发布base层内官方公钥 `6165ee59`/`616ae350` RSA验签通过。
  zlib 1.3.2-r1：x86_64 `C:Q1jut3bNPRHp/JoWp0GWNhtIWnnQE=`（S 56205），aarch64
  `C:Q1932/MXUWsUUsB2j59i8WBsgG3S0=`（S 54331），aports提交 `b92621a82657b055485976b304462bddf9aa9183`，
  与第2节维护者补丁一致。
- base 16包中13个在当前签名索引里同版本且 `C:` 全等，证明安装库的 `C:` 就是签名索引身份，可直接pin。
  libcrypto3/libssl3 3.5.8-r0已被索引中openssl 3.5.9-r0（约9-30构建）取代，但10-13 secdb对3.5.8-r0
  无待修复项；本合同不升级openssl，发布时门读取最新secdb，出现修复即失败关闭。
- 已发布fc4692c两平台层还原后，110/109个APK拥有文件按 `Q1`+SHA1（文件内容或符号链接目标）逐一核对，
  仅 `etc/shadow` 被官方base层自身改写（包内 `Q1miRFe6…`，实际 `Q1/czoE9…`）；builder复制的CA bundle
  与包内字节相同；world为6项，repositories为两条官方v3.23地址；os-release归alpine-release包所有。
- 本地pinned alpine 3.23.6（apk-tools 3.0.8，`--network none`）对77组已安装/修复版本原生比较，两架构
  结果相同，唯一 `<` 为 `1.3.2-r0 < 1.3.2-r1`（CVE-2026-85091）；候选r1无 `<`。

门合同（`scripts/check-os-advisories.mjs` + 已审基线 `scripts/os-apk-baseline.json`）：

- 只接受工作流加载的 `openreader-ci:TAG` 与 `openreader-ci-arm64:TAG`（同TAG），无任何绕过参数。
  `docker save` 落盘后解析：唯一manifest，config摘要与路径一致，每层解压后匹配diff_id；tar头校验和、
  结束标记、条目类型、路径失败关闭；按OCI whiteout/opaque/非目录替换语义展开最终文件系统。
- 基线固定两平台16包的版本/origin/`C:`（base原样加zlib r1 pin），world（含 `zlib=1.3.2-r1`）、
  repositories、Cmd，唯一放行修改 `etc/shadow`（同时固定包内与实际校验和），5个无secdb记录的origin
  记为unknown（不是clean），复审期限2026-11-16，过期或被secdb收录即失败。
- 失败关闭：OS/架构/variant/Cmd/Entrypoint不符；os-release不是3.23.6；APK库空、截断、未知字段、重复、
  缺字段、非法路径或校验和；包身份不符、多包或少包；APK拥有文件缺失或校验和不符；两平台包集合不同；
  secdb schema变化、超过30天或时间在未来；原生比较集合与计划不完全一致或出现 `<`/`=`/`>` 以外结果；
  任一 `<` 待修复。OS发现没有not-applicable通道，不凭推理豁免。
- workflow：门测试在构建前运行；amd64验证镜像后新增arm64验证镜像（load、不推送、独立gha缓存），
  然后对两平台运行门，再跑卷门和推送；推送步骤同时读取arm64缓存以复用已验证的层。
- Dockerfile：运行时阶段在FROM之后、ARG之前执行 `RUN apk add --no-cache 'zlib=1.3.2-r1'`，不用
  `--allow-untrusted`、upgrade、额外仓库或latest，CMD不变，builder阶段不改。

测试先于实现：实现前门测试必须整体失败，实现与workflow后只剩Dockerfile断言为红，Dockerfile修复后
全部转绿；另需用本地已有的真实已发布镜像和完整secdb走一遍真实采集路径。实际候选镜像构建、两平台门
运行、新旧卷与可信双平台发布只能在合入main后的Actions中完成，本地不构建、不推送镜像，生产不操作。

## 12. OS/C APK门与zlib修复实施记录（2026-10-16，本地已验证，Actions待验证）

第11节合同 `6e008a9ed1c4eb3be0a4c18b9774b412e48c95d0` 推送核验后依次提交并逐个ls-remote核验：

- `97e956044ed6c713a337d0c6b22163e765d24eee` test：19项门合同测试、已审基线和真实fixture（已发布两平台
  installed库、secdb子集、原生比较结果、签名索引zlib记录及来源）。该提交没有实现，测试整体以
  ERR_MODULE_NOT_FOUND失败，即门可用性red。
- `6aeda8b35cd853754bf0d02bfff41a5b529ab86b` ci：门实现与workflow；此时18/19，仅Dockerfile断言为红。
- `d1d774fd8fb6b0d10b2ee6d37c5315e1dc0fa120` build：运行时阶段FROM之后、ARG之前
  `RUN apk add --no-cache 'zlib=1.3.2-r1'`；19/19。

验证：门测试19/19，连同既有Go/前端门共128/128，宿主Node 26与本地pinned Node 22.23.3（断网、只读挂载）
均通过；`docker compose config` 通过；workflow经YAML解析，顺序为门测试→amd64验证镜像→arm64验证镜像→
两平台门→卷门→登录→推送。每个失败关闭用例逐一打印拒绝原因，均命中预期分支。真实制品：本地已有的已发布
fc4692c arm64镜像经 `docker save`（Docker 29/containerd）走真实解析路径，14.58MB、6层diff_id全部匹配、
586条目；base-only基线16包/109文件通过（仅放行etc/shadow），候选基线在zlib 1.3.2-r0被拒；完整secdb
（main 282包、community 461包）通过严格schema；真实镜像内原生比较72组与fixture一致，唯一pending为
zlib r0<r1。本地没有已发布amd64内容，未拉取。

未验证：含zlib r1的候选镜像实际构建、CI网络下 `apk add` 结果、两平台门实际运行、新旧卷/portable、可信
双平台发布与OCI/APK核验，均需合入main后由Actions执行；本地未构建或推送镜像。状态
**os-gate-implemented / zlib-build-fix-applied / locally-validated / Actions-pending**；WAL调度仍未复现；
镜像仍为fc4692c；生产未访问或修改。

## 13. 发布记录（2026-10-16，可信Actions发布并独立核验）

经用户确认，`main` 由 `24e55bab922e6c55206ef0a64d7fd64c02374da4` 快进至
`186c762252ebbf8b33734a92d91ccac4975f5ac0`（非force，ls-remote核验）。可信Actions `38896494238` 成功，
24m37s，全部步骤success：backend测试4m42s、Go/前端审计门与实际扫描、OS门测试、frontend测试/build、
Compose、隔离存储HTTP合同、amd64验证镜像1m55s、arm64验证镜像15m29s；两平台OS门结果为
pass-with-unknown-origins（16/16包、110/109个APK文件、仅etc/shadow、secdb main 10-13 `1f2de215…`与
community 10-09 `96d0e5fc…`、72组比较无pending、5个unknown origin）；fresh/historical卷与portable备份、
推送8s（复用已验证层缓存）及平台核验均通过。

镜像：`ghcr.io/changshengyu/openreader:186c762` 与 `latest` 指向OCI index
`sha256:bfda54269b4cf53bab46f1debdd3d9bef640f23c4de5a7f6b248ed081c43f83c`；linux/amd64
`sha256:05014b1c05d841a87f8a11bcd52d36b62fa8761b9eb14bcfe22f13d72a54372f`，linux/arm64
`sha256:d78a1190a58b1d99205fc4458283ea470b0d78d9607b313a744ce840a4703740`，另有两份attestation。

发布后独立核验（按index digest拉取两平台，`docker save` 后用门同一解析逻辑）：两平台各7层diff_id全部
匹配；16包身份与基线一致，110/109个APK文件逐一核对，仅放行etc/shadow；zlib 1.3.2-r1的 `C:` 与pin相同
（x86_64 `Q1jut3bNPRHp/JoWp0GWNhtIWnnQE=`，aarch64 `Q1932/MXUWsUUsB2j59i8WBsgG3S0=`）；revision标签为
完整186c762…；二进制为go1.26.9、go-sqlite3 v1.14.36（vendored构建信息不含h1，由go.sum固定），含SQLite
3.51.3 source ID `2026-03-13 10:38:09 737ae4a3…`，不含3.45.1的 `e876e51a…`；二进制sha256 amd64
`f88b3cab…`（22,107,264 B）、arm64 `daa51d8a…`（20,374,944 B）。

未做：任何生产操作或生产运行commit核验；本轮未执行三视口浏览器与原生arm64设备回归；WAL-reset确定性调度
仍为未复现/open；openssl 3.5.9-r0待secdb列出修复再评估；community secdb超过30天未更新或unknown origin
复审期（2026-11-16）到期时，门会失败关闭，需要复审。状态 **released / Docker-published /
awaiting-device-verification**；生产实际运行版本未知。
