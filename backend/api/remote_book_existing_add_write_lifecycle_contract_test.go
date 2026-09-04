package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/models"
)

func TestRemoteBookExistingAddPreservesConcurrentOwnedColumns(t *testing.T) {
	fixture := newRemoteBookExistingAddWriteLifecycleFixture(t, "existingaddcolumns")
	installRemoteBookExistingAddWriteLifecycleHook(t, func(stage string, _ *gorm.DB, bookID uint) {
		if stage != "after_lookup" || bookID != fixture.book.ID {
			return
		}
		if err := fixture.server.db.Model(&models.Book{}).
			Where("id = ? AND user_id = ?", bookID, fixture.owner.ID).
			Updates(map[string]any{
				"source_id":  fixture.replacementSource.ID,
				"title":      "concurrent title",
				"intro":      "concurrent intro",
				"variable":   `{"current":"state"}`,
				"can_update": false,
			}).Error; err != nil {
			t.Errorf("write concurrent Book columns: %v", err)
		}
	})

	response := performRemoteBookExistingAddWriteLifecycleRequest(
		fixture,
		context.Background(),
		fmt.Sprintf(`{"title":"request title","bookUrl":%q,"sourceId":%d,"categoryIds":[%d]}`,
			fixture.book.URL, fixture.source.ID, fixture.nextCategory.ID),
	)
	assertRemoteBookExistingAddWriteLifecycleStatus(t, response, http.StatusOK)

	book, categoryIDs := loadRemoteBookExistingAddWriteLifecycleState(t, fixture)
	assertRemoteBookExistingAddWriteLifecycleBook(
		t, book, categoryIDs, fixture.replacementSource.ID, "concurrent title", "concurrent intro",
		`{"current":"state"}`, false, fixture.nextCategory.ID,
	)
	assertRemoteBookExistingAddWriteLifecycleProjection(
		t, response.Body.Bytes(), fixture.replacementSource.ID, "concurrent title", "concurrent intro",
		`{"current":"state"}`, false, fixture.nextCategory.ID,
	)
	assertRemoteBookExistingAddWriteLifecycleEvent(
		t, fixture.events, fixture.replacementSource.ID, "concurrent title", "concurrent intro",
		`{"current":"state"}`, false, fixture.nextCategory.ID,
	)
}

func TestRemoteBookExistingAddDoesNotResurrectDeletedTarget(t *testing.T) {
	fixture := newRemoteBookExistingAddWriteLifecycleFixture(t, "existingadddelete")
	installRemoteBookExistingAddWriteLifecycleHook(t, func(stage string, _ *gorm.DB, bookID uint) {
		if stage != "after_lookup" || bookID != fixture.book.ID {
			return
		}
		if err := fixture.server.db.Transaction(func(tx *gorm.DB) error {
			return deleteBookRecords(tx, fixture.owner.ID, bookID, &fixture.book)
		}); err != nil {
			t.Errorf("delete existing Book: %v", err)
		}
	})

	response := performRemoteBookExistingAddWriteLifecycleRequest(
		fixture,
		context.Background(),
		fmt.Sprintf(`{"title":"request title","bookUrl":%q,"sourceId":%d,"categoryIds":[%d]}`,
			fixture.book.URL, fixture.source.ID, fixture.nextCategory.ID),
	)
	if response.Code != http.StatusNotFound || response.Body.String() != `{"error":{"code":"NOT_FOUND","message":"book not found"}}` {
		t.Errorf("deleted existing add = %d %s, want owner-safe 404", response.Code, response.Body.String())
	}

	var bookCount, relationCount int64
	if err := fixture.server.db.Model(&models.Book{}).Where("id = ?", fixture.book.ID).Count(&bookCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.server.db.Model(&models.BookCategory{}).Where("book_id = ?", fixture.book.ID).Count(&relationCount).Error; err != nil {
		t.Fatal(err)
	}
	if bookCount != 0 || relationCount != 0 {
		t.Errorf("existing add resurrected deleted state: books=%d relations=%d", bookCount, relationCount)
	}
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("existing add broadcast deleted target: %v", events)
	}
}

func TestRemoteBookExistingAddHonorsCancellationAfterRelationWrite(t *testing.T) {
	fixture := newRemoteBookExistingAddWriteLifecycleFixture(t, "existingaddcancel")
	before := snapshotRemoteBookExistingAddWriteLifecycleState(t, fixture)
	ctx, cancel := context.WithCancel(context.Background())
	installRemoteBookExistingAddWriteLifecycleHook(t, func(stage string, _ *gorm.DB, bookID uint) {
		if stage == "after_relation_write" && bookID == fixture.book.ID {
			cancel()
		}
	})

	response := performRemoteBookExistingAddWriteLifecycleRequest(
		fixture,
		ctx,
		fmt.Sprintf(`{"title":"request title","bookUrl":%q,"sourceId":%d,"categoryIds":[%d]}`,
			fixture.book.URL, fixture.source.ID, fixture.nextCategory.ID),
	)
	if response.Body.Len() != 0 {
		t.Errorf("cancelled existing add returned a business response: %d %s", response.Code, response.Body.String())
	}
	after := snapshotRemoteBookExistingAddWriteLifecycleState(t, fixture)
	if !bytes.Equal(after, before) {
		t.Errorf("cancelled existing add changed durable state\nbefore=%s\nafter=%s", before, after)
	}
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("cancelled existing add broadcast events: %v", events)
	}
}

func TestRemoteBookExistingAddReloadsResponseAndEvent(t *testing.T) {
	fixture := newRemoteBookExistingAddWriteLifecycleFixture(t, "existingaddprojection")
	installRemoteBookExistingAddWriteLifecycleHook(t, func(stage string, tx *gorm.DB, bookID uint) {
		if stage != "after_book_write" || bookID != fixture.book.ID {
			return
		}
		if err := tx.Model(&models.Book{}).Where("id = ? AND user_id = ?", bookID, fixture.owner.ID).
			Updates(map[string]any{"title": "authoritative title", "intro": "authoritative intro"}).Error; err != nil {
			t.Errorf("write authoritative Book projection: %v", err)
		}
	})

	response := performRemoteBookExistingAddWriteLifecycleRequest(
		fixture,
		context.Background(),
		fmt.Sprintf(`{"title":"request title","bookUrl":%q,"sourceId":%d,"categoryIds":[%d]}`,
			fixture.book.URL, fixture.source.ID, fixture.nextCategory.ID),
	)
	assertRemoteBookExistingAddWriteLifecycleStatus(t, response, http.StatusOK)
	assertRemoteBookExistingAddWriteLifecycleProjection(
		t, response.Body.Bytes(), fixture.source.ID, "authoritative title", "authoritative intro",
		`{"initial":"state"}`, false, fixture.nextCategory.ID,
	)
	assertRemoteBookExistingAddWriteLifecycleEvent(
		t, fixture.events, fixture.source.ID, "authoritative title", "authoritative intro",
		`{"initial":"state"}`, false, fixture.nextCategory.ID,
	)
}

func TestRemoteBookExistingAddWithoutPositiveCategoriesIsNoOp(t *testing.T) {
	for _, bodyCategories := range []string{"", `,"categoryIds":[]`} {
		t.Run(fmt.Sprintf("categories-%d", len(bodyCategories)), func(t *testing.T) {
			fixture := newRemoteBookExistingAddWriteLifecycleFixture(t, "existingaddnoop"+fmt.Sprint(len(bodyCategories)))
			before := snapshotRemoteBookExistingAddWriteLifecycleState(t, fixture)
			response := performRemoteBookExistingAddWriteLifecycleRequest(
				fixture,
				context.Background(),
				fmt.Sprintf(`{"title":"request title","bookUrl":%q,"sourceId":%d%s}`,
					fixture.book.URL, fixture.source.ID, bodyCategories),
			)
			assertRemoteBookExistingAddWriteLifecycleStatus(t, response, http.StatusOK)
			after := snapshotRemoteBookExistingAddWriteLifecycleState(t, fixture)
			if !bytes.Equal(after, before) {
				t.Errorf("category-free existing add changed durable state\nbefore=%s\nafter=%s", before, after)
			}
			assertRemoteBookExistingAddWriteLifecycleProjection(
				t, response.Body.Bytes(), fixture.source.ID, fixture.book.Title, fixture.book.Intro,
				fixture.book.Variable, false, fixture.initialCategory.ID,
			)
			assertRemoteBookExistingAddWriteLifecycleEvent(
				t, fixture.events, fixture.source.ID, fixture.book.Title, fixture.book.Intro,
				fixture.book.Variable, false, fixture.initialCategory.ID,
			)
		})
	}
}

type remoteBookExistingAddWriteLifecycleFixture struct {
	router            *gin.Engine
	server            *Server
	auth              string
	owner             models.User
	source            models.BookSource
	replacementSource models.BookSource
	book              models.Book
	initialCategory   models.Category
	nextCategory      models.Category
	events            <-chan []byte
}

func newRemoteBookExistingAddWriteLifecycleFixture(
	t *testing.T,
	username string,
) remoteBookExistingAddWriteLifecycleFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	source := models.BookSource{Name: username + " source", BaseURL: "https://" + username + ".test", Enabled: true}
	replacementSource := models.BookSource{Name: username + " replacement", BaseURL: "https://replacement." + username + ".test", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&replacementSource).Error; err != nil {
		t.Fatal(err)
	}
	initialCategory := models.Category{UserID: owner.ID, Name: "initial category", Show: true}
	nextCategory := models.Category{UserID: owner.ID, Name: "next category", Show: true}
	if err := server.db.Create(&initialCategory).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&nextCategory).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: owner.ID, SourceID: source.ID, Title: "initial title", Intro: "initial intro",
		URL: source.BaseURL + "/book", Variable: `{"initial":"state"}`, CategoryID: &initialCategory.ID,
		CanUpdate: false,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&models.Book{}).Where("id = ?", book.ID).UpdateColumn("can_update", false).Error; err != nil {
		t.Fatal(err)
	}
	book.CanUpdate = false
	if err := server.db.Create(&models.BookCategory{
		UserID: owner.ID, BookID: book.ID, CategoryID: initialCategory.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	events := server.hub.AddClient(owner.ID, nil).Send
	return remoteBookExistingAddWriteLifecycleFixture{
		router: router, server: server, auth: auth, owner: owner, source: source,
		replacementSource: replacementSource, book: book, initialCategory: initialCategory,
		nextCategory: nextCategory, events: events,
	}
}

func installRemoteBookExistingAddWriteLifecycleHook(
	t *testing.T,
	hook func(string, *gorm.DB, uint),
) {
	t.Helper()
	remoteBookExistingAddWriteLifecycleTestHook = hook
	t.Cleanup(func() { remoteBookExistingAddWriteLifecycleTestHook = nil })
}

func performRemoteBookExistingAddWriteLifecycleRequest(
	fixture remoteBookExistingAddWriteLifecycleFixture,
	ctx context.Context,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/books/remote", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Authorization", fixture.auth)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func assertRemoteBookExistingAddWriteLifecycleStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("existing remote add status = %d, want %d: %s", response.Code, want, response.Body.String())
	}
}

func loadRemoteBookExistingAddWriteLifecycleState(
	t *testing.T,
	fixture remoteBookExistingAddWriteLifecycleFixture,
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

func snapshotRemoteBookExistingAddWriteLifecycleState(
	t *testing.T,
	fixture remoteBookExistingAddWriteLifecycleFixture,
) []byte {
	t.Helper()
	book, categoryIDs := loadRemoteBookExistingAddWriteLifecycleState(t, fixture)
	payload, err := json.Marshal(struct {
		Book        models.Book `json:"book"`
		CategoryIDs []uint      `json:"categoryIds"`
	}{Book: book, CategoryIDs: categoryIDs})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertRemoteBookExistingAddWriteLifecycleBook(
	t *testing.T,
	book models.Book,
	categoryIDs []uint,
	sourceID uint,
	title string,
	intro string,
	variable string,
	canUpdate bool,
	wantCategoryIDs ...uint,
) {
	t.Helper()
	if book.SourceID != sourceID || book.Title != title || book.Intro != intro || book.Variable != variable || book.CanUpdate != canUpdate {
		t.Errorf("existing remote add lost a current Book column: %+v", book)
	}
	if len(wantCategoryIDs) == 0 {
		if book.CategoryID != nil || len(categoryIDs) != 0 {
			t.Errorf("existing add category projection = primary %v relations %v, want empty", book.CategoryID, categoryIDs)
		}
		return
	}
	if book.CategoryID == nil || *book.CategoryID != wantCategoryIDs[0] || fmt.Sprint(categoryIDs) != fmt.Sprint(wantCategoryIDs) {
		t.Errorf("existing add category projection = primary %v relations %v, want %v", book.CategoryID, categoryIDs, wantCategoryIDs)
	}
}

func assertRemoteBookExistingAddWriteLifecycleProjection(
	t *testing.T,
	payload []byte,
	sourceID uint,
	title string,
	intro string,
	variable string,
	canUpdate bool,
	wantCategoryIDs ...uint,
) {
	t.Helper()
	var item bookListItem
	if err := json.Unmarshal(payload, &item); err != nil {
		t.Fatalf("decode existing add projection: %v: %s", err, payload)
	}
	assertRemoteBookExistingAddWriteLifecycleBook(
		t, item.Book, item.CategoryIDs, sourceID, title, intro, variable, canUpdate, wantCategoryIDs...,
	)
}

func assertRemoteBookExistingAddWriteLifecycleEvent(
	t *testing.T,
	events <-chan []byte,
	sourceID uint,
	title string,
	intro string,
	variable string,
	canUpdate bool,
	wantCategoryIDs ...uint,
) {
	t.Helper()
	emitted := drainBookGroupWriteEvents(events)
	if len(emitted) != 1 {
		t.Fatalf("existing remote add emitted %d events, want 1: %v", len(emitted), emitted)
	}
	var event struct {
		Type    string       `json:"type"`
		Payload bookListItem `json:"payload"`
	}
	if err := json.Unmarshal([]byte(emitted[0]), &event); err != nil {
		t.Fatalf("decode existing add event: %v: %s", err, emitted[0])
	}
	if event.Type != "bookshelf_update" {
		t.Errorf("existing remote add event type = %q, want bookshelf_update", event.Type)
	}
	assertRemoteBookExistingAddWriteLifecycleBook(
		t, event.Payload.Book, event.Payload.CategoryIDs, sourceID, title, intro, variable, canUpdate, wantCategoryIDs...,
	)
}
