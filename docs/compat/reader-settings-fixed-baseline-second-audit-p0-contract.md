# ReaderSettings 第二轮固定基准复审与重建合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

审计日期：2026-08-02。

状态：**implementation-complete / Docker-pending**。本文件最初由只读源码审查生成；审查合同以
`609a7a1` 单独提交后，已按“失败测试 → store/面板实现 → 全量与真实浏览器”顺序完成重建。
历史 ReaderSettings 实施记录仍只能说明做过什么，本节末尾的新证据才是本轮签收依据。

## 1. 权威源码与当前映射

上游权威：

- `web/src/components/ReadSettings.vue`：设置标题、可见顺序、控件、提示、动作和局部状态机。
- `web/src/plugins/config.js`：白天/黑夜默认方案、八个主题、五个字体、可同步方案字段和 TTS
  独立配置。
- `web/src/plugins/vuex.js#setConfig/setNightTheme/setCustomConfigList`：活动方案同步、自动昼夜、
  mini 判定和本地持久化。
- `web/src/views/Reader.vue`：ReadSettings Popover、设置变更前的位置捕获，以及独立的 TTS/read bar。
- `web/src/App.vue#autoSetTheme`：启用自动主题后立即套用当前系统昼夜状态。

当前映射：

- `frontend/src/components/reader/ReaderSettingsPanel.vue`、`ReaderSettingStepper.vue`。
- `frontend/src/stores/reader.js`、`utils/readerFonts.js`、`utils/readerThemeType.js`。
- `frontend/src/views/Reader.vue`、`composables/useReaderMode.js`、`useReaderPanels.js`、
  `useReaderAppearanceAssets.js`。
- `ReaderMobileWorkspacePanel.vue`、`ReaderDesktopWorkspacePanel.vue`、`ReaderTTSBar.vue`。
- `PUT/GET /api/settings/reader`、Pinia 本地持久化和设置同步事件。

## 2. 外壳与可见顺序合同

| 项目 | 固定上游行为 | 当前行为 | 裁决 |
|---|---|---|---|
| 标题和滚动宿主 | `设置 / 重置为默认配置` 是 `.setting-list` 的前置兄弟；标题 `18px/22px/400`、下距 28px，只有列表 `max-height:45vh; overflow-y:auto`。 | 已采用同一结构和几何；移动/桌面外壳把 24px inset 交给面板。 | `aligned technical equivalent`；保留。 |
| 行几何 | 所有行均为 56px 标签 + 16px 间距，控件在同一行；行间距 20px。 | 桌面和移动均已是 72px 控件起点，行间距 20px。 | `aligned`；继续以真实像素断言，不只做源码正则。 |
| 分隔线 | 特殊模式后、操作区前各有一条 divider。 | 两条均缺失。 | `must-fix`。 |
| 核心顺序 | 特殊模式 → 配置方案 → 方案类型 → 阅读主题 → 自定义块 → 正文字体 → 简繁转换 → 字体大小 → 字体粗细 → 段落行高 → 段落间距 → 字体颜色 → 页面模式 → 桌面页面宽度 → 翻页方式 → 动画时长 → 自动翻页 → 条件滚动像素 → 翻页速度 → 全屏点击 → 选择文字 → 操作区。 | 大体同序，但混入亮度、字体预览、字号预设、字体颜色局部重置和三项 TTS；“自动翻页”写成“自动阅读”。 | 按下表逐项处理，不能以“大体同序”签收。 |
| 操作区 | divider 后无左侧行标签；两端红色文本动作为“显示翻页区域”“过滤规则管理”。两者先关闭设置，再打开目标。 | 用“替换规则”左标签和两个 Element 按钮；第二项为“管理全局替换规则”。底层 handler 已先关闭设置。 | `must-fix` 可见结构/文案；保留已对齐的关闭顺序。 |
| 工具层并存 | 移动 ReadSettings 打开不隐藏 Reader 工具层；面板点击不穿透；再次点设置或切到其它主工具关闭/切换。 | 当前主面板状态机已满足。 | `aligned`；本批不得回归。 |
| iPad | 1024×1366、1366×1024 使用可关闭的桌面工作区，不进入覆盖整屏且无法关闭的手机面板。 | 已有响应式门与外部点击关闭合同。 | `aligned`；作为发布回归门保留。 |

## 3. 允许保留的用户定制与必须删除的重复项

| 当前额外项 | 裁决 | 最终合同 |
|---|---|---|
| 减号 / 可编辑数字 / 加号 | `intentional-redesign` | 用户明确要求，所有数值项继续使用同一 stepper；点击中间数字可输入，Enter/失焦提交、Escape 取消。 |
| 亮度 | `intentional-redesign` | 用户明确要求保留；位于主题与字体之间，范围 50–150、步进 5，中间值可编辑。渲染继续使用无事件遮罩，不能把 `filter` 放回滚动祖先。 |
| 字体预览与字号预设 | `intentional-redesign` | 用户给出的目标设置截图明确包含预览和 14/16/18/20/22/24/28/32 预设；保留，但不得改变五个上游字体槽或数值输入能力。 |
| 纯黑夜间面 | `intentional-redesign` | 用户明确要求黑色背景 + 白色文字；继续覆盖上游 `#171717/#666` 的低对比夜间值，并保持普通正文与 EPUB 实际文字节点一致。 |
| 字体颜色“恢复默认”按钮 | `must-fix` | 删除额外按钮；颜色选择器与标题级“重置为默认配置”是权威入口。 |
| 设置底部 TTS 语速/音调/语音 | `must-fix` | 删除重复行和 ReaderSettings 的 TTS props/events。上游及当前均已有独立 `ReaderTTSBar`，语音、语速、音调、定时继续由该栏持有并持久化。 |
| OTF/WOFF/WOFF2 安全上传 | `acceptable-change` | 上游 UI 只接受 TTF；当前后端已对四种字体做真实签名校验和用户资产隔离。可继续接受四种格式，但可见字体槽和上传/恢复状态机必须复刻上游。 |

原生手指/滚轮连续滚动而点击/键盘仍离散分页，是另一个既有用户定制；本批设置重建不得改变
该运行时边界。动画提示可以保留一行，明确数值只影响离散定位。

## 4. 主题、字体与背景合同

| 项目 | 固定上游行为 | 当前行为 | 裁决 |
|---|---|---|---|
| 主题入口 | 八个 34×34 圆形主题，顺序 0…7；第 6 项是月亮；其后是 78×34 的“自定义”。 | 八个可见语义 key 与几何已等价，另有隐藏 `black` 兼容 key。 | `aligned`：保留语义 key 和隐藏 legacy alias；主题圆点颜色应恢复上游八项 rgba。 |
| 主题实际表面 | 0/1/2/3/5/6 分别使用固定上游 body/content/popup 纹理，4/7 使用固定颜色。 | 所有非自定义白天主题共用一张 base64 纸纹，只替换底色。 | `must-fix`：引入固定基准主题资源并按八项映射；第 6 夜间最终仍应用用户要求的纯黑/纯白覆盖。 |
| 自定义块 | 只在 custom 显示；一行左标签“自定义”，内部依次为主题模式、三种颜色、背景图。 | 结构已接近。 | `aligned`，保留语义 `themeType`、安全色彩对比和纯文本绑定。 |
| 内置背景图 | 依次显示 `山水画、山水墨影、羊皮纸1、护眼漫绿、羊皮纸2、新羊皮纸、羊皮纸3、明媚倾城、羊皮纸4、深宫魅影、午后沙滩、清新时光、宁静夜色、边彩画布` 共 14 张 36×36 预览。 | 只显示用户上传列表，14 张入口和资源都缺失。 | `must-fix`：复制固定基准资源并恢复顺序；内置 URL 不进入用户上传删除 API。 |
| 背景选中 | 点击任一已选背景会取消；点击其它背景切换。删除当前自定义图后回到第一张内置背景。 | 切换逻辑已等价；删除当前图会清空。 | `must-fix` 删除 fallback；保留上传的事务、CAS、账号隔离和安全清理。 |
| 字体槽 | 只显示系统、黑体、楷体、宋体、仿宋五项，上传图标位于右上；已上传后点同一图标可选择继续上传或恢复默认。 | 可见槽已精确为五项；legacy mono 只用于读取旧值。但已上传字体另加第二个恢复图标。 | `must-fix`：恢复单图标状态机；继续支持 legacy mono 读取但不得作为第六预设。 |
| 默认日间文字 | `#262626`。 | 空字符串回退到 `#24282c`。 | `must-fix`：新默认与重置恢复 `#262626`；已有显式用户颜色不得迁移覆盖。 |

## 5. 配置方案状态机

当前最大偏差不在样式，而在方案数据所有权。

| 动作 | 固定上游状态机 | 当前状态机 | 裁决 |
|---|---|---|---|
| 活动方案同步 | 只把 `config.js#defaultDayConfig` 的字段加 `contentBGImg` 同步回当前方案；`pageType`、`autoTheme`、TTS 和资产清单不属于方案。 | `readerConfigSnapshot()` 从完整 reader settings 复制，方案包含 pageType、brightness、TTS、customFontsMap、customBgImageList 等。 | `must-fix`：建立显式 scheme field allowlist。亮度是唯一允许加入方案的 OpenReader 字段。 |
| 选择方案 | 在当前全局 config 上覆盖方案字段并设置方案名；不改特殊模式、自动切换、TTS 或上传资产清单。 | sanitize 后整包 assign，会改 pageType/TTS，并可能用空 map/list 清掉当前字体/背景资产清单。 | `must-fix`；不得丢用户资产引用。 |
| 新增方案 | trim 名称、拒绝空/重复；复制第一个“内置白天”，追加后保持当前方案和当前阅读外观不变。 | 复制当前设置并立即切到新方案，成功文案还声称“已保存当前配置”。 | `must-fix`：复制内置白天、不自动选中、不显示错误文案。 |
| 删除方案 | 内置、越界和当前活动方案不可删；确认后只删目标。 | 核心限制已等价。 | `aligned`；恢复上游确认语义即可。 |
| 方案类型 | 确认文案明确“将替换现有的白天/黑夜默认方案”；同类型只能一个。 | 唯一性已等价，但确认文案省略替换警告。 | `must-fix` 文案。 |
| 自动切换 | 切换为 true 后立即根据系统主题选择相应默认方案；false 后停止跟随。 | App watcher 已等价。 | `aligned`；账号设置加载完成前仍不得用本地旧账号主题覆盖。 |
| 重置 | `config = settings.config`：恢复内置白天活动配置，但 `customConfigList` 独立保留。TTS 独立配置也不变。 | `Object.assign(defaultReaderSettings())` 会重建并清空全部自定义方案，且重置 TTS 和资产清单。 | `must-fix P0 data-loss`：保留自定义方案、TTS、上传字体/背景清单和文件；清除当前自定义选择并恢复日间基础值、亮度 100。 |

方案字段必须至少覆盖上游：主题/主题类型、字体选择、简繁、字号、字重、字体颜色、body/content/
popup 自定义颜色、阅读方式、点击方式、动画时长、页面宽度、行高、段距、自动翻页方式、滚动像素、
翻页速度、页面模式、选择文字和当前背景图。OpenReader 的 `brightness` 可作为明确用户扩展加入。

`customFontsMap`、`customBgImageList` 是全局资产清单；`ttsRate/ttsPitch/ttsVoiceURI` 是独立 TTS
配置；`pageType` 和 `autoTheme` 是全局模式。四类字段都不得被方案切换覆盖。

## 6. 正常 / 简洁模式状态机

固定上游在两套模式之间各自持久保存最近配置：

1. 正常 → 简洁：保存完整正常配置；读取最近简洁配置。若没有，使用 `animate=0`、
   `fontSize<=20`、纯白主题、左右滑动、忽略选择文字、手机模式的默认简洁配置；不强改全屏点击。
2. 简洁 → 正常：保存完整简洁配置；读取最近正常配置；两套最近配置跨组件重建/刷新继续存在。
3. 变更前先捕获当前可见段落，mode/pageMode/排版共同进入一个 generation 化恢复事务。

当前只保存一个运行时 `normalModeSnapshot`；进入简洁总是重建硬编码默认，简洁内的调整不会在下次
恢复；刷新后也无法回到上一次正常配置。`setAnimateDuration()` 还在简洁模式强制值永远为 0，
面板则禁用输入，而固定上游仍显示并允许调整该值。

裁决为 `must-fix`：持久保存 normal/kindle 两套最近配置，切换顺序和默认值复刻上游；不禁用动画
控件，不把 `clickMethod` 改成 `none`。位置事务和当前 Vue 3 有效模式适配继续保留。

## 7. 数值、默认值与输入边界

| 字段 | 固定上游 | 当前 | 最终裁决 |
|---|---|---|---|
| fontSize | 默认 18，min 8，步进 1，无产品 max。 | max 36。 | `must-fix`：移除 36 截断；有限数且 min 8。 |
| fontWeight | 400，100…900，步进 100。 | 等价。 | 保留。 |
| lineHeight | 1.8，1…5，步进 0.2。 | 等价。 | 保留。 |
| paragraphSpace | 0.2，0…5，步进 0.2。 | 等价。 | 保留。 |
| animateMSTime | 300，0…500，步进 50。 | 数值等价，但简洁强制/禁用。 | 恢复可编辑；运行时离散动画继续直接读取下一次动作的值。 |
| autoReadingPixel | 1，min 1，步进 5，无产品 max。 | max 80。 | `must-fix`：移除 80 截断。 |
| autoReadingLineTime | 1000，min 10，步进 50，无产品 max。 | max 3000。 | `must-fix`：移除 3000 截断。 |
| readWidth | min `min(floor(viewport/160),4)*160`，max `floor(viewport/160)*160`，步进 160；仅非 mini 显示。 | 固定 UI 480…1120，store 320…1200。 | `must-fix`：恢复随视口计算并继续使用可编辑中值。 |
| brightness | 上游无。 | 100，50…150，步进 5。 | `intentional-redesign`，保留。 |

无上游 max 的字段仍必须拒绝 NaN/Infinity；这属于输入有效性，不得用一个更小的任意上限悄悄
改写已保存数据。`ReaderSettingStepper` 因此需要支持可选 max。

## 8. API、数据和迁移合同

本批不新增 REST 路由、不迁移 SQLite、不改变 `data/cache/library`：

- 保留按用户隔离的 `GET/PUT /api/settings/reader`、CAS 时间戳、认证 generation、WebSocket 排队
  和本地未登录设置；方案重建不能退回全局匿名配置。
- 读旧 settings 时接受当前语义 theme key、legacy `mono`、现有完整 custom config 和旧
  `normalModeSnapshot`。迁移只重新划分字段所有权，不删除自定义方案或上传资产。
- 新版本方案序列化只写 allowlist；读取旧方案时忽略其中 pageType、autoTheme、TTS 和资产清单
  对活动全局状态的覆盖，但保留这些全局值本身。
- normal/kindle 最近配置需要进入 reader settings JSON 并经过账号 scope；不得使用跨用户的裸
  localStorage key。旧数据没有快照时按固定上游默认安全初始化。
- 重置、方案切换和默认昼夜不得触发文件删除。显式删除背景/恢复字体继续走现有设置先保存、
  目标引用校验、服务端 409 引用保护和用户根校验。
- 14 张内置背景和主题纹理是只读前端资源，不写入上传目录，也不进入 portable backup manifest。

如需提升 `settingsVersion`，只能做非破坏迁移；必须使用 `data-migration-compat` 门验证旧 JSON、
portable v1/v2 assets、历史挂载卷和账号隔离。

## 9. 必须先替换/增加的测试

1. `readerSettingsPanelContract.test.mjs` 增加完整核心顺序、两条 divider、精确“自动翻页”和操作区
   文案；删除把额外 TTS/字体颜色重置视为可保留的可能性，同时明确保留亮度、预览、字号预设。
2. 增加 14 张内置背景顺序/资源、选中取消、删除自定义图回退第一张内置图，以及内置图不调用
   DELETE 的合同。
3. 字体合同锁定五个可见槽、legacy mono 隐藏兼容、单上传图标和“继续上传/恢复默认”分支。
4. store 单元测试先证明：新增方案复制内置白天且不激活；方案切换不改 pageType/autoTheme/TTS/
   资产清单；活动方案只同步 allowlist；重置不删除方案或资产。
5. normal/kindle 单元和持久化测试覆盖 A→B→刷新→A、两边各自修改、默认简洁字段、clickMethod
   不变、动画可编辑和位置事务只结算最新 generation。
6. 数值测试覆盖无上限字段、动态 readWidth、NaN/Infinity 拒绝和点击中值编辑；已有 brightness
   87 真实浏览器断言保留。
7. 删除 ReaderSettings 内 TTS 行后，`reader-tts-contract` 必须证明独立 TTS bar 的语音/语速/
   音调/定时和持久值均未丢失。

## 10. 真实浏览器与发布门禁

必须覆盖 1440×900、390×844、360×800、1024×1366、1366×1024：

- 设置打开时工具层仍显示；标题固定、列表独立滚动、点击不穿透、再次点击/切工具可关闭或切换。
- 两个分隔线、核心顺序、八主题、14 内置背景、五字体、单上传图标、精确操作区和无重复 TTS。
- 亮度中值可编辑；字体预览/字号预设保留；其它 stepper 中值同样可编辑且无任意截断。
- 新增/选择/删除/设默认/自动昼夜/重置，以及 normal↔kindle 跨刷新状态机。
- 主题 body/content/popup 资源逐项生效；夜间仍是用户要求的纯黑页面和白字，普通正文与真实 EPUB
  都检查实际文字节点。
- 设置导致 mode、pageMode、字体、宽度或主题变化后保持当前可见段落；手指/滚轮仍原生连续，
  点击/键盘动画 0/100/500ms 继续可区分。
- iPad 工作区不覆盖整个上半屏且存在关闭路径；所有视口无横向溢出、控制台异常或 Chrome 退出。

常规门禁为 frontend 全量、Go 全量、Vite production build、`git diff --check`。发布候选另跑本机
Docker、新卷 portable v1/v2 assets/cross-user/restart，以及历史 TXT/EPUB/UMD/CBZ、
relative-cache/owner-isolation。通过后可作为 ReaderSettings 半模块 Docker 验收切片发布。

## 11. 受控实施顺序

1. 先提交本合同，不修改应用代码。
2. 使用 `frontend-ux-compat`、`openreader-frontend` 和 `data-migration-compat` 增加失败测试。
3. 先修 store 的方案 allowlist、无损重置、normal/kindle 双快照和数值边界；再改面板结构、资源和
   精确文案，避免用 UI 掩盖数据错误。
4. 运行单元/构建后做五视口浏览器验证；若状态与视觉切片均完整，即使 Reader 其它模块尚未开始
   下一轮，也可本地构建并发布 Docker 供设备验证。

## 12. 重建结果（2026-08-02）

- 方案采用显式 allowlist；新增方案复制第一项内置白天且不激活，旧方案中的 `pageType`、
  `autoTheme`、TTS 和全局资产清单不会再覆盖活动全局状态。
- 重置恢复内置白天，但保留全部自定义方案、字体/背景资产清单和 TTS 配置；删除当前自定义背景
  后回到第一张内置背景，不触碰文件删除以外的持久资源。
- normal/kindle 两套最近配置进入既有 per-user reader JSON，并兼容读取旧 `normalModeSnapshot`；
  Kindle 动画值不再被 setter/normalize 强制归零，当前全屏点击选择保持不变。
- 面板恢复两条 divider、固定上游核心顺序、精确操作区、14 张内置背景、五字体单上传图标和
  “继续上传/恢复默认”分支；ReaderSettings 内重复 TTS 行及字体颜色局部重置已删除。
- body/content/popup 使用固定基准独立纹理，粉色/纯白仍为纯色；用户要求的纯黑页面、白字和
  无夜间纹理覆盖继续优先。数值 stepper 支持上游无产品 max 字段与动态页面宽度边界。

验证证据：frontend `680/680`、Go 全量、Vite production build、`git diff --check` 通过；真实
浏览器覆盖 1440×900、390×844、360×800、1024×1366、1366×1024，并额外覆盖 1024×1366
强制手机模式。工具层并存、面板关闭/切换、固定标题与列表滚动、亮度 87 直接输入、背景预览、
纯黑夜间正文、iPad 关闭路径、无横向溢出和无控制台异常均通过。Docker 新旧卷门与 GHCR 发布
在代码提交后执行，完成前状态保持 `Docker-pending`。
