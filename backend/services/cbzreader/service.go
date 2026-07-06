package cbzreader

import (
	"archive/zip"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	resourceCapabilityPurpose = "openreader:cbz-resource:v1"
	resourceCapabilityTTL     = 12 * time.Hour
	maxCBZArchiveBytes        = 1 << 30
	maxCBZEntries             = 20_000
	maxCBZEntryBytes          = 128 << 20
	maxCBZTotalBytes          = 2 << 30
	maxResourceBytes          = 128 << 20
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
	cfg config.Config
	db  *gorm.DB
	now func() time.Time
}

type PreparedChapter struct {
	ResourceURL string
	ExpiresAt   time.Time
}

type Resource struct {
	Data        []byte
	ContentType string
}

type resourceClaims struct {
	UserID      uint   `json:"u"`
	BookID      uint   `json:"b"`
	Fingerprint string `json:"f"`
	Purpose     string `json:"p"`
	ExpiresAt   int64  `json:"e"`
}

func New(cfg config.Config, database *gorm.DB) *Service {
	return &Service{cfg: cfg, db: database, now: time.Now}
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

func (s *Service) PrepareChapter(book models.Book, chapter *models.Chapter) (PreparedChapter, error) {
	if chapter == nil || chapter.BookID != book.ID || !IsLocalCBZ(book) {
		return PreparedChapter{}, ErrNotCBZ
	}
	sourcePath, err := s.sourcePath(book)
	if err != nil {
		return PreparedChapter{}, err
	}
	fingerprint, err := fingerprintFile(sourcePath)
	if err != nil {
		return PreparedChapter{}, err
	}

	resourcePath := strings.TrimSpace(chapter.ResourcePath)
	if resourcePath == "" {
		resourcePath, err = s.recoverChapterResourcePath(sourcePath, chapter.Index)
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
	if _, _, err := readImageEntry(sourcePath, resourcePath); err != nil {
		return PreparedChapter{}, err
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
	sourcePath, err := s.sourcePath(book)
	if err != nil {
		return Resource{}, err
	}
	fingerprint, err := fingerprintFile(sourcePath)
	if err != nil {
		return Resource{}, err
	}
	if !strings.EqualFold(fingerprint, claims.Fingerprint) {
		return Resource{}, ErrInvalidCapability
	}
	resourcePath, err := engine.NormalizeCBZResourcePath(strings.TrimPrefix(requestedPath, "/"))
	if err != nil || resourcePath == "" {
		return Resource{}, ErrUnsafePath
	}
	contentType, data, err := readImageEntry(sourcePath, resourcePath)
	if err != nil {
		return Resource{}, err
	}
	return Resource{Data: data, ContentType: contentType}, nil
}

func (s *Service) sourcePath(book models.Book) (string, error) {
	libraryRoot, err := canonicalPath(s.cfg.LibraryDir)
	if err != nil {
		return "", err
	}
	candidates := make([]string, 0, 4)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		for _, existing := range candidates {
			if existing == candidate {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	original := strings.TrimSpace(book.OriginalFile)
	if original != "" && !filepath.IsAbs(original) {
		add(filepath.Join(libraryRoot, original))
	}
	libraryPath := strings.TrimSpace(book.LibraryPath)
	if libraryPath != "" && !filepath.IsAbs(libraryPath) {
		bookRoot := filepath.Join(libraryRoot, filepath.Clean(libraryPath))
		if original != "" {
			add(filepath.Join(bookRoot, filepath.Base(original)))
		}
		if entries, err := os.ReadDir(bookRoot); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".cbz") {
					add(filepath.Join(bookRoot, entry.Name()))
				}
			}
		}
	}
	if filepath.IsAbs(original) {
		add(original)
	}

	for _, candidate := range candidates {
		if validCBZSource(candidate, libraryRoot) {
			return candidate, nil
		}
	}
	return "", ErrNotFound
}

func validCBZSource(candidate, libraryRoot string) bool {
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(candidate), ".cbz") {
		return false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	return withinPath(libraryRoot, resolved)
}

func (s *Service) recoverChapterResourcePath(sourcePath string, chapterIndex int) (string, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	parsed, err := engine.ParseCBZ(data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	if chapterIndex < 0 || chapterIndex >= len(parsed.Chapters) {
		return "", ErrNotFound
	}
	resourcePath, err := engine.NormalizeCBZResourcePath(parsed.Chapters[chapterIndex].ResourcePath)
	if err != nil || resourcePath == "" {
		return "", ErrNotFound
	}
	return resourcePath, nil
}

func readImageEntry(sourcePath, resourcePath string) (string, []byte, error) {
	contentType, ok := engine.CBZImageContentType(resourcePath)
	if !ok {
		return "", nil, ErrUnsupportedMedia
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, ErrNotFound
		}
		return "", nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	if info.Size() > maxCBZArchiveBytes {
		return "", nil, ErrExtractionLimit
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return "", nil, ErrInvalidArchive
	}
	if len(reader.File) > maxCBZEntries {
		return "", nil, ErrExtractionLimit
	}
	seen := make(map[string]bool, len(reader.File))
	var total uint64
	for _, entry := range reader.File {
		canonical, err := engine.NormalizeCBZResourcePath(entry.Name)
		if err != nil {
			return "", nil, ErrUnsafePath
		}
		if canonical == "" {
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return "", nil, ErrUnsafePath
		}
		key := strings.ToLower(canonical)
		if seen[key] {
			return "", nil, ErrUnsafePath
		}
		seen[key] = true
		if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, "/") {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxCBZEntryBytes) {
			return "", nil, ErrExtractionLimit
		}
		if ^uint64(0)-total < entry.UncompressedSize64 {
			return "", nil, ErrExtractionLimit
		}
		total += entry.UncompressedSize64
		if total > uint64(maxCBZTotalBytes) {
			return "", nil, ErrExtractionLimit
		}
		if canonical == resourcePath {
			opened, err := entry.Open()
			if err != nil {
				return "", nil, err
			}
			defer opened.Close()
			data, err := io.ReadAll(io.LimitReader(opened, maxResourceBytes+1))
			if err != nil {
				return "", nil, err
			}
			if len(data) > maxResourceBytes {
				return "", nil, ErrExtractionLimit
			}
			return contentType, data, nil
		}
	}
	return "", nil, ErrNotFound
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
