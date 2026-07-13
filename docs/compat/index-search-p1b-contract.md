# P1-B Index 搜索、探索与 BookInfo 连续流程契约

状态：**P1-B 搜索默认值、并发偏好、稳定续页 cursor、跨页去重与搜索/探索/BookInfo 三视口 smoke 已实现；其余 BookInfo 入口与结果场景外的工作台动作仍在后续 P1-B/P1-C 范围内。**
基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
上游证据：`web/src/views/Index.vue`、`web/src/plugins/config.js`、`BookController.kt#searchBook` / `#searchBookMulti`；当前证据：`frontend/src/{stores/preferences.js,composables/useAppSidebarSearch.js,stores/indexWorkspace.js,views/Search.vue}`、`backend/api/search.go`、`backend/api/settings.go`。

本合同仅覆盖工作台的远程搜索、探索结果与共享 BookInfo 的连续操作；阅读器内“书源”候选面板另由 Reader P0/P2 合同覆盖。

## 1. 上游行为与状态转换

| 触发 | 上游状态 / 请求 | 用户可见结果 |
|---|---|---|
| 新用户默认 | `searchConfig = { searchType: 'multi', bookSourceGroup: '', bookSourceUrl: '', concurrentCount: 24 }`；可选并发为 `12,18,24,30,36,42,48,54,60`。 | 进入工作台后默认是多源搜索、24 并发；设置保存于用户配置。 |
| 侧栏 Enter | 输入为空提示“请输入关键词进行搜索”；单源未选书源提示“请选择书源进行搜索”；否则开始第一页并重置 `searchLastIndex=-1`。 | 工作台显示搜索结果，不离开 Index 场景。 |
| 单源 | `POST /searchBook`，参数 `key, bookSourceUrl, page`；单源页码由 `page` 推进。 | 结果可继续加载，单源失败显示明确错误。 |
| 多源 / 分组 | `POST /searchBookMulti` 或 SSE，参数 `key, bookSourceGroup, concurrentCount, lastIndex, page`；后端按用户书源及分组筛选，游标从 `lastIndex + 1` 开始；结果按 `bookUrl` 去重。 | 第一页清空旧结果，多源“加载更多”推进游标；没有书源/没有更多有明确错误。 |
| 探索 | `Explore.vue` 把书源入口结果交给 Index `showSearchList(data)`，使 `isSearchResult=true`、`isExploreResult=true`；Index 顶栏改为“探索”。 | 探索在同一结果场景，加载更多继续当前探索入口，返回书架清除结果状态。 |
| 结果动作 | 书架封面、搜索、探索均进入同一 `BookInfo`；阅读走 `toDetail`；添加书架先选择分组。 | 不出现独立 BookDetail 产品页面或入口各自实现的添加/阅读逻辑。 |

## 2. 当前映射与判定

| 合同层 | 当前实现 | 判定 |
|---|---|---|
| 根场景和状态机 | `indexWorkspace.mode = shelf|search|explore`，`Home.vue` 在同一根场景挂载 `Search.vue`/`Discover.vue`，旧 URL 转为 workspace intent。 | **技术栈等价，待浏览器流程复验**。三态比两个布尔值更不易出现非法组合，且没有改变可见的搜索/探索/返回语义。 |
| 搜索模式 | `all` = 所有启用书源、`group` = 分组内启用书源、`single` = 指定一个书源；`Search.vue` 把选择结果传为 `sourceIds`。 | **技术栈等价**。Go/多用户数据库以 source ID 代替上游 `bookSourceUrl/bookSourceGroup`；`all/group/single` 可保留为内部/持久化枚举，不能改成另一套用户流程。 |
| API | `POST /api/search` 接受 `keyword, sourceIds, concurrentCount, page, lastIndex, searchSize`；单源用页码，多源用游标；按 caller 的用户书源查询并保持 `sourceIds` 顺序。 | **技术栈等价，待 API 合同补强**。路径/JSON 可以不同，但必须保持单源分页、多源游标、分组筛选、去重、超时、局部书源失败与用户隔离。 |
| 默认并发 | 新前端默认、workspace fallback、搜索视图 fallback 和后端 `normalizedConcurrentCount(<=0)` 都是 **60**；当前候选仅 `8,16,32,60`。 | **must-fix**。上游用户配置默认是多源 24，候选为 `12..60` 的九档；当前默认会无理由提高远程书源压力。 |
| 已有并发设置 | `preferences` 目前只接受 `8,16,32,60`，`Search.vue` / workspace 对其它值回退 60；服务器设置和备份把 search JSON 原样保存。 | **must-fix（数据兼容）**。不能为了恢复上游候选把已存 `8/16/32/60` 静默丢失或改为 24；尤其备份恢复已验证保存 `concurrent:32`。 |
| 空书源错误 | 当前 Search 在 `sourceIds` 为空时提前提示“请至少选择一个书源”，后端无 source 时返回空数组/空分页。 | **must-fix（错误语义）**。上游多源/分组无可用书源最终反馈“未配置书源”；当前错误文案和 API 空成功会掩盖配置问题。Go 的 HTTP 状态可适配，但前端必须稳定呈现该语义。 |
| 搜索结果去重 | `Search.vue` 优先以 `bookUrl` 去重，空 URL 才用 `sourceId-bookUrl` key；Go 每个多源批次也用 `title|author` 去重。 | **技术栈等价，待跨页合同**。上游前端以 `bookUrl` 合并；要测试跨游标页仍不重复，不能把不同书源的空 URL 错误合并。 |
| BookInfo 交接 | `Search.vue`、`Discover.vue`、`Home.vue`、Reader 统一调用 `overlay.openBookInfo()`，并用 `useBookInfoAddToShelf` 完成选择分组→创建→阅读。 | **技术栈等价，待五入口浏览器回归**。此前实现记录不是最终签收证据。 |
| 本地书籍搜索 | `mode=local` 复用结果场景搜索本地书仓/书架。 | **intentional-redesign**。保留为当前运行环境增强，但不得成为 Enter 远程多源搜索的默认分支、不得改变远程搜索请求或 BookInfo 动作。 |

## 3. 数据迁移合同

不更改 SQLite schema、`data/`、`cache/`、`library/`，也不批量写入用户设置。迁移只在前端读取/规范化和下一次用户显式保存时发生。

| 已保存值 | 读取后的语义 | 写入策略 |
|---|---|---|
| 缺失、非法或 `concurrent <= 0` | 新用户默认 **24**。 | 下一次正常保存使用 24。 |
| 上游档位 `12/18/24/30/36/42/48/54/60` | 原值保留。 | 原值保留。 |
| 现有 OpenReader 旧档位 `8/16/32` | 原值保留并在下拉中以“旧配置”可见，不能静默回退。 | 用户选择任意正式上游档位后才写成该值。 |
| `searchType=all/group/single` | 原值保留；分别等价于上游多源全量/多源分组/单源。 | 继续写该内部枚举，避免旧客户端与备份失效。 |
| 上游备份的 `searchType=multi`、`bookSourceGroup`、`bookSourceUrl`、`concurrentCount` | 仅在导入/恢复适配层映射为当前枚举与字段；本批不直接修改原备份文件。 | 需要专门的 backup restore 合同，不能由浏览器偏好 sanitizer 猜测 source ID。 |

## 4. 实施前测试门槛

1. 前端 preferences / workspace：新用户和缺失值为 `all + 24`；九档上游值不丢失；旧 `8/16/32` 可读、可见、未选择新值前不被重写。
2. 侧栏与结果视图：Enter 将 `all + 24` 传进同一 workspace；分组与单源的 source ID 选择正确；无可用书源显示“未配置书源”。
3. Go API：无 `concurrentCount` 时以 24 限制；单源 `page` 和多源 `lastIndex` 不串用；指定 source ID 顺序、分组映射、跨页去重、部分源失败、无源错误和用户隔离都有测试。
4. 浏览器：1440×900、390×844、360×800 执行 `Enter 搜索 → 结果 → 加载更多 → BookInfo → 取消/确认分组 → 阅读 → 返回书架`；探索也走共享 BookInfo。
5. 回归：前端全量测试、后端全量测试、生产构建；有 Docker 发布时还需本地镜像和卷/备份 smoke。

## 5. 受控实施顺序

1. 使用 `data-migration-compat` 与 `api-contract-compat` 把上列迁移和 `/api/search` 失败/默认行为变成失败测试。
2. 抽取共享并发选项/默认值规范化函数，替换四处不一致的 60 / `8,16,32,60` 常量；保留旧值可见性。
3. 改正前端无书源提示和 Go 默认/空源错误，随后验证单源、多源、分组和连续分页。
4. 最后再做真实浏览器及 BookInfo 五入口回归；P1-B 完整通过后才将其标为对齐。

## 6. 2026-07-13 实施记录：搜索默认值与错误语义切片

- 前端统一从 `searchPreference.js` 读取默认值和选项：新设置为 `all + 24`，标准上游候选为 `12/18/24/30/36/42/48/54/60`。
- 已部署的 `8/16/32` 不会被读取逻辑静默重置；下拉继续显示该值并明确标注“旧配置”，只有用户主动选择标准档位才会更新。
- 工作台意图、搜索视图、侧栏搜索和服务端的缺省并发全部收敛为 24；正数仍按实际书源数限流。
- `POST /api/search` 在没有任何启用/选中书源时返回 `400 {"error":"未配置书源"}`。已配置书源若被该用户的失效缓存全部临时抑制，仍返回成功空结果，保持“跳过失效书源”而非误报配置错误。
- 覆盖了前端偏好兼容、侧栏搜索参数、服务端默认值/无源错误，以及失效书源缓存的回归。全量前端 `npm test` 为 **369 项通过**，`go test ./...` 与 `npm run build` 通过。
- `scripts/smoke/index-workspace-contract.mjs` 已在真实 Chrome 以 `1440×900`、`390×844`、`360×800` 通过：新会话侧栏搜索发出 24 并发；旧链接的 8 并发保持；搜索、BookInfo 的取消/分组确认/阅读跳转、探索及返回书架均无页面异常和横向溢出。

## 7. 后续续页与跨页结果复审（2026-07-13，仅合同；尚未改动应用代码）

上游证据：`Index.vue#searchBook/#searchBookByEventStream/#loadMore/#showSearchList`、`BookController.kt#searchBookMulti`、`Explore.vue#loadMore`。当前证据：`Search.vue#requestRemoteSearch/#loadMoreRemote`、`Discover.vue#loadMoreBooks`、`indexWorkspace.js`、`backend/api/search.go`。

| 合同层 | 固定上游行为 | 当前 OpenReader 行为 | 判定与改造边界 |
|---|---|---|---|
| 续页入口 | 搜索/探索结果均在同一 Index 标题操作区提供“加载更多”；触发时记住列表滚动位置。 | Search/Discover 各自在正文底部维护独立按钮；Search 在 `hasMore=false` 后直接隐藏/禁用。 | **must-fix**：将可见续页动作与结果标题的 Index 工作台职责收敛；保留按钮禁用这一无障碍增强，但要以“没有更多了”明确结束，而不是悄然移除动作。 |
| 新搜索重置 | 首次搜索将 `searchPage=1`、`searchLastIndex=-1`、结果清空；SSE 新搜索会关闭前一流。 | `beginSearch` 有 revision，Search 的异步 REST 调用未以 revision/请求代号拒绝旧响应。 | **must-fix**：新搜索、返回书架或进入探索后，旧搜索及旧“加载更多”响应不得覆盖当前场景或拼入结果。 |
| 单源分页 | `page` 是单源真实页码；多源请求携带它但服务端由 `lastIndex` 决定书源进度。 | 单源和多源都递增 `searchPage`，且多源响应的 `lastIndex` 写回，但 UI 状态未说明哪一个是权威游标。 | **技术栈等价，需显式化**：单源只以 `page` 判定续页；多源只以 `lastIndex` 判定续页。工作台 continuation 必须保存并展示服务端返回的权威字段。 |
| 多源游标与失效缓存 | 上游游标是稳定的用户书源数组下标；一次请求从 `lastIndex + 1` 继续。 | OpenReader 会先过滤当前用户的失效书源，再把旧 `lastIndex` 套到缩短后的数组。失败缓存 TTL 变化会令下标漂移，可能跳源或重试已扫描书源。 | **must-fix（OpenReader 安全增强的兼容修复）**：游标始终指向原始排序后的已选书源序列；暂时抑制的源只在执行时跳过，不改变 ordinal。 |
| 跨页去重 | Index 在整个搜索会话以 `bookUrl` 去重；空/无效 URL 不应把不相关结果合并。没有新增条目时提示“没有更多啦”。 | 前端以 `bookUrl`、再以带书源的 fallback key 去重，方向正确；后端每个请求另以 `title|author` 去重，跨 cursor 页不持久；重复批仍可能推进/终止状态不清晰。 | **技术栈等价但待验证**：前端会话级 key 是最终可见去重依据；后端的批内去重不得替代它。重复页仍推进安全游标，且当服务端无后续或页面无新增时显示明确反馈。 |
| 探索续页 | Explore 递增当前入口页并把合并结果通过 `showSearchList` 写回同一 Index 场景。 | 当前按 `remoteBookKey` 合并且有 `hasMore`，但入口仍在 Discover 正文底部、并与 Search 的 continuation 动作分离。 | **must-fix（工作台结构）**：探索与搜索使用同一结果标题续页动作和 loading 防重入；保留 REST `hasMore` 作为现代适配。 |

本小节允许的差异：OpenReader 继续使用 JWT REST、`sourceIds`、`hasMore` 与失败书源抑制；不复制上游的 EventSource 实现。上述差异只有在不改变可见的 Index 工作台续页流程、稳定游标、跨页去重和错误/结束反馈时才成立。

实施前测试：

1. Go：单源第二页只推进 `page`；多源第二批只推进 `lastIndex`；失效缓存中的中间书源不令后续 cursor 跳源或回退；全被暂时抑制仍为成功空结果。
2. 前端：新搜索和切换到探索会使旧请求结果失效；重复 `bookUrl` 不重复显示、空 URL 保留不同书源结果；重复页无新增有明确“没有更多了”反馈。
3. 浏览器：1440×900、390×844、360×800 依次验证单源两页、多源两批、探索两页、加载中防重入、返回书架后旧响应不污染结果及标题续页动作的可见状态。

## 8. 2026-07-13 实施记录：稳定续页与跨页结果切片

- `/api/search` 的多源 `lastIndex` 现在始终对应原始排序的已选书源序列。当前用户的失效书源缓存只跳过执行，不会压缩数组或重编号 cursor；因此缓存的进入/过期不会让续页跳过或重复书源。
- 单源请求只发送/消费 `page`；多源请求只发送/消费 `lastIndex` 与 `searchSize`。前端仍保存页面号作为工作台展示状态，但不会把它作为多源执行 cursor。
- 搜索和探索的“加载更多”已回到结果标题操作区，并保留可见的禁用终态“没有更多了”。滚动位置会在续页前写入共享工作台状态。
- 共享请求门同时检查本地异步代号、工作台场景和场景 revision：新搜索、进入探索、返回书架或选择另一个探索入口后，迟到的旧响应不得写入当前结果。
- 搜索和探索使用同一会话级结果合并规则：优先以 `bookUrl` 去重；缺失 URL 时退回 `sourceId + title + author`，不会把不相关空 URL 行合并。
- 验证：Go 覆盖失效缓存中间源的 cursor 稳定性；前端覆盖结果 key 与陈旧请求门；全量 `npm test` **371 项通过**、`go test ./...`、`npm run build` 通过；真实 Chrome `1440×900`、`390×844`、`360×800` 覆盖多源两批、单源两页、探索两页、重复结果、陈旧搜索切换探索、BookInfo 分组确认/阅读及返回书架。

允许差异：OpenReader 保持 REST `hasMore`、JWT、`sourceIds` 与用户级失败缓存，而不复制上游 EventSource；这些增强已通过上列稳定 cursor 和可见工作台流程约束。

仍未完成：BookInfo 从书架、搜索、探索、阅读器及旧链接五入口的完整复审，以及 P1-C 的其余全局工作台操作收敛。

## 9. 2026-07-13 BookInfo 五入口复审（仅合同；尚未改动应用代码）

本小节以 `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 的
`web/src/views/Index.vue#toDetail/#showBookInfoDialog`、
`web/src/components/BookInfo.vue` 与
`web/src/views/Reader.vue#showReadingBookInfo` 为权威。当前代码的共享
`OverlayBookInfo` 不能作为正确性的证据。

### 上游共享状态与可见动作

- 全部入口写入同一个全局 `showBookInfo`，再打开同一个 `BookInfo` 对话框；关闭只关闭对话框，不切换 Index/Reader 场景，也不触发阅读。
- 书架、搜索和探索结果的**封面**打开 BookInfo；书架条目的其余区域进入 Reader。搜索/探索结果的其余区域把该结果写为临时 `readingBook` 后进入 `/reader?search=1`，不要求先加入书架。
- 对话框以“是否已在书架”为唯一动作分支：已在书架时显示封面更新、来源、追更、分组和本地更新等书架动作；不在书架时只显示一个“加入书架”。卡片上的“加入书架”快捷入口同样先进入分组选择。没有按搜索/探索/阅读入口分别注入“查看详情”“继续阅读”“开始阅读”按钮的第二套流程。
- 阅读器从当前 `readingBook` 与书架同 URL 的记录合并后打开该对话框。它不关闭阅读工具层，也不因为打开/关闭 BookInfo 改变章节、进度或阅读路线。

### 五入口矩阵

| 入口 | 上游状态转换 | 当前 OpenReader 证据 | 本轮判定与改造边界 |
|---|---|---|---|
| 书架 | 封面 → 全局 BookInfo；条目正文 → 已保存书籍的 Reader。关闭信息框仍留在书架。 | `Home.vue#openDetail/#handleBookRowClick` 分别调用 `overlay.openBookInfo` 与 Reader 路由；`OverlayBookInfo` 以书架记录判定封面/追更/分组/本地更新权限。 | **已复核一致**。保留当前进度 query、用户隔离与安全的纯文本简介；新增浏览器断言确保关闭后不产生路由变化。 |
| 搜索远程结果 | 封面 → 非书架 BookInfo；卡片其他区域 → 临时 Reader（`?search=1`），不创建书架记录；对话框中的唯一非书架动作是加入书架。 | `RemoteBookResultGroups.vue` 将整张卡和“查看信息”都发为 `preview`；`Search.vue#openPreview` 总是打开信息框，并按上下文注入“查看详情/继续阅读”或“加入书架/加入并阅读”。 | **必须重建**。恢复封面/正文两个入口；实现不落库的远程阅读会话，不能把“加入并阅读”当作临时阅读的替代品。BookInfo 本身收敛为书架状态动作，而非搜索动作菜单。分组确认是 OpenReader 的多分类安全适配，可保留在“加入书架”事务中。 |
| 探索远程结果 | 与搜索相同：封面信息、正文临时阅读、同一 BookInfo/加入书架分支。 | `Discover.vue#openPreview` 与 Search 使用同一上下文按钮模型；`RemoteBookResultGroups.vue` 使整卡只能预览。 | **必须重建**，与搜索共用同一远程临时阅读与 BookInfo 策略；不得复制第二个 Explore 专用状态机。 |
| 阅读器 | 合并当前阅读记录和同 URL 的书架记录 → 全局 BookInfo；工具层与 Reader 路由保持不变。 | `useReaderPanels#openBookInfo` 直接打开共享 overlay；`Reader.vue` 把目录刷新结果同步到 `overlay.bookInfoBook`。没有额外读/加书架按钮。 | **技术栈等价，待回归**。当前“阅读中”上下文状态不替代上游字段；须验证已加入/临时远程阅读两种 Reader 状态都能正确显示书架权限，且关闭不影响移动工具层。 |
| 旧 `/books/:id` 链接 | 上游没有详情页路由。 | `router/index.js` 重定向到 `/?bookInfo=:id`，`AppLayout#openRouteBookInfoOverlay` 拉取当前用户书籍并以共享 BookInfo 打开；对话框关闭后未清除查询参数。 | **允许兼容入口 + 必须修复关闭语义**。保留旧链接和一个“开始阅读”兼容动作，但关闭、无权/不存在和再次导航后必须仅清理 `bookInfo` intent，不能令 overlay 重新弹出或污染其他工作台 query。 |

### 当前 BookInfo 壳与数据门槛

`OverlayBookInfo.vue → BookInfoDialog.vue → BookInfoPanel.vue` 是唯一宿主，这一点可保留；当前封面、标题/分类、作者、来源、最新章节、追更、分组、本地更新和简介的顺序与权限门槛总体等价。以下差异要在实现时一并收敛：

1. 远程结果若已在书架，必须在打开时替换为**实际书架记录**，不能拿搜索结果伪装成书架记录后再提供“查看详情”跳转。
2. 非书架 BookInfo 的“加入书架”必须落在与上游相同的属性/操作区域；`加入并阅读`、`查看详情`、`继续阅读`等上下文按钮不能成为普通 BookInfo 的第二业务流程。
3. 字数、进度、浏览器缓存、纯文本简介及多分类选择均为允许的 Vue 3/多用户/安全适配，但不得挤掉上游字段、改变已在书架判定或绕过取消加入。
4. `bookInfoVisible=false` 后应清除已消耗的旧链接 query intent；保留 `bookInfoBook` 缓存可以作为实现细节，但不得在后续 watcher 中重新打开已关闭的对话框。

### 后续测试闸门（先测试，再实现）

1. 为 `RemoteBookResultGroups` 写交互契约：封面仅发 `preview`，正文发 `read`；在 1440×900、390×844、360×800 三种尺寸验证没有点击穿透。
2. 为远程临时阅读建立 API/Pinia 合同：搜索和探索共享同一 payload、临时阅读不执行 `POST /books/remote`、Reader 可加载目录/章节并保存临时进度；加入书架后再切换为用户书架记录。该项先用 `api-contract-compat` 提取当前 Go 可承载的接口，禁止为 UI 临时绕过授权或把远程数据写入其他用户书架。
3. 重写当前 `bookInfoRouteContract`：旧详情链接加载成功、404/403、关闭清 query、保留其他 query、再次导航不重开；兼容链接唯一允许“开始阅读”动作。
4. 浏览器：书架封面/正文、搜索和探索的封面/正文、阅读器信息按钮、旧链接共五条流程；同时验证加入取消零写入、加入成功一写入、关闭不改 Reader/mobile 工具层与无水平溢出。

在上述测试和实现完成前，P1-B 只能称为“搜索续页已完成，BookInfo 五入口仍在重建”，不能称为工作台搜索流程完全对齐。

### API 设计结论（2026-07-13；仍未实现）

现有 `/books/:id` Reader 管线只能读取已保存书籍，且 `POST /books/remote` 会创建 Book/Chapter 行、写入书架并广播，不能作为上游临时阅读的替代。已在 `docs/compat/api-contract.md` 定义三个新增的用户绑定远程会话端点：创建会话、读取会话目录、读取会话章节正文。会话只保存于有 TTL 的服务端运行时内存，复用现有受限书源抓取/变量规则，不把客户端传入的章节 URL 当作抓取目标，也不写书架、目录、进度、书签、缓存或备份。下一步必须先为该合同新增 Go/前端失败测试，再写 handler 或 Reader 分支。
