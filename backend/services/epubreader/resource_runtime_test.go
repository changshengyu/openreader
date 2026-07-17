package epubreader

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if fingerprintCalls != 1 {
		t.Fatalf("immutable extracted resource rehashed the EPUB: calls = %d", fingerprintCalls)
	}
}
