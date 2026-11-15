# 连续阅读窗口中，目录选中非当前失败章

2026-11-15 盘点。OpenReader 基线 `4c1f286b`，上游基线 `fa22f271849d45f93349ae1636223e27b16a4691`。依据 [Reader 失败态进度合同第 11 节第 6 项](reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md) 单独审查本入口。以下为源码证据与拟定验收，尚未执行本切片的浏览器复现。

## 行为与依据

上游 `web/src/components/PopCatalog.vue#gotoChapter` 从目录数组求出所选行的 index，先关闭目录再发出 getContent。Reader.vue 第 34 行直接接到 getContent，第 998 行起把阅读书籍 index 改为该行、回到顶部、从目录取出真实 chapterIndex 并调用 getBookContent。此调用没有传入强制刷新参数，实际网络读取取决于内容缓存；目录选择不会因为正文窗口已经有对应 DOM 就跳过加载。旧请求完成时检查 bookUrl 和 index。

| 场景 | 当前行为及证据 | 判定 |
| --- | --- | --- |
| 连续模式目录点击已呈现错误的非当前块 | useReaderToc.jump 关闭目录，当前失败章分支未命中。useReaderNavigation.goChapter 按 data-index 找到块，重建窗口并滚入它；此分支没有调用 retry。错误块的存在不能证明正文已加载。 | 差异，待裁决 |
| 目录点击主加载失败的当前章 | useReaderToc.jump 走 reloadCurrentChapter，已有 P6 和真实服务合同。 | 已签收 |
| 正文内点错误块的重试按钮 | Reader.vue 的 retryContinuousChapter 调 chapterWindow.retry，原位更新，支持失败、恢复及换书后丢弃旧结果。 | 已签收 |
| 目录点击健康相邻章、分页与 flip、连续滚动经过错误块 | 前者沿用窗口跳转，后者沿用各自导航与进度合同。D2 已签收滚进下一章的进度行为。 | 本切片保留 |

## 裁决范围与测试

仅裁决用户主动从目录选择已在当前书连续窗口内的非当前失败章。方案 A 按上游把目录选择视为一次加载：关闭目录，尝试一次目标正文，导航到所选章；再次失败时可继续重试，成功时显示目标正文。方案 B 保留跳至错误块，由用户按块内重试。数据格式、API 和原 P6/D2 均沿用。此盘点不把“所有目录点击都强制联网”列为要求。

若选 A，先给目录组合函数增加失败相邻块与健康块、当前失败章的对照；结合窗口 controller 现有的失败重试、跨书迟到结果用例验证请求归属。在真实服务构造第二章相邻加载失败，保留第一章为当前章；用目录选第二章，在源仍失败和已恢复两组场景分别数请求，验证每次点击发起一次目标加载，恢复后正文及进度均指向同书第二行。检查 scroll、scroll2 与桌面、移动视口。旧合同未能通过原生滚动把最后一个短错误块变成顶部可见块，本案用目录选择驱动，测试不能用操作滚动位置代替目录点击。

现有 readerChapterWindowController.test.mjs 验证块内重试与换书后丢弃迟到结果；reader-failure-progress-contract.mjs 验证当前失败章目录重试。上述两类证据均未覆盖此次目录入口。

实施默认采用方案 A。失败块的目录点击与当前失败章目录点击共用目标章加载入口。沿用 D2 的连续滚动与进度规则，保留健康块目录跳转。验收覆盖源仍失败和已恢复两种结果。
