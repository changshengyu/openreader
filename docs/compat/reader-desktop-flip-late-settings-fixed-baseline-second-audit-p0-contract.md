# Reader 桌面端晚到的“左右滑动”设置仍进入翻页布局（P0）

状态：**implementation-complete / regression-validated**（合同 `098a662`、红测 `0a73689`、实现 `8cb15dc`，见第 11 节）。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本会话 scratchpad 里的克隆，工作树干净；约定路径
`/private/tmp/changshengyu-reader-dev` 在会话沙箱里不可写）。OpenReader 基线：`ec3736f`（origin/main，`24511c0` 发布之后只有文档提交），
行号以它为准。

起因：[面板正文控件合同](reader-desktop-panel-content-controls-fixed-baseline-second-audit-p0-contract.md)的浏览器合同里，
`C1 flip` 的 12 组（3 个视口 × 4 个面板）每次跑都有 2–6 组在第一步 `page.waitForSelector('.reader-chapter-end')` 超时，失败的组每次不同，
`c62c619` 与 `24511c0` 两个构建都有（该合同第 4.1 节也记过一次）。本文查明这不是 harness 的等待问题，而是 Reader 的竞态。

## 1. 范围

- 对象：桌面场景（`isMobileReader` 为假：宽度大于 750 的自适应模式，含 iPad 1366×1024、1024×1366）下，阅读方式设置为
  `flip`（左右滑动）时 Reader 的渲染模式。
- 只看设置“晚到”的路径：Reader 视图完成 setup 之后，`reader.mode` 才变成 `flip`。来源有：
  - 账号设置 `GET /api/settings/reader`（`App.vue` 挂载时发出，与 Reader 路由组件的 setup 并发）；
  - 其他设备改设置后的 WebSocket `settings_update`（`useSync.js:129–135` → `reconcileReaderSettingsUpdate` → `loadReaderSettings`）；
  - 保存时的冲突响应（`saveReaderSettings` 收到 `x-openreader-setting-conflict`）与备份恢复（`restoreSync.js:29`）；
  - 本设备套用带 `mode: 'flip'` 的方案（配置方案、日夜切换与自动日夜，`setNightTheme` / `setCustomConfig` 用方案快照覆盖 `mode`）。
- 不含：移动端（mini 界面）的左右滑动；朗读条、自动阅读、EPUB、音频、普通图片漫画把 flip 降为 page 的规则（`readerEffectiveMode`）；
  setup 时与界面切换时已有的桌面转换本身（第 3.2 节、第 10 节）。

## 2. 上游证据（fa22f271）

- `web/src/plugins/vuex.js` L451–453：`isSlideRead: state => state.miniInterface && state.config.readMethod === "左右滑动"`，是 getter，
  随 `state.config` 与 `miniInterface` 实时重算。
- `web/src/views/Reader.vue` L586–594：组件的 `isSlideRead` 在自动阅读、朗读条、EPUB、漫画、音频时为假，否则取上面的 getter；
  `isScrollRead`（L595–603）、`chapterClass`（L607–612）、章末 `加载下一章`（L246 `v-if="show && !isSlideRead && !error && !isScrollRead"`）
  都由它派生。L417–422 的 watcher 在 `isSlideRead` 变假时清掉横向位移。
- `web/src/plugins/vuex.js` L175–200 `setConfig`：整份替换 `state.config`，不因界面改写 `readMethod`。配置从本地缓存或服务器恢复时
  （L636–645 `syncFromLocalStorage`，`Index.vue` L2094–2103 `getUserConfig` 后再 `syncFromLocalStorage`）走的是同一个 mutation。
- `web/src/components/ReadSettings.vue` L322–334：桌面（`!miniInterface`）不显示“左右滑动”选项；L568–590 简洁模式写入
  `readMethod: "左右滑动"` 的同时把 `pageMode` 设为“手机模式”并切到 mini 界面。
- 结论：上游桌面端从不进入左右滑动分支，与设置何时到达无关；设置本身保持“左右滑动”，回到 mini 界面时继续滑动。

## 3. 当前 OpenReader

### 3.1 桌面不进入 flip 的机制

- `frontend/src/composables/useReaderMode.js:162–170`：`watch(options.isMobileReader, …, { immediate: true })`，桌面且
  `reader.mode === 'flip'` 时调用 `reader.setMode('page')`。它只在 Reader setup 时和 `isMobileReader` 变化时运行。
- 桌面的设置面板也不显示“左右滑动”（`ReaderSettingsPanel.vue:465–470` 的 `mobileOnly`，`:494–496` 过滤）。
- 其余代码依赖“桌面上 `reader.mode` 从不是 `flip`”这一不变量，直接读原始设置：`useReaderLayout.js:40`、`:54`，
  `useReaderKeyboard.js:20`、`:32`、`:44`、`:51`，`useReaderPositionRestore.js:52`，`useReaderPointer.js:72`、`:94`、`:150`、`:187`，
  `useReaderChapterWindow.js:21`、`:118`、`:195`、`:252`，`useReaderLocalProgress.js:62`、`:67`，`Reader.vue:1980`。
- `Reader.vue:924–933` 的 `effectiveReaderMode` 只在 EPUB、音频、普通图片漫画、朗读条、自动阅读时把 flip 降为 page；`Reader.vue:959–965`
  的 `showChapterEndPrompt` 要求 `effectiveReaderMode === 'page'`。

### 3.2 竞态

- `App.vue:73–91` 挂载时先 `applyAutoThemeFromSystem()`（`followDeviceColorScheme` → `setNightTheme(…, { deviceLocal: true })`，
  `stores/reader.js:255–291`）：`autoTheme` 为真时用日/夜方案快照覆盖设置，快照字段 `READER_SCHEME_FIELDS`（`:31–55`）第一个就是
  `mode`；随后 `loadReaderSettingsAndApplyTheme()` 发出账号设置 GET。
- Reader 是路由组件，`app.mount` 不等首次导航，它的 setup 与这个 GET 并发：
  - GET 先到：`applyReaderSettings`（`stores/reader.js:547–560`）把 `mode` 设为 `flip`，随后 Reader setup 的 immediate watcher 转成
    `page`，桌面按 page 渲染。
  - Reader 先 setup：watcher 看到的是方案快照里的 `page`，什么也不做；GET 随后到达，本地没有更新的修改（`settingsUpdatedAt` 为空），
    `loadReaderSettings`（`:561–587`）照常应用服务器值，`mode` 变成 `flip`。`isMobileReader` 不再变化，watcher 不再运行，桌面停在
    flip：根节点 `.reader-shell.flip`，横向分页，`showChapterEndPrompt` 为假，`加载下一章` 永远不挂载。
- WebSocket、冲突响应、备份恢复和本设备套用 flip 方案的路径不是竞态：只要 Reader 已打开，桌面每次都会进入 flip。

### 3.3 已有转换写回账号

`setMode('page')` 经 `markSettingsDirty()`（`stores/reader.js:520–527`）刷新 `settingsUpdatedAt` 并在 700ms 后 `PUT` 整份设置，所以
setup 时的转换会把账号里的 `flip` 改成 `page`。上游从不因界面改写 `readMethod`（第 2 节）。本文不重开这一点（第 6、10 节），但目标合同
不能把它扩大到晚到路径：WebSocket 会把其他设备刚选的 `flip` 推到这台桌面，若这里也写回 `page`，那台设备随后收到 `settings_update`，
选择就被撤销。

## 4. 真实浏览器复现（2026-10-30）

环境：本 worktree 编译的 go1.26.10 二进制，服务只提供静态文件，接口全部由 `page.route` 模拟（同面板正文控件合同第 11 节）；Playwright
1.61.1 / chromium-headless-shell，Node v26.4.0；服务与浏览器在沙箱外运行（会话沙箱禁止 loopback 监听，Chromium 在沙箱内也起不来）。

1. 现行代码（`ec3736f` 的 `npm run build`）：`SMOKE_CASES='^C1 flip'` 连跑两次，各 9/12 通过；失败组第一次是 1440 书源、1024×1366
   书源与设置，第二次是 1440 书架与书源、1366×1024 目录，失败点都是 `harness: page.waitForSelector: Timeout 10000ms exceeded.`。
2. 临时探针（scratchpad 里的前端副本，在 `useReaderChapterLoader`、`useReaderMode`、reader store 和 `Reader.vue` 里记录时间点，构建到
   独立目录，未入库）：
   - 失败组（1366×1024 设置）：92ms Reader setup，`isMobileReader` watcher 看到 `mode: page`；102ms 账号设置 GET 到达，
     `store:apply page → flip`；随后一次 page→flip 布局事务；118–131ms 第 0 章加载完成（`chapterLoaded: true`）；10.1s 超时时
     `reader.mode` 仍为 `flip`，根节点 `reader-shell flip`，正文 60 段在，`.reader-chapter-end` 不存在。
   - 通过组（1440 书架）：181ms GET 先到并应用 `flip`；196ms Reader setup 时 watcher 把 `flip` 转成 `page`；第 0 章加载后按钮出现。
   - 两组的差别只有 GET 与 Reader setup 的先后。`C1 page` 与 C2–C4 的服务器设置不是 flip，不受影响。
3. 定序复现：同一 harness 让账号设置 GET 等到 `.reader-shell` 出现后才返回（固定为“Reader 先 setup”），1440×900 四个面板：
   `C1 flip` 4/4 FAIL（都是上面的超时），`C1 page` 4/4 PASS。竞态由此变成确定的失败。
4. 探针还显示一个相关问题：只在 `reader.mode` 上加一个 watcher 把晚到的 `flip` 转回 `page` 时，`useReaderMode` 的 `layoutState`
   watcher 先看到 `flip`，跑一次 page→flip 位置事务，随即被 flip→page 的第二次事务取代；第二次按 flip 的几何在仍是 page 布局的
   DOM 上取锚点。正文读到一半时收到其他设备的设置，这一跳可能把阅读位置带走。所以目标合同要求有效模式本身在桌面不出现 flip（第 7 节 F1）。

## 5. 差异分类

| # | 场景 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| D1 | 桌面 Reader setup 之后才到的 `flip`（账号设置 GET 晚到、WebSocket、冲突响应、备份恢复） | 不滑动，按非滑动分支渲染（`isSlideRead` 为假） | 进入 flip：横向分页，章末 `加载下一章` 不挂载，键盘、指针、布局按 flip 处理 | **`must-fix`** |
| D2 | 桌面上套用带 `flip` 的方案（配置方案、日夜切换、自动日夜） | 同上 | 同上 | **`must-fix`**，同一规则 |
| D3 | 晚到的 `flip` 在桌面转为 page 后是否写回账号 | 不改写设置 | —（当前不转换） | 目标：**只改本设备**，不刷新设置时间戳、不 `PUT`、不改方案快照。属于 `acceptable-change`：上游连本设备也不改写，这里沿用 OpenReader“桌面原始设置不是 flip”的实现，只是不把它写回账号 |
| D4 | setup 与界面切换时已有的转换写回账号（第 3.3 节） | 不改写 | 写回 `page` | 不在本切片，见第 10 节第 1 条 |

没有 `intentional-redesign` 或 `unknown` 项。

## 6. 已签收、不重开

- 桌面“左右翻页”按 page 渲染（面板正文控件合同第 3.1 节、gap analysis 中按钮层级条目），设置面板桌面隐藏“左右滑动”。
- setup 时、`isMobileReader` 变化时的转换与它写回账号的行为（D4）；朗读条、自动阅读、EPUB、音频、普通图片漫画的 `readerEffectiveMode` 规则；
  CBZ 在 mini 界面保留显式 flip。
- 方案快照包含 `mode`、自动日夜只改本设备、账号设置的时间戳比较与冲突处理（`loadReaderSettings`、`saveReaderSettings`）。
- 面板正文控件合同的 P1–P6 与它的 108 组浏览器合同；后端、API、数据库、浏览器存储 key、持久文件；镜像与生产。

## 7. 目标合同

F1. 有效模式按上游 `isSlideRead` 派生：`readerEffectiveMode` 在桌面（非 mini 界面）把 `flip` 当作 `page`。Reader 打开期间
  `reader.mode` 一旦变为 `flip`（不论来自账号设置、其他设备、冲突响应、备份恢复还是本设备套用的方案），根节点不出现 `.reader-shell.flip`，
  章末 `加载下一章` 照常挂载，结果与“setup 前就收到 flip”相同。桌面的有效模式不因此变化，`useReaderMode` 不做位置事务，阅读位置不动。

F2. 直接读原始设置的代码（第 3.1 节）仍依赖“桌面上 `reader.mode` 不是 `flip`”：晚到的 `flip` 在同一轮更新里转回 `page`，只改本设备，即
  `setMode('page', { deviceLocal: true })`。它与自动日夜的 `deviceLocal` 相同，经 `markSettingsDirty({ localOnly: true })` 返回，不刷新
  `settingsUpdatedAt`，不安排 `PUT /api/settings/reader`，也不改当前方案快照。账号里的 `flip` 保留给做出选择的设备。

F3. 不变：
- setup 时与 `isMobileReader` 变化时的转换仍是 `setMode('page')`（含写回账号，D4）。随 F1，桌面 setup 时这次转换不再改变有效模式，
  所以不再跑那次 flip→page 位置事务（它发生在第 0 章加载之前，没有可恢复的位置）；mini→桌面切换只跑一次从 flip 到 page 的事务；
- mini 界面里 `flip` 保持 `flip`，包括同一次应用里先写 `mode`、再把界面切到 mini 的情况（以应用完成后的 `isMobileReader` 为准）；
- `readerEffectiveMode` 不传新参数时行为不变；朗读条、自动阅读、EPUB、音频、普通图片漫画的规则不变；`setMode(mode)` 不带选项时不变；
  直接读原始设置的代码不改；
- 不新增 API、数据库字段、浏览器存储 key 或持久文件，后端不改。

实现提示（不强制）：`readerEffectiveMode` 加一个默认 `false` 的 `isDesktop` 参数，`Reader.vue` 传 `!isMobileReader.value`（`isMobileReader`
的声明要移到 `effectiveReaderMode` 之前）；`useReaderMode` 加一个只看 `reader.mode` 的 watcher（默认 `pre` 时机，在整份设置应用完成后才看
`isMobileReader`，避免 `Object.assign` 先写 `mode`、后写 `pageMode` 时误判）；store 的 `setMode` 增加 `options.deviceLocal`。
不采用：
- 只加原始设置的 watcher、不改有效模式（第 4 节第 4 条：两次位置事务）；
- `flush: 'sync'`（在 `pageMode` 写入前误判界面）；
- 去掉原始设置的转换、所有代码改读有效模式（第 3.1 节约 20 处，涉及指针、键盘、布局、窗口与进度键），这属于 D4 的范围。

## 8. 测试计划

### 8.1 单元合同（`node --test`）

新增 `frontend/tests/readerDesktopFlipLateSettings.test.mjs`：

- `readerEffectiveMode`：桌面的 `flip` 为 `page`；mini 界面与不传新参数时仍是 `flip`；桌面的其他模式不变。
- `useReaderMode`：
  - 桌面 setup 之后 `reader.mode` 变成 `flip`：变回 `page`，唯一的调用是 `setMode('page', { deviceLocal: true })`，`capturePosition`、
    `updateLayout`、`restoreCapturedPosition`、`restorePosition`、`saveProgress` 都没有被调用（没有位置事务）；
  - mini 界面同样的变化：保持 `flip`，不调用 `setMode`，按原规则走一次位置事务（对照）；
  - 同一次应用里先写 `mode: 'flip'`、再把界面切到 mini：保持 `flip`，不调用 `setMode`（对照，锁住 F3 第 2 项）。
- reader store（经 Vite SSR 加载真实 store，写法同 `readerDayNightDeviceScope.test.mjs`）：`setMode('page', { deviceLocal: true })` 不刷新
  `settingsUpdatedAt`、不改方案列表、800ms 内没有 `PUT`；`setMode('page')` 照旧刷新并写回账号（对照）。
- 现有 `readerMode.test.mjs` 等断言不动。
- 预期红点：现行代码上，`readerEffectiveMode` 第 1 条、`useReaderMode` 第 1 条（mode 停在 `flip`，还跑了一次 page→flip 事务）、store
  第 1 条（时间戳被刷新、设置被写回）失败；三条对照通过。

### 8.2 真实浏览器

新增 `scripts/smoke/reader-desktop-flip-late-settings-contract.mjs`（模拟接口，写法同面板正文控件合同）：

- L1：1440×900 鼠标、iPad 1366×1024 与 1024×1366 触摸。账号设置 GET 等 `.reader-shell` 出现后才返回 `mode: 'flip'`（固定“Reader 先
  setup”）：第 0 章显示后根节点是 `.reader-shell.page`，`.reader-chapter-end` 挂载；设置应用后的 1.5s 内没有 `PUT /api/settings/reader`。
- L2：同样三个视口，Reader 已按 page 打开、正文已滚动后，账号设置变成 `flip`，经 `page.routeWebSocket` 模拟的同步连接发一条
  `settings_update`：仍是 page，阅读位置不变（滚动差不超过 1px），`加载下一章` 仍在，没有设置 `PUT`。
- L3（对照）：390×844 触摸手机，L1 的定序下进入 `.reader-shell.flip`。
- 预期红点：现行代码上 L1、L2 的 3 个视口都停在 `.reader-shell.flip`（L1 的 `.reader-chapter-end` 不存在）；L3 通过。
- 面板正文控件合同 `reader-desktop-panel-controls-contract.mjs` 不改；修复后连续 5 次全量 108/108。

### 8.3 回归

- frontend `npm test`（写明新增条数）、`npm run build`；浏览器：新合同、面板正文控件合同全量 5 次、`reader-tts-contract`（其中桌面
  “normalize flip to page before opening TTS”一条依赖同一转换）、`reader-settings-position-contract`、`reader-text-modes-contract`、
  `reader-mobile-contract`、`reader-error-surface-contract`（含 flip 设置）、`openreader-smoke`。
- 跳过并写明原因：Go `test`/`vet`/`race`（后端不改）；依赖 advisory、Compose 与卷门（依赖、Dockerfile、数据格式都不改）。

## 9. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交。
2. **红测**：8.1、8.2，记录精确失败点，`test: expose late flip settings entering the desktop slide layout`。
3. **实现**：`fix: keep a late flip setting off the desktop slide layout`，只改 `useReaderMode.js`、`stores/reader.js` 与 `Reader.vue`。
4. **回归**：按 8.3 执行。
5. **记录**：提交号、结果和状态标签写回本文件、gap analysis、matrix；面板正文控件合同的记录节写明 C1 flip 偶发超时的原因与证据。
6. **发布**：实现提交改了 `frontend/**`，合入 main 会触发镜像发布，由协调方决定。

## 10. 观察（不在本切片）

观察 1、2 由 D4 切片处理，见 [`reader-desktop-flip-account-fixed-baseline-second-audit-p1-contract.md`](reader-desktop-flip-account-fixed-baseline-second-audit-p1-contract.md)（它的第 3.3 节修正了本文第 3.1 节的读者清单）。

1. D4：setup 时与界面切换时的桌面转换写回账号。手机选了“左右滑动”后在桌面打开 Reader，账号就变成“上下滑动”；iPad 从手机模式切回
   自适应再切回去，左右滑动也丢了。上游只在渲染上派生（第 2 节）。改成派生要处理第 3.1 节列出的约 20 处直接读原始设置的代码，适合单独
   盘点。
2. 桌面在 F2 之后修改其他设置时，`PUT` 的整份设置会带上本设备的 `page`，账号里的 `flip` 仍会被覆盖，与 D4 同源。
3. 面板正文控件合同的 harness 在 `localStorage.reader` 里写了 `mode`，但 App 挂载时的自动日夜方案先覆盖了它，真正起作用的是模拟的账号
   设置；这正是它能暴露本竞态的原因，本切片不改 harness。

## 11. 实施与验证（2026-10-30）

| 提交 | 阶段 | 内容 |
| --- | --- | --- |
| `098a662` | 合同 | 本文件、gap analysis 与 matrix 顶部条目。 |
| `0a73689` | 红测 | 单元合同 `frontend/tests/readerDesktopFlipLateSettings.test.mjs`（5 条）；浏览器合同 `scripts/smoke/reader-desktop-flip-late-settings-contract.mjs`（L1–L3，7 组）。 |
| `8cb15dc` | 实现 | `useReaderMode.js` +16、`stores/reader.js` +2/−2、`Reader.vue` +2/−1。 |

分支 `claude/beautiful-morse-fb1f94` 先快进到 origin/main `ec3736f`。`git diff ec3736f 8cb15dc -- backend` 为空。

**实现**：

- `readerEffectiveMode` 增加第 7 个参数 `isDesktop`（默认 `false`），桌面上的 `flip` 返回 `page`。`Reader.vue` 传 `!isMobileReader.value`，
  并把 `isMobileReader` 的声明移到 `effectiveReaderMode` 之前（`readerIPadWorkspaceContract.test.mjs` 的源码断言与位置无关）。
- `useReaderMode` 在已有的 `isMobileReader` watcher 之后加一个只看 `reader.mode` 的 watcher：桌面上变成 `flip` 时调用
  `setMode('page', { deviceLocal: true })`。
- store 的 `setMode(mode, options = {})` 调用 `markSettingsDirty({ localOnly: options.deviceLocal === true })`。
- 没改：setup 时与界面切换时的转换（仍写回账号，D4）；直接读原始设置的代码；朗读条、自动阅读、EPUB、音频、普通图片漫画的规则；
  后端、API、数据与存储 key。

**单元**（`node --test`）：

| 代码 | 结果 |
| --- | --- |
| `0a73689`（产品代码同 `ec3736f`） | 新增 5 条中 3 条失败：`readerEffectiveMode('flip', …, true)` 得到 `'flip'`；桌面 setup 之后的 `flip` 停在 `'flip'`（`the raw setting must not stay flip on the desktop`），还跑了一次 page→flip 事务；`setMode('page', { deviceLocal: true })` 刷新了 `settingsUpdatedAt`（`a device-local mode change must not look newer than the account`）。两条对照通过。 |
| `8cb15dc` | 5/5；`readerMode.test.mjs` 10/10 不变；全量 841/841（原 836 + 5）；`npm run build` 通过，只有既有的 chunk 大小提示。 |

**浏览器**：本 worktree 编译的 go1.26.10 二进制，服务只提供静态文件；接口由 `page.route` 模拟，新合同的同步连接由 `page.routeWebSocket`
模拟。Playwright 1.61.1 / chromium-headless-shell，Node v26.4.0，服务与浏览器在沙箱外运行。每个构建一个服务，各有独立的临时
data/cache/library 与随机 JWT secret（未输出）。

| 构建 | 新合同（7 组） |
| --- | --- |
| `ec3736f` 的 `npm run build`（两次） | 两次都是 6/7 FAIL。L1 三个视口都停在 `.reader-shell.flip`，`加载下一章` 缺失。L2 三个视口变成 flip，文档滚动 1660/1619/1505 → 0，`加载下一章` 缺失。L3 通过。所有组都没有设置 `PUT`：现行代码根本不转换晚到的 flip。 |
| scratchpad 原型：只加原始设置的 watcher（未入库） | L1、L3 通过；L2 三个视口只在位置断言失败（1660/1619/1505 → 54）。`layoutState` 先看到了 flip（第 4 节第 4 条），这是 F1 必要的直接证据。 |
| `8cb15dc` | 7/7 PASS（两次）。L1 是 `.reader-shell.page`，按钮挂载，没有设置 `PUT`。L2 滚动 1660/1619/1505 不变，按钮仍在，没有 `PUT`。L3 进入 flip。 |

**面板正文控件合同**（`reader-desktop-panel-controls-contract.mjs`，脚本与断言未改）：

- 修复前（`ec3736f` 构建）：`SMOKE_CASES='^C1 flip'` 两次各 9/12，失败组不同，都是 `加载下一章` 等待超时。用 scratchpad 里的
  harness 副本让账号设置 GET 等 `.reader-shell` 出现后才返回：1440×900 `C1 flip` 4/4 FAIL，`C1 page` 4/4 PASS。
- 修复后（`8cb15dc` 构建）：连续 5 次全量，每次 108/108（每次约 399s）；`C1 flip` 共 60/60，`C1 page` 60/60；没有 harness、页面或路由错误。
  同一 harness 副本在修复后的定序晚到下：三个视口 × 四个面板的 `C1 flip` 与 `C1 page` 共 24/24 PASS。

**回归**（`8cb15dc`，同一服务）：

- 新合同第二次 7/7。
- `reader-tts-contract`：默认 390×844，另用 `SMOKE_VIEWPORT_WIDTH/HEIGHT` 跑 1440×900、1366×1024、1024×1366，都通过；桌面分支的
  `desktop reader should normalize flip to page before opening TTS` 与本切片同一转换。
- `reader-settings-position-contract`、`reader-text-modes-contract`、`reader-continuous-contract`：通过。
- `reader-mobile-contract`：通过（桌面、手机、自适应 iPad、强制移动 iPad）。
- `reader-error-surface-contract`：30/30（含 flip 设置）。
- `openreader-smoke`：三个视口通过（入口 `/login`）。
- 服务日志：除 `/ws/sync` 的 401（假 token，其余合同不接管同步连接）外都是 200；没有 5xx，没有 panic。

**其余门**：Go `test`/`vet`/`race` 跳过（后端无改动）；依赖 advisory、Compose 与卷门跳过（依赖、Dockerfile、数据格式都没改，卷门只在 Actions
上运行）。实现提交改了 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布，是否合入由协调方决定。

**状态**：Git 为本地分支 `claude/beautiful-morse-fb1f94` 上的提交（`098a662`、`0a73689`、`8cb15dc` 与本记录），未推送，未合入 main。
镜像：未构建，未推送。生产：未访问，未操作。
