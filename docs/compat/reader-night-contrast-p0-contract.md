# Reader 移动夜间模式正文对比度合同

状态：2026-07-28 已按固定上游完成差异提取、测试先行实现、全量自动回归和
390×844 / 360×800 的 TXT、EPUB 真实浏览器验证；Git 与 Docker 发布门待执行。

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
4. 本问题与当前未提交的登录/书源浏览器补门一起提交推送；通过后再恢复 Docker 旧卷和发布门。

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
