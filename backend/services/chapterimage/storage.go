package chapterimage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"openreader/backend/models"
)

type manifestEntry struct {
	Key         string `json:"key"`
	ContentType string `json:"contentType"`
	Fingerprint string `json:"fingerprint"`
	Size        int64  `json:"size"`
}

type referenceManifest struct {
	Version   int             `json:"version"`
	ChapterID uint            `json:"chapterId"`
	Images    []manifestEntry `json:"images"`
}

type FileStats struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

func (s *Service) bookRoot(book models.Book) (string, error) {
	return s.resolveBookRoot(book, true)
}

func (s *Service) existingBookRoot(book models.Book) (string, error) {
	return s.resolveBookRoot(book, false)
}

func (s *Service) resolveBookRoot(book models.Book, create bool) (string, error) {
	if book.ID == 0 || book.UserID == 0 || book.SourceID == 0 {
		return "", ErrInvalidInput
	}
	cacheRoot, err := s.safeCacheRoot()
	if err != nil {
		return "", err
	}
	root := filepath.Join(cacheRoot, "chapter-images", fmt.Sprintf("user-%d", book.UserID), fmt.Sprintf("book-%d", book.ID))
	if create {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return "", err
		}
	} else {
		info, err := os.Lstat(root)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafePath
		}
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || !pathWithin(cacheRoot, resolved) {
		return "", ErrUnsafePath
	}
	return resolved, nil
}

func (s *Service) safeCacheRoot() (string, error) {
	root, err := filepath.Abs(s.cfg.CacheDir)
	if err != nil || strings.TrimSpace(s.cfg.CacheDir) == "" {
		return "", ErrUnsafePath
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func pathWithin(root, target string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func blobPath(root, key string) (string, error) {
	if !validImageKey(key) {
		return "", ErrUnsafePath
	}
	path := filepath.Join(root, "blobs", key)
	if !pathWithin(root, path) {
		return "", ErrUnsafePath
	}
	return path, nil
}

func manifestPath(root string, chapterID uint) (string, error) {
	if chapterID == 0 {
		return "", ErrInvalidInput
	}
	path := filepath.Join(root, "refs", fmt.Sprintf("chapter-%d.json", chapterID))
	if !pathWithin(root, path) {
		return "", ErrUnsafePath
	}
	return path, nil
}

func (s *Service) readManifest(book models.Book, chapterID uint) (referenceManifest, error) {
	root, err := s.existingBookRoot(book)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return referenceManifest{Version: 1, ChapterID: chapterID}, nil
		}
		return referenceManifest{}, err
	}
	path, err := manifestPath(root, chapterID)
	if err != nil {
		return referenceManifest{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return referenceManifest{Version: 1, ChapterID: chapterID}, nil
		}
		return referenceManifest{}, err
	}
	if !info.Mode().IsRegular() {
		return referenceManifest{}, ErrUnsafePath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return referenceManifest{}, err
	}
	return decodeReferenceManifest(data, chapterID)
}

func decodeReferenceManifest(data []byte, chapterID uint) (referenceManifest, error) {
	var manifest referenceManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.Version != 1 || manifest.ChapterID != chapterID || chapterID == 0 {
		return referenceManifest{}, ErrUnsafePath
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return referenceManifest{}, ErrUnsafePath
	}
	for _, entry := range manifest.Images {
		if !validImageKey(entry.Key) || entry.Size <= 0 || entry.ContentType == "" || len(entry.Fingerprint) != sha256.Size*2 {
			return referenceManifest{}, ErrUnsafePath
		}
		if _, err := hex.DecodeString(entry.Fingerprint); err != nil {
			return referenceManifest{}, ErrUnsafePath
		}
	}
	return manifest, nil
}

func writeManifestAtomic(root string, manifest referenceManifest) error {
	path, err := manifestPath(root, manifest.ChapterID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

func writeBlobAtomic(root, key string, data []byte) (string, error) {
	path, err := blobPath(root, key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() {
			return "", ErrUnsafePath
		}
		return path, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	if err := writeAtomic(path, data); err != nil {
		return "", err
	}
	return path, nil
}

func writeAtomic(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".chapter-image-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return ErrUnsafePath
	}
	return os.Rename(temporaryPath, path)
}

func validatedEntry(root string, entry manifestEntry) (manifestEntry, string, error) {
	path, err := blobPath(root, entry.Key)
	if err != nil {
		return manifestEntry{}, "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifestEntry{}, "", ErrNotFound
		}
		return manifestEntry{}, "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return manifestEntry{}, "", ErrUnsafePath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return manifestEntry{}, "", err
	}
	contentType, ok := detectImageType(data)
	if !ok {
		return manifestEntry{}, "", ErrUnsupportedImage
	}
	fingerprint := fingerprintBytes(data)
	if entry.Fingerprint != "" && !strings.EqualFold(entry.Fingerprint, fingerprint) {
		return manifestEntry{}, "", ErrInvalidCapability
	}
	if entry.ContentType != "" && entry.ContentType != contentType {
		return manifestEntry{}, "", ErrInvalidCapability
	}
	return manifestEntry{Key: entry.Key, ContentType: contentType, Fingerprint: fingerprint, Size: int64(len(data))}, path, nil
}

func fingerprintBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Service) RemoveChapterReferences(book models.Book, chapterIDs []uint) (FileStats, error) {
	root, err := s.existingBookRoot(book)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileStats{}, nil
		}
		return FileStats{}, err
	}
	stats := FileStats{}
	for _, chapterID := range chapterIDs {
		path, pathErr := manifestPath(root, chapterID)
		if pathErr != nil {
			return stats, pathErr
		}
		if removed, size, removeErr := removeRegular(path); removeErr != nil {
			return stats, removeErr
		} else if removed {
			stats.Files++
			stats.Bytes += size
		}
	}
	pruned, err := pruneUnreferencedBlobs(root)
	stats.Files += pruned.Files
	stats.Bytes += pruned.Bytes
	return stats, err
}

func (s *Service) RemoveBook(book models.Book) (FileStats, error) {
	root, err := s.existingBookRoot(book)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileStats{}, nil
		}
		return FileStats{}, err
	}
	stats := treeStats(root)
	if err := os.RemoveAll(root); err != nil {
		return FileStats{}, err
	}
	return stats, nil
}

func (s *Service) StatsUser(userID uint) (FileStats, error) {
	if s == nil || userID == 0 {
		return FileStats{}, ErrInvalidInput
	}
	cacheRoot, err := s.safeCacheRoot()
	if err != nil {
		return FileStats{}, err
	}
	root := filepath.Join(cacheRoot, "chapter-images", fmt.Sprintf("user-%d", userID))
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileStats{}, nil
		}
		return FileStats{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !pathWithin(cacheRoot, root) {
		return FileStats{}, ErrUnsafePath
	}
	stats := FileStats{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return ErrUnsafePath
		}
		stats.Files++
		stats.Bytes += info.Size()
		return nil
	})
	return stats, err
}

func removeRegular(path string) (bool, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if !info.Mode().IsRegular() {
		return false, 0, ErrUnsafePath
	}
	if err := os.Remove(path); err != nil {
		return false, 0, err
	}
	return true, info.Size(), nil
}

func pruneUnreferencedBlobs(root string) (FileStats, error) {
	referenced := make(map[string]struct{})
	refsRoot := filepath.Join(root, "refs")
	entries, err := os.ReadDir(refsRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return FileStats{}, err
	}
	for _, item := range entries {
		if !strings.HasSuffix(item.Name(), ".json") {
			continue
		}
		path := filepath.Join(refsRoot, item.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return FileStats{}, statErr
		}
		if !info.Mode().IsRegular() {
			return FileStats{}, ErrUnsafePath
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return FileStats{}, readErr
		}
		chapterID, parseErr := chapterIDFromManifestName(item.Name())
		if parseErr != nil {
			return FileStats{}, ErrUnsafePath
		}
		manifest, manifestErr := decodeReferenceManifest(data, chapterID)
		if manifestErr != nil {
			return FileStats{}, manifestErr
		}
		for _, image := range manifest.Images {
			referenced[image.Key] = struct{}{}
		}
	}
	blobsRoot := filepath.Join(root, "blobs")
	blobs, err := os.ReadDir(blobsRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileStats{}, nil
		}
		return FileStats{}, err
	}
	stats := FileStats{}
	for _, blob := range blobs {
		if blob.IsDir() || !validImageKey(blob.Name()) {
			continue
		}
		if _, keep := referenced[blob.Name()]; keep {
			continue
		}
		removed, size, removeErr := removeRegular(filepath.Join(blobsRoot, blob.Name()))
		if removeErr != nil {
			return stats, removeErr
		}
		if removed {
			stats.Files++
			stats.Bytes += size
		}
	}
	return stats, nil
}

func chapterIDFromManifestName(name string) (uint, error) {
	if !strings.HasPrefix(name, "chapter-") || !strings.HasSuffix(name, ".json") {
		return 0, ErrUnsafePath
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(name, "chapter-"), ".json")
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, ErrUnsafePath
	}
	return uint(value), nil
}

func treeStats(root string) FileStats {
	stats := FileStats{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr == nil && info.Mode().IsRegular() {
			stats.Files++
			stats.Bytes += info.Size()
		}
		return nil
	})
	return stats
}

func canonicalEntries(entries map[string]manifestEntry) []manifestEntry {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]manifestEntry, 0, len(keys))
	for _, key := range keys {
		result = append(result, entries[key])
	}
	return result
}
