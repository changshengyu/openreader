# 书架一致性与阅读器首屏运行时合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同覆盖 2026-07-17 用户报告的五个运行时问题：新导入书籍短暂消失、移动端选中文字无操作弹窗、设置数值不可直接编辑、普通书首屏等待过长、EPUB 首屏及子资源加载过慢。当前 OpenReader 的既有实现和测试不构成正确性依据。

## 上游合同与当前差异

| 场景 | 上游权威行为与文件 | OpenReader 当前证据 | 判定与目标 |
|---|---|---|---|
| 导入/刷新后的书架 | `web/src/App.vue::loadBookShelf` 的单次 `networkFirstRequest` 成功后整体提交；`Index.vue::refreshShelf` 显式刷新；导入完成会重新加载书架。较新的用户动作不能被较早请求的迟到响应反向覆盖。 | `frontend/src/stores/bookshelf.js::loadBooks` 允许多个 `force` 请求并行，所有响应都无条件写入 `books`；`upsertBook` 后，先前发出的旧列表请求仍可覆盖新书。WebSocket 重连和多个客户端通知会放大竞态，但其它客户端本身不应决定当前页面是否显示新书。 | `must-fix`：同一用户作用域、同一列表条件采用最新请求/最新本地变更优先；导入或同步 payload 先即时 upsert；旧请求不得删除后来加入的书。手动刷新仍必须请求服务端。 |
| 移动端选中文字 | `web/src/views/Reader.vue::handleTouchEnd` 首先调用 `checkSelection(true)`；检测到选区立即返回，约 200ms 后按 `selectionAction` 打开操作弹窗，不能继续触发翻页/工具层。 | 正文只有 `mouseup` 调用 `scheduleSelectedTextOperation`；移动 `touchend` 直接进入手势/翻页逻辑。现有 mobile smoke 通过人工 Range + `MouseEvent('mouseup')`，未覆盖真实触摸结束。 | `must-fix`：touchend 必须先消费正文选区并调度操作；选区存在时不得翻页、切换工具层或穿透。保留 mouseup 供桌面端。 |
| 减号/数值/加号控件 | `web/src/components/ReadSettings.vue` 的字号、字重、行高、段距中间区域是可编辑 `el-input`。用户明确要求 OpenReader 保留减号/数值/加号布局，同时数字本身可点击自定义。 | `ReaderSettingStepper.vue` 中间是不可交互的 `<output>`。亮度及其它所有 stepper 都无法直接输入。 | `acceptable-change + must-fix`：点击中间数值进入输入；Enter/失焦提交，Escape 取消；提交值必须为有限数字并限制在 min/max，允许输入步长档位之间的合法值；加减仍按 step 调整。 |
| 普通书从书架到首章 | 上游 `BookShelf.vue::changeBook` 先把书架已有书籍数据写入 `readingBook`；Reader 直接用它，再加载目录和正文。 | OpenReader 路由只携带 ID；`useReaderBookLoad` 在首章请求前同时等待 `/books/:id` 和 `/books/:id/chapters`，即使 Pinia 书架已有完整书籍信息。 | `must-fix`：用当前书架项即时初始化 book；书籍详情刷新不得阻塞目录/首章关键路径。章节目录仍是定位章节的权威输入，错误和跨路由丢弃语义不变。 |
| EPUB 文档和资源 | `BookController.kt::extractEpub` 仅在提取目录不存在或显式 force 时解压；已提取的 HTML/CSS/图片直接读取，不为每个资源重新哈希原 EPUB。 | `epubreader.Service.OpenResource` 对 iframe 文档及每个 CSS/图片/字体都调用 `ensureExtraction`；该方法持有全服务全局锁并 SHA-256 整本 EPUB 后才检查完成标记。 | `must-fix`：签名 capability 已绑定用户、书、指纹和有效期；资源请求必须直接打开该指纹对应的已完成、不可变提取目录，不得重读/重哈希源 EPUB。首次 prepare 仍执行有界安全解压和指纹校验。不同书不得被一把全局锁串行化。 |

## 状态与数据合同

### 书架最新结果优先

1. 每次列表请求取得单调递增 revision，并绑定当前用户 scope 与 request key。
2. 只有仍为该 key 最新 revision、且没有被后续本地 upsert/delete/reset 失效的响应可以替换列表。
3. `upsertBook`、`removeBookLocal`、导入成功与 WebSocket payload 都推进本地 mutation revision；它们之后到达的旧全量响应只能被丢弃。
4. 被丢弃响应不得更新内存时间戳或浏览器书架缓存。
5. 401/切换用户继续沿用既有 reset；不能把上一用户请求写进新用户 scope。
6. WebSocket 首次连接或断线重连必须强制进行一次服务端书架/分组刷新；不能因为 5 秒内存缓存仍新就跳过，否则断线期间其它客户端的导入会永久缺失到下一次手动刷新。

### 选区优先级

`touchend -> 检查属于 reader-body 的非空选区 -> 调度操作弹窗并抑制后续 click -> return`。仅在无选区时才进入滑动翻页、点击区或工具层状态机。面板内触摸仍由面板自身 stop，不得读取正文选区。

### EPUB capability 安全边界

- capability 的 `Fingerprint` 只允许映射到 `<bookRoot>/.epub-resources/<64位小写sha256>`。
- 目录必须位于该书私有 library root 内，并且 `.openreader-complete` 内容与 capability 指纹完全一致。
- `OpenResource` 仍校验 JWT secret 签名、purpose、过期时间、用户/书所有权、媒体类型、规范化归档路径、真实路径边界与文档大小。
- 资源请求只对当前源 EPUB 做轻量文件身份检查（大小与纳秒修改时间）；身份未变时不读取、不哈希归档。身份变化时才重新计算一次 SHA-256：指纹不同则旧 capability 立即失效，指纹相同则原子更新 marker。源文件缺失时，旧 capability 仍只能访问它签名时已经安全提取的不可变版本。
- 提取目录意外丢失时允许一次兼容自愈：重新执行有界解压并要求新算指纹与 capability 完全一致；marker 存在但内容错误时不得自愈，必须失败关闭。
- 新章节 prepare 对变更后的源文件重新计算指纹并生成新的提取版本/capability。不得为提速关闭 ZIP 炸弹、路径穿越、符号链接或大小限制。

## 测试闸门

1. Pinia 单元测试构造“旧请求后返回 + 中途 upsert”的确定性竞态，断言新书始终保留；切换用户的旧响应也不得提交。
2. Reader 单元/浏览器合同以 `touchend` 而非仅 `mouseup` 驱动选区，断言出现“选择文字”弹窗且没有翻页或工具层切换。
3. Stepper 测试覆盖点击编辑、合法小数、范围裁剪、无效输入回滚、Enter/blur/Escape 和继续加减。
4. Reader 加载测试人为延迟 book 详情，断言目录一到即可请求正文，迟到详情只合并元数据且不改变章节位置。
5. EPUB 服务测试在 prepare 后移走源 EPUB，已有 capability 仍可读取已提取资源；源内容变化使旧 capability 失败；提取目录缺失可按同指纹自愈；错误指纹/marker、越界路径继续失败。增加大文件探针，确认单章的多个资源请求不重复读取源归档。
6. 完成 `go test ./...`、前端全量测试/build，并在 1440×900、390×844、360×800 做书架导入刷新、移动选词、数值编辑、普通书/EPUB 首屏真实浏览器回归。

## 本批不授权的变化

- 不改变现有数据库、书籍/章节 API schema、旧路由、用户数据和备份格式。
- 不通过延长陈旧缓存、隐藏 loading、提前显示错误章节或放松 EPUB 安全边界伪造性能提升。
- EPUB 首次有界解压仍可能随文件体积增长；本批目标是移除重复整本哈希和全局串行，并缩短可见首屏关键路径。若首次解压仍不满足验收，再单独审查导入期预热。

## 2026-07-17 实施记录

- 书架列表增加用户作用域内的请求/本地变更 revision。较旧的 force 请求、导入前已发出的请求及上一用户请求均不能再提交到 Pinia 或浏览器缓存；导入和 WebSocket payload 仍即时 upsert。
- 同步 WebSocket 每次成功连接（含断线重连）都强制刷新一次服务端书架和分组，补回断线期间其它客户端的导入；并发刷新由同一 revision 门保证最新结果优先。
- 移动选区检查在 touchend 后保留 720ms 有界重试窗口，每 80ms 读取一次，只处理 reader-body 内的选区；选区命中后继续抑制正文 click，桌面 mouseup 行为不变。
- 所有 `ReaderSettingStepper` 中间数值均可点击编辑；Enter/失焦提交、Escape 取消，非法输入回滚、越界裁剪，按钮仍使用原 step 增减。
- 从书架进入 Reader 时直接复用 Pinia 书籍数据，目录到达后立即加载正文、执行路由定位、书签和进度协调；`/books/:id` 详情只在之后合并，不再阻塞首屏。
- EPUB capability 优先直达签名指纹的已完成提取目录。未变化源只做 stat 身份检查，不再对每个 HTML/CSS/图片/字体重哈希整本 EPUB；身份变化时才哈希并使旧 capability 失效。提取锁由全局一把改为每个源文件独立，自愈和旧纯文本 marker 继续兼容。
- 验证：前端全量 409 项、生产构建、Go 全包通过；Reader 在 1440×900、390×844、360×800 通过真实 Chrome，覆盖 touchend 选词、数值直改和面板/正文几何；书架在 1440×900、390×844 通过“旧响应延迟 900ms、较新 force 先返回”的真实浏览器竞态合同。
