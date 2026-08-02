# Index 搜索与探索可见工作区第二轮固定基准契约（P1）

审查日期：2026-08-02

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**implementation-complete / validation-complete / Docker-pending**

本合同只覆盖 Index 侧栏搜索设置、远程搜索结果场景、书海入口选择器和探索结果场景的可见
结构与状态转换。搜索/探索解析器、稳定多源 cursor、失败书源缓存、临时 Reader、BookInfo、
加入书架事务和本地书仓搜索已有独立合同；本轮不得以这些能力已经可用为理由保留错误 UI。

按照 `readerdev-compat-inventory`，本轮先只读提取固定上游合同并以 `5d18871` 单独提交；随后才
进入测试和实现。当前状态段与第 7 节记录实现结果，正文中的“当前”均保留为合同提交时的审查
现场，不能再解释为实现后的代码现状。

## 1. 权威文件与当前映射

| 合同层 | 固定上游 | OpenReader 当前映射 |
|---|---|---|
| 搜索设置、结果状态、标题与卡片 | `web/src/views/Index.vue` 的侧栏 search、`searchBook`、`searchBookByEventStream`、`bookList`、`loadMore`、结果 card | `layouts/AppLayout.vue`、`useAppSidebarSearch.js`、`indexSearchPresentation.js`、`views/Home.vue`、`views/Search.vue`、`RemoteBookResultList.vue` |
| 书海选择器 | `web/src/components/Explore.vue`、`Index.vue` 中 `el-popover` | `ExploreWorkspacePopover.vue`、`AppLayout.vue` 的 chooser host |
| 探索结果与续页 | `Explore.vue#exploreBookSource/loadMore`、`Index.vue#showSearchList/loadMore` | `views/Discover.vue`、`stores/indexWorkspace.js`、`api/explore.js` |
| 结果点击与加入 | `Index.vue#toDetail/showBookInfoDialog/editBook/addBookToShelf` | `RemoteBookResultList.vue`、`RemoteBookJsonEditorDialog.vue`、`useRemoteBookResultEditor.js`、Search/Discover 的 temporary Reader、共享 BookInfo 和 add transaction |
| API/游标/解析 | `BookController.searchBook*`、`Explore.vue#getExploreGroup` | `POST /api/search`、`GET /api/explore/*`、Go source engine |

## 2. 固定上游状态机

### 2.1 单一 Index 场景

- 固定上游不创建独立搜索页或探索页；`isSearchResult/isExploreResult/searchResult` 只替换同一
  `.shelf-wrapper` 内的标题、分组/编辑搜索和书籍列表。
- 搜索结果标题为 `搜索 (N)`；探索结果标题为 `探索 (N)`。没有副标题、来源摘要、分隔线或
  第二层页面 header。
- 搜索结果标题动作的可见顺序是 `加载更多 → 书架`；探索结果是
  `书海 → 加载更多 → 书架`。点击书海先打开 chooser，不预先替换当前结果或书架。
- 返回书架清空结果、探索标志和 loading；侧栏、固定底部 GitHub/昼夜按钮与根 Index 现场不
  卸载。OpenReader 以 `Home` 内部子组件表达同一状态可作为 Vue 3 技术等价，但可见 DOM 和
  状态转换必须一致。

当前 canonical `/` workspace、旧 `/search`/`/discover` redirect、迟到请求 gate 和返回书架
不换根路由已经是技术等价；当前 Search/Discover 自建的大标题、副标题、底边线、不同滚动区和
页面级空态属于 `must-fix`。

### 2.2 搜索设置与触发

- 可见搜索方式只有两个，且顺序/文案为：`单源搜索`、`多源搜索(过滤书名/作者名)`。
- 单源时只显示书源选择器，不显示分组和并发选择器；没有选择书源时 Enter 提示
  `请选择书源进行搜索`。
- 多源时显示书源分组和并发选择器。分组第一项固定为 `全部分组 (全部书源数)`，随后按书源
  列表首次出现顺序列出非空组及数量；并发档位为 `12/18/24/30/36/42/48/54/60`。
- 空关键词 Enter 提示 `请输入关键词进行搜索`。开始新搜索立即进入结果场景、清空旧结果并将
  标题动作显示为 `加载中...`；结果区自身不叠加 skeleton、`el-empty` 或另一层 loading overlay。
- 结果场景中修改任一搜索设置会以当前关键词重新执行第一页；旧请求或旧流不得回写。
- 搜索结果按服务端返回顺序追加，以 `bookUrl` 做会话级去重。不得按来源重新分组，因为分组会
  把交错返回的 `A1,B1,A2` 改成 `A1,A2,B1`。

OpenReader 内部可继续用 `all/group/single`：可见“多源”在分组为空时写 `all`，选定组时写
`group`；这只是持久化兼容，不得再暴露第三个“分组搜索”模式。已部署 `8/16/32` 并发值继续
以“旧配置”显示，JWT REST 和 `hasMore` 终态按钮是已记录的允许适配。

当前可见三模式、分组选择器没有“全部分组”项、并发在单源也显示，以及设置变化不自动重搜，
均为 `must-fix`。

### 2.3 搜索与探索结果卡片

搜索和探索复用普通书架的固定 grid 与 card 几何：

- 桌面 `.wrapper` 为 `repeat(auto-fill, 380px)`、`space-around`、10px gap；card 为
  360px、24px padding、18px 下间距。
- 手机 `<=750px` 改为纵向 flex；card 为 `width:100%`、`box-sizing:border-box`、
  `padding:10px 20px`、无下间距。
- card 顺序固定为 cover → operation → name → author/dot/chapter count → latest chapter →
  `加入书架`；结果场景不显示已读行和未读 badge。
- 封面点击只打开共享 BookInfo；card/信息区点击进入临时 Reader；加入和编辑图标均阻止冒泡。
- operation 只显示一个编辑图标。它打开结果 JSON 编辑器；确认加入后才保存编辑后的书籍，取消
  零写入。底部 `加入书架` 仍走上游“设置分组/暂不加入”事务，两者不是同一个动作。
- 可见结果不按来源加 section header，也不额外显示来源 tag、更新时间、kind、word count、简介
  或“未知作者/暂无简介”等占位文本。
- 最新章节沿用同一卡片规则：有 `lastCheckTime` 时显示相对时间，否则显示 `最新`。

当前 `RemoteBookResultGroups` 按 source regroup，使用自动宽度 panel card，并增加来源标题、条数、
来源 tag、更新时间、类别、字数和两行简介；同时缺失上游编辑图标。该组件结构属于
`incorrect-refactor`，应删除或改为唯一扁平结果列表，不得在 CSS 上伪装。

共享 capability cover、图片失败回退、ARIA/键盘触发、用户绑定临时 Reader 和多分类加入确认是
允许差异，但不得改变上述点击所有权、列表顺序或可见字段。

### 2.4 加载、空态、夜间和滚动

- 初次搜索/探索请求期间，结果列表是空 wrapper；标题动作负责显示加载状态。
- 请求完成但无结果仍为空 wrapper；固定上游没有 `el-empty` 插图、搜索提示、来源摘要或
  “从书海选择……”占位。
- 结果列表与书架使用同一滚动宿主和滚动条隐藏规则。续页前保存 scrollTop，结果追加后恢复；
  进入新搜索、另一个探索入口或返回书架则使用新场景 generation 拒绝迟到写入。
- 夜间继续使用 shelf 的 `#222` 表面、标题/书名 `#bbb`、operation/sub `#6b6b6b`、
  latest `#969ba3`。工具层或 chooser 显隐不得重排结果卡片。

当前页面级 `v-loading`、两套 `el-empty`、硬编码 `#26394a` 标题和独立 result-card 夜间表面均为
`must-fix`。REST 取消/generation、防重复点击和显式禁用的 `没有更多了` 可以保留。

### 2.5 书海 chooser

- chooser 是 `探索书源`/结果标题 `书海` 共用的一个 `el-popover` 内容，不是新页面或 modal。
- 桌面 popover 宽度为 600px，最终 `top:0`，无 border/shadow；位于触发按钮右侧。桌面不显示
  内部关闭图标，仍可通过触发按钮或外部点击关闭。
- mini 界面最终 `left:0;top:0;width:100vw`，但内容高度仍由标题、分组和 300px data wrapper
  决定；不是 `100vh` 全屏 Drawer，也没有半透明 backdrop。mini 才显示 20px close icon。
- 内部 padding 为 24px并计算顶部 safe area；标题区下间距 20px；标题为 18px/400 红色下划线，
  右侧是当前筛选后的 `共N个可用书源`。
- source group 按首次出现顺序排列，并无条件在末尾加入 `未分组`；再次点击已选组恢复全部。
- collapse 使用数组，允许同时展开多个书源，不是 accordion；书源顺序不重排。关闭/重开保留
  group、展开项和 scrollTop。
- explore entry 按 `layout_flexBasisPercent` 每累计到 1 分行；普通文本规则按空行分组。每行
  `space-between`、横向可滚动，按钮间距和虚线分隔沿用 day/night 色。
- 选择 entry 后请求第一页；成功才把同一 Index 切到探索结果并关闭 chooser。失败留在 chooser
  并显示错误；续页继续绑定同一 source/url/page。

当前桌面 520px、靠触发器垂直定位、始终显示 ×；手机 `100vh`、modal backdrop/点击拦截；
accordion 单开；分组 locale 排序且按是否存在未分组决定按钮；entry flex-wrap；空态 `el-empty`，
均为 `must-fix`。由 Go 端安全解析 explore groups、认证 scope、请求代号和 loading 防重入可以作为
技术适配保留。

## 3. 差异矩阵

| 关注点 | 当前状态 | 判定 | 实施要求 |
|---|---|---|---|
| 根场景/兼容路由 | canonical `/` + 内部 workspace | `technical-equivalent` | 保留；不得恢复独立业务路由 |
| 结果标题 | 自建 header/subtitle/border | `must-fix` | 复用 shelf title 几何与精确动作 |
| 搜索可见模式 | all/group/single 三项 | `must-fix` | 只显示 single/multi；内部值兼容映射 |
| 设置变化 | 只保存，不自动重搜 | `must-fix` | 结果场景以当前关键词重启第一页 |
| 结果顺序 | 按来源 regroup | `must-fix` | 扁平保持服务端/会话顺序 |
| 结果卡片 | 富信息 panel、缺少编辑 | `must-fix` | 与 shelf 同 grid/card，恢复 editor/add 双动作 |
| 远程 loading/empty | overlay + el-empty | `must-fix` | 空 wrapper；标题动作承载 loading |
| chooser desktop | 520px、动态 top、始终 close | `must-fix` | 600px、top 0、桌面无内部 close |
| chooser mobile | 100vh modal + backdrop | `must-fix` | 100vw 顶部自适应高度、无 backdrop |
| chooser collapse/group | accordion + locale sort | `must-fix` | 多开数组、首次出现顺序、固定未分组 |
| REST/游标/会话隔离 | 现代 JWT/hasMore/request gate | `acceptable-change` | 保留既有 API 合同和稳定 cursor |
| 本地书籍搜索 | 独立 `mode=local` | `intentional-redesign` | 保留，但不得改变普通 Enter 远程流程 |

## 4. API、数据与安全边界

- 本批不修改 SQLite schema、`data/`、`cache/`、`library/`、backup 或 WebDAV 格式。
- `POST /api/search`、`GET /api/explore/sources`、`GET /api/explore/:sourceId`、远程会话和
  `POST /api/books/remote` 路径/字段保持不变；前端只重建可见投影和动作编排。
- Go 端解析 exploreUrl 替代浏览器 `new Function` 是安全增强，继续保留。它必须保持 JSON
  `layout_flexBasisPercent/layout_flexGrow`、空行分组、默认入口和源顺序合同。
- 结果 JSON editor 只能提交当前用户可访问的 source；API 继续校验 source ID、URL、字段大小和
  用户隔离。错误信息保持有界，不复制上游原始异常泄露。
- 旧并发值、旧 `/search`/`/discover` URL、本地搜索 mode 和工作台 generation 均无损兼容。

## 5. 先测后改闸门

### 5.1 静态/单元合同

1. 删除把 source group headers、富信息字段、520px chooser 和手机 100vh modal 固化为正确行为的
   断言；保留 cover preview/card read/add ownership与临时 Reader 合同。
2. 新结果组件必须是一个扁平 `v-for`，顺序不 regroup；DOM 顺序和字段固定，包含 stop-propagation
   的 edit/add，且不包含 source section/title/count/intro/kind/word-count/update-time。
3. 搜索可见选项只有 single/multi；multi group 包含全部分组，single 不渲染 concurrent；旧内部
   all/group 值和 8/16/32 并发值不丢失。
4. chooser 组序、多开 collapse、关闭后状态/scroll 保留、entry 行拆分和 mini/desktop close gate
   有可执行单元合同。
5. raw editor 覆盖非法 JSON、缺 name/url/source、取消确认、保存失败、成功只写一次及账号切换
   迟到结果隔离。

### 5.2 真实浏览器

在 `1440×900`、`1024×1366`、`390×844`、`360×800` 验证：

- 搜索设置 exact options/conditional rows、Enter、配置变更自动重搜和旧值保留；
- 搜索/探索标题动作、空 wrapper、初载/续页 loading、结束反馈和返回书架；
- 结果固定 2/1/1/1 列，与书架相同 cover/card/info 几何；交错来源顺序不变；
- cover → BookInfo、card → temporary Reader、add → category chooser、edit → JSON editor，四者互不
  穿透；取消零写、成功单写且留在结果现场；
- chooser 桌面 600px/top 0/无 close，手机 100vw 非 100vh/无 backdrop/有 close；多展开、分组
  顺序、scroll 保留、选择成功后才切场景；
- day/night computed colors、无横向溢出、侧栏关闭/拖动和固定底部按钮不回退。

现有 `index-workspace-contract.mjs` 中“手机 chooser 等于 viewport 高度并拦截中点”“桌面宽度不
超过 520px”“结果必须存在 source group card”都是冲突合同，必须先改成上述固定基准断言。

## 6. 实施顺序

1. 单独提交并推送本合同、gap 摘要和总矩阵状态。
2. 使用 `frontend-ux-compat`/`openreader-frontend` 先建立预期失败的 search setting、flat result、
   chooser 和 raw editor 合同。
3. 抽取唯一结果 card/list，并让 Search 与 Discover 共用；复用已通过第二轮审查的 shelf CSS
   tokens/几何，删除错误分组组件和重复页面样式。
4. 重建 visible single/multi 映射、结果场景自动重搜和 Explore chooser；不得修改既有 Go API、
   cursor、远程会话或多用户事务。
5. 跑 frontend、Go、build、四视口新专项和受影响的 Index/session/BookInfo/remote Reader smokes。
6. 形成可独立验收切片后提交推送；新旧卷/备份门通过再由本机发布 Docker。

## 7. 实施与验证记录

本轮按上述合同完成以下替换：

- 侧栏只呈现 `单源搜索`、`多源搜索(过滤书名/作者名)`；单源只显示书源，多源才显示按首次
  出现顺序生成的分组和并发设置。内部 `all/group/single`、旧并发值以及 local mode 继续无损
  映射；结果现场修改设置会保留关键词并重启第一页，旧 generation 不得回写。
- 删除 `RemoteBookResultGroups.vue`，由 Search 与 Discover 共用唯一
  `RemoteBookResultList.vue`。远程结果保持服务端交错顺序，复用普通书架固定 grid/card/夜间
  几何，只呈现固定上游字段和 cover/read/edit/add 四种独立动作。
- 新增共享原始 JSON 编辑器及操作 composable。非法 JSON、缺失 name/url/source、取消确认、
  保存失败、账号切换和成功单写均有合同；保存前仍执行上游“先加入书架”的确认。
- Explore chooser 恢复 desktop 600px/top 0/无内部 close、mini 100vw 顶部内容高度/无 backdrop/
  有 close；分组采用首次出现顺序并固定追加未分组，collapse 可多开，关闭重开保留筛选、展开项
  和 scrollTop。首页与结果标题的书海触发器阻止 workspace 冒泡，避免打开后立即关闭。
- 旧兼容路由、JWT REST、稳定 cursor、`hasMore` 终态、请求 generation、多用户隔离、安全 Go
  exploreUrl 解析和本地书仓搜索继续作为已记录的技术适配；本批未改数据库、持久卷、API 字段
  或解析器语义。

验证结果：

- frontend 单元/静态合同：`701/701`；Go：`go test ./...` 全部通过；Vite production build 与
  `git diff --check` 通过。
- Index 工作台专项在 `1440×900`、`1024×1366`、`390×844`、`360×800` 通过，覆盖 exact
  搜索设置、配置重搜、交错顺序、书架/结果几何相等、编辑单写、chooser 几何/多开/状态保留。
- Remote Reader 在 `1440×900`、`390×844`、`360×800` 通过；Index session isolation 与
  workspace overlay session isolation 在桌面及两个手机视口通过。
- 真实 Go API 的 CSS/JSONPath/XPath 书源搜索、书籍信息、目录、正文和元数据工作流通过。

当前切片已达到提交与本地 Docker 候选条件；Docker 标签、OCI digest、新旧卷和备份门结果将在
本机发布完成后回填。
