package api

import (
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/bookcatalog"
)

// bookCleanupPlan captures only derived artifacts. It is collected while the
// database rows still exist and executed only after the owning transaction has
// committed, so a failed database write cannot delete readable user data.
type bookCleanupPlan struct {
	remoteCachePaths     []string
	remoteImageBook      *models.Book
	privateLibrary       string
	privateLibraryUserID uint
}

func (s *Server) captureBookCleanup(tx *gorm.DB, userID uint, book models.Book) (bookCleanupPlan, error) {
	plan := bookCleanupPlan{}
	var chapters []models.Chapter
	if err := tx.Select("cache_path").Where("book_id = ? AND cache_path <> ''", book.ID).Find(&chapters).Error; err != nil {
		return plan, err
	}
	if book.SourceID > 0 {
		bookCopy := book
		plan.remoteImageBook = &bookCopy
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
	if path, ok := s.resolvedPrivateImportedBookDirectory(user.Username, book.LibraryPath); ok {
		plan.privateLibrary = path
		plan.privateLibraryUserID = userID
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

func (s *Server) resolvedPrivateImportedBookDirectory(username, libraryPath string) (string, bool) {
	resolvedOwnerRoot, resolved, relative, ok := s.resolvePrivateImportedBookDirectory(username, libraryPath)
	if !ok {
		return "", false
	}
	// A deletion candidate must not traverse a symlink below the configured
	// owner root. References may resolve such legacy aliases, but cleanup fails
	// closed instead of choosing and deleting the symlink target.
	if filepath.Clean(resolved) != filepath.Clean(filepath.Join(resolvedOwnerRoot, relative)) {
		return "", false
	}
	return resolved, true
}

func (s *Server) resolvePrivateImportedBookDirectory(username, libraryPath string) (string, string, string, bool) {
	candidate, ok := s.privateImportedBookDirectory(username, libraryPath)
	if !ok {
		return "", "", "", false
	}
	ownerRoot := filepath.Join(s.cfg.LibraryDir, "data", engine.SafeFilename(username))
	relative, ok := relativePathInside(ownerRoot, candidate)
	if !ok {
		return "", "", "", false
	}
	resolvedOwnerRoot, err := filepath.EvalSymlinks(ownerRoot)
	if err != nil {
		return "", "", "", false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", "", "", false
	}
	if !pathInside(resolvedOwnerRoot, resolved) {
		return "", "", "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", "", "", false
	}
	return resolvedOwnerRoot, resolved, relative, true
}

func (s *Server) cleanupDeletedBookArtifacts(plans []bookCleanupPlan) {
	paths := make([]string, 0)
	type privateDirectory struct {
		path   string
		userID uint
	}
	directories := make(map[privateDirectory]struct{})
	for _, plan := range plans {
		paths = append(paths, plan.remoteCachePaths...)
		if plan.remoteImageBook != nil {
			_, _ = s.chapterImages.RemoveBook(*plan.remoteImageBook)
		}
		if plan.privateLibrary != "" {
			directories[privateDirectory{path: plan.privateLibrary, userID: plan.privateLibraryUserID}] = struct{}{}
		}
	}
	s.pruneUnreferencedRemoteCachePaths(paths)
	for directory := range directories {
		if s.privateImportedBookDirectoryReferenced(directory.userID, directory.path) {
			continue
		}
		_ = os.RemoveAll(directory.path)
	}
}

func (s *Server) privateImportedBookDirectoryReferenced(userID uint, target string) bool {
	var user models.User
	if err := s.db.Select("username").First(&user, userID).Error; err != nil {
		return true
	}
	var references []struct {
		LibraryPath string
	}
	if err := s.db.Model(&models.Book{}).
		Select("library_path").
		Where("user_id = ? AND source_id = 0 AND library_path <> ''", userID).
		Find(&references).Error; err != nil {
		return true
	}
	target = filepath.Clean(target)
	for _, reference := range references {
		_, candidate, _, ok := s.resolvePrivateImportedBookDirectory(user.Username, reference.LibraryPath)
		if ok && filepath.Clean(candidate) == target {
			return true
		}
	}
	return false
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

// replaceBookChapterRows is the durable catalogue boundary used by refresh and
// source-change operations. Chapter rows are intentionally replaced instead of
// patched by index: a source can remove a chapter or change its URL, and a
// retained CachePath would otherwise make the reader serve content from the
// former catalogue. The caller may prune the returned paths only after this
// transaction commits.
func (s *Server) replaceBookChapterRows(tx *gorm.DB, userID, bookID uint, next []models.Chapter) ([]string, map[int]uint, error) {
	return bookcatalog.ReplaceChapterRows(tx, userID, bookID, next)
}

func (s *Server) pruneUnreferencedRemoteCachePaths(cachePaths []string) (int, int64) {
	s.remoteCacheMu.Lock()
	defer s.remoteCacheMu.Unlock()

	storage, err := s.remoteCacheStorage()
	if err != nil {
		return 0, 0
	}
	paths := make(map[string]struct{})
	for _, cachePath := range cachePaths {
		if relative, err := s.remoteCacheRelativePath(cachePath); err == nil {
			paths[relative] = struct{}{}
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
		Find(&references).Error; err != nil {
		return 0, 0
	}
	for _, reference := range references {
		relative, err := s.remoteCacheRelativePath(reference.CachePath)
		if err != nil {
			return 0, 0
		}
		delete(paths, relative)
	}

	files := 0
	size := int64(0)
	for relative := range paths {
		if removed, bytes := s.removeRemoteCacheFile(storage, relative); removed {
			files++
			size += bytes
		}
	}
	return files, size
}
