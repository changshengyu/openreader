# 备份上传恢复 multipart 请求边界固定基准第二轮合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
当前实现基线：`OpenReader@a0fb1bd`
审查日期：2026-08-25
状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

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

## 8. 实施与发布证据

- 合同 `7a2a44a`、旧实现红测 `20ac551` 和实现 `a0fb1bd` 依次提交并推送 `main`。实现新增窄作用域
  multipart parser，严格校验唯一 `file`、零 scalar/额外 file、255-byte UTF-8 ZIP filename，并在
  parsed form 存在时立即注册 `RemoveAll()`；现有 auth/permission、restore dispatch 和 transaction 未改。
- 旧实现红测逐项证明 scalar、重复同名 file、其它 file field 和 256-byte filename 会恢复 Book，且
  success、invalid shape/extension、oversized file、invalid ZIP 的 forced-disk temp 在直接 handler 返回后
  存留；实现后全部转绿。focused restore、focused race、`go vet ./...` 和 Go 全量通过。
- 前端 741/741 与 production build 通过。真实 Go HTTP 探针证明 ambiguous parts=400、单一大写 `.ZIP`=200
  并恢复 1 本书、declared/chunked overflow=413，服务运行期间 multipart temp files=0。WebDAV 恢复会话
  隔离在 1440x900、390x844、360x800 均通过，无 stale toast/event/reload。
- 本地 `a0fb1bd` candidate 的 fresh portable-v1/v2-assets/cross-user/restart 卷门通过。historical 首次并行
  运行在 fixture 后遇到瞬时 404；同镜像独立 trace 重跑完整通过 TXT/EPUB/UMD/CBZ、relative-cache、
  owner isolation、logical/portable restore、archive hash 和 restart，未形成可复现产品回归。
- 本机完成 amd64/arm64 构建后发布 `ghcr.io/changshengyu/openreader:a0fb1bd` 和 `latest`；两标签共同
  指向 OCI index `sha256:b25f5b05df983532bf656ec8647e553188db3ba7fb291b826cb45b65deae6f3c`。
  amd64 manifest 为 `sha256:4bd4e8e85e3213247191a1a2c7df37efade1a62a38d39851dab897766cc38224`，arm64
  manifest 为 `sha256:a470eaf82724c71d0d8100dc60676b54d16dc5e122ba33cfd57090495a5b44e3`。
  用户生产环境当前运行提交仍未知，故保留 `awaiting-device-verification`。
