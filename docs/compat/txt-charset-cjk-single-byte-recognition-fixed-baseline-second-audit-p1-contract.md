# 文本编码识别：日文/韩文与单字节编码合同（P1，E4/E8）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前实施基线：`origin/main` 为 `5a2fc7d`（应用代码与已发布的 `7c447a5` 相同）。

审查日期：2026-10-25
状态：**implementation-complete / regression-validated**（证据见第 10 节；尚未经可信 Actions 运行）

来源：[`txt-charset-malformed-tolerance-fixed-baseline-second-audit-p1-contract.md`](txt-charset-malformed-tolerance-fixed-baseline-second-audit-p1-contract.md)
（下称 E2 合同）第 5 节的 E4、E8。用户 2026-10-25 要求：开始下一个编码切片，E4（Shift_JIS/EUC-KR 被评分判成 GB18030）和 E8
（Latin-1 等单字节编码），同样先用上游 ICU 实测、写合同和红测。E1、E2 已签收的行为不重开。

## 1. 范围与结论

OpenReader 只有 `gb18030`、`big5`、`shift_jis`、`euc-kr` 四个旧编码候选，用自写的评分挑选：汉字（主区段）每字加分，简体/
繁体常用字再加分，假名、谚文只在对应候选里加分。日文、韩文文本的字节在 GBK 里也几乎都是合法双字节，解出来全是汉字，得分
反而最高，于是 Shift_JIS 日文、EUC-KR 韩文被解成 GB18030 乱码，EUC-JP 日文被解成 Big5 乱码；ISO-2022-JP 是 7 位字节，被当成
UTF-8 原样显示转义码；没有任何单字节编码候选，Latin-1/windows-1252 英文、法文、德文、俄文、希腊文、土耳其文、希伯来文、
阿拉伯文要么报 `unable to detect text encoding`，要么被解成 GB18030/Big5/Shift_JIS 乱码。

上游用 ICU 识别器：多字节编码按“常用字命中”给置信度，单字节编码按语言三字母组（n-gram）命中率给置信度，取最高者。用上游
代码实测（第 3 节），只要文本不太短，这套方法对日文、韩文、各单字节语言都判对；但文本很短时（约 10 个字以内）它很不可靠：
短的 GBK 中文被判成 Big5，短的 Big5 被判成 ISO-8859-1，甚至 OpenReader 现有测试里 13 个字的 GB18030 文本也被判成 EUC-JP。

裁决：**must-fix（P1，日文/韩文/西文等书籍导入与书源正文解码错误）**。移植上游 ICU 的多字节、ISO-2022 与单字节识别器，
在有足够证据时按上游的置信度选择编码；证据不足（很短或无统计依据）时保持 OpenReader 现有结果，避免把短中文判成乱码。

## 2. 固定上游

均在 `src/main/java/io/legado/app/lib/icu4j/`（文件头为 Unicode/ICU 许可声明）。

| 位置 | 行为 |
|---|---|
| `CharsetDetector.java` 识别器列表（L463–494） | 依次为 UTF-8、UTF-16BE/LE、UTF-32BE/LE、Shift_JIS、ISO-2022-JP/CN/KR、GB18030、EUC-JP、EUC-KR、Big5、ISO-8859-1、ISO-8859-2、ISO-8859-5（俄）、ISO-8859-6（阿）、ISO-8859-7（希）、ISO-8859-8-I（希伯来）、ISO-8859-8（希伯来）、windows-1251、windows-1256、KOI8-R、ISO-8859-9（土）；IBM424/IBM420 默认关闭。 |
| 同上 `#detectAll`（L181–203） | 置信度大于 0 的结果升序排序后反转：高者优先，相同时列表中靠后的识别器优先。 |
| 同上 `#MungeInput`（L331 起） | 前 `min(length, 8000)` 字节进 `fInputBytes`，并记录其中是否有 C1 字节（0x80–0x9F）`fC1Bytes`。 |
| `CharsetRecog_mbcs.java#match`（L49–131） | 多字节识别器逐字遍历 `fRawInput` 全部字节（L157–163）：坏字符 `badCharCount`、多字节字符 `doubleByteCharCount`、常用字命中 `commonCharCount`。扫描中 `bad >= 2 && bad * 5 >= dbcs` 即为 0；`dbcs <= 10 && bad == 0` 时为 10（“数据太少，但不排斥”，`dbcs == 0 && total < 10` 时为 0）；`dbcs < 20 * bad` 为 0；否则 `(int)(log(common + 1) * 90 / log((float) dbcs / 4) + 10)`，最高 100。 |
| 同上 `CharsetRecog_sjis`（L185 起）、`big5`（L245 起）、`euc`（`euc_jp`/`euc_kr`，L313 起）、`gb_18030`（L457 起） | 各自的取字规则与约 100 个常用字码表（按码值排序，二分查找）。GB18030 的第二字节判断写作 `secondByte >= 80`（十进制 80，即 0x50），照此移植。 |
| `CharsetRecog_2022.java#match`（L20 起） | 只看 `fInputBytes`：按各自的转义序列表统计命中、未命中与 SO/SI，置信度 `(100*hits - 100*misses)/(hits+misses)`，命中加 SO/SI 少于 5 次时每少一次减 10。 |
| `CharsetRecog_sbcs.java#NGramParser`（L25–158） | 只看 `fInputBytes`：每个字节经各编码的 `byteMap` 归一（字母小写化，其余多为空格，0 表示忽略），连续空格只算一次；每加一个字节查一次三字节组（64 项有序表），末尾补一个空格；命中率大于 0.33 记 98，否则 `(int)(命中率 * 300)`。 |
| 同上各识别器（L282–1046） | ISO-8859-1 用 da/de/en/es/fr/it/nl/no/pt/sv 十种语言、ISO-8859-2 用 cs/hu/pl/ro 四种语言，取最高；ISO-8859-1/2/7/8-I/8/9 在 `fC1Bytes` 为真时分别改名 windows-1252/1250/1253/1255/1255/1254；ISO-8859-5、ISO-8859-6、windows-1251、windows-1256、KOI8-R 名称固定。 |
| `EncodingDetect.kt#getEncode`（L54–57）、`TextFile.kt#getChapterList`（L69–74）、`Book.kt#fileCharset`（L121–123） | 取 `detect()` 第一名，没有匹配为 `"UTF-8"`；随后 `Charset.forName(名称)` 解码。 |

## 3. 上游实测

方法同 E2 合同第 3 节：在 `eclipse-temurin:21-jdk` 里编译上游 8 个 ICU 源文件，照 `TextFile` 取前 512000 字节调用
`detectAll()`/`detect()`，再用 `new String(bytes, charset)` 解码整份文件。夹具是自写的日文、韩文、英文、法文、德文、西班牙文、
波兰文、俄文、希腊文、土耳其文、希伯来文、阿拉伯文段落，以及简体/繁体中文段落，按一段、约 3 KB、约 100 KB 三档重复，再用
Python 对应编解码器编码；另取用户 GBK 小说 4 个位置各 8 种长度（1 KB–512 KB）的片段；同一批输入也在 `5a2fc7d` 上跑了 OpenReader。

### 3.1 一段及更长的文本

| 文本 | 编码 | 上游结果（置信度） | OpenReader `5a2fc7d` |
|---|---|---|---|
| 日文 | Shift_JIS | Shift_JIS（100；其余 ≤ 10） | `gb18030` 乱码 |
| 日文 | EUC-JP | EUC-JP（100；GB18030 73–89，Big5 70–88） | `big5` 乱码 |
| 日文 | ISO-2022-JP | ISO-2022-JP（70–100；单字节 ≤ 19，UTF-8 15） | `utf-8`，显示转义码 |
| 韩文 | EUC-KR | EUC-KR（100；GB18030 30–70） | `gb18030` 乱码 |
| 英文（含弯引号、破折号） | windows-1252 | windows-1252（79–80） | 报错 |
| 英文 | Latin-1 | ISO-8859-1（80–82） | 报错 |
| 法文 | windows-1252 / Latin-1 | windows-1252（71–73）/ ISO-8859-1（74–76） | 报错 |
| 德文、西班牙文 | windows-1252 / Latin-1 | ISO-8859-1（81–87；文本无 C1 字节） | 德文 `gb18030` 乱码；西班牙文报错 |
| 波兰文 | windows-1250 / ISO-8859-2 | windows-1250 / ISO-8859-2（53–54） | 报错 |
| 俄文 | windows-1251 / KOI8-R / ISO-8859-5 | 各自（34–35） | 报错；KOI8-R 为 `shift_jis` 半角片假名乱码 |
| 希腊文 | windows-1253 / ISO-8859-7 | ISO-8859-7（57–58；文本无 C1 字节） | 报错 |
| 土耳其文 | windows-1254 / ISO-8859-9 | ISO-8859-9（48–50） | `big5` 乱码 |
| 希伯来文 | windows-1255 / ISO-8859-8 | ISO-8859-8-I（50–53），随后 `Charset.forName` 抛 `UnsupportedCharsetException`，**上游导入失败** | 报错 |
| 阿拉伯文 | windows-1256 / ISO-8859-6 | 各自（67–68） | 报错 |
| 用户 GBK 小说 32 个片段 | GBK | 全部 GB18030（100）；EUC-JP 74–92，EUC-KR 54–81，Big5 30–73 | 全部 `gb18030` |

### 3.2 短文本

同一段落截前 4/8/12/16/24/32/48 个字符，以及 OpenReader 现有测试里的短文本：

| 输入 | 上游结果 | OpenReader `5a2fc7d` |
|---|---|---|
| GBK 中文 4、8 字 | **Big5**（多字节全为 10，列表靠后者胜） | `gb18030` |
| GBK 中文 12–48 字 | GB18030（100） | `gb18030` |
| Big5 中文 4、8 字 | **ISO-8859-1**（33–35） | `big5` |
| Big5 中文 12–48 字 | Big5（100；ISO-8859-1 19–28） | `big5` |
| Shift_JIS 日文 4 / 8 字 | **Big5**（10）/ **windows-1255**（50） | `gb18030` 乱码 |
| Shift_JIS 日文 12–48 字 | Shift_JIS（100；单字节 ≤ 30） | `gb18030` 乱码 |
| EUC-JP 日文 4、8 字 | **windows-1251**（33–56） | `euc-kr`/`big5` 乱码 |
| EUC-JP 日文 12–48 字 | EUC-JP（100；Big5 35–66，GB18030 44–60） | `big5` 乱码 |
| EUC-KR 韩文 4–12 字 | **ISO-8859-1**（25–37） | `gb18030` 乱码 |
| EUC-KR 韩文 16–48 字 | EUC-KR（100） | `gb18030` 乱码 |
| 俄文 windows-1251 4 字 / 8、12 字 / 16–48 字 | windows-1251（60）/ **Big5**（10）/ windows-1251（17–37） | `gb18030` 乱码 / 报错 / 报错 |
| 德文 48 字（含一个 `ö`） | ISO-8859-1（93） | `gb18030` 乱码 |
| 现有测试 `无目录章节内容。\n第二段内容。`（GB18030，13 字） | **EUC-JP**（EUC-JP 88 与 GB18030 88 并列，列表靠后者胜） | `gb18030` |
| 现有测试其余短文本（GBK/GB18030/Big5，约 20–30 字） | GB18030 / Big5（100） | 相同 |

结论：多字节编码在置信度大于 10 时（即命中了该语言的常用字）结果可靠；置信度为 10 时（ICU 的“多字节字符不超过 10 个”分支）
没有统计依据，上游结果取决于识别器列表顺序；单字节置信度在短的中日韩文本上可达 25–56，与真实俄文（34–37）重叠，不能单靠
阈值区分。

## 4. 当前实现（`5a2fc7d`）

`backend/engine/text_encoding.go#detectAndDecodeTextSample`：空输入与 UTF-32/16 BOM、去 BOM 后的合法 UTF-8、零字节比例判
UTF-16、E2 的“基本是 UTF-8”，然后四个旧编码候选：无坏字符者优先，否则在满足 20:1 的候选中按 `scoreDecodedText` 取最高。
`decodeTextWithEncoding` 只认 UTF 系列与这四个旧编码。`DecodeBody` 自动 charset 时共用同一探测器。

## 5. 差异与裁决

| ID | 差异 | 裁决 |
|---|---|---|
| E4 | Shift_JIS、EUC-KR 被评分判成 GB18030；EUC-JP 没有候选，被判成 Big5；ISO-2022-JP 被当成 UTF-8。上游在文本不太短时判对。 | **must-fix（本切片）** |
| E8 | 没有单字节候选：西文、俄文、希腊文、土耳其文、希伯来文、阿拉伯文报错或被解成乱码。上游按 n-gram 判对（希伯来文除外，见 H1）。 | **must-fix（本切片）** |
| S1 | 短文本（多字节置信度只有 10）上游按列表顺序或单字节噪声取名，常把中日韩短文本判成 Big5/ISO-8859-1/windows-125x 乱码。 | `intentional-redesign`：证据不足时保持 OpenReader 现有结果（第 6 节第 9 步），不照搬上游的乱码。 |
| S2 | 多字节置信度并列第一时上游取列表靠后者（13 字 GB18030 测试文本被判成 EUC-JP）。 | `intentional-redesign`：并列时用 OpenReader 现有的语言评分决出。 |
| H1 | 上游把希伯来文判成 ISO-8859-8-I，Java 不认这个名称，导入失败。 | `intentional-redesign`：ISO-8859-8-I 与 ISO-8859-8 字节映射相同，按 ISO-8859-8 解码。 |
| J1 | ISO-2022-JP 的判断要在“合法 UTF-8 即 UTF-8”之前（它全是 7 位字节）。 | 与上游一致：ISO-2022-JP 的置信度高于 UTF-8 的纯 ASCII 15 与单字节最高者时采用。 |
| J2 | ISO-2022-KR、ISO-2022-CN：上游可解码；x/text 没有对应解码器。 | `unknown`，本切片不改：保持现在的结果（按 UTF-8 显示），记录待后续。 |
| U1 | UTF-8/UTF-16/UTF-32 识别器：OpenReader 继续用自己的 BOM、合法 UTF-8、零字节比例与 E2 规则，且排在 ICU 排名之前。 | `technical-stack-equivalent`：合法 UTF-8 文本上游也是 100（或 80）；极少数“只有 1–3 个非 ASCII 字符的合法 UTF-8”上游可能被单字节压过而成乱码，OpenReader 保持 UTF-8。 |
| D1 | 解码表：x/text 与 Java 在个别码位上不同（如 E2 记录的 GBK 自定义区；Shift_JIS 的厂商扩展字）。 | `technical-stack-equivalent`。 |
| E5 | 坏字节超出容错或不是文本。 | 不改：多字节证据存在但解码不过 20:1 时报错；单字节最高置信度不超过 10 时报错。 |

## 6. 目标行为

`detectAndDecodeTextSample` 的判断顺序（“ICU 分析”指移植的识别器对同一样本的结果，多字节读全部样本，ISO-2022 与单字节读前
8000 字节）：

1. 空输入、UTF-32/16 BOM：不变。
2. 新增：样本含 `0x1B` 时，ISO-2022-JP 是 ICU 的 ISO-2022 第一名、置信度大于 15 且大于单字节最高者 → `iso-2022-jp`。
3. 去 BOM 后的合法 UTF-8 → `utf-8`；零字节比例 → UTF-16；E2“基本是 UTF-8” → `utf-8`：不变。
4. 新增：多字节（Shift_JIS、GB18030、EUC-JP、EUC-KR、Big5）最高置信度大于 10 且大于单字节最高者 → 在置信度并列第一的
   多字节编码中按 OpenReader 现有评分选出；解码（沿用 E1 前缀解码）须满足 E2 的 20:1，否则报错。
5. 新增：单字节最高置信度大于 10，且没有任何多字节识别器处在“不超过 10 个多字节字符、无坏字符、且多字节字符至少占其遍历
   字符的一半”的状态 → 采用 ICU 单字节第一名（并列时列表靠后者优先），ISO-8859-8-I 按 ISO-8859-8 解码。“至少占一半”是实现中
   的细化（第 10 节）：一段德文的 5 个变音字母各自后接字母，被 GBK 读成 5 个无坏字符的双字节字符，原条件把它留在现有选择里
   （GB18030 乱码）；ICU 在这个分支的注释本就是“ASCII or ISO file?”，多字节字符只占很小一部分时应视为单字节文本。
6. 否则沿用 E2 的四候选选择（无坏字符优先、20:1、评分）；仍无结果则报 `unable to detect text encoding`。

全量解码新增 `euc-jp`、`iso-2022-jp` 与上述单字节编码名称。目录规则、伪章节、导入流程与已导入书籍不变。

## 7. 移植与验证

- 新增 `backend/engine/charset_icu.go`（识别逻辑）与 `charset_icu_tables.go`（常用字表、`byteMap`、n-gram 表），从上游 Java
  源文件按原样转写，保留 Unicode/ICU 许可声明并注明来源文件。
- 差异测试：把 Go 生成的夹具字节交给上游 Java 识别器，记录每个识别器的置信度作为期望值；Go 移植对同一输入必须给出完全
  相同的名称与置信度。
- 移植只用于排序与判断；实际解码用 `golang.org/x/text`（`japanese.EUCJP`、`japanese.ISO2022JP`、`charmap.*`）。

## 8. 计划测试

红测（在 `5a2fc7d` 上必须失败）：

1. engine `ParseTXT`：日文 Shift_JIS、EUC-JP、ISO-2022-JP，韩文 EUC-KR，英文 windows-1252（弯引号）、法文 Latin-1、德文、
   波兰文 ISO-8859-2、俄文 windows-1251 与 KOI8-R、希腊文 ISO-8859-7、土耳其文 ISO-8859-9、希伯来文 ISO-8859-8、阿拉伯文
   windows-1256，约 3 KB；断言解码全文与原文一致。
2. engine `DecodeBody(body, "")`：无 `<meta charset>` 的 Shift_JIS 与 windows-1251 页面按各自编码解码。
3. api：直接上传 Shift_JIS 日文书与 windows-1251 俄文书，预览成功、导入后正文与原文一致。

正常对照（修复前后都通过）：

4. 4 字、8 字的 GBK 与 Big5 短文本，以及 13 字的 GB18030 测试文本仍分别解为 GB18030、Big5、GB18030。
5. E1、E2 全部测试与现有编码测试不变；用户小说仍为 `gb18030`、455 章、解码不变。
6. 随机字节、坏字节超出容错的 GBK 仍报错。

## 9. 不重开

- E1 截断前缀解码、E2 的 UTF-8 认定与 20:1 门槛、512000 字节样本。
- E5 的报错立场；J2（ISO-2022-KR/CN）。
- 目录规则、伪章节、导入暂存与前端；SQLite、`data/`、`cache/`、`library/` 与已导入书籍。

## 10. 记录（2026-10-25）

分支 `claude/charset-cjk-single-byte`，基于 `5a2fc7d`：

| 提交 | 内容 |
|---|---|
| `7a161c8` docs | 本合同、gap-analysis 与矩阵条目。 |
| `b45f75c` test | engine 14 种语言/编码 `TestParseTXTRecognizesJapaneseKoreanAndSingleByteTextLikeUpstream`、短中文对照 `TestParseTXTKeepsShortChineseTextOnItsCurrentCharset`、`TestDecodeBodyAutoCharsetRecognizesJapaneseAndCyrillicPages`；api `TestJapaneseAndCyrillicTXTImportAndRead`。 |
| `2521eca` test | `TestParseTXTRecognizesAParagraphOfGermanAsLatin1`（实现第一版暴露的情形，见第 6 节第 5 步）。 |
| `c03827d` fix | `charset_icu.go`、`charset_icu_tables.go`（表格由脚本从上游 Java 源转写，保留 Unicode/ICU 声明）、`charset_icu_test.go`（与上游 Java 逐项比对）、`text_encoding.go` 的判断顺序与解码表。 |
| `d84b837` test | 为比对测试加一个 Shift_JIS 置信度未饱和的夹具。 |

红测：

- `b45f75c` 在 `5a2fc7d` 上：14 种语言/编码全部失败（乱码或报错）；`DecodeBody` 的 Shift_JIS、windows-1251 页面失败；api 的
  Shift_JIS 书导入成乱码（“崱擔偼…”）、windows-1251 书预览 `400 unable to detect text encoding`；五个短中文对照通过。完整
  engine、api 包只有这些失败，`services/localbook` 通过。
- `2521eca`：一段德文 ISO-8859-1 在 `5a2fc7d` 上被解成 GB18030（语料 `de_*_1p`），在实现第一版上也是（“sch鰊”）。

实现后：

- 上述用例全部通过；E1、E2 全部测试与现有编码测试通过。
- 移植比对：Go 对 178 个语料文件给出的前五名名称与置信度与上游 Java `detectAll` 完全一致；`TestICUCharsetConfidencesMatchUpstream`
  的 31 个夹具逐个识别器（全部非零结果）一致。
- 语料判断（完整新流程）：上游判对的日文、韩文、各单字节夹具全部给出同一编码；希伯来文解为 `iso-8859-8`（上游导入失败）；
  用户小说 32 个片段全为 `gb18030`；GBK/Big5 4–48 字与现有测试短文本保持原编码；4、8 字的日文、4–12 字的韩文、4 字俄文保持现有
  结果（与现在相同的乱码，S1），8、12 字俄文仍报错；E2 夹具结果不变，Latin-1 英文由报错变为 `iso-8859-1`；随机字节与超出容错的输入仍报错。细化第 5 步
  后与第一版相比只有三处德文输入由 `gb18030` 变为 `iso-8859-1`。
- 用户 GBK 小说：`gb18030`、455 章，解码全文 SHA-256 仍为 `d2cb6d97…3057`。
- 变异：M1 去掉多字节证据一步 → 日文/韩文、Shift_JIS 页面、既有 Big5 用例与“UTF-8 每 20 字节坏一个”对照失败；M2 并列按 ICU
  列表顺序 → 13 字 GB18030 文本解成 EUC-JP（“涙朕村嫗准坪否”），既有无目录测试与短中文对照失败；M3 去掉短文本保护 → Big5 4、8
  字对照失败；M4 去掉“至少一半” → 德文段落失败；M5 去掉 ISO-2022-JP 一步 → ISO-2022-JP 用例失败；M6 改动 Shift_JIS 常用字表
  一项 → 第一版比对测试未发现（置信度饱和），加入未饱和夹具后失败（70 对 84）。均已还原。

回归：

- backend `go test ./...` 29 个包全部 ok，`go vet ./...` 通过；`go test -race -timeout 40m ./engine ./services/localbook ./api`
  全部 ok（api 1023.0s），没有 data race。
- `backend/go.mod`、`go.sum` 未变（`charmap` 属于已有的 `golang.org/x/text`）。frontend `npm test` 815/815、`npm run build` 通过；
  advisory 门自测 131/131，Go 与前端实际扫描 exit 0；`docker compose config --quiet`、`store-read-list-lifecycle-contract.mjs`
  10 项 PASS、`release-browser-smoke.test.mjs` 10/10。含本地端口的用例在沙箱外运行。
- 未在本地运行：真实浏览器（前端与导入流程未改；api 用例覆盖直接上传）；镜像、浏览器启动门、OS APK 门、fresh/historical 卷门与
  双平台发布由合入 main 后的可信 Actions 运行。

J2（ISO-2022-KR/CN）与 E5 未改。Git：分支已推送并经 `git ls-remote` 核验。镜像：未构建。生产：未访问。
