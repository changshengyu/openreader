package api

import (
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"openreader/backend/middleware"
	"openreader/backend/services/localbook"
	"openreader/backend/services/webdavfs"
)

type localStoreItem struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Extension    string    `json:"extension"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
	IsDir        bool      `json:"isDir"`
	Importable   bool      `json:"importable"`
}

func (s *Server) listLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	relativePath, err := normalizeLocalStorePath(c.Query("path"))
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to access local store")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	resource, err := service.Stat(relativePath)
	if errors.Is(err, webdavfs.ErrNotFound) && relativePath != "" {
		if err := service.Mkdir(relativePath); err != nil {
			writeLocalStoreFilesystemError(c, err, "failed to create local store")
			return
		}
		resource, err = service.Stat(relativePath)
	}
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to read local store")
		return
	}
	if !resource.Info.IsDir() {
		if !resource.Info.Mode().IsRegular() {
			writeLocalStoreFilesystemError(c, webdavfs.ErrUnsafePath, "failed to read local store")
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read local store"})
		return
	}
	targetDir, _, err := service.Resolve(relativePath)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to read local store")
		return
	}
	recursive := c.Query("recursive") == "1" || strings.EqualFold(c.Query("recursive"), "true")

	items := make([]localStoreItem, 0)
	if recursive {
		err := filepath.WalkDir(targetDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if path == targetDir {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if strings.HasPrefix(entry.Name(), ".") {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			if !entry.IsDir() && !info.Mode().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(targetDir, path)
			if err != nil {
				return nil
			}
			items = append(items, makeLocalStoreItem(entry.Name(), cleanRelativePath(filepath.Join(relativePath, rel)), info, entry.IsDir()))
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read local store"})
			return
		}
	} else {
		entries, err := os.ReadDir(targetDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read local store"})
			return
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if !entry.IsDir() && !info.Mode().IsRegular() {
				continue
			}
			itemPath := cleanRelativePath(filepath.Join(relativePath, entry.Name()))
			items = append(items, makeLocalStoreItem(entry.Name(), itemPath, info, entry.IsDir()))
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Path) < strings.ToLower(items[j].Path)
	})

	c.JSON(http.StatusOK, gin.H{
		"path":      relativePath,
		"recursive": recursive,
		"items":     items,
	})
}

func makeLocalStoreItem(name string, itemPath string, info os.FileInfo, isDir bool) localStoreItem {
	ext := strings.ToLower(filepath.Ext(name))
	return localStoreItem{
		Name:         name,
		Path:         itemPath,
		Extension:    ext,
		Size:         info.Size(),
		LastModified: info.ModTime().UTC(),
		IsDir:        isDir,
		Importable:   !isDir && isImportableExtension(ext),
	}
}

func (s *Server) uploadToLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	upload, err := s.parseLocalStoreUpload(c)
	if upload != nil && upload.form != nil {
		defer func() {
			_ = upload.form.RemoveAll()
		}()
	}
	if err != nil {
		writeLocalStoreUploadRequestError(c, err)
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if upload.path != "" {
		if err := service.Mkdir(upload.path); err != nil {
			writeLocalStoreFilesystemError(c, err, "failed to create directory")
			return
		}
	}

	paths := make([]string, 0, len(upload.files))
	for _, file := range upload.files {
		path, err := s.saveLocalStoreUpload(c, service, upload.path, file)
		if err != nil {
			switch {
			case errors.Is(err, errLocalImportTooLarge):
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
				return
			case errors.Is(err, webdavfs.ErrUnsafePath),
				errors.Is(err, webdavfs.ErrNotDirectory),
				errors.Is(err, webdavfs.ErrIsDirectory),
				errors.Is(err, webdavfs.ErrConflict),
				errors.Is(err, errLocalStorePathInvalid):
				writeLocalStoreFilesystemError(c, err, "failed to save file")
				return
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
				return
			}
		}
		paths = append(paths, path)
	}

	c.JSON(http.StatusCreated, gin.H{"path": paths[0], "paths": paths})
}

func (s *Server) saveLocalStoreUpload(c *gin.Context, service *webdavfs.Service, parentPath string, file *multipart.FileHeader) (string, error) {
	if file.Size > s.maxLocalImportBytes() {
		return "", errLocalImportTooLarge
	}
	name, err := normalizeLocalStoreName(file.Filename)
	if err != nil {
		return "", err
	}
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	rawPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(parentPath), name))
	_, relativePath, err := service.Resolve(rawPath)
	if err != nil {
		return "", err
	}
	if err := service.Put(c.Request.Context(), relativePath, src, s.maxLocalImportBytes()); err != nil {
		if errors.Is(err, webdavfs.ErrTooLarge) {
			return "", errLocalImportTooLarge
		}
		return "", err
	}
	return relativePath, nil
}

func (s *Server) downloadFromLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	relativePath, err := normalizeLocalStorePath(c.Query("path"))
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to access local store")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot download local store root"})
		return
	}
	file, info, err := service.Open(relativePath)
	if errors.Is(err, webdavfs.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "local store item not found"})
		return
	}
	if errors.Is(err, webdavfs.ErrIsDirectory) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot download directory"})
		return
	}
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to download local store item")
		return
	}
	defer file.Close()
	if disposition := mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}); disposition != "" {
		c.Header("Content-Disposition", disposition)
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

func (s *Server) createLocalStoreDirectory(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	var req *struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decodeLocalStoreJSON(c, &req, maxLocalStoreMetadataBodyBytes, "directory name is required") {
		return
	}
	if req == nil || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "directory name is required"})
		return
	}
	name, ok := cleanLocalStoreName(c, req.Name)
	if !ok {
		return
	}
	parentPath, err := normalizeLocalStorePath(req.Path)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to create directory")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if parentPath != "" {
		if err := service.Mkdir(parentPath); err != nil {
			writeLocalStoreFilesystemError(c, err, "failed to create parent directory")
			return
		}
	}
	requestedPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(parentPath), name))
	_, relativePath, err := service.Resolve(requestedPath)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to create directory")
		return
	}
	if _, err := service.Stat(relativePath); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "failed to create directory"})
		return
	} else if !errors.Is(err, webdavfs.ErrNotFound) {
		writeLocalStoreFilesystemError(c, err, "failed to create directory")
		return
	}
	if err := service.Mkdir(relativePath); err != nil {
		if errors.Is(err, webdavfs.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "failed to create directory"})
		} else {
			writeLocalStoreFilesystemError(c, err, "failed to create directory")
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"path": relativePath})
}

func (s *Server) renameLocalStoreItem(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	var req *struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decodeLocalStoreJSON(c, &req, maxLocalStoreMetadataBodyBytes, "path and name are required") {
		return
	}
	if req == nil || strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and name are required"})
		return
	}
	relativePath, err := normalizeLocalStorePath(req.Path)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to rename local store item")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot rename local store root"})
		return
	}
	name, ok := cleanLocalStoreName(c, req.Name)
	if !ok {
		return
	}
	newRelativePath := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(relativePath)), name))
	if filepath.Dir(filepath.FromSlash(relativePath)) == "." {
		newRelativePath = name
	}
	_, newRelativePath, err = service.Resolve(newRelativePath)
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to rename local store item")
		return
	}
	if err := service.Move(relativePath, newRelativePath, true); err != nil {
		if errors.Is(err, webdavfs.ErrUnsafePath) || errors.Is(err, webdavfs.ErrNotDirectory) {
			writeLocalStoreFilesystemError(c, err, "failed to rename local store item")
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "failed to rename local store item"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": newRelativePath})
}

func (s *Server) deleteFromLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	relativePath, err := normalizeLocalStorePath(c.Query("path"))
	if err != nil {
		writeLocalStoreFilesystemError(c, err, "failed to delete local store item")
		return
	}
	service, ok := s.localStoreFileService(c)
	if !ok {
		return
	}
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete local store root"})
		return
	}
	if err := service.Remove(relativePath); err != nil && !errors.Is(err, webdavfs.ErrNotFound) {
		if errors.Is(err, webdavfs.ErrUnsafePath) || errors.Is(err, webdavfs.ErrNotDirectory) {
			writeLocalStoreFilesystemError(c, err, "failed to delete local store item")
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete local store item"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) importFromLocalStore(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)

	req, ok := decodeLocalStoreImportRequest(c)
	if !ok {
		return
	}
	plan, ok := s.prepareLocalStoreImport(c, *req)
	if !ok {
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

	for _, target := range plan.targets {
		if target.override.ImportToken != "" {
			importRequest, err := s.stagedStorageImportRequest(userID, userName, target.override.ImportToken, target.override, primaryCategoryID)
			if err != nil {
				imported = append(imported, gin.H{"path": target.relativePath, "error": err.Error()})
				continue
			}
			book, err := s.importStagedLocalBook(userID, target.override.ImportToken, importer, importRequest)
			if err != nil {
				imported = append(imported, gin.H{"path": target.relativePath, "error": err.Error()})
				continue
			}
			s.removeStagedLocalImport(userID, target.override.ImportToken)
			if len(categoryIDs) > 0 {
				_ = s.setBookCategories(s.db, userID, book.ID, categoryIDs)
			}
			item := s.bookShelfListItem(userID, book)
			imported = append(imported, gin.H{"path": target.relativePath, "book": item})
			importedBooks = append(importedBooks, item)
			continue
		}
		file := target.file
		if file.validationError != "" {
			imported = append(imported, gin.H{"path": file.relativePath, "error": file.validationError})
			continue
		}
		data, err := s.readBoundedLocalStoreImport(plan.service, file.relativePath)
		if err != nil {
			imported = append(imported, gin.H{"path": file.relativePath, "error": localStoreImportReadError(err)})
			continue
		}
		book, err := importer.Import(localbook.ImportRequest{
			UserID:     userID,
			UserName:   userName,
			FileName:   filepath.Base(filepath.FromSlash(file.relativePath)),
			Extension:  file.extension,
			Data:       data,
			Title:      target.override.Title,
			Author:     target.override.Author,
			CategoryID: primaryCategoryID,
			TOCRule:    target.override.TOCRule,
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

	_ = s.hub.Broadcast(userID, nil, gin.H{"type": "bookshelf_update", "payload": importedBooks})
	c.JSON(http.StatusOK, gin.H{"imported": imported})
}

func (s *Server) previewLocalStoreImport(c *gin.Context) {
	if !s.requireLocalStoreAccess(c) {
		return
	}
	userID, _ := middleware.UserID(c)
	req, ok := decodeLocalStoreImportRequest(c)
	if !ok {
		return
	}
	plan, ok := s.prepareLocalStoreImport(c, *req)
	if !ok {
		return
	}
	results := make([]gin.H, 0)
	for _, target := range plan.targets {
		if target.override.ImportToken != "" {
			preview, importToken, err := s.reparseStagedStorageImport(userID, target.override.ImportToken, target.override)
			if err != nil {
				results = append(results, gin.H{"path": target.relativePath, "error": err.Error(), "importToken": importToken})
				continue
			}
			results = append(results, gin.H{"path": target.relativePath, "book": preview, "importToken": importToken})
			continue
		}
		file := target.file
		if file.validationError != "" {
			results = append(results, gin.H{"path": file.relativePath, "error": file.validationError})
			continue
		}
		data, err := s.readBoundedLocalStoreImport(plan.service, file.relativePath)
		if err != nil {
			results = append(results, gin.H{"path": file.relativePath, "error": localStoreImportReadError(err)})
			continue
		}
		preview, importToken, err := s.previewStagedStorageImportData(
			userID,
			filepath.Base(filepath.FromSlash(file.relativePath)),
			file.extension,
			data,
			target.override,
		)
		if err != nil {
			results = append(results, gin.H{"path": file.relativePath, "error": err.Error(), "importToken": importToken})
			continue
		}
		results = append(results, gin.H{"path": file.relativePath, "book": preview, "importToken": importToken})
	}
	c.JSON(http.StatusOK, gin.H{"items": results})
}

type localStoreImportFile struct {
	filePath        string
	relativePath    string
	extension       string
	validationError string
}

func cleanLocalStoreName(c *gin.Context, value string) (string, bool) {
	name, err := normalizeLocalStoreName(value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return "", false
	}
	return name, true
}

func cleanRelativePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, sLocalStorePrefix(value))
	value = strings.TrimPrefix(value, "/")
	value = strings.TrimPrefix(value, "\\")
	if value == "" || value == "." {
		return ""
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func sLocalStorePrefix(value string) string {
	cleaned := filepath.Clean(value)
	if filepath.IsAbs(cleaned) {
		return filepath.VolumeName(cleaned)
	}
	return ""
}

func isImportableExtension(ext string) bool {
	switch ext {
	case ".txt", ".text", ".md", ".epub", ".pdf", ".umd", ".cbz":
		return true
	default:
		return false
	}
}
