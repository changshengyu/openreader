# P2-B 可移植自定义外观资产合同

状态：**2026-07-23 已完成固定上游、当前 API/ZIP/SQLite/文件事务审计；尚未实施。**
本文件是失败测试和运行时代码的前置合同，不授权修改普通逻辑备份或既有 portable v1
语义。

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游权威文件：

- `src/main/java/com/htmake/reader/api/controller/WebdavController.kt`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt`
- `web/src/components/ReadSettings.vue`

当前 OpenReader 对应文件：

- `backend/services/backup/backup.go`
- `backend/services/backup/portable.go`
- `backend/api/backup.go`
- `backend/api/portable_backup.go`
- `backend/api/backup_restore_plan.go`
- `backend/api/webdav.go`
- `backend/api/uploads.go`
- `backend/services/assets/validator.go`
- `frontend/src/composables/useWorkspaceBackupActions.js`
- `frontend/src/layouts/AppLayout.vue`

## 1. 上游合同与允许差异

固定上游的 `backupToWebdav` 只在阅读 App 已生成的 `backup*.zip` 上调用
`BookController.saveToWebdav`。后者替换 `bookSource.json`、`bookshelf.json`、
`bookGroup.json`、`rssSources.json`、`replaceRule.json` 和 `bookmark.json` 后重新压缩；
`syncFromWebdav` 也只恢复这些逻辑 JSON 与 WebDAV 阅读进度。它不打包
`storage/assets/<namespace>/...`。

上游 `ReadSettings.vue` 上传背景或字体后，把 `/assets/<namespace>/<type>/<filename>`
写入配置；`UserController.uploadFile` 把字节写在当前用户 namespace，`deleteFile`
也只允许删除该 namespace。由此只能得出“配置保存 URL、文件按用户隔离”，不能得出
“逻辑备份携带文件”。

OpenReader 当前普通备份同样只在 `userSettings.json` 和 `bookshelf.json` 中保存 URL。
portable v1 是另行命名的本地书原 archive 扩展，也没有资产条目。以下裁决固定：

| 项目 | 裁决 |
|---|---|
| 普通 `backup_*.zip` | **保持不变**。继续兼容 reader-dev/Legado/OpenReader 逻辑恢复，不加入资产 manifest、文件或占位符。 |
| `openreader-portable-v1.json` | **保持可生成文件的旧恢复兼容**。v1 仍按 URL 字符串恢复，不臆造缺失资产。新 trigger 不再生成 v1。 |
| 完整跨实例资产恢复 | **允许的 OpenReader 扩展**。新增显式 portable v2，不冒充上游格式。 |
| 新用户根、随机文件名、magic/尺寸校验 | **允许且必须保留的多用户/安全强化**。不得退回上游可覆盖原文件名的写法。 |
| legacy `/uploads/<kind>/...`、外部 URL、内置主题 | **字符串兼容**。不跨用户复制、移动或删除；legacy 引用数量必须在生成/恢复结果中透明报告。 |

## 2. Portable v2 格式

新 trigger 生成 `portable_backup_<timestamp>.zip`，继续包含 portable v1 的普通逻辑
JSON 与本地书原 archive，但根 manifest 改为：

```text
openreader-portable-v2.json
local-books/b0001/original.epub
appearance-assets/a0001.png
appearance-assets/a0002.woff2
```

manifest 最小结构：

```json
{
  "format": "openreader-portable-backup",
  "version": 2,
  "createdAt": "RFC3339 timestamp",
  "books": [],
  "assets": [{
    "id": "a0001",
    "kind": "backgrounds",
    "extension": ".png",
    "entry": "appearance-assets/a0001.png",
    "size": 123,
    "sha256": "lowercase hex"
  }]
}
```

`books` 字段与 v1 完全相同。`assets` 遵守：

1. `id` 仅为包内连续不透明 slot；manifest 和 entry 均不得包含源 user ID、用户名、
   数据库 ID、宿主路径、JWT、cookie、WebDAV 凭证或原始文件名。
2. `kind` 仅允许 `covers`、`backgrounds`、`fonts`、`misc`；扩展名必须满足现有上传
   kind 白名单，且文件内容必须再次通过现有 magic、图片尺寸和像素限制。
3. 每个 entry 必须恰好被一个 manifest 项声明；每个 manifest 项必须至少被一个逻辑
   占位符引用。禁止未声明 `appearance-assets/` 条目、重复 ID、规范化路径冲突、
   大小写冲突和同一 `(kind, extension, sha256)` 的重复项。
4. 相同源 URL 只生成一个 slot；不同 URL 但 `(kind, extension, sha256)` 相同的内容也
   归一到同一 slot。kind 或扩展不同的相同 digest 不合并。
5. 文件条目只保存原始已验证字节，不保存用户目录结构或额外元数据。

## 3. 逻辑 JSON 占位符合同

portable v2 的逻辑副本使用保留 scheme：

```text
openreader-asset://a0001
```

它只存在于 v2 ZIP 内，不得写入源数据库。生成时：

1. 从当前用户全部 `UserSetting.Value` 的合法 JSON 值递归收集**完整字符串值**，并从
   当前用户全部 `Book.customCoverUrl` 收集引用。
2. 只处理精确匹配
   `/uploads/users/<caller-id>/<covers|backgrounds|fonts|misc>/<basename>` 的新资产 URL。
   不做子串替换，不解释查询串、fragment、反斜杠、编码后的分隔符或嵌套路径。
3. 在 portable 副本的 `userSettings.json` 中递归把精确 URL 字符串改为对应占位符；
   在 `bookshelf.json` 中只改 `customCoverUrl`，不把远程 `coverUrl` 当成本地资产。
   Reader 顶层配置和 `customConfigList` 因此走同一递归规则。
4. 当前用户 URL 指向缺失、非 regular、越界、symlink、超限或内容不匹配文件时，
   整个 trigger 失败，不写出部分 ZIP。指向另一用户新资产根的 URL 也失败且不泄露
   owner/path。
5. 源逻辑值若已包含保留 `openreader-asset://` scheme，生成失败，避免把用户数据误作
   包控制指令。
6. legacy `/uploads/<kind>/...` 保持原字符串并计入 `legacyAssets`；外部 URL、data URL、
   `/assets/...` 内置资源不计为待打包资产。

普通逻辑备份和 portable v1 不使用占位符。恢复普通/v1 ZIP 时遇到该 scheme 仍按普通
字符串处理；只有通过 v2 manifest 校验的包可以解释它。

## 4. API 合同

既有路由不增加第二套管理器：

| Method / path | v2 请求与成功响应 | 错误与兼容 |
|---|---|---|
| `POST /api/backup/portable/trigger` | 认证、无 body；生成 v2，返回既有字段加 `format:"openreader-portable-v2"`、`localBooks:n`、`assets:n`、`legacyAssets:n`。 | 缺失/非法/跨 owner 的已引用资产或本地 archive 返回不泄露路径的 `409`；限额为 `413`；失败无最终 ZIP。普通 trigger 不变。 |
| `GET /api/backup/list` | 实际读取根 manifest，把已生成文件标为 `openreader-portable-v1` 或 `openreader-portable-v2`；逻辑包仍为 `logical`。 | 文件名前缀不能代替版本识别。损坏/未知 portable 文件不得伪报为 v1。 |
| `GET /api/backup/download/:name` | 继续下载调用者根内允许 basename。 | 路径和权限合同不变。 |
| `POST /api/backup/restore-legado`、`POST /api/backup/restore-webdav` | v2 成功结果增加 `assets` 和 `legacyAssets`；设置、书籍、本地 archive 仍恢复到认证用户。 | v1 继续原流程。恰好一个受支持 manifest 才能恢复；多个 manifest、未知/未来版本、孤立占位符或资产项在任何写入前返回 `400/413/409`。 |

portable 检测必须识别规范根名 `openreader-portable-v<整数>.json`。未来 v3 不能落入普通
逻辑恢复并只写书架；它必须 fail closed。列表读取 manifest 也使用有界读取和规范路径校验。

工作台动作改名为“保存完整可移植备份”。确认文案明确包含本地书原文件和当前账户实际
引用的自定义封面/背景/字体；成功文案显示书本数、资产数。`legacyAssets > 0` 时追加
“旧版资源仅保留链接”的非成功隐瞒提示。v1 列表/恢复文案仍显示“含本地书原文件”；
v2 显示“含本地书及自定义资源”。

## 5. 限额与安全预检

v2 复用 portable 独立的压缩大小、entry 数、单 entry 和总展开字节限制。资产还必须满足
上传端较小的 kind 限制：图片/cover/background/misc-image 最大 8 MiB，字体/misc-font
最大 32 MiB。所有 logical、local-book、asset 和 manifest entry 一起计入 ZIP entry/总量；
不能因分组而各获得一份总预算。

在第一条数据库或最终资产文件写入前，恢复必须完成：

- ZIP 规范路径、NUL/绝对路径/反斜杠/`..`、symlink、重复名/大小写冲突、entry/压缩/
  展开总量校验；
- 唯一受支持 manifest、严格字段、slot/entry 集合、size、SHA-256 和引用闭包校验；
- 每个资产有界流式复制、实际字节数/hash、kind/扩展/magic/图片尺寸校验；
- 所有逻辑 JSON、占位符、Reader settings、bookshelf 变量、分类/来源映射和本地书
  archive 的既有完整 preflight；
- 目标用户现有 local-book identity 冲突检查及目标资产路径根验证。

客户端错误只返回通用包/资产原因和安全计数，不包含 entry 名、源/目标 host path、
user ID、设置原文或文件内容。日志也不得记录 JWT、WebDAV 密码、完整设置 JSON 或资产
字节。

## 6. 恢复事务与失败补偿

1. 校验后的资产先写到调用者私有 `cache/portable-restores/<user-id>/...` staging，
   权限为 `0700/0600`；staging 名不来自 manifest。
2. 服务端为每个 slot 生成目标用户私有的随机 basename 和最终
   `/uploads/users/<target-id>/<kind>/<random><extension>` URL，不复用源 ID/文件名，
   不覆盖现有文件。
3. 仅在所有预检通过后，把 v2 逻辑计划中的已声明占位符重写为目标 URL。未知、缺失、
   多余或出现在不允许字段的占位符 fail closed。
4. 最终文件采用同文件系统临时文件、`fsync`、原子 rename 写入；随后在**一个 SQLite
   transaction** 中恢复已重写的 UserSetting、Book `customCoverUrl`、自定义方案及其它
   逻辑行。数据库失败/commit 失败时回滚事务并删除本次唯一新文件。
5. 任一资产校验、staging、最终写入或数据库步骤失败时，目标用户原设置、原书籍行、
   原资产文件均不得被覆盖或删除；本次新文件和 staging 必须补偿清理，且不广播 sync。
6. 成功后才广播既有 settings/bookshelf/progress/bookmark 事件。旧目标资产不在恢复时
   自动删除，避免破坏仍被其它行/客户端引用的文件。
7. 文件系统与 SQLite 不能形成真正的跨介质 ACID。实现必须测试上述同步错误补偿；
   进程崩溃窗口使用仅含本次随机新文件的恢复 journal/startup cleanup 收敛，不得通过
   删除旧引用资产来伪造原子性。

portable v1 既有“逻辑 transaction + 每本 archive rehydrate/补偿”保持；v2 资产事务不得
反向改变 v1 的冲突和恢复结果。

## 7. 测试先行与发布闸门

### 失败测试

- service 导出：Reader 顶层/自定义方案/Book cover 收集、同 URL/digest 去重、无源 user ID
  manifest、占位符只进入 v2 副本、legacy 计数、普通/v1 不变。
- service 拒绝：缺失、symlink、越界、跨 owner、magic 错配、超限、保留 scheme 污染，
  且不留下最终 ZIP。
- API 恢复：v1 向后兼容；v2 跨 user ID 写新 URL/字节；Reader、custom config、Book 同一
  transaction；未知版本/多个 manifest/孤立或多余 slot/重复 digest/path/hash/magic/预算
  失败均零数据库、零最终文件、零广播 mutation。
- 注入最终文件写失败、设置/书籍写失败和 commit 失败，断言旧行/旧文件不变、新文件清理。
- 列表按实际 manifest 区分 v1/v2，损坏/未来版不伪报 v1。
- 前端文案、确认、v1/v2 行标签、`assets/legacyAssets` 提示契约。

### 完成门禁

1. `cd backend && go test ./...`
2. `cd frontend && npm test`
3. `cd frontend && npm run build`
4. 真实 Go + Chromium 在 1440×900、390×844、360×800 完成：
   上传背景/字体/自定义封面 → 生成 v2 → 恢复到不同用户 → 刷新 → Reader 与 BookInfo
   仍显示目标用户新 URL；v1 仍可恢复。
5. 本地 Docker 新卷和历史卷执行：
   v1 恢复、v2 跨实例恢复、重启后资产可读、legacy 只保留链接、用户隔离、卷级备份不回归。
6. 只有上述全部通过才可把 P2-B 标记完成并发布本机构建的 amd64/arm64 GHCR 镜像。

