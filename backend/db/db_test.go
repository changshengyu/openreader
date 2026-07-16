package db

import (
	"os"
	"path/filepath"
	"testing"

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
	if !filepath.IsAbs(updated.CachePath) {
		t.Fatalf("expected absolute library path, got %q", updated.CachePath)
	}
	if _, err := os.Stat(updated.CachePath); err != nil {
		t.Fatalf("expected migrated content file, stat err=%v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old cache file removed, stat err=%v", err)
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
