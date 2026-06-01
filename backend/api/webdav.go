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
		Name  string `xml:"displayname"`
		IsDir bool   `xml:"iscollection"`
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
		response.Response = append(response.Response, fileEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
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
			imported = append(imported, gin.H{"path": file.relativePath, "book": book})
		}
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update"})
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
	return files, true
}

func (s *Server) restoreLegadoBackupData(data []byte, userID uint) (gin.H, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("invalid backup zip")
	}

	var sourcesCount, booksCount, progressCount, settingsCount int

	for _, zipFile := range zipReader.File {
		switch {
		case strings.HasSuffix(zipFile.Name, "bookSource.json"):
			sourcesCount, _ = s.restoreSourcesFromZip(zipFile)
		case strings.HasSuffix(zipFile.Name, "userSettings.json"):
			settingsCount, _ = s.restoreUserSettingsFromZip(zipFile, userID)
		case strings.HasSuffix(zipFile.Name, "myBookShelf.json"),
			strings.HasSuffix(zipFile.Name, "bookshelf.json"):
			restoredBooks, restoredProgress, _ := s.restoreBookshelfFromZip(zipFile, userID)
			booksCount += restoredBooks
			progressCount += restoredProgress
		case strings.HasPrefix(zipFile.Name, "bookProgress/"):
			n, _ := s.restoreProgressFromZip(zipFile, userID)
			progressCount += n
		}
	}

	return gin.H{
		"sources":  sourcesCount,
		"books":    booksCount,
		"progress": progressCount,
		"settings": settingsCount,
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
			Value:     setting.Value,
			UpdatedAt: time.Now(),
		}
		if err := s.db.Where("user_id = ? AND key = ?", userID, key).Assign(next).FirstOrCreate(&next).Error; err == nil {
			count++
		}
	}
	if count > 0 {
		_ = s.hub.Broadcast(userID, nil, gin.H{"type": "settings_update", "payload": gin.H{"key": "all"}})
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
		Name            string `json:"name"`
		Author          string `json:"author"`
		BookURL         string `json:"bookUrl"`
		CoverURL        string `json:"coverUrl"`
		Intro           string `json:"intro"`
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
		if b.Name == "" {
			continue
		}
		book := models.Book{
			UserID:   userID,
			Title:    strings.TrimSpace(b.Name),
			Author:   strings.TrimSpace(b.Author),
			URL:      strings.TrimSpace(b.BookURL),
			CoverURL: strings.TrimSpace(b.CoverURL),
			Intro:    strings.TrimSpace(b.Intro),
		}
		query := s.db.Where("user_id = ? AND title = ?", userID, book.Title)
		if book.URL != "" {
			query = s.db.Where("user_id = ? AND url = ?", userID, book.URL)
		}
		var existing models.Book
		if query.First(&existing).Error == nil {
			existing.Author = book.Author
			existing.CoverURL = book.CoverURL
			existing.Intro = book.Intro
			if book.URL != "" {
				existing.URL = book.URL
			}
			_ = s.db.Save(&existing).Error
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
			title = strings.TrimSpace(payload.Name)
		}
		if title == "" {
			title = strings.TrimSpace(payload.Title)
		}

		var book models.Book
		query := s.db.Where("user_id = ?", userID)
		if bookURL != "" {
			query = query.Where("url = ?", bookURL)
		} else if title != "" {
			query = query.Where("title = ?", title)
		} else {
			continue
		}
		if err := query.First(&book).Error; err != nil {
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
