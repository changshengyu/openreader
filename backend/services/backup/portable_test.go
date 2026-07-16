package backup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

func TestPortableBackupExportsOnlyCallerOriginalArchives(t *testing.T) {
	libraryDir := filepath.Join(t.TempDir(), "library")
	webdavDir := filepath.Join(t.TempDir(), "webdav")
	database := portableBackupTestDB(t)
	service := New(database, webdavDir, config.Config{LibraryDir: libraryDir})

	owner := models.User{Username: "portable-owner", PasswordHash: "hash"}
	other := models.User{Username: "portable-other", PasswordHash: "hash"}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	ownerArchive := createPortableArchiveFixture(t, libraryDir, owner.Username, "first-book", "first.txt", "第一本 archive 正文")
	otherArchive := createPortableArchiveFixture(t, libraryDir, other.Username, "second-book", "second.epub", "不应导出的其他用户正文")
	ownerBook := models.Book{
		UserID:       owner.ID,
		SourceID:     0,
		Title:        "第一本",
		Author:       "作者 A",
		URL:          "local://book_101",
		LibraryPath:  filepath.Join("data", owner.Username, "first-book"),
		OriginalFile: filepath.Join("data", owner.Username, "first-book", "first.txt"),
		TOCRule:      "^第.+章",
	}
	otherBook := models.Book{
		UserID:       other.ID,
		SourceID:     0,
		Title:        "第二本",
		URL:          "local://book_202",
		LibraryPath:  filepath.Join("data", other.Username, "second-book"),
		OriginalFile: filepath.Join("data", other.Username, "second-book", "second.epub"),
	}
	if err := database.Create(&ownerBook).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&otherBook).Error; err != nil {
		t.Fatal(err)
	}

	logicalPath, err := service.RunNowForUser(owner.ID, owner.Username)
	if err != nil {
		t.Fatal(err)
	}
	logicalEntries, err := sortedPortableEntries(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range logicalEntries {
		if entry == portableManifestName || strings.HasPrefix(entry, "local-books/") {
			t.Fatalf("ordinary logical backup must not include portable entry %q", entry)
		}
	}

	portableDir := filepath.Join(webdavDir, "users", owner.Username)
	portablePath, localBooks, err := service.RunPortableForUser(owner.ID, owner.Username, portableDir)
	if err != nil {
		t.Fatal(err)
	}
	if localBooks != 1 {
		t.Fatalf("portable local book count = %d, want 1", localBooks)
	}
	if filepath.Base(portablePath) == "" || !strings.HasPrefix(filepath.Base(portablePath), "portable_backup_") {
		t.Fatalf("portable backup name = %q", portablePath)
	}

	manifest, err := PortableManifestForTest(portablePath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Format != "openreader-portable-backup" || manifest.Version != 1 || len(manifest.Books) != 1 {
		t.Fatalf("unexpected portable manifest: %+v", manifest)
	}
	entry := manifest.Books[0]
	if entry.BookURL != ownerBook.URL || entry.Title != ownerBook.Title || entry.Author != ownerBook.Author || entry.TOCRule != ownerBook.TOCRule || entry.Extension != ".txt" {
		t.Fatalf("portable manifest book = %+v", entry)
	}
	if strings.Contains(entry.Entry, owner.Username) || strings.Contains(entry.Entry, "first-book") || strings.Contains(entry.Entry, "library") {
		t.Fatalf("portable entry leaks persistent path data: %q", entry.Entry)
	}
	if !strings.HasPrefix(entry.Entry, "local-books/b") || !strings.HasSuffix(entry.Entry, "/original.txt") {
		t.Fatalf("portable entry path = %q", entry.Entry)
	}

	ownerData, err := os.ReadFile(ownerArchive)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(ownerData)
	if entry.Size != int64(len(ownerData)) || entry.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("portable manifest checksum/size = %+v", entry)
	}
	got := portableEntryData(t, portablePath, entry.Entry)
	if string(got) != string(ownerData) {
		t.Fatalf("portable archive bytes = %q, want %q", got, ownerData)
	}
	if string(got) == "不应导出的其他用户正文" {
		t.Fatal("portable backup exported another user's archive")
	}
	if _, err := os.Stat(otherArchive); err != nil {
		t.Fatalf("other user's source archive changed: %v", err)
	}
}

func TestPortableBackupRejectsUnavailableOrAudioArchiveWithoutWritingPackage(t *testing.T) {
	libraryDir := filepath.Join(t.TempDir(), "library")
	webdavDir := filepath.Join(t.TempDir(), "webdav")
	database := portableBackupTestDB(t)
	service := New(database, webdavDir, config.Config{LibraryDir: libraryDir})
	user := models.User{Username: "portable-invalid", PasswordHash: "hash"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{
		UserID:       user.ID,
		SourceID:     0,
		Title:        "遗失 archive",
		URL:          "local://book_1",
		LibraryPath:  filepath.Join("data", user.Username, "missing-book"),
		OriginalFile: filepath.Join("data", user.Username, "missing-book", "missing.txt"),
	}
	if err := database.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(webdavDir, "users", user.Username)
	if _, _, err := service.RunPortableForUser(user.ID, user.Username, backupDir); !errors.Is(err, ErrPortableArchiveUnavailable) {
		t.Fatalf("missing archive error = %v, want unavailable", err)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unavailable archive must not leave a portable package: %+v", entries)
	}

	if err := database.Model(&book).Updates(map[string]any{"type": 1, "title": "本地音频"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.RunPortableForUser(user.ID, user.Username, backupDir); !errors.Is(err, ErrPortableArchiveUnavailable) {
		t.Fatalf("audio archive error = %v, want unavailable", err)
	}
}

func portableBackupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "portable.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(
		&models.User{},
		&models.BookSource{},
		&models.RSSSource{},
		&models.UserSetting{},
		&models.Category{},
		&models.Book{},
		&models.Chapter{},
		&models.Bookmark{},
		&models.ReadingProgress{},
		&models.ReplaceRule{},
		&models.BookCategory{},
	); err != nil {
		t.Fatal(err)
	}
	return database
}

func createPortableArchiveFixture(t *testing.T, libraryDir, username, directory, fileName, contents string) string {
	t.Helper()
	path := filepath.Join(libraryDir, "data", username, directory, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func portableEntryData(t *testing.T, archivePath, name string) []byte {
	t.Helper()
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("entry %q not found", name)
	return nil
}
