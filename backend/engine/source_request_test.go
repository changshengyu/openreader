package engine

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

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
					<span class="kind">玄幻</span><span class="kind">热血</span>
					<span class="words">12345</span>
				</article>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 3, Name: "POST 搜索源", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:         `https://source.example/search, {"method":"POST","body":"key={keyword}&page={page}","headers":{"X-Search-Page":"{page}"}}`,
		BookListRule:      ".book",
		BookNameRule:      ".name",
		BookKindRule:      ".kind",
		BookWordCountRule: ".words",
		BookURLRule:       ".name|attr:href",
		Headers:           map[string]string{"X-Source-Token": "source-secret"},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SearchBooksPage(source, "中文 书", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "POST 分页书籍" ||
		result.Items[0].Kind != "玄幻,热血" || result.Items[0].WordCount != "1.2万字" {
		t.Fatalf("unexpected POST search result: %+v", result)
	}
}

func TestSearchBooksPageExecutesUpstreamRetryOption(t *testing.T) {
	attempts := 0
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader("temporary")),
					Header:     make(http.Header),
					Request:    request,
				}, nil
			}
			return searchPaginationResponse(request, `
				<article class="book">
					<a class="name" href="/book/retried">重试成功</a>
				</article>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 5, Name: "重试搜索源", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    `https://source.example/search?key={keyword}, {"retry":1}`,
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SearchBooksPage(source, "重试", 1)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || len(result.Items) != 1 || result.Items[0].Title != "重试成功" {
		t.Fatalf("retry option did not reach search execution: attempts=%d result=%+v", attempts, result)
	}
}

func TestSearchBooksPageUsesSourceCharsetForRequestAndResponse(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.RawQuery != "key=%D6%D0%CE%C4+%CA%E9" {
				t.Fatalf("GBK search query = %q", request.URL.RawQuery)
			}
			body, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(`
				<article class="book">
					<a class="name" href="/book/gbk">中文书名</a>
				</article>
			`))
			if err != nil {
				t.Fatal(err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 6, Name: "GBK 搜索源", Charset: "gbk"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    `https://source.example/search?key={keyword}`,
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SearchBooksPage(source, "中文 书", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Title != "中文书名" {
		t.Fatalf("GBK search result = %+v", result)
	}
}

func TestSearchBooksPageAppliesSourceConcurrentRate(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return searchPaginationResponse(request, `
				<article class="book">
					<a class="name" href="/book/rate">限速书籍</a>
				</article>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{
		ID:             77,
		Name:           "限速搜索源",
		BaseURL:        "https://rate-source.example",
		Charset:        "utf-8",
		ConcurrentRate: "60",
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "https://rate-source.example/search?key={keyword}",
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchBooksPage(source, "限速", 1); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := SearchBooksPage(source, "限速", 1); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 45*time.Millisecond {
		t.Fatalf("book source concurrentRate did not reach request execution: %v", time.Since(started))
	}
}

func TestSearchBooksPageUsesBookURLPatternForDirectDetail(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Method != http.MethodPost || string(body) != "id=1" || request.Header.Get("X-Detail") != "yes" {
				t.Fatalf("direct detail request = %s body=%s headers=%v", request.Method, body, request.Header)
			}
			return searchPaginationResponse(request, `
				<h1 class="detail-name">直接详情书</h1>
				<span class="detail-author">详情作者</span>
				<span class="detail-last">最新章</span>
				<span class="detail-kind">科幻</span>
				<span class="detail-words">9999</span>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{
		ID:             88,
		Name:           "直接详情源",
		BaseURL:        "https://source.example",
		BookURLPattern: `https://source\.example/detail/\d+`,
		SourceType:     1,
		Charset:        "utf-8",
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:                 `https://source.example/detail/1, {"method":"POST","body":"id=1","headers":{"X-Detail":"yes"}}`,
		BookListRule:              ".missing-list",
		BookInfoNameRule:          ".detail-name",
		BookInfoAuthorRule:        ".detail-author",
		BookInfoLatestChapterRule: ".detail-last",
		BookInfoKindRule:          ".detail-kind",
		BookInfoWordCountRule:     ".detail-words",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SearchBooksPage(source, "ignored", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 ||
		result.Items[0].Title != "直接详情书" ||
		result.Items[0].Author != "详情作者" ||
		result.Items[0].LatestChapter != "最新章" ||
		result.Items[0].Kind != "科幻" ||
		result.Items[0].WordCount != "9999字" ||
		result.Items[0].Type != 1 ||
		!strings.Contains(result.Items[0].BookURL, `"body":"id=1"`) ||
		!strings.Contains(result.Items[0].BookURL, `"X-Detail":"yes"`) {
		t.Fatalf("direct detail result = %+v", result)
	}
}

func TestSearchBooksPageFallsBackToDetailOnlyWithoutPattern(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return searchPaginationResponse(request, `<h1 class="detail-name">空列表详情</h1>`), nil
		}),
	})
	defer restore()

	newSource := func(pattern string) models.BookSource {
		source := models.BookSource{
			Name:           "空列表源",
			BaseURL:        "https://source.example",
			BookURLPattern: pattern,
			Charset:        "utf-8",
		}
		if err := source.SetRules(models.BookSourceRule{
			SearchURL:        "https://source.example/search",
			BookInfoNameRule: ".detail-name",
		}); err != nil {
			t.Fatal(err)
		}
		return source
	}

	fallback, err := SearchBooksPage(newSource(""), "ignored", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(fallback.Items) != 1 || fallback.Items[0].Title != "空列表详情" {
		t.Fatalf("empty list detail fallback = %+v", fallback)
	}

	noFallback, err := SearchBooksPage(newSource(`/detail/\d+$`), "ignored", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(noFallback.Items) != 0 {
		t.Fatalf("non-matching pattern must disable detail fallback: %+v", noFallback)
	}
}

func TestSearchBooksPageRejectsInvalidBookURLPattern(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return searchPaginationResponse(request, `<html></html>`), nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "坏正则源", BookURLPattern: "[", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{SearchURL: "https://source.example/search"}); err != nil {
		t.Fatal(err)
	}
	_, err := SearchBooksPage(source, "ignored", 1)
	if err == nil || !strings.Contains(err.Error(), "invalid book URL pattern") {
		t.Fatalf("invalid pattern error = %v", err)
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

func TestPrepareSourceRequestSupportsUpstreamPageChoices(t *testing.T) {
	for _, test := range []struct {
		page int
		url  string
		body string
	}{
		{page: 1, url: "https://source.example/list?offset=0", body: "page=first"},
		{page: 2, url: "https://source.example/list?offset=20", body: "page=second"},
		{page: 4, url: "https://source.example/list?offset=40", body: "page=last"},
	} {
		request, err := PrepareSourceRequest(
			`https://source.example/list?offset=<0,20,40>, {"method":"POST","body":"page=<first,second,last>"}`,
			"",
			test.page,
			"utf-8",
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if request.URL != test.url || request.Body != test.body {
			t.Fatalf("page %d request = %+v, want url=%q body=%q", test.page, request, test.url, test.body)
		}
	}
}

func TestPrepareSourceRequestUsesConfiguredCharsetForFields(t *testing.T) {
	request, err := PrepareSourceRequest(
		`https://source.example/search?key={keyword}&kept=%D2%D1, {"method":"POST","charset":"gbk","body":"key={keyword}&kept=%D2%D1"}`,
		"中文 书",
		1,
		"utf-8",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.URL != "https://source.example/search?key=%D6%D0%CE%C4+%CA%E9&kept=%D2%D1" ||
		request.Body != "key=%D6%D0%CE%C4+%CA%E9&kept=%D2%D1" {
		t.Fatalf("GBK request fields were not encoded like upstream: %+v", request)
	}
}

func TestPrepareSourceRequestSupportsEscapeCharset(t *testing.T) {
	request, err := PrepareSourceRequest(
		`https://source.example/search?key={keyword}, {"charset":"escape"}`,
		"中文😀",
		1,
		"utf-8",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.URL != "https://source.example/search?key=%u4e2d%u6587%ud83d%ude00" {
		t.Fatalf("escape request field = %q", request.URL)
	}
}

func TestPrepareSourceRequestPreservesRetryAndBinaryType(t *testing.T) {
	request, err := PrepareSourceRequest(
		`https://source.example/binary, {"method":"POST","retry":"2","type":"image/png"}`,
		"",
		1,
		"utf-8",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Retry != 2 || request.Type != "image/png" {
		t.Fatalf("retry/type options were not preserved: %+v", request)
	}
	if request.Headers["Content-Type"] != "application/x-www-form-urlencoded; charset=utf-8" {
		t.Fatalf("type must not be treated as request Content-Type: %+v", request.Headers)
	}
}

func TestPrepareSourceRequestExtractsStaticProxyOnly(t *testing.T) {
	request, err := PrepareSourceRequest(
		`https://source.example/search, {"headers":{"proxy":"url-option-value"}}`,
		"",
		1,
		"utf-8",
		map[string]string{"Proxy": "http://proxy.example:8080", "X-Source": "yes"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Proxy != "http://proxy.example:8080" ||
		request.Headers["proxy"] != "url-option-value" ||
		request.Headers["X-Source"] != "yes" {
		t.Fatalf("static proxy extraction mismatch: %+v", request)
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

func TestSearchBooksPreservesRequestOptionsInBookURL(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return searchPaginationResponse(request, `
				<article class="book">
					<a class="name" href='/book/detail, {"method":"POST","body":"id=7"}'>带请求参数的详情</a>
				</article>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 6, Name: "详情选项源", BaseURL: "https://source.example/root/", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "/search",
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	results, err := SearchBooks(source, "测试")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 ||
		results[0].BookURL != `https://source.example/book/detail, {"method":"POST","body":"id=7"}` {
		t.Fatalf("book request options were not preserved: %+v", results)
	}
}

func TestFetchBookInfoAndTOCExecutesRequestOptionsAcrossPages(t *testing.T) {
	requests := make([]string, 0, 3)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Method != http.MethodPost || request.Header.Get("X-Source") != "static" {
				t.Fatalf("unexpected request: %s %s headers=%v", request.Method, request.URL, request.Header)
			}
			requests = append(requests, request.URL.Path+"?"+string(body))

			responseBody := ""
			switch request.URL.Path + "?" + string(body) {
			case "/book/detail?id=7":
				if request.Header.Get("X-Detail") != "yes" {
					t.Fatalf("detail request options missing: %v", request.Header)
				}
				responseBody = `
					<h1 class="title">POST 详情书</h1>
					<a class="catalog" href='/catalog, {"method":"POST","body":"book=7","headers":{"X-Catalog":"one"}}'>目录</a>
				`
			case "/catalog?book=7":
				if request.Header.Get("X-Catalog") != "one" || request.Header.Get("X-Detail") != "" {
					t.Fatalf("catalog options leaked or missing: %v", request.Header)
				}
				responseBody = `
					<div class="chapter">
						<span class="chapter-title">第一章</span>
						<a href='/content, {"method":"POST","body":"chapter=1"}'>阅读</a>
					</div>
					<a class="next" href='/catalog, {"method":"POST","body":"book=7&page=2","headers":{"X-Catalog":"two"}}'>下一页</a>
				`
			case "/catalog?book=7&page=2":
				if request.Header.Get("X-Catalog") != "two" || request.Header.Get("X-Detail") != "" {
					t.Fatalf("next catalog options leaked or missing: %v", request.Header)
				}
				responseBody = `
					<div class="chapter">
						<span class="chapter-title">第二章</span>
						<a href='/content, {"method":"POST","body":"chapter=2"}'>阅读</a>
					</div>
				`
			default:
				t.Fatalf("unexpected request: %s %s body=%s", request.Method, request.URL, body)
			}
			return searchPaginationResponse(request, responseBody), nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "POST 详情目录源", BaseURL: "https://source.example/root/", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoNameRule: ".title",
		TOCURLRule:       ".catalog|attr:href",
		ChapterListRule:  ".chapter",
		ChapterNameRule:  ".chapter-title",
		ChapterURLRule:   "a|attr:href",
		NextTOCURLRule:   ".next|attr:href",
		Headers:          map[string]string{"X-Source": "static"},
	}); err != nil {
		t.Fatal(err)
	}

	info, chapters, err := FetchBookInfoAndTOC(
		`/book/detail, {"method":"POST","body":"id=7","headers":{"X-Detail":"yes"}}`,
		source,
	)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "POST 详情书" || len(chapters) != 2 {
		t.Fatalf("unexpected detail/toc result: info=%+v chapters=%+v", info, chapters)
	}
	if chapters[0].URL != `https://source.example/content, {"method":"POST","body":"chapter=1"}` ||
		chapters[1].URL != `https://source.example/content, {"method":"POST","body":"chapter=2"}` {
		t.Fatalf("chapter request options were not preserved: %+v", chapters)
	}
	if strings.Join(requests, ",") != "/book/detail?id=7,/catalog?book=7,/catalog?book=7&page=2" {
		t.Fatalf("same URL with different POST bodies was incorrectly deduplicated: %+v", requests)
	}
}

func TestParseTOCExecutesDirectPostURLRule(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Method != http.MethodPost ||
				request.URL.String() != "https://source.example/catalog" ||
				string(body) != "book=8" {
				t.Fatalf("direct toc URL options were not executed: %s %s body=%s", request.Method, request.URL, body)
			}
			return searchPaginationResponse(request, `
				<div class="chapter"><span class="title">直接 POST 目录</span><a href="/chapter/1">阅读</a></div>
			`), nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "直接 POST 目录源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		TOCURLRule:      `/catalog, {"method":"POST","body":"book=8"}`,
		ChapterListRule: ".chapter",
		ChapterNameRule: ".title",
		ChapterURLRule:  "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	chapters, err := ParseTOC("/book/8", source)
	if err != nil || len(chapters) != 1 || chapters[0].Title != "直接 POST 目录" {
		t.Fatalf("direct POST toc failed: chapters=%+v err=%v", chapters, err)
	}
}

func TestFetchChapterContentExecutesPostOptionsWithoutHeaderLeakage(t *testing.T) {
	requests := make([]string, 0, 2)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Method != http.MethodPost || request.Header.Get("X-Source") != "static" {
				t.Fatalf("unexpected content request: %s %s headers=%v", request.Method, request.URL, request.Header)
			}
			requests = append(requests, string(body))
			switch string(body) {
			case "chapter=1":
				if request.Header.Get("X-Chapter") != "one" || request.Header.Get("X-Page") != "" {
					t.Fatalf("initial content headers mismatch: %v", request.Header)
				}
				return searchPaginationResponse(request, `
					<main class="content">第一页</main>
					<a class="next" href='/content, {"method":"POST","body":"chapter=1&page=2","headers":{"X-Page":"two"}}'>下一页</a>
				`), nil
			case "chapter=1&page=2":
				if request.Header.Get("X-Page") != "two" || request.Header.Get("X-Chapter") != "" {
					t.Fatalf("content page options leaked or missing: %v", request.Header)
				}
				return searchPaginationResponse(request, `<main class="content">第二页</main>`), nil
			default:
				t.Fatalf("unexpected content body: %s", body)
			}
			return nil, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "POST 正文源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule:        ".content",
		NextContentURLRule: ".next|attr:href",
		Headers:            map[string]string{"X-Source": "static"},
	}); err != nil {
		t.Fatal(err)
	}

	content, err := FetchChapterContent(
		`/content, {"method":"POST","body":"chapter=1","headers":{"X-Chapter":"one"}}`,
		source,
	)
	if err != nil {
		t.Fatal(err)
	}
	if content != "第一页\n第二页" || strings.Join(requests, ",") != "chapter=1,chapter=1&page=2" {
		t.Fatalf("unexpected POST content result: content=%q requests=%v", content, requests)
	}
}

func TestFetchChapterContentExecutesDirectContentURLRuleOptions(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if request.Method != http.MethodPost ||
				request.URL.String() != "https://source.example/api/content" ||
				string(body) != "chapter=9" {
				t.Fatalf("content URL rule options were not executed: %s %s body=%s", request.Method, request.URL, body)
			}
			return searchPaginationResponse(request, `<main class="content">规则正文</main>`), nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "正文地址规则源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentURLRule: `/api/content, {"method":"POST","body":"chapter=9"}`,
		ContentRule:    ".content",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := FetchChapterContent("/chapter/9", source)
	if err != nil || content != "规则正文" {
		t.Fatalf("direct content URL rule failed: content=%q err=%v", content, err)
	}
}

func TestFetchBookInfoAndTOCResolvesLinksFromRedirectedResponseURL(t *testing.T) {
	requested := make([]string, 0, 2)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = append(requested, request.URL.String())
			responseBody := ""
			responseRequest := request
			switch request.URL.String() {
			case "https://source.example/book/12":
				responseBody = `
					<h1 class="title">重定向详情</h1>
					<a class="catalog" href="../catalog">目录</a>
				`
				redirectedURL, err := url.Parse("https://cdn.example/books/12/detail")
				if err != nil {
					t.Fatal(err)
				}
				responseRequest = request.Clone(request.Context())
				responseRequest.URL = redirectedURL
			case "https://cdn.example/books/catalog":
				responseBody = `
					<div class="chapter">
						<span class="chapter-title">重定向目录章节</span>
						<a href="./content/1">阅读</a>
					</div>
				`
			default:
				t.Fatalf("relative URL did not use redirected response URL: %s", request.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(responseBody)),
				Header:     make(http.Header),
				Request:    responseRequest,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "重定向地址源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoNameRule: ".title",
		TOCURLRule:       ".catalog|attr:href",
		ChapterListRule:  ".chapter",
		ChapterNameRule:  ".chapter-title",
		ChapterURLRule:   "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	info, chapters, err := FetchBookInfoAndTOC("/book/12", source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "重定向详情" ||
		len(chapters) != 1 ||
		chapters[0].URL != "https://cdn.example/books/content/1" {
		t.Fatalf("redirected response URL was not used as parsing base: info=%+v chapters=%+v", info, chapters)
	}
	if strings.Join(requested, ",") != "https://source.example/book/12,https://cdn.example/books/catalog" {
		t.Fatalf("unexpected redirected request sequence: %+v", requested)
	}
}
