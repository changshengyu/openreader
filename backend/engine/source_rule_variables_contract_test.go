package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"openreader/backend/models"
)

func sourceRuleVariableMapForTest(t *testing.T, raw string) map[string]string {
	t.Helper()
	values := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatalf("decode persisted variables %q: %v", raw, err)
	}
	return values
}

func TestSourceRuleVariablesAreItemAndOperationScoped(t *testing.T) {
	const searchBody = `
		<article class="book"><span class="token">第一令牌</span><h2 class="name">第一本书</h2><a href="/books/one">详情</a></article>
		<article class="book"><h2 class="name">第二本书</h2><a href="/books/two">详情</a></article>`
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(searchBody)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	defer restore()

	writer := models.BookSource{Name: "变量搜索源", BaseURL: "https://variables.example", Charset: "utf-8"}
	if err := writer.SetRules(models.BookSourceRule{
		SearchURL:      "/search",
		BookListRule:   ".book",
		BookNameRule:   `@put:{"marker":".token|text"}.name|text`,
		BookAuthorRule: "@get:{marker}",
		BookURLRule:    "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := SearchBooks(writer, "变量")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Author != "第一令牌" || items[1].Author != "" {
		t.Fatalf("per-result variable values = %#v", items)
	}

	reader := models.BookSource{Name: "变量读取源", BaseURL: "https://variables.example", Charset: "utf-8"}
	if err := reader.SetRules(models.BookSourceRule{
		SearchURL:      "/search",
		BookListRule:   ".book",
		BookNameRule:   ".name|text",
		BookAuthorRule: "@get:{marker}",
		BookURLRule:    "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	items, err = SearchBooks(reader, "变量")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Author != "" || items[1].Author != "" {
		t.Fatalf("a later source operation must not inherit variables: %#v", items)
	}
}

func TestSourceRuleVariablesFlowFromBookInfoToTOCAndAcrossContentPages(t *testing.T) {
	t.Run("book info to toc", func(t *testing.T) {
		restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := ""
			switch request.URL.Path {
			case "/book":
				body = `<h1 class="name">变量详情书</h1><a class="toc" href="/toc">目录</a>`
			case "/toc":
				body = `<div class="chapter"><a href="/chapter/1">第一章</a></div>`
			default:
				t.Fatalf("unexpected request %s", request.URL.String())
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
		})})
		defer restore()

		source := models.BookSource{Name: "详情目录变量源", BaseURL: "https://variables.example", Charset: "utf-8"}
		if err := source.SetRules(models.BookSourceRule{
			BookInfoNameRule: `@put:{"tocPath":".toc|attr:href"}h1|text`,
			TOCURLRule:       "@get:{tocPath}",
			ChapterListRule:  ".chapter",
			ChapterNameRule:  "a|text",
			ChapterURLRule:   "a|attr:href",
		}); err != nil {
			t.Fatal(err)
		}
		info, chapters, err := FetchBookInfoAndTOC("/book", source)
		if err != nil {
			t.Fatal(err)
		}
		if info.Title != "变量详情书" || len(chapters) != 1 || chapters[0].URL != "https://variables.example/chapter/1" {
			t.Fatalf("info/toc variables = %+v %#v", info, chapters)
		}
	})

	t.Run("single content chain", func(t *testing.T) {
		restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := ""
			switch request.URL.Path {
			case "/chapter/1":
				body = `<main class="content">变量正文第一页</main><a class="next" href="/chapter/1-2">下一页</a>`
			case "/chapter/1-2":
				body = `<main class="content">变量正文第二页</main>`
			default:
				t.Fatalf("unexpected request %s", request.URL.String())
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
		})})
		defer restore()

		source := models.BookSource{Name: "正文变量源", BaseURL: "https://variables.example", Charset: "utf-8"}
		if err := source.SetRules(models.BookSourceRule{
			ContentRule:        `@put:{"nextPath":".next|attr:href"}.content|text`,
			NextContentURLRule: "@get:{nextPath}",
		}); err != nil {
			t.Fatal(err)
		}
		content, err := FetchChapterContent("/chapter/1", source)
		if err != nil || content != "变量正文第一页\n变量正文第二页" {
			t.Fatalf("content variables = %q, %v", content, err)
		}
	})
}

func TestSourceRuleVariablesRejectMalformedWritesAndKeepTemplatesDisabled(t *testing.T) {
	document, err := newSourceRuleDocument(`<main class="content">正文</main>`)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := sourceRuleString(document.Root(), "@get:{missing}")
	if err != nil || missing != "" {
		t.Fatalf("missing variable = %q, %v", missing, err)
	}
	for _, rule := range []string{
		`@put:{"bad":1}.content|text`,
		`@put:{bad}.content|text`,
		`@put:{"` + strings.Repeat("k", maxSourceRuleVariableKeySize+1) + `":".content|text"}.content|text`,
	} {
		if _, err := sourceRuleString(document.Root(), rule); !errors.Is(err, ErrInvalidSourceRule) {
			t.Fatalf("malformed variable write %q error = %v, want ErrInvalidSourceRule", rule, err)
		}
	}
	if _, err := sourceRuleString(document.Root(), "{{result}}"); !errors.Is(err, ErrUnsupportedSourceRule) {
		t.Fatalf("template error = %v, want ErrUnsupportedSourceRule", err)
	}
	runtime := newSourceRuleRuntime()
	for index := 0; index < maxSourceRuleVariables; index++ {
		runtime.variables[fmt.Sprintf("existing-%d", index)] = "x"
	}
	if _, err := sourceRuleString(document.RootWithRuntime(runtime), `@put:{"overflow":".content|text"}.content|text`); !errors.Is(err, ErrInvalidSourceRule) {
		t.Fatalf("runtime variable count error = %v, want ErrInvalidSourceRule", err)
	}
}

func TestPersistentSourceRuleVariablesFollowReaderDevScopes(t *testing.T) {
	t.Run("search copies request variables into every result book", func(t *testing.T) {
		const searchBody = `
			<div class="scope">搜索令牌</div>
			<article class="book"><span class="token">第一令牌</span><h2 class="name">第一本书</h2><a href="/books/one">详情</a></article>
			<article class="book"><span class="token">第二令牌</span><h2 class="name">第二本书</h2><a href="/books/two">详情</a></article>`
		restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(searchBody)), Header: make(http.Header), Request: request}, nil
		})})
		defer restore()

		source := models.BookSource{Name: "持久变量搜索源", BaseURL: "https://variables.example", Charset: "utf-8"}
		if err := source.SetRules(models.BookSourceRule{
			SearchURL:      "/search",
			BookListRule:   `@put:{"scope":".scope|text"}.book`,
			BookNameRule:   `@put:{"item":".token|text"}.name|text`,
			BookAuthorRule: "@get:{bookName}",
			BookURLRule:    "a|attr:href",
		}); err != nil {
			t.Fatal(err)
		}
		items, err := SearchBooks(source, "变量")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || items[0].Author != "第一本书" || items[1].Author != "第二本书" {
			t.Fatalf("search result special variables = %#v", items)
		}
		first := sourceRuleVariableMapForTest(t, items[0].Variable)
		second := sourceRuleVariableMapForTest(t, items[1].Variable)
		if first["scope"] != "搜索令牌" || first["item"] != "第一令牌" || second["scope"] != "搜索令牌" || second["item"] != "第二令牌" {
			t.Fatalf("per-result persisted variables = %#v %#v", first, second)
		}
	})

	t.Run("book, chapter and content variables retain reader-dev precedence", func(t *testing.T) {
		restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			var body string
			switch request.URL.Path {
			case "/book":
				body = `<h1 class="name">持久书</h1><a class="toc" href="/toc">目录</a>`
			case "/toc":
				body = `<div class="chapter"><span class="token">/chapter/one</span><a class="name">第一章</a></div>`
			case "/chapter/one":
				body = `<main class="content">忽略正文</main><span class="token">正文令牌</span>`
			default:
				t.Fatalf("unexpected request %s", request.URL.String())
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: request}, nil
		})})
		defer restore()

		source := models.BookSource{Name: "持久变量目录源", BaseURL: "https://variables.example", Charset: "utf-8"}
		if err := source.SetRules(models.BookSourceRule{
			BookInfoNameRule:   `@put:{"tocPath":".toc|attr:href"}.name|text`,
			BookInfoAuthorRule: "@get:{searchToken}",
			TOCURLRule:         "@get:{tocPath}",
			ChapterListRule:    ".chapter",
			ChapterNameRule:    `@put:{"chapterPath":".token|text"}.name|text`,
			ChapterURLRule:     "@get:{chapterPath}",
			ContentRule:        `@put:{"contentToken":".token|text"}@get:{bookName}:@get:{title}`,
		}); err != nil {
			t.Fatal(err)
		}

		info, chapters, bookVariable, err := FetchBookInfoAndTOCWithVariables("/book", source, `{"searchToken":"搜索结果令牌"}`, "搜索书名")
		if err != nil {
			t.Fatal(err)
		}
		if info.Title != "持久书" || info.Author != "搜索结果令牌" || len(chapters) != 1 || chapters[0].URL != "https://variables.example/chapter/one" {
			t.Fatalf("persistent book/toc parse = %+v %#v", info, chapters)
		}
		bookValues := sourceRuleVariableMapForTest(t, bookVariable)
		chapterValues := sourceRuleVariableMapForTest(t, chapters[0].Variable)
		if bookValues["searchToken"] != "搜索结果令牌" || bookValues["tocPath"] != "/toc" || chapterValues["chapterPath"] != "/chapter/one" {
			t.Fatalf("book/chapter state = %#v %#v", bookValues, chapterValues)
		}

		content, nextState, err := FetchChapterContentContextWithState(context.Background(), chapters[0].URL, "", source, SourceRuleVariableState{
			BookVariable:    bookVariable,
			ChapterVariable: chapters[0].Variable,
			BookName:        info.Title,
			ChapterTitle:    chapters[0].Title,
		})
		if err != nil || content != "持久书:第一章" {
			t.Fatalf("content special-variable precedence = %q, %v", content, err)
		}
		nextBookValues := sourceRuleVariableMapForTest(t, nextState.BookVariable)
		nextChapterValues := sourceRuleVariableMapForTest(t, nextState.ChapterVariable)
		if nextBookValues["tocPath"] != "/toc" || nextChapterValues["chapterPath"] != "/chapter/one" || nextChapterValues["contentToken"] != "正文令牌" {
			t.Fatalf("content state persistence = %#v %#v", nextBookValues, nextChapterValues)
		}
	})
}

func TestPersistentSourceRuleVariablesRejectInvalidStateBeforeFetch(t *testing.T) {
	requests := 0
	restore := SetHTTPClient(&http.Client{Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return nil, fmt.Errorf("a malformed persisted variable must not make a request")
	})})
	defer restore()

	source := models.BookSource{Name: "持久变量校验源", BaseURL: "https://variables.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{BookInfoNameRule: "h1|text"}); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := FetchBookInfoAndTOCWithVariables("/book", source, `{"bad":1}`, "书名")
	if !errors.Is(err, ErrInvalidSourceRule) || requests != 0 {
		t.Fatalf("invalid persisted state error = %v, requests = %d", err, requests)
	}
}
