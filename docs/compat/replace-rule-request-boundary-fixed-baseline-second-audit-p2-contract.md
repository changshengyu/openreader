# ReplaceRule 请求边界第二轮固定基准合同

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只审计已签收 ReplaceRule 模块的 JSON wire、工作准入和请求取消边界。可见管理器/编辑器、
Reader 替换语义、SQLite 顺序、备份恢复和允许差异继续由
[`replace-rule-fixed-baseline-p2-contract.md`](replace-rule-fixed-baseline-p2-contract.md) 约束，不因当前
`ShouldBindJSON` 长尾而重开。

## 1. 权威源码

### reader-dev

- `src/main/java/com/htmake/reader/api/YueduApi.kt:337-341`
- `src/main/java/com/htmake/reader/api/controller/ReplaceRuleController.kt:73-231`
- `src/main/java/io/legado/app/data/entities/ReplaceRule.kt`
- `web/src/components/ReplaceRule.vue`
- `web/src/components/ReplaceRuleForm.vue`

固定上游在认证后把单条保存/删除解析为 JSON object，把批量保存/删除解析为 JSON array；保存按精确
name 更新最早位置或追加，批量按输入顺序处理，精确空 name/pattern 行跳过。上游没有可照抄的 body、
字段、匹配或输出预算，OpenReader 已发布的安全上限继续作为允许适配。

### OpenReader

- `backend/api/server.go:182-188`
- `backend/api/replace_rules.go`
- `backend/api/request_body.go`
- `backend/services/replacerules/engine.go`
- `frontend/src/api/replaceRules.js`
- `frontend/src/composables/useOverlayReplaceRules.js`

## 2. 路由与 wire 合同

五个 JSON 入口都必须在认证后执行 actual-read admission，只接受一个非 null UTF-8 JSON 文档。完整 wire
字节数包含 JSON、转义和尾随空白；已知 `Content-Length` 与 chunked/未知长度实际读取使用同一边界，恰好
等于上限成功，上限加一返回 `413 {"error":"request body too large"}`。第二个 JSON value、尾随垃圾、
非法 UTF-8、`null` 和错误顶层类型返回 400；除下节明确保留 target-first 的 `PUT` owner lookup 外，
不得查询后续业务目标、执行规则、写 SQLite 或广播。

| 路径 | 顶层与上限 | 业务准入与成功语义 | malformed 400 文案 |
|---|---|---|---|
| `POST /api/replace-rules` | object；512 KiB | 保留精确字段、默认值、字段预算、RE2 验证和当前用户精确 name-upsert；append `201`、replace `200` | `pattern is required` |
| `PUT /api/replace-rules/:id` | object；512 KiB | 路径/owner target 先于 body；随后使用同一字段验证，更新稳定 ID，重命名冲突 `409` | `invalid replace rule` |
| `POST /api/replace-rules/batch` | array；16 MiB；1..2,000 个 raw row | 只 skip 精确空 name/pattern；其余整批预检后按输入顺序单事务 upsert | `invalid replace rules payload` |
| `POST /api/replace-rules/batch-delete` | object `{ids}`；128 KiB；最多 2,000 个 raw ID | cardinality 在去重/查询前检查；只按请求中首次出现顺序删除当前用户正 ID | `invalid replace rule ids` |
| `POST /api/replace-rules/test` | object；4 MiB | 保留 pattern/replacement 字段预算、1 MiB text、20,000 matches、8 MiB output；只返回测试结果 | `pattern and text are required` |

未知 JSON 字段继续忽略，以保留 reader-dev entity、旧客户端和备份别名 round-trip；本轮不引入 strict
unknown-field 拒绝。字段上限仍按解码后 UTF-8 字节计数，raw 非法 UTF-8 不允许先替换为 `U+FFFD` 再成为
所谓“精确字节”规则。

## 3. 错误与优先级

1. 认证 middleware 先于所有 body 大小、语法和业务信息，未认证请求保持既有 `401`。
2. `PUT /:id` 保留 path/owner target-first：非法 ID 是既有 `400`，缺失或他人 ID 是 `404`，均不得因
   随附 oversized/malformed body 泄漏另一种裁决。
3. 其余四路先完整准入 body/字段/cardinality，再查询、执行、持久化或广播。
4. 只有实际 body overflow 使用 413；缺字段、错误 shape、第二文档、字段超限、regex/执行预算继续 400。
5. 成功响应、`created/updated/skipped/deletedIds`、HTTP 状态和 post-durable-write WebSocket envelope 不变。

## 4. 事务与取消

- 所有 list/mutation GORM 调用使用 `c.Request.Context()`；批量 upsert/delete 的 transaction 和内部查询/
  写入共享该 context。
- batch 循环在每一项前检查 context。提交前取消必须回滚整批，不留下部分行或广播，也不得继续后续
  查询/写入。
- 单条 create/update/delete 若 context 使数据库操作失败，不得广播成功事件。若 durable commit 已成功，
  仍发送既有一次收敛事件，即使客户端随后断开；不能产生“已提交但永不通知”的状态。
- 隐藏 `/test` 已有 1 MiB 输入、20,000 匹配和 8 MiB 输出硬预算；本轮只在执行前后尊重已取消 context，
  不改 `services/replacerules.Apply` 签名，不重写已签收的 Reader pipeline。

## 5. 数据与兼容边界

- 不改 `replace_rules` schema、ID、`user_id`、时间戳、`group_name/sort_order` 或既有重复名称。
- 不迁移、trim、规范化或重写历史 name/pattern/replacement/scope；legacy 空 scope shim 不变。
- 不改 `replaceRule.json`、`replaceRules.json`、portable/reader-dev/Legado restore 或 WebDAV 文件。
- 不改前端 payload、旧 URL、JWT、多用户隔离、规则执行顺序、RE2 子集或输出预算。
- 被 body/字段/cardinality/context 拒绝的请求必须对 SQLite、Hub、Reader cache、backup 和文件系统零副作用。

## 6. Inventory 时差异矩阵

| 层 | 当前证据 | 判定 |
|---|---|---|
| actual-read / 413 | 五路仅 `MaxBytesReader + ShouldBindJSON`；Gin bind overflow 落入各路普通 400 | `must-fix` |
| 单文档 / UTF-8 | `ShouldBindJSON` 只 decode 首个 value；`encoding/json` 可把非法 UTF-8 替换后继续 | `must-fix` |
| shape / null | object `null` 可进入零值业务验证；第二 value 未被消费 | `must-fix` |
| 事务取消 | GORM 调用未绑定 request context，2,000-row batch 断开后仍可继续 | `must-fix` |
| field/cardinality/执行预算 | 512 KiB/16 MiB/128 KiB/4 MiB、2,000 rows/IDs、字段/RE2/match/output 已存在 | `aligned`；只补 exact-edge 回归 |
| UI/Reader/SQLite/backup | 已由 `a7abcdd` 模块合同和 Docker 证据签收 | `closed`；不得借本轮重构 |

上述 `must-fix` 是实现前红灯证据。`9f5a52b` 已关闭四项请求/事务差异；可见模块、持久格式和允许差异
保持不变。

## 7. 测试先行门

应用实现前，新增聚焦 Go 合同并在旧实现上证明失败：

1. 五路分别覆盖 declared/chunked exact-limit 与 `+1`、第二 JSON、尾随垃圾、非法 UTF-8、`null` 和错误
   top-level；overflow 必须稳定 413，其余 malformed 必须保持路由文案。
2. 所有拒绝验证零 SQLite mutation、零 Hub event、零 `/test` 执行结果；PUT 另锁定 auth/path/owner
   target-first。
3. batch upsert/delete 锁定 raw 2,000 接受、2,001 拒绝和 pre-dedupe 语义；取消在 commit 前回滚整批，
   不继续后续 row、不广播。
4. 成功回归继续锁定精确 whitespace、默认值、same-name stable ID、skipped、输入顺序、ordered
   `deletedIds`、test 结果和既有错误状态。

实现后运行：

```bash
cd backend && go test ./api -run 'TestReplaceRuleRequestBoundary' -count=1
cd backend && go test -race ./api -run 'TestReplaceRuleRequestBoundary' -count=1
cd backend && go test ./...
cd backend && go vet ./...
cd frontend && npm test
cd frontend && npm run build
```

再以真实 Go 服务验证 declared/chunked 请求，并运行现有 ReplaceRule manager/Reader smoke 的
1440x900、1024x1366、390x844、360x800 视口，确认正常 create/toggle/import/batch-delete 与 Reader
共享 editor 未回归。形成可验证切片后才执行 fresh/historical/portable 卷门和本机多架构发布。

## 8. Inventory 结论

2026-08-16 重新扫描 `backend/api` 后，除公共 bounded decoder 和已签收入口外，剩余直接
`ShouldBindJSON` 全部集中在 `replace_rules.go` 这五路。它们是当前 REST action 长尾的下一项 must-fix；
本 inventory 阶段只新增/更新合同文档，没有修改应用或测试。

## 9. 实现、验证与发布

- `ff6d7e3` 固化本合同，`c70f04e` 在旧实现上证明五路 declared/chunked overflow、第二 JSON、尾随垃圾、
  非法 UTF-8、null/shape 和预取消缺口，`9f5a52b` 再实施共享 admission 与 request-context transaction。
- 五路现在按 512 KiB/16 MiB/128 KiB/4 MiB 实际读取，只接受一个非 null UTF-8 object/array；真实 overflow
  为稳定 413，其余 malformed 保持路由级 400。PUT target-first、2,000 raw row/ID、精确字符串、默认值、
  stable ID、输入顺序、skipped/deletedIds、RE2/match/output 和 durable-only event 均有聚焦回归。
- focused/full/race/vet、frontend 741/741、production build 和真实 Go HTTP smoke 通过。现有 manager/editor/
  import/toggle/batch flow 在 1440x900、1024x1366、390x844、360x800 四视口通过。
- 本机候选通过 fresh portable-v1/v2-assets、cross-user、restart，以及 historical TXT/EPUB/UMD/CBZ、
  relative-cache、owner-isolation 和 restore 门。随后本机构建并发布
  `ghcr.io/changshengyu/openreader:9f5a52b` 与 `latest`，OCI index 均为
  `sha256:7a72f2d01b26d1d28c35bb13970cb64a1f7dbf97ddebc3aa704957f58f2f56c3`；amd64/arm64 manifests 分别为
  `sha256:333515ea7c5601bbb1567f39f989d63ad377659347bb27986766b143669e142b` 和
  `sha256:1c67d6f6e274fe0638fe77458778d566ed7c90da3dd9ee8ee11739805307933d`。
- Docker CLI 强制 arm64 回拉受 macOS `osxkeychain` `-50` 阻断；只读 GHCR Registry API 已解析远端 arm64
  config `sha256:ca3cc698073f6741075f41300ef0062590d73d59ea87cd842e2fa25115910fd6`，确认
  `architecture=arm64` 且 revision 为完整 `9f5a52b3ea4da8ca557653052c5190d8023dfa61`。这项替代验证不等同于
  用户生产环境部署；生产运行提交仍未知。
