# 本地 TXT 旧编码探测边界合同（P1）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前实施基线：`origin/main` 为 `1b8ef35`。

审查日期：2026-10-25
状态：**contract-only**

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
- 前端预检对话框的呈现：后端预检成功后，“待修复”自然变为“可导入”，前端不改。
- SQLite、`data/`、`cache/`、`library/`、备份与已导入书籍。
