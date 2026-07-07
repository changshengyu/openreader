package audioreader

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

const (
	resourceCapabilityPurpose = "openreader:audio-resource:v1"
	resourceCapabilityTTL     = 12 * time.Hour
)

type Service struct {
	cfg config.Config
	db  *gorm.DB
	now func() time.Time
}

type PreparedResource struct {
	ResourceURL string
	ExpiresAt   time.Time
}

type Resource struct {
	Path        string
	ContentType string
}

type resourceClaims struct {
	UserID       uint   `json:"u"`
	BookID       uint   `json:"b"`
	Fingerprint  string `json:"f"`
	ResourcePath string `json:"r"`
	Purpose      string `json:"p"`
	ExpiresAt    int64  `json:"e"`
}

func New(cfg config.Config, database *gorm.DB) *Service {
	return &Service{cfg: cfg, db: database, now: time.Now}
}

func IsAudioBook(book models.Book) bool {
	return book.Type == 1
}

func PrepareDirectOrLocal(svc *Service, book models.Book, chapter *models.Chapter, content string) (PreparedResource, error) {
	if directURL, ok := directAudioURL(book, chapter, content); ok {
		expiresAt := svc.now().UTC().Add(resourceCapabilityTTL)
		return PreparedResource{ResourceURL: directURL, ExpiresAt: expiresAt}, nil
	}
	return svc.PrepareChapter(book, chapter, content)
}

func (s *Service) PrepareChapter(book models.Book, chapter *models.Chapter, content string) (PreparedResource, error) {
	if chapter == nil || chapter.BookID != book.ID || !IsAudioBook(book) {
		return PreparedResource{}, ErrNotAudio
	}
	bookRoot, err := s.bookRoot(book)
	if err != nil {
		return PreparedResource{}, err
	}
	resourcePath, filePath, err := s.resolveFirstExistingAudio(bookRoot, content, chapter)
	if err != nil {
		return PreparedResource{}, err
	}
	contentType, ok := audioMediaType(resourcePath)
	if !ok || contentType == "" {
		return PreparedResource{}, ErrUnsupportedMedia
	}
	fingerprint, err := fingerprintFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return PreparedResource{}, ErrNotFound
		}
		return PreparedResource{}, err
	}
	expiresAt := s.now().UTC().Add(resourceCapabilityTTL)
	capability, err := signResourceCapability(s.cfg.JWTSecret, resourceClaims{
		UserID:       book.UserID,
		BookID:       book.ID,
		Fingerprint:  fingerprint,
		ResourcePath: resourcePath,
		Purpose:      resourceCapabilityPurpose,
		ExpiresAt:    expiresAt.Unix(),
	})
	if err != nil {
		return PreparedResource{}, err
	}
	return PreparedResource{
		ResourceURL: "/api/audio-resource/" + url.PathEscape(capability) + "/" + escapeResourcePath(resourcePath),
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
	if !IsAudioBook(book) {
		return Resource{}, ErrNotFound
	}
	resourcePath, err := normalizeResourcePath(strings.TrimPrefix(requestedPath, "/"))
	if err != nil || resourcePath == "" {
		return Resource{}, ErrUnsafePath
	}
	if resourcePath != claims.ResourcePath {
		return Resource{}, ErrInvalidCapability
	}
	bookRoot, err := s.bookRoot(book)
	if err != nil {
		return Resource{}, err
	}
	filePath, err := resolveRelativeUnder(bookRoot, resourcePath)
	if err != nil {
		return Resource{}, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	if info.IsDir() {
		return Resource{}, ErrNotFound
	}
	contentType, ok := audioMediaType(resourcePath)
	if !ok {
		return Resource{}, ErrUnsupportedMedia
	}
	fingerprint, err := fingerprintFile(filePath)
	if err != nil {
		return Resource{}, err
	}
	if !strings.EqualFold(fingerprint, claims.Fingerprint) {
		return Resource{}, ErrInvalidCapability
	}
	return Resource{Path: filePath, ContentType: contentType}, nil
}

func directAudioURL(book models.Book, chapter *models.Chapter, content string) (string, bool) {
	candidates := candidateValues(book, chapter, content)
	for _, candidate := range candidates {
		if parsed, ok := parseSafeDirectAudio(candidate); ok {
			parsed.Fragment = ""
			return parsed.String(), true
		}
		if chapter != nil {
			baseRaw := strings.TrimSpace(chapter.URL)
			if baseRaw == "" {
				baseRaw = strings.TrimSpace(book.URL)
			}
			base, baseOK := parseSafeDirectAudio(baseRaw)
			if baseOK {
				relative, err := url.Parse(candidate)
				if err == nil && !relative.IsAbs() && relative.Path != "" && relative.RawQuery == "" {
					resolved := base.ResolveReference(relative)
					if safeDirectAudioURL(resolved) {
						resolved.Fragment = ""
						return resolved.String(), true
					}
				}
			}
		}
	}
	return "", false
}

func parseSafeDirectAudio(value string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !parsed.IsAbs() || !safeDirectAudioURL(parsed) {
		return nil, false
	}
	return parsed, true
}

func safeDirectAudioURL(value *url.URL) bool {
	if value == nil {
		return false
	}
	if value.Scheme != "http" && value.Scheme != "https" {
		return false
	}
	if value.Host == "" || value.User != nil {
		return false
	}
	return true
}

func (s *Service) resolveFirstExistingAudio(bookRoot string, content string, chapter *models.Chapter) (string, string, error) {
	var firstErr error
	for _, candidate := range candidateValues(models.Book{}, chapter, content) {
		resourcePath, filePath, err := resolveCandidateUnder(bookRoot, candidate)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		info, statErr := os.Stat(filePath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return "", "", statErr
		}
		if info.IsDir() {
			continue
		}
		if _, ok := audioMediaType(resourcePath); !ok {
			return "", "", ErrUnsupportedMedia
		}
		return resourcePath, filePath, nil
	}
	if firstErr != nil {
		return "", "", firstErr
	}
	return "", "", ErrNotFound
}

func candidateValues(book models.Book, chapter *models.Chapter, content string) []string {
	values := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range values {
			if existing == value {
				return
			}
		}
		values = append(values, value)
	}
	add(content)
	if chapter != nil {
		add(chapter.URL)
		add(chapter.ResourcePath)
	}
	add(book.URL)
	return values
}

func (s *Service) bookRoot(book models.Book) (string, error) {
	if !IsAudioBook(book) || book.SourceID != 0 {
		return "", ErrNotAudio
	}
	libraryRoot, err := canonicalPath(s.cfg.LibraryDir)
	if err != nil {
		return "", err
	}
	libraryPath := strings.TrimSpace(book.LibraryPath)
	if libraryPath == "" {
		original := strings.TrimSpace(book.OriginalFile)
		switch {
		case original != "" && !filepath.IsAbs(original):
			libraryPath = filepath.Dir(original)
		case original != "" && filepath.IsAbs(original):
			canonicalOriginal, canonicalErr := canonicalPath(original)
			if canonicalErr != nil {
				return "", canonicalErr
			}
			if !withinPath(libraryRoot, canonicalOriginal) {
				return "", ErrUnsafePath
			}
			relative, relativeErr := filepath.Rel(libraryRoot, filepath.Dir(canonicalOriginal))
			if relativeErr != nil {
				return "", ErrUnsafePath
			}
			libraryPath = relative
		}
	}
	if strings.TrimSpace(libraryPath) == "" || libraryPath == "." {
		return "", ErrUnsafePath
	}
	libraryPath = filepath.Clean(libraryPath)
	if filepath.IsAbs(libraryPath) ||
		libraryPath == ".." || strings.HasPrefix(libraryPath, ".."+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	return joinUnder(libraryRoot, libraryPath)
}

func resolveCandidateUnder(bookRoot, candidate string) (string, string, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || strings.ContainsRune(candidate, 0) {
		return "", "", ErrUnsafePath
	}
	if parsed, err := url.Parse(candidate); err == nil {
		if parsed.IsAbs() {
			return "", "", ErrUnsafePath
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", "", ErrUnsafePath
		}
		candidate = parsed.Path
	}
	if filepath.IsAbs(candidate) {
		canonicalCandidate, err := canonicalPath(candidate)
		if err != nil {
			return "", "", err
		}
		if !withinPath(bookRoot, canonicalCandidate) {
			return "", "", ErrUnsafePath
		}
		relative, err := filepath.Rel(bookRoot, canonicalCandidate)
		if err != nil {
			return "", "", ErrUnsafePath
		}
		resourcePath, err := normalizeResourcePath(filepath.ToSlash(relative))
		if err != nil {
			return "", "", err
		}
		return resourcePath, canonicalCandidate, nil
	}
	resourcePath, err := normalizeResourcePath(filepath.ToSlash(candidate))
	if err != nil {
		return "", "", err
	}
	filePath, err := resolveRelativeUnder(bookRoot, resourcePath)
	if err != nil {
		return "", "", err
	}
	return resourcePath, filePath, nil
}

func resolveRelativeUnder(bookRoot, resourcePath string) (string, error) {
	resourcePath, err := normalizeResourcePath(resourcePath)
	if err != nil {
		return "", err
	}
	filePath, err := joinUnder(bookRoot, filepath.FromSlash(resourcePath))
	if err != nil {
		return "", err
	}
	if !withinPath(bookRoot, filePath) {
		return "", ErrUnsafePath
	}
	return filePath, nil
}

func normalizeResourcePath(resourcePath string) (string, error) {
	resourcePath = strings.TrimSpace(strings.ReplaceAll(resourcePath, "\\", "/"))
	if resourcePath == "" || strings.ContainsRune(resourcePath, 0) || strings.HasPrefix(resourcePath, "/") {
		return "", ErrUnsafePath
	}
	cleaned := path.Clean(resourcePath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrUnsafePath
	}
	return cleaned, nil
}

func audioMediaType(resourcePath string) (string, bool) {
	switch strings.ToLower(path.Ext(resourcePath)) {
	case ".mp3":
		return "audio/mpeg", true
	case ".m4a", ".mp4":
		return "audio/mp4", true
	case ".aac":
		return "audio/aac", true
	case ".ogg", ".oga":
		return "audio/ogg", true
	case ".opus":
		return "audio/ogg; codecs=opus", true
	case ".wav":
		return "audio/wav", true
	case ".flac":
		return "audio/flac", true
	case ".webm":
		return "audio/webm", true
	}
	if detected := mime.TypeByExtension(path.Ext(resourcePath)); strings.HasPrefix(detected, "audio/") {
		return detected, true
	}
	return "", false
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
	if _, err := normalizeResourcePath(claims.ResourcePath); err != nil {
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

func joinUnder(root, child string) (string, error) {
	if filepath.IsAbs(child) {
		return "", ErrUnsafePath
	}
	cleanChild := filepath.Clean(child)
	if cleanChild == "." || cleanChild == ".." || strings.HasPrefix(cleanChild, ".."+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	fullPath := filepath.Join(root, cleanChild)
	if !withinPath(root, fullPath) {
		return "", ErrUnsafePath
	}
	return fullPath, nil
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
