# LocalStore 工作台所有权收敛合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：**2026-07-27 已完成只读审计，尚未开始本批应用代码改动。**

本合同只处理 LocalStore 的前端所有权和不可达页面壳，不重开已经完成的目录列表、搜索阈值、
101 项显示门槛、导入预览、权限、解析器、文件 API、旧卷或备份合同。

## 1. 上游权威行为

- `web/src/views/Index.vue` 持有 `showLocalStoreManageDialog`，侧栏“本地书仓”只打开工作台内的
  LocalStore Dialog；书架和导航仍属于同一个 Index 场景。
- `web/src/components/LocalStore.vue` 自己就是 `书仓文件管理` Dialog，不存在独立
  LocalStore 产品页面、独立页头或 `embedded` 双形态。
- 关闭后仍回到原工作台；再次打开从书仓根目录重新加载。面板点击不得穿透到移动侧栏或书架。

## 2. 当前证据与裁决

| 项目 | 当前实现 | 裁决 |
|---|---|---|
| 兼容路由 | `router/index.js` 将 `/local-store` 重定向到 `/?overlay=local-store`，保留其它 query。 | **aligned**，不得恢复独立路由。 |
| 可见所有权 | `GlobalOverlayHost -> OverlayLocalStore.vue` 是唯一可见入口，使用工作台内 `el-dialog`，移动端 fullscreen。 | **aligned**，保留。 |
| 业务体 | `OverlayLocalStore.vue` 异步导入 `views/LocalStore.vue` 并传入 `embedded`。 | **错误结构**：业务体仍以 page/view 命名并保留双壳协议。 |
| 不可达分支 | `views/LocalStore.vue` 保留 `embedded=false`、`app-page`、独立 `store-head` 和“本地书仓”页头；产品路由没有任何入口以该形态挂载。 | **must-fix**：删除不可达独立页面分支，不能把死代码当兼容能力保留。 |
| 业务行为 | 当前目录、筛选、加载更多、选择、上传、删除和共享导入预览均由同一文件实现。 | **保持不变**：本批只迁移所有权，不改动作、文案、请求或数据。 |

## 3. 实施边界

1. 将业务体迁移为工作台组件，例如
   `frontend/src/components/workspace/LocalStoreManager.vue`。
2. 组件根节点只表达文件管理 body，不接受 `embedded`，不渲染独立页面标题。
3. `OverlayLocalStore.vue` 继续唯一持有 `el-dialog`、标题、compact fullscreen 和销毁重建。
4. 删除旧 `frontend/src/views/LocalStore.vue`；测试和动态导入不得继续引用该路径。
5. `/local-store` 继续只产生 root workspace overlay intent；关闭只清理自己的 intent query。
6. 不修改后端 API、SQLite、`data/cache/library`、导入 token、解析格式或权限。

允许差异仅为 Vue 3/Element Plus 的 Dialog 实现、JWT/多用户权限和现有安全导入预览。

## 4. 测试先行合同

应用实现前先让以下断言失败：

1. 静态所有权测试要求 `components/workspace/LocalStoreManager.vue` 存在，且
   `views/LocalStore.vue` 不存在。
2. Manager 不声明 `embedded` prop，不包含 `store-head`、独立 `app-page` 页头或第二个
   `el-dialog`。
3. `OverlayLocalStore.vue` 只导入 Manager，仍保留 `书仓文件管理`、desktop width、
   mobile fullscreen 与 `destroy-on-close`。
4. 现有路由合同继续证明 `/local-store` 保留无关 query 并进入根工作台。
5. 现有 LocalStore 工作流、导入重试、文件管理、格式和存储导入测试全部改为读取 Manager，
   且行为断言不减少。
6. 真实浏览器在 1440×900、390×844、360×800 验证侧栏打开书仓、旧链接打开、关闭返回书架、
   根目录重载、无横向溢出和移动点击不穿透。

## 5. 发布边界

这是不改变用户数据和 API 的结构收敛切片。完成前端全量测试、生产构建、三视口浏览器和
后端全量回归后即可提交 GitHub。它本身不必单独发布 Docker；如果与下一项可见修复形成
连贯候选，则按用户允许的半模块节奏本地构建并推送 GHCR。
