# Reader 移动端上下滑动点击翻页与章末入口合同（P0）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-19 第六次用户实测修复、全量门禁和 Docker 发布已完成，等待本批实机复验。第五批
已发布，但实机仍能感到到页时的轻微顿挫；新渲染轨迹确认最终动画帧同步承载了章节/布局/
进度结算，旧合同把“动画结束后结算”错误地等同于“不会阻塞最后一帧”。前三批分别
移除逐帧重型同步、尝试合成层分页、清理普通点击选文轮询并缩短起步死区；第四次复审确认
“把整章正文提升为合成层”本身会在真实移动设备制造不可由 Long Task 观测的栅格化/GPU 压力，
不能继续以当前 WAAPI 轨迹通过作为丝滑度证据。第一批
移除逐帧重型同步并恢复章末入口；第二批把移动端“上下滑动”的点击分页移到浏览器合成层；
第三批继续移除普通点击的选文轮询、提前准备合成层并缩短可见起步死区。不能再沿用“50ms 内
移动 1px”作为丝滑度已达标的证据。

## 权威文件

- 上游 `web/src/views/Reader.vue`
  - `nextPage()`、`prevPage()`、`scrollContent()`、`scrollHandler()`、`toNextChapter()`；
  - 模板中的 `.bottom-bar > .bottom-btn`；
  - 移动 `.chapter`、`.content-inner` 和通用 `.bottom-bar` 样式。
- 上游 `web/src/plugins/animate.js`
  - `requestAnimationFrame` 驱动的 cubic ease-in-out。
- 当前 `frontend/src/utils/readerAnimation.js`
- 当前 `frontend/src/composables/useReaderNavigation.js`
- 当前 `frontend/src/composables/useReaderScrollSync.js`
- 当前 `frontend/src/views/Reader.vue`
- 当前 `scripts/smoke/reader-text-modes-contract.mjs`

## 兼容矩阵

| 项目 | 固定上游行为 | 当前行为 | 判定与动作 |
|---|---|---|---|
| 点击分页距离 | `上下滑动` 每次移动 `windowHeight - scrollOffset`；默认排版在 390×844 / 360×800 分别为 772px / 728px。 | `scrollStep()` 和真实浏览器合同得到相同距离。 | `aligned`；不得修改分页距离。 |
| 持续时间与缓动 | `animateMSTime` 为 0 时同步定位；正数使用 cubic ease-in-out，并在设置的毫秒数内完成。 | `createReaderScrollAnimator()` 使用等价缓动并区分 0/100/500ms。 | `aligned`；本批不以改变缓动曲线掩盖性能问题。 |
| 动画帧工作量 | `scrollContent()` 每帧只写 document scrollTop；`scrollHandler()` 在普通上下滑动分支只计算页码，并以 timer 延迟保存进度。章节窗口扩展只属于连续滚动分支。 | 每次动画写 `reader-content.scrollTop` 都触发 `useReaderScrollSync.handle()`；该方法同步执行章节 DOM 判定、窗口扩展判断、布局重算、可见段落扫描、本地进度组装和保存调度。 | `must-fix`；程序化点击动画期间不得执行重型 scroll 同步，动画完成后统一结算一次。 |
| 原生手指/滚轮 | 用户明确要求手指和滚轮保持原生连续滚动。 | touch/wheel 会取消程序动画并交给滚动容器。 | `intentional-redesign`；本批优化不得量化或接管原生滚动。 |
| 重复输入与取消 | 上游 `transforming` 拒绝重叠翻页。 | animator 拒绝重叠；touchstart/wheel/切章/卸载可取消。 | `aligned`；取消后不得执行旧动画的最终进度结算。 |
| 章末入口 | 普通非 slide、非连续滚动、非错误正文在内容流末尾显示可点击的 `加载下一章`；它不是固定工具层或 Toast。 | 当前 `ReaderChapterContent` 后没有章末入口。 | `must-fix`；在 page 分支正文流末尾恢复入口，工具层显隐不得影响它。 |
| 章末点击 | 有下一章时调用 `toNextChapter()`；最后一章仍显示入口，点击提示 `本章是最后一章`，不越界、不回到章首。 | 当前没有对应动作；`goChapter()` 会 clamp 越界 index，不能直接拿来处理末章。 | `must-fix`；增加显式边界动作，不能依赖 clamp。 |
| 事件穿透 | 上游 `.bottom-btn` 自己处理点击。 | 当前移动端 touchend 由整个 reader page 处理；若只增加 click handler，按钮触摸可能先触发正文分区翻页。 | `must-fix`；章末按钮必须阻止 touch/click 穿透，同时保留键盘可访问性。 |

## 先失败的测试

1. `useReaderScrollSync`：当页动画 active 时，连续 scroll 事件不得调用章节同步、窗口扩展、
   布局重算或进度组装；动画 settled 后恰好统一执行一次。
2. `useReaderNavigation`：竖向点击分页完成时触发一次 settled 回调；被取消或重叠拒绝时不触发。
3. Reader 源码/组件合同：page 正文流末尾存在 `加载下一章`；flip、scroll、scroll2、audio
   不出现该入口；按钮阻止 touch/click 穿透。
4. 章末动作：非末章进入 index + 1；末章显示 `本章是最后一章` 且不导航、不回章首。
5. 真实 390×844、360×800 触控合同：
   - 300ms 点击分页的逐帧轨迹单调、终点仍为 772px / 728px；
   - 动画期间重型 scroll-sync 计数为 0，完成后为 1；
   - 连续快速采样不得出现超过两个刷新周期的静止平台后再突跳；
   - wheel/touch 原生连续滚动仍通过；
   - 滚至正文末尾可见并点击 `加载下一章`，进入下一章；末章点击显示边界提示。

## 实施边界

- 不改变动画时长取值、默认值、分页步长、cubic 缓动或用户已批准的原生连续滚动。
- 不新增后端 API、数据库字段、缓存格式或持久化设置。
- 可以为 scroll 同步增加“动画中延迟、完成后单次 flush”机制；原生滚动仍需及时更新页码和进度，
  不能因优化而丢失位置。
- 章末入口属于正文流；不能放进固定移动工具层，也不能在工具层隐藏时消失。

## 发布闸门

完成失败测试、实现、前端全量、生产构建、Go 全量及三视口 Reader 浏览器回归后，
这一批具备独立人工验证价值，应本地构建并推送 Docker。

## 2026-07-18 实施与验证结果

- `useReaderScrollSync` 在程序化竖向分页动画 active 时只记录待结算状态，不执行章节 DOM
  判定、窗口扩展、布局读取或进度组装；动画完成后由导航回调 flush 一次。
- flush 记录最终 scrollTop，浏览器在最终帧之后补发相同位置的 `scroll` 时直接去重；被
  touch/wheel 取消的动画不会执行旧完成回调，下一次真实滚动仍可正常同步。
- 保留原有 cubic ease-in-out、0…500ms、772px / 728px 移动分页步长和原生 touch/wheel。
- page 正文流末尾恢复 `加载下一章`；按钮拦截 touch/click 穿透。有下一章时显式进入
  index + 1，末章提示 `本章是最后一章`，不调用会把越界 index clamp 回当前章的通用路径。
- 前端全量 471/471、Vite 生产构建、后端 `go test ./...` 通过。
- `reader-text-modes-contract.mjs` 在 390×844 和 360×800 采样 300ms 触控分页轨迹：运动区间
  单调且有连续不同帧，终点仍为 772px / 728px；正文末尾触摸连续进入第二、第三章，末章
  保持 `chapter=2` 并显示边界提示。
- Reader desktop/mobile、continuous、image、volume 浏览器合同通过；隔离 Go+SQLite 实例的
  EPUB 1440×900/390×844/360×800 真实上传解析阅读和 CBZ 合同通过。
- 已在本机完成多架构 Docker 构建和历史卷/可移植备份门禁，并推送
  `ghcr.io/changshengyu/openreader:79645c8` 与 `latest`；两标签共同指向索引
  `sha256:075a69352e7aa862383408a765c35f4638dc1bbbc45fc4e6dde8d11095803670`，
  包含 `linux/amd64` 和 `linux/arm64`。

## 2026-07-18 第二次移动端复审

第一批实现解决了动画期间重复执行章节同步的问题，但没有消除动画自身的主线程负担：

| 项目 | 固定上游 | 当前实现 | 第二次判定 |
|---|---|---|---|
| 动画承载 | 上游直接滚动页面根文档；页面结构较轻，动画帧只写根滚动位置。 | Vue Reader 使用独立 `.reader-content` 滚动容器；`createReaderScrollAnimator()` 每个 `requestAnimationFrame` 写一次 `scrollTop`。即使业务同步被延迟，内层滚动、绘制和浏览器滚动事件仍在主线程发生。 | `must-fix`：移动端 page 点击分页改为合成层上的正文视觉位移，结束时一次性提交精确 `scrollTop`。桌面和无合成能力环境保留安全回退。 |
| 旧 cadence 测试 | 无。 | 采样器和滚动动画都运行在页面主线程。主线程停顿时二者一起暂停；只要恢复后仍有足够不同位置，旧断言就会通过。当前旧 smoke 已再次通过，但用户仍可复现顿挫。 | `invalid evidence`：保留终点/步长断言，新增动画机制、输入延迟和最长停顿契约，不能再用“位置单调”代替丝滑度。 |
| 首帧与结束提交 | 上游首个动画帧开始 cubic ease-in-out，结束后写最终根滚动位置。 | 当前首帧同样从接近零位移开始，但还会触发内层滚动管线；结束后再 flush 业务同步。 | `must-fix`：合成动画必须在一次渲染提交中启动；结束时“提交 scrollTop + 清除视觉 transform”不得产生闪回或二次过渡。 |
| 手指/滚轮打断 | 上游普通手指滚动不属于用户要求的连续原生改造。 | 用户已明确要求手指/滚轮保持原生连续滚动；touchstart/wheel 会取消点击动画。 | `intentional-redesign`：取消合成动画时先把当前视觉进度提交为真实 scrollTop，再交还原生滚动，不能跳回起点。 |
| 动画中再次点击 | 上游 `transforming` 直接拒绝。 | 当前 `isActive()` 也直接返回，快速点击会没有反馈，体感上像停顿。 | `acceptable user-requested improvement`：至少保留一个同方向待执行分页；当前动画落稳后立即执行，不能无限排队，也不能影响触摸取消。 |

### 第二批先失败测试

1. animator 合同：移动 page 分支使用正文视觉元素的 transform/WAAPI 动画，动画期间不逐帧写
   `scrollTop`；结束时只提交一次目标 scrollTop，并清除临时合成状态。
2. 取消合同：touch/wheel 在动画中断时，把已经显示的视觉位移换算为真实 scrollTop，再取消
   transform；不得执行旧 onFinish，也不得丢失用户当前看到的位置。
3. 连续点击合同：同方向第二次点击在第一段动画结束后执行下一页；最多保留一个待执行动作，
   反向输入或原生手势取消时清空，避免产生失控翻页。
4. 真实 390×844、360×800：验证移动 page 点击时存在合成 transform，动画中 `scrollTop`
   保持起点，结束后精确落到 772px / 728px；连续两次点击到达两页距离；touch/wheel 仍为
   原生连续滚动。
5. `0ms` 继续同步跳转；100/300/500ms 仍使用配置时长。EPUB、flip、scroll、scroll2、桌面
   分支不得被错误套用移动 page 合成动画。

第二批属于用户明确要求的流畅度优化。它保持上游的分页距离、设置时长、最终位置和章节边界，
只把移动端 page 点击的中间视觉运动移到浏览器合成层，并为连续点击增加一个有界缓冲。

### 第二批实施与验证结果

- `createReaderScrollAnimator()` 在移动 text/page 分支使用正文 transform 的 Web Animations
  合成动画；以约 16ms 间隔采样原 cubic ease-in-out 曲线，不在动画中逐帧写内层滚动容器。
- 动画结束时在同一脚本提交中写一次目标 `scrollTop` 并移除临时 animation/`will-change`；
  touch/wheel 打断则根据当前动画时间提交已经显示的视觉位移，不闪回、不调用旧完成回调。
- 普通触摸按下不再直接取消动画；只有移动距离超过 14px、确认进入拖动后才交还原生滚动。
  同方向快速点击最多缓冲一次，反向输入或原生手势取消会清空缓冲。
- 合成路径只用于移动普通文本 page；桌面、flip、scroll、scroll2、EPUB、audio 和普通图片漫画
  均保留原路径，避免给大图或 iframe 建立不必要的合成层。
- 前端全量 `479/479`、生产构建和 Go 全量测试通过。
- `reader-text-modes-contract.mjs` 在 390×844 与 360×800 验证：输入到首个可见位移不超过
  50ms、运动区间没有超过 50ms 的停帧、动画中真实 `scrollTop` 保持起点、结束精确到
  772px / 728px，60ms 间隔双触摸最终到达两页距离。
- Reader desktop/mobile、continuous、image 合同通过；隔离 Go+SQLite 服务的真实 EPUB
  上传、解析、iframe 阅读和返回行为在两个移动视口通过。
- Git 提交 `18d9183` 已推送 `main`；本机完成 ARM64 预构建和 `data/cache/library` 卷、
  可移植备份兼容门禁后，再本地生成并上传 AMD64+ARM64 OCI 镜像。
- `ghcr.io/changshengyu/openreader:18d9183` 与 `latest` 共同指向索引
  `sha256:25a3ec40992b2804e68d2bfee9d2137110d6a022ff63fe4fafec65ba8d9b4eed`；
  AMD64 manifest 为 `sha256:b391dc6b9f3c2f210def19d0d0d23dcbeb0c0d582f99c3fdb5d04d8e0aed9143`，
  ARM64 manifest 为 `sha256:3475cae614407bcef99f3607e1286601e3d75600350eb0ec6548f18f02ea1f32`。
- 两个远端标签均通过 host-network `docker buildx imagetools inspect` 核验。OrbStack daemon
  随后的 `docker pull` 三次在 GHCR `HEAD` 请求处返回 `502 Bad Gateway`；这条本机 daemon
  网络路径异常不改变已验证的远端索引，未伪报为远端拉取通过。

## 2026-07-18 第三次移动端复审

第二批证明了动画已进入合成层、终点和持续时间正确，但用户人工验证仍感到起步顿挫。重新沿
`touchstart → touchend → 选文检查 → nextPage/previousPage → WAAPI 首帧` 审查后，确认现有
合同遗漏了输入前预热和普通点击的旁路工作量：

| 项目 | 固定上游 / 用户合同 | `18d9183` 当前行为 | 第三次判定 |
|---|---|---|---|
| 普通点击的选文检查 | 上游在触摸开始时同步检查一次已有选区；普通翻页不会在动画期间持续轮询选区。移动选文操作弹窗仍必须能处理浏览器延迟建立的选区。 | 每次 `touchend` 先安排一次 200ms 检查，随后 `tapPoint()` 又改为 0ms；即使没有选区，`useReaderSelection.schedule()` 仍以约 80ms 间隔重试，最长约 720ms。 | `must-fix`：已有选区继续优先，长按/拖选保留延迟重试；普通短点击必须只做一次同步判定，不能在翻页动画期间建立选文轮询。 |
| 合成层准备时机 | 上游没有独立超长内层正文需要临时提升。当前 Vue 结构允许用等价的提前合成准备，但不能常驻占用所有模式的图层资源。 | `will-change: transform` 在 `touchend` 启动 WAAPI 的同一调用栈才写入；移动浏览器可能在输入后才为整段正文建立合成层和栅格。 | `must-fix`：仅移动 text/page 在 `touchstart` 预热，真正翻页复用该准备；中心点击、选文、拖动、切章和卸载要释放，EPUB/图片/其它模式不得常驻提升。 |
| 首段可见速度 | 上游 power-cubic ease-in-out 在数学上从零速度开始。用户本轮明确要求移动点击更丝滑；分页距离、总时长和终点仍是硬合同。 | 300ms、772px 时，现有曲线第一个刷新周期约移动 0.5px；旧浏览器断言只要求 touchend 后 50ms 内超过 1px，因此自然缓入造成的肉眼“先停一下”仍会通过。 | `intentional-redesign`：允许仅对移动 text/page 合成动画使用更快建立可见速度、仍平滑收尾的曲线；桌面/回退路径先保持上游曲线。必须测量 16/32ms 位移和最终持续时间，不能把动画改成瞬移。 |
| 测试可观测性 | 用户体感是最终验收依据；自动测试需要否决已知假阳性。 | 现有 cadence 测试检查 animation 存在、50ms 内 1px、帧间隔不超过 50ms，但不记录选文轮询、预热状态、首 16/32ms 有效位移或动画期间 Long Task。 | `invalid evidence / must replace`：新增普通点击零重试、预热复用、首 32ms 明显位移、无主线程长任务和精确终点合同；保留原生连续触摸/滚轮回归。 |

### 第三批先失败测试

1. `useReaderSelection` / `useReaderPointer`：普通短点击没有选区时不创建任何延迟重试；已有选区
   仍阻止翻页；长按或真实拖选仍可在浏览器延迟建立选区后打开操作弹窗。
2. animator / navigation：移动 text/page 的 `touchstart` 可提前准备唯一视觉元素；翻页动画
   复用准备状态，结束或取消恢复原样；非翻页点击和非目标格式不泄漏 `will-change`。
3. 动画曲线：`0ms` 同步；正数在首个 16/32ms 已建立可感知但非跳变的位移，100/300/500ms
   仍按配置结束，最终位置仍为一个页面步长。
4. 真实 390×844、360×800：记录 touchstart/touchend、动画创建、16/32ms 视觉位移、Long Task、
   `scrollTop` 提交和选文轮询次数；不得再用“50ms 内超过 1px”替代完整起步合同。
5. 中心点击、工具层、长按选文、手指/滚轮原生连续滚动、连续点击缓冲、章末入口、EPUB、图片、
   flip/scroll/scroll2 和桌面路径全部保持既有合同。

### 实施边界

- 本轮不修改后端、数据库、阅读进度格式、分页距离、动画设置范围或章节边界。
- 曲线变化只允许用于用户指出问题的移动普通文本 page 点击合成路径，并单独记录为用户要求的
  流畅度优化；键盘、桌面、无 WAAPI 回退仍保留原先语义。
- 不以永久 `will-change` 换取测试分数；预热必须有明确生命周期，避免超长章节占用图层资源。

### 第三批实施与验证结果

- `useReaderSelection.schedule()` 新增显式 `retry:false` 单次检查；普通短点击无选区时不建立
  timer。移动端不再让浏览器合成的 `mouseup` 启动桌面选文轮询，长按 350ms 以上和真实已有
  选区仍走 touchend 延迟处理。
- `touchstart` 只为移动普通文本 page 准备正文 transform 图层；动画消费该准备，中心点击、
  选文、拖动、touchcancel、切章和卸载均释放并恢复原 `will-change`。其它格式和模式的准备
  函数直接返回，不常驻提升 EPUB、图片或超长连续正文。
- 移动合成路径从首段极慢的 power-cubic 改为 1.5 次幂对称 ease-in-out；它仍从/到零速度、
  保持设置总时长与精确终点，但在首个刷新周期建立明显位移。桌面和无 WAAPI 回退继续使用
  上游 cubic。
- 单元合同 25/25 通过；前端全量 487/487、Vite 生产构建和 Go 全量通过。
- `reader-text-modes-contract.mjs` 在 390×844、360×800 使用分离的真实 touchstart/touchend：
  按下阶段观测到预热，松手后 40ms 内位移至少达到页面步长 1%，普通点击选文检查不超过一次，
  输入至 300ms 动画窗口没有 ≥50ms Long Task，结束后 `will-change` 清空且落点仍为
  772px / 728px；连续点击、四文本模式和章末入口继续通过。
- `reader-mobile-contract.mjs` 的桌面/双移动全合同通过，真实 touch 长按可分别进入“添加过滤
  规则”和“添加书签”；`reader-continuous-contract.mjs` 通过，手指/滚轮仍原生连续。
- 修复提交 `a201c1e` 已推送 `main`，并随聚合发布提交 `06ac89b` 从本机完成 ARM64 预构建、
  历史 `data/cache/library` 卷和可移植备份/恢复门禁，再本地构建并上传 AMD64+ARM64 镜像。
  `ghcr.io/changshengyu/openreader:06ac89b` 与 `latest` 共同指向 OCI index
  `sha256:32f790021105003c1ce67aefd78b6416edb597cd441505aa3d7f888ba561bb77`；两个标签的远端
  manifest 均已核验。自动门禁不能代替用户在真实手机上的最终流畅度验收。

## 2026-07-19 第四次移动端复审

第三批在桌面无头 Chrome 中通过了输入延迟、Long Task 和轨迹合同，但用户在真实手机上仍能
感到点击分页“顿一顿”。本轮重新审查动画目标的尺寸和浏览器合成成本，不再只审查 JavaScript
主线程：

| 项目 | 固定上游 / 用户合同 | `06ac89b` 当前行为 | 第四次判定 |
|---|---|---|---|
| 动画目标尺寸 | 上游逐帧写根滚动位置，没有把整章正文永久或临时提升为单个 transform 图层。 | `contentBody` 指向整章 `.reader-body`；`touchstart` 写入 `will-change: transform`，松手后对整章执行 WAAPI。长章节高度可达数万至数十万像素。 | `must-fix`：禁止把整章正文作为移动点击分页的合成目标；预热不得触发超长图层栅格化。 |
| GPU / 栅格化停顿 | 上游没有独立的整章合成图层。用户要求手机点击分页稳定、连续，同时此前还报告 Chrome 偶发退出。 | Web Animations 与 `will-change` 可能触发大图层分块、纹理分配和重新栅格化；这些停顿发生在渲染/GPU 管线，`PerformanceObserver('longtask')` 不保证可见。 | `must-fix`：移除这条内存风险路径；将 Chrome 稳定性作为同一回归风险验证，不再把“无 Long Task”当充分条件。 |
| WAAPI 关键帧 | 上游每个刷新周期只计算一个位置。 | 当前预生成最多 61 个 transform 关键帧并交给浏览器插值；关键帧数量不是主因，但会增加启动阶段样式和动画对象工作。 | `must-fix`：目标路径不再创建整章 transform 关键帧。 |
| 逐帧滚动回退 | 上游使用 `requestAnimationFrame + scrollTop`；第一批已让程序化分页期间的业务 scroll-sync 延迟到结束后一次结算。 | 回退路径仍可用，但使用旧 cubic 起步曲线；移动主路径被整章 WAAPI 覆盖，第一批减负后的轻量滚动路径没有成为实际手机主路径。 | `must-fix`：移动普通文本 page 回到时间驱动的轻量 `scrollTop` 动画，复用第一批的 scroll-sync 抑制，并使用第三批已验收的快速起步/平滑收尾曲线。 |
| 原生手势与设置时长 | 手指/滚轮必须原生连续；点击分页离散；`0/100/300/500ms` 必须立即生效。 | 手势、分页步长和设置合同已有测试。 | `aligned`：切换动画承载方式时不得改变这些语义。 |

### 第四批先失败测试

1. 移动 text/page 点击不得调用正文 `animate()`，不得写入整章 `will-change: transform`；动画中
   只按刷新周期更新滚动容器 `scrollTop`，结束后仍只 flush 一次业务同步。
2. 移动逐帧路径使用快速起步/平滑收尾曲线；`0ms` 同步，`100/300/500ms` 按设置结束，终点
   保持一个页面步长，取消时保留当前已显示位置。
3. 同方向快速点击仍最多缓冲一次；反向点击、真实 touchmove、wheel、切章和卸载取消动画并
   清空缓冲，不产生旧完成回调。
4. 390×844、360×800 真实触控合同改为断言：动画期间 `scrollTop` 连续变化、正文不存在
   WAAPI/`will-change`、没有超过两个刷新周期的停帧、结束点精确；同时保留原生连续滚动、
   长按选文、工具层和章末入口回归。
5. 增加超长文本 fixture；重复分页不得持续增加 Animation 对象或遗留正文合成提示，并记录
   `pageerror`、浏览器断连和 renderer 崩溃作为硬失败。

### 实施边界

- 不修改后端、数据库、阅读进度、分页距离、设置范围、章节边界和用户要求的原生连续手势。
- 不恢复第一批已经移除的逐帧章节判定、布局重算和进度组装；程序动画仍只在结束后统一结算。
- 第三批的选文单次检查和有界连续点击继续保留；仅替换造成真实设备压力的整章合成承载。
- 若未来采用 View Transition 或可视窗口快照，必须先证明快照仅覆盖视口且在目标移动浏览器
  稳定；本批不以未经设备验证的新大图层机制替换旧大图层机制。

### 第四批实施与发布前验证结果

- 删除整章 `visualElement.animate()`、transform 关键帧和 `will-change` 预热生命周期；普通
  `touchstart` 不再触发正文图层提升，也不再为中心点击/选文/拖动做无用合成准备。
- 移动普通文本 page 点击改用时间驱动的 `requestAnimationFrame + scrollTop`，并继续复用第一批
  “动画期间只标记、结束后单次 flush”的滚动同步机制；移动路径使用第三批已接受的 1.5 次幂
  对称曲线，桌面/其它路径仍使用原 cubic。
- 0ms 同步定位，100/300/500ms 总时长、一个视口步长、取消保留当前位置和同向一次缓冲均由
  单元合同覆盖；动画目标即使暴露 `animate()` 也不得被调用。
- 前端全量 489/489、Vite 生产 build 和 Go 全量通过。360 段长章节在 390×844、360×800 的
  分离 touchstart/touchend 合同中没有正文 Animation/transform/`will-change`，运动轨迹单调、
  前 40ms 可见、无超过两个刷新周期停帧并精确落点；desktop/mobile、长按选文、工具层、
  continuous 原生手势和章末入口继续通过。
- 修复随提交 `32dc6161e4fe559b21855b4b9f963b538098313a` 推送 `main`；本地 ARM64 镜像通过历史
  TXT/EPUB/UMD/CBZ、相对缓存、owner 隔离、挂载卷和 portable backup/restore 门禁。
- 本机发布的 `ghcr.io/changshengyu/openreader:32dc616` 与 `latest` 同指 OCI index
  `sha256:e5db5dd67e9dafc93803230ec2dba9c4ce09dc39632fcec3d9882b47a6ae781d`，包含已核验的
  linux/amd64 与 linux/arm64 manifest。自动轨迹不能代替用户在真实手机上的最终体感复验。

## 2026-07-19 第五次移动端实机复审

用户在 `32dc616` 实机复验后仍报告滑动阅读模式的点击翻页有轻微停顿。本轮把“轨迹中没有
Long Task”进一步拆成触摸松手首帧、连续点击页间衔接和三种竖向文本模式的一致性合同：

| 项目 | 固定上游 / 用户合同 | `32dc616` 当前行为 | 第五次判定 |
|---|---|---|---|
| 松手到首个可见位移 | 上游在 `touchend` 直接调用点击动作；用户要求点击分页立即响应。 | OpenReader 同样不等待合成 `click`，但动画以事件时刻作为起点，首个 rAF 时间戳可能不晚于该起点；进度会被钳制为 0。移动快速曲线又是起点斜率为 0 的对称 1.5 次幂曲线，首帧可能写回原位置。 | `must-fix`：移动竖向文本点击分页使用非零起步速度、平滑收尾曲线，并在启动事务内种下一个极小但单调的可见位移；第一帧不得写回起点。 |
| 同方向连续点击 | 上游 `transforming` 期间忽略第二次点击；OpenReader 允许有界缓冲一次是用户体验增强。 | 第一页完成回调先执行 `onVerticalPageSettled()`，同步扫描当前章节、更新布局与进度，再以 microtask 启动缓冲页；页间存在一次不必要的主线程结算。 | `must-fix`：同方向缓冲存在时先无缝启动下一页，整段动画链只在最终页结束后结算一次。反向输入、手指拖动、滚轮、切章和卸载仍取消整条链。 |
| 三种竖向文本模式 | 手指/滚轮在 `page`、`scroll`、`scroll2` 中保持用户要求的原生连续；点击/键盘仍按一个视口离散翻页。 | 快速曲线只在移动普通文本 `page` 生效；`scroll`、`scroll2` 点击仍使用起步更慢的 cubic。 | `must-fix`：响应型曲线覆盖移动普通文本的全部竖向模式；不接管原生 touch/wheel，也不用于 EPUB、音频或图片漫画。 |
| 设置时长与终点 | `0/100/300/500ms` 必须有真实差异，0ms 立即跳转，正数精确落在一个页面步长。 | 设置值、步长和终点合同已对齐。 | `aligned`：优化曲线和结算时机不得缩短配置总时长或改变落点。 |

### 第五批先失败测试与实施边界

1. 时间戳等于或早于启动时刻的首个 rAF 也必须得到大于起点的单调位置；非零动画总时长和
   最终落点保持精确，0ms 不引入预位移。
2. 两次同方向点击只运行两段有界动画，第一页结束不得调用结算，第二页结束只结算一次；
   单次点击和反向/原生输入取消仍各自正确结算或作废。
3. 390×844、360×800 分别验证 `page`、`scroll`、`scroll2`：touchend 后首个可观察帧已移动、
   轨迹单调、页间没有结算停帧、没有整章 Animation/transform/will-change，最终位置精确。
4. 本轮只改前端动画调度与对应合同，不修改后端、数据、分页距离、章节窗口、选文、工具层、
   章末入口以及手指/滚轮的原生连续行为。响应型曲线属于本次用户明确要求的
   `intentional-redesign`，不声称是上游 cubic 的逐值复制。

### 第五批实施与发布前验证结果

- 移动普通文本全部竖向模式改用非零起步、平滑收尾的 1.5 次幂 ease-out；动画启动时在
  `running` 保护内写入 1ms 进度种子，首个 rAF 时间戳等于或早于启动时刻也不会退回起点。
  桌面、EPUB、音频、图片漫画以及非响应型调用仍保留 cubic。
- 同方向重复点击仍只缓冲一次；第一页结束直接在同一任务的 microtask 阶段启动第二页，
  不在两页之间扫描章节或提交进度。最终页结束后只执行一次 scroll-sync 结算。
- 新单元合同先在旧实现上得到 2 个确定性失败，实施后相关 26/26、前端全量 489/489、Vite
  生产构建和 Go 全量通过。
- `reader-text-modes-contract.mjs` 现对 390×844、360×800 的 `page`、`scroll`、`scroll2`
  分别采样：touchend 后首个可观察帧大于起点，运动单调、无超过 50ms 停帧、无整章
  Animation/transform/will-change，单次和缓冲双页精确落点。移动工具层、连续跨章、原生
  touch/wheel、章末入口和图片阅读真实 Chrome 合同同时通过。
- 自动轨迹已经覆盖此前遗漏的代码路径，但最终体感仍以用户真实手机安装本批 Docker 后的
  复验为准。

### 第五批 Docker 发布

- 实现提交 `895d53627a108b05a022f933ad678a5c2d7a72ec` 已推送 `main`。本地 ARM64 候选通过
  历史 `data/cache/library` 挂载卷、重启、TXT/EPUB/UMD/CBZ、相对缓存、用户隔离和
  portable backup/restore 门禁。
- 本机生成并上传 `ghcr.io/changshengyu/openreader:895d536` 与 `latest`；两标签共同指向
  OCI index `sha256:155fd51087400d0831ee840661e6391a8bfd70ba2b908858057c3c05397d2804`。
  linux/amd64 manifest 为
  `sha256:306367b6fcadeda3814d6c0e9feaff9999cfaedcc87252a97bc9b9195303b952`，linux/arm64
  manifest 为 `sha256:f6320c32c0cfad36b943c37d5c6a5bb08b5185b946aad5fd963cb6c4bbb3f0db`；两个远端
  标签的平台清单均已核验。
- 本批允许差异只有用户要求的移动点击响应型 ease-out、原生手指/滚轮连续滚动和既有
  数值 stepper；BookGroup 等后续全量上游复审仍未完成，不属于本镜像完成范围。

## 2026-07-19 第六次移动端实机复审

用户在 `895d536` 实机复验后仍报告滑动阅读模式点击翻页有轻微顿挫。本轮不再只比较
`scrollTop` 采样间隔，而是用 Chrome tracing 在 4 倍 CPU 降速下展开每一个
`FireAnimationFrame` 的内部工作。

| 项目 | 固定上游 / 用户合同 | `895d536` 当前行为与轨迹 | 第六次判定 |
|---|---|---|---|
| 最终视觉帧 | 上游 `Animate` 的每个 rAF 只写滚动位置；`scrollContent.onEnd` 解除动画状态，并用 timer 延迟保存进度，不在最终绘制回调中扫描章节或重算布局。 | `readerAnimation.draw()` 在写入最终 `scrollTop` 后同步调用 `onFinish`；`useReaderNavigation` 立即执行 `flushReaderScrollSync()`，串行运行当前章节判定、章节窗口扩展、布局、可见进度和保存调度。4× CPU 轨迹中普通运动帧约 `0.1–0.3ms`，最终 rAF 约 `35ms`。 | `must-fix`：最终位置必须先提交给浏览器完成一次绘制，业务结算再从后续任务运行；不能只从函数命名上声称“动画完成后”已足够。 |
| 缓冲双页 | 用户允许同方向最多缓冲一次，页间不能执行重型结算。 | 第五批已把第一页结算移除，但第二页最终 rAF 仍同步承担全部结算。 | `must-fix`：单页和缓冲链都只在最终视觉帧提交后结算；页间不得新增静止帧或重活。 |
| 动画活动期 | 原生手指/滚轮可以打断程序动画；打断后由原生 scroll 同步当前位置。 | animator 在 rAF 结束前是 active，但若把完成回调简单异步化，必须同时保证待结算窗口内仍被视为 active，且 cancel 能撤销旧回调。 | `must-fix`：延后完成回调时保留 generation/cancel 语义，避免旧结算覆盖新手势或新一页。 |
| 页面亮度滤镜 | 当前亮度是用户需要保留的设置；不得凭猜测删除。上游固定基准没有同一滚动祖先滤镜。 | 对同一 720 段移动正文分别保留 `brightness(0.87)` 和运行时关闭滤镜；两者移动帧间隔、Paint 和 RasterTask 基本同量级，最长 rAF 都来自 `readerAnimation.draw()`。 | `ruled-out for this bug`：滤镜仍列为长期移动渲染风险，但本轮不以无证据的视觉变化替代已定位的调度修复。 |

### 第六批先失败测试与实施边界

1. 正数点击分页到达目标位置时，完成/结算回调不得在写最终位置的 rAF 中执行；浏览器获得一次
   呈现机会后才运行回调。`0ms` 仍同步跳转并立即结算。
2. 延后窗口内 animator 仍为 active；新同向点击只进入既有一次缓冲，touchmove、wheel、切章
   和卸载必须能取消待执行完成回调，旧 generation 不得结算。
3. 4× CPU 真实 Chrome 轨迹必须区分运动 rAF 和结算任务：运动帧不得包含章节/布局结算，最终
   `scrollTop` 已可见后才出现业务任务；390×844、360×800 的 `page/scroll/scroll2` 均验证。
4. 保持 0/100/300/500ms、一个视口步长、响应型曲线、原生连续手势、选文、工具层、章末入口
   与连续跨章合同。本批不修改后端、数据、亮度视觉效果或 BookGroup 审查内容。

### 第六批实施与发布前验证结果

- 移动普通文本竖向点击的正数动画在最终 rAF 中只提交目标 `scrollTop`；章节、窗口、布局和
  进度结算改由后续可取消任务接手。交接任务执行前 animator 仍保持 active，所以同向输入仍
  只缓冲一次，touchmove、wheel、切章和卸载仍可取消旧 generation；`0ms` 保持同步跳转。
- 新增三个确定性单元合同并先在旧实现上得到 3 个失败：最终帧不得结算、交接期可取消、
  `0ms` 不异步化。实施后相关 24/24、前端全量 492/492、生产构建和 Go 全量通过。
- 同一 720 段正文、390×844、4× CPU Chrome tracing 中，最长动画 rAF 从修复前约 `35ms`
  降到 `0.88ms`，`readerAnimation.draw()` 最大约 `0.33ms`；17 个运动采样的最大帧间隔约
  `17.5ms`，终点仍为 `772px`。亮度 `brightness(0.87)` 保持开启。
- `reader-text-modes-contract.mjs` 已通过 1440×900、390×844、360×800，并在两个移动尺寸
  覆盖 `page/scroll/scroll2` 的时长、轨迹、连续双击与章末入口；
  `reader-continuous-contract.mjs` 保持原生 touch/wheel 和跨章；完整 desktop/mobile 合同继续
  通过工具层、面板、选文、当前段落书签及数值设置交互。
- 本批没有后端、数据库、进度格式、亮度视觉、分页距离或章节边界变化。自动轨迹已直接覆盖
  本次定位到的最终帧瓶颈，但真实手机体感仍以新 Docker 的用户复验为最终依据。

### 第六批 Docker 发布

- 实现提交 `a92f6a13f5d9a1be065f4d2ee1d804e37f09ea83` 已推送 `main`。镜像在本机完成
  linux/amd64 与 linux/arm64 构建，再由本机 OCI 发布器上传；未使用云端构建。
- `ghcr.io/changshengyu/openreader:a92f6a1` 与 `latest` 共同指向 OCI index
  `sha256:704563d2f5d4fe8224977cd0e3d04f79cb6296b44f1300ddcfa30eebecf8c8b3`；
  amd64 manifest 为
  `sha256:4e7a202adf7f560339888e7c6231d70ccd23997f65ab91f91e9e70789b1064b4`，
  arm64 manifest 为
  `sha256:b46a47be22d821470f0868f10225f2d60c64aaa3de7ba7e9df87ce27e19be8ba`。
  两个远端标签已分别检查并确认平台清单一致。
- Docker daemon 首次直接从 GHCR 拉取时仍在 manifest `HEAD` 遇到 `502 Bad Gateway`；随后用
  发布时相同 `BUILD_DATE` 从本机缓存加载，输出的 arm64 manifest 与远端
  `b46a47be22d8…` 完全一致。该确切镜像通过历史 TXT/EPUB/UMD/CBZ、相对缓存、重启、
  owner 隔离和 portable backup/restore 门禁。
- 允许差异仍只有用户要求的响应型移动点击曲线、原生手指/滚轮连续滚动和数值 stepper。
  BookGroup 全量复审在本批紧急修复期间暂停，发布后继续，不属于此镜像完成范围。
