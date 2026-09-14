# Reader 远程章节并发加载真机反馈合同（P0）

状态：**implemented / regression-validated / Docker-pending / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
正常对照镜像：`OpenReader@d0600ab`（2026-08-25）。  
问题基线：`OpenReader@919b588`，真机仍出现“章节加载失败，请检查书源或网络后重试”。

## 上游与回归证据

- 上游 `web/src/views/Reader.vue` 的连续章节窗口会并行请求相邻章节。
- 上游 `BookController.kt#getBookContent` 每个请求读取当前书架书和目录，命中缓存即返回；远程抓取
  使用该请求的 `Book`/`BookChapter` 状态，但没有把另一个正文请求刚发布的临时变量/cache 解释成
  必须暴露给用户的全局版本冲突。
- `d0600ab` 不含 Reader 正文快照 CAS，用户确认该镜像没有当前网络报错。
- `0a8a0ef` 新增的 `validateReaderChapterContentSnapshot` 要求远程抓取前后的 Book、Chapter 和 source
  snapshot 完全相等。最小复现是两个同章请求从同一 `Chapter.variable/cache_path` 起步：先完成者发布
  变量/cache 后，后完成者固定返回 `409 chapter content changed; retry`。连续窗口、当前章加载、预取、
  reload 或另一浏览器标签可以形成这种重叠；具体书源若在内容阶段共享 Book 状态，还会扩大到相邻章。
- `2bbb276` 只在前端对精确 409 以 `refresh:true` 重试一次。即使冲突来自另一请求已经发布的可用 cache，
  该 retry 也会强制绕过 cache 再访问书源；这次真实网络失败最终被投影成普通“网络问题”。因此该提交
  不能作为问题已解决的证据。

## 目标合同

1. 缓存命中继续无锁快速返回；不同用户或不同书籍的远程章节仍可并发。
2. 同一 `user/book` 的未缓存远程抓取按请求到达顺序串行。等待者取得执行权后必须重读当前 Book、
   Chapter 和当前用户可见的 BookSource，再用最新合法变量抓取，不能沿用入队前的旧变量。
3. 等待期间若相同章节已产生缓存且请求不是显式 refresh，应直接读取缓存，不重复访问书源。
4. 书被删除、换源、目录替换、章节身份改变或书源语义改变时，旧请求仍返回稳定 409；不得提交旧
   cache/变量，也不得记为书源网络失败。
5. caller cancellation 在排队、抓取、stage 或 commit 任一阶段停止该请求；不能阻塞队列、留下 cache、
   改写变量或发送迟到响应。
6. 成功结果仍以 staged file + guarded transaction 发布；本合同只移除正常重叠加载造成的伪冲突，
   不放松删除、换源、目录和书源配置的生命周期保护。
7. 前端保留一次精确 stale 409 恢复作为跨标签页/真实换源保护，但正常连续阅读不得依赖该重试才能
   加载相邻章节，也不能把服务端 409 文案直接显示给用户。

## 测试门

1. 两个同章未缓存请求同时执行：旧实现稳定产生一个 409；修复后均为 200，等待请求读取先完成者的
   cache，远程抓取只有一次，cache 与章节变量正确。
2. 两个相邻未缓存章节同时请求：若规则只写各章变量，两章都成功；若后续支持共享 Book variable，
   第二次抓取必须观察到第一次提交后的合法状态，不能产生伪 409。
3. 显式 refresh 在同章请求之后仍会串行再抓取；不能错误命中普通等待者的 cache 快路径。
4. 不同书籍的阻塞 fixture 证明远程抓取仍并行。
5. 排队取消、抓取后取消、换源/删书/换目录的现有生命周期测试全部保持。
6. 真实 Go + 浏览器在 1440x900、390x844、360x800 连续跨章，断言无 409、无错误占位、无 console
   error，并记录实际请求顺序。

本切片不修改 API 路径、response schema、SQLite schema、cache 命名、备份格式或三个持久目录。

## 实施与验证（2026-09-14）

- `e1631d0` 为每个 `user/book` 增加可取消的远程章节抓取门；等待者取得门后重读 Book/Chapter，普通
  请求优先消费先行请求刚发布的 cache，不再以旧 variable 重复抓取。不同书使用不同门。
- 真正的删除、换源、目录替换和 source 语义变化仍由原 staged cache + guarded transaction 返回 409；
  前端仅对精确 stale 冲突重试一次，但重试不再强制绕过另一请求已发布的 cache。
- 两个同章并发请求的 Gin/engine 合同均返回 200 且只抓取一次；排队取消、不同书独立门、抓取后取消、
  source 变化和 cache 提交均通过，相关包 race 通过。
- Go 全量、`go vet ./...`、frontend 753/753、Vite build、Compose config 通过；连续 Reader 的 scroll 与
  scroll2 在 1440x900、1024x1366、390x844、360x800 通过，移动/面板及章节 cache 四视口合同也通过。
- 可信 GitHub Actions run `34794997078` 通过 backend/frontend/Compose、native、fresh/portable、historical
  volume 和 platform 门并发布 `e1631d0`/`latest`；OCI index 为
  `sha256:94030bd8f72dcb5135ade46571a9b81d686da616fc704e4144dc78414a33c3ee`。用户真机仍待验证；在真机
  完成前不得写成 device-closed。

## 2026-09-14 第二次真机反馈与历史定位

用户在 `e1631d0` 发布后再次确认普通章节仍会显示“章节加载失败，请检查书源或网络后重试”，而
`d0600ab` 没有该问题。本次按 `d0600ab..HEAD` 的提交历史重新取证，上一节的整书串行合同被真机证据
否决，不再作为正确实现依据。

历史差异收敛如下：

1. `d0600ab` 对同一本书的不同章节并行远程抓取；Reader 的连续窗口和半径为 2 的预载早已存在，故
   相邻章节并发本身不是新增行为。
2. `0a8a0ef` 增加 Book/Chapter/source 快照 CAS。同章重复请求会竞争同一 Chapter variable/cache，后
   完成者返回 409；这是第一次真机可见错误的直接来源。
3. `2bbb276` 只在前端重取 409，没有消除服务端竞争。
4. `e1631d0` 以 `user/book` 为 key 串行所有未缓存远程章节，消除了同章竞争，却把相邻章也放进同一
   队列。浏览器通用 Axios 超时为 12 秒，服务端单次书源请求预算为 15 秒；因此一个慢章节足以让后续
   章节只因排队超过浏览器预算，被前端投影成“网络问题”。这条队头阻塞是 `d0600ab` 到当前版本之间
   新增且可确定复现的第二次回归。
5. 正文 parser 创建 chapter variable scope 后，正文 `@put` 写入 Chapter variable；同一本书不同章节
   不共享该提交目标。Book variable 在正文解析中只作为父级读取状态返回，正常相邻章节无需串行抓取。

修订后的合同：

1. 同一 `user/book/chapter` 的未缓存普通请求合并：等待者在先行请求发布后读取 cache，不重复抓取。
2. 同一书的不同章节必须保持并行，且继续服从书源自身 `concurrentRate`；OpenReader 不额外施加整书
   串行队列。
3. Chapter/source/book 身份与变量的 staged publish、CAS、取消和换源保护保持；不得退回 `d0600ab`
   的无保护覆盖写入。
4. 同章显式 refresh 与普通请求继续按同章边界协调，不能覆盖另一请求正在发布的 cache/变量。
5. 新回归测试必须让同书两个不同章节的第一轮 HTTP 请求都在任一请求释放前进入 transport；旧整书
   gate 必须稳定失败。另保留同章只抓取一次、排队取消、换源/删书/source 编辑等既有门。
6. 浏览器 12 秒超时与服务端 15 秒安全预算是既存配置，本切片不靠放大客户端超时隐藏排队问题；在
   固定慢请求 fixture 下，相邻章节必须各自只承担自己的网络耗时。

## 第二次修复与验证

- 合同 `981d400`、旧实现红测 `99cfc88` 与实现 `0ecc4d9` 依次落地。远程章节 gate key 从
  `user/book` 收缩为 `user/book/chapter`；同章普通请求仍只抓取一次并读取已发布 cache，同书相邻章
  恢复固定上游已有的并行请求行为。
- 红测通过两个真实 Gin 章节正文 GET 阻塞 transport：旧实现第二章在 250 ms 观察窗内无法开始；修复后
  两章均在任一响应释放前进入 transport，并各自返回 200 正文。
- 同章合并、相邻章并行、排队取消、不同书隔离、source 语义变化、fetch 后取消的 focused 与 race
  通过；章节 API/remote reader 相邻集、engine chapter/source-rule 集、Go 全量与 vet 通过。
- frontend 754/754、Vite build、Compose config 通过。本切片没有前端或可见布局改动；真实设备仍需用
  发布后的镜像复验，当前不得标记 device-closed。
