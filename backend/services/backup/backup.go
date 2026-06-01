package backup

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"openreader/backend/models"
)

// Service handles automated backups.
type Service struct {
	db        *gorm.DB
	webdavDir string
	stopCh    chan struct{}
}

// New creates a backup service.
func New(db *gorm.DB, webdavDir string) *Service {
	return &Service{
		db:        db,
		webdavDir: webdavDir,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the daily backup schedule (23:50).
func (s *Service) Start() {
	go s.loop()
	log.Println("backup service started, scheduled at 23:50 daily")
}

// Stop gracefully stops the backup service.
func (s *Service) Stop() {
	close(s.stopCh)
}

func (s *Service) loop() {
	for {
		next := nextScheduledTime(23, 50)
		select {
		case <-time.After(time.Until(next)):
			s.run()
		case <-s.stopCh:
			return
		}
	}
}

func nextScheduledTime(hour, minute int) time.Time {
	now := time.Now()
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if target.Before(now) {
		target = target.Add(24 * time.Hour)
	}
	return target
}

// RunNow triggers an immediate backup. Returns the backup file path.
func (s *Service) RunNow() (string, error) {
	return s.run()
}

func (s *Service) run() (string, error) {
	backupPath := filepath.Join(s.webdavDir, fmt.Sprintf("backup_%s.zip", time.Now().Format("20060102_150405")))
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return "", err
	}

	zipFile, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	s.addSources(zipWriter)
	s.addUserSettings(zipWriter)
	s.addCategories(zipWriter)
	s.addBookshelf(zipWriter)
	s.addBookmarks(zipWriter)
	s.addProgress(zipWriter)
	s.addReplaceRules(zipWriter)

	log.Printf("backup created: %s", backupPath)
	return backupPath, nil
}

func (s *Service) addSources(zipWriter *zip.Writer) {
	var sources []models.BookSource
	if err := s.db.Order("id asc").Find(&sources).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "bookSource.json", data)
}

func (s *Service) addUserSettings(zipWriter *zip.Writer) {
	var settings []models.UserSetting
	if err := s.db.Order("user_id, key").Find(&settings).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "userSettings.json", data)
}

func (s *Service) addCategories(zipWriter *zip.Writer) {
	var categories []models.Category
	if err := s.db.Order("user_id, sort_order, name").Find(&categories).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(categories, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "categories.json", data)
}

func (s *Service) addBookshelf(zipWriter *zip.Writer) {
	var books []models.Book
	if err := s.db.Order("id asc").Find(&books).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(books, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "bookshelf.json", data)
}

func (s *Service) addBookmarks(zipWriter *zip.Writer) {
	type bookmarkExport struct {
		models.Bookmark
		BookTitle string `json:"bookTitle"`
		BookURL   string `json:"bookUrl"`
	}
	var bookmarks []models.Bookmark
	if err := s.db.Order("user_id, book_id, updated_at").Find(&bookmarks).Error; err != nil {
		return
	}
	rows := make([]bookmarkExport, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		row := bookmarkExport{Bookmark: bookmark}
		var book models.Book
		if err := s.db.Select("title", "url").First(&book, bookmark.BookID).Error; err == nil {
			row.BookTitle = book.Title
			row.BookURL = book.URL
		}
		rows = append(rows, row)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "bookmarks.json", data)
}

func (s *Service) addProgress(zipWriter *zip.Writer) {
	var progresses []models.ReadingProgress
	if err := s.db.Order("user_id, book_id").Find(&progresses).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(progresses, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "readingProgress.json", data)
}

func (s *Service) addReplaceRules(zipWriter *zip.Writer) {
	var rules []models.ReplaceRule
	if err := s.db.Order("user_id, updated_at").Find(&rules).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "replaceRules.json", data)
}

func writeZipEntry(zipWriter *zip.Writer, name string, data []byte) {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return
	}
	_, _ = writer.Write(data)
}
