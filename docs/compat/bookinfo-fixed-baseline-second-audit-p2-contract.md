# BookInfo 第二轮固定基准复审与重建合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

审计日期：2026-08-02。

状态：**audit-complete / implementation-pending**。本文件是在不修改应用代码的只读阶段生成；
旧组件、旧合同和既有测试均不构成保留依据。

## 1. 权威源码与当前映射

上游权威：

- `web/src/components/BookInfo.vue`：唯一弹层结构、书架身份、封面、字段顺序、简介、动作与文案。
- `web/src/App.vue#showBookInfoDialog`：根级唯一实例及打开状态。
- `web/src/views/Index.vue#showBookInfoDialog/addBookToShelf/saveBook`：搜索/探索结果卡与 BookInfo
  的两个不同加书入口。
- `web/src/views/Reader.vue#showReadingBookInfo`：Reader 先按 `bookUrl` 合并书架记录再打开。
- `web/src/components/BookManage.vue#showBookInfo/setBookGroup`：父管理器与 BookInfo/BookGroup 叠层。
- `web/src/plugins/vuex.js#dialogSmallWidth`：桌面 `500px`、mini `85%`，而 BookInfo 在 mini
  最终使用 fullscreen。

当前映射：

- `GlobalOverlayHost.vue -> OverlayBookInfo.vue -> BookInfoDialog.vue -> BookInfoPanel.vue`。
- `Home.vue`、`Search.vue`、`Discover.vue`、`Reader.vue`、`OverlayBookManagement.vue` 与旧链接
  hydration 均调用 `overlay.openBookInfo()`。
- `useOverlayBookInfo.js`、`useOverlayBookCacheState.js`、`useBookInfoAddToShelf.js`、
  `OverlayBookAddToShelf.vue`、`RemoteBookResultGroups.vue`。
- `POST /api/books/remote`、`PUT /api/books/:id`、`POST /api/uploads`、
  `POST /api/books/:id/refresh-local`、`PUT /api/books/:id/category`。

唯一根级 BookInfo 和五类入口已经收敛，这一点保留；当前内部结构、动作所有权和旧测试仍有
固定基准偏差，必须重建。

## 2. 上游可见结构合同

| 项目 | 固定上游行为 | 当前行为 | 裁决 |
|---|---|---|---|
| 根与可见 gate | App 只挂一个 BookInfo；`show=false`，事件先替换当前书再打开；正文只在 `show` 时挂载，关闭只隐藏弹层。 | 唯一全局 overlay 已成立；账号失效会额外安全清空。 | `acceptable-change`：保留唯一实例、会话清理和旧链接 intent。 |
| 几何 | 桌面宽度精确 `500px`，Element 默认 top；mini fullscreen。 | 桌面固定 `480px`；mini predicate/fullscreen 已等价。 | `must-fix`：恢复 500px；在 1024×1366 与 1366×1024 上不得误入 fullscreen。 |
| 内容骨架 | 单列：150px 全宽封面区 → 居中书名 → 红色 kind → 属性 → 简介。 | dialog 视觉分支勉强覆盖上游，但源组件还保留未使用的两列 `detail` variant、状态、章节、进度和缓存骨架。 | `must-fix`：删除无调用者的详情 variant 和 dead props，以唯一上游结构为组件本体。 |
| 前景封面 | 图片只固定高度 150px，宽度按原比例，不裁剪；背景用同一封面全区 blur(50px)。 | 共享 BookCover 被强制为 `100×150` 且 `object-fit:cover`，会裁掉横向或窄幅封面。 | `must-fix`：BookInfo 前景恢复 height-only/natural-ratio；背景仍走安全 capability URL。 |
| 封面提示 | 上游没有“更换封面/上传中”遮罩文字。 | 当前可见遮罩覆盖封面底部。 | `must-fix`：删除额外提示；上传期间只允许非侵入 loading/cursor。 |
| 书名 | `16px/500`、居中、`padding:10px 0`；夜间显式浅色。 | dialog 分支接近，但由通用 h2 与主题变量间接实现。 | `must-fix`：固定上游 class/几何并保留夜间可读性。 |
| kind | 仅按英文逗号拆分，过滤空项，完整显示全部项；每项红色文本，无胶囊边框。 | 同时按中英文逗号、竖线、斜线和顿号拆分，trim 并截断到 8 项。 | `must-fix`：恢复英文逗号和不截断语义；Vue 文本节点替代上游 `v-html`。 |
| 属性顺序 | 作者 → 来源（本地可更新）→ 最新（书架态右侧追更）→ 分组或加入书架。 | 额外显示字数；若字段缺失还可显示 `-`、`远程书籍`。 | `must-fix`：dialog 删除字数及所有额外 stats；最新缺失为空，未知远程源精确为“未知书源”。 |
| 追更 | 只要已在书架就显示，包括本地书；与最新章同一 flex 行，右侧区域宽 80px。 | 仅 `sourceId>0` 显示，因而本地书缺少该上游控件。 | `must-fix`：恢复所有书架书可见；Go 精确字段更新仍保留。 |
| 分组 | 书架态显示全部命中自定义组，以英文逗号连接；无命中显示内置未分组名或“未分组”；“设置分组”右浮。 | 多对多名称以顿号连接。 | `must-fix`：显示分隔和布局恢复；Category 多对多是允许的数据适配。 |
| 简介 | `intro || 暂无简介` 按每个 `\n` 逐行生成段落，保留空行，只去每行开头空白并在正文前加入六个 nbsp；line-height 1.6，桌面/mini 使用上游动态 max-height。 | trim 全文、合并连续换行、删除空段，并用 2em text-indent；固定结构还继承通用详情样式。 | `must-fix`：恢复逐行和空行语义；仍用转义文本节点，不能恢复可注入 HTML。 |
| 夜间 tag | 加书 tag 的 effect 随日夜 light/dark。 | 固定 light。 | `must-fix`：接入当前安全主题状态并复刻 effect。 |

## 3. 书架身份与打开状态

上游唯一身份是 `bookUrl`：BookInfo 的 `isInShelf` 以及 Reader 打开前的 shelf merge 都按 URL
查找。当前 `isShelfBook/findShelfBook` 先按数值 ID，再按 URL。搜索/探索/临时 Reader 对象可能
携带来源内部 ID 或 `id=0`；若恰与书架 ID 碰撞，当前会把一本不同 URL 的远程书替换成无关书架
记录，并错误显示追更/分组而隐藏加书按钮。

实施必须：

1. 对有 `url/bookUrl` 的对象始终以规范 URL 为权威，不允许 ID 覆盖 URL 不同的结果。
2. 只有输入没有 URL、且来自受信任的 `/books/:id` 兼容 hydration 时才允许 ID fallback。
3. shelf/分类/书源后台刷新可以保留，但打开弹层不能等待这些请求才渲染已有 BookInfo。
4. 删除当前打开时不可见的浏览器章节缓存统计。上游不显示该字段，该 IndexedDB 扫描也不应
   成为每次打开 BookInfo 的隐形工作；BookManage 自己的缓存统计职责不受影响。
5. 分类、书架或书源同步后，当前弹层继续收敛到同 URL 的最新服务器投影；账号 generation
   失效后，迟到响应不得恢复旧账号对象。

## 4. 两种“加入书架”动作不能再互换

固定上游明确存在两个入口：

| 入口 | 上游状态机 | 当前状态机 | 裁决 |
|---|---|---|---|
| 搜索/探索结果卡的“加入书架” | 结果卡自身显示 tag；点击先打开“设置分组”，取消不写入，确认后保存所选分组。 | `RemoteBookResultGroups` 完全没有加书按钮。 | `must-fix`：恢复结果卡独立 tag，点击必须 stop，不触发临时阅读。Search 与 Discover 复用同一控制器。 |
| 未入架 BookInfo 的“加入书架” | BookInfo 内 tag 直接 `saveBook(showBookInfo,true)`，不打开分组选择；通常以未分组加入。 | BookInfo 接管了上一个入口的分组选择弹层，取消/确认后才写入。 | `must-fix`：直接创建/解析并 upsert；不得打开 `.book-add-category-dialog`。 |

这不是“统一交互优化”：它交换了上游两个可见动作的所有权。此前
`bookinfo-action-ownership-cleanup-p1-contract.md`、`api-contract.md` 和测试中把“唯一
BookInfo category-confirmed transaction”视为正确的段落，由本合同取代。

OpenReader 可继续让两条入口共用一个安全的 remote-book mutation utility，但调用策略必须不同：

- BookInfo 传空分组（或对象已有、合法且当前用户可见的分组），直接调用
  `POST /api/books/remote`。
- Search/Discover 先运行现有 category chooser，再调用同一 API。
- 新建返回 `201`、URL 已存在返回 `200`；成功只用服务端投影 upsert，并广播当前用户。
- BookInfo 精确成功文案恢复“加入书架成功”，失败前缀“加入书架失败”；结果卡沿用同一上游文案。
- 临时 Reader 的 BookInfo 直接加入后保持当前 reader route 和工具层；结果卡确认加书后仍停留在
  Index 搜索/探索场景。
- `POST /api/reader/remote-sessions` 仍只负责临时阅读，不能因结果卡或 BookInfo 打开而持久化。

## 5. 其余动作与精确反馈

| 动作 | 上游合同 | OpenReader 实施边界 |
|---|---|---|
| 封面 | 所有封面看起来可点击；JPG/JPEG/PNG，上传失败“上传文件失败”，保存使用普通“操作成功/操作失败”。 | 出于 JWT 用户资产归属和“不用隐式封面操作绕过明确加书”的边界，仍只允许已入架记录上传，这是 `acceptable-change`；删除可见遮罩，保留精确 `{customCoverUrl}` 更新、capability URL、8MiB/内容校验和旧资产兼容。 |
| 本地更新 | 仅 `origin=loc_book` 显示“更新”；成功“更新成功”，失败“更新失败…”，弹层保持打开。 | 保留 stage→transaction→promote、进度/书签恢复、缓存失效和广播；只恢复可见文案及按钮 loading，不改 archive。 |
| 追更 | switch 变更后保存，成功“操作成功”，失败“操作失败…”。 | 保留精确 `{canUpdate}`、用户隔离和服务端投影；覆盖远程与本地书架书。失败恢复服务端值。 |
| 设置分组 | 打开同一个 BookGroup set 模式，不关闭 BookInfo；BookGroup 关闭后 BookInfo 仍在。 | 当前叠层所有权保留；BookGroup 第二轮合同继续生效。 |
| 关闭 | header close/遮罩/ESC 只关闭 BookInfo；不改变 Index 场景、Reader route、移动工具层或父 BookManage。 | 保留会话清理、删除消费者收敛和兼容 route intent 的安全处理。 |

## 6. API、数据与安全合同

本批不新增路由、不迁移 SQLite、不改 `data/cache/library`：

- `POST /api/books/remote` 必须接受省略或空 `categoryIds`，创建未分组书；URL 已存在且请求未
  提供任何 category 字段时，不得清空既有分组。来源、URL、变量和用户 namespace 仍验证。
- URL 身份冲突测试必须证明：同用户书架 ID 与远程结果 ID 相同、URL 不同时，BookInfo 不能
  读取或更新错误书籍。
- `PUT /api/books/:id` 的追更和封面仍只发送一个字段；其他客户端并发保存的标题、简介、分组
  与进度不能被旧快照覆盖。
- 上传仍限制真实图片类型、大小、用户根和路径；legacy 全局 URL 只读；portable v2 资产重写
  和 mounted-volume 合同不变。
- 本地 refresh 保持事务、失败无损、路径根校验和解析资源限制。
- HTML 简介仍作为纯文本渲染；不复制上游 `v-html` 的注入面。

## 7. 必须替换的错误测试

1. `bookInfoAddToShelfIntegrationContract.test.mjs` 当前要求 BookInfo 拥有 category chooser；改为
   锁定“结果卡确认分组、BookInfo 直接加入”及一套共享 mutation utility。
2. `bookInfoAddToShelf.test.mjs` 当前控制器把 `selectCategories` 设为强制第一步；拆出 direct 与
   confirmed 两种策略，并覆盖取消、失败、迟到账号响应和重复 URL。
3. `index-workspace-contract.mjs`、`remote-reader-contract.mjs` 当前明确要求 BookInfo 点击后出现
   `.book-add-category-dialog`；改为断言 BookInfo 直接 POST，而结果卡 add 才出现 chooser。
4. `api-contract.md` 当前把所有 search/explore add 约束成“canonical BookInfo category-confirmed”；
   改为区分结果卡 add、BookInfo add 和临时 reader session。
5. `BookInfoPanel` 静态合同新增：桌面 500px、唯一单列、150px natural-ratio cover、完整 kind、
   无 wordCount/stats/status/cover label、逐行简介、所有 shelf book 的追更、精确来源 fallback。
6. shelf identity 单元测试加入“相同 ID、不同 URL”反例，以及 URL 命中后使用最新 shelf 投影。
7. controller 单元测试删除 BookInfo 打开时的 cache-count 期待；BookManage 的 cache-count 测试保留。

## 8. 真实浏览器与发布门禁

浏览器必须覆盖 1440×900、390×844、360×800、1024×1366、1366×1024：

- 桌面 dialog 500px、mini fullscreen、两种 iPad 非 fullscreen 且 header close 可达。
- 上游字段顺序、自然比例封面、无裁切/额外字段、kind 全量、简介空行和滚动高度。
- shelf/local/unshelved 三态；本地也显示追更，本地更新、远程追更、封面和 BookGroup 叠层。
- Home、Search、Discover、Reader、BookManage、旧 `/books/:id` 六类入口仍只有一个 BookInfo。
- Search/Discover 结果卡 add 的取消/确认；BookInfo direct add 不出现 chooser；临时 Reader direct add
  不改 route 或工具层；ID 碰撞不误认书架态。
- 父 BookManage 保持打开；Reader 工具层保持打开；无点击穿透、根横向溢出和控制台异常。
- 无 API mock 的 Go/SQLite 流程验证 `201/200`、空分组、已有分组不被省略字段清空、精确 patch、
  多客户端投影和账号隔离。

最终还需通过 frontend 全量、Go 全量、Vite production build、`git diff --check`、新卷 portable
v1/v2 assets/跨用户/重启和历史 TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation。通过后允许作为
独立 Docker 用户验收切片发布。

## 9. 允许差异

仅保留：Vue 3/Pinia/Element Plus/Gin transport、JWT 多用户、Category 多对多、精确字段更新、
事务化本地刷新、认证 generation guard、封面 capability 和用户资产根、纯文本简介、旧 URL
兼容 intent、临时 Reader session、非阻塞的书架/书源后台收敛。未使用的详情 variant、480px、
裁剪封面、额外字段/标签、ID 优先、隐形缓存扫描及加书动作互换都不是允许差异。
