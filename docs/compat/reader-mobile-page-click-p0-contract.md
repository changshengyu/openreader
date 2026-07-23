# Reader 移动端上下滑动点击翻页与章末入口合同（P0）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-23 第十批用户实测后再次打开。第十批已完成全量门禁和 Docker 发布，但用户继续
观察到移动端竖向模式点击上下翻页有明显卡顿，并指出也可能是文字变化造成的视觉顿挫。第五批
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

## 2026-07-22 第七次移动端连续点击复审

用户在当前镜像继续实测后报告：移动端竖向滑动阅读中连续点击翻页，偶尔会先停顿一下才继续
移动。本轮重新审查第六批为保护最终绘制而引入的 `after-paint` 交接，确认单页终点与重型结算
已经分离，但“是否继续缓冲的下一页”也被错误地放进同一个后续 task，因而在两段视觉动画
之间主动留下了一个终点停驻窗口。移动 Safari 在主线程繁忙时会放大这个 task 等待时间。

| 项目 | 固定上游 / 用户合同 | 当前实现 | 第七次判定 |
|---|---|---|---|
| 单页最终绘制 | 最终滚动位置先交给浏览器，章节/布局/进度结算随后执行。 | `finish: after-paint` 已把重型 `onVerticalPageSettled()` 移到可取消 task。 | `aligned`；不得把重型结算移回最终 rAF。 |
| 缓冲页视觉衔接 | 上游会拒绝重叠输入；OpenReader 的同向一次缓冲是用户要求的有界增强。已接受的增强应连续呈现，页间不得等待业务 task。 | animator 到达第一页终点后仍保持 active，等 `setTimeout(0)` 执行 `onFinish`；导航直到该 task 内才发现 `queuedVerticalDirection`，再以 microtask 启动第二页。 | **must-fix**：把“视觉段完成/是否续页”与“最终业务结算”拆成两个回调。第一页最终 rAF 内只允许无重活地启动已缓冲下一段；整条链最后一段才进入 after-paint 结算。 |
| 输入有界性 | 连续点击不能无限排队，反向输入和原生手势应取消待执行同向动作。 | 最多保存一个同方向值；更多点击折叠，反向点击清空，touchmove/wheel/切章/卸载取消 generation。 | `aligned`；保留一次缓冲，不以无界队列掩盖衔接问题。 |
| 三种竖向文本模式 | 用户反馈的是移动滑动阅读；`page`、`scroll`、`scroll2` 的点击翻页应共享同一轻量轨迹。 | 三种模式均走 responsive animator，因此同受 after-paint 页间窗口影响。 | **must-fix together**；EPUB、音频、图片、flip、桌面及原生手指/滚轮不进入此改动。 |

### 第七批先失败测试与实施边界

1. animator 增加独立视觉完成钩子：第一页最终 rAF 写入精确终点后，钩子可在同一渲染机会内
   启动下一段；只要下一段已启动，前一段不得创建 after-paint 完成 task。
2. 两次同方向点击必须在第一页最终 rAF 内把第二页推进到非零种子位置，不能先绘制一个静止
   终点再等待 task；第二页最终位置提交后才创建一个可取消结算 task，整链只结算一次。
3. 单次点击仍在最终位置后异步结算；`0ms` 保持同步；反向输入、touchmove、wheel、切章和卸载
   仍清空缓冲并使旧视觉/结算回调失效。
4. 真实 390×844、360×800 的 `page/scroll/scroll2` 采样快速双击：第一页终点附近不得出现超过
   一个刷新周期的静止平台，第二段必须单调继续，最终精确到两个视口步长；同时保留单击时长、
   原生手指/滚轮、选文、工具层、章末入口和跨章合同。
5. 本批不修改后端、数据、动画时长范围、分页步长、响应型曲线或已完成的 iPad 面板修复。

### 第七批实现与验证证据

- `createReaderScrollAnimator()` 现在把最终视觉帧与业务结算明确分开：`onVisualFinish`
  在精确终点写入后的同一 rAF 内检查一次有界缓冲；成功续页时，下一段立即写入非零运动种子，
  前一段不再创建 `after-paint` task。只有整条视觉链最后一段才异步结算。
- 若第二次点击恰好落在最终 rAF 与结算 task 之间，`takeOverPendingFinish()` 会取消旧结算并在
  当前输入 task 内开始下一段；因此不再依赖浏览器何时调度 `setTimeout(0)`，旧结算也不会重复执行。
- `useReaderNavigation()` 仍只缓冲一次同方向输入；反向点击、原生手势、切章和卸载的取消语义
  未改变。改动只作用于响应式移动端竖向文本点击翻页。
- 两类回归均先以失败测试锁定：第一终点不得产生中间结算 task；结算交接窗口内的新点击必须
  立即接管。实现后前端全量 `509/509`、Go 全量测试和 Vite 生产构建通过。
- 真实 Chromium 合同已在 `390×844` 与 `360×800` 下覆盖 `page`、`scroll`、`scroll2`：
  快速双击到达第一页边界后的三个采样帧内必须进入第二页，轨迹保持单调，最终精确到两个
  视口步长；`reader text-mode contract smoke passed`。
- 本批随 `544e1fb` 从本机完成 linux/amd64、linux/arm64 构建并发布；`544e1fb` 与 `latest`
  均指向 OCI index `sha256:ad083dc1e996c62fbf88ee7367a4c08330912088bb144848886daa1fe2fb8966`。
  amd64 manifest 为 `sha256:84be279ea27d836a73647a0a4ea4af9423942ec7022a3c8e98974e01970a7f05`，
  arm64 manifest 为 `sha256:30891d3289e01ceae7f12d2b52961b3b81c20dfc57be5e2de55eb1753e88bd8a`。
  新卷、历史 TXT/EPUB/UMD/CBZ/相对缓存、用户隔离、便携备份恢复和候选容器 WebDAV 协议门禁通过。

## 2026-07-22 第八次移动点击视觉节奏复审

第七批只消除了连续两页之间的任务队列空窗，没有约束单页内每帧文字位移的速度变化。当前
`responsive` 曲线为 `1 - (1 - t)^1.5`，300ms/772px 下第一个约 16.7ms 刷新周期理论位移约
64px（接近两行默认正文），而启动事务还会先同步写入 1ms 种子位置。即使每帧都准时，这种
“瞬间大步移动、随后持续减速”的轨迹也会让文字看起来像先跳一下再滑动；旧 smoke 只验证单调、
无静止平台和最终落点，反而会把这种光学顿挫判为通过。

| 项目 | 固定上游 / 用户合同 | 当前实现 | 第八次判定 |
|---|---|---|---|
| 单页速度轮廓 | 上游 power-cubic ease-in-out 起止速度均为 0；用户要求点击后及时响应，但不接受明显跳步。 | 移动响应曲线初始斜率为 1.5、末端为 0；前 16.7ms 文字位移远高于后半程，先前为消除“起步等待”过度提高了初速。 | **must-fix**：使用有界非零初速、连续加速度和零终速的移动曲线；不能恢复肉眼停顿，也不能首帧跳两行。 |
| 文字重排与布局位移 | 正常翻页只改变滚动位置；字体、行高、段落宽度和 DOM 顺序在一段动画内不变。 | 旧测试只采样 `scrollTop`，没有观测 `LayoutShift`、段落几何或最终结算前后的可见文字锚点。 | **must-verify**：动画期间 layout-shift 为 0；相同段落尺寸不变。若位置连续而文字锚点跳变，才允许修改章节窗口/锚点逻辑。 |
| 每帧主线程成本 | 上游 rAF 只写滚动位置；当前也已把业务结算移到 after-paint。 | Chromium 旧轨迹证明最终 rAF 已变轻，但没有给每帧位移/速度/加速度设上限，不能区分掉帧与轨迹本身突兀。 | **invalid old evidence**：同时记录 rAF 间隔和视觉速度；两者都合格才算丝滑。 |
| 时长设置 | `0ms` 立即定位；正数持续时间应有真实差异。 | `100/300/500ms` 已按设置结束。 | **preserve**：不以固定浏览器 smooth 或缩短时长掩盖节奏问题；100ms 天然帧数较少，但仍须按同一有界曲线。 |

### 第八批先失败测试与边界

1. 动画纯函数测试必须断言：非零起步、首个 16.7ms 位移不超过页面步长的 4%、位置单调、速度
   连续、结尾速度趋零、最终精确为 1；`0ms` 不引入种子位移。
2. 390×844、360×800 的 `page/scroll/scroll2` 真实触控采样同时记录 rAF、`scrollTop`、顶部可见
   段落和 `PerformanceObserver(layout-shift)`；不得出现超过两个刷新周期的停帧、首帧大跳、
   DOM 重排或结算后的可见锚点回跳。
3. 单击、同向双击和反向/手势取消分别验证；双击仍最多缓冲一次，页间无业务结算。
4. 不修改分页距离、设置值、原生手指/滚轮、章节边界、选文、工具层、EPUB/图片/音频/flip、
   后端或持久数据。只有测量证明亮度滤镜或章节窗口导致额外卡顿时才进入对应改动。

### 第八批实现与发布前验证结果

- 移动普通文本竖向点击的 `responsive` 曲线改为三次 Hermite 轨迹：归一化初速为 `0.35`、
  末速为 `0`，位置和速度在整段内连续。300ms 时启动种子仍不超过 2px，首个约 16ms
  刷新周期被限制在约 6–24px 的合同范围，不再沿用旧 ease-out 理论约 64px 的文字跳步。
- 分页距离、`0/100/300/500ms`、after-paint 最终结算和同方向一次有界缓冲均保持不变；
  `page`、`scroll`、`scroll2` 共用新轨迹，原生手指/滚轮仍不进入程序动画。
- `reader-text-modes-contract.mjs` 在 390×844、360×800 对三个竖向模式分别验证向下和向上
  点击：首刷新区间不超过页面步长 4%，运动单调、帧间隔不超过 50ms、终点精确；同向双击
  仍连续到两页距离。输入窗口内没有 Long Task、LayoutShift、整章 transform/Animation 或
  段落宽高变化，因此本次证据指向旧速度曲线造成的视觉顿挫，而不是正文重排。
- 针对性 28 项测试、前端全量 537/537、Vite 生产构建和 Go 全量通过。真实 Chromium Reader
  合同通过；最终体感仍等待用户在真实手机安装本批 Docker 后复验。

### 第八批 Docker 发布

- 实现提交 `a54bd725991bc19535fb8263ab4a17efbc7d7f87` 已推送 `main`。本机 ARM64 候选通过
  新卷和历史 TXT/EPUB/UMD/CBZ/相对缓存、重启、用户隔离及便携备份恢复门禁；历史卷首次
  资源读取出现一次 404，随后完整重跑全链通过，未跳过失败步骤。
- 镜像在本机完成 linux/amd64、linux/arm64 构建并由本机 OCI 发布器上传，未使用云端构建。
  `ghcr.io/changshengyu/openreader:a54bd72` 与 `latest` 共同指向 OCI index
  `sha256:5caae8c4277459431c9265e159e85702d8b1433e11d4083af8fb413d0aeedb96`；amd64 manifest 为
  `sha256:b20613aa0c6a5994c4e622cf9aaf66d77d862e1f4988bd30a07c91c1ea35107a`，arm64 manifest 为
  `sha256:7d0ab38ed496347e37fcd4646505a9a24d38a2f15fb201a2139b80df48a29ba5`。两个远端标签的平台
  清单和 index digest 已分别核验一致。
- 允许差异仍是用户要求的原生手指/滚轮连续滚动、移动点击响应型曲线和数值 stepper；本镜像
  同时包含此前已验证但尚未发布的删除消费者收敛与本批书架刷新 pending/CAS 修复。全量重构
  的其它模块不因本次镜像发布而视为完成。

## 2026-07-22 第九次移动点击固定上游复审

用户在 `a54bd72` 实机复验后仍报告：移动端滑动阅读中点击上/下翻页看起来有明显卡顿，并明确
要求恢复上游那种滑动体感。本轮重新逐行核对固定基准
`Reader.vue::nextPage/prevPage/scrollContent/getCurrentParagraph/scrollHandler`，不再把此前自创的
响应型曲线和连续点击缓冲视为保留理由。

| 项目 | 固定上游合同 | `a54bd72` 当前实现 | 第九次判定 |
|---|---|---|---|
| 点击动画轨迹 | `scrollContent()` 使用 `makeEaseInOut(power(3))`；起止速度均为 0，动画中每个 rAF 只写 `scrollTop`。 | 移动文本改成初速 `0.35`、末速 `0` 的 Hermite 曲线，并同步写入 1ms 种子。 | **must-fix**：按上游恢复三次幂 ease-in-out，移除移动专用种子和响应型曲线。 |
| 重叠点击 | `nextPage()` / `prevPage()` 在 `transforming` 时立即返回，不缓冲第二次输入。 | 同方向点击会缓冲一次，并允许在最终 rAF 或 after-paint 交接期接管下一段。 | **must-fix**：恢复动画期间拒绝重叠输入，避免页间状态机和额外任务影响节奏。 |
| 动画结束与进度 | `onEnd` 精确写入终点、清除 `transforming`，再用 `setTimeout(saveReadingPosition, duration)` 延迟保存。 | 最终绘制后用 `setTimeout(0)` 立即运行章节识别、窗口扩展、布局与本地进度结算。 | **must-fix**：视觉动画结束先释放输入；进度/章节结算按上游延后并可取消，不能紧贴最终帧阻塞下一次触控。 |
| 可见段落查找 | `getCurrentParagraph()` 按 DOM 顺序读取 `h3,p` 矩形，命中第一个越过可见顶线的元素后立即 `break`。 | `currentProgressElement()` / `currentVisibleParagraph()` 先测量整章或连续窗口内全部节点，再筛选。一次结算还可能重复捕获快照。 | **must-fix**：恢复命中即停的有界查找，并让同一稳定位置复用一次快照。 |
| 滚动宿主 | 上游写 `documentElement/body.scrollTop`；OpenReader 因固定工具层和 Vue 3 场景使用 `.reader-content` 独立滚动容器。 | 点击和原生滚动都作用于 `.reader-content`。 | `technical-stack-equivalent`：保留滚动宿主，但动画函数的曲线、互斥、逐帧职责和结算顺序必须对齐。 |
| 原生滑动 | 上游移动端可原生纵向滚动。 | 手指/滚轮原生连续滚动，不使用点击动画时长。 | `user-requested acceptable-change`：继续保留。 |

### 第九批先失败测试与实施边界

1. `100/300/500ms` 点击轨迹必须逐值符合上游三次幂 ease-in-out；正数动画无同步种子，`0ms`
   仍立即到达终点。
2. 动画活动期间的同向和反向点击均被拒绝，不产生缓冲段；动画结束后新点击可立即启动。
3. 最终视觉帧只写目标 `scrollTop` 并释放动画互斥；章节识别、布局和进度结算延后，不得在最终
   帧或紧随其后的首个输入窗口同步执行。
4. 长章节夹具记录段落几何读取数：可见节点命中后停止；一次稳定结算只捕获一次可见快照。
5. 390×844、360×800 的 `page/scroll/scroll2` 同时验证向上/向下、动画时长、重叠点击拒绝、
   动画后下一次触控延迟和段落锚点稳定。
6. 不修改分页距离、原生手指/滚轮、章节扩展、进度数据格式、选文、工具层、章末入口、
   EPUB/图片/音频/flip、后端或持久数据。

### 第九批实现与发布前验证结果

- 移动竖向点击不再走专用 Hermite 分支；`createReaderScrollAnimator()` 现在统一使用固定上游的
  power-cubic ease-in-out，正数动画启动前不再同步改写 `scrollTop`，`0ms` 仍精确立即定位。
- `useReaderNavigation()` 恢复上游 `transforming` 互斥：动画中的同向和反向点击都直接拒绝，
  不再缓存或接管下一段。最终视觉帧释放互斥后，新点击立即可用。
- 章节、布局和进度结算在视觉动画结束后继续延迟一个配置时长；新点击或原生手势会取消旧结算。
  滚动同步在延迟期保持抑制，避免浏览器最终 scroll 事件提前触发重活。
- 可见段落查找恢复上游“命中即停”；720 段夹具不再无条件测量全部 720 个节点。同一稳定滚动
  结算只捕获一次可见快照，并复用于章节状态和本地进度；flip 的横向换列查找保留完整语义。
- 先失败合同在旧实现上稳定得到 4 个失败。实施后前端全量 `532/532`、生产构建和 Go 全量通过。
  真实 Chromium 已通过 1440×900、390×844、360×800；移动两个尺寸分别覆盖
  `page/scroll/scroll2` 的向上/向下 cubic 轨迹、重叠点击拒绝、动画后新点击、延迟结算和段落
  读取上限。移动工具层/iPad 面板与连续跨章/原生触控合同也保持通过。

### 第九批 Docker 发布

- 实现提交 `fe32c006132ac1ffbe3946a6a98482cb0dcca5e4` 已推送 `main`；镜像在本机完成
  linux/amd64、linux/arm64 构建并由本机 OCI 发布器上传，未使用云端构建。
- `ghcr.io/changshengyu/openreader:fe32c00` 与 `latest` 共同指向 OCI index
  `sha256:09394c2183212b130db1eb8150952ca6ce688c5baf9ea8d6d47136f93db90617`；amd64 manifest 为
  `sha256:a3205b085bda3faf78426098d4aa3fba3b70e70bb2e9ea73f021850110c5c77e`，arm64 manifest 为
  `sha256:74048c04ca9e581439eb2255c74aca9b26f43b149391e75a3a0bcfb8562cf27c`。两个远端标签的平台
  清单已分别核验一致。
- 本地候选和从 GHCR 拉回的确切 arm64 发布镜像均通过新旧 `data/cache/library`、重启、
  TXT/EPUB/UMD/CBZ、相对缓存、用户隔离和 portable backup/restore 门禁。
- 本批允许差异仍只有用户明确要求的原生手指/滚轮连续滚动、数值 stepper，以及独立
  `.reader-content` 滚动宿主这一 Vue 3 工程适配；点击动画语义已回到固定上游。其它全量重构
  模块不因本镜像发布而视为完成。

## 2026-07-22 第十次移动点击启动与渲染层复审

用户对 `fe32c00` 的真实设备验收仍报告点击滑动卡顿，因此第九批“点击动画语义已回到固定
上游”的结论无效。本轮直接比较固定提交的 `plugins/animate.js`、`Reader.vue#eventHandler /
nextPage / prevPage / scrollContent` 与当前 `useReaderPointer.js`、`useReaderNavigation.js`、
`readerAnimation.js` 和最终 CSS，不再把相同 easing 函数视为整条动画链已经相同。

| 项目 | 固定上游合同 | `fe32c00` 当前实现 | 第十次判定 |
|---|---|---|---|
| 动画计时源 | `Animate` 在 rAF 回调体内执行 `Date.now() - start`；不使用 rAF 形参。 | `draw(timestamp)` 以浏览器传入的帧时间戳减去触摸处理期间读取的 `performance.now()`。当事件发生在帧中段，首回调时间戳可早于起点并被钳制到 `0`。 | **must-fix**：每帧以回调实际执行时钟计算 elapsed，精确复刻上游；测试必须注入“旧帧时间戳 + 新执行时钟”。 |
| 首帧合同 | 上游从首个可执行动画回调开始计算已过时间；power-cubic 的慢启动来自曲线本身，不能再叠加一帧人为零进度。 | 390×844 生产构建探针记录：触摸结束后约 `15ms` 的第一采样仍为 `scrollTop=0`，约 `32ms` 才出现位移。旧 smoke 只断言首 8–24ms 不超过 4%，零位移也会通过。 | **must-fix test gap**：增加首回调使用执行时钟的确定性单元合同；真实触控记录 animator 首次写入/首个非零视觉位置，不能仅设上限。 |
| 滚动渲染层 | 上游写根 `documentElement/body.scrollTop`，正文祖先没有 `filter`。主题由背景资源/颜色实现。 | 独立 `.reader-content` 是允许的 Vue 3 滚动宿主，但它的父级 `.reader-page` 始终应用 `filter:brightness(...)`，包括 100%。 | **must-fix**：保留用户要求的亮度值，但改成不包裹滚动内容的黑色无事件遮罩；滚动祖先计算样式必须为 `filter:none`。独立滚动宿主暂保留为技术栈适配，不在同一批大改根滚动。 |
| 点击入口 | 上游 touchend 在无选择且位移不超过 3px 时直接调用 `eventHandler`，翻页前隐藏工具层。 | 当前 touchend 以 12px 容差判定 tap，普通点击检查一次选择后调用翻页并隐藏工具层。 | `acceptable-change / preserve`：12px 是移动触控容错，不是本轮已测出的动画停顿来源；选择、工具层和面板合同不得回归。 |
| 结算与互斥 | 上游动画期间拒绝输入；onEnd 精确落点并释放 `transforming`，保存延后。 | 当前已拒绝重叠点击、精确落点并把业务结算延后；新点击会取消旧结算。 | `aligned / preserve`：本轮不恢复缓冲、不把章节/进度重活放回最终帧。 |

### 第十批先失败测试与实施边界

1. animator 注入 rAF 参数 `100`、执行时钟 `116`、起点 `100` 时，首回调必须按 `16ms` 的
   cubic 进度写入，而不是因为 rAF 参数陈旧而写回起点；持续时间和最终落点仍以执行时钟为准。
2. 390×844 与 360×800 的 `page/scroll/scroll2` 真实触控合同同时记录 touchend、首个动画写入、
   rAF 采样和 scroll 位置；测试必须拒绝“首动画回调人为 0%”，并保留 100/300/500ms 差异。
3. `.reader-page` 和 `.reader-content` 的祖先链不得有 CSS filter；亮度 100 时遮罩透明，87 时
   遮罩 alpha 为 0.13，遮罩不接管 pointer/touch，不改变正文宽度、文字几何或工具层层级。
4. 三次幂 ease-in-out、一个视口减两行/段距的步长、动画互斥、延迟结算、原生手指/滚轮、
   选文、工具层、章末入口、EPUB/图片/音频/flip 和持久数据都不在本批改变范围。
5. 只有上述失败合同转绿并通过真实浏览器后，才能再次把设备状态写为待验收并考虑发布 Docker。

### 第十批实施与发布前验证结果

- `createReaderScrollAnimator()` 不再使用 rAF 传入的时间戳，而是像固定上游
  `plugins/animate.js` 一样，在每个回调实际执行时读取时钟。注入“旧帧时间戳 + 16ms 回调
  执行时钟”的先失败单元合同已转绿，首回调不再人为重写起点。
- `.reader-page` 上的 `filter:brightness(...)` 已移除。亮度改为等价黑色伪元素遮罩：
  100% 时透明，87% 时 alpha 为 0.13，`pointer-events:none`；因此滚动正文不再处于 CSS
  过滤/合成边界中，同时不会截获点击、触摸、章末按钮或工具层。
- 前端全量 `534/534`、Go 全量、Vite 生产构建通过。真实浏览器已覆盖
  1440×900、390×844、360×800 和 iPad 自适应；`page/scroll/scroll2` 分别覆盖
  100/300/500ms、首次 animator 写入、滚动祖先样式、向上/向下、连续跨章、图片与
  真实 Go 后端 EPUB 流程，全部通过。
- 音频扩展 smoke 在切章后的测试桩绑定了已替换的旧 `<audio>` 实例，因此卡在
  测试自身的 `playCalls` 等待；它未命中本批动画或亮度遮罩的产品断言，不作为
  本批失败。音频测试桩可在后续独立合同中改为原型级或按新实例重新安装。
- 状态进入 **browser-validated / awaiting device verification**；用户真机确认之前不再声称
  体感问题已完全关闭。保留的差异仅是原生手指/滚轮连续滚动、数值 stepper 和
  Vue 3 的独立 `.reader-content` 滚动宿主。

### 第十批 Docker 发布

- 实现提交 `0a77632a4368d26f9774f77cfcd2b4b5c0d7149b` 已推送 `main`。本地 ARM64
  候选镜像通过新旧 `data/cache/library`、重启、TXT/EPUB/UMD/CBZ、相对缓存、用户隔离
  和 portable backup/restore 完整门禁。
- 镜像在本机完成 linux/amd64、linux/arm64 构建并上传 GHCR，未使用云端构建。
  `ghcr.io/changshengyu/openreader:0a77632` 与 `latest` 共同指向 OCI index
  `sha256:226f982cf31ef20d5d6c9c2e6e5c03c18da19bcc8a23db38acc3fcb43a177c11`；amd64 manifest 为
  `sha256:815e0d27623e9ce6f02bcd9bf3880916e24af8843476e45e6c5f11acbb030393`，arm64 manifest 为
  `sha256:70aa13fcaa9b6ebee1a27391003168df20ac25dbc4fb601645fbf139066ff609`。两个远程标签已
  分别核验且平台清单一致。
- 当前状态为 **Docker-published / awaiting device verification**。全量重构的其它模块不因
  本批发布而视为完成。

## 2026-07-23 第十一次固定上游全设置与滚动宿主复审

用户对 `0a77632` 的真实移动设备验收仍认为“上下滑动”点击翻页明显不如固定上游丝滑，因而
第十批把独立 `.reader-content` 滚动宿主列为 `technical-stack-equivalent` 的结论不再成立。
本轮先完整提取 `plugins/config.js`、`plugins/vuex.js`、`ReadSettings.vue`、`App.vue`、
`Reader.vue` 和 `Content.vue`，确认哪些设置会改变普通点击分页的有效状态、距离或渲染成本，
再决定实现；不能继续把问题缩小成单一 `animateMSTime` 参数。

### 关联设置与隐式状态矩阵

| 设置/状态 | 固定上游行为 | 当前行为 | 对点击体感的影响与判定 |
|---|---|---|---|
| 特殊模式 `pageType` | 默认“正常”；切到 Kindle 会恢复/生成一套简洁配置，默认包含 `animateMSTime:0`、`fontSize<=20`、纯白主题、左右滑动、忽略选文和手机模式；不会强制改 `clickMethod`。 | 默认 `normal`；`kindle` 同样强制 0ms/手机/左右滑动，但还强制 `clickMethod:none`，且恢复快照字段不完全等同上游。 | **直接影响**：0ms 会使竖向定位跳变；当前额外改变全屏点击是 `must-fix`。测试必须断言最终有效模式/时长，不能只读面板上的一个值。 |
| 配置方案 | 上游白天/黑夜方案都包含翻页方式、动画时长、页面模式、字体和段落参数；选择方案会整套覆盖当前配置。 | 方案包含大部分同类字段，但 `pageMode` 被标成仅本机且不进入方案快照/服务端 payload。 | **直接或间接影响**：方案切换可同时改变动画、模式和分页距离；`pageMode` 丢失是 `must-fix` 存储语义。 |
| 自动切换 | 默认 `autoTheme:true`；系统深浅色变化会选择对应默认方案，因此不只换颜色，也会应用该方案中的翻页和排版设置。 | 默认 `false`；开启后同样会调用 `setCustomConfig()` 整套应用方案。 | **隐式覆盖来源**：运行中系统主题变化可能改变翻页体验。默认值与上游不符，判为 `must-fix`；切换必须在动画边界安全生效。 |
| 页面模式 `pageMode` | “自适应/手机模式”决定 mini interface；它属于默认配置和方案同步字段。 | “自适应/手机模式（本机）”只决定响应布局，不同步。 | 不改变 easing，但改变滚动视口、正文宽度和触控结构；`must-fix` 方案/持久化语义。 |
| 翻页方式 `readMethod` | “上下滑动/左右滑动/上下滚动/上下滚动2”；普通竖向点击均走根页面 `scrollContent`，滚动2会移除已读章节并可能抖动。 | `page/flip/scroll/scroll2`；普通竖向点击共享内部元素 animator，连续模式另有 Vue 章节窗口。 | **直接影响结构**；`scroll2` 的章节窗口抖动提示不是普通 `page` 卡顿解释。三种竖向模式仍需分别回归。 |
| 动画时长 | 默认 300ms，范围 0…500、步进 50；0 同步定位，正数使用 power-cubic ease-in-out。 | 值、范围和 cubic 已对齐。 | `aligned`；它只能控制持续时间，不能消除滚动宿主/任务调度造成的掉帧。 |
| 字号/行高/段距 | 默认 18/1.8/0.2；分页距离为视口高度减“两行 + 两个段距”。 | 默认值和 `readerScrollStep()` 公式一致。 | **直接改变分页距离和每帧位移**；保持上游公式，并对非默认组合验证精确落点。 |
| 字重/字体文件 | 字重和内置/上传字体改变字形测量、换行与栅格化；设置变化会重排正文。 | 同类设置存在，上传字体通过 `@font-face` 生效。 | 动画期间不应变化；复杂字体可能增加绘制成本，但不是固定默认配置下的结构差异。需测量字体加载后首轮翻页，不能凭猜测删除功能。 |
| 主题/背景图 | 上游主题使用颜色或平铺资源；自定义背景可 `background-attachment:fixed`。 | 主题含多层渐变/纹理、cover 背景和亮度遮罩。 | 可能增加移动栅格化成本；第十批只排除了 `filter`，尚未把默认主题与“纯色最小渲染面”做同宿主对照。列为 `must-measure`，不先改变视觉。 |
| 全屏点击 | “下一页/自动/不翻页”只决定点击区域映射。 | 同类状态。 | 只改变入口，不改变动画物理；保持上游中心 20% 和上下区域合同。 |
| 选择文字 | “操作弹窗/忽略”；touchend 前检查选文。 | 同类状态，并有移动选择重试。 | 会影响点击判定，不应进入逐帧路径；普通短按只允许一次同步选区检查。 |
| 自动阅读参数 | 像素/段落模式、滚动像素和间隔只在自动阅读开启时生效。 | 同类设置。 | **不影响普通点击**；不得用自动阅读速度解释普通点击顿挫。 |
| 亮度 | 固定上游没有独立亮度参数。 | 用户要求保留的 50…150 数值，当前以无事件遮罩实现。 | `user-requested acceptable-change`；继续验证遮罩不成为滚动祖先 filter，不把它当成已证实原因。 |

### 仍未对齐的执行路径

| 执行点 | 固定上游 | 当前 `0a77632` | 第十一次判定 |
|---|---|---|---|
| 滚动宿主 | `document.documentElement/body.scrollTop`；正文随根文档滚动。 | 固定高度 `.reader-page` 内的 `.reader-content { overflow-y:auto }`。 | **must-fix / must-test-first**：真实设备已否定“无害技术适配”。优先建立根滚动适配器并让移动竖向文本恢复根文档滚动；EPUB、audio、flip 可保留各自宿主。 |
| 动画结束 | `onEnd` 精确落点并立即 `transforming=false`；仅保存进度以 `duration` 延后。 | animator 落点后释放 active，但导航随后又等待一个完整 `animateDuration` 才调用 `settleVerticalPageScroll()`；这段时间 scroll-sync 仍被抑制。 | **must-fix**：不能把“保存延后”误实现成“整个章节/页状态再锁一个时长”。视觉互斥应立即释放；保存可延后，轻量页码可同步，重型扫描应取消/空闲调度。 |
| 结算工作 | 普通上下滑动的 scroll handler 只计算页码并以 100ms timer 保存；连续模式才扩展章节。 | 结算集中执行可见段落快照、当前章、窗口、布局、响应式进度与保存。 | **must-fix**：普通 page 不应在下次输入附近集中跑完整结算；同一位置只捕获一次快照，重型工作必须可取消并避开动画/输入窗口。 |
| 默认绘制表面 | 根页面 + 上游主题资源。 | 独立滚动层位于多层渐变、纹理、内阴影和亮度遮罩中。 | `must-measure`：用根滚动 + 当前视觉和纯色基线两组 trace 分离宿主与主题成本，不能再让主线程 rAF 采样自行证明丝滑。 |

### 第十一次先失败测试与实施边界

1. 移动 `page/scroll/scroll2` 的普通文本使用 `document.scrollingElement` 作为权威滚动宿主；
   `.reader-content` 在该场景不得形成 `overflow:auto` 的第二滚动面。桌面、flip、EPUB、audio
   不得被无条件改成根滚动。
2. 上/下点击、顶部/底部、进度定位、恢复位置、搜索跳转、自动阅读、原生 touch/wheel 和连续
   章节窗口都必须从同一个有效滚动宿主读写；不得出现点击移动根页面而进度仍读取内部元素。
3. 正数动画结束后立即释放输入互斥，不再额外锁定一个 `animateDuration`。页码更新与保存/章节
   快照分级调度；新点击、原生手势、切章和卸载可取消旧重型结算。
4. 配置合同覆盖 normal/Kindle、方案切换、autoTheme、pageMode、非默认字号/行高/段距：最终
   有效模式、时长和步长必须明确；Kindle 不得额外强制“全屏点击=不翻页”，`autoTheme` 默认
   与上游一致，`pageMode` 进入方案和持久化映射。
5. 390×844、360×800 真实浏览器同时记录根/内部 `scrollTop`、输入到首位移、帧间隔、Long Task、
   LayoutShift 和段落几何。当前主题与纯色诊断基线均运行，诊断基线只用于归因，不改变默认 UI。
6. 保留用户要求的手指/滚轮原生连续滚动、数值 stepper、亮度和当前数据；不改后端阅读进度
   格式，不以恢复此前自创缓冲/easing/整章 transform 方案代替固定上游根滚动复刻。

### 第十一次实施与候选验证结果

- 移动普通文本 `page/scroll/scroll2` 已恢复以 `document.scrollingElement` 为权威滚动宿主；
  `.reader-content` 在这些场景改为随文档增长且保持 `scrollTop=0`。桌面工作区、flip、EPUB、
  audio 和图片漫画继续使用各自原有宿主。
- 顶部/底部、页码滑条、点击分页、位置恢复、搜索定位、连续章节窗口和进度计算统一通过滚动
  视口适配器读写；段落/章节目标使用 DOM 几何映射到当前宿主，不再混用内部 `offsetTop`。
- 正数动画结束后立即释放分页互斥，结算任务只跨一个任务边界；单章 page 结算只更新布局和
  延迟保存，不再扫描整章段落。连续模式仍保留章节窗口和锚点结算。
- `autoTheme` 默认恢复为 `true`，`pageMode` 进入服务端 payload、方案快照和用户 scope，Kindle
  不再额外强制 `clickMethod:none`；页面模式标签不再伪装成“仅本机”。
- 启动浏览器合同额外复现并修复一条设置竞态：旧启动顺序会在远端设置读取完成前应用默认
  自动主题方案并安排写回；重章节渲染时，该写操作可能使正在读取的设置失效，从而把接口中的
  `0ms/autoTheme=false` 变成运行态 `300ms/autoTheme=true`。现改为登录用户先完成远端读取，
  再按最终 `autoTheme` 应用系统主题；离线或匿名场景仍能应用本地主题。
- 前端全量 `544/544`、生产构建和 Go 全量通过。真实 Chromium 已通过：
  `reader-text-modes-contract`（390×844、360×800 的 0/100/500ms、双向/连续点击、章末入口、
  Long Task/LayoutShift）、`reader-mobile-contract`（桌面、双手机、自适应/强制手机 iPad）、
  `reader-continuous-contract`（三视口、scroll/scroll2、跨章锚点/事务）和
  `reader-image-contract`。
- 实现提交 `99e3e433b4a52f4aded9192a3182e083ffbdf3be` 已推送 `main`。本地 ARM64
  候选镜像通过新卷、历史卷、重启、TXT/EPUB/UMD/CBZ、相对缓存、用户隔离和 portable
  backup/restore 门禁；候选容器另通过真实 EPUB 上传、解析、1440×900/390×844/360×800
  iframe 阅读与浏览器返回合同。
- 未使用云端构建；本机完成 linux/amd64、linux/arm64 构建并上传
  `ghcr.io/changshengyu/openreader:99e3e43` 与 `latest`。两个标签共同指向 OCI index
  `sha256:345a32db535c95e601dd19152e635e47e15a8d4f9b02bbf38467eb8140ebae3b`；amd64 manifest 为
  `sha256:65fa88ae9dcfbfa0967a4c3b0a1073e53aed5e5a7e54c53f299c4997249a53a6`，arm64 manifest 为
  `sha256:fe44b78dbaa6b5d0c9e7ae07578ed1cfced948acf9fce35edd5b01b83888dc00`。远端两标签和平台
  清单已分别核验一致。
- 当前状态为 **Docker-published / awaiting device verification**。自动门禁只证明执行路径已
  回到固定上游结构，最终丝滑度仍由用户设备验收。

## 2026-07-23 第十二次设置切换与当前位置连续性复审

第十一次恢复根滚动宿主后，`Reader` 仍把“读取当前位置”放在 `reader.mode` 已经改变之后。
这与固定上游的状态顺序相反：上游 `ReadSettings#setReadMethod()` 会先发出
`readMethodChange`，`Reader#beforeReadMethodChange()` 同步保存当前可见段落，然后才写入新的
`readMethod`；`pageType` 在整套 Kindle/正常配置生效前也会先走相同预捕获。随后
`Reader` 的 `isSlideRead` watcher 重新分页，并优先 `showParagraph(currentParagraph, true)`，
只有没有段落锚点时才退回 `showPage(currentPage, 0)`。

配置方案和自动昼夜没有单独发出 `readMethodChange`，但它们会整套覆盖配置；一旦
`isSlideRead` 改变，仍进入同一个“重新分页后优先恢复 `currentParagraph`”分支。上游滚动保存
也持续维护 `currentParagraph`。因此 OpenReader 的等价实现不能只修“翻页方式”按钮，必须覆盖
所有可能隐式改变有效阅读模式或滚动宿主的配置入口。

### 状态与入口矩阵

| 入口 | 固定上游状态顺序 | 当前 `99e3e43` | 第十二次判定 |
|---|---|---|---|
| 直接切换翻页方式 | 变更前同步保存当前可见 `h3,p`；变更后重分页并恢复同一段落。 | `setMode()` 先改 Pinia；默认 watcher 随后调用 `currentOffset()`。此时 computed 已指向新 mode/新宿主。 | **must-fix**：捕获顺序反了。 |
| 正常 ↔ Kindle | `setPageType()` 在整套配置覆盖前触发段落预捕获，再触发 mode/pageMode 变化。 | store 一次同步改 `pageType/pageMode/mode/font/theme`；Reader 只事后观察 `mode` 和排版字段。 | **must-fix**：可能同时启动 mode restore 与 typography restore，后完成者覆盖前者。 |
| 配置方案 | 方案整套覆盖；若横/竖分支变化，Reader 重分页并优先恢复已维护的当前段落。 | `setCustomConfig()` 可同时改 mode、pageMode、字体和段距，没有统一的 Reader 位置事务。 | **must-fix**：不能由多个 watcher 对同一批变更并发恢复。 |
| 自动昼夜 | 系统主题选择默认白天/黑夜方案，语义与选择配置方案相同。 | `App.vue` 在 Reader 外部直接调用 `applyAutoTheme()` → `setCustomConfig()`。 | **must-fix**：面板 emit 无法覆盖这一入口，必须由 Reader/Store 边界统一处理。 |
| 页面模式 | 改变 mini interface；由重排/窗口尺寸路径保持当前页。 | `pageMode` 可在 auto/mobile 间改变移动判定和 document/internal viewport 选择，但没有独立位置事务。 | **must-fix**：宿主改变时也要使用变更前锚点。 |
| EPUB、音频、普通图片漫画 | 有效模式固定为非 slide 分支；配置中的 readMethod 不应重建文本章节窗口。 | `readerEffectiveMode()` 已做格式门禁，但 raw `reader.mode` watcher 仍会执行重建/恢复。 | **must-fix**：只在有效模式或有效宿主变化时执行文本模式事务。 |

### 已确认的错误机制

1. `useReaderMode()` 的 watcher 是默认 flush；回调执行时 `reader.mode` 已是新值。
   `scrollViewport`、`isContinuousScrollRead` 和 `currentOffset()` 也都按新状态解释旧 DOM。
2. 移动竖向 → flip 时，旧位置在 `document.scrollingElement`，事后读取却切到内部
   `.reader-content`/`page`；flip → 竖向则可能读到根页面遗留的 `scrollTop`。两者都可能回到
   本章开头或错误页。
3. `restoreReadingPosition()` 的 `offset` 不是跨模式同一种量纲：flip 把它当页号，竖向文本把
   它当章节文本位置。把旧模式 offset 直接交给新模式恢复，即使捕获时机正确也不等价。
4. 配置方案/Kindle 会在一个同步批次内同时改变 mode 与字号、行高、段距；
   `useReaderMode()` 和 `useReaderTypographySync()` 当前可并发执行两套异步恢复，没有代际或
   单事务所有权。

### 第十二批先失败测试与实施边界

1. 建立统一的“设置布局事务”：在任何有效 mode/viewport/排版变更写入 DOM 前，从旧宿主同步
   捕获 `{chapterIndex, paragraphPos, paragraph identity/viewport offset}`；重排后按段落锚点
   恢复。只有锚点不存在时才允许退回旧页/章节百分比。
2. 覆盖 `page ↔ flip`、`scroll/scroll2 ↔ flip`、normal ↔ Kindle、自定义方案、系统自动昼夜
   和 auto ↔ mobile；每项都断言章节不变、恢复后首个可见段落不回退到章首。
3. 同一同步批次内 mode、pageMode、字号、行高、段距同时变化时只允许一个恢复事务；较旧的
   异步事务必须失效，不能在新事务完成后把页面拉回旧位置。
4. 普通文本以段落位置为跨模式权威语义，不把 flip 页号与文本 offset 混用。EPUB/音频/普通
   图片漫画仅在自身有效宿主实际改变时恢复，不因 raw mode 配置变化清空或重建内容。
5. 390×844、360×800 和 iPad 1024×1366 真实浏览器从章节中段分别执行上述入口；记录变更前后
   `data-pos`、章节索引、根/内部 `scrollTop`、工具层/设置面板并存和控制台错误。
6. 本批不改变用户正在验收的 cubic 点击动画、分页步长、根滚动、原生手指/滚轮、配置值或
   后端进度格式；它只修复设置变化造成的位置跳转和竞态。

### 第十二批实施与发布前验证结果

- `useReaderMode()` 现在观察“有效阅读模式 + 有效移动界面 + 排版字段”这一份布局状态，而不是
  只在 raw `reader.mode` 写入后读取位置。Vue pre-flush 阶段从旧 mode 对应的 document/internal
  viewport 同步捕获当前 `h3,p` 段落锚点；同一配置方案中 mode、pageMode、字号、字重、行高、
  段距和宽度的同步写入只形成一个恢复事务。
- 跨横竖模式不再传递不同量纲的 offset。锚点保存章节、段落 `data-pos`、DOM 顺序和百分比；
  新布局稳定后优先恢复同一段落，缺失时才按同模式 offset 或跨模式 percent 回退。横向多栏
  恢复后会检查目标实际可见性，并校正恰好落在下一栏边界的页码。
- 配置事务带代际门禁；较旧异步恢复失效后不能再次校正页码或保存进度。原
  `useReaderTypographySync()` 在 Reader 中只负责字体资源同步，排版位置恢复由统一事务拥有，
  不再与 mode restore 竞跑。
- `readerEffectiveMode()` 成为 watcher 的输入。EPUB、音频和普通图片漫画的有效模式未变化时，
  修改 raw 阅读方式不会清空/重建内容；TTS 临时 page 分支仍通过有效模式事务和既有朗读段落
  恢复合同验证。
- 新增 `reader-settings-position-contract.mjs`，在 390×844、360×800、1024×1366 从章节中段
  验证 page ↔ flip、整套配置方案（同时改变排版）、normal ↔ Kindle、系统自动昼夜及
  auto ↔ mobile；每次以切换瞬间的当前可见段落为权威，工具层和设置面板保持并存。
- 前端全量 **551/551**、Vite 生产构建和 Go 全量通过。真实 Chromium 通过设置位置、文本三模式、
  桌面/双手机/自适应与强制移动 iPad、连续跨章、图片/漫画、音频和 TTS 合同。独立
  `reader-epub-contract` 指向纯 Vite preview 时在导入 API 得到预期的代理 502，因此未把该次
  启动计作 EPUB 产品回归；本批对固定格式“不因 raw mode 重建”的单元合同已通过，Docker 候选
  仍须运行真实 Go 后端 EPUB 门禁。
