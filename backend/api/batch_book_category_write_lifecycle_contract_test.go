package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestBatchBookCategoryWritePreservesConcurrentOwnedColumns(t *testing.T) {
	fixture := newBatchBookCategoryWriteLifecycleFixture(t, "batchcategorycolumns")
	installBatchBookCategoryWriteLifecycleHook(t, func(stage string, tx *gorm.DB, bookID uint) {
		if stage != "before_book_write" || bookID != fixture.book.ID {
			return
		}
		if err := tx.Model(&models.Book{}).Where("id = ? AND user_id = ?", bookID, fixture.owner.ID).Updates(map[string]any{
			"title":      "concurrent title",
			"intro":      "concurrent intro",
			"can_update": true,
		}).Error; err != nil {
			t.Errorf("write concurrent Book columns: %v", err)
		}
	})

	response := performBatchBookCategoryWriteLifecycleRequest(
		fixture,
		context.Background(),
		fmt.Sprintf(`{"action":"category-add","bookIds":[%d],"categoryId":%d}`, fixture.book.ID, fixture.nextCategory.ID),
	)
	assertBatchBookCategoryWriteLifecycleStatus(t, response, http.StatusOK)

	book, categoryIDs := loadBatchBookCategoryWriteLifecycleState(t, fixture)
	assertBatchBookCategoryWriteLifecycleBook(t, book, categoryIDs, "concurrent title", "concurrent intro", true, fixture.initialCategory.ID, fixture.nextCategory.ID)
	assertBatchBookCategoryWriteLifecycleResponse(t, response.Body.Bytes(), "concurrent title", "concurrent intro", true, fixture.initialCategory.ID, fixture.nextCategory.ID)
	assertBatchBookCategoryWriteLifecycleEvent(t, fixture.events, "concurrent title", "concurrent intro", true, fixture.initialCategory.ID, fixture.nextCategory.ID)
}

func TestBatchBookCategoryWriteDoesNotResurrectDeletedTarget(t *testing.T) {
	fixture := newBatchBookCategoryWriteLifecycleFixture(t, "batchcategorydelete")
	installBatchBookCategoryWriteLifecycleHook(t, func(stage string, tx *gorm.DB, bookID uint) {
		if stage != "before_book_write" || bookID != fixture.book.ID {
			return
		}
		if err := tx.Where("user_id = ? AND book_id = ?", fixture.owner.ID, bookID).Delete(&models.BookCategory{}).Error; err != nil {
			t.Errorf("delete concurrent BookCategory rows: %v", err)
		}
		if err := tx.Where("id = ? AND user_id = ?", bookID, fixture.owner.ID).Delete(&models.Book{}).Error; err != nil {
			t.Errorf("delete concurrent Book: %v", err)
		}
	})

	response := performBatchBookCategoryWriteLifecycleRequest(
		fixture,
		context.Background(),
		fmt.Sprintf(`{"action":"category","bookIds":[%d],"categoryIds":[%d]}`, fixture.book.ID, fixture.nextCategory.ID),
	)
	assertBatchBookCategoryWriteLifecycleStatus(t, response, http.StatusOK)

	var bookCount, relationCount int64
	if err := fixture.server.db.Model(&models.Book{}).Where("id = ?", fixture.book.ID).Count(&bookCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Model(&models.BookCategory{}).Where("book_id = ?", fixture.book.ID).Count(&relationCount).Error; err != nil {
		t.Fatal(err)
	}
	if bookCount != 0 || relationCount != 0 {
		t.Errorf("batch category write resurrected deleted state: books=%d relations=%d", bookCount, relationCount)
	}
	assertBatchBookCategoryWriteLifecycleEmptyResponse(t, response.Body.Bytes())
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("batch category write broadcast deleted target: %v", events)
	}
}

func TestBatchBookCategoryWriteHonorsCancellationBeforeCommit(t *testing.T) {
	fixture := newBatchBookCategoryWriteLifecycleFixture(t, "batchcategorycancel")
	before := snapshotBatchBookCategoryWriteLifecycleState(t, fixture)
	ctx, cancel := context.WithCancel(context.Background())
	installBatchBookCategoryWriteLifecycleHook(t, func(stage string, _ *gorm.DB, bookID uint) {
		if stage == "before_book_write" && bookID == fixture.book.ID {
			cancel()
		}
	})

	response := performBatchBookCategoryWriteLifecycleRequest(
		fixture,
		ctx,
		fmt.Sprintf(`{"action":"category","bookIds":[%d],"categoryIds":[%d]}`, fixture.book.ID, fixture.nextCategory.ID),
	)
	if response.Body.Len() != 0 {
		t.Errorf("cancelled batch category write returned a business response: %d %s", response.Code, response.Body.String())
	}
	after := snapshotBatchBookCategoryWriteLifecycleState(t, fixture)
	if !bytes.Equal(after, before) {
		t.Errorf("cancelled batch category write changed durable state\nbefore=%s\nafter=%s", before, after)
	}
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("cancelled batch category write broadcast events: %v", events)
	}
}

func TestBatchBookCategoryWritePropagatesRelationReadFailure(t *testing.T) {
	fixture := newBatchBookCategoryWriteLifecycleFixture(t, "batchcategoryreaderror")
	before := snapshotBatchBookCategoryWriteLifecycleState(t, fixture)
	callbackName := "test:batch-category-relation-read-error"
	var failOnce sync.Once
	if err := fixture.server.db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "book_categories" {
			failOnce.Do(func() { tx.AddError(errors.New("injected relation read failure")) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.server.db.Callback().Query().Remove(callbackName) })

	response := performBatchBookCategoryWriteLifecycleRequest(
		fixture,
		context.Background(),
		fmt.Sprintf(`{"action":"category-add","bookIds":[%d],"categoryId":%d}`, fixture.book.ID, fixture.nextCategory.ID),
	)
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"failed to update book categories"}` {
		t.Errorf("relation read failure = %d %s, want stable 500", response.Code, response.Body.String())
	}
	after := snapshotBatchBookCategoryWriteLifecycleState(t, fixture)
	if !bytes.Equal(after, before) {
		t.Errorf("relation read failure changed durable state\nbefore=%s\nafter=%s", before, after)
	}
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("relation read failure broadcast events: %v", events)
	}
}

func TestBatchBookCategoryWriteReloadsResponseAndEvent(t *testing.T) {
	fixture := newBatchBookCategoryWriteLifecycleFixture(t, "batchcategoryprojection")
	installBatchBookCategoryWriteLifecycleHook(t, func(stage string, tx *gorm.DB, bookID uint) {
		if stage != "after_book_write" || bookID != fixture.book.ID {
			return
		}
		if err := tx.Model(&models.Book{}).Where("id = ? AND user_id = ?", bookID, fixture.owner.ID).
			Updates(map[string]any{"title": "authoritative title", "intro": "authoritative intro"}).Error; err != nil {
			t.Errorf("write authoritative Book projection: %v", err)
		}
	})

	response := performBatchBookCategoryWriteLifecycleRequest(
		fixture,
		context.Background(),
		fmt.Sprintf(`{"action":"category-remove","bookIds":[%d],"categoryId":%d}`, fixture.book.ID, fixture.initialCategory.ID),
	)
	assertBatchBookCategoryWriteLifecycleStatus(t, response, http.StatusOK)
	assertBatchBookCategoryWriteLifecycleResponse(t, response.Body.Bytes(), "authoritative title", "authoritative intro", false)
	assertBatchBookCategoryWriteLifecycleEvent(t, fixture.events, "authoritative title", "authoritative intro", false)
}

type batchBookCategoryWriteLifecycleFixture struct {
	router          *gin.Engine
	server          *Server
	auth            string
	owner           models.User
	book            models.Book
	initialCategory models.Category
	nextCategory    models.Category
	events          <-chan []byte
}

func newBatchBookCategoryWriteLifecycleFixture(t *testing.T, username string) batchBookCategoryWriteLifecycleFixture {
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
	return batchBookCategoryWriteLifecycleFixture{
		router: router, server: server, auth: auth, owner: owner, book: book,
		initialCategory: initialCategory, nextCategory: nextCategory, events: events,
	}
}

func installBatchBookCategoryWriteLifecycleHook(
	t *testing.T,
	hook func(string, *gorm.DB, uint),
) {
	t.Helper()
	batchBookCategoryWriteLifecycleTestHook = hook
	t.Cleanup(func() { batchBookCategoryWriteLifecycleTestHook = nil })
}

func performBatchBookCategoryWriteLifecycleRequest(
	fixture batchBookCategoryWriteLifecycleFixture,
	ctx context.Context,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func assertBatchBookCategoryWriteLifecycleStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("batch category status = %d, want %d: %s", response.Code, want, response.Body.String())
	}
}

func loadBatchBookCategoryWriteLifecycleState(
	t *testing.T,
	fixture batchBookCategoryWriteLifecycleFixture,
) (models.Book, []uint) {
	t.Helper()
	var book models.Book
	if err := fixture.server.db.First(&book, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	var categoryIDs []uint
	if err := fixture.server.db.Model(&models.BookCategory{}).
		Where("user_id = ? AND book_id = ?", fixture.owner.ID, fixture.book.ID).
		Order("id asc").Pluck("category_id", &categoryIDs).Error; err != nil {
		t.Fatal(err)
	}
	return book, categoryIDs
}

func snapshotBatchBookCategoryWriteLifecycleState(t *testing.T, fixture batchBookCategoryWriteLifecycleFixture) []byte {
	t.Helper()
	book, categoryIDs := loadBatchBookCategoryWriteLifecycleState(t, fixture)
	payload, err := json.Marshal(struct {
		Book        models.Book `json:"book"`
		CategoryIDs []uint      `json:"categoryIds"`
	}{Book: book, CategoryIDs: categoryIDs})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertBatchBookCategoryWriteLifecycleBook(
	t *testing.T,
	book models.Book,
	categoryIDs []uint,
	title string,
	intro string,
	canUpdate bool,
	wantCategoryIDs ...uint,
) {
	t.Helper()
	if book.Title != title || book.Intro != intro || book.CanUpdate != canUpdate {
		t.Errorf("batch category write lost an owned Book column: %+v", book)
	}
	if len(wantCategoryIDs) == 0 {
		if book.CategoryID != nil || len(categoryIDs) != 0 {
			t.Errorf("batch category projection = primary %v relations %v, want empty", book.CategoryID, categoryIDs)
		}
		return
	}
	if book.CategoryID == nil || *book.CategoryID != wantCategoryIDs[0] {
		t.Errorf("batch category primary = %v, want %d", book.CategoryID, wantCategoryIDs[0])
	}
	if fmt.Sprint(categoryIDs) != fmt.Sprint(wantCategoryIDs) {
		t.Errorf("batch category relations = %v, want %v", categoryIDs, wantCategoryIDs)
	}
}

func assertBatchBookCategoryWriteLifecycleResponse(
	t *testing.T,
	payload []byte,
	title string,
	intro string,
	canUpdate bool,
	wantCategoryIDs ...uint,
) {
	t.Helper()
	var response struct {
		Affected int64          `json:"affected"`
		Books    []bookListItem `json:"books"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode batch category response: %v: %s", err, payload)
	}
	if response.Affected != 1 || len(response.Books) != 1 {
		t.Fatalf("batch category response = affected %d books %d, want 1/1", response.Affected, len(response.Books))
	}
	assertBatchBookCategoryWriteLifecycleBook(t, response.Books[0].Book, response.Books[0].CategoryIDs, title, intro, canUpdate, wantCategoryIDs...)
}

func assertBatchBookCategoryWriteLifecycleEmptyResponse(t *testing.T, payload []byte) {
	t.Helper()
	var response struct {
		Affected int64          `json:"affected"`
		Books    []bookListItem `json:"books"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode empty batch category response: %v: %s", err, payload)
	}
	if response.Affected != 0 || len(response.Books) != 0 {
		t.Errorf("deleted target response = affected %d books %d, want 0/0", response.Affected, len(response.Books))
	}
}

func assertBatchBookCategoryWriteLifecycleEvent(
	t *testing.T,
	events <-chan []byte,
	title string,
	intro string,
	canUpdate bool,
	wantCategoryIDs ...uint,
) {
	t.Helper()
	emitted := drainBookGroupWriteEvents(events)
	if len(emitted) != 1 {
		t.Fatalf("batch category write emitted %d events, want 1: %v", len(emitted), emitted)
	}
	var event struct {
		Type    string         `json:"type"`
		Payload []bookListItem `json:"payload"`
	}
	if err := json.Unmarshal([]byte(emitted[0]), &event); err != nil {
		t.Fatalf("decode batch category event: %v: %s", err, emitted[0])
	}
	if event.Type != "bookshelf_update" || len(event.Payload) != 1 {
		t.Fatalf("batch category event = type %q books %d, want bookshelf_update/1", event.Type, len(event.Payload))
	}
	assertBatchBookCategoryWriteLifecycleBook(t, event.Payload[0].Book, event.Payload[0].CategoryIDs, title, intro, canUpdate, wantCategoryIDs...)
}
