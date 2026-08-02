# 书架可见布局第二轮固定基准契约（P1）

审查日期：2026-08-02

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**audit-complete / implementation-pending**

本文件只审查普通 Index 书架的标题、编辑搜索、分组标签、书籍卡片、加载/空态、昼夜和响应式
布局。书架网络新鲜度、阅读进度、最后更新时间、BookManage、BookGroup manager、BookInfo、搜索/
探索结果页已经有独立合同，不因本文件重复打开。

按照 `readerdev-compat-inventory`，本轮先完成固定源码取证和差异判定；合同单独提交前不修改应用
代码或用当前测试反向定义产品行为。

## 1. 权威文件

| 合同层 | 固定上游 | OpenReader 当前映射 |
|---|---|---|
| 普通书架 DOM、状态、事件 | `web/src/views/Index.vue` 的 `.shelf-wrapper`、`bookList`、`showShelfBooks`、`getShowShelfBooks`、`toDetail`、`showBookInfoDialog` | `frontend/src/views/Home.vue` |
| 分组默认值与持久化 | `web/src/plugins/config.js#shelfConfig`、`plugins/vuex.js#setShelfConfig` | `utils/bookGroups.js`、`stores/preferences.js` |
| 卡片数据与顺序 | `plugins/vuex.js#shelfBooks`、Index `dateFormat` | `stores/bookshelf.js`、`utils/bookOrder.js`、`Home.vue` |
| 封面能力 | Index `getCover/getBookCoverUrl`、`el-image fit="cover" lazy` | `BookCover.vue`、安全 cover capability |
| 响应式与夜间 | `Index.vue` scoped Stylus、`@media max-width:750px`、`.night` | `Home.vue` scoped CSS、`html.dark-reader` 外壳 |

## 2. 固定上游状态与可见合同

### 2.1 标题、计数与编辑搜索

- 初始 `showBookEditButton=false`、`shelfSearch=""`。
- 普通书架标题依次显示 `书架 (当前 bookList 数量)`、编辑/取消、刷新/刷新中、RSS、书海。
- `bookList` 是先按当前书架分组筛选，再以 `shelfSearch.trim().toLowerCase()` 对书名或作者做精确
  子串筛选后的列表。因此标题数量必须随分组和编辑搜索实时变化，不是账号书架总数。
- 只有普通模式显示编辑、RSS、书海；非普通模式仍显示刷新。
- 编辑态只新增搜索框和每卡删除/编辑图标，同时隐藏未读 badge；退出编辑态隐藏搜索框，但上游不
  清空搜索词，重新进入仍使用原值。
- 卡片主体点击进入 Reader；封面点击阻止冒泡并打开 BookInfo。

当前 `Home.vue` 的动作顺序已经在上一合同恢复，但计数固定取 `bookshelf.books.length`，属于
`must-fix`。当前编辑搜索复用会删除空白和标点的本地书仓搜索归一化，也超出上游精确子串语义，
属于 `must-fix`；独立本地书搜索入口不受影响。

### 2.2 分组标签

- 默认持久选择是固定上游 `showBookGroup=-1`（OpenReader 等价 token 为 `builtin:all`）。
- 只渲染 `show=true` 且当前有书的内置/自定义分组，按 `order` 排序；tab 只显示名称。
- 上游 Element tabs 使用 `stretch`，占满可用宽度；容器为 `padding:5px 0`、`margin-bottom:10px`，
  不额外绘制一条自定义横贯底边。
- 隐藏、变空或未知的持久 token 不被自动改写；正文仍按该 token 筛选。只有没有任何可见非空
  分组时临时使用“全部”语义。这一状态机已由 BookGroup 第二轮合同关闭，本轮只保护其可见布局。

当前按钮 tab 可作为 Vue 3 等价结构，但 48px 固定行、自定义整栏 border、126px 轨道和缺失的
10px 下间距不是固定上游外观，判定 `must-fix`。ARIA tab/键盘语义是可保留的无障碍增强。

### 2.3 书籍卡片内容与动作

每张普通书架卡片按以下顺序显示：

1. `84×112`、`object-fit:cover` 的封面；
2. 右上角未读 badge，或编辑态的删除、编辑图标；
3. 最多两行书名；
4. 一行 `作者`、可选分隔点 `•`、可选 `共N章`；
5. 可选一行 `已读：章节名`；
6. 可选一行 `相对 lastCheckTime：最新章节名`。

未读数是 `totalChapterNum - 1 - durChapterIndex` 的正值，最大显示 99；最后章节前的时间只取
`lastCheckTime`。这些数据语义已经有专项合同，布局重建不得换成添加时间、元数据更新时间或其它
字段。

当前把作者和章节数合并成一个 `small` 字符串并使用 `·`，缺少上游独立 `.sub/.dot/.size` DOM；
桌面卡片 padding、高度分配、操作按钮位置、文本权重与截断也不同，均为 `must-fix`。共享
`BookCover` 的 capability URL、错误回退和 alt/键盘支持是安全/无障碍适配，可保留，但书架场景
必须覆盖为上游尺寸、直角、无额外阴影的可见几何。

### 2.4 桌面几何

固定源码的桌面几何为：

- shelf：`padding:48px`、满高 flex column、白天 `#fff`；
- 标题：`20px/600`、`margin-bottom:5px`、最小宽度 320px；动作 `14px/28px`、左间距 10px；
- books wrapper：占剩余高度、仅纵向滚动、隐藏 scrollbar；
- grid：`repeat(auto-fill, 380px)`、`justify-content:space-around`、`gap:10px`；
- card：横向 flex、`width:360px`、`padding:24px`、`margin-bottom:18px`；
- info：高度 112px、左间距 20px、space-between；操作区 `right:5px/top:0/font-size:24px`；
- name：`16px/700`、最多两行、普通/编辑分别为 38/62px 右避让；
- sub：`12px/600/#969ba3`；已读/最新：`13px/500/#6b6b6b`、单行截断。

在 `1440×900` 且 260px 侧栏可见时，48px 两侧内边距后的固定 380px 轨道只能形成两列；当前
`minmax(320px,360px)` 会形成三列，属于结构级 `must-fix`。在 `1024×1366` 自适应桌面模式下也
必须按同一固定轨道计算，不可用平板专属两列掩盖差异。

固定源码没有桌面网格/列表切换。上一批虽然已移除标题按钮，但 `preferences.shelf.view=list`
仍能驱动全宽列表，且用户已没有入口恢复。该残留属于 `must-fix`：所有历史 list 值必须无损迁移
到固定 grid；稳定 group token 保留。可以继续在兼容设置 payload 中写固定 `view:grid`，但不能再
影响可见结构。

### 2.5 移动几何

固定上游只在 `<=750px` 进入移动书架：

- shelf 去除 48px padding，顶部加入 safe-area；标题 `padding:20px 24px 0`；
- 分组容器左右各 24px；其 `padding:5px 0` 和 `margin-bottom:10px` 继续成立；
- grid wrapper 改为纵向 flex；
- 卡片 `box-sizing:border-box;width:100%;margin-bottom:0;padding:10px 20px`；
- 封面和 info 仍为 `84×112` 与 20px 间距，信息仍按桌面相同顺序和 space-between 分配。

当前移动卡片主尺寸大体一致，但额外的 520px 第二套断点、分组横线、操作区贴到卡片最右 12px、
多套重复选择器和缺失 10px 分组下间距会导致尺寸/层级漂移，判定 `must-fix`。为避免窄屏溢出的
`min-width:0`、安全 flex shrink、标题动作区局部滚动可以作为技术适配保留，但真实结果必须在
390×844、360×800 无横向溢出且不改变上游卡片内边距。

### 2.6 加载、空态与夜间

- 上游初次加载和显式刷新都在 `.books-wrapper` 上显示 Element loading overlay，文案分别是
  `正在获取书籍信息`、`正在刷新书籍信息`；已有卡片可留在 overlay 下，不替换为假卡片。
- 请求完成后的空书架或空分组只是空的 `.wrapper`；没有 `el-empty` 插图和“暂无书籍/这个分组里
  还没有书/没有匹配的书籍”等 OpenReader 自造文案。
- 夜间 shelf 为 `#222`，标题/书名为 `#bbb`，操作和 sub 为 `#6b6b6b`，已读/最新为
  `#969ba3`，搜索输入为 `#444`。夜间不得继续渲染白色 shelf、标题或 tab 背景。

当前初载使用 8 个 skeleton card，空结果使用 `el-empty`，且 Home 多处硬编码 `#fff` 而没有
`html.dark-reader` 对应样式，均为 `must-fix`。

## 3. 差异矩阵

| 关注点 | 当前状态 | 判定 | 实施要求 |
|---|---|---|---|
| 标题数量 | 始终显示书架总数 | `must-fix` | 显示当前分组和编辑搜索后的 `bookList.length` |
| 编辑搜索 | 删除标点/空白后模糊匹配 | `must-fix` | 恢复 trim + lowercase 精确子串 |
| 分组可见状态 | token 状态已复核 | `aligned` | 不改写隐藏/空/未知 token |
| 分组可见样式 | 自定义 48px/126px tab 和横线 | `must-fix` | 恢复 stretch、padding 和下间距 |
| 桌面布局 | 320–360 自适应三列或历史全宽 list | `must-fix` | 固定 380px grid；迁移 list 到 grid |
| 移动卡片 | 主尺寸接近，但有重复断点和操作区偏移 | `must-fix` | 单一 750px 合同，恢复上游内部几何 |
| 卡片元数据 | 作者/章数合并为一行字符串 | `must-fix` | 恢复 sub/dot/size 和精确截断 |
| 封面请求/回退 | capability + shared fallback | `acceptable-change` | 保留安全能力，覆盖书架可见阴影/圆角 |
| role/键盘/ARIA | 当前额外提供 | `acceptable-change` | 保留且不得制造双重点击 |
| 加载 | skeleton 替换真实卡片区域 | `must-fix` | 目标区域 loading overlay 与精确文案 |
| 空态 | 自造 el-empty/三套文案 | `must-fix` | 恢复空 wrapper |
| 夜间 | shelf/tab 仍硬编码白色 | `must-fix` | 恢复固定上游夜间表面和文字层 |

## 4. 先测后改闸门

### 4.1 单元与静态合同

- 替换会固化 `layoutVersion:2 + list` 的测试；历史 list 输入必须得到固定 grid，同时保留 groupKey
  和账号作用域/迟到结果隔离。
- `Home.vue` 不再读取 `shelf.view` 决定 DOM，不存在 `list-view` 可见分支。
- 标题数量使用当前 displayed list；编辑搜索只做 trim/lowercase；退出/重进编辑保留查询。
- 卡片 DOM 固定 cover → operation → name → sub/dot/size → read → latest 顺序。
- loading/empty/night 静态结构和文字固定；不再存在 skeleton/`el-empty`。
- 现有 `homeMobileShelfGeometry.test.mjs` 必须升级为桌面、移动、night 和加载状态合同，不能只搜索
  几个 CSS 字符串。

### 4.2 真实浏览器

在 `1440×900`、`1024×1366`、`390×844`、`360×800` 验证：

- 1440 桌面固定两列，1024 自适应桌面固定一列；手机固定一列；卡片间不重叠；
- 桌面/手机 cover、card padding、info 间距、标题/metadata 截断和操作区位置；
- 分组 stretch、隐藏 token、空 token、选中 underline 和标题计数；
- 编辑搜索的精确匹配、查询保留、图标/badge 互斥、cover/row 点击所有权；
- 初载与刷新 loading overlay 文案，完成后的空 wrapper；
- 日间和夜间 shelf/card/tab/input 的实际 computed color；
- 四个视口 `scrollWidth <= innerWidth + 1`，滚动宿主与移动侧栏不回退。

至少一条浏览器场景必须把 `view:list/layoutVersion:2` 写入当前账号偏好，证明升级后仍渲染固定
grid；不能只用默认 grid 夹具。

## 5. 实施顺序

1. 单独提交并推送本合同及总矩阵摘要。
2. 使用 `frontend-ux-compat` 和 `openreader-frontend` 先增加预期失败的布局/偏好/状态合同。
3. 重建 Home DOM/CSS；只在必要处调整 shelf 偏好迁移，不改书架数据、新鲜度或 API。
4. 跑 frontend、Go、production build、四视口专项和受影响的 Index/BookGroup/多客户端烟测。
5. 通过新旧卷/备份门后由本机构建发布 Docker；报告明确本批只签收普通书架可见布局。
