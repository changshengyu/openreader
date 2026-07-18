package cbzreader

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	extractionDirectoryName   = ".cbz-resources"
	extractionMarkerName      = ".openreader-complete"
	resourceCapabilityPurpose = "openreader:cbz-resource:v1"
	resourceCapabilityTTL     = 12 * time.Hour
	maxCBZArchiveBytes        = 1 << 30
	maxCBZEntries             = 20_000
	maxCBZEntryBytes          = 128 << 20
	maxCBZTotalBytes          = 2 << 30
)

var (
	ErrMalformedCapability = errors.New("malformed CBZ resource capability")
	ErrInvalidCapability   = errors.New("invalid CBZ resource capability")
	ErrExpiredCapability   = errors.New("expired CBZ resource capability")
	ErrUnsafePath          = errors.New("unsafe CBZ resource path")
	ErrNotFound            = errors.New("CBZ resource not found")
	ErrUnsupportedMedia    = errors.New("unsupported CBZ media type")
	ErrInvalidArchive      = errors.New("invalid CBZ archive")
	ErrExtractionLimit     = errors.New("CBZ archive exceeds safe limits")
	ErrNotCBZ              = errors.New("not a local CBZ book")
)

type Service struct {
	cfg          config.Config
	db           *gorm.DB
	now          func() time.Time
	limits       extractionLimits
	extractLocks sync.Map
	fingerprint  func(string) (string, error)
}

type PreparedChapter struct {
	ResourceURL string
	ExpiresAt   time.Time
}

type Resource struct {
	Path        string
	ContentType string
}

type extractionMarker struct {
	Fingerprint string `json:"fingerprint"`
	Size        int64  `json:"size"`
	ModTimeNano int64  `json:"modTimeNano"`
	CoverPath   string `json:"coverPath"`
}

type resourceClaims struct {
	UserID      uint   `json:"u"`
	BookID      uint   `json:"b"`
	Fingerprint string `json:"f"`
	Purpose     string `json:"p"`
	ExpiresAt   int64  `json:"e"`
}

func New(cfg config.Config, database *gorm.DB) *Service {
	return &Service{
		cfg:         cfg,
		db:          database,
		now:         time.Now,
		limits:      extractionLimitsFromConfig(cfg),
		fingerprint: fingerprintFile,
	}
}

func IsLocalCBZ(book models.Book) bool {
	if book.SourceID != 0 {
		return false
	}
	for _, candidate := range []string{book.OriginalFile, book.URL, book.LibraryPath} {
		if strings.EqualFold(filepath.Ext(strings.TrimSpace(candidate)), ".cbz") {
			return true
		}
	}
	return false
}

// PrepareBookResources eagerly creates the bounded immutable image tree for a
// newly allocated local-book archive. The importer owns that archive until its
// database transaction commits, so its existing compensation can remove both
// the source and this derived generation on failure.
func (s *Service) PrepareBookResources(book models.Book) error {
	if !IsLocalCBZ(book) {
		return ErrNotCBZ
	}
	sourcePath, bookRoot, err := s.sourcePath(book)
	if err != nil {
		return err
	}
	_, _, err = s.ensureExtraction(sourcePath, bookRoot)
	return err
}

func (s *Service) PrepareChapter(book models.Book, chapter *models.Chapter) (PreparedChapter, error) {
	if chapter == nil || chapter.BookID != book.ID || !IsLocalCBZ(book) {
		return PreparedChapter{}, ErrNotCBZ
	}
	sourcePath, bookRoot, err := s.sourcePath(book)
	if err != nil {
		return PreparedChapter{}, err
	}
	fingerprint, extractionRoot, err := s.ensureExtraction(sourcePath, bookRoot)
	if err != nil {
		return PreparedChapter{}, err
	}
	resourcePath := strings.TrimSpace(chapter.ResourcePath)
	if resourcePath == "" {
		resourcePath, err = s.recoverChapterResourcePath(extractionRoot, chapter.Index)
		if err != nil {
			return PreparedChapter{}, err
		}
		chapter.ResourcePath = resourcePath
		if err := s.db.Model(chapter).Update("resource_path", resourcePath).Error; err != nil {
			return PreparedChapter{}, err
		}
	}
	resourcePath, err = engine.NormalizeCBZResourcePath(resourcePath)
	if err != nil || resourcePath == "" {
		return PreparedChapter{}, ErrUnsafePath
	}
	if _, err := s.resourceFile(extractionRoot, resourcePath); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return PreparedChapter{}, err
		}
		fingerprint, extractionRoot, err = s.rebuildExtraction(sourcePath, bookRoot)
		if err != nil {
			return PreparedChapter{}, err
		}
		if _, err := s.resourceFile(extractionRoot, resourcePath); err != nil {
			return PreparedChapter{}, err
		}
	}
	return s.prepareResource(book, fingerprint, resourcePath)
}

// PrepareCover exposes reader-dev's first-safe-image CBZ cover through the
// existing same-origin resource capability. The generated URL is response
// data only: callers must not save it to the Book row or archived metadata.
func (s *Service) PrepareCover(book models.Book) (PreparedChapter, error) {
	if !IsLocalCBZ(book) {
		return PreparedChapter{}, ErrNotCBZ
	}
	sourcePath, bookRoot, err := s.sourcePath(book)
	if err != nil {
		return PreparedChapter{}, err
	}
	fingerprint, extractionRoot, err := s.ensureExtraction(sourcePath, bookRoot)
	if err != nil {
		return PreparedChapter{}, err
	}
	marker, err := readExtractionMarker(filepath.Join(extractionRoot, extractionMarkerName))
	if err != nil || marker.CoverPath == "" {
		return PreparedChapter{}, ErrNotFound
	}
	if _, err := s.resourceFile(extractionRoot, marker.CoverPath); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return PreparedChapter{}, err
		}
		fingerprint, extractionRoot, err = s.rebuildExtraction(sourcePath, bookRoot)
		if err != nil {
			return PreparedChapter{}, err
		}
		marker, err = readExtractionMarker(filepath.Join(extractionRoot, extractionMarkerName))
		if err != nil || marker.CoverPath == "" {
			return PreparedChapter{}, ErrNotFound
		}
		if _, err := s.resourceFile(extractionRoot, marker.CoverPath); err != nil {
			return PreparedChapter{}, err
		}
	}
	return s.prepareResource(book, fingerprint, marker.CoverPath)
}

func (s *Service) prepareResource(book models.Book, fingerprint, resourcePath string) (PreparedChapter, error) {
	resourcePath, err := engine.NormalizeCBZResourcePath(resourcePath)
	if err != nil || resourcePath == "" {
		return PreparedChapter{}, ErrUnsafePath
	}
	if _, ok := engine.CBZImageContentType(resourcePath); !ok {
		return PreparedChapter{}, ErrUnsupportedMedia
	}

	expiresAt := s.now().UTC().Add(resourceCapabilityTTL)
	capability, err := signResourceCapability(s.cfg.JWTSecret, resourceClaims{
		UserID:      book.UserID,
		BookID:      book.ID,
		Fingerprint: fingerprint,
		Purpose:     resourceCapabilityPurpose,
		ExpiresAt:   expiresAt.Unix(),
	})
	if err != nil {
		return PreparedChapter{}, err
	}
	return PreparedChapter{
		ResourceURL: "/api/cbz-resource/" + url.PathEscape(capability) + "/" + escapeResourcePath(resourcePath),
		ExpiresAt:   expiresAt,
	}, nil
}

func (s *Service) OpenResource(capability, requestedPath string) (Resource, error) {
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
	if !IsLocalCBZ(book) {
		return Resource{}, ErrNotFound
	}
	sourcePath, bookRoot, sourceErr := s.sourcePath(book)
	if sourceErr != nil {
		if !errors.Is(sourceErr, ErrNotFound) {
			return Resource{}, sourceErr
		}
		_, bookRoot, err = s.bookRoots(book)
		if err != nil {
			return Resource{}, err
		}
		sourcePath = ""
	}
	extractionRoot, err := s.extractionForFingerprint(bookRoot, claims.Fingerprint)
	if errors.Is(err, ErrNotFound) {
		if sourcePath == "" {
			return Resource{}, ErrNotFound
		}
		fingerprint, rebuiltRoot, rebuildErr := s.ensureExtraction(sourcePath, bookRoot)
		if rebuildErr != nil {
			return Resource{}, rebuildErr
		}
		if !strings.EqualFold(fingerprint, claims.Fingerprint) {
			return Resource{}, ErrInvalidCapability
		}
		extractionRoot = rebuiltRoot
		err = nil
	}
	if err != nil {
		return Resource{}, err
	}
	if sourcePath != "" {
		if err := s.validateExtractionSource(extractionRoot, claims.Fingerprint, sourcePath); err != nil {
			return Resource{}, err
		}
	}
	resourcePath, err := engine.NormalizeCBZResourcePath(strings.TrimPrefix(requestedPath, "/"))
	if err != nil || resourcePath == "" {
		return Resource{}, ErrUnsafePath
	}
	contentType, ok := engine.CBZImageContentType(resourcePath)
	if !ok {
		return Resource{}, ErrUnsupportedMedia
	}
	filePath, err := s.resourceFile(extractionRoot, resourcePath)
	if errors.Is(err, ErrNotFound) && sourcePath != "" {
		fingerprint, rebuiltRoot, rebuildErr := s.rebuildExtraction(sourcePath, bookRoot)
		if rebuildErr != nil {
			return Resource{}, rebuildErr
		}
		if !strings.EqualFold(fingerprint, claims.Fingerprint) {
			return Resource{}, ErrInvalidCapability
		}
		filePath, err = s.resourceFile(rebuiltRoot, resourcePath)
	}
	if err != nil {
		return Resource{}, err
	}
	return Resource{Path: filePath, ContentType: contentType}, nil
}

func (s *Service) sourcePath(book models.Book) (string, string, error) {
	libraryRoot, bookRoot, err := s.bookRoots(book)
	if err != nil {
		return "", "", err
	}
	original := strings.TrimSpace(book.OriginalFile)
	candidates := make([]string, 0, 3)
	if original != "" && !filepath.IsAbs(original) {
		if candidate, joinErr := joinUnder(libraryRoot, original); joinErr == nil {
			candidates = append(candidates, candidate)
		}
	}
	if original != "" {
		candidates = append(candidates, filepath.Join(bookRoot, filepath.Base(original)))
	}
	if filepath.IsAbs(original) {
		if candidate, absErr := filepath.Abs(original); absErr == nil && withinPath(libraryRoot, candidate) {
			candidates = append(candidates, candidate)
		}
	}
	for _, candidate := range candidates {
		if validCBZSource(candidate, libraryRoot, bookRoot) {
			return candidate, bookRoot, nil
		}
	}
	entries, err := os.ReadDir(bookRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".cbz") {
			continue
		}
		candidate := filepath.Join(bookRoot, entry.Name())
		if validCBZSource(candidate, libraryRoot, bookRoot) {
			return candidate, bookRoot, nil
		}
	}
	return "", "", ErrNotFound
}

func (s *Service) bookRoots(book models.Book) (string, string, error) {
	libraryRoot, err := canonicalPath(s.cfg.LibraryDir)
	if err != nil {
		return "", "", err
	}
	original := strings.TrimSpace(book.OriginalFile)
	libraryPathValue := strings.TrimSpace(book.LibraryPath)
	if libraryPathValue == "" {
		switch {
		case original != "" && !filepath.IsAbs(original):
			libraryPathValue = filepath.Dir(original)
		case filepath.IsAbs(original):
			if canonicalOriginal, canonicalErr := canonicalPath(original); canonicalErr == nil && withinPath(libraryRoot, canonicalOriginal) {
				if relative, relativeErr := filepath.Rel(libraryRoot, filepath.Dir(canonicalOriginal)); relativeErr == nil {
					libraryPathValue = relative
				}
			}
		}
	}
	if strings.TrimSpace(libraryPathValue) == "" {
		libraryPathValue = "."
	}
	libraryPath := filepath.Clean(libraryPathValue)
	if filepath.IsAbs(libraryPath) || libraryPath == ".." || strings.HasPrefix(libraryPath, ".."+string(filepath.Separator)) {
		return "", "", ErrUnsafePath
	}
	bookRoot, err := joinUnder(libraryRoot, libraryPath)
	if err != nil {
		return "", "", err
	}
	return libraryRoot, bookRoot, nil
}

func validCBZSource(candidate, libraryRoot, bookRoot string) bool {
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(candidate), ".cbz") {
		return false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	return withinPath(libraryRoot, resolved) && withinPath(bookRoot, resolved)
}

func (s *Service) ensureExtraction(sourcePath, bookRoot string) (string, string, error) {
	return s.ensureExtractionMode(sourcePath, bookRoot, false)
}

func (s *Service) rebuildExtraction(sourcePath, bookRoot string) (string, string, error) {
	return s.ensureExtractionMode(sourcePath, bookRoot, true)
}

func (s *Service) ensureExtractionMode(sourcePath, bookRoot string, forceRebuild bool) (string, string, error) {
	lockValue, _ := s.extractLocks.LoadOrStore(sourcePath, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if !forceRebuild {
		if fingerprint, extractionRoot, ok := s.reusableExtraction(sourcePath, bookRoot); ok {
			return fingerprint, extractionRoot, nil
		}
	}

	fingerprint, err := s.fingerprint(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	parent, err := joinUnder(bookRoot, extractionDirectoryName)
	if err != nil {
		return "", "", err
	}
	finalRoot, err := joinUnder(parent, fingerprint)
	if err != nil {
		return "", "", err
	}
	markerPath := filepath.Join(finalRoot, extractionMarkerName)
	if marker, readErr := readExtractionMarker(markerPath); readErr == nil && marker.Fingerprint == fingerprint && !forceRebuild {
		if info, statErr := os.Stat(sourcePath); statErr == nil && (marker.Size != info.Size() || marker.ModTimeNano != info.ModTime().UnixNano()) {
			if err := writeExtractionMarker(markerPath, fingerprint, sourcePath, marker.CoverPath); err != nil {
				return "", "", err
			}
		}
		s.pruneOtherExtractions(parent, fingerprint)
		return fingerprint, finalRoot, nil
	}
	if err := os.RemoveAll(finalRoot); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", "", err
	}
	staging, err := os.MkdirTemp(parent, ".extract-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(staging)
	coverPath, err := extractArchiveFile(sourcePath, staging, s.limits)
	if err != nil {
		return "", "", err
	}
	if err := writeExtractionMarker(filepath.Join(staging, extractionMarkerName), fingerprint, sourcePath, coverPath); err != nil {
		return "", "", err
	}
	if err := os.Rename(staging, finalRoot); err != nil {
		if marker, readErr := readExtractionMarker(markerPath); readErr == nil && marker.Fingerprint == fingerprint {
			return fingerprint, finalRoot, nil
		}
		return "", "", err
	}
	s.pruneOtherExtractions(parent, fingerprint)
	return fingerprint, finalRoot, nil
}

func (s *Service) pruneOtherExtractions(parent, currentFingerprint string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, entry := range entries {
		fingerprint := strings.ToLower(strings.TrimSpace(entry.Name()))
		if !entry.IsDir() || fingerprint == currentFingerprint || len(fingerprint) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(fingerprint); err != nil {
			continue
		}
		extractionRoot, err := joinUnder(parent, fingerprint)
		if err != nil {
			continue
		}
		marker, err := readExtractionMarker(filepath.Join(extractionRoot, extractionMarkerName))
		if err == nil && marker.Fingerprint == fingerprint {
			_ = os.RemoveAll(extractionRoot)
		}
	}
}

func (s *Service) reusableExtraction(sourcePath, bookRoot string) (string, string, bool) {
	info, err := os.Stat(sourcePath)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", false
	}
	parent, err := joinUnder(bookRoot, extractionDirectoryName)
	if err != nil {
		return "", "", false
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fingerprint := strings.ToLower(strings.TrimSpace(entry.Name()))
		if len(fingerprint) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(fingerprint); err != nil {
			continue
		}
		extractionRoot, err := joinUnder(parent, fingerprint)
		if err != nil {
			continue
		}
		marker, err := readExtractionMarker(filepath.Join(extractionRoot, extractionMarkerName))
		if err != nil || marker.Fingerprint != fingerprint || marker.Size <= 0 || marker.ModTimeNano == 0 {
			continue
		}
		if marker.Size == info.Size() && marker.ModTimeNano == info.ModTime().UnixNano() {
			return fingerprint, extractionRoot, true
		}
	}
	return "", "", false
}

func (s *Service) extractionForFingerprint(bookRoot, fingerprint string) (string, error) {
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	if len(fingerprint) != sha256.Size*2 {
		return "", ErrInvalidCapability
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return "", ErrInvalidCapability
	}
	parent, err := joinUnder(bookRoot, extractionDirectoryName)
	if err != nil {
		return "", err
	}
	extractionRoot, err := joinUnder(parent, fingerprint)
	if err != nil {
		return "", err
	}
	marker, err := readExtractionMarker(filepath.Join(extractionRoot, extractionMarkerName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	if marker.Fingerprint != fingerprint {
		return "", ErrInvalidCapability
	}
	return extractionRoot, nil
}

func (s *Service) validateExtractionSource(extractionRoot, fingerprint, sourcePath string) error {
	markerPath := filepath.Join(extractionRoot, extractionMarkerName)
	marker, err := readExtractionMarker(markerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if marker.Size == info.Size() && marker.ModTimeNano == info.ModTime().UnixNano() {
		return nil
	}
	currentFingerprint, err := s.fingerprint(sourcePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(currentFingerprint, fingerprint) {
		return ErrInvalidCapability
	}
	return writeExtractionMarker(markerPath, currentFingerprint, sourcePath, marker.CoverPath)
}

func readExtractionMarker(markerPath string) (extractionMarker, error) {
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return extractionMarker{}, err
	}
	var marker extractionMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return extractionMarker{}, ErrInvalidCapability
	}
	marker.Fingerprint = strings.ToLower(strings.TrimSpace(marker.Fingerprint))
	if len(marker.Fingerprint) != sha256.Size*2 || marker.Size <= 0 || marker.ModTimeNano == 0 {
		return extractionMarker{}, ErrInvalidCapability
	}
	if _, err := hex.DecodeString(marker.Fingerprint); err != nil {
		return extractionMarker{}, ErrInvalidCapability
	}
	coverPath, err := engine.NormalizeCBZResourcePath(marker.CoverPath)
	if err != nil || coverPath == "" {
		return extractionMarker{}, ErrInvalidCapability
	}
	if _, ok := engine.CBZImageContentType(coverPath); !ok {
		return extractionMarker{}, ErrInvalidCapability
	}
	marker.CoverPath = coverPath
	return marker, nil
}

func writeExtractionMarker(markerPath, fingerprint, sourcePath, coverPath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	data, err := json.Marshal(extractionMarker{
		Fingerprint: strings.ToLower(strings.TrimSpace(fingerprint)),
		Size:        info.Size(),
		ModTimeNano: info.ModTime().UnixNano(),
		CoverPath:   coverPath,
	})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(markerPath), ".marker-*")
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
	return os.Rename(tempPath, markerPath)
}

func (s *Service) recoverChapterResourcePath(extractionRoot string, chapterIndex int) (string, error) {
	images := make([]string, 0)
	err := filepath.WalkDir(extractionRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() == extractionMarkerName {
			return nil
		}
		relative, err := filepath.Rel(extractionRoot, filePath)
		if err != nil {
			return err
		}
		resourcePath, err := engine.NormalizeCBZResourcePath(filepath.ToSlash(relative))
		if err != nil || resourcePath == "" {
			return ErrUnsafePath
		}
		if _, ok := engine.CBZImageContentType(resourcePath); ok {
			images = append(images, resourcePath)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(images)
	if chapterIndex < 0 || chapterIndex >= len(images) {
		return "", ErrNotFound
	}
	return images[chapterIndex], nil
}

func (s *Service) resourceFile(extractionRoot, resourcePath string) (string, error) {
	filePath, err := joinUnder(extractionRoot, filepath.FromSlash(resourcePath))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	if info.IsDir() {
		return "", ErrNotFound
	}
	resolved, err := filepath.EvalSymlinks(filePath)
	if err != nil || !withinPath(extractionRoot, resolved) {
		return "", ErrUnsafePath
	}
	return resolved, nil
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
	signature := capabilitySignature(secret, encodedPayload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
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
	if claims.UserID == 0 || claims.BookID == 0 ||
		claims.Purpose != resourceCapabilityPurpose ||
		claims.ExpiresAt <= 0 ||
		len(claims.Fingerprint) != sha256.Size*2 {
		return ErrInvalidCapability
	}
	if _, err := hex.DecodeString(claims.Fingerprint); err != nil {
		return ErrInvalidCapability
	}
	return nil
}

func capabilitySignature(secret, payload string) []byte {
	derivation := hmac.New(sha256.New, []byte(secret))
	_, _ = derivation.Write([]byte(resourceCapabilityPurpose))
	key := derivation.Sum(nil)

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func fingerprintFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func joinUnder(root, relative string) (string, error) {
	rootAbs, err := canonicalPath(root)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(relative) {
		return "", ErrUnsafePath
	}
	target := filepath.Join(rootAbs, filepath.Clean(relative))
	if !withinPath(rootAbs, target) {
		return "", ErrUnsafePath
	}
	return target, nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	current := abs
	missingSegments := make([]string, 0, 4)
	for {
		if _, statErr := os.Lstat(current); statErr == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", resolveErr
			}
			for index := len(missingSegments) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missingSegments[index])
			}
			return filepath.Clean(resolved), nil
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		missingSegments = append(missingSegments, filepath.Base(current))
		current = parent
	}
}

func withinPath(root, target string) bool {
	rootAbs, err := canonicalPath(root)
	if err != nil {
		return false
	}
	targetAbs, err := canonicalPath(target)
	if err != nil {
		return false
	}
	if rootAbs == targetAbs {
		return true
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func escapeResourcePath(resourcePath string) string {
	segments := strings.Split(resourcePath, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
