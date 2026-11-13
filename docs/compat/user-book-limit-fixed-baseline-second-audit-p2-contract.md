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
| B2 | 没有实例级上限配置 | `must-fix` | `OPENREADER_USER_BOOK_LIMIT`；未设置时不限（第 5 节，2026-11-13 用户决定，原为默认 200） |
| B3 | 达到上限时前端没有上游文案 | `must-fix` | 显示“超过用户书籍数上限”（第 6 节） |
| A1 | 每用户上限：`BookLimit > 0` 覆盖实例值（实例不限时同样生效），`0` 跟随实例值 | `acceptable-change`（OpenReader 已有字段，只能由管理员 API 设置；相当于上游 `doc.md` 的“新用户默认书籍上限”） | — |
| A2 | 用 HTTP `403 {"error":"book limit reached"}`，而不是 200 加 `isSuccess=false` | `acceptable-change`（REST 适配，与 `user limit reached` 一致） | — |
| A3 | “已在书架上”沿用 OpenReader 的去重规则（远程按 `url`，本地按恢复占位行），不是上游的书名加作者 | `acceptable-change`（去重规则不在本切片重开） | OpenReader 会新建一行的地方都受上限约束 |
| A4 | `OPENREADER_USER_BOOK_LIMIT` 未设置、为空、不是整数或 ≤ 0 时不限制；上游默认 200，也不校验（0 在上游表示什么都不能加） | `acceptable-change`（用户要求的差异，2026-11-13 决定；第 5 节、第 12 节） | — |
| A5 | 批量导入逐项报告上限错误，前端在该项显示上游文案 | `acceptable-change`（现有逐项结果界面） | — |
| C1 | 本地导入的预览与解析不检查，只在确认导入时检查 | 一致（上游上传和预览也不检查） | 不变 |
| C2 | 恢复不受上限约束 | 一致 | 不变 |

## 4. 目标行为（B1）

- 有效上限：
  - 用户 `BookLimit > 0` 时就是该值，实例不限时同样生效；
  - 否则是实例值 `OPENREADER_USER_BOOK_LIMIT`；实例值未设置时不限（2026-11-13 起，第 5 节）；
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

2026-11-13 用户决定改为默认不限，取代 2026-11-12 的默认 200；变更记录见第 12 节。

- `OPENREADER_USER_BOOK_LIMIT`，与上游 `READER_APP_USERBOOKLIMIT` 同义：
  - 正整数时作为实例上限；
  - 未设置、为空、不是整数或 ≤ 0 时不限制。
  - 上游默认 200。这里不取上游默认值，属于用户要求的差异（2026-11-13）。
- 用户 `BookLimit > 0` 仍然覆盖实例值，实例不限时同样生效；`BookLimit` 为 0 时跟随实例值，实例不限即不限。
- 不经 `config.Load` 构造、`UserBookLimit` 为 0 的配置也是不限。
- 理由与对现有部署的影响：
  - 用户要求升级不能挡住已有用户：升级后，书架已有 200 本或以上的用户可以照常新增；
  - 2026-11-12 版本（镜像 `f8786ab`）按默认 200 发布过。升级到本版本后，未设置变量的部署恢复为不限；
  - 需要限制的运营者设置正整数，管理员也可以用 `PUT /api/admin/users/:id` 的 `bookLimit` 给个别用户设上限；
  - 实例值的 0 表示“没有设置”，即不限；用户 `bookLimit` 的 0 表示“没有自己的值”，即跟随实例值。两者都是“没有设置”的意思，不再有“0 表示什么都不能加”。
- `README.md`、`README_CN.md` 的环境变量说明和 `docker-compose.yml` 的注释写明：默认不限、与上游变量同义但上游默认 200，以及管理员按用户设置的方式。compose 中默认为空，即不限。

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
- L9 配置：`5` 为 5，` 7 ` 为 7（2026-11-13 起：未设置、`abc`、`0`、`-5` 与零值 `Config` 都不限，见第 12 节）。
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

**contract-only**（由第 10 节记录取代；默认值的变更见第 12 节）。

## 10. 记录（2026-11-12）

提交：
- 合同 `523f104`；
- 红测 `e34ad5b`；
- 实现：后端 `1e18d6a`（config、`services/booklimit`、五条加书路径），前端 `dac67c3`（`utils/bookLimit.js`、两个加入书架组合函数、导入流程、搜索页本地导入），
  文档 `87e239a`（README、`docker-compose.yml`、`api-contract.md`）。

红测在 `523f104` 上的结果：
- 后端 9 个用例都在断言处失败：应为 403 的地方返回 201，并发 8 个全部建成，配置解析 7 种情况都不限；
  L4（占位行挂接）以及 L2、L8、L10 中的对照部分通过。
- 前端 7 个用例失败（`bookLimit.js` 不存在，各处显示原文 `book limit reached`，搜索页没有接线），`authErrors.js` 未改动的检查通过。
- 浏览器合同停在 desktop：第三本导入返回 201，没有任何错误提示。

实现后，同一组断言全部转绿。

验证：
- 后端：`go test ./...` 31 个包通过，`go vet ./...` 通过；`go test -race -timeout 40m ./api ./config ./services/booklimit ./services/localbook` 通过（api 1102.0 秒，config 1.3 秒，localbook 8.7 秒；`booklimit` 没有单独的测试文件，由 api 合同覆盖），没有数据竞争。
- 前端：890/890（原 882 加新增 8 个），build 通过。
- 浏览器：真实 Go 服务，全新数据目录，Chromium：
  - `OPENREADER_USER_BOOK_LIMIT=2` 时，`user-book-limit-contract.mjs` 在三个视口都通过：API 403，导入对话框保持打开并显示“超过用户书籍数上限”，书架不变；
  - 同一服务上 `reader-reauthentication-contract.mjs` 四个视口、`openreader-smoke.mjs` 三个视口通过；
  - 另一全新服务（`OPENREADER_INVITE_CODE=smoke-invite`、`OPENREADER_USER_LIMIT=3`）上，`registration-limits-contract.mjs` 三个视口通过；
  - 两个服务日志都没有 5xx。
- 没有跑公告门与存储合同：本切片不改依赖、构建或发布工具，也不改存储格式。

状态 **implementation-complete / regression-validated**；分支已推送，未合入 main，镜像未发布，生产未访问。

## 11. 发布（2026-11-13）

本节内容（每用户书籍上限）随本次合并发布。合并提交 `4856215`（闲读）、`310745d`（书籍上限与记住登录信息）、`f5bfd5d`（Reader P0）、`6175cae`（桌面“左右滑动”两项），main 由 `f82e0f5` 快进到 `f8786ab`（ls-remote 核验，未强推）。

推送前在合并树上：后端 `go test ./...` 31 个包与 vet，race api/db/services（api 1088.9 秒）无数据竞争；前端 971/971 与 build；公告门自测 114/114，现场 Go 与前端扫描 0 个发现（推送前重跑）；compose 配置、存储 HTTP 合同、浏览器 smoke 接线 10/10；secdb main 与 11-10 快照一致。真实 Go 服务、全新数据目录下浏览器合同全部通过：openreader-smoke，标签页标题四个流程（壳标题 `闲读`），错误页 48 例，运行标题，桌面面板正文控件连续 3 次 120/120（`C1 flip` 36/36），晚到 flip 设置，D4 9/9，设置位置，TTS 五个视口，连续，重新认证四视口，记住登录信息，书籍上限，注册限制，移动（含 iPad），会话生命周期，首页会话隔离，文本模式，EPUB，音频，书源调试。服务日志没有 panic；唯一的 5xx 是注册限制合同关页时被取消的 `PUT /api/settings/shelf`（`settings.go` 把取消的请求记为 500，原有行为，另起切片）。合并适配：`f45f766` 让“闲读”技术名对照改查 `utils/authToken.js` 的 `AUTH_TOKEN_KEY = 'openreader_token'`，`f8786ab` 让 D4 合同从 `da438bc7` 之后的 `.rail-item[aria-label="设置"]` 打开设置，断言都没改。合入 flip 分支前，`C1 flip` 在 main `f82e0f5`（三次共 8/36 失败）、Reader 分支与合并树上都偶发超时，原因是晚到的账号设置让桌面停在 flip；合入后消失。

可信 Actions run `43400193943` 43/43 步成功（09:18–09:35 UTC）：两平台 OS 门 `pass-with-unknown-origins`（16/16 包、110/109 个 APK 文件、74 组比较无待修复项，secdb main 11-12 22:52:32，复审期限 2026-12-10），Go 公告门 go1.26.11 0 个发现（250 条公告，x/crypto pin 复审期限 2026-12-16），前端公告门 0 个发现，卷门与平台核验通过。

镜像：`ghcr.io/changshengyu/openreader:f8786ab` 与 `latest` 指向 OCI index `sha256:0066943b9fb879041a503391723bf9bc4b31ee19aee06d273073b6ac4142890f`；linux/amd64 `sha256:6944f67944131f2d90e15ccd83e56d6b808ec3cbb4a4f8474d808f15fa064d13`，linux/arm64 `sha256:2537a5de7dc67046ee4449db329c77d0028ab03d291cfe142b290ec4c0bfa729`，另有两个 attestation。两平台 revision 标签均为 `f8786abe2679ca34b76b6aaa6017558225662ecd`；两平台 `/app/openreader` 均由 go1.26.11 构建，`/app/public/index.html` 标题为 `闲读`（逐层校验 sha256 后读取）。

状态 **released / Docker-published / awaiting-device-verification**；生产未访问，实际运行版本未知。

## 12. 2026-11-13 用户决定：默认不限

用户决定把每用户书籍上限改为默认不限，取代第 5 节原先的默认 200。分支 `claude/user-book-limit-default-unlimited`，基于 main `2f19133c`（已含本切片的首个版本，以及“记住登录信息”与 PWA）。

- 改动范围：
  - 只改实例默认值：第 3 节的 B2、A1、A4，第 4 节的有效上限和第 5 节；
  - 计数时机、事务、403 响应体、批量逐项报告、前端文案“超过用户书籍数上限”、不受限的路径（已有书、占位行挂接、恢复）都不变。
- 测试（先红后绿）：
  - 新增 L11：不设变量时，已有 200 本的用户还能通过 API 新增；零值 `Config` 同样不限。
  - 新增 L12：实例不限、用户 `bookLimit` 为 2 时，第 3 本返回 403（对照）；`bookLimit` 改回 0 后跟随实例值，书架补到 200 本仍能新增。
  - 改写 L9 `TestBookLimitEnvironmentParsing`：原来断言未设置、`abc`、`0`、`-5` 与零值 `Config` 按 200 限制，与新合同冲突；改为这五种情况都能加到 201 本以上，`5` 与 ` 7 ` 仍按该值限制（对照）。
  - L1–L8、L10 都设置了正整数上限，不受影响，不改。
  - 前端测试不改：文案和接口不变。
- 浏览器：
  - `OPENREADER_USER_BOOK_LIMIT=2` 的服务上，`user-book-limit-contract.mjs` 三个视口仍应限制；
  - 不设变量的服务上，用同一脚本的 `USER_BOOK_LIMIT=unlimited` 模式：先经 API 放入 200 本，第 201 本经 API 返回 201，再在界面导入一本成功、没有“超过用户书籍数上限”，三个视口；
  - `openreader-smoke.mjs` 三个视口。
- 文档：`README.md`、`README_CN.md`、`docker-compose.yml` 注释与 `api-contract.md` 同步为默认不限。

状态 **contract-only**（由下面的记录取代）。

### 12.1 记录（2026-11-13）

提交：合同与文档 `9a83857`；红测 `ef71f3b`；实现 `1ac6c99`（`config.go` 与 `services/booklimit`，前端不改）。

红测在 `2f19133c` 上的结果：
- L9 的五个“不限”情况（零值 `Config`、未设置、`abc`、`0`、`-5`）都在第 201 本返回 403；
- L11 在已有 230 本时第一次新增就返回 403；L12 后半（`bookLimit` 改回 0，补到 200 本）返回 403；
- 对照（L9 的 `5` 与 ` 7 `、L12 前半）和 L1–L8、L10 通过；
- 浏览器 `USER_BOOK_LIMIT=unlimited` 模式停在 desktop：第 201 本返回 403。

实现后，同一组断言全部转绿。改写的既有测试只有 L9（第 12 节已注明），其余断言没有改动。

验证：
- 后端：`go test ./...` 31 个包通过，`go vet ./...` 通过；`go test -race -timeout 40m ./api ./config ./services/...` 通过（26 个包：api 1101.2 秒、config 与 services 下 24 个有测试的包；`booklimit` 没有单独的测试文件，由 api 合同覆盖），没有数据竞争。
- 前端：997/997（前端代码未改），build 通过（含 service worker 生成）。
- 浏览器：真实 Go 服务，全新数据目录，Chromium：
  - `OPENREADER_USER_BOOK_LIMIT=2` 的服务上，`user-book-limit-contract.mjs` 三个视口仍然限制：API 403，导入对话框显示“超过用户书籍数上限”，书架不变；
  - 不设变量的服务上，`USER_BOOK_LIMIT=unlimited` 模式三个视口通过：经 API 放入 200 本后第 201 本返回 201，再在界面导入成功（返回 201，书架显示 202 本），没有“超过用户书籍数上限”；
  - 同一服务上 `openreader-smoke.mjs` 三个视口通过；
  - 两个服务日志都没有 5xx。
- 没有跑公告门与存储合同：不改依赖、构建工具或存储格式。

状态 **implementation-complete / regression-validated**；分支已推送，未合入 main，镜像未发布，生产未访问。
