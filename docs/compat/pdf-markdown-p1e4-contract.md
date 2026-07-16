# P1-E4 PDF、Markdown 与 `.text` 历史本地书兼容合同

状态：**已完成实现、回归与 Docker 发布。** 实现仍以固定上游清单为准，不把 OpenReader
原有的额外 parser、API 或 UI 当作正确性依据。

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
| 直接导入可见入口 | `OverlayBookImport.vue` 仅声明 `.txt,.epub,.umd,.cbz` 与四格式文案；`useOverlayBookImport.js` 用 `isDirectImportableLocalPath` 在预览 API 之前拒绝被“所有文件”强行选入的历史扩展名。 | **aligned（E4-PDFMD-1）**：新用户入口已收敛。 |
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
| E4-PDFMD-1A | **已完成**：最小文本 PDF 通过历史 direct API 导入后删除 chapter cache，正文从 archive 恢复；`refresh-local` 重建目录且字节级保留原 archive。 | 保留既有 PDF 用户数据。 |
| E4-PDFMD-1B | **已完成**：无可读文本 PDF 与超过 parsed-text 预算的 PDF，在 preview/import 都返回 `400`；断言零 book、零 chapter、零持久 archive。 | 防止解析失败污染书架或卷。 |
| E4-PDFMD-1C | **已完成**：`.md`、`.text` 的历史导入删除 cache 后恢复、刷新后可读且不生成 Markdown HTML。 | 保留历史文本数据但不扩张产品语义。 |
| E4-PDFMD-1D | **已完成**：单元合同锁定四格式 chooser；真实 Chrome 在 1440×900 与 390×844 强制选入 PDF 时不发 preview 请求，TXT 仍正常预览；LocalStore/WebDAV 格式矩阵不变。 | 修正可见工作台行为。 |
| E4-PDFMD-1E | **已完成**：用户 B 不能消费用户 A 的 Markdown stage token，也不能读取用户 A 已导入的历史章节；解析限额仍在 archive/SQLite 写入前生效。 | 多用户与资源边界。 |

测试已先在错误 UI 上失败，再实施 UI 收敛。`go test ./...`、`npm test`、`npm run build` 与
隔离真实 Chrome 的直接导入验证均已通过；历史 PDF/Markdown/`.text` 阅读、刷新、cache 回建和
跨用户隔离由 API 合同测试覆盖。本切片可以构建 Docker；纯文档或纯测试更新不重新发布镜像。

## 5. 非目标

- 不把 PDF、Markdown、`.text` 加入上游工作台，也不为其补上游不存在的书源、书仓或 WebDAV 流程。
- 不把“当前 API 能工作”写成上游对齐。
- 不删除用户已有本地书，也不将历史格式转换成 TXT 后覆盖原始 archive。

## 6. 发布记录

2026-07-16 已完成本地镜像构建、卷/备份 smoke 与 GHCR 多架构发布：

- Git：`d0a0f5b fix: align visible local import formats`；
- tags：`ghcr.io/changshengyu/openreader:d0a0f5b`、`ghcr.io/changshengyu/openreader:latest`；
- remote multi-architecture index digest：`sha256:b55e119fbb272065f1c8b447d783a371d00c633f183f583f987d7471aab0914d`；
- linux/amd64：`sha256:cd19fa3cd0a7c7d75ea8803af67d2897892b30785bb1083513a008ebd1927979`；linux/arm64：`sha256:8831ac5442fa5d06b7940e5bfd7ca85a18cec4d66d2723d00eddf9054d9218bd`。

本合同的发布记录提交不改变镜像内容，不会重复发布 Docker。
