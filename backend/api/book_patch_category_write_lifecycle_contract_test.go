package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

func TestBookMetadataPatchMergesConcurrentOwnedColumnWrites(t *testing.T) {
	fixture := newBookPatchWriteLifecycleFixture(t, "bookpatchmerge")
	blocker := installBookPatchWriteLifecycleBlocker(t, "metadata")

	response, done := startBookPatchWriteLifecycleRequest(
		fixture,
		"metadata",
		`{"title":"metadata title","author":"metadata author"}`,
		context.Background(),
	)
	blocker.wait(t, "metadata patch did not reach the pre-transaction barrier")

	concurrentMetadata := performBookPatchWriteLifecycleRequest(
		fixture,
		"metadata",
		`{"intro":"concurrent intro","canUpdate":true}`,
	)
	assertBookPatchWriteLifecycleStatus(t, concurrentMetadata, http.StatusOK)
	concurrentCategory := performBookPatchWriteLifecycleRequest(
		fixture,
		"category",
		fmt.Sprintf(`{"categoryIds":[%d]}`, fixture.nextCategory.ID),
	)
	assertBookPatchWriteLifecycleStatus(t, concurrentCategory, http.StatusOK)
	if events := drainBookWriteEvents(fixture.events); len(events) != 2 {
		t.Fatalf("concurrent writes emitted %d events, want 2: %v", len(events), events)
	}

	blocker.unblock()
	waitBookPatchWriteLifecycleHandler(t, done)
	assertBookPatchWriteLifecycleStatus(t, response, http.StatusOK)

	book, categoryIDs := loadBookPatchWriteLifecycleState(t, fixture)
	assertBookPatchWriteLifecycleBook(t, book, categoryIDs, "metadata title", "metadata author", "concurrent intro", true, fixture.nextCategory.ID)
	assertBookPatchWriteLifecycleProjection(t, response.Body.Bytes(), "metadata title", "metadata author", "concurrent intro", true, fixture.nextCategory.ID)
	assertBookPatchWriteLifecycleEvent(t, fixture.events, "metadata title", "metadata author", "concurrent intro", true, fixture.nextCategory.ID)
}

func TestBookCategoryPatchMergesConcurrentMetadataWrites(t *testing.T) {
	fixture := newBookPatchWriteLifecycleFixture(t, "bookcategorymerge")
	blocker := installBookPatchWriteLifecycleBlocker(t, "category")

	response, done := startBookPatchWriteLifecycleRequest(
		fixture,
		"category",
		fmt.Sprintf(`{"categoryIds":[%d]}`, fixture.nextCategory.ID),
		context.Background(),
	)
	blocker.wait(t, "category patch did not reach the pre-transaction barrier")

	concurrentMetadata := performBookPatchWriteLifecycleRequest(
		fixture,
		"metadata",
		`{"title":"concurrent title","author":"concurrent author","intro":"concurrent intro","canUpdate":true}`,
	)
	assertBookPatchWriteLifecycleStatus(t, concurrentMetadata, http.StatusOK)
	if events := drainBookWriteEvents(fixture.events); len(events) != 1 {
		t.Fatalf("concurrent metadata emitted %d events, want 1: %v", len(events), events)
	}

	blocker.unblock()
	waitBookPatchWriteLifecycleHandler(t, done)
	assertBookPatchWriteLifecycleStatus(t, response, http.StatusOK)

	book, categoryIDs := loadBookPatchWriteLifecycleState(t, fixture)
	assertBookPatchWriteLifecycleBook(t, book, categoryIDs, "concurrent title", "concurrent author", "concurrent intro", true, fixture.nextCategory.ID)
	assertBookPatchWriteLifecycleProjection(t, response.Body.Bytes(), "concurrent title", "concurrent author", "concurrent intro", true, fixture.nextCategory.ID)
	assertBookPatchWriteLifecycleEvent(t, fixture.events, "concurrent title", "concurrent author", "concurrent intro", true, fixture.nextCategory.ID)
}

func TestBookPatchWritesDoNotResurrectConcurrentDeletes(t *testing.T) {
	for _, action := range []string{"metadata", "category"} {
		t.Run(action, func(t *testing.T) {
			fixture := newBookPatchWriteLifecycleFixture(t, "bookpatchdelete"+action)
			blocker := installBookPatchWriteLifecycleBlocker(t, action)
			body := `{"title":"late title"}`
			if action == "category" {
				body = fmt.Sprintf(`{"categoryIds":[%d]}`, fixture.nextCategory.ID)
			}

			response, done := startBookPatchWriteLifecycleRequest(fixture, action, body, context.Background())
			blocker.wait(t, action+" patch did not reach the pre-transaction barrier")

			deleteRequest := httptest.NewRequest(http.MethodDelete, fixture.bookPath(), nil)
			deleteRequest.Header.Set("Authorization", fixture.auth)
			deleteResponse := httptest.NewRecorder()
			fixture.router.ServeHTTP(deleteResponse, deleteRequest)
			assertBookPatchWriteLifecycleStatus(t, deleteResponse, http.StatusNoContent)
			drainBookWriteEvents(fixture.events)

			blocker.unblock()
			waitBookPatchWriteLifecycleHandler(t, done)
			if response.Code != http.StatusNotFound || response.Body.String() != `{"error":{"code":"NOT_FOUND","message":"book not found"}}` {
				t.Errorf("late %s patch = %d %s, want owner-safe 404", action, response.Code, response.Body.String())
			}

			var bookCount, relationCount int64
			if err := fixture.server.db.Model(&models.Book{}).Where("id = ?", fixture.book.ID).Count(&bookCount).Error; err != nil {
				t.Fatal(err)
			}
			if err := fixture.server.db.Model(&models.BookCategory{}).Where("book_id = ?", fixture.book.ID).Count(&relationCount).Error; err != nil {
				t.Fatal(err)
			}
			if bookCount != 0 || relationCount != 0 {
				t.Errorf("late %s patch resurrected deleted state: books=%d relations=%d", action, bookCount, relationCount)
			}
			if events := drainBookWriteEvents(fixture.events); len(events) != 0 {
				t.Errorf("late %s patch broadcast after delete: %v", action, events)
			}
		})
	}
}

func TestBookPatchWritesHonorCancellationBeforeTransaction(t *testing.T) {
	for _, action := range []string{"metadata", "category"} {
		t.Run(action, func(t *testing.T) {
			fixture := newBookPatchWriteLifecycleFixture(t, "bookpatchcancel"+action)
			before := snapshotBookPatchWriteLifecycleState(t, fixture)
			blocker := installBookPatchWriteLifecycleBlocker(t, action)
			body := `{"title":"cancelled title"}`
			if action == "category" {
				body = fmt.Sprintf(`{"categoryIds":[%d]}`, fixture.nextCategory.ID)
			}

			ctx, cancel := context.WithCancel(context.Background())
			response, done := startBookPatchWriteLifecycleRequest(fixture, action, body, ctx)
			blocker.wait(t, action+" patch did not reach the pre-transaction barrier")
			cancel()
			blocker.unblock()
			waitBookPatchWriteLifecycleHandler(t, done)

			if response.Body.Len() != 0 {
				t.Errorf("cancelled %s patch wrote a business response: %d %s", action, response.Code, response.Body.String())
			}
			after := snapshotBookPatchWriteLifecycleState(t, fixture)
			if !bytes.Equal(after, before) {
				t.Errorf("cancelled %s patch changed durable state\nbefore=%s\nafter=%s", action, before, after)
			}
			if events := drainBookWriteEvents(fixture.events); len(events) != 0 {
				t.Errorf("cancelled %s patch broadcast events: %v", action, events)
			}
		})
	}
}

func TestEmptyBookMetadataPatchDoesNotAdvanceUpdatedAt(t *testing.T) {
	fixture := newBookPatchWriteLifecycleFixture(t, "bookpatchempty")
	fixed := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := fixture.server.db.Model(&models.Book{}).Where("id = ?", fixture.book.ID).UpdateColumn("updated_at", fixed).Error; err != nil {
		t.Fatal(err)
	}
	var before models.Book
	if err := fixture.server.db.First(&before, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}

	response := performBookPatchWriteLifecycleRequest(fixture, "metadata", `{}`)
	assertBookPatchWriteLifecycleStatus(t, response, http.StatusOK)
	var after models.Book
	if err := fixture.server.db.First(&after, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("empty metadata patch advanced updated_at: before=%s after=%s", before.UpdatedAt, after.UpdatedAt)
	}
}

type bookPatchWriteLifecycleFixture struct {
	router          *gin.Engine
	server          *Server
	auth            string
	owner           models.User
	book            models.Book
	initialCategory models.Category
	nextCategory    models.Category
	events          <-chan []byte
}

func newBookPatchWriteLifecycleFixture(t *testing.T, username string) bookPatchWriteLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	initialCategory := models.Category{UserID: owner.ID, Name: "initial category", Show: true, SortOrder: 10}
	nextCategory := models.Category{UserID: owner.ID, Name: "next category", Show: true, SortOrder: 20}
	if err := server.db.Create(&initialCategory).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&nextCategory).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: owner.ID, Title: "initial title", Author: "initial author", Intro: "initial intro",
		CategoryID: &initialCategory.ID, CanUpdate: false,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&models.Book{}).Where("id = ?", book.ID).UpdateColumn("can_update", false).Error; err != nil {
		t.Fatal(err)
	}
	book.CanUpdate = false
	if err := server.db.Create(&models.BookCategory{UserID: owner.ID, BookID: book.ID, CategoryID: initialCategory.ID}).Error; err != nil {
		t.Fatal(err)
	}
	events := server.hub.AddClient(owner.ID, nil).Send
	return bookPatchWriteLifecycleFixture{
		router: router, server: server, auth: auth, owner: owner, book: book,
		initialCategory: initialCategory, nextCategory: nextCategory, events: events,
	}
}

func (fixture bookPatchWriteLifecycleFixture) bookPath() string {
	return "/api/books/" + strconv.FormatUint(uint64(fixture.book.ID), 10)
}

type bookPatchWriteLifecycleBlocker struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func installBookPatchWriteLifecycleBlocker(t *testing.T, action string) *bookPatchWriteLifecycleBlocker {
	t.Helper()
	blocker := &bookPatchWriteLifecycleBlocker{started: make(chan struct{}), release: make(chan struct{})}
	bookPatchWriteLifecycleTestHook = func(currentAction string) {
		if currentAction != action {
			return
		}
		bookPatchWriteLifecycleTestHook = nil
		blocker.startedOnce.Do(func() { close(blocker.started) })
		<-blocker.release
	}
	t.Cleanup(func() {
		blocker.unblock()
		bookPatchWriteLifecycleTestHook = nil
	})
	return blocker
}

func (blocker *bookPatchWriteLifecycleBlocker) wait(t *testing.T, message string) {
	t.Helper()
	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		blocker.unblock()
		t.Fatal(message)
	}
}

func (blocker *bookPatchWriteLifecycleBlocker) unblock() {
	blocker.releaseOnce.Do(func() { close(blocker.release) })
}

func startBookPatchWriteLifecycleRequest(
	fixture bookPatchWriteLifecycleFixture,
	action string,
	body string,
	ctx context.Context,
) (*httptest.ResponseRecorder, <-chan struct{}) {
	request := httptest.NewRequest(http.MethodPut, bookPatchWriteLifecyclePath(fixture, action), strings.NewReader(body)).WithContext(ctx)
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

func performBookPatchWriteLifecycleRequest(
	fixture bookPatchWriteLifecycleFixture,
	action string,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPut, bookPatchWriteLifecyclePath(fixture, action), strings.NewReader(body))
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func bookPatchWriteLifecyclePath(fixture bookPatchWriteLifecycleFixture, action string) string {
	if action == "category" {
		return fixture.bookPath() + "/category"
	}
	return fixture.bookPath()
}

func waitBookPatchWriteLifecycleHandler(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Book patch handler did not finish")
	}
}

func assertBookPatchWriteLifecycleStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("Book patch status = %d, want %d: %s", response.Code, want, response.Body.String())
	}
}

func loadBookPatchWriteLifecycleState(t *testing.T, fixture bookPatchWriteLifecycleFixture) (models.Book, []uint) {
	t.Helper()
	var book models.Book
	if err := fixture.server.db.First(&book, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	var categoryIDs []uint
	if err := fixture.server.db.Model(&models.BookCategory{}).
		Where("user_id = ? AND book_id = ?", fixture.owner.ID, fixture.book.ID).
		Order("category_id asc").Pluck("category_id", &categoryIDs).Error; err != nil {
		t.Fatal(err)
	}
	return book, categoryIDs
}

func snapshotBookPatchWriteLifecycleState(t *testing.T, fixture bookPatchWriteLifecycleFixture) []byte {
	t.Helper()
	book, categoryIDs := loadBookPatchWriteLifecycleState(t, fixture)
	payload, err := json.Marshal(struct {
		Book        models.Book `json:"book"`
		CategoryIDs []uint      `json:"categoryIds"`
	}{Book: book, CategoryIDs: categoryIDs})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertBookPatchWriteLifecycleBook(
	t *testing.T,
	book models.Book,
	categoryIDs []uint,
	title string,
	author string,
	intro string,
	canUpdate bool,
	categoryID uint,
) {
	t.Helper()
	if book.Title != title || book.Author != author || book.Intro != intro || book.CanUpdate != canUpdate {
		t.Errorf("stored Book lost a concurrent owned-column write: %+v", book)
	}
	if book.CategoryID == nil || *book.CategoryID != categoryID || len(categoryIDs) != 1 || categoryIDs[0] != categoryID {
		t.Errorf("stored category projection is inconsistent: primary=%v relations=%v want=%d", book.CategoryID, categoryIDs, categoryID)
	}
}

func assertBookPatchWriteLifecycleProjection(
	t *testing.T,
	payload []byte,
	title string,
	author string,
	intro string,
	canUpdate bool,
	categoryID uint,
) {
	t.Helper()
	var item bookListItem
	if err := json.Unmarshal(payload, &item); err != nil {
		t.Fatalf("decode shelf projection: %v: %s", err, payload)
	}
	assertBookPatchWriteLifecycleBook(t, item.Book, item.CategoryIDs, title, author, intro, canUpdate, categoryID)
}

func assertBookPatchWriteLifecycleEvent(
	t *testing.T,
	events <-chan []byte,
	title string,
	author string,
	intro string,
	canUpdate bool,
	categoryID uint,
) {
	t.Helper()
	emitted := drainBookWriteEvents(events)
	if len(emitted) != 1 {
		t.Fatalf("Book patch emitted %d events, want 1: %v", len(emitted), emitted)
	}
	var event struct {
		Type    string       `json:"type"`
		Payload bookListItem `json:"payload"`
	}
	if err := json.Unmarshal([]byte(emitted[0]), &event); err != nil {
		t.Fatalf("decode shelf event: %v: %s", err, emitted[0])
	}
	if event.Type != "bookshelf_update" {
		t.Errorf("Book patch event type = %q, want bookshelf_update", event.Type)
	}
	assertBookPatchWriteLifecycleBook(t, event.Payload.Book, event.Payload.CategoryIDs, title, author, intro, canUpdate, categoryID)
}
