# Reader 连续跨章固定基准合同（P0）

状态：**2026-07-18 已按固定上游完成 CONT-FIX-1…6：连续模式使用顶部安全区后的 `h3/p` 确定章节和进度，窗口事务在锚点恢复前禁止保存，append/retry/compute 均受书籍、模式和 generation 约束。生产实现、单元测试和三视口真实浏览器合同均已通过；等待本批 Docker 发布。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游权威实现为 `web/src/views/Reader.vue` 的 `isScrollRead`、`loadShowChapter()`、
`computeShowChapterList()`、`scrollHandler()`、`getCurrentParagraph()`、`captureScrollAnchor()`、
`restoreScrollAnchor()`、`saveReadingPosition()`、`toNextChapter()` 和 `toLastChapter()`，以及
`web/src/components/Content.vue#renderScrollChapterList()`。

## 1. 固定上游状态合同

| 关注点 | 固定上游行为 | OpenReader 要求 |
|---|---|---|
| 模式资格 | 只有 `上下滚动`、`上下滚动2` 且不处于 EPUB、audio、普通漫画/slide 分支时进入连续窗口。 | `scroll`/`scroll2` 使用多章 DOM；EPUB、audio、flip/page 和普通漫画不误入。CBZ 是否保留 mode 由独立 CBZ 合同决定。 |
| 初始窗口 | `scrollStartChapterIndex = chapterIndex`、`showNextChapterSize = 1`、`showPrevChapterSize = 0`。 | 首次只渲染当前章和最多一章相邻下一章；不渲染上一章或第二个下一章。 |
| `上下滚动` | 从显式进入/跳转的 `scrollStartChapterIndex` 开始保留已渲染章节；自然滚动只更新当前章。 | 追加下一章时不删除此前章节。 |
| `上下滚动2` | 每次重算从当前可见 `chapterIndex - showPrevChapterSize` 开始；固定基准中 previous size 为 0。 | 只在受控的前向扩章事务中删除当前可见章之前的已读章，不合成一个上一章窗口。 |
| 自然反向扩章 | 顶部预加载上一章的代码被注释。 | 滚到顶部不加载上一章；上一章按钮、目录、搜索和书签仍可显式重建目标章。 |
| 前向阈值/大小 | `scrollTop > scrollHeight - 4 * viewportHeight` 时，串行加载最后一个已显示 index 的下一章。 | 严格使用四视口阈值，每个事务只追加一个相邻 index，书末不继续调度。 |
| 当前段落 | 若没有 `.reading`，按 DOM 顺序取第一个 `h3,p`，条件是其底边超过 `30 + 20 + webAppDistance + safeArea.top`。 | 连续模式的当前章、标题、锚点和保存位置必须由顶部安全区后的第一个可读标题/段落决定，不能改用视口中部锚点。 |
| 保存位置 | `saveReadingPosition()` 在当前段落所属 `.chapter-content` 中切换 chapter/index/title，并以该章 `innerText` 中当前段落的起始位置保存。 | 顶部标题对应位置 0；同一快照更新章身份、本地位置和服务端进度。Go 数据模型可保留 percent/offset 增强。 |
| 事务隔离 | `preCaching` 串行化扩章；`startSavePosition = false` 覆盖列表替换、锚点恢复和重新分页，`saveReadingPosition()` 在两种锁期间直接返回。 | DOM 删除/追加至锚点恢复完成之前不得生成本地或服务端进度；完成后再从稳定 DOM 生成一个快照。成功、失败、跳转和卸载都必须释放锁。 |
| 锚点 | 替换前捕获当前 `h3/p` 的 chapter index、`data-pos` 和 viewport top；替换后按同一节点恢复并 clamp 到真实滚动范围。 | `scroll2` 删除已读章和两种模式追加下一章均不得移动正在阅读的节点。 |
| 显式跳章 | 上/下一章、目录、搜索和书签设置新的 start chapter，重建窗口并定位目标；与自然扩章不同。 | 显式事务必须使所有更早的扩章/重试结果失效，旧书/旧章结果不得写回新窗口。 |
| 相邻失败 | 上游缓存可见的 `获取章节内容失败！` error block，后续加载允许替换 error。 | 当前章保持可读；显示内联错误和明确重试，不能空白、无限静默重试或锁死。 |

## 2. 当前实现矩阵

| 层 | 当前文件与行为 | 判定 | 必须动作 |
|---|---|---|---|
| 模式/窗口 | `readerEffectiveMode()`、`readerChapterWindowIndexes()` 和 `useReaderChapterWindow.compute()` 初始生成 `[current, next]`；`scroll` 保留显式起点，`scroll2` 在追加时 prune。 | `aligned` | 保留现有窗口测试。 |
| 阈值/边界 | `readerChapterWindowExtension()` 使用严格的 `top > height - 4 * viewport`；`adjacentReaderChapterIndex()` 限制书籍边界。 | `aligned` | 保留阈值两侧和书末不请求测试。 |
| 原生与分段输入 | 浏览器 wheel/touch 操作原生滚动容器；点击区和键盘调用 viewport-sized animator。 | `user-requested acceptable-change` | 原生滚动不得套动画时长；点击/键盘仍使用用户动画时长。 |
| 当前段落 | `currentProgressElement()` 在连续模式按顶部 50px 边界选择 `h3[data-pos], [data-reader-block]`；`currentVisibleParagraph()` 仍单独服务书签、选择文字和自动阅读。 | `aligned` | 标题参与章身份并固定为位置 0；中部可见段落与进度边界不再混用。 |
| 位置精度 | `readerBlockTextOffset()` 可按段落内部可见比例细化 offset；服务端另存 chapter/full-book percent。 | `acceptable-change with guard` | 可保留更细 offset，但标题可见时必须为 0，且不得改变上游当前章切换边界。 |
| 扩章进度隔离 | `useReaderChapterWindow` 暴露共享 `busy`；`useReaderScrollSync` 和 `useReaderProgressPersistence` 在 busy 期间不写本地快照、不调度/发送 PUT，锚点恢复后由 `onStable` 保存一次稳定快照。 | `aligned` | 保留 busy 从请求开始覆盖到 DOM 替换及锚点恢复结束的测试。 |
| 异步失效 | compute/append/retry 共享 generation 与 scope key；书籍、远程会话、来源、mode、显式加载和卸载都会使旧事务失效。 | `aligned` | 保留延迟 append/retry 在 rebuild、换书和切 mode 后不得写回的测试。 |
| 后台预取 | `useReaderChapterContent` 在当前章可见后后台预取半径 2，按 book/chapter/refresh 去重；额外章只进内存，不进入 DOM。 | `acceptable-change` | 不阻塞当前章，不产生重复请求，不得绕过 generation 写入另一书窗口。 |
| 错误/重试 | 相邻失败生成 `.chapter-inline-error`；自动扩章暂停，按钮可 refresh 重试。 | `acceptable-change` | 比上游更明确的重试入口保留；重试同样受 generation 约束。 |
| 内部滚动容器 | 上游滚动 document；OpenReader 滚动 `.reader-content`。 | `technical-stack-equivalent` | 几何、阈值、锚点、键鼠触摸和安全区结果须等价。 |

## 3. 路由、API 与数据边界

- 保留 `/books/:id/read?chapter=&offset=&percent=&resume=1` 与旧链接兼容；显式 route jump 进入
  同一窗口重建事务。
- 保留认证章节 API 和当前 browser memory cache。连续窗口不新增后端路由或 SQLite 列。
- 保留 `PUT /progress` 的多用户、冲突时间戳、chapter id/index、offset、chapter/full-book percent
  与 mode；但 payload 必须来自同一个稳定可见章快照。
- 本批不迁移 `data/`、`cache/`、`library/` 或本地 persisted reader settings。
- 额外后台预取是性能适配，不得改变渲染窗口、当前章、错误显示或进度顺序。

## 4. 先失败的合同测试

| 编号 | 必须先失败的断言 | 层 |
|---|---|---|
| CONT-FIX-1 | 顶部安全区后标题可见时 snapshot 为该章/index/offset 0；跨章时只在上一章最后一个 `h3/p` 离开顶部边界后切换，不按 32% 视口锚点延迟。 | visibility/progress |
| CONT-FIX-2 | append/prune 从请求开始到锚点恢复结束保持 busy；期间 scroll 不写 local snapshot、不调度 PUT；完成后只保存稳定章和稳定 offset。失败也释放 busy。 | window/scroll sync |
| CONT-FIX-3 | 延迟 append 后执行显式 jump、切 mode 或切 book，旧结果不改变新 blocks/current chapter/content；延迟 retry 同样失效。 | async state |
| CONT-FIX-4 | `scroll` 追加保留起点；`scroll2` 删除当前章以前 blocks；两者严格四视口阈值、每次一章、无自然反向扩章、书末稳定。 | window policy |
| CONT-FIX-5 | 相邻失败保留当前正文、显示一次错误、手动重试成功；请求去重与后台预取不能造成重复可见事务。 | loader/error |
| CONT-FIX-6 | 1440×900、390×844、360×800 真实浏览器验证顶部章切换、原生 137px wheel/touch、点击/键盘分段、append/prune 无跳动、延迟事务期间无错误 progress PUT、显式跳章使旧请求失效。 | browser |

## 5. 实施与发布闸门

1. 先提交本合同，不修改应用代码。
2. 添加 CONT-FIX-1…3 的失败测试，并纠正旧 smoke 中只检查 DOM/请求数、未检查进度事务的缺口。
3. 实现独立的顶部当前段落策略、可观察窗口 transaction/generation 和稳定后单次同步。
4. 跑前端全量、Go 全量、生产构建及 Reader mobile/image/EPUB/CBZ/continuous 浏览器矩阵。
5. 这一切片不改持久格式，但下一张 Docker 仍必须通过历史 volume/portable backup 门禁；达到可人工验证状态即可中途发布。

## 6. 允许差异与非目标

- 保留用户明确要求的原生连续 touch/wheel 与离散点击/键盘翻页。
- 保留 Vue 3/Pinia、内部滚动容器、Go 多用户 progress、精细 percent/offset、后台有界预取和
  显式错误重试，但它们不得改变上游当前章边界或保存瞬态状态。
- 本批不顺带签收 EPUB、CBZ、audio、TTS、自动阅读引擎或 Index 工作台；相邻模块只跑回归。

## 7. 2026-07-18 实施与验证结果

- CONT-FIX-1：新增顶部边界选择器；标题位于安全区时章节 offset 为 0，上一章末段越过边界后才切换到下一章。
- CONT-FIX-2：窗口 busy 覆盖相邻章请求、DOM 替换和锚点恢复；瞬态滚动不再写本地或服务端进度，稳定后只保存一次。
- CONT-FIX-3：compute、append 和 retry 绑定 generation、book/session/source scope 与 mode；显式跳章、换书、切模式和卸载均使旧结果失效。
- CONT-FIX-4…5：保留已对齐的四视口阈值、`scroll` 保留、`scroll2` prune、书末稳定、内联错误和手动重试。
- CONT-FIX-6：`reader-continuous-contract.mjs` 在 1440×900、390×844、360×800 验证两种连续模式、顶部章切换、137px 原生滚动、离散点击/键盘、失败重试、锚点稳定和延迟事务期间无错误 progress PUT。

回归证据：

- `backend/go test ./...`：通过。
- `frontend/npm test`：434/434 通过。
- `frontend/npm run build`：通过。
- mock 浏览器：continuous、mobile、image、text modes、TTS、volume、audio 合同通过。
- 真实 Go 导入/阅读：EPUB 三视口与 CBZ 合同通过。
- macOS 首次启动 text-modes Chrome 时曾因 Mach port 权限在测试执行前失败；获准重启相同合同后通过，未发现产品崩溃。
