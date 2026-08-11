package engine

import (
	"archive/zip"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	unicodeencoding "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"

	"openreader/backend/models"
)

func TestParseTXTDetectsChineseChapterTitles(t *testing.T) {
	input := []byte("第一章、初见\n这一章的正文。\n第二章再会\n下一章的正文。")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "第一章、初见" {
		t.Fatalf("unexpected first title: %q", chapters[0].Title)
	}
	if chapters[1].Title != "第二章再会" {
		t.Fatalf("unexpected second title: %q", chapters[1].Title)
	}
	if chapters[1].Content != "下一章的正文。" {
		t.Fatalf("unexpected second content: %q", chapters[1].Content)
	}
	if chapters[0].Start != 0 || chapters[0].End <= chapters[0].Start {
		t.Fatalf("unexpected first offsets: start=%d end=%d", chapters[0].Start, chapters[0].End)
	}
	if chapters[1].Start < chapters[0].End || chapters[1].End != len(string(input)) {
		t.Fatalf("unexpected second offsets: start=%d end=%d", chapters[1].Start, chapters[1].End)
	}
}

func TestParseTXTPreservesShortFrontMatterAsUpstreamPreface(t *testing.T) {
	input := []byte("测试书名\n作者：某人\n分类：仙侠\n\n序章、剑宗少年\n这一段是序章内容。\n第一章、缘起\n这一段是第一章内容。\n第四十一章 夺异宝\n这一段是第四十一章内容。")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 4 {
		t.Fatalf("expected preface plus 3 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "前言" || chapters[0].Content != "测试书名\n作者：某人\n分类：仙侠" {
		t.Fatalf("short front matter must be the upstream preface, got %+v", chapters[0])
	}
	if chapters[1].Title != "序章、剑宗少年" {
		t.Fatalf("unexpected first titled chapter: %q", chapters[1].Title)
	}
	if chapters[2].Title != "第一章、缘起" || chapters[3].Title != "第四十一章 夺异宝" {
		t.Fatalf("unexpected titled chapters: %+v", chapters)
	}
}

func TestParseTXTDetectsUpstreamDefaultTitleRules(t *testing.T) {
	input := []byte("1、初见\n这一段是普通内容。\n2、再会\n另一段也是普通内容。")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d: %+v", len(chapters), chapters)
	}
	if chapters[0].Title != "1、初见" || chapters[1].Title != "2、再会" {
		t.Fatalf("unexpected detected chapters: %+v", chapters)
	}
}

func TestParseTXTSelectsUpstreamRuleWithOneMatch(t *testing.T) {
	input := []byte("1、唯一章节\n唯一章节正文。")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].Title != "1、唯一章节" {
		t.Fatalf("one matching enabled upstream rule must select a catalogue, got %+v", chapters)
	}
}

func TestParseTXTAutomaticDetectionOnlyUsesInitialUpstreamProbe(t *testing.T) {
	input := []byte(strings.Repeat("a", 512000) + "\n1、迟到标题\n尾部正文")

	automatic, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(automatic) == 0 {
		t.Fatal("automatic fallback must still return readable pseudo chapters")
	}
	for _, chapter := range automatic {
		if chapter.Title == "1、迟到标题" {
			t.Fatalf("heading after the 512 KiB probe must not enable automatic TOC parsing: %+v", automatic)
		}
	}

	explicit, err := ParseTXTWithRule(input, `^\d+、.+$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit) < 2 || explicit[len(explicit)-1].Title != "1、迟到标题" {
		t.Fatalf("explicit TOC rule must parse the full staged document, got %+v", explicit)
	}
}

func TestParseTXTNoTocUsesUpstreamPseudoChapterChunks(t *testing.T) {
	input := strings.Repeat("a", 10*1024) + "\n" + strings.Repeat("b", 10*1024) + "\n" + strings.Repeat("c", 400)

	chapters, err := ParseTXT([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected three upstream fallback chunks, got %d: %+v", len(chapters), chapters)
	}
	for index, want := range []string{"第1章(1)", "第1章(2)", "第1章(3)"} {
		if chapters[index].Title != want {
			t.Fatalf("fallback chapter %d title = %q, want %q", index, chapters[index].Title, want)
		}
	}
	if reconstructed := joinTXTChapterContents(chapters); reconstructed != input {
		t.Fatalf("fallback chapters must preserve contiguous input\n got %q\nwant %q", reconstructed, input)
	}
}

func TestParseTXTNoTocShortTailAppendsToPreviousChapter(t *testing.T) {
	input := strings.Repeat("a", 10*1024) + "\n" + strings.Repeat("b", 80)

	chapters, err := ParseTXT([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].Title != "第1章(1)" {
		t.Fatalf("short fallback tail must extend the preceding pseudo chapter, got %+v", chapters)
	}
	if chapters[0].Content != input {
		t.Fatalf("short fallback tail was not retained: got %q want %q", chapters[0].Content, input)
	}
}

func TestParseTXTNoTocShortFileStillHasUpstreamPseudoTitle(t *testing.T) {
	input := "这是没有目录的短文本。"

	chapters, err := ParseTXT([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].Title != "第1章(1)" || chapters[0].Content != input {
		t.Fatalf("short title-less TXT must retain the upstream pseudo chapter, got %+v", chapters)
	}
}

func TestParseTXTDetectsEnglishChapterDefaultRule(t *testing.T) {
	input := []byte("Chapter 1 Begin\nFirst body.\nChapter 2 Finale\nSecond body.")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d: %+v", len(chapters), chapters)
	}
	if chapters[0].Title != "Chapter 1 Begin" || chapters[1].Title != "Chapter 2 Finale" {
		t.Fatalf("unexpected detected chapters: %+v", chapters)
	}
}

func TestParseTXTWithRuleAcceptsUpstreamLookbehindPrefix(t *testing.T) {
	input := []byte("正文前说明\n第一章 起始\n这是第一章内容。\n第二章 转折\n这是第二章内容。")

	chapters, err := ParseTXTWithRule(input, `(?<=[　\s])第?\s{0,4}[\d〇零一二两三四五六七八九十百千万]+?\s{0,4}章.{0,30}$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected preface plus 2 chapters, got %d: %+v", len(chapters), chapters)
	}
	if chapters[0].Title != "前言" || chapters[1].Title != "第一章 起始" || chapters[2].Title != "第二章 转折" {
		t.Fatalf("unexpected chapters: %+v", chapters)
	}
}

func TestParseTXTWithRuleAcceptsUpstreamNegativeLookahead(t *testing.T) {
	rule := `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))).{0,30}$`
	input := []byte(strings.Join([]string{
		"第一章 开始",
		"第一节课外讲义",
		"正文完结说明",
		"第二章 继续",
		"这是第二章内容。",
	}, "\n"))

	chapters, err := ParseTXTWithRule(input, rule)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d: %+v", len(chapters), chapters)
	}
	if chapters[0].Title != "第一章 开始" || chapters[1].Title != "第二章 继续" {
		t.Fatalf("unexpected chapters: %+v", chapters)
	}
	if strings.Contains(chapters[0].Content, "第二章正文") {
		t.Fatalf("chapter split failed: %+v", chapters)
	}
}

func TestParseTXTWithRuleDoesNotApplyUnrelatedTitleHeuristics(t *testing.T) {
	title := "== " + strings.Repeat("很长的目录标题", 12) + " ==。"
	input := []byte(title + "\n正文。")

	chapters, err := ParseTXTWithRule(input, `^== .+ ==。$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].Title != title {
		t.Fatalf("explicit matching rules must retain long/punctuation-ended titles, got %+v", chapters)
	}
}

func TestParseTXTWithExplicitNonMatchingRuleReturnsEmptyCatalog(t *testing.T) {
	chapters, err := ParseTXTWithRule([]byte("普通正文，没有匹配的目录。"), `^== .+ ==$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 0 {
		t.Fatalf("an explicit rule with no matches must not silently fall back to 正文: %+v", chapters)
	}
}

func TestDefaultTXTTocRulesExactlyMatchFixedUpstream(t *testing.T) {
	want := []TXTTocRule{
		{ID: -1, Enable: true, Name: "目录(去空白)", Rule: `(?<=[　\s])(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))).{0,30}$`, SerialNumber: 0},
		{ID: -2, Enable: true, Name: "目录", Rule: `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))).{0,30}$`, SerialNumber: 1},
		{ID: -3, Enable: false, Name: "目录(匹配简介)", Rule: `(?<=[　\s])(?:(?:内容|文章)?简介|文案|前言|序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|回(?![合来事去])|场(?![和合比电是])|篇(?!张))).{0,30}$`, SerialNumber: 2},
		{ID: -4, Enable: false, Name: "目录(古典、轻小说备用)", Rule: `^[ 　\t]{0,4}(?:序章|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|回(?![合来事去])|场(?![和合比电是])|话|篇(?!张))).{0,30}$`, SerialNumber: 3},
		{ID: -5, Enable: false, Name: "数字(纯数字标题)", Rule: `(?<=[　\s])\d+\.?[ 　\t]{0,4}$`, SerialNumber: 4},
		{ID: -6, Enable: true, Name: "数字 分隔符 标题名称", Rule: `^[ 　\t]{0,4}\d{1,5}[：:,.， 、_—\-].{1,30}$`, SerialNumber: 5},
		{ID: -7, Enable: true, Name: "大写数字 分隔符 标题名称", Rule: `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|[〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 、_—\-].{1,30}$`, SerialNumber: 6},
		{ID: -8, Enable: true, Name: "正文 标题/序号", Rule: `^[ 　\t]{0,4}正文[ 　]{1,4}.{0,20}$`, SerialNumber: 7},
		{ID: -9, Enable: true, Name: "Chapter/Section/Part/Episode 序号 标题", Rule: `^[ 　\t]{0,4}(?:[Cc]hapter|[Ss]ection|[Pp]art|ＰＡＲＴ|[Nn][oO]\.|[Ee]pisode|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)\s{0,4}\d{1,4}.{0,30}$`, SerialNumber: 8},
		{ID: -10, Enable: false, Name: "Chapter(去简介)", Rule: `^[ 　\t]{0,4}(?:[Cc]hapter|[Ss]ection|[Pp]art|ＰＡＲＴ|[Nn][Oo]\.|[Ee]pisode)\s{0,4}\d{1,4}.{0,30}$`, SerialNumber: 9},
		{ID: -11, Enable: true, Name: "特殊符号 序号 标题", Rule: `(?<=[\s　])[【〔〖「『〈［\[](?:第|[Cc]hapter)[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,10}[章节].{0,20}$`, SerialNumber: 10},
		{ID: -12, Enable: false, Name: "特殊符号 标题(成对)", Rule: `(?<=[\s　]{0,4})(?:[\[〈「『〖〔《（【\(].{1,30}[\)】）》〕〗』」〉\]]?|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)[ 　]{0,4}$`, SerialNumber: 11},
		{ID: -13, Enable: true, Name: "特殊符号 标题(单个)", Rule: `(?<=[\s　]{0,4})(?:[☆★✦✧].{1,30}|(?:内容|文章)?简介|文案|前言|序章|楔子|正文(?!完|结)|终章|后记|尾声|番外)[ 　]{0,4}$`, SerialNumber: 12},
		{ID: -14, Enable: true, Name: "章/卷 序号 标题", Rule: `^[ \t　]{0,4}(?:(?:内容|文章)?简介|文案|前言|序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|[卷章][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8})[ 　]{0,4}.{0,30}$`, SerialNumber: 13},
		{ID: -15, Enable: false, Name: "顶格标题", Rule: `^\S.{1,20}$`, SerialNumber: 14},
		{ID: -16, Enable: false, Name: "双标题(前向)", Rule: `(?m)(?<=[ \t　]{0,4})第[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$(?=[\s　]{0,8}第[\d零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章)`, SerialNumber: 15},
		{ID: -17, Enable: false, Name: "双标题(后向)", Rule: `(?m)(?<=[ \t　]{0,4}第[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$[\s　]{0,8})第[\d零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}章.{0,30}$`, SerialNumber: 16},
		{ID: -18, Enable: true, Name: "标题 特殊符号 序号", Rule: `^.{1,20}[(（][\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]{1,8}[)）][ 　\t]{0,4}$`, SerialNumber: 17},
	}

	got := DefaultTXTTocRules()
	if len(got) != len(want) {
		t.Fatalf("default TXT TOC rule count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("default TXT TOC rule %d\n got: %+v\nwant: %+v", index, got[index], want[index])
		}
		if _, err := compileTXTTitleMatcher(got[index].Rule); err != nil {
			t.Fatalf("default rule %d %s does not compile through compatibility matcher: %v", got[index].ID, got[index].Name, err)
		}
	}
}

func TestParseTXTWithDisabledUpstreamRulesSelectedManually(t *testing.T) {
	tests := []struct {
		id       int
		input    string
		title    string
		rejected string
	}{
		{id: -3, input: "简介\n这是简介正文。", title: "简介", rejected: "正文完结\n普通正文。"},
		{id: -4, input: "第一话 开场\n这是第一话正文。", title: "第一话 开场", rejected: "第一场比赛\n普通正文。"},
		{id: -5, input: "1\n这是第一节正文。", title: "1", rejected: "1. 正文\n普通正文。"},
		{id: -10, input: "Chapter 1 Opening\nThis is the chapter body.", title: "Chapter 1 Opening", rejected: "简介 1\n普通正文。"},
		{id: -12, input: "【第一幕】\n这是第一幕正文。", title: "【第一幕】", rejected: "【\n普通正文。"},
		{id: -15, input: "短标题\n这是一段明显超过二十个字符而不应该被顶格标题规则匹配的正文内容。", title: "短标题", rejected: "这是一行明确超过二十个字符且不应被识别成目录标题的普通正文内容。"},
	}
	rules := DefaultTXTTocRules()
	for _, test := range tests {
		t.Run(strconv.Itoa(test.id), func(t *testing.T) {
			rule := findTXTTocRule(t, rules, test.id)
			chapters, err := ParseTXTWithRule([]byte(test.input), rule.Rule)
			if err != nil {
				t.Fatal(err)
			}
			if len(chapters) != 1 || chapters[0].Title != test.title {
				t.Fatalf("manual rule %d parsed %+v, want one chapter %q", test.id, chapters, test.title)
			}
			rejected, err := ParseTXTWithRule([]byte(test.rejected), rule.Rule)
			if err != nil {
				t.Fatal(err)
			}
			if len(rejected) != 0 {
				t.Fatalf("manual rule %d matched rejected fixture: %+v", test.id, rejected)
			}
		})
	}
}

func TestParseTXTAutomaticDetectionSkipsDisabledUpstreamRules(t *testing.T) {
	input := []byte("1\n这是第一段正文。\n2\n这是第二段正文。")
	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, chapter := range chapters {
		if chapter.Title == "1" || chapter.Title == "2" {
			t.Fatalf("disabled pure-number rule must be manual-only: %+v", chapters)
		}
	}
}

func TestParseTXTWithUpstreamDoubleTitleLookaroundRules(t *testing.T) {
	rules := DefaultTXTTocRules()
	input := []byte("第一章 总标题\n第二章 分标题\n这是第二章的正文内容。")

	forward, err := ParseTXTWithRule(input, findTXTTocRule(t, rules, -16).Rule)
	if err != nil {
		t.Fatal(err)
	}
	if len(forward) != 1 || forward[0].Title != "第一章 总标题" || !strings.Contains(forward[0].Content, "第二章 分标题") {
		t.Fatalf("forward double-title rule must select only the first adjacent title: %+v", forward)
	}

	backward, err := ParseTXTWithRule(input, findTXTTocRule(t, rules, -17).Rule)
	if err != nil {
		t.Fatal(err)
	}
	if len(backward) != 2 || backward[0].Title != "前言" || backward[1].Title != "第二章 分标题" {
		t.Fatalf("backward double-title rule must select only the second adjacent title: %+v", backward)
	}

	isolated := []byte("第一章 孤立标题\n这里没有相邻的第二个标题。")
	for _, id := range []int{-16, -17} {
		chapters, err := ParseTXTWithRule(isolated, findTXTTocRule(t, rules, id).Rule)
		if err != nil {
			t.Fatal(err)
		}
		if len(chapters) != 0 {
			t.Fatalf("double-title rule %d matched an isolated title: %+v", id, chapters)
		}
	}
}

func TestCompileTXTTitleMatcherOnlySpecializesExactUpstreamDoubleTitleRules(t *testing.T) {
	if _, err := compileTXTTitleMatcher(txtForwardDoubleTitleRule + `|^附录$`); err == nil {
		t.Fatal("modified lookaround rule must not silently inherit the fixed-upstream double-title matcher")
	}
}

func findTXTTocRule(t *testing.T, rules []TXTTocRule, id int) TXTTocRule {
	t.Helper()
	for _, rule := range rules {
		if rule.ID == id {
			return rule
		}
	}
	t.Fatalf("TXT TOC rule %d not found", id)
	return TXTTocRule{}
}

func TestParseTXTDetectsCommonUpstreamTextEncodings(t *testing.T) {
	tests := []struct {
		name   string
		encode func([]byte) ([]byte, error)
		bom    []byte
	}{
		{name: "GB18030", encode: simplifiedchinese.GB18030.NewEncoder().Bytes},
		{name: "Big5", encode: traditionalchinese.Big5.NewEncoder().Bytes},
		{
			name: "UTF-16LE",
			encode: unicodeencoding.UTF16(
				unicodeencoding.LittleEndian,
				unicodeencoding.IgnoreBOM,
			).NewEncoder().Bytes,
			bom: []byte{0xFF, 0xFE},
		},
		{
			name: "UTF-16BE",
			encode: unicodeencoding.UTF16(
				unicodeencoding.BigEndian,
				unicodeencoding.IgnoreBOM,
			).NewEncoder().Bytes,
			bom: []byte{0xFE, 0xFF},
		},
		{
			name:   "UTF-32LE",
			encode: utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM).NewEncoder().Bytes,
			bom:    []byte{0xFF, 0xFE, 0x00, 0x00},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []byte("第一章 開始\n這是第一章內容。\n第二章 再會\n這是第二章內容。")
			encoded, err := test.encode(input)
			if err != nil {
				t.Fatal(err)
			}
			chapters, err := ParseTXT(append(test.bom, encoded...))
			if err != nil {
				t.Fatal(err)
			}
			if len(chapters) != 2 || chapters[0].Title != "第一章 開始" || chapters[1].Title != "第二章 再會" {
				t.Fatalf("decoded chapters = %+v", chapters)
			}
		})
	}
}

func TestParseTXTNoTocFallbackPreservesLegacyEncodingContent(t *testing.T) {
	input := []byte("无目录章节内容。\n第二段内容。")
	encoded, err := simplifiedchinese.GB18030.NewEncoder().Bytes(input)
	if err != nil {
		t.Fatal(err)
	}
	chapters, err := ParseTXT(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].Title != "第1章(1)" || chapters[0].Content != string(input) {
		t.Fatalf("legacy no-TOC fallback = %+v", chapters)
	}
}

func joinTXTChapterContents(chapters []TXTChapter) string {
	var builder strings.Builder
	for _, chapter := range chapters {
		builder.WriteString(chapter.Content)
	}
	return builder.String()
}

func TestParseEPUBUsesSpineOrder(t *testing.T) {
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	writeZipFile(t, zipWriter, "META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)
	writeZipFile(t, zipWriter, "OEBPS/content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>测试 EPUB</dc:title>
    <dc:creator>测试作者</dc:creator>
  </metadata>
  <manifest>
    <item id="chapter2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
    <itemref idref="chapter2"/>
  </spine>
</package>`)
	writeZipFile(t, zipWriter, "OEBPS/chapter1.xhtml", `<html><head><title>第一章</title></head><body><h1>第一章</h1><p>第一章正文。</p></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/chapter2.xhtml", `<html><head><title>第二章</title></head><body><h1>第二章</h1><p>第二章正文。</p></body></html>`)
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	book, err := ParseEPUB(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "测试 EPUB" || book.Author != "测试作者" {
		t.Fatalf("unexpected metadata: %#v", book)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(book.Chapters))
	}
	if book.Chapters[0].Title != "第一章" || book.Chapters[1].Title != "第二章" {
		t.Fatalf("chapters not in spine order: %#v", book.Chapters)
	}
	if book.Chapters[0].ResourcePath != "OEBPS/chapter1.xhtml" || book.Chapters[1].ResourcePath != "OEBPS/chapter2.xhtml" {
		t.Fatalf("chapter resource paths not preserved: %#v", book.Chapters)
	}
	tocOnly, err := ParseEPUBWithRule(buffer.Bytes(), "toc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tocOnly.Chapters) != 0 {
		t.Fatalf("fixed upstream pure toc rule must remain empty when toc is missing: %#v", tocOnly.Chapters)
	}
	tocFallback, err := ParseEPUBWithRule(buffer.Bytes(), "toc+spin")
	if err != nil {
		t.Fatal(err)
	}
	if len(tocFallback.Chapters) != 2 || tocFallback.Chapters[0].Title != "第一章" {
		t.Fatalf("toc+spin should fall back to readable spine when toc is missing: %#v", tocFallback.Chapters)
	}
}

func TestParseEPUBWithRuleCombinesSpineAndNav(t *testing.T) {
	data := testEPUBWithNav(t)

	tests := []struct {
		rule   string
		titles []string
		bodies []string
	}{
		{rule: "spin", titles: []string{"文档标题一", "文档标题二"}, bodies: []string{"第一章内容。", "第二章内容。"}},
		{rule: "spin+toc", titles: []string{"目录一", "目录二"}, bodies: []string{"第一章内容。", "第二章内容。"}},
		{rule: "spin<toc", titles: []string{"目录一", "目录二"}, bodies: []string{"第一章内容。", "第二章内容。"}},
		{rule: "toc", titles: []string{"目录二", "目录附录", "目录一"}, bodies: []string{"第二章内容。", "附录内容。", "第一章内容。"}},
		{rule: "toc+spin", titles: []string{"目录二", "目录附录", "目录一"}, bodies: []string{"第二章内容。", "附录内容。", "第一章内容。"}},
		{rule: "toc<spin", titles: []string{"目录二", "目录附录", "目录一"}, bodies: []string{"第二章内容。", "附录内容。", "第一章内容。"}},
		{rule: "", titles: []string{"目录一", "目录二"}, bodies: []string{"第一章内容。", "第二章内容。"}},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			book, err := ParseEPUBWithRule(data, tt.rule)
			if err != nil {
				t.Fatal(err)
			}
			if len(book.Chapters) != len(tt.titles) {
				t.Fatalf("chapter count = %d, want %d", len(book.Chapters), len(tt.titles))
			}
			for index := range tt.titles {
				if book.Chapters[index].Title != tt.titles[index] {
					t.Fatalf("chapter %d title = %q, want %q", index, book.Chapters[index].Title, tt.titles[index])
				}
				if !strings.Contains(book.Chapters[index].Content, tt.bodies[index]) {
					t.Fatalf("chapter %d content = %q, want body %q", index, book.Chapters[index].Content, tt.bodies[index])
				}
				if book.Chapters[index].ResourcePath == "" {
					t.Fatalf("chapter %d lost its EPUB resource path: %#v", index, book.Chapters[index])
				}
			}
		})
	}
}

func TestParseEPUBCatalogDoesNotMaterializeSpineBodies(t *testing.T) {
	data := testEPUBWithNav(t)
	limits := DefaultLocalBookParseLimits()
	limits.MaxParsedTextBytes = 1

	catalog, err := ParseEPUBCatalogWithLimits(data, "spin+toc", limits)
	if err != nil {
		t.Fatalf("catalog-only EPUB preview must not apply the body text limit: %v", err)
	}
	if len(catalog.Chapters) != 2 {
		t.Fatalf("catalog chapter count = %d, want 2", len(catalog.Chapters))
	}
	for index, chapter := range catalog.Chapters {
		if chapter.Content != "" {
			t.Fatalf("catalog chapter %d materialized body content: %q", index, chapter.Content)
		}
		if chapter.ResourcePath == "" {
			t.Fatalf("catalog chapter %d lost its EPUB resource path: %+v", index, chapter)
		}
	}
	if _, err := ParseEPUBWithLimits(data, "spin+toc", limits); !errors.Is(err, ErrLocalBookParseLimit) {
		t.Fatalf("full EPUB parse error = %v, want body text limit", err)
	}
}

func TestParseEPUBCatalogUsesDocumentTitleInsteadOfBodyHeading(t *testing.T) {
	data := testEPUBWithNav(t)
	tests := []struct {
		rule   string
		titles []string
	}{
		{rule: "spin", titles: []string{"文档标题一", "文档标题二"}},
		{rule: "spin+toc", titles: []string{"目录一", "目录二"}},
		{rule: "spin<toc", titles: []string{"目录一", "目录二"}},
		{rule: "toc", titles: []string{"目录二", "目录附录", "目录一"}},
		{rule: "toc+spin", titles: []string{"目录二", "目录附录", "目录一"}},
		{rule: "toc<spin", titles: []string{"目录二", "目录附录", "目录一"}},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			catalog, err := ParseEPUBCatalogWithLimits(data, tt.rule, DefaultLocalBookParseLimits())
			if err != nil {
				t.Fatal(err)
			}
			if len(catalog.Chapters) != len(tt.titles) {
				t.Fatalf("chapter count = %d, want %d", len(catalog.Chapters), len(tt.titles))
			}
			for index, want := range tt.titles {
				if got := catalog.Chapters[index].Title; got != want {
					t.Fatalf("chapter %d title = %q, want %q", index, got, want)
				}
				if strings.Contains(catalog.Chapters[index].Title, "正文标题") {
					t.Fatalf("body heading leaked into catalogue title: %+v", catalog.Chapters[index])
				}
			}
		})
	}
}

func TestParseEPUBRetainsFirstImageOnlyTitlepageAsCover(t *testing.T) {
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	writeZipFile(t, zipWriter, "META-INF/container.xml", `<?xml version="1.0"?>
<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`)
	writeZipFile(t, zipWriter, "OPS/content.opf", `<?xml version="1.0"?>
<package>
  <metadata><title>封面 EPUB</title></metadata>
  <manifest>
    <item id="cover" href="titlepage.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>
    <item id="image" href="images/cover.svg" media-type="image/svg+xml"/>
  </manifest>
  <spine><itemref idref="cover"/><itemref idref="chapter"/></spine>
</package>`)
	writeZipFile(t, zipWriter, "OPS/titlepage.xhtml", `<html><body><img src="images/cover.svg" alt="封面"/></body></html>`)
	writeZipFile(t, zipWriter, "OPS/chapter.xhtml", `<html><head><title>第一章</title></head><body><h1>第一章</h1><p>第一章正文。</p></body></html>`)
	writeZipFile(t, zipWriter, "OPS/images/cover.svg", `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>`)
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	book, err := ParseEPUBWithRule(buffer.Bytes(), "spin")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("first image-only titlepage must remain a readable EPUB chapter, got %+v", book.Chapters)
	}
	if book.Chapters[0].Title != "封面" || book.Chapters[0].ResourcePath != "OPS/titlepage.xhtml" {
		t.Fatalf("image-only titlepage = %+v, want upstream cover resource", book.Chapters[0])
	}
	if book.Chapters[1].Title != "第一章" || book.Chapters[1].ResourcePath != "OPS/chapter.xhtml" {
		t.Fatalf("ordinary chapter after titlepage = %+v", book.Chapters[1])
	}
}

func TestParseEPUBWithRuleReadsNCX(t *testing.T) {
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	writeZipFile(t, zipWriter, "META-INF/container.xml", `<?xml version="1.0"?>
<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`)
	writeZipFile(t, zipWriter, "OPS/content.opf", `<?xml version="1.0"?>
<package>
  <metadata><title>NCX EPUB</title></metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="one" href="one.xhtml" media-type="application/xhtml+xml"/>
    <item id="two" href="two.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx"><itemref idref="one"/><itemref idref="two"/></spine>
</package>`)
	writeZipFile(t, zipWriter, "OPS/toc.ncx", `<?xml version="1.0"?>
<ncx><navMap>
  <navPoint><navLabel><text>NCX 二</text></navLabel><content src="two.xhtml"/></navPoint>
  <navPoint><navLabel><text>NCX 一</text></navLabel><content src="one.xhtml"/></navPoint>
</navMap></ncx>`)
	writeZipFile(t, zipWriter, "OPS/one.xhtml", `<html><body><h1>正文一</h1><p>一。</p></body></html>`)
	writeZipFile(t, zipWriter, "OPS/two.xhtml", `<html><body><h1>正文二</h1><p>二。</p></body></html>`)
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	book, err := ParseEPUBWithRule(buffer.Bytes(), "toc")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Chapters) != 2 || book.Chapters[0].Title != "NCX 二" || book.Chapters[1].Title != "NCX 一" {
		t.Fatalf("unexpected NCX chapters: %+v", book.Chapters)
	}
}

func TestParseEPUBNAVFragmentsCollapseToFixedUpstreamResourceCatalog(t *testing.T) {
	data := testEPUBWithFragmentNavigation(t, true)
	tests := []struct {
		rule   string
		titles []string
	}{
		{rule: "spin", titles: []string{"文档标题一", "文档标题二"}},
		{rule: "spin+toc", titles: []string{"第二节", "第三节"}},
		{rule: "spin<toc", titles: []string{"第二节", "第三节"}},
		{rule: "toc", titles: []string{"第二节", "第三节"}},
		{rule: "toc+spin", titles: []string{"第二节", "第三节"}},
		{rule: "toc<spin", titles: []string{"第二节", "第三节"}},
		{rule: "", titles: []string{"第二节", "第三节"}},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			book, err := ParseEPUBWithRule(data, tt.rule)
			if err != nil {
				t.Fatal(err)
			}
			if len(book.Chapters) != 2 {
				t.Fatalf("fixed upstream catalog must dedupe TOC fragments by href: %+v", book.Chapters)
			}
			for index, wantPath := range []string{"OPS/Text/one.xhtml", "OPS/Text/two.xhtml"} {
				chapter := book.Chapters[index]
				if chapter.Title != tt.titles[index] || chapter.ResourcePath != wantPath || chapter.ResourceFragment != "" || chapter.ResourceEndFragment != "" {
					t.Fatalf("chapter %d = %+v, want title=%q path=%q with empty fragments", index, chapter, tt.titles[index], wantPath)
				}
			}
			if !strings.Contains(book.Chapters[0].Content, "片段一正文") || !strings.Contains(book.Chapters[0].Content, "片段二正文") || strings.Contains(book.Chapters[0].Content, "跨资源正文") {
				t.Fatalf("first resource text must contain the whole current XHTML only: %q", book.Chapters[0].Content)
			}
			if !strings.Contains(book.Chapters[1].Content, "跨资源正文") || strings.Contains(book.Chapters[1].Content, "片段二正文") {
				t.Fatalf("second resource text crossed resource boundaries: %q", book.Chapters[1].Content)
			}
		})
	}
}

func TestParseEPUBNCXFragmentsCollapseToFixedUpstreamResourceCatalog(t *testing.T) {
	book, err := ParseEPUBWithRule(testEPUBWithFragmentNavigation(t, false), "toc")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("NCX fragments must collapse by resource href, got %+v", book.Chapters)
	}
	if book.Chapters[0].Title != "第二节" || book.Chapters[0].ResourcePath != "OPS/Text/one.xhtml" ||
		book.Chapters[1].Title != "第三节" || book.Chapters[1].ResourcePath != "OPS/Text/two.xhtml" {
		t.Fatalf("unexpected fixed NCX catalogue: %+v", book.Chapters)
	}
	for _, chapter := range book.Chapters {
		if chapter.ResourceFragment != "" || chapter.ResourceEndFragment != "" {
			t.Fatalf("new NCX catalogue retained fragment metadata: %+v", chapter)
		}
	}
}

func TestBuildEPUBChaptersUsesLastTOCTitleWriteIncludingBlankFallback(t *testing.T) {
	spine := []epubChapter{
		{Path: "OPS/Text/one.xhtml", Title: "文档标题一"},
		{Path: "OPS/Text/two.xhtml", Title: "文档标题二"},
	}
	toc := []epubTOCEntry{
		{Path: "OPS/Text/one.xhtml", Fragment: "part-a", Title: "第一节"},
		{Path: "OPS/Text/one.xhtml", Fragment: "part-b", Title: ""},
		{Path: "OPS/Text/two.xhtml", Fragment: "opening", Title: "第三节"},
	}
	for _, rule := range []string{"spin+toc", "spin<toc", "toc", "toc+spin", "toc<spin", ""} {
		t.Run(rule, func(t *testing.T) {
			chapters := buildEPUBChapters(spine, toc, rule)
			if len(chapters) != 2 || chapters[0].Title != "文档标题一" || chapters[1].Title != "第三节" {
				t.Fatalf("last blank TOC title must fall back to the document title for %q: %+v", rule, chapters)
			}
		})
	}
}

func testEPUBWithNav(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	writeZipFile(t, zipWriter, "META-INF/container.xml", `<?xml version="1.0"?>
<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`)
	writeZipFile(t, zipWriter, "OEBPS/content.opf", `<?xml version="1.0"?>
<package>
  <metadata><title>规则 EPUB</title><creator>作者</creator></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="one" href="text/one.xhtml" media-type="application/xhtml+xml"/>
    <item id="two" href="text/two.xhtml" media-type="application/xhtml+xml"/>
    <item id="appendix" href="text/appendix.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="one"/><itemref idref="two"/></spine>
</package>`)
	writeZipFile(t, zipWriter, "OEBPS/nav.xhtml", `<html><body><nav epub:type="toc">
  <ol><li><a href="text/two.xhtml#start">目录二</a></li><li><a href="text/appendix.xhtml">目录附录</a></li><li><a href="text/one.xhtml">目录一</a></li></ol>
</nav></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/text/one.xhtml", `<html><head><title>文档标题一</title></head><body><h1>正文标题一</h1><p>第一章内容。</p></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/text/two.xhtml", `<html><head><title>文档标题二</title></head><body><h1>正文标题二</h1><p>第二章内容。</p></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/text/appendix.xhtml", `<html><head><title>文档附录</title></head><body><h1>正文附录</h1><p>附录内容。</p></body></html>`)
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func testEPUBWithFragmentNavigation(t *testing.T, includeNav bool) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	write := func(name string, content string) {
		writeZipFile(t, writer, name, content)
	}
	write("META-INF/container.xml", `<?xml version="1.0"?>
<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`)
	navManifest := ""
	navFile := ""
	if includeNav {
		navManifest = `<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`
		navFile = `<html><body><nav epub:type="toc"><ol>
  <li><a href="Text/one.xhtml#part-a">第一节</a></li>
  <li><a href="Text/missing.xhtml#ignored">无效目录项</a></li>
  <li><a href="Text/one.xhtml#part-b">第二节</a></li>
  <li><a href="Text/two.xhtml#opening">第三节</a></li>
</ol></nav></body></html>`
	} else {
		navManifest = `<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>`
		navFile = `<?xml version="1.0"?><ncx><navMap>
  <navPoint><navLabel><text>第一节</text></navLabel><content src="Text/one.xhtml#part-a"/></navPoint>
  <navPoint><navLabel><text>第二节</text></navLabel><content src="Text/one.xhtml#part-b"/></navPoint>
  <navPoint><navLabel><text>第三节</text></navLabel><content src="Text/two.xhtml#opening"/></navPoint>
</navMap></ncx>`
	}
	spineTOC := ""
	if !includeNav {
		spineTOC = ` toc="ncx"`
	}
	write("OPS/content.opf", `<?xml version="1.0"?>
<package><metadata><title>Fragment EPUB</title></metadata><manifest>`+navManifest+`
  <item id="one" href="Text/one.xhtml" media-type="application/xhtml+xml"/>
  <item id="two" href="Text/two.xhtml" media-type="application/xhtml+xml"/>
</manifest><spine`+spineTOC+`><itemref idref="one"/><itemref idref="two"/></spine></package>`)
	if includeNav {
		write("OPS/nav.xhtml", navFile)
	} else {
		write("OPS/toc.ncx", navFile)
	}
	write("OPS/Text/one.xhtml", `<html><head><title>文档标题一</title></head><body>
  <section id="part-a"><h1>第一节</h1><p>片段一正文</p><a id="to-part-b" href="#part-b">下一节</a></section>
  <section id="part-b"><h1>第二节</h1><p>片段二正文</p><a id="to-two" href="two.xhtml#opening">跨资源章节</a></section>
</body></html>`)
	write("OPS/Text/two.xhtml", `<html><head><title>文档标题二</title></head><body><section id="opening"><h1>第三节</h1><p>跨资源正文</p></section></body></html>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestApplyTextReplacementsSupportsRegex(t *testing.T) {
	got := ApplyTextReplacements("广告一\n正文\n广告二", []models.TextReplaceRule{
		{Pattern: `广告.`, Replacement: ""},
	})
	if got != "\n正文\n" {
		t.Fatalf("unexpected replacement result: %q", got)
	}
}

func TestNormalizeChapterHTMLKeepsTextAndSafeImages(t *testing.T) {
	got := normalizeChapterHTML(`
		<div>第一段 <strong>正文</strong></div>
		<p><img data-src="../images/one.jpg" alt="插图一"></p>
		<script>alert(1)</script>
		<img src="javascript:alert(1)">
		<p>第二段</p>
	`, "https://books.example/chapters/1.html")
	want := "第一段 正文\n<img src=\"https://books.example/images/one.jpg\" alt=\"插图一\">\n第二段"
	if got != want {
		t.Fatalf("normalized html = %q, want %q", got, want)
	}
}

func TestParseSearchResultsIncludesLatestChapter(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div class="book">
			<a class="name" href="/book/1">测试书</a>
			<span class="author">作者</span>
			<span class="latest">第一百章 新消息</span>
		</div>
	`))
	if err != nil {
		t.Fatal(err)
	}
	results := parseSearchResults(doc, models.BookSourceRule{
		BookListRule:      ".book",
		BookNameRule:      ".name",
		BookAuthorRule:    ".author",
		LatestChapterRule: ".latest",
		BookURLRule:       ".name|attr:href",
	}, models.BookSource{ID: 1, Name: "测试源", BaseURL: "https://source.example"})
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].LatestChapter != "第一百章 新消息" {
		t.Fatalf("latest chapter was not parsed: %+v", results[0])
	}
}

func TestParseSearchResultsHonorsListOrderPrefix(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div class="book"><a class="name" href="/book/1">第一本</a></div>
		<div class="book"><a class="name" href="/book/2">第二本</a></div>
	`))
	if err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{ID: 1, Name: "顺序源", BaseURL: "https://source.example"}
	baseRule := models.BookSourceRule{
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}
	for _, test := range []struct {
		name      string
		listRule  string
		firstBook string
	}{
		{name: "plain", listRule: ".book", firstBook: "第一本"},
		{name: "explicit keep", listRule: "+.book", firstBook: "第一本"},
		{name: "reverse", listRule: "-.book", firstBook: "第二本"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rule := baseRule
			rule.BookListRule = test.listRule
			results := parseSearchResults(doc, rule, source)
			if len(results) != 2 || results[0].Title != test.firstBook {
				t.Fatalf("unexpected %s results: %+v", test.listRule, results)
			}
		})
	}
}

func TestEffectiveExploreRuleMatchesUpstreamFallback(t *testing.T) {
	searchOnly := models.BookSourceRule{
		BookListRule:   ".search-book",
		BookNameRule:   ".search-name",
		BookURLRule:    ".search-link|attr:href",
		PaginationRule: ".search-next|attr:href",
	}
	fallback := effectiveExploreRule(searchOnly)
	if fallback.BookListRule != ".search-book" ||
		fallback.BookNameRule != ".search-name" ||
		fallback.BookURLRule != ".search-link|attr:href" ||
		fallback.PaginationRule != ".search-next|attr:href" {
		t.Fatalf("empty explore bookList should reuse search rules: %+v", fallback)
	}

	independent := searchOnly
	independent.ExploreBookListRule = ".explore-book"
	independent.ExploreBookNameRule = ".explore-name"
	independent.ExploreBookURLRule = ".explore-link|attr:data-url"
	independent.ExplorePaginationRule = ".explore-next|attr:href"
	explore := effectiveExploreRule(independent)
	if explore.BookListRule != ".explore-book" ||
		explore.BookNameRule != ".explore-name" ||
		explore.BookURLRule != ".explore-link|attr:data-url" ||
		explore.PaginationRule != ".explore-next|attr:href" {
		t.Fatalf("non-empty explore bookList should use independent explore rules: %+v", explore)
	}
}

func writeZipFile(t *testing.T, writer *zip.Writer, name string, content string) {
	t.Helper()
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}
