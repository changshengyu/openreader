package api

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openreader/backend/models"
	"openreader/backend/services/localbook"
)

var (
	errInvalidPortableBackup  = errors.New("invalid portable backup")
	errPortableBackupLimit    = errors.New("portable backup exceeds safety limits")
	errPortableBackupConflict = errors.New("portable backup conflicts with an existing local book")
)

const portableBackupManifestName = "openreader-portable-v1.json"

type portableBackupLimits struct {
	maxCompressed int64
	maxEntries    int
	maxEntryBytes int64
	maxTotalBytes int64
}

type portableBackupManifest struct {
	Format  string                       `json:"format"`
	Version int                          `json:"version"`
	Books   []portableBackupManifestBook `json:"books"`
}

type portableBackupManifestBook struct {
	BookURL   string `json:"bookUrl"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	TOCRule   string `json:"tocRule"`
	Extension string `json:"extension"`
	Entry     string `json:"entry"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

type portableStagedBook struct {
	manifest portableBackupManifestBook
	path     string
	reuse    bool
}

type portableBackupPackage struct {
	logicalData []byte
	books       []portableStagedBook
	stagingDir  string
}

func (s *Server) portableLimits() portableBackupLimits {
	limits := portableBackupLimits{
		maxCompressed: s.cfg.MaxPortableBackupBytes,
		maxEntries:    s.cfg.MaxPortableArchiveEntries,
		maxEntryBytes: s.cfg.MaxPortableArchiveBytes,
		maxTotalBytes: s.cfg.MaxPortableArchiveTotal,
	}
	if limits.maxCompressed <= 0 {
		limits.maxCompressed = 512 * 1024 * 1024
	}
	if limits.maxEntries <= 0 {
		limits.maxEntries = 10_000
	}
	if limits.maxEntryBytes <= 0 {
		limits.maxEntryBytes = 256 * 1024 * 1024
	}
	if limits.maxTotalBytes <= 0 {
		limits.maxTotalBytes = 512 * 1024 * 1024
	}
	return limits
}

func (s *Server) restorePortableBackupFile(archivePath string, userID uint, username string) (gin.H, error) {
	packageData, err := s.preparePortableBackup(archivePath, userID, username)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(packageData.stagingDir)

	result, err := s.restoreLegadoBackupDataWithoutBroadcast(packageData.logicalData, userID)
	if err != nil {
		return nil, err
	}
	limits := s.portableLimits()
	importer := s.portableLocalBookImporter(limits)
	for _, staged := range packageData.books {
		if staged.reuse {
			continue
		}
		var book models.Book
		if err := s.db.Where("user_id = ? AND url = ? AND source_id = ?", userID, staged.manifest.BookURL, 0).First(&book).Error; err != nil {
			return nil, fmt.Errorf("%w: restored local book is missing", errInvalidPortableBackup)
		}
		data, err := readPortableStagedFile(staged.path, limits.maxEntryBytes)
		if err != nil {
			return nil, err
		}
		if _, err := importer.RestoreExisting(book, localbook.ImportRequest{
			UserID:    userID,
			UserName:  username,
			FileName:  filepath.Base(staged.path),
			Extension: staged.manifest.Extension,
			Data:      data,
			Title:     staged.manifest.Title,
			Author:    staged.manifest.Author,
			TOCRule:   staged.manifest.TOCRule,
		}); err != nil {
			return nil, err
		}
	}
	result["localBooks"] = len(packageData.books)
	s.broadcastRestoreUpdates(userID, result)
	return result, nil
}

func (s *Server) stageUploadedBackup(fileHeader *multipart.FileHeader, userID uint, maxBytes int64) (string, error) {
	if fileHeader == nil || maxBytes <= 0 {
		return "", errInvalidPortableBackup
	}
	input, err := fileHeader.Open()
	if err != nil {
		return "", errInvalidPortableBackup
	}
	defer input.Close()
	root := filepath.Join(s.cfg.CacheDir, "backup-uploads", fmt.Sprintf("%d", userID))
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	output, err := os.CreateTemp(root, "restore-*.zip")
	if err != nil {
		return "", err
	}
	path := output.Name()
	cleanup := true
	defer func() {
		_ = output.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	written, err := io.Copy(output, io.LimitReader(input, maxBytes+1))
	if err != nil || written > maxBytes {
		return "", errPortableBackupLimit
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	cleanup = false
	return path, nil
}

func (s *Server) restoreBackupFile(path string, userID uint, username string) (gin.H, error) {
	info, statErr := os.Stat(path)
	if statErr != nil || info.IsDir() {
		return nil, errInvalidBackupArchive
	}
	legacyLimit := s.backupRestoreLimits().MaxCompressedBytes
	portable, err := isPortableBackupFile(path)
	if err != nil {
		if info.Size() > legacyLimit {
			return nil, errBackupRestoreTooLarge
		}
		return nil, err
	}
	if portable {
		return s.restorePortableBackupFile(path, userID, username)
	}
	if info.Size() > legacyLimit {
		return nil, errBackupRestoreTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errInvalidBackupArchive
	}
	defer file.Close()
	data, err := readBoundedBackup(file, s.backupRestoreLimits().MaxCompressedBytes)
	if err != nil {
		return nil, err
	}
	return s.restoreLegadoBackupData(data, userID)
}

func isPortableBackupFile(path string) (bool, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return false, errInvalidBackupArchive
	}
	defer reader.Close()
	for _, file := range reader.File {
		name, err := normalizeBackupArchivePath(file.Name)
		if err != nil {
			return false, errInvalidBackupArchive
		}
		if strings.EqualFold(name, portableBackupManifestName) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) preparePortableBackup(archivePath string, userID uint, username string) (*portableBackupPackage, error) {
	limits := s.portableLimits()
	info, err := os.Stat(archivePath)
	if err != nil || info.IsDir() {
		return nil, errInvalidPortableBackup
	}
	if info.Size() > limits.maxCompressed {
		return nil, errPortableBackupLimit
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, errInvalidPortableBackup
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > limits.maxEntries {
		return nil, errPortableBackupLimit
	}

	files := make(map[string]*zip.File, len(reader.File))
	var total int64
	for _, file := range reader.File {
		name, err := normalizeBackupArchivePath(file.Name)
		if err != nil || file.FileInfo().IsDir() || file.Mode()&os.ModeSymlink != 0 {
			return nil, errInvalidPortableBackup
		}
		key := strings.ToLower(name)
		if _, exists := files[key]; exists {
			return nil, errInvalidPortableBackup
		}
		if file.UncompressedSize64 > uint64(limits.maxEntryBytes) || total > limits.maxTotalBytes-int64(file.UncompressedSize64) {
			return nil, errPortableBackupLimit
		}
		total += int64(file.UncompressedSize64)
		files[key] = file
	}

	manifestFile := files[portableBackupManifestName]
	if manifestFile == nil {
		return nil, errInvalidPortableBackup
	}
	manifestData, err := readPortableZipEntry(manifestFile, minInt64(limits.maxEntryBytes, 1024*1024))
	if err != nil {
		return nil, err
	}
	var manifest portableBackupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil || manifest.Format != "openreader-portable-backup" || manifest.Version != 1 {
		return nil, errInvalidPortableBackup
	}
	if err := validatePortableManifest(manifest, files); err != nil {
		return nil, err
	}

	logicalEntries := make(map[string][]byte)
	for key, file := range files {
		if key == portableBackupManifestName || strings.HasPrefix(key, "local-books/") {
			continue
		}
		if !portableLogicalEntryName(key) {
			return nil, errInvalidPortableBackup
		}
		data, err := readPortableZipEntry(file, s.backupRestoreLimits().MaxEntryBytes)
		if err != nil {
			return nil, err
		}
		name, err := normalizeBackupArchivePath(file.Name)
		if err != nil {
			return nil, errInvalidPortableBackup
		}
		logicalEntries[name] = data
	}
	if _, ok := logicalEntries["bookshelf.json"]; !ok {
		return nil, errInvalidPortableBackup
	}
	if err := validatePortableShelfMappings(logicalEntries["bookshelf.json"], manifest); err != nil {
		return nil, err
	}
	if err := s.validatePortableCollisions(userID, manifest); err != nil {
		return nil, err
	}

	stagingRoot := filepath.Join(s.cfg.CacheDir, "portable-restores", fmt.Sprintf("%d", userID))
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return nil, err
	}
	stagingDir, err := os.MkdirTemp(stagingRoot, "restore-*")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	portableImporter := s.portableLocalBookImporter(limits)
	staged := make([]portableStagedBook, 0, len(manifest.Books))
	for index, book := range manifest.Books {
		file := files[strings.ToLower(book.Entry)]
		stagePath := filepath.Join(stagingDir, fmt.Sprintf("b%04d%s", index+1, book.Extension))
		if err := copyAndValidatePortableEntry(file, stagePath, book, limits.maxEntryBytes); err != nil {
			return nil, err
		}
		reuse, err := s.reusePortableArchiveIfIdentical(userID, book)
		if err != nil {
			return nil, err
		}
		data, err := readPortableStagedFile(stagePath, limits.maxEntryBytes)
		if err != nil {
			return nil, err
		}
		if _, err := portableImporter.Preview(localbook.ImportRequest{
			UserID: userID, UserName: username, FileName: filepath.Base(stagePath), Extension: book.Extension,
			Data: data, Title: book.Title, Author: book.Author, TOCRule: book.TOCRule,
		}); err != nil {
			return nil, fmt.Errorf("%w: local archive cannot be parsed", errInvalidPortableBackup)
		}
		staged = append(staged, portableStagedBook{manifest: book, path: stagePath, reuse: reuse})
	}
	logicalData, err := makePortableLogicalZIP(logicalEntries)
	if err != nil {
		return nil, err
	}
	cleanup = false
	return &portableBackupPackage{logicalData: logicalData, books: staged, stagingDir: stagingDir}, nil
}

// portableLocalBookImporter keeps the parser budget aligned with the independently bounded
// portable archive entry. A package that passes portable preflight must not later be rejected
// merely because the ordinary interactive-upload cap is lower.
func (s *Server) portableLocalBookImporter(limits portableBackupLimits) localbook.Importer {
	cfg := s.cfg
	if limits.maxEntryBytes > cfg.MaxImportBytes {
		cfg.MaxImportBytes = limits.maxEntryBytes
	}
	return localbook.NewImporter(cfg, s.db)
}

func validatePortableManifest(manifest portableBackupManifest, files map[string]*zip.File) error {
	seenURL := make(map[string]struct{}, len(manifest.Books))
	seenEntry := make(map[string]struct{}, len(manifest.Books))
	for _, book := range manifest.Books {
		if strings.TrimSpace(book.BookURL) == "" || !strings.HasPrefix(book.BookURL, "local://") ||
			strings.TrimSpace(book.Title) == "" || !isImportableExtension(strings.ToLower(book.Extension)) ||
			book.Size < 0 || len(book.SHA256) != sha256.Size*2 {
			return errInvalidPortableBackup
		}
		if _, err := hex.DecodeString(book.SHA256); err != nil {
			return errInvalidPortableBackup
		}
		entry, err := normalizeBackupArchivePath(book.Entry)
		if err != nil || entry != book.Entry || !portableArchiveEntryName(entry, book.Extension) {
			return errInvalidPortableBackup
		}
		key := strings.ToLower(entry)
		if _, exists := files[key]; !exists {
			return errInvalidPortableBackup
		}
		if _, exists := seenURL[book.BookURL]; exists {
			return errInvalidPortableBackup
		}
		if _, exists := seenEntry[key]; exists {
			return errInvalidPortableBackup
		}
		seenURL[book.BookURL] = struct{}{}
		seenEntry[key] = struct{}{}
	}
	return nil
}

func portableArchiveEntryName(entry, extension string) bool {
	parts := strings.Split(entry, "/")
	if len(parts) != 3 || parts[0] != "local-books" || len(parts[1]) != 5 || !strings.HasPrefix(parts[1], "b") {
		return false
	}
	for _, character := range parts[1][1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return parts[2] == "original"+strings.ToLower(extension)
}

func portableLogicalEntryName(name string) bool {
	switch name {
	case "booksource.json", "rsssources.json", "usersettings.json", "categories.json", "bookgroup.json", "bookshelf.json", "chaptervariables.json", "bookmarks.json", "readingprogress.json", "replacerules.json":
		return true
	default:
		return false
	}
}

func validatePortableShelfMappings(data []byte, manifest portableBackupManifest) error {
	var rows []restoredBookshelfRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return errInvalidPortableBackup
	}
	localURLs := make(map[string]struct{})
	for _, row := range rows {
		if row.SourceID != 0 || row.Type == 1 {
			continue
		}
		bookURL := strings.TrimSpace(row.URL)
		if bookURL == "" {
			bookURL = strings.TrimSpace(row.BookURL)
		}
		if bookURL == "" || !strings.HasPrefix(bookURL, "local://") {
			return errInvalidPortableBackup
		}
		localURLs[bookURL] = struct{}{}
	}
	if len(localURLs) != len(manifest.Books) {
		return errInvalidPortableBackup
	}
	for _, book := range manifest.Books {
		if _, exists := localURLs[book.BookURL]; !exists {
			return errInvalidPortableBackup
		}
	}
	return nil
}

func (s *Server) validatePortableCollisions(userID uint, manifest portableBackupManifest) error {
	for _, entry := range manifest.Books {
		var book models.Book
		err := s.db.Where("user_id = ? AND url = ?", userID, entry.BookURL).First(&book).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		path, ok := s.localBookSourcePath(book)
		if !ok {
			return errPortableBackupConflict
		}
		digest, size, err := portableFileDigest(path)
		if err != nil || size != entry.Size || !strings.EqualFold(digest, entry.SHA256) {
			return errPortableBackupConflict
		}
	}
	return nil
}

func (s *Server) reusePortableArchiveIfIdentical(userID uint, entry portableBackupManifestBook) (bool, error) {
	var book models.Book
	err := s.db.Where("user_id = ? AND url = ?", userID, entry.BookURL).First(&book).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	path, ok := s.localBookSourcePath(book)
	if !ok {
		return false, errPortableBackupConflict
	}
	digest, size, err := portableFileDigest(path)
	if err != nil || size != entry.Size || !strings.EqualFold(digest, entry.SHA256) {
		return false, errPortableBackupConflict
	}
	return true, nil
}

func copyAndValidatePortableEntry(file *zip.File, target string, manifest portableBackupManifestBook, maxBytes int64) error {
	if file == nil || manifest.Size > maxBytes {
		return errPortableBackupLimit
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = output.Close()
		if cleanup {
			_ = os.Remove(target)
		}
	}()
	input, err := file.Open()
	if err != nil {
		return errInvalidPortableBackup
	}
	defer input.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, maxBytes+1))
	if err != nil || written > maxBytes {
		return errPortableBackupLimit
	}
	if written != manifest.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), manifest.SHA256) {
		return errInvalidPortableBackup
	}
	if err := output.Close(); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func readPortableZipEntry(file *zip.File, maxBytes int64) ([]byte, error) {
	if file == nil || maxBytes <= 0 || file.UncompressedSize64 > uint64(maxBytes) {
		return nil, errPortableBackupLimit
	}
	input, err := file.Open()
	if err != nil {
		return nil, errInvalidPortableBackup
	}
	defer input.Close()
	data, err := io.ReadAll(io.LimitReader(input, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return nil, errPortableBackupLimit
	}
	return data, nil
}

func readPortableStagedFile(path string, configuredLimit int64) ([]byte, error) {
	limit := configuredLimit
	if limit <= 0 {
		limit = 128 * 1024 * 1024
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errInvalidPortableBackup
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errPortableBackupLimit
	}
	return data, nil
}

func makePortableLogicalZIP(entries map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(entries[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func portableFileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
