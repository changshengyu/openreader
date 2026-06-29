package engine

import (
	"strings"
	"testing"
)

func TestParseRSSRuleArticlesSupportsCommonUpstreamRules(t *testing.T) {
	body := `
		<section>
			<article class="entry">
				<a class="title" href="/first">第一篇</a>
				<time datetime="2026-06-20T10:00:00Z"></time>
				<div class="summary"><b>摘要一</b><script>alert(1)</script></div>
				<img data-src="/images/first.jpg">
			</article>
			<article class="entry">
				<a class="title" href="/second">第二篇</a>
				<time datetime="2026-06-20T11:00:00Z"></time>
				<div class="summary">摘要二</div>
				<img data-src="/images/second.jpg">
			</article>
		</section>`
	rows, err := ParseRSSRuleArticles(body, "https://rss.example/list", RSSRuleSet{
		Articles:    "-//article[@class='entry']",
		Title:       ".title|text",
		PubDate:     "time@datetime",
		Description: ".summary|html",
		Image:       "img@data-src",
		Link:        ".title@href",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Title != "第二篇" || rows[0].Link != "https://rss.example/second" || rows[0].Image != "https://rss.example/images/second.jpg" {
		t.Fatalf("unexpected reversed first row: %+v", rows[0])
	}
	if rows[1].PubDate != "2026-06-20T10:00:00Z" || !strings.Contains(rows[1].Description, "<b>摘要一</b>") {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestSanitizeRSSHTMLRemovesActiveContentAndResolvesURLs(t *testing.T) {
	value := SanitizeRSSHTML(`
		<p onclick="steal()">正文 <a href="/next">下一页</a></p>
		<img src="../cover.jpg" onerror="steal()">
		<script>alert(1)</script>
		<a href="javascript:alert(1)">危险链接</a>`,
		"https://rss.example/articles/1",
	)
	for _, forbidden := range []string{"onclick", "onerror", "<script", "javascript:"} {
		if strings.Contains(strings.ToLower(value), forbidden) {
			t.Fatalf("sanitized HTML still contains %q: %s", forbidden, value)
		}
	}
	if !strings.Contains(value, `href="https://rss.example/next"`) ||
		!strings.Contains(value, `src="https://rss.example/cover.jpg"`) {
		t.Fatalf("relative URLs were not resolved: %s", value)
	}
}

func TestExtractRSSFirstImageMatchesUpstreamEmbeddedImageFallback(t *testing.T) {
	if value := ExtractRSSFirstImageSource(`<p>摘要</p><img src="../covers/raw.jpg">`); value != "../covers/raw.jpg" {
		t.Fatalf("raw first embedded image = %q", value)
	}
	value := ExtractRSSFirstImage(`
		<p>摘要</p>
		<img src="../covers/first.jpg">
		<img src="/covers/second.jpg">`,
		"https://rss.example/feeds/news.xml",
	)
	if value != "https://rss.example/covers/first.jpg" {
		t.Fatalf("first embedded image = %q", value)
	}
	if value := ExtractRSSFirstImage(`<img src="javascript:alert(1)">`, "https://rss.example/feed.xml"); value != "" {
		t.Fatalf("unsafe embedded image = %q", value)
	}
}

func TestParseRSSRuleArticlesReadsXMLLinkText(t *testing.T) {
	rows, err := ParseRSSRuleArticles(
		`<rss><channel><item><title>XML 文章</title><link>https://rss.example/xml</link></item></channel></rss>`,
		"https://rss.example/feed.xml",
		RSSRuleSet{Articles: "//channel/item", Title: "title", Link: "link"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Title != "XML 文章" || rows[0].Link != "https://rss.example/xml" {
		t.Fatalf("unexpected XML rule result: %+v", rows)
	}
}

func TestParseRSSRulePageResolvesNextRequestOptions(t *testing.T) {
	page, err := ParseRSSRulePage(
		`<main>
			<article><a class="title" href="/post/1">第一页</a></article>
			<a class="next" data-url='/list, {"method":"POST","body":"page=2"}'>下一页</a>
		</main>`,
		"https://rss.example/news/page/1",
		RSSRuleSet{Articles: "article", Title: ".title", Link: ".title@href"},
		".next@data-url",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Articles) != 1 || page.Articles[0].Link != "https://rss.example/post/1" {
		t.Fatalf("unexpected page articles: %+v", page.Articles)
	}
	if page.NextURL != `https://rss.example/list, {"method":"POST","body":"page=2"}` {
		t.Fatalf("next request options were not preserved: %q", page.NextURL)
	}
}

func TestParseRSSRulePageResolvesArticleLinksAgainstSourceURL(t *testing.T) {
	page, err := ParseRSSRulePage(
		`<article><a href="../post/1">文章</a><img src="../cover.jpg"></article><a class="next" href="../page/2">下一页</a>`,
		"https://cdn.rss.example/categories/tech/page/1",
		RSSRuleSet{
			Articles:    "article",
			Title:       "a",
			Image:       "img@src",
			Link:        "a@href",
			LinkBaseURL: `https://rss.example/feeds/main.xml, {"headers":{"Referer":"https://rss.example/"}}`,
		},
		".next@href",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Articles) != 1 {
		t.Fatalf("article count = %d", len(page.Articles))
	}
	if page.Articles[0].Link != "https://rss.example/post/1" {
		t.Fatalf("article link = %q", page.Articles[0].Link)
	}
	if page.Articles[0].Image != "https://cdn.rss.example/categories/tech/cover.jpg" {
		t.Fatalf("article image = %q", page.Articles[0].Image)
	}
	if page.NextURL != "https://cdn.rss.example/categories/tech/page/2" {
		t.Fatalf("next page URL = %q", page.NextURL)
	}
}

func TestParseRSSRulePageSupportsPageMode(t *testing.T) {
	page, err := ParseRSSRulePage(
		`<article><a href="/post/1">文章</a></article>`,
		"https://rss.example/list?page=1",
		RSSRuleSet{Articles: "article", Title: "a", Link: "a@href"},
		"PAGE",
	)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextURL != "https://rss.example/list?page=1" {
		t.Fatalf("PAGE next URL = %q", page.NextURL)
	}
}
