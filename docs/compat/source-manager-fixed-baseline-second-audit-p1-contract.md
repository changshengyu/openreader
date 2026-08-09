# 书源管理器第二轮固定基准兼容合同（P1）

状态：`contract-complete / implementation-blocked-until-red-tests`

固定权威：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

本轮只审查用户从 Index 打开的“书源管理 / 失效书源管理 / 导入书源 / 远程书源”工作流。
`BookSource.vue` 是 Reader 内换源面板，不是本合同的书源管理器；独立 `SourceDebug.vue` 已由书源调试器
第二轮合同约束，不得重新塞回管理器。

## 1. 权威源码和审查范围

- `web/src/views/Index.vue:720-890`：管理器 Dialog、失败检测表单、分组、表格、分页和 footer。
- `web/src/views/Index.vue:1699-1909`：本地/远程导入、导入预览和确认。
- `web/src/views/Index.vue:1964-2001`、`2238-2244`：失败缓存入口和显式检测。
- `web/src/views/Index.vue:2480-2614`：导出、清空、恢复默认和 JSON 编辑器。
- `web/src/views/Index.vue:2878-2952`：列表、分组、失败类型过滤和分页状态。
- `web/src/plugins/vuex.js:592-618`：mini interface 和共享 Dialog 几何。
- `web/src/plugins/config.js:171-185`：失败类型固定顺序。
- `web/src/plugins/axios.js:129-137`：普通请求错误进入失败缓存的分类规则。

当前映射：

- 唯一根 owner：`frontend/src/components/overlays/OverlaySources.vue`。
- 当前业务体：`frontend/src/components/workspace/SourceManager.vue`。
- 六个 Index 入口：`frontend/src/layouts/AppLayout.vue`。
- overlay intent：`frontend/src/stores/overlay.js` 和兼容路由 `frontend/src/router/index.js`。
- 导入控制器：`frontend/src/composables/useSourceTransfer.js`。
- API：`frontend/src/api/sources.js`、`backend/api/sources.go`、`backend/api/source_debug.go`。
- 持久化/隔离：`backend/services/booksources`、`models.BookSource`、`UserBookSource`、
  `SourceFailure` 和 `sources_update`。

## 2. 根场景、入口和 Dialog 几何

| 合同点 | 固定上游 | 当前实现 | 裁决 |
|---|---|---|---|
| 入口分工 | Index 侧栏依次为 `书源管理 → 探索书源 → 导入书源 → 远程书源 → 失效书源 → 调试书源`；六项是独立动作。 | `AppLayout.vue` 已保持六入口顺序，但 import/remote 先打开整个管理器，再嵌套 Dialog。 | `partial`：入口顺序保留；import/remote 直接进入各自流程，不显示管理器作为背景壳。 |
| owner | 正常页面中的 Index 根 Dialog；Reader 不拥有管理器。 | `GlobalOverlayHost → OverlaySources` 是唯一 owner，兼容 `/sources` 重定向到 `/`。 | `technology-equivalent`：保留唯一 owner 和旧 URL；增加正常页面 gate。 |
| 标题 | 正常为 `书源管理`，失败为 `失效书源管理`。 | 固定 `书源管理`。 | `must-fix`。 |
| mini | `miniInterface <= 750px` 时 fullscreen。 | 使用同一 responsive predicate 并 fullscreen。 | `aligned`。 |
| desktop width | `min(max(viewportWidth × 0.7, 750), 1000)px`。 | `min(1120px, 100vw - 48px)`。 | `must-fix`。 |
| desktop content/top | content 高 `min(0.7h - 184, 400)`；top `(h - contentHeight - 184) / 2`。 | Element Plus 默认 top，内容随现有组件扩展。 | `must-fix`；使用共享固定上游几何族。 |
| close reset | 仅把 failure mode 设 false、group 设空；分页和页尺寸不因销毁重建而强制重置。 | `destroy-on-close` 销毁全部状态。 | `must-fix`：移除错误的全量生命周期重置。 |

## 3. 正常管理器可见结构

固定上游唯一结构如下：

1. 标题右侧可见顺序为 `新增 → 导出 → 恢复默认 → 清空`。
2. 分组 chip 按当前列表中非空分组的首次出现顺序生成，最后始终追加 `未分组`；再次点击已选
   chip 会取消过滤。
3. 桌面和移动/fullscreen 都使用同一张 `el-table`，不得切换移动卡片。
4. 表格列严格为：
   - selection：25px；mini 时 fixed；已被书架使用的源不可选择。
   - `书源名称`：最小 120px；mini 时 fixed。
   - `书源链接`：最小 120px；外链在新标签页打开。
   - `书架书籍`：最小 120px；逐行显示引用该源的书名。
   - `操作`：100px；只有 `编辑`。
5. 表格高度为 `dialogContentHeight - 42 - 42`。
6. 分页默认每页 25；选项 `[25,50,100,200,300,400]`；layout 为
   `total,sizes,prev,pager,next`；mini pager 5，桌面 7。过滤后若当前页越界，上游显示空页，
   不暗中夹到最后一页。
7. footer 为左侧主按钮 `批量删除`、`已选择 N 个` 和 `取消`。

当前以下可见能力不是上游适配，必须从管理器删除：关键词搜索、分组下拉、搜索配置常驻区、启用开关、
健康列、只看失败、失效检测常驻按钮、停用失效源、批量启用/停用/设置分组、逐行删除/调试、
移动 card、sticky mobile batch footer，以及管理器里的导入/远程/“设为默认”按钮。管理员保存默认书源的
能力继续留在 UserManage/admin API，不得作为普通书源管理操作。

## 4. 失败书源管理状态机

| 合同点 | 必须行为 |
|---|---|
| 打开 | 先读取当前用户 600 秒失败缓存，再把同一管理器切换为 failure mode；不得自动开始 live check。 |
| 表单 | 仅在 failure mode 显示：`搜索词`、`超时(ms)`、`并发数`。默认值分别为 `斗罗大陆`、`5000`、`5`；范围/step 为 `1000..15000 / 500`、`3..15 / 1`。 |
| 分组 | 固定失败类型顺序后追加 `timeout`；按 `errorMsg.includes(type)` 过滤；再次点击取消。 |
| 表格 | selection、名称、链接、`错误信息`、书架书籍；不显示操作列。表格高度额外减 32px。 |
| footer | 批量删除、选择计数、`检测书源` 及 `requestCount/total`、取消；检测中按钮 disabled。 |
| 空关键词 | 精确错误 `请输入搜索关键词`。 |
| live check | 用户显式点击后才检查整个源列表；空选择也不改变检查范围。继续使用有界 Go API，但 UI 状态/文案必须按上游。 |

失败分类顺序锁定为：

1. `UnknownHostException`
2. `ConnectException: Failed to connect`
3. `SocketException: Connection reset`
4. `SSLHandshakeException`
5. `responseCode: 307/400/403/404/500/502/503/504/513`
6. `timeout`

OpenReader 可以返回脱敏的 Go 安全错误类，但必须映射为上述可见筛选类别，且不得泄露凭据、header、
完整 query、响应正文或宿主文件路径。

## 5. 批量删除、清空、恢复和导出

| 动作 | 固定上游可见合同 | OpenReader 保留的运行时适配 |
|---|---|---|
| 批量删除 | 空选择：`请选择需要删除的源`；确认：`确认要删除所选择的书源吗?` / `提示`；成功：`删除书源成功`；清除选择和失败行并 reload。 | 后端继续以当前用户 scope 事务删除；已被书架使用的源不可选择且服务端仍二次防护。 |
| 清空 | 确认：`确认要清空所有书源吗?`；成功：`清空书源成功`。 | 多用户 namespace、事务、derived failure 清理和 broadcast 保留。 |
| 恢复默认 | 确认：`确认要恢复默认书源吗?`；成功：`恢复默认书源成功`；失败前缀 `操作失败 `。 | 当前用户快照/系统默认、事务与 copy-on-write 保留。 |
| 导出 | 始终导出全部当前源，不读取 table selection；4-space JSON；文件名 `reader书源-<currentDateTime>.json`。 | REST blob 下载可保留；reader-dev 字段和稳定顺序不得变化。 |

列表 API 现有 `usedBookCount` 只能保护删除，不能满足上游可见 `书架书籍`。实现阶段应增量返回
`usedBookNames: string[]`（`gorm:"-"`，无迁移），按当前用户和稳定书架顺序查询；不得把其他用户书名
投影进来。

## 6. 新增/编辑 JSON 合同

上游使用根级通用 JSON 编辑器，不是字段化表单或 Drawer：桌面使用共享 `dialogWidth` 且 top `10vh`，
mini fullscreen；标题新增/编辑都为 `编辑书源`；footer 为 `取 消`、`保 存`。

新增 JSON 的最小默认对象锁定为：

```json
{
    "bookSourceComment": "",
    "bookSourceGroup": "",
    "bookSourceName": "新增书源",
    "bookSourceType": 0,
    "bookSourceUrl": "",
    "bookUrlPattern": "",
    "enabled": true,
    "enabledExplore": true,
    "exploreUrl": "",
    "ruleBookInfo": {},
    "ruleContent": { "content": "" },
    "ruleExplore": {},
    "ruleSearch": {
        "author": "",
        "bookList": "",
        "bookUrl": "",
        "coverUrl": "",
        "intro": "",
        "kind": "",
        "lastChapter": "",
        "name": ""
    },
    "ruleToc": {
        "chapterList": "",
        "chapterName": "",
        "chapterUrl": ""
    },
    "searchUrl": ""
}
```

编辑先读取该行完整配置，再以 4-space reader-dev JSON 打开。保存依次验证：

- 缺名称：`书源名称不能为空`。
- 缺 URL：`书源链接不能为空`。
- JSON 无法解析：`书源必须是JSON格式`。
- 成功：`保存书源成功`，关闭编辑器并 reload。

前端可使用等价的 monospaced textarea（当前依赖中没有 CodeJar/Prism），但必须允许完整 JSON 编辑、
保留未知/不执行字段并按 reader-dev alias 往返。OpenReader 更新可继续用数值 ID 以安全保持书架关联；
创建/更新必须走现有 payload 转换，不能把未知字段或 dormant JavaScript/WebView 配置静默删除或执行。

## 7. 本地/远程导入合同

- 本地导入是侧栏独立文件动作。非空 JSON 数组进入 `导入书源` 预览；解析失败或空数组精确显示
  `书源文件错误`。
- 远程导入先 prompt：`请输入远程书源链接` / `导入远程书源文件`，记住当前用户上次 URL；
  成功进入同一个导入预览。空/无效结果为 `远程书源文件错误`；请求错误前缀
  `读取远程书源文件内容失败 `。
- 预览 desktop 使用共享 width、top `15vh`，mini fullscreen；checkbox 行显示名称、URL 和标签。
- footer：左侧 bordered `全选`、`已选择 N 个`、`取消`、主按钮 `确定`。
- 全选默认跳过真实 `@js:` / `webView:` 可执行源并提示
  `部分使用了Javascript和Webview的书源未勾选`；安全增强可额外识别实际可执行动态字段，
  但 dormant 字段仍允许手动选择并原样保存，服务端不得执行。
- 空选择：`请选择需要导入的源`；成功：`导入书源成功`，reload、关闭并重置预览。
- 远程抓取继续保留 SSRF、scheme/host、DNS/rebinding、redirect、timeout、body size 和 credential 限制。

当前 `useSourceTransfer` 的用户隔离、operation generation、同一预览/确认事务和安全筛选可保留；
需要纠正默认自动全选、错误文案、文件名/远程文案和 manager 嵌套 ownership。

## 8. API、数据和同步映射

Go REST 路径不是可见上游合同，可以保留，但每个动作必须映射到上述状态：

| 动作 | OpenReader API | 本轮要求 |
|---|---|---|
| list/detail/create/update | `GET/POST /sources`、`GET/PUT /sources/:id` | list 增量投影当前用户 `usedBookNames`；detail/editor 使用完整 reader-dev JSON 往返。 |
| delete/clear | `DELETE /sources/:id`、`DELETE /sources`、`POST /sources/batch` | 只暴露上游批量删除/清空 UI；保留服务端 usage guard、事务和兼容 API。 |
| default restore | `GET /sources/default`、`POST /sources/default/restore` | 管理器只显示恢复默认；admin 保存默认留在 UserManage。 |
| export/import | `GET /sources/export`、`POST /sources/import` | 全量 4-space 导出；导入只在确认预览后 mutation。 |
| remote preview | `POST /sources/remote-preview` | 只读、安全、有界；确认后复用 import mutation。 |
| failure | `GET /sources/invalid`、`POST /sources/batch-test` | entry 只读 cache；live check 只来自显式按钮。 |
| sync | WebSocket `sources_update` → browser `openreader:sources-update` | durable mutation 后 reload；账号切换/关闭后的旧请求不得写入新 session。 |

不进行数据库破坏性迁移。`usedBookNames` 是响应投影；现有 `data/`、`cache/`、`library/`、backup、
WebDAV 和历史卷格式不变。

## 9. 旧 P1-C 结论撤销

`reader-dev-openreader-gap-analysis.md` 的 2026-07-10 P1-C 曾把“当前页移动卡片、结构化 Drawer
编辑器、管理器内 import/remote/debug 和额外 batch/health 控件”标成 Vue 3/安全改进。第二轮逐行
复审确认这些改变了上游可见结构、动作入口和状态机，不能继续作为保留理由。只保留：

- Vue 3/Pinia/Element Plus 的实现方式；
- Go/JWT/SQLite 多用户隔离、事务和 REST 路径；
- 远程抓取/脚本源安全边界；
- 用户 scoped failure cache 和同步事件；
- 兼容旧 URL 与隐藏兼容 API；
- 已独立重建的 SourceDebug 工作区。

## 10. 测试先行与实施门禁

合同提交后，应用代码必须按以下顺序推进：

1. 新增静态失败合同，锁定动态标题/几何、单 table、列和 footer、分组顺序、失败表单、JSON editor；
   明确禁止 mobile cards、Drawer 和额外管理命令。先在当前实现上证明失败。
2. 更新 overlay intent/transfer 测试，锁定 import/remote 独立入口、failure entry 零 live check、close reset。
3. 新增后端失败合同，锁定 `usedBookNames` 当前用户投影、稳定顺序、used-source guard 和无迁移。
4. 重建 outer shell、正常/失败 manager、JSON editor，再拆出共享 import/remote preview。
5. 替换 `scripts/smoke/source-workspace-contract.mjs` 中把 mobile card、sticky footer、结构化 Drawer
   当成正确行为的旧断言；保留 standalone source debugger、session isolation、failure cache 和 security 回归。
6. 运行 frontend full test/build、Go full/focused race/vet、diff check；真实浏览器覆盖
   1440×900、1024×1366、390×844、360×800，以及正常/失败/新增编辑/本地导入/远程导入/旧 URL。
7. fresh/historical volume 和 backup compatibility 通过后，形成可验收切片并本地构建、推送 GHCR。

本合同完成前禁止书源管理应用代码修改；红测试通过定义前禁止实现。
