# P2 备份、恢复与 WebDAV 工作台固定基线合同

状态：**2026-07-22 固定上游审查、测试与实现已完成；三视口浏览器和 Docker/旧卷发布门禁待完成。**

本合同以 `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
为唯一产品基线，纠正此前把“ZIP 结构预检通过”概括成“备份/WebDAV 已完成”的审计结论。
它重新打开的是工作台备份入口、普通逻辑 ZIP 的字段/文件名兼容、生成可靠性、恢复事务和书源权限；
不重新打开已经单独验收的原始 WebDAV 协议，也不改变
[`portable-local-archive-backup-p1e4-contract.md`](portable-local-archive-backup-p1e4-contract.md)
定义的 `openreader-portable-v1` 恢复语义。新 trigger 的 v2 资产扩展另见
[`portable-appearance-assets-p2b-contract.md`](portable-appearance-assets-p2b-contract.md)。

## 1. 上游权威与当前映射

| 合同层 | 固定上游权威 | OpenReader 当前对应 |
|---|---|---|
| Index 工作台动作 | `web/src/views/Index.vue#saveUserConfig`、`restoreUserConfig`、`backupToWebdav` | `frontend/src/layouts/AppLayout.vue` |
| WebDAV 文件管理 | `web/src/components/WebDAV.vue` | `WebDAVBrowser.vue`、`OverlayWebDAV.vue` |
| 用户配置快照 | `UserController.kt#saveUserConfig`、`getUserConfig` | `GET/PUT /api/settings/:key`、reader/preferences Pinia store |
| 逻辑 ZIP 写入/恢复 | `WebdavController.kt#backupToWebdav`、`BookController.kt#saveToWebdav`、`syncFromWebdav` | `services/backup/backup.go`、`api/backup.go`、`api/webdav.go` |
| 数据实体 | `Book.kt`、`Bookmark.kt`、`ReplaceRule.kt` 及书源/RSS JSON | Go models、source compatibility encoder、restore DTO |

### 1.1 上游可见状态转换

1. `备份用户配置` 先确认，再把当前终端的 `config`、`shelfConfig`、
   `searchConfig`、`customConfigList` 覆盖写成该用户的 `userConfig` 快照；成功后才提示“备份成功”。
2. `同步用户配置` 先确认，再读取该快照、替换四个本地缓存并重新 hydrate store。
   没有快照时明确返回“没有备份文件”，不能把当前本地值反向创建成备份后报告恢复成功。
3. WebDAV 区域的 `文件管理` 打开唯一的 WebDAV 根 Dialog；`保存备份` 是确认后的直接动作，
   不先打开第二个备份管理器。
4. WebDAV Dialog 自己负责目录导航、选择、删除/批量删除、上传、下载、恢复任意 `.zip`，
   以及单个/批量导入 `.txt/.epub/.umd`。恢复成功后刷新 Index 数据。
5. 上游 `backupToWebdav` 要求 WebDAV 根或 `legado/` 中已有最新 `backup*.zip`，
   用当前 `bookSource.json`、`bookshelf.json`、`bookGroup.json`、`rssSources.json`、
   `replaceRule.json`、`bookmark.json` 替换其中同名数据，再生成 `backupYYYY-MM-DD.zip`。
   `syncFromWebdav` 恢复这六类数据，并从 WebDAV `bookProgress/` 读取阅读进度。

## 2. 工作台与用户配置合同

| 关注点 | 当前证据 | 分类与要求 |
|---|---|---|
| `文件管理` | `overlay.openWebDAV()` 打开 `WebDAVBrowser`。 | `aligned`：保留唯一文件管理器及其已发布的 Bearer/Basic、调用者根和原子文件操作。 |
| `保存备份` | `overlay.openBackup()` 打开 `OverlayBackups`；该弹层再次提供列表、上传、下载、恢复和保存。 | `must-fix`：侧边栏确认后直接调用普通备份 API；文件列表、上传、下载和恢复只归 WebDAV 管理器。删除第二套业务流及其错误测试。 |
| portable 扩展 | `OverlayBackups` 提供“保存完整本地书备份”。 | `acceptable-change`：保留为单独、明确命名的侧边栏直接动作；它不能改名为普通“保存备份”，也不能使第二个列表/恢复管理器继续存在。 |
| 旧 `/settings?panel=backup` | 当前重定向成 `overlay=backup`。 | `must-fix`：兼容链接打开唯一 WebDAV 文件管理器；不得自动执行写操作，也不得复活重复备份弹层。 |
| 显式配置备份 | 三个 store 的普通 CAS 保存发生冲突时会应用服务端值并返回 truthy，调用方仍显示“已备份”。 | `must-fix`：显式备份必须携带受认证、仅限现有 setting key 的 `force` 意图，覆盖当前用户的服务端值；普通 700ms 自动同步仍保留 CAS。任何一项失败都不得显示成功。 |
| 显式配置同步 | `loadReaderSettings/loadPreference` 在服务端无值时会调用 save，可能把“恢复”变成“创建”。 | `must-fix`：显式同步使用 `createIfMissing: false`；缺少任一必需快照时提示“没有备份文件”并不写服务端。成功后再执行允许的额外书架/缓存刷新。 |
| 配置存储模型 | 上游单个 `userConfig`；OpenReader 按 `reader/shelf/search` 三个当前用户 setting 保存，并排除设备本地 `pageMode/miniInterface`。 | `acceptable-change`：这是 Vue 3/Pinia/多终端适配，不增加另一套无界配置文件 API，不恢复设备本地布局。 |

`PUT /api/settings/:key` 可增加可选布尔字段 `force`。只有经过现有 JWT、合法 key 和 JSON
校验的请求才可使用；`force` 只绕过该行的 stale-base 判断，不跨用户、不能更改允许 key，
并继续在持久化成功后广播。未提供或为 `false` 时，现有 CAS 响应和
`X-OpenReader-Setting-Conflict` 完全不变。

## 3. 普通逻辑 ZIP 格式合同

OpenReader 不依赖手机“阅读”App 先创建 Legado ZIP。`POST /api/backup/trigger` 继续在调用者
WebDAV 根创建独立 `backup_*.zip`，这是容器/服务端运行方式所需的
`acceptable-change`。确认文案必须说“创建当前账户备份”，不能声称覆盖手机备份。
独立文件名和 list/download API 保持兼容；ZIP 内容则必须能在固定上游与旧 OpenReader 间往返。

| 逻辑数据 | 上游名称/关键字段 | 旧 OpenReader 名称/字段 | 新生成与恢复合同 |
|---|---|---|---|
| 书源 | `bookSource.json`; `bookSourceName/bookSourceUrl/ruleSearch/ruleBookInfo/ruleToc/ruleContent` | 同文件名，但当前备份错误地直接序列化 `BookSource{name,baseUrl,rules}` | 生成必须复用书源导出 API 的上游兼容 encoder；恢复继续接受上游和旧内部字段。 |
| 书架 | `bookshelf.json`; `bookUrl/origin/originName/name/latestChapterTitle/totalChapterNum/durChapterIndex/durChapterPos/durChapterTime/...` | `title/url/sourceName/lastChapter/chapterCount` 加 OpenReader 分类/变量字段 | 每行同时保留旧字段并写上游别名；恢复接受两组别名，远程书源按名称/URL解析，绝不复用来源数据库 ID。 |
| 分组 | `bookGroup.json` | `bookGroup.json` 与 `categories.json` | 保留现有 mask/分类兼容；同一逻辑分组 artifact 只规划一次。额外 `categories.json` 是 OpenReader 扩展。 |
| RSS | `rssSources.json` 上游 RSS 字段 | 当前已同时写内部字段和上游别名 | 保持并补齐严格错误传播。 |
| 书签 | `bookmark.json`; `time/bookName/bookAuthor/chapterIndex/chapterPos/chapterName/bookText/content` | `bookmarks.json`; OpenReader ID/offset/percent/excerpt/note/timestamps | 同时生成：单数文件使用上游 shape，复数文件使用原 OpenReader richer shape。恢复只出现单数时完成字段映射；两者同时存在时优先复数且只恢复一次。 |
| 替换规则 | `replaceRule.json`; `id/name/group/pattern/replacement/scope/isEnabled/isRegex/order` | `replaceRules.json`; `enabled`、SQLite 插入顺序 | 同时生成上游单数与旧 OpenReader 复数文件。恢复接受 `enabled/isEnabled`，按 archive order/`order` 建立稳定 pipeline；双文件只恢复一次。上游 `group/order` 必须可无损保留，所需列只能通过加性迁移增加。 |
| 进度 | WebDAV `bookProgress/*.json` 和书架 `durChapter*` | `readingProgress.json` | 两者是互补数据，不做别名去重；同一书最终按较新有效时间/现有进度冲突合同合并，不允许旧进度倒退当前用户。 |
| OpenReader 扩展 | 无 | `userSettings.json`、`categories.json`、`chapterVariables.json`、`readingProgress.json` | 保留。固定上游忽略未知项；旧 OpenReader 仍可恢复。不得放入 JWT、密码、WebDAV 凭证、主机路径、SQLite 或 `library/`。 |

恢复先建立“逻辑 artifact 计划”，再写数据库。ZIP 路径匹配大小写不敏感但必须保持现有
规范化/重复路径防护。别名优先级固定：新的 richer OpenReader 文件优先于上游单数别名；
`bookshelf.json` 优先于 `myBookShelf.json`；每个选中的 supported artifact 最多执行一次。
未知文件继续忽略。多个合法 `bookProgress/*.json` 和单独的 `readingProgress.json` 可以共同执行。

## 4. 生成可靠性与恢复事务

### 4.1 生成

当前 `services/backup.Service.run` 直接 `os.Create` 最终路径，并吞掉所有查询、marshal、ZIP entry、
writer close 和 file close 错误；同秒触发还可能碰撞。它可能返回一个可见但不完整的成功文件，
属于 `must-fix`。

新合同：

1. 同一目标根的生成串行化；名字碰撞时选择新的兼容名称，绝不截断已有备份。
2. 所有数据库读取使用同一个稳定只读事务/快照和同一个 DB handle；任一查询失败即终止。
3. 每个 JSON encode、ZIP entry 创建/写入、`zip.Close`、文件 `Sync/Close` 错误都向上传播。
4. 先在同目录创建仅当前进程可写的临时文件；全部成功后原子 rename 到最终名。
   失败必须关闭并移除临时文件，list API 永远看不到半包。
5. API 只有在最终文件已经可读时返回 `200 {message,path,name}`；错误为客户端安全的 `500`，
   不暴露挂载路径或 SQL/ZIP 内部信息。

### 4.2 恢复

现有 archive 大小、路径、symlink、重复名称和 expansion 预检继续保留，但“结构有效”不等于
“内容恢复成功”。当前 dispatcher 忽略大部分 helper 错误并逐行提交，合法 ZIP 中较后的坏 JSON
或数据库错误会得到 `200` 和部分数据，属于 `must-fix`。

新合同：

1. 在首条写入前读取并解码所有选中的 supported artifact，验证顶层类型、变量、分组、规则和
   必需 identity；语法错误/错误顶层类型返回 `400 invalid backup package`，零写入。
2. 书架、分类、分组、设置、RSS、书签、规则、进度以及被允许的全局书源在同一个 SQLite
   事务内执行。任何数据库错误回滚全部逻辑数据；不能以“跳过失败行”吞掉持久化错误。
3. 为历史兼容，语法正确但缺少稳定 identity 的单条旧记录可计入 `skipped`/不计恢复数；
   这一规则必须逐 DTO 明确，不能把任意 decode/DB 错误降级成 skip。
4. 只有 commit 成功后发送 settings/source/category/bookshelf/bookmark/rule/RSS sync 事件。
   回滚或权限跳过不得广播对应资源已改变。
5. 现有 count 字段保留；可增加 `skipped` 和权限提示字段，旧客户端可忽略。

Portable v1 在逻辑计划前仍先完成 manifest/hash/path/容量/identity 的 package-controlled 预检，
并沿用其已记录的文件系统 staging/补偿边界。本合同不把 SQLite 与文件系统宣称为一个不可实现的
跨介质原子事务。

## 5. 权限与多用户合同

| 动作 | 必需权限 | 数据作用域 |
|---|---|---|
| trigger/list/download/upload/restore | JWT + effective `canAccessWebdav` | 管理员旧根或普通用户 `users/<safe-username>/`；检查必须先于 path/body/file 工作。 |
| 恢复个人 setting/shelf/category/RSS/bookmark/rule/progress | 同上 | 只写认证 `userID`。archive 中的 user ID 永不可信。 |
| 恢复 `bookSource.json` | 上述权限 **并且** `canEditSources` | 书源表是当前 OpenReader 的有意全局模型，只有已有书源编辑权限可修改。 |

当前恢复接口只检查 WebDAV 权限，随后无条件调用全局 `importBookSources`，使
`canEditSources=false` 的用户可以借 ZIP 绕过权限，属于安全 `must-fix`。为了不阻断同一包中的
个人数据恢复，后端在 archive 含书源但调用者无编辑权限时：

- 不调用任何书源写路径，`sources` 保持 `0`；
- 继续事务性恢复其个人数据；
- 返回加性 `sourcesSkipped: true`（并可在统一 `skipped` 中计数）；
- 前端显示“个人数据已恢复，书源因权限未恢复”，不能笼统提示全部恢复成功。

备份中可保留调用者可用的全局书源快照用于可移植性，但它不携带写权限；现有书源 header/规则
脱敏与可见性合同不得因备份扩大。任何备份、错误、日志和事件都不得包含 JWT、Cookie、
Authorization、WebDAV 凭证或主机路径。

## 6. 数据与迁移边界

- 不移动或删除 `data/`、`cache/`、`library/`、管理员旧 WebDAV 根或普通用户私有根。
- 不重命名/删除已有 `backup_*.zip`；旧列表、下载和 restore URL 继续工作。
- 旧的仅复数 OpenReader ZIP、仅单数 reader-dev ZIP、Legado `myBookShelf.json`/`bookProgress/`
  ZIP 以及新双别名 ZIP 都必须可恢复。
- 替换规则若增加 `group`/`order`，只能是 nullable/defaulted 加性 SQLite migration；旧行继续按
  `id ASC`，不得重排、去重或删除。其它格式修复优先 DTO/encoder，不授权破坏性 schema 改动。
- 普通逻辑 ZIP 继续不包含本地书原 archive；只有明确版本化的 portable v1/v2 包含受验证的
  原始本地书，v2 另可包含受验证的当前用户自定义外观资产。

## 7. 实现前测试合同

### Backend/API

1. 生成包含上游格式 `bookSource.json`、alias-rich `bookshelf.json`、单数
   `bookmark.json/replaceRule.json` 和旧复数文件；双别名恢复每类只计/写一次。
2. 真实固定上游书签、替换规则、书架和书源 fixture 恢复并可再次导出；旧 OpenReader fixture
   保持字段、顺序、变量、分类和进度。
3. `canAccessWebdav=true, canEditSources=false` 恢复含书源+个人数据的包：个人数据成功，
   全局书源不变，响应 `sourcesSkipped:true`，无 source broadcast。
4. supported JSON 语法/顶层类型错误、注入的数据库写失败均回滚先前有效 artifact，且无 broadcast；
   未知文件仍被忽略。
5. 注入查询、marshal/ZIP 写/close、最终 rename 失败和同秒并发触发：不出现可列出的半包，
   不覆盖旧包，API 不泄漏路径。
6. user-setting 普通 CAS 行为不变；显式 `force:true` 只覆盖当前用户合法 key；显式同步缺少记录时
   不创建记录并得到可显示的“没有备份文件”状态。

### Frontend/browser

1. `保存备份` 确认后直接 trigger；取消无请求；成功/失败文案真实；无 `OverlayBackups` 第二管理器。
2. `文件管理` 仍是唯一 list/upload/download/ZIP restore 入口；portable 是独立、明确命名的直接动作。
3. `/settings?panel=backup` 打开 WebDAV manager 且不自动写；桌面、390×844、360×800 均无重复弹层。
4. 配置备份发送 force；同步缺失不反向保存；三个 setting 任一个失败均没有成功 toast。
5. 无书源编辑权限的部分恢复显示明确提示，并刷新实际已恢复的个人 store。

### 发布门禁

- `cd backend && go test ./...`
- `cd frontend && npm test && npm run build`
- 上述三视口真实浏览器合同。
- 本地 Docker build、真实旧 SQLite/三卷重启、普通及 portable trigger/list/download/restore、
  reader-dev/旧 OpenReader fixture、用户隔离和备份失败无半包检查。

## 8. 实现记录（2026-07-22）

- 工作台已删除 `OverlayBackups`/`useOverlayBackups` 第二管理器；`保存备份` 与明确命名的
  `保存完整本地书备份` 现在均为确认后的直接动作，旧 `panel=backup` 只打开唯一 WebDAV 管理器。
- 显式用户配置备份以 `force:true` 写当前用户的三个合法 setting；普通后台同步仍使用 CAS。
  显式同步以 `createIfMissing:false` 读取，缺失时显示“没有备份文件”且不反向创建。
- 普通逻辑 ZIP 复用上游书源 encoder，书架写双字段，书签/替换规则同时写单数上游文件和
  复数 OpenReader 文件。恢复 planner 固定别名优先级，每类只执行一次；进度以毫秒时间合并，
  旧包不能倒退较新的当前阅读位置。
- 生成使用同目录私有临时文件、完整错误传播、`Sync/Close` 和原子 rename；同进程触发串行且
  同秒文件名不覆盖。恢复在写前解码所有选中 artifact，并在一个 SQLite 事务内执行；数据库
  错误回滚，提交后才广播。
- 无 `canEditSources` 的 WebDAV 用户恢复个人数据但跳过全局书源，并返回
  `sourcesSkipped:true`。替换规则仅增加 `group_name`、`sort_order` 两个默认值列；旧行仍按
  `sort_order=0,id ASC` 执行，不删除或改写已有数据。
- 自动证据：后端 `go test ./...`、前端 534/534、Vite production build、smoke 脚本语法和
  `git diff --check` 均通过。真实 Chromium 三视口启动需要沙箱外权限，本次自动审批因 Codex
  使用额度耗尽被拒；因此浏览器、旧卷 Docker 与镜像发布仍明确保持待完成状态。

这些门禁全部通过前，不再把“用户、备份、RSS、替换规则、书签”整行概括为已完成；
RSS、替换规则、书签自身已验收的交互/API 合同保持有效，只有它们的 archive bridge 被本合同重新打开。
