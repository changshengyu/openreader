# Reader 移动端阅读内书架宽度合同（P0）

审查日期：2026-08-09

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

状态：**inventory complete / must-fix / implementation pending**

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
