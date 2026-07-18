# 书源元数据规范化与改名语义合同（P2）

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-18 已完成固定上游与当前调用链盘点；本文件是测试和实现的前置闸门，
本阶段未修改应用代码。

## 权威文件

- 上游 `BookList.kt#getSearchItem/getInfoItem`
- 上游 `BookInfo.kt#analyzeBookInfo`
- 上游 `BookHelp.kt#formatBookName/formatBookAuthor`
- 上游 `AppPattern.kt#nameRegex/authorRegex`
- 上游 `StringExtensions.kt#htmlFormat`
- 当前 `backend/engine/source_parser.go`
- 当前 `backend/api/books.go`、`remote_reader.go`
- 当前 `backend/engine/source_rule_evaluator_test.go` 与 `backend/api/api_test.go`

## 固定上游合同

### 搜索与探索列表

`BookList.getSearchItem()` 对 CSS、JSONPath、XPath、正则等规则使用同一组后处理：

1. 书名先执行 `BookHelp.formatBookName`；结果为空时整条结果不进入列表。
2. 作者执行 `BookHelp.formatBookAuthor`。
3. 简介执行 `htmlFormat`，将 `br/p/div` 的可见分段变为换行并清理首尾空白。
4. 分类仍按完整字符串列表以逗号连接；字数继续使用既有 `wordCountFormat`。
5. 详情式搜索结果调用 `BookInfo.analyzeBookInfo(..., canReName=false)`，但新建的空 Book 仍会获得
   规范化后的书名和作者。

固定基准的转换字面量为：

```text
nameRegex   = \s+作\s*者.*|\s+\S+\s+著
authorRegex = ^\s*作\s*者[:：\s]+|\s+著
```

两者替换后都只裁剪字符值 `<= U+0020` 的首尾空白。`htmlFormat` 将 `br/p/div` 边界转换为
换行、移除 `&nbsp;` 和标签噪声、压缩空行，并在后续段落前保留两个全角空格的缩进语义。

### 详情与 `canReName`

- `BookInfo` 同样规范化书名、作者和简介。
- `ruleBookInfo.canReName` 在固定基准中是**配置存在标志**：
  `mCanReName = callerCanReName && !rule.canReName.isNullOrBlank()`。
- 上游不会执行 `canReName` 字段并把其返回值解释为布尔。即使选择器无匹配、值是 `0`、
  `false` 或空文本，只要该配置字段非空且调用方允许，远端非空书名/作者就可以覆盖已有值。
- 没有配置该字段时，已有的搜索书名/作者必须保留；只有当前值为空时才用详情值补齐。
- 分类、字数、最新章节、简介和封面不受改名标志控制；远端非空值仍按各自规则更新。

## 当前 OpenReader 差距

| 范围 | 当前行为 | 判定与必须动作 |
| --- | --- | --- |
| CSS 搜索/详情 | `parseBookResults`、`parseRemoteBookInfo` 直接返回原始书名、作者、简介；详情将非空 `canReName` 配置视为 true。 | 元数据后处理 `must-fix`；改名标志本身已对齐。 |
| JSONPath/XPath/正则搜索/详情 | evaluator 路径也直接返回原始元数据；`parseRemoteBookInfoWithEvaluator` 还会执行 `canReName` 规则并调用 `sourceRuleBool`。 | `must-fix`：同一配置不能因规则模式而改变改名结果；该字段不得进入求值器。 |
| API 合并 | `firstNonBlankCanRename(remote,current,allowRename)` 已实现“当前为空时补齐；允许改名时覆盖”。创建书架、刷新、换源和临时 Reader 共用该合并。 | `technical-stack-equivalent`：保留 API 与事务，只修正引擎提供的规范化值和配置标志。 |
| 已有数据 | 旧 SQLite 行可能保存未规范化元数据。 | `must-preserve`：不批量迁移、不扫描或重写旧书架；只在后续搜索、添加、刷新、换源或临时会话中使用修正结果。 |

## API、数据和安全边界

- 不改变任何路由、请求体、响应字段、状态码、同步事件或 SQLite schema。
- 不改变书源导入/导出 JSON；`canReName` 原字符串继续无损保存。
- 元数据规范化只作用于解析结果，不作用于用户手工编辑的书名/作者，也不作用于本地书导入。
- 简介结果仍作为普通字符串序列化和渲染；实现不得引入 `v-html`、脚本执行或实体扩展炸弹。
- 当前长度、远端响应、规则求值和安全 HTML 边界继续生效；格式化失败不能写入失效书源缓存。

## 实施前测试

1. engine 黄金测试以 CSS、JSONPath、XPath 三种书源分别覆盖：
   - `书名 作者：某人`、`书名 某人 著` 得到同一规范化书名；
   - `作者：某人 著` 得到规范化作者；
   - `p/div/br` 简介得到固定换行与全角缩进结果；
   - 搜索列表与详情式回退使用相同转换。
2. `canReName` 模式一致性：CSS、JSONPath、XPath 的配置字段分别指向 `0`、`false`、空文本或
   不存在节点，均应仅按“字段已配置”得到 true；未配置时为 false。测试同时证明该字段不会触发
   `ErrInvalidSourceRule`、`ErrUnsupportedSourceRule` 或额外远端请求。
3. API 契约覆盖创建书架、刷新、换源和临时 Reader：配置存在时使用规范化详情书名/作者，配置缺失时
   保留请求/书架已有值；简介等非改名字段继续更新。
4. 旧 CSS、JSONPath、XPath 搜索→BookInfo→目录→正文真实工作流继续通过；无路由、数据库、缓存或
   备份格式变化。
5. 全量 Go、前端测试、生产构建与三视口书源浏览器合同通过后，才可提交实现；是否发布 Docker 按
   当时切片完整度判断。

## 允许差异

- OpenReader 可以用等价、线性时间且有界的 Go 字符串处理实现 Kotlin 正则与 `htmlFormat`，但黄金输出
  必须一致。
- 出于浏览器安全，简介始终作为文本数据消费；不复制上游 WebView/HTML 注入能力。
