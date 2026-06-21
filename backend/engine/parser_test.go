package engine

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

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

func TestParseTXTSkipsShortFrontMatterBeforeFirstChapter(t *testing.T) {
	input := []byte("测试书名\n作者：某人\n分类：仙侠\n\n序章、剑宗少年\n序章正文。\n第一章、缘起\n第一章正文。\n第四十一章 夺异宝\n第四十一章正文。")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "序章、剑宗少年" {
		t.Fatalf("front matter was not skipped, first title: %q", chapters[0].Title)
	}
	if chapters[1].Title != "第一章、缘起" {
		t.Fatalf("unexpected second title: %q", chapters[1].Title)
	}
	if chapters[2].Title != "第四十一章 夺异宝" {
		t.Fatalf("unexpected third title: %q", chapters[2].Title)
	}
}

func TestParseTXTDetectsUpstreamDefaultTitleRules(t *testing.T) {
	input := []byte("1、初见\n第一节正文。\n2、再会\n第二节正文。")

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
	input := []byte("正文前说明\n第一章 起始\n第一章正文。\n第二章 转折\n第二章正文。")

	chapters, err := ParseTXTWithRule(input, `(?<=[　\s])第?\s{0,4}[\d〇零一二两三四五六七八九十百千万]+?\s{0,4}章.{0,30}$`)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d: %+v", len(chapters), chapters)
	}
	if chapters[0].Title != "第一章 起始" || chapters[1].Title != "第二章 转折" {
		t.Fatalf("unexpected chapters: %+v", chapters)
	}
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
	writeZipFile(t, zipWriter, "OEBPS/chapter1.xhtml", `<html><body><h1>第一章</h1><p>第一章正文。</p></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/chapter2.xhtml", `<html><body><h1>第二章</h1><p>第二章正文。</p></body></html>`)
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
	tocFallback, err := ParseEPUBWithRule(buffer.Bytes(), "toc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tocFallback.Chapters) != 2 || tocFallback.Chapters[0].Title != "第一章" {
		t.Fatalf("toc rule should fall back to readable spine when toc is missing: %#v", tocFallback.Chapters)
	}
}

func TestParseEPUBWithRuleCombinesSpineAndNav(t *testing.T) {
	data := testEPUBWithNav(t)

	tests := []struct {
		rule   string
		titles []string
		bodies []string
	}{
		{rule: "spin", titles: []string{"正文一", "正文二"}, bodies: []string{"第一章内容。", "第二章内容。"}},
		{rule: "spin<toc", titles: []string{"目录一", "目录二"}, bodies: []string{"第一章内容。", "第二章内容。"}},
		{rule: "toc", titles: []string{"目录二", "目录一"}, bodies: []string{"第二章内容。", "第一章内容。"}},
		{rule: "toc<spin", titles: []string{"正文二", "正文一"}, bodies: []string{"第二章内容。", "第一章内容。"}},
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
			}
		})
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
  </manifest>
  <spine><itemref idref="one"/><itemref idref="two"/></spine>
</package>`)
	writeZipFile(t, zipWriter, "OEBPS/nav.xhtml", `<html><body><nav epub:type="toc">
  <ol><li><a href="text/two.xhtml#start">目录二</a></li><li><a href="text/one.xhtml">目录一</a></li></ol>
</nav></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/text/one.xhtml", `<html><body><h1>正文一</h1><p>第一章内容。</p></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/text/two.xhtml", `<html><body><h1>正文二</h1><p>第二章内容。</p></body></html>`)
	if err := zipWriter.Close(); err != nil {
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
