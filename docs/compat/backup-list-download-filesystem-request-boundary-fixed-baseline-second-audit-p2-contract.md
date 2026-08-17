# 备份列表/下载文件系统请求边界第二轮固定基准合同

状态：**aligned / regression-validated / Docker-pending**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只审计 `GET /api/backup/list` 与 `GET /api/backup/download/:name` 的 caller root、文件类型、
文件名和打开后句柄边界。普通/portable 生成、logical/v1/v2 格式、restore、WebDAV 协议、唯一工作台
和生成 request lifecycle 继续由已有专项合同约束，不因本次直接安全反例重开。

## 1. 权威源码

### reader-dev

- `src/main/java/com/htmake/reader/api/controller/WebdavController.kt:230-379`
- `web/src/components/WebDAV.vue:292-301`

固定上游 WebDAV list/download 在认证用户 home 下解析目标；download 拒绝缺失目标和目录，再发送该文件。
OpenReader 的独立 backup list/download 路由、JWT/effective WebDAV permission、多用户 caller root 和
logical/portable 文件名前缀是已发布的 Go 运行时适配。OpenReader 的 rooted/symlink fail-closed 是允许的
多用户安全增强，不能扩大固定上游可见文件集合。

### OpenReader

- `backend/api/backup.go:100-151`
- `backend/api/portable_assets.go:584-610`
- `backend/api/helpers.go:145-159`
- `backend/services/webdavfs/service.go:38-193`
- `docs/compat/api-contract.md`
- `docs/compat/portable-local-archive-backup-p1e4-contract.md`

## 2. 稳定动作合同

| 路径 | 成功语义 | 安全/错误语义 | 本轮保持 |
|---|---|---|---|
| `GET /api/backup/list` | 返回调用者根内可安全打开的普通 `backup_*.zip` 与 `portable_backup_*.zip`；保持 `{name,size,time,format}`，logical 与既有 portable format 投影不变 | JWT + effective WebDAV access 优先；缺失根仍为 `200 []`。非普通文件、不安全根/祖先、symlink、目录、特殊文件和不匹配名称不列出；单个坏项不阻断其它合法备份 | 管理员 legacy root、普通用户私有根、数组字段、时间/大小与 portable-invalid 兼容投影 |
| `GET /api/backup/download/:name` | 从已验证且已打开的同一 caller-root 普通文件句柄返回原始 ZIP 字节 | 非 basename/不匹配名称 `400 {"error":"invalid backup name"}`；缺失、symlink、目录、特殊文件、不安全根或检查/打开身份变化统一为不泄露存在性的安全 `404`；其它内部失败为固定安全 500 | 旧 URL、Bearer JWT、权限优先、生成文件名和 ZIP bytes |

允许名称必须是单个 basename，前缀为 `backup_` 或 `portable_backup_`，并以大小写不敏感 `.zip`
结尾。兼容人工放入 caller root 的历史大写扩展名，但不接受只匹配前缀的任意文件。

## 3. Inventory 证据与差异

1. `backupFileNameAllowed` 当前只检查两个前缀，不要求 `.zip`。因此 `backup_not_zip.txt` 会被 list 暴露，
   direct download 返回其任意字节，违反现有 `backup_*.zip` / `portable_backup_*.zip` 合同。
2. `listBackups` 使用 `os.ReadDir` 后直接信任 entry，并忽略 `entry.Info()` 错误。symlink 不是 directory，
   会进入结果；portable 项还会按路径再次 `zip.OpenReader`。root/ancestor/entry 均没有 scoped symlink 检查。
3. `downloadBackup` 只做 `filepath.Base`/前缀检查后调用 `c.File(path)`。该调用重新按路径打开并跟随
   symlink；目录也进入 `net/http` 的 redirect/index 路径。检查与打开之间没有 same-file 身份保证。
4. 在已发布 `ghcr.io/changshengyu/openreader:cd3a17c` 上的真实 HTTP 反例确认：普通用户私有根中的
   `backup_escape.zip -> /app/data/outside-caller-root-secret` 被 list 暴露，download 返回 `200` 和 caller root
   外内容；`backup_not_zip.txt` 被 list/download 且返回 `200`；`backup_directory.zip` direct download 返回
   `301`。这不是仅凭静态代码推断的风险。
5. `webdavfs.NewScoped` 已检查 trusted WebDAV boundary 到 caller root 的每个现有组件；`Service.Open`
   已执行 rooted resolve、`Lstat`、普通文件检查、打开后 `Stat` 与 `os.SameFile`。本轮应复用该已发布
   helper，不增加另一套字符串路径规则。

判定：**must-fix**。当前实现直接违反既有备份下载合同与多用户 rooted-path 安全要求；这是新运行时
证据，不是从旧日志重开已签收的生成/恢复模块。

## 4. Rooted opened-file 边界

- 权限和 caller root 解析先于文件名之外的任何 stat/open。regular user 只能以
  `data/webdav/users/<safe-username>` 为 root，管理员继续兼容 `data/webdav`。
- root service 使用 `webdavfs.NewScoped(data/webdav, callerRoot)`；list 不调用 `EnsureRoot`，避免一个只读
  请求创建目录。缺失 root 投影为空列表；root/祖先 symlink 或非目录 fail closed。
- list 对每个候选先做严格 basename/prefix/extension，再通过 scoped service 打开普通文件；不安全项
  单独跳过。`size/time` 来自打开后 `File.Stat()`；portable format 从同一 `io.ReaderAt + size` 判断，不能
  在检查后再次按 path 打开。
- download 从 `Service.Open(name)` 得到的同一 `*os.File` 发送，并始终关闭句柄；不得回退 `c.File(path)`、
  `os.ReadFile(path)` 或第二次 path open。客户端断开只终止本次响应，不删除或改写备份。
- 文件在 `Lstat` 与 open 间被替换时，`os.SameFile` 失败并安全拒绝；原文件句柄打开后，同名路径被替换
  不得改变本次响应字节。

## 5. 响应、日志与兼容边界

- invalid/missing/unsafe 响应不得包含 `DataDir`、caller root、目标名之外的 host path、symlink target、
  OS 错误、ZIP member、JWT 或 WebDAV 凭据。list 不返回内部错误字段。
- 不新增 list 字段，不改变合法 logical/portable 的 `format`，不验证 logical ZIP 内容完整性；portable
  invalid/future manifest 继续投影 `portable-invalid`，restore 仍负责完整 archive policy。
- 不扫描、删除、重命名或修复现有不安全/非 ZIP 项；它们只从 backup API 视图中隐藏。原始 WebDAV
  协议继续按其独立合同管理 caller root 内文件。
- 不改表、migration、backup member、manifest、prefix、生成位置、download URL、前端 API 或持久设置。

## 6. 测试先行门

应用实现前必须在旧实现上锁定失败：

1. 普通用户 root 的 `backup_escape.zip` symlink 指向 caller root 外 sentinel：list 不出现，download 不得
   返回 sentinel；root/`users`/用户目录任一祖先 symlink 同样 fail closed。
2. `backup_not_zip.txt`、prefix-mismatch `.zip`、目录、FIFO/设备/socket 均不列出且不可下载；对象保持
   原样，不被 API 清理。
3. 合法 logical、portable v1/v2、portable-invalid 普通 ZIP 保持 name/size/time/format 和逐字节 download；
   缺失 root 为 `200 []`，缺失 download 为安全 404，权限优先不变。
4. 检查后路径替换为 symlink/另一文件不能改变已打开响应；同一打开句柄在随后同名 path replacement
   后仍返回原 bytes。
5. 管理员 legacy root、两个 regular user 私有根与跨用户同名文件保持隔离；响应/日志无 host path。

实现后运行 focused/race、Go 全量/vet、frontend 全量/build、真实 HTTP symlink/non-ZIP/directory/valid
探针，以及 fresh/historical/portable mounted-volume 门后才可发布 Docker。

## 7. Inventory 结论

备份 generation lifecycle 发布后，按 route/work amplification 与 path-traversal checklist 继续枚举得到的
下一项确定 must-fix 是 list/download 的文件系统读取边界。当前 inventory 只新增/更新合同文档，没有
修改应用或测试。

## 8. 测试先行实现记录

- 合同 `b9deec2` 与旧实现红测 `d7810ca` 先后落地；红测在未修复实现上复现 symlink 外逃、prefix-only
  非 ZIP、目录重定向、特殊文件暴露、不安全祖先和非固定 missing 错误。
- `listBackups` 现在先建立 caller-root `webdavfs.Service`，再逐项用严格 basename/prefix/extension 过滤
  和 `Service.Open` 验证；size/time 与 portable format 都来自该打开句柄。缺失或不安全 root 继续投影
  `200 []`，单个坏项不影响合法 ZIP。
- `downloadBackup` 不再使用 `filepath.Base` 静默归一化或 `c.File(path)` 二次按路径打开，而是从同一
  scoped、same-file-verified `*os.File` 调用 `http.ServeContent`。invalid name 固定 400，missing/unsafe/
  non-regular 固定安全 404，其它打开失败固定安全 500。
- focused API、WebDAV/portable/backup 专项、focused race、Go 全量与 `go vet` 均通过；frontend
  741/741 与 Vite build 通过。实现不改 schema、备份格式、生成位置、restore、WebDAV 原始协议或前端。

真实 HTTP symlink/non-ZIP/directory/valid/ancestor 探针与 fresh/historical/portable mounted-volume 门仍须
在本地候选镜像上通过，才可把本合同推进到 Docker published。
