# Reader 桌面端把账号的“左右滑动”改写成“上下滑动”（D4，P1）

状态：**contract-only**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`（本会话 scratchpad 里用
`git archive fa22f27 web/src` 取出的只读副本）。OpenReader 基线：`b52def1`，即晚到设置切片的 `a574d78`（分支
`claude/beautiful-morse-fb1f94`，未合入 main）合并 origin/main `6193f79` 之后的树，行号以它为准。

起因：[晚到设置合同](reader-desktop-flip-late-settings-fixed-baseline-second-audit-p0-contract.md)第 10 节观察 1（D4）与观察 2。
晚到设置切片让有效模式在桌面把 `flip` 当作 `page`（F1），晚到的 `flip` 只在本设备转回 `page`（F2），但保留了 Reader setup 与
界面切换时写回账号的转换，理由是“约 20 处代码直接读原始设置”。本文盘点这些代码，给出不再改写原始设置的目标合同。

## 1. 范围

- 对象：阅读方式设置 `reader.mode`（上游 `config.readMethod`）在桌面场景（`isMobileReader` 为假）下的写入与读取。
- 路径：
  - 桌面打开 Reader 时账号或本设备缓存里已是 `flip`（例如手机上选了“左右滑动”，再在电脑或横屏 iPad 自适应模式打开同一本书）；
  - mini 界面切到桌面：iPad 页面模式“手机模式”→“自适应”、窗口从 750 以下拉宽；
  - 之后的往返：桌面→mini（iPad“自适应”→“手机模式”、窗口变窄）；
  - 桌面上任何一次设置保存（字号、主题、页面模式等）所带的整份设置。
- 不含：有效模式的派生规则本身（晚到设置合同 F1，`readerEffectiveMode`）；朗读条、自动阅读、EPUB、音频、普通图片漫画把 flip
  降为 page 的规则；mini 界面的左右滑动行为；方案、日夜与账号时间戳机制本身。

## 2. 上游证据（fa22f271）

- `web/src/plugins/vuex.js` L451–453：`isSlideRead: state => state.miniInterface && state.config.readMethod === "左右滑动"`，是 getter。
- `web/src/views/Reader.vue` L586–603：组件 `isSlideRead` 在自动阅读、朗读条、EPUB、漫画、音频时为假，否则取上面的 getter；
  `isScrollRead` 由它派生。Reader 里直接读 `config.readMethod` 的只有 L600–601（`isScrollRead` 内部）、L1266 与 L1304（都在
  `computeShowChapterList` 里 `if (!this.isScrollRead) return` 之后，只区分“上下滚动2”）。没有任何代码因界面改写 `readMethod`。
- `web/src/plugins/vuex.js` L210–216 `setMiniInterface`：只改 `state.miniInterface`；`App.vue` 在 `beforeCreate`（L259）与
  `window.onresize`（L271）时调用它。
  `web/src/components/ReadSettings.vue` L595–603 `setPageMode`：只改 `pageMode` 并调用 `setMiniInterface`。界面切换不碰 `readMethod`。
- `web/src/plugins/vuex.js` L175–209 `setConfig`：整份替换 `state.config`，并按 `syncConfigFiled`（`config.js` L203，即 `defaultDayConfig`
  的全部字段，含 `readMethod`）把当前配置原样同步进当前自定义方案。`readMethod` 在这里也只被原样保存。
- `web/src/components/ReadSettings.vue` L320–334：翻页方式在桌面（`!miniInterface`）隐藏“左右滑动”，高亮条件是
  `config.readMethod == method`；`readMethod` 为“左右滑动”时桌面上没有任何一项高亮。L604–607 `setReadMethod` 只在用户点选时写入。
- 结论：上游的“桌面不滑动”完全是派生；`readMethod` 只由用户选择、方案与配置恢复改变。手机上选的“左右滑动”在桌面打开后仍是
  “左右滑动”，回到 mini 界面继续滑动。

## 3. 当前 OpenReader（`b52def1`）

### 3.1 写回账号的转换

- `frontend/src/composables/useReaderMode.js:166–174`：`watch(options.isMobileReader, …, { immediate: true })`，桌面且
  `reader.mode === 'flip'` 时 `reader.setMode('page')`。
- `setMode` 不带选项时经 `markSettingsDirty()`（`stores/reader.js:520–527`）：`syncActiveCustomConfig()` 把当前方案快照的 `mode`
  改成 `page`，刷新 `settingsUpdatedAt`，700ms 后 `PUT /api/settings/reader`（整份设置，`readerSettingsPayload` 的 `mode: state.mode`，
  `:987–989`）。
- 触发：Reader setup 时（immediate）原始设置已是 `flip`；`isMobileReader` 由真变假。结果：
  - 手机选“左右滑动”后在桌面打开 Reader，账号变成“上下滑动”，手机随后经 `settings_update` 收到 `page`；
  - iPad“手机模式”（左右滑动）→“自适应”→“手机模式”，第二次回到 mini 时是上下滑动，左右滑动丢失；
  - 当前自定义方案的 `mode` 一并改成 `page`。

### 3.2 只改本设备的转换（晚到设置合同 F2）

`useReaderMode.js:176–186` 在 setup 之后变成 `flip` 时调用 `setMode('page', { deviceLocal: true })`，当时不写账号；但本设备的
原始设置已是 `page`，桌面随后任意一次设置保存都会把整份设置（含 `mode: 'page'`）`PUT` 上去，账号里的 `flip` 仍被覆盖
（晚到设置合同第 10 节观察 2）。

### 3.3 直接读原始设置的代码（修正晚到设置合同第 3.1 节）

晚到设置合同第 3.1 节列出的约 20 处并不都读原始设置。`Reader.vue` 把一个只暴露有效模式的对象 `effectiveReaderState`（`:936–952`，
`get mode()` 返回 `effectiveReaderMode.value`）作为 `reader` 传给布局、键盘、指针与滚轮，所以这些位置读的是有效模式，晚到设置切片之后
桌面上已不会是 `flip`：

| 位置 | 传入的 `reader` | 读到的 |
| --- | --- | --- |
| `useReaderLayout.js:40`、`:54` | `effectiveReaderState`（`Reader.vue:1085`） | 有效模式 |
| `useReaderKeyboard.js:20`、`:32`、`:44`、`:51` | `effectiveReaderState`（`Reader.vue:2477`） | 有效模式 |
| `useReaderPointer.js:72`、`:94`、`:150`、`:187` | `effectiveReaderState`（`Reader.vue:1727`） | 有效模式 |
| `useReaderViewportProgress`、`useReaderNavigation`、`useReaderSearchNavigation`、`useReaderProgressControls`、`useReaderProgressPersistence` | 不读 `reader.mode`，`getMode: () => effectiveReaderMode.value` | 有效模式 |
| `useReaderTTS` | `isSlideRead: () => effectiveReaderMode.value === 'flip'` | 有效模式 |

真正读原始设置、依赖“桌面上原始设置不是 `flip`”的只有下面这些：

| # | 位置 | 用途 | 原始设置在桌面为 `flip` 时 |
| --- | --- | --- | --- |
| R1 | `useReaderPositionRestore.js:52`（`Reader.vue:1187` 传入 store） | `restore(offset, { restorePercent })` 按 flip 页号还是按滚动恢复位置 | 只设 `page`（横向页号）并返回，不滚动：带进度打开、章节加载后、布局事务的兜底恢复都停在章首 |
| R2 | `useReaderChapterWindow.js:21`、`:118`、`:195`、`:252`（`Reader.vue:1046`） | 事务作用域键；窗口起点、修剪、扩展（只区分 `scroll2`） | 作用域键随原始设置变化；flip 与 page 在窗口工具里等价，桌面上没有可见差异 |
| R3 | `useReaderLocalProgress.js:62`、`:67`（`Reader.vue:1024`） | 本地进度去重键；本地进度记录的 `mode` | 本地进度记录写成 `flip`，而偏移量是上下布局测得的 |
| R4 | `stores/reader.js:733`、`:741` `saveProgress` | 乐观进度记录与 `PUT /api/progress` 的 `mode` | 进度接口收到 `flip` |
| R5 | `stores/reader.js:807` `syncLocalProgress` | 本地记录缺 `mode` 时的回退值 | 缺 `mode` 的旧记录同步时带上 `flip` |
| R6 | `Reader.vue:1997` `closeTTSBar` | 关闭朗读条后，回到 flip 布局时重新排版并跳回朗读段落 | 桌面关闭朗读条后多跑一次 flip 专用的重排与跳转 |
| R7 | `ReaderSettingsPanel.vue:575` | 翻页方式高亮 | 桌面隐藏“左右滑动”，没有一项高亮（与上游相同） |
| R8 | `stores/reader.js:989` `readerSettingsPayload` | 账号设置 `PUT` 的 `mode` | 即本切片要保留的账号值 |

`progress.mode` 只是元数据：前端读它的只有去重指纹（`stores/reader.js:878`）与本地持久化（`:906`），恢复位置不看它；后端
（`api/progress.go:24`、`:74`，`services/readingprogress/ingress.go:164–172`）原样保存。

## 4. 差异分类

| # | 场景 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| D4-1 | 桌面打开 Reader 时原始设置已是 `flip` | 不改写，按非滑动分支渲染 | 渲染 page，但 `setMode('page')` 刷新时间戳、改写当前方案并 `PUT` `page`，其他设备随后收到 `page` | **`must-fix`** |
| D4-2 | mini→桌面切换（iPad 手机模式→自适应、窗口拉宽） | 不改写；回到 mini 继续滑动 | 同 D4-1 写回 `page`；回到 mini 是上下滑动 | **`must-fix`** |
| D4-3 | 本设备转成 `page` 之后桌面再保存任意设置（晚到设置合同观察 2） | 设置原样保存，`readMethod` 仍是“左右滑动” | 整份设置带 `mode: 'page'` 写回 | **`must-fix`**，同一根因 |
| D4-4 | 桌面翻页方式高亮，账号为 `flip` | 没有一项高亮 | 转换后“上下滑动”高亮 | **`must-fix`**，随 D4-1 与上游一致 |
| D4-5 | 第 3.3 节 R1–R4、R6：原始设置不再改写后，它们需要的“本设备的阅读方式” | 上游只有 `isSlideRead`/`isScrollRead` 两个派生值 | 读原始设置，靠转换保证桌面上不是 `flip` | 目标：改读界面模式（第 6 节 G2–G3），**行为不变**（桌面仍按 page，mini 仍按原始设置） |
| D4-6 | R5：缺 `mode` 的旧本地进度在桌面同步 | —（上游进度没有 `mode`） | 回退值是转换后的 `page` | 改为账号值（可能是 `flip`）。**`acceptable-change`**：只是元数据，恢复位置不读它；只影响写入 `mode` 之前的旧本地记录 |

没有 `intentional-redesign` 或 `unknown` 项。

## 5. 已签收、不重开

- 晚到设置合同 F1：`readerEffectiveMode` 在桌面把 `flip` 当作 `page`，桌面不出现 `.reader-shell.flip`，章末“加载下一章”照常挂载；
  其新浏览器合同 L1–L3 与面板正文控件合同的 108 组都必须继续通过。
- 朗读条、自动阅读、EPUB、音频、普通图片漫画的有效模式规则；CBZ 在 mini 界面保留显式 flip；桌面设置面板隐藏“左右滑动”。
- store 的 `setMode(mode, { deviceLocal })` 选项与其单元测试保留（本切片之后 Reader 不再调用它）。
- 方案快照包含 `mode`、自动日夜只改本设备（它套用的方案字段会随本设备之后的保存一起写回，这是自动日夜的既有行为，不属于 D4）、
  账号时间戳比较与冲突处理。
- 后端、API、数据库、浏览器存储 key、持久文件；镜像与生产。

## 6. 目标合同

G1. 原始设置 `reader.mode` 只由明确的选择改变：设置面板点选、配置方案、日夜方案、简洁/正常切换、账号设置的应用、同步、冲突与恢复。
  Reader setup、界面切换（任一方向）和晚到的设置都不改写它：不调用 `setMode`，不刷新 `settingsUpdatedAt`，不改当前方案，不安排
  `PUT /api/settings/reader`。删除 `useReaderMode.js:166–186` 的两个转换 watcher。

G2. 界面模式 `readerInterfaceMode(mode, isDesktop)`（`useReaderMode.js` 导出）：桌面上的 `flip` 为 `page`，其余原样，不传 `isDesktop` 时
  原样返回。它替代“桌面上原始设置不是 `flip`”这一不变量，值与旧不变量下原始设置的值相同。`readerEffectiveMode` 的桌面规则改为经它
  表达，行为与签名都不变。`Reader.vue` 提供 `interfaceReaderMode = computed(() => readerInterfaceMode(reader.mode, !isMobileReader.value))`。

G3. 第 3.3 节的原始设置读者改读界面模式，保持它们不看有效模式的既有语义（朗读条、自动阅读、EPUB、音频、漫画照旧不影响它们）：
- `useReaderPositionRestore`、`useReaderChapterWindow`、`useReaderLocalProgress` 增加 `getMode` 选项（与 `useReaderNavigation` 等相同的
  写法），不再读 `options.reader.mode`；`Reader.vue` 传 `getMode: () => interfaceReaderMode.value`。前两者不再需要 store。
- `closeTTSBar` 判断 `interfaceReaderMode.value`。
- store `saveProgress(payload)` 的 `mode` 取 `payload.mode`，缺省时仍是原始设置；`Reader.vue` 的 `saveRemote` 传入界面模式。
- `syncLocalProgress` 的回退不变（D4-6）。设置面板高亮与账号设置 `PUT` 继续用原始设置（R7、R8）。

G4. 位置事务（`useReaderMode` 的 `layoutState`，看有效模式与界面）：
- 桌面 setup 时原始设置为 `flip`：有效模式一开始就是 `page`，不跑事务（同晚到设置合同 F3）；
- setup 之后晚到的 `flip`：有效模式不变，不跑事务，原始设置保持 `flip`；
- mini→桌面：一次从 flip 到 page 的事务；桌面→mini：一次从 page 到 flip 的事务，左右滑动恢复。

G5. 不变：`setMode(mode)` 不带选项时照旧写回账号；`readerEffectiveMode` 的参数与结果；朗读条、自动阅读、EPUB、音频、漫画规则；
  后端、API、数据库、浏览器存储 key、持久文件。

不采用：
- 保留转换但一律只改本设备：D4-3 仍在（之后任一次保存把 `page` 写回），本设备往返仍丢失左右滑动；
- 另存一份“账号里的 mode”并在保存时替换：两个真相来源，冲突、恢复与方案同步都要跟着改；
- 原始设置读者改读完整的有效模式：会改变朗读条、自动阅读、EPUB 下的位置恢复、窗口作用域、关闭朗读条与进度元数据的既有语义，
  超出本切片。

## 7. 测试计划

### 7.1 单元合同（`node --test`）

新增 `frontend/tests/readerDesktopFlipAccount.test.mjs`：

- U1 `readerInterfaceMode`：桌面 `flip` 为 `page`；mini 与不传 `isDesktop` 时仍是 `flip`；其他模式不变（命名空间导入，函数缺失时断言失败，
  不让整个文件加载失败）。
- U2 `useReaderMode`（记录调用的假 reader）：桌面 setup 时原始设置为 `flip`：没有 `setMode`，原始设置仍是 `flip`，没有位置事务。
- U3 `useReaderMode`：mini→桌面→mini：两次切换都不调用 `setMode`，原始设置一直是 `flip`；第一次事务从 `{ mode: 'flip', mobile: true }`
  捕获，第二次从 `{ mode: 'page', mobile: false }` 捕获，回到 mini 后有效模式是 `flip`。
- U4 真实 store（Vite SSR，写法同 `readerDesktopFlipLateSettings.test.mjs`）+ `useReaderMode`：账号设置为 `flip` 时桌面 setup、mini→桌面→mini：
  `settingsUpdatedAt` 与当前方案不变，800ms 内没有设置 `PUT`；之后桌面改字号，`PUT` 的 `mode` 是 `flip`（D4-3）。
- U5 store `saveProgress`：原始设置为 `flip`、`payload.mode` 为 `page` 时，乐观记录与 `PUT /api/progress` 的 `mode` 都是 `page`；不带
  `payload.mode` 时仍是原始设置（对照）。
- U6 `useReaderPositionRestore`：`getMode` 为 `page`、传入的 store 原始设置为 `flip` 时按滚动恢复，`page` 不变；`getMode` 为 `flip` 时按页号
  恢复（对照）。
- U7 `useReaderChapterWindow`：窗口按 `getMode` 计算（`getMode` 为 `scroll2`、store 为 `scroll` 时窗口从锚点章开始）。
- U8 `useReaderLocalProgress`：本地进度记录的 `mode` 取 `getMode`。
- U9 `Reader.vue` 源码接线：三个组合函数传 `getMode: () => interfaceReaderMode.value`，`saveRemote` 带界面模式，`closeTTSBar` 判断
  `interfaceReaderMode.value`，脚本里不再出现 `reader.mode ===`/`!==` 比较。

改写（与上游冲突的旧断言，按 CLAUDE.md 重写而不是保留）：
- `readerMode.test.mjs` 的 `forces flip mode back to page on desktop` → 桌面保留 `flip`、有效模式为 `page`、不调用 `setMode`。
- `readerDesktopFlipLateSettings.test.mjs` 第 2 条（断言晚到的 `flip` 被转回 `page` 并调用 `setMode('page', { deviceLocal: true })`）→ 原始设置
  保持 `flip`，不调用 `setMode`，不跑位置事务。其余 4 条不动。

现有 `readerPositionRestore`、`readerChapterWindowController`、`readerLocalProgress` 测试的夹具增加 `getMode: () => reader.mode`，断言不动。

预期红点（`b52def1`）：U1、U2、U3、U4、U5、U6、U7、U8、U9 与两条改写失败（现行代码转换原始设置并写回账号，组合函数忽略 `getMode`，
`saveProgress` 忽略 `payload.mode`）。

### 7.2 真实浏览器

新增 `scripts/smoke/reader-desktop-flip-account-contract.mjs`（模拟接口，写法同晚到设置合同）。账号设置先到：Reader 路由组件的
代码块（构建产物 `assets/Reader-*.js`，开发服务器 `src/views/Reader.vue`）一直等到账号设置已应用才放行，固定“设置先到、Reader 后
setup”。

- A1（1440×900 鼠标、iPad 1366×1024 与 1024×1366 触摸）：账号 `flip`，`?chapter=0&percent=0.5` 打开：`.reader-shell.page`，不是 mini，
  `加载下一章` 挂载，正文滚到一半附近（R1 的守卫），Reader 打开后 1.5s 内没有设置 `PUT`；点“加载下一章”后进度 `PUT` 的 `mode` 是
  `page`（R4 的守卫）。
- A2（iPad 1024×1366 与 1366×1024 触摸）：账号 `flip` + 手机模式：`.reader-shell.flip`；设置面板“页面模式”→“自适应”：桌面
  `.reader-shell.page`；再“手机模式”：重新进入 `.reader-shell.flip`；期间每次设置 `PUT` 的 `mode` 都是 `flip`。
- A3（390×844 手机，对照）：账号 `flip`：`.reader-shell.flip`，没有设置 `PUT`。
- A4（与 A1 相同三个视口）：桌面设置面板翻页方式没有高亮项；“增大字号”之后的设置 `PUT` 的 `mode` 是 `flip`、字号已增大。
- 预期红点（`b52def1` 构建）：A1 三个视口都有一次 `mode: 'page'` 的设置 `PUT`；A2 切到自适应时写回 `page`，回到手机模式停在 page；
  A4 “上下滑动”高亮、字号保存带 `page`。A1 的位置与进度 `mode`、A3 通过。
- 晚到设置合同的 `reader-desktop-flip-late-settings-contract.mjs`（7 组）与面板正文控件合同 `reader-desktop-panel-controls-contract.mjs`
  （108 组）不改，修复后都必须通过。

### 7.3 回归

- frontend `npm test`（写明新增条数）、`npm run build`。
- 浏览器：新合同；晚到设置合同；面板正文控件合同全量；`reader-tts-contract`（手机与三个桌面视口，桌面的“normalize flip to page”与
  “TTS close should retain the normalized page branch”两条依赖 R6）；`reader-settings-position-contract`（含 iPad 页面模式事务）；
  `reader-text-modes-contract`、`reader-mobile-contract`、`reader-error-surface-contract`（含 flip 设置）、`reader-continuous-contract`、
  `openreader-smoke`。
- 跳过并写明原因：Go `test`/`vet`/`race`（后端不改）；依赖 advisory、Compose 与卷门（依赖、Dockerfile、数据格式都不改）。

## 8. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交。
2. **红测**：7.1、7.2，记录精确失败点，`test: expose the desktop rewriting the account's flip setting`。
3. **实现**：`fix: keep the account's flip setting on the desktop`，改 `useReaderMode.js`、三个组合函数、`stores/reader.js` 与 `Reader.vue`。
4. **回归**：按 7.3 执行。
5. **记录**：提交号、结果和状态标签写回本文件、gap analysis、matrix；晚到设置合同第 10 节注明观察 1、2 已由本切片关闭。
6. **发布**：实现提交改了 `frontend/**`，合入 main 会触发镜像发布；本分支包含未合入 main 的晚到设置切片，是否合入由协调方决定。
