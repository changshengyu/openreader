# 运行时与依赖 advisory 第二轮固定基准合同（P2）

审查日期：2026-10-15。固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
初始源码基线：`36e0697f777d54a2df224cc0d9df9d10eba25781`；当时已发布应用：`b4ff9ca`。
当前状态：**Go-foundation-regression-validated / Docker-b2632a4-published / wider-advisory-open / awaiting-device-verification**。

2026-10-16下一阶段已单列
[`frontend-dependency-advisory-fixed-baseline-second-audit-p2-contract.md`](frontend-dependency-advisory-fixed-baseline-second-audit-p2-contract.md)：
固定上游客户端再提取、8high包适用性/必要patch与有界测试、全锁npm发布门；其第8节已记录
fc4692c双架构正式交付（Actions38868093878及独立实际产物/完整镜像验证），取代早期仅合同快照。
OS/C仍由[`sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md`](sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md)
继续，当前新合同尚无实现，不把Go/npm成功当C或WAL-reset覆盖。

2026-10-22：Go 门因标准库 GO-2026-6809（修复于 Go 1.26.10）失败，工具链升级见
[`dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md`](dependency-advisory-release-blockers-fixed-baseline-second-audit-p1-contract.md)；第10–12节结论不重开。

2026-11-12：Go 门因标准库 GO-2026-6859（`net/http`，修复于 Go 1.26.11）失败，工具链升级见
[`dependency-advisory-go-1.26.11-maxbytesreader-p1-contract.md`](dependency-advisory-go-1.26.11-maxbytesreader-p1-contract.md)；第10–12节结论不重开。

初始提交仅合同；第7–9节记录独立红测/早期checkpoint，第10–12节为基础链实施/验证/发布证据。不是生产受攻击结论；
已恢复的原书故障不重开，生产另一台Mac不升级、不写入。旧“npm8high/advisory待审计”记录由本项
开始逐证据收敛，不以扫描数量或版本陈旧直接宣称可利用。

## 1. 固定上游和必须保持的产品合同

本轮从 pinned raw source 再读：

- `src/main/java/com/htmake/reader/api/controller/BaseController.kt:87–172`：登录生成独立token，
  保存当前namespace会话；自动登录核验用户及token期限，续期既有会话。上游不是Go JWT，不能
  将依赖漏洞或版本号当上游功能要求；OpenReader JWT/SQLite/多用户为已签约适配。
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeRule.kt:204–244,270–326`：列表和文本
  根据规则mode分派，XPath参与同一变量/替换/组合链。
- 同目录 `AnalyzeByXPath.kt` 全文：节点/元素和文本求值，以及`&&/||/%%`合并、回退和交错。
  不从当前Go节点适配器推导JVM标量XPath完整对齐，更不复制Go依赖的无限循环。

已签收的认证会话、source evaluator/fetcher、HTML安全处理、Reader、local import、WebDAV和backup
合同保持。安全适配可以修补不终止/过量分配、维护支持中的构建链，但不得改成功协议来迎合依赖。

## 2. 初始取证及扫描精度

官方工具`golang.org/x/vuln/cmd/govulncheck@v1.9.0`，数据库`https://vuln.go.dev`，本次
`db_last_modified=2026-10-08T22:31:09Z`。只读元数据/扫描不改变`go.mod/go.sum`；工具通过
独立`go run ...@v1.9.0`使用缓存，未添加为应用依赖。官方文档说明JSON模式即使发现漏洞也exit0，
因此必须检查消息和findings，不能将工具exit0称“通过”。

- 当前源码source/symbol扫描使用本机`go1.26.5/darwin-arm64`，不含test；246条返回OSV中，
  57条有版本/包匹配、28条有报告的符号路径。28由x/text1、jwt1、xpath1、x/net9、stdlib16组成。
  静态接口/反射和OS前提仍需复核；不是28个已复现漏洞，也不是正式Linux工具链扫描。
- 已核验自有`openreader-import-durable-local:b4ff9ca`完整OCI revision后，以非root、断网、只读、
  无挂载/端口的短命cat容器提取`/app/openreader`；没有运行服务，容器已移除。`go version -m`
  证明实际`go1.24.13`、CGO1、linux/arm64和现有模块版本，不拿宿主1.26.5冒充候选。
- 对这份精确候选的binary扫描匹配76条，但官方extract仅有build/module信息，**pkgSymbols=0**。
  已读官方v1.9.0 `internal/vulncheck/binary.go:105–114`：该情形退化为go.mod级精度并枚举已知
  affected symbols；输出仍标symbol不等于实际可达。`go mod why golang.org/x/crypto/ssh`明确
  main不需要SSH。因此76只能记模块级保守匹配，不能记76实际符号/生产漏洞。
- `npm audit --json --package-lock-only`终态exit1，8个high受影响包、无critical/moderate包计数；
  一个包可关联多条不同严重度advisory。未执行fix/install脚本、未改lock；8不等于8独立漏洞。

本地证据：`/private/tmp/openreader-dependency-govuln-source-initial.json`、
`/private/tmp/openreader-dependency-govuln-binary-b4.json`、
`/private/tmp/openreader-advisory-b4.JjgmeY/govuln-extract.json`、
`/private/tmp/openreader-dependency-audit-npm-initial.json`。精度校验器只在tmp，不依赖extract格式
作为长期公开协议；原始结果保留，不删报错或用空输出当clean。

## 3. 可达性、版本和分类矩阵

| 面 | 初始证据 | 裁决与约束 |
|---|---|---|
| JWT malformed分段 | v5.2.1，`authsession.parseClaims→jwt.ParseWithClaims→ParseUnverified`；[GO-2025-3553](https://pkg.go.dev/vuln/GO-2025-3553)指向分段过量分配，v5.2.2修复。 | `must-fix dependency`，需有界自有输入实际分配红测；不改HS256、claims、sessionTTL、legacy默认secret兼容或401。扫描不是行为red。 |
| XPath true布尔列表 | v1.3.3，`sourceRuleXPathElements→htmlquery.QueryAll`；[GO-2026-4526](https://pkg.go.dev/vuln/GO-2026-4526)指向logicalQuery.Select对true条件不终止，v1.3.6修复。 | `must-fix dependency`，必须在可杀的自有worker中真实调用当前evaluator，不在主测试进程泄漏死循环；不删除XPath/简单黑名单或借机重做标量语义。 |
| x/net HTML/IDNA/HTTP2、x/text | v0.33.0/v0.21.0；source符号路径到远程HTML、EPUB、网络策略、文本初始化；HTML死循环/复杂度和HTTP2等有官方OSV。 | `must-fix version baseline`，逐类保持正常解析/编码/namespace/网络策略黄金输出；不能从input byte limit或末尾ctx推导底层CPU问题已修复。 |
| Go标准库和构建工具链 | 正式候选Go1.24.13，宿主1.26.5均有后续安全补丁；[官方支持政策及发布记录](https://go.dev/doc/devel/release)确认1.26.9/1.27.2于2026-10-08发布。 | `must-fix maintained runtime`，选择支持中且覆盖已确认修复的精确patch/index；本轮最低必要目标Go1.26.9，不无故跨到1.27。依赖x/net0.60.0要求Go1.26.0，须显式记录最低构建版本变化。 |
| Alpine runtime与builder | 当前runtime为固定3.20 digest；[官方release表](https://www.alpinelinux.org/releases/)列3.20普通支持止于2026-04-01，之后on-request。 | `must-fix support baseline`，选择支持中的精确官方镜像并核验真实平台/包/CGO/CA。EOL不是某CVE可利用证据；govulncheck不覆盖musl、SQLite C、Node或OS包。 |
| Axios | lock1.16.1，多项Node/HTTP2/原型污染gadget；实际`client.js`仅axios.create实例，普通方法alias，浏览器XHR/fetch。 | `mixed: maintenance must-fix / exploitability conditional`，不称Node代理/HTTP2已部署；[默认实例method advisory](https://github.com/axios/axios/security/advisories/GHSA-9fr6-4gfg-395g)明确区分create/alias。仍核对header/auth/form/fetch边界，不因无Node运行时忽略浏览器gadget。 |
| Vue/SSR | lockvue/server-renderer3.5.34；[维护者SSR advisory](https://github.com/vuejs/core/security/advisories/GHSA-g2v6-rqmx-r4w6)涉及不可信属性键；应用main.js使用createApp，无SSR server。 | `not proven production SSR exposure`；npm修复range与维护者页面“None”不同，目标版本须再用发布/源码/行为核验，不能只相信fixAvailable。正常Reader/Index响应式行为必须回归。 |
| Vite开发服务器 | lock8.0.13；[维护者advisory](https://github.com/vitejs/vite/security/advisories/GHSA-fx2h-pf6j-xcff)要求Windows/NTFS等条件，修复8.0.16；当前dev命令确有host0.0.0.0。 | `development maintenance must-fix`，不是Linux Go生产server的任意文件泄漏；保留已签收静态树防护，不能将修复变成启用生产Vite或SSR。 |
| form-data/nanoid/postcss/source-map-js | lock4.0.5/3.3.12/8.5.14/1.2.1；分别涉及Node multipart、非安全generator边界、构建map读取、indexed map offsets。 | `unknown reachability / maintenance pending`，逐一查依赖链、实际bundle/构建输入及维护者patch；不运行不可信外部PoC、不用omit-dev消除报告。 |
| 其它模块/包/平台 | module-only匹配（例如SSH）、Windows-only、未测试反射路径、SQLite C与基础OS包。 | `unknown/conditional`，单列证据和处置；不能把本机darwin source或剥离binary扫描升级成两架构全覆盖或“所有依赖无漏洞”。 |

## 4. 修复范围和不变项

1. 本轮覆盖实际部署二进制/工具链、Go模块、前端客户端与构建依赖、官方基础镜像和发布审计门。
   目标是已确认advisory修复、或逐条有证据的不可达/不适用说明；不是只把npm数字清零或只改JWT。
2. 选择与当前major兼容的必要修复版本及MVS传递更新，记录精确前后graph、Go最低版本和官方
   registry digest。不做无证据全量latest、`audit fix --force`、GORM/SQLite schema或Vue架构重写。
3. 认证继续Bearer/既有WebDAV/WS入口；旧合法JWT和会话继续可用，不重新签发、强制所有人退出、
   更换secret或修改claims/TTL/error/status；异常输入只做原协议的安全拒绝，不能暴露原token。
4. CSS/JSONPath/XPath/regex/变量/分页、HTML清理、charset/encoding、relative URL、安全网络规则和
   已签收Reader布局/交互保持。布尔XPath须有界结束，但本项不授权新标量产品功能或移除XPath。
5. Go/Alpine变更不能改变public/私有权限、native原实体/取消/补偿、同源/CA/代理和持久路径；
   `data/cache/library`、numeric URL、SQLite/backup、进度/书签/分组、prepared tokens和前端key不迁移。
6. 工具退出码与漏洞结论分开。发布门必须分清source配置、精确候选binary精度、OS/架构/CGO与
   锁文件作用域；解析失败、DB不可用、空文件或剥离符号不能默认为clean。不得为了绿灯忽略dev、
   整类CVE或未验证路径；必要豁免需逐项证据，不能宽泛永久排除。

## 5. 合同→实际红测→实施门禁

此合同和matrix/API/data/security须先独立提交推送、核验精确SHA；之后才添加测试。

1. JWT：固定安全小输入（仅大量`.`、无凭证）调用当前authsession/parseClaims路径；真实分配
   统计需充分预热、定次数和宽裕线性上界，独立正常HS256及错误算法控制。编译失败、版本字符串
   断言或扫描metadata不计行为red；不向生产发送输入。
2. XPath：`@XPath:1=1`和`@XPath:true()`真实evaluator调用，隔离自有subprocess，握手证明已进入
   工作再观察终态，超时必须杀掉并Wait回收。普通节点/text/attribute/false/组合作为控制；不以
   留后台死循环或虚假sleep驱动red，不锁定尚未签约的标量返回内容。
3. HTML/编码/HTTP/parser及其它advisory：优先使用维护者最小fixture或同等自有fixture，逐条
   区分已绿控制与实际red；保留既有完整parser/认证/RootedFS/网络策略测试。
4. 再更新必要模块/锁文件/精确builder/runtime与审计门；补清scan格式、失败模式、平台和工具链
   差异控制。每个readable失败应是实际行为/确定缺少安全版本，不把故意red推main发布。
5. focused/race/fullGo/vet、frontend全量/build、真实Go+SQLite/HTTP+Chromium1440×900/
   390×844/360×800、非root native Linux两平台可执行范围、同SHA候选fresh/historical/
   portable/backup/restart、可信Actions双架构与独立OCI/config/二进制审计全部逐项记证。
6. 修改后的审计结果需注明DB时点和未涵盖C/OS/Node内容。包/符号精度不充分时保留未完成，
   不宣布完整安全审计、全parser CPU取消、跨进程exactly-once或整个重构完成。

## 6. 当前版本和开放项

Git HEAD36e0697，main最后核验9e10ba3（纯发布ledger）；应用Docker b4ff9ca/latest OCI index
`sha256:60e97193265ff47808a3910cae1d0bf8a8cec6a3c7b5b4a12efeb7bab144c6ee`。
生产最后只读仍完整`b2b32f7b40bb8cefc18d316aefb29865aede9872`/statusok；无本轮生产变更。
所有scanner/提取容器已终态，工具缓存/自有tmp证据保留，工作树仍未改应用。
本合同的行为红测、修复、后置审计及全部新候选门尚未进行。完整parser CPU/regex/DOM抢占、
scan/峰值FD预算、late-tx控制、真实设备签收和其它action-level/整体审计仍未完成。

## 7. 独立合同之后的实际旧依赖红测（2026-10-15，10-16恢复核验）

合同 `93fd5fd2e7b4e2c6b04fbd2e98cacf14173a9271` 先独立推送并精确核验，随后只添加两份
测试，模块图、lock、应用和base不变。10-16中断恢复时再次核验远端合同与main9e10ba3；此前
额度错误导致的文档更新没有执行，不能倒填为当时已保存的证据。原测试及日志完整保留。

初轮focused测试终态exit1（engine0.946s/authsession1.064s），**3个leaf实际失败、11控制通过**：

- XPath `@XPath:1=1`：实际`sourceRuleElements→htmlquery.QueryAll`在一个article的文档上
  持续分配，entered-work握手后触发32MiB TotalAlloc看门狗。自有worker以86退出且parent已Wait。
  soft memory limit不是硬RSS保证；看门狗、外层取消和kill+Wait限制实验，不遗留主进程死循环。
  nodes/text/attribute/组合、false表达式、false()和**true()当前也已安全**，共7控制通过；
  不能按advisory描述扩大为所有true规则均已复现。
- JWT custom/legacy-default各1：32KiB只有`.`的非凭证输入，实际Authenticate/parseClaims路径
  16次，仍返回原ErrInvalidSession，但分别分配8,536,528/17,067,504 bytes，超过宽裕的
  四倍input×attempts预算2,097,152。预热及GC后测量，输入分配不计入窗口；无真实token或数据库。
  正常HS256、拒绝HS384和双默认key兼容共4控制通过。

最终测试源码重复3轮仍exit1（engine1.061s/authsession1.242s），每轮同3失败/11控制，总9实际
失败/33控制；custom分配8,530,952–8,542,104、compat17,061,816–17,062,528。focused race
终态exit1（engine8.357s/authsession0.491s），仍3失败/11控制，无DATA RACE、panic、编译失败
或未触发fixture；JWT分配8,543,664/17,074,744。全部自有worker已退出，独立pgrep无遗留。
这些是3个leaf反例、两类问题，不是3CVE或28/76个扫描项均已复现。宿主Go1.26.5运行旧依赖
源码，精确Linux Go1.24候选的额外行为门仍未签收。

日志：`/private/tmp/openreader-runtime-advisory-initial-red.log`、
`/private/tmp/openreader-runtime-advisory-repeat-red.log`、
`/private/tmp/openreader-runtime-advisory-race-red.log`。故意red仅feature checkpoint，不可推main
或发布；实现前先独立提交推送此状态。全量/浏览器/新旧卷/后置审计/可信发布待实现后完成，
第3节其它HTML/text/网络/运行时/base/frontend及整体重构仍开放，不把两个库的反例缩小为最终范围。
无生产请求、数据写入、升级或原书故障重开。

## 8. 两项最小依赖修复与下一基础链（2026-10-16）

第7节的未实施状态保留为历史。独立真实red `8fb3970bb846af353010c42869f40435166c241b`
精确推送核验后，才将jwt/v5从5.2.1改为**5.2.2**、xpath从1.3.3改为**1.3.6**，并用官方模块
工具生成checksum。没有改parser/authsession业务代码、token/session规则、API、schema或默认数据。
此时其它Go模块、go最低1.24.1、前端lock、Dockerfile和workflow均未变。

原3个行为red和11控制全部PASS（engine0.974s/authsession1.153s）；同32KiB输入16次的实际JWT
分配降为8,584/16,064 bytes，低于原2,097,152预算，非更换或放宽测试。完整engine/authsession
race终态PASS（10.528s/2.073s），完整Go终态PASS（API116.144s/engine2.070s/authsession3.200s），
fullvet exit0。Frontend锁文件不变，offline npm ci --ignore-scripts后762/762零skip/build861ms
通过；已有大chunk警告保留，未为绿灯改阈值或安装全局工具。

这些是**两项修复的本地checkpoint**，不是完整API race、真实HTTP/三视口、Linux native、
candidate卷/后置advisory或新Docker发布证明。相关日志前缀
`/private/tmp/openreader-runtime-advisory-jwt-xpath-`，full-go、full-packages-race、frontend-test、
frontend-build及vet分别保留。生产最后只读仍b2；不把未发布源码当作用户已升级。

下一基础链已只读核验、尚未实施：x/net0.60.0要求go1.26.0，并通过MVS要求x/crypto0.57.0、
x/sys0.48.0、x/text0.42.0。官方Go1.26.9已独立下载验证；未改变系统默认Go或项目go指令。
应先补HTML和norm的隔离fixture：官方CL709875包含一个混合table/svg/select的最小文档；
CL794100包含三个invalid UTF-8 bytes。norm.Iter的库级复现不能直接称OpenReader公网可达，
与实际sourceRuleDocument解析路径分开分类。新red/已绿控制需单列，不回写第7节3/11计数。

官方Docker Registry于2026-10-16T01:40:24.859Z核验index实际字节SHA、两真实Linux平台的
manifest/config与版本（只读，尚未pull/build，未改Dockerfile）：

- Go1.26.9-alpine3.23 index
  `sha256:6750308561953d451210ed1a8086d36c8c8bdcbf672d38d9d0d424b8a7a0216c`；
- Alpine3.23.6 index
  `sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0`；
- Node22-alpine3.23，两个config实际Node22.23.3，index
  `sha256:baf676f7d0e552f3231945c2f979055ca121bce128c152f7a34e6bd1728b1c5a`。

记录 `/private/tmp/openreader-runtime-advisory-base-images.json`，匿名bearer仅内存，不输出凭证。
这不是OS/C/Node漏洞全清或实际运行的Alpine版本证明；仍需native/CA/CGO/包清单与新旧卷检查。
后续必须完成整个第3–5节及发布门，不能停在这两个库或用元数据代替候选验证。

## 9. HTML与Unicode追加真实红测（2026-10-16，基础链仍未修改）

第8节计划及两项实现bccaac3远端精确核验后，在同SHA原生受管理后继工作树中，只新增
`backend/engine/parser_dependency_advisory_lifecycle_contract_test.go`；x/net0.33.0、x/text0.21.0、
Go最低1.24.1、Dockerfile/frontendlock均未改。使用第8节已提取官方CL的微小输入，没有生产网络
请求或用户数据，分清应用入口和依赖级实验：

- HTML：真实`newSourceRuleDocument`同时构造CSS/XPath表征；混合table/svg/select文档在实际
  entered-work握手后3秒仍不结束。只杀掉自有worker并Wait，未在主测试进程留不可取消解析。
- NFC/NFKC：库级`norm.Iter.InitString/Next`处理`f3 cc 80`时连续12步仍position0/producedBytes0，
  触发确定的进度断言。不是计时概率，也不是断言不同的Unicode输出；不称这3字节已证明可从
  OpenReader公网入口传入。NFD/NFKD同非法输入、合法NFC/NFKC及普通HTML共5控制通过。

初轮exit1/engine3.959s，**追加3个leaf实际red/5个已绿控制**。最终带进度诊断源码重复3次
exit1/engine10.045s，仍每轮3/5（共9red/15控制）；race同源码exit1/engine8.672s，仍3/5。
无DATA RACE/panic/编译失败/未握手或未回收worker。沿用旧XPath的私有输出握手记录器，不改变
应用解析器、旧14项JWT/XPath测试或任何依赖；这14项另跑全绿（engine0.418s/authsession0.553s）。
日志`/private/tmp/openreader-runtime-foundation-parser-{initial,repeat,race}-red.log`及
`/private/tmp/openreader-runtime-foundation-prior-core-controls.log`。

这是第二批3leaf反例，不是第7节3red重复计数；两个阶段共6leaf，norm仍明确库级。先独立
test/docs checkpoint推送核验，之后才更新Go/MVS/base。故意red不能推main或触发正式发布；
全量回归、候选runtime/卷/审计/浏览器/可信OCI门待基础链实现后重新运行。

## 10. 支持中工具链与最小模块基础链（2026-10-16，候选门尚未完成）

独立追加red `d3cc2d14664de15013fbf70036c6c89a0043ee0a` 远端精确核验后才实施：go指令从
1.24.1改为**1.26.9**，x/net0.60.0及其必要MVS基础crypto0.57.0/sys0.48.0/text0.42.0；
保留前两项jwt5.2.2/xpath1.3.6，不批量latest。go mod tidy生成checksum/移除废弃选中版本，
go mod verify终态all modules verified；实际模块内go version为go1.26.9/darwin-arm64/CGO1。
系统默认Go未修改，最低源码构建版本的提高在中英文README badge/开发要求/技术栈显式记录。

Dockerfile采用第8节已核验的精确官方Go1.26.9-alpine3.23、Node22-alpine3.23和Alpine3.23.6
index pins；保留native前端构建、CGO/vendor、单Go服务、CA复制、data/cache/library挂载、原URL和
原环境变量，不增加迁移。尚未实际pull/build新candidate，不能从pins推出新Alpine/CGO运行证明。

两批共22个focused leaf全部PASS（engine0.533s/authsession0.462s），既有预算/断言不改。
完整Go终态PASS（API126.002s/engine2.716s/authsession2.595s/localbook9.445s），fullvet exit0，
frontend762/762零skip/build819ms、Compose静态配置通过。前端lock未动，本机Node26.4.0离线
ci --ignore-scripts仅用于本地验证，不能冒充固定Node22.23.3镜像构建。完整API/engine/auth/
rootedfs/stage/localbook/EPUB/CBZ race使用25分钟预算已真实运行，API进程已核验live，**未终态**。
日志`/private/tmp/openreader-runtime-foundation-{focused-green,full-go,full-race,vet,frontend-test,frontend-build}.log`。

官方govulncheck1.9.0在真实go1.26.9 source/symbol配置、数据库时点2026-10-08T22:31:09Z完成。
实际报告246返回OSV中只有1条模块级匹配、0包级/0符号级；不是“246漏洞”或CLI exit0=clean。
唯一[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)覆盖不再维护的crypto/openpgp族，
所有版本均无修复，不能靠再升一个crypto版本消除。`go mod why`显示main不需要该包；完整
宿主358包、linux/amd64 CGO1的359包、linux/arm64 CGO1的358包导入图均无openpgp及子包，
保留所需bcrypt（及sha3/blowfish）。这是精确不适用前提，不是宽泛CVE忽略；后续审计门应继续
强制该前提，未来一旦引入OpenPGP不得沿用本结论。

审计原始/summary证据`/private/tmp/openreader-runtime-foundation-source-audit*.json`、三份
`*-packages.txt`及`*-mod-verify.log`保留。Linux导入图只是选择检查，不是两架构编译/运行，
source扫描不覆盖SQLite C、musl、Node/OS包、前端advisory或剥离二进制的真实符号精度。
候选native/HTTP/三视口/新旧卷/portable/backup、准确失败关闭的发布审计门、可信双架构及
独立OCI/实际二进制仍待办。main/Docker/生产未升级；更广frontend和整体审计继续开放。

## 11. 精确候选本地终态及发布门（2026-10-16，尚未正式发布）

实现448720376ba53fbbf3c9d73caa796851899190db精确推送核验后，未再修改Go/模块/前端资产。
同一完整race最终exit0：API1028.510s、engine20.638s、auth4.131s、rootedfs5.237s、stage2.462s、
localbook9.168s、EPUB5.206s、CBZ4.879s；没有重启长测试或将空缓冲日志当成功。
真实source-built Go+SQLite HTTP的三用户、双WebDAV前缀/Basic-Bearer、Range/304/416、旧token
reparse/确认/正文/隔离及邻接目录上传合同通过；全部仅自有临时数据和本机地址。

`PUSH=0`构建精确448本地candidate `openreader-runtime-foundation-local:4487203`，Linuxarm64，
本地image index/id `sha256:3cc9a6335d6af6fdc2100eb4ea64ba73c62aba38374cad181c5db1abe4ff18e0`，
不是GHCR发布digest。实际Go buildinfo1.26.9/linux-arm64/CGO1；固定Node22镜像前端构建通过，
runtime3.23.6/musl1.2.5-r23/CA存在，非root只读断网下动态链接检查通过。apk info因断网有DNS
警告，保留原始日志，不作联网更新/漏洞清零证明。两Linux架构的完整rootedfs独立合同均在
非root/只读/断网/tmpfs容器PASS，amd64为模拟执行官方runtime，非整应用amd64候选签收。

精确本地candidate新卷与历史卷两门均通过：旧用户名登录、TXT/EPUB/UMD/CBZ/相对cache、
原文件哈希、用户隔离、fresh/portable-v1/v2外观资源跨用户恢复及重启。真实该Go+SQLite容器
加独立Chromium1228在1440×900/390×844/360×800的数值resume URL、content200/正文、对称
留白、原生wheel/上下点击翻页及移动工具默认可见/设置不穿透/中心切换均通过；无API mock。
初轮浏览器缓存缺失、临时夹具误期望200而非合同201、误点中央/覆盖区域的失败记录保留；
仅修测试夹具/目标，不改产品或放宽断言。最终日志`*-browser-final-verified.log`，截图
`/private/tmp/openreader-runtime-foundation-browser-sjGVCz`，临时容器全部回收。

实际剥离candidate官方binary扫描仅匹配GO-2026-5932，15finding含7枚举symbol，但extract
**pkgSymbols0**；按官方实现只能计模块级回退，非7条真实可达。实际binaryGo/平台与三图证据
相符，不以source结论抹掉原始binary报告。其余C/OS/Node/frontend不在该扫描范围。
新失败关闭source发布门独立合同ceb6c28→测试35c6330后实施，58合同与真实扫描通过，详见
[`go-advisory-release-gate-contract.md`](go-advisory-release-gate-contract.md)。

所有本地日志前缀`/private/tmp/openreader-runtime-foundation-`，精确二进制/独立tests保留在
`/private/tmp/openreader-runtime-foundation-native.PmXuhH`。正式可信Actions的Linuxamd64候选/
两架构构建发布及独立OCI/config仍待执行；本地arm64不是全部平台证明。当前切片可进入可信
发布门，不称整个advisory或重构完成：前端8high逐项维护、OS/C、parserCPU/scan/峰值FD/late-tx、
真实设备及更广审计继续开放。Docker已发布仍b4ff9ca；生产最后只读b2b32f7，均不能用本地448替代。

## 12. 可信双架构发布与独立实际二进制核验（2026-10-16）

上节未发布状态为历史。本轮实现 **b2632a4c248a53ce7bade8bdba7bdef66b89b236** 已普通快进推送
main，可信 [Actions38841543709](https://github.com/changshengyu/openreader/actions/runs/38841543709)
终态success，全部门通过：backend、frontend762/762、build、Compose、真实隔离HTTP、59审计门
合同、实际Linuxamd64 source审计、nativeamd64候选、fresh/portable-v1-v2/跨用户/重启及历史
TXT/EPUB/UMD/CBZ/相对cache/owner/hash、最终两架构构建推送和平台核验。
CI实际source工具链go1.26.9/linux-amd64/CGO1，同DB2026-10-08、1纯模块OpenPGP、0包/符号；
现场三图前提通过，不以CLI exit0、OSV记录总数或ID单独放行。

已发布 `ghcr.io/changshengyu/openreader:b2632a4` 和 `latest`；2026-10-16T03:06:08.304Z匿名只读
核验两tag相同实际index字节SHA：
`sha256:936939c55f86fe456af4425ac35506fccd7ba93dd2858402446b7ba01d2284ab`。

| 真正平台 | manifest digest | config digest |
| --- | --- | --- |
| linux/amd64 | `sha256:e1e14f024dc5e6622a65e5f952ad99133d2505b59e60ed5bd47e01d10d93f225` | `sha256:a98913b00999d580fc2f2d7766390a49159e9dc4829996d6780f69099e2f0036` |
| linux/arm64 | `sha256:d0e673cac38e03482dcd240bb5154f38f15ba238487d036c315dd2ac326959d7` | `sha256:18f95c6a603133367d56d1f62f06c003b65cd341d2b876ca6a15eb2cf4cd64f5` |

两config的os/arch/revision均精确匹配完整b263。逐层有界下载并核验真实blob SHA/size，经tar仅
stdout读取最终实际`app/openreader`（不向宿主解压路径或执行外部脚本），二进制均含完整revision，
buildinfo实际Go1.26.9/linux对应架构/CGO1和本轮选中模块。完整二进制SHA分别
`80ff811e1e89c4ed2ecaf0e683ef4f7e29862790cd3ce92c9dab5bb6d2d8128e`（amd64）、
`110d1860a2c05a5298baffdd4137f1eaa5549136ec6207e2dc2e92bd6b228d1b`（arm64）。

两个实际发布二进制的官方扫描及extract均终态0、stderr空；都为pkgSymbols0，唯一OpenPGP
15finding/7枚举symbol只能计**模块级回退**，非7条可达或整镜像clean。原始/summary/extract
`/private/tmp/openreader-runtime-foundation-published-{amd64,arm64}-*`保留。
额外将实际已发布arm64二进制只读挂入同pin的本地candidate runtime/同源码Node22资产，真实
health确认完整b263，并通过临时SQLite/HTTP/三视口与第11节相同交互。**不是完整远端arm64
镜像的端到端卷测试**；完整amd64候选卷门由CI完成，local448整镜像arm64卷门单列。
日志`*-published-arm64-browser.log`、截图`/private/tmp/openreader-runtime-foundation-browser-LfXPL4`，
容器已移除。独立registry report `*-published-verification.json`，完整CI日志`*-actions-final.log`。

允许差异仅支持中Go1.26.9/MVS/官方构建基础和安全拒绝不终止/过量分配；没有业务API/token/
schema/旧URL/data-cache-library/backup迁移。源码最低Go版本已显式提高，Docker用户仍直接拉取。
本记录是发布后纯文档ledger，后续Git HEAD可晚于镜像commit，不意味着新应用镜像未同步。
生产另一台Mac本轮未操作/未重新核验，最后独立只读记录仍b2b32f7，不等于现在已升级b263。
原恢复事件关闭；旧扫描8high前端包逐项维护、OS/C、完整parserCPU/scan/峰值FD/late-tx、设备及
更广重构仍开放，不把这个已交付维护切片升级成完整advisory或整个重构完成。
