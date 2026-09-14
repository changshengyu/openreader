# Reader 左上角当前章节标注真机反馈合同（P0）

状态：**implemented / regression-validated / Docker-published / awaiting-device-verification**。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
当前基线：`OpenReader@919b588`。

## 上游证据与当前差异

- 上游 `web/src/views/Reader.vue` 在 `.chapter > .top-bar` 渲染 mini interface 的当前 `title`。
- mini CSS 将 top bar 固定在左上角，宽 `100vw`、高 `30px + safe-area-inset-top`、左右 padding 16px、
  字号 12px；正文从其后开始。slide 模式保留同一 30px 顶部区，但 top bar 改为相对定位。
- 当前 OpenReader 桌面 `.reader-page-head` 左侧显示书名、右侧只显示 `current/total`；mini interface
  又直接 `display:none`，因此手机阅读缺少上游的当前章节名。

## 目标合同

1. 文本和 EPUB Reader 顶部均使用同一派生 label：`第 N 章 章节名`，其中 N
   是当前目录的 1-based 位置，章节名来自当前可见/当前激活 Chapter；标题为空时只显示 `第 N 章`。
2. mini interface 恢复上游左上固定章节栏，使用当前主题背景/文字色、12px、单行省略、30px + safe
   area 和 16px 左右边距。它不接收点击，不遮挡工具栏/面板，不因长标题撑高或横向溢出。
3. scroll/scroll2 连续模式以 viewport 已更新的 `currentIndex` 切换 label；点击翻页、目录跳转、进度恢复、
   EPUB hash 跳转也在章节事务完成后更新。加载中的旧请求不能回写标题。
4. flip 模式保留固定格式顶部区与正文 `top: 30px + safe area`；普通滚动正文已有同等 top offset，不能
   再叠加造成额外空白。
5. 桌面 header 左侧改为同一章节 label，右侧保留 `N / total`；书名仍由浏览器标题、工具层和书架面板
   提供，不再占用章节运行标注的位置。
6. 音频沿用上游隐藏 top/bottom bar 的例外；CBZ 若当前设计为全幅漫画，也不新增遮图栏。允许差异是
   用户明确要求在上游章节名基础上补充 1-based 章节序号。

## 测试门

1. 单元合同覆盖首章、中间章、空标题、长标题、连续 viewport 切章及迟到请求。
2. CSS 合同覆盖 fixed/relative、safe area、16px、单行省略、pointer-events 和正文 offset。
3. 1440x900、390x844、360x800、1024x1366 验证顶部标注、长标题、夜间/自定义背景、工具层与四个
   主面板无重叠；scroll/scroll2/flip 切章时 label 正确。

本切片不增加浏览器存储 key、API、数据库字段或持久文件。

## 实施与验证（2026-09-14）

- `e1631d0` 将桌面 header 左侧从书名改为当前 `第 N 章 章节名`，右侧继续显示 `N / total`；移动 mini
  恢复上游 30px + safe-area、16px 边距、12px 固定章节栏。长标题单行省略且 `pointer-events:none`。
- scroll/scroll2 使用 viewport 已提交的 `currentIndex` 和 Chapter title，因此跨章滚动同步更新；flip
  保持相对顶部区。音频与全幅 CBZ 继续隐藏该栏。
- frontend 753/753 与 Vite build 通过；1440x900、1024x1366、390x844、360x800 的 scroll/scroll2
  浏览器合同验证了初始文字、跨章切换、fixed/absolute 几何、无溢出和不拦截点击，原移动/面板合同通过。
- 可信 GitHub Actions run `34794997078` 已发布 `e1631d0`/`latest` 双架构镜像；用户真机显示效果仍待签收。
