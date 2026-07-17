package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func createCacheStreamSource(t *testing.T, server *Server, baseURL string) models.BookSource {
	t.Helper()
	source := models.BookSource{Name: "缓存流契约书源", BaseURL: baseURL, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{ContentRule: ".content"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	return source
}

func TestCacheBookStreamEmitsProgressAndTerminalShelfItem(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://cache-stream.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<main class="content">` + req.URL.Path + ` 正文</main>`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := createCacheStreamSource(t, server, upstream)
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "缓存流书籍", URL: upstream + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: index, Title: "章节", URL: upstream + "/chapter-" + strconv.Itoa(index)}).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache/stream", strings.NewReader(`{"all":true,"count":2,"chapterIndex":0}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cache stream: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("cache stream should use SSE content type, got %q", contentType)
	}
	body := w.Body.String()
	if strings.Count(body, "event: message") != 2 || !strings.Contains(body, "event: end") ||
		!strings.Contains(body, `"cached":2`) || !strings.Contains(body, `"requested":2`) || !strings.Contains(body, `"bookId":1`) {
		t.Fatalf("unexpected cache stream events: %s", body)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].CachePath == "" || chapters[1].CachePath == "" {
		t.Fatalf("completed stream should persist both bounded chapter caches: %+v", chapters)
	}
}

func TestCacheBookStreamRejectsAnotherUsersBook(t *testing.T) {
	router, server := setupTestServer(t)
	tokenA := registerLifecycleToken(t, router, "streamowner")
	registerLifecycleToken(t, router, "streamother")
	owner := lifecycleUser(t, server, "streamowner")
	other := lifecycleUser(t, server, "streamother")
	source := createCacheStreamSource(t, server, "https://cache-stream-owner.test")
	book := models.Book{UserID: other.ID, SourceID: source.ID, Title: "他人缓存书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if owner.ID == other.ID {
		t.Fatal("cache stream fixture users unexpectedly overlap")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache/stream", strings.NewReader(`{"all":true,"count":1}`))
	req.Header.Set("Authorization", tokenA)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound || strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("foreign stream must fail before opening SSE, code=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
	}
}

func TestCacheBookStreamEmitsTerminalErrorWhenNoChapterCanBeCached(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://cache-stream-error.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("forced cache source failure")
		}),
	})
	defer restoreHTTPClient()
	source := createCacheStreamSource(t, server, upstream)
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "失败缓存书", URL: upstream + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "章节", URL: upstream + "/chapter"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache/stream", strings.NewReader(`{"all":true,"count":1}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: error") || strings.Contains(w.Body.String(), "event: end") {
		t.Fatalf("all-failed cache stream should emit only a terminal error, code=%d body=%s", w.Code, w.Body.String())
	}
	var persisted models.Chapter
	if err := server.db.First(&persisted, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.CachePath != "" {
		t.Fatalf("failed cache stream must not write a cache path, got %q", persisted.CachePath)
	}
}

func TestCacheBookStreamStopsSchedulingAfterCancellation(t *testing.T) {
	router, server := setupTestServer(t)
	authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://cache-stream-cancel.test"
	requestedPaths := make([]string, 0, 3)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedPaths = append(requestedPaths, req.URL.Path)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<main class="content">` + req.URL.Path + ` 正文</main>`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := createCacheStreamSource(t, server, upstream)
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "可取消缓存书", URL: upstream + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: index, Title: "章节", URL: upstream + "/chapter-" + strconv.Itoa(index)}).Error; err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cached, requested, failed, err := server.cacheBookChaptersStream(ctx, book, nil, true, 3, func(progress cacheStreamProgress) error {
		if progress.ChapterIndex == 0 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || cached != 1 || requested != 3 || failed != 0 {
		t.Fatalf("cancelled cache stream returned cached=%d requested=%d failed=%d err=%v", cached, requested, failed, err)
	}
	if strings.Join(requestedPaths, ",") != "/chapter-0" {
		t.Fatalf("cancelled cache stream scheduled later chapter fetches: %v", requestedPaths)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if chapters[0].CachePath == "" || chapters[1].CachePath != "" || chapters[2].CachePath != "" {
		t.Fatalf("cancelled stream should retain only completed bounded work: %+v", chapters)
	}
}
