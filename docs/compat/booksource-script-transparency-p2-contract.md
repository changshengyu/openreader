# 书源脚本兼容性提示合同（P2）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 已完成固定基准与当前调用链盘点；本文件是测试和实现的前置闸门，
本阶段未修改应用代码。

## 权威文件

- 上游 `web/src/views/Index.vue`
  - `handleCheckAllChange()`、`getSourceTag()`、本地/远程书源导入预览。
- 上游 `BaseSource.kt`、`AnalyzeUrl.kt`、`AnalyzeRule.kt`、`WebBook.kt`
  - 动态 Header、`loginCheckJs`、规则脚本和 WebView 的真实执行点。
- 当前 `frontend/src/composables/useSourceTransfer.js`
- 当前 `frontend/src/components/workspace/SourceManager.vue`
- 当前 `backend/engine/source_parser.go#ensureSourceScriptEntryPointsSupported`
- 当前 `backend/api/source_errors.go`

## 上游与 OpenReader 合同

| 范围 | 固定上游行为 | 当前 OpenReader | 判定与要求 |
|---|---|---|---|
| 导入预览 | 上游序列化整条书源，发现 `@js:` 或 `webView:` 时显示 `@Javascript` / `@WebView`，并且不默认勾选；用户仍可手动选择，以便无损保存。 | 当前只复制这两个字符串判断。`<js>...</js>` 动态 Header 和非空 `loginCheckJs` 会被默认勾选，但后端使用时必然拒绝。 | `must-fix`：预览分析必须与当前真实运行时能力一致；确定无法执行的源不得默认勾选，但仍允许用户明确选择并无损导入。 |
| 动态 Header | 上游在每次请求前执行 `header: "@js:…"` 或 `<js>…</js>`。 | Go 在任何远端请求前返回 `source_rule_unsupported`，字段无损保存。 | `acceptable security difference`；导入和编辑界面必须明确显示“配置会保留、当前服务不会执行”。 |
| 登录检测 | 上游搜索、探索、详情和目录响应后执行 `loginCheckJs`。 | Go 在请求前拒绝非空 `loginCheckJs`，避免把登录页当书籍/目录/正文。 | `acceptable security difference`；导入预览必须识别并取消默认勾选，编辑器必须显示阻断原因。 |
| 规则脚本/模板 | 上游执行字段规则中的 `@js:`、`<js>` 与 `{{…}}`。 | 统一规则解释器返回安全的 `source_rule_unsupported`，不写失效源缓存。 | `acceptable security difference`；分析器需要识别实际规则字段中的脚本/模板，不能等到阅读空白后才暴露。 |
| WebView | 上游可使用 `webView:` 流程。 | Go 单容器没有浏览器 WebView 执行环境。 | 保留上游 `@WebView` 标签和非默认选择；明确当前不支持。 |
| 固定基准未消费字段 | `ruleToc.preUpdateJs`、普通 HTTP 路径中的 `ruleContent.webJs/sourceRegex` 在本固定提交的实际调用链没有执行。 | OpenReader 无损保存但不执行。 | `aligned-dormant`：显示“保留字段”说明即可，不得把它们误报成当前必然阻断，也不得擅自执行。 |
| 调试反馈 | 上游调试链直接展示规则异常。 | API 已返回安全的 `code`、`stage`、`error`，但管理器只原样打印 JSON。 | `must-fix UX`：在保留原始安全 JSON 的同时显示可读结论，指出不支持类别和阶段；不得回显脚本、Header、cookie、URL query、JWT 或响应正文。 |
| 数据与导出 | 上游导入后保留原书源字段。 | SQLite、导入/导出、备份已经保留这些字段。 | `must-preserve`：本批不迁移、不清空、不重写书源 JSON。 |

## 兼容分析状态

前端需要一个纯函数分析器，返回稳定的结构而不是由模板重复扫描字符串：

```text
supported
  -> 默认勾选
  -> 无警告

unsupported-script
  -> 动态 Header、loginCheckJs、实际规则字段中的 @js:/<js>/{{…}}
  -> 不默认勾选
  -> 标签 @Javascript，显示具体但不回显内容的原因

unsupported-webview
  -> 任意实际 WebView 入口
  -> 不默认勾选
  -> 标签 @WebView

preserved-dormant
  -> 固定基准普通 HTTP 链未消费的 preUpdateJs/webJs/sourceRegex
  -> 不单独阻止默认勾选
  -> 编辑器说明字段仅无损保存
```

分析只检查已知字段/规则值，不能因为书源名称、注释里出现“JavaScript”或一段普通文本包含
`{{` 就把可运行书源误判为阻断。旧的序列化全文扫描必须被替换。

## API、数据与错误边界

- 不改变 `/api/sources*` 路径、请求体、响应字段或 HTTP 状态。
- 不改变 `book_sources` schema、默认书源快照、备份、导入/导出 JSON 和未知字段 round-trip。
- `source_rule_unsupported` 仍是后端唯一权威；前端分析仅用于提前告知，不能绕过后端检查。
- 用户手动导入阻断源后，保存和导出必须成功；搜索/探索/详情/目录/正文/debug 使用时仍在零网络请求下明确失败。
- 客户端提示只使用安全类别和阶段，不显示脚本正文或敏感请求配置。

## 实施前测试

1. `useSourceTransfer` 单元测试：
   - 静态 JSON Header、普通 CSS/JSONPath/XPath 规则默认勾选；
   - `@js:` Header、`<js>` Header、非空 `loginCheckJs`、规则 `@js:`/`<js>`/`{{…}}`、`webView:` 均不默认勾选；
   - `preUpdateJs/webJs/sourceRegex` 单独存在时只标记保留字段，不误判阻断；
   - 名称/注释中的相似文本不误判；全选仍只选择可运行项。
2. 静态组件合同：导入行显示兼容标签和安全原因；编辑器在阻断字段存在时持续显示警告；调试结果对 `source_rule_unsupported` 显示可读阶段说明。
3. 后端现有脚本入口合同保持：五条调用链零网络请求、零失效源缓存、API 不泄露敏感值。
4. 真实浏览器 1440×900、390×844、360×800：本地导入预览、手动选择/保存、重新编辑、debug 安全错误、关闭回到同一 Index 工作台，且无横向溢出和点击穿透。
5. 全量前后端测试与生产构建通过；本批不涉及持久化格式，因此 Docker 发布前仍按普通卷/备份门禁执行。

## 允许差异

- OpenReader 不在 Go 服务进程执行可访问文件、内网、cookie、缓存或用户凭据的任意脚本。
- Vue 3 可以用结构化警告和标签替代上游仅有的纯文本 tag，但导入预览、默认选择和无损保存顺序必须保留。
