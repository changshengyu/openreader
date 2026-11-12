# 每用户书籍上限 P2 合同

日期：2026-11-12。分支 `claude/account-book-limit-remember-login-a2fb9c`，基于 main `f82e0f5a`。固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

来源：注册限制合同（`registration-invite-code-user-limit-fixed-baseline-second-audit-p1-contract.md`）的 O1 项把书籍上限列为另起。
“记住登录信息”是另一个切片（`remember-login-fixed-baseline-second-audit-p2-contract.md`），本合同不涉及。

## 1. 上游行为（fa22f271）

- 配置：
  - `config/AppConfig.kt` 中 `var userBookLimit = 200    // 用户书籍上限`；
  - `application.yml` 与 `application-prod.yml` 中 `userBookLimit: 200`；
  - `docker-compose.yml` 中 `READER_APP_USERBOOKLIMIT=200 #用户书籍上限,默认200`。
  - `doc.md` 写的是 `defaultUserBookLimit: 200 # 新用户默认书籍上限`，但固定提交的代码里没有这个字段：`entity/User.kt` 没有每用户上限，用户管理 `UserManage.vue` 也没有这一列。`UPDATELOG.md` v3.0.5 的“去掉书籍上限启动参数”去掉的是启动参数，配置项和检查都还在。
- 检查只有一处，在 `BookController.kt#saveBook`（L1189）：
  1. 先鉴权，再要求 `origin`、`bookUrl` 非空；
  2. 按书名加作者在书架上查找（L1207–1215）；
  3. 不在书架上，并且 `bookshelf.size() >= appConfig.userBookLimit` 时，返回 `超过用户书籍数上限`（L1216–1221）；
  4. 已在书架上的书照常更新，并保留阅读进度（L1310–1318）。
  - 检查在移动或解压本地文件、远程 `getBookInfo`（L1305）之前，所以达到上限时既不写文件，也不发网络请求。
- 所有加书入口都调用 `saveBook`：
  - 搜索、发现和书籍详情的“加入书架”（`BookInfo.vue#saveBook`、`Index.vue#addBookToShelf`）；
  - 本地上传导入（`/assets/` 临时文件）、本地书仓（`storage/localStore`）和 WebDAV 书仓（`webdav/`）导入；
  - 批量导入在 `Index.vue` L2329 逐本调用 `saveBook`，单本失败用 `.catch(() => {})` 忽略，继续下一本。
- 检查不区分管理员，也不区分是否开启 `secure`；所有命名空间用同一个全局值。
- 恢复不受上限约束：`WebdavController.kt#restoreFromWebdav`（L726）调用 `BookController.kt#syncFromWebdav`（L2099），直接用备份里的 `bookshelf.json` 覆盖用户书架（L2117–2123），不检查数量。
- 前端：失败时 `plugins/axios.js` 对 `isSuccess=false` 用 `Message.error(errorMsg)` 显示原文 2 秒（L141）。`saveBook` 的成功分支不执行，导入对话框保持打开。

## 2. 当前 OpenReader（f82e0f5a）

- 数据：
  - `models.User.BookLimit`（`gorm:"default:0"`），`GET /api/admin/users` 列出；
  - `POST /api/admin/users` 与 `PUT /api/admin/users/:id` 可以设置，负数返回 400，管理员账号受保护、不能修改；
  - 没有任何代码读取这个字段。用户管理界面没有这一列，与上游一致。
- 配置：没有实例级的书籍上限。
- 会新增书架行的路径，都不检查数量：

| 编号 | 路径 | 前端入口 | 已在书架上的判断 |
| --- | --- | --- | --- |
| P1 | `POST /api/books`（`books.go#createBook`） | 前端未使用（`bookshelf.addBook` 没有调用方），保留给 API 客户端 | 无：总是新建一行 |
| P2 | `POST /api/books/remote`（`books.go#createRemoteBook`） | 搜索、发现、书籍详情“加入书架”，结果编辑后保存（`useRemoteBookAddToShelf`、`useRemoteBookResultEditor`） | 同一用户同一 `url` 已存在时返回 200，只更新分组；否则远程抓取详情和目录，建行返回 201 |
| P3 | `POST /api/imports/books` 与别名 `/api/imports/txt`（`imports.go#importTXT`） | “导入书籍”直接上传（`OverlayStorageImport`，含 `importToken`） | 经 `localbook.persistParsedBook`：唯一一个同名同作者、没有文件和章节的恢复占位行会被重新挂接，算更新；否则新建 |
| P4 | `POST /api/local-store/import`（`localstore.go#importFromLocalStore`） | 本地书仓“加入书架/批量加入书架”（`OverlayStorageImport`），搜索页本地结果导入（`Search.vue#importLocalPaths`） | 同 P3；逐项返回 `{path, book}` 或 `{path, error}`，整体 200 |
| P5 | `POST /api/webdav/import`（`webdav.go#importFromWebDAV`） | WebDAV“加入书架/批量加入书架”（`OverlayStorageImport`） | 同 P4 |
| R1 | `POST /api/backup/restore-legado`、`/api/backup/restore-webdav`，便携备份恢复 | 备份恢复 | `restoreBookshelfFromDataWithGroupMap` 按 `url`（没有时按书名）合并：已有则更新，否则新建；便携恢复再用 `RestoreExisting` 给本地书挂文件 |

  其它写书的路径（`updateBook`、`batchBooks`、`changeBookSource`、`refreshBook`、`refreshLocalBook`、`updateBookCategory`、分组）只改已有行。
- 前端：失败时显示后端英文。书籍详情的 `readError` 取 `response.data.error`；导入流程的 `row.lastError` 与 `ElMessage.error` 取后端字符串。

## 3. 差异分类

| 编号 | 差异 | 分类 | 目标 |
| --- | --- | --- | --- |
| B1 | P1–P5 新增书籍没有数量上限 | `must-fix` | 第 4 节 |
| B2 | 没有实例级上限配置 | `must-fix` | `OPENREADER_USER_BOOK_LIMIT`，默认 200（第 5 节） |
| B3 | 达到上限时前端没有上游文案 | `must-fix` | 显示“超过用户书籍数上限”（第 6 节） |
| A1 | 每用户上限：`BookLimit > 0` 覆盖实例值，`0` 跟随实例值 | `acceptable-change`（OpenReader 已有字段，只能由管理员 API 设置；相当于上游 `doc.md` 的“新用户默认书籍上限”） | — |
| A2 | 用 HTTP `403 {"error":"book limit reached"}`，而不是 200 加 `isSuccess=false` | `acceptable-change`（REST 适配，与 `user limit reached` 一致） | — |
| A3 | “已在书架上”沿用 OpenReader 的去重规则（远程按 `url`，本地按恢复占位行），不是上游的书名加作者 | `acceptable-change`（去重规则不在本切片重开） | OpenReader 会新建一行的地方都受上限约束 |
| A4 | `OPENREADER_USER_BOOK_LIMIT` 未设置、不是整数或 ≤ 0 时按 200；上游不校验，0 在上游表示什么都不能加 | `acceptable-change` | — |
| A5 | 批量导入逐项报告上限错误，前端在该项显示上游文案 | `acceptable-change`（现有逐项结果界面） | — |
| C1 | 本地导入的预览与解析不检查，只在确认导入时检查 | 一致（上游上传和预览也不检查） | 不变 |
| C2 | 恢复不受上限约束 | 一致 | 不变 |

## 4. 目标行为（B1）

- 有效上限：
  - 用户 `BookLimit > 0` 时就是该值；
  - 否则是实例值 `OPENREADER_USER_BOOK_LIMIT`；
  - 管理员同样适用。管理员行受保护、改不了 `BookLimit`，所以管理员按实例值。
- 计数：该用户在 `books` 表中的行数，包括恢复出来、还没挂文件的本地占位行，与上游 `bookshelf.size()` 一致。
- 只有“将要新建一行”时才检查：计数 ≥ 上限时拒绝。拒绝时：
  - 不建行，不写章节、分组和候选源；
  - 不写书库文件，不广播；
  - P3 的 `importToken` 和暂存内容保留，腾出位置后可以用同一个 token 重试。
- 已在书架上的书照常更新，即使已经超过上限：P2 的同一 `url` 返回 200，P3–P5 的占位行挂接。
- 检查时机（计数与插入在同一个 `BEGIN IMMEDIATE` 事务里，并发新增不会超过上限）：
  - P1：在建书事务内检查。
  - P2：确认不是已有 `url` 之后、远程抓取之前先查一次，达到上限时不发网络请求；建书事务内再查一次。
  - P3–P5：在 `persistParsedBook` 确定是新建（不是占位挂接）之后、写书库归档之前先查一次，达到上限时不写任何文件；建行前在事务内再查一次。事务失败时，已写的归档由现有的 `OwnedTree` 清理。
- 响应：
  - P1–P3：`403 {"error":"book limit reached"}`；
  - P4–P5：该项为 `{"path": "...", "error": "book limit reached"}`，整体仍是 200，其余各项照常处理。与上游批量导入逐本调用、失败后继续一致。
  - 其它错误的状态码和响应体不变。
- R1 恢复：不检查，备份中的书全部恢复（上游语义）；恢复后超过上限的书架只是不能再新增。
- 不改数据库结构，不改、不删任何已有行或文件。

## 5. 配置与默认值（B2、A4）

- `OPENREADER_USER_BOOK_LIMIT`，对应上游 `READER_APP_USERBOOKLIMIT`：
  - 正整数时取该值；
  - 未设置、为空、不是整数或 ≤ 0 时取默认 200。
  - 没有上限值；要实际取消上限，设一个很大的数。
- `config.Config.UserBookLimit` 为 0（不经 `Load` 构造的配置）时也按 200，与 `EffectiveUserLimit` 一样。
- 默认取 200 的理由，以及它对现有部署的影响：
  - 上游行为是产品合同，用户数上限已按同样理由取了上游默认 50；
  - OpenReader 以前没有上限。升级后，书架已有 200 本或以上的用户保留全部书籍，仍可阅读和更新，但不能再新增；
  - 运营者可以调高 `OPENREADER_USER_BOOK_LIMIT`，管理员也可以用 `PUT /api/admin/users/:id` 的 `bookLimit` 给个别用户放宽；
  - 不新增“0 表示不限”。上游没有这个值；用户层面的 0 已经表示“跟随实例值”，再给 0 一个含义容易混淆。
  - 如果用户要求升级后默认不限，在实施前修改本节。
- `README.md` 的环境变量表和 `docker-compose.yml` 写明变量、默认值、与上游的对应关系，以及管理员按用户设置的方式。compose 中默认为空，即 200。

## 6. 前端文案（B3、A5）

- `frontend/src/utils/bookLimit.js` 是唯一的转换点：`403` 加 `book limit reached`，或者批量结果里的 `book limit reached`，显示为“超过用户书籍数上限”。不改 `utils/authErrors.js`。
- 加入书架（`useRemoteBookAddToShelf`）和结果编辑后保存（`useRemoteBookResultEditor`）：错误先转换再交给调用方，书籍详情、搜索、发现三处的 `readError` 都显示上游原文。
- 导入（`useStorageImportWorkflow`）：直接上传抛出的 403，以及书仓和 WebDAV 的逐项错误，都显示上游原文，包括 `row.lastError` 和 `onError` 弹出的提示。
- 搜索页本地结果导入（`Search.vue#importLocalPaths`）：若有项因上限失败，另外 `ElMessage.error('超过用户书籍数上限')`。

## 7. 测试计划（先红后绿）

后端（`backend/api`，配置经真实的 `config.Load`）：
- L1 P1：上限 2 时前两本 201，第三本 403 `book limit reached`，表中仍是 2 行。
- L2 P2：达到上限时，新 `url` 返回 403，远程请求数为 0；已有 `url` 返回 200 并更新分组（对照）。
- L3 P3：达到上限时导入返回 403，不建行，书库目录下没有新的归档；删掉一本后，用同一个 `importToken` 重试得到 201。
- L4 P3 占位：达到上限时，同名同作者的恢复占位行可以挂接文件，行数不变（对照）。
- L5 P4/P5：上限 1、书仓里有两个文件时，第一项成功，第二项错误为 `book limit reached`，整体 200；WebDAV 同样。
- L6 每用户上限：实例值 1、用户 `BookLimit` 为 3 时可以加到 3 本；`BookLimit` 为 0 时按实例值。
- L7 并发：上限 3 时并发 8 个 `POST /api/books`，最终正好 3 行，其余 403。
- L8 恢复：上限 2 的用户恢复含 3 本书的 Legado 备份，3 本都恢复；之后新增返回 403（恢复部分为对照）。
- L9 配置：默认 200；`abc`、`0`、`-5` 按 200；`5` 为 5；零值 `Config` 为 200。
- L10 已超过上限的书架：5 本、上限 2 时，`PUT /api/books/:id` 与已有 `url` 的远程加入都成功，新增返回 403，没有删除任何书。

前端（`node --test`）：
- F1 `utils/bookLimit.js`：403 加 `book limit reached` 转为上游文案；其它错误原样保留；逐项错误字符串同样转换。
- F2 `useRemoteBookAddToShelf`、`useRemoteBookResultEditor`：交给 `onError` 的错误按 `readError` 的取法得到上游文案。
- F3 `useStorageImportWorkflow`：直接上传抛出的 403 和逐项错误，都使 `row.lastError` 与 `onError` 为上游文案。
- F4 `Search.vue` 本地结果导入在有上限错误时提示上游文案。

浏览器：真实 Go 服务，全新数据目录，`OPENREADER_USER_BOOK_LIMIT=2`，在 1440×900、390×844、360×800 下：
- 注册后用 API 放入两本书，再在界面“导入书籍”一个 TXT；
- 提示“超过用户书籍数上限”，书架仍是两本；
- 视口内没有横向溢出。

另外，`registration-limits-contract.mjs` 与 `reader-reauthentication-contract.mjs` 仍须通过。

## 8. 明确不重开

- 远程按 `url`、本地按占位行的去重规则；恢复的合并语义；导入的 multipart 与 `importToken` 合同。
- 用户数上限、邀请码、管理员接口的字段与校验、用户管理界面。
- “记住登录信息”（另一切片）。

## 9. 状态

**contract-only**。
