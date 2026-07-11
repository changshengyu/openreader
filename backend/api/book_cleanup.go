package api

import (
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
)

// bookCleanupPlan captures only derived artifacts. It is collected while the
// database rows still exist and executed only after the owning transaction has
// committed, so a failed database write cannot delete readable user data.
type bookCleanupPlan struct {
	remoteCachePaths []string
	privateLibrary   string
}

func (s *Server) captureBookCleanup(tx *gorm.DB, userID uint, book models.Book) (bookCleanupPlan, error) {
	plan := bookCleanupPlan{}
	var chapters []models.Chapter
	if err := tx.Select("cache_path").Where("book_id = ? AND cache_path <> ''", book.ID).Find(&chapters).Error; err != nil {
		return plan, err
	}
	if book.SourceID > 0 {
		for _, chapter := range chapters {
			plan.remoteCachePaths = append(plan.remoteCachePaths, chapter.CachePath)
		}
		return plan, nil
	}

	if strings.TrimSpace(book.LibraryPath) == "" {
		return plan, nil
	}
	var user models.User
	if err := tx.Select("username").First(&user, userID).Error; err != nil {
		return plan, err
	}
	if path, ok := s.privateImportedBookDirectory(user.Username, book.LibraryPath); ok {
		plan.privateLibrary = path
	}
	return plan, nil
}

func (s *Server) privateImportedBookDirectory(username, libraryPath string) (string, bool) {
	libraryPath = strings.TrimSpace(libraryPath)
	if libraryPath == "" || filepath.IsAbs(libraryPath) {
		return "", false
	}
	ownerName := engine.SafeFilename(username)
	if ownerName == "" {
		return "", false
	}
	ownerRoot := filepath.Join(s.cfg.LibraryDir, "data", ownerName)
	candidate := filepath.Join(s.cfg.LibraryDir, libraryPath)
	if _, ok := relativePathInside(ownerRoot, candidate); !ok {
		return "", false
	}
	return candidate, true
}

func (s *Server) cleanupDeletedBookArtifacts(plans []bookCleanupPlan) {
	paths := make([]string, 0)
	directories := make(map[string]struct{})
	for _, plan := range plans {
		paths = append(paths, plan.remoteCachePaths...)
		if plan.privateLibrary != "" {
			directories[plan.privateLibrary] = struct{}{}
		}
	}
	s.pruneUnreferencedRemoteCachePaths(paths)
	for directory := range directories {
		_ = os.RemoveAll(directory)
	}
}

func (s *Server) clearRemoteBookCacheRows(tx *gorm.DB, bookIDs []uint) (int, []string, error) {
	if len(bookIDs) == 0 {
		return 0, nil, nil
	}
	var chapters []models.Chapter
	if err := tx.Where("book_id IN ? AND cache_path <> ''", bookIDs).Find(&chapters).Error; err != nil {
		return 0, nil, err
	}
	paths := make([]string, 0, len(chapters))
	for _, chapter := range chapters {
		paths = append(paths, chapter.CachePath)
	}
	if len(paths) == 0 {
		return 0, paths, nil
	}
	if err := tx.Model(&models.Chapter{}).
		Where("book_id IN ? AND cache_path <> ''", bookIDs).
		Update("cache_path", "").Error; err != nil {
		return 0, nil, err
	}
	return len(paths), paths, nil
}

func (s *Server) pruneUnreferencedRemoteCachePaths(cachePaths []string) (int, int64) {
	paths := make(map[string]struct{})
	for _, cachePath := range cachePaths {
		if path, ok := s.remoteCacheFilePath(cachePath); ok {
			paths[path] = struct{}{}
		}
	}
	if len(paths) == 0 {
		return 0, 0
	}

	type cacheReference struct {
		CachePath string
	}
	var references []cacheReference
	if err := s.db.Model(&models.Chapter{}).
		Select("chapters.cache_path").
		Joins("JOIN books ON books.id = chapters.book_id").
		Where("books.source_id > 0 AND chapters.cache_path <> ''").
		Scan(&references).Error; err == nil {
		for _, reference := range references {
			if path, ok := s.remoteCacheFilePath(reference.CachePath); ok {
				delete(paths, path)
			}
		}
	}

	files := 0
	size := int64(0)
	for path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if err := os.Remove(path); err == nil {
			files++
			size += info.Size()
		}
	}
	return files, size
}
