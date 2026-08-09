# Reader 移动端阅读内书架宽度合同（P0）

审查日期：2026-08-09

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**aligned / Docker-republished in `e7f168e` / deployment-update-required**

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
