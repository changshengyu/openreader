# TXT 目录规则：阅读器内置规则切换与章回目录识别（P1，R1–R5、D1–D7）

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

审计日期：2026-11-04。

状态：**aligned / Docker-published / awaiting-device-verification**（第 11 节）。

## 1. 用户反馈

1. “某本书的章节是这么做的：第x章 章节名 第x回 回名……某章第一回会像这样：第x章 章节名 第x回 回名，但是后面的回就只有
   第x回 回名了”，自动解析不出来；“目录规则还是不完善，需要参照上游或者其他项目的目录规则进行完善”。
2. “导入书籍后再想修改目录规则，就不能选择内置的几个规则了，只能自己输入正则，这和上游项目不符。上游项目是可以在书籍目录界面
   切换目录规则选择内置规则的”。

## 2. 上游权威（reader-dev）

- `src/main/resources/defaultData/txtTocRule.json`：18 条规则，启用 -1、-2、-6、-7、-8、-9、-11、-13、-14、-18。“回”只出现在
  禁用的 -3“目录(匹配简介)”与 -4“目录(古典、轻小说备用)”；标题行尾都限制为 `.{0,30}$`。
- `src/main/java/io/legado/app/model/localBook/TextFile.kt` 64–90：`tocUrl` 为空时用前 512000 字节选规则；336–355 `getTocRule`：
  启用规则逆序，Java `find()` 原始命中数，`cs >= maxCs`（初值 1）覆盖；358–362 只取启用规则。负向前瞻（如 `部(?![分赛游])`）
  只约束紧跟在单位字之后的一个字。
- `src/main/java/com/htmake/reader/api/controller/BookController.kt` 270–276 `getTxtTocRules`：返回全部 18 条（含禁用）；
  `web/src/App.vue` 596–608 启动时载入 store。
- `web/src/components/PopCatalog.vue`：15–21 本地书目录栏“修改规则”；101–118 `tocRuleList`：TXT 用 store 的 18 条，其他用六个 EPUB
  选项（“根据 Spin 获取章节，使用 Toc 补充章节名”等）；164–199 `changeRule`：`$msgbox` 标题“修改目录规则”、“确定”/“取消”，规则
  未变时 `$message.error("未修改规则")` 且不请求，否则 `saveBook` 后“操作成功”并刷新目录；201–252 `renderComp`：
  “请选择规则：” + `el-select size="mini" filterable placeholder="未分组"`（label 规则名、value 规则文本）+ 共用同一值的
  `el-input type="textarea" rows=3`（`margin-top:10px`），初值为书的 `tocUrl`。
- `web/src/views/Index.vue` 945–975、3004–3021：导入确认框同样是内置规则下拉（placeholder“内置规则”）+ 文本框，EPUB 六个选项名同上。
- 刷新只更新章节数和最新章（`BookController.kt` 1785–1815、1924–1945），不把自动选中的规则写回书架记录。

## 3. 参照的其他项目（用户要求）

| 项目 | 版本 | 结论 |
|---|---|---|
| Legado（reader-dev TXT 逻辑的来源） | 官方 `gedoor/legado` 已于 2026-05 清空（仅剩侵权公告）；镜像 `hectorqin/legado@da17bb2`，`txtTocRule.json` 最后修改 `46087e8f`（2023-10-17）；分叉 `huajideshutiao/legado`、`skybbk1001/legadoT` 规则与之逐字相同 | 26 条；“回”仍只在禁用的 -3/-4；-1/-2 改为必须带“第”，新增“书名 序号”“字数分割 分节阅读”等。`getTocRule` 改为一处命中只有距上一次计入的命中结尾超过 1000 字才计数（修正密集误命中）。 |
| Readest | `readest/readest@6ab667b`（2026-11-04）`apps/readest-app/src/utils/txt.ts` 735–790 | 中文默认把 `[章节節回讲講篇话話]` 与卷类单位放在同一个交替里，注释说明“卷包章”时分开的规则会“识别一层、静默丢掉另一层”。 |
| Anx Reader | `Anxcye/anx-reader@0c7fc1a` `lib/models/chapter_split_presets.dart` | 只认“第x章/第x卷”，不认“回/话”。 |

结论：上游与 Legado 都没有在自动识别里认“回”，属于本切片要做的、用户要求的改进；做法参照 Readest（同一规则同时认章、回）
和 Legado 的 1000 字间隔计数（压低目录区、相邻双标题等密集命中）。

## 4. 当前行为（`origin/main` `8c3cf79`，2026-11-04 实测）

`ParseTXT` 自动识别：

| 版式 | 结果 |
|---|---|
| a 单行短：`第一章 章节名一 第一回 回名甲乙丙丁一` | “目录(去空白)”，5 章，标题为整行（正确） |
| b 单行长：章字后超过 30 字 | 无规则命中，退化为 `第1章(1)` 伪章节 |
| c 章行后紧跟回行（各一行，1:1） | “目录(去空白)”，5 章，回行留在正文开头 |
| d 用户的书：每章首行 `第x章 … 第x回 …`，其后各回只有 `第x回 …` | 只有章，2 章各含 4 回 |
| e 只有 `第x回 …`（古典小说） | 退化为伪章节 |
| f 全角空格分隔的 a | 同 a |

阅读器目录栏“修改规则”（`frontend/src/composables/useReaderLocalTocRulePicker.js`）：TXT 只弹 `ElMessageBox.prompt` 正则文本框
（“填写 TXT 目录行正则……”、确认按钮“刷新目录”），没有内置规则；EPUB 是原生 `select`，选项文案（“按 Spine 排序，Toc 补充标题”等，
`frontend/src/utils/localBookToc.js`）与上游不同。确认后总是调用 `refresh-local`，规则未变也刷新。

此外，`backend/engine/txt_parser.go` `txtTitleExclusions` 把负向前瞻近似为“整行出现即排除”：`第一章 部分真相`、`第十章 集合`、
`第五章 一节课` 被拒，而上游只看单位字后一位，会接受。

## 5. 差异与裁决

| # | 项 | 上游 | 当前 | 分类 | 目标 |
|---|---|---|---|---|---|
| R1 | 阅读器“修改规则”（TXT） | 内置规则下拉 + 可编辑文本框，初值为当前规则 | 只有正则输入框 | `must-fix` | 同上游：标题“修改目录规则”，“请选择规则：” + 可搜索下拉（`GET /api/txt-toc-rules` 全部规则，含禁用的手动规则）+ 共用值的 3 行文本框，“确定”/“取消”。 |
| R2 | 同一弹窗（EPUB） | 同一组件，六个上游选项名 | 原生 `select`，文案不同 | `must-fix` | 与 TXT 同一弹窗；六个选项名改为上游文字（导入确认框共用同一列表，一并对齐）。 |
| R3 | 规则未变时确定 | “未修改规则”，不请求 | 照常刷新 | `must-fix` | 提示“未修改规则”，不调用 `refresh-local`。 |
| R4 | 下拉 placeholder | “未分组”（复制自分组选择的笔误） | — | `acceptable-change` | 用上游导入框的“内置规则”。 |
| R5 | 确定后的流程与提示 | `saveBook` + “操作成功” + 刷新 | 一次 `refresh-local`，“目录规则已更新，共 N 章” | `acceptable-change` | 不变。 |
| D1 | 用户的章回混排（版式 d） | 只识别章，各回并入章正文 | 同上游 | `intentional-redesign`（用户要求） | 每一回一个目录项，首回标题保留整行 `第x章 … 第x回 …`。 |
| D2 | 只有“第x回”（版式 e） | 伪章节 | 同上游 | `intentional-redesign`（用户要求） | 每回一项。 |
| D3 | 章回合并行超过 30 字（版式 b） | 不识别 | 同上游 | `intentional-redesign`（用户要求） | 回字前后各允许 30 字。 |
| D4 | “第x话”（轻小说） | 伪章节（“话”只在禁用的 -4） | 同上游 | `intentional-redesign`（参照 Readest、Legado 备用规则） | 与“回”同一规则识别。 |
| D5 | 负向前瞻 | 只约束单位字后一位 | 整行出现即排除 | `must-fix`（对齐上游） | 精确：只检查标记位置之后的文字。 |
| D6 | 章行后紧跟回行（版式 c） | 章标题，回行在正文 | 同上游 | 对照 | 不变：新规则不接管。 |
| D7 | 现代小说偶有一段以“第一回”开头 | 不受影响 | 同上游 | 对照 | 不变：新规则不接管。 |

## 6. 目标行为

### 6.1 新规则（OpenReader 扩展）

在 18 条上游规则之后追加一条：`id: -19`、`enable: true`、`name: "目录(章回、话)"`、`serialNumber: 18`，规则为“目录”(-2)
原文加上回/话一支（`[N]` 为 `[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]`）：

```text
^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[N]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))|(?:第\s{0,4}[N]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张)).{0,30}?[ 　\t])?第\s{0,4}[N]+?\s{0,4}(?:回(?![合来事去])|话)).{0,30}$
```

- 回/话必须带“第”（避免“一回到家……”）；不收“场”（“第一场雪”）。
- 章回合并行：可选前缀“第x章（或节、卷、集、部、篇）+ 至多 30 字 + 空白”，回/话之后再允许 30 字。
- 这条规则也出现在 `GET /api/txt-toc-rules`，导入确认框与阅读器弹窗都能手动选。

### 6.2 自动选规则

1. 18 条上游规则中启用的 10 条，仍按 reader-dev 方式选出 U：逐行原始命中数，逆序，`>=` 覆盖，至少 1。
2. 另为每条启用规则（含 -19）算“间隔计数”（Legado `getTocRule`）：一行命中只有在距上一次计入的命中行尾超过 1000 个字符
   （解码后探测文本的 Unicode 码点）时才计入，第一处总计入。
3. 只有当 -19 的间隔计数 ≥ 每条启用上游规则的间隔计数 + 2 时选 -19，否则选 U（没有 U 时仍是上游的 10 KiB 伪章节）。
4. 选中的规则文本照旧写回书的 `tocRule`（导入时）。

推论：没有回/话标题的书，-19 命中的行与“目录”完全相同，间隔计数不可能多出 2，选择结果与 reader-dev 相同；版式 c 中回行距章行
不足 1000 字，不另计；单独一处误命中最多多出 1；目录区（书首列出的标题）只计一次。

### 6.3 负向前瞻（D5）

规则里内容为字面量选择（`(?!完|结)`）或字面量字符类（`(?![合来事去])`）的负向前瞻，编译时换成空的命名标记组；RE2 匹配后，
每个参与匹配的标记之后的文字不得以任一被排除的字面量开头。取代“整行出现即排除”。其他环视写法仍按现状显式报错
（两条上游“双标题”规则保持专用匹配器）。只会比现状接受更多行，且接受的都是 Java 也接受的行。

### 6.4 阅读器弹窗（R1–R4）

`useReaderLocalTocRulePicker.choose()`：TXT 与 EPUB 共用一个 `ElMessageBox` 弹窗（见 R1/R2），初值为书的 `tocRule`；
TXT 规则列表打开时按需请求并缓存，请求失败提示“加载目录规则失败”、弹窗照常打开（文本框可用）；确定后规则与初值相同则提示
“未修改规则”并返回 `null`；取消/关闭返回 `null`；其余返回所选文本，后续沿用现有 `refresh-local` 流程。

## 7. 允许差异

R4、R5；D1–D4 的新规则与选择方式（用户要求）；Element UI `size="mini"` 对应 Element Plus `size="small"`。

## 8. 计划测试

| # | 位置 | 断言 | 改前预期 |
|---|---|---|---|
| T1 | `backend/engine/txt_toc_zhang_hui_contract_test.go` | 版式 d（2 章 × 4 回）自动识别为 8 项，标题依次为各标题行，生效规则为 -19 | 失败（2 项） |
| T2 | 同上 | 版式 d 的章回合并行超过 30 字，仍为 8 项 | 失败 |
| T3 | 同上 | 版式 e 只有回：每回一项 | 失败（伪章节） |
| T4 | 同上 | 只有“第x话”：每话一项 | 失败 |
| T5 | 同上 | 手动 -2：接受 `第一章 部分真相`，拒绝 `第一部分……`、`第一节课外讲义`；手动 -19：接受 `第三回 宝玉回来`，拒绝 `第一回来人……`、`第二回合` | 失败 |
| T6 | `backend/engine/parser_test.go`、`backend/api/api_test.go` | 规则列表 19 条：前 18 条与上游逐字相同，第 19 条为 6.1 原文 | 失败（18 条） |
| C1–C5 | `txt_toc_zhang_hui_contract_test.go` | 普通章节小说、版式 a、版式 c（D6）、单处“第一回”（D7）、书首目录区：选择与目录不变 | 通过（对照） |
| P1–P6 | `frontend/tests/readerLocalTocRulePicker.test.mjs`（与上游冲突的旧断言按本合同重写） | 弹窗结构与文案；选内置规则、改文本框；未变提示且返回 `null`；取消；EPUB 同一弹窗；规则加载失败仍可用 | 失败 |
| P7 | `frontend/tests/localBookTocOptions.test.mjs` | EPUB 六个选项名与上游一致 | 失败 |
| P8 | `frontend/tests/nightElementThemeContract.test.mjs`（原生 `select` 样式断言随 R2 重写） | 选择器用 Element 组件、不再内联原生 `select` 样式 | 失败 |
| B1 | `scripts/smoke/reader-local-toc-rule-contract.mjs`（真实服务） | 导入版式 d 的 TXT：目录项数为回数；打开“修改规则”，下拉显示“目录(章回、话)”、文本框为规则；选“目录”后确定，目录变为章数；再次确定不改规则提示“未修改规则”且没有 `refresh-local`；1440×900、390×844、360×800 | 失败 |

回归：backend `go test ./...`、`go vet ./...`、`go test -race ./api ./db ./services/...`；frontend `npm test`、`npm run build`；依赖公告门；
B1 三视口；本地导入既有浏览器合同。

## 9. 不重开

- reader-dev 上游规则之间的原始计数竞争、512000 字节探测窗、10 KiB 伪章节、前言、偏移与缓存。
- Legado 的其他改动（-1/-2 必须带“第”、新增规则、对所有规则改用间隔计数）不采纳，留作后续候选。
- UMD/CBZ 本地书不显示“修改规则”（上游会对它们显示 EPUB 选项，无意义）。
- `refresh-local` 不把自动选中的规则写回（与上游书架记录一致）；导入确认框除 EPUB 选项名外不变。

## 10. 记录（2026-11-04）

分支 `claude/txt-toc-zhang-hui-detection`，从 `origin/main` `8c3cf79` 切出：合同 `9d3ec44`、红测 `1fbd58d`、实现 `f8c20ab`、
浏览器合同点击修正 `e61157f`。

- 红测（`8c3cf79`）：T1 版式 d 只得 2 项（应 8）；T2、T3、T4 退化为 `第1章(1)`–`(3)` 伪章节；T5“目录”对 `第一章 部分真相`、
  `第二章 集合`、`第三章 一节课`、`第四章 正文完结之后` 0 命中，-19 规则把 `第三回 宝玉回来` 整行排除（并入“前言”）；T7 3 项（应 5）；
  T6 引擎与 `GET /api/txt-toc-rules` 都只有 18 条。对照 C1–C5 与 T5“仍应拒绝”子项通过。前端 9 项失败（TXT 打开 prompt、EPUB 打开
  原生 `select` 确认框、EPUB 选项名、夜间断言、阅读器接线），11 项夜间对照通过。浏览器合同在改前构建停在第一处检查（规则列表无 -19）。
- 实现后同一断言转绿：engine 包全绿（另加前瞻改写辅助测试：转义、字符类内文字不改写，非字面量前瞻仍显式报错），
  `TestListTXTTocRules` 通过，前端相关 20/20。
- 回归：backend `go test ./...` 31 个包通过、7 个无测试（api 126.9 s），`go vet ./...` 无输出；race `./api ./db ./services/...`
  26 个包通过（api 1049.6 s，`-timeout 40m`），无数据竞争；frontend `npm test` 877/877、`npm run build`；公告门自测 114/114，实时 Go 与前端扫描 `pass`、0 项；
  浏览器合同 B1 三视口（1440×900、390×844、360×800）通过，1440×900 夜间主题截图核对弹窗、下拉与文本框取夜间配色；
  既有 `local-book-import-contract.mjs` 三视口通过。
- 浏览器核对中的两处调整：下拉打开时滚到当前规则，“目录”在列表视口外，按坐标点击会落到消息框遮罩并关闭弹窗（Element 的点击遮罩
  关闭，上游同样如此），合同脚本改为先滚入视野并等待弹层动画（`e61157f`），不是产品缺陷；Element Plus 的 `el-select` 默认宽 100%，
  补两条全局样式让“请选择规则：”与 200px 下拉同行、文本框占满宽度，与上游布局一致。
- 状态 **implementation-complete / regression-validated**；未合入 main，镜像未发布，生产未动。

## 11. 发布（2026-11-04）

`origin/main` 仍为 `8c3cf79`，没有进行中的发布，本分支直接快进：main 到 `257f458`（ls-remote 核验，未强推）。可信 Actions
`41832867417`（docker-publish）43 步全部成功。`ghcr.io/changshengyu/openreader:257f458` 与 `latest` 同为 OCI index
`sha256:43c89177cb84fa906682a248e400f00ab080b661149f72284c7d6b98cce6c776`：linux/amd64 `sha256:0ead270a…dfa9`、linux/arm64
`sha256:1c8419b1…ddce`，两个平台的 `org.opencontainers.image.revision` 均为 `257f45830faf9b82031c1b7ebbe646120ff7fbc6`。

状态 **aligned / Docker-published / awaiting-device-verification**。生产未访问，实际运行版本未知。已导入的书不会自动重新解析：
设备核验时在阅读器“目录 → 修改规则”选“目录(章回、话)”后确定，确认目录按回切分；规则未变时应提示“未修改规则”。
