# P1-E4 PDF、Markdown 与 `.text` 历史本地书兼容合同

状态：**上游清单已提取；尚未进入实现。** 本文只记录合同和待写失败测试，不把当前
OpenReader 的额外 parser、API 或 UI 当作正确性依据。

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

## 1. 上游事实

| 入口 | 固定基准行为 | 结论 |
|---|---|---|
| 直接导入预览/确认 | [`BookController#importBookPreview`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/com/htmake/reader/api/controller/BookController.kt) 只接受 `txt`、`epub`、`umd`、`cbz`；其他扩展名返回“不支持导入…格式的书籍文件”。 | PDF、Markdown、`.text` 不是上游新增书籍格式。 |
| 本地书仓 | [`LocalStore.vue`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/web/src/views/LocalStore.vue) 只给 `txt`、`epub`、`umd`、`cbz` 提供“加入书架”。 | 不得把额外格式重新放回书仓工作台。 |
| WebDAV | [`WebDAV.vue`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/web/src/views/WebDAV.vue) 只给 `txt`、`epub`、`umd` 提供“加入书架”。 | WebDAV 不能借由兼容逻辑暴露 CBZ/PDF/Markdown/`.text`。 |
| 本地书读取内部 | [`LocalBook.kt`](https://raw.githubusercontent.com/changshengyu/reader-dev/fa22f271849d45f93349ae1636223e27b16a4691/src/main/java/io/legado/app/model/localBook/LocalBook.kt) 对已构造的未知文本类 book 会走 `TextFile` 默认分支，但 Controller 不会由上传入口创建这类 book。 | 不能据此把 `.md` 或 `.text` 当成上游公开导入能力。 |

## 2. 当前映射与差距

| 合同层 | 当前证据 | 判定 |
|---|---|---|
| 直接导入可见入口 | `OverlayBookImport.vue` 的文件选择器为 `.txt,.text,.md,.epub,.pdf,.umd,.cbz`，文案还公开 PDF；`useOverlayBookImport.js` 将 `.text/.md` 视作 TXT 目录规则文件。 | **must-fix**：新用户入口没有对齐上游。 |
| 直接导入 API | `backend/api/imports.go`、`services/localbook/importer.go` 接受 `.text/.md/.pdf`；PDF 走受限提取，Markdown/`.text` 走普通 TXT parser。 | **明确的遗留数据兼容差异**：不作为 UI 能力宣传，但暂保留路由，以免已部署客户端、已暂存导入流和历史自动化调用失效。 |
| 既有书架阅读、刷新、缓存恢复 | `backend/api/books.go` 的 `parseLocalBookChapters` 和 `isSupportedLocalBookFile` 仍能读取 `.text/.md/.pdf` 原始 archive。 | **必须保留**：不能为了 UI 对齐而让已导入书籍白屏、无法刷新或无法从缺失 cache 恢复。 |
| 书仓 / WebDAV 工作台 | `storageImportable.js` 分别限制为 `txt/epub/umd/cbz` 与 `txt/epub/umd`，已有回归测试覆盖。 | **aligned（P1-E3）**：本切片不得回退。 |
| PDF 安全限制 | `ParsePDFWithLimits` 有页数与提取文本预算，无法提取文字的扫描 PDF 返回错误；导入器在 archive/SQLite 写入前解析。 | **acceptable-change（安全）**：上游没有这项格式，不能以兼容为由放宽资源限制。 |

## 3. 最终行为合同

1. 面向新用户的“导入本地书籍”弹窗只显示并接受 `TXT / EPUB / UMD / CBZ`。文件选择器
   以外强行选入的 `.pdf/.md/.text` 也必须在发起预览前得到明确、不创建书架的拒绝提示。
2. LocalStore 与 WebDAV 继续保持各自的上游格式闸门；本合同不改变 P1-E3 的批量预检、暂存
   token 或上传能力。
3. 已存在的 `SourceID=0` PDF、Markdown、`.text` 书籍是用户数据：章节读取、`refresh-local`、
   缺失派生 cache 时的 archive 回建、备份恢复和旧 URL 都必须继续可用。不得做删除、批量迁移
   或扩展名改写。
4. 为兼容旧 OpenReader 客户端，原有直接上传 route 在本阶段保留对这些格式的处理；它是**未在
   工作台公开的历史兼容通道**，而非与上游相同的产品功能。任何后续弃用必须先提供可验证的
   导出/迁移方案，不能随 UI 收敛一起破坏。
5. `.md` 与 `.text` 的历史恢复按现有纯文本/TXT 语义处理，不新增 Markdown 富文本渲染；PDF
   只接受可提取的文本 PDF。扫描件/加密件/无法读取文本的 PDF 必须正常报错，且不得创建
   book、chapter 或 `library/` archive。预览失败可保留用户范围的短期 stage token 供重试，
   但不能留下书架或持久 archive 写入。
6. 不修改 SQLite schema、`data/`、`cache/`、`library/` 根、备份格式和已有 original file。
   解析限额、路径约束和用户隔离继续适用于历史恢复路径。

## 4. 先失败的测试

| 编号 | 夹具与断言 | 目的 |
|---|---|---|
| E4-PDFMD-1A | 真实最小文本 PDF：历史 direct API 导入后删除 chapter cache，正文仍可由 archive 恢复；`refresh-local` 重建目录不改原 archive。 | 保留既有 PDF 用户数据。 |
| E4-PDFMD-1B | 扫描/无可读文本 PDF 与超过页数/文本预算的 PDF：preview/import 都返回受控错误；断言零 book、零 chapter、零持久 archive。 | 防止解析失败污染书架或卷。 |
| E4-PDFMD-1C | `.md`、`.text` 的历史导入/旧 SQLite fixture：按 TXT 正文读取、删 cache 后恢复、刷新不白屏；不产生 Markdown HTML。 | 保留历史文本数据但不扩张产品语义。 |
| E4-PDFMD-1D | 浏览器与单元合同：直接导入 chooser/文案只包含四种上游格式；通过“所有文件”绕过 chooser 选择额外扩展名时不调用 preview API；LocalStore/WebDAV 的既有格式矩阵不变。 | 修正可见工作台行为。 |
| E4-PDFMD-1E | 用户 A 的历史 archive、stage token 与 chapter route 不可由用户 B 读取；非法路径和超额输入在解析/归档前被拒绝。 | 多用户与资源边界。 |

测试必须先落地并在当前错误 UI 上失败，再实施 UI 收敛。实现后的门禁是 `go test ./...`、
`npm test`、`npm run build`，以及真实浏览器的直接导入和历史书阅读 smoke。此项完成后再判断是否
构建 Docker；仅文档或纯测试提交不重新发布镜像。

## 5. 非目标

- 不把 PDF、Markdown、`.text` 加入上游工作台，也不为其补上游不存在的书源、书仓或 WebDAV 流程。
- 不把“当前 API 能工作”写成上游对齐。
- 不删除用户已有本地书，也不将历史格式转换成 TXT 后覆盖原始 archive。
