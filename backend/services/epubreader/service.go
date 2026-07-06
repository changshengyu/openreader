package epubreader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
)

const (
	extractionDirectoryName = ".epub-resources"
	extractionMarkerName    = ".openreader-complete"
	resourceCapabilityTTL   = 12 * time.Hour
	maxDocumentBytes        = 16 << 20
)

type Service struct {
	cfg       config.Config
	db        *gorm.DB
	now       func() time.Time
	limits    extractionLimits
	extractMu sync.Mutex
}

type PreparedChapter struct {
	ResourceURL string
	ExpiresAt   time.Time
}

type Resource struct {
	Path        string
	Data        []byte
	ContentType string
	CSP         string
	Document    bool
}

func New(cfg config.Config, database *gorm.DB) *Service {
	return &Service{
		cfg:    cfg,
		db:     database,
		now:    time.Now,
		limits: defaultExtractionLimits(),
	}
}

func IsLocalEPUB(book models.Book) bool {
	if book.SourceID != 0 {
		return false
	}
	for _, candidate := range []string{book.OriginalFile, book.URL} {
		if strings.EqualFold(filepath.Ext(strings.TrimSpace(candidate)), ".epub") {
			return true
		}
	}
	return false
}

func (s *Service) PrepareChapter(book models.Book, chapter *models.Chapter) (PreparedChapter, error) {
	if chapter == nil || chapter.BookID != book.ID || !IsLocalEPUB(book) {
		return PreparedChapter{}, ErrNotEPUB
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
		resourcePath, err = s.recoverChapterResourcePath(sourcePath, book, chapter.Index)
		if err != nil {
			return PreparedChapter{}, err
		}
		chapter.ResourcePath = resourcePath
		if err := s.db.Model(chapter).Update("resource_path", resourcePath).Error; err != nil {
			return PreparedChapter{}, err
		}
		s.backfillArchivedChapter(book, chapter.Index, resourcePath)
	}
	resourcePath, err = normalizeArchivePath(resourcePath)
	if err != nil || resourcePath == "" {
		return PreparedChapter{}, ErrUnsafePath
	}
	if _, err := s.resourceFile(extractionRoot, resourcePath); err != nil {
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
		ResourceURL: "/api/epub-resource/" + url.PathEscape(capability) + "/" + escapeResourcePath(resourcePath),
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
	if !IsLocalEPUB(book) {
		return Resource{}, ErrNotFound
	}
	sourcePath, bookRoot, err := s.sourcePath(book)
	if err != nil {
		return Resource{}, err
	}
	fingerprint, extractionRoot, err := s.ensureExtraction(sourcePath, bookRoot)
	if err != nil {
		return Resource{}, err
	}
	if !strings.EqualFold(fingerprint, claims.Fingerprint) {
		return Resource{}, ErrInvalidCapability
	}
	resourcePath, err := normalizeArchivePath(strings.TrimPrefix(requestedPath, "/"))
	if err != nil || resourcePath == "" {
		return Resource{}, ErrUnsafePath
	}
	filePath, err := s.resourceFile(extractionRoot, resourcePath)
	if err != nil {
		return Resource{}, err
	}
	contentType, document, ok := resourceMediaType(resourcePath)
	if !ok {
		return Resource{}, ErrUnsupportedMedia
	}
	resource := Resource{
		Path:        filePath,
		ContentType: contentType,
		CSP:         documentCSP(),
		Document:    document,
	}
	if !document {
		return resource, nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return Resource{}, err
	}
	if len(data) > maxDocumentBytes {
		return Resource{}, ErrExtractionLimit
	}
	resource.Data, err = sanitizeAndInjectDocument(data)
	if err != nil {
		return Resource{}, err
	}
	return resource, nil
}

func (s *Service) sourcePath(book models.Book) (string, string, error) {
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
	if filepath.IsAbs(libraryPath) ||
		libraryPath == ".." || strings.HasPrefix(libraryPath, ".."+string(filepath.Separator)) {
		return "", "", ErrUnsafePath
	}
	bookRoot, err := joinUnder(libraryRoot, libraryPath)
	if err != nil {
		return "", "", err
	}

	candidates := make([]string, 0, 3)
	if original != "" && !filepath.IsAbs(original) {
		if candidate, err := joinUnder(libraryRoot, original); err == nil {
			candidates = append(candidates, candidate)
		}
	}
	if original != "" {
		candidates = append(candidates, filepath.Join(bookRoot, filepath.Base(original)))
	}
	if filepath.IsAbs(original) {
		if candidate, err := filepath.Abs(original); err == nil {
			if withinPath(libraryRoot, candidate) {
				candidates = append(candidates, candidate)
			}
		}
	}

	for _, candidate := range candidates {
		if validEPUBSource(candidate, libraryRoot, bookRoot) {
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
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".epub") {
			continue
		}
		candidate := filepath.Join(bookRoot, entry.Name())
		if validEPUBSource(candidate, libraryRoot, bookRoot) {
			return candidate, bookRoot, nil
		}
	}
	return "", "", ErrNotFound
}

func validEPUBSource(candidate, libraryRoot, bookRoot string) bool {
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(candidate), ".epub") {
		return false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	return withinPath(libraryRoot, resolved) && withinPath(bookRoot, resolved)
}

func (s *Service) ensureExtraction(sourcePath, bookRoot string) (string, string, error) {
	s.extractMu.Lock()
	defer s.extractMu.Unlock()

	fingerprint, err := fingerprintFile(sourcePath)
	if err != nil {
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
	marker := filepath.Join(finalRoot, extractionMarkerName)
	if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == fingerprint {
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
	if err := extractArchiveFile(sourcePath, staging, s.limits); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(staging, extractionMarkerName), []byte(fingerprint+"\n"), 0o644); err != nil {
		return "", "", err
	}
	if err := os.Rename(staging, finalRoot); err != nil {
		if data, readErr := os.ReadFile(marker); readErr == nil && strings.TrimSpace(string(data)) == fingerprint {
			return fingerprint, finalRoot, nil
		}
		return "", "", err
	}
	return fingerprint, finalRoot, nil
}

func (s *Service) recoverChapterResourcePath(sourcePath string, book models.Book, chapterIndex int) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", ErrNotFound
	}
	if info.Size() > s.limits.MaxArchiveBytes {
		return "", ErrExtractionLimit
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	parsed, err := engine.ParseEPUBWithRule(data, book.TOCRule)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	if chapterIndex < 0 || chapterIndex >= len(parsed.Chapters) {
		return "", ErrNotFound
	}
	resourcePath, err := normalizeArchivePath(parsed.Chapters[chapterIndex].ResourcePath)
	if err != nil || resourcePath == "" {
		return "", ErrNotFound
	}
	return resourcePath, nil
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

func (s *Service) backfillArchivedChapter(book models.Book, chapterIndex int, resourcePath string) {
	tocPath := strings.TrimSpace(book.TOCFile)
	if tocPath == "" || filepath.IsAbs(tocPath) {
		return
	}
	fullPath, err := joinUnder(s.cfg.LibraryDir, tocPath)
	if err != nil {
		return
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}
	var chapters []engine.ArchivedChapter
	if err := json.Unmarshal(data, &chapters); err != nil {
		return
	}
	changed := false
	for index := range chapters {
		if chapters[index].Index == chapterIndex && chapters[index].ResourcePath != resourcePath {
			chapters[index].ResourcePath = resourcePath
			changed = true
		}
	}
	if !changed {
		return
	}
	updated, err := json.MarshalIndent(chapters, "", "  ")
	if err != nil {
		return
	}
	updated = append(updated, '\n')
	temp, err := os.CreateTemp(filepath.Dir(fullPath), ".chapters-*.json")
	if err != nil {
		return
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(updated); err != nil {
		_ = temp.Close()
		return
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return
	}
	if err := temp.Close(); err != nil {
		return
	}
	_ = os.Rename(tempPath, fullPath)
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

func withinPath(root, target string) bool {
	rootAbs, err := canonicalPath(root)
	if err != nil {
		return false
	}
	targetAbs, err := canonicalPath(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func canonicalPath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	current := absolute
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
			return absolute, nil
		}
		missingSegments = append(missingSegments, filepath.Base(current))
		current = parent
	}
}

func resourceMediaType(resourcePath string) (string, bool, bool) {
	extension := strings.ToLower(path.Ext(resourcePath))
	switch extension {
	case ".xhtml", ".html", ".htm":
		return "text/html; charset=utf-8", true, true
	case ".css":
		return "text/css; charset=utf-8", false, true
	case ".jpg", ".jpeg":
		return "image/jpeg", false, true
	case ".png":
		return "image/png", false, true
	case ".gif":
		return "image/gif", false, true
	case ".webp":
		return "image/webp", false, true
	case ".bmp":
		return "image/bmp", false, true
	case ".avif":
		return "image/avif", false, true
	case ".svg":
		return "image/svg+xml", false, true
	case ".woff":
		return "font/woff", false, true
	case ".woff2":
		return "font/woff2", false, true
	case ".ttf":
		return "font/ttf", false, true
	case ".otf":
		return "font/otf", false, true
	}
	if detected := mime.TypeByExtension(extension); detected != "" {
		return detected, false, false
	}
	return "application/octet-stream", false, false
}

func escapeResourcePath(resourcePath string) string {
	segments := strings.Split(resourcePath, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return strings.Join(segments, "/")
}
