# 用户资产文件系统与引用生命周期第二轮固定基准合同（P2）

审查日期：2026-09-10

状态：**inventory-complete / tests-and-implementation-pending**

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只复审已部署的封面、阅读背景、字体与 misc 用户资产在下列边界中的物理文件与
SQLite 引用生命周期：

- `POST /api/uploads`、`DELETE /api/uploads`；
- `POST /api/books`、`PUT /api/books/:id` 引入 `custom_cover_url` 的分支；
- `PUT /api/settings/:key` 引入当前用户资产 URL 的分支；
- portable v2 资产收集、目标分配、promote/rollback 和启动 journal 清理；
- 逻辑/portable restore 对 Book/Setting 资产引用的持久提交。

已发布的上传 wire 预算、内容 magic/图片尺寸、公开 rooted same-file 读、BookInfo/Reader 可见交互、
portable v2 格式和跨用户重写保持关闭。

## 1. 权威证据

### reader-dev

- `src/main/java/com/htmake/reader/api/controller/UserController.kt#uploadFile/deleteFile`
- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `/reader3/uploadFile` 与 `/reader3/deleteFile`
- `web/src/components/ReadSettings.vue#uploadBGFile/uploadFontFile/deleteCustomBGImg`
- `web/src/components/BookInfo.vue#onCoverFileChange`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt#saveBook`

固定上游认证后把原文件复制到当前 namespace 的 `/assets/...`，删除只做 namespace 前缀检查后
`deleteRecursively`。阅读设置与 BookInfo 按“上传→写引用”的可见顺序工作。上游的原名覆盖、
跟随 symlink、无内容校验、无引用检查和先删文件后改配置不是 OpenReader 的产品合同。

### OpenReader

- `backend/api/uploads.go#uploadAsset/deleteAsset/userUploadAsset/userUploadAssetReferenced`
- `backend/api/books.go#createBook/updateBook/validateBookCustomCoverURL`
- `backend/api/settings.go#updateUserSetting`
- `backend/api/portable_assets.go#allocatePortableAssetTarget/promotePortableAssets/cleanupPortableAssetRestoreJournals`
- `backend/services/backup/portable_assets.go#validatePortableAssetReference`
- `backend/api/backup_restore_plan.go#restoreLegadoBackupDataWithPermissions`
- `backend/api/upload_resource.go#uploadPublicResource`
- `frontend/src/composables/useReaderAppearanceAssets.js`
- `frontend/src/composables/useOverlayBookInfo.js`

OpenReader 使用 JWT user ID、随机文件名、`/uploads/users/<user>/<kind>/<name>` 稳定 URL、上传内容校验、
引用中 `409` 和 portable v2 资产闭包。这些是已签收的多用户/安全/可迁移适配，本轮只使其
物理路径与引用提交符合已声称的语义。

## 2. 当前差异矩阵

| 合同点 | 当前 OpenReader | 裁决 |
|---|---|---|
| 上传 root | `os.MkdirAll` 和 Gin `SaveUploadedFile` 只用词法路径；Gin 内部再 `MkdirAll` + `os.Create` | **P2 must-fix**：`uploads`、`users`、user/kind 任一已存组件为 symlink/特殊文件时可越出 `data/uploads` 写入 |
| 上传发布 | 验证后直写最终随机名；copy/close/取消失败可留部分 final file；随机源失败回退全零名并可覆盖 | **must-fix**：context-aware private stage、fsync/close、无覆盖 publish，失败零 final |
| 删除 root | URL 仅词法归一后 `os.Remove(asset.Path)`；祖先 symlink 可使删除落在根外 | **P2 must-fix**：只删除 rooted current regular entry；symlink/目录/FIFO/device/socket 均 fail closed |
| 封面验证 | `os.Stat` 跟随 symlink，且文件验证与 Book transaction 分离 | **must-fix**：只接受当前 caller root 内的 rooted regular same-file，提交前再验证 |
| 引用删除竞争 | `userUploadAssetReferenced` 与 `os.Remove` 之间无协调；Book/Setting 可在检查后引入同 URL | **must-fix**：写引用与删除共享 caller-scoped coordinator，结果只能是引用先提交后删除 `409`，或删除先完成后新引用失败 |
| Setting 新引用 | `updateUserSetting` 只校验 JSON，不区分历史失效 URL 与新引入 URL | **must-fix with compatibility**：保留当前行已有历史/缺失 URL；仅对 next value 新增的当前用户 URL 执行 rooted current-file admission |
| portable 导出 | `validatePortableAssetReference` 以 `EvalSymlinks(userRoot)` 作为新边界，未证明该 root 仍在 `data/uploads` 内；后续又按 path 重开读 | **P2 must-fix**：从受信 uploads boundary 验证所有祖先，对同一 opened regular handle 校验/摘要/打包 |
| portable 恢复/清理 | `MkdirAll`/`EvalSymlinks(userRoot)` 也可把根外当新根；journal 清理重用词法路径 + `os.Remove` | **P2 must-fix**：promote/rollback/journal 只操作 rooted current entries，不触碰根外或替换后实体 |
| 公开读 | `uploadPublicResource` 已使用 `webdavfs.New(...).Open` 拒绝 symlink/特殊文件并从同一 handle 服务 | **closed / preserve**：不重开 GET/HEAD/MIME/Range/304 合同 |
| 可见交互 | Reader 已先保存引用再删旧资产；BookInfo 上传后精确 patch 封面 | **closed / preserve**：不改 UI、文案、面板、route 或上传成功顺序 |

## 3. Rooted 文件生命周期

1. 受信边界是经绝对归一的 `data/uploads`，不是经 `EvalSymlinks` 后的 user root。每个已存组件从
   uploads root 到 `users/<caller>/<kind>` 均必须是 non-symlink directory；root 本身是 symlink/普通文件时
   fail closed。
2. 上传在已验证的 caller/kind 目录中创建 request-private stage，以 request context 有界复制，执行
   file sync/close 后以“目标不存在”语义发布。copy、sync、close、取消、随机生成或 publish 失败
   删除 stage，不留 final，不覆盖旧资产。
3. 随机源失败必须整个上传失败，不再使用全零 fallback。成功 URL 仍保持当前 kind、扩展名和
   不可预测服务端名；响应 `201 {url,name,size,type}` 不变。
4. 删除前以 rooted `Lstat -> open/current identity` 证明目标是 caller 目录内当前 regular entry。在同一
   coordinator 内再验证引用和 entry identity 后移除；不允许路径检查后跟随新祖先或删除
   替换文件。原本就不存在的安全路径继续幂等 `200 {deleted:true}`。
5. portable export 必须从同一 rooted opened handle 完成大小、magic、digest 与 ZIP copy，不在验证后按路径
   重开。portable restore 的 allocate/stage/publish/rollback 与 journal cleanup 遵守同一 uploads boundary。

## 4. 引用提交资格

1. 为当前用户新引入 `/uploads/users/<caller>/...` URL 的 Book/Setting 写入，必须在共享 caller-scoped
   asset coordinator 内验证 rooted current regular file，并持有该协调边界到 SQLite commit。当前 Book/Setting 已
   持有的相同 URL 不触发新 admission。
2. Setting 引用以有界 JSON 递归遍历完整字符串值，不以 SQL `LIKE` 的子串命中定义“新增”。原行已有
   legacy URL、外部 URL 或缺失的当前用户 URL 可原样保留/移除，不因普通设置保存被破坏；只有
   next-old 集合中新增的当前用户 URL 需要存在性/所有权/kind 验证。
3. Book 新引用仍只允许 caller `covers` URL。在事务内重读 Book，以当前行判断 URL 是新增还是原值，
   并在 commit 前复验文件 identity。现有显式列更新、owner/404/400、category 事务与 durable-only
   `bookshelf_update` 不变。
4. 删除同样在 coordinator 内重查 Book 精确 `custom_cover_url` 与所有 Setting 精确 JSON 字符串引用。写入
   先完成时返回现有 `409 upload is still in use`；删除先完成时，后续新引用返回现有
   Book `400 invalid custom cover url` 或 Setting 平面 `400`，不得提交断引用。
5. 逻辑备份依然是 URL-only，因此历史 ZIP 中原本缺失的 URL 可按已签收语义恢复，不追溯生成文件。
   但恢复中指向当前 caller 且当时存在的 URL 不能与并发删除产生新断引用；portable v2 受控
   新资产必须将 promote 与重写行 commit 置于同一协调边界。

## 5. API、错误与可见行为

- JWT、方法、路径、33 MiB multipart/16 KiB JSON、单 part/单文档、8/32 MiB 文件限额、内容签名/
  图片尺寸、成功 shape 不变。
- unsafe root、symlink、特殊文件、替换 identity、随机/stage/publish 错误为现有 path-free `400` 或 `500`，
  不回显主机路径、inode、SQLite 或 parser 文本。caller cancellation 不合成响应。
- Reader 的上传→显式 setting save→旧资产 best-effort delete 和 BookInfo 的上传→精确 Book patch 保持。
  BookInfo patch 失败后的新文件仍是当前已签收的可安全后续清理 orphan，本轮不新增 UI 流程。
- 公开 `GET|HEAD /uploads/*` 及 MIME/Range/304 完全不变。文件写入成功后仍可用返回 URL 直接加载。

## 6. 数据、迁移与允许差异

- 不新增/删除 SQLite 表、列、索引、migration marker；不增加新根或环境变量。
- `data/uploads`、Book `custom_cover_url`、UserSetting JSON、逻辑 ZIP、portable v1/v2 manifest/占位符和 WebDAV
  格式不变。已有 regular 资产不扫描、移动、重命名或重写 URL。
- 已有 legacy/外部/缺失 URL 保持可读与备份语义；未受控 symlink/特殊文件从此对新写、删除、
  portable 读写 fail closed，这是明确安全差异。
- caller-scoped coordinator、rooted opened handle、request-private stage 和无覆盖 publish 是 Go/SQLite/挂载卷适配，
  不改变固定上游的可见上传、选择和删除结果。
- 回滚旧镜像继续读同一卷，但会重新引入跟随祖先 symlink、部分 final file、路径重开和引用竞争风险。

## 7. 测试先行门

实现前用确定性 barrier/注入点在旧实现上证明：

1. 把 `uploads`、`users`、caller 或 kind 任一祖先替换为指向根外的 symlink；上传不得创建/覆盖根外
   文件，删除不得移除根外文件，Book 封面不得引用该路径。root/目录/FIFO/device/socket/入口
   symlink 同样 fail closed，错误不含 path。
2. 在上传 copy、sync、close、publish 和 caller cancellation 分别注入失败；stage 收敛、零 final、零 DB/event。
   固定随机失败不生成全零名；重复目标不覆盖旧 bytes。
3. 删除完成引用查询后暂停，并发 Book/Setting 引入同 URL，再继续删除。最终只允许“写入成功+
   删除 409+文件存在”或“删除 200+新引用失败+无行引用”，不得两个 200 后断引用。
4. Book 封面完成文件验证后暂停，并发删除或替换实体；Setting 同样覆盖递归嵌套字符串、
   新增与原有缺失 URL。陈旧 identity 不提交；历史失效 URL 可原样保留或移除。
5. portable export 在验证后替换文件/祖先，ZIP 不得包含根外或替换 bytes；根外 user-root symlink 不能被
   `EvalSymlinks` 重定义为新安全根。portable restore promote/DB failure 与 journal cleanup 不得删除替换或
   根外文件。
6. 两用户相同 basename/kind 与并发操作互不影响。已有安全 regular asset 的 upload/read/reference-409/
   delete、BookInfo 封面、Reader 背景/字体、portable v1/v2 跨 user ID 恢复保持。
7. 实现后运行 focused/race、相邻 upload/public-read/Book patch/Setting/portable tests、Go full/vet、frontend full/build、
   Compose、BookInfo/Reader 资产四视口，以及可信 Actions fresh/historical/portable/platform 门。

## 8. Inventory 结论

判定：**must-fix**。已发布合同证明了 wire/content/user-ID 隔离、静态 rooted 读和顺序引用保护，但
旧上传测试只在普通目录上检查最终结果，没有覆盖挂载卷祖先 symlink、写入中途失败、文件
identity 替换或引用查询与删除之间的并发写。`user-asset-write-boundary` 合同第 4 节已明确把跨请求
原子引用模型留给后续专项，本轮是对该未完成边界的新取证，不重开已签收 UI 或 wire 模块。

下一步必须先提交本合同，再在旧实现上添加确定性红测，最后实施共享 rooted asset storage、
context-aware staged publication 和 caller-scoped reference coordinator。本 inventory 不修改应用或测试代码。
