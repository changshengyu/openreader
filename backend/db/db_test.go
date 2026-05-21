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
