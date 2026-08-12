package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

type bookGroupWriteBoundaryFixture struct {
	router         *gin.Engine
	server         *Server
	auth           string
	owner          models.User
	method         string
	path           string
	body           string
	successStatus  int
	malformedError string
	before         []byte
	events         <-chan []byte
}

func newBookGroupWriteBoundaryFixture(t *testing.T, route string) bookGroupWriteBoundaryFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	fixture := bookGroupWriteBoundaryFixture{
		router: router,
		server: server,
		auth:   auth,
		owner:  owner,
	}

	switch route {
	case "category-create":
		fixture.method = http.MethodPost
		fixture.path = "/api/categories"
		fixture.body = `{"name":"boundary-created","color":"#123456"}`
		fixture.successStatus = http.StatusCreated
		fixture.malformedError = "category name is required"
	case "category-update":
		category := createBookGroupWriteCategory(t, server, owner.ID, "boundary-original", 10)
		fixture.method = http.MethodPut
		fixture.path = "/api/categories/" + strconv.FormatUint(uint64(category.ID), 10)
		fixture.body = `{"name":"boundary-updated","color":"#654321","show":false}`
		fixture.successStatus = http.StatusOK
		fixture.malformedError = "invalid category payload"
	case "category-reorder":
		first := createBookGroupWriteCategory(t, server, owner.ID, "boundary-first", 10)
		second := createBookGroupWriteCategory(t, server, owner.ID, "boundary-second", 20)
		fixture.method = http.MethodPut
		fixture.path = "/api/categories/reorder"
		fixture.body = fmt.Sprintf(`{"ids":[%d,%d]}`, second.ID, first.ID)
		fixture.successStatus = http.StatusOK
		fixture.malformedError = "ids is required"
	case "built-in-update":
		if _, err := server.bookGroups.List(owner.ID); err != nil {
			t.Fatal(err)
		}
		fixture.method = http.MethodPut
		fixture.path = "/api/book-groups/all"
		fixture.body = `{"name":"boundary-all","show":false}`
		fixture.successStatus = http.StatusOK
		fixture.malformedError = "invalid book group payload"
	case "book-group-reorder":
		category := createBookGroupWriteCategory(t, server, owner.ID, "boundary-custom", 20)
		rows, err := server.bookGroups.List(owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		keys := make([]string, 0, len(rows))
		for index := len(rows) - 1; index >= 0; index-- {
			keys = append(keys, rows[index].Key)
		}
		if len(keys) == 0 || keys[0] != "category:"+strconv.FormatUint(uint64(category.ID), 10) {
			t.Fatalf("unexpected book-group fixture keys: %v", keys)
		}
		encoded, err := json.Marshal(gin.H{"keys": keys})
		if err != nil {
			t.Fatal(err)
		}
		fixture.method = http.MethodPut
		fixture.path = "/api/book-groups/reorder"
		fixture.body = string(encoded)
		fixture.successStatus = http.StatusOK
		fixture.malformedError = "keys is required"
	case "book-category":
		first := createBookGroupWriteCategory(t, server, owner.ID, "boundary-book-first", 10)
		second := createBookGroupWriteCategory(t, server, owner.ID, "boundary-book-second", 20)
		book := models.Book{UserID: owner.ID, Title: "boundary-book", CategoryID: &first.ID}
		if err := server.db.Create(&book).Error; err != nil {
			t.Fatal(err)
		}
		if err := server.db.Create(&models.BookCategory{UserID: owner.ID, BookID: book.ID, CategoryID: first.ID}).Error; err != nil {
			t.Fatal(err)
		}
		fixture.method = http.MethodPut
		fixture.path = "/api/books/" + strconv.FormatUint(uint64(book.ID), 10) + "/category"
		fixture.body = fmt.Sprintf(`{"categoryIds":[%d]}`, second.ID)
		fixture.successStatus = http.StatusOK
		fixture.malformedError = "invalid category payload"
	default:
		t.Fatalf("unknown BookGroup write boundary route %q", route)
	}

	fixture.before = snapshotBookGroupWriteState(t, server, owner.ID)
	fixture.events = server.hub.AddClient(owner.ID, nil).Send
	return fixture
}

func TestBookGroupWriteBoundaryRejectsDeclaredAndChunkedOversizedBodies(t *testing.T) {
	for _, route := range bookGroupWriteBoundaryRoutes() {
		for _, chunked := range []bool{false, true} {
			transport := "declared"
			if chunked {
				transport = "chunked"
			}
			t.Run(route+"/"+transport, func(t *testing.T) {
				fixture := newBookGroupWriteBoundaryFixture(t, route)
				body := padBookGroupWriteBody(fixture.body, int(maxBookGroupWriteRequestBodyBytes)+1)
				response := performBookGroupWriteRequest(fixture.router, fixture.auth, fixture.method, fixture.path, body, chunked)
				assertBookGroupWriteFlatError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
				assertBookGroupWriteFailureStable(t, fixture)
			})
		}
	}
}

func TestBookGroupWriteBoundaryRejectsTrailingJSONAndGarbage(t *testing.T) {
	for _, route := range bookGroupWriteBoundaryRoutes() {
		for _, suffix := range []struct {
			name string
			body string
		}{
			{name: "second-json", body: `{"ignored":true}`},
			{name: "garbage", body: `garbage`},
		} {
			t.Run(route+"/"+suffix.name, func(t *testing.T) {
				fixture := newBookGroupWriteBoundaryFixture(t, route)
				response := performBookGroupWriteRequest(fixture.router, fixture.auth, fixture.method, fixture.path, fixture.body+suffix.body, false)
				assertBookGroupWriteFlatError(t, response, http.StatusBadRequest, fixture.malformedError)
				assertBookGroupWriteFailureStable(t, fixture)
			})
		}
	}

	for _, route := range bookGroupWriteBoundaryRoutes() {
		t.Run(route+"/null-document", func(t *testing.T) {
			fixture := newBookGroupWriteBoundaryFixture(t, route)
			response := performBookGroupWriteRequest(fixture.router, fixture.auth, fixture.method, fixture.path, "null", false)
			assertBookGroupWriteFlatError(t, response, http.StatusBadRequest, fixture.malformedError)
			assertBookGroupWriteFailureStable(t, fixture)
		})
	}
}

func TestBookGroupWriteBoundaryAcceptsExactLimitAndTrailingWhitespace(t *testing.T) {
	for _, route := range bookGroupWriteBoundaryRoutes() {
		t.Run(route, func(t *testing.T) {
			fixture := newBookGroupWriteBoundaryFixture(t, route)
			const whitespace = "\r\n\t"
			body := padBookGroupWriteBody(fixture.body, int(maxBookGroupWriteRequestBodyBytes)-len(whitespace)) + whitespace
			if len(body) != int(maxBookGroupWriteRequestBodyBytes) {
				t.Fatalf("exact-limit fixture bytes=%d", len(body))
			}
			response := performBookGroupWriteRequest(fixture.router, fixture.auth, fixture.method, fixture.path, body, false)
			if response.Code != fixture.successStatus {
				t.Fatalf("exact-limit %s = %d, want %d: %s", route, response.Code, fixture.successStatus, response.Body.String())
			}
		})
	}
}

func TestBookGroupWriteBoundaryValidatesBeforePersistenceOrBodyRead(t *testing.T) {
	t.Run("blank-create-does-not-seed-built-ins", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := authHeader(t, router)
		response := performBookGroupWriteRequest(router, auth, http.MethodPost, "/api/categories", `{"name":"   "}`, false)
		assertBookGroupWriteFlatError(t, response, http.StatusBadRequest, "category name is required")
		var categories, preferences int64
		if err := server.db.Model(&models.Category{}).Count(&categories).Error; err != nil {
			t.Fatal(err)
		}
		if err := server.db.Model(&models.BookGroupPreference{}).Count(&preferences).Error; err != nil {
			t.Fatal(err)
		}
		if categories != 0 || preferences != 0 {
			t.Fatalf("blank create left categories=%d built-in preferences=%d", categories, preferences)
		}
	})

	t.Run("authentication-precedes-body", func(t *testing.T) {
		router, _ := setupTestServer(t)
		body := &bookGroupWriteNoReadBody{}
		response := performBookGroupWriteReaderRequest(router, "", http.MethodPost, "/api/categories", body)
		assertBookGroupWriteFlatError(t, response, http.StatusUnauthorized, "missing bearer token")
		if body.read {
			t.Fatal("unauthenticated request body was read")
		}
	})

	t.Run("unknown-built-in-key-precedes-body", func(t *testing.T) {
		router, _ := setupTestServer(t)
		auth := authHeader(t, router)
		body := &bookGroupWriteNoReadBody{}
		response := performBookGroupWriteReaderRequest(router, auth, http.MethodPut, "/api/book-groups/not-real", body)
		assertBookGroupWriteFlatError(t, response, http.StatusBadRequest, "invalid built-in book group")
		if body.read {
			t.Fatal("unknown built-in key request body was read")
		}
	})

	for _, route := range []struct {
		name string
		path string
	}{
		{name: "missing-category", path: "/api/categories/999999"},
		{name: "missing-book", path: "/api/books/999999/category"},
	} {
		t.Run(route.name+"-precedes-body", func(t *testing.T) {
			router, _ := setupTestServer(t)
			auth := authHeader(t, router)
			body := &bookGroupWriteNoReadBody{}
			response := performBookGroupWriteReaderRequest(router, auth, http.MethodPut, route.path, body)
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "not found") {
				t.Fatalf("missing target priority = %d %s", response.Code, response.Body.String())
			}
			if body.read {
				t.Fatal("missing target request body was read")
			}
		})
	}
}

func TestBookGroupWriteBoundaryEnforcesUTF8FieldBudgetsAndPreservesHistoricalRows(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		body   string
		status int
		error  string
	}{
		{name: "name-80-ascii", body: fmt.Sprintf(`{"name":%q}`, strings.Repeat("a", 80)), status: http.StatusCreated},
		{name: "name-80-utf8", body: fmt.Sprintf(`{"name":%q}`, strings.Repeat("组", 26)+"ab"), status: http.StatusCreated},
		{name: "name-81-ascii", body: fmt.Sprintf(`{"name":%q}`, strings.Repeat("b", 81)), status: http.StatusBadRequest, error: "category name is too long"},
		{name: "name-81-utf8", body: fmt.Sprintf(`{"name":%q}`, strings.Repeat("界", 27)), status: http.StatusBadRequest, error: "category name is too long"},
		{name: "color-24", body: fmt.Sprintf(`{"name":"color-24","color":%q}`, strings.Repeat("c", 24)), status: http.StatusCreated},
		{name: "color-25", body: fmt.Sprintf(`{"name":"color-25","color":%q}`, strings.Repeat("d", 25)), status: http.StatusBadRequest, error: "category color is too long"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performBookGroupWriteRequest(router, auth, http.MethodPost, "/api/categories", test.body, false)
			if test.error != "" {
				assertBookGroupWriteFlatError(t, response, test.status, test.error)
				return
			}
			if response.Code != test.status {
				t.Fatalf("%s = %d, want %d: %s", test.name, response.Code, test.status, response.Body.String())
			}
		})
	}

	name80 := strings.Repeat("n", 80)
	response := performBookGroupWriteRequest(router, auth, http.MethodPut, "/api/book-groups/all", fmt.Sprintf(`{"name":%q}`, name80), false)
	if response.Code != http.StatusOK {
		t.Fatalf("built-in name at limit = %d: %s", response.Code, response.Body.String())
	}
	response = performBookGroupWriteRequest(router, auth, http.MethodPut, "/api/book-groups/all", fmt.Sprintf(`{"name":%q}`, strings.Repeat("x", 81)), false)
	assertBookGroupWriteFlatError(t, response, http.StatusBadRequest, "book group name is too long")

	historical := models.Category{
		UserID: owner.ID, Name: strings.Repeat("h", 96), Color: strings.Repeat("e", 32), Show: true, SortOrder: 90,
	}
	if err := server.db.Create(&historical).Error; err != nil {
		t.Fatal(err)
	}
	response = performBookGroupWriteRequest(
		router,
		auth,
		http.MethodPut,
		"/api/categories/"+strconv.FormatUint(uint64(historical.ID), 10),
		`{"show":false}`,
		false,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("show-only historical update = %d: %s", response.Code, response.Body.String())
	}
	var reloaded models.Category
	if err := server.db.First(&reloaded, historical.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.Name != historical.Name || reloaded.Color != historical.Color || reloaded.Show {
		t.Fatalf("show-only update changed historical fields: %+v", reloaded)
	}
	before := reloaded
	response = performBookGroupWriteRequest(router, auth, http.MethodPut, "/api/categories/"+strconv.FormatUint(uint64(historical.ID), 10), fmt.Sprintf(`{"name":%q}`, strings.Repeat("z", 81)), false)
	assertBookGroupWriteFlatError(t, response, http.StatusBadRequest, "category name is too long")
	assertBookGroupWriteCategoryUnchanged(t, server, before)
}

func TestBookCategoryWriteBoundaryValidatesFinalEffectiveOwnerSet(t *testing.T) {
	for _, test := range []struct {
		name       string
		body       func(ownID, foreignID uint) string
		wantStatus int
		wantOwn    bool
	}{
		{name: "empty-array-falls-back-to-owned-id", body: func(ownID, _ uint) string { return fmt.Sprintf(`{"categoryId":%d,"categoryIds":[]}`, ownID) }, wantStatus: http.StatusOK, wantOwn: true},
		{name: "zero-array-falls-back-to-owned-id", body: func(ownID, _ uint) string { return fmt.Sprintf(`{"categoryId":%d,"categoryIds":[0,0]}`, ownID) }, wantStatus: http.StatusOK, wantOwn: true},
		{name: "non-empty-owned-array-wins", body: func(ownID, foreignID uint) string {
			return fmt.Sprintf(`{"categoryId":%d,"categoryIds":[%d]}`, foreignID, ownID)
		}, wantStatus: http.StatusOK, wantOwn: true},
		{name: "empty-array-cannot-fallback-to-foreign-id", body: func(_, foreignID uint) string { return fmt.Sprintf(`{"categoryId":%d,"categoryIds":[]}`, foreignID) }, wantStatus: http.StatusBadRequest},
		{name: "zero-array-cannot-fallback-to-foreign-id", body: func(_, foreignID uint) string { return fmt.Sprintf(`{"categoryId":%d,"categoryIds":[0]}`, foreignID) }, wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := authHeader(t, router)
			var owner models.User
			if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
				t.Fatal(err)
			}
			_, other := registerBookGroupContractUser(t, router, server, "boundaryother")
			own := createBookGroupWriteCategory(t, server, owner.ID, "owned-category", 10)
			foreign := createBookGroupWriteCategory(t, server, other.ID, "foreign-category", 10)
			book := models.Book{UserID: owner.ID, Title: "owner-set-book"}
			if err := server.db.Create(&book).Error; err != nil {
				t.Fatal(err)
			}
			events := server.hub.AddClient(owner.ID, nil).Send
			response := performBookGroupWriteRequest(
				router,
				auth,
				http.MethodPut,
				"/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/category",
				test.body(own.ID, foreign.ID),
				false,
			)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			var reloaded models.Book
			if err := server.db.First(&reloaded, book.ID).Error; err != nil {
				t.Fatal(err)
			}
			var links []models.BookCategory
			if err := server.db.Where("user_id = ? AND book_id = ?", owner.ID, book.ID).Find(&links).Error; err != nil {
				t.Fatal(err)
			}
			if test.wantOwn {
				if reloaded.CategoryID == nil || *reloaded.CategoryID != own.ID || len(links) != 1 || links[0].CategoryID != own.ID {
					t.Fatalf("valid effective set not persisted as owned category: book=%+v links=%+v", reloaded, links)
				}
				return
			}
			if reloaded.CategoryID != nil || len(links) != 0 {
				t.Fatalf("foreign effective set persisted: book=%+v links=%+v", reloaded, links)
			}
			if !strings.Contains(response.Body.String(), "category not found") {
				t.Fatalf("foreign category error is not stable: %s", response.Body.String())
			}
			if events := drainBookGroupWriteEvents(events); len(events) != 0 {
				t.Fatalf("foreign effective set broadcast events: %v", events)
			}
		})
	}
}

func TestBookGroupWriteBoundaryPreservesHistoricalOversizedRowsAcrossBackupRestore(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	historical := models.Category{
		UserID: owner.ID, Name: strings.Repeat("历史", 50), Color: strings.Repeat("c", 30), Show: true, SortOrder: 10,
	}
	if err := server.db.Create(&historical).Error; err != nil {
		t.Fatal(err)
	}
	path, err := server.backupSvc.RunNowForUser(owner.ID, owner.Username)
	if err != nil {
		t.Fatal(err)
	}
	entries := readFixedBaselineBackupEntries(t, path)
	var exported []models.Category
	if err := json.Unmarshal(entries["categories.json"], &exported); err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].Name != historical.Name || exported[0].Color != historical.Color || !exported[0].Show {
		t.Fatalf("historical oversized category was truncated in backup: %+v", exported)
	}
	archive, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	destinationRouter, destination := setupTestServer(t)
	_ = authHeader(t, destinationRouter)
	var destinationUser models.User
	if err := destination.db.Where("username = ?", "testuser").First(&destinationUser).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := destination.restoreLegadoBackupData(archive, destinationUser.ID); err != nil {
		t.Fatal(err)
	}
	restored := findCategoryByName(t, destination, destinationUser.ID, historical.Name)
	if restored.Color != historical.Color || !restored.Show {
		t.Fatalf("historical oversized category was changed by restore: %+v", restored)
	}
}

func bookGroupWriteBoundaryRoutes() []string {
	return []string{
		"category-create",
		"category-update",
		"category-reorder",
		"built-in-update",
		"book-group-reorder",
		"book-category",
	}
}

func createBookGroupWriteCategory(t *testing.T, server *Server, userID uint, name string, sortOrder int) models.Category {
	t.Helper()
	category := models.Category{UserID: userID, Name: name, Color: "#216869", Show: true, SortOrder: sortOrder}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	return category
}

func snapshotBookGroupWriteState(t *testing.T, server *Server, userID uint) []byte {
	t.Helper()
	var snapshot struct {
		Categories  []models.Category
		Preferences []models.BookGroupPreference
		Links       []models.BookCategory
		Books       []models.Book
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Categories).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Preferences).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Links).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Books).Error; err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertBookGroupWriteFailureStable(t *testing.T, fixture bookGroupWriteBoundaryFixture) {
	t.Helper()
	after := snapshotBookGroupWriteState(t, fixture.server, fixture.owner.ID)
	if !bytes.Equal(after, fixture.before) {
		t.Errorf("rejected request changed BookGroup persistence\nbefore=%s\nafter=%s", fixture.before, after)
	}
	if events := drainBookGroupWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("rejected request broadcast events: %v", events)
	}
}

func assertBookGroupWriteCategoryUnchanged(t *testing.T, server *Server, before models.Category) {
	t.Helper()
	var after models.Category
	if err := server.db.First(&after, before.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("rejected field update changed category\nbefore=%+v\nafter=%+v", before, after)
	}
}

func padBookGroupWriteBody(body string, total int) string {
	prefix := strings.TrimSuffix(body, "}") + `,"padding":"`
	suffix := `"}`
	if total < len(prefix)+len(suffix) {
		panic("BookGroup write contract target body is too small")
	}
	return prefix + strings.Repeat("p", total-len(prefix)-len(suffix)) + suffix
}

func performBookGroupWriteRequest(router http.Handler, auth, method, path, body string, chunked bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	if chunked {
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func performBookGroupWriteReaderRequest(router http.Handler, auth, method, path string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertBookGroupWriteFlatError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	want := fmt.Sprintf(`{"error":%q}`, message)
	if response.Code != status || response.Body.String() != want {
		t.Errorf("BookGroup write error = %d %s, want %d %s", response.Code, response.Body.String(), status, want)
	}
}

func drainBookGroupWriteEvents(channel <-chan []byte) []string {
	var events []string
	for {
		select {
		case payload := <-channel:
			events = append(events, string(payload))
		default:
			return events
		}
	}
}

type bookGroupWriteNoReadBody struct {
	read bool
}

func (body *bookGroupWriteNoReadBody) Read([]byte) (int, error) {
	body.read = true
	return 0, errors.New("request body must not be read")
}
