# WebDAV 导入/恢复文件系统与请求边界第二轮固定基准合同（P2）

状态：**aligned / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
inventory 审查基线：`OpenReader@930be4dded3d8b54985606e92c45b7484115ffa7`。
当前实现基线：`OpenReader@616a076571b61a548f4f5f5779192a275b06516c`。
审查日期：2026-08-16。

inventory 阶段只提取合同、不修改应用或测试；合同、正式红测和实现随后已分阶段落地。原生
`/webdav/*`、`/reader3/webdav/*` 协议已由
[`webdav-protocol-p2-contract.md`](webdav-protocol-p2-contract.md) 签收；逻辑/portable ZIP 内容、事务、
所有权和唯一文件管理器已由
[`backup-restore-fixed-baseline-p2-contract.md`](backup-restore-fixed-baseline-p2-contract.md) 签收。本合同不
重开这些行为，只关闭工作台从 mounted WebDAV 读取书籍/备份时仍存在的 HTTP wire、目录展开和
opened-file 边界。

## 1. 权威文件与可见流程

### 固定上游

- `web/src/components/WebDAV.vue:145-357`
  - 每次打开从根目录加载；只显示当前目录；支持单个/批量选择、上传、下载、删除、ZIP 恢复和
    TXT/EPUB/UMD 加入书架；
  - 单项和批量导入都向 `importFromLocalPathPreview` 发送有序 `path` 数组和 `webdav:true`；
  - 恢复只由用户在 `.zip` 行确认后发送一个 path。
- `src/main/java/com/htmake/reader/api/YueduApi.kt:220,301-320`
  - 注册导入预览、WebDAV 文件管理、恢复和备份动作。
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:2262-2315`
  - 逐个读取所选路径并返回预览；WebDAV 权限先于 mounted path 读取。
- `src/main/java/com/htmake/reader/api/controller/WebdavController.kt:515-793`
  - 用户 WebDAV 根、文件列表/上传/下载/删除、`restoreFromWebdav` 和 `backupToWebdav`。

### 当前 OpenReader

- `backend/api/server.go:204-211`
  - `POST /api/backup/restore-webdav`
  - `POST /api/webdav/import-preview`
  - `POST /api/webdav/import`
- `backend/api/webdav.go`、`backend/api/webdav_boundary.go`、`backend/api/request_body.go`
  - WebDAV restore/import handler、bounded single-JSON admission、完整 import plan 和 scoped opened read。
- `backend/services/webdavfs/service.go`
  - caller-rooted path、逐组件 symlink 检查、regular-file `Stat/Open` 和 opened identity recheck。
- `frontend/src/components/WebDAVBrowser.vue`、`frontend/src/components/overlays/OverlayStorageImport.vue`
  - 唯一文件管理器、确认恢复和逐本 preview/import 状态机。
- `frontend/src/api/webdav.js`、`frontend/src/api/backup.js`
  - 当前稳定请求和响应映射。

## 2. 动作合同与允许差异

| 动作 | 固定上游可见语义 | 当前稳定 OpenReader 合同 | 裁决 |
|---|---|---|---|
| `POST /api/webdav/import-preview` | 一个或多个可见 WebDAV 文件先解析目录和元数据，再交给共享导入确认流程。 | JWT + effective WebDAV permission；接受 `{paths}` 或 `{items}`；返回 `200 {items}`，每项为 `{path,book,importToken}` 或 item-level error。 | 保留稳定 API、immutable token 和逐项错误；补 request/cardinality/rooted-open 边界。 |
| `POST /api/webdav/import` | 用户确认后逐本加入书架，批量模式仍有逐本可见结果。 | 接受 paths/items/category；成功项持久化并消费 token，失败项保留 token；返回 `200 {imported}` 并只广播成功书。 | 保留逐项提交，不改成 all-or-nothing 批事务；但整份请求和目录展开必须在首个 stage/DB/event 前通过 admission。 |
| `POST /api/backup/restore-webdav` | 只在用户确认某一 `.zip` 后恢复；源 WebDAV 文件保留。 | 接受 `{path}`；逻辑或 portable restore 返回既有 count/skip 字段；source 行仍受 `canEditSources`。 | 保留 ZIP/事务/权限合同；补 bounded single JSON、regular opened snapshot 和 caller-private staging。 |

以下为明确允许差异：JWT/Bearer 与独立 effective WebDAV permission；管理员 legacy 根和普通用户私有根；
稳定 `/api/*` 路由；immutable preview token；有界 parser/archive；OpenReader logical/portable ZIP；现有旧 API
可读取的额外本地格式。可见 WebDAV 入口仍只显示固定上游 TXT/EPUB/UMD，不能借本轮扩大 UI。

## 3. 旧实现缺口与 must-fix（已关闭）

1. 三个 handler 都直接 `ShouldBindJSON`。声明长度或 chunked 实际读取没有上限，且第一个合法 JSON
   后的第二文档/垃圾会被忽略；有效第一文档仍可 stage、写书架或恢复备份。
2. preview/import 对 `paths`、`items`、`categoryIds` 和目录展开没有 cardinality。一个目录可在同一请求
   中触发无界文件枚举、parser stage、SQLite 写和事件结果分配。
3. `webdavPath` 只在顶层请求 path 调用 `Service.Resolve`，随后把 host path 交给 `os.Stat`、
   `filepath.WalkDir` 和 `os.Open`。目录内的 symlink/special file 没有逐项经过 `Service.Stat/Open`；
   symlink 可读取根外文件，FIFO/device/socket 可阻塞或进入 parser。resolve 后到重新 open 之间也没有
   opened identity 约束。
4. `restoreWebDAVBackup` 先 stat path，再让 restore helper 按 path 重新 stat/open。恢复内容可受这两个
   时点之间的 rename/replace 影响，且没有证明源是 caller-rooted regular file。
5. raw WebDAV 协议已经使用 `webdavfs.Service`，不能用它的绿测替代上述三个 `/api/*` handler 的证据。

## 4. HTTP wire 与 admission 合同

1. JWT middleware 和 `requireWebDAVAccess` 必须先于 body decode、root 创建、path resolve、stage、DB、
   parser 和 backup restore。无凭据 `401`，有效身份无权限 `403`；超大恶意 body 也不得改变优先级。
2. import-preview/import 使用 **1 MiB actual-read** 上限；restore-webdav 使用 **16 KiB actual-read**
   上限。声明长度或 chunked 超限都返回 `413`，不能只检查 `Content-Length`。
3. body 必须恰好一个非 `null` JSON object。第二 JSON、尾随非空 token、数组/标量/null、语法错误为
   `400`；不产生 stage、SQLite、文件、event 或 restore 副作用。
4. import 请求只能二选一使用 `paths` 或 `items`。原始 `paths`、`items`、`categoryIds` 各最多 200；
   空请求为 `400`。每个 JSON path 必须为有效 UTF-8、无 NUL、最多 4096 bytes，并通过共享 WebDAV
   relative-path normalizer；Windows volume、UNC 和 `..` fail closed。
5. 先 normalize、去重并完整规划所有请求项，再开始 preview/import。所有请求 path 合计展开后的唯一
   importable regular file/token target 最多 200；第 201 项返回 `400`，且零 stage/DB/cache/event
   副作用。排序继续使用当前 case-insensitive relative-path 稳定顺序。
6. 通过 admission 后保留既有逐项产品语义：单项 parser/持久化失败只形成该项 error；其它合法项可按
   用户所见顺序成功。不能把 fixed-upstream 多项流程缩成不可区分的一笔失败。

## 5. caller-rooted 文件系统与 snapshot 合同

1. mounted WebDAV boundary 和认证用户 root 继续由现有 `storeRoot` 决定，不移动目录。source-backed
   读取使用一个不创建缺失目录的 `webdavfs.Service`；token-only retry/import 不 resolve source、
   不要求源存在，也不创建 WebDAV 根。
2. 显式请求的 root/ancestor/target symlink 或 special file 整体 fail closed。递归目录中的 symlink
   不跟随，special file 不打开；安全相邻 regular file 可继续进入计划。任何返回/日志不得含 host path。
3. 每个 source-backed preview/import 文件必须以 normalized relative path 调用 `Service.Open`，并由
   opened handle 的 regular mode 与 same-file recheck 证明身份，再执行既有 `maxLocalImportBytes`
   actual-read。不能把 `Resolve` 返回的 host path 重新交给 `os.Open`。
4. restore-webdav 必须以 `Service.Open` 打开 `.zip` regular file，将同一 opened handle 在现有 portable
   compressed limit 内复制到 `cache/backup-uploads/<user-id>/` 的私有临时文件，再调用既有 restore
   planner。成功/失败都删除临时副本；mounted 源永不改写、移动或删除。打开后的源被 rename/delete/
   replace 不得改变本次恢复字节。
5. ZIP archive entry、expanded-size、logical transaction、portable staging/compensation、source permission
   和 post-commit broadcast 继续由既有备份合同负责。本轮只固定 mounted source 到私有 snapshot，
   不重写 archive 语义。

## 6. 稳定响应与错误语义

- preview 成功继续返回 `200 {items:[...]}`；无匹配项是空数组，不伪装 transport failure。
- import 成功继续返回 `200 {imported:[...]}`；每个成功 book 和 item error 的 shape 不变。
- restore 成功继续返回现有 count、`sourcesSkipped`、portable localBooks/assets 等加性字段。
- body/cardinality/path admission 使用 `400/413`；缺失/非 ZIP restore source 保留 client-safe `400`；
  parser/单文件读取失败保持 item-level error。内部错误不得暴露绝对路径、token、SQL 或 archive detail。
- importToken 仍只在 durable import 成功后消费；reparse/parser/DB 失败保留同一 caller-scoped token。

## 7. 数据与迁移边界

- 不新增或修改 SQLite schema；不重写现有 book/progress/source/category/setting 行。
- 不移动、删除或 chmod `data/webdav/`、管理员 legacy tree、`users/<safe-name>/`、`library/` 或已有
  backup ZIP。预先存在的 symlink/special file 留在卷上，但 API 不跟随或导入。
- import preview 和 restore snapshot 都是 caller-scoped derived cache，可在失败/过期/成功清理；不进入
  logical/portable backup，不成为 mounted source 的替代迁移。
- 旧路由、响应、import token 和历史 archive 保持可读；本轮没有启动时扫描或后台批量迁移。

## 8. 失败测试与实现结果

合同提交后先新增失败测试，覆盖：

1. 三动作 declared/chunked 超限、第二 JSON/垃圾/null；断言认证优先和零 stage/row/event/restore。
2. 201 个 raw paths/items/categoryIds、单目录 201 个 importable files、重复 path；第 201 项必须在首个
   stage/DB/event 前拒绝，200 项保持稳定排序。
3. 请求目录内放置指向根外诱饵的 `.txt` symlink 和可控 FIFO/special file；preview/import 不读取诱饵、
   不阻塞、不返回 host path，安全 regular 邻项仍可处理。root/private-parent/显式 target symlink 继续拒绝。
4. source 在 preview 后删除，token reparse/import 仍使用原始 staged bytes且不访问 mounted root。
5. restore 从 regular ZIP 的 opened snapshot 工作；在 open 后替换/删除源不改变恢复结果。symlink、FIFO、
   directory、非 ZIP 和 oversized source 均零写入失败，源文件保持不变。
6. existing raw WebDAV Bearer/Basic/DAV 方法、唯一 WebDAV 管理器、TXT/EPUB/UMD UI gate、logical/
   portable restore transaction 和 private-root tests 全部继续通过。

验证门：focused API/service、focused race、`go vet ./...`、Go 全量、frontend 全量/build、宿主真实
declared/chunked HTTP、1440x900/390x844/360x800 WebDAV 导入与恢复 smoke，以及 fresh/historical
`data/cache/library` mounted-volume + logical/portable backup/restore。合同、红测、实现、runtime probe、
Docker 发布记录必须单独提交。合同 `cf46e22`、旧实现红测 `1bb904a`、实现与 runtime probe
`616a076` 已依次推送；Docker 仍等待本地 candidate 与卷门。

## 9. 非本轮范围与后续排序

- `POST /api/backup/restore-legado` 已有 compressed multipart actual-read envelope 和 archive limits；其
  singular part/handler-owned multipart cleanup 仍应在独立 wire audit 中证明，不与本轮 mounted-path
  修复混为一项已完成结论。
- direct local import/source import multipart、远程 search/source-debug/cache JSON、progress 和其余 batch
  JSON 仍在动作差集内。WebDAV 本项优先是因为同时具备 mounted data、目录展开、special-file 和恢复
  事务入口；完成后重新按副作用与 remote-work 风险排序。

## 10. 旧实现 inventory probe（2026-08-16）

为避免只依赖静态审查，本 inventory 用 Go `-overlay` 从 `/tmp` 注入一次性 package-local probe；仓库
没有新增测试或应用文件。命令在 `OpenReader@930be4d` 运行四项并全部按“旧缺口存在”通过：

```bash
cd backend
GOCACHE="$PWD/.gocache" go test \
  -overlay=/tmp/webdav_inventory_overlay.json \
  ./api -run '^TestInventoryProbe' -count=1 -v
```

观测结果：

1. `import-preview` 接受合法第一 JSON + 1 MiB 空白 + 第二 JSON，返回 `200` 并创建 importToken。
2. 请求一个普通目录时，其内部 `.txt` symlink 指向根外诱饵；返回 `200`，stage `.book` 字节与根外
   文件逐字节一致。
3. 一个目录含 201 个 importable regular files 时，返回 201 个 preview item，而不是在首个 stage 前
   拒绝。
4. `restore-webdav` 接受合法 `{path}` + 16 KiB 空白 + 第二 JSON，并对合法 ZIP 返回 `200`。

这些 probe 是固定旧缺口的 inventory 证据；正式红测已经在 `1bb904a` 覆盖 chunked、认证优先、零副作用、
special file、opened restore snapshot 和 exact 200/201 边界。

## 11. 实现与回归记录（2026-08-16）

`616a076` 完成以下边界，不改变成功响应、SQLite schema、mounted root 或备份格式：

1. import-preview/import 使用 1 MiB、restore-webdav 使用 16 KiB actual-read UTF-8 single JSON；声明长度与
   chunked 超限统一 `413`，第二文档、垃圾、null/数组和非法 UTF-8 在副作用前失败。
2. raw paths/items/categoryIds 与去重后的完整目录展开均限制为 200；计划完成前不 stage、不写 DB、
   不广播，exact 200 保持 case-insensitive 稳定顺序。
3. JSON path 通过 `webdavfs.NormalizeImportPath`；source-backed 项逐个使用 scoped `Service.Open` 的
   regular/same-file 复验。嵌套 symlink/special file 不进入 parser，安全相邻文件仍可处理。
4. token-only retry/import 不实例化 mounted service；源删除或根被替换为 symlink 后仍只消费 caller-scoped
   immutable stage。
5. restore-webdav 把同一 opened regular ZIP handle 有界复制到 `cache/backup-uploads/<user-id>/`，再调用
   既有 planner/事务；成功或失败均删除私有 snapshot，mounted ZIP 保持不变。

验证结果：WebDAV API/service 专项无缓存通过，focused race、`go vet ./...`、Go 全量、frontend 740/740、
Vite build 与 diff check 通过；`webdav-import-restore-boundary-contract.mjs` 在真实宿主服务通过 declared/
chunked、symlink/FIFO、token-only 与 snapshot 清理；导入状态机、token 重试和恢复会话隔离均通过
1440x900、390x844、360x800。默认 LocalStore+WebDAV 组合也通过。实现提交时剩余门禁为本地 Docker
candidate、fresh/historical 三卷及 logical/portable backup/restore；下一节记录其后续通过与发布证据。

## 12. Docker 发布记录（2026-08-16）

本地 arm64 candidate `65a9870` 先通过 health revision、WebDAV 与 LocalStore 两个真实文件系统/HTTP
探针。随后 fresh 与 historical 三卷门顺序通过，覆盖 TXT/EPUB/UMD/CBZ/relative-cache、owner isolation、
logical restore、portable v1/v2 assets、cross-user restore 和 restart。正式镜像由本机双架构构建并推送：

- tags：`ghcr.io/changshengyu/openreader:65a9870`、`ghcr.io/changshengyu/openreader:latest`；
- OCI index：`sha256:255c81b43dbb7f49c707d6c609b920aa183b730401ad1c1ca32157eb0a945c71`；
- platforms：`linux/amd64`、`linux/arm64`，另含对应 provenance attestation；
- GHCR 回拉 arm64 容器 `/api/health` 返回 version `65a9870` 和完整 commit
  `65a987049d6a9bff7feeb2618f7257620cd896a9`。

发布不代表用户生产服务器已经升级；其当前运行 commit 仍未知。允许差异仍只有多用户/JWT/private
root、immutable token、有界安全策略与已列出的 OpenReader runtime 适配；没有 SQLite、目录或备份格式迁移。
