# ReplaceRule P2 固定基准复审合同

状态：2026-07-28 完成合同提取，应用代码与旧测试尚未按本合同重建。

固定上游：
`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

本合同重新核对替换规则的管理器、编辑器、导入、Reader 执行、REST/SQLite 映射和
备份恢复。历史提交、当前组件结构和既有绿灯测试均不构成正确性证明；它们只作为迁移证据。

## 1. 权威源码

### reader-dev

- `web/src/components/ReplaceRule.vue`
- `web/src/components/ReplaceRuleForm.vue`
- `web/src/plugins/config.js#defaultReplaceRule`
- `web/src/views/Reader.vue#filterContent`
- `web/src/views/Reader.vue#showTextFilterPrompt`
- `web/src/App.vue#loadReplaceRules`
- `src/main/java/com/htmake/reader/api/controller/ReplaceRuleController.kt`
- `src/main/java/io/legado/app/data/entities/ReplaceRule.kt`

### OpenReader 当前映射

- `frontend/src/components/overlays/OverlayReplaceRules.vue`
- `frontend/src/composables/useOverlayReplaceRules.js`
- `frontend/src/composables/useReaderSelectedTextActions.js`
- `frontend/src/stores/overlay.js`
- `frontend/src/api/replaceRules.js`
- `backend/api/replace_rules.go`
- `backend/api/books.go#applyUserReplaceRules`
- `backend/api/webdav.go#restoreReplaceRulesFromData`
- `backend/services/backup/backup.go#addReplaceRules`
- `backend/models/models.go#ReplaceRule`

## 2. 上游用户界面合同

### 管理器

上游 `ReplaceRule.vue` 是根级普通页面 Dialog：

- 标题固定为 `替换规则管理`。
- 桌面宽度为视口宽度 70%，限制在 `750px…1000px`；按共享 `dialogTop` 垂直定位。
- mini interface 使用全屏 Dialog，不另建移动卡片页面。
- 标题栏右侧唯一动作是 `导入`；文件输入隐藏。
- 表格按保存数组顺序显示，列严格为：
  1. 多选，宽 `25px`，mini 时固定；
  2. `规则名称`，最小宽 `150px`，mini 时固定；
  3. `替换范围`，最小宽 `150px`；
  4. `是否启用`，最小宽 `80px`，使用 switch；
  5. `操作`，宽 `100px`，只有 `编辑`。
- footer 左侧为 `批量删除`，中间显示 `已选择 N 个`，右侧只有 `取消`。
- 没有管理器级 `新增规则`、`刷新`、逐行删除、pattern/replacement/regex 列、测试器或
  移动卡片重排。
- 未选择规则时点击批量删除提示 `请选择需要删除的替换规则`；确认文案为
  `确认要删除所选择的替换规则吗?`。
- 管理器关闭后选择态结束。编辑器是独立 sibling Dialog，管理器关闭不得顺带关闭直接由
  Reader 打开的编辑器。

### 编辑器

上游 `ReplaceRuleForm.vue` 也是根级 sibling Dialog：

- 标题始终为 `替换规则`，新增和编辑不改变标题。
- 桌面使用同一个 `dialogWidth/dialogTop`，mini interface 全屏。
- 字段标签依次为 `名称`、`规则`、`替换为`、`替换范围`。
- `使用正则表达式` 与 `是否启用` 都是 checkbox，不是带双状态文字的 switch。
- 不显示测试文本、测试结果或独立测试按钮。
- footer 文案固定为 `取 消`、`确 定`。
- 默认草稿为：

```json
{
  "name": "",
  "pattern": "",
  "replacement": "",
  "scope": "",
  "isRegex": false,
  "isEnabled": true
}
```

- 可见编辑器要求 name、pattern、scope 的 JavaScript 值非空；不得先 trim 后保存另一份值。
- 新增模式在当前规则数组中按**精确名称**检查重复；编辑模式不做前端重复名检查。
- 保存成功后刷新全局规则、关闭编辑器；取消不写 API、不广播、不刷新 Reader。

### 打开关系

- 设置页只打开管理器。
- 管理器的 `编辑` 打开 sibling 编辑器，管理器保留在其下。
- Reader 选中文字并选择过滤动作时，直接打开同一个编辑器，不打开管理器。
- 面板/Dialog 点击不得穿透到 Reader 正文、工具层或翻页区域。

## 3. 导入合同

上游只把非空 JSON 数组识别为导入文件，并在发送前确认
`确认要导入文件中的N条替换规则吗?`。数组按输入顺序原样交给批量保存：

- name 或 pattern 为精确空字符串的行由服务端跳过。
- 不得为缺失 name 的行伪造 `导入规则 N`。
- pattern/name/scope 的前导和尾随空白是规则数据，不得 trim。
- 缺失 `isRegex` 使用实体默认 `false`；缺失启用值使用 `true`。

OpenReader 可保留两项显式增强：

- 兼容顶层 `{rules:[...]}` 及旧 `title/regex/match/replace` 字段别名；
- 为缺失/null/空 scope 的外部导入写入显式 `*`，避免上游空 scope 在 Reader 中抛错并
  破坏后续规则。

增强不得改变以下结果：缺失 name 仍保持无效并计入 `skipped`；pattern 精确字节和数组顺序
保持不变；确认条数使用输入数组总行数，而不是客户端预过滤后的行数。

## 4. Reader 执行合同

### 范围与顺序

- 只处理普通文本章节；EPUB 与音频绕过全局替换规则。
- 按持久数组的插入位置顺序执行。编辑原规则不移动位置；新增规则追加到末尾。
- `order` 是兼容实体字段，但固定上游 Web Reader 不用它排序。OpenReader 等价顺序是
  SQLite `id ASC`，不得由 `sort_order` 或 `updated_at` 改变执行、列表或备份顺序。
- `isEnabled === false` 的规则跳过；缺失模式按 plain、缺失启用值按 enabled 处理。
- scope 使用 `split(";")`：
  - 第一段为精确 `*` 或精确书名；
  - 没有第二段时只按第一段匹配；
  - 存在第二段时必须精确等于书籍 URL；
  - `书名;` 的第二段是空字符串，不等于非空 URL，因此不得被视为“任意 URL”；
  - 不 trim scope、书名或 URL。
- 已部署 OpenReader 的空 scope 行可继续作为全局规则读取，作为不改写用户数据的唯一 shim；
  新建、编辑和导入不得再产生空 scope。

### 替换语义

上游使用 JavaScript：

```js
content.replace(rule.pattern, rule.replacement)
content.replace(new RegExp(rule.pattern, "ig"), rule.replacement)
```

因此：

- plain 只替换第一个匹配，大小写敏感；
- regex 全局、大小写不敏感；
- replacement 是 JavaScript replacement string，不是普通字面量。对受支持 pattern，
  `$$`、`$&`、``$` ``、`$'`、数字捕获和命名捕获必须与 JavaScript 结果一致；
- 一个非法 regex 会由包住整个循环的 `try/catch` 中止剩余 pipeline，不能降级为 plain；
- 普通章节加载只过滤正文，不应额外改写章节标题。

Go 标准 RE2 不支持 JavaScript 的全部回溯、lookaround 和反向引用 pattern。为避免用户规则
导致无界 CPU/内存，OpenReader 保留“写入前以 RE2 子集验证，非法/不支持表达式返回 400”
这一安全适配；不得宣称该 pattern 语言与浏览器 RegExp 完全等价。对 RE2 可接受的 pattern，
匹配大小写、全局性、顺序和 replacement-string 语义仍必须对齐。

### 选中文字

默认选择动作 `操作弹窗` 保持：

- 原始选中文字只用于判空，不折叠、不截短、不 trim 后另存。
- 过滤草稿名为 `文本替换 YYYY-MM-DD HH:mm:ss`。
- `pattern` 为原始选中文字，replacement 为空，plain、enabled。
- scope 为精确 `book.title + ";" + book.url`。
- 选择过滤仅打开共享编辑器；在用户点击 `确 定` 前不得持久化或广播。
- 选择书签、关闭操作弹窗的分支不创建替换规则。

## 5. REST/API 翻译合同

reader-dev 以 namespace JSON 和 `{isSuccess,data,errorMsg}` 返回；OpenReader 保留已部署的
JWT REST 路径与明确 HTTP 状态。

| 路径 | 必须保持的语义 | 允许适配 |
|---|---|---|
| `GET /api/replace-rules` | 当前用户；`id ASC`；同时输出 `enabled`/`isEnabled` | SQLite ID、JWT、`500` 读错误 |
| `POST /api/replace-rules` | 原样 name/pattern/scope；当前用户精确 name-upsert；更新不移动 ID | append `201`、replace `200`、字段大小限制、RE2 验证 |
| `PUT /api/replace-rules/:id` | 当前用户 ID 更新且不移动位置 | 重命名撞到另一现有名称返回 `409`，避免上游误覆盖另一规则 |
| `POST /api/replace-rules/batch` | 数组；只跳过精确空 name/pattern；按输入顺序精确 name-upsert | 单事务、计数响应、显式 scope/RE2 验证 |
| 删除路由 | 只删除当前用户选中的 ID，按请求顺序回报 `deletedIds` | 保留 single DELETE 供旧客户端，但不在对齐 UI 中暴露 |
| `/test` | 与真实 Reader 使用同一引擎且不持久化 | 保留为隐藏兼容 API，不在对齐编辑器中暴露 |

所有成功 mutation 只在 durable write 后广播 `replace_rules_update`。浏览器本地事件与 WebSocket
去抖刷新可保留，但旧账号/已关闭 manager 的迟到请求不得写入新会话 UI。

## 6. SQLite、备份与恢复合同

- 保留现有 `replace_rules` 表、ID、用户 ID、字段、时间戳和已添加的 `group_name/sort_order`；
  不做破坏迁移、不去重、不批量改写现有行。
- `sort_order` 继续持久化/导出用于旧客户端 round-trip，但不参与固定上游 Web Reader 的
  列表、执行或备份 pipeline。
- `replaceRule.json` 保持上游字段：
  `id,name,group,pattern,replacement,scope,isEnabled,isRegex,order`。
- `replaceRules.json` 作为 OpenReader 丰富别名继续保留；恢复 planner 同时存在时只执行较丰富
  artifact 一次。
- 备份按 `user_id,id`（单用户即 `id`）输出，不能按 `sort_order` 重排。
- 恢复按 archive 数组顺序处理，并以当前用户**精确 name**为 upsert 身份；更新既有最早 ID
  不移动其 pipeline，新增按 archive 顺序追加。
- 缺失/精确空 name 或 pattern 的恢复行跳过，不得用 pattern 伪造 name。
- 外部空 scope 规范为 `*`；missing `isRegex` 为 false，missing enabled 为 true。
- 当前数据库可能已有重复名称；启动和本批修复都不得删除或合并它们。新 name-upsert 只更新
  最早 ID，ID 编辑仍可逐条处理旧重复行。
- 恢复继续处于既有全备份事务、ZIP 大小/路径/条目限制和当前用户边界内；失败不得留下部分规则。

## 7. 当前差异矩阵

| 层 | 当前状态 | 判定 |
|---|---|---|
| 管理器 | 额外新增/刷新/逐行删除/测试信息和 pattern 等列；移动端另建 cards；标题错误 | `must-fix` |
| 编辑器 | 520px、动态标题、switch、测试器、`取消/保存` | `must-fix` |
| 导入 | 伪造缺失 name；trim pattern；客户端丢弃无 pattern 行后再计数 | `must-fix` |
| 保存 | 前后端 trim name/pattern/scope | `must-fix` |
| 顺序 | list/apply/backup 使用 `sort_order,id` | `must-fix` |
| scope | trim；把 `书名;` 当任意 URL | `must-fix` |
| restore | 以 pattern upsert并用 pattern 补 name | `must-fix` |
| replacement string | Go plain 字面 replacement、regex Go expansion，未证明 JavaScript token 等价 | `must-fix` |
| regex pattern | RE2 拒绝部分 JS pattern | `acceptable security adaptation`，必须在 UI/API 报错而非静默改变 |
| JWT/ID/事务/同步 | 当前用户隔离、稳定 ID、批量事务、post-write WebSocket | `acceptable technical adaptation` |
| legacy 空 scope | 作为全局读取 | `acceptable deployed-data shim`，不得扩展到新数据 |
| hidden single-delete/test API | 已部署但上游 UI 不暴露 | `deployed compatibility only` |

历史 `57b1dc0` 只把 manager shell 从 Drawer 改成 Dialog，未重建其内部可见结构；历史绿灯测试
继续断言当前额外 UI、trim 与 `sort_order` 行为，因此必须替换，不能作为本轮完成证据。

## 8. 测试先行与发布门

### 先失败的合同测试

1. 前端静态/组件合同证明管理器标题、五列表格、footer、唯一导入动作、共享桌面宽度和移动全屏；
   明确禁止卡片、管理器新增/刷新/逐行删除与编辑器测试器。
2. composable 合同证明导入不伪造 name、不 trim pattern/name/scope、按总行数确认并把无效行交给
   batch 计为 skipped。
3. API 合同证明精确空值与 whitespace-only 的区别、精确 name-upsert、`id ASC`、更新不移动、
   `书名;` 不匹配非空 URL、旧空 scope shim 和跨用户隔离。
4. Reader engine 合同覆盖 plain first、regex global/case-insensitive、pipeline 顺序、非法 regex
   中止、JavaScript replacement tokens、EPUB/audio bypass。
5. backup/restore 合同覆盖 `user_id,id` 输出、name identity、archive 顺序、无名行跳过、
   alias 单次执行、事务回滚和旧重复名称不丢失。

### 真实浏览器

在 `1440×900`、`1024×1366`、`390×844`、`360×800` 验证：

- 设置 → 替换规则管理器：表格结构、selection、toggle、import、batch delete、close；
- 编辑打开时 manager 保持，关闭 manager 不关闭 Reader 直接编辑器；
- Reader 原始选中文字 → 操作弹窗 → shared editor → cancel/save；
- 移动全屏不遮死关闭入口，任何 Dialog 点击不穿透工具层或翻页；
- mutation 后当前 Reader 只刷新一次，其他已登录同账号客户端同步，换号迟到响应被丢弃。

### 最终门

```bash
cd backend && go test ./...
cd frontend && npm test
cd frontend && npm run build
```

随后运行真实浏览器 smoke、普通与历史 mounted-volume/backup smoke。本模块可在 UI/API/数据完整
切片通过后本地构建并发布 Docker；发布报告必须列出完成项、RE2 安全差异、未完成项、标签、
digest 和验证矩阵。
