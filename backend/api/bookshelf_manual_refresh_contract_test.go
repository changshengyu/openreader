package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestManualShelfRefreshReportsSafePartialFailureAndChangedShelfItems(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	restore := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.URL.Path, "failure") {
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("secret upstream response")), Header: make(http.Header), Request: request}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<html><body><li class="chapter"><a href="/new-one">新第一章</a></li></body></html>`)),
			Header:     make(http.Header), Request: request,
		}, nil
	})})
	defer restore()

	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "部分失败源", BaseURL: "https://manual-refresh-api.test", Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	books := []models.Book{
		{UserID: user.ID, SourceID: source.ID, Title: "成功书", URL: "https://manual-refresh-api.test/success", LastChapter: "旧第一章", ChapterCount: 1, CanUpdate: true},
		{UserID: user.ID, SourceID: source.ID, Title: "失败书", URL: "https://manual-refresh-api.test/failure", LastChapter: "第一章", ChapterCount: 1, CanUpdate: true},
	}
	if err := server.db.Create(&books).Error; err != nil {
		t.Fatal(err)
	}
	for _, book := range books {
		if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "旧第一章", URL: "https://manual-refresh-api.test/old-one"}).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/check-updates", strings.NewReader(`{"legacyExtra":true}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("manual refresh status = %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Checked         int            `json:"checked"`
		Updated         int            `json:"updated"`
		Failed          int            `json:"failed"`
		NewChapters     int            `json:"newChapters"`
		ReplacedBookIDs []uint         `json:"replacedBookIds"`
		Books           []bookListItem `json:"books"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Checked != 2 || response.Updated != 1 || response.Failed != 1 || response.NewChapters != 0 {
		t.Fatalf("partial refresh summary = %+v", response)
	}
	if len(response.ReplacedBookIDs) != 1 || response.ReplacedBookIDs[0] != books[0].ID {
		t.Fatalf("replaced ids = %v", response.ReplacedBookIDs)
	}
	if len(response.Books) != 1 || response.Books[0].ID != books[0].ID || response.Books[0].LastChapter != "新第一章" {
		t.Fatalf("changed shelf items = %+v", response.Books)
	}
	if strings.Contains(w.Body.String(), "secret upstream response") || strings.Contains(w.Body.String(), "manual-refresh-api.test") {
		t.Fatalf("response leaked remote failure details: %s", w.Body.String())
	}
}
