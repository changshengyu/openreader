# Reader iPad / 宽屏触控面板固定基准第二轮合同（P0）

审查日期：2026-08-09

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**inventory-complete / current-source-aligned / regression-rerun-pending**

本合同重新核对 iPad Pro 自适应场景、显式手机模式和 Reader 主面板关闭路径，并取代总矩阵中
“宽屏 iPad 仍自动进入手机面板”的过期结论。历史合同
[`reader-ipad-responsive-p0-contract.md`](reader-ipad-responsive-p0-contract.md) 和
[`reader-ipad-panel-dismiss-p0-contract.md`](reader-ipad-panel-dismiss-p0-contract.md) 继续保留实施历史，
但当前状态和后续发布判断以本文件为准。

## 1. 固定上游权威行为

权威文件：

- `web/src/plugins/helper.js`：`isMiniInterface()` 只判断 `window.innerWidth <= 750`，不读取 UA、
  `navigator.platform` 或触摸点数；
- `web/src/App.vue`：启动和 `window.onresize` 均重新提交宽度判断；
- `web/src/plugins/vuex.js#setMiniInterface`：`pageMode === "自适应"` 使用宽度判断，其它页面模式
  显式强制 mini；
- `web/src/views/Reader.vue`：宽屏自适应场景保留桌面左右工具和 click-triggered Popover；mini
  场景才把工具改为上下横条，并用全屏 Popover 根几何；
- Element UI click Popover 允许再次点击当前 reference 或点击外部关闭。Reader 的正文事件处理在
  任一主 Popover 打开时直接返回，不得触发翻页或工具层切换。

因此固定合同为：

| 场景 | Reader 结构 | 主面板关闭路径 |
|---|---|---|
| 自适应 751/1024/1366px | 桌面左右工具、桌面进度、非全宽主 Popover | 同工具、外点 |
| 自适应 750/390/360px | 手机顶部/底部工具、全宽主 Popover | 同工具、外点 |
| 显式手机模式 1024/1366px | 完整手机结构，不混挂桌面工具 | 同工具、外点 |
| 旋转/分屏跨越 750px | 按当前宽度切换一次完整场景 | 不得同时残留两套工具或两套面板 |

上游没有面板内部独立关闭按钮。OpenReader 保留共享 44×44 可见关闭按钮，判定为已明确记录的
触控可用性增强；它只能补充同工具/外点路径，不能替代或改变上游状态机。

## 2. 当前源码映射与判定

| 合同层 | 当前文件 / 行为 | 第二轮判定 |
|---|---|---|
| 共享响应 predicate | `frontend/src/utils/responsive.js`：`isMobileLikeViewport(width)` 仅为 `width <= 750`；`shouldUseMiniInterface` 仅在显式 `pageMode === 'mobile'` 时强制手机结构。 | `aligned technical equivalent` |
| 共享消费者 | Reader、Home、AppLayout、Search/Discover、BookInfo、BookEdit、GlobalOverlayHost 和 SourceManager 都调用 `shouldUseMiniInterface`。 | `aligned`；不得重新引入 UA/iPad 分支 |
| Reader 单一场景 | `Reader.vue#isMobileReader` 同时控制根 `mini-interface` class、桌面工具/进度和手机 chrome；`desktopWorkspacePanel` 在手机场景为空。 | `aligned` |
| 主面板单选/同工具关闭 | `openDesktopToolPanel` 对活动面板先统一关闭；再点活动工具直接关闭，A→B 先清空再打开。 | `aligned` |
| 外点关闭 | `ReaderDesktopWorkspacePanel` 的全屏透明 dismiss button 在 `pointerdown/click` 停止传播并只 emit `close`。 | `aligned technical equivalent` |
| 可见关闭 | 同一共享组件拥有一个 44×44、固定在面板右上角、不可滚出视口的关闭按钮。 | `intentional usability improvement` |
| 桌面层级 | ReaderClickZones `z2`、dismiss `z3`、workspace `z4`、左右工具 `z5`。 | `aligned`：外点不能翻页，工具仍可切换 |
| 内容边界 | shelf/source/toc 为 300px 内滚动；settings 由内部 `.settings-list` 维护 `45vh` 滚动；外壳 `max-height:100dvh`。 | `aligned` |
| 手机层级 | 手机主面板、dismiss 和工具条仍为一套独立层级；桌面修复没有 iPad 专用 CSS。 | `aligned / must-preserve` |

本轮源码审查没有发现新的 `must-fix`。总矩阵此前把 `39a5244` 之前的 UA 驱动状态继续写成
“当前错误”，属于审计记录漂移；不能据此重复修改已经对齐的 shared predicate。

## 3. 当前测试覆盖审查

- `frontend/tests/readerIPadWorkspaceContract.test.mjs` 锁定 750/751 边界、iPad UA 不参与判断、
  1024/1366 自适应桌面和宽屏显式手机模式；并检查所有共享消费者；
- `frontend/tests/readerDesktopPrimaryPopoverContract.test.mjs` 锁定桌面 Popover 根几何、300px/45vh
  内部边界、44×44 关闭按钮、外点事件消费和 z2→z5 层级；
- `scripts/smoke/reader-mobile-contract.mjs` 已包含 1024×1366、1366×1024 iPad UA + touch，逐个
  打开书架/书源/目录/设置并验证可见关闭、外点关闭、同工具关闭、无翻页/滚动/路由副作用；同时
  覆盖 1024×1366 显式手机模式与 390×844、360×800 手机场景。

这些测试的断言范围足够，但历史绿色结果不能证明当前 `main`。本合同之后必须重新运行：

1. 两个专项 Node test；
2. frontend 全量测试和 production build；
3. 当前构建上的 `reader-mobile-contract.mjs` 五视口；
4. 若应用代码无需修改，仍要记录本次无代码复审证据；不为了制造版本号重复改 UI；
5. 若运行暴露偏差，重新进入“失败测试 → 实现 → 全量回归 → 本地 Docker”流程。

## 4. 发布判定

本轮在浏览器复验前不发布 Docker。若当前实现全部通过且没有应用代码变化，复审只更新合同与总
矩阵，沿用已包含实现的最新镜像；若测试或浏览器暴露真实回归，则修复后按本地双架构流程发布，
并记录 tag、OCI digest、新旧卷门禁和真实设备待验项。
