package backup

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/gorm"

	"openreader/backend/config"
	"openreader/backend/engine"
	"openreader/backend/models"
	"openreader/backend/services/bookgroups"
	"openreader/backend/services/sourcecompat"
)

// Service handles automated backups.
type Service struct {
	db        *gorm.DB
	webdavDir string
	cfg       config.Config
	stopCh    chan struct{}
	mu        sync.Mutex
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	backupPath, err := nextBackupPath(backupDir, "backup_"+time.Now().Format("20060102_150405"))
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(backupDir, ".backup-*.tmp")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	completed := false
	defer func() {
		_ = temporary.Close()
		if !completed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", err
	}

	zipWriter := zip.NewWriter(temporary)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.writeLogicalEntries(tx, zipWriter, userID); err != nil {
			_ = zipWriter.Close()
			return err
		}
		return zipWriter.Close()
	}); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, backupPath); err != nil {
		return "", err
	}
	completed = true

	log.Printf("backup created: %s", backupPath)
	return backupPath, nil
}

func nextBackupPath(backupDir, stem string) (string, error) {
	for suffix := 0; suffix < 10000; suffix++ {
		name := stem + ".zip"
		if suffix > 0 {
			name = fmt.Sprintf("%s_%02d.zip", stem, suffix)
		}
		path := filepath.Join(backupDir, name)
		_, err := os.Lstat(path)
		switch {
		case os.IsNotExist(err):
			return path, nil
		case err != nil:
			return "", err
		}
	}
	return "", fmt.Errorf("backup filename space exhausted")
}

func (s *Service) writeLogicalEntries(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	steps := []func() error{
		func() error { return s.addSources(db, zipWriter) },
		func() error { return s.addRSSSources(db, zipWriter, userID) },
		func() error { return s.addUserSettings(db, zipWriter, userID) },
		func() error { return s.addCategories(db, zipWriter, userID) },
		func() error { return s.addBookGroups(db, zipWriter, userID) },
		func() error { return s.addBookshelf(db, zipWriter, userID) },
		func() error { return s.addChapterVariables(db, zipWriter, userID) },
		func() error { return s.addBookmarks(db, zipWriter, userID) },
		func() error { return s.addProgress(db, zipWriter, userID) },
		func() error { return s.addReplaceRules(db, zipWriter, userID) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) addSources(db *gorm.DB, zipWriter *zip.Writer) error {
	var sources []models.BookSource
	if err := db.Order("custom_order asc, id asc").Find(&sources).Error; err != nil {
		return err
	}
	data, err := json.MarshalIndent(sourcecompat.Export(sources), "", "  ")
	if err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "bookSource.json", data)
}

func (s *Service) addRSSSources(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	type rssSourceExport struct {
		models.RSSSource
		SourceName    string `json:"sourceName,omitempty"`
		SourceURL     string `json:"sourceUrl,omitempty"`
		SourceIcon    string `json:"sourceIcon,omitempty"`
		SourceGroup   string `json:"sourceGroup,omitempty"`
		SourceComment string `json:"sourceComment,omitempty"`
	}
	var sources []models.RSSSource
	query := db.Order("user_id, custom_order, updated_at")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&sources).Error; err != nil {
		return err
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
		return err
	}
	return writeZipEntry(zipWriter, "rssSources.json", data)
}

func (s *Service) addUserSettings(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	var settings []models.UserSetting
	query := db.Order("user_id, key")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&settings).Error; err != nil {
		return err
	}
	for i := range settings {
		settings[i].Value = sanitizeBackupUserSettingValue(settings[i].Key, settings[i].Value)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "userSettings.json", data)
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

func (s *Service) addCategories(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	var categories []models.Category
	query := db.Order("user_id, sort_order, name")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&categories).Error; err != nil {
		return err
	}
	data, err := json.MarshalIndent(categories, "", "  ")
	if err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "categories.json", data)
}

func (s *Service) addBookGroups(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	if userID == nil {
		return nil
	}
	rows, _, err := bookgroups.New(db).Backup(*userID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "bookGroup.json", data)
}

func (s *Service) addBookshelf(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	type bookExport struct {
		models.Book
		CategoryName       string   `json:"categoryName,omitempty"`
		CategoryNames      []string `json:"categoryNames,omitempty"`
		Group              int      `json:"group,omitempty"`
		SourceName         string   `json:"sourceName,omitempty"`
		Name               string   `json:"name"`
		BookURL            string   `json:"bookUrl"`
		Origin             string   `json:"origin"`
		OriginName         string   `json:"originName"`
		LatestChapterTitle string   `json:"latestChapterTitle,omitempty"`
		TotalChapterNum    int      `json:"totalChapterNum"`
		DurChapterIndex    int      `json:"durChapterIndex"`
		DurChapterPos      int      `json:"durChapterPos"`
		DurChapterTitle    string   `json:"durChapterTitle,omitempty"`
		DurChapterTime     int64    `json:"durChapterTime"`
	}
	maskByCategory := make(map[uint]int)
	if userID != nil {
		_, masks, err := bookgroups.New(db).Backup(*userID)
		if err != nil {
			return err
		}
		maskByCategory = masks
	}
	var books []models.Book
	query := db.Order("id asc")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&books).Error; err != nil {
		return err
	}
	var progresses []models.ReadingProgress
	progressQuery := db.Order("user_id, book_id")
	if userID != nil {
		progressQuery = progressQuery.Where("user_id = ?", *userID)
	}
	if err := progressQuery.Find(&progresses).Error; err != nil {
		return err
	}
	progressByBook := make(map[uint]models.ReadingProgress, len(progresses))
	for _, progress := range progresses {
		progressByBook[progress.BookID] = progress
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
		row := bookExport{
			Book:               book,
			Name:               book.Title,
			BookURL:            book.URL,
			Origin:             "loc_book",
			OriginName:         book.OriginalFile,
			LatestChapterTitle: book.LastChapter,
			TotalChapterNum:    book.ChapterCount,
		}
		if progress, ok := progressByBook[book.ID]; ok {
			row.DurChapterIndex = progress.ChapterIndex
			row.DurChapterPos = progress.Offset
			row.DurChapterTitle = progress.ChapterTitle
			row.DurChapterTime = progress.UpdatedAt.UnixMilli()
		}
		if book.SourceID > 0 {
			var source models.BookSource
			if err := db.Select("name", "base_url").First(&source, book.SourceID).Error; err == nil {
				row.SourceName = source.Name
				row.Origin = source.BaseURL
				row.OriginName = source.Name
			} else {
				return err
			}
		}
		var categoryRows []models.Category
		if err := db.
			Joins("JOIN book_categories ON book_categories.category_id = categories.id").
			Where("book_categories.user_id = ? AND book_categories.book_id = ?", book.UserID, book.ID).
			Order("book_categories.id asc").
			Find(&categoryRows).Error; err != nil {
			return err
		}
		for _, category := range categoryRows {
			row.CategoryNames = append(row.CategoryNames, category.Name)
			row.Group |= maskByCategory[category.ID]
		}
		if len(row.CategoryNames) > 0 {
			row.CategoryName = row.CategoryNames[0]
		} else if book.CategoryID != nil {
			var category models.Category
			if err := db.Select("name").First(&category, *book.CategoryID).Error; err == nil {
				row.CategoryName = category.Name
				row.CategoryNames = []string{category.Name}
				row.Group |= maskByCategory[category.ID]
			} else {
				return err
			}
		}
		rows = append(rows, row)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "bookshelf.json", data)
}

func (s *Service) addChapterVariables(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
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
	query := db.Table("chapters").
		Select("book_sources.name AS source_name, books.url AS book_url, books.title AS book_title, chapters.url AS chapter_url, chapters.title AS chapter_title, chapters.`index` AS chapter_index, chapters.variable").
		Joins("JOIN books ON books.id = chapters.book_id").
		Joins("JOIN book_sources ON book_sources.id = books.source_id").
		Where("books.source_id > 0 AND chapters.variable <> ''").
		Order("books.id ASC, chapters.`index` ASC")
	if userID != nil {
		query = query.Where("books.user_id = ?", *userID)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return err
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
		return nil
	}
	data, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "chapterVariables.json", data)
}

func (s *Service) addBookmarks(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	type bookmarkExport struct {
		models.Bookmark
		BookTitle string `json:"bookTitle"`
		BookURL   string `json:"bookUrl"`
	}
	var bookmarks []models.Bookmark
	// Reader-dev exposes its persisted bookmark array in insertion order.  Keep
	// that ordering in exports too: an edit changes updated_at but must not move
	// a bookmark in the manager or in the next restored backup.
	query := db.Order("user_id, id")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&bookmarks).Error; err != nil {
		return err
	}
	rows := make([]bookmarkExport, 0, len(bookmarks))
	upstreamRows := make([]upstreamBookmarkExport, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		row := bookmarkExport{Bookmark: bookmark}
		var book models.Book
		if err := db.Select("title", "author", "url").First(&book, bookmark.BookID).Error; err != nil {
			return err
		}
		row.BookTitle = book.Title
		row.BookURL = book.URL
		rows = append(rows, row)
		upstreamRows = append(upstreamRows, upstreamBookmarkExport{
			Time:         bookmark.CreatedAt.UnixMilli(),
			BookName:     book.Title,
			BookAuthor:   book.Author,
			ChapterIndex: bookmark.ChapterIndex,
			ChapterPos:   bookmark.Offset,
			ChapterName:  bookmark.Title,
			BookText:     bookmark.Excerpt,
			Content:      bookmark.Note,
		})
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	upstreamData, err := json.MarshalIndent(upstreamRows, "", "  ")
	if err != nil {
		return err
	}
	if err := writeZipEntry(zipWriter, "bookmark.json", upstreamData); err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "bookmarks.json", data)
}

type upstreamBookmarkExport struct {
	Time         int64  `json:"time"`
	BookName     string `json:"bookName"`
	BookAuthor   string `json:"bookAuthor"`
	ChapterIndex int    `json:"chapterIndex"`
	ChapterPos   int    `json:"chapterPos"`
	ChapterName  string `json:"chapterName"`
	BookText     string `json:"bookText"`
	Content      string `json:"content"`
}

func (s *Service) addProgress(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	type progressExport struct {
		models.ReadingProgress
		BookTitle string `json:"bookTitle"`
		BookURL   string `json:"bookUrl"`
	}
	var progresses []models.ReadingProgress
	query := db.Order("user_id, book_id")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&progresses).Error; err != nil {
		return err
	}
	rows := make([]progressExport, 0, len(progresses))
	for _, progress := range progresses {
		row := progressExport{ReadingProgress: progress}
		var book models.Book
		if err := db.Select("title", "url").First(&book, progress.BookID).Error; err != nil {
			return err
		}
		row.BookTitle = book.Title
		row.BookURL = book.URL
		rows = append(rows, row)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "readingProgress.json", data)
}

func (s *Service) addReplaceRules(db *gorm.DB, zipWriter *zip.Writer, userID *uint) error {
	var rules []models.ReplaceRule
	// Replacement order is user-visible: a backup must preserve the same
	// insertion pipeline that the reader applies, not reorder rows by a recent
	// edit timestamp.
	query := db.Order("user_id, sort_order, id")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if err := query.Find(&rules).Error; err != nil {
		return err
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	upstreamRows := make([]upstreamReplaceRuleExport, 0, len(rules))
	for _, rule := range rules {
		upstreamRows = append(upstreamRows, upstreamReplaceRuleExport{
			ID:          int64(rule.ID),
			Name:        rule.Name,
			Group:       rule.Group,
			Pattern:     rule.Pattern,
			Replacement: rule.Replacement,
			Scope:       rule.Scope,
			IsEnabled:   rule.Enabled,
			IsRegex:     rule.IsRegex != nil && *rule.IsRegex,
			Order:       rule.Order,
		})
	}
	upstreamData, err := json.MarshalIndent(upstreamRows, "", "  ")
	if err != nil {
		return err
	}
	if err := writeZipEntry(zipWriter, "replaceRule.json", upstreamData); err != nil {
		return err
	}
	return writeZipEntry(zipWriter, "replaceRules.json", data)
}

type upstreamReplaceRuleExport struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group,omitempty"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Scope       string `json:"scope,omitempty"`
	IsEnabled   bool   `json:"isEnabled"`
	IsRegex     bool   `json:"isRegex"`
	Order       int    `json:"order"`
}

func writeZipEntry(zipWriter *zip.Writer, name string, data []byte) error {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}
