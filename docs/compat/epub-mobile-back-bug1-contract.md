# EPUB 移动端返回手势 Bug 1 兼容合同

状态：**2026-07-15 已完成失败测试、实现、三视口真实浏览器回归并随 `28eb413` 发布。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同只处理用户报告的 Bug 1：阅读 EPUB 时，手机系统/浏览器返回手势不应先回到上一章开头；在正常“书架 → Reader”进入路径中，它应返回书架。

## 1. 上游状态合同

| 场景 | 上游证据 | 必须保持的行为 |
|---|---|---|
| 进入阅读器 | `web/src/views/Index.vue#readBook` 将阅读状态写进 Vuex，再 `router.push('/reader')`。 | 书架到 Reader 仅产生一次顶层历史跳转。 |
| 章节切换 | `web/src/views/Reader.vue#getContent(index)` 更新 `readingBook.index`、获取内容并恢复阅读位置。 | 翻章是 Reader 内部状态转换，不新增浏览器历史条目。 |
| 返回书架 | `web/src/views/Reader.vue#toShelf` 调用 `this.$router.push('/')`。 | 返回目的地是书架工作台，而非前一章或 iframe 资源 URL。 |
| EPUB 显示 | `web/src/components/Content.vue#renderEpub` 使用 iframe；相对资源和链接由文档 URL 解析。 | OpenReader 可以保留受控 resource capability/CSP 安全适配，但不能让 iframe 子导航改变顶层返回语义。 |

## 2. 当前差异和根因

| 项目 | 当前证据 | 判定 |
|---|---|---|
| Reader 顶层章节路由 | `Reader.vue` 的普通章节导航使用 `router.replace(readerRouteLocation(query))`。 | **技术栈等价**：正常章节切换不会累积顶层历史。 |
| EPUB iframe 的跨资源链接 | `backend/services/epubreader/document.go` 的 bridge 原先只拦截外链和同 XHTML hash；同 EPUB resource root 下、不同 XHTML 的 `<a>` 直接走 iframe 默认导航。 | **must-fix**：iframe 的 joint session history 被写入 EPUB 章节 URL。 |
| 父 Reader 切换 resource | `ReaderEpubContent.vue` 把新的 `resource.url` 写入同一 iframe 的 `src`。 | **must-fix**：即使 bridge 拦截 anchor，复用同一 browsing context 仍可留下 iframe 历史条目；资源切换必须重建 iframe，而不是只更新 `src`。 |
| iframe 加载回传 | bridge 在 iframe `load` 时发送 `load`；`Reader.vue#handleEpubLoad` 据此匹配 resource 并直接 `loadChapter(..., 0)`。 | **must-fix 的连锁表现**：浏览器返回先回到上一 iframe 历史条目，再被解析为上一章节开头。 |

根因不是网速、目录解析或正常的 Reader `router.replace`：跨 XHTML 链接原先没有 `preventDefault()`，且父 Reader 随后复用 iframe 的 `src`。两条路径都会让浏览器保留 iframe 子页面历史。

## 3. 目标状态转换

1. 从书架进入 EPUB Reader 后，顶层浏览器历史只含书架和 Reader，而不含 iframe 内的章节资源。每次 resource 切换必须替换 iframe element/browsing context，不能复用旧 iframe 的 `src` 历史。
2. EPUB 文档内链接分支必须是：

   - 外部/非当前 capability 根：阻止默认行为，发送既有 `externalLink` 事件；
   - 当前 XHTML 且 slice 内存在的 `#fragment`：阻止默认行为，发送 `clickHash`，在当前 Reader 内容区域原地定位；
   - 当前 EPUB capability 根内、但目标 fragment 不在当前 slice，或目标是另一个 XHTML：阻止默认行为，发送 `navigate`，由父 Reader 完成既有的章节加载事务；
   - 无效 URL：阻止默认行为，不导航。

3. `navigate` 仍只接受当前 iframe、同 origin 的 postMessage；resource capability、CSP、用户隔离、相对 CSS/字体/图片和 fragment 目录匹配不得放宽或改变。
4. 正常由书架进入的移动端（390×844 与 360×800）中，跨 XHTML 跳转后调用浏览器返回，必须到书架 `/`，不得显示上一章节资源或将章节重置到开头。
5. 直接打开 Reader 深链接没有书架历史时，浏览器返回由宿主浏览器决定；本 Bug 不用伪造全局 `popstate` 来破坏浏览器的正常离开行为。

## 4. 必须先失败的回归用例

| 编号 | 测试 | 断言 |
|---|---|---|
| EPUB-BUG1-A | `backend/services/epubreader/document_test.go` | 跨 XHTML、同 capability 根链接在 bridge 内显式 `preventDefault()` 并发出 `navigate`；同文档 slice 内 hash 仍走 `clickHash`，外链仍走 `externalLink`。 |
| EPUB-BUG1-B | `frontend/tests/readerEpubFrame.test.mjs` 与 `ReaderEpubContent` 合同测试 | 合法 iframe `navigate` 事件仍传递给父 Reader；来源/origin 不合法时被拒绝；resource URL 改变时以 `key` 替换 iframe，而非复用同一 browsing context。 |
| EPUB-BUG1-C | `scripts/smoke/reader-epub-contract.mjs` | 真实 Go 服务 + 两章节 EPUB：从书架进入、点击跨 XHTML 链接后执行 `page.goBack()`，URL 为书架且不重新显示上一 EPUB 章节；390×844 与 360×800 必跑，1440×900 回归也应继续通过。 |

## 5. 允许差异和发布门禁

- 允许差异仅是 OpenReader 的受签名 iframe resource/CSP 与 Vue Router 适配；它们不得暴露 archive 路径或修改顶层返回语义。
- 本 Bug 不增加 SQLite 字段、持久化设置、API 路径或 mounted volume 内容，因此无数据迁移。
- 实施后至少运行 Go 全量测试、前端全量测试与生产构建，并在真实浏览器运行 EPUB smoke。通过后此切片可作为独立 Docker 用户验收批次；发布记录必须单列 Bug 1 及镜像 digest。

## 6. 实施记录

- EPUB bridge 对同 capability 根内的跨 XHTML/当前 slice 外 fragment 链接执行
  `preventDefault()` 并向父 Reader 发送 `navigate`；外链和当前 slice hash 分支保持独立。
- `ReaderEpubContent.vue` 使用 `:key="resource.url"` 在资源切换时替换 iframe browsing
  context，避免子 frame 历史消费移动端返回手势。
- Go bridge 测试、`readerEpubFrame.test.mjs` 和真实 Go 的
  `reader-epub-contract.mjs` 已覆盖来源/origin 校验、跨 XHTML 和 `page.goBack()` 返回书架；
  1440×900、390×844、360×800 均通过。
- 源码提交 `28eb413 fix: return EPUB reader back to shelf` 已同步 GitHub；后续 EPUB
  目录/旧卷修复继续保留该返回合同。
