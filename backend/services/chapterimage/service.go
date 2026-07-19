package chapterimage

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

const capabilityTTL = 12 * time.Hour

type Limits struct {
	MaxImages     int
	MaxImageBytes int64
	MaxTotalBytes int64
	Timeout       time.Duration
	MaxRedirects  int
}

type CacheResult struct {
	Found      int `json:"found"`
	Downloaded int `json:"downloaded"`
	Reused     int `json:"reused"`
	Failed     int `json:"failed"`
}

type Resource struct {
	Path        string
	ContentType string
	Size        int64
	Data        []byte
}

type CachedFile struct {
	OriginalURL string
	Key         string
	ContentType string
	Size        int64
	Data        []byte
}

type downloadCall struct {
	done  chan struct{}
	entry manifestEntry
	err   error
}

type Service struct {
	cfg           config.Config
	db            *gorm.DB
	now           func() time.Time
	limits        Limits
	lookupIP      lookupIPFunc
	clientFactory func(requestPolicy) *http.Client

	mu        sync.Mutex
	downloads map[string]*downloadCall
}

func New(cfg config.Config, database *gorm.DB) *Service {
	service := &Service{
		cfg: cfg,
		db:  database,
		now: time.Now,
		limits: Limits{
			MaxImages:     positiveInt(cfg.MaxChapterImages, 64),
			MaxImageBytes: positiveInt64(cfg.MaxChapterImageBytes, 8*1024*1024),
			MaxTotalBytes: positiveInt64(cfg.MaxChapterImageTotalBytes, 32*1024*1024),
			Timeout:       time.Duration(positiveInt(cfg.ChapterImageTimeoutSeconds, 12)) * time.Second,
			MaxRedirects:  positiveInt(cfg.MaxChapterImageRedirects, 3),
		},
		downloads: make(map[string]*downloadCall),
	}
	service.lookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		ips := make([]net.IP, 0, len(addresses))
		for _, address := range addresses {
			ips = append(ips, address.IP)
		}
		return ips, nil
	}
	service.clientFactory = func(policy requestPolicy) *http.Client {
		return defaultClientForPolicy(policy, service.limits.Timeout)
	}
	return service
}

func positiveInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func (s *Service) CacheChapter(ctx context.Context, source models.BookSource, book models.Book, chapter models.Chapter, content string) (CacheResult, error) {
	if s == nil || s.db == nil || book.ID == 0 || book.UserID == 0 || book.SourceID == 0 ||
		source.ID != book.SourceID || chapter.ID == 0 || chapter.BookID != book.ID {
		return CacheResult{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return CacheResult{}, err
	}
	references := extractImageReferences(content, chapter.URL, s.limits.MaxImages)
	result := CacheResult{Found: len(references)}
	if len(references) == 0 {
		_, err := s.RemoveChapterReferences(book, []uint{chapter.ID})
		return result, err
	}
	root, err := s.bookRoot(book)
	if err != nil {
		return result, err
	}
	published := false
	defer func() {
		if !published {
			_, _ = pruneUnreferencedBlobs(root)
		}
	}()
	oldManifest, oldErr := s.readManifest(book, chapter.ID)
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return result, oldErr
	}
	oldEntries := make(map[string]manifestEntry, len(oldManifest.Images))
	oldUsable := false
	for _, entry := range oldManifest.Images {
		oldEntries[entry.Key] = entry
		if !oldUsable {
			_, _, validErr := validatedEntry(root, entry)
			oldUsable = validErr == nil
		}
	}
	entries := make(map[string]manifestEntry, len(references))
	remaining := s.limits.MaxTotalBytes
	policy := buildRequestPolicy(source, book, chapter, s.lookupIP, s.limits.MaxRedirects)
	client := s.clientFactory(policy)
	for _, reference := range references {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if old, exists := oldEntries[reference.Key]; exists {
			if valid, _, validErr := validatedEntry(root, old); validErr == nil && valid.Size <= remaining {
				entries[reference.Key] = valid
				remaining -= valid.Size
				result.Reused++
				continue
			}
		}
		if remaining <= 0 {
			result.Failed++
			continue
		}
		maxBytes := s.limits.MaxImageBytes
		if remaining < maxBytes {
			maxBytes = remaining
		}
		entry, reused, cacheErr := s.cacheOne(ctx, root, book, reference, client, policy, maxBytes)
		if cacheErr != nil {
			if errors.Is(cacheErr, context.Canceled) || errors.Is(cacheErr, context.DeadlineExceeded) {
				return result, cacheErr
			}
			result.Failed++
			continue
		}
		if entry.Size > remaining {
			result.Failed++
			continue
		}
		entries[reference.Key] = entry
		remaining -= entry.Size
		if reused {
			result.Reused++
		} else {
			result.Downloaded++
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if result.Failed > 0 && oldUsable {
		return result, nil
	}
	manifest := referenceManifest{Version: 1, ChapterID: chapter.ID, Images: canonicalEntries(entries)}
	if err := writeManifestAtomic(root, manifest); err != nil {
		return result, err
	}
	published = true
	_, _ = pruneUnreferencedBlobs(root)
	return result, nil
}

func (s *Service) cacheOne(ctx context.Context, root string, book models.Book, reference imageReference, client *http.Client, policy requestPolicy, maxBytes int64) (manifestEntry, bool, error) {
	placeholder := manifestEntry{Key: reference.Key}
	if existing, reusable, err := prepareExistingBlob(root, placeholder, maxBytes); err != nil {
		return manifestEntry{}, false, err
	} else if reusable {
		return existing, true, nil
	}
	callKey := strings.Join([]string{uintString(book.UserID), uintString(book.ID), reference.Key}, ":")
	s.mu.Lock()
	if active := s.downloads[callKey]; active != nil {
		s.mu.Unlock()
		select {
		case <-active.done:
			return active.entry, true, active.err
		case <-ctx.Done():
			return manifestEntry{}, false, ctx.Err()
		}
	}
	call := &downloadCall{done: make(chan struct{})}
	s.downloads[callKey] = call
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.downloads, callKey)
		close(call.done)
		s.mu.Unlock()
	}()
	if existing, reusable, err := prepareExistingBlob(root, placeholder, maxBytes); err != nil {
		call.err = err
		return manifestEntry{}, false, err
	} else if reusable {
		call.entry = existing
		return existing, true, nil
	}

	data, contentType, err := fetchImageBytes(ctx, client, policy, reference.URL, maxBytes)
	if err != nil {
		call.err = err
		return manifestEntry{}, false, err
	}
	path, err := writeBlobAtomic(root, reference.Key, data)
	if err != nil {
		call.err = err
		return manifestEntry{}, false, err
	}
	if !pathWithin(root, path) {
		call.err = ErrUnsafePath
		return manifestEntry{}, false, call.err
	}
	fetched := manifestEntry{
		Key:         reference.Key,
		ContentType: contentType,
		Fingerprint: fingerprintBytes(data),
		Size:        int64(len(data)),
	}
	actual, _, err := validatedEntry(root, placeholder)
	if err != nil {
		call.err = err
		return manifestEntry{}, false, err
	}
	if actual.Size > maxBytes {
		call.err = ErrImageLimit
		return manifestEntry{}, false, call.err
	}
	call.entry = actual
	return call.entry, actual.Fingerprint != fetched.Fingerprint, nil
}

func prepareExistingBlob(root string, placeholder manifestEntry, maxBytes int64) (manifestEntry, bool, error) {
	existing, path, err := validatedEntry(root, placeholder)
	if err == nil {
		if existing.Size > maxBytes {
			return manifestEntry{}, false, ErrImageLimit
		}
		return existing, true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return manifestEntry{}, false, nil
	}
	if !errors.Is(err, ErrUnsupportedImage) {
		return manifestEntry{}, false, err
	}
	if path == "" {
		var pathErr error
		path, pathErr = blobPath(root, placeholder.Key)
		if pathErr != nil {
			return manifestEntry{}, false, pathErr
		}
	}
	if _, _, removeErr := removeRegular(path); removeErr != nil {
		return manifestEntry{}, false, removeErr
	}
	return manifestEntry{}, false, nil
}

func (s *Service) CachedImages(book models.Book, chapter models.Chapter, content string) (map[string]string, time.Time, error) {
	if s == nil || chapter.BookID != book.ID || book.ID == 0 || book.UserID == 0 || book.SourceID == 0 {
		return nil, time.Time{}, ErrInvalidInput
	}
	manifest, err := s.readManifest(book, chapter.ID)
	if err != nil {
		return nil, time.Time{}, err
	}
	if len(manifest.Images) == 0 {
		return nil, time.Time{}, nil
	}
	root, err := s.existingBookRoot(book)
	if err != nil {
		return nil, time.Time{}, err
	}
	byKey := make(map[string]manifestEntry, len(manifest.Images))
	for _, entry := range manifest.Images {
		byKey[entry.Key] = entry
	}
	expiresAt := s.now().UTC().Add(capabilityTTL)
	mapping := make(map[string]string)
	for _, reference := range extractImageReferences(content, chapter.URL, s.limits.MaxImages) {
		entry, exists := byKey[reference.Key]
		if !exists {
			continue
		}
		valid, _, validErr := validatedEntry(root, entry)
		if validErr != nil {
			continue
		}
		token, signErr := signCapability(s.cfg.JWTSecret, capabilityClaims{
			UserID:      book.UserID,
			BookID:      book.ID,
			SourceID:    book.SourceID,
			Key:         valid.Key,
			Fingerprint: valid.Fingerprint,
			Purpose:     capabilityPurpose,
			ExpiresAt:   expiresAt.Unix(),
		})
		if signErr != nil {
			return nil, time.Time{}, signErr
		}
		mapping[reference.URL] = "/api/chapter-image/" + url.PathEscape(token)
	}
	if len(mapping) == 0 {
		return nil, time.Time{}, nil
	}
	return mapping, expiresAt, nil
}

func (s *Service) OpenResource(token string) (Resource, error) {
	claims, err := verifyCapability(s.cfg.JWTSecret, token, s.now().UTC())
	if err != nil {
		return Resource{}, err
	}
	var book models.Book
	if err := s.db.Where("id = ? AND user_id = ? AND source_id = ?", claims.BookID, claims.UserID, claims.SourceID).First(&book).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	root, err := s.existingBookRoot(book)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	entry, path, err := validatedEntry(root, manifestEntry{Key: claims.Key, Fingerprint: claims.Fingerprint})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Resource{}, ErrNotFound
		}
		return Resource{}, err
	}
	contentType, supported := detectImageType(data)
	if !supported || contentType != entry.ContentType || fingerprintBytes(data) != claims.Fingerprint {
		return Resource{}, ErrInvalidCapability
	}
	return Resource{Path: path, ContentType: contentType, Size: int64(len(data)), Data: data}, nil
}

func (s *Service) CachedFiles(book models.Book, chapter models.Chapter, content string) ([]CachedFile, error) {
	if s == nil || chapter.BookID != book.ID || book.ID == 0 || book.UserID == 0 || book.SourceID == 0 {
		return nil, ErrInvalidInput
	}
	manifest, err := s.readManifest(book, chapter.ID)
	if err != nil {
		return nil, err
	}
	if len(manifest.Images) == 0 {
		return nil, nil
	}
	root, err := s.existingBookRoot(book)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]manifestEntry, len(manifest.Images))
	for _, entry := range manifest.Images {
		byKey[entry.Key] = entry
	}
	files := make([]CachedFile, 0, len(manifest.Images))
	for _, reference := range extractImageReferences(content, chapter.URL, s.limits.MaxImages) {
		entry, exists := byKey[reference.Key]
		if !exists {
			continue
		}
		valid, path, validErr := validatedEntry(root, entry)
		if validErr != nil {
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil || int64(len(data)) != valid.Size || fingerprintBytes(data) != valid.Fingerprint {
			continue
		}
		files = append(files, CachedFile{
			OriginalURL: reference.URL,
			Key:         valid.Key,
			ContentType: valid.ContentType,
			Size:        valid.Size,
			Data:        data,
		})
	}
	return files, nil
}

func ExtensionForContentType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/avif":
		return ".avif"
	default:
		return ""
	}
}

func uintString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
