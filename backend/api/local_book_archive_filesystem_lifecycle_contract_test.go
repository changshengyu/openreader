package api

import (
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

const outsideLocalArchiveSentinel = "outside local archive sentinel"

type ownerRootSymlinkBookFixture struct {
	router           *gin.Engine
	server           *Server
	auth             string
	book             models.Book
	chapter          models.Chapter
	ownerLink        string
	outsideOwnerRoot string
	outsideBookRoot  string
	sourcePath       string
	cachePath        string
}

func setupOwnerRootSymlinkBook(t *testing.T, username string) ownerRootSymlinkBookFixture {
	t.Helper()
	router, server := setupTestServer(t)
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)

	ownerLink := filepath.Join(server.cfg.LibraryDir, "data", username)
	if err := os.MkdirAll(filepath.Dir(ownerLink), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideOwnerRoot := t.TempDir()
	if err := os.Symlink(outsideOwnerRoot, ownerLink); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}

	libraryPath := filepath.Join("data", username, "mounted-book")
	outsideBookRoot := filepath.Join(outsideOwnerRoot, "mounted-book")
	sourcePath := writeLifecycleCache(t, outsideBookRoot, "source.txt", "第一章 越界正文\n"+outsideLocalArchiveSentinel+"\n")
	cacheRelative := filepath.Join("content", "active", "chapter.txt")
	cachePath := writeLifecycleCache(t, outsideBookRoot, cacheRelative, outsideLocalArchiveSentinel)
	book := models.Book{
		UserID:       owner.ID,
		SourceID:     0,
		Title:        "mounted owner root",
		Author:       "boundary fixture",
		URL:          "local://mounted-owner-root",
		LibraryPath:  libraryPath,
		OriginalFile: filepath.Join(libraryPath, "source.txt"),
		TOCFile:      filepath.Join(libraryPath, "chapters.json"),
		SourceFile:   filepath.Join(libraryPath, "bookSource.json"),
		TOCRule:      `^第.+章.*$`,
		LastChapter:  "旧目录",
		ChapterCount: 1,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{
		BookID: book.ID, Index: 0, Title: "旧目录", URL: book.URL + "/chapter_0", CachePath: cacheRelative,
	}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	return ownerRootSymlinkBookFixture{
		router: router, server: server, auth: auth, book: book, chapter: chapter, ownerLink: ownerLink,
		outsideOwnerRoot: outsideOwnerRoot, outsideBookRoot: outsideBookRoot, sourcePath: sourcePath, cachePath: cachePath,
	}
}

func TestLocalBookArchiveReadAndExportRejectOwnerRootSymlink(t *testing.T) {
	t.Run("chapter cache", func(t *testing.T) {
		fixture := setupOwnerRootSymlinkBook(t, "archiveownerread")
		request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/chapters/0/content", nil)
		request.Header.Set("Authorization", fixture.auth)
		response := httptest.NewRecorder()
		fixture.router.ServeHTTP(response, request)

		if strings.Contains(response.Body.String(), outsideLocalArchiveSentinel) {
			t.Fatalf("chapter API read bytes through a symlinked owner root: status=%d body=%q", response.Code, response.Body.String())
		}
		if _, err := os.Stat(fixture.cachePath); err != nil {
			t.Fatalf("rejected mounted cache must remain untouched: %v", err)
		}
	})

	t.Run("generated export fallback", func(t *testing.T) {
		fixture := setupOwnerRootSymlinkBook(t, "archiveownerexport")
		body := `{"bookIds":[` + strconv.FormatUint(uint64(fixture.book.ID), 10) + `],"format":"txt"}`
		request := httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(body))
		request.Header.Set("Authorization", fixture.auth)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		fixture.router.ServeHTTP(response, request)

		if strings.Contains(response.Body.String(), outsideLocalArchiveSentinel) {
			t.Fatalf("generated export read bytes through a symlinked owner root: status=%d body=%q", response.Code, response.Body.String())
		}
		if _, err := os.Stat(fixture.sourcePath); err != nil {
			t.Fatalf("rejected mounted source must remain untouched: %v", err)
		}
	})
}

func TestLocalBookRefreshRejectsOwnerRootSymlinkBeforeMutation(t *testing.T) {
	fixture := setupOwnerRootSymlinkBook(t, "archiveownerrefresh")
	request := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/refresh-local", nil)
	request.Header.Set("Authorization", fixture.auth)
	response := httptest.NewRecorder()
	fixture.router.ServeHTTP(response, request)

	if response.Code == http.StatusOK {
		t.Errorf("refresh through a symlinked owner root returned 200: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), fixture.server.cfg.LibraryDir) || strings.Contains(response.Body.String(), fixture.outsideOwnerRoot) {
		t.Errorf("refresh rejection exposed a host path: %s", response.Body.String())
	}
	var chapters []models.Chapter
	if err := fixture.server.db.Where("book_id = ?", fixture.book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 || chapters[0].ID != fixture.chapter.ID || chapters[0].CachePath != fixture.chapter.CachePath {
		t.Errorf("unsafe refresh replaced the active catalogue: %+v", chapters)
	}
	var persisted models.Book
	if err := fixture.server.db.First(&persisted, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.LastChapter != fixture.book.LastChapter || persisted.ChapterCount != fixture.book.ChapterCount {
		t.Errorf("unsafe refresh changed book metadata: %+v", persisted)
	}
	entries, err := os.ReadDir(fixture.outsideBookRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".refresh-") || entry.Name() == "chapters.json" || entry.Name() == "bookSource.json" {
			t.Errorf("unsafe refresh wrote %q outside LibraryDir", entry.Name())
		}
	}
}

func TestLocalBookRefreshRejectsSymlinkedDerivedParentsBeforeMutation(t *testing.T) {
	router, server := setupTestServer(t)
	username := "archivederivedlink"
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	libraryPath := filepath.Join("data", username, "mounted-book")
	bookRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
	sourcePath := writeLifecycleCache(t, bookRoot, "source.txt", "第一章 新目录\n合法源正文\n")
	outsideContent := t.TempDir()
	outsideMetadata := t.TempDir()
	if err := os.Symlink(outsideContent, filepath.Join(bookRoot, "content")); err != nil {
		t.Skipf("content symlink fixture unavailable: %v", err)
	}
	if err := os.Symlink(outsideMetadata, filepath.Join(bookRoot, "metadata")); err != nil {
		t.Skipf("metadata symlink fixture unavailable: %v", err)
	}
	book := models.Book{
		UserID: owner.ID, SourceID: 0, Title: "symlinked derived parents", URL: "local://derived-parent",
		LibraryPath: libraryPath, OriginalFile: filepath.Join(libraryPath, "source.txt"),
		TOCFile:    filepath.Join(libraryPath, "metadata", "chapters.json"),
		SourceFile: filepath.Join(libraryPath, "metadata", "bookSource.json"),
		TOCRule:    `^第.+章.*$`, LastChapter: "旧目录", ChapterCount: 1,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧目录", URL: book.URL + "/chapter_0"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code == http.StatusOK {
		t.Errorf("refresh with symlinked derived parents returned 200: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), outsideContent) || strings.Contains(response.Body.String(), outsideMetadata) {
		t.Errorf("derived-parent rejection exposed a host path: %s", response.Body.String())
	}
	for name, root := range map[string]string{"content": outsideContent, "metadata": outsideMetadata} {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("unsafe refresh wrote outside %s parent: %v", name, entries)
		}
	}
	var persistedChapter models.Chapter
	if err := server.db.First(&persistedChapter, chapter.ID).Error; err != nil {
		t.Errorf("unsafe refresh replaced the active chapter before rejecting derived parents: %v", err)
	}
	if data, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(data), "合法源正文") {
		t.Errorf("unsafe refresh changed the original archive: data=%q err=%v", string(data), err)
	}
}

func TestLocalBookRefreshRejectsDerivedParentReplacementBeforeCommit(t *testing.T) {
	router, server := setupTestServer(t)
	username := "archiverefreshreplace"
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	libraryPath := filepath.Join("data", username, "mounted-book")
	bookRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
	_ = writeLifecycleCache(t, bookRoot, "source.txt", "第一章 新目录\n合法源正文\n")
	book := models.Book{
		UserID: owner.ID, SourceID: 0, Title: "derived replacement", URL: "local://derived-replacement",
		LibraryPath: libraryPath, OriginalFile: filepath.Join(libraryPath, "source.txt"),
		TOCFile: filepath.Join(libraryPath, "chapters.json"), SourceFile: filepath.Join(libraryPath, "bookSource.json"),
		TOCRule: `^第.+章.*$`, LastChapter: "旧目录", ChapterCount: 1,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧目录", URL: book.URL + "/chapter_0"}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	outsideContent := t.TempDir()
	localRefreshStageTestHook = func(string) error {
		localRefreshStageTestHook = nil
		if err := os.Symlink(outsideContent, filepath.Join(bookRoot, "content")); err != nil {
			t.Skipf("replacement symlink unavailable: %v", err)
		}
		return nil
	}
	defer func() { localRefreshStageTestHook = nil }()

	request := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code == http.StatusOK {
		t.Errorf("refresh committed after derived parent replacement: %s", response.Body.String())
	}
	var persistedChapter models.Chapter
	if err := server.db.First(&persistedChapter, chapter.ID).Error; err != nil {
		t.Errorf("derived parent replacement changed the active catalogue: %v", err)
	}
	var persistedBook models.Book
	if err := server.db.First(&persistedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedBook.LastChapter != "旧目录" || persistedBook.ChapterCount != 1 {
		t.Errorf("derived parent replacement changed book metadata: %+v", persistedBook)
	}
	entries, err := os.ReadDir(outsideContent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("derived parent replacement received staged content: %v", entries)
	}
}

func TestLocalBookDeletionNeverRemovesThroughOwnerRootSymlink(t *testing.T) {
	for _, action := range []string{"single", "batch"} {
		t.Run(action, func(t *testing.T) {
			fixture := setupOwnerRootSymlinkBook(t, "archiveownerdelete"+action)
			deleteBookByContractAction(t, fixture.router, fixture.auth, action, fixture.book.ID)

			if _, err := os.Stat(fixture.sourcePath); err != nil {
				t.Errorf("durable %s delete removed source outside LibraryDir: %v", action, err)
			}
			if _, err := os.Stat(fixture.cachePath); err != nil {
				t.Errorf("durable %s delete removed cache outside LibraryDir: %v", action, err)
			}
			if info, err := os.Lstat(fixture.ownerLink); err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Errorf("unsafe owner-root symlink should remain untouched after %s delete: info=%v err=%v", action, info, err)
			}
		})
	}
}

func TestLocalBookArchiveReadsOpenedIdentityAfterMountedReplacement(t *testing.T) {
	for _, target := range []string{"cache", "source"} {
		t.Run(target, func(t *testing.T) {
			router, server := setupTestServer(t)
			username := "archivereplace" + target
			auth := registerLifecycleToken(t, router, username)
			owner := lifecycleUser(t, server, username)
			libraryPath := filepath.Join("data", username, "mounted-book")
			bookRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
			sourcePath := writeLifecycleCache(t, bookRoot, "source.txt", "第一章 已验证原文件\nverified source bytes\n")
			cacheRelative := filepath.Join("content", "active", "chapter.txt")
			cachePath := writeLifecycleCache(t, bookRoot, cacheRelative, "verified cache bytes")
			book := models.Book{
				UserID: owner.ID, SourceID: 0, Title: "opened identity", URL: "local://opened-identity",
				LibraryPath: libraryPath, OriginalFile: filepath.Join(libraryPath, "source.txt"),
				TOCRule: `^第.+章.*$`, ChapterCount: 1,
			}
			if err := server.db.Create(&book).Error; err != nil {
				t.Fatal(err)
			}
			chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "one", URL: book.URL + "/chapter_0", CachePath: cacheRelative}
			if err := server.db.Create(&chapter).Error; err != nil {
				t.Fatal(err)
			}
			replacement := writeLifecycleCache(t, t.TempDir(), "replacement.txt", outsideLocalArchiveSentinel)
			wantedPath := cachePath
			if target == "source" {
				wantedPath = sourcePath
			}
			resolvedWantedPath, err := filepath.EvalSymlinks(wantedPath)
			if err != nil {
				t.Fatal(err)
			}
			hookCalled := false
			localBookArchiveOpenTestHook = func(openedPath string) {
				if filepath.Clean(openedPath) != filepath.Clean(resolvedWantedPath) {
					return
				}
				hookCalled = true
				localBookArchiveOpenTestHook = nil
				backup := openedPath + ".verified-open"
				if err := os.Rename(openedPath, backup); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(replacement, openedPath); err != nil {
					t.Skipf("replacement symlink unavailable: %v", err)
				}
			}
			defer func() { localBookArchiveOpenTestHook = nil }()

			var request *http.Request
			if target == "cache" {
				request = httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
			} else {
				body := `{"bookIds":[` + strconv.FormatUint(uint64(book.ID), 10) + `],"format":"txt"}`
				request = httptest.NewRequest(http.MethodPost, "/api/books/export", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
			}
			request.Header.Set("Authorization", auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("%s replacement response: status=%d body=%s", target, response.Code, response.Body.String())
			}
			if !hookCalled {
				t.Fatalf("%s replacement hook did not observe the verified opened file", target)
			}
			if strings.Contains(response.Body.String(), outsideLocalArchiveSentinel) {
				t.Fatalf("%s response consumed the mounted replacement: %s", target, response.Body.String())
			}
			want := "verified cache bytes"
			if target == "source" {
				want = "verified source bytes"
			}
			if !strings.Contains(response.Body.String(), want) {
				t.Fatalf("%s response lost the verified opened object: %s", target, response.Body.String())
			}
		})
	}
}

func TestLocalBookDeletionRestoresReplacementInsteadOfRemovingIt(t *testing.T) {
	router, server := setupTestServer(t)
	username := "archivedeletereplace"
	auth := registerLifecycleToken(t, router, username)
	owner := lifecycleUser(t, server, username)
	libraryPath := filepath.Join("data", username, "mounted-book")
	bookRoot := filepath.Join(server.cfg.LibraryDir, libraryPath)
	sourcePath := writeLifecycleCache(t, bookRoot, "source.txt", "verified original directory")
	resolvedBookRoot, err := filepath.EvalSymlinks(bookRoot)
	if err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: owner.ID, SourceID: 0, Title: "delete replacement", LibraryPath: libraryPath, OriginalFile: filepath.Join(libraryPath, "source.txt")}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	backupRoot := resolvedBookRoot + ".verified-directory"
	replacementSentinel := filepath.Join(resolvedBookRoot, "replacement.txt")
	localBookArchiveCleanupTestHook = func(target string) {
		localBookArchiveCleanupTestHook = nil
		if filepath.Clean(target) != filepath.Clean(resolvedBookRoot) {
			t.Fatalf("cleanup hook target=%q want=%q", target, resolvedBookRoot)
		}
		if err := os.Rename(resolvedBookRoot, backupRoot); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(resolvedBookRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(replacementSentinel, []byte(outsideLocalArchiveSentinel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { localBookArchiveCleanupTestHook = nil }()

	deleteBookByContractAction(t, router, auth, "single", book.ID)
	if data, err := os.ReadFile(replacementSentinel); err != nil || string(data) != outsideLocalArchiveSentinel {
		t.Fatalf("validated cleanup removed the replacement object: data=%q err=%v", string(data), err)
	}
	if data, err := os.ReadFile(filepath.Join(backupRoot, filepath.Base(sourcePath))); err != nil || string(data) != "verified original directory" {
		t.Fatalf("cleanup replacement lost the originally validated directory: data=%q err=%v", string(data), err)
	}
}
