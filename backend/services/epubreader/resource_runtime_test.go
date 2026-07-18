package epubreader

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestOpenResourceUsesTheImmutableExtractionBoundToTheCapability(t *testing.T) {
	libraryDir := t.TempDir()
	bookDirectory := filepath.Join("data", "reader", "epub-one")
	bookRoot := filepath.Join(libraryDir, bookDirectory)
	if err := os.MkdirAll(bookRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceRelative := filepath.Join(bookDirectory, "book.epub")
	sourcePath := filepath.Join(libraryDir, sourceRelative)
	archive := epubZip(t, map[string]string{
		"META-INF/container.xml": "<container/>",
		"OPS/Text/one.xhtml":     "<html><body><p>正文</p></body></html>",
		"OPS/Styles/book.css":    "body { color: #333; }",
	}, nil)
	if err := os.WriteFile(sourcePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "epub.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Book{}, &models.Chapter{}); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       9,
		Title:        "EPUB",
		URL:          sourceRelative,
		LibraryPath:  bookDirectory,
		OriginalFile: sourceRelative,
	}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{
		BookID:       book.ID,
		Index:        0,
		Title:        "第一章",
		ResourcePath: "OPS/Text/one.xhtml",
	}
	if err := database.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	service := New(config.Config{LibraryDir: libraryDir, JWTSecret: "epub-runtime-secret"}, database)
	fingerprintCalls := 0
	service.fingerprint = func(path string) (string, error) {
		fingerprintCalls++
		return fingerprintFile(path)
	}
	prepared, err := service.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	capability := strings.TrimPrefix(prepared.ResourceURL, "/api/epub-resource/")
	capability = strings.SplitN(capability, "/", 2)[0]
	capability, err = url.PathUnescape(capability)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprintCalls != 1 {
		t.Fatalf("prepare fingerprint calls = %d, want 1", fingerprintCalls)
	}
	if _, err := service.OpenResource(capability, "OPS/Styles/book.css"); err != nil {
		t.Fatal(err)
	}
	if fingerprintCalls != 1 {
		t.Fatalf("unchanged resource request rehashed the EPUB: calls = %d", fingerprintCalls)
	}
	fingerprint, err := fingerprintFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	extractedCSS := filepath.Join(bookRoot, extractionDirectoryName, fingerprint, "OPS", "Styles", "book.css")
	if err := os.Remove(extractedCSS); err != nil {
		t.Fatalf("remove derived stylesheet: %v", err)
	}
	if _, err := service.OpenResource(capability, "OPS/Styles/book.css"); err != nil {
		t.Fatalf("missing derived resource was not repaired: %v", err)
	}
	if fingerprintCalls != 2 {
		t.Fatalf("missing derived resource repair fingerprint calls = %d, want 2", fingerprintCalls)
	}
	if _, err := os.Stat(extractedCSS); err != nil {
		t.Fatalf("missing derived stylesheet was not restored: %v", err)
	}

	if err := os.Rename(sourcePath, sourcePath+".moved"); err != nil {
		t.Fatal(err)
	}
	resource, err := service.OpenResource(capability, "OPS/Styles/book.css")
	if err != nil {
		t.Fatalf("signed immutable extraction should not reopen the source EPUB: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(resource.Path), "/OPS/Styles/book.css") {
		t.Fatalf("resource path = %q", resource.Path)
	}
	if fingerprintCalls != 2 {
		t.Fatalf("immutable extracted resource rehashed the EPUB: calls = %d", fingerprintCalls)
	}
}

func TestPrepareChapterReusesCompleteMatchingExtractionWithoutRehashing(t *testing.T) {
	libraryDir := t.TempDir()
	bookDirectory := filepath.Join("data", "reader", "epub-warm")
	bookRoot := filepath.Join(libraryDir, bookDirectory)
	if err := os.MkdirAll(bookRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceRelative := filepath.Join(bookDirectory, "book.epub")
	sourcePath := filepath.Join(libraryDir, sourceRelative)
	archive := epubZip(t, map[string]string{
		"META-INF/container.xml": "<container/>",
		"OPS/Text/one.xhtml":     "<html><body><p>正文</p></body></html>",
	}, nil)
	if err := os.WriteFile(sourcePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "epub-warm.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Book{}, &models.Chapter{}); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       9,
		Title:        "EPUB",
		URL:          sourceRelative,
		LibraryPath:  bookDirectory,
		OriginalFile: sourceRelative,
	}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "第一章", ResourcePath: "OPS/Text/one.xhtml"}
	if err := database.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}

	warming := New(config.Config{LibraryDir: libraryDir, JWTSecret: "epub-warm-secret"}, database)
	if err := warming.PrepareBookResources(book); err != nil {
		t.Fatalf("prepare book resources: %v", err)
	}

	service := New(config.Config{LibraryDir: libraryDir, JWTSecret: "epub-warm-secret"}, database)
	fingerprintCalls := 0
	service.fingerprint = func(path string) (string, error) {
		fingerprintCalls++
		return fingerprintFile(path)
	}
	prepared, err := service.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprintCalls != 0 {
		t.Fatalf("matching complete extraction rehashed the EPUB %d times, want 0", fingerprintCalls)
	}
	text, err := service.ReadChapterText(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "正文") {
		t.Fatalf("requested chapter text = %q, want extracted document body", text)
	}
	if fingerprintCalls != 0 {
		t.Fatalf("requested chapter text rehashed the EPUB %d times, want 0", fingerprintCalls)
	}
	fingerprint, err := fingerprintFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	extractedChapter := filepath.Join(bookRoot, extractionDirectoryName, fingerprint, "OPS", "Text", "one.xhtml")
	if err := os.Remove(extractedChapter); err != nil {
		t.Fatalf("remove derived chapter resource: %v", err)
	}
	if _, err := service.PrepareChapter(book, &chapter); err != nil {
		t.Fatalf("missing chapter resource was not repaired: %v", err)
	}
	if fingerprintCalls != 1 {
		t.Fatalf("missing chapter repair fingerprint calls = %d, want 1", fingerprintCalls)
	}
	if _, err := os.Stat(extractedChapter); err != nil {
		t.Fatalf("missing chapter resource was not restored: %v", err)
	}

	updated := epubZip(t, map[string]string{
		"META-INF/container.xml": "<container/>",
		"OPS/Text/one.xhtml":     "<html><body><p>替换后的正文</p></body></html>",
	}, nil)
	if err := os.WriteFile(sourcePath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	changedTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(sourcePath, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareChapter(book, &chapter); err != nil {
		t.Fatalf("changed valid source should build a new extraction: %v", err)
	}
	if fingerprintCalls == 0 {
		t.Fatal("changed source did not fall back to fingerprint validation")
	}
	oldCapability := strings.TrimPrefix(prepared.ResourceURL, "/api/epub-resource/")
	oldCapability = strings.SplitN(oldCapability, "/", 2)[0]
	oldCapability, err = url.PathUnescape(oldCapability)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenResource(oldCapability, "OPS/Text/one.xhtml"); err != ErrInvalidCapability {
		t.Fatalf("old capability after source replacement error = %v, want %v", err, ErrInvalidCapability)
	}
}
