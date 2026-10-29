package umdcover

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openreader/backend/config"
	readerdb "openreader/backend/db"
	"openreader/backend/models"
)

// Contract: docs/compat/local-book-cover-fixed-baseline-second-audit-p1-contract.md (C2, A6)

var (
	firstCover  = append([]byte("\x89PNG\r\n\x1a\n"), []byte("first cover")...)
	secondCover = append([]byte{0xff, 0xd8, 0xff, 0xe0}, []byte("second cover")...)
)

// umdWithCover frames a UMD whose segment 130 additional section holds cover;
// a nil cover leaves the segment out.
func umdWithCover(cover []byte) []byte {
	var data bytes.Buffer
	data.Write([]byte{0x89, 0x9b, 0x9a, 0xde})
	section := func(segmentType uint16, flag byte, payload []byte) {
		data.WriteByte('#')
		_ = binary.Write(&data, binary.LittleEndian, segmentType)
		data.WriteByte(flag)
		data.WriteByte(byte(len(payload) + 5))
		data.Write(payload)
	}
	section(0x01, 0, []byte{0x01, 0x11, 0x22})
	section(0x02, 0, []byte{'U', 0, 'M', 0, 'D', 0})
	if cover != nil {
		section(0x82, 0x10, []byte{0x01, 0xfe, 0xca, 0xad, 0x0b})
		data.WriteByte('$')
		_ = binary.Write(&data, binary.LittleEndian, uint32(0x0badcafe))
		_ = binary.Write(&data, binary.LittleEndian, uint32(len(cover)+9))
		data.Write(cover)
	}
	return data.Bytes()
}

func umdCoverFixture(t *testing.T, data []byte) (*Service, models.Book, string) {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{
		DataDir:      filepath.Join(root, "data"),
		CacheDir:     filepath.Join(root, "cache"),
		LibraryDir:   filepath.Join(root, "library"),
		DatabasePath: filepath.Join(root, "data", "openreader.db"),
		JWTSecret:    "umd-cover-secret",
	}
	database, err := readerdb.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := readerdb.AutoMigrate(database); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "umd-cover", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	libraryPath := filepath.Join("data", "umd-cover", "fixture_")
	bookRoot := filepath.Join(cfg.LibraryDir, libraryPath)
	if err := os.MkdirAll(bookRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bookRoot, "fixture.umd"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       user.ID,
		Title:        "UMD cover",
		URL:          "local://umd-cover",
		LibraryPath:  libraryPath,
		OriginalFile: filepath.Join(libraryPath, "fixture.umd"),
	}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	return New(cfg, database), book, bookRoot
}

func capabilityOf(t *testing.T, prepared PreparedCover) string {
	t.Helper()
	escaped, ok := strings.CutPrefix(prepared.ResourceURL, "/api/umd-cover/")
	if !ok || strings.Contains(escaped, "/") {
		t.Fatalf("cover URL = %q", prepared.ResourceURL)
	}
	capability, err := url.PathUnescape(escaped)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func readCover(t *testing.T, service *Service, capability string) (string, []byte) {
	t.Helper()
	resource, err := service.OpenCover(capability)
	if err != nil {
		t.Fatalf("open cover: %v", err)
	}
	defer resource.File.Close()
	data, err := io.ReadAll(resource.File)
	if err != nil {
		t.Fatal(err)
	}
	return resource.ContentType, data
}

func extractionNames(t *testing.T, bookRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(bookRoot, extractionDirectoryName))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestCoverIsExtractedOnceAndServedThroughItsCapability(t *testing.T) {
	service, book, bookRoot := umdCoverFixture(t, umdWithCover(firstCover))
	prepared, err := service.PrepareCover(book)
	if err != nil {
		t.Fatal(err)
	}
	contentType, data := readCover(t, service, capabilityOf(t, prepared))
	if contentType != "image/png" || !bytes.Equal(data, firstCover) {
		t.Fatalf("cover = %q %q", contentType, data)
	}
	names := extractionNames(t, bookRoot)
	if len(names) != 1 {
		t.Fatalf("extractions = %v", names)
	}
	markerPath := filepath.Join(bookRoot, extractionDirectoryName, names[0], extractionMarkerName)
	before, err := os.Stat(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareCover(book); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(markerPath)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("an unchanged archive must reuse its extraction: %v", err)
	}
}

func TestCoverlessUMDRecordsThatItHasNoCover(t *testing.T) {
	service, book, bookRoot := umdCoverFixture(t, umdWithCover(nil))
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.PrepareCover(book); !errors.Is(err, ErrNotFound) {
			t.Fatalf("attempt %d: err = %v, want ErrNotFound", attempt, err)
		}
	}
	names := extractionNames(t, bookRoot)
	if len(names) != 1 {
		t.Fatalf("extractions = %v", names)
	}
	data, err := os.ReadFile(filepath.Join(bookRoot, extractionDirectoryName, names[0], extractionMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	var marker extractionMarker
	if err := json.Unmarshal(data, &marker); err != nil || marker.CoverName != "" || marker.Fingerprint != names[0] {
		t.Fatalf("no-cover marker = %s, %v", data, err)
	}
}

func TestReplacedArchiveInvalidatesTheOldCapability(t *testing.T) {
	service, book, bookRoot := umdCoverFixture(t, umdWithCover(firstCover))
	first, err := service.PrepareCover(book)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(bookRoot, "fixture.umd")
	if err := os.WriteFile(sourcePath, umdWithCover(secondCover), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(sourcePath, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenCover(capabilityOf(t, first)); !errors.Is(err, ErrInvalidCapability) {
		t.Fatalf("old capability after replacement: err = %v", err)
	}
	second, err := service.PrepareCover(book)
	if err != nil {
		t.Fatal(err)
	}
	contentType, data := readCover(t, service, capabilityOf(t, second))
	if contentType != "image/jpeg" || !bytes.Equal(data, secondCover) {
		t.Fatalf("replacement cover = %q %q", contentType, data)
	}
	if names := extractionNames(t, bookRoot); len(names) != 1 {
		t.Fatalf("older extractions must be pruned: %v", names)
	}
}

func TestRemovedExtractionIsRebuiltAndSurvivesAMissingArchive(t *testing.T) {
	service, book, bookRoot := umdCoverFixture(t, umdWithCover(firstCover))
	prepared, err := service.PrepareCover(book)
	if err != nil {
		t.Fatal(err)
	}
	capability := capabilityOf(t, prepared)
	if err := os.RemoveAll(filepath.Join(bookRoot, extractionDirectoryName)); err != nil {
		t.Fatal(err)
	}
	if _, data := readCover(t, service, capability); !bytes.Equal(data, firstCover) {
		t.Fatalf("rebuilt cover = %q", data)
	}
	if err := os.Remove(filepath.Join(bookRoot, "fixture.umd")); err != nil {
		t.Fatal(err)
	}
	if _, data := readCover(t, service, capability); !bytes.Equal(data, firstCover) {
		t.Fatalf("complete extraction without its archive = %q", data)
	}
}

func TestDeletedBookDirectoryIsNotRecreated(t *testing.T) {
	service, book, bookRoot := umdCoverFixture(t, umdWithCover(firstCover))
	prepared, err := service.PrepareCover(book)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(bookRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareCover(book); !errors.Is(err, ErrNotFound) {
		t.Fatalf("prepare after directory removal: err = %v", err)
	}
	if _, err := service.OpenCover(capabilityOf(t, prepared)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("open after directory removal: err = %v", err)
	}
	if _, err := os.Lstat(bookRoot); !os.IsNotExist(err) {
		t.Fatalf("book directory was recreated: %v", err)
	}
}

func TestSymlinkedArchiveIsNotRead(t *testing.T) {
	service, book, bookRoot := umdCoverFixture(t, umdWithCover(nil))
	outside := filepath.Join(t.TempDir(), "outside.umd")
	if err := os.WriteFile(outside, umdWithCover(firstCover), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(bookRoot, "fixture.umd")
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, sourcePath); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrepareCover(book); !errors.Is(err, ErrNotFound) {
		t.Fatalf("symlinked archive: err = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(bookRoot, extractionDirectoryName)); !os.IsNotExist(err) {
		t.Fatalf("nothing may be extracted from a symlinked archive: %v", err)
	}
}

func TestCapabilitiesAreBoundToTheirSignatureAndLifetime(t *testing.T) {
	service, book, _ := umdCoverFixture(t, umdWithCover(firstCover))
	prepared, err := service.PrepareCover(book)
	if err != nil {
		t.Fatal(err)
	}
	capability := capabilityOf(t, prepared)
	payload, signature, _ := strings.Cut(capability, ".")
	for _, test := range []struct {
		capability string
		want       error
	}{
		{capability: "not-a-capability", want: ErrMalformedCapability},
		{capability: payload + "." + strings.Repeat("A", len(signature)), want: ErrInvalidCapability},
		{capability: payload + "x." + signature, want: ErrInvalidCapability},
	} {
		if _, err := service.OpenCover(test.capability); !errors.Is(err, test.want) {
			t.Fatalf("OpenCover(%q) err = %v, want %v", test.capability, err, test.want)
		}
	}
	service.now = func() time.Time { return prepared.ExpiresAt }
	if _, err := service.OpenCover(capability); !errors.Is(err, ErrExpiredCapability) {
		t.Fatalf("expired capability: err = %v", err)
	}
}
