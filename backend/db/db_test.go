package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestMigrateLocalBookCacheMovesLocalContentToLibrary(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
	}
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	cachePath := filepath.Join("aa", "chapter.txt")
	oldPath := filepath.Join(cfg.CacheDir, cachePath)
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("本地正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := models.Book{UserID: 1, Title: "本地书", LibraryPath: filepath.Join("data", "test", "book")}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: cachePath}
	if err := database.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	if err := MigrateLocalBookCache(database, cfg); err != nil {
		t.Fatal(err)
	}

	var updated models.Chapter
	if err := database.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	expectedCachePath := filepath.Join("content", cachePath)
	if updated.CachePath != expectedCachePath {
		t.Fatalf("expected portable cache path %q, got %q", expectedCachePath, updated.CachePath)
	}
	migratedPath := filepath.Join(cfg.LibraryDir, book.LibraryPath, expectedCachePath)
	if content, err := os.ReadFile(migratedPath); err != nil || string(content) != "本地正文" {
		t.Fatalf("expected byte-for-byte migrated content, content=%q err=%v", string(content), err)
	}
	if _, err := os.Stat(migratedPath); err != nil {
		t.Fatalf("expected migrated content file, stat err=%v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old cache file removed, stat err=%v", err)
	}
	if err := MigrateLocalBookCache(database, cfg); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CachePath != expectedCachePath {
		t.Fatalf("second migration changed portable cache path to %q", updated.CachePath)
	}
}

func TestMigrateLocalBookCacheSkipsUnsafeHistoricalCachePath(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
	}
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	// This file mimics a stale cache_path from a historical SQLite volume. It
	// resolves outside cache/ and must therefore never be copied or removed by
	// startup migration.
	unsafeCachePath := filepath.Join("..", "retired-host-cache.txt")
	hostPath := filepath.Join(root, "retired-host-cache.txt")
	if err := os.WriteFile(hostPath, []byte("must remain outside migration"), 0o644); err != nil {
		t.Fatal(err)
	}
	libraryPath := filepath.Join("data", "testuser", "unsafe-cache")
	book := models.Book{UserID: 1, SourceID: 0, Title: "unsafe cache", LibraryPath: libraryPath}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", CachePath: unsafeCachePath}
	if err := database.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	if err := MigrateLocalBookCache(database, cfg); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(hostPath); err != nil || string(content) != "must remain outside migration" {
		t.Fatalf("unsafe cache path was touched: content=%q err=%v", string(content), err)
	}
	if _, err := os.Stat(filepath.Join(cfg.LibraryDir, libraryPath, "retired-host-cache.txt")); !os.IsNotExist(err) {
		t.Fatalf("unsafe cache path was copied into library, stat err=%v", err)
	}
	var updated models.Chapter
	if err := database.First(&updated, chapter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.CachePath != unsafeCachePath {
		t.Fatalf("unsafe cache path was rewritten to %q", updated.CachePath)
	}
}

func TestAutoMigrateAddsEPUBResourcePathWithoutLosingChapters(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
	}
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: 1, Title: "旧 EPUB"}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", URL: "local://book/chapter_0", CachePath: "content/one.txt"}
	if err := database.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Migrator().DropColumn(&models.Chapter{}, "ResourcePath"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrator().DropColumn(&models.Chapter{}, "ResourceFragment"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrator().DropColumn(&models.Chapter{}, "ResourceEndFragment"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrator().DropColumn(&models.Chapter{}, "Variable"); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrator().DropColumn(&models.Book{}, "Variable"); err != nil {
		t.Fatal(err)
	}
	if database.Migrator().HasColumn(&models.Chapter{}, "ResourcePath") ||
		database.Migrator().HasColumn(&models.Chapter{}, "ResourceFragment") ||
		database.Migrator().HasColumn(&models.Chapter{}, "ResourceEndFragment") {
		t.Fatal("EPUB resource metadata columns should be absent in the legacy fixture")
	}
	if database.Migrator().HasColumn(&models.Book{}, "Variable") || database.Migrator().HasColumn(&models.Chapter{}, "Variable") {
		t.Fatal("variable columns should be absent in the legacy fixture")
	}

	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	if !database.Migrator().HasColumn(&models.Chapter{}, "ResourcePath") ||
		!database.Migrator().HasColumn(&models.Chapter{}, "ResourceFragment") ||
		!database.Migrator().HasColumn(&models.Chapter{}, "ResourceEndFragment") {
		t.Fatal("EPUB resource metadata columns were not added")
	}
	if !database.Migrator().HasColumn(&models.Book{}, "Variable") || !database.Migrator().HasColumn(&models.Chapter{}, "Variable") {
		t.Fatal("variable columns were not added")
	}
	var migratedBook models.Book
	var migratedChapter models.Chapter
	if err := database.First(&migratedBook, book.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where("book_id = ?", book.ID).First(&migratedChapter).Error; err != nil {
		t.Fatal(err)
	}
	if migratedBook.Title != "旧 EPUB" || migratedBook.Variable != "" {
		t.Fatalf("legacy book changed during migration: %+v", migratedBook)
	}
	if migratedChapter.Title != "第一章" || migratedChapter.CachePath != "content/one.txt" ||
		migratedChapter.ResourcePath != "" || migratedChapter.ResourceFragment != "" ||
		migratedChapter.ResourceEndFragment != "" || migratedChapter.Variable != "" {
		t.Fatalf("legacy chapter changed during migration: %+v", migratedChapter)
	}
}

func TestAutoMigrateAddsNullableWebDAVPermissionWithoutRewritingLegacyStorePermission(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{DatabasePath: filepath.Join(root, "data", "openreader.db")}
	database, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	if database.Migrator().HasColumn("users", "can_access_webdav") {
		if err := database.Exec("ALTER TABLE users DROP COLUMN can_access_webdav").Error; err != nil {
			t.Fatalf("prepare legacy users schema: %v", err)
		}
	}

	createdAt := time.Now().UTC()
	if err := database.Exec(`INSERT INTO users (username, password_hash, role, book_limit, source_limit, can_edit_sources, can_access_store, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy-user", "legacy-hash", "user", 0, 0, true, false, createdAt, createdAt, createdAt,
	).Error; err != nil {
		t.Fatalf("create legacy user schema row: %v", err)
	}
	if err := AutoMigrate(database); err != nil {
		t.Fatalf("migrate legacy users schema: %v", err)
	}
	if !database.Migrator().HasColumn("users", "can_access_webdav") {
		t.Fatal("WebDAV permission column was not added")
	}
	var webdavPermission sql.NullBool
	if err := database.Raw("SELECT can_access_webdav FROM users WHERE username = ?", "legacy-user").Scan(&webdavPermission).Error; err != nil {
		t.Fatalf("read migrated WebDAV permission: %v", err)
	}
	if webdavPermission.Valid {
		t.Fatalf("legacy user must preserve nullable WebDAV fallback, got %+v", webdavPermission)
	}
	var migrated models.User
	if err := database.Where("username = ?", "legacy-user").First(&migrated).Error; err != nil {
		t.Fatalf("load migrated user: %v", err)
	}
	if migrated.CanAccessStore {
		t.Fatalf("migration rewrote legacy local-store permission: %+v", migrated)
	}
}
