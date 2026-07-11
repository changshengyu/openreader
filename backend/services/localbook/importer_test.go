package localbook

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

func TestImporterPreviewRejectsExplicitTXTTOCRuleWithNoMatches(t *testing.T) {
	_, err := (Importer{}).Preview(ImportRequest{
		FileName:  "规则不匹配.txt",
		Extension: ".txt",
		Data:      []byte("这是正文，但不包含自定义目录。"),
		TOCRule:   `^== .+ ==$`,
	})
	if !errors.Is(err, ErrNoReadableChapters) {
		t.Fatalf("explicit no-match TOC rule error = %v, want %v", err, ErrNoReadableChapters)
	}
}

func TestImporterArchivesLocalBookByUserNamespace(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
	}

	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	user := models.User{Username: "tester", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	book, err := NewImporter(cfg, database).Import(ImportRequest{
		UserID:    user.ID,
		UserName:  user.Username,
		FileName:  "测试书.txt",
		Extension: ".txt",
		Data:      []byte("第一章 起\n这是第一章的内容。\n第二章 承\n这是第二章的内容。"),
	})
	if err != nil {
		t.Fatal(err)
	}

	wantDir := filepath.Join("data", "tester", "测试书_")
	if book.LibraryPath != wantDir {
		t.Fatalf("LibraryPath = %q, want %q", book.LibraryPath, wantDir)
	}

	for _, relativePath := range []string{book.OriginalFile, book.SourceFile, book.TOCFile} {
		if _, err := os.Stat(filepath.Join(cfg.LibraryDir, relativePath)); err != nil {
			t.Fatalf("%s was not created: %v", relativePath, err)
		}
	}

	var chapterCount int64
	if err := database.Model(&models.Chapter{}).Where("book_id = ?", book.ID).Count(&chapterCount).Error; err != nil {
		t.Fatal(err)
	}
	if chapterCount != 2 {
		t.Fatalf("chapter count = %d, want 2", chapterCount)
	}

	var chapter models.Chapter
	if err := database.Where("book_id = ?", book.ID).Order("`index` asc").First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(chapter.CachePath) {
		t.Fatalf("chapter cache path should be portable, got absolute path %q", chapter.CachePath)
	}
	if _, err := os.Stat(filepath.Join(cfg.LibraryDir, book.LibraryPath, chapter.CachePath)); err != nil {
		t.Fatalf("chapter content was not created at portable path %q: %v", chapter.CachePath, err)
	}
}

func TestImporterUsesCustomTxtTocRule(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
	}

	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}

	user := models.User{Username: "tester", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	book, err := NewImporter(cfg, database).Import(ImportRequest{
		UserID:    user.ID,
		UserName:  user.Username,
		FileName:  "规则书.txt",
		Extension: ".txt",
		Data:      []byte("== 第一节 ==\n第一节正文。\n== 第二节 ==\n第二节正文。"),
		TOCRule:   `^== .+ ==$`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if book.TOCRule != `^== .+ ==$` {
		t.Fatalf("TOCRule = %q", book.TOCRule)
	}

	var chapters []models.Chapter
	if err := database.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("chapter count = %d, want 2", len(chapters))
	}
	if chapters[0].Title != "== 第一节 ==" || chapters[1].Title != "== 第二节 ==" {
		t.Fatalf("unexpected chapters: %+v", chapters)
	}
}
