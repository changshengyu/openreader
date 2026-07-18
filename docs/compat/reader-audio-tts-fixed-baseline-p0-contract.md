# Reader 音频与 TTS 固定基准合同（P0）

状态：**2026-07-18 已按固定基准完成合同、失败测试、实现和全量验证。历史“音频/TTS 已对齐”结论经重新审查后已由本合同替代；AUDIO-FIX-1…4、TTS-FIX-1…6 与 FORMAT-FIX-7 均已转绿。**

基准：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。

上游权威：

- `web/src/components/Content.vue`：`renderAudio()`、`play()`、`computeDuration()`、`prevChapter()`、`nextChapter()`、`onTimeupdate()`、`onEnd()` 及 `.content-audio`。
- `web/src/views/Reader.vue`：`showReadBar`、`speechAvalable`、`readBarTheme`、`startSpeech()`、`speechPrev()`、`speechNext()`、`getCurrentParagraph()`、`exitRead()` 和 `showParagraph()`。
- `web/src/plugins/config.js`：`speechVoiceConfig` 默认值和持久字段。

OpenReader 当前映射：

- 音频：`ReaderAudioContent.vue`、`ReaderChapterContent.vue`、`Reader.vue` 的 audio state/progress/navigation、`useReaderChapterLoader.js`。
- TTS：`ReaderTTSBar.vue`、`useReaderTTS.js`、`useTTS.js`、`readerTTS.js`、`useReaderMode.js`、`Reader.vue` 和 reader Pinia settings。
- 现有证据：`readerAudioContent.test.mjs`、`readerTTS.test.mjs`、`reader-audio-contract.mjs`、`reader-tts-contract.mjs`。

## 1. 音频状态与界面矩阵

| 关注点 | 固定上游行为 | OpenReader 当前行为 | 判定 / 必须动作 |
|---|---|---|---|
| 格式资格 | `readingBook.type === 1` 进入独立 audio 分支，不渲染普通章节；slide、自动阅读、TTS 和普通文本键鼠翻页失效。 | `format === audio`、`readerEffectiveMode()`、pointer/keyboard guards 和工具可见性做相同分流。 | `aligned technical equivalent`；保留三视口格式门禁。 |
| 媒体元素 | 原生 `audio` 只负责媒体能力，主界面使用自定义控制；初始化 load/metadata 后恢复 `startTime`。 | 隐藏 `audio preload=metadata`，自定义播放、seek、音量和恢复秒数。 | `aligned`；保留浏览器媒体元素，不恢复 native controls。 |
| 可见结构 | 顺序为大封面 → 时间/进度 → 五个操作 → 音量 → 含小封面、章节标题、书名和作者的底部 book-info。宽度 100%，封面最大 200px。 | 已删除自创 card/kicker，`ReaderAudioContent` 按同一顺序渲染并由 Reader 传入章节、书名、作者和封面。 | `aligned technical equivalent`；Vue 原生媒体和 CSS 是框架适配。 |
| 首末章操作 | 上一章/下一章图标始终可点击；Reader 在越界时提示“本章是第一章/最后一章”。 | 按钮保持可点击；`goAudioChapter()` 对首末章显示相同提示并不发送章节请求。 | `aligned`。 |
| 手动转章 | previous/next 先把全局 `autoPlay=true`，再切章。目标 metadata 可用后显式 `play()`。 | 切章先保留 `audioAutoplay`，目标 metadata 后调用真实 `play()`；只有 `play` 事件或可见的浏览器拒绝结果才清除 intent。 | `aligned`；浏览器限制首次自动播放时允许用户手动继续。 |
| 播放结束 | 清零当前媒体状态，置 autoplay，再进入下一章；书末由 Reader 边界规则结束。 | 先保存完成进度，再为非末章置 autoplay 并跳转；末章提示且停留。 | `aligned acceptable enhancement`；浏览器合同证明书末无越界请求。 |
| 秒级进度 | `startTime` 恢复到 `audio.currentTime`；`timeupdate` 发出保存，offset 语义是秒。 | Reader 用 offset 秒恢复，并节流保存 chapter/full-book percent。 | `aligned Go adaptation`；不得与文本 offset 混用。 |
| 音量/播放速率 | 音量 0–100，静音切到 0/100；`currentSpeed=1` 写入 playbackRate，没有可见倍速入口。 | 音量保持本组件会话状态；浏览器默认速率 1。 | `aligned`；不新增上游不存在的音频倍速 UI。 |
| 错误 | 媒体错误显示消息并退出 playing；页面不空白。 | 统一提示“音频加载失败，请检查书源或网络后重试”。 | `acceptable-change`；保留更稳定的用户文案并补重试/继续浏览断言。 |
| 资源安全 | 上游直接使用内容 URL。 | 本地/私有音频使用 user/book/purpose/expiry capability，GET/HEAD/Range 和 allow-list MIME。 | `acceptable-change security hardening`；本批不削弱 capability、范围请求或日志脱敏。 |

## 2. TTS 状态与界面矩阵

| 关注点 | 固定上游行为 | OpenReader 当前行为 | 判定 / 必须动作 |
|---|---|---|---|
| 支持检测 | 只有 `window.speechSynthesis && window.speechSynthesis.getVoices` 才显示入口。 | `readerSpeechSynthesisSupported()` 检查 `getVoices`；voiceschanged 事件 API 使用能力分支，残缺对象不会让 Reader setup 抛错。 | `aligned`。 |
| 可用格式 | EPUB、普通图片漫画、audio 隐藏；CBZ 保留。 | 共享 capability helper 做相同判定。 | `aligned`；保留 CBZ 例外。 |
| 打开/工具层 | read bar 打开不自动朗读；移动端 `showToolBar=false`，中心点击不能重新切换，关闭不自动重开工具层。 | `ttsBarRequested` 与播放态分离，并实现相同 mobile chrome 例外。 | `aligned`。 |
| 模式与留白 | read bar 使 slide 分支退出；展开/收起分别保留 280/80px，关闭后恢复原模式。 | `flip → page → flip` 与 280/80px 已实现；关闭前冻结 active 段，分栏重排完成后映射回对应页。 | `aligned`。 |
| 栏位几何 | 固定底部 0；桌面宽 500px 并与 Reader 工作区对齐；mini 为 `right:0;width:100vw`。 | 桌面栏贴底 500px 并对齐 Reader 右边界；移动栏 `left/right/bottom:0;width:100vw`。 | `aligned`。 |
| 栏位结构 | 关闭、上一段、播放/停止、下一段、展开；展开后是横向可滚动 voice radio buttons、语速、语调、定时。 | voice 使用横向按钮组；rate/pitch/sleep 使用用户要求的减号/可编辑数值/加号；progress 与 pause/resume 保留。 | `aligned + acceptable user-requested enhancement`。 |
| 语音选择 | `voiceName` 默认空；未显式选择或已选 voice 不存在时不启动朗读。 | 空或失效 voiceURI 不朗读并提示“请先选择语音库”；选择后按 voiceURI 持久化。 | `aligned acceptable browser adaptation`。 |
| 段落范围 | 只遍历 `h3,p`；无 active 时按 slide right 或顶部 `50 + webAppDistance + safeArea.top` 找第一段。 | 只遍历当前章节 `h3,p`；顶部判断使用 Reader 内容与正文渲染后的真实安全边界。 | `aligned technical equivalent`。 |
| 同章前后段 | 当前 `.reading` 优先，前后段停止旧 utterance、定位、标记并朗读。 | 静态 DOM list + currentIndex 完成同章切换并标记 `.reading/.tts-active`。 | `technical-stack-equivalent`；保留 cancellation token。 |
| 跨章 | 上游订阅一次 `showContent` 完成事件，再延迟 100ms 开始；不设总加载超时，失败由正常 Reader 错误流程处理。 | `useReaderChapterReady` 等待目标 scope/index 的 loaded、非 loading、无 error 状态；AbortController 取消旧事务，不设总超时。 | `aligned technical equivalent`。 |
| 自动续读 | utterance end 进入下一段；跨章后从第一段继续，上一段跨章从末段继续。 | 自动和手动前后段共用 cancellable token；4.1 秒延迟章节仍在 ready 后从正确段落继续。 | `aligned`。 |
| 参数更新 | voice/rate/pitch 改变时重启当前段；范围 rate 0.5–2、pitch 0–2、sleep 0–180。 | 范围与 restart 已实现。 | `aligned`；改控件不改存储范围。 |
| 关闭定位 | stop 后获取当前可见段落，隐藏栏，并在恢复 slide 时把该段映射到对应页。 | 关闭前冻结当前段；双帧分栏重排后按 rendered column geometry 定位，且不自动重开 mobile chrome。 | `aligned`。 |
| 错误与销毁 | utterance error 显示“朗读错误”；离开 Reader 取消语音。 | 错误提示、composable unmount cancel、scope 变化和显式 AbortController 均已实现。 | `aligned`；chapter-ready 单元合同覆盖失败、换 scope 和 abort 无残留 waiter。 |

## 3. 路由、状态与数据边界

- 不新增后端路由或 SQLite 列。音频继续复用章节 content API 和用户 progress 表，offset 对 audio 表示秒。
- TTS rate/pitch/voice 继续存入现有 reader setting；`voiceURI` 替代上游 `voiceName` 是允许的浏览器稳定适配。
- `goChapter()` 仍保持通用路由语义；TTS 额外通过 `useReaderChapterReady` 等待真实章节 DOM 状态，不改变其他导航调用方。
- 本批不改变 audio capability、远程抓取限制、书源 parser 或历史 data/cache/library 格式。
- 暂停/继续和段落进度标签可以保留，但不得改变上游的默认“打开栏不播放”、格式门禁、工具层例外和关闭定位。

## 4. 先失败的合同测试

| 编号 | 必须先失败的断言 | 层 |
|---|---|---|
| AUDIO-FIX-1 | 音频 DOM 顺序和可见字段包含大封面、时间进度、五操作、音量、小封面、章节标题、书名、作者；不存在自创 kicker/card-only 结构。 | component/browser |
| AUDIO-FIX-2 | 第一/末章按钮不 disabled；点击不越界请求并显示固定边界消息。 | component/Reader/browser |
| AUDIO-FIX-3 | 手动上一/下一章及 ended 后，目标 audio 在 metadata/ready 时真实调用 `play()`；intent 只在 play 或明确 rejection 后结束。 | state/browser |
| AUDIO-FIX-4 | offset 秒恢复、seek、±15、timeupdate save、末章 ended、媒体错误和 capability Range 回归通过。 | state/API/browser |
| TTS-FIX-1 | speechSynthesis 缺 getVoices/addEventListener 时 Reader 不抛错且不显示入口。 | composable/browser |
| TTS-FIX-2 | desktop 栏贴底 500px 并与工作区对齐；390/360 栏贴底全宽；voice 为可滚动选择，rate/pitch/sleep 使用可编辑 steppers。 | component/browser |
| TTS-FIX-3 | 未选 voice 不播放；选择、rate、pitch 持久化及当前段重启符合合同。 | state/browser |
| TTS-FIX-4 | 安全区顶部按真实边界选择 `h3,p`；没有 h1/h2 兼容借口。 | utility/browser |
| TTS-FIX-5 | 相邻章延迟超过 3.6 秒仍在 ready 后续读；加载失败、显式停止、换书和旧事务均不会错章或产生 unhandled rejection。 | controller/browser |
| TTS-FIX-6 | flip 中朗读到后续段落后关闭栏，恢复到该段所在页；mobile chrome 不被自动重开。 | pagination/browser |
| FORMAT-FIX-7 | 1440×900、390×844、360×800 同时验证 audio 与 TTS；真实/模拟媒体事件、console/pageerror、请求次数和可见几何均为门禁。 | browser |

## 5. 实施顺序与发布闸门

1. 先提交本合同，不修改应用代码。
2. 添加 AUDIO-FIX-1…3、TTS-FIX-1…6 的失败测试；删除仅检查元素存在或 shell class 的错误覆盖。
3. 先重建音频可见结构和 autoplay/boundary transaction，再重建 TTS 栏和 chapter-ready/退出定位事务。
4. 跑前端全量、Go 全量、生产 build，以及 mobile、text modes、continuous、audio、TTS、EPUB、CBZ 浏览器矩阵。
5. 这是 Reader P0 最后一个内容/朗读切片；达到半批可人工验证状态即可本地 Docker 发布，但必须继续通过历史 volume/portable backup。

## 6. 允许差异

- Vue 3/Pinia、内部滚动容器、voiceURI、TTS pause/resume、段落进度标签和安全 audio capability 可保留。
- 用户明确要求的所有数值设置使用“减号 / 可点击数值 / 加号”，因此 TTS 数值控件不恢复上游 slider。
- 浏览器 autoplay policy 可以要求首次用户手势，但手动切章/播放结束后的 intent、失败反馈和状态清理必须确定。
- 不借本批修改书源 parser、音频资源 API、EPUB/CBZ 或已签收的连续窗口策略。

## 7. 2026-07-18 实施与验证结果

- 前端自动测试：`444/444`；生产构建通过。
- Go：`go test ./...` 全量通过；未新增 API、数据库列或迁移。
- 音频浏览器合同：`1440×900`、`390×844`、`360×800` 全部通过，覆盖固定结构、首末章无越界请求、秒数恢复、seek、音量、真实 play、ended 续章和浏览器拒绝 autoplay 后的可见手动恢复。
- TTS 浏览器合同：三个标准视口全部通过，覆盖残缺 speech API、显式 voice、数值 steppers、贴底几何、参数持久化、4.1 秒延迟跨章、错误提示、工具层例外和 flip 关闭定位。
- Reader 回归：mobile、text modes、continuous 全部通过；真实 Go 服务上的 EPUB 与 CBZ 导入/阅读三视口合同通过。
- 共享分栏定位改为使用 rendered column geometry；这是上游 `showParagraph()` 的 Vue 等价实现，同时修复 TTS、书签和正文搜索跳到分栏段落时 `offsetLeft` 不可靠的问题。
- 源码 commit `5260efd083dea8002bed491eed7c2cc039d6cf0f` 已在本机构建 `linux/amd64` 与 `linux/arm64`，发布为 `ghcr.io/changshengyu/openreader:5260efd` 和 `latest`。两 tag 的 OCI index digest 均为 `sha256:5c8c7d9ab186ec80b26ed709123c1c88fe37261f18cf937988c4ffbfdc9a4df4`；amd64 manifest 为 `sha256:dd30334f4e301d283980d0f9cb1877e984218f47602dec1987755a54ebd755bf`，arm64 manifest 为 `sha256:30376bc19ca4941bc0a2cf56ce0430363ad69bee7d6132c5e13f23f46163a9f6`。
- 当前镜像的历史 volume/portable backup 脚本因 Codex 自动授权额度拒绝 OrbStack socket 而未能重复执行。此项不记为“当次通过”；兼容判断继承 `370d0f7` 已通过的旧 TXT、EPUB、UMD、CBZ、相对 cache、owner isolation 和 portable restore 证据，并额外确认 `370d0f7..5260efd` 的 backend、Dockerfile、volume/backup 脚本均为零差异。
