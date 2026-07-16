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

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
)

// Service handles automated backups.
type Service struct {
	db        *gorm.DB
	webdavDir string
	cfg       config.Config
	stopCh    chan struct{}
}

// New creates a backup service.
//
// config is optional only to keep older in-process callers source compatible. Portable local
// archive backup requires cfg.LibraryDir and is unavailable until the production caller passes it.
func New(db *gorm.DB, webdavDir string, configs ...config.Config) *Service {
	cfg := config.Config{}
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return &Service{
		db:        db,
		webdavDir: webdavDir,
		cfg:       cfg,
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
			s.runScheduled()
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
	return s.run(nil, s.webdavDir)
}

// RunNowForUser creates a user-scoped backup below the existing WebDAV mount.
// The caller supplies the persisted username only to derive a safe directory;
// all exported personal rows are filtered by the authenticated user id.
func (s *Service) RunNowForUser(userID uint, username string) (string, error) {
	return s.run(&userID, filepath.Join(s.webdavDir, "users", engine.SafeFilename(username)))
}

func (s *Service) runScheduled() {
	var users []models.User
	if err := s.db.Select("id", "username").Find(&users).Error; err != nil {
		log.Printf("scheduled backup: load users: %v", err)
		return
	}
	for _, user := range users {
		if _, err := s.RunNowForUser(user.ID, user.Username); err != nil {
			log.Printf("scheduled backup for user %d failed: %v", user.ID, err)
		}
	}
}

func (s *Service) run(userID *uint, backupDir string) (string, error) {
	backupPath := filepath.Join(backupDir, fmt.Sprintf("backup_%s.zip", time.Now().Format("20060102_150405")))
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
	s.addRSSSources(zipWriter, userID)
	s.addUserSettings(zipWriter, userID)
	s.addCategories(zipWriter, userID)
	s.addBookshelf(zipWriter, userID)
	s.addChapterVariables(zipWriter, userID)
	s.addBookmarks(zipWriter, userID)
	s.addProgress(zipWriter, userID)
	s.addReplaceRules(zipWriter, userID)

	log.Printf("backup created: %s", backupPath)
	return backupPath, nil
}

func (s *Service) addSources(zipWriter *zip.Writer) {
	var sources []models.BookSource
	if err := s.db.Order("custom_order asc, id asc").Find(&sources).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "bookSource.json", data)
}

func (s *Service) addRSSSources(zipWriter *zip.Writer, userID *uint) {
	type rssSourceExport struct {
		models.RSSSource
		SourceName    string `json:"sourceName,omitempty"`
		SourceURL     string `json:"sourceUrl,omitempty"`
		SourceIcon    string `json:"sourceIcon,omitempty"`
		SourceGroup   string `json:"sourceGroup,omitempty"`
		SourceComment string `json:"sourceComment,omitempty"`
	}
	var sources []models.RSSSource
	query := s.db.Order("user_id, custom_order, updated_at")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&sources).Error; err != nil {
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

func (s *Service) addUserSettings(zipWriter *zip.Writer, userID *uint) {
	var settings []models.UserSetting
	query := s.db.Order("user_id, key")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&settings).Error; err != nil {
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

func (s *Service) addCategories(zipWriter *zip.Writer, userID *uint) {
	var categories []models.Category
	query := s.db.Order("user_id, sort_order, name")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&categories).Error; err != nil {
		return
	}
	data, err := json.MarshalIndent(categories, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "categories.json", data)
}

func (s *Service) addBookshelf(zipWriter *zip.Writer, userID *uint) {
	type bookExport struct {
		models.Book
		CategoryName  string   `json:"categoryName,omitempty"`
		CategoryNames []string `json:"categoryNames,omitempty"`
		SourceName    string   `json:"sourceName,omitempty"`
	}
	var books []models.Book
	query := s.db.Order("id asc")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&books).Error; err != nil {
		return
	}
	rows := make([]bookExport, 0, len(books))
	for _, book := range books {
		if book.SourceID == 0 {
			// Local/imported books have no reader-dev source-rule context. Do not
			// turn a stale legacy database value into a portable remote token.
			book.Variable = ""
		} else if variable, err := models.NormalizeSourceRuleVariables(book.Variable); err == nil {
			book.Variable = variable
		} else {
			book.Variable = ""
		}
		row := bookExport{Book: book}
		if book.SourceID > 0 {
			var source models.BookSource
			if err := s.db.Select("name").First(&source, book.SourceID).Error; err == nil {
				row.SourceName = source.Name
			}
		}
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

func (s *Service) addChapterVariables(zipWriter *zip.Writer, userID *uint) {
	type chapterVariableExport struct {
		SourceName   string `json:"sourceName"`
		BookURL      string `json:"bookUrl"`
		BookTitle    string `json:"bookTitle"`
		ChapterURL   string `json:"chapterUrl"`
		ChapterTitle string `json:"chapterTitle"`
		ChapterIndex int    `json:"chapterIndex"`
		Variable     string `json:"variable"`
	}
	type chapterVariableRow struct {
		SourceName   string
		BookURL      string
		BookTitle    string
		ChapterURL   string
		ChapterTitle string
		ChapterIndex int
		Variable     string
	}

	var rows []chapterVariableRow
	query := s.db.Table("chapters").
		Select("book_sources.name AS source_name, books.url AS book_url, books.title AS book_title, chapters.url AS chapter_url, chapters.title AS chapter_title, chapters.`index` AS chapter_index, chapters.variable").
		Joins("JOIN books ON books.id = chapters.book_id").
		Joins("JOIN book_sources ON book_sources.id = books.source_id").
		Where("books.source_id > 0 AND chapters.variable <> ''").
		Order("books.id ASC, chapters.`index` ASC")
	if userID != nil {
		query = query.Where("books.user_id = ?", *userID)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return
	}

	exported := make([]chapterVariableExport, 0, len(rows))
	for _, row := range rows {
		variable, err := models.NormalizeSourceRuleVariables(row.Variable)
		if err != nil {
			continue
		}
		exported = append(exported, chapterVariableExport{
			SourceName:   row.SourceName,
			BookURL:      row.BookURL,
			BookTitle:    row.BookTitle,
			ChapterURL:   row.ChapterURL,
			ChapterTitle: row.ChapterTitle,
			ChapterIndex: row.ChapterIndex,
			Variable:     variable,
		})
	}
	if len(exported) == 0 {
		return
	}
	data, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return
	}
	writeZipEntry(zipWriter, "chapterVariables.json", data)
}

func (s *Service) addBookmarks(zipWriter *zip.Writer, userID *uint) {
	type bookmarkExport struct {
		models.Bookmark
		BookTitle string `json:"bookTitle"`
		BookURL   string `json:"bookUrl"`
	}
	var bookmarks []models.Bookmark
	// Reader-dev exposes its persisted bookmark array in insertion order.  Keep
	// that ordering in exports too: an edit changes updated_at but must not move
	// a bookmark in the manager or in the next restored backup.
	query := s.db.Order("user_id, id")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&bookmarks).Error; err != nil {
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

func (s *Service) addProgress(zipWriter *zip.Writer, userID *uint) {
	type progressExport struct {
		models.ReadingProgress
		BookTitle string `json:"bookTitle"`
		BookURL   string `json:"bookUrl"`
	}
	var progresses []models.ReadingProgress
	query := s.db.Order("user_id, book_id")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&progresses).Error; err != nil {
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

func (s *Service) addReplaceRules(zipWriter *zip.Writer, userID *uint) {
	var rules []models.ReplaceRule
	// Replacement order is user-visible: a backup must preserve the same
	// insertion pipeline that the reader applies, not reorder rows by a recent
	// edit timestamp.
	query := s.db.Order("user_id, id")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&rules).Error; err != nil {
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
