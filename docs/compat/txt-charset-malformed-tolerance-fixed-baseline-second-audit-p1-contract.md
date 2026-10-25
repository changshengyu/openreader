# 文本编码探测的少量非法字节容错合同（P1，E2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`

当前实施基线：`origin/main` 为 `83f7e48`（应用代码与已发布的 `94c696f` 相同）。

审查日期：2026-10-25
状态：**aligned / Docker-published / awaiting-device-verification**（证据见第 9 节，发布见第 10 节；E4/E5/E8 未处理）

来源：[`txt-legacy-encoding-probe-boundary-fixed-baseline-second-audit-p1-contract.md`](txt-legacy-encoding-probe-boundary-fixed-baseline-second-audit-p1-contract.md)
（下称 E1 合同）第 4 节的 E2。用户 2026-10-25 要求：按上游 ICU 的容错规则，让样本里少量非法字节不再导致
`unable to detect text encoding`，先合同和红测，并处理损坏的 UTF-8 的优先级。E1 已签收的截断边界不重开。

## 1. 范围与结论

`detectAndDecodeText` 只要某个旧编码候选的解码结果里有一个 U+FFFD 就淘汰它；文件不是合法 UTF-8 时也不再考虑 UTF-8。
所以样本里只要有一个坏字节（下载拼接残缺、混入的 `0xFF`、编辑器写坏的字符），GBK、Big5、UTF-8 文件都会报
`unable to detect text encoding`。上游的 ICU 识别器容许少量坏字符：多字节编码在有效多字节字符不少于坏字符 20 倍时仍判定
该编码，UTF-8 在有效序列多于无效序列 10 倍时仍给出置信度，然后整本按该编码解码，坏字节变成 U+FFFD。

用上游代码实测（第 3 节）还发现：不带 BOM、只有个别坏字节的 UTF-8 中文文件，上游的 UTF-8 置信度只有 25，被 Shift_JIS
（53–72）压过，整本解成乱码。而 GB18030 在这种文件上也能通过多字节容错（坏字符约 5%），如果只照搬多字节容错、不先认
UTF-8，OpenReader 会把它解成 GB18030 乱码。这就是用户要求处理的“损坏 UTF-8 的优先级”。

裁决：**must-fix（P1，本地导入与书源正文解码失败）**。实现顺序：现有 BOM、合法 UTF-8、UTF-16 判断不变；随后按 ICU
的 UTF-8 计数认定“基本是 UTF-8”的输入；再按现有方式选择无坏字符的旧编码候选；都没有时，在满足 ICU 多字节容错的候选中
按现有评分选择。仍然没有候选时照旧报错。

## 2. 固定上游

| 位置 | 行为 |
|---|---|
| `src/main/java/io/legado/app/model/localBook/TextFile.kt#getChapterList`（L64–88） | 读最多 512000 字节，`EncodingDetect.getEncode(buffer.copyOf(length))` 定编码（L69–71）。 |
| `src/main/java/io/legado/app/utils/EncodingDetect.kt#getEncode(bytes)`（L54–57） | `CharsetDetector().setText(bytes).detect()`，没有匹配时为 `"UTF-8"`。`getHtmlEncode`（L16–52）先找 `<meta charset>`，否则同样调用 `getEncode`（L51）。 |
| `src/main/java/io/legado/app/help/http/EncodeConverter.kt`（L30–34）、`OkHttpUtils.kt`（L113–114） | 响应没有声明 charset 时用 `getHtmlEncode(responseBytes)` 定编码，再 `String(responseBytes, Charset.forName(...))`。书源正文与本地 TXT 用同一个探测器。 |
| `src/main/java/io/legado/app/lib/icu4j/CharsetDetector.java#setText`（L89–94） | `fRawInput` 为传入的全部字节。`MungeInput`（L331 起）只把前 `min(length, 8000)` 字节复制到 `fInputBytes`（L373–384）。 |
| 同上 `#detectAll`（L181–203） | 依次运行识别器（列表 L463–494：UTF-8、UTF-16/32、Shift_JIS、ISO-2022、GB18030、EUC-JP、EUC-KR、Big5、单字节…），收集置信度大于 0 的结果，`Collections.sort` 后 `Collections.reverse`（L198–199）：置信度高者优先，相同时列表中靠后的识别器优先。 |
| `src/main/java/io/legado/app/lib/icu4j/CharsetRecog_UTF8.java#match`（L25–96） | 读 `fRawInput` 全部字节。高位字节按前导位判断应有的尾字节数；非法前导或尾字节不对各记一个 `numInvalid`，完整序列记 `numValid`；序列被输入末尾截断时两者都不记。置信度：有 BOM 且无无效序列 100；有 BOM 且 `numValid > numInvalid * 10` 为 80（L81）；无无效序列且 `numValid > 3` 为 100、`numValid > 0` 为 80；纯 ASCII 15；否则 `numValid > numInvalid * 10` 为 25（L92–94），其余为 0（不参与排序）。 |
| `src/main/java/io/legado/app/lib/icu4j/CharsetRecog_mbcs.java#iteratedChar.nextByte`（L157–163） | 多字节识别器同样读 `fRawInput` 全部字节。 |
| 同上 `#match`（L49–131） | 坏字符记 `badCharCount`，有效多字节字符记 `doubleByteCharCount`；扫描中 `badCharCount >= 2 && badCharCount * 5 >= doubleByteCharCount` 即放弃（L80）；`doubleByteCharCount < 20 * badCharCount` 置信度为 0（L106）；否则按常用字频率给分。 |
| `src/main/java/io/legado/app/lib/icu4j/CharsetRecog_2022.java`、`CharsetRecog_sbcs.java` | ISO-2022 与单字节识别器读 `fInputBytes`（前 8000 字节）。 |
| `TextFile.kt#analyze`（L105–135） | `String(buffer, 0, end, charset)` 解码，非法序列替换为 U+FFFD。 |

### 2.1 E1 合同勘误

E1 合同说“ICU `CharsetDetector` 最多只看样本的前 8000 字节”，并据此把 E3（上游 8000 字节、OpenReader 512000 字节）判为
`acceptable-change`。这不对：本上游副本中 UTF-8、UTF-16/32 和全部多字节识别器读 `fRawInput`，即 `TextFile` 传入的整个
512000 字节缓冲区；只有 ISO-2022 和单字节识别器只看前 8000 字节。所以对 OpenReader 支持的编码，探测样本与上游一致，E3
应为 `aligned`；2026-07-11 TXT 目录合同“Detection sample”一行原本就是对的。E1 的结论不变：截断拆开的末尾字符在多字节
识别器里至多记一个坏字符，在 UTF-8 识别器里不计数，上游不会因此失败。

## 3. 上游实测

方法：在 `eclipse-temurin:21-jdk` 容器里编译上游 `io.legado.app.lib.icu4j` 全部 8 个源文件（只依赖 JDK），再用一个
小程序照 `TextFile` 的做法：取文件前 `min(length, 512000)` 字节调用 `detectAll()` 与 `detect()`，没有匹配时取 `"UTF-8"`，
再用 `new String(bytes, charset)` 解码整份文件。夹具用自写的简体/繁体中文段落生成（GBK、Big5、UTF-8，约 3 KB 与约 1 MB
两档），坏字节插在行首。同一批夹具也在 `83f7e48` 上跑了 OpenReader 当前的探测。

| 夹具 | 上游 `getEncode`（置信度前几名） | 上游全文 U+FFFD | OpenReader `83f7e48` |
|---|---|---|---|
| GBK 无坏字节（3 KB） | GB18030（100；EUC-JP 71） | 0 | `gb18030` |
| GBK 1 个 `0xFF`（3 KB） | GB18030（100；EUC-JP 70） | 1 | `unable to detect text encoding` |
| GBK 3 个 `0xFF`（1 MB） | GB18030（100；EUC-JP 83） | 3 | 同上报错 |
| GBK 3 个孤立前导字节 `0xD6`（1 MB） | GB18030（100） | 3 | 同上报错 |
| Big5 无坏字节（3 KB） | Big5（100） | 0 | `big5` |
| Big5 3 个 `0xFF`（1 MB） | Big5（100） | 3 | 同上报错 |
| Big5 3 对 GBK 合法、Big5 非法的 `0x85 0x85`（1 MB） | Big5（100） | 6 | 同上报错 |
| UTF-8 BOM + 1 个 `0xFF`（5 KB） | UTF-8（80；Shift_JIS 53） | 1 | 同上报错 |
| UTF-8 末尾截断半个字符（5 KB） | UTF-8（100） | 1 | 同上报错 |
| UTF-8 无 BOM、1 个 `0xFF`（5 KB） | **Shift_JIS**（53；windows-1252 29；UTF-8 25） | 461（乱码） | 同上报错 |
| UTF-8 无 BOM、3 个 `0xFF`（1 MB） | **Shift_JIS**（72；UTF-8 25） | 92003（乱码） | 同上报错 |
| UTF-8 无 BOM、3 处 Latin-1 `é`（1 MB） | **Shift_JIS**（72；UTF-8 25） | 92003（乱码） | 同上报错 |
| GBK 约 8% 字节为 `0xFF`（3 KB） | EUC-JP（66） | 424（乱码） | 同上报错 |
| UTF-8 每 20 字节一个 `0xFF`（5 KB） | Shift_JIS（51） | 691（乱码） | 同上报错 |
| 随机字节（20 KB） | UTF-16LE（10） | 311（乱码） | 同上报错 |
| Latin-1 英文（4 KB，1 个 `é`） | ISO-8859-1（78） | 0 | 同上报错 |
| 用户 GBK 小说（4,117,155 字节） | GB18030（100） | 0 | `gb18030` |

用 OpenReader 的解码器统计同一批夹具（样本即前 512000 字节）：

- GBK 坏字节夹具：`gb18030` 坏字符 1–39 个，多字节字符 1,720–254,519 个，满足 20 倍；`euc-kr`、`big5` 也满足，但评分
  远低于 `gb18030`（如 19,880 对 10,040 与 12,420）。
- Big5 坏字节夹具：`big5` 坏字符 3、多字节 254,519；`shift_jis` 也满足但评分 893,778 对 2,941,778；`gb18030` 坏字符
  106,550，不满足。
- 损坏 UTF-8 夹具：ICU 计数有效 1,720/170,007 个、无效 1/3 个；但 `gb18030` 坏字符 121/11,863、多字节 2,520/249,080，
  也满足 20 倍。
- 合法 GBK、Big5 按 UTF-8 计数：有效 400、无效 1,680–1,780，远不满足 10 倍。
- 超出容错的夹具（GBK 8%、UTF-8 每 20 字节、随机字节、Latin-1）：没有任何候选满足。

## 4. 当前实现（`83f7e48`）

| 位置 | 行为 |
|---|---|
| `backend/engine/text_encoding.go#detectAndDecodeTextSample`（L43–97） | 空输入、UTF-32/16 BOM 直接解码；去掉 UTF-8 BOM 后 `utf8.Valid` 即 UTF-8（L61）；按零字节比例判 UTF-16（L64–69）；否则依次试 `gb18030`、`big5`、`shift_jis`、`euc-kr`（L71），解码结果含 U+FFFD 即淘汰（L82），按 `scoreDecodedText` 取最高分；全部淘汰时报 `unable to detect text encoding`（L94）。 |
| `decodeTextWithEncoding`（L158 起） | `utf-8` 原样保留字节（L161）；旧编码用 x/text 解码，非法序列替换为 U+FFFD。 |
| `backend/engine/fetcher.go#DecodeBody`（L769） | charset 为空或 `auto` 且没有 `<meta charset>` 时调用同一个 `detectAndDecodeText`。 |

## 5. 差异与裁决

| ID | 差异 | 裁决 |
|---|---|---|
| T1 | 旧多字节编码文件（GBK/GB18030、Big5）样本里有少量坏字节：上游按该编码解码，OpenReader 报错。 | **must-fix**。按 ICU 多字节容错：有效多字节字符不少于坏字符 20 倍的候选可以入选。 |
| T2 | 带 UTF-8 BOM 且有少量坏字节，或 UTF-8 文件末尾截断半个字符：上游判 UTF-8（80/100），OpenReader 报错。 | **must-fix**，与上游一致。 |
| T3 | 不带 BOM、有少量坏字节的 UTF-8：上游 UTF-8 只得 25，被 Shift_JIS 等压过，整本乱码；OpenReader 报错。 | **intentional-redesign（用户 2026-10-25 要求）**：按 ICU 的 UTF-8 计数，有效序列多于无效序列 10 倍即认定为 UTF-8，排在旧编码候选之前。合法的 GBK/Big5 中文按 UTF-8 计数时无效序列远多于有效序列，不会被误认。 |
| T4 | ICU 在扫描途中坏字符偏多（`bad >= 2 && bad * 5 >= dbcs`）就放弃该编码，结果取决于坏字节在样本中的位置。 | **intentional-redesign（同 T3）**：OpenReader 按整个样本的比例判断，开头有几个坏字节的文件也能识别。 |
| T5 | ICU 不优先无错误的候选；OpenReader 先在无坏字符的候选中选，都没有时才用容错候选。 | `technical-stack-equivalent`：实测夹具上结果相同；保证当前能导入的文件结果完全不变。 |
| T6 | 坏字节变成几个 U+FFFD：Java 按畸形序列计；x/text 按其解码单元计；OpenReader 的 `utf-8` 路径保留原字节，接口 JSON 输出时每个非法字节变成一个 U+FFFD。 | `technical-stack-equivalent`：坏字节处显示替换字符，其余正文相同。 |
| T7 | 书源正文（`DecodeBody` 自动 charset）共用同一探测器。 | 上游同样共用（`getHtmlEncode` → `getEncode`），T1–T6 一并适用，`aligned`。 |
| E5 | 坏字节超出容错或根本不是文本：上游选一个不相关的编码（EUC-JP、Shift_JIS、UTF-16LE）导入乱码，OpenReader 报错。 | `unknown`，本切片不改，保留报错。 |
| E8（新） | 单字节编码（如 Latin-1 英文）：上游判 ISO-8859-1 正确解码，OpenReader 没有单字节候选，报错。 | `must-fix`，另起切片（与 E4 的日文/韩文评分一起盘点编码覆盖）。 |

## 6. 目标行为

`detectAndDecodeTextSample` 的判断顺序：

1. 空输入、UTF-32/16 BOM、去 BOM 后的合法 UTF-8、零字节比例判 UTF-16：不变。
2. 新增：按 `CharsetRecog_UTF8#match` 的方式计数（被样本末尾截断的序列不计），有效序列大于 0 且多于无效序列 10 倍时判
   `utf-8`。解码文本沿用现有 `utf-8` 做法，原样保留字节。
3. 旧编码候选（沿用 E1 的前缀解码）：有无坏字符的候选时，按现有评分在其中选择，结果与现在完全相同。
4. 新增：都有坏字符时，只保留“有效多字节字符（解码出的非 ASCII、非 U+FFFD 字符）不少于 U+FFFD 个数 20 倍”的候选，
   按现有评分选择；全量解码时坏字节由 x/text 替换为 U+FFFD。
5. 仍无候选：照旧返回 `unable to detect text encoding`。

不改：E1 的截断前缀解码、512000 字节样本、候选列表与评分、目录规则、全量解码函数、导入暂存与前端流程、已导入书籍。

## 7. 计划测试

红测（在 `83f7e48` 上必须失败）：

1. engine `ParseTXT`：GBK 小样本 1 个 `0xFF`、大样本（超过 512000 字节）几个 `0xFF`、孤立前导字节；Big5 几个 `0xFF`；
   UTF-8 带 BOM 加坏字节；UTF-8 末尾截断；UTF-8 不带 BOM 加 `0xFF` 与 Latin-1 `é`。断言解析成功，正文除坏字节位置外与原文
   一致（旧编码在坏字节处是 U+FFFD；UTF-8 在坏字节处是原字节），损坏的 UTF-8 不被解成 GB18030。
2. api：直接上传一个带坏字节的 GBK 书和一个带坏字节的 UTF-8 书，预览成功、导入后正文可读。
3. engine `DecodeBody(body, "")`：带坏字节的 UTF-8 网页与 GBK 网页按各自编码解码。

正常对照（修复前后都通过）：

4. 无坏字节的 GBK、Big5、UTF-8、UTF-16、E1 截断夹具与既有编码测试结果不变。
5. 超出容错的输入仍报 `unable to detect text encoding`：GBK 约 8% 坏字节、UTF-8 每 20 字节一个坏字节、随机字节。

## 8. 不重开

- E1 截断边界与 512000 字节样本；目录规则、伪章节、前言。
- 候选列表、评分、UTF-16 无 BOM 判断与 HTML `<meta charset>` 解析。
- E4（Shift_JIS/EUC-KR 评分）、E5（超出容错时报错）、E8（单字节编码）。
- 导入暂存、token、LocalStore/WebDAV 流程、parser 预算与前端；SQLite、`data/`、`cache/`、`library/` 与已导入书籍。

## 9. 记录（2026-10-25）

分支 `claude/txt-charset-malformed-tolerance`，基于 `83f7e48`：

| 提交 | 内容 |
|---|---|
| `3350984` docs | 本合同、gap-analysis 与矩阵条目，以及 E1 合同和 E1 gap-analysis 条目的勘误注记。 |
| `5ca5684` test | engine `TestParseTXTToleratesSparseMalformedBytesLikeUpstream`、`TestParseTXTStillRejectsInputBeyondUpstreamMalformedTolerance`、`TestDecodeBodyAutoCharsetToleratesSparseMalformedBytes`；api `TestTXTWithSparseMalformedBytesImportsAndReads`。 |
| `9ce3d39` fix | `text_encoding.go`：新增 `mostlyUTF8`（移植 ICU UTF-8 计数）与 `countDecodedRunes`；在 UTF-16 判断之后、旧编码之前认定损坏的 UTF-8；旧编码候选按 20:1 容错入选，无坏字符的候选优先。 |

红测（`5ca5684`，应用代码为 `83f7e48`）：

- engine：八个容错用例全部报 `unable to detect text encoding`：GBK 1 个坏字节；GBK 571,503 字节、样本内 3 个坏字节；
  GBK 换行前孤立前导字节；Big5 571,503 字节、样本内 3 个坏字节；UTF-8 带 BOM 加坏字节；UTF-8 末尾截断；UTF-8 无 BOM
  加坏字节；UTF-8 760,015 字节、样本内 3 处 Latin-1 `é`。无坏字节的 GBK、Big5 两行通过。
- 对照 `TestParseTXTStillRejectsInputBeyondUpstreamMalformedTolerance`（GBK 每 12 字节一个坏字节、UTF-8 每 20 字节一个、
  随机字节）通过：仍报错。
- `DecodeBody`：带一个坏字节的 UTF-8 网页报错。api：GBK 与 UTF-8 各带一个坏字节，预览都返回
  `400 failed to parse local book: unable to detect text encoding`。
- 完整 engine、api 包只有这些用例失败，`services/localbook` 通过。

实现后（`9ce3d39`）：

- 上述用例全部转绿，对照仍报错；E1 的截断测试、既有编码测试、`DecodeBody` 既有测试与 GBK/GB18030/BOM 导入测试通过。
- 变异：M1 去掉 UTF-8 认定，M2 把它移到旧编码之后（即错误的优先级），都恰好让四个 UTF-8 用例、`DecodeBody` 与 api UTF-8
  失败；M3 把旧编码恢复为见坏字符即淘汰，恰好让四个 GBK/Big5 用例、`DecodeBody`（GBK 部分）与 api GBK 失败；M4 去掉
  20:1 门槛，恰好让三个超出容错的对照失败。每次都已还原。
- 第 3 节全部夹具：GBK、Big5（含 GBK 合法的坏对）判 `gb18030`、`big5`，坏字节处为 U+FFFD；UTF-8 带 BOM、末尾截断、不带
  BOM 的坏字节与 Latin-1 `é` 都判 `utf-8`，坏字节原样保留；GBK 8%、UTF-8 每 20 字节、随机字节、Latin-1 英文仍报错。
  GBK 行首孤立前导字节会和下一个字的前导字节拼成一个字，该行其余部分在上游和 OpenReader 中都是乱码；乱码的具体字符不同
  （x/text 把 GBK 自定义区码位解成 U+FFFD，Java 解成私用区字符），属于 T6。
- 用户 GBK 小说结果不变：`gb18030`、455 章、解码全文 SHA-256 仍为 `d2cb6d97…3057`。在其第 100,000 字节后的换行处插入一个
  `0xFF`：GBK 原文判 `gb18030`，455 章，1 个 U+FFFD；转成 UTF-8 后同样插入，判 `utf-8`，455 章，1 个原样保留的坏字节。

回归：

- backend `go test ./...` 29 个包全部 ok，`go vet ./...` 通过；`go test -race -timeout 40m ./engine ./services/localbook ./api`
  全部 ok（api 1021.3s），没有 data race。后台运行时用 `go -C backend` 指定模块目录：此前两次后台运行在模块解析阶段失败
  （`directory prefix . does not contain main module`），没有执行任何测试，不计为结果。
- frontend `npm test` 815/815、`npm run build` 通过；advisory 门自测 131/131，Go 与前端实际扫描 exit 0；
  `docker compose config --quiet` 通过；`store-read-list-lifecycle-contract.mjs` 10 项 PASS；`release-browser-smoke.test.mjs` 10/10。
  含本地端口的用例均在沙箱外运行。
- 未在本地运行：真实浏览器（前端与导入流程未改，E1 已在 1440×900 验证同一流程；本切片由 engine 与 api 用例覆盖）；镜像、
  浏览器启动门、OS APK 门、fresh/historical 卷门与双平台发布由合入 main 后的可信 Actions 运行。

E4、E5、E8 未处理。Git：分支已推送并经 `git ls-remote` 核验。镜像：未构建。生产：未访问。

## 10. 发布（2026-10-25）

`main` 由 `83f7e48` 快进到 `7c447a5`（本切片的四个提交，没有 force push）。可信 Actions run `40179627517` 全部步骤成功。

- `ghcr.io/changshengyu/openreader:7c447a5` 与 `latest`：OCI index
  `sha256:afe0290b923ee8bd599a9abf787d982558382d273bbcf5fbbb65f7c3506d64ba`。
- `linux/amd64` manifest `sha256:af11e858a86d927b98ec7610af06b7a36cf8a9e39f9d7a8ef24a321d11baee49`；
  `linux/arm64` manifest `sha256:0c53f8929397dc19e458c02d3a7bf412c0244ae6d3ada1c31df8040d90003570`。
- 两个平台的 `org.opencontainers.image.revision` 都是 `7c447a5ae9b3b83819d7505721a5b3559882f5ff`。

本记录提交只改文档，不触发新镜像。生产在另一台 Mac，未访问，实际运行版本未知。升级后，此前因少量坏字节报
`unable to detect text encoding` 的书重新导入即可；已导入的书不变。
