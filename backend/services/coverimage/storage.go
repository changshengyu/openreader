package coverimage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type FileStats struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type cacheEntry struct {
	path    string
	modTime time.Time
	size    int64
}

func (s *Service) cacheRoot() (string, error) {
	if strings.TrimSpace(s.cfg.CacheDir) == "" {
		return "", ErrUnsafePath
	}
	base, err := filepath.Abs(s.cfg.CacheDir)
	if err != nil {
		return "", ErrUnsafePath
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, "cover-images")
	if info, statErr := os.Lstat(root); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafePath
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	} else if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || !pathWithin(base, resolved) {
		return "", ErrUnsafePath
	}
	return resolved, nil
}

func (s *Service) userRoot(userID uint, create bool) (string, error) {
	if userID == 0 {
		return "", ErrUnsafePath
	}
	root, err := s.cacheRoot()
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, "user-"+strconv.FormatUint(uint64(userID), 10))
	info, err := os.Lstat(target)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || !create {
			return "", err
		}
		if err := os.Mkdir(target, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		info, err = os.Lstat(target)
		if err != nil {
			return "", err
		}
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrUnsafePath
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil || !pathWithin(root, resolved) {
		return "", ErrUnsafePath
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

func coverCacheKey(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:])
}

func (s *Service) cachePath(userID uint, rawURL string, create bool) (string, error) {
	root, err := s.userRoot(userID, create)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, coverCacheKey(rawURL)+".img")
	if !pathWithin(root, target) {
		return "", ErrUnsafePath
	}
	return target, nil
}

func (s *Service) readCached(userID uint, rawURL string) (Resource, error) {
	path, err := s.cachePath(userID, rawURL, false)
	if err != nil {
		return Resource{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Resource{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > s.limits.MaxImageBytes {
		_ = os.Remove(path)
		return Resource{}, os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return Resource{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, s.limits.MaxImageBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return Resource{}, readErr
	}
	if closeErr != nil || int64(len(data)) != info.Size() {
		return Resource{}, os.ErrNotExist
	}
	contentType, ok := detectImageType(data)
	if !ok || !validImageBytes(data, contentType) {
		_ = os.Remove(path)
		return Resource{}, os.ErrNotExist
	}
	now := s.now().UTC()
	_ = os.Chtimes(path, now, now)
	return Resource{Data: data, ContentType: contentType, Size: int64(len(data))}, nil
}

func (s *Service) writeCached(userID uint, rawURL string, data []byte) error {
	path, err := s.cachePath(userID, rawURL, true)
	if err != nil {
		return err
	}
	parent := filepath.Dir(path)
	temporary, err := os.CreateTemp(parent, ".cover-*.tmp")
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	now := s.now().UTC()
	_ = os.Chtimes(path, now, now)
	return s.enforceUserLimit(userID, path)
}

func (s *Service) enforceUserLimit(userID uint, keepPath string) error {
	root, err := s.userRoot(userID, false)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	files := make([]cacheEntry, 0, len(entries))
	var total int64
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, infoErr := os.Lstat(path)
		if infoErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		files = append(files, cacheEntry{path: path, modTime: info.ModTime(), size: info.Size()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, entry := range files {
		if total <= s.limits.MaxCacheBytes {
			break
		}
		if entry.path == keepPath {
			continue
		}
		if err := os.Remove(entry.path); err == nil {
			total -= entry.size
		}
	}
	if total > s.limits.MaxCacheBytes {
		_ = os.Remove(keepPath)
		return ErrCacheLimit
	}
	return nil
}

func (s *Service) StatsUser(userID uint) (FileStats, error) {
	root, err := s.userRoot(userID, false)
	if errors.Is(err, os.ErrNotExist) {
		return FileStats{}, nil
	}
	if err != nil {
		return FileStats{}, err
	}
	return directoryStats(root)
}

func (s *Service) removeUserCache(userID uint) (FileStats, error) {
	root, err := s.userRoot(userID, false)
	if errors.Is(err, os.ErrNotExist) {
		return FileStats{}, nil
	}
	if err != nil {
		return FileStats{}, err
	}
	stats, err := directoryStats(root)
	if err != nil {
		return FileStats{}, err
	}
	if err := os.RemoveAll(root); err != nil {
		return FileStats{}, err
	}
	return stats, nil
}

func directoryStats(root string) (FileStats, error) {
	var stats FileStats
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if info.Mode().IsRegular() {
			stats.Files++
			stats.Bytes += info.Size()
		}
		return nil
	})
	return stats, err
}
