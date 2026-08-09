# 本地 TXT/Markdown parser 资源预算 P2 合同

审查日期：2026-08-09

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**fixed-baseline-audited / pre-implementation**

本合同只关闭本地纯文本分支仍未接入统一 parser 预算的问题，并校正旧审计中已经过时的
“ZIP/UMD/PDF/暂存清理尚未实现”结论。它不改变可见导入格式、目录识别语义、SQLite、书架数据、
备份格式、LocalStore/WebDAV 根目录或已缓存章节。

## 1. 固定上游合同

权威文件：

- `src/main/java/io/legado/app/model/localBook/TextFile.kt`
- `src/main/java/io/legado/app/model/localBook/LocalBook.kt`

上游行为：

1. 先用最多 512000 字节探测编码和默认目录规则，再按同一编码顺序扫描完整文件。
2. 显式目录规则按多行正则识别；匹配前正文生成“前言”，目录顺序与原文件顺序相同。
3. 没有目录规则时，以 512000 字节块、约 10KiB 一章并优先换行边界生成
   `第<块>章(<序号>)`。
4. 章节目录保存源文件 byte offset；读取正文时只读取该章节区间。上游没有服务端上传、
   解码后文本、章节数或总 CPU 的安全预算。

OpenReader 可以增加有界资源策略，这是 Go 多用户服务所需的安全适配；但预算以内的编码探测、
标题识别、前言、空目录和无目录 fallback 结果必须保持现有已对齐语义。

## 2. 当前实现与裁决

| 合同层 | 当前证据 | 判定 | 必须结果 |
|---|---|---|---|
| 原始输入 | direct、LocalStore、WebDAV 和确认重读均先受 `OPENREADER_MAX_IMPORT_BYTES`（默认 128MiB）限制。 | `aligned security adaptation` | 保留现有 413、原子 staging 和用户 token 隔离。 |
| 编码/目录 | `decodeTXTForCatalog` 只用 512000 字节探测；`ParseTXTWithRule` 再解码完整 staged bytes；默认规则和 10KiB fallback 已按上游恢复。 | `aligned visible behavior` | 预算内输出不能改变；显式无匹配规则仍返回合法的零章节 preview。 |
| 解码后文本 | TXT/`.text`/Markdown 在 `parseUploadedBookWithLimits` 中调用不带 limits 的 `ParseTXTWithRule`；UTF-16/32、GB18030 等解码结果没有核对 `MaxParsedTextBytes`。 | `must-fix` | 解码后、分章前检查 UTF-8 byte 总量，超限返回 `ErrLocalBookParseLimit`。不得产生 archive、DB row、广播或消费 retry token。 |
| 章节数量 | TXT 每个匹配标题都 append；prepared snapshot 又错误借用 `MaxUMDChapters` 作为所有格式上限，direct import 与 preview 的行为可能不同。 | `must-fix` | 增加格式无关的 `MaxParsedChapters`，所有新 parse/preview/confirm 在相同边界失败；UMD 自身仍保留独立结构上限。 |
| 正则工作量 | 自定义规则已有 16KiB 输入限制，Go RE2 无灾难性回溯；默认规则只对 512000 字节 probe 执行多次，显式规则对完整文本线性扫描一次。 | `bounded after byte/chapter fix` | 不另设任意墙钟超时；以输入、解码文本、规则长度和章节数共同构成确定工作预算。请求取消后不得继续持久化。 |
| 历史归档恢复 | `refreshLocalBook` 与 `rebuildLocalChapterText` 先 `os.ReadFile`，随后调用 legacy parser wrapper；文档声称“更宽但仍有界”，实际读取前没有执行 1GiB legacy ceiling。 | `must-fix implementation/documentation mismatch` | 用 bounded file reader 在分配完整字节前执行 legacy input ceiling；预算内旧 TXT/EPUB/PDF/UMD/CBZ 继续可刷新/重建，超限只使显式刷新失败或该次 lazy rebuild 返回空，不改旧行/文件。 |
| ZIP/UMD/PDF/preview cleanup | 当前已有 archive path/symlink/duplicate/count/per-entry/expanded-size，UMD segment/zlib/text，PDF page/text，以及启动+每小时跨用户 stage cleanup。 | `already aligned` | 不重复改写；保留现有专项测试。旧 checklist 未勾选项必须勘误。 |

## 3. 新的加法配置与兼容规则

新增：

```text
OPENREADER_MAX_PARSED_CHAPTERS=100000
```

- 未设置时默认 100000；为兼容已部署的自定义策略，如果只设置了
  `OPENREADER_MAX_UMD_CHAPTERS`，通用章节默认值随该既有值收敛。
- `OPENREADER_MAX_UMD_CHAPTERS` 继续约束 UMD 声明/offset/title 结构；通用 parsed chapter 上限
  约束最终 parser 输出。UMD 使用两者中更严格的结果。
- `OPENREADER_MAX_PARSED_TEXT_BYTES` 继续是所有格式最终解析文本的总预算；TXT/Markdown 只是补上
  原先漏接的分支。
- 配置只影响新 preview/import/confirm、显式本地刷新和丢失缓存后的 lazy rebuild。启动时不得扫描、
  重写或删除既有 `books`、`chapters`、`library/`、`cache/`、进度或书签。
- 旧归档恢复继续使用 `LegacyLocalBookParseLimits`：1GiB 输入、2GiB 解码文本、1000000 通用/
  UMD 章节。这是有界兼容路径，不把新上传默认值追溯应用到历史数据。

## 4. API、错误与数据合同

1. `/api/imports/books/preview` 的 parser-budget 失败仍为 `400`，保留同一用户可重试的
   `importToken`；错误只说明 parser 安全上限，不包含 staged path、原始正文或 token。
2. `/api/imports/books` 确认失败不得消费 stage；修正设置后可用同一 token 重试。LocalStore/
   WebDAV 批量 envelope 保留现有逐项错误形状。
3. 超过 raw upload 上限仍为现有 `413`，不能把 transport-size 和 parser-output 错误混在一起。
4. `refresh-local` 在任何 parser/read limit 失败前不修改章节、书籍、缓存或 archive metadata。
5. lazy cache rebuild 的超限/读取失败不删除现有 DB chapter，也不暴露宿主路径。
6. 不增加数据库表/列、迁移 marker、备份 JSON 字段或持久设置；旧 Docker volume 可直接启动。

## 5. 测试先行门禁

实现前必须先加入并观察稳定失败：

1. Engine：UTF-32/UTF-16 解码后超过 `MaxParsedTextBytes`；显式逐行标题和无目录 10KiB fallback
   超过 `MaxParsedChapters`；错误均为 `ErrLocalBookParseLimit`。
2. Engine：普通 UTF-8、UTF-16、GB18030、自定义无匹配规则、前言和 fallback 的既有 golden 输出
   不变。
3. Importer/API：direct、LocalStore、WebDAV 的 TXT/Markdown preview 使用同一 limits；失败时零
   archive/DB/broadcast，staged token 仍可重试；prepared snapshot 使用通用章节配置，不再借用 UMD。
4. Legacy data：小型历史 TXT/EPUB/UMD/CBZ/PDF 仍可 rebuild/refresh；超过 legacy input ceiling 的
   fixture 在完整读取前拒绝，并保留旧 book/chapter/cache。
5. Config：新环境值、无效值 fallback、只设置旧 UMD 值时的兼容 fallback。

完成 focused 和 race 后还需通过：`go test ./...`、frontend 全量测试和 build、真实 Go 的本地书导入
三视口 smoke，以及 fresh/historical Docker volume/portable backup 门。该切片没有前端结构变化，
无需凭 UI 测试替代 parser/API/旧卷证据。

