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
- 可信 GitHub Actions run `34816091195` 通过 backend/frontend/Compose、native、fresh/portable、
  historical volume 和 published-platform 门，并发布 `5b79ad3`/`latest`。amd64/arm64 OCI index 为
  `sha256:558d4476ab2857905f194f18157da9a8b195477ad7733e08160ddf45af4a2e69`；平台 manifests 分别为
  `sha256:4b4864634b595afa13df18562c692ec3712c050e5453b29c29189a941fa42f81` 和
  `sha256:e8dd34ad3a0302516742f10a2ede082ac892a88f35eb05122931971529ac218a`。

## 2026-09-14 第三次真机反馈：正文请求预算与同章请求所有权

用户在 `5b79ad3` 发布后再次确认章节仍会显示“章节加载失败，请检查书源或网络后重试”。因此
`0ecc4d9` 只关闭了整书 gate 的队头阻塞，不能作为真机问题已解决的证据。本轮继续逐项比较
`d0600ab..HEAD` 后确认两个尚未覆盖的请求边界：

1. 固定上游 `web/src/App.vue#getBookContent` 对章节正文显式设置 `timeout: 30000`，其 Axios 全局默认
   是 5 分钟。OpenReader 的 shelf/temporary Reader 正文 GET 没有专用预算，继承通用 API 的 12 秒。
   后端单次书源请求预算本身是 15 秒，正文还可能先解析 `contentUrl` 或继续正文分页；合法慢响应会在
   后端预算到期前先被浏览器中止，并被当前 `readError` 投影成笼统网络提示。这是明确的上游合同差异。
2. `0a8a0ef` 开始把主章节的 `AbortSignal` 直接绑定到共享内容请求的唯一内部 controller。第二个同 scope、
   同 chapter 的主 load 会先 abort 前一个主 load，然后命中 `inFlight` 并复用已经被 abort 的 Promise；
   新 load 不能接管仍可复用的网络工作。目录路由、跨端进度协调或重复导航均可形成该重入。
3. `a131aa9` 让本地书 cache rebuild 正确服从 request context。它消除了断开后继续写 cache 的陈旧提交，
   但与过短的 12 秒客户端预算组合后，大型本地书首次重建会在每次请求中止时回滚，无法像旧实现那样
   在浏览器断开后偶然完成并供下一次请求命中。不能通过恢复 contextless 后台写来解决，应修正调用方预算。
4. `0a8a0ef` 的后端 snapshot/CAS、`a131aa9` 的本地重建事务以及 `0ecc4d9` 的同章 gate 都是正确的数据
   生命周期保护，必须保留；本轮不得退回断开后继续写文件/数据库的旧行为。

修订合同如下：

1. shelf Reader 与 temporary Reader 的章节正文请求显式使用固定上游的 30 秒预算；普通 API 的 12 秒
   默认值不变。AbortSignal 仍可在切书、换源、换章、清缓存、会话失效和卸载时提前取消请求。
2. 同 cache scope、chapter index 和 refresh mode 的底层请求只创建一次。每个调用方拥有独立订阅；一个
   调用方取消只结束自身等待，不能取消仍有其它调用方接管/等待的共享请求。
3. 当最后一个订阅取消且当前事件循环没有新的同章订阅接管时，才取消底层 HTTP。scope clear 必须立即
   取消该 scope 的全部底层请求，不等待订阅计数归零。
4. 同章重入的新主 load 必须能取得共享请求结果并成为唯一可见 generation；旧 generation 仍静默退出，
   不写正文、error、位置或进度。不同章节继续并行，refresh 与 normal 继续使用不同 request key。
5. 测试先证明旧实现的正文 Axios config 没有 30 秒预算，并证明“主 load A 开始 -> 同章主 load B 取消 A
   -> B 复用已取消 Promise”无法成功；实现后断言一个底层请求、A 返回 false、B 返回 true 且显示正文。
6. 真实浏览器以超过 12 秒、低于 30 秒的可控章节响应验证 shelf/temporary Reader 不再提前显示网络错误；
   另以同章快速重入验证请求未重复、最终正文正确、无 409/console error。移动与桌面结果必须一致。

本轮不修改 API 路径、响应 schema、后端书源安全预算、SQLite schema、cache 命名、备份格式或三个持久
目录。30 秒是固定上游正文专用值，不是对所有接口放宽超时。

## 第三次修复与本地验证

- 合同 `1512534`、旧实现红测 `46e4933` 与实现 `c7fbf73` 依次落地。shelf 和 temporary Reader 正文
  Axios config 现显式使用 30 秒；其它 API 仍保留 12 秒默认值。
- 同 request key 的底层 fetch 与调用方订阅已分离。旧主 load 被新 generation 取消时只退出旧订阅；同章
  新 load 可接管仍在执行的请求。最后一个订阅离开后在 microtask 边界确认无人接管才 abort transport；
  scope clear 仍立即取消全部相关请求。
- 旧实现测试稳定得到 `12000 != 30000`，且同章接管者复用已取消 Promise 并收到 `AbortError`；修复后
  shelf/temporary config、同章单 fetch 接管、最后订阅取消和 scope clear 全部通过。
- frontend **756/756**、Vite production build、Go full/vet、Compose config 通过。可配置延迟的 temporary
  Reader Chromium 在 1440x900、390x844、360x800 以 13 秒正文响应跨过旧 12 秒边界，三个视口均显示
  正文且无可见错误、409 或 console error。
- 可信 GitHub Actions run `34853164985` 已通过 backend/frontend/build/Compose、native、fresh/portable、
  historical volume 和 published-platform 全部门，并发布 `c7fbf73`/`latest`。amd64/arm64 OCI index 为
  `sha256:874b0262c48a19c6861f80ca8cfdb157b6c419ebc46e7a7f4dd320358b8897ca`；平台 manifests 分别为
  `sha256:37fd7eab62adabbed3095a8a16fbf745c1ed8e4d5f364d149f99ab727c356dd8` 和
  `sha256:22b12499f54b2527c6fffe8d151efe37cad9403a966f1ccb1386449d572c0021`。状态为
  **implemented / regression-validated / Docker-published / awaiting-device-verification**。

## 2026-09-15 第四次真机反馈：detached 书源快照被错误拒绝

用户在 `c7fbf73` 已发布后仍确认章节无法加载，因此前三轮并发、队列、客户端预算与请求接管修复不能
作为问题关闭依据。重新逐项比较 `d0600ab..HEAD` 的后端正文提交条件后发现一条与既有数据合同直接
冲突、且会稳定复现的路径：

1. `booksources.Service.FindForBook` 从 2026-07-27 起就明确接受 active 或 detached association；清空、
   恢复或替换书源列表时，仍被书架书籍引用的 source 会保留为 detached snapshot，使现有书籍继续阅读。
2. `0a8a0ef` 新增的 `validateReaderChapterContentSnapshot` 却用 `detached=false` 查询同一 association。
   detached 书籍可以通过 fetch 前的 `FindForBook` 并成功抓到正文，却在发布 cache/variable 前固定返回
   `409 chapter content changed; retry`；前端重试不会改变 association，因此仍固定失败。
3. `d0600ab` 没有该提交校验，所以同一数据状态可以读取。这是 8 月 25 日镜像正常、当前镜像失败的
   确定性差异，不依赖网络速度、相邻章节调度或设备视口。
4. 固定上游没有 OpenReader 的多用户 COW/detached 存储层，但其删除/替换书源列表不能使已入架书籍的
   当前来源在正文已成功获取后被内部版本门拒绝。保留 detached snapshot 是已签收的多用户兼容适配。

修订合同如下：

1. 正文 fetch 和提交都接受当前书籍所属用户的 existing association，不要求它仍在活动书源列表中。
2. association 缺失、书籍换源/删除、章节替换、source 行缺失或抓取语义改变仍返回安全 409；不得放宽
   Book/Chapter/source identity、variable/cache CAS、取消或 staged publish 保护。
3. 确定性 API 红测必须创建已入架远程书及 detached association，断言正文只抓取一次、返回 200、保存
   Chapter variable/cache，并且 source 仍保持 detached、不重新出现在活动列表。
4. 同一测试还要删除 association 后重试，证明真正失去用户所有权时仍不发远程请求且不能发布 cache。
5. 不修改 API、SQLite schema、association 数据、备份格式、缓存命名或三个持久目录；不扫描或重写旧卷。

## 第四次修复与本地验证

- 合同 `7e83024`、旧实现红测 `015d255` 与实现 `8bebcbf` 依次落地。正文提交阶段现在和抓取前的
  `FindForBook` 使用同一 existing-association 合同：active 与 detached snapshot 均可提交，但关联缺失
  仍会失败；修复不会把 detached source 重新激活。
- 红测在真实 Gin/SQLite 路径创建被既有书籍引用的 detached source。旧实现抓取一次后固定返回
  `409 chapter content changed; retry`；修复后返回 200，发布 Chapter variable/cache，并保持 source
  detached。随后删除 association 的负向用例证明不会再访问远程书源或发布状态。
- focused/race、章节 API 与 engine source-variable 集、Go 全量/vet、frontend 757/757、Vite build 和
  Compose config 通过。真实 Go + loopback source + Chromium 在 1440x900、390x844、360x800 均返回
  200 并显示正文，无错误占位或 409；三个视口合计只抓取一次，证明 cache 生效且 source 仍 detached。
- 可信 GitHub Actions run `34963585121` 已通过 backend/frontend/Compose、native、fresh/portable、
  historical volume 和 published-platform 门，并发布 `8bebcbf`/`latest`。amd64/arm64 OCI index 为
  `sha256:5d097551c7d5c37bc54b69030ef07146d7b24888583ba2abc3903b7eff8d6a03`；平台 manifests 分别为
  `sha256:d805484871070bbb6215e48015da14cc58b78a2bcc33bc1c584516cac636f371` 和
  `sha256:dccb59866f3b9a611fbc0d81286f6b4d1ddbcd8774f20f91c67e83cda6478cb5`。用户生产环境尚未升级
  验证，当前状态为 **implemented / regression-validated / Docker-published / awaiting-device-verification**。
