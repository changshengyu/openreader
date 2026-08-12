package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

const (
	testBookmarkSingleBodyLimit      = 64 << 10
	testBookmarkBatchCreateBodyLimit = 16 << 20
	testBookmarkBatchDeleteBodyLimit = 16 << 10
	testBookmarkBatchItemLimit       = 2000
)

type bookmarkWriteBoundaryFixture struct {
	router *gin.Engine
	server *Server
	auth   string
	user   models.User
	book   models.Book
	method string
	path   string
	body   string
	limit  int
	before []byte
	events <-chan []byte
}

func newBookmarkWriteBoundaryFixture(t *testing.T, route string) bookmarkWriteBoundaryFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	user := bookmarkContractUser(t, server)
	book, chapter := bookmarkContractBook(t, server, user.ID, "bookmark-write-"+route)
	fixture := bookmarkWriteBoundaryFixture{
		router: router,
		server: server,
		auth:   auth,
		user:   user,
		book:   book,
		method: http.MethodPost,
		limit:  testBookmarkSingleBodyLimit,
	}

	switch route {
	case "create":
		fixture.path = "/api/books/" + uintString(book.ID) + "/bookmarks"
		fixture.body = fmt.Sprintf(`{"chapterId":%d,"excerpt":"bounded create","note":"before"}`, chapter.ID)
	case "batch-create":
		fixture.path = "/api/books/" + uintString(book.ID) + "/bookmarks/batch"
		fixture.body = fmt.Sprintf(`[{"chapterId":%d,"excerpt":"bounded batch create"}]`, chapter.ID)
		fixture.limit = testBookmarkBatchCreateBodyLimit
	case "update":
		bookmark := bookmarkBoundarySeed(t, server, user.ID, book, chapter, "bounded update", "before")
		fixture.method = http.MethodPut
		fixture.path = "/api/bookmarks/" + uintString(bookmark.ID)
		fixture.body = `{"note":"after"}`
	case "batch-delete":
		bookmark := bookmarkBoundarySeed(t, server, user.ID, book, chapter, "bounded delete", "before")
		fixture.path = "/api/books/" + uintString(book.ID) + "/bookmarks/batch-delete"
		fixture.body = fmt.Sprintf(`{"ids":[%d]}`, bookmark.ID)
		fixture.limit = testBookmarkBatchDeleteBodyLimit
	default:
		t.Fatalf("unknown bookmark write route %q", route)
	}

	fixture.before = snapshotBookmarkWriteRows(t, server, user.ID)
	client := server.hub.AddClient(user.ID, nil)
	t.Cleanup(func() { server.hub.RemoveClient(client) })
	fixture.events = client.Send
	return fixture
}

func TestBookmarkJSONWritesRejectDeclaredAndChunkedOversizedBodies(t *testing.T) {
	for _, route := range []string{"create", "batch-create", "update", "batch-delete"} {
		for _, chunked := range []bool{false, true} {
			transport := "declared"
			if chunked {
				transport = "chunked"
			}
			t.Run(route+"/"+transport, func(t *testing.T) {
				fixture := newBookmarkWriteBoundaryFixture(t, route)
				response := bookmarkBoundaryRequest(fixture, bookmarkBoundaryBodyAtSize(fixture, fixture.limit+1), chunked)
				assertBookmarkBoundaryError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
				assertBookmarkBoundaryFailureStable(t, fixture)
			})
		}
	}
}

func TestBookmarkJSONWritesAcceptExactLimitWithTrailingWhitespace(t *testing.T) {
	for _, route := range []string{"create", "batch-create", "update", "batch-delete"} {
		t.Run(route, func(t *testing.T) {
			fixture := newBookmarkWriteBoundaryFixture(t, route)
			body := fixture.body + strings.Repeat(" ", fixture.limit-len(fixture.body))
			response := bookmarkBoundaryRequest(fixture, body, false)
			want := http.StatusOK
			if route == "create" || route == "batch-create" {
				want = http.StatusCreated
			}
			if response.Code != want {
				t.Fatalf("exact-limit bookmark %s = %d %s, want %d", route, response.Code, boundaryDiagnostic(response.Body.String()), want)
			}
		})
	}
}

func TestBookmarkJSONWritesAcceptOnlyOneDocumentOfTheExpectedShape(t *testing.T) {
	for _, route := range []string{"create", "batch-create", "update", "batch-delete"} {
		for _, document := range []struct {
			name string
			body func(bookmarkWriteBoundaryFixture) string
		}{
			{name: "null", body: func(bookmarkWriteBoundaryFixture) string { return "null" }},
			{name: "wrong-container", body: func(fixture bookmarkWriteBoundaryFixture) string {
				if fixture.path == "/api/books/"+uintString(fixture.book.ID)+"/bookmarks/batch" {
					return `{}`
				}
				return `[]`
			}},
			{name: "scalar", body: func(bookmarkWriteBoundaryFixture) string { return `"bookmark"` }},
			{name: "second-json", body: func(fixture bookmarkWriteBoundaryFixture) string { return fixture.body + `{}` }},
			{name: "trailing-garbage", body: func(fixture bookmarkWriteBoundaryFixture) string { return fixture.body + `garbage` }},
		} {
			t.Run(route+"/"+document.name, func(t *testing.T) {
				fixture := newBookmarkWriteBoundaryFixture(t, route)
				response := bookmarkBoundaryRequest(fixture, document.body(fixture), false)
				if response.Code != http.StatusBadRequest {
					t.Errorf("bookmark %s %s = %d %s, want 400", route, document.name, response.Code, boundaryDiagnostic(response.Body.String()))
				}
				assertBookmarkBoundaryFailureStable(t, fixture)
			})
		}
	}
}

func TestBookmarkWritePrioritizesOwnedTargetBeforeBody(t *testing.T) {
	t.Run("authentication-precedes-body", func(t *testing.T) {
		router, _ := setupTestServer(t)
		body := &bookmarkNoReadBody{}
		response := bookmarkBoundaryReaderRequest(router, "", http.MethodPost, "/api/books/1/bookmarks", body)
		if response.Code != http.StatusUnauthorized || body.read {
			t.Fatalf("unauthenticated bookmark body priority = %d read=%t body=%s", response.Code, body.read, response.Body.String())
		}
	})

	router, _ := setupTestServer(t)
	auth := authHeader(t, router)
	for _, request := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "create", method: http.MethodPost, path: "/api/books/999999/bookmarks"},
		{name: "batch-create", method: http.MethodPost, path: "/api/books/999999/bookmarks/batch"},
		{name: "update", method: http.MethodPut, path: "/api/bookmarks/999999"},
		{name: "batch-delete", method: http.MethodPost, path: "/api/books/999999/bookmarks/batch-delete"},
	} {
		t.Run(request.name, func(t *testing.T) {
			body := &bookmarkNoReadBody{}
			response := bookmarkBoundaryReaderRequest(router, auth, request.method, request.path, body)
			if response.Code != http.StatusNotFound || body.read {
				t.Errorf("missing bookmark target priority = %d read=%t body=%s", response.Code, body.read, response.Body.String())
			}
		})
	}
}

func TestBookmarkBatchCreateEnforcesRawItemLimitBeforeMutation(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		fixture := newBookmarkWriteBoundaryFixture(t, "batch-create")
		response := bookmarkBoundaryRequest(fixture, bookmarkBoundaryBatchCreatePayload(testBookmarkBatchItemLimit), false)
		if response.Code != http.StatusCreated {
			t.Fatalf("exact bookmark batch limit = %d %s", response.Code, boundaryDiagnostic(response.Body.String()))
		}
		var count int64
		if err := fixture.server.db.Model(&models.Bookmark{}).Where("user_id = ? AND book_id = ?", fixture.user.ID, fixture.book.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != testBookmarkBatchItemLimit {
			t.Fatalf("exact bookmark batch persisted %d rows, want %d", count, testBookmarkBatchItemLimit)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		fixture := newBookmarkWriteBoundaryFixture(t, "batch-create")
		response := bookmarkBoundaryRequest(fixture, bookmarkBoundaryBatchCreatePayload(testBookmarkBatchItemLimit+1), false)
		assertBookmarkBoundaryError(t, response, http.StatusBadRequest, "invalid bookmarks payload")
		assertBookmarkBoundaryFailureStable(t, fixture)
	})
}

func TestBookmarkBatchDeleteEnforcesRawIDLimitBeforeMutation(t *testing.T) {
	for _, count := range []int{testBookmarkBatchItemLimit, testBookmarkBatchItemLimit + 1} {
		name := "exact"
		want := http.StatusOK
		if count > testBookmarkBatchItemLimit {
			name = "overflow"
			want = http.StatusBadRequest
		}
		t.Run(name, func(t *testing.T) {
			fixture := newBookmarkWriteBoundaryFixture(t, "batch-delete")
			var existing models.Bookmark
			if err := fixture.server.db.Where("user_id = ? AND book_id = ?", fixture.user.ID, fixture.book.ID).First(&existing).Error; err != nil {
				t.Fatal(err)
			}
			response := bookmarkBoundaryRequest(fixture, bookmarkBoundaryBatchDeletePayload(existing.ID, count), false)
			if response.Code != want {
				t.Errorf("bookmark batch-delete %s = %d %s, want %d", name, response.Code, boundaryDiagnostic(response.Body.String()), want)
			}
			if count > testBookmarkBatchItemLimit {
				assertBookmarkBoundaryFailureStable(t, fixture)
			}
		})
	}
}

func TestBookmarkUpdateRequiresExplicitStringNote(t *testing.T) {
	for _, body := range []string{`{}`, `{"ignored":true}`, `{"note":null}`, `{"note":7}`, `{"note":[]}`} {
		t.Run(body, func(t *testing.T) {
			fixture := newBookmarkWriteBoundaryFixture(t, "update")
			response := bookmarkBoundaryRequest(fixture, body, false)
			assertBookmarkBoundaryError(t, response, http.StatusBadRequest, "invalid bookmark payload")
			assertBookmarkBoundaryFailureStable(t, fixture)
		})
	}

	t.Run("explicit-empty-clears-note", func(t *testing.T) {
		fixture := newBookmarkWriteBoundaryFixture(t, "update")
		response := bookmarkBoundaryRequest(fixture, `{"note":""}`, false)
		if response.Code != http.StatusOK {
			t.Fatalf("explicit empty bookmark note = %d %s", response.Code, response.Body.String())
		}
		var stored models.Bookmark
		if err := fixture.server.db.Where("user_id = ? AND book_id = ?", fixture.user.ID, fixture.book.ID).First(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Note != "" {
			t.Fatalf("explicit empty note persisted %q", stored.Note)
		}
	})
}

func TestBookmarkNoteUpdateDoesNotOverwriteConcurrentReaderContext(t *testing.T) {
	fixture := newBookmarkWriteBoundaryFixture(t, "update")
	var target models.Bookmark
	if err := fixture.server.db.Where("user_id = ? AND book_id = ?", fixture.user.ID, fixture.book.ID).First(&target).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`
		CREATE TRIGGER bookmark_note_concurrent_context
		BEFORE UPDATE OF note ON bookmarks
		WHEN OLD.id = %d
		BEGIN
			UPDATE bookmarks SET excerpt = 'concurrent context', "offset" = 91 WHERE id = OLD.id;
		END;
	`, target.ID)
	if err := fixture.server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	response := bookmarkBoundaryRequest(fixture, `{"note":"after"}`, false)
	if response.Code != http.StatusOK {
		t.Fatalf("bookmark note update = %d %s", response.Code, response.Body.String())
	}
	var stored models.Bookmark
	if err := fixture.server.db.First(&stored, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Note != "after" || stored.Excerpt != "concurrent context" || stored.Offset != 91 {
		t.Fatalf("note update overwrote concurrent reader context: %+v", stored)
	}
	var projected models.Bookmark
	if err := json.Unmarshal(response.Body.Bytes(), &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Excerpt != stored.Excerpt || projected.Offset != stored.Offset {
		t.Fatalf("note update returned stale snapshot: %+v stored=%+v", projected, stored)
	}
}

func TestBookmarkNoteUpdateCannotReviveConcurrentlyDeletedTarget(t *testing.T) {
	fixture := newBookmarkWriteBoundaryFixture(t, "update")
	var target models.Bookmark
	if err := fixture.server.db.Where("user_id = ? AND book_id = ?", fixture.user.ID, fixture.book.ID).First(&target).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`
		CREATE TRIGGER bookmark_note_concurrent_delete
		BEFORE UPDATE OF note ON bookmarks
		WHEN OLD.id = %d
		BEGIN
			DELETE FROM bookmarks WHERE id = OLD.id;
		END;
	`, target.ID)
	if err := fixture.server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}

	response := bookmarkBoundaryRequest(fixture, `{"note":"after delete"}`, false)
	if response.Code != http.StatusNotFound {
		t.Errorf("concurrently deleted bookmark update = %d %s, want 404", response.Code, response.Body.String())
	}
	var count int64
	if err := fixture.server.db.Model(&models.Bookmark{}).Where("id = ?", target.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("note update revived concurrently deleted bookmark %d", target.ID)
	}
	if queuedPayloadContains(fixture.events, `"type":"bookmarks_update"`) {
		t.Error("concurrently deleted bookmark update broadcast success")
	}
}

func bookmarkBoundarySeed(t *testing.T, server *Server, userID uint, book models.Book, chapter models.Chapter, excerpt, note string) models.Bookmark {
	t.Helper()
	bookmark := models.Bookmark{
		UserID: userID, BookID: book.ID, ChapterID: chapter.ID, ChapterIndex: chapter.Index,
		Offset: 7, Percent: 0.25, Title: chapter.Title, Excerpt: excerpt, Note: note,
	}
	if err := server.db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}
	return bookmark
}

func bookmarkBoundaryRequest(fixture bookmarkWriteBoundaryFixture, body string, chunked bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(fixture.method, fixture.path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", fixture.auth)
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)
	return response
}

func bookmarkBoundaryReaderRequest(router http.Handler, auth, method, path string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func bookmarkBoundaryBodyAtSize(fixture bookmarkWriteBoundaryFixture, size int) string {
	if size <= 0 {
		panic("bookmark boundary target size must be positive")
	}
	var prefix, suffix string
	switch {
	case strings.HasSuffix(fixture.path, "/bookmarks/batch"):
		prefix = `[{"excerpt":"bounded","padding":"`
		suffix = `"}]`
	default:
		prefix = strings.TrimSuffix(fixture.body, "}") + `,"padding":"`
		suffix = `"}`
	}
	if size < len(prefix)+len(suffix) {
		panic("bookmark boundary target body is too small")
	}
	return prefix + strings.Repeat("p", size-len(prefix)-len(suffix)) + suffix
}

func bookmarkBoundaryBatchCreatePayload(count int) string {
	entry := `{"excerpt":"batch context"}`
	var builder strings.Builder
	builder.Grow(count*(len(entry)+1) + 2)
	builder.WriteByte('[')
	for index := 0; index < count; index++ {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(entry)
	}
	builder.WriteByte(']')
	return builder.String()
}

func bookmarkBoundaryBatchDeletePayload(id uint, count int) string {
	value := strconv.FormatUint(uint64(id), 10)
	var builder strings.Builder
	builder.Grow(count*(len(value)+1) + 10)
	builder.WriteString(`{"ids":[`)
	for index := 0; index < count; index++ {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(value)
	}
	builder.WriteString(`]}`)
	return builder.String()
}

func snapshotBookmarkWriteRows(t *testing.T, server *Server, userID uint) []byte {
	t.Helper()
	var rows []models.Bookmark
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertBookmarkBoundaryFailureStable(t *testing.T, fixture bookmarkWriteBoundaryFixture) {
	t.Helper()
	after := snapshotBookmarkWriteRows(t, fixture.server, fixture.user.ID)
	if string(after) != string(fixture.before) {
		t.Errorf(
			"rejected bookmark write changed rows\nbefore=%s\nafter=%s",
			boundaryDiagnostic(string(fixture.before)),
			boundaryDiagnostic(string(after)),
		)
	}
	if queuedPayloadContains(fixture.events, `"type":"bookmarks_update"`) {
		t.Error("rejected bookmark write broadcast a sync event")
	}
}

func assertBookmarkBoundaryError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	want := fmt.Sprintf(`{"error":%q}`, message)
	if response.Code != status || response.Body.String() != want {
		t.Errorf("bookmark write error = %d %s, want %d %s", response.Code, boundaryDiagnostic(response.Body.String()), status, want)
	}
}

func boundaryDiagnostic(value string) string {
	const limit = 512
	if len(value) <= limit {
		return value
	}
	return fmt.Sprintf("%s... (%d bytes total)", value[:limit], len(value))
}

type bookmarkNoReadBody struct {
	read bool
}

func (body *bookmarkNoReadBody) Read([]byte) (int, error) {
	body.read = true
	return 0, errors.New("bookmark request body must not be read")
}
