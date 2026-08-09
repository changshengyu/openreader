# Reader 移动端阅读内书架宽度合同（P0）

审查日期：2026-08-09

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**aligned / Docker-published / awaiting-device-verification**

本合同只处理 Reader 顶部“书架”按钮打开的阅读内书架，不改变 Index 首页普通书架。普通书架在
`7971e23` 已恢复移动书籍行等于视口宽度；两者是不同 DOM、CSS 和验收边界。

## 1. 固定上游最终几何

上游权威文件：

- `web/src/views/Reader.vue`：移动 `popperWidth` 初值为 `windowWidth - 33`，但最终由全局 mini CSS 覆盖；
- `web/src/App.vue`：`.mini-interface .popper-component` 固定 `left:0`、`width:100vw !important`、
  `box-sizing:border-box`、`margin:0`，并由 `.popper-component.el-popover` 去除 border；
- Element UI 2 的 `.el-popover` 内容 padding 为 `12px`；
- `web/src/components/BookShelf.vue`：根 `.popup-wrapper` 使用左右 `margin:-16px` 和 `padding:24px`，
  书架列表及条目均为 `width:100%`。

因此上游最终可见书架内容的左右 inset 不是 24px，而是：

```text
12px（Popover padding）- 16px（负 margin）+ 24px（BookShelf padding）= 20px
```

最终合同：

| 视口 | Popover 根宽 | 标题/列表左右 inset | 列表/书籍条目宽 |
|---|---:|---:|---:|
| 390px | 390px | 20px / 20px | 350px |
| 360px | 360px | 20px / 20px | 320px |

工具层显隐、主面板高度 300px、滚动定位和点击所有权不参与宽度计算，也不得因为宽度修复而改变。

## 2. 当前偏差

当前 `frontend/src/components/reader/ReaderMobileWorkspacePanel.vue` 已保持主面板根 `100vw`；但
`frontend/src/views/Reader.vue` 的 `.reader-mobile-primary-popover-body` 直接使用左右各 `24px`
padding，失去了上游外层 Popover padding 与负 margin 的抵消语义。最终列表宽度为：

- 390px：`390 - 24 - 24 = 342px`，比上游窄 8px；
- 360px：`360 - 24 - 24 = 312px`，比上游窄 8px。

`scripts/smoke/reader-mobile-contract.mjs` 目前读取当前 computed padding 后再计算 expected width，
只会证明“列表填满当前错误内容盒”，不能发现 inset 从 20px 漂移到 24px，判定为错误测试假设。

## 3. 先测后改

1. 静态合同锁定移动主 Popover 的水平 padding 为 `20px`，不得再接受任意当前值。
2. 真实浏览器在 390×844、360×800 直接断言：根宽等于视口，内容左/右 inset 均为 20px，
   列表和首个书籍条目分别为 350/320px。
3. 保留工具层默认显示、打开书架后工具层不隐藏、顶部工具高于面板、面板不穿透、同按钮关闭、
   书架列表 300px 高及当前书籍自动定位合同。
4. 同时复跑 Index 普通书架合同，确认其移动书籍行仍为 390/360px，不把两种书架宽度混为一谈。

实施仅调整 Reader 移动主 Popover 的水平内容 inset 和对应测试，不修改书架数据、排序、刷新、进度、
API、SQLite、缓存或持久化格式。

## 4. 实施与验证结果

- `Reader.vue` 的移动主 Popover 只把水平 padding 从 24px 恢复为 20px；纵向 safe-area、300px
  列表高度、工具层层级、点击关闭和正文几何未修改。
- 静态合同先在旧实现稳定失败，修复后通过。真实浏览器在 390×844、360×800 分别得到 350px、
  320px 的列表/单列书籍条目；根层和内容层保持 `100vw`，左右 inset 均为 20px。
- 1024×1366 强制手机模式继续按上游 `min-width:900px` 四列规则渲染：984px 列表中每卡 228px；
  测试不再把手机单列假设错误套到宽屏强制模式。
- Reader 完整浏览器合同通过桌面、双手机、1024×1366/1366×1024 自适应 iPad 和 1024×1366
  强制手机模式；Index 普通书架在 1440×900、1024×1366、390×844、360×800 继续保持固定网格、
  390/360px 移动整行宽度、加载/空态/夜间合同。
- frontend 706/706、production build、全量 Go 和 `git diff --check` 通过。

## 5. Docker 发布结果

实现提交 `515160960996d6c63159871e1f7b20a6a6c8d1ae` 已推送 `main`。镜像只使用本机 OrbStack
构建并直接上传 GHCR，没有使用云端构建：

- `ghcr.io/changshengyu/openreader:5151609`
- `ghcr.io/changshengyu/openreader:latest`
- OCI index：`sha256:d3110429a422e092832afde3b7780d6a3c193c01316c5e251c7c6ba8cd85f23c`
- amd64：`sha256:98365bb846817b34747cd565b4f502a26546af48eb909400bda6efd43e3e18e8`
- arm64：`sha256:79d342d85db6cef9c55d346031d6abdd879525c7a680ea70776f6caded7e2822`

候选镜像通过 fresh volume 的 portable v1/v2 assets、cross-user、restart，以及 historical volume
的 TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation。当前等待真实设备确认 20px inset 与上游
视觉一致；本批没有允许的 UI 差异，也没有数据/API/持久化变更。

## 6. 设备反馈与线上部署核验（2026-08-09）

设备再次反馈“移动端阅读书架明显窄”。对 `https://openreader.yuchsh.top` 的已登录线上页面做 390px
只读测量后确认：站点版本按钮仍显示 `7971e23`；首页普通书架行宽为 390px，但 Reader 内书架内容仍
是旧实现的左右 24px、列表 342px。它与当前 `main` 的左右 20px、列表 350px 差异完全一致，因此
本次反馈判定为 **部署版本滞后**，不是 `5151609` 之后源码再次回归。

包含该宽度修复和 P2-N2 网络策略的 `d198c2e` 已重新由本机发布为同名标签与 `latest`，OCI index 为
`sha256:021817e602aa589c1583ec7ccb65828172c1a2afe1e038e23651dd51c455fcc1`。线上容器必须拉取并重建到
`d198c2e`（或该 digest）后再做设备验收；仅执行容器 restart 不会自动替换旧镜像。

随后本机发布的 `e7f168e` / `latest` 继续包含相同 20px 修复，并通过 fresh/historical volume 门；
其 OCI index 为 `sha256:8d64bbb187f65c433388bddc5385ce68d42e8b40d9b397787e4c1d354c892dac`。当前部署应直接更新到
`e7f168e`（或该 digest），无需先部署中间镜像。

本轮又在当前 `main@2ea6e8c` 对完整 Reader 合同和普通书架合同重新做真实浏览器验证：Reader 内书架
在 390×844、360×800 仍精确保持 20px/350px 与 20px/320px，普通书架保持 390/360px 整行宽度。
该提交已由本机发布为 `2ea6e8c` 与 `latest`，OCI index 为
`sha256:678b019c34ac1f92a38dbd650de48867002ae6425a4206aff2e8f315e189d6ac`。线上仍需 pull 并 force
recreate 到该版本后才能验收；旧容器 restart 不能更新镜像内容。

2026-08-09 设备再次反馈移动书架偏窄时，线上 `/api/health` 仍明确返回 `7971e23`。当前
`main@77a60d8` 重新生产构建后，完整 Reader 合同继续在 390×844、360×800 得到 20px/350px 与
20px/320px；普通首页书架合同在 390×844、360×800 继续得到 390/360px 整行宽度。该提交已由
本机发布为 `77a60d8`/`latest`，OCI index 为
`sha256:a1a37b223e10a3c43febd23250dd7790394c200d69e7c9548255cf1fdba3b017`。因此当前反馈仍判定为线上
容器滞后；服务器必须 pull 并 force recreate，不能只 restart。

随后线上 `/api/health` 已于同日确认运行 `77a60d8`，不再属于旧镜像；当前 `43635a1`/`latest`
继续包含完全相同的 Reader 内 20px/350px、20px/320px 合同，OCI index 为
`sha256:0f75a0434d209af901cde81f86127f8e62fa78d6cb3610d6c10ef2e0863053c0`。设备仍反馈“明显窄”，
因此本项不再以“部署滞后”关闭，而改为 **device-feedback-open**。在取得实际设备可见层证据前不得
随意把 20px 改成 0：需要先区分 Index 普通书架（书籍行应为 390/360px）与 Reader 顶栏书架
Popover（列表应为 350/320px），再对照截图逐层测量视口、根层、内容层和列表层。

## 7. 真实账号复现与第二个结构偏差（2026-08-09）

使用真实账号和 389 本书在当前线上 `77a60d8` 的 390×844 视口重新逐层测量后，确认横向几何
本身仍是：Popover 390px、左右 20px、列表/条目 350px；首页普通书架仍为 390px 整行。设备所见的
“明显变窄”不是这四个横向数值再次回归，而是阅读内书架的纵向轨道被压缩后形成的错误视觉结果：

- `.reader-shelf-list` 固定高 300px 且使用 CSS Grid；
- 移动分支没有锁定 `align-content:start`、内容高度行轨和 16px 行间距；
- 当书架有 389 项时，浏览器把自动行轨压进 300px，首个 `.reader-shelf-card` 实测仅 17px 高；
- 标题本身高 22px，章节行被压为 0px，文字溢出并互相覆盖，截图表现为所有内容挤在一条窄带内；
- 原浏览器合同只返回一本书且只断言横向宽度，因此无法暴露多书架下的轨道坍缩。

固定上游 `BookShelf.vue` 的权威结构不是可收缩的单列 Grid：移动端 `.shelfbook-list` 为纵向 flex，
行间距 16px，每项包含 8px 上下 padding、16px 标题、12px 未读数和 14px 章节；300px 高度属于可
滚动视口，不得拿来平均压缩所有条目。`min-width:900px` 时才切四列、24px 列间距。

本次 must-fix 合同：

1. 维持 20px 横向 inset，不用删除 padding 掩盖真正问题；
2. 移动单列条目按内容高度排列、行间距 16px，章节行可见，首项高度不得小于 60px；
3. 任意书架数量不得改变单项高度，多余项目只在 300px 列表内纵向滚动；
4. `min-width:900px` 继续保持上游四列，每行同样按内容高度排列；
5. 浏览器契约必须至少提供 12 本书，并同时断言条目高度、标题/章节高度、行间距和列表
   `scrollHeight > clientHeight`，不能再用单书架假数据验收。

实施顺序保持“合同 → 失败测试 → CSS 修复 → 双手机真实浏览器 → 全量回归”。本节确认的是此前
宽度专项遗漏的第二个结构错误，不能把先前的 `automated-upstream-geometry-aligned` 视为设备验收。

## 8. 行轨修复与回归结果（2026-08-09）

- 浏览器合同先在旧 CSS 上稳定失败：390px 视口首个条目高度只有 25px；该失败与真实账号 389 本书
  时实测的 17px/章节 0px 属于同一行轨坍缩。
- `ReaderShelfPanel.vue` 给滚动列表增加 `grid-auto-rows:max-content`、`align-content:start` 和上游
  16px 行间距。单项不再按书架总数压缩，300px 仅作为滚动视口。
- 合同数据扩展为 12 本书，并锁定首项至少 60px、标题至少 20px、章节至少 16px、相邻行间距
  16px、`scrollHeight > clientHeight`；原有 20px/350px 与 20px/320px 横向合同继续保留。
- 开发构建与生产构建的完整 Reader 浏览器合同均通过 1440×900、390×844、360×800、
  1024×1366/1366×1024 自适应 iPad 和 1024×1366 强制移动模式。
- 首页普通书架独立回归通过 1440×900、1024×1366、390×844、360×800，移动书籍行继续保持
  390/360px 整行宽度；本批没有混改 Index 书架。
- frontend 713/713、production build、全量 Go 和 `git diff --check` 通过。

本批没有 API、SQLite、缓存、进度、书架排序或数据迁移变化；允许差异仍只有项目既有的连续滚动与
数值 stepper，本次行轨修复本身无上游可见差异。Docker 已发布，真实设备复验仍待完成。

## 9. Docker 发布结果（2026-08-09）

实现提交 `a445121d263342a297e10f3c1304fcb6dacfed22` 已推送 `main`，并只使用本机 OrbStack 完成
amd64/arm64 构建和 GHCR 上传：

- `ghcr.io/changshengyu/openreader:a445121`
- `ghcr.io/changshengyu/openreader:latest`
- OCI index：`sha256:79c56060dc0101f0d5bc07f09ae4cb02b7b1a5618681eb08c13bd2fcbe7f6238`
- amd64：`sha256:5d014f8f8fdfbe7ba9ca27d0eee53e5dd0e19c83b010655c9de820e9b14c66a5`
- arm64：`sha256:8caed99599fa33cda01bc2750f4a86a55cb4a702f06c3dec43950423322fac3d`

## 10. 当前发布与线上版本核验（2026-08-09）

完整 Reader 浏览器合同已在当前生产构建重新通过桌面、390×844、360×800、自适应 iPad 与强制
移动 iPad；横向 20px/350px、20px/320px 和多书内容高度行轨均未回归。包含上述修复的最新本机构建
镜像为 `f8f263d`/`latest`，OCI index
`sha256:9c83821de9e5f4df223b6e69a6d67eff512fa55d4a271f544718ccad8ae58ba1`。

同一时间线上 `https://openreader.yuchsh.top/api/health` 仍返回 `77a60d8`，它早于 `a445121` 行轨
修复。因此本次设备看到的“明显窄”不能用线上旧容器否定当前源码合同；部署端必须 pull
`f8f263d`（或上述 digest）并 force recreate，确认 health 返回 `f8f263d` 后再复验。只 restart
现有容器不会拉取新镜像。

候选镜像通过 fresh volume 的 portable v1/v2 assets、cross-user、restart，以及 historical volume
的 TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation。当前仅待服务器 pull/force recreate
到该镜像后进行真实移动设备签收；仅重启旧容器不会更新镜像。

## 11. `f8f263d` 线上真实账号复测（2026-08-09）

线上 `/api/health` 已确认运行 `f8f263d`，旧镜像因素已经排除。随后复用真实登录账号，在
390×844 Chromium 视口分别测量首页普通书架和 Reader 顶部“书架”面板：

- 首页 `.book-list` 与每本 `.book-row` 均为 390px，移动行没有被父容器或侧栏压窄；
- Reader 面板根 `.reader-mobile-workspace` 为 390px，内部左右 padding 均为 20px；
- `.reader-shelf-list` 与 `.reader-shelf-card` 均为 350px，左右 inset 对称；
- 389 本书时列表保持 300px 可滚动视口，`scrollHeight=31483px`；首项高 65px、标题高 22px、
  章节行高 20px，`grid-auto-rows:max-content`、`align-content:start` 和 16px gap 均已生效。

这组线上权威 DOM 证据与固定上游合同一致，因此本次“明显窄”的设备反馈继续保持 open，但不再
直接归类为已知水平 padding 或行轨回归。下一步需要设备截图区分：用户指的是首页普通书架、Reader
内书架，还是浏览器缩放/安全区造成的可见层变化。在取得该证据前不把 20px 擅自改为 0，也不以
扩大内容区的方式制造新的上游偏差。

## 12. 同版本第二次线上可见层复核（2026-08-09）

用户再次反馈“移动端阅读书架明显窄”后，确认线上 `/api/health` 已是 `f8f263d`，并在真实登录账号的
390×844 Chrome 视口重新测量首页普通书架：`visualViewport.scale=1`，`body`、`.app-shell`、
`.app-workspace`、`.app-content`、`.shelf-page`、`.books-wrapper`、`.book-list` 和 `.book-row`
均为 390px；侧栏宽 260px 且完全位于 `x=-260px`，没有占用工作区；单行仍为左右 20px padding，
封面 84px、封面与信息区间距 20px。该结果再次排除了旧镜像、浏览器缩放、侧栏残留占位及首页
外层宽度回归。

因此本轮不改应用 CSS：当前源码与线上数值都符合固定上游，盲目删除 20px/24px 上游留白会制造
新的偏差。设备反馈继续标记为 **device-feedback-open**；下一份有效证据必须是问题设备的完整屏幕
截图，并注明是首页“书架”还是 Reader 顶部“书架”面板，以便继续定位浏览器 UI、安全区或尚未被
合同覆盖的具体内层。
