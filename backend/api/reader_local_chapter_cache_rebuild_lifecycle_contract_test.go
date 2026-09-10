package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestReaderLocalChapterCacheRebuildDoesNotResurrectDeletedBook(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcachedelete")
	shared := models.Book{
		UserID: fixture.user.ID, SourceID: 0, Title: "shared archive",
		URL: fixture.book.URL + "/shared", LibraryPath: fixture.book.LibraryPath,
		OriginalFile: fixture.book.OriginalFile, TOCRule: fixture.book.TOCRule,
	}
	if err := fixture.server.db.Create(&shared).Error; err != nil {
		t.Fatal(err)
	}

	installReaderLocalChapterCacheRebuildLifecycleHook(t, func(stage string, book models.Book, chapter models.Chapter) {
		if stage != "after_local_rebuild" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		readerLocalChapterCacheRebuildLifecycleTestHook = nil
		request := httptest.NewRequest(http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10), nil)
		request.Header.Set("Authorization", fixture.auth)
		response := httptest.NewRecorder()
		fixture.router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Errorf("concurrent delete = %d %s, want 204", response.Code, response.Body.String())
		}
	})

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, context.Background())
	assertReaderLocalChapterCacheRebuildLifecycleStale(t, response)
	assertReaderLocalChapterCacheRebuildLifecycleRows(t, fixture, 0, 0)
	assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
}

func TestReaderLocalChapterCacheRebuildRejectsRefreshedCatalogue(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcacherefresh")
	var replacement models.Chapter
	installReaderLocalChapterCacheRebuildLifecycleHook(t, func(stage string, book models.Book, chapter models.Chapter) {
		if stage != "after_local_rebuild" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		readerLocalChapterCacheRebuildLifecycleTestHook = nil
		request := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
		request.Header.Set("Authorization", fixture.auth)
		response := httptest.NewRecorder()
		fixture.router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Errorf("concurrent refresh = %d %s, want 200", response.Code, response.Body.String())
			return
		}
		if err := fixture.server.db.Where("book_id = ? AND `index` = ?", book.ID, chapter.Index).First(&replacement).Error; err != nil {
			t.Errorf("load replacement Chapter: %v", err)
		}
	})

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, context.Background())
	assertReaderLocalChapterCacheRebuildLifecycleStale(t, response)
	if replacement.ID == 0 || replacement.ID == fixture.chapter.ID {
		t.Fatalf("refresh did not replace Chapter identity: old=%d next=%d", fixture.chapter.ID, replacement.ID)
	}
	var current models.Chapter
	if err := fixture.server.db.Where("book_id = ? AND `index` = 0", fixture.book.ID).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.ID != replacement.ID || current.Title != replacement.Title || current.URL != replacement.URL || current.CachePath != replacement.CachePath {
		t.Errorf("late rebuild changed refreshed catalogue: current=%+v replacement=%+v", current, replacement)
	}
	assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
}

func TestReaderLocalChapterCacheRebuildRejectsChangedChapterSnapshot(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcachechapter")
	currentCachePath := filepath.Join("content", "current", "chapter.txt")
	currentCacheFile := filepath.Join(fixture.bookRoot, currentCachePath)
	if err := os.MkdirAll(filepath.Dir(currentCacheFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentCacheFile, []byte("current chapter bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	installReaderLocalChapterCacheRebuildLifecycleHook(t, func(stage string, book models.Book, chapter models.Chapter) {
		if stage != "after_local_rebuild" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		if err := fixture.server.db.Model(&models.Chapter{}).
			Where("id = ? AND book_id = ?", chapter.ID, book.ID).
			Updates(map[string]any{
				"title":                 "current chapter title",
				"url":                   book.URL + "/current-chapter",
				"cache_path":            currentCachePath,
				"resource_path":         "current/chapter.xhtml",
				"resource_fragment":     "current-start",
				"resource_end_fragment": "current-end",
				"variable":              `{"current":"chapter"}`,
			}).Error; err != nil {
			t.Errorf("change current Chapter snapshot: %v", err)
		}
	})

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, context.Background())
	assertReaderLocalChapterCacheRebuildLifecycleStale(t, response)
	var current models.Chapter
	if err := fixture.server.db.First(&current, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Title != "current chapter title" || current.URL != fixture.book.URL+"/current-chapter" ||
		current.CachePath != currentCachePath || current.ResourcePath != "current/chapter.xhtml" ||
		current.ResourceFragment != "current-start" || current.ResourceEndFragment != "current-end" ||
		current.Variable != `{"current":"chapter"}` {
		t.Errorf("late rebuild overwrote current Chapter columns: %+v", current)
	}
	if data, err := os.ReadFile(currentCacheFile); err != nil || string(data) != "current chapter bytes" {
		t.Errorf("late rebuild changed current cache: data=%q err=%v", data, err)
	}
	assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
}

func TestReaderLocalChapterCacheRebuildCancellationCommitsNothing(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcachecancel")
	ctx, cancel := context.WithCancel(context.Background())
	installReaderLocalChapterCacheRebuildLifecycleHook(t, func(stage string, book models.Book, chapter models.Chapter) {
		if stage == "after_local_rebuild" && book.ID == fixture.book.ID && chapter.ID == fixture.chapter.ID {
			cancel()
		}
	})

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, ctx)
	if response.Body.Len() != 0 {
		t.Errorf("cancelled request returned %d %s", response.Code, response.Body.String())
	}
	var current models.Chapter
	if err := fixture.server.db.First(&current, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.URL != fixture.chapter.URL || current.CachePath != fixture.chapter.CachePath {
		t.Errorf("cancelled rebuild changed Chapter: %+v", current)
	}
	assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
}

func TestReaderLocalChapterCacheRebuildRollsBackDatabaseFailure(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcachedbfailure")
	if err := fixture.server.db.Exec(`
		CREATE TRIGGER fail_reader_local_cache_update
		BEFORE UPDATE OF cache_path ON chapters
		BEGIN
			SELECT RAISE(FAIL, 'injected local cache update failure');
		END
	`).Error; err != nil {
		t.Fatal(err)
	}

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, context.Background())
	if response.Code != http.StatusBadGateway {
		t.Errorf("database failure = %d %s, want 502", response.Code, response.Body.String())
	}
	var current models.Chapter
	if err := fixture.server.db.First(&current, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.URL != fixture.chapter.URL || current.CachePath != fixture.chapter.CachePath {
		t.Errorf("database failure changed Chapter: %+v", current)
	}
	assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
}

func TestReaderLocalChapterCacheRebuildCancellationPhasesCommitNothing(t *testing.T) {
	for _, stage := range []string{"after_local_cache_stage", "before_local_cache_update", "before_local_cache_publish"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcancel"+strings.ReplaceAll(stage, "_", ""))
			ctx, cancel := context.WithCancel(context.Background())
			installReaderLocalChapterCacheRebuildLifecycleHook(t, func(currentStage string, book models.Book, chapter models.Chapter) {
				if currentStage == stage && book.ID == fixture.book.ID && chapter.ID == fixture.chapter.ID {
					cancel()
				}
			})

			response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, ctx)
			if response.Body.Len() != 0 {
				t.Errorf("cancelled %s request returned %d %s", stage, response.Code, response.Body.String())
			}
			assertReaderLocalChapterCacheRebuildLifecycleChapterUnchanged(t, fixture)
			assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
			assertReaderLocalChapterCacheRebuildLifecycleNoSidecars(t, fixture)
		})
	}
}

func TestReaderLocalChapterCacheRebuildRejectsReplacedSourceFile(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcachesource")
	sourceFile := filepath.Join(fixture.server.cfg.LibraryDir, fixture.book.OriginalFile)
	installReaderLocalChapterCacheRebuildLifecycleHook(t, func(stage string, book models.Book, chapter models.Chapter) {
		if stage != "after_local_rebuild" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		if err := os.Rename(sourceFile, sourceFile+".old"); err != nil {
			t.Errorf("detach parsed source: %v", err)
			return
		}
		if err := os.WriteFile(sourceFile, []byte("第一章 本地回建\n替换后的正文。\n"), 0o644); err != nil {
			t.Errorf("replace parsed source: %v", err)
		}
	})

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, context.Background())
	assertReaderLocalChapterCacheRebuildLifecycleStale(t, response)
	assertReaderLocalChapterCacheRebuildLifecycleChapterUnchanged(t, fixture)
	assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
}

func TestReaderLocalChapterCacheRebuildRollsBackPublishFailure(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcachepublish")
	installReaderLocalChapterCacheRebuildLifecycleHook(t, func(stage string, book models.Book, chapter models.Chapter) {
		if stage != "before_local_cache_publish" || book.ID != fixture.book.ID || chapter.ID != fixture.chapter.ID {
			return
		}
		entries, err := os.ReadDir(filepath.Dir(fixture.cacheFile))
		if err != nil {
			t.Errorf("read staged cache directory: %v", err)
			return
		}
		prefix := filepath.Base(fixture.cacheFile) + ".stage-"
		removed := false
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), prefix) {
				removed = true
				if err := os.Remove(filepath.Join(filepath.Dir(fixture.cacheFile), entry.Name())); err != nil {
					t.Errorf("remove staged cache: %v", err)
				}
			}
		}
		if !removed {
			t.Error("local cache stage was not present before publish")
		}
	})

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, context.Background())
	if response.Code != http.StatusBadGateway {
		t.Errorf("publish failure = %d %s, want 502", response.Code, response.Body.String())
	}
	assertReaderLocalChapterCacheRebuildLifecycleChapterUnchanged(t, fixture)
	assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t, fixture)
	assertReaderLocalChapterCacheRebuildLifecycleNoSidecars(t, fixture)
}

func TestReaderLocalChapterCacheRebuildCoordinatesSameChapterRequests(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcacheconcurrent")
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	installReaderLocalChapterCacheRebuildLifecycleHook(t, func(stage string, book models.Book, chapter models.Chapter) {
		if stage == "after_local_rebuild" && book.ID == fixture.book.ID && chapter.ID == fixture.chapter.ID {
			arrived <- struct{}{}
			<-release
		}
	})

	responses := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	done := make(chan struct{}, len(responses))
	for _, response := range responses {
		go func(response *httptest.ResponseRecorder) {
			request := httptest.NewRequest(
				http.MethodGet,
				"/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/chapters/0/content",
				nil,
			)
			request.Header.Set("Authorization", fixture.auth)
			fixture.router.ServeHTTP(response, request)
			done <- struct{}{}
		}(response)
	}
	for range responses {
		select {
		case <-arrived:
		case <-time.After(3 * time.Second):
			close(release)
			t.Fatal("same-chapter requests did not both reach the parse barrier")
		}
	}
	close(release)
	for range responses {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("same-chapter request did not finish")
		}
	}

	okCount := 0
	staleCount := 0
	for _, response := range responses {
		switch {
		case response.Code == http.StatusOK:
			okCount++
		case response.Code == http.StatusConflict && response.Body.String() == `{"error":"chapter content changed; retry"}`:
			staleCount++
		default:
			t.Errorf("same-chapter response = %d %s", response.Code, response.Body.String())
		}
	}
	if okCount != 1 || staleCount != 1 {
		t.Errorf("same-chapter results = ok:%d stale:%d, want one of each", okCount, staleCount)
	}
	var current models.Chapter
	if err := fixture.server.db.First(&current, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.URL != "" || current.CachePath == "" {
		t.Errorf("same-chapter commit persisted synthetic URL or missed cache path: %+v", current)
	}
	if data, err := os.ReadFile(filepath.Join(fixture.bookRoot, current.CachePath)); err != nil || string(data) != "旧解析正文。" {
		t.Errorf("same-chapter current cache = %q, %v", data, err)
	}
	assertReaderLocalChapterCacheRebuildLifecycleNoSidecars(t, fixture)
}

func TestReaderLocalChapterCacheRebuildRestoresMissingCanonicalFile(t *testing.T) {
	fixture := newReaderLocalChapterCacheRebuildLifecycleFixture(t, "localcachecanonical")
	canonicalPath, ok := relativePathInside(fixture.bookRoot, fixture.cacheFile)
	if !ok {
		t.Fatal("fixture cache path is outside the local archive")
	}
	if err := fixture.server.db.Model(&models.Chapter{}).
		Where("id = ? AND book_id = ?", fixture.chapter.ID, fixture.book.ID).
		UpdateColumn("cache_path", canonicalPath).Error; err != nil {
		t.Fatal(err)
	}
	fixture.chapter.CachePath = canonicalPath

	response := performReaderLocalChapterCacheRebuildLifecycleRequest(fixture, context.Background())
	if response.Code != http.StatusOK {
		t.Fatalf("canonical cache rebuild = %d %s, want 200", response.Code, response.Body.String())
	}
	var current models.Chapter
	if err := fixture.server.db.First(&current, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.CachePath != canonicalPath || current.URL != "" {
		t.Errorf("canonical cache rebuild changed catalogue identity: %+v", current)
	}
	if data, err := os.ReadFile(fixture.cacheFile); err != nil || string(data) != "旧解析正文。" {
		t.Errorf("canonical cache rebuild = %q, %v", data, err)
	}
	assertReaderLocalChapterCacheRebuildLifecycleNoSidecars(t, fixture)
}

func TestReaderLocalChapterCacheSnapshotIdentity(t *testing.T) {
	book := models.Book{
		ID: 3, UserID: 5, SourceID: 0, Type: 0, Title: "metadata",
		URL: "local://book", LibraryPath: "data/user/book", OriginalFile: "data/user/book/source.txt",
		TOCFile: "data/user/book/chapters.json", SourceFile: "data/user/book/bookSource.json", TOCRule: "^chapter",
	}
	metadataEdit := book
	metadataEdit.Title = "concurrent metadata"
	metadataEdit.Author = "concurrent author"
	if !sameReaderLocalChapterCacheBookSnapshot(metadataEdit, book) {
		t.Fatal("ordinary Book metadata edit invalidated the local parse snapshot")
	}
	bookChanges := []func(*models.Book){
		func(value *models.Book) { value.ID++ },
		func(value *models.Book) { value.UserID++ },
		func(value *models.Book) { value.SourceID++ },
		func(value *models.Book) { value.Type++ },
		func(value *models.Book) { value.URL += "/changed" },
		func(value *models.Book) { value.LibraryPath += "/changed" },
		func(value *models.Book) { value.OriginalFile += ".changed" },
		func(value *models.Book) { value.TOCFile += ".changed" },
		func(value *models.Book) { value.SourceFile += ".changed" },
		func(value *models.Book) { value.TOCRule += ".changed" },
	}
	for index, change := range bookChanges {
		current := book
		change(&current)
		if sameReaderLocalChapterCacheBookSnapshot(current, book) {
			t.Errorf("Book parse identity change %d was accepted", index)
		}
	}

	chapter := models.Chapter{
		ID: 7, BookID: book.ID, Index: 1, Title: "chapter", URL: "local://chapter",
		Tag: "tag", CachePath: "content/old", ResourcePath: "OPS/one.xhtml",
		ResourceFragment: "start", ResourceEndFragment: "end", Variable: `{"old":true}`,
	}
	chapterChanges := []func(*models.Chapter){
		func(value *models.Chapter) { value.ID++ },
		func(value *models.Chapter) { value.BookID++ },
		func(value *models.Chapter) { value.Index++ },
		func(value *models.Chapter) { value.Title += " changed" },
		func(value *models.Chapter) { value.URL += "/changed" },
		func(value *models.Chapter) { value.IsVolume = !value.IsVolume },
		func(value *models.Chapter) { value.Tag += " changed" },
		func(value *models.Chapter) { value.CachePath += ".changed" },
		func(value *models.Chapter) { value.ResourcePath += ".changed" },
		func(value *models.Chapter) { value.ResourceFragment += ".changed" },
		func(value *models.Chapter) { value.ResourceEndFragment += ".changed" },
		func(value *models.Chapter) { value.Variable += " changed" },
	}
	for index, change := range chapterChanges {
		current := chapter
		change(&current)
		if sameReaderLocalChapterCacheChapterSnapshot(current, chapter) {
			t.Errorf("Chapter parse identity change %d was accepted", index)
		}
	}
}

type readerLocalChapterCacheRebuildLifecycleFixture struct {
	router    http.Handler
	server    *Server
	auth      string
	user      models.User
	book      models.Book
	chapter   models.Chapter
	bookRoot  string
	cacheFile string
}

func newReaderLocalChapterCacheRebuildLifecycleFixture(t *testing.T, username string) readerLocalChapterCacheRebuildLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	user := lifecycleUser(t, server, username)
	libraryPath := filepath.Join("data", username, "local-cache-rebuild")
	bookRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
	originalFile := filepath.Join(libraryPath, "source.txt")
	if err := os.MkdirAll(bookRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(server.cfg.LibraryDir, originalFile),
		[]byte("第一章 本地回建\n旧解析正文。\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: user.ID, SourceID: 0, Title: "local cache rebuild book",
		URL: "local://" + username + "/book", LibraryPath: libraryPath,
		OriginalFile: originalFile, TOCRule: `^第一章.*$`, ChapterCount: 1,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章 本地回建"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	chapterURL := "local://book_" + strconv.FormatUint(uint64(book.ID), 10) + "/chapter_0"
	cacheRelative := filepath.Join("content", engine.ChapterCachePath(book.URL, chapterURL))
	return readerLocalChapterCacheRebuildLifecycleFixture{
		router: router, server: server, auth: auth, user: user, book: book, chapter: chapter,
		bookRoot: bookRoot, cacheFile: filepath.Join(bookRoot, cacheRelative),
	}
}

func performReaderLocalChapterCacheRebuildLifecycleRequest(
	fixture readerLocalChapterCacheRebuildLifecycleFixture,
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

func assertReaderLocalChapterCacheRebuildLifecycleStale(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusConflict || response.Body.String() != `{"error":"chapter content changed; retry"}` {
		t.Errorf("stale local rebuild = %d %s, want stable 409", response.Code, response.Body.String())
	}
}

func assertReaderLocalChapterCacheRebuildLifecycleRows(
	t *testing.T,
	fixture readerLocalChapterCacheRebuildLifecycleFixture,
	wantBooks int64,
	wantChapters int64,
) {
	t.Helper()
	var books int64
	var chapters int64
	if err := fixture.server.db.Model(&models.Book{}).Where("id = ?", fixture.book.ID).Count(&books).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Model(&models.Chapter{}).Where("book_id = ?", fixture.book.ID).Count(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if books != wantBooks || chapters != wantChapters {
		t.Errorf("durable rows = books:%d chapters:%d, want books:%d chapters:%d", books, chapters, wantBooks, wantChapters)
	}
}

func assertReaderLocalChapterCacheRebuildLifecycleFileAbsent(t *testing.T, fixture readerLocalChapterCacheRebuildLifecycleFixture) {
	t.Helper()
	if _, err := os.Stat(fixture.cacheFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("late local rebuild left final cache %q: %v", fixture.cacheFile, err)
	}
}

func assertReaderLocalChapterCacheRebuildLifecycleChapterUnchanged(
	t *testing.T,
	fixture readerLocalChapterCacheRebuildLifecycleFixture,
) {
	t.Helper()
	var current models.Chapter
	if err := fixture.server.db.First(&current, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !sameReaderLocalChapterCacheChapterSnapshot(current, fixture.chapter) {
		t.Errorf("local rebuild changed Chapter: current=%+v initial=%+v", current, fixture.chapter)
	}
}

func assertReaderLocalChapterCacheRebuildLifecycleNoSidecars(
	t *testing.T,
	fixture readerLocalChapterCacheRebuildLifecycleFixture,
) {
	t.Helper()
	err := filepath.WalkDir(fixture.bookRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.Contains(entry.Name(), ".stage-") || strings.Contains(entry.Name(), ".backup-") {
			t.Errorf("local rebuild left sidecar %q", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func installReaderLocalChapterCacheRebuildLifecycleHook(
	t *testing.T,
	hook func(string, models.Book, models.Chapter),
) {
	t.Helper()
	readerLocalChapterCacheRebuildLifecycleTestHook = hook
	t.Cleanup(func() { readerLocalChapterCacheRebuildLifecycleTestHook = nil })
}
