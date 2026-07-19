package backup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"openreader/backend/engine"
	"openreader/backend/models"
)

var (
	// ErrPortableArchiveUnavailable deliberately carries no filesystem information. Callers may
	// present the book title but must never reveal a configured library or host path.
	ErrPortableArchiveUnavailable = errors.New("local archive unavailable for portable backup")
	ErrPortableBackupUnavailable  = errors.New("portable backup storage is unavailable")
)

const portableManifestName = "openreader-portable-v1.json"

type portableManifest struct {
	Format    string                 `json:"format"`
	Version   int                    `json:"version"`
	CreatedAt time.Time              `json:"createdAt"`
	Books     []portableManifestBook `json:"books"`
}

type portableManifestBook struct {
	BookURL   string `json:"bookUrl"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	TOCRule   string `json:"tocRule,omitempty"`
	Extension string `json:"extension"`
	Entry     string `json:"entry"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

type portableArchiveInput struct {
	book models.Book
	path string
}

// RunPortableForUser produces a separately named, self-describing OpenReader package. It is
// intentionally not used by RunNow/RunNowForUser: reader-dev-compatible logical backups must not
// start carrying library files as an accidental schema change.
func (s *Service) RunPortableForUser(userID uint, username, backupDir string) (string, int, error) {
	if strings.TrimSpace(s.cfg.LibraryDir) == "" {
		return "", 0, ErrPortableBackupUnavailable
	}
	books, err := s.collectPortableArchives(userID, username)
	if err != nil {
		return "", 0, err
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", 0, err
	}
	name := fmt.Sprintf("portable_backup_%s.zip", time.Now().Format("20060102_150405"))
	finalPath := filepath.Join(backupDir, name)
	temporary, err := os.CreateTemp(backupDir, ".portable-backup-*.tmp")
	if err != nil {
		return "", 0, err
	}
	temporaryPath := temporary.Name()
	completed := false
	defer func() {
		_ = temporary.Close()
		if !completed {
			_ = os.Remove(temporaryPath)
		}
	}()

	writer := zip.NewWriter(temporary)
	// Keep exactly the same logical entries as the ordinary per-user backup. The portable
	// manifest and archive entries are additive only to this explicit format.
	s.addSources(writer)
	s.addRSSSources(writer, &userID)
	s.addUserSettings(writer, &userID)
	s.addCategories(writer, &userID)
	s.addBookGroups(writer, &userID)
	s.addBookshelf(writer, &userID)
	s.addChapterVariables(writer, &userID)
	s.addBookmarks(writer, &userID)
	s.addProgress(writer, &userID)
	s.addReplaceRules(writer, &userID)

	manifest := portableManifest{
		Format:    "openreader-portable-backup",
		Version:   1,
		CreatedAt: time.Now().UTC(),
		Books:     make([]portableManifestBook, 0, len(books)),
	}
	for index, item := range books {
		extension := strings.ToLower(filepath.Ext(item.path))
		entryName := fmt.Sprintf("local-books/b%04d/original%s", index+1, extension)
		entry, err := writer.Create(entryName)
		if err != nil {
			_ = writer.Close()
			return "", 0, err
		}
		file, err := os.Open(item.path)
		if err != nil {
			_ = writer.Close()
			return "", 0, portableUnavailable(item.book)
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(entry, hash), file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			_ = writer.Close()
			return "", 0, portableUnavailable(item.book)
		}
		manifest.Books = append(manifest.Books, portableManifestBook{
			BookURL:   item.book.URL,
			Title:     item.book.Title,
			Author:    item.book.Author,
			TOCRule:   item.book.TOCRule,
			Extension: extension,
			Entry:     entryName,
			Size:      written,
			SHA256:    hex.EncodeToString(hash.Sum(nil)),
		})
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		_ = writer.Close()
		return "", 0, err
	}
	manifestWriter, err := writer.Create(portableManifestName)
	if err != nil {
		_ = writer.Close()
		return "", 0, err
	}
	if _, err := manifestWriter.Write(manifestData); err != nil {
		_ = writer.Close()
		return "", 0, err
	}
	if err := writer.Close(); err != nil {
		return "", 0, err
	}
	if err := temporary.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return "", 0, err
	}
	completed = true
	return finalPath, len(books), nil
}

func (s *Service) collectPortableArchives(userID uint, username string) ([]portableArchiveInput, error) {
	var books []models.Book
	if err := s.db.Where("user_id = ? AND source_id = ?", userID, 0).Order("id asc").Find(&books).Error; err != nil {
		return nil, err
	}
	items := make([]portableArchiveInput, 0, len(books))
	for _, book := range books {
		if book.Type == 1 {
			return nil, portableUnavailable(book)
		}
		path, ok := portableOriginalPath(s.cfg.LibraryDir, username, book)
		if !ok {
			return nil, portableUnavailable(book)
		}
		items = append(items, portableArchiveInput{book: book, path: path})
	}
	return items, nil
}

func portableUnavailable(book models.Book) error {
	title := strings.TrimSpace(book.Title)
	if title == "" {
		return ErrPortableArchiveUnavailable
	}
	return fmt.Errorf("%w: %s", ErrPortableArchiveUnavailable, title)
}

func portableOriginalPath(libraryDir, username string, book models.Book) (string, bool) {
	libraryRoot, err := filepath.EvalSymlinks(filepath.Clean(libraryDir))
	if err != nil {
		return "", false
	}
	relativeBookRoot, ok := cleanPortableRelativePath(book.LibraryPath)
	if !ok {
		return "", false
	}
	expectedPrefix := filepath.Join("data", engine.SafeFilename(username))
	if relativeBookRoot == expectedPrefix || !strings.HasPrefix(relativeBookRoot, expectedPrefix+string(filepath.Separator)) {
		return "", false
	}
	bookRoot, ok := portableJoinInside(libraryRoot, relativeBookRoot)
	if !ok {
		return "", false
	}

	originalName := filepath.Base(strings.TrimSpace(book.OriginalFile))
	if originalName == "." || originalName == string(filepath.Separator) || !portableExtensionAllowed(filepath.Ext(originalName)) {
		return "", false
	}
	path, ok := portableJoinInside(bookRoot, originalName)
	if !ok {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return "", false
	}
	return path, true
}

func cleanPortableRelativePath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, 0) || filepath.IsAbs(value) {
		return "", false
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

func portableJoinInside(root, relative string) (string, bool) {
	candidate := filepath.Join(root, relative)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return resolved, true
}

func portableExtensionAllowed(ext string) bool {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".txt", ".text", ".md", ".epub", ".pdf", ".umd", ".cbz":
		return true
	default:
		return false
	}
}

// PortableManifestForTest exposes a decoded manifest to package-level regression tests without
// making the on-disk representation part of the public API surface.
func PortableManifestForTest(path string) (portableManifest, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return portableManifest{}, err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != portableManifestName {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			return portableManifest{}, err
		}
		defer opened.Close()
		var manifest portableManifest
		if err := json.NewDecoder(opened).Decode(&manifest); err != nil {
			return portableManifest{}, err
		}
		return manifest, nil
	}
	return portableManifest{}, os.ErrNotExist
}

func sortedPortableEntries(path string) ([]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	entries := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		entries = append(entries, file.Name)
	}
	sort.Strings(entries)
	return entries, nil
}
