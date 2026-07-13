package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestRemoteChapterContentStopsAtAdjacentCatalogChapter(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://chapter-boundary.example"
	source := models.BookSource{Name: "正文边界契约源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule:        ".content",
		NextContentURLRule: ".next|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "正文跨章测试", URL: upstream + "/book/1"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: upstream + "/chapter/1"}
	nextChapter := models.Chapter{BookID: book.ID, Index: 1, Title: "第二章", URL: "/chapter/2"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&nextChapter).Error; err != nil {
		t.Fatal(err)
	}

	requested := make([]string, 0, 2)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.Path)
		if request.URL.Path != "/chapter/1" {
			t.Fatalf("reader must not fetch adjacent catalog chapter as a content continuation: %s", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<main class="content">第一章正文</main><a class="next" href="/chapter/2">下一页</a>`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	defer restoreHTTPClient()

	content, err := server.loadChapterTextContextResult(context.Background(), book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	if content != "第一章正文" {
		t.Fatalf("chapter content = %q, want only current chapter", content)
	}
	if got := strings.Join(requested, ","); got != "/chapter/1" {
		t.Fatalf("content requests = %s, want only current chapter", got)
	}
}

func TestBlankTextContentRuleDoesNotCacheOrInvalidateSource(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	source := models.BookSource{Name: "空正文规则契约源", BaseURL: "https://blank-content.example", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "空正文规则书", URL: source.BaseURL + "/book/1"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: source.BaseURL + "/chapter/1"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
	req.Header.Set("Authorization", auth)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, req)
	if writer.Code != http.StatusBadGateway {
		t.Fatalf("blank content rule status = %d, want %d: %s", writer.Code, http.StatusBadGateway, writer.Body.String())
	}

	var refreshedChapter models.Chapter
	if err := server.db.First(&refreshedChapter, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshedChapter.CachePath != "" {
		t.Fatalf("blank content rule must not write a cache path, got %q", refreshedChapter.CachePath)
	}
	var failures int64
	if err := server.db.Model(&models.SourceFailure{}).Where("user_id = ? AND source_id = ?", user.ID, source.ID).Count(&failures).Error; err != nil {
		t.Fatal(err)
	}
	if failures != 0 {
		t.Fatalf("local blank content rule must not invalidate source, got %d failure rows", failures)
	}
}
