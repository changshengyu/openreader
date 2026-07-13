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

func sourceCompatFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "source_compat", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
