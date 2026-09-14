# Reader 远程章节并发加载真机反馈合同（P0）

状态：**implemented / regression-validated / Docker-published / awaiting-device-verification**。

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
