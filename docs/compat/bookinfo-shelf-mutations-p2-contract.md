# P2 BookInfo 书架态字段变更合同

状态：2026-07-16 已按固定基准
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
实施并完成 Go、前端和三视口真实浏览器验证。本合同只涵盖已在书架的 BookInfo 字段变更；
书架/搜索/探索/Reader/旧链接的共享 BookInfo 入口已由 P1-B 完成，不能借
本合同重新引入第二套详情或阅读动作。

上游权威：`web/src/components/BookInfo.vue` 的封面上传、`refreshLocalBook`、
`toggleBookCanUpdate`、`showSetBookGroup` 和 `saveBook`。上游为单用户服务，
因此其整本 `saveBook` 写入、静态上传路径和无资产所有者概念不能直接作为
OpenReader 多用户运行时的数据边界。

## 上游动作与当前映射

| 动作 | 上游可见语义 | 当前 OpenReader 路径 | 审查结论 |
|---|---|---|---|
| 自定义封面 | 选择 JPG/PNG，上传成功后把 `customCoverUrl` 保存到当前书；失败保持原封面/提示失败。 | `POST /api/uploads` → 精确 `PUT /api/books/:id {customCoverUrl}`，`OverlayBookInfo` 再合并返回书架项。 | **已实施 P2**：不再重发显示字段；新上传/删除路径按 JWT 用户隔离，旧上传路径只读兼容。 |
| 追更开关 | 只切换当前书的追更字段并刷新书架记录。 | `PUT /api/books/:id` 精确 `{canUpdate}`，成功后使用服务器返回的书架投影。 | **已实施 P2**：不会用过期的标题、简介、分组或封面覆盖并发保存。 |
| 设置分组 | 打开同一 BookGroup 设置模式；上游拒绝空选择。 | `OverlayBookGroups` → `PUT /api/books/:id/category`，空选择在 UI 侧拦截。 | **已验证**：P2 BookManage/BookGroup 无 mock 浏览器合同已覆盖预选、空选择无请求、保存和批量动作。不得回退到通用整书 PUT。 |
| 本地更新 | 重新解析本地源，成功后替换目录/当前书并刷新书架；失败不能破坏原书。 | `POST /api/books/:id/refresh-local` 先 stage，SQLite 事务替换章节/恢复位置，再原子 promote，最后广播。 | **已对齐的 E4 技术栈实现，P2 只需保持回归**：不为 BookInfo UI 改动重写 archive、原文件、章节缓存或同步顺序。 |

## API 合同

| 方法/路径 | 请求与授权 | 成功与副作用 | 失败/兼容性要求 |
|---|---|---|---|
| `PUT /api/books/:id`（追更） | 当前用户；精确 JSON `{canUpdate:boolean}`。 | `200` 返回当前用户的完整 shelf item；只改变该 book 的 `canUpdate` 和 `updatedAt`，提交后发一个当前用户 `bookshelf_update`。 | 外书 `404`；非法 JSON `400`；不得因追更重写 title/author/intro/category/customCover。现有广义部分更新路径保持给编辑器/旧客户端使用。 |
| `POST /api/uploads`（新资产） | 当前用户 multipart `file`,`type`；`type=cover` 仅 JPG/JPEG/PNG，8 MiB 上限。 | `201 {url,name,size,type}`；新文件写入用户私有子树，返回可供 `<img>`/字体/背景直接加载的稳定 capability URL。 | 仍拒绝扩展名伪造、超限和路径穿越；没有书籍数据库写入。上传成功而后续书籍更新失败只会留下可安全清理的孤儿新资产，绝不能覆盖旧封面。 |
| `PUT /api/books/:id`（封面） | 当前用户；精确 JSON `{customCoverUrl:string}`，空串代表清除。 | `200` 返回完整 shelf item，提交后广播；当前用户的新上传 URL 必须可归属到该用户。 | 外书 `404`；其他用户的新资产 URL、任意 `/uploads/` 路径逃逸或新外部 URL `400`。原有数据库/备份中的 legacy `/uploads/<kind>/<file>` 值只允许原值继续读取/返回，不能被后台迁移或删改。 |
| `DELETE /api/uploads` | 当前用户 JSON `{url}`。 | `200 {deleted:true}` 仅可删除当前用户新子树中的资产；删除只在没有当前用户配置/书籍引用时执行。 | 非所有者/旧全局路径/路径逃逸返回安全 `400` 或 `404`，绝不能删除其他用户文件。历史全局路径保持可读，不把“删除历史资产”伪装成成功。 |
| `POST /api/books/:id/refresh-local` | 当前用户；可选 `{tocRule}`。 | `200 {book,chapterCount}`；先写 inactive stage，事务更新 books/chapters/bookmarks/progress，promote 后才广播。 | 非本地 `400`、外书 `404`、解析/读源/阶段失败不改变活动目录、原 archive、书架行或位置。 |

## 数据、迁移与静态资源边界

当前 `data/uploads/<kind>/<timestamp>-<random>.<ext>` 是全局目录，`
router.Static("/uploads", ...)` 使其公开可读；`DELETE /api/uploads` 只检查根
路径，没有用户归属。这与 OpenReader 的多用户隔离目标冲突。

实施时的新写入使用：

```text
data/uploads/users/<user-id>/<kind>/<timestamp>-<random>.<ext>
/uploads/users/<user-id>/<kind>/<timestamp>-<random>.<ext>
```

- 用户 ID 从 JWT/持久化 User 行取得，绝不从 multipart 字段或 URL 推断。
- `data/uploads/<kind>/...` 的所有既有 legacy 文件、数据库 `customCoverUrl`、
  Reader 背景/字体配置和备份 JSON 均保持可读；不启动扫描、移动或删除。
- 新路径采用现有随机命名但在用户根内生成；静态 URL 是浏览器资源能力而非新的
  认证 API。该能力的不可枚举性不能替代写入/删除的 user-root 校验。
- 更换/清除封面时，只有已经证明处于当前用户新资产根、且没有当前用户 Book 或
  Setting 引用的旧文件才可在数据库提交后清理。legacy 文件绝不自动删除。阅读
  背景/字体在客户端必须先同步移除后的 `reader` setting，成功后才调用删除接口；
  同步失败时恢复 UI 状态且绝不请求删除。
- Docker `data/` 卷、旧 SQLite 行、WebDAV/portable backup 的字符串字段无需迁移；
  新 URL 只是已有字符串字段的新合法形态。卷升级必须同时验证 legacy 与新用户路径。

## 必须先写的测试

1. 前端 composable 合同：追更只发送 `{canUpdate}`；封面保存只发送
   `{customCoverUrl}`，并使用返回值替换 Overlay/书架状态，不携带过期分类或文本。
2. Go API：两用户上传同类资产；URL 根、数据库引用、删除、跨用户覆盖和跨用户
   删除均隔离。外书仍 `404`，大小/扩展名/路径逃逸仍拒绝。
3. Go API：legacy `/uploads/covers/...` 的已有 Book 与备份恢复可读；新客户端不能
   把别人的新 URL 写进自己的 `customCoverUrl`，也不能让删除接口触碰 legacy 文件。
4. Go/API：本地 refresh 成功、解析失败、stage 写入失败和外书访问分别保持已记录的
   archive/章节/进度/书签边界。
5. 真实浏览器（1440×900、390×844、360×800）：从 BookInfo 依次切追更、上传封面、
   设置分组、刷新本地书，断言对话框/书架/Reader 同步且没有路由或移动工具层变化。
6. 迁移/Docker：旧挂载卷含 legacy cover，两个用户各有新 cover；升级、备份/恢复和
   `scripts/docker-volume-backup-smoke.sh` 均不丢失或跨用户删除资产。

在上述测试、API/数据合同与实现通过前，不允许把任一 BookInfo 字段动作标为完整
P2 对齐，也不允许以清理孤儿文件为由批量删除旧 `data/uploads/`。

## 2026-07-23 资产备份证据勘误

本合同已经证明 BookInfo 新封面的用户归属、引用保护、legacy 只读和 mounted
`data/` 卷升级，但没有证明普通逻辑备份或 `openreader-portable-v1` 携带上传文件。
两种 ZIP 当前只保存 `customCoverUrl`/Reader setting 字符串；恢复到不同实例或
不同 user ID 时可能成为失效引用。该缺口由
[`reader-appearance-assets-p2-contract.md`](reader-appearance-assets-p2-contract.md)
的 P2-B 统一处理，避免为封面与 Reader 字体/背景发明两套资产迁移格式。在 P2-B
后续 P2-B 已实现 portable v2 的资产字节打包和跨 user ID 重写；普通逻辑 ZIP 与已有
portable v1 仍仅按本段记录保存字符串。Docker 发布证据见 P2-B 合同的后续状态。
