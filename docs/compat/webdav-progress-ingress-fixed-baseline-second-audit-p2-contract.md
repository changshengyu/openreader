# WebDAV 上传进度接收链路固定基准第二轮盘点（P2）

状态：**inventory-complete / tests-and-implementation-pending**。
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
