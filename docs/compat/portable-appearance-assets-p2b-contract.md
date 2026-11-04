# P2-B 可移植自定义外观资产合同

状态：**2026-07-27 已完成固定上游审计、失败测试、运行时实现、全量自动测试、
三视口真实 Go + Chromium 回归、本地 Docker 新旧卷门禁与双架构镜像发布。**
本文件固定普通逻辑备份、portable v1 与新增 portable v2 的兼容边界。

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

## 8. 当前实现证据（2026-07-23）

- 新 trigger 生成 `openreader-portable-v2.json`，打包当前用户实际引用的
  cover/background/font 字节；相同 `(kind, extension, sha256)` 内容只生成一个 slot。
  普通逻辑 ZIP 不变，已有 v1 继续恢复且不解释资产占位符。
- 恢复器先严格检查规范 manifest、未知字段/未来版本、ZIP 路径、大小、hash、magic、
  引用闭包和 legacy 计数，再把占位符改写为目标用户随机 URL。数据库失败会删除本次
  新文件；启动 journal 只清理未被数据库引用的崩溃窗口文件。
- `POST /api/backup/portable/trigger` 返回 v2 的 `localBooks/assets/legacyAssets`；
  列表按实际根 manifest 区分 v1/v2/invalid；工作台文案明确“完整可移植备份”。
- `backend/services/backup/portable_assets_test.go` 与
  `backend/api/portable_appearance_assets_p2b_contract_test.go` 覆盖普通 ZIP 不变、
  去重、跨 user ID、v1、未来版 fail closed、非法资产零写入、数据库失败补偿和
  startup journal。
- Go 全量、前端 558/558 与生产构建通过；
  `scripts/smoke/portable-appearance-assets-real-api-contract.mjs` 在
  1440×900、390×844、360×800 真实验证目标用户书架封面、Reader 背景/字体、刷新、
  移动工具层并存和 legacy 链接。
- 本地 arm64 候选通过新卷 v2 自定义资产跨用户恢复、手工 v1 兼容、重启后资产读取，
  以及历史 TXT、EPUB、UMD、CBZ、相对缓存、用户隔离、普通备份和 portable v2
  跨卷恢复；历史门禁由保留现场逐项复现、带行号跟踪和正式命令复跑确认。
- 从远端 Git 提交 `54a528f4c0d697c72778fcd9062221629ea54fda` 在本机完成
  `linux/amd64`、`linux/arm64` 构建并推送
  `ghcr.io/changshengyu/openreader:54a528f` 与 `latest`。两个标签均经
  `docker buildx imagetools inspect` 核验为 OCI index
  `sha256:047f9636a78604a1d5320da2972d0b16256b95d47253320b79095eaf6101a571`。

## 9. 书架封面浏览器断言复核（2026-11-03，门禁 4 维护，R1–R3）

状态：**contract-only**。

问题：`scripts/smoke/portable-appearance-assets-real-api-contract.mjs` 376–377 行在第一个视口 1440×900 失败：
恢复后书架行 `.list-cover` 的计算 `background-image` 只有默认渐变
（`radial-gradient(circle at 76% 18%, …), linear-gradient(135deg, rgb(251, 250, 244) 0%, rgb(244, 240, 223) 100%)`），
不含 `restored.coverURL`。`69c27bc`（main）与 `claude/reader-menu-alignment` 上同样失败，早于 Reader 菜单切片。

固定上游 `fa22f27`：

- `web/src/views/Index.vue` 589–598：书架卡片 `.cover-img` 内是
  `<el-image class="cover" :src="getCover(getBookCoverUrl(book), true)" fit="cover" lazy>`，即真实 `<img>`，不是 CSS 背景；
  3295–3302 行 `.cover-img`、`.cover` 均为 84×112。
- 同文件 2729–2731：`getBookCoverUrl(book)` 为 `book.customCoverUrl || book.coverUrl`。
- `web/src/main.js` 78–104：`getImagePath` 只把 `http(s)://`、`//` 地址交给 `/cover?path=` 代理，其余（本地
  `/assets/...`）为 `apiRoot + url`；`getCover(url, true)` 直接返回该地址。

当前 OpenReader（`69c27bc`）：

- `frontend/src/views/Home.vue` 63–68：`.cover-img` 内 `<BookCover class="list-cover" :book="book" />`。
- `frontend/src/components/BookCover.vue`：`<img :src="bookCoverUrl(book)">`，加载成功给图片加 `loaded`、外层加
  `has-cover`，失败回到“暂无封面”占位；外层始终保留共享渐变底色。
- `frontend/src/utils/bookCover.js` 1–8：`customCoverUrl` 优先，其次 `coverResourceUrl`/`coverUrl`。
- 恢复后的 `customCoverUrl` 是目标用户同源 `/uploads/users/<target-id>/covers/<random>.png`；脚本已在 API 层断言
  前缀并按字节核对文件。

历史：脚本在 `54a528f`（2026-07-23）写成时，书架 `.list-cover` 是带内联 `background-image: url(...)` 的 `<span>`。
`ceb4baa`（2026-07-27，远程封面代理，见 [`book-cover-proxy-p2-contract.md`](book-cover-proxy-p2-contract.md)
“加载失败回退”“重复封面逻辑”两行）把书架收敛到 `BookCover` 的 `<img>`，同时改了 `index-workspace-contract.mjs`，
没有迁移本脚本的书架断言。此后 `.list-cover` 的背景永远只是渐变底色，该断言自 `ceb4baa` 起必然失败。

实测（`69c27bc` 构建、真实 Go 服务、Chromium，临时插桩副本，未提交）：三个视口书架行 `.list-cover` 都含
`<img src="/uploads/users/<target-id>/covers/portable-….png" class="loaded">`，`complete`、`naturalWidth` 1、不透明度 1、
`object-fit: cover`、与 84×112 封面框重合，外层有 `has-cover`；脚本其后的 Reader 背景、字体与设置面板步骤三个视口
全部通过。

| 项 | 分类 | 处理 |
|---|---|---|
| R1 书架封面用 `<img>` 渲染，`customCoverUrl` 优先，同源地址直接加载 | 与上游一致（`el-image` 即 `<img>`） | 不改产品代码 |
| R2 恢复映射：目标用户新随机 URL、字节一致 | 已符合本合同第 3、6 节 | 不改后端 |
| R3 浏览器合同读取书架封面已不存在的 CSS 背景 | **must-fix（测试工具）** | 迁移断言 |

目标：浏览器合同在书架行 `.list-cover` 内等待 `img.loaded`，断言其 `src` **等于** `restored.coverURL`（精确匹配，
比原来的子串包含更严），`naturalWidth × naturalHeight` 为种子 PNG 的 1×1；写法沿用
`local-book-cover-contract.mjs` 的 `loadedCover`。不再读取 `.list-cover` 的 `background-image`。没有新增允许差异。

验证计划：

1. 旧断言在 `69c27bc` 构建上的失败（上面的确切信息）就是本节的红；产品行为正确，不另造产品红测，也没有 `fix:` 提交。
2. 新断言在同一构建三视口通过；两项临时变异（不提交）证明它能拦下真实回归：M1 `bookCoverUrl` 忽略
   `customCoverUrl`（书架不显示恢复封面）、M2 恢复封面地址 404（图片加载失败回到占位）。
3. 前端 `npm test`、`npm run build`，后端 `go test ./...`、`go vet ./...`。

不重开：书架布局与封面几何（`bookshelf-visible-layout`）、封面代理与 capability（`book-cover-proxy-p2`）、portable v2
格式与恢复事务。`claude/reader-menu-alignment` 改了本脚本 343 行的设置按钮选择器，与 376–377 行不冲突。

记录（2026-11-03，取代本节状态）：合同 `2727bc9`、断言迁移 `df0ac15`（`test:`；产品无改动，没有 `fix:` 提交）。
红：旧断言在 `69c27bc` 构建上于 1440×900 失败（本节开头的确切信息），从未走到另外两个视口。绿：同一构建上新断言
1440×900、390×844、360×800 全部通过；`claude/reader-menu-alignment`（`3bc8788`）加上本断言迁移（自动合并，与 343 行
改动不冲突）三视口同样通过。临时变异（未提交）都在 1440×900 被拦下：M1 忽略 `customCoverUrl`、M2 封面地址加
`/api` 前缀而 404，都停在 “shelf cover image did not load: 暂无封面”；追加的 M3 可加载的 `?stale` 地址停在
“shelf cover does not use restored URL”，旧的子串断言会放过它。前端 855/855 与 build、后端 `go test ./...`（31 个包
通过、7 个无测试）与 `go vet ./...`、公告门自测 109/109 通过；实时公告扫描未跑：本节不涉及依赖，Go 实时门被
GO-2026-5932 阻塞，另案处理。状态 **implementation-complete / regression-validated**（仅测试工具）。本分支只改文档与
该脚本，都不在 `docker-publish.yml` 路径过滤内，单独合入 main 不会触发镜像发布。
