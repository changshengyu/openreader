package engine

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/models"
)

func TestSearchBooksPageUsesPagePlaceholder(t *testing.T) {
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			page := request.URL.Query().Get("page")
			body := `
				<article class="book">
					<a class="name" href="/book/` + page + `">第` + page + `页书籍</a>
				</article>
			`
			return searchPaginationResponse(request, body), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 1, Name: "分页源", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:    "https://source.example/search?q={keyword}&page={page}",
		BookListRule: ".book",
		BookNameRule: ".name",
		BookURLRule:  ".name|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SearchBooksPage(source, "测试 关键词", 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Page != 2 || !result.HasMore || len(result.Items) != 1 {
		t.Fatalf("unexpected page result: %+v", result)
	}
	if result.Items[0].Title != "第2页书籍" || result.Items[0].BookURL != "https://source.example/book/2" {
		t.Fatalf("page placeholder was not applied: %+v", result.Items[0])
	}
}

func TestSearchBooksPageFollowsLegacyNextPageRule(t *testing.T) {
	requested := make([]string, 0, 2)
	restore := SetHTTPClient(&http.Client{
		Transport: contextRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requested = append(requested, request.URL.Path)
			page := 1
			if request.URL.Path == "/page/2" {
				page = 2
			}
			next := ""
			if page == 1 {
				next = `<a class="next" href="/page/2">下一页</a>`
			}
			body := `
				<article class="book">
					<a class="name" href="/book/` + strconv.Itoa(page) + `">第` + strconv.Itoa(page) + `页书籍</a>
				</article>
				` + next
			return searchPaginationResponse(request, body), nil
		}),
	})
	defer restore()

	source := models.BookSource{ID: 2, Name: "旧分页源", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{
		SearchURL:      "https://source.example/search?q={keyword}",
		BookListRule:   ".book",
		BookNameRule:   ".name",
		BookURLRule:    ".name|attr:href",
		PaginationRule: ".next|attr:href",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := SearchBooksPage(source, "测试", 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(requested, ",") != "/search,/page/2" {
		t.Fatalf("unexpected pagination requests: %v", requested)
	}
	if result.HasMore || len(result.Items) != 1 || result.Items[0].Title != "第2页书籍" {
		t.Fatalf("unexpected second page: %+v", result)
	}
}

func searchPaginationResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}
}
