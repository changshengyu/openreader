package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestReaderSourceChangeRejectsDeletedBookAfterFetch(t *testing.T) {
	fixture := newReaderSourceChangeWriteLifecycleFixture(t, "sourcechangedelete")
	started, _, release := installBlockingRemoteBookLifecycleClient(t)
	response, done := startReaderSourceChangeWriteLifecycleRequest(fixture, context.Background())
	waitRemoteBookLifecycleSignal(t, started, release, "source change did not reach the remote transport")

	if err := fixture.server.db.Transaction(func(tx *gorm.DB) error {
		book := fixture.book
		return deleteBookRecords(tx, fixture.owner.ID, book.ID, &book)
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitReaderSourceChangeWriteLifecycleRequest(t, done)
	assertReaderSourceChangeWriteLifecycleStale(t, response)

	for _, model := range []struct {
		name  string
		value any
		where string
		args  []any
	}{
		{name: "Book", value: &models.Book{}, where: "id = ?", args: []any{fixture.book.ID}},
		{name: "Chapter", value: &models.Chapter{}, where: "book_id = ?", args: []any{fixture.book.ID}},
		{name: "Progress", value: &models.ReadingProgress{}, where: "book_id = ?", args: []any{fixture.book.ID}},
		{name: "Bookmark", value: &models.Bookmark{}, where: "book_id = ?", args: []any{fixture.book.ID}},
		{name: "Candidate", value: &models.BookSourceCandidate{}, where: "book_id = ?", args: []any{fixture.book.ID}},
	} {
		var count int64
		if err := fixture.server.db.Model(model.value).Where(model.where, model.args...).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("late source change resurrected %s rows: %d", model.name, count)
		}
	}
	if data, err := os.ReadFile(fixture.cacheFile); err != nil || string(data) != fixture.cacheContent {
		t.Errorf("stale source change removed old cache: data=%q err=%v", data, err)
	}
	assertReaderSourceChangeWriteLifecycleNoEvents(t, fixture.events)
}

func TestReaderSourceChangeRejectsNewerSourceAfterFetch(t *testing.T) {
	fixture := newReaderSourceChangeWriteLifecycleFixture(t, "sourcechangenewer")
	started, _, release := installBlockingRemoteBookLifecycleClient(t)
	response, done := startReaderSourceChangeWriteLifecycleRequest(fixture, context.Background())
	waitRemoteBookLifecycleSignal(t, started, release, "source change did not reach the remote transport")

	var replacementChapter models.Chapter
	if err := fixture.server.db.Transaction(func(tx *gorm.DB) error {
		var current models.Book
		if err := tx.Where("id = ? AND user_id = ?", fixture.book.ID, fixture.owner.ID).First(&current).Error; err != nil {
			return err
		}
		next := []models.Chapter{{
			BookID: current.ID, Index: 0, Title: "newer source chapter",
			URL: fixture.replacementSource.BaseURL + "/newer-chapter", Variable: `{"newer":"chapter"}`,
		}}
		if _, _, err := fixture.server.replaceBookChapterRows(tx, fixture.owner.ID, current.ID, next); err != nil {
			return err
		}
		if err := tx.Model(&models.Book{}).Where("id = ? AND user_id = ?", current.ID, current.UserID).
			Updates(map[string]any{
				"source_id":     fixture.replacementSource.ID,
				"url":           fixture.replacementSource.BaseURL + "/newer-book",
				"variable":      `{"newer":"book"}`,
				"last_chapter":  "newer source chapter",
				"chapter_count": 1,
			}).Error; err != nil {
			return err
		}
		if err := tx.Where("book_id = ? AND `index` = 0", current.ID).First(&replacementChapter).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ? AND user_id = ?", current.ID, current.UserID).First(&current).Error; err != nil {
			return err
		}
		return fixture.server.sourceCandidates.SeedCurrent(tx, current, &fixture.replacementSource)
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitReaderSourceChangeWriteLifecycleRequest(t, done)
	assertReaderSourceChangeWriteLifecycleStale(t, response)

	current := loadReaderSourceChangeWriteLifecycleBook(t, fixture)
	if current.SourceID != fixture.replacementSource.ID || current.URL != fixture.replacementSource.BaseURL+"/newer-book" ||
		current.Variable != `{"newer":"book"}` {
		t.Errorf("late source change overwrote newer source identity: %+v", current)
	}
	var chapters []models.Chapter
	if err := fixture.server.db.Where("book_id = ?", fixture.book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].ID != replacementChapter.ID || chapters[0].URL != replacementChapter.URL ||
		chapters[0].Variable != replacementChapter.Variable {
		t.Errorf("late source change replaced newer catalogue: %+v", chapters)
	}
	if countReaderSourceChangeTargetCandidates(t, fixture) != 0 {
		t.Error("late source change seeded a candidate for its stale target")
	}
	assertReaderSourceChangeWriteLifecycleNoEvents(t, fixture.events)
}

func TestReaderSourceChangeRejectsChangedTargetSourceAfterFetch(t *testing.T) {
	fixture := newReaderSourceChangeWriteLifecycleFixture(t, "sourcechangetarget")
	started, _, release := installBlockingRemoteBookLifecycleClient(t)
	response, done := startReaderSourceChangeWriteLifecycleRequest(fixture, context.Background())
	waitRemoteBookLifecycleSignal(t, started, release, "source change did not reach the remote transport")

	if err := fixture.server.db.Model(&models.BookSource{}).Where("id = ?", fixture.targetSource.ID).
		Updates(map[string]any{"rules": `{"chapterListRule":".changed"}`, "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	close(release)
	waitReaderSourceChangeWriteLifecycleRequest(t, done)
	assertReaderSourceChangeWriteLifecycleStale(t, response)

	current := loadReaderSourceChangeWriteLifecycleBook(t, fixture)
	if current.SourceID != fixture.oldSource.ID || current.URL != fixture.book.URL || current.Variable != fixture.book.Variable {
		t.Errorf("stale target source result changed Book: %+v", current)
	}
	var chapters []models.Chapter
	if err := fixture.server.db.Where("book_id = ?", fixture.book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].ID != fixture.chapter.ID || chapters[0].URL != fixture.chapter.URL ||
		chapters[0].CachePath != fixture.chapter.CachePath {
		t.Errorf("stale target source result replaced catalogue: %+v", chapters)
	}
	if countReaderSourceChangeTargetCandidates(t, fixture) != 0 {
		t.Error("stale target source result seeded a candidate")
	}
	var failureCount int64
	if err := fixture.server.db.Model(&models.SourceFailure{}).
		Where("user_id = ? AND source_id = ?", fixture.owner.ID, fixture.targetSource.ID).
		Count(&failureCount).Error; err != nil {
		t.Fatal(err)
	}
	if failureCount != 0 {
		t.Errorf("stale target source result recorded %d source failures", failureCount)
	}
	if data, err := os.ReadFile(fixture.cacheFile); err != nil || string(data) != fixture.cacheContent {
		t.Errorf("stale target source result removed old cache: data=%q err=%v", data, err)
	}
	assertReaderSourceChangeWriteLifecycleNoEvents(t, fixture.events)
}

func TestReaderSourceChangePreservesConcurrentUnownedColumnsAndReloadsProjection(t *testing.T) {
	fixture := newReaderSourceChangeWriteLifecycleFixture(t, "sourcechangecolumns")
	started, _, release := installBlockingRemoteBookLifecycleClient(t)
	response, done := startReaderSourceChangeWriteLifecycleRequest(fixture, context.Background())
	waitRemoteBookLifecycleSignal(t, started, release, "source change did not reach the remote transport")

	if err := fixture.server.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Book{}).Where("id = ? AND user_id = ?", fixture.book.ID, fixture.owner.ID).
			Updates(map[string]any{
				"category_id":      fixture.nextCategory.ID,
				"custom_cover_url": "https://covers.test/concurrent.jpg",
				"can_update":       false,
				"library_path":     "concurrent/library/path",
				"original_file":    "concurrent-original.txt",
			}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND book_id = ?", fixture.owner.ID, fixture.book.ID).
			Delete(&models.BookCategory{}).Error; err != nil {
			return err
		}
		return tx.Create(&models.BookCategory{
			UserID: fixture.owner.ID, BookID: fixture.book.ID, CategoryID: fixture.nextCategory.ID,
		}).Error
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitReaderSourceChangeWriteLifecycleRequest(t, done)
	if response.Code != http.StatusOK {
		t.Fatalf("source change status=%d body=%s, want 200", response.Code, response.Body.String())
	}

	current := loadReaderSourceChangeWriteLifecycleBook(t, fixture)
	assertReaderSourceChangeWriteLifecycleConcurrentColumns(t, current, fixture)
	responseItem := decodeReaderSourceChangeWriteLifecycleItem(t, response.Body.Bytes())
	assertReaderSourceChangeWriteLifecycleConcurrentColumns(t, responseItem.Book, fixture)
	events := drainBookGroupWriteEvents(fixture.events)
	if len(events) != 1 {
		t.Fatalf("source change emitted %d events, want 1: %v", len(events), events)
	}
	var event struct {
		Type    string       `json:"type"`
		Payload bookListItem `json:"payload"`
	}
	if err := json.Unmarshal([]byte(events[0]), &event); err != nil {
		t.Fatalf("decode source change event: %v: %s", err, events[0])
	}
	if event.Type != "bookshelf_update" {
		t.Errorf("source change event type=%q, want bookshelf_update", event.Type)
	}
	assertReaderSourceChangeWriteLifecycleConcurrentColumns(t, event.Payload.Book, fixture)
}

func TestReaderSourceChangeCancellationReachesUpstreamAndHasNoSideEffects(t *testing.T) {
	fixture := newReaderSourceChangeWriteLifecycleFixture(t, "sourcechangecancel")
	before := snapshotReaderSourceChangeWriteLifecycleState(t, fixture)
	started, upstreamCanceled, release := installBlockingRemoteBookLifecycleClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	response, done := startReaderSourceChangeWriteLifecycleRequest(fixture, ctx)
	waitRemoteBookLifecycleSignal(t, started, release, "source change did not reach the remote transport")
	cancel()

	canceled := false
	select {
	case <-upstreamCanceled:
		canceled = true
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	waitReaderSourceChangeWriteLifecycleRequest(t, done)
	if !canceled {
		t.Error("caller cancellation did not reach target-source fetch")
	}
	if response.Body.Len() != 0 {
		t.Errorf("cancelled source change wrote a business response: %d %s", response.Code, response.Body.String())
	}
	after := snapshotReaderSourceChangeWriteLifecycleState(t, fixture)
	if !bytes.Equal(after, before) {
		t.Errorf("cancelled source change changed durable state\nbefore=%s\nafter=%s", before, after)
	}
	assertReaderSourceChangeWriteLifecycleNoEvents(t, fixture.events)
}

func TestReaderSourceChangeRollsBackCandidateFailure(t *testing.T) {
	fixture := newReaderSourceChangeWriteLifecycleFixture(t, "sourcechangecandidate")
	before := snapshotReaderSourceChangeWriteLifecycleState(t, fixture)
	callbackName := "test:reader-source-change-candidate-failure"
	if err := fixture.server.db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "BookSourceCandidate" {
			tx.AddError(errors.New("injected source candidate failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.server.db.Callback().Create().Remove(callbackName) })

	started, _, release := installBlockingRemoteBookLifecycleClient(t)
	response, done := startReaderSourceChangeWriteLifecycleRequest(fixture, context.Background())
	waitRemoteBookLifecycleSignal(t, started, release, "source change did not reach the remote transport")
	close(release)
	waitReaderSourceChangeWriteLifecycleRequest(t, done)
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"failed to change source"}` {
		t.Errorf("candidate failure=%d %s, want stable 500", response.Code, response.Body.String())
	}
	after := snapshotReaderSourceChangeWriteLifecycleState(t, fixture)
	if !bytes.Equal(after, before) {
		t.Errorf("candidate failure changed durable state\nbefore=%s\nafter=%s", before, after)
	}
	assertReaderSourceChangeWriteLifecycleNoEvents(t, fixture.events)
}

type readerSourceChangeWriteLifecycleFixture struct {
	router            http.Handler
	server            *Server
	auth              string
	owner             models.User
	oldSource         models.BookSource
	targetSource      models.BookSource
	replacementSource models.BookSource
	book              models.Book
	chapter           models.Chapter
	initialCategory   models.Category
	nextCategory      models.Category
	cacheFile         string
	cacheContent      string
	events            <-chan []byte
}

func newReaderSourceChangeWriteLifecycleFixture(t *testing.T, username string) readerSourceChangeWriteLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	oldSource := createRemoteBookLifecycleSource(t, server, "https://old."+username+".test")
	targetSource := createRemoteBookLifecycleSource(t, server, "https://target."+username+".test")
	replacementSource := createRemoteBookLifecycleSource(t, server, "https://replacement."+username+".test")
	initialCategory := models.Category{UserID: owner.ID, Name: "initial source-change category", Show: true}
	nextCategory := models.Category{UserID: owner.ID, Name: "next source-change category", Show: true}
	if err := server.db.Create(&initialCategory).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&nextCategory).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: owner.ID, SourceID: oldSource.ID, Type: oldSource.SourceType,
		CategoryID: &initialCategory.ID, Title: "source change initial title", Author: "initial author",
		CoverURL: "https://covers.test/initial.jpg", CustomCoverURL: "https://covers.test/custom-initial.jpg",
		Intro: "initial intro", Kind: "initial kind", WordCount: "1000 words",
		URL: oldSource.BaseURL + "/book", Variable: `{"old":"book"}`,
		LibraryPath: "legacy/remote/library", OriginalFile: "legacy-original.txt", TOCFile: "legacy-toc.json",
		TOCRule: "legacy toc rule", SourceFile: "legacy-source.json", LastChapter: "initial chapter",
		ChapterCount: 1, LastCheckTime: 1700000000000, CanUpdate: true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("source-change-write-lifecycle", username+"-old.txt")
	cacheContent := "source change old cache"
	cacheFile := writeLifecycleCache(t, server.cfg.CacheDir, cachePath, cacheContent)
	chapter := models.Chapter{
		BookID: book.ID, Index: 0, Title: "initial chapter", URL: oldSource.BaseURL + "/initial-chapter",
		CachePath: cachePath, Variable: `{"old":"chapter"}`,
	}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReadingProgress{
		UserID: owner.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 71, Percent: 0.42,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Bookmark{
		UserID: owner.ID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: 0, Offset: 29, Title: "bookmark",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookCategory{
		UserID: owner.ID, BookID: book.ID, CategoryID: initialCategory.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.sourceCandidates.SeedCurrent(server.db, book, &oldSource); err != nil {
		t.Fatal(err)
	}
	events := server.hub.AddClient(owner.ID, nil).Send
	return readerSourceChangeWriteLifecycleFixture{
		router: router, server: server, auth: auth, owner: owner,
		oldSource: oldSource, targetSource: targetSource, replacementSource: replacementSource,
		book: book, chapter: chapter, initialCategory: initialCategory, nextCategory: nextCategory,
		cacheFile: cacheFile, cacheContent: cacheContent, events: events,
	}
}

func startReaderSourceChangeWriteLifecycleRequest(
	fixture readerSourceChangeWriteLifecycleFixture,
	ctx context.Context,
) (*httptest.ResponseRecorder, <-chan struct{}) {
	body := `{"sourceId":` + strconv.FormatUint(uint64(fixture.targetSource.ID), 10) +
		`,"bookUrl":` + strconv.Quote(fixture.targetSource.BaseURL+"/book") + `}`
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/change-source",
		strings.NewReader(body),
	).WithContext(ctx)
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		fixture.router.ServeHTTP(response, request)
		close(done)
	}()
	return response, done
}

func waitReaderSourceChangeWriteLifecycleRequest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("source change handler did not finish")
	}
}

func assertReaderSourceChangeWriteLifecycleStale(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusConflict || response.Body.String() != `{"error":"book changed during source switch"}` {
		t.Errorf("stale source change=%d %s, want stable 409", response.Code, response.Body.String())
	}
}

func loadReaderSourceChangeWriteLifecycleBook(
	t *testing.T,
	fixture readerSourceChangeWriteLifecycleFixture,
) models.Book {
	t.Helper()
	var book models.Book
	if err := fixture.server.db.First(&book, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	return book
}

func countReaderSourceChangeTargetCandidates(t *testing.T, fixture readerSourceChangeWriteLifecycleFixture) int64 {
	t.Helper()
	var count int64
	if err := fixture.server.db.Model(&models.BookSourceCandidate{}).
		Where("user_id = ? AND book_id = ? AND source_id = ?", fixture.owner.ID, fixture.book.ID, fixture.targetSource.ID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func assertReaderSourceChangeWriteLifecycleConcurrentColumns(
	t *testing.T,
	book models.Book,
	fixture readerSourceChangeWriteLifecycleFixture,
) {
	t.Helper()
	if book.SourceID != fixture.targetSource.ID || book.URL != fixture.targetSource.BaseURL+"/book" {
		t.Errorf("source change did not commit target identity: %+v", book)
	}
	if book.CategoryID == nil || *book.CategoryID != fixture.nextCategory.ID ||
		book.CustomCoverURL != "https://covers.test/concurrent.jpg" || book.CanUpdate ||
		book.LibraryPath != "concurrent/library/path" || book.OriginalFile != "concurrent-original.txt" {
		t.Errorf("source change overwrote concurrent unowned columns: %+v", book)
	}
}

func decodeReaderSourceChangeWriteLifecycleItem(t *testing.T, payload []byte) bookListItem {
	t.Helper()
	var item bookListItem
	if err := json.Unmarshal(payload, &item); err != nil {
		t.Fatalf("decode source change shelf item: %v: %s", err, payload)
	}
	return item
}

func assertReaderSourceChangeWriteLifecycleNoEvents(t *testing.T, events <-chan []byte) {
	t.Helper()
	if emitted := drainBookGroupWriteEvents(events); len(emitted) != 0 {
		t.Errorf("source change emitted unexpected events: %v", emitted)
	}
}

func snapshotReaderSourceChangeWriteLifecycleState(
	t *testing.T,
	fixture readerSourceChangeWriteLifecycleFixture,
) []byte {
	t.Helper()
	var snapshot struct {
		Books      []models.Book
		Chapters   []models.Chapter
		Progress   []models.ReadingProgress
		Bookmarks  []models.Bookmark
		Categories []models.BookCategory
		Candidates []models.BookSourceCandidate
		Cache      string
	}
	queries := []struct {
		model any
		where string
		order string
	}{
		{model: &snapshot.Books, where: "id = ?", order: "id asc"},
		{model: &snapshot.Chapters, where: "book_id = ?", order: "id asc"},
		{model: &snapshot.Progress, where: "book_id = ?", order: "id asc"},
		{model: &snapshot.Bookmarks, where: "book_id = ?", order: "id asc"},
		{model: &snapshot.Categories, where: "book_id = ?", order: "id asc"},
		{model: &snapshot.Candidates, where: "book_id = ?", order: "id asc"},
	}
	for _, query := range queries {
		if err := fixture.server.db.Where(query.where, fixture.book.ID).Order(query.order).Find(query.model).Error; err != nil {
			t.Fatal(err)
		}
	}
	cache, err := os.ReadFile(fixture.cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Cache = string(cache)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
