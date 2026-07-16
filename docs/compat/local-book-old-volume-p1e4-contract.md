# P1-E4-VOLUME-1 旧挂载卷恢复合同

状态：**旧 SQLite、相对/绝对路径、相对 cache 迁移及 EPUB/UMD/CBZ/TXT archive 的 API 与
Docker 旧卷回归均已完成；跨用户 Docker 与可移植 archive 备份仍是后续范围。**

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
| SQLite 增量列 | 旧 EPUB chapter 没有 resource/fragment 列时仍可阅读；不丢失已存在的行。 | `db.AutoMigrate` 只添加字段；已有 `TestAutoMigrateAddsEPUBResourcePathWithoutLosingChapters` 覆盖单行。`TestHistoricalMountedVolumeMigratesRowsAndNeverReadsRetiredHostPaths` 新建、关闭并重开旧 SQLite 文件；Docker fixture 在同一旧库中载入 archive 与相对 cache。 | **已证实（API + Docker）** | 新增格式时仍必须复用真实旧 SQLite，而非已迁移的 model fixture。 |
| 原始 archive | reader-dev 本地书以原始文件为正文重建来源。 | `library/<LibraryPath>/<OriginalFile>` 与 `localBookSourcePath`。`TestHistoricalMountedVolumeRebuildsEPUBUMDAndCBZArchives` 覆盖 stale absolute source + 无派生 content 的三种 archive；Docker smoke 同时验证 TXT、EPUB、UMD、CBZ。 | **已证实（API + Docker）** | 对四种 archive 删除 `content/` 后均可读；refresh 只能新建派生 generation，archive 字节哈希不变。 |
| 历史相对字段 | 已有版本保存相对 `OriginalFile`/`CachePath`。 | `library` 和旧 `cache` 均有候选回退；迁移后将 cache 规范为 archive-root 相对 `content/...`。 | **已证实（API + Docker）** | 升级时从挂载路径重建，并将可移植 cache 字段规范到该书目录；不得删除有效 archive。 |
| 历史绝对字段 | 早期 OpenReader 可能保存开发机/Docker 绝对路径；换机器后只能利用相同 archive 名/旧 `LibraryPath` 重定位。 | `localBookArchiveRoot` 将 archive 绑定到 `library/data/<safe-user>/...`，绝对字段只可在该根按 suffix/base-name 重定位；`chapterCacheCandidates` 不再把绝对 cache 作为主机候选。 | **已完成（全格式 API + Docker）** | 不得读取或写入 `library/`、该用户根及允许的旧 `cache/` 迁移根之外的任意宿主路径。 |
| `LibraryPath`/metadata | 原始、`chapters.json`、`bookSource.json` 都属于书自己的目录。 | `localBookArchiveRoot` 复用私有目录边界并校验真实路径的符号链接解析；旧 archive 恢复、正文重建和 export 都使用它。 | **已完成第一批 must-fix** | 不可信旧字段不得跨用户、`..`、绝对路径或符号链接逃逸；刷新 metadata 的失败不能部分写入。 |
| 旧 cache 迁移 | 失去生成 cache 后应按 archive 惰性重建；存在旧 cache 时可以一次迁移。 | `MigrateLocalBookCache` 只迁移 `cache/` 根内相对 regular file，逐段验证目标 `library/.../content/` 目录未越界或经符号链接逃逸，并把 SQLite 规范为相对 `content/...`。顺序为 copy → SQLite save → best-effort delete source。 | **已证实（API + Docker）** | 合法 cache 至多迁移一次；非法/绝对/越界值不可触发读写或删除，且不会阻止其余健康书籍启动。 |
| EPUB metadata | 旧行/`chapters.json` 没有 resource fragments 仍有效。 | 首次打开可从 archive 恢复 fragment。 | **已证实（API + Docker）** | 旧卷继续覆盖空 resource 字段、无 `content/` 与保留封面/章节的恢复。 |
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

### VOLUME-CACHE-3 实现与证据

reader-dev 没有 OpenReader 的 SQLite、`cache/` 或 Docker 挂载迁移；它的本地书正文合同只要求
archive 可作为恢复来源。因此下列规则是 OpenReader 的**技术栈兼容/安全适配**，不能以当前
绝对路径实现或既有测试作为依据：

- 输入只接受 `SourceID=0`、有效私有 `LibraryPath` 且相对于当前 `cache/` 根的 regular file；
  `..`、绝对路径、NUL、符号链接逃逸或其他用户/书根均不参与迁移，也不改写原 SQLite 字段。
- 合法旧值 `legacy-cache/chapter.txt` 必须复制到私有 archive 根的
  `content/legacy-cache/chapter.txt`，并把 `chapters.cache_path` 规范为可移植的相对值
  `content/legacy-cache/chapter.txt`，而不是 Docker/开发机绝对路径。
- 目标字节必须等于旧 cache；已存在的目标不得被无条件覆盖。数据库更新成功前不得删除旧
  cache。更新成功后的删除失败只能留下可回收副本，不能使已更新的 chapter 不可读。
- 第二次启动不得再次迁移或改变 archive；`chapterCacheCandidates` 必须优先解析规范后的
  `archiveRoot/content/...`，而非退回宿主 cache 路径。
- 真实旧 SQLite API 和 Docker fixture 都必须至少包含一个上述合法相对 cache，以及一个
  绝对/越界诱饵；读取应返回合法 legacy cache 正文，重启后仍可读，且原始 local archive
  SHA-256 不变。

### VOLUME-OWNER-5 后续实现合同

用户作用域是 OpenReader 对 reader-dev 单用户文件模型的必要运行时适配。旧 SQLite/Docker
夹具必须在**升级前**同时写入用户 A 和 B、各自私有 `library/data/<user>/...` archive 与
chapter 行；不能只在升级后通过注册空用户代替 B。完成条件：

- A、B 登录后各自 `GET /api/books` 只能看到自己的旧书；不得因旧 `LibraryPath`、cache 或
  archive basename 相同而交叉列出。
- B 读取或 `POST /refresh-local` A 的任一 TXT/EPUB/UMD/CBZ/relative-cache book 必须为 404；
  A 对 B 同样如此。拒绝不得泄露 archive/cache 的宿主绝对路径。
- A、B 均能读取自己的 old-volume chapter。A 的 refresh、backup trigger/list、restore 与
  container restart 不得改变 B 的 book/chapter rows、cache 或原 archive SHA-256；反向同理。
- Docker smoke 要用真实 HTTP/JWT 断言上述行为。API contract 仍保留相同旧 SQLite 的快速
  回归，防止容器脚本成为唯一证据。

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
- `TestMigrateLocalBookCacheMovesLocalContentToLibrary` 和
  `TestHistoricalMountedVolumeMigratesRelativeCacheOnce` 以真实旧 SQLite 断言
  `legacy-cache/chapter.txt` 复制为 `content/legacy-cache/chapter.txt`，SQLite 只保存该
  相对值，第二次迁移不再变更，读取仍优先返回 legacy cache 正文且 archive 不变。
- `backend/cmd/create-old-volume-fixture` 生成可由容器启动的旧 SQLite + 相对路径 TXT、以及
  stale absolute `OriginalFile` 的 EPUB、标准 reader-dev UMD、CBZ archive，外加 archive
  正文不同的相对 cache 书；`HISTORICAL_VOLUME=1 scripts/docker-volume-backup-smoke.sh` 断言
  旧 cache 已删、私有 content 字节相同、SQLite 保存相对字段，并在读取、刷新、逻辑备份/restore
  和 restart 后持续成立。普通新卷 smoke 也在同一镜像通过。这完成 VOLUME-DB/PATH、
  VOLUME-FORMATS-2 与 VOLUME-CACHE-3，不替代跨用户 Docker 夹具。
- `TestHistoricalMountedVolumeRebuildsEPUBUMDAndCBZArchives` 将最小 EPUB、标准 reader-dev
  UMD 与 CBZ archive 写进同一类 user-private 旧卷目录，保留 stale absolute `OriginalFile`
  和缺失 content。三者分别经 EPUB resource、文本恢复、CBZ resource 路径读取，并在
  `refresh-local` 后验证原 archive SHA-256 不变。Docker fixture 复用等价 archive，在容器
  内完成同一读取、刷新、备份恢复与重启链路。
- 本批通过 `go test ./...`、`npm test`（386）、`npm run build`、`PUSH=0` 本地镜像构建，以及
  `c7d5abb` 的普通与 `HISTORICAL_VOLUME=1` 全格式/相对 cache Docker smoke。已从本机发布
  `ghcr.io/changshengyu/openreader:c7d5abb` 与 `:latest`；两者同为 OCI index
  `sha256:d7000822b4a135c3ee9ab12c4cbef5c5343cfc87c125cc3e5f05f52098d46fa7`
  （amd64 `sha256:cda62a0be6b051d28277eaf4659beca91169b368363c318e768dc48c6c268168`；arm64
  `sha256:72361558537e01c9ed32ca1bbd46aca9758df8ae02b00ddd8942bc48b28df838`）。

## 5. 非目标

- 不把 PDF/Markdown/`.text` 重新显示为新的工作台导入格式；它们只保留既有 archive/API
  兼容合同。
- 不把逻辑备份 ZIP 扩展成无约束的 library 打包，也不以删除旧 archive 换取 schema 简化。
- 不在本合同审查阶段修改应用代码；所有路径收紧和 cache 迁移变更必须先由上述失败测试
  驱动。
