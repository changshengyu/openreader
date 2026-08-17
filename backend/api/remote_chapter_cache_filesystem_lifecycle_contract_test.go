package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

func createRemoteCacheContractBook(t *testing.T, server *Server, userID uint, cachePaths ...string) models.Book {
	t.Helper()
	source := models.BookSource{Name: "remote cache boundary source", Enabled: true}
	if err := server.db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:   userID,
		SourceID: source.ID,
		Title:    "remote cache boundary book",
		URL:      "https://remote-cache-boundary.test/book/" + strconv.FormatUint(uint64(userID), 10),
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for index, cachePath := range cachePaths {
		chapter := models.Chapter{
			BookID:    book.ID,
			Index:     index,
			Title:     "chapter " + strconv.Itoa(index),
			URL:       book.URL + "/chapter/" + strconv.Itoa(index),
			CachePath: cachePath,
		}
		if err := server.db.Create(&chapter).Error; err != nil {
			t.Fatal(err)
		}
	}
	return book
}

func TestRemoteChapterCacheReadRejectsOutsideAndSymlinkPaths(t *testing.T) {
	for _, test := range []struct {
		name      string
		cachePath func(t *testing.T, server *Server, outsidePath string) string
	}{
		{
			name: "outside absolute",
			cachePath: func(_ *testing.T, _ *Server, outsidePath string) string {
				return outsidePath
			},
		},
		{
			name: "ancestor symlink",
			cachePath: func(t *testing.T, server *Server, outsidePath string) string {
				t.Helper()
				link := filepath.Join(server.cfg.CacheDir, "escaped")
				if err := os.Symlink(filepath.Dir(outsidePath), link); err != nil {
					t.Fatal(err)
				}
				return filepath.Join("escaped", filepath.Base(outsidePath))
			},
		},
		{
			name: "entry symlink",
			cachePath: func(t *testing.T, server *Server, outsidePath string) string {
				t.Helper()
				link := filepath.Join(server.cfg.CacheDir, "entry.txt")
				if err := os.Symlink(outsidePath, link); err != nil {
					t.Fatal(err)
				}
				return "entry.txt"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, server := setupTestServer(t)
			outsideDir := filepath.Join(filepath.Dir(server.cfg.CacheDir), "outside-read")
			if err := os.MkdirAll(outsideDir, 0o700); err != nil {
				t.Fatal(err)
			}
			outsidePath := filepath.Join(outsideDir, "secret.txt")
			if err := os.WriteFile(outsidePath, []byte("outside chapter sentinel"), 0o600); err != nil {
				t.Fatal(err)
			}
			book := models.Book{ID: 1, UserID: 1, SourceID: 1}
			cachePath := test.cachePath(t, server, outsidePath)
			content, _, err := server.readChapterCache(book, cachePath)
			if err == nil || strings.Contains(string(content), "outside chapter sentinel") {
				t.Fatalf("unsafe cache read returned content=%q err=%v", string(content), err)
			}
			if data, readErr := os.ReadFile(outsidePath); readErr != nil || string(data) != "outside chapter sentinel" {
				t.Fatalf("unsafe read changed outside file: data=%q err=%v", string(data), readErr)
			}
		})
	}
}

func TestRemoteCacheStatsCountOnlyExistingSafeRegularFiles(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	out := filepath.Join(filepath.Dir(server.cfg.CacheDir), "outside-stats")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(out, "secret.txt")
	if err := os.WriteFile(secret, []byte("outside stats sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(server.cfg.CacheDir, "escaped")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(server.cfg.CacheDir, "entry.txt")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(server.cfg.CacheDir, "pipe.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	createRemoteCacheContractBook(t, server, 1,
		filepath.Join("escaped", "secret.txt"),
		"entry.txt",
		"pipe.txt",
		filepath.Join("missing", "chapter.txt"),
	)

	request := httptest.NewRequest(http.MethodGet, "/api/cache/stats", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("cache stats status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"files":0`) || !strings.Contains(response.Body.String(), `"cachedChapters":0`) {
		t.Fatalf("unsafe or missing cache rows were counted: %s", response.Body.String())
	}
	for _, path := range []string{
		secret,
		filepath.Join(server.cfg.CacheDir, "escaped"),
		filepath.Join(server.cfg.CacheDir, "entry.txt"),
		filepath.Join(server.cfg.CacheDir, "pipe.txt"),
	} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("stats changed mounted object %s: %v", path, err)
		}
	}
}

func TestRemoteCacheClearLeavesUnsafeAndSharedObjectsUntouched(t *testing.T) {
	router, server := setupTestServer(t)
	token := authHeader(t, router)
	other := models.User{Username: "remote-cache-other", PasswordHash: "unused", Role: "user"}
	if err := server.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(filepath.Dir(server.cfg.CacheDir), "outside-clear")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	ancestorTarget := filepath.Join(out, "ancestor.txt")
	entryTarget := filepath.Join(out, "entry.txt")
	if err := os.WriteFile(ancestorTarget, []byte("ancestor sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryTarget, []byte("entry sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	ancestorLink := filepath.Join(server.cfg.CacheDir, "escaped")
	entryLink := filepath.Join(server.cfg.CacheDir, "entry.txt")
	fifo := filepath.Join(server.cfg.CacheDir, "pipe.txt")
	if err := os.Symlink(out, ancestorLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(entryTarget, entryLink); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	sharedRelative := filepath.Join("shared", "chapter.txt")
	sharedPath := filepath.Join(server.cfg.CacheDir, sharedRelative)
	if err := os.MkdirAll(filepath.Dir(sharedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sharedPath, []byte("shared chapter"), 0o600); err != nil {
		t.Fatal(err)
	}
	ownerBook := createRemoteCacheContractBook(t, server, 1,
		filepath.Join("escaped", "ancestor.txt"),
		"entry.txt",
		"pipe.txt",
		sharedRelative,
	)
	otherBook := createRemoteCacheContractBook(t, server, other.ID, sharedRelative)
	localBook := models.Book{UserID: 1, SourceID: 0, Title: "local cache must remain"}
	if err := server.db.Create(&localBook).Error; err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join("local", "chapter.txt")
	if err := server.db.Create(&models.Chapter{BookID: localBook.ID, Index: 0, Title: "local", CachePath: localPath}).Error; err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/cache", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("cache clear status=%d body=%s", response.Code, response.Body.String())
	}
	for path, expected := range map[string]string{
		ancestorTarget: "ancestor sentinel",
		entryTarget:    "entry sentinel",
		sharedPath:     "shared chapter",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != expected {
			t.Fatalf("cache clear changed protected file %s: data=%q err=%v", path, string(data), err)
		}
	}
	for _, path := range []string{ancestorLink, entryLink, fifo} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("cache clear removed unsafe mounted object %s: %v", path, err)
		}
	}
	var ownerPaths, otherPaths, localPaths int64
	if err := server.db.Model(&models.Chapter{}).Where("book_id = ? AND cache_path <> ''", ownerBook.ID).Count(&ownerPaths).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&models.Chapter{}).Where("book_id = ? AND cache_path <> ''", otherBook.ID).Count(&otherPaths).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Model(&models.Chapter{}).Where("book_id = ? AND cache_path <> ''", localBook.ID).Count(&localPaths).Error; err != nil {
		t.Fatal(err)
	}
	if ownerPaths != 0 || otherPaths != 1 || localPaths != 1 {
		t.Fatalf("cache clear row scope owner=%d other=%d local=%d", ownerPaths, otherPaths, localPaths)
	}
}

func TestRemoteCachePruneFailsClosedWhenReferenceQueryFails(t *testing.T) {
	_, server := setupTestServer(t)
	relative := filepath.Join("query-failure", "chapter.txt")
	path := filepath.Join(server.cfg.CacheDir, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("must survive query failure"), 0o600); err != nil {
		t.Fatal(err)
	}
	callbackName := "test:remote-cache-reference-query-failure"
	if err := server.db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "chapters" {
			tx.AddError(errors.New("forced cache reference query failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = server.db.Callback().Query().Remove(callbackName)
	})

	files, size := server.pruneUnreferencedRemoteCachePaths([]string{relative})
	if files != 0 || size != 0 {
		t.Fatalf("query failure reported deleted files=%d size=%d", files, size)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "must survive query failure" {
		t.Fatalf("query failure deleted candidate: data=%q err=%v", string(data), err)
	}
}

func TestWriteChapterCacheRejectsSymlinkBoundaries(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, root, relative, outside string)
	}{
		{
			name: "root symlink",
			setup: func(t *testing.T, root, _ string, outside string) {
				t.Helper()
				if err := os.Symlink(outside, root); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "ancestor symlink",
			setup: func(t *testing.T, root, relative, outside string) {
				t.Helper()
				if err := os.MkdirAll(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, filepath.Dir(relative))); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "entry symlink",
			setup: func(t *testing.T, root, relative, outside string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, filepath.Dir(relative)), 0o700); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(outside, "existing.txt")
				if err := os.WriteFile(target, []byte("existing outside bytes"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(root, relative)); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "cache")
			outside := filepath.Join(base, "outside")
			if err := os.MkdirAll(outside, 0o700); err != nil {
				t.Fatal(err)
			}
			bookURL := "https://remote-cache-write.test/book"
			chapterURL := "https://remote-cache-write.test/chapter"
			relative := engine.ChapterCachePath(bookURL, chapterURL)
			test.setup(t, root, relative, outside)
			_, err := engine.WriteChapterCache(root, bookURL, chapterURL, "replacement chapter bytes")
			if err == nil {
				t.Fatal("unsafe chapter-cache write succeeded")
			}
			outsideEntries, readErr := os.ReadDir(outside)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, entry := range outsideEntries {
				path := filepath.Join(outside, entry.Name())
				if entry.Name() == "existing.txt" {
					data, fileErr := os.ReadFile(path)
					if fileErr != nil || string(data) != "existing outside bytes" {
						t.Fatalf("unsafe write changed entry target: data=%q err=%v", string(data), fileErr)
					}
					continue
				}
				t.Fatalf("unsafe write created outside object %s", path)
			}
		})
	}
}
