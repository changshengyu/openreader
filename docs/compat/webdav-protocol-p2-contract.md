# WebDAV 原生协议兼容 P2 合同

固定基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

状态：2026-07-19 已完成上游与当前实现审查；实施尚未开始。本阶段遵守
`readerdev-compat-inventory`，只记录合同，不修改应用代码。

本合同只处理外部 WebDAV 客户端协议兼容。工作台 `WebDAVBrowser`、书籍导入预览、逻辑/可移植
备份恢复、用户私有目录和独立 WebDAV 权限已经由此前 P1/P2 合同完成；本批不得重建这些界面或
改变它们的数据流。

## 权威文件

上游：

- `src/main/java/com/htmake/reader/api/controller/WebdavController.kt`
  - `/reader3/webdav*` 路由、Basic 认证和 `enable_webdav`；
  - `OPTIONS`、`PROPFIND`、`MKCOL`、`PUT`、`GET`、`DELETE`、`MOVE`、`COPY`、
    `LOCK`、`UNLOCK`；
  - `DAV`、`Allow`、`MS-Author-Via`、`WWW-Authenticate` 响应头；
  - 一级目录 multistatus XML 和无持久锁的兼容响应。

当前：

- `backend/api/server.go` 的 `/webdav/*path` 路由组；
- `backend/api/webdav.go` 的文件操作和 caller-scoped root；
- `backend/middleware/auth.go`、`backend/api/auth.go` 的 Bearer/JWT 与 bcrypt 用户认证；
- `frontend/src/api/webdav.js`、`frontend/src/components/WebDAVBrowser.vue` 的网页端 Bearer +
  `GET` 目录列表兼容调用；
- `backend/api/workspace_storage_access_contract_test.go` 的权限和私有根证据。

## 上游合同与当前差异

| 范围 | 固定上游行为 | 当前 OpenReader | 判定与目标 |
|---|---|---|---|
| 原始路径 | 外部客户端使用 `/reader3/webdav*`。 | 只注册 `/webdav/*path`。 | `must-fix`：新增 `/reader3/webdav/*path` 兼容别名；保留 `/webdav/*path`，不能破坏已部署网页端和客户端。 |
| 发现与能力头 | 无凭据 `OPTIONS` 可返回 `200`；所有协议响应声明 `DAV: 1,2`、完整 `Allow` 和 `MS-Author-Via: DAV`。受保护请求未认证时带 Basic challenge。 | 没有 `OPTIONS`，Gin 通常返回 `404/405`；没有 DAV/Allow/challenge 头。 | `must-fix`：恢复协议发现。不得复制上游 `Access-Control-Allow-Origin: *` 与 credentials 同时开启的组合；同源网页端保持现有 CORS 策略。 |
| 认证 | secure 模式接受用户名/密码 Basic，验证用户及 `enable_webdav`。 | 只接受 Bearer JWT；常见 WebDAV 客户端无法登录。 | `must-fix with security adaptation`：同一路径接受 Bearer 或 Basic。Basic 用现有 bcrypt hash，成功后设置同一个 user id；随后仍检查 `CanAccessWebDAV`。无凭据/坏密码 `401`，有效身份无权限 `403`。密码、Authorization 和 token 不得记录。只建议在 HTTPS 反向代理后使用 Basic。 |
| 用户根 | 上游按用户名 namespace 取根，但以字符串拼接路径。 | 管理员保留历史 `data/webdav/`，普通用户使用 `data/webdav/users/<safe-name>/`。 | `acceptable security/data adaptation`：必须保留当前根，不移动或合并旧卷，不允许 Basic 切换到另一用户目录。 |
| `PROPFIND` | 文件或目录返回 `207` DAV multistatus；目录包含自身和一级子项。 | 无该方法；网页端用 `GET` 目录得到 OpenReader 私有 XML。 | `must-fix`：新增标准 namespaced `PROPFIND`，支持常用 `Depth: 0/1`，无效/`infinity` 安全夹紧到 1。保留现有目录 `GET` 形状给网页端，不强迫当前前端迁移。 |
| 只读不得写盘 | 不存在路径返回 `404`。 | `webdavList()` 对请求目录调用 `MkdirAll`，读取一个不存在的嵌套路径会创建目录并返回 `207`。 | `must-fix`：只有当前用户 WebDAV 根可在授权后的显式入口初始化；读取不存在的子路径必须 `404` 且零文件副作用。 |
| `GET` | 只下载普通文件；缺失 `404`，目录 `405`。 | 文件可下载；目录被复用为网页端列表。 | `deployed compatibility adapter`：外部协议以 `PROPFIND` 列目录；现有 `/webdav` 目录 `GET` 继续返回当前列表，`/reader3/webdav` 的目录 `GET` 可保持上游 `405`。 |
| `MKCOL` | 创建目录，已存在仍返回 `201`。 | 只创建最后一级，父目录缺失时先创建父目录；已存在返回 `409`。 | `must-fix visible status`：兼容别名恢复上游幂等 `201`；当前路径也采用同一安全、可预测语义，根目录仍禁止作为创建目标。 |
| `PUT` | 父目录缺失 `409`，目录目标 `405`，成功 `201`；覆盖已有文件。 | 会自动创建父目录；已有原子 staging、128 MiB 默认上限。 | `must-fix + security difference`：父目录必须先存在；保留有界、同目录 staging 和原子替换，不复制上游无界整包读取。 |
| `DELETE` | 缺失 `404`，成功 `200`。 | `os.RemoveAll` 对缺失路径也成功，并返回 `204`。 | `must-fix`：缺失必须可见；成功状态按路由兼容，`/reader3/webdav` 返回上游 `200`，已部署 `/webdav` 可保留 `204`。根不可删除。 |
| `MOVE`/`COPY` | 源缺失 `412`；缺少 Destination `400`；目标存在且无 Overwrite `412`；成功 `201`。COPY 递归复制。 | 只有 MOVE；未实现 Overwrite，目标父目录被自动创建。 | `must-fix`：两方法共享源/目标安全解析；目标必须位于同一调用者根，父目录必须存在，拒绝复制/移动到自身后代、symlink 和跨根；保留上游状态语义。 |
| `LOCK`/`UNLOCK` | LOCK 返回临时 `urn:uuid:` token、XML 和 `200`；UNLOCK 缺 token `400`，有 token `204`。上游不保存或强制锁。 | 未实现。 | `must-fix compatibility shim`：复刻无持久锁协议响应，不新增锁表，不虚假阻断现有文件写入。token 使用加密随机值且不写日志。 |
| 路径与 symlink | 上游路径字符串拼接，Destination 只取 URL path。 | 绝对化和根前缀检查能拒绝 `..`，但文件读写会跟随根内 symlink。 | `must-fix security adaptation`：每个已存在路径组件都用 `Lstat` 拒绝 symlink；源、目标父目录、递归 COPY 项和最终读取都必须验证。客户端错误不返回宿主路径。 |

## 固定路由、认证和响应合同

### 支持的路径

- 保留：`/webdav/` 与 `/webdav/*path`；
- 新增上游兼容：`/reader3/webdav/` 与 `/reader3/webdav/*path`；
- 两组都接受 Bearer JWT 和 HTTP Basic。网页端继续使用 Bearer，不把 JWT 或密码放进 query。

### 发现头

成功或协议错误响应至少包含：

```text
DAV: 1,2
Allow: OPTIONS, DELETE, GET, PUT, PROPFIND, MKCOL, MOVE, COPY, LOCK, UNLOCK
MS-Author-Via: DAV
```

`401` 额外包含 `WWW-Authenticate: Basic realm="OpenReader WebDAV"`。无 Authorization 的
`OPTIONS` 返回 `200` 和上述发现头；携带无法验证的 Authorization 时返回 `401`。

### 认证顺序

1. Authorization 为 `Bearer` 时沿用现有 JWT 解析；
2. Authorization 为 `Basic` 时严格解码一次 `username:password`，以规范化用户名查用户并使用
   `bcrypt.CompareHashAndPassword`；
3. 两者都只把持久化 user id 写入 Gin context，再进入现有活动记录和 `requireWebDAVAccess`；
4. 认证失败前不得解析 path、Destination、Depth、body，也不得触碰文件系统；
5. 不支持同时提供两种凭据或凭据降级重试，不在响应中区分用户不存在和密码错误。

### `PROPFIND`

- `Depth: 0` 只返回目标；省略或 `Depth: 1` 返回目标和一级子项；`infinity` 安全按 1 处理；
- XML 使用 `DAV:` namespace，每个 response 至少包含 href、displayname、resourcetype、
  getlastmodified；文件另含 getcontentlength/getcontenttype；
- href 基于当前请求前缀生成并对每个路径段 URL 编码，不暴露物理根；
- 目录尾部使用 `/`，输出按名称稳定排序；读取错误不返回文件路径。

### 文件变更

- 继续使用当前用户根和现有上传字节上限；PUT/COPY/MOVE 不能写过 symlink；
- MOVE/COPY 的 Destination 可为同一路由前缀下的绝对 URL 或绝对/相对路径，但规范化后必须属于
  当前调用者的同一 WebDAV 根；`/webdav` 与 `/reader3/webdav` 前缀可互相指向同一用户数据；
- Overwrite 只有 `T`（大小写不敏感）允许替换，`F` 或省略在目标存在时返回 `412`；
- 替换先验证完整计划，再在目标根内执行；失败不得删除原源或预先清空旧目标；
- COPY/MOVE 不能以 WebDAV 根为源，不能把目录复制/移动到自身或后代。

## 数据与迁移边界

- 不新增 SQLite 表/列/index，不改变用户密码 hash、JWT、备份成员或逻辑恢复格式；
- 不移动 `data/webdav/` 历史管理员文件，也不迁移 `data/webdav/users/<name>/` 私有目录；
- LOCK token 只存在于单次响应，不写数据库、文件、日志或浏览器存储；
- Basic 是新的认证入口，不是新凭据。改密后旧密码立即失效；删除/禁用用户后 Basic 访问立即失败；
- COPY/MOVE/MKCOL/PUT 只改变调用者明确指定的 WebDAV 文件。升级和启动不得扫描、重命名、删除
  或自动复制历史目录；
- Docker 历史卷必须证明管理员旧根和普通用户私有根在升级前后保持原位且互不可见。

## 必须先写的失败测试

1. 认证/发现：无凭据 OPTIONS `200`；无凭据 PROPFIND `401` + challenge；正确/错误 Basic、
   正确 Bearer、WebDAV-disabled、改密后旧 Basic、删除用户分别得到稳定结果，且认证失败零文件访问。
2. 别名：同一用户经 `/webdav` 与 `/reader3/webdav` 看到同一根；管理员历史根和普通用户私有根
   不交叉；Destination 可以在两个前缀间规范化但不能跨用户。
3. PROPFIND：Depth 0/1、自身+一级子项、目录斜杠、URL 编码、DAV namespace、稳定排序、文件
   length/type/date；不存在路径 `404` 且不创建目录。
4. MKCOL/PUT/GET/DELETE：父目录、已存在目标、目录目标、覆盖、上传上限、原子替换、缺失状态、
   根保护与当前 `/webdav` 网页端 GET listing 兼容。
5. MOVE/COPY：文件和递归目录、缺 source/destination、Overwrite T/F/省略、父目录缺失、目标已存在、
   自身/后代、失败不破坏源/旧目标。
6. 路径安全：`..`、百分号编码 traversal、绝对路径、Windows volume、NUL、Destination 逃逸、源/父/
   子项 symlink 全部在文件操作前失败；错误不含宿主根。
7. LOCK/UNLOCK：token 格式、DAV XML、默认/请求 Timeout、缺失 Lock-Token、零持久化和其它用户隔离。
8. 回归：现有 WebDAVBrowser Bearer + GET 目录、上传、下载、删除、导入/恢复不变；Go 全量、前端
   全量/build、三视口 WebDAV 工作台 smoke 通过。
9. Docker：历史管理员根、两个普通用户私有根、备份 ZIP 和导入文件在升级/重启后保持；运行
   volume/portable-backup smoke，并用真实 `curl --user` 执行 OPTIONS、PROPFIND、PUT、COPY、GET、DELETE。

## 实施顺序

1. 先添加上述路由/auth/XML/文件事务的 Go 失败合同，不改生产实现；
2. 建立独立 `backend/services/webdavfs`，集中根、symlink、PROPFIND、COPY/MOVE 和原子写语义；Gin
   handler 只做认证后的参数/状态映射；
3. 增加 Bearer-or-Basic 中间件、两个路由前缀和 DAV 响应头；保留网页端 GET adapter；
4. 运行 targeted Go、Go 全量、前端全量/build、真实浏览器 WebDAV 工作台与真实 Basic curl；
5. 提交并推送形成可验收切片；若历史卷/备份门禁通过，本地构建 amd64/arm64 并发布 Docker。
