# 备份上传恢复 multipart 请求边界固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
当前审查基线：`OpenReader@7045827`
审查日期：2026-08-24
状态：**inventory-complete / implementation-pending**

## 1. 范围

本轮只覆盖 `POST /api/backup/restore-legado` 的 multipart wire identity 和解析临时文件生命周期：

- 认证、WebDAV 权限与错误优先级；
- declared/actual-read 总包络；
- 唯一 `file` part、额外 scalar/file part、文件名与 ZIP 后缀；
- `multipart.Form.RemoveAll()` 的 handler ownership；
- shape 拒绝发生在 stage、ZIP preflight、SQLite transaction、文件恢复和事件之前。

已经发布的 reader-dev/Legado/OpenReader logical restore、portable v1/v2、appearance assets、ZIP 路径与
解压预算、caller/source ownership、事务/回滚、错误脱敏和 WebDAV opened-snapshot 合同不重开。

## 2. 固定上游与 OpenReader 适配

固定上游 `WebdavController.restoreFromWebdav` 和 `web/src/components/WebDAV.vue` 让用户从当前 namespace
显式选择一个 `.zip` 行并确认恢复。它没有 OpenReader 的 JWT multipart restore route，也没有多文件
合并语义。`uploadFileToWebdav` 虽可上传多文件，但那是独立文件管理动作，不是一次恢复多个包。

OpenReader 保留部署中的 `/api/backup/restore-legado` 和 `frontend/src/api/backup.js#restoreLegadoBackup`，
把客户端选中的一个 ZIP 作为 `FormData.file` 直接恢复；当前规范可见流程仍从唯一 WebDAV 管理器走
`restore-webdav`。因此“恰好一个 `file`，零 scalar/额外 file”是固定上游单选恢复的安全适配，不删减
任何已签收可见流程或备份格式。

## 3. 当前差异与真实反例

| 边界 | `OpenReader@7045827` | 裁决 |
|---|---|---|
| auth/permission | JWT middleware 和 `requireWebDAVAccess` 均在 multipart 解析前。 | **aligned / must-preserve**。无凭据 401、无权限 403 必须优先于 body shape/size。 |
| 总包络 | declared `Content-Length` 与 `http.MaxBytesReader` 均限制为 `maxCompressed + 1 MiB`；stage 和 ZIP reader 另有 compressed/expanded budgets。 | **aligned / regression-only**。不得放宽、重复分配或把恰好边界改坏。 |
| multipart identity | `c.FormFile("file")` 只选择第一个同名 part；任意 scalar、第二个 `file` 和其它 file field 均被完整解析后忽略。 | **must-fix**。歧义请求不得决定由哪一个包产生持久副作用。 |
| 临时文件 ownership | handler 不保留 `*multipart.Form`，也不调用 `RemoveAll()`；真实 `net/http` 外层可能最终清理，但直接 handler、测试或未来 adapter 不能证明每条返回路径都释放。 | **must-fix**。解析成功或部分成功后由本 handler 立即 `defer` 清理。 |
| 文件名/内容 | 只用 client filename 的大小写无关 `.zip` 后缀；真正内容仍进入随机 caller-private stage 和完整 ZIP preflight。 | **preserve with bounded admission**。文件名必须为非空 UTF-8、至多 255 bytes、后缀 `.zip`；不得由文件名派生 stage path。 |

一次性 Go overlay 探针在 `7045827` 上把一个合法 `file=backup.zip`、额外 scalar 和 34 MiB
`other=ignored.bin` 同时提交。handler 返回 200、真实写入书架 Book，且直接 `router.ServeHTTP` 返回后
`TMPDIR/multipart-*` 仍存在。探针随后显式清理，仓库没有修改。该证据证明 shape 和 handler-owned
cleanup 不能从总包络或 ZIP transaction 绿测间接推导。

## 4. 目标 API 合同

1. 仍先执行 JWT、activity 和 `requireWebDAVAccess`。无效身份/权限不解析 multipart，不创建 temp、
   stage、row 或 event。
2. 请求总包络仍为 `portableLimits.maxCompressed + 1 MiB`，并同时约束 declared 和 chunked body。
   超限保持 `413 {"error":"backup file exceeds size limit"}`。
3. parser 必须完整取得 `*multipart.Form` 后验证：`form.Value` 为空；所有 file field 合计恰好一项；
   唯一 field 名为 `file` 且只含一个 header。缺失/非 multipart 保持
   `400 {"error":"backup file is required"}`；额外/重复/歧义 part 返回
   `400 {"error":"invalid backup upload"}`。
4. 只要 request 或 Gin 已持有非 nil `MultipartForm`，handler 就在任何 filename、size、stage、ZIP、
   restore 或 response 分支前注册 `RemoveAll()`；cleanup 失败不覆盖更早的稳定 API 结果，也不记录
   host temp path、field value 或 filename。
5. client filename trim 后必须是 1..255 UTF-8 bytes 且大小写无关 `.zip`。非法/空/过长名称返回现有
   `400 {"error":"backup file must be a zip archive"}`；随机 caller-private stage path 继续独立于名称。
6. 唯一文件的 `FileHeader.Size` 和实际 opened bytes 继续分别受 compressed limit；ZIP entry/path/
   duplicate/symlink/count/per-entry/expanded 预检、portable manifest/collision/hash 和 restore transaction
   完全沿用现合同。
7. shape/cleanup 收紧不增加响应字段、前端入口、WebSocket event、日志详情、备份 member 或状态迁移。

## 5. 测试先行门

1. 在旧实现上分别锁定第二个同名 `file`、其它 file field、scalar part 仍返回 200 并恢复 Book；目标均
   返回安全 400，且 Book/setting/source/archive/asset/stage/event 不变。
2. 将 `router.MaxMultipartMemory=1`，覆盖 valid success、invalid shape、wrong extension、oversized header、
   stage/restore failure；直接 handler 返回时 temp root 必须为空。旧实现至少在成功路径留下 temp。
3. non-multipart/missing file、declared/chunked exactly limit/+1、单合法 ZIP、大小写 `.ZIP`、logical、
   portable v1/v2、appearance assets、source permission 和 transaction rollback 保持。
4. focused API、focused race、`go vet ./...`、Go 全量、frontend 全量/build、真实 HTTP declared/chunked/
   duplicate/scalar/temp probe、WebDAV 恢复三视口回归，以及 fresh/historical/portable/restart 卷门通过后
   才可发布 Docker。

## 6. 数据与允许差异

- 不新增 schema、migration、startup scan、目录、环境变量、route、payload、备份 member、manifest、
  ZIP 预算、浏览器 key 或可见 UI。
- 已存在的 logical/portable ZIP 和 mounted `data/cache/library` 不扫描、不移动、不重写；本切片只拒绝
  未来歧义 HTTP 请求并缩短请求临时文件生命周期。
- 比固定上游更严格的总包络、ZIP preflight、JWT/权限、caller-private stage、multipart singularity 和
  handler-owned cleanup 是 OpenReader 多用户/Go runtime 安全适配。

## 7. 实施顺序

1. 独立提交本合同及 API/REST/迁移/安全矩阵，不改应用或测试。
2. 在 `7045827` 旧实现上提交 shape/cleanup 红测。
3. 提取窄作用域 multipart parser，handler 只做权限、parse/defer cleanup、现有 restore dispatch。
4. 完成专项/race/full/runtime/卷门；形成可验证切片后提交推送并只在本机发布 amd64/arm64。
