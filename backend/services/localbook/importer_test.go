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
	"openreader/backend/services/epubreader"
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

func TestImporterEPUBPreviewStoresCatalogueOnlyAndConfirmationPreparesReaderResources(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
		JWTSecret:    "epub-import-secret",
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "epub-catalogue", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{
		UserID:    user.ID,
		UserName:  user.Username,
		FileName:  "catalogue.epub",
		Extension: ".epub",
		Data:      localBookTestEPUB(t),
		TOCRule:   "spin+toc",
	}
	importer := NewImporter(cfg, database)
	preview, prepared, err := importer.Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ChapterCount != 2 || len(prepared.Book.Chapters) != 2 {
		t.Fatalf("EPUB preview = %+v / %+v", preview, prepared)
	}
	if !prepared.EPUBCatalogOnly {
		t.Fatal("EPUB preview snapshot must identify catalogue-only content")
	}
	for _, chapter := range prepared.Book.Chapters {
		if chapter.Content != "" {
			t.Fatalf("EPUB preview materialized body content: %+v", chapter)
		}
	}

	book, err := importer.ImportPrepared(request, prepared)
	if err != nil {
		t.Fatal(err)
	}
	var chapters []models.Chapter
	if err := database.Where("book_id = ?", book.ID).Order("`index` asc").Find(&chapters).Error; err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("confirmed chapter count = %d, want 2", len(chapters))
	}
	for _, chapter := range chapters {
		content, err := os.ReadFile(filepath.Join(cfg.LibraryDir, book.LibraryPath, chapter.CachePath))
		if err != nil {
			t.Fatalf("confirmed chapter %d cache: %v", chapter.Index, err)
		}
		if !strings.Contains(string(content), "正文") {
			t.Fatalf("confirmed chapter %d content = %q", chapter.Index, content)
		}
	}
	extractionParent := filepath.Join(cfg.LibraryDir, book.LibraryPath, ".epub-resources")
	entries, err := os.ReadDir(extractionParent)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("confirmed EPUB did not prepare one immutable resource tree: entries=%+v err=%v", entries, err)
	}
}

func TestImporterAcceptsLegacyFullContentEPUBPreparedSnapshot(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
		JWTSecret:    "legacy-epub-snapshot-secret",
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "legacy-epub-snapshot", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{
		UserID: user.ID, UserName: user.Username, FileName: "legacy.epub", Extension: ".epub", Data: localBookTestEPUB(t), TOCRule: "spin",
	}
	parsed, err := engine.ParseEPUBWithRule(request.Data, request.TOCRule)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Chapters[0].Content = "旧版完整快照正文"
	prepared := NewPreparedImport(request, parsed)
	if prepared.EPUBCatalogOnly {
		t.Fatal("legacy full-content snapshot unexpectedly marked catalogue-only")
	}
	book, err := NewImporter(cfg, database).ImportPrepared(request, prepared)
	if err != nil {
		t.Fatal(err)
	}
	var chapter models.Chapter
	if err := database.Where("book_id = ? AND `index` = 0", book.ID).First(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(cfg.LibraryDir, book.LibraryPath, chapter.CachePath))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "旧版完整快照正文" {
		t.Fatalf("legacy prepared content = %q", content)
	}
}

func TestImporterEPUBExtractionIsCompensatedWhenDatabaseCommitCannotStart(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
		JWTSecret:    "failed-epub-import-secret",
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "failed-epub-import", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	request := ImportRequest{
		UserID: user.ID, UserName: user.Username, FileName: "failed.epub", Extension: ".epub", Data: localBookTestEPUB(t), TOCRule: "spin+toc",
	}
	importer := NewImporter(cfg, database)
	_, prepared, err := importer.Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := importer.ImportPrepared(request, prepared); err == nil {
		t.Fatal("EPUB import with a closed database must fail")
	}
	userLibrary := filepath.Join(cfg.LibraryDir, "data", user.Username)
	entries, readErr := os.ReadDir(userLibrary)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed EPUB import left archive/extraction entries: %+v", entries)
	}
}

func TestClassifyEPUBPreparationErrorKeepsClientMessageSafe(t *testing.T) {
	cause := errors.Join(epubreader.ErrUnsafePath, errors.New("/private/library/secret.epub"))
	err := classifyEPUBPreparationError(cause)
	if !errors.Is(err, ErrParseFailed) || !errors.Is(err, epubreader.ErrUnsafePath) {
		t.Fatalf("classified error = %v, want parse and unsafe-path identity", err)
	}
	if strings.Contains(err.Error(), "/private/library") || strings.Contains(err.Error(), "secret.epub") {
		t.Fatalf("client-visible preparation error leaked a host path: %q", err)
	}
	if got := classifyEPUBPreparationError(errors.New("disk unavailable")); got.Error() != "disk unavailable" {
		t.Fatalf("internal storage error was incorrectly classified as client input: %v", got)
	}
}

func localBookTestEPUB(t *testing.T) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	write := func(name, body string) {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("META-INF/container.xml", `<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`)
	write("OPS/content.opf", `<package><metadata><title>目录快照</title><creator>作者</creator></metadata><manifest>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  <item id="one" href="one.xhtml" media-type="application/xhtml+xml"/>
  <item id="two" href="two.xhtml" media-type="application/xhtml+xml"/>
</manifest><spine><itemref idref="one"/><itemref idref="two"/></spine></package>`)
	write("OPS/nav.xhtml", `<html><body><nav epub:type="toc"><a href="one.xhtml">目录一</a><a href="two.xhtml">目录二</a></nav></body></html>`)
	write("OPS/one.xhtml", `<html><head><title>文档一</title></head><body><h1>正文一</h1><p>第一章正文。</p></body></html>`)
	write("OPS/two.xhtml", `<html><head><title>文档二</title></head><body><h1>正文二</h1><p>第二章正文。</p></body></html>`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
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
