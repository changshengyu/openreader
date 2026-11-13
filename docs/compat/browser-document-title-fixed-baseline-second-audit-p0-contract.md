# 浏览器标签页标题（document.title）合同盘点（P0）

状态：第 7.1 节（Reader）为 **implementation-complete / regression-validated**（红测 `377e8b8`、实现 `4a0aae8`，
记录见第 12 节，只在本地，未发布）；第 7.2、7.3 节仍是 **contract-only**，标题文字要等用户先做第 6 节的决定 D1。
下面第 1–11 节是合同提交 `c8ebd30`（在 `claude/xenodochial-carson-5004c3` 上 cherry-pick 为 `c75e730`）的原文，
写作时的状态是：本切片只写文档；产品代码、单元合同和浏览器合同都没有改。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。约定路径
`/private/tmp/changshengyu-reader-dev` 在本会话沙箱里不可写，上游克隆到会话 scratchpad 后 checkout 同一提交。  
当前基线：`OpenReader@4092ea2`（`claude/nervous-saha-c99e83`）；`git diff 186c762 4092ea2 -- frontend backend`
为空，前后端产品代码与已发布的 `186c762` 相同。

## 1. 范围

- 对象：顶层文档的 `document.title`，即浏览器标签页、窗口切换器、历史记录和书签默认名称里显示的字符串。
- 覆盖所有可到达的前端状态：上游的两条 hash 路由和独立静态页；OpenReader 的全部 history 路由、兼容重定向与别名、
  `App.vue` 的根状态（登录过渡、认证阻断）。
- 不含：EPUB iframe 内部文档的 `<title>`（不改变顶层标签页）；后端 EPUB 导出 `epubNavXHTML`、`epubChapterXHTML`
  写的 `<title>`（导出文件内容）；PWA manifest 的 `name/short_name` 与 `apple-mobile-web-app-title`（主屏应用名，
  不是 `document.title`，见第 11 节）。
- 起因：[桌面运行标注合同](reader-desktop-running-header-fixed-baseline-second-audit-p0-contract.md)第 5 节最后一行
  把这一项留给另行盘点；[运行标题合同](reader-running-chapter-header-device-feedback-p0-contract.md)第 5 条“书名仍由
  浏览器标题提供”与当前代码不符。

## 2. 上游证据（fa22f271）

### 2.1 写入点

在整个上游仓库里 `grep -rn "document\.title"`，只有 2 个文件共 6 行；`web/` 下的 `<title>` 标签共 3 个（服务端
`src/main/resources/epub/*.html` 和 `epublib` 里的 `<title>` 是 EPUB 导出模板，不是页面标题）。没有别的写标题方式
（没有 `querySelector('title')`，也没有 vue-meta 之类的库）。

| 位置 | 时机 | 值 |
| --- | --- | --- |
| `web/public/index.html` L24 | 首屏，脚本运行前 | `阅读` |
| `web/src/views/Index.vue` L1188–1189 `mounted` | Index 首次挂载 | `阅读` |
| 同文件 L1207–1208 `activated` | 每次回到 Index | `阅读` |
| `web/src/views/Reader.vue` L335–343 `activated` | 每次进入 Reader，紧跟在 `this.init()` 之后 | `this.title` 非空时为 `readingBook.name + " - " + this.title`，否则为 `readingBook.name` |
| 同文件 L410–416 `watch.title` | `this.title` 变化 | 同上 |
| `web/public/bookSourceDebug/index.html` L6 | 独立静态页 | `Reader书源编辑器`；页面脚本不改标题 |
| `web/public/browsertest.html` L7 | 独立诊断页 | `ES5测试`；仓库里没有任何入口引用它，不在可见流程里 |

### 2.2 路由、keep-alive 与恢复

- `web/src/router/index.js` L12–25 只有 `/`（Index）和 `/reader`（Reader）两条路由；L28 `mode: "history"` 被注释，
  用的是 hash 模式。
- `web/src/App.vue` L3–5 是不带 include/exclude 的 `<keep-alive><router-view>`。两个视图都会缓存，切换时触发
  `activated`/`deactivated`。
- 登录不是路由。`App.vue` L6 的根级 `el-dialog`（`showLogin`）盖在当前视图上，不改标题：登录框在 Index 上时标题是
  `阅读`，在 Reader 上时仍是 `书名 - 章节名`。
- Reader `deactivated`（L372–383）不恢复标题。离开 Reader 只能回到 Index（`toShelf()` L1400–1402
  `$router.push("/")`，或浏览器后退），或者离开整个页面；回到 Index 时由 Index `activated` 设为 `阅读`。上游的
  “离开 Reader 后恢复”是由 Index 完成的。
- 书源调试：Index L220–227 的“调试书源”调用 `debugBookSource()`（L2243–2250），执行
  `window.open(origin + pathname + "bookSourceDebug/#domain=" + api, "_target")`，在命名窗口里打开独立静态页。
  该页标题是 `Reader书源编辑器`，L15 的 `←主页` 链回 `../index.html`。

### 2.3 Reader 标题里的书名和章节名

书名是 `readingBook.name`。`readingBook` getter（`web/src/plugins/vuex.js` L627–634）总是返回对象
`{...state.readingBook, ...同 bookUrl 的书架书}`，state 默认值是 `{}`（L27）。从书架进入时，`toDetail`
（Index L1477–1501）先 `setReadingBook({ name, ... })` 再 push `/reader`；直接打开时，`App.vue` `beforeCreate`
（L256–257）先执行 `syncFromLocalStorage`，从 `<用户>@readingRecent` 恢复（vuex.js L637–657）。所以进入 Reader
时书名已经同步可用。

章节名是 `this.title`，写入点如下：

| 位置 | 写入 |
| --- | --- |
| `init()` L870–912（L878） | 首次进入或换了书：`""`，标签页只剩书名 |
| `loadCatalog()` L946–948 | 初始化时目录加载失败：`""` |
| `getContent(index)` L1038–1040 | 目标章的目录标题 `catalog[index].title`。写在请求正文（L1045）**之前**，正文失败也保留；目录里没有该行时（L1023–1036）直接返回，不写 |
| `epubLocationChangeHandler()` L1684–1710（L1707） | EPUB 内链跳到其他章：该章目录标题 |
| `saveReadingPosition()` L2488 起（L2528） | 连续滚动中可见段落所属章节变化：该章目录标题。`scrollHandler` 在 L2440 以 100ms 防抖调用它 |
| `watch.chineseFont` L500–504 | 切换简繁：`this.title = this.filterContent(this.title)`（替换规则加简繁转换），下一次写入之前一直是转换后的值 |

另外，`watch.chapterName`（L388–390）监听的属性不存在，是死代码；计算属性 `formatedTitle`（L862–864）没有被使用。

### 2.4 上游可见行为

1. 首屏和 Index：`阅读`。
2. Reader：`书名 - 章节名`；章节名为空（刚进入、目录加载失败）时只有 `书名`。章节名是原始目录标题：不带
   `第 N 章` 序号，不经过替换规则或简繁转换（只有切换简繁后、下一次切章前例外）。分隔符是空格、ASCII 连字符、空格。
3. 切章时标题立即更新：翻页、目录、书签和正文搜索跳转在导航开始时就换成目标章，正文加载失败也不回退；连续滚动在
   可见章节变化后约 100ms 更新；EPUB 内链跳章时更新。文本、EPUB、音频、漫画规则相同。
4. 离开 Reader（回书架、后退）后为 `阅读`。
5. 登录框不改标题。
6. 书源调试在另一个标签页，标题 `Reader书源编辑器`，主标签页保持 `阅读`。
7. 上游缺陷（判定见第 5 节）：
   - 全新浏览器没有 `readingRecent` 时直接打开 `#/reader`，书名是 `undefined`，标题变成字符串 `undefined`；
   - 只在 `this.title` 变化时写标题。在 Reader 里用书架换书（`changeBook` L913–919 不重置 `this.title`），如果新旧
     当前章节名相同，或者书名被修改，标题会保留旧书名；
   - 章节名只有空白时 `if (title)` 为真，标题是 `书名 - ` 加空白（读取 `document.title` 时折叠成 `书名 -`）。

## 3. 当前 OpenReader（4092ea2）

- `frontend/src` 里没有任何代码写 `document.title`（`grep -rn "document\.title" frontend/src` 无结果）。整个仓库
  只有 `scripts/smoke/openreader-smoke.mjs:47` 读取它并打印，没有断言。
- 因此所有路由、所有状态的标签页都是 `frontend/index.html:13` 的 `OpenReader`：

| OpenReader 状态 | 路由/位置 | 上游对应 | 上游标题 | 当前标题 |
| --- | --- | --- | --- | --- |
| 首屏 | `index.html:13` | `public/index.html` | `阅读` | `OpenReader` |
| 首页工作台 | `/`（`router/index.js:52`），包括 overlay、workspace、bookInfo 等 query | Index | `阅读` | `OpenReader` |
| 兼容重定向 | `/search`、`/discover`、`/local-store`、`/sources`（非 debug）、`/settings`、`/books/:id`（`:54–98`、`:105–120`），都落到 `/` | Index | `阅读` | `OpenReader` |
| 阅读 | `/books/:id/read`（`:121`） | Reader | `书名 - 章节名` | `OpenReader` |
| 远程临时阅读 | `/reader/remote/:sessionId`（`:122`） | Reader（`?search=1`，直接阅读搜索结果） | `书名 - 章节名` | `OpenReader` |
| 登录页 | `/login`（`:53`、`:126–138`） | 盖在 Index 上的登录框 | `阅读` | `OpenReader` |
| 登录过渡态 | `App.vue:22–28`（“正在恢复当前账号…”“正在进入登录页面…”） | 同上 | `阅读` | `OpenReader` |
| 认证阻断 | `App.vue:9–11`（Reader 被 `v-if` 卸载，显示“正在等待重新登录…”） | 盖在 keep-alive Reader 上的登录框 | `书名 - 章节名` | `OpenReader` |
| 书源调试 | `/source-debug`，别名 `/bookSourceDebug`、`/bookSourceDebug/`，以及 `/sources?action=debug`（`:86–88`、`:99–104`）；`AppLayout.vue:725–728` 用 `window.open(..., '_target')` 打开 | `bookSourceDebug/index.html` | `Reader书源编辑器` | `OpenReader` |

- `App.vue` 没有 keep-alive。Reader 以 `:key="readerSessionKey"`（L5）挂载，离开即卸载；认证阻断时被 `v-if` 卸载
  （L4–11）；`watch(isLoggedIn)`（L103–120）在退出登录后 `router.replace` 到登录页。
- 同一个 Reader 实例里可以换书：`useReaderShelf.js:62–73` 用 `router.push` 进入另一本书的 `/books/:id/read`，
  路由组件被复用，`useReaderRouteSync.js:18–25` 监听 `bookId` 重新加载。
- Reader 状态（第 7.1 节派生规则的依据）：
  - `Reader.vue:543–547`：`book`、`chapters`（目录）、`chapter`（已提交正文的章节）、`currentIndex`。书名字段是
    `book.title`，对应上游 `readingBook.name`；目录行的 `title` 对应上游 `catalog[index].title`。
  - `useReaderChapterLoader.js:24` 在请求正文之前就把 `currentIndex` 设为目标章；`:58` 正文返回后才提交 `chapter`；
    失败分支（`:128–134`）不改 `chapter`。
  - 连续滚动跨章 `useReaderChapterWindow.js:206–212`、已加载窗口内跳转 `useReaderNavigation.js:106–116`、正文搜索
    跳转 `useReaderSearchNavigation.js:69–70`，都同时提交 `currentIndex` 和 `chapter`；EPUB 内链
    `Reader.vue:2156–2165` 走同一个 `loadChapter` 事务。
  - 书架书：`useReaderBookLoad.js:115` 在第一个 `await` 之前就用书架对象设置 `book`，目录（`:136`）和
    `currentIndex`（`:152`/`:154`）要等之后的 `await` 才更新。远程临时阅读在 `:93–94` 同步替换 `book` 和 `chapters`。
  - 目录行带 `bookId`（`backend/models/models.go:411–415`）。
- 其他可见名称（第 4 节的证据）：侧栏品牌 `AppLayout.vue:14–23` 是 `阅读` + `清风不识字，何故乱翻书`，与上游
  Index L22–30 相同；`Login.vue:4` 页面标题是 `OpenReader`；`SourceDebug.vue:5` 页内标题是 `书源调试`；
  `manifest.webmanifest` 的 `name/short_name` 和 `index.html:10` 的 `apple-mobile-web-app-title` 是 `OpenReader`
  （上游 `public/manifest.json` 与 `vue.config.js` L91–92 `pwa.name` 是 `阅读`）。

## 4. 来源与用户要求的证据

| 提交 | 日期 | 内容 |
| --- | --- | --- |
| `131d3f8`（另一个初始提交 `e0207e7` 相同） | 2026-05-18 | 初始提交：`<title>OpenReader</title>`、登录页 `OpenReader`、侧栏 `OpenReader / 自部署阅读空间`。重写初期的自有命名，没有用户要求的记录。 |
| `bd8afe5` | 2026-05-19 | “Use upstream sidebar title”：侧栏改成上游的 `阅读 / 清风不识字，何故乱翻书`。只改了侧栏，`<title>` 和登录页没动；提交信息没有引用用户要求。 |
| `f0e17c7` | 2026-06-30 | “Add portrait web app manifest”：新增 manifest 和 `apple-mobile-web-app-title`，名称都用 `OpenReader`。`docs/upstream-alignment-audit.md` L153 说目的是对齐 fork 的竖屏锁定，没有讨论名称。 |
| `9b96c50`、`e1631d0` | 2026-09-13/14 | 运行标题合同第 5 条：“书名仍由浏览器标题、工具层和书架面板提供”。当时没有任何代码设置 `document.title`，这句话不成立。 |
| `4092ea2` | 2026-10-16 | 桌面运行标注合同第 5 节：Reader 标签页标题记为 `must-fix`，Index 用 `阅读` 还是产品名记为 `unknown`，另行盘点（即本文件）。 |

检索范围：

- `git log --all` 的 pickaxe：`<title>OpenReader</title>`、`apple-mobile-web-app-title`、侧栏副标题、登录页标题；
  `--grep`（不区分大小写）：`document.title`、`tab title`、`page title`、`标签页`、`浏览器标题`、`产品名`、`品牌`、
  `brand`、`rename app`；
- `docs/` 下全部文档和 README：`产品名`、`品牌`、`改名`、`应用名`、`站点名`、`标签页`、`浏览器标题`、`页面标题`、
  `标题栏`、`document.title`、`<title>`、`阅读3`；
- 交接文档 `claude-code-refactor-handoff-2026-10-16.md`、主检出的 `CLAUDE.md`、`AGENTS.md`、`.agents/`，以及
  `/private/tmp/openreader-*-handoff.md`。

结果：

- Reader 标签页标题没有任何允许差异的记录，运行标题合同反而假定它已经存在。
- 产品名的取舍找不到用户决定。仓库里两种信号并存：发行名（仓库 `changshengyu/openreader`、镜像、`<title>`、
  登录页、manifest）用 `OpenReader`；应用内唯一的品牌位置（侧栏）在 `bd8afe5` 主动改回上游的 `阅读`。两边都没有
  用户要求的记录，不能据此替用户决定。
- 局限：原始对话不在仓库里，本盘点没有查阅，结论以仓库里的记录为准。

## 5. 差异分类

| # | 项目 | 上游 | 当前 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | Reader 标题：书架书和远程临时阅读，所有书籍类型 | `书名 - 章节名`；章节名为空时 `书名` | `OpenReader` | **`must-fix`**：上游可见行为，不属于任何允许差异（运行时适配、多用户隔离、缓存加固、安全修复、用户要求的 UX 改进） |
| 2 | 切章时更新（翻页、目录、书签/搜索跳转、进度恢复、连续滚动跨章、EPUB 内链、自动翻章） | 导航开始时换成目标章的目录标题，失败不回退 | 不变 | **`must-fix`**，随第 1 项 |
| 3 | 离开 Reader 后恢复 | 回到 Index 时由 Index 设为 `阅读` | 从未改过，谈不上恢复 | **`must-fix`**，随第 1 项：Reader 卸载时恢复为壳标题。恢复成哪个字符串取决于 D1 |
| 4 | 首页工作台、兼容重定向、所有 overlay 和 workspace 状态 | `阅读` | `OpenReader` | **`unknown`**：需要用户决定 D1（第 6 节） |
| 5 | 首屏静态 `<title>` | `阅读` | `OpenReader` | **`unknown`**：同 D1 |
| 6 | 登录页 `/login` 与登录过渡态 | 没有登录路由；登录框盖在 Index 上，标题 `阅读` | `OpenReader` | 结构上跟随壳标题，不另设登录标题（独立登录路由本身是已签收的 JWT 多用户适配，不重开）。文字 **`unknown`**：同 D1 |
| 7 | 认证阻断（Reader 被卸载） | 登录框盖在 keep-alive 的 Reader 上，标题仍是 `书名 - 章节名` | `OpenReader` | **`acceptable-change`**（多用户隔离）：[重新认证隔离合同](reader-reauthentication-isolation-p0-contract.md)要求阻断面不含书名、正文或进度，标签页标题也是可见内容，阻断时恢复为壳标题；同账号重新登录后由新的 Reader 重新设置 |
| 8 | 书源调试标签页 | 独立页面，标题 `Reader书源编辑器`，与主标签页可区分 | `OpenReader`，与主标签页无法区分 | 有独立标题：**`must-fix`**（[书源调试合同](source-debug-fixed-baseline-second-audit-p2-contract.md)恢复了独立工作区和命名窗口，但没有对照标题）。文字 **`unknown`**：上游字符串含上游产品名 `Reader`，同 D1 |
| 9 | 书名未知（刚进入、书籍加载失败、远程会话失效） | 全新浏览器直接打开时为 `undefined` | — | **`acceptable-change`**（运行时适配）：上游进入 Reader 时书名已从 `readingRecent` 同步可用；OpenReader 按 `/books/:id/read` 异步加载，书名未知时不写标题、保留前一个标题，不复现 `undefined` |
| 10 | 书名变化后的陈旧标题（换书时章节名相同、改书名） | 只在 `this.title` 变化时写，保留旧书名 | — | **`acceptable-change`**（运行时适配）：这是上游 Vue 2 命令式 watcher 的副作用；OpenReader 从当前书和当前章派生标题，书名一变就更新，不复现陈旧值 |
| 11 | 简繁切换后的临时转换 | 切换后、下一次切章前，章节名经过替换规则和简繁转换 | — | **`acceptable-change`**（运行时适配）：上游把同一个可变的 `title` 同时用作标签页、mini 顶栏和正文 `<h3>` 的来源，切换时 watcher 把过滤后的值写回这个共享变量；OpenReader 的显示转换是只读派生（`useReaderChapterPresentation`），没有对应物。稳定状态（原始目录标题）保持一致 |
| 12 | 只有空白的章节名 | `书名 -` | — | **`acceptable-change`**：按空章节名处理，只显示 `书名`，与运行标题的 `trim()` 一致 |

没有一项判为 `intentional-redesign`：仓库里没有任何文档把“标签页始终显示产品名”作为有意的重新设计提出过，它只是
初始 `index.html` 的默认值。

## 6. 需要用户决定：D1 标签页里的产品名

D1 只影响第 5 节第 3 项（恢复成的字符串）和第 4–8 项的文字。Reader 标题本身不含产品名，不受影响。

| 状态 | A：上游原样 | B：保留 OpenReader 品牌 |
| --- | --- | --- |
| 首屏 `<title>`、首页、兼容重定向、登录页、登录过渡态、认证阻断、离开 Reader 后（下文统称“壳标题”） | `阅读` | `OpenReader` |
| 书源调试 | `Reader书源编辑器` | `OpenReader书源编辑器`（按上游 `<产品名>书源编辑器` 的格式替换） |

- 选 A 的理由：CLAUDE.md 规定上游可见行为是产品合同，允许差异只有列出的几类，产品名不在其中；应用内的侧栏品牌已在
  `bd8afe5` 改回 `阅读`，选 A 后标签页与侧栏一致。
- 选 B 的理由：仓库、镜像和登录页都叫 OpenReader，用户可能希望在多个标签页里看到发行名。选 B 需要用户明确要求，并
  记为“用户要求的差异”。
- 用户也可以给出别的字符串（C）。
- 本盘点的建议：没有用户要求时，按项目纪律默认选 A。但产品名是用户可以合理选择的品牌问题，发行物也已经叫
  OpenReader，所以记为 `unknown` 等用户确认，不替用户决定。
- 不论选哪项，本合同都不改 PWA manifest 的 `name/short_name` 和 `apple-mobile-web-app-title`；选 A 时是否同步修改，
  另行决定。

## 7. 目标合同

### 7.1 Reader（`reader`、`remote-reader` 路由；不依赖 D1）

R1. 标题为 `书名 - 章节名`；章节名为空时只有 `书名`。
  - 书名：当前书的 `book.title`（远程临时阅读取会话里的 `book.title`），原样使用。
  - 章节名：当前索引对应的目录行标题 `chapters[currentIndex].title`，去掉首尾空白后使用。为空、只有空白，或者目录里
    没有这一行时，视为空。
  - 分隔符恰好是空格、ASCII 连字符 `-`、空格。不加 `第 N 章` 序号（序号是运行标题里用户要求的补充，不属于上游
    标签页标题），不做简繁转换，不套用替换规则，不加产品名后缀。
  - 适用于所有书籍类型：文本、EPUB、音频、漫画。运行标题对音频和 CBZ 不渲染，标签页标题照样设置。

R2. 切章时立即更新：只要 `currentIndex` 指向新章就更新。覆盖点击和按键翻页、滑动、目录、书签、正文搜索、路由
  `chapter` 参数、进度恢复、自动翻页、朗读和音频自动进入下一章、EPUB 内链（`navigateEpubChapterFromLocation`）、
  连续滚动跨章（`syncCurrentChapter`）和已加载窗口内跳转（`jumpToLoadedChapter`）。与上游一样，在加载开始时就换成
  目标章；正文加载失败时保持目标章，不退回旧章。被取代或中止的加载不能让标题停在更早的章节。

R3. 书名和章节名必须属于同一本书。Reader 内换书时，中间状态只允许保留旧书的完整标题，或者只显示新书名；不允许出现
  “新书名 - 旧书的章节名”或“旧书名 - 新书的章节名”。换书完成后为新书的 `书名 - 章节名`，即使新旧章节名字符串相同。
  书名被修改（例如书籍信息编辑或同步事件更新了 `book.title`）时，标题也随之更新。

R4. 书名未知（书籍尚未加载、加载失败、远程会话失效）时不写标题，保留当前值。任何时候都不能出现 `undefined`、
  `null`、空字符串或以 ` - ` 开头的标题。

R5. 离开 Reader：Reader 卸载时（回书架 `navigateHome`、浏览器后退、书被删除后回首页、退出登录后跳转登录页、
  认证阻断的 `v-if` 卸载、会话 generation 变化后重新挂载），在同一次卸载里把标题恢复为壳标题，不依赖下一个视图。
  新的 Reader 实例按 R1–R4 重新设置。

R6. 打开或关闭 Reader 里的面板（目录、设置、书架、换源、书籍信息、书签、正文搜索、全局 overlay）不改标题。

### 7.2 首页、静态壳、登录与中性状态（文字依赖 D1）

S1. `frontend/index.html` 的 `<title>` 为壳标题。

S2. 首页 `/`、所有落到首页的兼容重定向，以及打开或关闭 overlay、切换 workspace 时，标题都是壳标题。首页挂载时主动
  设置一次（对应上游 Index 的 `mounted`/`activated`），这样从改过标题的视图回来时也能恢复。

S3. `/login`、登录过渡态、认证阻断面，以及盖在首页上的认证对话框：壳标题。

### 7.3 书源调试（文字依赖 D1）

T1. `source-debug` 路由（含别名和 `/sources?action=debug` 跳转）挂载时，把标题设为书源调试标题；调试过程中、切换
  右侧标签或 hash `tab` 时不变。在同一个标签页里卸载（例如会话失效后跳转登录页）时恢复为壳标题。打开它的主标签页
  保持原标题。

### 7.4 约束

- 只有一个写入点：所有写入都经过一个小工具模块（建议 `frontend/src/utils/documentTitle.js`），其他文件不直接给
  `document.title` 赋值，便于测试禁止分散写入。`typeof document === 'undefined'` 时工具什么都不做，`node --test`
  可以直接导入。
- 不新增依赖（vue-meta、unhead、@vueuse 等）、浏览器存储 key、API、数据库字段或持久文件；后端不改。
- 不改运行标题、侧栏品牌、登录页正文标题和 manifest。
- 建议的实现形状（不强制）：纯函数 `formatReaderDocumentTitle(bookName, chapterName)`；组合函数
  `useReaderDocumentTitle({ book, chapters, currentIndex, setTitle, shellTitle })` 在 Reader 的 setup 里调用，用
  `watch` 写入、`onScopeDispose` 恢复。R3 的机制由实现阶段决定（例如比较目录行的 `bookId` 和 `book.id`，或者在加载
  事务里同时提交书名和目录归属），但必须由第 8.1 节的单元合同固定下来。

## 8. 测试计划（后续阶段执行，本切片不写测试）

D1 决定之前，测试里的壳标题一律从 `frontend/index.html` 的 `<title>` 读取，不写死字符串。这样 Reader 部分的红测和
实现不必等 D1；D1 落地时只改 `index.html` 和常量，测试自动跟随。

### 8.1 单元合同（`node --test`）

1. `frontend/tests/documentTitle.test.mjs`（新增）：
   - `formatReaderDocumentTitle('斗破苍穹', '第一章 陨落的天才') === '斗破苍穹 - 第一章 陨落的天才'`；
   - 章节名为 `''`、`null`、`'   '` 时为 `'斗破苍穹'`；
   - 不加序号：`formatReaderDocumentTitle('书', '第二章')` 恰好等于 `'书 - 第二章'`；
   - 书名为 `''`、`null`、`undefined` 时返回空值，表示“不写”；
   - 壳标题常量等于 `frontend/index.html` 里 `<title>` 的文本，防止两处漂移。
2. `frontend/tests/readerDocumentTitle.test.mjs`（新增，用 `effectScope` + `ref`，`setTitle` 换成记录调用的假函数）：
   - 书已知、目录为空 → `书A`；目录加载完、`currentIndex = 0` → `书A - 第一章`；
   - 只改 `currentIndex = 1`、不改 `chapter`（模拟加载开始或正文失败）→ 立即变为 `书A - 第二章`；
   - 空标题章 → `书A`；只有空白的标题 → `书A`；
   - 按 `useReaderBookLoad` 的真实顺序换书：先只把 `book` 换成书B（目录仍是书A的）→ 记录里不得出现
     `书B - <书A的章节名>`；再换目录和 `currentIndex` → `书B - 第一章`，书A的当前章也叫 `第一章` 时同样成立；
   - `book` 为 `null` → 不调用 `setTitle`；
   - 修改书名 → 标题跟随；
   - `scope.stop()` → 最后一次调用写入壳标题（R5）。
3. `frontend/tests/documentTitleWiringContract.test.mjs`（新增，源码合同）：
   - `Reader.vue` 用 `book`、`chapters`、`currentIndex` 调用 `useReaderDocumentTitle`；
   - 首页（`Home.vue` 或 `AppLayout.vue`）挂载时设置壳标题；`SourceDebug.vue` 挂载时设置书源调试标题，卸载时恢复；
   - 扫描 `frontend/src/**/*.{js,vue}`：给 `document.title` 赋值的代码只出现在 `utils/documentTitle.js`。
- 红测阶段的写法：在测试体内 `await import(...)` 新模块，把导入失败转成断言消息（例如
  “reader document title helper must exist”），让每条测试在当前实现上以明确的断言失败，而不是整个文件加载出错。
- 预期：当前实现上全部失败；实现后同一批断言转绿。

### 8.2 真实浏览器

1. 新增 `scripts/smoke/document-title-contract.mjs`。接口用 `page.route` 模拟（同 `reader-continuous-contract.mjs`），
   视口 1440×900、390×844、360×800。用 `addInitScript` 在 `document.head` 上安装 `MutationObserver`（childList、
   subtree、characterData），把每次变化后的 `document.title` 记入 `window.__titleLog`，用来检查中间状态。步骤：
   1. 打开 `/`：标题等于壳标题。打开再关闭一个 overlay（如 `/?overlay=sources`）、访问 `/search`：仍是壳标题。
   2. 进入 `/books/1/read?chapter=0`：等待 `标题契约测试 - 第一章 起点`；`__titleLog` 里没有 `undefined`、`null`、
      空串或以 ` - ` 开头的值。
   3. 用下一章控件（桌面 `ReaderDesktopProgress.vue:15`，移动端 `ReaderMobileChrome.vue:98`）：
      `标题契约测试 - 第二章 中途`。
   4. 用目录跳到第 5 章：`标题契约测试 - 第五章`。
   5. 跳到标题为空的章：`标题契约测试`。
   6. 第 4 章正文接口返回 500：错误提示可见，标题为 `标题契约测试 - 第四章 失败`（保留目标章，不退回）。
   7. 用 Reader 书架面板换到书 2，书 2 的目录接口延迟 800ms 返回：最终标题为 `标题契约测试二 - <书 2 当前章>`，
      `__titleLog` 里不得出现“书 2 的书名 + 书 1 的章节名”。再做一次新旧当前章同名（都是 `第一章 起点`）的换书，
      最终标题必须带新书名。
   8. 点工具层“返回书架”：URL 变为 `/` 后立即检查，标题为壳标题。
   9. 再次进入 Reader 后 `page.goBack()`：壳标题。
   10. 正文请求返回 401：出现阻断面后，标题为壳标题且不含书名；同账号重新登录后恢复为 `标题契约测试 - …`。
   11. 清除 token 后打开 `/login`：壳标题。`/source-debug`、`/bookSourceDebug/`、`/sources?action=debug`：书源调试
       标题。从首页点“调试书源”，用 `context.waitForEvent('page')` 拿到新标签页：新标签页是书源调试标题，原标签页
       仍是壳标题。
   12. `/reader/remote/<id>`（模拟会话）：`远程临时书 - 第一章`。
   - 预期红点：Reader 步骤（2–10、12）在第一次等待标题处超时，因为标题一直是 `OpenReader`；第 11 步的书源调试标题
     失败；第 1、11 步的壳标题在 D1 选 B 时通过，选 A 时失败。逐视口记录精确失败点。
2. `scripts/smoke/reader-continuous-contract.mjs`（模拟接口，4 个视口 × scroll/scroll2）：在 L152 起的 `initial`
   采样里加 `documentTitle: document.title`，断言等于 `连续跨章契约测试 - ${chapterTitle(0)}`；L264–265 跨到第 2 章
   后等待 `连续跨章契约测试 - 第 2 章`；L413–414 处等待 `… - 第 3 章`。夹具的章节名本身就是 `第 N 章`，运行标题显示
   `第 2 章 第 2 章`，标签页只能是 `… - 第 2 章`，顺带验证不加序号。
3. `scripts/smoke/reader-epub-contract.mjs`（真实 Go 服务，3 个视口）：L710–712 运行标题变为 `第 3 章 第二章` 之后，
   等待 `document.title === 'EPUB 浏览器契约 - 第二章'`（夹具 OPF 的 `dc:title` 在 L213）；L738 `page.goBack()` 回到
   `/` 之后，断言标题等于壳标题。注意 L96 的 `EPUB 浏览器契约 - OpenReader` 是书签表格里的“书名 - 作者”（作者
   `dc:creator` 是 `OpenReader`，L214），与标签页标题无关。
4. `scripts/smoke/reader-reauthentication-contract.mjs`（模拟接口）：`openReader` 之后标题为
   `用户 A 的私有阅读书籍 - 第一章`（夹具目录行 L139 是 `第一章`，正文接口 L146 返回的章节名是 `第1章`，标签页必须用
   目录标题）；`invalidateReader` 出现阻断面后，标题不含 `用户 A 的私有阅读书籍`，且等于壳标题；`assertSameAccount`
   重新登录后恢复为 `用户 A 的私有阅读书籍 - 第一章`；`assertDifferentAccount` 和 `assertExplicitLogout` 在首页和
   登录页都是壳标题，不含 A 的书名。
5. `scripts/smoke/source-debug-contract.mjs`（真实 Go 服务）：L229–231 跳到 `/source-debug` 之后，断言书源调试标题。
6. 可选：`scripts/smoke/openreader-smoke.mjs` 对 `/` 断言 `result.title` 等于壳标题；提供 `SMOKE_READER_URL` 时，
   断言标题以书名开头。

### 8.3 回归与临时变异（实现阶段）

- frontend `npm test`（把新增的测试条数写进记录）、`npm run build`。
- 真实浏览器：8.2 的全部脚本；另跑 `reader-mobile-contract`、`reader-text-modes-contract`、`index-workspace-contract`
  作为对照，确认面板和工作台行为不受影响。
- 临时变异只在工作区修改并构建到独立目录，构建后立即还原，不提交：
  - M1：Reader 不写标题 → 8.1 第 2 项和所有 Reader 浏览器等待失败；
  - M2：Reader 卸载时不恢复，只靠首页挂载设置 → 认证阻断和退出到登录页的断言失败（标题还带书名），证明 R5 不能只靠
    首页；
  - M3：章节名改用已提交的 `chapter.title` → 8.1 第 2 项“只改 `currentIndex`”、8.2 第 1 项第 6 步（失败章）和重新
    认证夹具（`第1章` 与 `第一章` 不同）失败；
  - M4：加上 `第 N 章` 序号 → 8.1 第 1 项和连续阅读合同的精确相等断言失败；
  - M5：去掉同书校验 → 8.1 第 2 项的换书顺序和 8.2 第 1 项第 7 步（目录延迟返回）失败。
- 跳过并写明原因：Go `test`/`vet`/`race`（后端不改）；依赖 advisory、Compose 与卷门（依赖、Dockerfile、数据格式都
  不改）。
- 会话沙箱禁止 loopback 监听，服务和 Playwright 需在沙箱外运行（见 EPUB 移动端章节计数合同第 9 节）。

## 9. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交。
2. **用户决定 D1**：只影响 7.2、7.3 的文字；Reader 部分不用等。
3. **Reader 红测**：8.1 第 1、2 项和第 3 项的 Reader 部分；8.2 第 1 项的 Reader 步骤和第 2–4 项。壳标题从
   `index.html` 读取。提交 `test: expose missing reader document title`。
4. **Reader 实现**：新增工具模块和组合函数，`Reader.vue` 接入，同一批断言转绿。提交
   `fix: set reader document title from book and chapter`。
5. **壳标题与书源调试**（D1 之后）：先红测（8.1 第 3 项其余部分、8.2 第 1 项第 1 和 11 步、第 5 项），再改
   `index.html`、首页和 `SourceDebug.vue`。
6. **回归**：按 8.3 执行，包括临时变异。
7. **记录**：提交号、各视口结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
8. **发布**：实现提交改动 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；是否合入由
   用户决定。

## 10. 观察（不在本切片）

- 运行标题的章节名来自已提交的 `chapter.title`（`Reader.vue:1426`），序号来自 `currentIndex`。按
  `useReaderChapterLoader.js:24`、`:58`、`:128–134`，加载过程中会短暂显示“新序号 + 旧章节名”，正文加载失败时会一直
  停在这个状态。上游 mini 顶栏和标签页共用 `this.title`，导航开始时就换成目标章。这是读代码得出的推断，没有在浏览器
  里复现；它属于运行标题合同第 3 条的范围，应另行对照上游盘点，本切片不重开。

## 11. 不重开

- 运行标题 `第 N 章 章节名` 及其 1-based 序号（用户要求）、桌面计数的裁决（`4092ea2`）、侧栏品牌 `阅读`、登录页
  正文标题、独立登录路由（JWT 多用户适配）、认证阻断状态机（重新认证隔离合同）、书源调试工作区本身（书源调试合同）、
  history 路由白名单（[SPA 路由合同](frontend-static-spa-route-boundary-fixed-baseline-second-audit-p2-contract.md)）。
- 本合同不改 manifest 的 `name/short_name` 和 `apple-mobile-web-app-title`。
- API、数据库、浏览器存储 key、持久文件、镜像和生产：本切片都不改，不在本地构建或推送镜像，不做任何生产操作。

## 12. Reader 部分实施与验证（2026-10-17）

本节只覆盖第 7.1 节（Reader）和第 9 节的阶段 3、4、6、7。第 7.2、7.3 节（壳标题文字、首页主动设置、登录页与书源调试
标题）要等用户决定 D1，没有改；`index.html`、首页、`SourceDebug.vue` 都没动。

| 提交 | 阶段 | 内容 |
| --- | --- | --- |
| `c8ebd30`（cherry-pick 为 `c75e730`） | 合同 | 第 1–11 节、gap analysis 与 matrix。为了不和桌面计数切片并行改同一批文件，`claude/xenodochial-carson-5004c3` 先快进到桌面计数切片的 `9839270`，再 cherry-pick 合同提交；`claude/youthful-blackburn-9df7fb` 因此被本分支取代，不再单独合入。 |
| `377e8b8` | 红测 | 8.1 第 1、2 项和第 3 项的 Reader 部分；8.2 第 1 项的 Reader 步骤和第 2–4 项。 |
| `4a0aae8` | 实现 | 新增 `frontend/src/utils/documentTitle.js`（唯一写入点 `setDocumentTitle`、壳标题常量 `SHELL_DOCUMENT_TITLE`、纯函数 `formatReaderDocumentTitle`）和 `frontend/src/composables/useReaderDocumentTitle.js`；`Reader.vue` 只加一行 import 和一行调用。没有改后端、API、浏览器存储 key、依赖和 `index.html`。 |

**实现要点**

- 标题只由 `book`、`chapters`、`currentIndex` 派生，用默认 pre flush 的 `watch`：同一次同步更新里的中间值会合并，
  结果为空时不写。`onScopeDispose` 写回壳标题，所以回首页、浏览器后退、认证阻断的 `v-if` 卸载、会话 generation
  变化后重新挂载，都在 Reader 自己卸载时恢复（R5），不依赖下一个视图。
- 章节名取 `chapters[currentIndex].title` 去首尾空白，不读已提交的 `chapter`。`useReaderChapterLoader` 在请求正文
  之前就把 `currentIndex` 设为目标章，所以加载开始即换、正文失败不回退（R2）。
- R3 的机制（第 7.4 节留给实现阶段决定）：比较目录行的 `bookId` 和 `book.id`。两者都是正整数且相等时才用目录行
  标题。远程临时会话不入库，接口把 `book.id` 和行的 `bookId` 都序列化为 0，两边都是 0 时视为同一会话内的配对；
  一边有 id、一边没有（从远程会话换到书架书的中间态）视为不属于当前书。不属于当前书时只显示书名，与上游 `init()`
  清空 `this.title` 后的状态相同。书架换书的真实顺序、新旧当前章同名、远程行三条单元合同固定了这个规则。
- R4：书名为 `''`、`null`、`undefined` 或只有空白时不写，保留当前标题。只有空白的书名按未知处理，因为写出去就是
  以分隔符开头的标题（第 5 节第 12 项的同类情形）。书名本身不做其他处理。

**测试与第 8 节计划的差异**（都在 `377e8b8` 里）：

1. 8.1：`documentTitle.test.mjs` 6 条、`readerDocumentTitle.test.mjs` 9 条、`documentTitleWiringContract.test.mjs`
   2 条（Reader 以且只以 `book, chapters, currentIndex` 调用组合函数；扫描 `frontend/src/**/*.{js,vue}`，只有
   `utils/documentTitle.js` 给 `document.title` 赋值）。组合函数测试另外传入一个不变的已提交 `chapter`，模拟“目标章
   正文还在加载或已经失败”，实现不读它。首页和 `SourceDebug.vue` 的接线断言属于阶段 5，没有写。
2. 8.2 第 1 项（新增 `scripts/smoke/document-title-contract.mjs`）按视口分三个独立的浏览器 context：
   - navigation：第 2–9 步。第 5、6 步交错成“第五章 → 空标题章 → 失败章 → 只有空白的章”，每一步标题都会变化。
     第 3 步之后加了 R6 检查：开关目录和设置面板，标题不变，也没有任何写入。
   - 第 7 步没有用固定的 800ms 延迟，而是把目标书的目录接口挂起，在挂起期间检查：没有任何以“新书名 - ”开头的
     写入（目录到达之前，任何章节名都只能来自旧书），当前标题是旧的完整标题、新书名或壳标题之一；之后再放行，等待
     最终标题。书 2 的当前章 `序章 新书` 与书 1 的所有章节名都不同；同名换书是书 2 停在 `第一章 起点` 时换到当前章
     同为 `第一章 起点` 的书 3。
   - auth：第 10 步放在新的 context 里，401 只对没有预加载过的第 5 章（`chapter=4`）返回一次；在同一个 context 里，
     缓存过的章节会直接从浏览器缓存读取，不发请求。同账号重新登录后等待 `标题契约测试 - 第五章`。
   - remote：第 12 步；另加一次“远程会话 → 书架书 2”的挂起目录换书，验证没有 id 的会话行不会配上书架书名。
   - `__titleLog`：init script 运行时 `document.head` 还不存在，所以 `MutationObserver` 观察整个 `document`；另外包装
     `Document.prototype.title` 的 setter，同一个任务里被覆盖的写入也会被记录。
   - 第 1、11 步（首页 overlay、登录页和书源调试标题）属于阶段 5，等 D1 之后再写。
   - `SMOKE_VIEWPORTS` 和 `SMOKE_TITLE_FLOWS` 可以按视口和流程单独运行，逐视口记录不需要临时副本。
3. 夹具：连续阅读和重新认证合同的目录行补上 `bookId: 1`，与真实 `/books/:id/chapters` 一致（`models.Chapter.BookID`），
   R3 的机制依赖它。新增 `scripts/smoke/document-title.mjs`：从 `frontend/index.html` 读壳标题，并提供失败时报告实际
   标题的等待函数。
4. 8.2 第 3 项（EPUB）和第 4 项（重新认证）按计划加入。重新认证合同在首页、登录页、重新登录后的首页、换账号后的
   首页断言壳标题且不含 A 的书名；这几处在旧实现上本来就通过，是对照。认证阻断后的壳标题断言（`:292`）在旧实现上
   排在 `:214` 之后、跑不到，它是否有效由下面的 M2 证明。

**环境**：用本 worktree 构建的 Go 二进制加 `vite build` 生产前端。`git diff 5bc38a3 4a0aae8 -- backend` 为空，
所有代码状态共用同一个二进制。每个代码状态单独起一个服务，各用独立的临时 data/cache/library 和随机开发 JWT secret
（未输出）。`377e8b8` 与 `4a0aae8` 实际服务的构建，分别与在这两个提交上重新执行 `vite build` 的产物逐字节相同。
Go 1.26.9、Node v26.4.0、Playwright 1.61.1 / Chrome Headless Shell 1228。会话沙箱禁止 loopback 监听，服务、浏览器
合同和完整 `npm test` 都在沙箱外运行。红测的单元结果是用 `git archive 377e8b8` 解出的树（`node_modules` 用符号
链接）在沙箱外运行得到的。

**单元合同**与 frontend `npm test`：

| 代码状态 | 新增 17 条 | `npm test` |
| --- | --- | --- |
| `377e8b8`（红测，产品代码与 `c75e730` 相同） | 17 条全部失败，都是断言失败而不是文件加载错误：`documentTitle.test.mjs:11` 6 条（模块不存在）；`readerDocumentTitle.test.mjs:32` 9 条（模块不存在）；`documentTitleWiringContract.test.mjs:23`（`Reader.vue` 没有导入）、`:41`（写入者是 `[]`） | 788/805，失败的只有这 17 条 |
| `4a0aae8` | PASS | 805/805 |

**标签页标题浏览器合同**（`document-title-contract.mjs`，按视口、按流程单独运行）：

| 代码状态 | 流程 | 1440×900 | 390×844 | 360×800 |
| --- | --- | --- | --- | --- |
| `377e8b8` | navigation | FAIL `:367` 第 2 步等待 `标题契约测试 - 第一章 起点` 超时，实际是 `OpenReader` | 同左 | 同左 |
| `377e8b8` | auth | FAIL `:426` 第 10 步进入阅读时的同一等待，实际是 `OpenReader` | 同左 | 同左 |
| `377e8b8` | remote | FAIL `:455` 等待 `远程临时书 - 第一章`，实际是 `OpenReader` | 同左 | 同左 |
| `4a0aae8` | 三个流程 | PASS | PASS | PASS |

`4a0aae8` 上缺省参数的一次运行（三个视口、三个流程）也通过。

**其他 Reader 浏览器合同**（提交的脚本；红测阶段用于逐视口记录的临时副本见下文）：

| 合同 | `377e8b8` | `4a0aae8`（提交的脚本原样运行一次） |
| --- | --- | --- |
| EPUB（真实 Go 服务，`SMOKE_VIEWPORTS` 逐视口） | 1440×900、390×844、360×800 都 FAIL `:715`：跨文档链接到第二章后，等待 `EPUB 浏览器契约 - 第二章` 超时，实际是 `OpenReader` | PASS（三个视口；`:753` 后退到 `/` 后为壳标题） |
| 连续阅读（模拟接口） | 1440×900、1024×1366、390×844、360×800 的 scroll 与 scroll2 都 FAIL `:182`：`initial` 采样的标签页标题是 `OpenReader`；390×844 进度事务 FAIL `:423`：跨入第 3 章后等待 `连续跨章契约测试 - 第 3 章` 超时。提交的脚本原样运行停在 `1440/scroll :182` | PASS |
| 重新认证（模拟接口） | 1440×900、1024×1366、390×844、360×800 都 FAIL `:214`：`openReader` 等待 `用户 A 的私有阅读书籍 - 第一章` 超时（正文接口的章节名是 `第1章`，标签页必须用目录标题） | PASS（四个视口 × 显式退出、同账号、换账号） |

连续阅读和重新认证合同没有视口参数。为了逐视口记录红点，在会话 scratchpad 生成了临时副本，只改两处：两个辅助模块
改用绝对路径导入；`main()` 的视口和模式列表改为从环境变量读取。`main()` 之前的所有行都与提交的脚本相同，所以行号
一致。实现后的结论以提交的脚本原样运行为准。

**对照**（`4a0aae8`，提交的脚本原样运行）：`reader-mobile-contract`、`reader-text-modes-contract`、
`index-workspace-contract`（第 8.3 节列出的三项），以及 `remote-reader-contract`、`reader-source-switch-contract`、
`reader-audio-contract`、`reader-cbz-contract`、`openreader-smoke`，全部通过；`source-debug-contract` 也通过（运行
方式见下文“书源调试合同”）。

**音频与漫画**（R1“所有书籍类型”，补充观察，没有提交）：临时副本在 `reader-audio-contract` 里加了两处标签页标题等待
（夹具目录行补上 `bookId`），在 `reader-cbz-contract` 里加了一处。`4a0aae8` 上：音频为 `音频契约测试 - 第一集`，
手动和自动进入下一集后都是 `音频契约测试 - 第二集`；CBZ 为 `CBZ 浏览器契约 - pages/001.svg`（目录标题就是资源路径）。
`377e8b8` 上两者都在第一处等待时得到 `OpenReader`。运行标题对这两类书不渲染，标签页标题照样设置。

**服务日志**：红测和实现两个服务都没有 panic，除 WebSocket 外没有 4xx/5xx。401 全部来自 `/ws/sync`：模拟接口的
合同用假 token，而 `page.route` 拦不到 WebSocket。第 10 步的 401 和第 6 步的 500 是 `page.route` 返回的，不经过服务。

**临时变异**：每一项都只改工作区，先跑三个新增的单元文件，再构建到独立目录，随后立即 `git checkout -- frontend/src`
还原，确认工作树与 `4a0aae8` 一致，没有提交。每个变异单独起一个服务；标签页标题合同按视口、按流程单独运行，其他三个
合同原样运行一次（停在第一处失败）。

| 变异 | 单元（新增 17 条） | 标签页标题合同（三个视口结果相同） | 连续阅读 | EPUB | 重新认证 |
| --- | --- | --- | --- | --- | --- |
| M1：组合函数不写标题（保留卸载恢复） | 组合函数 9 条失败；纯函数和接线通过 | 三个流程都在第一处等待失败（`:367`、`:426`、`:455`），实际 `OpenReader` | FAIL `:182` | FAIL `:715` | FAIL `:214` |
| M1b：`Reader.vue` 不调用组合函数 | 只有接线 `:27` 失败 | 同 M1 | FAIL `:182` | FAIL `:715` | FAIL `:214` |
| M2：去掉卸载恢复，改由首页 `onMounted` 设壳标题 | 只有卸载恢复 `:249` 失败 | navigation PASS（回首页、后退都由首页设回）；auth FAIL `:435`：阻断面的标题仍是 `标题契约测试 - 第五章`；remote PASS | PASS | PASS | FAIL `:292`：认证阻断后标题仍是 `用户 A 的私有阅读书籍 - 第一章` |
| M3：章节名改用已提交的 `chapter.title`（`Reader.vue` 传入 `chapter`） | 10 条失败：接线 `:27` 和组合函数 9 条 | navigation FAIL `:382`（第 6 步）：失败章停在 `标题契约测试`，即上一个已提交的空标题章；auth PASS；remote FAIL `:347`：挂起目录期间写出 `标题契约测试二 - 第二章`（远程会话已提交的章） | PASS | PASS | FAIL `:214`：实际 `用户 A 的私有阅读书籍 - 第1章` |
| M4：章节名前加 `第 N 章` 序号 | 组合函数 9 条失败 | 三个流程都在第一处等待失败，实际 `标题契约测试 - 第 1 章 第一章 起点`、`远程临时书 - 第 1 章 第一章` | FAIL `:182`：`连续跨章契约测试 - 第 1 章 第 1 章 …` | FAIL `:715`：`EPUB 浏览器契约 - 第 3 章 第二章` | FAIL `:214`：`… - 第 1 章 第一章` |
| M5：去掉同书校验 | 3 条失败：`:149`（写出 `书B - 第二章`、`书B - 第一章`）、`:170`、`:194` | navigation FAIL `:347`（第 7 步）：挂起书 2 目录期间写出 `标题契约测试二 - 第一章 起点`；auth PASS；remote FAIL `:347`：写出 `标题契约测试二 - 第一章` | PASS | PASS | PASS |

- M1 按第 8.3 节的写法让组合函数不写，8.1 第 2 项失败；M1b 补充证明：只删掉 `Reader.vue` 里的调用，会被接线合同和
  所有浏览器等待拦住。
- M2 下 navigation 流程通过，正好说明第 8.3 节的判断：只靠首页挂载设回，回首页和后退看起来都正常；只有认证阻断
  （以及重新认证合同）能证明 R5 必须在 Reader 自己卸载时完成。第 8.3 节还预计“退出到登录页”的断言会失败，但重新
  认证合同是先整页打开首页再退出，标题由 `index.html` 重置，所以这一处在 M2 下仍通过；Reader 本身没有退出入口。
- M3 下连续阅读和 EPUB 通过，因为它们的正文响应和目录的章节名相同；失败章、重新认证夹具的 `第1章`、挂起目录的
  换书把它拦住。
- M4 的序号加在组合函数里（只有那里知道索引）；`formatReaderDocumentTitle` 本身拿不到索引，由 8.1 第 1 项的
  “不加序号”断言单独守住。
- M5 只被三条单元合同和挂起目录的换书检查拦住；连续阅读、EPUB 和重新认证合同都不换书，所以通过。
- 六个变异服务的日志都没有 panic。除 `/ws/sync` 外，只有 M2 服务上的两条 `POST /api/auth/register` 409，来自下面
  记录的书源调试合同端口冲突，与变异无关。

**书源调试合同**：`source-debug-contract.mjs` 不读 `TARGET_URL`，它自己用 `go run .` 在
`OPENREADER_SOURCE_DEBUG_PORT`（默认 18093）起后端，并服务 `frontend/dist`。第一次运行时该端口被 M2 变异服务占用，
脚本连到了 M2 服务，结果作废（上面那两条 409 就是重复注册）。停掉变异服务后，用 `4a0aae8` 构建的 `frontend/dist`
重跑，通过。

**其余门**：

- frontend `npm run build`：通过。警告与 `377e8b8` 上的构建相同：两条 `@vueuse/core` 的 `#__PURE__` 注释位置警告，
  一条“chunk 超过 500 kB”提示。
- 跳过并说明原因：Go `test`/`vet`/`race`（后端与 `5bc38a3` 无 diff）；依赖 advisory 门、Compose 与卷门（依赖、
  Dockerfile、工作流、数据格式都没改，卷门只在 Actions 上运行）。

**观察（不在本切片）**：书架换书时，如果新书目录还没返回，路由同步会先按旧目录加载目标章，正文加载完成后，附近章节
预加载也按旧目录的长度计算。新书章节更少时，会请求不存在的章节正文（夹具里得到 404）。这是第 7 步把目录挂起后才
暴露的，与标签页标题无关；夹具把书 3 设为 3 章避开了它，没有放宽失败收集。建议另行对照上游盘点。第 10 节关于运行
标题的观察仍然有效：标签页标题现在在加载开始时就换成目标章，运行标题的章节名仍要等正文提交。

**状态**：Git 为 `claude/xenodochial-carson-5004c3` 上的本地提交（`9839270` 之上：`c75e730`、`377e8b8`、`4a0aae8` 与
本记录），未推送、未合入 main。分支包含桌面计数切片和 EPUB 移动端计数切片的提交；`4a0aae8` 改了 `frontend/**`，
命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布，是否合入由用户决定。镜像：未构建、未推送。生产：
未访问、未操作。

## 13. 发布记录（2026-10-17）

经用户确认，`refs/heads/claude/xenodochial-carson-5004c3` 快进合入，main 由 `5bc38a3` 快进至
`2528d9c2f905f0098f98155aa733f9138f926052`（非 force，ls-remote 核验）。发布前本地门：Go 29 包与 vet、frontend
805/805 与 build、审计门测试 128/128、Go/npm 实际扫描、Compose；本地 Go 服务（临时数据）加生产前端跑 openreader-smoke
三视口、document-title 三视口 × navigation/auth/remote、EPUB 三视口、连续阅读、重新认证四视口、移动端（含 iPad）合同，
全部通过，访问日志无 5xx（401 只来自 mock 合同伪造 token 的 `/ws/sync`）。可信 Actions `39039046891` 成功（20m49s，
两平台 OS 门 pass-with-unknown-origins，新旧卷与 portable 备份通过）。`ghcr.io/changshengyu/openreader:2528d9c` 与
`latest` 指向 index `sha256:18b6c2992d9ef02bb89474186501afa8c9f72b3575bf60cfab1ba2d7ac9121fa`（amd64
`sha256:e47d2f6984e3075f0ef6ee4f9ba068ba9bdaf95eed2238424fbe54963a802906`，arm64
`sha256:bd61d51d650f10f58e08e82176bc2695743234a9dad7114df3f653cb93b7c090`）。按 digest 独立核验两平台：`/app/public`
63 个文件与本地同一提交的 build 逐字节一致，bundle 含 document.title 写入；zlib 1.3.2-r1、SQLite 3.51.3 与完整
revision 标签保持。生产未访问，实际运行版本未知。
状态：Reader 部分 **released / awaiting-device-verification**；首页、首屏、登录与书源调试标题仍 contract-only，等用户决定 D1。

## 14. D1 决定：产品显示名“闲读”（2026-11-12）

用户在第 6 节的 A（`阅读`）、B（`OpenReader`）之外选了 C：产品显示名为 **闲读**。2026-11-12 先请求“想个更好听的名字”，在候选
开卷、书舟、青灯、拾卷、闲读、书斋中选定“闲读”。这是品牌名的决定，所以范围比第 6 节的标签页文字更大，第 6 节末尾“不改 manifest”的约束由本节取代。

| 位置 | 现状 | 目标 | 分类 |
| --- | --- | --- | --- |
| `frontend/index.html` `<title>`（壳标题，S1） | `OpenReader` | `闲读` | `must-fix`（D1 已决定） |
| `utils/documentTitle.js` `SHELL_DOCUMENT_TITLE` | `OpenReader` | `闲读`（与 `<title>` 一致，已有单元合同） | `must-fix` |
| 首页工作区挂载（S2）与 `/login` 挂载（S3） | 只靠 `index.html` 初值和 Reader 卸载恢复 | 挂载时主动设壳标题 | `must-fix` |
| 书源调试（T1） | 壳标题 | 挂载设 `闲读书源编辑器`（上游 `Reader书源编辑器` 的 `<产品名>书源编辑器` 格式），卸载恢复壳标题 | `must-fix` |
| 侧栏品牌 `.app-brand-title` | `阅读`（`bd8afe5` 按上游改回） | `闲读` | 用户要求的差异 |
| 登录页 `<h1 class="auth-title">` | `OpenReader` | `闲读` | 用户要求的差异 |
| `manifest.webmanifest` `name`/`short_name`、`apple-mobile-web-app-title` | `OpenReader` | `闲读` | 用户要求的差异 |
| 图标 `openreader.svg` 的 `aria-label` | `OpenReader` | `闲读` | 用户要求的差异 |

不改：仓库与镜像名 `changshengyu/openreader`、`OPENREADER_*` 环境变量、浏览器存储 key（`openreader_token` 等）、`OpenReaderCustom*`
字体族、导出文件名前缀（`OpenReader-source-debug-…`）、README 中作为项目名的 OpenReader、Reader 标题格式与运行标题。

测试（先红后绿）：
- 单元 `frontend/tests/displayNameContract.test.mjs`：
  - 产品名是 `闲读`，`<title>`、`apple-mobile-web-app-title`、manifest `name`/`short_name`、图标 `aria-label`、侧栏品牌与登录页 `<h1>` 都等于它；
  - `SOURCE_DEBUG_DOCUMENT_TITLE === '闲读书源编辑器'`；
  - 首页工作区与登录页挂载时调用 `setDocumentTitle(SHELL_DOCUMENT_TITLE)`；书源调试挂载时设书源调试标题、卸载时恢复壳标题；
  - 仍然只有 `utils/documentTitle.js` 给 `document.title` 赋值。
- 浏览器 `scripts/smoke/document-title-contract.mjs` 补上第 8.2 节第 1 步与第 11 步，即新增 `shell` 流程：
  - `/`、打开再关闭 `/?overlay=sources`、`/search` 都是壳标题，侧栏品牌文字等于壳标题；
  - 无 token 时打开 `/login` 是壳标题，`<h1>` 等于壳标题；
  - `/source-debug`、`/bookSourceDebug/`、`/sources?action=debug` 是 `<壳标题>书源编辑器`；
  - 1440×900 下从首页点“调试书源”，新标签页是书源调试标题，原标签页仍是壳标题。

  移动视口的侧栏需要先展开，这一步只在桌面检查，其余在三个视口都检查。

状态 **contract-only**（由下方记录取代）。

### 14.1 记录（2026-11-13）

提交：
- 合同 `cb2c545`；
- 红测 `7b181ca`：单元 4 项中 3 项失败（`<title>` 仍为 `OpenReader`、壳标题常量、挂载接线），技术名对照通过；浏览器 `shell` 流程在 1440×900 因侧栏品牌为 `阅读` 失败；
- 实现 `52cec1e`。实现中另把侧栏单字徽标 `阅` 改为 `闲`，与新名一致。

验证：
- 前端 886/886（原 882 加新增 4 项），build 通过；
- 真实 Go 服务提供生产构建：
  - `document-title-contract.mjs` 全部四个流程（navigation、auth、remote、shell）在 1440×900、390×844、360×800 通过，壳标题为 `闲读`；
  - `openreader-smoke.mjs` 三视口、`reader-reauthentication-contract.mjs` 四视口、`source-debug-contract.mjs` 通过；
  - `registration-limits-contract.mjs` 三视口 6 项检查通过；
- 后端与依赖未改，Go 测试与公告门留给推送前检查和 CI。

状态 **implementation-complete / regression-validated**；分支 `claude/display-name-xiandu` 已推送，未合入 main，镜像未发布，生产未访问。

## 15. 发布（2026-11-13）

本节内容（D1 产品显示名“闲读”）随本次合并发布。合并提交 `4856215`（闲读）、`310745d`（书籍上限与记住登录信息）、`f5bfd5d`（Reader P0）、`6175cae`（桌面“左右滑动”两项），main 由 `f82e0f5` 快进到 `f8786ab`（ls-remote 核验，未强推）。

推送前在合并树上：后端 `go test ./...` 31 个包与 vet，race api/db/services（api 1088.9 秒）无数据竞争；前端 971/971 与 build；公告门自测 114/114，现场 Go 与前端扫描 0 个发现（推送前重跑）；compose 配置、存储 HTTP 合同、浏览器 smoke 接线 10/10；secdb main 与 11-10 快照一致。真实 Go 服务、全新数据目录下浏览器合同全部通过：openreader-smoke，标签页标题四个流程（壳标题 `闲读`），错误页 48 例，运行标题，桌面面板正文控件连续 3 次 120/120（`C1 flip` 36/36），晚到 flip 设置，D4 9/9，设置位置，TTS 五个视口，连续，重新认证四视口，记住登录信息，书籍上限，注册限制，移动（含 iPad），会话生命周期，首页会话隔离，文本模式，EPUB，音频，书源调试。服务日志没有 panic；唯一的 5xx 是注册限制合同关页时被取消的 `PUT /api/settings/shelf`（`settings.go` 把取消的请求记为 500，原有行为，另起切片）。合并适配：`f45f766` 让“闲读”技术名对照改查 `utils/authToken.js` 的 `AUTH_TOKEN_KEY = 'openreader_token'`，`f8786ab` 让 D4 合同从 `da438bc7` 之后的 `.rail-item[aria-label="设置"]` 打开设置，断言都没改。合入 flip 分支前，`C1 flip` 在 main `f82e0f5`（三次共 8/36 失败）、Reader 分支与合并树上都偶发超时，原因是晚到的账号设置让桌面停在 flip；合入后消失。

可信 Actions run `43400193943` 43/43 步成功（09:18–09:35 UTC）：两平台 OS 门 `pass-with-unknown-origins`（16/16 包、110/109 个 APK 文件、74 组比较无待修复项，secdb main 11-12 22:52:32，复审期限 2026-12-10），Go 公告门 go1.26.11 0 个发现（250 条公告，x/crypto pin 复审期限 2026-12-16），前端公告门 0 个发现，卷门与平台核验通过。

镜像：`ghcr.io/changshengyu/openreader:f8786ab` 与 `latest` 指向 OCI index `sha256:0066943b9fb879041a503391723bf9bc4b31ee19aee06d273073b6ac4142890f`；linux/amd64 `sha256:6944f67944131f2d90e15ccd83e56d6b808ec3cbb4a4f8474d808f15fa064d13`，linux/arm64 `sha256:2537a5de7dc67046ee4449db329c77d0028ab03d291cfe142b290ec4c0bfa729`，另有两个 attestation。两平台 revision 标签均为 `f8786abe2679ca34b76b6aaa6017558225662ecd`；两平台 `/app/openreader` 均由 go1.26.11 构建，`/app/public/index.html` 标题为 `闲读`（逐层校验 sha256 后读取）。

状态 **released / Docker-published / awaiting-device-verification**；生产未访问，实际运行版本未知。

## 16. W19 决定：manifest 按上游 `fullscreen`、不锁方向（2026-11-13）

来源：PWA 合同（`pwa-service-worker-fixed-baseline-second-audit-p2-contract.md`）把 W19（manifest 的 `display` 与 `orientation`）交给本合同判断。用户 2026-11-13 选择“按上游 fullscreen”。基于 main `684c849`。

上游（fa22f27 的实际构建产物，见 PWA 合同 2.9）：发布的 `manifest.json` 中 `display` 为 `fullscreen`，没有 `orientation`。fork 提交 `0556c7b` 的 `public/manifest.json`（`standalone` + `portrait`）被构建插件忽略，没有进入产物。

| 编号 | 项 | 当前 | 目标 | 分类 |
| --- | --- | --- | --- | --- |
| M1 | `display` | `standalone` | `fullscreen` | must-fix（用户决定，对齐上游构建产物） |
| M2 | `orientation` | `portrait` | 删除该键，不锁方向 | must-fix（同上） |
| M3 | `name`、`short_name`、颜色、图标、`start_url`、`scope`、`lang` | 现值 | 不变 | 不在本切片：名称是 D1 的“闲读”；上游的白色 `theme_color`、黑色 `background_color` 与 android-chrome 图标不在这次的选择里 |
| M4 | `index.html` 的 `theme-color`、`apple-mobile-web-app-*` 与 manifest 链接 | 现值 | 不变 | iOS 主屏幕网页不读 manifest 的 `display`，由 `apple-mobile-web-app-capable` 决定 |

行为：
- 支持 `fullscreen` 的浏览器（如 Android Chrome）从主屏幕打开时隐藏系统状态栏；不支持的浏览器依次回退到 `standalone`、`minimal-ui`、`browser`。
- 去掉方向锁后，手机和平板可以横屏。首页、搜索、发现与工作台已有的 `orientationchange` 处理照常重排。

测试计划：
- 单元 `frontend/tests/manifestDisplayContract.test.mjs`：
  - 红：`display === 'fullscreen'`，没有 `orientation` 键；
  - 对照（绿）：`name`、`short_name` 为“闲读”，`start_url`、`scope` 为 `/`，`index.html` 仍有 manifest 链接与 `apple-mobile-web-app-capable`。
- 回归：前端全量与 build（manifest 在 SW 预缓存清单中，构建号会变）；PWA 合同、标签页标题合同、openreader-smoke、移动端合同。

状态 **contract-only**（由下方记录取代）。

### 16.1 记录（2026-11-13）

提交：合同 `508fa5a`；红测 `61598ac`（M1/M2 在 `display: standalone` 处失败，M3/M4 对照通过）；实现 `47375c8`（`display` 改为 `fullscreen`，删除 `orientation`）。

验证：
- 前端 999/999（新增 2 项），build 通过；SW 构建号变为 `e5c767f8429a3a17`，仍预缓存 64 个文件；产物 manifest 为 `fullscreen`、无 `orientation`、名称“闲读”。
- 真实 Go 服务（后端与 `2f19133` 相同）：PWA 合同 E1–E7、openreader-smoke 三视口、标签页标题合同四个流程、移动端合同（含 iPad）通过，服务日志无 5xx。

状态 **implementation-complete / regression-validated**；分支 `claude/manifest-fullscreen-w19`，未合入 main，镜像未发布，生产未访问。

