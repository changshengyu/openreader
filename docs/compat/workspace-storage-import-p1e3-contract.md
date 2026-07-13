# P1-E3 工作台文件管理与格式入口兼容合同

状态：**已完成上游审查，尚未实施。**  
基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
上游证据：`web/src/components/LocalStore.vue`、`web/src/components/WebDAV.vue`、`web/src/views/Index.vue`、`BookController.kt:2263-2531`。  
当前映射：`OverlayLocalStore.vue`、`LocalStore.vue`、`OverlayWebDAV.vue`、`WebDAVBrowser.vue`、`backend/api/localstore.go`、`backend/api/webdav.go`、`backend/services/localbook/importer.go`。

本合同只处理 P1-E2 之后仍残留的文件管理器偏差：可见操作、目录列表、上传语义和可导入格式入口。它不改变已经发布的 P1-E1 暂存 token 重试、P1-E2 共享确认状态机，也不删除已有书籍、缓存、挂载卷或旧 API 路径。

## 1. 上游权威行为

| 场景 | LocalStore 上游行为 | WebDAV 上游行为 | 不可丢失的结果 |
|---|---|---|---|
| 打开与目录 | 每次打开从 `/` 读取当前目录；目录链接进入子目录，非根目录第一行有 `..` 返回父级。 | 同样每次打开 `/`，同样以 `..` 返回父级。 | 打开不继承上次目录/选择；目录浏览不是递归混合树。 |
| 表格与搜索 | 同一张 Element 表格；文件名、大小、修改时间；搜索框位于“操作”表头，关键字长度大于 2 才过滤；超过 101 项时表格尾部显示一行“加载更多”。 | 同一张 Element 表格；文件名、大小、修改时间；没有 LocalStore 的关键字筛选或格式筛选。 | 移动端仍是同一张固定选择/名称列的表格语义，而不是另一套操作流。 |
| 单项操作 | 删除（文件或目录）与可导入文件的“加入书架”。 | `.zip` 还原、所有文件下载、可导入文件“加入书架”、删除（文件或目录）。 | 每次删除先确认，成功后刷新当前目录；不可导入项不出现导入按钮。 |
| 批量操作 | 批量删除、批量加入书架、上传书籍、取消。 | 批量删除、批量加入书架、上传文件、取消。 | 批量导入只交给 `Index.importMultiBooks`；当前 P1-E2 的共享 controller 是等价宿主。 |
| 上传 | 文件输入允许多选，不设客户端格式白名单；服务端把每个上传原样保存到当前目录。 | 文件输入允许多选，不设客户端格式白名单；服务端把每个上传原样保存到当前 WebDAV 目录。 | 上传成功刷新当前目录；上传并不等于导入，普通非书籍文件仍可作为文件管理项目存在。 |
| 可导入格式 | 仅 `.txt`、`.epub`、`.umd`、`.cbz`。 | 仅 `.txt`、`.epub`、`.umd`。 | “可见导入入口”必须严格按来源分别判断，不能以当前 Go parser 可处理的格式反推上游 UI。 |
| 非上游操作 | 没有新建目录、重命名、下载、递归扫描、按格式筛选、导入当前目录、导入筛选或导入目录。 | 没有刷新按钮、新建目录、重命名、导入目录。 | 不把当前额外能力视为上游重构成果；从主 UI 移除。 |

## 2. 当前差异和处置

| 合同层 | 当前实现 | 判定 | P1-E3 处置 |
|---|---|---|---|
| LocalStore 列表 | 当前有面包屑、格式列、路径列、格式筛选和 `recursiveScan`；列表响应没有 `lastModified`。 | `must-fix` | 重建为当前目录列表，补回修改时间列、搜索阈值和表内“加载更多”行；移除格式筛选、递归开关及递归请求。面包屑可作为 Vue 3 的目录返回实现保留，前提是能等价回到每级父目录。 |
| LocalStore 额外操作 | 刷新、新建目录、重命名、下载、导入当前目录、导入筛选、导入目录均对用户可见。 | `must-fix` | 从 LocalStore 主 UI 删除；现有 Go 路由仅作为旧客户端/API 兼容层保留，不得再由工作台入口调用。 |
| WebDAV 额外操作 | 刷新、新建目录、重命名、导入目录均对用户可见。 | `must-fix` | 从 WebDAV 主 UI 删除；原始 WebDAV `MKCOL/MOVE` 等协议端点因客户端兼容保留，但不再加入工作台操作菜单。 |
| WebDAV 恢复/下载/删除 | `.zip` 恢复确认、文件下载、单项/批量删除已存在。 | `technical-stack-equivalent` | 保留，复核确认文案、当前目录刷新与 `applyRestoreResult` 的数据同步。 |
| 格式入口 | 两个 UI 都公开 `.text/.md/.pdf/.cbz`；WebDAV 额外公开 `.cbz`。 | `must-fix` | LocalStore 仅显示 `.txt/.epub/.umd/.cbz` 的导入入口；WebDAV 仅显示 `.txt/.epub/.umd`。`.text/.md/.pdf` 和 WebDAV `.cbz` 不再显示导入按钮或进入 P1-E2 controller。 |
| Go parser 能力 | Go 可解析 `.text/.md/.pdf/.cbz`，`/local-store/import`、`/webdav/import` 也能接受它们。 | `acceptable-change` | 保留已导入书籍、直接 API 与 parser，避免破坏用户数据和旧客户端；P1-E3 只收窄上游工作台的可见入口。此差异必须继续记录，不能再次作为 UI 对齐依据。 |
| 上传 | LocalStore 前端 `accept` 限制格式，Go 仅接收一个 `file` 且拒绝非可导入扩展；WebDAV 前端也只能一次选择一个文件。 | `must-fix` | 恢复多选且不设前端格式白名单。LocalStore API 改为接受多个同名 multipart `file` 字段，并在大小/路径校验后逐个原样保存；不把“可上传”绑定到“可导入”。WebDAV 通过安全的顺序 PUT 达到同一可见多文件结果。 |
| 当前目录与隐藏文件 | Go 支持递归并可能返回点文件；上游只列当前目录且跳过 `.` 开头条目。 | `must-fix` | 工作台请求不再传递 `recursive`，后端列表默认/工作台行为跳过隐藏项；递归 endpoint 如为旧客户端兼容保留，不能暴露在 P1-E3 工作台。 |
| P1-E1/E2 | 暂存 preview/token 重试与共享导入 controller 已通过回归。 | `aligned` | 不重写；格式入口收窄后，所有允许的文件仍进入相同的单书/多书确认状态机。 |

## 3. 数据、安全与兼容边界

1. 不做 SQLite migration，不移动 `data/`、`cache/`、`library/`、`localStore/` 或 WebDAV 私有根；已有 `.md/.pdf/.text/.cbz` 书籍照常可读。
2. 不删除 `POST /local-store/directory`、`PUT /local-store/rename`、`GET /local-store/download`、WebDAV `MKCOL/MOVE` 或递归参数；它们可服务旧客户端，但新的工作台不得产生这些请求。
3. LocalStore 多文件上传必须保持已有大小上限、路径归一化、私有用户根、临时文件原子替换与失败时不破坏同名旧文件的安全保证。不能为多选恢复而接受目录名、路径分隔符或越权目标。
4. WebDAV 的原始认证、`Destination` 路径校验、重定向/下载授权、ZIP 恢复边界继续由现有安全合同约束；P1-E3 不扩大其协议权限。
5. P1-E1 暂存 token 仍是确认导入的唯一字节来源。收窄前端格式入口不得导致已选合法上游格式重新读取挂载文件或绕过 token。

## 4. 实施顺序与测试门禁

先写/替换测试，再改代码：

| 层级 | 必须新增或替换的断言 |
|---|---|
| 前端结构 | LocalStore 不再包含新建目录、重命名、下载、递归、格式筛选、目录/筛选导入入口；WebDAV 不再包含新建目录、重命名、目录导入入口；两者保留上游单项/批量删除、上传和允许格式的 `openStorageImport`。 |
| 前端格式 | LocalStore `canImport`/展示映射仅 `txt/epub/umd/cbz`；WebDAV 仅 `txt/epub/umd`；`.md/.text/.pdf` 与 WebDAV `.cbz` 不触发 controller。 |
| LocalStore API | 当前目录默认非递归、隐藏项不返回、`lastModified` 存在；多文件上传接受普通非书籍文件，逐项写入、大小超限仍为 413、任一失败不得覆盖先前已有文件。 |
| 浏览器 | 1440×900、390×844、360×800：两种管理器打开均回到根目录；目录返回、搜索阈值/101 项加载更多（LocalStore）、批量删除确认、允许格式的导入进入 P1-E2 controller、禁止格式无导入入口、多选上传刷新当前目录、无横向溢出和点击穿透。 |
| 保留回归 | P1-E1 token reparse、P1-E2 单书/批量/逐本状态机、WebDAV `.zip` 恢复、旧 API/多用户隔离、Docker volume/backup smoke。 |

## 5. 允许差异

- Vue 3/Element Plus 的面包屑、移动响应式和 Pinia 共享 controller 可以不同于 Vue 2/Vuex，只要目录、选择、取消、格式可见性和导入写入顺序等价。
- Go 的多用户私有目录、大小边界、暂存 token、安全错误和保留旧 API 是必要运行时适配。
- Go parser 对额外格式的保留只是一项向后兼容能力；不授权在 P1-E3 工作台重新公开这些入口。

## 6. 后续

完成 P1-E3 后，再进入 P1-E4：用真实 TXT、EPUB、UMD、CBZ、PDF、Markdown 和旧挂载卷验证 parser/导入/阅读语义。P1-E4 才评估额外格式是否应通过单独、明确授权的产品入口暴露，不能绕过 reader-dev 工作台合同。
