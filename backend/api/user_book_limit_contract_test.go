package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
	"openreader/backend/models"
)

// Contract: docs/compat/user-book-limit-fixed-baseline-second-audit-p2-contract.md.
// reader-dev BookController.kt#saveBook refuses a book that is not on the shelf
// once the shelf holds reader.app.userBookLimit books (default 200), before any
// file is moved or remote book info is fetched; books already on the shelf are
// still updated, and backup restore is not counted. The limit comes from the
// real environment loader, so these cases also pin OPENREADER_USER_BOOK_LIMIT.
// Since the user's decision of 2026-11-13 (contract section 12), an unset or
// non-positive OPENREADER_USER_BOOK_LIMIT means no limit instead of 200.

// legacyDefaultBookLimit is reader-dev's default, which OpenReader shipped
// from 2026-11-12 until the 2026-11-13 decision.
const legacyDefaultBookLimit = 200

func setupBookLimitServer(t *testing.T, limit string) (*gin.Engine, *Server) {
	t.Helper()
	return setupRegistrationLimitServer(t, map[string]string{"OPENREADER_USER_BOOK_LIMIT": limit})
}

func bookLimitUser(t *testing.T, server *Server, username string) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

// seedShelfBooks writes rows directly, like a shelf that existed before the
// limit, without going through any add path.
func seedShelfBooks(t *testing.T, server *Server, userID uint, count int, prefix string) []models.Book {
	t.Helper()
	if count == 0 {
		return nil
	}
	books := make([]models.Book, 0, count)
	for index := 0; index < count; index++ {
		books = append(books, models.Book{
			UserID:    userID,
			Title:     fmt.Sprintf("%s%03d", prefix, index),
			URL:       fmt.Sprintf("https://seeded-shelf.test/%d/%s/%d", userID, prefix, index),
			CanUpdate: true,
		})
	}
	if err := server.db.CreateInBatches(&books, 200).Error; err != nil {
		t.Fatal(err)
	}
	return books
}

func shelfBookCount(t *testing.T, server *Server, userID uint) int64 {
	t.Helper()
	var count int64
	if err := server.db.Model(&models.Book{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func bookLimitJSONRequest(router http.Handler, method, path, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func createShelfBook(router http.Handler, auth, title string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"title": title})
	return bookLimitJSONRequest(router, http.MethodPost, "/api/books", auth, string(body))
}

func assertBookLimitReached(t *testing.T, response *httptest.ResponseRecorder, label string) {
	t.Helper()
	if response.Code != http.StatusForbidden {
		t.Fatalf("%s: status=%d, want 403: %s", label, response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("%s: decode %q: %v", label, response.Body.String(), err)
	}
	if len(payload) != 1 || payload["error"] != "book limit reached" {
		t.Fatalf("%s: body=%s, want {\"error\":\"book limit reached\"}", label, response.Body.String())
	}
}

func libraryFileSnapshot(t *testing.T, root string) []string {
	t.Helper()
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return filepath.SkipDir
			}
			return err
		}
		if !entry.IsDir() {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

const bookLimitRemoteBase = "https://limit-book.test"

// bookLimitRemoteSource serves one book page with one chapter and counts every
// remote request, so a refused add can prove it fetched nothing.
func bookLimitRemoteSource(t *testing.T, server *Server) (models.BookSource, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	restore := engine.SetHTTPClientForTesting(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body><section class="detail-main">
					<h1 class="detail-name">上限远程书</h1>
					<span class="detail-author">上限作者</span>
					<li class="chapter"><a href="/chapter-1"><span class="chapter-title">第一章</span></a></li>
				</section></body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	t.Cleanup(restore)
	source := models.BookSource{Name: "上限测试源", BaseURL: bookLimitRemoteBase, SourceType: 0, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoInitRule:   ".detail-main",
		BookInfoNameRule:   ".detail-name",
		BookInfoAuthorRule: ".detail-author",
		ChapterListRule:    ".chapter",
		ChapterNameRule:    ".chapter-title",
		ChapterURLRule:     "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	return source, &calls
}

func remoteBookBody(title, bookURL string, sourceID uint) string {
	return `{"title":` + strconv.Quote(title) + `,"bookUrl":` + strconv.Quote(bookURL) + `,"sourceId":` + strconv.FormatUint(uint64(sourceID), 10) + `}`
}

// L1
func TestBookLimitStopsCreateBookAtTheConfiguredLimit(t *testing.T) {
	router, server := setupBookLimitServer(t, "2")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")

	for index := 1; index <= 2; index++ {
		if response := createShelfBook(router, auth, fmt.Sprintf("上限书%d", index)); response.Code != http.StatusCreated {
			t.Fatalf("book %d below the limit: status=%d, want 201: %s", index, response.Code, response.Body.String())
		}
	}
	assertBookLimitReached(t, createShelfBook(router, auth, "上限书3"), "third book with a limit of 2")
	if count := shelfBookCount(t, server, user.ID); count != 2 {
		t.Fatalf("a refused add must not create a row: %d books, want 2", count)
	}
}

// L2
func TestBookLimitRefusesNewRemoteBookBeforeFetching(t *testing.T) {
	router, server := setupBookLimitServer(t, "1")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")
	source, calls := bookLimitRemoteSource(t, server)
	existing := models.Book{UserID: user.ID, SourceID: source.ID, Title: "已有远程书", URL: bookLimitRemoteBase + "/existing", CanUpdate: true}
	if err := server.db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	response := bookLimitJSONRequest(router, http.MethodPost, "/api/books/remote", auth,
		remoteBookBody("新远程书", bookLimitRemoteBase+"/new", source.ID))
	assertBookLimitReached(t, response, "new remote book at the limit")
	if fetched := calls.Load(); fetched != 0 {
		t.Fatalf("a refused remote add fetched the source %d times, want 0", fetched)
	}
	if count := shelfBookCount(t, server, user.ID); count != 1 {
		t.Fatalf("a refused remote add must not create a row: %d books, want 1", count)
	}

	// Control: the book already on the shelf is still updated at the limit.
	again := bookLimitJSONRequest(router, http.MethodPost, "/api/books/remote", auth,
		remoteBookBody("已有远程书", bookLimitRemoteBase+"/existing", source.ID))
	if again.Code != http.StatusOK {
		t.Fatalf("existing remote book at the limit: status=%d, want 200: %s", again.Code, again.Body.String())
	}
	if count := shelfBookCount(t, server, user.ID); count != 1 {
		t.Fatalf("re-adding an existing remote book must not add a row: %d books, want 1", count)
	}
}

// L3
func TestBookLimitRefusesLocalImportWithoutWritingTheArchive(t *testing.T) {
	router, server := setupBookLimitServer(t, "1")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")
	seeded := seedShelfBooks(t, server, user.ID, 1, "已有书")

	preview := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books/preview", "上限导入.txt",
		[]byte("第一章 开始\n这是正文内容"), nil)
	if preview.Code != http.StatusOK {
		t.Fatalf("preview: status=%d, want 200: %s", preview.Code, preview.Body.String())
	}
	var staged struct {
		ImportToken string `json:"importToken"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &staged); err != nil || staged.ImportToken == "" {
		t.Fatalf("preview returned no import token: %v %s", err, preview.Body.String())
	}

	before := libraryFileSnapshot(t, server.cfg.LibraryDir)
	refused := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "", nil,
		map[string]string{"importToken": staged.ImportToken})
	assertBookLimitReached(t, refused, "local import at the limit")
	if count := shelfBookCount(t, server, user.ID); count != 1 {
		t.Fatalf("a refused import must not create a row: %d books, want 1", count)
	}
	if after := libraryFileSnapshot(t, server.cfg.LibraryDir); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("a refused import must not write the library archive:\nbefore %v\nafter  %v", before, after)
	}

	// Freeing a slot lets the same staged token import.
	deleted := bookLimitJSONRequest(router, http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(seeded[0].ID), 10), auth, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete seeded book: status=%d: %s", deleted.Code, deleted.Body.String())
	}
	retried := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "", nil,
		map[string]string{"importToken": staged.ImportToken})
	if retried.Code != http.StatusCreated {
		t.Fatalf("retry with the kept import token: status=%d, want 201: %s", retried.Code, retried.Body.String())
	}
	if count := shelfBookCount(t, server, user.ID); count != 1 {
		t.Fatalf("after the retry: %d books, want 1", count)
	}
}

// L4 (control): a restored reader-dev placeholder is already on the shelf.
func TestBookLimitStillReattachesARestoredLocalPlaceholder(t *testing.T) {
	router, server := setupBookLimitServer(t, "1")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")
	placeholder := models.Book{UserID: user.ID, SourceID: 0, Title: "占位书", Author: "占位作者", URL: "local://restored-placeholder", CanUpdate: true}
	if err := server.db.Create(&placeholder).Error; err != nil {
		t.Fatal(err)
	}

	response := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "占位书.txt",
		[]byte("第一章 开始\n这是正文内容"), map[string]string{"title": "占位书", "author": "占位作者"})
	if response.Code != http.StatusCreated {
		t.Fatalf("placeholder import at the limit: status=%d, want 201: %s", response.Code, response.Body.String())
	}
	var imported struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.ID != placeholder.ID {
		t.Fatalf("import created book %d instead of reattaching placeholder %d", imported.ID, placeholder.ID)
	}
	if count := shelfBookCount(t, server, user.ID); count != 1 {
		t.Fatalf("reattaching a placeholder must not add a row: %d books, want 1", count)
	}
}

type storageImportOutcome struct {
	Imported []struct {
		Path  string          `json:"path"`
		Book  json.RawMessage `json:"book"`
		Error string          `json:"error"`
	} `json:"imported"`
}

func assertOneImportedOneRefused(t *testing.T, response *httptest.ResponseRecorder, label string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("%s: status=%d, want 200: %s", label, response.Code, response.Body.String())
	}
	var outcome storageImportOutcome
	if err := json.Unmarshal(response.Body.Bytes(), &outcome); err != nil {
		t.Fatal(err)
	}
	imported, refused := 0, 0
	for _, item := range outcome.Imported {
		switch {
		case len(item.Book) > 0 && string(item.Book) != "null" && item.Error == "":
			imported++
		case item.Error == "book limit reached" && (len(item.Book) == 0 || string(item.Book) == "null"):
			refused++
		default:
			t.Fatalf("%s: unexpected item %+v in %s", label, item, response.Body.String())
		}
	}
	if imported != 1 || refused != 1 {
		t.Fatalf("%s: imported=%d refused=%d, want 1 and 1: %s", label, imported, refused, response.Body.String())
	}
}

// L5
func TestBookLimitReportsEachRefusedStorageImport(t *testing.T) {
	t.Run("local store", func(t *testing.T) {
		router, server := setupBookLimitServer(t, "1")
		auth := authHeader(t, router)
		user := bookLimitUser(t, server, "testuser")
		if err := os.MkdirAll(server.cfg.LocalStoreDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"limit-a.txt", "limit-b.txt"} {
			if err := os.WriteFile(filepath.Join(server.cfg.LocalStoreDir, name), []byte("第一章 开始\n"+name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		response := bookLimitJSONRequest(router, http.MethodPost, "/api/local-store/import", auth, `{"paths":["limit-a.txt","limit-b.txt"]}`)
		assertOneImportedOneRefused(t, response, "local store import past the limit")
		if count := shelfBookCount(t, server, user.ID); count != 1 {
			t.Fatalf("local store import: %d books, want 1", count)
		}
	})
	t.Run("webdav", func(t *testing.T) {
		router, server := setupBookLimitServer(t, "1")
		auth := authHeader(t, router)
		user := bookLimitUser(t, server, "testuser")
		for _, name := range []string{"limit/limit-a.txt", "limit/limit-b.txt"} {
			writeWebDAVTestFile(t, server, name, []byte("第一章 开始\n"+name))
		}
		response := bookLimitJSONRequest(router, http.MethodPost, "/api/webdav/import", auth, `{"paths":["limit/limit-a.txt","limit/limit-b.txt"]}`)
		assertOneImportedOneRefused(t, response, "WebDAV import past the limit")
		if count := shelfBookCount(t, server, user.ID); count != 1 {
			t.Fatalf("WebDAV import: %d books, want 1", count)
		}
	})
}

// L6
func TestBookLimitPerUserOverride(t *testing.T) {
	router, server := setupBookLimitServer(t, "1")
	adminAuth := authHeader(t, router)
	readerAuth := "Bearer " + assertRegistered(t, registerAccount(t, router, "reader02", nil), "reader02")
	reader := bookLimitUser(t, server, "reader02")
	setLimit := func(limit int) {
		t.Helper()
		response := bookLimitJSONRequest(router, http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(reader.ID), 10), adminAuth,
			`{"bookLimit":`+strconv.Itoa(limit)+`}`)
		if response.Code != http.StatusOK {
			t.Fatalf("set bookLimit %d: status=%d: %s", limit, response.Code, response.Body.String())
		}
	}

	setLimit(3)
	for index := 1; index <= 3; index++ {
		if response := createShelfBook(router, readerAuth, fmt.Sprintf("读者书%d", index)); response.Code != http.StatusCreated {
			t.Fatalf("reader book %d under a per-user limit of 3: status=%d: %s", index, response.Code, response.Body.String())
		}
	}
	assertBookLimitReached(t, createShelfBook(router, readerAuth, "读者书4"), "fourth book with a per-user limit of 3")

	// bookLimit 0 follows the instance value again (1 here).
	setLimit(0)
	assertBookLimitReached(t, createShelfBook(router, readerAuth, "读者书5"), "instance limit after clearing the per-user value")
	if count := shelfBookCount(t, server, reader.ID); count != 3 {
		t.Fatalf("reader shelf: %d books, want 3", count)
	}

	// The administrator, whose row cannot be edited, uses the instance value.
	if response := createShelfBook(router, adminAuth, "管理员书1"); response.Code != http.StatusCreated {
		t.Fatalf("administrator first book: status=%d: %s", response.Code, response.Body.String())
	}
	assertBookLimitReached(t, createShelfBook(router, adminAuth, "管理员书2"), "administrator past the instance limit")
}

// L7
func TestBookLimitHoldsUnderConcurrentCreates(t *testing.T) {
	router, server := setupBookLimitServer(t, "3")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")

	const attempts = 8
	statuses := make([]int, attempts)
	bodies := make([]string, attempts)
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for index := 0; index < attempts; index++ {
		done.Add(1)
		go func(index int) {
			defer done.Done()
			start.Wait()
			response := createShelfBook(router, auth, fmt.Sprintf("并发书%d", index))
			statuses[index] = response.Code
			bodies[index] = response.Body.String()
		}(index)
	}
	start.Done()
	done.Wait()

	created, refused := 0, 0
	for index, status := range statuses {
		switch {
		case status == http.StatusCreated:
			created++
		case status == http.StatusForbidden && strings.Contains(bodies[index], "book limit reached"):
			refused++
		default:
			t.Fatalf("attempt %d: status=%d body=%s", index, status, bodies[index])
		}
	}
	if created != 3 || refused != attempts-3 {
		t.Fatalf("concurrent creates: created=%d refused=%d, want 3 and %d", created, refused, attempts-3)
	}
	if count := shelfBookCount(t, server, user.ID); count != 3 {
		t.Fatalf("concurrent creates left %d books, want 3", count)
	}
}

// L8: restore is not counted (control), later additions are.
func TestBookLimitDoesNotLimitBackupRestore(t *testing.T) {
	router, server := setupBookLimitServer(t, "2")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")

	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	shelf, err := zipWriter.Create("myBookShelf.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shelf.Write([]byte(`[
		{"name":"恢复书一","author":"作者","bookUrl":"https://restore-limit.test/1"},
		{"name":"恢复书二","author":"作者","bookUrl":"https://restore-limit.test/2"},
		{"name":"恢复书三","author":"作者","bookUrl":"https://restore-limit.test/3"}
	]`)); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "backup.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/restore-legado", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", auth)
	restored := httptest.NewRecorder()
	router.ServeHTTP(restored, req)
	if restored.Code != http.StatusOK || !strings.Contains(restored.Body.String(), `"books":3`) {
		t.Fatalf("restore three books with a limit of 2: status=%d: %s", restored.Code, restored.Body.String())
	}
	if count := shelfBookCount(t, server, user.ID); count != 3 {
		t.Fatalf("restore must keep every backed-up book: %d books, want 3", count)
	}

	assertBookLimitReached(t, createShelfBook(router, auth, "恢复后新增"), "new book after a restore past the limit")
	if count := shelfBookCount(t, server, user.ID); count != 3 {
		t.Fatalf("after the refused add: %d books, want 3", count)
	}
}

// L9, rewritten for the 2026-11-13 contract: unset, invalid, zero and
// negative values (and a Config built without config.Load) mean no limit;
// positive values still limit (controls). limit 0 means "no limit".
func TestBookLimitEnvironmentParsing(t *testing.T) {
	cases := []struct {
		name  string
		value *string
		limit int
	}{
		{name: "zero-value config", value: nil, limit: 0},
		{name: "unset", value: stringPointer(""), limit: 0},
		{name: "not a number", value: stringPointer("abc"), limit: 0},
		{name: "zero", value: stringPointer("0"), limit: 0},
		{name: "negative", value: stringPointer("-5"), limit: 0},
		{name: "five", value: stringPointer("5"), limit: 5},
		{name: "padded seven", value: stringPointer(" 7 "), limit: 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var router *gin.Engine
			var server *Server
			if tc.value == nil {
				// A Config built without config.Load (UserBookLimit 0).
				router, server = setupTestServer(t)
			} else {
				router, server = setupBookLimitServer(t, *tc.value)
			}
			auth := authHeader(t, router)
			user := bookLimitUser(t, server, "testuser")
			if tc.limit == 0 {
				seedShelfBooks(t, server, user.ID, legacyDefaultBookLimit, "解析")
				for index := 1; index <= 2; index++ {
					if response := createShelfBook(router, auth, fmt.Sprintf("不限的一本%d", index)); response.Code != http.StatusCreated {
						t.Fatalf("book %d with no limit: status=%d, want 201: %s", legacyDefaultBookLimit+index, response.Code, response.Body.String())
					}
				}
				if count := shelfBookCount(t, server, user.ID); count != legacyDefaultBookLimit+2 {
					t.Fatalf("shelf holds %d books, want %d", count, legacyDefaultBookLimit+2)
				}
				return
			}
			seedShelfBooks(t, server, user.ID, tc.limit-1, "解析")
			if response := createShelfBook(router, auth, "最后一本"); response.Code != http.StatusCreated {
				t.Fatalf("book %d of %d: status=%d, want 201: %s", tc.limit, tc.limit, response.Code, response.Body.String())
			}
			assertBookLimitReached(t, createShelfBook(router, auth, "超出的一本"), fmt.Sprintf("book %d with a limit of %d", tc.limit+1, tc.limit))
			if count := shelfBookCount(t, server, user.ID); count != int64(tc.limit) {
				t.Fatalf("shelf holds %d books, want %d", count, tc.limit)
			}
		})
	}
}

// L11: a shelf that already holds more than the old default of 200 books keeps
// accepting new books on every add path when OPENREADER_USER_BOOK_LIMIT is unset.
func TestBookLimitUnsetKeepsAnUpgradedShelfOpen(t *testing.T) {
	router, server := setupBookLimitServer(t, "")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")
	source, calls := bookLimitRemoteSource(t, server)
	seedShelfBooks(t, server, user.ID, legacyDefaultBookLimit+30, "升级前")

	if response := createShelfBook(router, auth, "升级后新书"); response.Code != http.StatusCreated {
		t.Fatalf("POST /api/books past 200 books with no limit: status=%d, want 201: %s", response.Code, response.Body.String())
	}
	remote := bookLimitJSONRequest(router, http.MethodPost, "/api/books/remote", auth,
		remoteBookBody("升级后远程书", bookLimitRemoteBase+"/after-upgrade", source.ID))
	if remote.Code != http.StatusCreated {
		t.Fatalf("remote add past 200 books with no limit: status=%d, want 201: %s", remote.Code, remote.Body.String())
	}
	if calls.Load() == 0 {
		t.Fatalf("the remote add should have fetched the book")
	}
	imported := directLocalBookMultipartRequest(t, router, auth, "/api/imports/books", "升级后导入.txt",
		[]byte("第一章 开始\n这是正文内容"), nil)
	if imported.Code != http.StatusCreated {
		t.Fatalf("local import past 200 books with no limit: status=%d, want 201: %s", imported.Code, imported.Body.String())
	}
	if count := shelfBookCount(t, server, user.ID); count != legacyDefaultBookLimit+33 {
		t.Fatalf("shelf holds %d books, want %d", count, legacyDefaultBookLimit+33)
	}
}

// L12: with no instance limit, a positive per-user bookLimit still limits
// (control); back at 0 the user follows the unlimited instance again.
func TestBookLimitPerUserLimitWithoutAnInstanceLimit(t *testing.T) {
	router, server := setupBookLimitServer(t, "")
	adminAuth := authHeader(t, router)
	readerAuth := "Bearer " + assertRegistered(t, registerAccount(t, router, "reader12", nil), "reader12")
	reader := bookLimitUser(t, server, "reader12")
	setLimit := func(limit int) {
		t.Helper()
		response := bookLimitJSONRequest(router, http.MethodPut, "/api/admin/users/"+strconv.FormatUint(uint64(reader.ID), 10), adminAuth,
			`{"bookLimit":`+strconv.Itoa(limit)+`}`)
		if response.Code != http.StatusOK {
			t.Fatalf("set bookLimit %d: status=%d: %s", limit, response.Code, response.Body.String())
		}
	}

	setLimit(2)
	for index := 1; index <= 2; index++ {
		if response := createShelfBook(router, readerAuth, fmt.Sprintf("读者书%d", index)); response.Code != http.StatusCreated {
			t.Fatalf("reader book %d under a per-user limit of 2: status=%d: %s", index, response.Code, response.Body.String())
		}
	}
	assertBookLimitReached(t, createShelfBook(router, readerAuth, "读者书3"), "third book with a per-user limit of 2 and no instance limit")

	setLimit(0)
	seedShelfBooks(t, server, reader.ID, legacyDefaultBookLimit-2, "补足")
	if response := createShelfBook(router, readerAuth, "跟随实例的一本"); response.Code != http.StatusCreated {
		t.Fatalf("book %d after clearing the per-user limit: status=%d, want 201: %s", legacyDefaultBookLimit+1, response.Code, response.Body.String())
	}
	if count := shelfBookCount(t, server, reader.ID); count != legacyDefaultBookLimit+1 {
		t.Fatalf("reader shelf holds %d books, want %d", count, legacyDefaultBookLimit+1)
	}
}

// L10
func TestBookLimitKeepsAnOverLimitShelfUsable(t *testing.T) {
	router, server := setupBookLimitServer(t, "2")
	auth := authHeader(t, router)
	user := bookLimitUser(t, server, "testuser")
	source, calls := bookLimitRemoteSource(t, server)
	remote := models.Book{UserID: user.ID, SourceID: source.ID, Title: "超限远程书", URL: bookLimitRemoteBase + "/over-existing", CanUpdate: true}
	if err := server.db.Create(&remote).Error; err != nil {
		t.Fatal(err)
	}
	seeded := seedShelfBooks(t, server, user.ID, 4, "超限书")

	renamed := bookLimitJSONRequest(router, http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(seeded[0].ID), 10), auth, `{"title":"超限书改名"}`)
	if renamed.Code != http.StatusOK {
		t.Fatalf("update a book on an over-limit shelf: status=%d, want 200: %s", renamed.Code, renamed.Body.String())
	}
	readded := bookLimitJSONRequest(router, http.MethodPost, "/api/books/remote", auth,
		remoteBookBody("超限远程书", bookLimitRemoteBase+"/over-existing", source.ID))
	if readded.Code != http.StatusOK {
		t.Fatalf("re-add an existing remote book on an over-limit shelf: status=%d, want 200: %s", readded.Code, readded.Body.String())
	}
	assertBookLimitReached(t, createShelfBook(router, auth, "超限新书"), "new book on an over-limit shelf")
	refusedRemote := bookLimitJSONRequest(router, http.MethodPost, "/api/books/remote", auth,
		remoteBookBody("超限新远程书", bookLimitRemoteBase+"/over-new", source.ID))
	assertBookLimitReached(t, refusedRemote, "new remote book on an over-limit shelf")
	if fetched := calls.Load(); fetched != 0 {
		t.Fatalf("over-limit shelf fetched the source %d times, want 0", fetched)
	}
	if count := shelfBookCount(t, server, user.ID); count != 5 {
		t.Fatalf("an over-limit shelf must keep every book: %d books, want 5", count)
	}
}
