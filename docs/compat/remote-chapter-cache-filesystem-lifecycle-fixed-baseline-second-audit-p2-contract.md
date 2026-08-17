# 远程章节文本缓存文件系统生命周期第二轮固定基准合同

状态：**inventory-complete / tests-and-implementation-pending**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审远程书章节文本缓存在 `cache/` 下的读、写、统计、清理和引用收敛。
本地导入书的 `library/` 归档、浏览器缓存、章节图片/封面 capability 和整本缓存的可见
进度/取消状态机继续由已发布专项合同约束，不因文件系统反例重开。

## 1. 权威源码

### reader-dev

- `src/main/java/com/htmake/reader/api/YueduApi.kt:249-254`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:2581-2699,2701-2772`
- `web/src/components/BookManage.vue:388-492,566-595`

固定上游按当前用户 namespace 定位书架书：远程书可缓存到由书 URL 派生的私有目录，
cache-info 只投影该用户书架中实际 `.txt` 文件数，delete 先验证登录、书架归属和非本地书，
再递归删除该书缓存目录并刷新可见缓存信息。

### OpenReader

- `backend/api/cache.go`
- `backend/api/cache_stream.go`
- `backend/api/book_cleanup.go:162-234`
- `backend/api/books.go:2635-2707,2968-3009`
- `backend/engine/cache.go`
- `backend/services/chapterimage/storage.go:278-361`
- `backend/services/coverimage/storage.go:236-287`
- `docs/compat/data-migration.md#p1-d4-book-deletion-cache-and-refresh-lifecycle`
- `docs/compat/book-management-cache-p2-contract.md`

## 2. 差异矩阵

| 合同点 | 固定上游 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 作用域 | 按当前 namespace 的单书查询/删除 | 另有当前 user 级 stats/clear，BookManage/Reader 保留单书/批量 clear | **technical-stack-equivalent / 保持**；不得跨用户、不清本地书归档 |
| 持久表示 | 每书 namespace 目录中的 `.txt` 文件 | `chapters.cache_path` 指向 `cache/` 下派生文件，可被多行/多用户共享 | **允许的数据适配 / 保持**；删除前必须查全部引用 |
| rooted path | 上游由服务端派生目录 | 远程读/统计/清理使用 lexical prefix 后 `os.ReadFile/Stat/Remove`，写入用 `MkdirAll/WriteFile` | **must-fix**；root/ancestor/entry symlink 可把读、覆盖或删除扩展到 `cache/` 外 |
| 统计语义 | 统计实际存在的章节文件 | `cachedChapters` 先按非空 DB path 计数，missing/unsafe 行也被计入 | **must-fix**；只有已验证非空普通文件才是 cached chapter |
| 引用感知删除 | 单书私有目录删除 | OpenReader 清行后查所有远程章节引用，但查询失败时仍继续删文件 | **must-fix**；无法证明 unreferenced 时必须 fail closed |
| 写入/清理并发 | 上游当前用户目录内操作 | 文件写入→DB 发布与 DB 清行→文件剪枝无共享串行边界 | **must-fix**；新引用不能在 prune 查询后被删成悬空行 |

## 3. 稳定 API 与可见语义

| 动作 | 成功合同 | 失败/隔离合同 |
|---|---|---|
| `GET /api/cache/stats` | 保持 `{files,size,cachedChapters,scope:"current-user"}`；text file 按同一物理路径去重，chapter 按当前用户远程章节的已验证非空文件计数；当前用户的章节图/封面派生缓存继续纳入 files/size | missing、outside absolute、traversal、root/ancestor/entry symlink、目录/特殊文件和超界对象不计数；DB/私有图片树统计失败保持路径无关 500 |
| `DELETE /api/cache` | 保持 `{clearedFiles,clearedSize}`；一个 DB transaction 只清当前用户远程章节 path，commit 后剪枝已证明无引用的安全普通文件和该用户图片/封面派生树，计数只包含实际删除 bytes；持久成功后广播权威书架 | 未认证 401；列书/DB transaction 失败为安全 500 且不删文件；unsafe/missing/shared 对象保持原样。显式 clear 可清该用户的 unsafe/missing DB 引用，但不得触碰其指向对象 |
| BookManage/Reader 单书或批量 `clear-cache` | 继续只接受当前用户远程书 ID，保持现有返回计数、本地浏览器清理和完成后可见刷新 | 外用户/本地书不进入缓存文件清理；与 user-wide clear 使用同一 rooted/reference/concurrency 边界 |

OpenReader 的当前用户级 stats/clear 是已发布的多用户运维适配，不是上游新 URL 复制。保留其 JSON
shape 和侧边栏确认/反馈；不新增管理员全局 clear、不返回 server path。

## 4. Rooted 远程章节文件边界

- trusted root 固定为当前 `Config.CacheDir`。新写入仍使用
  `engine.ChapterCachePath(bookURL, chapterURL)` 的现有 MD5 相对名，不改名、不扫描或重写旧卷。
- persisted remote `cache_path` 可为相对路径，也可为当前 `cache/` 内的历史绝对路径。两者先投影为
  同一正规相对 identity；retired-host/outside absolute、NUL、traversal 和空 root 失败关闭。
- 每次读、stat 和 remove 都检查 root 到 entry 的现存组件；root/ancestor/entry symlink、目录、
  FIFO/device/socket 不可作为章节文件。不得使用 lexical prefix + `os.Stat/ReadFile/Remove`。
- 读取用 `Lstat -> open -> SameFile` 后的同一句柄，并以当前 `MaxSourceResponseBytes` 作为最大章节
  缓存读取预算；超限/不安全视为 cache miss，不把宿主 bytes 作为章节正文返回。
- 写入在已验证父目录中使用私有 temp + close + atomic rename；既有 symlink/非普通目标拒绝，失败不
  截断已有普通文件、不发布新 `cache_path`。请求取消在文件发布前停止 copy。
- 剪枝只删除已验证普通文件；unsafe/missing/非普通对象原样保留。错误响应和日志不得包含
  `CacheDir`、symlink target、OS error、章节正文、source credential 或 JWT。

## 5. 数据、引用与并发合同

- clear/delete/refresh/change-source 继续先提交 SQLite 行变更，再做可补偿派生文件清理；DB 失败
  不得删文件。文件清理失败不回滚已提交行，但计数不可虚报。
- prune 查询必须包含所有用户的远程章节非空 `cache_path`。任意查询/规范化失败都是“无法证明
  无引用”，本轮不删任何候选文件。
- 远程 cache 文件发布 + `chapters.cache_path` 提交与引用查询 + prune 在单进程共享串行边界中：
  清理在先则后续写入重建文件；写入在先则清理必须看到新 DB 引用。
- 读取可与 clear 并发：已打开句柄可完成本次读，后续请求按新 DB/文件状态处理。
- 远程缓存读取不转移 Book/Chapter 归属。user-wide clear 不清 `source_id=0` 行、`library/`
  正文、导入原件、浏览器 key、progress、bookmark、Book 或 source 行。

## 6. 数据迁移与回滚边界

- 不增加表/列/索引/迁移标记，不改 `data/cache/library` 目录、chapter-cache 哈希名、backup/
  portable/WebDAV 格式或环境变量。
- 升级不扫描、重写、删除或修复已有缓存。安全普通文件继续懒读；unsafe/missing 行保留到
  该书被重新缓存或用户显式 clear/delete。
- 回滚旧版本仍可读相同 SQLite 与普通缓存文件。新版本不清理 symlink/特殊对象，因此不会把挂载
  反例转换成不可回滚的文件系统变更。

## 7. Inventory 证据

1. `remoteCacheFilePath` 只用 `filepath.Abs` + 字符串前缀验证；随后 `os.Stat`、`os.ReadFile` 和
   `os.Remove` 会跟随 root/ancestor symlink。指向 `cache/` 外的 ancestor symlink 可泄露 size/body，
   并在 clear/refresh/change-source/delete 后删除外部目标文件。
2. `engine.WriteChapterCache` 用 `MkdirAll + WriteFile`；已有父 symlink 或 entry symlink 可使写入/截断超出
   `cache/`。新写入名本身是服务端哈希，但 mounted ancestor 仍是持久边界输入。
3. `remoteCacheStats` 先用非空 DB path 设置 `cachedChapters`，再尝试 stat；因此 missing/unsafe
   行仍显示已缓存，与上游“实际 `.txt` 文件”不同。
4. `pruneUnreferencedRemoteCachePaths` 只在引用查询成功时删除已引用候选；查询失败会带着完整
   候选集继续删除，与既有 fail-closed 数据合同相反。
5. 已有测试证明普通相对文件、当前/其它用户隔离、本地书保留、图片/封面计入与 DB
   行清理；没有 root/ancestor/entry symlink、special file、outside absolute、stale stats、引用查询
   失败或 write/prune 交错合同。

判定：**must-fix**。这是已挂载 `cache/` 卷上的读/写/删边界和共享引用完整性问题；只修
`GET /api/cache/stats` 或 `DELETE /api/cache` 都会留下同一 helper 的其它调用面。

## 8. 测试先行与发布门

实现前必须在旧实现上锁定失败：

1. relative/current-absolute 普通缓存可读、可统计、可清理；retired-host absolute、traversal、
   root/ancestor/entry symlink、目录、FIFO/特殊文件不读、不计、不删、不阻塞。
2. 章节读取不返回 outside sentinel；同句柄读在 path 替换后仍是原 bytes；超 `MaxSourceResponseBytes`
   不分配/返回无界内容，错误无 host path。
3. 安全哈希写入是原子普通文件；ancestor/entry symlink、特殊目标、取消和写入失败不改
   outside/existing bytes，不发布新 DB path，不留 temp。
4. stats 的 `cachedChapters` 只计实际安全非空文件；`files/size` 去重并保留当前用户图片/
   封面语义；其它用户与本地书不进入。
5. clear 只清当前用户远程 DB path；shared path 在任何用户引用存在时保留。引用查询失败、
   write-before-prune 与 prune-before-write 均不制造悬空行；广播只在 DB commit 后。
6. focused/race、Go 全量/vet、frontend 全量/build、真实 Reader/BookManage/侧边栏缓存流程、本地候选
   HTTP/挂载卷探针、fresh/historical/portable 卷门均通过后，才可本机发布 amd64/arm64。

## 9. Inventory 结论

公开 upload read 边界发布后，下一项 must-fix 确定为远程章节文本缓存的共享文件系统
生命周期。本提取阶段只新增/更新合同，没有修改应用或测试代码。
