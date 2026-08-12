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
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

const testBookWriteRequestBodyLimit = 1 << 20

type bookWriteBoundaryFixture struct {
	router *gin.Engine
	server *Server
	auth   string
	owner  models.User
	method string
	path   string
	body   string
	before []byte
	events <-chan []byte
}

func newBookWriteBoundaryFixture(t *testing.T, route string) bookWriteBoundaryFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, "bookwriteowner")
	owner := lifecycleUser(t, server, "bookwriteowner")
	fixture := bookWriteBoundaryFixture{
		router: router,
		server: server,
		auth:   auth,
		owner:  owner,
	}

	switch route {
	case "create":
		fixture.method = http.MethodPost
		fixture.path = "/api/books"
		fixture.body = `{"title":"bounded create"}`
	case "update":
		book := models.Book{UserID: owner.ID, Title: "bounded update", Author: "before", CanUpdate: true}
		if err := server.db.Create(&book).Error; err != nil {
			t.Fatal(err)
		}
		fixture.method = http.MethodPut
		fixture.path = "/api/books/" + strconv.FormatUint(uint64(book.ID), 10)
		fixture.body = `{"author":"after"}`
	default:
		t.Fatalf("unknown book write route %q", route)
	}

	fixture.before = snapshotBookWriteState(t, server, owner.ID)
	fixture.events = server.hub.AddClient(owner.ID, nil).Send
	return fixture
}

func TestBookWriteBoundaryRejectsDeclaredAndChunkedOversizedBodies(t *testing.T) {
	for _, route := range []string{"create", "update"} {
		for _, chunked := range []bool{false, true} {
			transport := "declared"
			if chunked {
				transport = "chunked"
			}
			t.Run(route+"/"+transport, func(t *testing.T) {
				fixture := newBookWriteBoundaryFixture(t, route)
				body := padBookWriteBody(fixture.body, testBookWriteRequestBodyLimit+1)
				response := performBookWriteRequest(fixture.router, fixture.auth, fixture.method, fixture.path, body, chunked)
				assertBookWriteFlatError(t, response, http.StatusRequestEntityTooLarge, "request body too large")
				assertBookWriteFailureStable(t, fixture)
			})
		}
	}
}

func TestBookWriteBoundaryAcceptsExactLimitAndOnlyOneObject(t *testing.T) {
	for _, route := range []string{"create", "update"} {
		t.Run(route+"/exact-limit", func(t *testing.T) {
			fixture := newBookWriteBoundaryFixture(t, route)
			body := padBookWriteBody(fixture.body, testBookWriteRequestBodyLimit)
			response := performBookWriteRequest(fixture.router, fixture.auth, fixture.method, fixture.path, body, false)
			want := http.StatusOK
			if route == "create" {
				want = http.StatusCreated
			}
			if response.Code != want {
				t.Fatalf("exact-limit %s = %d, want %d: %s", route, response.Code, want, response.Body.String())
			}
		})

		for _, document := range []struct {
			name string
			body string
		}{
			{name: "null", body: "null"},
			{name: "array", body: `[]`},
			{name: "scalar", body: `"book"`},
			{name: "second-json", body: `{"title":"one"}{"title":"two"}`},
			{name: "trailing-garbage", body: `{"title":"one"}garbage`},
		} {
			t.Run(route+"/"+document.name, func(t *testing.T) {
				fixture := newBookWriteBoundaryFixture(t, route)
				response := performBookWriteRequest(fixture.router, fixture.auth, fixture.method, fixture.path, document.body, false)
				assertBookWriteFlatError(t, response, http.StatusBadRequest, "invalid book payload")
				assertBookWriteFailureStable(t, fixture)
			})
		}
	}
}

func TestBookWriteBoundaryPrioritizesAuthAndOwnedTargetBeforeBody(t *testing.T) {
	t.Run("authentication-precedes-create-body", func(t *testing.T) {
		router, _ := setupTestServer(t)
		body := &bookWriteNoReadBody{}
		response := performBookWriteReaderRequest(router, "", http.MethodPost, "/api/books", body)
		assertBookWriteFlatError(t, response, http.StatusUnauthorized, "missing bearer token")
		if body.read {
			t.Fatal("unauthenticated create body was read")
		}
	})

	for _, target := range []string{"missing", "foreign"} {
		t.Run(target+"-update-target-precedes-body", func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := registerLifecycleToken(t, router, "bookwritepriority")
			bookID := uint(999999)
			if target == "foreign" {
				registerLifecycleToken(t, router, "bookwriteforeign")
				other := lifecycleUser(t, server, "bookwriteforeign")
				book := models.Book{UserID: other.ID, Title: "foreign target"}
				if err := server.db.Create(&book).Error; err != nil {
					t.Fatal(err)
				}
				bookID = book.ID
			}
			body := &bookWriteNoReadBody{}
			response := performBookWriteReaderRequest(
				router,
				auth,
				http.MethodPut,
				"/api/books/"+strconv.FormatUint(uint64(bookID), 10),
				body,
			)
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "book not found") {
				t.Fatalf("missing or foreign target = %d: %s", response.Code, response.Body.String())
			}
			if body.read {
				t.Fatal("missing or foreign update body was read")
			}
		})
	}
}

func TestBookCreateIgnoresServerOwnedFields(t *testing.T) {
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, "bookwritemass")
	owner := lifecycleUser(t, server, "bookwritemass")
	category := createBookGroupWriteCategory(t, server, owner.ID, "owned-create-category", 10)
	coverURL := createBookWriteCover(t, server, owner, "owned.jpg")
	events := server.hub.AddClient(owner.ID, nil).Send

	payload := map[string]any{
		"id":             900123,
		"userId":         999,
		"sourceId":       777,
		"type":           1,
		"title":          "  allowed title  ",
		"author":         "  allowed author  ",
		"coverUrl":       "  https://example.com/cover.jpg  ",
		"customCoverUrl": "  " + coverURL + "  ",
		"intro":          "  allowed intro  ",
		"kind":           "  allowed kind  ",
		"wordCount":      "  12万字  ",
		"url":            "  https://example.com/book  ",
		"variable":       `{"secret":"client-owned"}`,
		"libraryPath":    "data/bookwritemass/victim",
		"originalFile":   "source.txt",
		"tocFile":        "chapters.json",
		"tocRule":        "forged rule",
		"sourceFile":     "source.json",
		"lastChapter":    "forged latest",
		"chapterCount":   99,
		"lastCheckTime":  123,
		"createdAt":      "2000-01-01T00:00:00Z",
		"updatedAt":      "2000-01-02T00:00:00Z",
		"categoryId":     category.ID,
		"categoryIds":    []uint{0},
		"canUpdate":      false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response := performBookWriteRequest(router, auth, http.MethodPost, "/api/books", string(body), false)
	if response.Code != http.StatusCreated {
		t.Fatalf("server-owned create = %d: %s", response.Code, response.Body.String())
	}

	var stored models.Book
	if err := server.db.Where("user_id = ? AND title = ?", owner.ID, "allowed title").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ID == 900123 || stored.UserID != owner.ID || stored.SourceID != 0 || stored.Type != 0 {
		t.Errorf("client controlled identity/source/type: %+v", stored)
	}
	if stored.LibraryPath != "" || stored.OriginalFile != "" || stored.TOCFile != "" || stored.TOCRule != "" || stored.SourceFile != "" {
		t.Errorf("client controlled local storage state: %+v", stored)
	}
	if stored.Variable != "" || stored.LastChapter != "" || stored.ChapterCount != 0 || stored.LastCheckTime == 123 {
		t.Errorf("client controlled parser/progress state: %+v", stored)
	}
	if stored.CreatedAt.Year() == 2000 || stored.UpdatedAt.Year() == 2000 {
		t.Errorf("client controlled timestamps: created=%s updated=%s", stored.CreatedAt, stored.UpdatedAt)
	}
	if stored.Author != "allowed author" || stored.CoverURL != "https://example.com/cover.jpg" ||
		stored.CustomCoverURL != coverURL || stored.Intro != "allowed intro" || stored.Kind != "allowed kind" ||
		stored.WordCount != "12万字" || stored.URL != "https://example.com/book" || stored.CanUpdate {
		t.Errorf("allowed create fields were not preserved: %+v", stored)
	}
	if stored.CategoryID == nil || *stored.CategoryID != category.ID {
		t.Fatalf("owned fallback category missing: %+v", stored)
	}
	var links []models.BookCategory
	if err := server.db.Where("user_id = ? AND book_id = ?", owner.ID, stored.ID).Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].CategoryID != category.ID {
		t.Fatalf("owned category links = %+v", links)
	}
	if emitted := drainBookWriteEvents(events); len(emitted) != 1 || !strings.Contains(emitted[0], "bookshelf_update") {
		t.Fatalf("successful create events = %v", emitted)
	}
}

func TestBookCreateValidatesFinalEffectiveCategoryOwner(t *testing.T) {
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, "bookwritecategory")
	owner := lifecycleUser(t, server, "bookwritecategory")
	registerLifecycleToken(t, router, "bookwritecategoryother")
	other := lifecycleUser(t, server, "bookwritecategoryother")
	owned := createBookGroupWriteCategory(t, server, owner.ID, "owned-final-category", 10)
	foreign := createBookGroupWriteCategory(t, server, other.ID, "foreign-final-category", 10)
	events := server.hub.AddClient(owner.ID, nil).Send

	foreignBody := fmt.Sprintf(`{"title":"foreign category fallback","categoryId":%d,"categoryIds":[0]}`, foreign.ID)
	response := performBookWriteRequest(router, auth, http.MethodPost, "/api/books", foreignBody, false)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "category not found") {
		t.Fatalf("foreign category fallback = %d: %s", response.Code, response.Body.String())
	}
	var count int64
	if err := server.db.Model(&models.Book{}).Where("user_id = ?", owner.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("foreign category fallback created %d books", count)
	}
	if emitted := drainBookWriteEvents(events); len(emitted) != 0 {
		t.Fatalf("foreign category fallback events = %v", emitted)
	}

	ownedBody := fmt.Sprintf(`{"title":"owned category fallback","categoryId":%d,"categoryIds":[0,0]}`, owned.ID)
	response = performBookWriteRequest(router, auth, http.MethodPost, "/api/books", ownedBody, false)
	if response.Code != http.StatusCreated {
		t.Fatalf("owned category fallback = %d: %s", response.Code, response.Body.String())
	}
	var stored models.Book
	if err := server.db.Where("user_id = ? AND title = ?", owner.ID, "owned category fallback").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CategoryID == nil || *stored.CategoryID != owned.ID {
		t.Fatalf("owned category fallback was not stored: %+v", stored)
	}
}

func TestBookCreateEnforcesCustomCoverOwnership(t *testing.T) {
	for _, scenario := range []string{"external", "foreign", "missing", "owned"} {
		t.Run(scenario, func(t *testing.T) {
			router, server := setupTestServer(t)
			auth := registerLifecycleToken(t, router, "bookwritecover")
			owner := lifecycleUser(t, server, "bookwritecover")
			coverURL := "https://example.com/cover.jpg"
			want := http.StatusBadRequest
			switch scenario {
			case "foreign":
				registerLifecycleToken(t, router, "bookwritecoverother")
				other := lifecycleUser(t, server, "bookwritecoverother")
				coverURL = createBookWriteCover(t, server, other, "foreign.jpg")
			case "missing":
				coverURL = "/uploads/users/" + strconv.FormatUint(uint64(owner.ID), 10) + "/covers/missing.jpg"
			case "owned":
				coverURL = createBookWriteCover(t, server, owner, "owned.jpg")
				want = http.StatusCreated
			}
			body := fmt.Sprintf(`{"title":"cover %s","customCoverUrl":%q}`, scenario, coverURL)
			response := performBookWriteRequest(router, auth, http.MethodPost, "/api/books", body, false)
			if response.Code != want {
				t.Fatalf("%s cover = %d, want %d: %s", scenario, response.Code, want, response.Body.String())
			}
			if want == http.StatusBadRequest && response.Body.String() != `{"error":"invalid custom cover url"}` {
				t.Fatalf("%s cover error = %s", scenario, response.Body.String())
			}
			var count int64
			if err := server.db.Model(&models.Book{}).Where("user_id = ?", owner.ID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			wantCount := int64(0)
			if want == http.StatusCreated {
				wantCount = 1
			}
			if count != wantCount {
				t.Fatalf("%s cover left %d books, want %d", scenario, count, wantCount)
			}
		})
	}
}

func TestBookWriteBoundaryEnforcesUTF8FieldBudgetsAndPreservesHistoricalRows(t *testing.T) {
	fields := []struct {
		name  string
		key   string
		limit int
		error string
	}{
		{name: "title", key: "title", limit: 240, error: "book title is too long"},
		{name: "author", key: "author", limit: 160, error: "book author is too long"},
		{name: "cover-url", key: "coverUrl", limit: 600, error: "book cover url is too long"},
		{name: "kind", key: "kind", limit: 400, error: "book kind is too long"},
		{name: "word-count", key: "wordCount", limit: 120, error: "book word count is too long"},
		{name: "url", key: "url", limit: 800, error: "book url is too long"},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			router, _ := setupTestServer(t)
			auth := registerLifecycleToken(t, router, "bookwritefield")
			exact := map[string]any{"title": "field exact " + field.name, field.key: strings.Repeat("a", field.limit)}
			if field.key == "title" {
				exact[field.key] = strings.Repeat("界", 79) + "abc"
			}
			body, err := json.Marshal(exact)
			if err != nil {
				t.Fatal(err)
			}
			response := performBookWriteRequest(router, auth, http.MethodPost, "/api/books", string(body), false)
			if response.Code != http.StatusCreated {
				t.Fatalf("%s exact limit = %d: %s", field.name, response.Code, response.Body.String())
			}

			over := map[string]any{"title": "field over " + field.name, field.key: strings.Repeat("b", field.limit+1)}
			body, err = json.Marshal(over)
			if err != nil {
				t.Fatal(err)
			}
			response = performBookWriteRequest(router, auth, http.MethodPost, "/api/books", string(body), false)
			assertBookWriteFlatError(t, response, http.StatusBadRequest, field.error)
		})
	}

	t.Run("custom-cover-over-limit-precedes-capability-lookup", func(t *testing.T) {
		router, _ := setupTestServer(t)
		auth := registerLifecycleToken(t, router, "bookwritecoverlimit")
		body := fmt.Sprintf(`{"title":"cover over","customCoverUrl":%q}`, strings.Repeat("c", 601))
		response := performBookWriteRequest(router, auth, http.MethodPost, "/api/books", body, false)
		assertBookWriteFlatError(t, response, http.StatusBadRequest, "book custom cover url is too long")
	})

	t.Run("historical-oversized-row-can-patch-unrelated-field", func(t *testing.T) {
		router, server := setupTestServer(t)
		auth := registerLifecycleToken(t, router, "bookwritehistorical")
		owner := lifecycleUser(t, server, "bookwritehistorical")
		historical := models.Book{
			UserID: owner.ID, Title: strings.Repeat("h", 260), Author: strings.Repeat("a", 180),
			CoverURL: strings.Repeat("c", 620), URL: strings.Repeat("u", 820), CanUpdate: true,
		}
		if err := server.db.Create(&historical).Error; err != nil {
			t.Fatal(err)
		}
		path := "/api/books/" + strconv.FormatUint(uint64(historical.ID), 10)
		response := performBookWriteRequest(router, auth, http.MethodPut, path, `{"canUpdate":false}`, false)
		if response.Code != http.StatusOK {
			t.Fatalf("historical unrelated patch = %d: %s", response.Code, response.Body.String())
		}
		var reloaded models.Book
		if err := server.db.First(&reloaded, historical.ID).Error; err != nil {
			t.Fatal(err)
		}
		if reloaded.Title != historical.Title || reloaded.Author != historical.Author || reloaded.CoverURL != historical.CoverURL || reloaded.URL != historical.URL || reloaded.CanUpdate {
			t.Fatalf("historical row changed unexpectedly: %+v", reloaded)
		}
		response = performBookWriteRequest(router, auth, http.MethodPut, path, fmt.Sprintf(`{"title":%q}`, strings.Repeat("x", 241)), false)
		assertBookWriteFlatError(t, response, http.StatusBadRequest, "book title is too long")

		backupPath, err := server.backupSvc.RunNowForUser(owner.ID, owner.Username)
		if err != nil {
			t.Fatal(err)
		}
		entries := readFixedBaselineBackupEntries(t, backupPath)
		if !bytes.Contains(entries["bookshelf.json"], []byte(historical.Title)) {
			t.Fatal("historical oversized book title was truncated in logical backup")
		}
		archive, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatal(err)
		}
		destinationRouter, destination := setupTestServer(t)
		_ = registerLifecycleToken(t, destinationRouter, "bookwritehistoricalrestore")
		destinationOwner := lifecycleUser(t, destination, "bookwritehistoricalrestore")
		if _, err := destination.restoreLegadoBackupData(archive, destinationOwner.ID); err != nil {
			t.Fatal(err)
		}
		var restored models.Book
		if err := destination.db.Where("user_id = ? AND url = ?", destinationOwner.ID, historical.URL).First(&restored).Error; err != nil {
			t.Fatal(err)
		}
		if restored.Title != historical.Title || restored.Author != historical.Author || restored.CoverURL != historical.CoverURL || restored.URL != historical.URL {
			t.Fatalf("historical oversized book changed across logical restore: %+v", restored)
		}
	})
}

func TestBookDeletionPreservesSharedLocalArchiveUntilLastReference(t *testing.T) {
	for _, action := range []string{"single", "batch"} {
		t.Run(action, func(t *testing.T) {
			router, server := setupTestServer(t)
			username := "bookwriteshared" + action
			auth := registerLifecycleToken(t, router, username)
			owner := lifecycleUser(t, server, username)
			libraryPath := "data/" + username + "/shared-archive"
			lexicalAlias := "data/" + username + "/nested/../shared-archive"
			archiveRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
			sourcePath := writeLifecycleCache(t, archiveRoot, "source.txt", "shared local source")
			first := models.Book{UserID: owner.ID, Title: "shared first", LibraryPath: libraryPath, OriginalFile: filepath.Join(libraryPath, "source.txt")}
			second := models.Book{UserID: owner.ID, Title: "shared second", LibraryPath: lexicalAlias, OriginalFile: filepath.Join(lexicalAlias, "source.txt")}
			if err := server.db.Create(&first).Error; err != nil {
				t.Fatal(err)
			}
			if err := server.db.Create(&second).Error; err != nil {
				t.Fatal(err)
			}

			deleteBookByContractAction(t, router, auth, action, first.ID)
			if _, err := os.Stat(sourcePath); err != nil {
				t.Fatalf("shared archive was removed while a live reference remained: %v", err)
			}
			var survivor models.Book
			if err := server.db.First(&survivor, second.ID).Error; err != nil {
				t.Fatalf("surviving book row missing: %v", err)
			}

			deleteBookByContractAction(t, router, auth, action, second.ID)
			if _, err := os.Stat(archiveRoot); !os.IsNotExist(err) {
				t.Fatalf("last local archive reference did not clean directory, stat err=%v", err)
			}
		})
	}
}

func TestBookDeletionLeavesUnsafeSymlinkArchiveUntouched(t *testing.T) {
	router, server := setupTestServer(t)
	username := "bookwritesymlink"
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	ownerRoot := filepath.Join(server.cfg.LibraryDir, "data", username)
	if err := os.MkdirAll(ownerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideFile := writeLifecycleCache(t, outside, "source.txt", "outside source")
	linkPath := filepath.Join(ownerRoot, "linked-archive")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	book := models.Book{
		UserID: owner.ID, Title: "unsafe linked archive", LibraryPath: filepath.Join("data", username, "linked-archive"),
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	deleteBookByContractAction(t, router, auth, "single", book.ID)
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("unsafe linked archive reached outside private root: %v", err)
	}
	if info, err := os.Lstat(linkPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("unsafe linked archive should be left untouched, info=%v err=%v", info, err)
	}
}

func TestBookDeletionPreservesArchiveReferencedThroughSafeSymlink(t *testing.T) {
	router, server := setupTestServer(t)
	username := "bookwritesafelink"
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	ownerRoot := filepath.Join(server.cfg.LibraryDir, "data", username)
	targetRoot := filepath.Join(ownerRoot, "real-archive")
	sourcePath := writeLifecycleCache(t, targetRoot, "source.txt", "safe linked source")
	resolvedTargetRoot, err := filepath.EvalSymlinks(targetRoot)
	if err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(ownerRoot, "linked-archive")
	if err := os.Symlink(targetRoot, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	direct := models.Book{
		UserID: owner.ID, Title: "direct archive", LibraryPath: filepath.Join("data", username, "real-archive"),
	}
	linked := models.Book{
		UserID: owner.ID, Title: "linked archive", LibraryPath: filepath.Join("data", username, "linked-archive"),
	}
	if err := server.db.Create(&direct).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&linked).Error; err != nil {
		t.Fatal(err)
	}
	if resolved, ok := server.localBookArchiveRoot(linked); !ok || filepath.Clean(resolved) != filepath.Clean(resolvedTargetRoot) {
		t.Fatalf("safe linked archive was not readable before deletion: path=%q ok=%v", resolved, ok)
	}

	deleteBookByContractAction(t, router, auth, "single", direct.ID)
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("safe linked reference did not preserve its target: %v", err)
	}
	if resolved, ok := server.localBookArchiveRoot(linked); !ok || filepath.Clean(resolved) != filepath.Clean(resolvedTargetRoot) {
		t.Fatalf("safe linked archive became unreadable after deleting direct alias: path=%q ok=%v", resolved, ok)
	}
}

func TestBookCleanupFailsClosedWhenReferencesCannotBeQueried(t *testing.T) {
	router, server := setupTestServer(t)
	username := "bookwritequeryfailure"
	_ = registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	libraryPath := filepath.Join("data", username, "query-failure")
	archiveRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
	sourcePath := writeLifecycleCache(t, archiveRoot, "source.txt", "query failure source")
	resolved, ok := server.resolvedPrivateImportedBookDirectory(username, libraryPath)
	if !ok {
		t.Fatal("failed to resolve query-failure fixture")
	}
	sqlDB, err := server.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	server.cleanupDeletedBookArtifacts([]bookCleanupPlan{{
		privateLibrary: resolved, privateLibraryUserID: owner.ID,
	}})
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("reference query failure removed private archive: %v", err)
	}
}

func deleteBookByContractAction(t *testing.T, router http.Handler, auth, action string, bookID uint) {
	t.Helper()
	var request *http.Request
	if action == "single" {
		request = httptest.NewRequest(http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(bookID), 10), nil)
	} else {
		body := fmt.Sprintf(`{"action":"delete","bookIds":[%d]}`, bookID)
		request = httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	want := http.StatusNoContent
	if action == "batch" {
		want = http.StatusOK
	}
	if response.Code != want {
		t.Fatalf("%s delete book %d = %d, want %d: %s", action, bookID, response.Code, want, response.Body.String())
	}
}

func createBookWriteCover(t *testing.T, server *Server, user models.User, name string) string {
	t.Helper()
	url := "/uploads/users/" + strconv.FormatUint(uint64(user.ID), 10) + "/covers/" + name
	path := filepath.Join(server.cfg.DataDir, "uploads", strings.TrimPrefix(url, "/uploads/"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	return url
}

func snapshotBookWriteState(t *testing.T, server *Server, userID uint) []byte {
	t.Helper()
	var snapshot struct {
		Books []models.Book
		Links []models.BookCategory
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Books).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("user_id = ?", userID).Order("id asc").Find(&snapshot.Links).Error; err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func assertBookWriteFailureStable(t *testing.T, fixture bookWriteBoundaryFixture) {
	t.Helper()
	after := snapshotBookWriteState(t, fixture.server, fixture.owner.ID)
	if !bytes.Equal(after, fixture.before) {
		t.Errorf("rejected book write changed persistence\nbefore=%s\nafter=%s", fixture.before, after)
	}
	if events := drainBookWriteEvents(fixture.events); len(events) != 0 {
		t.Errorf("rejected book write broadcast events: %v", events)
	}
}

func padBookWriteBody(body string, total int) string {
	prefix := strings.TrimSuffix(body, "}") + `,"padding":"`
	suffix := `"}`
	if total < len(prefix)+len(suffix) {
		panic("book write contract target body is too small")
	}
	return prefix + strings.Repeat("p", total-len(prefix)-len(suffix)) + suffix
}

func performBookWriteRequest(router http.Handler, auth, method, path, body string, chunked bool) *httptest.ResponseRecorder {
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

func performBookWriteReaderRequest(router http.Handler, auth, method, path string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertBookWriteFlatError(t *testing.T, response *httptest.ResponseRecorder, status int, message string) {
	t.Helper()
	want := fmt.Sprintf(`{"error":%q}`, message)
	if response.Code != status || response.Body.String() != want {
		t.Errorf("book write error = %d %s, want %d %s", response.Code, response.Body.String(), status, want)
	}
}

func drainBookWriteEvents(channel <-chan []byte) []string {
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

type bookWriteNoReadBody struct {
	read bool
}

func (body *bookWriteNoReadBody) Read([]byte) (int, error) {
	body.read = true
	return 0, errors.New("request body must not be read")
}

var _ io.Reader = (*bookWriteNoReadBody)(nil)
