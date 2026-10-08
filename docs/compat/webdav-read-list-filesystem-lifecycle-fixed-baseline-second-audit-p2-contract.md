# WebDAV / LocalStore 读取与列表生命周期固定基准第二轮合同（P2）

状态：**implemented / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
当前审查基线：`OpenReader@08de4decfb91248901ca73adee5f102563181d68`。
审查日期：2026-10-08。

盘点阶段仅更新合同与矩阵，独立提交 `3ecf48a` 后才加入下述红测，`80bb48c` 后才实施。
不能称为已复现的生产漏洞，也不能从 PUT/COPY/MOVE/MKCOL 的门禁推导本项已完成。

## 1. 范围与固定上游证据

权威源码均通过 `git show <固定 SHA>:<路径>` 读取，不使用上游工作副本当前 HEAD：

- `src/main/java/com/htmake/reader/api/controller/WebdavController.kt:90–181,230–314`：
  认证后 PROPFIND 文件/目录返回207；目录包含自身及一级子项，不递归；缺失404。
- 同文件 `363–379`：GET 普通文件下载，缺失404，目录405；调用 `sendFile`。
- 同文件 `515–562`：网页端目录列表跳过点名前缀，路径缺失/非目录为可见错误。
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:2378–2419,2422–2461`：
  LocalStore 权限先行，当前目录一级列表跳过点名前缀，缺失/非目录报错，普通文件可下载。
- `src/main/java/com/htmake/reader/api/controller/BaseController.kt:275–290`：caller home 惰性初始化。
- `web/src/components/LocalStore.vue:192–224`：先请求当前目录，成功才更新列表/打开管理面板。

保留 OpenReader `/webdav` 网页兼容目录 GET、`/reader3/webdav` 外部协议、JWT/Basic、
管理员历史根和普通用户私有根。此次不改 UI、parser、导入确认、backup 格式或读写路由。

本项 public action 是两前缀 GET/PROPFIND、LocalStore list/download；共享 service.Stat/List/Open
的内部及其他 API caller 必须做相邻回归，但不能据此宣称它们的整个请求生命周期重新签收。
LocalStore/WebDAV import 的绝对目录展开与其他独立读取动作继续保留下一项审查，不在此项中
偷偷改动其200项 admission、token、SQL提交或缓存补偿语义。

## 2. 当前映射与裁决

| 层 | 当前 OpenReader@08de4de | 裁决 |
|---|---|---|
| 外部协议 | 双前缀 Basic/Bearer、caller scope、DAV 能力头、PROPFIND207 XML/Depth0或一级，缺失404；上游目录GET405。 | **aligned / 保留**。Depth infinity/其他值夹紧一级为既有安全适配。 |
| 网页兼容 | `/webdav` 目录GET207私有XML；LocalStore200 `{path,recursive,items}`，dirs-first/大小写不敏感 path 稳定排序，隐藏点名/link/special。 | **acceptable-change / 保留已部署 adapter**。不换 JSON/XML、字段名或前端调用顺序。 |
| 数据与权限 | WebDAV管理员`data/webdav`、LocalStore管理员`library/localStore`；普通用户`users/<safe-name>`。权限先于 filesystem，根可惰性初始化，缺失子路径不创建。 | **aligned / 保留**，不迁移、不扫描历史卷。 |
| Stat | `Resolve/rejectSymlinks` 后再绝对`Lstat`，没有绑定configured boundary/user ancestors的opened identity。 | **must-fix / 源码证据**。不能只验证最终目标或继续绝对路径二次读取。 |
| Open | `Resolve→Lstat→os.Open→file.Stat/SameFile`，能拒绝稳定link/special及部分final替换；祖先可在检查后切换，`os.Open` 可跟随晚到link，晚到FIFO可能阻塞。 | **must-fix / 源码证据**。同目录fd、NOFOLLOW/NONBLOCK及最终regular复验；不能把SameFile等同于整个caller root绑定。 |
| WebDAV List | `Resolve→Lstat→os.ReadDir→每项绝对Lstat`；目录替换后可能混用另一棵树，目录扫描不观察request context。 | **must-fix / 源码证据**。读取和metadata均相对原opened目录，返回前复验root/ancestors/目标。 |
| LocalStore List | service.Stat/Resolve后绕回绝对`WalkDir/ReadDir/DirEntry.Info`；recursive callback吞一般读取错误/跳过隐藏或不安全项。 | **must-fix identity/context**；保留隐藏规则、recursive query和正常排序。不得把生命周期unsafe/cancellation作为可跳过错误并返回部分成功列表。 |
| 请求生命周期 | EnsureRoot已承接context；后续Stat/List/Open本身无context；GET在同一个request中重复建service/stat/open。 | **must-fix**。public读操作使用request context；同一响应不重新选择caller根/目标；成功输出前必须完成本次admission。 |
| 资源上限 | DAV一级列举、LocalStore递归结果目前没有新增分页/cardinality上限。 | **unknown / 单独后续盘点**。本项不静默截断既有结果或凭猜测新增协议错误。 |

## 3. 目标读取合同

1. 授权、权限、现有路径规范化先行；打开当前服务的configured boundary，并按fd逐组件绑定
   caller private root及目标祖先。拒绝link、非法父组件或意外root/parent替换；不能仅以私有根
   自身作为anchor而遗漏`users`。根外诱饵的文件字节及名字不得进入响应。
2. 每次操作检查request context。目标metadata相对原parent获取；普通文件相对同一parent
   `NOFOLLOW|NONBLOCK`打开，再验证regular及admitted identity。FIFO/socket/device不得阻塞、
   读取或改变；目录与普通文件的既有错误区分保持。所有失败关闭本请求已开的句柄。
3. 在向HTTP交付文件前，复验boundary/root/ancestor/final身份。交付后由同一个打开的文件提供
   bytes、size、mtime及Seek/Range，不重新按名字打开；后续合法rename可继续提供原handle的
   字节，不能转而读取新路径实体。对正在原inode上修改内容不承诺immutable snapshot或全文件CAS。
4. 列表只读原opened目录；每个子项metadata和递归目录打开均通过同一父fd。每层/每批检查ctx，
   输出前复验admitted root/ancestors/目标；验证到的中途替换或取消不输出成功列表。正常并发
   列表不是全文件系统事务，不承诺所有子项在同一个时间点存在。
5. public GET在已获得resource后选择一次file/list路径；不重新初始化root并把不同admission
   的metadata/bytes混合。允许共享background wrapper保护内部补偿caller，但public handler
   不得借background wrapper丢弃request cancellation。
6. GET body传输继续使用既有`http.ServeContent`的普通下载、Range、conditional及content-type
   行为；发送前和读取/Seek边界检查ctx。已经发出的字节无法回收，不声称取消具备HTTP原子性。
   不增加499/host-path错误；WebDAV cancellation沿用无新增正文，LocalStore不得输出成功items
   或文件字节（保留当前固定错误shape，不泄漏实体名字）。
7. 正常metadata读取不因文件000变成内容读取；List/GET所需目录或文件权限不可擅自chmod。
   当前一般I/O错误映射保持；对target/root/ancestor身份变更采用现有unsafe映射，而非500详情。

metadata实现约束补充：Darwin实际非root探针证明O_EVTONLY仍拒绝000文件，不能以chmod或
读取正文代替stat。允许从本请求已持有的directory fd，经server生成`/dev/fd/<numeric fd>`构造
`os.Root`并绑定native SameFile，再只对单组件执行Root.Lstat；该路径不是用户输入，也不是
访问实体的absolute fallback。Mac与Alpine非root探针/正式Linux回归必须验证此标准fd入口可用；
不可用时fail closed，不转回用户absolute路径。fd原持有者与Root均须在操作结束关闭。

共享读取句柄兼容补充：返回的`os.File.Name()`仍须为原configured boundary与已规范化relative
生成的完整路径标签。Reader本地归档/章节缓存身份复验、EPUB资源及现有错误封装依赖该标签；
标签不是重新打开文件的授权。fd仍只能来自原父目录Openat，不能以恢复Name为由恢复绝对Open。
全量回归已发现只返回basename导致Reader `book-identity` 409和EPUB资源路径控制组失败；
先补完整Name与返回后rename控制红测，再恢复此标签，不改现有caller身份校验。

## 4. 可见 API 与列表差异

| 动作 | 正常响应 / 错误 | 必须保持 |
|---|---|---|
| 两前缀普通文件GET | 200普通字节；Range206/416及conditional304；缺失404，unsafe403空body，一般500空body | DAV/Allow/MS-Author-Via头、同一handle、原始空白/中文路径、不增加body/query字段。 |
| `/reader3/webdav`目录GET | 405空body | 不改为网页私有列表。 |
| `/webdav`目录GET | 207既有私有XML | 自身空displayname及一级子项，目录size0、mtime格式和网页客户端保持。 |
| 两前缀PROPFIND | 207 DAV namespace XML，Depth0或一级；缺失404、unsafe403空body | href当前前缀/逐段escape/目录尾斜杠、displayname及原文件metadata；安全夹紧infinity。任一未隐藏子项link/special仍整体拒绝，不删除。 |
| LocalStore list | 200 `{path,recursive,items}`；missing-child404 `{error:"local store path not found"}`；非目录500固定message；unsafe400 `{error:"invalid path"}` | recursive仅`1/true`；隐藏点名和link/special且不进入隐藏目录；普通neighbor保留；字段/dirs-first和现有排序保持。根惰性初始化不等于缺失子路径写盘。 |
| LocalStore download | 200原字节/Range；缺失404 `{error:"local store item not found"}`；根或目录400固定message；unsafe400固定JSON | Content-Disposition filename、普通下载、same-handle bytes/metadata；不走parser/import。 |

上游无安全root/opened-handle合同；不复制其绝对路径拼接/发送或Authorization日志。
上游的`Cache-Control: 86400`不是本项新要求，保持已部署ServeContent传输适配；是否需要新的
缓存/attachment策略须单独审查，不能夹带在filesystem实现中。

## 5. 数据和允许差异

无SQLite schema、配置、root布局、backup成员、cache generation或旧URL变化。无启动扫描、
迁移、chmod、清理、repair或本次读取的子路径创建；fresh合法root初始化沿用08de4de的已审查能力。
读操作不触发WebDAV bookProgress PUT ingress、SQL进度写入或WebSocket广播。

允许Go fd-relative/no-follow/context的安全适配，以及已部署的多用户隔离、REST/目录GET适配；
不允许隐藏正常目录、取消recursive读取、禁止所有historical root或破坏Basic/Bearer来消除风险。
稳定symlink/special保持原位，DAV fail-closed与LocalStore隐藏的区别不能无意统一。

## 6. 红测与实施门

先独立提交本合同及API/data/security矩阵，再补红测；不得先实现再补一组只证明新代码的测试。

- 最小测试接缝只插入实际admission/open/list边界、不改变原逻辑；root/`users`/private-root/
  target-parent/target真实目录或link替换fixture必须记录fired。红测分别证明外部名字/bytes被
  泄漏、unsafe实体被接收或读取仍成功，不以未触发fixture当作已复现。
- after-admission最终regular→link/FIFO/directory及parent替换；special测试具备有界超时和明确
  goroutine/fd清理，不能令Go测试进程挂死。原文件、外部诱饵、替换实体均不得写/删/chmod。
- 已授权请求在root初始化后、目录读取/递归第二层/交付文件前取消，BOTH DAV prefixes和
  LocalStore normal/recursive/download均不得有成功bytes/items；认证失败必须在文件访问前。
- 控制组：管理员旧根、两个普通用户隔离、fresh roots、空目录、Depth0/1/infinity、hidden与
  link/special策略差异、Unicode/原始空白、重复正常读取、000文件metadata/权限、Range206/416、
  conditional304、目录405、缺失404、正常LocalStorerecursive排序。不能为红灯绕过控制组。
- 已打开regular返回后真实rename/换路径：只读取原handle，不泄漏替换目标。无immutable-content
  或全球文件系统snapshot断言。

实现后focused/full/race/vet、frontend/build、Compose、Linuxamd64/arm64交叉编译与非rootLinux
权限/FIFO测试、独立新临时卷Basic/curl/LocalStore真实HTTP必须通过；共享Open/Stat的cache、
local-book archive、upload/backup/import/restore与相邻Reader回归必测。可信fresh/historical/
portable/backup和最终amd64/arm64 OCI门独立核验后才可Docker-published。

盘点和红测建立时目录创建08de4de的发布运行37740704252仍在进行，故未向main推送应用测试以
取消该运行。此后该运行已终态success，exact08de4de/latest双架构OCI已核验，见目录创建合同。
生产db1ea21、原书用户验收已恢复；这些事实不证明本项read/list完成。

## 7. 独立红测记录（2026-10-08）

合同 `3ecf48a` 已提交并推送后，只插入nil-in-production的after-admission/before-read测试接缝，
未改变原Stat/List/Open、目录扫描或ServeContent逻辑。红测共30个确定性失败：

- 20项Stat/List × boundary/users/user/parent/target × real-directory/symlink实际替换全部fired。
  Stat错误接收已变更namespace；10项List返回`foreign-secret.txt`诱饵名。原节点和诱饵字节未变。
- 1项最终regular在admission后被同inode symlink替换，原Open错误返回句柄及原字节。
  SameFile不能证明仍未跟随symlink；保留原字节不等于正确接受unsafe namespace。
- 1项最终regular变FIFO，原Open超过500ms仍阻塞；测试用临时NONBLOCK peer释放旧reader并
  join worker，随后旧逻辑返回unsafe。这是阻塞红灯，不是声称FIFO成功读出数据。
- 8项真实Gin授权后/root初始化后取消：两前缀文件GET、两前缀PROPFIND、网页DAV目录GET、
  LocalStore normal/recursive list/download仍返回200原正文或207/200成功列表。

独立控制组通过：原文件000 metadata且不chmod、已返回regular handle在rename/新final之后仍
读取原字节，以及管理员/普通用户三种下载路由Range206精确bytes、conditional304和无效Range416。
新增接缝测试不并行，不修改生产状态。该独立红测阶段全量Go预期失败，不称候选或Docker发布。

本阶段红测提交保存在独立codex分支，避免main应用push取消正在发布的08de4de。实施、深层扫描
取消/子项替换、完整正常控制组、共享caller回归、真实HTTP及最终卷/双架构门仍须逐项完成。

## 8. 实施与本地候选证据（2026-10-08）

rootedfs.ReadHandle绑定configured boundary、users/user及全部祖先，native fd-root Lstat和
NOFOLLOW/NONBLOCK Openat绑定metadata/bytes；全部句柄关闭，返回的regular handle独立生存。
列表按原directory fd分批ReadDir，单组件native metadata，逐层/批/子项检查context与身份；
LocalStore不再绝对WalkDir/ReadDir/DirEntry.Info。public GET只接收一次，ServeContent读取/Seek/
writer观察request context；Range/conditional、旧完整File.Name标签和内部background wrappers保留。
稳定link/special仍在原位，DAV整体拒绝与LocalStore隐藏差异保留。无schema/配置/布局/备份迁移。

- 原30项红灯转绿；深层递归cancel/real-directory/symlink替换、子项替换不被best-effort吞掉，
  无部分成功列表；returned-file完整Name＋真实rename仍读原fd、取消的Read/Seek不触及source。
- 首轮全量发现basename标签回退导致Reader `book-identity`409/EPUB控制失败；先补合同与红测
  （确实失败），再仅恢复生成的完整标签。没有削弱Reader身份校验或重新绝对Open。
- 最终Go全量/vet通过（API83.650s），rootedfs/webdavfs全包race与相邻WebDAV/LocalStore/
  ChapterCache/LocalBookArchive/Backup/UploadResource API race203.667s通过；frontend762/762/build、
  Compose及diff检查通过。Go全量包含TXT/GB18030、EPUB、CBZ、本地缓存/导入/恢复等共享caller。
- Linuxamd64/arm64两个服务测试包交叉编译通过；当前arm64二进制在无网络/只读/非root501:20
  临时tmpfs容器全包通过，包含000 native FileInfo、lateFIFO、identity/cancel/permissions。
  使用旧8dc61c3仅作Alpine执行环境，测试二进制来自当前候选，不冒称旧镜像证明新代码。
- 新自带服务/卷的`store-read-list-lifecycle-contract.mjs`真实HTTP通过：管理员历史根与两个私有
  用户、双前缀Basic/Bearer、Depth0/1/infinity、目录GET207/405、中文/原始DAV空白、206精确bytes/
  304/416、000metadata、不chmod、recursive/隐藏/排序、404不创建、DAVunsafe/LocalStore隐藏。
  原始DAV保留尾空白，LocalStoreREST沿用whole-path外侧trim；没有为测试改变规范化。
  同服务实际Basic/curl协议和LocalStore目录/upload/download相邻smoke也通过。
- 真实Go+SQLite+既有无头Chromium1228三视口1440x900/390x844/360x800：双客户端CAS、WS、
  cold Reader正文/进度、WebDAV mirror与Basic进度上传在线/冷定位/reset/no-echo全通过。
  最新默认Chromium1234未安装；系统Chrome诊断首次cold-progress超时不当作通过，使用已有
  1228独立临时实例完成全部断言。没有访问用户生产登录态。

HTTP gate已加入可信Actions；候选commit/push与其fresh/historical/portable/backup/platform及
exact/latest OCI现已独立核验，见下节。生产最后health为db1ea21、用户原书已恢复；没有部署或
生产数据变更。整体重构、import绝对目录展开、
列表cardinality及npm advisory独立审查继续未完成。

## 9. 可信发布核验（2026-10-08）

实现`11235c3ff111261dead932fab56fa36f98333fd5`已推送；可信Actions`37747103667`终态success。
backend/frontend/build/Compose、新真实HTTP/native image、新卷/portable backup、历史卷兼容及
最终双架构发布/platform核验均success。未重启运行，不从旧08de4de发布推导本候选成功。

exact tag `ghcr.io/changshengyu/openreader:11235c3`和`latest`读回同一OCI index，并与可信日志一致：

- index：`sha256:c3aa7a6c1ecc897de83a145d58e86a48084219249dcdd1161ac9c42f663dafa3`
- linux/amd64：`sha256:99dcc7617b0f032ad5fbe91000e330fc2464b584e347d82dc0969ff58935ebed`
- linux/arm64：`sha256:2cefeaef51950c7726b6fbf5477237149736e290d339ec5cfda4fa8011c486ba`

unknown/unknown成员是attestation，不是缺失CPU平台。允许差异仍为Go fd-relative/no-follow/
context、既有多用户及REST适配；不改schema/root/backup/API字段/旧URL。设备验收未从CI推导。
生产health重新读取仍`db1ea216f9849bc44a90b5b760241df1c6d069b0`/statusok；无自动升级。
