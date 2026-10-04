# WebDAV PUT 文件系统生命周期第二轮固定基准合同（P2）

状态：**inventory-complete / tests-and-implementation-pending**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`，
`WebdavController.kt#webdavUpload`。本轮只处理上传写入生命周期；其它 DAV 动作和 UI 不重开。

## 上游与差异矩阵

上游在当前用户 home 下读取原始 PUT body，父目录缺失返回 409、目录目标 405、成功创建或覆盖
返回 201，失败 500。上游先删除旧文件再写；OpenReader 已采用 staged replacement，失败保持原文件
属于允许的数据保护适配，不应误写为上游自身提供的保证。Basic/Bearer、私有根和 body 大小预算保持。

| 合同点 | 当前 OpenReader | 裁决 |
|---|---|---|
| 两路 PUT 与原始 body | `/reader3/webdav/*path` 与 `/webdav/*path`；无 query 或 JSON 包络；成功 201 空 body。 | aligned / preserve |
| 创建与覆盖 | Resolve/Lstat 后在绝对 parent CreateTemp，再按路径 Chmod/rename/清理。 | must-fix：读取 body 期间 root/parent 可替换，发布和清理必须相对同一已打开父目录。 |
| 目标身份 | 上传前验证 regular/missing，上传后不复验，可能覆盖期间新放入的目录或文件。 | must-fix：regular identity 或初始缺失必须在发布前仍成立，变化 403。 |
| staged identity | 关闭后按名称 chmod/publish，不核对仍为原 stage inode。 | must-fix：内容、权限、sync 在打开句柄上完成；发布只允许同一 stage identity。 |
| 失败保护 | copy/取消/超限保留 final，但 ancestor 替换会遗漏或错误清理 stage。 | preserve + strengthen：失败清理只在原 opened parent，保留旧 final 和外部实体。 |

## API 与数据合同

- 方法与路径保持两路 `PUT /*path`，认证先于文件访问；body 是文件字节，成功仍为 201 空 body。
- parent 缺失或非目录 409，目录 target 405，symlink/特殊文件/生命周期替换 403，超限 413，
  未预期 I/O 错误 500 空 body；取消不发送新错误正文。已有认证与权限状态不变。
- 从受信 WebDAV boundary 打开根；逐组件拒绝 symlink，保留 root 和 parent 的打开 identity。
  从 stage 创建、copy、chmod、sync、close 到发布和清理只能使用同一 parent fd 的相对操作。
- body 读取后复验 root/所有祖先/parent 仍在原位置且同 identity；target 必须仍是原 regular inode
  或仍缺失，stage 必须同 identity。任何变化 fail closed，不覆盖新的目标或触碰根外 sentinel。
- final 为原文件完整内容或完整新内容，copy/超限/取消/发布前失败不能留下部分 final。原有正常覆盖
  仍允许；这是数据保护适配，不新增条件请求头或改变正常串行 PUT。
- 只改变服务内部文件提交方法；不新增 SQLite、配置、URL、持久目录或备份成员，不迁移 existing
  `data/webdav`，不清理用户数据。服务错误不得带入 HTTP 正文暴露宿主路径。

## 测试与完成门

先添加 body reader 触发的确定性旧实现红测：root/parent 被 symlink 或另一真实目录替换，target 从
regular 或 missing 变为不同文件/目录/symlink，以及 stage 名称被替换。要求 403 对应的服务错误、
原文件和新实体保持、根外 sentinel 保持、仍可定位的请求自身 stage 在原 parent 内清理。
再覆盖创建/覆盖/空文件、权限 0644、exact limit/+1、读取错误、预先及末次读取取消、缺失 parent、
目录/FIFO、私有 scoped root；API 锁定两路状态、认证和根隔离。

实现优先在 `rootedfs` 添加可复用的 opened-parent staged regular-file 写入，WebDAV adapter 映射
既有服务错误。使用 `openat(O_NOFOLLOW|O_EXCL)`、句柄 chmod/sync 和 parent-relative publication；
不得回退到绝对 CreateTemp/Chmod/rename/RemoveAll。最终校验和提交之间发生的新名称不能被
无条件覆盖；新建使用 no-overwrite publication，覆盖使用 identity quarantine 后 no-overwrite
publication，失败恢复不得覆盖同期新实体。

运行 focused/race、WebDAV API、Go 全量/vet、frontend 全量/build、Compose、Linux 双架构编译和
真实 Basic/curl PUT/GET/覆盖/错误 smoke。没有前端改动，不重复 Reader 几何截图。发布须通过可信
Actions 的 fresh/portable、historical volume 和多架构门；发布不自动证明用户生产已升级。
