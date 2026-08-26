# 默认书源快照文件系统与提交生命周期固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前审查基线：`OpenReader@b504a24`

审查日期：2026-08-26

状态：**inventory-complete / tests-and-implementation-pending**

## 1. 范围与结论

本轮只复审默认书源快照的持久化边界及以下既有动作，不重开已签收的 BookSource ownership/COW、
管理器、导入、搜索、探索、Reader、备份或 WebDAV 主合同：

- `GET /api/sources/default`；
- `POST /api/sources/default/save`；
- `POST /api/sources/default/restore`；
- `POST /api/admin/users/:id/sources/default`；
- `POST /api/admin/users/sources/reset`；
- 从旧 OpenReader 卷初始化 `data/defaultBookSources.json` 与 SQLite default namespace。

固定上游把目标用户现有 `bookSource.json` 完整复制到 `default/bookSource.json`，显式空数组也是已配置
默认；未初始化用户首次读取时复制当时默认文件。OpenReader 以 SQLite namespace/COW 表达同一产品
状态是允许的技术适配，但兼容 JSON、SQLite 和响应不能互相矛盾，也不能把挂载卷对象、宿主路径或
并发时序暴露为产品语义。

当前实现存在四组 must-fix：

1. `loadDefaultBookSources()` 对固定文件直接 `os.ReadFile`，没有 16 MiB actual-read 上限、regular-file/
   same-file 验证或 symlink/special-file 拒绝；合法 JSON 可在解析项数前放大内存。
2. `defaultSourcesStatus()` 把底层 `err.Error()` 放进 `200` JSON，可暴露绝对 data 路径、SQLite 文本或
   文件系统诊断；restore/reset 的错误分类又与 status 不一致。
3. 两个 save 动作先写 JSON、后事务写 SQLite，只做无锁 best-effort 文件补偿。两个并发 save 可形成
   “文件来自 B、SQLite 来自 A”的交叉提交；请求取消和数据库失败也没有统一权威边界。
4. ownership-v1 迁移先把旧全局书源设为 default namespace；路由层随后看到 SQLite 已配置便跳过
   `defaultBookSources.json`。因此直接从旧卷升级时，历史自定义默认快照没有被证明可保留。

裁决：**must-fix**。实现顺序必须保持“本合同 -> 旧实现红测 -> 实现”。

## 2. 固定上游与现有允许差异

上游证据：

- `BookSourceController#getUserBookSourceJson`：用户文件不存在时复制 default 文件；存在空数组时不回退；
- `BookSourceController#setAsDefaultBookSources`：管理员读取目标用户已存在文件并完整写入 default；
- `BookSourceController#deleteUserBookSource/deleteBookSourcesFile`：删除私有文件后才在下次读取继承默认；
- `UserManage.vue#setAsDefaultBookSources`：目标用户确认、成功/失败可见状态；
- `YueduApi.kt` 的对应 `/reader3/*` 路由。

继续允许的 OpenReader 差异：

- SQLite `user_book_sources + book_source_namespaces` 是运行时权威，JSON 是旧卷输入和兼容镜像；
- 数值 user ID 路由、JWT 管理员权限、COW/detached 和多用户事务是 Go/SQLite 安全适配；
- 文件系统 hardening、大小限制、安全错误和请求取消是明确安全增强。

不允许的差异：已成功动作只更新其中一种持久化、并发最后写入者在 JSON/SQLite 中不同、旧默认文件
被静默替换成全部全局源、或错误响应暴露挂载路径。

## 3. API 合同

路径、认证、成功状态和现有成功 body 保持：

| 动作 | 成功 | 失败与副作用 |
|---|---|---|
| `GET /api/sources/default` | `200 {configured,count}`；读取本身不初始化普通用户。 | 默认状态/兼容镜像校验失败为安全 `500 {error}`，不得返回 raw OS/SQLite/path；未配置仍为 `200 {configured:false,count:0}`。 |
| `POST /api/sources/default/save` | 管理员把自己的已初始化活动源设为默认，`200 {count}`，count 可为 0。 | 权限优先级不变；取消或权威提交失败不得改变 default namespace 或成功响应。 |
| `POST /api/admin/users/:id/sources/default` | 目标用户已初始化列表成为默认，`200 {count}`。 | 目标 `404`、未初始化 `409` 保持；失败不得回退调用者或留下 JSON/SQLite 半提交。 |
| `POST /api/sources/default/restore` | 当前用户按 SQLite 权威默认 reconcile，并发送现有当前用户事件。 | 未配置 `404`；安全初始化失败 `500`；事务失败不改用户 namespace、不广播。 |
| `POST /api/admin/users/sources/reset` | 去重目标在一个事务中按同一默认 generation reconcile，成功 body/event 不变。 | body/cardinality/目标预检保持；默认初始化或任一目标失败整批不变且无事件。 |

所有 GORM 查询/事务和可能持续的文件复制必须使用 request/startup context。认证、管理员和目标用户
预检仍早于持久化工作；取消不转换为包含内部原因的 JSON。

## 4. 数据、文件与升级合同

### 4.1 权威关系与兼容镜像

- default namespace 一旦配置，SQLite 是运行时权威；普通用户初始化、restore/reset、搜索和 Reader 不得
  在每次请求重新解释 JSON。
- `data/defaultBookSources.json` 保留为 reader-dev/旧 OpenReader 兼容镜像，使用现有 reader-dev encoder；
  不写数值 source ID、user ID、credential、failure/session/cache 状态。
- 默认 save 必须按一个 server-level serialization boundary 排队。完成响应前 SQLite 与镜像应对应同一
  已提交快照；进程崩溃窗口由下次启动从 SQLite 权威状态重建镜像，不能反向覆盖已配置 SQLite。
- 数据库提交失败时旧 SQLite 与旧镜像保持。镜像最终 rename 失败不能伪造数据库 rollback；应保留可
  重试状态并使用安全诊断，下一启动在服务请求前完成修复。

### 4.2 固定文件读取边界

- 唯一允许路径是配置 `DataDir` 下直接子项 `defaultBookSources.json`。
- legacy read 只接受该根下 non-symlink regular file；使用同一 `Lstat -> open -> fstat/SameFile` handle
  读取，拒绝 entry symlink、目录、FIFO/device/socket 和检查后替换。
- actual-read 上限为 16 MiB，与书源导入 payload 上限一致；最多 300 个原始 source object，必须是单个
  UTF-8 JSON array/wrapper/single-source 兼容形状，继续保留未知 reader-dev 字段往返。
- missing 是“没有 legacy default”，不是错误；unsafe、oversized、malformed 不得导入、扫描外部对象、
  创建 namespace 或泄漏路径。

### 4.3 历史卷迁移

- ownership-v1 尚未应用的旧卷：先安全读取 legacy 文件。文件有效时，现有用户仍关联全部旧活动源，
  但 default namespace 按文件 URL identity/完整规则快照建立；文件不存在时才沿用旧全局活动列表。
- 已应用 ownership-v1 的卷：SQLite default namespace 保持权威，升级不得用可能陈旧的 JSON 盲目覆盖；
  启动只把权威 namespace 重写成 canonical mirror。
- 显式 `[]` 必须迁移为 configured-empty；不存在文件与空数组不能合并。
- 迁移/修复 marker 幂等，重启不重复 source/COW 行，不改用户私有 namespace、Book.SourceID、
  SourceFailure、缓存、书架或 backup 内容。

## 5. 并发、取消和恢复边界

- 兼容 save 与目标用户 save 共用一把 server-level mutex；restore/reset 在取得权威 default snapshot 时也
  进入同一边界，不能读到 save 的中间 generation。
- snapshot 列表、default transaction、mirror stage/sync/rename 和启动修复有明确顺序；临时文件必须位于
  DataDir、权限私有、异常/取消后清理。
- 并发 A/B save 的最终 SQLite 和 canonical JSON 必须都对应最后完成的同一请求；不得混合两者字段。
- injected DB failure、rename failure、request cancellation 和 restart recovery 都必须有确定性测试；任何
  失败不得广播 `sources_update/users_update` 或初始化无关用户。
- 日志只允许稳定动作名和错误类别，不包含绝对路径、JSON 内容、书源 URL/header/cookie、用户名、JWT、
  SQLite 文本或临时文件名。

## 6. 测试先行闸门

实现前必须提交在 `b504a24` 失败的合同测试：

1. 16 MiB+1、chunked overflow、symlink、目录和 FIFO legacy file 均 fail closed；文件外内容不读取。
2. invalid/unsafe legacy status/restore 返回稳定 path-free 错误，不能把 `err.Error()` 放入 200。
3. 两个来源不同的并发 save 最终 SQLite 与 JSON 是同一快照；race 模式通过。
4. SQLite trigger 注入失败后 default namespace、镜像和事件均保持旧值。
5. 请求已取消时不开始或不提交 save/restore/reset，context 进入 GORM。
6. pre-ownership 历史卷的自定义 default 文件优先于旧全局 default seed；当前用户活动关联与 source ID
   不被重写；显式空文件迁移为 configured-empty。
7. ownership-v1 已应用且 JSON 陈旧时 SQLite 保持权威，启动 canonicalize 镜像而不改普通用户。
8. ordinary/portable/Legado backup 继续导出当前用户活动源，不新增 default mirror、migration journal 或
   主机路径；fresh/historical/restart mounted-volume 门通过。

## 7. 实施与发布闸门

实现后至少运行：

```bash
cd backend && go test ./...
cd backend && go test -race ./api ./services/booksources ./db
cd backend && go vet ./...
cd frontend && npm test
cd frontend && npm run build
docker compose config --quiet
```

另需真实 HTTP 验证 admin/current-user save、status、restore/reset、path-free failure 和并发最终状态；
该切片不改变可见 UI，浏览器只回归 UserManage 设默认/批量重置与 SourceManager 恢复默认的桌面、
390x844、360x800 现有流程。发布前通过 fresh/historical/portable/restart 卷，再由受信 Actions 发布并
回拉不可变标签确认 health revision、旧卷 default 及 JSON/SQLite 一致性。

## 8. 非目标

- 不改变书源字段、URL identity、COW/detached、sourceLimit、导入/导出、失败缓存、调试或 Reader 解析。
- 不新增默认书源编辑 UI、公开文件下载、环境变量、备份条目或 WebDAV 文件。
- 不扫描或重写用户私有 `data/cache/library`，不把 default mirror 当作普通用户 backup。
- 不处理固定上游自身无 size/symlink/transaction hardening 的缺陷；这些是 OpenReader 明确安全增强。
