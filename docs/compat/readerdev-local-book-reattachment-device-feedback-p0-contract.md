# reader-dev 本地书回填与章节引用真机反馈合同（P0）

状态：**implemented / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
当前基线：`OpenReader@919b588`。

## 已确认的合同缺口

- reader-dev `BookController.kt#saveBook` 以同一 namespace 内的 `name + author` 查找已有书。重新导入
  本地原文件时替换该书的信息，但明确保留 `durChapterIndex`、`durChapterTitle` 和阅读时间。
- reader-dev 逻辑备份只携带本地书的书架 metadata/进度，不携带原始 TXT/EPUB/UMD/CBZ；迁移时需
  另行补回原文件。这一限制不等于补回原文件后可以新建一个与旧进度无关的 Book。
- 当前 OpenReader 逻辑恢复会建立 `origin=loc_book`、`SourceID=0` 的 shelf row，却没有 archive 和
  Chapter rows；随后 direct/LocalStore/WebDAV 普通导入始终分配新的 `local://book_*`。旧 metadata、
  进度和书签因此与新目录分裂。
- `bookcatalog.reconcileChapterReferences` 对 TXT/UMD 等只按旧 index 回落。显式刷新或恢复后目录插入、
  删除章节时，旧 index 会指向另一个标题；不存在的 index 又会保留为悬空位置。

## 目录解析边界

固定上游 TXT 自动目录继续以首 512000 **源字节**探测编码和启用规则，按倒序规则统计且同票由后处理
规则覆盖；正文按同一规则扫描完整原文件。OpenReader 不能为修复引用而静默换规则、删卷标题、按数字
重排或覆盖原 archive。现有真实本地书只可作为只读诊断输入；合同测试使用自建无版权 fixture。

## 目标合同

1. direct、LocalStore、WebDAV 最终确认一本本地书前，以 trim 后精确 `title + author` 查询当前用户的
   `SourceID=0` 行。只有唯一、可证明为“缺少原 archive/目录”的 reader-dev 逻辑恢复占位行可以自动
   回填；远程同名书、其他用户、多个歧义候选均不得被覆盖。
2. 回填复用旧 Book ID 和 URL，并在一个可补偿事务中生成 archive、Chapter、`chapters.json` 与
   `bookSource.json`。原进度、书签、分类和自定义 metadata 保持绑定；失败时占位行与所有引用原样保留，
   新 archive/stage 不残留。
3. 已经完整导入的本地书默认仍按普通导入处理，避免无确认覆盖原文件；本轮不自动合并历史重复书，
   不猜测两个完整 archive 哪个正确。
4. 目录替换时，引用解析顺序为：EPUB canonical resource identity；旧章节标题在新目录中的唯一精确
   匹配；合法旧 index；最后夹取到现有目录边界。匹配到新章时同时更新 ChapterID、ChapterIndex 和
   ReadingProgress.ChapterTitle。
5. 重复标题不做任意选择，回退到 index；书签保留 offset/percent/note，进度保留 offset/percent/mode。
   零章节目录不得伪造 ChapterID。
6. 不改写原文件字节，不删除现有 archive，不跨用户，不修改 reader-dev 逻辑 ZIP 的既有含义；完整
   OpenReader portable v2 和三目录卷恢复保持原合同。

## 测试门

1. 恢复 reader-dev `myBookShelf.json` 本地 row（含 progress/title/category/bookmark），再以 exact
   title/author 补传自建 TXT：最终只有该占位 Book 被回填，ID/URL/引用保持且章节可读。
2. 不同作者、远程同名、外用户、两个占位候选、已有完整本地书均不被隐式覆盖。
3. 新目录在旧目录前插入一章时，进度和书签按唯一标题移动到新 index；标题删除时夹取合法 index，
   不再留下超范围位置；重复标题走稳定 index fallback。
4. parser/archive/DB/metadata stage 注入失败时，旧 row/progress/bookmark/category 和文件状态字节级不变。
5. TXT/EPUB/UMD/CBZ 历史卷、portable restore、重启及跨用户门全部通过。

若后续真实文件证明 TXT 默认规则本身仍与固定上游不同，应另开 parser fixture 合同；不得用本轮引用
修复掩盖 parser 偏差。

## 实施与验证（2026-09-14）

- `e1631d0` 让统一 localbook Importer 仅在当前用户存在一个精确 `title + author`、`SourceID=0`、无
  archive 且无 Chapter 的占位行时原地回填，因此 direct、LocalStore、WebDAV 的共同确认路径都会复用
  reader-dev 恢复后的 Book ID/URL。多个同名候选继续创建新书，绝不猜测覆盖。
- 回填先在新 archive 中完成 EPUB/CBZ 资源准备和逐章 cache，再在事务中替换 Chapter、重连引用并更新
  Book；失败会清理新 archive，不删除或改写原占位记录。
- 目录引用依次使用完整 EPUB `ResourcePath + ResourceFragment + ResourceEndFragment` 身份、旧新目录中
  均唯一的 trim 后标题、原索引和最终有效边界。重复标题回退索引；进度标题随新章更新，offset、percent、
  mode、书签 note 等位置细节保持。
- TXT 占位书原地回填、唯一标题前插、无旧 Chapter 的进度标题恢复、目录缩短钳制、EPUB 同文件多 fragment、
  重复标题、同名歧义不覆盖和 CBZ 资源预备合同均通过；Go 全量、相关包 race 与 `go vet ./...` 通过。
- 本轮没有根据用户真实书籍修改 TXT 解析规则；如真机补传原文件后仍出现目录本身缺章，需要取得不含
  版权正文的最小 fixture 后另开 parser 合同。
- 可信 GitHub Actions run `34794997078` 通过 fresh、portable 和 historical volume 门并发布
  `e1631d0`/`latest` 双架构 OCI index
  `sha256:94030bd8f72dcb5135ade46571a9b81d686da616fc704e4144dc78414a33c3ee`；当前只待用户真机补传原文件签收。
