# 备份生成请求边界第二轮固定基准合同

状态：**inventory-complete / implementation-pending**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只审计普通/portable 备份生成动作的错误脱敏、请求取消和临时文件提交边界。逻辑 ZIP 内容、
portable v2 manifest/assets、caller root、list/download/restore、SQLite/文件兼容及唯一工作台入口继续由
[`backup-restore-fixed-baseline-p2-contract.md`](backup-restore-fixed-baseline-p2-contract.md) 和
[`portable-local-archive-backup-p1e4-contract.md`](portable-local-archive-backup-p1e4-contract.md) 约束，
不因当前 action 长尾重开。

## 1. 权威源码

### reader-dev

- `src/main/java/com/htmake/reader/api/controller/WebdavController.kt:769-793`
- `web/src/views/Index.vue:2152-2176`

固定上游在认证/WebDAV 权限和确认后执行一次备份；失败只投影固定“备份失败”，不把文件路径或底层
异常放进响应。OpenReader 独立创建 caller-scoped ZIP、JWT/Bearer、logical/portable 双动作及明确输出预算
是已签收的服务端运行适配。

### OpenReader

- `backend/api/server.go:204-209`
- `backend/api/backup.go:15-79`
- `backend/services/backup/backup.go:82-180`
- `backend/services/backup/portable.go:82-228`
- `frontend/src/api/backup.js`

## 2. 稳定动作合同

| 路径 | 成功语义 | 已签收错误语义 | 本轮保持 |
|---|---|---|---|
| `POST /api/backup/trigger` | 无 body；在调用者普通/管理员兼容根原子创建 `backup_*.zip`，返回 `200 {message,path,name}`，其中 path/name 仅为 basename | 失败为客户端安全固定 500；不暴露挂载路径、SQL/ZIP/OS 细节，不出现可列出的半包 | JWT + effective WebDAV access、caller-only logical artifacts、同秒不覆盖、普通格式/别名不变 |
| `POST /api/backup/portable/trigger` | 无 body；在调用者根原子创建 `portable_backup_*.zip`，返回既有 v2 `format/localBooks/assets/legacyAssets` | 既有 typed `409/413` 与安全 500 不变 | manifest/hash/asset/archive budget、caller identity、portable v1/v2 restore 不变 |

请求附带 body 继续被忽略；本轮不增加 JSON shape，也不改变前端确认、状态码或成功响应。认证/权限和
caller-root 解析必须先于任何数据库快照、目录创建、文件打开或归档工作。

## 3. Inventory 证据与差异

1. `triggerBackup` 当前把 `"backup failed: " + err.Error()` 直接返回。`Service.run` 原样传播
   `MkdirAll/CreateTemp/Chmod/Sync/Close/Rename/Lstat` 和 GORM/ZIP 错误；这些错误可含绝对挂载路径、
   SQL 或内部归档细节，违反固定上游及已发布 API 的安全 500 合同。
2. `Service.run`、`RunPortableV2ForUser` 和所有 HTTP 调用都没有 context 参数。请求在等待全局 `mu`、
   数据库读取、JSON/ZIP 写入或大 archive/asset copy 时取消，工作仍继续到成功/失败；portable 可在客户端
   离开后继续消耗数百 MiB I/O。
3. 普通生成的 GORM transaction 未绑定 request context；portable 查询同样使用裸 `s.db`。临时文件虽由
   defer 清理失败路径，但请求取消不会产生错误，因此不能触发该清理边界。
4. 成功日志写完整 `backupPath`。这不改变 API，但普通运行日志不需要暴露 mounted host path；basename
   足够关联 list/download 结果。
5. 已发布原子 temp+rename、单 Service 串行、typed portable 错误、caller scope、archive budgets 和无半包
   测试仍成立。当前问题只属于 action lifecycle 与 diagnostics，不证明格式/恢复模块错误。

判定：**must-fix**。这是当前代码和固定上游/现有 OpenReader 合同的直接冲突，不是从旧日志重开模块。

## 4. 请求取消与 durable boundary

- 新增 context-aware service 入口供两个 HTTP handler 使用；既有无 context 方法保留为
  `context.Background()` 兼容 wrapper，scheduled backup 继续独立于任一 HTTP 请求。
- 已取消请求在等待生成锁时必须返回，不得随后获得锁并开始查询或创建 temp。进入工作后，在每个 DB/
  logical artifact、book/archive/asset copy、ZIP close、sync 和 rename 前后设置有界取消检查。
- 普通生成的稳定 DB transaction 使用同一 request context。portable 的 shelf/settings/assets 查询与文件
  copy 也使用该 context；复制不能只在整本大文件完成后才观察取消。
- final rename 前取消：关闭 ZIP/文件、回滚未提交 DB transaction、删除 temp，不创建 list 可见文件，
  不返回成功。final rename 已成功后：文件是 durable fact，不因随后断开删除；handler 可不再写响应。
- 取消不映射为带内部文本的 500，也不记录为备份格式失败；客户端已断开时允许 handler 直接返回。

## 5. 错误与日志边界

- 普通 trigger 的内部生成错误固定返回 `500 {"error":"backup failed"}`。Portable 保留现有
  `409/413` typed 文案；其它错误固定 `500 {"error":"portable backup failed"}`。
- 响应和普通日志不得包含 `DataDir/CacheDir/LibraryDir`、caller root、temp/final absolute path、SQLite
  路径、SQL、ZIP entry 内部错误、原书文件路径或凭据。成功日志最多记录安全 basename 和用户数字 ID。
- 内部错误仍可由测试/服务返回给调用者进行分类，但 HTTP 层不得序列化 `err.Error()`；不为实现方便
  把底层路径包装成新的客户端 code/message。

## 6. 数据与兼容边界

- 不改表、列、索引、migration、备份成员、manifest version、文件名 prefix、caller root 或旧 URL。
- 不扫描、重写或删除既有 `backup_*.zip` / `portable_backup_*.zip`。取消只清理本次未 rename 的私有 temp。
- logical/portable 读取的 SQLite 行、书源 ownership、历史空 namespace、ReplaceRule 顺序、进度/书签/
  分类、TXT/EPUB/UMD/CBZ archive hash 和 asset placeholder 语义保持。
- 跨用户全局串行是否改为按根串行不在本轮范围；先保证取消 waiter 不在取消后进入工作。任何并发策略
  调整都必须保留同一目标根不覆盖和全局输出预算。

## 7. 测试先行门

应用实现前必须先在旧实现上证明失败：

1. HTTP 普通 trigger 注入包含绝对路径/SQL sentinel 的 GORM 或文件错误，只得到固定安全 500；响应、
   list 和日志不出现 sentinel，目录没有 final/temp 包。
2. 普通与 portable context 在开始前已取消：零数据库查询、零 archive/asset open、零 temp/final file。
3. 一个生成持锁时取消第二个 waiter：第二个立即结束，释放锁后也不开始。取消首个 ordinary logical
   artifact 或 portable 大文件 copy：停止后续工作并清理 temp。
4. 取消与 final rename 竞态锁定 durable boundary：rename 前无可见包；rename 成功后的包保持可列出、
   下载、恢复，不被补偿删除。
5. 既有普通/portable success、typed 409/413、同秒不覆盖、logical aliases、v2 assets、跨用户和失败无
   半包测试继续通过。

实现后运行 focused/race、Go 全量/vet、frontend 全量/build、真实 HTTP 的 error/cancel/success 探针，
并通过 fresh/historical/portable mounted-volume 门后才发布 Docker。

## 8. Inventory 结论

direct Gin JSON binder 差集关闭后，按 route/work amplification 重新枚举得到的下一项确定 must-fix 是
备份生成 action lifecycle：一个已证明的安全 500 冲突，以及普通/portable 两条无 context 长 I/O 路径。
本 inventory 只新增/更新合同文档，没有修改应用或测试。
