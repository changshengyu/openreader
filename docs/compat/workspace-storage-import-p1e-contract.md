# P1-E 工作台书仓、WebDAV 与本地导入兼容合同

状态：**P1-E1 已实现并完成回归；P1-E2~P1-E4 仍未开始。**
基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
上游证据：`web/src/views/Index.vue`、`web/src/components/LocalStore.vue`、`web/src/components/WebDAV.vue`、`BookController.kt`、`LocalBook.kt`、`TextFile.kt`。  
当前映射：`OverlayLocalStore.vue`、`LocalStore.vue`、`OverlayWebDAV.vue`、`WebDAVBrowser.vue`、`LocalBookImportPreviewDialog.vue`、`backend/api/localstore.go`、`backend/api/webdav.go`、`backend/api/local_import_stage.go`、`backend/services/localbook/*`。

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
| 当前成功预览后的规则编辑 | `LocalBookImportPreviewDialog` 允许编辑 TXT/EPUB `tocRule`，但只在“确认导入”时随请求发送；预览章节数仍是旧规则的结果。 | `must-fix` | 规则改动必须显式使用原 token 调用 preview/reparse，刷新章节数、目录和错误状态，然后才允许确认导入。 |
| 当前失败预览后的规则重试 | 失败行仅渲染错误文字；`previewLocalStoreImport(paths)` / `previewWebDAVImport(paths)` 只能发送 `paths`，虽然后端已支持 `{items:[{path, importToken, tocRule}]}`。 | `must-fix` | 失败的 TXT 行必须保留令牌并可填写规则、点击“重新解析”。不得要求用户重新上传或重新读取 LocalStore/WebDAV 源文件。此缺口是“目录解析失败后看似受网速影响”的首要 UI 候选原因。 |
| 导入确认链路 | 当前 LocalStore/WebDAV 各自直接打开同一个批量预览 dialog，然后一次性确认、展示结果；不复刻上游“单本逐本 dialog / 多本选择批量或逐一确认”的状态机。 | `must-fix` | 下一阶段必须把两者收敛到一个工作台导入控制器，并先恢复上游单本/多本/取消/统一分组状态转换。保留可编辑表格式预览只能作为不替换这些分支的安全增强。 |
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

## 4. P1-E 后续顺序

1. **P1-E1**：暂存 token 的可见重试桥接（本合同定义的最小切片）。
2. **P1-E2**：将 LocalStore/WebDAV 的单本/多本确认状态机收敛到 Index 共享导入控制器，重新审查取消、统一分组与结果刷新。
3. **P1-E3**：逐一处理未获授权的递归/目录/重命名/下载/格式扩展，删除错误 UI 或补写明确的用户授权与兼容依据。
4. **P1-E4**：以真实 EPUB、CBZ、PDF、TXT、标准 UMD 及旧挂载卷做格式、资源、升级和 Docker 回归；此前的历史 smoke 不能替代此门禁。
