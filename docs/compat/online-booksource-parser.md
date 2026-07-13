# 在线书源解析兼容契约

状态：2026-07-13 已从固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 提取；尚未开始实现。本文件是下一批在线书源重构的前置闸门。

## 审查范围与上游证据

上游并非把书源规则当作单一 CSS selector。其通用解释器由下列文件共同定义，并被搜索、详情、目录和正文共用：

- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeRule.kt`
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeByJSoup.kt`
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeByXPath.kt`
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeByJSonPath.kt`
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeByRegex.kt`
- `src/main/java/io/legado/app/model/webBook/BookList.kt`
- `src/main/java/io/legado/app/model/webBook/BookInfo.kt`
- `src/main/java/io/legado/app/model/webBook/BookChapterList.kt`
- `src/main/java/io/legado/app/model/webBook/BookContent.kt`

上游请求与失败缓存入口仍以 `BookController.kt#searchBookWithSource` 为准；请求 URL 规则由 `AnalyzeUrl.kt` 解释。

当前 OpenReader 对应文件是：

- `backend/engine/source_parser.go`
- `backend/engine/parser.go`
- `backend/engine/source_request.go`
- `backend/api/sources.go`
- `backend/models/models.go`

## 上游解释器契约

| 层 | reader-dev 行为 | OpenReader 当前行为 | 判定 |
| --- | --- | --- | --- |
| 规则模式 | `AnalyzeRule` 会按规则和内容识别 CSS/Jsoup、JSONPath、XPath、正则和 JS；同一字段可由多段规则连续转换。 | `Extract` 只接受 `CSS selector\|text/html/attr:name`；`findItems` 只取第一个 `\|` 前的 CSS selector。 | `must-fix` |
| 规则前缀 | 支持 `@CSS:`、`@XPath:`、`@Json:`、`$.`/`$[`、`//`、正则 `:` 以及 JS 片段。 | 简单 `selector@text/attr` 会在导入时变为当前 CSS 语法；其余前缀大多原样保存后静默按 CSS 执行，通常得到空结果。 | `must-fix` |
| 规则组合 | `&&`、`||`、`##regex##replacement`、捕获组 `$1`、`@put:{...}`、`@get:` 和 `{{...}}` 均在解析器中按上下文处理，且不会误切 XPath/JSONPath 过滤表达式内部的 `&&/||`。 | 只对正文 `ContentReplaceRegex` 实现了受限 `##` 分割；一般字段没有规则链、变量或安全的语法诊断。 | `must-fix` |
| URL 规则 | `getString(..., isUrl=true)` 用最终重定向地址解析相对链接；空 URL 字段回退页面 base URL。 | `prepareSourceRequest` 已覆盖 URL 选项、GET/POST、请求头、编码、重试、`{keyword}/{page}` 与最终响应 URL；但字段取值仍受简化 selector 限制，搜索结果空 `bookUrl` 会被直接丢弃。 | URL 请求层为 `technical-stack-equivalent`；字段 URL 回退为 `must-fix` |
| 搜索/探索 | `BookList` 先按通用规则取书目；空书目且无详情 URL pattern 时再按详情解析。每本书若 URL 为空，回退当前页面 URL。分类取完整字符串列表并以逗号连接，简介经 HTML 格式化。 | CSS 书目/详情回退已有；`BookURL` 为空的项目会被跳过，分类只取第一项，简介/名称/作者的上游格式化语义未完整复刻。 | `must-fix` |
| 详情 | `BookInfo` 的 `init` 是通用 `getElement` 规则；分类是列表连接；`canReName` 同时取决于调用意图和规则结果。封面/目录 URL 使用重定向后的地址。 | `bookInfoScope` 仅把非 `@` 的简单 CSS 作为 scope；分类只取一项，`canRename` 仅检查规则是否非空。 | `must-fix` |
| 目录 | `BookChapterList` 通过通用规则解析目录、卷/VIP/更新时间与下一页；目录 URL 可由详情规则、直接 URL 或取值规则得到。 | 已有直接/取值 TOC URL、最终响应地址、去重、固定 1000 页上限、取消边界和卷/VIP；但 `chapterPreUpdateJsRule` 与非 CSS 的目录规则未执行。 | CSS/分页安全边界为 `acceptable-change`；通用规则为 `must-fix` |
| 正文 | `BookContent` 按通用规则提取正文/下一页；单下一页按链顺序，多下一页并发后仍按规则返回顺序拼接；再执行通用替换规则，保留图片 HTML。 | 已有可取消、去重、固定 1000 页、相对图片 URL 与安全 HTML 处理；正文/下一页只使用 CSS，队列分页对分叉下一页的拼接顺序与上游不同，空正文规则会回退 `body\|text`。 | 安全 HTML、页数上限、取消为 `acceptable-change`；提取/顺序/空规则为 `must-fix` |
| JS/WebJS | 上游 JS 能访问书籍、章节、变量、cookie、缓存及网络请求。 | 模型和导入/导出保留 `preUpdateJs`、`webJs`、`sourceRegex`，但运行时不执行也不明确报错。 | 不允许把不受限用户 JS 放进 Go 服务进程。作为安全适配，必须保留原字段、在使用时返回明确“该书源规则暂不支持”的结构化错误，并在后续单独评估受限沙箱；不得静默返回空列表/空正文。 |

## 已确认的结构性根因

1. `backend/api/sources.go#normalizeUpstreamSelectorRule` 只把简单 `selector@text/attr` 改为当前语法。它保留复杂规则，但 `backend/engine/parser.go#Extract` 无法解释这些规则。
2. 导入/导出没有丢弃 `ruleToc.preUpdateJs`、`ruleContent.webJs/sourceRegex` 等字段，因此 UI 显示“已导入”并不等于该书源可运行。
3. `source_parser.go#parseBookResults` 要求 `Title` 和 `BookURL` 同时非空；上游对于空 `bookUrl` 会使用当前页面 URL。这会让部分合法详情式书源的搜索结果消失。
4. `bookInfoScope`、`firstMatch` 与 `findItems` 把上游的通用规则降格为第一个 CSS 值，导致 JSON API 书源、XPath 书源和多段规则不能进入后续详情/目录/正文流程。
5. 当前错误常表现为“无搜索结果”“无目录”或阅读页空白，未区分“规则无匹配”“规则语法不支持”“远端请求失败”。上一批的失效书源缓存只能抑制真实请求失败，不能掩盖解释器不兼容。

## 保留的 OpenReader 适配

- Go 请求器的超时、响应大小、重定向、并发率、分页上限、上下文取消、JWT 用户隔离和失效源缓存继续保留。
- 最终重定向 URL 解析、GET/POST/JSON/form body、字符集和请求选项已有契约与测试，解释器重建不得回退这些能力。
- 书源 JSON 的未知/JS 规则必须无损保存和导出；不受限执行 JS、`webJs` 或可访问宿主网络/文件的脚本不是可接受的兼容实现。
- 正文 HTML 继续经过安全化，图片只允许安全 URL；这比上游宽松输出更严格，属于用户数据与浏览器安全适配。

## 实施顺序与测试闸门

### P2-Parser-0：先建立黄金夹具（不得先改线上行为）

在 `backend/engine/testdata/source_compat/` 加入不联网 fixture，并让每个 fixture 同时写明上游期望：

1. CSS：`@CSS:`、`@text`、`@html`、`@href`、当前节点/子节点、多个分类值。
2. JSON：JSONPath 搜索、详情、目录、正文、数组字段、相对 URL。
3. XPath：元素列表、文本、属性、目录/下一页。
4. 正则：`:...` 列表/捕获组、`##` 替换、无匹配和非法正则。
5. 组合：`&&`、`||` 位于 XPath/JSON 过滤表达式、变量读写、空 URL 回退。
6. 目录和正文分页：单链、多分叉链接、最终拼接顺序、循环、页数上限、取消。
7. JS/WebJS：字段能够 round-trip；执行请求必须得到明确、安全且可定位的“不支持”错误，绝不能伪装为空结果。

黄金测试必须覆盖同一个规则分别用于搜索、详情、目录、正文，防止只在单一路径实现。

### P2-Parser-1：重建无脚本规则解释器

新增内部、不可导出的规则 AST/执行器，输入为 HTML/JSON/文本与 `book/chapter/redirectUrl` 上下文，输出为节点列表、字符串列表或单字符串。先支持 CSS、JSONPath、XPath、正则与安全的 `##`/变量替换；不允许继续以 `strings.Split("|")` 解释上游规则。

所有调用点（搜索、探索、详情、TOC、正文、调试）必须迁移到同一执行器。旧的简单 CSS 规则和已有数据库书源不需要迁移。

### P2-Parser-2：语义恢复与错误边界

- 搜索结果空 URL 回退页面 URL；分类保留所有项；详情 `init/canReName` 按上游状态恢复。
- 目录/正文按上游链接顺序拼接；保留当前循环检测、1000 页上限和取消。
- 将“请求失败”“规则不支持”“规则语法错误”“规则无匹配”映射为不同的客户端安全错误；仅真实远端失败进入 `source_failures`。
- 调试接口显示规则阶段和安全错误类别，但不泄露 headers、cookies、令牌、完整敏感 URL 或响应正文。

### P2-Parser-3：受限脚本决策

除非能提供与 Go 进程、文件系统、内网和用户凭据隔离的受限运行时、超时/内存/网络白名单及回归测试，否则 `preUpdateJs/webJs/sourceRegex` 维持“无损保存 + 明确不支持”。这项安全差异必须在书源编辑器、导入报告和调试结果中可见，不能伪装成上游完全兼容。

## 完成标准

本模块完成不以“当前测试全绿”为准，必须同时满足：

1. 每种规则模式都能用上游黄金 fixture 在搜索、详情、目录、正文得到相同字段和顺序。
2. 旧 CSS 书源、带 URL 请求选项的书源、用户已有 SQLite 书源和导入/导出 round-trip 均不回归。
3. 真实浏览器完成“搜索 → BookInfo → 目录 → 正文”流程，并分别验证 CSS/JSON/XPath fixture 源。
4. 失败源缓存只记录远端请求错误，不因不支持规则、语法错误或空结果错误封禁书源。
5. 后端全量测试、前端测试/构建、Docker 卷备份门禁均通过后，才可发布 Docker。
