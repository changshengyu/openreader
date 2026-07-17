package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func contentSearchContractUser(t *testing.T, server *Server) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", "testuser").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func createContentSearchCacheChapter(t *testing.T, server *Server, bookID uint, index int, content string) models.Chapter {
	t.Helper()
	cachePath := filepath.Join("content-search-contract", strconv.Itoa(index)+".txt")
	path := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: bookID, Index: index, Title: "第" + strconv.Itoa(index+1) + "章", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return chapter
}

func TestContentSearchDoesNotSkipDenseFinalChapter(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{UserID: user.ID, Title: "密集正文搜索"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		lines = append(lines, "第"+strconv.Itoa(i+1)+"段目标词")
	}
	createContentSearchCacheChapter(t, server, book.ID, 0, strings.Join(lines, "\n"))
	createContentSearchCacheChapter(t, server, book.ID, 1, "下一章目标词")

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标词")+"&paged=1&lastIndex=-1&chapterLimit=2&matchLimit=3&perChapterLimit=3",
		nil,
	)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("dense content search: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page struct {
		List      []contentMatch `json:"list"`
		LastIndex int            `json:"lastIndex"`
		HasMore   bool           `json:"hasMore"`
		Truncated bool           `json:"truncated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 12 || page.LastIndex != 0 || !page.HasMore || page.Truncated {
		t.Fatalf("the final scanned chapter must be complete before its cursor advances: %+v", page)
	}
}

func TestContentSearchReportsUnavailableRemoteChapters(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{UserID: user.ID, Title: "不可用搜索书", SourceID: 99999}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://unavailable.example/chapter"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=1", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unavailable chapter search: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page struct {
		List                []contentMatch `json:"list"`
		Incomplete          bool           `json:"incomplete"`
		UnavailableChapters int            `json:"unavailableChapters"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 0 || !page.Incomplete || page.UnavailableChapters != 1 {
		t.Fatalf("an unavailable remote scan must not masquerade as an ordinary empty result: %+v", page)
	}
}

func TestContentSearchMakesSafetyTruncationExplicit(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	book := models.Book{UserID: user.ID, Title: "搜索安全上限"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	createContentSearchCacheChapter(t, server, book.ID, 0, strings.Repeat("目标\n", contentSearchMaxMatchesPerChapter+1))

	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/search?q="+url.QueryEscape("目标")+"&paged=1&lastIndex=-1&chapterLimit=1&matchLimit=1", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("safety-capped content search: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page struct {
		List       []contentMatch `json:"list"`
		Incomplete bool           `json:"incomplete"`
		Truncated  bool           `json:"truncated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.List) != contentSearchMaxMatchesPerChapter || !page.Incomplete || !page.Truncated {
		t.Fatalf("a safety cap must remain visible instead of silently skipping matches: %+v", page)
	}
}

func TestContentSearchStopsSchedulingRemoteChaptersAfterCancellation(t *testing.T) {
	_, server := setupTestServer(t)
	user := models.User{Username: "cancel-search-user", PasswordHash: "not-used"}
	if err := server.db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	source := models.BookSource{Name: "取消搜索源", BaseURL: "https://search-cancel.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, Title: "取消正文搜索", SourceID: source.ID}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://search-cancel.example/1"},
		{BookID: book.ID, Index: 1, Title: "第二章", URL: "https://search-cancel.example/2"},
	}
	if err := server.db.Create(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make([]string, 0, 2)
	restoreClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.URL.Path)
			cancel()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`<main class="content">目标正文</main>`)),
				Request:    request,
			}, nil
		}),
	})
	defer restoreClient()

	scan := server.collectContentMatchesContext(ctx, book, chapters, "目标", 0, 2, 20, 20)
	if !scan.Canceled || len(requests) != 1 || len(scan.Matches) != 0 {
		t.Fatalf("cancellation must stop the search before later chapter requests: scan=%+v requests=%v", scan, requests)
	}
}

func TestLegacyContentSearchPropagatesRequestCancellation(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := contentSearchContractUser(t, server)
	source := models.BookSource{Name: "兼容接口取消搜索源", BaseURL: "https://legacy-search-cancel.example", Charset: "utf-8"}
	if err := source.SetRules(models.BookSourceRule{ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:   user.ID,
		Title:    "兼容接口取消正文搜索",
		URL:      "https://legacy-search-cancel.example/book",
		SourceID: source.ID,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "第一章", URL: "https://legacy-search-cancel.example/1"},
		{BookID: book.ID, Index: 1, Title: "第二章", URL: "https://legacy-search-cancel.example/2"},
	}
	if err := server.db.Create(&chapters).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make([]string, 0, 2)
	restoreClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.URL.Path)
			cancel()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`<main class="content">没有命中</main>`)),
				Request:    request,
			}, nil
		}),
	})
	defer restoreClient()

	body := `{"bookUrl":"https://legacy-search-cancel.example/book","keyword":"目标","lastIndex":-1,"size":20}`
	request := httptest.NewRequest(http.MethodPost, "/api/reader3/searchBookContent", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if len(requests) != 1 {
		t.Fatalf("legacy cancellation must stop before the next chapter request: %v", requests)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("a canceled compatibility search must not serialize false success: %s", response.Body.String())
	}
}
