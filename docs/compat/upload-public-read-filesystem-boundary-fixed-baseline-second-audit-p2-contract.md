# 上传资产公开读取文件系统边界第二轮固定基准合同

状态：**aligned / regression-validated / Docker-pending**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只审计公开 `GET|HEAD /uploads/*resourcePath` 的 rooted path、文件类型、打开句柄和 HTTP 读取语义。
`POST|DELETE /api/uploads` 的 multipart/JSON、magic、所有权、引用事务，BookInfo/Reader UI 与 portable v2
资产格式继续由已发布专项合同约束，不因本次 mounted symlink 反例重开。

## 1. 权威源码

### reader-dev

- `src/main/java/com/htmake/reader/api/YueduApi.kt:95-106`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt:467-532`

固定上游把当前 namespace 上传到 `storage/assets/<namespace>/<type>/<filename>`，返回公开
`/assets/<namespace>/<type>/<filename>`，并用静态资源 handler 读取。浏览器无需额外 Authorization 即可
加载封面、背景和字体；资源读取不改变用户配置或文件。

### OpenReader

- `backend/api/server.go:215-218`
- `backend/api/uploads.go:43-141,213-243`
- `github.com/gin-gonic/gin@v1.10.0/fs.go`
- `github.com/gin-gonic/gin@v1.10.0/routergroup.go:190-238`
- `docs/compat/bookinfo-shelf-mutations-p2-contract.md`
- `docs/compat/reader-appearance-assets-p2-contract.md`
- `docs/compat/portable-appearance-assets-p2b-contract.md`
- `docs/compat/user-asset-write-boundary-fixed-baseline-second-audit-p2-contract.md`

## 2. 差异矩阵

| 合同点 | 固定上游 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 公开稳定 URL | `/assets/<namespace>/...` 可由页面直接加载 | `/uploads/users/<id>/...` 与 legacy `/uploads/<kind>/...` 公开 GET/HEAD | **technical-stack-equivalent / 保持**；不能改成 Bearer-only 或签名短链，避免破坏 `<img>`、CSS 字体、背景和历史设置 |
| namespace/文件名 | 用户 namespace + 原文件名，可覆盖 | JWT user ID + kind + 随机名；legacy 全局 URL 只读 | **允许的多用户/迁移增强 / 保持**；不移动、不改名、不扫描已有资产 |
| 目录可见性 | 静态资源读取，不构成目录管理动作 | Gin `Static` 禁止目录 listing | **aligned / 保持**；目录、root 和空路径不列举、不重定向到索引 |
| rooted path | 上游没有证明 mounted symlink 安全 | `http.Dir.Open` 跟随 root/ancestor/entry symlink | **must-fix**；公开 capability 只能读取 uploads root 内同一普通文件，不能扩展到 DB、备份或其它 mounted bytes |
| 文件类型 | 返回被选择的资源文件 | `http.Dir` 可尝试打开目录和特殊文件 | **must-fix**；只允许普通文件，symlink、目录、FIFO/device/socket 全部快速 404，不能阻塞请求 |
| 检查/发送身份 | 上游未定义 Go 文件句柄生命周期 | Gin 先 `fs.Open` 检查并关闭，再由 `http.FileServer` 按 path 重开 | **must-fix**；metadata 和响应必须来自一个 `Lstat -> open -> SameFile` 句柄，消除 check/reopen 窗口 |
| HTTP 读取 | 浏览器静态资源语义 | stdlib file server 提供 GET/HEAD、Content-Length/Type、Last-Modified、conditional/range | **aligned / 保持**；安全修复不能把合法图片/字体变成下载附件、JSON 包络或无 Range 流 |

## 3. 稳定读取合同

| 路径 | 成功语义 | 失败语义 |
|---|---|---|
| `GET /uploads/*resourcePath` | 无认证；从 `data/uploads` 内同一已验证普通文件句柄返回原始 bytes；保持按 basename 推导 Content-Type、Content-Length、Last-Modified、条件请求和单 Range 语义 | missing、root/ancestor/entry symlink、目录、特殊文件、绝对/遍历/NUL/反斜杠路径统一为无路径信息 404；无目录 listing/redirect |
| `HEAD /uploads/*resourcePath` | 与 GET 相同的安全打开和成功 headers，不返回 body | 与 GET 相同的安全 404；不得因 HEAD 回退到第二套 path open |

- 允许任意历史层级、Unicode/空格 basename 和扩展名的**普通文件**，因为读取层不能追溯应用未来写入的
  magic/extension 规则；新写入仍只能由已签收的 authenticated upload action 产生。
- legacy `/uploads/<kind>/<name>` 与当前 `/uploads/users/<id>/<kind>/<name>` 都保持可读；公开 URL 是
  浏览器 capability，不新增跨用户 list/write/delete 权限，也不宣称文件名 secrecy 是授权边界。
- 不对资源内容重新做图片/字体解码，不读取 SQLite 引用，不按当前 JWT user ID过滤。读取必须保持
  无 cookie/Bearer 依赖，以兼容 CSS `@font-face`、浏览器缓存和跨页面资源请求。

## 4. Rooted opened-file 边界

- trusted root 固定为 `filepath.Join(DataDir, "uploads")`。服务启动仍可创建缺失 root；GET/HEAD 本身不
  创建、修复、删除、移动或改写任何对象。
- 每次请求检查 root 到目标的每个已存在组件。root、ancestor 或 entry 任一 symlink 均 fail closed；
  lexical `..` 清理、`EvalSymlinks` 后比较或字符串前缀都不能替代逐组件 `Lstat`。
- 目标必须 `Mode().IsRegular()`。先 `Lstat`，再打开，随后 `File.Stat` 与 `os.SameFile` 验证；发送时继续
  使用该 `*os.File`，始终关闭，不得回退 `router.Static`、`c.File(path)` 或 `http.FileServer(http.Dir)`。
- 检查与 open 间替换必须拒绝；成功打开后同名 path 被替换不能改变本响应 bytes。请求取消由
  `http.ServeContent`/net/http 停止响应读取，不删除或截断资源。

## 5. 响应与数据兼容

- 404 body、header 和日志不得包含 `DataDir`、uploads root、symlink target、OS error、SQLite path、
  JWT、WebDAV credential 或文件内容。不存在和不安全对象不可区分。
- 合法资源不增加 `Content-Disposition`，不包 JSON，不改变 URL。保持浏览器 inline 显示、字体加载、
  `If-Modified-Since`/`Range` 兼容；非法 Range 继续由标准库投影 416。
- 不改 SQLite、migration、`data/cache/library` 布局、普通/portable backup、manifest、设置 URL、Book
  `customCoverUrl`、上传响应或删除语义。不安全 mounted 对象只对公开读取隐藏，不自动清理。

## 6. Inventory 证据

1. `server.go` 当前先 `MkdirAll(data/uploads)`，再注册 `router.Static("/uploads", uploadsDir)`。
2. Gin 1.10 的 `Static` 使用 `http.Dir`。其 handler 先 `fs.Open(file)` 检查并关闭，然后调用
   `http.FileServer` 再开一次；`onlyFilesFS` 只禁用 `Readdir`，不拒绝 symlink 或特殊文件。
3. 已发布 `ghcr.io/changshengyu/openreader:2986357` 的真实容器反例确认：
   `uploads/users/1/covers/escape.png -> /app/data/outside-upload-secret` 返回 `200` 与 uploads root 外 bytes；
   `uploads/users/2 -> /app/data/outside-users/2` 的祖先 symlink 也返回 `200` 与目标 bytes。
4. 现有 Go 测试只证明 legacy regular cover 可 GET；没有 root/ancestor/entry symlink、special file、HEAD、
   Range、same-handle 或路径替换合同。

判定：**must-fix**。这是公开未认证读取面上的 mounted path escape，并有已发布运行时证据；它不改变
公开 capability 设计，也不是从旧日志重开已签收的上传写入、Reader 或 portable 模块。

## 7. 测试先行与发布门

应用实现前必须在旧实现上锁定失败：

1. root、`users`、user/kind ancestor 与 entry symlink 指向 uploads root 外 sentinel：GET/HEAD 均为 404，
   不返回 sentinel；对象保持原样。
2. directory、FIFO、Unix socket 和其它非普通文件快速 404；目录不 listing、不 redirect，特殊文件请求
   不挂起。missing、空 path、encoded/raw traversal、NUL 与反斜杠变体安全拒绝。
3. legacy/current/portable-restored regular assets 保持公开 GET/HEAD；逐字节、Content-Type/Length、
   Last-Modified、If-Modified-Since、Range/416 与 Unicode/空格路径兼容。
4. 检查/open 身份变化 fail closed；同一打开句柄在随后同名 path replacement 后仍返回原 bytes。
5. 错误响应与日志无 host path。公开读取不查用户、JWT 或 DB，不改变文件、设置和 Book 行。

实现后运行 focused/race、Go 全量/vet、frontend 741/741 与 build、真实 HTTP/浏览器资源加载，以及
fresh/historical/portable mounted-volume 门；本地多架构发布前回读远端两平台 revision。

## 8. Inventory 结论

backup list/download 发布后继续按公开文件读取与 path-traversal 枚举，下一项确定 must-fix 是
`GET|HEAD /uploads/*resourcePath`。本轮只新增/更新兼容与安全合同，没有修改应用或测试。

## 9. 测试先行实现记录

- 合同 `d0c948c` 与旧实现红测 `7181634` 已按顺序完成；红测在未修复实现上证明 root、ancestor、entry
  symlink 与编码反斜杠文件名都会返回 200，其中三类 symlink 直接暴露 uploads root 外 sentinel。
- `server.go` 保留 startup `MkdirAll`，但把 Gin `router.Static/http.Dir` 替换为显式 GET/HEAD handler。
  handler 拒绝空/双前导斜杠、NUL 和反斜杠，复用 `webdavfs.New/Open` 的逐组件 symlink、普通文件与
  `os.SameFile` 边界，并从同一 `*os.File` 调用 `http.ServeContent`。
- 专项测试覆盖 root/ancestor/entry symlink、目录、FIFO、反斜杠、对象不变，以及 legacy/current
  Unicode/空格路径的公开 GET/HEAD、Content-Length/Last-Modified、304、Range 206 与 416。
- focused API、既有 upload/asset/portable appearance、`webdavfs`、focused race、Go 全量、`go vet`、
  frontend 741/741 与 Vite build 均通过。实现不改写入/删除、URL、schema、备份格式或前端。

真实 HTTP/浏览器资源加载与 fresh/historical/portable mounted-volume 门仍须在本地候选镜像上通过，
才可把本合同推进到 Docker published。
