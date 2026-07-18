# P1-E4 EPUB fragment、跨资源与相对资源兼容合同

状态：**历史 OpenReader fragment 数据兼容与移动返回手势已实现；新目录生成合同已被 2026-07-18 固定基准复审纠正。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本文件是 E4-EPUB-2 的前置门禁。先记录行为、数据和安全合同，再写失败夹具，最后修改
parser、归档或 Reader；当前 OpenReader 实现不构成正确性依据。

> 复审纠正：本文件此前把 `EpubFile#getContent()` 的原生/搜索辅助分支误当成固定基准 Web
> Reader 的目录与可见正文合同。固定提交实际由 `TableOfContents#getAllUniqueResources()`
> 按 href 去重并丢弃 fragment，`BookController#getBookContent()` 也只向单 iframe 返回当前
> XHTML。新导入/刷新应以
> [`epub-fixed-baseline-catalog-reader-contract.md`](epub-fixed-baseline-catalog-reader-contract.md)
> 为准；本文件保留的 fragment slice 能力只服务已发布历史数据，不再证明新目录生成正确。

## 1. 历史实现所依据的上游辅助能力（不再作为新目录合同）

| 证据 | 提取的行为 |
|---|---|
| `src/main/java/io/legado/app/data/entities/BookChapter.kt` | 实体具备 `startFragmentId`、`endFragmentId` 等通用字段，但固定本地 EPUB 目录创建路径并未写入这些字段。OpenReader 已发布 fragment rows 因数据兼容仍须继续读取。 |
| `src/main/java/io/legado/app/model/localBook/EpubFile.kt#getContent` | 原生/搜索辅助路径具备 fragment 和跨 resource 处理代码；固定本地 EPUB 目录没有写入其依赖的 fragment/`nextUrl`，Web Reader 的 EPUB 分支也不会调用它。因此它只能解释历史 OpenReader slice 能力，不能定义新目录。 |
| `EpubFile.kt#getChapterList*` | TOC 实际按 href 去重并丢弃 fragment；含 TOC 的混合规则还会先把最终 TOC title 写回 spine 共用 resource。完整实际状态见新合同。 |
| `src/main/java/com/htmake/reader/api/controller/BookController.kt#getBookContent` | EPUB 解压后直接返回当前目录 row 的 XHTML URL并提前结束，不拼接下一 resource。 |
| `web/src/components/Content.vue#renderEpub` | EPUB 以 iframe 显示；同资源锚点、相对 CSS/图片/字体和跨 XHTML 链接均由浏览器相对 URL 解析，而不是被文本段落渲染器改写。 |

上游直接解压 archive；OpenReader 不复制这个不受限实现。受控 resource capability、严格 ZIP
限制、iframe CSP 和多用户隔离仍是允许的安全适配。历史 fragment rows 的可见 slice 行为必须
保留，但新目录必须按固定 href 去重合同生成。

## 2. 当前差距

| 合同层 | 当前 OpenReader 证据 | 判定 |
|---|---|---|
| TOC fragment | NAV/NCX 使用 `(path, fragment)` 去重；同一 XHTML 的不同 fragment 保留为独立目录项，并按下一个同资源 TOC 项生成终止边界。 | **错误重构；仅历史数据兼容** |
| 章节边界数据 | `models.Chapter`、`TXTChapter` 与 `ArchivedChapter` 均保存起止 fragment；已发布 SQLite/`chapters.json` 必须继续读取，但新导入/显式刷新不得再生成这些本地 EPUB fragment rows。 | **legacy-compatible / must-fix for new** |
| iframe 文档切片 | `epubreader.OpenResource` 仅在 capability 绑定的 XHTML 资源上应用已签名的 DOM 边界；同资源静态资源不切片，源 archive 不写回。 | **aligned（安全适配）** |
| 链接与相对资源 | capability 根目录保持稳定；Reader 先精确匹配 `(resourcePath, resourceFragment)`，再按 resource 回退。跨 XHTML 和同 XHTML 的已截出锚点应进入完整 Reader 跳章事务，但 bridge 尚未阻止跨 XHTML iframe 默认导航。 | **must-fix（EPUB Bug 1）**：详见 [`epub-mobile-back-bug1-contract.md`](epub-mobile-back-bug1-contract.md)。 |
| 同资源锚点 | 当前 slice 内的目标继续原地滚动；不在当前 slice 的目标发送受验证 `navigate` bridge 事件并重载目标章节资源。 | **aligned** |

## 3. 历史 fragment row 的兼容合同

1. EPUB archive 路径和 fragment 必须独立保存/校验：

   - `resourcePath` 始终是规范化的 archive POSIX 路径，不能带 `#`、query、绝对路径或
     host 路径；
   - `resourceFragment` 与 `resourceEndFragment` 为可空、长度受限的 decoded DOM id；二者
     只用于 XHTML document 的显示边界，永远不能参与文件系统路径拼接；
   - 已保存 TOC/NCX fragment rows 继续按 `(resourcePath, fragment)` 精确读取；这条规则只适用
     于升级前数据或尚未过期的升级前 staged snapshot，不得用于新解析结果。

2. 新导入和显式 `refresh-local` 的目录顺序、标题和 fragment 空值遵循固定基准新合同；应用启动、
   普通读章、备份或恢复不得后台合并既有 rows。首个 titlepage `封面` 和无 fragment EPUB 的
   已验证能力继续保留。

3. nullable fragment 字段与读取能力继续存在，不能做破坏性 schema 迁移。旧 row/旧 archive
   缺失字段时显示完整 XHTML；旧 row 带字段时继续 slice。只有用户显式刷新才以新 parser
   重建目录，且不得删除书签或改写原 archive。

4. `GET /api/books/:id/chapters/:index/content` 对 fragment 章节仍返回同一类 EPUB
   response。`resourceUrl` 必须引用同一个受限 archive resource，并携带明确、受限的
   document-slice 参数和起始 hash，使 iframe 打开时定位当前 fragment；后端只有在服务
   XHTML 时读取该 slice 参数，静态 CSS/图片/字体相对 URL 不带它也必须继续可读。

5. XHTML slice 必须与上游可见边界一致：起始 fragment 之前的同级正文不显示；当终止
   fragment 与起始 fragment 不同时，终止节点及其后的同级正文不显示。DOM id 缺失则
   维持完整文档并返回可读内容，而不是白屏、host-path 错误或删除 archive。

6. Reader 状态转换：

   - iframe 加载 `two.xhtml#part-b` 时，Reader 精确选择 `(resourcePath, fragment)` 对应
     的目录项，而不是同 resource 的第一个章节；
   - 当前 slice 内同资源 hash 继续原地滚动；目标 fragment 属于另一个目录 slice 时，切换
     到该目录项并重新加载；
   - 跨 XHTML 链接切换相应章节；工具层、设置/目录面板和阅读位置恢复的既有 P0 规则不变。

## 4. 安全与数据兼容边界

- fragment 在解析、持久化和 query 解码后都必须拒绝 NUL、过长值和无法 UTF-8 表示的值；
  不能使用未转义 fragment 拼 CSS selector，也不能把它写入日志。
- capability 仍绑定用户、book、archive fingerprint 与过期时间；slice 参数不扩大其可读
  archive 范围，也不能让资源读取跳出 extraction root。
- 继续只允许 XHTML/HTML、CSS、图片、字体的 EPUB resource 类型；不得因 fragment
  支持而放开 script、base、iframe、外链、表单或 CSP。
- SQLite 仅新增 nullable metadata；`data/`、`cache/`、`library/` 根、原 EPUB、旧
  `chapters.json` 和 backup/WebDAV 格式保持兼容。备份恢复缺失 fragment 字段时按空值处理。

## 5. 已发布历史能力与纠正后的新门禁

| 编号 | 自建 EPUB fixture 与断言 | 覆盖 |
|---|---|---|
| E4-EPUB-2A | **已发布但目录断言已作废**：旧夹具曾要求 TOC 保留三个 fragment 目录项。现在改由 EPUB-FIXED-1 断言新 parser 按 href 收敛为两个 resource；原夹具仅可用于验证历史 rows 继续可读。 | engine parser、refresh-local |
| E4-EPUB-2B | **历史兼容保留**：旧 SQLite/`chapters.json` fragment metadata 不被启动迁移删除，旧无 fragment row 也可读；新导入与显式刷新改由 EPUB-FIXED-2/3 断言不再写 fragment。 | importer、API、data migration |
| E4-EPUB-2C | 第一 slice 只含 part-a、不含 part-b；第二 slice 只含 part-b；缺失 id 维持可读 document；archive/cache 未写回。 | `/chapters/:index/content`、`/api/epub-resource`、安全 headers |
| E4-EPUB-2D | fixture 含相对 CSS、图片、字体、当前 slice hash、到另一个 fragment 的 hash 和跨 `two.xhtml#opening` 链接。三视口下验证正确目录索引、URL、iframe 内容、工具层不隐藏、无 console/page error。 | 真实 Go + Playwright：1440×900、390×844、360×800 |
| E4-EPUB-2E | 恶意 archive path、恶意/超长 fragment、篡改 capability/slice、过期 capability、其他用户和已替换 archive 均被拒绝或安全降级；不得泄露 archive/library 路径或 JWT。 | parser/service/API/security |

## 6. 历史实现与复审记录

- `backend/engine/parser_test.go` 与 `backend/api/api_test.go` 中要求新解析生成同 XHTML 多
  fragment 目录的断言属于错误测试，必须由 EPUB-FIXED-1/2 替换；旧 SQLite row、旧 staged
  snapshot 和受签名 iframe slice 的兼容测试继续保留。
- `backend/db/db_test.go` 使用缺失旧列的 SQLite fixture，证明 GORM 只新增
  `resource_path`、`resource_fragment`、`resource_end_fragment`，不改变已有 chapter 数据。
- `backend/services/epubreader/document_test.go` 与 `capability_test.go` 覆盖 document slice、
  缺失 id 的可读降级、bridge 导航、规范 path、NUL/超长 fragment 拒绝和 capability 绑定。
- `frontend/tests/readerEpubFrame.test.mjs` 覆盖相同 resource 的精确 fragment 目录映射及
  `navigate` bridge 事件。`scripts/smoke/reader-epub-contract.mjs` 在 1440×900、390×844、
  360×800 验证 slice、同 XHTML 跳转、跨 XHTML 跳转、相对 CSS/图片/字体、移动工具层和
  无 console/page error。

## 7. 发布记录

2026-07-15 已完成本地 Docker 构建、`docker-volume-backup-smoke.sh` 与 GHCR 发布：

- Git：`8f5e979 fix: align EPUB fragment reader navigation`；
- tags：`ghcr.io/changshengyu/openreader:8f5e979`、`ghcr.io/changshengyu/openreader:latest`；
- remote multi-architecture index digest：`sha256:1f17a4a028742515c065d00995df8e2f109a87386f9e5e221f4033851663de34`；
- architectures：linux/amd64 `sha256:aa8ad7e258bf7e393cac636ccdf1212b0e5fc98eabc4dfaedb3472195557ec72`，linux/arm64 `sha256:b9cb926970bd22bb82511b3a5d620a8ad92a25f3a95d1df3c7cc13e7b9627a59`。

该发布早于 Bug 1 和固定基准二次复审，不能作为 EPUB 新目录已对齐的证据。下一次 EPUB Docker
发布须同时包含 EPUB-FIXED 测试和既有返回手势浏览器回归；纯文档提交不发布 Docker。
