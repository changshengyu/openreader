# Reader 移动夜间模式正文对比度合同

状态：2026-07-28 第一轮已发布，但用户实机复验确认目标定义错误：达到 WCAG 对比度不等于
用户要求的“纯黑正文背景 + 白色正文”。第二轮已完成固定上游/当前渲染层复审、测试先行实现、
后端与前端全量回归、production build，以及 390×844、360×800 的 TXT/EPUB 实际渲染层
验证。实现提交 `a90d10b` 已推送并通过本地 Docker 新卷、历史卷、备份恢复门，GHCR
`a90d10b` 与 `latest` 已发布。

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
