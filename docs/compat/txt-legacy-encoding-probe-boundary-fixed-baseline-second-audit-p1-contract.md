# 本地 TXT 旧编码探测边界合同（P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前实施基线：`origin/main` 为 `1b8ef35`。

审查日期：2026-10-25
状态：**aligned / Docker-published / awaiting-device-verification**（E1；证据见第 8 节，发布见第 9 节；E2/E4/E5 未处理）

来源：用户报告。`前浪(校对加料版全本 1-425) 作者：刀一耕.txt`（4,117,155 字节，GBK）在导入预检里显示“待修复”和
`unable to detect text encoding`，同一文件在上游能正常导入。2026-07-11 签收的 TXT 目录规则合同
（[`reader-dev-openreader-gap-analysis.md`](reader-dev-openreader-gap-analysis.md)“Immediate parser contract: TXT local import
catalog rules”）里 512000 字节探测窗口、目录规则选择、无目录伪章节等结论都不重开。

## 1. 范围与结论

OpenReader 从文件开头截取 512000 字节，用来判定编码。截取位置只对合法 UTF-8 文件按字符边界回退；GBK、GB18030、Big5
等旧编码文件直接在第 512000 字节处截断。截断点落在一个双字节（或 GB18030 四字节）字符中间时，`golang.org/x/text` 的
解码器把样本末尾不完整的字节当作输入结束，输出一个 U+FFFD；`detectAndDecodeText` 只要看到 U+FFFD 就拒绝这个候选编码。
于是每个旧编码候选都被拒绝，导入返回 `unable to detect text encoding`。截断点落在字符中间的概率约为一半（中文正文里大多数
字节属于双字节字符），所以超过 512000 字节的中文旧编码 TXT 大约有一半会这样失败。

上游的编码判定不会因为截断位置失败：ICU `CharsetDetector` 最多只看样本的前 8000 字节，并且容许被截断的末尾字符和少量
非法字节。

裁决：**must-fix（P1，本地导入失败）**。旧编码候选在截断样本上按“未到输入末尾”解码：样本末尾不完整的字符属于文件的后续
部分，不算作该编码的错误。其余检测规则、全量解码与解析结果不变。

## 2. 固定上游

| 位置 | 行为 |
|---|---|
| `src/main/java/io/legado/app/model/localBook/TextFile.kt#getChapterList`（L64–88） | `bis.read(buffer)` 读最多 `bufferSize = 512000` 字节（L50、L67–68）；`book.charset` 为空时调用 `EncodingDetect.getEncode(buffer.copyOf(length))`（L69–71）；随后用 `String(buffer, 0, length, charset)` 解码同一段字节来选目录规则（L73–76）。 |
| `src/main/java/io/legado/app/utils/EncodingDetect.kt#getEncode(bytes)`（L54–57） | `CharsetDetector().setText(bytes).detect()`，没有匹配时回退 `"UTF-8"`，从不报错。 |
| `src/main/java/io/legado/app/lib/icu4j/CharsetDetector.java` | `kBufSize = 8000`（L96）；`fStripTags` 默认 `false`（L438–439）；`MungeInput`（L331 起）只把前 `min(length, 8000)` 字节复制给识别器（L373–384）。所以编码判定实际只看前 8000 字节。 |
| `src/main/java/io/legado/app/lib/icu4j/CharsetRecog_mbcs.java#match`（L49 起） | 逐字符计数 `badCharCount`；`badCharCount >= 2 && badCharCount * 5 >= doubleByteCharCount` 时提前放弃（L80），`doubleByteCharCount < 20 * badCharCount` 时置信度为 0（L106）。也就是非法字符不超过双字节字符的约 5% 时，该编码仍可胜出。 |
| `CharsetRecog_gb_18030#nextChar`（L457 起，Big5 L245、Shift_JIS L185、EUC-KR L420 同理） | 样本末尾只有前导字节、没有后续字节时，记一个错误字符并结束。被截断的末尾字符至多算一个 `badCharCount`，不会让 GB18030/Big5 等编码落选。 |
| `TextFile.kt#analyze`（L105–135） | 正文按 512000 字节块读，每块在最后一个 `0x0a`（`blank`，L47）处切开，再用 `String(buffer, 0, end, charset)` 解码。Java 的这个构造方法把非法序列替换为 U+FFFD，不抛异常。 |

结论：上游的编码判定只看前 8000 字节，被截断的字符至多算一个错误，判定从不失败，所以截断位置不影响导入。

## 3. 当前实现

| 位置 | 行为 |
|---|---|
| `backend/engine/txt_parser.go#decodeTXTForCatalog`（L509–528） | 文件长于 512000 字节时截取 `data[:512000]`（L511–512）；只有整个文件是合法 UTF-8 时才把样本回退到字符边界（L513–517）。旧编码文件的截断不对齐字符。 |
| `backend/engine/text_encoding.go#detectAndDecodeText`（L32–86） | 依次尝试 `gb18030`、`big5`、`shift_jis`、`euc-kr`（L60–65），用 `NewDecoder().Bytes` 解码，把样本末尾当作输入结束；解码结果含 U+FFFD 即跳过该候选（L71）；全部跳过时返回 `unable to detect text encoding`（L83）。 |
| `golang.org/x/text@v0.42.0/encoding/simplifiedchinese/gbk.go#gbkDecoder.Transform` | 双字节字符缺后续字节时：未到末尾返回 `transform.ErrShortSrc`（L65–66），到末尾输出 `utf8.RuneError`（L69）；四字节字符同理（L80–86）。Big5、Shift_JIS、EUC-KR 的解码器行为相同。 |
| `decodeTextWithEncoding` | 选定编码后解码完整文件；完整文件里没有被截断的字符，这一步本身没有问题。 |

所有 TXT 入口都经过 `ParseTXTWithLimits` → `decodeTXTForCatalog`：直接上传的预览/导入、LocalStore/WebDAV 预检（“待修复”）/
重新解析/确认、`refreshLocalBook` 与丢失缓存后的重建、`cmd/reparse-local-book`。

### 3.1 用户文件

- 第 511999 字节是 `0xCC`，第 512000 字节（已在样本之外）是 `0xA7`，两者合起来是 GBK 的“抬”（“孔泉抬手”）。
- 整个文件按 GBK 和 GB18030 严格解码都没有错误（`iconv -f GBK`、`iconv -f GB18030` 均成功），共 2,088,820 字符、
  48,945 行。
- 在 `1b8ef35` 上临时调用 `engine.ParseTXT`：`bytes=4117155 chapters=0 err=unable to detect text encoding`。

### 3.2 合成夹具实验（`1b8ef35`，临时测试，未提交）

每种编码一行正文重复到约 600000 字节，开头加 0–3 个空格来移动截断位置；`tail` 是截断后留在样本外、属于同一字符的字节数。

| 编码 | tail=0 | tail≥1 |
|---|---|---|
| GBK 双字节（GB18030 解码器） | 判定 `gb18030`，解析成功 | `unable to detect text encoding`（tail=1） |
| GB18030 四字节（`𠀀𠀁𠀂`） | 判定 `gb18030`，解析成功 | `unable to detect text encoding`（tail=1、2、3） |
| Big5 | 判定 `big5`，解析成功 | `unable to detect text encoding`（tail=1） |
| Shift_JIS | 判定为 `gb18030`（乱码） | `unable to detect text encoding` |
| EUC-KR | 判定为 `gb18030`（乱码） | `unable to detect text encoding` |

## 4. 差异与裁决

| ID | 差异 | 裁决 |
|---|---|---|
| E1 | 截断样本末尾被拆开的多字节字符让所有旧编码候选落选，导入失败；上游不受截断位置影响。 | **must-fix（本切片）** |
| E2 | 样本内部真正存在的少量非法字节序列：上游在约 5% 以内仍判定该编码并以 U+FFFD 解码；当前只要有一个 U+FFFD 就拒绝，最终报错。 | **must-fix，另起切片**。需要同时设计损坏 UTF-8 与旧编码的容错先后，否则带个别坏字节的 UTF-8 文件会被判成 GB18030 乱码。本切片不改。 |
| E3 | 编码判定样本：上游只看前 8000 字节，当前看前 512000 字节。 | `acceptable-change`：样本更大只增加证据（例如前 8000 字节全是 ASCII 时上游会判错）。不重开。 |
| E4 | Shift_JIS、EUC-KR：截断对齐时也被当前评分判成 `gb18030`；上游 ICU 有各自的常用字识别。 | `unknown`，需另行盘点。本切片的修复让这两种编码在截断时与对齐时结果一致，不改它们的评分。 |
| E5 | 没有任何候选时，上游回退 UTF-8 继续导入，当前返回错误。 | `unknown`：对无法解码的文件，明确报错与按 UTF-8 导入乱码哪个更合适，与 E2 一起另起切片判定。本切片不改。 |
| E6 | 用于选目录规则的样本文本：上游含被截断字符的一个 U+FFFD，修复后 OpenReader 不含这个残缺字符。 | `technical-stack-equivalent`：只差样本末尾一个字符，不影响规则匹配计数。 |

## 5. 目标行为（E1）

1. `decodeTXTForCatalog` 截取了样本（文件长于 512000 字节）时，把样本当作更长文档的前缀：旧编码候选用
   `Transform(..., atEOF=false)` 解码，样本末尾不完整的字符不解码、不产生 U+FFFD、不算该编码的错误。
2. 样本内部的非法序列仍按现有规则拒绝该候选（E2 不在本切片）。
3. 不截取的完整输入（不超过 512000 字节的文件，以及 `DecodeBody` 的 HTTP 正文）仍按完整输入处理，行为不变。
4. UTF-8 回退对齐、BOM、UTF-16/32 分支和候选评分都不变。
5. 选出的编码名、全量解码、章节、正文缓存与 archive 格式不变。已导入的书不迁移、不重新解析；此前因此失败的导入没有写入
   书架数据，用户重新导入即可。

## 6. 计划测试

红测（在 `1b8ef35` 上必须失败）：

1. engine：GBK 双字节、GB18030 四字节、Big5 三种夹具，都超过 512000 字节，且测试先断言夹具确实在样本边界拆开一个字符
   （不满足时测试自身失败，避免夹具漂移后静默通过）。夹具带 `第N章` 标题，断言 `ParseTXT` 成功、章节标题正确、拼接正文
   等于原文，并包含位于样本边界之后的标记。
2. api：用同类 GBK 夹具走直接上传预览 → token 导入 → 删除章节缓存 → 正文重建；LocalStore 与 WebDAV 预检（不再返回条目
   错误）→ 删除挂载源 → 确认导入 → 删除章节缓存 → 正文重建。

正常对照（修复前后都通过）：

3. 同样的夹具在截断对齐（tail=0）时解析成功，证明红测只来自截断位置。
4. 超过 512000 字节、截断落在 UTF-8 三字节字符中间的合法 UTF-8 文件解析成功（现有 UTF-8 回退对齐）。
5. 现有编码、无目录、512 KiB 探测窗口、GB18030/GBK/BOM 导入与存储导入测试保持通过。

## 7. 不重开

- 512000 字节探测窗口、目录规则选择、前言、无目录 10 KiB 伪章节、显式规则空目录（2026-07-11 签收）。
- 编码候选顺序与评分、UTF-16 无 BOM 判定、`DecodeBody` 的 HTML charset 检测。
- 导入暂存、token、LocalStore/WebDAV 流程与 parser 预算（[`local-text-parser-budget-p2-contract.md`](local-text-parser-budget-p2-contract.md)）。
- 前端导入流程不改：`useStorageImportWorkflow.js#chooseInitialPhase` 只在有失败行时显示预检列表（“待修复”）；全部预检
  成功时直接进入确认，单本即“导入本地书籍”；混合批次里成功的行显示“可导入”。
- SQLite、`data/`、`cache/`、`library/`、备份与已导入书籍。

## 8. 记录（2026-10-25）

分支 `claude/txt-legacy-probe-boundary`，基于 `1b8ef35`：

| 提交 | 内容 |
|---|---|
| `364aa82` docs | 本合同、gap-analysis 与矩阵条目。 |
| `8f001c8` test | engine `TestParseTXTDetectsEncodingWhereverTheCharsetProbeEnds`；api `TestTXTSplitAtCharsetProbeImportsThroughEveryLocalEntry`。 |
| `f84398d` fix | `text_encoding.go` 新增 `detectAndDecodeTextSample(data, atEOF)` 与 `decodeLegacyText`；`decodeTXTForCatalog` 截取样本时传 `atEOF=false`。`detectAndDecodeText`（`DecodeBody` 的完整 HTTP 正文）仍为 `atEOF=true`。 |

红测（`8f001c8`，应用代码为 `1b8ef35`）：

- engine：截断拆开字符的三项失败，都是 `unable to detect text encoding`：GBK 双字节（样本保留该字符 1 字节）、GB18030
  四字节（保留 2 字节）、Big5（保留 1 字节）。五项对照通过：三种编码截断对齐，以及 UTF-8 截断拆字与对齐。
- api：直接上传预览返回 `400 failed to parse local book: unable to detect text encoding`；LocalStore、WebDAV 预检条目带
  同一错误且没有 book，即界面上的“待修复”。
- 完整 engine 与 api 包只有这六个子测试失败，`services/localbook` 通过。含 httptest 监听的用例在沙箱外运行（沙箱禁止监听本地端口）。

实现后（`f84398d`）：

- 同一六个子测试转绿，五项对照仍通过；既有 `TestDirectTXTEncodingAndLongRuleCatalogRemainReadableAcrossStageImport`
  （UTF-8 BOM、GBK、GB18030、长规则章节）与 `TestStorageGB18030NoTocStageImportRemainsReadable` 通过。
- 变异 M1：把 `decodeTXTForCatalog` 的调用改回 `atEOF=true`，恰好这六个子测试重新失败；已还原。
- 用户文件：`engine.ParseTXT` 得到 455 章（`前言`、`序章 如果有来生`、`第001章 有来生` … `第425章 春节档`、`终章 盗梦`），
  解码全文的 SHA-256 与 `iconv -f GB18030 -t UTF-8` 的输出一致（`d2cb6d97…3057`）。
- 真实浏览器（1440×900；`f84398d` 构建的二进制加生产前端，全新数据卷，书仓里放该文件的副本）：书仓“加入书架”和勾选后
  “批量加入书架”都直接进入“导入本地书籍”，章节列表 455；确定导入后提示“导入 1 本”。Reader 打开 `前言` 和样本截断处所在的
  `第056章 担心`（索引 58），正文含“孔泉抬手、做投降状，片刻后，说：”，U+FFFD 为 0。

回归：

- backend `go test ./...` 29 个包全部 ok，`go vet ./...` 通过；`go test -race -timeout 40m ./engine ./services/localbook ./api` 全部 ok（api 1018.8s），没有 data race。
- frontend 按 lock `npm ci`（vite 8.1.6）后 `npm test` 815/815（沙箱外；沙箱内 3 条 advisory worker 用例因禁止监听
  127.0.0.1 失败），`npm run build` 通过。
- advisory 门自测（Go、前端、OS）131/131；Go 与前端实际扫描均 exit 0。
- `docker compose config --quiet` 通过；`store-read-list-lifecycle-contract.mjs` 全部 PASS；
  `release-browser-smoke.test.mjs` 10/10。
- 未在本地运行：三视口浏览器（前端未改）；native/arm64 镜像、浏览器启动门、OS APK 门、fresh/historical 卷门与双平台发布
  由合入 main 后的可信 Actions 运行。

E2、E4、E5 未处理，见第 4 节。Git：分支已推送并经 `git ls-remote` 核验。镜像：未构建。生产：未访问。

## 9. 发布（2026-10-25）

`main` 由 `1b8ef35` 快进到 `94c696f`（本切片的四个提交，没有 force push）。可信 Actions run `40174734994` 全部步骤成功：
Go 测试、Go/前端/OS advisory 门、前端测试与构建、Compose、存储 HTTP 合同、native 候选镜像与浏览器启动门、arm64 候选、
OS APK 门、fresh/historical 卷门、双平台发布与平台核验。

- `ghcr.io/changshengyu/openreader:94c696f` 与 `latest`：OCI index
  `sha256:50cd42f1fb17df94fae00b42253cfdc88ad4ca0b935e45ba5ea93215f60bc62d`。
- `linux/amd64` manifest `sha256:3699aaea397b2335a833a404d7a397e4574e3d9e841c0b5623b0bdcb124135f8`；
  `linux/arm64` manifest `sha256:73ae128eb07e7a6f070e8ffa2ae553c4fa56da927f783da1c399b4c3a92643fe`。
- 两个平台的 `org.opencontainers.image.revision` 都是 `94c696ffb4ec7c9a3757f88349aa5df54dc79a74`。

本记录提交只改文档，不触发新镜像。生产在另一台 Mac，未访问，实际运行版本未知。升级到该镜像后重新导入此前“待修复”的书即可：
失败的预检没有写入书架数据，不需要迁移。
