package engine

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"openreader/backend/models"
)

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
