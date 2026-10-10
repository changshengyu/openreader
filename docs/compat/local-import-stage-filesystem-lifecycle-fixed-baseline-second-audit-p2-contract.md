# 本地图书暂存 token 文件系统全生命周期第二轮固定基准合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**。
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
源码审查基线：`OpenReader@5cfc53d4993e0b7c020d5bb8d2747af6e07ab73c`，2026-10-10。

以下盘点保留初始合同阶段说明；第8–11节记录测试与实施历史，第12节为最终发布证据，当前状态以上为准。
初始盘点只修改合同，不修改应用。原书已恢复事件保持关闭；当时只记录源码窗口，尚无本项
确定性红灯或生产复现。mounted-source plan→bounded bytes 已独立实现、验证、发布，不能从它推导
stage、prepared publication、消费或 TTL 安全。合同必须独立提交推送，随后实际红测，再实施。

## 1. 固定上游及不可改变的可见语义

所有上游引用均读取固定 SHA，不以工作副本 HEAD 为权威：

- `web/src/views/Index.vue:2259–2399`：浏览器上传→preview，单本确认或批量/逐本选择；取消当前项
  继续下一项，确认前不保存书架；关闭请求删除该项工作文件，不能删除原 mounted 文件。
- `Index.vue:2745–2760`：用户改目录规则后，对服务端已准备的书刷新目录，而不是重新上传原文件。
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:212–270`：认证、格式检查、
  namespace 工作文件、准备目录；空目录仍返回可确认 preview。
- 同文件 `2263–2323`、`web/src/components/LocalStore.vue:286–310`、`WebDAV.vue:330–356`：
  两个存储来源先预览，再回到 Index 的共同确认流，不在列表动作中自动写书架。

既有 P1-E1/E2/E3、direct multipart、prepared snapshot 合同仍有效：OpenReader 的 JWT/多用户、
随机 token、24h 派生缓存、失败可重试、关闭后 TTL 清理而非同步删除，是已部署的允许差异。
不得复制上游共享绝对工作路径、同名覆盖或无界读取。保持 TXT/GB18030、空目录、旧额外格式 API、
规则覆盖、成功 snapshot 无重复解析及 durable-only 通知；不改 UI、URL、请求字段或 parser 语义。

## 2. 当前动作、调用链与差异矩阵

| 动作 | 当前源码与行为 | 裁决与目标 |
|---|---|---|
| 新 token | `local_import_stage.go#stageLocalImport` 随机24 bytes→48位小写hex；绝对MkdirAll、先TTL扫描，再分别WriteFile原始bytes和metadata，失败按name删除。 | **must-fix**：原configured cache/derived root/user链接收一次；写入只在原parent，exclusive自有文件、owned补偿和ctx。 |
| token load | 先校验格式，再独立ReadFile metadata、Open `.book`；metadata无大小上限，过期/坏数据按name删除三文件。 | **must-fix**：same-user原bundle admission；有限metadata、regular no-follow/nonblock、实际Read前后ctx及交付复验；未知替换实体不能删除。 |
| prepared save | validPrepared及JSON大小已有界；绝对CreateTemp/Chmod/Write/Close/Rename，可覆盖当时同名parsed target；defer按name删除temp。 | **must-fix**：原bundle/session拥有temporary与接收时parsed target；原parent publication，拒绝unknown newcomer，失败只回收自有inode。 |
| prepared load | 独立Open/LimitReader；invalid/oversized缓存绝对Remove后false；missing/corrupt/mismatch→Prepare/raw fallback。 | **must-fix**：仍允许稳定普通cache miss，但身份变化/ctx不能被false吞掉并继续parse/SQL；match只ImportPrepared。 |
| 成功消费 | `imports.go`、`localstore.go`、`webdav.go` 在durable Book后Remove三filename，无原bundle identity。 | **must-fix**：只消费该请求已接收的原token文件；无法确认的newcomer保留。cleanup失败不伪造Book回滚/失败，不抑制成功书通知。 |
| TTL / orphan cleanup | startup+hourly绝对ReadDir；metadata无界；Stat跟随link；分类后按nameRemove三files及aged parsed/temp。ctx仅停止ticker。 | **must-fix**：原scan/root/user/selected-file identity、有限metadata、逐步ctx、同进程active-token互斥/lease；取消或replacement不删除unknown。 |
| 路由映射 | 3个direct POST + 4个LocalStore/WebDAV POST；shared helpers可多次重接收cache。 | **must-fix**：同一请求token从load→parse/prepared→durable前验收→consume持有同一stage session，不把独立安全Open串起来算全生命周期。 |
| 数据与parser | token-only不依赖mounted root；source/parsed预算、24h寿命、原bytes和Prepared Matches已有。 | **aligned controls**：不破坏；两文件legacy stage需正常fallback。 |

这是源码窗口，不称已复现漏洞。相邻 rootedfs/read、PUT/MOVE、directory 安全实现不是本项签收。

## 3. API、错误与副作用合同

全部路由保持 `AuthRequired`、现有activity/LocalStore/WebDAV权限和输入形状：

| 方法/路径 | 正常响应 | 普通失败保持 |
|---|---|---|
| POST `/api/imports/books/preview` | 200单preview，含原importToken；新file只上传一次。 | invalid/expired/foreign token400 `invalid or expired local import token`；初始stage普通失败400 `failed to stage import`；parser400+token；prepared save500 `failed to stage parsed import`+token。 |
| POST `/api/imports/books`、`/api/imports/txt` | 201 Book，确认成功后消费原token并通知。raw file直接确认无需强制stage/cache写入。 | 既有request400/401、input413、parser400、durability500 fixed `failed to import book`。 |
| POST `/api/local-store/import-preview`、`/api/webdav/import-preview` | 200 `{items}`；path/token/book或安全error；token-only不接收mounted根。 | 普通stage/parser/token错误仍200 per-item；初始stage安全 `failed to stage import`，prepared filesystem错误统一 `failed to stage parsed import`，不输出PathError。 |
| POST `/api/local-store/import`、`/api/webdav/import` | 200 `{imported}`，逐项durable成功；先前成功项与事件保持。 | invalid token安全per-item；prepared filesystem错误同上；保持普通parser错误、格式和权限优先级。 |

新安全拒绝规则：token读取的unsafe/stable link/FIFO/changed bundle表现为opaque invalid token；
新stage/parsed写入unsafe映射既有固定stage错误，不泄漏root/token/temp/credentials，不引入499。
本阶段实际stage work期间context取消/超时，统一500 `{"error":"local import stage canceled"}`，
四个storage路由整请求停止并仅广播此前已durable成功项；不能返回200空列表伪装取消成功。
direct cancellation不可丢失已存在的可重试token；能安全交付时沿用error+importToken，未知身份时
不伪造新token。durable提交后取消/消费受阻仍保持成功响应和通知，保留无法确认的残留。

上下文范围是本项stage读写/发布及durable前stage验证；完整parserCPU超时、SQL各表/category事务、
archive补偿的逐步ctx仍独立unknown，不能把最后一次ctx检查签成它们已经原子取消。无整批rollback。

## 4. 原实体、预算与资源合同

1. 在filesystem工作前检查ctx及token格式；invalid格式不创建cache/user目录，不探测其它user。
   token-only只有caller数字ID namespace；不尝试mounted源或按display path恢复缺失token。
2. 从configured CacheDir的可信现有anchor开始，绑定cache、`import-previews`、user祖先原identity。
   初始化仅创建缺失derived目录700；不chmod已有目录、不修复link/000、不建新配置根。
   stage session持有原opened目录及token各文件接收identity/absence，贯穿一个item的后续动作。
3. regular文件no-follow/nonblock打开；稳定或接收后symlink/FIFO/socket/device不能跟随或阻塞。
   scan→load、metadata→book、raw→parsed、parser后→publish、durable后→consume、TTL分类→unlink
   都不能切换到替换namespace。通过原openedfd读文件后合法final rename可继续原bytes，但绝不删除
   当前同名newcomer。原祖先替换拒绝；不承诺inode内容不可变或完整FS原子快照。
4. metadata JSON读取固定上限 **1 MiB**，上限内含有效旧格式继续接受，+1不无界分配、不触发parser。
   当前direct filename≤255 bytes（`direct_local_import_boundary.go`），native storage basename同样
   是单个文件名；1MiB远高于正常JSON转义开销且保留旧附加字段空间。超限是不可信缓存安全拒绝，
   不批量迁移/截断/删除历史文件。stage编码也遵守上限；未知超限内容保留，不按“坏JSON”盲删。
5. 原bytes仍用既有MaxImportBytes（默认128MiB），parsed仍用现有source/parsedtext/chapter预算与
   有限JSON界。limit+1必须饱和或用不溢出的探测，MaxInt64配置不能变成负LimitReader。ctx在每次
   Read/Write前后、scan每项、所有publication/消费前检查；阻塞native文件系统不承诺可强行中断。
6. 所有成功、错误、取消、partial初始化和red fixture结束路径关闭自有handles。partial stage回收
   仅原自有文件/空derived目录；同名unknown/newcomer/未知成员保留，不递归删除用户根。
   新随机token必须在写入/TTL工作前确认三份bundle文件都原本缺失；接收了旧inode不等于创建并
   拥有它。任何后缀碰撞都拒绝，不覆盖、消费或通过失败补偿删除旧文件，不复用旧parsed内容。
7. prepared publication保持旧格式与atomic visibility；新temp600。只替换原接收的parsed target或
   确认缺失的name；原有效snapshot失败重parse后保持，真正cache miss按同session原raw fallback。
   corruption清理必须有原identity，lifecycle change不能降级到cache miss。
8. 同进程同token reparse/confirm/cleanup串行或lease，避免TTL/第二请求消费正在处理的bundle；
   等待可取消，闲置lease回收不能无限缓存随机token。跨进程/崩溃SQL↔token exactly-once不在本项
   签收，无新DB table/锁文件/token格式；未知并发实体保留，残留仍按后续合法TTL处理。

## 5. 数据与清理判定

位置继续 `cache/import-previews/<numeric-user-id>/`：`<48hex>.book`原bytes、`<48hex>.json`
含fileName/extension/CreatedAt UTC，optional `<48hex>.parsed.json` PreparedImport；temp仍token
`.parsed-*`。寿命24h、startup/hourly cadence、新目录700/新文件600不变；旧两文件stage重试不迁移。
没有SQLite/config/backup/portable/library/book cache/浏览器持久key变更。不能删除mounted源文件。

cleanup只在原trusted目录处理原regular已选实体：metadata缺失或有效过期token按既有规则清理
该原bundle；孤立 `.book`、孤立parsed、崩溃 `.parsed-` temporary按24h age处理，fresh/original
active-token保持。现有unknown filenames不扩大删除范围；link/special/oversized metadata安全跳过
且保留，不跟随、chmod或删除其外部target。不跨namespace追踪rename后原token来“修复”；不删user
目录。malformed普通metadata按既有策略只清理该原owned bundle，未知替换依然保留。

durable Book与token consume跨SQLite/FS不是原子事务；消费受阻不删除Book、不丢成功响应/通知、
不称token exactly-once。源bytes已读并不授权消费后来出现的同名文件。cleanup background应把启动
ctx传入每次pass，而不是仅取消下一次ticker。

## 6. 实际红测与独立控制组（必须先于实现）

接缝nil-in-production，设置在实际legacy/native工作边界；每个race测试断言fixture fired并有界join，
仅调用一次测试根，不用未触发hook/编译失败证明红灯。不从相邻already-green测试推导本项：

- 三入口新preview：稳定/工作期root/user替换、授权后stage取消，零token/Book/Chapter/category/event；
  old root和foreign bait bytes/模式保留，prepared失败不得泄露hostpath。
- 同token三入口重parse/四个confirm routes：metadata→raw同名或祖先替换、raw→prepared替换、实际
  bounded读取中取消；无bait进入preview/archive/Reader，不fallback mounted原文件；已成功前项通知。
- stage与prepared exclusive publication/rollback：写入后ancestor/temp/target替换、同名newcomer、
  metadata写入失败；原prepared与unknown新文件不被覆盖/删，所有自有handles关闭。
- consume/cleanup：durable后同名newcomer和目录替换不误删；过期metadata分类后replacement、orphan/
  parsed/temp删除前替换、startup/pass逐项取消；两个用户和非数字目录/unknown/fresh邻居保持。
- stable/late same-inode symlink、FIFO、000原权限、metadata精确1MiB/+1、bytes/parsed精确界与
  MaxInt64、防FD泄漏、同token并发lease等待取消及TTL竞态控制。
- 正常控组：历史两/三文件、match no-reparse、missing/corrupt/mismatch fallback、同token失败后
  修规则成功、GB18030/emptyTOC、mounted源删除仍重试、两用户foreign token、失败不consume、成功
  consume三文件、24h过期/old orphan/temp清理、新根700/文件600/旧权限不chmod、raw直接confirm。

合同→先精确实际red独立提交→服务拥有stage session→thin API传ctx/映射错误→全量Go/race/vet、
parser/相邻Reader、frontend全量/build→隔离Go+SQLite真实HTTP及三视口共享导入/Reader→同候选
fresh/historical/portable/backup/Linux双架构trusted Actions→OCI独立核验。阶段记录必须区分
Git、Docker、production及设备验收，不以旧757/762绿灯代替本轮。

## 7. 尚未签收及允许差异

允许差异是多用户随机stage/TTL、bounded native安全与ctx、未知实体保留及durable成功不伪回滚。
尚未签收：本项剩余覆盖/实现/资源并发/runtime/发布；完整parser/SQL/category/archive生命周期；目录
扫描cardinality/depth/FD预算；桌面click/wheel复审、dependency advisory及整体审计。生产独立health
仍db1ea21；已发布mounted-source app5cfc53d，不代表生产升级或新stage问题已解决。

## 8. 独立初轮真实红测（2026-10-10，应用未修复）

合同4104499已独立提交并远端main精确核验后，才在新工作树添加
`backend/api/local_import_stage_lifecycle_contract_test.go` 与nil-in-production阶段接缝；所有原绝对
I/O、bool cache miss、ctx缺口及删除逻辑仍未修改。此前source app5cfc53d仍是发布版。

`go test ./api -run '^TestLocalImportStageLifecycle' -count=1` 在旧应用精确exit1/API5.805s，
共 **90项实际失败**（89个leaf subtest +1个MaxInt64测试），无编译失败、未触发fixture或未join：

- 18新preview在cache/derived/user目录或link替换后仍写foreign namespace并接受token。
- 17授权后取消，覆盖七route的load-metadata/parsed-ready及三preview create；仍成功stage或Book/
  Chapter/category/event，违反已固定500与无当前失败项副作用。
- 21 metadata→bytes阶段cache/user/同名book替换仍进入preview/library；原held bytes保留。
- 18 stable book/user/prepared link被接受；prepared link非普通cache miss，原foreign target保留合同失败。
- 2 prepared target/temp未知newcomer被覆盖/发布；user目录替换这项在旧实现已安全报错，是正常
  控制组，不计红灯，不以它推导其它阶段安全。
- 4 durable后consume删除后来出现的三份同名newcomer；Book正常成功不伪回滚。
- 5 TTL分类后bundle/user/orphan book/parsed/temp替换，被name cleanup误删fresh newcomer。
- metadata1MiB+1无界接收；MaxInt64 read/copy budget+1溢出后错误地交付空bytes。
- 3 metadata/book/prepared FIFO在150ms窗阻塞且peer释放后仍被接收；使用自有nonblocking writer
  显式释放并2s内join，原FIFO与held files保留；没有遗留阻塞goroutine。

最终日志 `/private/tmp/openreader-import-stage-initial-red-final.log`。独立控组：exact1MiB有效
metadata及prepared-user replacement正常拒绝API0.329s；既有历史stage/TTL/两用户/预算/GB18030/
源移除后同token重试和成功消费API0.838s均exit0，另controls.log/API0.861s。轮换前控制0.868s、
新树继承控制0.751s均通过。仅测试自有root，没有用户/生产修改。

初轮红测不是实现或发布候选；只能推feature branch，不将故意红灯main发布。完整Go/frontend/
browser/Linux/volume门在应用实现后执行，当前不能声称全量绿。剩余before-code覆盖包括实际
bounded read/write中取消、parsed load中同名/祖先变化及cache-miss安全错误映射、partial-owned
rollback、consume目录变化、TTL逐项ctx/active lease等待、000/late同inode link及FD所有权；
必要时追加实际红测后再实现对应部分，不把90测试签成整个stage合同已完成。

## 9. 跨阶段与实际工作期追加红测（2026-10-10，仍先于应用修复）

在独立d300cff初轮red远端精确核验后，增加只观察实际Read返回的nil-in-production接缝与
`local_import_stage_deep_lifecycle_contract_test.go`。`^TestLocalImportStageDeep` 在旧应用exit1/
API2.386s，追加 **30项实际失败**（28 leaf +partial rollback/lease各1），无未触发/未join：

- 11实际raw/parsed Read后取消仍返回成功/写Book；不是仅“开始前ctx已取消”。
- 11 raw已加载后user/parsed替换，被后续独立namespace admission接受，preview覆盖foreign parsed
  或confirm导入其内容。原metadata/raw相同不能授权另一个prepared实体。
- partial `.book` 写入后同名unknown替换，再令metadata写入失败，legacy rollback误删unknown。
- 4 durable后user目录替换，consume误删新目录中同名三文件，但成功Book/响应不伪回滚。
- startup已取消和TTL分类后取消仍清理原expired bundle，各1；background取消不能只停止ticker。
- 同token第一请求实际进入work并等待时，第二请求不等待就进入另一bundle；随后ctx取消仍200。
  两个worker都2s内join，第一合法preview仍成功。等待要求是进程内lease，不是跨进程exactly-once。

日志 `/private/tmp/openreader-import-stage-deep-red.log`。30与初轮90互补，不把其它调用/FD/000/
late-link/active-TTL的未测范围算完成；应用修复仍需全合同服务/薄API和对应进一步控制与门禁。

## 10. 服务实现与本地验证（2026-10-10，尚未发布）

独立合同4104499、red d300cff与追加red3572f02均已远端核验后才实施。新
`services/importstage` 拥有每项原cache/user/bundle session、可取消同token lease、bounded actual
read/write、prepared缓存命中/普通fallback、durable前验收、原文件消费及ctx startup/hourly TTL。
`rootedfs.PrivateScope` 从原native目录句柄初始化/读写/发布/回收，新增700/600不修改相邻公开
755/644写入合同。七路由只传request ctx并映射既有错误；raw直接confirm不强制stage。

120初始/追加反例全绿；补23项控制（service9、native8、API durable6）：实际三种写中取消及
owned补偿、合法opened rename仍读原bytes且不consume newcomer、late同inode link、active TTL
lease、000/既有权限保持、120次成功/失败/Close/scan后FD与idle lease回收、actual partial mkdir
取消/unknown成员、原prepared保持、批次先前成功通知和durable后取消仍成功。另实际观察
metadata-write中替换raw的反例先exit1（`metadata-handoff-red.log`），再补整bundle最后复验和仅原
metadata/raw补偿转绿；它是实现复审反例，不能混算初始旧应用120红灯。
再补7路由的prepared发布wire控制，其中4项中间实现实际红灯（`publication-wire-red.log`）：
direct confirm/alias错误400，两个storage confirm错误item message；已将工作期parsed发布失败与
opaque token读取拒绝分开，保持direct500及storage200项固定错误。其余3项旧已绿不混算红灯。

本地全Go曾通过API91.824s；最终write-classification版本full/API103.532s、vet通过。
最终120+30控制精确race/API70.957s、service3.054s/root2.215s通过；共享服务全race也通过。全API race
默认10m总预算超时（605.881s，堆栈为其它replace-rule测试注册时bcrypt运算，未报数据竞争）。
首个中间25m run因源码完善主动中止；后续bundle版25m full/API886.942s终态通过，但在最后
wire错误分类/native观察控制前编译，不能冒充最终源码完整race。最终版本还需完整25m run；
不把任一超时/中止/pending写成绿，不降低bcrypt生产成本或修改其它模块以获得绿灯。

frontend762/762、Vite build、Compose通过。隔离Go+SQLite真实HTTP三账号（管理员旧根、两个
普通用户）验证两个storage入口、源移除后同token重parse/confirm、精确Reader正文与foreign
隔离；WebDAV真实HTTP额外验证token-only不接收换成link的mounted根。真实Chromium1228
1440×900/390×844/360×800上传取消/单本目录刷新/批量/逐本确认通过；默认1234路径缺失的
第一次失败不算绿。最终source-built二进制及扩展smoke已复跑，三个视口都将改numeric目录规则后
同token确认的Book直接打开Reader，实际第一段正文可见、无章节失败/横溢出，无章节响应mock。

非root Linux arm64上使用新编译的service/native测试二进制，断网/只读/私有tmp，在既有8dc61c3
runtime中全部通过（不是新候选Docker或发布证明）。CGO-disabled完整app交叉编译因既有SQLite
ErrBusy/ErrLocked类型缺失失败，不能声称完整跨架构编译；完整应用必须由Go1.24/CGO可信发布
流程验证。隔离开发Docker Go1.24/CGO/native arm64构建成功（PUSH=0，local tag标注dirty，
旧中间bundle且非GHCR；不冒充最终candidate），最终同SHA Docker/fresh/historical/portable/
backup及可信双架构/OCI仍待办。

所有日志 `/private/tmp/openreader-import-stage-*`；没有生产写入/迁移/旧root清理。Git当前阶段
准备形成feature checkpoint、未推main/发布；已发布app仍5cfc53d/index ba5456fe…，生产只读health独立再次核验
完整db1ea216f9849bc44a90b5b760241df1c6d069b0/status ok。其生产阅读已恢复事件保持关闭。
完整parser/SQL/category/archive原子取消、跨进程exactly-once、扫描cardinality/depth/FD上限、
desktop click/wheel复审、dependency advisory与整审计均仍未完成。

## 11. 候选验证及创建所有权复审（2026-10-10，仍未发布）

feature checkpoint `0a40298cab145941bebdf0e7a3d7a7739796dd38` 已远端精确核验，main仍4104499。
该确切提交完整API race终态通过（878.190s；service2.557s/root2.319s）。Go1.24/CGO本地arm64
候选镜像及fresh/portable/backup/restart通过；historical首轮HTTP404保留失败日志，未定位原因，
随后诊断与原始sh独立复跑全通过，不将未复现写为已定位修复。它不是GHCR新发布或生产证明。

随后人工复审在上述第4节exclusive创建/owned补偿合同内发现新窗口：原随机token `.json`
碰撞时，0a创建raw后metadata写入拒绝，但补偿将已接收的旧metadata误认作自有并删除；仅
`.parsed.json`碰撞时仍接受新stage。先添加确定性entropy反例，再改应用，不能发布已知红灯。
`TestStageCreateCollisionCannotOwnPreexistingBundleFiles` 三个后缀都断言真实生成token碰撞接缝
已触发；`.book`本已安全拒绝是绿色控制，`.json`实际旧文件被删、`.parsed.json`实际接受及
产生额外文件为两个红灯（service0.635s，exit1），没有编译失败或概率等待。仅测试自有目录，
crypto/rand.Reader于每项cleanup恢复；不修改生产随机源。日志
`/private/tmp/openreader-import-stage-token-collision-red.log`。这不是生产复现，也不混算旧应用120。
修复之后须重跑候选源码回归/真实浏览器及同SHA镜像卷门，再推main与可信Actions发布。

独立red `7b47c8d62a0d0681bc00c5ae428f1b027f624b59` 已在feature远端精确核验，main4104499
不变，随后才加创建guard：原session验收后、任何cleanup/写入前，三entry有任一个原inode即
返回既有ErrStageWrite。没有重用旧bundle或改token格式；不引入自动碰撞重试/新API。
三个后缀控制全部转绿（service0.467s），120初始反例及30补充控制继续绿；最后修复版fullGo
API100.565s、vet、exact race/API72.899s/service2.114s/root1.402s、frontend762/build/Compose、
三账号真实HTTP、Chromium三个视口confirmed Reader及非root Linux arm64 service（含新碰撞反例）通过。新确切提交完整race、候选
Go1.24/CGO Docker新旧卷/portable/backup以及可信双架构GHCR/OCI仍需验证，未发布。

## 12. 确切候选发布与独立核验（2026-10-10）

应用提交 `b2b32f7b40bb8cefc18d316aefb29865aede9872` 在上述全Go/vet、frontend762/build、
精确race、非root Linux、三账号HTTP与真实Chromium三个视口之外，完整API/service/root race
终态通过（API870.086s/service2.107s/root3.380s，25m预算；没有降低bcrypt成本）。该SHA的本地
Go1.24/CGO arm64候选通过原始sh新卷及历史卷门：TXT/EPUB/UMD/CBZ、相对缓存、归档hash、
owner隔离、logical/portable-v1/v2-assets备份恢复及重启。保留测试目录与早先0a历史404失败记录，
该旧失败原因仍unknown，不能写为已定位修复；最终b2两门均一次通过。

本地门全部终态成功后才将main4104499快进到确切b2（force=false）。可信
[Actions run 38040641233](https://github.com/changshengyu/openreader/actions/runs/38040641233)
完整headSHA一致且终态success，重新通过后端、前端、build/Compose、真实HTTP、native镜像、
fresh/historical/portable/backup和发布平台门，正式发布：

- `ghcr.io/changshengyu/openreader:b2b32f7`、`ghcr.io/changshengyu/openreader:latest`；
- 两标签独立公开Registry API读取与bytes SHA256核验，同一OCI index
  `sha256:ed57cc69e10e2bfcc90f401fee578f13c2df513faee97123886416ebbf92f4dc`；
- amd64 manifest `sha256:ffa4c1bb0dc3b86d86f2cc9200519e0a39e863c526206079c709055a915e9a02`；
- arm64 manifest `sha256:892857fd926f1aad1d241a2b3c686f5cebf3f7b3a649632fe8a0b642bb154789`。

两平台实际config均验证linux/对应architecture与完整b2 OCI revision，不仅信任index平台名称或
Actions日志。核验日志 `/private/tmp/openreader-import-stage-registry-b2b32f7.json`；访问令牌只留
内存，无凭据日志。此证据不是GHCR回拉容器health或生产运行证明。

生产只读health最后确认完整 `db1ea216f9849bc44a90b5b760241df1c6d069b0`、status ok，运行在
另一台Mac；没有生产写入或升级。原书已恢复事件仍关闭，设备升级验收尚未完成。允许差异仅原
多用户/随机token/TTL、bounded native/ctx、owned cleanup与durable成功保留；不改UI、schema、
旧URL、data/cache/library或备份格式。完整parser/SQL/category/archive原子ctx、初始library归档
所有权、跨进程exactly-once、扫描预算、desktop click/wheel、dependency advisory和整体审计仍未完成。
