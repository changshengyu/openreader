// Package umdcover serves the cover reader-dev shows for a local UMD book:
// umdlib UmdReader keeps the additional section after segment 130. The image
// is derived data next to the archived book (.umd-resources/<fingerprint>/)
// and is reachable only through a signed same-origin capability URL that
// never enters the Book row, archives or backups.
package umdcover

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/webdavfs"
)

const (
	extractionDirectoryName   = ".umd-resources"
	extractionMarkerName      = ".openreader-complete"
	resourceCapabilityPurpose = "openreader:umd-cover:v1"
	resourceCapabilityTTL     = 12 * time.Hour
	// A mounted legacy UMD may exceed today's upload limit; larger files are
	// not scanned and simply keep the default cover.
	maxUMDSourceBytes = 256 << 20
	maxCoverBytes     = 8 << 20
	maxMarkerBytes    = 4 << 10
)

var (
	ErrMalformedCapability = errors.New("malformed UMD cover capability")
	ErrInvalidCapability   = errors.New("invalid UMD cover capability")
	ErrExpiredCapability   = errors.New("expired UMD cover capability")
	ErrUnsafePath          = errors.New("unsafe UMD cover path")
	ErrNotFound            = errors.New("UMD cover not found")
	ErrNotUMD              = errors.New("not a local UMD book")
)

// coverNames maps each served cover type to its file name in an extraction.
var coverNames = map[string]string{
	"image/jpeg": "cover.jpg",
	"image/png":  "cover.png",
	"image/gif":  "cover.gif",
	"image/webp": "cover.webp",
}

type Service struct {
	cfg          config.Config
	db           *gorm.DB
	now          func() time.Time
	extractLocks sync.Map
}

type PreparedCover struct {
	ResourceURL string
	ExpiresAt   time.Time
}

type Resource struct {
	File        *os.File
	Info        os.FileInfo
	Name        string
	ContentType string
}

// extractionMarker completes an extraction. It is written also when the book
// has no cover (CoverName empty) so the archive is scanned once per version.
type extractionMarker struct {
	Fingerprint string `json:"fingerprint"`
	Size        int64  `json:"size"`
	ModTimeNano int64  `json:"modTimeNano"`
	CoverName   string `json:"coverName,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

type resourceClaims struct {
	UserID      uint   `json:"u"`
	BookID      uint   `json:"b"`
	Fingerprint string `json:"f"`
	Purpose     string `json:"p"`
	ExpiresAt   int64  `json:"e"`
}

// bookFiles locates one book's archive: bookRoot is its library directory and
// sourceName the UMD file relative to it.
type bookFiles struct {
	libraryRoot string
	bookRoot    string
	sourceName  string
}

func New(cfg config.Config, database *gorm.DB) *Service {
	return &Service{cfg: cfg, db: database, now: time.Now}
}

func IsLocalUMD(book models.Book) bool {
	if book.SourceID != 0 {
		return false
	}
	for _, candidate := range []string{book.OriginalFile, book.URL} {
		if strings.EqualFold(filepath.Ext(strings.TrimSpace(candidate)), ".umd") {
			return true
		}
	}
	return false
}

// PrepareCover returns the response-only cover URL of a local UMD book, or
// ErrNotFound when it has none. The first call for a book version reads the
// archive once and records the result next to it.
func (s *Service) PrepareCover(book models.Book) (PreparedCover, error) {
	if !IsLocalUMD(book) {
		return PreparedCover{}, ErrNotUMD
	}
	files, err := s.bookFiles(book)
	if err != nil {
		return PreparedCover{}, err
	}
	marker, err := s.ensureExtraction(files)
	if err != nil {
		return PreparedCover{}, err
	}
	if marker.CoverName == "" {
		return PreparedCover{}, ErrNotFound
	}
	expiresAt := s.now().UTC().Add(resourceCapabilityTTL)
	capability, err := signResourceCapability(s.cfg.JWTSecret, resourceClaims{
		UserID:      book.UserID,
		BookID:      book.ID,
		Fingerprint: marker.Fingerprint,
		Purpose:     resourceCapabilityPurpose,
		ExpiresAt:   expiresAt.Unix(),
	})
	if err != nil {
		return PreparedCover{}, err
	}
	return PreparedCover{ResourceURL: "/api/umd-cover/" + url.PathEscape(capability), ExpiresAt: expiresAt}, nil
}

// OpenCover authorizes a capability against the live book row and returns the
// cover of the signed book version.
func (s *Service) OpenCover(capability string) (Resource, error) {
	claims, err := verifyResourceCapability(s.cfg.JWTSecret, capability, s.now().UTC())
	if err != nil {
		return Resource{}, err
	}
	var book models.Book
	if err := s.db.Where("id = ? AND user_id = ?", claims.BookID, claims.UserID).First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	if !IsLocalUMD(book) {
		return Resource{}, ErrNotFound
	}
	files, err := s.bookFiles(book)
	var marker extractionMarker
	switch {
	case err == nil:
		// Rebuilds a removed extraction and notices a replaced archive.
		marker, err = s.ensureExtraction(files)
		if err != nil {
			return Resource{}, err
		}
		if marker.Fingerprint != claims.Fingerprint {
			return Resource{}, ErrInvalidCapability
		}
	case errors.Is(err, ErrNotFound) && files.bookRoot != "":
		marker, err = s.readMarker(files, claims.Fingerprint)
		if err != nil {
			return Resource{}, err
		}
	default:
		return Resource{}, err
	}
	if marker.CoverName == "" {
		return Resource{}, ErrNotFound
	}
	storage, err := webdavfs.NewScoped(files.libraryRoot, files.bookRoot)
	if err != nil {
		return Resource{}, mapOpenError(err)
	}
	file, info, err := storage.Open(extractionDirectoryName + "/" + marker.Fingerprint + "/" + marker.CoverName)
	if err != nil {
		return Resource{}, mapOpenError(err)
	}
	return Resource{File: file, Info: info, Name: marker.CoverName, ContentType: marker.ContentType}, nil
}

// ensureExtraction reuses the extraction recorded for the archive's current
// size and modification time, and otherwise reads the archive once, writes
// its cover (if any) and marker into a fresh directory and drops older ones.
func (s *Service) ensureExtraction(files bookFiles) (extractionMarker, error) {
	lockValue, _ := s.extractLocks.LoadOrStore(filepath.Join(files.bookRoot, files.sourceName), &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	storage, err := webdavfs.NewScoped(files.libraryRoot, files.bookRoot)
	if err != nil {
		return extractionMarker{}, mapOpenError(err)
	}
	source, info, err := storage.Open(files.sourceName)
	if err != nil {
		return extractionMarker{}, mapOpenError(err)
	}
	defer source.Close()
	if !info.Mode().IsRegular() {
		return extractionMarker{}, ErrNotFound
	}
	if marker, ok := s.reusableExtraction(files, info); ok {
		return marker, nil
	}
	if info.Size() <= 0 || info.Size() > maxUMDSourceBytes {
		return extractionMarker{}, ErrNotFound
	}
	data, err := io.ReadAll(io.LimitReader(source, maxUMDSourceBytes+1))
	if err != nil {
		return extractionMarker{}, err
	}
	if int64(len(data)) != info.Size() {
		// The archive changed while it was read; try again on a later request.
		return extractionMarker{}, ErrNotFound
	}
	sum := sha256.Sum256(data)
	marker := extractionMarker{
		Fingerprint: hex.EncodeToString(sum[:]),
		Size:        info.Size(),
		ModTimeNano: info.ModTime().UnixNano(),
	}

	parent := filepath.Join(files.bookRoot, extractionDirectoryName)
	// Mkdir, not MkdirAll: a book directory removed by a concurrent delete
	// must not be recreated.
	if err := os.Mkdir(parent, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		if errors.Is(err, os.ErrNotExist) {
			return extractionMarker{}, ErrNotFound
		}
		return extractionMarker{}, err
	}
	if _, err := storage.Stat(extractionDirectoryName); err != nil {
		return extractionMarker{}, mapOpenError(err)
	}
	finalRoot := filepath.Join(parent, marker.Fingerprint)
	if existing, err := s.readMarker(files, marker.Fingerprint); err == nil && coverPresent(storage, existing) {
		// Same content with a new modification time: only refresh the marker.
		existing.Size, existing.ModTimeNano = marker.Size, marker.ModTimeNano
		if err := writeMarker(finalRoot, existing); err != nil {
			return extractionMarker{}, err
		}
		pruneOtherExtractions(parent, marker.Fingerprint)
		return existing, nil
	}
	staging, err := os.MkdirTemp(parent, ".extract-")
	if err != nil {
		return extractionMarker{}, err
	}
	defer os.RemoveAll(staging)
	if contentType, image, ok := engine.UMDCoverImage(data, maxCoverBytes); ok {
		if name := coverNames[contentType]; name != "" {
			if err := writeFileSynced(filepath.Join(staging, name), image); err != nil {
				return extractionMarker{}, err
			}
			marker.CoverName, marker.ContentType = name, contentType
		}
	}
	if err := writeMarker(staging, marker); err != nil {
		return extractionMarker{}, err
	}
	if err := os.RemoveAll(finalRoot); err != nil {
		return extractionMarker{}, err
	}
	if err := os.Rename(staging, finalRoot); err != nil {
		return extractionMarker{}, err
	}
	pruneOtherExtractions(parent, marker.Fingerprint)
	return marker, nil
}

func (s *Service) reusableExtraction(files bookFiles, info os.FileInfo) (extractionMarker, bool) {
	entries, err := os.ReadDir(filepath.Join(files.bookRoot, extractionDirectoryName))
	if err != nil {
		return extractionMarker{}, false
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validFingerprint(entry.Name()) {
			continue
		}
		marker, err := s.readMarker(files, entry.Name())
		if err != nil || marker.Size != info.Size() || marker.ModTimeNano != info.ModTime().UnixNano() {
			continue
		}
		storage, err := webdavfs.NewScoped(files.libraryRoot, files.bookRoot)
		if err != nil || !coverPresent(storage, marker) {
			return extractionMarker{}, false
		}
		return marker, true
	}
	return extractionMarker{}, false
}

// coverPresent reports whether a marker's cover file still exists, so an
// extraction whose image was removed is rebuilt rather than reused.
func coverPresent(storage *webdavfs.Service, marker extractionMarker) bool {
	if marker.CoverName == "" {
		return true
	}
	resource, err := storage.Stat(extractionDirectoryName + "/" + marker.Fingerprint + "/" + marker.CoverName)
	return err == nil && resource.Info != nil && resource.Info.Mode().IsRegular()
}

// readMarker reads the completed extraction of one fingerprint without
// following symbolic links.
func (s *Service) readMarker(files bookFiles, fingerprint string) (extractionMarker, error) {
	if !validFingerprint(fingerprint) {
		return extractionMarker{}, ErrInvalidCapability
	}
	storage, err := webdavfs.NewScoped(files.libraryRoot, files.bookRoot)
	if err != nil {
		return extractionMarker{}, mapOpenError(err)
	}
	file, _, err := storage.Open(extractionDirectoryName + "/" + fingerprint + "/" + extractionMarkerName)
	if err != nil {
		return extractionMarker{}, mapOpenError(err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMarkerBytes+1))
	if err != nil {
		return extractionMarker{}, err
	}
	var marker extractionMarker
	if len(data) > maxMarkerBytes || json.Unmarshal(data, &marker) != nil {
		return extractionMarker{}, ErrNotFound
	}
	if marker.Fingerprint != fingerprint || marker.Size <= 0 || marker.ModTimeNano == 0 {
		return extractionMarker{}, ErrNotFound
	}
	if marker.CoverName != "" || marker.ContentType != "" {
		if coverNames[marker.ContentType] == "" || coverNames[marker.ContentType] != marker.CoverName {
			return extractionMarker{}, ErrNotFound
		}
	}
	return marker, nil
}

// bookFiles resolves a book's library directory like the other local-book
// readers and finds its archived UMD there. When only the archive is missing
// the returned bookFiles still carries the directory.
func (s *Service) bookFiles(book models.Book) (bookFiles, error) {
	libraryRoot, err := canonicalPath(s.cfg.LibraryDir)
	if err != nil {
		return bookFiles{}, err
	}
	original := strings.TrimSpace(book.OriginalFile)
	libraryPathValue := strings.TrimSpace(book.LibraryPath)
	if libraryPathValue == "" && original != "" && !filepath.IsAbs(original) {
		libraryPathValue = filepath.Dir(original)
	}
	if libraryPathValue == "" {
		libraryPathValue = "."
	}
	libraryPath := filepath.Clean(libraryPathValue)
	if filepath.IsAbs(libraryPath) || libraryPath == ".." || strings.HasPrefix(libraryPath, ".."+string(filepath.Separator)) {
		return bookFiles{}, ErrUnsafePath
	}
	files := bookFiles{libraryRoot: libraryRoot, bookRoot: filepath.Join(libraryRoot, libraryPath)}
	storage, err := webdavfs.NewScoped(files.libraryRoot, files.bookRoot)
	if err != nil {
		return bookFiles{}, mapOpenError(err)
	}
	candidates := make([]string, 0, 2)
	if original != "" {
		if !filepath.IsAbs(original) {
			if relative, err := filepath.Rel(libraryPath, filepath.Clean(original)); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				candidates = append(candidates, filepath.ToSlash(relative))
			}
		}
		candidates = append(candidates, filepath.Base(original))
	}
	for _, candidate := range candidates {
		if isUMDFile(storage, candidate) {
			files.sourceName = candidate
			return files, nil
		}
	}
	entries, err := os.ReadDir(files.bookRoot)
	if err != nil {
		return files, mapOpenError(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && isUMDFile(storage, entry.Name()) {
			files.sourceName = entry.Name()
			return files, nil
		}
	}
	return files, ErrNotFound
}

func isUMDFile(storage *webdavfs.Service, relative string) bool {
	if !strings.EqualFold(filepath.Ext(relative), ".umd") {
		return false
	}
	resource, err := storage.Stat(relative)
	return err == nil && resource.Info != nil && resource.Info.Mode().IsRegular()
}

func pruneOtherExtractions(parent, currentFingerprint string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != currentFingerprint && validFingerprint(entry.Name()) {
			_ = os.RemoveAll(filepath.Join(parent, entry.Name()))
		}
	}
}

func writeMarker(directory string, marker extractionMarker) error {
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(directory, ".marker-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, filepath.Join(directory, extractionMarkerName))
}

func writeFileSynced(filePath string, data []byte) error {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func mapOpenError(err error) error {
	switch {
	case errors.Is(err, webdavfs.ErrNotFound), errors.Is(err, webdavfs.ErrIsDirectory), errors.Is(err, os.ErrNotExist):
		return ErrNotFound
	case errors.Is(err, webdavfs.ErrUnsafePath), errors.Is(err, webdavfs.ErrNotDirectory):
		return ErrUnsafePath
	default:
		return err
	}
}

func validFingerprint(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func signResourceCapability(secret string, claims resourceClaims) (string, error) {
	if err := validateResourceClaims(claims); err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(capabilitySignature(secret, encodedPayload)), nil
}

func verifyResourceCapability(secret, token string, now time.Time) (resourceClaims, error) {
	var claims resourceClaims
	if len(token) == 0 || len(token) > 2048 {
		return claims, ErrMalformedCapability
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return claims, ErrMalformedCapability
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrMalformedCapability
	}
	if !hmac.Equal(signature, capabilitySignature(secret, parts[0])) {
		return claims, ErrInvalidCapability
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, ErrMalformedCapability
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return resourceClaims{}, ErrMalformedCapability
	}
	if err := validateResourceClaims(claims); err != nil {
		return resourceClaims{}, ErrInvalidCapability
	}
	if now.Unix() >= claims.ExpiresAt {
		return resourceClaims{}, ErrExpiredCapability
	}
	return claims, nil
}

func validateResourceClaims(claims resourceClaims) error {
	if claims.UserID == 0 || claims.BookID == 0 || claims.Purpose != resourceCapabilityPurpose ||
		claims.ExpiresAt <= 0 || !validFingerprint(claims.Fingerprint) {
		return ErrInvalidCapability
	}
	return nil
}

func capabilitySignature(secret, payload string) []byte {
	derivation := hmac.New(sha256.New, []byte(secret))
	_, _ = derivation.Write([]byte(resourceCapabilityPurpose))
	mac := hmac.New(sha256.New, derivation.Sum(nil))
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return filepath.Clean(abs), nil
		}
		return "", err
	}
	return filepath.Clean(resolved), nil
}
