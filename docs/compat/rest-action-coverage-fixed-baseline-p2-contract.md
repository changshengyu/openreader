# REST 长尾动作固定基准覆盖合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`  
当前审查基线：`OpenReader@d2f7bfe`  
审查日期：2026-08-11

## 1. 目的与范围

本合同补齐 `backend/api/server.go` 中此前没有以精确路径写入专项合同的六个动作。路径字符串没有
出现在旧文档不等于功能缺失；每个动作仍须分别核对上游入口、状态转换、请求/响应、持久化和可见
调用方。结论只分为 `aligned`、`technical-stack-equivalent`、`compatibility-only`、`must-fix`。

覆盖路径：

- `POST /api/auth/login`
- `POST /api/auth/register`
- `GET /api/txt-toc-rules`
- `GET /api/books/:id/source-candidates`
- `POST /api/books/:id/bookmarks/batch-delete`
- `PUT /api/categories/reorder`

## 2. 动作矩阵

| OpenReader 动作 | reader-dev 权威动作 | 当前语义 | 裁决 |
|---|---|---|---|
| `POST /api/auth/login` | `POST /reader3/login`，`isLogin=true` | 独立登录路由，JWT `{token,user}`；未知用户和错误密码统一 401；成功后更新 `last_active_at`。 | **technical-stack-equivalent**。拆分路由、JWT、通用 401 是 Go/多用户安全适配；既有账号包括旧卷账号必须继续可登录。 |
| `POST /api/auth/register` | 同一 `/reader3/login`，`isLogin=false` | 独立注册路由；新账号用户名/密码规则与上游一致；首个账号成为管理员；重复账号 409；成功直接签发 JWT。 | **technical-stack-equivalent**。显式注册界面替代 `isLogin` 开关，不能把注册重新混回登录或破坏旧账号兼容。 |
| `GET /api/txt-toc-rules` | `GET /reader3/getTxtTocRules` | 当前仅返回 10 条 `enable=true` 规则，导入前端又过滤 `enable:false`。 | **must-fix**。必须返回固定上游全部 18 条及原始顺序，手动规则列表不得按 `enable` 过滤。 |
| `GET /api/books/:id/source-candidates` | `/reader3/getAvailableBookSource`、`searchBookSource(SSE)` | 当前打开面板就按活动书源远程搜索，并接收非精确结果；没有每书候选缓存。 | **must-fix**。见 [`reader-source-switch-fixed-baseline-second-audit-p0-contract.md`](reader-source-switch-fixed-baseline-second-audit-p0-contract.md)。 |
| `POST /api/books/:id/bookmarks/batch-delete` | `POST /reader3/deleteBookmarks` | 当前按不可变 bookmark ID、当前用户和当前书过滤；事务删除；按请求中的首次有效顺序返回 `deletedIds`；提交后只广播一次。 | **technical-stack-equivalent / aligned**。ID 身份修复上游按书名作者删除首条的歧义，必须保留。 |
| `PUT /api/categories/reorder` | `/reader3/saveBookGroupOrder` 的旧 Category-only 映射 | 当前按当前用户 category ID 更新顺序；前端 API/store 仍保留，但可见 BookGroup 已统一使用 `/api/book-groups/reorder {keys}`。 | **compatibility-only**。旧内部 API 保留以兼容历史调用，不得重新接入可见 UI；统一混合分组排序继续以 `book-groups/reorder` 为唯一主链。 |

## 3. 登录与注册合同

### reader-dev

`UserController.login` 用一个动作和 `isLogin` 区分登录/注册：空用户名或密码失败；注册时执行固定的
用户名、保留名和密码校验；登录时读取既有用户、验证密码并保存 session/最后登录时间。

### OpenReader 映射

- `AuthForm` 显式切换登录/注册，分别调用两个路由，用户可见能力等价于上游 `isLogin`。
- 新账号仍执行：trim 后用户名至少 5 位、只允许 ASCII 字母数字、大小写不敏感保留 `default`、
  密码至少 8 位。
- 上述新账号规则不得追溯拒绝旧 SQLite 用户；登录只 trim 用户名，不重新执行注册校验。
- 登录成功后 `lastActiveAt` 和兼容字段 `lastLoginAt` 必须反映本次登录；失败登录不得更新时间。
- 首个注册用户成为管理员，以及 JWT token/role/permission 投影，是 OpenReader 多用户运行所需适配。
- 不泄漏“用户名存在但密码错误”的差异；当前统一 401 文案属于允许的安全增强。

既有 `user_management_p2_contract_test.go` 已覆盖新账号约束、旧账号登录与登录时间；本轮不以路径
遗漏为理由重写已对齐逻辑。后续安全门应单独验证认证 JSON body 上限，但不得借此改变可见状态机。

## 4. TXT 目录规则合同

固定上游 `defaultData/txtTocRule.json` 是权威清单：`id=-1…-18`、`serialNumber=0…17`，其中
`-3/-4/-5/-10/-12/-15/-16/-17` 为 `enable=false`。`enable` 只控制自动探测是否尝试该规则，
不控制用户能否在导入界面手动选择。

必须满足：

1. API 直接数组必须逐项保留 `id/enable/name/rule/serialNumber`，顺序与上游 JSON 完全一致。
2. `useOverlayBookImport.loadTocRules()` 只剔除没有规则文本的损坏行，不得过滤 `enable=false`。
3. 自动检测仍只遍历 `enable=true`，并继续受 512 KiB probe、输入/解码文本/章节数预算约束。
4. 手动选择 18 条中的任一条都必须可执行；不能只把 RE2 无法编译的 Java 正则显示出来。
5. `-16` 前向双标题和 `-17` 后向双标题包含跨行 lookaround。Go 实现必须保留相邻标题语义，
   不能简单删除 lookaround 后把每一个普通章节行都当成匹配。
6. 自定义用户规则继续支持当前已实现的上游 lookbehind 前缀和负向排除兼容，超长规则与解析预算
   继续拒绝。

错误测试 `overlayBookImport.test.mjs` 中“filters enabled TXT TOC rules once”必须改写；
`TestDefaultTXTTocRulesIncludeUpstreamEnabledRules` 必须升级为精确 18 条合同，并为八条手动规则增加
至少一组成功/不应匹配 fixture，特别覆盖 `-16/-17` 的相邻行语义。

## 5. 书签批删合同

上游以书名和作者定位，每个 payload 删除找到的第一项；在同书多书签和跨用户环境中身份不稳定。
OpenReader 的 bookmark ID、`user_id + book_id + id` 过滤、去重、事务及一次 post-commit event 是明确
允许的数据安全适配。请求中的外用户、外书和不存在 ID 被忽略，响应只返回实际删除 ID；空/无正数
请求为 400。此语义由
[`bookmark-fixed-baseline-second-audit-p2-contract.md`](bookmark-fixed-baseline-second-audit-p2-contract.md)
继续约束，本轮只补精确路径覆盖。

## 6. 分类排序兼容层合同

第二轮 BookGroup 重建已确认可见排序必须同时覆盖四个内置组和自定义 Category，并以稳定字符串 key
执行完整顺序事务。因此：

- `OverlayBookGroups`、`useOverlayBookGroups` 与新代码只能调用
  `PUT /api/book-groups/reorder {keys}`。
- `PUT /api/categories/reorder {ids}` 和 `reorderCategoryIds()` 只作为旧 OpenReader API 兼容层保留；
  不得在模板、composable 或新测试中把它当成主流程。
- 兼容层仍必须只更新当前用户 ID、事务失败不留下部分顺序，并广播 Category 和统一 BookGroup 投影。
- 如果未来有版本化 API 删除计划，必须先提供迁移窗口；本轮不得直接删除，因为旧合同和旧调用方
  已明确承诺兼容。

## 7. 实施闸门

本合同完成后，应用实现顺序为：

1. TXT 精确清单与手动解析测试先失败，再修改 parser/API/导入前端。
2. 阅读器换源按专项合同先建立 API、数据和前端状态失败测试，再做加法迁移与实现。
3. 登录/注册、书签批删和分类兼容层只补覆盖或安全上限，不做无证据的功能改写。
4. 聚焦测试后必须跑 Go 全量、frontend 全量与 production build；TXT 导入和换源各做真实浏览器。
5. 涉及候选缓存表时必须通过 fresh/historical volume、用户删除/书籍删除清理和 portable backup
   不包含派生缓存的合同。

当前状态：**inventory-complete / implementation-pending**。
