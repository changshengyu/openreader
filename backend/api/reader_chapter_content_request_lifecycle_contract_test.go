package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestReaderConcurrentSameChapterLoadsSharePublishedResult(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapterconcurrentduplicate")
	var fetches atomic.Int32
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		fetches.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<main class="content">shared remote content</main><span class="token">shared token</span>`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	t.Cleanup(restoreHTTPClient)

	firstFetched := make(chan struct{})
	releaseFirst := make(chan struct{})
	var hookCalls atomic.Int32
	installReaderChapterContentLifecycleHook(t, func(stage string, _ context.Context, book models.Book, chapter models.Chapter) {
		if stage != "after_remote_fetch" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		if hookCalls.Add(1) == 1 {
			close(firstFetched)
			<-releaseFirst
		}
	})

	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		responses <- performReaderChapterContentLifecycleRequest(fixture, context.Background())
	}()
	select {
	case <-firstFetched:
	case <-time.After(2 * time.Second):
		t.Fatal("first chapter request did not reach the remote-fetch boundary")
	}
	go func() {
		responses <- performReaderChapterContentLifecycleRequest(fixture, context.Background())
	}()

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		var chapter models.Chapter
		if err := fixture.server.db.First(&chapter, fixture.chapter.ID).Error; err != nil {
			t.Fatal(err)
		}
		if chapter.Variable != fixture.chapter.Variable {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(releaseFirst)

	for range 2 {
		select {
		case response := <-responses:
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "shared remote content") {
				t.Errorf("concurrent same-chapter response = %d %s, want shared 200", response.Code, response.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent chapter request did not finish")
		}
	}
	if fetches.Load() != 1 {
		t.Fatalf("same chapter remote fetches = %d, want one published request", fetches.Load())
	}
}

func TestReaderDetachedSourceSnapshotStillServesExistingBook(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapterdetachedsource")
	if err := fixture.server.db.Model(&models.UserBookSource{}).
		Where("user_id = ? AND source_id = ?", fixture.user.ID, fixture.source.ID).
		Update("detached", true).Error; err != nil {
		t.Fatal(err)
	}

	var fetches atomic.Int32
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		fetches.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<main class="content">detached source content</main><span class="token">detached token</span>`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	t.Cleanup(restoreHTTPClient)

	response := performReaderChapterContentLifecycleRequest(fixture, context.Background())
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "detached source content") {
		t.Fatalf("detached source chapter = %d %s, want existing-book 200", response.Code, response.Body.String())
	}
	if fetches.Load() != 1 {
		t.Fatalf("detached source fetches = %d, want one", fetches.Load())
	}

	book, chapter := loadReaderChapterContentLifecycleState(t, fixture)
	if book.Variable != fixture.book.Variable || chapter.CachePath == "" || !strings.Contains(chapter.Variable, "detached token") {
		t.Fatalf("detached source state was not published: book=%q chapter=%q cache=%q", book.Variable, chapter.Variable, chapter.CachePath)
	}
	if cached, err := engine.ReadChapterCache(fixture.server.cfg.CacheDir, chapter.CachePath); err != nil || cached != "detached source content" {
		t.Fatalf("detached source cache = %q, %v", cached, err)
	}
	var association models.UserBookSource
	if err := fixture.server.db.Where("user_id = ? AND source_id = ?", fixture.user.ID, fixture.source.ID).
		First(&association).Error; err != nil || !association.Detached {
		t.Fatalf("chapter read reactivated detached source: %+v err=%v", association, err)
	}

	if err := os.Remove(filepath.Join(fixture.server.cfg.CacheDir, filepath.FromSlash(chapter.CachePath))); err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Model(&models.Chapter{}).
		Where("id = ? AND book_id = ?", fixture.chapter.ID, fixture.book.ID).
		Updates(map[string]any{"variable": fixture.chapter.Variable, "cache_path": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Delete(&association).Error; err != nil {
		t.Fatal(err)
	}

	response = performReaderChapterContentLifecycleRequest(fixture, context.Background())
	if response.Code != http.StatusBadGateway {
		t.Fatalf("unowned source chapter = %d %s, want rejection", response.Code, response.Body.String())
	}
	if fetches.Load() != 1 {
		t.Fatalf("unowned source made %d remote fetches, want no additional fetch", fetches.Load())
	}
	_, chapter = loadReaderChapterContentLifecycleState(t, fixture)
	if chapter.CachePath != "" || chapter.Variable != fixture.chapter.Variable {
		t.Fatalf("unowned source published chapter state: variable=%q cache=%q", chapter.Variable, chapter.CachePath)
	}
}

func TestReaderAdjacentChapterLoadsDoNotQueueBehindSameBook(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapteradjacentparallel")
	secondChapter := models.Chapter{
		BookID:   fixture.book.ID,
		Index:    1,
		Title:    "second chapter",
		URL:      fixture.source.BaseURL + "/chapter/second",
		Variable: `{"chapter":"second"}`,
	}
	if err := fixture.server.db.Create(&secondChapter).Error; err != nil {
		t.Fatal(err)
	}

	started := make(chan string, 2)
	release := make(chan struct{})
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		started <- request.URL.Path
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`<main class="content">` + request.URL.Path + `</main><span class="token">` + request.URL.Path + `</span>`,
			)),
			Header:  make(http.Header),
			Request: request,
		}, nil
	})})
	t.Cleanup(restoreHTTPClient)

	results := make(chan *httptest.ResponseRecorder, 2)
	load := func(index int) {
		results <- performReaderChapterContentLifecycleRequestAtIndex(
			fixture,
			context.Background(),
			index,
		)
	}

	go load(fixture.chapter.Index)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("first adjacent chapter did not start its remote request")
	}
	go load(secondChapter.Index)

	overlapped := false
	select {
	case <-started:
		overlapped = true
	case <-time.After(250 * time.Millisecond):
	}
	close(release)

	for range 2 {
		select {
		case response := <-results:
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "/chapter/") {
				t.Errorf("adjacent chapter response = %d %s", response.Code, response.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("adjacent chapter load did not finish")
		}
	}
	if !overlapped {
		t.Fatal("second adjacent chapter queued behind the first chapter of the same book")
	}
}

func TestReaderChapterGateWaiterCanCancelWithoutBlockingOtherBooks(t *testing.T) {
	_, server := setupTestServer(t)
	firstKey := readerChapterGateKey{userID: 1, bookID: 1, chapterID: 1}
	releaseFirst, err := server.acquireReaderChapterGate(context.Background(), firstKey)
	if err != nil {
		t.Fatal(err)
	}

	waitingContext, cancelWaiting := context.WithCancel(context.Background())
	cancelWaiting()
	if _, err := server.acquireReaderChapterGate(waitingContext, firstKey); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled gate wait error = %v, want context.Canceled", err)
	}

	releaseOther, err := server.acquireReaderChapterGate(context.Background(), readerChapterGateKey{userID: 1, bookID: 2, chapterID: 1})
	if err != nil {
		t.Fatalf("different book gate was blocked: %v", err)
	}
	releaseOther()
	releaseFirst()

	server.remoteChapterMu.Lock()
	defer server.remoteChapterMu.Unlock()
	if len(server.remoteChapterMap) != 0 {
		t.Fatalf("released chapter gates retained %d entries", len(server.remoteChapterMap))
	}
}

func TestReaderChapterContentRejectsSourceSemanticChangeAfterFetch(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapterstalesource")
	currentCachePath := ""
	installReaderChapterContentLifecycleHook(t, func(stage string, _ context.Context, book models.Book, chapter models.Chapter) {
		if stage != "after_remote_fetch" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		if err := fixture.server.db.Model(&models.BookSource{}).
			Where("id = ?", fixture.source.ID).
			Update("rules", `{"content":"main|text"}`).Error; err != nil {
			t.Errorf("update source semantics: %v", err)
		}
		if err := fixture.server.db.Model(&models.Book{}).
			Where("id = ? AND user_id = ?", fixture.book.ID, fixture.user.ID).
			Update("variable", "").Error; err != nil {
			t.Errorf("clear Book variable: %v", err)
		}
		var err error
		currentCachePath, err = engine.WriteChapterCache(
			fixture.server.cfg.CacheDir,
			fixture.book.URL,
			fixture.chapter.URL,
			"new current content",
		)
		if err != nil {
			t.Errorf("write current chapter cache: %v", err)
		}
		if err := fixture.server.db.Model(&models.Chapter{}).
			Where("id = ? AND book_id = ?", fixture.chapter.ID, fixture.book.ID).
			Updates(map[string]any{"variable": "", "cache_path": currentCachePath}).Error; err != nil {
			t.Errorf("clear Chapter state: %v", err)
		}
	})

	response := performReaderChapterContentLifecycleRequest(fixture, context.Background())
	if response.Code != http.StatusConflict || response.Body.String() != `{"error":"chapter content changed; retry"}` {
		t.Errorf("stale source result = %d %s, want safe 409", response.Code, response.Body.String())
	}

	book, chapter := loadReaderChapterContentLifecycleState(t, fixture)
	if book.Variable != "" || chapter.Variable != "" || chapter.CachePath != currentCachePath {
		t.Errorf("stale source result restored state: book=%q chapter=%q cache=%q", book.Variable, chapter.Variable, chapter.CachePath)
	}
	cached, err := engine.ReadChapterCache(fixture.server.cfg.CacheDir, currentCachePath)
	if err != nil || cached != "new current content" {
		t.Errorf("stale source result replaced current cache: content=%q err=%v", cached, err)
	}
	var failures int64
	if err := fixture.server.db.Model(&models.SourceFailure{}).
		Where("user_id = ? AND source_id = ?", fixture.user.ID, fixture.source.ID).
		Count(&failures).Error; err != nil {
		t.Fatal(err)
	}
	if failures != 0 {
		t.Fatalf("stale result wrote %d source failures", failures)
	}
}

func TestReaderChapterContentCancellationAfterFetchCommitsNothing(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chaptercancelcommit")
	ctx, cancel := context.WithCancel(context.Background())
	installReaderChapterContentLifecycleHook(t, func(stage string, _ context.Context, book models.Book, chapter models.Chapter) {
		if stage == "after_remote_fetch" && book.ID == fixture.book.ID && chapter.ID == fixture.chapter.ID {
			cancel()
		}
	})

	response := performReaderChapterContentLifecycleRequest(fixture, ctx)
	if response.Body.Len() != 0 {
		t.Errorf("cancelled content request returned %d %s", response.Code, response.Body.String())
	}
	book, chapter := loadReaderChapterContentLifecycleState(t, fixture)
	if book.Variable != fixture.book.Variable || chapter.Variable != fixture.chapter.Variable || chapter.CachePath != "" {
		t.Errorf("cancelled content request changed state: book=%q chapter=%q cache=%q", book.Variable, chapter.Variable, chapter.CachePath)
	}
	cachePath := engine.ChapterCachePath(fixture.book.URL, fixture.chapter.URL)
	if _, err := engine.ReadChapterCache(fixture.server.cfg.CacheDir, cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cancelled content request left cache %q: %v", cachePath, err)
	}
}

func TestReaderChapterCachePathNormalizationOwnsOnlyCachePath(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapternormalizecolumns")
	cachePath, err := engine.WriteChapterCache(
		fixture.server.cfg.CacheDir,
		fixture.book.URL,
		fixture.chapter.URL,
		"cached chapter",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Model(&models.Chapter{}).
		Where("id = ?", fixture.chapter.ID).
		Update("cache_path", cachePath).Error; err != nil {
		t.Fatal(err)
	}
	fixture.chapter.CachePath = cachePath

	installReaderChapterContentLifecycleHook(t, func(stage string, _ context.Context, book models.Book, chapter models.Chapter) {
		if stage != "before_cache_path_normalize" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		if err := fixture.server.db.Model(&models.Chapter{}).
			Where("id = ? AND book_id = ?", fixture.chapter.ID, fixture.book.ID).
			Updates(map[string]any{
				"title":         "concurrent title",
				"url":           fixture.source.BaseURL + "/chapter/current",
				"variable":      `{"current":"chapter"}`,
				"resource_path": "current/resource.xhtml",
			}).Error; err != nil {
			t.Errorf("write concurrent Chapter fields: %v", err)
		}
	})

	content, err := fixture.server.loadChapterTextContextResult(context.Background(), fixture.book, &fixture.chapter)
	if err != nil || content != "cached chapter" {
		t.Fatalf("cache hit = %q, %v", content, err)
	}
	_, chapter := loadReaderChapterContentLifecycleState(t, fixture)
	if chapter.Title != "concurrent title" || chapter.URL != fixture.source.BaseURL+"/chapter/current" ||
		chapter.Variable != `{"current":"chapter"}` || chapter.ResourcePath != "current/resource.xhtml" ||
		chapter.CachePath != cachePath {
		t.Fatalf("cache-path normalization overwrote Chapter columns: %+v", chapter)
	}
}

func TestReaderChapterCachePathNormalizationDoesNotReinsertReplacedChapter(t *testing.T) {
	fixture := newReaderChapterContentLifecycleFixture(t, "chapternormalizedelete")
	cachePath, err := engine.WriteChapterCache(
		fixture.server.cfg.CacheDir,
		fixture.book.URL,
		fixture.chapter.URL,
		"cached chapter",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Model(&models.Chapter{}).
		Where("id = ?", fixture.chapter.ID).
		Update("cache_path", cachePath).Error; err != nil {
		t.Fatal(err)
	}
	fixture.chapter.CachePath = cachePath

	installReaderChapterContentLifecycleHook(t, func(stage string, _ context.Context, book models.Book, chapter models.Chapter) {
		if stage != "before_cache_path_normalize" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		if err := fixture.server.db.Delete(&models.Chapter{}, fixture.chapter.ID).Error; err != nil {
			t.Errorf("delete stale Chapter: %v", err)
		}
	})

	content, err := fixture.server.loadChapterTextContextResult(context.Background(), fixture.book, &fixture.chapter)
	if err != nil || content != "cached chapter" {
		t.Fatalf("cache hit = %q, %v", content, err)
	}
	var count int64
	if err := fixture.server.db.Model(&models.Chapter{}).Where("id = ?", fixture.chapter.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("cache-path normalization reinserted deleted Chapter %d", fixture.chapter.ID)
	}
}

func TestStagedRemoteChapterCacheRollbackRestoresPreviousFile(t *testing.T) {
	_, server := setupTestServer(t)
	bookURL := "https://cache-stage.test/book"
	chapterURL := "https://cache-stage.test/chapter"
	cachePath, err := engine.WriteChapterCache(server.cfg.CacheDir, bookURL, chapterURL, "current content")
	if err != nil {
		t.Fatal(err)
	}

	staged, err := server.stageRemoteChapterCache(context.Background(), bookURL, chapterURL, "candidate content")
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if content, err := engine.ReadChapterCache(server.cfg.CacheDir, cachePath); err != nil || content != "candidate content" {
		t.Fatalf("published cache = %q, %v", content, err)
	}
	staged.rollback()
	if content, err := engine.ReadChapterCache(server.cfg.CacheDir, cachePath); err != nil || content != "current content" {
		t.Fatalf("rolled-back cache = %q, %v", content, err)
	}

	committed, err := server.stageRemoteChapterCache(context.Background(), bookURL, chapterURL, "committed content")
	if err != nil {
		t.Fatal(err)
	}
	if err := committed.publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	committed.finalize()
	committed.rollback()
	if content, err := engine.ReadChapterCache(server.cfg.CacheDir, cachePath); err != nil || content != "committed content" {
		t.Fatalf("finalized cache = %q, %v", content, err)
	}
}

type readerChapterContentLifecycleFixture struct {
	router  http.Handler
	server  *Server
	auth    string
	user    models.User
	source  models.BookSource
	book    models.Book
	chapter models.Chapter
}

func newReaderChapterContentLifecycleFixture(t *testing.T, username string) readerChapterContentLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	user := lifecycleUser(t, server, username)
	source := models.BookSource{
		Name:    username + " source",
		BaseURL: "https://" + username + ".test",
		Charset: "utf-8",
		Enabled: true,
	}
	if err := source.SetRules(models.BookSourceRule{
		ContentRule: `@put:{"contentToken":".token|text"}.content|text`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	association := models.UserBookSource{UserID: user.ID, SourceID: source.ID}
	if err := server.db.Where("user_id = ? AND source_id = ?", user.ID, source.ID).
		FirstOrCreate(&association).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:   user.ID,
		SourceID: source.ID,
		Title:    "lifecycle book",
		URL:      source.BaseURL + "/book",
		Variable: `{"book":"initial"}`,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{
		BookID:   book.ID,
		Index:    0,
		Title:    "initial chapter",
		URL:      source.BaseURL + "/chapter/old",
		Variable: `{"chapter":"initial"}`,
	}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	restoreHTTPClient := engine.SetHTTPClientForTesting(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`<main class="content">old remote content</main><span class="token">old token</span>`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})})
	t.Cleanup(restoreHTTPClient)
	return readerChapterContentLifecycleFixture{
		router: router, server: server, auth: auth, user: user, source: source, book: book, chapter: chapter,
	}
}

func performReaderChapterContentLifecycleRequest(
	fixture readerChapterContentLifecycleFixture,
	ctx context.Context,
) *httptest.ResponseRecorder {
	return performReaderChapterContentLifecycleRequestAtIndex(fixture, ctx, fixture.chapter.Index)
}

func performReaderChapterContentLifecycleRequestAtIndex(
	fixture readerChapterContentLifecycleFixture,
	ctx context.Context,
	index int,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/chapters/"+strconv.Itoa(index)+"/content",
		nil,
	).WithContext(ctx)
	request.Header.Set("Authorization", fixture.auth)
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func loadReaderChapterContentLifecycleState(
	t *testing.T,
	fixture readerChapterContentLifecycleFixture,
) (models.Book, models.Chapter) {
	t.Helper()
	var book models.Book
	var chapter models.Chapter
	if err := fixture.server.db.First(&book, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.First(&chapter, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	return book, chapter
}

func installReaderChapterContentLifecycleHook(
	t *testing.T,
	hook func(string, context.Context, models.Book, models.Chapter),
) {
	t.Helper()
	readerChapterContentLifecycleTestHook = hook
	t.Cleanup(func() { readerChapterContentLifecycleTestHook = nil })
}
