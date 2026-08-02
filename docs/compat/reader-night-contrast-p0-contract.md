# Reader 移动夜间模式正文对比度合同

状态：2026-07-28 已完成四轮固定上游复审与真实渲染修订。第五轮用户复验所访问的线上
`/api/health` 仍返回 `59e11a9`，早于纯黑自定义夜间内容面修复 `3ee3a82`；GHCR
`latest` 实际已经是包含该修复的 `342d736`。当前阻断点是部署端复用了旧本地镜像，不是
`342d736` 中再次复现同一实现缺口。部署验收必须先升级运行容器，并以健康检查版本为准。

固定上游：

- `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
- `web/src/plugins/config.js`
- `web/src/plugins/vuex.js#setNightTheme`
- `web/src/components/Content.vue#containerStyle`
- `web/src/components/ReadSettings.vue`
- `web/src/views/Reader.vue#toogleNight`
- `web/src/App.vue#autoSetTheme`

当前映射：

- `frontend/src/stores/reader.js`
- `frontend/src/composables/useReaderAppearanceAssets.js`
- `frontend/src/views/Reader.vue`
- `frontend/src/layouts/AppLayout.vue`
- `frontend/src/components/reader/ReaderSettingsPanel.vue`
- `frontend/src/components/reader/ReaderChapterContent.vue`
- `frontend/src/components/reader/ReaderTocPanel.vue`
- `frontend/src/utils/readerThemeContrast.js`

## 审查矩阵

| 合同 | 固定上游 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 浏览器深浅色自动切换 | `App.vue#autoSetTheme` 调用 `setNightTheme(true/false)`；该 mutation 选择“黑夜默认/白天默认”并一次应用完整方案。 | `applyAutoTheme()` 会选择默认方案，但 Reader/Index 的手动月亮按钮绕过方案，只调用 `setTheme("dark"/"parchment")`。自动和手动没有共用一个状态转换。 | `must-fix`：所有昼夜入口必须走同一个完整默认方案动作。 |
| 手动夜间按钮 | `Reader.vue#toogleNight` 只切换语义昼夜，但由 `setNightTheme` 应用方案中的背景、字体色、排版和主题类型。 | `useReaderAppearanceAssets.toggleNight()` 与 `AppLayout.toggleNightTheme()` 只换 preset；当前 `customConfigName` 不变，并会把新 theme 反写到旧活动方案。 | `must-fix`：不得把“内置白天”原地改成夜间外观，也不得遗留白天字体色。 |
| 正文字体色 | `Content.vue#containerStyle` 使用方案 `fontColor`；缺失时按语义夜间回退 `#666`、白天回退 `#262626`。内置昼夜方案均带明确字体色。 | `reader.fontColor || reader.currentTheme.text` 让任何旧/自定义字体色永久压过 preset；深色背景上可继续使用白天深色文字。 | `must-fix`：有效正文颜色必须检查与有效背景的可读对比度；不足时使用语义高对比回退。 |
| 自定义颜色与背景图的作用域 | 上游 `currentThemeConfig` 仅在 `theme === "custom"` 时读取 `bodyColor/contentColor/contentBGImg/popupColor`；普通 preset 完全使用 preset 资源。 | `customBgImage` 与 `customBodyColor` 不区分当前主题，切到 `dark` 后仍可能覆盖深色 preset；正文色却已变为夜间色，形成亮图/亮字或暗图/暗字。 | `must-fix`：自定义背景色、背景图和弹层色只在 custom 主题生效。 |
| 自定义主题“白天/黑夜” | 上游只改变语义 `themeType`，不擅自丢弃用户颜色。 | 同样只改变 `themeType`。 | 保留操作语义，但按用户明确要求增加可读性保护：对比不足时仅对渲染使用安全色，不覆盖持久化的用户颜色。 |
| EPUB/普通文本/音频 | 上游普通正文和 EPUB 注入样式共享当前方案字体色；音频控件使用夜间语义色。 | 普通文本和 EPUB 各自重复 `fontColor || currentTheme.text`；音频继承 `--reader-text`。 | `must-fix`：三者统一使用一个计算后的有效正文色，避免其中一种仍暗字暗底。 |
| 移动端 | 上游 mini interface 仍使用同一 `currentThemeConfig` 与 Content 字体色。 | 移动端使用同一 CSS 变量，但问题最容易在全屏背景上暴露。 | 必须在 390×844、360×800 验证，不接受仅桌面或仅 store 测试。 |

## 实施合同

1. 在 Reader store 建立唯一 `setNightTheme(isNight)` 状态转换：
   - 优先应用当前配置列表中相应的“黑夜默认/白天默认”完整方案；
   - 缺失或损坏时回退内置昼夜方案；
   - 保持 `customConfigName`、`themeType`、背景、字体色和其它方案字段一致；
   - 浏览器自动切换、Reader 月亮按钮、Index 侧栏按钮都调用它。
2. preset 主题渲染不得继续读取 custom 专属背景图片或外层/弹层颜色；custom 主题仍完整保留。
3. 新增纯函数计算有效正文色：
   - 可解析的纯色背景与字体色使用 WCAG 相对亮度计算；
   - 普通正文最低对比度为 `4.5:1`；
   - 用户颜色达到阈值时原样保留；
   - 不足或不可解析时，按有效背景从深色/浅色安全候选中选择对比度更高者；
   - 该保护只影响渲染，不重写用户保存的 `fontColor`。
4. `Reader.vue` 的 `--reader-text` 和 EPUB 注入 CSS 必须共享该有效正文色；Audio 与普通章节继续继承该变量。
5. 如果 custom 背景图存在，语义夜间优先浅色安全正文、语义白天优先深色安全正文，并保留可见的轻量文字阴影，以应对图片局部明暗变化；非 custom preset 不加载该图片。

## 测试与发布门

1. 先增加失败测试，覆盖：
   - 白天深色 `fontColor` 切夜间后不能产生暗字暗底；
   - 手动和自动昼夜入口使用同一完整方案转换；
   - 手动切换不改坏原昼夜方案身份；
   - custom 资源不污染 preset；
   - 普通文本、EPUB 与 Audio 使用同一有效正文色；
   - 足够对比的用户颜色仍被保留。
2. 前端全量测试和 production build。
3. 真实浏览器 390×844、360×800：
   - Reader 月亮按钮切换；
   - 设置中的“内置黑夜”和 custom 黑夜语义；
   - 计算后的正文/背景对比度不低于 `4.5:1`；
   - 工具层与设置面板保持原有并存状态。
4. 以独立、可追踪提交推送本问题；随后执行本地 Docker 构建、历史卷、备份恢复与 GHCR 发布门。

## 实施结果

- Reader store 已建立唯一 `setNightTheme(isNight)`；浏览器自动主题、Reader 月亮按钮和
  Index 侧栏按钮共用该动作，并完整应用“白天默认/黑夜默认”方案。损坏或缺失方案回退内置
  preset，切换过程不再把夜间字段反写进原白天方案。
- preset 只使用自身的 body/content/popup 资源；自定义背景色与背景图只在 `custom` 生效。
- `readerThemeContrast.js` 在渲染层计算正文与有效背景的 WCAG 对比度。低于 `4.5:1` 时使用
  安全明/暗色，但不覆盖持久化的 `fontColor`；自定义背景图增加轻量文字阴影。
- 普通章节、EPUB 注入样式和 Audio 继承同一 `--reader-text`。设置、目录、书架、换源、
  缓存、桌面与移动工具层改为同一 popup/text/control 语义变量。
- 日间强调色仍以固定上游 `#ed4259` 为 fallback；夜间改用 `#ff7589`。这是用户要求的
  可读性增强，夜间强调色对 `#303030` 控件面的对比度为 `5.13:1`。
- 自动回归：后端 `go test ./...` 通过；前端 `636/636` 通过；Vite production build 通过。
- 真实浏览器：390×844 与 360×800 的 TXT 正文为 `#d8d4c8` / `#2d2d2d`，
  对比度 `9.29:1`；夜间普通控件文字/背景为 `#d8d4c8` / `#303030`，
  对比度 `8.91:1`；EPUB iframe 使用同一有效正文色。设置面板与移动工具层可并存，
  无控制台错误。
- 浏览器 `prefers-color-scheme` 入口由 store/bootstrap 契约测试覆盖；真实浏览器覆盖了
  同一动作的手动月亮入口。当前内置浏览器运行时不支持直接改写系统配色媒体查询，因此没有
  把无法执行的媒体仿真写成真实证据。

## 第二轮实机反馈与修订合同

用户在 `cca1320` 实机确认：从浏览器自动主题或设置切换夜间后，正文仍不是黑底白字；其判断是
黑色被设置为阅读背景后，实际承载文字的页面层仍存在其它背景。

### 重新取证

| 层级 | 固定上游 | `cca1320` | 第二轮裁决 |
|---|---|---|---|
| 内置夜间配色 | `defaultNightConfig` 为 `fontColor: #666666`、`bodyColor: #121212`、`contentColor: #171717`；preset 6 实际使用 `body_6/content_6` 暗纹理。 | `dark` preset 为正文 `#2d2d2d`、文字 `#d8d4c8`。上一轮只验证二者对比度 `9.29:1`。 | 上游和当前都不满足用户明确要求。登记 `intentional-redesign`：内置夜间必须是 `#000000` 正文面和 `#ffffff` 默认正文。 |
| Reader 外壳 | 上游夜间使用 `body_6` 暗纹理。 | `.reader-shell` 无条件应用 `--reader-body-texture`，即使 `themeType === "night"`。 | `must-fix`：内置夜间的外壳和正文都不得叠加日间纸张纹理。 |
| 实际正文页 | 上游 `chapterTheme` 把当前主题 content 直接应用到 `.chapter`。 | `.reader-page` 同时声明 `background-color: --reader-bg` 和带 fallback 的 `background-image`；是否纯黑不能只由 store 色值证明。 | `must-fix`：为页面提供显式语义背景图变量；内置夜间必须计算为 `background-color: rgb(0,0,0)`、`background-image: none`。 |
| 连续滚动 | 上游移动滚动仍由同一个 `.chapter` 主题承载。 | document-scroll 改变高度与滚动宿主，但继续复用 `.reader-page`；底层 shell 在长页面边缘仍可能露出纹理。 | `must-fix`：普通、scroll、scroll2、flip 都必须继承同一纯黑页面合同。 |
| EPUB | 上游把字体样式注入 iframe，但 EPUB 自带 CSS 仍可参与背景。 | iframe 本身透明，`body` 强制透明；`html` 仅普通 `background: transparent`，不能压过 EPUB 作者样式，标题等非 `p` 文本也没有强制继承有效正文色。真实浏览器进一步证明透明 iframe 画布仍可能显示为白色。 | `must-fix`：iframe 外壳以及 `html/body` 必须显式绘制当前 Reader 页面背景，不能依赖透明透出；常见文本块必须继承有效正文色。 |
| 自定义主题 | 上游允许自定义 body/content/popup/font。 | 第一轮已把 custom 资源限制在 custom。 | 保留：纯黑白强制合同只适用于内置夜间；custom 夜间继续尊重用户颜色，并保留 `4.5:1` 渲染保护。 |
| 亮度 | 上游亮度属于单独阅读设置。 | `.reader-page::after` 以黑色遮罩实现低亮度，会同时压暗视觉文字。 | 保留亮度语义；真实浏览器验收先以默认 `100%` 验证准确黑白，再单独确认降低亮度不会引入其它色层。 |

### 第二轮测试先行门

1. store 契约锁定内置 `dark/black` 正文 `#000000`、默认文字 `#ffffff`、外层 `#000000`。
2. Reader CSS 变量锁定内置夜间：
   - `--reader-body-bg: #000000`；
   - `--reader-body-bg-image: none`；
   - `--reader-bg: #000000`；
   - `--reader-bg-image: none`；
   - `--reader-text: #ffffff`（无有效用户自定义色时）。
3. 日间 preset 保留已有纸张纹理；custom 只加载自己的背景图，不继承日间纹理。
4. EPUB 注入合同必须覆盖 iframe 外壳和 `html/body` 显式绘制当前页面背景，并让标题、段落、
   列表、引用、表格文字继承有效正文色。
5. 390×844、360×800 真实浏览器逐项读取 `.reader-shell`、`.reader-page`、正文段落和
   EPUB iframe 的 computed style，不能再用 store 值或截图观感替代实际渲染层证据。

### 第二轮实施与真实证据

- 内置 `dark/black` preset 已改为 `#000000` 正文、`#ffffff` 默认文字和 `#000000` 外壳；
  `resolveReaderSurface()` 为所有非 custom 夜间方案统一返回无纹理的纯黑页面。
- Reader 现在分别传递 body/page 背景图、页面边框和页面阴影语义变量。内置夜间四项分别为
  `none / none / transparent / none`；日间保留纸张纹理，custom 只使用自己的背景图。
- EPUB iframe 外壳直接使用 `--reader-bg`。注入 iframe 的 `html/body` 不再依赖透明画布，
  而是显式绘制有效 Reader 页面背景，并强制清除 EPUB 自带根背景图；常见文本块继承有效正文色。
- 第一轮真实复验曾观察到：EPUB `html/body` 计算为透明、标题和段落为白色，但 iframe 画布
  截图仍是白色，导致白字不可见。这一实际渲染证据直接触发了上述“显式绘制背景”修正。
- 修复后 390×844：
  - TXT shell/page 为 `rgb(0,0,0)`、`background-image: none`，段落为 `rgb(255,255,255)`；
  - EPUB iframe、`html/body` 为 `rgb(0,0,0)`，标题与段落为 `rgb(255,255,255)`。
- 修复后 360×800 的 TXT 与 EPUB 得到相同结果；黑白对比度为 `21:1`，浏览器控制台无错误。
- 浏览器自动深色和手动入口仍调用同一个 `setNightTheme()`；自动入口由 store/bootstrap
  契约测试覆盖，手动入口由上述真实浏览器流程覆盖。
- 最终自动回归：后端 `go test ./...` 通过；前端 `639/639` 通过；Vite production build
  通过。真实浏览器与自动回归共同覆盖 TXT、EPUB、手动/自动入口和主题资源隔离。
- 实现提交 `a90d10b` 已推送 `main`；本地构建并发布
  `ghcr.io/changshengyu/openreader:a90d10b` 与 `:latest`。两者共同指向多架构 OCI 索引
  `sha256:c0480023418b94d06f55baa8e25e3976f7aa4e9b86b8ba4854ca136d99be1b3e`，
  包含 `linux/amd64` 与 `linux/arm64`。
- 新卷 smoke 通过 portable v1/v2 assets、跨用户、重启和备份恢复；历史卷 smoke 通过
  TXT、EPUB、UMD、CBZ、相对缓存与 owner isolation。该批状态为
  **Docker-published / awaiting device verification**。

## 第三轮实机反馈：实际文字承载层仍可覆盖黑底（2026-07-28）

用户在 `a90d10b` 及其后续镜像实机复验确认：内置夜间仍没有稳定呈现为“纯黑正文背景 +
纯白正文”，其观察是 Reader 外层已经变黑，但实际承载文字的内容层仍保留其它背景。

本轮固定上游复审仍以
`reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 的 `Content.vue`、
`Reader.vue`、`config.js` 和 `vuex.js` 为准。上游只把主题内容面应用到 Reader 章节容器，
EPUB 只向 iframe 的 `body` 和 `p` 注入字体/颜色，并不会清除 EPUB 作者样式中的内层
`div/section/main/span/table` 背景。OpenReader 第二轮虽然进一步把 iframe 外壳及
`html/body` 设为黑色，但真实 EPUB 的内层作者背景仍可盖在该黑底之上。因为“纯黑底白字”
是用户明确要求，这一行为登记为相对上游的 `intentional-redesign`，不能继续照搬上游缺口。

### 第三轮审查矩阵

| 层级 | 当前实现 | 裁决 |
|---|---|---|
| 内置夜间文字 | `resolveReaderTextColor()` 仍会保留任何达到 `4.5:1` 的旧 `fontColor`，例如灰白色；这满足第一轮 WCAG 合同，却不满足用户要求的纯白。 | `must-fix`：`dark/black` 内置夜间直接使用 `#ffffff`，不得被旧方案字体色覆盖。自定义夜间继续保留用户颜色与对比度保护。 |
| TXT/在线正文 | 正文 HTML 已通过 `readerContent.js` 白名单清除属性，普通文本承载层通常透明；但没有显式的内置夜间内容后代合同。 | `must-fix`：内置夜间的章节、段落及白名单内联后代都必须继承白字、透明背景，不得重新建立浅色文字面。 |
| EPUB 根层 | iframe、`html/body` 已显式使用 Reader 页面黑底。 | 保留，但真实验收不能只读这三层。 |
| EPUB 作者内容层 | 注入 CSS 只强制常见文字块的 `color`，未清除 `main/section/div/span/table/pre/code` 等节点的 `background/background-image`。作者 CSS 或内联样式可在黑色 `body` 上重新绘制浅色块。 | `must-fix`：只在内置夜间注入后代透明背景重置和白色前景；图片本身继续显示，custom/day 主题不得被该重置污染。 |
| 测试证据 | 第二轮浏览器 fixture 的 EPUB 只有简单标题与段落，因此即使内层重置缺失也会通过。 | `must-fix`：fixture 必须包含带浅色 `main/div/span/table` 作者背景与深色作者文字的 EPUB，再读取这些实际节点的 computed style。 |

### 第三轮测试先行门

1. 纯函数/静态契约先证明当前实现会保留“可读但非白色”的内置夜间字体，并且 EPUB 注入
   尚未清除内层作者背景；测试必须先失败。
2. Reader 根增加明确的 `built-in-night` 语义状态；只有 `themeType === "night"` 且
   `theme !== "custom"` 时生效。
3. 内置夜间的最终正文色固定为 `#ffffff`，正文面固定为 `#000000`；custom 夜间仍使用
   用户颜色、背景图和 `4.5:1` 渲染保护。
4. EPUB 注入在内置夜间必须让 `body` 后代的文字继承白色，并把作者
   `background/background-color/background-image` 重置为透明/无；`html/body` 自身保持
   黑色。普通正文的白名单内联后代必须遵守同一语义。
5. 真实浏览器在 390×844、360×800 和 1440×900 分别检查：
   `.reader-shell`、`.reader-page`、普通段落/内联节点、EPUB iframe、
   EPUB `html/body/main/div/span/table`；内置夜间要求实际文字面为透明叠加纯黑根面、
   最终文字 `rgb(255,255,255)`。日间与 custom 回归不得被强制黑白。

### 第三轮实施结果

- Reader 根新增 `built-in-night` 语义状态。只有非 `custom` 的语义夜间进入强制黑白；
  `resolveReaderTextColor()` 在该分支不再接受“对比度合格但不是白色”的旧 `fontColor`。
- 普通 TXT/在线正文的 `[data-reader-block]`、`mark/span` 等白名单后代在内置夜间统一为
  `#ffffff`，自身背景透明并叠加 `.reader-page` 的 `#000000`；日间退出后浏览器默认
  `mark` 背景恢复。
- EPUB `setStyle` bridge 现在同时接收 `builtInNight`。bridge 在 iframe 内保存
  `html/body` 和正文后代原有的 inline value/priority，再以 inline `important` 接管
  `color/-webkit-text-fill-color/background-color/background-image/box-shadow/text-shadow`；
  新增节点由独立 MutationObserver 接管。退出内置夜间会断开 observer 并逐项恢复原值，
  因此日间与 custom 不会永久丢失出版物样式。
- 测试先行证据：新增断言在旧实现分别失败于灰白 `#d8d4c8` 被保留、缺少
  `built-in-night` 后代重置，以及 EPUB bridge 未携带/执行内置夜间状态；实现后均转绿。
- 自动回归：Go `go test ./...` 通过；frontend `640/640` 通过；Vite production build
  通过。
- 普通正文完整 Reader smoke 在桌面、390×844、360×800、iPad 自适应与强制手机模式通过：
  shell/page 为纯黑无纹理，段落、`mark`、`span` 为纯白且透明叠黑，退出夜间恢复日间
  `mark` 表面。
- 真实 API EPUB smoke 导入带 `!important` 白底/渐变/黑字/阴影的
  `main/div/span/table/td` fixture；1440×900、390×844、360×800 均证明夜间
  `html/body` 为纯黑、所有实际文字后代透明叠黑并为纯白，切回日间后作者白底、渐变与
  `#111` 文字原样恢复。

### 第三轮发布记录

- 夜间实际文字承载层实现提交为 `4d40487`。Docker 发布门在真实容器的首次登录并发请求中
  额外发现 `GET /api/explore/sources` / `GET /api/sources` 的 SQLite 读事务升级可能立即
  返回 `database is locked`。随后以确定性失败测试锁定“同命名空间并发”和“无关写事务占锁”
  两种场景，`f9723ad` 跨 Service 串行化首次初始化，`9a13d8e` 仅对 SQLite
  `BUSY/LOCKED` 增加有限重试。路由、成功响应、数据库结构和持久数据均未改变。
- 最终提交 `9a13d8e8edf3059042fc9ad2bc19e017e926401b` 已推送 `main`；本地构建并发布
  `ghcr.io/changshengyu/openreader:9a13d8e` 与 `:latest`。两者共同指向多架构 OCI 索引
  `sha256:777bcb96fa59d718b413b22756b3b30696b891bed7826930af168ce15d0e6bed`，
  包含 `linux/amd64` 与 `linux/arm64`。
- 最终容器验证：
  - EPUB 真实 API 在 1440×900、390×844、360×800 通过嵌套作者背景接管与日间恢复；
  - 普通正文在桌面、两种手机、iPad 自适应和强制手机模式通过黑底白字、面板与工具层合同；
  - 首次登录并发启动请求零 `500`；
  - 新卷通过 portable v1/v2 assets、跨用户、重启和备份恢复；
  - 历史卷通过 TXT、EPUB、UMD、CBZ、相对缓存和 owner isolation。
- 允许差异：固定上游内置夜间仍为暗纹理和灰字；本批按用户明确要求将 OpenReader 的
  **内置**夜间固定为纯黑/纯白。`custom` 夜间继续保留用户颜色、图片和 `4.5:1` 可读性保护，
  不被强制改写。
- 未完成：本批状态为 **Docker-published / awaiting device verification**；全量 Reader
  与其余 P2 重构仍按总审查计划继续，本合同只关闭“内置夜间实际文字层”这一 P0 切片。

## 线上版本漂移复验与重新发布（2026-07-28）

用户再次报告“外层黑、实际文字承载层不黑”后，通过用户已登录的线上 OpenReader 页面进行
只读取证：侧栏健康版本明确显示 `a90d10b`。该版本只包含第二轮 iframe 根层修复，早于
`4d40487` 的普通正文后代/EPUB 作者内容层接管，因此用户观察到的现象与该旧版本的已知缺口
完全一致。同期 GHCR `latest` 已指向第三轮镜像
`sha256:777bcb96fa59d718b413b22756b3b30696b891bed7826930af168ce15d0e6bed`，
说明问题是运行实例未真正换到新镜像，而不是第三轮源码或远端镜像缺失。

为排除宿主机 `latest` 缓存、旧容器复用和标签歧义，当前 Git 提交
`2c4ede639e3d80ef7b82e6ac5b5b98c20ea4380c` 已从本机重新执行完整发布门：

- frontend `641/641`、Go 全量和 Vite production build 通过；
- 本地 Docker 镜像中的普通正文在桌面、390×844、360×800、iPad 自适应与强制移动模式
  通过纯黑页面、纯白文字和透明文字承载层检查；
- 真实导入的 EPUB fixture 使用作者 `!important` 白底、渐变、深色文字与阴影；
  1440×900、390×844、360×800 均通过夜间接管和日间原样恢复；
- 新卷 smoke 通过 portable v1/v2 assets、跨用户、重启和备份恢复；历史卷 smoke 通过
  TXT、EPUB、UMD、CBZ、相对缓存和 owner isolation；
- 本地多架构发布完成：
  `ghcr.io/changshengyu/openreader:2c4ede6` 与 `:latest` 共同指向
  `sha256:04ba137d8da8988ae7e2053b6c9051d438f3357ca35df8aad371cd11aced071d`，
  包含 `linux/amd64` 与 `linux/arm64`。

设备验收必须先确认侧栏版本显示 `2c4ede6`。如果仍显示 `a90d10b`，该结果只证明宿主仍在
运行旧容器，不能作为当前夜间实现的复验结论。

## 旧容器再次确认与不可变标签发布（2026-07-28）

用户再次报告“黑色阅读背景下实际文字所在背景仍不是黑色”。通过用户 Chrome 的已登录线上
实例进行只读取证，侧栏版本仍明确显示 `a90d10b`；这正是第三轮修复之前的版本，因此该现象
仍属于宿主没有替换旧容器，不是当前源码重新出现同一回归。

为避免继续依赖宿主对 `latest` 的缓存行为，当前 `59e11a9` 发布前重新执行了实际内容层门禁：

- 普通正文在桌面、390×844、360×800、iPad 自适应和强制移动模式下，shell/page 为纯黑，
  `p/mark/span` 为纯白且自身背景透明；
- 真实 EPUB fixture 在 `main/div/span/table/td` 上故意写入作者 `!important` 白底、渐变、
  深色文字和阴影，1440×900、390×844、360×800 均通过夜间接管和日间原样恢复；
- frontend `643/643`、Go 全量、production build、新卷和历史卷 Docker 门均通过。

本机发布的不可变标签为 `ghcr.io/changshengyu/openreader:59e11a9`，与 `:latest` 共同指向
`sha256:8ce5f345fb376ac13e0b5f80d246a7421c18bb2cf0647039d73298d3255b511b`，
包含 `linux/amd64` 与 `linux/arm64`。本轮设备验收必须先确认侧栏版本为 `59e11a9`；
任何仍显示 `a90d10b` 的页面都仍是旧容器。

## 第四轮实机反馈：黑色自定义夜间方案未进入内容面接管（2026-07-28）

线上 `/api/health` 已确认实际运行 `59e11a9`，用户仍能稳定观察到“阅读背景选择黑色，
实际承载文字的背景却不是黑色”。这次不能再归因于旧容器。

### 固定上游与当前状态

固定上游 `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 的
`vuex.js#setNightTheme` 会加载任意标记为“黑夜默认”的方案；该方案可以是内置主题，也可以是
用户新增的 `theme === "custom"` 方案。`currentThemeConfig` 随后把自定义
`contentColor/contentBGImg` 作为章节内容面。上游没有把“黑夜默认”等同于“内置主题”。

OpenReader 同样允许把任意配置方案标记为“黑夜默认”，但第三轮新增的内容面接管条件是：

```text
themeType === "night" && theme !== "custom"
```

因此存在一个确定的状态分支：自定义方案为 `themeType === "night"`、实际
`customBgColor === "#000000"` 且没有背景图时，`.reader-page` 已经是纯黑，但
`built-in-night` 为 false。普通正文的 `mark/span` 用户代理背景和 EPUB 的作者
`main/div/span/table` 背景便不会被清除，最终形成“黑色页面外层 + 非黑文字承载层”。
现有真实浏览器 fixture 只点击内置月亮入口，没有覆盖可被设为黑夜默认的纯黑自定义方案。

### 第四轮审查矩阵

| 合同层 | 当前行为 | 裁决 |
|---|---|---|
| 接管判定 | 按主题身份 `theme !== "custom"` 判定，而不是按最终渲染面判定。 | `must-fix`：接管条件必须由最终语义与最终表面共同决定。 |
| 纯黑自定义夜间 | 页面可计算为纯黑，但内容后代接管被关闭。 | `must-fix`：`themeType === "night"`、最终页面为纯黑且没有背景图时，普通正文和 EPUB 都必须应用纯黑/纯白内容面合同。 |
| 带背景图或非黑自定义夜间 | 第三轮有意保留用户背景资源和 WCAG 保护。 | `acceptable-change`：不得因本次修复清除自定义图片或把任意非黑自定义方案强制改成黑色。 |
| 内置 dark/black | 已通过 `59e11a9` 的多视口 fixture。 | 保留；改为与纯黑自定义方案共享同一个“黑色夜间内容面”语义。 |
| 测试 | 只覆盖内置方案，无法证明黑色自定义方案。 | `must-fix`：新增自定义黑夜默认和直接自定义纯黑两条真实渲染路径。 |

### 第四轮测试先行门

1. 新增纯函数合同：只有 `themeType === "night"`、最终页面颜色为纯黑且最终页面背景图为
   `none` 时，返回“黑色夜间内容面接管”；主题名不参与判定。
2. 先让旧实现失败于 `theme === "custom" + customBgColor === "#000000"`：
   - 有效正文色必须为 `#ffffff`；
   - Reader 根必须进入统一黑色夜间内容面状态；
   - EPUB bridge 必须收到内容面接管状态。
3. 普通正文 fixture 必须包含带显式浅色背景的 `span/mark`，分别验证：
   - 内置纯黑夜间；
   - 自定义纯黑夜间；
   - 自定义非黑夜间；
   - 自定义背景图夜间。
   前两者必须白字透明面叠加纯黑页面，后两者必须保留用户外观并只执行对比度保护。
4. EPUB fixture 继续使用作者 `!important` 白底/渐变/深色文字，在
   1440×900、390×844、360×800 验证同一四类状态及日间可逆恢复。
5. 本轮不得通过把所有 `themeType === "night"` 无条件改黑来规避状态判断；这会破坏上游
   允许的自定义夜间方案和用户已有背景图数据。

### 第四轮实施与候选验证

- 新增 `isBlackNightReaderSurface()`，以最终 `themeType/pageColor/pageImage` 判定内容面
  所有权，不再以 preset/custom 身份代替实际渲染结果。函数拒绝日间、非黑页面、透明黑和
  任意背景图，只接受不透明纯黑夜间页面。
- Reader 根状态统一为 `black-night-surface`。内置 dark/black 与“纯黑、无图”的自定义
  夜间方案共同接管普通正文、`mark/span` 和 EPUB bridge；纯黑自定义方案中的旧深色
  `fontColor` 也由渲染层改为纯白。
- 自定义背景图夜间和非黑自定义夜间不进入该状态，继续保留用户背景资源与已有 WCAG
  对比度保护。本轮没有改写保存的数据、配置方案或 API。
- 测试先行已证明旧实现因缺少最终表面判定失败。实现后 frontend `644/644`、Go 全量、
  Vite production build、脚本语法和差异检查通过。
- 普通正文真实浏览器门通过桌面 1440×900、手机 390×844/360×800，以及既有 iPad
  自适应/强制手机模式；新增纯黑自定义夜间 fixture 验证页面纯黑、正文/mark/span 纯白且
  自身透明，同时带背景图的自定义夜间明确保持非接管状态。
- 真实 API EPUB 重新导入带作者 `!important` 白底、渐变、深色文字和阴影的 fixture。
  1440×900、390×844、360×800 均验证内置夜间和纯黑自定义夜间接管
  `html/body/main/div/span/table/td`，并保留日间作者样式恢复。
- 实现提交 `3ee3a82438684bb035b60ab4dc08349984e36fa1` 已推送 `main`。本机先构建
  arm64 候选并通过新卷 portable v1/v2 assets、跨用户和重启门；历史卷通过
  TXT/EPUB/UMD/CBZ、相对缓存和 owner isolation。
- 随后从本机生成并发布 `ghcr.io/changshengyu/openreader:3ee3a82` 与 `:latest`。
  两者共同指向 amd64/arm64 OCI index
  `sha256:23454f80db395e45c660e41b9fe5a314936be89a0acbe27eeab0e4761a332f18`。
  当前状态为 **Docker-published / awaiting device verification**。

## 第五轮实机反馈：线上仍在运行修复前镜像（2026-07-28）

用户再次报告：选用黑色阅读背景时，实际文字所在的内容面仍不是黑色。本轮先核对交付链，
不继续假定运行中的静态资源来自当前 Git 或 GHCR `latest`。

### 运行证据

| 检查 | 结果 | 裁决 |
|---|---|---|
| 线上 `GET https://openreader.yuchsh.top/api/health` | `version: 59e11a9`、`commit: 59e11a9a6d2805233745f23960d8364d386a4d62`、`buildDate: 2026-07-28T07:20:43Z`。 | 线上仍运行第四轮修复之前的镜像。 |
| `59e11a9` Reader 内容面判定 | 根 class 与 CSS 仍为 `built-in-night`，只在 `theme !== "custom"` 时接管正文后代。 | 与用户“黑色背景方案的文字承载层仍浅色”现象一致；纯黑自定义/黑夜默认方案仍会漏过。 |
| GHCR `3ee3a82` | amd64/arm64 OCI index `sha256:23454f80db395e45c660e41b9fe5a314936be89a0acbe27eeab0e4761a332f18`。 | 首个包含最终表面判定修复的不可变镜像。 |
| GHCR `342d736` 与 `latest` | 两者均为 amd64/arm64 OCI index `sha256:1643625269f5a04f867c56da9e3bee04c1318d807e73ca6fc0913ab408645921`。 | 当前推荐部署镜像，包含 `3ee3a82` 及后续已验证提交。 |

### 部署验收合同

1. `docker compose up -d` 本身不能证明升级；本机已存在 `latest` 时可能直接复用旧镜像。
2. 标准更新必须先执行 `docker compose pull openreader`，再执行
   `docker compose up -d --force-recreate openreader`。
3. 更新后必须查询站点 `/api/health`；只有返回版本不早于 `3ee3a82` 才能开始本轮设备
   外观验收。推荐直接核对当前不可变版本 `342d736`。
4. 如果健康检查仍返回 `59e11a9`，应检查实际 Compose/面板使用的镜像名、tag、容器 ID 和
   反向代理上游；浏览器强制刷新无法替换后端容器所托管的旧前端资源。
5. 只有运行版本已经升级且同一真实书籍仍复现时，才进入第五轮应用实现修改；届时必须采集
   真实文字节点的 tag/class、computed `background-*`、祖先背景链和书籍格式，不能继续用
   简单 fixture 推断。

### 部署防呆与发布结果

- 仓库 `docker-compose.yml` 已加入 `pull_policy: always`；中英文 README 明确已有部署必须
  先 `docker compose pull openreader`，再
  `docker compose up -d --force-recreate openreader`，最后查询 `/api/health`。
- Compose 配置解析和差异检查通过；frontend `645/645`、Go 全量和 production build 通过。
- TXT Reader 真实浏览器门通过；真实 API 导入的 EPUB 在 1440×900、390×844、360×800
  三个视口通过夜间内容面合同。
- 本地 `9048831` 候选通过 portable v1、portable v2 assets、cross-user、restart，以及
  TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation 历史卷门。
- 本机发布 `ghcr.io/changshengyu/openreader:9048831` 与 `:latest`。两者共同指向
  amd64/arm64 OCI index
  `sha256:f021b995611b961441ed3a2cb0cb06860c41b18c90fd444f6901dcab938fa6f6`；
  amd64 manifest 为
  `sha256:7eecf84d8a7a5d6c1765ab8a493297898e0d6daeb14a359b35fc1345059451ba`，
  arm64 manifest 为
  `sha256:186e9058492a59422ed6c2930b37671d8d4082a6e30cc2b9ecaf9bad1db6bfb3`。
- 当前状态是 **Docker-published / deployed site still requires pull**。不能在站点
  `/api/health` 返回 `9048831` 前声称线上问题已复验关闭。

## 第六轮实机反馈：新镜像已再次发布，线上仍未替换旧容器（2026-07-28）

用户再次确认“黑色阅读背景下，实际文字所在背景仍不是黑色”。本轮禁缓存读取
`https://openreader.yuchsh.top/api/health`，结果仍为 `version: 59e11a9`、
`commit: 59e11a9a6d2805233745f23960d8364d386a4d62`、
`buildDate: 2026-07-28T07:20:43Z`。因此设备当前加载的仍是第四轮最终表面判定修复之前的
静态前端，现象继续与该版本已知缺口一致。

为避免只引用旧发布证据，本轮直接对新候选容器重新执行：

- 普通正文 Reader 在桌面、390×844、360×800、自适应 iPad 和强制移动 iPad 通过
  纯黑 shell/page、纯白正文及透明 `p/mark/span` 内容面合同；
- 真实导入 EPUB 在 1440×900、390×844、360×800 通过：
  `html/body` 为纯黑，作者 `main/div/span/table/td` 的 `!important` 白底/渐变/深色文字
  被可逆接管为透明背景、无背景图和纯白文字；
- frontend `649/649`、Go 全量、production build、新卷 portable v1/v2 assets、
  cross-user、restart，以及历史 TXT/EPUB/UMD/CBZ/relative-cache/owner-isolation 均通过。

本机已发布 `ghcr.io/changshengyu/openreader:a7abcdd` 与 `:latest`，共同指向
amd64/arm64 OCI index
`sha256:93840cf72e9a0a783333ac5ab485551d892e42b9bf2e8eb2e2a1039e56b5dd53`。
设备验收必须先把运行容器替换到不可变标签 `a7abcdd`，并确认站点 `/api/health` 返回
`version: a7abcdd`；在此之前，浏览器刷新或重新选择黑色主题都不会加载到当前修复。

## 第七轮实机反馈：已部署新镜像后仍有移动端浅色正文面（2026-07-28）

线上禁缓存健康检查已经返回 `version: a7abcdd`。通过用户 Chrome 的真实登录会话检查：

- 普通章节 `book 384` 在桌面及 390×844 移动视口，工具栏切换后的
  `.reader-shell/.reader-page` 为纯黑，当前可见正文为纯白；
- 真实本地 EPUB `book 393` 在桌面及 390×844 移动视口，iframe、`html/body` 为纯黑，
  当前章节后代为纯白且无浅色背景；
- 由此排除“线上仍是旧容器”和“所有章节都会失败”，剩余问题是设备持久化状态或真实
  EPUB/富文本结构中未覆盖的分支。

### 新增缺口与裁决

| 合同层 | 当前行为 | 裁决 |
|---|---|---|
| 黑色内容面判定 | `isBlackNightReaderSurface()` 同时要求最终页面纯黑、无图和 `themeType === "night"`。旧移动端配置或黑色自定义方案若昼夜语义未同步，实际页面虽黑却不接管正文。 | `must-fix`：内容面所有权只由最终不透明纯黑页面和无背景图决定，不能继续信任可能陈旧的昼夜标签。 |
| 普通正文结构面 | `.reader-content/.reader-body/.chapter-content` 自身透明，只依赖 `.reader-page` 在更下层提供黑色。 | `must-fix`：进入黑色内容面时，实际承载正文的结构层也必须显式为纯黑，避免移动 WebKit/合成层显示旧背景。 |
| EPUB 根伪元素 | 已接管 `html/body` 和 `body *`，但 `html::before/after`、`body::before/after` 不在选择器内。作者样式可用固定伪元素覆盖整个黑色根面。 | `must-fix`：黑色内容面必须清除四个根伪元素的背景、背景图和阴影，并在退出后恢复作者样式。 |
| 非黑自定义主题/背景图 | 不属于纯黑内容面。 | 保留已有行为，不清除用户图片或任意非黑自定义主题。 |

### 第七轮测试先行门

1. 纯函数新增“`themeType: day` + 最终纯黑无图”仍进入黑色内容面；非黑、透明黑和带图继续拒绝。
2. 静态合同要求 `.reader-content/.reader-body/.chapter-content` 在黑色内容面显式
   `background-color: #000000`。
3. EPUB fixture 新增作者 `body::before` 固定白底和渐变；黑色内容面必须清除，退出后必须
   恢复。
4. 普通正文和真实 API EPUB 均增加 390×844 的“纯黑页面但持久化 `themeType: day`”
   回归路径。

### 第七轮实施与候选验证

- `isBlackNightReaderSurface()` 改为只读取最终页面颜色和背景图：不透明纯黑且无图即进入
  黑色内容面，不再依赖可能陈旧的 `themeType`；透明黑、非黑页面和任意背景图仍不接管。
- 普通正文的 `.reader-content/.reader-body/.chapter-content` 在黑色内容面下显式设为
  `#000000`，默认文字显式设为 `#ffffff`，后代浅色背景继续清为透明，避免移动 WebKit
  合成层露出旧背景。
- EPUB bridge 在黑色内容面下同时接管 `html/body` 和
  `html::before/after`、`body::before/after`，清除作者固定白底、渐变、阴影和动画；退出
  黑色内容面后不修改原 EPUB，作者日间样式可恢复。
- 测试先行已证明旧实现失败于“最终页面纯黑但 `themeType: day`”及结构层未显式为黑色。
  实现后 frontend `649/649`、Go 全量、Vite production build、脚本语法与差异检查通过。
- 普通正文真实浏览器门通过 1440×900、390×844、360×800，并通过既有 1024×1366、
  1366×1024 iPad 自适应及强制移动模式；新增 390×844 陈旧昼夜标签路径通过。
- 真实 API 重新导入含作者固定 `body::before` 白底/渐变及多层 `!important` 浅色面的
  EPUB。1440×900、390×844、360×800 均验证根面纯黑、文字纯白、作者浅色承载层透明，
  390×844 陈旧昼夜标签路径和退出夜间后的作者样式恢复也通过。
- 实现提交 `b78d39c4cb8bed0a7dfc8ec1f0f69dbf243309c8` 已推送 `main`。本地 arm64 候选
  通过新卷 portable v1/v2 assets、cross-user、restart；历史卷通过
  TXT/EPUB/UMD/CBZ、relative-cache 和 owner isolation。
- 本机已发布 `ghcr.io/changshengyu/openreader:b78d39c` 与 `:latest`，共同指向
  amd64/arm64 OCI index
  `sha256:edcb5ec1247a21d7b439239163df41efb54335af3c68730374ee47f4f7f2e679`；
  amd64 manifest 为
  `sha256:547d03d5c9df70b02081e09015ebb0847c9fcf2a8c8edd8c8928c003f937bf4e`，
  arm64 manifest 为
  `sha256:5599c546e4e109d38ad7139bac7f8892f820ece2f3956322fa2c9c15b7b835c2`。
  当前状态为 **Docker-published / awaiting device verification**。

## 第八轮实机反馈：文字节点不能继续依赖透明穿透（2026-07-29）

线上禁缓存健康检查已经返回 `version: b78d39c`，首页也以 `cache-control: no-cache`
提供当前资源指纹。通过用户 Chrome 登录会话在 390×844 复验普通章节 `book 384`：

- `.reader-shell/.reader-page/.reader-content/.reader-body/.chapter-content` 均已计算为
  `rgb(0, 0, 0)`；
- 正文 `p`、内层 `mark/span` 仍按第七轮合同计算为
  `rgba(0, 0, 0, 0)`，其黑色观感完全依赖祖先表面的透明穿透；
- EPUB bridge 同样把 `main/div/span/table/td` 的作者背景改为透明，只把
  `html/body` 本身设为黑色。

桌面 Chromium 能正确合成这条透明链，但用户移动设备仍观察到“外层已经黑色，实际文字
所在背景不是黑色”。这证明第七轮“清除浅色背景后保持透明”的验收条件不足以覆盖移动
WebKit、浏览器强制深色和 EPUB 独立合成层。

### 第八轮裁决

| 合同层 | 第七轮行为 | 第八轮裁决 |
|---|---|---|
| 普通正文文字块 | `p[data-reader-block]` 及其 `span/mark` 为透明，依赖 `.chapter-content` 黑底。 | `must-fix`：纯黑内容面下，文字块和其文本后代自身必须计算为 `#000000`，不能依赖祖先透明穿透。 |
| EPUB 文字节点 | `html/body` 为黑，其余节点为透明。 | `must-fix`：纯黑内容面下，EPUB 后代也必须由 bridge 写入 `background-color: #000000 !important`；退出夜间后仍完整恢复作者样式。 |
| 图片、漫画和自定义背景 | 当前会清除 CSS 背景，但不覆盖 `<img>` 像素。 | 保留：节点黑底只能位于实际图像像素之后；非黑自定义主题和任意背景图继续不进入纯黑内容面。 |
| 根伪元素 | 透明且背景图已清除。 | 保留透明，避免固定伪元素自身形成额外黑色遮罩；其下方的 `html/body` 已明确纯黑。 |

### 第八轮测试先行门

1. 静态合同必须拒绝普通正文 `data-reader-block` 继续使用透明背景。
2. 普通 Reader fixture 的 `p/mark/span` 在 1440×900、390×844、360×800 均必须计算为
   `rgb(0, 0, 0)`，文字为 `rgb(255, 255, 255)`；切回日间后作者 `mark` 背景必须恢复。
3. 真实 API EPUB fixture 的 `main/card/text/table/cell` 在相同视口均必须计算为纯黑；
   切回日间后作者白底、渐变和深色文字必须恢复。
4. 纯黑自定义方案以及昼夜标签陈旧但最终表面纯黑的路径必须执行同一合同；非黑自定义
   方案和背景图不得被黑化。
5. 不改写用户保存的 `fontColor`、主题方案或 EPUB 文件，只强化纯黑内容面的渲染所有权。

### 第八轮实施与候选验证

- 普通正文 `data-reader-block` 及其文本后代在纯黑内容面下改为显式
  `background-color: #000000 !important`；纯白文字、无背景图和无阴影合同保持不变。
- EPUB 注入样式与后端 bridge 同时把实际后代节点写为纯黑；bridge 继续保存原 inline
  value/priority，退出纯黑内容面后恢复 EPUB 作者白底、渐变、阴影和深色文字。
- 新增 `SMOKE_NIGHT_ONLY=1` 浏览器入口，使夜间内容面可独立于其它正在复审的 Reader
  overlay 运行，仍覆盖内置黑夜、纯黑自定义黑夜和昼夜标签陈旧的纯黑自定义方案。
- 失败测试先证明第七轮透明合同不能满足本轮要求；实现后 frontend **652/652**、Go
  全量和 Vite production build 通过。
- 普通 Reader 在 1440×900、390×844、360×800 验证 `p/mark/span` 均为纯黑背景、
  纯白文字，并验证切回日间恢复作者标记面；纯黑自定义方案及陈旧昼夜标签路径同样通过。
- 真实本地 Go API 导入的 EPUB 在上述三个视口验证
  `html/body/main/card/text/table/cell` 均为纯黑、纯白文字和无背景图，并验证切回日间
  完整恢复作者外观。
- 全量 `reader-mobile-contract` 已到达独立的 Bookmark 跳转流程后失败；这是正在进行的
  Bookmark 第二轮上游复审门，不是夜间内容面失败。本轮用新增的聚焦入口隔离证明夜间
  合同，Bookmark 浏览器门会在恢复该模块时继续处理。

### 第八轮 Docker 发布结果（2026-08-02）

- 实现提交 `6fde5abf3c5e9507c0a82d5f221b68674b318cce` 已推送 `main`；镜像从该提交的
  干净 detached worktree 本地构建，未包含工作区中尚未完成的 Bookmark 复审改动。
- 已发布 `ghcr.io/changshengyu/openreader:6fde5ab` 与 `:latest`，两者共同指向
  amd64/arm64 OCI index
  `sha256:4ace53462bc85e5f05e4441a5d250edd96f5b72d406b0ab109830501a129c4c0`。
- amd64 manifest 为
  `sha256:4d87fcbdad834f75a30dac5a030dafd14823d0a5134da69776790dbfa7f33a23`；
  arm64 manifest 为
  `sha256:c975509039663632ff69b87f92df42086b75c17a9668f5edfdeea32a1ba2c6ea`。
- 同一干净提交的本地 arm64 候选已通过 portable v1、portable v2 assets、
  cross-user、restart，以及历史 TXT/EPUB/UMD/CBZ、relative-cache、owner-isolation
  全部持久化门。
- `docker buildx imagetools inspect` 能够连续读取上述两个 tag 及其同一 digest；
  当次本机 Docker daemon 的 `docker pull` 路径三次收到 GHCR `502 Bad Gateway`，
  因此卷门使用同提交本地加载镜像完成。该外部 registry 拉取抖动不改变已发布
  manifest 和本地验证结果，但设备部署时仍应以实际 `docker compose pull` 成功为准。
