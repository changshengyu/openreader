# Reader 移动端上下滑动点击翻页与章末入口合同（P0）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 第二次用户实测后重新打开并完成第二批实现与 Docker 发布。第一批实现已经
移除逐帧重型同步并恢复章末入口；第二批把移动端“上下滑动”的点击分页移到浏览器合成层，
并补齐输入延迟、取消位置和连续点击合同，等待用户人工验证体感。

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
