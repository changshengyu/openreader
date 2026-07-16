package api

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
	"openreader/backend/services/backup"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

// historicalLocalVolume is deliberately written as an old on-disk SQLite
// database, then closed and reopened. It is not a current models.Book fixture:
// the resource/variable columns are removed before the production-style
// AutoMigrate pass. The original archive is also intentionally addressed by a
// stale absolute path, which is how early OpenReader cache rows can look after
// a Docker host move.
type historicalLocalVolume struct {
	cfg              config.Config
	username         string
	password         string
	book             models.Book
	chapter          models.Chapter
	progress         models.ReadingProgress
	bookmark         models.Bookmark
	archivePath      string
	hostSourcePath   string
	hostCachePath    string
	archiveSource    string
	archiveDirectory string
	cacheBook        models.Book
	cacheChapter     models.Chapter
	cacheArchivePath string
	cacheArchiveBody string
	legacyCachePath  string
	legacyCacheBody  string
	otherUsername    string
	otherPassword    string
	otherBook        models.Book
	otherChapter     models.Chapter
	otherArchivePath string
	otherArchiveBody string
}

func setupHistoricalLocalVolume(t *testing.T) (*gin.Engine, *Server, historicalLocalVolume) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	fixture := historicalLocalVolume{
		cfg: config.Config{
			DataDir:       filepath.Join(root, "data"),
			CacheDir:      filepath.Join(root, "cache"),
			LibraryDir:    filepath.Join(root, "library"),
			DatabasePath:  filepath.Join(root, "data", "openreader.db"),
			JWTSecret:     "old-volume-test-secret",
			LocalStoreDir: filepath.Join(root, "library", "localStore"),
		},
		username:      "legacy_owner",
		password:      "legacy-secret",
		otherUsername: "legacy_other",
		otherPassword: "other-secret",
	}

	database, err := readerdb.Open(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(fixture.password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: fixture.username, PasswordHash: string(passwordHash)}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	fixture.archiveDirectory = filepath.Join("data", fixture.username, "历史迁移书")
	fixture.archivePath = filepath.Join(fixture.cfg.LibraryDir, fixture.archiveDirectory, "legacy.txt")
	fixture.archiveSource = "第一章\n应从历史归档恢复的正文。\n"
	if err := os.MkdirAll(filepath.Dir(fixture.archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.archivePath, []byte(fixture.archiveSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(fixture.archivePath), "chapters.json"), []byte("[{\"title\":\"第一章\",\"index\":0}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(fixture.archivePath), "bookSource.json"), []byte("[{\"name\":\"历史本地书\"}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fixture.hostSourcePath = filepath.Join(root, "retired-host", fixture.archiveDirectory, "legacy.txt")
	if err := os.MkdirAll(filepath.Dir(fixture.hostSourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.hostSourcePath, []byte("绝不能从宿主绝对路径读取的正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.hostCachePath = filepath.Join(root, "retired-host-cache", "chapter.txt")
	if err := os.MkdirAll(filepath.Dir(fixture.hostCachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.hostCachePath, []byte("绝不能从宿主绝对路径读取的缓存"), 0o644); err != nil {
		t.Fatal(err)
	}

	fixture.book = models.Book{
		UserID:       user.ID,
		SourceID:     0,
		Title:        "历史挂载 TXT",
		Author:       "迁移测试",
		URL:          "local://historical-volume",
		LibraryPath:  fixture.archiveDirectory,
		OriginalFile: fixture.hostSourcePath,
		TOCFile:      filepath.Join(fixture.archiveDirectory, "chapters.json"),
		SourceFile:   filepath.Join(fixture.archiveDirectory, "bookSource.json"),
		TOCRule:      `^第.+章.*$`,
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := database.Create(&fixture.book).Error; err != nil {
		t.Fatal(err)
	}
	fixture.chapter = models.Chapter{
		BookID:    fixture.book.ID,
		Index:     0,
		Title:     "第一章",
		URL:       fixture.book.URL + "/chapter_0",
		CachePath: fixture.hostCachePath,
	}
	if err := database.Create(&fixture.chapter).Error; err != nil {
		t.Fatal(err)
	}
	fixture.progress = models.ReadingProgress{UserID: user.ID, BookID: fixture.book.ID, ChapterID: fixture.chapter.ID, ChapterIndex: 0, Offset: 7, Percent: 0.25, ChapterTitle: "第一章"}
	if err := database.Create(&fixture.progress).Error; err != nil {
		t.Fatal(err)
	}
	fixture.bookmark = models.Bookmark{UserID: user.ID, BookID: fixture.book.ID, ChapterID: fixture.chapter.ID, ChapterIndex: 0, Offset: 4, Percent: 0.1, Title: "历史书签", Excerpt: "历史"}
	if err := database.Create(&fixture.bookmark).Error; err != nil {
		t.Fatal(err)
	}

	cacheArchiveDirectory := filepath.Join("data", fixture.username, "历史相对缓存书")
	fixture.cacheArchivePath = filepath.Join(fixture.cfg.LibraryDir, cacheArchiveDirectory, "legacy.txt")
	fixture.cacheArchiveBody = "第一章\narchive 回退正文，不应覆盖旧 cache。\n"
	if err := os.MkdirAll(filepath.Dir(fixture.cacheArchivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.cacheArchivePath, []byte(fixture.cacheArchiveBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(fixture.cacheArchivePath), "chapters.json"), []byte("[{\"title\":\"第一章\",\"index\":0}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(fixture.cacheArchivePath), "bookSource.json"), []byte("[{\"name\":\"历史相对缓存书\"}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.legacyCachePath = filepath.Join("legacy-cache", "chapter.txt")
	fixture.legacyCacheBody = "历史相对 cache 正文必须优先于 archive。"
	legacyCacheFile := filepath.Join(fixture.cfg.CacheDir, fixture.legacyCachePath)
	if err := os.MkdirAll(filepath.Dir(legacyCacheFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyCacheFile, []byte(fixture.legacyCacheBody), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.cacheBook = models.Book{
		UserID:       user.ID,
		SourceID:     0,
		Title:        "历史相对缓存书",
		Author:       "迁移测试",
		URL:          "local://historical-relative-cache",
		LibraryPath:  cacheArchiveDirectory,
		OriginalFile: filepath.Join(cacheArchiveDirectory, "legacy.txt"),
		TOCFile:      filepath.Join(cacheArchiveDirectory, "chapters.json"),
		SourceFile:   filepath.Join(cacheArchiveDirectory, "bookSource.json"),
		TOCRule:      `^第.+章.*$`,
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := database.Create(&fixture.cacheBook).Error; err != nil {
		t.Fatal(err)
	}
	fixture.cacheChapter = models.Chapter{
		BookID:    fixture.cacheBook.ID,
		Index:     0,
		Title:     "第一章",
		URL:       fixture.cacheBook.URL + "/chapter_0",
		CachePath: fixture.legacyCachePath,
	}
	if err := database.Create(&fixture.cacheChapter).Error; err != nil {
		t.Fatal(err)
	}

	otherPasswordHash, err := bcrypt.GenerateFromPassword([]byte(fixture.otherPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	otherUser := models.User{Username: fixture.otherUsername, PasswordHash: string(otherPasswordHash)}
	if err := database.Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	otherArchiveDirectory := filepath.Join("data", fixture.otherUsername, "历史隔离书")
	fixture.otherArchivePath = filepath.Join(fixture.cfg.LibraryDir, otherArchiveDirectory, "legacy.txt")
	fixture.otherArchiveBody = "第一章\n用户 B 的旧卷正文必须保持私有。\n"
	if err := os.MkdirAll(filepath.Dir(fixture.otherArchivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.otherArchivePath, []byte(fixture.otherArchiveBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(fixture.otherArchivePath), "chapters.json"), []byte("[{\"title\":\"第一章\",\"index\":0}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(fixture.otherArchivePath), "bookSource.json"), []byte("[{\"name\":\"历史隔离书\"}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.otherBook = models.Book{
		UserID:       otherUser.ID,
		SourceID:     0,
		Title:        "历史隔离书",
		Author:       "迁移测试",
		URL:          "local://historical-other-user",
		LibraryPath:  otherArchiveDirectory,
		OriginalFile: filepath.Join(otherArchiveDirectory, "legacy.txt"),
		TOCFile:      filepath.Join(otherArchiveDirectory, "chapters.json"),
		SourceFile:   filepath.Join(otherArchiveDirectory, "bookSource.json"),
		TOCRule:      `^第.+章.*$`,
		LastChapter:  "第一章",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := database.Create(&fixture.otherBook).Error; err != nil {
		t.Fatal(err)
	}
	fixture.otherChapter = models.Chapter{
		BookID:    fixture.otherBook.ID,
		Index:     0,
		Title:     "第一章",
		URL:       fixture.otherBook.URL + "/chapter_0",
		CachePath: filepath.Join("content", "missing.txt"),
	}
	if err := database.Create(&fixture.otherChapter).Error; err != nil {
		t.Fatal(err)
	}

	// Persist the older schema before the test performs the same migration order
	// as a process opening a mounted volume after an upgrade.
	for _, field := range []string{"ResourcePath", "ResourceFragment", "ResourceEndFragment", "Variable"} {
		if err := database.Migrator().DropColumn(&models.Chapter{}, field); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrator().DropColumn(&models.Book{}, "Variable"); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = readerdb.Open(fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	if err := readerdb.MigrateLocalBookCache(database, fixture.cfg); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.book, fixture.book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.chapter, fixture.chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.progress, fixture.progress.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.bookmark, fixture.bookmark.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.cacheBook, fixture.cacheBook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.cacheChapter, fixture.cacheChapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.otherBook, fixture.otherBook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&fixture.otherChapter, fixture.otherChapter.ID).Error; err != nil {
		t.Fatal(err)
	}

	hub := readersync.NewHub()
	sched := scheduler.New(database, time.Second)
	backupSvc := backup.New(database, filepath.Join(fixture.cfg.DataDir, "webdav"))
	router := gin.New()
	server := RegisterRoutes(router, fixture.cfg, database, hub, sched, backupSvc)
	return router, server, fixture
}

func historicalVolumeAuth(t *testing.T, router *gin.Engine, fixture historicalLocalVolume) string {
	t.Helper()
	return historicalVolumeAuthAs(t, router, fixture.username, fixture.password)
}

func historicalVolumeAuthAs(t *testing.T, router *gin.Engine, username, password string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("historical-volume login: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Token == "" {
		t.Fatalf("historical-volume login response = %s, err=%v", response.Body.String(), err)
	}
	return "Bearer " + payload.Token
}

func TestHistoricalMountedVolumeMigratesRowsAndNeverReadsRetiredHostPaths(t *testing.T) {
	router, server, fixture := setupHistoricalLocalVolume(t)

	if !server.db.Migrator().HasColumn(&models.Chapter{}, "ResourcePath") ||
		!server.db.Migrator().HasColumn(&models.Chapter{}, "ResourceFragment") ||
		!server.db.Migrator().HasColumn(&models.Chapter{}, "ResourceEndFragment") {
		t.Fatal("old mounted database did not receive additive EPUB columns")
	}
	if fixture.progress.Offset != 7 || fixture.bookmark.Title != "历史书签" || fixture.chapter.ResourcePath != "" {
		t.Fatalf("old mounted rows changed during migration: chapter=%+v progress=%+v bookmark=%+v", fixture.chapter, fixture.progress, fixture.bookmark)
	}

	path, ok := server.localBookSourcePath(fixture.book)
	archiveInfo, archiveErr := os.Stat(fixture.archivePath)
	resolvedInfo, resolvedErr := os.Stat(path)
	if !ok || archiveErr != nil || resolvedErr != nil || !os.SameFile(archiveInfo, resolvedInfo) {
		t.Fatalf("historical absolute OriginalFile must rebase to its mounted archive, got %q ok=%v want %q", path, ok, fixture.archivePath)
	}
	for _, candidate := range server.chapterCacheCandidates(fixture.book, fixture.chapter.CachePath) {
		if candidate == fixture.hostCachePath {
			t.Fatalf("historical absolute CachePath must never be a readable host candidate: %q", candidate)
		}
	}

	auth := historicalVolumeAuth(t, router, fixture)
	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/chapters/0/content", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("historical mounted chapter: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "宿主绝对路径") || !strings.Contains(response.Body.String(), "应从历史归档恢复") {
		t.Fatalf("historical mounted chapter read an unsafe host artifact: %s", response.Body.String())
	}
	if archive, err := os.ReadFile(fixture.archivePath); err != nil || string(archive) != fixture.archiveSource {
		t.Fatalf("migration/recovery changed original archive: content=%q err=%v", string(archive), err)
	}
}

func TestHistoricalMountedVolumeRemainsPrivateAfterMigration(t *testing.T) {
	router, _, fixture := setupHistoricalLocalVolume(t)
	auth := historicalVolumeAuthAs(t, router, fixture.otherUsername, fixture.otherPassword)
	chapterRequest := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/chapters/0/content", nil)
	chapterRequest.Header.Set("Authorization", auth)
	chapterResponse := httptest.NewRecorder()
	router.ServeHTTP(chapterResponse, chapterRequest)
	if chapterResponse.Code != http.StatusNotFound {
		t.Fatalf("other user read historical local book: expected 404, got %d: %s", chapterResponse.Code, chapterResponse.Body.String())
	}
}

func TestHistoricalMountedVolumeExistingUsersStayIsolated(t *testing.T) {
	router, server, fixture := setupHistoricalLocalVolume(t)
	ownerAuth := historicalVolumeAuth(t, router, fixture)
	otherAuth := historicalVolumeAuthAs(t, router, fixture.otherUsername, fixture.otherPassword)

	for _, test := range []struct {
		name         string
		auth         string
		ownTitle     string
		foreignTitle string
	}{
		{name: "owner", auth: ownerAuth, ownTitle: fixture.book.Title, foreignTitle: fixture.otherBook.Title},
		{name: "other", auth: otherAuth, ownTitle: fixture.otherBook.Title, foreignTitle: fixture.book.Title},
	} {
		t.Run(test.name+" list", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/books", nil)
			request.Header.Set("Authorization", test.auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.ownTitle) || strings.Contains(response.Body.String(), test.foreignTitle) {
				t.Fatalf("historical user list leaked or omitted books: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	for _, test := range []struct {
		name      string
		auth      string
		foreignID uint
	}{
		{name: "owner cannot read or refresh other", auth: ownerAuth, foreignID: fixture.otherBook.ID},
		{name: "other cannot read or refresh owner", auth: otherAuth, foreignID: fixture.book.ID},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, methodPath := range []struct {
				method string
				path   string
			}{
				{method: http.MethodGet, path: "/api/books/" + strconv.FormatUint(uint64(test.foreignID), 10) + "/chapters/0/content"},
				{method: http.MethodPost, path: "/api/books/" + strconv.FormatUint(uint64(test.foreignID), 10) + "/refresh-local"},
			} {
				request := httptest.NewRequest(methodPath.method, methodPath.path, nil)
				request.Header.Set("Authorization", test.auth)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "legacy.txt") {
					t.Fatalf("cross-user %s %s: expected safe 404, got %d: %s", methodPath.method, methodPath.path, response.Code, response.Body.String())
				}
			}
		})
	}

	otherRead := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(fixture.otherBook.ID), 10)+"/chapters/0/content", nil)
	otherRead.Header.Set("Authorization", otherAuth)
	otherReadResponse := httptest.NewRecorder()
	router.ServeHTTP(otherReadResponse, otherRead)
	if otherReadResponse.Code != http.StatusOK || !strings.Contains(otherReadResponse.Body.String(), "用户 B 的旧卷正文必须保持私有。") {
		t.Fatalf("other user's historical archive was not readable: status=%d body=%s", otherReadResponse.Code, otherReadResponse.Body.String())
	}
	if err := server.db.First(&fixture.otherChapter, fixture.otherChapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	otherCachePath := fixture.otherChapter.CachePath
	otherArchive, err := os.ReadFile(fixture.otherArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	otherArchiveHash := sha256.Sum256(otherArchive)

	ownerRefresh := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/refresh-local", nil)
	ownerRefresh.Header.Set("Authorization", ownerAuth)
	ownerRefreshResponse := httptest.NewRecorder()
	router.ServeHTTP(ownerRefreshResponse, ownerRefresh)
	if ownerRefreshResponse.Code != http.StatusOK {
		t.Fatalf("owner historical refresh: expected 200, got %d: %s", ownerRefreshResponse.Code, ownerRefreshResponse.Body.String())
	}
	if err := server.db.First(&fixture.otherChapter, fixture.otherChapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fixture.otherChapter.CachePath != otherCachePath {
		t.Fatalf("owner refresh changed other user's cache path from %q to %q", otherCachePath, fixture.otherChapter.CachePath)
	}
	otherArchiveAfter, err := os.ReadFile(fixture.otherArchivePath)
	if err != nil || sha256.Sum256(otherArchiveAfter) != otherArchiveHash {
		t.Fatalf("owner refresh changed other user's archive: err=%v", err)
	}
}

func TestHistoricalMountedVolumeMigratesRelativeCacheOnce(t *testing.T) {
	router, server, fixture := setupHistoricalLocalVolume(t)
	expectedCachePath := filepath.Join("content", fixture.legacyCachePath)
	if fixture.cacheChapter.CachePath != expectedCachePath {
		t.Fatalf("historical relative cache was not normalized: got %q want %q", fixture.cacheChapter.CachePath, expectedCachePath)
	}
	migratedCachePath := filepath.Join(filepath.Dir(fixture.cacheArchivePath), expectedCachePath)
	if content, err := os.ReadFile(migratedCachePath); err != nil || string(content) != fixture.legacyCacheBody {
		t.Fatalf("historical relative cache was not copied byte-for-byte: content=%q err=%v", string(content), err)
	}
	if _, err := os.Stat(filepath.Join(fixture.cfg.CacheDir, fixture.legacyCachePath)); !os.IsNotExist(err) {
		t.Fatalf("historical cache source remained after successful migration: %v", err)
	}
	if err := readerdb.MigrateLocalBookCache(server.db, fixture.cfg); err != nil {
		t.Fatal(err)
	}
	if err := server.db.First(&fixture.cacheChapter, fixture.cacheChapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fixture.cacheChapter.CachePath != expectedCachePath {
		t.Fatalf("second migration changed cache path to %q", fixture.cacheChapter.CachePath)
	}

	auth := historicalVolumeAuth(t, router, fixture)
	request := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(fixture.cacheBook.ID), 10)+"/chapters/0/content", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), fixture.legacyCacheBody) {
		t.Fatalf("migrated historical cache was not served: status=%d body=%s", response.Code, response.Body.String())
	}
	if archive, err := os.ReadFile(fixture.cacheArchivePath); err != nil || string(archive) != fixture.cacheArchiveBody {
		t.Fatalf("relative-cache migration changed original archive: content=%q err=%v", string(archive), err)
	}
}

func TestHistoricalMountedVolumeRebuildsEPUBUMDAndCBZArchives(t *testing.T) {
	router, server, fixture := setupHistoricalLocalVolume(t)
	auth := historicalVolumeAuth(t, router, fixture)

	tests := []struct {
		name        string
		filename    string
		archive     []byte
		tocRule     string
		wantFormat  string
		wantContent string
	}{
		{
			name:       "EPUB",
			filename:   "legacy.epub",
			archive:    testEPUBArchive(t),
			tocRule:    "toc",
			wantFormat: "epub",
		},
		{
			name:        "reader-dev UMD",
			filename:    "legacy.umd",
			archive:     readerDevUMDImportFixture(t),
			wantContent: "第一段",
		},
		{
			name:       "CBZ",
			filename:   "legacy.cbz",
			archive:    testCBZArchive(t, "old-volume-first-page"),
			wantFormat: "cbz",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			book, archivePath := addHistoricalFormatArchive(t, server, fixture, test.filename, test.archive, test.tocRule)
			archiveHash := sha256.Sum256(test.archive)

			contentRequest := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/chapters/0/content", nil)
			contentRequest.Header.Set("Authorization", auth)
			contentResponse := httptest.NewRecorder()
			router.ServeHTTP(contentResponse, contentRequest)
			if contentResponse.Code != http.StatusOK {
				t.Fatalf("read %s historical archive: expected 200, got %d: %s", test.name, contentResponse.Code, contentResponse.Body.String())
			}
			var content struct {
				Content     string `json:"content"`
				Format      string `json:"format"`
				ResourceURL string `json:"resourceUrl"`
			}
			if err := json.Unmarshal(contentResponse.Body.Bytes(), &content); err != nil {
				t.Fatal(err)
			}
			if test.wantFormat != "" && (content.Format != test.wantFormat || content.ResourceURL == "") {
				t.Fatalf("historical %s content response = %+v", test.name, content)
			}
			if test.wantContent != "" && !strings.Contains(content.Content, test.wantContent) {
				t.Fatalf("historical %s content = %q, want %q", test.name, content.Content, test.wantContent)
			}

			refreshRequest := httptest.NewRequest(http.MethodPost, "/api/books/"+strconv.FormatUint(uint64(book.ID), 10)+"/refresh-local", nil)
			refreshRequest.Header.Set("Authorization", auth)
			refreshResponse := httptest.NewRecorder()
			router.ServeHTTP(refreshResponse, refreshRequest)
			if refreshResponse.Code != http.StatusOK {
				t.Fatalf("refresh %s historical archive: expected 200, got %d: %s", test.name, refreshResponse.Code, refreshResponse.Body.String())
			}
			archiveAfter, err := os.ReadFile(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(archiveAfter) != archiveHash {
				t.Fatalf("refresh %s rewrote the old original archive", test.name)
			}
		})
	}
}

func addHistoricalFormatArchive(t *testing.T, server *Server, fixture historicalLocalVolume, filename string, archive []byte, tocRule string) (models.Book, string) {
	t.Helper()
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	libraryPath := filepath.Join("data", fixture.username, "old-volume-"+stem)
	archivePath := filepath.Join(server.cfg.LibraryDir, libraryPath, filename)
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(archivePath), "chapters.json"), []byte("[{\"title\":\"旧目录\",\"index\":0}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(archivePath), "bookSource.json"), []byte("[{\"name\":\"旧卷格式夹具\"}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := models.Book{
		UserID:       fixture.book.UserID,
		SourceID:     0,
		Title:        "旧卷 " + filename,
		Author:       "格式夹具",
		URL:          "local://old-volume-" + stem,
		LibraryPath:  libraryPath,
		OriginalFile: filepath.Join("/retired-host", libraryPath, filename),
		TOCFile:      filepath.Join(libraryPath, "chapters.json"),
		SourceFile:   filepath.Join(libraryPath, "bookSource.json"),
		TOCRule:      tocRule,
		LastChapter:  "旧目录",
		ChapterCount: 1,
		CanUpdate:    true,
	}
	if err := server.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "旧目录", URL: book.URL + "/chapter_0", CachePath: filepath.Join("content", "missing.txt")}
	if err := server.db.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return book, archivePath
}
