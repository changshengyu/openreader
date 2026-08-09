# 书源调试固定基准第二轮复审合同（P2）

状态：`aligned / Docker-published / awaiting-device-verification`

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本轮只提取合同，不修改应用代码。当前 `SourceManager.vue` 的三个测试标签和
`backend/api/source_debug.go` 不能作为正确行为的依据。

## 1. 固定基准证据

权威文件：

- `web/src/views/Index.vue#debugBookSource`
- `web/public/bookSourceDebug/index.html`
- `web/public/bookSourceDebug/index.js`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`
- `src/main/java/com/htmake/reader/api/controller/BookSourceController.kt`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#bookSourceDebugSSE`
- `src/main/java/io/legado/app/model/Debugger.kt`
- `src/main/java/io/legado/app/model/webBook/WebBook.kt`

当前对应文件：

- `frontend/src/layouts/AppLayout.vue`
- `frontend/src/components/workspace/SourceManager.vue`
- `frontend/src/api/sources.js`
- `backend/api/server.go`
- `backend/api/source_debug.go`
- `backend/api/source_errors.go`
- `backend/api/source_failure.go`
- `backend/engine/source_parser.go`

## 2. 可见结构与入口合同

| 关注点 | 固定基准 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| Index 入口 | “调试书源”调用 `window.open(...bookSourceDebug/#domain=<api>, "_target")`，在命名的新标签中打开独立编辑器；不把普通书源管理面板改成调试面板。 | `AppLayout` 打开 Sources overlay，并让 `SourceManager` 自动挑第一条启用源。 | `must-fix`：恢复独立调试工作区和命名窗口；普通 Sources overlay 保持管理职责。 |
| 工作区 | 页面占满视口：左侧完整规则表单、中间固定命令栏、右侧“编辑源/调试源/源列表/帮助信息”输出区，各列独立滚动。 | 一个 720px Dialog，只有搜索/目录/正文三个标签和 JSON `<pre>`。 | `must-fix`：当前 Dialog 结构删除，不作为重建骨架。Vue 3/Element Plus 可等价实现，但信息架构、命令和并存关系必须恢复。 |
| 当前规则 | 基础、搜索、发现、详情、目录、正文和其它规则同时属于一个可编辑源；“生成源/编辑源”在表单与完整 reader-dev JSON 间双向转换。 | 调试只能读取已持久化 `BookSource`；编辑 Drawer 与调试 Dialog 相互割裂，不能调试未确认的表单。 | `must-fix`：调试动作必须使用当前编辑器快照，并在发起调试前按上游先保存该快照。 |
| 命令 | 推送、拉取、编辑 JSON、生成 JSON、清空、撤销、重做、调试、保存，以及源列表导入/导出/删除/清空。 | 调试 Dialog 无这些命令；管理页另有部分不等价动作。 | `must-fix`：恢复调试工作区命令；CRUD/import/export 复用现有唯一 API/转换工具，不能再复制一套字段映射。 |
| 浏览器状态 | `debug@BookSources` 保存本地源列表；`debug@bookSourceRecord` 保存 `old/now/new`，撤销栈最多 50；hash `tab` 恢复右侧标签。 | 无可恢复的调试状态、撤销/重做或 tab hash。 | `must-fix`，但 OpenReader 必须按账号 scope key，避免多用户在同一浏览器互相看到草稿；这是允许的多用户安全适配。 |
| 默认关键字 | 输入为空时使用“我的”；Enter 与调试按钮等价。 | 打开时清空；空值点击/Enter 不执行且无提示。 | `must-fix`。 |
| 手机布局 | 固定基准页面没有专门移动断点，三列在窄屏并不理想。 | Dialog 在 mini 界面全屏。 | `acceptable-change`：可以提供单列/分区移动布局，但不得删除任何上游命令或改变状态机。 |

## 3. 调试状态机合同

### 3.1 启动和关键字分派

1. 用户点击调试或在关键字输入框按 Enter。
2. 当前完整规则对象先以 `bookSourceUrl` 身份保存到当前用户的书源集合；失败时不发起网络调试。
3. 保存成功后清空旧控制台，显示源名和关键字，启动一条可取消的流。
4. 固定基准按以下优先顺序解释关键字：

| 关键字形式 | 起始阶段 | 后续阶段 |
|---|---|---|
| 绝对 URL | 详情 | 目录 → 第一章正文 |
| 包含 `::` | 发现 URL（取 `::` 后内容） | 第一条结果详情 → 目录 → 第一章正文 |
| `++<url>` | 目录 URL | 第一章正文 |
| `--<url>` | 正文 URL | 无自动前序阶段 |
| 其它 | 搜索词 | 第一条结果详情 → 目录 → 第一章正文 |

搜索/发现为空时记录“未获取到书籍”并正常结束；目录为空时记录“目录列表为空”并正常结束。
搜索、发现和目录都只选择第一条进入下一阶段。正文解析使用第一章，同时把第二章 URL 作为
`nextChapterUrl`，防止正文分页越过章节边界。

### 3.2 状态和日志

- 同一工作区同一时刻只允许一个调试任务；再次启动先取消旧任务。
- 控制台按发生顺序追加日志并自动滚到底部；不能用最后一个 JSON 覆盖此前阶段。
- 每个阶段至少有 `start`、`success | empty | error` 事件，并有严格递增序号和耗时。
- 固定基准在搜索/发现后只把首条 `bookUrl` 送入 `getBookInfo(bookUrl)`，因此搜索结果自身的
  `SearchBook.variable` **不会**进入详情；OpenReader 不得把“理想上应传递”误写成上游合同。
- 详情解析从一个新 Book runtime 开始；详情产生的 `Book.variable` 必须进入目录，目录继续更新的
  `Book.variable`、首章 `BookChapter.variable`、最终书名、章名和第二章 URL 必须进入正文。
- 关闭页面、退出登录、账号切换或 AbortSignal 取消后，后端停止后续请求；取消不是失败源。
- 完成流必须有唯一 `end`；终端错误必须有唯一 `error`，客户端不得同时显示成功结束。

当前“手工复制第一条 URL → 独立目录请求 → 手工复制第一章 URL → 独立正文请求”会丢失变量、
详情解析状态和下一章边界，判定为 `must-fix`。

## 4. API 合同

OpenReader 保留已部署 REST 路径作为兼容端点，并增加一个 JWT 友好的流式翻译层；不复制
reader-dev 的 cookie-only EventSource 身份模型。

### 4.1 新的规范调试流

| 字段 | 合同 |
|---|---|
| 方法/路径 | `POST /api/sources/:id/debug/stream` |
| Auth | Bearer JWT；源必须属于当前账号的 active association。缺失/无效 JWT 为现有 `401`；外账号或 detached ID 为 `404 {"error":"source not found"}`。流本身只读，不额外要求管理员；前置保存继续要求 `CanEditSources`。 |
| Body | JSON `{ "keyword": string }`，请求体遵守全局有界 JSON；缺失、非字符串或空白在服务端归一为上游默认“我的”。 |
| Success headers | `200`, `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`。 |
| Events | `log`/`stage` 数据至少包含 `{seq, elapsedMs, stage, status?, message}`；终端 `end` 包含安全摘要；终端 `error` 保留安全 `error/code/stage`。未知新增字段可由旧客户端忽略。 |
| Side effects | **零持久化副作用**：不改书架、章节、缓存、变量表、书源、`source_failures` 或同步事件。当前规则的持久化由发流前现有 create/update API 完成。 |
| Cancellation | 使用请求 Context；客户端断开/Abort 后不再开始新远程请求，不写失败缓存，不发送伪 `end`。 |
| Errors before stream | 非法 ID `400`、源不存在 `404`、加载失败 `500`；均用现有 `{error}` JSON。 |
| Errors after stream | HTTP 已为 200 时只发送一个 `event:error`，包含脱敏错误和 `code/stage`；不把 URL query、header/cookie、变量值、响应正文、JWT、WebDAV 凭据或主机路径写入 SSE。 |

使用 `fetch` + Bearer 读取 SSE 是 `technical-stack-equivalent`：原生 `EventSource` 不能携带 OpenReader
现有 Authorization header。不得把 JWT 放入 URL query。

### 4.2 已部署兼容端点

| 方法/路径 | Body | 当前成功响应 | 必须保留/修正 |
|---|---|---|---|
| `POST /api/sources/:id/test` | `{keyword}` | `200 {results,error,code?,stage?}` | 保留字段和状态；当前账号 scope；改为无 `source_failures` 副作用。规范 UI 不再依赖它串联阶段。 |
| `POST /api/sources/:id/test-chapter` | `{bookUrl}` | `200 {chapters,count,error,code?,stage?}` | 保留字段和状态；无失败缓存。该旧 body 无法携带搜索变量，只是兼容探针，不能作为完整上游链证明。 |
| `POST /api/sources/:id/test-content` | `{chapterUrl}` | `200 {content,fullLength,error,code?,stage?}`；`content` 最多 2000 rune | 保留字段、截断和状态；无失败缓存。该旧 body 无变量/下一章上下文，只是兼容探针。 |

三条旧端点共同遵守：JWT `401`；非法 ID `400`；外账号/detached `404`；缺失 required 字段 `400`；
读取失败 `500`；解析或远程失败继续放在 authenticated `200` debug envelope 中。`code/stage` 仍是
可选安全附加字段。此前文档把远程失败写入 `source_failures` 视为正确行为，现由固定基准推翻。
批量“失效检测”是独立显式健康动作，仍可按其合同记录失败；调试不是健康检测。

## 5. 数据、权限和安全

- 不新增 SQLite 表或破坏性迁移；当前 source create/update 的事务、copy-on-write 和用户 association
  继续权威。
- 保存当前规则需要 `CanEditSources`，这是 OpenReader 多用户权限适配；无权限用户不能借调试工作区
  修改源。
- 本地草稿、撤销栈和源列表缓存必须以稳定用户 identity + schema version scope；注销清空内存，
  切换账号不得恢复另一账号内容。
- 调试仍使用共享 fetcher 的 timeout、16 MiB body、5 redirect、3 retry、SSRF/DNS/proxy policy；
  不复制上游无界网络行为。
- 不执行 JavaScript/WebView。遇到这些规则时在对应阶段发送
  `source_rule_unsupported`，保留原始配置但不输出脚本内容；这是已批准的 Go/security 差异。
- 日志有事件数与总字节上限；超过上限发送安全终端错误并停止，防止规则制造浏览器/服务端 DoS。

## 6. 预定测试闸门

### 后端失败合同

1. 搜索 → 详情 → 目录 → 正文按顺序运行，只取第一条/第一章；搜索结果 variable 按固定基准丢弃，
   详情/目录产生的 Book/Chapter variable 与 `nextChapterUrl` 连续传递。
2. 绝对 URL、`::`、`++`、`--` 五种入口的阶段序列准确；空搜索/目录正常 `end`。
3. 外账号、detached、401、非法 ID、取消、请求/规则错误状态和唯一终端事件。
4. 调试远程失败、规则失败和取消均不创建/更新 `source_failures`；批量健康检查仍按原合同记录。
5. SSE 不泄漏 URL query、动态 header、变量、响应 body、JWT、主机路径；事件/字节有界。
6. 三个旧 `/test*` 端点响应回归，同时证明失败缓存副作用已移除。

### 前端合同

1. Index “调试书源”打开独立命名标签；普通书源 overlay 不自动挑第一条弹出旧 Dialog。
2. 表单/JSON双向转换，先保存后调试；保存失败不发流；默认“我的”和 Enter 等价。
3. 流式控制台追加而非覆盖，阶段顺序、自动滚动、再次运行取消、关闭/401/账号切换取消。
4. localStorage 用户隔离、50 步撤销/重做、tab 恢复，以及 pull/push/import/export/delete/clear。
5. 1440×900、390×844、360×800 和 1024×1366 真实浏览器；窄屏保留全部命令且无不可关闭遮罩。

### 发布门

- focused Go + race、`go test ./...`、`go vet ./...`。
- frontend focused + full tests、production build。
- 真实 CSS/JSONPath/XPath 链路和取消链路。
- fresh/historical volume、portable backup/restart；本批无 schema 变化仍需证明旧卷可启动。
- 完成上述门后可作为半模块本地构建并发布 Docker；报告必须说明 JavaScript/WebView 仍是允许差异。

## 7. 实施与验证结果（2026-08-09）

- `SourceManager.vue` 已删除错误的三探针 Dialog；Index 与书源管理均用命名 `_target` 标签打开独立
  `/source-debug` 工作区，旧 `/bookSourceDebug` 和 `/sources?action=debug` 继续兼容跳转。
- 新工作区恢复完整规则表单、九项命令栏、JSON/调试/源列表/帮助标签、账号 scoped 本地源列表、
  最多 50 步 `old/now/new` 历史、hash 恢复，以及 pull/push/import/export/delete/clear。表单生成和
  导出采用完整 reader-dev JSON；`rules` 仅作为与后端正式导出器一致的无损 OpenReader 扩展。
- `POST /api/sources/:id/debug/stream` 已实现五类关键字分派、搜索/详情/目录/正文自动链、固定上游
  variable 边界、第二章 URL 边界、严格事件顺序、唯一终端、取消和有界脱敏日志。三个旧探针保持
  既有响应/status，但所有调试路径均不再写 `source_failures`。
- focused/race、全量 Go、`go vet ./...`、frontend 724/724、production build 和 `git diff --check`
  通过。真实 Go/SQLite/Chromium 在 1440×900、1024×1366、390×844、360×800 验证 save-before-stream、
  Bearer header、reader-dev JSON、完整链、无相邻章节越界、无失败缓存污染、hash 恢复及传输取消；
  控制台没有正文、JWT、secret、变量或 URL 凭据泄漏。smoke 也会有界清理临时 Go/Chromium 进程。
- 本批不改 SQLite schema、书架/章节/缓存、备份和挂载目录。实现提交 `f8f263d` 已推送 `main`；
  fresh volume 的 portable v1/v2 assets、cross-user、restart 与 historical TXT/EPUB/UMD/CBZ、
  relative-cache、owner-isolation 门均通过。只使用本机 OrbStack 构建并推送
  `ghcr.io/changshengyu/openreader:f8f263d` 和 `latest`，二者 OCI index 均为
  `sha256:9c83821de9e5f4df223b6e69a6d67eff512fa55d4a271f544718ccad8ae58ba1`；amd64 为
  `sha256:8558b903640579fcd18970cf1f02e8044a081bfd6a7356dae9702eba8fd03351`，arm64 为
  `sha256:b2ce3d3624053a8340a662e77fae5be55aeadd9379276a478d4585b08aefce0f`。
