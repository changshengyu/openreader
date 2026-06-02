# OpenReader 上游对齐审查

上游基准：`/private/tmp/hectorqin-reader/web/src`。

本文件是提交前闸门。每一批重构都要回答：改动对应上游哪个组件、哪个方法、哪些差异是后端缺口或用户明确要求导致的。

## 当前结论

| 模块 | 上游基准 | 当前状态 | 结论 |
| --- | --- | --- | --- |
| 首页/书架 | `views/Index.vue` | 已恢复移动端侧边导航思路，正文区继续收敛为书架列表；本批已移除移动正文区搜索和行内操作按钮 | 继续对齐 |
| `miniInterface` 判定 | `plugins/helper.js` `isMiniInterface`、`plugins/vuex.js` `setMiniInterface` | 已收敛为 `<=750px` 或手动“手机模式”，不再用触摸设备/1180px 误判 | 本批完成 |
| 移动侧边栏/书架宽度 | `Index.vue` `navigation-wrapper`、`shelf-wrapper` | 已使用 260px 侧栏、右滑打开/左滑关闭；本批修正移动端分组栏 `width:100% + margin` 导致的横向溢出 | 继续真机验收 |
| 阅读器工具栏 | `views/Reader.vue` `showToolBar/showReadBar` | 移动端工具栏默认隐藏，中心点击显示；桌面右侧快捷工具栏已恢复为上游式单列圆形按钮，不再挤成两列 | 基本对齐 |
| 阅读点击区 | `Reader.vue` `eventHandler`、`ReadSettings.vue` `clickMethod` | 本批补齐“下一页 / 自动 / 不翻页”；自动模式按上游区分左右滑动和上下滚动 | 本批完成 |
| 滚动阅读手势 | `Reader.vue` `handleTouchMove`、`isSlideRead` | 本批确认上下滚动不拦截手指滑动，固定距离翻页只由点击区触发 | 本批完成 |
| 阅读方式 | `ReadSettings.vue` `readMethods`、`animateMSTime` | 当前有上下滑动、左右滑动、上下滚动、上下滚动2；本批补齐 `scroll2` 顶部附近自动加载上一章并保持视口位置，连续滚动不再只能向下追加章节 | 继续真机验收长书跨章节滚动 |
| 设置写入路径 | `ReadSettings.vue` `setReadMethod/setPageMode`、`Index.vue` 搜索设置 | 本批收敛为单一 computed setter 写入；阅读器设置抽屉、设置页、首页侧边栏搜索设置不再同时通过 `v-model`、`@input`、`@change` 双写同一项 | 本批完成 |
| 目录定位 | `PopCatalog.vue` + `Reader.vue` 当前章节定位 | 本批按上游目录弹层补齐倒序/顺序、顶部、底部、刷新入口；打开目录和切换顺序都会重新定位当前章节 | 基本对齐，继续真机验收 |
| 书籍信息 | `components/BookInfo.vue` | 已复核书架、搜索、书海、阅读器均走全局 `openBookInfo`，详情页直接复用 `BookInfoPanel`；已补齐上游书名下方分类/标签信息层，并把“设置分组”放回分组属性行 | 基本对齐，继续核对加书/刷新/封面细节 |
| 书架管理 | `components/BookManage.vue`、`BookGroup.vue` | 已有全局弹层；本批补齐批量导出入口，批量删除/分组/服务器缓存/清缓存均走真实后端接口 | 继续对齐移动端细节 |
| 书源管理 | `components/BookSource.vue`、首页书源入口 | 基础管理、导入预览、远程预览、三段调试、失效检测、批量启停/删除、批量设置分组、恢复默认已有；本批补齐上游式“设为默认书源/恢复默认书源”快照闭环 | 继续真机验收移动端管理细节 |
| 本地书仓 | `components/LocalStore.vue` | 已支持目录浏览、递归扫描、新建目录、上传、下载、重命名、删除、导入当前目录/筛选/选中/单文件/目录；本地书搜索可命中已导入书架的本地书 | 基本对齐，继续真机验收 |
| WebDAV/备份 | `components/WebDAV.vue` | 本批抽出共享 `WebDAVBrowser`，设置页和全局弹层共用同一套目录浏览、上传、新建目录、下载、重命名、删除、导入书架、恢复备份逻辑 | 基本对齐，继续移动端验收 |
| RSS/书海 | `Rss*`、`Explore.vue` | RSS 已抽出共享 `RSSManager`；书海补齐上游式书源分组、展开源、按 `exploreUrl` 入口触发发现结果，后端支持指定入口分页加载；本批抽出 `RemoteBookResultGroups`，书海和书源搜索共用同一套结果展示/书籍信息入口 | 继续真机验收书海移动端 |

## 后续提交前检查清单

- 首页移动端没有底部主导航，没有桌面窄侧栏常驻。
- 书架正文区不被固定宽度、表格列、按钮组或长标题撑出屏幕。
- 阅读页移动端默认只有正文；顶部栏、底部进度、目录/书签/搜索/设置/更多只在中心点击后出现。
- 上下滚动模式手指滑动必须是原生滚动；固定步长翻页只来自点击上/下区域。
- 新增按钮必须能对应上游组件职责；后端没有能力时隐藏或禁用并说明原因。
- 文档和 `plan.md` 不得再保留“移动底部导航”这类已经被上游对齐否定的方案。
