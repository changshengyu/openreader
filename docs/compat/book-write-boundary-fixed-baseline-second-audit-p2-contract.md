# Book 写入与本地书档引用边界第二轮固定基准合同（P2）

状态：**inventory-complete / implementation-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

合同阶段只提取固定上游行为、当前反例和允许的 Go/JWT/SQLite 安全适配；红灯测试与应用实现随后
分别独立提交。范围限定为已有书架生命周期的以下边界：

- `POST /api/books` 的兼容创建入口；
- `PUT /api/books/:id` 的精确部分更新入口；
- `DELETE /api/books/:id` 与 `POST /api/books/batch {action:"delete"}` 提交后的本地书档清理。

远程书确认继续走 `POST /api/books/remote`，本地 TXT/EPUB/UMD/CBZ/音频继续走已有 import/stage
入口。Reader 换源、刷新、阅读进度、章节缓存、备份格式和 BookInfo 可见编辑器均不在本轮重建。

## 1. 固定上游与当前映射

固定上游权威点：

- `BookController.kt#saveBook` 在认证后把请求映射为 `Book`，要求 `origin/bookUrl`，按当前用户 namespace
  读取书架并以书名作者判重；
- 本地书的临时上传、LocalStore 和 WebDAV 路径由服务端检查存在性、移动或投影到
  `storage/data/<namespace>/<book>/...`，并由服务端执行 EPUB/CBZ 提取；
- 已存在书只由服务端保留阅读位置，再整体替换 namespace 内该书；远程书缺少目录信息时由服务端
  读取书源补全，而不是信任客户端伪造解析状态；
- `Book.kt` 虽是宽 JSON 模型，但路径、目录、进度、时间和缓存字段都处于同一个用户 namespace 和
  服务端文件流程中，不构成任意宿主路径或跨表主键写能力。

OpenReader 已将上游单一 `saveBook` 分解为显式远程确认、本地导入、元数据 patch、分组、刷新和换源
事务。这是已签收的 Go/SQLite 技术栈适配；通用 `POST /api/books` 只保留为旧客户端兼容入口，当前
Vue 生产流没有调用者。该入口不能重新成为绕过专用事务写入存储、解析和进度状态的后门。

## 2. 当前反例与裁决

2026-08-12 在当前 `24b788a` 生产形态服务、隔离临时 SQLite 和临时 `data/cache/library` 根上提交：

```json
{
  "id": 900123,
  "userId": 999,
  "sourceId": 777,
  "type": 1,
  "title": "runtime forged alias",
  "libraryPath": "data/bookwriteaudit1538/victim",
  "originalFile": "source.txt",
  "tocFile": "chapters.json",
  "tocRule": "forged",
  "sourceFile": "source.json",
  "lastChapter": "forged",
  "chapterCount": 99,
  "lastCheckTime": 123
}
```

服务返回 `201`；除强制覆盖 `userId` 外，指定主键、来源、类型、全部书档/解析字段、章节计数和时间均
按客户端值持久化并回显。静态审查还确认：

- create/update 都先 `io.ReadAll`，无 declared 或 actual-read 上限；update 对 JSON `null` 解码为零值
  patch 后仍执行 `Save` 和广播；
- create 先按原始 `categoryIds` 是否非空选择校验分支，再由 `categoryIDsFromRequest` 过滤零值并回退
  `categoryId`；`foreign categoryId + categoryIds:[0]` 可把另一用户分类写进当前用户书籍；
- create 不校验 `customCoverUrl`，可持久化外部 URL 或另一用户的私有上传 URL；
- 删除本地书时只做当前用户名根的词法路径检查，提交后直接 `os.RemoveAll`。若同一用户另一书籍行仍
  引用相同归一化目录，该存活行的原书档仍会被提前删除；当前宽 create 可直接构造这条破坏链。

| 合同点 | 当前状态 | 裁决 |
|---|---|---|
| wire body | create/update 无界整包读取。 | **must-fix security adaptation**：统一 1 MiB actual-read 单 JSON。 |
| create 字段 | 直接反序列化完整 `models.Book`。 | **must-fix mass assignment**：显式 DTO；身份、来源、格式、存储、解析、进度、计数和时间均由服务端拥有。 |
| update 语义 | 已用指针和 raw key 保持精确 patch，但 `null` 仍成功且字段长度无实际 SQLite 约束。 | **partial / must-fix boundary**：保留精确 patch，补 wire/null/未来字段边界。 |
| 分类 | create 可在零数组 fallback 时绕过 owner 校验。 | **must-fix multi-user isolation**：先算最终有效集合，再完整验证 caller ownership。 |
| 自定义封面 | update 已验证受控上传，create 未验证。 | **must-fix capability ownership**：两入口使用同一当前用户 cover 规则。 |
| 本地书档删除 | 事务后清理正确，但不检查存活引用。 | **must-fix data integrity**：仅最后一个有效引用消失后清理；查询失败时 fail closed。 |

## 3. 共同 wire、优先级与错误合同

- 两个 JSON 写入口继续要求有效 JWT。认证 middleware 必须先于 body；缺失/无效 token 保持既有平面
  `401`，不得为检查大小提前读取或记录 body。
- 完整 wire body 最多 `1 << 20` bytes，包含 JSON 标点、转义、未知字段和尾部空白。
  `Content-Length` 与 unknown-length/chunked 使用相同 actual-read 上限；精确 1 MiB 进入业务状态机，
  1 MiB + 1 返回 `413 {"error":"request body too large"}`。
- 只接受一个非 null JSON object。第二 JSON、尾随垃圾、数组、scalar、空 body 或 `null` 均映射既有
  `400 {"error":"invalid book payload"}`；尾部空格、tab、CR/LF 可接受。
- `PUT /api/books/:id` 先解析正整数 ID 并查 caller-owned book，再读取 body。非法 ID 保持 `400`；
  缺失或外用户目标保持 `404`，即使 body 超限、不可读或 malformed 也不得读取 body。
- 拒绝请求不创建/更新 book、category relation 或文件，不推进时间，不广播。错误和日志不得回显
  JWT、body、外用户分类/上传信息、宿主路径或 SQLite 文本。

1 MiB 是 OpenReader 多用户服务的有界输入安全差异，不伪称固定上游限制。它保留大段简介和旧兼容
客户端的宽 JSON 余量，同时把内存、UTF-8 校验和未知字段工作限定在可预测范围。

## 4. `POST /api/books` 显式兼容 DTO

允许写入的字段只有：

- `title`、`author`、`coverUrl`、`customCoverUrl`、`intro`；
- `kind`、`wordCount`、`url`；
- `categoryId`、`categoryIds`、`canUpdate`。

未知字段继续忽略，以兼容旧客户端发送完整 Book object；但下列已知服务端字段必须与未知字段一样被
忽略，绝不能进入持久化：`id`、`userId`、`sourceId`、`type`、`variable`、`libraryPath`、
`originalFile`、`tocFile`、`tocRule`、`sourceFile`、`lastChapter`、`chapterCount`、`lastCheckTime`、
`createdAt`、`updatedAt`，以及未来新增的 cache/progress/chapter/storage 字段。

- `userId` 必须取 JWT；`id` 由 SQLite 分配；`sourceId/type` 保持兼容创建的本地默认零值；
  `lastCheckTime/createdAt/updatedAt` 由服务端生成；计数、解析和路径字段保持空/零。
- `title` trim 后必填。`canUpdate` 缺失时保持既有默认 `true`；显式 bool 保留。
- `customCoverUrl` 为空可接受；非空必须解析为当前用户现存的 `covers` 上传 capability。外部 URL、
  另一用户上传、错误 kind、缺失文件均返回 `400 invalid custom cover url`。
- `categoryIds` 的非零去重集合优先；显式空/全零数组继续按已部署兼容语义回退非零 `categoryId`；
  两者均无正值则不分组。必须先得到这一最终集合，再验证全部 Category 属于当前用户。
- 成功在一个 SQLite transaction 内创建 book 与全部 category relation，返回既有 `201` shelf projection，
  commit 后只广播一次当前用户 `bookshelf_update`。

`variable` 只由 `POST /api/books/remote` 的有界字符串 map 和后续来源事务管理；本地书格式与路径只由
import/stage 管理。兼容 create 忽略这些字段是明确的多用户数据完整性差异。

## 5. `PUT /api/books/:id` 精确 patch

保留已签收的 `book-edit-metadata-p2-contract.md`：只允许显式 patch `title`、`author`、`coverUrl`、
`customCoverUrl`、`intro`、`categoryId`、`categoryIds`、`canUpdate`；未知及服务端字段忽略。未提交的列、
并发保存的分组/追更/进度、章节和缓存均保持。

- 仍以 raw object key 区分“未提交”和显式空/false/null；`title` 显式提交时 trim 后必填；
- `categoryIds` 显式出现时以它的非零去重结果替换关系，空/全零表示清空；只有 `categoryId` 出现时
  才按单值替换。所有最终正 ID 在 transaction 前验证为 caller-owned；
- `customCoverUrl` 沿用当前值、空值或当前用户现存 cover capability 规则；
- 成功只保存显式允许字段和必要关系，返回 `200` 完整 shelf projection，commit 后广播一次；
- `{}` 保持现有 no-op 成功兼容；JSON `null` 不是 object，必须 400 且不得 `Save` 或广播。

## 6. UTF-8 字段与历史数据边界

以下限制按 trim 后 UTF-8 bytes 执行，只检查本次显式提交的字段：

| 字段 | 最大 bytes | 依据 |
|---|---:|---|
| `title` | 240 | `models.Book` 既有 size tag |
| `author` | 160 | 同上 |
| `coverUrl` / `customCoverUrl` | 600 | 同上 |
| `kind` | 400 | create 兼容字段的既有 size tag |
| `wordCount` | 120 | 同上 |
| `url` | 800 | 同上 |

`intro` 没有独立字段上限，只受 1 MiB wire cap。边界值成功，+1 返回稳定平面 400，例如
`book title is too long`、`book author is too long`、`book url is too long`；多字节 fixture 必须证明按
bytes 而不是 rune/UTF-16 单位。

SQLite 不执行这些 varchar size tag，因此这是未来 HTTP 写入验证，不是 schema migration。历史
oversized row 不修改、不回填、不删除，仍可 list/get/backup/restore；PUT 只切换未超限字段时不得因
未触碰的旧字段失败。恢复和内部 parser/import transaction 不复用 HTTP 限制。

## 7. 本地书档引用安全

- `captureBookCleanup` 继续只在 book/chapter 行尚存的 transaction 内收集候选；任何数据库失败回滚，
  不能删除文件。远程缓存与图片保持既有引用修剪逻辑。
- 本地候选仍必须是相对 `libraryPath`，并经当前用户名的
  `library/data/<SafeFilename(username)>` 根校验；非法、绝对、越界路径永不清理。
- 删除 transaction 提交后，清理前重新查询同一用户剩余的本地书籍行。将每个非空 `libraryPath`
  经同一根校验和规范化后比较；只要任一存活行指向同一目录，就跳过 `RemoveAll`。
- 单删或 batch 只删部分重复引用时目录和存活书必须可读；同一 transaction 删除最后几个引用时，
  目录最多清理一次。最后一个引用以后被删除时才清理。
- 查询用户/引用失败、路径无法可靠规范化或任何状态不确定时 fail closed：保留目录。文件清理失败
  仍沿用提交后 best-effort 语义，不回滚已完成的数据库删除，也不把宿主路径回显给客户端。
- 该规则兼容历史污染/重复引用，不新增 refcount 列，不扫描/重写现有数据库，不改变
  `data/cache/library`、portable backup、WebDAV 或 logical backup 格式。

## 8. 必须先失败的合同测试

1. create/update 的 declared/chunked `1 MiB + 1` 均为精确平面 413；精确 1 MiB 与尾部 whitespace
   进入原状态机；第二 JSON、垃圾、scalar/array/null 均 400。
2. 未认证超大 create/update 保持 401；不存在/外用户 update target 对不可读或超大 body 保持 body 前
   404；所有拒绝路径无 row/time/category/event 副作用。
3. create 提交全部服务端字段后仍只生成当前用户、SQLite ID、默认来源/类型和服务端时间；路径、解析、
   进度、章节、变量和计数字段为空/零。允许元数据、`canUpdate` 和合法分类仍按请求保存。
4. `foreign categoryId + categoryIds:[0]` 返回 `400 category not found` 且无 book/relation；当前用户同形
   fallback 成功。其它单/多/空/去重分类语义保持。
5. create 的外部/外用户/缺失 custom cover 被拒绝，当前用户现存 cover 接受；update 现有 cover 合同
   不回退。
6. 每个字段精确 byte 上限成功，+1 失败；UTF-8 多字节边界、历史 oversized row 的读、只改其它字段、
   logical/portable backup/restore 保持。
7. 两个同用户本地 book 指向同一规范目录时，单删或 batch 删一个不能删除文件；删最后一个才清理。
   词法等价路径需视为同一引用，外用户/越界路径不能扩大清理范围。
8. focused/full/race/vet、frontend 全量、production build、隔离生产形态真实 HTTP 与 fresh/historical
   mounted-volume/backup 门通过后，才可把状态更新为 implementation-complete 或发布 Docker。

## 9. 实施约束

实现应复用 `request_body.go#decodeBoundedSingleJSON`，在 book handler 映射既有平面错误；不得新增
全局 body middleware，也不得顺带改变 remote/import/batch/category/refresh/change-source 的 request
上限。DTO 和字段校验可放在窄的 book write helper，但数据库 transaction 仍由 handler/service 边界
持有。

本地目录引用检查必须发生在删除 commit 后、`RemoveAll` 前，并以数据库剩余行作为权威。不得用预先
计算的删除 ID 数量代替引用查询，不得通过 schema 迁移或启动时清扫修复历史行，也不得因为清理
best-effort 失败改变已有 `204`/batch `200` 成功响应。
