# BookGroup / Category 写请求边界第二轮固定基准合同（P2）

状态：**implementation-complete / regression-validated / Docker-published**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

合同阶段只提取固定上游行为和当前反例；红灯测试与应用实现随后分别独立提交。范围严格限定为同一
BookGroup 状态机的六个现有 JSON 写入口：

- `POST /api/categories`
- `PUT /api/categories/:id`
- `PUT /api/categories/reorder`
- `PUT /api/book-groups/:key`
- `PUT /api/book-groups/reorder`
- `PUT /api/books/:id/category`

BookGroup 已签收的单表 UI、精确文案、混合投影、隐藏状态、many-to-many、备份恢复和 WebSocket
收敛不在本轮重建。`DELETE /api/categories/:id` 没有 JSON body，也不属于本合同。

## 1. 权威文件与当前映射

固定上游：

- `web/src/components/BookGroup.vue` 的 add/edit/show/order/set 动作；
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#getBookGroups`、
  `saveBookGroup`、`saveBookGroupOrder`、`saveBookGroupId`；
- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的对应 `/reader3/*` 路由；
- `src/main/java/io/legado/app/data/entities/BookGroup.kt`；
- `src/main/java/com/htmake/reader/verticle/RestVerticle.kt` 的共享 `BodyHandler`。

OpenReader：

- `backend/api/server.go`、`categories.go`、`book_groups.go`、
  `books.go#updateBookCategory`、`request_body.go`；
- `backend/services/bookgroups/service.go`、`backend/models/models.go#Category/BookGroupPreference/BookCategory`；
- `frontend/src/api/categories.js`、`bookGroups.js`、`books.js`；
- `frontend/src/composables/useOverlayBookGroups.js`、`stores/bookshelf.js`；
- `book-group-p2-contract.md` 与 `book-group-fixed-baseline-second-audit-p2-contract.md`。

固定上游在认证后先把请求反序列化。`saveBookGroup` 在读取用户 `bookGroup` 存储前拒绝空
`groupName`；`saveBookGroupOrder` 在读取存储前要求 `order` 数组；`saveBookGroupId` 先解析
`bookUrl/groupId`，再读取当前 namespace 的书架。上游没有可直接复制的显式 body、字段或数组上限。

OpenReader 保留 JWT REST 路径、稳定 ID/key、SQLite 事务与用户隔离。它把上游位掩码分组映射为
`Category + BookCategory` many-to-many，并用 `BookGroupPreference + Category` 组成统一投影；这些是已
签收的技术栈适配。本轮只关闭未来 HTTP 写入的资源边界，不把数据改回 JSON 文件或位掩码。

## 2. 当前反例与差异矩阵

2026-08-12 在隔离的当前生产形态服务、临时 SQLite 和临时 `data/cache/library` 根上得到以下反例：

- `POST /api/categories` 接受 20 KiB `name`，返回 `201` 并把完整值写入 SQLite；
- `PUT /api/categories/:id` 的两个连续 JSON 对象返回 `200`，只采用首对象 `first-json`；
- 空白分类名虽然返回既有 `400 category name is required`，但此前已经调用 `NextSortOrder`，在空用户
  数据库中创建了四行内置 `book_group_preferences`；
- 无 token 的超大请求仍由 JWT middleware 优先返回既有 `401 missing bearer token`；未知内置 key
  最终返回既有 `400 invalid built-in book group`；
- `PUT /api/books/:id/category` 已用 `json.Unmarshal` 拒绝尾随数据，但先 `io.ReadAll`，仍可无界分配。
- 另一个隔离双用户反例确认：A 用户提交 B 用户的 `categoryId`，同时显式提供空
  `categoryIds:[]`，当前代码只验证空数组，却在后续 fallback 中采用未验证的 `categoryId`；请求返回
  `200`，A 的 `books.category_id` 和 `book_categories` 都持久引用 B 的 Category ID。

| 合同点 | 审计时 OpenReader | 裁决 |
|---|---|---|
| wire body | 六个入口均无 actual-read 上限；五个使用 `ShouldBindJSON`，书籍分组入口无界 `io.ReadAll`。 | **must-fix security adaptation**：统一 16 KiB wire 上限。 |
| 单 JSON | 五个 Gin bind 入口忽略首值后的第二 JSON/垃圾；书籍分组入口碰巧严格。 | **must-fix**：六个入口统一只接受一个 JSON 值，只允许尾部 whitespace。 |
| 创建校验顺序 | 空白自定义组在返回 400 前惰性创建四个内置偏好。 | **must-fix**：decode、trim 和字段校验必须先于 `NextSortOrder`/SQLite。 |
| 路径/owner 优先级 | 分类更新和书籍分组更新先解析并查 owned row；内置 key 在 decode 后才由 service 校验。 | 前两项 **aligned security adaptation**；内置 key **must-fix** 为 body 前 allowlist。 |
| 字段长度 | `gorm:"size:80/24"` 在 SQLite 不执行长度约束；旧专项把 80-byte 名称上限误记成已存在。 | **must-fix data validation**：未来提交的名称/颜色显式有界，历史行不迁移。 |
| 排序/归属 | 混合排序要求当前完整投影；兼容 category-only 排序和书籍分组赋值均按当前用户验证。 | **aligned**：保持事务、旧兼容入口和 owner 语义。 |
| 书籍分类双字段 | 显式 `categoryIds` 只被用于选择校验分支；空/零数组后又回退到未校验 `categoryId`。 | **must-fix multi-user isolation**：先确定最终有效 ID，再对该集合完整 owner 校验。 |
| 成功事件 | durable commit 后广播既有 category/book-group/bookshelf 事件。 | **aligned**：只阻止拒绝请求产生写入或广播。 |

## 3. 共同 wire、认证与错误合同

- 六个路径和 method 不变，继续要求当前有效 JWT。认证 middleware 必须先于 body；缺失/无效 token
  保持既有平面 `401`，不得为检查大小先读取或记录 body。
- 完整 wire body 最多 `16 << 10` bytes，包含 JSON 标点、转义、未知字段和尾部空白。
  `Content-Length` 与 unknown-length/chunked 必须使用相同 actual-read 上限。
- 精确 16 KiB 进入原业务状态机；16 KiB + 1 统一返回
  `413 {"error":"request body too large"}`。
- 只接受一个 JSON object。尾部空格、tab、CR/LF 可接受；第二对象/数组/scalar 或非空垃圾映射为各
  端点现有 malformed `400`。未知 object 字段继续忽略。
- 错误、日志和事件不得包含 JWT、请求 body、分组名/颜色、SQLite 文本或 host path。

OpenReader 的 path/owner 优先级保持如下：

1. `PUT /api/categories/:id` 与 `PUT /api/books/:id/category` 先解析正整数 ID 并确认 caller-owned
   target；非法 ID 或不存在/外用户 target 保持既有 `400/404`，不读取 body。
2. `PUT /api/book-groups/:key` 在读取 body 前 trim 并校验 `all/local/audio/ungrouped`；未知 key 保持
   `400 {"error":"invalid built-in book group"}`，不惰性创建 preference。
3. 无 target path 的 create/reorder 在认证后直接执行有界单 JSON decode。

## 4. 端点合同

| Method / path | 请求与成功保持 | 拒绝与零副作用 |
|---|---|---|
| `POST /api/categories` | `{name,color?}`；trim，空 color 仍默认 `#216869`；有效请求取统一投影最大顺序后 `201 Category`，commit 后保持既有两类 group event。 | malformed/多 JSON 保持 `400 category name is required`；空白名同样 400，并且不得创建内置偏好、Category 或 event；overflow 为共同 413。 |
| `PUT /api/categories/:id` | `{name?,color?,show?}`；HTTP DTO 只接受显式已知字段，未知字段继续忽略；`200 Category` 与既有事件保持。为兼容已部署客户端，未提交任何已知字段的 object 仍沿当前 no-op 成功路径。SQL 列所有权、取消和并发删除由 [`category-patch-write-lifecycle-fixed-baseline-second-audit-p2-contract.md`](category-patch-write-lifecycle-fixed-baseline-second-audit-p2-contract.md) 第二轮关闭。 | owned target lookup 后，malformed/多 JSON 为 `400 invalid category payload`；空白显式 name 为 `400 category name is required`；拒绝请求不更新 `updated_at`、不广播。 |
| `PUT /api/categories/reorder` | `{ids:[...]}`；该 compatibility-only 路径继续只重排所列 caller-owned Category，不替代完整混合排序主链；成功返回现有 Category array 和事件。 | 缺失/空/多 JSON 保持 `400 ids is required`；外用户/不存在 ID 保持事务回滚和 `400 failed to reorder categories`；overflow 为共同 413。 |
| `PUT /api/book-groups/:key` | body 至少一个 `{name?,show?}`；有效内置 key 的 transaction、`200` projection row 和 post-commit event 保持。 | 未知 key 的 body 前 400 如上；empty/malformed/多 JSON 为 `400 invalid book group payload`；blank/overlong name 为字段错误且零 preference 写入/event。 |
| `PUT /api/book-groups/reorder` | `{keys:[...]}` 必须精确包含当前四内置加全部自定义 token 各一次；一个事务更新两表，返回完整 projection 后保持两类事件。 | 缺失/空/多 JSON 为 `400 keys is required`；缺失、重复、额外、malformed 或 foreign token 保持 `400 invalid book group order` 和全事务回滚；overflow 为共同 413。 |
| `PUT /api/books/:id/category` | `{categoryId:number|null}` 或显式 `{categoryIds:number[]}`；非空有效 `categoryIds` 优先，空/全零有效数组保持现有 fallback 到非空 `categoryId`，两者都无有效 ID 时保持直接 API 清空兼容。先确定并去重最终 ID，再完整 owner 校验；many-to-many transaction、legacy primary `categoryId`、`200 Book` 和一次 bookshelf event 保持。 | owned book lookup 后，malformed/多 JSON 为 `400 invalid category payload`；任何最终采用的 foreign Category 为 `400 category not found`，不能通过同时提供另一个空字段绕过；overflow 413；拒绝请求不改 `books/book_categories`、`updated_at` 或 event。 |

## 5. 字段与有界工作合同

- 新建 Category、显式改 Category name、显式改内置 name 时，trim 后名称必须非空且最多 80 UTF-8
  bytes。超过上限返回 `400 {"error":"category name is too long"}` 或
  `400 {"error":"book group name is too long"}`。
- 新建或显式修改 Category color 时，trim 后最多 24 UTF-8 bytes；超过上限返回
  `400 {"error":"category color is too long"}`。空值继续采用当前默认色，不新增 CSS color 格式
  allowlist，以免破坏旧客户端自定义值。
- 长度只检查本次显式提交的字段。历史 oversized row 仍可读取、备份、恢复，也可只切换 `show`；不得
  因未触碰的旧 name/color 阻止该操作。
- 16 KiB wire cap 已把排序 token/ID 数组和去重前工作限制在有限范围。本轮不另加 Category 总数或
  2,000 项产品配额，也不把上游 32-bit group mask 的约 30 个自定义组限制移植到 many-to-many 模型。
- 混合排序继续以完整当前投影自然约束 cardinality；旧 category-only 排序不接回可见 UI。

80/24 bytes 是现有模型和旧专项已经宣称、但 SQLite 未实际执行的未来写入边界。固定上游没有该限制，
因此它被明确记录为多用户服务的安全/数据适配，而不是伪称上游行为。

## 6. 数据、事务与迁移边界

- 不修改 `categories`、`book_group_preferences`、`book_categories` 或 `books` schema、索引、ID、关系、
  时间字段和现有行；不新增回填或 destructive migration。
- 不修改 `data/`、`cache/`、`library/`、backup/portable/WebDAV 格式或浏览器 persisted key。
- logical/portable/reader-dev/categories-only restore 继续按各自 archive/transaction 合同恢复历史值，
  不复用 HTTP 16 KiB 或 80/24-byte 限制；恢复不是未来直接 HTTP mutation。
- 所有 body/字段拒绝发生在业务事务前。只有两个带 owner target 的 PUT 可以在 decode 前执行当前用户
  existence 查询；它们仍不得写行或广播。
- 书籍分类赋值必须在一个地方先计算 `categoryIDsFromRequest` 的最终有效 ID 集合，再对该集合执行
  caller-owner 查询；禁止先按原始字段存在性选择验证分支、随后又从另一个字段 fallback。
- Category create 必须在 body 和字段全部通过后才调用 `NextSortOrder`，从而不再让失败请求触发
  `ensureBuiltIns`。成功路径的惰性四内置创建与最大顺序计算保持。
- 既有多表排序和书籍分类替换继续使用单 SQLite transaction；sync event 只在 durable commit 后发送。

## 7. 测试先行闸门

实现前必须先让以下合同在当前代码上失败：

1. 六个入口的 declared/chunked 16 KiB + 1 都返回精确平面 413；精确 16 KiB 与尾部 whitespace
   进入原状态机。
2. 五个旧 Gin bind 入口拒绝第二 JSON/垃圾；书籍分类入口保持相同 strict single-JSON；各自映射既有
   400，不以 413 或 500 代替。
3. 失败 Category create 不新增 Category，不惰性创建四个 `BookGroupPreference`，不发任何 group
   event；合法 create 仍在统一最大 order 后追加。
4. 未认证超大请求保持 401；invalid built-in key 超大请求保持 body 前 400；不存在/foreign
   Category/Book target 保持 body 前 404。错误不回显输入。
5. 80-byte 名称和 24-byte color 成功，+1 失败且 row/time/event 不变；多字节 fixture 按 UTF-8 bytes
   证明边界。直接预置 oversized 历史行后 list、backup/restore 和 show-only update 仍无损。
6. mixed reorder 的完整/缺失/重复/额外/foreign/rollback、category-only compatibility reorder、书籍
   单/多/empty/foreign category 与 legacy primary projection 的现有测试继续通过；新增双字段矩阵，
   证明 foreign `categoryId` + empty/zero-only `categoryIds` 不再跨用户落库，而合法 fallback 保持。
7. 两个用户拥有同名/不同 ID 分组时，所有成功与拒绝动作都只影响 caller；post-commit event recipient
   和 payload 不回退。
8. focused/full/race/vet、frontend 全量、production build 和隔离生产形态真实 HTTP 通过。该切片无
   UI 几何变化，不新增视口行为；发布前仍需 fresh/historical mounted-volume/backup 门。

## 8. 实施边界

实现应复用 `request_body.go#decodeBoundedSingleJSON`，由 BookGroup/Category handler 映射现有平面错误；
不得新增全局 body middleware，也不得顺带给 books/source/RSS/bookmark/上传等其它入口套用相同上限。
`updateBookCategory` 必须保留现有最终值选择语义，不能因替换 `io.ReadAll` 改变 non-empty array、fallback
或 clear 结果；owner 校验必须覆盖最终采用的集合。字段校验可用窄 helper/service，但不得把 DB
business transaction 搬进 decoder。

合同实现前不改前端。服务端是直接 API 的权威边界；当前 prompt 校验和 set-mode 空选择只能作为 UX，
不能代替 HTTP 防护。

## 9. 实施与验证记录

- 合同 `3873781` 先行提交；红灯合同 `dc4589d` 随后在旧实现上复现六入口超限/尾随 JSON、空白创建
  副作用、字段无界和双字段 owner 绕过。
- 实现 `6f54be3` 在 BookGroup/Category handler 内复用共享 bounded single-JSON decoder；没有新增全局
  middleware，也没有改变其它 JSON、上传或恢复入口。未知内置 key 在 body 前 allowlist，Category
  create 在 `NextSortOrder` 前完成字段校验，书籍分组先计算最终有效 ID 再执行 caller-owner 校验。
- `backend/api/book_group_write_boundary_contract_test.go` 覆盖六路 declared/chunked `16 KiB + 1`、精确
  16 KiB、第二 JSON/垃圾/`null`、优先级、零 row/time/event 副作用、80/24 UTF-8-byte 边界、历史
  oversized row 的 show/backup/restore，以及双用户 fallback 矩阵；既有 BookGroup、Category、排序、
  many-to-many 与 backup 合同继续通过。
- focused/full/race Go、`go vet ./...`、frontend 740/740 和 Vite production build 通过。隔离生产形态
  `scripts/smoke/book-group-write-boundary-contract.mjs` 以临时 SQLite 和临时 `data/cache/library` 重跑
  六条真实 HTTP 路径并通过；本切片没有前端或视口行为变化，因此不新增浏览器几何门。
- 该实现随最终聚合镜像 `231aa9e` 完成 fresh volume 的 portable-v1/v2-assets、cross-user、restart，
  以及 historical TXT/EPUB/UMD/CBZ、relative-cache、owner-isolation 和 portable restore 门。
  `ghcr.io/changshengyu/openreader:231aa9e` 与 `latest` 已由本机 amd64/arm64 构建并发布；OCI index
  digest 为 `sha256:e4affbeaf133220409c82dc1316d7cc2e2e7267fe8623d817205b1fa0340a5c6`，两平台
  revision label 均为 `231aa9e0a572a1a34d64e016063860a42da9570e`。
