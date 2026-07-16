package db

import (
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/models"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o755); err != nil {
		return nil, err
	}

	database, err := gorm.Open(sqlite.Open(cfg.DatabasePath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
	}
	for _, statement := range pragmas {
		if err := database.Exec(statement).Error; err != nil {
			return nil, err
		}
	}

	return database, nil
}

func AutoMigrate(database *gorm.DB) error {
	return database.AutoMigrate(
		&models.User{},
		&models.UserSetting{},
		&models.BookSource{},
		&models.SourceFailure{},
		&models.ReplaceRule{},
		&models.RSSSource{},
		&models.RSSArticle{},
		&models.Category{},
		&models.Book{},
		&models.BookCategory{},
		&models.Chapter{},
		&models.ReadingProgress{},
		&models.Bookmark{},
	)
}

func MigrateLocalBookCache(database *gorm.DB, cfg config.Config) error {
	cacheRoot, err := canonicalDirectory(cfg.CacheDir)
	if err != nil {
		return err
	}
	libraryRoot, err := canonicalDirectory(cfg.LibraryDir)
	if err != nil {
		return err
	}
	var books []models.Book
	if err := database.Where("source_id = 0 AND library_path <> ''").Find(&books).Error; err != nil {
		return err
	}
	for _, book := range books {
		bookRelativePath, ok := cleanRelativeLocalPath(book.LibraryPath)
		if !ok {
			continue
		}
		var chapters []models.Chapter
		if err := database.Where("book_id = ? AND cache_path <> ''", book.ID).Find(&chapters).Error; err != nil {
			return err
		}
		for _, chapter := range chapters {
			cacheRelativePath, ok := cleanRelativeLocalPath(chapter.CachePath)
			if !ok {
				continue
			}
			oldPath, ok := existingRegularFileUnder(cacheRoot, cacheRelativePath)
			if !ok {
				continue
			}
			bookRoot, ok := ensureDirectoryUnder(libraryRoot, bookRelativePath)
			if !ok {
				continue
			}
			contentDir, ok := ensureDirectoryUnder(bookRoot, "content")
			if !ok {
				continue
			}
			newParent, ok := ensureDirectoryUnder(contentDir, filepath.Dir(cacheRelativePath))
			if !ok {
				continue
			}
			newPath := filepath.Join(newParent, filepath.Base(cacheRelativePath))
			if !pathInside(contentDir, newPath) {
				continue
			}
			newCachePath := filepath.Join("content", cacheRelativePath)
			if _, err := os.Stat(newPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			if _, err := os.Stat(newPath); err != nil {
				data, readErr := os.ReadFile(oldPath)
				if readErr != nil {
					return readErr
				}
				if writeErr := os.WriteFile(newPath, data, 0o644); writeErr != nil {
					return writeErr
				}
			}
			// Persist a relative path so a later Docker host move does not turn a
			// successfully migrated chapter into another stale absolute path. Keep
			// the old cache until the SQLite row points at its private archive copy:
			// a database failure may leave a harmless duplicate, but never loses the
			// only readable chapter body.
			chapter.CachePath = newCachePath
			if err := database.Save(&chapter).Error; err != nil {
				return err
			}
			_ = os.Remove(oldPath)
		}
	}
	return nil
}

func cleanRelativeLocalPath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) || strings.ContainsRune(value, 0) {
		return "", false
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

func canonicalDirectory(path string) (string, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = os.ErrNotExist
		}
		return "", err
	}
	return resolved, nil
}

func existingRegularFileUnder(root, relativePath string) (string, bool) {
	path, ok := joinUnder(root, relativePath)
	if !ok {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() || !pathInside(root, resolved) {
		return "", false
	}
	return resolved, true
}

func ensureDirectoryUnder(root, relativePath string) (string, bool) {
	if relativePath == "." || relativePath == "" {
		return root, true
	}
	clean, ok := cleanRelativeLocalPath(relativePath)
	if !ok {
		return "", false
	}
	current := root
	for _, segment := range strings.Split(clean, string(filepath.Separator)) {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
		candidate := filepath.Join(current, segment)
		if err := os.Mkdir(candidate, 0o755); err != nil && !os.IsExist(err) {
			return "", false
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || !pathInside(root, resolved) {
			return "", false
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return "", false
		}
		current = resolved
	}
	return current, true
}

func joinUnder(root, relativePath string) (string, bool) {
	clean, ok := cleanRelativeLocalPath(relativePath)
	if !ok {
		return "", false
	}
	candidate := filepath.Join(root, clean)
	if !pathInside(root, candidate) {
		return "", false
	}
	return candidate, true
}

func pathInside(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbs, candidateAbs)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
