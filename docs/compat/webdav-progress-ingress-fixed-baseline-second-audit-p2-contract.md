# WebDAV 上传进度接收链路固定基准第二轮盘点（P2）

状态：**implemented / regression-validated / Docker-pending**。
固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

## 权威行为与当前缺口

固定 `WebdavController.kt#webdavUpload` 在写入路径包含 `/bookProgress/` 的 JSON 后，调用
`BookController.kt#syncBookProgressFromWebdav`，从 Book 对象读取 `durChapterIndex/Pos/Time/Title`。
`editShelfBook` 只编辑当前 namespace 的现有书，匹配非空 bookUrl，或非空 name+author；不会创建书籍。
解析异常可能在文件写入后产生 500，不能把这种半提交误记为上游的原子保证。

OpenReader `webdavPut` 只调用文件服务并返回 201；readingprogress 服务只实现数据库到 WebDAV 的
逐书镜像，ZIP 恢复是独立动作。当前没有上传文件到 ReadingProgress/书架/在线 Reader 的接收链路。
因此外部阅读 App 成功上传进度后，网页冷启动和在线书架仍停在旧位置，是可见 **must-fix**，不能
凭出站镜像和 ZIP 恢复已通过宣称双向同步完成。

## 目标与适配边界

- 保留两路 PUT、认证、raw file body、成功 201 空 body 和原文件存储布局。只有当前用户的
  `bookProgress/*.json` 或 `legado/bookProgress/*.json` 进入进度识别；无关文件上传保持普通文件行为。
- 必须先完成当前上传身份/字节验证，才能提交进度。不能重新按绝对路径读取已被另一上传替换的
  文件，也不能让迟到上传的数据库副作用覆盖后到文件/较新阅读进度。
- 只匹配 caller-owned 现有书：唯一 URL 优先，否则唯一且非空 name+author；不存在、外书或歧义
  均不创建/修改书。这是多用户和避免错书同步的允许收紧。
- 进度 JSON 使用有界单 UTF-8 object 识别预算（16 KiB），忽略 bounded unknown metadata；index
  必须显式非负，offset 默认 0 且非负；从当前目录得到 chapter ID/title。普通文件的上传预算不改变。
- 更旧/重复时间或相同位置不得倒退现有进度和重复广播；数据库候选使用现有 CAS/事务，不信任
  payload 的数据库 ID，也不把外部时钟直接当作新的服务端 CAS 版本。缺失/无效时间、无法识别的
  JSON 或无效目录可按普通文件保存而不应用进度，作为保护已部署文件上传的允许适配。
- 成功更新才发送当前用户一个既有 `progress_update`，书架投影、在线 Reader 和冷启动恢复使用
  同一 committed 位置；不得回声重写刚上传文件或触发另一普通进度 PUT。文件提交已成功而数据库
  失败的补偿/诊断必须明确记录，不能把失败描述为两个状态都已同步。
- 不增加 SQLite schema、公开 endpoint、配置或 backup 成员；data/cache/library 与既有逐书镜像、
  backup restore、普通 Reader 保存和旧 URL 继续兼容。

## 下一步红测与完成证据

先以真实 Gin/SQLite 测试在当前实现上传合法进度，证明文件 201 但 GET progress/书架仍旧；覆盖
两路、顶层/legado、Basic/Bearer、普通用户私有根。然后实现接收适配，并覆盖第一章 offset=0 的
显式重置、不同章节/精确 offset、canonical title、URL/name-author 匹配、歧义/外书/未知书、旧时间、
重复上传、非法/超识别预算文件、取消、目录变化、同步上传与普通 Reader 保存竞争及数据库失败。

真实 Go/SQLite/WebSocket + 浏览器三视口必须证明外部上传后书架、在线 Reader 与全新上下文恢复
  一致，且没有回声 PUT。最后运行全量 Go/race/vet、frontend/build、fresh/historical/portable 卷门和
  可信双架构发布；不能只用直接数据库更新或 API shape 测试证明可见同步已完成。

## 2026-10-04 红灯证据与位置恢复审查

接收链路基础红测已在 `70d4fa8` 应用代码上执行：两路 × 两目录 × URL/name-author 共八个
fixture，均确认文件 PUT 201 且字节正确，ReadingProgress 仍为旧第一章 offset=5，未成为目标第二章
offset=37。这是接收副作用缺失的直接证据，不能记为普通上传失败。

后续实现必须同时检查百分比缺省语义：当前在线 external updates 与 BookLoad 会将持久
`chapterPercent=0` 当作显式百分比；PositionRestore 又对 `null` 执行 `Number(null)` 得到 0。
外部 payload 只有 `durChapterPos`，因此不能伪造一个 0 百分比覆盖非零 offset。须先补对应前端红测，
使“没有有效百分比”保持缺省，再由真实正文布局验证正向 offset 和第一章 offset=0 显式重置。
已有非零百分比恢复和正常 Reader 保存继续按原合同回归，不能用 API 行值验证替代页面位置验证。

## 接收协调与失败诊断

识别入口在文件服务已规范化的相对路径上判断，读取上传流时仅保留前 16 KiB+1，不在提交后重新
打开文件。接收服务按用户串行化进度上传（包括两前缀与不同文件名），持有可取消、可回收的门直到
文件提交、数据库事务与通知结束；普通文件不进入该门。普通 Reader 保存仍使用原 CAS，事务快照
竞争必须回滚，不覆盖后到普通保存；客户端可重传原文件。

`durChapterTime` 是正整数毫秒、不得晚于接收服务器当前时间。未来时钟、等于/早于现有服务端版本
按普通文件保存且跳过同步。这是无 schema 迁移条件下的服务端时钟安全适配，并不保证不同设备
时钟失准时自动校正。新版本用服务器提交时间，不保存外部时间为 CAS；因此相同外部时间的重传
也不会再次改写/广播。相同 canonical 章节、offset 与 title 保持原记录，包括非零已测量百分比。

文件提交后 SQL/取消失败不回滚或重写已提交的 raw 文件，也不广播；保持 PUT 201 空 body，附加
`X-OpenReader-Progress-Sync: failed`，明确“文件已保存，但阅读进度未同步”。重传是补偿路径；此
附加诊断不宣称跨文件/数据库原子性。目录/文件写入失败仍走既有 WebDAV 状态，绝不提交进度。

持久 `chapterPercent=0` 且 offset>0 是未测量比例的旧格式占位，恢复时优先精确 offset；offset=0
的明确零值和路由显式 `percent=0` 仍是章首，不改变有效非零比例的布局适配。

## 实施与回归证据（2026-10-04）

合同 `9190091` / `49642f0`，红测 `180132a`，实施按顺序落地。新增 readingprogress 接收服务负责
有界 TeeReader、按用户可取消门、匹配与 SQL 事务/CAS；Gin 只调用已 scoped/rooted 文件服务、
映射错误、发送 durable progress_update 与附加失败诊断。无新 schema、配置、公开端点或备份成员。

两前缀 × 两目录 × URL/name-author 的实际 Gin/SQLite 上传、Basic/Bearer、普通用户私有根、
只通知 caller、重复无广播、SQL 失败 201+诊断/raw 文件保留均通过。服务测试覆盖初次进度、
精确 16 KiB/越界捕获、非法 UTF-8/多对象/index/time、未来时钟、旧时间、歧义/外书、同位置保留
已测量比例、显式章首、文件失败、post-file 取消、目录/章节身份变化回滚、排队取消与门回收、
其他用户不受门阻塞、上传 body 工作期间普通 Reader 保存获胜。

Go full、focused API/service race、rootedfs/webdavfs race、vet、Compose 与 frontend 762/762/build
通过。首次扩大 Basic fixture 时书架 GET 错用 Basic 导致 401，已修正为 Bearer 并重跑全量通过，
未扩展 REST 的认证类型。浏览器首次运行发现本机默认 headless shell 版本缺失，后用已安装
Chromium headless shell 1228 验证；失败也会关闭隔离 Go 进程，生产未触碰。

`scripts/smoke/reader-progress-multiclient-contract.mjs` 在真实独立 Go/SQLite、导入两章 TXT、真实
WebSocket 下通过 1440×900、390×844、360×800：Basic raw 上传保持原字节、canonical shelf 位置、
在线第二章 offset=2400 恢复到目标 paragraph（顶边距视口 80±3px）、全新上下文精确恢复、
第一章 offset=0 显式重置和无回声 progress PUT。原有两客户端 CAS、冲突、cold restore 和出站镜像
三视口也一并通过。章首以章节顶边对齐实际滚动视口为证，不把外层文档布局偏移误当倒退。

Docker fresh/historical/portable 与双架构发布待本切片可信 Actions 验证；生产最后确认仍为
`db1ea21`，原书章节恢复已由用户验收，此次进度同步尚无另一台 Mac 的生产签收。
