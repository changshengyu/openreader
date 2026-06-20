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
	s.addRSSSources(zipWriter)
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

func (s *Service) addRSSSources(zipWriter *zip.Writer) {
	type rssSourceExport struct {
		models.RSSSource
		SourceName    string `json:"sourceName,omitempty"`
		SourceURL     string `json:"sourceUrl,omitempty"`
		SourceIcon    string `json:"sourceIcon,omitempty"`
		SourceGroup   string `json:"sourceGroup,omitempty"`
		SourceComment string `json:"sourceComment,omitempty"`
	}
	var sources []models.RSSSource
	if err := s.db.Order("user_id, custom_order, updated_at").Find(&sources).Error; err != nil {
		return
	}
	rows := make([]rssSourceExport, 0, len(sources))
	for _, source := range sources {
		rows = append(rows, rssSourceExport{
			RSSSource:     source,
			SourceName:    source.Title,
			SourceURL:     source.URL,
			SourceIcon:    source.Icon,
			SourceGroup:   source.Group,
			SourceComment: source.Comment,
		})
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "rssSources.json", data)
}

func (s *Service) addUserSettings(zipWriter *zip.Writer) {
	var settings []models.UserSetting
	if err := s.db.Order("user_id, key").Find(&settings).Error; err != nil {
		return
	}
	for i := range settings {
		settings[i].Value = sanitizeBackupUserSettingValue(settings[i].Key, settings[i].Value)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "userSettings.json", data)
}

func sanitizeBackupUserSettingValue(key string, value string) string {
	if key != "reader" || !json.Valid([]byte(value)) {
		return value
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		return value
	}
	delete(data, "pageMode")
	delete(data, "miniInterface")
	encoded, err := json.Marshal(data)
	if err != nil {
		return value
	}
	return string(encoded)
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
	type bookExport struct {
		models.Book
		CategoryName  string   `json:"categoryName,omitempty"`
		CategoryNames []string `json:"categoryNames,omitempty"`
	}
	var books []models.Book
	if err := s.db.Order("id asc").Find(&books).Error; err != nil {
		return
	}
	rows := make([]bookExport, 0, len(books))
	for _, book := range books {
		row := bookExport{Book: book}
		var categoryRows []models.Category
		_ = s.db.
			Joins("JOIN book_categories ON book_categories.category_id = categories.id").
			Where("book_categories.user_id = ? AND book_categories.book_id = ?", book.UserID, book.ID).
			Order("book_categories.id asc").
			Find(&categoryRows).Error
		for _, category := range categoryRows {
			row.CategoryNames = append(row.CategoryNames, category.Name)
		}
		if len(row.CategoryNames) > 0 {
			row.CategoryName = row.CategoryNames[0]
		} else if book.CategoryID != nil {
			var category models.Category
			if err := s.db.Select("name").First(&category, *book.CategoryID).Error; err == nil {
				row.CategoryName = category.Name
				row.CategoryNames = []string{category.Name}
			}
		}
		rows = append(rows, row)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
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
	type progressExport struct {
		models.ReadingProgress
		BookTitle string `json:"bookTitle"`
		BookURL   string `json:"bookUrl"`
	}
	var progresses []models.ReadingProgress
	if err := s.db.Order("user_id, book_id").Find(&progresses).Error; err != nil {
		return
	}
	rows := make([]progressExport, 0, len(progresses))
	for _, progress := range progresses {
		row := progressExport{ReadingProgress: progress}
		var book models.Book
		if err := s.db.Select("title", "url").First(&book, progress.BookID).Error; err == nil {
			row.BookTitle = book.Title
			row.BookURL = book.URL
		}
		rows = append(rows, row)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
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
