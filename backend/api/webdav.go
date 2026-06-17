package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/models"
	"openreader/backend/services/localbook"
)

// ---------- WebDAV endpoints ----------

func (s *Server) webdavGetOrList(c *gin.Context) {
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	filePath, _, ok := s.webdavPath(c, relPath)
	if !ok {
		return
	}
	if relPath == "" {
		s.webdavList(c, "")
		return
	}
	if info, err := os.Stat(filePath); err == nil && info.IsDir() {
		s.webdavList(c, relPath)
		return
	}
	s.webdavGet(c)
}

func (s *Server) webdavList(c *gin.Context, relPath string) {
	baseDir, cleanRel, ok := s.webdavPath(c, relPath)
	if !ok {
		return
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	type fileEntry struct {
		Name         string `xml:"displayname"`
		IsDir        bool   `xml:"iscollection"`
		Size         int64  `xml:"getcontentlength"`
		LastModified string `xml:"lastmodified"`
	}

	response := struct {
		XMLName  xml.Name    `xml:"multistatus"`
		Response []fileEntry `xml:"response>propstat>prop"`
	}{
		Response: []fileEntry{
			{Name: cleanRel, IsDir: true},
		},
	}

	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		size := info.Size()
		if e.IsDir() {
			size = 0
		}
		response.Response = append(response.Response, fileEntry{
			Name:         e.Name(),
			IsDir:        e.IsDir(),
			Size:         size,
			LastModified: info.ModTime().Format(time.RFC1123),
		})
	}

	c.XML(http.StatusMultiStatus, response)
}

func (s *Server) webdavGet(c *gin.Context) {
	relPath := strings.TrimPrefix(c.Param("path"), "/")
	filePath, _, ok := s.webdavPath(c, relPath)
	if !ok {
		return
	}

	c.File(filePath)
}

func (s *Server) webdavPut(c *gin.Context) {
	filePath, _, ok := s.webdavPath(c, strings.TrimPrefix(c.Param("path"), "/"))
	if !ok {
		return
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusCreated)
}

func (s *Server) webdavMkcol(c *gin.Context) {
	filePath, relPath, ok := s.webdavPath(c, strings.TrimPrefix(c.Param("path"), "/"))
	if !ok {
		return
	}
	if relPath == "" {
		c.Status(http.StatusForbidden)
		return
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if err := os.Mkdir(filePath, 0o755); err != nil {
		c.Status(http.StatusConflict)
		return
	}
	c.Status(http.StatusCreated)
}

func (s *Server) webdavMove(c *gin.Context) {
	sourcePath, sourceRelPath, ok := s.webdavPath(c, strings.TrimPrefix(c.Param("path"), "/"))
	if !ok {
		return
	}
	if sourceRelPath == "" {
		c.Status(http.StatusForbidden)
		return
	}

	destinationRelPath, ok := webdavDestinationPath(c.GetHeader("Destination"))
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	destinationPath, _, ok := s.webdavPath(c, destinationRelPath)
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if err := os.Rename(sourcePath, destinationPath); err != nil {
		c.Status(http.StatusConflict)
		return
	}
	c.Status(http.StatusCreated)
}

func (s *Server) webdavDelete(c *gin.Context) {
	filePath, relPath, ok := s.webdavPath(c, strings.TrimPrefix(c.Param("path"), "/"))
	if !ok {
		return
	}
	if relPath == "" {
		c.Status(http.StatusForbidden)
		return
	}
	if err := os.RemoveAll(filePath); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

func webdavDestinationPath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Path != "" {
		value = parsed.Path
	}
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, "webdav/")
	return cleanRelativePath(value), true
}

// ---------- Reading app backup restoration ----------

func (s *Server) importLegadoBackup(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file is required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open backup"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read backup"})
		return
	}

	result, err := s.restoreLegadoBackupData(data, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

type restoreWebDAVBackupRequest struct {
	Path string `json:"path" binding:"required"`
}

func (s *Server) restoreWebDAVBackup(c *gin.Context) {
	userID, _ := middleware.UserID(c)
	var req restoreWebDAVBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	filePath, _, ok := s.webdavPath(c, req.Path)
	if !ok {
		return
	}
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file not found"})
		return
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read backup file"})
		return
	}
	result, err := s.restoreLegadoBackupData(data, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) importFromWebDAV(c *gin.Context) {
	userID, _ := middleware.UserID(c)

	var req struct {
		Paths      []string `json:"paths" binding:"required"`
		CategoryID *uint    `json:"categoryId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	if !s.validateCategory(c, userID, req.CategoryID) {
		return
	}

	userName, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}

	importer := localbook.NewImporter(s.cfg, s.db)
	imported := make([]gin.H, 0)
	importedBooks := make([]bookListItem, 0)
	seen := make(map[string]bool)

	for _, rawPath := range req.Paths {
		files, ok := s.webDAVImportFiles(c, rawPath)
		if !ok {
			continue
		}
		for _, file := range files {
			if seen[file.relativePath] {
				continue
			}
			seen[file.relativePath] = true

			data, err := os.ReadFile(file.filePath)
			if err != nil {
				imported = append(imported, gin.H{"path": file.relativePath, "error": err.Error()})
				continue
			}

			book, err := importer.Import(localbook.ImportRequest{
				UserID:     userID,
				UserName:   userName,
				FileName:   filepath.Base(file.filePath),
				Extension:  file.extension,
				Data:       data,
				CategoryID: req.CategoryID,
			})
			if err != nil {
				imported = append(imported, gin.H{"path": file.relativePath, "error": err.Error()})
				continue
			}
			item := s.bookShelfListItem(userID, book)
			imported = append(imported, gin.H{"path": file.relativePath, "book": item})
			importedBooks = append(importedBooks, item)
		}
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": importedBooks})
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (s *Server) webDAVImportFiles(c *gin.Context, rawPath string) ([]localStoreImportFile, bool) {
	filePath, relativePath, ok := s.webdavPath(c, rawPath)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, true
	}
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(filePath))
		if !isImportableExtension(ext) {
			return []localStoreImportFile{{filePath: filePath, relativePath: relativePath, extension: ext}}, true
		}
		return []localStoreImportFile{{filePath: filePath, relativePath: relativePath, extension: ext}}, true
	}

	files := make([]localStoreImportFile, 0)
	_ = filepath.WalkDir(filePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !isImportableExtension(ext) {
			return nil
		}
		rel, err := filepath.Rel(filePath, path)
		if err != nil {
			return nil
		}
		files = append(files, localStoreImportFile{
			filePath:     path,
			relativePath: cleanRelativePath(filepath.Join(relativePath, rel)),
			extension:    ext,
		})
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i].relativePath) < strings.ToLower(files[j].relativePath)
	})
	return files, true
}

func (s *Server) restoreLegadoBackupData(data []byte, userID uint) (gin.H, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("invalid backup zip")
	}

	var sourcesCount, rssSourcesCount, booksCount, progressCount, settingsCount, categoriesCount, bookmarksCount, replaceRulesCount int

	for _, zipFile := range zipReader.File {
		switch {
		case strings.HasSuffix(zipFile.Name, "bookSource.json"):
			n, _ := s.restoreSourcesFromZip(zipFile)
			sourcesCount += n
		case strings.HasSuffix(zipFile.Name, "rssSources.json"):
			n, _ := s.restoreRSSSourcesFromZip(zipFile, userID)
			rssSourcesCount += n
		case strings.HasSuffix(zipFile.Name, "userSettings.json"):
			n, _ := s.restoreUserSettingsFromZip(zipFile, userID)
			settingsCount += n
		case strings.HasSuffix(zipFile.Name, "categories.json"):
			n, _ := s.restoreCategoriesFromZip(zipFile, userID)
			categoriesCount += n
		}
	}

	for _, zipFile := range zipReader.File {
		switch {
		case strings.HasSuffix(zipFile.Name, "myBookShelf.json"),
			strings.HasSuffix(zipFile.Name, "bookshelf.json"):
			restoredBooks, restoredProgress, _ := s.restoreBookshelfFromZip(zipFile, userID)
			booksCount += restoredBooks
			progressCount += restoredProgress
		}
	}

	for _, zipFile := range zipReader.File {
		switch {
		case strings.HasSuffix(zipFile.Name, "bookmarks.json"):
			n, _ := s.restoreBookmarksFromZip(zipFile, userID)
			bookmarksCount += n
		case strings.HasSuffix(zipFile.Name, "replaceRules.json"):
			n, _ := s.restoreReplaceRulesFromZip(zipFile, userID)
			replaceRulesCount += n
		case strings.HasSuffix(zipFile.Name, "readingProgress.json"):
			n, _ := s.restoreProgressFromZip(zipFile, userID)
			progressCount += n
		case strings.Contains(zipFile.Name, "bookProgress/"):
			n, _ := s.restoreProgressFromZip(zipFile, userID)
			progressCount += n
		}
	}

	s.broadcastRestoreUpdates(userID, gin.H{
		"sources":      sourcesCount,
		"rssSources":   rssSourcesCount,
		"books":        booksCount,
		"progress":     progressCount,
		"settings":     settingsCount,
		"categories":   categoriesCount,
		"bookmarks":    bookmarksCount,
		"replaceRules": replaceRulesCount,
	})

	return gin.H{
		"sources":      sourcesCount,
		"rssSources":   rssSourcesCount,
		"books":        booksCount,
		"progress":     progressCount,
		"settings":     settingsCount,
		"categories":   categoriesCount,
		"bookmarks":    bookmarksCount,
		"replaceRules": replaceRulesCount,
	}, nil
}

func (s *Server) restoreSourcesFromZip(file *zip.File) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	sources, err := decodeBookSources(data)
	if err != nil {
		return 0, err
	}

	result := s.importBookSources(sources)
	return (result["imported"].(int) + result["updated"].(int)), nil
}

func (s *Server) restoreRSSSourcesFromZip(file *zip.File, userID uint) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	var sources []rssSourceRequest
	if err := json.Unmarshal(data, &sources); err != nil {
		var source rssSourceRequest
		if err := json.Unmarshal(data, &source); err != nil {
			return 0, err
		}
		sources = []rssSourceRequest{source}
	}

	count := 0
	for _, sourceReq := range sources {
		sourceReq.normalize()
		url := strings.TrimSpace(sourceReq.URL)
		if url == "" {
			continue
		}
		title := strings.TrimSpace(sourceReq.Title)
		if title == "" {
			title = url
		}
		enabled := true
		if sourceReq.Enabled != nil {
			enabled = *sourceReq.Enabled
		}
		source := models.RSSSource{
			UserID:       userID,
			Title:        title,
			URL:          url,
			Icon:         strings.TrimSpace(sourceReq.Icon),
			Group:        strings.TrimSpace(sourceReq.Group),
			CustomOrder:  sourceReq.orderOrDefault(s, userID),
			SingleURL:    sourceReq.singleURLOrDefault(),
			ArticleStyle: sourceReq.articleStyleOrDefault(),
			SortURL:      strings.TrimSpace(sourceReq.SortURL),
			RuleArticles: strings.TrimSpace(sourceReq.RuleArticles),
			RuleTitle:    strings.TrimSpace(sourceReq.RuleTitle),
			RulePubDate:  strings.TrimSpace(sourceReq.RulePubDate),
			RuleImage:    strings.TrimSpace(sourceReq.RuleImage),
			RuleLink:     strings.TrimSpace(sourceReq.RuleLink),
			RuleContent:  strings.TrimSpace(sourceReq.RuleContent),
			EnableJS:     sourceReq.enableJSOrDefault(),
			Enabled:      enabled,
			UpdatedAt:    time.Now(),
		}
		query := s.db.Where("user_id = ? AND url = ?", userID, url)
		if err := query.Assign(source).FirstOrCreate(&source).Error; err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Server) broadcastRestoreUpdates(userID uint, result gin.H) {
	if s.hub == nil {
		return
	}
	if restoreResultCount(result, "sources") > 0 {
		s.broadcastSourcesUpdate("restore-backup")
	}
	if restoreResultCount(result, "settings") > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "settings_update", "payload": gin.H{"key": "all"}})
	}
	if restoreResultCount(result, "categories") > 0 {
		var categories []models.Category
		if err := s.db.Where("user_id = ?", userID).Order("sort_order asc, name asc").Find(&categories).Error; err == nil {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "categories_update", "payload": categories})
		} else {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "categories_update"})
		}
	}
	if restoreResultCount(result, "books")+restoreResultCount(result, "progress") > 0 {
		if items, err := s.listAllBookShelfItems(userID); err == nil {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": items})
		} else {
			_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
		}
	}
	if restoreResultCount(result, "bookmarks") > 0 {
		s.broadcastBookmarksUpdate(userID, "restore-backup", 0, nil)
	}
	if restoreResultCount(result, "replaceRules") > 0 {
		s.broadcastReplaceRulesUpdate(userID, "restore-backup")
	}
	if restoreResultCount(result, "rssSources") > 0 {
		s.broadcastRSSUpdate(userID, "restore-backup", gin.H{"sources": true})
	}
}

func restoreResultCount(result gin.H, key string) int {
	switch value := result[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func (s *Server) restoreUserSettingsFromZip(file *zip.File, userID uint) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	var settings []models.UserSetting
	if err := json.Unmarshal(data, &settings); err != nil {
		return 0, err
	}

	count := 0
	for _, setting := range settings {
		key := normalizeUserSettingKey(setting.Key)
		if key == "" || !json.Valid([]byte(setting.Value)) {
			continue
		}
		next := models.UserSetting{
			UserID:    userID,
			Key:       key,
			Value:     string(sanitizeUserSettingValue(key, json.RawMessage(setting.Value))),
			UpdatedAt: time.Now(),
		}
		if err := s.db.Where("user_id = ? AND key = ?", userID, key).Assign(next).FirstOrCreate(&next).Error; err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Server) restoreCategoriesFromZip(file *zip.File, userID uint) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	var categories []models.Category
	if err := json.Unmarshal(data, &categories); err != nil {
		return 0, err
	}

	count := 0
	for _, category := range categories {
		name := strings.TrimSpace(category.Name)
		if name == "" {
			continue
		}
		next := models.Category{
			UserID:    userID,
			Name:      name,
			Color:     category.Color,
			SortOrder: category.SortOrder,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := s.db.Where("user_id = ? AND name = ?", userID, name).Assign(next).FirstOrCreate(&next).Error; err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Server) restoreBookshelfFromZip(file *zip.File, userID uint) (int, int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, 0, err
	}

	var books []struct {
		Title           string `json:"title"`
		Name            string `json:"name"`
		Author          string `json:"author"`
		URL             string `json:"url"`
		BookURL         string `json:"bookUrl"`
		CoverURL        string `json:"coverUrl"`
		CustomCoverURL  string `json:"customCoverUrl"`
		Intro           string `json:"intro"`
		LastChapter     string `json:"lastChapter"`
		ChapterCount    int    `json:"chapterCount"`
		CanUpdate       *bool  `json:"canUpdate"`
		CategoryName    string `json:"categoryName"`
		OriginName      string `json:"originName"`
		DurChapter      int    `json:"durChapter"`
		DurChapterPos   int    `json:"durChapterPos"`
		DurChapterTitle string `json:"durChapterTitle"`
	}
	if err := json.Unmarshal(data, &books); err != nil {
		return 0, 0, err
	}

	count := 0
	progressCount := 0
	for _, b := range books {
		title := strings.TrimSpace(b.Title)
		if title == "" {
			title = strings.TrimSpace(b.Name)
		}
		if title == "" {
			continue
		}
		bookURL := strings.TrimSpace(b.URL)
		if bookURL == "" {
			bookURL = strings.TrimSpace(b.BookURL)
		}
		canUpdate := true
		if b.CanUpdate != nil {
			canUpdate = *b.CanUpdate
		}
		book := models.Book{
			UserID:         userID,
			Title:          title,
			Author:         strings.TrimSpace(b.Author),
			URL:            bookURL,
			CoverURL:       strings.TrimSpace(b.CoverURL),
			CustomCoverURL: strings.TrimSpace(b.CustomCoverURL),
			Intro:          strings.TrimSpace(b.Intro),
			LastChapter:    strings.TrimSpace(b.LastChapter),
			ChapterCount:   b.ChapterCount,
			CanUpdate:      canUpdate,
		}
		if categoryID := s.findRestoredCategoryID(userID, b.CategoryName); categoryID != nil {
			book.CategoryID = categoryID
		}
		query := s.db.Where("user_id = ? AND title = ?", userID, book.Title)
		if book.URL != "" {
			query = s.db.Where("user_id = ? AND url = ?", userID, book.URL)
		}
		var existing models.Book
		if query.First(&existing).Error == nil {
			existing.Author = book.Author
			existing.CoverURL = book.CoverURL
			existing.CustomCoverURL = book.CustomCoverURL
			existing.Intro = book.Intro
			existing.LastChapter = book.LastChapter
			existing.ChapterCount = book.ChapterCount
			existing.CanUpdate = book.CanUpdate
			existing.CategoryID = book.CategoryID
			if book.URL != "" {
				existing.URL = book.URL
			}
			if err := s.db.Save(&existing).Error; err != nil {
				continue
			}
			count++
			if s.restoreBookshelfProgress(userID, existing.ID, b.DurChapter, b.DurChapterPos, b.DurChapterTitle) {
				progressCount++
			}
			continue
		}
		if err := s.db.Create(&book).Error; err != nil {
			continue
		}
		if s.restoreBookshelfProgress(userID, book.ID, b.DurChapter, b.DurChapterPos, b.DurChapterTitle) {
			progressCount++
		}
		count++
	}
	return count, progressCount, nil
}

func (s *Server) restoreBookmarksFromZip(file *zip.File, userID uint) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	var rows []struct {
		models.Bookmark
		BookTitle string `json:"bookTitle"`
		BookURL   string `json:"bookUrl"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, err
	}

	count := 0
	for _, row := range rows {
		book, ok := s.findRestoredBook(userID, row.BookURL, row.BookTitle)
		if !ok {
			continue
		}
		bookmark := models.Bookmark{
			UserID:       userID,
			BookID:       book.ID,
			ChapterID:    0,
			ChapterIndex: row.ChapterIndex,
			Offset:       row.Offset,
			Percent:      clampProgressPercent(row.Percent),
			Title:        row.Title,
			Excerpt:      row.Excerpt,
			Note:         row.Note,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		query := s.db.Where("user_id = ? AND book_id = ? AND chapter_index = ? AND offset = ? AND title = ?",
			userID, book.ID, bookmark.ChapterIndex, bookmark.Offset, bookmark.Title)
		if err := query.Assign(bookmark).FirstOrCreate(&bookmark).Error; err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Server) restoreReplaceRulesFromZip(file *zip.File, userID uint) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	var rules []struct {
		Name        string `json:"name"`
		Pattern     string `json:"pattern"`
		Replacement string `json:"replacement"`
		Scope       string `json:"scope"`
		IsRegex     *bool  `json:"isRegex"`
		Enabled     *bool  `json:"enabled"`
		IsEnabled   *bool  `json:"isEnabled"`
	}
	if err := json.Unmarshal(data, &rules); err != nil {
		return 0, err
	}

	count := 0
	for _, rule := range rules {
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			continue
		}
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			name = pattern
		}
		enabled := true
		if rule.Enabled != nil {
			enabled = *rule.Enabled
		}
		if rule.IsEnabled != nil {
			enabled = *rule.IsEnabled
		}
		isRegex := true
		if rule.IsRegex != nil {
			isRegex = *rule.IsRegex
		}
		next := models.ReplaceRule{
			UserID:      userID,
			Name:        name,
			Pattern:     pattern,
			Replacement: rule.Replacement,
			Scope:       normalizeReplaceRuleScope(rule.Scope),
			IsRegex:     &isRegex,
			Enabled:     enabled,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := s.db.Where("user_id = ? AND pattern = ?", userID, pattern).Assign(next).FirstOrCreate(&next).Error; err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Server) restoreBookshelfProgress(userID uint, bookID uint, chapterIndex int, offset int, chapterTitle string) bool {
	if chapterIndex <= 0 && offset <= 0 && strings.TrimSpace(chapterTitle) == "" {
		return false
	}
	if chapterIndex < 0 {
		chapterIndex = 0
	}
	if offset < 0 {
		offset = 0
	}
	progress := models.ReadingProgress{
		UserID:       userID,
		BookID:       bookID,
		ChapterIndex: chapterIndex,
		Offset:       offset,
		ChapterTitle: chapterTitle,
		Mode:         "scroll",
		UpdatedAt:    time.Now(),
	}
	return s.db.Where("user_id = ? AND book_id = ?", userID, bookID).Assign(progress).FirstOrCreate(&progress).Error == nil
}

func (s *Server) restoreProgressFromZip(file *zip.File, userID uint) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}

	type progressPayload struct {
		Name            string `json:"name"`
		BookName        string `json:"bookName"`
		BookTitle       string `json:"bookTitle"`
		Title           string `json:"title"`
		BookURL         string `json:"bookUrl"`
		URL             string `json:"url"`
		DurChapter      int    `json:"durChapter"`
		DurChapterIndex int    `json:"durChapterIndex"`
		ChapterIndex    int    `json:"chapterIndex"`
		DurChapterPos   int    `json:"durChapterPos"`
		Offset          int    `json:"offset"`
		DurChapterTitle string `json:"durChapterTitle"`
		ChapterTitle    string `json:"chapterTitle"`
	}

	var payloads []progressPayload
	if err := json.Unmarshal(data, &payloads); err != nil {
		var payload progressPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return 0, err
		}
		payloads = []progressPayload{payload}
	}

	count := 0
	for _, payload := range payloads {
		bookURL := strings.TrimSpace(payload.BookURL)
		if bookURL == "" {
			bookURL = strings.TrimSpace(payload.URL)
		}
		title := strings.TrimSpace(payload.BookName)
		if title == "" {
			title = strings.TrimSpace(payload.BookTitle)
		}
		if title == "" {
			title = strings.TrimSpace(payload.Name)
		}
		if title == "" {
			title = strings.TrimSpace(payload.Title)
		}

		book, ok := s.findRestoredBook(userID, bookURL, title)
		if !ok {
			continue
		}

		chapterIndex := payload.ChapterIndex
		if chapterIndex == 0 && payload.DurChapterIndex > 0 {
			chapterIndex = payload.DurChapterIndex
		}
		if chapterIndex == 0 && payload.DurChapter > 0 {
			chapterIndex = payload.DurChapter
		}
		offset := payload.Offset
		if offset == 0 && payload.DurChapterPos > 0 {
			offset = payload.DurChapterPos
		}
		chapterTitle := strings.TrimSpace(payload.ChapterTitle)
		if chapterTitle == "" {
			chapterTitle = strings.TrimSpace(payload.DurChapterTitle)
		}
		if s.restoreBookshelfProgress(userID, book.ID, chapterIndex, offset, chapterTitle) {
			count++
		}
	}
	return count, nil
}

func (s *Server) findRestoredCategoryID(userID uint, categoryName string) *uint {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return nil
	}
	var category models.Category
	if err := s.db.Where("user_id = ? AND name = ?", userID, categoryName).First(&category).Error; err != nil {
		return nil
	}
	return &category.ID
}

func (s *Server) findRestoredBook(userID uint, bookURL string, title string) (models.Book, bool) {
	bookURL = strings.TrimSpace(bookURL)
	title = strings.TrimSpace(title)
	var book models.Book
	query := s.db.Where("user_id = ?", userID)
	if bookURL != "" {
		query = query.Where("url = ?", bookURL)
	} else if title != "" {
		query = query.Where("title = ?", title)
	} else {
		return book, false
	}
	if err := query.First(&book).Error; err != nil {
		return book, false
	}
	return book, true
}

func (s *Server) webdavDir() string {
	return filepath.Join(s.cfg.DataDir, "webdav")
}

func (s *Server) webdavPath(c *gin.Context, rawPath string) (string, string, bool) {
	baseDir, err := filepath.Abs(s.webdavDir())
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return "", "", false
	}
	relPath := cleanRelativePath(rawPath)
	targetPath := filepath.Join(baseDir, relPath)
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return "", "", false
	}
	if targetAbs != baseDir && !strings.HasPrefix(targetAbs, baseDir+string(os.PathSeparator)) {
		c.Status(http.StatusForbidden)
		return "", "", false
	}
	return targetAbs, relPath, true
}
