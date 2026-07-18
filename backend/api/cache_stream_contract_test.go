package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func cacheStreamEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	events := make([]map[string]any, 0)
	for _, block := range strings.Split(body, "\n\n") {
		if !strings.Contains(block, "data: ") {
			continue
		}
		var payload map[string]any
		data := strings.SplitN(block, "data: ", 2)[1]
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			t.Fatalf("decode cache event %q: %v", block, err)
		}
		events = append(events, payload)
	}
	return events
}

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

func TestCacheBookStreamRejectsMissingSourceBeforeOpeningSSE(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")
	book := models.Book{UserID: user.ID, SourceID: 999999, Title: "缺失书源缓存书"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: book.ID, Index: 0, Title: "章节", URL: "https://missing-source.test/chapter"}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache/stream", strings.NewReader(`{"all":true}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("missing source must fail before opening SSE, code=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "book source not found") {
		t.Fatalf("missing source must return a stable client error: %s", w.Body.String())
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
	result, err := server.cacheBookChapters(ctx, book, nil, true, 3, false, func(progress cacheStreamProgress) error {
		if progress.ChapterIndex == 0 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || result.SelectedCached != 1 || result.Total != 3 || result.Processed != 1 || result.FailedCount != 0 {
		t.Fatalf("cancelled cache stream returned result=%+v err=%v", result, err)
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

func TestCacheBookStreamReportsExistingSuccessAndFailureCounts(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://cache-stream-counts.test"
	requestedPaths := make([]string, 0, 3)
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestedPaths = append(requestedPaths, req.URL.Path)
			if req.URL.Path == "/chapter-5" {
				return nil, errors.New("forced final chapter failure with secret=do-not-expose")
			}
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
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "缓存计数书", URL: upstream + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 6; index++ {
		chapter := models.Chapter{BookID: book.ID, Index: index, Title: "章节", URL: upstream + "/chapter-" + strconv.Itoa(index)}
		if index < 3 {
			cachePath := "cache-counts/chapter-" + strconv.Itoa(index) + ".txt"
			fullPath := server.cfg.CacheDir + "/" + cachePath
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fullPath, []byte("已有正文"), 0o644); err != nil {
				t.Fatal(err)
			}
			chapter.CachePath = cachePath
		}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache/stream", strings.NewReader(`{"all":true}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: end") {
		t.Fatalf("mixed cache stream should end normally, code=%d body=%s", w.Code, w.Body.String())
	}
	events := cacheStreamEvents(t, w.Body.String())
	terminal := events[len(events)-1]
	if terminal["cachedCount"] != float64(5) || terminal["successCount"] != float64(2) || terminal["failedCount"] != float64(1) || terminal["processed"] != float64(6) || terminal["total"] != float64(6) {
		t.Fatalf("unexpected canonical terminal counts: %#v", terminal)
	}
	if terminal["cached"] != float64(5) || terminal["requested"] != float64(6) || terminal["failed"] != float64(1) {
		t.Fatalf("legacy aliases must remain compatible: %#v", terminal)
	}
	if got := strings.Join(requestedPaths, ","); got != "/chapter-3,/chapter-4,/chapter-5" {
		t.Fatalf("valid existing cache must be skipped, requested=%s", got)
	}
	if strings.Contains(w.Body.String(), "do-not-expose") {
		t.Fatalf("cache stream exposed an internal source error: %s", w.Body.String())
	}
}

func TestCacheBookStreamRefreshRefetchesExistingChapter(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://cache-stream-refresh.test"
	requests := 0
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<main class="content">刷新后的正文</main>`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := createCacheStreamSource(t, server, upstream)
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "强制刷新缓存书", URL: upstream + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := "cache-refresh/chapter-0.txt"
	fullPath := filepath.Join(server.cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("旧正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "章节", URL: upstream + "/chapter-0", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache/stream", strings.NewReader(`{"all":true,"refresh":true}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: end") {
		t.Fatalf("refresh cache stream: code=%d body=%s", w.Code, w.Body.String())
	}
	if requests != 1 {
		t.Fatalf("refresh=true must refetch existing cache, requests=%d", requests)
	}
	var persisted models.Chapter
	if err := server.db.First(&persisted, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	content, _, err := server.readChapterCache(book, persisted.CachePath)
	if err != nil || !strings.Contains(string(content), "刷新后的正文") {
		t.Fatalf("refresh must replace readable cache, content=%q err=%v", string(content), err)
	}
}

func TestCacheBookStreamClearsMissingCacheReferenceBeforeFailedRefetch(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://cache-stream-missing.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("forced missing-cache refetch failure")
		}),
	})
	defer restoreHTTPClient()

	source := createCacheStreamSource(t, server, upstream)
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "缓存文件缺失书", URL: upstream + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{
		BookID: book.ID, Index: 0, Title: "章节", URL: upstream + "/chapter-0",
		CachePath: "missing-cache/chapter-0.txt",
	}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/cache/stream", strings.NewReader(`{"all":true}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "event: error") {
		t.Fatalf("missing-cache refetch should end with a client-safe error, code=%d body=%s", w.Code, w.Body.String())
	}
	var persisted models.Chapter
	if err := server.db.First(&persisted, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.CachePath != "" {
		t.Fatalf("missing cache file must not remain counted by its stale database reference: %q", persisted.CachePath)
	}
}
