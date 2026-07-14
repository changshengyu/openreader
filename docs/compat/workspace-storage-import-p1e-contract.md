# P1-E 工作台书仓、WebDAV 与本地导入兼容合同

状态：**P1-E1、P1-E2、P1-E3 已实现并完成前端/浏览器回归；P1-E4 仍未开始。**
基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
上游证据：`web/src/views/Index.vue`、`web/src/components/LocalStore.vue`、`web/src/components/WebDAV.vue`、`BookController.kt`、`LocalBook.kt`、`TextFile.kt`。  
当前映射：`OverlayLocalStore.vue`、`LocalStore.vue`、`OverlayWebDAV.vue`、`WebDAVBrowser.vue`、`OverlayStorageImport.vue`、`useStorageImportWorkflow.js`、`backend/api/localstore.go`、`backend/api/webdav.go`、`backend/api/local_import_stage.go`、`backend/services/localbook/*`。

本合同不以既有“已实现”记录或静态测试为对齐证据。先固定上游的可见操作与导入状态转换，再决定哪些 Go/多用户/安全适配能够保留。

## 1. 上游权威状态转换

| 场景 | reader-dev 行为 | 不可丢失的结果 |
|---|---|---|
| 工作台宿主 | `Index.vue` 同时挂载 `LocalStore` 与 `WebDAV`；侧栏操作只切换相应 dialog 布尔状态。 | 文件管理不是另一套产品页面；关闭后仍是原来的 Index/书架场景。 |
| 打开 LocalStore / WebDAV | 两个组件的 `show` watcher 在每次打开时调用根路径 `/` 的列表请求。移动小界面使用 fullscreen dialog。 | 重新打开不继承上一次的子目录、选择或过滤状态。 |
| 列表与选择 | 目录可进入；非目录可选；删除必须确认；上传后刷新当前路径。LocalStore 还提供关键字筛选和超过 101 项的“加载更多”。 | 管理动作的选择、确认、刷新不会隐式导入或离开工作台。 |
| 可导入项 | 上游 LocalStore 表面入口为 `.txt/.epub/.umd/.cbz`；上游 WebDAV 表面入口为 `.txt/.epub/.umd`。 | 文件类型可见性必须由固定上游组件与实际解析能力共同核验，不能凭当前后端 allow-list 直接宣布等价。 |
| 单个/批量加入书架 | LocalStore/WebDAV 只调用 `importFromLocalPathPreview`，将预览结果交回 `Index.importMultiBooks`。单本进入逐本导入 dialog；多本明确让用户选择“批量导入”或“逐一确认”，再选择统一分组或逐本确认。 | 预览永远先于持久化；用户可以取消；多本不能静默导入；所有来源最终汇入同一导入确认流程。 |
| 恢复与下载 | WebDAV 的 `.zip` 显示恢复确认，其他文件可下载；恢复成功后由 Index 刷新工作台数据。 | 恢复的覆盖确认、结果同步和错误可见性不能因 Vue/Go 重写而丢失。 |

## 2. 当前映射和审查判定

| 合同层 | 当前证据 | 判定 | 后续动作 |
|---|---|---|---|
| Index 宿主与旧链接 | `GlobalOverlayHost.vue` 在根工作台挂载两个 `el-dialog`；`/local-store` 重定向至 `/?overlay=local-store`，`/settings?panel=webdav` 转为 `/?overlay=webdav`。 | `aligned` | 真实浏览器验证：打开、关闭、旧链接清理 query 后仍在原书架。 |
| 根目录重置/移动全屏 | Overlay 使用 `destroy-on-close`，`LocalStore`/`WebDAVBrowser` 的初始路径为空，根 dialog 使用 `:fullscreen="isMobile"`。 | `technical-stack-equivalent` | 浏览器断言关闭后重新打开为根路径、无残留选择/结果，并在 390×844/360×800 无横向溢出。 |
| 安全的不可变输入 | `previewLocalStoreImport` / `previewWebDAVImport` 将每个文件复制到用户私有 `cache/import-previews/<user>/<token>`；确认导入和规则重试可只读取 token。令牌有 24 小时生命周期、大小上限和用户隔离。 | `acceptable-change` | 必须保留；这是多用户、挂载卷和源文件可变性的安全/稳定性适配，不能退回到确认时重读源路径。 |
| 规则编辑与失败重试 | `OverlayStorageImport` 的预检和单书确认均可编辑 TXT/EPUB `tocRule`，每次“重新解析/刷新目录”携带原 token 并替换当前行章节预览。 | `acceptable-change` | 这是 P1-E1 的稳定性增强；失败不重新读取 LocalStore/WebDAV 原文件，也不替代后续上游确认分支。 |
| 导入确认链路 | LocalStore/WebDAV 只创建 `{source, paths}` 请求；唯一的根工作台 controller 实现单书直达、多书方式选择、统一分组、逐本取消后继续与稳定的单项 token 写入。 | `aligned` | `OverlayStorageImport` 关闭只重置临时 UI；文件管理 dialog 仍保持打开。 |
| 当前额外书仓动作 | 当前 UI 新增递归导入当前目录、导入筛选、导入目录、新建目录、重命名和 LocalStore 下载；这些均不在固定上游 LocalStore 可见流程中。 | `must-fix` | 在没有用户明确授权的前提下，不能把额外产品流当作上游重构成果。先从主 UI 移除或放到不干扰上游操作的兼容入口；后端端点仅可因旧客户端兼容而保留。 |
| 当前格式范围 | 当前 LocalStore/WebDAV 界面将 `.text/.md/.pdf/.cbz` 等同于上游可导入书籍；与两个上游组件的入口范围不同。 | `unknown` | 对照上游对应 parser/controller 的真实格式支持及用户显式要求后再决定。CBZ 已有独立上游格式审计；其余格式不能仅凭当前解析器支持而作为对齐依据。 |
| WebDAV 备份 | 当前恢复操作有确认、统一 `applyRestoreResult`，并由后端执行带界限的 ZIP 验证。 | `technical-stack-equivalent` | P1-E2 复验恢复后书架/书源/RSS/进度同步、旧备份格式、错误不泄露路径。 |
| 数据与卷 | `data/`、`cache/`、`library/` 未迁移；暂存仅为可过期派生数据，导入成功才写私有 library archive。 | `aligned` | 用已有卷升级、暂存 token 和导入后 archive 进行 Docker/备份烟测。 |

## 3. P1-E1：先修复暂存重试桥接

这是下一批的最小实现边界，仅解决“从书仓/WebDAV进入的本地书，首次目录解析失败或规则变更后不能在同一份字节上可靠重试”。不在本批删除所有额外书仓功能，也不改 SQLite、备份格式、源文件路径或 parser 的已验证 TXT/UMD 语义。

### 必须实现的状态机

1. 用户选择书仓/WebDAV 文件或目录，调用 preview；每个结果保留 `{path, importToken, book|error}`。
2. 不论初次是否成功，TXT（以及有明确规则的 EPUB）行都可以编辑规则并点击“重新解析”。
3. 重新解析请求必须提交 `{items:[{path, importToken, title, author, tocRule}]}`；不能再次以 `paths` 触发从 mounted 源文件读取。
4. 服务端以相同 token 的暂存字节解析，成功则替换该行的章节数/目录，失败则保留 token、已填规则和可读错误，允许再次修正。
5. 确认导入只提交最终选中的同一批 token；服务端成功后删除本用户 token，失败项继续保留 token。源文件在预览后被删除、重命名或被慢速挂载更新，不得影响重试或确认。
6. 取消关闭只关闭界面；暂存文件按既有 TTL 清理。关闭不能删除 LocalStore/WebDAV 原始文件，也不能创建书架记录。

### 允许保留的差异

- Gin/JWT/SQLite 多用户隔离、受大小限制的暂存文件、令牌重试、隐私化错误和挂载卷路径校验是必要安全适配。
- 当前表格式预览可保留为更安全的批量编辑外观，但其状态转换必须先覆盖上游的预览、取消、分组确认和单本/多本分支；本 P1-E1 不把它认定为最终上游等价。

### 必须先新增的测试

| 层级 | 失败用例与断言 |
|---|---|
| 前端单元 | LocalStore/WebDAV API helper 能发送 token 化 `items` 重新预览；预览 dialog 的失败 TXT 行保留 token、规则输入和“重新解析”；成功行规则变更会替换可见章节预览而非只延后至导入。 |
| Go API | 自定义 TXT 规则首次无目录后，以同一 token 重试成功；每次重试仅使用已暂存字节；预览后删除/改名源文件仍可重试及导入；失败导入不消费 token；跨用户 token 返回无内部路径的错误。 |
| 浏览器 | 在根 Index dialog 中完成 LocalStore 和 WebDAV 各一次“失败 → 填规则 → 重新解析 → 确认导入”；检查请求先 preview、再 token preview、最后 token import，且关闭/错误不离开工作台。390×844 与 360×800 同时验证全屏、无点击穿透和无横向溢出。 |
| 发布/数据 | 全量 Go、前端测试、生产构建；已有数据卷上验证缓存书籍不变、暂存 token 正常重试、导入 archive 可读；本地 Docker volume/backup smoke。 |

### P1-E1 实现证据（2026-07-13）

- `LocalBookImportPreviewDialog.vue` 现在对 TXT/EPUB 的成功或失败预览行都保留规则输入和“重新解析”。失败行保留服务端的 `importToken`；成功重试会原位刷新章节数和目录，失败重试保留规则、错误和 token。
- LocalStore 与 WebDAV 的 preview API helper 都接受 token 化 `items`；两个根工作台 dialog 都把重试行原样回传给对应 `*-import-preview` 接口。确认导入仍只发送同一 token，不会以原路径重新读取挂载文件。
- 新增 `frontend/tests/localBookImportRetryContract.test.mjs`；前端全量 `npm test` 通过（376 个测试），生产 `npm run build` 通过。
- `backend/api/workspace_import_stage_contract_test.go` 的 LocalStore/WebDAV 回归通过：第一次预览后删除原始文件，先以错误规则失败、再以正确规则使用同一 token 成功重解析并导入。完整 `go test ./...` 通过。
- 真实浏览器 `scripts/smoke/workspace-storage-retry-contract.mjs` 和既有 `workspace-operation-contract.mjs` 均在 1440×900、390×844、360×800 的生产预览中通过；前者验证两种入口的失败→重试→确认 token 请求链，后者验证根 dialog、旧链接、重新打开根目录和移动全屏不回归。

本实现不改变 parser、SQLite、备份格式、`data/`/`cache/`/`library/` 目录或 token 生命周期；Docker 卷/备份烟测是本批发布前的剩余门禁。

## 4. P1-E2：恢复上游单本/多本导入确认状态机

状态：**已完成源码审查；尚未开始本批应用代码改动。**

### 上游状态转换（`Index.vue#importMultiBooks`）

| 前置条件 / 用户动作 | 上游状态与副作用 | OpenReader 必须达到的结果 |
|---|---|---|
| 预览为空 | 不打开导入确认。 | 不写书架、不打开一个空 dialog。 |
| 预览恰有一本可导入书 | 直接打开“导入本地书籍”确认；默认分组为空；可编辑书名、作者、分组、TXT 规则，刷新目录后才点“确定导入”。 | 不先显示批量选择；关闭即取消该书，不创建书架行。 |
| 预览有多本可导入书 | 弹出不可点遮罩/不可 Esc 关闭的方式选择：确认是“批量导入”，取消是“逐一确认导入”，关闭才是整体取消。 | “取消”不能等同于整体取消；必须可明确选择逐本模式。 |
| 多本 → 批量导入 | 再打开“统一设置分组”；确认后按原预览顺序逐本 `await saveBook`；取消分组选择则不导入任何书。 | 分组确认是写入前的单独门；不能把当前书仓筛选分组静默带入。每项必须保留自己的 token、标题、作者和规则。 |
| 多本 → 逐一确认 | 依次打开同一单书 dialog，标题标记 `（i/n）`；每本独立编辑和确认。关闭当前项会继续下一项，全部结束后清理临时状态。 | 顺序、取消当前项后继续、确认后只写当前项必须固定。 |
| 单书规则刷新 | 编辑规则后 `getChapterListByRule` 原位更新该书章节列表；确认始终使用当前规则。 | 使用 P1-E1 的同 token preview/reparse，更新当前书的章节和错误，绝不重新读取 LocalStore/WebDAV 原路径。 |
| 单书保存成功/失败 | 成功关闭当前确认，刷新书架；失败只提示失败，逐本流程仍可继续后续书。 | Go 多用户 API 与 websocket 可以不同，但每本的成功/失败必须独立、顺序可见、不会吞掉后续预览项。 |

### 当前差异与处置

| 当前实现 | 判定 | P1-E2 处置 |
|---|---|---|
| `LocalStore.vue` 与 `WebDAVBrowser.vue` 曾各自持有预览、确认和结果状态。 | `resolved` | 两者现只调用 `overlay.openStorageImport(source, paths)`；`GlobalOverlayHost` 是唯一 controller 宿主。 |
| `LocalBookImportPreviewDialog` 曾是全量表格勾选/统一分组/一次确认。 | `resolved` | 已删除；正常有效预览按单书直达、方式选择、批量分组、逐本确认四状态运行；失败/重试行只作为 P1-E1 预检安全层。 |
| `targetCategoryIds` 会由当前书仓/WebDAV UI 的筛选状态预填导入分组。 | `resolved` | controller 初始和每次方式选择均显式重置为 `[]`；只有单书或统一分组 dialog 的用户选择会写 `categoryIds`。 |
| 后端 `/local-store/import`、`/webdav/import` 已能接受多项或单项 token，按循环返回每项结果，且成功后才消费 token。 | `technical-stack-equivalent` | UI 批量和逐本模式均按上游顺序逐项调用现有 endpoint（每次只传一项）；不能为了减少请求而把上游逐本副作用变成一笔不可区分的批量写入。 |
| P1-E1 失败项可编辑规则、同 token 重试。 | `acceptable-change` | 保留为上游没有显式错误表时的稳定性/可恢复性增强；正常成功项不可因此跳过上游确认分支。 |
| 当前“导入结果”列表。 | `must-fix` | 不能替代逐项成功/失败提示或改变下一个确认的时机。可保留一个非阻塞汇总仅作所有项完成后的安全提示，前提是不增加额外确认路径。 |

### 共享控制器与数据约束

1. 新控制器必须在根工作台只有一个可见实例；LocalStore/WebDAV 只发起同一个 controller 的来源请求。打开其确认面板不得关闭对应的文件管理 dialog。
2. controller 的受控状态至少包含：`source`、不可变预览行、当前模式、当前逐本索引、显式类别选择、进行中的单项写入和可恢复失败行。关闭时必须重置所有临时状态，但不删除暂存 token 或源文件。
3. 每次 API 写入只发送当前预览行的 `path/importToken/title/author/tocRule` 与当前由 dialog 明确选中的 `categoryIds`。不得从 `targetCategoryIds`、当前路径、已删除的源文件或浏览器持久化状态补写字段。
4. 批量和逐本写入都按 preview 返回的稳定顺序调用；某项失败须记录安全错误并继续下一项。成功项 upsert 书架并由现有 websocket/本地 store 处理同步。
5. TXT/EPUB 规则刷新只更新当前行的预览 metadata；失败重试仍保留 token。任意取消不得消费 token 或创建 `Book`/`Chapter`/`BookCategory` 行。

### 实施前必须新增的测试

| 层级 | 必须覆盖的契约 |
|---|---|
| controller 单元 | 空/单项/多项分支；方式选择的 confirm/cancel/close 三态；批量分组取消零写入；逐本取消仍推进；稳定顺序与逐项 API 调用；失败不阻止后续项；关闭完全重置。 |
| 前端结构 | LocalStore/WebDAV 不再拥有独立导入状态机；共同 controller 只挂在 GlobalOverlayHost；进入导入的默认类别为空；每次 payload 保留同一行 token。 |
| Go/API | 重用并扩展现有暂存 token 测试：一项导入失败不消费它的 token、另一个项成功并消费自己的 token；类别隔离、顺序和安全错误不得泄露路径。P1-E2 不允许 schema/migration 修改。 |
| 真实浏览器 | 1440×900、390×844、360×800 分别验证 LocalStore 和 WebDAV：一项直达；两项选择批量→分组确认→按序两次 token 写入；两项选择逐一→取消第一项→确认第二项；失败规则重试后再进入同一状态机；文件管理 dialog 保持、无点击穿透/横向溢出。 |

### 允许差异

- Vue 3/Pinia 的单一 controller、Gin/JWT 多用户 token、全屏移动 dialog 与上游 Vuex/Element messagebox 的内部结构不同，但上表的可见选择、默认值、取消语义和写入顺序不可改变。
- P1-E1 的失败预检可以提供比上游更多的错误/重试信息；它只能在写入前帮助恢复，不能替代上游单本/多本选择或自动导入。

### P1-E2 实施与验证记录

- 新增 `useStorageImportWorkflow.js`，固定 `预检 → 单书 / 方式选择 → 批量分组或逐本确认` 的状态转换；批量与逐本都按预览顺序一次请求一项。
- `OverlayStorageImport.vue` 是唯一全局导入界面。LocalStore、WebDAV 不再持有第二份预览、分组或结果 dialog；关闭/取消只清理 controller 临时状态。
- `storageImportWorkflow.test.mjs` 和 `storageImportOverlayContract.test.mjs` 覆盖单书默认未分组、批量/逐本/关闭取消、逐项 token 写入、失败项留在当前确认和根宿主唯一性。
- 真实浏览器 `workspace-storage-import-state-machine.mjs` 在 1440×900、390×844、360×800 对 LocalStore 与 WebDAV 分别覆盖单本直达、批量分组、逐本跳过、关闭方式选择零写入、移动全屏和无横向溢出；`workspace-storage-retry-contract.mjs` 同时复验两来源的同 token 规则重试。

## 5. P1-E 后续顺序

1. **P1-E1**：暂存 token 的可见重试桥接（已发布）。
2. **P1-E2**：将 LocalStore/WebDAV 的单本/多本确认状态机收敛到 Index 共享导入控制器，重新审查取消、统一分组与结果刷新。
3. **P1-E3**：已完成，见 `docs/compat/workspace-storage-import-p1e3-contract.md`；已移除未获授权的递归/目录/重命名/下载/格式扩展 UI，补回上游列表、上传和格式入口语义，并保留 P1-E2 的共享确认状态机。
4. **P1-E4**：以真实 EPUB、CBZ、PDF、TXT、标准 UMD 及旧挂载卷做格式、资源、升级和 Docker 回归；此前的历史 smoke 不能替代此门禁。
