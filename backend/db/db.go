package db

import (
	"os"
	"path/filepath"

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
	var books []models.Book
	if err := database.Where("source_id = 0 AND library_path <> ''").Find(&books).Error; err != nil {
		return err
	}
	for _, book := range books {
		var chapters []models.Chapter
		if err := database.Where("book_id = ? AND cache_path <> ''", book.ID).Find(&chapters).Error; err != nil {
			return err
		}
		contentDir := filepath.Join(cfg.LibraryDir, book.LibraryPath, "content")
		for _, chapter := range chapters {
			if filepath.IsAbs(chapter.CachePath) {
				continue
			}
			oldPath := filepath.Join(cfg.CacheDir, chapter.CachePath)
			if _, err := os.Stat(oldPath); err != nil {
				continue
			}
			newPath := filepath.Join(contentDir, chapter.CachePath)
			if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
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
			_ = os.Remove(oldPath)
			chapter.CachePath = newPath
			if err := database.Save(&chapter).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
