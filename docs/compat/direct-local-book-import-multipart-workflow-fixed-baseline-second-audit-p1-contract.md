# 直接本地图书导入与 multipart 边界第二轮固定基准合同（P1/P2）

状态：**inventory-complete / implementation-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
当前审查基线：`OpenReader@a0a0b31`。  
审查日期：2026-08-16。

本轮重新审查首页“导入书籍”的直接浏览器文件选择、预览、批量/逐本确认，以及以下已部署
multipart 动作：

- `POST /api/imports/books/preview`
- `POST /api/imports/books`
- `POST /api/imports/txt`

TXT/EPUB/UMD/CBZ parser、不可变 stage、prepared snapshot、归档补偿、分类多对多和 LocalStore/WebDAV
导入已由既有合同签收，本轮不重建这些内部语义。合同阶段只修改文档；失败测试和实现必须后续独立
提交。

## 1. 固定上游权威行为

权威文件：

- `web/src/views/Index.vue:260-269`：隐藏文件输入含 `multiple="multiple"`；
- `web/src/views/Index.vue:2259-2399`：`onBookFileChange`、`importMultiBooks`、
  `waitForImportBook`、`customImportBookInfo`；
- `web/src/views/Index.vue:895-1001`：单书确认可编辑书名、作者、分组和 TXT/EPUB 目录规则；
- `src/main/java/com/htmake/reader/api/YueduApi.kt:217`：`POST /reader3/importBookPreview`；
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:212-270`：认证后遍历
  `context.fileUploads()`，按提交顺序准备每本 TXT/EPUB/UMD/CBZ 并返回 preview list。

固定上游的可见状态机是：

1. 用户一次可选择一本或多本本地图书，文件顺序保持浏览器选择顺序。
2. 单本预览直接进入“导入本地书籍”确认，可编辑书名、作者、分组和适用格式的目录规则。
3. 多本预览先出现不可通过点击遮罩或 Escape 绕过的方式选择：`批量导入` 或 `逐一确认导入`。
4. 批量导入先统一选择分组，然后按 preview 顺序逐本保存；单项失败不把已成功项回滚成伪事务。
5. 逐一确认显示 `（i/n）`；确认、取消当前项后继续下一项，完成/关闭才结束整批。
6. LocalStore 和 WebDAV 的多项预览进入同一个 `importMultiBooks` 状态机，不存在三套互相漂移的
   书名/分组/规则确认业务逻辑。

上游把整批文件放在一个 multipart 请求，用服务端工作目录路径承载待确认文件，并使用弱文件名覆盖、
无显式包络/数量上限和 manager-session 认证。OpenReader 不复制这些传输、路径和安全边界；产品合同是
上面的多选与确认状态转换。

## 2. 当前 OpenReader 证据

- `OverlayBookImport.vue` 的 `el-upload` 没有 `multiple`，只显示一个 `draft.file`；选择后立即进入一套
  独立的书名/作者/分组/规则表单。
- `useOverlayBookImport.js` 只拥有一个 file/token/preview，并直接调用 `bookshelf.importTXT`；它没有
  多项方式选择、顺序队列或批量分组状态。
- `OverlayStorageImport.vue` 与 `useStorageImportWorkflow.js` 已拥有固定上游要求的单本确认、批量/逐本
  方式选择、统一分组、逐本失败/跳过、目录重解析和登录失效隔离，但只接收 LocalStore/WebDAV path。
- `previewLocalBook` 和 `bookshelf.importTXT` 每次发送一个 `file` 或一个 `importToken`，preview 返回单个
  `{title,author,chapterCount,chapters,importToken}`，import 返回单个 `201 Book`。该 wire 已部署且已有
  旧客户端/测试，不应为了恢复多选而破坏响应 shape。
- `imports.go#readLocalImportPayload` 在任何 `MaxBytesReader` 之前调用 `PostForm`/`FormFile`。Gin 的默认
  `MaxMultipartMemory` 只是内存到磁盘的阈值，不是请求上限；超大 declared/chunked body、额外 part
  和超长 scalar 可先被完整解析到内存/临时磁盘。
- `FormFile("file")`、`PostForm` 和 `PostFormArray` 只采用当前逻辑关心的值。重复 file/token/title、
  file+token、未知 file/value part 或过量 category IDs 会被静默忽略、降级或部分采用。
- handler 不显式调用 `MultipartForm.RemoveAll()`；生产 `net/http.Server` 通常会在请求结束清理，但
  直接 handler、嵌入调用和所有返回分支没有明确的临时文件所有权。

## 3. 差异矩阵

| 合同点 | 固定上游 | 当前行为 | 裁决 |
|---|---|---|---|
| 直接选择数量 | 一个 chooser 可多选并保持顺序。 | 只能选择/预览一本。 | **must-fix visible workflow**。 |
| 单本确认 | preview 后编辑当前书并确认。 | 独立单书表单可完成相同动作。 | **partial**：字段能力保留，但必须迁入共享状态机。 |
| 多本方式 | 明确选择批量或逐一；关闭方式选择取消整批。 | 不存在。 | **must-fix state machine**。 |
| 统一业务流 | 直接、LocalStore、WebDAV 都进入 `importMultiBooks`。 | direct 与 storage 两套 composable/dialog。 | **must-fix architecture/behavior**：收敛到一个确认控制器。 |
| preview wire | 一个 multipart 可含多个任意名字 file，返回 list。 | 一个 `file`，返回单 object。 | **acceptable Go/Vue adaptation**：保留单 object API，由前端按顺序逐文件预览并聚合。 |
| 不可变确认 | 上游确认使用服务端已准备文件。 | token 后续只读 caller-scoped stage。 | **aligned security adaptation**：不得在确认时重传或重读浏览器 File。 |
| multipart 总量 | 上游共享 BodyHandler，无模块精确合同。 | direct routes 无 actual-read 总包络。 | **must-fix security boundary**。 |
| multipart shape | 遍历所有 upload。 | 只采用第一个匹配 part/值。 | **must-fix request identity**：稳定单对象 API 每次只接受一个 file 或 token。 |
| 临时文件 | Vert.x 工作文件由框架/控制器处理。 | Go handler 不拥有 `RemoveAll` 生命周期。 | **must-fix resource ownership**。 |
| 数据格式 | 工作目录准备后 `saveBook`。 | 24h stage + parsed snapshot + SQLite/library transaction。 | **acceptable multi-user/durability adaptation**；无迁移。 |

## 4. 目标前端合同

1. 首页和侧栏的同一个“导入书籍”命令打开支持 `multiple` 的浏览器选择器；可见新导入格式仍严格为
   `.txt,.epub,.umd,.cbz`。可保留拖放入口，但不得先显示第二套书名/分组确认表单。
2. 一次最多接收 64 个选择项。零项不改变现有 flow；超过 64 项或任何一项后缀不可见时，整次选择在
   发起 preview 前失败并清空 input，使同一文件可再次选择。隐藏兼容 API 的 `.text/.md/.pdf` 不重新
   出现在 chooser。
3. 每个文件通过现有 `/api/imports/books/preview` 单独上传一次，最多一个请求在途，并按原始选择顺序
   聚合。这样保留单 object API、每文件 128 MiB 配置边界和可取消网络工作，不创建新的 multi-response
   兼容面。
4. 每个 row 必须有与文件名分离的稳定 client identity；两个同名文件仍是两个有序项，Vue key、
   preview 结果、reparse 和 import 不能以显示文件名互相覆盖。
5. preview 成功后 row 只保留服务端 `importToken` 和返回元数据供后续 reparse/import。最终确认不得
   再提交原始浏览器 File；取消/跳过的未消费 token 仅按既有 24 小时 derived-cache 清理，不创建 Book、
   archive、category relation 或 event。
6. 一个成功 row 直接进入共享单本确认；多个成功 row 进入共享 `批量导入/逐一确认导入` 选择。若有
   parser 失败 row，先进入现有 preflight，显示安全错误并允许有效项继续；失败项只有持有 token 且
   格式可配置时才能对同一 stage 重解析。
7. 批量确认只统一明确选中的分组；每本保留 preview title/author/rule，并按选择顺序调用现有单 token
   import。失败计入 summary 后继续下一项，成功项立即使用服务端 Book 更新书架。
8. 逐本确认允许编辑 title/author/categories 和 TXT/EPUB rule，显示 `（i/n）`；取消当前项只跳过该项，
   关闭方式选择或统一分组对话框取消整批。单项 import 失败留在当前项供重试或跳过。
9. 关闭 flow、选择新批次、账号失效或组件卸载必须 abort 当前 preview/import generation；旧响应不能
   打开 dialog、覆盖 row、更新旧账号书架或显示迟到错误。已在服务端成功提交的 durable Book 不伪装
   回滚，后续由权威 shelf reload/sync 收敛。

直接文件只扩展 `useStorageImportWorkflow` 的 source adapter/row identity，不复制其 phase、队列、分组、
错误和账号隔离逻辑。`OverlayBookImport` 可以退化为文件选择宿主或删除，但不能继续拥有第二套确认状态机。

## 5. direct import multipart 精确合同

### 5.1 通用顺序与包络

- 三个路由继续位于 `AuthRequired`/activity middleware 之后。缺失/无效 JWT 的超大 body 先返回现有
  `401`，handler 不解析 multipart、不创建 stage、用户 cache/library 目录或数据库行。
- 每个请求完整 multipart wire body 上限为 `maxLocalImportBytes + 1 MiB`，使用饱和加法。
  `Content-Length` 大于上限在解析前返回
  `413 {"error":"local import request is too large"}`；chunked/未知长度在实际读取上限后一字节时返回
  同一状态和错误。精确上限进入正常 shape/file admission。
- 请求必须是合法 `multipart/form-data`。成功获得 `request.MultipartForm` 后，handler 在所有成功/
  失败返回分支显式 `RemoveAll()`；解析中途失败沿用标准库自己的部分临时文件清理。清理失败不得覆盖
  已确定的业务响应或删除已发布 archive/stage。

### 5.2 唯一 source 与字段形状

- source 必须二选一：恰好一个名为 `file` 的 file part，或恰好一个 `importToken` scalar。两者同时、
  两个同名 file、其它名字 file、重复 token 或两者都无均为 `400`。缺失 source 保留
  `{"error":"file or importToken is required"}`；其它 shape 错误使用
  `{"error":"invalid local import request"}`。
- `preview` 允许的 scalar 只有 `importToken,title,author,tocRule`；两个 import 路由另允许
  `categoryId,categoryIds`。未知 scalar、在 preview 提交 category、重复 title/author/tocRule/categoryId
  均为 invalid request；`categoryIds` 是唯一允许重复的字段，也继续接受每个值内的逗号分隔兼容形式。
- token trim 后必须是既有 48 位小写十六进制格式；file source 不能附带 token。token source 不允许
  file part，也不得访问浏览器或 mounted 原文件；不存在、过期或外用户 token 继续返回安全的
  `invalid or expired local import token`。
- file 名必须是有效 UTF-8，trim 后 1..255 bytes，不含 NUL；原名只参与显示 title fallback、扩展和
  caller-private archive 构造，不能选择 host 路径。`FileHeader.Size` 与实际读取继续受
  `maxLocalImportBytes` 限制，超限保留 `413 {"error":"local book exceeds maximum import size"}`。
- `title` trim 后最多 240 UTF-8 bytes，`author` 最多 160 bytes，`tocRule` 最多 16 KiB；非法 UTF-8、
  NUL 或超限返回 invalid request。省略/空白继续使用 parser/文件名默认值；合法空 rule 保留自动规则。
- `categoryId/categoryIds` 每个原始 scalar 最多 32 bytes，原始值总数最多 200；只接受逗号分隔的十进制
  非负整数。`0`/空片段保持未分组兼容，正数去重后按首次出现顺序完整执行 caller-owner 校验；任一
  malformed、overflow、外用户 ID 或超过 raw cardinality 在 importer/DB/event 前整体 `400`。

### 5.3 成功与失败副作用

- 新 file preview 在 parser 前创建 caller-scoped immutable stage；成功仍返回
  `200 {title,author,chapterCount,chapters,importToken}`，显式无匹配目录仍是可确认的空目录。
- 新 file parser 失败继续返回 `400 {error,importToken}` 并保留可重试 stage；token reparse 失败保留
  上一次成功 prepared snapshot。shape/metadata/body 拒绝发生在 stage 前，响应不得制造 token。
- `/api/imports/books` 与兼容 alias `/api/imports/txt` 成功继续返回 `201 Book`，只在 durable DB/archive/
  category 写入成功后消费 token并广播一次。失败保留 token/snapshot，且无新 Book/chapter/category、
  orphan archive 或 event。
- 不新增 `/reader3/importBookPreview` 兼容路由，不把当前单 object response 改成 list，也不接受一个
  request 内多个 file；多选是前端 adapter 对稳定 API 的有序组合。

## 6. 数据、迁移与允许差异

- 不修改 SQLite schema、model、stage token、`cache/import-previews/<user-id>/` 布局、24 小时 TTL、
  `library/` archive、`data/cache/library` 根、backup/WebDAV/portable 格式或浏览器持久 key。
- 既有两文件/三文件 stage、历史 `.text/.md/.pdf` API 导入、本地书和 oversized 历史标题不扫描、
  截断、移动或删除。新的 wire/field gate 只作用于未来直接 HTTP 请求。
- 固定上游的一 multipart 多文件 list 被“顺序单文件 request + shared frontend aggregation”替代，是
  保持 OpenReader 已部署 API 和 caller-scoped stage 的技术/安全适配；可见多选、顺序、方式选择和
  逐本确认必须等价。
- JWT user ID、随机 token、不可变 stage、SQLite/category many-to-many、补偿事务和安全 parser limits
  是允许差异；上游 manager secret、共享工作路径、原文件名覆盖和无界 multipart 不是产品合同。

## 7. 测试先行门禁

应用代码修改前，新增/替换测试必须先在 `OpenReader@a0a0b31` 精确失败：

1. 前端一次选择 1、2、64 个有效文件；单本直达确认，多本进入方式选择，批量/逐本按原顺序运行；
   65 个或含不可见格式时零 preview。两个同名文件保持两个独立 row/token。
2. direct adapter 对每个文件只上传一次且最多一个 preview 在途；成功后 reparse/import 只提交 token。
   中途 parser failure 进入 preflight；取消、新选择和账号失效 abort 当前 generation，迟到响应无 UI/
   shelf 副作用。
3. shared workflow 同时覆盖 direct、LocalStore、WebDAV：批量统一分组、逐本编辑/跳过、单项失败重试、
   `（i/n）`、关闭方式选择和账号隔离；删除旧单文件业务流的固化测试。
4. 三个路由的 declared/chunked `maxLocalImportBytes + 1 MiB + 1` 精确 `413`，无 token 同体仍 `401`；
   精确包络和精确 file bytes 进入正常 admission，file limit +1 保留既有 413。
5. preview/import/alias 的 file+token、重复/额外 file、重复单值、未知 scalar/file、preview category、
   201 个 category scalar、malformed ID、超长/非法 UTF-8 filename/title/author/rule 均为 400，并证明
   零 stage/book/archive/category/event。
6. token-only preview/import 不访问原 File；外用户/过期 token 安全失败。合法 repeated/comma categories
   保持首次顺序、去重和完整 owner validation，两个 import 路由成功 shape 一致。
7. 将测试 router `MaxMultipartMemory` 降到极小，证明 success、parser/file/shape rejection 后
   `multipart-*` 均不可再打开；成功 stage/archive 仍存在，清理不误删 durable 文件。
8. focused 后运行 Go full/race/vet、frontend 全量/build；真实 HTTP 覆盖 declared/chunked、临时文件和
   token-only；1440x900、390x844、360x800 覆盖 direct 单/多选和两种确认。发布前再跑 fresh/historical
   `data/cache/library`、logical/portable、跨用户和重启门。

## 8. 实施边界

- 后端使用 direct-import 专用窄 helper，在认证之后、任何 `PostForm`/`FormFile`/stage 之前统一执行
  包络、`ParseMultipartForm`、shape/metadata/category admission 和 `RemoveAll`。不得修改全局 Gin
  `MaxMultipartMemory` 或增加会改变其它路由错误优先级的全局 body middleware。
- handler 从已验证 form 读取字段，不再混用会隐式二次解析/首值降级的 `PostForm`/`FormFile`。
  importer/service/parser 不负责 HTTP shape；其现有 size/parser/durability gate继续作为第二道防线。
- 前端优先给 `useStorageImportWorkflow` 增加 source adapter 和稳定 row identity，使 direct 复用其 phase；
  不通过复制 `importMultiBooks` 或保留 `useOverlayBookImport` 确认逻辑来“实现”多选。
- 合同、红测、实现、runtime/browser/发布记录依次独立提交。实现前状态保持
  `inventory-complete / implementation-pending`，不得因旧单文件测试仍绿而标记 aligned。
