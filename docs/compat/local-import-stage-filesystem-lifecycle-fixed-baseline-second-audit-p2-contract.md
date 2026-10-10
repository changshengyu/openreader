# 本地图书暂存 token 文件系统全生命周期第二轮固定基准合同（P2）

状态：**inventory-complete / must-fix / tests-and-implementation-pending**。
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
源码审查基线：`OpenReader@5cfc53d4993e0b7c020d5bb8d2747af6e07ab73c`，2026-10-10。

本盘点只修改合同，不修改应用。原书已恢复事件保持关闭；这里只记录当前源码窗口，尚无本项
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
| POST `/api/local-store/import`、`/api/webdav/import` | 200 `{items}`，逐项durable成功；先前成功项与事件保持。 | invalid token安全per-item；prepared filesystem错误同上；保持普通parser错误、格式和权限优先级。 |

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
尚未签收：本项红测/实现/资源并发/runtime/发布；完整parser/SQL/category/archive生命周期；目录
扫描cardinality/depth/FD预算；桌面click/wheel复审、dependency advisory及整体审计。生产独立health
仍db1ea21；已发布mounted-source app5cfc53d，不代表生产升级或新stage问题已解决。
