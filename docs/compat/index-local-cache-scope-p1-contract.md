# Index 本地缓存作用域与事务契约（P1）

审查日期：2026-07-27

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本文件只记录上游行为、OpenReader 差异、允许适配和实施闸门。按照
`readerdev-compat-inventory`，本轮审查完成前不修改应用代码。

## 1. 上游权威行为

| 场景 | 上游文件与方法 | 可见行为与状态 |
|---|---|---|
| 工作台入口 | `web/src/views/Index.vue` 的“本地缓存”分组 | 长驻 Index 侧栏直接显示总量，并提供“清空书源缓存 / 清空 RSS 源缓存 / 清空章节列表缓存 / 清空章节内容缓存”四个动作；不打开独立设置页或 Drawer。 |
| 初始统计 | `Index.vue#mounted`、`scanCacheStorage` | 进入 Index 后扫描 `window.$cacheStorage`；总量统计缓存存储中的值，四个分项按 key 是否包含对应名称统计。 |
| 清理 | `Index.vue#clearCache` | 只删除 key 以 `localCache@` 开头且包含目标分组名称的项，随后重新扫描；其它分组不受影响。 |
| 书源/RSS key | `web/src/App.vue#loadBookSource/loadRssSources` | key 分别为 `localCache@bookSourceList@<currentUserName>` 与 `localCache@rssSources@<currentUserName>`。 |
| 章节正文 key | `PopCatalog.vue#computeCachedCata`、`BookManage.vue#computeCachedCata` | key 为 `localCache@<bookName>_<author>@<bookUrl>@chapterContent-<index>`；上游默认单用户，章节 key 本身没有账号作用域。 |

上游没有定义多个账号在同一浏览器并存时的所有权，也没有异步扫描期间切换账号的状态转换。
因此 OpenReader 必须保留四分组心智和侧栏职责，同时按现有 JWT 多用户运行环境补充隔离。

## 2. 当前 OpenReader key 与职责

| 缓存类型 | 当前生产者/key | 目标所有权 |
|---|---|---|
| 书源列表 | `useAppSidebarSearch.js`：`bookSourceList@user:<id>` | key 中精确匹配当前作用域。 |
| RSS 源列表 | `RSSManager.vue`：`rssSources@user:<id>` | key 中精确匹配当前作用域。 |
| Reader 书籍/目录 | `readerDataCache.js`：`reader@user:<id>@book:<id>`、`reader@user:<id>@chapters:<id>` | `reader@<scope>@` 只归属于该账号；目录项进入“章节列表缓存”。 |
| 章节正文 | `bookChapterCache.js`：`user:<id>@<book>@<url>@chapterContent-<index>` | `<scope>@` 只归属于该账号；正文项进入“章节内容缓存”。 |
| 书架离线快照 | `bookshelf.js`：`bookshelf@getBookshelf:<request>:user:<id>` | 计入当前账号浏览器缓存总量，但不属于上游四个可单独清理分组。 |
| 旧版章节正文 | `<book>@<url>@chapterContent-<index>` | 无法证明所有者；已登录账号不得自动认领、读取、统计或通过账号分组动作删除。 |

浏览器缓存总量不是四个分项的简单相加：它应包含所有**能够证明属于当前账号**的
OpenReader 浏览器缓存（例如书架快照和 Reader 书籍数据），但必须排除其它账号、匿名作用域、
未知格式和无法证明所有者的旧版 key。

## 3. 差异矩阵

| 关注点 | 当前实现 | 判定 | 要求 |
|---|---|---|---|
| UI 所有权 | `AppLayout.vue` 的长驻侧栏拥有统计和四个浏览器清理动作，并额外提供服务器缓存动作。 | `technical-stack-equivalent`；服务器缓存为 `acceptable-change` | 保留单一侧栏入口，不恢复已删除的通用 Settings Drawer。服务器缓存继续只作用于当前 API 用户。 |
| 空分组可见性 | 章节列表/正文按钮始终存在，但书源/RSS 按文件数过滤；空缓存时两个上游按钮消失。 | `must-fix` | 四个上游分组动作始终显示；空分组点击只提示为空，不发起删除。 |
| 总量作用域 | `currentBrowserLocalCacheStats()` 在分类前把 `listBrowserCacheKeys('')` 返回的每个 key 都计入 total。 | `must-fix` | total 只计入调用开始时捕获的账号可证明拥有的 key；其它账号和未知/旧版无归属 key 不得进入数量或字节数。 |
| 书源/RSS 分组 | `cacheGroupForKey` 只判断是否包含 `bookSourceList` / `rssSources`，不判断账号。 | `must-fix` | 必须精确识别 `<group>@<captured-scope>`；不能使用包含匹配删除另一账号数据。 |
| 章节列表分组 | 含 `chapterList` 的 key 无条件归类；`@chapters:` 分支在每次异步分类时重新读取当前 scope。 | `must-fix` | 仅 `reader@<captured-scope>@chapters:` 和明确兼容且可归属的格式进入当前账号分组。 |
| 章节正文分组 | 当前作用域 key 可识别，但无作用域旧 key 被每个账号视为自己的缓存。 | `must-fix` | 已登录账号只识别 `<captured-scope>@...@chapterContent-`；无归属旧 key 不得自动迁给任意账号。 |
| 旧缓存读取 | `loadBrowserChapterContent` 在 scoped miss 后读取无作用域正文并复制到当前账号；统计、批量计数和清理也包含旧 key。 | `must-fix` | 已登录状态不读取、不复制、不计数旧版无归属正文；只保留匿名/迁移工具显式处理的可能性。本改动只舍弃可重建缓存命中，不删除书籍、进度或用户数据。 |
| 扫描作用域冻结 | `currentUserScope()` 在异步 key 分类过程中动态读取。 | `must-fix` | 统计/清理开始时捕获一次 identity；列举、读取、分类和删除始终使用该 scope。账号切换不能改变已开始操作的目标。 |
| 请求所有权 | `useAppCacheManagement#loadStats` 没有 generation/identity gate；旧请求可覆盖新账号或较新刷新，旧 `finally` 也可关闭新 loading。 | `must-fix` | 服务器和浏览器统计必须作为同一代操作提交；只有最新且 identity 未变的操作可写 stats/loading。 |
| 清理所有权 | 确认框之后才由底层动态读取 scope；完成提示与后续刷新没有账号 gate。 | `must-fix` | 点击动作时捕获 identity；确认、清理、提示、刷新和 busy 状态只归属该操作。账号变化后不得清理新账号缓存，也不得把旧账号结果显示到新账号。 |
| 失败语义 | 单个统计源失败时对应部分回退为空，另一个仍可显示。 | `acceptable-change` | 保留局部容错，但旧/失效操作不得用空值覆盖当前有效统计。 |

## 4. 前端状态事务

### 4.1 统计

1. `loadStats` 开始时创建带 `generation + scope + token` 的操作快照。
2. 把捕获的 `scope` 显式传给浏览器统计函数；服务器请求使用开始时的认证请求。
3. 等待服务器与浏览器结果；局部失败只清空该部分。
4. 只有操作仍为最新且当前 identity 与快照完全一致时，才一次性提交两部分统计。
5. 旧操作的成功、失败和 `finally` 均不得修改新操作的 stats 或 loading。
6. token/账号变化时立即失效旧操作并清空旧账号可见统计；新账号进入工作台后重新统计。

### 4.2 浏览器分组清理

1. 点击时读取当前分组数量并捕获 identity。
2. 空分组仅提示“为空”，不进入确认和删除。
3. 确认完成后仍须验证 identity；若已变化，操作静默失效。
4. `clearBrowserLocalCacheGroup(group, capturedScope)` 只能删除该 scope 的目标分组 key。
5. 删除完成后只有同一操作仍有效时才显示成功并启动新一代统计。
6. 新统计的 generation 高于清理前统计；较旧刷新不能回填已删除项。

### 4.3 服务器缓存清理

服务器删除继续由请求所携带的 JWT 决定用户。账号在请求发出后变化时，旧请求可以完成旧账号的
服务端事务，但前端不得向新账号显示旧结果、刷新旧统计或关闭新账号的 busy 状态。

## 5. 数据与安全边界

- 不修改 SQLite、GORM 模型、API、Docker volume、`data/`、`cache/`、`library/`、备份或 WebDAV 格式。
- 不重命名已有 scoped browser key；当前账号现有缓存继续命中。
- 旧版无作用域章节内容是可重新抓取的派生缓存，不是书籍、目录、书签或阅读进度。
- 因无法从 key 或 value 证明所有者，禁止在登录后“首次使用者自动认领”。这避免账号 B 读取账号 A
  留在同一浏览器中的正文。旧 key 可原样留存，不因升级被破坏性删除。
- 清理分组不使用模糊子串作为所有权判定；未知 key 失败关闭（fail closed）。
- value 大小估算失败只按 0 字节计算，不能扩大删除范围。

## 6. 先测后改闸门

### 6.1 纯函数与缓存层

- 当前 scope 的书源、RSS、Reader 目录、章节正文能精确分组。
- 其它账号、匿名、名字中碰巧包含分组词的未知 key 均不分组。
- total 包含当前账号书架/Reader/四分组缓存，排除其它账号与无归属旧 key。
- 清理计划只返回当前 scope 的目标分组 key。
- 统计或清理过程中更换 `currentUserScope()` 不改变已捕获目标。
- 已登录 Reader 不读取、复制、计数或批量清理无作用域旧正文。

### 6.2 controller

- 两次并发统计后发先回时，只有新一代结果可提交。
- 统计中更换 scope/token 后，旧成功、旧失败和旧 `finally` 都不能写入。
- 确认期间切换账号时不调用浏览器清理；删除期间切换账号时不显示旧成功、不刷新新账号统计。
- 服务器清理与浏览器清理的 busy 状态由各自最新操作拥有。
- 单侧统计失败仍保留另一侧结果。

### 6.3 真实浏览器

在 `1440×900`、`390×844`、`360×800` 验证：

- 侧栏仍是唯一“本地缓存”入口，四个上游分组和允许的服务器缓存动作均可见。
- 注入账号 A、账号 B、无归属旧版和未知 key 后，账号 A 的总量/分组不包含 B 或旧版数据。
- 清空账号 A 的任一分组后，A 对应 key 删除，B、旧版、未知和其它分组保持不变。
- 延迟账号 A 统计并切换到账号 B 时，A 结果不会覆盖 B，也不会提前结束 B 的 loading。
- 移动侧栏的 260px 外壳、270px 拖动范围、底部 GitHub/昼夜按钮固定和点击隔离不回退。

## 7. 实施顺序

1. 提交并推送本契约以及总矩阵摘要。
2. 先增加上述失败测试，不以当前测试为正确行为依据。
3. 抽出显式 scope 的 key 所有权/分组纯函数，修正统计、清理和旧章节读取边界。
4. 用 `createAuthenticatedOperationGuard`（或等价的 scope+token generation guard）收敛
   `useAppCacheManagement`，并在 `AppLayout` 账号变化时重置/重载。
5. 跑 frontend 全量测试、生产构建、backend 全量测试和真实浏览器三视口契约。
6. 形成可独立验收批次后提交并同步 GitHub；本地构建 Docker，完成 volume/backup 闸门后再发布。

## 8. 初次实施记录（历史）

状态：代码、自动化验证与三视口真实浏览器检查均已完成。

- `localCacheStats.js` 现在先按调用时捕获的 scope 判定 key 所有权，再读取 value、统计或删除；
  书源/RSS 使用精确 key，Reader/章节/书架使用各自已部署的 scoped 形态，未知和无归属 key
  失败关闭。
- `useAppCacheManagement` 复用 `createAuthenticatedOperationGuard`，把 server/browser stats、
  clear、提示和 busy 状态绑定到 scope+token+generation；`AppLayout` 在 token 变化时重置旧状态
  并触发新账号统计。
- 四个上游浏览器缓存动作现在始终可见。服务器缓存仍是明确允许的当前用户扩展。
- Reader、书籍管理和删除收敛只读取、计数或删除 scoped 章节正文；旧无作用域项原样保留在
  浏览器存储中，但任何登录账号都不会自动认领。
- 聚焦测试先在旧实现上失败，修正后 14 项缓存契约通过；frontend 全量 587 项、生产构建和
  backend 全量测试已通过。当时三视口 browser smoke 只完成语法检查，首次启动无头 Chromium
  的外部权限申请因审批服务断线被拒，因此未把 browser gate 标记为完成；该缺口已由下一节闭环。

## 9. 2026-07-28 浏览器门禁闭环

- `scripts/smoke/index-cache-scope-contract.mjs` 已在 `1440×900`、`390×844`、`360×800`
  真实 Chromium 中通过。
- fixture 同时放入当前账号、其它账号、无归属旧章节和名称子串碰撞 key；统计只包含当前账号，
  旧版错误全局书源缓存不被读取，新版 owner-v1 key 由当前账号网络响应写入。
- 清理书源缓存只删除当前账号的 legacy/versioned 书源 key，其它账号、无归属旧章节和未知
  碰撞 key 均保留；延迟旧统计不能覆盖较新的 generation。
- 三个视口均无横向溢出，输出为
  `source-key-v1=true legacy-unread=true current-only=true stale-generation=true scoped-clear=true`。
- 本批关闭的是此前因浏览器启动权限中断留下的证据缺口；运行时代码已包含在后续
  `59e11a9` 发布镜像中，因此不为纯测试/文档变更重复发布 Docker。
