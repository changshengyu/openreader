package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"

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
	cachePaths := []string{"manual-refresh/success-old.txt", "manual-refresh/failure-old.txt"}
	for index, book := range books {
		fullPath := filepath.Join(server.cfg.CacheDir, cachePaths[index])
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("旧缓存正文"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "旧第一章", URL: "https://manual-refresh-api.test/old-one", CachePath: cachePaths[index]}).Error; err != nil {
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
	if strings.Contains(w.Body.String(), "secret upstream response") {
		t.Fatalf("response leaked remote failure details: %s", w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(server.cfg.CacheDir, cachePaths[0])); !os.IsNotExist(err) {
		t.Fatalf("committed replacement cache was not pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.CacheDir, cachePaths[1])); err != nil {
		t.Fatalf("failed book lost its readable cache: %v", err)
	}
}

func TestManualShelfRefreshReturnsStableTopLevelReadFailure(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	callbackName := "test:manual-shelf-refresh-read-failure"
	if err := server.db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "books" {
			tx.AddError(errors.New("injected shelf read failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.db.Callback().Query().Remove(callbackName) })

	req := httptest.NewRequest(http.MethodPost, "/api/books/check-updates", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError || w.Body.String() != `{"error":"检查书籍更新失败"}` {
		t.Fatalf("top-level failure = %d %s", w.Code, w.Body.String())
	}
}
