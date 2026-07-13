# P2 失效书源缓存兼容契约

状态：2026-07-12 已从固定上游 `reader-dev@fa22f271849d45f93349ae1636223e27b16a4691` 提取、实现并验证。

## 上游事实

- `BookController.kt` 将异常写入 `storage/cache/invalidBookSourceCache/<userNameSpace>`，键为 `bookSourceUrl`，值为 `{sourceUrl,time,error}`，有效期固定 `600` 秒。
- `getInvalidBookSources` 只读取当前用户未过期缓存；`Index.vue#showFailureBookSource` 仅请求这份缓存、切换到“失效书源管理”并打开同一管理器，不启动新的检测请求。
- 普通多源搜索在 `searchBookWithSource` 开始时跳过仍在失败缓存中的源；章节内容、搜索、目录/换源等书源请求异常写入同一缓存。
- 前端把缓存行按 `sourceUrl` 合并到当前书源列表；已删除或不再匹配的书源不显示。

## OpenReader 数据等价方案

OpenReader 使用 JWT + SQLite，不复制上游哈希文件格式。新增仅作运行时缓存的 `source_failures` 行：

| 字段 | 含义 / 兼容规则 |
|---|---|
| `user_id` + `source_id` | 唯一的当前用户/全局书源组合；绝不跨用户共享。 |
| `source_url` | 记录时的 `BookSource.BaseURL` 快照，只供检测编辑后失效的旧记录，不能作为跨用户查询条件。 |
| `message` | 截断后的客户端安全失败类别（如“请求超时”“请求书源失败”）；不得保存或响应 cookie、认证头、源 URL 查询参数、响应正文或宿主路径。 |
| `failed_at` / `expires_at` | UTC 时间，`expires_at = failed_at + 600s`；过期后不可返回或跳过书源，并可在读/写时删除。 |

这是允许的 Go/SQLite 安全适配：不会修改 `book_sources`、`books`、`chapters`、`data/`、`cache/` 或 `library/` 的既有格式；不会写入备份，也不需要导入旧数据。SQLite AutoMigrate 只能添加表/索引，绝不删除或重写现有行。

## API 与状态契约

| 操作 | OpenReader API | 行为 |
|---|---|---|
| 查看失效源 | `GET /api/sources/invalid` | JWT 必需。返回当前用户且未过期、仍能匹配当前 `BookSource` 的源行，含原书源字段、`errorMessage`、`failedAt`、`expiresAt`。无结果为 `[]`，不触发网络检测；缓存读取异常明确返回 `500`，不会伪装成空结果。 |
| 上游兼容读取 | `POST /api/reader3/getInvalidBookSources` | JWT 适配端点，仅返回当前用户的同一短期失败信息；不新增客户端依赖。 |
| 正常源请求失败 | 内部记录 | `POST /api/search`、单源分页、换源候选、远程书籍创建/刷新/换源、章节内容、探索以及手动健康检测中的真实请求错误，记录当前用户+源；客户端取消不记录。 |
| 正常搜索/候选 | 内部过滤 | 未过期且源未更新的记录使该用户的普通多源搜索/换源候选跳过该源，等价上游 600 秒抑制。显式手动“失效检测”仍可请求该源，且仅真实错误刷新失败记录。 |

`SourceManager` 的 `health` intent 在打开时先调用 `GET /sources/invalid` 并填充现有 `health` 映射、`failedOnly` 视图；不得调用 `POST /sources/batch-test`。手动“失效检测”仍是唯一的即时批量请求入口。关闭/重开会重新加载短期缓存，避免把过期结果长期留在前端状态。

## 实现前测试闸门

1. API：用户 A 的搜索失败只在 A 的 `/sources/invalid` 可见；用户 B 和匿名请求看不到；过期行不会返回且会被清理。
2. API：失败搜索在 600 秒内跳过该用户的源；空结果/客户端取消不创建失败记录；手动健康检测可在失败视图为空时检测全量源。
3. API：章节拉取、刷新/换源、候选搜索和探索的真实源异常都写入同一行，更新 `failedAt/expiresAt` 而不重复累积。
4. API：编辑/删除书源后旧记录不展示、不影响新配置；响应错误永不包含认证头、cookie、完整 query、文件路径或远端响应正文。
5. Frontend：进入 `health` intent 仅请求失效列表、不会请求 `batch-test`；现有失败源立即在 `failedOnly` 视图可见，关闭后状态重置。
6. 数据：AutoMigrate 旧 SQLite 数据库后既有书源/书架/进度不变；新表不进入备份；全量 Go/前端测试和 Docker 挂载卷验证通过。

## 实现记录

- `models.SourceFailure` 和 `services/sourcefailure.Service` 添加唯一的 `user_id + source_id` 短期行，按 UTC 写入 600 秒到期时间；读取时删除过期行、忽略并清理书源 URL 已变或已删除的行。
- `GET /api/sources/invalid` 返回当前书源行及安全的 `errorMessage/failedAt/expiresAt`；`POST /api/reader3/getInvalidBookSources` 保留为上游读取语义的 JWT 适配。两者均不触发源请求。
- 多源搜索、单源分页、换源候选、批量/调试源测试、远程书籍创建/刷新/换源、探索与章节请求的真实错误都会记录；`context.Canceled` 和空结果不会记录。普通搜索/候选读取活动记录后跳过该用户的源；手动检测不受抑制。
- 章节内容请求现在会在远程抓取失败时返回客户端安全错误而不是 `200` 空内容，避免旧实现造成的阅读页空白。源编辑、删除、导入、清空和恢复默认会清理相应的派生失败记录。
- `SourceManager` 的 health intent 加载缓存失败行填充既有 `health`/`failedOnly` 状态，仍不会发起 `batch-test`；显式按钮仍是唯一即时检测入口。

验证：`backend/api/source_failure_contract_test.go` 覆盖用户隔离、抑制重试、过期、取消、编辑后的过时记录和手动检测；`frontend/tests/sourceFailureCacheContract.test.mjs` 覆盖 API/状态约束；`scripts/smoke/source-workspace-contract.mjs` 在 1440×900、390×844、360×800 真实浏览器中确认缓存显示且不会自动检测。
