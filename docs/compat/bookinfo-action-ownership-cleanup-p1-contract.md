# BookInfo 上下文动作遗留清理合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：**2026-07-27 已完成只读审计、失败测试、遗留删除、全量自动化与三视口真实浏览器验证。**

> 2026-08-02 勘误：本合同删除不可达动作 builder 的结论仍有效，但其中“搜索/探索加书由
> OverlayBookInfo 统一选择分组”的历史判断已被固定基准第二轮复审取代。当前动作所有权是
> “搜索/探索结果卡选择分组，BookInfo 直接加入”，详见
> [`bookinfo-fixed-baseline-second-audit-p2-contract.md`](bookinfo-fixed-baseline-second-audit-p2-contract.md)。

本合同只清理共享 BookInfo 收敛后遗留的不可达动作 builder，不重开已经完成的 BookInfo
加书、分组、封面、编辑、追更、本地刷新、缓存或旧链接行为。

## 上游与当前所有权

- 上游 `App.vue` 持有一个全局 `BookInfo.vue`；Index、搜索/探索和 Reader 只提交当前书并打开
  它。是否已在书架由 BookInfo 自己决定可见动作。
- 当前可见实现已经等价收敛为
  `GlobalOverlayHost -> OverlayBookInfo -> BookInfoDialog -> BookInfoPanel`。
- `Home.vue`、`Search.vue`、`Discover.vue`、`Reader.vue`、`AppLayout.vue` 和 BookManage 都直接
  调用 `overlay.openBookInfo()`；当前 `frontend/src` 没有任何文件导入
  `utils/bookInfoOverlayActions.js`。
- `bookInfoOverlayActions.js` 仍导出“查看详情 / 继续阅读 / 开始阅读 / 加入并阅读”等入口级
  动作数组。这是 2026-07-07 共享 Overlay 收敛前的遗留第二套动作策略，现已完全不可达。

## 裁决与实施边界

| 项目 | 裁决 |
|---|---|
| 唯一可见 BookInfo | **保持**：所有入口继续只调用 `overlay.openBookInfo()`。 |
| `bookInfoOverlayActions.js` | **错误结构 / must-fix**：删除，不保留无调用者的第二套动作标签和 handler builder。 |
| 搜索/探索未入架动作 | **保持**：由唯一 `OverlayBookInfo` 的 `useBookInfoAddToShelf` 事务决定，只显示上游单一“加入书架”。 |
| 旧 `/books/:id` | **保持**：继续归一化成根工作台 `bookInfo` intent，由 AppLayout 打开同一 overlay。 |
| Reader | **保持**：打开纯 BookInfo，不注入“完整详情/开始阅读/继续阅读”等入口动作。 |

本批不修改 API、Pinia 结构、请求顺序、路由、数据或可见文案。

## 测试先行合同

1. 新测试先要求 `frontend/src/utils/bookInfoOverlayActions.js` 不存在；实现前必须失败。
2. 继续断言 Home/Search/Discover/Reader/AppLayout 只调用 `overlay.openBookInfo()`，不导入或调用
   `buildBookInfoReadActions`、`buildBookInfoStartReadActions`、
   `buildSearchExistingBookActions`、`buildSearchAddBookActions`。
3. 既有 BookInfo 五入口、加书确认/取消、Reader plain BookInfo 和旧链接测试不得减少。
4. 前端全量、生产构建和后端全量必须通过。因删除的是零引用模块且产物不含该代码，现有
   三视口 BookInfo 工作台合同通过即可；无需新增视觉结构。

## 发布边界

本切片只删除不可达代码，不产生用户可验证的运行时差异。通过门禁后立即同步 GitHub，但不应
单独发布 Docker；应与下一项可见修复合并成验收镜像。

## 实施与验证记录

- 新增的所有权测试在删除前按预期仅因 `bookInfoOverlayActions.js` 仍存在而失败；五个入口
  的唯一 OverlayBookInfo 断言当时已经通过。
- 零引用 `bookInfoOverlayActions.js` 已删除，Home/Search/Discover/Reader/AppLayout 没有
  应用改动；搜索/探索加书仍由 `OverlayBookInfo` 的唯一事务处理。
- 前端 **573/573**、Vite 生产构建和后端 `go test ./...` 通过。
- `index-workspace-contract.mjs` 在 1440×900、390×844、360×800 通过 legacy redirects、
  sidebar search、canonical BookInfo 和 Explore cover→BookInfo 检查。
- 本批未修改 API、路由、Pinia、SQLite、持久化目录或用户可见 UI，不单独发布 Docker。
