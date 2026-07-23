# Reader 自定义外观资产 P2 合同

状态：2026-07-23 已完成固定上游合同提取、P2-A runtime 实施和 Docker 发布；
P2-B 已完成独立合同审计，尚未进入失败测试/运行时实施。

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游权威文件：

- `web/src/components/ReadSettings.vue`
- `web/src/plugins/vuex.js`
- `src/main/java/com/htmake/reader/api/controller/UserController.kt`
- `src/main/java/com/htmake/reader/api/YueduApi.kt`

当前 OpenReader 对应文件：

- `frontend/src/components/reader/ReaderSettingsPanel.vue`
- `frontend/src/composables/useReaderAppearanceAssets.js`
- `frontend/src/stores/reader.js`
- `frontend/src/api/uploads.js`
- `backend/api/uploads.go`
- `backend/api/settings.go`
- `backend/services/backup/portable.go`

## 上游行为合同

1. 背景上传成功后立刻加入 `customBGImgList`、选中为 `contentBGImg`，Vuex
   `setConfig` 同步写入浏览器缓存；当前浏览器刷新后仍可恢复。
2. 字体上传成功后立刻写入当前字体槽位的 `customFontsMap`。固定上游 UI
   只接受 TTF；已上传槽位再次点击时可选择继续上传或恢复默认。
3. 资产写到当前用户 namespace 下的 `/assets/<namespace>/<type>/<filename>`；
   删除只接受当前 namespace 的路径。上游使用原文件名并允许覆盖。
4. 上游先删除文件再修改配置，且只做文件名/namespace 检查。这些顺序和
   内容校验不足不构成 OpenReader 多用户、安全或事务设计的保留理由。
5. 上游普通用户配置备份保存 URL 字符串，不证明资产文件随逻辑备份迁移。

## 当前差异矩阵

| 合同点 | 当前实现 | 裁决 |
|---|---|---|
| 上传后的持久化结果 | `pickBgImage`/`pickFontFile` 上传后只修改 Pinia；`markSettingsDirty` 在 700ms 后后台保存，但 UI 立即提示成功。同步失败或 CAS 冲突时刷新会丢失新选择，并留下孤儿文件。 | **错误重构 / must-fix P2-A**：明确保存并验证返回的 `reader` setting 包含新 URL 后才提示成功。失败时恢复仍属于本次尝试的本地状态，并尽力删除刚上传的新文件。 |
| CAS 冲突 | `saveReaderSettings()` 遇冲突会应用服务器值并返回 truthy；当前资产 composable 把 truthy 当作“本次引用已保存”。 | **must-fix P2-A**：资产动作验证目标引用，而不是只验证响应非空。不得把服务器胜出的旧设置误报为上传/删除成功。 |
| 自身同步广播与保存响应 | `PUT /settings/reader` 提交后，后端会先广播 `settings_update`；`useSync` 立即调用 `loadReaderSettings()`，与仍在等待响应的 `saveReaderSettings()` 共用同一个 operation revision，可能使已经成功的保存返回 `null`，继而误触发资产回滚/清理。三视口真实后端测试已复现。 | **错误重构 / must-fix P2-A**：保存进行中不得由自身广播启动竞争 load。广播时间晚于本地基线时先排队；保存响应落定后，同一时间戳忽略，更晚的远端时间戳再补一次 load。不得丢失其它客户端真正较新的设置。 |
| 上游五个字体槽位 | 设置 UI 暴露系统、黑体、楷体、宋体、仿宋，但 store 的白名单只允许系统、楷体、宋体、等宽；黑体/仿宋上传后被归一化回系统，刷新不能恢复。 | **错误重构 / must-fix P2-A**：`system/hei/kai/serif/fangsong` 五个上游槽位必须完整选择、上传、保存和恢复；既有 `mono` 作为旧 OpenReader 数据兼容槽位只读兼容保留。 |
| 删除顺序 | 当前先从配置移除、显式保存，成功后才 `DELETE /api/uploads`；保存失败会恢复 UI。 | **允许且应保留的事务强化**。继续保证先提交引用变更、后清理文件。 |
| 删除仍被其它配置引用的文件 | 后端对当前用户全部 `UserSetting`/Book 引用返回 `409`；前端把清理阶段的 `409` 当成整个设置动作失败。 | **must-fix P2-A**：引用移除已经持久化时，`409` 只表示文件仍被其它方案/书籍使用；当前设置动作成功，文件安全保留。其它清理错误需提示“设置已保存但文件清理失败”，不能回滚已提交的服务器配置。 |
| 替换字体后的旧文件 | 新 URL 保存后不尝试清理旧 URL，长期形成孤儿；若其它配置仍引用，直接删除又会冲突。 | **must-fix P2-A**：新引用确认后才 best-effort 清理旧的当前用户资产；`409` 安全保留，不影响替换成功。legacy/外部 URL 不自动删除。 |
| 文件类型 | 后端只按扩展名判断；测试中的 `.png`/`.ttf` 甚至是任意文本。合同却声称拒绝“扩展名伪造”。 | **must-fix P2-A 安全边界**：在落盘前核对图片/字体 magic 与类型，图片还要有界校验尺寸；保持现有 8 MiB/32 MiB 限制。 |
| 路径/所有权 | 新资产使用 `/uploads/users/<user-id>/<kind>/<random>`；删除严格校验 JWT user root；legacy 全局路径只读。 | **允许且已验证的多用户强化**：不得退回上游原文件名覆盖或全局目录。公开静态 URL 是已记录的浏览器 capability 适配，不是写/删授权。 |
| 可接受格式 | 当前背景支持 JPG/PNG/WebP/GIF，字体支持 TTF/OTF/WOFF/WOFF2；上游字体 UI 只接收 TTF。 | **明确允许差异**：现代浏览器格式增强可以保留，但每种扩展都必须匹配真实内容签名。BookInfo cover 仍只允许 JPG/JPEG/PNG。 |
| 普通/portable 备份 | `userSettings.json` 与 `bookshelf.json` 保存 URL；portable v1 只额外打包本地书 archive，不包含 `data/uploads/users/...`。恢复到不同实例/用户 ID 后 URL 可能失效或仍指向旧用户。 | **此前证据不足 / must-fix P2-B**：不能再声称 custom assets 已完成 portable 恢复。P2-B 必须设计显式版本化资产 manifest、字节打包、目标用户重写和碰撞/限额合同；不得偷偷改变 reader-dev-compatible 普通逻辑备份格式。 |

## P2-A API 与状态事务

### `POST /api/uploads`

- 请求保持 authenticated multipart `file` + `type`。
- `cover/background/font` 必须拒绝扩展名与文件签名不匹配、空文件、截断的
  可识别图片和超出安全尺寸的图片；失败为 `400`，不得留下最终文件。
- 大小上限、用户私有目录、随机文件名和 `201 {url,name,size,type}` 保持。
- `misc` 仅保留现有兼容入口，不得绕过 image/font 内容检查。

### Reader 上传

1. 保存旧 Pinia 槽位/列表与当前字体。
2. 上传新资产并得到唯一 URL。
3. 更新 Pinia、同步 `@font-face`，显式保存 `reader` setting。
4. 只有返回 setting 精确包含目标 URL，才显示成功。
5. 保存失败：
   - 如果当前 store 仍是本次尝试值，恢复快照；
   - 如果 CAS 已应用服务器值，保留服务器胜出状态，不用旧快照覆盖；
   - 尽力删除刚上传且尚未引用的新资产；
   - 显示同步失败，不显示“已上传”。
6. 字体替换成功后可 best-effort 删除旧的当前用户 URL；`409` 表示仍被其它
   配置引用，静默安全保留。legacy/外部 URL 不触发自动删除。
7. 保存期间收到 `settings_update` 时：
   - 时间戳不晚于 `settingsSyncBaseUpdatedAt` 的事件忽略；
   - 更晚事件在保存落定前排队，不得新开 load 抢占保存 operation；
   - 保存响应把基线推进到同一或更晚时间戳时清除队列；
   - 队列仍比新基线更新时，保存完成后执行一次合并 load。

### Reader 删除/恢复默认

1. 先修改 Pinia 并同步字体。
2. 显式保存并验证返回 setting 已不再包含目标槽位/背景引用。
3. 保存失败时只回滚仍属于本次尝试的本地状态。
4. 保存成功后调用删除；`200` 为已清理，`409` 为仍被其它配置安全引用，
   两者都表示当前设置动作成功。
5. 删除 `500` 等清理错误不得伪装成设置失败；显示部分成功提示并保留服务端
   已提交设置，供后续安全清理。

## P2-A 测试先行闸门

1. composable 单元测试先失败并覆盖：
   - 背景/字体在设置精确保存后才成功；
   - 网络失败回滚并清理新资产；
   - CAS 服务器胜出时不以旧快照覆盖服务器值；
   - 字体替换先保存新引用，再清理旧文件；
   - `409` 清理保留文件但不把设置动作报错；
   - 删除设置失败时不发物理删除。
   - 自身 `settings_update` 先于 PUT 响应到达时，保存仍返回已提交值且不启动
     竞争 GET；真正更晚的远端时间戳在保存落定后只补一次 GET。
2. Go API 测试先替换任意文本伪装的 `.png/.ttf` fixture，并增加：
   - 真 PNG/JPEG/GIF/WebP 与 TTF/OTF/WOFF/WOFF2 签名允许；
   - HTML/随机字节伪装、扩展名错配、空/截断图片拒绝且不落盘；
   - 图片安全尺寸限制；
   - 两用户、引用 `409`、legacy 只读和路径穿越保持。
3. 真实 Go + Chromium 在 1440×900、390×844、360×800 验证：
   - 上传背景、刷新仍选中；
   - 上传/替换字体、刷新后字体 URL 与显示保持；
   - 清除当前引用；其它方案仍引用时文件保留；
   - 强制设置保存失败时 UI 回滚、无成功提示、无孤儿新资产；
   - 无 401/409/500、控制台错误或移动设置几何回归。
4. 全量前端、Go、生产构建及 Reader 核心合同必须通过。

## P2-B 数据与备份闸门

P2-A 不授权修改 portable format。P2-B 的固定格式、无源 user ID 占位符、资产闭包、
限额、跨 user ID 重写、SQLite/文件补偿、旧 v1 与普通逻辑 ZIP 兼容策略及失败测试见
[`portable-appearance-assets-p2b-contract.md`](portable-appearance-assets-p2b-contract.md)。

该合同目前只有审计文档，尚未实施。在 P2-B 完成前，Docker 报告仍必须写明：挂载
`data/` 的卷级备份保存资产字节，普通逻辑/portable v1 只保存引用字符串，不能声称
跨实例恢复自定义资产。

## P2-A 实施结果（2026-07-23）

- 背景/字体上传、替换、删除均以显式 Reader setting 保存结果为事务边界；
  CAS/网络失败不会误报成功，孤儿新资产会 best-effort 回收，`409` 只表示
  其它当前用户引用仍安全保留。
- Reader settings WebSocket 自身广播不再抢占尚未落定的保存响应；同一时间戳
  被合并，真正较新的远端时间戳在保存后只补载一次。
- 五个上游字体槽位全部可选择、上传和刷新恢复，旧 `mono` 槽位继续兼容。
- Go 上传在最终写盘前验证 JPEG/PNG/GIF/WebP 与 TTF/OTF/WOFF/WOFF2
  内容签名，并限制图片尺寸/像素；API 路径、大小限制、响应和用户目录不变。
- 前端 558/558、Go 全量、生产构建通过。真实 Go + Chromium 外观资产合同、
  BookInfo 受影响合同均在 1440×900、390×844、360×800 通过；Reader
  桌面/两种手机/自适应与强制移动 iPad 全矩阵通过。
- 本地 arm64 候选通过新卷重启/备份/portable 恢复，以及历史卷 TXT、EPUB、
  UMD、CBZ、相对缓存和用户隔离 smoke。本机完成 amd64/arm64 正式构建并发布
  `ghcr.io/changshengyu/openreader:9cae206` 与 `latest`；两个标签均核验为
  OCI index `sha256:800cff1326caa8740f343cc233f7ffcd87ef38b38f744b47d1bc7712c27dc7c6`。
