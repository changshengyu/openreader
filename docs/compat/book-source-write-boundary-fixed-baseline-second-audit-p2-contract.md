# BookSource 写入、导入与配额边界第二轮固定基准合同（P2）

状态：**inventory-complete / implementation-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只提取合同、记录当前反例并定义失败测试，不修改应用或测试代码。范围限定为现有书源管理主链：

- `POST /api/sources`
- `PUT /api/sources/:id`
- `POST /api/sources/batch`
- `POST /api/sources/import`
- `POST /api/sources/remote-preview`
- `POST /api/sources/remote`

无 body 的清空、默认保存/恢复与单项删除继续由 ownership/COW 合同约束。搜索、探索、Reader 换源、
三个旧 test probe、debug stream 和显式 `batch-test` 健康检查已有独立合同，本轮不得借书源写边界
改变它们的网络、失败缓存或可见状态机。

## 1. 权威文件与动作映射

固定上游：

- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `saveBookSource`、`saveBookSources`、
  `deleteBookSources`、`readRemoteSourceFile`、`deleteAllBookSources` 路由；
- `src/main/java/com/htmake/reader/api/controller/BookSourceController.kt`；
- `src/main/java/io/legado/app/data/entities/BookSource.kt`；
- `web/src/views/Index.vue` 的本地/远程预览、选择导入、JSON 编辑、批删和清空流程。

OpenReader：

- `backend/api/server.go`、`sources.go`、`request_body.go`；
- `backend/models/models.go#BookSource/UserBookSource/BookSourceNamespace/User`；
- `backend/services/booksources/service.go`；
- `frontend/src/api/sources.js`、`components/workspace/SourceManager.vue`、
  `SourceTransferOverlay.vue`、`composables/useSourceTransfer.js`；
- `book-source-ownership-p2-contract.md`、`source-manager-fixed-baseline-second-audit-p1-contract.md`、
  `shared-source-fetcher-p2-contract.md` 与 `booksource-metadata-normalization-p2-contract.md`。

固定上游以 `bookSourceUrl` 作为保存/导入身份，在当前用户 namespace 内替换或追加。浏览器先读取本地
或远程 JSON，再让用户勾选，最后只提交所选数组；编辑器要求名称和链接。上游没有可直接移植的
HTTP body、数组数量或 SQLite 配额边界。

OpenReader 保留已部署的 JWT REST 路径、关系型 source ID、当前用户 association、COW、派生失败缓存
和兼容字段转换。`sourceLimit` 是 OpenReader 多用户管理字段，不是 reader-dev 行为；既然管理员 API
允许配置并向用户投影，它必须成为真实的未来新增配额，不能继续是无效果字段。

## 2. 当前反例与裁决

2026-08-12 在 `OpenReader@fe8abf9` 的隔离生产形态服务、临时 SQLite 与临时
`data/cache/library` 根上实测：

- 给普通用户设置 `sourceLimit=1` 后连续五次 `POST /api/sources` 均返回 `201`；数据库最终显示
  `source_limit=1`、五个 active associations，证明配额完全没有消费点；
- 两个连续 JSON object 只消费第一个，返回 `201` 并写入；
- 含约 2 MiB 未知字段的 create 返回 `201`，未知字段被无界封装进 `rules` 后又在响应回显；
- 仅有名称、没有 URL 的直接 create 仍返回 `201`。固定上游 controller 同样不强制 URL，只有可见
  编辑器会校验，因此本轮不把空 URL 重新定义为服务端错误。

| 合同点 | 审查时 OpenReader | 裁决 |
|---|---|---|
| 单项 create/update wire | 两个入口直接 `ShouldBindJSON`，无 declared/actual-read 上限且忽略第二 JSON。 | **must-fix security adaptation**：各接受一个最大 16 MiB 的非 null JSON object。 |
| batch mutation wire | 已有 300 raw ID 上限，但 body 无界且忽略第二 JSON/垃圾。 | **must-fix**：16 KiB actual-read single-object；保留 300 raw IDs。 |
| remote control wire | preview/import 的 `{url}` 无界且忽略第二 JSON；空白 URL 会进入 fetcher。 | **must-fix**：16 KiB single-object，trim 后空 URL 在网络前 `400`。 |
| 本地文件 bytes | multipart 总体受约 17 MiB 限制，文件实际读取受 16 MiB 限制。 | **aligned security adaptation**：保留，不扩大；授权仍先于 multipart 解析。 |
| 导入 cardinality | 16 MiB JSON 可解码任意多的小对象；remote preview/import 同样无条目上限。 | **must-fix parser/work boundary**：原始数组最多 5,000 项，解码后再进入数据库或响应。 |
| `sourceLimit` | 可由管理员创建/更新并在用户响应中显示，但 create/import/remote/default 均不读取。 | **must-fix multi-user quota**：对用户发起的未来新增原子执行；0 继续表示无限。 |
| owner/COW | CRUD/import 已按 active association、URL identity 和 caller-only COW 执行。 | **aligned**：不能为配额或 decoder 重写为全局 source ownership。 |
| 字段与脚本 | reader-dev 字段、未知 dormant extras 和不支持脚本可保存/往返，但不执行。 | **aligned compatibility/security**：只加总 wire/cardinality 边界，不发明逐字段截断。 |
| 无效/空 import | 可返回 `{imported:0,updated:0,skipped:N}`，但仍清 failure cache 并广播。 | **partial / must-fix side effect**：保留成功统计；没有 durable source mutation 时不清缓存、不发事件。 |

## 3. 共同授权、JSON 与优先级合同

- 六个入口继续先通过 Bearer JWT 和 `CanEditSources`。缺失/无效 token 与禁止编辑分别保持既有
  `401/403`，不得为了检查大小而先读取 body、multipart 文件或 URL。
- JSON control body 只接受一个非 null object。数组、scalar、`null`、空 body、第二 JSON 或尾随非空
  垃圾均进入该路径既有平面 `400`；尾部 JSON whitespace 可接受。未知 object 字段继续忽略或由既有
  BookSource compatibility envelope 保留。
- `POST /api/sources` 与 `PUT /api/sources/:id` 的完整 wire body 最多 `16 << 20` bytes。精确边界进入
  既有 source normalization；+1 返回 `413 {"error":"request body too large"}`。
- `POST /api/sources/batch`、`/remote-preview` 与 `/remote` 的 control body 最多 `16 << 10` bytes，
  overflow 使用同一平面 `413`。declared `Content-Length` 与未知长度/chunked 必须按实际读取一致裁决。
- update 保持当前优先级：权限通过后先解析正 ID 并查询 caller-active source，再读取 body。非法 ID
  保持 `400`，missing/foreign/detached 保持 `404`，即使 body 超限或 malformed 也不得读取 body。
- batch 继续在数据库前限制原始 `sourceIds` 最多 300 项；`enable/disable/delete/group` 及其现有
  `200 {affected,skippedUsed}` 保持。unsupported action、空 ID 和 overflow 不写 association/source/
  failure/variable，也不广播。
- remote URL 在网络前 trim；空值仍为 `400 {"error":"url is required"}`。非空 URL 继续完全复用
  已发布的 HTTP(S)、userinfo、DNS/rebinding、私网 allowlist、proxy、redirect、timeout、response-body
  与错误脱敏边界，不创建第二套 fetcher。

16 MiB 单项上限与现有本地/远程书源文件上限相同，给大型 reader-dev rule 和 dormant extras 留出完整
兼容空间，同时阻止无界内存、SQLite 和响应放大。16 KiB control 上限足以容纳 300 IDs 或一个 URL，
且不应扩散为全局 body middleware。

## 4. 字段、身份与持久化合同

- create 继续接受当前 `bookSourcePayload` 的 OpenReader 与 reader-dev aliases；`enabled`、
  `enabledExplore`、`respondTime`、charset、selector/URL normalization 和未知顶层字段往返不变。
- create 仍只要求 trim 后名称非空；本轮不新增逐字段 byte 上限、不拒绝空 base URL，也不执行
  JavaScript/WebView 规则。`id`、timestamps 和 used-book projection 仍由服务端拥有。
- update 继续是完整 source replacement，而不是部分 patch；caller-active target、COW 后可能变化的 ID、
  caller book/failure remap 和变量清理语义不变。请求中的 `id`、timestamps、used-book 字段没有写权限。
- import 继续接受数组、`bookSources`/`sources` wrapper 和单对象；在 caller namespace 内按 trim 后
  base URL、再按名称 fallback 的现有 identity 更新/新增/跳过。输入 ID 永不成为 owner authority。
- 5,000 项限制按最外层原始 source 数组长度执行，发生在 normalization、COW 和 SQLite transaction
  前。5,000 项仍进入完整业务流程；5,001 项的本地文件为
  `400 {"error":"too many sources"}`，remote preview/import 使用同一错误。16 MiB bytes overflow 仍与
  cardinality 分开映射为 `413`。
- import 的全部 updates/creates 保持一个事务。任一 COW、association、变量清理或 quota 检查失败，
  整批回滚；不得留下部分新源、失效缓存清理或 sync event。

## 5. `sourceLimit` 原子配额合同

- `User.SourceLimit == 0` 保持无限；正值表示该用户可拥有的 caller-active、`detached=false`
  association 最大数。全局 snapshot 数、default namespace、其它用户和 detached 在用源不计入。
- `POST /api/sources` 每次新增一个 active association。配额检查与 source/association 创建必须处于
  同一并发安全事务；两个并发请求不能同时观察剩余一个名额并把最终数量写成 limit+1。
- 本地/remote import 先在事务内按当前 active identity 模拟完整结果。更新现有 identity 和输入内重复
  不占新名额；只有新 active identity 计入 projected count。若 projected count 超过正限额，整个请求
  返回 `409 {"error":"source limit exceeded"}`，不执行其中原本可成功的 update/create。
- 用户已因历史数据、默认初始化、恢复或管理员降额而超过当前 limit 时，仍可读取、导出、删除、
  启停、分组和完整更新已有源；import 若只更新已有 identity 也可成功。任何新增在 active count 降到
  limit 以下并留出名额前都返回 `409`。
- `PUT` 的 COW 会创建新的内部 snapshot，但只替换同一个 association，不消耗 active 配额。
- 默认 namespace 初始化、`default/restore`、管理员 reset、logical/portable/WebDAV restore 与历史卷
  迁移继续按各自已签收的恢复合同执行，不因新 HTTP quota 丢弃数据。它们可以产生超额历史状态，
  后续普通新增按上一条 fail closed。
- quota 失败不清 `source_failures`、不清持久 parser variables、不推进 source/namespace timestamp、
  不创建孤立 snapshot，不广播。错误不披露其它用户、默认源数量、SQLite 文本或规则内容。

该配额是 OpenReader 已有多用户字段的落实，不是对 reader-dev 的产品删减。管理员可通过现有 API
提高或清零限额；本切片不新增管理界面，也不顺带实施独立的 `bookLimit` 合同。

## 6. 成功、事件与历史数据

- create 继续返回 `201 BookSource`；update 返回 `200 BookSource`；import/remote import 返回现有
  `{imported,updated,skipped}`；remote preview 返回 `{count,names,sources}`；batch 返回现有统计。
- durable mutation commit 后只向当前用户广播一次 `sources_update`，kind 保持现有值。空数组、全部
  invalid/duplicate skip、foreign-only batch 或 used-only delete 若没有 source/association/字段变化，
  保留成功统计但不清 failure cache、不广播虚假更新。
- 不新增 SQLite 表、列、索引或 destructive migration，不改 `data/cache/library`、backup/WebDAV、
  default source 文件、浏览器 storage key 或可见 SourceManager 几何。
- 历史 oversized source row 和超额 namespace 仍可 list/get/export/backup/restore/read。16 MiB/5,000 项
  只约束未来这些 HTTP 写入口；备份/恢复继续由各自 archive、entry 和事务预算约束。
- full-document update 若未来提交本身超过 16 MiB 必须明确 `413`，不得截断或部分保存；用户仍可先
  导出历史源。禁止启动时扫描、截断规则、删除超额 source 或自动改写 `sourceLimit`。

## 7. 失败测试先行闸门

实现前必须先让以下合同在当前代码上失败：

1. create/update 的 declared 与 chunked 16 MiB + 1 为精确 `413`；batch/remote 两路的 16 KiB + 1
   同样失败。精确边界、尾部 whitespace 进入原业务状态机。
2. 五个 JSON 路径拒绝第二 JSON、垃圾、数组/scalar/null；拒绝不写 source/association/failure/
   variable/timestamp/event。update 的 foreign/missing target 继续先于 body 返回 `404`。
3. 本地 import、remote preview/import 的 5,000 项成功，5,001 项在 normalization/SQLite/响应前
   `400`；现有 16 MiB 文件/remote response 边界仍通过。
4. `sourceLimit=1` 时首个 direct create 成功、第二个 `409`；配额内 import 成功，projected overflow
   全批回滚。已有 identity 更新和 COW 在满额时仍成功。
5. 两个并发 create、create+import 和两个 import 竞争最后名额，最终 active count 永不超过 limit；
   一个成功后其它请求稳定为 quota conflict，而不是部分提交或不可解释 `500`。
6. 历史/default/restore 产生超额 namespace 后仍可读取、更新已有、导出、备份和删除；不能新增。
   `sourceLimit=0` 保持无限，foreign/detached association 不影响 caller count。
7. quota/cardinality/malformed/overflow/空操作不清 failure cache、不清 book/chapter variables、不创建
   orphan snapshot、不广播；成功 mutation 仍只广播一次。
8. 现有 reader-dev aliases、unknown dormant extras、URL/selector normalization、source ownership、COW、
   used-source guard、default namespace、backup/restore 和 SSRF/fetcher 回归保持。

实现后运行 focused API/service tests、focused race、`go test ./...`、`go vet ./...`、frontend 全量和
production build。该切片无前端几何变化；隔离生产形态真实 HTTP 覆盖 declared/chunked、single JSON、
5,000/5,001、配额并发和零副作用。发布前顺序执行 fresh/historical mounted-volume、source ownership
与 portable backup gates，再由本机完成 amd64/arm64 构建和 GHCR 发布。

## 8. 实施约束

- 复用 `decodeBoundedSingleJSON` 的 actual-read primitive，但由 source handler 映射既有平面错误；
  object/null 检查可放在 source 专用 helper。不得修改 auth/admin/settings/book 已发布的上限或错误体。
- cardinality 必须在完整数组已经有界解码后、任何逐项 normalization/transaction 前检查。不得依赖
  frontend checkbox 数量或 multipart `Content-Length`。
- quota 决策必须在 `booksources` service 的同一写事务内使用 authoritative user row 和 active
  association count；handler 预查只能用于提示，不能作为并发正确性依据。
- 不通过全表 count、全局 source mutex 或复制 owner 字段破坏现有 association/COW 架构。若 SQLite
  deferred transaction 需要窄的同用户写串行化或 bounded BUSY retry，必须以并发红测证明且保持
  跨用户隔离。
- 远程 preview 只解析和返回，不写 source/failure/event；remote import 必须在 fetch+decode 全部成功
  后才进入原子 quota/import transaction。调用取消继续停止 transport，绝不转成成功或失败缓存。
