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
		username: "legacy_owner",
		password: "legacy-secret",
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

	hub := readersync.NewHub()
	sched := scheduler.New(database, time.Second)
	backupSvc := backup.New(database, filepath.Join(fixture.cfg.DataDir, "webdav"))
	router := gin.New()
	server := RegisterRoutes(router, fixture.cfg, database, hub, sched, backupSvc)
	return router, server, fixture
}

func historicalVolumeAuth(t *testing.T, router *gin.Engine, fixture historicalLocalVolume) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+fixture.username+`","password":"`+fixture.password+`"}`))
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
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"legacy_other","password":"other-secret"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("register second historical-volume user: expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &login); err != nil || login.Token == "" {
		t.Fatalf("second user response = %s, err=%v", response.Body.String(), err)
	}

	chapterRequest := httptest.NewRequest(http.MethodGet, "/api/books/"+strconv.FormatUint(uint64(fixture.book.ID), 10)+"/chapters/0/content", nil)
	chapterRequest.Header.Set("Authorization", "Bearer "+login.Token)
	chapterResponse := httptest.NewRecorder()
	router.ServeHTTP(chapterResponse, chapterRequest)
	if chapterResponse.Code != http.StatusNotFound {
		t.Fatalf("other user read historical local book: expected 404, got %d: %s", chapterResponse.Code, chapterResponse.Body.String())
	}
}
