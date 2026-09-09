package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"openreader/backend/engine"
	"openreader/backend/models"
)

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
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/chapters/0/content",
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
