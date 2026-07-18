package cbzreader

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

func TestPrepareChapterCreatesReusableImmutableImageGeneration(t *testing.T) {
	service, book, chapter, _ := cbzRuntimeFixture(t)

	prepared, err := service.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(prepared.ResourceURL, "/api/cbz-resource/") {
		t.Fatalf("resource URL = %q", prepared.ResourceURL)
	}

	parent := filepath.Join(service.cfg.LibraryDir, book.LibraryPath, extractionDirectoryName)
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("immutable CBZ generation = %+v, err=%v", entries, err)
	}
	marker := filepath.Join(parent, entries[0].Name(), extractionMarkerName)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("complete marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, entries[0].Name(), "pages", "001.jpg")); err != nil {
		t.Fatalf("derived first page: %v", err)
	}
}

func TestSignedImmutableResourceSurvivesTemporarySourceAbsence(t *testing.T) {
	service, book, chapter, sourcePath := cbzRuntimeFixture(t)
	prepared, err := service.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	capability := capabilityFromResourceURL(t, prepared.ResourceURL)
	if err := os.Rename(sourcePath, sourcePath+".offline"); err != nil {
		t.Fatal(err)
	}

	resource, err := service.OpenResource(capability, "/pages/001.jpg")
	if err != nil {
		t.Fatalf("complete signed generation should survive missing source: %v", err)
	}
	if resource.ContentType != "image/jpeg" {
		t.Fatalf("content type = %q", resource.ContentType)
	}
}

func TestWarmPrepareAndOpenDoNotRehashSourceArchive(t *testing.T) {
	service, book, chapter, _ := cbzRuntimeFixture(t)
	originalFingerprint := service.fingerprint
	fingerprintCalls := 0
	service.fingerprint = func(sourcePath string) (string, error) {
		fingerprintCalls++
		return originalFingerprint(sourcePath)
	}

	prepared, err := service.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareChapter(book, &chapter); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenResource(capabilityFromResourceURL(t, prepared.ResourceURL), "/pages/001.jpg"); err != nil {
		t.Fatal(err)
	}
	if fingerprintCalls != 1 {
		t.Fatalf("warm Prepare/Open hashed source %d times, want once", fingerprintCalls)
	}
}

func TestMissingDerivedImageRebuildsOnce(t *testing.T) {
	service, book, chapter, _ := cbzRuntimeFixture(t)
	originalFingerprint := service.fingerprint
	fingerprintCalls := 0
	service.fingerprint = func(sourcePath string) (string, error) {
		fingerprintCalls++
		return originalFingerprint(sourcePath)
	}
	if _, err := service.PrepareChapter(book, &chapter); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(service.cfg.LibraryDir, book.LibraryPath, extractionDirectoryName)
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 {
		t.Fatalf("generation entries=%+v err=%v", entries, err)
	}
	pagePath := filepath.Join(parent, entries[0].Name(), "pages", "001.jpg")
	if err := os.Remove(pagePath); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareChapter(book, &chapter); err != nil {
		t.Fatalf("missing derived image was not rebuilt: %v", err)
	}
	if _, err := service.PrepareChapter(book, &chapter); err != nil {
		t.Fatal(err)
	}
	if fingerprintCalls != 2 {
		t.Fatalf("missing image rebuild fingerprint calls=%d, want initial+one rebuild", fingerprintCalls)
	}
}

func TestSourceReplacementInvalidatesOldCapability(t *testing.T) {
	service, book, chapter, sourcePath := cbzRuntimeFixture(t)
	prepared, err := service.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, cbzRuntimeArchiveWithFirstPage(t, "replacement-page"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = service.OpenResource(capabilityFromResourceURL(t, prepared.ResourceURL), "/pages/001.jpg")
	if !errors.Is(err, ErrInvalidCapability) {
		t.Fatalf("old capability after source replacement = %v, want ErrInvalidCapability", err)
	}
	replacement, err := service.PrepareChapter(book, &chapter)
	if err != nil {
		t.Fatalf("replacement source did not create a new generation: %v", err)
	}
	if replacement.ResourceURL == prepared.ResourceURL {
		t.Fatal("source replacement reused the old capability")
	}
	resource, err := service.OpenResource(capabilityFromResourceURL(t, replacement.ResourceURL), "/pages/001.jpg")
	if err != nil || resource.ContentType != "image/jpeg" {
		t.Fatalf("replacement capability resource = %+v, err=%v", resource, err)
	}
	entries, err := os.ReadDir(filepath.Join(service.cfg.LibraryDir, book.LibraryPath, extractionDirectoryName))
	if err != nil || len(entries) != 1 {
		t.Fatalf("source replacement retained stale generations: entries=%+v err=%v", entries, err)
	}
}

func TestExtractionRejectsFileDirectoryConflictWithoutActiveGeneration(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"pages", "pages/001.jpg"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("image")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	sourcePath := filepath.Join(root, "conflict.cbz")
	if err := os.WriteFile(sourcePath, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "derived")
	_, err := extractArchiveFile(sourcePath, destination, extractionLimitsFromConfig(config.Config{}))
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("file/directory conflict error = %v, want ErrUnsafePath", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("failed extraction left active destination: %v", statErr)
	}
}

func cbzRuntimeFixture(t *testing.T) (*Service, models.Book, models.Chapter, string) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
		JWTSecret:    "cbz-runtime-secret",
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "cbz-runtime", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	libraryPath := filepath.Join("data", "cbz-runtime", "fixture_")
	bookRoot := filepath.Join(cfg.LibraryDir, libraryPath)
	if err := os.MkdirAll(bookRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(bookRoot, "fixture.cbz")
	if err := os.WriteFile(sourcePath, cbzRuntimeArchive(t), 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       user.ID,
		Title:        "CBZ runtime",
		URL:          "local://cbz-runtime",
		LibraryPath:  libraryPath,
		OriginalFile: filepath.Join(libraryPath, "fixture.cbz"),
		ChapterCount: 2,
	}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	chapter := models.Chapter{BookID: book.ID, Index: 0, Title: "pages/001.jpg", ResourcePath: "pages/001.jpg"}
	if err := database.Create(&chapter).Error; err != nil {
		t.Fatal(err)
	}
	return New(cfg, database), book, chapter, sourcePath
}

func cbzRuntimeArchive(t *testing.T) []byte {
	return cbzRuntimeArchiveWithFirstPage(t, "first")
}

func cbzRuntimeArchiveWithFirstPage(t *testing.T, firstPage string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, item := range []struct{ name, body string }{
		{"ComicInfo.xml", `<ComicInfo><Title>CBZ runtime</Title></ComicInfo>`},
		{"pages/002.png", "second"},
		{"pages/001.jpg", firstPage},
		{"notes/readme.txt", "not served"},
	} {
		entry, err := writer.Create(item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(item.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func capabilityFromResourceURL(t *testing.T, resourceURL string) string {
	t.Helper()
	parsed, err := url.Parse(resourceURL)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/api/cbz-resource/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		t.Fatalf("unexpected resource URL: %q", resourceURL)
	}
	return parts[0]
}
