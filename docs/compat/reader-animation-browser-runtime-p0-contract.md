# Reader 翻页动画与浏览器运行器合同（P0）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-17 已实施，并通过单元、生产构建、三视口 Reader 与重复浏览器启动验证。

## 权威文件

- 上游 `web/src/views/Reader.vue`
  - `nextPage()`、`prevPage()`、`showPage()`、`transform()`、`scrollContent()`。
- 上游 `web/src/components/ReadSettings.vue`
  - `animateMSTime` 输入及 `0…500ms / 50ms` 步进约束。
- 当前 `frontend/src/composables/useReaderNavigation.js`
- 当前 `frontend/src/utils/readerPagination.js`
- 当前 `frontend/src/views/Reader.vue`
- 当前 `scripts/smoke/*.mjs`

## 兼容矩阵

| 项目 | 固定上游行为 | 当前行为 | 判定与动作 |
|---|---|---|---|
| 动画值 | `animateMSTime` 为 `0…500ms`，步进 `50ms`；设置中的数字可直接输入。 | `animateDuration` 保存相同范围，当前 stepper 允许点击数字输入。 | `aligned`，保留用户要求的减号/数字/加号控件。 |
| `0ms` | `transform()` / `scrollContent()` 直接执行最终位置，不创建动画。 | `readerScrollBehaviorForDuration(0)` 返回 `auto`，浏览器直接定位。 | 结果等价，但应并入同一逐帧控制器测试。 |
| 非零竖向点击翻页 | 上游把设置值直接交给 `Animate.duration`，每帧写入 scrollTop；`100ms` 与 `500ms` 的完成时间不同。 | `createReaderScrollAnimator()` 现以设置毫秒数驱动逐帧 scrollTop；导航只把点击/键盘分段翻页交给它。 | `aligned`：0/100/500ms 单元与真实浏览器时间合同通过。 |
| 手指/滚轮滚动 | 用户已明确要求上下滑动保持原生连续滚动。 | wheel/touch 由滚动容器原生处理。 | `intentional-redesign`：动画时长不能接管或量化手指/滚轮滚动。 |
| 横向翻页 | 上游逐帧改变 translateX，持续时间使用同一设置值。 | CSS `transition-duration` 已直接绑定 `--reader-animate-duration`。 | `technical-stack-equivalent`，补时间差浏览器断言，不能改成原生 smooth。 |
| 动画期间重复输入 | 上游 `transforming` 阻止第二次翻页，结束后再保存进度。 | 控制器拒绝重叠动画；切章、顶部/底部、卸载、wheel 和 touchstart 会取消未完成动画。 | `aligned`，原生用户输入不会被未完成的程序动画抢回位置。 |
| smoke 浏览器启动 | 不属于产品上游合同。测试运行器应稳定且不能影响用户浏览器。 | 24 个 smoke 已统一调用 `playwright-runtime.mjs`；默认使用固定 Playwright 1.61.1 的 Chromium Headless Shell，只有显式 `CDP_URL`/`CHROME_PATH` 才覆盖。 | `resolved test infrastructure`：不再默认启动系统 Chrome，也不触碰日常 Chrome 配置。 |

## 实施前测试

1. 单元测试使用可控时间帧验证：
   - `0ms` 同步到最终位置；
   - `100ms` 在 100ms 完成；
   - `500ms` 在相同中间时间仍未完成，最终精确到目标位置；
   - 动画期间重复请求被拒绝或安全取消，不叠加页面位移。
2. `useReaderNavigation` 测试不再只断言字符串 `smooth`，而是断言传入真实持续时间并由动画完成后保存。
3. 真实浏览器在 `page`/上下滑动模式分别以 `0ms`、`100ms`、`500ms` 点击下一页：
   - `0ms` 下一帧前已到目标；
   - `100ms` 显著早于 `500ms` 完成；
   - 三者终点一致且不切章；
   - wheel 仍为原生小步连续滚动。
4. 浏览器运行器契约验证 CDP 连接与自有进程两条路径；多个 smoke 顺序运行不再反复启动/关闭系统 Chrome。

## 允许差异

- Vue 3 使用 `requestAnimationFrame` 和内层 `.reader-content`，代替上游的 Animate 类和
  document 滚动；持续时间、缓动完成点和可见交互必须等价。
- 原生连续手指/滚轮滚动是用户明确要求的优化，不受动画时长控制；动画值只作用于
  点击、键盘和程序化的分段翻页。
- smoke 可以复用独立的持久 CDP Chrome，但不得连接或关闭用户日常 Chrome 配置目录。

## 2026-07-17 实施与验证记录

- 新增 `createReaderScrollAnimator()`：采用与上游等价的 ease-in-out cubic 逐帧滚动；
  `0ms` 同步定位，正数严格使用设置持续时间，终点按滚动范围夹紧。
- `useReaderNavigation` 的竖向点击/键盘分页不再调用浏览器 `smooth`；动画完成后才更新
  可见分页版本并调度保存。重叠翻页会被拒绝，切章、卸载和用户 wheel/touch 会取消动画。
- `reader-text-modes-contract.mjs` 在 390×844 真实生产构建中验证 `0ms` 立即完成、
  `100ms` 在短窗口完成、`500ms` 在 180ms 尚处中途而后精确到同一终点；wheel 仍是
  小步原生连续滚动。原有 1440×900、390×844、360×800 排版/翻页合同继续通过。
- 24 个 browser smoke 删除重复 `loadPlaywright()` 与系统 Chrome 默认路径，统一到共享运行器。
  前端固定 Playwright 1.61.1，并提供 `npm run smoke:install-browser` 安装 Headless Shell。
- macOS 崩溃报告确认旧路径在 `RegisterApplication/TransformProcessType` 阶段 `SIGABRT`；
  新 Headless Shell 不进入 GUI 注册路径。连续启动 text-mode、mobile、continuous、audio
  四组进程全部通过，且没有生成新的系统 Chrome 崩溃报告。
- 当前门禁：前端 418 项测试、后端 `go test ./...`、Vite 生产构建和脚本语法检查通过。
