package engine

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

type icuParityFixture struct {
	name string
	data []byte
}

// icuParityFixtures builds the inputs whose detectAll results were recorded
// from reader-dev's Java detector.
func icuParityFixtures(t *testing.T) []icuParityFixture {
	t.Helper()
	encode := func(textEncoding encoding.Encoding, text string) []byte {
		data, err := textEncoding.NewEncoder().Bytes([]byte(text))
		if err != nil {
			t.Fatalf("encode %q: %v", firstRunes(text, 8), err)
		}
		return data
	}
	simplified := "前浪推后浪，他们在雨里说了很多话。彭向明站在窗前，看着楼下来来往往的人群，心里想着明天的事情。\n"
	traditional := "前浪推後浪，他們在雨裡說了很多話。彭向明站在窗前，看著樓下來來往往的人群，心裡想著明天的事情。\n"
	random := make([]byte, 4000)
	state := uint32(7)
	for index := range random {
		state = state*1664525 + 1013904223
		random[index] = byte(state >> 24)
	}
	return []icuParityFixture{
		{name: "japanese-shift_jis", data: encode(japanese.ShiftJIS, recognitionTestJapanese)},
		{name: "japanese-euc-jp", data: encode(japanese.EUCJP, recognitionTestJapanese)},
		{name: "japanese-iso-2022-jp", data: encode(japanese.ISO2022JP, recognitionTestJapanese)},
		{name: "japanese-shift_jis-html", data: encode(japanese.ShiftJIS, "<html><body><p>"+recognitionTestJapanese+"</p></body></html>")},
		{name: "japanese-shift_jis-8", data: encode(japanese.ShiftJIS, "今日はとても良い")},
		{name: "japanese-shift_jis-kanji", data: encode(japanese.ShiftJIS, strings.Repeat("東京都千代田区丸の内一丁目九番一号。\n", 20))},
		{name: "korean-euc-kr", data: encode(korean.EUCKR, recognitionTestKorean)},
		{name: "korean-euc-kr-8", data: encode(korean.EUCKR, "오늘은 날씨가 정")},
		{name: "english-windows-1252", data: encode(charmap.Windows1252, recognitionTestEnglish)},
		{name: "french-iso-8859-1", data: encode(charmap.ISO8859_1, recognitionTestFrench)},
		{name: "german-iso-8859-1", data: encode(charmap.ISO8859_1, recognitionTestGerman)},
		{name: "polish-iso-8859-2", data: encode(charmap.ISO8859_2, recognitionTestPolish)},
		{name: "polish-windows-1250", data: encode(charmap.Windows1250, recognitionTestPolish)},
		{name: "russian-windows-1251", data: encode(charmap.Windows1251, recognitionTestRussian)},
		{name: "russian-koi8-r", data: encode(charmap.KOI8R, recognitionTestRussian)},
		{name: "russian-iso-8859-5", data: encode(charmap.ISO8859_5, recognitionTestRussian)},
		{name: "greek-iso-8859-7", data: encode(charmap.ISO8859_7, recognitionTestGreek)},
		{name: "turkish-iso-8859-9", data: encode(charmap.ISO8859_9, recognitionTestTurkish)},
		{name: "hebrew-iso-8859-8", data: encode(charmap.ISO8859_8, recognitionTestHebrew)},
		{name: "arabic-windows-1256", data: encode(charmap.Windows1256, recognitionTestArabic)},
		{name: "arabic-iso-8859-6", data: encode(charmap.ISO8859_6, recognitionTestArabic)},
		{name: "chinese-gbk", data: encode(simplifiedchinese.GBK, strings.Repeat(simplified, 4))},
		{name: "chinese-gb18030-four-byte", data: encode(simplifiedchinese.GB18030, "前浪推后浪，𠀀𠀁𠀂他们在雨里说了很多话。\n")},
		{name: "chinese-big5", data: encode(traditionalchinese.Big5, strings.Repeat(traditional, 4))},
		{name: "chinese-gbk-4", data: encode(simplifiedchinese.GBK, "前浪推后")},
		{name: "chinese-big5-8", data: encode(traditionalchinese.Big5, "前浪推後浪，他們")},
		{name: "chinese-gb18030-13", data: encode(simplifiedchinese.GB18030, "无目录章节内容。\n第二段内容。")},
		{name: "chinese-gbk-stray-byte", data: append(encode(simplifiedchinese.GBK, simplified), append([]byte{0xFF}, encode(simplifiedchinese.GBK, simplified)...)...)},
		{name: "chinese-big5-stray-byte", data: append(encode(traditionalchinese.Big5, traditional), append([]byte{0xFF}, encode(traditionalchinese.Big5, traditional)...)...)},
		{name: "utf-8-stray-byte", data: []byte(simplified + "\xff" + simplified)},
		{name: "random-bytes", data: random},
	}
}

// TestICUCharsetConfidencesMatchUpstream checks the port against detectAll of
// reader-dev's Java detector, run on the same bytes with the UTF-8/16/32
// recognizers left out.
func TestICUCharsetConfidencesMatchUpstream(t *testing.T) {
	want := map[string]string{
		"arabic-iso-8859-6":         "ISO-8859-6=67 windows-1256=29 ISO-8859-8-I=7 ISO-8859-7=6 ISO-8859-8=3",
		"arabic-windows-1256":       "windows-1256=67 ISO-8859-6=26 ISO-8859-8-I=20 ISO-8859-7=6 ISO-8859-8=5 ISO-8859-5=2",
		"chinese-big5":              "Big5=100 ISO-8859-1=25 GB18030=10 Shift_JIS=10",
		"chinese-big5-8":            "ISO-8859-1=35 Big5=10 GB18030=10 Shift_JIS=10",
		"chinese-big5-stray-byte":   "Big5=100 ISO-8859-1=25 GB18030=10 Shift_JIS=10",
		"chinese-gb18030-13":        "EUC-JP=88 GB18030=88 Big5=10 EUC-KR=10 Shift_JIS=10",
		"chinese-gb18030-four-byte": "GB18030=100 EUC-JP=48 EUC-KR=10 windows-1252=9",
		"chinese-gbk":               "GB18030=100 EUC-JP=61 Big5=10 EUC-KR=10 Shift_JIS=10 ISO-8859-8=9 ISO-8859-1=7 ISO-8859-7=5 KOI8-R=3 windows-1256=3 ISO-8859-6=3",
		"chinese-gbk-4":             "Big5=10 EUC-KR=10 EUC-JP=10 GB18030=10",
		"chinese-gbk-stray-byte":    "GB18030=93 EUC-JP=41 EUC-KR=29 Big5=10 Shift_JIS=10 ISO-8859-8=9 ISO-8859-1=7 ISO-8859-7=5 KOI8-R=3 windows-1256=3 ISO-8859-6=2",
		"english-windows-1252":      "windows-1252=79 windows-1250=20 windows-1254=10",
		"french-iso-8859-1":         "ISO-8859-1=74 ISO-8859-2=42 ISO-8859-9=23",
		"german-iso-8859-1":         "ISO-8859-1=81 ISO-8859-9=40 ISO-8859-2=24 Big5=10 GB18030=10 Shift_JIS=10",
		"greek-iso-8859-7":          "ISO-8859-7=57 KOI8-R=23 windows-1256=9 ISO-8859-8=6 ISO-8859-8-I=4 ISO-8859-6=3",
		"hebrew-iso-8859-8":         "ISO-8859-8-I=50 ISO-8859-8=9 KOI8-R=4 ISO-8859-7=2",
		"japanese-euc-jp":           "EUC-JP=100 GB18030=73 Big5=70 EUC-KR=29 Shift_JIS=10 windows-1251=6 ISO-8859-8=6 ISO-8859-7=6 KOI8-R=3 ISO-8859-1=3 ISO-8859-6=2",
		"japanese-iso-2022-jp":      "ISO-2022-JP=70 ISO-8859-2=18 ISO-8859-1=16 Big5=10 EUC-KR=10 EUC-JP=10 GB18030=10 Shift_JIS=10 ISO-8859-9=4",
		"japanese-shift_jis":        "Shift_JIS=100 KOI8-R=10 Big5=10 GB18030=10 windows-1251=8 windows-1253=7 windows-1250=7 windows-1252=5 windows-1255=3 ISO-8859-5=1",
		"japanese-shift_jis-8":      "windows-1255=50 windows-1252=21 windows-1253=17 Big5=10 GB18030=10 Shift_JIS=10",
		"japanese-shift_jis-html":   "Shift_JIS=100 Big5=10 GB18030=10 KOI8-R=8 windows-1251=7 windows-1253=6 windows-1250=6 windows-1252=5 windows-1255=2 ISO-8859-5=1",
		"japanese-shift_jis-kanji":  "Shift_JIS=84 windows-1253=10 windows-1252=10 Big5=10 GB18030=10",
		"korean-euc-kr":             "EUC-KR=100 GB18030=30 ISO-8859-1=12 ISO-8859-8-I=11 Big5=10 EUC-JP=10 ISO-8859-7=8 KOI8-R=6 ISO-8859-6=3 ISO-8859-5=2 windows-1251=1",
		"korean-euc-kr-8":           "ISO-8859-1=30 Big5=10 EUC-KR=10 EUC-JP=10 GB18030=10 Shift_JIS=10",
		"polish-iso-8859-2":         "ISO-8859-2=53 ISO-8859-1=34 ISO-8859-9=10",
		"polish-windows-1250":       "windows-1250=54 windows-1252=34 windows-1254=10",
		"random-bytes":              "windows-1252=4 windows-1250=3 windows-1253=2 KOI8-R=1 windows-1251=1 ISO-8859-5=1",
		"russian-iso-8859-5":        "ISO-8859-5=34 ISO-8859-7=6 KOI8-R=2",
		"russian-koi8-r":            "KOI8-R=34 ISO-8859-7=8 windows-1251=6 ISO-8859-5=2",
		"russian-windows-1251":      "windows-1251=34 ISO-8859-6=7 ISO-8859-8=4 KOI8-R=2 ISO-8859-8-I=2",
		"turkish-iso-8859-9":        "ISO-8859-9=48 ISO-8859-1=13 Big5=10 GB18030=10 Shift_JIS=10 ISO-8859-2=9",
		"utf-8-stray-byte":          "windows-1252=35 windows-1253=19 Big5=10 GB18030=10 Shift_JIS=10 KOI8-R=9 windows-1255=6 windows-1255=3",
	}
	for _, fixture := range icuParityFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			parts := []string{}
			for _, match := range analyzeICUCharsets(fixture.data).ranked() {
				parts = append(parts, fmt.Sprintf("%s=%d", match.name, match.confidence))
			}
			if got := strings.Join(parts, " "); got != want[fixture.name] {
				t.Fatalf("ICU matches\n got %s\nwant %s", got, want[fixture.name])
			}
		})
	}
}
