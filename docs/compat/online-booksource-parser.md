# 在线书源解析兼容契约

状态：2026-07-13 已从固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 提取；P2-Parser-0 与 P2-Parser-1 的搜索/探索子集正在实现。本文件仍是目录、正文和剩余规则链重构的前置闸门。

当前已落地（尚未宣告整模块对齐）：统一的无脚本 CSS、JSONPath、XPath、正则基础求值器；搜索/探索、详情、目录、正文的基础列表/字段/分页调用链；分类多值与空详情 URL 回退；不执行 JS/WebJS 的明确错误；以及“解析错误不写入失效书源缓存”的错误边界。组合规则、变量、正文分叉顺序、普通文本空正文规则和结构化客户端错误仍未完成。

## 2026-07-13 P2-Parser-1B：详情、目录、正文调用链复审

本节是在 P2-Parser-0/1 搜索与探索子集之后的下一道实现闸门。上游语义仍以本文件开头列出的 `BookInfo`、`BookChapterList`、`BookContent` 和 `AnalyzeRule` 为基准；表中的“审查时 OpenReader”记录旧路径，避免把已经存在但未接入的 Go helper 误认为已完成。

| 流程 | 上游语义 | 当前 OpenReader 实际调用路径 | 判定与实施约束 |
| --- | --- | --- | --- |
| 详情 | `ruleBookInfo.init`、全部字段和分类列表均通过通用规则解释器；封面、目录链接以最终重定向后的详情 URL 解析。 | `parseRemoteBookInfoWithEvaluator` 已存在，但 `FetchBookInfoAndTOC` 仍抓取 `*goquery.Document` 并调用旧 `parseRemoteBookInfo`；因此 JSONPath/XPath/正则详情字段从未生效，分类仍只取首项。 | `must-fix`：详情请求改为 `sourceRuleDocument`，只有纯旧 CSS 书源保留旧快路径；分类必须逗号连接，`init` 无匹配时回退根作用域。 |
| 目录 URL | `tocUrl` 可以为空（详情页即目录）、直接 URL，或从详情文档用通用规则取值；解析后继续保留请求选项并使用最终 URL 作为相对地址基准。 | `parseTOCWithRule` 使用 `isDirectTOCURLRule + firstMatch`。`@XPath:`/JSONPath/正则不会取值；`@XPath:` 会落入 CSS `firstMatch` 后静默回退详情页。 | `must-fix`：只把明确 URL/请求选项当直接 URL；其余非空规则一律先在详情 `sourceRuleDocument` 求值，再决定是否请求另一页。空值仍复用详情页。 |
| 章节目录 | 章节列表、标题、URL、卷/VIP/更新时间、下一页均使用通用规则；去重与顺序遵循书源规则，分页链接顺序不能因为抓取策略改变。 | `parseChapterList`、`extractResolvedURLs`、`NextTOCURLRule` 仍使用 `findItems/Extract`，只支持 CSS。现有循环检测、1000 页上限、重定向 URL、请求选项、卷/VIP 与去重是可保留安全/运行时适配。 | `must-fix`：目录页面和分页全部接入统一执行器；保留上限、取消、重定向去重和请求头。JS `chapterPreUpdateJs` 不执行且必须产生明确错误，不能伪装成空目录。 |
| 正文 URL | `contentUrl` 是章节页上的通用取值规则，得到的 URL 才请求正文页；空规则使用章节页本身。 | `FetchChapterContentContext` 把任何非空 `ContentURLRule` 直接交给 URL 请求器，因而 XPath/JSONPath/正则会被当作 URL；没有先获取章节页供规则求值。 | `must-fix`：先抓章节 `sourceRuleDocument`，非直接 URL 规则在该文档求值；只有生成的 URL 与章节最终 URL 不同才进行第二次请求。音频空规则的“直接返回章节 URL”保留为已批准差异。 |
| 正文/下一页 | `content`、`nextContentUrl` 走通用规则。单链按链顺序；多链接的结果按规则返回顺序拼接，不能由并发完成顺序决定。空正文规则不应被静默替换成无关的 `body` 文本。 | `extractChapterContent`、`NextContentURLRule` 都是 CSS-only；当前队列分页与上游分叉顺序不同；普通书源空正文规则会回退 `body|text`。安全 HTML 与图片 URL 过滤属于允许的安全适配。 | `must-fix`：所有正文规则接入执行器，按解析出的 URL 顺序提交/拼接；保留取消、循环检测、1000 页上限及安全 HTML。普通文本空规则改为明确的空规则/内容错误边界，不能抓取整页噪声。 |
| 错误和失效缓存 | 规则不支持、规则语法错误和网络错误应可区分；只有真实远端请求失败可进入失效书源缓存。 | `ErrUnsupportedSourceRule` 与 `ErrSourceRequest` 已可区分，正常搜索缓存已按此过滤；详情/目录/正文迁移必须继续保留错误包装链。 | `must-fix`：黄金测试应断言 `errors.Is` 可穿透，API 不记录本地规则错误；结构化客户端错误另列 P2-Parser-2。 |

### P2-Parser-1B 实施记录

- `FetchBookInfoAndTOC`、`ParseTOC`、`FetchChapterContentContext` 已改为在需要时使用 `sourceRuleDocument`；旧 CSS 保留原快路径，JSONPath/XPath/正则与显式 `@CSS:` 走同一无脚本执行器。
- `tocUrl` 与 `contentUrl` 的非直接 URL 规则先在详情/章节响应上求值，再发起第二个请求；空值复用当前已抓取页面。协议相对 URL 仍被识别为直接 URL，而裸 `//a/@href` 进入 XPath 分支。
- 详情多分类、章节字段与正文/下一页 URL 已加入 JSONPath/XPath 黄金 fixture。JS 规则在详情、目录、正文三条路径都返回可由 `errors.Is(err, ErrUnsupportedSourceRule)` 识别的错误。
- 当前目录/正文分页仍按受限的串行队列抓取；它保持取消、重定向去重、请求头、1000 页上限和当前安全顺序，但尚未完成上游多分叉链接的严格返回顺序契约。

### 实施前测试清单

新增的 fixture 和测试必须在改动调用点之前覆盖以下组合：

1. HTML、JSONPath、XPath 三种详情：`init` 作用域、封面相对 URL、多分类、字数、目录链接。
2. HTML、JSONPath、XPath 三种目录：直接目录、详情页取目录、空目录链接回退、章节字段、下一页、反序、循环/去重和最终重定向基准。
3. HTML、JSONPath、XPath 三种正文：章节页取 `contentUrl`、同页正文、下一页单链/多链接及固定拼接顺序、相对图片和安全 HTML。
4. `@js:`/`<js>` 用于详情、目录和正文时均得到 `ErrUnsupportedSourceRule`，且 API 的失效书源列表保持不变。
5. 旧 CSS、直接 URL 与带 POST/headers/charset 请求选项的既有 fixture 全量通过，避免为了新规则回退已发布书源。

### 允许的差异

- Go 服务继续不执行 `preUpdateJs`、`webJs`、`sourceRegex`；字段无损保存，执行点返回明确不支持错误。
- 远端抓取继续使用当前超时、响应大小、重定向、限速、上下文取消与 1000 页上限；这些安全边界优先于上游的无限脚本/分页能力。
- 正文 HTML 继续做图片 URL 校验与主动内容清理；这是浏览器安全适配，不改变文本与安全图片的可读顺序。

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
