package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

func (s *Server) cacheStats(c *gin.Context) {
	stats, err := s.remoteCacheStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count cached chapters"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":           s.cfg.CacheDir,
		"files":          stats.files,
		"size":           stats.size,
		"cachedChapters": stats.chapters,
	})
}

func (s *Server) clearCache(c *gin.Context) {
	cacheDir, err := filepath.Abs(s.cfg.CacheDir)
	if err != nil || cacheDir == string(os.PathSeparator) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid cache directory"})
		return
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to recreate cache directory"})
		return
	}

	var chapters []models.Chapter
	if err := s.db.
		Joins("JOIN books ON books.id = chapters.book_id").
		Where("books.source_id > 0 AND chapters.cache_path <> ''").
		Find(&chapters).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list cached chapters"})
		return
	}

	files := 0
	size := int64(0)
	for _, chapter := range chapters {
		if deleted, deletedSize := s.deleteRemoteCacheFile(chapter.CachePath); deleted {
			files++
			size += deletedSize
		}
	}

	if err := s.db.Model(&models.Chapter{}).
		Where("book_id IN (?)", s.db.Model(&models.Book{}).Select("id").Where("source_id > 0")).
		Where("cache_path <> ''").
		Update("cache_path", "").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset chapter cache state"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"clearedFiles": files, "clearedSize": size})
}

type cacheStatSummary struct {
	files    int
	size     int64
	chapters int64
}

func (s *Server) remoteCacheStats() (cacheStatSummary, error) {
	var chapters []models.Chapter
	if err := s.db.
		Joins("JOIN books ON books.id = chapters.book_id").
		Where("books.source_id > 0 AND chapters.cache_path <> ''").
		Find(&chapters).Error; err != nil {
		return cacheStatSummary{}, err
	}

	seen := map[string]struct{}{}
	summary := cacheStatSummary{chapters: int64(len(chapters))}
	for _, chapter := range chapters {
		path, ok := s.remoteCacheFilePath(chapter.CachePath)
		if !ok {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		summary.files++
		summary.size += info.Size()
	}
	return summary, nil
}

func (s *Server) deleteRemoteCacheFile(cachePath string) (bool, int64) {
	path, ok := s.remoteCacheFilePath(cachePath)
	if !ok {
		return false, 0
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false, 0
	}
	size := info.Size()
	if err := os.Remove(path); err != nil {
		return false, 0
	}
	return true, size
}

func (s *Server) remoteCacheFilePath(cachePath string) (string, bool) {
	if cachePath == "" || filepath.IsAbs(cachePath) {
		return "", false
	}
	fullPath := filepath.Join(s.cfg.CacheDir, cachePath)
	cleanPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", false
	}
	cleanCacheDir, err := filepath.Abs(s.cfg.CacheDir)
	if err != nil {
		return "", false
	}
	if cleanPath != cleanCacheDir && !startsWithPath(cleanPath, cleanCacheDir) {
		return "", false
	}
	return cleanPath, true
}

func startsWithPath(path, parent string) bool {
	return len(path) > len(parent) && path[:len(parent)] == parent && path[len(parent)] == os.PathSeparator
}

func directoryStats(root string) (int, int64) {
	var fileCount int
	var totalSize int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		fileCount++
		totalSize += info.Size()
		return nil
	})
	return fileCount, totalSize
}
