package localbook

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/engine"
	"openreader/backend/models"
)

func TestImporterPreviewAllowsExplicitTXTTOCRuleWithNoMatches(t *testing.T) {
	preview, err := (Importer{}).Preview(ImportRequest{
		FileName:  "规则不匹配.txt",
		Extension: ".txt",
		Data:      []byte("这是正文，但不包含自定义目录。"),
		TOCRule:   `^== .+ ==$`,
	})
	if err != nil {
		t.Fatalf("explicit no-match TOC preview error = %v", err)
	}
	if preview.Title != "规则不匹配" || preview.ChapterCount != 0 || len(preview.Chapters) != 0 {
		t.Fatalf("explicit no-match TOC preview = %+v, want a normal empty catalog", preview)
	}
}

func TestImporterRejectsOversizedLocalTOCRuleBeforeRegexCompilation(t *testing.T) {
	_, err := (Importer{}).Preview(ImportRequest{
		FileName:  "oversized-rule.txt",
		Extension: ".txt",
		Data:      []byte("第一章 正文"),
		TOCRule:   strings.Repeat("a", maxLocalBookTOCRuleBytes+1),
	})
	if !errors.Is(err, ErrParseFailed) {
		t.Fatalf("oversized TOC rule error = %v, want ErrParseFailed", err)
	}
}

func TestImporterPreparedPreviewIsTheConfirmedChapterSource(t *testing.T) {
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
	user := models.User{Username: "prepared-import", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	request := ImportRequest{
		UserID:    user.ID,
		UserName:  user.Username,
		FileName:  "prepared.txt",
		Extension: ".txt",
		Data:      []byte("第一章 原目录\n原正文"),
		TOCRule:   `^第.+章.*$`,
	}
	importer := NewImporter(cfg, database)
	preview, prepared, err := importer.Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ChapterCount != 1 || len(prepared.Book.Chapters) != 1 {
		t.Fatalf("prepared preview = %+v / %+v", preview, prepared)
	}
	prepared.Book.Chapters[0].Title = "快照章节"
	prepared.Book.Chapters[0].Content = "快照正文"

	book, err := importer.ImportPrepared(request, prepared)
	if err != nil {
		t.Fatal(err)
	}
	var chapter models.Chapter
	if err := database.Where("book_id = ?", book.ID).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	if chapter.Title != "快照章节" {
		t.Fatalf("confirmed chapter title = %q, want prepared snapshot", chapter.Title)
	}
	content, err := os.ReadFile(filepath.Join(cfg.LibraryDir, book.LibraryPath, chapter.CachePath))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "快照正文" {
		t.Fatalf("confirmed chapter content = %q, want prepared snapshot", content)
	}
}

func TestImporterRemovesNewArchiveWhenDatabaseCommitCannotStart(t *testing.T) {
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
	user := models.User{Username: "failed-import", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = NewImporter(cfg, database).Import(ImportRequest{
		UserID:    user.ID,
		UserName:  user.Username,
		FileName:  "failed.txt",
		Extension: ".txt",
		Data:      []byte("第一章 失败\n正文"),
	})
	if err == nil {
		t.Fatal("import with a closed database must fail")
	}
	userLibrary := filepath.Join(cfg.LibraryDir, "data", user.Username)
	entries, readErr := os.ReadDir(userLibrary)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed import left durable library entries: %+v", entries)
	}
}

func TestImporterImportsExplicitTXTTOCRuleWithNoMatches(t *testing.T) {
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
	user := models.User{Username: "empty-catalog", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	book, err := NewImporter(cfg, database).Import(ImportRequest{
		UserID:    user.ID,
		UserName:  user.Username,
		FileName:  "规则不匹配.txt",
		Extension: ".txt",
		Data:      []byte("这是正文，但不包含自定义目录。"),
		TOCRule:   `^== .+ ==$`,
	})
	if err != nil {
		t.Fatalf("explicit no-match TOC import error = %v", err)
	}
	if book.ChapterCount != 0 || book.LastChapter != "" || book.TOCRule != `^== .+ ==$` {
		t.Fatalf("empty-catalog book = %+v", book)
	}
	var chapterCount int64
	if err := database.Model(&models.Chapter{}).Where("book_id = ?", book.ID).Count(&chapterCount).Error; err != nil {
		t.Fatal(err)
	}
	if chapterCount != 0 {
		t.Fatalf("empty-catalog import created %d chapters", chapterCount)
	}
	for _, relativePath := range []string{book.OriginalFile, book.SourceFile, book.TOCFile} {
		if _, err := os.Stat(filepath.Join(cfg.LibraryDir, relativePath)); err != nil {
			t.Fatalf("empty-catalog %s was not archived: %v", relativePath, err)
		}
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

func TestImporterRejectsArchiveLimitsBeforeCreatingBookOrLibraryArchive(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataDir:           filepath.Join(root, "data"),
		CacheDir:          filepath.Join(root, "cache"),
		LibraryDir:        filepath.Join(root, "library"),
		DatabasePath:      filepath.Join(root, "data", "openreader.db"),
		MaxArchiveEntries: 1,
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "limit-tester", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"001.jpg", "002.jpg"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = NewImporter(cfg, database).Import(ImportRequest{
		UserID: user.ID, UserName: user.Username, FileName: "limit.cbz", Extension: ".cbz", Data: archive.Bytes(),
	})
	if !errors.Is(err, ErrParseFailed) || !errors.Is(err, engine.ErrLocalBookParseLimit) {
		t.Fatalf("archive-limit import error = %v", err)
	}
	var books int64
	if err := database.Model(&models.Book{}).Where("user_id = ?", user.ID).Count(&books).Error; err != nil {
		t.Fatal(err)
	}
	if books != 0 {
		t.Fatalf("rejected archive must not create a book, got %d", books)
	}
	if _, err := os.Stat(cfg.LibraryDir); !os.IsNotExist(err) {
		t.Fatalf("rejected archive must not create a library archive, stat err=%v", err)
	}
}
