# P1-E4-VOLUME-1 旧挂载卷恢复合同

状态：**第一批旧 SQLite/路径/缓存夹具和实现已完成；EPUB/UMD/CBZ/TXT 全格式旧卷及 Docker
夹具仍待继续。**

基准仍为 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。本合同把
reader-dev 的本地书格式行为与 OpenReader 已发布的 SQLite/挂载目录表示分开：前者决定
TXT、EPUB、UMD、CBZ 的解析和阅读结果；后者是 OpenReader 为 Docker、多用户和安全适配
必须持续读取的历史数据。当前组件和既有测试都不是本合同的正确性依据。

## 1. 历史卷的事实表示

一个已挂载的 OpenReader 卷由下列根组成，不能在升级、启动或刷新时整体替换：

```text
data/openreader.db                 SQLite（WAL 可同时存在 -wal/-shm）
cache/                             远端缓存、导入预览和早期本地派生缓存
library/data/<safe-user>/<book>/
  <original>.txt|epub|umd|cbz      原始本地书 archive
  chapters.json                    目录和可恢复的 chapter metadata
  bookSource.json                  本地书 metadata
  content/...                      可再生正文 cache（可能不存在）
```

SQLite 的 `books` 行保存 `LibraryPath`、`OriginalFile`、`TOCFile`、`SourceFile` 和
`TOCRule`；`chapters` 保存索引、`CachePath` 及新版本才有的 EPUB
`ResourcePath`/fragment 字段。`main.go` 在启动时先执行 GORM `AutoMigrate`，再执行
`MigrateLocalBookCache`。后者只允许把早期 `cache/` 下尚存在的本地派生正文迁到该书的
`library/.../content/`；不得改写 archive、目录、进度、书签或其他用户的数据。

当前逻辑备份 ZIP 位于 `data/webdav/`，只备份书架、进度、书签、设置、书源等逻辑 JSON。
它不含 `library/` 原始 archive，也不会在恢复时重建 `LibraryPath`、`OriginalFile` 或
chapter 行。因此“恢复一个旧 Docker 卷”必须保留三个挂载根；逻辑 ZIP 的合同只是**不能
损坏已挂载的 archive/旧书**，不是“ZIP 可以单独恢复本地书文件”。完整的本地书导出/备份
语义仍是 `reader-dev-openreader-gap-analysis.md` 中的独立 P2 项。

## 2. 路径、状态与安全矩阵

| 范围 | 历史/上游期望 | 当前映射 | 判定 | E4-VOLUME-1 要求 |
|---|---|---|---|---|
| SQLite 增量列 | 旧 EPUB chapter 没有 resource/fragment 列时仍可阅读；不丢失已存在的行。 | `db.AutoMigrate` 只添加字段；已有 `TestAutoMigrateAddsEPUBResourcePathWithoutLosingChapters` 覆盖单行。`TestHistoricalMountedVolumeMigratesRowsAndNeverReadsRetiredHostPaths` 新建、关闭并重开旧 SQLite 文件。 | 部分已证实 | 继续将完整 EPUB/UMD/CBZ/TXT archive fixture 接入同一旧库。 |
| 原始 archive | reader-dev 本地书以原始文件为正文重建来源。 | `library/<LibraryPath>/<OriginalFile>` 与 `localBookSourcePath`。 | 部分已证实 | 对 TXT、EPUB、UMD、CBZ 删除 `content/` 后均能读；refresh 只能新建派生 generation，archive 字节哈希不变。 |
| 历史相对字段 | 已有版本保存相对 `OriginalFile`/`CachePath`。 | `library` 和旧 `cache` 均有候选回退。 | 待旧卷验证 | 升级时从挂载路径重建，并将可移植 cache 字段规范到该书目录；不得删除有效 archive。 |
| 历史绝对字段 | 早期 OpenReader 可能保存开发机/Docker 绝对路径；换机器后只能利用相同 archive 名/旧 `LibraryPath` 重定位。 | `localBookArchiveRoot` 将 archive 绑定到 `library/data/<safe-user>/...`，绝对字段只可在该根按 suffix/base-name 重定位；`chapterCacheCandidates` 不再把绝对 cache 作为主机候选。 | **已完成第一批 must-fix** | 全格式旧卷和 Docker 中复验；不得读取或写入 `library/`、该用户根及允许的旧 `cache/` 迁移根之外的任意宿主路径。 |
| `LibraryPath`/metadata | 原始、`chapters.json`、`bookSource.json` 都属于书自己的目录。 | `localBookArchiveRoot` 复用私有目录边界并校验真实路径的符号链接解析；旧 archive 恢复、正文重建和 export 都使用它。 | **已完成第一批 must-fix** | 不可信旧字段不得跨用户、`..`、绝对路径或符号链接逃逸；刷新 metadata 的失败不能部分写入。 |
| 旧 cache 迁移 | 失去生成 cache 后应按 archive 惰性重建；存在旧 cache 时可以一次迁移。 | `MigrateLocalBookCache` 现只迁移 `cache/` 根内相对 regular file，并逐段验证目标 `library/.../content/` 目录未越界或经符号链接逃逸。 | **已完成第一批 must-fix** | 全格式旧卷中验证合法 cache 只迁移一次；非法/绝对/越界值不可触发读写或删除，且不会阻止其余健康书籍启动。 |
| EPUB metadata | 旧行/`chapters.json` 没有 resource fragments 仍有效。 | 首次打开可从 archive 恢复 fragment。 | 部分已证实 | 真实旧卷覆盖空 resource 字段、无 `content/` 与保留封面/章节的恢复。 |
| 用户隔离 | 书、chapter、progress、bookmark 是用户作用域。 | API `ensureBook` 按 user ID 查询；`TestHistoricalMountedVolumeRemainsPrivateAfterMigration` 验证用户 B 读取 A 的旧书为 404。 | **已完成 API 读路径验证** | Docker 旧卷中继续验证 refresh 与文件路径不能跨用户。 |
| 逻辑备份/恢复 | 不应破坏安装卷。 | trigger/list 在 `data/webdav/`，restore 仅恢复 JSON 书架语义。 | 技术栈差异，需显式记录 | 旧卷存在时 trigger、list、一次无本地书 archive 的 restore 和重启不得改动 archive 哈希、chapter 行或本地书可读性；不得声称 ZIP 独立恢复 archive。 |

允许差异：OpenReader 可以采用 capability、私有路径、惰性重建和有界 legacy parser
limits，代替上游未受限的文件访问/解压。这些差异只能收紧攻击面，不能使可读的已挂载书籍
消失。

## 3. 先于实现的失败夹具

夹具必须由测试临时创建，不提交受版权保护书籍。它必须是一个真正的旧 SQLite 文件：先用
旧列创建并插入数据，再关闭数据库；生产 `main`/同等启动路径打开后才允许 AutoMigrate。
不要通过当前 `models.Book` 直接制造一个“已迁移”的 fixture。

| 编号 | 夹具 | 失败断言与完成条件 |
|---|---|---|
| VOLUME-DB-1 | 用户 A、local `books`/`chapters`、progress、bookmark，且 chapter 缺少 EPUB 新列。 | 启动后原 ID/标题/进度/书签和 archive 均保留；新增列存在但旧值不被伪造；列表和本地章节 API 可读。 |
| VOLUME-FORMATS-2 | 相对路径 TXT、历史绝对路径 EPUB、UMD、CBZ；每书有 `chapters.json`、`bookSource.json`，删除所有 `content/`。 | 章节 API 能从 archive 回建；refresh 只替换章节/派生 generation 和合法 metadata，四个 archive 哈希不变。EPUB 保留封面与惰性 resource metadata。 |
| VOLUME-CACHE-3 | 一条早期 `cache/<relative>` 正文和一条非法 `../`/绝对 `CachePath`。 | 合法正文至多迁移一次且可读；非法值不读、不写、不删宿主路径，启动继续服务健康本地书。 |
| VOLUME-PATH-4 | 历史绝对 `OriginalFile` 指向不存在的旧机器路径，同时在 A 的 archive 根保留同名文件；另设一个真实的主机诱饵文件、`../`、绝对 `LibraryPath` 和跨用户 B 路径。 | 只使用 A archive 根中的受控回退；诱饵内容永不返回/不参与 refresh；所有恶意字段失败关闭且不泄露绝对路径。 |
| VOLUME-OWNER-5 | A、B 的独立书和旧字段。 | B 无法 list/read/refresh A，A 的恢复不改变 B 的 rows/files。 |
| VOLUME-BACKUP-6 | 完整旧卷先启动并验证，然后 trigger/list 逻辑备份、执行一个安全的 restore、重启容器。 | archive/metadata 哈希、chapter/progress/bookmark、可读章节保持；报告明确 ZIP 本身未携带 local archive。 |
| VOLUME-DOCKER-7 | 将同一 fixture 写进三个将要挂载给 release image 的目录。 | 实际 Docker 启动、登录/读取、refresh、backup、停止/再启动均通过；覆盖 amd64/arm64 发布前本地 image 的实际运行。 |

每个失败路径都要断言：没有 archive 改写、没有跨用户读取、没有宿主路径泄漏，也没有部分
chapter catalogue 提交。任何 test fixture 记录格式、大小、SHA-256 及自建来源说明。

## 4. 实施顺序与发布闸门

1. 提交本合同和父矩阵更新（本提交只含文档，不发布 Docker）。
2. 添加 VOLUME-DB-1 至 VOLUME-PATH-4 的失败测试；先让现有路径越界行为显式失败。
3. 以共享、私有、根约束的 archive/cache resolver 取代散落的绝对路径候选；保留可验证的
   相对和 historical absolute 回退。
4. 加入 VOLUME-OWNER-5、VOLUME-BACKUP-6 与 `docker-volume-backup-smoke.sh` 的真实卷
   fixture，随后跑完整 Go、前端 build/测试和 Docker 本地 build。
5. 只有在 Docker volume fixture、读取/刷新、逻辑备份不破坏性与重启都通过后，才发布
   下一张 GHCR 镜像；发布报告列出 archive 可恢复格式、逻辑备份的边界、允许安全收紧和
   未完成项。

### 当前实现证据（未发布 Docker）

- `backend/api/old_volume_contract_test.go` 先创建并关闭缺少 EPUB resource/fragment/variable
  列的 SQLite 文件，随后按启动顺序重开、AutoMigrate、执行 cache migration。测试保留
  progress/bookmark，删除本地 derived cache 后从 archive 读取正文，且旧宿主诱饵文件绝不
  成为 source/cache 候选；另一测试验证用户 B 得到 404。
- `backend/db/TestMigrateLocalBookCacheSkipsUnsafeHistoricalCachePath` 先证明原实现会删除
  `cache/../...` 外的文件，再锁定新行为：越界文件不复制、不删除、不改写 SQLite 值。
- `localBookArchiveRoot`、`existingRegularPathInside` 和 db 的受控目录迁移保持无 archive
  的相对 cache 以及音频库的相对 cache 读取，以免破坏真实早期记录；这一兼容分支从不接受
  绝对 cache 路径。
- 本批已通过 `go test ./...`、`npm test`（386）和 `npm run build`。尚未运行/发布 Docker，
  原因是 VOLUME-FORMATS-2 至 VOLUME-DOCKER-7 尚未完成，不能把现有通用 smoke 冒充旧卷
  验证。

## 5. 非目标

- 不把 PDF/Markdown/`.text` 重新显示为新的工作台导入格式；它们只保留既有 archive/API
  兼容合同。
- 不把逻辑备份 ZIP 扩展成无约束的 library 打包，也不以删除旧 archive 换取 schema 简化。
- 不在本合同审查阶段修改应用代码；所有路径收紧和 cache 迁移变更必须先由上述失败测试
  驱动。
