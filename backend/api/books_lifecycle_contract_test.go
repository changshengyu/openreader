package api

import (
	"encoding/json"
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

func registerLifecycleToken(t *testing.T, router *gin.Engine, username string) string {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/register",
		strings.NewReader(`{"username":"`+username+`","password":"lifecycle-pass"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("register %s: expected 200, got %d: %s", username, w.Code, w.Body.String())
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Token == "" {
		t.Fatalf("register %s returned no token", username)
	}
	return "Bearer " + response.Token
}

func lifecycleUser(t *testing.T, server *Server, username string) models.User {
	t.Helper()
	var user models.User
	if err := server.db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func writeLifecycleCache(t *testing.T, root, relativePath, content string) string {
	t.Helper()
	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return fullPath
}

func TestCacheStatsAndClearAreScopedToCurrentUser(t *testing.T) {
	router, server := setupTestServer(t)
	tokenA := registerLifecycleToken(t, router, "cache-owner")
	registerLifecycleToken(t, router, "cache-other")
	userA := lifecycleUser(t, server, "cache-owner")
	userB := lifecycleUser(t, server, "cache-other")

	source := models.BookSource{Name: "cache lifecycle source", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	bookA := models.Book{UserID: userA.ID, SourceID: source.ID, Title: "owner remote"}
	bookB := models.Book{UserID: userB.ID, SourceID: source.ID, Title: "other remote"}
	if err := server.db.Create(&bookA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&bookB).Error; err != nil {
		t.Fatal(err)
	}

	cacheA := filepath.Join("lifecycle", "owner.txt")
	cacheB := filepath.Join("lifecycle", "other.txt")
	cacheAPath := writeLifecycleCache(t, server.cfg.CacheDir, cacheA, "owner cache")
	cacheBPath := writeLifecycleCache(t, server.cfg.CacheDir, cacheB, "other cache")
	if err := server.db.Create(&models.Chapter{BookID: bookA.ID, Index: 0, Title: "owner", CachePath: cacheA}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: bookB.ID, Index: 0, Title: "other", CachePath: cacheB}).Error; err != nil {
		t.Fatal(err)
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/api/cache/stats", nil)
	statsReq.Header.Set("Authorization", tokenA)
	statsWriter := httptest.NewRecorder()
	router.ServeHTTP(statsWriter, statsReq)
	if statsWriter.Code != http.StatusOK {
		t.Fatalf("cache stats: expected 200, got %d: %s", statsWriter.Code, statsWriter.Body.String())
	}
	var stats struct {
		Path           string `json:"path"`
		Files          int    `json:"files"`
		CachedChapters int64  `json:"cachedChapters"`
	}
	if err := json.Unmarshal(statsWriter.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Path != "" {
		t.Fatalf("cache stats leaked server cache path %q", stats.Path)
	}
	if stats.Files != 1 || stats.CachedChapters != 1 {
		t.Fatalf("cache stats should contain only the current user's cache, got %+v", stats)
	}

	clearReq := httptest.NewRequest(http.MethodDelete, "/api/cache", nil)
	clearReq.Header.Set("Authorization", tokenA)
	clearWriter := httptest.NewRecorder()
	router.ServeHTTP(clearWriter, clearReq)
	if clearWriter.Code != http.StatusOK {
		t.Fatalf("clear current user cache: expected 200, got %d: %s", clearWriter.Code, clearWriter.Body.String())
	}
	if _, err := os.Stat(cacheAPath); !os.IsNotExist(err) {
		t.Fatalf("current user's cache should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(cacheBPath); err != nil {
		t.Fatalf("other user's cache must remain, stat err=%v", err)
	}
	var chapterA, chapterB models.Chapter
	if err := server.db.Where("book_id = ?", bookA.ID).First(&chapterA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Where("book_id = ?", bookB.ID).First(&chapterB).Error; err != nil {
		t.Fatal(err)
	}
	if chapterA.CachePath != "" || chapterB.CachePath != cacheB {
		t.Fatalf("cache clear crossed user boundaries: owner=%q other=%q", chapterA.CachePath, chapterB.CachePath)
	}
}

func TestBookDeletionCleansOnlyOwnedDerivedFiles(t *testing.T) {
	router, server := setupTestServer(t)
	tokenA := registerLifecycleToken(t, router, "delete-owner")
	registerLifecycleToken(t, router, "delete-other")
	userA := lifecycleUser(t, server, "delete-owner")
	userB := lifecycleUser(t, server, "delete-other")

	source := models.BookSource{Name: "delete lifecycle source", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: userA.ID, Name: "删除分组", Show: true}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	remoteA := models.Book{UserID: userA.ID, SourceID: source.ID, Title: "owner remote delete"}
	remoteB := models.Book{UserID: userB.ID, SourceID: source.ID, Title: "other remote keep"}
	if err := server.db.Create(&remoteA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&remoteB).Error; err != nil {
		t.Fatal(err)
	}
	cacheA := filepath.Join("delete-lifecycle", "owner.txt")
	cacheB := filepath.Join("delete-lifecycle", "other.txt")
	cacheAPath := writeLifecycleCache(t, server.cfg.CacheDir, cacheA, "owner remote cache")
	cacheBPath := writeLifecycleCache(t, server.cfg.CacheDir, cacheB, "other remote cache")
	if err := server.db.Create(&models.Chapter{BookID: remoteA.ID, Index: 0, Title: "owner", CachePath: cacheA}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Chapter{BookID: remoteB.ID, Index: 0, Title: "other", CachePath: cacheB}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.BookCategory{UserID: userA.ID, BookID: remoteA.ID, CategoryID: category.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.Bookmark{UserID: userA.ID, BookID: remoteA.ID, Title: "owner bookmark"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReadingProgress{UserID: userA.ID, BookID: remoteA.ID, ChapterIndex: 0}).Error; err != nil {
		t.Fatal(err)
	}

	remoteDelete := httptest.NewRequest(http.MethodDelete, "/api/books/"+strconv.FormatUint(uint64(remoteA.ID), 10), nil)
	remoteDelete.Header.Set("Authorization", tokenA)
	remoteWriter := httptest.NewRecorder()
	router.ServeHTTP(remoteWriter, remoteDelete)
	if remoteWriter.Code != http.StatusNoContent {
		t.Fatalf("delete remote book: expected 204, got %d: %s", remoteWriter.Code, remoteWriter.Body.String())
	}
	if _, err := os.Stat(cacheAPath); !os.IsNotExist(err) {
		t.Fatalf("deleted remote book cache should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(cacheBPath); err != nil {
		t.Fatalf("other user's cache must remain, stat err=%v", err)
	}
	var deletedBookCount int64
	if err := server.db.Model(&models.Book{}).Where("id = ?", remoteA.ID).Count(&deletedBookCount).Error; err != nil {
		t.Fatal(err)
	}
	if deletedBookCount != 0 {
		t.Fatalf("deleted remote book left %d book rows", deletedBookCount)
	}
	for _, model := range []any{&models.Chapter{}, &models.BookCategory{}, &models.Bookmark{}, &models.ReadingProgress{}} {
		var count int64
		if err := server.db.Model(model).Where("book_id = ?", remoteA.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("deleted remote book left %d %T rows", count, model)
		}
	}

	ownerLibraryPath := filepath.Join("data", "delete-owner", "direct-import")
	otherLibraryPath := filepath.Join("data", "delete-other", "direct-import")
	ownerLibraryRoot := filepath.Join(server.cfg.LibraryDir, ownerLibraryPath)
	otherLibraryRoot := filepath.Join(server.cfg.LibraryDir, otherLibraryPath)
	writeLifecycleCache(t, ownerLibraryRoot, "source.txt", "owner local source")
	writeLifecycleCache(t, otherLibraryRoot, "source.txt", "other local source")
	localA := models.Book{UserID: userA.ID, Title: "owner local delete", LibraryPath: ownerLibraryPath, OriginalFile: filepath.Join(ownerLibraryPath, "source.txt")}
	localB := models.Book{UserID: userB.ID, Title: "other local keep", LibraryPath: otherLibraryPath, OriginalFile: filepath.Join(otherLibraryPath, "source.txt")}
	if err := server.db.Create(&localA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&localB).Error; err != nil {
		t.Fatal(err)
	}

	batchDelete := httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(`{"action":"delete","bookIds":[`+strconv.FormatUint(uint64(localA.ID), 10)+`]}`))
	batchDelete.Header.Set("Authorization", tokenA)
	batchDelete.Header.Set("Content-Type", "application/json")
	batchWriter := httptest.NewRecorder()
	router.ServeHTTP(batchWriter, batchDelete)
	if batchWriter.Code != http.StatusOK {
		t.Fatalf("batch delete local book: expected 200, got %d: %s", batchWriter.Code, batchWriter.Body.String())
	}
	if _, err := os.Stat(ownerLibraryRoot); !os.IsNotExist(err) {
		t.Fatalf("deleted direct-import archive should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(otherLibraryRoot); err != nil {
		t.Fatalf("other user's import archive must remain, stat err=%v", err)
	}
}

func TestSingleLocalBookExportReturnsOriginalArchive(t *testing.T) {
	router, server := setupTestServer(t)
	token := registerLifecycleToken(t, router, "export-owner")
	user := lifecycleUser(t, server, "export-owner")

	libraryPath := filepath.Join("data", "export-owner", "original-export")
	originalName := "原始书籍.epub"
	originalPath := filepath.Join(libraryPath, originalName)
	original := []byte("original epub bytes")
	writeLifecycleCache(t, server.cfg.LibraryDir, originalPath, string(original))
	book := models.Book{
		UserID:       user.ID,
		Title:        "导出本地书",
		LibraryPath:  libraryPath,
		OriginalFile: originalPath,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(`{"bookIds":[`+strconv.FormatUint(uint64(book.ID), 10)+`],"format":"txt"}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("export local original: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.Bytes(); string(got) != string(original) {
		t.Fatalf("local export should preserve original archive bytes, got %q", string(got))
	}
	if disposition := w.Header().Get("Content-Disposition"); !strings.Contains(disposition, "filename*=UTF-8''") || !strings.Contains(disposition, "%E5%8E%9F%E5%A7%8B%E4%B9%A6%E7%B1%8D.epub") {
		t.Fatalf("local export should use safe original filename, got %q", disposition)
	}
}

func TestShelfBatchOperationsRejectForeignBookIDs(t *testing.T) {
	router, server := setupTestServer(t)
	tokenA := registerLifecycleToken(t, router, "batch-owner")
	registerLifecycleToken(t, router, "batch-other")
	userA := lifecycleUser(t, server, "batch-owner")
	userB := lifecycleUser(t, server, "batch-other")

	bookA := models.Book{UserID: userA.ID, Title: "owner batch book"}
	bookB := models.Book{UserID: userB.ID, Title: "other batch book"}
	categoryB := models.Category{UserID: userB.ID, Name: "other category", Show: true}
	if err := server.db.Create(&bookA).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&bookB).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&categoryB).Error; err != nil {
		t.Fatal(err)
	}

	foreignCategory := httptest.NewRequest(http.MethodPut, "/api/books/"+strconv.FormatUint(uint64(bookA.ID), 10)+"/category", strings.NewReader(`{"categoryIds":[`+strconv.FormatUint(uint64(categoryB.ID), 10)+`]}`))
	foreignCategory.Header.Set("Authorization", tokenA)
	foreignCategory.Header.Set("Content-Type", "application/json")
	foreignCategoryWriter := httptest.NewRecorder()
	router.ServeHTTP(foreignCategoryWriter, foreignCategory)
	if foreignCategoryWriter.Code != http.StatusBadRequest {
		t.Fatalf("foreign category: expected 400, got %d: %s", foreignCategoryWriter.Code, foreignCategoryWriter.Body.String())
	}

	foreignID := strconv.FormatUint(uint64(bookB.ID), 10)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/books/batch", strings.NewReader(`{"action":"delete","bookIds":[`+foreignID+`]}`)),
		httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(`{"bookIds":[`+foreignID+`],"format":"json"}`)),
	} {
		request.Header.Set("Authorization", tokenA)
		request.Header.Set("Content-Type", "application/json")
		writer := httptest.NewRecorder()
		router.ServeHTTP(writer, request)
		if writer.Code != http.StatusNotFound {
			t.Fatalf("foreign book operation %s: expected 404, got %d: %s", request.URL.Path, writer.Code, writer.Body.String())
		}
	}

	var otherCount int64
	if err := server.db.Model(&models.Book{}).Where("id = ?", bookB.ID).Count(&otherCount).Error; err != nil {
		t.Fatal(err)
	}
	if otherCount != 1 {
		t.Fatalf("foreign batch operation must not remove the other user's book, got %d rows", otherCount)
	}
}
