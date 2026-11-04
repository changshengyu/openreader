package engine

import (
	"fmt"
	"strings"
	"testing"
)

// wantZhangHuiRule is the OpenReader rule -19 "目录(章回、话)" pinned by
// docs/compat/txt-toc-rule-selection-fixed-baseline-second-audit-p1-contract.md
// section 6.1: rule -2 "目录" plus a 第x回/第x话 branch that may follow a
// 第x章 (节、卷、集、部、篇) prefix on the same line.
const wantZhangHuiRule = `^[ 　\t]{0,4}(?:序章|序言|卷首语|扉页|楔子|正文(?!完|结)|终章|后记|尾声|番外|第?\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张))|(?:第\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:章|节(?!课)|卷|集(?![合和])|部(?![分赛游])|篇(?!张)).{0,30}?[ 　\t])?第\s{0,4}[\d〇零一二两三四五六七八九十百千万壹贰叁肆伍陆柒捌玖拾佰仟]+?\s{0,4}(?:回(?![合来事去])|话)).{0,30}$`

// zhangHuiBody returns well over 1000 characters of prose that no default
// rule matches, so consecutive headings are far enough apart for Legado's
// spaced count.
func zhangHuiBody(label string) string {
	var body strings.Builder
	for paragraph := 1; paragraph <= 30; paragraph++ {
		fmt.Fprintf(&body, "　　%s第%d段：山风穿过松林，远处的钟声一下一下地敲着，少年把剑背在身后，沿着石阶慢慢往上走。\n", label, paragraph)
	}
	return body.String()
}

func zhangHuiBook(headings ...string) string {
	var text strings.Builder
	for index, heading := range headings {
		text.WriteString(heading)
		text.WriteString("\n")
		text.WriteString(zhangHuiBody(fmt.Sprintf("甲%d", index+1)))
	}
	return text.String()
}

func zhangHuiPrefix(value string, runes int) string {
	letters := []rune(value)
	if len(letters) > runes {
		letters = letters[:runes]
	}
	return string(letters)
}

func detectZhangHuiCatalog(t *testing.T, text string) ([]TXTChapter, string) {
	t.Helper()
	chapters, rule, err := parseTXTCatalogWithLimits([]byte(text), "", LegacyLocalBookParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	return chapters, rule
}

func zhangHuiTitles(chapters []TXTChapter) []string {
	titles := make([]string, 0, len(chapters))
	for _, chapter := range chapters {
		titles = append(titles, chapter.Title)
	}
	return titles
}

func assertZhangHuiTitles(t *testing.T, chapters []TXTChapter, want []string) {
	t.Helper()
	got := zhangHuiTitles(chapters)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("catalogue titles\n got (%d): %q\nwant (%d): %q", len(got), got, len(want), want)
	}
}

func upstreamDirectoryRule(t *testing.T) string {
	t.Helper()
	return findTXTTocRule(t, DefaultTXTTocRules(), -1).Rule
}

// T1 (D1): the reported book. The first 回 of every 章 shares the 章 line,
// later 回 have their own "第x回" line.
func TestParseTXTSplitsEveryHuiWhenTheFirstHuiSharesTheZhangLine(t *testing.T) {
	headings := []string{
		"第一章 风起青萍 第一回 少年初入江湖路",
		"第二回 古寺夜话",
		"第三回 剑出霜寒",
		"第四回 别过故人",
		"第二章 云涌四海 第五回 长街初遇",
		"第六回 雨夜追踪",
		"第七回 渡口斗智",
		"第八回 灯下论剑",
	}
	chapters, rule := detectZhangHuiCatalog(t, zhangHuiBook(headings...))
	assertZhangHuiTitles(t, chapters, headings)
	if rule != wantZhangHuiRule {
		t.Fatalf("automatic detection must select rule -19 for a 章回 book, got %q", rule)
	}
	if chapters[0].Start != 0 || !strings.HasPrefix(chapters[1].Content, "甲2第1段") {
		t.Fatalf("回 chapters must keep their own body: first start=%d second=%q", chapters[0].Start, zhangHuiPrefix(chapters[1].Content, 20))
	}
}

// T2 (D3): the same layout when the shared 章回 line is longer than the
// upstream 30-character tail.
func TestParseTXTRecognisesLongSharedZhangHuiLines(t *testing.T) {
	headings := []string{
		"第一章 风起青萍之末浪成于微澜之间 第一回 少年初入江湖路 古寺夜半听钟声",
		"第二回 古寺夜话",
		"第三回 剑出霜寒",
		"第二章 云涌四海八荒六合尽入眼底 第四回 长街初遇故人 雨夜追踪旧事",
		"第五回 渡口斗智",
		"第六回 灯下论剑",
	}
	chapters, rule := detectZhangHuiCatalog(t, zhangHuiBook(headings...))
	assertZhangHuiTitles(t, chapters, headings)
	if rule != wantZhangHuiRule {
		t.Fatalf("automatic detection must select rule -19, got %q", rule)
	}
}

// T3 (D2): a classical novel with only 第x回 headings.
func TestParseTXTSplitsClassicalHuiOnlyNovels(t *testing.T) {
	headings := []string{
		"第一回 青石镇少年拜师 白云观老道传经",
		"第二回 夜半钟声惊旧梦 江心月色照归舟",
		"第三回 三入深山寻古剑 一封书信定前盟",
		"第四回 风雪关前逢故旧 灯花帐里论英雄",
		"第五回 渡口斗智脱险境 长亭送别诉衷肠",
	}
	chapters, rule := detectZhangHuiCatalog(t, zhangHuiBook(headings...))
	assertZhangHuiTitles(t, chapters, headings)
	if rule != wantZhangHuiRule {
		t.Fatalf("automatic detection must select rule -19, got %q", rule)
	}
}

// T4 (D4): light novels numbered with 第x话.
func TestParseTXTSplitsHuaNumberedLightNovels(t *testing.T) {
	headings := []string{
		"第1话 转学生",
		"第2话 社团招新",
		"第3话 雨天的约定",
		"第4话 文化祭前夜",
		"第5话 告白",
	}
	chapters, rule := detectZhangHuiCatalog(t, zhangHuiBook(headings...))
	assertZhangHuiTitles(t, chapters, headings)
	if rule != wantZhangHuiRule {
		t.Fatalf("automatic detection must select rule -19, got %q", rule)
	}
}

// T5 (D5): negative lookaheads only constrain the character right after the
// unit, as Java does, both for the upstream rules and for rule -19.
func TestTXTNegativeLookaheadsOnlyCheckTheCharacterAfterTheUnit(t *testing.T) {
	directory := findTXTTocRule(t, DefaultTXTTocRules(), -2).Rule
	t.Run("upstream rule accepts excluded words later in the title", func(t *testing.T) {
		chapters, err := ParseTXTWithRule([]byte(strings.Join([]string{
			"第一章 部分真相",
			"内容甲。",
			"第二章 集合",
			"内容乙。",
			"第三章 一节课",
			"内容丙。",
			"第四章 正文完结之后",
			"内容丁。",
		}, "\n")), directory)
		if err != nil {
			t.Fatal(err)
		}
		assertZhangHuiTitles(t, chapters, []string{"第一章 部分真相", "第二章 集合", "第三章 一节课", "第四章 正文完结之后"})
	})
	t.Run("upstream rule still rejects excluded words right after the unit", func(t *testing.T) {
		chapters, err := ParseTXTWithRule([]byte(strings.Join([]string{
			"第一章 开始",
			"第一部分 总论",
			"第一节课外讲义",
			"正文完结说明",
			"第二章 继续",
			"内容。",
		}, "\n")), directory)
		if err != nil {
			t.Fatal(err)
		}
		assertZhangHuiTitles(t, chapters, []string{"第一章 开始", "第二章 继续"})
	})
	t.Run("rule -19 accepts 回来 in a title and rejects 回来/回合 after the unit", func(t *testing.T) {
		chapters, err := ParseTXTWithRule([]byte(strings.Join([]string{
			"第三回 宝玉回来",
			"内容甲。",
			"第一回来人说道",
			"第二回合开始了",
			"第四回 黛玉进府",
			"内容乙。",
		}, "\n")), wantZhangHuiRule)
		if err != nil {
			t.Fatal(err)
		}
		assertZhangHuiTitles(t, chapters, []string{"第三回 宝玉回来", "第四回 黛玉进府"})
		if !strings.Contains(chapters[0].Content, "第一回来人说道") || !strings.Contains(chapters[0].Content, "第二回合开始了") {
			t.Fatalf("回来/回合 lines must stay in the body: %q", chapters[0].Content)
		}
	})
}

// T7: rule -19 takes over with two more spaced headings than every upstream
// rule; C4 below pins the one-more side.
func TestParseTXTZhangHuiRuleTakesOverWithTwoExtraHeadings(t *testing.T) {
	headings := []string{
		"第一章 初入江湖",
		"第二章 古寺夜话",
		"第一回 夜访",
		"第二回 斗剑",
		"第三章 剑出霜寒",
	}
	chapters, rule := detectZhangHuiCatalog(t, zhangHuiBook(headings...))
	assertZhangHuiTitles(t, chapters, headings)
	if rule != wantZhangHuiRule {
		t.Fatalf("two extra spaced 回 headings must select rule -19, got %q", rule)
	}
}

// C1: an ordinary 章 novel keeps reader-dev's rule and catalogue.
func TestParseTXTOrdinaryZhangNovelKeepsUpstreamRule(t *testing.T) {
	headings := []string{"第一章 初入江湖", "第二章 古寺夜话", "第三章 剑出霜寒", "第四章 别过故人", "第五章 长街初遇"}
	chapters, rule := detectZhangHuiCatalog(t, zhangHuiBook(headings...))
	assertZhangHuiTitles(t, chapters, headings)
	if rule != upstreamDirectoryRule(t) {
		t.Fatalf("an ordinary 章 novel must keep rule -1, got %q", rule)
	}
}

// C2 (layout a): one short 章回 line per chapter is already a full-line
// title under rule -1.
func TestParseTXTShortSharedZhangHuiLinesKeepUpstreamRule(t *testing.T) {
	headings := []string{
		"第一章 风起 第一回 少年初入江湖路",
		"第二章 云涌 第二回 古寺夜话",
		"第三章 霜寒 第三回 剑出鞘",
		"第四章 故人 第四回 别过",
		"第五章 长街 第五回 初遇",
	}
	chapters, rule := detectZhangHuiCatalog(t, zhangHuiBook(headings...))
	assertZhangHuiTitles(t, chapters, headings)
	if rule != upstreamDirectoryRule(t) {
		t.Fatalf("short shared 章回 lines must keep rule -1, got %q", rule)
	}
}

// C3 (D6, layout c): a 章 line directly followed by its 回 line keeps the
// 章 title with the 回 line at the start of the body.
func TestParseTXTZhangLineFollowedByHuiLineKeepsUpstreamRule(t *testing.T) {
	var text strings.Builder
	want := make([]string, 0, 5)
	for index, number := range []string{"一", "二", "三", "四", "五"} {
		zhang := fmt.Sprintf("第%s章 卷名%s", number, number)
		want = append(want, zhang)
		fmt.Fprintf(&text, "%s\n第%s回 回名%s\n%s", zhang, number, number, zhangHuiBody(fmt.Sprintf("乙%d", index+1)))
	}
	chapters, rule := detectZhangHuiCatalog(t, text.String())
	assertZhangHuiTitles(t, chapters, want)
	if rule != upstreamDirectoryRule(t) {
		t.Fatalf("adjacent 章/回 lines must keep rule -1, got %q", rule)
	}
	for index, number := range []string{"一", "二", "三", "四", "五"} {
		if !strings.HasPrefix(chapters[index].Content, fmt.Sprintf("第%s回 回名%s", number, number)) {
			t.Fatalf("chapter %d must start with its 回 line, got %q", index, zhangHuiPrefix(chapters[index].Content, 20))
		}
	}
}

// C4 (D7): one prose paragraph that happens to start with 第一回 does not
// switch a 章 novel to rule -19.
func TestParseTXTSingleStrayHuiParagraphKeepsUpstreamRule(t *testing.T) {
	stray := "第一回见到她，是在城南的旧书店里。"
	text := "第一章 初入江湖\n" + zhangHuiBody("丙1") +
		"第二章 古寺夜话\n" + zhangHuiBody("丙2") +
		"第三章 剑出霜寒\n" + zhangHuiBody("丙3") + stray + "\n" + zhangHuiBody("丙3续") +
		"第四章 别过故人\n" + zhangHuiBody("丙4") +
		"第五章 长街初遇\n" + zhangHuiBody("丙5")
	chapters, rule := detectZhangHuiCatalog(t, text)
	assertZhangHuiTitles(t, chapters, []string{"第一章 初入江湖", "第二章 古寺夜话", "第三章 剑出霜寒", "第四章 别过故人", "第五章 长街初遇"})
	if rule != upstreamDirectoryRule(t) {
		t.Fatalf("a single stray 第一回 paragraph must keep rule -1, got %q", rule)
	}
	if !strings.Contains(chapters[2].Content, stray) {
		t.Fatalf("the stray paragraph must stay in chapter 3: %q", chapters[2].Content)
	}
}

// C5: a table of contents at the start of an ordinary 章 novel keeps
// reader-dev's rule.
func TestParseTXTLeadingTableOfContentsKeepsUpstreamRule(t *testing.T) {
	headings := []string{"第一章 初入江湖", "第二章 古寺夜话", "第三章 剑出霜寒", "第四章 别过故人", "第五章 长街初遇"}
	text := "目录\n" + strings.Join(headings, "\n") + "\n\n" + zhangHuiBook(headings...)
	_, rule := detectZhangHuiCatalog(t, text)
	if rule != upstreamDirectoryRule(t) {
		t.Fatalf("a leading table of contents must keep rule -1, got %q", rule)
	}
}
