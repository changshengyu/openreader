package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"openreader/backend/models"
)

func TestLocalBookRefreshCancellationBeforeReadHasNoBusinessSideEffects(t *testing.T) {
	fixture := newLocalBookRefreshLifecycleFixture(t, "localrefreshcancel", false)
	before := snapshotLocalBookRefreshLifecycle(t, fixture)
	blocker := installLocalBookRefreshOpenBlocker(t)

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, localBookRefreshPath(fixture.book.ID), nil).WithContext(ctx)
	request.Header.Set("Authorization", fixture.auth)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		fixture.router.ServeHTTP(response, request)
		close(done)
	}()

	blocker.wait(t, "local refresh did not open the archived source")
	cancel()
	blocker.unblock()
	waitLocalBookRefreshHandler(t, done)

	if response.Body.Len() != 0 {
		t.Errorf("cancelled local refresh wrote a business response: %d %s", response.Code, response.Body.String())
	}
	after := snapshotLocalBookRefreshLifecycle(t, fixture)
	if !bytes.Equal(after, before) {
		t.Errorf("cancelled local refresh changed durable state\nbefore=%s\nafter=%s", before, after)
	}
	if queuedPayloadContains(fixture.events, `"type":"bookshelf_update"`) {
		t.Error("cancelled local refresh broadcast a shelf update")
	}
}

func TestLocalBookRefreshRejectsDeletedBookAfterStage(t *testing.T) {
	fixture := newLocalBookRefreshLifecycleFixture(t, "localrefreshdelete", true)
	oldTOC := readLocalRefreshLifecycleFile(t, fixture.tocPath)
	oldSource := readLocalRefreshLifecycleFile(t, fixture.sourceMetadataPath)
	blocker := installLocalBookRefreshStageBlocker(t)

	response, done := startLocalBookRefreshLifecycleRequest(fixture)
	blocker.wait(t, "local refresh did not finish its inactive stage")

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10), nil)
	deleteRequest.Header.Set("Authorization", fixture.auth)
	deleteResponse := httptest.NewRecorder()
	fixture.router.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("concurrent delete = %d %s, want 204", deleteResponse.Code, deleteResponse.Body.String())
	}
	drainLocalBookRefreshEvents(fixture.events)

	blocker.unblock()
	waitLocalBookRefreshHandler(t, done)
	assertLocalBookRefreshStaleResponse(t, response)

	var bookCount, chapterCount int64
	if err := fixture.server.db.Model(&models.Book{}).Where("id = ?", fixture.book.ID).Count(&bookCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Model(&models.Chapter{}).Where("book_id = ?", fixture.book.ID).Count(&chapterCount).Error; err != nil {
		t.Fatal(err)
	}
	if bookCount != 0 || chapterCount != 0 {
		t.Errorf("late local refresh resurrected deleted state: books=%d chapters=%d", bookCount, chapterCount)
	}
	if got := readLocalRefreshLifecycleFile(t, fixture.tocPath); !bytes.Equal(got, oldTOC) {
		t.Errorf("late local refresh replaced active TOC metadata after delete: %q", got)
	}
	if got := readLocalRefreshLifecycleFile(t, fixture.sourceMetadataPath); !bytes.Equal(got, oldSource) {
		t.Errorf("late local refresh replaced active source metadata after delete: %q", got)
	}
	assertNoLocalRefreshInactiveStage(t, fixture.bookRoot)
	if queuedPayloadContains(fixture.events, `"type":"bookshelf_update"`) {
		t.Error("stale local refresh broadcast a shelf update after delete")
	}
}

func TestLocalBookRefreshRejectsConcurrentBookEditAfterStage(t *testing.T) {
	fixture := newLocalBookRefreshLifecycleFixture(t, "localrefreshedit", false)
	oldTOC := readLocalRefreshLifecycleFile(t, fixture.tocPath)
	oldSource := readLocalRefreshLifecycleFile(t, fixture.sourceMetadataPath)
	blocker := installLocalBookRefreshStageBlocker(t)

	response, done := startLocalBookRefreshLifecycleRequest(fixture)
	blocker.wait(t, "local refresh did not finish its inactive stage")

	editBody := `{"title":"并发编辑后的标题","author":"并发作者","intro":"并发简介","canUpdate":false}`
	editRequest := httptest.NewRequest(http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10), strings.NewReader(editBody))
	editRequest.Header.Set("Authorization", fixture.auth)
	editRequest.Header.Set("Content-Type", "application/json")
	editResponse := httptest.NewRecorder()
	fixture.router.ServeHTTP(editResponse, editRequest)
	if editResponse.Code != http.StatusOK {
		t.Fatalf("concurrent book edit = %d %s, want 200", editResponse.Code, editResponse.Body.String())
	}
	var edited models.Book
	if err := fixture.server.db.First(&edited, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	var oldChapter models.Chapter
	if err := fixture.server.db.Where("book_id = ? AND `index` = 0", fixture.book.ID).First(&oldChapter).Error; err != nil {
		t.Fatal(err)
	}
	drainLocalBookRefreshEvents(fixture.events)

	blocker.unblock()
	waitLocalBookRefreshHandler(t, done)
	assertLocalBookRefreshStaleResponse(t, response)

	var current models.Book
	if err := fixture.server.db.First(&current, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Title != edited.Title || current.Author != edited.Author || current.Intro != edited.Intro || current.CanUpdate != edited.CanUpdate {
		t.Errorf("late local refresh overwrote concurrent metadata: got %+v want %+v", current, edited)
	}
	var chapters []models.Chapter
	if err := fixture.server.db.Where("book_id = ?", fixture.book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].ID != oldChapter.ID || chapters[0].CachePath != oldChapter.CachePath {
		t.Errorf("late local refresh replaced the current catalogue: %+v", chapters)
	}
	if got := readLocalRefreshLifecycleFile(t, fixture.tocPath); !bytes.Equal(got, oldTOC) {
		t.Errorf("late local refresh replaced active TOC metadata after edit: %q", got)
	}
	if got := readLocalRefreshLifecycleFile(t, fixture.sourceMetadataPath); !bytes.Equal(got, oldSource) {
		t.Errorf("late local refresh replaced active source metadata after edit: %q", got)
	}
	assertNoLocalRefreshInactiveStage(t, fixture.bookRoot)
	if queuedPayloadContains(fixture.events, `"type":"bookshelf_update"`) {
		t.Error("stale local refresh broadcast a shelf update after edit")
	}
}

type localBookRefreshLifecycleFixture struct {
	router             http.Handler
	server             *Server
	auth               string
	user               models.User
	book               models.Book
	chapter            models.Chapter
	events             <-chan []byte
	bookRoot           string
	tocPath            string
	sourceMetadataPath string
}

func newLocalBookRefreshLifecycleFixture(t *testing.T, username string, sharedArchive bool) localBookRefreshLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	user := lifecycleUser(t, server, username)
	libraryPath := filepath.Join("data", username, "local-refresh-lifecycle")
	bookRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
	originalFile := filepath.Join(libraryPath, "source.txt")
	tocFile := filepath.Join(libraryPath, "chapters.json")
	sourceFile := filepath.Join(libraryPath, "bookSource.json")
	if err := os.MkdirAll(bookRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LibraryDir, originalFile), []byte("第一章 刷新后目录\n刷新后的正文。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTOC := []byte("[{\"title\":\"刷新前目录\"}]\n")
	oldSource := []byte("[{\"name\":\"刷新前本地元数据\"}]\n")
	if err := os.WriteFile(filepath.Join(server.cfg.LibraryDir, tocFile), oldTOC, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LibraryDir, sourceFile), oldSource, 0o644); err != nil {
		t.Fatal(err)
	}
	oldCachePath := filepath.Join("content", "active-before", "chapter.txt")
	if err := os.MkdirAll(filepath.Join(bookRoot, filepath.Dir(oldCachePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bookRoot, oldCachePath), []byte("刷新前正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := models.Book{
		UserID: user.ID, SourceID: 0, Title: "刷新前标题", Author: "刷新前作者", Intro: "刷新前简介",
		CoverURL: "https://cover.example/before.jpg", URL: "local://" + username,
		LibraryPath: libraryPath, OriginalFile: originalFile, TOCFile: tocFile, SourceFile: sourceFile,
		TOCRule: `^第一章.*$`, LastChapter: "刷新前目录", ChapterCount: 1, Variable: `{"before":"value"}`,
		CanUpdate: true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "刷新前目录", URL: book.URL + "/chapter_0", CachePath: oldCachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 17, Percent: 0.25}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 9, Title: "刷新前书签"}).Error; err != nil {
		t.Fatal(err)
	}
	if sharedArchive {
		shared := models.Book{
			UserID: user.ID, SourceID: 0, Title: "共享归档引用", URL: "local://" + username + "/shared",
			LibraryPath: libraryPath, OriginalFile: originalFile, TOCFile: tocFile, SourceFile: sourceFile,
		}
		if err := server.db.Create(&shared).Error; err != nil {
			t.Fatal(err)
		}
	}
	events := server.hub.AddClient(user.ID, nil).Send
	return localBookRefreshLifecycleFixture{
		router: router, server: server, auth: auth, user: user, book: book, chapter: chapter, events: events,
		bookRoot: bookRoot, tocPath: filepath.Join(server.cfg.LibraryDir, tocFile),
		sourceMetadataPath: filepath.Join(server.cfg.LibraryDir, sourceFile),
	}
}

type localBookRefreshBlocker struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newLocalBookRefreshBlocker() *localBookRefreshBlocker {
	return &localBookRefreshBlocker{started: make(chan struct{}), release: make(chan struct{})}
}

func (blocker *localBookRefreshBlocker) block() {
	blocker.startedOnce.Do(func() { close(blocker.started) })
	<-blocker.release
}

func (blocker *localBookRefreshBlocker) unblock() {
	blocker.releaseOnce.Do(func() { close(blocker.release) })
}

func (blocker *localBookRefreshBlocker) wait(t *testing.T, message string) {
	t.Helper()
	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		blocker.unblock()
		t.Fatal(message)
	}
}

func installLocalBookRefreshOpenBlocker(t *testing.T) *localBookRefreshBlocker {
	t.Helper()
	blocker := newLocalBookRefreshBlocker()
	localBookArchiveOpenTestHook = func(string) { blocker.block() }
	t.Cleanup(func() {
		blocker.unblock()
		localBookArchiveOpenTestHook = nil
	})
	return blocker
}

func installLocalBookRefreshStageBlocker(t *testing.T) *localBookRefreshBlocker {
	t.Helper()
	blocker := newLocalBookRefreshBlocker()
	localRefreshStageTestHook = func(string) error {
		blocker.block()
		return nil
	}
	t.Cleanup(func() {
		blocker.unblock()
		localRefreshStageTestHook = nil
	})
	return blocker
}

func startLocalBookRefreshLifecycleRequest(fixture localBookRefreshLifecycleFixture) (*httptest.ResponseRecorder, <-chan struct{}) {
	request := httptest.NewRequest(http.MethodPost, localBookRefreshPath(fixture.book.ID), nil)
	request.Header.Set("Authorization", fixture.auth)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		fixture.router.ServeHTTP(response, request)
		close(done)
	}()
	return response, done
}

func localBookRefreshPath(bookID uint) string {
	return "/api/books/" + strconv.FormatUint(uint64(bookID), 10) + "/refresh-local"
}

func waitLocalBookRefreshHandler(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("local refresh handler did not finish")
	}
}

func assertLocalBookRefreshStaleResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusConflict || response.Body.String() != `{"error":"book changed during refresh"}` {
		t.Errorf("stale local refresh = %d %s, want stable 409", response.Code, response.Body.String())
	}
}

func snapshotLocalBookRefreshLifecycle(t *testing.T, fixture localBookRefreshLifecycleFixture) []byte {
	t.Helper()
	var snapshot struct {
		Book       models.Book
		Chapters   []models.Chapter
		Progress   []models.ReadingProgress
		Bookmarks  []models.Bookmark
		TOC        string
		Source     string
		Cache      string
		StageNames []string
	}
	if err := fixture.server.db.First(&snapshot.Book, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Where("book_id = ?", fixture.book.ID).Order("id asc").Find(&snapshot.Chapters).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Where("book_id = ?", fixture.book.ID).Order("id asc").Find(&snapshot.Progress).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Where("book_id = ?", fixture.book.ID).Order("id asc").Find(&snapshot.Bookmarks).Error; err != nil {
		t.Fatal(err)
	}
	snapshot.TOC = string(readLocalRefreshLifecycleFile(t, fixture.tocPath))
	snapshot.Source = string(readLocalRefreshLifecycleFile(t, fixture.sourceMetadataPath))
	snapshot.Cache = string(readLocalRefreshLifecycleFile(t, filepath.Join(fixture.bookRoot, fixture.chapter.CachePath)))
	entries, err := os.ReadDir(fixture.bookRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".refresh-") {
			snapshot.StageNames = append(snapshot.StageNames, entry.Name())
		}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func readLocalRefreshLifecycleFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertNoLocalRefreshInactiveStage(t *testing.T, bookRoot string) {
	t.Helper()
	entries, err := os.ReadDir(bookRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".refresh-") {
			t.Errorf("local refresh left inactive stage %q", entry.Name())
		}
	}
}

func drainLocalBookRefreshEvents(events <-chan []byte) {
	for {
		select {
		case <-events:
		default:
			return
		}
	}
}
