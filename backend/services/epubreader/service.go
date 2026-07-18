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
	Data        []byte
	ContentType string
	CSP         string
	Document    bool
}

type extractionMarker struct {
	Fingerprint string `json:"fingerprint"`
	Size        int64  `json:"size"`
	ModTimeNano int64  `json:"modTimeNano"`
}

func New(cfg config.Config, database *gorm.DB) *Service {
	return &Service{
		cfg:         cfg,
		db:          database,
		now:         time.Now,
		limits:      defaultExtractionLimits(),
		fingerprint: fingerprintFile,
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

// PrepareBookResources eagerly creates the same bounded immutable extraction
// that PrepareChapter would otherwise build on the first Reader request. Local
// import calls this only for a newly allocated caller-owned archive, so a later
// database failure can compensate by deleting that whole archive directory.
func (s *Service) PrepareBookResources(book models.Book) error {
	if !IsLocalEPUB(book) {
		return ErrNotEPUB
	}
	sourcePath, bookRoot, err := s.sourcePath(book)
	if err != nil {
		return err
	}
	_, _, err = s.ensureExtraction(sourcePath, bookRoot)
	return err
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
	resourceFragment, err := normalizeResourceFragment(chapter.ResourceFragment)
	if err != nil {
		return PreparedChapter{}, ErrUnsafePath
	}
	resourceEndFragment, err := normalizeResourceFragment(chapter.ResourceEndFragment)
	if err != nil {
		return PreparedChapter{}, ErrUnsafePath
	}
	if resourcePath == "" || (resourceFragment == "" && resourceEndFragment == "" && epubTOCRuleCanCarryFragments(book.TOCRule)) {
		recovered, recoverErr := s.recoverChapterResourceMetadata(sourcePath, book, chapter.Index)
		if recoverErr != nil && resourcePath == "" {
			return PreparedChapter{}, recoverErr
		}
		if recoverErr == nil && (resourcePath == "" || resourcePath == recovered.ResourcePath) {
			resourcePath = recovered.ResourcePath
			resourceFragment = recovered.ResourceFragment
			resourceEndFragment = recovered.ResourceEndFragment
		}
	}
	resourcePath, err = normalizeArchivePath(resourcePath)
	if err != nil || resourcePath == "" {
		return PreparedChapter{}, ErrUnsafePath
	}
	if resourcePath != chapter.ResourcePath || resourceFragment != chapter.ResourceFragment || resourceEndFragment != chapter.ResourceEndFragment {
		chapter.ResourcePath = resourcePath
		chapter.ResourceFragment = resourceFragment
		chapter.ResourceEndFragment = resourceEndFragment
		if err := s.db.Model(chapter).Updates(map[string]any{
			"resource_path":         resourcePath,
			"resource_fragment":     resourceFragment,
			"resource_end_fragment": resourceEndFragment,
		}).Error; err != nil {
			return PreparedChapter{}, err
		}
		s.backfillArchivedChapter(book, chapter.Index, resourcePath, resourceFragment, resourceEndFragment)
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

	expiresAt := s.now().UTC().Add(resourceCapabilityTTL)
	capability, err := signResourceCapability(s.cfg.JWTSecret, resourceClaims{
		UserID:              book.UserID,
		BookID:              book.ID,
		Fingerprint:         fingerprint,
		Purpose:             resourceCapabilityPurpose,
		ExpiresAt:           expiresAt.Unix(),
		DocumentPath:        resourcePath,
		ResourceFragment:    resourceFragment,
		ResourceEndFragment: resourceEndFragment,
	})
	if err != nil {
		return PreparedChapter{}, err
	}
	resourceURL := "/api/epub-resource/" + url.PathEscape(capability) + "/" + escapeResourcePath(resourcePath)
	if resourceFragment != "" {
		resourceURL += "#" + url.PathEscape(resourceFragment)
	}
	return PreparedChapter{
		ResourceURL: resourceURL,
		ExpiresAt:   expiresAt,
	}, nil
}

// ReadChapterText rebuilds only the requested EPUB chapter's searchable text
// from the verified immutable extraction. It is used when an old or manually
// cleared chapter cache is missing; unlike the legacy recovery path it does not
// parse every spine resource in the source archive.
func (s *Service) ReadChapterText(book models.Book, chapter *models.Chapter) (string, error) {
	if _, err := s.PrepareChapter(book, chapter); err != nil {
		return "", err
	}
	sourcePath, bookRoot, err := s.sourcePath(book)
	if err != nil {
		return "", err
	}
	_, extractionRoot, err := s.ensureExtraction(sourcePath, bookRoot)
	if err != nil {
		return "", err
	}
	resourcePath, err := normalizeArchivePath(strings.TrimSpace(chapter.ResourcePath))
	if err != nil || resourcePath == "" {
		return "", ErrUnsafePath
	}
	filePath, err := s.resourceFile(extractionRoot, resourcePath)
	if err != nil {
		return "", err
	}
	contentType, document, ok := resourceMediaType(resourcePath)
	if !ok || !document || !strings.Contains(contentType, "html") {
		return "", ErrUnsupportedMedia
	}
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxDocumentBytes {
		return "", ErrExtractionLimit
	}
	return extractDocumentPlainText(data, chapter.ResourceFragment, chapter.ResourceEndFragment)
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
		sourcePath, sourceBookRoot, sourceErr := s.sourcePath(book)
		if sourceErr != nil {
			return Resource{}, sourceErr
		}
		fingerprint, rebuiltRoot, rebuildErr := s.ensureExtraction(sourcePath, sourceBookRoot)
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
	resourcePath, err := normalizeArchivePath(strings.TrimPrefix(requestedPath, "/"))
	if err != nil || resourcePath == "" {
		return Resource{}, ErrUnsafePath
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
		extractionRoot = rebuiltRoot
		filePath, err = s.resourceFile(extractionRoot, resourcePath)
	}
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
	fragment := ""
	endFragment := ""
	if claims.DocumentPath == resourcePath {
		fragment = claims.ResourceFragment
		endFragment = claims.ResourceEndFragment
	}
	resource.Data, err = sanitizeAndInjectDocument(data, fragment, endFragment)
	if err != nil {
		return Resource{}, err
	}
	return resource, nil
}

func (s *Service) sourcePath(book models.Book) (string, string, error) {
	libraryRoot, bookRoot, err := s.bookRoots(book)
	if err != nil {
		return "", "", err
	}
	original := strings.TrimSpace(book.OriginalFile)

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
	if filepath.IsAbs(libraryPath) ||
		libraryPath == ".." || strings.HasPrefix(libraryPath, ".."+string(filepath.Separator)) {
		return "", "", ErrUnsafePath
	}
	bookRoot, err := joinUnder(libraryRoot, libraryPath)
	if err != nil {
		return "", "", err
	}
	return libraryRoot, bookRoot, nil
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
	if current, err := readExtractionMarker(marker); err == nil && current.Fingerprint == fingerprint {
		if !forceRebuild {
			if current.Size == 0 || current.ModTimeNano == 0 {
				_ = writeExtractionMarker(marker, fingerprint, sourcePath)
			}
			return fingerprint, finalRoot, nil
		}
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
	if err := writeExtractionMarker(filepath.Join(staging, extractionMarkerName), fingerprint, sourcePath); err != nil {
		return "", "", err
	}
	if err := os.Rename(staging, finalRoot); err != nil {
		if current, readErr := readExtractionMarker(marker); readErr == nil && current.Fingerprint == fingerprint {
			return fingerprint, finalRoot, nil
		}
		return "", "", err
	}
	return fingerprint, finalRoot, nil
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
	return writeExtractionMarker(markerPath, currentFingerprint, sourcePath)
}

func readExtractionMarker(markerPath string) (extractionMarker, error) {
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return extractionMarker{}, err
	}
	var marker extractionMarker
	if json.Unmarshal(data, &marker) == nil && marker.Fingerprint != "" {
		marker.Fingerprint = strings.ToLower(strings.TrimSpace(marker.Fingerprint))
		return marker, nil
	}
	legacy := strings.ToLower(strings.TrimSpace(string(data)))
	if len(legacy) == sha256.Size*2 {
		if _, err := hex.DecodeString(legacy); err == nil {
			return extractionMarker{Fingerprint: legacy}, nil
		}
	}
	return extractionMarker{}, ErrInvalidCapability
}

func writeExtractionMarker(markerPath, fingerprint, sourcePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	data, err := json.Marshal(extractionMarker{
		Fingerprint: strings.ToLower(strings.TrimSpace(fingerprint)),
		Size:        info.Size(),
		ModTimeNano: info.ModTime().UnixNano(),
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

func (s *Service) recoverChapterResourceMetadata(sourcePath string, book models.Book, chapterIndex int) (engine.TXTChapter, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return engine.TXTChapter{}, ErrNotFound
	}
	if info.Size() > s.limits.MaxArchiveBytes {
		return engine.TXTChapter{}, ErrExtractionLimit
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return engine.TXTChapter{}, err
	}
	parsed, err := engine.ParseEPUBWithRule(data, book.TOCRule)
	if err != nil {
		return engine.TXTChapter{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}
	if chapterIndex < 0 || chapterIndex >= len(parsed.Chapters) {
		return engine.TXTChapter{}, ErrNotFound
	}
	chapter := parsed.Chapters[chapterIndex]
	resourcePath, err := normalizeArchivePath(chapter.ResourcePath)
	if err != nil || resourcePath == "" {
		return engine.TXTChapter{}, ErrNotFound
	}
	chapter.ResourcePath = resourcePath
	if chapter.ResourceFragment, err = normalizeResourceFragment(chapter.ResourceFragment); err != nil {
		return engine.TXTChapter{}, ErrUnsafePath
	}
	if chapter.ResourceEndFragment, err = normalizeResourceFragment(chapter.ResourceEndFragment); err != nil {
		return engine.TXTChapter{}, ErrUnsafePath
	}
	return chapter, nil
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

func (s *Service) backfillArchivedChapter(book models.Book, chapterIndex int, resourcePath, resourceFragment, resourceEndFragment string) {
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
		if chapters[index].Index == chapterIndex && (chapters[index].ResourcePath != resourcePath || chapters[index].ResourceFragment != resourceFragment || chapters[index].ResourceEndFragment != resourceEndFragment) {
			chapters[index].ResourcePath = resourcePath
			chapters[index].ResourceFragment = resourceFragment
			chapters[index].ResourceEndFragment = resourceEndFragment
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

func epubTOCRuleCanCarryFragments(rule string) bool {
	switch strings.ToLower(strings.TrimSpace(rule)) {
	case "toc", "toc+spin", "toc<spin":
		return true
	default:
		return false
	}
}

func normalizeResourceFragment(value string) (string, error) {
	return engine.NormalizeEPUBFragment(value)
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
