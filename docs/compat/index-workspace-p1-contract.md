# P1 Index 工作台上游契约与重建边界

状态：**复审中，尚未开始本批应用代码改动**。  
基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
上游工作副本：`/private/tmp/reader-dev-upstream-audit`。

本文件取代此前“路由已收敛/组件已共用”即可视为 P1 完成的判断。Vue 3、Pinia、Go 多用户适配可以改变实现形态，但不自动构成用户可见行为的等价证据。

## 1. 上游场景契约

上游权威入口为 `web/src/views/Index.vue`，不是多个独立业务路由：

| 场景 | 上游状态与动作 | 必须保留的可见结果 |
|---|---|---|
| 工作台根 | `.index-wrapper` 同时挂载 `.navigation-wrapper` 和 `.shelf-wrapper`。书架、搜索、探索、书源管理、导入、本地书仓、WebDAV、用户、备份、RSS、替换规则、BookInfo 都在同一 Index 场景内完成。 | 打开工作台操作不能把用户带到另一套产品页面；关闭操作后仍回到原工作台和书架状态。 |
| 搜索 | 侧栏输入 Enter 调 `searchBook(1)`；上游默认 `searchType = multi`、`concurrentCount = 24`，选项为 `12,18,24,30,36,42,48,54,60`。搜索将 `isSearchResult=true`、`isExploreResult=false`；“书架”清除结果状态。 | 搜索配置、结果、加载更多、BookInfo/直接阅读都属于同一场景，默认与持久化语义不能漂移。 |
| 探索 | 侧栏“探索书源”打开 popover；`showSearchList(data)` 设置 `isSearchResult=true`、`isExploreResult=true`；书架顶部显示“探索”和“加载更多”。 | 探索结果不是新路由；返回书架会同时清空搜索/探索结果状态。 |
| 书架与书籍信息 | 书架封面点击 `showBookInfoDialog(book)`，其它书籍区域 `toDetail(book)`；搜索结果也进入相同 BookInfo/阅读动作。 | 只能有一份 BookInfo 状态与动作策略；不能因入口不同显示不同业务流程。 |
| 移动侧栏 | `collapseMenu` 时初始隐藏（`navigation-hidden` 为 `margin-left:-260px`）；右滑开、左滑关，视觉宽度 260px、拖动手势窗口 270px、过渡 300ms；书架点击关闭，菜单按钮阻止冒泡。 | 390×844 和 360×800 都应无横向溢出，拖动跟手，点击工作区关闭，而侧栏自身和底部控件不穿透。 |
| 侧栏底部 | `.bottom-icons` 是导航层内独立的绝对定位区：`bottom:30px; left:36px; width:188px`，不随导航内容滚动。 | GitHub 与昼夜切换固定于侧栏底部。根据用户明确要求，**拖动侧栏时它们可保持屏幕位置而不随侧栏滑动**；这是对上游的允许交互优化。 |

## 2. 当前映射与差异判定

| 上游职责 | 当前映射 | 结论 | 证据 / 后续动作 |
|---|---|---|---|
| 单一 Index 根场景 | `layouts/AppLayout.vue` + `views/Home.vue` + `stores/indexWorkspace.js`；兼容的 `/search`、`/discover`、`/sources`、`/settings`、`/local-store`、`/books/:id` 均在 `router/index.js` 重定向到 `/`。 | **技术栈等价，待真实浏览器验收**。旧链接没有再成为独立业务页面，这一点符合上游场景边界。 | 必须测试根场景保持挂载：搜索/探索/弹层打开与关闭后书架、侧栏和筛选状态均正确保留。 |
| 结果状态机 | `indexWorkspace.mode = shelf|search|explore`；`Home.vue` 依次挂载 `Search.vue`、`Discover.vue` 或书架；`backToShelf()` 清掉结果 query 并让 store 返回 shelf。 | **技术栈等价，待状态契约**。`search/explore` 没有保持上游的两个布尔变量，但三态互斥可表达相同结果。 | 为 Enter 搜索、探索→加载更多、书架返回、路由兼容意图、刷新书架编写 Pinia/浏览器转换表。 |
| 搜索默认值和持久化 | `preferences.js`、`useAppSidebarSearch.js`、`Search.vue` 现在使用 `all`、`group`、`single`，默认并发为 **60**，选项为 **8/16/32/60**。 | **must-fix**。上游默认 `multi + 24` 和离散并发选项是用户可见、会持久化并直接影响书源压力的契约；多用户运行不构成任意改写默认值的理由。 | 后续实施必须先完成 `data-migration-compat`：已保存的 `all`、`group`、`single` 与并发值如何无损映射；然后以测试锁定新用户默认值、旧用户设置、单源实际 1 并发和多源请求参数。 |
| 搜索类型的扩展 | 当前 `group` 与“本地书籍”入口提供了上游之外的范围选择。 | **intentional-redesign，待逐项证据**。分组搜索可由上游 `bookSourceGroup` 的选择表达；本地书仓搜索是现有运行环境能力，但不能替代或污染远程多源默认流程。 | 保留前须证明：默认仍是上游多源，group/local 不改变 remote API 的请求、加载更多和 BookInfo→阅读顺序。 |
| 书架点击语义 | `Home.vue` 封面 `openDetail()`，行点击 `continueRead()`；`OverlayBookInfo` 是 `BookInfoDialog → BookInfoPanel` 的唯一渲染入口，搜索/探索/Reader 都调用 `overlay.openBookInfo()`。 | **技术栈等价，待操作回归**。当前没有发现第二个 BookInfo 产品页面；旧 `/books/:id` 只触发此共享弹层。 | 测试书架、搜索、探索、阅读器、旧 URL 五个入口的标题、来源、分组、添加、继续阅读、关闭后的状态一致。 |
| 全局操作宿主 | `GlobalOverlayHost.vue` 将书源、导入、书籍管理、分组、书签、本地书仓、WebDAV、备份、用户、RSS、规则放在根工作台宿主下。 | **unknown**。结构上可等价于上游在 Index 中声明的 dialog/popover；现有旧审查记录不是本轮证据。 | 分别提取每个 overlay 的打开、关闭、嵌套、移动全屏与点击穿透契约；在 P1-B~P1-E 前不得宣称全部对齐。 |
| 侧栏几何与拖动 | `tokens.css` 为 260px；`useAppMobileNavigation.js` 为 260px 视觉宽度/270px 手势范围；`AppLayout.vue` 以 `margin-left` 跟手，工作区点击调用 `close()`。 | **技术栈等价，待像素和手势验收**。几何参数和开关方向与上游相同。 | 在三种目标视口断言初始隐藏、左/右拖动、动画结束、工作区点击、边缘 20px 不启动手势、无横向滚动。 |
| 固定 GitHub/昼夜控件 | `AppLayout.vue` 的 `.sidebar-bottom-icons` 位于滚动容器外，拖动期间以反向 `translateX` 固定。 | **acceptable-change**：用户明确要求其不随侧栏滑动，且它仍固定于侧栏底部并可点击。 | 浏览器断言拖动中控件 viewport `x` 不变、打开后位于侧栏底部、点击不穿透关闭侧栏。 |
| 侧栏内容布局 | 上游是纵向设置分区，分区内的标签操作以内容宽度自然换行；当前移动端把多数管理动作重排成等宽双列按钮，桌面还增加网格/列表视图切换。 | **must-fix（视觉与误触风险）**。这些改变没有列入允许差异，不能凭“功能可达”保留。 | P1-A 需要基于上游侧栏的间距、标签自然换行语义、操作顺序和最小点击区域重建；网格/列表仅在明确记录为用户可选增强后才可保留。 |

## 3. P1 分批实施顺序

### P1-A：Index 壳与移动侧栏

1. 先写失败的 DOM/浏览器契约：260/270、默认隐藏、拖动跟手、工作区关闭、底部控件固定、没有点击穿透、桌面 260px 工作区偏移。
2. 以 `Index.vue` 的纵向信息层级和标签自然换行语义重建 `AppLayout.vue` 侧栏；不得把源管理/账户等还原成独立路由。
3. 保留用户明确的底部控件“拖动中不移动”优化，并单独记录在测试名中。
4. 通过 1440×900、390×844、360×800 后，可作为半模块 Docker 验收点。

### P1-B：搜索、探索与 BookInfo 连续流程

1. 使用 `api-contract-compat` 和 `data-migration-compat` 提取搜索配置迁移及 `/api/search` 参数语义。
2. 先替换 60 / 8,16,32 默认值及旧数据迁移测试，再实施 UI；不能静默重置已有用户的搜索偏好。
3. 验证 `侧栏搜索 → 结果 → 共享 BookInfo → 加入书架或直接阅读 → 返回书架`，以及探索的同一流程。

### P1-C：逐个复审全局工作台操作

按书源、导入/本地书仓、WebDAV/备份、书籍管理/分组、用户、RSS、替换规则顺序处理。每个操作都先单独写出上游打开/关闭、API/数据副作用、移动层级合同；本文件不把历史“已完成”记录当作证据。

## 4. 本批禁止事项

- 在 P1-A 契约和测试完成前，不更改应用侧栏或搜索逻辑。
- 不因现有 `GlobalOverlayHost`、兼容重定向或 Pinia store 存在，就宣称 Index 已完成上游对齐。
- 不修改 `data/`、`cache/`、`library/` 或 SQLite 现有值来迁移搜索默认值；迁移必须是读取时兼容、写入时规范化且可回归。
- 不将用户明确允许的 Reader 连续滚动/数值步进器扩展为工作台 UI 的自由重设计理由。

## 5. 2026-07-13 P1-A 实施记录：侧栏壳与标签操作流

完成项：

- 保留并复验了移动端上游几何：260px 侧栏、270px 拖动窗口、初始隐藏、300ms 开关动画、工作区点击关闭和边缘 20px 手势排除。
- GitHub 与昼夜按钮仍在侧栏滚动容器外、固定底部；拖动时以反向位移保持屏幕位置不随侧栏滑动，这是用户明确要求的允许差异。
- 侧栏搜索恢复为输入框 Enter 触发；移除了上游没有的“书源搜索 / 本地书籍”双按钮捷径。
- 书源、书架、用户、WebDAV、缓存等操作由当前等宽双列网格恢复为与上游 `setting-item` 对应的内容宽度标签流：块级分区、操作按钮自然换行、15px 横向/纵向间距。
- 没有改变搜索偏好数据、远程搜索 API、BookInfo、任何 overlay 或用户持久化数据；搜索默认值迁移仍是 P1-B 的独立数据兼容工作。

验证：

- `frontend/npm test`：365 项通过（新增侧栏标签流静态合同）。
- `frontend/npm run build`：通过。
- `backend/go test ./...`：通过。
- `scripts/smoke/index-mobile-sidebar-contract.mjs` 已扩展为运行时标签流断言，覆盖 390×844 与 360×800；本机独立 Chrome 在创建浏览器上下文前被 macOS 以 `SIGABRT` 终止，因此本次未计为真实浏览器通过。该错误未进入页面、网络 mock 或产品断言；之后应在可启动的独立浏览器进程中重跑，不能作为 P1-A 完整浏览器签收依据。

本切片可以发布 Docker 供人工验收的范围：移动侧栏拖动/固定底部控件、Enter 搜索入口、标签自然换行和书架横向几何。未完成范围：搜索默认值与旧设置迁移、搜索/探索/BookInfo 连续流程、各工作台 overlay 的逐项上游复审。
