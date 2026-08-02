# Index 设置与操作面第二轮固定基准契约（P1）

审查日期：2026-08-02

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**contract-extracted / implementation-pending**

本文件只提取固定上游的 Index 侧栏、书架标题操作和入口所有权，并判定当前 OpenReader
差异。按照 `readerdev-compat-inventory`，本轮审查完成并单独提交前不修改应用代码。

## 1. 范围和权威文件

| 层 | 固定上游证据 | 当前对应 |
|---|---|---|
| Index 侧栏结构 | `web/src/views/Index.vue` 模板、`init`、`setIP`、`clearCache`、`showMPCode`、`joinTGChannel` | `frontend/src/layouts/AppLayout.vue`、`useAppSidebarSearch.js`、`useAppCacheManagement.js` |
| 书架标题操作 | `Index.vue` 的 `.shelf-title` | `frontend/src/views/Home.vue` |
| 全局初始化 | `web/src/App.vue#init` | Bookshelf/Reader/Preferences Pinia、sidebar source cache、各 overlay 的打开加载 |
| RSS 入口 | `Index.vue#showRssDialog` | `Home.vue` 的 RSS 标题按钮、`OverlayRSS.vue` |
| 替换规则入口 | `ReadSettings.vue` 发出 `showReplaceRuleDialog` | `ReaderSettingsPanel.vue`、`OverlayReplaceRules.vue` |
| 项目链接 | `Index.vue` 的原仓库 GitHub、公众号与 Telegram | `AppLayout.vue` 的 OpenReader GitHub 底部图标 |

本合同不重新审查书架卡片、分组、RSS、替换规则或备份内部事务；这些继续由各自专项合同
约束。本合同只决定入口是否存在、位于哪里、显示顺序以及点击后的顶层动作。

## 2. 固定上游可见合同

### 2.1 侧栏分区与顺序

固定上游按以下顺序渲染：

1. 搜索设置；
2. 最近阅读；
3. 后端设定；
4. 书源设置；
5. 书架设置；
6. 用户空间；
7. WebDAV（按权限显示）；
8. 其它；
9. 本地缓存。

书源设置的动作顺序固定为：

`书源管理 → 探索书源 → 导入书源 → 远程书源 → 失效书源 → 调试书源`。

书架设置的动作顺序固定为：

`书籍管理 → 分组管理 → 导入书籍 → 浏览书仓（按权限显示） → 刷新缓存`。

上游没有在“书架设置”中再放一个“书架”导航按钮。Index 本身就是书架/搜索/探索工作台，
搜索和探索场景各自提供“书架”返回入口。

### 2.2 后端设定

`Index.vue#setIP` 点击连接状态后打开接口地址输入框，校验新地址能够读取书架，成功后保存
`api_prefix` 并重新初始化。它不是“刷新书架”的别名。

OpenReader 采用同源单二进制服务，不能照搬可变后端地址；允许把此动作收敛为同源后端状态
检查，但必须满足：

- 标签描述后端连接状态，不能把 WebSocket 实现细节冒充产品概念；
- 点击最多重新检查 `/api/health` 并显示结果；
- 点击不得刷新书架、跳转首页或改变阅读进度；
- 不新增可把 JWT 发往任意地址的后端 URL 输入框。

这是 **security/runtime adaptation**，不是删除状态语义的理由。

### 2.3 刷新缓存

`Index.vue#init(true)` 转交 `App.vue#init(true)`。固定上游会重新加载：

- 书架；
- 书源；
- 书籍分组；
- RSS 源；
- 替换规则；
- 书签。

因此侧栏动作名称和心智模型是“刷新缓存/重取工作台状态”，而不是只刷新书架。它不等同于
“清空章节内容缓存”，后者仍属于下方本地缓存分组。

OpenReader 没有把 RSS、规则和书签全部常驻在一个全局数组中，允许用等价事务实现：强制重取
当前账号的书架、分类、书籍分组、书源和持久化偏好；使已打开的 RSS、规则、书签宿主重新取数，
未打开宿主必须在下次打开时走最新网络状态。动作必须捕获当前 JWT/账号作用域，迟到结果不得写入
另一个账号。

### 2.4 用户空间与 WebDAV

固定上游的 manager mode 允许选择 namespace、加载用户空间、管理用户空间和退出管理模式。
OpenReader 使用 JWT 登录身份和管理员 API，禁止在普通工作台中切换 namespace/冒充另一个用户。

判定：

- 当前账号标题、注销、备份用户配置、同步用户配置：`technical-stack-equivalent`；
- 管理员只打开受鉴权的用户管理：`security adaptation`；
- 不恢复 namespace 下拉框、加载用户空间或退出管理模式；
- WebDAV 的文件管理与保存备份保持上游顺序；“保存完整可移植备份”是已有数据兼容增强，放在
  两个上游动作之后。

### 2.5 “其它”、RSS 与替换规则

固定上游“其它”只有原作者公众号二维码和 Telegram 频道，`MPCode.vue` 也只渲染该公众号图片。
这些内容不属于阅读功能，也不属于 OpenReader 项目所有者：

- 不复制公众号二维码、Telegram 地址或原作者宣传文案；
- 不用空壳“其它”分区假装对齐；
- 不用无关外链替代；
- OpenReader GitHub 链接保留在固定底部图标，并指向
  `https://github.com/changshengyu/openreader`。

RSS 的固定上游入口位于书架标题栏；当前 `Home.vue` 已存在该入口。替换规则的固定上游管理入口
位于阅读设置；当前 Reader 也已存在该入口。两者再次出现在 Index 侧栏“其它”中属于重复业务
入口，会制造错误的信息架构，应删除。

### 2.6 本地缓存

固定上游在侧栏末尾显示总量和四个动作：

`清空书源缓存 → 清空 RSS 源缓存 → 清空章节列表缓存 → 清空章节内容缓存`。

当前 owner-scoped 四分组已由
[`index-local-cache-scope-p1-contract.md`](index-local-cache-scope-p1-contract.md) 关闭；当前账号的
服务器章节缓存动作是允许增强。实施不得改变缓存 key、删除范围或账号隔离，只需保证本分区位于
最后一个功能分区，且不被重复“其它”入口打乱。

### 2.7 书架标题操作

普通书架场景的固定上游可见顺序为：

`编辑/取消 → 刷新/刷新中 → RSS → 书海`。

搜索/探索场景另有“书架”和“加载更多”，由既有工作台合同约束。当前普通书架顺序为
`书海 → RSS → 刷新 → 编辑`，与固定上游相反，判定为 `must-fix`。

当前桌面“网格/列表”开关不是固定上游标题操作，也没有用户明确授权为差异。本切片先把它从
上游标题操作序列中移出；是否保留其持久化值及书架渲染能力由后续书架布局专项复审决定，不能
用本合同反向宣称卡片布局已经对齐。

## 3. 当前差异矩阵

| 关注点 | 当前行为 | 判定 | 实施要求 |
|---|---|---|---|
| 后端状态 | `同步在线/同步未连接`，点击调用 `refreshShelfData`。 | `must-fix` | 改为同源后端状态检查；禁止书架副作用。 |
| 书源动作 | 六项标签和顺序与上游一致。 | `aligned` | 保持顺序和 overlay 所有权。 |
| 书架重复入口 | 首项额外显示“书架”。 | `must-fix` | 删除；品牌、搜索/探索返回入口继续负责回书架。 |
| 侧栏刷新 | “刷新书架”，只重取 books/categories/groups。 | `must-fix` | 恢复“刷新缓存”及当前账号工作台重取事务。 |
| 用户管理 | JWT 管理员弹层取代 namespace 切换。 | `security adaptation` | 保留，不恢复冒充式 namespace 切换。 |
| 可移植备份 | WebDAV 多一个完整可移植备份动作。 | `acceptable-change` | 位于上游两动作之后；继续走当前账号确认与事务。 |
| 原作者宣传 | 当前未复制公众号/Telegram。 | `intentional omission` | 明确记录，不恢复。 |
| RSS 侧栏入口 | 标题栏和“其它”各有一个。 | `must-fix` | 保留标题栏入口，删除侧栏重复入口。 |
| 替换规则侧栏入口 | Reader 设置和“其它”各有一个。 | `must-fix` | 保留 Reader 设置入口，删除侧栏重复入口。 |
| 本地缓存 | 四个浏览器分组和一个服务器扩展。 | `aligned + acceptable-change` | 不改变 owner scope；保持末尾分区。 |
| 书架标题顺序 | `书海 → RSS → 刷新 → 编辑`。 | `must-fix` | 恢复 `编辑 → 刷新 → RSS → 书海`。 |
| 桌面视图切换 | 标题栏额外“网格/列表”。 | `unapproved difference` | 从本轮上游操作序列移出；渲染策略另行审查。 |

## 4. 先测后改闸门

### 4.1 单元/静态合同

- 侧栏六个书源动作和五个书架动作的标签、顺序、权限条件固定。
- 书架设置中不存在重复“书架”。
- 不存在 `MPCode`、公众号、`t.me/facker_channel` 或其它旧宣传入口/资源。
- RSS 不在侧栏重复出现，替换规则不在 Index 侧栏出现。
- 后端状态点击调用 health 检查，不引用 `refreshShelfData`。
- “刷新缓存”调用独立 workspace refresh，不复用只面向书架标题按钮的动作。
- 本地缓存四分组仍全部可见，服务器缓存仍只作用当前账号。
- 普通书架标题四个上游动作顺序固定；视图开关不插入该序列。

### 4.2 状态与请求合同

- 点击后端状态只请求 health；不请求 books/categories/groups，不跳路由。
- 点击“刷新缓存”强制请求当前账号的 books、categories、book-groups、sources，并重取当前账号
  持久化设置；可见 overlay 的刷新不得跨账号提交。
- 刷新期间切换 token/账号时，旧成功、旧失败、toast 和 loading 都不得写到新账号。
- 书源或其它局部刷新失败时保留已成功的书架快照并给出局部错误，不能把所有区域清空。
- 书架标题“刷新”继续遵守阅读进度 pending/CAS 合同，不因侧栏全局刷新而回退。

### 4.3 真实浏览器

在 `1440×900`、`390×844`、`360×800` 验证：

- 侧栏分区、标签和自然换行顺序；移动端拖动和固定底部图标不回退；
- 后端状态点击无书架闪动和路由变化；
- “刷新缓存”有明确 loading/完成反馈，刷新后书源和书架均为最新状态；
- Home 标题操作顺序与固定上游一致，窄屏可换行但 DOM/键盘顺序不变；
- RSS 只由书架标题打开，替换规则只由 Reader 设置打开；
- 页面中不存在原作者公众号/Telegram 宣传内容。

## 5. 实施顺序

1. 单独提交并推送本合同及总矩阵摘要。
2. 使用 `frontend-ux-compat` 与 `openreader-frontend` 先增加失败测试。
3. 收敛侧栏 section 构造与 workspace refresh 状态事务，再调整 Home 标题操作。
4. 跑 frontend 全量、production build、backend 全量和三视口真实浏览器。
5. 若动作面形成可独立人工验收切片，则本地构建并发布 Docker；发布报告必须列明本合同未覆盖的
   书架卡片布局与各 overlay 内部逻辑。
