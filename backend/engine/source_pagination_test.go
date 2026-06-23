package engine

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestParseTOCFollowsNextPagesWithoutLoopsOrDuplicates(t *testing.T) {
	requested := make([]string, 0, 3)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = append(requested, request.URL.Path)
			if request.Header.Get("X-Book-Token") != "secret" {
				t.Fatalf("paginated toc request missing headers: %v", request.Header)
			}
			body := ""
			switch request.URL.Path {
			case "/catalog/1":
				body = `
					<div class="chapter"><span class="title">第一章</span><a href="/chapter/1">阅读</a></div>
					<a class="next" href="/catalog/2">下一页</a>
					<a class="next" href="/catalog/3">第三页</a>
				`
			case "/catalog/2":
				body = `
					<div class="chapter"><span class="title">第一章重复</span><a href="/chapter/1">阅读</a></div>
					<div class="chapter"><span class="title">第二章</span><a href="/chapter/2">阅读</a></div>
					<a class="next" href="/catalog/3">下一页</a>
				`
			case "/catalog/3":
				body = `
					<div class="chapter"><span class="title">第三章</span><a href="/chapter/3">阅读</a></div>
					<a class="next" href="/catalog/1">循环首页</a>
				`
			default:
				t.Fatalf("unexpected toc page: %s", request.URL.String())
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

	source := models.BookSource{Name: "分页目录源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		TOCURLRule:      "/catalog/1",
		ChapterListRule: ".chapter",
		ChapterNameRule: ".title",
		ChapterURLRule:  "a|attr:href",
		NextTOCURLRule:  ".next|attr:href",
		Headers: map[string]string{
			"X-Book-Token": "secret",
		},
	}); err != nil {
		t.Fatal(err)
	}

	chapters, err := ParseTOC("https://source.example/book/1", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 deduplicated chapters, got %+v", chapters)
	}
	for index, title := range []string{"第一章重复", "第二章", "第三章"} {
		if chapters[index].Index != index || chapters[index].Title != title {
			t.Fatalf("unexpected chapter order/index: %+v", chapters)
		}
	}
	if strings.Join(requested, ",") != "/catalog/1,/catalog/2,/catalog/3" {
		t.Fatalf("expected each toc page once in source order, got %+v", requested)
	}
}

func TestNormalizeChapterOrderHonorsListPrefixSemantics(t *testing.T) {
	raw := []RemoteChapter{
		{Title: "第一章早期", URL: "/chapter/1"},
		{Title: "第二章", URL: "/chapter/2"},
		{Title: "第一章后期", URL: "/chapter/1"},
		{Title: "第三章", URL: "/chapter/3"},
	}
	normal := normalizeChapterOrder(raw, false)
	if len(normal) != 3 ||
		normal[0].Title != "第二章" ||
		normal[1].Title != "第一章后期" ||
		normal[2].Title != "第三章" {
		t.Fatalf("normal chapter order should keep the last duplicate: %+v", normal)
	}
	reversed := normalizeChapterOrder(raw, true)
	if len(reversed) != 3 ||
		reversed[0].Title != "第三章" ||
		reversed[1].Title != "第二章" ||
		reversed[2].Title != "第一章早期" {
		t.Fatalf("reversed chapter order should keep the first duplicate: %+v", reversed)
	}
	for index := range reversed {
		if reversed[index].Index != index {
			t.Fatalf("reversed chapter indexes were not normalized: %+v", reversed)
		}
	}
}

func TestFetchChapterContentFollowsNextPagesInOrder(t *testing.T) {
	requested := make([]string, 0, 3)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = append(requested, request.URL.Path)
			if request.Header.Get("X-Book-Token") != "secret" {
				t.Fatalf("paginated content request missing headers: %v", request.Header)
			}
			body := ""
			switch request.URL.Path {
			case "/chapter/1":
				switch request.URL.Query().Get("page") {
				case "2":
					body = `<main class="content">第二页</main><a class="next" href="/chapter/1?page=3">下一页</a>`
				case "3":
					body = `<main class="content">第三页</main><a class="next" href="/chapter/1">循环首页</a>`
				default:
					body = `
						<main class="content">第一页 广告</main>
						<a class="next" href="/chapter/1?page=2">下一页</a>
						<a class="next" href="/chapter/1?page=3">第三页</a>
					`
				}
			default:
				t.Fatalf("unexpected content page: %s", request.URL.String())
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

	source := models.BookSource{Name: "分页正文源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule:        ".content",
		NextContentURLRule: ".next|attr:href",
		Headers: map[string]string{
			"X-Book-Token": "secret",
		},
		TextReplaceRules: []models.TextReplaceRule{
			{Pattern: " 广告", Replacement: ""},
		},
	}); err != nil {
		t.Fatal(err)
	}

	content, err := FetchChapterContent("https://source.example/chapter/1", source)
	if err != nil {
		t.Fatal(err)
	}
	if content != "第一页\n第二页\n第三页" {
		t.Fatalf("unexpected paginated content: %q", content)
	}
	if len(requested) != 3 {
		t.Fatalf("expected three content requests without loops, got %+v", requested)
	}
}
