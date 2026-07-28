# Reader 文本位置语义选择器 P0 合同

状态：2026-07-28 完成固定上游与当前实现的只读复审。原总矩阵中“书源入口、`h3`
标题、长词断行尚未实现”的描述已经被后续代码推翻；实际剩余缺口是标题重构遗漏的一个
`h1[data-pos]` 位置选择器。应用代码尚未在本合同阶段修改。

固定上游：

- `changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`
- `web/src/components/Content.vue`
- `web/src/views/Reader.vue#showPosition`
- `web/src/views/Reader.vue#getCurrentParagraph/getPrevParagraph/getNextParagraph`

当前映射：

- `frontend/src/components/reader/ReaderChapterContent.vue`
- `frontend/src/composables/useReaderNavigation.js`
- `frontend/src/composables/useReaderPositionRestore.js`
- `frontend/src/composables/useReaderViewportProgress.js`
- `frontend/src/composables/useReaderSearchNavigation.js`
- `frontend/src/utils/readerTTS.js`
- `frontend/tests/readerUpstreamTypographyContract.test.mjs`
- `frontend/tests/readerNavigation.test.mjs`

## 审查矩阵

| 合同 | 固定上游 | 当前 OpenReader | 裁决 |
|---|---|---|---|
| 阅读内书源入口 | Reader 工具始终可打开 `BookSource`；可用性由面板流程决定。 | 桌面/移动按钮没有 `disabled`；`openSource()` 只拒绝无持久书籍身份的临时阅读，不再按本地/远程禁用。 | `aligned`；原总矩阵当前证据已过期。 |
| 章节标题 DOM | 普通、卷、连续章节使用 `h3[data-pos="0"]`。 | 三种文本分支均渲染 `h3[data-pos="0"]`，没有 Reader `h1`。 | `aligned`。 |
| 标题排版 | `28px / 1.2 / 1em 0 / center`。 | `ReaderChapterContent.vue` 完整保留相同值。 | `aligned`。 |
| 段落换行 | `word-wrap: break-word`、`word-break: break-all`、首行缩进 `2em`。 | 三项均存在。 | `aligned`；原总矩阵当前证据已过期。 |
| 书签、正文搜索、TTS、视口进度 | 上游统一遍历 `h3,p`，标题是字符位置零和段落序列的一部分。 | 对应 Reader、搜索、TTS、viewport progress 已使用 `h3`/`[data-reader-block]`。 | `aligned`。 |
| 已加载章节字符位置跳转 | 上游 `showPosition()` 在 `.reading-chapter h3,p` 中按 `data-pos` 定位。 | `paragraphByChapterPosition()` 仍查询旧的 `h1[data-pos]` 与正文块；该 `h1` 自 `32a1eaa` 后已不存在。 | `must-fix`：改为 `h3[data-pos]`，并把该 consumer 纳入统一静态与行为合同。 |

## 状态与兼容边界

- 旧链接、书签和进度继续使用现有 `chapter`、`offset`、`percent` 路由/数据字段。
- `offset <= 0` 仍表示章节边界，不额外进行字符段落定位。
- `offset > 0` 时，普通章节和已加载的连续章节必须从
  `h3[data-pos], [data-reader-block][data-pos]` 的有序序列中选择最后一个
  `data-pos <= offset` 的节点；没有可用节点时保留现有章节顶部回退。
- 本批不改变 API、SQLite、缓存、同步事件或 EPUB/音频/CBZ 分支。
- OpenReader 的 `[data-reader-block]` 是 Vue 3 内容白名单适配；它与上游 `p` 在该位置合同中
  等价。图片块仍可携带 `data-pos`，这是已有格式兼容能力，不属于偏差。

## 测试先行与发布门

1. 先扩展静态排版合同，要求所有文本位置 consumer 不再引用 `h1[data-pos]`；旧代码必须失败。
2. 在 `readerNavigation.test.mjs` 建立真实选择器行为：
   - 断言查询 `h3[data-pos], [data-reader-block][data-pos]`；
   - 字符位置落在标题后、段落之间和段落后时选择正确节点；
   - 已加载连续章节的 `goChapter(index, offset)` 使用该节点而不是退回章节顶部。
3. 实现只替换语义选择器，不改变位置算法。
4. 运行 frontend 全量、production build、Go 全量，以及文本/连续阅读真实浏览器合同。
5. 该修复可立即提交 Git；是否单独发布 Docker 由浏览器结果和下一 Reader 切片大小共同决定。
