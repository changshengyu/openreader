# 管理员删除用户工作区文件系统生命周期第二轮固定基准合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只提取合同并记录当前反例，不修改应用或测试代码。范围仅是：

- `POST /api/admin/users/batch-delete`；
- `POST /api/admin/cleanup-inactive` 的兼容后端动作；
- 两个动作在 SQLite 提交后的 regular-user 私有工作区清理。

用户管理 UI、管理员认证、请求 body/cardinality、SQLite 删除事务、source namespace 和 WebSocket
recipient scope 已有专项合同，本轮不得借文件系统修复重开。

## 1. 固定上游与当前映射

固定上游权威入口：

- `src/main/java/com/htmake/reader/api/controller/UserController.kt#deleteUsers`；
- `web/src/components/UserManage.vue#deleteUserList`。

上游在管理确认后，从 `data/users` 删除所选用户，并对
`storage/data/<username>` 执行递归删除。OpenReader 保留现有管理员确认和批量 REST 适配，但因
SQLite、多用户 WebDAV、LocalStore、本地归档、上传和封面缓存而扩展为：

- `data/webdav/users/<safe-username>`；
- `library/localStore/users/<safe-username>`；
- `library/data/<safe-username>`；
- `data/uploads/users/<user-id>`；
- `cache/cover-images/user-<user-id>`。

多目录清理是允许的数据布局适配；它不能把固定上游的“目标用户 namespace”放宽为任意词法后代。

## 2. 当前反例与差异矩阵

当前 `privateUserWorkspacePath` 只做 `filepath.Abs` 和字符串前缀检查，清理直接调用
`os.RemoveAll(path)`。2026-09-14 的隔离文件系统反例把 `root/users` 建成指向 root 外的符号链接后，
`os.RemoveAll(root/users/victim)` 返回成功并删除了外部 `victim/sentinel`；链接本身仍存在。这证明
已发布的普通目录测试不能支撑现有 API 合同中的“validated private descendants”。

| 合同点 | 当前 OpenReader | 裁决 |
|---|---|---|
| 配置根与祖先 | 只验证绝对词法前缀；中间 symlink 会被 `RemoveAll` 跟随。 | **P2 must-fix**：从受信配置根打开 rooted handle，逐组件拒绝 symlink/特殊文件，绝不触碰根外实体。 |
| 最终用户目录 | 不验证类型和打开实体；最终 symlink 会被直接移除。 | **must-fix**：只清理当前 regular directory；symlink、regular file、FIFO、socket/device 均 fail closed。 |
| 校验到删除竞态 | 没有 identity snapshot 或 detach；校验后替换可改变删除对象。 | **must-fix**：在已打开父目录内按同一 identity 脱离活动名字，再递归清理隔离名字。 |
| 历史用户名 | `SafeFilename` 可把不同旧用户名投影到同一路径。 | **must-fix data compatibility**：若任一未删除用户共享安全目录名，目录必须保留并计清理失败。新账号 ASCII 规则不用于重写旧账号。 |
| SQLite 与文件顺序 | 一个事务删除全部 owner row，提交后逐目录清理；失败返回计数，不回滚 DB。 | **aligned / preserve**：文件删除不可事务回滚，必须保持 DB-first；失败不得删除其他用户数据或泄露 host path。 |
| 缺失目录 | `RemoveAll` 把缺失视为成功。 | **aligned / preserve**：历史卷可缺少任意派生目录，仍为成功且不增加失败数。 |

## 3. 目标 API、数据与错误合同

- 两个管理员动作的成功 status/body 不变：批量删除返回
  `{deleted:number, cleanupFailures:number}`；无不活跃用户仍返回 `{deleted:0}`。
- SQLite transaction 仍先删除 caller 选中的普通用户及全部 owner row/source namespace；事务失败时
  零工作区删除。提交后文件失败不能复活已删除数据库行。
- 每个工作区只能以其配置根为受信边界：`DataDir`、`LocalStoreDir`、`LibraryDir` 或 `CacheDir`。
  配置根自身必须是当前打开的真实目录；从根到父目录的任一 symlink、非目录或打开 identity 变化均
  fail closed。
- 缺失的安全路径是幂等成功。现存目标必须是目录；在已打开且 identity 已验证的父目录内，将该目录
  原子重命名为请求私有 quarantine 名，再验证移动实体仍与计划 identity 相同，之后才递归删除。
- 验证后目标被替换为 symlink/文件/其它目录时，不得跟随或删除替换实体；可恢复时恢复原名字，否则
  只报告清理失败。quarantine 名必须随机且不得覆盖已有项。
- 目标目录内部的 symlink 或特殊文件不得把递归清理带出已打开的 rooted parent；外部 sentinel 必须
  保持。挂载点/平台不支持的 rooted 操作返回清理失败，不回退到裸 `os.RemoveAll(path)`。
- 删除前按完整持久 username 计算现有安全投影；若未删除用户与目标用户的 `SafeFilename` 相同，三个
  username 路径均保留。ID 路径仍可独立清理。不得迁移、重命名或禁用旧账号。
- API 和日志只能暴露安全计数及 user ID；不得返回/记录配置根、完整目标路径、symlink target、文件
  内容或底层错误文本。
- 不修改 SQLite schema、JWT、URL、backup wire、环境变量和 `data/cache/library` 的现有目录名。

## 4. 测试先行闸门

实现前必须先让以下合同在当前代码上失败：

1. 四个受信配置根分别覆盖 root、祖先和最终 entry symlink；批量删除和 cleanup-inactive 均删除 DB
   row，但根外 sentinel/链接保持，`cleanupFailures` 为稳定非零安全计数。
2. 最终 entry 为 regular file、FIFO 或其它特殊文件时 fail closed；安全缺失目录不算失败。
3. 在计划/验证后、detach 前把目标替换为指向根外的 symlink，根外内容和替换链接保持；原目标不得被
   错当作新实体删除。
4. 正常目录完整清理，另一个用户和管理员 legacy 根保持；DB transaction 注入失败时所有目录保持。
5. 构造两个历史用户名投影到同一 `SafeFilename`，删除其中一个时保留共享 username 路径和存活用户
   内容；目标的 ID 路径仍可删除，旧用户可继续登录/访问既有数据。
6. 清理错误正文、日志和测试诊断不包含配置根、外部路径或底层错误；同批多个失败只返回数值。

实现后运行 focused user-management/admin filesystem tests、focused race、`go test ./...`、
`go vet ./...`、frontend 全量与 production build。该切片没有 UI 几何变化，无需重做用户管理截图；
发布候选仍须通过 Compose、fresh/portable/historical mounted-volume 和双架构平台门。

## 5. 实施边界

实现应使用 Go 1.24 rooted filesystem handle（或等价 dirfd 操作）完成逐组件验证、同父目录 detach 和
root-confined recursive removal。不能用 `EvalSymlinks` 后的目标作为新信任根，也不能在失败时回退
`os.RemoveAll(absolutePath)`。现有 cover-image service 自己的 rooted cleanup 继续保留；不得因工作区
清理而削弱其缓存锁、capability 或统计合同。

## 6. 实施、回归与发布结果（2026-09-14）

- 合同先以 `386f555` 独立提交。旧实现红测 `0f61853` 随后通过两个真实 Gin 管理 API，稳定证明
  WebDAV、LocalStore、本地归档、uploads 与 cover cache 的祖先 symlink 会删除根外 sentinel；另一个
  用例证明两个历史 username 的 `SafeFilename` 碰撞会删除存活用户的共享目录。
- 实现 `016a346` 新增 Go 1.24 兼容的 `services/rootedfs`：从当前配置根打开 `os.Root`，逐组件验证，
  在已打开父目录句柄内用 `renameat` 把同一 inode 移入随机 quarantine，再用 `openat/unlinkat` 递归
  清理。最终/祖先 symlink、非目录、identity 替换均 fail closed，内部 symlink 只移除链接本身。
- 用户删除计划在 SQLite transaction 内查询未删除账号的完整 username 投影；碰撞的三个 username
  路径保留并计清理失败，目标账号独有的 uploads/cover ID 路径仍可清理。数据库先提交、缺失路径
  幂等和 path-free `cleanupFailures` 保持不变。
- focused service/API、focused race、`go test ./...`（API 73.563 秒）、`go vet ./...`、frontend
  `754/754`、Vite production build、Compose 和 `git diff --check` 全部通过。本切片无 UI 几何变化，
  因此没有重复截图。
- 可信 GitHub Actions run `34825873958` 通过 backend/frontend/build/Compose、native image、fresh/
  portable、historical volume 和 published-platform 门，发布
  `ghcr.io/changshengyu/openreader:016a346` 与 `:latest`。amd64/arm64 OCI index 为
  `sha256:50031b016e22c18d6c06634ed4e809be9570bff48f8840dffae3d460b1595881`；amd64 manifest 为
  `sha256:6c68839b7906baad032e5c11edb1f4d50d27786fe796c2a11126857905bec767`，arm64 manifest 为
  `sha256:a2dc36bfdcdf5f6fe98c8199c3fecaa93fd181b777bae2de7393e7b1c14ae20c`。其余
  `unknown/unknown` 条目是 build provenance attestation。
