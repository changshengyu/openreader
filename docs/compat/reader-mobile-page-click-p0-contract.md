# Reader 移动端上下滑动点击翻页与章末入口合同（P0）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 已实施并通过回归。本合同记录用户实测的“上下滑动”点击翻页顿挫，
以及正文末尾缺少上游“加载下一章”入口；合同先于应用实现独立提交。

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
