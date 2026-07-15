# P1-E4 EPUB fragment、跨资源与相对资源兼容合同

状态：**已完成上游提取；尚未进入实现。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本文件是 E4-EPUB-2 的前置门禁。先记录行为、数据和安全合同，再写失败夹具，最后修改
parser、归档或 Reader；当前 OpenReader 实现不构成正确性依据。

## 1. 上游权威行为

| 证据 | 提取的行为 |
|---|---|
| `src/main/java/io/legado/app/data/entities/BookChapter.kt` | 章节同时保存 `url`、`startFragmentId`、`endFragmentId` 和可序列化变量；fragment 是章节边界数据，不能当作可丢弃的 URL 装饰。 |
| `src/main/java/io/legado/app/model/localBook/EpubFile.kt#getContent` | EPUB 当前章节以 `chapter.url.substringBeforeLast("#")` 定位起始 resource；以 `startFragmentId` 删除该节点之前的兄弟内容，以 `endFragmentId` 删除该节点及之后的兄弟内容。若章节跨多个 resource，按 EPUB 内容顺序读取，直到下一章节 `nextUrl` 的 resource 边界为止。该分支专门修复了多 HTML 资源章节的内容丢失问题。 |
| `EpubFile.kt#getChapterList*` | `toc`、`spin` 及四种混合规则仍决定目录顺序和标题优先级；fragment 不能改变这些规则的默认值或把 titlepage 封面丢出目录。 |
| `src/main/java/com/htmake/reader/api/controller/BookController.kt#getBookContent` | 取当前目录项和相邻目录项的精确 URL；EPUB 解压后返回当前 resource 的实际可访问地址，供 Reader iframe 加载。 |
| `web/src/components/Content.vue#renderEpub` | EPUB 以 iframe 显示；同资源锚点、相对 CSS/图片/字体和跨 XHTML 链接均由浏览器相对 URL 解析，而不是被文本段落渲染器改写。 |

上游直接解压 archive；OpenReader 不复制这个不受限实现。受控 resource capability、严格 ZIP
限制、iframe CSP 和多用户隔离仍是允许的安全适配，但不能丢失上述目录/fragment 可见行为。

## 2. 当前差距

| 合同层 | 当前 OpenReader 证据 | 判定 |
|---|---|---|
| TOC fragment | `backend/engine/epub_parser.go` 的 `resolveEPUBPath`/`canonicalEPUBPath` 去除 `#` 与 query；`epubTOCEntry` 只存路径，`buildEPUBChapters` 用路径 map/去重。不同 `one.xhtml#part-a`、`one.xhtml#part-b` 因而合并为一个目录项。 | **must-fix** |
| 章节边界数据 | `models.Chapter` 与 `engine.ArchivedChapter` 只有 `ResourcePath`，本地导入把 URL 固定成 `local://…/chapter_N`；不存在起止 fragment 或相邻 resource 边界。 | **must-fix** |
| iframe 文档切片 | `epubreader.OpenResource` 对同一 XHTML 总是送出完整、已消毒的 document；无法根据当前目录项裁剪 fragment，重复章节会显示相同全文。 | **must-fix** |
| 链接与相对资源 | capability 根目录保持稳定，`ReaderEpubContent`/bridge 已能让浏览器加载相对 CSS、图片、字体和跨资源 iframe URL；`Reader.handleEpubLoad` 能按 resource path 更新章节。 | **partial**：对于唯一 resource 已等价；有多个 fragment 的 resource 无法选择正确目录项，必须随本项修复。 |
| 同资源锚点 | bridge 对存在于当前 iframe document 的 hash 发送几何位置；父 Reader 据此滚动。当前资源未切片时可用，但切片后链接到另一个目录 fragment 不能仅滚动到一个已被截出的节点。 | **must-fix** |

## 3. 目标数据、API 与状态合同

1. EPUB archive 路径和 fragment 必须独立保存/校验：

   - `resourcePath` 始终是规范化的 archive POSIX 路径，不能带 `#`、query、绝对路径或
     host 路径；
   - `resourceFragment` 与 `resourceEndFragment` 为可空、长度受限的 decoded DOM id；二者
     只用于 XHTML document 的显示边界，永远不能参与文件系统路径拼接；
   - TOC/NCX 中同一资源的不同 fragment 是不同目录项；同一 `(resourcePath, fragment)`
     的重复链接只保留目录中的第一个，保持上游的稳定目录语义。

2. `toc`/`toc+spin`/`toc<spin` 在 TOC 为主的分支保留 fragment 顺序；`spin`/`spin+toc`/
   `spin<toc` 的 spine 目录仍是一 resource 一项。标题回退规则、首个 titlepage `封面`
   和无 fragment EPUB 的现有输出不得改变。

3. 导入、刷新和懒恢复必须把 fragment 元数据同时写入 SQLite 与 `chapters.json`；这是仅加
   nullable 字段的迁移。旧 row/旧 archive 缺失字段时继续显示完整 XHTML，且不做破坏性
   重建、删除或重新上传。

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

## 5. 必须先失败的夹具与验证

| 编号 | 自建 EPUB fixture 与断言 | 覆盖 |
|---|---|---|
| E4-EPUB-2A | NAV 与 NCX 都含 `Text/one.xhtml#part-a`、`Text/one.xhtml#part-b`、`Text/two.xhtml#opening`；TOC 规则必须保留三个目录项、路径/fragment 顺序和标题，spin 规则保持两个 resource 目录项。 | engine parser、refresh-local |
| E4-EPUB-2B | 导入后 SQLite 与 `chapters.json` 有 fragment metadata；旧 row 删除 metadata 后懒恢复可再次导出正确 metadata，旧无 fragment row 仍可读。 | importer、API、data migration |
| E4-EPUB-2C | 第一 slice 只含 part-a、不含 part-b；第二 slice 只含 part-b；缺失 id 维持可读 document；archive/cache 未写回。 | `/chapters/:index/content`、`/api/epub-resource`、安全 headers |
| E4-EPUB-2D | fixture 含相对 CSS、图片、字体、当前 slice hash、到另一个 fragment 的 hash 和跨 `two.xhtml#opening` 链接。三视口下验证正确目录索引、URL、iframe 内容、工具层不隐藏、无 console/page error。 | 真实 Go + Playwright：1440×900、390×844、360×800 |
| E4-EPUB-2E | 恶意 archive path、恶意/超长 fragment、篡改 capability/slice、过期 capability、其他用户和已替换 archive 均被拒绝或安全降级；不得泄露 archive/library 路径或 JWT。 | parser/service/API/security |

在 E4-EPUB-2A 至 E4-EPUB-2E 未通过前，不能把 EPUB fragment/跨资源列为已对齐，也不能以
现有 image-only cover smoke 取代该门禁。
