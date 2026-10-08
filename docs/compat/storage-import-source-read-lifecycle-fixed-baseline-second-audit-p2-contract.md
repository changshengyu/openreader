# LocalStore / WebDAV 导入源读取生命周期固定基准第二轮合同（P2）

状态：**inventory-complete / red-tests-confirmed / implementation-pending**。
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
当前审查基线：`OpenReader@11235c3ff111261dead932fab56fa36f98333fd5`，2026-10-08。

盘点合同772fae2已独立提交推送，之后才补实际接收边界红测；尚未实施。
当前read/list可信运行37747103667已终态success，11235c3/latest OCI独立核验；不从它或旧目录
创建发布推导本项通过。生产health仍db1ea21，无自动升级。

## 1. 上游证据和已部署适配

权威证据通过固定SHA的git show/git grep读取，而非上游checkout HEAD：

- `BookController.kt:2263–2323#importFromLocalPathPreview`：先认证、按webdav选择home及权限；
  POST `path`数组；存在的文件按txt/epub/umd/cbz获取章节预览，缺失跳过，TocEmpty返回空目录。
  未写书架；不具备目录递归或opened-identity安全合同。
- `web/src/components/LocalStore.vue:286–310`、`WebDAV.vue:330–356`：单项/批量选择只调用preview，
  有结果才向Index的共同确认流程发事件；WebDAV请求显式`webdav:true`。
- 既有`workspace-storage-import-p1e-contract.md`、`workspace-storage-import-p1e3-contract.md`固定
  上游先预览、再明确确认；原文件变动后的规则重试和确认必须使用原user-scoped staged token。

本项不改变UI入口、parser格式/规则、目录检测、默认分组或先预览后确认。Go旧API目录递归、额外
格式、JSON/JWT、多用户和24小时token是已记载适配，不把它们再次暴露为新增工作台功能。

## 2. 当前映射和裁决

| 层 | 当前11235c3 | 裁决 |
|---|---|---|
| 路由/认证 | `/api/local-store/import-preview`、`/import`与WebDAV对应POST先权限；有界JSON、路径归一化、用户根。 | aligned/保留，不能先碰FS再授权。 |
| 规划 | `prepareLocalStoreImport`与`prepareWebDAVImport`先规范化、去重，token-only项不创建service，正常项调用同一`localStoreImportFilesWithService`。 | 保留输入顺序、去重、raw paths/items/categoryIds各200项限制与mixed输入拒绝。 |
| 单文件源 | helper用background `service.Stat`；只保留relative/extension，没有已接收文件或ancestor身份。随后每项background `service.Open`重新接收。 | must-fix/源码证据；两个独立安全Open不能证明整个plan→read仍是同一源。 |
| 目录展开 | Stat/Resolve后绝对`filepath.WalkDir(directoryPath)`和`DirEntry.Info`；无request context；skip稳定link/special，支持的文件最终case-insensitive排序。 | must-fix identity/context；不能拿新普通list的隐藏规则覆盖导入目录既有规则。 |
| 字节读取 | `readBoundedLocalStoreImport`/`readBoundedWebDAVImport`独立Open后LimitReader(max+1)，不观察request ctx。 | must-fix source admission/read；保留128MiB默认或配置上限和固定安全错误。 |
| 预览/确认 | preview先stage字节、再Prepare并缓存parsed；确认可仅token并只成功消费；失败和空目录重试保留token。 | aligned/必须保留，不因mounted源读失败重新读取已有token路径。 |
| stage/SQL全生命周期 | stage/load/cleanup及Importer.Prepare/ImportPrepared为其他共享流程，无全面ctx/fd合同。 | unknown/独立后续审查；本项只保护mounted源规划→bounded read交付，不签收完整SQL/缓存提交或TTL删除。 |

这是源码窗口，不是已复现生产漏洞。尤其service11235c3已拒绝每一次接收后的替换，不能再声称
旧Stat内部race仍存在；必须在绝对directory scan、规划到读取之间实际注入并验证fixture fired。

## 3. 目标源读取合同

1. 认证、JSON/cardinality/path规范化仍先行。token-only重试/确认不触碰mounted boundary或lazy
   root，即使该根已删除/变link仍用同一用户token；不将token匹配失败fallback为源路径读取。
2. 对非token项在规划阶段绑定configured boundary、users/user、目标父链与源原始identity，
   直到该项bounded bytes交付前保持同次接收；文件或目录真实替换/link必须拒绝，不把新namespace
   的名字、metadata、byte交给stage/parser。不能仅缓存absolute path、重接收service或最终SameFile。
3. 目录只扫描原opened目录，单组件metadata和no-follow递归，逐层/批检查request ctx。稳定link/
   special跳过且safe neighbor仍导入；明确单个special/link仍沿用invalid path。导入WalkDir当前
   没有隐藏点名规则：保留支持格式的隐藏文件/目录处理，不擅自复用LocalStore普通列表HideDot。
4. 缺失输入源目前跳过，空目录返回空结果，不创建源子路径或Book/token；不能用读取重构统一成
   普通list的404。LocalStore初始root沿用合法lazy初始化，WebDAVReadService不增加EnsureRoot。
5. preserve raw200项检查；目录展开和跨输入去重后最多200项，201返回400 `too many paths`且
   在任何stage/parser/import之前失败。不会静默截断、分页、改变sort或额外取消支持格式。
6. 文件通过同次原父fd NOFOLLOW/NONBLOCK/regular复验；bounded reader每个Read观察原请求。
   返回后的合法rename可从原handle读原字节，不能偷偷重开新路径。原inode并发内容修改不承诺
   immutable全文件snapshot。目录或file admission取消/替换不是可忽略I/O错误。
7. 在mounted源bounded bytes交付至现有stage/parser前再检查request context。该边界之前取消
   不stage、不调用parser/Importer、不创建Book/Chapter/BookCategory、不广播，不返回成功项。
   之前已交付完成的项不能凭本合同承诺批量rollback；stage后、parser/SQL进行中取消留独立审查。
8. 一般文件读取失败继续现有每项安全错误、邻项独立处理。红测阶段固定整请求终止映射：
   request canceled/deadline为500 `{"error":"import source read canceled"}`；已经接收的身份
   变更为400 `{"error":"invalid path"}`。保持已有JSON error形状、不泄漏host path/entity/token/credentials，不新增499；
   不把取消映射为200成功book/token。原文件/邻项/外部诱饵不写、不删、不chmod。

## 4. 数据和 API 保持

无SQLite、配置、root、cache/import-previews格式、token TTL、backup成员或旧URL迁移。
token-only不扩大mounted权限；保持`items`200预览响应`{path,book|error,importToken}`及确认
`{imported:[{path,book|error}]}`、现有category权限/去重顺序和成功后token消费。unsupported明确
文件保留逐项错误，目录只选支持类型。已有128MiB/parsed-text/chapter安全上限不改。
原UI先preview→token reparse→token import、取消零书架写、失败token保留和GB18030/空目录保持。

独立未完成：token stage/read/TTL清理的opened ownership；Prepare/ImportPrepared与SQL/缓存的
request lifecycle；扫描非书文件的大目录cardinality/深度/FD资源上限。不能仅因为结果200项就
声称扫描工作有界，也不能为了本切片任意新增协议错误。具体边界须先独立盘点，不能静默截断。

## 5. 先红測再实现及最终门

合同独立提交后，最小nil-in-production seam应在真正的目录scan和plan→read接收边界触发：

- 两来源×boundary/users/user/parent/target的real-directory/symlink替换，记录fired；断言根外
  诱饵名字/bytes不stage、不解析、不创建book、不发事件。目录内深层替换不能当stable-skip吞掉。
- plan后file真实同名替换、late同inode symlink/FIFO；FIFO有界timeout＋worker/fd清理。
  不重新测试已经修复的服务admission窗口而假称本项旧实现仍红。
- root授权之后、递归深层、bounded Read中、source交付之前真实取消；无成功preview/import项，
  无新token/Book/Chapter/category/event；token-only既有控制不改。阶段必须明确，非全流程原子性。
- controls：管理员historical根、两个普通用户隔离；正常nested/hidden/nonbook/stable link/special/
  unsupported/missing/empty；200/201、cross-input dedup、排序；正常bytes/size/000不chmod；
  现有token-only mountedroot删除或link、TTL/跨用户、规则重试、GB18030、prepared/no-reparse。

实现后focused/full/race/vet、frontend/build、相邻Reader/存储真实HTTP与三视口、Linux双架构/
非root、可信fresh/historical/portable/backup/platform门逐项记录。红测阶段不称候选通过；
生产db1ea21、用户原书已恢复，未部署/清理任何用户数据。

## 6. 独立红测取证（2026-10-08，实施前）

合同772fae2已推送后，只插入nil-in-production的directory-scan、两来源file-read和source-handoff
观察接缝，不增加身份/context检查。原absolute WalkDir、background Stat/Open、bounded read、
stage/parser/import保持原逻辑。新增36项确定性红灯，fixture均实际fired：

- 20项planner：两来源×boundary/users/user/parent/target×real-directory/symlink在Resolve后替换。
  18项返回`foreign-secret.txt`诱饵名字；2项target symlink得到nil/空计划而非unsafe。后两项不是
  字节泄漏，只是错误接收已变更namespace。原文件未变，不能混称20项都泄漏或认证HTTP全部。
- 12项真实授权Gin请求：两来源preview/import×directory-scan/file-read/source-handoff取消。
  6项preview仍200成功book/token并留下3个暂存文件；6项import仍200成功book、写Book/Chapter/
  BookCategory并广播bookshelf_update。分类为本测试私有fixture，源文件字节未变。
- 4项真实授权Gin同名regular文件在plan后被真实新文件替换。两来源preview实际stage保存完整
  `foreign-secret-bytes`诱饵输入，两来源import实际library保存该输入并写Book/Chapter/广播。
  检查了暂存/library文件字节，不只依赖响应标题；原文件held及替换实体原字节均保持。

独立controls通过：正常nested/hidden导入（与普通list隐藏策略不同）、case-insensitive排序、
nonbook过滤、稳定symlink/special skip+neighbor、missing skip无创建；现有token-only mounted-root
删除/link、同token规则重试、GB18030可读、raw JSON/200项边界。最终controls约1.5s exit0。
完整36项最后一次红测API1.864s exit1（测试本身曾有变量遮蔽编译错误，修正后重新跑；编译错误
不算红灯）。日志`/private/tmp/openreader-storage-import-source-red.log`与controls.log。

独立红测提交保存在codex feature branch，不推送main作为可发布应用；published11235c3的可信
运行37747103667已终态success且exact/latest OCI已核验。此项仍待实现和后续deep scan/bounded Read
取消、late link/FIFO的实际控制、全部权限/私有用户/200展开/去重/相邻回归及候选卷/发布门。
不缩减第5节要求，不把published普通read/list窗口重新描述成未修复。

## 7. 追加深层 / 读取中红测（实施前）

在真实深层目录遍历以及实际文件Read返回非零字节后追加nil观察接缝，不增加ctx检查。
两来源×preview/import的deep-directory-scan和source-read共8项实际红灯：fixture均fired，
仍返回book/token或写书架并广播。读取中测试源大于初始Read缓冲区，不是读取入口取消的别名。
同阶段补明确上述取消500安全JSON裁决，既有20项取消均须满足；不会把无成功项的200误签收。
证据：`/private/tmp/openreader-storage-import-source-deep-read-red-v2.log`。首个重复多段落fixture
导致旧Importer展开大量SQL写入，诊断终止，不算已完成红测；缩小为仍大于初始Read缓冲区的
连续正文后独立重新验证。这些测试与裁决先于实现。
