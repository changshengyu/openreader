package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/engine"
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

func TestRemoteRefreshReplacesCatalogueAndClearsSupersededCaches(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://refresh-replace.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != upstream+"/book" {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<span class="rename">1</span><h1 class="name">刷新后的目录</h1>
					<div class="chapter"><span class="title">新第一章</span><a href="/new-0">阅读</a></div>
					<div class="chapter"><span class="title">新第二章</span><a href="/new-1">阅读</a></div>
				</body></html>`)),
				Header:  make(http.Header),
				Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "完整刷新书源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{
		BookInfoCanRenameRule: ".rename",
		BookInfoNameRule:      ".name",
		ChapterListRule:       ".chapter",
		ChapterNameRule:       ".title|text",
		ChapterURLRule:        "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "旧目录", URL: upstream + "/book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}

	oldPaths := []string{
		filepath.Join("refresh-replace", "old-0.txt"),
		filepath.Join("refresh-replace", "old-1.txt"),
		filepath.Join("refresh-replace", "old-2.txt"),
	}
	for _, path := range oldPaths {
		writeLifecycleCache(t, server.cfg.CacheDir, path, "obsolete cached chapter")
	}
	oldChapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "旧第一章", URL: upstream + "/old-0", CachePath: oldPaths[0]},
		{BookID: book.ID, Index: 1, Title: "旧第二章", URL: upstream + "/old-1", CachePath: oldPaths[1]},
		{BookID: book.ID, Index: 2, Title: "已删除章节", URL: upstream + "/old-2", CachePath: oldPaths[2]},
	}
	for index := range oldChapters {
		if err := server.db.Create(&oldChapters[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := server.db.Create(&models.ReadingProgress{
		UserID: user.ID, BookID: book.ID, ChapterID: oldChapters[0].ID, ChapterIndex: 0, Offset: 73, Percent: 0.41,
	}).Error; err != nil {
		t.Fatal(err)
	}
	bookmarks := []models.Bookmark{
		{UserID: user.ID, BookID: book.ID, ChapterID: oldChapters[0].ID, ChapterIndex: 0, Offset: 19, Title: "保留位置"},
		{UserID: user.ID, BookID: book.ID, ChapterID: oldChapters[2].ID, ChapterIndex: 2, Offset: 91, Title: "移除章节位置"},
	}
	if err := server.db.Create(&bookmarks).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh remote catalogue: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].Index != 0 || chapters[1].Index != 1 ||
		chapters[0].URL != upstream+"/new-0" || chapters[1].URL != upstream+"/new-1" {
		t.Fatalf("refresh must replace, not merge, the catalogue: %+v", chapters)
	}
	for _, chapter := range chapters {
		if chapter.CachePath != "" {
			t.Fatalf("refreshed chapter %d retained stale cache path %q", chapter.Index, chapter.CachePath)
		}
	}
	for _, oldPath := range oldPaths {
		if _, err := os.Stat(filepath.Join(server.cfg.CacheDir, oldPath)); !os.IsNotExist(err) {
			t.Fatalf("superseded remote cache %q should be removed after commit, stat err=%v", oldPath, err)
		}
	}
	if chapters[0].ID == oldChapters[0].ID {
		t.Fatalf("full catalogue replacement must not reuse old chapter row id %d", oldChapters[0].ID)
	}
	var progress models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterID != chapters[0].ID || progress.ChapterIndex != 0 || progress.Offset != 73 || progress.Percent != 0.41 {
		t.Fatalf("progress should be rebound without losing position: %+v", progress)
	}
	var refreshedBookmarks []models.Bookmark
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).Order("chapter_index asc").Find(&refreshedBookmarks).Error; err != nil {
		t.Fatal(err)
	}
	if len(refreshedBookmarks) != 2 || refreshedBookmarks[0].ChapterID != chapters[0].ID || refreshedBookmarks[0].Offset != 19 ||
		refreshedBookmarks[1].ChapterID != 0 || refreshedBookmarks[1].ChapterIndex != 2 || refreshedBookmarks[1].Offset != 91 {
		t.Fatalf("bookmarks should retain positions but never reference deleted chapter rows: %+v", refreshedBookmarks)
	}
}

func TestChangeSourceReplacesCatalogueAndPrunesOldRemoteCache(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://source-change-replace.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != upstream+"/new-book" {
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header), Request: req}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`<html><body>
					<span class="rename">1</span><h1 class="name">替换书源后的书</h1>
					<div class="chapter"><span class="title">换源第一章</span><a href="/source-b-0">阅读</a></div>
				</body></html>`)),
				Header: make(http.Header), Request: req,
			}, nil
		}),
	})
	defer restoreHTTPClient()

	oldSource := models.BookSource{Name: "旧书源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := server.db.Create(&oldSource).Error; err != nil {
		t.Fatal(err)
	}
	newSource := models.BookSource{Name: "新书源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := newSource.SetRules(models.BookSourceRule{
		BookInfoCanRenameRule: ".rename",
		BookInfoNameRule:      ".name",
		ChapterListRule:       ".chapter",
		ChapterNameRule:       ".title|text",
		ChapterURLRule:        "a|attr:href",
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&newSource).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: oldSource.ID, Title: "旧书源书籍", URL: upstream + "/old-book"}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("source-change-replace", "old.txt")
	writeLifecycleCache(t, server.cfg.CacheDir, cachePath, "旧书源正文")
	oldChapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧章节", URL: upstream + "/old-chapter", CachePath: cachePath}
	if err := server.db.Create(&oldChapter).Error; err != nil {
		t.Fatal(err)
	}
	staleChapter := models.Chapter{BookID: book.ID, Index: 1, Title: "旧章节二", URL: upstream + "/old-chapter-1"}
	if err := server.db.Create(&staleChapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterID: oldChapter.ID, ChapterIndex: 0, Offset: 37}).Error; err != nil {
		t.Fatal(err)
	}
	bookmark := models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterID: staleChapter.ID, ChapterIndex: 1, Offset: 64, Title: "旧章节书签"}
	if err := server.db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"sourceId":` + strconv.FormatUint(uint64(newSource.ID), 10) + `,"bookUrl":` + strconv.Quote(upstream+"/new-book") + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/change-source", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("change source: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].URL != upstream+"/source-b-0" || chapters[0].CachePath != "" || chapters[0].ID == oldChapter.ID {
		t.Fatalf("source change did not publish a clean replacement catalogue: %+v", chapters)
	}
	if _, err := os.Stat(filepath.Join(server.cfg.CacheDir, cachePath)); !os.IsNotExist(err) {
		t.Fatalf("old source cache must be removed after source change, stat err=%v", err)
	}
	var progress models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterID != chapters[0].ID || progress.ChapterIndex != 0 || progress.Offset != 37 {
		t.Fatalf("source change should rebind progress to the replacement chapter: %+v", progress)
	}
	var refreshedBookmark models.Bookmark
	if err := server.db.First(&refreshedBookmark, bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshedBookmark.ChapterID != 0 || refreshedBookmark.ChapterIndex != 1 || refreshedBookmark.Offset != 64 {
		t.Fatalf("removed source chapter should clear bookmark id but preserve its position: %+v", refreshedBookmark)
	}
}

func TestRemoteRefreshFetchFailureLeavesCatalogueAndCacheReadable(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://refresh-failure.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("forced remote refresh fetch failure")
		}),
	})
	defer restoreHTTPClient()

	source := models.BookSource{Name: "刷新失败书源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := source.SetRules(models.BookSourceRule{ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: source.ID, Title: "刷新前目录", URL: upstream + "/book", ChapterCount: 1}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("refresh-failure", "chapter.txt")
	cacheFile := writeLifecycleCache(t, server.cfg.CacheDir, cachePath, "刷新前缓存正文")
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "刷新前章节", URL: upstream + "/old", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("failed refresh: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var persistedBook models.Book
	if err := server.db.First(&persistedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	var persistedChapter models.Chapter
	if err := server.db.First(&persistedChapter, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedBook.Title != book.Title || persistedBook.ChapterCount != 1 || persistedChapter.CachePath != cachePath || persistedChapter.URL != chapter.URL {
		t.Fatalf("failed refresh must retain the readable catalogue: book=%+v chapter=%+v", persistedBook, persistedChapter)
	}
	if content, err := os.ReadFile(cacheFile); err != nil || string(content) != "刷新前缓存正文" {
		t.Fatalf("failed refresh must retain cached content, content=%q err=%v", string(content), err)
	}
}

func TestChangeSourceFetchFailureLeavesCatalogueAndCacheReadable(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	const upstream = "https://source-change-failure.test"
	restoreHTTPClient := engine.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("forced source-change fetch failure")
		}),
	})
	defer restoreHTTPClient()

	oldSource := models.BookSource{Name: "换源失败旧源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := server.db.Create(&oldSource).Error; err != nil {
		t.Fatal(err)
	}
	newSource := models.BookSource{Name: "换源失败新源", BaseURL: upstream, Charset: "utf-8", Enabled: true}
	if err := newSource.SetRules(models.BookSourceRule{ChapterListRule: ".chapter", ChapterNameRule: "a|text", ChapterURLRule: "a|attr:href"}); err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&newSource).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: user.ID, SourceID: oldSource.ID, Title: "换源前目录", URL: upstream + "/old-book", ChapterCount: 1}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join("source-change-failure", "chapter.txt")
	cacheFile := writeLifecycleCache(t, server.cfg.CacheDir, cachePath, "换源前缓存正文")
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "换源前章节", URL: upstream + "/old-chapter", CachePath: cachePath}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"sourceId":` + strconv.FormatUint(uint64(newSource.ID), 10) + `,"bookUrl":` + strconv.Quote(upstream+"/new-book") + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/change-source", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("failed source change: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var persistedBook models.Book
	if err := server.db.First(&persistedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	var persistedChapter models.Chapter
	if err := server.db.First(&persistedChapter, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedBook.SourceID != oldSource.ID || persistedBook.URL != book.URL || persistedChapter.CachePath != cachePath || persistedChapter.URL != chapter.URL {
		t.Fatalf("failed source change must retain current source catalogue: book=%+v chapter=%+v", persistedBook, persistedChapter)
	}
	if content, err := os.ReadFile(cacheFile); err != nil || string(content) != "换源前缓存正文" {
		t.Fatalf("failed source change must retain cached content, content=%q err=%v", string(content), err)
	}
}

func TestLocalRefreshClearsStaleChapterReferencesWithoutDeletingOriginal(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	libraryPath := filepath.Join("data", "testuser", "local-refresh-reference-contract")
	originalFile := filepath.Join(libraryPath, "source.txt")
	originalPath := filepath.Join(server.cfg.LibraryDir, originalFile)
	originalContent := "第一章 新内容\n这是唯一保留章节。\n"
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte(originalContent), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: user.ID, Title: "本地刷新引用", URL: "local://refresh-reference", LibraryPath: libraryPath,
		OriginalFile: originalFile, TOCRule: `^第.+章.*$`, ChapterCount: 2,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	oldChapters := []models.Chapter{
		{BookID: book.ID, Index: 0, Title: "旧第一章", URL: book.URL + "/chapter_0", CachePath: filepath.Join("content", "old-0.txt")},
		{BookID: book.ID, Index: 1, Title: "旧第二章", URL: book.URL + "/chapter_1", CachePath: filepath.Join("content", "old-1.txt")},
	}
	for index := range oldChapters {
		if err := server.db.Create(&oldChapters[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := server.db.Create(&models.ReadingProgress{UserID: user.ID, BookID: book.ID, ChapterID: oldChapters[1].ID, ChapterIndex: 1, Offset: 88, Percent: 0.63}).Error; err != nil {
		t.Fatal(err)
	}
	bookmark := models.Bookmark{UserID: user.ID, BookID: book.ID, ChapterID: oldChapters[0].ID, ChapterIndex: 0, Offset: 12, Title: "首章书签"}
	if err := server.db.Create(&bookmark).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh local book: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if data, err := os.ReadFile(originalPath); err != nil || string(data) != originalContent {
		t.Fatalf("local refresh must retain the original archive, data=%q err=%v", string(data), err)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].Index != 0 || chapters[0].ID == oldChapters[0].ID || chapters[0].CachePath == "" {
		t.Fatalf("local refresh must replace catalogue rows and publish derived cache: %+v", chapters)
	}
	var progress models.ReadingProgress
	if err := server.db.Where("user_id = ? AND book_id = ?", user.ID, book.ID).First(&progress).Error; err != nil {
		t.Fatal(err)
	}
	if progress.ChapterID != 0 || progress.ChapterIndex != 1 || progress.Offset != 88 || progress.Percent != 0.63 {
		t.Fatalf("removed local chapter should clear progress id but preserve resume position: %+v", progress)
	}
	var refreshedBookmark models.Bookmark
	if err := server.db.First(&refreshedBookmark, bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshedBookmark.ChapterID != chapters[0].ID || refreshedBookmark.ChapterIndex != 0 || refreshedBookmark.Offset != 12 {
		t.Fatalf("surviving local chapter bookmark should be rebound: %+v", refreshedBookmark)
	}
}

func TestLocalRefreshStageFailurePreservesActiveCatalogueAndArchive(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	libraryPath := filepath.Join("data", "testuser", "local-refresh-stage-failure")
	originalFile := filepath.Join(libraryPath, "source.txt")
	originalPath := filepath.Join(server.cfg.LibraryDir, originalFile)
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	originalContent := "第一章 新正文\n这次刷新不应写入活动目录。\n"
	if err := os.WriteFile(originalPath, []byte(originalContent), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTOC := []byte("[\n  {\"title\": \"旧目录\"}\n]\n")
	oldSource := []byte("[{\"name\":\"旧书源元数据\"}]\n")
	if err := os.WriteFile(filepath.Join(server.cfg.LibraryDir, libraryPath, "chapters.json"), oldTOC, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LibraryDir, libraryPath, "bookSource.json"), oldSource, 0o644); err != nil {
		t.Fatal(err)
	}
	oldCachePath := filepath.Join("content", "previous", "chapter.txt")
	oldContentPath := writeLifecycleCache(t, filepath.Join(server.cfg.LibraryDir, libraryPath), oldCachePath, "旧活动正文")
	book := models.Book{
		UserID: user.ID, Title: "本地刷新写入失败", URL: "local://stage-failure", LibraryPath: libraryPath,
		OriginalFile: originalFile, TOCFile: filepath.Join(libraryPath, "chapters.json"), SourceFile: filepath.Join(libraryPath, "bookSource.json"),
		TOCRule: `^第.+章.*$`, LastChapter: "旧目录", ChapterCount: 1,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	oldChapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧目录", URL: book.URL + "/chapter_0", CachePath: oldCachePath}
	if err := server.db.Create(&oldChapter).Error; err != nil {
		t.Fatal(err)
	}

	localRefreshStageTestHook = func(string) error {
		return errors.New("forced staged artifact write failure")
	}
	defer func() { localRefreshStageTestHook = nil }()
	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("staged write failure: expected 500, got %d: %s", w.Code, w.Body.String())
	}

	var persistedBook models.Book
	if err := server.db.First(&persistedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedBook.ChapterCount != 1 || persistedBook.LastChapter != "旧目录" || persistedBook.TOCRule != `^第.+章.*$` {
		t.Fatalf("failed staging must retain the previous catalogue metadata: %+v", persistedBook)
	}
	var chapters []models.Chapter
	if err := server.db.Where("book_id = ?", book.ID).Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].ID != oldChapter.ID || chapters[0].CachePath != oldCachePath {
		t.Fatalf("failed staging must retain the active chapter row: %+v", chapters)
	}
	if content, err := os.ReadFile(oldContentPath); err != nil || string(content) != "旧活动正文" {
		t.Fatalf("failed staging must retain active derived content, content=%q err=%v", string(content), err)
	}
	if content, err := os.ReadFile(filepath.Join(server.cfg.LibraryDir, libraryPath, "chapters.json")); err != nil || string(content) != string(oldTOC) {
		t.Fatalf("failed staging must retain chapters metadata, content=%q err=%v", string(content), err)
	}
	if content, err := os.ReadFile(filepath.Join(server.cfg.LibraryDir, libraryPath, "bookSource.json")); err != nil || string(content) != string(oldSource) {
		t.Fatalf("failed staging must retain book-source metadata, content=%q err=%v", string(content), err)
	}
	if content, err := os.ReadFile(originalPath); err != nil || string(content) != originalContent {
		t.Fatalf("failed staging must retain original import, content=%q err=%v", string(content), err)
	}
	entries, err := os.ReadDir(filepath.Join(server.cfg.LibraryDir, libraryPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".refresh-") {
			t.Fatalf("failed staging left an inactive refresh directory %q", entry.Name())
		}
	}
}

func TestLocalRefreshPromotesNewGenerationAndPrunesOldDerivedContent(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	user := lifecycleUser(t, server, "testuser")

	libraryPath := filepath.Join("data", "testuser", "local-refresh-promote")
	originalFile := filepath.Join(libraryPath, "source.txt")
	originalPath := filepath.Join(server.cfg.LibraryDir, originalFile)
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	originalContent := "第一章 新目录\n新的活动正文。\n"
	if err := os.WriteFile(originalPath, []byte(originalContent), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID: user.ID, Title: "本地刷新切换", URL: "local://refresh-promote", LibraryPath: libraryPath,
		OriginalFile: originalFile, TOCFile: filepath.Join(libraryPath, "chapters.json"), SourceFile: filepath.Join(libraryPath, "bookSource.json"),
		TOCRule: `^第.+章.*$`, LastChapter: "旧目录", ChapterCount: 1,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	oldCachePath := filepath.Join("content", "old-generation", "chapter.txt")
	oldContentPath := writeLifecycleCache(t, filepath.Join(server.cfg.LibraryDir, libraryPath), oldCachePath, "旧活动正文")
	oldChapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧目录", URL: book.URL + "/chapter_0", CachePath: oldCachePath}
	if err := server.db.Create(&oldChapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LibraryDir, libraryPath, "chapters.json"), []byte("[{\"title\":\"旧目录\"}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.cfg.LibraryDir, libraryPath, "bookSource.json"), []byte("[{\"name\":\"旧元数据\"}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh local book: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var refreshedBook models.Book
	if err := server.db.First(&refreshedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshedBook.LastChapter != "第一章 新目录" || refreshedBook.ChapterCount != 1 {
		t.Fatalf("local refresh did not commit the new catalogue metadata: %+v", refreshedBook)
	}
	var chapter models.Chapter
	if err := server.db.Where("book_id = ? AND `index` = 0", book.ID).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if chapter.ID == oldChapter.ID || !strings.HasPrefix(chapter.CachePath, "content"+string(os.PathSeparator)) || chapter.CachePath == oldCachePath {
		t.Fatalf("local refresh should point at a new content generation: %+v", chapter)
	}
	newContentPath := filepath.Join(server.cfg.LibraryDir, libraryPath, chapter.CachePath)
	if content, err := os.ReadFile(newContentPath); err != nil || !strings.Contains(string(content), "新的活动正文") {
		t.Fatalf("new generation should contain refreshed content, content=%q err=%v", string(content), err)
	}
	if _, err := os.Stat(oldContentPath); !os.IsNotExist(err) {
		t.Fatalf("obsolete derived content should be pruned after promotion, stat err=%v", err)
	}

	var archived []engine.ArchivedChapter
	tocData, err := os.ReadFile(filepath.Join(server.cfg.LibraryDir, refreshedBook.TOCFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tocData, &archived); err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != chapter.ID || archived[0].CachePath != chapter.CachePath || archived[0].Title != chapter.Title {
		t.Fatalf("active chapters metadata must match committed chapter rows: %+v", archived)
	}
	var archivedSource []engine.ArchivedBookSource
	sourceData, err := os.ReadFile(filepath.Join(server.cfg.LibraryDir, refreshedBook.SourceFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sourceData, &archivedSource); err != nil {
		t.Fatal(err)
	}
	if len(archivedSource) != 1 || archivedSource[0].LatestChapterTitle != refreshedBook.LastChapter {
		t.Fatalf("active source metadata must match committed book metadata: %+v", archivedSource)
	}
	if content, err := os.ReadFile(originalPath); err != nil || string(content) != originalContent {
		t.Fatalf("refresh must retain original archive, content=%q err=%v", string(content), err)
	}
}
