# WebDAV DELETE 文件系统生命周期第二轮固定基准合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只提取合同并记录当前反例，不修改应用或测试代码。范围仅是原生 WebDAV
`DELETE /reader3/webdav/*path` 和已部署兼容路由 `DELETE /webdav/*path` 从路径验证到物理删除的
文件系统生命周期。WebDAV 认证、caller scope、其它 DAV 方法、网页工作台和备份恢复已有专项合同，
本轮不得借此重开。

## 1. 固定上游与当前映射

固定上游权威入口：

- `src/main/java/com/htmake/reader/api/controller/WebdavController.kt#webdavDelete`；
- `src/main/java/com/htmake/reader/utils/Ext.kt#File.deleteRecursively`。

上游在当前用户 WebDAV home 下解析路径，缺失返回 `404`，文件或目录递归删除后返回 `200`。
OpenReader 保留该可见语义，并以 `/webdav` 成功 `204` 兼容已部署网页端；多用户私有根、Bearer/Basic
认证和 symlink 防护是允许的安全适配。

## 2. 当前反例与差异矩阵

当前 `webdavfs.Service.Remove` 先经 `Resolve` 和 `Lstat` 验证目标，再把绝对路径交给
`os.RemoveAll`。验证与删除不共享已打开父目录和目标 identity：若父目录在验证后被移走，并在原名
放入指向调用者根外的 symlink，`RemoveAll` 会沿新的父路径删除外部同名目标。静态 symlink 测试无法
覆盖该校验到使用竞态。

| 合同点 | 当前 OpenReader | 裁决 |
|---|---|---|
| 路由与状态 | 上游别名成功 `200`；兼容路由成功 `204`；缺失 `404`；不安全路径 `403`。 | **aligned / preserve**：修复不得改变 wire contract。 |
| caller scope | 每次请求使用管理员历史根或普通用户私有根。 | **aligned / preserve**：不得迁移、合并或跨用户解析。 |
| 验证到删除 | `Resolve`/`Lstat` 后按绝对路径 `RemoveAll`，祖先可在窗口内被替换。 | **P2 must-fix**：从受信 root handle 逐组件验证，在同一打开父目录内按 identity detach。 |
| 目标类型 | 普通文件和目录可删；symlink/特殊文件拒绝。 | **aligned intent / strengthen lifecycle**：文件和目录都必须保持，且校验后替换 fail closed。 |
| 目录递归 | `RemoveAll` 可递归移除目录。 | **must-fix security adaptation**：递归遍历只能相对已打开目录句柄；内部 symlink 只移除链接本身，不得触碰目标。 |
| 缺失与根 | 缺失映射 `ErrNotFound`，空路径拒绝删除 caller root。 | **aligned / preserve**。 |

## 3. 目标 API、数据与错误合同

- `DELETE /reader3/webdav/*path` 成功仍为 `200`，`DELETE /webdav/*path` 成功仍为 `204`；两者缺失
  `404`，根、symlink、特殊文件和不安全生命周期返回 `403`。响应不得暴露宿主路径。
- 删除对象只能是当前 caller root 中已验证的 regular file 或 directory。root 自身永不可删；从 root
  到目标父目录的每个现存组件必须是同一当前打开的真实目录。
- 在已打开父目录内打开目标并记录 identity。最终删除前必须再次以该父目录句柄核对当前名称仍指向
  同一实体，然后原子重命名到随机、不可覆盖的请求私有 quarantine 名。
- quarantine 实体须再次以 `O_NOFOLLOW` 打开并验证 identity。任何祖先/目标替换、symlink、特殊文件、
  不支持的 rooted 操作或 identity 变化都 fail closed，不得回退到绝对路径 `RemoveAll`。
- regular file 通过父目录句柄 unlink；directory 只相对已打开 quarantine directory 递归处理，再从
  同一父目录句柄移除。内部 symlink 可作为链接项本身删除，但其目标永不打开或递归。
- 验证后名称被替换时，替换实体和原已验证实体都必须保留；根外 sentinel 必须保持。若已完成 detach
  后续物理清理失败，可返回服务错误，不得改删活动名称下的新实体。
- 不新增 SQLite schema、配置、URL、目录或备份成员；不扫描、重命名或迁移现有
  `data/webdav` 内容。普通文件、空/非空目录的正常递归删除行为保持。

## 4. 测试先行闸门

实现前必须先让以下合同在当前实现上失败：

1. 目录父级在验证后被替换为根外 symlink 时返回 `ErrUnsafePath`，根外同名目录与 sentinel 保持，
   原目录和替换链接也保持。
2. regular file 父级在验证后被替换时同样 fail closed，外部文件、原文件和替换链接保持。
3. 目标自身在验证后被 symlink、不同文件或不同目录替换时，不删除替换实体，也不误删原实体。
4. 正常 regular file、空目录、嵌套目录继续删除；目录内 symlink 只删除链接，外部目标保持。
5. 静态祖先/目标 symlink、FIFO/其它特殊文件、空路径和缺失路径继续分别映射既有服务错误。
6. API 回归锁定两路成功状态、`404`/`403`、认证先于文件访问和 caller 私有根隔离。

实现后运行 `webdavfs`/`rootedfs` focused tests 与 race、WebDAV API tests、Go 全量/vet、frontend 全量/
build 和 Compose。该切片没有前端几何变化；真实协议 smoke 应至少覆盖 Basic 登录后的文件/递归目录
DELETE 和缺失状态。发布候选仍须通过 trusted Actions 的 fresh/portable、historical volume 和双架构门。

## 5. 实施边界

优先扩展现有 `backend/services/rootedfs`，让 regular file 和 directory 共用 Go 1.24 `os.Root` 与
`openat/renameat/unlinkat` 生命周期，同时保留 `RemoveDirectory` 的缺失幂等合同。WebDAV adapter 负责把
rooted missing/unsafe 精确映射为 `ErrNotFound`/`ErrUnsafePath`。不得使用 `EvalSymlinks` 结果作为新信任
根，也不得以删除前再次拼接绝对路径代替同句柄 identity 验证。

## 6. 实施与发布记录（2026-09-15）

- 合同 `7d364ff`、旧实现红测 `2c96481`、实现 `daa435d` 按顺序落地。红测确定性证明父目录在验证后
  被 symlink 替换时，旧 `os.RemoveAll` 会返回成功并删除根外同名 regular file 或 directory。
- `rootedfs.RemovePath` 现在从受信 boundary 打开 `os.Root`，逐组件验证 root/parent/target identity，
  在同一 parent fd 内把当前 regular file 或 directory detach 到随机 quarantine，再以
  `openat(O_NOFOLLOW)`、`renameat` 和 `unlinkat` 完成句柄相对删除。管理员工作区的缺失幂等
  `RemoveDirectory` 继续复用同一实现。
- root、parent 或 target 在验证后被替换均返回安全错误；替换实体、原实体和根外 sentinel 保持。
  普通文件、非空目录、内部 symlink、缺失路径及两路 WebDAV 状态均有 service/API 回归覆盖。
- focused/race、Go 全量/vet、Linux amd64/arm64 编译、frontend `757/757`、Vite build、Compose 和最终
  Basic/curl WebDAV 协议 smoke 通过；没有 UI 变化，因此不重复浏览器几何截图。
- 可信 Actions run `34960341835` 通过 backend/frontend/build/Compose、native、fresh/portable、
  historical volume 和 published-platform 门，发布 `ghcr.io/changshengyu/openreader:daa435d` 与
  `latest`。amd64/arm64 OCI index 为
  `sha256:f563313d1d38358ba354189a62fd47beda2ba4f83d2c1e9f15ddd8ae131cd417`；amd64 manifest 为
  `sha256:ecb4ed2cb283233708a63ef194d0c8ca1ec4bdfc447cea9e43d1fe3b4ddcd998`，arm64 manifest 为
  `sha256:8a5d7dd1d3e6fb5d871a1cad4b72cf8d6e3559b11798412139d3f1346e4e4d4a`。
