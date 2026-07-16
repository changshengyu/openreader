# P1-E4-VOLUME-4 可移植本地书 archive 备份合同

状态：**已完成。** 已实现运行时代码、契约/失败测试、真实浏览器确认流程和历史 Docker
三卷 export → upload → restore → read/refresh → restart 冒烟验证；本次 GHCR 发布标签将在发布后记录。

本合同固定比较 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。它不把
当前 OpenReader 的逻辑 ZIP 当成完整本地书备份，也不允许为了加入 archive 而改变已部署的
reader-dev/Legado 恢复语义。

## 1. 审查结论与边界

上游 `WebdavController.backupToWebdav` 先找到阅读 App 已生成的 `backup*.zip`，而
`BookController.saveToWebdav` 只替换 `bookSource.json`、`bookshelf.json`、`bookGroup.json`、
`rssSources.json`、`replaceRule.json`、`bookmark.json`，随后重新压缩。反向的
`syncFromWebdav` 也只同步这些 JSON 和 WebDAV 的阅读进度；它从不导出或恢复
`LocalBook` 的 TXT/EPUB/UMD/CBZ 原文件。

因此下表是产品合同，而不是实现建议：

| 项目 | 上游/现有行为 | OpenReader 判定 |
|---|---|---|
| `POST /api/backup/trigger` 与既有 `backup_*.zip` | reader-dev/Legado 逻辑数据同步；当前 OpenReader 额外携带 RSS、设置、分类、书签、进度和替换规则。 | **必须保持**。不得因为本功能加入 `library/`、SQLite、用户名、JWT、WebDAV 凭证或本地原文件。 |
| 旧 ZIP 的 upload/WebDAV restore | 当前恢复器接受 reader-dev、Legado 与 OpenReader JSON 位置，并在写数据库前做结构性 ZIP 校验。 | **必须保持**。旧 ZIP 仍可恢复，且不需要 manifest。 |
| `library/data/<user>/<book>/` | OpenReader 的 Docker/多用户适配；其中原始 archive 是本地书可恢复来源，`chapters.json`、`bookSource.json`、`content/` 和 EPUB/CBZ 解压资源可再生。 | **必须保留**。挂载卷升级仍是最直接的本地书恢复方式。 |
| 完整本地书迁移 | 上游没有对应的 WebDAV ZIP 格式。 | **允许的 OpenReader 扩展**：新增显式的 portable 包，不改变旧 ZIP 的含义。 |

本阶段的 portable 范围是 `SourceID=0 && Type!=1` 且原始 archive 位于经过
`localBookArchiveRoot` 验证的私有 library 根内的本地书。它包含当前已存档、可读的
`.txt`、`.text`、`.md`、`.epub`、`.pdf`、`.umd`、`.cbz` 原文件；PDF/Markdown 即使不再作为
工作台新导入入口，也必须保留已有 archive 的导出/恢复兼容。Type=1 本地音频目录不是一个
单 archive 模型，必须另立合同，不能在首版 portable 包中悄悄遗漏后仍称“完整备份”。

## 2. Portable v1 包格式

portable 包是独立的 OpenReader 格式，文件名为 `portable_backup_<timestamp>.zip`，保存在当前
调用者的 WebDAV/备份根。它包含现有逻辑 JSON（字段、顺序和序列化规则不变）以及：

```text
openreader-portable-v1.json
local-books/<slot>/original.<extension>
```

`<slot>` 是包内生成的无业务含义安全标识，不使用数据库 ID、用户名或 library 路径。manifest
至少有下列字段：

```json
{
  "format": "openreader-portable-backup",
  "version": 1,
  "createdAt": "RFC3339 timestamp",
  "books": [{
    "bookUrl": "local://book_42",
    "title": "…",
    "author": "…",
    "tocRule": "…",
    "extension": ".epub",
    "entry": "local-books/b0001/original.epub",
    "size": 123,
    "sha256": "lowercase hex"
  }]
}
```

- archive 条目只保存原始字节。`content/`、`chapters.json`、`bookSource.json`、EPUB/CBZ 展开
  资源、预览 cache 和 SQLite 均不打包；恢复后按当前 parser/refresh 合同惰性重建。
- 每个可移植本地书恰好对应一个 manifest 项；manifest 不得包含 library 路径、原始绝对路径、
  user ID、密码、token、cookie 或 WebDAV 凭证。
- 任何本地书 archive 缺失、越界、符号链接逃逸、非 regular file、扩展名不支持、无法读取，或
  Type=1 本地音频存在时，portable trigger 返回 `409`，列出安全的书名/通用原因，且**不写出
  部分 portable ZIP**。逻辑备份仍可独立执行。
- 普通 `backup_*.zip` 不新增 manifest 或 `local-books/`。portable 包不得伪装为旧逻辑包。

## 3. API、前端和恢复状态合同

以下 additive API 已实现；既有路径及其成功响应保持不变。

| Method / path | 请求和响应 | 鉴权/副作用 | 错误与兼容 |
|---|---|---|---|
| `POST /api/backup/portable/trigger` | 无 body。`200` 返回既有 `{message,path,name}` 加 `{format:"openreader-portable-v1",localBooks:n}`。 | `requireStoreAccess`、JWT；始终只导出当前用户的 local books/个人行，即使调用者是 admin 也不能把其他用户 archive 打进包。写入调用者自己的 backup 根。 | `409 {error,unavailable:[...]}` 表示 archive/音频范围未满足；`413` 表示导出超过 portable 限额；`400/500` 不泄露宿主路径。失败不创建可列出的 ZIP。 |
| `GET /api/backup/list` | 继续返回旧逻辑备份；增加 portable 文件，且每项可附加 `format: "logical"|"openreader-portable-v1"`。 | 同现有 caller-scoped 根。 | `format` 是加性字段；旧客户端仍可按 `name/size/time` 渲染。 |
| `GET /api/backup/download/:name` | 支持已验证的 `backup_*.zip` 与 `portable_backup_*.zip` basename。 | 同现有 caller-scoped 根。 | 拒绝其他 prefix、目录、`..` 和跨用户文件，且不将真实路径写入响应。 |
| `POST /api/backup/restore-legado`、`POST /api/backup/restore-webdav` | 旧逻辑 ZIP 走原恢复流程。检测到 v1 manifest 时必须切换到 portable 恢复流程，并在成功结果附加 `localBooks`。 | 仍由当前确认 UI 触发；包只恢复到认证用户。 | 不允许把 portable ZIP 当普通 ZIP 而只恢复书架 JSON。未知/未来 portable version 返回 `400` 且不写库/文件。 |

前端备份弹层新增“保存完整本地书备份”这一明确操作，并在普通“保存到 WebDAV”旁说明其只
保存书架和设置。列表将 portable 标为“含本地书原文件”；上传和 WebDAV 浏览器的恢复确认
会显示将恢复本地书。现有普通备份按钮、列表和 reader-dev 风格的恢复确认不改变。

## 4. 安全、事务与数据一致性

portable 不能复用当前逻辑 ZIP 的“全部读入内存、每条 16 MiB”策略。它新增独立、默认安全的
compressed/entry/total limit，并将上传先有界地写入调用者私有临时区；所有大 archive 按流式
校验、哈希和暂存。实现必须：

1. 在任何数据库或 library mutation 前校验 ZIP 名、manifest schema、条目集合、重复规范名、
   CRC/可读性、每个字节数、总量、SHA-256、文件扩展名、slot 结构、全量 local shelf 映射，及
   目标用户的 archive 路径边界。拒绝绝对路径、反斜杠、NUL、`..`、symlink、重复/大小写冲突
   和未声明 entry。
2. 把原文件写到受控 staging 根并用同一 local parser 限制预解析；不相信 ZIP header 的
   uncompressed size。失败时删除 staging，数据库、既有 library、进度、书签和 WebDAV 备份
   均不变。
3. 防止 `local://book_N` 跨安装 ID 冲突造成静默覆盖。恢复前若目标已有同 identity 但 archive
   hash 不同的 local book，返回 `409` 而不是将进度/书签绑定到错误正文；首版不做隐式合并。
4. 所有包控制的失败（ZIP/manifest/hash/解析/identity 冲突/限额）必须在第一条逻辑恢复写入前
   完成。每本 archive 的 rehydrate 在自己的 SQLite transaction 中完成，并在该 transaction 失败
   时删除本次 archive；持久写入成功后才发送 bookshelf/progress/bookmark sync 事件。旧
   reader-dev/Legado 逻辑 JSON 恢复本身是顺序兼容路径，首版不会为了 portable 格式改写其历史
   upsert 事务边界；极端的磁盘/数据库运行时故障应重试恢复，不能被描述为跨数据库与文件系统的
   全局原子提交。
5. 不复制 archive 的派生 cache。恢复后的第一次阅读可由 archive 重建正文，且不得读取包中或
   旧 SQLite 中的绝对宿主路径。常规逻辑 ZIP restore 对已挂载 local archive 的“不破坏性”合同
   保持不变。

## 5. 先于实现的测试与 Docker 闸门

| 编号 | 夹具/动作 | 必须断言 |
|---|---|---|
| PORTABLE-1 | 同一用户的 TXT、EPUB、标准 reader-dev UMD、CBZ 及一个历史 PDF/Markdown archive。 | manifest 只记录安全 metadata；每个 archive SHA-256/字节一致；普通 backup ZIP 仍不含 `local-books/`。 |
| PORTABLE-2 | 将 PORTABLE-1 上传/下载到新的 data/cache/library 三卷并恢复。 | 每本正文可从恢复 archive 读取；archive hash 不变；目录/派生 content 能惰性重建；进度、书签、分类与设置指向正确书。 |
| PORTABLE-3 | 缺失 archive、越界旧字段、symlink、Type=1 本地音频、损坏 manifest、重复/zip-slip entry、错误 hash、超限输入。 | trigger/restore 失败且无部分 ZIP、SQLite/library/cache mutation、事件或绝对路径泄漏。 |
| PORTABLE-4 | A、B 各有 archive，A 创建/恢复 package，B 访问 A 的 WebDAV 包或把 A 包恢复到 B。 | A package 不含 B archive/个人记录；跨根下载为 404；B 的已有 archive 不被 A 的动作改写；恢复仍仅写认证用户。 |
| PORTABLE-5 | 目标已有同 `local://` identity 但不同 archive。 | `409` 在任何 mutation 前返回；不错误合并书签/进度，也不删除目标 archive。 |
| PORTABLE-DOCKER-6 | 真实历史 SQLite + TXT/EPUB/UMD/CBZ mounted fixture，生成 portable 包后在独立三卷容器恢复并重启。 | release image 在 Docker 中完成 export→download/upload(or scoped WebDAV restore)→read→refresh→restart；原卷与目的卷两者的 archive SHA-256 均保持。 |

实现证据：`backend/services/backup/portable_test.go` 锁定普通逻辑 ZIP 不含原文件、owner-only
archive/hash 和缺失 archive/音频失败；`backend/api/portable_backup_contract_test.go` 锁定完整
archive+逻辑数据恢复、identity 冲突的零写入、通用上传分流、坏 manifest 零写入及 portable
独立单文件预算。`scripts/docker-volume-backup-smoke.sh` 的 `HISTORICAL_VOLUME=1` 路径会生成
TXT/EPUB/UMD/CBZ/相对 cache 历史卷、导出 portable 包并恢复到新的三卷，核对 hash、HTTP
阅读/刷新和重启。浏览器冒烟在真实登录后打开“备份恢复”，确认“保存完整本地书备份”二次确认
明确提示音频目录和缺失原文件会阻止生成。
