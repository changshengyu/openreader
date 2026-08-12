# 用户资产上传/删除请求边界第二轮固定基准合同（P2）

状态：**aligned / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本轮只审查已部署用户资产 API 的 HTTP wire、multipart 生命周期和失败副作用：

- `POST /api/uploads`
- `DELETE /api/uploads`

Reader/BookInfo 的上传后设置事务、内容签名/图片尺寸、用户目录、引用保护、portable v2 资产闭包和
静态 URL 已由既有合同签收，本轮不重建这些流程。合同阶段只修改文档；失败测试和应用实现必须后续
独立提交。

## 1. 权威文件与动作映射

固定上游：

- `src/main/java/com/htmake/reader/api/controller/UserController.kt#uploadFile/#deleteFile`；
- `src/main/java/com/htmake/reader/api/YueduApi.kt` 的 `/reader3/uploadFile`、`/reader3/deleteFile`；
- `web/src/components/BookInfo.vue`、`ReadSettings.vue` 的 multipart 和删除调用；
- `src/main/java/com/htmake/reader/verticle/RestVerticle.kt` 的共享 body/upload 运行时。

OpenReader：

- `backend/api/server.go`、`uploads.go`、`request_body.go`；
- `backend/services/assets/policy.go`；
- `frontend/src/api/uploads.js`、`useReaderAppearanceAssets.js`、`useOverlayBookInfo.js`；
- `bookinfo-shelf-mutations-p2-contract.md`、`reader-appearance-assets-p2-contract.md` 和
  `portable-appearance-assets-p2b-contract.md`。

固定上游认证后接受上传集合，`type` 缺省为 `images`，按原文件名写当前用户 namespace，并返回 URL
数组；删除只允许当前 namespace 下的 URL。原文件名覆盖、仅前缀路径检查、无内容校验和先删文件后改
配置不构成 OpenReader 产品合同。OpenReader 已部署的单文件 `201` object、随机名、JWT user ID、严格
路径、内容校验、引用 `409` 与先保存引用变更后删除，是明确的 API/多用户/安全适配，继续保留。

## 2. 当前证据与差异矩阵

2026-08-12 对 `OpenReader@12036de`、固定上游、Go 1.26 标准库和 Gin 1.10.0 复审确认：

| 合同点 | 当前行为 | 裁决 |
|---|---|---|
| multipart 总量 | `uploadAsset` 直接调用 `c.FormFile`，完整解析后才比较 `FileHeader.Size`。Gin 的 32 MiB `MaxMultipartMemory` 只是内存阈值，剩余数据会写 `multipart-*` 临时文件，不是请求上限。 | **must-fix security adaptation**：在解析前限制实际读取的完整 multipart 包络。 |
| 声明/chunked | 没有 `Content-Length` 快速拒绝，也没有 `MaxBytesReader`；未知长度请求可持续读取并落临时文件。 | **must-fix wire boundary**：两种传输必须共享同一个 actual-read 上限。 |
| multipart shape | `FormFile("file")` 与 `PostForm("type")` 只采用首值；额外 file/type part 会被解析后静默忽略。 | **must-fix request identity**：稳定 OpenReader API 每次只接受一个 `file` 和至多一个 `type`。 |
| multipart 清理 | 生产 `net/http.Server` 在请求结束时清理成功解析的 `MultipartForm`；直接 handler、嵌入式调用或测试没有该收尾保证，handler 本身也不释放。 | **must-fix ownership**：handler 对自己触发的成功解析显式 `RemoveAll`，所有返回分支一致。解析失败由标准库清理部分临时文件。 |
| 文件 admission | 解析后按 `type` 执行 8 MiB 图片/杂项或 32 MiB 字体上限，再验证扩展、magic 和图片尺寸。 | **aligned after wire gate**：文件级上限和现有 `400` 不改；wire gate 不能代替内容 gate。 |
| 最终写入 | JWT user root、随机目标名和最终文件前内容验证已实现；失败前不创建最终文件。 | **aligned**：继续保持，不能因 multipart 修复改用原文件名或共享目录。 |
| 删除 body | `DELETE` 直接 `ShouldBindJSON`，无实际读取上限，且接受首个 object 后的第二 JSON/垃圾。 | **must-fix wire boundary**：16 KiB actual-read、单个非 null JSON object。 |
| 删除身份/引用 | 精确解析 `/uploads/users/<id>/<kind>/<basename>`，外用户 `404`，legacy/逃逸 `400`，仍被 Book/Setting 引用 `409`。 | **aligned**：状态、平面错误和文件语义保持。 |
| 数据/备份 | 本轮入口只写 `data/uploads/users/...`；portable v2 另行拥有跨用户资产打包/重写。 | **aligned**：无 schema、迁移、扫描、重命名或备份格式变化。 |

现有测试覆盖用户隔离、legacy 只读、路径逃逸、引用保护、扩展/magic/尺寸和最终文件失败清理，但没有
证明 multipart actual-read、重复 part、成功解析临时文件释放或 DELETE 单文档。既有绿测不能替代本合同。

## 3. `POST /api/uploads` 精确 wire 合同

- JWT middleware 保持第一道门：缺少/无效 token 的超大 body 仍返回既有 `401`，handler 不解析
  multipart、不创建用户目录、最终文件或应用拥有的临时文件。
- 完整 multipart wire body 上限为 `33 << 20` bytes，即 32 MiB 最大字体加 1 MiB 固定包络预算。
  `Content-Length` 大于上限时解析前返回
  `413 {"error":"request body too large"}`；chunked/未知长度在实际读取第 `33 MiB + 1` byte 时返回同一
  状态和错误。上限以内才进入 multipart 和文件级校验。
- 请求必须是可解析的 `multipart/form-data`，且所有 file part 合计恰好一个、字段名必须为 `file`。
  重复 `file`、额外其它名字的 file part 或缺少 file 均返回 `400`，不得采用第一项后静默成功。
- `type` scalar part 可省略，省略继续映射 `misc`；存在时只能有一个，trim 后最多 32 UTF-8 bytes。
  `cover/covers`、`background/backgrounds`、`font/fonts` 映射保持；其它短值继续按既有兼容语义映射
  `misc`。重复或超长 `type` 返回 `400`。未知非文件 scalar part 不参与语义，但仍受总包络约束。
- 原始上传文件名只用于扩展判断和成功响应 `name`，不得用于目标路径；trim 后 UTF-8 bytes 最多
  255，空名/超长名返回 `400`。合法 Unicode 文件名可保留在响应，不写日志或主机路径。
- multipart shape/metadata 错误统一为
  `400 {"error":"invalid upload request"}`；真正缺少 file 保留
  `400 {"error":"file is required"}`。不得把 parser 文本、边界、临时路径或原始 header 回显。
- `FileHeader.Size` 大于对应 kind 的 8 MiB/32 MiB 时继续返回既有
  `400 {"error":"file is too large"}`。扩展、内容签名、图片尺寸和 `201 {url,name,size,type}` 完全保持。
- 一旦 multipart 成功解析，handler 必须在每个成功/失败返回分支后调用该 form 的 `RemoveAll`。这只
  清理 Go multipart 临时文件，不得清理已成功发布的随机目标文件。解析中途失败由标准库的失败清理
  负责，handler 不允许按客户端文件名拼接额外清理路径。

33 MiB 是 HTTP 包络而不是新的文件上限；它给合法 32 MiB 字体保留固定 metadata/boundary 空间，同时
关闭额外 part、超长 scalar 和 chunked body 对磁盘/内存的无界消耗。

## 4. `DELETE /api/uploads` 精确 wire 合同

- JWT 继续优先；成功认证后只接受一个非 null JSON object `{url:string}`，actual-read wire 上限
  `16 << 10` bytes。精确上限进入业务校验，`16 KiB + 1` 的 declared/chunked 请求返回
  `413 {"error":"request body too large"}`。
- 空 body、`null`、错误顶层类型、第二 JSON、尾随非 whitespace 垃圾、空白 URL 和 wrong-type URL
  都返回既有 `400 {"error":"url is required"}`，且不执行引用查询或文件删除。合法文档后的 whitespace
  可接受；未知 JSON 字段可与合法 URL 一同忽略，保持 Go 客户端兼容。
- 合法 body 后继续执行既有 URL 规范化、owner、引用和文件语义：非法/legacy 为 `400 unsupported upload
  url`，非 owner 为 `404 upload not found`，引用中为 `409 upload is still in use`，删除/幂等缺失成功为
  `200 {"deleted":true}`，内部查询/删除失败保持安全 `500`。
- 本轮不声称 Book/Setting/restore 与物理删除之间已具有跨请求原子引用锁；它只保持现有“删除前立即
  查询引用”的顺序。若后续改变共享引用并发模型，必须单独盘点所有引用写入口，不能在本 wire 切片
  中只锁一个 handler 后误报原子性。

## 5. 数据、兼容与失败副作用

- 不改变路由、frontend FormData/JSON、成功响应、文件格式、SQLite、`data/cache/library` 根、静态
  `/uploads` 可读路径、普通备份、portable v1/v2 或 WebDAV 格式。
- 历史 oversized 文件、legacy `/uploads/<kind>/...` 与已有数据库/设置 URL 不扫描、不移动、不删除；
  新 wire 上限只约束未来 HTTP 请求。
- 任一 400/401/413/500 不得创建最终用户资产、Book/Setting 行、同步事件或目录外文件。已存在同名
  客户端文件不能被覆盖，因为目标仍使用服务器随机名。
- cleanup 错误不回显临时路径；最终响应已确定时不得用清理失败覆盖业务响应。测试必须自行清理旧实现
  暴露的临时文件，避免红测污染开发机。

## 6. 失败测试先行闸门

实现前必须先让 `OpenReader@12036de` 在以下合同上失败：

1. `POST` 的 declared 与 chunked `33 MiB + 1` 均精确 `413`；无 token 的同一请求仍为 `401`。所有
   overflow/invalid shape 后用户资产根没有最终文件。
2. 一个合法 `file` 成功；重复同名 file、合法 file 加其它 file 字段、重复 `type`、超长 `type` 和超长
   filename 都为 `400`，不能只采用第一项。省略 `type` 继续得到 `misc`。
3. 通过把测试 router 的 `MaxMultipartMemory` 降到很小，证明成功、文件级拒绝和 shape 拒绝后，
   `request.MultipartForm` 中的临时文件都已不可打开；成功发布的目标文件仍可读取。
4. 8 MiB 图片/杂项和 32 MiB 字体文件级边界继续使用既有错误与类型验证；multipart 包络不能误拒绝
   合法最大字体。内容 fixture 可使用受控 reader，避免把巨型常量写入仓库。
5. `DELETE` 的 declared/chunked 16 KiB + 1 为 `413`；第二 JSON、垃圾、`null`/wrong shape 为 `400`，
   原文件保持。无 token 仍 `401`；有效 URL 的 200/404/409/legacy/path traversal 回归保持。
6. focused 测试后运行 `go test ./...`、相关 race、`go vet ./...`、frontend 全量和 production build；用真实
   HTTP server 复测 declared/chunked、multipart success/delete 和磁盘结果。本切片没有 UI 几何变化，
   不新增浏览器视口门，但既有上传调用单元测试必须保持。

## 7. 实施边界

- 实现应在 `uploads.go` 使用窄 helper，把 `Content-Length` 快速拒绝、`http.MaxBytesReader`、multipart
  shape/metadata 校验和 `RemoveAll` 生命周期集中起来；不得修改全局 Gin `MaxMultipartMemory` 或增加
  全局 body middleware。
- DELETE 复用 `request_body.go#decodeBoundedSingleJSON`，由 upload handler 映射本文既有平面错误。
- 不把整个文件再次读入内存；内容验证继续读取 `FileHeader.Open()`，最终写入继续流式复制到随机目标。
- 合同、红测、实现、验证/发布记录依次独立提交。

## 8. 实施、验证与发布结果（2026-08-12）

- 合同 `ce38478`、旧实现红测 `bb0c067`、实现 `f386016` 和可重复真实 HTTP 探针 `be83a0f`
  已按门禁顺序独立提交并推送。旧实现反例精确证明 declared/chunked 超限、重复 file/type、超长
  metadata、multipart 临时文件和 DELETE 多 JSON 缺口，不是仅由代码审查推断。
- `uploads.go` 现在在解析前执行 33 MiB `Content-Length`/`MaxBytesReader` 总包络，完整检查唯一 file、
  至多一个 type 与 filename/type byte 上限，并由 handler 对成功解析的 form 统一 `RemoveAll`。DELETE
  复用 16 KiB 有界单 JSON primitive。既有 8/32 MiB 文件上限、内容校验、用户根、引用保护、成功
  shape 和错误语义保持。
- 新合同 focused、既有 BookInfo/Reader 资产 focused、Go 全量、focused race、`go vet ./...`、frontend
  740/740 和 Vite production build 通过。真实宿主、arm64 候选容器和 GHCR 回拉容器三次运行
  `scripts/smoke/user-asset-write-boundary-contract.mjs` 均通过 declared/chunked 413、认证优先、重复 part、
  正常上传/静态读取、DELETE single JSON/exact limit、最终文件删除和 `multipart-*` 清理。
- 本地 `be83a0f` 候选先通过 fresh volume 的 portable-v1、portable-v2-assets、cross-user、restart。顺序
  historical 第一次在 fixture 后出现无上下文 `curl 404`，未记为通过；保留现场并启用 shell trace 的
  原样重跑完整通过 TXT、EPUB、UMD、CBZ、relative-cache、owner-isolation、portable restore/restart
  与归档 hash。
- 本机完成 amd64/arm64 构建并发布 `ghcr.io/changshengyu/openreader:be83a0f` 与 `latest`；两标签远端
  回读为 OCI index
  `sha256:e1f31f3dd728bc27fbc89bbc8c21f81e8c5511c5e99196891feb21cd47138b73`。amd64 manifest 为
  `sha256:eef842be04742dd892158039ea7c31581be2c854bec293c977b187b36c8303f5`，arm64 为
  `sha256:bac0fb70f1fe5ccf111920d7d9b80e20a2b811bbb5a439da94b5bf33222c2bc2`，两平台 revision label
  均为 `be83a0f7bdafe68fe5dfda5f590b90ec3a9d9615`。

本切片的允许差异仍是 OpenReader 已部署的单文件 object API、随机名、多用户路径、内容校验、引用保护
和 portable v2；没有复制固定上游的多文件数组、原文件名覆盖或弱路径删除。未完成项仅为用户真实设备
上传/删除签收，以及全项目其它尚未逐动作复审的长尾；生产实例当前运行 commit 仍未知。
