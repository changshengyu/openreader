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
	ErrPortableAssetUnavailable   = errors.New("custom asset unavailable for portable backup")
	ErrPortableBackupLimit        = errors.New("portable backup exceeds safety limits")
)

const portableManifestName = "openreader-portable-v2.json"

type portableManifest struct {
	Format       string                  `json:"format"`
	Version      int                     `json:"version"`
	CreatedAt    time.Time               `json:"createdAt"`
	Books        []portableManifestBook  `json:"books"`
	Assets       []portableManifestAsset `json:"assets"`
	LegacyAssets int                     `json:"legacyAssets,omitempty"`
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
	size int64
}

type portableManifestAsset struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Extension string `json:"extension"`
	Entry     string `json:"entry"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

type portableAssetInput struct {
	manifest portableManifestAsset
	path     string
}

type PortableResult struct {
	Path         string
	LocalBooks   int
	Assets       int
	LegacyAssets int
}

// RunPortableForUser produces a separately named, self-describing OpenReader package. It is
// intentionally not used by RunNow/RunNowForUser: reader-dev-compatible logical backups must not
// start carrying library files as an accidental schema change.
func (s *Service) RunPortableForUser(userID uint, username, backupDir string) (string, int, error) {
	result, err := s.RunPortableV2ForUser(userID, username, backupDir)
	return result.Path, result.LocalBooks, err
}

func (s *Service) RunPortableV2ForUser(userID uint, username, backupDir string) (PortableResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(s.cfg.LibraryDir) == "" {
		return PortableResult{}, ErrPortableBackupUnavailable
	}
	books, err := s.collectPortableArchives(userID, username)
	if err != nil {
		return PortableResult{}, err
	}
	logicalEntries, assets, legacyAssets, err := s.collectPortableAssetBundle(userID)
	if err != nil {
		return PortableResult{}, err
	}
	createdAt := time.Now().UTC()
	if err := s.validatePortableExportBudget(logicalEntries, books, assets, legacyAssets, createdAt); err != nil {
		return PortableResult{}, err
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return PortableResult{}, err
	}
	finalPath, err := nextBackupPath(backupDir, "portable_backup_"+time.Now().Format("20060102_150405"))
	if err != nil {
		return PortableResult{}, err
	}
	temporary, err := os.CreateTemp(backupDir, ".portable-backup-*.tmp")
	if err != nil {
		return PortableResult{}, err
	}
	temporaryPath := temporary.Name()
	completed := false
	defer func() {
		_ = temporary.Close()
		if !completed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return PortableResult{}, err
	}

	writer := zip.NewWriter(temporary)
	if err := writePortableLogicalEntries(writer, logicalEntries); err != nil {
		_ = writer.Close()
		return PortableResult{}, err
	}

	manifest := portableManifest{
		Format:       "openreader-portable-backup",
		Version:      2,
		CreatedAt:    createdAt,
		Books:        make([]portableManifestBook, 0, len(books)),
		Assets:       make([]portableManifestAsset, 0, len(assets)),
		LegacyAssets: legacyAssets,
	}
	for index, item := range books {
		extension := strings.ToLower(filepath.Ext(item.path))
		entryName := fmt.Sprintf("local-books/b%04d/original%s", index+1, extension)
		entry, err := writer.Create(entryName)
		if err != nil {
			_ = writer.Close()
			return PortableResult{}, err
		}
		file, err := os.Open(item.path)
		if err != nil {
			_ = writer.Close()
			return PortableResult{}, portableUnavailable(item.book)
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != item.size {
			_ = file.Close()
			_ = writer.Close()
			return PortableResult{}, portableUnavailable(item.book)
		}
		hash := sha256.New()
		written, copyErr := io.Copy(
			io.MultiWriter(entry, hash),
			io.LimitReader(file, item.size+1),
		)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || written != item.size {
			_ = writer.Close()
			return PortableResult{}, portableUnavailable(item.book)
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
	for _, item := range assets {
		if err := writePortableAssetEntry(writer, item); err != nil {
			_ = writer.Close()
			return PortableResult{}, err
		}
		manifest.Assets = append(manifest.Assets, item.manifest)
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		_ = writer.Close()
		return PortableResult{}, err
	}
	manifestWriter, err := writer.Create(portableManifestName)
	if err != nil {
		_ = writer.Close()
		return PortableResult{}, err
	}
	if _, err := manifestWriter.Write(manifestData); err != nil {
		_ = writer.Close()
		return PortableResult{}, err
	}
	if err := writer.Close(); err != nil {
		return PortableResult{}, err
	}
	if err := temporary.Sync(); err != nil {
		return PortableResult{}, err
	}
	if err := temporary.Close(); err != nil {
		return PortableResult{}, err
	}
	if info, err := os.Stat(temporaryPath); err != nil {
		return PortableResult{}, err
	} else if info.Size() > portableConfiguredCompressedLimit(s.cfg.MaxPortableBackupBytes) {
		return PortableResult{}, ErrPortableBackupLimit
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return PortableResult{}, err
	}
	completed = true
	return PortableResult{
		Path:         finalPath,
		LocalBooks:   len(books),
		Assets:       len(assets),
		LegacyAssets: legacyAssets,
	}, nil
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
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
			return nil, portableUnavailable(book)
		}
		items = append(items, portableArchiveInput{book: book, path: path, size: info.Size()})
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

func writePortableLogicalEntries(writer *zip.Writer, entries map[string][]byte) error {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write(entries[name]); err != nil {
			return err
		}
	}
	return nil
}

func portableConfiguredCompressedLimit(value int64) int64 {
	if value > 0 {
		return value
	}
	return 512 * 1024 * 1024
}

func (s *Service) validatePortableExportBudget(
	logicalEntries map[string][]byte,
	books []portableArchiveInput,
	assets []portableAssetInput,
	legacyAssets int,
	createdAt time.Time,
) error {
	maxEntries := s.cfg.MaxPortableArchiveEntries
	if maxEntries <= 0 {
		maxEntries = 10_000
	}
	maxEntryBytes := s.cfg.MaxPortableArchiveBytes
	if maxEntryBytes <= 0 {
		maxEntryBytes = 256 * 1024 * 1024
	}
	maxTotalBytes := s.cfg.MaxPortableArchiveTotal
	if maxTotalBytes <= 0 {
		maxTotalBytes = 512 * 1024 * 1024
	}
	if len(logicalEntries)+len(books)+len(assets)+1 > maxEntries {
		return ErrPortableBackupLimit
	}
	var total int64
	add := func(size int64) error {
		if size < 0 || size > maxEntryBytes || total > maxTotalBytes-size {
			return ErrPortableBackupLimit
		}
		total += size
		return nil
	}
	for _, data := range logicalEntries {
		if err := add(int64(len(data))); err != nil {
			return err
		}
	}
	manifest := portableManifest{
		Format:       "openreader-portable-backup",
		Version:      2,
		CreatedAt:    createdAt,
		Books:        make([]portableManifestBook, 0, len(books)),
		Assets:       make([]portableManifestAsset, 0, len(assets)),
		LegacyAssets: legacyAssets,
	}
	for index, book := range books {
		if err := add(book.size); err != nil {
			return err
		}
		extension := strings.ToLower(filepath.Ext(book.path))
		manifest.Books = append(manifest.Books, portableManifestBook{
			BookURL: book.book.URL, Title: book.book.Title, Author: book.book.Author,
			TOCRule: book.book.TOCRule, Extension: extension,
			Entry: fmt.Sprintf("local-books/b%04d/original%s", index+1, extension),
			Size:  book.size, SHA256: strings.Repeat("0", sha256.Size*2),
		})
	}
	for _, asset := range assets {
		if err := add(asset.manifest.Size); err != nil {
			return err
		}
		manifest.Assets = append(manifest.Assets, asset.manifest)
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if len(manifestData) > 1024*1024 {
		return ErrPortableBackupLimit
	}
	if err := add(int64(len(manifestData))); err != nil {
		return err
	}
	return nil
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
