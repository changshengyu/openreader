# Reader 日间/夜间：按设备跟随系统与内置夜间配色合同（P1）

状态：**aligned / Docker-published / awaiting-device-verification**（发布见文末；记录见第 7 节）。审查日期：2026-10-29。审查基线：`OpenReader@b4cf2be`。

## 1. 来源与用户决定

用户 2026-10-29 要求“修改日间和夜间阅读的逻辑，向上游原项目对齐”，并在问答中确认：

- 实际遇到的问题是“其它设备被带着切换”：一台设备切换或打开页面后，另一台设备的日/夜跟着变；
- 内置夜间配色**改回上游**：暗纹理背景 + `#666` 灰字，自定义方案不受影响。

第二条取代 [`reader-night-contrast-p0-contract.md`](reader-night-contrast-p0-contract.md) 第二至五轮与
[`reader-settings-fixed-baseline-second-audit-p0-contract.md`](reader-settings-fixed-baseline-second-audit-p0-contract.md)
第 3 节“纯黑夜间面”这一 `intentional-redesign` 中**内置夜间**的部分；那几轮为“纯黑/自定义纯黑”建立的内容面接管在自定义方案上继续保留。

## 2. 固定上游

`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`：

- `web/src/plugins/vuex.js`：`state.config` 只来自本浏览器 `localStorage`（`syncFromLocalStorage`、`setConfig`、
  `setNightTheme`）；`isNight = config.themeType === "night"`。
- `web/src/App.vue#beforeCreate/created/autoSetTheme/setTheme`：`beforeCreate` 先读本地配置，`created` 立即按
  `prefers-color-scheme` 调 `setNightTheme`（`autoTheme` 为真时），并监听系统深浅色变化；`autoTheme` 变为真时立即套用。
- `web/src/views/Reader.vue#toogleNight`、`web/src/views/Index.vue#toogleNight`：手动切换只调用 `setNightTheme(!isNight)`，
  不关闭自动切换。
- `web/src/plugins/config.js`：`defaultNightConfig` 为 `theme: 6`、`fontColor: "#666666"`、`bodyColor: "#121212"`、
  `contentColor: "#171717"`、`popupColor: "#121212"`；`themes[6]` 为 `body: url(body_6) repeat`、
  `content: url(content_6) repeat`、`popup: "#121212"`。
- `web/src/components/Content.vue#containerStyle`：正文色为 `config.fontColor || (isNight ? "#666" : "#262626")`，无对比度替换；
  EPUB `setStyle` 只把同一颜色注入 `body p`，不清除出版物背景。
- `web/src/views/Reader.vue` `.night` 样式：`.chapter` 边框 `1px solid #444`。

上游没有跨设备设置同步：配置只能经首页“备份/恢复用户配置”手动搬运。日/夜因此天然是**每台设备各自**的状态，
自动切换只看本机系统深浅色。

## 3. 当前 OpenReader（`b4cf2be`）

- `frontend/src/stores/reader.js`：阅读设置整体是账号级 JSON（`GET/PUT /api/settings/reader`），写入后服务端广播
  `settings_update`，其它设备 `reconcileReaderSettingsUpdate → loadReaderSettings → applyReaderSettings` 整包套用。
  `applyAutoTheme → setNightTheme → markSettingsDirty`，与手动切换一样会在 700ms 后写服务端。
- `frontend/src/App.vue`：登录时先 `await loadReaderSettings()` 再按系统套用自动主题；系统深浅色变化与 `autoTheme`
  变化时再次套用。收到其它设备的设置广播后不再按本机系统判定。
- `themePresets.dark`：`bg #000000 / text #ffffff / body #000000 / popup #171717`；内置黑夜方案 `fontColor: '#ffffff'`。
- `utils/readerThemeContrast.js#resolveReaderSurface`：所有非 custom 夜间返回无纹理纯黑页面；`isBlackNightReaderSurface`
  使内置夜间进入 `black-night-surface`，正文强制 `#ffffff` 并接管 EPUB 后代背景。`resolveReaderTextColor` 对所有主题做
  4.5:1 对比度替换，`#666` 放在暗纹理上会被换成白色。

## 4. 差异与裁决

### 4.1 日/夜的设备范围

| 编号 | 场景 | 上游 | 当前 | 裁决 |
| --- | --- | --- | --- | --- |
| N1 | 自动切换（启动、系统深浅色变化、打开自动切换）套用默认方案 | 只改本设备 | 写账号设置并广播；其它设备整包套用后变成同一日/夜 | **must-fix**：自动套用只在本设备生效，不写服务端、不广播 |
| N2 | 收到账号设置（其它设备广播、手动同步、冲突回读）且本设备开着自动切换 | 不存在跨设备写入；本设备始终跟随自己的系统 | 直接采用别的设备的日/夜，直到本机系统再变化或重载 | **must-fix**：套用账号设置后按本机系统重新选择白天/黑夜默认方案（仍只在本设备生效） |
| N3 | 每次打开/刷新页面 | 本地读配置后即套用，不产生写入 | 只要开着自动切换，每次启动都写一次阅读设置并广播（N1 的直接后果），正在阅读的其它设备随之重新套用设置 | **must-fix**（随 N1 关闭） |
| N4 | 启动时首屏 | `created` 里同步套用，首屏即是系统对应的日/夜 | 先显示本地上次的日/夜，等服务端读取完成后才跟随系统 | **must-fix**：本地开着自动切换时启动即按系统在本设备套用，服务端读取后再按 N2 复核 |
| N5 | 手动点月亮/太阳 | 只改本设备，不关闭自动切换 | 写账号设置；不关闭自动切换 | **acceptable-change**：账号级阅读设置是已签收的多设备适配。修复 N2 后，开着自动切换的其它设备会按各自系统复核，不会被带着切换；关闭自动切换时日/夜与其它阅读设置一样账号内一致 |
| N6 | “自动切换”开关本身 | 每个浏览器各自一份 | 账号级 | **acceptable-change**（同上，不重开） |
| N7 | 没有标记为“黑夜默认/白天默认”的方案 | 弹出“未配置黑夜默认方案”，不切换 | 回退内置方案 | **acceptable-change**：2026-06-02 前后保存的旧方案可能没有类型标记，沿用夜间 P0 合同的回退，避免旧数据无法切换 |

N2 的复核只发生在“开着自动切换且浏览器能报告深浅色”时；关闭自动切换或无 `matchMedia` 时照常采用账号设置。
复核只换白天/黑夜默认方案（与上游 `setNightTheme` 相同的完整方案），不改 `autoTheme`、不写服务端、不改
`settingsUpdatedAt`；之后本设备若有真正的设置修改，照常以当前本地状态保存。

### 4.2 内置夜间配色

| 编号 | 项目 | 上游 | 当前 | 裁决 |
| --- | --- | --- | --- | --- |
| P1 | 内置夜间（`dark`，上游主题 6）外层/正文面/浮窗 | `body_6`、`content_6` 纹理；浮窗 `#121212` | 纯黑无纹理；浮窗 `#171717` | **must-fix**：外层 `#121212` + `body_6`，正文面 `#171717` + `content_6`，浮窗 `#121212` 无纹理（两张纹理平均色为 `#101113`、`#17191a`，底色取上游 `defaultNightConfig` 的同名颜色） |
| P2 | 内置黑夜方案默认字体色 | `#666666` | `#ffffff` | **must-fix** |
| P3 | 内置夜间的有效正文色 | `fontColor`，空值回落 `#666`，不做对比度替换 | 强制 `#ffffff`；对比度不足 4.5 时替换 | **must-fix**：`dark` 主题使用方案字体色，空值用 `#666`，不替换 |
| P4 | 内置夜间 EPUB | 只注入同一正文色 | 进入纯黑接管：后代背景强制黑、文字强制白 | **must-fix**：`dark` 主题不进入纯黑接管，EPUB 与普通正文共用 P3 的颜色；iframe 根层继续绘制当前页面底色（`#171717`）以免透明画布显示成白色（已签收的技术修正） |
| P5 | 内置夜间页面边框/阴影 | `.chapter` 边框 `#444` | 透明、无阴影 | **must-fix**：边框 `#444`，无日间纸张阴影 |
| P6 | 已保存的旧默认值 | — | 2026-08-16 起内置黑夜方案保存为 `#ffffff`（之前为空） | **must-fix 数据规范化**：设置版本 14→15 时，名为“内置黑夜”的内置方案、以及正处于该方案的当前设置/正常与简洁最近配置，若 `theme: 'dark'` 且字体色恰为 `#ffffff`，改为 `#666666`；空值保留（已回落 `#666`）；其它颜色、自定义方案不动 |

**不受影响（不重开）**：`custom` 主题（颜色、背景图、4.5:1 对比度保护、纯黑无图时的内容面接管）；隐藏的旧 `black`
兼容主题（继续纯黑白）；全部日间主题；Reader 工具栏/设置面板的夜间控件色（`#303030`、`#565656`、`#ff7589`、`#7ed8d4`）
与首页/弹窗的夜间样式——上游对应为 `#666` 图标/`#444` 边框/`#ed4259` 强调色，这次用户只确认了阅读背景与正文色，
控件色如需一并改回另起切片。

## 5. 目标行为

1. `setNightTheme(isNight, { deviceLocal })`：选择方案与套用方式不变；`deviceLocal` 为真时只改本地状态，
   不同步服务端、不更新 `settingsUpdatedAt`。`applyAutoTheme` 一律以 `deviceLocal` 调用；手动入口不变。
2. 新增“按本设备系统复核”：`autoTheme` 为真且浏览器可报告 `prefers-color-scheme` 时调用 `applyAutoTheme(系统深色)`。
   `applyReaderSettings` 在套用账号设置后执行；App 启动时（已登录）在等待服务端之前先执行一次。
3. P1–P5 只作用于 `theme === 'dark'`；`black-night-surface` 判定保留“页面纯黑且无背景图”的规则，因此 `dark` 不再命中，
   纯黑无图的自定义夜间和 `black` 照旧命中。
4. P6 在本地持久化状态（`normalizeSettings`）与服务端载荷（`sanitizeReaderSettings`，以载荷自身版本判断）两处执行一次。

## 6. 计划测试（先红）

`frontend/tests`（node --test，`authenticatedRuntimeScope` 同款 Pinia/Vite 夹具）：

1. 自动套用黑夜后 700ms 内不调用 `PUT /settings/reader`，`settingsUpdatedAt` 不变；手动 `setNightTheme(true)` 仍会保存（对照）。
2. 本设备系统为浅色、开着自动切换：收到另一设备保存的“内置黑夜”账号设置后仍是内置白天；系统为深色时收到白天设置后仍是黑夜；
   关闭自动切换时照常采用账号设置（对照）。
3. App 源码合同：已登录启动在 `loadReaderSettings` 之前按系统套用一次。
4. `themePresets.dark`、内置黑夜方案、`resolveReaderSurface({theme:'dark'})`、`resolveReaderTextColor` 断言 P1–P5 的确切值；
   custom 低对比度仍被替换、纯黑自定义夜间仍接管（对照）。
5. P6：版本 14 的本地状态与服务端载荷里 `#ffffff` 变为 `#666666`；空值、其它颜色、自定义方案不变；版本 15 载荷里的 `#ffffff`
   不再被改（用户新选择）。
6. 与新决定冲突的旧断言（内置夜间纯黑/纯白）按 CLAUDE.md 改写为新合同，不保留。

浏览器：1440×900、390×844、360×800 打开 TXT 与 EPUB，点月亮后读取 `.reader-shell`/`.reader-page`/段落/EPUB `body p`
的 computed style，确认纹理、`#171717` 底色与 `rgb(102, 102, 102)` 正文；两个浏览器上下文登录同一账号、分别模拟深/浅色，
一侧切换或刷新后另一侧日/夜不变。

## 7. 记录（2026-10-29）

提交（分支 `claude/day-night-import-alignment`）：合同 `a2a3eb7`；红测 `1feb7da`；实现 `07f4752`；补充红测 `d1188ce`；
补充实现 `02cefe7`；浏览器合同改写 `936f672`。

- 红测：`1feb7da` 在 `d87bb2d` 上 4 个文件中 9 项失败、43 项通过——N1（自动套用改了 `settingsUpdatedAt` 并写账号）、
  N2（浅色设备收到“内置黑夜”后照单全收）、N4（启动先等服务端）、P1–P5（纯黑白配色与纯黑接管）、P6（`#ffffff` 不迁移）；
  手动切换仍保存、关闭自动切换照常采用账号设置、自定义低对比度替换与纯黑接管、隐藏 `black`、版本 15 的白色与空值等对照通过。
  与新决定冲突的三处纯黑断言按 CLAUDE.md 改写。`07f4752` 后同一断言全部转绿。
- 实现中细化 N2（`d1188ce` 红 2 项 → `02cefe7`）：准备双设备浏览器检查时发现，开着自动切换在 A 上手动点月亮后，B 只要保存任何设置
  （例如字号），A 收到广播复核时就按系统改回白天。上游的手动选择会一直保持到刷新、系统深浅色变化或重新打开自动切换
  （`App.vue#autoSetTheme` 只在 `created`、`matchMedia change` 与 `autoTheme` watcher 触发）。现在本设备的手动选择只在本页会话内记住
  （不持久、不同步），复核时优先使用；系统深浅色变化（`followSystemColorSchemeChange`）、重新打开自动切换、换账号或刷新后结束。
- 测试：frontend 827/827 与 build 通过；同分支 backend `go test ./...` 29 个包、`go vet ./...`、`go test -race`（engine、services/localbook、services/importstage，以及 api 1037 秒）在 `7a18346` 上通过（其后只改前端与冒烟脚本），未见 DATA RACE。
- 真实浏览器（本地 Go 服务 + `aeaccf0` 构建，新数据目录）：1440×900、390×844、360×800 打开 TXT，日间为 `content_0`/`rgb(38, 38, 38)`；
  点月亮后 `.reader-shell` 为 `rgb(18, 18, 18)` + `body_6.png`、`.reader-page` 为 `rgb(23, 23, 23)` + `content_6.png`、正文
  `rgb(102, 102, 102)`、桌面页边框 `rgb(68, 68, 68)`，没有 `black-night-surface`。同一账号两个浏览器上下文（A 浅色 1440×900、B 深色
  390×844，账号设置已存在）：打开与刷新都不写阅读设置（A 0、B 0 次 PUT），A 白天、B 夜间互不影响；A 手动月亮写入 1 次、B 仍夜间；
  B 手动太阳写入 1 次、A 保持手动夜间；A 刷新后回到跟随系统的白天，B 保持手动白天。改写后的 `reader-epub-contract`（三视口，
  内置夜间 EPUB 根层 `#171717`/`#666`、出版物样式保留；自定义纯黑接管段不变）、`reader-mobile-contract`（桌面、手机、iPad 两种）
  与 `bookshelf-visible-layout-contract` 通过，无页面错误、控制台错误或 5xx。
- 仍保留的差异：N5–N7（见第 4.1 节）；Reader 工具栏/设置面板与首页的夜间控件色未改回上游（`#303030`/`#565656`/`#ff7589`/`#7ed8d4`），
  如需一并对齐另起切片。

状态：**implementation-complete / regression-validated**，待可信 Actions；镜像未发布；生产未访问。

## 发布（2026-10-29）

main 快进到 `b3164e3`（ls-remote 核验，未强推）；可信 Actions run `40834553001` 全部成功（07:18–07:43Z）。`ghcr.io/changshengyu/openreader:b3164e3` 与 `latest` 为 OCI index `sha256:39d3d1cc3c9b55171fbe2aeef53270d37f65c55b6eed370a52e1831e31da0ea1`（linux/amd64 `sha256:a8ed9027…`、linux/arm64 `sha256:46a98c99…`），两平台 `org.opencontainers.image.revision` 均为 `b3164e3d91888714d3748a95c9b7264c2f2c79a3`。状态 **aligned / Docker-published / awaiting-device-verification**；生产未访问，实际运行版本未知，需在生产升级容器后实机确认。
