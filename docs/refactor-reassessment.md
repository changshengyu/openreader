# OpenReader 历史重构重新审查

审查基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

## 1. 新标准

- 每批先读取 fork 中对应的页面、组件、方法和样式，再决定当前代码如何实现。
- 当前实现、此前重构组件和既有测试都不是必须保留的骨架；与上游职责或交互冲突时直接删除或替换。
- Vue 3、Pinia、Go REST、多用户隔离和浏览器缓存可以作为技术实现差异保留，但不能改变上游可见信息架构和操作链。
- 只有用户明确要求的优化或上游无法满足当前运行环境的适配，才在上游行为之上增加；必须在审计记录中单独标明。
- “测试通过”只证明被测试行为没有回退，不证明上游对齐完成；每批必须同时提供源码映射和真实桌面/移动界面证据。

## 2. 历史风险信号

仓库共有 476 个提交，历史跨度为 2026-05-18 至 2026-07-06，全部由同一作者提交。全历史热点与修复提交交叉后，风险顺序如下：

| 文件/模块 | 修改次数 | fix/bug 关联次数 | 重新审查结论 |
| --- | ---: | ---: | --- |
| `frontend/src/views/Reader.vue` | 219 | 14 | 最高风险；不能再以旧 Reader 为骨架，按上游阅读场景逐块重建 |
| `frontend/src/layouts/AppLayout.vue` | 77 | 6 | 高风险；虽然侧栏字段接近 `Index.vue`，但多路由/按钮式导航仍需重新核对 |
| `frontend/src/views/Home.vue` | 69 | 4 | 高风险；书架列表已多次修补，需按 fork 最终 `Index.vue` 净结构重新验收 |
| `frontend/src/components/GlobalOverlayHost.vue` 与 overlays | 118（宿主） | 3 | 宿主注册中心思路符合上游 `App.vue`，但每个业务 overlay 必须独立重审 |
| `frontend/src/views/BookDetail.vue` | 49 | 2 | 当前路由页不是上游 `BookInfo.vue` 的天然替代，需要重建共享信息组件和操作链 |
| `frontend/src/views/Search.vue` | — | 2 | 当前独立搜索页可能打断上游首页搜索→信息→入架流程，需要与 `Index.vue` 合并审查 |
| `backend/api/books.go` | 53 | 4 | Go 技术实现可保留，但每个接口响应和事务顺序继续按上游行为测试 |

没有 revert/hotfix/emergency 提交，但 Reader、AppLayout、Home 的高频修改与修复重叠，说明主要风险是长期局部修补和结构漂移，而不是一次性回滚事故。

## 3. 模块重新判定

| 优先级 | 当前模块 | 上游权威文件 | 判定 | 后续动作 |
| --- | --- | --- | --- | --- |
| P0（进行中） | `Reader.vue`、桌面工具、设置、目录 | `views/Reader.vue`、`Content.vue`、`ReadSettings.vue`、`PopCatalog.vue`、`BookShelf.vue`、`BookSource.vue` | 需要重建 | 已完成桌面左右工具、纸张内工作区、目录双列、上游设置外观、连续滚动/点击分页分流；继续重审右侧书签/正文搜索/信息、缓存、听书和 Content 多内容类型 |
| P0 | 阅读器移动端 | `Reader.vue` 的 `mini-interface` 分支 | 需要重建 | 重新核对顶部/底部工具、中心点击、抽屉方向、安全区、翻页/滚动手势；不以当前 `ReaderMobileChrome` 为保留前提 |
| P1 | `AppLayout.vue` + `Home.vue` | `views/Index.vue` | 需要重新对齐 | 重新决定首页工作台与多路由边界；侧栏、搜索设置、最近阅读、书架主区和移动侧栏按 fork 最终结构逐项验收 |
| P1 | `Search.vue` + `Discover.vue` | `Index.vue` 搜索/探索结果、`Explore.vue`、`BookInfo.vue` | 倾向删除重建 | 恢复首页内“搜索/探索结果→书籍信息→加入书架/阅读”连续流程；当前独立页面只保留可复用 API/controller |
| P1 | `Sources.vue` | `Index.vue` 内书源管理对话框、阅读器 `BookSource.vue` | 结构需重建 | 本地/远程导入 controller 可复用；桌面管理表格、失效视图、编辑/调试和入口形态按上游内嵌管理界面重做 |
| P1 | `OverlayBookInfo.vue`、`BookInfoPanel.vue`、`BookDetail.vue` | `BookInfo.vue` | 需要重建 | 先建立唯一共享 BookInfo，再让书架、搜索、阅读器复用；路由详情不再拥有另一套操作逻辑 |
| P1 | `OverlayBookManagement.vue` 及拆分子组件 | `BookManage.vue` | 重新审查后重建外壳 | 已拆 controller 不等于对齐；保留真实批量 API，重新核对搜索、选中、分组、缓存、导出和桌面/移动布局 |
| P1 | `OverlayBookGroups.vue` | `BookGroup.vue` | 暂可保留 controller，重审 UI | 多分组和排序是当前增强；上游显隐、排序、增删和给书设组流程必须逐项验证 |
| P2 | `Settings.vue` | `Index.vue` 用户空间/WebDAV入口、`ReadSettings.vue` | 倾向拆散重建 | 阅读设置归阅读器；账户、用户、备份、WebDAV 按上游入口和弹层职责回归，不保留“后台设置中心”作为默认信息架构 |
| P2 | `LocalStore.vue`、`WebDAVBrowser.vue` | `LocalStore.vue`、`WebDAV.vue` | 逐组件重审 | 后端增强可保留；目录导航、上传、删除、导入、备份恢复及移动布局按上游核对 |
| P2 | RSS、替换规则、书签 overlays | 对应上游独立组件 | 逐组件重审 | 全局注册中心可保留，业务组件和打开/关闭链路按上游逐项替换 |
| 持续 | Pinia stores、Go API、缓存/用户 scope | 上游 Vuex/API 行为 | 技术层可条件保留 | 只有响应模型、事务顺序、同步事件和可见行为与上游一致时保留；不以已有单测数量代替语义核对 |

## 4. 可直接保留与不可直接保留

可直接保留的是“已经证明服务于上游行为的技术能力”，例如用户 scope 隔离、进度冲突保护、浏览器章节缓存、Go 事务测试和应用级 overlay 注册中心。它们仍会在触及对应模块时重新验证。

不可直接保留的是页面/组件外壳、按钮位置、抽屉/路由选择、信息层级和操作流程。即使此前已经拆成 composable 或小组件，只要上游不是这种职责边界，就应重新组合或删除。

## 5. 执行顺序

1. 完成阅读器全场景重审：桌面工作区、右侧操作、移动工具、正文 Content、书签/搜索/信息/换源/缓存/听书。
2. 重审首页 `Index.vue`：把 AppLayout、Home、Search、Discover、Sources 的现有边界放回上游工作台流程中判断。
3. 重建唯一 BookInfo，并重审 BookManage/BookGroup。
4. 重审用户空间、WebDAV、本地书仓、RSS、替换规则和备份。
5. 对每个 Go API 以触发它的上游方法为依据补充事务/响应回归。

每一项完成后都更新 `docs/upstream-alignment-audit.md`；适合用户验收的界面模块才本地构建并推送 Docker，同时附重构进度和仍未完成项。
