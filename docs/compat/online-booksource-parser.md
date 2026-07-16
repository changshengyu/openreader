# 在线书源解析兼容契约

状态：2026-07-13 已从固定上游 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 提取；P2-Parser-0 与 P2-Parser-1 的搜索/探索子集正在实现。本文件仍是目录、正文和剩余规则链重构的前置闸门。

当前已落地（尚未宣告整模块对齐）：统一的无脚本 CSS、JSONPath、XPath、正则基础求值器和规则级 `##` 替换；搜索/探索、详情、目录、正文的基础列表/字段/分页调用链；分类多值与空详情 URL 回退；受限的 `@put`/`@get` 变量及其搜索→书架→目录→正文的持久状态；不执行 JS/WebJS/`{{...}}` 模板的明确错误；以及“解析错误不写入失效书源缓存”、跨章节正文分页边界、空文本正文规则和结构化安全错误边界。上游 JavaScript/模板及其隔离执行策略仍未完成。

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
- 目录/正文分页已由后续 P2-Parser-1C 改为显式状态机：单链接继续跟随；多个初始链接严格按规则返回顺序抓取一级页面，并禁止展开分叉页面的子链接。Go 实现为串行抓取，替代上游协程并发，但请求和拼接顺序保持一致；保留取消、重定向去重、请求头与 1000 页安全上限。

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

## 2026-07-13 P2-Parser-1C：上游规则组合与分叉分页复审

本节已直接复核新的固定上游副本 `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`：

- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeRule.kt`
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeByJSoup.kt`
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeByJSonPath.kt`
- `src/main/java/io/legado/app/model/analyzeRule/AnalyzeByXPath.kt`
- `src/main/java/io/legado/app/model/webBook/BookChapterList.kt`
- `src/main/java/io/legado/app/model/webBook/BookContent.kt`

| 行为 | 上游确切实现 | 当前 OpenReader | 判定 / 下一步 |
| --- | --- | --- | --- |
| 单一下一页 | `BookChapterList` 与 `BookContent` 在初页只得到一个 next URL 时使用 `while` 串行跟随；循环由已访问 URL 列表停止。 | 当前队列能得到相同常见结果，并额外保留 1000 页、取消、重定向去重。 | `acceptable-change`：保留安全上限和取消，但改为显式单链分支，保证不与多分叉混用。 |
| 多个下一页 | 上游在初页 next URL 数量大于 1 时并发请求每个一级 URL，按原规则数组顺序 `await` 拼接；对子页调用关闭 `getNextUrl`/`printLog`，因此不再递归子链接。 | 当前 FIFO 队列会继续抓取每个分叉页的 next URL，既可能多读页面，又会输出 `首页 → 分叉 A → 分叉 B → A 的子页`，与上游不同。 | `must-fix`：目录/正文建立“单链递归 vs 首层多分叉”状态机；可保持串行抓取作为资源安全适配，但结果和请求顺序必须是上游规则顺序，且多分叉不继续展开。 |
| 章节边界 | 上游正文单链遇到下一章节 URL 时停止，避免章节分页跳进下一章。 | 当前 `FetchChapterContent` 没有下章 URL 参数，无法执行同一比较。 | `known Go API gap`：保留 URL 去重与页数上限；后续在缓存/章节上下文调用处传入 next chapter URL，再增加边界测试。 |
| `&&` / `||` / `%%` | JSoup、JSONPath、XPath 的 `RuleAnalyzer` 在嵌套/过滤语法外拆分：`&&` 合并，`||` 首个非空回退，`%%` 按索引交错合并。JSONPath 使用平衡代码组，避免把过滤表达式中的 `&&`/`||` 错切。 | 当前执行器只解释一个 CSS/JSONPath/XPath/正则表达式，错误地把多数组合交给底层解析器。 | `must-fix`：先为 CSS/XPath/JSONPath 组合写黄金测试和安全分割器；JSONPath 过滤式内部的逻辑运算不得切开。 |
| 正则与替换 | 上游 all-in-one `:regex` 提供捕获组，后续 `$1..$n`、`##pattern##replacement[##first]` 在同一 `SourceRule` 阶段生效。 | 当前 `:regex` 与单次 `$n` 有基础支持；常规字段的 `##` 只在正文替换的旧专用路径实现。 | `must-fix`：把受限 RE2 的捕获与替换并入统一执行器；非法正则明确报错，不能静默按 CSS。 |
| `@put` / `@get` / `{{ }}` | 上游以书籍/章节变量保存、读取和执行 JS 表达式；JS 可访问网络、cookie、缓存和本地对象。 | P2-Parser-1C 审查时没有上下文变量，也不执行 JS。 | `split`：`@put/@get` 已在后续 1F 作为无脚本、请求级变量实现；`{{ }}` 继续作为 `ErrUnsupportedSourceRule`，除非获得隔离沙箱。 |

### P2-Parser-1C 先行测试

1. 目录/正文：初页有两个 next，第一分叉又声明子页；结果只能包含初页和两个一级分叉，且请求/拼接顺序稳定。
2. 单链分页仍会完整跟随到末页，循环和 1000 页上限保持。
3. CSS、XPath、JSONPath 的 `||` 回退与 `&&` 合并；JSONPath 过滤表达式内的 `&&` / `||` 不被错误分割。
4. 正则捕获、全局替换、只替换首项与非法规则错误。
5. `@put/@get` 与 `{{ }}` 不在本小批实现；必须有明确的不支持测试，不能产生空结果或错误失效缓存。

### P2-Parser-1C 实施记录

- 目录与正文已不再使用 FIFO 递归队列：初页返回一个 URL 时继续单链；初页返回多个 URL 时按规则原顺序抓取每个一级页并禁止继续展开其 next 链接。
- 为了保持现有的 Go 请求限速、取消与资源上限，多分叉目前串行抓取而非上游协程并发；输出和请求顺序与上游 `await` 顺序一致，因此这是允许的运行时安全适配。
- 重定向后的 URL 仍参与去重，单链循环与 1000 页上限保持。下章 URL 边界和变量继续留在后续子批。
- `&&`、`||`、`%%` 已由统一执行器在顶层组合：分别合并非空结果、选择首个非空结果、按索引交错非空结果。分割器会跟踪引号、方括号、圆括号和花括号；JSONPath/XPath 过滤表达式内部的逻辑操作符不会被当作规则边界。CSS、XPath 和 JSONPath 的前缀会传播到同一组合中的简写后续分段，避免退化为 CSS。
- 新增黄金测试覆盖 CSS/XPath/JSONPath 的回退、合并、交错以及 JSONPath 嵌套 `&&` 的分割保护。规则级 `##` 替换、`@put/@get` 变量和 `{{ }}` 的安全诊断仍在下一子批。

### 2026-07-13 P2-Parser-1C 复核记录

本轮重新读取固定上游的 `BookChapterList.analyzeChapterList` 与 `BookContent.analyzeContent`，并逐项核对当前 `parseTOCWithRule`、`fetchChapterContentContextWithNextChapterRuntime` 和 `source_pagination_test.go`。此前 P2-Parser-1B 的“仍未完成”表述已被本节实施记录覆盖，不能再作为新的重构缺口。

| 分页状态 | 上游固定行为 | 当前 OpenReader | 结论 |
| --- | --- | --- | --- |
| 初页恰有一个下一页链接 | `while` 跟随下一页，直到空链接或已访问链接。 | 单链接分支持续解析下一页；请求 URL 和最终重定向 URL 均进入 visited 集合。 | 对齐；额外保留取消与 1000 页上限。 |
| 初页有多个下一页链接 | 依次 `await` 每个初始分叉的并发任务；子页调用关闭下一页解析，故不递归展开。 | 按规则数组顺序串行抓取每个一级分叉，`includeNext=false`，不读取分叉的子链接。 | 对齐的安全运行时适配：不使用上游并发，但可见内容与请求顺序相同。 |
| 正文越过章节边界 | 仅单链接路径中，将下一页绝对 URL 与下一章节 URL 比较，相同即停止。 | 仅单链接分支调用 `contentNextURLIsCatalogChapter`；多分叉保持只取一级的上游语义。 | 对齐。 |

`TestParseTOCOnlyUsesFirstLevelWhenRuleReturnsMultipleNextPages` 与 `TestFetchChapterContentOnlyUsesFirstLevelWhenRuleReturnsMultipleNextPages` 分别验证目录和正文只请求 `root → A → B`、不访问 `A` 的子页，并断言输出顺序；相关单链、循环、章节边界测试同在 `source_pagination_test.go`。因此本项不修改生产代码，也不重新发布 Docker；下一轮应审查真实尚未验证的上游差距，而不是重复实现本状态机。

## 2026-07-13 P2-Parser-1D：规则级替换与变量安全复审

上游 `AnalyzeRule.SourceRule` 在每一段规则求值后才执行 `##`：第一段为取值规则，第二段是正则，第三段是替换文本，存在第四段即仅替换首个匹配。规则主体为空时仍可将替换作用于当前输入；捕获组 `$1..$n` 同样先取值再替换。非法正则应当是可定位的规则错误，不应退化为普通字符串替换。

`@put:{...}`、`@get:{...}` 与 `{{...}}` 在上游绑定书籍或章节的可变状态，并可进入 JS 引擎；其中 JS 能访问请求、cookie、缓存和本地对象。OpenReader 不得在 Go 服务进程内直接实现这种权限模型。P2-Parser-1D 当时的安全契约是把三类语法都拒绝，而不是当 CSS 解析后默默为空；后续 1F 已在独立生命周期、隔离和多用户测试下接入前两者的受限请求级版本。

### P2-Parser-1D 测试闸门

1. CSS、JSONPath、XPath 和正则捕获结果都能执行全局 `##pattern##replacement` 与首个替换标记。
2. 规则主体为空时替换当前文本；无匹配时保持原文本；无效 RE2 正则返回可由 `errors.Is(err, ErrInvalidSourceRule)` 区分的规则错误，且与“不支持”错误不同。
3. P2-Parser-1D 时，`@put:`、`@get:` 与 `{{ }}` 分别返回明确不支持错误，且不能触发远端请求失败缓存；此条已由 1F 的受限变量契约替代。
4. 旧 `ContentReplaceRegex` 行为保持，直到其单独迁移至同一 helper 并通过正文 fixture。

### P2-Parser-1D 实施记录

- 统一执行器现在先解析并校验 `##` 尾部变换，再求值 CSS/JSONPath/XPath/正则，最后对每个字符串结果单独应用 RE2 全局替换或首个替换。搜索、详情、目录和正文已用现有上游 fixture 做真实调用链验证。
- 空规则主体可变换当前字符串；捕获组 `$1..$n` 会先取值再变换。首个替换没有匹配时保留原字符串，而不是上游 Android 实现的空字符串，这是为避免书名、章节标题和 URL 因无害规则失配消失的显式可用性适配。
- 新增 `ErrInvalidSourceRule` 区分错误正则与 `ErrUnsupportedSourceRule`。P2-Parser-1D 时，`@put:`、`@get:`、`{{ }}` 统一返回后者，既不执行脚本，也不作为 CSS 静默解析；P2-Parser-1F 已替代前两者为有界请求级实现，模板仍返回后者。
- `/api/sources/:id/test*` 维持原有的成功响应形状，但本地无效/不支持规则不再写入 `source_failures`；只有远端请求错误可以让书源被短暂抑制。

## 2026-07-13 P2-Parser-1E：正文跨章节与空规则复审

上游 `BookContent.analyzeContent` 在初页只解析到一个 `nextContentUrl` 时，先将该 URL 按正文最终重定向 URL 绝对化，再与目录中 `chapter.index + 1` 的 URL 比较；相同即停止，避免将下一章误拼进当前章节。初页返回多个 next URL 时，仍只抓取一级分叉，不做此单链扩展。

当前 `extractChapterContent*` 在普通书源的 `contentRule` 为空时回退 `body|text`，这与上游 `getString("") == ""` 不同，也会把导航、广告等整页噪声缓存为正文。用户要求的安全/可用性适配是：非音频书源把空正文规则作为 `ErrInvalidSourceRule` 返回；音频书源的既有“空规则直接返回章节资源 URL”能力保留。

| 行为 | 上游 | OpenReader 审查时 | 实施契约 |
| --- | --- | --- | --- |
| 单链下一页边界 | 下一页绝对 URL 与下一章 URL 相同即停止。 | engine 带可选 `nextChapterURL` 入口，API 在远程缓存未命中时按同一 `book_id,index+1` 查询并传入。 | `complete`：正文最终 URL 为相对地址基准，下一页和下一章均规范化后比较；旧入口保持无目录上下文的兼容行为。 |
| 多分叉正文 | 抓取初页声明的一级 URL，禁止继续展开。 | 已按上游顺序串行实现。 | `complete`：不得因新增边界逻辑改变多分叉结果。 |
| 空正文规则 | 上游返回空字符串。 | 普通文本返回 `ErrInvalidSourceRule`；音频保留章节资源 URL。 | `complete`：不请求/缓存整页噪声，也不进入失效书源缓存；音频例外为用户已批准的可用性适配。 |
| 客户端错误 | 上游 UI 直接显示调试/规则结果。 | 阅读接口统一映射为 `502 failed to load chapter content`。 | `deferred`：当前先保留稳定响应，后续 P2-Parser-2 增加安全的结构化错误类型。 |

### P2-Parser-1E 测试闸门

1. engine：下一页等于下一章 URL 时只返回当前页且不发起下一章请求；下一章 URL 为空或不同仍按单链规则继续。
2. engine：多 next URL 的一级分叉输出和请求顺序不变，即使其中一个与下一章 URL 相同。
3. API：远程章节缓存未命中时传递相邻章节 URL；缓存命中、最后一章、局部书籍和音频不产生额外远程请求或数据库副作用。
4. 非音频空 `contentRule` 返回 `ErrInvalidSourceRule`，不写缓存、不进入失效书源缓存；音频空规则现有测试继续通过。

### P2-Parser-1E 实施记录

- `FetchChapterContentContextWithNextChapter` 保持旧公开入口兼容，并让阅读 API 在远程缓存未命中时传入相邻章节 URL。比较时会以正文最终请求 URL 为基准解析相对地址，随后连同请求选项规范化，避免相对目录 URL 失配。
- 单链正文在请求下一页前停止；多分叉仍按上游一级分叉规则处理，新增边界判断不会抑制分叉中的同名 URL。
- 非音频空 `contentRule` 在任何远端请求前返回 `ErrInvalidSourceRule`，因此既不会把整个 HTML 页面写入章节缓存，也不会把本地规则错误写入 `source_failures`。音频源空规则仍返回已解析的章节媒体 URL。
- 新增 engine 与 API 契约测试，覆盖相对的下一章节 URL、缓存未命中目录传递、分叉不变、空规则无缓存/无失效源记录及音频回归。

## 2026-07-13 P2-Parser-1F：变量规则与结构化错误复审

本轮上游证据来自固定基准的 `AnalyzeRule.kt`、`AnalyzeUrl.kt`、`RuleDataInterface.kt`、`RuleData.kt`、`Book.kt` 与 `BookChapter.kt`。结论是 `@put:`/`@get:` 与 `{{...}}` 不能视为同一项能力：前者是可在无脚本解释器中收敛的规则变量，后者能调用上游 JavaScript 运行时并获得 cookie、缓存、书源、书籍、章节及网络访问能力。

| 行为 | 上游确切语义 | 当前 OpenReader | 判定 / 后续边界 |
| --- | --- | --- | --- |
| `@put:{...}` | 每个 `SourceRule` 在选择器求值前取出 JSON object；每个 value 再通过 `getString` 作为规则求值后写入变量。`AnalyzeRule` 写入优先级为 chapter → book → 临时 `RuleData`。 | 已实现受限请求级 map：仅 JSON object、字符串键和值规则；value 仍经过无脚本 CSS/JSONPath/XPath/正则求值。键按字典序处理，不依赖上游无顺序 map 的偶然顺序。 | `aligned for request scope`：值、键、总字节、数量和嵌套深度均受限；不执行 JS、不访问 cookie、文件或网络。 |
| `@get:{key}` | 可嵌入规则字符串；读取 `bookName`、`title` 特殊值，随后按 chapter → book → `RuleData` 查找，缺失为空字符串。 | 已在同一顶层解析操作内替换受限 map 的键；缺失值为空字符串，替换结果作为字面量而不被二次执行为选择器或 URL 规则。 | `acceptable security difference`：尚未注入 `bookName`/`title` 特殊值，避免把未审查的模型数据暴露进请求规则；绝不从 HTTP header、cookie、JWT、WebDAV 凭证或环境变量取值。 |
| 变量生存期 | `RuleData` 是一次调用内存 map；`Book.variable` 与 `BookChapter.variable` 是序列化 JSON，模型保存后可跨请求保留。上游 map 无顺序合同，不能依赖同一 `@put` object 内键的写入顺序。 | 搜索、详情→目录、正文单链各自创建请求级 map；搜索结果和正文多分叉各自克隆 map，map 不进缓存、数据库或 API 响应。 | `split`：跨请求 book/chapter 变量仍需要单独的 P2-Parser-1G 数据契约、迁移、用户隔离、删除/备份/导出策略和冲突测试，不能暗中加入。 |
| `{{...}}` | 若内容是规则，上游继续递归解析；否则交给 JS 引擎。URL 模板也能执行 JS；绑定包含 cookie、cache、source、book、chapter、result，且 JS 扩展可发网络请求。 | 识别并返回 `ErrUnsupportedSourceRule`。 | `acceptable security difference`：保持明确不支持，不把它降格为空字符串或 CSS。任何未来支持须有隔离运行时、时间/内存/网络白名单、无宿主文件访问和多用户秘密隔离；它不属于 1F。 |
| 错误响应 | 上游 UI 可显示调试链；规则与请求异常并不以 Go REST 为边界。 | 阅读接口稳定返回 `{error:"failed to load chapter content"}`；其他入口有混合的 400/502 原文错误。 | `deferred P2-Parser-2`：先保持部署客户端的状态码和字段；增加不泄露 URL query/header/cookie 的 `code`/`stage` 前，必须写 API 契约和端到端测试。 |

### P2-Parser-1F 推荐运行时与测试闸门

1. 新增不可导出的 `sourceRuleRuntime`，生命周期只能由单次顶层操作创建并显式传递；搜索的并行书源任务、不同用户、不同 HTTP 请求和章节缓存任务绝不共用 map。
2. `@put:` 只接受大小受限的 JSON object：键和值规则数量、键长度、值长度、总字节数和递归深度都必须有上限；空/非字符串值、坏 JSON、循环/过深规则返回 `ErrInvalidSourceRule`，而不是远端失败。
3. fixture 分别覆盖 HTML、JSONPath、XPath 与正则 value 规则，`@put` 后的 `@get`、缺失键、特殊 `bookName`/`title`、正文单链与多分叉的 map 隔离，以及并发两个书源/两个用户的无泄漏。
4. 在详情→目录及正文分页中验证同一操作的变量可用；缓存命中、重新进入阅读页、另一个章节、刷新书源和 source-debug 的独立步骤不得意外复用临时 map。
5. `/api/sources/:id/test*`、搜索、详情、目录、正文遇到变量语法/大小/递归错误时都不写 `source_failures`；日志、debug 响应与结构化错误不得回显变量值、URL query、cookie 或授权 header。
6. P2-Parser-1G 开始前，必须由 `data-migration-compat` 单独审查 book/chapter 持久变量：旧 SQLite、备份恢复、书籍删除、source change、章节刷新、导入导出、缓存键和用户所有权均需有测试。未经这道闸门，1F 的 map 不落库。

### P2-Parser-1F 实施记录

- 新增不可导出的 `sourceRuleRuntime`，每个顶层解析操作新建；变量最多 32 项，单个键最多 128 字节、单个值最多 4096 字节、总量最多 16 KiB、嵌套最多 8 层。坏 JSON、非字符串值、超限和递归深度都会返回 `ErrInvalidSourceRule`。
- `@put:` 只接受 JSON object；每个 value 通过既有无脚本规则解释器取值。`@get:` 缺失时为空，命中时作为字面量返回，不能把变量内容重新解释成选择器、JavaScript 或请求 URL。
- 搜索中每个结果克隆 runtime，独立搜索操作绝不继承变量；详情与目录共享同一 runtime，正文单链分页共享同一 runtime，多分叉正文各自克隆 runtime。这是对上游可变对象并发行为的多用户安全适配。
- `{{...}}` 与 JavaScript 仍然明确返回 `ErrUnsupportedSourceRule`；变量值不落库、不进入章节缓存键、不写入调试响应或失效书源缓存。
- 新增 `source_rule_variables_contract_test.go`，覆盖结果/操作隔离、详情→目录、正文单链、缺失键、坏 JSON、非字符串值、超长键和模板禁用；API source-debug 回归继续验证本地变量规则错误不会写入 `source_failures`。

## 2026-07-13 P2-Parser-1G：持久变量与结构化错误审查、实施与发布记录

本节重新从固定上游 `fa22f271849d45f93349ae1636223e27b16a4691` 提取证据，使用 `AnalyzeRule.kt`、`AnalyzeUrl.kt`、`RuleData.kt`、`RuleDataInterface.kt`、`Book.kt`、`BookChapter.kt`、`SearchBook.kt`、`BookList.kt`、`BookChapterList.kt`、`BookContent.kt` 与 `WebBook.kt`。下表保留实施前证据；其差距已由随后 `P2-Parser-1G 实施记录` 覆盖，并随提交 `a45053a` 本地构建、推送和发布。

| 范围 | 上游确切语义 | 当前 OpenReader | 判定 |
| --- | --- | --- | --- |
| 搜索变量 | `WebBook.searchBook` 先以临时 `SearchBook` 承接 URL/列表级变量；`BookList` 为每项建立独立 `SearchBook(variable = variableBook.variable)`，再把该 JSON 字符串复制到 `Book`。 | `SearchResult` 只有可见书籍字段；请求级 runtime 在搜索响应结束即丢弃。 | `must-fix`：可选 `variable` 必须随搜索结果并在“加入书架”请求中显式传递；结果之间不得共享写入。 |
| 书籍变量 | `Book.variable` 是 JSON 字符串；`AnalyzeRule.put/get` 的优先级为章节 → 书籍 → 临时 `RuleData`，`bookName` 是书名特殊值。详情、目录、正文的同一 `Book` 会持续携带其变量。 | `models.Book` 没有变量列；详情→目录只在一次请求的 map 内共享，后续刷新/阅读会重新开始。 | `must-fix`：增加可为空的 `books.variable` 文本列，作为受限变量 JSON 的唯一持久来源；旧空列必须等价为空 map。 |
| 章节变量 | `BookChapter.variable` 是 JSON 字符串。目录逐项新建章节后，字段规则可写入该章节；正文读取/写入该章节，`title` 是章节特殊值。 | `models.Chapter` 没有变量列，正文没有章节变量输入/输出。 | `must-fix`：增加可为空的 `chapters.variable` 文本列；刷新目录时在同一事务中替换章节变量，正文只更新当前书籍下的当前章节。 |
| 变量写入 | 上游 map 未限制大小、并发分叉共享可变 `Book`。 | 1F 已有 32 项、128/4096 字节、16 KiB、8 层的受限 runtime；分叉克隆 map。 | `acceptable security difference`：持久化只能接受并输出同一受限 JSON map，保持单链共享、分叉克隆；不复制上游无界/并发共享写入。 |
| 生命周期与切换 | 上游 `Book` 的变量会保留；来源切换的实际对象更新不提供跨来源隔离。 | 多用户 `Book` 以 `user_id` 所有；来源可能变更，备份/恢复按 URL 合并。 | `must-fix with security adaptation`：变量必须通过 `Book.UserID` 与 `Chapter.BookID` 间接隔离；来源 ID 改变、书源 URL 变更或规则集变更时清空该书变量与章节变量，避免把旧来源令牌带入新来源。 |
| 备份/恢复 | 上游 Book/BookChapter JSON 含 `variable`；SearchBook 序列化也保留 `variable`。 | `bookshelf.json` 由 `models.Book` 导出，但当前无此列；章节不在备份中。 | `must-fix`：`bookshelf.json` 增加可选 `variable`；新增可选 `chapterVariables.json`，按目标用户的书 URL + 章节 URL/index 恢复。旧备份缺失字段/文件必须保持可恢复。 |
| 错误反馈 | 上游是应用内异常/调试链，不存在 OpenReader REST 响应形状。 | P2-Parser-2A 已为正文、单书源分页搜索、探索、加书/刷新/换源和 source debug 实现稳定 `error`、可选 `code`/`stage`，并移除原始 error 序列化。 | `aligned security adaptation`：保留状态码和 `error` 字段；禁止回显规则字面量、变量值、URL query、cookie、headers、JWT、文件路径或响应正文。 |

### P2-Parser-1G/2 测试与实施门槛

1. 先为 `Book.Variable`、`Chapter.Variable` 写 SQLite 加列、空旧值、无效/过大 JSON、来源切换清空、用户隔离和回滚测试；不得对旧 `data/`、`cache/`、`library/` 做扫描或改写。
2. 搜索 fixture 必须证明每项从临时变量继承初值后独立写入；“搜索 → BookInfo → 加书 → 重开目录 → 正文”必须保留正确的书籍/章节变量，正文多分叉不相互写入。
3. 备份/恢复 fixture 必须同时覆盖新 `variable`、`chapterVariables.json`、旧 OpenReader/reader-dev 备份缺失字段、重复恢复幂等性和目标用户隔离。
4. P2-Parser-2A 已在不改 HTTP 状态码的前提下覆盖正文、单书源分页搜索、探索、加书/换源和 `/api/sources/:id/test*`；旧客户端继续只读 `error`，现代客户端可读取 `code`/`stage`。P2-Parser-1G 持久变量已经实施、全量验证并发布；后续脚本入口的差距由下节单独处理。
5. 任何变量或错误实现完成后都必须跑完整 Go/前端测试、真实浏览器书源流程和 Docker 挂载卷/备份烟测；本节未完成前不得称为持久变量对齐。

### P2-Parser-2A 实施记录

- `backend/api/source_errors.go` 将远端请求、无效规则、未支持规则和其他正文不可用错误映射为稳定 `code`；底层错误只保留给服务端的失效书源记录，绝不进入 JSON 响应。
- 正文、单书源分页搜索、探索、远端加书、书籍刷新、换源、三步 source debug 和批量测试均保留已有 HTTP 状态和顶层 `error`，并按需要附加 `stage`。因此部署中的旧客户端不需要改动。
- `backend/api/source_error_contract_test.go` 使用带用户名、密码和 query token 的模拟上游错误，证明这些值不会泄露到搜索、探索、debug 或远端加书响应；同时覆盖正文规则错误的 `source_rule_invalid` / `content`。
- 本项是 Go/JWT 多用户运行时的安全适配，不执行上游 JavaScript，也不改变仍待 P2-Parser-1G 完成的 `Book.variable` / `BookChapter.variable` 数据迁移。

### P2-Parser-1G 实施记录

- `models.Book.Variable` 和 `models.Chapter.Variable` 以 GORM 的纯加列迁移加入 SQLite；空值就是空 map。读写统一经过 `models.NormalizeSourceRuleVariables`，只允许 32 项以内、键 128 字节、值 4096 字节、合计 16 KiB 以内的 `map[string]string`。无效持久值在任何远端请求之前返回本地 `ErrInvalidSourceRule`，不会触发抓取或失效书源缓存。
- 搜索列表级临时变量在每个结果边界复制为独立 Book 变量；前端仅把不透明 JSON 字符串透传到既有“加入书架”请求。详情与目录更新同一 Book map；目录每一章获得独立 Chapter map；正文严格按 `chapter → book → temporary` 读取，并把成功结果与章节缓存元数据在同一数据库事务内写回。
- 刷新目录、切换来源、刷新本地书、编辑/导入/清空/恢复书源都会清空不再具有同一语义的远程变量。规则、基础 URL、请求头、编码、登录地址或来源类型的改变都在源配置写入的同一事务里清空关联 Book/Chapter map；本地书永远导出和恢复为空变量。
- 备份仍使用兼容的 `bookshelf.json`，新增可选 `sourceName` 和 `variable` 字段；同时新增 `chapterVariables.json`，按目标用户的来源名、书 URL/title、章节 URL/index/title 还原变量，绝不重用来源数据库 ID、章节 ID、缓存路径、请求头或凭证。恢复先验证所有新增 map，先恢复书源和书架，再在事务中恢复章节变量；旧备份缺失新字段时保持原有行为。
- 契约覆盖见 `backend/engine/source_rule_variables_contract_test.go`、`backend/api/persistent_source_variables_contract_test.go`、`backend/api/backup_restore_contract_test.go` 与 `frontend/tests/remoteBookResultVariableContract.test.mjs`：结果隔离、特殊变量优先级、目录/正文持久化、请求前拒绝坏值、来源语义切换清空和跨实例备份恢复都已验证。

### 2026-07-13 P2-Parser-3A：书源脚本入口复审、实施与验证记录

本轮重新核对固定上游的 `BaseSource.kt`、`AnalyzeUrl.kt`、`WebBook.kt`、`BookChapterList.kt`、`BookContent.kt` 与当前 `models.go`、`source_parser.go`。结论只依据实际调用链，不把字段名当成已运行的功能。

| 脚本入口 | 上游固定基准的实际行为 | 当前 OpenReader | 判定与下一步 |
| --- | --- | --- | --- |
| `header: "@js:…"` / `<js>…</js>` | `BaseSource.getHeaderMap()` 在每次请求前调用 `evalJS`，把返回 JSON 转为请求头；`AnalyzeUrl` 将其用于搜索、探索、详情、目录和正文请求。脚本能访问上游 cookie/cache/source 上下文。 | `models.parseBookSourceHeader()` 识别这两种前缀后直接返回 `nil`；请求照常发出但丢失动态头。 | `must-fix`：在任一远端请求前返回 `ErrUnsupportedSourceRule`，禁止发出半配置请求、禁止静默空结果，且不得记录为远端失效书源。静态 JSON header 保持不变。 |
| `loginCheckJs` | `WebBook.searchBook`、`exploreBook`、`getBookInfo`、`getChapterList` 都在收到响应后调用 `AnalyzeUrl.evalJS(checkJs, response)`；可将未登录响应转换为有效响应或抛出登录状态错误。 | 字段导入、导出、备份均保留，但 Go 没有上游的登录 cookie/session 或脚本运行时，正常书源流程从未调用。 | `must-fix security adaptation`：在相关搜索、探索、详情/目录和正文流程启动前返回明确不支持，避免把登录页误解析为书籍/目录/正文。原字段无损保存；不实现宿主可访问 JS。 |
| `ruleToc.preUpdateJs` | 固定上游中 `TocRule` 有此字段，但 `WebBook` → `BookChapterList` 的目录调用链没有引用它。 | 字段无损保存但未执行。 | `aligned-dormant`：本批不把一个上游未调用的字段改为失败，也不执行它；保留到后续发现实际 WebView/插件调用点时再立契约。 |
| `ruleContent.webJs`、URL 选项 `webJs` 与 `sourceRegex` | `WebBook.getBookContent` 把 `content.webJs/sourceRegex` 传给 `AnalyzeUrl.getStrResponseAwait`；但固定基准该方法当前普通 HTTP 分支未消费这些参数，`BookContent` 也未引用字段。URL 选项仅被解析到私有值，当前调用链未见消费。 | 字段无损保存但未执行。 | `aligned-dormant / unknown`：不可因字段存在就声称上游执行，也不可静默宣称兼容。后续若发现独立 WebView/插件调用点，再先建立隔离运行时契约；本批不改其行为。 |
| URL/字段规则中的 `@js:`、`<js>`、`{{…}}` | 上游在 `AnalyzeUrl`/`AnalyzeRule` 内执行，且可访问 book/chapter/cookie/cache/network。 | 统一求值器已返回 `ErrUnsupportedSourceRule`，不触发远端失效缓存，并由 API 映射为安全的 `source_rule_unsupported`。 | `implemented security difference`：保持显式拒绝。 |

本批的 API/测试契约：

1. `Header` 脚本或非空 `loginCheckJs` 必须在搜索、探索、详情/目录、正文和 source-debug 对应请求**前**失败；fixture transport 的请求计数必须为零。普通静态 header 的五条调用链仍各自发送且保持原有头覆盖/安全过滤。
2. 引擎错误必须满足 `errors.Is(err, ErrUnsupportedSourceRule)`；`recordSourceFailure` 与 `recordSourceHealthFailure` 均不得创建 `source_failures` 行。
3. 不改稳定路由、HTTP 状态或顶层 `error` 字段：既有 API 继续添加 `code: "source_rule_unsupported"` 和对应 `stage`；debug 保持 `200` 包络，常规书源路由沿用既有状态码。响应不能回显 JavaScript、header、cookie、URL query 或远端内容。
4. `preUpdateJs`、`content.webJs`、URL option `webJs`、`sourceRegex` 的上述“固定基准未消费”事实必须有回归说明；本批不凭猜测改变其无损导入/导出和普通静态书源行为。

允许差异是 Go/JWT 多用户服务端不运行能读取 cookie、缓存或网络的上游 JS。只有在脚本运行时与 Go 进程、文件系统、内网、用户凭据隔离，并具备 CPU/内存/超时/网络白名单和回归夹具后，才可重新评估执行支持。

实施结果：

- `backend/engine/source_parser.go` 新增唯一的 `ensureSourceScriptEntryPointsSupported` 闸门。搜索、探索、详情/目录和非空正文请求均在构造远端请求之前检查动态 `Header` 与 `loginCheckJs`，并返回 `ErrUnsupportedSourceRule`。没有搜索/探索 URL、文本空正文规则和音频空正文直链继续保留原有的无请求语义。
- 没有变更路由、请求体、SQLite schema、导入/导出字段或静态 header 合并。动态字段仍可无损保存、备份和导出；它们首次被使用时得到确定的安全错误，不会再伪装成请求失败或空结果。
- `backend/engine/source_script_entrypoints_contract_test.go` 覆盖 `@js:`、`<js>` 和 `loginCheckJs` 在搜索、探索、详情/目录、目录和正文五条引擎链路上均零网络请求；`backend/api/source_error_contract_test.go` 覆盖所有稳定 API 错误 stage、零请求、零失效源缓存和敏感脚本文本不回显。
- `backend/api/api_test.go` 的上游书源导入全链路 fixture 继续验证静态 `headerMap` 的搜索、探索、详情、目录、正文和加书流程；登录检查脚本不再被该静态路径误当成可执行依赖。

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

## 2026-07-16 P2-Parser-4：真实浏览器工作流验收矩阵

本节是对既有 engine/API 黄金测试的补充，不重写已经完成的解析器。固定上游在 `WebBook.searchBook`、`BookList`、`BookChapterList`、`BookContent` 中把同一书源规则连续用于搜索、详情、目录与正文；OpenReader 对应路径是 `POST /api/search`、`POST /api/reader/remote-sessions` 和 `GET /api/reader/remote-sessions/:id/chapters/:index/content`。此前浏览器 smoke 只拦截这些 API 并返回模拟数据，不能证明浏览器工作台实际消费了 Go 解析器的输出。

| 项目 | 上游合同 | 现有证据 | 判定 | 验收方式 |
| --- | --- | --- | --- | --- |
| CSS 书源 | 搜索结果可进入详情/目录，再显示章节正文；相对 URL 以响应 URL 解析。 | engine fixture 与真实浏览器/API 流程均已覆盖。 | `verified` | 本地 HTML fixture：搜索、BookInfo、远程目录会话、目录面板、正文。 |
| JSONPath 书源 | JSON 搜索、详情、目录、正文规则在同一书源连续生效，分页正文按顺序拼接。 | engine fixture 与真实浏览器/API 流程均已覆盖。 | `verified` | 本地 JSON fixture：同一 UI 流程并断言目录数和拼接正文。 |
| XPath 书源 | XPath 列表、属性 URL、详情 `init`、目录和正文可连续使用。 | engine fixture 与真实浏览器/API 流程均已覆盖。 | `verified` | 本地 HTML fixture：同一 UI 流程并断言 XPath 详情/目录/正文输出。 |
| 安全边界 | 上游可联网执行脚本；OpenReader 不得在该验收中放宽 JS/模板/私密网络限制。 | `ErrUnsupportedSourceRule` 和结构化错误已有契约。 | `acceptable security difference` | 夹具仅监听 loopback、无真实凭证/外网；不为测试添加生产环境放行或跳过校验。 |

实施约束：浏览器合同必须启动隔离的临时 OpenReader 数据目录和本地 fixture 服务，先经真实注册、真实书源创建和真实 HTTP API，再进入 Vue 工作台；不得 route/mock `/api`。每个规则模式至少断言搜索标题、BookInfo、三章目录和跨页正文。该合同通过后，P2-Parser 的 CSS/JSONPath/XPath 主路径可标记为已完成；JS/WebJS/模板仍维持已记录的安全差异。

### P2-Parser-4 实施记录

- 新增 `scripts/smoke/source-parser-workflow-contract.mjs`：脚本读取既有不联网的 `backend/engine/testdata/source_compat/` fixture，临时构建并启动 Go 服务，使用隔离 SQLite/data/cache/library 注册管理员和创建 CSS、JSONPath、XPath 三个书源。
- Playwright 直接访问真实 Vue/API 服务，不 route/mock `/api`；每种书源依次验证工作台搜索结果、搜索结果 BookInfo、远程阅读会话详情字段、三章目录面板以及两页拼接的正文。`canReName` 未声明时保持搜索标题的上游语义，同时用详情作者断言详情规则确实被执行。
- 2026-07-16 本地执行结果：`source-parser-workflow: ok CSS, JSONPath, XPath realApi=true fixtureOnly=true searchBookInfoTocContent=true`。夹具仅监听 `127.0.0.1`，无外网、cookie、令牌或生产数据；没有为测试放宽生产书源安全策略。
