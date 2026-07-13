package engine

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestSourceRuleEvaluatorMatchesCoreReaderDevRuleModes(t *testing.T) {
	htmlBody := sourceCompatFixture(t, "books.html")
	jsonBody := sourceCompatFixture(t, "books.json")

	t.Run("CSS prefix and attribute syntax", func(t *testing.T) {
		document, err := newSourceRuleDocument(htmlBody)
		if err != nil {
			t.Fatal(err)
		}
		items, err := sourceRuleElements(document.Root(), "@CSS:article.book")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 {
			t.Fatalf("CSS items = %d, want 2", len(items))
		}
		name, err := sourceRuleString(items[0], "@CSS:.name@text")
		if err != nil || name != "第一本书" {
			t.Fatalf("CSS name = %q, %v", name, err)
		}
		url, err := sourceRuleString(items[0], "@CSS:.detail@href")
		if err != nil || url != "/books/one" {
			t.Fatalf("CSS href = %q, %v", url, err)
		}
		kinds, err := sourceRuleStrings(items[0], "@CSS:.kind@text")
		if err != nil || !reflect.DeepEqual(kinds, []string{"玄幻", "仙侠"}) {
			t.Fatalf("CSS kinds = %#v, %v", kinds, err)
		}
	})

	t.Run("JSONPath list and scalar fields", func(t *testing.T) {
		document, err := newSourceRuleDocument(jsonBody)
		if err != nil {
			t.Fatal(err)
		}
		items, err := sourceRuleElements(document.Root(), "$.data.books[*]")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 {
			t.Fatalf("JSONPath items = %d, want 2", len(items))
		}
		name, err := sourceRuleString(items[0], "$.name")
		if err != nil || name != "JSON 第一书" {
			t.Fatalf("JSONPath name = %q, %v", name, err)
		}
		kinds, err := sourceRuleStrings(items[0], "$.kinds[*]")
		if err != nil || !reflect.DeepEqual(kinds, []string{"玄幻", "仙侠"}) {
			t.Fatalf("JSONPath kinds = %#v, %v", kinds, err)
		}
		missing, err := sourceRuleStrings(document.Root(), "$.data.optional")
		if err != nil || len(missing) != 0 {
			t.Fatalf("JSONPath missing optional value = %#v, %v", missing, err)
		}
	})

	t.Run("XPath elements text and attributes", func(t *testing.T) {
		document, err := newSourceRuleDocument(htmlBody)
		if err != nil {
			t.Fatal(err)
		}
		items, err := sourceRuleElements(document.Root(), "@XPath://article[@class='book']")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 {
			t.Fatalf("XPath items = %d, want 2", len(items))
		}
		name, err := sourceRuleString(items[1], "@XPath:.//h2/text()")
		if err != nil || name != "第二本书" {
			t.Fatalf("XPath name = %q, %v", name, err)
		}
		url, err := sourceRuleString(items[1], "@XPath:.//a/@href")
		if err != nil || url != "/books/two" {
			t.Fatalf("XPath href = %q, %v", url, err)
		}
	})

	t.Run("regex capture and unsupported JavaScript", func(t *testing.T) {
		document, err := newSourceRuleDocument(htmlBody)
		if err != nil {
			t.Fatal(err)
		}
		items, err := sourceRuleElements(document.Root(), `:(?s)<entry>(.*?)</entry>`)
		if err != nil || len(items) != 1 {
			t.Fatalf("regex elements = %#v, %v", items, err)
		}
		value, err := sourceRuleString(items[0], "$1")
		if err != nil || value != "正则标题" {
			t.Fatalf("regex capture = %q, %v", value, err)
		}
		_, err = sourceRuleString(document.Root(), "@js:result")
		if !errors.Is(err, ErrUnsupportedSourceRule) {
			t.Fatalf("JavaScript rule error = %v, want ErrUnsupportedSourceRule", err)
		}
	})

	t.Run("relative URL is not inferred as XPath", func(t *testing.T) {
		if _, isXPath := sourceRuleXPath("/books/detail"); isXPath {
			t.Fatal("bare relative URL must not be inferred as XPath")
		}
	})

	t.Run("CSS XPath and JSONPath combined rules", func(t *testing.T) {
		htmlDocument, err := newSourceRuleDocument(htmlBody)
		if err != nil {
			t.Fatal(err)
		}
		cssItems, err := sourceRuleElements(htmlDocument.Root(), "@CSS:article.book")
		if err != nil || len(cssItems) == 0 {
			t.Fatalf("CSS list = %#v, %v", cssItems, err)
		}
		fallback, err := sourceRuleString(cssItems[0], "@CSS:.missing@text||.name@text")
		if err != nil || fallback != "第一本书" {
			t.Fatalf("CSS fallback = %q, %v", fallback, err)
		}
		combined, err := sourceRuleStrings(cssItems[0], "@CSS:.name@text&&.kind@text")
		if err != nil || !reflect.DeepEqual(combined, []string{"第一本书", "玄幻", "仙侠"}) {
			t.Fatalf("CSS combined = %#v, %v", combined, err)
		}
		interleaved, err := sourceRuleStrings(cssItems[0], "@CSS:.name@text%%.kind@text")
		if err != nil || !reflect.DeepEqual(interleaved, []string{"第一本书", "玄幻"}) {
			t.Fatalf("CSS interleaved = %#v, %v", interleaved, err)
		}
		xpath, err := sourceRuleString(htmlDocument.Root(), "@XPath://missing/text()||//article[@data-id='two']/h2/text()")
		if err != nil || xpath != "第二本书" {
			t.Fatalf("XPath fallback = %q, %v", xpath, err)
		}

		jsonDocument, err := newSourceRuleDocument(jsonBody)
		if err != nil {
			t.Fatal(err)
		}
		jsonFallback, err := sourceRuleString(jsonDocument.Root(), "$.data.absent||$.data.books[0].name")
		if err != nil || jsonFallback != "JSON 第一书" {
			t.Fatalf("JSONPath fallback = %q, %v", jsonFallback, err)
		}
		jsonCombined, err := sourceRuleStrings(jsonDocument.Root(), "$.data.books[0].name&&$.data.books[0].kinds[*]")
		if err != nil || !reflect.DeepEqual(jsonCombined, []string{"JSON 第一书", "玄幻", "仙侠"}) {
			t.Fatalf("JSONPath combined = %#v, %v", jsonCombined, err)
		}
		parts, operator := sourceRuleCompositeParts("$.data.books[?(@.name == 'JSON 第一书' && @.url == '/json/one')].name||$.data.books[1].name")
		if operator != "||" || !reflect.DeepEqual(parts, []string{
			"$.data.books[?(@.name == 'JSON 第一书' && @.url == '/json/one')].name",
			"$.data.books[1].name",
		}) {
			t.Fatalf("JSONPath nested condition split = %#v, operator = %q", parts, operator)
		}
	})
}

func TestSearchBooksExecutesReaderDevJSONPathAndXPathRules(t *testing.T) {
	jsonBody := sourceCompatFixture(t, "books.json")
	htmlBody := sourceCompatFixture(t, "books.html")

	tests := []struct {
		name  string
		body  string
		rules models.BookSourceRule
		want  []SearchResult
	}{
		{
			name: "JSONPath",
			body: jsonBody,
			rules: models.BookSourceRule{
				SearchURL:    "https://source.example/search?q={keyword}",
				BookListRule: "$.data.books[*]",
				BookNameRule: "$.name",
				BookURLRule:  "$.url",
				BookKindRule: "$.kinds[*]",
			},
			want: []SearchResult{
				{Title: "JSON 第一书", BookURL: "https://source.example/json/one", Kind: "玄幻,仙侠"},
				{Title: "JSON 第二书", BookURL: "https://source.example/json/two", Kind: "科幻"},
			},
		},
		{
			name: "XPath",
			body: htmlBody,
			rules: models.BookSourceRule{
				SearchURL:    "https://source.example/search?q={keyword}",
				BookListRule: "@XPath://article[@class='book']",
				BookNameRule: "@XPath:.//h2/text()",
				BookURLRule:  "@XPath:.//a/@href",
				BookKindRule: "@XPath:.//span[@class='kind']/text()",
			},
			want: []SearchResult{
				{Title: "第一本书", BookURL: "https://source.example/books/one", Kind: "玄幻,仙侠"},
				{Title: "第二本书", BookURL: "https://source.example/books/two", Kind: "科幻"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := SetHTTPClient(&http.Client{
				Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tt.body)),
						Header:     make(http.Header),
						Request:    request,
					}, nil
				}),
			})
			defer restore()

			source := models.BookSource{ID: 71, Name: tt.name + " 书源", BaseURL: "https://source.example", Charset: "utf-8"}
			if err := source.SetRules(tt.rules); err != nil {
				t.Fatal(err)
			}
			items, err := SearchBooks(source, "测试")
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != len(tt.want) {
				t.Fatalf("items = %#v, want %#v", items, tt.want)
			}
			for index, want := range tt.want {
				if items[index].Title != want.Title || items[index].BookURL != want.BookURL || items[index].Kind != want.Kind {
					t.Fatalf("item %d = %#v, want title=%q url=%q kind=%q", index, items[index], want.Title, want.BookURL, want.Kind)
				}
			}
		})
	}
}

func TestSearchBooksFallsBackToCurrentPageWhenBookURLRuleIsEmpty(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<article class="book"><h2 class="name">无链接详情书</h2></article>`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 72, Name: "详情回退源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "https://source.example/search?q={keyword}",
		BookListRule: ".book",
		BookNameRule: ".name",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := SearchBooks(source, "测试")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].BookURL != "https://source.example/search?q=%E6%B5%8B%E8%AF%95" {
		t.Fatalf("empty book URL fallback = %#v", items)
	}
}

func TestExploreBooksExecutesReaderDevJSONPathRules(t *testing.T) {
	jsonBody := sourceCompatFixture(t, "books.json")
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(jsonBody)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 73, Name: "JSON 探索源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ExploreURL:            "https://source.example/explore?page={page}",
		ExploreBookListRule:   "$.data.books[*]",
		ExploreBookNameRule:   "$.name",
		ExploreBookURLRule:    "$.url",
		ExploreBookKindRule:   "$.kinds[*]",
		ExplorePaginationRule: "$.data.next",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := ExploreBooksPage(source, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0].BookURL != "https://source.example/json/one" || result.Items[0].Kind != "玄幻,仙侠" {
		t.Fatalf("explore JSONPath items = %#v", result.Items)
	}
	if result.NextURL != "https://source.example/explore?page=2" || !result.HasMore {
		t.Fatalf("explore JSONPath pagination = %#v", result)
	}
}

func TestFetchBookInfoAndTOCExecutesReaderDevJSONPathRules(t *testing.T) {
	pages := map[string]string{
		"/book.json":  sourceCompatFixture(t, "book_detail.json"),
		"/toc.json":   sourceCompatFixture(t, "toc.json"),
		"/toc-2.json": sourceCompatFixture(t, "toc-2.json"),
	}
	restore := SetHTTPClient(&http.Client{Transport: sourceCompatTransport(t, pages)})
	defer restore()

	source := models.BookSource{ID: 74, Name: "JSON 详情目录源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoInitRule:          "$.book",
		BookInfoNameRule:          "$.title",
		BookInfoAuthorRule:        "$.author",
		BookInfoCoverRule:         "$.cover",
		BookInfoIntroRule:         "$.intro",
		BookInfoKindRule:          "$.kinds[*]",
		BookInfoLatestChapterRule: "$.latest",
		BookInfoUpdateTimeRule:    "$.updated",
		BookInfoWordCountRule:     "$.words",
		TOCURLRule:                "$.book.toc",
		ChapterListRule:           "$.chapters[*]",
		ChapterNameRule:           "$.title",
		ChapterURLRule:            "$.url",
		ChapterIsVIPRule:          "$.vip",
		ChapterUpdateTimeRule:     "$.updated",
		NextTOCURLRule:            "$.next",
	}); err != nil {
		t.Fatal(err)
	}

	info, chapters, err := FetchBookInfoAndTOC("/book.json", source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "JSON 详情书" || info.Author != "JSON 作者" || info.CoverURL != "https://source.example/covers/json-detail.jpg" || info.Kind != "玄幻,仙侠" || info.WordCount != "2.3万字" {
		t.Fatalf("JSONPath book info = %+v", info)
	}
	if len(chapters) != 3 || chapters[0].Title != "JSON 第一章" || chapters[1].Title != "🔒JSON 第二章" || chapters[2].URL != "https://source.example/chapters/json-3" {
		t.Fatalf("JSONPath chapters = %+v", chapters)
	}
}

func TestFetchBookInfoAndTOCExecutesReaderDevXPathRules(t *testing.T) {
	pages := map[string]string{
		"/book.html":  sourceCompatFixture(t, "book_detail.html"),
		"/toc.html":   sourceCompatFixture(t, "toc.html"),
		"/toc-2.html": sourceCompatFixture(t, "toc-2.html"),
	}
	restore := SetHTTPClient(&http.Client{Transport: sourceCompatTransport(t, pages)})
	defer restore()

	source := models.BookSource{ID: 75, Name: "XPath 详情目录源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoInitRule:          "@XPath://section[@id='detail']",
		BookInfoNameRule:          "@XPath:.//h1/text()",
		BookInfoAuthorRule:        "@XPath:.//span[@class='author']/text()",
		BookInfoCoverRule:         "@XPath:.//img[@class='cover']/@data-src",
		BookInfoIntroRule:         "@XPath:.//p[@class='intro']/text()",
		BookInfoKindRule:          "@XPath:.//span[@class='kind']/text()",
		BookInfoLatestChapterRule: "@XPath:.//span[@class='latest']/text()",
		BookInfoUpdateTimeRule:    "@XPath:.//span[@class='updated']/text()",
		BookInfoWordCountRule:     "@XPath:.//span[@class='words']/text()",
		TOCURLRule:                "@XPath://a[@id='toc']/@href",
		ChapterListRule:           "@XPath://li[@class='chapter']",
		ChapterNameRule:           "@XPath:.//a/text()",
		ChapterURLRule:            "@XPath:.//a/@href",
		ChapterIsVIPRule:          "@XPath:.//span[@class='vip']/text()",
		ChapterUpdateTimeRule:     "@XPath:.//span[@class='updated']/text()",
		NextTOCURLRule:            "@XPath://a[@id='next']/@href",
	}); err != nil {
		t.Fatal(err)
	}

	info, chapters, err := FetchBookInfoAndTOC("/book.html", source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "XPath 详情书" || info.Author != "XPath 作者" || info.CoverURL != "https://source.example/covers/xpath-detail.jpg" || info.Kind != "历史,军事" || info.WordCount != "3.2万字" {
		t.Fatalf("XPath book info = %+v", info)
	}
	if len(chapters) != 3 || chapters[0].Title != "XPath 第一章" || chapters[1].Title != "🔒XPath 第二章" || chapters[2].URL != "https://source.example/chapters/xpath-3" {
		t.Fatalf("XPath chapters = %+v", chapters)
	}
}

func TestFetchChapterContentExecutesReaderDevJSONPathContentURLAndPagination(t *testing.T) {
	pages := map[string]string{
		"/chapter.json":   sourceCompatFixture(t, "chapter.json"),
		"/content.json":   sourceCompatFixture(t, "content.json"),
		"/content-2.json": sourceCompatFixture(t, "content-2.json"),
	}
	restore := SetHTTPClient(&http.Client{Transport: sourceCompatTransport(t, pages)})
	defer restore()

	source := models.BookSource{ID: 76, Name: "JSON 正文源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentURLRule:     "$.contentUrl",
		ContentRule:        "$.payload.body",
		NextContentURLRule: "$.next",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := FetchChapterContent("/chapter.json", source)
	if err != nil {
		t.Fatal(err)
	}
	if content != "JSON 正文第一页\nJSON 正文第二页" {
		t.Fatalf("JSONPath content = %q", content)
	}
}

func TestFetchChapterContentExecutesReaderDevXPathContentURLAndPagination(t *testing.T) {
	pages := map[string]string{
		"/chapter.html":   sourceCompatFixture(t, "chapter.html"),
		"/content.html":   sourceCompatFixture(t, "content.html"),
		"/content-2.html": sourceCompatFixture(t, "content-2.html"),
	}
	restore := SetHTTPClient(&http.Client{Transport: sourceCompatTransport(t, pages)})
	defer restore()

	source := models.BookSource{ID: 78, Name: "XPath 正文源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentURLRule:     "@XPath://a[@id='content']/@href",
		ContentRule:        "@XPath://main[@id='content']/text()",
		NextContentURLRule: "@XPath://a[@id='next']/@href",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := FetchChapterContent("/chapter.html", source)
	if err != nil {
		t.Fatal(err)
	}
	if content != "XPath 正文第一页\nXPath 正文第二页" {
		t.Fatalf("XPath content = %q", content)
	}
}

func TestFetchChapterContentKeepsLegacyCSSContentURLRule(t *testing.T) {
	pages := map[string]string{
		"/chapter.html": `<a class="content-link" href="/content.html">正文</a>`,
		"/content.html": `<main class="content">CSS 正文</main>`,
	}
	restore := SetHTTPClient(&http.Client{Transport: sourceCompatTransport(t, pages)})
	defer restore()

	source := models.BookSource{ID: 79, Name: "CSS 正文地址源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentURLRule: ".content-link|attr:href",
		ContentRule:    ".content",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := FetchChapterContent("/chapter.html", source)
	if err != nil || content != "CSS 正文" {
		t.Fatalf("legacy CSS content URL = %q, %v", content, err)
	}
}

func TestBookInfoTOCAndContentRejectUnsupportedJavaScriptRules(t *testing.T) {
	pages := map[string]string{
		"/book":    `<div class="chapter"><a href="/chapter">第一章</a></div>`,
		"/toc":     `<div class="chapter"><a href="/chapter">第一章</a></div>`,
		"/chapter": `<main class="content">正文</main>`,
	}
	restore := SetHTTPClient(&http.Client{Transport: sourceCompatTransport(t, pages)})
	defer restore()

	base := models.BookSource{ID: 77, Name: "脚本规则源", BaseURL: "https://source.example", Charset: "utf-8"}
	t.Run("book info", func(t *testing.T) {
		source := base
		if err := source.SetRules(models.BookSourceRule{
			BookInfoNameRule: "@js:book.name",
			ChapterListRule:  ".chapter",
			ChapterNameRule:  "a|text",
			ChapterURLRule:   "a|attr:href",
		}); err != nil {
			t.Fatal(err)
		}
		_, _, err := FetchBookInfoAndTOC("/book", source)
		if !errors.Is(err, ErrUnsupportedSourceRule) {
			t.Fatalf("book info JavaScript error = %v", err)
		}
	})
	t.Run("toc", func(t *testing.T) {
		source := base
		if err := source.SetRules(models.BookSourceRule{
			TOCURLRule:      "/toc",
			ChapterListRule: "@js:chapters",
		}); err != nil {
			t.Fatal(err)
		}
		_, err := ParseTOC("/book", source)
		if !errors.Is(err, ErrUnsupportedSourceRule) {
			t.Fatalf("toc JavaScript error = %v", err)
		}
	})
	t.Run("content", func(t *testing.T) {
		source := base
		if err := source.SetRules(models.BookSourceRule{ContentRule: "@js:content"}); err != nil {
			t.Fatal(err)
		}
		_, err := FetchChapterContent("/chapter", source)
		if !errors.Is(err, ErrUnsupportedSourceRule) {
			t.Fatalf("content JavaScript error = %v", err)
		}
	})
}

func sourceCompatTransport(t *testing.T, pages map[string]string) http.RoundTripper {
	t.Helper()
	return contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, ok := pages[request.URL.Path]
		if !ok {
			t.Fatalf("unexpected source fixture request: %s", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})
}

func sourceCompatFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "source_compat", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
