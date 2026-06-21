package engine

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestBookSourceRequestsApplyConfiguredHeaders(t *testing.T) {
	paths := make(map[string]int)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("X-Source-Token") != "secret" {
				t.Fatalf("%s request missing source token: %v", request.URL.Path, request.Header)
			}
			if request.Header.Get("Referer") != "https://reader.example/" {
				t.Fatalf("%s request missing referer: %v", request.URL.Path, request.Header)
			}
			if request.Header.Get("User-Agent") != "Configured Reader" {
				t.Fatalf("%s request did not preserve configured user agent: %v", request.URL.Path, request.Header)
			}
			if request.Host == "evil.example" || request.Header.Get("Host") == "evil.example" || request.ContentLength == 999 {
				t.Fatalf("%s request allowed unsafe host/content-length override: host=%q contentLength=%d headers=%v", request.URL.Path, request.Host, request.ContentLength, request.Header)
			}
			paths[request.URL.Path]++
			body := ""
			switch request.URL.Path {
			case "/search", "/explore":
				body = `<article class="book"><span class="name">测试书</span><a href="/book/1">详情</a></article>`
			case "/book/1":
				body = `<div class="chapter"><span class="title">第一章</span><a href="/chapter/1">阅读</a></div>`
			case "/chapter/1":
				body = `<main class="content">正文内容</main>`
			default:
				t.Fatalf("unexpected source request path: %s", request.URL.Path)
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

	source := models.BookSource{
		ID:      7,
		Name:    "带请求头书源",
		BaseURL: "https://source.example",
		Charset: "utf-8",
	}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:       "https://source.example/search?q={keyword}",
		ExploreURL:      "https://source.example/explore?page={page}",
		BookListRule:    ".book",
		BookNameRule:    ".name|text",
		BookURLRule:     "a|attr:href",
		ChapterListRule: ".chapter",
		ChapterNameRule: ".title|text",
		ChapterURLRule:  "a|attr:href",
		ContentRule:     ".content|text",
		Headers: map[string]string{
			"X-Source-Token": "secret",
			"Referer":        "https://reader.example/",
			"User-Agent":     "Configured Reader",
			"Host":           "evil.example",
			"Content-Length": "999",
		},
	}); err != nil {
		t.Fatal(err)
	}

	search, err := SearchBooks(source, "测试")
	if err != nil || len(search) != 1 {
		t.Fatalf("search with source headers failed: results=%+v err=%v", search, err)
	}
	explore, err := ExploreBooksPage(source, 1)
	if err != nil || len(explore.Items) != 1 {
		t.Fatalf("explore with source headers failed: result=%+v err=%v", explore, err)
	}
	chapters, err := ParseTOC("https://source.example/book/1", source)
	if err != nil || len(chapters) != 1 {
		t.Fatalf("toc with source headers failed: chapters=%+v err=%v", chapters, err)
	}
	content, err := FetchChapterContent("https://source.example/chapter/1", source)
	if err != nil || content != "正文内容" {
		t.Fatalf("content with source headers failed: content=%q err=%v", content, err)
	}
	for _, path := range []string{"/search", "/explore", "/book/1", "/chapter/1"} {
		if paths[path] != 1 {
			t.Fatalf("expected one %s request, got %d", path, paths[path])
		}
	}
}
