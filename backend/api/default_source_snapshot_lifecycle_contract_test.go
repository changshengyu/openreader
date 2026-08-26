package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
	"openreader/backend/services/backup"
	"openreader/backend/services/scheduler"
	readersync "openreader/backend/sync"
)

const defaultSourceSnapshotLimit = 16 << 20

func TestDefaultSourceLegacyFileRejectsUnsafeAndOversizedObjects(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		dataDir := t.TempDir()
		router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
			cfg.DataDir = dataDir
		})
		admin := registerSourceContractAccount(t, router, "defaultsymlinkadmin")

		outside := filepath.Join(t.TempDir(), "outside-default.json")
		writeDefaultSourceFixture(t, outside, "outside", "https://outside-default.example")
		if err := os.Symlink(outside, server.defaultBookSourcesPath()); err != nil {
			t.Fatal(err)
		}

		response := defaultSourceStatusRequest(router, admin.Auth)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("symlink default status = %d %s, want safe 500", response.Code, response.Body.String())
		}
		assertNoDefaultSourceState(t, server.db)
		if strings.Contains(response.Body.String(), outside) || strings.Contains(response.Body.String(), dataDir) {
			t.Fatalf("symlink error leaked host path: %s", response.Body.String())
		}
	})

	t.Run("directory", func(t *testing.T) {
		dataDir := t.TempDir()
		router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
			cfg.DataDir = dataDir
		})
		admin := registerSourceContractAccount(t, router, "defaultdirectoryadmin")
		if err := os.Mkdir(server.defaultBookSourcesPath(), 0o755); err != nil {
			t.Fatal(err)
		}

		response := defaultSourceStatusRequest(router, admin.Auth)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("directory default status = %d %s, want safe 500", response.Code, response.Body.String())
		}
		assertNoDefaultSourceState(t, server.db)
		if strings.Contains(response.Body.String(), dataDir) || strings.Contains(response.Body.String(), "defaultBookSources.json") {
			t.Fatalf("directory error leaked filesystem detail: %s", response.Body.String())
		}
	})

	t.Run("actual-read-overflow", func(t *testing.T) {
		dataDir := t.TempDir()
		router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
			cfg.DataDir = dataDir
		})
		admin := registerSourceContractAccount(t, router, "defaultoverflowadmin")
		payload := `[{"bookSourceName":"` + strings.Repeat("x", defaultSourceSnapshotLimit) +
			`","bookSourceUrl":"https://oversized-default.example","enabled":true}]`
		if err := os.WriteFile(server.defaultBookSourcesPath(), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}

		response := defaultSourceStatusRequest(router, admin.Auth)
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("oversized default status = %d %s, want safe 500", response.Code, response.Body.String())
		}
		assertNoDefaultSourceState(t, server.db)
	})
}

func TestCanceledDefaultSourceInitializationHasNoSideEffects(t *testing.T) {
	dataDir := t.TempDir()
	_, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.DataDir = dataDir
	})
	writeDefaultSourceFixture(t, server.defaultBookSourcesPath(), "cancelled", "https://cancelled-default.example")

	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/sources/default", nil).WithContext(requestContext)
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = request
	server.defaultSourcesStatus(ginContext)

	assertNoDefaultSourceState(t, server.db)
}

func TestConcurrentDefaultSourceSavesPublishOneMatchingSnapshot(t *testing.T) {
	router, server := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "defaultconcurrentalice")
	bob := registerSourceContractAccount(t, router, "defaultconcurrentbob")
	createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"Alice concurrent default",
		"baseUrl":"https://alice-concurrent-default.example",
		"enabled":true
	}`)
	createSourceThroughAPI(t, router, bob.Auth, `{
		"name":"Bob concurrent default",
		"baseUrl":"https://bob-concurrent-default.example",
		"enabled":true
	}`)

	blocked := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	const callbackName = "test:block-first-default-source-save"
	var queryCount atomic.Int32
	if err := server.db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if queryCount.Add(1) != 3 {
			return
		}
		close(blocked)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.db.Callback().Query().Remove(callbackName) })

	type saveResult struct {
		name  string
		count int
		err   error
	}
	results := make(chan saveResult, 2)
	go func() {
		count, err := server.saveDefaultSourceSnapshot(alice.ID, true)
		results <- saveResult{name: "alice", count: count, err: err}
	}()
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("alice save did not reach the deterministic interleave point")
	}

	go func() {
		count, err := server.saveDefaultSourceSnapshot(bob.ID, true)
		results <- saveResult{name: "bob", count: count, err: err}
	}()
	completed := make([]saveResult, 0, 2)
	select {
	case result := <-results:
		completed = append(completed, result)
	case <-time.After(100 * time.Millisecond):
		// A serialized implementation keeps Bob outside the lifecycle until Alice
		// finishes. The old implementation lets Bob publish a conflicting mirror.
	}

	releaseOnce.Do(func() { close(release) })
	for len(completed) < 2 {
		select {
		case result := <-results:
			completed = append(completed, result)
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent default saves did not finish after release")
		}
	}
	for _, result := range completed {
		if result.err != nil || result.count != 1 {
			t.Fatalf("%s save = count:%d err:%v", result.name, result.count, result.err)
		}
	}

	defaults, err := server.bookSources.ListActive(0)
	if err != nil || len(defaults) != 1 {
		t.Fatalf("SQLite defaults = %+v err=%v", defaults, err)
	}
	mirror := readDefaultSourceFixture(t, server.defaultBookSourcesPath())
	if len(mirror) != 1 || mirror[0].BaseURL != defaults[0].BaseURL {
		t.Fatalf("default generations diverged: SQLite=%+v mirror=%+v", defaults, mirror)
	}
}

func TestDirectLegacyUpgradePreservesExplicitDefaultSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	cfg := config.Config{
		DataDir:       filepath.Join(root, "data"),
		CacheDir:      filepath.Join(root, "cache"),
		LibraryDir:    filepath.Join(root, "library"),
		DatabasePath:  filepath.Join(root, "data", "openreader.db"),
		JWTSecret:     "legacy-default-secret",
		LocalStoreDir: filepath.Join(root, "library", "localStore"),
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.BookSource{}, &models.Book{}, &models.SourceFailure{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "legacy-default-user", PasswordHash: "hash", Role: "user"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	legacyGlobal := []models.BookSource{
		{Name: "Legacy global A", BaseURL: "https://legacy-global-a.example", Enabled: true},
		{Name: "Legacy global B", BaseURL: "https://legacy-global-b.example", Enabled: true},
	}
	if err := database.Create(&legacyGlobal).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDefaultSourceFixture(t, filepath.Join(cfg.DataDir, "defaultBookSources.json"), "Explicit legacy default", "https://explicit-legacy-default.example")
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	hub := readersync.NewHub()
	sched := scheduler.New(database, time.Hour)
	backupSvc := backup.New(database, filepath.Join(cfg.DataDir, "webdav"), cfg)
	server := RegisterRoutes(gin.New(), cfg, database, hub, sched, backupSvc)
	defaults, err := server.bookSources.ListActive(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults) != 1 || defaults[0].BaseURL != "https://explicit-legacy-default.example" {
		t.Fatalf("direct legacy default = %+v, want explicit file snapshot", defaults)
	}
	userSources, err := server.bookSources.ListExistingActive(user.ID)
	if err != nil || len(userSources) != len(legacyGlobal) {
		t.Fatalf("legacy user associations changed: %+v err=%v", userSources, err)
	}
	for index := range legacyGlobal {
		if userSources[index].ID != legacyGlobal[index].ID {
			t.Fatalf("legacy source IDs changed: got=%+v want=%+v", userSources, legacyGlobal)
		}
	}
}

func TestConfiguredSQLiteDefaultCanonicalizesStaleMirror(t *testing.T) {
	dataDir := t.TempDir()
	router, server := setupTestServerWithConfig(t, func(cfg *config.Config) {
		cfg.DataDir = dataDir
	})
	admin := registerSourceContractAccount(t, router, "defaultcanonicaladmin")
	current := createSourceThroughAPI(t, router, admin.Auth, `{
		"name":"SQLite authoritative default",
		"baseUrl":"https://sqlite-authoritative-default.example",
		"enabled":true
	}`)
	if _, err := server.bookSources.SaveDefaultFromExistingUser(admin.ID); err != nil {
		t.Fatal(err)
	}
	writeDefaultSourceFixture(t, server.defaultBookSourcesPath(), "Stale mirror", "https://stale-default-mirror.example")

	restarted := RegisterRoutes(
		gin.New(),
		server.cfg,
		server.db,
		readersync.NewHub(),
		scheduler.New(server.db, time.Hour),
		backup.New(server.db, filepath.Join(server.cfg.DataDir, "webdav"), server.cfg),
	)
	mirror := readDefaultSourceFixture(t, restarted.defaultBookSourcesPath())
	if len(mirror) != 1 || mirror[0].BaseURL != current.BaseURL {
		t.Fatalf("restart mirror = %+v, want SQLite source %q", mirror, current.BaseURL)
	}
	defaults, err := restarted.bookSources.ListActive(0)
	if err != nil || len(defaults) != 1 || defaults[0].ID != current.ID {
		t.Fatalf("restart rewrote SQLite defaults: %+v err=%v", defaults, err)
	}
}

func TestDirectLegacyUpgradePreservesExplicitEmptyDefaultSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	cfg := config.Config{
		DataDir:       filepath.Join(root, "data"),
		CacheDir:      filepath.Join(root, "cache"),
		LibraryDir:    filepath.Join(root, "library"),
		DatabasePath:  filepath.Join(root, "data", "openreader.db"),
		JWTSecret:     "legacy-empty-default-secret",
		LocalStoreDir: filepath.Join(root, "library", "localStore"),
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}, &models.BookSource{}, &models.Book{}, &models.SourceFailure{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "legacy-empty-default-user", PasswordHash: "hash", Role: "user"}
	legacy := models.BookSource{Name: "Legacy user source", BaseURL: "https://legacy-empty-user.example", Enabled: true}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDir, "defaultBookSources.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	server := RegisterRoutes(
		gin.New(), cfg, database, readersync.NewHub(), scheduler.New(database, time.Hour),
		backup.New(database, filepath.Join(cfg.DataDir, "webdav"), cfg),
	)
	configured, count, err := server.bookSources.DefaultStatus()
	if err != nil || !configured || count != 0 {
		t.Fatalf("explicit empty default = configured:%v count:%d err:%v", configured, count, err)
	}
	userSources, err := server.bookSources.ListExistingActive(user.ID)
	if err != nil || len(userSources) != 1 || userSources[0].ID != legacy.ID {
		t.Fatalf("explicit empty migration changed user sources: %+v err=%v", userSources, err)
	}
}

func TestDefaultSourceDatabaseFailureKeepsPreviousMirror(t *testing.T) {
	router, server := setupTestServer(t)
	alice := registerSourceContractAccount(t, router, "defaultrollbackalice")
	bob := registerSourceContractAccount(t, router, "defaultrollbackbob")
	createSourceThroughAPI(t, router, alice.Auth, `{
		"name":"Previous default",
		"baseUrl":"https://previous-default.example",
		"enabled":true
	}`)
	bobSource := createSourceThroughAPI(t, router, bob.Auth, `{
		"name":"Rejected default",
		"baseUrl":"https://rejected-default.example",
		"enabled":true
	}`)
	if _, err := server.saveDefaultSourceSnapshot(alice.ID, true); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(server.defaultBookSourcesPath())
	if err != nil {
		t.Fatal(err)
	}
	trigger := `CREATE TRIGGER reject_default_snapshot
		BEFORE INSERT ON user_book_sources
		WHEN NEW.user_id = 0 AND NEW.source_id = ` + strconv.FormatUint(uint64(bobSource.ID), 10) + `
		BEGIN
			SELECT RAISE(ABORT, 'rejected default snapshot');
		END`
	if err := server.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := server.saveDefaultSourceSnapshot(bob.ID, true); err == nil {
		t.Fatal("database-rejected default save unexpectedly succeeded")
	}

	defaults, err := server.bookSources.ListExistingActive(0)
	if err != nil || len(defaults) != 1 || defaults[0].BaseURL != "https://previous-default.example" {
		t.Fatalf("database failure changed defaults: %+v err=%v", defaults, err)
	}
	current, err := os.ReadFile(server.defaultBookSourcesPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(previous) {
		t.Fatalf("database failure changed mirror: before=%s after=%s", previous, current)
	}
}

func defaultSourceStatusRequest(router *gin.Engine, auth string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/sources/default", nil)
	request.Header.Set("Authorization", auth)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func writeDefaultSourceFixture(t *testing.T, path, name, sourceURL string) {
	t.Helper()
	payload := []map[string]any{{
		"bookSourceName": name,
		"bookSourceUrl":  sourceURL,
		"enabled":        true,
	}}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readDefaultSourceFixture(t *testing.T, path string) []models.BookSource {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := decodeBookSources(data)
	if err != nil {
		t.Fatalf("decode default mirror: %v", err)
	}
	return sources
}

func assertNoDefaultSourceState(t *testing.T, database *gorm.DB) {
	t.Helper()
	var namespaces int64
	if err := database.Model(&models.BookSourceNamespace{}).Where("user_id = ?", 0).Count(&namespaces).Error; err != nil {
		t.Fatal(err)
	}
	var associations int64
	if err := database.Model(&models.UserBookSource{}).Where("user_id = ?", 0).Count(&associations).Error; err != nil {
		t.Fatal(err)
	}
	if namespaces != 0 || associations != 0 {
		t.Fatalf("unsafe legacy file created default state: namespaces=%d associations=%d", namespaces, associations)
	}
}
