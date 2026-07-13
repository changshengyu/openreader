package engine

import (
	"context"
	"errors"
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

func TestParseTOCOnlyUsesFirstLevelWhenRuleReturnsMultipleNextPages(t *testing.T) {
	pages := map[string]string{
		"/branch-root.html": sourceCompatFixture(t, "branch-root.html"),
		"/branch-a.html":    sourceCompatFixture(t, "branch-a.html"),
		"/branch-b.html":    sourceCompatFixture(t, "branch-b.html"),
		"/branch-c.html":    sourceCompatFixture(t, "branch-c.html"),
	}
	requested := make([]string, 0, 4)
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.Path)
		body, ok := pages[request.URL.Path]
		if !ok {
			t.Fatalf("unexpected toc branch page: %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	source := models.BookSource{Name: "目录分叉源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		TOCURLRule:      "/branch-root.html",
		ChapterListRule: ".chapter",
		ChapterNameRule: "a|text",
		ChapterURLRule:  "a|attr:href",
		NextTOCURLRule:  ".toc-next|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	chapters, err := ParseTOC("/book", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 || chapters[0].Title != "目录首页" || chapters[1].Title != "目录 A" || chapters[2].Title != "目录 B" {
		t.Fatalf("multi-next toc must retain only first-level branches: %+v", chapters)
	}
	if got := strings.Join(requested, ","); got != "/branch-root.html,/branch-a.html,/branch-b.html" {
		t.Fatalf("multi-next toc requests = %s", got)
	}
}

func TestParseTOCHonorsChapterFlagsAndFallbackURLs(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := `
				<div class="chapter">
					<span class="title">第一卷</span>
					<span class="volume">yes</span>
				</div>
				<div class="chapter">
					<span class="title">收费章</span>
					<a href="/vip">阅读</a>
					<span class="vip">1</span>
					<span class="updated">昨日</span>
				</div>
				<div class="chapter">
					<span class="title">无链接章</span>
					<span class="vip">0</span>
					<span class="updated">今日</span>
				</div>
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

	source := models.BookSource{Name: "章节标记源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		TOCURLRule:            "/catalog",
		ChapterListRule:       ".chapter",
		ChapterNameRule:       ".title",
		ChapterURLRule:        "a|attr:href",
		ChapterIsVolumeRule:   ".volume",
		ChapterIsVIPRule:      ".vip",
		ChapterUpdateTimeRule: ".updated",
	}); err != nil {
		t.Fatal(err)
	}

	chapters, err := ParseTOC("https://source.example/book/1", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %+v", chapters)
	}
	if !chapters[0].IsVolume || chapters[0].Title != "第一卷" || chapters[0].URL != "第一卷0" {
		t.Fatalf("volume chapter should use title/index URL fallback: %+v", chapters[0])
	}
	if chapters[1].IsVolume || chapters[1].Title != "🔒收费章" || chapters[1].URL != "https://source.example/vip" || chapters[1].Tag != "昨日" {
		t.Fatalf("vip chapter flags were not parsed: %+v", chapters[1])
	}
	if chapters[2].IsVolume || chapters[2].Title != "无链接章" || chapters[2].URL != "https://source.example/catalog" || chapters[2].Tag != "今日" {
		t.Fatalf("normal chapter should fall back to toc page URL: %+v", chapters[2])
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

func TestFetchChapterContentOnlyUsesFirstLevelWhenRuleReturnsMultipleNextPages(t *testing.T) {
	pages := map[string]string{
		"/branch-root.html": sourceCompatFixture(t, "branch-root.html"),
		"/branch-a.html":    sourceCompatFixture(t, "branch-a.html"),
		"/branch-b.html":    sourceCompatFixture(t, "branch-b.html"),
		"/branch-c.html":    sourceCompatFixture(t, "branch-c.html"),
	}
	requested := make([]string, 0, 4)
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.Path)
		body, ok := pages[request.URL.Path]
		if !ok {
			t.Fatalf("unexpected content branch page: %s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
	})})
	defer restore()

	source := models.BookSource{Name: "正文分叉源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule:        ".content",
		NextContentURLRule: ".content-next|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	content, err := FetchChapterContentContextWithNextChapter(context.Background(), "/branch-root.html", "/branch-b.html", source)
	if err != nil {
		t.Fatal(err)
	}
	if content != "正文首页\n正文 A\n正文 B" {
		t.Fatalf("multi-next content must retain only first-level branches: %q", content)
	}
	if got := strings.Join(requested, ","); got != "/branch-root.html,/branch-a.html,/branch-b.html" {
		t.Fatalf("multi-next content requests = %s", got)
	}
}

func TestFetchChapterContentStopsBeforeNextCatalogChapter(t *testing.T) {
	t.Run("matching next chapter is not fetched", func(t *testing.T) {
		requested := make([]string, 0, 2)
		restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = append(requested, request.URL.Path)
			if request.URL.Path != "/chapter/1" {
				t.Fatalf("next catalog chapter must not be requested: %s", request.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<main class="content">第一章正文</main><a class="next" href="/chapter/2">下一页</a>`)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		})})
		defer restore()

		source := models.BookSource{Name: "章节边界源", BaseURL: "https://source.example", Charset: "utf-8"}
		if err := source.SetRules(models.BookSourceRule{ContentRule: ".content", NextContentURLRule: ".next|attr:href"}); err != nil {
			t.Fatal(err)
		}
		content, err := FetchChapterContentContextWithNextChapter(context.Background(), "https://source.example/chapter/1", "/chapter/2", source)
		if err != nil || content != "第一章正文" {
			t.Fatalf("next chapter boundary content = %q, %v", content, err)
		}
		if got := strings.Join(requested, ","); got != "/chapter/1" {
			t.Fatalf("next chapter boundary requests = %s", got)
		}
	})

	t.Run("different or absent next chapter keeps the single-page chain", func(t *testing.T) {
		for _, nextChapterURL := range []string{"", "https://source.example/chapter/3"} {
			requested := make([]string, 0, 2)
			restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				requested = append(requested, request.URL.Path)
				switch request.URL.Path {
				case "/chapter/1":
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<main class="content">第一页</main><a class="next" href="/appendix">下一页</a>`)), Header: make(http.Header), Request: request}, nil
				case "/appendix":
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`<main class="content">第二页</main>`)), Header: make(http.Header), Request: request}, nil
				default:
					t.Fatalf("unexpected content page: %s", request.URL.String())
					return nil, nil
				}
			})})

			source := models.BookSource{Name: "章节边界延续源", BaseURL: "https://source.example", Charset: "utf-8"}
			if err := source.SetRules(models.BookSourceRule{ContentRule: ".content", NextContentURLRule: ".next|attr:href"}); err != nil {
				restore()
				t.Fatal(err)
			}
			content, err := FetchChapterContentContextWithNextChapter(context.Background(), "https://source.example/chapter/1", nextChapterURL, source)
			restore()
			if err != nil || content != "第一页\n第二页" {
				t.Fatalf("next chapter %q content = %q, %v", nextChapterURL, content, err)
			}
			if got := strings.Join(requested, ","); got != "/chapter/1,/appendix" {
				t.Fatalf("next chapter %q requests = %s", nextChapterURL, got)
			}
		}
	})
}

func TestFetchChapterContentRejectsBlankTextContentRule(t *testing.T) {
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("blank text content rule must fail before fetching: %s", request.URL.String())
		return nil, nil
	})})
	defer restore()

	source := models.BookSource{Name: "空正文规则文本源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{}); err != nil {
		t.Fatal(err)
	}
	_, err := FetchChapterContent("https://source.example/chapter/1", source)
	if !errors.Is(err, ErrInvalidSourceRule) {
		t.Fatalf("blank text content rule error = %v, want ErrInvalidSourceRule", err)
	}
}

func TestFetchChapterContentAppliesContentReplaceRegex(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<main class="content">第一段 广告
第二段 广告</main>`)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "正文替换源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule:         ".content",
		ContentReplaceRegex: "##\\s*广告##",
	}); err != nil {
		t.Fatal(err)
	}

	content, err := FetchChapterContent("https://source.example/chapter/1", source)
	if err != nil {
		t.Fatal(err)
	}
	if content != "第一段\n第二段" {
		t.Fatalf("unexpected replaced content: %q", content)
	}
}

func TestFetchChapterContentAppliesFullImageStyle(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`
					<main class="content">
						<p>图文正文</p>
						<img data-src="/images/full.jpg" alt="插图">
					</main>
				`)),
				Header:  make(http.Header),
				Request: request,
			}, nil
		}),
	})
	defer restore()

	source := models.BookSource{Name: "正文图片样式源", BaseURL: "https://source.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule:       ".content|html",
		ContentImageStyle: "FULL",
	}); err != nil {
		t.Fatal(err)
	}

	content, err := FetchChapterContent("https://source.example/chapter/1", source)
	if err != nil {
		t.Fatal(err)
	}
	want := "图文正文\n<img src=\"https://source.example/images/full.jpg\" alt=\"插图\" data-image-style=\"FULL\">"
	if content != want {
		t.Fatalf("unexpected image style content: %q, want %q", content, want)
	}
}
