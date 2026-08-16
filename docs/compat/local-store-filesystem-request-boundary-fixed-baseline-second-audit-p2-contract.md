# LocalStore 文件系统与请求边界第二轮固定基准合同（P2）

状态：**aligned / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
当前审查基线：`OpenReader@c7e669e87c5e5b282405be4cef6468981b35763f`。  
审查日期：2026-08-12。

## 1. 范围

本合同复审以下已部署动作的文件系统与 HTTP 请求边界，不重建已签收的工作台 UI、导入确认
状态机或 parser：

- `GET /api/local-store`
- `GET /api/local-store/download`
- `POST /api/local-store/directory`
- `PUT /api/local-store/rename`
- `POST /api/local-store/upload`
- `DELETE /api/local-store`
- `POST /api/local-store/import-preview`
- `POST /api/local-store/import`

`/webdav` 原生协议已由 `webdavfs.Service` 和 WebDAV P2 合同签收；本轮只把同等级的 rooted、
symlink-safe 文件系统约束落实到 LocalStore，不改变 WebDAV 路由。

## 2. 固定上游权威行为

权威文件：

- `src/main/java/com/htmake/reader/api/YueduApi.kt:235-244`
- `src/main/java/com/htmake/reader/api/controller/BookController.kt:2263-2531`
- `web/src/components/LocalStore.vue:180-333`
- `src/main/java/com/htmake/reader/verticle/RestVerticle.kt:73`

固定上游的可见合同是：每次打开列当前目录；多选文件上传到当前目录；上传成功刷新列表；单项/
批量删除先确认；可导入文件进入预览，再由 Index 的确认状态机加入书架。上传 chooser 不限制后缀，
后端遍历所有 `fileUploads()` 并按原文件名逐个写入；同名目标会被覆盖。目录列表跳过点名前缀。

上游用 `BodyHandler.create()` 统一解析 body，并以 `home + path` 拼接文件路径。它没有模块级
actual-read、part 数、JSON cardinality、symlink 或最终解析路径合同。上述弱边界不是产品行为，不能
复制到 Go 多用户/挂载卷实现。

## 3. 当前 OpenReader 映射与缺口

| 层 | 当前行为 | 裁决 |
|---|---|---|
| 路由与权限 | JWT/活动追踪先运行；每个动作再读取 `canAccessStore`。管理员保留历史 `library/localStore` 根，普通用户使用 `library/localStore/users/<safe-name>`。 | **aligned / 保留**。拒绝必须发生在读取 body 或访问文件系统前。 |
| 可见上传 | Vue 发送一个 `path` 和多个同名 `file` part；成功 `201 {path,paths}`。 | **technical-stack-equivalent / 保留**。不改多选、不加后缀白名单、不改响应 shape。 |
| 文件 admission | 默认每文件 128 MiB，可由 `MaxImportBytes` 收紧；写入同目录临时文件后 rename；后续 part 超限时，已成功的前序文件保留且同名旧目标不被截断。 | **aligned / 保留**。多文件继续是逐文件独立提交，不伪造整批事务。 |
| multipart wire | `c.PostForm("path")` 会先触发完整 multipart 解析；随后才按 `FileHeader.Size`/流复制限制每文件。没有请求总包络、part 数或唯一 path/file-field 约束。 | **must-fix**。大量小文件、额外字段或 chunked body 可在业务限制前消耗磁盘/内存。 |
| multipart 生命周期 | handler 不调用 `MultipartForm.RemoveAll()`；真实 `net/http` 请求结束会清理，但 handler 直接调用、异常控制流和复用测试没有明确所有权。 | **must-fix**。成功获得 form 后所有退出路径由 handler 清理。 |
| JSON wire | directory/rename/import-preview/import 使用无界 `ShouldBindJSON`，接受第一个 JSON 后的第二文档/尾随数据；导入数组没有请求级 cardinality。 | **must-fix**。小元数据动作与可能触发 parser/DB 的导入动作分别设界限。 |
| 路径解析 | `localStorePath` 只做 lexical `Abs`/prefix；`os.Stat`、`ReadDir`、`Open`、`Mkdir(All)`、`Rename`、`RemoveAll` 和导入读取会跟随已存在的 symlink。 | **must-fix / 高风险**。挂载卷中的 symlink 可把读、写或删操作引向用户根外。 |
| 目录导入 | 旧 API 可递归展开目录；当前没有展开文件数上限，也会直接按 host path 二次打开。工作台已不暴露目录导入。 | **compatibility-only + must-fix boundary**。保留旧能力，但必须稳定排序、有界展开并使用验证后的 regular file。 |

## 4. 目标 API 合同

### 4.1 通用身份和路径

1. `AuthRequired` 和 `requireLocalStoreAccess` 继续先于 body 解析、目录创建、stat 或 open。无 token
   为 401，无权限为 403；超大未认证 body 仍先返回 401/403。
2. 所有 `path`、文件名和 import item path 必须是有效 UTF-8。相对路径最大 4,096 bytes，文件名
   最大 255 bytes。为兼容固定上游，`/` 与单前导 `/subdir` 仍解释为当前用户根/相对路径；NUL、
   `//`/UNC、Windows volume、`..` segment 和路径分隔符文件名返回 400，不能把任意 host absolute
   path 静默改写为相对路径。
3. 以配置的 LocalStore 根为 trusted boundary，以当前用户 root 为操作根。每个已存在 path component
   用 `Lstat` 验证；root、祖先、目标或目标父目录出现 symlink/特殊文件都 fail closed。
4. 列表跳过 symlink/特殊子项而保留同目录安全项；直接访问 symlink 或经 symlink 访问 descendant
   返回路径无关的 400。失败不得读取、覆盖、移动或删除根外字节。
5. 下载必须在打开后复验同一 regular file，不能在验证路径后再按字符串二次打开。导入 staging
   同样只能从已验证的 regular file 读取。

### 4.2 `POST /api/local-store/upload`

- 请求总包络为 `maxLocalImportBytes + 1 MiB`，同时检查 declared `Content-Length` 和实际读取；
  chunked/未知长度使用同一上限。超过上限返回 413
  `{"error":"local store upload request is too large"}`，且不得创建目录或文件。
- 允许省略 `path` 表示当前用户根；若出现只能有一个。只允许同名 `file` 文件字段，文件数
  `1..64`；额外文件字段、重复 path、额外 value 字段、超长 metadata 或无文件返回 400。
  无文件继续使用 `{"error":"file is required"}`，其它形状错误使用
  `{"error":"invalid upload request"}`。
- 每文件仍受既有 `maxLocalImportBytes` 实际复制上限；超限继续 413
  `{"error":"local book exceeds maximum import size"}`。请求包络不替代每文件校验。
- 文件按 multipart 顺序逐个提交并按原 basename 投影到 `paths`。若后续文件 admission/写入失败，
  已成功的前序文件继续保留；失败文件的旧同名目标保持原字节。重复目标名沿用顺序覆盖语义。
- 只有完整解析并验证请求形状后才能创建 target directory。只要 parser 获得了 multipart form，
  handler 必须在所有成功/失败退出路径执行 `RemoveAll()`。

### 4.3 兼容 JSON 动作

- `POST /api/local-store/directory` 与 `PUT /api/local-store/rename`：actual-read 最大 16 KiB，只接受
  一个非 null JSON object 和尾随 whitespace。超限为 413 `request body too large`；malformed、第二
  JSON 或尾随垃圾沿用各自动作现有 400 文案，且不得发生文件系统变更。
- `POST /api/local-store/import-preview` 与 `POST /api/local-store/import`：actual-read 最大 1 MiB，
  只接受一个非 null JSON object；`paths` 与 `items` 不得同时表达两套输入，总请求项最多 200，
  category IDs 继续按当前用户校验。超限为 413 `request body too large`，形状/cardinality 错误为
  400；不得创建 stage、book、chapter、category relation、cache 或广播。
- 旧目录导入最多展开 200 个可导入 regular files，按 normalized relative path 大小写不敏感稳定
  排序。超过上限在 preview/import 开始任何 stage/DB 写前整体 400；symlink 和 special file 不计入
  可导入项且不可读取。
- 正常 preview 的 `{items}`、import 的 `{imported}`、item-level parser error、token 重试/消费、
  partial item success 和 post-commit bookshelf broadcast 保持现有合同。

## 5. 数据与允许差异

- 不修改 SQLite schema、现有 row、`data/`、`cache/`、`library/` 目录布局、管理员历史根、普通用户
  私有根、backup/WebDAV 成员或 import token 格式。
- 保留 OpenReader REST 路由、JWT、`canAccessStore`、`201 {path,paths}`、多同名 `file` part、
  每文件上限、原子同名替换，以及 `.text/.md/.pdf` 等旧直连 parser 兼容。
- 不复制上游路径拼接、manager secret、弱 symlink 处理或无资源界限。LocalStore 与 WebDAV 共享
  hardened filesystem primitive 属于 Go 安全适配，不合并两者的权限或持久根。
- 已存在的 symlink 不删除、不迁移，只对 API 隐藏并拒绝穿越；这是 fail-closed 安全差异，不是
  破坏性数据迁移。

## 6. 测试先行门禁

应用代码修改前，新增测试必须先在旧实现上精确失败：

1. declared 与 chunked multipart `max+1 MiB+1` 均为 413；未认证/无权限仍优先 401/403；失败无新目录。
2. 64 个小文件成功且顺序稳定；65 个、额外 file key、重复 path、额外 value、超长 path/filename
   均为 400 且零写入。配置为 8 bytes 时精确 8 bytes 成功、9 bytes 保持既有每文件 413。
3. 将 Gin `MaxMultipartMemory` 降低后，成功、文件超限、路径拒绝和后续 part 失败均无法再打开
   parser 创建的 `multipart-*` 临时文件；前序成功文件和失败项旧目标语义保持。
4. directory/rename 的 declared/chunked 16 KiB+1、第二 JSON、尾随垃圾不变更文件系统；import 两
   动作的 1 MiB+1、双输入、201 项不创建 stage/book/cache 或广播。
5. 对每个 LocalStore 动作放置 root/ancestor/target symlink 与根外诱饵：list/download/create/rename/
   upload/delete/preview/import 均不能读取或改变诱饵。列表仍返回同目录 regular neighbor。
6. 安全 nested directory、普通文件、多文件覆盖、管理员旧根、普通用户私有根、token reparse、
   parser budget 和前端多选上传合同继续通过。

完成红测后才可实施；合同、红测、实现、运行探针和发布记录必须分别提交。实现后至少运行 focused、
Go 全量、race、vet、frontend test/build、真实 declared/chunked HTTP 探针，以及 fresh/historical
`data/cache/library` 卷和 portable backup/restore 门。

## 7. 实施与回归记录（2026-08-12）

- 合同 `69145fc`、旧实现红测 `8c78775`、实现 `bba99e1` 和真实 HTTP 探针 `930be4d` 已按门禁顺序
  独立提交并推送。旧实现证据覆盖 declared/chunked 聚合 body、65/额外/重复 multipart part、临时文件、
  多 JSON、201 项/目录展开，以及每个 LocalStore 动作穿越 symlink 的真实失败，不以静态审查代替。
- `localstore_boundary.go` 现在在权限之后、目录创建之前执行 `maxLocalImportBytes + 1 MiB` actual-read
  multipart 包络、`1..64` 同名 file part、唯一 path 和 UTF-8 byte/path 校验，并由 handler 统一
  `RemoveAll`。directory/rename 使用 16 KiB、preview/import 使用 1 MiB single-JSON；请求和展开均最多
  200 项，完整计划成功前不 stage、不写库。
- LocalStore 复用 `webdavfs.Service` 的 caller-rooted 逐组件 `Lstat`、同目录 staged replace 和 opened-file
  identity recheck。共享 primitive 同时补齐 special-file 拒绝：非 regular/directory 目标不能被上传覆盖、
  rename 替换、删除或列出；symlink/special 既不迁移也不删除。
- focused、Go 33 包全量、focused race、`go vet ./...`、frontend 740/740、Vite production build 和
  `git diff --check` 通过。宿主二进制与本机 arm64 candidate `930be4d` 均运行
  `scripts/smoke/local-store-filesystem-request-boundary-contract.mjs`，确认 34 MiB 配置下 declared/chunked
  413、认证优先、32 MiB 以上磁盘 multipart 零临时文件、201 文件整体拒绝、symlink/FIFO fail closed、
  下载字节和源文件删除后的 token 确认导入。
- 本机 candidate 镜像 revision 为 `930be4dded3d8b54985606e92c45b7484115ffa7`，仅完成 arm64 本地构建；
  fresh/historical/portable 卷脚本因当前 Docker socket 提升权限额度审批被拒而未执行。因此没有发布
  `930be4d` 或改写 `latest` 的远端标签；正式 GHCR 仍是 `be83a0f`/`latest`，OCI index
  `sha256:e1f31f3dd728bc27fbc89bbc8c21f81e8c5511c5e99196891feb21cd47138b73`。卷门恢复前不得标记
  Docker-published。

## 8. Docker 发布记录（2026-08-16）

后续 `65a9870` candidate 重新运行 LocalStore 真实边界探针，确认 34 MiB multipart declared/chunked
包络、磁盘临时文件清理、200/201 展开、symlink/FIFO、download/open identity 和 source-independent token。
同一镜像顺序通过 fresh/historical `data/cache/library`、logical/portable v1/v2、跨用户和重启门禁，并由
本机发布 `65a9870`/`latest`。远端 amd64/arm64 OCI index 为
`sha256:255c81b43dbb7f49c707d6c609b920aa183b730401ad1c1ca32157eb0a945c71`；GHCR 回拉 health revision
与 `65a987049d6a9bff7feeb2618f7257620cd896a9` 一致。未修改 SQLite schema、mounted root 或备份格式；
用户生产环境运行 commit 仍未知。
