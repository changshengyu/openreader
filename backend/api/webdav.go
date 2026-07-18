package api

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
	if c.Request.ContentLength > s.maxLocalImportBytes() {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	staged, err := os.CreateTemp(filepath.Dir(filePath), ".webdav-upload-")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := s.copyBoundedLocalImport(staged, c.Request.Body); err != nil {
		_ = staged.Close()
		if errors.Is(err, errLocalImportTooLarge) {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusBadRequest)
		return
	}
	if err := staged.Chmod(0o644); err != nil {
		_ = staged.Close()
		c.Status(http.StatusInternalServerError)
		return
	}
	if err := staged.Close(); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if err := os.Rename(stagedPath, filePath); err != nil {
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

const backupMultipartEnvelopeBytes int64 = 1 << 20

func (s *Server) importLegadoBackup(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	limits := s.portableLimits()
	if c.Request.ContentLength > limits.maxCompressed+backupMultipartEnvelopeBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limits.maxCompressed+backupMultipartEnvelopeBytes)
	userID, _ := middleware.UserID(c)
	username, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file is required"})
		return
	}
	if !strings.EqualFold(filepath.Ext(fileHeader.Filename), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file must be a zip archive"})
		return
	}
	if fileHeader.Size > limits.maxCompressed {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
		return
	}
	stagedPath, err := s.stageUploadedBackup(fileHeader, userID, limits.maxCompressed)
	if err != nil {
		writeBackupRestoreError(c, err)
		return
	}
	defer os.Remove(stagedPath)
	result, err := s.restoreBackupFile(stagedPath, userID, username)
	if err != nil {
		writeBackupRestoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

type restoreWebDAVBackupRequest struct {
	Path string `json:"path" binding:"required"`
}

func (s *Server) restoreWebDAVBackup(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)
	username, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}
	var req restoreWebDAVBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(req.Path)), ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file must be a zip archive"})
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
	limits := s.portableLimits()
	if info.Size() > limits.maxCompressed {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
		return
	}
	result, err := s.restoreBackupFile(filePath, userID, username)
	if err != nil {
		writeBackupRestoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func writeBackupRestoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errPortableBackupConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "portable backup conflicts with an existing local book"})
	case errors.Is(err, errPortableBackupLimit):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
	case errors.Is(err, errBackupRestoreTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "backup file exceeds size limit"})
	case errors.Is(err, errBackupArchiveLimit):
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup archive exceeds safety limits"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup package"})
	}
}

func (s *Server) importFromWebDAV(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)

	var req localBookImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	paths := req.requestedPaths()
	if len(paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	categoryIDs := categoryIDsFromRequest(req.CategoryID, req.CategoryIDs)
	if len(req.CategoryIDs) > 0 {
		if !s.validateCategoryIDs(c, userID, categoryIDs) {
			return
		}
	} else if !s.validateCategory(c, userID, req.CategoryID) {
		return
	}
	var primaryCategoryID *uint
	if len(categoryIDs) > 0 {
		primaryCategoryID = &categoryIDs[0]
	}

	userName, ok := s.currentUserName(c, userID)
	if !ok {
		return
	}

	importer := localbook.NewImporter(s.cfg, s.db)
	imported := make([]gin.H, 0)
	importedBooks := make([]bookListItem, 0)
	seen := make(map[string]bool)
	itemByPath := req.itemByPath()

	for _, rawPath := range paths {
		_, requestedPath, ok := s.webdavPath(c, rawPath)
		if !ok {
			return
		}
		override := itemByPath[requestedPath]
		if override.ImportToken != "" {
			if seen[requestedPath] {
				continue
			}
			seen[requestedPath] = true
			importRequest, err := s.stagedStorageImportRequest(userID, userName, override.ImportToken, override, primaryCategoryID)
			if err != nil {
				imported = append(imported, gin.H{"path": requestedPath, "error": err.Error()})
				continue
			}
			book, err := s.importStagedLocalBook(userID, override.ImportToken, importer, importRequest)
			if err != nil {
				imported = append(imported, gin.H{"path": requestedPath, "error": err.Error()})
				continue
			}
			s.removeStagedLocalImport(userID, override.ImportToken)
			if len(categoryIDs) > 0 {
				_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
			}
			item := s.bookShelfListItem(userID, book)
			imported = append(imported, gin.H{"path": requestedPath, "book": item})
			importedBooks = append(importedBooks, item)
			continue
		}
		files, ok := s.webDAVImportFiles(c, rawPath)
		if !ok {
			return
		}
		for _, file := range files {
			if seen[file.relativePath] {
				continue
			}
			seen[file.relativePath] = true
			if file.validationError != "" {
				imported = append(imported, gin.H{"path": file.relativePath, "error": file.validationError})
				continue
			}

			data, err := s.readBoundedLocalImportFile(file.filePath)
			if err != nil {
				imported = append(imported, gin.H{"path": file.relativePath, "error": err.Error()})
				continue
			}

			override := itemByPath[file.relativePath]
			book, err := importer.Import(localbook.ImportRequest{
				UserID:     userID,
				UserName:   userName,
				FileName:   filepath.Base(file.filePath),
				Extension:  file.extension,
				Data:       data,
				Title:      override.Title,
				Author:     override.Author,
				CategoryID: primaryCategoryID,
				TOCRule:    override.TOCRule,
			})
			if err != nil {
				imported = append(imported, gin.H{"path": file.relativePath, "error": err.Error()})
				continue
			}
			if len(categoryIDs) > 0 {
				_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
			}
			item := s.bookShelfListItem(userID, book)
			imported = append(imported, gin.H{"path": file.relativePath, "book": item})
			importedBooks = append(importedBooks, item)
		}
	}

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": importedBooks})
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (s *Server) previewWebDAVImport(c *gin.Context) {
	if !s.requireWebDAVAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)
	var req localBookImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	paths := req.requestedPaths()
	if len(paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths is required"})
		return
	}
	results := make([]gin.H, 0)
	seen := make(map[string]bool)
	itemByPath := req.itemByPath()
	for _, rawPath := range paths {
		_, requestedPath, ok := s.webdavPath(c, rawPath)
		if !ok {
			continue
		}
		if override, exists := itemByPath[requestedPath]; exists && override.ImportToken != "" {
			if seen[requestedPath] {
				continue
			}
			seen[requestedPath] = true
			preview, importToken, err := s.reparseStagedStorageImport(userID, override.ImportToken, override)
			if err != nil {
				results = append(results, gin.H{"path": requestedPath, "error": err.Error(), "importToken": importToken})
				continue
			}
			results = append(results, gin.H{"path": requestedPath, "book": preview, "importToken": importToken})
			continue
		}

		files, ok := s.webDAVImportFiles(c, rawPath)
		if !ok {
			continue
		}
		for _, file := range files {
			if seen[file.relativePath] {
				continue
			}
			seen[file.relativePath] = true
			if file.validationError != "" {
				results = append(results, gin.H{"path": file.relativePath, "error": file.validationError})
				continue
			}
			override := itemByPath[file.relativePath]
			preview, importToken, err := s.previewStagedStorageImport(userID, file, override)
			if err != nil {
				results = append(results, gin.H{"path": file.relativePath, "error": err.Error(), "importToken": importToken})
				continue
			}
			results = append(results, gin.H{"path": file.relativePath, "book": preview, "importToken": importToken})
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": results})
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
			return []localStoreImportFile{{filePath: filePath, relativePath: relativePath, extension: ext, validationError: "unsupported file type"}}, true
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
	return s.restoreLegadoBackupDataWithBroadcast(data, userID, true)
}

// restoreLegadoBackupDataWithoutBroadcast lets a higher-level restore add durable local archive
// state before reader clients are told that their restored shelf is ready to load.
func (s *Server) restoreLegadoBackupDataWithoutBroadcast(data []byte, userID uint) (gin.H, error) {
	return s.restoreLegadoBackupDataWithBroadcast(data, userID, false)
}

func (s *Server) restoreLegadoBackupDataWithBroadcast(data []byte, userID uint, broadcast bool) (gin.H, error) {
	archive, err := newBackupRestoreArchive(data, s.backupRestoreLimits())
	if err != nil {
		return nil, err
	}

	var sourcesCount, rssSourcesCount, booksCount, chapterVariablesCount, progressCount, settingsCount, categoriesCount, bookmarksCount, replaceRulesCount int

	// Validate all additive variable artifacts before any source/settings/shelf
	// mutation. A malformed value must not leave a partly restored account.
	for _, entry := range archive.entries {
		entryData, err := archive.dataFor(entry.file)
		if err != nil {
			return nil, err
		}
		switch {
		case strings.HasSuffix(entry.name, "myBookShelf.json"), strings.HasSuffix(entry.name, "bookshelf.json"):
			if err := validateRestoredBookshelfVariables(entryData); err != nil {
				return nil, errInvalidBackupArchive
			}
		case strings.HasSuffix(entry.name, "chapterVariables.json"):
			if err := validateRestoredChapterVariables(entryData); err != nil {
				return nil, errInvalidBackupArchive
			}
		}
	}

	for _, entry := range archive.entries {
		zipFile := entry.file
		entryData, err := archive.dataFor(zipFile)
		if err != nil {
			return nil, err
		}
		switch {
		case strings.HasSuffix(entry.name, "bookSource.json"):
			n, _ := s.restoreSourcesFromData(entryData)
			sourcesCount += n
		case strings.HasSuffix(entry.name, "rssSources.json"):
			n, _ := s.restoreRSSSourcesFromData(entryData, userID)
			rssSourcesCount += n
		case strings.HasSuffix(entry.name, "userSettings.json"):
			n, _ := s.restoreUserSettingsFromData(entryData, userID)
			settingsCount += n
		case strings.HasSuffix(entry.name, "categories.json"):
			n, _ := s.restoreCategoriesFromData(entryData, userID)
			categoriesCount += n
		}
	}

	for _, entry := range archive.entries {
		zipFile := entry.file
		entryData, err := archive.dataFor(zipFile)
		if err != nil {
			return nil, err
		}
		switch {
		case strings.HasSuffix(entry.name, "myBookShelf.json"),
			strings.HasSuffix(entry.name, "bookshelf.json"):
			restoredBooks, restoredProgress, _ := s.restoreBookshelfFromData(entryData, userID)
			booksCount += restoredBooks
			progressCount += restoredProgress
		}
	}

	// Chapter variables belong to book/chapter records. Restore them only after
	// the shelf has materialized those rows, so a valid backup cannot silently
	// lose parser state merely because archive entries are sorted differently.
	for _, entry := range archive.entries {
		if !strings.HasSuffix(entry.name, "chapterVariables.json") {
			continue
		}
		entryData, err := archive.dataFor(entry.file)
		if err != nil {
			return nil, err
		}
		n, restoreErr := s.restoreChapterVariablesFromData(entryData, userID)
		if restoreErr != nil {
			return nil, restoreErr
		}
		chapterVariablesCount += n
	}

	for _, entry := range archive.entries {
		zipFile := entry.file
		entryData, err := archive.dataFor(zipFile)
		if err != nil {
			return nil, err
		}
		switch {
		case strings.HasSuffix(entry.name, "bookmarks.json"):
			n, _ := s.restoreBookmarksFromData(entryData, userID)
			bookmarksCount += n
		case strings.HasSuffix(entry.name, "replaceRules.json"):
			n, _ := s.restoreReplaceRulesFromData(entryData, userID)
			replaceRulesCount += n
		case strings.HasSuffix(entry.name, "readingProgress.json"):
			n, _ := s.restoreProgressFromData(entryData, userID)
			progressCount += n
		case strings.Contains(entry.name, "bookProgress/"):
			n, _ := s.restoreProgressFromData(entryData, userID)
			progressCount += n
		}
	}

	result := gin.H{
		"sources":          sourcesCount,
		"rssSources":       rssSourcesCount,
		"books":            booksCount,
		"chapterVariables": chapterVariablesCount,
		"progress":         progressCount,
		"settings":         settingsCount,
		"categories":       categoriesCount,
		"bookmarks":        bookmarksCount,
		"replaceRules":     replaceRulesCount,
	}
	if broadcast {
		s.broadcastRestoreUpdates(userID, result)
	}
	return result, nil
}

func (s *Server) restoreSourcesFromZip(file *zip.File) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreSourcesFromData(data)
}

func (s *Server) restoreSourcesFromData(data []byte) (int, error) {
	sources, err := decodeBookSources(data)
	if err != nil {
		return 0, err
	}

	result := s.importBookSources(sources)
	return (result["imported"].(int) + result["updated"].(int)), nil
}

func (s *Server) restoreRSSSourcesFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreRSSSourcesFromData(data, userID)
}

func (s *Server) restoreRSSSourcesFromData(data []byte, userID uint) (int, error) {
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
			UserID:          userID,
			Title:           title,
			URL:             url,
			Icon:            strings.TrimSpace(sourceReq.Icon),
			Group:           strings.TrimSpace(sourceReq.Group),
			Comment:         strings.TrimSpace(sourceReq.Comment),
			CustomOrder:     sourceReq.orderOrDefault(s, userID),
			ConcurrentRate:  strings.TrimSpace(sourceReq.ConcurrentRate),
			Header:          sourceReq.headerText(),
			LoginURL:        strings.TrimSpace(sourceReq.LoginURL),
			LoginCheckJS:    strings.TrimSpace(sourceReq.LoginCheckJS),
			SingleURL:       sourceReq.singleURLOr(false),
			ArticleStyle:    sourceReq.articleStyleOrDefault(),
			SortURL:         strings.TrimSpace(sourceReq.SortURL),
			RuleArticles:    strings.TrimSpace(sourceReq.RuleArticles),
			RuleNextPage:    strings.TrimSpace(sourceReq.RuleNextPage),
			RuleTitle:       strings.TrimSpace(sourceReq.RuleTitle),
			RulePubDate:     strings.TrimSpace(sourceReq.RulePubDate),
			RuleDescription: strings.TrimSpace(sourceReq.RuleDescription),
			RuleImage:       strings.TrimSpace(sourceReq.RuleImage),
			RuleLink:        strings.TrimSpace(sourceReq.RuleLink),
			RuleContent:     strings.TrimSpace(sourceReq.RuleContent),
			Style:           strings.TrimSpace(sourceReq.Style),
			EnableJS:        sourceReq.enableJSOrDefault(),
			LoadWithBaseURL: sourceReq.loadWithBaseURLOrDefault(),
			Enabled:         enabled,
			UpdatedAt:       time.Now(),
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
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreUserSettingsFromData(data, userID)
}

func (s *Server) restoreUserSettingsFromData(data []byte, userID uint) (int, error) {
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
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreCategoriesFromData(data, userID)
}

func (s *Server) restoreCategoriesFromData(data []byte, userID uint) (int, error) {
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
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, 0, err
	}
	return s.restoreBookshelfFromData(data, userID)
}

type restoredBookshelfRow struct {
	Title           string   `json:"title"`
	Name            string   `json:"name"`
	Author          string   `json:"author"`
	URL             string   `json:"url"`
	BookURL         string   `json:"bookUrl"`
	SourceID        uint     `json:"sourceId"`
	SourceName      string   `json:"sourceName"`
	Type            int      `json:"type"`
	CoverURL        string   `json:"coverUrl"`
	CustomCoverURL  string   `json:"customCoverUrl"`
	Intro           string   `json:"intro"`
	Kind            string   `json:"kind"`
	WordCount       string   `json:"wordCount"`
	Variable        string   `json:"variable"`
	LastChapter     string   `json:"lastChapter"`
	ChapterCount    int      `json:"chapterCount"`
	CanUpdate       *bool    `json:"canUpdate"`
	CategoryName    string   `json:"categoryName"`
	CategoryNames   []string `json:"categoryNames"`
	OriginName      string   `json:"originName"`
	DurChapter      int      `json:"durChapter"`
	DurChapterPos   int      `json:"durChapterPos"`
	DurChapterTitle string   `json:"durChapterTitle"`
}

type restoredChapterVariableRow struct {
	SourceName   string `json:"sourceName"`
	BookURL      string `json:"bookUrl"`
	BookTitle    string `json:"bookTitle"`
	ChapterURL   string `json:"chapterUrl"`
	ChapterTitle string `json:"chapterTitle"`
	ChapterIndex int    `json:"chapterIndex"`
	Variable     string `json:"variable"`
}

func validateRestoredBookshelfVariables(data []byte) error {
	var books []restoredBookshelfRow
	if err := json.Unmarshal(data, &books); err != nil {
		return err
	}
	for index := range books {
		variable, err := models.NormalizeSourceRuleVariables(books[index].Variable)
		if err != nil {
			return err
		}
		books[index].Variable = variable
	}
	return nil
}

func validateRestoredChapterVariables(data []byte) error {
	var rows []restoredChapterVariableRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	for index := range rows {
		variable, err := models.NormalizeSourceRuleVariables(rows[index].Variable)
		if err != nil {
			return err
		}
		rows[index].Variable = variable
	}
	return nil
}

func (s *Server) restoreBookshelfFromData(data []byte, userID uint) (int, int, error) {
	var books []restoredBookshelfRow
	if err := json.Unmarshal(data, &books); err != nil {
		return 0, 0, err
	}
	if err := validateRestoredBookshelfVariables(data); err != nil {
		return 0, 0, errInvalidBackupArchive
	}
	for index := range books {
		variable, _ := models.NormalizeSourceRuleVariables(books[index].Variable)
		books[index].Variable = variable
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
		sourceID := s.restoredBookSourceID(b.SourceName, b.SourceID)
		variable := b.Variable
		if sourceID == 0 || strings.TrimSpace(b.SourceName) == "" {
			// A source token is meaningful only with a resolved remote source. This
			// also makes old/local backups safe when they contain unknown fields or
			// only a legacy numeric source ID.
			variable = ""
		}
		book := models.Book{
			UserID:         userID,
			SourceID:       sourceID,
			Type:           b.Type,
			Title:          title,
			Author:         strings.TrimSpace(b.Author),
			URL:            bookURL,
			CoverURL:       strings.TrimSpace(b.CoverURL),
			CustomCoverURL: strings.TrimSpace(b.CustomCoverURL),
			Intro:          strings.TrimSpace(b.Intro),
			Kind:           strings.TrimSpace(b.Kind),
			WordCount:      strings.TrimSpace(b.WordCount),
			Variable:       variable,
			LastChapter:    strings.TrimSpace(b.LastChapter),
			ChapterCount:   b.ChapterCount,
			CanUpdate:      canUpdate,
		}
		categoryIDs := s.restoredCategoryIDs(userID, b.CategoryName, b.CategoryNames)
		if len(categoryIDs) > 0 {
			book.CategoryID = &categoryIDs[0]
		}
		query := s.db.Where("user_id = ? AND title = ?", userID, book.Title)
		if book.URL != "" {
			query = s.db.Where("user_id = ? AND url = ?", userID, book.URL)
		}
		var existing models.Book
		if query.First(&existing).Error == nil {
			existing.SourceID = book.SourceID
			existing.Type = book.Type
			existing.Author = book.Author
			existing.CoverURL = book.CoverURL
			existing.CustomCoverURL = book.CustomCoverURL
			existing.Intro = book.Intro
			existing.Kind = book.Kind
			existing.WordCount = book.WordCount
			existing.Variable = book.Variable
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
			_ = s.setBookCategories(s.db, userID, existing.ID, categoryIDs)
			count++
			if s.restoreBookshelfProgress(userID, existing.ID, b.DurChapter, b.DurChapterPos, b.DurChapterTitle) {
				progressCount++
			}
			continue
		}
		if err := s.db.Create(&book).Error; err != nil {
			continue
		}
		_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
		if s.restoreBookshelfProgress(userID, book.ID, b.DurChapter, b.DurChapterPos, b.DurChapterTitle) {
			progressCount++
		}
		count++
	}
	return count, progressCount, nil
}

func (s *Server) restoreChapterVariablesFromData(data []byte, userID uint) (int, error) {
	var rows []restoredChapterVariableRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, err
	}
	if err := validateRestoredChapterVariables(data); err != nil {
		return 0, errInvalidBackupArchive
	}
	for index := range rows {
		variable, _ := models.NormalizeSourceRuleVariables(rows[index].Variable)
		rows[index].Variable = variable
	}

	count := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if row.ChapterIndex < 0 || strings.TrimSpace(row.Variable) == "" || strings.TrimSpace(row.SourceName) == "" {
				continue
			}
			book, ok := s.findRestoredBook(userID, row.BookURL, row.BookTitle)
			if !ok || book.SourceID == 0 {
				continue
			}
			var source models.BookSource
			if err := tx.Select("name").First(&source, book.SourceID).Error; err != nil || source.Name != strings.TrimSpace(row.SourceName) {
				continue
			}

			chapterURL := strings.TrimSpace(row.ChapterURL)
			chapterTitle := strings.TrimSpace(row.ChapterTitle)
			var chapter models.Chapter
			query := tx.Where("book_id = ? AND `index` = ?", book.ID, row.ChapterIndex)
			if err := query.First(&chapter).Error; err == nil {
				if chapterURL != "" && strings.TrimSpace(chapter.URL) != "" && chapter.URL != chapterURL {
					continue
				}
				chapter.Variable = row.Variable
				if chapter.URL == "" {
					chapter.URL = chapterURL
				}
				if chapter.Title == "" {
					chapter.Title = chapterTitle
				}
				if err := tx.Save(&chapter).Error; err != nil {
					return err
				}
				count++
				continue
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			if chapterURL == "" {
				continue
			}
			if chapterTitle == "" {
				chapterTitle = fmt.Sprintf("第 %d 章", row.ChapterIndex+1)
			}
			chapter = models.Chapter{
				BookID:   book.ID,
				Index:    row.ChapterIndex,
				Title:    chapterTitle,
				URL:      chapterURL,
				Variable: row.Variable,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func (s *Server) restoredBookSourceID(sourceName string, fallback uint) uint {
	name := strings.TrimSpace(sourceName)
	if name != "" {
		var source models.BookSource
		if err := s.db.Select("id").Where("name = ?", name).First(&source).Error; err == nil {
			return source.ID
		}
		return 0
	}
	if fallback == 0 {
		return 0
	}
	var source models.BookSource
	if err := s.db.Select("id").First(&source, fallback).Error; err == nil {
		return source.ID
	}
	return 0
}

func (s *Server) restoreBookmarksFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreBookmarksFromData(data, userID)
}

func (s *Server) restoreBookmarksFromData(data []byte, userID uint) (int, error) {
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
		chapterIndex := row.ChapterIndex
		if chapterIndex < 0 {
			chapterIndex = 0
		}
		offset := row.Offset
		if offset < 0 {
			offset = 0
		}
		chapterID := uint(0)
		var chapter models.Chapter
		if err := s.db.Where("book_id = ? AND `index` = ?", book.ID, chapterIndex).First(&chapter).Error; err == nil {
			chapterID = chapter.ID
		}
		createdAt := row.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		updatedAt := row.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = createdAt
		}
		bookmark := models.Bookmark{
			UserID:       userID,
			BookID:       book.ID,
			ChapterID:    chapterID,
			ChapterIndex: chapterIndex,
			Offset:       offset,
			Percent:      clampProgressPercent(row.Percent),
			Title:        strings.TrimSpace(row.Title),
			Excerpt:      strings.TrimSpace(row.Excerpt),
			Note:         strings.TrimSpace(row.Note),
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		// Modern OpenReader exports include CreatedAt, which is the stable identity
		// needed to retain more than one bookmark at the same chapter/offset.  Old
		// reader-dev exports may omit it, so their narrower, legacy identity remains
		// available as a read-compatibility fallback.
		identity := models.Bookmark{
			UserID:       userID,
			BookID:       book.ID,
			ChapterIndex: bookmark.ChapterIndex,
			Offset:       bookmark.Offset,
			Title:        bookmark.Title,
			Excerpt:      bookmark.Excerpt,
			Note:         bookmark.Note,
		}
		if !row.CreatedAt.IsZero() {
			identity.CreatedAt = row.CreatedAt
		}
		if err := s.db.Where(&identity).FirstOrCreate(&bookmark).Error; err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Server) restoreReplaceRulesFromZip(file *zip.File, userID uint) (int, error) {
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreReplaceRulesFromData(data, userID)
}

func (s *Server) restoreReplaceRulesFromData(data []byte, userID uint) (int, error) {
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
		isRegex := false
		if rule.IsRegex != nil {
			isRegex = *rule.IsRegex
		}
		scope := strings.TrimSpace(rule.Scope)
		if scope == "" {
			scope = "*"
		}
		next := models.ReplaceRule{
			UserID:      userID,
			Name:        name,
			Pattern:     pattern,
			Replacement: rule.Replacement,
			Scope:       scope,
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
	data, err := readBackupZipFile(file)
	if err != nil {
		return 0, err
	}
	return s.restoreProgressFromData(data, userID)
}

func (s *Server) restoreProgressFromData(data []byte, userID uint) (int, error) {
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

func (s *Server) restoredCategoryIDs(userID uint, categoryName string, categoryNames []string) []uint {
	names := make([]string, 0, len(categoryNames)+1)
	names = append(names, categoryNames...)
	if strings.TrimSpace(categoryName) != "" {
		names = append(names, categoryName)
	}
	seen := make(map[string]struct{}, len(names))
	ids := make([]uint, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if categoryID := s.findRestoredCategoryID(userID, name); categoryID != nil {
			ids = append(ids, *categoryID)
		}
	}
	return ids
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
	storeRoot, ok := s.storeRoot(c, s.webdavDir())
	if !ok {
		return "", "", false
	}
	baseDir, err := filepath.Abs(storeRoot)
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
