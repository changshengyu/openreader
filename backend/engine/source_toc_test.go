package engine

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestParseTOCResolvesCatalogURLFromBookInfoPage(t *testing.T) {
	requested := make([]string, 0, 2)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = append(requested, request.URL.String())
			if request.Header.Get("Referer") != "https://source.example/" {
				t.Fatalf("catalog request missing configured header: %v", request.Header)
			}
			body := ""
			switch request.URL.Path {
			case "/book/1":
				body = `
					<h1 class="book-name">详情书名</h1>
					<span class="book-author">详情作者</span>
					<img class="book-cover" data-src="/cover.jpg">
					<div class="book-intro">详情简介</div>
					<span class="book-kind">玄幻</span>
					<span class="book-latest">最新章</span>
					<span class="book-update">今天</span>
					<span class="book-words">100万字</span>
					<a class="catalog" href="/catalog/1">目录</a>
				`
			case "/catalog/1":
				body = `<div class="chapter"><span class="title">第一章</span><a href="/chapter/1">阅读</a></div>`
			default:
				t.Fatalf("unexpected request: %s", request.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "详情目录源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoNameRule:          ".book-name",
		BookInfoAuthorRule:        ".book-author",
		BookInfoCoverRule:         ".book-cover|attr:data-src",
		BookInfoIntroRule:         ".book-intro",
		BookInfoKindRule:          ".book-kind",
		BookInfoLatestChapterRule: ".book-latest",
		BookInfoUpdateTimeRule:    ".book-update",
		BookInfoWordCountRule:     ".book-words",
		TOCURLRule:                ".catalog|attr:href",
		ChapterListRule:           ".chapter",
		ChapterNameRule:           ".title",
		ChapterURLRule:            "a|attr:href",
		Headers: map[string]string{
			"Referer": "https://source.example/",
		},
	}); err != nil {
		t.Fatal(err)
	}

	info, chapters, err := FetchBookInfoAndTOC("https://source.example/book/1", source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "详情书名" ||
		info.Author != "详情作者" ||
		info.CoverURL != "https://source.example/cover.jpg" ||
		info.Intro != "详情简介" ||
		info.Kind != "玄幻" ||
		info.LatestChapter != "最新章" ||
		info.UpdateTime != "今天" ||
		info.WordCount != "100万字" {
		t.Fatalf("unexpected book info: %+v", info)
	}
	if len(chapters) != 1 || chapters[0].Title != "第一章" || chapters[0].URL != "https://source.example/chapter/1" {
		t.Fatalf("unexpected chapters: %+v", chapters)
	}
	if len(requested) != 2 ||
		requested[0] != "https://source.example/book/1" ||
		requested[1] != "https://source.example/catalog/1" {
		t.Fatalf("expected book info then catalog requests, got %+v", requested)
	}
}

func TestFetchBookInfoAndTOCHonorsBookInfoInitScope(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := `
				<section class="recommend">
					<h1 class="book-name">推荐书名</h1>
					<span class="book-author">推荐作者</span>
					<img class="book-cover" data-src="/wrong-cover.jpg">
					<p class="book-intro">推荐简介</p>
					<span class="book-kind">推荐分类</span>
					<span class="book-latest">推荐最新章</span>
					<span class="book-update">昨天</span>
					<span class="book-words">123</span>
				</section>
				<section class="book-detail">
					<h1 class="book-name">详情书名</h1>
					<span class="book-author">详情作者</span>
					<img class="book-cover" data-src="/right-cover.jpg">
					<p class="book-intro">详情简介</p>
					<span class="book-kind">玄幻</span>
					<span class="book-latest">最新章</span>
					<span class="book-update">今天</span>
					<span class="book-words">23456</span>
					<div class="chapter"><span class="title">第一章</span><a href="/chapter/1">阅读</a></div>
				</section>
			`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "详情 init 源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoInitRule:          ".book-detail",
		BookInfoNameRule:          ".book-name",
		BookInfoAuthorRule:        ".book-author",
		BookInfoCoverRule:         ".book-cover|attr:data-src",
		BookInfoIntroRule:         ".book-intro",
		BookInfoKindRule:          ".book-kind",
		BookInfoLatestChapterRule: ".book-latest",
		BookInfoUpdateTimeRule:    ".book-update",
		BookInfoWordCountRule:     ".book-words",
		ChapterListRule:           ".chapter",
		ChapterNameRule:           ".title",
		ChapterURLRule:            "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	info, chapters, err := FetchBookInfoAndTOC("https://source.example/book/1", source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "详情书名" ||
		info.Author != "详情作者" ||
		info.CoverURL != "https://source.example/right-cover.jpg" ||
		info.Intro != "详情简介" ||
		info.Kind != "玄幻" ||
		info.LatestChapter != "最新章" ||
		info.UpdateTime != "今天" ||
		info.WordCount != "2.3万字" {
		t.Fatalf("book info should be parsed inside init scope: %+v", info)
	}
	if len(chapters) != 1 || chapters[0].Title != "第一章" {
		t.Fatalf("init scope should not break toc parsing: %+v", chapters)
	}
}

func TestParseTOCFallsBackToBookPageWhenCatalogRuleIsEmpty(t *testing.T) {
	requestCount := 0
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`
					<div class="chapter"><span class="title">详情页目录</span><a href="/chapter/1">阅读</a></div>
				`)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "同页目录源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		TOCURLRule:      ".missing-catalog|attr:href",
		ChapterListRule: ".chapter",
		ChapterNameRule: ".title",
		ChapterURLRule:  "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	chapters, err := ParseTOC("https://source.example/book/1", source)
	if err != nil || len(chapters) != 1 || chapters[0].Title != "详情页目录" {
		t.Fatalf("expected book page fallback, chapters=%+v err=%v", chapters, err)
	}
	if requestCount != 1 {
		t.Fatalf("book page fallback should reuse the fetched document, got %d requests", requestCount)
	}
}

func TestParseTOCPreservesLegacyDirectCatalogURL(t *testing.T) {
	requested := ""
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = request.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`
					<div class="chapter"><span class="title">直接目录</span><a href="/chapter/1">阅读</a></div>
				`)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "旧目录源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		TOCURLRule:      "/catalog/1",
		ChapterListRule: ".chapter",
		ChapterNameRule: ".title",
		ChapterURLRule:  "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	chapters, err := ParseTOC("https://source.example/book/1", source)
	if err != nil || len(chapters) != 1 {
		t.Fatalf("legacy direct catalog failed: chapters=%+v err=%v", chapters, err)
	}
	if requested != "https://source.example/catalog/1" {
		t.Fatalf("expected direct catalog request, got %q", requested)
	}
}

func TestIsDirectTOCURLRuleDistinguishesProtocolRelativeURLsFromRawXPath(t *testing.T) {
	if !isDirectTOCURLRule("//catalog.source.example/chapters") {
		t.Fatal("protocol-relative catalog URL must remain a direct URL rule")
	}
	if isDirectTOCURLRule("//a[@id='toc']/@href") {
		t.Fatal("raw XPath catalog rule must be evaluated against the book document")
	}
}
