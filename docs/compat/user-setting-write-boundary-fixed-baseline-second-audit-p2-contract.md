# 用户配置写入请求边界第二轮固定基准合同（P2）

状态：**implementation-complete / regression-validated / Docker-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

合同提取阶段只记录当前反例、不修改应用或测试代码；后续实现仍严格限定为：

- `PUT /api/settings/:key` 的认证、key、wire body、单 JSON、错误与写入前边界；
- 现有 `reader` / `shelf` / `search` 三键映射、CAS、显式 `force`、Reader 本地字段清理、响应和
  `settings_update` 副作用的保持；
- `GET /api/settings/:key` 与 backup/restore 只复核兼容边界，不重建已签收流程。

Reader 可见设置、Index 备份/同步配置、跨账号 operation generation、WebSocket recipient、portable
appearance assets 与 SQLite 并发 upsert 已有专项合同，本轮不得借 body boundary 重开。

## 1. 权威文件与当前映射

固定上游：

- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `/reader3/saveUserConfig`、`getUserConfig`；
- `UserController.kt#saveUserConfig/getUserConfig`；
- `web/src/views/Index.vue#saveUserConfig/restoreUserConfig`；
- `RestVerticle.kt` 的共享 `BodyHandler`。

OpenReader：

- `backend/api/server.go`、`settings.go`、`request_body.go`、`helpers.go`；
- `backend/models/models.go#UserSetting`、backup/portable restore；
- `frontend/src/stores/reader.js`、`preferences.js`、`layouts/AppLayout.vue`、`api/client.js`；
- `settings_concurrency_contract_test.go`、`backup_fixed_baseline_contract_test.go`、
  `bookshelf-network-first-sync-p2-contract.md`、`backup-restore-fixed-baseline-p2-contract.md`。

固定上游把当前终端 `config`、`shelfConfig`、`searchConfig`、`customConfigList` 组成一个 JSON object，
认证后写入调用者 namespace 的 `userConfig`，并增加 `@updateTime`。读取缺失文件显示“没有备份文件”。
OpenReader 将其技术栈等价地拆成当前用户的三个 `user_settings` 行，以 CAS 支持多终端后台同步，并用
显式 `force` 实现上游确认后的“备份当前终端配置”。该映射已签收，本轮不合并回单文件接口。

## 2. 当前反例与差异矩阵

2026-08-12 在隔离生产形态服务、临时 SQLite 和临时挂载根上得到以下反例：

- 声明 `Content-Length` 的 8 MiB + 1 `reader` 请求返回 `200`，写入 marker，并回显 8,388,671 bytes；
- 未知长度/chunked 的 8 MiB + 1 `shelf` 请求返回 `200`，同样写入并回显约 8 MiB；
- `search` 的两个连续 JSON 对象返回 `200`，只持久化首对象 `first-json`；
- 合法对象后的非空垃圾返回 `200`，只持久化垃圾前对象；
- 无 token 的超大请求仍由认证中间件优先返回既有平面 `401 missing bearer token`；合法 token 但非法
  key 仍在读 body 前返回既有平面 `400 invalid setting key`。

| 合同点 | 审计时 OpenReader | 裁决 |
|---|---|---|
| wire 总量 | `ShouldBindJSON` 没有声明或实际读取上限，随后把完整 value 写 SQLite 并在响应回显。 | **must-fix security adaptation**：单个 PUT wire body 最多 8 MiB。 |
| JSON 文档边界 | Gin 只 decode 首值，第二值或垃圾被忽略。 | **must-fix**：只接受一个 JSON 值；仅尾部 JSON whitespace 合法。 |
| 认证/key 优先级 | JWT middleware 先于 handler；handler 又在 bind 前规范化 key。 | **aligned**：无 token `401`、非法 key `400` 均不读取 body。 |
| value 类型 | 服务端接受任意合法 JSON；浏览器仅消费 object，旧直接客户端可能写 scalar/null。 | **deployed compatibility**：本轮不新增 object-only 限制。 |
| 三键映射 | 仅 `reader/shelf/search`；reader 顶层删除 `pageMode/miniInterface`。 | **technical-stack-equivalent / aligned**，保持。 |
| CAS 与 force | 普通 stale 写返回当前行、`200` 和 conflict header；确认备份可 `force`。 | **aligned**，不得因 decoder 重排。 |
| 并发初写 | `(user_id,key)` conflict upsert 已签收。 | **aligned**，边界通过后继续原事务。 |
| 历史持久化 | SQLite `TEXT` 与 backup restore 可包含审计前产生的大行。 | **data compatibility**：不扫描、不截断、不删除；GET/backup/restore 继续读取。 |

8 MiB 是 Go 服务端显式安全上限，不复制上游未证明的无界行为。固定上游 payload 来自浏览器
localStorage 的配置 JSON，不包含上传资产 bytes；OpenReader 的自定义字体/背景只保存调用者私有 URL。
该上限覆盖正常三键快照并把单请求内存、SQLite 写入和同尺寸响应放大变成可测试边界。

## 3. 目标 API 合同

### 3.1 认证、key 与 wire body

- 路径保持 `PUT /api/settings/:key`，只接受规范化后的 `reader`、`shelf`、`search`。
- 现有 JWT/活动跟踪中间件先执行。缺 token/坏 token 保持现有平面 `401`，不得为检查 body 先读取它。
- 身份通过后先校验 key。非法 key 保持 `400 {"error":"invalid setting key"}`，不读取 body、不查
  `user_settings`、不广播。
- 合法 key 的完整 wire body 最多 `8 << 20` bytes，包含 JSON 标点、转义、未知字段和尾部空白。
  `Content-Length` 与 unknown-length/chunked 必须使用同一实际读取上限。
- 精确 8 MiB 进入既有业务状态机；8 MiB + 1 返回
  `413 {"error":"request body too large"}`。
- 只允许一个 JSON 值。尾部 whitespace 可接受；第二对象/数组/scalar 或非空垃圾保持平面
  `400 {"error":"setting value is required"}`。
- envelope 未知字段继续忽略。缺失 `value`、不能解码的字段类型和 malformed JSON 保持既有 400。

### 3.2 value、CAS 与成功响应

- `value` 继续接受任意合法 JSON，包括 object、array、scalar 和 `null`；本轮不把当前 object-only
  Web UI 误升格为部署 API 限制。
- `reader` object 继续只删除顶层 `pageMode`、`miniInterface`；嵌套同名字段、其它 key 和 scalar
  不变。不得借 body limit 添加设置字段 allowlist 或静默丢弃历史扩展字段。
- `baseUpdatedAt`、`force`、stale conflict、原子初写和当前用户隔离完全保持。拒绝 wire 请求发生在
  setting 查询、upsert 和 event 前。
- 成功仍为 `200 {key,value,updatedAt}`，返回实际清理/持久化值；stale 普通写仍加
  `X-OpenReader-Setting-Conflict: 1` 并返回当前行；显式 force 仍只对合法当前用户 key 生效。
- durable write 后仍广播一次当前用户 `settings_update {key,updatedAt}`；overflow/malformed/multi JSON、
  invalid key、stale conflict 和数据库失败不得广播成功事件。
- 错误、日志与 event 不得包含 setting value、JWT、私有 asset URL、host path、SQLite 文本或请求 body。

## 4. 数据与迁移边界

- 不修改 `user_settings` schema、唯一索引、现有行、时间戳、JWT、浏览器 key、`data/`、`cache/`、
  `library/`、backup/WebDAV 或 portable format。
- 8 MiB 只限制未来 HTTP PUT wire body；`GET` 不拒绝、不截断审计前的历史大行。
- logical/portable/WebDAV restore 继续受各自 archive/entry/transaction 限制，不复用 HTTP PUT 上限，
  从而可无损保留旧 OpenReader setting 行。恢复后的下一次 Web PUT 若超过新 wire 上限必须明确失败，
  不能截断或部分覆盖旧行。
- body 拒绝发生在 SQLite 查询/写入、backup、文件计划和 sync event 前；无需 schema migration 或回填。

## 5. 测试先行闸门

实现前必须先让以下合同在当前代码上失败：

1. `reader/shelf/search` 分别覆盖 declared/chunked 8 MiB + 1，均为精确平面 413；失败不新增/替换行，
   不改变 `updated_at`，不广播。
2. 三键分别拒绝第二 JSON 与尾随垃圾为精确 400；尾部 whitespace 和 unknown envelope 字段保持成功。
3. 精确 8 MiB 成功并保存完整 marker；8 MiB + 1 不进入查询/upsert。测试不得把 JSON value bytes 与
   wire bytes 混为同一上限。
4. 无 token/非当前身份的超大请求保持既有 401；非法 key 超大请求保持既有 400 key error，且均不泄漏
   body。另一个用户的同 key 行不变。
5. object/array/string/number/bool/null 继续可 round-trip；reader 只清理顶层设备字段。
6. 普通 CAS conflict、force 覆盖、八路并发初写、实际持久化响应和一次 post-commit event 的旧测试继续
   通过；rejected body 零 event。
7. 直接预置超过 8 MiB 的历史 `UserSetting` 后，GET、logical backup 和 restore 仍不截断；无需在前端
   创建这类 payload。
8. focused/full/race/vet、frontend 全量和 production build 通过；隔离生产形态真实 HTTP 覆盖
   declared/chunked、双 JSON、精确边界、认证/key 优先级和 SQLite 零副作用。

## 6. 实施边界

实现应复用 `request_body.go` 的窄有界单 JSON decoder，由 settings handler 映射现有平面错误体；不得
改成全局 body middleware，也不得顺带给 book/source/RSS/replace-rule/上传入口套用相同上限。handler
继续负责 key、value、CAS、持久化和响应。该切片没有可见 UI 几何变化，不新增前端 maxlength 或提示。

## 7. 实施与回归结果（2026-08-12）

- 合同先以 `7233add` 独立提交。随后
  `backend/api/user_setting_write_boundary_contract_test.go` 在旧实现上复现 declared/chunked 8 MiB + 1、
  第二 JSON 和尾随垃圾仍写库并广播，再由 `c2bc736` 实现转绿。
- `settings.go` 只在合法 key 后复用既有 `decodeBoundedSingleJSON`：完整 wire body 最多 8 MiB，overflow
  映射到既有平面 `413`，其它 decode 失败映射到既有平面 `400`。未引入全局 middleware 或其它端点上限。
- 专项测试覆盖三键、两种传输方式、精确 8 MiB、尾部 whitespace、未知 envelope 字段、六类 JSON value、
  reader 顶层字段清理、另一个用户、零错误广播，以及超过 8 MiB 历史行的 GET/logical backup/restore。
  既有 CAS、显式 force 和八路并发初写测试继续通过。
- `go test ./...`、focused race、`go vet ./...`、frontend 740/740、production build 和 `git diff --check`
  全部通过。隔离生产形态真实 HTTP 又确认 declared/chunked `413`、精确 8 MiB `200`、双 JSON/垃圾
  `400`、拒绝后 marker 不变，以及无 token `401`/非法 key `400` 优先级；服务随后停止。
- 没有 SQLite/schema/JWT、现有 setting 行、`data/`、`cache/`、`library/`、backup/WebDAV、浏览器 key
  或前端几何变化。本地 arm64 候选已从 `c2bc736` 构建并带正确 OCI revision label。
- fresh/historical mounted-volume 门和正式 amd64/arm64 GHCR 发布仍 pending：访问 OrbStack socket 的提升
  权限自动审批通道在执行时断开并拒绝，本轮没有绕过审批，也没有发布远端标签或填写 digest。
