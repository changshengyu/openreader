package engine

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestSearchBooksPageExecutesUpstreamPostFormOptions(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", request.Method)
			}
			if request.URL.String() != "https://source.example/search" {
				t.Fatalf("url = %s", request.URL.String())
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != "key=%E4%B8%AD%E6%96%87+%E4%B9%A6&page=2" {
				t.Fatalf("unexpected form body: %s", body)
			}
			if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded; charset=utf-8" ||
				request.Header.Get("X-Search-Page") != "2" ||
				request.Header.Get("X-Source-Token") != "source-secret" {
				t.Fatalf("unexpected request headers: %v", request.Header)
			}
			return searchPaginationResponse(request, `
				<article class="book">
					<a class="name" href="/book/2">POST 分页书籍</a>
				</article>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 3, Name: "POST 搜索源", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    `https://source.example/search, {"method":"POST","body":"key={keyword}&page={page}","headers":{"X-Search-Page":"{page}"}}`,
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
		Headers:      map[string]string{"X-Source-Token": "source-secret"},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SearchBooksPage(source, "中文 书", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "POST 分页书籍" {
		t.Fatalf("unexpected POST search result: %+v", result)
	}
}

func TestPrepareSourceRequestKeepsJSONKeywordUnescaped(t *testing.T) {
	request, err := prepareSourceRequest(
		`https://source.example/search, {"method":"POST","body":{"keyword":"{keyword}","page":"{page}"},"headers":"{\"Content-Type\":\"application/json\",\"X-Keyword\":\"{keyword}\"}"}`,
		"中文 书",
		3,
		"utf-8",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != http.MethodPost ||
		request.Body != `{"keyword":"中文 书","page":"3"}` ||
		request.Headers["Content-Type"] != "application/json" ||
		request.Headers["X-Keyword"] != "中文 书" {
		t.Fatalf("unexpected JSON request: %+v", request)
	}
}

func TestSplitSourceURLOptionLeavesOrdinaryCommasInURL(t *testing.T) {
	raw := `https://source.example/search?categories=1,2,3, {"method":"POST","body":"key={keyword}"}`
	urlPart, optionPart := splitSourceURLOption(raw)
	if urlPart != "https://source.example/search?categories=1,2,3" ||
		!strings.Contains(optionPart, `"method":"POST"`) {
		t.Fatalf("unexpected split: url=%q option=%q", urlPart, optionPart)
	}
}

func TestExploreBooksPageExecutesRelativePostOptions(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Method != http.MethodPost ||
				request.URL.String() != "https://source.example/api/explore" ||
				string(body) != "page=2" ||
				request.Header.Get("X-Explore") != "2" {
				t.Fatalf("unexpected explore request: %s %s body=%s headers=%v", request.Method, request.URL, body, request.Header)
			}
			return searchPaginationResponse(request, `
				<article class="explore-book">
					<a class="name" href="/book/explore-2">书海 POST 第二页</a>
				</article>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 4, Name: "POST 书海源", BaseURL: "https://source.example/root/", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ExploreURL:          `/api/explore, {"method":"POST","body":"page={page}","headers":{"X-Explore":"{page}"}}`,
		ExploreBookListRule: ".explore-book",
		ExploreBookNameRule: ".name",
		ExploreBookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := ExploreBooksPage(source, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Page != 2 || len(result.Items) != 1 ||
		result.Items[0].BookURL != "https://source.example/book/explore-2" {
		t.Fatalf("unexpected explore POST result: %+v", result)
	}
}

func TestSearchBooksResolvesRelativeURLBeforeOptions(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() != "https://source.example/api/search?q=%E6%B5%8B%E8%AF%95" {
				t.Fatalf("relative search URL was not resolved: %s", request.URL)
			}
			return searchPaginationResponse(request, `
				<article class="book"><a class="name" href="/book/1">相对搜索地址</a></article>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 5, Name: "相对地址源", BaseURL: "https://source.example/root/", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "/api/search?q={keyword}",
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchBooks(source, "测试"); err != nil {
		t.Fatal(err)
	}
}
